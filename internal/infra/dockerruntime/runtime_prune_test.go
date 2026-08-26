package dockerruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimePruneRunner struct {
	*lifecycleObservationRunner
	removals        []string
	failRemove      bool
	removeThenFail  bool
	keepTag         bool
	preserveContent bool
}

type failedRuntimePruneRunner struct {
	build     *managedRuntimeBuildRunner
	lifecycle *lifecycleObservationRunner
}

func (r *failedRuntimePruneRunner) Run(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	if (len(args) >= 5 && args[0] == "image" && args[1] == "inspect" && strings.Contains(args[3], `"repo_tags"`)) || (len(args) >= 2 && args[0] == "container") {
		return r.lifecycle.Run(ctx, args, environment, in, out, errOut)
	}
	if len(args) == 3 && args[0] == "image" && args[1] == "rm" {
		fixture := r.lifecycle.images[args[2]]
		err := r.build.Run(ctx, args, environment, in, out, errOut)
		if err == nil {
			delete(r.lifecycle.images, args[2])
			delete(r.lifecycle.images, fixture.observation.ID)
		}
		return err
	}
	return r.build.Run(ctx, args, environment, in, out, errOut)
}

func (r *failedRuntimePruneRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	return r.build.Output(ctx, args, environment)
}

func (r *runtimePruneRunner) Run(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	if len(args) == 3 && args[0] == "image" && args[1] == "rm" {
		r.removals = append(r.removals, args[2])
		if (!r.failRemove || r.removeThenFail) && !r.keepTag {
			fixture := r.images[args[2]]
			delete(r.images, args[2])
			if fixture.observation.ID != "" {
				content := r.images[fixture.observation.ID]
				if r.preserveContent {
					content.observation.RepoTags = slices.DeleteFunc(content.observation.RepoTags, func(tag string) bool { return tag == args[2] })
					r.images[fixture.observation.ID] = content
				} else {
					delete(r.images, fixture.observation.ID)
				}
			}
		}
		if r.failRemove {
			return errors.New("synthetic image removal uncertainty")
		}
		return nil
	}
	return r.lifecycleObservationRunner.Run(ctx, args, environment, in, out, errOut)
}

func runtimePruneFixture(t *testing.T, preserveContent bool) (*Runtime, *runtimePruneRunner, tobari.RuntimePrunePlan, tobari.RuntimeManifest) {
	t.Helper()
	root := t.TempDir()
	base := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{}, containers: map[string]runtimeContainerObservation{}, containerLists: map[string]string{}}
	runner := &runtimePruneRunner{lifecycleObservationRunner: base, preserveContent: preserveContent}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	id := "018bcfe5-687b-7000-8000-000000000077"
	digest := "sha256:" + strings.Repeat("b", 64)
	manifest := installRuntimeLifecycleRevision(t, runtime, id, "frontend", digest, "FROM example.invalid/runtime\n")
	selector := managedLibraryRuntimeImage(manifest.Name, manifest.ID, manifest.Revisions[0].Revision)
	observation := managedLifecycleImage(manifest.ID, manifest.Revisions[0].Revision, selector)
	if preserveContent {
		observation.RepoTags = append(observation.RepoTags, "example.invalid/shared:keep")
	}
	runner.images[selector] = lifecycleImageFixture{observation: observation}
	runner.images[digest] = lifecycleImageFixture{observation: observation}
	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatalf("read split Runtime lifecycle snapshot: %v", err)
	}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil || !plan.Applicable || len(plan.Candidates) != 1 {
		t.Fatalf("split Runtime prune plan = %+v/%v", plan, err)
	}
	return runtime, runner, plan, manifest
}

