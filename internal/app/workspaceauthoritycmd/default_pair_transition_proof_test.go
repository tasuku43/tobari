package workspaceauthoritycmd

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type defaultPairClusterRefreshFixture struct {
	root        string
	observation tobari.FinalDefaultPairObservation
	observeErr  error
	entries     int
}

func (f *defaultPairClusterRefreshFixture) ObserveFinalCanonicalProjectRoot(context.Context) (string, error) {
	return f.root, nil
}

func (f *defaultPairClusterRefreshFixture) ObserveFinalDefaultPair(context.Context, string) (tobari.FinalDefaultPairObservation, error) {
	if f.observeErr != nil {
		return tobari.FinalDefaultPairObservation{}, f.observeErr
	}
	return f.observation.Clone(), nil
}

func (f *defaultPairClusterRefreshFixture) ObserveFinalDefaultPairContext(_ context.Context, _ string, id tobari.ContextID) (tobari.FinalDefaultPairObservation, error) {
	if f.observeErr != nil {
		return tobari.FinalDefaultPairObservation{}, f.observeErr
	}
	if f.observation.Context == nil || f.observation.Context.Context.ID != id {
		return tobari.FinalDefaultPairObservation{}, tobari.ErrContextBindingNotFound
	}
	return f.observation.Clone(), nil
}

func (f *defaultPairClusterRefreshFixture) EnterContextByReference(_ context.Context, _ string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (tobari.ContextEntryPublication, error) {
	return f.EnterFinalDefaultPair(context.Background(), f.observation, f.observation.ProjectRoot, session, in, out, errOut)
}

func (f *defaultPairClusterRefreshFixture) EnterFinalDefaultPair(_ context.Context, observation tobari.FinalDefaultPairObservation, invocationRoot string, _ tobari.WorkspaceSessionRequest, _ io.Reader, _, _ io.Writer) (tobari.ContextEntryPublication, error) {
	if err := tobari.ValidateRootContains(observation.ProjectRoot, invocationRoot); err != nil {
		return tobari.ContextEntryPublication{}, err
	}
	f.entries++
	snapshot := observation.Context.Clone()
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID,
		TemplateRevision: snapshot.Template.Current.Revision, EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest,
		RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision,
		ResolvedSpec: digest("8"), ReconciledAt: time.Unix(20, 0).UTC(),
	}
	snapshot.Workspace = &tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: snapshot.Context.ID,
		ProjectRoot: "/workspace/example", Home: "/workspace/home",
		CreationDefaults: snapshot.Template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied,
	}
	snapshot.ContextHome = snapshot.Workspace.Home
	snapshot.ContextCreationDefaults = snapshot.Workspace.CreationDefaults
	snapshot.Workspaces = []tobari.WorkspaceBinding{*snapshot.Workspace}
	return tobari.ContextEntryPublication{Snapshot: snapshot, Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}, nil
}

func defaultPairClusterTransitionFixture(t *testing.T) (tobari.FinalDefaultPairObservation, tobari.FinalDefaultPairObservation, FinalClusterReconciliation) {
	t.Helper()
	snapshot := snapshotFixture(t, false, false)
	defaultID := snapshot.Template.ID
	previous, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{snapshot.Template},
		[]tobari.WorkspaceAuthorityContextRecord{{Context: snapshot.Context, PolicyMemory: snapshot.PolicyMemory}},
		[]tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, &defaultID, nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish inactive default pair: changed=%t err=%v", changed, err)
	}
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil || !transition.Plan.EnvelopeChanged {
		t.Fatalf("plan active receipts: changed=%t err=%v", transition.Plan.EnvelopeChanged, err)
	}
	before, err := tobari.NewFinalDefaultPairObservation(previous, true, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	beforeContext := snapshot.Clone()
	before.Context = &beforeContext
	if err := before.Validate(); err != nil {
		t.Fatal(err)
	}
	after, err := tobari.NewFinalDefaultPairObservation(transition.Next, true, "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	afterSnapshots, err := transition.Next.ContextSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range afterSnapshots {
		if candidate.Context.ID == snapshot.Context.ID {
			value := candidate.Clone()
			after.Context = &value
		}
	}
	if err := after.Validate(); err != nil {
		t.Fatal(err)
	}
	cluster, err := NewFinalClusterReconciliation(transition.Plan, finalClusterIdentityFixture())
	if err != nil {
		t.Fatal(err)
	}
	return before, after, cluster
}

type defaultPairReceiptDriftFixture struct {
	*defaultPairFixture
	observations int
}

