package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type managedRuntimeTestImage struct {
	id     string
	labels map[string]string
}

type managedRuntimeBuildRunner struct {
	runs            []runnerCall
	outputs         []runnerCall
	images          map[string]managedRuntimeTestImage
	failBuild       bool
	corruptEvidence bool
}

func newManagedRuntimeBuildRunner() *managedRuntimeBuildRunner {
	return &managedRuntimeBuildRunner{images: make(map[string]managedRuntimeTestImage)}
}

func (r *managedRuntimeBuildRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 2 && args[0] == "buildx" && args[1] == "build" {
		labels := make(map[string]string)
		for index := 0; index+1 < len(args); index++ {
			if args[index] != "--label" {
				continue
			}
			key, value, ok := strings.Cut(args[index+1], "=")
			if ok {
				labels[key] = value
			}
		}
		tag := args[slices.Index(args, "--tag")+1]
		r.images[tag] = managedRuntimeTestImage{id: "sha256:" + strings.Repeat("c", 64), labels: labels}
		if r.failBuild {
			return errors.New("synthetic build failure")
		}
		return nil
	}
	if len(args) == 4 && args[0] == "image" && args[1] == "tag" {
		image, ok := r.images[args[2]]
		if !ok {
			return errors.New("source image missing")
		}
		r.images[args[3]] = image
		return nil
	}
	if len(args) == 3 && args[0] == "image" && args[1] == "rm" {
		delete(r.images, args[2])
		return nil
	}
	return fmt.Errorf("unexpected Runtime build mutation: %v", args)
}

func (r *managedRuntimeBuildRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) < 5 || args[0] != "image" || args[1] != "inspect" {
		return nil, fmt.Errorf("unexpected Runtime build observation: %v", args)
	}
	format, imageName := args[3], args[4]
	if strings.Contains(format, tobari.RuntimeImageAPILabel) {
		return compatibleImageInspection(), nil
	}
	image, ok := r.images[imageName]
	if !ok {
		return []byte("Error: No such image"), errors.New("image missing")
	}
	owned := image.labels[ownerLabel] == ownerValue && image.labels[componentLabel] == managedRuntimeComponentLabel && image.labels[managedRuntimeIDLabel] != "" && image.labels[managedRuntimeRevisionLabel] != ""
	if r.corruptEvidence {
		owned = false
	}
	return []byte(fmt.Sprintf(`{"id":%q,"owned":%t}`, image.id, owned)), nil
}

func TestManagedRuntimeBuildCreatesImmutableRevisionWithoutChangingContext(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	before, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}

	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || created.Runtime.SourcePath != filepath.Join(root, "config", "runtimes", "frontend", "source") {
		t.Fatalf("created = %+v", created)
	}
	install := filepath.Join(created.Runtime.SourcePath, "install.sh")
	if err := os.WriteFile(install, []byte("#!/bin/sh\nset -eu\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !built.Built || built.NoChange || len(built.Runtime.Revisions) != 1 {
		t.Fatalf("built = %+v", built)
	}
	revision := built.Runtime.Revisions[0]
	if !strings.Contains(revision.SnapshotPath, filepath.Join("revisions", strings.TrimPrefix(revision.Revision, "sha256:"), "source")) {
		t.Fatalf("snapshot = %q", revision.SnapshotPath)
	}
	after, err := runtime.ShowContext(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if before.Image != after.Image || before.Runtime.Revision != after.Runtime.Revision {
		t.Fatalf("Runtime build changed Context: before=%+v after=%+v", before.Runtime, after.Runtime)
	}

	noChange, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !noChange.NoChange || noChange.Built || len(noChange.Runtime.Revisions) != 1 || len(runner.runs) != 3 {
		t.Fatalf("no-change build = %+v, runs=%d", noChange, len(runner.runs))
	}
	buildArgs := runner.runs[0].args
	for _, label := range []string{
		ownerLabel + "=" + ownerValue,
		componentLabel + "=" + managedRuntimeComponentLabel,
		managedRuntimeIDLabel + "=" + built.Runtime.ID,
		managedRuntimeRevisionLabel + "=" + revision.Revision,
	} {
		if !slices.Contains(buildArgs, label) {
			t.Fatalf("managed Runtime build lacks trusted label %q: %v", label, buildArgs)
		}
	}
	if _, exists := runner.images[revision.Image]; !exists {
		t.Fatalf("published Runtime tag is absent: %v", runner.images)
	}
	if _, exists := runner.images[managedRuntimeStagingImage(built.Runtime.ID, revision.Revision)]; exists {
		t.Fatalf("successful build retained staging tag: %v", runner.images)
	}
	if _, err := os.Lstat(runtime.runtimeBuildJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("successful build retained journal: %v", err)
	}
}

func TestManagedRuntimeBuildKeepsExactFailedArtifactJournal(t *testing.T) {
	for _, test := range []struct {
		name            string
		failBuild       bool
		corruptEvidence bool
	}{
		{name: "build failure", failBuild: true},
		{name: "ownership verification failure", corruptEvidence: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := newManagedRuntimeBuildRunner()
			runner.failBuild = test.failBuild
			runner.corruptEvidence = test.corruptEvidence
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
				t.Fatal("unsafe managed Runtime build succeeded")
			}
			manifest, err := runtime.readRuntimeManifest("frontend")
			if err != nil || len(manifest.Revisions) != 0 {
				t.Fatalf("failed build published history = %+v/%v", manifest, err)
			}
			journal, err := runtime.readRuntimeBuildJournalObserved()
			if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseFailed || journal.RuntimeID != created.Runtime.ID || journal.Revision == "" || journal.StagingImage != managedRuntimeStagingImage(journal.RuntimeID, journal.Revision) {
				t.Fatalf("failed build journal = %+v/%v", journal, err)
			}
			if info, err := os.Stat(journal.SnapshotPath); err != nil || !info.IsDir() {
				t.Fatalf("failed build staging snapshot = %v/%v", info, err)
			}
		})
	}
}

