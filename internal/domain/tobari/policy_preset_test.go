package tobari

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuiltinPolicyPresetsHaveStableRevisions(t *testing.T) {
	for _, origin := range []string{"builtin/offline", DefaultPolicyPresetOrigin, "builtin/reviewed-exact", "builtin/get-only-reviewed"} {
		preset, ok := BuiltinPolicyPreset(origin)
		if !ok {
			t.Fatalf("missing built-in %q", origin)
		}
		normalized, first, revision, err := NormalizePolicyPreset(preset)
		if err != nil {
			t.Fatal(err)
		}
		_, second, repeated, err := NormalizePolicyPreset(normalized)
		if err != nil || !bytes.Equal(first, second) || revision != repeated || !strings.HasPrefix(revision, "sha256:") {
			t.Fatalf("built-in normalization = %q/%q grants=%d err=%v", revision, repeated, len(normalized.BaselineGrants), err)
		}
		if origin == DefaultPolicyPresetOrigin && len(normalized.BaselineGrants) == 0 {
			t.Fatal("agent-ready preset has no reviewed core grants")
		}
		if origin != DefaultPolicyPresetOrigin && len(normalized.BaselineGrants) != 0 {
			t.Fatalf("strict built-in %q unexpectedly grants %d effects", origin, len(normalized.BaselineGrants))
		}
	}
}

func TestHTTPOnlyPresetNormalizationRemainsCompatibleWithoutSemanticKeys(t *testing.T) {
	preset, _ := BuiltinPolicyPreset("builtin/reviewed-exact")
	preset.BaselineGrants = []PolicyPresetExactRule{{Scheme: "https", Host: "api.github.com", Port: 443, Method: "POST", Path: "/graphql"}}
	_, first, revision, err := NormalizePolicyPreset(preset)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(first, []byte(`"protocol"`)) || bytes.Contains(first, []byte(`"graphql_operation_type"`)) || bytes.Contains(first, []byte(`"graphql_root_field"`)) {
		t.Fatalf("HTTP-only normalized snapshot gained semantic keys: %s", first)
	}
	var decoded PolicyPreset
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatal(err)
	}
	_, second, repeated, err := NormalizePolicyPreset(decoded)
	if err != nil || !bytes.Equal(first, second) || revision != repeated {
		t.Fatalf("HTTP-only snapshot did not round trip byte-identically: revisions=%q/%q err=%v", revision, repeated, err)
	}
}

func TestAgentReadyPresetPreservesNativeDiscoveryAndSeparatesMCPExecution(t *testing.T) {
	preset, ok := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	if !ok {
		t.Fatal("agent-ready preset is missing")
	}
	want := map[string]bool{
		"GET api.anthropic.com/api/claude_cli/bootstrap":        true,
		"GET api.anthropic.com/api/oauth/claude_cli/roles":      true,
		"GET api.anthropic.com/api/oauth/profile":               true,
		"GET api.anthropic.com/api/oauth/usage":                 true,
		"POST api.anthropic.com/v1/messages":                    true,
		"GET platform.claude.com/v1/oauth/hello":                true,
		"POST platform.claude.com/v1/oauth/token":               true,
		"POST auth.openai.com/api/accounts/deviceauth/usercode": true,
		"POST auth.openai.com/api/accounts/deviceauth/token":    true,
		"POST auth.openai.com/oauth/token":                      true,
		"GET chatgpt.com/backend-api/codex/models":              true,
		"POST chatgpt.com/backend-api/codex/responses":          true,
		"POST ab.chatgpt.com/otlp/v1/metrics":                   true,
	}
	for _, rule := range preset.BaselineGrants {
		delete(want, rule.Method+" "+rule.Host+rule.Path)
	}
	if len(want) != 0 {
		t.Fatalf("agent-ready core grants are missing: %v", want)
	}
	for _, required := range []string{"GET api.anthropic.com/mcp-registry/v0/servers", "GET api.anthropic.com/api/claude_code_penguin_mode", "GET chatgpt.com/backend-api/connectors/directory/list", "GET chatgpt.com/backend-api/ps/plugins/installed"} {
		found := false
		for _, rule := range preset.BaselineGrants {
			found = found || rule.Method+" "+rule.Host+rule.Path == required
		}
		if !found {
			t.Fatalf("native discovery grant missing: %s", required)
		}
	}
	if len(preset.BaselineTemplates) != 1 || preset.BaselineTemplates[0].Path != "/api/eval/{id}" {
		t.Fatalf("Claude eval template = %+v", preset.BaselineTemplates)
	}
	if len(preset.MCPEndpoints) != 1 || len(preset.MCPBaselineGrants) == 0 {
		t.Fatalf("MCP boundary = endpoints:%+v grants:%+v", preset.MCPEndpoints, preset.MCPBaselineGrants)
	}
	for _, rule := range preset.MCPBaselineGrants {
		if rule.MCPMethod == "tools/call" || rule.MCPToolName != "" {
			t.Fatalf("MCP action entered baseline: %+v", rule)
		}
	}
	for _, forbidden := range []string{"upload", "download", "release", "update"} {
		for _, rule := range preset.BaselineGrants {
			if strings.Contains(strings.ToLower(rule.Host+rule.Path), forbidden) {
				t.Fatalf("optional surface %q entered agent-ready baseline: %+v", forbidden, rule)
			}
		}
	}
}

