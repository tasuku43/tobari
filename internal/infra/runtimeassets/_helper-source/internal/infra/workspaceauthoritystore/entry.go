package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// WorkspaceEntryRuntimeAuthority owns bounded runtime planning, mutation, and
// exact Docker observation. Planning is read-only. Reconcile must be
// receipt-idempotent for one unchanged decision reference because its effect
// can precede an interrupted final-envelope publication.
type WorkspaceEntryRuntimeAuthority interface {
	AcquireWorkspaceEntryAttachment(context.Context, tobari.ContextID, string) (func() error, error)
	AcquireWorkspaceReconciliationFence(context.Context) (func() error, error)
	ContextHomeForID(context.Context, tobari.ContextID) (string, error)
	PrepareWorkspaceRuntimeMaterial(context.Context, tobari.RuntimeBinding) error
	PlanWorkspaceEntry(context.Context, tobari.ContextAuthoritySnapshot, tobari.WorkspaceTemplateEntryAuthority, string, tobari.WorkspaceID, time.Time) (tobari.WorkspaceEntryReconciliationPlan, error)
	ReconcileWorkspaceEntry(context.Context, tobari.WorkspaceEntryReconciliationPlan, string) (tobari.WorkspaceEntryReconciliationReceipt, error)
	ConfirmWorkspaceEntry(context.Context, tobari.WorkspaceEntryReconciliationPlan, string) (tobari.WorkspaceEntryReconciliationReceipt, error)
}

// WorkspacePolicyActivationAuthority confirms both independently selected
// policy axes through one exact live aggregate observation. Separate semantic
// receipts remain mandatory; duplicated Gateway/OPA observation does not.
type WorkspacePolicyActivationAuthority interface {
	ObserveWorkspacePolicyAxesCurrent(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.TemplatePolicyActivationReceipt, tobari.PolicyMemoryActivationReceipt) (bool, error)
	ConfirmWorkspacePolicyAxesActive(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.TemplatePolicyActivationReceipt, tobari.PolicyMemoryActivationReceipt) error
}

// WorkspaceSessionOwner is the task-owned result required from the deferred
// concrete WP07 final-identity bridge. Begin happens under the installation
// lifecycle lock so Workspace deletion cannot pass between confirmed entry and
// attachment ownership. Run and Close happen after that lock is released and
// own the complete session lifetime.
type WorkspaceSessionOwner interface {
	Run(context.Context, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.WorkspaceSessionOutcome, error)
	Close(context.Context) error
}

type workspaceSessionHandoffOwner interface {
	RunWithHandoff(context.Context, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer, func()) (tobari.WorkspaceSessionOutcome, error)
}

// workspaceEntryAttachmentOwner keeps the shared Context/Project attachment
// fence for the complete interactive session, not only runtime reconciliation.
// That makes Workspace entry and the direct-egress Configurator mutually
// exclusive while either can mutate the same Context Home.
type workspaceEntryAttachmentOwner struct {
	owner   WorkspaceSessionOwner
	release func() error
	once    sync.Once
	err     error
}

