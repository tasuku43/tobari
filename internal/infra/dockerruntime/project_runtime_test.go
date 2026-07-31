package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func projectRuntimeInstance(t *testing.T, runtime *Runtime) tobari.ProjectInstance {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func TestInspectProjectRuntimeClassifiesDockerUnreachable(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: errors.New("Docker daemon unavailable")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	diagnostic, err := runtime.InspectProjectRuntime(context.Background(), instance)
	if err != nil || diagnostic != tobari.RuntimeDiagnosticUnreachable {
		t.Fatalf("InspectProjectRuntime() = (%q, %v)", diagnostic, err)
	}
}

func TestEnterProjectRuntimeMirrorsHostCWDPath(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	nested := filepath.Join(instance.Root, "root")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.EnterProjectRuntime(context.Background(), instance, nested, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 {
		t.Fatalf("EnterProjectRuntime() run count = %d, want 1", len(runner.runs))
	}
	want := "/workspace" + nested
	for index, arg := range runner.runs[0].args {
		if arg == "--workdir" {
			if index+1 >= len(runner.runs[0].args) || runner.runs[0].args[index+1] != want {
				t.Fatalf("EnterProjectRuntime() workdir = %q, want %q", runner.runs[0].args[index+1], want)
			}
			return
		}
	}
	t.Fatalf("EnterProjectRuntime() args = %v, missing --workdir", runner.runs[0].args)
}

func TestDeleteProjectRemovesLogicalStateWhenRuntimeResourcesAreMissing(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	home, err := runtime.ProjectHome(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.DeleteProject(context.Background(), instance); err != nil {
		t.Fatalf("DeleteProject() error = %v", err)
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("project home stat error = %v, want removed", err)
	}
	if _, found, err := runtime.ResolveProject(context.Background(), instance.Root); err != nil || found {
		t.Fatalf("ResolveProject() = found=%t err=%v, want not found", found, err)
	}
}

func TestEnsureProjectAgentStateMergesSharedAndLocalSettings(t *testing.T) {
	t.Parallel()
	runtime := newProjectStateRuntime(t)
	profile, err := runtime.ensureSharedProfile(tobari.DefaultProfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "common", "settings.json"), []byte(`{"shared":true,"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	if err := runtime.ensureProjectAgentState(instance.ID, profile); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(runtime.projectHomePath(instance.ID), ".claude", "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"theme":"light","local":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureProjectAgentState(instance.ID, profile); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["shared"] != true || got["theme"] != "light" || got["local"] != true {
		t.Fatalf("merged settings = %v", got)
	}
}

type projectReconcileRunner struct {
	failOn          func([]string) bool
	networkExists   bool
	containerExists bool
}

type projectSpecDriftRunner struct {
	instanceID string
	stale      bool
	calls      [][]string
}

func (r *projectSpecDriftRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectSpecDriftRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{}, args...))
	if len(args) == 0 {
		return nil, errors.New("empty Docker argv")
	}
	switch args[0] {
	case "inspect":
		format := ""
		if len(args) > 2 {
			format = args[2]
		}
		switch {
		case strings.Contains(format, ".State.Health"):
			return []byte("healthy\n"), nil
		case strings.Contains(format, projectSpecLabel):
			if r.stale {
				return []byte("sha256:stale\n"), nil
			}
			return []byte("sha256:desired\n"), nil
		case strings.Contains(format, projectIDLabel):
			return []byte(r.instanceID + "\n"), nil
		case strings.Contains(format, projectRoleLabel):
			return []byte(projectWorkRole + "\n"), nil
		case strings.Contains(format, ownerLabel):
			return []byte(ownerValue + "\n"), nil
		default:
			return []byte("container-id\n"), nil
		}
	case "rm":
		r.stale = false
		return nil, nil
	case "create", "start":
		return nil, nil
	default:
		return nil, nil
	}
}

type projectExitRunner struct{ code int }

func (r *projectExitRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return projectExitError{code: r.code}
}

func (r *projectExitRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

type projectExitError struct{ code int }

func (e projectExitError) Error() string { return fmt.Sprintf("child exited with status %d", e.code) }
func (e projectExitError) ExitCode() int { return e.code }

func (r *projectReconcileRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *projectReconcileRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if r.failOn != nil && r.failOn(args) {
		return []byte("injected failure"), errors.New("injected failure")
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("empty Docker argv")
	}
	switch args[0] {
	case "image":
		return []byte(`{"api":"1","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`), nil
	case "network":
		if len(args) > 1 && args[1] == "inspect" {
			if !r.networkExists {
				return nil, errors.New("No such network")
			}
			return []byte("network-id\n"), nil
		}
		if len(args) > 1 && args[1] == "create" {
			r.networkExists = true
			return []byte("network-id\n"), nil
		}
		return nil, nil
	case "inspect":
		if len(args) > 2 && strings.Contains(args[2], ".State.Health") {
			return []byte("healthy\n"), nil
		}
		if len(args) > 2 && strings.Contains(args[2], ".NetworkSettings.Networks") {
			return []byte(`{}`), nil
		}
		if r.containerExists {
			return []byte("container-id\n"), nil
		}
		return nil, errors.New("No such object")
	case "create":
		r.containerExists = true
		return []byte("container-id\n"), nil
	case "start":
		return nil, nil
	default:
		return nil, nil
	}
}

func TestEnsureProjectContainerRecreatesOnSpecDrift(t *testing.T) {
	t.Parallel()
	runner := &projectSpecDriftRunner{stale: true, instanceID: "01900000-0000-7000-8000-000000000001"}
	runtime, err := newRuntimeWithData(
		filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), filepath.Join(t.TempDir(), "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := tobari.ProjectInstance{
		SchemaVersion: tobari.ProjectStateSchemaVersion,
		ID:            runner.instanceID,
		Root:          filepath.Join(t.TempDir(), "project"),
		Profile:       tobari.DefaultProfile,
		Image:         tobari.BuiltinImageSelector,
	}
	if err := os.MkdirAll(instance.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 2, RuntimeDirectory: filepath.Join(t.TempDir(), "runtime"),
		PolicyDirectory: filepath.Join(t.TempDir(), "policy"), CredentialConfig: filepath.Join(t.TempDir(), "credentials.json"),
		CredentialDir: filepath.Join(t.TempDir(), "credentials"), AssetVersion: "asset",
		ProxyEndpoint: "http://gateway:8080", Tobari: []tobari.Instance{},
	}
	if err := runtime.ensureProjectContainer(context.Background(), state, instance, "/profile", "tobari-project", "tobari-network", "tobari-image", "sha256:desired"); err != nil {
		t.Fatalf("ensureProjectContainer() error = %v", err)
	}
	var removed, created bool
	for _, call := range runner.calls {
		if len(call) > 0 && call[0] == "rm" {
			removed = true
		}
		if len(call) > 0 && call[0] == "create" {
			created = true
			if !strings.Contains(strings.Join(call, " "), projectSpecLabel+"=sha256:desired") {
				t.Fatalf("recreated container is missing desired spec hash: %v", call)
			}
		}
	}
	if !removed || !created {
		t.Fatalf("spec drift calls = %v, want rm followed by create", runner.calls)
	}
}

func TestEnsureProjectRuntimeFaultsPreserveLogicalState(t *testing.T) {
	t.Parallel()
	stages := map[string]func([]string) bool{
		"network creation": func(args []string) bool {
			return len(args) > 1 && args[0] == "network" && args[1] == "create"
		},
		"Gateway connection": func(args []string) bool {
			return len(args) > 1 && args[0] == "network" && args[1] == "connect"
		},
		"container creation": func(args []string) bool {
			return len(args) > 0 && args[0] == "create"
		},
		"container start": func(args []string) bool {
			return len(args) > 0 && args[0] == "start"
		},
	}
	for name, failOn := range stages {
		name, failOn := name, failOn
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			runner := &projectReconcileRunner{failOn: failOn}
			runtime, err := newRuntimeWithData(
				filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
			)
			if err != nil {
				t.Fatal(err)
			}
			projectRoot := filepath.Join(root, "project")
			if err := os.MkdirAll(projectRoot, 0o700); err != nil {
				t.Fatal(err)
			}
			instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
				t.Fatal("EnsureProjectRuntime() unexpectedly succeeded")
			}
			stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
			if err != nil || !found || stored.ID != instance.ID || stored.Root != instance.Root {
				t.Fatalf("logical state after %s failure = (%+v, %t, %v)", name, stored, found, err)
			}
			if _, err := os.Stat(runtime.projectHomePath(instance.ID)); err != nil {
				t.Fatalf("project home after %s failure: %v", name, err)
			}
		})
	}
}

func TestEnsureProjectRuntimeStateWriteFailurePreservesLogicalState(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &projectReconcileRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	instance, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	runtime.projectStateWriter = func(tobari.ProjectInstance) error {
		return errors.New("injected state update failure")
	}
	if _, err := runtime.EnsureProjectRuntime(context.Background(), runtimeState(root), instance); err == nil {
		t.Fatal("EnsureProjectRuntime() unexpectedly succeeded")
	}
	stored, found, err := runtime.ResolveProject(context.Background(), projectRoot)
	if err != nil || !found || stored.ID != instance.ID || stored.Runtime != (tobari.ProjectRuntime{}) {
		t.Fatalf("logical state after state update failure = (%+v, %t, %v)", stored, found, err)
	}
}

func TestEnterProjectRuntimePreservesChildExitStatus(t *testing.T) {
	t.Parallel()
	runtimeRoot := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(runtimeRoot, "config"), filepath.Join(runtimeRoot, "state"), filepath.Join(runtimeRoot, "data"),
		&projectExitRunner{code: 37},
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	code, err := runtime.EnterProjectRuntime(context.Background(), instance, instance.Root, nil, io.Discard, io.Discard)
	if err != nil || code != 37 {
		t.Fatalf("EnterProjectRuntime() = (%d, %v), want child status 37", code, err)
	}
}
