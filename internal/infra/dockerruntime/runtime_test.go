package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runnerCall struct{ args []string }
type recordingRunner struct {
	runs        []runnerCall
	outputs     []runnerCall
	outputData  []byte
	outputErr   error
	outputQueue [][]byte
	onOutput    func(int)
}

type ownershipInspectFailureRunner struct {
	outputs []runnerCall
	runs    []runnerCall
}

type policyProbeRunner struct {
	outputs []runnerCall
}

type clusterUpProgressRunner struct{}

func (clusterUpProgressRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (clusterUpProgressRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) >= 3 && args[0] == "inspect" {
		if strings.Contains(args[2], "NetworkSettings.Networks") {
			return []byte(`{}`), nil
		}
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	return []byte{}, nil
}

func TestClusterUpWithProgressReportsEachRuntimeStageInOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"),
		clusterUpProgressRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	var events []tobari.ClusterUpProgress
	if _, err := runtime.ClusterUpWithProgress(context.Background(), func(event tobari.ClusterUpProgress) {
		events = append(events, event)
	}); err != nil {
		t.Fatal(err)
	}
	wantSteps := []tobari.ClusterUpProgressStep{
		tobari.ClusterUpProgressPrepare,
		tobari.ClusterUpProgressPolicy,
		tobari.ClusterUpProgressBuildImage,
		tobari.ClusterUpProgressStartServices,
		tobari.ClusterUpProgressConnectNetworks,
		tobari.ClusterUpProgressWaitForHealth,
		tobari.ClusterUpProgressReconcileProjects,
		tobari.ClusterUpProgressFinalize,
	}
	if len(events) != len(wantSteps)*2 {
		t.Fatalf("progress event count = %d, events = %+v", len(events), events)
	}
	for index, step := range wantSteps {
		start, complete := events[index*2], events[index*2+1]
		if start != (tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressStarted}) ||
			complete != (tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressCompleted}) {
			t.Fatalf("stage %q events = %+v, %+v", step, start, complete)
		}
	}
}