func TestApplyRuntimePruneIsExactIdempotentAndPreservesDurableRuntime(t *testing.T) {
	runtime, runner, plan, manifest := runtimePruneFixture(t, false)
	paths := []string{runtime.runtimeManifestPath(manifest.ID), manifest.SourcePath, manifest.Revisions[0].SnapshotPath}
	before := make([]os.FileInfo, len(paths))
	for index, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[index] = info
	}

	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || len(result.Items) != 1 || result.Items[0].Disposition != tobari.RuntimePruneRemoved || result.Items[0].LastUsed != tobari.RuntimeLastUsedUnknown || result.RemovedTagCount != 1 || result.ReceiptRevision != 1 || result.ReclaimedBytes != nil {
		t.Fatalf("apply Runtime prune = %+v/%v", result, err)
	}
	if len(runner.removals) != 1 || runner.removals[0] != managedLibraryRuntimeImage(manifest.Name, manifest.ID, manifest.Revisions[0].Revision) {
		t.Fatalf("image removals = %v", runner.removals)
	}
	fresh, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || len(fresh.Materials) != 1 || fresh.Materials[0].Availability != tobari.RuntimeAvailabilityPruned {
		t.Fatalf("post-prune lifecycle material = %+v/%v", fresh.Materials, err)
	}
	selector := managedLibraryRuntimeImage(manifest.Name, manifest.ID, manifest.Revisions[0].Revision)
	reappeared := managedLifecycleImage(manifest.ID, manifest.Revisions[0].Revision, selector)
	runner.images[selector] = lifecycleImageFixture{observation: reappeared}
	runner.images[manifest.Revisions[0].ImageDigest] = lifecycleImageFixture{observation: reappeared}
	fresh, _, err = runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || len(fresh.Materials) != 1 || fresh.Materials[0].Availability != tobari.RuntimeAvailabilityAvailable {
		t.Fatalf("reappeared lifecycle material = %+v/%v", fresh.Materials, err)
	}
	for index, path := range paths {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before[index], after) {
			t.Fatalf("durable Runtime path changed %s: %v", path, err)
		}
	}

	replayed, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || replayed.State != tobari.RuntimePruneAlreadyApplied || replayed.ReceiptRevision != result.ReceiptRevision || len(runner.removals) != 1 {
		t.Fatalf("replayed Runtime prune = %+v/%v removals=%v", replayed, err, runner.removals)
	}
}

func TestRuntimePruneReceiptRetirementRequiresExactStableAuthority(t *testing.T) {
	runtime, _, plan, manifest := runtimePruneFixture(t, false)
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil {
		t.Fatal(err)
	}
	store, err := runtime.readRuntimePruneReceiptStoreObserved()
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("receipt store = %+v/%v", store, err)
	}
	target := runtimeMaterialTarget{RuntimeID: manifest.ID, Revision: manifest.Revisions[0].Revision, TagRole: tobari.RuntimeMaterialTagPublishedRevision, Name: manifest.Name}
	if !runtimePruneReceiptRetired(store, target) {
		t.Fatal("exact terminal receipt did not prove retirement")
	}
	newID := target
	newID.RuntimeID = "018bcfe5-687b-7000-8000-000000000099"
	if runtimePruneReceiptRetired(store, newID) {
		t.Fatal("same-name fresh Runtime ID inherited stale prune authority")
	}
	otherRevision := target
	otherRevision.Revision = "sha256:" + strings.Repeat("e", 64)
	if runtimePruneReceiptRetired(store, otherRevision) {
		t.Fatal("different semantic revision inherited prune authority")
	}
	staging := target
	staging.TagRole = tobari.RuntimeMaterialTagJournaledStaging
	if runtimePruneReceiptRetired(store, staging) {
		t.Fatal("failed-build staging material inherited revision prune authority")
	}
}

