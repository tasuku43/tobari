package tobari

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func reviewedCandidateFixture(t *testing.T, collection WorkspaceAuthorityCollection, path string) PolicyCandidateAuthority {
	t.Helper()
	effect := PolicyCandidateEffect{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: path,
		Segments: []string{}, Examples: []string{path},
	}
	candidate, err := NewPolicyCandidateAuthority(collection.Contexts[0].Context.ID, collection.Workspaces[0].ID, effect)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func reviewedExactDecisionFixture(t *testing.T, candidate PolicyCandidateAuthority, decision PolicyMemoryDecision) PolicyMemoryReviewedDecision {
	t.Helper()
	result, err := NewPolicyMemoryReviewedDecision(
		candidate.ID,
		[]PolicyCandidateAuthority{candidate}, []PolicyMemoryRule{}, decision, candidate.Effect.RuleBody(candidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func reviewedTemplateAuthorityFixture(t *testing.T) (WorkspaceAuthorityCollection, PolicyMemoryReviewedDecision) {
	t.Helper()
	base := workspaceAuthorityCollectionFixture(t)
	candidate := reviewedCandidateFixture(t, base, "/teams/b")
	sourceBody := policyMemoryBodyFixture("/teams/a")
	source, err := NewPolicyMemoryRule(base.Contexts[0].Context.ID, PolicyMemoryAllow, sourceBody)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := PublishPolicyMemory(base.Contexts[0].Context.ID, []PolicyMemoryRule{source}, &base.Contexts[0].PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("publish source memory: changed=%t err=%v", changed, err)
	}
	contexts := cloneWorkspaceAuthorityContextRecords(base.Contexts)
	contexts[0].PolicyMemory = memory
	contexts[0].ActivePolicyMemory = collectionPolicyMemoryPtr(memory)
	receipt := PolicyMemoryActivationReceipt{ContextID: contexts[0].Context.ID, Revision: memory.Revision}
	contexts[0].ActivePolicyMemoryRef = &receipt
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, base.Workspaces, []PolicyCandidateAuthority{candidate}, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish template review predecessor: changed=%t err=%v", changed, err)
	}
	sources := append([]string{candidate.ID}, source.Body.SourceCandidates...)
	sort.Strings(sources)
	sources = uniqueSortedStrings(sources)
	rule := PolicyMemoryRuleBody{
		PolicyProtocolIdentity: candidate.Effect.PolicyProtocolIdentity,
		Match:                  PolicyMatchPathTemplate, Host: candidate.Effect.Host, Port: candidate.Effect.Port, Method: candidate.Effect.Method,
		Path: "/teams/" + PolicyPathTemplatePlaceholder, Segments: []string{"teams", PolicyPathTemplatePlaceholder},
		Examples: []string{"/teams/a", "/teams/b"}, SourceCandidates: sources,
	}
	identityProbe := PolicyMemoryReviewedDecision{Candidates: []PolicyCandidateAuthority{candidate}, Rule: rule}
	reviewItemID, err := policyMemoryReviewedItemID(identityProbe)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewPolicyMemoryReviewedDecision(
		reviewItemID,
		[]PolicyCandidateAuthority{candidate}, []PolicyMemoryRule{source}, PolicyMemoryAllow, rule,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collection, decision
}

func reviewedPublicationFixture(
	t *testing.T,
	previous WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
) PolicyMemoryReviewedSetPublication {
	t.Helper()
	contexts := cloneWorkspaceAuthorityContextRecords(previous.Contexts)
	pending := clonePolicyCandidateAuthorities(previous.PendingCandidates)
	consumed := map[string]struct{}{}
	decisionsByContext := map[ContextID][]PolicyMemoryReviewedDecision{}
	for _, decision := range set.Decisions {
		decisionsByContext[decision.ContextID()] = append(decisionsByContext[decision.ContextID()], decision)
		for _, candidate := range decision.Candidates {
			consumed[candidate.ID] = struct{}{}
		}
	}
	keptPending := pending[:0]
	for _, candidate := range pending {
		if _, remove := consumed[candidate.ID]; !remove {
			keptPending = append(keptPending, candidate)
		}
	}
	changes := make([]PolicyMemoryReviewedContextChange, 0, len(decisionsByContext))
	for index := range contexts {
		decisions := decisionsByContext[contexts[index].Context.ID]
		if len(decisions) == 0 {
			continue
		}
		rules := clonePolicyMemoryRules(contexts[index].PolicyMemory.Rules)
		remove := map[string]struct{}{}
		for _, decision := range decisions {
			for _, source := range decision.SourceRules {
				remove[source.ID] = struct{}{}
			}
		}
		kept := rules[:0]
		for _, rule := range rules {
			if _, drop := remove[rule.ID]; !drop {
				kept = append(kept, rule)
			}
		}
		rules = kept
		for _, decision := range decisions {
			rule, err := NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
			if err != nil {
				t.Fatal(err)
			}
			rules = append(rules, rule)
		}
		before := contexts[index].PolicyMemory.Clone()
		nextMemory, changed, err := PublishPolicyMemory(contexts[index].Context.ID, rules, &before)
		if err != nil || !changed {
			t.Fatalf("publish reviewed memory: changed=%t err=%v", changed, err)
		}
		contexts[index].PolicyMemory = nextMemory
		contexts[index].ActivePolicyMemory = collectionPolicyMemoryPtr(nextMemory)
		receipt := PolicyMemoryActivationReceipt{ContextID: contexts[index].Context.ID, Revision: nextMemory.Revision}
		contexts[index].ActivePolicyMemoryRef = &receipt
		changes = append(changes, PolicyMemoryReviewedContextChange{ContextID: contexts[index].Context.ID, Previous: before, Current: nextMemory.Clone()})
	}
	next, changed, err := PublishWorkspaceAuthorityCollection(
		previous.Templates, contexts, previous.Workspaces, keptPending, previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("publish reviewed collection: changed=%t err=%v", changed, err)
	}
	projection, err := BuildReviewedWorkspacePolicyProjection(next, policyMemoryReviewedTargetContexts(set))
	if err != nil {
		t.Fatal(err)
	}
	_ = changes // the constructor derives and validates the exhaustive result.
	publication, err := NewPolicyMemoryReviewedSetPublication(
		previous, next, set,
		PolicyMemoryReviewedSettlementReceipt{
			DecisionSetDigest: set.Digest, PlanDigest: projection.PlanDigest, ContentDigest: projection.ContentDigest,
			AggregateRevision: strings.Repeat("a", 64), PolicyArtifact: authorityDigest("b"),
			GatewayArtifact: authorityDigest("c"), PrincipalDigest: authorityDigest("d"),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return publication
}

func reviewedSecondContextFixture(
	t *testing.T,
	base WorkspaceAuthorityCollection,
	path string,
) (WorkspaceAuthorityCollection, PolicyCandidateAuthority) {
	t.Helper()
	contextID := ContextID("01912345-6789-7abc-8def-0123456789a4")
	workspaceID := WorkspaceID("01912345-6789-7abc-8def-0123456789a5")
	template := base.Templates[0]
	binding := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion, ID: contextID,
		TemplateID: template.ID,
	}
	memory, _, err := PublishPolicyMemory(contextID, []PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := TemplatePolicyActivationReceipt{
		ContextID: contextID, TemplateID: template.ID, PolicySliceDigest: template.Current.Slices.PolicySliceDigest,
	}
	memoryReceipt := PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	record := WorkspaceAuthorityContextRecord{
		Context: binding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt,
		ActivePolicyMemory: collectionPolicyMemoryPtr(memory), ActivePolicyMemoryRef: &memoryReceipt,
	}
	applied := WorkspaceAppliedEntry{
		ContextID: contextID, TemplateID: template.ID, TemplateRevision: template.Current.Revision,
		EntrySliceDigest: template.Current.Slices.EntrySliceDigest, RuntimeID: template.Current.Slices.RuntimeID,
		RuntimeRevision: template.Current.Slices.RuntimeRevision, ResolvedSpec: authorityDigest("6"),
		ReconciledAt: base.Workspaces[0].LastSuccessfulEntry.ReconciledAt,
	}
	workspace := WorkspaceBinding{
		SchemaVersion: WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID,
		ProjectRoot: "/workspace/second", Home: "/workspace/second-home",
		CreationDefaults: template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied,
	}
	effect := PolicyCandidateEffect{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Match:                  PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: path,
		Segments: []string{}, Examples: []string{path},
	}
	candidate, err := NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	contexts := append(cloneWorkspaceAuthorityContextRecords(base.Contexts), record)
	workspaces := append(cloneWorkspaceBindings(base.Workspaces), workspace)
	pending := append(clonePolicyCandidateAuthorities(base.PendingCandidates), candidate)
	collection, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, workspaces, pending, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish second reviewed Context: changed=%t err=%v", changed, err)
	}
	return collection, candidate
}

func TestPolicyMemoryReviewedDecisionSetRejectsEmptyAndPreservesReviewItemID(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	if _, err := NewPolicyMemoryReviewedDecisionSet(base, []PolicyMemoryReviewedDecision{}); err == nil {
		t.Fatal("empty reviewed set passed")
	}
	decision := reviewedExactDecisionFixture(t, base.PendingCandidates[0], PolicyMemoryDeny)
	if decision.ReviewItemID != base.PendingCandidates[0].ID {
		t.Fatalf("exact review item ID = %q", decision.ReviewItemID)
	}
	set, err := NewPolicyMemoryReviewedDecisionSet(base, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, base, set)
	if err := publication.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(publication.AppliedDecisions) != 1 || publication.AppliedDecisions[0].ReviewItemID != decision.ReviewItemID ||
		publication.AppliedDecisions[0].Decision != PolicyMemoryDeny || publication.AppliedDecisions[0].Match != PolicyMatchExact {
		t.Fatalf("applied decision lost reviewed identity: %#v", publication.AppliedDecisions)
	}
}

func TestPolicyMemoryReviewedPublicationConstructorRejectsMalformedSetWithoutPanic(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	malformed := PolicyMemoryReviewedDecisionSet{
		SchemaVersion: PolicyMemoryReviewedSetSchemaVersion, TargetID: PolicyDecisionSetID,
		Decisions: []PolicyMemoryReviewedDecision{{Candidates: []PolicyCandidateAuthority{}}},
		Digest:    authorityDigest("1"),
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("malformed reviewed set panicked: %v", recovered)
		}
	}()
	if _, err := NewPolicyMemoryReviewedSetPublication(
		base, base, malformed,
		PolicyMemoryReviewedSettlementReceipt{
			DecisionSetDigest: authorityDigest("1"), PlanDigest: authorityDigest("2"), ContentDigest: authorityDigest("3"),
			AggregateRevision: strings.Repeat("a", 64), PolicyArtifact: authorityDigest("4"),
			GatewayArtifact: authorityDigest("5"), PrincipalDigest: authorityDigest("6"),
		},
	); err == nil {
		t.Fatal("malformed reviewed set passed publication constructor")
	}
}

func TestPolicyMemoryReviewedTemplateCompactsExactRuleAndPendingCandidate(t *testing.T) {
	previous, decision := reviewedTemplateAuthorityFixture(t)
	if !strings.HasPrefix(decision.ReviewItemID, "ptp_") || decision.ReviewItemID == string(decision.ProposalDigest) {
		t.Fatalf("template review item ID was replaced by internal digest: %#v", decision)
	}
	set, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, previous, set)
	if err := publication.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(publication.Next.Contexts[0].PolicyMemory.Rules) != 1 ||
		publication.Next.Contexts[0].PolicyMemory.Rules[0].Body.Match != PolicyMatchPathTemplate ||
		len(publication.Next.PendingCandidates) != 0 || len(publication.AppliedDecisions[0].ReplacedSourceRules) != 1 {
		t.Fatalf("template compaction did not replace exact authority: %#v", publication)
	}
	clone := publication.Clone()
	clone.DecisionSet.Decisions[0].Rule.Examples[0] = "/changed"
	clone.AppliedDecisions[0].ConsumedCandidates[0] = "pcy_00000000000000000000000000000000"
	if reflect.DeepEqual(clone, publication) || publication.DecisionSet.Decisions[0].Rule.Examples[0] == "/changed" {
		t.Fatal("reviewed publication clone shares nested authority")
	}
}

func TestPolicyMemoryReviewedTemplateKeepsStableSelectedItemAcrossCompatibleEvidence(t *testing.T) {
	previous, selected := reviewedTemplateAuthorityFixture(t)
	additional := reviewedCandidateFixture(t, previous, "/teams/c")
	candidates := append(clonePolicyCandidateAuthorities(selected.Candidates), additional)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	rule := selected.Rule.Clone()
	rule.Examples = []string{"/teams/a", "/teams/b", "/teams/c"}
	rule.SourceCandidates = append(append([]string{}, selected.Rule.SourceCandidates...), additional.ID)
	sort.Strings(rule.SourceCandidates)
	rule.SourceCandidates = uniqueSortedStrings(rule.SourceCandidates)
	expanded, err := NewPolicyMemoryReviewedDecision(
		selected.ReviewItemID, candidates, selected.SourceRules, PolicyMemoryAllow, rule,
	)
	if err != nil {
		t.Fatal(err)
	}
	if expanded.ReviewItemID != selected.ReviewItemID || expanded.ProposalDigest == selected.ProposalDigest {
		t.Fatalf("stable item/evidence identities were conflated: selected=%#v expanded=%#v", selected, expanded)
	}
	proposal := PolicyPathTemplateProposal{
		PolicyProtocolIdentity: selected.Rule.PolicyProtocolIdentity,
		WorkspaceManifestID:    string(selected.Candidates[0].ContextID),
		ProjectID:              string(selected.Candidates[0].ObservingWorkspaceID),
		Host:                   selected.Rule.Host,
		Port:                   selected.Rule.Port,
		Method:                 selected.Rule.Method,
		Path:                   selected.Rule.Path,
	}
	if got := pathTemplateProposalID(proposal); got != selected.ReviewItemID {
		t.Fatalf("existing proposal item identity did not round-trip: got=%q want=%q", got, selected.ReviewItemID)
	}
}

func TestPolicyMemoryReviewedTemplateRejectsSelectedItemScopeOrTemplateDrift(t *testing.T) {
	previous, selected := reviewedTemplateAuthorityFixture(t)
	otherWorkspace := WorkspaceID("01912345-6789-7abc-8def-0123456789a5")
	tests := []struct {
		name      string
		context   ContextID
		workspace WorkspaceID
		prefix    string
	}{
		{name: "Context scope", context: ContextID("01912345-6789-7abc-8def-0123456789a4"), workspace: selected.Candidates[0].ObservingWorkspaceID, prefix: "/teams/"},
		{name: "Workspace scope", context: selected.ContextID(), workspace: otherWorkspace, prefix: "/teams/"},
		{name: "template path", context: selected.ContextID(), workspace: selected.Candidates[0].ObservingWorkspaceID, prefix: "/groups/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidates := make([]PolicyCandidateAuthority, 0, 2)
			for _, suffix := range []string{"a", "b"} {
				effect := reviewedCandidateFixture(t, previous, test.prefix+suffix).Effect
				candidate, err := NewPolicyCandidateAuthority(test.context, test.workspace, effect)
				if err != nil {
					t.Fatal(err)
				}
				candidates = append(candidates, candidate)
			}
			sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
			sources := []string{candidates[0].ID, candidates[1].ID}
			sort.Strings(sources)
			rule := PolicyMemoryRuleBody{
				PolicyProtocolIdentity: candidates[0].Effect.PolicyProtocolIdentity,
				Match:                  PolicyMatchPathTemplate, Host: candidates[0].Effect.Host, Port: candidates[0].Effect.Port,
				Method: candidates[0].Effect.Method, Path: test.prefix + PolicyPathTemplatePlaceholder,
				Segments: []string{strings.Trim(test.prefix, "/"), PolicyPathTemplatePlaceholder},
				Examples: []string{test.prefix + "a", test.prefix + "b"}, SourceCandidates: sources,
			}
			if _, err := NewPolicyMemoryReviewedDecision(
				selected.ReviewItemID, candidates, nil, PolicyMemoryAllow, rule,
			); err == nil {
				t.Fatal("selected path-template item survived scope/template drift")
			}
		})
	}
}

func TestPolicyMemoryReviewedPublicationIsOneOrderedMultiContextResult(t *testing.T) {
	first, templateDecision := reviewedTemplateAuthorityFixture(t)
	previous, secondCandidate := reviewedSecondContextFixture(t, first, "/second")
	denyDecision := reviewedExactDecisionFixture(t, secondCandidate, PolicyMemoryDeny)
	set, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{templateDecision, denyDecision})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{denyDecision, templateDecision})
	if err != nil || !reflect.DeepEqual(reversed, set) {
		t.Fatalf("reversed reviewed input did not canonicalize to one set: reversed=%#v set=%#v err=%v", reversed, set, err)
	}
	noncanonical := set.Clone()
	noncanonical.Decisions[0], noncanonical.Decisions[1] = noncanonical.Decisions[1], noncanonical.Decisions[0]
	noncanonical.Digest, err = policyMemoryReviewedDecisionSetDigest(noncanonical)
	if err != nil || noncanonical.Validate() == nil {
		t.Fatalf("noncanonical reviewed set passed strict validation: err=%v", err)
	}
	publication := reviewedPublicationFixture(t, previous, set)
	if err := publication.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(publication.Changes) != 2 || len(publication.AppliedDecisions) != 2 ||
		publication.AppliedDecisions[0].ReviewItemID != set.Decisions[0].ReviewItemID ||
		publication.AppliedDecisions[1].ReviewItemID != set.Decisions[1].ReviewItemID {
		t.Fatalf("multi-Context result lost reviewed order or authority: %#v", publication.AppliedDecisions)
	}
	seenTemplate, seenDeny := false, false
	for _, item := range publication.AppliedDecisions {
		seenTemplate = seenTemplate || item.Match == PolicyMatchPathTemplate
		seenDeny = seenDeny || item.Decision == PolicyMemoryDeny
	}
	if !seenTemplate || !seenDeny {
		t.Fatalf("multi-Context result lost reviewed decisions: %#v", publication.AppliedDecisions)
	}
	projection, err := BuildReviewedWorkspacePolicyProjection(publication.Next, policyMemoryReviewedTargetContexts(set))
	if err != nil || len(projection.TargetContextIDs) != 2 || projection.TargetContextIDs[0] >= projection.TargetContextIDs[1] {
		t.Fatalf("reviewed projection did not bind the complete sorted target set: %#v err=%v", projection, err)
	}
}

