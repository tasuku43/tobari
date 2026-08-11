package tobari

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	policyProjectA = "01912345-6789-7abc-8def-0123456789ab"
	policyProjectB = "01912345-6789-7abc-8def-0123456789ac"
	policyContextA = "01912345-6789-7abc-8def-0123456789ad"
	policyContextB = "01912345-6789-7abc-8def-0123456789ae"
)

func validPolicyDenial() PolicyDenial {
	return PolicyDenial{
		Timestamp: "2026-07-30T10:41:11Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: policyContextA, ContextName: "default",
		ProjectID: policyProjectA, ProjectRoot: "/workspace/project-a",
		Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
		Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
	}
}

func TestDenialReportPreservesEmptyBoundedScope(t *testing.T) {
	t.Parallel()
	report := DenialReport{
		Task: TaskClusterDenials, PolicyDirectory: "/config/tobari/policy",
		WindowLines: 200, Items: []PolicyDenial{},
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	report.Items = nil
	if err := report.Validate(); err == nil {
		t.Fatal("unknown denial collection was accepted")
	}
}

func TestPolicyDenialRejectsInterpretationSensitiveFields(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*PolicyDenial){
		"timestamp":  func(value *PolicyDenial) { value.Timestamp = "recently" },
		"request id": func(value *PolicyDenial) { value.RequestID = "GET-api.github.com" },
		"host":       func(value *PolicyDenial) { value.Host = "api.github.com\nallow=true" },
		"port":       func(value *PolicyDenial) { value.Port = 0 },
		"method":     func(value *PolicyDenial) { value.Method = "GET POST" },
		"path":       func(value *PolicyDenial) { value.Path = "repos/cli/cli" },
		"status":     func(value *PolicyDenial) { value.StatusCode = 200 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			value := validPolicyDenial()
			mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid denial was accepted")
			}
		})
	}
	profile := "profile\nunsafe"
	value := validPolicyDenial()
	value.CredentialProfile = &profile
	if err := value.Validate(); err == nil {
		t.Fatal("control-bearing credential profile was accepted")
	}
}

func TestPolicyProtocolIdentityValidationAndEffectiveProtocol(t *testing.T) {
	t.Parallel()
	valid := []PolicyProtocolIdentity{
		{},
		{Protocol: PolicyProtocolHTTP},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "_viewer"},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationMutation, GraphQLRootField: "updateIssue"},
	}
	for _, identity := range valid {
		if err := identity.Validate(); err != nil {
			t.Fatalf("valid policy protocol identity %+v was rejected: %v", identity, err)
		}
	}
	if got := (PolicyProtocolIdentity{}).EffectiveProtocol(); got != PolicyProtocolHTTP {
		t.Fatalf("absent protocol resolved to %q, want %q", got, PolicyProtocolHTTP)
	}

	invalid := []PolicyProtocolIdentity{
		{Protocol: "grpc"},
		{Protocol: PolicyProtocolHTTP, GraphQLOperationType: GraphQLOperationQuery},
		{Protocol: PolicyProtocolHTTP, GraphQLRootField: "viewer"},
		{Protocol: PolicyProtocolGraphQL, GraphQLRootField: "viewer"},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: "subscription", GraphQLRootField: "events"},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "1viewer"},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "bad-name"},
		{Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: strings.Repeat("a", 257)},
	}
	for _, identity := range invalid {
		if err := identity.Validate(); err == nil {
			t.Fatalf("invalid policy protocol identity %+v was accepted", identity)
		}
	}
}

