//go:build tobari_dev && tobari_research

package workspaceauthoritystore

import (
	"context"
	"fmt"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalAuthDoctorBrokerFixture struct {
	*finalAuthBrokerFixture
	wantContexts  int
	wantProviders int
	calls         int
}

func (b *finalAuthDoctorBrokerFixture) ObserveFinalAuthDoctorRuntimeCheck(_ context.Context, _ tobari.WorkspaceAuthorityCollection, present bool, inventories []authbroker.ContextStatusObservation, _ doctor.CheckID) (doctor.Observation, error) {
	b.calls++
	if len(inventories) != b.wantContexts {
		return doctor.Observation{}, fmt.Errorf("inventories=%d want=%d", len(inventories), b.wantContexts)
	}
	for _, inventory := range inventories {
		if !inventory.InventoryComplete || len(inventory.Providers) != b.wantProviders {
			return doctor.Observation{}, fmt.Errorf("incomplete inventory")
		}
	}
	if present != (b.wantContexts != 0) {
		return doctor.Observation{}, fmt.Errorf("presence mismatch")
	}
	return doctor.Observation{Status: doctor.CheckStatusPass, Detail: "exact final auth fixture"}, nil
}

func finalAuthDoctorCollection(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	base := storeCollectionFixture(t)
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789a4")
	workspaceID := tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789a5")
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := base.Templates[0].Current
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: base.Templates[0].ID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	record := tobari.WorkspaceAuthorityContextRecord{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: base.Templates[0].ID}, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt}
	workspace := base.Workspaces[0]
	if workspace.LastSuccessfulEntry != nil {
		entry := *workspace.LastSuccessfulEntry
		workspace.LastSuccessfulEntry = &entry
	}
	workspace.ID, workspace.ContextID, workspace.ProjectRoot, workspace.Home = workspaceID, contextID, "/workspace/second", "/workspace/home-second"
	entry := *workspace.LastSuccessfulEntry
	entry.ContextID = contextID
	workspace.LastSuccessfulEntry = &entry
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(base.Templates, append(base.Contexts, record), append(base.Workspaces, workspace), []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestFinalAuthDoctorAdapterCleanEmptyAndMultiContextProviderInventories(t *testing.T) {
	for _, test := range []struct {
		name       string
		collection *tobari.WorkspaceAuthorityCollection
		contexts   int
		providers  []string
	}{
		{name: "clean empty", collection: nil},
		{name: "multi Context and provider", collection: func() *tobari.WorkspaceAuthorityCollection { value := finalAuthDoctorCollection(t); return &value }(), contexts: 2, providers: []string{"alpha", "beta"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, mutator, _, _, _ := newMutationFixture(t, test.collection)
			base := newFinalAuthBrokerFixture()
			for _, provider := range test.providers {
				for _, record := range func() []tobari.WorkspaceAuthorityContextRecord {
					if test.collection == nil {
						return nil
					}
					return test.collection.Contexts
				}() {
					base.statuses[finalAuthKey(record.Context.ID, provider)] = authbroker.ProviderStatus{Provider: provider, State: authbroker.ProviderCredentialNotConfigured}
				}
			}
			broker := &finalAuthDoctorBrokerFixture{finalAuthBrokerFixture: base, wantContexts: test.contexts, wantProviders: len(test.providers)}
			adapter, err := NewFinalContextAuthAdapter(mutator, broker, context.Background())
			if err != nil {
				t.Fatal(err)
			}
			selected, _, err := store.ReadComplete(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			got, err := adapter.ObserveFinalAuthDoctorCheck(context.Background(), selected, doctor.CheckIDAuthVaultIntegrity)
			if err != nil || got.Status != doctor.CheckStatusPass || broker.calls != 1 {
				t.Fatalf("observation=%#v calls=%d err=%v", got, broker.calls, err)
			}
		})
	}
}
