package tobari

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func policyReadCollectionFixture(t *testing.T) WorkspaceAuthorityCollection {
	t.Helper()
	base := workspaceAuthorityCollectionFixture(t)
	body := PolicyMemoryRuleBody{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/remembered",
		Segments: []string{}, Examples: []string{"/remembered"}, SourceCandidates: []string{"pcy_" + strings.Repeat("1", 32)},
	}
	rule, err := NewPolicyMemoryRule(base.Contexts[0].Context.ID, PolicyMemoryDeny, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := PublishPolicyMemory(base.Contexts[0].Context.ID, []PolicyMemoryRule{rule}, &base.Contexts[0].PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("publish memory: changed=%t err=%v", changed, err)
	}
	contexts := cloneWorkspaceAuthorityContextRecords(base.Contexts)
	contexts[0].PolicyMemory = memory
	collection, changed, err := PublishWorkspaceAuthorityCollection(base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base)
	if err != nil || !changed {
		t.Fatalf("publish read collection: changed=%t err=%v", changed, err)
	}
	return collection
}

func twoContextPolicyReadCollection(t *testing.T) WorkspaceAuthorityCollection {
	t.Helper()
	base := policyReadCollectionFixture(t)
	secondContextID := ContextID("01912345-6789-7abc-8def-0123456789b2")
	secondWorkspaceID := WorkspaceID("01912345-6789-7abc-8def-0123456789b3")
	template := base.Templates[0]
	secondContext := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: secondContextID, TemplateID: template.ID}
	ruleBody := base.Contexts[0].PolicyMemory.Rules[0].Body.Clone()
	ruleBody.SourceCandidates = []string{"pcy_" + strings.Repeat("3", 32)}
	secondRule, err := NewPolicyMemoryRule(secondContextID, base.Contexts[0].PolicyMemory.Rules[0].Decision, ruleBody)
	if err != nil {
		t.Fatal(err)
	}
	secondMemory, _, err := PublishPolicyMemory(secondContextID, []PolicyMemoryRule{secondRule}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{ContextID: secondContextID, TemplateID: template.ID, PolicySliceDigest: template.Current.Slices.PolicySliceDigest}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: secondContextID, Revision: secondMemory.Revision}
	secondRecord := WorkspaceAuthorityContextRecord{Context: secondContext, PolicyMemory: secondMemory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: collectionPolicyMemoryPtr(secondMemory), ActivePolicyMemoryRef: &memoryReceipt}
	entry := WorkspaceAppliedEntry{
		ContextID: secondContextID, TemplateID: template.ID, TemplateRevision: template.Current.Revision,
		EntrySliceDigest: template.Current.Slices.EntrySliceDigest, RuntimeID: template.Current.Slices.RuntimeID,
		RuntimeRevision: template.Current.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("8"), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	secondWorkspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: secondWorkspaceID, ContextID: secondContextID,
		ProjectRoot: "/workspace/second", Home: "/workspace/home-second",
		CreationDefaults: template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &entry,
	}
	secondCandidate, err := NewPolicyCandidateAuthority(secondContextID, secondWorkspaceID, base.PendingCandidates[0].Effect)
	if err != nil {
		t.Fatal(err)
	}
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		cloneWorkspaceTemplates(base.Templates),
		append(cloneWorkspaceAuthorityContextRecords(base.Contexts), secondRecord),
		append(cloneWorkspaceBindings(base.Workspaces), secondWorkspace),
		append(clonePolicyCandidateAuthorities(base.PendingCandidates), secondCandidate), base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish two-Context collection: changed=%t err=%v", changed, err)
	}
	return collection
}

