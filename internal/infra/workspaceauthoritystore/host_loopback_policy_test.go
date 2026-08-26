package workspaceauthoritystore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type hostLoopbackPolicyRuntimeFixture struct {
	read    tobari.DenialRead
	applied []tobari.AttachmentGrant
}

func (f *hostLoopbackPolicyRuntimeFixture) HasLiveFinalWorkspaceSession(context.Context) (bool, error) {
	return true, nil
}

func (f *hostLoopbackPolicyRuntimeFixture) ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error) {
	return f.read, nil
}

func (f *hostLoopbackPolicyRuntimeFixture) ApplyAttachmentGrantDecisionSet(_ context.Context, grants []tobari.AttachmentGrant) (tobari.PolicyActivationReceipt, error) {
	f.applied = append([]tobari.AttachmentGrant{}, grants...)
	return tobari.PolicyActivationReceipt{ActiveRevision: strings.Repeat("a", 64)}, nil
}

func TestHostLoopbackPolicyAdapterKeepsAttachmentCandidateOutsidePolicyMemory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	collection := storeCollectionFixture(t)
	materializeCollection(t, root, collection)
	store, err := NewFinalOnly(root, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "http", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-25T12:00:00Z", RequestID: strings.Repeat("1", 32),
		WorkspaceManifestID: string(storeContextID), WorkspaceManifestName: collection.Templates[0].Name,
		ProjectID: string(storeWorkspaceID), ProjectRoot: collection.Workspaces[0].ProjectRoot,
		Host: tobari.HostLoopbackHostname, Port: 3000, Method: "GET", Path: "/health",
		Reason: "policy review required", StatusCode: 403, Learnable: true,
		DestinationKind: tobari.PolicyDestinationHostLoopback, AuthorityLifetime: tobari.AuthorityLifetimeAttachment,
		AttachmentEpochID: "att_" + strings.Repeat("2", 32),
	}
	runtime := &hostLoopbackPolicyRuntimeFixture{read: tobari.DenialRead{Items: []tobari.PolicyDenial{denial}}}
	adapter, err := NewHostLoopbackPolicyAdapter(store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	list, err := adapter.ListPolicyCandidatesIncludingAttachments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var attachment *tobari.PolicyCandidateAuthorityView
	for index := range list.Items {
		if list.Items[index].AttachmentAuthority != nil {
			attachment = &list.Items[index]
		}
	}
	if attachment == nil || attachment.Effect.Host != tobari.HostLoopbackHostname || attachment.ContextID != storeContextID || attachment.ObservingWorkspaceID != storeWorkspaceID {
		t.Fatalf("attachment candidate=%#v", attachment)
	}
	publication, handled, err := adapter.ApplyAttachmentPolicyCandidate(context.Background(), attachment.ID, tobari.PolicyMemoryAllow)
	if err != nil || !handled || len(runtime.applied) != 1 || runtime.applied[0].SourceCandidate != attachment.ID {
		t.Fatalf("publication=%#v handled=%t grants=%#v err=%v", publication, handled, runtime.applied, err)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || after.Revision != collection.Revision || len(after.Contexts[0].PolicyMemory.Rules) != 0 {
		t.Fatalf("attachment action changed final authority: present=%t after=%#v err=%v", present, after, err)
	}
	if _, handled, err := adapter.ApplyAttachmentPolicyCandidate(context.Background(), "pcy_"+strings.Repeat("f", 32), tobari.PolicyMemoryAllow); err != nil || handled {
		t.Fatalf("stale candidate handled=%t err=%v", handled, err)
	}
}
