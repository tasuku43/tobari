package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const (
	ContextPolicySchemaVersion = 1
	AgentReadyClaudeVersion    = "2.1.220"
	AgentReadyCodexVersion     = "0.147.0"
	AgentReadyGitHubCLIVersion = "2.96.0"
	AgentReadyPupVersion       = "1.10.7"
	AgentReadyTWGVersion       = "1.2.5"

	ContextMethodAllow       ContextMethodDecision = "allow"
	ContextMethodExactReview ContextMethodDecision = "exact_review"
	ContextMethodDeny        ContextMethodDecision = "deny"
)

var (
	contextPolicyNamePattern   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	contextPolicyMethodPattern = regexp.MustCompile(`^[A-Z][A-Z0-9!#$%&'*+.^_` + "`" + `|~-]{0,31}$`)
)

type ContextPolicyAuthority struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

func (a ContextPolicyAuthority) Validate() error {
	if a.Scheme != "https" && a.Scheme != "http" {
		return fmt.Errorf("context policy authority scheme is invalid")
	}
	if a.Port < 1 || a.Port > 65535 || contextPolicyIPv4Literal(a.Host) || !validNormalizedPolicyHost(a.Host) || !strings.Contains(a.Host, ".") || contextPolicyReservedHost(a.Host) {
		return fmt.Errorf("context policy authority is not an exact public destination")
	}
	return nil
}

// contextPolicyIPv4Literal keeps the domain validator independent from the net
// package while rejecting the only IP literal shape that can otherwise pass
// the normalized DNS-label grammar. IPv6 literals already fail that grammar.
func contextPolicyIPv4Literal(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return false
	}
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return false
		}
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 || value > 255 {
			return false
		}
	}
	return true
}

func contextPolicyReservedHost(host string) bool {
	if host == "localhost" || host == "localhost.localdomain" || host == "example.com" || host == "example.net" || host == "example.org" {
		return true
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".test", ".invalid", ".example"} {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

type ContextPolicyExactRule struct {
	Scheme               string `json:"scheme"`
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Method               string `json:"method"`
	Path                 string `json:"path"`
	Protocol             string `json:"protocol,omitempty"`
	GraphQLOperationType string `json:"graphql_operation_type,omitempty"`
	GraphQLRootField     string `json:"graphql_root_field,omitempty"`
}

// ContextPolicyPathTemplateRule is a reviewed built-in HTTP path shape. It is
// deliberately narrower than learned templates: exactly one direct {id}
// segment is permitted and no observed identifier is retained.
type ContextPolicyPathTemplateRule struct {
	Scheme   string   `json:"scheme"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Segments []string `json:"segments"`
}

func (r ContextPolicyPathTemplateRule) Validate() error {
	if err := (ContextPolicyExactRule{Scheme: r.Scheme, Host: r.Host, Port: r.Port, Method: r.Method, Path: r.Path}).Validate(); err != nil {
		return err
	}
	if len(r.Segments) < 2 || len(r.Segments) > 32 || r.Path != "/"+strings.Join(r.Segments, "/") {
		return fmt.Errorf("context policy path template is invalid")
	}
	wildcards := 0
	for _, segment := range r.Segments {
		if segment == "{id}" {
			wildcards++
			continue
		}
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\\%?#`) {
			return fmt.Errorf("context policy path template segment is invalid")
		}
	}
	if wildcards != 1 || r.Segments[len(r.Segments)-1] != "{id}" {
		return fmt.Errorf("context policy path template must end in one identifier segment")
	}
	return nil
}

