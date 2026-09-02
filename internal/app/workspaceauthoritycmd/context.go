package workspaceauthoritycmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type ContextReadPort interface {
	ListContextAuthority(context.Context) ([]ContextSnapshot, error)
	ReadContextAuthorityByReference(context.Context, string) (ContextSnapshot, error)
}

type CurrentContextReadPort interface {
	ReadCurrentContextAuthority(context.Context) (tobari.ContextAuthoritySnapshot, error)
}

type CurrentContextSelectionPort interface {
	SetCurrentContextByReference(context.Context, string) (tobari.ContextSelectionResult, error)
}

type ContextSourceObservationPort interface {
	ObserveContextSource(context.Context, tobari.ContextBinding) (tobari.ResourceSourceObservation, error)
}

type ContextDraftReadPort interface {
	ListContextDrafts(context.Context) ([]tobari.ContextDraft, error)
}

type ContextDraftCreatePort interface {
	CreateContextDraftByTemplateReference(context.Context, string) (tobari.ContextDraft, error)
}
type ContextPlanPort interface {
	PlanContextSourceByReference(context.Context, string) (tobari.ContextActivationPlan, error)
}
type ContextApplyPlanPort interface {
	ApplyContextSourceByPlan(context.Context, string) (ContextSnapshot, bool, error)
}

