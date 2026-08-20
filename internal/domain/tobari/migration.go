package tobari

import (
	"errors"
	"fmt"
	"path/filepath"
)

const (
	TaskMigrationApply = "state.migrate"

	MigrationTargetKind = "installation-state"
	MigrationTargetID   = "installation-state"

	MigrationSourcePreV1ContextPolicyRuntime = "pre_v1_context_policy_runtime"
)

var (
	ErrMigrationNotSupported    = errors.New("installation state is not a supported migration source")
	ErrMigrationSourceUnsafe    = errors.New("migration source is unsafe or invalid")
	ErrMigrationRuntimeConflict = errors.New("migration Runtime conflicts with existing state")
	ErrMigrationRuntimeFailed   = errors.New("migration Runtime could not be prepared")
	ErrMigrationBackupFailed    = errors.New("migration backup could not be created")
	ErrMigrationSourceChanged   = errors.New("migration source changed during review")
	ErrMigrationWriteFailed     = errors.New("migration state could not be committed")
)

// MigrationContextState distinguishes a newly converted Context from an
// already-current Context included in the exhaustive installation result.
type MigrationContextState string

const (
	MigrationContextCurrent  MigrationContextState = "current"
	MigrationContextMigrated MigrationContextState = "migrated"
)

func (s MigrationContextState) Validate() error {
	if s != MigrationContextCurrent && s != MigrationContextMigrated {
		return fmt.Errorf("migration Context state is invalid")
	}
	return nil
}

// MigrationContextResult is one Context in the complete migration scope.
type MigrationContextResult struct {
	ID             string                `json:"id"`
	Name           string                `json:"name"`
	State          MigrationContextState `json:"state"`
	Runtime        string                `json:"runtime"`
	PolicyRevision string                `json:"policy_revision"`
}

func (r MigrationContextResult) Validate() error {
	if err := ValidateContextID(r.ID); err != nil {
		return err
	}
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if err := r.State.Validate(); err != nil {
		return err
	}
	if _, _, err := ParseRuntimeSelection(r.Runtime); err != nil {
		return err
	}
	if err := ValidateDigest(r.PolicyRevision); err != nil {
		return err
	}
	return nil
}

// MigrationReport confirms one complete observation and any committed local
// migration. Backup is nil for an already-current no-op.
type MigrationReport struct {
	Task     string                   `json:"task"`
	Source   string                   `json:"source"`
	Changed  bool                     `json:"changed"`
	Backup   *string                  `json:"backup"`
	Contexts []MigrationContextResult `json:"contexts"`
}

func (r MigrationReport) Validate() error {
	if r.Task != TaskMigrationApply || r.Source != MigrationSourcePreV1ContextPolicyRuntime {
		return fmt.Errorf("migration task identity is invalid")
	}
	if r.Contexts == nil || len(r.Contexts) == 0 {
		return fmt.Errorf("migration Context collection is empty")
	}
	seen := make(map[string]struct{}, len(r.Contexts))
	migrated := false
	for _, item := range r.Contexts {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, ok := seen[item.Name]; ok {
			return fmt.Errorf("migration Context is duplicated")
		}
		seen[item.Name] = struct{}{}
		migrated = migrated || item.State == MigrationContextMigrated
	}
	if r.Changed != migrated {
		return fmt.Errorf("migration changed state is inconsistent")
	}
	if r.Changed {
		if r.Backup == nil || *r.Backup == "" || !filepath.IsAbs(*r.Backup) || filepath.Clean(*r.Backup) != *r.Backup {
			return fmt.Errorf("migration backup path is invalid")
		}
	} else if r.Backup != nil {
		return fmt.Errorf("unchanged migration cannot report a backup")
	}
	return nil
}
