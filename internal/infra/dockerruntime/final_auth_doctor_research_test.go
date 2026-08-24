//go:build tobari_dev && tobari_research

package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func finalAuthDoctorInventory(t *testing.T, runtime *Runtime, collection tobari.WorkspaceAuthorityCollection, configured string) []authbroker.ContextStatusObservation {
	t.Helper()
	projection, err := runtime.loadAuthProviders()
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	result := make([]authbroker.ContextStatusObservation, 0, len(snapshots))
	for _, snapshot := range snapshots {
		ref, _ := tobari.ContextRef(snapshot.Context.ID)
		authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, ref)
		if err != nil {
			t.Fatal(err)
		}
		statuses := make([]authbroker.ProviderStatus, 0, len(projection.Providers))
		for _, provider := range projection.Providers {
			status := authbroker.ProviderStatus{Provider: provider.ID, State: authbroker.ProviderCredentialNotConfigured}
			if provider.ID == configured {
				status.State = authbroker.ProviderCredentialConfigured
				status.CredentialRevision = authDoctorRevision
			}
			statuses = append(statuses, status)
		}
		observation, err := authbroker.NewContextStatusObservation(authority, authbroker.StorageBackendXDGFile, authbroker.BrokerStateReady, statuses, true)
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, observation)
	}
	return result
}

func TestFinalAuthDoctorUsesCompleteFinalInventoriesAndExactHandleDimensions(t *testing.T) {
	runner := &authDoctorRunner{bindingState: "ready"}
	runtime := newFinalClusterStatusRuntime(t, runner)
	collection := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	inventories := finalAuthDoctorInventory(t, runtime, collection, authbroker.BuiltinGitHubProviderID)
	// Poison predecessor stores. The exact final handle observation must not
	// enumerate or decode either tree.
	if err := os.MkdirAll(runtime.contextsDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.contextsDirectory(), "malformed-predecessor"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtime.stateDirectory, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.stateDirectory, "projects", "malformed-predecessor"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}

	vault := runtime.observeFinalAuthVaultIntegrity(collection, inventories)
	if vault.Status != doctor.CheckStatusPass {
		t.Fatalf("vault observation=%#v", vault)
	}
	handles := runtime.observeFinalAuthContextHandles(context.Background(), collection, inventories)
	if handles.Status != doctor.CheckStatusPass {
		t.Fatalf("handle observation=%#v", handles)
	}
	calls := authDoctorControlCalls(runner, "binding_status")
	if len(calls) != 1 {
		t.Fatalf("binding calls=%v", calls)
	}
	call := calls[0]
	for name, want := range map[string]string{
		"--context-id": string(collection.Contexts[0].Context.ID),
		"--project-id": string(collection.Workspaces[0].ID),
		"--provider":   authbroker.BuiltinGitHubProviderID,
		"--revision":   authDoctorRevision,
	} {
		if got := authDoctorArgument(call, name); got != want {
			t.Fatalf("binding %s=%q want=%q argv=%v", name, got, want, call)
		}
	}
	if !slices.Contains(call, "--bindings") {
		t.Fatalf("binding call lacks normalized exact projection: %v", call)
	}
}

func TestFinalAuthDoctorFailsIncompleteUnknownAndUnsafeAuthority(t *testing.T) {
	runtime := newFinalClusterStatusRuntime(t, &authDoctorRunner{})
	collection := finalProjectionCollectionFixture(t, "")
	inventories := finalAuthDoctorInventory(t, runtime, collection, "")
	incomplete, err := authbroker.NewContextStatusObservation(inventories[0].Authority, authbroker.StorageBackendXDGFile, authbroker.BrokerStateUnavailable, []authbroker.ProviderStatus{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.observeFinalAuthVaultIntegrity(collection, []authbroker.ContextStatusObservation{incomplete}); got.Status != doctor.CheckStatusFail {
		t.Fatalf("incomplete inventory=%#v", got)
	}
	if got := finalAuthClusterComponentObservation("auth-broker", tobari.FinalClusterComponentObservation{Name: "auth-broker", State: tobari.FinalClusterRuntimeUnknown, Identity: tobari.FinalClusterEvidenceUnknown, Topology: tobari.FinalClusterEvidenceUnknown}); got.Status != doctor.CheckStatusFail {
		t.Fatalf("unknown component=%#v", got)
	}
	unknown := filepath.Join(runtime.stateDirectory, "auth", "contexts", "unknown")
	if err := os.MkdirAll(unknown, 0o700); err != nil {
		t.Fatal(err)
	}
	if got, err := runtime.ObserveFinalAuthDoctorRuntimeCheck(context.Background(), collection, true, inventories, doctor.CheckIDAuthVaultPaths); err != nil || got.Status != doctor.CheckStatusFail {
		t.Fatalf("unsafe paths=%#v err=%v", got, err)
	}
}

func TestFinalAuthDoctorClusterComponentRunningAndStoppedCanaries(t *testing.T) {
	running := tobari.FinalClusterComponentObservation{Name: "auth-broker", State: tobari.FinalClusterRuntimeRunning, Health: "healthy", Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact}
	if got := finalAuthClusterComponentObservation("auth-broker", running); got.Status != doctor.CheckStatusPass {
		t.Fatalf("running=%#v", got)
	}
	stopped := tobari.FinalClusterComponentObservation{Name: "credential-companion", State: tobari.FinalClusterRuntimeStopped, Health: "stopped", Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact}
	if got := finalAuthClusterComponentObservation("credential-companion", stopped); got.Status != doctor.CheckStatusWarn {
		t.Fatalf("stopped=%#v", got)
	}
}