func (o *workspaceEntryAttachmentOwner) Run(ctx context.Context, request tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (tobari.WorkspaceSessionOutcome, error) {
	return o.owner.Run(ctx, request, in, out, errOut)
}

func (o *workspaceEntryAttachmentOwner) RunWithHandoff(ctx context.Context, request tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer, handoff func()) (tobari.WorkspaceSessionOutcome, error) {
	if exact, ok := o.owner.(workspaceSessionHandoffOwner); ok {
		return exact.RunWithHandoff(ctx, request, in, out, errOut, handoff)
	}
	handoff()
	return o.owner.Run(ctx, request, in, out, errOut)
}

func (o *workspaceEntryAttachmentOwner) Close(ctx context.Context) error {
	o.once.Do(func() {
		o.err = errors.Join(o.owner.Close(ctx), o.release())
	})
	return o.err
}

type WorkspaceSessionAuthority interface {
	BeginWorkspaceSession(context.Context, tobari.WorkspaceSessionBinding, string) (WorkspaceSessionOwner, error)
}

type ContextEntryPublicationBarrier interface {
	CheckContextEntryPublicationBarrier(context.Context, tobari.ContextID) error
}

// ContextEntryAdapter is dormant until the atomic WP11 composition cutover.
// It reuses Mutator's one lifecycle authority, stage, and durable effect
// decision rather than introducing an entry-specific lock or recovery command.
type ContextEntryAdapter struct {
	mutator            *Mutator
	runtime            WorkspaceEntryRuntimeAuthority
	templatePolicy     WorkspacePolicyActivationAuthority
	sessions           WorkspaceSessionAuthority
	lifetime           context.Context
	settlementTimeout  time.Duration
	afterDecision      func() error
	publicationBarrier ContextEntryPublicationBarrier
}

// Gateway/OPA replacement owns a bounded 30-second component-readiness window
// plus exact principal, policy, and receipt observations. Keep one finite
// process-lifetime-derived budget large enough for the normal full settlement;
// tests lower it to prove timeout recovery and lifecycle-lock release.
const workspaceEntrySettlementTimeout = 90 * time.Second

func NewContextEntryAdapter(mutator *Mutator, runtime WorkspaceEntryRuntimeAuthority, templatePolicy WorkspacePolicyActivationAuthority, sessions WorkspaceSessionAuthority, lifetime context.Context, barrier ContextEntryPublicationBarrier) (*ContextEntryAdapter, error) {
	if mutator == nil || mutator.store == nil || mutator.lifecycle == nil {
		return nil, fmt.Errorf("final Workspace authority mutator is required")
	}
	if runtime == nil || templatePolicy == nil || mutator.activation == nil || mutator.settlement == nil || sessions == nil || lifetime == nil || barrier == nil {
		return nil, fmt.Errorf("Context entry runtime, activation, and session authorities are required")
	}
	return &ContextEntryAdapter{mutator: mutator, runtime: runtime, templatePolicy: templatePolicy, sessions: sessions, lifetime: lifetime, settlementTimeout: workspaceEntrySettlementTimeout, publicationBarrier: barrier}, nil
}

func (a *ContextEntryAdapter) EnterContextByReference(ctx context.Context, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	return a.enterContext(ctx, contextRef, nil, "", session, nil, false, in, out, errOut)
}

func (a *ContextEntryAdapter) EnterContextByReferenceAtRoot(ctx context.Context, contextRef, projectRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return publication, err
	}
	return a.enterContext(ctx, contextRef, nil, projectRoot, session, nil, false, in, out, errOut)
}

// EnterFinalDefaultPair keeps the bare command's already-reviewed default
// Template, canonical Project, and Context receipt intact through the same
// lifecycle lock that owns entry planning and effects.
func (a *ContextEntryAdapter) EnterFinalDefaultPair(ctx context.Context, expected tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	return a.EnterFinalDefaultPairWithProgress(ctx, expected, invocationRoot, session, nil, in, out, errOut)
}

func (a *ContextEntryAdapter) EnterFinalDefaultPairWithProgress(ctx context.Context, expected tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	return a.enterFinalDefaultPair(ctx, expected, invocationRoot, session, progress, false, in, out, errOut)
}

// EnterCurrentFinalDefaultPair borrows an exactly current Workspace without
// preparing Runtime material, reconciling Docker, or publishing authority.
// Any drift after the caller's read-only status proof fails closed.
func (a *ContextEntryAdapter) EnterCurrentFinalDefaultPair(ctx context.Context, expected tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	return a.enterFinalDefaultPair(ctx, expected, invocationRoot, session, nil, true, in, out, errOut)
}

func (a *ContextEntryAdapter) enterFinalDefaultPair(ctx context.Context, expected tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, currentOnly bool, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	if err := expected.Validate(); err != nil {
		return publication, err
	}
	if expected.DefaultTemplate == nil || expected.Context == nil {
		return publication, fmt.Errorf("final default pair is incomplete")
	}
	if err := tobari.ValidateRootContains(expected.ProjectRoot, invocationRoot); err != nil {
		return publication, err
	}
	contextRef, err := tobari.ContextRef(expected.Context.Context.ID)
	if err != nil {
		return publication, err
	}
	value := expected.Clone()
	return a.enterContext(ctx, contextRef, &value, invocationRoot, session, progress, currentOnly, in, out, errOut)
}

