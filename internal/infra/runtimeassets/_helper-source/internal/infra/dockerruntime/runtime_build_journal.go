package dockerruntime

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

var errManagedRuntimeImageMissing = errors.New("managed Runtime image is missing")

const (
	runtimeBuildJournalSchema = 1
	runtimeBuildJournalFile   = "build.json"
	runtimeBuildSnapshotDir   = "build-source"

	runtimeBuildPhaseSnapshotting      = "snapshotting"
	runtimeBuildPhasePrepared          = "prepared"
	runtimeBuildPhaseBuilding          = "building"
	runtimeBuildPhaseBuilt             = "built"
	runtimeBuildPhaseFailed            = "failed"
	runtimeBuildPhaseCompleting        = "completing"
	runtimeBuildPhaseOrphanStaging     = "orphan_staging"
	runtimeBuildPhaseFinalTagged       = "final_tagged"
	runtimeBuildPhaseStagingReleased   = "staging_released"
	runtimeBuildPhaseSnapshotPublished = "snapshot_published"
	runtimeBuildPhaseManifestCommitted = "manifest_committed"

	runtimeBuildOrphanExactManaged = "exact_managed"
	runtimeBuildOrphanUnknown      = "ownership_unknown"
	runtimeBuildOrphanAbsent       = "observed_absent"
	runtimeBuildStagingOwned       = "transaction_owned"
	runtimeBuildStagingAbsent      = "observed_absent"
	runtimeBuildStagingUnknown     = "outcome_unknown"
	runtimeBuildAttemptUnsettled   = "unsettled"
	runtimeBuildAttemptSettled     = "settled"

	managedRuntimeComponentLabel           = "runtime-revision"
	managedRuntimeIDLabel                  = "io.tobari.runtime-id"
	managedRuntimeRevisionLabel            = "io.tobari.runtime-revision"
	managedRuntimeBuildAttemptLabel        = "io.tobari.runtime-build-attempt"
	runtimeBuildRecoveryObservationTimeout = 30 * time.Second
)

func (r *Runtime) runtimeBuildRecoveryContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := runtimeBuildRecoveryObservationTimeout
	if r.runtimeBuildRecoveryTimeout > 0 {
		timeout = r.runtimeBuildRecoveryTimeout
	}
	return context.WithTimeout(parent, timeout)
}

type runtimeBuildJournal struct {
	SchemaVersion       int    `json:"schema_version"`
	Phase               string `json:"phase"`
	Restore             bool   `json:"restore,omitempty"`
	RuntimeID           string `json:"runtime_id"`
	RuntimeName         string `json:"runtime_name"`
	AttemptID           string `json:"attempt_id"`
	Revision            string `json:"revision,omitempty"`
	StagingImage        string `json:"staging_image,omitempty"`
	FinalImage          string `json:"final_image,omitempty"`
	ImageDigest         string `json:"image_digest,omitempty"`
	ExpectedImageDigest string `json:"expected_image_digest,omitempty"`
	SnapshotPath        string `json:"snapshot_path"`
	CleanupFrom         string `json:"cleanup_from,omitempty"`
	OrphanStaging       string `json:"orphan_staging,omitempty"`
	RemoveStaging       bool   `json:"remove_staging,omitempty"`
	StagingArtifact     string `json:"staging_artifact,omitempty"`
	AttemptSettlement   string `json:"attempt_settlement,omitempty"`
	CreatedAt           string `json:"created_at,omitempty"`
}

type runtimeBuildRecoveryReferenceContextKey struct{}

type runtimeBuildRecoveryTarget struct {
	RuntimeRef  string
	RevisionRef string
}

func requireRuntimeBuildRecoveryReference(ctx context.Context, journal *runtimeBuildJournal) error {
	expected, exact := ctx.Value(runtimeBuildRecoveryReferenceContextKey{}).(runtimeBuildRecoveryTarget)
	if !exact {
		return nil
	}
	if journal == nil || tobari.RuntimeRef(journal.RuntimeID) != expected.RuntimeRef {
		return fmt.Errorf("Runtime build recovery target authority changed")
	}
	if expected.RevisionRef == "" {
		if journal.Restore {
			return fmt.Errorf("Runtime restore recovery requires exact revision authority")
		}
		return nil
	}
	if !journal.Restore || tobari.RuntimeRevisionRef(journal.RuntimeID, journal.Revision) != expected.RevisionRef {
		return fmt.Errorf("Runtime restore recovery target authority changed")
	}
	return nil
}

// RecoverRuntimeBuildByReference dispatches one review-confirmed recovery
// without decoding or rediscovering its exact Runtime target. The reference is
// compared to the current journal under the same lifecycle and Runtime locks.
func (r *Runtime) RecoverRuntimeBuildByReference(ctx context.Context, runtimeRef string, kind tobari.RuntimeBuildRecoveryKind) error {
	if err := tobari.ValidateRuntimeRef(runtimeRef); err != nil {
		return fmt.Errorf("Runtime build recovery reference is invalid: %w", err)
	}
	if runtimeRef == tobari.StandardRuntimeID {
		return fmt.Errorf("built-in Runtime has no managed build recovery")
	}
	ctx = context.WithValue(ctx, runtimeBuildRecoveryReferenceContextKey{}, runtimeBuildRecoveryTarget{RuntimeRef: runtimeRef})
	return r.recoverRuntimeBuildByKind(ctx, kind)
}

func (r *Runtime) recoverRuntimeBuildByKind(ctx context.Context, kind tobari.RuntimeBuildRecoveryKind) error {
	switch kind {
	case tobari.RuntimeBuildRecoveryPreDocker:
		return r.RecoverRuntimeBuildPreDocker(ctx)
	case tobari.RuntimeBuildRecoveryBuilding:
		return r.RecoverRuntimeBuildBuilding(ctx)
	case tobari.RuntimeBuildRecoveryPublication:
		return r.RecoverRuntimeBuildPublication(ctx)
	case tobari.RuntimeBuildRecoveryCleanup:
		return r.RecoverRuntimeBuildCleanup(ctx)
	case tobari.RuntimeBuildRecoveryOrphan:
		return r.RecoverRuntimeBuildOrphanStaging(ctx)
	case tobari.RuntimeBuildRecoveryFailed:
		return r.RecoverRuntimeBuildFailed(ctx)
	default:
		return fmt.Errorf("Runtime build recovery kind is invalid")
	}
}

func (r *Runtime) runtimeLifecycleDirectory() string {
	return filepath.Join(r.stateDirectory, "runtime-lifecycle")
}

func (r *Runtime) runtimeBuildJournalPath() string {
	return filepath.Join(r.runtimeLifecycleDirectory(), runtimeBuildJournalFile)
}

