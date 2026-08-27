package tobari

import (
	"fmt"
	"sort"
	"strings"
)

// WorkspaceTemplateSemanticModules is the closed static policy taxonomy used
// by final Template policy source. It contains request matchers only; parser,
// provider authorization, credentials, and external discovery remain outside
// Template authority.
type WorkspaceTemplateSemanticModules struct {
	Protocols WorkspaceTemplateSemanticProtocols `json:"protocols"`
	Providers WorkspaceTemplateSemanticProviders `json:"providers"`
}

type WorkspaceTemplateSemanticProtocols struct {
	HTTP WorkspaceTemplateSemanticHTTP `json:"http"`
}

type WorkspaceTemplateSemanticHTTP struct {
	Generic SemanticHTTPPolicy    `json:"generic"`
	GraphQL SemanticGraphQLPolicy `json:"graphql"`
	MCP     SemanticMCPPolicy     `json:"mcp"`
	Git     SemanticGitPolicy     `json:"git"`
	OCI     SemanticOCIPolicy     `json:"oci"`
}

type WorkspaceTemplateSemanticProviders struct {
	AWS        SemanticAWSPolicy        `json:"aws"`
	Kubernetes SemanticKubernetesPolicy `json:"kubernetes"`
}

func EmptyWorkspaceTemplateSemanticModules() WorkspaceTemplateSemanticModules {
	emptyEndpoints := []SemanticHTTPEndpoint{}
	return WorkspaceTemplateSemanticModules{
		Protocols: WorkspaceTemplateSemanticProtocols{HTTP: WorkspaceTemplateSemanticHTTP{
			Generic: SemanticHTTPPolicy{Allow: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{}}, Deny: SemanticHTTPRuleSet{Rules: []SemanticHTTPRule{}}},
			GraphQL: SemanticGraphQLPolicy{Endpoints: emptyEndpoints, Allow: SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{}}, Deny: SemanticGraphQLRuleSet{Rules: []SemanticGraphQLRule{}}},
			MCP:     SemanticMCPPolicy{Endpoints: []SemanticHTTPEndpoint{}, Allow: SemanticMCPRuleSet{Rules: []SemanticMCPRule{}}, Deny: SemanticMCPRuleSet{Rules: []SemanticMCPRule{}}},
			Git:     SemanticGitPolicy{Allow: SemanticGitRuleSet{Rules: []SemanticGitRule{}}, Deny: SemanticGitRuleSet{Rules: []SemanticGitRule{}}},
			OCI:     SemanticOCIPolicy{Allow: SemanticOCIRuleSet{Rules: []SemanticOCIRule{}}, Deny: SemanticOCIRuleSet{Rules: []SemanticOCIRule{}}},
		}},
		Providers: WorkspaceTemplateSemanticProviders{
			AWS:        SemanticAWSPolicy{Allow: SemanticAWSRuleSet{Rules: []SemanticAWSRule{}}, Deny: SemanticAWSRuleSet{Rules: []SemanticAWSRule{}}},
			Kubernetes: SemanticKubernetesPolicy{Allow: SemanticKubernetesRuleSet{Rules: []SemanticKubernetesRule{}}, Deny: SemanticKubernetesRuleSet{Rules: []SemanticKubernetesRule{}}},
		},
	}
}

func (p WorkspaceTemplatePolicyBody) FinalSemanticModules() (WorkspaceTemplateSemanticModules, bool) {
	if p.SemanticModules == nil {
		return WorkspaceTemplateSemanticModules{}, false
	}
	return p.SemanticModules.Normalize(), true
}

func (m WorkspaceTemplateSemanticModules) GraphQLEndpoints() []GraphQLEndpoint {
	return expandSemanticEndpoints(m.Protocols.HTTP.GraphQL.Endpoints)
}

func (m WorkspaceTemplateSemanticModules) MCPEndpoints() []MCPEndpoint {
	expanded := expandSemanticEndpoints(m.Protocols.HTTP.MCP.Endpoints)
	result := make([]MCPEndpoint, 0, len(expanded))
	for _, endpoint := range expanded {
		result = append(result, MCPEndpoint(endpoint))
	}
	return result
}

func expandSemanticEndpoints(source []SemanticHTTPEndpoint) []GraphQLEndpoint {
	result := []GraphQLEndpoint{}
	for _, endpoint := range source {
		for _, host := range endpoint.hosts() {
			result = append(result, GraphQLEndpoint{Scheme: endpoint.Scheme, Host: host, Port: endpoint.Port, Path: endpoint.Path})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Scheme+"\x00"+result[i].Host+"\x00"+result[i].Path < result[j].Scheme+"\x00"+result[j].Host+"\x00"+result[j].Path
	})
	return result
}