func (a *ContextEntryAdapter) enterContext(ctx context.Context, contextRef string, expected *tobari.FinalDefaultPairObservation, invocationRoot string, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, currentOnly bool, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
	if a == nil || a.mutator == nil || a.runtime == nil || a.templatePolicy == nil || a.sessions == nil {
		return publication, fmt.Errorf("Context entry adapter is unavailable")
	}
	contextID, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return publication, err
	}
	if err := session.Validate(); err != nil {
		return publication, err
	}
	if err := a.publicationBarrier.CheckContextEntryPublicationBarrier(ctx, contextID); err != nil {
		return publication, err
	}
	// Lifecycle locking may create its host lock file. Reject pre-existing
	// unsupported authority through the non-creating Store observation first;
	// reconcileAndBegin keeps the lock-held selection fences.
	if _, _, err := a.mutator.store.ReadComplete(ctx); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return publication, errors.Join(tobari.ErrWorkspaceEntryCanceledBeforeDecision, err)
		}
		return publication, err
	}

	var owner WorkspaceSessionOwner
	resultErr = a.mutator.lifecycle.WithLifecycleLock(ctx, func(lockedContext context.Context) error {
		var err error
		publication.Snapshot, owner, err = a.reconcileAndBegin(lockedContext, contextRef, contextID, expected, invocationRoot, currentOnly)
		return err
	})
	if resultErr != nil {
		return publication, resultErr
	}
	if owner == nil {
		return publication, fmt.Errorf("Context entry did not establish an interactive attachment owner")
	}
	emitFirstEntryProgress(progress, tobari.FirstEntryPrepareWorkspace, tobari.FirstEntryStageSucceeded)
	emitFirstEntryProgress(progress, tobari.FirstEntryEnterWorkspace, tobari.FirstEntryStageRunning)
	defer func() {
		cleanupContext, cancel := a.newSettlementContext(ctx)
		closeErr := owner.Close(cleanupContext)
		cancel()
		if closeErr != nil && !containsCleanupIssue(publication.Outcome.CleanupIssues, tobari.WorkspaceCleanupInteractiveSession) {
			publication.Outcome.CleanupIssues = append(publication.Outcome.CleanupIssues, tobari.WorkspaceCleanupInteractiveSession)
		}
	}()
	handoff := func() {
		emitFirstEntryProgress(progress, tobari.FirstEntryEnterWorkspace, tobari.FirstEntryStageSucceeded)
	}
	if exact, ok := owner.(workspaceSessionHandoffOwner); ok {
		publication.Outcome, resultErr = exact.RunWithHandoff(ctx, session, in, out, errOut, handoff)
	} else {
		handoff()
		publication.Outcome, resultErr = owner.Run(ctx, session, in, out, errOut)
	}
	if resultErr != nil {
		return publication, confirmedEntryAttachmentError(resultErr)
	}
	if validationErr := publication.Outcome.Validate(); validationErr != nil {
		return publication, fmt.Errorf("Workspace session owner returned invalid outcome: %w", validationErr)
	}
	return publication, resultErr
}

func emitFirstEntryProgress(sink tobari.FirstEntryProgressSink, stage tobari.FirstEntryStage, state tobari.FirstEntryStageState) {
	if sink != nil {
		sink(tobari.FirstEntryProgress{Stage: stage, State: state})
	}
}