func TestFinalPolicyReadJSONProducesOnlyCandidateAndRuleReferences(t *testing.T) {
	collection := twoContextPolicyReadCollection(t)
	candidates, _ := NewPolicyCandidateAuthorityList(collection, true)
	rules, _ := NewPolicyMemoryRuleList(collection, true)
	if len(candidates.Items) != 2 || candidates.Items[0].Context != candidates.Items[1].Context ||
		candidates.Items[0].Template != candidates.Items[1].Template ||
		candidates.Items[0].ProjectRoot == candidates.Items[1].ProjectRoot ||
		candidates.Items[0].ObservingWorkspace == candidates.Items[1].ObservingWorkspace ||
		!reflect.DeepEqual(candidates.Items[0].Effect, candidates.Items[1].Effect) {
		t.Fatalf("same-effect candidate scope is not visibly distinct: %#v", candidates.Items)
	}
	if len(rules.Items) != 2 || rules.Items[0].Context != rules.Items[1].Context || rules.Items[0].Template != rules.Items[1].Template ||
		rules.Items[0].ContextID == rules.Items[1].ContextID ||
		rules.Items[0].Body.Host != rules.Items[1].Body.Host || rules.Items[0].Body.Path != rules.Items[1].Body.Path {
		t.Fatalf("same-body rule scope is not visibly distinct: %#v", rules.Items)
	}
	for name, value := range map[string]any{"candidates": candidates, "rules": rules} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(encoded, &document); err != nil {
			t.Fatal(err)
		}
		if key := firstPolicyReadReferenceKey(document); key != "" {
			t.Fatalf("%s unexpectedly produced owner reference field %q: %s", name, key, encoded)
		}
		root := document.(map[string]any)
		items := root["items"].([]any)
		for _, raw := range items {
			item := raw.(map[string]any)
			if item["context_id"] == nil || item["workspace_template_id"] == nil {
				t.Fatalf("%s omitted non-reference final identity dimensions: %s", name, encoded)
			}
			if name == "rules" {
				if _, present := item["project_root"]; present {
					t.Fatalf("location-free Policy Memory rule leaked a Project root: %s", encoded)
				}
			} else if item["project_root"] == nil {
				t.Fatalf("candidate omitted its observing Workspace root: %s", encoded)
			}
			if name == "candidates" && item["observing_workspace_id"] == nil {
				t.Fatalf("candidate omitted observing Workspace identity: %s", encoded)
			}
		}
	}
}

func TestPolicyCandidateReadCoalescesLegacyAndCurrentIDsByContextEffect(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	current := base.PendingCandidates[0].Clone()
	legacy := current.Clone()
	legacy.ID = legacyPolicyCandidateAuthorityID(legacy.ContextID, legacy.ObservingWorkspaceID, legacy.PayloadDigest)
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, base.Workspaces,
		[]PolicyCandidateAuthority{legacy}, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish legacy/current aliases: changed=%t err=%v", changed, err)
	}
	list, err := NewPolicyCandidateAuthorityListWithObservations(collection, true, []PolicyCandidateAuthority{current}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].ID != legacy.ID || list.Items[0].Authority.ContextID != current.ContextID || list.Items[0].Authority.PayloadDigest != current.PayloadDigest {
		t.Fatalf("legacy/current aliases remained independently actionable: %#v", list.Items)
	}
	review, err := NewPolicyMemoryReviewSnapshot(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Items) != 1 || review.Items[0].Match != PolicyMatchExact {
		t.Fatalf("legacy/current aliases produced duplicate review items: %#v", review.Items)
	}
}

func TestFinalPolicyReadSchemaThreeRejectsPredecessorSchema(t *testing.T) {
	collection := policyReadCollectionFixture(t)
	candidates, err := NewPolicyCandidateAuthorityList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := NewPolicyMemoryRuleList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	if candidates.SchemaVersion != 3 || rules.SchemaVersion != 3 {
		t.Fatalf("final Policy read schemas = %d/%d, want 3/3", candidates.SchemaVersion, rules.SchemaVersion)
	}
	candidates.SchemaVersion = 2
	if err := candidates.Validate(); err == nil {
		t.Fatal("Policy candidate schema 2 was accepted after the semantic-coordinate cut")
	}
	rules.SchemaVersion = 2
	if err := rules.Validate(); err == nil {
		t.Fatal("Policy rule schema 2 was accepted after the semantic-coordinate cut")
	}
}

func firstPolicyReadReferenceKey(value any) string {
	switch current := value.(type) {
	case map[string]any:
		for key, child := range current {
			if strings.HasSuffix(key, "_ref") {
				return key
			}
			if found := firstPolicyReadReferenceKey(child); found != "" {
				return found
			}
		}
	case []any:
		for _, child := range current {
			if found := firstPolicyReadReferenceKey(child); found != "" {
				return found
			}
		}
	}
	return ""
}