func (r *Runtime) runtimeBuildSnapshotPath() string {
	return filepath.Join(r.runtimeLifecycleDirectory(), runtimeBuildSnapshotDir, "source")
}

func (j runtimeBuildJournal) Validate(r *Runtime) error {
	if j.SchemaVersion != runtimeBuildJournalSchema || tobari.ValidateRuntimeID(j.RuntimeID) != nil || tobari.ValidateName(j.RuntimeName) != nil || !validRuntimeBuildAttemptID(j.AttemptID) || j.SnapshotPath != r.runtimeBuildSnapshotPath() {
		return fmt.Errorf("Runtime build journal authority is invalid")
	}
	if j.Phase == runtimeBuildPhaseCompleting {
		allowedOrigin := j.CleanupFrom == runtimeBuildPhaseSnapshotting ||
			j.CleanupFrom == runtimeBuildPhasePrepared ||
			j.CleanupFrom == runtimeBuildPhaseFailed ||
			j.CleanupFrom == runtimeBuildPhaseOrphanStaging ||
			j.CleanupFrom == runtimeBuildPhaseManifestCommitted
		if !allowedOrigin {
			return fmt.Errorf("completing Runtime build journal origin is invalid")
		}
		origin := j
		origin.Phase = j.CleanupFrom
		origin.CleanupFrom = ""
		origin.RemoveStaging = false
		if err := origin.Validate(r); err != nil {
			return err
		}
		if origin.Phase == runtimeBuildPhaseOrphanStaging && origin.OrphanStaging != runtimeBuildOrphanExactManaged && origin.OrphanStaging != runtimeBuildOrphanAbsent {
			return fmt.Errorf("completing Runtime orphan staging authority is invalid")
		}
		if origin.Phase == runtimeBuildPhaseFailed && origin.AttemptSettlement != runtimeBuildAttemptSettled {
			return fmt.Errorf("unsettled Runtime build attempt cannot enter cleanup")
		}
		if j.RemoveStaging && !(origin.Phase == runtimeBuildPhaseFailed && origin.StagingArtifact == runtimeBuildStagingOwned) && !(origin.Phase == runtimeBuildPhaseOrphanStaging && origin.OrphanStaging == runtimeBuildOrphanExactManaged) {
			return fmt.Errorf("Runtime build cleanup lacks staging removal authority")
		}
		return nil
	}
	if j.CleanupFrom != "" || j.RemoveStaging {
		return fmt.Errorf("active Runtime build journal has premature cleanup evidence")
	}
	if j.Restore {
		if tobari.ValidateDigest(j.Revision) != nil || tobari.ValidateDigest(j.ExpectedImageDigest) != nil ||
			j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) ||
			j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.RuntimeID, j.Revision) {
			return fmt.Errorf("Runtime restore journal target is invalid")
		}
		createdAt, err := time.Parse(time.RFC3339Nano, j.CreatedAt)
		if err != nil || createdAt.Location() != time.UTC {
			return fmt.Errorf("Runtime restore journal immutable creation time is invalid")
		}
	} else if j.ExpectedImageDigest != "" {
		return fmt.Errorf("Runtime build journal has restore-only digest authority")
	}
	switch j.Phase {
	case runtimeBuildPhaseSnapshotting:
		if (!j.Restore && (j.Revision != "" || j.StagingImage != "" || j.FinalImage != "" || j.CreatedAt != "")) || j.ImageDigest != "" || j.OrphanStaging != "" || j.StagingArtifact != "" || j.AttemptSettlement != "" {
			return fmt.Errorf("snapshotting Runtime build journal has premature evidence")
		}
	case runtimeBuildPhasePrepared, runtimeBuildPhaseBuilding, runtimeBuildPhaseFailed, runtimeBuildPhaseOrphanStaging:
		if tobari.ValidateDigest(j.Revision) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.RuntimeID, j.Revision) {
			return fmt.Errorf("Runtime build journal target is invalid")
		}
		if j.Phase == runtimeBuildPhasePrepared && (j.ImageDigest != "" || j.StagingArtifact != "" || j.AttemptSettlement != "" || (!j.Restore && j.CreatedAt != "")) {
			return fmt.Errorf("pre-build-completion Runtime journal has premature image evidence")
		}
		if j.Phase == runtimeBuildPhaseBuilding && (j.ImageDigest != "" || j.StagingArtifact != runtimeBuildStagingUnknown || j.AttemptSettlement != runtimeBuildAttemptUnsettled || (!j.Restore && j.CreatedAt != "")) {
			return fmt.Errorf("building Runtime journal lacks exact outcome-unknown evidence")
		}
		if j.ImageDigest != "" && tobari.ValidateDigest(j.ImageDigest) != nil {
			return fmt.Errorf("Runtime build journal image evidence is invalid")
		}
		if j.Phase == runtimeBuildPhaseOrphanStaging {
			if j.OrphanStaging != runtimeBuildOrphanExactManaged && j.OrphanStaging != runtimeBuildOrphanUnknown && j.OrphanStaging != runtimeBuildOrphanAbsent {
				return fmt.Errorf("Runtime orphan staging disposition is invalid")
			}
			if (j.OrphanStaging == runtimeBuildOrphanExactManaged) != (j.ImageDigest != "") {
				return fmt.Errorf("Runtime orphan staging evidence is invalid")
			}
			if j.StagingArtifact != "" || j.AttemptSettlement != "" {
				return fmt.Errorf("Runtime orphan staging has transaction-owned evidence")
			}
		} else if j.OrphanStaging != "" {
			return fmt.Errorf("Runtime build journal has premature orphan evidence")
		}
		if j.Phase == runtimeBuildPhaseFailed {
			if j.StagingArtifact != runtimeBuildStagingOwned && j.StagingArtifact != runtimeBuildStagingAbsent && j.StagingArtifact != runtimeBuildStagingUnknown {
				return fmt.Errorf("failed Runtime staging disposition is invalid")
			}
			if (j.StagingArtifact == runtimeBuildStagingOwned) != (j.ImageDigest != "") {
				return fmt.Errorf("failed Runtime staging ownership evidence is invalid")
			}
			if !j.Restore && j.CreatedAt != "" {
				return fmt.Errorf("failed Runtime journal has publication time evidence")
			}
			if j.AttemptSettlement != runtimeBuildAttemptUnsettled && j.AttemptSettlement != runtimeBuildAttemptSettled {
				return fmt.Errorf("failed Runtime journal lacks explicit attempt settlement")
			}
		}
	case runtimeBuildPhaseBuilt, runtimeBuildPhaseFinalTagged, runtimeBuildPhaseStagingReleased, runtimeBuildPhaseSnapshotPublished, runtimeBuildPhaseManifestCommitted:
		if tobari.ValidateDigest(j.Revision) != nil || tobari.ValidateDigest(j.ImageDigest) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.RuntimeID, j.Revision) {
			return fmt.Errorf("built Runtime journal evidence is invalid")
		}
		if j.OrphanStaging != "" || j.StagingArtifact != runtimeBuildStagingOwned || j.AttemptSettlement != runtimeBuildAttemptSettled {
			return fmt.Errorf("built Runtime journal has orphan staging evidence")
		}
		createdAt, err := time.Parse(time.RFC3339Nano, j.CreatedAt)
		if err != nil || createdAt.Location() != time.UTC {
			return fmt.Errorf("built Runtime journal publication time is invalid")
		}
	default:
		return fmt.Errorf("Runtime build journal phase is invalid")
	}
	return nil
}