func TestGraphQLEndpointValidationAndExactMatch(t *testing.T) {
	t.Parallel()
	endpoint := GraphQLEndpoint{Scheme: "https", Host: "api.example.com", Port: 443, Path: "/graphql"}
	if err := endpoint.Validate(); err != nil {
		t.Fatalf("valid GraphQL endpoint was rejected: %v", err)
	}
	local := GraphQLEndpoint{Scheme: "http", Host: "mock-upstream", Port: 8080, Path: "/graphql/v1/"}
	if err := local.Validate(); err != nil {
		t.Fatalf("valid local GraphQL endpoint was rejected: %v", err)
	}
	if !endpoint.Matches("https", "api.example.com", 443, "/graphql") {
		t.Fatal("exact GraphQL endpoint did not match")
	}
	for _, coordinates := range []struct {
		scheme string
		host   string
		port   int
		path   string
	}{
		{scheme: "http", host: endpoint.Host, port: endpoint.Port, path: endpoint.Path},
		{scheme: endpoint.Scheme, host: "other.example.com", port: endpoint.Port, path: endpoint.Path},
		{scheme: endpoint.Scheme, host: endpoint.Host, port: 8443, path: endpoint.Path},
		{scheme: endpoint.Scheme, host: endpoint.Host, port: endpoint.Port, path: "/graphql/"},
	} {
		if endpoint.Matches(coordinates.scheme, coordinates.host, coordinates.port, coordinates.path) {
			t.Fatalf("non-exact GraphQL endpoint coordinates %+v matched", coordinates)
		}
	}

	invalid := []GraphQLEndpoint{
		{Scheme: "ws", Host: endpoint.Host, Port: endpoint.Port, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: "API.example.com", Port: endpoint.Port, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: "api.example.com.", Port: endpoint.Port, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: "api_example.com", Port: endpoint.Port, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: 0, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: 65536, Path: endpoint.Path},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: "graphql"},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: "/graphql?op=viewer"},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: "/graphql/../admin"},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: "/graphql//v1"},
		{Scheme: endpoint.Scheme, Host: endpoint.Host, Port: endpoint.Port, Path: "/graphql%2fv1"},
	}
	for _, value := range invalid {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid GraphQL endpoint %+v was accepted", value)
		}
	}
}

func TestHTTPProtocolIdentityRetainsLegacyOpaqueIDs(t *testing.T) {
	t.Parallel()
	legacyDenial := validPolicyDenial()
	explicitHTTPDenial := legacyDenial
	explicitHTTPDenial.Protocol = PolicyProtocolHTTP

	legacyCandidate, err := NewPolicyCandidate(legacyDenial)
	if err != nil {
		t.Fatal(err)
	}
	explicitHTTPCandidate, err := NewPolicyCandidate(explicitHTTPDenial)
	if err != nil {
		t.Fatal(err)
	}
	if legacyCandidate.ID != explicitHTTPCandidate.ID {
		t.Fatalf("explicit HTTP changed candidate ID: legacy=%s explicit=%s", legacyCandidate.ID, explicitHTTPCandidate.ID)
	}

	legacyAllow, err := NewExactLearnedPolicyRule(legacyCandidate)
	if err != nil {
		t.Fatal(err)
	}
	explicitHTTPAllow, err := NewExactLearnedPolicyRule(explicitHTTPCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if legacyAllow.ID != explicitHTTPAllow.ID {
		t.Fatalf("explicit HTTP changed allow rule ID: legacy=%s explicit=%s", legacyAllow.ID, explicitHTTPAllow.ID)
	}

	legacyDeny, err := NewExactPolicyDenyRule(legacyCandidate)
	if err != nil {
		t.Fatal(err)
	}
	explicitHTTPDeny, err := NewExactPolicyDenyRule(explicitHTTPCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if legacyDeny.ID != explicitHTTPDeny.ID {
		t.Fatalf("explicit HTTP changed deny rule ID: legacy=%s explicit=%s", legacyDeny.ID, explicitHTTPDeny.ID)
	}
}

func TestGraphQLIdentityBindsCandidatesRulesAndMatching(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	denial.Method = "POST"
	denial.Path = "/graphql"
	denial.PolicyProtocolIdentity = PolicyProtocolIdentity{
		Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationMutation, GraphQLRootField: "updateIssue",
	}
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.PolicyProtocolIdentity != denial.PolicyProtocolIdentity {
		t.Fatalf("candidate identity = %+v, want %+v", candidate.PolicyProtocolIdentity, denial.PolicyProtocolIdentity)
	}

	httpDenial := denial
	httpDenial.PolicyProtocolIdentity = PolicyProtocolIdentity{}
	httpCandidate, err := NewPolicyCandidate(httpDenial)
	if err != nil {
		t.Fatal(err)
	}
	queryDenial := denial
	queryDenial.GraphQLOperationType = GraphQLOperationQuery
	queryCandidate, err := NewPolicyCandidate(queryDenial)
	if err != nil {
		t.Fatal(err)
	}
	otherRootDenial := denial
	otherRootDenial.GraphQLRootField = "deleteIssue"
	otherRootCandidate, err := NewPolicyCandidate(otherRootDenial)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.ID == httpCandidate.ID || candidate.ID == queryCandidate.ID || candidate.ID == otherRootCandidate.ID {
		t.Fatalf("GraphQL candidate IDs did not bind protocol coordinates: mutation=%s http=%s query=%s other-root=%s", candidate.ID, httpCandidate.ID, queryCandidate.ID, otherRootCandidate.ID)
	}

	allow, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !allow.MatchesIdentity(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path, denial.PolicyProtocolIdentity) ||
		!deny.MatchesIdentity(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path, denial.PolicyProtocolIdentity) {
		t.Fatal("GraphQL rules did not match their exact coordinate")
	}
	if allow.Matches(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path) ||
		deny.Matches(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path) {
		t.Fatal("GraphQL rules matched the legacy HTTP coordinate")
	}
	if allow.MatchesIdentity(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path, queryDenial.PolicyProtocolIdentity) ||
		deny.MatchesIdentity(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path, otherRootDenial.PolicyProtocolIdentity) {
		t.Fatal("GraphQL rule matched a different operation type or root field")
	}

	items, err := CurrentPolicyRules([]LearnedPolicyRule{allow}, []PolicyDenyRule{deny})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].PolicyProtocolIdentity != denial.PolicyProtocolIdentity || items[1].PolicyProtocolIdentity != denial.PolicyProtocolIdentity {
		t.Fatalf("current rule inventory lost GraphQL identity: %+v", items)
	}

	allow.GraphQLRootField = "deleteIssue"
	if err := allow.Validate(); err == nil {
		t.Fatal("allow rule ID did not bind GraphQL identity")
	}
	deny.GraphQLRootField = "deleteIssue"
	if err := deny.Validate(); err == nil {
		t.Fatal("deny rule ID did not bind GraphQL identity")
	}
}

func TestPolicyCandidateAggregationKeepsGraphQLCoordinatesDistinct(t *testing.T) {
	t.Parallel()
	update := validPolicyDenial()
	update.Method = "POST"
	update.Path = "/graphql"
	update.PolicyProtocolIdentity = PolicyProtocolIdentity{
		Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationMutation, GraphQLRootField: "updateIssue",
	}
	repeated := update
	repeated.Timestamp = "2026-07-30T10:42:11Z"
	repeated.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	deleteIssue := update
	deleteIssue.Timestamp = "2026-07-30T10:43:11Z"
	deleteIssue.RequestID = "9185da2688d7469aae9cd9068e920b0b"
	deleteIssue.GraphQLRootField = "deleteIssue"
	http := update
	http.Timestamp = "2026-07-30T10:44:11Z"
	http.RequestID = "a185da2688d7469aae9cd9068e920b0b"
	http.PolicyProtocolIdentity = PolicyProtocolIdentity{}

	items, err := PolicyCandidates([]PolicyDenial{update, repeated, deleteIssue, http}, []LearnedPolicyRule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("candidate count = %d, want 3: %+v", len(items), items)
	}
	counts := make(map[string]int, len(items))
	for _, item := range items {
		key := item.EffectiveProtocol() + ":" + item.GraphQLOperationType + ":" + item.GraphQLRootField
		counts[key] = item.EffectiveObservationCount()
	}
	if counts["graphql:mutation:updateIssue"] != 2 || counts["graphql:mutation:deleteIssue"] != 1 || counts["http::"] != 1 {
		t.Fatalf("candidate aggregation = %+v", counts)
	}

	allowedCandidate, err := NewPolicyCandidate(update)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := NewExactLearnedPolicyRule(allowedCandidate)
	if err != nil {
		t.Fatal(err)
	}
	items, err = PolicyCandidates([]PolicyDenial{update, deleteIssue}, []LearnedPolicyRule{allowed})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].GraphQLRootField != "deleteIssue" {
		t.Fatalf("exact GraphQL allow hid a sibling root coordinate: %+v", items)
	}
}