func TestAgentReadyNativeToolAuthReadinessBundlesArePinnedAndExact(t *testing.T) {
	bundles := nativeToolAuthReadinessBundles()
	wantVersions := map[string]string{
		"claude_ready": AgentReadyClaudeVersion,
		"codex_ready":  AgentReadyCodexVersion,
		"gh_ready":     AgentReadyGitHubCLIVersion,
	}
	for _, bundle := range bundles {
		version, ok := wantVersions[bundle.ID]
		if !ok || bundle.Version != version || len(bundle.BaselineGrants) == 0 {
			t.Fatalf("native tool readiness bundle is not pinned: %+v", bundle)
		}
		delete(wantVersions, bundle.ID)
	}
	if len(wantVersions) != 0 {
		t.Fatalf("native tool readiness bundles are missing: %v", wantVersions)
	}

	var githubHTTP []PolicyPresetExactRule
	var githubEndpoints []PolicyPresetExactRule
	var githubGraphQL []PolicyPresetExactRule
	for _, bundle := range bundles {
		if bundle.ID == "gh_ready" {
			for _, rule := range bundle.BaselineGrants {
				if rule.Protocol == PolicyProtocolGraphQL {
					githubGraphQL = append(githubGraphQL, rule)
				} else {
					githubHTTP = append(githubHTTP, rule)
				}
			}
			githubEndpoints = bundle.GraphQLEndpoints
		}
	}
	wantGitHub := []PolicyPresetExactRule{
		{Scheme: "https", Host: "github.com", Port: 443, Method: "POST", Path: "/login/device/code"},
		{Scheme: "https", Host: "github.com", Port: 443, Method: "POST", Path: "/login/oauth/access_token"},
	}
	if len(githubHTTP) != len(wantGitHub) {
		t.Fatalf("gh_ready HTTP grants = %+v", githubHTTP)
	}
	for index := range wantGitHub {
		if githubHTTP[index] != wantGitHub[index] {
			t.Fatalf("gh_ready HTTP grant %d = %+v, want %+v", index, githubHTTP[index], wantGitHub[index])
		}
	}
	wantEndpoint := PolicyPresetExactRule{Scheme: "https", Host: "api.github.com", Port: 443, Method: "POST", Path: "/graphql"}
	wantGraphQL := PolicyPresetExactRule{Scheme: "https", Host: "api.github.com", Port: 443, Method: "POST", Path: "/graphql", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "viewer"}
	if len(githubEndpoints) != 1 || githubEndpoints[0] != wantEndpoint || len(githubGraphQL) != 1 || githubGraphQL[0] != wantGraphQL {
		t.Fatalf("gh_ready semantic grants = endpoints:%+v grants:%+v", githubEndpoints, githubGraphQL)
	}
	for _, rule := range agentReadyBaselineGrants() {
		if rule.Host == "github.com" && (rule.Method != "POST" || (rule.Path != "/login/device/code" && rule.Path != "/login/oauth/access_token")) {
			t.Fatalf("neighboring GitHub authority entered baseline: %+v", rule)
		}
	}
}

