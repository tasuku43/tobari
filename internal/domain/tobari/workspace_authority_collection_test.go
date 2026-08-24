package tobari

import (
	"testing"
	"time"
)

func workspaceAuthorityCollectionFixture(t *testing.T) WorkspaceAuthorityCollection {
	t.Helper()
	revision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, templateBodyFixture("items"))
	if err != nil {
		t.Fatal(err)
	}
	template := WorkspaceTemplate{
		SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "restricted",
		Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()},
	}
	context := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion, ID: testContextAuthorityID,
		ProjectRoot: "/workspace/example", TemplateID: testTemplateAuthorityID,
	}
	memory, _, err := PublishPolicyMemory(testContextAuthorityID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{
		ContextID: context.ID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest,
	}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: context.ID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{
		Context: context, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt,
		ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &memoryReceipt,
	}
	applied := WorkspaceAppliedEntry{
		ContextID: context.ID, TemplateID: template.ID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("7"), ReconciledAt: time.Unix(1, 0).UTC(),
	}
	workspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: testWorkspaceAuthorityID, ContextID: context.ID,
		ProjectRoot: context.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	effect := PolicyCandidateEffect{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/candidate",
		Segments: []string{}, Examples: []string{"/candidate"},
	}
	candidate, err := NewPolicyCandidateAuthority(context.ID, workspace.ID, effect)
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		[]WorkspaceTemplate{template}, []WorkspaceAuthorityContextRecord{record}, []WorkspaceBinding{workspace},
		[]PolicyCandidateAuthority{candidate}, ptrTemplateID(template.ID), nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish collection: changed=%t err=%v", changed, err)
	}
	return collection
}

func collectionPolicyMemoryPtr(memory PolicyMemoryRevision) *PolicyMemoryRevision {
	clone := memory.Clone()
	return &clone
}

func TestWorkspaceAuthorityCollectionRequiresPendingCandidatesToBeUnconsumed(t *testing.T) {
	valid := workspaceAuthorityCollectionFixture(t)
	candidate := valid.PendingCandidates[0]
	rule, err := NewPolicyMemoryRule(candidate.ContextID, PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := PublishPolicyMemory(candidate.ContextID, []PolicyMemoryRule{rule}, &valid.Contexts[0].PolicyMemory)
	if err != nil {
		t.Fatal(err)
	}

	consumed := valid.Clone()
	consumed.Contexts[0].PolicyMemory = memory
	consumed.PendingCandidates = []PolicyCandidateAuthority{}
	consumed.Revision, _ = workspaceAuthorityCollectionRevision(consumed)
	if err := consumed.Validate(); err != nil {
		t.Fatalf("consumed candidate without pending record must remain valid: %v", err)
	}

	retained := consumed.Clone()
	retained.PendingCandidates = []PolicyCandidateAuthority{candidate}
	retained.Revision, _ = workspaceAuthorityCollectionRevision(retained)
	if err := retained.Validate(); err == nil {
		t.Fatal("consumed candidate remained actionable as pending")
	}
}

func ptrTemplateID(id WorkspaceTemplateID) *WorkspaceTemplateID {
	value := id
	return &value
}

func TestWorkspaceAuthorityCollectionPublishesOneCoherentEnvelope(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	if err := collection.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil || len(snapshots) != 1 || snapshots[0].Workspace == nil || snapshots[0].Workspace.ID != testWorkspaceAuthorityID {
		t.Fatalf("snapshots=%#v err=%v", snapshots, err)
	}
	noOp, changed, err := PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates,
		collection.DefaultTemplateID, &collection,
	)
	if err != nil || changed || noOp.Generation != collection.Generation || noOp.Revision != collection.Revision {
		t.Fatalf("no-op=%#v changed=%t err=%v", noOp, changed, err)
	}
	clone := collection.Clone()
	clone.Templates[0].Current.Body.Policy.BaselineGrants[0].Path = "/changed"
	clone.Contexts[0].PolicyMemory.Rules = append(clone.Contexts[0].PolicyMemory.Rules, PolicyMemoryRule{})
	clone.PendingCandidates[0].Effect.Examples[0] = "/changed"
	clone.Workspaces[0].Home = "/changed"
	if collection.Templates[0].Current.Body.Policy.BaselineGrants[0].Path == "/changed" || len(collection.Contexts[0].PolicyMemory.Rules) != 0 || collection.PendingCandidates[0].Effect.Examples[0] == "/changed" || collection.Workspaces[0].Home == "/changed" {
		t.Fatal("Workspace authority collection clone shares nested authority")
	}
}

func TestWorkspaceAuthorityCollectionRejectsPartialOrMixedAuthority(t *testing.T) {
	valid := workspaceAuthorityCollectionFixture(t)
	tests := map[string]func(*WorkspaceAuthorityCollection){
		"missing Template": func(value *WorkspaceAuthorityCollection) { value.Templates = []WorkspaceTemplate{} },
		"duplicate Workspace for Context": func(value *WorkspaceAuthorityCollection) {
			copy := value.Workspaces[0]
			copy.ID = "01912345-6789-7abc-8def-0123456789a4"
			value.Workspaces = append(value.Workspaces, copy)
		},
		"candidate crosses Workspace": func(value *WorkspaceAuthorityCollection) {
			value.PendingCandidates[0].ObservingWorkspaceID = "01912345-6789-7abc-8def-0123456789a4"
		},
		"active Policy Memory crosses Context": func(value *WorkspaceAuthorityCollection) {
			value.Contexts[0].ActivePolicyMemoryRef.ContextID = "01912345-6789-7abc-8def-0123456789a4"
		},
		"default Template missing": func(value *WorkspaceAuthorityCollection) {
			id := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a4")
			value.DefaultTemplateID = &id
		},
		"Workspace creation receipt missing": func(value *WorkspaceAuthorityCollection) {
			value.Workspaces[0].CreationDefaults = authorityDigest("4")
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid.Clone()
			mutate(&candidate)
			candidate.Revision, _ = workspaceAuthorityCollectionRevision(candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("partial or mixed Workspace authority passed")
			}
		})
	}
}
