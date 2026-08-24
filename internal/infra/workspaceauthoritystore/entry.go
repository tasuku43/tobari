package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// WorkspaceEntryRuntimeAuthority owns bounded runtime planning, mutation, and
// exact Docker observation. Planning is read-only. Reconcile must be
// receipt-idempotent for one unchanged decision reference because its effect
// can precede an interrupted final-envelope publication.
type WorkspaceEntryRuntimeAuthority interface {
	WorkspaceHomeForID(context.Context, tobari.WorkspaceID) (string, error)
	PlanWorkspaceEntry(context.Context, tobari.ContextAuthoritySnapshot, tobari.WorkspaceTemplateEntryAuthority, tobari.WorkspaceID, time.Time) (tobari.WorkspaceEntryReconciliationPlan, error)
	ReconcileWorkspaceEntry(context.Context, tobari.WorkspaceEntryReconciliationPlan, string) (tobari.WorkspaceEntryReconciliationReceipt, error)
	ConfirmWorkspaceEntry(context.Context, tobari.WorkspaceEntryReconciliationPlan, string) (tobari.WorkspaceEntryReconciliationReceipt, error)
}

// TemplatePolicyActivationAuthority observes the independently activated
// static policy axis. Context entry does not repair or replace cluster policy;
// a stale receipt requires the existing explicit cluster reconciliation.
type TemplatePolicyActivationAuthority interface {
	ConfirmTemplatePolicyActive(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.TemplatePolicyActivationReceipt) error
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

type WorkspaceSessionAuthority interface {
	BeginWorkspaceSession(context.Context, tobari.WorkspaceSessionBinding) (WorkspaceSessionOwner, error)
}

// ContextEntryAdapter is dormant until the atomic WP11 composition cutover.
// It reuses Mutator's one lifecycle authority, stage, and durable effect
// decision rather than introducing an entry-specific lock or recovery command.
type ContextEntryAdapter struct {
	mutator           *Mutator
	runtime           WorkspaceEntryRuntimeAuthority
	templatePolicy    TemplatePolicyActivationAuthority
	sessions          WorkspaceSessionAuthority
	lifetime          context.Context
	settlementTimeout time.Duration
	afterDecision     func() error
}

// Gateway/OPA replacement owns a bounded 30-second component-readiness window
// plus exact principal, policy, and receipt observations. Keep one finite
// process-lifetime-derived budget large enough for the normal full settlement;
// tests lower it to prove timeout recovery and lifecycle-lock release.
const workspaceEntrySettlementTimeout = 90 * time.Second

func NewContextEntryAdapter(mutator *Mutator, runtime WorkspaceEntryRuntimeAuthority, templatePolicy TemplatePolicyActivationAuthority, sessions WorkspaceSessionAuthority, lifetime context.Context) (*ContextEntryAdapter, error) {
	if mutator == nil || mutator.store == nil || mutator.lifecycle == nil {
		return nil, fmt.Errorf("final Workspace authority mutator is required")
	}
	if runtime == nil || templatePolicy == nil || mutator.activation == nil || mutator.settlement == nil || sessions == nil || lifetime == nil {
		return nil, fmt.Errorf("Context entry runtime, activation, and session authorities are required")
	}
	return &ContextEntryAdapter{mutator: mutator, runtime: runtime, templatePolicy: templatePolicy, sessions: sessions, lifetime: lifetime, settlementTimeout: workspaceEntrySettlementTimeout}, nil
}

func (a *ContextEntryAdapter) EnterContextByReference(ctx context.Context, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (publication tobari.ContextEntryPublication, resultErr error) {
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

	var owner WorkspaceSessionOwner
	resultErr = a.mutator.lifecycle.WithLifecycleLock(ctx, func(lockedContext context.Context) error {
		var err error
		publication.Snapshot, owner, err = a.reconcileAndBegin(lockedContext, contextRef, contextID)
		return err
	})
	if resultErr != nil {
		return publication, resultErr
	}
	if owner == nil {
		return publication, fmt.Errorf("Context entry did not establish an interactive attachment owner")
	}
	defer func() {
		cleanupContext, cancel := a.newSettlementContext(ctx)
		closeErr := owner.Close(cleanupContext)
		cancel()
		if closeErr != nil && !containsCleanupIssue(publication.Outcome.CleanupIssues, tobari.WorkspaceCleanupInteractiveSession) {
			publication.Outcome.CleanupIssues = append(publication.Outcome.CleanupIssues, tobari.WorkspaceCleanupInteractiveSession)
		}
	}()
	publication.Outcome, resultErr = owner.Run(ctx, session, in, out, errOut)
	if resultErr != nil {
		return publication, confirmedEntryAttachmentError(resultErr)
	}
	if validationErr := publication.Outcome.Validate(); validationErr != nil {
		return publication, fmt.Errorf("Workspace session owner returned invalid outcome: %w", validationErr)
	}
	return publication, resultErr
}

func (a *ContextEntryAdapter) reconcileAndBegin(ctx context.Context, contextRef string, contextID tobari.ContextID) (snapshotResult tobari.ContextAuthoritySnapshot, ownerResult WorkspaceSessionOwner, resultErr error) {
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
	if err := m.reconcileDecisionArtifacts(); err != nil {
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
	terminal, terminalPresent, err := m.readTerminalEffectDecision()
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}

	if !active && terminalPresent && terminal.Operation == "context-entry" && terminal.Target == contextRef {
		if snapshot, consequenceErr := entryTerminalConsequence(current, terminal, contextID); consequenceErr == nil {
			settlementContext, cancel := a.newSettlementContext(ctx)
			receipt, confirmErr := a.confirmEntry(settlementContext, current, snapshot, *terminal.EntryPlan, entryDecisionRef(*terminal.EntryPlan, terminal.NextRevision))
			cancel()
			if confirmErr == nil {
				publicationConfirmed = true
				if err := m.removeTerminalEntryStage(terminal); err != nil {
					return snapshot, nil, err
				}
				owner, beginErr := a.beginSession(ctx, snapshot, receipt)
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
		if decision.NextGeneration != decision.PreviousGeneration && current.Generation == decision.NextGeneration && current.Revision == decision.NextRevision {
			publicationConfirmed = true
			snapshot, err := snapshotForContext(current, contextID)
			if err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			settlementContext, cancel := a.newSettlementContext(ctx)
			receipt, confirmErr := a.confirmEntry(settlementContext, current, snapshot, *decision.EntryPlan, entryDecisionRef(*decision.EntryPlan, decision.NextRevision))
			cancel()
			if confirmErr != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, confirmErr
			}
			if err := m.clearEffectDecision(); err != nil {
				return tobari.ContextAuthoritySnapshot{}, nil, err
			}
			owner, err := a.beginSession(ctx, snapshot, receipt)
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
	if err := a.confirmCurrentActivations(ctx, current, desired); err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}

	var plan tobari.WorkspaceEntryReconciliationPlan
	var next tobari.WorkspaceAuthorityCollection
	var changed bool
	if active {
		plan = decision.EntryPlan.Clone()
		if err := plan.ValidateFor(desired); err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		next, changed, err = entryCollection(current, plan)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if next.Generation != decision.NextGeneration || next.Revision != decision.NextRevision {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("same-target Context entry no longer matches its durable decision")
		}
		encoded, err := EncodeComplete(next)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		if err := m.validatePreparedStage(encoded); err != nil {
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
		entryAuthority, err := tobari.DeriveWorkspaceTemplateEntryAuthority(desired.Template.Current)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		reconciledAt := m.clock().UTC()
		expectedHome, err := a.runtime.WorkspaceHomeForID(ctx, workspaceID)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		plan, err = a.runtime.PlanWorkspaceEntry(ctx, desired.Clone(), entryAuthority, workspaceID, reconciledAt)
		if err != nil {
			return tobari.ContextAuthoritySnapshot{}, nil, err
		}
		exactNoOp := desired.Workspace != nil && reflect.DeepEqual(plan.Workspace, *desired.Workspace)
		if err := plan.ValidateFor(desired); err != nil || plan.Workspace.ID != workspaceID || plan.Workspace.Home != expectedHome || (plan.Applied.ReconciledAt != reconciledAt && !exactNoOp) {
			return tobari.ContextAuthoritySnapshot{}, nil, fmt.Errorf("Workspace entry runtime plan changed task-owned authority: %w", err)
		}
		next, changed, err = entryCollection(current, plan)
		if err != nil {
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

	decisionRef := entryDecisionRef(plan, decision.NextRevision)
	receipt, err := a.runtime.ReconcileWorkspaceEntry(ctx, plan.Clone(), decisionRef)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, nil, err
	}
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
	owner, err := a.beginSession(ctx, confirmed, confirmedReceipt)
	if err != nil {
		return confirmed, nil, confirmedEntryAttachmentError(err)
	}
	return confirmed, owner, nil
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
	stage := m.store.root + ".wp11-mutation-stage"
	if err := os.Remove(stage); err != nil {
		return fmt.Errorf("remove terminal no-op Context entry stage: %w", err)
	}
	return m.sync(filepath.Dir(stage))
}

func (m *Mutator) removeTerminalEntryStage(decision effectDecision) error {
	stage := m.store.root + ".wp11-mutation-stage"
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
	if err := a.templatePolicy.ConfirmTemplatePolicyActive(ctx, collection.Clone(), snapshot.Context.ID, *snapshot.ActiveTemplatePolicy); err != nil {
		return errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, err)
	}
	if err := a.mutator.activation.ConfirmPolicyMemoryActive(ctx, collection.Clone(), snapshot.Context.ID, *snapshot.ActivePolicyMemoryRef); err != nil {
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

func (a *ContextEntryAdapter) beginSession(ctx context.Context, snapshot tobari.ContextAuthoritySnapshot, receipt tobari.WorkspaceEntryReconciliationReceipt) (WorkspaceSessionOwner, error) {
	binding, err := tobari.NewWorkspaceSessionBinding(snapshot, receipt)
	if err != nil {
		return nil, fmt.Errorf("derive final Workspace session authority: %w", err)
	}
	return a.sessions.BeginWorkspaceSession(ctx, binding)
}

func entryCollection(current tobari.WorkspaceAuthorityCollection, plan tobari.WorkspaceEntryReconciliationPlan) (tobari.WorkspaceAuthorityCollection, bool, error) {
	workspaces := cloneWorkspaceBindings(current.Workspaces)
	replaced := false
	for index := range workspaces {
		if workspaces[index].ContextID == plan.Workspace.ContextID {
			workspaces[index] = plan.Workspace
			replaced = true
			break
		}
	}
	if !replaced {
		workspaces = append(workspaces, plan.Workspace)
	}
	return publishCollection(current, true, cloneTemplates(current.Templates), cloneContextRecords(current.Contexts), workspaces, clonePolicyCandidates(current.PendingCandidates), current.DefaultTemplateID)
}

func entryTerminalConsequence(current tobari.WorkspaceAuthorityCollection, decision effectDecision, contextID tobari.ContextID) (tobari.ContextAuthoritySnapshot, error) {
	if decision.EntryPlan == nil {
		return tobari.ContextAuthoritySnapshot{}, fmt.Errorf("terminal Context entry evidence is incomplete")
	}
	snapshot, err := snapshotForContext(current, contextID)
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

func entryDecisionRef(plan tobari.WorkspaceEntryReconciliationPlan, revision tobari.SemanticDigest) string {
	return "workspace-entry:" + string(plan.Workspace.ID) + ":" + string(revision)
}

func pointerEntryPlan(plan tobari.WorkspaceEntryReconciliationPlan) *tobari.WorkspaceEntryReconciliationPlan {
	value := plan.Clone()
	return &value
}
