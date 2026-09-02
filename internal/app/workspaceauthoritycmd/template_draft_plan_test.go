package workspaceauthoritycmd

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestTemplateServiceReturnsGenerationOneDraftPlanUnchanged(t *testing.T) {
	active := templateFixture(t)
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{active}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	draftID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789f7")
	body := bodyFixture("/later")
	body.Boundary.DestinationCeiling = tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}
	source, err := tobari.NewWorkspaceTemplateDraftSource(draftID, "later", body)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.NewWorkspaceTemplateChangePlan(collection, draftID, source, body.EntryDefaults.Runtime, map[tobari.WorkspaceID]bool{}, strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	port := &fakePort{plan: plan}
	ref, _ := tobari.WorkspaceTemplateRef(draftID)
	got, err := NewTemplateService(port).Plan(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlanRef != plan.PlanRef || got.TemplateRef != ref || got.ActiveRevision != nil || port.lastRef != ref {
		t.Fatalf("draft plan=%+v requested=%q", got, port.lastRef)
	}
}
