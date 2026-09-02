package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type observedPolicyCandidateReader struct {
	collection tobari.WorkspaceAuthorityCollection
	candidates []tobari.PolicyCandidateAuthority
}

func (r *observedPolicyCandidateReader) ListPolicyCandidatesIncludingAttachments(context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	return tobari.NewPolicyCandidateAuthorityListWithObservations(r.collection, true, r.candidates, nil)
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
	reader := &observedPolicyCandidateReader{collection: collection, candidates: []tobari.PolicyCandidateAuthority{candidate}}
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

func TestMutatorAppliesReviewedLiveCandidatesWithoutPersistingInbox(t *testing.T) {
	for _, decision := range []tobari.PolicyMemoryDecision{tobari.PolicyMemoryAllow, tobari.PolicyMemoryDeny} {
		t.Run(string(decision), func(t *testing.T) {
			collection := storeCollectionFixture(t)
			candidate := collection.PendingCandidates[0].Clone()
			var err error
			collection, _, err = tobari.PublishWorkspaceAuthorityCollection(
				collection.Templates, collection.Contexts, collection.Workspaces, []tobari.PolicyCandidateAuthority{},
				collection.DefaultTemplateID, &collection,
			)
			if err != nil {
				t.Fatal(err)
			}
			reviewed, err := tobari.NewPolicyMemoryReviewedDecision(
				candidate.ID, []tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryRule{},
				decision, candidate.Effect.RuleBody(candidate.ID),
			)
			if err != nil {
				t.Fatal(err)
			}
			set, err := tobari.NewPolicyMemoryReviewedDecisionSetWithObservations(collection, []tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryReviewedDecision{reviewed})
			if err != nil {
				t.Fatal(err)
			}
			store, mutator, _, _, _ := newMutationFixture(t, &collection)
			settlement := mutator.settlement.(*finalSettlementFixture)
			mutator.bindPolicyCandidateObservation(&observedPolicyCandidateReader{collection: collection, candidates: []tobari.PolicyCandidateAuthority{candidate}})
			publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
			if err != nil {
				t.Fatal(err)
			}
			if err := publication.Validate(); err != nil || len(publication.LiveCandidates) != 1 || settlement.reviewedCalls != 1 {
				t.Fatalf("publication=%#v settlement=%d validate=%v", publication, settlement.reviewedCalls, err)
			}
			current, present, err := store.ReadComplete(context.Background())
			if err != nil || !present || len(current.PendingCandidates) != 0 || len(current.Contexts[0].PolicyMemory.Rules) != 1 || current.Contexts[0].PolicyMemory.Rules[0].Decision != decision {
				t.Fatalf("current=%#v present=%t err=%v", current, present, err)
			}
			adapter, err := NewFinalPolicyCandidateAdapter(store, &finalPolicyCandidateRuntimeFixture{read: tobari.DenialRead{Items: []tobari.PolicyDenial{}}}, mutator, &HostLoopbackPolicyAdapter{})
			if err != nil {
				t.Fatal(err)
			}
			terminalSnapshot, err := adapter.ReadPolicyMemoryReviewSnapshot(context.Background())
			if err != nil || terminalSnapshot.ReviewedApplyRecovery != nil {
				t.Fatalf("terminal Apply was projected as replayable recovery: %#v err=%v", terminalSnapshot.ReviewedApplyRecovery, err)
			}
			terminal, err := publication.CompactTerminal()
			if err != nil || len(terminal.LiveCandidates) != 0 || terminal.Validate() != nil {
				t.Fatalf("terminal=%#v err=%v validate=%v", terminal, err, terminal.Validate())
			}
		})
	}
}

func TestMutatorReviewedLiveCandidatesRejectDisappearMismatchAndStaleReceipt(t *testing.T) {
	base := storeCollectionFixture(t)
	candidate := base.PendingCandidates[0].Clone()
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, base.Workspaces, []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := tobari.NewPolicyMemoryReviewedDecision(candidate.ID, []tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryRule{}, tobari.PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSetWithObservations(collection, []tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryReviewedDecision{reviewed})
	if err != nil {
		t.Fatal(err)
	}
	changed := candidate.Clone()
	changed.Effect.Path = "/changed"
	changed.Effect.Examples = []string{"/changed"}
	changed, err = tobari.NewPolicyCandidateAuthority(changed.ContextID, changed.ObservingWorkspaceID, changed.Effect)
	if err != nil {
		t.Fatal(err)
	}
	staleCandidate := candidate.Clone()
	staleCandidate.Effect.Path = "/unrelated"
	staleCandidate.Effect.Examples = []string{"/unrelated"}
	staleCandidate, err = tobari.NewPolicyCandidateAuthority(staleCandidate.ContextID, staleCandidate.ObservingWorkspaceID, staleCandidate.Effect)
	if err != nil {
		t.Fatal(err)
	}
	stale, _, err := tobari.PublishWorkspaceAuthorityCollection(
		collection.Templates, collection.Contexts, collection.Workspaces, []tobari.PolicyCandidateAuthority{staleCandidate}, collection.DefaultTemplateID, &collection,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		collection tobari.WorkspaceAuthorityCollection
		candidates []tobari.PolicyCandidateAuthority
	}{
		{name: "disappeared", collection: collection, candidates: []tobari.PolicyCandidateAuthority{}},
		{name: "changed", collection: collection, candidates: []tobari.PolicyCandidateAuthority{changed}},
		{name: "stale-receipt", collection: stale, candidates: []tobari.PolicyCandidateAuthority{candidate}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, mutator, _, _, _ := newMutationFixture(t, &collection)
			settlement := mutator.settlement.(*finalSettlementFixture)
			mutator.bindPolicyCandidateObservation(&observedPolicyCandidateReader{collection: test.collection, candidates: test.candidates})
			_, applyErr := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
			if !errors.Is(applyErr, tobari.ErrPolicyReviewChanged) {
				t.Fatalf("error=%v", applyErr)
			}
			if settlement.reviewedCalls != 0 {
				t.Fatalf("settlement calls=%d", settlement.reviewedCalls)
			}
		})
	}
}

func TestMutatorAppliesReviewedLivePathTemplateAndMultipleExactChoices(t *testing.T) {
	base := storeCollectionFixture(t)
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, base.Workspaces, []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	makeCandidate := func(path string) tobari.PolicyCandidateAuthority {
		effect := base.PendingCandidates[0].Effect.Clone()
		effect.Path, effect.Examples = path, []string{path}
		candidate, candidateErr := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, effect)
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		return candidate
	}
	first, second, third := makeCandidate("/teams/a"), makeCandidate("/teams/b"), makeCandidate("/other")
	list, err := tobari.NewPolicyCandidateAuthorityListWithObservations(collection, true, []tobari.PolicyCandidateAuthority{first, second, third}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := tobari.NewPolicyMemoryReviewSnapshot(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err = tobari.JoinPolicyMemoryReviewCandidates(snapshot, list)
	if err != nil {
		t.Fatal(err)
	}
	choices := map[string]tobari.PolicyMemoryDecision{third.ID: tobari.PolicyMemoryDeny}
	for _, item := range snapshot.Items {
		if item.Match == tobari.PolicyMatchPathTemplate && strings.HasPrefix(item.Rule.Path, "/teams/") {
			choices[item.ID] = tobari.PolicyMemoryAllow
		}
	}
	set, err := snapshot.ReviewedSet(choices)
	if err != nil || len(set.Decisions) != 2 {
		t.Fatalf("set=%#v err=%v", set, err)
	}
	_, mutator, _, _, _ := newMutationFixture(t, &collection)
	settlement := mutator.settlement.(*finalSettlementFixture)
	mutator.bindPolicyCandidateObservation(&observedPolicyCandidateReader{collection: collection, candidates: []tobari.PolicyCandidateAuthority{first, second, third}})
	publication, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set)
	if err != nil || publication.AllowCount != 1 || publication.DenyCount != 1 || settlement.reviewedCalls != 1 {
		t.Fatalf("publication=%#v settlement=%d err=%v", publication, settlement.reviewedCalls, err)
	}
}

func TestMutatorRecoversAcceptedReviewedLiveSetWithoutReobservingCandidate(t *testing.T) {
	base := storeCollectionFixture(t)
	candidate := base.PendingCandidates[0].Clone()
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, base.Workspaces, []tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := tobari.NewPolicyMemoryReviewedDecision(candidate.ID, []tobari.PolicyCandidateAuthority{candidate}, nil, tobari.PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSetWithObservations(collection, []tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryReviewedDecision{reviewed})
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, lifecycle, deletion, activation := newMutationFixture(t, &collection)
	reader := &observedPolicyCandidateReader{collection: collection, candidates: []tobari.PolicyCandidateAuthority{candidate}}
	mutator.bindPolicyCandidateObservation(reader)
	realRename := mutator.rename
	stage, authority := mutationStagePath(store.root), filepath.Join(store.root, authorityFileName)
	mutator.rename = func(source, target string) error {
		if source == stage && target == authority {
			return fmt.Errorf("injected publication interruption")
		}
		return realRename(source, target)
	}
	if _, err := mutator.ApplyReviewedPolicyMemory(context.Background(), set); err == nil {
		t.Fatal("interrupted reviewed live publication reported success")
	}
	reader.candidates = []tobari.PolicyCandidateAuthority{}

	// Reconstruct every process-local owner. The only caller-facing recovery
	// authority is the fresh Permission Inbox snapshot; the disappeared denial
	// is neither reinserted nor rediscovered.
	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.legacyGuard = mutationLegacyGuard{}
	recoveredMutator, err := NewMutator(context.Background(), reopened, lifecycle, &templateRuntimeRevisionFixture{}, deletion, activation, mutator.settlement)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &finalPolicyCandidateRuntimeFixture{read: tobari.DenialRead{Items: []tobari.PolicyDenial{}}}
	adapter, err := NewFinalPolicyCandidateAdapter(reopened, runtime, recoveredMutator, &HostLoopbackPolicyAdapter{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := adapter.ReadPolicyMemoryReviewSnapshot(context.Background())
	if err != nil || len(snapshot.Items) != 0 || snapshot.ReviewedApplyRecovery == nil || !reflect.DeepEqual(snapshot.ReviewedApplyRecovery.DecisionSet, set) {
		t.Fatalf("fresh Permission Inbox snapshot=%#v err=%v", snapshot, err)
	}
	service := workspaceauthoritycmd.NewPolicyMemoryService(adapter)
	applyIntent := operation.Intent{
		Command: workspaceauthoritycmd.TaskPolicyApply, Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ParentID: tobari.PolicyDecisionSetID},
		Impact: workspaceauthoritycmd.PolicyMemoryImpact(),
	}
	publication, err := service.ApplyReviewed(context.Background(), applyIntent, snapshot.ReviewedApplyRecovery.DecisionSet)
	if err != nil || publication.Validate() != nil || len(publication.AppliedDecisions) != 1 {
		t.Fatalf("publication=%#v err=%v validate=%v", publication, err, publication.Validate())
	}
}