func TestRuntimeRestoreSupersedesPrunedAvailabilityAndPreservesPlanReplay(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, runner, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	labels := map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: manifest.ID, managedRuntimeRevisionLabel: revision.Revision,
	}
	runner.images[revision.Image] = managedRuntimeTestImage{id: digest, labels: labels}

	beforeSnapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	beforePlan, err := tobari.PlanRuntimePrune(beforeSnapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), beforePlan.PlanRef); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil); err != nil {
		t.Fatal(err)
	}

	restoredSnapshot, restoredAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || restoredSnapshot.Materials[0].Availability != tobari.RuntimeAvailabilityAvailable || len(restoredSnapshot.RetirementGenerations) != 1 || restoredSnapshot.RetirementGenerations[0].Generation != 1 {
		t.Fatalf("restored lifecycle authority = %+v/%v", restoredSnapshot, err)
	}
	restoredPlan, err := tobari.PlanRuntimePrune(restoredSnapshot, restoredAt)
	if err != nil || restoredPlan.PlanRef == beforePlan.PlanRef {
		t.Fatalf("post-restore prune plan = %+v/%v; old=%q", restoredPlan, err, beforePlan.PlanRef)
	}
	if replay, err := runtime.ApplyRuntimePrune(context.Background(), beforePlan.PlanRef); err != nil || replay.State != tobari.RuntimePruneAlreadyApplied {
		t.Fatalf("old prune receipt replay = %+v/%v", replay, err)
	}
	if _, exists := runner.images[revision.Image]; !exists {
		t.Fatal("old prune plan replay removed the restored image")
	}

	// A later ordinary external disappearance is missing, not attributed to the
	// older Tobari prune whose evidence the restore durably superseded.
	delete(runner.images, revision.Image)
	missing, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || missing.Materials[0].Availability != tobari.RuntimeAvailabilityMissing {
		t.Fatalf("post-restore external disappearance = %+v/%v", missing.Materials, err)
	}

	// Reappearance can be reviewed again, and the new plan outruns the old
	// supersession with a new receipt revision.
	runner.images[revision.Image] = managedRuntimeTestImage{id: digest, labels: labels}
	result, err := runtime.ApplyRuntimePrune(context.Background(), restoredPlan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || result.ReceiptRevision != 2 {
		t.Fatalf("post-restore prune = %+v/%v", result, err)
	}
	pruned, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || pruned.Materials[0].Availability != tobari.RuntimeAvailabilityPruned {
		t.Fatalf("new prune did not outrun restore supersession = %+v/%v", pruned.Materials, err)
	}
}

func TestRuntimeRestorePersistsPruneSupersessionBeforeCleanupRecovery(t *testing.T) {
	digest := "sha256:" + strings.Repeat("c", 64)
	runtime, runner, manifest := restoreRuntimeFixture(t, digest)
	revision := manifest.Revisions[0]
	labels := map[string]string{
		ownerLabel: ownerValue, componentLabel: managedRuntimeComponentLabel,
		managedRuntimeIDLabel: manifest.ID, managedRuntimeRevisionLabel: revision.Revision,
	}
	runner.images[revision.Image] = managedRuntimeTestImage{id: digest, labels: labels}
	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err != nil {
		t.Fatal(err)
	}

	runtime.runtimeBuildCompletionWrite = func(runtimeBuildJournal) error {
		return errors.New("synthetic interruption before completing publication")
	}
	if _, err := runtime.RestoreManagedRuntimeByRevisionReference(context.Background(), tobari.RuntimeRevisionRef(manifest.ID, revision.Revision), nil); !errors.Is(err, tobari.ErrRuntimeRestoreInterrupted) {
		t.Fatalf("interrupted restore = %v", err)
	}
	store, err := runtime.readRuntimePruneReceiptStoreObserved()
	journal, journalErr := runtime.readRuntimeBuildJournalObserved()
	if err != nil || journalErr != nil || runtimePruneSupersededThrough(store, manifest.ID, revision.Revision) != 1 || journal == nil || journal.Phase != runtimeBuildPhaseManifestCommitted {
		t.Fatalf("interrupted restore authority = store:%+v journal:%+v errors:%v/%v", store, journal, err, journalErr)
	}

	runtime.runtimeBuildCompletionWrite = nil
	if err := runtime.RecoverRuntimeBuildPublication(context.Background()); err != nil {
		t.Fatal(err)
	}
	if journal, err := runtime.readRuntimeBuildJournalObserved(); err != nil || journal != nil {
		t.Fatalf("restore recovery retained journal = %+v/%v", journal, err)
	}
	delete(runner.images, revision.Image)
	missing, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil || missing.Materials[0].Availability != tobari.RuntimeAvailabilityMissing {
		t.Fatalf("recovered restore external disappearance = %+v/%v", missing.Materials, err)
	}
}