func (a *ContextEntryAdapter) reconcileAndBegin(ctx context.Context, contextRef string, contextID tobari.ContextID, expected *tobari.FinalDefaultPairObservation, invocationRoot string, currentOnly bool) (snapshotResult tobari.ContextAuthoritySnapshot, ownerResult WorkspaceSessionOwner, resultErr error) {
	m := a.mutator
	decisionDurable := false
	publicationConfirmed := false
	defer func() {
		if resultErr == nil || errors.Is(resultErr, tobari.ErrContextBindingNotFound) || errors.Is(resultErr, tobari.ErrWorkspaceEntryInterrupted) || errors.Is(resultErr, tobari.ErrWorkspaceEntryReconciliationConfirmed) {
			return
		}
		if publicationConfirmed {
			resultErr = confirmedEntryAttachmentError(resultErr)
			return
		}
		if decisionDurable {
			resultErr = errors.Join(tobari.ErrWorkspaceEntryInterrupted, resultErr)
			return
		}
		if errors.Is(resultErr, tobari.ErrWorkspaceRuntimePreparationUncertain) {
			return
		}
		if errors.Is(resultErr, tobari.ErrWorkspaceEntryTemplatePolicyInactive) || errors.Is(resultErr, tobari.ErrWorkspaceEntryPolicyMemoryInactive) || errors.Is(resultErr, tobari.ErrWorkspaceEntryObservationUnavailable) {
			return
		}
		if errors.Is(resultErr, context.Canceled) || errors.Is(resultErr, context.DeadlineExceeded) {
			resultErr = errors.Join(tobari.ErrWorkspaceEntryCanceledBeforeDecision, resultErr)
			return
		}
		resultErr = errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, resultErr)
	}()
	if err := ctx.Err(); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if err := validateMutationDirectory(filepath.Dir(m.store.root), 0o700); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("validate final Workspace authority parent: %w", err)
	}
	if currentOnly {
		if _, err := os.Lstat(m.effectDecisionTempPath()); err == nil {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("steady Workspace entry found pending exact recovery")
		} else if !errors.Is(err, os.ErrNotExist) {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
	} else if err := m.reconcileDecisionArtifacts(); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	decision, active, err := m.readEffectDecision()
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	decisionDurable = active
	current, present, err := m.store.ReadComplete(ctx)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if !present {
		return tobari.ContextAuthoritySnapshot{}, nil, tobari.ErrContextBindingNotFound
	}
	if expected != nil {
		observed, err := tobari.NewFinalDefaultPairContextObservation(current, true, expected.ProjectRoot, contextID)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if !observed.SameReceipt(*expected) || !reflect.DeepEqual(observed, *expected) || observed.Context == nil || observed.Context.Context.ID != contextID {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("final default pair changed before entry")
		}
	}
	selected, err := snapshotForContext(current, contextID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	projectRoot := invocationRoot
	if expected != nil {
		projectRoot = expected.ProjectRoot
	} else if projectRoot == "" && len(selected.Workspaces) == 1 {
		projectRoot = selected.Workspaces[0].ProjectRoot
	}
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("Workspace entry requires one explicit Project root: %w", err)
	}
	selected, err = selected.SelectWorkspaceAtRoot(projectRoot)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	releaseAttachment, err := a.runtime.AcquireWorkspaceEntryAttachment(ctx, contextID, projectRoot)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	defer func() {
		if resultErr != nil || ownerResult == nil {
			resultErr = errors.Join(resultErr, releaseAttachment())
			return
		}
		ownerResult = &workspaceEntryAttachmentOwner{owner: ownerResult, release: releaseAttachment}
	}()
	var releaseReconciliation func() error
	defer func() {
		if releaseReconciliation != nil {
			resultErr = errors.Join(resultErr, releaseReconciliation())
		}
	}()
	acquireReconciliation := func() error {
		if releaseReconciliation != nil {
			return nil
		}
		release, acquireErr := a.runtime.AcquireWorkspaceReconciliationFence(ctx)
		if acquireErr != nil {
			return acquireErr
		}
		releaseReconciliation = release
		return nil
	}
	terminal, terminalPresent, err := m.readTerminalEffectDecision()
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	terminalEntryRequiresSettlement := terminalPresent && terminal.Operation == "context-entry" && terminal.Target == contextRef &&
		entryPlanMatchesSelection(terminal.EntryPlan, contextID, projectRoot, selected)
	_, stageErr := os.Lstat(mutationStagePath(m.store.root))
	stagePresent := stageErr == nil
	if stageErr != nil && !errors.Is(stageErr, os.ErrNotExist) {
		return tobari.ContextAuthoritySnapshot{}, nil, stageErr
	}
	if currentOnly && (active || stagePresent) {
		return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("steady Workspace entry found pending exact recovery")
	}

	if !active && terminalEntryRequiresSettlement {
		if snapshot, consequenceErr := entryTerminalConsequence(current, terminal, contextID); consequenceErr == nil {
			decisionRef, refErr := entryDecisionRef(*terminal.EntryPlan)
			if refErr != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, refErr
			}
			settlementContext, cancel := a.newSettlementContext(ctx)
			receipt, confirmErr := a.confirmEntry(settlementContext, current, snapshot, *terminal.EntryPlan, decisionRef)
			cancel()
			if confirmErr == nil {
				publicationConfirmed = true
				if err := m.removeTerminalEntryStage(terminal); err != nil {
					return snapshot, nil, err
				}
				owner, beginErr := a.beginSession(ctx, snapshot, receipt, invocationRoot)
				if beginErr != nil {
					return snapshot, nil, confirmedEntryAttachmentError(beginErr)
				}
				return snapshot, owner, nil
			}
			if !errors.Is(confirmErr, tobari.ErrWorkspaceEntryRuntimeNotCurrent) {
				return tobari.ContextAuthoritySnapshot{}, nil, errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, confirmErr)
			}
			// Only an exact missing/mismatch disposition ends the terminal
			// fast path. Generic Docker/activation errors remain read-only
			// observation uncertainty and never authorize reconciliation.
		}
	}

	if active {
		if decision.Operation != "context-entry" || decision.Target != contextRef || decision.EntryPlan == nil {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("another final-authority mutation requires exact same-target recovery")
		}
		if !entryPlanMatchesSelection(decision.EntryPlan, contextID, projectRoot, selected) {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("pending Context entry belongs to another Workspace root")
		}
		if decision.NextGeneration != decision.PreviousGeneration && current.Generation == decision.NextGeneration && current.Revision == decision.NextRevision {
			publicationConfirmed = true
			snapshot, err := snapshotForContext(current, contextID)
			if err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			snapshot, err = snapshot.SelectWorkspace(decision.EntryPlan.Workspace.ID)
			if err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			decisionRef, err := entryDecisionRef(*decision.EntryPlan)
			if err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			settlementContext, cancel := a.newSettlementContext(ctx)
			receipt, confirmErr := a.confirmEntry(settlementContext, current, snapshot, *decision.EntryPlan, decisionRef)
			cancel()
			if confirmErr != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, confirmErr
			}
			if err := m.clearEffectDecision(); err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			owner, err := a.beginSession(ctx, snapshot, receipt, invocationRoot)
			if err != nil {
				return snapshot, nil, confirmedEntryAttachmentError(err)
			}
			return snapshot, owner, nil
		}
		if current.Generation != decision.PreviousGeneration || current.Revision != decision.PreviousRevision {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("active Context entry crosses unexpected envelope authority")
		}
	}

	desired, err := snapshotForContext(current, contextID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	desired, err = desired.SelectWorkspaceAtRoot(projectRoot)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	entryAuthority, err := tobari.DeriveWorkspaceTemplateEntryAuthority(desired.Template.Current)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if currentOnly {
		return a.confirmAndBeginCurrent(ctx, current, desired, entryAuthority, invocationRoot)
	}
	var plan tobari.WorkspaceEntryReconciliationPlan
	var next tobari.WorkspaceAuthorityCollection
	var changed bool
	if active {
		plan = decision.EntryPlan.Clone()
		if decision.legacyEntryPlan && desired.ContextHome == "" {
			plan.InitializeContextHome = true
		}
		if err := plan.ValidateFor(desired); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := a.runtime.PrepareWorkspaceRuntimeMaterial(ctx, plan.Authority.Runtime); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		next, changed, err = tobari.PublishWorkspaceEntryAuthority(current, plan)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if next.Generation != decision.NextGeneration || next.Revision != decision.NextRevision {
			if !decision.legacyEntryPlan || next.Generation != decision.NextGeneration {
				return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("same-target Context entry no longer matches its durable decision")
			}
		}
		encoded, err := EncodeComplete(next)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		legacyStage := false
		if decision.legacyEntryPlan {
			if err := m.validatePreparedStage(encoded); err != nil {
				if legacyErr := m.validateLegacyContextHomeEntryStage(next); legacyErr != nil {
					return tobari.ContextAuthoritySnapshot{}, nil, errors.Join(err, legacyErr)
				}
				legacyStage = true
			}
			decision.EntryPlan = &plan
			decision.NextGeneration = next.Generation
			decision.NextRevision = next.Revision
			decision.EntryPlanCompatibility = entryPlanContextHomeCompatibility
			decision.legacyEntryPlan = false
			if err := m.replaceEffectDecision(decision); err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			if legacyStage {
				if err := m.prepareEffectStage(encoded); err != nil {
					return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("replace predecessor Context entry stage: %w", err)
				}
			}
		} else if err := m.validatePreparedStage(encoded); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := acquireReconciliation(); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
	} else {
		workspaceID := tobari.WorkspaceID("")
		if desired.Workspace != nil {
			workspaceID = desired.Workspace.ID
		} else {
			workspaceID, err = tobari.IssueWorkspaceID(m.clock().UTC(), m.entropy)
			if err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
		}
		if err := a.runtime.PrepareWorkspaceRuntimeMaterial(ctx, entryAuthority.Runtime); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		reconciledAt := m.clock().UTC()
		expectedHome, err := a.runtime.ContextHomeForID(ctx, desired.Context.ID)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		plan, err = a.runtime.PlanWorkspaceEntry(ctx, desired.Clone(), entryAuthority, projectRoot, workspaceID, reconciledAt)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		exactNoOp := desired.Workspace != nil && reflect.DeepEqual(plan.Workspace, *desired.Workspace)
		if err := plan.ValidateFor(desired); err != nil || plan.Workspace.ID != workspaceID || plan.Workspace.Home != expectedHome || (plan.Applied.ReconciledAt != reconciledAt && !exactNoOp) {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("Workspace entry runtime plan changed task-owned authority: %w", err)
		}
		next, changed, err = tobari.PublishWorkspaceEntryAuthority(current, plan)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if !changed && !terminalEntryRequiresSettlement && a.afterDecision == nil {
			// A current Workspace needs no durable mutation decision. Confirm the
			// exact existing runtime receipt and both active policy axes, then
			// borrow the live session directly. A terminal receipt from an earlier
			// completed operation is historical evidence, not authority to force a
			// new reconciliation. Only an exact runtime mismatch is allowed to fall
			// through; ambiguous observations and inactive protection remain
			// fail-closed.
			decisionRef, refErr := entryDecisionRef(plan)
			if refErr != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, refErr
			}
			settlementContext, cancel := a.newSettlementContext(ctx)
			receipt, confirmErr := a.confirmEntry(settlementContext, current, desired, plan, decisionRef)
			cancel()
			if confirmErr == nil {
				publicationConfirmed = true
				owner, beginErr := a.beginSession(ctx, desired, receipt, invocationRoot)
				if beginErr != nil {
					return desired, nil, confirmedEntryAttachmentError(beginErr)
				}
				return desired, owner, nil
			}
			if !errors.Is(confirmErr, tobari.ErrWorkspaceEntryRuntimeNotCurrent) {
				return tobari.ContextAuthoritySnapshot{}, nil, confirmErr
			}
		}
		if err := acquireReconciliation(); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		encoded, err := EncodeComplete(next)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		complete := effectDecision{
			SchemaVersion: effectDecisionSchemaVersion, Operation: "context-entry", Target: contextRef,
			PreviousGeneration: current.Generation, PreviousRevision: current.Revision,
			NextGeneration: next.Generation, NextRevision: next.Revision, EntryPlan: pointerEntryPlan(plan),
		}
		if err := complete.validate(); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := m.reconcileStage(); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := m.prepareEffectStage(encoded); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := m.writeEffectDecision(complete); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		decisionDurable = true
		decision = complete
	}
	if a.afterDecision != nil {
		if err := a.afterDecision(); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
	}

	decisionRef, err := entryDecisionRef(plan)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	receipt, err := a.runtime.ReconcileWorkspaceEntry(ctx, plan.Clone(), decisionRef)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if err := releaseReconciliation(); err != nil {
		releaseReconciliation = nil
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	releaseReconciliation = nil
	if err := receipt.ValidateFor(plan); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("Workspace reconciliation returned another authority: %w", err)
	}
	completionContext, cancelSettlement := a.newSettlementContext(ctx)
	if err := m.settlement.SettleFinalAuthority(completionContext, current.Clone(), next.Clone(), contextID, "context-entry", decisionRef); err != nil {
		cancelSettlement()
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	nextSnapshot, snapshotErr := snapshotForContext(next, contextID)
	if snapshotErr != nil {
		cancelSettlement()
		return tobari.ContextAuthoritySnapshot{}, nil, snapshotErr
	}
	nextSnapshot, snapshotErr = nextSnapshot.SelectWorkspace(plan.Workspace.ID)
	if snapshotErr != nil {
		cancelSettlement()
		return tobari.ContextAuthoritySnapshot{}, nil, snapshotErr
	}
	confirmedReceipt, confirmErr := a.confirmEntry(completionContext, next, nextSnapshot, plan, decisionRef)
	cancelSettlement()
	if confirmErr != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, confirmErr
	}
	encoded, err := EncodeComplete(next)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if changed {
		if err := m.publishPreparedEffect(current, next, encoded); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
	}
	committed, present, err := m.readPublishedComplete()
	if err != nil || !present || committed.Generation != next.Generation || committed.Revision != next.Revision {
		return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("read confirmed Context entry publication: %w", err)
	}
	confirmed, err := snapshotForContext(committed, contextID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	confirmed, err = confirmed.SelectWorkspace(plan.Workspace.ID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	publicationConfirmed = true
	if err := m.clearEffectDecision(); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if !changed {
		if err := m.removeExactPreparedEntryStage(encoded); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return confirmed, nil, confirmedEntryAttachmentError(err)
	}
	owner, err := a.beginSession(ctx, confirmed, confirmedReceipt, invocationRoot)
	if err != nil {
		return confirmed, nil, confirmedEntryAttachmentError(err)
	}
	return confirmed, owner, nil
}

// validateLegacyContextHomeEntryStage admits only the exact predecessor
// serialization that omitted Context-owned Home mirrors while retaining the
// same Workspace bindings and collection revision. Arbitrary or partially
// changed stage bytes remain corruption and are never overwritten.
func (m *Mutator) validateLegacyContextHomeEntryStage(expected tobari.WorkspaceAuthorityCollection) error {
	data, err := readAuthorityFile(mutationStagePath(m.store.root))
	if err != nil {
		return fmt.Errorf("read predecessor Context entry stage: %w", err)
	}
	var legacy tobari.WorkspaceAuthorityCollection
	if err := decodeStrictJSON(data, &legacy); err != nil {
		return fmt.Errorf("decode predecessor Context entry stage: %w", err)
	}
	workspaceByContext := make(map[tobari.ContextID]tobari.WorkspaceBinding, len(legacy.Workspaces))
	for _, workspace := range legacy.Workspaces {
		if prior, found := workspaceByContext[workspace.ContextID]; found && (prior.Home != workspace.Home || prior.CreationDefaults != workspace.CreationDefaults) {
			return fmt.Errorf("predecessor Context entry stage has divergent Context Home mirrors")
		}
		workspaceByContext[workspace.ContextID] = workspace
	}
	normalized := legacy.Clone()
	changed := false
	for index := range normalized.Contexts {
		record := &normalized.Contexts[index]
		if (record.ContextHome == "") != (record.CreationDefaults == "") {
			return fmt.Errorf("predecessor Context entry stage has partial Context Home authority")
		}
		if record.ContextHome != "" {
			continue
		}
		workspace, found := workspaceByContext[record.Context.ID]
		if !found {
			continue
		}
		record.ContextHome = workspace.Home
		record.CreationDefaults = workspace.CreationDefaults
		changed = true
	}
	if !changed || normalized.Validate() != nil || !reflect.DeepEqual(normalized, expected) {
		return fmt.Errorf("predecessor Context entry stage does not match the active decision")
	}
	return nil
}

func containsCleanupIssue(values []tobari.WorkspaceAttachmentCleanupIssue, target tobari.WorkspaceAttachmentCleanupIssue) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (m *Mutator) removeExactPreparedEntryStage(encoded []byte) error {
	if err := m.validatePreparedStage(encoded); err != nil {
		return err
	}
	stage := mutationStagePath(m.store.root)
	if err := os.Remove(stage); err != nil {
		return fmt.Errorf("remove terminal no-op Context entry stage: %w", err)
	}
	return m.sync(filepath.Dir(stage))
}

func (m *Mutator) removeTerminalEntryStage(decision effectDecision) error {
	stage := mutationStagePath(m.store.root)
	if _, err := os.Lstat(stage); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	data, err := readAuthorityFile(stage)
	if err != nil {
		return fmt.Errorf("read terminal Context entry stage: %w", err)
	}
	var collection tobari.WorkspaceAuthorityCollection
	if err := decodeStrictJSON(data, &collection); err != nil {
		return fmt.Errorf("decode terminal Context entry stage: %w", err)
	}
	if err := validateCollectionBounds(collection); err != nil {
		return err
	}
	if err := collection.Validate(); err != nil {
		return fmt.Errorf("validate terminal Context entry stage: %w", err)
	}
	if collection.Generation != decision.NextGeneration || collection.Revision != decision.NextRevision {
		return fmt.Errorf("terminal Context entry stage does not belong to its exact decision")
	}
	if err := os.Remove(stage); err != nil {
		return fmt.Errorf("remove terminal Context entry stage: %w", err)
	}
	return m.sync(filepath.Dir(stage))
}

func confirmedEntryAttachmentError(cause error) error {
	return errors.Join(tobari.ErrWorkspaceEntryReconciliationConfirmed, cause)
}

func (a *ContextEntryAdapter) newSettlementContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := a.settlementTimeout
	if timeout <= 0 {
		timeout = workspaceEntrySettlementTimeout
	}
	base := a.lifetime
	if base == nil {
		base = parent
	}
	return context.WithTimeout(base, timeout)
}

