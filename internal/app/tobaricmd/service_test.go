package tobaricmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeRuntime struct {
	state               tobari.State
	clusterCalls        int
	loadStateCalls      int
	inspectCalls        int
	configured          *bool
	clusterReady        *bool
	inspectErr          error
	buildIdentityErr    error
	learnedCalls        int
	denyCalls           int
	decisionSetCalls    int
	decisionSetRevision string
	activationReceipt   tobari.PolicyActivationReceipt
	denials             []tobari.PolicyDenial
	rules               []tobari.LearnedPolicyRule
	denyRules           []tobari.PolicyDenyRule
}

func (f *fakeRuntime) CurrentDirectory(context.Context) (string, error) {
	return filepath.Join(filepath.Dir(f.state.RuntimeDirectory), "work"), nil
}
func (f *fakeRuntime) IsTerminal(io.Writer) bool { return false }

func (f *fakeRuntime) ValidateClusterBuildIdentity(context.Context) error {
	return f.buildIdentityErr
}
func (f *fakeRuntime) ClusterUp(context.Context) (tobari.State, error) {
	f.clusterCalls++
	return f.state, nil
}
func (f *fakeRuntime) ClusterUpWithProgress(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) (tobari.State, error) {
	if progress != nil {
		progress(tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressStarted,
		})
		progress(tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressCompleted,
		})
	}
	return f.ClusterUp(ctx)
}
func (f *fakeRuntime) LoadState(context.Context) (tobari.State, bool, error) {
	f.loadStateCalls++
	if f.configured != nil {
		return f.state, *f.configured, nil
	}
	return f.state, true, nil
}
func (f *fakeRuntime) InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error) {
	f.inspectCalls++
	if f.inspectErr != nil {
		return tobari.ClusterStatus{}, f.inspectErr
	}
	running := true
	if f.clusterReady != nil {
		running = *f.clusterReady
	}
	return tobari.ClusterStatus{
		Configured: true, Running: running,
		Policy: f.state.PolicyDirectory, TobariCount: 0,
		ContextCount: f.state.ContextCount, PolicyRevision: f.state.AggregateRevision,
		PolicyProjection: "valid", PrincipalRegistry: "valid", CredentialProjection: "valid",
		AuthProviderProjection: "valid", AuthBrokerState: "ready", CredentialCompanionState: "ready", RootKeyBackend: "xdg_file",
		Components: []tobari.ComponentStatus{
			{Name: "auth-broker", State: "running", Health: "healthy"},
			{Name: "gateway", State: "running", Health: "healthy"},
			{Name: "opa", State: "running", Health: "healthy"},
		},
	}, nil
}

func TestClusterUpWithProgressKeepsMutationAndStatusContracts(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	var events []tobari.ClusterUpProgress
	status, err := New(runtime).ClusterUpWithProgress(
		context.Background(), createIntent("cluster up"),
		func(event tobari.ClusterUpProgress) { events = append(events, event) },
	)
	if err != nil {
		t.Fatal(err)
	}
	want := []tobari.ClusterUpProgress{
		{Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressStarted},
		{Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressCompleted},
		{Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressStarted},
		{Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressCompleted},
	}
	if len(events) != len(want) {
		t.Fatalf("progress events = %+v, want %+v", events, want)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("progress event %d = %+v, want %+v", index, events[index], want[index])
		}
	}
	if status.Task != tobari.TaskClusterUp || !status.Running || runtime.clusterCalls != 1 || runtime.inspectCalls != 1 {
		t.Fatalf("status=%+v cluster calls=%d inspect calls=%d", status, runtime.clusterCalls, runtime.inspectCalls)
	}
}

func TestClusterUpRejectsBuildIdentityBeforeLifecycleMutation(t *testing.T) {
	t.Parallel()
	identityErr := fault.New(
		fault.KindContract, "runtime_image_api_mismatch", "source and selected APIs differ", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect build identity."},
	)
	runtime := &fakeRuntime{buildIdentityErr: identityErr}
	_, err := New(runtime).ClusterUp(context.Background(), operation.Intent{
		Command: "cluster up", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "runtime_image_api_mismatch" || runtime.clusterCalls != 0 || runtime.inspectCalls != 0 {
		t.Fatalf("fault=%#v cluster_calls=%d inspect_calls=%d", public, runtime.clusterCalls, runtime.inspectCalls)
	}
}

func TestClusterUpWithProgressMarksStatusFailure(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir()), inspectErr: context.Canceled}
	var events []tobari.ClusterUpProgress
	_, err := New(runtime).ClusterUpWithProgress(
		context.Background(), createIntent("cluster up"),
		func(event tobari.ClusterUpProgress) { events = append(events, event) },
	)
	if err == nil || len(events) != 4 || events[2].Status != tobari.ClusterUpProgressStarted || events[3].Status != tobari.ClusterUpProgressFailed {
		t.Fatalf("err=%v events=%+v", err, events)
	}
}

