package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type appliedStateInspectRunner struct {
	payloads map[string][][]byte
	errors   map[string]error
	stderr   map[string][]byte
	calls    []string
}

func (r *appliedStateInspectRunner) Run(
	_ context.Context, args, _ []string, _ io.Reader, output, errorOutput io.Writer,
) error {
	if len(args) < 4 || args[0] != "inspect" || args[2] != appliedClusterInspectTemplate {
		return errors.New("unexpected applied-state command")
	}
	container := args[len(args)-1]
	r.calls = append(r.calls, container)
	if err := r.errors[container]; err != nil {
		diagnostic := r.stderr[container]
		if len(diagnostic) == 0 {
			diagnostic = []byte("synthetic inspect failure")
		}
		_, _ = errorOutput.Write(diagnostic)
		return err
	}
	queue := r.payloads[container]
	if len(queue) == 0 {
		diagnostic := r.stderr[container]
		if len(diagnostic) == 0 {
			diagnostic = []byte("Error: No such object: " + container + "\n")
		}
		_, _ = errorOutput.Write(diagnostic)
		return errors.New("No such object")
	}
	payload := queue[0]
	r.payloads[container] = queue[1:]
	if payload == nil {
		_, _ = errorOutput.Write([]byte("Error: No such object: " + container + "\n"))
		return errors.New("No such object")
	}
	_, err := output.Write(payload)
	return err
}

func TestAppliedStateMissingRequiresExactInspectDiagnostic(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		diagnostic  string
		wantMissing bool
	}{
		{name: "exact", diagnostic: "Error: No such object: " + authBrokerContainer + "\n", wantMissing: true},
		{name: "prefix", diagnostic: "wrapper: Error: No such object: " + authBrokerContainer},
		{name: "suffix", diagnostic: "Error: No such object: " + authBrokerContainer + ": daemon unavailable"},
		{name: "multiline", diagnostic: "Error: No such object: " + authBrokerContainer + "\ntransport failed"},
		{name: "unrelated", diagnostic: "Error: network not found while inspecting " + authBrokerContainer},
		{name: "other container", diagnostic: "Error: No such object: foreign-container"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &appliedStateInspectRunner{
				payloads: map[string][][]byte{},
				stderr:   map[string][]byte{authBrokerContainer: []byte(test.diagnostic)},
			}
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			_, missing, err := runtime.observeAppliedClusterComponent(context.Background(), "auth-broker", authBrokerContainer)
			if test.wantMissing {
				if err != nil || !missing {
					t.Fatalf("exact missing = %t, %v", missing, err)
				}
			} else if err == nil || missing {
				t.Fatalf("ambiguous missing = %t, %v", missing, err)
			}
		})
	}
}

func (*appliedStateInspectRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("unbounded Output must not observe applied state")
}

func appliedStateObservation(container, image string) appliedClusterComponentObservation {
	component, role := "", ""
	switch container {
	case gatewayContainer:
		component, role = "gateway", gatewayRole
	case opaContainer:
		component = "opa"
	case authBrokerContainer:
		component = "auth-broker"
	}
	return appliedClusterComponentObservation{
		ContainerID: strings.Repeat("9", 64), Owner: ownerValue,
		Component: component, Role: role, ImageID: image,
		State: "running", Health: "healthy", Environment: []string{}, MountDestinations: []string{},
	}
}

func appliedStatePayload(t *testing.T, observation appliedClusterComponentObservation) []byte {
	t.Helper()
	data, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func appliedStateSnapshotRunner(t *testing.T) *appliedStateInspectRunner {
	t.Helper()
	gateway := appliedStatePayload(t, appliedStateObservation(gatewayContainer, "sha256:"+strings.Repeat("a", 64)))
	opa := appliedStatePayload(t, appliedStateObservation(opaContainer, "sha256:"+strings.Repeat("b", 64)))
	runner := &appliedStateInspectRunner{payloads: map[string][][]byte{
		gatewayContainer: {gateway, gateway},
		opaContainer:     {opa, opa},
	}}
	if brokerRuntimeEnabled {
		broker := appliedStatePayload(t, appliedStateObservation(authBrokerContainer, "sha256:"+strings.Repeat("c", 64)))
		runner.payloads[authBrokerContainer] = [][]byte{broker, broker}
	}
	return runner
}

func TestAppliedStateSnapshotUsesBoundedRealInspectIdentity(t *testing.T) {
	t.Parallel()
	runner := appliedStateSnapshotRunner(t)
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := runtime.observeAppliedClusterSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.gateway.ContainerID != strings.Repeat("9", 64) ||
		snapshot.gateway.ImageID != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("real Docker identity shapes were not retained: %+v", snapshot.gateway)
	}
	wantCalls := 6
	if len(runner.calls) != wantCalls {
		t.Fatalf("bounded inspect calls = %d, want %d", len(runner.calls), wantCalls)
	}
	if _, err := runner.Output(context.Background(), nil, nil); err == nil {
		t.Fatal("test runner allowed unbounded applied-state Output")
	}
}