func (f *defaultPairReceiptDriftFixture) ObserveFinalDefaultPair(ctx context.Context, root string) (tobari.FinalDefaultPairObservation, error) {
	observation, err := f.defaultPairFixture.ObserveFinalDefaultPair(ctx, root)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	f.observations++
	// The first pair is the stable existing-pair resolution. The second pair is
	// the application-owned fence immediately before nested Context entry.
	// Return another valid receipt on that second pass without changing
	// authority, modelling concurrent drift.
	if f.observations == 4 {
		observation.CollectionGeneration++
		observation.CollectionRevision = digest("f")
	}
	return observation, observation.Validate()
}

func (f *defaultPairReceiptDriftFixture) ObserveFinalDefaultPairContext(ctx context.Context, root string, id tobari.ContextID) (tobari.FinalDefaultPairObservation, error) {
	observation, err := f.ObserveFinalDefaultPair(ctx, root)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	if observation.Context == nil || observation.Context.Context.ID != id {
		return tobari.FinalDefaultPairObservation{}, tobari.ErrContextBindingNotFound
	}
	return observation, nil
}

func TestDefaultPairReceiptDriftBeforeNestedEntryMakesZeroEntryEffect(t *testing.T) {
	body := bodyFixture("/first-use")
	base := &defaultPairFixture{root: "/workspace/example", revisionDigit: 'a'}
	if _, err := base.InitializeFinalDefaultPair(context.Background(), base.root, body); err != nil {
		t.Fatal(err)
	}
	base.initializeCalls = 0
	base.templateCreates = 0
	base.defaultWrites = 0
	base.contextCreates = 0
	base.entries = 0
	beforeGeneration := base.generation
	fixture := &defaultPairReceiptDriftFixture{defaultPairFixture: base}
	service := NewDefaultPairService(fixture, fixture, NewContextService(fixture))
	intent := operation.Intent{
		Command: TaskDefaultPairEnter, Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID},
		Impact: DefaultPairEnterImpact(),
	}
	_, err := service.Enter(context.Background(), intent, body, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "default_pair_changed" || public.ChangeState != fault.ChangeNone {
		t.Fatalf("default receipt drift classification=%+v err=%v", public, err)
	}
	if base.initializeCalls != 0 || base.templateCreates != 0 || base.defaultWrites != 0 || base.contextCreates != 0 ||
		base.entries != 0 || base.generation != beforeGeneration {
		t.Fatalf("receipt drift crossed nested entry or changed default authority: %+v", base)
	}
}

func TestDefaultPairRefreshesCanonicalClusterReceiptBeforeEntry(t *testing.T) {
	before, after, cluster := defaultPairClusterTransitionFixture(t)
	fixture := &defaultPairClusterRefreshFixture{root: before.ProjectRoot, observation: before}
	service := NewDefaultPairService(fixture, nil, NewContextService(fixture))
	resolution := DefaultPairResolution{Observation: before, InvocationRoot: before.ProjectRoot, AuthorityChanged: false}
	fixture.observation = after
	refreshed, err := service.RefreshAfterCluster(context.Background(), resolution, cluster)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AuthorityChanged || !refreshed.Observation.SameReceipt(after) || !reflect.DeepEqual(refreshed.Observation, after) {
		t.Fatalf("refreshed resolution=%#v want receipt=%#v", refreshed, after)
	}
	result, err := service.EnterResolved(context.Background(), refreshed, tobari.NewWorkspaceShellSession(), nil, strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || fixture.entries != 1 || result.Snapshot.Workspace == nil {
		t.Fatalf("one-invocation entry=%#v entries=%d err=%v", result, fixture.entries, err)
	}
}

func TestDefaultPairRefreshPreservesConfirmedClusterMutationOnPostClusterObservationFailure(t *testing.T) {
	before, after, cluster := defaultPairClusterTransitionFixture(t)
	drifted := after.Clone()
	drifted.DefaultTemplate.Name = "drifted"
	drifted.Context.Template.Name = "drifted"
	if err := drifted.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name        string
		observation tobari.FinalDefaultPairObservation
		observeErr  error
	}{
		{name: "read failure", observation: after, observeErr: errors.New("post-cluster observation failed")},
		{name: "desired mismatch", observation: drifted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := &defaultPairClusterRefreshFixture{root: before.ProjectRoot, observation: tt.observation, observeErr: tt.observeErr}
			service := NewDefaultPairService(fixture, nil, NewContextService(fixture))
			_, err := service.RefreshAfterCluster(context.Background(), DefaultPairResolution{Observation: before, InvocationRoot: before.ProjectRoot, AuthorityChanged: false}, cluster)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "default_pair_initialized" || public.ChangeState != fault.ChangePartial || fixture.entries != 0 {
				t.Fatalf("post-cluster fault=%#v entries=%d err=%v", public, fixture.entries, err)
			}
		})
	}
}
