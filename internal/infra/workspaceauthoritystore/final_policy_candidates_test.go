package workspaceauthoritystore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalPolicyCandidateRuntimeFixture struct {
	live bool
	read tobari.DenialRead
}

func TestFinalPolicyCandidateAdapterReadsAfterMultipleContextPolicyAllows(t *testing.T) {
	collection := storeCollectionFixture(t)
	secondTemplateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	secondContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b2")
	secondWorkspaceID := tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789b3")
	secondTemplate, err := tobari.CopyWorkspaceTemplateRevision(secondTemplateID, "second", collection.Templates[0].Current)
	if err != nil {
		t.Fatal(err)
	}
	newRule := func(contextID tobari.ContextID, path string) tobari.PolicyMemoryRule {
		rule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryAllow, tobari.PolicyMemoryRuleBody{
			PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
			Match:                  tobari.PolicyMatchExact, Host: "api.synthetic.example", Port: 443, Method: "GET", Path: path,
			Segments: []string{}, Examples: []string{path}, SourceCandidates: []string{"pcy_" + strings.Repeat("2", 32)},
		})
		if err != nil {
			t.Fatal(err)
		}
		return rule
	}
	firstMemory, _, err := tobari.PublishPolicyMemory(storeContextID, []tobari.PolicyMemoryRule{newRule(storeContextID, "/brokered-default")}, &collection.Contexts[0].PolicyMemory)
	if err != nil {
		t.Fatal(err)
	}
	firstRecord := collection.Contexts[0].Clone()
	firstRecord.PolicyMemory = firstMemory
	firstActive := firstMemory.Clone()
	firstRecord.ActivePolicyMemory = &firstActive
	firstRecord.ActivePolicyMemoryRef = &tobari.PolicyMemoryActivationReceipt{ContextID: storeContextID, Revision: firstMemory.Revision}

	secondBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: secondContextID, TemplateID: secondTemplateID}
	secondMemory, _, err := tobari.PublishPolicyMemory(secondContextID, []tobari.PolicyMemoryRule{newRule(secondContextID, "/brokered-second")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondActive := secondMemory.Clone()
	secondRecord := tobari.WorkspaceAuthorityContextRecord{
		Context: secondBinding, PolicyMemory: secondMemory,
		ActiveTemplatePolicy: &tobari.TemplatePolicyActivationReceipt{ContextID: secondContextID, TemplateID: secondTemplateID, PolicySliceDigest: secondTemplate.Current.Slices.PolicySliceDigest},
		ActivePolicyMemory:   &secondActive, ActivePolicyMemoryRef: &tobari.PolicyMemoryActivationReceipt{ContextID: secondContextID, Revision: secondMemory.Revision},
	}
	secondWorkspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: secondWorkspaceID, ContextID: secondContextID,
		ProjectRoot: "/workspace/second", Home: "/workspace/home-second", CreationDefaults: secondTemplate.Current.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &tobari.WorkspaceAppliedEntry{
			ContextID: secondContextID, TemplateID: secondTemplateID, TemplateRevision: secondTemplate.Current.Revision,
			EntrySliceDigest: secondTemplate.Current.Slices.EntrySliceDigest, RuntimeID: secondTemplate.Current.Slices.RuntimeID,
			RuntimeRevision: secondTemplate.Current.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
		},
	}
	collection, _, err = tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{collection.Templates[0], secondTemplate},
		[]tobari.WorkspaceAuthorityContextRecord{firstRecord, secondRecord},
		[]tobari.WorkspaceBinding{collection.Workspaces[0], secondWorkspace}, []tobari.PolicyCandidateAuthority{}, collection.DefaultTemplateID, &collection,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := finalPolicyMemoryInputs(collection); err != nil {
		t.Fatalf("memory inputs: %v", err)
	}
	root := filepath.Join(t.TempDir(), "authority")
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
		Host: "mock-upstream.synthetic.example", Port: 8080, Method: "POST", Path: "/stream-upload", Reason: "policy denied", StatusCode: 403, Learnable: true,
	}}}}
	adapter, err := NewFinalPolicyCandidateAdapter(store, runtime, &Mutator{}, &HostLoopbackPolicyAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ListPolicyCandidatesIncludingAttachments(context.Background())
	if err != nil || len(result.Items) != 1 || result.Items[0].Effect.Path != "/stream-upload" {
		t.Fatalf("candidates=%#v err=%v", result, err)
	}
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

func TestFinalPermissionInboxJoinsLiveHostLoopbackWithoutPersistingIt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "authority")
	collection := storeCollectionFixture(t)
	materializeCollection(t, root, collection)
	store, err := NewFinalOnly(root, &legacyGuardFake{})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &finalPolicyCandidateRuntimeFixture{live: true, read: tobari.DenialRead{Items: []tobari.PolicyDenial{{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "http", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-25T12:00:00Z", RequestID: strings.Repeat("4", 32),
		WorkspaceManifestID: string(storeContextID), WorkspaceManifestName: collection.Templates[0].Name,
		ProjectID: string(storeWorkspaceID), ProjectRoot: collection.Workspaces[0].ProjectRoot,
		Host: tobari.HostLoopbackHostname, Port: 32123, Method: "GET", Path: "/health",
		Reason: "Host Loopback requires attachment policy review", StatusCode: 403, Learnable: true,
		DestinationKind: tobari.PolicyDestinationHostLoopback, AuthorityLifetime: tobari.AuthorityLifetimeAttachment,
		AttachmentEpochID: "att_" + strings.Repeat("5", 32),
	}}}}
	adapter, err := NewFinalPolicyCandidateAdapter(store, runtime, &Mutator{}, &HostLoopbackPolicyAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.ReadPolicyMemoryReviewSnapshot(context.Background())
	var attachment *tobari.PolicyMemoryReviewItem
	for index := range snapshot.Items {
		if snapshot.Items[index].AttachmentCandidate != nil {
			attachment = &snapshot.Items[index]
		}
	}
	if err != nil || attachment == nil || attachment.Rule.Host != tobari.HostLoopbackHostname {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
	if _, err := snapshot.ReviewedSet(map[string]tobari.PolicyMemoryDecision{attachment.ID: tobari.PolicyMemoryAllow}); err == nil {
		t.Fatal("attachment candidate entered persistent reviewed Policy Memory")
	}
	stored, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || stored.Revision != collection.Revision || len(stored.PendingCandidates) != len(collection.PendingCandidates) {
		t.Fatalf("read changed authority: present=%t stored=%#v err=%v", present, stored, err)
	}
}
