package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type firstEntryProcessLifetimeMarker struct{}

type failingConfiguratorStatusWriter struct{}

func (failingConfiguratorStatusWriter) Write([]byte) (int, error) {
	return 0, errors.New("status output failed")
}

type firstEntryPairFixture struct {
	order               *[]string
	observation         tobari.FinalDefaultPairObservation
	resolution          workspaceauthoritycmd.DefaultPairResolution
	resolveBody         *tobari.WorkspaceTemplateBody
	session             tobari.WorkspaceSessionRequest
	outcome             tobari.WorkspaceSessionOutcome
	resolveCalls        int
	contextResolveCalls int
	refreshCalls        int
	refreshContext      context.Context
	refreshCtxErr       error
	entryCalls          int
	cancelAt            string
	cancel              context.CancelFunc
	resolveErr          error
	refreshErr          error
	entryErr            error
	currentEntryErr     error
	recovery            tobari.FinalAuthorityMutationObservation
	recoveryErr         error
	selectionErr        error
	selected            *workspaceauthoritycmd.SelectedDefaultPair
}

func (f *firstEntryPairFixture) Observe(context.Context) (tobari.FinalDefaultPairObservation, error) {
	*f.order = append(*f.order, "observe")
	return f.observation.Clone(), nil
}

func (f *firstEntryPairFixture) Select(context.Context, io.Reader, io.Writer) (workspaceauthoritycmd.SelectedDefaultPair, error) {
	*f.order = append(*f.order, "observe")
	if f.selectionErr != nil {
		return workspaceauthoritycmd.SelectedDefaultPair{}, f.selectionErr
	}
	if f.selected != nil {
		return *f.selected, nil
	}
	selection := tobari.FinalDefaultPairSelection{
		SchemaVersion: tobari.FinalDefaultPairSelectionSchemaVersion, CollectionPresent: f.observation.CollectionPresent,
		CanonicalCWD: f.observation.ProjectRoot, Candidates: []tobari.FinalDefaultPairCandidate{},
	}
	return workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionCreate}}, nil
}

func TestFinalRootAlreadyInsideMatchesCatalogBeforeReadiness(t *testing.T) {
	command, pair, readiness, cluster, _, _, stderr, order := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	pair.selectionErr = fault.New(fault.KindRejected, "already_inside", "This process is already inside a Workspace; nested entry is not supported", false,
		fault.NextAction{Command: "help tobari", Reason: "Exit the current Workspace session before entering another."})
	if code := command.RunContext(context.Background(), nil); code != ExitRejected {
		t.Fatalf("inside exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe"}) || readiness.calls != 0 || cluster.calls != 0 || pair.resolveCalls != 0 || !strings.Contains(stderr.String(), "already_inside") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("inside order=%v stderr=%q", *order, stderr.String())
	}
}

func TestFinalRootEmitsDeclaredWorkspaceBusyFault(t *testing.T) {
	command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	pair.entryErr = fault.WithClassification(fault.New(
		fault.KindUnavailable, "workspace_entry_busy", "Workspace entry is temporarily blocked by a live Workspace session or an exclusive Context Home operation", true,
		fault.NextAction{Command: "status", Reason: "Read current authority, then retry after the blocking Workspace session or exclusive Context Home operation finishes."},
	), fault.PhasePrecondition, fault.ChangeNone)
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("root busy exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "workspace_entry_busy") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("root busy fault=%q", stderr.String())
	}
}

func TestFinalRootEmitsDeclaredGatewayBuildFault(t *testing.T) {
	command, _, _, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	cluster.err = fault.New(fault.KindUnavailable, "gateway_image_build_failed", "The pinned Gateway image could not be built.", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect Docker build support and network access for the pinned Gateway inputs."})
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("root Gateway build exit=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "gateway_image_build_failed") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("root Gateway build fault=%q", stderr.String())
	}
}

func TestFinalRootIgnoresRetiredAggregateConfiguratorDrafts(t *testing.T) {
	command, pair, readiness, cluster, reviewer, _, stderr, order := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("existing entry exit=%d stderr=%q", code, stderr.String())
	}
	if reviewer.calls != 0 || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 1 || !reflect.DeepEqual(*order, []string{"observe", "readiness", "resolve", "cluster", "refresh", "entry"}) {
		t.Fatalf("retired aggregate state affected root entry: order=%v", *order)
	}
}

func TestFinalRootExplicitCreateDoesNotReuseAttachedCurrentContext(t *testing.T) {
	command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("create-here exit=%d stderr=%q", code, stderr.String())
	}
	if pair.contextResolveCalls != 0 || pair.resolveCalls != 1 {
		t.Fatalf("attached current Context was reused: context resolves=%d ordinary resolves=%d", pair.contextResolveCalls, pair.resolveCalls)
	}
}

func TestFinalRootExplicitCreateUsesUnattachedCurrentContext(t *testing.T) {
	command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	snapshot.Workspace = nil
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	command.finalContexts = workspaceauthoritycmd.NewContextService(&currentContextSelectionFixture{snapshot: snapshot})
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("create-here exit=%d stderr=%q", code, stderr.String())
	}
	if pair.contextResolveCalls != 1 || pair.resolveCalls != 1 {
		t.Fatalf("unattached current Context was not used: context resolves=%d ordinary resolves=%d", pair.contextResolveCalls, pair.resolveCalls)
	}
}

