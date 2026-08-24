package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type entryRuntimeFixture struct {
	planCalls      int
	reconcileCalls int
	confirmCalls   int
	reconcileErr   error
	confirmErr     error
	confirmErrors  []error
	blockConfirm   bool
	reuseApplied   bool
	onConfirm      func()
	onReconcile    func()
	resolvedSpec   tobari.SemanticDigest
	containerID    string
	homes          map[tobari.WorkspaceID]string
}

func (r *entryRuntimeFixture) WorkspaceHomeForID(_ context.Context, id tobari.WorkspaceID) (string, error) {
	if home, exists := r.homes[id]; exists {
		return home, nil
	}
	return "/workspace/home-" + string(id), nil
}

func (r *entryRuntimeFixture) PlanWorkspaceEntry(_ context.Context, snapshot tobari.ContextAuthoritySnapshot, authority tobari.WorkspaceTemplateEntryAuthority, workspaceID tobari.WorkspaceID, reconciledAt time.Time) (tobari.WorkspaceEntryReconciliationPlan, error) {
	r.planCalls++
	if err := authority.ValidateFor(snapshot.Template.Current); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: snapshot.Context.ID,
		ProjectRoot: snapshot.Context.ProjectRoot, Home: "/workspace/home-" + string(workspaceID), CreationDefaults: snapshot.Template.Current.Slices.CreationDefaultsDigest,
	}
	if snapshot.Workspace != nil {
		workspace = *snapshot.Workspace
		if snapshot.Workspace.LastSuccessfulEntry != nil {
			previous := *snapshot.Workspace.LastSuccessfulEntry
			workspace.LastSuccessfulEntry = &previous
		}
	}
	if r.reuseApplied && snapshot.Workspace != nil && snapshot.Workspace.LastSuccessfulEntry != nil {
		return tobari.WorkspaceEntryReconciliationPlan{Workspace: workspace, Applied: *workspace.LastSuccessfulEntry}, nil
	}
	resolved := r.resolvedSpec
	if resolved == "" {
		resolved = tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64))
	}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision,
		EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest, RuntimeID: authority.Runtime.RuntimeID,
		RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision, ResolvedSpec: resolved, ReconciledAt: reconciledAt,
	}
	workspace.LastSuccessfulEntry = &applied
	return tobari.WorkspaceEntryReconciliationPlan{Workspace: workspace, Applied: applied}, nil
}

func (r *entryRuntimeFixture) ReconcileWorkspaceEntry(_ context.Context, plan tobari.WorkspaceEntryReconciliationPlan, _ string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	r.reconcileCalls++
	if r.onReconcile != nil {
		r.onReconcile()
	}
	if r.reconcileErr != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, r.reconcileErr
	}
	return r.receipt(plan), nil
}

func (r *entryRuntimeFixture) ConfirmWorkspaceEntry(ctx context.Context, plan tobari.WorkspaceEntryReconciliationPlan, _ string) (tobari.WorkspaceEntryReconciliationReceipt, error) {
	r.confirmCalls++
	if r.onConfirm != nil {
		r.onConfirm()
	}
	if r.blockConfirm {
		<-ctx.Done()
		return tobari.WorkspaceEntryReconciliationReceipt{}, ctx.Err()
	}
	if len(r.confirmErrors) > 0 {
		err := r.confirmErrors[0]
		r.confirmErrors = r.confirmErrors[1:]
		if err != nil {
			return tobari.WorkspaceEntryReconciliationReceipt{}, err
		}
	}
	if r.confirmErr != nil {
		return tobari.WorkspaceEntryReconciliationReceipt{}, r.confirmErr
	}
	return r.receipt(plan), nil
}

func (r *entryRuntimeFixture) receipt(plan tobari.WorkspaceEntryReconciliationPlan) tobari.WorkspaceEntryReconciliationReceipt {
	containerID := r.containerID
	if containerID == "" {
		containerID = strings.Repeat("a", 64)
	}
	return tobari.WorkspaceEntryReconciliationReceipt{WorkspaceID: plan.Workspace.ID, ContextID: plan.Workspace.ContextID, Applied: plan.Applied, ContainerID: containerID}
}

type templateActivationFixture struct {
	calls int
	err   error
}

func (a *templateActivationFixture) ConfirmTemplatePolicyActive(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, receipt tobari.TemplatePolicyActivationReceipt) error {
	a.calls++
	if a.err != nil {
		return a.err
	}
	snapshot, err := snapshotForContext(collection, contextID)
	if err != nil {
		return err
	}
	return receipt.ValidateFor(snapshot.Context, snapshot.Template.Current)
}