func TestPolicyPresetGraphQLBaselineGrantRequiresExactDeclaredEndpointAndIdentity(t *testing.T) {
	preset, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	if err := preset.Validate(); err != nil {
		t.Fatalf("agent-ready semantic grant rejected: %v", err)
	}

	semanticIndex := -1
	for index, rule := range preset.BaselineGrants {
		if rule.Protocol == PolicyProtocolGraphQL {
			semanticIndex = index
		}
	}
	if semanticIndex < 0 {
		t.Fatal("agent-ready GraphQL baseline grant is missing")
	}
	for name, mutate := range map[string]func(*PolicyPreset){
		"missing endpoint":      func(p *PolicyPreset) { p.GraphQLEndpoints = []PolicyPresetExactRule{} },
		"unsupported operation": func(p *PolicyPreset) { p.BaselineGrants[semanticIndex].GraphQLOperationType = "subscription" },
		"invalid root":          func(p *PolicyPreset) { p.BaselineGrants[semanticIndex].GraphQLRootField = "viewer-login" },
		"route mismatch":        func(p *PolicyPreset) { p.BaselineGrants[semanticIndex].Path = "/graphql/v2" },
		"semantic endpoint declaration": func(p *PolicyPreset) {
			p.GraphQLEndpoints[0].Protocol = PolicyProtocolGraphQL
			p.GraphQLEndpoints[0].GraphQLOperationType = GraphQLOperationQuery
			p.GraphQLEndpoints[0].GraphQLRootField = "viewer"
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := preset
			candidate.GraphQLEndpoints = append([]PolicyPresetExactRule{}, preset.GraphQLEndpoints...)
			candidate.BaselineGrants = append([]PolicyPresetExactRule{}, preset.BaselineGrants...)
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("unsafe GraphQL baseline grant accepted")
			}
		})
	}
}

func TestPolicyPresetRejectsNonExactOrReservedDestinations(t *testing.T) {
	for _, host := range []string{"mock-upstream", "*.example.com", "127.0.0.1", "::1", "localhost", "service.local", "service.internal", "service.test", "service.invalid", "service.example", "example.com", "UPPER.example.net", "bad_name.example.net", "trailing.example.net.", "é.example.net"} {
		authority := PolicyPresetAuthority{Scheme: "https", Host: host, Port: 443}
		if err := authority.Validate(); err == nil {
			t.Fatalf("unsafe destination %q was accepted", host)
		}
	}
	if err := (PolicyPresetAuthority{Scheme: "https", Host: "api.github.com", Port: 443}).Validate(); err != nil {
		t.Fatalf("public exact destination rejected: %v", err)
	}
}

func TestPolicyPresetRejectsDuplicatesAndExecutableShapedUnknownDataAtStrictDecoder(t *testing.T) {
	preset, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	preset.MethodCeiling = PolicyPresetMethodCeiling{Mode: "exact", Methods: []string{"GET", "GET"}}
	if err := preset.Validate(); err == nil {
		t.Fatal("duplicate methods accepted")
	}
	preset, _ = BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	preset.DestinationCeiling = PolicyPresetDestinationCeiling{Mode: "exact", Authorities: []PolicyPresetAuthority{{Scheme: "https", Host: "api.github.com", Port: 443}, {Scheme: "https", Host: "api.github.com", Port: 443}}}
	if err := preset.Validate(); err == nil {
		t.Fatal("duplicate authorities accepted")
	}
}