func (f *fakeRuntime) ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error) {
	return []byte("cluster\n"), nil
}
func (f *fakeRuntime) ClusterDenials(context.Context, tobari.State, int) ([]tobari.PolicyDenial, error) {
	if f.denials == nil {
		return []tobari.PolicyDenial{}, nil
	}
	return append([]tobari.PolicyDenial{}, f.denials...), nil
}
func (f *fakeRuntime) ReadLearnedPolicyRules(
	context.Context, tobari.State,
) ([]tobari.LearnedPolicyRule, error) {
	if f.rules == nil {
		return []tobari.LearnedPolicyRule{}, nil
	}
	return append([]tobari.LearnedPolicyRule{}, f.rules...), nil
}
func (f *fakeRuntime) ReadPolicyDenyRules(context.Context, tobari.State) (tobari.PolicyDenyRuleSet, error) {
	return tobari.PolicyDenyRuleSet{
		Baseline: []tobari.PolicyBaselineDenyRule{}, Exact: append([]tobari.PolicyDenyRule{}, f.denyRules...),
	}, nil
}
func (f *fakeRuntime) ApplyLearnedPolicyRules(
	_ context.Context, _ tobari.State, _, updated []tobari.LearnedPolicyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.learnedCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updated...)
	return f.policyActivationReceipt(), nil
}
func (f *fakeRuntime) ApplyPolicyDenyRules(
	_ context.Context, _ tobari.State, _ []tobari.LearnedPolicyRule,
	_ []tobari.PolicyDenyRule, updated []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.denyCalls++
	f.denyRules = append([]tobari.PolicyDenyRule{}, updated...)
	return f.policyActivationReceipt(), nil
}
func (f *fakeRuntime) ApplyPolicyDecisionSet(
	_ context.Context, _ tobari.State,
	_ []tobari.LearnedPolicyRule, updatedAllows []tobari.LearnedPolicyRule,
	_ []tobari.PolicyDenyRule, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	f.decisionSetCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updatedAllows...)
	f.denyRules = append([]tobari.PolicyDenyRule{}, updatedDenies...)
	if f.decisionSetRevision == "" {
		f.decisionSetRevision = strings.Repeat("b", 64)
	}
	receipt := f.policyActivationReceipt()
	receipt.ActiveRevision = f.decisionSetRevision
	return receipt, nil
}

func (f *fakeRuntime) policyActivationReceipt() tobari.PolicyActivationReceipt {
	if f.activationReceipt.PolicyDirectory == "" {
		f.activationReceipt.PolicyDirectory = filepath.Join(filepath.Dir(f.state.PolicyDirectory), "confirmed", "policy")
	}
	if f.activationReceipt.ActiveRevision == "" {
		f.activationReceipt.ActiveRevision = strings.Repeat("c", 64)
	}
	return f.activationReceipt
}

func (f *fakeRuntime) ClusterDown(context.Context, tobari.State, bool) error { return nil }
func (f *fakeRuntime) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	return action(ctx)
}
func (f *fakeRuntime) Doctor(context.Context, string) (doctor.Report, error) {
	return doctor.Report{Checks: []doctor.Check{{Name: "docker", Status: doctor.CheckStatusPass, Detail: "available"}}}, nil
}

type activeContextFake struct {
	*fakeRuntime
	active string
}

func (f *activeContextFake) ActiveContextName(context.Context) (string, error) {
	return f.active, nil
}

type projectRuntimeFake struct {
	*fakeRuntime
	cwd             string
	terminal        bool
	inside          bool
	project         tobari.ProjectInstance
	created         tobari.ProjectInstance
	resolved        tobari.ProjectInstance
	projects        []tobari.ProjectInstance
	found           bool
	resolveCalls    int
	createCalls     int
	ensureCalls     int
	validateCalls   int
	validateErr     error
	enterCalls      int
	deleteCalls     int
	sessionCalls    int
	sessionAttached bool
	sessionErr      error
	cwdCalls        int
	listCalls       int
	runtimeCalls    int
	observeErr      error
}

func (f *projectRuntimeFake) CurrentDirectory(context.Context) (string, error) {
	f.cwdCalls++
	return f.cwd, nil
}
func (f *projectRuntimeFake) IsTerminal(io.Writer) bool          { return f.terminal }
func (f *projectRuntimeFake) IsInputTerminal(io.Reader) bool     { return f.terminal }
func (f *projectRuntimeFake) InsideProject(context.Context) bool { return f.inside }
func (f *projectRuntimeFake) ResolveProject(context.Context, string) (tobari.ProjectInstance, bool, error) {
	f.resolveCalls++
	if f.resolved.ID != "" {
		return f.resolved, f.found, nil
	}
	return f.project, f.found, nil
}
func (f *projectRuntimeFake) ObserveContext(_ context.Context, name string) (tobari.ContextObservation, error) {
	if f.observeErr != nil {
		return tobari.ContextObservation{}, f.observeErr
	}
	if name == "missing" {
		return tobari.ContextObservation{}, tobari.ErrContextNotFound
	}
	if name == "" {
		name = tobari.DefaultContextName
	}
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000099",
		Name: name, AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode: tobari.ContextPolicyModeGuided,
	}
	return tobari.ContextObservation{
		State: tobari.ContextObservationPersisted, Name: name, Manifest: &manifest,
	}, nil
}
func (f *projectRuntimeFake) ObserveBoundProject(ctx context.Context, cwd string, _ tobari.ContextManifest) (tobari.ProjectInstance, bool, error) {
	return f.ResolveProject(ctx, cwd)
}
func (f *projectRuntimeFake) CreateProject(context.Context, string) (tobari.ProjectInstance, error) {
	f.createCalls++
	if f.created.ID != "" {
		return f.created, nil
	}
	return f.project, nil
}
func (f *projectRuntimeFake) ListProjects(context.Context) ([]tobari.ProjectInstance, error) {
	f.listCalls++
	if f.projects != nil {
		return append([]tobari.ProjectInstance{}, f.projects...), nil
	}
	if !f.found {
		return []tobari.ProjectInstance{}, nil
	}
	return []tobari.ProjectInstance{f.project}, nil
}
func (f *projectRuntimeFake) ProjectHome(context.Context, tobari.ProjectInstance) (string, error) {
	return "/tmp/tobari-home", nil
}
func (f *projectRuntimeFake) ValidateProjectRuntime(context.Context, tobari.State) error {
	f.validateCalls++
	return f.validateErr
}
func (f *projectRuntimeFake) EnsureProjectRuntime(_ context.Context, _ tobari.State, instance tobari.ProjectInstance) (tobari.ProjectInstance, error) {
	f.ensureCalls++
	return instance, nil
}
func (f *projectRuntimeFake) InspectProjectRuntime(context.Context, tobari.ProjectInstance) (tobari.RuntimeDiagnostic, error) {
	f.runtimeCalls++
	return tobari.RuntimeDiagnosticMissing, nil
}
func (f *projectRuntimeFake) ProjectSessionAttached(context.Context, tobari.ProjectInstance) (bool, error) {
	f.sessionCalls++
	return f.sessionAttached, f.sessionErr
}
func (f *projectRuntimeFake) EnterProjectRuntime(context.Context, tobari.ProjectInstance, tobari.ContextManifest, string, io.Reader, io.Writer, io.Writer) (int, error) {
	f.enterCalls++
	return 0, nil
}
func (f *projectRuntimeFake) DeleteProject(context.Context, tobari.ProjectInstance) error {
	f.deleteCalls++
	return nil
}

