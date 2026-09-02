package workspaceauthoritystore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type entryRuntimeFixture struct {
	prepareCalls             int
	planCalls                int
	reconcileCalls           int
	confirmCalls             int
	reconcileErr             error
	confirmErr               error
	confirmErrors            []error
	blockConfirm             bool
	reuseApplied             bool
	onConfirm                func()
	onReconcile              func()
	resolvedSpec             tobari.SemanticDigest
	containerID              string
	homes                    map[tobari.ContextID]string
	prepareErr               error
	planErr                  error
	prepared                 []tobari.RuntimeBinding
	attachmentHeld           bool
	attachmentGets           int
	attachmentPuts           int
	reconcileFenceErr        error
	reconcileFenceReleaseErr error
	reconcileFenceGets       int
	reconcileFencePuts       int
}

type sharedEntryRuntimeFixture struct {
	*entryRuntimeFixture
	mu        sync.Mutex
	borrowers int
}

func (r *sharedEntryRuntimeFixture) AcquireWorkspaceEntryAttachment(_ context.Context, _ tobari.ContextID, _ string) (func() error, error) {
	r.mu.Lock()
	r.borrowers++
	r.mu.Unlock()
	return func() error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.borrowers--
		return nil
	}, nil
}

func (r *sharedEntryRuntimeFixture) borrowerCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.borrowers
}

func (r *sharedEntryRuntimeFixture) acquireExclusiveHomeOperation() error {
	if r.borrowerCount() != 0 {
		return tobari.ErrContextBindingProtected
	}
	return nil
}

type blockingEntrySessionAuthority struct {
	mu      sync.Mutex
	owners  []*blockingEntrySessionOwner
	started chan int
}

type blockingEntrySessionOwner struct {
	authority *blockingEntrySessionAuthority
	index     int
	release   chan struct{}
	closeOnce sync.Once
}

func (s *blockingEntrySessionAuthority) BeginWorkspaceSession(context.Context, tobari.WorkspaceSessionBinding, string) (WorkspaceSessionOwner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	owner := &blockingEntrySessionOwner{authority: s, index: len(s.owners), release: make(chan struct{})}
	s.owners = append(s.owners, owner)
	return owner, nil
}

func (o *blockingEntrySessionOwner) Run(context.Context, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.WorkspaceSessionOutcome, error) {
	o.authority.started <- o.index
	<-o.release
	return tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}, nil
}

func (o *blockingEntrySessionOwner) Close(context.Context) error {
	o.closeOnce.Do(func() {})
	return nil
}

func (r *entryRuntimeFixture) AcquireWorkspaceEntryAttachment(_ context.Context, _ tobari.ContextID, _ string) (func() error, error) {
	if r.attachmentHeld {
		return nil, tobari.ErrContextBindingProtected
	}
	r.attachmentHeld = true
	r.attachmentGets++
	return func() error {
		r.attachmentHeld = false
		r.attachmentPuts++
		return nil
	}, nil
}

func (r *entryRuntimeFixture) AcquireWorkspaceReconciliationFence(_ context.Context) (func() error, error) {
	r.reconcileFenceGets++
	if r.reconcileFenceErr != nil {
		return nil, r.reconcileFenceErr
	}
	return func() error {
		r.reconcileFencePuts++
		return r.reconcileFenceReleaseErr
	}, nil
}

func (r *entryRuntimeFixture) ContextHomeForID(_ context.Context, id tobari.ContextID) (string, error) {
	if home, exists := r.homes[id]; exists {
		return home, nil
	}
	return "/context/home-" + string(id), nil
}

func (r *entryRuntimeFixture) PrepareWorkspaceRuntimeMaterial(_ context.Context, binding tobari.RuntimeBinding) error {
	r.prepareCalls++
	r.prepared = append(r.prepared, binding)
	return r.prepareErr
}

