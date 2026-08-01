package tobaricmd

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeRuntime struct {
	state          tobari.State
	clusterCalls   int
	loadStateCalls int
	inspectCalls   int
	configured     *bool
	clusterReady   *bool
	inspectErr     error
	attachCalls    int
	detachCalls    int
	policyCalls    int
	learnedCalls   int
	execSeen       tobari.Instance
	devConfig      tobari.DevContainerConfig
	denials        []tobari.PolicyDenial
	rules          []tobari.LearnedPolicyRule
}

func (f *fakeRuntime) ResolveRoot(_ context.Context, root string) (string, error) { return root, nil }
func (f *fakeRuntime) CurrentDirectory(context.Context) (string, error) {
	if len(f.state.Tobari) == 0 {
		return "/", nil
	}
	return f.state.Tobari[0].Root, nil
}
func (f *fakeRuntime) IsTerminal(io.Writer) bool { return false }
func (f *fakeRuntime) ResolveImageSelector(_ context.Context, image string) (string, error) {
	if image == "" {
		return tobari.BuiltinImageSelector, nil
	}
	return image, nil
}
func (f *fakeRuntime) ReadDevContainer(_ context.Context, _, _ string) (tobari.DevContainerConfig, error) {
	if f.devConfig.Properties != nil {
		return f.devConfig, nil
	}
	return tobari.DevContainerConfig{
		Image: "devcontainer:local", Properties: []string{"image"},
	}, nil
}

func TestAttachRejectsUnsupportedDevContainerBeforeMutation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime := &fakeRuntime{
		state: testState(root),
		devConfig: tobari.DevContainerConfig{
			Image: "devcontainer:local", Properties: []string{"image", "mounts"},
		},
	}
	_, err := New(runtime).Attach(
		context.Background(), createIntent("attach"), "config", filepath.Join(root, "config"),
		"", ".devcontainer/devcontainer.json",
	)
	if err == nil || runtime.attachCalls != 0 {
		t.Fatalf("Attach() error = %v, calls = %d", err, runtime.attachCalls)
	}
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
		Configured: true, Running: running, Proxy: f.state.ProxyEndpoint,
		Policy: f.state.PolicyDirectory, TobariCount: len(f.state.Tobari),
		Components: []tobari.ComponentStatus{},
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
func (f *fakeRuntime) Attach(_ context.Context, state tobari.State, name, root, image string) (tobari.State, error) {
	f.attachCalls++
	state.Tobari = append(state.Tobari, tobari.Instance{
		ID: "tbr_abcdef0123456789abcdef0123456789", Name: name, Root: root,
		Container: "tobari-" + name, Network: "tobari-" + name + "-net",
		HomeVolume: "tobari-" + name + "-home", Image: image,
	})
	f.state = state
	return state, nil
}
func (f *fakeRuntime) InspectTobari(_ context.Context, state tobari.State) ([]tobari.ItemStatus, error) {
	items := make([]tobari.ItemStatus, 0, len(state.Tobari))
	for _, instance := range state.Tobari {
		items = append(items, tobari.ItemStatus{
			ID: instance.ID, Name: instance.Name, Root: instance.Root,
			Image: instance.ImageSelector(), Running: true, Container: instance.Container,
		})
	}
	return items, nil
}
func (f *fakeRuntime) Exec(_ context.Context, instance tobari.Instance, _ tobari.ExecRequest, _ io.Reader, _ io.Writer, _ io.Writer) (int, error) {
	f.execSeen = instance
	return 23, nil
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
func (f *fakeRuntime) ApplyLearnedPolicyRules(
	_ context.Context, _ tobari.State, _, updated []tobari.LearnedPolicyRule,
) error {
	f.learnedCalls++
	f.rules = append([]tobari.LearnedPolicyRule{}, updated...)
	return nil
}
func (f *fakeRuntime) ApplyPolicy(context.Context, tobari.State) error {
	f.policyCalls++
	return nil
}
func (f *fakeRuntime) TobariLogs(context.Context, tobari.Instance, tobari.LogRequest) ([]byte, error) {
	return []byte("tobari\n"), nil
}
func (f *fakeRuntime) Detach(_ context.Context, state tobari.State, instance tobari.Instance, _ bool) (tobari.State, error) {
	f.detachCalls++
	state.Tobari = []tobari.Instance{}
	f.state = state
	return state, nil
}
func (f *fakeRuntime) ClusterDown(context.Context, tobari.State, bool) error { return nil }
func (f *fakeRuntime) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	return action(ctx)
}
func (f *fakeRuntime) Doctor(context.Context, string) (doctor.Report, error) {
	return doctor.Report{Checks: []doctor.Check{{Name: "docker", Status: doctor.CheckStatusPass, Detail: "available"}}}, nil
}