func managedRuntimeStagingImage(runtimeID, revision string) string {
	digest := strings.TrimPrefix(revision, "sha256:")
	return "tobari-runtime-build-" + runtimeID + ":" + digest
}

func validRuntimeBuildAttemptID(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func (r *Runtime) beginRuntimeBuildJournal(ctx context.Context, runtimeID, runtimeName string) (runtimeBuildJournal, error) {
	if err := r.ensurePrivateDirectory(r.runtimeLifecycleDirectory()); err != nil {
		return runtimeBuildJournal{}, err
	}
	if prune, err := r.readRuntimePruneJournalObserved(); err != nil {
		return runtimeBuildJournal{}, err
	} else if prune != nil {
		return runtimeBuildJournal{}, fmt.Errorf("a Runtime prune journal requires recovery before another build")
	}
	path := r.runtimeBuildJournalPath()
	if _, err := os.Lstat(path); err == nil {
		return runtimeBuildJournal{}, fmt.Errorf("a Runtime build journal requires recovery before another build")
	} else if !errors.Is(err, os.ErrNotExist) {
		return runtimeBuildJournal{}, err
	}
	snapshot := r.runtimeBuildSnapshotPath()
	if _, err := os.Lstat(filepath.Dir(snapshot)); err == nil {
		return runtimeBuildJournal{}, fmt.Errorf("Runtime build staging snapshot lacks journal authority")
	} else if !errors.Is(err, os.ErrNotExist) {
		return runtimeBuildJournal{}, err
	}
	attemptID, err := r.identities.newRuntimeBuildAttemptID()
	if err != nil {
		return runtimeBuildJournal{}, err
	}
	journal := runtimeBuildJournal{SchemaVersion: runtimeBuildJournalSchema, Phase: runtimeBuildPhaseSnapshotting, RuntimeID: runtimeID, RuntimeName: runtimeName, AttemptID: attemptID, SnapshotPath: snapshot}
	if err := journal.Validate(r); err != nil {
		return runtimeBuildJournal{}, err
	}
	if err := writeAtomicJSON(path, journal); err != nil {
		if observed, observeErr := r.readRuntimeBuildJournalObserved(); observeErr == nil && observed != nil && *observed == journal {
			if cleanupErr := r.completeRuntimeBuildJournal(ctx, journal); cleanupErr != nil {
				return runtimeBuildJournal{}, fmt.Errorf("initialize Runtime build journal and rollback uncertain publication: %w", errors.Join(err, cleanupErr))
			}
		}
		return runtimeBuildJournal{}, err
	}
	return journal, nil
}

func (r *Runtime) writeRuntimeBuildJournal(previous, next runtimeBuildJournal) error {
	if err := previous.Validate(r); err != nil {
		return err
	}
	if err := next.Validate(r); err != nil {
		return err
	}
	current, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return err
	}
	if current == nil || *current != previous {
		return fmt.Errorf("Runtime build journal current authority changed")
	}
	if err := validateRuntimeBuildJournalTransition(previous, next); err != nil {
		return err
	}
	if r.runtimeBuildJournalWrite != nil {
		return r.runtimeBuildJournalWrite(previous, next)
	}
	return writeAtomicJSON(r.runtimeBuildJournalPath(), next)
}

func runtimeBuildCompletingJournal(origin runtimeBuildJournal) runtimeBuildJournal {
	completing := origin
	completing.CleanupFrom = origin.Phase
	completing.Phase = runtimeBuildPhaseCompleting
	completing.RemoveStaging = origin.Phase == runtimeBuildPhaseFailed && origin.StagingArtifact == runtimeBuildStagingOwned
	return completing
}

func runtimeBuildCleanupOrigin(completing runtimeBuildJournal) runtimeBuildJournal {
	origin := completing
	origin.Phase = completing.CleanupFrom
	origin.CleanupFrom = ""
	origin.RemoveStaging = false
	return origin
}

