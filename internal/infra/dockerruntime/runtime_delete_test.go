package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestDeleteManagedRuntimeRetiresWholeAuthorityAndIsIdempotent(t *testing.T) {
	runtime, runner, _, manifest := runtimePruneFixture(t, true)
	ref := tobari.RuntimeRef(manifest.ID)
	result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), ref)
	if err != nil || result.State != tobari.RuntimeDeleted || result.RuntimeRef != ref || len(result.Items) != 1 ||
		result.Items[0].Disposition != tobari.RuntimePrunePreservedShared || result.RemovedTagCount != 1 || result.ReceiptRevision != 1 || result.ReclaimedBytes != nil {
		t.Fatalf("delete Runtime = %+v/%v", result, err)
	}
	if _, err := os.Lstat(runtime.runtimeDirectory(manifest.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Runtime directory remains: %v", err)
	}
	if len(runner.removals) != 1 || runner.removals[0] != managedLibraryRuntimeImage(manifest.Name, manifest.ID, manifest.Revisions[0].Revision) {
		t.Fatalf("Runtime image removals = %v", runner.removals)
	}
	if _, err := os.Lstat(runtime.runtimeDeleteJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Runtime delete journal remains: %v", err)
	}
	if receipt, err := runtime.readRuntimeDeleteReceiptObserved(manifest.ID); err != nil || receipt == nil || receipt.State != tobari.RuntimeDeleted {
		t.Fatalf("Runtime delete receipt = %+v/%v", receipt, err)
	}

	replayed, err := runtime.DeleteManagedRuntimeByReference(context.Background(), ref)
	if err != nil || replayed.State != tobari.RuntimeAlreadyDeleted || !reflect.DeepEqual(replayed.Items, result.Items) || len(runner.removals) != 1 {
		t.Fatalf("replayed Runtime delete = %+v/%v removals=%v", replayed, err, runner.removals)
	}

	created, err := runtime.CreateRuntime(context.Background(), manifest.Name, tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil || created.Runtime.ID == manifest.ID {
		t.Fatalf("same-name fresh Runtime = %+v/%v", created, err)
	}
	oldAgain, err := runtime.DeleteManagedRuntimeByReference(context.Background(), ref)
	if err != nil || oldAgain.State != tobari.RuntimeAlreadyDeleted {
		t.Fatalf("old Runtime reference after recreation = %+v/%v", oldAgain, err)
	}
	current, err := runtime.readRuntimeManifest(manifest.Name)
	if err != nil || current.ID != created.Runtime.ID {
		t.Fatalf("old receipt affected fresh Runtime = %+v/%v", current, err)
	}
}

func TestDeleteManagedRuntimeAllowsZeroRevisionDraft(t *testing.T) {
	root := t.TempDir()
	runner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}, containerLists: map[string]string{}}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "draft", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(created.Runtime.ID))
	if err != nil || result.State != tobari.RuntimeDeleted || len(result.Items) != 0 || result.SnapshotLogicalBytes != 0 || result.RemovedTagCount != 0 {
		t.Fatalf("zero-revision Runtime delete = %+v/%v", result, err)
	}
}

func TestReadRuntimeDeleteRecoveryIsExactAndZeroCreate(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}, containerLists: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotOwnedTree(t, root)
	if recovery, found, err := runtime.ReadRuntimeDeleteRecovery(context.Background()); err != nil || found || recovery != (tobari.RuntimeSummary{}) {
		t.Fatalf("fresh Runtime delete recovery = %+v found=%t err=%v", recovery, found, err)
	}
	if after := snapshotOwnedTree(t, root); !reflect.DeepEqual(after, before) {
		t.Fatalf("fresh Runtime delete recovery created state: before=%v after=%v", before, after)
	}

	runtime, _, _, manifest := runtimePruneFixture(t, false)
	runtime.runtimeDeleteReceiptWrite = func(tobari.RuntimeDeleteResult) error { return errors.New("synthetic receipt interruption") }
	if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) {
		t.Fatalf("create active Runtime delete journal = %v", err)
	}
	before = snapshotOwnedTree(t, filepath.Dir(runtime.configDirectory))
	recovery, found, err := runtime.ReadRuntimeDeleteRecovery(context.Background())
	if err != nil || !found || recovery.ID != manifest.ID || recovery.RuntimeRef != tobari.RuntimeRef(manifest.ID) || recovery.Name != manifest.Name || recovery.Kind != tobari.RuntimeKindManaged {
		t.Fatalf("active Runtime delete recovery = %+v found=%t err=%v", recovery, found, err)
	}
	if after := snapshotOwnedTree(t, filepath.Dir(runtime.configDirectory)); !reflect.DeepEqual(after, before) {
		t.Fatalf("active Runtime delete recovery mutated authority: before=%v after=%v", before, after)
	}
}

