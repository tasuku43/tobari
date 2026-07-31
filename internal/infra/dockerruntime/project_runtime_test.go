package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