func (r *entryRuntimeFixture) PlanWorkspaceEntry(_ context.Context, snapshot tobari.ContextAuthoritySnapshot, authority tobari.WorkspaceTemplateEntryAuthority, projectRoot string, workspaceID tobari.WorkspaceID, reconciledAt time.Time) (tobari.WorkspaceEntryReconciliationPlan, error) {
	r.planCalls++
	if r.planErr != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, r.planErr
	}
	if err := authority.ValidateFor(snapshot.Template.Current); err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: snapshot.Context.ID,
		ProjectRoot: projectRoot, Home: "/context/home-" + string(snapshot.Context.ID), CreationDefaults: snapshot.Template.Current.Slices.CreationDefaultsDigest,
	}
	if snapshot.Workspace != nil {
		workspace = *snapshot.Workspace
		if snapshot.Workspace.LastSuccessfulEntry != nil {
			previous := *snapshot.Workspace.LastSuccessfulEntry
			workspace.LastSuccessfulEntry = &previous
		}
	}
	creationDefaults := authority.CreationDefaults.Clone()
	if snapshot.Workspace != nil {
		for _, revision := range snapshot.Template.Retained {
			if revision.Slices.CreationDefaultsDigest == snapshot.Workspace.CreationDefaults {
				creationDefaults = revision.Body.CreationDefaults.Clone()
			}
		}
	}
	_, network, err := tobari.ProjectResourceNames(string(workspaceID))
	if err != nil {
		return tobari.WorkspaceEntryReconciliationPlan{}, err
	}
	networkAuthority := tobari.WorkspaceRuntimeNetworkAuthority{Network: network, Subnet: "10.64.0.0/24", DockerGateway: "10.64.0.1", GatewayIP: "10.64.0.2", WorkspaceIP: "10.64.0.3"}
	if r.reuseApplied && snapshot.Workspace != nil && snapshot.Workspace.LastSuccessfulEntry != nil {
		return tobari.WorkspaceEntryReconciliationPlan{Workspace: workspace, Applied: *workspace.LastSuccessfulEntry, Authority: authority, CreationDefaults: creationDefaults, Network: networkAuthority}, nil
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
	return tobari.WorkspaceEntryReconciliationPlan{Workspace: workspace, Applied: applied, Authority: authority, CreationDefaults: creationDefaults, Network: networkAuthority}, nil
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
	calls      int
	err        error
	memory     *policyActivationFixture
	current    *bool
	observeErr error
}

func (a *templateActivationFixture) ObserveWorkspacePolicyAxesCurrent(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.TemplatePolicyActivationReceipt, tobari.PolicyMemoryActivationReceipt) (bool, error) {
	if a.observeErr != nil {
		return false, a.observeErr
	}
	if a.current != nil {
		return *a.current, nil
	}
	return true, nil
}

func (a *templateActivationFixture) ConfirmWorkspacePolicyAxesActive(_ context.Context, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, templateReceipt tobari.TemplatePolicyActivationReceipt, memoryReceipt tobari.PolicyMemoryActivationReceipt) error {
	a.calls++
	if a.err != nil {
		return a.err
	}
	snapshot, err := snapshotForContext(collection, contextID)
	if err != nil {
		return err
	}
	if err := templateReceipt.ValidateFor(snapshot.Context, snapshot.Template.Current); err != nil {
		return err
	}
	if snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || memoryReceipt != *snapshot.ActivePolicyMemoryRef || memoryReceipt.Revision != snapshot.ActivePolicyMemory.Revision {
		return fmt.Errorf("Policy Memory active receipt mismatch")
	}
	if a.memory != nil {
		a.memory.confirmCalls++
	}
	return nil
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
	handoffs  int
	onRun     func()
}

