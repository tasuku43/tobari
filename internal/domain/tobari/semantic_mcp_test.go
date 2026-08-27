package tobari

import "testing"

func mcpRule(host, method, tool string) SemanticMCPRule {
	return SemanticMCPRule{
		SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: host, Port: 443},
		Path:                  "/mcp", Method: method, ToolName: tool,
	}
}

func TestSemanticMCPRuleMatchesExactProjectionWithoutArguments(t *testing.T) {
	rule := mcpRule("mcp.vendor.dev", "tools/call", "issues.get")
	effect := SemanticRequestEffect{
		Scheme: "https", Host: "mcp.vendor.dev", Port: 443, Method: "POST", Path: "/mcp",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "issues.get"},
	}
	if !rule.Matches(effect) {
		t.Fatal("MCP rule did not match its complete effect")
	}
	for name, mutate := range map[string]func(*SemanticRequestEffect){
		"transport_method": func(value *SemanticRequestEffect) { value.Method = "GET" },
		"path":             func(value *SemanticRequestEffect) { value.Path = "/other" },
		"rpc_method": func(value *SemanticRequestEffect) {
			value.Identity.MCPMethod = "tools/list"
			value.Identity.MCPToolName = ""
		},
		"tool": func(value *SemanticRequestEffect) { value.Identity.MCPToolName = "issues.update" },
		"generic": func(value *SemanticRequestEffect) {
			value.Identity = PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolHTTP}
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := effect
			mutate(&changed)
			if rule.Matches(changed) {
				t.Fatalf("MCP rule matched changed effect %+v", changed)
			}
		})
	}
	nonTool := mcpRule("mcp.vendor.dev", "tools/list", "")
	nonToolEffect := effect
	nonToolEffect.Identity.MCPMethod, nonToolEffect.Identity.MCPToolName = "tools/list", ""
	if !nonTool.Matches(nonToolEffect) {
		t.Fatal("MCP non-tool method did not match without a tool name")
	}
}

func TestSemanticMCPRuleRejectsToolNameOutsideToolsCall(t *testing.T) {
	if err := mcpRule("mcp.vendor.dev", "tools/list", "issues.get").Validate(); err == nil {
		t.Fatal("MCP tool name outside tools/call was accepted")
	}
	if err := mcpRule("mcp.vendor.dev", "tools/call", "").Validate(); err == nil {
		t.Fatal("MCP tools/call without exact tool name was accepted")
	}
}

func TestSemanticMCPDynamicEffectRequiresPOST(t *testing.T) {
	effect := SemanticRequestEffect{
		Scheme: "https", Host: "mcp.vendor.dev", Port: 443, Method: "GET", Path: "/mcp",
		Identity: PolicyProtocolIdentity{Scheme: "https", Protocol: PolicyProtocolMCP, MCPMethod: "tools/list"},
	}
	if err := effect.Validate(); err == nil {
		t.Fatal("dynamic MCP GET effect was accepted")
	}
}

func TestSemanticMCPPolicyRequiresEndpointUnionAndRejectsCombinedShadow(t *testing.T) {
	allow := mcpRule("mcp.vendor.dev", "tools/call", "issues.get")
	allow.Host, allow.Hosts = "", []string{"mcp.vendor.dev", "mcp-alt.vendor.dev"}
	policy := SemanticMCPPolicy{
		Endpoints: []SemanticHTTPEndpoint{
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "mcp.vendor.dev", Port: 443}, Path: "/mcp"},
			{SemanticRuleAuthority: SemanticRuleAuthority{Scheme: "https", Host: "mcp-alt.vendor.dev", Port: 443}, Path: "/mcp"},
		},
		Allow: SemanticMCPRuleSet{Rules: []SemanticMCPRule{allow}},
		Deny: SemanticMCPRuleSet{Rules: []SemanticMCPRule{
			mcpRule("mcp.vendor.dev", "tools/call", "issues.get"),
			mcpRule("mcp-alt.vendor.dev", "tools/call", "issues.get"),
		}},
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("MCP Allow fully covered by combined Deny rules was accepted")
	}
	policy.Deny.Rules = policy.Deny.Rules[:1]
	if err := policy.Validate(); err != nil {
		t.Fatalf("partially covered MCP policy rejected: %v", err)
	}
	policy.Endpoints = policy.Endpoints[:1]
	if err := policy.Validate(); err == nil {
		t.Fatal("MCP rule without complete trusted endpoint coverage was accepted")
	}
}

func TestSemanticMCPRuleSetRejectsReorderedDuplicates(t *testing.T) {
	first := mcpRule("mcp.vendor.dev", "tools/list", "")
	first.Host, first.Hosts = "", []string{"mcp.vendor.dev", "mcp-alt.vendor.dev"}
	second := first
	second.Hosts = []string{"mcp-alt.vendor.dev", "mcp.vendor.dev"}
	if err := (SemanticMCPRuleSet{Rules: []SemanticMCPRule{first, second}}).Validate(); err == nil {
		t.Fatal("reordered duplicate MCP rules were accepted")
	}
}
