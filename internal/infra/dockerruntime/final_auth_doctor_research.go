//go:build tobari_dev && tobari_research

package dockerruntime

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/rootkey"
)

// ObserveFinalAuthDoctorRuntimeCheck is called inside the final adapter's one
// non-creating lifecycle observation. The complete collection and exhaustive
// per-Context Broker inventories are its only selection authority; all host
// paths below are fixed Runtime-owned roots.
func (r *Runtime) ObserveFinalAuthDoctorRuntimeCheck(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool, inventories []authbroker.ContextStatusObservation, id doctor.CheckID) (doctor.Observation, error) {
	if r == nil {
		return doctor.Observation{}, fmt.Errorf("final research authentication runtime is unavailable")
	}
	switch id {
	case doctor.CheckIDAuthProviderManifests:
		projection, err := r.loadAuthProviders()
		if err != nil {
			return finalAuthObserved(doctor.CheckStatusFail, "credential-provider manifests are invalid, unsafe, or unknown"), nil
		}
		return finalAuthObserved(doctor.CheckStatusPass, fmt.Sprintf("%d credential-provider manifests normalize to owner-only projection schema v1", len(projection.Providers))), nil
	case doctor.CheckIDAuthVaultPaths:
		exists, err := rootkey.EncryptedStateExists(r.stateDirectory)
		if err != nil {
			return finalAuthObserved(doctor.CheckStatusFail, "final Context vault paths are unsafe or contain unknown authority"), nil
		}
		if exists {
			return finalAuthObserved(doctor.CheckStatusPass, "encrypted final Context vault paths are owner-only"), nil
		}
		return finalAuthObserved(doctor.CheckStatusPass, "no encrypted final Context vault is present"), nil
	case doctor.CheckIDAuthRootKey:
		return r.observeFinalAuthRootKey(ctx), nil
	case doctor.CheckIDAuthBroker:
		return r.observeFinalAuthClusterComponent(ctx, collection, present, "auth-broker")
	case doctor.CheckIDCredentialCompanion:
		return r.observeFinalAuthClusterComponent(ctx, collection, present, "credential-companion")
	case doctor.CheckIDAuthVaultIntegrity:
		return r.observeFinalAuthVaultIntegrity(collection, inventories), nil
	case doctor.CheckIDAuthProjectHandles:
		return r.observeFinalAuthContextHandles(ctx, collection, inventories), nil
	default:
		return doctor.Observation{}, fmt.Errorf("unsupported final research authentication doctor check %q", id)
	}
}

func (r *Runtime) observeFinalAuthRootKey(ctx context.Context) doctor.Observation {
	vaultsExist, vaultErr := rootkey.EncryptedStateExists(r.stateDirectory)
	provider, rootErr := rootkey.New(r.stateDirectory)
	if vaultErr != nil || rootErr != nil {
		return finalAuthObserved(doctor.CheckStatusFail, "the final installation root-key backend is unavailable or unsafe")
	}
	backend, exists, err := provider.Inspect(ctx, vaultsExist)
	if err != nil {
		return finalAuthObserved(doctor.CheckStatusFail, "the "+string(backend)+" root-key backend is unavailable or inconsistent with final encrypted state")
	}
	if exists {
		return finalAuthObserved(doctor.CheckStatusPass, "the "+string(backend)+" installation root key is available")
	}
	return finalAuthObserved(doctor.CheckStatusWarn, "the "+string(backend)+" installation root key will be created by cluster up")
}

func (r *Runtime) observeFinalAuthClusterComponent(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool, name string) (doctor.Observation, error) {
	status, err := r.ObserveFinalCluster(ctx, collection, present)
	if err != nil || status.Validate() != nil {
		return finalAuthObserved(doctor.CheckStatusFail, "the surface-selected final "+name+" authority is unknown"), nil
	}
	if present && (status.Authority != tobari.FinalClusterAuthorityPresent || status.Generation != collection.Generation || status.CollectionRevision != collection.Revision || status.TemplateCount != len(collection.Templates) || status.ContextCount != len(collection.Contexts) || status.WorkspaceCount != len(collection.Workspaces)) {
		return finalAuthObserved(doctor.CheckStatusFail, "the surface-selected final "+name+" observation crossed complete collection authority"), nil
	}
	if !present && status.Authority != tobari.FinalClusterAuthorityAbsent {
		return finalAuthObserved(doctor.CheckStatusFail, "the surface-selected final "+name+" observation claimed unexpected authority"), nil
	}
	var component *tobari.FinalClusterComponentObservation
	for index := range status.Components {
		if status.Components[index].Name == name {
			component = &status.Components[index]
			break
		}
	}
	if component == nil {
		return finalAuthObserved(doctor.CheckStatusFail, "the research surface omitted final "+name+" authority"), nil
	}
	return finalAuthClusterComponentObservation(name, *component), nil
}