type defaultPairContextEntryPort interface {
	EnterFinalDefaultPair(context.Context, tobari.FinalDefaultPairObservation, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

func (s *entrySessionFixture) BeginWorkspaceSession(_ context.Context, binding tobari.WorkspaceSessionBinding, invocationRoot string) (WorkspaceSessionOwner, error) {
	s.begin++
	if s.lifecycle == nil || !s.lifecycle.held.Load() {
		return nil, fmt.Errorf("session owner was not acquired under lifecycle authority")
	}
	if err := binding.Validate(); err != nil {
		return nil, fmt.Errorf("session owner received invalid final authority: %w", err)
	}
	if err := tobari.ValidateRootContains(binding.ProjectRoot, invocationRoot); err != nil {
		return nil, err
	}
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return s, nil
}

func (s *entrySessionFixture) Run(_ context.Context, _ tobari.WorkspaceSessionRequest, _ io.Reader, _, _ io.Writer) (tobari.WorkspaceSessionOutcome, error) {
	s.run++
	if s.onRun != nil {
		s.onRun()
	}
	if s.lifecycle.held.Load() {
		return tobari.WorkspaceSessionOutcome{}, fmt.Errorf("interactive session retained installation lifecycle lock")
	}
	return s.outcome, s.runErr
}

func (s *entrySessionFixture) RunWithHandoff(ctx context.Context, request tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer, handoff func()) (tobari.WorkspaceSessionOutcome, error) {
	s.handoffs++
	handoff()
	return s.Run(ctx, request, in, out, errOut)
}

func (s *entrySessionFixture) Close(context.Context) error {
	s.close++
	if s.lifecycle.held.Load() {
		return fmt.Errorf("interactive session cleanup retained installation lifecycle lock")
	}
	return s.closeErr
}

type entryPublicationBarrierFixture struct{ err error }

func (f entryPublicationBarrierFixture) CheckContextEntryPublicationBarrier(context.Context, tobari.ContextID) error {
	return f.err
}

func newEntryFixture(t *testing.T, collection tobari.WorkspaceAuthorityCollection) (*Store, *Mutator, *ContextEntryAdapter, *mutationLifecycle, *entryRuntimeFixture, *templateActivationFixture, *policyActivationFixture, *entrySessionFixture) {
	t.Helper()
	store, mutator, lifecycle, _, memory := newMutationFixture(t, &collection)
	runtime := &entryRuntimeFixture{homes: map[tobari.ContextID]string{}}
	for _, workspace := range collection.Workspaces {
		runtime.homes[workspace.ContextID] = workspace.Home
	}
	templatePolicy := &templateActivationFixture{memory: memory}
	sessions := &entrySessionFixture{lifecycle: lifecycle, outcome: tobari.WorkspaceSessionOutcome{ExitCode: 7}}
	adapter, err := NewContextEntryAdapter(mutator, runtime, templatePolicy, sessions, context.Background(), entryPublicationBarrierFixture{})
	if err != nil {
		t.Fatal(err)
	}
	return store, mutator, adapter, lifecycle, runtime, templatePolicy, memory, sessions
}

func TestContextEntryPublicationBarrierRejectsBeforeRuntimeMutation(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	adapter.publicationBarrier = entryPublicationBarrierFixture{err: tobari.ErrContextBindingProtected}
	ref, err := tobari.ContextRef(collection.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.EnterContextByReference(context.Background(), ref, tobari.NewWorkspaceShellSession(), nil, io.Discard, io.Discard); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("publication barrier error=%v", err)
	}
	if runtime.prepareCalls != 0 || runtime.planCalls != 0 || runtime.reconcileCalls != 0 || sessions.begin != 0 {
		t.Fatalf("barrier performed runtime/session effects: prepare=%d plan=%d reconcile=%d begin=%d", runtime.prepareCalls, runtime.planCalls, runtime.reconcileCalls, sessions.begin)
	}
}

func TestContextEntryRuntimeRepairIsRejectedBeforeDecisionWhileAnotherWorkspaceSessionIsLive(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.reconcileFenceErr = tobari.ErrContextBindingProtected
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("live borrower repair error=%v", err)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 0 || runtime.reconcileFenceGets != 1 || sessions.begin != 0 {
		t.Fatalf("live borrower repair mutated plan/reconcile/fence/session=%d/%d/%d/%d", runtime.planCalls, runtime.reconcileCalls, runtime.reconcileFenceGets, sessions.begin)
	}
	if decision, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("live borrower repair created decision=%#v active=%t err=%v", decision, active, err)
	}
}

func TestContextEntryReconciliationFenceReleaseFailurePreservesDecisionAndSkipsSettlement(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.reconcileFenceReleaseErr = errors.New("synthetic reconciliation fence release failure")
	contextRef, _ := tobari.ContextRef(storeContextID)
	_, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if !errors.Is(err, tobari.ErrWorkspaceEntryInterrupted) || !strings.Contains(err.Error(), "synthetic reconciliation fence release failure") {
		t.Fatalf("reconciliation fence release error=%v", err)
	}
	if runtime.reconcileCalls != 1 || runtime.confirmCalls != 0 || runtime.reconcileFenceGets != 1 || runtime.reconcileFencePuts != 1 || sessions.begin != 0 {
		t.Fatalf("release failure crossed settlement: reconcile=%d confirm=%d fence=%d/%d begin=%d", runtime.reconcileCalls, runtime.confirmCalls, runtime.reconcileFenceGets, runtime.reconcileFencePuts, sessions.begin)
	}
	decision, active, readErr := mutator.readEffectDecision()
	if readErr != nil || !active || decision.Operation != "context-entry" || decision.Target != contextRef {
		t.Fatalf("decision=%#v active=%t err=%v", decision, active, readErr)
	}
}

func TestContextEntryConfirmsIndependentAxesBeforePublishingAppliedEntryAndRunsOutsideLock(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, _, adapter, lifecycle, runtime, templatePolicy, memory, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	sessions.onRun = func() {
		if !runtime.attachmentHeld {
			t.Fatal("Workspace attachment lease was released before the live session")
		}
	}
	publication, err := adapter.EnterContextByReferenceAtRoot(context.Background(), contextRef, "/workspace/example", tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
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
	if runtime.prepareCalls != 1 || runtime.planCalls != 1 || runtime.reconcileCalls != 1 || runtime.confirmCalls != 1 || templatePolicy.calls != 1 || memory.confirmCalls != 1 {
		t.Fatalf("runtime=%d/%d/%d/%d activation=%d/%d", runtime.prepareCalls, runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, templatePolicy.calls, memory.confirmCalls)
	}
	if sessions.begin != 1 || sessions.run != 1 || sessions.close != 1 || lifecycle.held.Load() {
		t.Fatalf("session=%d/%d/%d lifecycle-held=%t", sessions.begin, sessions.run, sessions.close, lifecycle.held.Load())
	}
	if runtime.attachmentGets != 1 || runtime.attachmentPuts != 1 || runtime.attachmentHeld {
		t.Fatalf("attachment lease get/put/held=%d/%d/%t", runtime.attachmentGets, runtime.attachmentPuts, runtime.attachmentHeld)
	}
}

func TestCurrentContextEntryConfirmsAndBorrowsWithoutAnotherReconciliationDecision(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	confirmed, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("confirmed collection present=%t err=%v", present, err)
	}
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || after.Generation != confirmed.Generation || after.Revision != confirmed.Revision {
		t.Fatalf("steady entry changed authority: before=%d/%s after=%d/%s present=%t err=%v", confirmed.Generation, confirmed.Revision, after.Generation, after.Revision, present, err)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 1 || runtime.confirmCalls != 3 {
		t.Fatalf("steady entry plan/reconcile/confirm=%d/%d/%d", runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls)
	}
	if sessions.begin != 3 || sessions.run != 3 || sessions.close != 3 {
		t.Fatalf("steady sessions begin/run/close=%d/%d/%d", sessions.begin, sessions.run, sessions.close)
	}
}

func TestCurrentDefaultPairEntryBorrowsWithoutRuntimePreparationOrReconciliation(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	baseSettlement := mutator.settlement.(*finalSettlementFixture)
	mutator.settlement = &clusterSettlementFixture{finalSettlementFixture: baseSettlement}
	cluster, err := NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cluster.Reconcile(context.Background(), successfulClusterReadiness); err != nil {
		t.Fatal(err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("current authority present=%t err=%v", present, err)
	}
	observation, err := tobari.NewFinalDefaultPairObservation(current, true, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	prepareBefore, reconcileBefore, planBefore, confirmBefore := runtime.prepareCalls, runtime.reconcileCalls, runtime.planCalls, runtime.confirmCalls
	if _, err := adapter.EnterCurrentFinalDefaultPair(context.Background(), observation, observation.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || after.Generation != current.Generation || after.Revision != current.Revision {
		t.Fatalf("steady borrow changed authority: before=%d/%s after=%d/%s present=%t err=%v", current.Generation, current.Revision, after.Generation, after.Revision, present, err)
	}
	if runtime.prepareCalls != prepareBefore || runtime.reconcileCalls != reconcileBefore || runtime.planCalls != planBefore+1 || runtime.confirmCalls != confirmBefore+1 {
		t.Fatalf("steady borrow prepare/reconcile/plan/confirm=%d/%d/%d/%d before=%d/%d/%d/%d", runtime.prepareCalls, runtime.reconcileCalls, runtime.planCalls, runtime.confirmCalls, prepareBefore, reconcileBefore, planBefore, confirmBefore)
	}
	if sessions.begin != 2 || sessions.run != 2 || sessions.close != 2 {
		t.Fatalf("steady sessions begin/run/close=%d/%d/%d", sessions.begin, sessions.run, sessions.close)
	}
}

func TestCurrentDefaultPairEntryLeavesRecoveryArtifactsByteExact(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("current authority present=%t err=%v", present, err)
	}
	observation, err := tobari.NewFinalDefaultPairObservation(current, true, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	path := mutator.effectDecisionTempPath()
	want := []byte("preserved exact recovery bytes")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	prepareBefore, reconcileBefore, planBefore, confirmBefore, beginBefore := runtime.prepareCalls, runtime.reconcileCalls, runtime.planCalls, runtime.confirmCalls, sessions.begin
	if _, err := adapter.EnterCurrentFinalDefaultPair(context.Background(), observation, observation.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable) {
		t.Fatalf("recovery residue error=%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("recovery residue=%q err=%v", got, err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("recovery residue mode=%v err=%v", info.Mode(), err)
	}
	if runtime.prepareCalls != prepareBefore || runtime.reconcileCalls != reconcileBefore || runtime.planCalls != planBefore || runtime.confirmCalls != confirmBefore || sessions.begin != beginBefore {
		t.Fatalf("residue path mutated runtime/session prepare=%d/%d reconcile=%d/%d plan=%d/%d confirm=%d/%d begin=%d/%d", runtime.prepareCalls, prepareBefore, runtime.reconcileCalls, reconcileBefore, runtime.planCalls, planBefore, runtime.confirmCalls, confirmBefore, sessions.begin, beginBefore)
	}
}

func TestCurrentDefaultPairEntryMapsRuntimePlanNotReadyToCanonicalRepair(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	baseSettlement := mutator.settlement.(*finalSettlementFixture)
	mutator.settlement = &clusterSettlementFixture{finalSettlementFixture: baseSettlement}
	cluster, err := NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cluster.Reconcile(context.Background(), successfulClusterReadiness); err != nil {
		t.Fatal(err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("current authority present=%t err=%v", present, err)
	}
	observation, err := tobari.NewFinalDefaultPairObservation(current, true, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	prepareBefore, reconcileBefore, planBefore, beginBefore := runtime.prepareCalls, runtime.reconcileCalls, runtime.planCalls, sessions.begin
	runtime.planErr = tobari.ErrRuntimeNotReady
	if _, err := adapter.EnterCurrentFinalDefaultPair(context.Background(), observation, observation.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceEntryRuntimeNotCurrent) || !errors.Is(err, tobari.ErrRuntimeNotReady) {
		t.Fatalf("current Runtime repair disposition=%v", err)
	}
	if runtime.prepareCalls != prepareBefore || runtime.reconcileCalls != reconcileBefore || runtime.planCalls != planBefore+1 || sessions.begin != beginBefore {
		t.Fatalf("current Runtime classification mutated prepare/reconcile/plan/begin=%d/%d %d/%d %d/%d %d/%d", runtime.prepareCalls, prepareBefore, runtime.reconcileCalls, reconcileBefore, runtime.planCalls, planBefore, sessions.begin, beginBefore)
	}
}

func TestCurrentDefaultPairEntryPreservesEveryRecoveryArtifactCombination(t *testing.T) {
	for _, test := range []struct {
		name       string
		keepActive bool
		keepStage  bool
	}{
		{name: "active only", keepActive: true},
		{name: "stage only", keepStage: true},
		{name: "active and stage", keepActive: true, keepStage: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous := storeCollectionFixture(t)
			store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
			current, present, err := store.ReadComplete(context.Background())
			if err != nil || !present {
				t.Fatalf("current authority present=%t err=%v", present, err)
			}
			observation, err := tobari.NewFinalDefaultPairObservation(current, true, "/workspace/example")
			if err != nil {
				t.Fatal(err)
			}
			runtime.reconcileErr = errors.New("stop after durable recovery artifacts")
			contextRef, _ := tobari.ContextRef(storeContextID)
			_, _ = adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
			activePath, stagePath := mutator.effectDecisionPath(), mutationStagePath(mutator.store.root)
			if !test.keepActive {
				if err := os.Remove(activePath); err != nil {
					t.Fatal(err)
				}
			}
			if !test.keepStage {
				if err := os.Remove(stagePath); err != nil {
					t.Fatal(err)
				}
			}
			paths := []string{}
			if test.keepActive {
				paths = append(paths, activePath)
			}
			if test.keepStage {
				paths = append(paths, stagePath)
			}
			type artifact struct {
				data []byte
				mode os.FileMode
			}
			before := map[string]artifact{}
			for _, path := range paths {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = artifact{data: data, mode: info.Mode()}
			}
			planBefore, confirmBefore, beginBefore := runtime.planCalls, runtime.confirmCalls, sessions.begin
			if _, err := adapter.EnterCurrentFinalDefaultPair(context.Background(), observation, observation.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil || (!errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable) && !errors.Is(err, tobari.ErrWorkspaceEntryInterrupted)) {
				t.Fatalf("recovery residue error=%v", err)
			}
			for path, want := range before {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				info, err := os.Lstat(path)
				if err != nil || !bytes.Equal(got, want.data) || info.Mode() != want.mode {
					t.Fatalf("artifact %s data=%q mode=%v err=%v", filepath.Base(path), got, info.Mode(), err)
				}
			}
			if runtime.planCalls != planBefore || runtime.confirmCalls != confirmBefore || sessions.begin != beginBefore {
				t.Fatalf("residue path crossed plan/confirm/session: %d/%d %d/%d %d/%d", runtime.planCalls, planBefore, runtime.confirmCalls, confirmBefore, sessions.begin, beginBefore)
			}
		})
	}
}

func TestCurrentContextEntryBorrowsWhileCompletedClusterReceiptAndAnotherWorkspaceSessionExist(t *testing.T) {
	previous := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	baseSettlement, ok := mutator.settlement.(*finalSettlementFixture)
	if !ok {
		t.Fatalf("settlement fixture type=%T", mutator.settlement)
	}
	clusterSettlement := &clusterSettlementFixture{finalSettlementFixture: baseSettlement}
	mutator.settlement = clusterSettlement
	cluster, err := NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cluster.Reconcile(context.Background(), successfulClusterReadiness); err != nil {
		t.Fatal(err)
	}
	terminal, present, err := mutator.readTerminalEffectDecision()
	if err != nil || !present || terminal.Operation != finalClusterReconciliationOperation {
		t.Fatalf("cluster terminal=%#v present=%t err=%v", terminal, present, err)
	}
	reconcileCalls := runtime.reconcileCalls
	reconcileFenceGets := runtime.reconcileFenceGets
	runtime.reconcileFenceErr = tobari.ErrContextBindingProtected
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatalf("shared steady entry was blocked by a live Workspace session: %v", err)
	}
	if runtime.reconcileCalls != reconcileCalls || runtime.reconcileFenceGets != reconcileFenceGets || runtime.confirmCalls < 2 {
		t.Fatalf("steady entry reconciled behind terminal receipt: reconcile=%d/%d fence=%d/%d confirm=%d", runtime.reconcileCalls, reconcileCalls, runtime.reconcileFenceGets, reconcileFenceGets, runtime.confirmCalls)
	}
	if sessions.begin != 2 || sessions.run != 2 || sessions.close != 2 {
		t.Fatalf("shared sessions begin/run/close=%d/%d/%d", sessions.begin, sessions.run, sessions.close)
	}
}

func TestCurrentContextEntryKeepsTwoLiveBorrowersAndExclusiveHomeOperationsBlockedUntilLastClose(t *testing.T) {
	previous := storeCollectionFixture(t)
	_, mutator, establishment, _, baseRuntime, templatePolicy, _, _ := newEntryFixture(t, previous)
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := establishment.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	baseSettlement := mutator.settlement.(*finalSettlementFixture)
	mutator.settlement = &clusterSettlementFixture{finalSettlementFixture: baseSettlement}
	cluster, err := NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cluster.Reconcile(context.Background(), successfulClusterReadiness); err != nil {
		t.Fatal(err)
	}
	baseRuntime.reconcileFenceErr = tobari.ErrContextBindingProtected
	sharedRuntime := &sharedEntryRuntimeFixture{entryRuntimeFixture: baseRuntime}
	sessions := &blockingEntrySessionAuthority{started: make(chan int, 2)}
	adapter, err := NewContextEntryAdapter(mutator, sharedRuntime, templatePolicy, sessions, context.Background(), entryPublicationBarrierFixture{})
	if err != nil {
		t.Fatal(err)
	}
	type result struct{ err error }
	results := make(chan result, 2)
	enter := func() {
		_, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
		results <- result{err: err}
	}
	go enter()
	if index := <-sessions.started; index != 0 {
		t.Fatalf("first live owner index=%d", index)
	}
	go enter()
	if index := <-sessions.started; index != 1 {
		t.Fatalf("second live owner index=%d", index)
	}
	if borrowers := sharedRuntime.borrowerCount(); borrowers != 2 {
		t.Fatalf("live shared borrowers=%d", borrowers)
	}
	if err := sharedRuntime.acquireExclusiveHomeOperation(); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("exclusive Home operation crossed two live borrowers: %v", err)
	}
	close(sessions.owners[0].release)
	if result := <-results; result.err != nil {
		t.Fatalf("first owner exit: %v", result.err)
	}
	if borrowers := sharedRuntime.borrowerCount(); borrowers != 1 {
		t.Fatalf("first close released all borrowers: %d", borrowers)
	}
	if err := sharedRuntime.acquireExclusiveHomeOperation(); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("exclusive Home operation crossed remaining borrower: %v", err)
	}
	close(sessions.owners[1].release)
	if result := <-results; result.err != nil {
		t.Fatalf("second owner exit: %v", result.err)
	}
	if borrowers := sharedRuntime.borrowerCount(); borrowers != 0 {
		t.Fatalf("last close retained borrower: %d", borrowers)
	}
	if err := sharedRuntime.acquireExclusiveHomeOperation(); err != nil {
		t.Fatalf("exclusive Home operation remained blocked after last close: %v", err)
	}
}

func TestContextEntryPreparesExactSelectedRuntimeBeforeReadOnlyPlan(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "build failure", err: errors.New("synthetic standard Runtime preparation failure")},
		{name: "canceled build", err: context.Canceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous := storeCollectionFixture(t)
			_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
			runtime.prepareErr = errors.Join(tobari.ErrWorkspaceRuntimePreparationUncertain, test.err)
			contextRef, _ := tobari.ContextRef(storeContextID)
			if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, tobari.ErrWorkspaceRuntimePreparationUncertain) {
				t.Fatalf("Runtime preparation failure classification=%v", err)
			}
			if runtime.prepareCalls != 1 || len(runtime.prepared) != 1 || runtime.prepared[0].RuntimeID != tobari.StandardRuntimeID || runtime.planCalls != 0 || runtime.reconcileCalls != 0 || sessions.begin != 0 {
				t.Fatalf("prepare=%d bindings=%+v plan=%d reconcile=%d session=%d", runtime.prepareCalls, runtime.prepared, runtime.planCalls, runtime.reconcileCalls, sessions.begin)
			}
			if _, active, err := mutator.readEffectDecision(); err != nil || active {
				t.Fatalf("preparation failure published decision: active=%t err=%v", active, err)
			}
		})
	}
}

