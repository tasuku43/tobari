package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// RestoreManagedRuntimeByRevisionReference rebuilds only the availability
// facet of one retained immutable revision. It shares the build journal so an
// interrupted effect has the same bounded, review-confirmed recovery path,
// but restore publication never appends or rewrites Runtime history.
func (r *Runtime) RestoreManagedRuntimeByRevisionReference(ctx context.Context, reference string, diagnostics io.Writer) (tobari.RuntimeRestoreResult, error) {
	if _, _, err := tobari.ParseRuntimeRevisionRef(reference); err != nil {
		return tobari.RuntimeRestoreResult{}, err
	}
	observed, _, err := r.ReadRuntimeLifecycleSnapshot(ctx)
	if err != nil {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("%w: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	observedTarget, err := tobari.RuntimeRestoreTargetFrom(observed, reference)
	if err != nil {
		return tobari.RuntimeRestoreResult{}, err
	}
	if observedTarget.Availability == tobari.RuntimeAvailabilityAvailable {
		result, err := r.observeAlreadyAvailableRuntimeRestore(ctx, reference, observedTarget)
		if err != nil {
			return tobari.RuntimeRestoreResult{}, err
		}
		return result, nil
	}
	var result tobari.RuntimeRestoreResult
	err = r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		snapshot, _, err := r.readRuntimeLifecycleSnapshotLocked(lockContext)
		if err != nil {
			return fmt.Errorf("%w: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
		}
		target, err := tobari.RuntimeRestoreTargetFrom(snapshot, reference)
		if err != nil {
			return err
		}
		if target.Availability == tobari.RuntimeAvailabilityAvailable {
			selector := managedLibraryRuntimeImage(target.Name, target.RuntimeID, target.Revision)
			if err := r.validateManagedRuntimeBuildCompatibility(lockContext, selector); err != nil {
				return fmt.Errorf("%w: current image compatibility changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
			}
			digest, err := r.inspectManagedRuntimeBuildEvidence(lockContext, selector, target.RuntimeID, target.Revision)
			if err != nil || digest != target.RecordedImageDigest {
				return fmt.Errorf("%w: current image authority changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
			}
			result = runtimeRestoreResult(target, tobari.RuntimeAlreadyAvailable, tobari.RuntimeRestoreArtifactNotCreated)
			return result.Validate()
		}
		mutationErr := r.withRuntimeStoreLock(lockContext, func() error {
			manifest, revision, err := r.resolveRuntimeRevisionForRestoreUnlocked(target)
			if err != nil {
				return err
			}
			journal, err := r.beginRuntimeRestoreJournal(lockContext, manifest, revision)
			if err != nil {
				return err
			}
			if err := os.Mkdir(filepath.Dir(journal.SnapshotPath), 0o700); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			observedRevision, err := copyRuntimeSource(lockContext, revision.SnapshotPath, journal.SnapshotPath)
			if err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			if observedRevision != target.Revision {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, fmt.Errorf("%w: retained snapshot digest changed", tobari.ErrRuntimeRevisionUnrestorable), journal)
			}
			if err := r.syncRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			if err := r.syncRuntimeBuildDirectory(filepath.Dir(journal.SnapshotPath)); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			if err := r.syncRuntimeBuildDirectory(r.runtimeLifecycleDirectory()); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			if err := r.requireRuntimeBuildSnapshotRevision(lockContext, journal.SnapshotPath, target.Revision); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			prepared := journal
			prepared.Phase = runtimeBuildPhasePrepared
			if err := r.writeRuntimeBuildJournal(journal, prepared); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal, prepared)
			}
			journal = prepared
			orphanDisposition, orphanDigest, err := r.observeUnusedRuntimeStagingTag(lockContext, journal)
			if err != nil {
				orphan := journal
				orphan.Phase = runtimeBuildPhaseOrphanStaging
				orphan.OrphanStaging = orphanDisposition
				orphan.ImageDigest = orphanDigest
				if journalErr := r.writeRuntimeBuildJournal(journal, orphan); journalErr != nil {
					return fmt.Errorf("Runtime restore staging conflict requires reconciliation: %w", errors.Join(err, journalErr))
				}
				return err
			}
			dockerfile := filepath.Join(journal.SnapshotPath, "Dockerfile")
			if info, err := os.Lstat(dockerfile); err != nil || !info.Mode().IsRegular() {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, fmt.Errorf("%w: retained snapshot lacks a regular Dockerfile", tobari.ErrRuntimeRevisionUnrestorable), journal)
			}
			args := []string{"buildx", "build", "--progress=plain", "--load",
				"--label", ownerLabel + "=" + ownerValue,
				"--label", componentLabel + "=" + managedRuntimeComponentLabel,
				"--label", managedRuntimeIDLabel + "=" + target.RuntimeID,
				"--label", managedRuntimeRevisionLabel + "=" + target.Revision,
				"--label", managedRuntimeBuildAttemptLabel + "=" + journal.AttemptID,
				"--tag", journal.StagingImage, "--file", dockerfile, journal.SnapshotPath,
			}
			building := journal
			building.Phase = runtimeBuildPhaseBuilding
			building.StagingArtifact = runtimeBuildStagingUnknown
			building.AttemptSettlement = runtimeBuildAttemptUnsettled
			if err := r.writeRuntimeBuildJournal(journal, building); err != nil {
				return r.rollbackRuntimeBuildBeforeDocker(lockContext, err, journal)
			}
			journal = building
			var tail runtimeBuildDiagnosticTail
			stream := io.MultiWriter(&bestEffortDiagnosticWriter{writer: diagnostics}, &tail)
			if err := r.runner.Run(lockContext, args, os.Environ(), nil, stream, stream); err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("%w: rebuild Runtime revision: %v: %s", tobari.ErrRuntimeRevisionUnrestorable, err, boundedDiagnostic(tail.Bytes())))
			}
			if err := r.validateManagedRuntimeBuildCompatibility(lockContext, journal.StagingImage); err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("%w: %v", tobari.ErrRuntimeRevisionUnrestorable, err))
			}
			imageDigest, err := r.inspectManagedRuntimeBuildEvidence(lockContext, journal.StagingImage, target.RuntimeID, target.Revision, journal.AttemptID)
			if err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("%w: %v", tobari.ErrRuntimeRevisionUnrestorable, err))
			}
			if err := r.freezeRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("freeze Runtime restore snapshot: %w", err))
			}
			if err := r.syncRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("sync Runtime restore snapshot: %w", err))
			}
			if err := r.requireRuntimeBuildSnapshotRevision(lockContext, journal.SnapshotPath, target.Revision); err != nil {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("%w: retained restore snapshot drifted: %v", tobari.ErrRuntimeRevisionUnrestorable, err))
			}
			if imageDigest != target.RecordedImageDigest {
				return r.retainRuntimeBuildFailure(lockContext, journal, fmt.Errorf("%w: rebuilt content digest differs from the immutable revision", tobari.ErrRuntimeRevisionUnrestorable))
			}
			built := journal
			built.ImageDigest = imageDigest
			built.Phase = runtimeBuildPhaseBuilt
			built.StagingArtifact = runtimeBuildStagingOwned
			built.AttemptSettlement = runtimeBuildAttemptSettled
			if err := r.writeRuntimeBuildJournal(journal, built); err != nil {
				return err
			}
			published, err := r.resumeManagedRuntimePublicationLocked(lockContext, built)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(published, manifest) {
				return fmt.Errorf("Runtime restore changed immutable manifest authority")
			}
			result = runtimeRestoreResult(target, tobari.RuntimeRestored, tobari.RuntimeRestoreArtifactRemoved)
			return result.Validate()
		})
		if mutationErr != nil {
			journal, observationErr := r.readRuntimeBuildJournalObserved()
			if observationErr != nil || journal != nil {
				return fmt.Errorf("%w: %w", tobari.ErrRuntimeRestoreInterrupted, errors.Join(mutationErr, observationErr))
			}
		}
		return mutationErr
	})
	if err != nil {
		return tobari.RuntimeRestoreResult{}, err
	}
	return result, nil
}

