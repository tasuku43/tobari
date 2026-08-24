package workspaceauthoritycmd

import (
	"context"
	"fmt"
	"io"
	"reflect"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	TaskDefaultPairEnter           = "tobari"
	TaskDefaultPairStatus          = "status"
	DefaultPairStatusSchemaVersion = 3
)

type DefaultPairAuthorityPort interface {
	ObserveFinalCanonicalProjectRoot(context.Context) (string, error)
	ObserveFinalDefaultPair(context.Context, string) (tobari.FinalDefaultPairObservation, error)
}

type DefaultPairInitializePort interface {
	InitializeFinalDefaultPair(context.Context, string, tobari.WorkspaceTemplateBody) (tobari.FinalDefaultPairPublication, error)
}

// DefaultPairResolution is one invocation-local receipt for the exact default
// Template and canonical Project Context selected by root entry. It does not
// create another durable authority or selection.
type DefaultPairResolution struct {
	Observation      tobari.FinalDefaultPairObservation
	AuthorityChanged bool
}

func (r DefaultPairResolution) Validate() error {
	if err := r.Observation.Validate(); err != nil {
		return err
	}
	if r.Observation.DefaultTemplate == nil || r.Observation.Context == nil {
		return fmt.Errorf("resolved final default pair is incomplete")
	}
	return nil
}

type DefaultPairStatus struct {
	SchemaVersion                    int
	Task                             string
	ProjectRoot                      string
	AuthorityState                   string
	DefaultTemplateState             string
	WorkspaceTemplateID              tobari.WorkspaceTemplateID
	TemplateName                     string
	DesiredTemplateGeneration        uint64
	DesiredTemplateRevision          tobari.SemanticDigest
	DesiredTemplatePolicySliceDigest tobari.SemanticDigest
	ContextID                        tobari.ContextID
	ActiveTemplatePolicySliceDigest  *tobari.SemanticDigest
	CurrentPolicyMemoryRevision      tobari.SemanticDigest
	ActivePolicyMemoryRevision       *tobari.SemanticDigest
	WorkspaceID                      tobari.WorkspaceID
	WorkspaceRef                     string
	WorkspaceHome                    string
	AppliedEntry                     *tobari.WorkspaceAppliedEntry
}

func NewDefaultPairStatus(observation tobari.FinalDefaultPairObservation) (DefaultPairStatus, error) {
	if err := observation.Validate(); err != nil {
		return DefaultPairStatus{}, err
	}
	result := DefaultPairStatus{SchemaVersion: DefaultPairStatusSchemaVersion, Task: TaskDefaultPairStatus, ProjectRoot: observation.ProjectRoot, AuthorityState: "empty", DefaultTemplateState: "absent"}
	if observation.CollectionPresent {
		result.AuthorityState = "initialized"
	}
	if observation.DefaultTemplate == nil {
		return result, result.Validate()
	}
	template := observation.DefaultTemplate
	result.DefaultTemplateState = "selected"
	result.WorkspaceTemplateID = template.ID
	result.TemplateName = template.Name
	result.DesiredTemplateGeneration = template.Current.Generation
	result.DesiredTemplateRevision = template.Current.Revision
	result.DesiredTemplatePolicySliceDigest = template.Current.Slices.PolicySliceDigest
	if observation.Context == nil {
		return result, result.Validate()
	}
	snapshot := observation.Context
	axes, err := tobari.NewContextAuthorityAxes(*snapshot)
	if err != nil {
		return DefaultPairStatus{}, err
	}
	result.ContextID = snapshot.Context.ID
	result.ActiveTemplatePolicySliceDigest = axes.ActiveTemplatePolicySliceDigest
	result.CurrentPolicyMemoryRevision = axes.CurrentPolicyMemoryRevision
	result.ActivePolicyMemoryRevision = axes.ActivePolicyMemoryRevision
	result.AppliedEntry = axes.AppliedEntry
	if snapshot.Workspace != nil {
		result.WorkspaceID = snapshot.Workspace.ID
		result.WorkspaceHome = snapshot.Workspace.Home
		result.WorkspaceRef, _ = tobari.WorkspaceRef(snapshot.Workspace.ID)
	}
	return result, result.Validate()
}