func TestDeleteManagedRuntimeResumesQuarantineAndReceiptBoundaries(t *testing.T) {
	t.Run("after quarantine rename", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		failed := true
		runtime.runtimeDeleteJournalWrite = func(_ *runtimeDeleteJournal, next runtimeDeleteJournal) error {
			if failed && next.Phase == runtimeDeleteQuarantined {
				failed = false
				return errors.New("synthetic process interruption after quarantine rename")
			}
			return writeAtomicJSON(runtime.runtimeDeleteJournalPath(), next)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) {
			t.Fatalf("quarantine interruption fault = %v", err)
		}
		journal, err := runtime.readRuntimeDeleteJournalObserved()
		if err != nil || journal == nil || journal.Phase != runtimeDeleteQuarantining {
			t.Fatalf("quarantine interruption journal = %+v/%v", journal, err)
		}
		if _, err := os.Lstat(runtime.runtimeDeleteQuarantinePath(manifest.ID)); err != nil {
			t.Fatalf("quarantined Runtime absent: %v", err)
		}
		assertProjectedRuntimeDelete(t, runtime, manifest.ID)
		result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
		if err != nil || result.State != tobari.RuntimeDeleted || len(runner.removals) != 1 {
			t.Fatalf("quarantine resume = %+v/%v removals=%v", result, err, runner.removals)
		}
	})

	t.Run("before receipt publication", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		failed := true
		runtime.runtimeDeleteReceiptWrite = func(result tobari.RuntimeDeleteResult) error {
			if failed {
				failed = false
				return errors.New("synthetic process interruption before receipt publication")
			}
			return writeAtomicJSON(runtime.runtimeDeleteReceiptPath(result.RuntimeID), result)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) {
			t.Fatalf("receipt interruption fault = %v", err)
		}
		journal, err := runtime.readRuntimeDeleteJournalObserved()
		if err != nil || journal == nil || journal.Phase != runtimeDeleteRemoved {
			t.Fatalf("receipt interruption journal = %+v/%v", journal, err)
		}
		assertProjectedRuntimeDelete(t, runtime, manifest.ID)
		result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
		if err != nil || result.State != tobari.RuntimeDeleted || len(runner.removals) != 1 {
			t.Fatalf("receipt resume = %+v/%v removals=%v", result, err, runner.removals)
		}
	})
}

func TestRuntimeDeleteLifecycleProjectsRetiringAuthorityAtCrashPhases(t *testing.T) {
	for _, stop := range []runtimeDeletePhase{runtimeDeleteQuarantined, runtimeDeleteRemoving} {
		t.Run(string(stop), func(t *testing.T) {
			runtime, runner, _, manifest := runtimePruneFixture(t, false)
			failed := true
			runtime.runtimeDeleteJournalWrite = func(_ *runtimeDeleteJournal, next runtimeDeleteJournal) error {
				boundary := next.Phase == runtimeDeleteRemoving
				if stop == runtimeDeleteRemoving {
					boundary = next.Phase == runtimeDeleteRemoved
				}
				if failed && boundary {
					failed = false
					return errors.New("synthetic process interruption at delete phase boundary")
				}
				return writeAtomicJSON(runtime.runtimeDeleteJournalPath(), next)
			}
			if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
				t.Fatal("delete phase interruption succeeded")
			}
			journal, err := runtime.readRuntimeDeleteJournalObserved()
			if err != nil || journal == nil || journal.Phase != stop {
				t.Fatalf("delete phase journal = %+v/%v, want %s", journal, err, stop)
			}
			assertProjectedRuntimeDelete(t, runtime, manifest.ID)
			result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
			if err != nil || result.State != tobari.RuntimeDeleted || len(runner.removals) != 1 {
				t.Fatalf("delete phase retry = %+v/%v removals=%v", result, err, runner.removals)
			}
		})
	}
}