func TestContextEntryKeepsCustomRuntimePreparationObservationReadOnly(t *testing.T) {
	previous := storeCollectionFixture(t)
	template := previous.Templates[0].Clone()
	body := template.Current.Body.Clone()
	body.EntryDefaults.Runtime = tobari.RuntimeBinding{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077",
		Name:      "custom",
		Revision:  "sha256:" + strings.Repeat("c", 64),
		Ordinal:   1,
		Image:     "custom-runtime:test",
	}
	nextRevision, changed, err := tobari.AdvanceWorkspaceTemplateRevision(template.Current, body)
	if err != nil || !changed {
		t.Fatalf("advance custom Runtime Template: changed=%t err=%v", changed, err)
	}
	template.Current = nextRevision
	template.Retained = append(template.Retained, nextRevision.Clone())
	custom, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, previous.Contexts, previous.Workspaces,
		previous.PendingCandidates, previous.DefaultTemplateID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, custom)
	runtime.prepareErr = errors.New("synthetic custom Runtime observation failure")
	contextRef, _ := tobari.ContextRef(storeContextID)
	_, err = adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if !errors.Is(err, tobari.ErrWorkspaceEntryObservationUnavailable) || errors.Is(err, tobari.ErrWorkspaceRuntimePreparationUncertain) {
		t.Fatalf("custom Runtime preparation classification=%v", err)
	}
	if runtime.prepareCalls != 1 || len(runtime.prepared) != 1 || runtime.prepared[0].RuntimeID != body.EntryDefaults.Runtime.RuntimeID || runtime.planCalls != 0 || sessions.begin != 0 {
		t.Fatalf("prepare=%d bindings=%+v plan=%d session=%d", runtime.prepareCalls, runtime.prepared, runtime.planCalls, sessions.begin)
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("custom Runtime observation published decision: active=%t err=%v", active, err)
	}
}