func TestFinalRootCurrentWorkspaceEntersDirectlyWithoutSetupProgressOrClusterMutation(t *testing.T) {
	command, pair, readiness, cluster, _, stdout, stderr, order := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, true)
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, PolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: snapshot.Context.ID, Revision: snapshot.PolicyMemory.Revision}
	activeMemory := snapshot.PolicyMemory.Clone()
	snapshot.ActiveTemplatePolicy = &templateReceipt
	snapshot.ActivePolicyMemory = &activeMemory
	snapshot.ActivePolicyMemoryRef = &memoryReceipt
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision,
		EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest, RuntimeID: snapshot.Template.Current.Body.EntryDefaults.Runtime.RuntimeID,
		RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	snapshot.Workspace.LastSuccessfulEntry = &applied
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	observationSource := statusObservationFromSnapshot(t, snapshot)
	selection, err := tobari.NewFinalDefaultPairSelection(observationSource.Collection, true, snapshot.Workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	choice := tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: snapshot.Context.ID}
	observation, err := selection.Observation(choice)
	if err != nil {
		t.Fatal(err)
	}
	selected := workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: choice}
	pair.observation = observation
	pair.selected = &selected
	pair.resolution = workspaceauthoritycmd.DefaultPairResolution{Observation: observation, InvocationRoot: observation.ProjectRoot}
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("direct current entry exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "resolve", "entry"}) || readiness.calls != 0 || cluster.calls != 0 || pair.resolveCalls != 1 || pair.refreshCalls != 0 || pair.entryCalls != 1 {
		t.Fatalf("direct current entry order=%v readiness=%d cluster=%d resolve=%d refresh=%d entry=%d", *order, readiness.calls, cluster.calls, pair.resolveCalls, pair.refreshCalls, pair.entryCalls)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("direct current entry replayed setup presentation stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootCurrentAuthorityWithStoppedProtectionReturnsToCanonicalRecoveryFlow(t *testing.T) {
	command, pair, readiness, cluster, _, _, stderr, order := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, true)
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, PolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: snapshot.Context.ID, Revision: snapshot.PolicyMemory.Revision}
	activeMemory := snapshot.PolicyMemory.Clone()
	snapshot.ActiveTemplatePolicy, snapshot.ActivePolicyMemory, snapshot.ActivePolicyMemoryRef = &templateReceipt, &activeMemory, &memoryReceipt
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision,
		EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest, RuntimeID: snapshot.Template.Current.Body.EntryDefaults.Runtime.RuntimeID,
		RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	snapshot.Workspace.LastSuccessfulEntry = &applied
	observationSource := statusObservationFromSnapshot(t, snapshot)
	selection, err := tobari.NewFinalDefaultPairSelection(observationSource.Collection, true, snapshot.Workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	choice := tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: snapshot.Context.ID}
	observation, err := selection.Observation(choice)
	if err != nil {
		t.Fatal(err)
	}
	pair.observation = observation
	pair.selected = &workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: choice}
	pair.resolution = workspaceauthoritycmd.DefaultPairResolution{Observation: observation, InvocationRoot: observation.ProjectRoot}
	pair.currentEntryErr = fault.WithClassification(fault.Wrap(
		fault.KindUnavailable, "workspace_entry_repair_required", "Workspace entry requires canonical Runtime or protection recovery", true,
		tobari.ErrWorkspaceEntryProtectionNotCurrent, fault.NextAction{Command: "tobari", Reason: "Repeat root entry."},
	), fault.PhasePrecondition, fault.ChangeNone)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("recovery entry exit=%d stderr=%q", code, stderr.String())
	}
	wantOrder := []string{"observe", "resolve", "entry", "readiness", "resolve", "cluster", "refresh", "entry"}
	if !reflect.DeepEqual(*order, wantOrder) || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 2 || pair.refreshCalls != 1 || pair.entryCalls != 2 {
		t.Fatalf("recovery order=%v readiness=%d cluster=%d resolve=%d refresh=%d entry=%d", *order, readiness.calls, cluster.calls, pair.resolveCalls, pair.refreshCalls, pair.entryCalls)
	}
	if !strings.Contains(stderr.String(), "Ensure protection") || !strings.Contains(stderr.String(), "Ensure Workspace") {
		t.Fatalf("recovery omitted canonical progress: %q", stderr.String())
	}
}