type entrySessionFixture struct {
	lifecycle *mutationLifecycle
	begin     int
	run       int
	close     int
	beginErr  error
	runErr    error
	closeErr  error
	outcome   tobari.WorkspaceSessionOutcome
}

func (s *entrySessionFixture) BeginWorkspaceSession(_ context.Context, binding tobari.WorkspaceSessionBinding) (WorkspaceSessionOwner, error) {
	s.begin++
	if s.lifecycle == nil || !s.lifecycle.held.Load() {
		return nil, fmt.Errorf("session owner was not acquired under lifecycle authority")
	}
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("session owner received invalid final authority: %w", err)
	}
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return s, nil
}

func (s *entrySessionFixture) Run(_ context.Context, _ tobari.WorkspaceSessionRequest, _ io.Reader, _, _ io.Writer) (tobari.WorkspaceSessionOutcome, error) {
	s.run++
	if s.lifecycle.held.Load() {
		return tobari.WorkspaceSessionOutcome{}, fmt.Errorf("interactive session retained installation lifecycle lock")
	}
	return s.outcome, s.runErr
}

func (s *entrySessionFixture) Close(context.Context) error {
	s.close++
	if s.lifecycle.held.Load() {
		return fmt.Errorf("interactive session cleanup retained installation lifecycle lock")
	}
	return s.closeErr
}