func TestPolicyMemoryReviewedTransitionRejectsValidNextForDifferentReviewedSet(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	previous, secondCandidate := reviewedSecondContextFixture(t, base, "/second")
	var firstCandidate PolicyCandidateAuthority
	for _, candidate := range previous.PendingCandidates {
		if candidate.ContextID == base.Contexts[0].Context.ID {
			firstCandidate = candidate
		}
	}
	setA, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{
		reviewedExactDecisionFixture(t, firstCandidate, PolicyMemoryAllow),
	})
	if err != nil {
		t.Fatal(err)
	}
	setB, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{
		reviewedExactDecisionFixture(t, secondCandidate, PolicyMemoryDeny),
	})
	if err != nil {
		t.Fatal(err)
	}
	nextB := reviewedPublicationFixture(t, previous, setB).Next
	if err := ValidatePolicyMemoryReviewedTransition(previous, nextB, setA); err == nil {
		t.Fatal("reviewed set A accepted the valid next authority produced by set B")
	}
}

func TestPolicyMemoryReviewedTemplateRejectsHostileOrStaleAuthority(t *testing.T) {
	previous, valid := reviewedTemplateAuthorityFixture(t)
	tests := map[string]func(PolicyMemoryReviewedDecision) PolicyMemoryReviewedDecision{
		"broader host": func(value PolicyMemoryReviewedDecision) PolicyMemoryReviewedDecision {
			value.Rule.Host = "other.example.dev"
			return value
		},
		"omitted evidence": func(value PolicyMemoryReviewedDecision) PolicyMemoryReviewedDecision {
			value.Rule.Examples = value.Rule.Examples[:1]
			return value
		},
		"deny template": func(value PolicyMemoryReviewedDecision) PolicyMemoryReviewedDecision {
			value.Decision = PolicyMemoryDeny
			return value
		},
		"changed source rule": func(value PolicyMemoryReviewedDecision) PolicyMemoryReviewedDecision {
			value.SourceRules[0].Body.Path = "/teams/c"
			value.SourceRules[0].Body.Examples = []string{"/teams/c"}
			return value
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := mutate(valid.Clone())
			if err := candidate.Validate(); err == nil {
				t.Fatal("hostile reviewed template passed")
			}
		})
	}

	set, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{valid})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, previous, set)
	stale := publication.Clone()
	staleBody := policyMemoryBodyFixture("/teams/c")
	staleRule, err := NewPolicyMemoryRule(stale.Previous.Contexts[0].Context.ID, PolicyMemoryAllow, staleBody)
	if err != nil {
		t.Fatal(err)
	}
	staleMemory, _, err := PublishPolicyMemory(stale.Previous.Contexts[0].Context.ID, []PolicyMemoryRule{staleRule}, &stale.Previous.Contexts[0].PolicyMemory)
	if err != nil {
		t.Fatal(err)
	}
	stale.Previous.Contexts[0].PolicyMemory = staleMemory
	stale.Previous.Contexts[0].ActivePolicyMemory = collectionPolicyMemoryPtr(staleMemory)
	staleMemoryReceipt := PolicyMemoryActivationReceipt{ContextID: stale.Previous.Contexts[0].Context.ID, Revision: staleMemory.Revision}
	stale.Previous.Contexts[0].ActivePolicyMemoryRef = &staleMemoryReceipt
	stale.Previous.Revision, _ = workspaceAuthorityCollectionRevision(stale.Previous)
	if err := stale.Validate(); err == nil {
		t.Fatal("stale source-rule authority passed publication validation")
	}

	publication.Next.Contexts[0].PolicyMemory = previous.Contexts[0].PolicyMemory.Clone()
	publication.Next.Contexts[0].ActivePolicyMemory = collectionPolicyMemoryPtr(previous.Contexts[0].PolicyMemory)
	receipt := PolicyMemoryActivationReceipt{ContextID: previous.Contexts[0].Context.ID, Revision: previous.Contexts[0].PolicyMemory.Revision}
	publication.Next.Contexts[0].ActivePolicyMemoryRef = &receipt
	publication.Next.Revision, _ = workspaceAuthorityCollectionRevision(publication.Next)
	if err := publication.Validate(); err == nil {
		t.Fatal("publication omitted reviewed source-rule replacement")
	}
}