func (r DefaultPairStatus) Validate() error {
	if r.SchemaVersion != DefaultPairStatusSchemaVersion || r.Task != TaskDefaultPairStatus || tobari.ValidateCanonicalRoot(r.ProjectRoot) != nil {
		return fmt.Errorf("default-pair status metadata is invalid")
	}
	if r.AuthorityState != "empty" && r.AuthorityState != "initialized" {
		return fmt.Errorf("default-pair authority state is invalid")
	}
	if r.DefaultTemplateState != "absent" && r.DefaultTemplateState != "selected" {
		return fmt.Errorf("default Template state is invalid")
	}
	if r.AuthorityState == "empty" && r.DefaultTemplateState != "absent" {
		return fmt.Errorf("empty final authority cannot select a default Template")
	}
	if r.DefaultTemplateState == "absent" {
		if r.WorkspaceTemplateID != "" || r.TemplateName != "" || r.DesiredTemplateGeneration != 0 || r.DesiredTemplateRevision != "" || r.DesiredTemplatePolicySliceDigest != "" || r.ContextID != "" || r.ActiveTemplatePolicySliceDigest != nil || r.CurrentPolicyMemoryRevision != "" || r.ActivePolicyMemoryRevision != nil || r.WorkspaceID != "" || r.WorkspaceRef != "" || r.WorkspaceHome != "" || r.AppliedEntry != nil {
			return fmt.Errorf("absent default Template status carries lower authority")
		}
		return nil
	}
	if r.WorkspaceTemplateID.Validate() != nil || tobari.ValidateName(r.TemplateName) != nil || r.DesiredTemplateGeneration == 0 || r.DesiredTemplateRevision.Validate() != nil || r.DesiredTemplatePolicySliceDigest.Validate() != nil {
		return fmt.Errorf("selected default Template status is invalid")
	}
	if r.ContextID == "" {
		if r.ActiveTemplatePolicySliceDigest != nil || r.CurrentPolicyMemoryRevision != "" || r.ActivePolicyMemoryRevision != nil || r.WorkspaceID != "" || r.WorkspaceRef != "" || r.WorkspaceHome != "" || r.AppliedEntry != nil {
			return fmt.Errorf("absent default Context status carries lower authority")
		}
		return nil
	}
	if r.ContextID.Validate() != nil || r.CurrentPolicyMemoryRevision.Validate() != nil {
		return fmt.Errorf("default Context status is invalid")
	}
	if r.ActiveTemplatePolicySliceDigest != nil && r.ActiveTemplatePolicySliceDigest.Validate() != nil {
		return fmt.Errorf("default Context active Template-policy status is invalid")
	}
	if r.ActivePolicyMemoryRevision != nil && r.ActivePolicyMemoryRevision.Validate() != nil {
		return fmt.Errorf("default Context active Policy-Memory status is invalid")
	}
	if r.WorkspaceID == "" {
		if r.WorkspaceRef != "" || r.WorkspaceHome != "" || r.AppliedEntry != nil {
			return fmt.Errorf("absent Workspace status carries applied authority")
		}
		return nil
	}
	wantRef, err := tobari.WorkspaceRef(r.WorkspaceID)
	if err != nil || wantRef != r.WorkspaceRef || r.WorkspaceHome == "" {
		return fmt.Errorf("default Workspace status is invalid")
	}
	if r.AppliedEntry != nil {
		if err := r.AppliedEntry.Validate(); err != nil {
			return fmt.Errorf("default Workspace AppliedEntry is invalid: %w", err)
		}
		if r.AppliedEntry.ContextID != r.ContextID || r.AppliedEntry.TemplateID != r.WorkspaceTemplateID {
			return fmt.Errorf("default Workspace AppliedEntry has another owner")
		}
	}
	return nil
}

type defaultPairMutationPolicy struct{}

func (defaultPairMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Command != TaskDefaultPairEnter || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.CurrentDirectoryTargetKind || intent.Target.ID != "" || intent.Target.ParentID != tobari.CurrentDirectoryTargetID {
		return fault.New(fault.KindRejected, "mutation_rejected", "default-pair entry target is not owned by Tobari", false)
	}
	return nil
}