func TestPolicyPresetRulesCannotExceedDestinationOrMethodCeilings(t *testing.T) {
	base := PolicyPreset{SchemaVersion: 1, Name: "bounded", Guardrail: PolicyPresetGuardrailReviewedExact, DestinationCeiling: PolicyPresetDestinationCeiling{Mode: "exact", Authorities: []PolicyPresetAuthority{{Scheme: "https", Host: "api.github.com", Port: 443}}}, MethodCeiling: PolicyPresetMethodCeiling{Mode: "exact", Methods: []string{"GET", "POST"}}, BaselineGrants: []PolicyPresetExactRule{}, BaselineTemplates: []PolicyPresetPathTemplateRule{}, MCPBaselineGrants: []PolicyPresetMCPRule{}, BaselineDenies: []PolicyPresetExactRule{}, GraphQLEndpoints: []PolicyPresetExactRule{}, MCPEndpoints: []PolicyPresetExactRule{}}
	inside := PolicyPresetExactRule{Scheme: "https", Host: "api.github.com", Port: 443, Method: "GET", Path: "/user"}
	for name, assign := range map[string]func(*PolicyPreset){
		"grant destination": func(p *PolicyPreset) {
			p.BaselineGrants = []PolicyPresetExactRule{{Scheme: "https", Host: "uploads.github.com", Port: 443, Method: "GET", Path: "/"}}
		},
		"deny method": func(p *PolicyPreset) {
			p.BaselineDenies = []PolicyPresetExactRule{{Scheme: "https", Host: "api.github.com", Port: 443, Method: "DELETE", Path: "/"}}
		},
		"GraphQL scheme": func(p *PolicyPreset) {
			p.GraphQLEndpoints = []PolicyPresetExactRule{{Scheme: "http", Host: "api.github.com", Port: 443, Method: "GET", Path: "/graphql"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			assign(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("out-of-ceiling rule accepted")
			}
		})
	}
	for _, assign := range []func(*PolicyPreset){func(p *PolicyPreset) { p.BaselineGrants = []PolicyPresetExactRule{inside} }, func(p *PolicyPreset) { p.BaselineDenies = []PolicyPresetExactRule{inside} }, func(p *PolicyPreset) {
		graphql := inside
		graphql.Method = "POST"
		p.GraphQLEndpoints = []PolicyPresetExactRule{graphql}
	}} {
		candidate := base
		assign(&candidate)
		if err := candidate.Validate(); err != nil {
			t.Fatalf("inside-ceiling rule rejected: %v", err)
		}
	}
	preset := base
	preset.GraphQLEndpoints = []PolicyPresetExactRule{inside}
	if err := preset.Validate(); err == nil {
		t.Fatal("non-POST GraphQL endpoint accepted")
	}
}

func TestSchemeChangesGuidedCandidateIdentity(t *testing.T) {
	base := PolicyDenial{PolicyProtocolIdentity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}, Timestamp: "2026-08-12T00:00:00Z", RequestID: strings.Repeat("a", 32), ContextID: "018bcfe5-687b-7000-8000-000000000000", ContextName: "default", ProjectID: "018bcfe5-687b-7000-8000-000000000001", ProjectRoot: "/project", Host: "api.github.com", Port: 443, Method: "GET", Path: "/repos", Reason: "review", StatusCode: 403, Learnable: true}
	httpsCandidate, err := NewPolicyCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Scheme = "http"
	httpCandidate, err := NewPolicyCandidate(base)
	if err != nil {
		t.Fatal(err)
	}
	if httpsCandidate.ID == httpCandidate.ID {
		t.Fatal("candidate identity omitted scheme")
	}
}

func TestMCPPolicyIdentityBindsMethodAndToolWithoutArguments(t *testing.T) {
	base := PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "codex_apps.search"}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, identity := range map[string]PolicyProtocolIdentity{
		"missing tool": {Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/call"},
		"tool on list": {Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/list", MCPToolName: "x"},
		"unsafe tool":  {Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "secret\nname"},
	} {
		if err := identity.Validate(); err == nil {
			t.Fatalf("%s identity accepted", name)
		}
	}
}