func (m WorkspaceTemplateSemanticModules) Validate(deniedMethods []string) error {
	if err := m.Protocols.HTTP.Generic.Validate(); err != nil {
		return fmt.Errorf("generic HTTP policy: %w", err)
	}
	if err := m.Protocols.HTTP.GraphQL.Validate(); err != nil {
		return fmt.Errorf("GraphQL policy: %w", err)
	}
	if err := m.Protocols.HTTP.MCP.Validate(); err != nil {
		return fmt.Errorf("MCP policy: %w", err)
	}
	if err := m.Protocols.HTTP.Git.Validate(); err != nil {
		return fmt.Errorf("Git policy: %w", err)
	}
	if err := m.Protocols.HTTP.OCI.Validate(); err != nil {
		return fmt.Errorf("OCI policy: %w", err)
	}
	if err := m.Providers.AWS.Validate(); err != nil {
		return fmt.Errorf("AWS policy: %w", err)
	}
	if err := m.Providers.Kubernetes.Validate(); err != nil {
		return fmt.Errorf("Kubernetes policy: %w", err)
	}
	if semanticEndpointSetsCollide(m.Protocols.HTTP.GraphQL.Endpoints, m.Protocols.HTTP.MCP.Endpoints) {
		return fmt.Errorf("GraphQL and MCP endpoint declarations collide")
	}
	denied := make(map[string]struct{}, len(deniedMethods))
	for _, method := range deniedMethods {
		denied[method] = struct{}{}
	}
	if semanticAllowsFullyMethodDenied(m, denied) {
		return fmt.Errorf("semantic Allow is fully shadowed by the Method Boundary")
	}
	return nil
}

func semanticEndpointSetsCollide(left, right []SemanticHTTPEndpoint) bool {
	for _, a := range left {
		for _, b := range right {
			if a.Scheme != b.Scheme || a.Port != b.Port || a.Path != b.Path {
				continue
			}
			hosts := make(map[string]struct{}, len(a.hosts()))
			for _, host := range a.hosts() {
				hosts[host] = struct{}{}
			}
			for _, host := range b.hosts() {
				if _, collision := hosts[host]; collision {
					return true
				}
			}
		}
	}
	return false
}

func semanticAllowsFullyMethodDenied(m WorkspaceTemplateSemanticModules, denied map[string]struct{}) bool {
	allDenied := func(methods ...string) bool {
		if len(methods) == 0 {
			return false
		}
		for _, method := range methods {
			if _, ok := denied[method]; !ok {
				return false
			}
		}
		return true
	}
	for _, rule := range m.Protocols.HTTP.Generic.Allow.Rules {
		if allDenied(rule.Method) {
			return true
		}
	}
	if len(m.Protocols.HTTP.GraphQL.Allow.Rules) > 0 && allDenied("POST") || len(m.Protocols.HTTP.MCP.Allow.Rules) > 0 && allDenied("POST") || len(m.Providers.AWS.Allow.Rules) > 0 && allDenied("POST") {
		return true
	}
	if len(m.Protocols.HTTP.Git.Allow.Rules) > 0 && allDenied("GET", "POST") {
		return true
	}
	for _, rule := range m.Protocols.HTTP.OCI.Allow.Rules {
		if allDenied(semanticOCIMethods(rule)...) {
			return true
		}
	}
	for _, rule := range m.Providers.Kubernetes.Allow.Rules {
		if allDenied(semanticKubernetesMethods(rule.identity())...) {
			return true
		}
	}
	return false
}

func semanticKubernetesMethods(identity PolicyProtocolIdentity) []string {
	if identity.KubernetesKind == KubernetesRequestNonResource {
		return []string{"GET"}
	}
	if identity.KubernetesVerb == "connect" {
		return []string{"GET", "POST"}
	}
	method := map[string]string{"get": "GET", "list": "GET", "watch": "GET", "create": "POST", "update": "PUT", "patch": "PATCH", "delete": "DELETE", "deletecollection": "DELETE"}[identity.KubernetesVerb]
	if method == "" {
		return nil
	}
	return []string{method}
}

func semanticOCIMethods(rule SemanticOCIRule) []string {
	switch rule.Action {
	case "list", "upload_status":
		return []string{"GET"}
	case "pull":
		if strings.HasPrefix(rule.Object, "referrers:") {
			return []string{"GET"}
		}
		return []string{"GET", "HEAD"}
	case "push":
		return []string{"PUT"}
	case "complete_upload":
		if strings.HasPrefix(rule.Object, "upload:") {
			return []string{"PUT"}
		}
		return []string{"POST"}
	case "delete", "cancel_upload":
		return []string{"DELETE"}
	case "start_upload", "mount":
		return []string{"POST"}
	case "upload_chunk":
		return []string{"PATCH"}
	default:
		return nil
	}
}

func (m WorkspaceTemplateSemanticModules) Clone() WorkspaceTemplateSemanticModules {
	return m.normalized(false)
}

// Normalize canonicalizes every semantic set before it contributes to an
// immutable revision. User-authored array order therefore carries no authority.
func (m WorkspaceTemplateSemanticModules) Normalize() WorkspaceTemplateSemanticModules {
	return m.normalized(true)
}

