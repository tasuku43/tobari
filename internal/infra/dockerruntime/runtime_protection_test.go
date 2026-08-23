package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeProtectionRunner struct {
	containerID string
	workspaceID string
	spec        string
	calls       [][]string
	fail        bool
}

func (r *runtimeProtectionRunner) Run(_ context.Context, args, _ []string, _ io.Reader, stdout, stderr io.Writer) error {
	r.calls = append(r.calls, slices.Clone(args))
	if r.fail {
		_, _ = io.WriteString(stderr, "synthetic Docker observation failure")
		return errors.New("synthetic Docker observation failure")
	}
	_, err := io.WriteString(stdout, `{"id":"`+r.containerID+`","owner":"`+ownerValue+`","component":"tobari","workspace":"`+r.workspaceID+`","role":"`+projectWorkRole+`","spec":"`+r.spec+`"}`)
	return err
}

func (r *runtimeProtectionRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.calls = append(r.calls, slices.Clone(args))
	if r.fail {
		return []byte("synthetic Docker observation failure"), errors.New("synthetic Docker observation failure")
	}
	return []byte(`{"id":"` + r.containerID + `","owner":"` + ownerValue + `","component":"tobari","workspace":"` + r.workspaceID + `","role":"` + projectWorkRole + `","spec":"` + r.spec + `"}`), nil
}

