package tobari

import "testing"

func TestContextAuthorityAxesKeepDesiredActiveAndAppliedAuthorityIndependent(t *testing.T) {
	snapshots, err := workspaceAuthorityCollectionFixture(t).ContextSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshots[0]
	axes, err := NewContextAuthorityAxes(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if axes.ActiveTemplatePolicySliceDigest == nil || axes.ActivePolicyMemoryRevision == nil || axes.AppliedEntry == nil {
		t.Fatalf("complete axes lost active or applied authority: %+v", axes)
	}

	driftedBody := snapshot.Template.Current.Body.Clone()
	driftedBody.Policy.AgentProfile = "reviewed-profile"
	driftedRevision, err := NewWorkspaceTemplateRevision(snapshot.Template.ID, snapshot.Template.Current.Generation+1, driftedBody)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Template.Current = driftedRevision
	snapshot.Template.Retained = append(snapshot.Template.Retained, driftedRevision.Clone())
	if err := snapshot.Validate(); err != nil {
		t.Fatal(err)
	}
	pending, err := NewContextAuthorityAxes(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if pending.DesiredTemplateRevision != driftedRevision.Revision || pending.DesiredTemplatePolicySliceDigest != driftedRevision.Slices.PolicySliceDigest || pending.ActiveTemplatePolicySliceDigest == nil || *pending.ActiveTemplatePolicySliceDigest == pending.DesiredTemplatePolicySliceDigest {
		t.Fatalf("desired B and active A collapsed: %+v", pending)
	}
	if pending.AppliedEntry == nil || pending.AppliedEntry.TemplateRevision == pending.DesiredTemplateRevision {
		t.Fatalf("AppliedEntry was inferred from desired authority: %+v", pending)
	}
}

func TestContextAuthorityAxesPreserveInactiveAxesAsAbsent(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	previous := collection.Clone()
	contexts := make([]WorkspaceAuthorityContextRecord, len(collection.Contexts))
	for index := range collection.Contexts {
		contexts[index] = collection.Contexts[index].Clone()
	}
	workspaces := append([]WorkspaceBinding{}, collection.Workspaces...)
	record := contexts[0].Clone()
	record.ActiveTemplatePolicy = nil
	record.ActivePolicyMemory = nil
	record.ActivePolicyMemoryRef = nil
	workspace := workspaces[0]
	workspace.LastSuccessfulEntry = nil
	contexts[0] = record
	workspaces[0] = workspace
	collection, _, err := PublishWorkspaceAuthorityCollection(collection.Templates, contexts, workspaces, collection.PendingCandidates, collection.DefaultTemplateID, &previous)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	axes, err := NewContextAuthorityAxes(snapshots[0])
	if err != nil {
		t.Fatal(err)
	}
	if axes.ActiveTemplatePolicySliceDigest != nil || axes.ActivePolicyMemoryRevision != nil || axes.AppliedEntry != nil {
		t.Fatalf("inactive authority was inferred: %+v", axes)
	}
}