type DefaultPairService struct {
	authority  DefaultPairAuthorityPort
	initialize DefaultPairInitializePort
	contexts   *ContextService
	mutator    *execution.Invoker
}

func NewDefaultPairService(authority DefaultPairAuthorityPort, initialize DefaultPairInitializePort, contexts *ContextService) *DefaultPairService {
	return &DefaultPairService{authority: authority, initialize: initialize, contexts: contexts, mutator: execution.New(defaultPairMutationPolicy{})}
}

func DefaultPairEnterImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *DefaultPairService) Status(ctx context.Context) (DefaultPairStatus, error) {
	observation, err := s.Observe(ctx)
	if err != nil {
		return DefaultPairStatus{}, err
	}
	result, err := NewDefaultPairStatus(observation)
	if err != nil {
		return DefaultPairStatus{}, contractFault("invalid_default_pair_status", "The final default-pair status is invalid", err)
	}
	return result, nil
}

// Observe returns the stable root pair without taking a lock or creating
// state. Root uses it only to decide whether the one fresh review is required.
func (s *DefaultPairService) Observe(ctx context.Context) (tobari.FinalDefaultPairObservation, error) {
	observation, err := s.observeStable(ctx)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, readFault(err, "default_pair_read_failed", "The final default Template and current Project pair could not be observed")
	}
	return observation, nil
}

// Resolve confirms an existing complete pair without mutation, or publishes
// the reviewed fresh Template and Context through the canonical initializer.
// A fresh body is required only for exact empty authority.
func (s *DefaultPairService) Resolve(ctx context.Context, intent operation.Intent, freshBody *tobari.WorkspaceTemplateBody) (DefaultPairResolution, error) {
	if s == nil || portcheck.IsNil(s.authority) {
		return DefaultPairResolution{}, missingPort("final default-pair")
	}
	observation, err := s.observeStable(ctx)
	if err != nil {
		return DefaultPairResolution{}, defaultPairMutationFault(err)
	}
	if observation.CollectionPresent {
		if observation.DefaultTemplate == nil {
			return DefaultPairResolution{}, defaultPairInitializationFault(tobari.ErrDefaultTemplateSelectionRequired)
		}
		if observation.Context != nil {
			resolution := DefaultPairResolution{Observation: observation.Clone()}
			if err := resolution.Validate(); err != nil {
				return DefaultPairResolution{}, contractFault("invalid_default_pair", "The final default pair is invalid", err)
			}
			return resolution, nil
		}
	}
	if portcheck.IsNil(s.initialize) {
		return DefaultPairResolution{}, missingPort("final default-pair initialization")
	}
	var body tobari.WorkspaceTemplateBody
	if observation.CollectionPresent {
		body = observation.DefaultTemplate.Current.Body.Clone()
	} else {
		if freshBody == nil {
			return DefaultPairResolution{}, invalidFault("invalid_template_body", "The reviewed first-use Template is required", fmt.Errorf("fresh Template body is absent"), "help "+TaskDefaultPairEnter)
		}
		body = freshBody.Clone()
	}
	if err := body.Validate(); err != nil {
		return DefaultPairResolution{}, invalidFault("invalid_template_body", "The reviewed first-use Template is invalid", err, "template create")
	}
	target := operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskDefaultPairEnter, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: DefaultPairEnterImpact()}
	var resolution DefaultPairResolution
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		current, err := s.observeStable(actionContext)
		if err != nil {
			return defaultPairMutationFault(err)
		}
		if !current.SameReceipt(observation) || !reflect.DeepEqual(current, observation) {
			return defaultPairMutationFault(fmt.Errorf("final authority changed before initialization"))
		}
		publication, err := s.initialize.InitializeFinalDefaultPair(actionContext, current.ProjectRoot, body.Clone())
		if err != nil {
			return defaultPairInitializationFault(err)
		}
		if err := publication.ValidateFor(current.ProjectRoot, body); err != nil {
			return contractFault("invalid_default_pair_initialization", "The final default-pair initialization publication is invalid", err)
		}
		confirmed, err := s.observeStable(actionContext)
		if err != nil {
			return defaultPairPostInitializationFault(err, publication.Changed)
		}
		if !confirmed.SameReceipt(publication.Current) || !reflect.DeepEqual(confirmed, publication.Current) {
			return defaultPairPostInitializationFault(fmt.Errorf("default-pair authority changed after initialization"), publication.Changed)
		}
		resolution = DefaultPairResolution{Observation: confirmed.Clone(), AuthorityChanged: publication.Changed}
		if err := resolution.Validate(); err != nil {
			return defaultPairPostInitializationFault(err, publication.Changed)
		}
		return nil
	})
	return resolution, err
}