func (m WorkspaceTemplateSemanticModules) normalized(order bool) WorkspaceTemplateSemanticModules {
	result := m
	cloneAuthority := func(authority SemanticRuleAuthority) SemanticRuleAuthority {
		if authority.Hosts != nil {
			authority.Hosts = append([]string{}, authority.Hosts...)
			if order {
				sort.Strings(authority.Hosts)
			}
		}
		return authority
	}
	cloneEndpoints := func(source []SemanticHTTPEndpoint) []SemanticHTTPEndpoint {
		result := append([]SemanticHTTPEndpoint{}, source...)
		for index := range result {
			result[index].SemanticRuleAuthority = cloneAuthority(result[index].SemanticRuleAuthority)
		}
		if order {
			sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
		}
		return result
	}
	result.Protocols.HTTP.GraphQL.Endpoints = cloneEndpoints(m.Protocols.HTTP.GraphQL.Endpoints)
	result.Protocols.HTTP.MCP.Endpoints = cloneEndpoints(m.Protocols.HTTP.MCP.Endpoints)

	result.Protocols.HTTP.Generic.Allow.Rules = append([]SemanticHTTPRule{}, m.Protocols.HTTP.Generic.Allow.Rules...)
	result.Protocols.HTTP.Generic.Deny.Rules = append([]SemanticHTTPRule{}, m.Protocols.HTTP.Generic.Deny.Rules...)
	for _, rules := range [][]SemanticHTTPRule{result.Protocols.HTTP.Generic.Allow.Rules, result.Protocols.HTTP.Generic.Deny.Rules} {
		for index := range rules {
			rules[index].SemanticRuleAuthority = cloneAuthority(rules[index].SemanticRuleAuthority)
		}
		if order {
			sort.Slice(rules, func(i, j int) bool { return rules[i].canonicalKey() < rules[j].canonicalKey() })
		}
	}
	result.Protocols.HTTP.GraphQL.Allow.Rules = cloneGraphQLRules(m.Protocols.HTTP.GraphQL.Allow.Rules, cloneAuthority, order)
	result.Protocols.HTTP.GraphQL.Deny.Rules = cloneGraphQLRules(m.Protocols.HTTP.GraphQL.Deny.Rules, cloneAuthority, order)
	result.Protocols.HTTP.MCP.Allow.Rules = cloneMCPRules(m.Protocols.HTTP.MCP.Allow.Rules, cloneAuthority, order)
	result.Protocols.HTTP.MCP.Deny.Rules = cloneMCPRules(m.Protocols.HTTP.MCP.Deny.Rules, cloneAuthority, order)
	result.Protocols.HTTP.Git.Allow.Rules = cloneGitRules(m.Protocols.HTTP.Git.Allow.Rules, cloneAuthority, order)
	result.Protocols.HTTP.Git.Deny.Rules = cloneGitRules(m.Protocols.HTTP.Git.Deny.Rules, cloneAuthority, order)
	result.Protocols.HTTP.OCI.Allow.Rules = cloneOCIRules(m.Protocols.HTTP.OCI.Allow.Rules, cloneAuthority, order)
	result.Protocols.HTTP.OCI.Deny.Rules = cloneOCIRules(m.Protocols.HTTP.OCI.Deny.Rules, cloneAuthority, order)
	result.Providers.AWS.Allow.Rules = cloneAWSRules(m.Providers.AWS.Allow.Rules, cloneAuthority, order)
	result.Providers.AWS.Deny.Rules = cloneAWSRules(m.Providers.AWS.Deny.Rules, cloneAuthority, order)
	result.Providers.Kubernetes.Allow.Rules = cloneKubernetesRules(m.Providers.Kubernetes.Allow.Rules, cloneAuthority, order)
	result.Providers.Kubernetes.Deny.Rules = cloneKubernetesRules(m.Providers.Kubernetes.Deny.Rules, cloneAuthority, order)
	return result
}

func cloneGraphQLRules(source []SemanticGraphQLRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticGraphQLRule {
	result := append([]SemanticGraphQLRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}

func cloneMCPRules(source []SemanticMCPRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticMCPRule {
	result := append([]SemanticMCPRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}

func cloneGitRules(source []SemanticGitRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticGitRule {
	result := append([]SemanticGitRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}

func cloneOCIRules(source []SemanticOCIRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticOCIRule {
	result := append([]SemanticOCIRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}

func cloneAWSRules(source []SemanticAWSRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticAWSRule {
	result := append([]SemanticAWSRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
		if result[index].Services != nil {
			result[index].Services = append([]string{}, result[index].Services...)
			if order {
				sort.Strings(result[index].Services)
			}
		}
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}

func cloneKubernetesRules(source []SemanticKubernetesRule, authority func(SemanticRuleAuthority) SemanticRuleAuthority, order bool) []SemanticKubernetesRule {
	result := append([]SemanticKubernetesRule{}, source...)
	for index := range result {
		result[index].SemanticRuleAuthority = authority(result[index].SemanticRuleAuthority)
		if result[index].Resource != nil {
			value := *result[index].Resource
			result[index].Resource = &value
		}
		if result[index].NonResource != nil {
			value := *result[index].NonResource
			result[index].NonResource = &value
		}
	}
	if order {
		sort.Slice(result, func(i, j int) bool { return result[i].canonicalKey() < result[j].canonicalKey() })
	}
	return result
}