type ContextEnterPort interface {
	EnterContextByReference(context.Context, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

type RootedContextEnterPort interface {
	EnterContextByReferenceAtRoot(context.Context, string, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

type DefaultPairContextEnterPort interface {
	EnterFinalDefaultPair(context.Context, tobari.FinalDefaultPairObservation, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

type DefaultPairContextEnterProgressPort interface {
	EnterFinalDefaultPairWithProgress(context.Context, tobari.FinalDefaultPairObservation, string, tobari.WorkspaceSessionRequest, tobari.FirstEntryProgressSink, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

// CurrentDefaultPairContextEnterPort is the mutation-free steady-entry seam.
// Implementations may only borrow a Workspace after exact live confirmation;
// they must not prepare, build, reconcile, or publish authority.
type CurrentDefaultPairContextEnterPort interface {
	EnterCurrentFinalDefaultPair(context.Context, tobari.FinalDefaultPairObservation, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

type ContextDeletePort interface {
	DeleteContextByReference(context.Context, string) (tobari.ContextDeleteResult, error)
}

// ContextDeleteWithReadinessPort keeps the generic Docker readiness probe
// inside the adapter's fresh-action fence. An already-durable delete decision
// must bypass this probe so the exact initiating action remains recoverable.
type ContextDeleteWithReadinessPort interface {
	DeleteContextByReferenceWithReadiness(context.Context, string, func(context.Context) error) (tobari.ContextDeleteResult, error)
}

type ContextDeleteReadinessPort interface {
	Check(context.Context) error
}

type ContextEntryResult struct {
	ContextRef   string
	WorkspaceRef string
	Snapshot     ContextSnapshot
	Outcome      tobari.WorkspaceSessionOutcome
}

type contextMutationPolicy struct{}

func (contextMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ContextReferenceKind && intent.Target.ID == "" && intent.Target.ParentID != "":
		_, err := tobari.ParseWorkspaceTemplateRef(intent.Target.ParentID)
		return err
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.WorkspaceReferenceKind && intent.Target.ID == "" && intent.Target.ParentID != "":
		_, err := tobari.ParseContextRef(intent.Target.ParentID)
		return err
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ContextReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseContextRef(intent.Target.ID)
		return err
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ContextActivationPlanReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseContextActivationPlanRef(intent.Target.ID)
		return err
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Context mutation target is not owned by Tobari", false)
	}
}

type ContextService struct {
	read        ContextReadPort
	currentRead CurrentContextReadPort
	currentSet  CurrentContextSelectionPort
	draftCreate ContextDraftCreatePort
	planner     ContextPlanPort
	applyPlan   ContextApplyPlanPort
	enter       ContextEnterPort
	delete      ContextDeletePort
	deleteReady ContextDeleteWithReadinessPort
	readiness   ContextDeleteReadinessPort
	sources     ContextSourceObservationPort
	drafts      ContextDraftReadPort
	mutator     *execution.Invoker
}

func NewContextService(port any, readiness ...any) *ContextService {
	service := &ContextService{mutator: execution.New(contextMutationPolicy{})}
	service.read, _ = port.(ContextReadPort)
	service.currentRead, _ = port.(CurrentContextReadPort)
	service.currentSet, _ = port.(CurrentContextSelectionPort)
	service.draftCreate, _ = port.(ContextDraftCreatePort)
	service.planner, _ = port.(ContextPlanPort)
	service.applyPlan, _ = port.(ContextApplyPlanPort)
	service.enter, _ = port.(ContextEnterPort)
	service.delete, _ = port.(ContextDeletePort)
	service.deleteReady, _ = port.(ContextDeleteWithReadinessPort)
	if len(readiness) == 1 {
		service.readiness, _ = readiness[0].(ContextDeleteReadinessPort)
	}
	service.sources, _ = port.(ContextSourceObservationPort)
	service.drafts, _ = port.(ContextDraftReadPort)
	return service
}

func ContextCreateImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func ContextApplyImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func ContextUseImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}
}

type ContextApplyResult struct {
	View    ContextView
	Changed bool
}

func ContextEnterImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func ContextDeleteImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *ContextService) List(ctx context.Context) (ContextList, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return ContextList{}, missingPort("Context read")
	}
	snapshots, err := s.read.ListContextAuthority(ctx)
	if err != nil {
		return ContextList{}, readFault(err, "context_read_failed", "Contexts could not be read")
	}
	result, err := NewContextList(snapshots)
	if err != nil {
		return ContextList{}, contractFault("invalid_context_list", "Context list is invalid", err)
	}
	if !portcheck.IsNil(s.currentRead) {
		current, currentErr := s.currentRead.ReadCurrentContextAuthority(ctx)
		if currentErr != nil && !errors.Is(currentErr, tobari.ErrCurrentContextRequired) {
			return ContextList{}, readFault(currentErr, "current_context_read_failed", "Current Context could not be read")
		}
		for index := range result.Items {
			value := false
			result.Items[index].Current = &value
		}
		if currentErr == nil {
			found := false
			for index := range result.Items {
				if result.Items[index].Snapshot.Context.ID == current.Context.ID {
					value := true
					result.Items[index].Current = &value
					found = true
					break
				}
			}
			if !found {
				return ContextList{}, contractFault("invalid_context_list", "Current Context is absent from Context discovery", fmt.Errorf("selected Context %s is missing", current.Context.ID))
			}
		}
	}
	if !portcheck.IsNil(s.sources) {
		for index := range result.Items {
			observation, observeErr := s.sources.ObserveContextSource(ctx, result.Items[index].Snapshot.Context)
			if observeErr != nil {
				return ContextList{}, readFault(observeErr, "context_source_read_failed", "Context source could not be inspected")
			}
			result.Items[index].Source = &observation
		}
	}
	if !portcheck.IsNil(s.drafts) {
		drafts, draftErr := s.drafts.ListContextDrafts(ctx)
		if draftErr != nil {
			return ContextList{}, readFault(draftErr, "context_source_read_failed", "Context drafts could not be read")
		}
		result.Drafts = make([]ContextDraftView, len(drafts))
		for index, draft := range drafts {
			view, viewErr := NewContextDraftView(draft)
			if viewErr != nil {
				return ContextList{}, contractFault("invalid_context_list", "Context draft is invalid", viewErr)
			}
			result.Drafts[index] = view
		}
	}
	return result, nil
}

func (s *ContextService) ShowResource(ctx context.Context, contextRef string) (ContextResourceView, error) {
	active, err := s.Show(ctx, contextRef)
	if err == nil {
		return ContextResourceView{Active: &active}, nil
	}
	if portcheck.IsNil(s.drafts) {
		return ContextResourceView{}, err
	}
	id, parseErr := tobari.ParseContextRef(contextRef)
	if parseErr != nil {
		return ContextResourceView{}, err
	}
	drafts, draftErr := s.drafts.ListContextDrafts(ctx)
	if draftErr != nil {
		return ContextResourceView{}, readFault(draftErr, "context_source_read_failed", "Context drafts could not be read")
	}
	for _, draft := range drafts {
		if draft.Source.ContextID == id {
			view, viewErr := NewContextDraftView(draft)
			if viewErr != nil {
				return ContextResourceView{}, contractFault("invalid_context", "Context draft is invalid", viewErr)
			}
			return ContextResourceView{Draft: &view}, nil
		}
	}
	return ContextResourceView{}, err
}

func (s *ContextService) Show(ctx context.Context, contextRef string) (ContextView, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return ContextView{}, missingPort("Context read")
	}
	id, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextView{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	snapshot, err := s.read.ReadContextAuthorityByReference(ctx, contextRef)
	if errors.Is(err, tobari.ErrContextBindingNotFound) {
		return ContextView{}, notFoundFault("context_not_found", "Context does not exist", "context list")
	}
	if err != nil {
		return ContextView{}, readFault(err, "context_read_failed", "Context could not be read")
	}
	if snapshot.Context.ID != id {
		return ContextView{}, contractFault("invalid_context", "Context read returned another authority", fmt.Errorf("Context reference mismatch"))
	}
	view, err := NewContextView(snapshot)
	if err != nil || view.ContextRef != contextRef {
		if err == nil {
			err = fmt.Errorf("Context reference was not re-emitted unchanged")
		}
		return ContextView{}, contractFault("invalid_context", "Context is invalid", err)
	}
	if !portcheck.IsNil(s.sources) {
		observation, observeErr := s.sources.ObserveContextSource(ctx, snapshot.Context)
		if observeErr != nil {
			return ContextView{}, readFault(observeErr, "context_source_read_failed", "Context source could not be inspected")
		}
		view.Source = &observation
	}
	return view, nil
}

// ResolveCurrentOrOverride resolves Context authority without observing CWD.
// An explicit opaque reference wins for this invocation only; omission reads
// the installation-owned current selection.
func (s *ContextService) ResolveCurrentOrOverride(ctx context.Context, contextRef string) (ContextView, error) {
	if contextRef != "" {
		return s.Show(ctx, contextRef)
	}
	if s == nil || portcheck.IsNil(s.currentRead) {
		return ContextView{}, missingPort("current Context read")
	}
	snapshot, err := s.currentRead.ReadCurrentContextAuthority(ctx)
	if errors.Is(err, tobari.ErrCurrentContextRequired) {
		return ContextView{}, fault.WithClassification(fault.New(
			fault.KindRejected, "current_context_required", "No current Context is selected.", false,
			fault.NextAction{Command: "context list", Reason: "Discover a Context reference, then select it with context use."},
		), fault.PhasePrecondition, fault.ChangeNone)
	}
	if err != nil {
		return ContextView{}, readFault(err, "context_read_failed", "Current Context could not be read")
	}
	ref, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		return ContextView{}, contractFault("invalid_context", "Current Context is invalid", err)
	}
	return s.Show(ctx, ref)
}

func (s *ContextService) Use(ctx context.Context, intent operation.Intent, contextRef string) (tobari.ContextSelectionResult, error) {
	if s == nil || portcheck.IsNil(s.currentSet) {
		return tobari.ContextSelectionResult{}, missingPort("current Context selection")
	}
	id, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return tobari.ContextSelectionResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextUse, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: ContextUseImpact()}
	var result tobari.ContextSelectionResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		selected, selectErr := s.currentSet.SetCurrentContextByReference(actionContext, contextRef)
		if selectErr != nil {
			return contextMutationFault(selectErr)
		}
		if selected.ContextID != id || !selected.Selected {
			return contractFault("invalid_context_use_result", "Context selection returned another authority", fmt.Errorf("Context reference mismatch"))
		}
		result = selected
		return nil
	})
	return result, err
}

func (s *ContextService) CreateDraft(ctx context.Context, intent operation.Intent, templateRef string) (ContextDraftView, error) {
	if s == nil || portcheck.IsNil(s.draftCreate) {
		return ContextDraftView{}, missingPort("Context draft create")
	}
	if _, err := tobari.ParseWorkspaceTemplateRef(templateRef); err != nil {
		return ContextDraftView{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: templateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextCreate, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: ContextCreateImpact()}
	var result ContextDraftView
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		draft, err := s.draftCreate.CreateContextDraftByTemplateReference(actionContext, templateRef)
		if err != nil {
			return contextMutationFault(err)
		}
		view, err := NewContextDraftView(draft)
		if err != nil {
			return contractFault("invalid_context_create_result", "created Context draft is invalid", err)
		}
		result = view
		return nil
	})
	return result, err
}

