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
		if tobari.ValidateDigest(j.Revision) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.Revision) {
			return fmt.Errorf("Runtime build journal target is invalid")
		}
		if j.ImageDigest != "" && tobari.ValidateDigest(j.ImageDigest) != nil {
			return fmt.Errorf("Runtime build journal image evidence is invalid")
		}
	case runtimeBuildPhaseBuilt:
		if tobari.ValidateDigest(j.Revision) != nil || tobari.ValidateDigest(j.ImageDigest) != nil || j.StagingImage != managedRuntimeStagingImage(j.RuntimeID, j.Revision) || j.FinalImage != managedLibraryRuntimeImage(j.RuntimeName, j.Revision) {
			return fmt.Errorf("built Runtime journal evidence is invalid")
		}
	default:
		return fmt.Errorf("Runtime build journal phase is invalid")
	}
	return nil
}

func managedRuntimeStagingImage(runtimeID, revision string) string {
	id := strings.ReplaceAll(runtimeID, "-", "")
	if len(id) > 12 {
		id = id[:12]
	}
	digest := strings.TrimPrefix(revision, "sha256:")
	if len(digest) > 12 {
		digest = digest[:12]
	}
	return "tobari-runtime-build-" + id + ":" + digest
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
		return runtimeBuildJournal{}, err
	}
	return journal, nil
}

func (r *Runtime) writeRuntimeBuildJournal(journal runtimeBuildJournal) error {
	if err := journal.Validate(r); err != nil {
		return err
	}
	return writeAtomicJSON(r.runtimeBuildJournalPath(), journal)
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
	if r.runtimeBuildCleanup != nil {
		return r.runtimeBuildCleanup(journal)
	}
	removeRuntimeSnapshot(journal.SnapshotPath)
	if _, err := os.Lstat(filepath.Dir(journal.SnapshotPath)); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("Runtime build staging snapshot was not removed")
		}
		return err
	}
	if err := os.Remove(r.runtimeBuildJournalPath()); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) rollbackRuntimeBuildBeforeDocker(journal runtimeBuildJournal, cause error) error {
	if cleanupErr := r.completeRuntimeBuildJournal(journal); cleanupErr != nil {
		return fmt.Errorf("Runtime build did not start and owned staging cleanup requires reconciliation: %w", errors.Join(cause, cleanupErr))
	}
	return cause
}

func (r *Runtime) retainRuntimeBuildFailure(journal runtimeBuildJournal, cause error) error {
	journal.Phase = runtimeBuildPhaseFailed
	if journalErr := r.writeRuntimeBuildJournal(journal); journalErr != nil {
		return fmt.Errorf("Runtime build outcome requires reconciliation: %w", errors.Join(cause, journalErr))
	}
	return cause
}

type managedRuntimeBuildEvidence struct {
	ID    string `json:"id"`
	Owned bool   `json:"owned"`
}

func (r *Runtime) inspectManagedRuntimeBuildEvidence(ctx context.Context, image, runtimeID, revision string) (string, error) {
	if tobari.ValidateImageSelector(image) != nil || tobari.ValidateRuntimeID(runtimeID) != nil || tobari.ValidateDigest(revision) != nil {
		return "", fmt.Errorf("managed Runtime build evidence request is invalid")
	}
	format := `{"id":{{json .Id}},"owned":{{json (and ` +
		`(eq (index .Config.Labels "` + ownerLabel + `") "` + ownerValue + `") ` +
		`(eq (index .Config.Labels "` + componentLabel + `") "` + managedRuntimeComponentLabel + `") ` +
		`(eq (index .Config.Labels "` + managedRuntimeIDLabel + `") "` + runtimeID + `") ` +
		`(eq (index .Config.Labels "` + managedRuntimeRevisionLabel + `") "` + revision + `"))}}}`
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
	if decodeStrictJSON(output, &evidence) != nil || tobari.ValidateDigest(evidence.ID) != nil || !evidence.Owned {
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