func (r *Runtime) validateRuntimeBuildCleanupStart(origin runtimeBuildJournal) error {
	if origin.Phase == runtimeBuildPhaseOrphanStaging || origin.Phase == runtimeBuildPhaseBuilding || origin.Phase == runtimeBuildPhaseBuilt || origin.Phase == runtimeBuildPhaseFinalTagged || origin.Phase == runtimeBuildPhaseStagingReleased || origin.Phase == runtimeBuildPhaseSnapshotPublished {
		return fmt.Errorf("Runtime build phase requires explicit recovery authorization")
	}
	if origin.Phase != runtimeBuildPhasePrepared && origin.Phase != runtimeBuildPhaseBuilding && origin.Phase != runtimeBuildPhaseFailed {
		return nil
	}
	info, err := os.Lstat(origin.SnapshotPath)
	if err != nil {
		return fmt.Errorf("active Runtime build snapshot cannot enter cleanup: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("active Runtime build snapshot is unsafe")
	}
	return nil
}

func validateRuntimeBuildJournalTransition(previous, next runtimeBuildJournal) error {
	if previous.SchemaVersion != next.SchemaVersion || previous.Restore != next.Restore || previous.ExpectedImageDigest != next.ExpectedImageDigest || (previous.Restore && previous.CreatedAt != next.CreatedAt) || previous.RuntimeID != next.RuntimeID || previous.RuntimeName != next.RuntimeName || previous.AttemptID != next.AttemptID || previous.SnapshotPath != next.SnapshotPath {
		return fmt.Errorf("Runtime build journal identity changed")
	}
	if previous.Revision != "" && (previous.Revision != next.Revision || previous.StagingImage != next.StagingImage || previous.FinalImage != next.FinalImage) {
		return fmt.Errorf("Runtime build journal target changed")
	}
	if previous.Phase == runtimeBuildPhaseBuilt ||
		previous.Phase == runtimeBuildPhaseFinalTagged ||
		previous.Phase == runtimeBuildPhaseStagingReleased ||
		previous.Phase == runtimeBuildPhaseSnapshotPublished {
		expected := previous
		expected.Phase = next.Phase
		if expected != next {
			return fmt.Errorf("Runtime build journal post-target authority changed")
		}
	}
	failedSettlement := previous.Phase == runtimeBuildPhaseFailed && next.Phase == runtimeBuildPhaseFailed && previous.AttemptSettlement == runtimeBuildAttemptUnsettled && next.AttemptSettlement == runtimeBuildAttemptSettled
	lateOwnedSettlement := previous.Phase == runtimeBuildPhaseFailed && next.Phase == runtimeBuildPhaseFailed && previous.AttemptSettlement == runtimeBuildAttemptSettled && next.AttemptSettlement == runtimeBuildAttemptSettled && previous.StagingArtifact == runtimeBuildStagingAbsent && next.StagingArtifact == runtimeBuildStagingOwned
	lateOwnedCleanup := previous.Phase == runtimeBuildPhaseCompleting && next.Phase == runtimeBuildPhaseCompleting && previous.CleanupFrom == runtimeBuildPhaseFailed && next.CleanupFrom == runtimeBuildPhaseFailed && previous.AttemptSettlement == runtimeBuildAttemptSettled && next.AttemptSettlement == runtimeBuildAttemptSettled && previous.StagingArtifact == runtimeBuildStagingAbsent && next.StagingArtifact == runtimeBuildStagingOwned
	if failedSettlement || lateOwnedSettlement {
		expected := previous
		expected.AttemptSettlement = next.AttemptSettlement
		expected.StagingArtifact = next.StagingArtifact
		expected.ImageDigest = next.ImageDigest
		if expected != next || (next.StagingArtifact != runtimeBuildStagingOwned && next.StagingArtifact != runtimeBuildStagingAbsent) || (next.StagingArtifact == runtimeBuildStagingOwned) != (next.ImageDigest != "") {
			return fmt.Errorf("Runtime failed build settlement authority changed")
		}
	}
	if lateOwnedCleanup {
		expected := previous
		expected.StagingArtifact = runtimeBuildStagingOwned
		expected.ImageDigest = next.ImageDigest
		expected.RemoveStaging = true
		if expected != next || tobari.ValidateDigest(next.ImageDigest) != nil {
			return fmt.Errorf("Runtime failed build cleanup authority changed")
		}
	}
	allowed := previous.Phase == runtimeBuildPhaseSnapshotting && next.Phase == runtimeBuildPhasePrepared ||
		previous.Phase == runtimeBuildPhasePrepared && next.Phase == runtimeBuildPhaseBuilding ||
		previous.Phase == runtimeBuildPhasePrepared && next.Phase == runtimeBuildPhaseOrphanStaging ||
		previous.Phase == runtimeBuildPhaseOrphanStaging && previous.OrphanStaging == runtimeBuildOrphanUnknown && next.Phase == runtimeBuildPhaseOrphanStaging && next.OrphanStaging == runtimeBuildOrphanAbsent && previous.ImageDigest == next.ImageDigest ||
		previous.Phase == runtimeBuildPhaseBuilding && (next.Phase == runtimeBuildPhaseBuilt || next.Phase == runtimeBuildPhaseFailed) ||
		previous.Phase == runtimeBuildPhaseBuilt && next.Phase == runtimeBuildPhaseFinalTagged ||
		previous.Phase == runtimeBuildPhaseFinalTagged && next.Phase == runtimeBuildPhaseStagingReleased ||
		previous.Phase == runtimeBuildPhaseStagingReleased && next.Phase == runtimeBuildPhaseSnapshotPublished ||
		previous.Phase == runtimeBuildPhaseSnapshotPublished && next.Phase == runtimeBuildPhaseManifestCommitted ||
		failedSettlement || lateOwnedSettlement || lateOwnedCleanup
	if !allowed {
		return fmt.Errorf("Runtime build journal phase transition is invalid")
	}
	return nil
}

type runtimeFailedAttemptObservation struct {
	stagingArtifact string
	imageDigest     string
}

func (r *Runtime) observeRuntimeFailedStaging(ctx context.Context, journal runtimeBuildJournal) (runtimeFailedAttemptObservation, error) {
	if journal.Phase != runtimeBuildPhaseFailed || journal.AttemptSettlement == "" {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build settlement authority is invalid")
	}
	digest, inspectErr := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
	if errors.Is(inspectErr, errManagedRuntimeImageMissing) {
		_, confirmErr := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
		if !errors.Is(confirmErr, errManagedRuntimeImageMissing) {
			return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build absence is not stable: %w", confirmErr)
		}
		return runtimeFailedAttemptObservation{stagingArtifact: runtimeBuildStagingAbsent}, nil
	}
	if inspectErr != nil {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build ownership is unknown: %w", inspectErr)
	}
	budget := runtimeLifecycleBudget{remaining: maxRuntimeContainersPerImage + 2}
	uses, err := r.observeRuntimeContainerUse(ctx, nil, map[string]string{journal.RuntimeID + "\x00" + journal.Revision: digest}, &budget)
	if err != nil {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build use is unknown: %w", err)
	}
	if use := uses[digest]; use.workspace || use.external {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build image remains in use")
	}
	return runtimeFailedAttemptObservation{stagingArtifact: runtimeBuildStagingOwned, imageDigest: digest}, nil
}

func (r *Runtime) observeRuntimeFailedAttempt(ctx context.Context, journal runtimeBuildJournal) (runtimeFailedAttemptObservation, error) {
	if err := r.requireRuntimeBuildSnapshotRevision(ctx, journal.SnapshotPath, journal.Revision); err != nil {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build snapshot authority changed: %w", err)
	}
	observed, err := r.observeRuntimeFailedStaging(ctx, journal)
	if err != nil {
		return runtimeFailedAttemptObservation{}, err
	}
	if err := r.requireRuntimeBuildSnapshotRevision(ctx, journal.SnapshotPath, journal.Revision); err != nil {
		return runtimeFailedAttemptObservation{}, fmt.Errorf("Runtime failed build snapshot authority changed: %w", err)
	}
	return observed, nil
}

func (r *Runtime) readRuntimeBuildJournalObserved() (*runtimeBuildJournal, error) {
	path := r.runtimeBuildJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Runtime build journal must be a regular owner-only file")
	}
	var journal runtimeBuildJournal
	if err := readStrictJSON(path, &journal); err != nil {
		return nil, err
	}
	if err := journal.Validate(r); err != nil {
		return nil, err
	}
	return &journal, nil
}