func (s *ContextService) Plan(ctx context.Context, contextRef string) (tobari.ContextActivationPlan, error) {
	if s == nil || portcheck.IsNil(s.planner) {
		return tobari.ContextActivationPlan{}, missingPort("Context activation planning")
	}
	if _, err := tobari.ParseContextRef(contextRef); err != nil {
		return tobari.ContextActivationPlan{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	plan, err := s.planner.PlanContextSourceByReference(ctx, contextRef)
	if err != nil {
		return tobari.ContextActivationPlan{}, contextPlanFault(err)
	}
	if err := plan.Validate(); err != nil || plan.ContextRef != contextRef {
		return tobari.ContextActivationPlan{}, contractFault("invalid_context_activation_plan", "Context activation plan is invalid", err)
	}
	return plan, nil
}

func contextPlanFault(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	if errors.Is(err, tobari.ErrFinalAuthorityMigrationRequired) || isPreReleaseLegacyAuthority(err) {
		return readFault(err, "context_plan_read_failed", "Context activation plan could not be read")
	}
	switch {
	case errors.Is(err, tobari.ErrResourceSourceMissing):
		return notFoundFault("resource_source_missing", "The exact Context source file is missing", "context list")
	case errors.Is(err, tobari.ErrResourceSourceInvalid):
		return fault.WithClassification(fault.New(fault.KindInvalidInput, "resource_source_invalid", "The exact Context source does not satisfy its strict schema", false, fault.NextAction{Command: "context list", Reason: "Rediscover the retained Context and correct the typed source diagnostic."}), fault.PhaseObservation, fault.ChangeNotApplicable)
	case errors.Is(err, tobari.ErrWorkspaceTemplateNotFound), errors.Is(err, tobari.ErrContextBindingNotFound):
		return notFoundFault("authority_not_found", "Context planning authority no longer exists", "context list")
	case errors.Is(err, tobari.ErrContextBindingExists):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_exists", "the Project and Workspace Template already have a Context", false, fault.NextAction{Command: "context list", Reason: "Use the existing Context reference."}), fault.PhaseObservation, fault.ChangeNotApplicable)
	default:
		return readFault(err, "context_plan_read_failed", "Context activation plan could not be read")
	}
}

func (s *ContextService) Apply(ctx context.Context, intent operation.Intent, contextRef string) (ContextApplyResult, error) {
	if s == nil || portcheck.IsNil(s.applyPlan) {
		return ContextApplyResult{}, missingPort("Context source apply")
	}
	id, planErr := tobari.ParseContextActivationPlanRef(contextRef)
	if planErr != nil {
		return ContextApplyResult{}, invalidFault("invalid_context_activation_plan_ref", "Context activation plan reference is invalid", planErr, "context list")
	}
	target := operation.TargetRef{Kind: tobari.ContextActivationPlanReferenceKind, ID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextApply, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: ContextApplyImpact()}
	var result ContextApplyResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		snapshot, changed, applyErr := s.applyPlan.ApplyContextSourceByPlan(actionContext, contextRef)
		if applyErr != nil {
			return contextMutationFault(applyErr)
		}
		if snapshot.Context.ID != id {
			return contractFault("invalid_context_apply_result", "Context source Apply returned another authority", fmt.Errorf("Context reference mismatch"))
		}
		view, viewErr := NewContextView(snapshot)
		if viewErr != nil {
			return contractFault("invalid_context_apply_result", "Context source Apply result is invalid", viewErr)
		}
		if !portcheck.IsNil(s.sources) {
			observation, observeErr := s.sources.ObserveContextSource(actionContext, snapshot.Context)
			if observeErr != nil || observation.State != tobari.ResourceSourceInSync {
				return contractFault("invalid_context_apply_result", "Applied Context source is not current", observeErr)
			}
			view.Source = &observation
		}
		result = ContextApplyResult{View: view, Changed: changed}
		return nil
	})
	return result, err
}

