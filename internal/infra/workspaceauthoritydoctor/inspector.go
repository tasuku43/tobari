// Package workspaceauthoritydoctor composes doctor observations across the
// final Workspace-authority owner and generic host diagnostics. It never
// gives the Docker runtime a Store path or permission to rediscover a
// predecessor Manifest tree.
package workspaceauthoritydoctor

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type FinalAuthorityReader interface {
	ReadComplete(context.Context) (tobari.WorkspaceAuthorityCollection, bool, error)
}

type FinalMutationRecoveryReader interface {
	ObserveMutationRecovery(context.Context) (tobari.FinalAuthorityMutationObservation, error)
}

type FinalClusterObserver interface {
	Observe(context.Context) (tobari.FinalClusterStatus, error)
}

type GenericInspector interface {
	ObserveDoctorCheck(context.Context, string, doctor.CheckID) (doctor.Observation, error)
}

// FinalRuntimeMaterialObserver receives the already selected complete final
// authority. It may inspect only the exact Runtime bindings carried by that
// envelope; names, selectors, paths, and predecessor stores are not selection
// inputs.
type FinalRuntimeMaterialObserver interface {
	ObserveFinalRuntimeMaterials(context.Context, tobari.WorkspaceAuthorityCollection) ([]tobari.RuntimeBinding, error)
}

// FinalAuthInspector is research-only in production. It receives one complete
// final envelope and must observe only Context-owned final Broker authority.
type FinalAuthInspector interface {
	ObserveFinalAuthDoctorCheck(context.Context, tobari.WorkspaceAuthorityCollection, doctor.CheckID) (doctor.Observation, error)
}

type Inspector struct {
	authority FinalAuthorityReader
	cluster   FinalClusterObserver
	generic   GenericInspector
	runtime   FinalRuntimeMaterialObserver
	auth      FinalAuthInspector
}

func New(authority FinalAuthorityReader, cluster FinalClusterObserver, generic GenericInspector, auth FinalAuthInspector) (*Inspector, error) {
	if authority == nil || cluster == nil || generic == nil {
		return nil, fmt.Errorf("final doctor authorities are required")
	}
	runtime, ok := generic.(FinalRuntimeMaterialObserver)
	if !ok {
		return nil, fmt.Errorf("final Runtime material observer is required")
	}
	return &Inspector{authority: authority, cluster: cluster, generic: generic, runtime: runtime, auth: auth}, nil
}

func (i *Inspector) ObserveDoctorCheck(ctx context.Context, root string, id doctor.CheckID) (doctor.Observation, error) {
	if ctx == nil {
		return doctor.Observation{}, fmt.Errorf("doctor observation context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Observation{}, err
	}
	switch id {
	case doctor.CheckIDContext, doctor.CheckIDState, doctor.CheckIDPolicy, doctor.CheckIDPolicyData,
		doctor.CheckIDImageConfig, doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths,
		doctor.CheckIDAuthRootKey, doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion,
		doctor.CheckIDAuthVaultIntegrity, doctor.CheckIDAuthProjectHandles:
		collection, present, err := i.authority.ReadComplete(ctx)
		if err != nil {
			if errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
				return doctor.Observation{Status: doctor.CheckStatusFail, Detail: "unsupported pre-release authority is present; reset or recreate the development installation", Cause: doctor.ObservationCauseLegacyStatePresent}, nil
			}
			return doctor.Observation{Status: doctor.CheckStatusFail, Detail: "final Workspace authority is invalid, unsafe, or changed during observation"}, nil
		}
		if id == doctor.CheckIDContext {
			if recoveryReader, ok := i.authority.(FinalMutationRecoveryReader); ok {
				recovery, recoveryErr := recoveryReader.ObserveMutationRecovery(ctx)
				if recoveryErr != nil {
					return doctor.Observation{Status: doctor.CheckStatusFail, Detail: "final mutation recovery authority is invalid or unsafe"}, nil
				}
				if recovery.ActiveDecision || recovery.StagePresent {
					return doctor.Observation{
						Status: doctor.CheckStatusFail,
						Detail: "a final-authority mutation decision or stage is preserved; recover it through the exact initiating command",
						Cause:  doctor.ObservationCauseMutationRecoveryRequired,
					}, nil
				}
			}
		}
		return i.observeFinal(ctx, id, collection, present)
	default:
		return i.generic.ObserveDoctorCheck(ctx, root, id)
	}
}