func assertProjectedRuntimeDelete(t *testing.T, runtime *Runtime, runtimeID string) {
	t.Helper()
	snapshot, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read projected Runtime delete lifecycle: %v", err)
	}
	foundRuntime, foundStorage, foundActivity := false, false, false
	for _, manifest := range snapshot.Runtimes {
		if manifest.ID == runtimeID && manifest.RuntimeRef == tobari.RuntimeRef(runtimeID) {
			foundRuntime = true
		}
	}
	for _, storage := range snapshot.Storage {
		if storage.RuntimeID == runtimeID {
			foundStorage = true
		}
	}
	for _, activity := range snapshot.Journals.Active {
		if activity.Kind == tobari.RuntimeLifecycleActivityDelete && activity.RuntimeID == runtimeID {
			foundActivity = true
		}
	}
	if !foundRuntime || !foundStorage || !foundActivity {
		t.Fatalf("retiring Runtime projection is incomplete: runtime=%v storage=%v activity=%v snapshot=%+v", foundRuntime, foundStorage, foundActivity, snapshot)
	}
}

func TestDeleteManagedRuntimeQuarantineSharesConfigFilesystemAndFailsClosedOnDrift(t *testing.T) {
	t.Run("cross-device outcome remains resumable", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		first := true
		runtime.runtimeDeleteRename = func(source, destination string) error {
			if filepath.Dir(filepath.Dir(source)) != runtime.configDirectory || filepath.Dir(filepath.Dir(destination)) != runtime.configDirectory {
				t.Fatalf("delete quarantine crossed config filesystem boundary: %s -> %s", source, destination)
			}
			if first {
				first = false
				return syscall.EXDEV
			}
			return os.Rename(source, destination)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) || !errors.Is(err, syscall.EXDEV) {
			t.Fatalf("cross-device interruption fault = %v", err)
		}
		journal, err := runtime.readRuntimeDeleteJournalObserved()
		if err != nil || journal == nil || journal.Phase != runtimeDeleteQuarantining {
			t.Fatalf("cross-device interruption journal = %+v/%v", journal, err)
		}
		result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
		if err != nil || result.State != tobari.RuntimeDeleted || len(runner.removals) != 1 {
			t.Fatalf("cross-device retry = %+v/%v removals=%v", result, err, runner.removals)
		}
	})

	t.Run("quarantined revision drift", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		failed := true
		runtime.runtimeDeleteJournalWrite = func(_ *runtimeDeleteJournal, next runtimeDeleteJournal) error {
			if failed && next.Phase == runtimeDeleteQuarantined {
				failed = false
				return errors.New("synthetic process interruption after quarantine rename")
			}
			return writeAtomicJSON(runtime.runtimeDeleteJournalPath(), next)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
			t.Fatal("quarantine interruption succeeded")
		}
		revisionFile := filepath.Join(runtime.runtimeDeleteQuarantinePath(manifest.ID), "revisions", strings.TrimPrefix(manifest.Revisions[0].Revision, "sha256:"), "source", "Dockerfile")
		if err := os.WriteFile(revisionFile, []byte("FROM example.invalid/drift\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
			t.Fatal("drifted quarantined Runtime was removed")
		}
		if _, err := os.Lstat(runtime.runtimeDeleteQuarantinePath(manifest.ID)); err != nil {
			t.Fatalf("drifted quarantine was not preserved: %v", err)
		}
		if len(runner.removals) != 1 {
			t.Fatalf("drifted quarantine replayed image effect: %v", runner.removals)
		}
	})
}