func TestApplyRuntimePrunePreservesSharedContent(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, true)
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || len(result.Items) != 1 || result.Items[0].Disposition != tobari.RuntimePrunePreservedShared || result.Items[0].RemovedTagCount != 1 {
		t.Fatalf("shared Runtime prune = %+v/%v", result, err)
	}
	if len(runner.removals) != 1 {
		t.Fatalf("shared tag removals = %v", runner.removals)
	}
}

func TestApplyRuntimePruneResumesJournalAndUnknownRemoveOutcome(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, false)
	interrupted := true
	runtime.runtimePruneBeforeRemove = func(tobari.RuntimePruneCandidate) error {
		if interrupted {
			interrupted = false
			return errors.New("synthetic process interruption")
		}
		return nil
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("interrupted Runtime prune succeeded")
	}
	journal, err := runtime.readRuntimePruneJournalObserved()
	if err != nil || journal == nil || journal.Items[0].State != runtimePruneRemoving || len(runner.removals) != 0 {
		t.Fatalf("interrupted journal/removals = %+v/%v/%v", journal, err, runner.removals)
	}
	runtime.runtimePruneBeforeRemove = nil
	runner.failRemove, runner.removeThenFail = true, true
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || len(runner.removals) != 1 {
		t.Fatalf("resume before remove = %+v/%v removals=%v", result, err, runner.removals)
	}
	// A removing journal revalidates the same exact tag. If it is still present,
	// the retry may safely issue the same exact untag effect.
	if result.Items[0].Disposition != tobari.RuntimePruneRemoved {
		t.Fatalf("resumed disposition = %q", result.Items[0].Disposition)
	}
}

func TestApplyRuntimePruneSettlesDockerOutcomeUnknown(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, false)
	runner.failRemove, runner.removeThenFail = true, true
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || len(runner.removals) != 1 || result.Items[0].Disposition != tobari.RuntimePruneRemoved {
		t.Fatalf("outcome-unknown Runtime prune = %+v/%v removals=%v", result, err, runner.removals)
	}
}

func TestApplyRuntimePruneRetainsJournalWhenDockerLeavesTag(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, false)
	runner.failRemove = true
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("failed image removal succeeded")
	}
	journal, err := runtime.readRuntimePruneJournalObserved()
	if err != nil || journal == nil || journal.Items[0].State != runtimePruneRemoving || len(runner.removals) != 1 {
		t.Fatalf("failed removal journal = %+v/%v removals=%v", journal, err, runner.removals)
	}
	runner.failRemove = false
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || len(runner.removals) != 2 {
		t.Fatalf("failed removal retry = %+v/%v removals=%v", result, err, runner.removals)
	}
}