type projectRuntimeFake struct {
	*fakeRuntime
	cwd          string
	terminal     bool
	inside       bool
	project      tobari.ProjectInstance
	found        bool
	resolveCalls int
	ensureCalls  int
	enterCalls   int
	deleteCalls  int
}

func (f *projectRuntimeFake) CurrentDirectory(context.Context) (string, error) { return f.cwd, nil }
func (f *projectRuntimeFake) IsTerminal(io.Writer) bool                        { return f.terminal }
func (f *projectRuntimeFake) InsideProject(context.Context) bool               { return f.inside }
func (f *projectRuntimeFake) ResolveProject(context.Context, string) (tobari.ProjectInstance, bool, error) {
	f.resolveCalls++
	return f.project, f.found, nil
}
func (f *projectRuntimeFake) ResolveOrCreateProject(context.Context, string) (tobari.ProjectInstance, bool, error) {
	f.resolveCalls++
	return f.project, true, nil
}
func (f *projectRuntimeFake) ListProjects(context.Context) ([]tobari.ProjectInstance, error) {
	if !f.found {
		return []tobari.ProjectInstance{}, nil
	}
	return []tobari.ProjectInstance{f.project}, nil
}
func (f *projectRuntimeFake) ProjectHome(context.Context, tobari.ProjectInstance) (string, error) {
	return "/tmp/tobari-home", nil
}
func (f *projectRuntimeFake) EnsureProjectRuntime(_ context.Context, _ tobari.State, instance tobari.ProjectInstance) (tobari.ProjectInstance, error) {
	f.ensureCalls++
	return instance, nil
}
func (f *projectRuntimeFake) InspectProjectRuntime(context.Context, tobari.ProjectInstance) (tobari.RuntimeDiagnostic, error) {
	return tobari.RuntimeDiagnosticMissing, nil
}
func (f *projectRuntimeFake) EnterProjectRuntime(context.Context, tobari.ProjectInstance, string, io.Reader, io.Writer, io.Writer) (int, error) {
	f.enterCalls++
	return 0, nil
}
func (f *projectRuntimeFake) DeleteProject(context.Context, tobari.ProjectInstance) error {
	f.deleteCalls++
	return nil
}