func TestDeleteManagedRuntimeRejectsStandardAndUnsafeStateBeforeEffect(t *testing.T) {
	freshRoot := t.TempDir()
	fresh, err := newRuntime(filepath.Join(freshRoot, "config"), filepath.Join(freshRoot, "state"), &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.DeleteManagedRuntimeByReference(context.Background(), tobari.StandardRuntimeID); !errors.Is(err, tobari.ErrRuntimeDeleteProtected) {
		t.Fatalf("fresh standard Runtime delete fault = %v", err)
	}
	for _, path := range []string{fresh.configDirectory, fresh.stateDirectory} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("standard Runtime delete created %s: %v", path, err)
		}
	}

	runtime, runner, _, manifest := runtimePruneFixture(t, false)
	if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.StandardRuntimeID); !errors.Is(err, tobari.ErrRuntimeDeleteProtected) {
		t.Fatalf("standard Runtime delete fault = %v", err)
	}
	if len(runner.removals) != 0 {
		t.Fatalf("standard Runtime delete removed images: %v", runner.removals)
	}

	if err := os.MkdirAll(runtime.runtimeLifecycleDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.runtimeLifecycleDirectory(), "unknown"), []byte("unsafe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
		t.Fatal("unsafe lifecycle inventory authorized Runtime delete")
	}
	if len(runner.removals) != 0 {
		t.Fatalf("unsafe inventory removed images: %v", runner.removals)
	}
	if _, err := os.Lstat(runtime.runtimeDeleteJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe inventory created delete journal: %v", err)
	}
}

func TestDeleteManagedRuntimeFailsClosedOnCurrentContainerUse(t *testing.T) {
	runtime, runner, _, manifest := runtimePruneFixture(t, false)
	containerID := strings.Repeat("c", 64)
	digest := manifest.Revisions[0].ImageDigest
	runner.containerLists[digest] = containerID + "\n"
	runner.containers[containerID] = runtimeContainerObservation{ID: containerID, Image: digest, Owner: "foreign"}
	if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); !errors.Is(err, tobari.ErrRuntimeDeleteProtected) {
		t.Fatalf("in-use Runtime delete fault = %v", err)
	}
	if len(runner.removals) != 0 {
		t.Fatalf("in-use Runtime delete removed images: %v", runner.removals)
	}
	if _, err := os.Lstat(runtime.runtimeDeleteJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("in-use Runtime delete created a journal: %v", err)
	}
}

func TestDeleteManagedRuntimeResumesImageAndJournalCompletionOutcomes(t *testing.T) {
	t.Run("image removed before terminal write", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		failed := true
		runtime.runtimeDeleteJournalWrite = func(_ *runtimeDeleteJournal, next runtimeDeleteJournal) error {
			if failed && next.Phase == runtimeDeleteMaterials && next.Items[0].State == runtimePruneTerminal {
				failed = false
				return errors.New("synthetic process interruption after image removal")
			}
			return writeAtomicJSON(runtime.runtimeDeleteJournalPath(), next)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
			t.Fatal("post-image-removal interruption succeeded")
		}
		journal, err := runtime.readRuntimeDeleteJournalObserved()
		if err != nil || journal == nil || journal.Items[0].State != runtimePruneRemoving || len(runner.removals) != 1 {
			t.Fatalf("post-image-removal journal = %+v/%v removals=%v", journal, err, runner.removals)
		}
		result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
		if err != nil || result.State != tobari.RuntimeDeleted || len(runner.removals) != 1 {
			t.Fatalf("post-image-removal retry = %+v/%v removals=%v", result, err, runner.removals)
		}
	})

	t.Run("receipt before journal unlink", func(t *testing.T) {
		runtime, runner, _, manifest := runtimePruneFixture(t, false)
		failed := true
		runtime.runtimeDeleteJournalRemove = func(path string) error {
			if failed {
				failed = false
				return errors.New("synthetic process interruption before delete journal unlink")
			}
			return os.Remove(path)
		}
		if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
			t.Fatal("journal-unlink interruption succeeded")
		}
		result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
		if err != nil || result.State != tobari.RuntimeAlreadyDeleted || len(runner.removals) != 1 {
			t.Fatalf("journal-unlink retry = %+v/%v removals=%v", result, err, runner.removals)
		}
	})
}