// RefreshAfterCluster replaces only the collection receipt and independently
// active policy axes that canonical cluster reconciliation is allowed to
// publish. The resolved Project, default Template, Context desired authority,
// Policy Memory, and Workspace binding must remain the same entry target.
func (s *DefaultPairService) RefreshAfterCluster(ctx context.Context, resolution DefaultPairResolution, cluster FinalClusterReconciliation) (DefaultPairResolution, error) {
	if err := resolution.Validate(); err != nil {
		return DefaultPairResolution{}, invalidFault("invalid_default_pair", "The final default pair is invalid", err, "status")
	}
	if err := cluster.Validate(); err != nil {
		return DefaultPairResolution{}, contractFault("invalid_cluster_reconciliation_result", "final cluster reconciliation result is invalid", err)
	}
	current, err := s.observeStable(ctx)
	if err != nil {
		return DefaultPairResolution{}, defaultPairPostInitializationFault(err, true)
	}
	if current.CollectionGeneration != cluster.Generation || current.CollectionRevision != cluster.CollectionRevision ||
		!sameDefaultPairDesiredAuthority(resolution.Observation, current) || !currentDefaultPairActivationsMatchCluster(current, cluster) {
		return DefaultPairResolution{}, defaultPairPostInitializationFault(fmt.Errorf("default-pair authority does not match the confirmed cluster receipt"), true)
	}
	return DefaultPairResolution{Observation: current.Clone(), AuthorityChanged: resolution.AuthorityChanged}, nil
}

func sameDefaultPairDesiredAuthority(previous, current tobari.FinalDefaultPairObservation) bool {
	if previous.ProjectRoot != current.ProjectRoot || previous.DefaultTemplate == nil || current.DefaultTemplate == nil || previous.Context == nil || current.Context == nil {
		return false
	}
	return reflect.DeepEqual(previous.DefaultTemplate, current.DefaultTemplate) &&
		reflect.DeepEqual(previous.Context.Context, current.Context.Context) &&
		reflect.DeepEqual(previous.Context.Template, current.Context.Template) &&
		reflect.DeepEqual(previous.Context.PolicyMemory, current.Context.PolicyMemory) &&
		reflect.DeepEqual(previous.Context.Workspace, current.Context.Workspace)
}

func currentDefaultPairActivationsMatchCluster(current tobari.FinalDefaultPairObservation, cluster FinalClusterReconciliation) bool {
	if current.Context == nil || current.Context.ActiveTemplatePolicy == nil || current.Context.ActivePolicyMemory == nil || current.Context.ActivePolicyMemoryRef == nil {
		return false
	}
	for _, activation := range cluster.Contexts {
		if activation.ContextID != current.Context.Context.ID {
			continue
		}
		return activation.WorkspaceTemplateID == current.Context.Template.ID &&
			*current.Context.ActiveTemplatePolicy == activation.TemplatePolicy &&
			*current.Context.ActivePolicyMemoryRef == activation.PolicyMemory &&
			reflect.DeepEqual(*current.Context.ActivePolicyMemory, current.Context.PolicyMemory)
	}
	return false
}