func (r *Runtime) writeRuntimeBuildCompletionAuthority(origin, completing runtimeBuildJournal) error {
	if err := origin.Validate(r); err != nil {
		return err
	}
	if err := completing.Validate(r); err != nil {
		return err
	}
	current, err := r.readRuntimeBuildJournalObserved()
	if err != nil || current == nil || *current != origin {
		return fmt.Errorf("Runtime build cleanup authority changed: %w", err)
	}
	writeCompletion := func() error { return writeAtomicJSON(r.runtimeBuildJournalPath(), completing) }
	if r.runtimeBuildCompletionWrite != nil {
		writeCompletion = func() error { return r.runtimeBuildCompletionWrite(completing) }
	}
	if writeErr := writeCompletion(); writeErr != nil {
		observed, observeErr := r.readRuntimeBuildJournalObserved()
		if observeErr != nil || observed == nil || *observed != completing {
			return fmt.Errorf("publish Runtime build cleanup authority: %w", errors.Join(writeErr, observeErr))
		}
		if syncErr := r.syncRuntimeBuildDirectory(r.runtimeLifecycleDirectory()); syncErr != nil {
			return fmt.Errorf("durably confirm Runtime build cleanup authority: %w", errors.Join(writeErr, syncErr))
		}
	}
	return nil
}

func (r *Runtime) validateCompletedRuntimeBuildAuthority(ctx context.Context, journal runtimeBuildJournal) error {
	if journal.Phase != runtimeBuildPhaseManifestCommitted {
		return nil
	}
	manifest, err := r.readRuntimeManifest(journal.RuntimeName)
	if err != nil || manifest.ID != journal.RuntimeID {
		return fmt.Errorf("completed Runtime build manifest authority changed: %w", err)
	}
	var matched *tobari.RuntimeRevision
	for index := range manifest.Revisions {
		revision := &manifest.Revisions[index]
		if revision.Revision != journal.Revision {
			continue
		}
		if matched != nil {
			return fmt.Errorf("completed Runtime build revision authority is duplicated")
		}
		matched = revision
	}
	if matched == nil || matched.Image != journal.FinalImage || matched.ImageDigest != journal.ImageDigest {
		return fmt.Errorf("completed Runtime build revision authority changed")
	}
	if journal.Restore && journal.ImageDigest != journal.ExpectedImageDigest {
		return fmt.Errorf("completed Runtime restore digest authority changed")
	}
	info, err := os.Lstat(matched.SnapshotPath)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("completed Runtime build snapshot authority changed: %w", err)
	}
	if err := r.requireRuntimeBuildSnapshotRevision(ctx, matched.SnapshotPath, journal.Revision); err != nil {
		return fmt.Errorf("completed Runtime build snapshot authority changed: %w", err)
	}
	digest, err := r.inspectManagedRuntimeBuildEvidence(ctx, journal.FinalImage, journal.RuntimeID, journal.Revision)
	if err != nil || digest != journal.ImageDigest {
		return fmt.Errorf("completed Runtime build image authority changed: %w", err)
	}
	return nil
}

func (r *Runtime) completeRuntimeBuildJournal(ctx context.Context, journal runtimeBuildJournal) error {
	if err := journal.Validate(r); err != nil {
		return err
	}
	origin := journal
	completing := journal
	if journal.Phase == runtimeBuildPhaseCompleting {
		origin = runtimeBuildCleanupOrigin(journal)
	} else {
		completing = runtimeBuildCompletingJournal(origin)
	}
	if err := completing.Validate(r); err != nil {
		return err
	}
	current, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return err
	}
	if current == nil || (*current != origin && *current != completing) {
		return fmt.Errorf("Runtime build journal completion authority changed")
	}
	if err := r.validateCompletedRuntimeBuildAuthority(ctx, origin); err != nil {
		return err
	}
	if *current == origin {
		if err := r.validateRuntimeBuildCleanupStart(origin); err != nil {
			return err
		}
		if err := r.writeRuntimeBuildCompletionAuthority(origin, completing); err != nil {
			return err
		}
	}
	if r.runtimeBuildCleanup != nil {
		return r.runtimeBuildCleanup(completing)
	}
	if origin.StagingImage != "" {
		attempt := []string(nil)
		if origin.Phase == runtimeBuildPhaseFailed {
			attempt = []string{origin.AttemptID}
		}
		observedDigest, inspectErr := r.inspectManagedRuntimeBuildEvidence(ctx, origin.StagingImage, origin.RuntimeID, origin.Revision, attempt...)
		if inspectErr == nil && completing.RemoveStaging && observedDigest != origin.ImageDigest {
			return fmt.Errorf("Runtime staging tag content changed before cleanup")
		}
		if inspectErr == nil && completing.RemoveStaging {
			if err := r.runner.Run(ctx, []string{"image", "rm", origin.StagingImage}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
				return fmt.Errorf("remove owned Runtime staging tag during cleanup: %w", err)
			}
			_, inspectErr = r.inspectManagedRuntimeBuildEvidence(ctx, origin.StagingImage, origin.RuntimeID, origin.Revision, attempt...)
		}
		if inspectErr == nil && !completing.RemoveStaging {
			return fmt.Errorf("Runtime staging tag appeared outside cleanup authority")
		}
		if inspectErr == nil {
			return fmt.Errorf("Runtime staging tag removal outcome requires reconciliation")
		}
		if !errors.Is(inspectErr, errManagedRuntimeImageMissing) {
			return fmt.Errorf("Runtime staging tag cleanup requires reconciliation: %w", inspectErr)
		}
	}
	if err := r.removeRuntimeBuildSnapshot(origin.SnapshotPath); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Dir(origin.SnapshotPath)); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("Runtime build staging snapshot was not removed")
		}
		return err
	}
	removeJournal := os.Remove
	if r.runtimeBuildJournalRemove != nil {
		removeJournal = r.runtimeBuildJournalRemove
	}
	if err := removeJournal(r.runtimeBuildJournalPath()); err != nil {
		if _, observeErr := os.Lstat(r.runtimeBuildJournalPath()); !errors.Is(observeErr, os.ErrNotExist) {
			return errors.Join(err, observeErr)
		}
	}
	return r.syncRuntimeBuildDirectory(r.runtimeLifecycleDirectory())
}