func (a *ContextEntryAdapter) confirmCurrentActivations(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, snapshot tobari.ContextAuthoritySnapshot) error {
	if snapshot.ActiveTemplatePolicy == nil || snapshot.ActiveTemplatePolicy.ValidateFor(snapshot.Context, snapshot.Template.Current) != nil {
		return tobari.ErrWorkspaceEntryTemplatePolicyInactive
	}
	if snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || snapshot.ActivePolicyMemory.Revision != snapshot.PolicyMemory.Revision || snapshot.ActivePolicyMemoryRef.ValidateFor(snapshot.Context, snapshot.PolicyMemory) != nil {
		return tobari.ErrWorkspaceEntryPolicyMemoryInactive
	}
	current, err := a.templatePolicy.ObserveWorkspacePolicyAxesCurrent(ctx, collection.Clone(), snapshot.Context.ID, *snapshot.ActiveTemplatePolicy, *snapshot.ActivePolicyMemoryRef)
	if err != nil {
		return errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, err)
	}
	if !current {
		return tobari.ErrWorkspaceEntryProtectionNotCurrent
	}
	if err := a.templatePolicy.ConfirmWorkspacePolicyAxesActive(ctx, collection.Clone(), snapshot.Context.ID, *snapshot.ActiveTemplatePolicy, *snapshot.ActivePolicyMemoryRef); err != nil {
		return errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, err)
	}
	return nil
}