// RecoverRuntimeRestoreByRevisionReference resumes one exact interrupted
// restore through the existing review-confirmed journal workflow. Internal
// phase transitions may require several durable steps, but one invocation
// carries the same opaque revision authority throughout and either reaches a
// validated restore result or leaves the journal for the same review path.
func (r *Runtime) RecoverRuntimeRestoreByRevisionReference(ctx context.Context, reference string, kind tobari.RuntimeBuildRecoveryKind, diagnostics io.Writer) (tobari.RuntimeRestoreResult, error) {
	runtimeID, _, err := tobari.ParseRuntimeRevisionRef(reference)
	if err != nil {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("Runtime restore recovery reference is invalid: %w", err)
	}
	guardContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	guardErr := r.WithLifecycleLock(guardContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, r.requireNoRuntimeDeleteRecoveryConflict)
	})
	cancel()
	if guardErr != nil {
		return tobari.RuntimeRestoreResult{}, guardErr
	}
	recoveryContext := context.WithValue(ctx, runtimeBuildRecoveryReferenceContextKey{}, runtimeBuildRecoveryTarget{
		RuntimeRef: tobari.RuntimeRef(runtimeID), RevisionRef: reference,
	})
	restoreFailed := false
	publicationRecovered := false
	for step := 0; step < 8; step++ {
		recovery, found, err := r.ReadRuntimeBuildRecovery(recoveryContext)
		if err != nil {
			return tobari.RuntimeRestoreResult{}, err
		}
		if !found {
			if restoreFailed {
				return tobari.RuntimeRestoreResult{}, tobari.ErrRuntimeRevisionUnrestorable
			}
			result, err := r.RestoreManagedRuntimeByRevisionReference(recoveryContext, reference, diagnostics)
			if err == nil && publicationRecovered && result.State == tobari.RuntimeAlreadyAvailable {
				result.State = tobari.RuntimeRestored
				result.ArtifactDisposition = tobari.RuntimeRestoreArtifactRemoved
				err = result.Validate()
			}
			return result, err
		}
		if recovery.RevisionRef != reference || (step == 0 && recovery.Kind != kind) {
			return tobari.RuntimeRestoreResult{}, fmt.Errorf("Runtime restore recovery target authority changed")
		}
		restoreFailed = restoreFailed || recovery.RestoreFailed
		if err := r.recoverRuntimeBuildByKind(recoveryContext, recovery.Kind); err != nil {
			return tobari.RuntimeRestoreResult{}, err
		}
		publicationRecovered = publicationRecovered || recovery.Kind == tobari.RuntimeBuildRecoveryBuilding || recovery.Kind == tobari.RuntimeBuildRecoveryPublication
	}
	return tobari.RuntimeRestoreResult{}, fmt.Errorf("Runtime restore recovery exceeded the phase bound")
}

