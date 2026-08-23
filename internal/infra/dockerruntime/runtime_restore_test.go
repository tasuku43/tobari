package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func restoreRuntimeFixture(t *testing.T, recordedDigest string) (*Runtime, *managedRuntimeBuildRunner, tobari.RuntimeManifest) {
	t.Helper()
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	manifest := installRuntimeLifecycleRevision(t, runtime, "018bcfe5-687b-7000-8000-000000000077", "frontend", recordedDigest, "FROM scratch\nLABEL io.tobari.runtime-api=1\n")
	return runtime, runner, manifest
}

func TestRuntimeRestoreRebuildsExactRetainedRevisionWithoutHistoryMutation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, runner, before := restoreRuntimeFixture(t, digest)
	revision := before.Revisions[0]
	reference := tobari.RuntimeRevisionRef(before.ID, revision.Revision)

	result, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), reference, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil || result.State != tobari.RuntimeRestored || result.RevisionRef != reference || !result.DigestMatch || result.RevisionAppended || result.ManifestChanged || result.WorkspaceChanged {
		t.Fatalf("restore result = %+v/%v", result, err)
	}
	after, err := runtime.readRuntimeManifest(before.Name)
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("restore changed Runtime history: before=%+v after=%+v error=%v", before, after, err)
	}
	image, exists := runner.images[revision.Image]
	if !exists || image.id != digest {
		t.Fatalf("restored normal image = %+v, exists=%t", image, exists)
	}
	if _, exists := runner.images[managedRuntimeStagingImage(before.ID, revision.Revision)]; exists {
		t.Fatal("successful restore retained a staging tag")
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("successful restore retained journal: %+v/%v", journal, err)
	}
	builds := 0
	for _, call := range runner.runs {
		if len(call.args) >= 2 && call.args[0] == "buildx" && call.args[1] == "build" {
			builds++
			if slices.Contains(call.args, "--pull") {
				t.Fatal("exact restore refreshed a mutable base image")
			}
		}
	}
	if builds != 1 {
		t.Fatalf("restore build effects = %d", builds)
	}
}

func TestRuntimeRestoreReturnsAlreadyAvailableWithoutMutation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, runner, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	runner.images[revision.Image] = managedRuntimeTestImage{id: digest, labels: map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: manifest.ID, managedRuntimeRevisionLabel: revision.Revision,
	}}
	root := filepath.Dir(runtime.configDirectory)
	beforeTree := snapshotOwnedTree(t, root)
	if _, err := os.Lstat(runtime.stateDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("already-available fixture has state authority: %v", err)
	}
	beforeRuns := len(runner.runs)
	result, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil)
	if err != nil || result.State != tobari.RuntimeAlreadyAvailable || result.ArtifactDisposition != tobari.RuntimeRestoreArtifactNotCreated {
		t.Fatalf("already-available restore = %+v/%v", result, err)
	}
	if len(runner.runs) != beforeRuns {
		t.Fatalf("already-available restore crossed mutation boundary: %+v", runner.runs[beforeRuns:])
	}
	if afterTree := snapshotOwnedTree(t, root); !reflect.DeepEqual(afterTree, beforeTree) {
		t.Fatalf("already-available restore changed durable tree\nbefore=%v\nafter=%v", beforeTree, afterTree)
	}
	if _, err := os.Lstat(runtime.stateDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("already-available restore created state or lifecycle lock: %v", err)
	}
}