func (a *ContextEntryAdapter) confirmEntry(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, snapshot tobari.ContextAuthoritySnapshot, plan tobari.WorkspaceEntryReconciliationPlan, decisionRef string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	if err := plan.ValidateFor(snapshot); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	if err := a.confirmCurrentActivations(ctx, collection, snapshot); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	receipt, err := a.runtime.ConfirmWorkspaceEntry(ctx, plan.Clone(), decisionRef)
	if err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	if err := receipt.ValidateFor(plan); err != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, err
	}
	return receipt, nil
}

func (a *ContextEntryAdapter) confirmAndBeginCurrent(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, snapshot tobari.ContextAuthoritySnapshot, authority tobari.WorkspaceTemplateEntryAuthority, invocationRoot string) (tobari.ContextAuthoritySnapshot, WorkspaceSessionOwner, error) {
	if snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil {
		return tobari.ContextAuthoritySnapshot{}, nil, tobari.ErrWorkspaceEntryRuntimeNotCurrent
	}
	plan, err := a.runtime.PlanWorkspaceEntry(ctx, snapshot.Clone(), authority, snapshot.Workspace.ProjectRoot, snapshot.Workspace.ID, a.mutator.clock().UTC())
	if err != nil {
		if errors.Is(err, tobari.ErrRuntimeNotReady) || errors.Is(err, tobari.ErrRuntimeNotFound) || errors.Is(err, tobari.ErrRuntimeRevisionNotFound) {
			return tobari.ContextAuthoritySnapshot{}, nil, errors.Join(tobari.ErrWorkspaceEntryRuntimeNotCurrent, err)
		}
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	next, changed, err := tobari.PublishWorkspaceEntryAuthority(collection, plan)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	if changed || next.Generation != collection.Generation || next.Revision != collection.Revision {
		return tobari.ContextAuthoritySnapshot{}, nil, tobari.ErrWorkspaceEntryRuntimeNotCurrent
	}
	decisionRef, err := entryDecisionRef(plan)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	settlementContext, cancel := a.newSettlementContext(ctx)
	receipt, err := a.confirmEntry(settlementContext, collection, snapshot, plan, decisionRef)
	cancel()
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
	owner, err := a.beginSession(ctx, snapshot, receipt, invocationRoot)
	if err != nil {
		return snapshot, nil, confirmedEntryAttachmentError(err)
	}
	return snapshot, owner, nil
}

func (a *ContextEntryAdapter) beginSession(ctx context.Context, snapshot tobari.ContextAuthoritySnapshot, receipt tobari.WorkspaceEntryReconciliationReceipt, invocationRoot string) (WorkspaceSessionOwner, error) {
	binding, err := tobari.NewWorkspaceSessionBinding(snapshot, receipt)
	if err != nil {
		return nil, fmt.Errorf("derive final Workspace session authority: %w", err)
	}
	if invocationRoot == "" {
		invocationRoot = binding.ProjectRoot
	}
	if err := tobari.ValidateRootContains(binding.ProjectRoot, invocationRoot); err != nil {
		return nil, fmt.Errorf("validate final Workspace invocation root: %w", err)
	}
	return a.sessions.BeginWorkspaceSession(ctx, binding, invocationRoot)
}

func entryCollection(current tobari.WorkspaceAuthorityCollection, plan tobari.WorkspaceEntryReconciliationPlan) (tobari.WorkspaceAuthorityCollection, bool, error) {
	return tobari.PublishWorkspaceEntryAuthority(current, plan)
}

func entryTerminalConsequence(current tobari.WorkspaceAuthorityCollection, decision effectDecision, contextID tobari.ContextID) (tobari.ContextAuthoritySnapshot, error) {
	if decision.EntryPlan == nil {
		return tobari.ContextAuthoritySnapshot{}, fmt.Errorf("terminal Context entry evidence is incomplete")
	}
	snapshot, err := snapshotForContext(current, contextID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	snapshot, err = snapshot.SelectWorkspace(decision.EntryPlan.Workspace.ID)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	if snapshot.Workspace == nil || !reflect.DeepEqual(*snapshot.Workspace, decision.EntryPlan.Workspace) {
		return tobari.ContextAuthoritySnapshot{}, fmt.Errorf("terminal Context entry consequence is no longer current")
	}
	if err := decision.EntryPlan.ValidateFor(snapshot); err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	return snapshot, nil
}

func entryPlanMatchesSelection(plan *tobari.WorkspaceEntryReconciliationPlan, contextID tobari.ContextID, projectRoot string, selected tobari.ContextAuthoritySnapshot) bool {
	if plan == nil || plan.Workspace.ContextID != contextID || plan.Workspace.ProjectRoot != projectRoot {
		return false
	}
	if selected.Workspace != nil {
		return selected.Workspace.ID == plan.Workspace.ID && selected.Workspace.ProjectRoot == projectRoot
	}
	for _, workspace := range selected.Workspaces {
		if workspace.ID == plan.Workspace.ID || workspace.ProjectRoot == projectRoot {
			return false
		}
	}
	return true
}

func entryDecisionRef(plan tobari.WorkspaceEntryReconciliationPlan) (string, error) {
	_, digest, err := encodeAuthorityObject(plan)
	if err != nil {
		return "", fmt.Errorf("derive Workspace entry decision reference: %w", err)
	}
	return "workspace-entry:" + string(plan.Workspace.ID) + ":" + digest, nil
}

func pointerEntryPlan(plan tobari.WorkspaceEntryReconciliationPlan) *tobari.WorkspaceEntryReconciliationPlan {
	value := plan.Clone()
	return &value
}