func TestStandardAppliedStateFencesBrokerAbsenceTwice(t *testing.T) {
	t.Parallel()
	if brokerRuntimeEnabled {
		t.Skip("research build fences a present Broker in both tuple passes")
	}
	broker := appliedStatePayload(t, appliedStateObservation(authBrokerContainer, "sha256:"+strings.Repeat("c", 64)))
	for _, test := range []struct {
		name     string
		payloads [][]byte
		stderr   []byte
	}{
		{name: "appeared between passes", payloads: [][]byte{nil, broker}},
		{name: "present both", payloads: [][]byte{broker, broker}},
		{name: "ambiguous missing", stderr: []byte("wrapper: Error: No such object: " + authBrokerContainer)},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := appliedStateSnapshotRunner(t)
			if test.payloads != nil {
				runner.payloads[authBrokerContainer] = test.payloads
			}
			if test.stderr != nil {
				runner.stderr = map[string][]byte{authBrokerContainer: test.stderr}
			}
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.observeAppliedClusterSnapshot(context.Background()); err == nil {
				t.Fatal("standard applied snapshot accepted unfenced Broker absence")
			}
		})
	}
}

func TestAppliedStateSnapshotFencesEveryStableComponentField(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		container string
		mutate    func(*appliedClusterComponentObservation)
	}{
		{name: "Gateway container", container: gatewayContainer, mutate: func(value *appliedClusterComponentObservation) { value.ContainerID = strings.Repeat("8", 64) }},
		{name: "Gateway image", container: gatewayContainer, mutate: func(value *appliedClusterComponentObservation) { value.ImageID = "sha256:" + strings.Repeat("8", 64) }},
		{name: "Gateway environment", container: gatewayContainer, mutate: func(value *appliedClusterComponentObservation) { value.Environment = []string{"TOBARI_TEST=drift"} }},
		{name: "Gateway mounts", container: gatewayContainer, mutate: func(value *appliedClusterComponentObservation) { value.MountDestinations = []string{"/run/drift"} }},
		{name: "OPA container", container: opaContainer, mutate: func(value *appliedClusterComponentObservation) { value.ContainerID = strings.Repeat("7", 64) }},
		{name: "OPA image", container: opaContainer, mutate: func(value *appliedClusterComponentObservation) { value.ImageID = "sha256:" + strings.Repeat("7", 64) }},
	}
	if brokerRuntimeEnabled {
		tests = append(tests,
			struct {
				name      string
				container string
				mutate    func(*appliedClusterComponentObservation)
			}{name: "Broker container", container: authBrokerContainer, mutate: func(value *appliedClusterComponentObservation) { value.ContainerID = strings.Repeat("6", 64) }},
			struct {
				name      string
				container string
				mutate    func(*appliedClusterComponentObservation)
			}{name: "Broker image", container: authBrokerContainer, mutate: func(value *appliedClusterComponentObservation) { value.ImageID = "sha256:" + strings.Repeat("6", 64) }},
		)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := appliedStateSnapshotRunner(t)
			var observation appliedClusterComponentObservation
			if err := json.Unmarshal(runner.payloads[test.container][1], &observation); err != nil {
				t.Fatal(err)
			}
			test.mutate(&observation)
			runner.payloads[test.container][1] = appliedStatePayload(t, observation)
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.observeAppliedClusterSnapshot(context.Background()); err == nil {
				t.Fatal("one-field tuple drift passed the second observation fence")
			}
		})
	}
}

