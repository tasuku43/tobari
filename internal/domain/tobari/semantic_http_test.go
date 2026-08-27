package tobari

import "testing"

func semanticHTTPTestEffect(protocol string) SemanticRequestEffect {
	identity := PolicyProtocolIdentity{Scheme: "https", Protocol: protocol}
	if protocol == PolicyProtocolGraphQL {
		identity.GraphQLOperationType = GraphQLOperationQuery
		identity.GraphQLRootField = "viewer"
	}
	return SemanticRequestEffect{
		Scheme: "https", Host: "api.vendor.dev", Port: 443, Method: "GET", Path: "/v1/items/abc", Identity: identity,
	}
}

func TestSemanticHTTPRuleAuthorityRequiresHostXORHosts(t *testing.T) {
	valid := []SemanticRuleAuthority{
		{Scheme: "https", Host: "api.vendor.dev", Port: 443},
		{Scheme: "https", Hosts: []string{"api.vendor.dev", "uploads.vendor.dev"}, Port: 443},
	}
	for _, authority := range valid {
		if err := authority.Validate(); err != nil {
			t.Fatalf("Validate(%+v) error = %v", authority, err)
		}
	}
	invalid := []SemanticRuleAuthority{
		{Scheme: "https", Port: 443},
		{Scheme: "https", Host: "api.vendor.dev", Hosts: []string{}, Port: 443},
		{Scheme: "https", Hosts: []string{}, Port: 443},
		{Scheme: "https", Hosts: []string{"api.vendor.dev"}, Port: 443},
		{Scheme: "https", Hosts: []string{"api.vendor.dev", "api.vendor.dev"}, Port: 443},
		{Scheme: "http", Host: "api.vendor.dev", Port: 80},
		{Scheme: "http", Host: "localhost", Port: 80},
	}
	for _, authority := range invalid {
		if err := authority.Validate(); err == nil {
			t.Fatalf("Validate(%+v) succeeded", authority)
		}
	}
}

func TestSemanticHTTPRuleMatchesExactAndOneTerminalIdentifier(t *testing.T) {
	effect := semanticHTTPTestEffect(PolicyProtocolHTTP)
	for _, rule := range []SemanticHTTPRule{
		{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: effect.Host, Port: 443}, Method: "GET", Path: effect.Path},
		{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Hosts: []string{"other.vendor.dev", effect.Host}, Port: 443}, Method: "GET", Path: "/v1/items/{id}"},
	} {
		if err := rule.Validate(); err != nil {
			t.Fatalf("Validate(%+v) error = %v", rule, err)
		}
		if !rule.Matches(effect) {
			t.Fatalf("rule %+v did not match %+v", rule, effect)
		}
	}
	for _, path := range []string{"/{id}", "/v1/{id}/detail", "/v1/{id}/{id}", "/v1/items/prefix-{id}"} {
		rule := SemanticHTTPRule{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: effect.Host, Port: 443}, Method: "GET", Path: path}
		if err := rule.Validate(); err == nil {
			t.Fatalf("Validate(path=%q) succeeded", path)
		}
	}
}

func TestSemanticHTTPRuleNeverMatchesClassifiedRequest(t *testing.T) {
	rule := SemanticHTTPRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443},
		Method:                "GET", Path: "/v1/items/{id}",
	}
	if rule.Matches(semanticHTTPTestEffect(PolicyProtocolGraphQL)) {
		t.Fatal("generic HTTP rule matched a classified GraphQL request")
	}
}

func TestSemanticHTTPPolicyRejectsDuplicatesAndExactShadowing(t *testing.T) {
	rule := SemanticHTTPRule{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Method: "GET", Path: "/v1/items"}
	if err := (SemanticHTTPPolicy{
		Allow: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{rule}},
		Deny:  SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{rule}},
	}).Validate(); err == nil {
		t.Fatal("exact Allow/Deny shadowing was accepted")
	}
	if err := (SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{rule, rule}}).Validate(); err == nil {
		t.Fatal("duplicate semantic HTTP rules were accepted")
	}
}

func TestSemanticHTTPPolicyRejectsFullyCoveredAllows(t *testing.T) {
	allow := SemanticHTTPRule{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Method: "GET", Path: "/v1/items/abc"}
	for _, deny := range []SemanticHTTPRule{
		{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Method: "GET", Path: "/v1/items/{id}"},
		{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Hosts: []string{"api.vendor.dev", "uploads.vendor.dev"}, Port: 443}, Method: "GET", Path: "/v1/items/{id}"},
	} {
		if err := (SemanticHTTPPolicy{Allow: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{allow}}, Deny: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{deny}}}).Validate(); err == nil {
			t.Fatalf("fully covered Allow accepted for Deny %+v", deny)
		}
	}
	partial := SemanticHTTPPolicy{
		Allow: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Hosts: []string{"api.vendor.dev", "uploads.vendor.dev"}, Port: 443}, Method: "GET", Path: "/v1/items/abc"}}},
		Deny:  SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{allow}},
	}
	if err := partial.Validate(); err != nil {
		t.Fatalf("partially shadowed Allow rejected: %v", err)
	}
	combined := SemanticHTTPPolicy{
		Allow: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Hosts: []string{"api.vendor.dev", "uploads.vendor.dev"}, Port: 443}, Method: "GET", Path: "/v1/items/abc"}}},
		Deny: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Method: "GET", Path: "/v1/items/{id}"},
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "uploads.vendor.dev", Port: 443}, Method: "GET", Path: "/v1/items/abc"},
		}},
	}
	if err := combined.Validate(); err == nil {
		t.Fatal("Allow fully covered by the union of Deny rules was accepted")
	}
}

func TestPolicyMemoryExactMatchUsesCompleteSemanticEffect(t *testing.T) {
	candidate, err := NewPolicyCandidate(validPolicyDenial())
	if err != nil {
		t.Fatal(err)
	}
	rule, err := NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	if !rule.MatchesIdentity(candidate.WorkspaceManifestID, candidate.ProjectID, candidate.Host, candidate.Port, candidate.Method, candidate.Path, candidate.PolicyProtocolIdentity) {
		t.Fatal("exact rule did not match its source effect")
	}
	changedScheme := candidate.PolicyProtocolIdentity
	changedScheme.Scheme = "http"
	if rule.MatchesIdentity(candidate.WorkspaceManifestID, candidate.ProjectID, candidate.Host, candidate.Port, candidate.Method, candidate.Path, changedScheme) {
		t.Fatal("exact rule ignored a changed scheme")
	}
}
