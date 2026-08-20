package tobari

// nativeToolAuthReadinessCatalog is the binary-embedded declarative source of
// truth for reviewed native-client readiness. A new client effect changes one
// current contract; prior contracts remain append-only so legacy snapshot
// authority can be removed before the current overlay is projected.
func nativeToolAuthReadinessCatalog() []nativeToolAuthReadinessFamily {
	twgCoreGrants := []ContextPolicyExactRule{
		nativeReadinessHTTP("POST", "auth.atlassian.com", "/oauth/device/code"),
		nativeReadinessHTTP("POST", "auth.atlassian.com", "/oauth/token"),
		nativeReadinessGraphQL("api.atlassian.com", "/graphql", "me"),
	}
	twgEndpoints := []ContextPolicyExactRule{
		nativeReadinessHTTP("POST", "api.atlassian.com", "/graphql"),
	}

	return []nativeToolAuthReadinessFamily{
		nativeReadinessFamily("claude_ready", 1,
			nativeReadinessContract("claude_ready", AgentReadyClaudeVersion, 1, []ContextPolicyExactRule{
				nativeReadinessHTTP("GET", "api.anthropic.com", "/api/oauth/claude_cli/roles"),
				nativeReadinessHTTP("GET", "api.anthropic.com", "/api/oauth/profile"),
				nativeReadinessHTTP("GET", "platform.claude.com", "/v1/oauth/hello"),
				nativeReadinessHTTP("POST", "platform.claude.com", "/v1/oauth/token"),
			}, nil),
		),
		nativeReadinessFamily("codex_ready", 1,
			nativeReadinessContract("codex_ready", AgentReadyCodexVersion, 1, []ContextPolicyExactRule{
				nativeReadinessHTTP("POST", "auth.openai.com", "/api/accounts/deviceauth/token"),
				nativeReadinessHTTP("POST", "auth.openai.com", "/api/accounts/deviceauth/usercode"),
				nativeReadinessHTTP("POST", "auth.openai.com", "/oauth/token"),
			}, nil),
		),
		nativeReadinessFamily("gh_ready", 1,
			nativeReadinessContract("gh_ready", AgentReadyGitHubCLIVersion, 1, []ContextPolicyExactRule{
				nativeReadinessHTTP("POST", "github.com", "/login/device/code"),
				nativeReadinessHTTP("POST", "github.com", "/login/oauth/access_token"),
				nativeReadinessGraphQL("api.github.com", "/graphql", "viewer"),
			}, []ContextPolicyExactRule{nativeReadinessHTTP("POST", "api.github.com", "/graphql")}),
		),
		nativeReadinessFamily("pup_ready", 1,
			nativeReadinessContract("pup_ready", AgentReadyPupVersion, 1, []ContextPolicyExactRule{
				nativeReadinessHTTP("POST", "api.datadoghq.com", "/api/v2/oauth2/register"),
				nativeReadinessHTTP("POST", "api.datadoghq.com", "/oauth2/v1/token"),
			}, nil),
		),
		nativeReadinessFamily("twg_ready", 2,
			nativeReadinessContract("twg_ready", AgentReadyTWGVersion, 1, twgCoreGrants, twgEndpoints),
			nativeReadinessContract("twg_ready", AgentReadyTWGVersion, 2,
				appendReadinessRules(twgCoreGrants,
					nativeReadinessHTTP("POST", "api.atlassian.com", "/accessible-products"),
					nativeReadinessHTTP("POST", "auth.atlassian.com", "/oauth/revoke"),
					nativeReadinessHTTP("GET", "teamwork-graph.atlassian.com", "/cli/manifest.json"),
				),
				twgEndpoints,
			),
		),
	}
}

func nativeReadinessFamily(id string, currentRevision int, contracts ...nativeToolAuthReadiness) nativeToolAuthReadinessFamily {
	return nativeToolAuthReadinessFamily{ID: id, CurrentContractRevision: currentRevision, Contracts: contracts}
}

func nativeReadinessContract(id, clientVersion string, revision int, grants, endpoints []ContextPolicyExactRule) nativeToolAuthReadiness {
	return nativeToolAuthReadiness{
		ID: id, ClientVersion: clientVersion, ContractRevision: revision,
		BaselineGrants:   append([]ContextPolicyExactRule(nil), grants...),
		GraphQLEndpoints: append([]ContextPolicyExactRule(nil), endpoints...),
	}
}

func nativeReadinessHTTP(method, host, path string) ContextPolicyExactRule {
	return ContextPolicyExactRule{Scheme: "https", Host: host, Port: 443, Method: method, Path: path}
}

func nativeReadinessGraphQL(host, path, root string) ContextPolicyExactRule {
	rule := nativeReadinessHTTP("POST", host, path)
	rule.Protocol = PolicyProtocolGraphQL
	rule.GraphQLOperationType = GraphQLOperationQuery
	rule.GraphQLRootField = root
	return rule
}

func appendReadinessRules(base []ContextPolicyExactRule, extra ...ContextPolicyExactRule) []ContextPolicyExactRule {
	return append(append([]ContextPolicyExactRule(nil), base...), extra...)
}