func TestPolicyMemoryReviewedPublicationRejectsUnrelatedSettlementOrAppliedResult(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	decision := reviewedExactDecisionFixture(t, base.PendingCandidates[0], PolicyMemoryAllow)
	set, err := NewPolicyMemoryReviewedDecisionSet(base, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	valid := reviewedPublicationFixture(t, base, set)
	tests := map[string]func(*PolicyMemoryReviewedSetPublication){
		"decision set digest": func(value *PolicyMemoryReviewedSetPublication) {
			value.Settlement.DecisionSetDigest = authorityDigest("9")
		},
		"plan digest":    func(value *PolicyMemoryReviewedSetPublication) { value.Settlement.PlanDigest = authorityDigest("9") },
		"content digest": func(value *PolicyMemoryReviewedSetPublication) { value.Settlement.ContentDigest = authorityDigest("9") },
		"review item ID": func(value *PolicyMemoryReviewedSetPublication) {
			value.AppliedDecisions[0].ReviewItemID = "pcy_00000000000000000000000000000000"
		},
		"rule ID": func(value *PolicyMemoryReviewedSetPublication) {
			value.AppliedDecisions[0].RuleID = "pmr_00000000000000000000000000000000"
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := valid.Clone()
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("unrelated reviewed result authority passed")
			}
		})
	}
}