type workspaceSelectorFake struct {
	choice tobari.ProjectSelectionChoice
	err    error
	calls  int
	seen   tobari.ProjectSelection
}

func (f *workspaceSelectorFake) Select(_ context.Context, selection tobari.ProjectSelection, _ io.Reader, _ io.Writer) (tobari.ProjectSelectionChoice, error) {
	f.calls++
	f.seen = selection
	if f.err != nil {
		return tobari.ProjectSelectionChoice{}, f.err
	}
	return f.choice, nil
}

func testState(root string) tobari.State {
	return tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: strings.Repeat("a", 64), ContextCount: 1,
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
	}
}

func createIntent(command string) operation.Intent {
	return operation.Intent{
		Command: command, Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
}

func projectCreateIntent(command string) operation.Intent {
	return operation.Intent{
		Command: command, Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ParentID: tobari.CurrentDirectoryTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}
}

func projectWriteIntent(command string) operation.Intent {
	return operation.Intent{
		Command: command, Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.CurrentDirectoryTargetKind, ID: tobari.CurrentDirectoryTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes,
		},
	}
}

func testProjectInstance() tobari.ProjectInstance {
	return tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            "01912345-6789-7abc-8def-0123456789ab",
		Root:          "/tmp/project", Profile: tobari.DefaultProfile,
		ContextID:   "018bcfe5-687b-7000-8000-000000000099",
		ContextName: tobari.DefaultContextName,
		Image:       tobari.BuiltinImageSelector,
	}
}

func TestEnterProjectRejectsNonTTYBeforeCreatingOrReconciling(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project", terminal: false, project: testProjectInstance(),
	}
	_, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err == nil {
		t.Fatal("EnterProject() accepted a non-TTY")
	}
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "tty_required" || fake.clusterCalls != 0 || fake.resolveCalls != 0 {
		t.Fatalf("error=%v public=%+v cluster=%d resolve=%d", err, public, fake.clusterCalls, fake.resolveCalls)
	}
}

func TestEnterProjectAcceptsCurrentDirectoryMutationAndNestedCWD(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project/root", terminal: true, found: true, project: testProjectInstance(),
	}
	selector := &workspaceSelectorFake{choice: tobari.ProjectSelectionChoice{
		Kind: tobari.ProjectSelectionUse, ID: testProjectInstance().ID,
	}}
	code, err := NewWithWorkspaceSelector(fake, selector).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProject() = (%d, %v)", code, err)
	}
	if fake.clusterCalls != 0 || fake.resolveCalls != 1 || fake.ensureCalls != 1 || fake.enterCalls != 1 {
		t.Fatalf("calls = cluster:%d resolve:%d ensure:%d enter:%d", fake.clusterCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
	}
	if selector.calls != 1 || selector.seen.CWD != fake.cwd || len(selector.seen.Candidates) != 1 {
		t.Fatalf("selection = calls:%d value:%+v", selector.calls, selector.seen)
	}
}