// RecoverRuntimeBuildCleanup is the explicit mutation boundary for resuming an
// already-authorized build cleanup. It never infers cleanup from a missing
// snapshot while an ordinary active phase is recorded.
func (r *Runtime) RecoverRuntimeBuildCleanup(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil {
				if _, snapshotErr := os.Lstat(filepath.Dir(r.runtimeBuildSnapshotPath())); !errors.Is(snapshotErr, os.ErrNotExist) {
					if snapshotErr == nil {
						return fmt.Errorf("Runtime build staging snapshot lacks journal authority")
					}
					return snapshotErr
				}
				return syncDirectoryIfPresent(r.runtimeLifecycleDirectory())
			}
			if journal.Phase != runtimeBuildPhaseCompleting {
				return fmt.Errorf("Runtime build journal is not in explicit cleanup recovery")
			}
			if journal.CleanupFrom == runtimeBuildPhaseFailed {
				origin := runtimeBuildCleanupOrigin(*journal)
				observed, err := r.observeRuntimeFailedStaging(recoveryContext, origin)
				if err != nil {
					return err
				}
				if journal.StagingArtifact == runtimeBuildStagingAbsent && observed.stagingArtifact == runtimeBuildStagingOwned {
					upgraded := *journal
					upgraded.StagingArtifact = runtimeBuildStagingOwned
					upgraded.ImageDigest = observed.imageDigest
					upgraded.RemoveStaging = true
					if err := r.writeRuntimeBuildJournal(*journal, upgraded); err != nil {
						return err
					}
					journal = &upgraded
				}
				if journal.StagingArtifact == runtimeBuildStagingOwned && observed.stagingArtifact == runtimeBuildStagingOwned && journal.ImageDigest != observed.imageDigest {
					return fmt.Errorf("Runtime failed build image authority changed")
				}
			}
			return r.completeRuntimeBuildJournal(recoveryContext, *journal)
		})
	})
}

// RecoverRuntimeBuildPublication resumes only the monotonic post-build
// publication phases. Prepared/building authority is never inferred as a
// completed Docker effect.
func (r *Runtime) RecoverRuntimeBuildPublication(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil || (journal.Phase != runtimeBuildPhaseBuilt && journal.Phase != runtimeBuildPhaseFinalTagged && journal.Phase != runtimeBuildPhaseStagingReleased && journal.Phase != runtimeBuildPhaseSnapshotPublished && journal.Phase != runtimeBuildPhaseManifestCommitted) {
				return fmt.Errorf("Runtime build publication recovery authority is absent")
			}
			_, err = r.resumeManagedRuntimePublicationLocked(recoveryContext, *journal)
			return err
		})
	})
}

// RecoverRuntimeBuildOrphanStaging explicitly authorizes recovery of a
// pre-existing staging conflict after re-observing it under the lifecycle
// mutation lock. Unknown ownership remains a durable blocker.
func (r *Runtime) RecoverRuntimeBuildOrphanStaging(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil || journal.Phase != runtimeBuildPhaseOrphanStaging {
				return fmt.Errorf("Runtime orphan staging recovery authority is absent")
			}
			origin := *journal
			observed, inspectErr := r.inspectManagedRuntimeBuildEvidence(recoveryContext, journal.StagingImage, journal.RuntimeID, journal.Revision)
			switch {
			case errors.Is(inspectErr, errManagedRuntimeImageMissing):
				if origin.OrphanStaging == runtimeBuildOrphanUnknown {
					settled := origin
					settled.OrphanStaging = runtimeBuildOrphanAbsent
					if err := r.writeRuntimeBuildJournal(origin, settled); err != nil {
						return err
					}
					origin = settled
				}
			case inspectErr == nil && journal.OrphanStaging == runtimeBuildOrphanExactManaged && observed == journal.ImageDigest:
				// Exact managed ownership is revalidated below before deletion.
			default:
				return fmt.Errorf("Runtime orphan staging ownership requires review: %w", inspectErr)
			}
			completing := runtimeBuildCompletingJournal(origin)
			completing.RemoveStaging = inspectErr == nil
			if err := completing.Validate(r); err != nil {
				return err
			}
			if err := r.writeRuntimeBuildCompletionAuthority(origin, completing); err != nil {
				return err
			}
			return r.completeRuntimeBuildJournal(recoveryContext, completing)
		})
	})
}

// RecoverRuntimeBuildPreDocker explicitly abandons only snapshotting or
// prepared work. Prepared recovery first proves that the staging selector is
// still absent. A newly observed selector is journaled as an orphan conflict
// and is never silently deleted by pre-Docker cleanup.
func (r *Runtime) RecoverRuntimeBuildPreDocker(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil || (journal.Phase != runtimeBuildPhaseSnapshotting && journal.Phase != runtimeBuildPhasePrepared) {
				return fmt.Errorf("Runtime pre-Docker recovery authority is absent")
			}
			if err := r.validateRuntimeBuildStagingSnapshotRoot(*journal); err != nil {
				return err
			}
			if journal.Phase == runtimeBuildPhasePrepared {
				if err := r.requireRuntimeBuildSnapshotRevision(recoveryContext, journal.SnapshotPath, journal.Revision); err != nil {
					return fmt.Errorf("prepared Runtime build snapshot authority changed: %w", err)
				}
				orphanDisposition, orphanDigest, observeErr := r.observeUnusedRuntimeStagingTag(recoveryContext, *journal)
				if observeErr != nil {
					orphan := *journal
					orphan.Phase = runtimeBuildPhaseOrphanStaging
					orphan.OrphanStaging = orphanDisposition
					orphan.ImageDigest = orphanDigest
					if journalErr := r.writeRuntimeBuildJournal(*journal, orphan); journalErr != nil {
						return fmt.Errorf("Runtime staging conflict requires reconciliation: %w", errors.Join(observeErr, journalErr))
					}
					return observeErr
				}
			}
			return r.completeRuntimeBuildJournal(recoveryContext, *journal)
		})
	})
}

func (r *Runtime) validateRuntimeBuildStagingSnapshotRoot(journal runtimeBuildJournal) error {
	parent := filepath.Dir(journal.SnapshotPath)
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		if journal.Phase == runtimeBuildPhasePrepared {
			return fmt.Errorf("prepared Runtime build snapshot is missing")
		}
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("Runtime build staging snapshot root is unsafe")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(journal.SnapshotPath) || entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return fmt.Errorf("Runtime build staging snapshot root contains unknown authority")
		}
	}
	if journal.Phase == runtimeBuildPhasePrepared && len(entries) != 1 {
		return fmt.Errorf("prepared Runtime build snapshot is missing")
	}
	return nil
}