func (s *ContextService) Enter(ctx context.Context, intent operation.Intent, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	return s.runEntry(ctx, intent, contextRef, session, in, out, errOut, nil)
}

func (s *ContextService) EnterAtRoot(ctx context.Context, intent operation.Intent, contextRef, projectRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_root", "Workspace Project root is invalid", err, "tobari")
	}
	return s.runEntryWithProgress(ctx, intent, contextRef, session, in, out, errOut, nil, projectRoot, nil, false)
}

func (s *ContextService) EnterDefaultPair(ctx context.Context, intent operation.Intent, observation tobari.FinalDefaultPairObservation, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	return s.EnterDefaultPairWithProgress(ctx, intent, observation, observation.ProjectRoot, session, nil, in, out, errOut)
}

func (s *ContextService) EnterDefaultPairWithProgress(ctx context.Context, intent operation.Intent, observation tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	return s.enterDefaultPair(ctx, intent, observation, invocationRoot, session, progress, false, in, out, errOut)
}

func (s *ContextService) EnterCurrentDefaultPair(ctx context.Context, intent operation.Intent, observation tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	return s.enterDefaultPair(ctx, intent, observation, invocationRoot, session, nil, true, in, out, errOut)
}

func (s *ContextService) enterDefaultPair(ctx context.Context, intent operation.Intent, observation tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, currentOnly bool, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	if err := observation.Validate(); err != nil || observation.Context == nil {
		if err == nil {
			err = fmt.Errorf("default-pair Context is absent")
		}
		return ContextEntryResult{}, invalidFault("invalid_default_pair", "The final default pair is invalid", err, "status")
	}
	if err := tobari.ValidateRootContains(observation.ProjectRoot, invocationRoot); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_default_pair", "The final default pair invocation root is invalid", err, "status")
	}
	contextRef, err := tobari.ContextRef(observation.Context.Context.ID)
	if err != nil {
		return ContextEntryResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "status")
	}
	value := observation.Clone()
	return s.runEntryWithProgress(ctx, intent, contextRef, session, in, out, errOut, &value, invocationRoot, progress, currentOnly)
}