func TestApplyRuntimePruneRetiresSettledFailedBuildAndResumesCleanup(t *testing.T) {
	runtime, buildRunner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingOwned)
	settled := failed
	settled.AttemptSettlement = runtimeBuildAttemptSettled
	if err := runtime.writeRuntimeBuildJournal(failed, settled); err != nil {
		t.Fatal(err)
	}
	observation := runtimeImageObservation{ID: settled.ImageDigest, Size: 4096, RepoTags: []string{settled.StagingImage}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: settled.RuntimeID, Revision: settled.Revision}
	lifecycleRunner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{settled.StagingImage: {observation: observation}, settled.ImageDigest: {observation: observation}}, containers: map[string]runtimeContainerObservation{}, containerLists: map[string]string{}}
	runtime.runner = &failedRuntimePruneRunner{build: buildRunner, lifecycle: lifecycleRunner}
	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil || len(plan.Candidates) != 1 || plan.Candidates[0].Kind != tobari.RuntimePruneCandidateFailedBuild {
		t.Fatalf("failed-build prune plan = %+v/%v", plan, err)
	}

	firstCleanup := true
	runtime.runtimeBuildCleanup = func(completing runtimeBuildJournal) error {
		if firstCleanup {
			firstCleanup = false
			return errors.New("synthetic process interruption during failed-build cleanup")
		}
		runtime.runtimeBuildCleanup = nil
		return runtime.completeRuntimeBuildJournal(context.Background(), completing)
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("interrupted failed-build cleanup succeeded")
	}
	prune, err := runtime.readRuntimePruneJournalObserved()
	if err != nil || prune == nil || prune.Items[0].State != runtimePruneTerminal {
		t.Fatalf("failed-build prune journal = %+v/%v", prune, err)
	}
	build, err := runtime.readRuntimeBuildJournalObserved()
	if err != nil || build == nil || build.Phase != runtimeBuildPhaseCompleting || build.CleanupFrom != runtimeBuildPhaseFailed {
		t.Fatalf("failed-build cleanup journal = %+v/%v", build, err)
	}
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || result.Items[0].Kind != tobari.RuntimePruneCandidateFailedBuild {
		t.Fatalf("resumed failed-build prune = %+v/%v", result, err)
	}
	if build, err := runtime.readRuntimeBuildJournalObserved(); err != nil || build != nil {
		t.Fatalf("failed-build journal remains = %+v/%v", build, err)
	}
	if _, err := os.Lstat(filepath.Dir(settled.SnapshotPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed-build staging snapshot remains: %v", err)
	}
}

func TestApplyRuntimePruneResumesAfterFailedBuildAuthorityWasCleaned(t *testing.T) {
	runtime, buildRunner, failed := failedRuntimeBuildAttemptFixture(t, runtimeBuildStagingOwned)
	settled := failed
	settled.AttemptSettlement = runtimeBuildAttemptSettled
	if err := runtime.writeRuntimeBuildJournal(failed, settled); err != nil {
		t.Fatal(err)
	}
	observation := runtimeImageObservation{ID: settled.ImageDigest, Size: 4096, RepoTags: []string{settled.StagingImage}, Owner: ownerValue, Component: managedRuntimeComponentLabel, RuntimeID: settled.RuntimeID, Revision: settled.Revision}
	lifecycleRunner := &lifecycleObservationRunner{images: map[string]lifecycleImageFixture{settled.StagingImage: {observation: observation}, settled.ImageDigest: {observation: observation}}, containers: map[string]runtimeContainerObservation{}, containerLists: map[string]string{}}
	runtime.runner = &failedRuntimePruneRunner{build: buildRunner, lifecycle: lifecycleRunner}
	snapshot, observedAt, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil || len(plan.Candidates) != 1 || plan.Candidates[0].Kind != tobari.RuntimePruneCandidateFailedBuild {
		t.Fatalf("failed-build prune plan = %+v/%v", plan, err)
	}

	interrupted := true
	runtime.runtimePruneAfterBuildCleanup = func(tobari.RuntimePruneCandidate) error {
		if interrupted {
			interrupted = false
			return errors.New("synthetic process interruption after build authority cleanup")
		}
		return nil
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("post-cleanup interruption succeeded")
	}
	prune, err := runtime.readRuntimePruneJournalObserved()
	if err != nil || prune == nil || prune.Items[0].State != runtimePruneTerminal {
		t.Fatalf("post-cleanup prune journal = %+v/%v", prune, err)
	}
	if build, err := runtime.readRuntimeBuildJournalObserved(); err != nil || build != nil {
		t.Fatalf("post-cleanup build authority = %+v/%v", build, err)
	}
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneApplied || result.Items[0].Kind != tobari.RuntimePruneCandidateFailedBuild {
		t.Fatalf("post-cleanup same-plan retry = %+v/%v", result, err)
	}
	if _, _, err := runtime.ReadRuntimeLifecycleSnapshot(context.Background()); err != nil {
		t.Fatalf("lifecycle remained blocked after same-plan retry: %v", err)
	}
}