func TestFinalRootCurrentAuthorityWithMissingStandardRuntimeReturnsToCanonicalRecoveryFlow(t *testing.T) {
	command, pair, readiness, cluster, _, _, stderr, order := newFirstEntryCLI(t, false, true, recommendedFirstUseStart)
	snapshot, _, _, _, _ := finalDesiredActiveSnapshotFixture(t, true)
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, PolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: snapshot.Context.ID, Revision: snapshot.PolicyMemory.Revision}
	activeMemory := snapshot.PolicyMemory.Clone()
	snapshot.ActiveTemplatePolicy, snapshot.ActivePolicyMemory, snapshot.ActivePolicyMemoryRef = &templateReceipt, &activeMemory, &memoryReceipt
	snapshot.Workspace.LastSuccessfulEntry = &tobari.WorkspaceAppliedEntry{
		ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision,
		EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest, RuntimeID: snapshot.Template.Current.Body.EntryDefaults.Runtime.RuntimeID,
		RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	observationSource := statusObservationFromSnapshot(t, snapshot)
	selection, err := tobari.NewFinalDefaultPairSelection(observationSource.Collection, true, snapshot.Workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	choice := tobari.FinalDefaultPairSelectionChoice{Kind: tobari.FinalDefaultPairSelectionUse, ContextID: snapshot.Context.ID}
	observation, err := selection.Observation(choice)
	if err != nil {
		t.Fatal(err)
	}
	pair.observation = observation
	pair.selected = &workspaceauthoritycmd.SelectedDefaultPair{Selection: selection, Choice: choice}
	pair.resolution = workspaceauthoritycmd.DefaultPairResolution{Observation: observation, InvocationRoot: observation.ProjectRoot}
	pair.currentEntryErr = fault.WithClassification(fault.Wrap(
		fault.KindUnavailable, "workspace_entry_repair_required", "Workspace entry requires canonical Runtime or protection recovery", true,
		tobari.ErrWorkspaceEntryRuntimeNotCurrent, fault.NextAction{Command: "tobari", Reason: "Repeat root entry."},
	), fault.PhasePrecondition, fault.ChangeNone)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("recovery entry exit=%d stderr=%q", code, stderr.String())
	}
	wantOrder := []string{"observe", "resolve", "entry", "readiness", "resolve", "cluster", "refresh", "entry"}
	if !reflect.DeepEqual(*order, wantOrder) || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 2 || pair.refreshCalls != 1 || pair.entryCalls != 2 {
		t.Fatalf("recovery order=%v readiness=%d cluster=%d resolve=%d refresh=%d entry=%d", *order, readiness.calls, cluster.calls, pair.resolveCalls, pair.refreshCalls, pair.entryCalls)
	}
	if !strings.Contains(stderr.String(), "Ensure protection") || !strings.Contains(stderr.String(), "Ensure Workspace") {
		t.Fatalf("recovery omitted canonical progress: %q", stderr.String())
	}
}

func (f *firstEntryPairFixture) ObserveMutationRecovery(context.Context) (tobari.FinalAuthorityMutationObservation, error) {
	return f.recovery, f.recoveryErr
}

func (f *firstEntryPairFixture) Resolve(_ context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.resolveCalls++
	*f.order = append(*f.order, "resolve")
	if f.cancelAt == "resolve" && f.cancel != nil {
		f.cancel()
	}
	if intent.Command != WorkspaceEntryCommandPath || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.CurrentDirectoryTargetKind || intent.Target.ParentID != tobari.CurrentDirectoryTargetID {
		panic("root resolution lost its Catalog mutation binding")
	}
	f.resolveBody = body
	return f.resolution, f.resolveErr
}

func (f *firstEntryPairFixture) ResolveSelected(ctx context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody, _ workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error) {
	return f.Resolve(ctx, intent, body)
}

func (f *firstEntryPairFixture) ResolveSelectedContext(ctx context.Context, _ tobari.ContextID, selected workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.contextResolveCalls++
	intent := operation.Intent{Command: WorkspaceEntryCommandPath, Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID}}
	return f.ResolveSelected(ctx, intent, nil, selected)
}

func (f *firstEntryPairFixture) ResolveSelectedWithTemplateID(ctx context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody, _ tobari.WorkspaceTemplateID, selected workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error) {
	return f.ResolveSelected(ctx, intent, body, selected)
}

func (f *firstEntryPairFixture) ResolveSelectedWithConfiguratorIDs(ctx context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody, _ tobari.WorkspaceTemplateID, contextID tobari.ContextID, selected workspaceauthoritycmd.SelectedDefaultPair) (workspaceauthoritycmd.DefaultPairResolution, error) {
	resolution, err := f.ResolveSelected(ctx, intent, body, selected)
	if err == nil && resolution.Observation.Context != nil && resolution.Observation.Context.Context.ID != contextID {
		return workspaceauthoritycmd.DefaultPairResolution{}, tobari.ErrResourceSourceRecoveryRequired
	}
	return resolution, err
}

func (f *firstEntryPairFixture) RefreshAfterCluster(ctx context.Context, resolution workspaceauthoritycmd.DefaultPairResolution, _ workspaceauthoritycmd.FinalClusterReconciliation) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.refreshCalls++
	f.refreshContext = ctx
	f.refreshCtxErr = ctx.Err()
	*f.order = append(*f.order, "refresh")
	if f.cancelAt == "refresh" && f.cancel != nil {
		f.cancel()
	}
	return resolution, f.refreshErr
}

func (f *firstEntryPairFixture) EnterResolved(_ context.Context, _ workspaceauthoritycmd.DefaultPairResolution, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, _ io.Reader, _, _ io.Writer) (workspaceauthoritycmd.ContextEntryResult, error) {
	f.entryCalls++
	*f.order = append(*f.order, "entry")
	f.session = session
	if f.cancelAt == "entry" && f.cancel != nil {
		f.cancel()
	}
	if f.entryErr != nil {
		return workspaceauthoritycmd.ContextEntryResult{}, f.entryErr
	}
	if progress != nil {
		progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageSucceeded})
		progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageRunning})
		progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageSucceeded})
	}
	return workspaceauthoritycmd.ContextEntryResult{Outcome: f.outcome}, nil
}