func TestManagedRuntimeBuildRollsBackJournalBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(created.Runtime.SourcePath, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.Runtime.SourcePath, "README"), []byte("no Dockerfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err == nil {
		t.Fatal("invalid pre-Docker source built")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("invalid source crossed Docker mutation: %v", runner.runs)
	}
	if _, err := os.Lstat(runtime.runtimeBuildJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-Docker failure retained journal: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(runtime.runtimeBuildSnapshotPath())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pre-Docker failure retained staging snapshot: %v", err)
	}
}

func TestManagedRuntimeBuildSurfacesPreDockerCleanupFailure(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(created.Runtime.SourcePath, "Dockerfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(created.Runtime.SourcePath, "README"), []byte("no Dockerfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime.runtimeBuildCleanup = func(runtimeBuildJournal) error { return errors.New("synthetic cleanup failure") }
	_, err = runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err == nil || !strings.Contains(err.Error(), "requires reconciliation") || !strings.Contains(err.Error(), "synthetic cleanup failure") {
		t.Fatalf("cleanup failure = %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("cleanup failure crossed Docker mutation: %v", runner.runs)
	}
	journal, journalErr := runtime.readRuntimeBuildJournalObserved()
	if journalErr != nil || journal == nil || journal.Phase != runtimeBuildPhasePrepared {
		t.Fatalf("cleanup failure lost journal authority = %+v/%v", journal, journalErr)
	}
}

func TestManagedRuntimeBuildReferenceCannotRetargetSameNameReuse(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	old, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	oldRef := tobari.RuntimeRef(old.Runtime.ID)
	resolved, err := runtime.ResolveRuntimeReference(context.Background(), oldRef)
	if err != nil || resolved.ID != old.Runtime.ID {
		t.Fatalf("application resolution = %+v/%v", resolved, err)
	}

	// Model retirement and same-name recreation in the window between the
	// application read and the effect boundary. The effect must re-resolve the
	// stable ID while locked instead of inheriting authority from this name.
	if err := os.RemoveAll(runtime.runtimeDirectory("frontend")); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Runtime.ID == old.Runtime.ID {
		t.Fatalf("same-name recreation reused Runtime ID %q", fresh.Runtime.ID)
	}
	manifestBefore, err := os.ReadFile(runtime.runtimeManifestPath("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	sourceBefore, err := os.ReadFile(filepath.Join(fresh.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.BuildManagedRuntimeByReference(context.Background(), oldRef, nil); !errors.Is(err, tobari.ErrRuntimeNotFound) {
		t.Fatalf("retired Runtime build error = %v", err)
	}
	manifestAfter, err := os.ReadFile(runtime.runtimeManifestPath("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	sourceAfter, err := os.ReadFile(filepath.Join(fresh.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	if string(manifestAfter) != string(manifestBefore) || string(sourceAfter) != string(sourceBefore) {
		t.Fatalf("fresh same-name Runtime changed: manifest=%t source=%t", string(manifestAfter) != string(manifestBefore), string(sourceAfter) != string(sourceBefore))
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker build crossed stale Runtime authority: %+v", runner.runs)
	}
	if len(runner.outputs) != 0 {
		t.Fatalf("Docker inspection crossed stale Runtime authority: %+v", runner.outputs)
	}
}

func TestResolveRuntimeReferenceCannotRetargetSameNameReuse(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	old, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	oldRef := tobari.RuntimeRef(old.Runtime.ID)
	if err := os.RemoveAll(runtime.runtimeDirectory("frontend")); err != nil {
		t.Fatal(err)
	}
	fresh, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ResolveRuntimeReference(context.Background(), oldRef); !errors.Is(err, tobari.ErrRuntimeNotFound) {
		t.Fatalf("retired reference resolved after same-name replacement: %v", err)
	}
	resolved, err := runtime.ResolveRuntimeReference(context.Background(), tobari.RuntimeRef(fresh.Runtime.ID))
	if err != nil || resolved.ID != fresh.Runtime.ID {
		t.Fatalf("fresh reference resolution = %+v/%v", resolved, err)
	}
}

func TestRuntimeCreateCopiesManagedEditableBaseAsStandaloneSource(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	base, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(base.Runtime.SourcePath, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "tool")
	if err := os.WriteFile(tool, []byte("synthetic executable\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(base.Runtime.SourcePath, "empty")
	if err := os.Mkdir(empty, 0o500); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil); err != nil {
		t.Fatal(err)
	}

	created, err := runtime.CreateRuntime(context.Background(), "mobile", tobari.RuntimeCopySource("frontend"))
	if err != nil {
		t.Fatal(err)
	}
	if !created.Created || created.Runtime.ID == base.Runtime.ID || created.Runtime.Name != "mobile" ||
		len(created.Runtime.Revisions) != 0 || created.Runtime.SourcePath == base.Runtime.SourcePath {
		t.Fatalf("standalone created Runtime = %+v, base = %+v", created, base)
	}
	copiedTool := filepath.Join(created.Runtime.SourcePath, "bin", "tool")
	data, err := os.ReadFile(copiedTool)
	if err != nil || string(data) != "synthetic executable\n" {
		t.Fatalf("copied tool = %q/%v", data, err)
	}
	if info, err := os.Stat(copiedTool); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("copied tool mode = %v/%v", info, err)
	}
	if info, err := os.Stat(filepath.Join(created.Runtime.SourcePath, "empty")); err != nil || info.Mode().Perm() != 0o500 {
		t.Fatalf("copied empty directory mode = %v/%v", info, err)
	}
	if err := os.WriteFile(tool, []byte("later Base edit\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(copiedTool)
	if err != nil || string(data) != "synthetic executable\n" {
		t.Fatalf("target changed after Base edit = %q/%v", data, err)
	}
}

func TestRuntimeCreateFromMissingOrInvalidBasePublishesNoTarget(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*testing.T, *Runtime)
		base  tobari.RuntimeCopySource
		code  string
	}{
		{name: "missing", setup: func(*testing.T, *Runtime) {}, base: "missing"},
		{name: "invalid source", setup: func(t *testing.T, runtime *Runtime) {
			created, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("Dockerfile", filepath.Join(created.Runtime.SourcePath, "link")); err != nil {
				t.Fatal(err)
			}
		}, base: "frontend", code: "runtime_source_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, runtime)
			_, err = runtime.CreateRuntime(context.Background(), "mobile", test.base)
			if test.code == "" {
				if !errors.Is(err, tobari.ErrRuntimeNotFound) {
					t.Fatalf("missing Base error = %v", err)
				}
			} else if public, ok := fault.PublicCopy(err); !ok || public.Code != test.code {
				t.Fatalf("invalid Base source fault = %+v/%v", public, err)
			}
			if _, statErr := os.Lstat(runtime.runtimeDirectory("mobile")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("failed creation published target: %v", statErr)
			}
		})
	}
}

func TestRuntimeCreateCancellationPublishesNoTarget(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.CreateRuntime(ctx, "mobile", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled create error = %v", err)
	}
	if _, err := os.Lstat(runtime.runtimeDirectory("mobile")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled creation published target: %v", err)
	}
}

func TestContextRuntimeSetPinsExactReadyRevision(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend", tobari.RuntimeCopySource(tobari.StandardRuntimeName)); err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), "frontend", nil)
	if err != nil {
		t.Fatal(err)
	}

	selected, err := runtime.SetContextRuntime(context.Background(), "default", "frontend@1")
	if err != nil {
		t.Fatal(err)
	}
	want := built.Runtime.Revisions[0]
	if selected.Task != tobari.TaskManifestRuntimeSet || selected.Runtime.RuntimeID != built.Runtime.ID || selected.Runtime.Revision != want.Revision || selected.Image != want.Image {
		t.Fatalf("selected = %+v", selected)
	}

	rolledBack, err := runtime.SetContextRuntime(context.Background(), "default", tobari.StandardRuntimeName)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Runtime.RuntimeID != tobari.StandardRuntimeID || rolledBack.Runtime.Status != tobari.ManifestRuntimeStatusOfficial {
		t.Fatalf("rolled back = %+v", rolledBack)
	}
}

func TestRuntimeSourceRejectsSymlinksBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "unsafe", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("Dockerfile", filepath.Join(created.Runtime.SourcePath, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.BuildManagedRuntime(context.Background(), "unsafe", nil); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("build error = %v", err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("Docker ran for unsafe source: %+v", runner.runs)
	}
}

func TestRuntimeSourceAcceptsPrivateBinaryWithinStreamedBounds(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "binary", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(created.Runtime.SourcePath, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(bin, "tool")
	file, err := os.OpenFile(tool, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(10 * 1024 * 1024); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	built, err := runtime.BuildManagedRuntime(context.Background(), "binary", nil)
	if err != nil || !built.Built || len(runner.runs) != 3 {
		t.Fatalf("binary build = %+v/%v runs=%d", built, err, len(runner.runs))
	}
	snapshot := filepath.Join(built.Runtime.Revisions[0].SnapshotPath, "bin", "tool")
	if info, err := os.Stat(snapshot); err != nil || info.Size() != 10*1024*1024 {
		t.Fatalf("binary snapshot = %v/%v", info, err)
	}
}

func TestRuntimeSourceSizeFailureReportsPathActualAndLimitBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "oversized", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(created.Runtime.SourcePath, "tool")
	file, err := os.OpenFile(tool, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxRuntimeSourceFile + 1); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "oversized", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"tool"`, "33554433 bytes", "33554432 bytes", "32 MiB"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source size fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source size code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourcePermissionFailureReportsCorrectionBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "permissions", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(created.Runtime.SourcePath, "tool")
	if err := os.WriteFile(tool, []byte("synthetic"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(tool, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "permissions", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"tool"`, "0755", "group/other", "owner-only", "0600 or 0700"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source permission fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source permission code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceDirectoryPermissionFailureReportsCorrectionBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "directory-permissions", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(created.Runtime.SourcePath, "bin")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "directory-permissions", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"bin"`, "0755", "group/other", "owner-only", "0700"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source directory permission fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source directory permission code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceTotalFailureReportsActualAndLimitBeforeDocker(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "total", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile, err := os.Stat(filepath.Join(created.Runtime.SourcePath, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for name, size := range map[string]int64{
		"a": maxRuntimeSourceFile,
		"b": maxRuntimeSourceTotal - maxRuntimeSourceFile - dockerfile.Size() + 1,
	} {
		file, err := os.OpenFile(filepath.Join(created.Runtime.SourcePath, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}

	_, err = runtime.BuildManagedRuntime(context.Background(), "total", nil)
	public, ok := fault.PublicCopy(err)
	for _, want := range []string{`"b"`, "67108865 bytes", "67108864 bytes", "64 MiB"} {
		if !ok || !strings.Contains(public.Message, want) {
			t.Fatalf("source total fault = %+v/%v, missing %q", public, err, want)
		}
	}
	if public.Code != "runtime_source_invalid" || len(runner.runs) != 0 {
		t.Fatalf("source total code/runs = %q/%d", public.Code, len(runner.runs))
	}
}

func TestRuntimeSourceCountBoundsRejectBeforeDocker(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, string)
		want  string
	}{
		{
			name: "files",
			setup: func(t *testing.T, source string) {
				for index := 0; index < maxRuntimeSourceFiles; index++ {
					path := filepath.Join(source, fmt.Sprintf("file-%04d", index))
					if err := os.WriteFile(path, nil, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "contains 1025 files; the limit is 1024",
		},
		{
			name: "directories",
			setup: func(t *testing.T, source string) {
				for index := 0; index <= maxRuntimeSourceDirs; index++ {
					path := filepath.Join(source, fmt.Sprintf("dir-%03d", index))
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
			},
			want: "contains 257 directories; the limit is 256",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			created, err := runtime.CreateRuntime(context.Background(), test.name, tobari.RuntimeCopySource(tobari.StandardRuntimeName))
			if err != nil {
				t.Fatal(err)
			}
			test.setup(t, created.Runtime.SourcePath)
			_, err = runtime.BuildManagedRuntime(context.Background(), test.name, nil)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "runtime_source_invalid" || !strings.Contains(public.Message, test.want) || len(runner.runs) != 0 {
				t.Fatalf("source count fault/runs = %+v/%v/%d", public, err, len(runner.runs))
			}
		})
	}
}
