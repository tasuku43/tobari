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
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
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

type localBaseBuildRunner struct {
	runs    []runnerCall
	outputs []runnerCall
}

func (r *localBaseBuildRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}

func (r *localBaseBuildRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 4 && args[0] == "image" && args[1] == "inspect" && args[3] == "{{.Id}}" {
		return nil, errors.New("image not found")
	}
	return compatibleImageInspection(), nil
}

type ownershipInspectFailureRunner struct {
	outputs []runnerCall
	runs    []runnerCall
}

type interruptedClusterDownRunner struct {
	recordingRunner
	interrupted bool
}

func (r *interruptedClusterDownRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if !r.interrupted && len(args) > 0 && args[0] == "compose" && slices.Contains(args, "down") {
		r.interrupted = true
		return context.Canceled
	}
	return r.recordingRunner.Run(ctx, args, environment, input, output, errorOutput)
}

type interruptedClusterUpRunner struct {
	clusterUpProgressRunner
	interrupted bool
}

func (r *interruptedClusterUpRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if !r.interrupted && len(args) > 0 && args[0] == "compose" && slices.Contains(args, "up") {
		r.interrupted = true
		return context.Canceled
	}
	return r.clusterUpProgressRunner.Run(ctx, args, environment, input, output, errorOutput)
}

type policyProbeRunner struct {
	outputs []runnerCall
}

type clusterUpProgressRunner struct {
	events             []string
	composeEnvironment []string
	networkConnections []runnerCall
	policyQueries      []runnerCall
	companionEpoch     string
	composed           bool
}

func (r *clusterUpProgressRunner) Run(_ context.Context, args, environment []string, _ io.Reader, out, _ io.Writer) error {
	if len(args) >= 2 && args[0] == "container" && args[1] == "cp" {
		archive, err := exposureHelperArchiveBytes(syntheticExposureHelperELF("arm64"), "arm64", nil)
		if err != nil {
			return err
		}
		_, err = out.Write(archive)
		return err
	}
	if len(args) > 0 && args[0] == "compose" {
		r.events = append(r.events, "compose")
		r.composeEnvironment = append([]string{}, environment...)
		r.composed = true
	}
	if slices.Contains(args, "authbroker.control") {
		operationIndex := slices.Index(args, "authbroker.control") + 1
		operation := args[operationIndex]
		switch operation {
		case "unlock", "health":
			_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"unlocked"}`+"\n")
		case "companion_prepare":
			epochIndex := slices.Index(args, "--epoch-id")
			r.companionEpoch = args[epochIndex+1]
			_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":"prepared","epoch_id":%q}`+"\n", r.companionEpoch)
		case "companion_status":
			_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":"ready","epoch_id":%q}`+"\n", r.companionEpoch)
		}
	}
	return nil
}