func newEntryFixture(t *testing.T, collection tobari.WorkspaceAuthorityCollection) (*Store, *Mutator, *ContextEntryAdapter, *mutationLifecycle, *entryRuntimeFixture, *templateActivationFixture, *policyActivationFixture, *entrySessionFixture) {
	t.Helper()
	store, mutator, lifecycle, _, memory := newMutationFixture(t, &collection)
	runtime := &entryRuntimeFixture{homes: map[tobari.WorkspaceID]string{}}
	for _, workspace := range collection.Workspaces {
		runtime.homes[workspace.ID] = workspace.Home
	}
	templatePolicy := &templateActivationFixture{}
	sessions := &entrySessionFixture{lifecycle: lifecycle, outcome: tobari.WorkspaceSessionOutcome{ExitCode: 7}}
	adapter, err := NewContextEntryAdapter(mutator, runtime, templatePolicy, sessions, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return store, mutator, adapter, lifecycle, runtime, templatePolicy, memory, sessions
}

func TestContextEntryConfirmsIndependentAxesBeforePublishingAppliedEntryAndRunsOutsideLock(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, _, adapter, lifecycle, runtime, templatePolicy, memory, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Outcome.ExitCode != 7 || publication.Snapshot.Workspace == nil || publication.Snapshot.Workspace.LastSuccessfulEntry == nil {
		t.Fatalf("publication=%#v", publication)
	}
	if publication.Snapshot.Workspace.LastSuccessfulEntry.ResolvedSpec != runtime.resolvedSpec && publication.Snapshot.Workspace.LastSuccessfulEntry.ResolvedSpec != tobari.SemanticDigest("sha256:"+strings.Repeat("8", 64)) {
		t.Fatal("entry did not publish runtime resolved spec")
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || current.Generation != previous.Generation+1 || current.Workspaces[0].LastSuccessfulEntry == nil || current.Workspaces[0].LastSuccessfulEntry.ReconciledAt != adapter.mutator.clock().UTC() {
		t.Fatalf("current=%#v present=%t err=%v", current, present, err)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 1 || runtime.confirmCalls != 1 || templatePolicy.calls != 2 || memory.confirmCalls != 2 {
		t.Fatalf("runtime=%d/%d/%d activation=%d/%d", runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, templatePolicy.calls, memory.confirmCalls)
	}
	if sessions.begin != 1 || sessions.run != 1 || sessions.close != 1 || lifecycle.held.Load() {
		t.Fatalf("session=%d/%d/%d lifecycle-held=%t", sessions.begin, sessions.run, sessions.close, lifecycle.held.Load())
	}
}

func TestContextEntryStaleActivationMakesZeroRuntimeAndEnvelopeMutation(t *testing.T) {
	collection := storeCollectionFixture(t)
	collection.Contexts[0].ActiveTemplatePolicy = nil
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	before, _ := EncodeComplete(collection)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryTemplatePolicyInactive) {
		t.Fatalf("stale Template policy activation error=%v", err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatal(err)
	}
	encoded, _ := EncodeComplete(after)
	if string(before) != string(encoded) || runtime.planCalls != 0 || runtime.reconcileCalls != 0 || sessions.begin != 0 {
		t.Fatalf("stale activation mutated authority runtime=%d/%d session=%d", runtime.planCalls, runtime.reconcileCalls, sessions.begin)
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("decision active=%t err=%v", active, err)
	}
}

func TestContextEntryCancellationBeforeDecisionIsTypedAndMakesZeroMutation(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(ctx, contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryCanceledBeforeDecision) {
		t.Fatalf("pre-decision cancellation error=%v", err)
	}
	current, _, _ := store.ReadComplete(context.Background())
	if current.Revision != collection.Revision || runtime.planCalls != 0 || sessions.begin != 0 {
		t.Fatal("pre-decision cancellation changed entry authority")
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("decision active=%t err=%v", active, err)
	}
}

func TestContextEntryInterruptionPreservesLastSuccessfulAndSameRefResumesExactDecision(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	runtime.reconcileErr = errors.New("synthetic runtime interruption")
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("runtime interruption passed")
	}
	interrupted, _, _ := store.ReadComplete(context.Background())
	if interrupted.Revision != previous.Revision || *interrupted.Workspaces[0].LastSuccessfulEntry != *previous.Workspaces[0].LastSuccessfulEntry {
		t.Fatal("failed entry advanced last-successful authority")
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || !active {
		t.Fatalf("durable decision active=%t err=%v", active, err)
	}
	if _, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), mustTemplateRef(t)); err == nil || !strings.Contains(err.Error(), "active-decision recovery") {
		t.Fatalf("different mutation was not excluded: %v", err)
	}
	runtime.reconcileErr = nil
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || publication.Snapshot.Workspace == nil || runtime.planCalls != 1 || runtime.reconcileCalls != 2 || sessions.run != 1 {
		t.Fatalf("resume publication=%#v plan=%d reconcile=%d run=%d err=%v", publication, runtime.planCalls, runtime.reconcileCalls, sessions.run, err)
	}
}

func TestContextEntryPostEffectSettlementTimeoutReleasesLifecycleAndRemainsResumable(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, mutator, adapter, lifecycle, runtime, _, _, sessions := newEntryFixture(t, collection)
	adapter.settlementTimeout = 20 * time.Millisecond
	runtime.blockConfirm = true
	contextRef, _ := tobari.ContextRef(storeContextID)
	started := time.Now()
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("settlement timeout error=%v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second || lifecycle.held.Load() {
		t.Fatalf("settlement elapsed=%s lifecycle-held=%t", elapsed, lifecycle.held.Load())
	}
	current, _, _ := store.ReadComplete(context.Background())
	if current.Revision != collection.Revision || sessions.begin != 0 {
		t.Fatal("timed-out settlement published entry or began session")
	}
	decision, active, err := mutator.readEffectDecision()
	if err != nil || !active || decision.Operation != "context-entry" {
		t.Fatalf("decision=%#v active=%t err=%v", decision, active, err)
	}
	runtime.blockConfirm = false
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || publication.Snapshot.Workspace == nil || runtime.planCalls != 1 || sessions.run != 1 {
		t.Fatalf("resume publication=%#v plan=%d run=%d err=%v", publication, runtime.planCalls, sessions.run, err)
	}
}

func TestContextEntryNoOpDecisionInterruptedBeforeReconcileStillExecutesIdempotentRecovery(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.reuseApplied = true
	interrupted := true
	adapter.afterDecision = func() error {
		if interrupted {
			interrupted = false
			return errors.New("synthetic death before runtime reconcile")
		}
		return nil
	}
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryInterrupted) {
		t.Fatalf("pre-reconcile interruption error=%v", err)
	}
	current, _, _ := store.ReadComplete(context.Background())
	if current.Revision != collection.Revision || runtime.reconcileCalls != 0 || sessions.begin != 0 {
		t.Fatal("pre-reconcile interruption changed no-op authority or runtime")
	}
	decision, active, err := mutator.readEffectDecision()
	if err != nil || !active || decision.PreviousGeneration != decision.NextGeneration || decision.PreviousRevision != decision.NextRevision {
		t.Fatalf("no-op decision=%#v active=%t err=%v", decision, active, err)
	}
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || publication.Snapshot.Workspace == nil || runtime.planCalls != 1 || runtime.reconcileCalls != 1 || runtime.confirmCalls != 1 || sessions.run != 1 {
		t.Fatalf("no-op recovery publication=%#v plan=%d reconcile=%d confirm=%d run=%d err=%v", publication, runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, sessions.run, err)
	}
}

