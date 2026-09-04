package tobari

import "testing"

func templateCandidateFixture(t *testing.T, path, timestamp string) PolicyCandidate {
	t.Helper()
	denial := PolicyDenial{
		PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP},
		Timestamp:              timestamp, RequestID: "0123456789abcdef0123456789abcdef",
		WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad", WorkspaceManifestName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
		Host: "api.example.com", Port: 443, Method: "GET", Path: path,
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	return candidate
}

func TestPolicyReviewItemsProposesOneSegmentTemplateAfterTwoDistinctPaths(t *testing.T) {
	t.Parallel()
	first := templateCandidateFixture(t, "/items/123", "2026-08-15T01:00:00Z")
	second := templateCandidateFixture(t, "/items/456", "2026-08-15T01:01:00Z")

	one, err := PolicyReviewItems([]PolicyCandidate{first}, []LearnedPolicyRule{})
	if err != nil || len(one) != 1 || one[0].Match != PolicyMatchExact || one[0].ID != first.ID {
		t.Fatalf("one example review = %+v, error = %v", one, err)
	}

	two, err := PolicyReviewItems([]PolicyCandidate{first, second}, []LearnedPolicyRule{})
	if err != nil || len(two) != 1 || two[0].Match != PolicyMatchPathTemplate || two[0].Template == nil {
		t.Fatalf("two example review = %+v, error = %v", two, err)
	}
	proposal := *two[0].Template
	if proposal.Path != "/items/{id}" || len(proposal.PendingCandidates) != 2 ||
		len(proposal.Examples) != 2 || proposal.Examples[0] != "/items/123" || proposal.Examples[1] != "/items/456" {
		t.Fatalf("proposal = %+v", proposal)
	}

	rule, err := NewPathTemplateLearnedPolicyRule(proposal)
	if err != nil {
		t.Fatal(err)
	}
	identity := PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}
	if !rule.MatchesIdentity(first.WorkspaceManifestID, first.ProjectID, first.Host, 443, "GET", "/items/789", identity) {
		t.Fatal("template did not authorize one unseen safe segment")
	}
	for _, path := range []string{"/items", "/items/789/child", "/other/789", "/items/a%2Fb", "/items/..", "/items/"} {
		if rule.MatchesIdentity(first.WorkspaceManifestID, first.ProjectID, first.Host, 443, "GET", path, identity) {
			t.Fatalf("template authorized boundary canary %q", path)
		}
	}
	if rule.MatchesIdentity(first.WorkspaceManifestID, first.ProjectID, first.Host, 443, "POST", "/items/789", identity) ||
		rule.MatchesIdentity(first.WorkspaceManifestID, "01912345-6789-7abc-8def-0123456789ac", first.Host, 443, "GET", "/items/789", identity) {
		t.Fatal("template crossed method or project identity")
	}
}

func TestPolicyReviewItemsUsesCurrentExactAllowAsFirstTemplateExample(t *testing.T) {
	t.Parallel()
	first := templateCandidateFixture(t, "/items/123", "2026-08-15T01:00:00Z")
	firstRule, err := NewExactLearnedPolicyRule(first)
	if err != nil {
		t.Fatal(err)
	}
	second := templateCandidateFixture(t, "/items/456", "2026-08-15T01:01:00Z")

	items, err := PolicyReviewItems([]PolicyCandidate{second}, []LearnedPolicyRule{firstRule})
	if err != nil || len(items) != 1 || items[0].Template == nil {
		t.Fatalf("promoted review = %+v, error = %v", items, err)
	}
	proposal := items[0].Template
	if len(proposal.SourceRuleIDs) != 1 || proposal.SourceRuleIDs[0] != firstRule.ID ||
		len(proposal.PendingCandidates) != 1 || proposal.PendingCandidates[0].ID != second.ID {
		t.Fatalf("promoted proposal evidence = %+v", proposal)
	}
}

func TestPolicyReviewItemsDoesNotTreatRepeatedExactObservationsAsSecondExample(t *testing.T) {
	t.Parallel()
	candidate := templateCandidateFixture(t, "/items/123", "2026-08-15T01:00:00Z")
	candidate.ObservationCount = 9
	items, err := PolicyReviewItems([]PolicyCandidate{candidate}, []LearnedPolicyRule{})
	if err != nil || len(items) != 1 || items[0].Match != PolicyMatchExact {
		t.Fatalf("repeated observation review = %+v, error = %v", items, err)
	}
}