func TestRuntimeRestoreAlreadyAvailableRejectsAuthorityDriftWithoutCreatingState(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, runner, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	runner.images[revision.Image] = managedRuntimeTestImage{id: digest, labels: map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: manifest.ID, managedRuntimeRevisionLabel: revision.Revision,
	}}
	root := filepath.Dir(runtime.configDirectory)
	mutated := false
	runner.afterImageInspect = func(args []string) {
		if mutated || len(args) < 4 || !strings.Contains(args[3], tobari.RuntimeImageAPILabel) {
			return
		}
		mutated = true
		drifted := manifest
		drifted.Revisions = append([]tobari.RuntimeRevision{}, manifest.Revisions...)
		drifted.Revisions[0].ImageDigest = "sha256:" + strings.Repeat("d", 64)
		if err := writeAtomicJSON(runtime.runtimeManifestPath(manifest.Name), drifted); err != nil {
			t.Fatalf("mutate Runtime authority during observation: %v", err)
		}
	}
	_, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil)
	if !errors.Is(err, tobari.ErrRuntimeRetirementObservationUnknown) || !mutated {
		t.Fatalf("already-available drift result = %v, mutated=%t", err, mutated)
	}
	if _, err := os.Lstat(runtime.stateDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("drifted read-only restore created state: %v; tree=%v", err, snapshotOwnedTree(t, root))
	}
}

func TestRuntimeRestoreDigestMismatchRetainsExactRecoveryAuthority(t *testing.T) {
	recorded := "sha256:" + strings.Repeat("d", 64)
	runtime, runner, manifest := restoreRuntimeFixture(t, recorded)
	revision := manifest.Revisions[0]
	_, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil)
	if !errors.Is(err, tobari.ErrRuntimeRevisionUnrestorable) {
		t.Fatalf("restore mismatch error = %v", err)
	}
	if _, exists := runner.images[revision.Image]; exists {
		t.Fatal("digest-mismatched restore published the normal tag")
	}
	journal, journalErr := runtime.readRuntimeBuildJournalObserved()
	if journalErr != nil || journal == nil || !journal.Restore || journal.Phase != runtimeBuildPhaseFailed || journal.ExpectedImageDigest != recorded || journal.ImageDigest == recorded || journal.StagingArtifact != runtimeBuildStagingOwned {
		t.Fatalf("restore mismatch journal = %+v/%v", journal, journalErr)
	}
	observed, _, observationErr := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if observationErr != nil || len(observed.Journals.Active) != 1 || observed.Journals.Active[0].Kind != tobari.RuntimeLifecycleActivityRestore || len(observed.Journals.FailedBuilds) != 0 {
		t.Fatalf("restore recovery inventory = %+v/%v", observed.Journals, observationErr)
	}
}

func TestRuntimeRestoreBuildingInterruptionRecoversWithoutManifestRewrite(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, _, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	runtime.runtimeBuildJournalWrite = func(previous, next runtimeBuildJournal) error {
		if previous.Phase == runtimeBuildPhaseBuilding && next.Phase == runtimeBuildPhaseBuilt {
			return errors.New("synthetic process interruption before built journal publication")
		}
		return writeAtomicJSON(runtime.runtimeBuildJournalPath(), next)
	}
	_, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil)
	if err == nil {
		t.Fatal("restore interruption unexpectedly succeeded")
	}
	journal, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseBuilding || !journal.Restore {
		t.Fatalf("interrupted restore journal = %+v/%v", journal, err)
	}
	runtime.runtimeBuildJournalWrite = nil
	if err := runtime.RecoverRuntimeBuildBuilding(context.Background()); err != nil {
		t.Fatal(err)
	}
	journal, err = runtime.readRuntimeBuildJournalObserved()
	if err != nil || journal == nil || journal.Phase != runtimeBuildPhaseBuilt || journal.ImageDigest != digest || journal.CreatedAt != revision.CreatedAt.Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("reconciled restore journal = %+v/%v", journal, err)
	}
	if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := runtime.readRuntimeManifest(manifest.Name)
	if err != nil || !reflect.DeepEqual(after, manifest) {
		t.Fatalf("recovered restore changed history: %+v/%v", after, err)
	}
}