func (r *clusterUpProgressRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer && args[2] == "/opa" {
		r.policyQueries = append(r.policyQueries, runnerCall{args: append([]string{}, args...)})
		if !r.composed {
			return nil, errors.New("OPA is not running")
		}
		return []byte("true"), nil
	}
	if len(args) > 0 && args[0] == "run" {
		return []byte("tobari-network-guard v1 gateway\n"), nil
	}
	if len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
		r.networkConnections = append(r.networkConnections, runnerCall{args: append([]string{}, args...)})
		return []byte{}, nil
	}
	if len(args) >= 2 && args[0] == "volume" && args[1] == "inspect" {
		return []byte(ownerValue + "\n"), nil
	}
	if len(args) >= 2 && args[0] == "volume" && args[1] == "create" {
		return []byte(policyBundleVolume + "\n"), nil
	}
	if len(args) >= 1 && args[0] == "image" {
		if strings.Contains(strings.Join(args, " "), tobari.RuntimeImageAPILabel) {
			return compatibleImageInspection(), nil
		}
		if strings.Contains(strings.Join(args, " "), exposureHelperAPILabel) {
			source, err := runtimeassets.ExposureHelperSourceVersion()
			if err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf(`{"architecture":"arm64","os":"linux","api":"1","source":%q}`, source)), nil
		}
		image := args[len(args)-1]
		switch image {
		case "tobari-auth-broker:dev":
			r.events = append(r.events, "auth-broker-image")
			return []byte(authBrokerMetadata("arm64", "")), nil
		case "tobari-gateway:dev":
			r.events = append(r.events, "gateway-image")
			return []byte(gatewayMetadata("arm64", "")), nil
		default:
			return nil, fmt.Errorf("unexpected shared image inspection: %s", image)
		}
	}
	if len(args) >= 1 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"arm64"}`), nil
	}
	if len(args) >= 3 && args[0] == "inspect" {
		if args[2] == "{{.Image}}" {
			return []byte("sha256:" + strings.Repeat("b", 64) + "\n"), nil
		}
		if strings.Contains(args[2], `"id"`) {
			uid, gid := currentIDs()
			return []byte(fmt.Sprintf(
				`{"id":"%s","owner":"default","component":"auth-broker","user":"%d:%d"}`,
				strings.Repeat("a", 64), uid, gid,
			)), nil
		}
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
	runner := &clusterUpProgressRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"),
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:dev",
		gateway:      sharedImageSelection{Image: "tobari-gateway:dev"},
		authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev"},
	}
	runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte{0x41}, 32), nil
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 32))
	var events []tobari.ClusterUpProgress
	state, err := runtime.ClusterUpWithProgress(context.Background(), func(event tobari.ClusterUpProgress) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSteps := []tobari.ClusterUpProgressStep{
		tobari.ClusterUpProgressPrepare,
		tobari.ClusterUpProgressPolicy,
		tobari.ClusterUpProgressPrepareImages,
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
	gatewayIndex := slices.Index(runner.events, "gateway-image")
	composeIndex := slices.Index(runner.events, "compose")
	validOrder := gatewayIndex >= 0 && composeIndex > gatewayIndex
	if brokerRuntimeEnabled {
		authIndex := slices.Index(runner.events, "auth-broker-image")
		validOrder = authIndex >= 0 && gatewayIndex > authIndex && composeIndex > gatewayIndex
	} else if slices.Contains(runner.events, "auth-broker-image") {
		validOrder = false
	}
	if !validOrder {
		t.Fatalf("shared image preparation order = %v", runner.events)
	}
	if len(runner.policyQueries) < 2 {
		t.Fatalf("cluster up policy readiness queries = %v", runner.policyQueries)
	}
	finalPolicyQuery := strings.Join(runner.policyQueries[len(runner.policyQueries)-1].args, "\n")
	if !strings.Contains(finalPolicyQuery, state.AggregateRevision) ||
		!strings.Contains(finalPolicyQuery, "/v1/data/tobari/http/decision") {
		t.Fatalf("cluster up final policy readiness query = %v", runner.policyQueries[len(runner.policyQueries)-1].args)
	}
	joinedEnvironment := strings.Join(runner.composeEnvironment, "\n")
	bindings := []string{"TOBARI_GATEWAY_IMAGE=tobari-gateway:dev"}
	if brokerRuntimeEnabled {
		bindings = append(bindings, "TOBARI_AUTH_BROKER_IMAGE=tobari-auth-broker:dev")
	}
	for _, binding := range bindings {
		if strings.Count(joinedEnvironment, binding) != 1 {
			t.Fatalf("compose environment lacks one verified %q binding: %s", binding, joinedEnvironment)
		}
	}
	wantNetworkConnections := [][]string{
		{"network", "connect", "--alias", "gateway", "tobari-control", gatewayContainer},
		{"network", "connect", "--alias", "opa", "tobari-control", opaContainer},
		{"network", "connect", "--alias", "gateway", "tobari-egress", gatewayContainer},
	}
	if brokerRuntimeEnabled {
		wantNetworkConnections = [][]string{
			{"network", "connect", "--alias", "gateway", "tobari-control", gatewayContainer},
			{"network", "connect", "--alias", "opa", "tobari-control", opaContainer},
			{"network", "connect", "--alias", "auth-broker", "tobari-control", authBrokerContainer},
			{"network", "connect", "--alias", "gateway", "tobari-egress", gatewayContainer},
			{"network", "connect", "--alias", "auth-broker", "tobari-egress", authBrokerContainer},
		}
	}
	if len(runner.networkConnections) != len(wantNetworkConnections) {
		t.Fatalf("shared network connections = %v", runner.networkConnections)
	}
	for index, want := range wantNetworkConnections {
		if !slices.Equal(runner.networkConnections[index].args, want) {
			t.Fatalf("shared network connection %d = %v, want %v", index, runner.networkConnections[index].args, want)
		}
	}
}

func TestClusterUpResumesAfterInterruptedComposeStartup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &interruptedClusterUpRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"),
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:dev",
		gateway:      sharedImageSelection{Image: "tobari-gateway:dev"},
		authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev"},
	}
	runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte{0x41}, 32), nil
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 32))

	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted ClusterUp() error = %v, want context cancellation", err)
	}
	journal, exists, err := runtime.readClusterJournal()
	if err != nil || !exists || journal.Operation != clusterOperationUp || journal.Phase != clusterPhaseStarted {
		t.Fatalf("interrupted journal = (%+v, %t, %v)", journal, exists, err)
	}
	if _, err := runtime.ClusterUp(context.Background()); err != nil {
		t.Fatalf("resumed ClusterUp() error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("cluster journal after resumed startup = exists:%t error:%v", exists, err)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || !exists {
		t.Fatalf("cluster state after resumed startup = exists:%t error:%v", exists, err)
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
		tobari.ClusterUpProgressPrepareImages,
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

func (r *recordingRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if slices.Contains(args, "authbroker.control") {
		_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"unlocked"}`+"\n")
	}
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
	if r.outputErr == nil && len(r.outputData) == 0 && len(args) >= 3 &&
		args[0] == "exec" && args[1] == opaContainer && args[2] == "/opa" {
		return []byte("true"), nil
	}
	if r.outputErr == nil && len(r.outputData) == 0 && len(args) > 0 &&
		(args[0] == "inspect" || (args[0] == "volume" && len(args) > 1 && args[1] == "inspect")) {
		return []byte(ownerValue + "\n"), nil
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
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1,
		PolicyDirectory: filepath.Join(root, "policy"),
		GatewayConfig:   filepath.Join(root, "gateway.json"), AssetVersion: "asset",
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
	projectRoot, err = runtime.ResolveProjectRoot(context.Background(), projectRoot)
	if err != nil {
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
			report, err := runRuntimeDoctor(context.Background(), runtime, candidate)
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

func TestDoctorDiagnosesUnsafeLearnedPolicyData(t *testing.T) {
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

	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	allowPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	if err := os.WriteFile(allowPath, []byte(`{"host":"api.github.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}

	report, err := runRuntimeDoctor(context.Background(), runtime, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range report.Checks {
		if check.Name != "policy_data" {
			continue
		}
		if check.Status != doctor.CheckStatusFail {
			t.Fatalf("policy_data check = %+v, want fail", check)
		}
		if !strings.Contains(check.Detail, "learned policy data is invalid or unsafe") ||
			!strings.Contains(check.Detail, "schema_version") {
			t.Fatalf("policy_data detail = %q", check.Detail)
		}
		return
	}
	t.Fatal("doctor report did not contain a policy_data check")
}

func TestLoadStateRejectsIncompleteAndTrailingDocuments(t *testing.T) {
	t.Parallel()
	for name, data := range map[string][]byte{
		"incomplete": []byte(`{"schema_version":1}`),
		"trailing":   append(mustJSON(t, runtimeState(t.TempDir())), []byte("\n{}\n")...),
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
	runtime.companion = &fakeCredentialCompanionLauncher{}
	if err := runtime.ClusterDown(context.Background(), runtimeState(root), true); err != nil {
		t.Fatalf("ClusterDown() = %v, want idempotent success for missing resources", err)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "volume" && slices.Contains(call.args, "rm") {
			t.Fatalf("missing volume was sent to rm: %v", call.args)
		}
	}
}

func TestClusterDownResumesAfterInterruptedComposeCleanup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &interruptedClusterDownRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), state, false); err == nil {
		t.Fatal("first ClusterDown() unexpectedly completed")
	}
	journal, exists, err := runtime.readClusterJournal()
	if err != nil || !exists || journal.Operation != clusterOperationDown || journal.Phase != clusterPhaseStarted {
		t.Fatalf("interrupted journal = (%+v, %t, %v)", journal, exists, err)
	}
	if err := runtime.ClusterDown(context.Background(), state, false); err != nil {
		t.Fatalf("resumed ClusterDown() error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("cluster journal after resumed cleanup = exists:%t error:%v", exists, err)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
		t.Fatalf("cluster state after resumed cleanup = exists:%t error:%v", exists, err)
	}
}

func TestPrepareStateUsesAggregateProjection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	state, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 1 || state.ManifestCount != 1 || state.AggregateRevision == "" {
		t.Fatalf("state = %+v", state)
	}
	for path, want := range map[string]os.FileMode{
		state.PolicyDirectory: 0o700, filepath.Join(state.PolicyDirectory, "router.rego"): 0o600,
		state.GatewayConfig:                                0o600,
		runtime.interactiveAttachmentDirectory():           0o700,
		runtime.interactiveAttachmentSessionRegistryPath(): 0o600,
		runtime.interactiveAttachmentSocketDirectory():     0o700,
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
			if _, err := runtime.prepareState(context.Background()); err == nil {
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

func TestResolveImageSelectorUsesInjectedResolverForBuiltin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:dev"}
	got, err := runtime.ResolveImageSelector(context.Background(), tobari.BuiltinImageSelector)
	if err != nil || got != "tobari-runtime:dev" {
		t.Fatalf("builtin selector resolved %q, %v", got, err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != "tobari-runtime:dev" {
		t.Fatalf("missing config resolved %q, %v", got, err)
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
	for _, key := range []string{"TOBARI_MITMPROXY_IMAGE=", "TOBARI_GATEWAY_IMAGE=", "TOBARI_OPA_IMAGE=", "TOBARI_DEBIAN_IMAGE="} {
		index := strings.LastIndex(joined, key)
		if index < 0 || !strings.Contains(joined[index:], "@sha256:") {
			t.Fatalf("%s is not digest pinned", key)
		}
	}
	if !strings.Contains(joined, "TOBARI_PRINCIPAL_DIR="+runtime.principalRegistryDirectory()) {
		t.Fatalf("compose environment does not expose the dedicated principal directory: %s", joined)
	}
	for _, binding := range []string{
		"TOBARI_INTERACTIVE_ATTACHMENT_DIR=" + runtime.interactiveAttachmentDirectory(),
		"TOBARI_PERMISSION_INGESTION_DIR=" + runtime.interactiveAttachmentSocketDirectory(),
	} {
		if !strings.Contains(joined, binding) {
			t.Fatalf("compose environment omits permission attachment binding %q: %s", binding, joined)
		}
	}
	authBindings := []string{"TOBARI_AUTH_PROVIDER_DIR=", "TOBARI_AUTH_CONTEXTS_DIR=", "TOBARI_AUTH_RUNTIME_DIR=", "TOBARI_AUTH_BROKER_IMAGE="}
	for _, binding := range authBindings {
		present := strings.Contains(joined, binding)
		if brokerRuntimeEnabled && !present {
			t.Fatalf("experimental compose environment omits %q", binding)
		}
		if !brokerRuntimeEnabled && present {
			t.Fatalf("standard compose environment exposes %q", binding)
		}
	}
	if strings.Contains(joined, "TOBARI_PRINCIPAL_CONFIG=") {
		t.Fatal("compose environment still exposes the single-file principal configuration")
	}
	if strings.Contains(joined, "TOBARI_AUTH_PROVIDER_CONFIG=") {
		t.Fatal("compose environment still exposes the single-file auth provider projection")
	}
}

func TestPermissionIngestionComposeProfileIsClosed(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	for _, test := range []struct {
		transport tobari.PermissionSessionTransport
		file      string
		hasSource bool
	}{
		{transport: tobari.PermissionSessionTransportUnix, file: "compose.permission-unix.yaml", hasSource: true},
		{transport: tobari.PermissionSessionTransportTCP, file: "compose.permission-loopback_tcp.yaml", hasSource: false},
	} {
		runtime.permissionIngestionTransport = test.transport
		args, err := runtime.permissionSessionComposeFileArgs("/runtime")
		if err != nil || len(args) != 2 || args[0] != "-f" || args[1] != filepath.Join("/runtime", test.file) {
			t.Fatalf("%s compose profile = %v, %v", test.transport, args, err)
		}
		environment, err := runtime.composeEnvironment(runtimeState(root))
		if err != nil {
			t.Fatal(err)
		}
		hasSource := strings.Contains(strings.Join(environment, "\n"), "TOBARI_PERMISSION_INGESTION_DIR=")
		if hasSource != test.hasSource {
			t.Fatalf("%s Unix source presence = %t", test.transport, hasSource)
		}
	}
	runtime.permissionIngestionTransport = "tcp"
	if _, err := runtime.permissionSessionComposeFileArgs("/runtime"); err == nil {
		t.Fatal("unsupported permission ingestion profile was accepted")
	}
}

func TestPrepareActiveContextImageReusesAndValidatesLocalOfficialRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: compatibleImageInspection()}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if runtime.defaultRuntimeImage() != localBaseRuntimeImage {
		t.Skip("local official base build is not selected by the development resolver")
	}
	initializeTestWorkspaceManifest(t, runtime)
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("runtime image mutation calls = %v", runner.runs)
	}
	if len(runner.outputs) != 2 || runner.outputs[0].args[0] != "image" || runner.outputs[0].args[1] != "inspect" {
		t.Fatalf("runtime image inspect calls = %v", runner.outputs)
	}
}

func TestPrepareActiveContextImageBuildsMissingLocalOfficialRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &localBaseBuildRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if runtime.defaultRuntimeImage() != localBaseRuntimeImage {
		t.Skip("local official base build is not selected by the development resolver")
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 || !slices.Equal(runner.runs[0].args[:4], []string{"buildx", "build", "--progress=plain", "--load"}) {
		t.Fatalf("runtime image build calls = %v", runner.runs)
	}
	if !containsArgs(runner.runs[0].args, "--tag") || !containsArgs(runner.runs[0].args, localBaseRuntimeImage) ||
		!containsArgs(runner.runs[0].args, "--build-arg") || !strings.Contains(strings.Join(runner.runs[0].args, "\n"), "TOBARI_UID=") {
		t.Fatalf("runtime image build argv = %v", runner.runs[0].args)
	}
	if len(runner.outputs) != 2 {
		t.Fatalf("runtime image inspection calls = %v", runner.outputs)
	}
}

func TestPrepareActiveContextImageDoesNotPullInjectedLocalRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: compatibleImageInspection()}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:dev"}
	initializeTestWorkspaceManifest(t, runtime)
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("local runtime image was pulled: %v", runner.runs)
	}
	if len(runner.outputs) != 1 || runner.outputs[0].args[len(runner.outputs[0].args)-1] != "tobari-runtime:dev" {
		t.Fatalf("runtime image inspect calls = %v", runner.outputs)
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

func TestEnsureAuthBrokerNetworkReconnectsAfterComposeReplacement(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		networks  string
		wantCalls int
	}{
		"already connected": {networks: `{"tobari-control":{}}`, wantCalls: 1},
		"reconnect":         {networks: `{}`, wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &gatewayNetworkRunner{networks: test.networks}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureAuthBrokerNetwork(context.Background(), "tobari-control"); err != nil {
				t.Fatal(err)
			}
			if len(runner.outputs) != test.wantCalls {
				t.Fatalf("Docker calls = %v, want %d", runner.outputs, test.wantCalls)
			}
			if test.wantCalls == 2 {
				want := []string{"network", "connect", "--alias", "auth-broker", "tobari-control", authBrokerContainer}
				if !slices.Equal(runner.outputs[1].args, want) {
					t.Fatalf("reconnect argv = %v, want %v", runner.outputs[1].args, want)
				}
			}
		})
	}
}
