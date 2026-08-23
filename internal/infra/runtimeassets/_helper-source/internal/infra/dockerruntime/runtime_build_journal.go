package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

var errManagedRuntimeImageMissing = errors.New("managed Runtime image is missing")

const (
	runtimeBuildJournalSchema = 1
	runtimeBuildJournalFile   = "build.json"
	runtimeBuildSnapshotDir   = "build-source"

	runtimeBuildPhaseSnapshotting = "snapshotting"
	runtimeBuildPhasePrepared     = "prepared"
	runtimeBuildPhaseBuilding     = "building"
	runtimeBuildPhaseBuilt        = "built"
	runtimeBuildPhaseFailed       = "failed"

	managedRuntimeComponentLabel = "runtime-revision"
	managedRuntimeIDLabel        = "io.tobari.runtime-id"
	managedRuntimeRevisionLabel  = "io.tobari.runtime-revision"
)

type runtimeBuildJournal struct {
	SchemaVersion int    `json:"schema_version"`
	Phase         string `json:"phase"`
	RuntimeID     string `json:"runtime_id"`
	RuntimeName   string `json:"runtime_name"`
	Revision      string `json:"revision,omitempty"`
	StagingImage  string `json:"staging_image,omitempty"`
	FinalImage    string `json:"final_image,omitempty"`
	ImageDigest   string `json:"image_digest,omitempty"`
	SnapshotPath  string `json:"snapshot_path"`
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
	if j.SchemaVersion != runtimeBuildJournalSchema || tobari.ValidateRuntimeID(j.RuntimeID) != nil || tobari.ValidateName(j.RuntimeName) != nil || j.SnapshotPath != r.runtimeBuildSnapshotPath() {
		return fmt.Errorf("Runtime build journal authority is invalid")
	}
	switch j.Phase {
	case runtimeBuildPhaseSnapshotting:
		if j.Revision != "" || j.StagingImage != "" || j.FinalImage != "" || j.ImageDigest != "" {
			return fmt.Errorf("snapshotting Runtime build journal has premature evidence")
		}
	case runtimeBuildPhasePrepared, runtimeBuildPhaseBuilding, runtimeBuildPhaseFailed:
		if tobari.ValidateDigest(j.Revision) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.RuntimeID, j.Revision) {
			return fmt.Errorf("Runtime build journal target is invalid")
		}
		if (j.Phase == runtimeBuildPhasePrepared || j.Phase == runtimeBuildPhaseBuilding) && j.ImageDigest != "" {
			return fmt.Errorf("pre-build-completion Runtime journal has premature image evidence")
		}
		if j.ImageDigest != "" && tobari.ValidateDigest(j.ImageDigest) != nil {
			return fmt.Errorf("Runtime build journal image evidence is invalid")
		}
	case runtimeBuildPhaseBuilt:
		if tobari.ValidateDigest(j.Revision) != nil || tobari.ValidateDigest(j.ImageDigest) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.RuntimeID, j.Revision) {
			return fmt.Errorf("built Runtime journal evidence is invalid")
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

func (r *Runtime) beginRuntimeBuildJournal(runtimeID, runtimeName string) (runtimeBuildJournal, error) {
	if err := r.ensurePrivateDirectory(r.runtimeLifecycleDirectory()); err != nil {
		return runtimeBuildJournal{}, err
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
	journal := runtimeBuildJournal{SchemaVersion: runtimeBuildJournalSchema, Phase: runtimeBuildPhaseSnapshotting, RuntimeID: runtimeID, RuntimeName: runtimeName, SnapshotPath: snapshot}
	if err := journal.Validate(r); err != nil {
		return runtimeBuildJournal{}, err
	}
	if err := writeAtomicJSON(path, journal); err != nil {
		if observed, observeErr := r.readRuntimeBuildJournalObserved(); observeErr == nil && observed != nil && *observed == journal {
			if cleanupErr := r.completeRuntimeBuildJournal(journal); cleanupErr != nil {
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
	return writeAtomicJSON(r.runtimeBuildJournalPath(), next)
}

func validateRuntimeBuildJournalTransition(previous, next runtimeBuildJournal) error {
	if previous.SchemaVersion != next.SchemaVersion || previous.RuntimeID != next.RuntimeID || previous.RuntimeName != next.RuntimeName || previous.SnapshotPath != next.SnapshotPath {
		return fmt.Errorf("Runtime build journal identity changed")
	}
	if previous.Revision != "" && (previous.Revision != next.Revision || previous.StagingImage != next.StagingImage || previous.FinalImage != next.FinalImage) {
		return fmt.Errorf("Runtime build journal target changed")
	}
	allowed := previous.Phase == runtimeBuildPhaseSnapshotting && next.Phase == runtimeBuildPhasePrepared ||
		previous.Phase == runtimeBuildPhasePrepared && next.Phase == runtimeBuildPhaseBuilding ||
		previous.Phase == runtimeBuildPhaseBuilding && (next.Phase == runtimeBuildPhaseBuilt || next.Phase == runtimeBuildPhaseFailed)
	if !allowed {
		return fmt.Errorf("Runtime build journal phase transition is invalid")
	}
	return nil
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

func (r *Runtime) completeRuntimeBuildJournal(journal runtimeBuildJournal) error {
	if err := journal.Validate(r); err != nil {
		return err
	}
	current, err := r.readRuntimeBuildJournalObserved()
	if err != nil {
		return err
	}
	if current == nil || *current != journal {
		return fmt.Errorf("Runtime build journal completion authority changed")
	}
	if r.runtimeBuildCleanup != nil {
		return r.runtimeBuildCleanup(journal)
	}
	if err := r.removeRuntimeBuildSnapshot(journal.SnapshotPath); err != nil {
		return err
	}
	if _, err := os.Lstat(filepath.Dir(journal.SnapshotPath)); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("Runtime build staging snapshot was not removed")
		}
		return err
	}
	if err := os.Remove(r.runtimeBuildJournalPath()); err != nil {
		return err
	}
	return r.syncRuntimeBuildDirectory(r.runtimeLifecycleDirectory())
}

func (r *Runtime) rollbackRuntimeBuildBeforeDocker(cause error, allowed ...runtimeBuildJournal) error {
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
	if cleanupErr := r.completeRuntimeBuildJournal(*current); cleanupErr != nil {
		return fmt.Errorf("Runtime build did not start and owned staging cleanup requires reconciliation: %w", errors.Join(cause, cleanupErr))
	}
	return cause
}

func (r *Runtime) retainRuntimeBuildFailure(journal runtimeBuildJournal, cause error) error {
	failed := journal
	failed.Phase = runtimeBuildPhaseFailed
	if journalErr := r.writeRuntimeBuildJournal(journal, failed); journalErr != nil {
		return fmt.Errorf("Runtime build outcome requires reconciliation: %w", errors.Join(cause, journalErr))
	}
	return cause
}

type managedRuntimeBuildEvidence struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	RuntimeID string `json:"runtime_id"`
	Revision  string `json:"revision"`
}

func (r *Runtime) inspectManagedRuntimeBuildEvidence(ctx context.Context, image, runtimeID, revision string) (string, error) {
	if tobari.ValidateImageSelector(image) != nil || tobari.ValidateRuntimeID(runtimeID) != nil || tobari.ValidateDigest(revision) != nil {
		return "", fmt.Errorf("managed Runtime build evidence request is invalid")
	}
	format := `{"id":{{json .Id}},` +
		`"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"runtime_id":{{json (index .Config.Labels "` + managedRuntimeIDLabel + `")}},` +
		`"revision":{{json (index .Config.Labels "` + managedRuntimeRevisionLabel + `")}}}`
	output, err := r.runner.Output(ctx, []string{"image", "inspect", "--format", format, image}, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) {
			return "", errManagedRuntimeImageMissing
		}
		return "", fmt.Errorf("inspect managed Runtime build evidence: %w", err)
	}
	if len(output) > 4096 {
		return "", fmt.Errorf("managed Runtime build evidence exceeds the observation bound")
	}
	var evidence managedRuntimeBuildEvidence
	if decodeStrictJSON(output, &evidence) != nil || tobari.ValidateDigest(evidence.ID) != nil || evidence.Owner != ownerValue || evidence.Component != managedRuntimeComponentLabel || evidence.RuntimeID != runtimeID || evidence.Revision != revision {
		return "", fmt.Errorf("managed Runtime build ownership evidence is invalid")
	}
	return evidence.ID, nil
}

func (r *Runtime) requireUnusedRuntimeStagingTag(ctx context.Context, journal runtimeBuildJournal) error {
	_, err := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision)
	if errors.Is(err, errManagedRuntimeImageMissing) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("Runtime staging image already exists without journal authority")
	}
	return fmt.Errorf("Runtime staging image ownership is unknown: %w", err)
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
	return tobari.RuntimeLifecycleActivity{Kind: tobari.RuntimeLifecycleActivityBuild, RuntimeID: journal.RuntimeID, Revisions: revisions}
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
