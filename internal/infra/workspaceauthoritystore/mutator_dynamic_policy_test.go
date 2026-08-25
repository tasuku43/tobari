package workspaceauthoritystore

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type observedPolicyCandidateReader struct {
	collection tobari.WorkspaceAuthorityCollection
	candidate  tobari.PolicyCandidateAuthority
}

func (r *observedPolicyCandidateReader) ListPolicyCandidatesIncludingAttachments(context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	return tobari.NewPolicyCandidateAuthorityListWithObservations(r.collection, true, []tobari.PolicyCandidateAuthority{r.candidate}, nil)
}

func TestMutatorAllowsObservedCandidateWithoutMutatingPredecessor(t *testing.T) {
	collection := storeCollectionFixture(t)
	previous := collection.Clone()
	template := collection.Templates[0].Clone()
	body := template.Current.Body.Clone()
	body.Boundary.DestinationCeiling = tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = []tobari.WorkspaceTemplateRevision{revision.Clone()}
	collection.Templates[0] = template
	collection.Contexts[0].ActiveTemplatePolicy.PolicySliceDigest = revision.Slices.PolicySliceDigest
	collection.Workspaces[0].LastSuccessfulEntry.TemplateRevision = revision.Revision
	collection.Workspaces[0].LastSuccessfulEntry.EntrySliceDigest = revision.Slices.EntrySliceDigest
	collection.PendingCandidates = []tobari.PolicyCandidateAuthority{}
	collection, _, err = tobari.PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates,
		collection.DefaultTemplateID, &previous,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := collection.Validate(); err != nil {
		t.Fatal(err)
	}
	effect := tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.synthetic.example", Port: 443, Method: "GET", Path: "/brokered-default",
		Segments: []string{}, Examples: []string{"/brokered-default"},
	}
	candidate, err := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, _, _, _ := newMutationFixture(t, &collection)
	reader := &observedPolicyCandidateReader{collection: collection, candidate: candidate}
	if observed, err := reader.ListPolicyCandidatesIncludingAttachments(context.Background()); err != nil {
		t.Fatalf("observed candidates: %v", err)
	} else if len(observed.Items) != 1 {
		t.Fatalf("observed candidates: %#v", observed)
	}
	mutator.bindPolicyCandidateObservation(reader)
	publication, err := mutator.AllowPolicyCandidateByReference(context.Background(), candidate.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := publication.ValidateFor(candidate.ID, tobari.PolicyMemoryAllow); err != nil {
		t.Fatal(err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || len(current.PendingCandidates) != 0 || len(current.Contexts[0].PolicyMemory.Rules) != 1 {
		t.Fatalf("published observed candidate authority=%#v present=%t err=%v", current, present, err)
	}
}