func TestContextEntryNoOpDecisionInterruptedAfterReconcileConvergesThroughSameEffect(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.reuseApplied = true
	adapter.settlementTimeout = 20 * time.Millisecond
	runtime.blockConfirm = true
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryInterrupted) {
		t.Fatalf("post-reconcile interruption error=%v", err)
	}
	if runtime.reconcileCalls != 1 || sessions.begin != 0 {
		t.Fatalf("post-reconcile effects=%d session=%d", runtime.reconcileCalls, sessions.begin)
	}
	if decision, active, err := mutator.readEffectDecision(); err != nil || !active || decision.PreviousGeneration != decision.NextGeneration {
		t.Fatalf("decision=%#v active=%t err=%v", decision, active, err)
	}
	runtime.blockConfirm = false
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 2 || runtime.confirmCalls != 2 || sessions.run != 1 {
		t.Fatalf("idempotent recovery plan=%d reconcile=%d confirm=%d run=%d", runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, sessions.run)
	}
}

func TestContextEntryCancellationAndSessionBeginFailurePreserveConfirmedPublication(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(context.CancelFunc, *entryRuntimeFixture, *entrySessionFixture)
	}{
		{name: "canceled after confirmation", configure: func(cancel context.CancelFunc, runtime *entryRuntimeFixture, _ *entrySessionFixture) {
			runtime.onConfirm = cancel
		}},
		{name: "session begin failed", configure: func(_ context.CancelFunc, _ *entryRuntimeFixture, sessions *entrySessionFixture) {
			sessions.beginErr = errors.New("synthetic attachment startup failure")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			collection := storeCollectionFixture(t)
			store, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			test.configure(cancel, runtime, sessions)
			contextRef, _ := tobari.ContextRef(storeContextID)
			publication, err := adapter.EnterContextByReference(ctx, contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
			if !errors.Is(err, tobari.ErrWorkspaceEntryReconciliationConfirmed) || publication.Outcome.ExitCode != 0 || len(publication.Outcome.CleanupIssues) != 0 {
				t.Fatalf("publication=%#v err=%v", publication, err)
			}
			current, _, readErr := store.ReadComplete(context.Background())
			if readErr != nil || current.Generation != collection.Generation+1 || current.Workspaces[0].LastSuccessfulEntry == nil || current.Workspaces[0].LastSuccessfulEntry.ResolvedSpec != tobari.SemanticDigest("sha256:"+strings.Repeat("8", 64)) {
				t.Fatalf("confirmed collection=%#v err=%v", current, readErr)
			}
			if sessions.run != 0 {
				t.Fatal("failed attachment fabricated a session run")
			}
		})
	}
}

func TestContextEntrySessionCancellationRetainsConfirmedEntryAndClosesOwnerOnce(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, _, adapter, _, _, _, _, sessions := newEntryFixture(t, collection)
	sessions.runErr = context.Canceled
	contextRef, _ := tobari.ContextRef(storeContextID)
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if !errors.Is(err, tobari.ErrWorkspaceEntryReconciliationConfirmed) || !errors.Is(err, context.Canceled) {
		t.Fatalf("session cancellation error=%v", err)
	}
	if publication.Outcome.ExitCode != 7 || len(publication.Outcome.CleanupIssues) != 0 || sessions.run != 1 || sessions.close != 1 {
		t.Fatalf("outcome=%#v run=%d close=%d", publication.Outcome, sessions.run, sessions.close)
	}
	current, _, readErr := store.ReadComplete(context.Background())
	if readErr != nil || current.Generation != collection.Generation+1 || current.Workspaces[0].LastSuccessfulEntry == nil {
		t.Fatalf("confirmed collection=%#v err=%v", current, readErr)
	}
}

func TestContextEntryPostTerminalRenameUncertaintyReplaysWithoutRepeatedRuntimeEffect(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	originalRename := mutator.rename
	injected := false
	mutator.rename = func(oldPath, newPath string) error {
		err := originalRename(oldPath, newPath)
		if err == nil && oldPath == mutator.effectDecisionPath() && newPath == mutator.effectDecisionDonePath() && !injected {
			injected = true
			return errors.New("synthetic post-terminal-rename uncertainty")
		}
		return err
	}
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("post-terminal rename uncertainty passed")
	}
	if _, active, _ := mutator.readEffectDecision(); active {
		t.Fatal("active decision survived real active-to-terminal rename")
	}
	if terminal, present, err := mutator.readTerminalEffectDecision(); err != nil || !present || terminal.Operation != "context-entry" {
		t.Fatalf("terminal=%#v present=%t err=%v", terminal, present, err)
	}
	if runtime.reconcileCalls != 1 || sessions.begin != 0 {
		t.Fatalf("first effect/session=%d/%d", runtime.reconcileCalls, sessions.begin)
	}
	mutator.rename = originalRename
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || publication.Snapshot.Workspace == nil || runtime.reconcileCalls != 1 || runtime.confirmCalls != 2 || sessions.run != 1 {
		t.Fatalf("terminal replay publication=%#v reconcile=%d confirm=%d run=%d err=%v", publication, runtime.reconcileCalls, runtime.confirmCalls, sessions.run, err)
	}
}