func TestRunClusterUpProgressStepReportsCompletionAndFailure(t *testing.T) {
	t.Parallel()
	var completed []tobari.ClusterUpProgress
	if err := runClusterUpProgressStep(
		func(event tobari.ClusterUpProgress) { completed = append(completed, event) },
		tobari.ClusterUpProgressPolicy,
		func() error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	want := []tobari.ClusterUpProgress{
		{Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressStarted},
		{Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressCompleted},
	}
	if len(completed) != len(want) || completed[0] != want[0] || completed[1] != want[1] {
		t.Fatalf("completed events = %+v, want %+v", completed, want)
	}
	failed := []tobari.ClusterUpProgress{}
	wantErr := errors.New("synthetic stage failure")
	if err := runClusterUpProgressStep(
		func(event tobari.ClusterUpProgress) { failed = append(failed, event) },
		tobari.ClusterUpProgressBuildImage,
		func() error { return wantErr },
	); !errors.Is(err, wantErr) {
		t.Fatalf("failed step error = %v, want %v", err, wantErr)
	}
	if len(failed) != 2 || failed[0].Status != tobari.ClusterUpProgressStarted || failed[1].Status != tobari.ClusterUpProgressFailed {
		t.Fatalf("failed events = %+v", failed)
	}
}

func TestWaitForClusterReadyEmitsHealthUpdates(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte(`{"state":"starting","health":"starting"}`)}
	runtime := &Runtime{runner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []tobari.ClusterUpProgress
	err := runtime.waitForClusterReady(ctx, func(event tobari.ClusterUpProgress) {
		events = append(events, event)
		if event.Status == tobari.ClusterUpProgressUpdated {
			cancel()
		}
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	if len(events) != 1 || events[0] != (tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressUpdated,
	}) {
		t.Fatalf("health progress events = %+v", events)
	}
}

func (r *ownershipInspectFailureRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}

func (r *ownershipInspectFailureRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	return []byte("Docker daemon unavailable"), errors.New("Docker daemon unavailable")
}

func (r *policyProbeRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *policyProbeRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "run" {
		return []byte("invalid mount config for type bind"), errors.New("policy bind is not accessible")
	}
	return nil, nil
}

func (r *recordingRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}
func (r *recordingRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if r.onOutput != nil {
		r.onOutput(len(r.outputs))
	}
	if len(r.outputQueue) > 0 {
		output := append([]byte{}, r.outputQueue[0]...)
		r.outputQueue = r.outputQueue[1:]
		return output, r.outputErr
	}
	return append([]byte{}, r.outputData...), r.outputErr
}

type gatewayNetworkRunner struct {
	outputs  []runnerCall
	networks string
}

func (r *gatewayNetworkRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return nil
}

func (r *gatewayNetworkRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "inspect" {
		return []byte(r.networks), nil
	}
	return []byte{}, nil
}

func runtimeState(root string) tobari.State {
	return tobari.State{
		SchemaVersion: 2, RuntimeDirectory: filepath.Join(root, "runtime"),
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
		ProxyEndpoint: "http://gateway:8080", Tobari: []tobari.Instance{},
	}
}

func runtimeInstance(root string) tobari.Instance {
	return tobari.Instance{
		ID: "tbr_0123456789abcdef0123456789abcdef", Name: "work", Root: root,
		Container: "tobari-work", Network: "tobari-work-net", HomeVolume: "tobari-work-home",
	}
}

func TestResolveRuntimeHomesUsesXDGAndPortableFallbacks(t *testing.T) {
	t.Parallel()
	config := filepath.Join(string(filepath.Separator), "xdg", "config")
	state := filepath.Join(string(filepath.Separator), "xdg", "state")
	gotConfig, gotState, err := resolveRuntimeHomes(config, state, func() (string, error) {
		return "", errors.New("must not resolve home")
	})
	if err != nil || gotConfig != config || gotState != state {
		t.Fatalf("resolved (%q,%q,%v)", gotConfig, gotState, err)
	}
	home := filepath.Join(string(filepath.Separator), "home", "example")
	gotConfig, gotState, err = resolveRuntimeHomes("", "", func() (string, error) { return home, nil })
	if err != nil || gotConfig != filepath.Join(home, ".config") || gotState != filepath.Join(home, ".local", "state") {
		t.Fatalf("fallback (%q,%q,%v)", gotConfig, gotState, err)
	}
}

func TestResolveProjectRootRejectsProtectedManagementPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.dataDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	protected := map[string]string{
		"filesystem root": string(filepath.Separator),
		"user home":       home,
		"config":          runtime.configDirectory,
		"config child":    filepath.Join(runtime.configDirectory, "policy"),
		"config ancestor": filepath.Dir(runtime.configDirectory),
		"state":           runtime.stateDirectory,
		"data":            runtime.dataDirectory,
		"docker runtime":  filepath.Join(string(filepath.Separator), "var", "run"),
	}
	for name, candidate := range protected {
		name, candidate := name, candidate
		t.Run(name, func(t *testing.T) {
			if candidate != string(filepath.Separator) {
				if err := os.MkdirAll(candidate, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := runtime.ResolveProjectRoot(context.Background(), candidate); err == nil {
				t.Fatalf("ResolveProjectRoot(%q) unexpectedly succeeded", candidate)
			}
		})
	}
	if _, err := runtime.ResolveProjectRoot(context.Background(), projectRoot); err != nil {
		t.Fatalf("ordinary project root rejected: %v", err)
	}
}

func TestDoctorRejectsProtectedProspectiveRootsAfterSymlinkResolution(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	runner := &recordingRunner{}
	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootAlias := filepath.Join(root, "root-alias")
	if err := os.Symlink(string(filepath.Separator), rootAlias); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]string{
		"filesystem root":            string(filepath.Separator),
		"symlink to filesystem root": rootAlias,
	} {
		t.Run(name, func(t *testing.T) {
			report, err := runtime.Doctor(context.Background(), candidate)
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range report.Checks {
				if check.Name != "root" {
					continue
				}
				if check.Status != doctor.CheckStatusFail {
					t.Fatalf("root check = %+v, want fail", check)
				}
				if !strings.Contains(check.Detail, "cannot be a Tobari project root") {
					t.Fatalf("root failure detail = %q", check.Detail)
				}
				return
			}
			t.Fatal("doctor report did not contain a root check")
		})
	}
	if len(runner.runs) != 0 {
		t.Fatalf("doctor performed Docker mutations: %v", runner.runs)
	}
}

func TestDoctorDiagnosesExistingPolicyBeforeClusterIsConfigured(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &policyProbeRunner{}
	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	policyDirectory := filepath.Join(runtime.configDirectory, "policy")
	if err := os.MkdirAll(policyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	report, err := runtime.Doctor(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range report.Checks {
		if check.Name != "policy" {
			continue
		}
		if check.Status != doctor.CheckStatusFail {
			t.Fatalf("policy check = %+v, want fail", check)
		}
		if !strings.Contains(check.Detail, "Docker Engine VM") {
			t.Fatalf("policy detail = %q, want Docker bind guidance", check.Detail)
		}
		for _, call := range runner.outputs {
			if len(call.args) > 0 && call.args[0] == "run" {
				return
			}
		}
		t.Fatal("doctor did not probe the existing policy directory")
	}
	t.Fatal("doctor report did not contain a policy check")
}

func TestLoadStateRejectsLegacyAndTrailingDocuments(t *testing.T) {
	t.Parallel()
	for name, data := range map[string][]byte{
		"legacy":   []byte(`{"schema_version":1}`),
		"trailing": append(mustJSON(t, runtimeState(t.TempDir())), []byte("\n{}\n")...),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err := os.MkdirAll(filepath.Dir(runtime.statePath()), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runtime.statePath(), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runtime.LoadState(context.Background()); err == nil {
				t.Fatal("invalid state was accepted")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExecMapsCWDAndPreservesExactArgv(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	subdirectory := filepath.Join(root, "repository")
	if err := os.Mkdir(subdirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	instance := runtimeInstance(root)
	code, err := runtime.Exec(
		context.Background(), instance,
		tobari.ExecRequest{HostCWD: subdirectory, CWDExplicit: true, Command: []string{"printf", "%s", "a value"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	uid, gid := currentIDs()
	want := []string{
		"exec", "-i", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid), "--workdir", "/workspace/repository",
		"tobari-work", "printf", "%s", "a value",
	}
	if err != nil || code != 0 || len(runner.runs) != 1 || !slices.Equal(runner.runs[0].args, want) {
		t.Fatalf("Exec() code=%d err=%v calls=%v want=%v", code, err, runner.runs, want)
	}
}

func TestPolicyValidationUsesReadOnlyMountAndPolicyOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err := runtime.testPolicy(context.Background(), runtimeState(root)); err != nil {
		t.Fatal(err)
	}
	args := runner.outputs[0].args
	uid, gid := currentIDs()
	wantUser := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	if !slices.Contains(args, wantUser) || !slices.Contains(args, "type=bind,src="+filepath.Join(root, "policy")+",dst=/policy,readonly") {
		t.Fatalf("policy argv = %v", args)
	}
}

func TestClusterDownStopsBeforeCleanupWhenOwnershipInspectionFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &ownershipInspectFailureRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), runtimeState(root), false); err == nil {
		t.Fatal("ClusterDown() ignored an ownership inspection failure")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("cleanup ran after ownership inspection failure: %v", runner.runs)
	}
}

func TestClusterDownPurgesMissingVolumesIdempotently(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), runtimeState(root), true); err != nil {
		t.Fatalf("ClusterDown() = %v, want idempotent success for missing resources", err)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "volume" && slices.Contains(call.args, "rm") {
			t.Fatalf("missing volume was sent to rm: %v", call.args)
		}
	}
}

func TestPrepareStateUsesXDGPolicyAndEmptySchemaTwoCollection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	state, err := runtime.prepareState()
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 2 || state.Tobari == nil || len(state.Tobari) != 0 {
		t.Fatalf("state = %+v", state)
	}
	for path, want := range map[string]os.FileMode{
		state.PolicyDirectory: 0o700, filepath.Join(state.PolicyDirectory, "tobari.rego"): 0o600,
		state.CredentialDir: 0o700, state.CredentialConfig: 0o600,
		filepath.Join(root, "config", "config.json"): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode=%v err=%v want=%o", path, info.Mode().Perm(), err, want)
		}
	}
}

func TestPrepareStateRejectsSymlinkedManagementDirectoriesBeforeDocker(t *testing.T) {
	t.Parallel()
	for name, target := range map[string]string{
		"configuration": "config",
		"state":         "state",
		"data":          "data",
	} {
		name, target := name, target
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config")
			state := filepath.Join(root, "state")
			data := filepath.Join(root, "data")
			for _, path := range []string{config, state, data} {
				if path == filepath.Join(root, target) {
					continue
				}
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			outside := filepath.Join(root, "outside-"+target)
			if err := os.Mkdir(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, target)); err != nil {
				t.Fatal(err)
			}
			runner := &recordingRunner{}
			runtime, err := newRuntimeWithData(config, state, data, runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.prepareState(); err == nil {
				t.Fatal("prepareState() accepted a symlinked management directory")
			}
			if len(runner.outputs) != 0 || len(runner.runs) != 0 {
				t.Fatalf("Docker calls after unsafe directory = outputs %v runs %v", runner.outputs, runner.runs)
			}
		})
	}
}

func TestEnsurePrivateDirectoryTightensExistingDirectory(t *testing.T) {
	t.Parallel()
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestCredentialPermissionsRejectSymlinkedDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-credentials")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(config, "credentials")); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.checkCredentialPermissions(); err == nil {
		t.Fatal("checkCredentialPermissions() accepted a symlinked credentials directory")
	}
}

func TestSharedStateWriterIsAtomicAndSerialized(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	const writers = 16
	errs := make(chan error, writers)
	for index := 0; index < writers; index++ {
		index := index
		go func() {
			copy := state
			copy.RecentError = fmt.Sprintf("writer-%d", index)
			errs <- runtime.writeState(copy)
		}()
	}
	for range writers {
		if err := <-errs; err != nil {
			t.Fatalf("writeState() error = %v", err)
		}
	}
	loaded, exists, err := runtime.LoadState(context.Background())
	if err != nil || !exists || loaded.SchemaVersion != state.SchemaVersion {
		t.Fatalf("LoadState() = (%+v, %t, %v)", loaded, exists, err)
	}
	if _, err := os.Stat(filepath.Join(runtime.stateDirectory, "cluster.lock")); err != nil {
		t.Fatalf("cluster lock was not durable: %v", err)
	}
	entries, err := os.ReadDir(runtime.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary state file remains: %s", entry.Name())
		}
	}
}

func TestInterruptedClusterReconcileFailsClosedInStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	if err := runtime.startClusterReconcile(clusterOperationUp); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InspectCluster(context.Background(), state); err == nil {
		t.Fatal("InspectCluster() succeeded with an interrupted reconcile journal")
	}
	if err := runtime.clearClusterJournal(); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptedClusterReconcilePublishesExplicitRecoveryActions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.startClusterReconcile(clusterOperationDown); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.InspectCluster(context.Background(), runtimeState(root))
	structured, ok := fault.PublicCopy(err)
	if !ok || structured.Code != "cluster_reconcile_interrupted" || len(structured.NextActions) != 2 ||
		structured.NextActions[0].Command != "cluster up" || structured.NextActions[1].Command != "cluster down" {
		t.Fatalf("InspectCluster() fault = %+v, %v; want explicit cluster recovery actions", structured, err)
	}
	if err := runtime.clearClusterJournal(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveImageSelectorUsesExplicitThenXDGDefaultThenBuiltin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	got, err := runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != tobari.BuiltinImageSelector {
		t.Fatalf("missing config resolved %q, %v", got, err)
	}
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(config, "config.json"),
		[]byte("{\"version\":\"v1\",\"default_image\":\"workbench:dev\"}\n"), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != "workbench:dev" {
		t.Fatalf("configured default resolved %q, %v", got, err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "explicit:dev")
	if err != nil || got != "explicit:dev" {
		t.Fatalf("explicit selector resolved %q, %v", got, err)
	}
}

func TestResolveImageSelectorRejectsUnsafeOrMalformedConfig(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		data []byte
		mode os.FileMode
	}{
		"unknown field": {data: []byte(`{"version":"v1","default_image":"builtin","extra":true}`), mode: 0o600},
		"invalid image": {data: []byte(`{"version":"v1","default_image":"--pull=always"}`), mode: 0o600},
		"unsafe mode":   {data: []byte(`{"version":"v1","default_image":"builtin"}`), mode: 0o644},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config")
			if err := os.MkdirAll(config, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, "config.json"), test.data, test.mode); err != nil {
				t.Fatal(err)
			}
			runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
			if _, err := runtime.ResolveImageSelector(context.Background(), ""); err == nil {
				t.Fatal("invalid config was accepted")
			}
		})
	}
}

func TestCredentialConfigValidationRejectsPathEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":"v1","profiles":{"escaped":{"type":"bearer","hosts":["api.example.com"],"secret_file":"/run/tobari/credentials/../credentials.json"}}}`)
	if err := os.WriteFile(filepath.Join(config, "credentials.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _ := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	_, status := runtime.checkCredentialConfig()
	if status != doctor.CheckStatusFail {
		t.Fatalf("status = %q", status)
	}
}

func TestComposeEnvironmentUsesPinnedImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	environment, err := runtime.composeEnvironment(runtimeState(root))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, key := range []string{"TOBARI_MITMPROXY_IMAGE=", "TOBARI_OPA_IMAGE=", "TOBARI_DEBIAN_IMAGE="} {
		index := strings.LastIndex(joined, key)
		if index < 0 || !strings.Contains(joined[index:], "@sha256:") {
			t.Fatalf("%s is not digest pinned", key)
		}
	}
}

func TestBuildTobariImageTagsVersionAndStableExtensionBase(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	state := runtimeState(root)
	if err := runtime.buildTobariImage(context.Background(), state, os.Environ()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("build calls = %v", runner.runs)
	}
	args := runner.runs[0].args
	for _, tag := range []string{tobariImage(state), "tobari-runtime:local"} {
		found := false
		for index := 0; index+1 < len(args); index++ {
			if args[index] == "--tag" && args[index+1] == tag {
				found = true
			}
		}
		if !found {
			t.Errorf("build args %v lack tag %q", args, tag)
		}
	}
}