func (f *firstEntryPairFixture) EnterResolvedCurrent(ctx context.Context, resolution workspaceauthoritycmd.DefaultPairResolution, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (workspaceauthoritycmd.ContextEntryResult, error) {
	if f.currentEntryErr != nil {
		f.entryCalls++
		*f.order = append(*f.order, "entry")
		return workspaceauthoritycmd.ContextEntryResult{}, f.currentEntryErr
	}
	return f.EnterResolved(ctx, resolution, session, nil, in, out, errOut)
}

type firstEntryReadinessFixture struct {
	order  *[]string
	calls  int
	err    error
	cancel context.CancelFunc
}

func (f *firstEntryReadinessFixture) Check(context.Context) error {
	f.calls++
	*f.order = append(*f.order, "readiness")
	if f.cancel != nil {
		f.cancel()
	}
	return f.err
}

type firstEntryClusterFixture struct {
	order  *[]string
	calls  int
	cancel context.CancelFunc
	err    error
}

func (f *firstEntryClusterFixture) Reconcile(_ context.Context, intent operation.Intent) (workspaceauthoritycmd.FinalClusterReconciliation, error) {
	f.calls++
	*f.order = append(*f.order, "cluster")
	if f.cancel != nil {
		f.cancel()
	}
	if intent.Command != "cluster up" || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.ClusterTargetKind || intent.Target.ParentID != tobari.ClusterTargetID {
		panic("root cluster call did not consume the canonical Catalog contract")
	}
	return workspaceauthoritycmd.FinalClusterReconciliation{}, f.err
}

type firstEntryReviewerFixture struct {
	order  *[]string
	action recommendedFirstUseAction
	calls  int
}

type firstEntrySetupFixture struct {
	order  *[]string
	choice firstUseSetupChoice
}

type configuratorSubmissionReviewerFixture struct {
	order  *[]string
	action configuratorSubmissionAction
}

func (f configuratorSubmissionReviewerFixture) Review(context.Context, tobari.ConfiguratorSeed, tobari.ConfiguratorSubmission, io.Reader, io.Writer) (configuratorSubmissionAction, error) {
	*f.order = append(*f.order, "submission review")
	return f.action, nil
}

func (f firstEntrySetupFixture) Choose(context.Context, tobari.ConfiguratorSeed, io.Reader, io.Writer) (firstUseSetupChoice, error) {
	*f.order = append(*f.order, "setup")
	return f.choice, nil
}

type firstEntryConfiguratorDraftFixture struct {
	order          *[]string
	draft          tobari.ConfiguratorDraft
	body           tobari.WorkspaceTemplateBody
	pending        *tobari.ConfiguratorSubmission
	taskSubmission tobari.ConfiguratorSubmission
	taskFrozen     bool
	taskConfirmed  bool
}

func (f firstEntryConfiguratorDraftFixture) Reserve(context.Context, tobari.ConfiguratorSeed, tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error) {
	*f.order = append(*f.order, "draft reserve")
	return f.draft, nil
}

func (f firstEntryConfiguratorDraftFixture) Materialize(context.Context, tobari.ConfiguratorDraft) error {
	*f.order = append(*f.order, "draft materialize")
	return nil
}

func (f firstEntryConfiguratorDraftFixture) Freeze(_ context.Context, draft tobari.ConfiguratorDraft) (tobari.ConfiguratorSubmission, error) {
	*f.order = append(*f.order, "draft freeze")
	return tobari.NewConfiguratorSubmission(draft, f.body)
}

func (f firstEntryConfiguratorDraftFixture) CompleteTask(context.Context, tobari.ConfiguratorSubmission) error {
	*f.order = append(*f.order, "task settle")
	return nil
}
func (f firstEntryConfiguratorDraftFixture) PendingTask(context.Context, string, tobari.ConfiguratorTask, string) (tobari.ConfiguratorDraft, tobari.ConfiguratorSubmission, bool, bool, error) {
	if f.taskSubmission.Draft.ID != "" || f.draft.ID != "" && (f.taskFrozen || f.taskConfirmed) {
		return f.draft, f.taskSubmission, f.taskFrozen, f.taskConfirmed, nil
	}
	return tobari.ConfiguratorDraft{}, tobari.ConfiguratorSubmission{}, false, false, nil
}
func (f firstEntryConfiguratorDraftFixture) ConfirmTask(context.Context, tobari.ConfiguratorSubmission) error {
	if f.order != nil {
		*f.order = append(*f.order, "task confirm")
	}
	return nil
}
func (f firstEntryConfiguratorDraftFixture) RetireUnmaterializedTask(context.Context, tobari.ConfiguratorDraft) error {
	return nil
}

func (f firstEntryConfiguratorDraftFixture) ArmHomeAdoption(context.Context, tobari.ConfiguratorSubmission) error {
	*f.order = append(*f.order, "home arm")
	return nil
}

func (f firstEntryConfiguratorDraftFixture) PendingHomeAdoption(context.Context, string) (tobari.ConfiguratorSubmission, bool, error) {
	if f.pending != nil {
		return *f.pending, true, nil
	}
	return tobari.ConfiguratorSubmission{}, false, nil
}

func (f firstEntryConfiguratorDraftFixture) AdoptHome(_ context.Context, _ tobari.ConfiguratorSubmission, _ tobari.ContextAuthoritySnapshot, settle ...func() error) error {
	*f.order = append(*f.order, "home adopt")
	if len(settle) > 0 && settle[0] != nil {
		return settle[0]()
	}
	return nil
}

type firstEntryConfiguratorRunnerFixture struct {
	order  *[]string
	runErr error
}

func (f firstEntryConfiguratorRunnerFixture) PrepareConfiguratorRuntime(_ context.Context, binding tobari.RuntimeBinding) error {
	*f.order = append(*f.order, "runtime prepare")
	return binding.Validate()
}
func (f firstEntryConfiguratorRunnerFixture) AcquireConfiguratorAuthorAttachment(context.Context, tobari.ConfiguratorDraft) (func() error, error) {
	return func() error { return nil }, nil
}
func (f firstEntryConfiguratorRunnerFixture) AcquireConfiguratorPublicationAttachment(context.Context, tobari.ConfiguratorSubmission) (func() error, error) {
	return func() error { return nil }, nil
}
func (f firstEntryConfiguratorRunnerFixture) ApplyConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource, io.Writer) (tobari.RuntimeBinding, error) {
	return tobari.RuntimeBinding{}, nil
}

