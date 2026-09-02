package tobari

import (
	"strings"
	"testing"
)

func TestTemplateChangePlanTreatsExactNullBaseSourceAsGenerationOneDraft(t *testing.T) {
	activeBody := templateBodyFixture("/active")
	activeBody.Boundary.DestinationCeiling = ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}}
	activeBody.Boundary.MethodPolicy = ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: []ManifestMethodOverride{}}
	activeRevision, err := NewWorkspaceTemplateRevision(testTemplateAuthorityID, 1, activeBody)
	if err != nil {
		t.Fatal(err)
	}
	active := WorkspaceTemplate{SchemaVersion: WorkspaceTemplateSchemaVersion, ID: testTemplateAuthorityID, Name: "active", Current: activeRevision, Retained: []WorkspaceTemplateRevision{activeRevision.Clone()}}
	collection, _, err := PublishWorkspaceAuthorityCollection([]WorkspaceTemplate{active}, []WorkspaceAuthorityContextRecord{}, []WorkspaceBinding{}, []PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	draftID := WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789f7")
	source, err := NewWorkspaceTemplateDraftSource(draftID, "later", activeBody)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := NewWorkspaceTemplateChangePlan(collection, draftID, source, activeBody.EntryDefaults.Runtime, map[WorkspaceID]bool{}, strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	if plan.ActiveRevision != nil || plan.ActiveMetadataRevision != nil || plan.BaseRevision != nil || plan.Impact != WorkspaceTemplateChangeMixed || plan.AffectedContextCount != 0 || plan.RunningWorkspaceCount != 0 {
		t.Fatalf("generation-one draft plan=%+v", plan)
	}
}