func TestPolicyReviewItemsSuppressesAmbiguousAndUnsafeTemplates(t *testing.T) {
	t.Parallel()
	paths := []string{"/a/1", "/a/2", "/b/1", "/encoded/a%2Fb", "/encoded/c%2Fd", "/single", "/other"}
	candidates := make([]PolicyCandidate, 0, len(paths))
	for index, path := range paths {
		candidates = append(candidates, templateCandidateFixture(t, path, "2026-08-15T01:0"+string(rune('0'+index))+":00Z"))
	}
	items, err := PolicyReviewItems(candidates, []LearnedPolicyRule{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Match != PolicyMatchExact {
			t.Fatalf("ambiguous or unsafe evidence produced template: %+v", item)
		}
	}
}

func TestPolicyReviewItemsKeepsScopesMethodsAndGraphQLSeparate(t *testing.T) {
	t.Parallel()
	first := templateCandidateFixture(t, "/items/123", "2026-08-15T01:00:00Z")
	otherMethod := templateCandidateFixture(t, "/items/456", "2026-08-15T01:01:00Z")
	otherMethod.Method = "POST"
	otherMethod.ID = ""
	denial := PolicyDenial{
		PolicyProtocolIdentity: otherMethod.PolicyProtocolIdentity,
		Timestamp:              otherMethod.ObservedAt, RequestID: "1123456789abcdef0123456789abcdef",
		WorkspaceManifestID: otherMethod.WorkspaceManifestID, WorkspaceManifestName: otherMethod.WorkspaceManifestName,
		ProjectID: otherMethod.ProjectID, ProjectRoot: otherMethod.ProjectRoot,
		Host: otherMethod.Host, Port: otherMethod.Port, Method: otherMethod.Method, Path: otherMethod.Path,
		Reason: otherMethod.Reason, StatusCode: otherMethod.StatusCode, Learnable: true,
	}
	var err error
	otherMethod, err = NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	graphql := templateCandidateFixture(t, "/items/789", "2026-08-15T01:02:00Z")
	graphql.PolicyProtocolIdentity = PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "item"}
	material := PolicyDenial{PolicyProtocolIdentity: graphql.PolicyProtocolIdentity, Timestamp: graphql.ObservedAt,
		RequestID: "2123456789abcdef0123456789abcdef", WorkspaceManifestID: graphql.WorkspaceManifestID, WorkspaceManifestName: graphql.WorkspaceManifestName,
		ProjectID: graphql.ProjectID, ProjectRoot: graphql.ProjectRoot, Host: graphql.Host, Port: graphql.Port,
		Method: graphql.Method, Path: graphql.Path, Reason: graphql.Reason, StatusCode: graphql.StatusCode, Learnable: true}
	graphql, err = NewPolicyCandidate(material)
	if err != nil {
		t.Fatal(err)
	}

	items, err := PolicyReviewItems([]PolicyCandidate{first, otherMethod, graphql}, []LearnedPolicyRule{})
	if err != nil || len(items) != 3 {
		t.Fatalf("cross-dimension review = %+v, error = %v", items, err)
	}
	for _, item := range items {
		if item.Match != PolicyMatchExact {
			t.Fatalf("cross-dimension evidence produced template: %+v", item)
		}
	}
}

func TestPolicyReviewItemsTreatsWorkspaceDisplayFactsAsProvenance(t *testing.T) {
	t.Parallel()
	first := templateCandidateFixture(t, "/items/123", "2026-08-15T01:00:00Z")
	second := templateCandidateFixture(t, "/items/456", "2026-08-15T01:01:00Z")
	second.WorkspaceManifestName = "renamed"
	items, err := PolicyReviewItems([]PolicyCandidate{first, second}, []LearnedPolicyRule{})
	if err != nil || len(items) != 1 || items[0].Match != PolicyMatchPathTemplate {
		t.Fatalf("Workspace provenance split compatible Context policy: items=%+v err=%v", items, err)
	}
}
