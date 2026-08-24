package workspaceauthoritystore

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestFreshDefaultPairEntrySettlesInactiveAxesAndPublishesAppliedEntry(t *testing.T) {
	store, mutator, lifecycle, _, memory := newMutationFixture(t, nil)
	body := storeCollectionFixture(t).Templates[0].Current.Body
	initialized, err := mutator.InitializeFinalDefaultPair(context.Background(), "/workspace/fresh-entry", body)
	if err != nil {
		t.Fatal(err)
	}
	if !initialized.Changed || initialized.Current.Context == nil || initialized.Current.Context.Workspace != nil ||
		initialized.Current.Context.ActiveTemplatePolicy != nil || initialized.Current.Context.ActivePolicyMemory != nil ||
		initialized.Current.Context.ActivePolicyMemoryRef != nil {
		t.Fatalf("fresh default pair did not stop at exact inactive Context authority: %#v", initialized)
	}

	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read fresh pair: present=%t err=%v", present, err)
	}
	runtime := &entryRuntimeFixture{homes: map[tobari.WorkspaceID]string{}}
	templatePolicy := &templateActivationFixture{}
	sessions := &entrySessionFixture{lifecycle: lifecycle, outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0}}
	baseSettlement := mutator.settlement.(*finalSettlementFixture)
	mutator.settlement = &clusterSettlementFixture{finalSettlementFixture: baseSettlement}
	adapter, err := NewContextEntryAdapter(mutator, runtime, templatePolicy, sessions, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	contextRef, err := tobari.ContextRef(current.Contexts[0].Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if publication.Snapshot.ActiveTemplatePolicy == nil || publication.Snapshot.ActivePolicyMemory == nil ||
		publication.Snapshot.ActivePolicyMemoryRef == nil || publication.Snapshot.Workspace == nil ||
		publication.Snapshot.Workspace.LastSuccessfulEntry == nil {
		t.Fatalf("first entry did not publish active axes and AppliedEntry: %#v", publication.Snapshot)
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read first entry: present=%t err=%v", present, err)
	}
	snapshot, err := snapshotForContext(after, publication.Snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ActiveTemplatePolicy == nil || snapshot.ActiveTemplatePolicy.PolicySliceDigest != snapshot.Template.Current.Slices.PolicySliceDigest ||
		snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemory.Revision != snapshot.PolicyMemory.Revision ||
		snapshot.ActivePolicyMemoryRef == nil || snapshot.ActivePolicyMemoryRef.Revision != snapshot.PolicyMemory.Revision ||
		snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil {
		t.Fatalf("persisted first-entry transition is incomplete: %#v", snapshot)
	}
	if runtime.reconcileCalls != 1 || sessions.run != 1 || templatePolicy.calls == 0 || memory.confirmCalls == 0 {
		t.Fatalf("first-entry calls runtime=%d session=%d template=%d memory=%d", runtime.reconcileCalls, sessions.run, templatePolicy.calls, memory.confirmCalls)
	}
}

func TestContextEntryAdvancesStaleAxesToCurrentTemplateWithoutReplacingWorkspace(t *testing.T) {
	previous := storeCollectionFixture(t)
	originalWorkspace := previous.Workspaces[0]
	originalTemplateReceipt := *previous.Contexts[0].ActiveTemplatePolicy
	originalMemoryReceipt := *previous.Contexts[0].ActivePolicyMemoryRef

	// Publish a valid current Template B while retaining the exact active
	// Template-policy receipt selected from A. This is the supported stale-axis
	// state entry must settle before applying B's entry slice.
	template := previous.Templates[0].Clone()
	body := template.Current.Body.Clone()
	body.Policy.NativeReadiness = tobari.ManifestNativeReadinessDisabled
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, template.Current.Generation+1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = append(template.Retained, revision.Clone())
	stale, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, previous.Contexts, previous.Workspaces,
		previous.PendingCandidates, previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("publish stale A-active/B-desired authority: changed=%t err=%v", changed, err)
	}
	store, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, stale)
	stale, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read stale axes: present=%t err=%v", present, err)
	}
	if stale.Contexts[0].ActiveTemplatePolicy == nil || *stale.Contexts[0].ActiveTemplatePolicy != originalTemplateReceipt ||
		stale.Contexts[0].ActivePolicyMemoryRef == nil || *stale.Contexts[0].ActivePolicyMemoryRef != originalMemoryReceipt {
		t.Fatalf("Template update unexpectedly activated Context axes: %#v", stale.Contexts[0])
	}

	contextRef, _ := tobari.ContextRef(storeContextID)
	entry, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Snapshot.ActiveTemplatePolicy == nil || entry.Snapshot.ActiveTemplatePolicy.PolicySliceDigest != entry.Snapshot.Template.Current.Slices.PolicySliceDigest ||
		entry.Snapshot.Workspace == nil || entry.Snapshot.Workspace.ID != originalWorkspace.ID ||
		entry.Snapshot.Workspace.Home != originalWorkspace.Home || entry.Snapshot.Workspace.CreationDefaults != originalWorkspace.CreationDefaults ||
		entry.Snapshot.Workspace.LastSuccessfulEntry == nil || entry.Snapshot.Workspace.LastSuccessfulEntry.TemplateRevision != entry.Snapshot.Template.Current.Revision {
		t.Fatalf("stale A to desired B entry changed create-once Workspace authority: %#v", entry.Snapshot)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 1 || sessions.run != 1 {
		t.Fatalf("stale-axis entry calls plan=%d reconcile=%d session=%d", runtime.planCalls, runtime.reconcileCalls, sessions.run)
	}
}

func TestContextEntryExactActiveReentryPreservesEnvelopeAndCreateOnceWorkspace(t *testing.T) {
	previous := storeCollectionFixture(t)
	store, _, adapter, _, runtime, _, _, sessions := newEntryFixture(t, previous)
	runtime.reuseApplied = true
	contextRef, _ := tobari.ContextRef(storeContextID)

	for attempt := 0; attempt < 2; attempt++ {
		publication, err := adapter.EnterContextByReference(context.Background(), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
		if err != nil {
			t.Fatalf("re-entry %d: %v", attempt+1, err)
		}
		if publication.Snapshot.Workspace == nil || publication.Snapshot.Workspace.ID != previous.Workspaces[0].ID ||
			publication.Snapshot.Workspace.Home != previous.Workspaces[0].Home ||
			publication.Snapshot.Workspace.CreationDefaults != previous.Workspaces[0].CreationDefaults {
			t.Fatalf("re-entry %d replaced create-once Workspace: %#v", attempt+1, publication.Snapshot.Workspace)
		}
	}
	after, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || !reflect.DeepEqual(after, previous) {
		t.Fatalf("exact re-entry changed envelope: present=%t err=%v\nwant=%#v\ngot=%#v", present, err, previous, after)
	}
	if runtime.planCalls != 1 || runtime.reconcileCalls != 1 || runtime.confirmCalls != 2 || sessions.run != 2 {
		t.Fatalf("exact re-entry did not use terminal confirmation: plan=%d reconcile=%d confirm=%d session=%d", runtime.planCalls, runtime.reconcileCalls, runtime.confirmCalls, sessions.run)
	}
}