func (f firstEntryConfiguratorRunnerFixture) RunConfigurator(_ context.Context, _ tobari.ConfiguratorDraft, isolation tobari.ConfiguratorIsolation, _ io.Reader, _ io.Writer) error {
	*f.order = append(*f.order, "agent")
	if f.runErr != nil {
		return f.runErr
	}
	return isolation.Validate()
}

func (f *firstEntryReviewerFixture) Review(context.Context, tobari.RecommendedFirstUseDraft, io.Reader, io.Writer) (recommendedFirstUseAction, error) {
	f.calls++
	*f.order = append(*f.order, "review")
	return f.action, nil
}

func newFirstEntryCLI(t *testing.T, fresh, interactive bool, action recommendedFirstUseAction) (*CLI, *firstEntryPairFixture, *firstEntryReadinessFixture, *firstEntryClusterFixture, *firstEntryReviewerFixture, *bytes.Buffer, *bytes.Buffer, *[]string) {
	t.Helper()
	order := []string{}
	observation := tobari.FinalDefaultPairObservation{SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, ProjectRoot: "/workspace/example", CollectionPresent: !fresh}
	pair := &firstEntryPairFixture{order: &order, observation: observation, outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}
	readiness := &firstEntryReadinessFixture{order: &order}
	cluster := &firstEntryClusterFixture{order: &order}
	reviewer := &firstEntryReviewerFixture{order: &order, action: action}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.processLifetime = context.WithValue(context.Background(), firstEntryProcessLifetimeMarker{}, "injected")
	command.finalDefaultPair = pair
	if !fresh {
		command.finalContexts = workspaceauthoritycmd.NewContextService(&currentContextSelectionFixture{snapshot: finalCurrentContextEntrySnapshotFixture(t)})
	}
	command.finalEntryReadiness = readiness
	command.finalCluster = cluster
	command.firstUse = reviewer
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return interactive }
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) {
		order = append(order, "template body")
		return finalAxisTemplateBody("/standard"), nil
	}
	command.firstUseCustomize = func(context.Context, tobari.RecommendedFirstUseDraft) (tobari.WorkspaceTemplateBody, error) {
		order = append(order, "customize")
		return finalAxisTemplateBody("/customized"), nil
	}
	return command, pair, readiness, cluster, reviewer, &stdout, &stderr, &order
}

func TestFinalRootFreshStartComposesReviewFiveCheckpointsAndExactDirectArgv(t *testing.T) {
	command, pair, readiness, cluster, reviewer, stdout, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
	argv := []string{"claude", "--model", "", "--flag=-value"}
	if code := command.RunContext(context.Background(), append([]string{"--"}, argv...)); code != ExitOK {
		t.Fatalf("root exit=%d order=%v stdout=%q stderr=%q", code, *order, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "readiness", "review", "template body", "resolve", "cluster", "refresh", "entry"}) {
		t.Fatalf("root order = %v", *order)
	}
	if reviewer.calls != 1 || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 1 || pair.refreshCalls != 1 || pair.entryCalls != 1 || pair.resolveBody == nil {
		t.Fatalf("root calls reviewer=%d readiness=%d cluster=%d resolve=%d refresh=%d entry=%d body=%v", reviewer.calls, readiness.calls, cluster.calls, pair.resolveCalls, pair.refreshCalls, pair.entryCalls, pair.resolveBody)
	}
	if !pair.session.Direct() || !reflect.DeepEqual(pair.session.Argv(), argv) {
		t.Fatalf("direct argv = %#v", pair.session.Argv())
	}
	wantProgress := "Tobari · Starting Workspace\n\n✓ Check requirements\n✓ Save setup\n✓ Prepare protection\n✓ Prepare Workspace\n✓ Enter Workspace\n"
	if stderr.String() != wantProgress || stdout.Len() != 0 {
		t.Fatalf("root streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootJSONReadinessFailureOwnsCompleteStderr(t *testing.T) {
	command, _, readiness, cluster, reviewer, stdout, stderr, _ := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
	readiness.err = fault.WithClassification(fault.New(
		fault.KindUnavailable,
		"docker_engine_unavailable",
		"The selected Docker engine is unavailable.",
		false,
		fault.NextAction{Command: "doctor", Reason: "Start the selected engine externally."},
	), fault.PhasePrecondition, fault.ChangeNone)
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "--", "agent", "--monkey-unknown=1"}); code != ExitUnavailable {
		t.Fatalf("root exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), `"code":"docker_engine_unavailable"`) ||
		strings.Contains(stderr.String(), "Tobari ·") || strings.Contains(stderr.String(), "Check requirements") {
		t.Fatalf("JSON readiness stderr=%q", stderr.String())
	}
	if stdout.Len() != 0 || readiness.calls != 1 || cluster.calls != 0 || reviewer.calls != 0 {
		t.Fatalf("stdout=%q readiness=%d cluster=%d review=%d", stdout.String(), readiness.calls, cluster.calls, reviewer.calls)
	}
}

func TestFinalRootFreshIgnoresRetiredAgentSetupSelector(t *testing.T) {
	command, pair, _, _, _, _, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
	body := finalAxisTemplateBody("/agent-draft")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	command.firstUseSetup = firstEntrySetupFixture{order: order, choice: firstUseSetupCodex}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) {
		*order = append(*order, "template body")
		return body, nil
	}
	runner := firstEntryConfiguratorRunnerFixture{order: order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: order},
		runner,
	)
	command.configuratorReview = configuratorSubmissionReviewerFixture{order: order, action: configuratorSubmissionApply}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(draft.TemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: draft.TemplateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pair.resolution.Observation.Context = &tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: draft.TemplateID}, Template: template, PolicyMemory: memory}
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("agent-first root exit=%d stderr=%q", code, stderr.String())
	}
	wantOrder := []string{"observe", "readiness", "review", "template body", "resolve", "cluster", "refresh", "entry"}
	if !reflect.DeepEqual(*order, wantOrder) {
		t.Fatalf("manual first-use order=%v want=%v", *order, wantOrder)
	}
	if pair.resolveBody == nil || !reflect.DeepEqual(*pair.resolveBody, body) {
		t.Fatalf("resolved body=%+v want reviewed manual body", pair.resolveBody)
	}
	if strings.Contains(stderr.String(), "Configurator") || slices.Contains(*order, "setup") || slices.Contains(*order, "agent") {
		t.Fatalf("fresh root reached retired agent setup: order=%v stderr=%q", *order, stderr.String())
	}
}