// RecoverRuntimeBuildBuilding reconciles only an outcome-unknown Docker build
// boundary. It never treats the building phase itself as cleanup authority.
// Exact managed staging evidence can be adopted only after compatibility and
// the immutable source snapshot are revalidated under the same bounded lock.
func (r *Runtime) RecoverRuntimeBuildBuilding(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil || journal.Phase != runtimeBuildPhaseBuilding || journal.StagingArtifact != runtimeBuildStagingUnknown {
				return fmt.Errorf("Runtime building recovery authority is absent")
			}

			imageDigest, inspectErr := r.inspectManagedRuntimeBuildEvidence(recoveryContext, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
			if errors.Is(inspectErr, errManagedRuntimeImageMissing) {
				failed := *journal
				failed.Phase = runtimeBuildPhaseFailed
				failed.StagingArtifact = runtimeBuildStagingAbsent
				return r.writeRuntimeBuildJournal(*journal, failed)
			}
			if inspectErr != nil {
				return fmt.Errorf("Runtime building image outcome requires review: %w", inspectErr)
			}
			if err := r.validateManagedRuntimeBuildCompatibility(recoveryContext, journal.StagingImage); err != nil {
				return fmt.Errorf("Runtime building image compatibility requires review: %w", err)
			}
			if err := r.requireRuntimeBuildSnapshotRevision(recoveryContext, journal.SnapshotPath, journal.Revision); err != nil {
				return fmt.Errorf("Runtime building snapshot drift requires review: %w", err)
			}
			if err := r.freezeRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
				return fmt.Errorf("freeze reconciled Runtime build snapshot: %w", err)
			}
			if err := r.syncRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
				return fmt.Errorf("sync reconciled Runtime build snapshot: %w", err)
			}
			if err := r.requireRuntimeBuildSnapshotRevision(recoveryContext, journal.SnapshotPath, journal.Revision); err != nil {
				return fmt.Errorf("Runtime building snapshot drift requires review: %w", err)
			}
			if journal.Restore && imageDigest != journal.ExpectedImageDigest {
				failed := *journal
				failed.Phase = runtimeBuildPhaseFailed
				failed.ImageDigest = imageDigest
				failed.StagingArtifact = runtimeBuildStagingOwned
				failed.AttemptSettlement = runtimeBuildAttemptSettled
				if err := r.writeRuntimeBuildJournal(*journal, failed); err != nil {
					return fmt.Errorf("publish irreproducible Runtime restore evidence: %w", err)
				}
				return tobari.ErrRuntimeRevisionUnrestorable
			}

			built := *journal
			built.Phase = runtimeBuildPhaseBuilt
			built.ImageDigest = imageDigest
			built.StagingArtifact = runtimeBuildStagingOwned
			built.AttemptSettlement = runtimeBuildAttemptSettled
			if !journal.Restore {
				createdAt := time.Now().UTC()
				if r.identities.now != nil {
					createdAt = r.identities.now().UTC()
				}
				built.CreatedAt = createdAt.Format(time.RFC3339Nano)
			}
			if err := r.writeRuntimeBuildJournal(*journal, built); err != nil {
				observed, observeErr := r.readRuntimeBuildJournalObserved()
				if observeErr == nil && observed != nil && *observed == built {
					return nil
				}
				return fmt.Errorf("publish reconciled Runtime build authority: %w", errors.Join(err, observeErr))
			}
			return nil
		})
	})
}

// RecoverRuntimeBuildFailed settles one exact failed attempt and continues its
// cleanup within the same bounded recovery invocation. Unknown ownership/use
// remains a durable blocker against late Docker effects and selector reuse.
func (r *Runtime) RecoverRuntimeBuildFailed(ctx context.Context) error {
	recoveryContext, cancel := r.runtimeBuildRecoveryContext(ctx)
	defer cancel()
	return r.WithLifecycleLock(recoveryContext, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			journal, err := r.readRuntimeBuildJournalObserved()
			if err != nil {
				return err
			}
			if err := requireRuntimeBuildRecoveryReference(recoveryContext, journal); err != nil {
				return err
			}
			if journal == nil || journal.Phase != runtimeBuildPhaseFailed {
				return fmt.Errorf("Runtime failed build recovery authority is absent")
			}
			observed, err := r.observeRuntimeFailedAttempt(recoveryContext, *journal)
			if err != nil {
				return err
			}
			if journal.StagingArtifact == runtimeBuildStagingOwned && observed.stagingArtifact == runtimeBuildStagingOwned && journal.ImageDigest != observed.imageDigest {
				return fmt.Errorf("Runtime failed build image authority changed")
			}
			if journal.AttemptSettlement == runtimeBuildAttemptUnsettled {
				settled := *journal
				settled.AttemptSettlement = runtimeBuildAttemptSettled
				settled.StagingArtifact = observed.stagingArtifact
				settled.ImageDigest = observed.imageDigest
				if err := r.writeRuntimeBuildJournal(*journal, settled); err != nil {
					return err
				}
				journal = &settled
			}
			if journal.StagingArtifact == runtimeBuildStagingAbsent && observed.stagingArtifact == runtimeBuildStagingOwned {
				settled := *journal
				settled.StagingArtifact = runtimeBuildStagingOwned
				settled.ImageDigest = observed.imageDigest
				if err := r.writeRuntimeBuildJournal(*journal, settled); err != nil {
					return err
				}
				journal = &settled
			}
			return r.completeRuntimeBuildJournal(recoveryContext, *journal)
		})
	})
}

func (r *Runtime) rollbackRuntimeBuildBeforeDocker(ctx context.Context, cause error, allowed ...runtimeBuildJournal) error {
	current, observeErr := r.readRuntimeBuildJournalObserved()
	matched := false
	if observeErr == nil && current != nil {
		for _, candidate := range allowed {
			if *current == candidate {
				matched = true
				break
			}
		}
	}
	if !matched {
		return fmt.Errorf("Runtime build did not start but journal authority requires reconciliation: %w", errors.Join(cause, observeErr))
	}
	if cleanupErr := r.completeRuntimeBuildJournal(ctx, *current); cleanupErr != nil {
		return fmt.Errorf("Runtime build did not start and owned staging cleanup requires reconciliation: %w", errors.Join(cause, cleanupErr))
	}
	return cause
}

func (r *Runtime) retainRuntimeBuildFailure(ctx context.Context, journal runtimeBuildJournal, cause error) error {
	failed := journal
	failed.Phase = runtimeBuildPhaseFailed
	failed.AttemptSettlement = runtimeBuildAttemptUnsettled
	digest, inspectErr := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
	switch {
	case inspectErr == nil:
		failed.StagingArtifact = runtimeBuildStagingOwned
		failed.ImageDigest = digest
	case errors.Is(inspectErr, errManagedRuntimeImageMissing):
		failed.StagingArtifact = runtimeBuildStagingAbsent
	default:
		failed.StagingArtifact = runtimeBuildStagingUnknown
	}
	if journalErr := r.writeRuntimeBuildJournal(journal, failed); journalErr != nil {
		return fmt.Errorf("Runtime build outcome requires reconciliation: %w", errors.Join(cause, inspectErr, journalErr))
	}
	if failed.StagingArtifact == runtimeBuildStagingUnknown {
		return fmt.Errorf("Runtime build staging outcome requires reconciliation: %w", errors.Join(cause, inspectErr))
	}
	return cause
}

type managedRuntimeBuildEvidence struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	RuntimeID string `json:"runtime_id"`
	Revision  string `json:"revision"`
	AttemptID string `json:"attempt_id"`
}

