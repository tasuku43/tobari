package tobari

import "testing"

func graphqlRule(host, root string) SemanticGraphQLRule {
	return SemanticGraphQLRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		Path:                  "/graphql", OperationType: GraphQLOperationQuery, RootField: root,
	}
}

func TestSemanticGraphQLRuleMatchesOnlyCompleteClassifiedEffect(t *testing.T) {
	rule := graphqlRule("api.vendor.dev", "viewer")
	effect := SemanticRequestEffect{
		Scheme: "https", Host: "api.vendor.dev", Port: 443, Method: "POST", Path: "/graphql",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "viewer"},
	}
	if !rule.Matches(effect) {
		t.Fatal("GraphQL rule did not match its complete classified effect")
	}
	for name, mutate := range map[string]func(*SemanticRequestEffect){
		"scheme": func(value *SemanticRequestEffect) {
			value.Scheme, value.Identity.Scheme = "http", "http"
		},
		"host":      func(value *SemanticRequestEffect) { value.Host = "other.vendor.dev" },
		"port":      func(value *SemanticRequestEffect) { value.Port = 8443 },
		"method":    func(value *SemanticRequestEffect) { value.Method = "GET" },
		"path":      func(value *SemanticRequestEffect) { value.Path = "/other" },
		"root":      func(value *SemanticRequestEffect) { value.Identity.GraphQLRootField = "issues" },
		"operation": func(value *SemanticRequestEffect) { value.Identity.GraphQLOperationType = GraphQLOperationMutation },
		"generic": func(value *SemanticRequestEffect) {
			value.Identity = PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := effect
			mutate(&changed)
			if rule.Matches(changed) {
				t.Fatalf("GraphQL rule matched changed effect %+v", changed)
			}
		})
	}
	multiHost := rule
	multiHost.Host = ""
	multiHost.Hosts = []string{"api.vendor.dev", "uploads.vendor.dev"}
	if !multiHost.Matches(effect) {
		t.Fatal("GraphQL hosts rule did not match a declared host")
	}
	mutation := rule
	mutation.OperationType, mutation.RootField = GraphQLOperationMutation, "updateIssue"
	mutationEffect := effect
	mutationEffect.Identity.GraphQLOperationType, mutationEffect.Identity.GraphQLRootField = GraphQLOperationMutation, "updateIssue"
	if !mutation.Matches(mutationEffect) {
		t.Fatal("GraphQL mutation rule did not match its complete effect")
	}
}

func TestSemanticGraphQLPolicyRequiresTrustedEndpoint(t *testing.T) {
	rule := graphqlRule("api.vendor.dev", "viewer")
	policy := SemanticGraphQLPolicy{
		Endpoints: []SemanticHTTPEndpoint{{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Path: "/graphql"}},
		Allow:     SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{rule}},
		Deny:      SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{}},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("valid GraphQL policy: %v", err)
	}
	policy.Endpoints[0].Path = "/other"
	if err := policy.Validate(); err == nil {
		t.Fatal("GraphQL rule without a trusted endpoint was accepted")
	}
}

func TestSemanticGraphQLEndpointSetUnionDeclaresMultiHostRule(t *testing.T) {
	rule := graphqlRule("api.vendor.dev", "viewer")
	rule.Host = ""
	rule.Hosts = []string{"api.vendor.dev", "uploads.vendor.dev"}
	policy := SemanticGraphQLPolicy{
		Endpoints: []SemanticHTTPEndpoint{
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "api.vendor.dev", Port: 443}, Path: "/graphql"},
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "uploads.vendor.dev", Port: 443}, Path: "/graphql"},
		},
		Allow: SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{rule}},
		Deny:  SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{}},
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("endpoint set union did not declare multi-host rule: %v", err)
	}
}

func TestSemanticGraphQLPolicyRejectsMalformedDuplicatesAndCombinedShadow(t *testing.T) {
	ruleA := graphqlRule("api.vendor.dev", "viewer")
	ruleB := graphqlRule("uploads.vendor.dev", "viewer")
	allow := ruleA
	allow.Host = ""
	allow.Hosts = []string{"api.vendor.dev", "uploads.vendor.dev"}
	policy := SemanticGraphQLPolicy{
		Endpoints: []SemanticHTTPEndpoint{{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Hosts: []string{"api.vendor.dev", "uploads.vendor.dev"}, Port: 443}, Path: "/graphql"}},
		Allow:     SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{allow}},
		Deny:      SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{ruleA, ruleB}},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("GraphQL Allow fully covered by combined Deny rules was accepted")
	}
	if err := (SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{ruleA, ruleA}}).Validate(); err == nil {
		t.Fatal("duplicate GraphQL rules were accepted")
	}
	reorderedA := allow
	reorderedB := allow
	reorderedB.Hosts = []string{"uploads.vendor.dev", "api.vendor.dev"}
	if err := (SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{reorderedA, reorderedB}}).Validate(); err == nil {
		t.Fatal("reordered duplicate GraphQL rules were accepted")
	}
	malformed := ruleA
	malformed.OperationType = "subscription"
	if err := malformed.Validate(); err == nil {
		t.Fatal("unsupported GraphQL operation was accepted")
	}
	policy.Endpoints = nil
	if err := policy.Validate(); err == nil {
		t.Fatal("unknown GraphQL endpoint collection was accepted")
	}
}