func TestGraphQLRulesAreExactAndExcludedFromCompaction(t *testing.T) {
	t.Parallel()
	rules := make([]LearnedPolicyRule, 0, 3)
	for index, root := range []string{"createIssue", "updateIssue", "deleteIssue"} {
		denial := validPolicyDenial()
		denial.RequestID = fmt.Sprintf("%032x", index+1)
		denial.Method = "POST"
		denial.Path = "/api/v1/graphql"
		denial.PolicyProtocolIdentity = PolicyProtocolIdentity{
			Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationMutation, GraphQLRootField: root,
		}
		candidate, err := NewPolicyCandidate(denial)
		if err != nil {
			t.Fatal(err)
		}
		rule, err := NewExactLearnedPolicyRule(candidate)
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	items, err := PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("GraphQL rules produced compaction candidates: %+v", items)
	}

	prefix := rules[0]
	prefix.Match = PolicyMatchPrefix
	prefix.Path = "/api/v1/"
	prefix.Examples = []string{"/api/v1/graphql"}
	prefix.ID = learnedRuleIDWithIdentity(
		prefix.Match, prefix.ContextID, prefix.ProjectID, prefix.Host, prefix.Port, prefix.Method, prefix.Path,
		prefix.Examples, prefix.SourceCandidates, prefix.PolicyProtocolIdentity,
	)
	if err := prefix.Validate(); err == nil {
		t.Fatal("GraphQL prefix rule was accepted")
	}
}