func TestRuntimeBuildAndRestoreRecoveryAreZeroEffectDuringDelete(t *testing.T) {
	runtime, runner, _, manifest := runtimePruneFixture(t, false)
	runtime.runtimeDeleteBeforeImageRemove = func(tobari.RuntimePruneCandidate) error {
		return errors.New("synthetic interruption before Runtime delete image effect")
	}
	if _, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID)); err == nil {
		t.Fatal("delete interruption succeeded")
	}
	journalBefore, err := os.ReadFile(runtime.runtimeDeleteJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	callsBefore := len(runner.outputs)
	for _, kind := range []tobari.RuntimeBuildRecoveryKind{
		tobari.RuntimeBuildRecoveryPreDocker,
		tobari.RuntimeBuildRecoveryBuilding,
		tobari.RuntimeBuildRecoveryPublication,
		tobari.RuntimeBuildRecoveryCleanup,
		tobari.RuntimeBuildRecoveryOrphan,
		tobari.RuntimeBuildRecoveryFailed,
	} {
		if err := runtime.RecoverRuntimeBuildByReference(context.Background(), tobari.RuntimeRef(manifest.ID), kind); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) {
			t.Fatalf("%s recovery during delete fault = %v", kind, err)
		}
	}
	if _, err := runtime.RecoverRuntimeRestoreByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, manifest.Revisions[0].Revision), tobari.RuntimeBuildRecoveryFailed, io.Discard); !errors.Is(err, tobari.ErrRuntimeDeleteInterrupted) {
		t.Fatalf("restore recovery during delete fault = %v", err)
	}
	journalAfter, err := os.ReadFile(runtime.runtimeDeleteJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(journalAfter, journalBefore) || len(runner.outputs) != callsBefore || len(runner.removals) != 0 {
		t.Fatalf("recovery crossed active delete authority: journal_changed=%v calls=%d/%d removals=%v", !reflect.DeepEqual(journalAfter, journalBefore), callsBefore, len(runner.outputs), runner.removals)
	}
	runtime.runtimeDeleteBeforeImageRemove = nil
	result, err := runtime.DeleteManagedRuntimeByReference(context.Background(), tobari.RuntimeRef(manifest.ID))
	if err != nil || result.State != tobari.RuntimeDeleted {
		t.Fatalf("delete retry after blocked recoveries = %+v/%v", result, err)
	}
}

func TestRuntimeDeleteJournalRejectsAuthorityAndPhaseDrift(t *testing.T) {
	runtime, _, _, manifest := runtimePruneFixture(t, false)
	snapshot, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target, err := tobari.RuntimeDeleteTargetFrom(snapshot, tobari.RuntimeRef(manifest.ID))
	if err != nil {
		t.Fatal(err)
	}
	base := runtimeDeleteJournal{SchemaVersion: runtimeDeleteJournalSchema, Target: target, Phase: runtimeDeleteMaterials, Items: []runtimePruneJournalItem{{Candidate: target.Materials[0].Candidate, State: runtimePrunePending}}}
	valid := cloneRuntimeDeleteJournal(base)
	valid.Items[0].State = runtimePruneRemoving
	if err := validateRuntimeDeleteJournalTransition(base, valid); err != nil {
		t.Fatalf("valid delete transition: %v", err)
	}
	invalid := []runtimeDeleteJournal{valid, valid, valid}
	invalid[0].Target.Runtime.ID = "018bcfe5-687b-7000-8000-000000000099"
	invalid[1].Items[0].Candidate.Name = "replacement"
	invalid[2].Phase = runtimeDeleteRemoved
	for _, next := range invalid {
		if err := validateRuntimeDeleteJournalTransition(base, next); err == nil {
			t.Fatalf("invalid delete transition passed: %+v", next)
		}
	}
}