func TestFinalPolicyReadViewsBindCompleteCollectionReferences(t *testing.T) {
	collection := policyReadCollectionFixture(t)
	candidates, err := NewPolicyCandidateAuthorityList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := NewPolicyMemoryRuleList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	contextRef, _ := ContextRef(collection.Contexts[0].Context.ID)
	templateRef, _ := WorkspaceTemplateRef(collection.Contexts[0].Context.TemplateID)
	workspaceRef, _ := WorkspaceRef(collection.Workspaces[0].ID)
	if len(candidates.Items) != 1 || candidates.Items[0].ID != collection.PendingCandidates[0].ID ||
		candidates.Items[0].ContextRef != contextRef || candidates.Items[0].TemplateRef != templateRef ||
		candidates.Items[0].ObservingWorkspaceRef != workspaceRef || !reflect.DeepEqual(candidates.Items[0].Effect, collection.PendingCandidates[0].Effect) {
		t.Fatalf("candidate views=%#v", candidates)
	}
	if len(rules.Items) != 1 || rules.Items[0].ID != collection.Contexts[0].PolicyMemory.Rules[0].ID ||
		rules.Items[0].ContextRef != contextRef || rules.Items[0].TemplateRef != templateRef {
		t.Fatalf("rule views=%#v", rules)
	}

	changed := candidates.Clone()
	changed.Items[0].TemplateRef = "wst_01912345-6789-7abc-8def-0123456789ff"
	if err := changed.ValidateFor(collection, true); err == nil {
		t.Fatal("candidate list accepted a relabeled Template reference")
	}
	changedRules := rules.Clone()
	changedRules.Items[0].Body.Path = "/other"
	if err := changedRules.ValidateFor(collection, true); err == nil {
		t.Fatal("rule list accepted body drift")
	}
}

func TestFinalPolicyReadViewsDistinguishFreshAndInitializedEmpty(t *testing.T) {
	freshCandidates, err := NewPolicyCandidateAuthorityList(WorkspaceAuthorityCollection{}, false)
	if err != nil {
		t.Fatal(err)
	}
	freshRules, err := NewPolicyMemoryRuleList(WorkspaceAuthorityCollection{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if freshCandidates.Items == nil || freshRules.Items == nil || freshCandidates.CollectionPresent || freshRules.CollectionPresent {
		t.Fatalf("fresh candidates=%#v rules=%#v", freshCandidates, freshRules)
	}

	collection := workspaceAuthorityCollectionFixture(t)
	collection, _, err = PublishWorkspaceAuthorityCollection(collection.Templates, collection.Contexts, collection.Workspaces, []PolicyCandidateAuthority{}, collection.DefaultTemplateID, &collection)
	if err != nil {
		t.Fatal(err)
	}
	initialized, err := NewPolicyCandidateAuthorityList(collection, true)
	if err != nil {
		t.Fatal(err)
	}
	if !initialized.CollectionPresent || initialized.CollectionGeneration == 0 || initialized.Items == nil || len(initialized.Items) != 0 {
		t.Fatalf("initialized empty=%#v", initialized)
	}
}

func TestFinalPolicyReadViewsAreCloneIsolatedAndRejectCollectionSubstitution(t *testing.T) {
	collection := policyReadCollectionFixture(t)
	candidates, _ := NewPolicyCandidateAuthorityList(collection, true)
	rules, _ := NewPolicyMemoryRuleList(collection, true)
	candidateClone := candidates.Clone()
	ruleClone := rules.Clone()
	candidateClone.Items[0].Effect.Examples[0] = "/changed"
	ruleClone.Items[0].Body.Examples[0] = "/changed"
	if candidates.Items[0].Effect.Examples[0] == "/changed" || rules.Items[0].Body.Examples[0] == "/changed" {
		t.Fatal("final Policy read clone shares nested authority")
	}

	other := collection.Clone()
	other.Generation++
	if err := candidates.ValidateFor(other, true); err == nil {
		t.Fatal("candidate list accepted another collection receipt")
	}
	if err := rules.ValidateFor(other, true); err == nil {
		t.Fatal("rule list accepted another collection receipt")
	}
}
