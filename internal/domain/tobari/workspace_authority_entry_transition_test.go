package tobari

import (
	"testing"
	"time"
)

func TestPublishWorkspaceEntryAuthorityTransitionTable(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	tests := []struct {
		name        string
		previous    WorkspaceAuthorityCollection
		newApplied  bool
		wantChanged bool
	}{
		{name: "exact active re-entry is a semantic no-op", previous: base, wantChanged: false},
		{name: "inactive axes become current", previous: entryTransitionWithoutActiveAxes(t, base), wantChanged: true},
		{name: "stale A active becomes desired B active and applied", previous: entryTransitionWithDesiredRevisionB(t, base), newApplied: true, wantChanged: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshots, err := test.previous.ContextSnapshots()
			if err != nil {
				t.Fatal(err)
			}
			plan := entryTransitionPlan(t, snapshots[0], test.newApplied)
			next, changed, err := PublishWorkspaceEntryAuthority(test.previous, plan)
			if err != nil {
				t.Fatal(err)
			}
			if changed != test.wantChanged {
				t.Fatalf("changed=%t want=%t", changed, test.wantChanged)
			}
			nextSnapshots, err := next.ContextSnapshots()
			if err != nil {
				t.Fatal(err)
			}
			current := nextSnapshots[0]
			if current.ActiveTemplatePolicy == nil || current.ActiveTemplatePolicy.PolicySliceDigest != current.Template.Current.Slices.PolicySliceDigest || current.ActivePolicyMemory == nil || current.ActivePolicyMemoryRef == nil || current.ActivePolicyMemory.Revision != current.PolicyMemory.Revision || current.ActivePolicyMemoryRef.Revision != current.PolicyMemory.Revision {
				t.Fatalf("entry did not publish exact current independent axes: %+v", current)
			}
			if current.Workspace == nil || current.Workspace.LastSuccessfulEntry == nil || *current.Workspace.LastSuccessfulEntry != plan.Applied {
				t.Fatalf("entry did not publish exact AppliedEntry: %+v", current.Workspace)
			}
		})
	}
}

func entryTransitionWithoutActiveAxes(t *testing.T, base WorkspaceAuthorityCollection) WorkspaceAuthorityCollection {
	t.Helper()
	contexts := make([]WorkspaceAuthorityContextRecord, len(base.Contexts))
	for index := range base.Contexts {
		contexts[index] = base.Contexts[index].Clone()
	}
	contexts[0].ActiveTemplatePolicy = nil
	contexts[0].ActivePolicyMemory = nil
	contexts[0].ActivePolicyMemoryRef = nil
	result, changed, err := PublishWorkspaceAuthorityCollection(base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("publish inactive fixture: changed=%t err=%v", changed, err)
	}
	return result
}

func entryTransitionWithDesiredRevisionB(t *testing.T, base WorkspaceAuthorityCollection) WorkspaceAuthorityCollection {
	t.Helper()
	templates := make([]WorkspaceTemplate, len(base.Templates))
	for index := range base.Templates {
		templates[index] = base.Templates[index].Clone()
	}
	body := templates[0].Current.Body.Clone()
	body.Policy.AgentProfile = "reviewed-profile"
	revision, err := NewWorkspaceTemplateRevision(templates[0].ID, templates[0].Current.Generation+1, body)
	if err != nil {
		t.Fatal(err)
	}
	templates[0].Current = revision
	templates[0].Retained = append(templates[0].Retained, revision.Clone())
	result, changed, err := PublishWorkspaceAuthorityCollection(templates, base.Contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("publish desired-B fixture: changed=%t err=%v", changed, err)
	}
	return result
}

func entryTransitionPlan(t *testing.T, snapshot ContextAuthoritySnapshot, newApplied bool) WorkspaceEntryReconciliationPlan {
	t.Helper()
	if snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil {
		t.Fatal("entry transition fixture requires an existing applied Workspace")
	}
	workspace := *snapshot.Workspace
	entry := *snapshot.Workspace.LastSuccessfulEntry
	if newApplied {
		entry = WorkspaceAppliedEntry{
			ContextID: snapshot.Context.ID, TemplateID: snapshot.Template.ID,
			TemplateRevision: snapshot.Template.Current.Revision, EntrySliceDigest: snapshot.Template.Current.Slices.EntrySliceDigest,
			RuntimeID: snapshot.Template.Current.Slices.RuntimeID, RuntimeRevision: snapshot.Template.Current.Slices.RuntimeRevision,
			ResolvedSpec: authorityDigest("9"), ReconciledAt: time.Unix(9, 0).UTC(),
		}
		workspace.LastSuccessfulEntry = &entry
	}
	authority, err := DeriveWorkspaceTemplateEntryAuthority(snapshot.Template.Current)
	if err != nil {
		t.Fatal(err)
	}
	creation := authority.CreationDefaults.Clone()
	for _, revision := range snapshot.Template.Retained {
		if revision.Slices.CreationDefaultsDigest == workspace.CreationDefaults {
			creation = revision.Body.CreationDefaults.Clone()
		}
	}
	_, network, err := ProjectResourceNames(string(workspace.ID))
	if err != nil {
		t.Fatal(err)
	}
	return WorkspaceEntryReconciliationPlan{
		Workspace: workspace, Applied: entry, Authority: authority, CreationDefaults: creation,
		Network: WorkspaceRuntimeNetworkAuthority{Network: network, Subnet: "10.64.0.0/24", DockerGateway: "10.64.0.1", GatewayIP: "10.64.0.2", WorkspaceIP: "10.64.0.3"},
	}
}
