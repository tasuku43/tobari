// Package migrationcmd owns the explicit installation-state migration task.
package migrationcmd

import (
	"context"
	"errors"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// MigrationPort is the one controlled infrastructure boundary used by the
// explicit migration task.
type MigrationPort interface {
	MigrateInstallation(context.Context, io.Writer) (tobari.MigrationReport, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.MigrationTargetKind &&
		intent.Target.ID == tobari.MigrationTargetID && intent.Target.ParentID == "" {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "migration target is not owned by Tobari", false)
}

type Service struct {
	migration MigrationPort
	mutator   *execution.Invoker
}

func New(migration MigrationPort) *Service {
	return &Service{migration: migration, mutator: execution.New(ownedPolicy{})}
}

// Apply validates one fixed-target migration intent and returns only a fully
// validated, task-owned report.
func (s *Service) Apply(ctx context.Context, intent operation.Intent, diagnostics io.Writer) (tobari.MigrationReport, error) {
	if s == nil || portcheck.IsNil(s.migration) {
		return tobari.MigrationReport{}, fault.New(fault.KindInternal, "missing_runtime", "installation migration is not configured", false)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "migrate apply", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.MigrationTargetKind, ID: tobari.MigrationTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.MigrationReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		migrated, err := s.migration.MigrateInstallation(actionContext, diagnostics)
		if err != nil {
			return migrationFault(err)
		}
		if err := migrated.Validate(); err != nil {
			return fault.Wrap(fault.KindContract, "invalid_migration_report", "migration report is invalid", false, err)
		}
		result = migrated
		return nil
	})
	if err != nil {
		return tobari.MigrationReport{}, err
	}
	return result, nil
}

func migrationFault(err error) error {
	switch {
	case errors.Is(err, tobari.ErrMigrationNotSupported):
		return fault.Wrap(fault.KindRejected, "migration_not_supported", "installation state is not a supported migration source", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the exact failed state boundary."})
	case errors.Is(err, tobari.ErrMigrationSourceUnsafe):
		return fault.Wrap(fault.KindRejected, "migration_source_rejected", "migration source is unsafe, invalid, or ambiguous", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the unchanged Context state."})
	case errors.Is(err, tobari.ErrMigrationRuntimeConflict):
		return fault.Wrap(fault.KindRejected, "migration_runtime_conflict", "a deterministic migration Runtime conflicts with existing state", false, err,
			fault.NextAction{Command: "runtime list", Reason: "Inspect the unchanged Runtime catalog."})
	case errors.Is(err, tobari.ErrMigrationRuntimeFailed):
		return fault.Wrap(fault.KindRejected, "migration_runtime_failed", "a legacy Runtime could not be promoted", false, err,
			fault.NextAction{Command: "runtime list", Reason: "Inspect any retained draft or ready migration Runtime."})
	case errors.Is(err, tobari.ErrMigrationBackupFailed):
		return fault.Wrap(fault.KindInternal, "migration_backup_failed", "the private migration backup could not be created", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the unchanged Context state and owner-only configuration paths."})
	case errors.Is(err, tobari.ErrMigrationSourceChanged):
		return fault.Wrap(fault.KindRejected, "migration_source_changed", "migration source changed during validation", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the current Context state before another migration."})
	case errors.Is(err, tobari.ErrMigrationWriteFailed):
		return fault.Wrap(fault.KindContract, "migration_incomplete", "migration did not commit every planned Context", false, err,
			fault.NextAction{Command: "doctor", Reason: "Reconcile the current and remaining predecessor Contexts before another mutation."})
	default:
		return err
	}
}