func TestFinalRootDoesNotRunRetiredConfiguratorCleanupPath(t *testing.T) {
	command, _, _, _, _, _, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
	body := finalAxisTemplateBody("/agent-draft")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	command.firstUseSetup = firstEntrySetupFixture{order: order, choice: firstUseSetupCodex}
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) {
		*order = append(*order, "template body")
		return body, nil
	}
	runner := firstEntryConfiguratorRunnerFixture{order: order, runErr: errors.Join(tobari.ErrNativeLoginBridgeUnavailable, tobari.ErrConfiguratorTransientCleanupUnknown)}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: order},
		runner,
	)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("manual root exit=%d order=%v stderr=%q", code, *order, stderr.String())
	}
	if strings.Contains(stderr.String(), "bounded cleanup could not confirm removal") || slices.Contains(*order, "agent") || slices.Contains(*order, "draft freeze") {
		t.Fatalf("retired Configurator affected fresh root: order=%v stderr=%q", *order, stderr.String())
	}
}

func TestFinalRootExistingPairSkipsReviewAndFreshBody(t *testing.T) {
	command, pair, _, _, reviewer, _, stderr, order := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("existing root exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "readiness", "resolve", "cluster", "refresh", "entry"}) || reviewer.calls != 0 || pair.resolveBody != nil {
		t.Fatalf("existing root order=%v review=%d body=%v", *order, reviewer.calls, pair.resolveBody)
	}
	if !strings.Contains(stderr.String(), "✓ Use Context\n") || strings.Contains(stderr.String(), "Save setup") {
		t.Fatalf("existing progress = %q", stderr.String())
	}
}

func TestFinalRootResumesExactPendingContextEntryBeforeClusterMutation(t *testing.T) {
	command, pair, _, cluster, _, _, stderr, order := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ad")
	contextRef, err := tobari.ContextRef(contextID)
	if err != nil {
		t.Fatal(err)
	}
	pair.resolution.Observation.Context = &tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{ID: contextID}}
	pair.recovery = tobari.FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "context-entry", Target: contextRef}
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("resume exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "readiness", "resolve", "entry"}) || pair.entryCalls != 1 || cluster.calls != 0 {
		t.Fatalf("resume order=%v entry=%d cluster=%d", *order, pair.entryCalls, cluster.calls)
	}
}

func TestFinalRootCustomizePublishesOnlyTheReviewedBody(t *testing.T) {
	command, pair, _, _, _, _, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseCustomize)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("customized root exit=%d order=%v stderr=%q", code, *order, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "readiness", "review", "customize", "resolve", "cluster", "refresh", "entry"}) || pair.resolveBody == nil {
		t.Fatalf("customized root order=%v body=%v", *order, pair.resolveBody)
	}
}