func TestAppliedStateObservationRejectsHostileFrames(t *testing.T) {
	t.Parallel()
	valid := appliedStatePayload(t, appliedStateObservation(gatewayContainer, "sha256:"+strings.Repeat("a", 64)))
	stopped := bytes.Replace(valid, []byte(`"state":"running"`), []byte(`"state":"exited"`), 1)
	wrongRole := bytes.Replace(valid, []byte(`"role":"enforcement"`), []byte(`"role":"observer"`), 1)
	prefixedContainerID := bytes.Replace(valid, []byte(`"container_id":"`), []byte(`"container_id":"sha256:`), 1)
	for _, test := range []struct {
		name    string
		payload []byte
		err     error
	}{
		{name: "valid whitespace", payload: append(append([]byte{}, valid...), []byte(" \n\t")...)},
		{name: "second value", payload: append(append([]byte{}, valid...), []byte(" {}")...)},
		{name: "malformed tail", payload: append(append([]byte{}, valid...), []byte(" {")...)},
		{name: "duplicate key", payload: bytes.Replace(valid, []byte(`"owner":`), []byte(`"owner":"tobari","owner":`), 1)},
		{name: "overflow", payload: bytes.Repeat([]byte("x"), appliedClusterInspectLimit+1)},
		{name: "stopped", payload: stopped},
		{name: "wrong role", payload: wrongRole},
		{name: "prefixed container ID", payload: prefixedContainerID},
		{name: "inspect error", err: errors.New("permission denied")},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &appliedStateInspectRunner{
				payloads: map[string][][]byte{gatewayContainer: {test.payload}},
				errors:   map[string]error{gatewayContainer: test.err},
			}
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			_, _, gotErr := runtime.observeAppliedClusterComponent(context.Background(), "gateway", gatewayContainer)
			if test.name == "valid whitespace" {
				if gotErr != nil {
					t.Fatal(gotErr)
				}
			} else if gotErr == nil {
				t.Fatal("hostile Docker observation was accepted")
			}
		})
	}
}

func TestSchemaOneMigrationRejectsUnprovenComponentTuple(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*appliedStateInspectRunner)
	}{
		{name: "missing Gateway", mutate: func(runner *appliedStateInspectRunner) { runner.payloads[gatewayContainer] = nil }},
		{name: "stopped OPA", mutate: func(runner *appliedStateInspectRunner) {
			var observation appliedClusterComponentObservation
			_ = json.Unmarshal(runner.payloads[opaContainer][0], &observation)
			observation.State = "exited"
			runner.payloads[opaContainer][0], _ = json.Marshal(observation)
		}},
		{name: "wrong Gateway role", mutate: func(runner *appliedStateInspectRunner) {
			var observation appliedClusterComponentObservation
			_ = json.Unmarshal(runner.payloads[gatewayContainer][0], &observation)
			observation.Role = "observer"
			runner.payloads[gatewayContainer][0], _ = json.Marshal(observation)
		}},
		{name: "replaced OPA", mutate: func(runner *appliedStateInspectRunner) {
			var observation appliedClusterComponentObservation
			_ = json.Unmarshal(runner.payloads[opaContainer][1], &observation)
			observation.ContainerID = strings.Repeat("1", 64)
			runner.payloads[opaContainer][1], _ = json.Marshal(observation)
		}},
		{name: "overflow Gateway", mutate: func(runner *appliedStateInspectRunner) {
			runner.payloads[gatewayContainer][0] = bytes.Repeat([]byte("x"), appliedClusterInspectLimit+1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, legacy, runner := schemaOneMigrationFixture(t)
			test.mutate(runner)
			if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); err == nil {
				t.Fatal("unproven predecessor tuple was migrated")
			}
		})
	}
}