func newRuntimeProtectionFixture(t *testing.T, runner *runtimeProtectionRunner) (*Runtime, tobari.Workspace, tobari.WorkspaceManifest) {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(root, "project")
	if err := os.Mkdir(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, _, err := runtime.ResolveOrCreateProject(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtime.activeContext()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.stateDirectory, "lifecycle.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return runtime, workspace, manifest
}

func TestRuntimeProtectionFreshObservationIsZeroWrite(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &runtimeProtectionRunner{}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil || !inventory.Complete || len(inventory.Items) != 0 {
		t.Fatalf("ReadRuntimeProtectionInventory() = %+v, %v", inventory, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 || len(runner.calls) != 0 {
		t.Fatalf("fresh protection observation mutated state: entries=%v calls=%v", entries, runner.calls)
	}
}

func TestLifecycleObservationRejectsStateAppearingDuringFreshRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &runtimeProtectionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.withLifecycleObservation(context.Background(), func(context.Context) error {
		return os.Mkdir(runtime.stateDirectory, 0o700)
	})
	var fault tobari.RuntimeProtectionInventoryError
	if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryObservationUnknown {
		t.Fatalf("withLifecycleObservation() error = %v", err)
	}
}

func TestReadOnlyObservationLocksCloseOnCancellationBeforeAcquisition(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("Windows lock adapter does not contend")
	}
	for name, setup := range map[string]func(*testing.T, *Runtime) (string, func(context.Context) error){
		"lifecycle": func(t *testing.T, runtime *Runtime) (string, func(context.Context) error) {
			if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.stateDirectory, "lifecycle.lock")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return path, func(ctx context.Context) error {
				return runtime.withLifecycleObservation(ctx, func(context.Context) error {
					t.Fatal("canceled observation ran its action")
					return nil
				})
			}
		},
		"project": func(t *testing.T, runtime *Runtime) (string, func(context.Context) error) {
			if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(runtime.stateDirectory, "project.lock")
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return path, func(ctx context.Context) error {
				return runtime.withExistingProjectLock(ctx, func() error {
					t.Fatal("canceled observation ran its action")
					return nil
				})
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &runtimeProtectionRunner{})
			if err != nil {
				t.Fatal(err)
			}
			path, observe := setup(t, runtime)
			holder, err := os.OpenFile(path, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			acquired, err := tryLockProjectFile(holder)
			if err != nil || !acquired {
				_ = holder.Close()
				t.Fatalf("hold test lock = %t, %v", acquired, err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			err = observe(ctx)
			cancel()
			unlockProjectFile(holder)
			if closeErr := holder.Close(); closeErr != nil {
				t.Fatal(closeErr)
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("canceled observation error = %v", err)
			}
			probe, err := os.OpenFile(path, os.O_RDONLY, 0)
			if err != nil {
				t.Fatalf("reopen observation lock after cancellation: %v", err)
			}
			if err := probe.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRuntimeProtectionRejectsStateWithoutLifecycleLockWithoutCreatingIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	state := filepath.Join(root, "state")
	if err := os.Mkdir(state, 0o700); err != nil {
		t.Fatal(err)
	}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), state, filepath.Join(root, "data"), &runtimeProtectionRunner{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.ReadRuntimeProtectionInventory(context.Background())
	var fault tobari.RuntimeProtectionInventoryError
	if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryObservationUnknown {
		t.Fatalf("ReadRuntimeProtectionInventory() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(state, "lifecycle.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read created lifecycle lock: %v", err)
	}
}

func TestRuntimeProtectionOmitsStandardBindings(t *testing.T) {
	runtime, _, _ := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
	inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Items) != 0 {
		t.Fatalf("standard Runtime entered managed lifecycle inventory: %+v", inventory.Items)
	}
}

func TestRuntimeProtectionCollapsesAppliedAndObservedExclusively(t *testing.T) {
	runner := &runtimeProtectionRunner{}
	runtime, workspace, manifest := newRuntimeProtectionFixture(t, runner)
	managedID := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("b", 64)
	spec := "sha256:" + strings.Repeat("c", 64)
	containerID := strings.Repeat("d", 64)
	manifest.RuntimeBinding = &tobari.RuntimeBinding{RuntimeID: managedID, Name: "tools", Revision: revision, Ordinal: 1, Image: "tobari-runtime-tools:bbbbbbbbbbbb"}
	workspace.Runtime.ContainerID = containerID
	workspace.LastSuccessfulEntry = &tobari.AppliedEntry{
		ManifestGeneration: manifest.Desired.Generation, ManifestRevision: manifest.Desired.Revision,
		EntryRevision: manifest.Desired.EntryRevision, RuntimeID: managedID, RuntimeRevision: revision,
		ResolvedSpec: spec, ReconciledAt: time.Date(2026, 8, 23, 0, 0, 0, 0, time.UTC),
	}
	if err := runtime.writeProjectInstance(workspace); err != nil {
		t.Fatal(err)
	}
	runner.containerID, runner.workspaceID, runner.spec = containerID, workspace.ID, spec
	observed, err := runtime.observeWorkspaceRuntimeProtection(context.Background(), workspace, lifecycleTestBudget())
	if err != nil || !observed {
		t.Fatalf("observeWorkspaceRuntimeProtection() = %t, %v", observed, err)
	}
	protection, ok := runtimeWorkspaceProtection(workspace, observed)
	if !ok || protection.Reason != tobari.RuntimeProtectedByWorkspaceObserved {
		t.Fatalf("observed protection = %+v, present=%t", protection, ok)
	}
	applied, ok := runtimeWorkspaceProtection(workspace, false)
	if !ok || applied.Reason != tobari.RuntimeProtectedByWorkspaceApplied {
		t.Fatalf("applied protection = %+v, present=%t", applied, ok)
	}
	if protection.RuntimeID != applied.RuntimeID || protection.RuntimeRevision != applied.RuntimeRevision ||
		protection.WorkspaceID != applied.WorkspaceID {
		t.Fatalf("exclusive reasons changed protection identity: observed=%+v applied=%+v", protection, applied)
	}
}

func TestMissingRuntimeContainerInspectRequiresExactDiagnostic(t *testing.T) {
	t.Parallel()
	containerID := strings.Repeat("d", 64)
	err := errors.New("container inspect failed")
	for _, diagnostic := range []string{
		"No such container: " + containerID,
		"wrapper: Error response from daemon: No such container: " + containerID,
		"Error response from daemon: No such container: " + containerID + " (wrapped)",
		"Error response from daemon: No such container: " + containerID + "\nunrelated failure",
		"Error response from daemon: No such container: unrelated\nError response from daemon: No such container: " + containerID,
	} {
		if isMissingRuntimeContainerInspect(err, []byte(diagnostic), containerID) {
			t.Fatalf("diagnostic authorized container absence %q", diagnostic)
		}
	}
	if !isMissingRuntimeContainerInspect(err, []byte("Error response from daemon: No such container: "+containerID), containerID) {
		t.Fatal("exact missing-container diagnostic was rejected")
	}
	if isMissingRuntimeContainerInspect(nil, []byte("Error response from daemon: No such container: "+containerID), containerID) {
		t.Fatal("successful inspect authorized container absence")
	}
}

func TestRuntimeProtectionPartialWorkspaceRuntimeIsMigrationUnverified(t *testing.T) {
	for name, mutate := range map[string]func(*tobari.Workspace){
		"network without container": func(workspace *tobari.Workspace) { workspace.Runtime.NetworkID = strings.Repeat("e", 64) },
		"container without applied": func(workspace *tobari.Workspace) { workspace.Runtime.ContainerID = strings.Repeat("f", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			runtime, workspace, _ := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
			mutate(&workspace)
			if err := runtime.writeProjectInstance(workspace); err != nil {
				t.Fatal(err)
			}
			_, err := runtime.ReadRuntimeProtectionInventory(context.Background())
			var fault tobari.RuntimeProtectionInventoryError
			if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryMigrationUnverified {
				t.Fatalf("ReadRuntimeProtectionInventory() error = %v", err)
			}
		})
	}
}

func TestRuntimeProtectionIncompleteOrUnsafeInventoryFailsClosed(t *testing.T) {
	tests := map[string]func(*testing.T, *Runtime, tobari.Workspace){
		"unsafe Manifest entry": func(t *testing.T, runtime *Runtime, _ tobari.Workspace) {
			target := filepath.Join(t.TempDir(), "target")
			if err := os.Mkdir(target, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(runtime.contextsDirectory(), "unsafe")); err != nil {
				t.Fatal(err)
			}
		},
		"missing Workspace state": func(t *testing.T, runtime *Runtime, workspace tobari.Workspace) {
			directory, err := runtime.projectDirectory(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(directory); err != nil {
				t.Fatal(err)
			}
		},
		"pending Workspace journal": func(t *testing.T, runtime *Runtime, workspace tobari.Workspace) {
			journal := projectJournal{
				SchemaVersion: projectJournalSchema, Operation: projectOpCreate, ProjectID: workspace.ID,
				Root: workspace.Root, WorkspaceManifestID: workspace.WorkspaceManifestID, Phase: projectPhaseStarted,
			}
			if err := runtime.writeProjectJournal(journal); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			runtime, workspace, _ := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
			mutate(t, runtime, workspace)
			inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
			var fault tobari.RuntimeProtectionInventoryError
			if !errors.As(err, &fault) || fault.Reason != tobari.RuntimeProtectionInventoryIncomplete {
				t.Fatalf("ReadRuntimeProtectionInventory() = %+v, %v", inventory, err)
			}
			if inventory.Complete {
				t.Fatalf("incomplete inventory remained complete: %+v", inventory)
			}
		})
	}
}

func TestRuntimeProtectionRejectsSubstitutedAuthorityDirectories(t *testing.T) {
	tests := map[string]func(*testing.T, *Runtime, tobari.Workspace, tobari.WorkspaceManifest) string{
		"contexts": func(_ *testing.T, runtime *Runtime, _ tobari.Workspace, _ tobari.WorkspaceManifest) string {
			return runtime.contextsDirectory()
		},
		"current Manifest": func(_ *testing.T, runtime *Runtime, _ tobari.Workspace, manifest tobari.WorkspaceManifest) string {
			return runtime.contextDirectory(manifest.Name)
		},
		"retained Manifest revisions": func(_ *testing.T, runtime *Runtime, _ tobari.Workspace, manifest tobari.WorkspaceManifest) string {
			return runtime.manifestRevisionsDirectory(manifest.Name)
		},
		"root indexes": func(_ *testing.T, runtime *Runtime, _ tobari.Workspace, _ tobari.WorkspaceManifest) string {
			return runtime.rootsDirectory()
		},
		"Workspace instances": func(_ *testing.T, runtime *Runtime, _ tobari.Workspace, _ tobari.WorkspaceManifest) string {
			return runtime.instancesDirectory()
		},
		"Workspace instance": func(t *testing.T, runtime *Runtime, workspace tobari.Workspace, _ tobari.WorkspaceManifest) string {
			directory, err := runtime.projectDirectory(workspace.ID)
			if err != nil {
				t.Fatal(err)
			}
			return directory
		},
	}
	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			runtime, workspace, manifest := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
			path := target(t, runtime, workspace, manifest)
			backup := filepath.Join(t.TempDir(), "authority-backup")
			if err := os.Rename(path, backup); err != nil {
				t.Fatal(err)
			}
			empty := t.TempDir()
			if err := os.Chmod(empty, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(empty, path); err != nil {
				t.Fatal(err)
			}
			inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
			if err == nil || inventory.Complete {
				t.Fatalf("substituted %s authorized complete protection = %+v/%v", path, inventory, err)
			}
		})
	}
}

func TestRuntimeProtectionRequiresOwnerOnlyAuthorityDirectories(t *testing.T) {
	for name, target := range map[string]func(*Runtime, tobari.WorkspaceManifest) string{
		"contexts": func(runtime *Runtime, _ tobari.WorkspaceManifest) string { return runtime.contextsDirectory() },
		"retained revisions": func(runtime *Runtime, manifest tobari.WorkspaceManifest) string {
			return runtime.manifestRevisionsDirectory(manifest.Name)
		},
		"root indexes":        func(runtime *Runtime, _ tobari.WorkspaceManifest) string { return runtime.rootsDirectory() },
		"Workspace instances": func(runtime *Runtime, _ tobari.WorkspaceManifest) string { return runtime.instancesDirectory() },
	} {
		t.Run(name, func(t *testing.T) {
			runtime, _, manifest := newRuntimeProtectionFixture(t, &runtimeProtectionRunner{})
			if err := os.Chmod(target(runtime, manifest), 0o755); err != nil {
				t.Fatal(err)
			}
			inventory, err := runtime.ReadRuntimeProtectionInventory(context.Background())
			if err == nil || inventory.Complete {
				t.Fatalf("non-private authority authorized complete protection = %+v/%v", inventory, err)
			}
		})
	}
}

func TestRuntimeProtectionOrderingIncludesRetainedManifestRevision(t *testing.T) {
	base := tobari.RuntimeProtection{
		RuntimeID: "018bcfe5-687b-7000-8000-000000000077", RuntimeRevision: "sha256:" + strings.Repeat("d", 64),
		Reason: tobari.RuntimeProtectedByManifestRetained, WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad",
	}
	first, second := base, base
	first.ManifestRevision = "sha256:" + strings.Repeat("a", 64)
	second.ManifestRevision = "sha256:" + strings.Repeat("b", 64)
	items := []tobari.RuntimeProtection{second, first}
	slices.SortFunc(items, func(left, right tobari.RuntimeProtection) int {
		return strings.Compare(runtimeProtectionSortKey(left), runtimeProtectionSortKey(right))
	})
	if items[0].ManifestRevision != first.ManifestRevision || items[1].ManifestRevision != second.ManifestRevision {
		t.Fatalf("retained protection ordering = %+v", items)
	}
}