func TestFinalRootFreshNoninteractiveAndCancelMakeZeroReadinessOrMutation(t *testing.T) {
	for _, test := range []struct {
		name        string
		interactive bool
		action      recommendedFirstUseAction
		want        int
	}{
		{name: "redirected", interactive: false, action: recommendedFirstUseStart, want: ExitRejected},
		{name: "cancel", interactive: true, action: recommendedFirstUseCancel, want: ExitCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			command, pair, readiness, cluster, reviewer, stdout, stderr, _ := newFirstEntryCLI(t, true, test.interactive, test.action)
			if code := command.RunContext(context.Background(), nil); code != test.want {
				t.Fatalf("exit=%d want=%d stderr=%q", code, test.want, stderr.String())
			}
			wantReadiness := 0
			if test.interactive {
				wantReadiness = 1
			}
			if readiness.calls != wantReadiness || cluster.calls != 0 || pair.resolveCalls != 0 || pair.entryCalls != 0 || stdout.Len() != 0 {
				t.Fatalf("redirected/cancel crossed setup: readiness=%d cluster=%d resolve=%d entry=%d stdout=%q", readiness.calls, cluster.calls, pair.resolveCalls, pair.entryCalls, stdout.String())
			}
			if test.interactive && reviewer.calls != 1 || !test.interactive && reviewer.calls != 0 {
				t.Fatalf("review calls = %d", reviewer.calls)
			}
		})
	}
}

func TestFinalRootManualChoiceCancellationDoesNotPrepareConfiguratorRuntime(t *testing.T) {
	command, _, _, _, _, _, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseCancel)
	body := finalAxisTemplateBody("/standard")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	command.firstUseSetup = firstEntrySetupFixture{order: order, choice: firstUseSetupManual}
	runner := firstEntryConfiguratorRunnerFixture{order: order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: order},
		runner,
	)
	if code := command.RunContext(context.Background(), nil); code != ExitCanceled {
		t.Fatalf("manual cancellation exit=%d stderr=%q", code, stderr.String())
	}
	if slices.Contains(*order, "runtime prepare") {
		t.Fatalf("manual cancellation prepared Configurator Runtime: %v", *order)
	}
}

func TestFinalRootCancellationNeverReachesRetiredConfiguratorBoundary(t *testing.T) {
	command, _, _, _, _, _, _, order := newFirstEntryCLI(t, true, true, recommendedFirstUseCancel)
	body := finalAxisTemplateBody("/standard")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	command.Err = &failingConfiguratorEntryWriter{}
	command.firstUseSetup = firstEntrySetupFixture{order: order, choice: firstUseSetupCodex}
	runner := firstEntryConfiguratorRunnerFixture{order: order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: order, draft: draft, body: body}, runner,
		configuratorStageFixture{order: order}, runner,
	)
	if code := command.RunContext(context.Background(), nil); code != ExitCanceled {
		t.Fatalf("root exit=%d order=%v", code, *order)
	}
	if slices.Contains(*order, "runtime prepare") || slices.Contains(*order, "draft reserve") || slices.Contains(*order, "agent") {
		t.Fatalf("root entry boundary failure crossed Runtime/draft mutation: %v", *order)
	}
}

func TestPreparedConfiguratorRuntimeIgnoresPostSuccessStatusWriteFailure(t *testing.T) {
	order := []string{}
	body := finalAxisTemplateBody("/standard")
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	command := newCLI(strings.NewReader(""), io.Discard, failingConfiguratorStatusWriter{}, DefaultCatalog(), nil)
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	command.configurator = configuratorcmd.New(
		firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: body},
		runner,
		configuratorStageFixture{order: &order},
		runner,
	)
	if err := prepareConfiguratorRuntime(context.Background(), command, seed); err != nil {
		t.Fatalf("confirmed Runtime was reversed by status output: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"runtime prepare"}) {
		t.Fatalf("Runtime prepare calls=%v", order)
	}
}

func TestFinalRootCallerCancellationBeforeHandoffExits130(t *testing.T) {
	command, pair, readiness, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	readiness.cancel = cancel
	readiness.err = context.Canceled
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("canceled root exit=%d stderr=%q", code, stderr.String())
	}
	if pair.resolveCalls != 0 || cluster.calls != 0 || pair.entryCalls != 0 {
		t.Fatalf("canceled root crossed mutation: resolve=%d cluster=%d entry=%d", pair.resolveCalls, cluster.calls, pair.entryCalls)
	}
}

func TestFinalRootCallerCancellationAtEachCanonicalPreHandoffBoundaryExits130(t *testing.T) {
	for _, boundary := range []string{"resolve", "cluster", "refresh", "entry"} {
		t.Run(boundary, func(t *testing.T) {
			command, pair, _, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch boundary {
			case "resolve":
				pair.cancelAt, pair.cancel, pair.resolveErr = boundary, cancel, context.Canceled
			case "cluster":
				cluster.cancel, cluster.err = cancel, context.Canceled
			case "refresh":
				pair.cancelAt, pair.cancel, pair.refreshErr = boundary, cancel, context.Canceled
			case "entry":
				pair.cancelAt, pair.cancel, pair.entryErr = boundary, cancel, context.Canceled
			}
			if code := command.RunContext(ctx, nil); code != ExitInterrupted {
				t.Fatalf("%s cancellation exit=%d stderr=%q", boundary, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "operation_canceled") || !strings.Contains(stderr.String(), "Canceled") {
				t.Fatalf("%s cancellation classification=%q", boundary, stderr.String())
			}
		})
	}
}

func TestFinalRootCallerCancellationPreservesUnknownMutationClassificationAndExits130(t *testing.T) {
	command, _, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster.cancel = cancel
	cluster.err = fault.WithClassification(
		fault.New(fault.KindContract, "unclassified_mutation_outcome", "synthetic unknown cluster outcome", false),
		fault.PhaseMutation, fault.ChangeUnknown,
	)
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("unknown cancellation exit=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unclassified_mutation_outcome") || !humanOutputHasRow(stderr.String(), "Change state", "unknown") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("unknown cancellation stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootLateCancellationAfterResolutionPreservesConfirmedAuthorityAndStops(t *testing.T) {
	command, pair, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pair.cancelAt, pair.cancel = "resolve", cancel
	pair.resolution.AuthorityChanged = true
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("late resolution cancellation exit=%d stderr=%q", code, stderr.String())
	}
	if cluster.calls != 0 || pair.refreshCalls != 0 || pair.entryCalls != 0 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "default_pair_initialized") || !humanOutputHasRow(stderr.String(), "Change state", "partial") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("late resolution cancellation cluster=%d refresh=%d entry=%d stdout=%q stderr=%q", cluster.calls, pair.refreshCalls, pair.entryCalls, stdout.String(), stderr.String())
	}
}

