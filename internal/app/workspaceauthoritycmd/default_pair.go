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
	observation, err := s.observeStable(ctx)
	if err != nil {
		return DefaultPairStatus{}, readFault(err, "default_pair_read_failed", "The final default Template and current Project pair could not be observed")
	}
	result, err := NewDefaultPairStatus(observation)
	if err != nil {
		return DefaultPairStatus{}, contractFault("invalid_default_pair_status", "The final default-pair status is invalid", err)
	}
	return result, nil
}

func (s *DefaultPairService) Enter(ctx context.Context, intent operation.Intent, standardBody tobari.WorkspaceTemplateBody, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	if s == nil || portcheck.IsNil(s.authority) || portcheck.IsNil(s.initialize) || s.contexts == nil {
		return ContextEntryResult{}, missingPort("final default-pair")
	}
	if err := standardBody.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_template_body", "The reviewed first-use Template is invalid", err, "template create")
	}
	if err := session.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_arguments", "Workspace session command is invalid", err, "help "+TaskDefaultPairEnter)
	}
	target := operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskDefaultPairEnter, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: DefaultPairEnterImpact()}
	var result ContextEntryResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		observation, err := s.observeStable(actionContext)
		if err != nil {
			return defaultPairMutationFault(err)
		}
		publication, err := s.initialize.InitializeFinalDefaultPair(actionContext, observation.ProjectRoot, standardBody.Clone())
		if err != nil {
			return defaultPairInitializationFault(err)
		}
		if err := publication.ValidateFor(observation.ProjectRoot, standardBody); err != nil {
			return contractFault("invalid_default_pair_initialization", "The final default-pair initialization publication is invalid", err)
		}
		observation, err = s.observeStable(actionContext)
		if err != nil {
			return defaultPairPostInitializationFault(err, publication.Changed)
		}
		if !observation.SameReceipt(publication.Current) || !reflect.DeepEqual(observation, publication.Current) || observation.DefaultTemplate == nil || observation.Context == nil {
			return defaultPairPostInitializationFault(fmt.Errorf("default-pair authority changed after initialization"), publication.Changed)
		}
		contextRef, _ := tobari.ContextRef(observation.Context.Context.ID)
		entryIntent := operation.Intent{Command: TaskContextEnter, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}, Impact: ContextEnterImpact()}
		result, err = s.contexts.EnterDefaultPair(actionContext, entryIntent, observation.Clone(), session, in, out, errOut)
		if err != nil {
			return defaultPairPostInitializationFault(err, publication.Changed)
		}
		return nil
	})
	return result, err
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
