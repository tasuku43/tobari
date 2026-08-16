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
	defaultImage := runtime.defaultRuntimeImage()
	if result.Task != tobari.TaskRuntimeInit || result.Image != defaultImage ||
		result.Runtime.Status != tobari.ContextRuntimeStatusPendingBuild {
		t.Fatalf("InitRuntime() result = %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "config", "contexts", "default", "runtime", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FROM "+defaultImage) {
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
	if len(runner.runs) != 1 || len(runner.runs[0].args) < 3 ||
		runner.runs[0].args[0] != "buildx" || runner.runs[0].args[1] != "build" {
		t.Fatalf("Docker build calls = %+v", runner.runs)
	}
	if !containsArgs(runner.runs[0].args, "--progress=plain") || !containsArgs(runner.runs[0].args, "--load") ||
		!containsArgs(runner.runs[0].args, "--file") ||
		!containsArgs(runner.runs[0].args, filepath.Join(root, "config", "contexts", "default", "runtime")) {
		t.Fatalf("Docker build argv = %+v", runner.runs[0].args)
	}
	if containsArgs(runner.runs[0].args, "--pull") {
		t.Fatalf("local built-in base must not be pulled: %+v", runner.runs[0].args)
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

func TestRuntimeBuildChangesOnlyCurrentContextAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtimeStore, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.CreateContext(context.Background(), "restricted", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	restrictedBefore, err := runtimeStore.ShowContext(context.Background(), "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimeStore.InitRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	defaultBuilt, err := runtimeStore.BuildRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	restrictedAfter, err := runtimeStore.ShowContext(context.Background(), "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if defaultBuilt.ID == restrictedAfter.ID || restrictedAfter.ID != restrictedBefore.ID ||
		restrictedAfter.Image != restrictedBefore.Image || restrictedAfter.Runtime != restrictedBefore.Runtime {
		t.Fatalf("default build changed restricted Context: before=%+v after=%+v built=%+v", restrictedBefore, restrictedAfter, defaultBuilt)
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
	data = []byte(strings.Replace(string(data), "FROM "+runtime.defaultRuntimeImage(), "FROM example.com/tobari/custom-base:dev", 1))
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

func TestBuildRuntimeStreamsDockerFailureDiagnosticsInNonTTYEnvironments(t *testing.T) {
	tests := []struct {
		name       string
		diagnostic string
	}{
		{name: "Dockerfile syntax", diagnostic: "ERROR: failed to solve: dockerfile parse error on line 4: unknown instruction: RNU\n"},
		{name: "RUN command", diagnostic: " > [2/2] RUN gh --version:\n/bin/sh: gh: not found\nERROR: process failed\n"},
		{name: "base image", diagnostic: "ERROR: failed to resolve source metadata for example.invalid/missing:latest\n"},
		{name: "daemon", diagnostic: "ERROR: Cannot connect to the Docker daemon at unix:///var/run/docker.sock\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &contextBuildFailureRunner{diagnostic: test.diagnostic}
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
			var diagnostics strings.Builder
			var events []tobari.RuntimeBuildProgress
			_, err = runtime.BuildRuntimeWithProgress(
				context.Background(), &diagnostics,
				func(event tobari.RuntimeBuildProgress) { events = append(events, event) },
			)
			if err == nil {
				t.Fatal("BuildRuntimeWithProgress() succeeded on a Docker failure")
			}
			if !strings.Contains(diagnostics.String(), "synthetic stdout progress") ||
				!strings.Contains(diagnostics.String(), strings.TrimSpace(test.diagnostic)) {
				t.Fatalf("diagnostics = %q", diagnostics.String())
			}
			if len(events) < 4 || events[len(events)-1].Stage != tobari.RuntimeBuildStageBuild ||
				events[len(events)-1].Status != tobari.RuntimeBuildProgressFailed ||
				events[len(events)-1].Selection != tobari.RuntimeBuildSelectionUnchanged {
				t.Fatalf("events = %+v", events)
			}
			after, err := runtime.ShowContext(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if after.Image != before.Image {
				t.Fatalf("selected image changed: before=%q after=%q", before.Image, after.Image)
			}
		})
	}
}

func TestBuildRuntimeFailurePreservesPreviouslyBuiltRuntime(t *testing.T) {
	root := t.TempDir()
	success := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), success)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListContexts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InitRuntime(context.Background()); err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildRuntime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := runtime.contextRuntimeDockerfile("default")
	data, err := os.ReadFile(dockerfile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dockerfile, append(data, []byte("\n# next candidate\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	failure := &contextBuildFailureRunner{diagnostic: "ERROR: process failed\n"}
	runtime.runner = failure
	if _, err := runtime.BuildRuntimeWithProgress(context.Background(), io.Discard, nil); err == nil {
		t.Fatal("second BuildRuntimeWithProgress() succeeded")
	}
	after, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if after.Image != built.Image || after.Runtime.ImageDigest != built.Runtime.ImageDigest {
		t.Fatalf("previous runtime changed: built=%+v after=%+v", built, after)
	}
	for _, call := range failure.runs {
		if len(call.args) > 0 && (call.args[0] == "rmi" || call.args[0] == "image" && len(call.args) > 1 && call.args[1] == "rm") {
			t.Fatalf("failure removed an image: %+v", call.args)
		}
	}
}

type contextBuildFailureRunner struct {
	recordingRunner
	diagnostic string
}

func (r *contextBuildFailureRunner) Run(
	_ context.Context, args, _ []string, _ io.Reader, out, errOut io.Writer,
) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 1 && args[0] == "buildx" && args[1] == "build" {
		_, _ = io.WriteString(out, "synthetic stdout progress\n")
		_, _ = io.WriteString(errOut, r.diagnostic)
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