func TestPolicyCandidatesDeduplicateLatestEffectAndHideCoveredRules(t *testing.T) {
	t.Parallel()
	first := validPolicyDenial()
	latest := first
	latest.Timestamp = "2026-07-30T10:42:11Z"
	latest.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	other := first
	other.RequestID = "9185da2688d7469aae9cd9068e920b0b"
	other.Path = "/repos/cli/other"
	coveredCandidate, err := NewPolicyCandidate(other)
	if err != nil {
		t.Fatal(err)
	}
	coveredRule, err := NewExactLearnedPolicyRule(coveredCandidate)
	if err != nil {
		t.Fatal(err)
	}

	items, err := PolicyCandidates(
		[]PolicyDenial{first, latest, other}, []LearnedPolicyRule{coveredRule},
	)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := NewPolicyCandidate(latest)
	want.ObservationCount = 2
	if len(items) != 1 || items[0] != want {
		t.Fatalf("candidates = %+v, want %+v", items, want)
	}
	original, _ := NewPolicyCandidate(first)
	if original.ID != want.ID || original.ObservedAt == want.ObservedAt {
		t.Fatalf("repeated exact effect did not retain a stable ID with latest evidence: original=%+v latest=%+v", original, want)
	}
}

func TestPolicyCandidateLegacyObservationCountDefaultsToOne(t *testing.T) {
	t.Parallel()
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	candidate.ObservationCount = 0
	encoded, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PolicyCandidate
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("legacy candidate was rejected: %v", err)
	}
	if got := decoded.EffectiveObservationCount(); got != 1 {
		t.Fatalf("legacy observation count = %d, want 1", got)
	}
	candidate.ObservationCount = -1
	if err := candidate.Validate(); err == nil {
		t.Fatal("negative observation count was accepted")
	}
}

func TestResolvedPolicyCandidatesRemainOutsidePendingAggregation(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	repeated := denial
	repeated.Timestamp = "2026-07-30T10:42:11Z"
	repeated.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	for name, rules := range map[string]struct {
		allows []LearnedPolicyRule
		denies PolicyDenyRuleSet
	}{
		"allow": {
			allows: []LearnedPolicyRule{allow},
			denies: PolicyDenyRuleSet{Baseline: []PolicyBaselineDenyRule{}, Exact: []PolicyDenyRule{}},
		},
		"deny": {
			allows: []LearnedPolicyRule{},
			denies: PolicyDenyRuleSet{Baseline: []PolicyBaselineDenyRule{}, Exact: []PolicyDenyRule{deny}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			items, err := PolicyCandidatesWithDenyRules(
				[]PolicyDenial{denial, repeated}, rules.allows, rules.denies,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 0 {
				t.Fatalf("resolved %s candidate returned to pending aggregation: %+v", name, items)
			}
		})
	}
}

func TestPolicyCandidateAggregationKeepsExactEffectDimensionsDistinct(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*PolicyDenial){
		"project": func(value *PolicyDenial) { value.ProjectID = policyProjectB },
		"host":    func(value *PolicyDenial) { value.Host = "uploads.github.com" },
		"port":    func(value *PolicyDenial) { value.Port = 8443 },
		"method":  func(value *PolicyDenial) { value.Method = "POST" },
		"path":    func(value *PolicyDenial) { value.Path = "/repos/cli/other" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			first := validPolicyDenial()
			second := first
			second.RequestID = "8185da2688d7469aae9cd9068e920b0b"
			second.Timestamp = "2026-07-30T10:42:11Z"
			mutate(&second)
			items, err := PolicyCandidates([]PolicyDenial{first, second}, []LearnedPolicyRule{})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 2 {
				t.Fatalf("%s-distinct candidates = %+v", name, items)
			}
		})
	}
}