func TestPolicyMemoryReviewedSetRejectsConcurrentCollectionDrift(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	confirmed, _ := reviewedSecondContextFixture(t, base, "/second-pending")
	var firstCandidate PolicyCandidateAuthority
	for _, candidate := range confirmed.PendingCandidates {
		if candidate.ContextID == base.Contexts[0].Context.ID {
			firstCandidate = candidate
		}
	}
	decision := reviewedExactDecisionFixture(t, firstCandidate, PolicyMemoryAllow)
	set, err := NewPolicyMemoryReviewedDecisionSet(confirmed, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	second := confirmed.Contexts[1]
	body := policyMemoryBodyFixture("/second-unrelated")
	rule, err := NewPolicyMemoryRule(second.Context.ID, PolicyMemoryDeny, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := PublishPolicyMemory(second.Context.ID, []PolicyMemoryRule{rule}, &second.PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("concurrent memory: changed=%t err=%v", changed, err)
	}
	contexts := cloneWorkspaceAuthorityContextRecords(confirmed.Contexts)
	contexts[1].PolicyMemory = memory
	contexts[1].ActivePolicyMemory = collectionPolicyMemoryPtr(memory)
	receipt := PolicyMemoryActivationReceipt{ContextID: second.Context.ID, Revision: memory.Revision}
	contexts[1].ActivePolicyMemoryRef = &receipt
	drifted, changed, err := PublishWorkspaceAuthorityCollection(
		confirmed.Templates, contexts, confirmed.Workspaces, confirmed.PendingCandidates, confirmed.DefaultTemplateID, &confirmed,
	)
	if err != nil || !changed {
		t.Fatalf("concurrent collection: changed=%t err=%v", changed, err)
	}
	projection, err := BuildReviewedWorkspacePolicyProjection(drifted, policyMemoryReviewedTargetContexts(set))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewPolicyMemoryReviewedSetPublication(
		drifted, drifted, set,
		PolicyMemoryReviewedSettlementReceipt{
			DecisionSetDigest: set.Digest, PlanDigest: projection.PlanDigest, ContentDigest: projection.ContentDigest,
			AggregateRevision: strings.Repeat("a", 64), PolicyArtifact: authorityDigest("b"),
			GatewayArtifact: authorityDigest("c"), PrincipalDigest: authorityDigest("d"),
		},
	)
	if err == nil {
		t.Fatal("reviewed set silently adopted an unrelated concurrent Context change")
	}
}

func TestPolicyMemoryReviewedPublicationJSONExposesOnlyTaskOwnedActiveReferences(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	decision := reviewedExactDecisionFixture(t, base.PendingCandidates[0], PolicyMemoryAllow)
	set, err := NewPolicyMemoryReviewedDecisionSet(base, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, base, set)
	encoded, err := json.Marshal(publication)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, key := range []string{
		`"task"`, `"active_revision"`, `"allow_count"`, `"deny_count"`, `"applied"`, `"decisions"`,
		`"review_item_id"`, `"rule_id"`,
	} {
		if !strings.Contains(text, key) {
			t.Fatalf("reviewed publication JSON omitted %s: %s", key, text)
		}
	}
	for _, key := range []string{
		`"target_id"`, `"decision_set"`, `"settlement"`, `"changed"`, `"context_id"`,
		`"context_ref"`, `"template_ref"`, `"observing_workspace_ref"`,
		`"consumed_candidate_ids"`, `"source_rule_ids"`, `"proposal_digest"`,
	} {
		if strings.Contains(text, key) {
			t.Fatalf("reviewed publication JSON exposed private %s: %s", key, text)
		}
	}
}

func TestPolicyMemoryReviewedPublicationCompactsOnlyBothCompleteCollections(t *testing.T) {
	previous, decision := reviewedTemplateAuthorityFixture(t)
	set, err := NewPolicyMemoryReviewedDecisionSet(previous, []PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	publication := reviewedPublicationFixture(t, previous, set)
	compact, err := publication.CompactTerminal()
	if err != nil || compact.Previous.SchemaVersion != 0 || compact.Next.SchemaVersion != 0 || compact.Validate() != nil {
		t.Fatalf("compact=%#v err=%v validate=%v", compact, err, compact.Validate())
	}
	half := compact.Clone()
	half.Previous = publication.Previous.Clone()
	if err := half.Validate(); err == nil {
		t.Fatal("half-compact reviewed terminal publication passed")
	}
}