func finalAuthClusterComponentObservation(name string, component tobari.FinalClusterComponentObservation) doctor.Observation {
	switch component.State {
	case tobari.FinalClusterRuntimeRunning:
		if component.Identity != tobari.FinalClusterEvidenceExact || component.Topology != tobari.FinalClusterEvidenceExact {
			return finalAuthObserved(doctor.CheckStatusFail, "the final "+name+" identity or topology drifted")
		}
		return finalAuthObserved(doctor.CheckStatusPass, "the final "+name+" is running with exact surface-selected authority")
	case tobari.FinalClusterRuntimeAbsent, tobari.FinalClusterRuntimeStopped:
		return finalAuthObserved(doctor.CheckStatusWarn, "the final "+name+" is stopped; run cluster up to reconcile it")
	default:
		return finalAuthObserved(doctor.CheckStatusFail, "the final "+name+" is unhealthy, drifted, or unknown")
	}
}

func (r *Runtime) observeFinalAuthVaultIntegrity(collection tobari.WorkspaceAuthorityCollection, inventories []authbroker.ContextStatusObservation) doctor.Observation {
	if len(collection.Contexts) == 0 {
		if len(inventories) != 0 {
			return finalAuthObserved(doctor.CheckStatusFail, "final Context credential inventory contains unknown authority")
		}
		return finalAuthObserved(doctor.CheckStatusPass, "no final Context vault requires Broker authentication")
	}
	projection, err := r.loadAuthProviders()
	if err != nil || len(inventories) != len(collection.Contexts) {
		return finalAuthObserved(doctor.CheckStatusFail, "final Context vault inventory is incomplete or its provider schema changed")
	}
	for index, inventory := range inventories {
		contextRef, refErr := tobari.ContextRef(collection.Contexts[index].Context.ID)
		if refErr != nil || inventory.ValidateFor(contextRef) != nil || !inventory.InventoryComplete || inventory.BrokerState != authbroker.BrokerStateReady || len(inventory.Providers) != len(projection.Providers) {
			return finalAuthObserved(doctor.CheckStatusFail, "an encrypted final Context vault could not be authenticated exhaustively")
		}
		for providerIndex, status := range inventory.Providers {
			if status.Provider != projection.Providers[providerIndex].ID {
				return finalAuthObserved(doctor.CheckStatusFail, "final Context credential inventory differs from the exact provider schema")
			}
		}
	}
	return finalAuthObserved(doctor.CheckStatusPass, fmt.Sprintf("all %d final Context vault inventories are complete and authenticated", len(inventories)))
}

func (r *Runtime) observeFinalAuthContextHandles(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, inventories []authbroker.ContextStatusObservation) doctor.Observation {
	if prerequisite := r.observeFinalAuthVaultIntegrity(collection, inventories); prerequisite.Status != doctor.CheckStatusPass {
		return finalAuthObserved(doctor.CheckStatusFail, "final Context handle prerequisites are incomplete")
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return finalAuthObserved(doctor.CheckStatusFail, "final Context handle provider authority is unavailable")
	}
	inventoryByContext := make(map[tobari.ContextID]authbroker.ContextStatusObservation, len(inventories))
	for _, inventory := range inventories {
		inventoryByContext[inventory.Authority.ContextID] = inventory
	}
	stale := 0
	checked := 0
	for _, workspace := range collection.Workspaces {
		inventory, ok := inventoryByContext[workspace.ContextID]
		if !ok {
			return finalAuthObserved(doctor.CheckStatusFail, "a final Workspace has no exact Context credential inventory")
		}
		for _, status := range inventory.Providers {
			if status.State != authbroker.ProviderCredentialConfigured {
				continue
			}
			_, encoded, _, bindingErr := brokerBindingsForProvider(projection, status.Provider)
			if bindingErr != nil {
				return finalAuthObserved(doctor.CheckStatusFail, "a final Context handle binding cannot be derived from the exact provider schema")
			}
			response, statusErr := r.runBrokerControl(ctx, nil,
				"binding_status", "--context-id", string(workspace.ContextID), "--project-id", string(workspace.ID),
				"--provider", status.Provider, "--revision", status.CredentialRevision, "--bindings", string(encoded),
			)
			if statusErr != nil || response.Provider != status.Provider || response.Revision != status.CredentialRevision {
				return finalAuthObserved(doctor.CheckStatusFail, "a final Context handle could not be verified exactly")
			}
			checked++
			switch response.State {
			case "ready":
			case "missing", "stale":
				stale++
			default:
				return finalAuthObserved(doctor.CheckStatusFail, "the Auth Broker returned unknown final Context handle state")
			}
		}
	}
	if stale != 0 {
		return finalAuthObserved(doctor.CheckStatusWarn, fmt.Sprintf("%d final Context handles require the next matching Context entry", stale))
	}
	return finalAuthObserved(doctor.CheckStatusPass, fmt.Sprintf("all %d expected final Context handles match exact Workspace and provider authority", checked))
}

func finalAuthObserved(status doctor.CheckStatus, detail string) doctor.Observation {
	return doctor.Observation{Status: status, Detail: detail}
}
