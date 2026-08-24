package tobari

import "testing"

func TestFinalRuntimeProtectionAuthorityBindsCompleteCollectionReceipt(t *testing.T) {
	collection := workspaceAuthorityCollectionFixture(t)
	authority, err := NewFinalRuntimeProtectionAuthority(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	if authority.CollectionGeneration != collection.Generation || authority.CollectionRevision != collection.Revision || len(authority.Templates) != 1 || len(authority.Contexts) != 1 || len(authority.Workspaces) != 1 {
		t.Fatalf("final protection projection = %+v", authority)
	}
	drifted := authority.Clone()
	drifted.CollectionRevision = authorityDigest("e")
	if err := drifted.Validate(); err == nil {
		t.Fatal("collection receipt drift retained a valid final protection digest")
	}
	drifted = authority.Clone()
	drifted.Workspaces[0].LastSuccessfulEntry = nil
	if err := drifted.Validate(); err == nil {
		t.Fatal("Workspace authority drift retained a valid final protection digest")
	}
}

func TestFinalRuntimeProtectionAuthorityRepresentsAbsentAndTemplateOnlyFinalState(t *testing.T) {
	absent, err := NewFinalRuntimeProtectionAuthority(WorkspaceAuthorityCollection{}, false)
	if err != nil || absent.Present || len(absent.Templates) != 0 || len(absent.Contexts) != 0 || len(absent.Workspaces) != 0 {
		t.Fatalf("absent final protection = %+v/%v", absent, err)
	}

	base := workspaceAuthorityCollectionFixture(t)
	templateOnly, _, err := PublishWorkspaceAuthorityCollection(base.Templates, []WorkspaceAuthorityContextRecord{}, []WorkspaceBinding{}, []PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := NewFinalRuntimeProtectionAuthority(templateOnly, true)
	if err != nil || !authority.Present || len(authority.Templates) != 1 || len(authority.Contexts) != 0 || len(authority.Workspaces) != 0 {
		t.Fatalf("Template-only final protection = %+v/%v", authority, err)
	}
}
