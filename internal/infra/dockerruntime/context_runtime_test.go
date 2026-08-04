package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func compatibleImageInspection() []byte {
	return []byte(`{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`)
}

func imageDigestInspection() []byte {
	return []byte("sha256:" + strings.Repeat("c", 64))
}

func TestInitRuntimeCreatesActiveContextRecipeWithoutChangingImage(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.InitRuntime(context.Background())
	if err != nil {
		t.Fatalf("InitRuntime() error = %v", err)
	}
	if result.Task != tobari.TaskRuntimeInit || result.Image != tobari.OfficialRuntimeBase ||
		result.Runtime.Status != tobari.ContextRuntimeStatusPendingBuild {
		t.Fatalf("InitRuntime() result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "contexts", "default", "runtime", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FROM "+tobari.OfficialRuntimeBase) {
		t.Fatalf("runtime template = %q", data)
	}
	if _, err := runtime.InitRuntime(context.Background()); !errors.Is(err, tobari.ErrRuntimeRecipeExists) {
		t.Fatalf("second InitRuntime() error = %v, want recipe exists", err)
	}
	manifestData, err := os.ReadFile(filepath.Join(root, "config", "contexts", "default", "context.json"))
	if err != nil || strings.Contains(string(manifestData), "credentials") {
		t.Fatalf("manifest = %q, error = %v", manifestData, err)
	}
}

func TestBuildRuntimeValidatesAndPromotesManagedImage(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InitRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.BuildRuntime(context.Background())
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if result.Task != tobari.TaskRuntimeBuild || result.Runtime.Status != tobari.ContextRuntimeStatusReady ||
		result.Image == tobari.BuiltinImageSelector || result.Runtime.ImageDigest == "" {
		t.Fatalf("BuildRuntime() result = %+v", result)
	}
	if len(runner.runs) != 1 || len(runner.runs[0].args) < 2 || runner.runs[0].args[0] != "build" {
		t.Fatalf("Docker build calls = %+v", runner.runs)
	}
	if !containsArgs(runner.runs[0].args, "--file") ||
		!containsArgs(runner.runs[0].args, filepath.Join(root, "config", "contexts", "default", "runtime")) {
		t.Fatalf("Docker build argv = %+v", runner.runs[0].args)
	}
	if !containsArgs(runner.runs[0].args, "--pull") {
		t.Fatalf("official runtime build must refresh the moving base: %+v", runner.runs[0].args)
	}

	dockerfile := filepath.Join(root, "config", "contexts", "default", "runtime", "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte(runtimeRecipeTemplate(runtime.defaultRuntimeImage())+"\n# changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shown, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if shown.Runtime.Status != tobari.ContextRuntimeStatusPendingBuild || shown.Image != result.Image {
		t.Fatalf("changed recipe report = %+v", shown)
	}
}

func TestInitRuntimeUsesInjectedResolverBase(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:dev"}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.InitRuntime(context.Background())
	if err != nil {
		t.Fatalf("InitRuntime() error = %v", err)
	}
	if result.Image != "tobari-runtime:dev" || result.Runtime.BaseReference != "tobari-runtime:dev" {
		t.Fatalf("InitRuntime() result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "contexts", "default", "runtime", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FROM tobari-runtime:dev") {
		t.Fatalf("runtime template = %q", data)
	}
}

func TestBuildRuntimeDoesNotPullExplicitCustomBase(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InitRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	dockerfile := filepath.Join(root, "config", "contexts", "default", "runtime", "Dockerfile")
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "FROM "+tobari.OfficialRuntimeBase, "FROM example.com/tobari/custom-base:dev", 1))
	if err := os.WriteFile(dockerfile, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildRuntime(context.Background()); err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if len(runner.runs) != 1 || containsArgs(runner.runs[0].args, "--pull") {
		t.Fatalf("custom-base runtime build unexpectedly pulled a base: %+v", runner.runs)
	}
}

func TestBuildRuntimeFailureLeavesSelectedImageUnchanged(t *testing.T) {
	root := t.TempDir()
	runner := &contextBuildFailureRunner{recordingRunner: recordingRunner{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InitRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildRuntime(context.Background()); err == nil {
		t.Fatal("BuildRuntime() succeeded on a Docker build failure")
	}
	after, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if after.Image != before.Image {
		t.Fatalf("selected image changed after failed build: before=%q after=%q", before.Image, after.Image)
	}
}

type contextBuildFailureRunner struct {
	recordingRunner
}

func (r *contextBuildFailureRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "build" {
		return errors.New("synthetic Docker build failure")
	}
	return nil
}

func containsArgs(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
