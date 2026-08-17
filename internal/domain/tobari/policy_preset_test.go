package tobari

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
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
	catalog := nativeToolAuthReadinessCatalog()
	seenFamilies := map[string]struct{}{}
	for _, family := range catalog {
		if family.ID == "" || family.CurrentContractRevision <= 0 || len(family.Contracts) == 0 {
			t.Fatalf("native tool readiness family is incomplete: %+v", family)
		}
		if _, duplicate := seenFamilies[family.ID]; duplicate {
			t.Fatalf("native tool readiness family is duplicated: %q", family.ID)
		}
		seenFamilies[family.ID] = struct{}{}
		seenRevisions := map[int]struct{}{}
		current := 0
		lastRevision := 0
		for _, bundle := range family.Contracts {
			if bundle.ID != family.ID || bundle.ClientVersion == "" || bundle.ContractRevision <= 0 {
				t.Fatalf("native tool readiness history escaped its family: family=%+v bundle=%+v", family, bundle)
			}
			if _, duplicate := seenRevisions[bundle.ContractRevision]; duplicate {
				t.Fatalf("native tool readiness contract revision is duplicated: %s@%d", family.ID, bundle.ContractRevision)
			}
			seenRevisions[bundle.ContractRevision] = struct{}{}
			if bundle.ContractRevision <= lastRevision {
				t.Fatalf("native tool readiness revisions are not append-only: %s@%d after %d", family.ID, bundle.ContractRevision, lastRevision)
			}
			lastRevision = bundle.ContractRevision
			if bundle.ContractRevision == family.CurrentContractRevision {
				current++
			}
		}
		if current != 1 {
			t.Fatalf("native tool readiness family %q selects %d current contracts", family.ID, current)
		}
	}

	bundles := nativeToolAuthReadinessBundles()
	wantContracts := map[string]struct {
		clientVersion string
		revision      int
	}{
		"claude_ready": {clientVersion: AgentReadyClaudeVersion, revision: 1},
		"codex_ready":  {clientVersion: AgentReadyCodexVersion, revision: 1},
		"gh_ready":     {clientVersion: AgentReadyGitHubCLIVersion, revision: 1},
		"pup_ready":    {clientVersion: AgentReadyPupVersion, revision: 1},
		"twg_ready":    {clientVersion: AgentReadyTWGVersion, revision: 2},
	}
	for _, bundle := range bundles {
		want, ok := wantContracts[bundle.ID]
		if !ok || bundle.ClientVersion != want.clientVersion || bundle.ContractRevision != want.revision || len(bundle.BaselineGrants) == 0 {
			t.Fatalf("native tool readiness bundle is not pinned: %+v", bundle)
		}
		delete(wantContracts, bundle.ID)
	}
	if len(wantContracts) != 0 {
		t.Fatalf("native tool readiness bundles are missing: %v", wantContracts)
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

	var pupRules []PolicyPresetExactRule
	for _, bundle := range bundles {
		if bundle.ID == "pup_ready" {
			pupRules = append(pupRules, bundle.BaselineGrants...)
			if len(bundle.GraphQLEndpoints) != 0 {
				t.Fatalf("pup_ready declared semantic endpoints: %+v", bundle.GraphQLEndpoints)
			}
		}
	}
	wantPup := []PolicyPresetExactRule{
		{Scheme: "https", Host: "api.datadoghq.com", Port: 443, Method: "POST", Path: "/api/v2/oauth2/register"},
		{Scheme: "https", Host: "api.datadoghq.com", Port: 443, Method: "POST", Path: "/oauth2/v1/token"},
	}
	if !reflect.DeepEqual(pupRules, wantPup) {
		t.Fatalf("pup_ready grants = %+v, want %+v", pupRules, wantPup)
	}
	for _, rule := range agentReadyBaselineGrants() {
		if strings.Contains(rule.Host, "datadog") && (rule.Host != "api.datadoghq.com" ||
			rule.Method != "POST" || (rule.Path != "/api/v2/oauth2/register" && rule.Path != "/oauth2/v1/token")) {
			t.Fatalf("neighboring Datadog authority entered baseline: %+v", rule)
		}
	}

	var twgHTTP []PolicyPresetExactRule
	var twgGraphQL []PolicyPresetExactRule
	var twgEndpoints []PolicyPresetExactRule
	for _, bundle := range bundles {
		if bundle.ID == "twg_ready" {
			for _, rule := range bundle.BaselineGrants {
				if rule.Protocol == PolicyProtocolGraphQL {
					twgGraphQL = append(twgGraphQL, rule)
				} else {
					twgHTTP = append(twgHTTP, rule)
				}
			}
			twgEndpoints = bundle.GraphQLEndpoints
		}
	}
	wantTWG := []PolicyPresetExactRule{
		{Scheme: "https", Host: "auth.atlassian.com", Port: 443, Method: "POST", Path: "/oauth/device/code"},
		{Scheme: "https", Host: "auth.atlassian.com", Port: 443, Method: "POST", Path: "/oauth/token"},
		{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/accessible-products"},
		{Scheme: "https", Host: "auth.atlassian.com", Port: 443, Method: "POST", Path: "/oauth/revoke"},
		{Scheme: "https", Host: "teamwork-graph.atlassian.com", Port: 443, Method: "GET", Path: "/cli/manifest.json"},
	}
	if !reflect.DeepEqual(twgHTTP, wantTWG) {
		t.Fatalf("twg_ready HTTP grants = %+v, want %+v", twgHTTP, wantTWG)
	}
	wantTWGEndpoint := PolicyPresetExactRule{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/graphql"}
	wantTWGGraphQL := PolicyPresetExactRule{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/graphql", Protocol: PolicyProtocolGraphQL, GraphQLOperationType: GraphQLOperationQuery, GraphQLRootField: "me"}
	if !reflect.DeepEqual(twgEndpoints, []PolicyPresetExactRule{wantTWGEndpoint}) || !reflect.DeepEqual(twgGraphQL, []PolicyPresetExactRule{wantTWGGraphQL}) {
		t.Fatalf("twg_ready semantic grants = endpoints:%+v grants:%+v", twgEndpoints, twgGraphQL)
	}
	for _, rule := range agentReadyBaselineGrants() {
		if rule.Host == "auth.atlassian.com" && (rule.Method != "POST" ||
			(rule.Path != "/oauth/device/code" && rule.Path != "/oauth/token" && rule.Path != "/oauth/revoke")) {
			t.Fatalf("neighboring Atlassian authority entered baseline: %+v", rule)
		}
		if rule.Host == "api.atlassian.com" && rule != wantTWGGraphQL &&
			rule != (PolicyPresetExactRule{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "POST", Path: "/accessible-products"}) {
			t.Fatalf("neighboring Atlassian GraphQL authority entered baseline: %+v", rule)
		}
		if rule.Host == "teamwork-graph.atlassian.com" &&
			rule != (PolicyPresetExactRule{Scheme: "https", Host: "teamwork-graph.atlassian.com", Port: 443, Method: "GET", Path: "/cli/manifest.json"}) {
			t.Fatalf("neighboring TWG manifest authority entered baseline: %+v", rule)
		}
		if strings.Contains(rule.Host, "atlassian") && rule.Host != "auth.atlassian.com" && rule.Host != "api.atlassian.com" && rule.Host != "teamwork-graph.atlassian.com" {
			t.Fatalf("unreviewed Atlassian host entered baseline: %+v", rule)
		}
	}
	for _, forbidden := range []PolicyPresetExactRule{
		{Scheme: "https", Host: "teamwork-graph.atlassian.com", Port: 443, Method: "GET", Path: "/cli/beta/manifest.json"},
		{Scheme: "https", Host: "teamwork-graph.atlassian.com", Port: 443, Method: "GET", Path: "/cli/install"},
		{Scheme: "https", Host: "api.atlassian.com", Port: 443, Method: "GET", Path: "/accessible-products"},
	} {
		if slices.Contains(agentReadyBaselineGrants(), forbidden) {
			t.Fatalf("neighboring TWG lifecycle effect entered baseline: %+v", forbidden)
		}
	}
}

func TestTWGReadinessRetainsRevisionOneAsExactRemovalHistory(t *testing.T) {
	var revisionOne nativeToolAuthReadiness
	for _, family := range nativeToolAuthReadinessCatalog() {
		if family.ID != "twg_ready" {
			continue
		}
		for _, contract := range family.Contracts {
			if contract.ContractRevision == 1 {
				revisionOne = contract
			}
		}
	}
	wantGrants := []PolicyPresetExactRule{
		nativeReadinessHTTP("POST", "auth.atlassian.com", "/oauth/device/code"),
		nativeReadinessHTTP("POST", "auth.atlassian.com", "/oauth/token"),
		nativeReadinessGraphQL("api.atlassian.com", "/graphql", "me"),
	}
	wantEndpoints := []PolicyPresetExactRule{nativeReadinessHTTP("POST", "api.atlassian.com", "/graphql")}
	if revisionOne.ID != "twg_ready" || revisionOne.ClientVersion != AgentReadyTWGVersion ||
		!reflect.DeepEqual(revisionOne.BaselineGrants, wantGrants) || !reflect.DeepEqual(revisionOne.GraphQLEndpoints, wantEndpoints) {
		t.Fatalf("twg_ready revision 1 history = %+v", revisionOne)
	}
}

func TestAgentReadySnapshotExcludesBinaryNativeReadinessAndProjectionRestoresCurrentSet(t *testing.T) {
	effective, ok := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	if !ok {
		t.Fatal("agent-ready preset is missing")
	}
	snapshot, ok := BuiltinPolicyPresetSnapshot(DefaultPolicyPresetOrigin)
	if !ok {
		t.Fatal("agent-ready snapshot source is missing")
	}
	stripped := withoutHistoricalNativeToolAuthReadiness(effective)
	if !reflect.DeepEqual(snapshot, stripped) {
		t.Fatalf("agent-ready snapshot retained binary readiness\n got: %+v\nwant: %+v", snapshot, stripped)
	}
	for _, bundle := range nativeToolAuthReadinessHistory() {
		for _, rule := range bundle.BaselineGrants {
			if slices.Contains(snapshot.BaselineGrants, rule) {
				t.Fatalf("historical readiness grant entered snapshot: %+v", rule)
			}
		}
		for _, endpoint := range bundle.GraphQLEndpoints {
			if slices.Contains(snapshot.GraphQLEndpoints, endpoint) {
				t.Fatalf("historical readiness endpoint entered snapshot: %+v", endpoint)
			}
		}
	}
	projected, err := ApplyNativeToolAuthReadiness(DefaultPolicyPresetOrigin, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	normalizedEffective, _, _, err := NormalizePolicyPreset(effective)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, normalizedEffective) {
		t.Fatalf("binary readiness projection differs from current built-in\n got: %+v\nwant: %+v", projected, normalizedEffective)
	}
}

func TestAgentReadyLegacySnapshotReadinessIsReplacedWithoutChangingSnapshot(t *testing.T) {
	legacy, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	legacy.BaselineDenies = append(legacy.BaselineDenies, PolicyPresetExactRule{
		Scheme: "https", Host: "auth.atlassian.com", Port: 443, Method: "POST", Path: "/oauth/token",
	})
	legacyBefore, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := ApplyNativeToolAuthReadiness(DefaultPolicyPresetOrigin, legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyAfter, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyAfter, legacyBefore) {
		t.Fatal("binary readiness projection mutated the immutable snapshot")
	}
	current, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
	current.BaselineDenies = append(current.BaselineDenies, legacy.BaselineDenies...)
	current, _, _, err = NormalizePolicyPreset(current)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, current) {
		t.Fatalf("legacy readiness accumulated instead of being replaced\n got: %+v\nwant: %+v", projected, current)
	}
	if !slices.Contains(projected.BaselineDenies, legacy.BaselineDenies[0]) {
		t.Fatal("snapshot exact Deny was lost while replacing readiness")
	}
}

func TestBinaryNativeReadinessAppliesOnlyToExactAgentReadyOrigin(t *testing.T) {
	custom, _ := BuiltinPolicyPreset("builtin/reviewed-exact")
	custom.Name = "custom-ready"
	custom.BaselineGrants = []PolicyPresetExactRule{{
		Scheme: "https", Host: "auth.atlassian.com", Port: 443, Method: "POST", Path: "/oauth/device/code",
	}}
	projected, err := ApplyNativeToolAuthReadiness("custom/custom-ready", custom)
	if err != nil {
		t.Fatal(err)
	}
	want, _, _, err := NormalizePolicyPreset(custom)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projected, want) {
		t.Fatalf("custom preset was changed by binary readiness: %+v", projected)
	}

	offline, _ := BuiltinPolicyPreset("builtin/offline")
	projected, err = ApplyNativeToolAuthReadiness("builtin/offline", offline)
	if err != nil || len(projected.BaselineGrants) != 0 {
		t.Fatalf("offline preset received binary readiness: %+v err=%v", projected, err)
	}

	invalid := custom
	invalid.Name = "not-agent-ready"
	if _, err := ApplyNativeToolAuthReadiness(DefaultPolicyPresetOrigin, invalid); err == nil {
		t.Fatal("agent-ready origin accepted a mismatched snapshot identity")
	}
	if _, err := ApplyNativeToolAuthReadiness("builtin/agent-ready/extra", custom); err == nil {
		t.Fatal("binary readiness accepted an invalid origin")
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