func (s *ContextService) runEntry(ctx context.Context, intent operation.Intent, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer, defaultPair *tobari.FinalDefaultPairObservation) (ContextEntryResult, error) {
	return s.runEntryWithProgress(ctx, intent, contextRef, session, in, out, errOut, defaultPair, "", nil, false)
}

func (s *ContextService) runEntryWithProgress(ctx context.Context, intent operation.Intent, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer, defaultPair *tobari.FinalDefaultPairObservation, invocationRoot string, progress tobari.FirstEntryProgressSink, currentOnly bool) (ContextEntryResult, error) {
	if s == nil || portcheck.IsNil(s.enter) {
		return ContextEntryResult{}, missingPort("Context entry")
	}
	contextID, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextEntryResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	if err := session.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_arguments", "Workspace session command is invalid", err, "tobari")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextEnter, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: ContextEnterImpact()}
	var result ContextEntryResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var publication tobari.ContextEntryPublication
		var err error
		if defaultPair == nil {
			if invocationRoot == "" {
				publication, err = s.enter.EnterContextByReference(actionContext, contextRef, session, in, out, errOut)
			} else if port, ok := s.enter.(RootedContextEnterPort); ok && !portcheck.IsNil(port) {
				publication, err = port.EnterContextByReferenceAtRoot(actionContext, contextRef, invocationRoot, session, in, out, errOut)
			} else {
				return missingPort("rooted Context entry")
			}
		} else {
			if currentOnly {
				port, ok := s.enter.(CurrentDefaultPairContextEnterPort)
				if !ok || portcheck.IsNil(port) {
					return missingPort("current final default-pair entry")
				}
				publication, err = port.EnterCurrentFinalDefaultPair(actionContext, defaultPair.Clone(), invocationRoot, session, in, out, errOut)
			} else if port, ok := s.enter.(DefaultPairContextEnterProgressPort); ok && !portcheck.IsNil(port) {
				publication, err = port.EnterFinalDefaultPairWithProgress(actionContext, defaultPair.Clone(), invocationRoot, session, progress, in, out, errOut)
			} else {
				port, ok := s.enter.(DefaultPairContextEnterPort)
				if !ok || portcheck.IsNil(port) {
					return missingPort("final default-pair entry")
				}
				publication, err = port.EnterFinalDefaultPair(actionContext, defaultPair.Clone(), invocationRoot, session, in, out, errOut)
			}
		}
		if err != nil {
			return contextMutationFault(err)
		}
		if err := publication.Snapshot.Validate(); err != nil {
			return contractFault("invalid_context_entry_result", "Context entry authority is invalid", err)
		}
		if err := publication.Outcome.Validate(); err != nil {
			return contractFault("invalid_context_entry_result", "Workspace session outcome is invalid", err)
		}
		snapshot := publication.Snapshot
		if snapshot.Context.ID != contextID || snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil || snapshot.Workspace.LastSuccessfulEntry.TemplateRevision != snapshot.Template.Current.Revision || snapshot.ActiveTemplatePolicy == nil || snapshot.ActiveTemplatePolicy.PolicySliceDigest != snapshot.Template.Current.Slices.PolicySliceDigest || snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || snapshot.ActivePolicyMemory.Revision != snapshot.PolicyMemory.Revision {
			return contractFault("invalid_context_entry_result", "Context entry did not confirm current Template, Policy Memory, and Workspace authority", fmt.Errorf("entry publication mismatch"))
		}
		workspaceRef, err := tobari.WorkspaceRef(snapshot.Workspace.ID)
		if err != nil {
			return contractFault("invalid_context_entry_result", "Context entry Workspace reference is invalid", err)
		}
		result = ContextEntryResult{ContextRef: contextRef, WorkspaceRef: workspaceRef, Snapshot: snapshot.Clone(), Outcome: publication.Outcome}
		return nil
	})
	return result, err
}