func TestEnterProjectWithoutAncestorCreatesDirectly(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project", terminal: true, found: false, project: testProjectInstance(),
	}
	code, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProject() = (%d, %v)", code, err)
	}
	if fake.createCalls != 1 || fake.resolveCalls != 0 || fake.ensureCalls != 1 || fake.enterCalls != 1 {
		t.Fatalf("calls = create:%d resolve:%d ensure:%d enter:%d", fake.createCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectChecksNewRuntimeBeforeCreatingWorkspace(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project", terminal: true, found: false, project: testProjectInstance(),
		validateErr: fault.New(fault.KindUnavailable, "image_not_found", "runtime image is unavailable", false),
	}
	_, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "image_not_found" {
		t.Fatalf("EnterProject() error = %v, public = %+v", err, public)
	}
	if fake.validateCalls != 1 || fake.createCalls != 0 || fake.ensureCalls != 0 || fake.enterCalls != 0 {
		t.Fatalf("calls = validate:%d create:%d ensure:%d enter:%d", fake.validateCalls, fake.createCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectExactCurrentRootReusesDirectly(t *testing.T) {
	t.Parallel()
	project := testProjectInstance()
	project.Root = "/tmp/project"
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project", terminal: true, found: true, project: project,
	}
	code, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProject() = (%d, %v)", code, err)
	}
	if fake.createCalls != 0 || fake.resolveCalls != 1 || fake.ensureCalls != 1 || fake.enterCalls != 1 {
		t.Fatalf("calls = create:%d resolve:%d ensure:%d enter:%d", fake.createCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectRequiresSelectorOnlyForAncestorChoice(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project/root", terminal: true, found: true, project: testProjectInstance(),
	}
	_, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "missing_workspace_selector" {
		t.Fatalf("error=%v public=%+v", err, public)
	}
	if fake.resolveCalls != 0 || fake.createCalls != 0 || fake.ensureCalls != 0 || fake.enterCalls != 0 {
		t.Fatalf("missing selector caused mutation calls: resolve=%d create=%d ensure=%d enter=%d", fake.resolveCalls, fake.createCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectExplicitCreateUsesCurrentDirectoryWithoutNearestReuse(t *testing.T) {
	t.Parallel()
	parent := testProjectInstance()
	created := parent
	created.ID = "01912345-6789-7abc-8def-0123456789ac"
	created.Root = "/tmp/project/root"
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project/root", terminal: true, found: true, project: parent,
		projects: []tobari.ProjectInstance{parent}, created: created,
	}
	selector := &workspaceSelectorFake{choice: tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}}
	code, err := NewWithWorkspaceSelector(fake, selector).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProject() = (%d, %v)", code, err)
	}
	if fake.createCalls != 1 || fake.resolveCalls != 0 || fake.ensureCalls != 1 || fake.enterCalls != 1 {
		t.Fatalf("calls = create:%d resolve:%d ensure:%d enter:%d", fake.createCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
	}
	if selector.calls != 1 || !selector.seen.CanCreate {
		t.Fatalf("selection = calls:%d value:%+v", selector.calls, selector.seen)
	}
}

func TestEnterProjectRejectsStaleWorkspaceChoiceBeforeRuntime(t *testing.T) {
	t.Parallel()
	parent := testProjectInstance()
	changed := parent
	changed.ID = "01912345-6789-7abc-8def-0123456789ac"
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project/root", terminal: true, found: true, project: parent, resolved: changed,
	}
	selector := &workspaceSelectorFake{choice: tobari.ProjectSelectionChoice{
		Kind: tobari.ProjectSelectionUse, ID: parent.ID,
	}}
	_, err := NewWithWorkspaceSelector(fake, selector).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "workspace_selection_stale" {
		t.Fatalf("error=%v public=%+v", err, public)
	}
	if fake.ensureCalls != 0 || fake.enterCalls != 0 || fake.createCalls != 0 {
		t.Fatalf("stale choice caused calls: create=%d ensure=%d enter=%d", fake.createCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectCancellationBeforeMutation(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project/root", terminal: true, found: true, project: testProjectInstance(),
	}
	selector := &workspaceSelectorFake{err: context.Canceled}
	_, err := NewWithWorkspaceSelector(fake, selector).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EnterProject() error = %v, want context.Canceled", err)
	}
	if fake.ensureCalls != 0 || fake.enterCalls != 0 || fake.createCalls != 0 || fake.resolveCalls != 0 {
		t.Fatalf("cancelled selection caused calls: resolve=%d create=%d ensure=%d enter=%d", fake.resolveCalls, fake.createCalls, fake.ensureCalls, fake.enterCalls)
	}
}

func TestEnterProjectRequiresReadyConfiguredClusterBeforeProjectResolution(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		configured bool
		ready      bool
		wantCode   string
	}{
		"unconfigured": {configured: false, ready: false, wantCode: "cluster_not_configured"},
		"unready":      {configured: true, ready: false, wantCode: "cluster_not_ready"},
	} {
		name, test := name, test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fake := &projectRuntimeFake{
				fakeRuntime: &fakeRuntime{
					state:        testState(t.TempDir()),
					configured:   &test.configured,
					clusterReady: &test.ready,
				},
				cwd: "/tmp/project", terminal: true, project: testProjectInstance(),
			}
			_, err := New(fake).EnterProject(
				context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
			)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.wantCode || fake.resolveCalls != 0 || fake.ensureCalls != 0 || fake.enterCalls != 0 || fake.clusterCalls != 0 {
				t.Fatalf("error=%v public=%+v calls: load=%d inspect=%d cluster=%d resolve=%d ensure=%d enter=%d", err, public, fake.loadStateCalls, fake.inspectCalls, fake.clusterCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
			}
		})
	}
}

func TestProjectStatusPreservesExistsWhenRuntimeIsMissing(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
		cwd:         "/tmp/project", found: true, project: testProjectInstance(),
	}
	result, err := New(fake).ProjectStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Exists || result.Runtime != tobari.RuntimeDiagnosticMissing || result.Attachment != tobari.AttachmentDetached ||
		fake.resolveCalls != 1 || fake.sessionCalls != 1 {
		t.Fatalf("status=%+v calls=%d", result, fake.resolveCalls)
	}
}

func TestProjectStatusDoesNotMisclassifyUnsafeContextObservationAsNotFound(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", observeErr: errors.New("Context manifest is unsafe"),
	}
	_, err := New(fake).ProjectStatus(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInternal || public.Code != "context_read_failed" {
		t.Fatalf("unsafe Context observation = %v, public=%+v", err, public)
	}
	if fake.cwdCalls != 0 || fake.resolveCalls != 0 || fake.runtimeCalls != 0 {
		t.Fatalf("unsafe Context observation crossed later read boundaries: %+v", fake)
	}
}

func TestLifecycleUnknownContextFailsBeforeCWDWorkspaceOrDockerObservation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "status", run: func(service *Service) error {
			_, err := service.ProjectStatusInContext(context.Background(), "missing")
			return err
		}},
		{name: "delete", run: func(service *Service) error {
			_, err := service.DeleteProjectInContext(context.Background(), projectWriteIntent("delete"), "missing", false)
			return err
		}},
		{name: "enter", run: func(service *Service) error {
			_, err := service.EnterProjectInContext(
				context.Background(), projectCreateIntent("tobari"), "missing",
				bytes.NewReader(nil), io.Discard, io.Discard,
			)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &projectRuntimeFake{
				fakeRuntime: &fakeRuntime{state: testState(t.TempDir())},
				cwd:         "/tmp/project", terminal: true, found: true, project: testProjectInstance(),
			}
			err := test.run(New(fake))
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindNotFound || public.Code != "context_not_found" {
				t.Fatalf("error=%v public=%+v", err, public)
			}
			if fake.cwdCalls != 0 || fake.resolveCalls != 0 || fake.listCalls != 0 || fake.runtimeCalls != 0 ||
				fake.sessionCalls != 0 || fake.loadStateCalls != 0 || fake.inspectCalls != 0 || fake.deleteCalls != 0 {
				t.Fatalf("unknown Context crossed lifecycle boundary: %+v runtime=%+v", fake, fake.fakeRuntime)
			}
		})
	}
}