func TestAttachRejectsMissingImageBeforeCreatingResources(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	runner := &recordingRunner{outputErr: errors.New("No such image")}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	_, err := runtime.Attach(context.Background(), runtimeState(root), "work", root, "workbench:dev")
	if err == nil {
		t.Fatal("missing image was accepted")
	}
	if len(runner.outputs) != 1 || len(runner.outputs[0].args) < 2 ||
		runner.outputs[0].args[0] != "image" || runner.outputs[0].args[1] != "inspect" {
		t.Fatalf("Docker calls = %v", runner.outputs)
	}
}

func TestValidateCompatibleImageRequiresRuntimeContract(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		configuration string
		wantErr       bool
	}{
		"compatible": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
		},
		"unlabeled": {
			configuration: `{"api":"","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
		"missing lifetime command": {
			configuration: `{"api":"1","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
		"overridden entrypoint": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/bin/sh"]}`,
			wantErr:       true,
		},
		"overridden user": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"root","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{outputData: []byte(test.configuration)}
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			err := runtime.validateCompatibleImage(context.Background(), "workbench:dev")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCompatibleImage() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEnsureGatewayNetworkReconnectsAfterComposeReplacement(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		networks  string
		wantCalls int
	}{
		"already connected": {networks: `{"tobari-work-net":{}}`, wantCalls: 1},
		"reconnect":         {networks: `{"tobari-control":{}}`, wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &gatewayNetworkRunner{networks: test.networks}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureGatewayNetwork(context.Background(), "tobari-work-net"); err != nil {
				t.Fatal(err)
			}
			if len(runner.outputs) != test.wantCalls {
				t.Fatalf("Docker calls = %v, want %d", runner.outputs, test.wantCalls)
			}
			if test.wantCalls == 2 {
				want := []string{"network", "connect", "--alias", "gateway", "tobari-work-net", gatewayContainer}
				if !slices.Equal(runner.outputs[1].args, want) {
					t.Fatalf("reconnect argv = %v, want %v", runner.outputs[1].args, want)
				}
			}
		})
	}
}
