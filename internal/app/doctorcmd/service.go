// Package doctorcmd implements the read-only doctor use case.
package doctorcmd

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
)

// InspectorPort is the smallest infrastructure capability needed by doctor.
// Infrastructure adapters satisfy it structurally and do not import app.
type InspectorPort interface {
	ObserveDoctorCheck(context.Context, string, doctor.CheckID) (doctor.Observation, error)
}

// Service coordinates the doctor inspection.
type Service struct {
	inspector InspectorPort
}

// New creates a doctor service.
func New(inspector InspectorPort) *Service {
	return &Service{inspector: inspector}
}

// Run validates the declared read intent before crossing the infrastructure
// boundary.
func (s *Service) Run(ctx context.Context, intent operation.Intent, root string) (doctor.Report, error) {
	if ctx == nil {
		return doctor.Report{}, fmt.Errorf("doctor context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Report{}, err
	}
	if err := intent.Validate(); err != nil {
		return doctor.Report{}, fmt.Errorf("doctor intent: %w", err)
	}
	if intent.Command != "doctor" || intent.Effect != operation.EffectRead {
		return doctor.Report{}, fmt.Errorf("doctor requires the doctor read intent")
	}
	if s == nil || portcheck.IsNil(s.inspector) {
		return doctor.Report{}, fmt.Errorf("doctor inspector is not configured")
	}

	inventory := doctor.CheckInventory()
	checks := make([]doctor.Check, 0, len(inventory))
	statuses := make(map[doctor.CheckID]doctor.CheckStatus, len(inventory))
	for _, spec := range inventory {
		if err := ctx.Err(); err != nil {
			return doctor.Report{}, err
		}
		if blockedBy, blocked := firstUnavailablePrerequisite(spec.Prerequisites, statuses); blocked {
			blocker := blockedBy
			check := doctor.Check{
				Name: spec.ID, Status: doctor.CheckStatusBlocked,
				BlockedBy: &blocker,
			}
			checks = append(checks, check)
			statuses[spec.ID] = check.Status
			continue
		}
		observation, err := s.inspector.ObserveDoctorCheck(ctx, root, spec.ID)
		if contextErr := ctx.Err(); contextErr != nil {
			return doctor.Report{}, contextErr
		}
		if err != nil {
			return doctor.Report{}, fmt.Errorf("inspect %s: %w", spec.ID, err)
		}
		if err := observation.Validate(); err != nil {
			return doctor.Report{}, fmt.Errorf("invalid %s observation: %w", spec.ID, err)
		}
		check := doctor.Check{
			Name: spec.ID, Status: observation.Status, Detail: observation.Detail,
			Recovery: recoveryFor(spec.ID, observation.Status),
		}
		checks = append(checks, check)
		statuses[spec.ID] = check.Status
	}
	if err := ctx.Err(); err != nil {
		return doctor.Report{}, err
	}
	report := doctor.Report{Checks: checks}
	if err := report.Validate(); err != nil {
		return doctor.Report{}, fmt.Errorf("invalid doctor report: %w", err)
	}
	return report, nil
}

func recoveryFor(id doctor.CheckID, status doctor.CheckStatus) *doctor.Recovery {
	if status == doctor.CheckStatusWarn {
		switch id {
		case doctor.CheckIDState:
			return &doctor.Recovery{Action: "Create or reconcile the shared Tobari cluster.", NextCommand: "cluster up"}
		case doctor.CheckIDAuthRootKey:
			return &doctor.Recovery{Action: "Initialize the installation root key through cluster reconciliation.", NextCommand: "cluster up"}
		case doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion:
			return &doctor.Recovery{Action: "Reconcile the shared cluster authentication services.", NextCommand: "cluster up"}
		default:
			return nil
		}
	}
	if status != doctor.CheckStatusFail {
		return nil
	}
	action := map[doctor.CheckID]string{
		doctor.CheckIDDockerCLI:             "Install a compatible Docker CLI and ensure docker is available on PATH.",
		doctor.CheckIDDockerEngine:          "Start or restore the local Docker Engine.",
		doctor.CheckIDDockerContext:         "Repair the selected Docker context so it can be read.",
		doctor.CheckIDDockerCompose:         "Install or enable the Docker Compose v2 plugin.",
		doctor.CheckIDProxyPort:             "Repair the Gateway proxy-port contract.",
		doctor.CheckIDRoot:                  "Choose an existing host directory that is safe for a Tobari project root.",
		doctor.CheckIDRootSharing:           "Repair Docker bind sharing for the selected project root.",
		doctor.CheckIDContext:               "Repair the current Context selection and Tobari XDG runtime paths.",
		doctor.CheckIDState:                 "Repair unsafe or invalid Tobari cluster state.",
		doctor.CheckIDPolicy:                "Correct the active policy source or its Docker-readable path.",
		doctor.CheckIDPolicyData:            "Repair invalid or unsafe learned policy data.",
		doctor.CheckIDImageConfig:           "Repair the selected runtime image configuration.",
		doctor.CheckIDAuthProviderManifests: "Repair the owner-controlled credential-provider manifest collection.",
		doctor.CheckIDAuthVaultPaths:        "Repair unsafe Auth Broker vault paths.",
		doctor.CheckIDAuthRootKey:           "Restore or repair the installation root-key backend.",
		doctor.CheckIDAuthBroker:            "Reconcile and unlock the shared Auth Broker.",
		doctor.CheckIDCredentialCompanion:   "Reconcile the trusted-host credential companion.",
		doctor.CheckIDAuthVaultIntegrity:    "Repair encrypted Context vault integrity without exposing contents.",
		doctor.CheckIDAuthProjectHandles:    "Repair inconsistent project-bound authentication state.",
		doctor.CheckIDOwnedResources:        "Repair Docker access before inspecting Tobari-owned resources.",
	}[id]
	nextCommand := "doctor"
	switch id {
	case doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion:
		nextCommand = "cluster up"
	}
	return &doctor.Recovery{Action: action, NextCommand: nextCommand}
}

func firstUnavailablePrerequisite(
	prerequisites []doctor.CheckID, statuses map[doctor.CheckID]doctor.CheckStatus,
) (doctor.CheckID, bool) {
	for _, prerequisite := range prerequisites {
		if statuses[prerequisite] != doctor.CheckStatusPass {
			return prerequisite, true
		}
	}
	return "", false
}
