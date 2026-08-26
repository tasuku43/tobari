package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalPolicyPortFixture struct {
	candidates tobari.PolicyCandidateAuthorityList
	rules      tobari.PolicyMemoryRuleList
	review     tobari.PolicyMemoryReviewSnapshot
	allow      tobari.PolicyCandidatePublication
	deny       tobari.PolicyCandidatePublication
	reset      tobari.PolicyRuleResetPublication
	allowCalls int
	denyCalls  int
	resetCalls int
}

func (f *finalPolicyPortFixture) ListPendingPolicyCandidateAuthority(context.Context) (tobari.PolicyCandidateAuthorityList, error) {
	return f.candidates.Clone(), nil
}

func (f *finalPolicyPortFixture) ListPolicyMemoryRuleAuthority(context.Context) (tobari.PolicyMemoryRuleList, error) {
	return f.rules.Clone(), nil
}

func (f *finalPolicyPortFixture) ReadPolicyMemoryReviewSnapshot(context.Context) (tobari.PolicyMemoryReviewSnapshot, error) {
	return f.review.Clone(), nil
}

func (f *finalPolicyPortFixture) AllowPolicyCandidateByReference(context.Context, string) (tobari.PolicyCandidatePublication, error) {
	f.allowCalls++
	return f.allow, nil
}

func (f *finalPolicyPortFixture) DenyPolicyCandidateByReference(context.Context, string) (tobari.PolicyCandidatePublication, error) {
	f.denyCalls++
	return f.deny, nil
}

func (f *finalPolicyPortFixture) ResetPolicyMemoryRuleByReference(context.Context, string) (tobari.PolicyRuleResetPublication, error) {
	f.resetCalls++
	return f.reset, nil
}

func finalPolicyCLIFixture(t *testing.T) (*finalPolicyPortFixture, string, string) {
	t.Helper()
	const (
		templateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
		contextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a2"
		workspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
	)
	digest := func(value string) tobari.SemanticDigest {
		return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
	}
	body := tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "payments", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/payments", TemplateID: templateID}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: binding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest}
	empty, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := "pcy_11111111111111111111111111111111"
	rememberedBody := tobari.PolicyMemoryRuleBody{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Match: tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/remembered", Segments: []string{}, Examples: []string{"/remembered"}, SourceCandidates: []string{sourceID}}
	remembered, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryDeny, rememberedBody)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{remembered}, &empty)
	if err != nil || !changed {
		t.Fatalf("publish remembered memory: changed=%t err=%v", changed, err)
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	activeMemory := memory.Clone()
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	record := tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt}
	effect := tobari.PolicyCandidateEffect{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Match: tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/pending", Segments: []string{}, Examples: []string{"/pending"}}
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record}, []tobari.WorkspaceBinding{workspace}, []tobari.PolicyCandidateAuthority{candidate}, nil, nil)
	if err != nil || !changed {
		t.Fatalf("publish collection: changed=%t err=%v", changed, err)
	}
	candidates, err := tobari.NewPolicyCandidateAuthorityList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := tobari.NewPolicyMemoryRuleList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	review, err := tobari.NewPolicyMemoryReviewSnapshot(collection, true)
	if err != nil {
		t.Fatal(err)
	}

	publication := func(decision tobari.PolicyMemoryDecision) tobari.PolicyCandidatePublication {
		rule, ruleErr := tobari.NewPolicyMemoryRule(contextID, decision, effect.RuleBody(candidate.ID))
		if ruleErr != nil {
			t.Fatal(ruleErr)
		}
		next, nextChanged, nextErr := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{remembered, rule}, &memory)
		if nextErr != nil || !nextChanged {
			t.Fatalf("publish candidate memory: changed=%t err=%v", nextChanged, nextErr)
		}
		active := next.Clone()
		receipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: next.Revision}
		snapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: next, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &active, ActivePolicyMemoryRef: &receipt, Workspace: &workspace}
		result := tobari.PolicyCandidatePublication{Candidate: candidate, RuleID: rule.ID, Previous: memory, Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: memory.Revision, Changed: true}}
		if err := result.ValidateFor(candidate.ID, decision); err != nil {
			t.Fatal(err)
		}
		return result
	}

	resetMemory, resetChanged, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, &memory)
	if err != nil || !resetChanged {
		t.Fatalf("publish reset memory: changed=%t err=%v", resetChanged, err)
	}
	resetActive := resetMemory.Clone()
	resetReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: resetMemory.Revision}
	resetSnapshot := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: resetMemory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &resetActive, ActivePolicyMemoryRef: &resetReceipt, Workspace: &workspace}
	reset := tobari.PolicyRuleResetPublication{RuleID: remembered.ID, RemovedFrom: memory, Memory: tobari.PolicyMemoryPublication{Snapshot: resetSnapshot, PreviousRevision: memory.Revision, Changed: true}}
	if err := reset.ValidateFor(remembered.ID); err != nil {
		t.Fatal(err)
	}
	return &finalPolicyPortFixture{candidates: candidates, rules: rules, review: review, allow: publication(tobari.PolicyMemoryAllow), deny: publication(tobari.PolicyMemoryDeny), reset: reset}, candidate.ID, remembered.ID
}