func TestRuntimeRestoreRecoveryUsesOneExactReviewedRevisionWorkflow(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, _, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	reference := tobari.RuntimeRevisionRef(manifest.ID, revision.Revision)
	runtime.runtimeBuildJournalWrite = func(previous, next runtimeBuildJournal) error {
		if previous.Phase == runtimeBuildPhaseBuilding && next.Phase == runtimeBuildPhaseBuilt {
			return errors.New("synthetic process interruption before built journal publication")
		}
		return writeAtomicJSON(runtime.runtimeBuildJournalPath(), next)
	}
	if _, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), reference, nil); !errors.Is(err, tobari.ErrRuntimeRestoreInterrupted) {
		t.Fatalf("interrupted restore error = %v", err)
	}
	runtime.runtimeBuildJournalWrite = nil
	recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
	if err != nil || !found || recovery.RevisionRef != reference || recovery.Kind != tobari.RuntimeBuildRecoveryBuilding || recovery.RestoreFailed {
		t.Fatalf("restore recovery authority = %+v/%t/%v", recovery, found, err)
	}
	result, err := runtime.RecoverRuntimeRestoreByRevisionReference(context.Background(), recovery.RevisionRef, recovery.Kind, nil)
	if err != nil || result.State != tobari.RuntimeRestored || result.ArtifactDisposition != tobari.RuntimeRestoreArtifactRemoved || result.RevisionRef != reference {
		t.Fatalf("one-step restore recovery = %+v/%v", result, err)
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("one-step restore recovery retained journal: %+v/%v", journal, err)
	}
}

func TestRuntimeRestoreRecoveryRejectsReferenceDriftBeforeMutation(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, _, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	reference := tobari.RuntimeRevisionRef(manifest.ID, revision.Revision)
	runtime.runtimeBuildJournalWrite = func(previous, next runtimeBuildJournal) error {
		if previous.Phase == runtimeBuildPhaseBuilding && next.Phase == runtimeBuildPhaseBuilt {
			return errors.New("synthetic interruption")
		}
		return writeAtomicJSON(runtime.runtimeBuildJournalPath(), next)
	}
	if _, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), reference, nil); err == nil {
		t.Fatal("restore interruption unexpectedly succeeded")
	}
	runtime.runtimeBuildJournalWrite = nil
	before, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || before == nil {
		t.Fatalf("interrupted restore journal = %+v/%v", before, err)
	}
	wrong := tobari.RuntimeRevisionRef(manifest.ID, "sha256:"+strings.Repeat("d", 64))
	if _, err := runtime.RecoverRuntimeRestoreByRevisionReference(context.Background(), wrong, tobari.RuntimeBuildRecoveryBuilding, nil); err == nil {
		t.Fatal("drifted restore recovery reference was accepted")
	}
	after, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || after == nil || *after != *before {
		t.Fatalf("drifted restore recovery changed journal: before=%+v after=%+v error=%v", before, after, err)
	}
	if err := runtime.RecoverRuntimeBuildByReference(context.Background(), manifest.ID, tobari.RuntimeBuildRecoveryBuilding); err == nil {
		t.Fatal("Runtime-only build recovery accepted restore authority")
	}
}

func TestRuntimeRestoreFailedRecoveryCleansOnceAndReportsUnrestorable(t *testing.T) {
	recorded := "sha256:" + strings.Repeat("d", 64)
	runtime, _, manifest := restoreRuntimeFixture(t, recorded)
	revision := manifest.Revisions[0]
	reference := tobari.RuntimeRevisionRef(manifest.ID, revision.Revision)
	if _, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), reference, nil); !errors.Is(err, tobari.ErrRuntimeRestoreInterrupted) {
		t.Fatalf("digest-mismatched restore error = %v", err)
	}
	recovery, found, err := runtime.ReadRuntimeBuildRecovery(context.Background())
	if err != nil || !found || recovery.RevisionRef != reference || recovery.Kind != tobari.RuntimeBuildRecoveryFailed || !recovery.RestoreFailed {
		t.Fatalf("failed restore recovery authority = %+v/%t/%v", recovery, found, err)
	}
	if _, err := runtime.RecoverRuntimeRestoreByRevisionReference(context.Background(), reference, recovery.Kind, nil); !errors.Is(err, tobari.ErrRuntimeRevisionUnrestorable) {
		t.Fatalf("failed restore recovery result = %v", err)
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("failed restore recovery retained journal: %+v/%v", journal, err)
	}
}

