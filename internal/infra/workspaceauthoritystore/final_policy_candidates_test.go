package workspaceauthoritystore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalPolicyCandidateRuntimeFixture struct {
	live bool
	read tobari.DenialRead
}

func (f *finalPolicyCandidateRuntimeFixture) HasLiveFinalWorkspaceSession(context.Context) (bool, error) {
	return f.live, nil
}

func (f *finalPolicyCandidateRuntimeFixture) ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error) {
	return f.read, nil
}

func TestFinalPolicyCandidateAdapterProjectsExternalDenialWithoutAttachmentSession(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	collection := storeCollectionFixture(t)
	materializeCollection(t, root, collection)
	store, err := NewFinalOnly(root, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &finalPolicyCandidateRuntimeFixture{read: tobari.DenialRead{Items: []tobari.PolicyDenial{{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-25T12:00:00Z", RequestID: strings.Repeat("3", 32),
		WorkspaceManifestID: string(storeContextID), WorkspaceManifestName: collection.Templates[0].Name,
		ProjectID: string(storeWorkspaceID), ProjectRoot: collection.Workspaces[0].ProjectRoot,
		Host: "api.synthetic.example", Port: 443, Method: "GET", Path: "/brokered-default",
		Reason: "policy denied", StatusCode: 403, Learnable: true,
	}}}}
	adapter, err := NewFinalPolicyCandidateAdapter(store, runtime, &Mutator{}, &HostLoopbackPolicyAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ListPolicyCandidatesIncludingAttachments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range result.Items {
		if item.ObservingWorkspaceID == storeWorkspaceID && item.Effect.Host == "api.synthetic.example" && item.Effect.Path == "/brokered-default" {
			found = true
			if item.AttachmentAuthority != nil || item.Effect.Scheme != "https" || item.Effect.Protocol != tobari.PolicyProtocolHTTP {
				t.Fatalf("external candidate was projected as attachment or lost protocol identity: %#v", item)
			}
		}
	}
	if !found {
		t.Fatalf("external denial was not projected: %#v", result.Items)
	}
}