func TestApplyRuntimePruneIgnoresPresentationNameWhenSelectingEffect(t *testing.T) {
	runtime, runner, plan, manifest := runtimePruneFixture(t, false)
	plan.Candidates[0].Name = "replacement"
	if err := plan.Validate(); err != nil {
		t.Fatalf("presentation-only plan change: %v", err)
	}
	result, err := runtime.applyRuntimePruneReviewedPlan(context.Background(), plan)
	if err != nil || result.Items[0].Name != manifest.Name || len(runner.removals) != 1 || !strings.Contains(runner.removals[0], manifest.ID) {
		t.Fatalf("presentation drift selected effect = %+v/%v removals=%v", result, err, runner.removals)
	}
}

func TestApplyRuntimePruneBoundsObservationAndReleasesLocks(t *testing.T) {
	runtime, _, plan, _ := runtimePruneFixture(t, false)
	runtime.runner = blockingLifecycleRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := runtime.ApplyRuntimePrune(ctx, plan.PlanRef); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unbounded Runtime prune observation: %v", err)
	}
	if err := runtime.WithLifecycleLock(context.Background(), func(lockContext context.Context) error {
		return runtime.withRuntimeStoreLock(lockContext, func() error { return nil })
	}); err != nil {
		t.Fatalf("Runtime prune cancellation retained locks: %v", err)
	}
}

func TestApplyRuntimePruneRejectsStaleOrNewlyUsedPlanBeforeImageEffect(t *testing.T) {
	runtime, runner, plan, manifest := runtimePruneFixture(t, false)
	if _, err := runtime.ApplyRuntimePrune(context.Background(), "not-a-plan"); err == nil {
		t.Fatal("invalid Runtime prune reference succeeded")
	}
	stale := plan
	stale.PlanRef = "sha256:" + strings.Repeat("0", 64)
	if _, err := runtime.ApplyRuntimePrune(context.Background(), stale.PlanRef); !errors.Is(err, tobari.ErrRuntimePrunePlanStale) {
		t.Fatalf("stale Runtime prune plan fault = %v", err)
	}
	if len(runner.removals) != 0 {
		t.Fatalf("stale plan removed images: %v", runner.removals)
	}

	containerID := strings.Repeat("c", 64)
	digest := manifest.Revisions[0].ImageDigest
	runner.containerLists[digest] = containerID + "\n"
	runner.containers[containerID] = runtimeContainerObservation{ID: containerID, Image: digest, Owner: "foreign"}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("newly used Runtime prune plan succeeded")
	}
	if len(runner.removals) != 0 {
		t.Fatalf("newly used plan removed images: %v", runner.removals)
	}
}

func TestApplyRuntimePruneRejectsDifferentPlanWhileJournalIsActive(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, false)
	journal := runtimePruneJournal{SchemaVersion: runtimePruneJournalSchema, Plan: plan, Items: []runtimePruneJournalItem{{Candidate: plan.Candidates[0], State: runtimePrunePending}}}
	if err := runtime.writeRuntimePruneJournal(nil, journal); err != nil {
		t.Fatal(err)
	}
	other := "sha256:" + strings.Repeat("0", 64)
	if _, err := runtime.ApplyRuntimePrune(context.Background(), other); !errors.Is(err, tobari.ErrRuntimePruneInterrupted) {
		t.Fatalf("different active Runtime prune plan fault = %v", err)
	}
	if len(runner.removals) != 0 {
		t.Fatalf("different active plan removed images: %v", runner.removals)
	}
}

