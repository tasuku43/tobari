package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type contextSwitchRunner struct {
	running     bool
	failCompose bool
	runs        []runnerCall
}

func (r *contextSwitchRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if r.failCompose && len(args) > 0 && args[0] == "compose" {
		return errors.New("synthetic compose failure")
	}
	return nil
}

func (r *contextSwitchRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) > 0 && args[0] == "image" {
		if strings.Contains(strings.Join(args, " "), tobari.RuntimeImageAPILabel) {
			return compatibleImageInspection(), nil
		}
		versions, err := runtimeassets.Versions()
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf(`{"RepoDigests":[%q],"Architecture":"arm64","Os":"linux","Config":{"User":"1000:1000","Labels":{"io.tobari.gateway-api":"1","io.tobari.gateway-role":"enforcement"},"Entrypoint":["/opt/tobari/entrypoint.sh"]}}`, versions["GATEWAY_IMAGE"])), nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"arm64"}`), nil
	}
	if len(args) >= 3 && args[0] == "inspect" {
		if strings.Contains(args[2], "NetworkSettings.Networks") {
			return []byte(`{}`), nil
		}
		if r.running {
			return []byte(`{"state":"running","health":"healthy"}`), nil
		}
		return []byte(`{"state":"exited","health":"none"}`), nil
	}
	return []byte{}, nil
}

func newContextSwitchRuntime(t *testing.T, runner *contextSwitchRunner) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "project-tools", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced); err != nil {
		t.Fatal(err)
	}
	return runtime
}

func contextSwitchState(runtime *Runtime, name string, root string) tobari.State {
	paths := runtime.contextPaths(name)
	state := runtimeState(root)
	state.ContextName = name
	state.AgentProfile = tobari.DefaultProfile
	state.PolicyDirectory = paths.PolicyDirectory
	state.CredentialConfig = paths.CredentialConfig
	state.CredentialDir = paths.CredentialDirectory
	return state
}

func TestUseContextReportsUnconfiguredAndStoppedStatesWithoutStartingDocker(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		runner *contextSwitchRunner
		want   tobari.ContextClusterStatus
	}{
		{name: "unconfigured", runner: &contextSwitchRunner{}, want: tobari.ContextClusterStatusNotConfigured},
		{name: "stopped", runner: &contextSwitchRunner{}, want: tobari.ContextClusterStatusNotRunning},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newContextSwitchRuntime(t, test.runner)
			if test.name == "stopped" {
				state := contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())
				if err := runtime.writeState(state); err != nil {
					t.Fatal(err)
				}
			}
			result, err := runtime.UseContext(context.Background(), "project-tools")
			if err != nil {
				t.Fatalf("UseContext() error = %v", err)
			}
			if result.Cluster != test.want || !result.Active || result.Name != "project-tools" {
				t.Fatalf("UseContext() result = %+v, want cluster %q", result, test.want)
			}
			if len(test.runner.runs) != 0 {
				t.Fatalf("Docker Run calls = %+v, want none", test.runner.runs)
			}
		})
	}
}

func TestUseContextReportsAlreadyReadyWithoutReconcile(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true}
	runtime := newContextSwitchRuntime(t, runner)
	if err := runtime.writeState(contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.UseContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	if result.Cluster != tobari.ContextClusterStatusAlreadyReady {
		t.Fatalf("cluster status = %q, want already_ready", result.Cluster)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker Run calls = %+v, want none", runner.runs)
	}
}

func TestUseContextReconcilesRunningClusterAndPersistsSelectedContext(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true}
	runtime := newContextSwitchRuntime(t, runner)
	if err := runtime.writeState(contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.UseContext(context.Background(), "project-tools")
	if err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	if result.Cluster != tobari.ContextClusterStatusReconciled {
		t.Fatalf("cluster status = %q, want reconciled", result.Cluster)
	}
	state, configured, err := runtime.LoadState(context.Background())
	if err != nil || !configured {
		t.Fatalf("LoadState() = %+v, %t, %v", state, configured, err)
	}
	if state.ContextName != "project-tools" || state.PolicyDirectory != runtime.contextPaths("project-tools").PolicyDirectory {
		t.Fatalf("persisted state = %+v", state)
	}
	active, err := runtime.readActiveContext()
	if err != nil || active != "project-tools" {
		t.Fatalf("active Context = %q, error = %v", active, err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("reconcile journal = exists:%t error:%v", exists, err)
	}
	foundCompose := false
	forceRecreate := false
	for _, call := range runner.runs {
		if len(call.args) > 0 && call.args[0] == "compose" {
			foundCompose = true
			for _, arg := range call.args {
				if arg == "--force-recreate" {
					forceRecreate = true
				}
			}
		}
	}
	if !foundCompose {
		t.Fatalf("Docker calls = %+v, want compose reconciliation", runner.runs)
	}
	if !forceRecreate {
		t.Fatalf("Context switch compose call = %+v, want --force-recreate", runner.runs)
	}
}

func TestUseContextRestoresPreviousStateAndBlocksUntilExplicitRecoveryAfterFailure(t *testing.T) {
	t.Parallel()
	runner := &contextSwitchRunner{running: true, failCompose: true}
	runtime := newContextSwitchRuntime(t, runner)
	previous := contextSwitchState(runtime, tobari.DefaultContextName, t.TempDir())
	if err := runtime.writeState(previous); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UseContext(context.Background(), "project-tools"); err == nil {
		t.Fatal("UseContext() succeeded despite synthetic compose failure")
	}
	active, err := runtime.readActiveContext()
	if err != nil || active != tobari.DefaultContextName {
		t.Fatalf("active Context after failure = %q, error = %v", active, err)
	}
	state, configured, err := runtime.LoadState(context.Background())
	if err != nil || !configured || state.ContextName != tobari.DefaultContextName {
		t.Fatalf("state after failure = %+v, configured=%t, error=%v", state, configured, err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
		t.Fatalf("reconcile journal after failure = exists:%t error:%v", exists, err)
	}
	if _, err := runtime.UseContext(context.Background(), tobari.DefaultContextName); err == nil {
		t.Fatal("UseContext() ignored the interrupted reconcile journal")
	}
	runner.failCompose = false
	if _, err := runtime.ClusterUp(context.Background()); err != nil {
		t.Fatalf("explicit ClusterUp() recovery error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("reconcile journal after recovery = exists:%t error:%v", exists, err)
	}
}

func TestContextStoreMigratesLegacyStoresAndPersistsRuntimeImage(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(filepath.Join(config, "policy"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(config, "credentials"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"legacy-runtime:dev"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	legacyPolicy := []byte("package tobari\n\nallow := false\n")
	if err := os.WriteFile(filepath.Join(config, "policy", "tobari.rego"), legacyPolicy, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyCredentials := []byte(`{"version":"v1","profiles":{}}`)
	if err := os.WriteFile(filepath.Join(config, "credentials.json"), legacyCredentials, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "credentials", "legacy-token"), []byte("synthetic-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := runtime.ListContexts(context.Background())
	if err != nil {
		t.Fatalf("ListContexts() error = %v", err)
	}
	if contexts.Active != tobari.DefaultContextName || len(contexts.Items) != 1 || contexts.Items[0].Image != "legacy-runtime:dev" {
		t.Fatalf("initial Contexts = %+v", contexts)
	}

	defaultPolicy := filepath.Join(config, "contexts", "default", "policy", "tobari.rego")
	data, err := os.ReadFile(defaultPolicy)
	if err != nil || string(data) != string(legacyPolicy) {
		t.Fatalf("migrated policy = %q, error = %v", data, err)
	}
	migratedCredentials, err := os.ReadFile(filepath.Join(config, "contexts", "default", "credentials", "legacy-token"))
	if err != nil || string(migratedCredentials) != "synthetic-secret" {
		t.Fatalf("migrated credential = %q, error = %v", migratedCredentials, err)
	}
	if _, err := os.Stat(filepath.Join(config, "policy", "tobari.rego")); err != nil {
		t.Fatalf("legacy policy was removed: %v", err)
	}

	created, err := runtime.CreateContext(context.Background(), "project-tools", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced)
	if err != nil {
		t.Fatalf("CreateContext() error = %v", err)
	}
	if created.Image != tobari.OfficialRuntimeBase || created.PolicyMode != tobari.ContextPolicyModeAdvanced {
		t.Fatalf("created Context = %+v", created)
	}
	if _, err := runtime.UseContext(context.Background(), "project-tools"); err != nil {
		t.Fatalf("UseContext() error = %v", err)
	}
	shown, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatalf("ShowContext() error = %v", err)
	}
	if !shown.Active || shown.Name != "project-tools" || shown.Image != tobari.OfficialRuntimeBase {
		t.Fatalf("active Context = %+v", shown)
	}
	manifestData, err := os.ReadFile(filepath.Join(config, "contexts", "project-tools", "context.json"))
	if err != nil || strings.Contains(string(manifestData), "synthetic-secret") {
		t.Fatalf("manifest contains credential material or could not be read: %q, %v", manifestData, err)
	}
	for _, path := range []string{
		filepath.Join(config, "contexts", "project-tools"),
		filepath.Join(config, "contexts", "project-tools", "policy"),
		filepath.Join(config, "contexts", "project-tools", "credentials"),
		filepath.Join(config, "contexts", "project-tools", "context.json"),
		filepath.Join(config, "contexts", "active.json"),
	} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("Context path %s is not owner-only: %o", path, info.Mode().Perm())
		}
	}
}

func TestContextImageBecomesProjectDefaultAndOutlivesLegacyConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "config")
	if err := os.MkdirAll(config, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"initial:dev"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntime(config, filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.UseContext(context.Background(), tobari.DefaultContextName); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config, "config.json"), []byte(`{"version":"v1","default_image":"--invalid"}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	shown, err := runtime.ShowContext(context.Background(), "")
	if err != nil || shown.Image != "initial:dev" {
		t.Fatalf("Context after legacy config change = %+v, error = %v", shown, err)
	}

	image, err := runtime.resolveContextImage(context.Background())
	if err != nil || image != "initial:dev" {
		t.Fatalf("resolveContextImage() = %q, error = %v", image, err)
	}
}
