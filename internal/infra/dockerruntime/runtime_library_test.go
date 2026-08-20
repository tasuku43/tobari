package dockerruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestManagedRuntimeBuildCreatesImmutableRevisionWithoutChangingContext(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
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

	created, err := runtime.CreateRuntime(context.Background(), "frontend")
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
	if !noChange.NoChange || noChange.Built || len(noChange.Runtime.Revisions) != 1 || len(runner.runs) != 1 {
		t.Fatalf("no-change build = %+v, runs=%d", noChange, len(runner.runs))
	}
}

func TestContextRuntimeSetPinsExactReadyRevision(t *testing.T) {
	root := t.TempDir()
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateRuntime(context.Background(), "frontend"); err != nil {
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
	if selected.Task != tobari.TaskContextRuntimeSet || selected.Runtime.RuntimeID != built.Runtime.ID || selected.Runtime.Revision != want.Revision || selected.Image != want.Image {
		t.Fatalf("selected = %+v", selected)
	}

	rolledBack, err := runtime.SetContextRuntime(context.Background(), "default", tobari.StandardRuntimeName)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Runtime.RuntimeID != tobari.StandardRuntimeID || rolledBack.Runtime.Status != tobari.ContextRuntimeStatusOfficial {
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
	created, err := runtime.CreateRuntime(context.Background(), "unsafe")
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
	runner := &recordingRunner{outputQueue: [][]byte{compatibleImageInspection(), imageDigestInspection()}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "binary")
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
	if err != nil || !built.Built || len(runner.runs) != 1 {
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
	created, err := runtime.CreateRuntime(context.Background(), "oversized")
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
	created, err := runtime.CreateRuntime(context.Background(), "permissions")
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
	created, err := runtime.CreateRuntime(context.Background(), "directory-permissions")
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
	created, err := runtime.CreateRuntime(context.Background(), "total")
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
			created, err := runtime.CreateRuntime(context.Background(), test.name)
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
