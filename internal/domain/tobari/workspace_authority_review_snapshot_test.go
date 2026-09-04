package tobari

import "testing"

func TestBatchCB3ReviewSnapshotBuildsTemplateAndOneCanonicalMultiContextSet(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	first := base.PendingCandidates[0]
	secondEffect := first.Effect.Clone()
	secondEffect.Path = "/v1/projects/second"
	secondEffect.Examples = []string{secondEffect.Path}
	firstEffect := first.Effect.Clone()
	firstEffect.Path = "/v1/projects/first"
	firstEffect.Examples = []string{firstEffect.Path}
	first, err := NewPolicyCandidateAuthority(first.ContextID, first.ObservingWorkspaceID, firstEffect)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPolicyCandidateAuthority(first.ContextID, first.ObservingWorkspaceID, secondEffect)
	if err != nil {
		t.Fatal(err)
	}
	withPair, changed, err := PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, base.Workspaces, []PolicyCandidateAuthority{first, second}, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("pair changed=%t err=%v", changed, err)
	}
	multi, other := reviewedSecondContextFixture(t, withPair, "/other")
	snapshot, err := NewPolicyMemoryReviewSnapshot(multi, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 2 {
		t.Fatalf("items=%#v", snapshot.Items)
	}
	choices := map[string]PolicyMemoryDecision{}
	for _, item := range snapshot.Items {
		if item.Match == PolicyMatchPathTemplate {
			choices[item.ID] = PolicyMemoryAllow
		}
		if item.ID == other.ID {
			choices[item.ID] = PolicyMemoryDeny
		}
	}
	set, err := snapshot.ReviewedSet(choices)
	if err != nil || len(set.Decisions) != 2 || set.ObservedGeneration != multi.Generation || set.ObservedRevision != multi.Revision {
		t.Fatalf("set=%#v err=%v", set, err)
	}
	if set.Decisions[0].ReviewItemID > set.Decisions[1].ReviewItemID {
		t.Fatalf("set order=%#v", set.Decisions)
	}
}

func TestBatchCB3ReviewSnapshotRejectsAChoiceFromAnotherReceipt(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	snapshot, err := NewPolicyMemoryReviewSnapshot(base, true)
	if err != nil {
		t.Fatal(err)
	}
	choice := PolicyMemoryReviewChoiceSet{ObservedGeneration: snapshot.CollectionGeneration, ObservedRevision: snapshot.CollectionRevision, Decisions: []PolicyMemoryReviewChoice{{ReviewItemID: snapshot.Items[0].ID, Decision: PolicyMemoryAllow}}}
	choice.ObservedGeneration++
	if _, err := snapshot.ReviewedChoiceSet(choice); err == nil {
		t.Fatal("snapshot accepted a choice from another collection receipt")
	}
}

func TestContextWideTemplateReviewOmitsSingularWorkspaceProvenance(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	sibling := base.Workspaces[0]
	sibling.ID = WorkspaceID("01912345-6789-7abc-8def-0123456789a4")
	sibling.ProjectRoot = "/workspace/sibling"
	sibling.LastSuccessfulEntry = nil
	firstEffect := base.PendingCandidates[0].Effect.Clone()
	firstEffect.Path, firstEffect.Examples = "/teams/a", []string{"/teams/a"}
	secondEffect := firstEffect.Clone()
	secondEffect.Path, secondEffect.Examples = "/teams/b", []string{"/teams/b"}
	first, err := NewPolicyCandidateAuthority(base.Contexts[0].Context.ID, base.Workspaces[0].ID, firstEffect)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPolicyCandidateAuthority(base.Contexts[0].Context.ID, sibling.ID, secondEffect)
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, append(cloneWorkspaceBindings(base.Workspaces), sibling),
		[]PolicyCandidateAuthority{first, second}, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish sibling review evidence: changed=%t err=%v", changed, err)
	}
	snapshot, err := NewPolicyMemoryReviewSnapshot(collection, true)
	if err != nil || len(snapshot.Items) != 1 {
		t.Fatalf("context-wide review: items=%#v err=%v", snapshot.Items, err)
	}
	item := snapshot.Items[0]
	if item.Match != PolicyMatchPathTemplate || item.ProjectRoot != "" || item.ObservingWorkspace != "" || item.ObservingWorkspaceID != "" {
		t.Fatalf("Context-wide proposal claimed singular Workspace provenance: %#v", item)
	}
	set, err := snapshot.ReviewedSet(map[string]PolicyMemoryDecision{item.ID: PolicyMemoryAllow})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, collection, set)
	if publication.AppliedDecisions[0].ObservingWorkspaceRef != "" || publication.AppliedDecisions[0].ObservingWorkspaceID != "" {
		t.Fatalf("Context-wide applied result claimed singular Workspace provenance: %#v", publication.AppliedDecisions[0])
	}
}
