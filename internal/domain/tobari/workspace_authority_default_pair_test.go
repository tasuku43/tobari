package tobari

import "testing"

func TestFinalDefaultPairObservationBindsDefaultTemplateAndCanonicalProject(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	observation, err := NewFinalDefaultPairObservation(collection, true, collection.Contexts[0].Context.ProjectRoot)
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

func TestFinalDefaultPairObservationRejectsCrossProjectContextRelabel(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	observation, err := NewFinalDefaultPairObservation(collection, true, collection.Contexts[0].Context.ProjectRoot)
	if err != nil {
		t.Fatal(err)
	}
	observation.Context.Context.ProjectRoot = "/workspace/other"
	if err := observation.Validate(); err == nil {
		t.Fatal("cross-Project Context relabel validated")
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
	root := currentCollection.Contexts[0].Context.ProjectRoot
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