func TestPolicyCandidateAggregationUsesLatestEvidenceWithoutIdentityDrift(t *testing.T) {
	t.Parallel()
	first := validPolicyDenial()
	latest := first
	latest.Timestamp = "2026-07-30T10:42:11Z"
	latest.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	latest.Reason = "new bounded denial reason"
	latest.StatusCode = 429
	items, err := PolicyCandidates([]PolicyDenial{first, latest}, []LearnedPolicyRule{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ObservationCount != 2 || items[0].ObservedAt != latest.Timestamp ||
		items[0].Reason != latest.Reason || items[0].StatusCode != latest.StatusCode {
		t.Fatalf("aggregated candidate = %+v", items)
	}
}

func TestConcurrentPolicyObservationsConvergeToOneCandidate(t *testing.T) {
	t.Parallel()
	const observations = 64
	base := time.Date(2026, 8, 4, 20, 52, 0, 0, time.UTC)
	denials := make([]PolicyDenial, 0, observations)
	var mutex sync.Mutex
	var wait sync.WaitGroup
	for index := 0; index < observations; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			denial := validPolicyDenial()
			denial.Timestamp = base.Add(time.Duration(index) * time.Second).Format(time.RFC3339)
			denial.RequestID = fmt.Sprintf("%032x", index+1)
			mutex.Lock()
			denials = append(denials, denial)
			mutex.Unlock()
		}(index)
	}
	wait.Wait()

	items, err := PolicyCandidates(denials, []LearnedPolicyRule{})
	if err != nil {
		t.Fatal(err)
	}
	wantLatest := base.Add((observations - 1) * time.Second).Format(time.RFC3339)
	if len(items) != 1 || items[0].ObservationCount != observations || items[0].ObservedAt != wantLatest {
		t.Fatalf("concurrent observations = %+v, want one count=%d latest=%s", items, observations, wantLatest)
	}
}