func TestApplyRuntimePruneClassifiesActiveJournalObservationFailureAsInterrupted(t *testing.T) {
	runtime, _, plan, _ := runtimePruneFixture(t, false)
	journal := runtimePruneJournal{SchemaVersion: runtimePruneJournalSchema, Plan: plan, Items: []runtimePruneJournalItem{{Candidate: plan.Candidates[0], State: runtimePrunePending}}}
	if err := runtime.writeRuntimePruneJournal(nil, journal); err != nil {
		t.Fatal(err)
	}
	runtime.runner = blockingLifecycleRunner{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := runtime.ApplyRuntimePrune(ctx, plan.PlanRef); !errors.Is(err, tobari.ErrRuntimePruneInterrupted) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active Runtime prune observation fault = %v", err)
	}
}

func TestApplyRuntimePruneReceiptInterruptionResumesWithoutDockerReplay(t *testing.T) {
	runtime, runner, plan, _ := runtimePruneFixture(t, false)
	first := true
	runtime.runtimePruneJournalRemove = func(path string) error {
		if first {
			first = false
			return errors.New("synthetic crash before journal unlink")
		}
		return os.Remove(path)
	}
	if _, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef); err == nil {
		t.Fatal("journal-unlink interruption succeeded")
	}
	if len(runner.removals) != 1 {
		t.Fatalf("first apply removals = %v", runner.removals)
	}
	result, err := runtime.ApplyRuntimePrune(context.Background(), plan.PlanRef)
	if err != nil || result.State != tobari.RuntimePruneAlreadyApplied || len(runner.removals) != 1 {
		t.Fatalf("receipt resume = %+v/%v removals=%v", result, err, runner.removals)
	}
	if _, err := os.Lstat(runtime.runtimePruneJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("prune journal remains after resume: %v", err)
	}
}

func TestRuntimePruneJournalTransitionRejectsAuthorityOrPhaseDrift(t *testing.T) {
	_, _, plan, _ := runtimePruneFixture(t, false)
	base := runtimePruneJournal{SchemaVersion: runtimePruneJournalSchema, Plan: plan, Items: []runtimePruneJournalItem{{Candidate: plan.Candidates[0], State: runtimePrunePending}}}
	valid := cloneRuntimePruneJournal(base)
	valid.Items[0].State = runtimePruneRemoving
	if err := validateRuntimePruneJournalTransition(base, valid); err != nil {
		t.Fatalf("valid prune transition: %v", err)
	}
	tests := map[string]func(*runtimePruneJournal){
		"plan drift":      func(j *runtimePruneJournal) { j.Plan.ObservedAt = j.Plan.ObservedAt.Add(time.Second) },
		"candidate drift": func(j *runtimePruneJournal) { j.Items[0].Candidate.Name = "replacement" },
		"terminal without evidence": func(j *runtimePruneJournal) {
			j.Items[0].State = runtimePruneTerminal
			j.Items[0].Disposition = ""
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			next := cloneRuntimePruneJournal(valid)
			mutate(&next)
			if err := validateRuntimePruneJournalTransition(base, next); err == nil {
				t.Fatalf("invalid transition validated: %+v", next)
			}
		})
	}
}

func TestRuntimePruneDurableStateIsBoundedBeforeWrite(t *testing.T) {
	if err := validateRuntimePruneStateSize(map[string]string{"value": "small"}); err != nil {
		t.Fatalf("small durable state: %v", err)
	}
	if err := validateRuntimePruneStateSize(map[string]string{"value": strings.Repeat("x", maxProjectStateBytes)}); err == nil {
		t.Fatal("oversized durable state crossed the write boundary")
	}
}
