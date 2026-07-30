package tobaricmd

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeRuntime struct {
	state        tobari.State
	attachCalls  int
	detachCalls  int
	policyCalls  int
	learnedCalls int
	execSeen     tobari.Instance
	devConfig    tobari.DevContainerConfig
	denials      []tobari.PolicyDenial
	rules        []tobari.LearnedPolicyRule
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
func (f *fakeRuntime) ClusterUp(context.Context) (tobari.State, error) { return f.state, nil }
func (f *fakeRuntime) LoadState(context.Context) (tobari.State, bool, error) {
	return f.state, true, nil
}
func (f *fakeRuntime) InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error) {
	return tobari.ClusterStatus{
		Configured: true, Running: true, Proxy: f.state.ProxyEndpoint,
		Policy: f.state.PolicyDirectory, TobariCount: len(f.state.Tobari),
		Components: []tobari.ComponentStatus{},
	}, nil
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
func (f *fakeRuntime) Doctor(context.Context, string) (doctor.Report, error) {
	return doctor.Report{Checks: []doctor.Check{{Name: "docker", Status: doctor.CheckStatusPass, Detail: "available"}}}, nil
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
		Host: "api.example.com", Method: "GET", Path: "/api/v1/items/one",
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
