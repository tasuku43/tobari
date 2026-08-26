package dockerruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimeCreateJournalFile = "create.json"
	runtimeCreateSchema      = 1
)

type runtimeCreatePhase string

const (
	runtimeCreatePrepared        runtimeCreatePhase = "prepared"
	runtimeCreateStatePublished  runtimeCreatePhase = "state_published"
	runtimeCreateConfigPublished runtimeCreatePhase = "config_published"
)

type runtimeCreateJournal struct {
	SchemaVersion int                `json:"schema_version"`
	Phase         runtimeCreatePhase `json:"phase"`
	RuntimeID     string             `json:"runtime_id"`
	RuntimeName   string             `json:"runtime_name"`
	ConfigStage   string             `json:"config_stage"`
	StateStage    string             `json:"state_stage"`
}

func (r *Runtime) runtimeCreateJournalPath() string {
	return filepath.Join(r.runtimeLifecycleDirectory(), runtimeCreateJournalFile)
}

func (j runtimeCreateJournal) validate(r *Runtime) error {
	if j.SchemaVersion != runtimeCreateSchema || tobari.ValidateRuntimeID(j.RuntimeID) != nil || tobari.ValidateName(j.RuntimeName) != nil {
		return fmt.Errorf("Runtime create transaction authority is invalid")
	}
	if j.Phase != runtimeCreatePrepared && j.Phase != runtimeCreateStatePublished && j.Phase != runtimeCreateConfigPublished {
		return fmt.Errorf("Runtime create transaction phase is invalid")
	}
	if filepath.Dir(j.ConfigStage) != r.configDirectory || filepath.Dir(j.StateStage) != r.stateDirectory ||
		!hasPathBasePrefix(j.ConfigStage, ".runtime-create-") || !hasPathBasePrefix(j.StateStage, ".runtime-create-") {
		return fmt.Errorf("Runtime create staging authority is not canonical")
	}
	return nil
}

func hasPathBasePrefix(path, prefix string) bool {
	base := filepath.Base(path)
	return len(base) > len(prefix) && base[:len(prefix)] == prefix
}

func (r *Runtime) readRuntimeCreateJournal() (*runtimeCreateJournal, error) {
	path := r.runtimeCreateJournalPath()
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("Runtime create journal must be a regular owner-only file")
	}
	var journal runtimeCreateJournal
	if err := readStrictJSON(path, &journal); err != nil {
		return nil, err
	}
	if err := journal.validate(r); err != nil {
		return nil, err
	}
	return &journal, nil
}

func (r *Runtime) writeRuntimeCreateJournal(previous *runtimeCreateJournal, next runtimeCreateJournal) error {
	if err := next.validate(r); err != nil {
		return err
	}
	current, err := r.readRuntimeCreateJournal()
	if err != nil {
		return err
	}
	if previous == nil {
		if current != nil {
			return fmt.Errorf("another Runtime create transaction is active")
		}
	} else if current == nil || !reflect.DeepEqual(*current, *previous) {
		return fmt.Errorf("Runtime create transaction authority changed")
	}
	if err := writeAtomicJSON(r.runtimeCreateJournalPath(), next); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) runtimeCreateBoundaryCall(phase string) error {
	if r.runtimeCreateBoundary != nil {
		return r.runtimeCreateBoundary(phase)
	}
	return nil
}

func (r *Runtime) settleRuntimeCreate(journal runtimeCreateJournal) (tobari.RuntimeReport, error) {
	if err := journal.validate(r); err != nil {
		return tobari.RuntimeReport{}, err
	}
	configTarget := r.runtimeDirectory(journal.RuntimeID)
	stateTarget := r.runtimeStateDirectory(journal.RuntimeID)
	if err := moveRuntimeCreateMember(journal.StateStage, stateTarget); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := syncDirectoryIfPresent(r.runtimeStatesDirectory()); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := r.runtimeCreateBoundaryCall("state_renamed"); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if journal.Phase == runtimeCreatePrepared {
		next := journal
		next.Phase = runtimeCreateStatePublished
		if err := r.writeRuntimeCreateJournal(&journal, next); err != nil {
			return tobari.RuntimeReport{}, err
		}
		journal = next
		if err := r.runtimeCreateBoundaryCall("journal_state_published"); err != nil {
			return tobari.RuntimeReport{}, err
		}
	}
	if err := moveRuntimeCreateMember(journal.ConfigStage, configTarget); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := syncDirectoryIfPresent(r.runtimesDirectory()); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := r.runtimeCreateBoundaryCall("config_renamed"); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if journal.Phase != runtimeCreateConfigPublished {
		next := journal
		next.Phase = runtimeCreateConfigPublished
		if err := r.writeRuntimeCreateJournal(&journal, next); err != nil {
			return tobari.RuntimeReport{}, err
		}
		journal = next
		if err := r.runtimeCreateBoundaryCall("journal_config_published"); err != nil {
			return tobari.RuntimeReport{}, err
		}
	}
	manifest, err := r.readRuntimeManifestByID(journal.RuntimeID)
	if err != nil || manifest.Name != journal.RuntimeName {
		return tobari.RuntimeReport{}, fmt.Errorf("Runtime create publication does not match its journal: %w", err)
	}
	if err := r.validateStrictRuntimeDirectory(manifest, nil); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := os.Remove(r.runtimeCreateJournalPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.RuntimeReport{}, err
	}
	if err := syncDirectoryIfPresent(r.runtimeLifecycleDirectory()); err != nil {
		return tobari.RuntimeReport{}, err
	}
	result := tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}
	return result, result.Validate()
}

func moveRuntimeCreateMember(stage, target string) error {
	stageInfo, stageErr := os.Lstat(stage)
	targetInfo, targetErr := os.Lstat(target)
	switch {
	case stageErr == nil && errors.Is(targetErr, os.ErrNotExist):
		if !stageInfo.IsDir() || stageInfo.Mode()&os.ModeSymlink != 0 || stageInfo.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime create staging directory is unsafe")
		}
		return os.Rename(stage, target)
	case errors.Is(stageErr, os.ErrNotExist) && targetErr == nil:
		if !targetInfo.IsDir() || targetInfo.Mode()&os.ModeSymlink != 0 || targetInfo.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime create target directory is unsafe")
		}
		return nil
	case stageErr == nil && targetErr == nil:
		return fmt.Errorf("Runtime create staging and target both exist")
	default:
		return errors.Join(stageErr, targetErr)
	}
}