func materializeReviewedPrePlatformRuntime(t *testing.T, stateDirectory string) string {
	t.Helper()
	directory := filepath.Join(stateDirectory, "runtime", prePlatformAssetVersion)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "compose.pre-platform.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "compose.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if brokerRuntimeEnabled {
		experimental, err := os.ReadFile(filepath.Join("testdata", "compose.pre-platform.experimental.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "compose.experimental.yaml"), experimental, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return directory
}

func schemaOneMigrationFixture(t *testing.T) (*Runtime, tobari.State, *appliedStateInspectRunner) {
	t.Helper()
	root := t.TempDir()
	stateDirectory := filepath.Join(root, "state")
	runner := appliedStateSnapshotRunner(t)
	runtime, err := newRuntime(filepath.Join(root, "config"), stateDirectory, runner)
	if err != nil {
		t.Fatal(err)
	}
	legacy := runtimeState(root)
	legacy.AssetVersion = prePlatformAssetVersion
	legacy.RuntimeDirectory = materializeReviewedPrePlatformRuntime(t, stateDirectory)
	return runtime, legacy, runner
}

func TestSchemaOneMigrationRequiresReviewedRuntimeAndHealthyTuple(t *testing.T) {
	t.Parallel()
	runtime, legacy, runner := schemaOneMigrationFixture(t)
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(runtime.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(`"applied"`)) {
		t.Fatalf("schema-1 state write exposed successor key: %s", raw)
	}
	migrated, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.SchemaVersion != 2 || migrated.Applied.PermissionProfile != tobari.SharedClusterProfilePrePlatform ||
		migrated.Applied.GatewayImageID != "sha256:"+strings.Repeat("a", 64) ||
		migrated.Applied.OPAImageID != "sha256:"+strings.Repeat("b", 64) {
		t.Fatalf("migrated state = %+v", migrated)
	}
	if len(runner.calls) != 6 {
		t.Fatalf("migration inspect calls = %d, want two fenced G/O/B observations", len(runner.calls))
	}
}

func TestSchemaOneMigrationRejectsUnsafeRuntimeAuthority(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *Runtime, *tobari.State)
	}{
		{name: "traversal", mutate: func(_ *testing.T, _ *Runtime, state *tobari.State) {
			state.RuntimeDirectory += "/../" + prePlatformAssetVersion
		}},
		{name: "broad runtime mode", mutate: func(t *testing.T, _ *Runtime, state *tobari.State) {
			if err := os.Chmod(state.RuntimeDirectory, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong asset bytes", mutate: func(t *testing.T, _ *Runtime, state *tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), bytes.Repeat([]byte("x"), prePlatformComposeSize), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "oversize asset", mutate: func(t *testing.T, _ *Runtime, state *tobari.State) {
			file, err := os.OpenFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = file.Write([]byte("x")); err != nil {
				_ = file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked runtime parent", mutate: func(t *testing.T, _ *Runtime, state *tobari.State) {
			parent := filepath.Dir(state.RuntimeDirectory)
			target := parent + "-real"
			if err := os.Rename(parent, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, parent); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "wrong version", mutate: func(_ *testing.T, _ *Runtime, state *tobari.State) { state.AssetVersion = "0000000000000000" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, legacy, _ := schemaOneMigrationFixture(t)
			test.mutate(t, runtime, &legacy)
			if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); err == nil {
				t.Fatal("unsafe predecessor runtime was accepted")
			}
		})
	}
}

func TestSchemaOneMigrationBindsExactBuildProfile(t *testing.T) {
	t.Parallel()
	if !brokerRuntimeEnabled {
		runtime, legacy, runner := schemaOneMigrationFixture(t)
		var gateway appliedClusterComponentObservation
		if err := json.Unmarshal(runner.payloads[gatewayContainer][0], &gateway); err != nil {
			t.Fatal(err)
		}
		gateway.Environment = []string{"TOBARI_AUTH_BROKER_SOCKET=/run/tobari-auth/runtime/broker.sock"}
		payload := appliedStatePayload(t, gateway)
		runner.payloads[gatewayContainer] = [][]byte{payload, payload}
		if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); err == nil {
			t.Fatal("standard migration accepted research Gateway projection")
		}
		return
	}
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "missing", mutate: func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered", mutate: func(t *testing.T, path string) {
			if err := os.WriteFile(path, bytes.Repeat([]byte("x"), prePlatformExperimentalComposeSize), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broad mode", mutate: func(t *testing.T, path string) {
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", mutate: func(t *testing.T, path string) {
			target := path + ".real"
			if err := os.Rename(path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, legacy, _ := schemaOneMigrationFixture(t)
			test.mutate(t, filepath.Join(legacy.RuntimeDirectory, "compose.experimental.yaml"))
			if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); err == nil {
				t.Fatal("research migration accepted unsafe experimental overlay")
			}
		})
	}
}

func TestSchemaOneMigrationPostRenameErrorLeavesExactSchemaTwo(t *testing.T) {
	t.Parallel()
	runtime, legacy, _ := schemaOneMigrationFixture(t)
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("post-rename fsync failed")
	runtime.clusterStateWriteHook = func(_ tobari.State, commit func() error) error {
		if err := commit(); err != nil {
			return err
		}
		return injected
	}
	if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); !errors.Is(err, injected) {
		t.Fatalf("migration error = %v", err)
	}
	loaded, exists, err := runtime.LoadState(context.Background())
	if err != nil || !exists || loaded.SchemaVersion != 2 || loaded.Applied.PermissionProfile != tobari.SharedClusterProfilePrePlatform {
		t.Fatalf("committed migration = %+v exists:%t error:%v", loaded, exists, err)
	}
}