func (r *Runtime) inspectManagedRuntimeBuildEvidence(ctx context.Context, image, runtimeID, revision string, attemptID ...string) (string, error) {
	if tobari.ValidateImageSelector(image) != nil || tobari.ValidateRuntimeID(runtimeID) != nil || tobari.ValidateDigest(revision) != nil || len(attemptID) > 1 || (len(attemptID) == 1 && !validRuntimeBuildAttemptID(attemptID[0])) {
		return "", fmt.Errorf("managed Runtime build evidence request is invalid")
	}
	format := `{"id":{{json .Id}},` +
		`"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"runtime_id":{{json (index .Config.Labels "` + managedRuntimeIDLabel + `")}},` +
		`"revision":{{json (index .Config.Labels "` + managedRuntimeRevisionLabel + `")}},` +
		`"attempt_id":{{json (index .Config.Labels "` + managedRuntimeBuildAttemptLabel + `")}}}`
	stdout := &boundedBuffer{limit: 4096}
	stderr := &boundedBuffer{limit: 4096}
	err := r.runner.Run(ctx, []string{"image", "inspect", "--format", format, image}, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return "", fmt.Errorf("managed Runtime build evidence exceeds the observation bound")
	}
	output := stdout.buffer.Bytes()
	if err != nil {
		if isMissingRuntimeImageInspect(err, stderr.buffer.Bytes(), image) {
			return "", errManagedRuntimeImageMissing
		}
		return "", fmt.Errorf("inspect managed Runtime build evidence: %w", err)
	}
	var evidence managedRuntimeBuildEvidence
	if decodeStrictJSON(output, &evidence) != nil || tobari.ValidateDigest(evidence.ID) != nil || evidence.Owner != ownerValue || evidence.Component != managedRuntimeComponentLabel || evidence.RuntimeID != runtimeID || evidence.Revision != revision {
		return "", fmt.Errorf("managed Runtime build ownership evidence is invalid")
	}
	if len(attemptID) == 1 && evidence.AttemptID != attemptID[0] {
		return "", fmt.Errorf("managed Runtime build attempt evidence is invalid")
	}
	return evidence.ID, nil
}

func (r *Runtime) validateManagedRuntimeBuildCompatibility(ctx context.Context, image string) error {
	if tobari.ValidateImageSelector(image) != nil {
		return fmt.Errorf("managed Runtime compatibility request is invalid")
	}
	format := `{"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},` +
		`"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},` +
		`"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`
	stdout := &boundedBuffer{limit: 4096}
	stderr := &boundedBuffer{limit: 4096}
	err := r.runner.Run(ctx, []string{"image", "inspect", "--format", format, image}, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return fmt.Errorf("managed Runtime compatibility evidence exceeds the observation bound")
	}
	if err != nil {
		return fmt.Errorf("inspect managed Runtime compatibility: %w", err)
	}
	var configuration struct {
		API        string   `json:"api"`
		Lifetime   string   `json:"lifetime"`
		User       string   `json:"user"`
		Entrypoint []string `json:"entrypoint"`
	}
	if decodeStrictJSON(bytes.TrimSpace(stdout.buffer.Bytes()), &configuration) != nil ||
		configuration.API != tobari.RuntimeImageAPI ||
		configuration.Lifetime != tobari.RuntimeImageLifetimeCommand ||
		configuration.User != "tobari" ||
		!equalStrings(configuration.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		return fmt.Errorf("managed Runtime compatibility evidence is invalid")
	}
	return nil
}

func isMissingRuntimeImageInspect(err error, diagnostic []byte, image string) bool {
	if err == nil {
		return false
	}
	message := string(diagnostic)
	if strings.HasSuffix(message, "\r\n") {
		message = strings.TrimSuffix(message, "\r\n")
	} else if strings.HasSuffix(message, "\n") {
		message = strings.TrimSuffix(message, "\n")
	}
	accepted := []string{
		"Error: No such image: " + image,
		"Error: No such object: " + image,
		"Error response from daemon: No such image: " + image,
		"Error response from daemon: No such object: " + image,
	}
	return slices.Contains(accepted, message)
}

func (r *Runtime) observeUnusedRuntimeStagingTag(ctx context.Context, journal runtimeBuildJournal) (string, string, error) {
	digest, err := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision)
	if errors.Is(err, errManagedRuntimeImageMissing) {
		return "", "", nil
	}
	if err == nil {
		return runtimeBuildOrphanExactManaged, digest, fmt.Errorf("Runtime staging image already exists without journal authority")
	}
	return runtimeBuildOrphanUnknown, "", fmt.Errorf("Runtime staging image ownership is unknown: %w", err)
}

func (r *Runtime) publishManagedRuntimeTag(ctx context.Context, journal runtimeBuildJournal) error {
	existing, err := r.inspectManagedRuntimeBuildEvidence(ctx, journal.FinalImage, journal.RuntimeID, journal.Revision)
	if err == nil {
		if existing != journal.ImageDigest {
			return fmt.Errorf("published Runtime tag has different content")
		}
		return nil
	}
	if !errors.Is(err, errManagedRuntimeImageMissing) {
		return fmt.Errorf("published Runtime tag ownership is unknown: %w", err)
	}
	if err := r.runner.Run(ctx, []string{"image", "tag", journal.StagingImage, journal.FinalImage}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("publish Runtime image tag: %w", err)
	}
	return nil
}

func runtimeLifecycleActivityFromBuild(journal runtimeBuildJournal) tobari.RuntimeLifecycleActivity {
	revisions := []string{}
	if journal.Revision != "" {
		revisions = []string{journal.Revision}
	}
	kind := tobari.RuntimeLifecycleActivityBuild
	if journal.Restore {
		kind = tobari.RuntimeLifecycleActivityRestore
	}
	return tobari.RuntimeLifecycleActivity{Kind: kind, RuntimeID: journal.RuntimeID, Revisions: revisions}
}

func sortRuntimeLifecycleJournals(journals *tobari.RuntimeLifecycleJournals) {
	sort.Slice(journals.Active, func(i, j int) bool {
		left, right := journals.Active[i], journals.Active[j]
		return left.RuntimeID+"\x00"+string(left.Kind)+"\x00"+strings.Join(left.Revisions, "\x00") < right.RuntimeID+"\x00"+string(right.Kind)+"\x00"+strings.Join(right.Revisions, "\x00")
	})
	sort.Slice(journals.FailedBuilds, func(i, j int) bool {
		left, right := journals.FailedBuilds[i], journals.FailedBuilds[j]
		return left.RuntimeID+"\x00"+left.Revision < right.RuntimeID+"\x00"+right.Revision
	})
}
