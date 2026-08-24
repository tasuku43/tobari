package workspaceauthoritycmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type policyReadPortFake struct {
	candidates tobari.PolicyCandidateAuthorityList
	rules      tobari.PolicyMemoryRuleList
	err        error
}

func (f policyReadPortFake) ListPendingPolicyCandidateAuthority(context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	return f.candidates.Clone(), f.err
}

func (f policyReadPortFake) ListPolicyMemoryRuleAuthority(context.Context) (tobari.PolicyMemoryRuleList, error) {
	return f.rules.Clone(), f.err
}

func applicationPolicyReadCollection(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	snapshot := snapshotFixture(t, true, true)
	effect := policyEffect("/pending")
	candidate, err := tobari.NewPolicyCandidateAuthority(snapshot.Context.ID, snapshot.Workspace.ID, effect)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewPolicyMemoryRule(snapshot.Context.ID, tobari.PolicyMemoryDeny, policyEffect("/remembered").RuleBody("pcy_11111111111111111111111111111111"))
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := tobari.PublishPolicyMemory(snapshot.Context.ID, []tobari.PolicyMemoryRule{rule}, &snapshot.PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("publish memory: changed=%t err=%v", changed, err)
	}
	record := tobari.WorkspaceAuthorityContextRecord{
		Context: snapshot.Context, PolicyMemory: memory, ActiveTemplatePolicy: snapshot.ActiveTemplatePolicy,
		ActivePolicyMemory: snapshot.ActivePolicyMemory, ActivePolicyMemoryRef: snapshot.ActivePolicyMemoryRef,
	}
	collection, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{snapshot.Template}, []tobari.WorkspaceAuthorityContextRecord{record},
		[]tobari.WorkspaceBinding{*snapshot.Workspace}, []tobari.PolicyCandidateAuthority{candidate}, nil, nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish collection: changed=%t err=%v", changed, err)
	}
	return collection
}

func TestPolicyMemoryReadServiceReturnsValidatedExhaustiveFinalAuthority(t *testing.T) {
	collection := applicationPolicyReadCollection(t)
	candidates, err := tobari.NewPolicyCandidateAuthorityList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := tobari.NewPolicyMemoryRuleList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	service := NewPolicyMemoryService(policyReadPortFake{candidates: candidates, rules: rules})
	gotCandidates, err := service.Candidates(context.Background())
	if err != nil || len(gotCandidates.Items) != 1 || gotCandidates.Items[0].ID != collection.PendingCandidates[0].ID {
		t.Fatalf("Candidates()=%#v err=%v", gotCandidates, err)
	}
	gotRules, err := service.Rules(context.Background())
	if err != nil || len(gotRules.Items) != 1 || gotRules.Items[0].ID != collection.Contexts[0].PolicyMemory.Rules[0].ID {
		t.Fatalf("Rules()=%#v err=%v", gotRules, err)
	}
	gotCandidates.Items[0].Effect.Examples[0] = "/changed"
	if candidates.Items[0].Effect.Examples[0] == "/changed" {
		t.Fatal("application candidate result shares adapter authority")
	}
}

func TestPolicyMemoryReadServiceRejectsInvalidAdapterAuthority(t *testing.T) {
	collection := applicationPolicyReadCollection(t)
	candidates, _ := tobari.NewPolicyCandidateAuthorityList(collection, true)
	rules, _ := tobari.NewPolicyMemoryRuleList(collection, true)
	candidates.Items[0].ContextRef = "ctx_01912345-6789-7abc-8def-0123456789ff"
	rules.Items[0].TemplateRef = "wst_01912345-6789-7abc-8def-0123456789ff"

	service := NewPolicyMemoryService(policyReadPortFake{candidates: candidates, rules: rules})
	if _, err := service.Candidates(context.Background()); faultCode(err) != "invalid_policy_candidate_list" {
		t.Fatalf("candidate error=%v", err)
	}
	if _, err := service.Rules(context.Background()); faultCode(err) != "invalid_policy_rule_list" {
		t.Fatalf("rule error=%v", err)
	}
}

func TestPolicyMemoryReadServicePreservesCleanBreakLegacyFault(t *testing.T) {
	service := NewPolicyMemoryService(policyReadPortFake{err: tobari.ErrPreReleaseLegacyAuthority})
	_, err := service.Candidates(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable {
		t.Fatalf("legacy candidate fault=%#v ok=%t err=%v", public, ok, err)
	}
	_, err = service.Rules(context.Background())
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" {
		t.Fatalf("legacy rule fault=%#v ok=%t err=%v", public, ok, err)
	}
}

func TestPolicyMemoryReadServiceRequiresReadPort(t *testing.T) {
	service := NewPolicyMemoryService(struct{}{})
	if _, err := service.Candidates(context.Background()); err == nil {
		t.Fatal("Candidates accepted missing read port")
	}
	if _, err := service.Rules(context.Background()); err == nil {
		t.Fatal("Rules accepted missing read port")
	}
	service = NewPolicyMemoryService(policyReadPortFake{err: errors.New("unavailable")})
	if _, err := service.Candidates(context.Background()); faultCode(err) != "policy_candidate_read_failed" {
		t.Fatalf("read error=%v", err)
	}
}

func faultCode(err error) string {
	value, ok := fault.PublicCopy(err)
	if !ok {
		return ""
	}
	return value.Code
}