func TestFinalRootLateCancellationAfterClusterUsesBoundedClassificationAndStopsBeforeEntry(t *testing.T) {
	command, pair, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster.cancel = cancel
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("late cluster cancellation exit=%d stderr=%q", code, stderr.String())
	}
	var deadline time.Time
	var hasDeadline bool
	var settlementParent any
	if pair.refreshContext != nil {
		deadline, hasDeadline = pair.refreshContext.Deadline()
		settlementParent = pair.refreshContext.Value(firstEntryProcessLifetimeMarker{})
	}
	remaining := time.Until(deadline)
	if pair.refreshCalls != 1 || pair.refreshCtxErr != nil || pair.entryCalls != 0 || stdout.Len() != 0 ||
		!hasDeadline || remaining <= 0 || remaining > firstEntryClassificationTimeout || settlementParent != "injected" ||
		!strings.Contains(stderr.String(), "default_pair_initialized") || !humanOutputHasRow(stderr.String(), "Change state", "partial") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("late cluster cancellation refresh=%d refresh_ctx=%v deadline=%v has_deadline=%v remaining=%v parent=%v entry=%d stdout=%q stderr=%q", pair.refreshCalls, pair.refreshCtxErr, deadline, hasDeadline, remaining, settlementParent, pair.entryCalls, stdout.String(), stderr.String())
	}
}

func TestFinalRootPreservesCanonicalClusterRecoveryFault(t *testing.T) {
	command, pair, _, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	cluster.err = fault.New(fault.KindUnavailable, "cluster_reconcile_interrupted", "synthetic interrupted cluster reconcile", false)
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("cluster fault exit=%d stderr=%q", code, stderr.String())
	}
	if pair.entryCalls != 0 || !strings.Contains(stderr.String(), "cluster_reconcile_interrupted") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("cluster recovery was not preserved: entry=%d stderr=%q", pair.entryCalls, stderr.String())
	}
}

func TestFinalRootPreservesCanonicalWorkspaceEntryRecoveryFault(t *testing.T) {
	command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	pair.entryErr = fault.WithClassification(
		fault.New(fault.KindUnavailable, "workspace_entry_interrupted", "synthetic interrupted Workspace entry", false),
		fault.PhaseMutation, fault.ChangePartial,
	)
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("entry fault exit=%d stderr=%q", code, stderr.String())
	}
	spec, found := command.catalog.Lookup("status")
	if !found {
		t.Fatal("status recovery command is absent from the Catalog")
	}
	expectedInvocation := recoveryCommandForProgram(spec.programName(), spec.Path)
	if !strings.Contains(stderr.String(), "workspace_entry_interrupted") || !evaluatorNextHasInvocation(stderr.String(), expectedInvocation) || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("entry recovery was not preserved: stderr=%q", stderr.String())
	}
}

func TestFinalRootPreservesChildStatusAndReportsSecondaryCleanup(t *testing.T) {
	command, pair, _, _, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	pair.outcome = tobari.WorkspaceSessionOutcome{ExitCode: 23, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{tobari.WorkspaceCleanupHostLoopback}}
	if code := command.RunContext(context.Background(), []string{"--", "claude"}); code != 23 {
		t.Fatalf("child exit=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || strings.Count(stderr.String(), "Tobari cleanup needs attention") != 1 || !strings.Contains(stderr.String(), "Next: tobari status") || strings.Contains(stderr.String(), "Command failed") {
		t.Fatalf("child boundary stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootShowsWorkspaceOwnedCredentialHintOnlyForFreshShell(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		fresh          bool
		freshWorkspace bool
		want           int
	}{
		{name: "fresh shell", fresh: true, freshWorkspace: true, want: 1},
		{name: "fresh direct command", args: []string{"--", "claude"}, fresh: true, freshWorkspace: true, want: 0},
		{name: "existing Workspace shell", fresh: false, freshWorkspace: false, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, tt.fresh, true, recommendedFirstUseStart)
			pair.resolution.Observation.Context = &tobari.ContextAuthoritySnapshot{}
			if !tt.freshWorkspace {
				pair.resolution.Observation.Context.Workspace = &tobari.WorkspaceBinding{}
			}
			if code := command.RunContext(context.Background(), tt.args); code != ExitOK {
				t.Fatalf("entry exit=%d stderr=%q", code, stderr.String())
			}
			if count := strings.Count(stderr.String(), "Credentials stay in this Workspace; sign in with the tool when needed."); count != tt.want {
				t.Fatalf("credential guidance count=%d want=%d stderr=%q", count, tt.want, stderr.String())
			}
		})
	}
}