func TestPolicyCandidatesHideBaselineAndExactDeniedEffects(t *testing.T) {
	t.Parallel()
	baseline := validPolicyDenial()
	baseline.Path = "/api/v1/secret"
	exact := validPolicyDenial()
	exact.Path = "/repos/cli/cli"
	candidate, err := NewPolicyCandidate(exact)
	if err != nil {
		t.Fatal(err)
	}
	exactRule, err := NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	items, err := PolicyCandidatesWithDenyRules(
		[]PolicyDenial{baseline, exact}, []LearnedPolicyRule{}, PolicyDenyRuleSet{
			Baseline: []PolicyBaselineDenyRule{{Host: baseline.Host, Method: baseline.Method, PathPrefix: "/api/v1/"}},
			Exact:    []PolicyDenyRule{exactRule},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("denied effects remained actionable: %+v", items)
	}
}

func TestPolicyDenyRuleBindsExactProjectAndRequest(t *testing.T) {
	t.Parallel()
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if !rule.Matches(candidate.ContextID, candidate.ProjectID, candidate.Host, candidate.Port, candidate.Method, candidate.Path) {
		t.Fatal("exact deny rule did not match its bound request")
	}
	if rule.Matches(candidate.ContextID, policyProjectB, candidate.Host, candidate.Port, candidate.Method, candidate.Path) {
		t.Fatal("exact deny rule crossed project boundary")
	}
	if rule.Matches(candidate.ContextID, candidate.ProjectID, candidate.Host, 8443, candidate.Method, candidate.Path) {
		t.Fatal("exact deny rule crossed port boundary")
	}
}

func TestPolicyCandidatesExcludeDenialsThatExactRulesCannotResolve(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	denial.Learnable = false
	items, err := PolicyCandidates(
		[]PolicyDenial{denial}, []LearnedPolicyRule{},
	)
	if err != nil || len(items) != 0 {
		t.Fatalf("ineligible denial candidates = %+v, error = %v", items, err)
	}
	if _, err := NewPolicyCandidate(denial); err == nil {
		t.Fatal("ineligible denial became a direct policy candidate")
	}
}

func TestExactLearnedRuleBindsCandidateAndDoesNotBroadenPath(t *testing.T) {
	t.Parallel()
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	if !rule.Matches(candidate.ContextID, candidate.ProjectID, candidate.Host, candidate.Port, candidate.Method, candidate.Path) {
		t.Fatal("exact rule did not match its approved effect")
	}
	for _, changed := range []struct {
		host, method, path string
		port               int
	}{
		{"uploads.github.com", candidate.Method, candidate.Path, candidate.Port},
		{candidate.Host, "POST", candidate.Path, candidate.Port},
		{candidate.Host, candidate.Method, candidate.Path + "/child", candidate.Port},
	} {
		if rule.Matches(candidate.ContextID, candidate.ProjectID, changed.host, changed.port, changed.method, changed.path) {
			t.Fatalf("exact rule broadened to %+v", changed)
		}
	}
	if rule.Matches(candidate.ContextID, candidate.ProjectID, candidate.Host, 8443, candidate.Method, candidate.Path) {
		t.Fatal("exact rule broadened to another port")
	}
	if rule.Matches(candidate.ContextID, policyProjectB, candidate.Host, candidate.Port, candidate.Method, candidate.Path) {
		t.Fatal("exact rule crossed the project boundary")
	}
	rule.Path += "/changed"
	if err := rule.Validate(); err == nil {
		t.Fatal("content-mismatched learned rule ID was accepted")
	}
}

func TestPolicyCandidateIdentityIncludesProjectPrincipal(t *testing.T) {
	t.Parallel()
	first := validPolicyDenial()
	second := first
	second.ProjectID = policyProjectB
	firstCandidate, err := NewPolicyCandidate(first)
	if err != nil {
		t.Fatal(err)
	}
	secondCandidate, err := NewPolicyCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstCandidate.ID == secondCandidate.ID {
		t.Fatal("project-scoped candidates share an opaque ID")
	}
}

func TestPolicyOpaqueReferencesIncludeContextAuthority(t *testing.T) {
	t.Parallel()
	first := validPolicyDenial()
	second := first
	second.ContextID = policyContextB
	second.ContextName = "restricted"
	firstCandidate, err := NewPolicyCandidate(first)
	if err != nil {
		t.Fatal(err)
	}
	secondCandidate, err := NewPolicyCandidate(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstCandidate.ID == secondCandidate.ID {
		t.Fatal("Context-scoped candidates share an opaque ID")
	}
	firstRule, err := NewExactLearnedPolicyRule(firstCandidate)
	if err != nil {
		t.Fatal(err)
	}
	secondRule, err := NewExactLearnedPolicyRule(secondCandidate)
	if err != nil {
		t.Fatal(err)
	}
	firstDeny, err := NewExactPolicyDenyRule(firstCandidate)
	if err != nil {
		t.Fatal(err)
	}
	secondDeny, err := NewExactPolicyDenyRule(secondCandidate)
	if err != nil {
		t.Fatal(err)
	}
	if firstRule.ID == secondRule.ID || firstDeny.ID == secondDeny.ID {
		t.Fatal("Context-scoped policy decisions share an opaque ID")
	}
	if firstRule.Matches(second.ContextID, second.ProjectID, second.Host, second.Port, second.Method, second.Path) ||
		firstDeny.Matches(second.ContextID, second.ProjectID, second.Host, second.Port, second.Method, second.Path) {
		t.Fatal("Context A decision matched Context B authority")
	}
	firstRules := []LearnedPolicyRule{
		exactRuleForPath(t, "1185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
		exactRuleForPath(t, "2185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
		exactRuleForPath(t, "3185da2688d7469aae9cd9068e920b0b", "/api/v1/items/three"),
	}
	secondRules := append([]LearnedPolicyRule{}, firstRules...)
	for index := range secondRules {
		secondRules[index].ContextID = policyContextB
		secondRules[index].ContextName = "restricted"
		secondRules[index].ID = learnedRuleID(
			secondRules[index].Match, secondRules[index].ContextID, secondRules[index].ProjectID,
			secondRules[index].Host, secondRules[index].Port, secondRules[index].Method,
			secondRules[index].Path, secondRules[index].Examples, secondRules[index].SourceCandidates,
		)
	}
	firstCompactions, err := PolicyCompactions(firstRules)
	if err != nil {
		t.Fatal(err)
	}
	secondCompactions, err := PolicyCompactions(secondRules)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstCompactions) != 1 || len(secondCompactions) != 1 || firstCompactions[0].ID == secondCompactions[0].ID {
		t.Fatalf("Context-scoped compactions = %+v / %+v", firstCompactions, secondCompactions)
	}
}

func exactRuleForPath(t *testing.T, requestID, path string) LearnedPolicyRule {
	t.Helper()
	denial := validPolicyDenial()
	denial.RequestID = requestID
	denial.Host = "api.example.com"
	denial.Path = path
	candidate, err := NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func TestPolicyCompactionRequiresThreeSpecificSiblingRules(t *testing.T) {
	t.Parallel()
	rules := []LearnedPolicyRule{
		exactRuleForPath(t, "1185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
		exactRuleForPath(t, "2185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
	}
	items, err := PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("two-rule compactions = %+v", items)
	}

	rules = append(
		rules,
		exactRuleForPath(t, "3185da2688d7469aae9cd9068e920b0b", "/api/v1/items/three"),
	)
	items, err = PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].PathPrefix != "/api/v1/items/" ||
		items[0].OutsideCanary != "/api/v1/items-outside-tobari-canary" {
		t.Fatalf("compactions = %+v", items)
	}

	shallow := []LearnedPolicyRule{
		exactRuleForPath(t, "4185da2688d7469aae9cd9068e920b0b", "/items/one"),
		exactRuleForPath(t, "5185da2688d7469aae9cd9068e920b0b", "/items/two"),
		exactRuleForPath(t, "6185da2688d7469aae9cd9068e920b0b", "/items/three"),
	}
	items, err = PolicyCompactions(shallow)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unsafe shallow compactions = %+v", items)
	}

	for name, unsafePath := range map[string]string{
		"encoded separator": "/api/v1/items/%2Fadmin",
		"dot segment":       "/api/v1/items/../admin",
		"empty segment":     "/api/v1/items//admin",
		"backslash":         `/api/v1/items\admin`,
	} {
		t.Run(name, func(t *testing.T) {
			rules := []LearnedPolicyRule{
				exactRuleForPath(t, "7185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
				exactRuleForPath(t, "8185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
				exactRuleForPath(t, "9185da2688d7469aae9cd9068e920b0b", unsafePath),
			}
			items, err := PolicyCompactions(rules)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 0 {
				t.Fatalf("unsafe path produced compaction: %+v", items)
			}
		})
	}
}

func TestCompactLearnedPolicyRulesPreservesExamplesAndRejectsStaleReference(t *testing.T) {
	t.Parallel()
	rules := []LearnedPolicyRule{
		exactRuleForPath(t, "1185da2688d7469aae9cd9068e920b0b", "/api/v1/items/one"),
		exactRuleForPath(t, "2185da2688d7469aae9cd9068e920b0b", "/api/v1/items/two"),
		exactRuleForPath(t, "3185da2688d7469aae9cd9068e920b0b", "/api/v1/items/three"),
	}
	items, err := PolicyCompactions(rules)
	if err != nil {
		t.Fatal(err)
	}
	updated, selected, prefixRule, err := CompactLearnedPolicyRules(rules, items[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated) != 1 || prefixRule.Match != PolicyMatchPrefix ||
		len(prefixRule.Examples) != 3 || len(prefixRule.SourceCandidates) != 3 ||
		selected.OutsideCanary != "/api/v1/items-outside-tobari-canary" {
		t.Fatalf("updated=%+v selected=%+v rule=%+v", updated, selected, prefixRule)
	}
	for _, example := range prefixRule.Examples {
		if !prefixRule.Matches(prefixRule.ContextID, prefixRule.ProjectID, prefixRule.Host, prefixRule.Port, prefixRule.Method, example) {
			t.Fatalf("prefix rule lost example %q", example)
		}
	}
	if prefixRule.Matches(
		prefixRule.ContextID, prefixRule.ProjectID, prefixRule.Host, prefixRule.Port, prefixRule.Method, selected.OutsideCanary,
	) {
		t.Fatal("prefix rule matched its outside boundary canary")
	}
	if _, _, _, err := CompactLearnedPolicyRules(
		updated, selected.ID,
	); err == nil {
		t.Fatal("stale compaction reference was accepted")
	}
}

func TestPolicyCandidateRejectsControlPathAndOpaqueKindMismatch(t *testing.T) {
	t.Parallel()
	denial := validPolicyDenial()
	denial.Path = "/safe\nunsafe"
	if _, err := NewPolicyCandidate(denial); err == nil {
		t.Fatal("control-bearing path became a candidate")
	}
	if err := ValidatePolicyCandidateID("pcx_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("compaction reference was accepted as a candidate")
	}
	if err := ValidatePolicyCompactionID("pcy_0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("candidate reference was accepted as a compaction")
	}
}

func TestPolicyReviewDecisionSetRequiresBoundedUniqueOpaqueChoices(t *testing.T) {
	t.Parallel()
	valid := PolicyReviewDecisionSet{Decisions: []PolicyReviewDecision{
		{CandidateID: "pcy_0123456789abcdef0123456789abcdef", Decision: PolicyDecisionAllow},
		{CandidateID: "pcy_abcdef0123456789abcdef0123456789", Decision: PolicyDecisionDeny},
	}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]PolicyReviewDecisionSet{
		"empty": {},
		"duplicate": {Decisions: []PolicyReviewDecision{
			valid.Decisions[0], valid.Decisions[0],
		}},
		"invalid decision": {Decisions: []PolicyReviewDecision{{
			CandidateID: valid.Decisions[0].CandidateID, Decision: "prompt",
		}}},
		"wrong reference kind": {Decisions: []PolicyReviewDecision{{
			CandidateID: "pcx_0123456789abcdef0123456789abcdef", Decision: PolicyDecisionAllow,
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := candidate.Validate(); err == nil {
				t.Fatalf("invalid reviewed set was accepted: %+v", candidate)
			}
		})
	}
}

func TestPolicyReviewChangeRequiresExactOrderedReceiptAndActiveRevision(t *testing.T) {
	t.Parallel()
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	receipt := PolicyReviewAppliedDecision{
		CandidateID: candidate.ID, Decision: PolicyDecisionAllow,
		ContextID: candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot,
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		Host:                   candidate.Host, Port: candidate.Port, Method: candidate.Method, Path: candidate.Path,
	}
	valid := PolicyReviewChange{
		Task: TaskPolicyReviewApply, PolicyDirectory: "/tmp/tobari/policy",
		AllowCount: 1, DenyCount: 0, Applied: true,
		ActiveRevision: strings.Repeat("a", 64), Decisions: []PolicyReviewAppliedDecision{receipt},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*PolicyReviewChange){
		"missing decisions": func(change *PolicyReviewChange) { change.Decisions = nil },
		"missing revision":  func(change *PolicyReviewChange) { change.ActiveRevision = "" },
		"count mismatch":    func(change *PolicyReviewChange) { change.AllowCount = 0 },
		"wrong reference kind": func(change *PolicyReviewChange) {
			change.Decisions[0].CandidateID = "pcx_0123456789abcdef0123456789abcdef"
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := valid
			invalid.Decisions = append([]PolicyReviewAppliedDecision{}, valid.Decisions...)
			mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatalf("invalid reviewed receipt was accepted: %+v", invalid)
			}
		})
	}
}

func TestCurrentPolicyRulesListsReversibleAllowAndDenyDecisions(t *testing.T) {
	t.Parallel()
	allowCandidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	allow, err := NewExactLearnedPolicyRule(allowCandidate)
	if err != nil {
		t.Fatal(err)
	}
	denyDenial := validPolicyDenial()
	denyDenial.Path = "/repos/cli/other"
	denyCandidate, err := NewPolicyCandidate(denyDenial)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewExactPolicyDenyRule(denyCandidate)
	if err != nil {
		t.Fatal(err)
	}

	items, err := CurrentPolicyRules([]LearnedPolicyRule{allow}, []PolicyDenyRule{deny})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Decision != PolicyDecisionAllow || items[1].Decision != PolicyDecisionDeny {
		t.Fatalf("policy decisions = %+v", items)
	}
	if items[0].ID != allow.ID || items[1].ID != deny.ID {
		t.Fatalf("policy decision IDs = %+v, want allow=%s deny=%s", items, allow.ID, deny.ID)
	}
	if items[1].Examples == nil || len(items[1].Examples) != 0 {
		t.Fatalf("exact deny examples = %#v, want known empty collection", items[1].Examples)
	}
	report := PolicyRuleReport{
		Task: TaskPolicyRules, PolicyDirectory: "/tmp/policy", Items: items,
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePolicyRuleReturnsDefaultDenyAndLeavesOtherDecisions(t *testing.T) {
	t.Parallel()
	allowCandidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	allow, err := NewExactLearnedPolicyRule(allowCandidate)
	if err != nil {
		t.Fatal(err)
	}
	denyDenial := validPolicyDenial()
	denyDenial.Path = "/repos/cli/other"
	denyCandidate, err := NewPolicyCandidate(denyDenial)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := NewExactPolicyDenyRule(denyCandidate)
	if err != nil {
		t.Fatal(err)
	}

	updatedAllow, updatedDenies, removed, err := RemovePolicyRule(
		[]LearnedPolicyRule{allow}, []PolicyDenyRule{deny}, allow.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Decision != PolicyDecisionAllow || len(updatedAllow) != 0 || len(updatedDenies) != 1 || updatedDenies[0].ID != deny.ID {
		t.Fatalf("allow reset = removed:%+v learned:%+v denies:%+v", removed, updatedAllow, updatedDenies)
	}

	updatedAllow, updatedDenies, removed, err = RemovePolicyRule(updatedAllow, updatedDenies, deny.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Decision != PolicyDecisionDeny || len(updatedAllow) != 0 || len(updatedDenies) != 0 {
		t.Fatalf("deny reset = removed:%+v learned:%+v denies:%+v", removed, updatedAllow, updatedDenies)
	}
	if _, _, _, err := RemovePolicyRule(updatedAllow, updatedDenies, deny.ID); err == nil {
		t.Fatal("stale policy rule was reset")
	}
	baselineID := "pdr_0123456789abcdef0123456789abcdef"
	if _, _, _, err := RemovePolicyRule(nil, []PolicyDenyRule{}, baselineID); err == nil {
		t.Fatal("non-current policy rule was reset")
	}
}