func TestDefaultPairEntryRechecksExactReceiptInsideLifecycleLockBeforeRuntimeEffect(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, _, adapter, lifecycle, runtime, templatePolicy, memory, sessions := newEntryFixture(t, previous)
	expected, err := tobari.NewFinalDefaultPairObservation(previous, true, previous.Workspaces[0].ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := any(adapter).(defaultPairContextEntryPort)
	if !ok {
		t.Fatalf("ContextEntryAdapter is missing the task-owned default-pair receipt entry seam")
	}
	lifecycle.before = func() {
		drifted, changed, publishErr := tobari.PublishWorkspaceAuthorityCollection(
			previous.Templates, previous.Contexts, previous.Workspaces, previous.PendingCandidates, nil, &previous,
		)
		if publishErr != nil || !changed {
			t.Fatalf("prepare default-selection drift: changed=%t err=%v", changed, publishErr)
		}
		encoded, encodeErr := EncodeComplete(drifted)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		writeAuthorityBytes(t, store.root, encoded)
		lifecycle.before = nil
	}
	if _, err := entry.EnterFinalDefaultPair(context.Background(), expected, expected.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("default-pair entry accepted collection/default drift inside the lifecycle lock")
	}
	if runtime.planCalls != 0 || runtime.reconcileCalls != 0 || runtime.confirmCalls != 0 || templatePolicy.calls != 0 || memory.confirmCalls != 0 || sessions.begin != 0 || sessions.run != 0 || sessions.close != 0 {
		t.Fatalf("drift performed runtime/session effect: runtime=%d/%d/%d activation=%d/%d session=%d/%d/%d",
			runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, templatePolicy.calls, memory.confirmCalls, sessions.begin, sessions.run, sessions.close)
	}
}

func TestDefaultPairEntryProgressEndsBeforeSessionStreamHandoff(t *testing.T) {
	previous := storeCollectionFixture(t)
	_, _, adapter, _, _, _, _, sessions := newEntryFixture(t, previous)
	expected, err := tobari.NewFinalDefaultPairObservation(previous, true, previous.Workspaces[0].ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	events := []tobari.FirstEntryProgress{}
	publication, err := adapter.EnterFinalDefaultPairWithProgress(
		context.Background(), expected, expected.ProjectRoot, tobari.NewWorkspaceShellSession(),
		func(event tobari.FirstEntryProgress) { events = append(events, event) },
		strings.NewReader(""), io.Discard, io.Discard,
	)
	if err != nil || publication.Outcome.ExitCode != 7 {
		t.Fatalf("publication=%+v err=%v", publication, err)
	}
	want := []tobari.FirstEntryProgress{
		{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageSucceeded},
		{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageRunning},
		{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageSucceeded},
	}
	if !reflect.DeepEqual(events, want) || sessions.handoffs != 1 || sessions.run != 1 {
		t.Fatalf("events=%+v handoffs=%d runs=%d", events, sessions.handoffs, sessions.run)
	}
}

func TestContextEntryInactiveActivationSettlesWithinParentAction(t *testing.T) {
	collection := storeCollectionFixture(t)
	collection.Contexts[0].ActiveTemplatePolicy = nil
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	contextRef, _ := tobari.ContextRef(storeContextID)
	publication, err := adapter.EnterContextByReferenceAtRoot(context.Background(), contextRef, "/workspace/example", tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatal(err)
	}
	if after.Revision == collection.Revision || publication.Snapshot.ActiveTemplatePolicy == nil || publication.Snapshot.ActivePolicyMemory == nil || publication.Snapshot.ActivePolicyMemoryRef == nil || runtime.planCalls != 1 || runtime.reconcileCalls != 1 || sessions.begin != 1 {
		t.Fatalf("inactive activation did not settle authority runtime=%d/%d session=%d snapshot=%+v", runtime.planCalls, runtime.reconcileCalls, sessions.begin, publication.Snapshot)
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
	recovery, err := store.ObserveMutationRecovery(context.Background())
	if err != nil || !recovery.ActiveDecision || !recovery.StagePresent || recovery.Operation != "context-entry" || recovery.Target != contextRef {
		t.Fatalf("mutation recovery observation=%#v err=%v", recovery, err)
	}
	if _, err := mutator.SetDefaultWorkspaceTemplateByReference(context.Background(), mustTemplateRef(t)); err == nil || !strings.Contains(err.Error(), "active-decision recovery") {
		t.Fatalf("different mutation was not excluded: %v", err)
	}
	runtime.reconcileErr = nil
	publication, err := adapter.EnterContextByReferenceAtRoot(context.Background(), contextRef, "/workspace/example", tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
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

func TestContextEntryNoOpObservationTimeoutRemainsReadOnlyAndRetryable(t *testing.T) {
	collection := storeCollectionFixture(t)
	_, mutator, adapter, _, runtime, _, _, sessions := newEntryFixture(t, collection)
	runtime.reuseApplied = true
	adapter.settlementTimeout = 20 * time.Millisecond
	runtime.blockConfirm = true
	contextRef, _ := tobari.ContextRef(storeContextID)
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, tobari.ErrWorkspaceEntryInterrupted) {
		t.Fatalf("read-only confirmation timeout error=%v", err)
	}
	if runtime.reconcileCalls != 0 || sessions.begin != 0 {
		t.Fatalf("read-only confirmation performed effects=%d session=%d", runtime.reconcileCalls, sessions.begin)
	}
	if decision, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("read-only confirmation created decision=%#v active=%t err=%v", decision, active, err)
	}
	runtime.blockConfirm = false
	if _, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if runtime.planCalls != 2 || runtime.reconcileCalls != 0 || runtime.confirmCalls != 2 || sessions.run != 1 {
		t.Fatalf("read-only retry plan=%d reconcile=%d confirm=%d run=%d", runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, sessions.run)
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
	publication, err := adapter.EnterContextByReferenceAtRoot(context.Background(), contextRef, "/workspace/example", tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	workspace := publication.Snapshot.Workspace
	if workspace == nil || workspace.ID == storeWorkspaceID || workspace.Home != "/context/home-"+string(workspace.ContextID) || workspace.CreationDefaults != publication.Snapshot.Template.Current.Slices.CreationDefaultsDigest {
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
	for _, path := range []string{mutator.effectDecisionPath(), mutationStagePath(mutator.store.root)} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("artifact %s mode=%v err=%v", filepath.Base(path), info.Mode(), err)
		}
	}
}