func (i *Inspector) observeFinal(ctx context.Context, id doctor.CheckID, collection tobari.WorkspaceAuthorityCollection, present bool) (doctor.Observation, error) {
	switch id {
	case doctor.CheckIDContext:
		if !present {
			return observed(doctor.CheckStatusPass, "the final Workspace authority store is clean and empty"), nil
		}
		return observed(doctor.CheckStatusPass, fmt.Sprintf("final authority contains %d Templates, %d Contexts, and %d Workspaces", len(collection.Templates), len(collection.Contexts), len(collection.Workspaces))), nil
	case doctor.CheckIDState:
		status, err := i.cluster.Observe(ctx)
		if err != nil {
			return observed(doctor.CheckStatusFail, "final cluster authority could not be observed coherently"), nil
		}
		if err := status.Validate(); err != nil {
			return doctor.Observation{}, fmt.Errorf("invalid final cluster status: %w", err)
		}
		switch {
		case status.Runtime == tobari.FinalClusterRuntimeRunning && status.Receipt == tobari.FinalClusterReceiptActive:
			return observed(doctor.CheckStatusPass, "the final shared cluster is running with exact Gateway and OPA authority"), nil
		case status.Runtime == tobari.FinalClusterRuntimeAbsent && status.Receipt == tobari.FinalClusterReceiptAbsent,
			status.Runtime == tobari.FinalClusterRuntimeStopped && status.Receipt == tobari.FinalClusterReceiptStopped:
			return observed(doctor.CheckStatusWarn, "the final shared cluster is stopped"), nil
		default:
			return observed(doctor.CheckStatusFail, "the final shared cluster is interrupted, unhealthy, drifted, or unknown"), nil
		}
	case doctor.CheckIDPolicy:
		if !present || len(collection.Contexts) == 0 {
			return observed(doctor.CheckStatusWarn, "no final Context policy is active"), nil
		}
		active := 0
		for _, record := range collection.Contexts {
			if record.ActiveTemplatePolicy != nil && record.ActivePolicyMemory != nil && record.ActivePolicyMemoryRef != nil {
				active++
			}
		}
		if active != len(collection.Contexts) {
			return observed(doctor.CheckStatusWarn, fmt.Sprintf("%d of %d final Context policy axes are active", active, len(collection.Contexts))), nil
		}
		return observed(doctor.CheckStatusPass, fmt.Sprintf("all %d final Context policy axes are active", active)), nil
	case doctor.CheckIDPolicyData:
		rules := 0
		for _, record := range collection.Contexts {
			rules += len(record.PolicyMemory.Rules)
		}
		return observed(doctor.CheckStatusPass, fmt.Sprintf("final authority contains %d remembered policy rules; live pending candidates are reported by policy candidates", rules)), nil
	case doctor.CheckIDImageConfig:
		if !present {
			return observed(doctor.CheckStatusPass, "no final Template Runtime bindings require material observation"), nil
		}
		bindings, err := i.runtime.ObserveFinalRuntimeMaterials(ctx, collection)
		if err != nil {
			return observed(doctor.CheckStatusFail, "final Template Runtime material is missing, incompatible, drifted, or could not be observed exactly"), nil
		}
		return observed(doctor.CheckStatusPass, fmt.Sprintf("all %d distinct final Template Runtime bindings have exact available compatible material", len(bindings))), nil
	case doctor.CheckIDAuthProviderManifests, doctor.CheckIDAuthVaultPaths, doctor.CheckIDAuthRootKey,
		doctor.CheckIDAuthBroker, doctor.CheckIDCredentialCompanion, doctor.CheckIDAuthVaultIntegrity,
		doctor.CheckIDAuthProjectHandles:
		if i.auth == nil {
			return observed(doctor.CheckStatusFail, "final Context research authentication diagnostics are unavailable"), nil
		}
		return i.auth.ObserveFinalAuthDoctorCheck(ctx, collection, id)
	default:
		return doctor.Observation{}, fmt.Errorf("unsupported final doctor check %q", id)
	}
}

func observed(status doctor.CheckStatus, detail string) doctor.Observation {
	return doctor.Observation{Status: status, Detail: detail}
}