func TestProjectStatusPreservesRequestedContextScopeWhenWorkspaceIsAbsent(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: false, project: testProjectInstance(),
	}
	result, err := New(fake).ProjectStatusInContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if result.Exists || result.ContextName != tobari.DefaultContextName || result.ContextID == "" ||
		result.Attachment != tobari.AttachmentNotApplicable || result.Runtime != tobari.RuntimeDiagnosticUnknown {
		t.Fatalf("absent scoped status = %+v", result)
	}
	if fake.resolveCalls != 1 || fake.runtimeCalls != 0 || fake.sessionCalls != 0 {
		t.Fatalf("absent status calls resolve/runtime/session = %d/%d/%d", fake.resolveCalls, fake.runtimeCalls, fake.sessionCalls)
	}
}

func TestLifecycleStaleContextBindingFailsBeforeRuntimeObservationOrDelete(t *testing.T) {
	t.Parallel()
	stale := testProjectInstance()
	stale.ContextName = "toolbox"
	for _, test := range []struct {
		name string
		run  func(*Service) error
	}{
		{name: "status", run: func(service *Service) error {
			_, err := service.ProjectStatusInContext(context.Background(), tobari.DefaultContextName)
			return err
		}},
		{name: "delete", run: func(service *Service) error {
			_, err := service.DeleteProjectInContext(context.Background(), projectWriteIntent("delete"), tobari.DefaultContextName, true)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &projectRuntimeFake{fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: stale}
			err := test.run(New(fake))
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "context_binding_stale" {
				t.Fatalf("error=%v public=%+v", err, public)
			}
			if fake.runtimeCalls != 0 || fake.sessionCalls != 0 || fake.deleteCalls != 0 {
				t.Fatalf("stale binding crossed runtime boundary: runtime/session/delete=%d/%d/%d", fake.runtimeCalls, fake.sessionCalls, fake.deleteCalls)
			}
		})
	}
}

func TestDeletePreviewContextIDMismatchFailsBeforeCWDOrWorkspaceLookup(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: testProjectInstance()}
	_, err := New(fake).DeleteProjectWithContextBinding(
		context.Background(), projectWriteIntent("delete"), tobari.DefaultContextName,
		"01912345-6789-7abc-8def-0123456789ff", true,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindContract || public.Code != "context_binding_stale" {
		t.Fatalf("error=%v public=%+v", err, public)
	}
	if fake.cwdCalls != 0 || fake.resolveCalls != 0 || fake.runtimeCalls != 0 || fake.deleteCalls != 0 {
		t.Fatalf("changed preview binding crossed lifecycle boundary: %+v", fake)
	}
}

func TestProjectListMarksNearestWorkspaceFromCurrentDirectory(t *testing.T) {
	t.Parallel()
	project := testProjectInstance()
	fake := &projectRuntimeFake{
		cwd: "/tmp/project/nested", found: true, project: project,
	}
	result, err := New(fake).ProjectList(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentID != project.ID {
		t.Fatalf("list current ID = %q, want %q", result.CurrentID, project.ID)
	}
}

func TestDeleteProjectDeletesDetachedWorkspaceWithoutForce(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: testProjectInstance(),
	}
	result, err := New(fake).DeleteProject(context.Background(), projectWriteIntent("delete"), false)
	if err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if !result.Deleted || fake.sessionCalls != 1 || fake.deleteCalls != 1 {
		t.Fatalf("result=%+v session calls=%d delete calls=%d", result, fake.sessionCalls, fake.deleteCalls)
	}
}

func TestDeleteProjectRejectsAttachedWorkspaceWithoutForce(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: testProjectInstance(),
		sessionAttached: true,
	}
	_, err := New(fake).DeleteProject(context.Background(), projectWriteIntent("delete"), false)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "project_session_attached" {
		t.Fatalf("DeleteProject() error=%v public=%+v", err, public)
	}
	if fake.sessionCalls != 1 || fake.deleteCalls != 0 {
		t.Fatalf("session calls=%d delete calls=%d, want guard before deletion", fake.sessionCalls, fake.deleteCalls)
	}
}

func TestDeleteProjectForceOverridesAttachedWorkspaceGuard(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: testProjectInstance(),
		sessionAttached: true,
	}
	result, err := New(fake).DeleteProject(context.Background(), projectWriteIntent("delete"), true)
	if err != nil {
		t.Fatalf("DeleteProject(force) error = %v", err)
	}
	if !result.Deleted || fake.sessionCalls != 0 || fake.deleteCalls != 1 {
		t.Fatalf("result=%+v session calls=%d delete calls=%d", result, fake.sessionCalls, fake.deleteCalls)
	}
}

