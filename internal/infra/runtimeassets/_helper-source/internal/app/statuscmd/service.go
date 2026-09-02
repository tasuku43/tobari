package statuscmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type SnapshotPort interface {
	ObserveStatusHome(context.Context) (tobari.StatusHomeObservation, error)
}

type Service struct{ port SnapshotPort }

func New(port SnapshotPort) *Service { return &Service{port: port} }

func (s *Service) Snapshot(ctx context.Context) (tobari.StatusHomeSnapshot, error) {
	if s == nil || portcheck.IsNil(s.port) {
		return tobari.StatusHomeSnapshot{}, fault.New(fault.KindInternal, "missing_port", "status snapshot adapter is unavailable", false)
	}
	observed, err := s.port.ObserveStatusHome(ctx)
	if err != nil {
		if errors.Is(err, tobari.ErrFinalAuthorityMigrationRequired) {
			return tobari.StatusHomeSnapshot{}, fault.WithClassification(fault.Wrap(
				fault.KindRejected,
				"installation_migration_required",
				"The supported typed authority.json must be explicitly reviewed and migrated before active authority can be used.",
				false,
				err,
				fault.NextAction{Command: "installation migration plan", Reason: "Create a fresh read-only stale-bound migration plan."},
			), fault.PhaseObservation, fault.ChangeNotApplicable)
		}
		if errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
			return tobari.StatusHomeSnapshot{}, fault.WithClassification(fault.Wrap(
				fault.KindRejected,
				"legacy_state_present",
				"Unsupported pre-release installation authority is present or unsafe; final authority was not used or changed.",
				false,
				err,
				fault.NextAction{Command: "doctor", Reason: "Reset or recreate this pre-release installation before initializing final authority."},
			), fault.PhaseObservation, fault.ChangeNotApplicable)
		}
		return tobari.StatusHomeSnapshot{}, fault.Wrap(fault.KindUnavailable, "status_observation_failed", "status could not obtain one coherent read-only snapshot", true, err)
	}
	result, err := tobari.NewStatusHomeSnapshot(observed.Collection, observed.Present, observed.ProjectRoot, observed.Live)
	if err != nil {
		return tobari.StatusHomeSnapshot{}, fault.Wrap(fault.KindContract, "invalid_status_snapshot", "status snapshot is contradictory", false, fmt.Errorf("derive status home: %w", err))
	}
	return result, nil
}