type ContextPolicyMCPRule struct {
	Scheme      string `json:"scheme"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	MCPMethod   string `json:"mcp_method"`
	MCPToolName string `json:"mcp_tool_name,omitempty"`
}

func (r ContextPolicyMCPRule) Validate() error {
	if err := (ContextPolicyExactRule{Scheme: r.Scheme, Host: r.Host, Port: r.Port, Method: r.Method, Path: r.Path}).Validate(); err != nil {
		return err
	}
	if r.Method != "POST" {
		return fmt.Errorf("context policy MCP transport method must be POST")
	}
	return (PolicyProtocolIdentity{Scheme: r.Scheme, Protocol: PolicyProtocolMCP, MCPMethod: r.MCPMethod, MCPToolName: r.MCPToolName}).Validate()
}

type ContextMethodDecision string

func (d ContextMethodDecision) Validate() error {
	switch d {
	case ContextMethodAllow, ContextMethodExactReview, ContextMethodDeny:
		return nil
	default:
		return fmt.Errorf("context policy method decision is invalid")
	}
}

type ContextMethodOverride struct {
	Method   string                `json:"method"`
	Decision ContextMethodDecision `json:"decision"`
}

func (o ContextMethodOverride) Validate() error {
	if !contextPolicyMethodPattern.MatchString(o.Method) {
		return fmt.Errorf("context policy method override is invalid")
	}
	return o.Decision.Validate()
}

type ContextMethodPolicy struct {
	Default   ContextMethodDecision   `json:"default"`
	Overrides []ContextMethodOverride `json:"overrides"`
}

func (p ContextMethodPolicy) Clone() ContextMethodPolicy {
	overrides := make([]ContextMethodOverride, len(p.Overrides))
	copy(overrides, p.Overrides)
	return ContextMethodPolicy{
		Default:   p.Default,
		Overrides: overrides,
	}
}

func NormalizeContextMethodPolicy(p ContextMethodPolicy) (ContextMethodPolicy, error) {
	if err := p.Validate(); err != nil {
		return ContextMethodPolicy{}, err
	}
	result := p.Clone()
	sort.Slice(result.Overrides, func(i, j int) bool { return result.Overrides[i].Method < result.Overrides[j].Method })
	return result, nil
}

func (p ContextMethodPolicy) Validate() error {
	if err := p.Default.Validate(); err != nil {
		return err
	}
	if p.Overrides == nil {
		return fmt.Errorf("context policy method overrides must be explicit")
	}
	seen := make(map[string]struct{}, len(p.Overrides))
	for _, override := range p.Overrides {
		if err := override.Validate(); err != nil {
			return err
		}
		if override.Decision == p.Default {
			return fmt.Errorf("context policy method override repeats the default")
		}
		if _, ok := seen[override.Method]; ok {
			return fmt.Errorf("context policy method override is duplicated")
		}
		seen[override.Method] = struct{}{}
	}
	return nil
}

func (p ContextMethodPolicy) Decision(method string) ContextMethodDecision {
	for _, override := range p.Overrides {
		if override.Method == method {
			return override.Decision
		}
	}
	return p.Default
}

type ContextPolicyDestinationCeiling struct {
	Mode        string                   `json:"mode"`
	Authorities []ContextPolicyAuthority `json:"authorities"`
}

func (r ContextPolicyExactRule) Validate() error {
	if err := (ContextPolicyAuthority{Scheme: r.Scheme, Host: r.Host, Port: r.Port}).Validate(); err != nil {
		return err
	}
	if !contextPolicyMethodPattern.MatchString(r.Method) || !strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, "\r\n") {
		return fmt.Errorf("context policy exact rule is invalid")
	}
	if r.Protocol == "" {
		if r.GraphQLOperationType != "" || r.GraphQLRootField != "" {
			return fmt.Errorf("context policy HTTP rule has semantic fields")
		}
		return nil
	}
	if r.Protocol != PolicyProtocolGraphQL || r.Method != "POST" {
		return fmt.Errorf("context policy semantic exact rule is invalid")
	}
	if err := (PolicyProtocolIdentity{Scheme: r.Scheme, Protocol: r.Protocol, GraphQLOperationType: r.GraphQLOperationType, GraphQLRootField: r.GraphQLRootField}).Validate(); err != nil {
		return err
	}
	return nil
}

// ContextPolicy is strict non-executable schema-V1 owner data. Empty
// collections are present in normalized bytes so revisions do not depend on
// decoder omission behavior.
type ContextPolicy struct {
	SchemaVersion      int                             `json:"schema_version"`
	Name               string                          `json:"name"`
	DestinationCeiling ContextPolicyDestinationCeiling `json:"destination_ceiling"`
	MethodPolicy       ContextMethodPolicy             `json:"method_policy"`
	BaselineGrants     []ContextPolicyExactRule        `json:"baseline_grants"`
	BaselineTemplates  []ContextPolicyPathTemplateRule `json:"baseline_templates"`
	MCPBaselineGrants  []ContextPolicyMCPRule          `json:"mcp_baseline_grants"`
	BaselineDenies     []ContextPolicyExactRule        `json:"baseline_denies"`
	GraphQLEndpoints   []ContextPolicyExactRule        `json:"graphql_endpoints"`
	MCPEndpoints       []ContextPolicyExactRule        `json:"mcp_endpoints"`
}

func (p ContextPolicy) Validate() error {
	if p.SchemaVersion != ContextPolicySchemaVersion || !contextPolicyNamePattern.MatchString(p.Name) {
		return fmt.Errorf("context policy identity is invalid")
	}
	if p.DestinationCeiling.Authorities == nil || p.MethodPolicy.Overrides == nil || p.BaselineGrants == nil || p.BaselineTemplates == nil || p.MCPBaselineGrants == nil || p.BaselineDenies == nil || p.GraphQLEndpoints == nil || p.MCPEndpoints == nil {
		return fmt.Errorf("context policy collections must be explicit")
	}
	if p.DestinationCeiling.Mode != "public_https" && p.DestinationCeiling.Mode != "exact" {
		return fmt.Errorf("context policy destination ceiling mode is invalid")
	}
	if err := p.MethodPolicy.Validate(); err != nil {
		return err
	}
	if p.DestinationCeiling.Mode == "public_https" && len(p.DestinationCeiling.Authorities) != 0 {
		return fmt.Errorf("public_https destination ceiling cannot contain exact authorities")
	}
	seen := map[string]struct{}{}
	for _, authority := range p.DestinationCeiling.Authorities {
		if err := authority.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s\x00%s\x00%d", authority.Scheme, authority.Host, authority.Port)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("context policy authority is duplicated")
		}
		seen[key] = struct{}{}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.BaselineGrants {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path, rule.Protocol, rule.GraphQLOperationType, rule.GraphQLRootField)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("context policy grant is duplicated")
		}
		seen[key] = struct{}{}
		if !contextPolicyRuleInsideDestination(p.DestinationCeiling, rule) || p.MethodPolicy.Decision(rule.Method) == ContextMethodDeny {
			return fmt.Errorf("context policy rule exceeds its ceiling")
		}
		if rule.Protocol == PolicyProtocolGraphQL {
			endpoint := rule
			endpoint.Protocol, endpoint.GraphQLOperationType, endpoint.GraphQLRootField = "", "", ""
			found := false
			for _, declared := range p.GraphQLEndpoints {
				if declared == endpoint {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("context policy GraphQL grant has no declared endpoint")
			}
		}
	}
	for _, rules := range [][]ContextPolicyExactRule{p.BaselineDenies, p.GraphQLEndpoints, p.MCPEndpoints} {
		seen = map[string]struct{}{}
		for _, rule := range rules {
			if err := rule.Validate(); err != nil {
				return err
			}
			if rule.Protocol != "" {
				return fmt.Errorf("context policy endpoint or deny cannot contain semantic grant identity")
			}
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("context policy rule is duplicated")
			}
			seen[key] = struct{}{}
			if !contextPolicyRuleInsideDestination(p.DestinationCeiling, rule) {
				return fmt.Errorf("context policy rule exceeds its destination ceiling")
			}
			if p.MethodPolicy.Decision(rule.Method) == ContextMethodDeny {
				return fmt.Errorf("context policy rule exceeds its method ceiling")
			}
		}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.BaselineTemplates {
		if err := rule.Validate(); err != nil {
			return err
		}
		exact := ContextPolicyExactRule{Scheme: rule.Scheme, Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("context policy path template is duplicated")
		}
		seen[key] = struct{}{}
		if !contextPolicyRuleInsideDestination(p.DestinationCeiling, exact) || p.MethodPolicy.Decision(rule.Method) == ContextMethodDeny {
			return fmt.Errorf("context policy path template exceeds its ceiling")
		}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.MCPBaselineGrants {
		if err := rule.Validate(); err != nil {
			return err
		}
		exact := ContextPolicyExactRule{Scheme: rule.Scheme, Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path, rule.MCPMethod, rule.MCPToolName)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("context policy MCP grant is duplicated")
		}
		seen[key] = struct{}{}
		if !contextPolicyRuleInsideDestination(p.DestinationCeiling, exact) || p.MethodPolicy.Decision(rule.Method) == ContextMethodDeny {
			return fmt.Errorf("context policy MCP grant exceeds its ceiling")
		}
		found := false
		for _, endpoint := range p.MCPEndpoints {
			if endpoint == exact {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("context policy MCP grant has no declared endpoint")
		}
	}
	for _, endpoint := range p.GraphQLEndpoints {
		if endpoint.Method != "POST" {
			return fmt.Errorf("context policy GraphQL endpoint method must be POST")
		}
	}
	for _, endpoint := range p.MCPEndpoints {
		if endpoint.Method != "POST" {
			return fmt.Errorf("context policy MCP endpoint method must be POST")
		}
	}
	for _, graphql := range p.GraphQLEndpoints {
		for _, mcp := range p.MCPEndpoints {
			if graphql.Scheme == mcp.Scheme && graphql.Host == mcp.Host && graphql.Port == mcp.Port && graphql.Path == mcp.Path {
				return fmt.Errorf("context policy semantic endpoint is ambiguous")
			}
		}
	}
	return nil
}

func contextPolicyRuleInsideDestination(ceiling ContextPolicyDestinationCeiling, rule ContextPolicyExactRule) bool {
	if ceiling.Mode == "public_https" {
		return rule.Scheme == "https"
	}
	return contextPolicyContainsAuthority(ceiling.Authorities, rule)
}

func contextPolicyContainsAuthority(authorities []ContextPolicyAuthority, rule ContextPolicyExactRule) bool {
	for _, authority := range authorities {
		if authority.Scheme == rule.Scheme && authority.Host == rule.Host && authority.Port == rule.Port {
			return true
		}
	}
	return false
}

func NormalizeContextPolicy(p ContextPolicy) (ContextPolicy, []byte, string, error) {
	if err := p.Validate(); err != nil {
		return ContextPolicy{}, nil, "", err
	}
	clone := p
	clone.DestinationCeiling.Authorities = append([]ContextPolicyAuthority{}, p.DestinationCeiling.Authorities...)
	clone.MethodPolicy.Overrides = append([]ContextMethodOverride{}, p.MethodPolicy.Overrides...)
	clone.BaselineGrants = append([]ContextPolicyExactRule{}, p.BaselineGrants...)
	clone.BaselineTemplates = append([]ContextPolicyPathTemplateRule{}, p.BaselineTemplates...)
	clone.MCPBaselineGrants = append([]ContextPolicyMCPRule{}, p.MCPBaselineGrants...)
	clone.BaselineDenies = append([]ContextPolicyExactRule{}, p.BaselineDenies...)
	clone.GraphQLEndpoints = append([]ContextPolicyExactRule{}, p.GraphQLEndpoints...)
	clone.MCPEndpoints = append([]ContextPolicyExactRule{}, p.MCPEndpoints...)
	sort.Slice(clone.DestinationCeiling.Authorities, func(i, j int) bool {
		a, b := clone.DestinationCeiling.Authorities[i], clone.DestinationCeiling.Authorities[j]
		return fmt.Sprintf("%s/%s/%05d", a.Scheme, a.Host, a.Port) < fmt.Sprintf("%s/%s/%05d", b.Scheme, b.Host, b.Port)
	})
	sort.Slice(clone.MethodPolicy.Overrides, func(i, j int) bool {
		return clone.MethodPolicy.Overrides[i].Method < clone.MethodPolicy.Overrides[j].Method
	})
	lessRule := func(a, b ContextPolicyExactRule) bool {
		return fmt.Sprintf("%s/%s/%05d/%s/%s/%s/%s/%s", a.Scheme, a.Host, a.Port, a.Method, a.Path, a.Protocol, a.GraphQLOperationType, a.GraphQLRootField) < fmt.Sprintf("%s/%s/%05d/%s/%s/%s/%s/%s", b.Scheme, b.Host, b.Port, b.Method, b.Path, b.Protocol, b.GraphQLOperationType, b.GraphQLRootField)
	}
	sort.Slice(clone.BaselineGrants, func(i, j int) bool { return lessRule(clone.BaselineGrants[i], clone.BaselineGrants[j]) })
	sort.Slice(clone.BaselineTemplates, func(i, j int) bool {
		a, b := clone.BaselineTemplates[i], clone.BaselineTemplates[j]
		return fmt.Sprintf("%s/%s/%05d/%s/%s", a.Scheme, a.Host, a.Port, a.Method, a.Path) < fmt.Sprintf("%s/%s/%05d/%s/%s", b.Scheme, b.Host, b.Port, b.Method, b.Path)
	})
	sort.Slice(clone.MCPBaselineGrants, func(i, j int) bool {
		a, b := clone.MCPBaselineGrants[i], clone.MCPBaselineGrants[j]
		return fmt.Sprintf("%s/%s/%05d/%s/%s/%s/%s", a.Scheme, a.Host, a.Port, a.Method, a.Path, a.MCPMethod, a.MCPToolName) < fmt.Sprintf("%s/%s/%05d/%s/%s/%s/%s", b.Scheme, b.Host, b.Port, b.Method, b.Path, b.MCPMethod, b.MCPToolName)
	})
	sort.Slice(clone.BaselineDenies, func(i, j int) bool { return lessRule(clone.BaselineDenies[i], clone.BaselineDenies[j]) })
	sort.Slice(clone.GraphQLEndpoints, func(i, j int) bool { return lessRule(clone.GraphQLEndpoints[i], clone.GraphQLEndpoints[j]) })
	sort.Slice(clone.MCPEndpoints, func(i, j int) bool { return lessRule(clone.MCPEndpoints[i], clone.MCPEndpoints[j]) })
	data, err := json.MarshalIndent(clone, "", "  ")
	if err != nil {
		return ContextPolicy{}, nil, "", err
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	return clone, data, "sha256:" + hex.EncodeToString(digest[:]), nil
}

// ComposeContextMethodPolicy replaces one Context snapshot's complete method
// policy and removes only positive baseline authority made unreachable by a
// terminal method Deny. Destination ceilings and exact Denies are unchanged.
func ComposeContextMethodPolicy(
	policy ContextPolicy, methodPolicy ContextMethodPolicy,
) (ContextPolicy, error) {
	if err := policy.Validate(); err != nil {
		return ContextPolicy{}, err
	}
	policy.MethodPolicy = methodPolicy.Clone()
	grants := make([]ContextPolicyExactRule, 0, len(policy.BaselineGrants))
	for _, rule := range policy.BaselineGrants {
		if methodPolicy.Decision(rule.Method) != ContextMethodDeny {
			grants = append(grants, rule)
		}
	}
	policy.BaselineGrants = grants
	templates := make([]ContextPolicyPathTemplateRule, 0, len(policy.BaselineTemplates))
	for _, rule := range policy.BaselineTemplates {
		if methodPolicy.Decision(rule.Method) != ContextMethodDeny {
			templates = append(templates, rule)
		}
	}
	policy.BaselineTemplates = templates
	mcpGrants := make([]ContextPolicyMCPRule, 0, len(policy.MCPBaselineGrants))
	for _, rule := range policy.MCPBaselineGrants {
		if methodPolicy.Decision(rule.Method) != ContextMethodDeny {
			mcpGrants = append(mcpGrants, rule)
		}
	}
	policy.MCPBaselineGrants = mcpGrants
	graphqlEndpoints := make([]ContextPolicyExactRule, 0, len(policy.GraphQLEndpoints))
	for _, endpoint := range policy.GraphQLEndpoints {
		if methodPolicy.Decision(endpoint.Method) != ContextMethodDeny {
			graphqlEndpoints = append(graphqlEndpoints, endpoint)
		}
	}
	policy.GraphQLEndpoints = graphqlEndpoints
	mcpEndpoints := make([]ContextPolicyExactRule, 0, len(policy.MCPEndpoints))
	for _, endpoint := range policy.MCPEndpoints {
		if methodPolicy.Decision(endpoint.Method) != ContextMethodDeny {
			mcpEndpoints = append(mcpEndpoints, endpoint)
		}
	}
	policy.MCPEndpoints = mcpEndpoints
	normalized, _, _, err := NormalizeContextPolicy(policy)
	return normalized, err
}

func defaultContextPolicy() ContextPolicy {
	return ContextPolicy{
		SchemaVersion:      ContextPolicySchemaVersion,
		Name:               "default",
		DestinationCeiling: ContextPolicyDestinationCeiling{Mode: "public_https", Authorities: []ContextPolicyAuthority{}},
		MethodPolicy:       ContextMethodPolicy{Default: ContextMethodExactReview, Overrides: []ContextMethodOverride{}},
		BaselineGrants:     agentReadyBaselineGrants(),
		BaselineTemplates:  agentReadyBaselineTemplates(),
		MCPBaselineGrants:  agentReadyMCPBaselineGrants(),
		BaselineDenies:     []ContextPolicyExactRule{},
		GraphQLEndpoints:   agentReadyGraphQLEndpoints(),
		MCPEndpoints:       agentReadyMCPEndpoints(),
	}
}

// nativeToolAuthReadiness is a compile-time compatibility bundle for one
// reviewed native client. Bundle names and process names never become runtime
// authority.
type nativeToolAuthReadiness struct {
	ID               string
	ClientVersion    string
	ContractRevision int
	BaselineGrants   []ContextPolicyExactRule
	GraphQLEndpoints []ContextPolicyExactRule
}

// nativeToolAuthReadinessFamily is the single reviewed registry entry for one
// native client. Contracts is append-only removal metadata; the current
// revision selects exactly one contract independently from the client pin.
type nativeToolAuthReadinessFamily struct {
	ID                      string
	CurrentContractRevision int
	Contracts               []nativeToolAuthReadiness
}

func nativeToolAuthReadinessBundles() []nativeToolAuthReadiness {
	var current []nativeToolAuthReadiness
	for _, family := range nativeToolAuthReadinessCatalog() {
		for _, bundle := range family.Contracts {
			if bundle.ID == family.ID && bundle.ContractRevision == family.CurrentContractRevision {
				current = append(current, bundle)
				break
			}
		}
	}
	return current
}

func nativeToolAuthReadinessHistory() []nativeToolAuthReadiness {
	var history []nativeToolAuthReadiness
	for _, family := range nativeToolAuthReadinessCatalog() {
		history = append(history, family.Contracts...)
	}
	return history
}

// DefaultContextPolicySnapshot returns the immutable Context-owned baseline.
// Native client readiness is deliberately absent: the trusted binary projects
// its current compatibility overlay at aggregate generation.
func DefaultContextPolicySnapshot() (ContextPolicy, bool) {
	normalized, _, _, err := NormalizeContextPolicy(withoutHistoricalNativeToolAuthReadiness(defaultContextPolicy()))
	if err != nil {
		return ContextPolicy{}, false
	}
	return normalized, true
}

// ApplyNativeToolAuthReadiness validates an immutable Context snapshot and,
// when enabled, replaces every historically snapshotted native readiness rule
// with the current compile-time bundle set. The Context policy's terminal
// destination and method decisions remain unchanged and authoritative over
// this overlay.
func ApplyNativeToolAuthReadiness(enabled bool, replaceHistorical bool, snapshot ContextPolicy) (ContextPolicy, error) {
	normalized, _, _, err := NormalizeContextPolicy(snapshot)
	if err != nil {
		return ContextPolicy{}, err
	}
	effective := normalized
	if replaceHistorical {
		effective = withoutHistoricalNativeToolAuthReadiness(effective)
	}
	if !enabled {
		return effective, nil
	}
	for _, bundle := range nativeToolAuthReadinessBundles() {
		for _, rule := range bundle.BaselineGrants {
			if contextPolicyRuleInsideDestination(effective.DestinationCeiling, rule) &&
				effective.MethodPolicy.Decision(rule.Method) != ContextMethodDeny {
				if !slices.Contains(effective.BaselineGrants, rule) {
					effective.BaselineGrants = append(effective.BaselineGrants, rule)
				}
			}
		}
		for _, endpoint := range bundle.GraphQLEndpoints {
			if contextPolicyRuleInsideDestination(effective.DestinationCeiling, endpoint) &&
				effective.MethodPolicy.Decision(endpoint.Method) != ContextMethodDeny {
				if !slices.Contains(effective.GraphQLEndpoints, endpoint) {
					effective.GraphQLEndpoints = append(effective.GraphQLEndpoints, endpoint)
				}
			}
		}
	}
	effective, _, _, err = NormalizeContextPolicy(effective)
	return effective, err
}

func withoutHistoricalNativeToolAuthReadiness(policy ContextPolicy) ContextPolicy {
	historicalGrants := make(map[ContextPolicyExactRule]struct{})
	historicalGraphQLEndpoints := make(map[ContextPolicyExactRule]struct{})
	for _, bundle := range nativeToolAuthReadinessHistory() {
		for _, rule := range bundle.BaselineGrants {
			historicalGrants[rule] = struct{}{}
		}
		for _, endpoint := range bundle.GraphQLEndpoints {
			historicalGraphQLEndpoints[endpoint] = struct{}{}
		}
	}
	grants := make([]ContextPolicyExactRule, 0, len(policy.BaselineGrants))
	for _, rule := range policy.BaselineGrants {
		if _, historical := historicalGrants[rule]; !historical {
			grants = append(grants, rule)
		}
	}
	endpoints := make([]ContextPolicyExactRule, 0, len(policy.GraphQLEndpoints))
	for _, endpoint := range policy.GraphQLEndpoints {
		if _, historical := historicalGraphQLEndpoints[endpoint]; !historical {
			endpoints = append(endpoints, endpoint)
		}
	}
	policy.BaselineGrants = grants
	policy.GraphQLEndpoints = endpoints
	return policy
}

// agentReadyBaselineGrants is coupled to reviewed native tool versions. The
// canonical base runtime supplies Claude Code, Codex, and GitHub CLI; TWG and
// pup readiness apply when their pinned versions are supplied by a custom runtime.
// These are exact HTTP or declared semantic grants, not process identity:
// every process in the Context receives the same exact effect decisions.
// Native first-party discovery and control
// plane routes are included; acquisition, file transfer, and self-update stay out.
func agentReadyBaselineGrants() []ContextPolicyExactRule {
	grants := []ContextPolicyExactRule{
		{Scheme: "https", Host: "ab.chatgpt.com", Port: 443, Method: "POST", Path: "/otlp/v1/metrics"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/claude_cli/bootstrap"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/claude_code/policy_limits"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/claude_code/settings"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/organization/claude_code_first_token_date"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/claude_code_penguin_mode"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/claude_code/organizations/metrics_enabled"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/mcp-registry/v0/servers"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/v1/mcp_servers"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/hello"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "HEAD", Path: "/api/hello"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "GET", Path: "/api/oauth/usage"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "POST", Path: "/api/event_logging/v2/batch"},
		{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "POST", Path: "/v1/messages"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/codex/models"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/codex/responses"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/plugins/featured"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/plugins/export/curated"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/connectors/directory/list"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/ps/plugins/list"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/ps/plugins/installed"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/ps/plugins/suggested"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/codex/analytics-events/events"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/codex/responses"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/wham/rate-limit-reset-credits"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/wham/settings/user"},
		{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "GET", Path: "/backend-api/wham/usage"},
	}
	for _, bundle := range nativeToolAuthReadinessBundles() {
		grants = append(grants, bundle.BaselineGrants...)
	}
	return grants
}

func agentReadyBaselineTemplates() []ContextPolicyPathTemplateRule {
	return []ContextPolicyPathTemplateRule{{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "POST", Path: "/api/eval/{id}", Segments: []string{"api", "eval", "{id}"}}}
}

func agentReadyGraphQLEndpoints() []ContextPolicyExactRule {
	var endpoints []ContextPolicyExactRule
	for _, bundle := range nativeToolAuthReadinessBundles() {
		endpoints = append(endpoints, bundle.GraphQLEndpoints...)
	}
	return endpoints
}

func agentReadyMCPEndpoints() []ContextPolicyExactRule {
	return []ContextPolicyExactRule{{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/ps/mcp"}}
}

func agentReadyMCPBaselineGrants() []ContextPolicyMCPRule {
	methods := []string{"initialize", "notifications/initialized", "ping", "tools/list", "resources/list", "resources/templates/list", "prompts/list"}
	rules := make([]ContextPolicyMCPRule, 0, len(methods))
	for _, method := range methods {
		rules = append(rules, ContextPolicyMCPRule{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/ps/mcp", MCPMethod: method})
	}
	return rules
}

func PolicyRevision(p ContextPolicy) (string, error) {
	_, _, revision, err := NormalizeContextPolicy(p)
	return revision, err
}

func DefaultContextPolicyRevision() string {
	policy, _ := DefaultContextPolicySnapshot()
	revision, _ := PolicyRevision(policy)
	return revision
}
