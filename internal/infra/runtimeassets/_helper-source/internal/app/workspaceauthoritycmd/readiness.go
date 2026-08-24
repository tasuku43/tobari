package workspaceauthoritycmd

import (
	"context"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

const minimumWorkspaceDockerEngineMajor = 24

type WorkspaceEntryReadinessPort interface {
	ObserveDoctorCheck(context.Context, string, doctor.CheckID) (doctor.Observation, error)
}

// WorkspaceEntryReadinessService applies the existing closed Doctor profile
// before root entry. It is read-only and owns no provider selection or repair.
type WorkspaceEntryReadinessService struct {
	observer WorkspaceEntryReadinessPort
}

func NewWorkspaceEntryReadinessService(observer WorkspaceEntryReadinessPort) *WorkspaceEntryReadinessService {
	return &WorkspaceEntryReadinessService{observer: observer}
}

func (s *WorkspaceEntryReadinessService) Check(ctx context.Context) error {
	if ctx == nil {
		return workspaceReadinessFault(fault.KindInternal, "missing_runtime", "Generic Docker readiness observation is not configured.")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || portcheck.IsNil(s.observer) {
		return workspaceReadinessFault(fault.KindInternal, "missing_runtime", "Generic Docker readiness observation is not configured.")
	}
	checks, err := doctor.ReadinessChecks(doctor.ReadinessProfileWorkspaceStart)
	if err != nil {
		return workspaceReadinessFault(fault.KindContract, "invalid_readiness_profile", "The Workspace readiness profile is invalid.")
	}
	for _, id := range checks {
		if err := ctx.Err(); err != nil {
			return err
		}
		observation, observeErr := s.observer.ObserveDoctorCheck(ctx, "", id)
		if observeErr != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return unavailableWorkspaceReadiness(id)
		}
		if err := observation.Validate(); err != nil {
			return workspaceReadinessFault(fault.KindContract, "invalid_readiness_observation", "A generic Docker readiness observation was invalid.")
		}
		if observation.Status != doctor.CheckStatusPass {
			return unavailableWorkspaceReadiness(id)
		}
		if id == doctor.CheckIDDockerEngine && !workspaceDockerEngineCompatible(observation.Value) {
			return workspaceReadinessFault(fault.KindUnsupported, "docker_engine_incompatible", "Docker Engine 24 or newer is required.")
		}
	}
	return nil
}

func workspaceDockerEngineCompatible(version string) bool {
	majorText, _, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	return majorText != "" && err == nil && major >= minimumWorkspaceDockerEngineMajor
}

func unavailableWorkspaceReadiness(id doctor.CheckID) error {
	pair, ok := map[doctor.CheckID][2]string{
		doctor.CheckIDDockerCLI:     {"docker_cli_unavailable", "The generic Docker CLI is unavailable."},
		doctor.CheckIDDockerEngine:  {"docker_engine_unavailable", "The selected Docker Engine is unavailable."},
		doctor.CheckIDDockerContext: {"docker_context_unavailable", "The selected Docker context cannot be read."},
		doctor.CheckIDDockerCompose: {"docker_compose_unavailable", "Docker Compose v2 is unavailable."},
	}[id]
	if !ok {
		return workspaceReadinessFault(fault.KindContract, "invalid_readiness_profile", "The Workspace readiness profile contains an unsupported check.")
	}
	return workspaceReadinessFault(fault.KindUnavailable, pair[0], pair[1])
}

func workspaceReadinessFault(kind fault.Kind, code, message string) error {
	return fault.WithClassification(fault.New(
		kind, code, message, false,
		fault.NextAction{Command: "doctor", Reason: "Inspect generic Docker readiness before starting a Workspace."},
	), fault.PhasePrecondition, fault.ChangeNone)
}