func TestFinalPolicyCatalogHasExactSchemaAndReferenceGraph(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"policy candidates", "review permissions", "policy rules", "policy allow", "policy deny", "policy reset", "policy apply-reviewed"} {
		if _, found := catalog.lookupRegistered(path); !found {
			t.Fatalf("missing %q", path)
		}
	}
	candidates, _ := catalog.Lookup("policy candidates")
	rules, _ := catalog.Lookup("policy rules")
	if candidates.Agent.Output.JSONSchemaVersion != 2 || rules.Agent.Output.JSONSchemaVersion != 2 {
		t.Fatalf("policy schemas candidates=%d rules=%d", candidates.Agent.Output.JSONSchemaVersion, rules.Agent.Output.JSONSchemaVersion)
	}
	if got := candidates.ProducedRefs(); !reflect.DeepEqual(got, []ProducedRef{{Kind: tobari.PolicyCandidateKind, Field: "id"}}) {
		t.Fatalf("candidate refs=%+v", got)
	}
	if got := rules.ProducedRefs(); !reflect.DeepEqual(got, []ProducedRef{{Kind: tobari.PolicyRuleKind, Field: "id"}}) {
		t.Fatalf("rule refs=%+v", got)
	}
	for _, path := range []string{"policy candidates", "policy rules"} {
		spec, _ := catalog.Lookup(path)
		encoded, _ := json.Marshal(spec.Agent.Output.Fields)
		if strings.Contains(string(encoded), "context_ref") || strings.Contains(string(encoded), "template_ref") || strings.Contains(string(encoded), "workspace_ref") || strings.Contains(string(encoded), "manifest") {
			t.Fatalf("%s output contract leaks forbidden reference/legacy fields: %s", path, encoded)
		}
	}
	apply, found := catalog.lookupRegistered("policy apply-reviewed")
	if !found || apply.Visibility != CommandVisibilityInternal || !reflect.DeepEqual(apply.ProducedRefs(), []ProducedRef{{Kind: tobari.PolicyRuleKind, Field: "decisions[].rule_id"}}) || apply.Agent.FixedTarget == nil || apply.Effect != operation.EffectCreate {
		t.Fatalf("apply reviewed contract=%+v", apply)
	}
	encoded, _ := json.Marshal(apply.Agent.Output.Fields)
	for _, forbidden := range []string{"context_ref", "template_ref", "observing_workspace_ref"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("apply reviewed output publishes forbidden owner ref %q: %s", forbidden, encoded)
		}
	}
}

func TestFinalPolicyReadsAndDirectMutationsUseOnlyFinalService(t *testing.T) {
	port, candidateID, ruleID := finalPolicyCLIFixture(t)
	for _, test := range []struct {
		args      []string
		envelope  string
		wantField string
	}{
		{[]string{"policy", "candidates", "--format", "json"}, "policy_candidates", candidateID},
		{[]string{"policy", "rules", "--format", "json"}, "policy_rules", ruleID},
		{[]string{"policy", "allow", "--id", candidateID, "--format", "json"}, "result", "\"decision\":\"allow\""},
		{[]string{"policy", "deny", "--id", candidateID, "--format", "json"}, "result", "\"decision\":\"deny\""},
		{[]string{"policy", "reset", "--id", ruleID, "--format", "json"}, "result", "\"removed\":true"},
	} {
		var out, errOut bytes.Buffer
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalPolicy = workspaceauthoritycmd.NewPolicyMemoryService(port)
		if code := command.RunContext(context.Background(), test.args); code != ExitOK {
			t.Fatalf("%v code=%d stdout=%q stderr=%q", test.args, code, out.String(), errOut.String())
		}
		if !strings.Contains(out.String(), `"schema_version":2`) || !strings.Contains(out.String(), `"`+test.envelope+`"`) || !strings.Contains(out.String(), test.wantField) || strings.Contains(out.String(), "manifest") {
			t.Fatalf("%v output=%q", test.args, out.String())
		}
	}
	if port.allowCalls != 1 || port.denyCalls != 1 || port.resetCalls != 1 {
		t.Fatalf("mutation calls allow=%d deny=%d reset=%d", port.allowCalls, port.denyCalls, port.resetCalls)
	}
}

func TestFinalPolicyInvalidReferencesFailBeforeAdapter(t *testing.T) {
	port, _, _ := finalPolicyCLIFixture(t)
	for _, args := range [][]string{
		{"policy", "allow", "--id", "manifest"},
		{"policy", "deny", "--id", "ctx_legacy"},
		{"policy", "reset", "--id", "plr_11111111111111111111111111111111"},
	} {
		var out, errOut bytes.Buffer
		command := newCLI(strings.NewReader(""), &out, &errOut, DefaultCatalog(), nil)
		command.finalPolicy = workspaceauthoritycmd.NewPolicyMemoryService(port)
		if code := command.RunContext(context.Background(), args); code == ExitOK || out.Len() != 0 {
			t.Fatalf("%v code=%d stdout=%q stderr=%q", args, code, out.String(), errOut.String())
		}
	}
	if port.allowCalls != 0 || port.denyCalls != 0 || port.resetCalls != 0 {
		t.Fatalf("invalid refs crossed adapter: %+v", port)
	}
}
