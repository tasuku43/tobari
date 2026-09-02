package tobari

import "testing"

func TestFinalDefaultPairObservationBindsDefaultTemplateAndCanonicalProject(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	observation, err := NewFinalDefaultPairObservation(collection, true, collection.Workspaces[0].ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	if observation.DefaultTemplate == nil || observation.DefaultTemplate.ID != *collection.DefaultTemplateID || observation.Context == nil || observation.Context.Context.ID != collection.Contexts[0].Context.ID || !observation.SameReceipt(observation.Clone()) {
		t.Fatalf("unexpected default-pair observation: %+v", observation)
	}
	clone := observation.Clone()
	clone.DefaultTemplate.Name = "changed"
	if observation.DefaultTemplate.Name == clone.DefaultTemplate.Name {
		t.Fatal("default-pair observation clone aliases Template authority")
	}
}

func TestFinalDefaultPairObservationRepresentsExactFreshEmptyAuthority(t *testing.T) {
	observation, err := NewFinalDefaultPairObservation(WorkspaceAuthorityCollection{}, false, "/workspace/fresh")
	if err != nil {
		t.Fatal(err)
	}
	if observation.CollectionPresent || observation.DefaultTemplate != nil || observation.Context != nil || observation.CollectionGeneration != 0 || observation.CollectionRevision != "" {
		t.Fatalf("fresh final authority is not exact empty: %+v", observation)
	}
}

func TestFinalDefaultPairObservationRejectsCrossProjectWorkspaceRelabel(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	observation, err := NewFinalDefaultPairObservation(collection, true, collection.Workspaces[0].ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	observation.Context.Workspace.ProjectRoot = "/workspace/other"
	if err := observation.Validate(); err == nil {
		t.Fatal("cross-Project Context relabel validated")
	}
}

func TestFinalDefaultPairObservationUsesUniqueCWDWorkspaceRegardlessOfDefaultTemplate(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	otherID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789d3")
	otherTemplate := collection.Templates[0].Clone()
	otherTemplate.ID = otherID
	otherTemplate.Name = "other"
	for index := range otherTemplate.Retained {
		otherTemplate.Retained[index].TemplateID = otherID
	}
	otherTemplate.Current.TemplateID = otherID
	otherTemplate.Current.Revision = otherTemplate.Retained[0].Revision
	contextRecord := collection.Contexts[0].Clone()
	contextRecord.Context.TemplateID = otherID
	contextRecord.ActiveTemplatePolicy = nil
	contextRecord.ActivePolicyMemory = nil
	contextRecord.ActivePolicyMemoryRef = nil
	workspace := collection.Workspaces[0]
	workspace.LastSuccessfulEntry = nil
	collection, _, err := PublishWorkspaceAuthorityCollection(
		[]WorkspaceTemplate{collection.Templates[0], otherTemplate}, []WorkspaceAuthorityContextRecord{contextRecord},
		[]WorkspaceBinding{workspace}, collection.PendingCandidates, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := NewFinalDefaultPairObservation(collection, true, workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Context == nil || observation.Context.Context.TemplateID != otherID || observation.DefaultTemplate == nil || observation.DefaultTemplate.ID != otherID {
		t.Fatalf("CWD Workspace did not own Context selection: %+v", observation)
	}
	selection, err := NewFinalDefaultPairSelection(collection, true, workspace.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	choice, automatic := selection.AutomaticChoice()
	if !automatic || choice.Kind != FinalDefaultPairSelectionUse || choice.ContextID != contextRecord.Context.ID {
		t.Fatalf("root selection did not reuse the unique nondefault Workspace: selection=%+v choice=%+v automatic=%t", selection, choice, automatic)
	}
}

func TestFinalDefaultPairSelectionRequiresExplicitAncestorChoiceNearestFirst(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	nestedID := ContextID("01912345-6789-7abc-8def-0123456789d1")
	memory, _, err := PublishPolicyMemory(nestedID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	nested := WorkspaceAuthorityContextRecord{Context: ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion, ID: nestedID,
		TemplateID: *collection.DefaultTemplateID,
	}, PolicyMemory: memory}
	nestedWorkspace := collection.Workspaces[0]
	nestedWorkspace.ID = WorkspaceID("01912345-6789-7abc-8def-0123456789d2")
	nestedWorkspace.ContextID = nestedID
	nestedWorkspace.ProjectRoot = "/workspace/example/src"
	nestedWorkspace.Home = "/workspace/home-nested"
	nestedWorkspace.LastSuccessfulEntry = nil
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		collection.Templates, append(collection.Contexts, nested), append(collection.Workspaces, nestedWorkspace),
		collection.PendingCandidates, collection.DefaultTemplateID, &collection,
	)
	if err != nil || !changed {
		t.Fatalf("publish nested Context: changed=%t err=%v", changed, err)
	}
	selection, err := NewFinalDefaultPairSelection(collection, true, "/workspace/example/src/pkg")
	if err != nil {
		t.Fatal(err)
	}
	if !selection.RequiresChoice() || len(selection.Candidates) != 2 {
		t.Fatalf("selection = %+v, want two explicit ancestor candidates", selection)
	}
	if got := selection.Candidates[0].Snapshot.Workspace.ProjectRoot; got != "/workspace/example/src" {
		t.Fatalf("nearest candidate = %q", got)
	}
	if _, automatic := selection.AutomaticChoice(); automatic {
		t.Fatal("ancestor-only selection chose implicitly")
	}
	use := FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionUse, ContextID: nestedID}
	observation, err := selection.Observation(use)
	if err != nil {
		t.Fatal(err)
	}
	if observation.ProjectRoot != "/workspace/example/src" || observation.Context == nil || observation.Context.Context.ID != nestedID {
		t.Fatalf("selected observation = %+v", observation)
	}
	create := FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionCreate}
	observation, err = selection.Observation(create)
	if err != nil {
		t.Fatal(err)
	}
	if observation.ProjectRoot != selection.CanonicalCWD || observation.Context != nil {
		t.Fatalf("create observation = %+v", observation)
	}
}

func TestFinalDefaultPairSelectionReusesExactRootWithoutChoice(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	selection, err := NewFinalDefaultPairSelection(collection, true, collection.Workspaces[0].ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	choice, automatic := selection.AutomaticChoice()
	if !automatic || choice.Kind != FinalDefaultPairSelectionUse || choice.ContextID != collection.Contexts[0].Context.ID || selection.RequiresChoice() {
		t.Fatalf("automatic exact-root choice = %+v, %t", choice, automatic)
	}
	if err := selection.ValidateChoice(FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionCreate}); err == nil {
		t.Fatal("exact-root selection accepted nested creation over an existing default Context")
	}
}

func TestFinalDefaultPairSelectionDoesNotInferProjectFromUnboundContext(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	withoutWorkspace, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, []WorkspaceBinding{}, []PolicyCandidateAuthority{},
		base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish location-free Context fixture: changed=%t err=%v", changed, err)
	}
	selection, err := NewFinalDefaultPairSelection(withoutWorkspace, true, "/workspace/new-project")
	if err != nil {
		t.Fatal(err)
	}
	choice, automatic := selection.AutomaticChoice()
	if len(selection.Candidates) != 0 || !automatic || choice.Kind != FinalDefaultPairSelectionCreate || choice.ContextID != "" {
		t.Fatalf("unbound Context inferred from Project: selection=%+v choice=%+v automatic=%t", selection, choice, automatic)
	}
}

func TestValidateRootContainsRejectsSiblingPrefix(t *testing.T) {
	if err := ValidateRootContains("/workspace/project", "/workspace/project/src"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRootContains("/workspace/project", "/workspace/project-other"); err == nil {
		t.Fatal("textual sibling prefix was accepted as a descendant")
	}
}

func TestFinalDefaultPairPublicationRejectsLowerAuthorityWheneverItCreatesContext(t *testing.T) {
	currentCollection := workspaceAuthorityCollectionFixture(t)
	previousCollection, changed, err := PublishWorkspaceAuthorityCollection(
		currentCollection.Templates, []WorkspaceAuthorityContextRecord{}, []WorkspaceBinding{}, []PolicyCandidateAuthority{},
		currentCollection.DefaultTemplateID, nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish initialized default without Context: changed=%t err=%v", changed, err)
	}
	root := currentCollection.Workspaces[0].ProjectRoot
	previous, err := NewFinalDefaultPairObservation(previousCollection, true, root)
	if err != nil {
		t.Fatal(err)
	}
	current, err := NewFinalDefaultPairObservation(currentCollection, true, root)
	if err != nil {
		t.Fatal(err)
	}
	publication := FinalDefaultPairPublication{Previous: previous, Current: current, Changed: true}
	if err := publication.ValidateFor(root, currentCollection.Templates[0].Current.Body); err == nil {
		t.Fatal("default-pair Context creation accepted pre-populated active/Workspace authority")
	}
}