// observeAlreadyAvailableRuntimeRestore proves the no-op result entirely
// through the non-creating read boundary. The second coherent snapshot rejects
// local or Docker authority drift around the compatibility observation.
func (r *Runtime) observeAlreadyAvailableRuntimeRestore(ctx context.Context, reference string, before tobari.RuntimeRestoreTarget) (tobari.RuntimeRestoreResult, error) {
	observationContext, cancel := context.WithTimeout(ctx, runtimeLifecycleWallBudget)
	defer cancel()
	selector := managedLibraryRuntimeImage(before.Name, before.RuntimeID, before.Revision)
	if err := r.validateManagedRuntimeBuildCompatibility(observationContext, selector); err != nil {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("%w: current image compatibility changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	digest, err := r.inspectManagedRuntimeBuildEvidence(observationContext, selector, before.RuntimeID, before.Revision)
	if err != nil || digest != before.RecordedImageDigest {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("%w: current image authority changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	afterSnapshot, _, err := r.ReadRuntimeLifecycleSnapshot(observationContext)
	if err != nil {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("%w: current lifecycle authority changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	after, err := tobari.RuntimeRestoreTargetFrom(afterSnapshot, reference)
	if err != nil || !reflect.DeepEqual(after, before) || after.Availability != tobari.RuntimeAvailabilityAvailable {
		return tobari.RuntimeRestoreResult{}, fmt.Errorf("%w: current restore target changed: %v", tobari.ErrRuntimeRetirementObservationUnknown, err)
	}
	result := runtimeRestoreResult(after, tobari.RuntimeAlreadyAvailable, tobari.RuntimeRestoreArtifactNotCreated)
	return result, result.Validate()
}

func runtimeRestoreResult(target tobari.RuntimeRestoreTarget, state tobari.RuntimeRestoreState, artifact tobari.RuntimeRestoreArtifactDisposition) tobari.RuntimeRestoreResult {
	return tobari.RuntimeRestoreResult{
		Task: tobari.TaskRuntimeRestore, RuntimeID: target.RuntimeID, RuntimeRef: target.RuntimeRef,
		Revision: target.Revision, RevisionRef: target.RevisionRef, Name: target.Name, Ordinal: target.Ordinal,
		State: state, DigestMatch: true, ArtifactDisposition: artifact,
	}
}

func (r *Runtime) resolveRuntimeRevisionForRestoreUnlocked(target tobari.RuntimeRestoreTarget) (tobari.RuntimeManifest, tobari.RuntimeRevision, error) {
	manifest, err := r.resolveManagedRuntimeReferenceUnlocked(target.RuntimeRef)
	if err != nil {
		return tobari.RuntimeManifest{}, tobari.RuntimeRevision{}, err
	}
	if manifest.ID != target.RuntimeID || manifest.Name != target.Name || manifest.Kind != tobari.RuntimeKindManaged {
		return tobari.RuntimeManifest{}, tobari.RuntimeRevision{}, tobari.ErrRuntimeRevisionUnrestorable
	}
	for _, revision := range manifest.Revisions {
		if revision.Revision != target.Revision {
			continue
		}
		if revision.Ordinal != target.Ordinal || revision.ImageDigest != target.RecordedImageDigest ||
			revision.Image != managedLibraryRuntimeImage(manifest.Name, manifest.ID, revision.Revision) {
			return tobari.RuntimeManifest{}, tobari.RuntimeRevision{}, tobari.ErrRuntimeRevisionUnrestorable
		}
		return manifest, revision, nil
	}
	return tobari.RuntimeManifest{}, tobari.RuntimeRevision{}, tobari.ErrRuntimeRevisionNotFound
}

func (r *Runtime) beginRuntimeRestoreJournal(ctx context.Context, manifest tobari.RuntimeManifest, revision tobari.RuntimeRevision) (runtimeBuildJournal, error) {
	if err := r.ensurePrivateDirectory(r.runtimeLifecycleDirectory()); err != nil {
		return runtimeBuildJournal{}, err
	}
	if prune, err := r.readRuntimePruneJournalObserved(); err != nil {
		return runtimeBuildJournal{}, err
	} else if prune != nil {
		return runtimeBuildJournal{}, fmt.Errorf("a Runtime prune journal requires recovery before restore")
	}
	if deletion, err := r.readRuntimeDeleteJournalObserved(); err != nil {
		return runtimeBuildJournal{}, err
	} else if deletion != nil {
		return runtimeBuildJournal{}, fmt.Errorf("a Runtime delete journal requires recovery before restore")
	}
	path := r.runtimeBuildJournalPath()
	if _, err := os.Lstat(path); err == nil {
		return runtimeBuildJournal{}, fmt.Errorf("a Runtime lifecycle journal requires recovery before restore")
	} else if !errors.Is(err, os.ErrNotExist) {
		return runtimeBuildJournal{}, err
	}
	snapshot := r.runtimeBuildSnapshotPath()
	if _, err := os.Lstat(filepath.Dir(snapshot)); err == nil {
		return runtimeBuildJournal{}, fmt.Errorf("Runtime restore staging snapshot lacks journal authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return runtimeBuildJournal{}, err
	}
	attemptID, err := r.identities.newRuntimeBuildAttemptID()
	if err != nil {
		return runtimeBuildJournal{}, err
	}
	journal := runtimeBuildJournal{
		SchemaVersion: runtimeBuildJournalSchema, Phase: runtimeBuildPhaseSnapshotting, Restore: true,
		RuntimeID: manifest.ID, RuntimeName: manifest.Name, AttemptID: attemptID,
		Revision: revision.Revision, StagingImage: managedRuntimeStagingImage(manifest.ID, revision.Revision),
		FinalImage:          managedLibraryRuntimeImage(manifest.Name, manifest.ID, revision.Revision),
		ExpectedImageDigest: revision.ImageDigest, SnapshotPath: snapshot, CreatedAt: revision.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if err := journal.Validate(r); err != nil {
		return runtimeBuildJournal{}, err
	}
	if err := writeAtomicJSON(path, journal); err != nil {
		if observed, observeErr := r.readRuntimeBuildJournalObserved(); observeErr == nil && observed != nil && *observed == journal {
			if cleanupErr := r.completeRuntimeBuildJournal(ctx, journal); cleanupErr != nil {
				return runtimeBuildJournal{}, fmt.Errorf("initialize Runtime restore journal and rollback uncertain publication: %w", errors.Join(err, cleanupErr))
			}
		}
		return runtimeBuildJournal{}, err
	}
	return journal, nil
}