func TestContextEntryTerminalObservationUnknownDoesNotAuthorizeNewReconciliation(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	confirmed, _, _ := store.ReadComplete(context.Background())
	runtime.confirmErr = errors.New("synthetic Docker inspect timeout")
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable) {
		t.Fatalf("terminal unknown observation error=%v", err)
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Revision != confirmed.Revision || after.Generation != confirmed.Generation || runtime.planCalls != 1 || runtime.reconcileCalls != 1 || sessions.run != 1 {
		t.Fatalf("unknown observation mutated plan=%d reconcile=%d session=%d", runtime.planCalls, runtime.reconcileCalls, sessions.run)
	}
	terminal, present, err := mutator.readTerminalEffectDecision()
	if err != nil || !present || terminal.Operation != "context-entry" {
		t.Fatalf("terminal=%#v present=%t err=%v", terminal, present, err)
	}
}

func TestContextEntryTerminalExactRuntimeDriftPermitsBoundedSameTargetRepair(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	confirmed, _, _ := store.ReadComplete(context.Background())
	runtime.confirmErrors = []error{tobari.ErrWorkspaceEntryRuntimeNotCurrent, nil}
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	after, _, _ := store.ReadComplete(context.Background())
	if after.Revision != confirmed.Revision || after.Generation != confirmed.Generation || runtime.planCalls != 2 || runtime.reconcileCalls != 2 || sessions.run != 2 {
		t.Fatalf("exact drift repair generation=%d/%d plan=%d reconcile=%d session=%d", confirmed.Generation, after.Generation, runtime.planCalls, runtime.reconcileCalls, sessions.run)
	}
}

func TestContextEntryCreatesFreshWorkspaceAndBindsCreationDefaults(t *testing.T) {
	collection := storeCollectionFixture(t)
	collection.Workspaces = []tobari.WorkspaceBinding{}
	collection.PendingCandidates = []tobari.PolicyCandidateAuthority{}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, _, adapter, _, _, _, _, _ := newEntryFixture(t, collection)
	contextRef, _ := tobari.ContextRef(storeContextID)
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	workspace := publication.Snapshot.Workspace
	if workspace == nil || workspace.ID == storeWorkspaceID || workspace.Home != "/workspace/home-"+string(workspace.ID) || workspace.CreationDefaults != publication.Snapshot.Template.Current.Slices.CreationDefaultsDigest {
		t.Fatalf("new Workspace=%#v", workspace)
	}
}

func TestContextEntryRejectsNonExactContainerEvidence(t *testing.T) {
	collection := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.containerID = strings.Repeat("a", 12)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("short Docker container ID passed")
	}
	current, _, _ := store.ReadComplete(context.Background())
	if current.Revision != collection.Revision || sessions.begin != 0 {
		t.Fatal("invalid container evidence advanced entry")
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || !active {
		t.Fatalf("failed exact evidence lost recovery decision active=%t err=%v", active, err)
	}
}

func TestContextEntryCleanupIssueDoesNotRewriteChildExit(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, _, adapter, _, _, _, _, sessions := newEntryFixture(t, collection)
	sessions.closeErr = errors.New("synthetic attachment cleanup failure")
	contextRef, _ := tobari.ContextRef(storeContextID)
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || publication.Outcome.ExitCode != 7 || len(publication.Outcome.CleanupIssues) != 1 || publication.Outcome.CleanupIssues[0] != tobari.WorkspaceCleanupInteractiveSession {
		t.Fatalf("outcome=%#v err=%v", publication.Outcome, err)
	}
}

func mustTemplateRef(t *testing.T) string {
	t.Helper()
	ref, err := tobari.WorkspaceTemplateRef(storeTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestContextEntryDecisionArtifactsRemainOwnerOnly(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, _ := newEntryFixture(t, collection)
	runtime.reconcileErr = errors.New("stop after decision")
	contextRef, _ := tobari.ContextRef(storeContextID)
	_, _ = adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	for _, path := range []string{mutator.effectDecisionPath(), mutator.store.root + ".wp11-mutation-stage"} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact %s mode=%v err=%v", filepath.Base(path), info.Mode(), err)
		}
	}
}
