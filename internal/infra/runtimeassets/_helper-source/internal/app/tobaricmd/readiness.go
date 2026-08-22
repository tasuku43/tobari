package tobaricmd

import (
	"context"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

const minimumDockerEngineMajor = 24

type readinessRuntimePort interface {
	ObserveDoctorCheck(context.Context, string, doctor.CheckID) (doctor.Observation, error)
}

type readinessContextKey uint8

// CheckWorkspaceStartPrerequisites performs the closed generic Docker profile
// and returns a context receipt that prevents a composed first-use flow from
// repeating the same observations in cluster up.
func (s *Service) CheckWorkspaceStartPrerequisites(ctx context.Context) (context.Context, error) {
	if err := s.checkWorkspaceStartPrerequisites(ctx); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, readinessContextKey(0), true), nil
}

func (s *Service) checkWorkspaceStartPrerequisites(ctx context.Context) error {
	if err := s.requireRuntime(); err != nil {
		return err
	}
	observer, ok := s.runtime.(readinessRuntimePort)
	if !ok || portcheck.IsNil(observer) {
		return classifiedReadinessFault(
			fault.KindInternal, "missing_runtime", "Generic Docker readiness observation is not configured.",
		)
	}
	checks, err := doctor.ReadinessChecks(doctor.ReadinessProfileWorkspaceStart)
	if err != nil {
		return classifiedReadinessFault(
			fault.KindContract, "invalid_readiness_profile", "The Workspace readiness profile is invalid.",
		)
	}
	for _, id := range checks {
		if err := ctx.Err(); err != nil {
			return err
		}
		observation, observeErr := observer.ObserveDoctorCheck(ctx, "", id)
		if observeErr != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return readinessUnavailable(id)
		}
		if err := observation.Validate(); err != nil {
			return classifiedReadinessFault(
				fault.KindContract, "invalid_readiness_observation", "A generic Docker readiness observation was invalid.",
			)
		}
		if observation.Status != doctor.CheckStatusPass {
			return readinessUnavailable(id)
		}
		if id == doctor.CheckIDDockerEngine && !compatibleDockerEngine(observation.Value) {
			return classifiedReadinessFault(
				fault.KindUnsupported, "docker_engine_incompatible", "Docker Engine 24 or newer is required.",
			)
		}
	}
	return nil
}

func workspaceStartPrerequisitesChecked(ctx context.Context) bool {
	checked, _ := ctx.Value(readinessContextKey(0)).(bool)
	return checked
}

func compatibleDockerEngine(version string) bool {
	majorText, _, _ := strings.Cut(version, ".")
	if majorText == "" {
		return false
	}
	major, err := strconv.Atoi(majorText)
	return err == nil && major >= minimumDockerEngineMajor
}

func readinessUnavailable(id doctor.CheckID) error {
	pair, ok := map[doctor.CheckID][2]string{
		doctor.CheckIDDockerCLI:     {"docker_cli_unavailable", "The generic Docker CLI is unavailable."},
		doctor.CheckIDDockerEngine:  {"docker_engine_unavailable", "The selected Docker Engine is unavailable."},
		doctor.CheckIDDockerContext: {"docker_context_unavailable", "The selected Docker context cannot be read."},
		doctor.CheckIDDockerCompose: {"docker_compose_unavailable", "Docker Compose v2 is unavailable."},
	}[id]
	if !ok {
		return classifiedReadinessFault(
			fault.KindContract, "invalid_readiness_profile", "The Workspace readiness profile contains an unsupported check.",
		)
	}
	return classifiedReadinessFault(fault.KindUnavailable, pair[0], pair[1])
}

func classifiedReadinessFault(kind fault.Kind, code, message string) error {
	return fault.WithClassification(fault.New(
		kind, code, message, false,
		fault.NextAction{Command: "doctor", Reason: "Inspect generic Docker readiness before starting a Workspace."},
	), fault.PhasePrecondition, fault.ChangeNone)
}
