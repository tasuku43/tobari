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