func testState(root string) tobari.State {
	instance := tobari.Instance{
		ID: "tbr_0123456789abcdef0123456789abcdef", Name: "work",
		Root: filepath.Join(root, "work"), Container: "tobari-work",
		Network: "tobari-work-net", HomeVolume: "tobari-work-home",
	}
	return tobari.State{
		SchemaVersion: 2, RuntimeDirectory: filepath.Join(root, "runtime"),
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
		ProxyEndpoint: "http://gateway:8080", Tobari: []tobari.Instance{instance},
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
		Image: tobari.BuiltinImageSelector,
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
	code, err := New(fake).EnterProject(
		context.Background(), projectCreateIntent("tobari"), bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProject() = (%d, %v)", code, err)
	}
	if fake.clusterCalls != 0 || fake.resolveCalls != 1 || fake.ensureCalls != 1 || fake.enterCalls != 1 {
		t.Fatalf("calls = cluster:%d resolve:%d ensure:%d enter:%d", fake.clusterCalls, fake.resolveCalls, fake.ensureCalls, fake.enterCalls)
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
	if !result.Exists || result.Runtime != tobari.RuntimeDiagnosticMissing || fake.resolveCalls != 1 {
		t.Fatalf("status=%+v calls=%d", result, fake.resolveCalls)
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

func applyPolicyIntent() operation.Intent {
	return operation.Intent{
		Command: "policy apply", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo,
		},
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

func TestAttachRejectsInvalidNameBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	_, err := New(runtime).Attach(
		context.Background(), createIntent("attach"), "../bad", "/tmp/root",
		tobari.BuiltinImageSelector, "",
	)
	if err == nil || runtime.attachCalls != 0 {
		t.Fatalf("Attach() error = %v, calls = %d", err, runtime.attachCalls)
	}
}

func TestAttachCanceledBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(runtime).Attach(
		ctx, createIntent("attach"), "work", "/tmp/root", "", ".devcontainer/devcontainer.json",
	)
	if err == nil || runtime.attachCalls != 0 {
		t.Fatalf("Attach() error = %v, calls = %d", err, runtime.attachCalls)
	}
}

func TestAttachRejectsConflictingImageSourcesBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	_, err := New(runtime).Attach(
		context.Background(), createIntent("attach"), "work", "/tmp/root",
		"workbench:dev", ".devcontainer/devcontainer.json",
	)
	if err == nil || runtime.attachCalls != 0 {
		t.Fatalf("Attach() error = %v, calls = %d", err, runtime.attachCalls)
	}
}

func TestAttachRejectsImageChangeForExistingNameAndRoot(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	existing := runtime.state.Tobari[0]
	_, err := New(runtime).Attach(
		context.Background(), createIntent("attach"), existing.Name, existing.Root, "workbench:dev", "",
	)
	if err == nil || runtime.attachCalls != 0 {
		t.Fatalf("Attach() error = %v, calls = %d", err, runtime.attachCalls)
	}
}

func TestAttachUsesDevContainerImageInsteadOfXDGDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime := &fakeRuntime{state: testState(root)}
	instance, err := New(runtime).Attach(
		context.Background(), createIntent("attach"), "config", filepath.Join(root, "config"),
		"", ".devcontainer/devcontainer.json",
	)
	if err != nil {
		t.Fatal(err)
	}
	if instance.Image != "devcontainer:local" || runtime.attachCalls != 1 {
		t.Fatalf("instance = %+v, calls = %d", instance, runtime.attachCalls)
	}
}

func TestExecUsesOpaqueIDWithoutNameDiscovery(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	instance := runtime.state.Tobari[0]
	code, err := New(runtime).Exec(
		context.Background(), instance.ID,
		tobari.ExecRequest{Command: []string{"sh", "-c", "exit 23"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil || code != 23 || runtime.execSeen.ID != instance.ID {
		t.Fatalf("Exec() code=%d err=%v seen=%+v", code, err, runtime.execSeen)
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

func TestPolicyCandidatesProduceExactOpaqueReferenceAndTailTask(t *testing.T) {
	t.Parallel()
	denial := validServiceDenial()
	runtime := &fakeRuntime{state: testState(t.TempDir()), denials: []tobari.PolicyDenial{denial}}
	result, err := New(runtime).PolicyCandidates(context.Background(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskPolicyCandidates || len(result.Items) != 1 ||
		result.Items[0].Host != denial.Host {
		t.Fatalf("candidate result = %+v", result)
	}
	if err := tobari.ValidatePolicyCandidateID(result.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	tail, err := New(runtime).PolicyTail(context.Background(), 75)
	if err != nil {
		t.Fatal(err)
	}
	if tail.Task != tobari.TaskPolicyTail || tail.Items[0].ID != result.Items[0].ID {
		t.Fatalf("tail = %+v, candidates = %+v", tail, result)
	}
}

func validServiceDenial() tobari.PolicyDenial {
	return tobari.PolicyDenial{
		Timestamp: "2026-07-30T10:41:11Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab",
		Host:      "api.example.com", Port: 443, Method: "GET", Path: "/api/v1/items/one",
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
		result.Rule.Match != tobari.PolicyMatchExact || !result.Applied {
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
		result.Rule.Match != tobari.PolicyMatchPrefix || result.SourceRuleCount != 3 {
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

func TestApplyPolicyUsesFixedClusterMutationAndConfirmsResult(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	result, err := New(runtime).ApplyPolicy(context.Background(), applyPolicyIntent())
	if err != nil {
		t.Fatal(err)
	}
	if runtime.policyCalls != 1 || !result.Applied ||
		result.PolicyDirectory != runtime.state.PolicyDirectory {
		t.Fatalf("activation = %+v, calls = %d", result, runtime.policyCalls)
	}
}

func TestApplyPolicyRejectsWrongIntentBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	intent := applyPolicyIntent()
	intent.Effect = operation.EffectCreate
	if _, err := New(runtime).ApplyPolicy(context.Background(), intent); err == nil {
		t.Fatal("invalid policy mutation was accepted")
	}
	if runtime.policyCalls != 0 {
		t.Fatalf("policy calls = %d", runtime.policyCalls)
	}
}

func TestDetachRequiresIntentIDToMatchConsumedReference(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	instance := runtime.state.Tobari[0]
	intent := operation.Intent{
		Command: "detach", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.TargetKind, ID: "tbr_abcdef0123456789abcdef0123456789"},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes,
		},
	}
	if err := New(runtime).Detach(context.Background(), intent, instance.ID, false); err == nil {
		t.Fatal("mismatched target was accepted")
	}
	if runtime.detachCalls != 0 {
		t.Fatalf("detach calls = %d", runtime.detachCalls)
	}
}

func TestClusterDownRejectsNonEmptyClusterBeforeMutation(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState(t.TempDir())}
	intent := operation.Intent{
		Command: "cluster down", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationYes,
		},
	}
	if _, err := New(runtime).ClusterDown(context.Background(), intent, false); err == nil {
		t.Fatal("non-empty cluster was removed")
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
	}
}