func (s *ContextService) Delete(ctx context.Context, intent operation.Intent, contextRef string) (ContextDeleteResult, error) {
	if s == nil || portcheck.IsNil(s.delete) && portcheck.IsNil(s.deleteReady) {
		return ContextDeleteResult{}, missingPort("Context delete")
	}
	id, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextDeleteResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextDelete, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: ContextDeleteImpact()}
	var result ContextDeleteResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var deleted tobari.ContextDeleteResult
		var err error
		if !portcheck.IsNil(s.deleteReady) {
			if portcheck.IsNil(s.readiness) {
				return missingPort("Context delete readiness")
			}
			deleted, err = s.deleteReady.DeleteContextByReferenceWithReadiness(actionContext, contextRef, s.readiness.Check)
		} else {
			deleted, err = s.delete.DeleteContextByReference(actionContext, contextRef)
		}
		if err != nil {
			return contextMutationFault(err)
		}
		if !deleted.Deleted || deleted.ContextID != id {
			return contractFault("invalid_context_delete_result", "Context deletion is not confirmed", fmt.Errorf("delete result mismatch"))
		}
		result = deleted
		return nil
	})
	return result, err
}

func contextMutationFault(err error) error {
	// Infrastructure owns the durable mutation decision boundary. Preserve a
	// valid structured outcome before inspecting its private sentinel cause;
	// otherwise a post-decision ErrContextBindingProtected would be rewritten
	// as the precondition-only context_in_use/ChangeNone fault.
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	if classified, ok := preReleaseLegacyMutationFault(err); ok {
		return classified
	}
	if classified, ok := finalAuthorityMutationRecoveryFault(err); ok {
		return classified
	}
	switch {
	case errors.Is(err, tobari.ErrResourceSourceMissing):
		return fault.WithClassification(fault.New(fault.KindNotFound, "resource_source_missing", "The exact Context source file is missing", false, fault.NextAction{Command: "context list", Reason: "Rediscover the retained Context and canonical source path; missing source never deletes active authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrResourceSourceInvalid):
		return fault.WithClassification(fault.New(fault.KindInvalidInput, "resource_source_invalid", "The exact Context source does not satisfy its strict schema", false, fault.NextAction{Command: "context list", Reason: "Rediscover the retained Context and correct the typed source diagnostic."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrResourceSourceModified):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_identity_immutable", "Context identity fields are immutable after creation", false, fault.NextAction{Command: "context list", Reason: "Rediscover the current binding; another Template requires a fresh Context."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_activation_plan_stale", "The reviewed Context activation plan no longer matches source or active authority", false, fault.NextAction{Command: "context list", Reason: "Discover the Context again, then create and review a fresh exact activation plan."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "template_not_found", "Workspace Template no longer exists", false, fault.NextAction{Command: "template list", Reason: "Discover current Template authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingExists):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_exists", "the Project and Workspace Template already have a Context", false, fault.NextAction{Command: "context list", Reason: "Use the existing Context reference."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "context_not_found", "Context no longer exists", false, fault.NextAction{Command: "context list", Reason: "Discover current Context authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable) && errors.Is(err, tobari.ErrContextBindingProtected):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_entry_busy", "Workspace entry is temporarily blocked by a live Workspace session or an exclusive Context Home operation", true, err, fault.NextAction{Command: "status", Reason: "Read current authority, then retry after the blocking Workspace session, Configurator, or Context Home retirement finishes."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingProtected):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_in_use", "Context still owns a Workspace, attachment, or research credential", false, fault.NextAction{Command: "context list", Reason: "Remove the exact blocking authority before Context deletion."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceEntryReconciliationConfirmed):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_entry_attachment_unavailable", "Workspace entry reconciliation is confirmed, but the interactive attachment did not start", false, err, fault.NextAction{Command: "context list", Reason: "Discover the confirmed Context authority before another explicit entry."}), fault.PhaseAttachment, fault.ChangeConfirmed)
	case errors.Is(err, tobari.ErrWorkspaceEntryInterrupted):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_entry_interrupted", "Workspace entry reconciliation requires exact same-Context recovery", false, err, fault.NextAction{Command: "tobari", Reason: "Repeat root entry so it can reconcile the exact CWD-selected Workspace."}), fault.PhaseMutation, fault.ChangePartial)
	case errors.Is(err, tobari.ErrWorkspaceEntryTemplatePolicyInactive):
		return fault.WithClassification(fault.Wrap(fault.KindRejected, "workspace_entry_template_policy_inactive", "Workspace entry requires the current Template policy activation", false, err, fault.NextAction{Command: "cluster status", Reason: "Read the current Template policy activation before explicit cluster reconciliation."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceEntryPolicyMemoryInactive):
		return fault.WithClassification(fault.Wrap(fault.KindRejected, "workspace_entry_policy_memory_inactive", "Workspace entry requires the current Policy Memory activation", false, err, fault.NextAction{Command: "context list", Reason: "Discover current Context authority before explicit policy reconciliation."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceEntryRuntimeNotCurrent), errors.Is(err, tobari.ErrWorkspaceEntryProtectionNotCurrent):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_entry_repair_required", "Workspace entry requires canonical Runtime or protection recovery", true, err, fault.NextAction{Command: "tobari", Reason: "Repeat root entry so readiness and the staged recovery flow can reconcile the exact current Workspace."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceRuntimePreparationUncertain):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_runtime_preparation_uncertain", "Standard Runtime preparation did not reach a classified Workspace entry decision", false, err, fault.NextAction{Command: "status", Reason: "Read current authority and Runtime material before deciding whether to retry entry."}), fault.PhaseMutation, fault.ChangeUnknown)
	case errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable):
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "workspace_entry_observation_unavailable", "Workspace entry prerequisites could not be observed exactly", false, err, fault.NextAction{Command: "context list", Reason: "Discover desired, applied, and active Context authority without reconciling it."}), fault.PhaseObservation, fault.ChangeNotApplicable)
	case errors.Is(err, tobari.ErrWorkspaceEntryCanceledBeforeDecision):
		return fault.WithClassification(fault.Wrap(fault.KindCanceled, "workspace_entry_canceled", "Workspace entry was canceled before a durable reconciliation decision", false, err, fault.NextAction{Command: "context list", Reason: "Discover current Context authority before deciding whether to enter again."}), fault.PhasePrecondition, fault.ChangeNone)
	default:
		return err
	}
}