func TestDeleteProjectFailsClosedWhenSessionStatusIsUnavailable(t *testing.T) {
	t.Parallel()
	fake := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{}, cwd: "/tmp/project", found: true, project: testProjectInstance(),
		sessionErr: errors.New("Docker daemon unavailable"),
	}
	_, err := New(fake).DeleteProject(context.Background(), projectWriteIntent("delete"), false)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInternal || public.Code != "session_status_failed" {
		t.Fatalf("DeleteProject() error=%v public=%+v", err, public)
	}
	if fake.sessionCalls != 1 || fake.deleteCalls != 0 {
		t.Fatalf("session calls=%d delete calls=%d, want no deletion after observation failure", fake.sessionCalls, fake.deleteCalls)
	}
}

func policyLearningIntent(command, kind, id string) operation.Intent {
	return operation.Intent{
		Command: command, Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: kind, ID: id},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
		},
	}
}

func TestClusterDenialsReturnsPolicyAndEmptyBoundedScope(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	result, err := New(runtime).ClusterDenials(context.Background(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskClusterDenials ||
		result.PolicyDirectory != runtime.state.PolicyDirectory ||
		result.WindowLines != 75 || result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("denial result = %+v", result)
	}
}

func TestPolicyCandidatesIgnoreCurrentContextAsAuthority(t *testing.T) {
	t.Parallel()
	state := testState(t.TempDir())
	runtime := &activeContextFake{fakeRuntime: &fakeRuntime{state: state}, active: "project-tools"}
	result, err := New(runtime).PolicyCandidates(context.Background(), 75)
	if err != nil || result.Items == nil {
		t.Fatalf("PolicyCandidates() = %+v, %v", result, err)
	}
}

func TestPolicyCandidatesProduceExactOpaqueReferenceAndTailTask(t *testing.T) {
	t.Parallel()
	denial := validServiceDenial()
	repeated := denial
	repeated.Timestamp = "2026-07-30T10:42:11Z"
	repeated.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{denial, repeated}}
	result, err := New(runtime).PolicyCandidates(context.Background(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskPolicyCandidates || len(result.Items) != 1 ||
		result.Items[0].Host != denial.Host || result.Items[0].ObservationCount != 2 ||
		result.Items[0].ObservedAt != repeated.Timestamp {
		t.Fatalf("candidate result = %+v", result)
	}
	if err := tobari.ValidatePolicyCandidateID(result.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	review, err := New(runtime).PolicyReview(context.Background(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if review.Task != tobari.TaskPolicyReview || review.Items[0].ID != result.Items[0].ID {
		t.Fatalf("review = %+v, candidates = %+v", review, result)
	}
}

func TestServiceInteractiveRequiresTerminalStreams(t *testing.T) {
	t.Parallel()
	runtime := &projectRuntimeFake{fakeRuntime: &fakeRuntime{}, terminal: true}
	service := New(runtime)
	if !service.IsInteractive(bytes.NewReader(nil), &bytes.Buffer{}) {
		t.Fatal("interactive runtime was not recognized")
	}
	runtime.terminal = false
	if service.IsInteractive(bytes.NewReader(nil), &bytes.Buffer{}) {
		t.Fatal("non-terminal runtime entered interactive mode")
	}
}

func validServiceDenial() tobari.PolicyDenial {
	return tobari.PolicyDenial{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Protocol: tobari.PolicyProtocolHTTP}, Timestamp: "2026-07-30T10:41:11Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "GET", Path: "/api/v1/items/one",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
}

func TestAllowPolicyCandidateBindsReferenceBeforeApplying(t *testing.T) {
	t.Parallel()
	denial := validServiceDenial()
	candidate, _ := tobari.NewPolicyCandidate(denial)
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{denial}}
	result, err := New(runtime).AllowPolicyCandidate(
		context.Background(),
		policyLearningIntent("policy allow", tobari.PolicyCandidateKind, candidate.ID),
		candidate.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.learnedCalls != 1 || result.TargetID != candidate.ID ||
		result.Rule.Match != tobari.PolicyMatchExact || !result.Applied ||
		result.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory {
		t.Fatalf("result=%+v calls=%d", result, runtime.learnedCalls)
	}

	wrong := policyLearningIntent(
		"policy allow", tobari.PolicyCandidateKind,
		"pcy_0123456789abcdef0123456789abcdef",
	)
	if _, err := New(runtime).AllowPolicyCandidate(
		context.Background(), wrong, candidate.ID,
	); err == nil {
		t.Fatal("mismatched policy candidate target was accepted")
	}
	if runtime.learnedCalls != 1 {
		t.Fatalf("mismatched target caused mutation: %d", runtime.learnedCalls)
	}
}

func TestPolicyActivationReceiptFailureDoesNotMakeConfirmedMutationRetryable(t *testing.T) {
	t.Parallel()
	denial := validServiceDenial()
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeRuntime{
		state:   testState(t.TempDir()),
		denials: []tobari.PolicyDenial{denial},
		activationReceipt: tobari.PolicyActivationReceipt{
			PolicyDirectory: "relative/policy",
			ActiveRevision:  strings.Repeat("d", 64),
		},
	}
	_, err = New(runtime).AllowPolicyCandidate(
		context.Background(),
		policyLearningIntent("policy allow", tobari.PolicyCandidateKind, candidate.ID),
		candidate.ID,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "unclassified_mutation_outcome" || public.Retryable || runtime.learnedCalls != 1 {
		t.Fatalf("fault=%#v mutation calls=%d", public, runtime.learnedCalls)
	}
}

func TestApplyPolicyReviewDecisionSetRevalidatesAndActivatesOnce(t *testing.T) {
	t.Parallel()
	allowDenial := validServiceDenial()
	denyDenial := validServiceDenial()
	denyDenial.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	denyDenial.Path = "/api/v1/items/two"
	allowCandidate, _ := tobari.NewPolicyCandidate(allowDenial)
	denyCandidate, _ := tobari.NewPolicyCandidate(denyDenial)
	runtime := &fakeRuntime{
		state: testState(t.TempDir()), denials: []tobari.PolicyDenial{allowDenial, denyDenial},
	}
	intent := operation.Intent{
		Command: "policy apply-reviewed", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
		},
	}
	result, err := New(runtime).ApplyPolicyReviewDecisionSet(
		context.Background(), intent, tobari.PolicyReviewDecisionSet{Decisions: []tobari.PolicyReviewDecision{
			{CandidateID: allowCandidate.ID, Decision: tobari.PolicyDecisionAllow},
			{CandidateID: denyCandidate.ID, Decision: tobari.PolicyDecisionDeny},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.decisionSetCalls != 1 || len(runtime.rules) != 1 || len(runtime.denyRules) != 1 ||
		result.AllowCount != 1 || result.DenyCount != 1 || !result.Applied ||
		result.ActiveRevision != strings.Repeat("b", 64) || len(result.Decisions) != 2 ||
		result.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory ||
		result.Decisions[0].CandidateID != allowCandidate.ID ||
		result.Decisions[1].CandidateID != denyCandidate.ID {
		t.Fatalf("result=%+v calls=%d allows=%+v denies=%+v", result, runtime.decisionSetCalls, runtime.rules, runtime.denyRules)
	}

	stale := tobari.PolicyReviewDecisionSet{Decisions: []tobari.PolicyReviewDecision{{
		CandidateID: "pcy_0123456789abcdef0123456789abcdef", Decision: tobari.PolicyDecisionAllow,
	}}}
	if _, err := New(runtime).ApplyPolicyReviewDecisionSet(context.Background(), intent, stale); err == nil {
		t.Fatal("stale reviewed candidate was accepted")
	}
	if runtime.decisionSetCalls != 1 {
		t.Fatalf("stale reviewed set caused a mutation: %d", runtime.decisionSetCalls)
	}
}

func TestApplyPolicyReviewDecisionSetRejectsMultipleContextSources(t *testing.T) {
	t.Parallel()
	first := validServiceDenial()
	second := validServiceDenial()
	second.RequestID = "9185da2688d7469aae9cd9068e920b0b"
	second.ContextID = "01912345-6789-7abc-8def-0123456789ae"
	second.ContextName = "restricted"
	second.ProjectID = "01912345-6789-7abc-8def-0123456789ac"
	second.ProjectRoot = "/workspace/restricted"
	second.Path = "/api/v1/items/restricted"
	firstCandidate, _ := tobari.NewPolicyCandidate(first)
	secondCandidate, _ := tobari.NewPolicyCandidate(second)
	runtime := &fakeRuntime{
		state: testState(t.TempDir()), denials: []tobari.PolicyDenial{first, second},
	}
	intent := operation.Intent{
		Command: "policy apply-reviewed", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ID: tobari.PolicyDecisionSetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
		},
	}
	_, err := New(runtime).ApplyPolicyReviewDecisionSet(
		context.Background(), intent, tobari.PolicyReviewDecisionSet{Decisions: []tobari.PolicyReviewDecision{
			{CandidateID: firstCandidate.ID, Decision: tobari.PolicyDecisionAllow},
			{CandidateID: secondCandidate.ID, Decision: tobari.PolicyDecisionDeny},
		}},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "policy_review_scope_mixed" {
		t.Fatalf("mixed-Context reviewed set error = %v", err)
	}
	if runtime.decisionSetCalls != 0 {
		t.Fatalf("mixed-Context reviewed set caused %d runtime calls", runtime.decisionSetCalls)
	}
}

func TestAllowPolicyCandidateRejectsStaleReferenceWithoutMutation(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{}}
	id := "pcy_0123456789abcdef0123456789abcdef"
	_, err := New(runtime).AllowPolicyCandidate(
		context.Background(),
		policyLearningIntent("policy allow", tobari.PolicyCandidateKind, id),
		id,
	)
	if err == nil || runtime.learnedCalls != 0 {
		t.Fatalf("stale candidate error=%v calls=%d", err, runtime.learnedCalls)
	}
}

func TestDenyPolicyCandidateBindsExactReferenceAndRemovesQueueItem(t *testing.T) {
	t.Parallel()
	denial := validServiceDenial()
	candidate, _ := tobari.NewPolicyCandidate(denial)
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{denial}}
	result, err := New(runtime).DenyPolicyCandidate(
		context.Background(),
		policyLearningIntent("policy deny", tobari.PolicyCandidateKind, candidate.ID),
		candidate.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.denyCalls != 1 || result.TargetID != candidate.ID || !result.Applied ||
		result.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory ||
		len(runtime.denyRules) != 1 || !runtime.denyRules[0].Matches(
		candidate.ContextID, candidate.ProjectID, candidate.Host, candidate.Port, candidate.Method, candidate.Path,
	) {
		t.Fatalf("result=%+v deny calls=%d rules=%+v", result, runtime.denyCalls, runtime.denyRules)
	}
}

func TestDenyPolicyCandidateRejectsStaleReferenceWithoutMutation(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{}}
	id := "pcy_0123456789abcdef0123456789abcdef"
	_, err := New(runtime).DenyPolicyCandidate(
		context.Background(),
		policyLearningIntent("policy deny", tobari.PolicyCandidateKind, id), id,
	)
	if err == nil || runtime.denyCalls != 0 {
		t.Fatalf("stale candidate error=%v calls=%d", err, runtime.denyCalls)
	}
}

func TestPolicyRulesAndResetKeepAllowAndDenyReversible(t *testing.T) {
	t.Parallel()
	allowDenial := validServiceDenial()
	allowCandidate, err := tobari.NewPolicyCandidate(allowDenial)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := tobari.NewExactLearnedPolicyRule(allowCandidate)
	if err != nil {
		t.Fatal(err)
	}
	denyDenial := allowDenial
	denyDenial.Path = "/api/v1/items/other"
	denyCandidate, err := tobari.NewPolicyCandidate(denyDenial)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := tobari.NewExactPolicyDenyRule(denyCandidate)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &fakeRuntime{
		state: testState(t.TempDir()), rules: []tobari.LearnedPolicyRule{allow},
		denyRules: []tobari.PolicyDenyRule{deny},
	}
	service := New(runtime)
	report, err := service.PolicyRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 2 || report.Items[0].Decision != tobari.PolicyDecisionAllow || report.Items[1].Decision != tobari.PolicyDecisionDeny {
		t.Fatalf("policy rule report = %+v", report)
	}

	resetAllow, err := service.ResetPolicyRule(
		context.Background(), policyLearningIntent("policy reset", tobari.PolicyRuleKind, allow.ID), allow.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resetAllow.Decision != tobari.PolicyDecisionAllow || !resetAllow.Applied || runtime.learnedCalls != 1 || len(runtime.rules) != 0 || len(runtime.denyRules) != 1 ||
		resetAllow.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory {
		t.Fatalf("allow reset=%+v learned calls=%d rules=%+v denies=%+v", resetAllow, runtime.learnedCalls, runtime.rules, runtime.denyRules)
	}

	resetDeny, err := service.ResetPolicyRule(
		context.Background(), policyLearningIntent("policy reset", tobari.PolicyRuleKind, deny.ID), deny.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if resetDeny.Decision != tobari.PolicyDecisionDeny || !resetDeny.Applied || runtime.denyCalls != 1 || len(runtime.denyRules) != 0 ||
		resetDeny.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory {
		t.Fatalf("deny reset=%+v deny calls=%d denies=%+v", resetDeny, runtime.denyCalls, runtime.denyRules)
	}
	if report, err := service.PolicyRules(context.Background()); err != nil || len(report.Items) != 0 {
		t.Fatalf("policy rules after reset = %+v, error=%v", report, err)
	}
	if _, err := service.ResetPolicyRule(
		context.Background(), policyLearningIntent("policy reset", tobari.PolicyRuleKind, deny.ID), deny.ID,
	); err == nil || runtime.denyCalls != 1 {
		t.Fatalf("stale reset error=%v deny calls=%d", err, runtime.denyCalls)
	}
}

func compactableServiceRules(t *testing.T) []tobari.LearnedPolicyRule {
	t.Helper()
	paths := []string{
		"/api/v1/items/one", "/api/v1/items/two", "/api/v1/items/three",
	}
	ids := []string{
		"1185da2688d7469aae9cd9068e920b0b",
		"2185da2688d7469aae9cd9068e920b0b",
		"3185da2688d7469aae9cd9068e920b0b",
	}
	rules := make([]tobari.LearnedPolicyRule, 0, len(paths))
	for index, path := range paths {
		denial := validServiceDenial()
		denial.RequestID, denial.Path = ids[index], path
		candidate, err := tobari.NewPolicyCandidate(denial)
		if err != nil {
			t.Fatal(err)
		}
		rule, err := tobari.NewExactLearnedPolicyRule(candidate)
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	return rules
}

func TestPolicyCompactionRoundTripUsesCurrentOpaqueReference(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir()), rules: compactableServiceRules(t)}
	report, err := New(runtime).PolicyCompactions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 {
		t.Fatalf("compactions = %+v", report.Items)
	}
	id := report.Items[0].ID
	result, err := New(runtime).CompactPolicy(
		context.Background(),
		policyLearningIntent("policy compact", tobari.PolicyCompactionKind, id),
		id,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.learnedCalls != 1 || result.TargetID != id ||
		result.Rule.Match != tobari.PolicyMatchPrefix || result.SourceRuleCount != 3 ||
		result.PolicyDirectory != runtime.policyActivationReceipt().PolicyDirectory {
		t.Fatalf("result=%+v calls=%d", result, runtime.learnedCalls)
	}
	if _, err := New(runtime).CompactPolicy(
		context.Background(),
		policyLearningIntent("policy compact", tobari.PolicyCompactionKind, id),
		id,
	); err == nil {
		t.Fatal("stale compaction was accepted")
	}
	if runtime.learnedCalls != 1 {
		t.Fatalf("stale compaction caused mutation: %d", runtime.learnedCalls)
	}
}

func TestClusterDownRejectsRemainingCWDProjectBeforeMutation(t *testing.T) {
	t.Parallel()
	runtime := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{},
		found:       true,
		project:     testProjectInstance(),
	}
	intent := operation.Intent{
		Command: "cluster down", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes,
		},
	}
	if _, err := New(runtime).ClusterDown(context.Background(), intent, false); err == nil {
		t.Fatal("cluster down accepted while a CWD-owned project remained")
	} else if public, ok := fault.PublicCopy(err); !ok || public.Code != "cluster_not_empty" ||
		!strings.Contains(public.Message, "delete every logical Workspace") ||
		strings.Contains(strings.ToLower(public.Message), "detach") {
		t.Fatalf("cluster down fault = %+v, structured=%t", public, ok)
	}
}