func TestRuntimeRestoreJournalRejectsOperationOrDigestDrift(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
	if err != nil {
		t.Fatal(err)
	}
	id := "018bcfe5-687b-7000-8000-000000000077"
	revision := "sha256:" + strings.Repeat("a", 64)
	journal := runtimeBuildJournal{
		SchemaVersion: runtimeBuildJournalSchema, Phase: runtimeBuildPhaseSnapshotting, Restore: true,
		RuntimeID: id, RuntimeName: "frontend", AttemptID: strings.Repeat("b", 64),
		Revision: revision, StagingImage: managedRuntimeStagingImage(id, revision),
		FinalImage: managedLibraryRuntimeImage("frontend", id, revision), ExpectedImageDigest: "sha256:" + strings.Repeat("c", 64),
		SnapshotPath: runtime.runtimeBuildSnapshotPath(), CreatedAt: "1970-01-01T00:00:01Z",
	}
	if err := journal.Validate(runtime); err != nil {
		t.Fatal(err)
	}
	prepared := journal
	prepared.Phase = runtimeBuildPhasePrepared
	if err := validateRuntimeBuildJournalTransition(journal, prepared); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*runtimeBuildJournal){
		func(value *runtimeBuildJournal) { value.Restore = false },
		func(value *runtimeBuildJournal) { value.ExpectedImageDigest = "sha256:" + strings.Repeat("d", 64) },
		func(value *runtimeBuildJournal) { value.CreatedAt = "1970-01-01T00:00:02Z" },
	} {
		changed := prepared
		mutate(&changed)
		if err := validateRuntimeBuildJournalTransition(journal, changed); err == nil {
			t.Fatalf("restore journal authority drift accepted: %+v", changed)
		}
	}
}

func TestRuntimeRestorePublicationResumesEveryJournalBoundary(t *testing.T) {
	for _, phase := range []string{
		runtimeBuildPhaseFinalTagged,
		runtimeBuildPhaseStagingReleased,
		runtimeBuildPhaseSnapshotPublished,
		runtimeBuildPhaseManifestCommitted,
	} {
		for _, afterWrite := range []bool{false, true} {
			t.Run(phase+map[bool]string{false: "_before_write", true: "_after_write"}[afterWrite], func(t *testing.T) {
				digest := "sha256:" + strings.Repeat("c", 64)
				runtime, runner, manifest := restoreRuntimeFixture(t, digest)
				revision := manifest.Revisions[0]
				runtime.runtimeBuildJournalWrite = func(_ runtimeBuildJournal, next runtimeBuildJournal) error {
					if next.Phase != phase {
						return writeAtomicJSON(runtime.runtimeBuildJournalPath(), next)
					}
					if afterWrite {
						if err := writeAtomicJSON(runtime.runtimeBuildJournalPath(), next); err != nil {
							return err
						}
					}
					return errors.New("synthetic restore publication interruption")
				}
				_, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil)
				if err == nil {
					t.Fatal("restore publication interruption unexpectedly succeeded")
				}
				runtime.runtimeBuildJournalWrite = nil
				if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err != nil {
					t.Fatal(err)
				}
				after, err := runtime.readRuntimeManifest(manifest.Name)
				if err != nil || !reflect.DeepEqual(after, manifest) {
					t.Fatalf("recovered restore changed manifest: %+v/%v", after, err)
				}
				if image, exists := runner.images[revision.Image]; !exists || image.id != digest {
					t.Fatalf("recovered restore image = %+v/%t", image, exists)
				}
				if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
					t.Fatalf("recovered restore retained journal: %+v/%v", journal, err)
				}
			})
		}
	}
}