// EnterResolved preserves the exact resolution through Context entry while
// allowing the root composition to run canonical cluster reconciliation in
// between. The optional sink is presentation-only.
func (s *DefaultPairService) EnterResolved(ctx context.Context, resolution DefaultPairResolution, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	if s == nil || s.contexts == nil {
		return ContextEntryResult{}, missingPort("final default-pair entry")
	}
	if err := resolution.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_default_pair", "The final default pair is invalid", err, "status")
	}
	if err := session.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_arguments", "Workspace session command is invalid", err, "help "+TaskDefaultPairEnter)
	}
	confirmed, err := s.observeStable(ctx)
	if err != nil {
		return ContextEntryResult{}, defaultPairPostInitializationFault(err, resolution.AuthorityChanged)
	}
	if !confirmed.SameReceipt(resolution.Observation) || !reflect.DeepEqual(confirmed, resolution.Observation) {
		return ContextEntryResult{}, defaultPairPostInitializationFault(fmt.Errorf("default-pair authority changed before entry"), resolution.AuthorityChanged)
	}
	contextRef, _ := tobari.ContextRef(resolution.Observation.Context.Context.ID)
	entryIntent := operation.Intent{Command: TaskContextEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}, Impact: ContextEnterImpact()}
	result, err := s.contexts.EnterDefaultPairWithProgress(ctx, entryIntent, confirmed.Clone(), session, progress, in, out, errOut)
	if err != nil {
		return ContextEntryResult{}, defaultPairPostInitializationFault(err, resolution.AuthorityChanged)
	}
	return result, nil
}

func (s *DefaultPairService) Enter(ctx context.Context, intent operation.Intent, standardBody tobari.WorkspaceTemplateBody, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	resolution, err := s.Resolve(ctx, intent, &standardBody)
	if err != nil {
		return ContextEntryResult{}, err
	}
	return s.EnterResolved(ctx, resolution, session, nil, in, out, errOut)
}

func (s *DefaultPairService) observeStable(ctx context.Context) (tobari.FinalDefaultPairObservation, error) {
	if s == nil || portcheck.IsNil(s.authority) {
		return tobari.FinalDefaultPairObservation{}, fmt.Errorf("final default-pair authority is unavailable")
	}
	firstRoot, err := s.authority.ObserveFinalCanonicalProjectRoot(ctx)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	first, err := s.authority.ObserveFinalDefaultPair(ctx, firstRoot)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	secondRoot, err := s.authority.ObserveFinalCanonicalProjectRoot(ctx)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	second, err := s.authority.ObserveFinalDefaultPair(ctx, secondRoot)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	if firstRoot != secondRoot || !first.SameReceipt(second) || !reflect.DeepEqual(first, second) {
		return tobari.FinalDefaultPairObservation{}, fmt.Errorf("final default Template or canonical Project changed during observation")
	}
	return second.Clone(), nil
}

func defaultPairMutationFault(err error) error {
	if classified, ok := preReleaseLegacyMutationFault(err); ok {
		return classified
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	return fault.WithClassification(fault.Wrap(fault.KindRejected, "default_pair_changed", "The final default Template and canonical Project changed before entry.", false, err, fault.NextAction{Command: "status", Reason: "Observe the current default pair before retrying."}), fault.PhasePrecondition, fault.ChangeNone)
}

func defaultPairInitializationFault(err error) error {
	if classified, ok := preReleaseLegacyMutationFault(err); ok {
		return classified
	}
	if err == tobari.ErrDefaultTemplateSelectionRequired {
		return fault.WithClassification(fault.New(fault.KindRejected, "default_template_required", "Final authority is initialized without a default Template selection.", false, fault.NextAction{Command: "template list", Reason: "Discover a Template reference, then select it with template default set."}), fault.PhasePrecondition, fault.ChangeNone)
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	return err
}

func defaultPairPostInitializationFault(err error, initialized bool) error {
	if !initialized {
		return defaultPairMutationFault(err)
	}
	if public, ok := fault.PublicCopy(err); ok && public.ChangeState != fault.ChangeNone && public.ChangeState != fault.ChangeNotApplicable {
		return public
	}
	return fault.WithClassification(fault.New(fault.KindUnavailable, "default_pair_initialized", "The final default Template and Context were confirmed, but Workspace entry did not complete.", false, fault.NextAction{Command: "status", Reason: "Observe the confirmed default pair before entering again."}), fault.PhaseMutation, fault.ChangePartial)
}
