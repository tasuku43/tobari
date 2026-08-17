package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	PolicyPresetSchemaVersion  = 1
	DefaultPolicyPresetOrigin  = "builtin/agent-ready"
	AgentReadyClaudeVersion    = "2.1.220"
	AgentReadyCodexVersion     = "0.147.0"
	AgentReadyGitHubCLIVersion = "2.96.0"
	AgentReadyPupVersion       = "1.10.7"
	AgentReadyTWGVersion       = "1.2.5"

	TaskPolicyPresetList     = "policy.preset.list"
	TaskPolicyPresetShow     = "policy.preset.show"
	TaskPolicyPresetInit     = "policy.preset.init"
	TaskPolicyPresetValidate = "policy.preset.validate"

	PolicyPresetCatalogTargetKind = "policy-preset-catalog"
	PolicyPresetCatalogTargetID   = "policy-preset-catalog"

	PolicyPresetGuardrailOffline         PolicyPresetGuardrail = "offline"
	PolicyPresetGuardrailReviewedExact   PolicyPresetGuardrail = "reviewed_exact"
	PolicyPresetGuardrailGetOnlyReviewed PolicyPresetGuardrail = "get_only_reviewed"
)

var (
	policyPresetNamePattern   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	policyPresetMethodPattern = regexp.MustCompile(`^[A-Z][A-Z0-9!#$%&'*+.^_` + "`" + `|~-]{0,31}$`)
)

type PolicyPresetGuardrail string

func (g PolicyPresetGuardrail) Validate() error {
	switch g {
	case PolicyPresetGuardrailOffline, PolicyPresetGuardrailReviewedExact, PolicyPresetGuardrailGetOnlyReviewed:
		return nil
	default:
		return fmt.Errorf("policy preset guardrail is invalid")
	}
}

type PolicyPresetAuthority struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

func (a PolicyPresetAuthority) Validate() error {
	if a.Scheme != "https" && a.Scheme != "http" {
		return fmt.Errorf("policy preset authority scheme is invalid")
	}
	if a.Port < 1 || a.Port > 65535 || policyPresetIPv4Literal(a.Host) || !validNormalizedPolicyHost(a.Host) || !strings.Contains(a.Host, ".") || policyPresetReservedHost(a.Host) {
		return fmt.Errorf("policy preset authority is not an exact public destination")
	}
	return nil
}

// policyPresetIPv4Literal keeps the domain validator independent from the net
// package while rejecting the only IP literal shape that can otherwise pass
// the normalized DNS-label grammar. IPv6 literals already fail that grammar.
func policyPresetIPv4Literal(host string) bool {
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

func policyPresetReservedHost(host string) bool {
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

type PolicyPresetExactRule struct {
	Scheme               string `json:"scheme"`
	Host                 string `json:"host"`
	Port                 int    `json:"port"`
	Method               string `json:"method"`
	Path                 string `json:"path"`
	Protocol             string `json:"protocol,omitempty"`
	GraphQLOperationType string `json:"graphql_operation_type,omitempty"`
	GraphQLRootField     string `json:"graphql_root_field,omitempty"`
}

// PolicyPresetPathTemplateRule is a reviewed built-in HTTP path shape. It is
// deliberately narrower than learned templates: exactly one direct {id}
// segment is permitted and no observed identifier is retained.
type PolicyPresetPathTemplateRule struct {
	Scheme   string   `json:"scheme"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Segments []string `json:"segments"`
}

func (r PolicyPresetPathTemplateRule) Validate() error {
	if err := (PolicyPresetExactRule{Scheme: r.Scheme, Host: r.Host, Port: r.Port, Method: r.Method, Path: r.Path}).Validate(); err != nil {
		return err
	}
	if len(r.Segments) < 2 || len(r.Segments) > 32 || r.Path != "/"+strings.Join(r.Segments, "/") {
		return fmt.Errorf("policy preset path template is invalid")
	}
	wildcards := 0
	for _, segment := range r.Segments {
		if segment == "{id}" {
			wildcards++
			continue
		}
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\\%?#`) {
			return fmt.Errorf("policy preset path template segment is invalid")
		}
	}
	if wildcards != 1 || r.Segments[len(r.Segments)-1] != "{id}" {
		return fmt.Errorf("policy preset path template must end in one identifier segment")
	}
	return nil
}

type PolicyPresetMCPRule struct {
	Scheme      string `json:"scheme"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Method      string `json:"method"`
	Path        string `json:"path"`
	MCPMethod   string `json:"mcp_method"`
	MCPToolName string `json:"mcp_tool_name,omitempty"`
}

func (r PolicyPresetMCPRule) Validate() error {
	if err := (PolicyPresetExactRule{Scheme: r.Scheme, Host: r.Host, Port: r.Port, Method: r.Method, Path: r.Path}).Validate(); err != nil {
		return err
	}
	if r.Method != "POST" {
		return fmt.Errorf("policy preset MCP transport method must be POST")
	}
	return (PolicyProtocolIdentity{Scheme: r.Scheme, Protocol: PolicyProtocolMCP, MCPMethod: r.MCPMethod, MCPToolName: r.MCPToolName}).Validate()
}

type PolicyPresetMethodCeiling struct {
	Mode    string   `json:"mode"`
	Methods []string `json:"methods"`
}

type PolicyPresetDestinationCeiling struct {
	Mode        string                  `json:"mode"`
	Authorities []PolicyPresetAuthority `json:"authorities"`
}

func (r PolicyPresetExactRule) Validate() error {
	if err := (PolicyPresetAuthority{Scheme: r.Scheme, Host: r.Host, Port: r.Port}).Validate(); err != nil {
		return err
	}
	if !policyPresetMethodPattern.MatchString(r.Method) || !strings.HasPrefix(r.Path, "/") || strings.ContainsAny(r.Path, "\r\n") {
		return fmt.Errorf("policy preset exact rule is invalid")
	}
	if r.Protocol == "" {
		if r.GraphQLOperationType != "" || r.GraphQLRootField != "" {
			return fmt.Errorf("policy preset HTTP rule has semantic fields")
		}
		return nil
	}
	if r.Protocol != PolicyProtocolGraphQL || r.Method != "POST" {
		return fmt.Errorf("policy preset semantic exact rule is invalid")
	}
	if err := (PolicyProtocolIdentity{Scheme: r.Scheme, Protocol: r.Protocol, GraphQLOperationType: r.GraphQLOperationType, GraphQLRootField: r.GraphQLRootField}).Validate(); err != nil {
		return err
	}
	return nil
}

// PolicyPreset is strict non-executable schema-V1 owner data. Empty
// collections are present in normalized bytes so revisions do not depend on
// decoder omission behavior.
type PolicyPreset struct {
	SchemaVersion      int                            `json:"schema_version"`
	Name               string                         `json:"name"`
	Guardrail          PolicyPresetGuardrail          `json:"guardrail"`
	DestinationCeiling PolicyPresetDestinationCeiling `json:"destination_ceiling"`
	MethodCeiling      PolicyPresetMethodCeiling      `json:"method_ceiling"`
	BaselineGrants     []PolicyPresetExactRule        `json:"baseline_grants"`
	BaselineTemplates  []PolicyPresetPathTemplateRule `json:"baseline_templates"`
	MCPBaselineGrants  []PolicyPresetMCPRule          `json:"mcp_baseline_grants"`
	BaselineDenies     []PolicyPresetExactRule        `json:"baseline_denies"`
	GraphQLEndpoints   []PolicyPresetExactRule        `json:"graphql_endpoints"`
	MCPEndpoints       []PolicyPresetExactRule        `json:"mcp_endpoints"`
}

func ValidatePolicyPresetOrigin(origin string) error {
	parts := strings.Split(origin, "/")
	if len(parts) != 2 || (parts[0] != "builtin" && parts[0] != "custom") || !policyPresetNamePattern.MatchString(parts[1]) {
		return fmt.Errorf("policy preset origin is invalid")
	}
	return nil
}

func (p PolicyPreset) Validate() error {
	if p.SchemaVersion != PolicyPresetSchemaVersion || !policyPresetNamePattern.MatchString(p.Name) {
		return fmt.Errorf("policy preset identity is invalid")
	}
	if err := p.Guardrail.Validate(); err != nil {
		return err
	}
	if p.DestinationCeiling.Authorities == nil || p.MethodCeiling.Methods == nil || p.BaselineGrants == nil || p.BaselineTemplates == nil || p.MCPBaselineGrants == nil || p.BaselineDenies == nil || p.GraphQLEndpoints == nil || p.MCPEndpoints == nil {
		return fmt.Errorf("policy preset collections must be explicit")
	}
	if p.DestinationCeiling.Mode != "public_https" && p.DestinationCeiling.Mode != "exact" {
		return fmt.Errorf("policy preset destination ceiling mode is invalid")
	}
	if p.MethodCeiling.Mode != "all" && p.MethodCeiling.Mode != "exact" {
		return fmt.Errorf("policy preset method ceiling mode is invalid")
	}
	if p.DestinationCeiling.Mode == "public_https" && len(p.DestinationCeiling.Authorities) != 0 {
		return fmt.Errorf("public_https destination ceiling cannot contain exact authorities")
	}
	if p.MethodCeiling.Mode == "all" && len(p.MethodCeiling.Methods) != 0 {
		return fmt.Errorf("all method ceiling cannot contain exact methods")
	}
	seen := map[string]struct{}{}
	for _, authority := range p.DestinationCeiling.Authorities {
		if err := authority.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s\x00%s\x00%d", authority.Scheme, authority.Host, authority.Port)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("policy preset authority is duplicated")
		}
		seen[key] = struct{}{}
	}
	seen = map[string]struct{}{}
	for _, method := range p.MethodCeiling.Methods {
		if !policyPresetMethodPattern.MatchString(method) {
			return fmt.Errorf("policy preset method is invalid")
		}
		if _, ok := seen[method]; ok {
			return fmt.Errorf("policy preset method is duplicated")
		}
		seen[method] = struct{}{}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.BaselineGrants {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path, rule.Protocol, rule.GraphQLOperationType, rule.GraphQLRootField)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("policy preset grant is duplicated")
		}
		seen[key] = struct{}{}
		if !policyPresetRuleInsideDestination(p.DestinationCeiling, rule) || (p.MethodCeiling.Mode == "exact" && !presetContainsMethod(p.MethodCeiling.Methods, rule.Method)) {
			return fmt.Errorf("policy preset rule exceeds its ceiling")
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
				return fmt.Errorf("policy preset GraphQL grant has no declared endpoint")
			}
		}
	}
	for _, rules := range [][]PolicyPresetExactRule{p.BaselineDenies, p.GraphQLEndpoints, p.MCPEndpoints} {
		seen = map[string]struct{}{}
		for _, rule := range rules {
			if err := rule.Validate(); err != nil {
				return err
			}
			if rule.Protocol != "" {
				return fmt.Errorf("policy preset endpoint or deny cannot contain semantic grant identity")
			}
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path)
			if _, ok := seen[key]; ok {
				return fmt.Errorf("policy preset rule is duplicated")
			}
			seen[key] = struct{}{}
			if !policyPresetRuleInsideDestination(p.DestinationCeiling, rule) {
				return fmt.Errorf("policy preset rule exceeds its destination ceiling")
			}
			if p.MethodCeiling.Mode == "exact" && !presetContainsMethod(p.MethodCeiling.Methods, rule.Method) {
				return fmt.Errorf("policy preset rule exceeds its method ceiling")
			}
		}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.BaselineTemplates {
		if err := rule.Validate(); err != nil {
			return err
		}
		exact := PolicyPresetExactRule{Scheme: rule.Scheme, Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("policy preset path template is duplicated")
		}
		seen[key] = struct{}{}
		if !policyPresetRuleInsideDestination(p.DestinationCeiling, exact) || (p.MethodCeiling.Mode == "exact" && !presetContainsMethod(p.MethodCeiling.Methods, rule.Method)) {
			return fmt.Errorf("policy preset path template exceeds its ceiling")
		}
	}
	seen = map[string]struct{}{}
	for _, rule := range p.MCPBaselineGrants {
		if err := rule.Validate(); err != nil {
			return err
		}
		exact := PolicyPresetExactRule{Scheme: rule.Scheme, Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", rule.Scheme, rule.Host, rule.Port, rule.Method, rule.Path, rule.MCPMethod, rule.MCPToolName)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("policy preset MCP grant is duplicated")
		}
		seen[key] = struct{}{}
		if !policyPresetRuleInsideDestination(p.DestinationCeiling, exact) || (p.MethodCeiling.Mode == "exact" && !presetContainsMethod(p.MethodCeiling.Methods, rule.Method)) {
			return fmt.Errorf("policy preset MCP grant exceeds its ceiling")
		}
		found := false
		for _, endpoint := range p.MCPEndpoints {
			if endpoint == exact {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("policy preset MCP grant has no declared endpoint")
		}
	}
	for _, endpoint := range p.GraphQLEndpoints {
		if endpoint.Method != "POST" {
			return fmt.Errorf("policy preset GraphQL endpoint method must be POST")
		}
	}
	for _, endpoint := range p.MCPEndpoints {
		if endpoint.Method != "POST" {
			return fmt.Errorf("policy preset MCP endpoint method must be POST")
		}
	}
	for _, graphql := range p.GraphQLEndpoints {
		for _, mcp := range p.MCPEndpoints {
			if graphql.Scheme == mcp.Scheme && graphql.Host == mcp.Host && graphql.Port == mcp.Port && graphql.Path == mcp.Path {
				return fmt.Errorf("policy preset semantic endpoint is ambiguous")
			}
		}
	}
	return nil
}

func policyPresetRuleInsideDestination(ceiling PolicyPresetDestinationCeiling, rule PolicyPresetExactRule) bool {
	if ceiling.Mode == "public_https" {
		return rule.Scheme == "https"
	}
	return presetContainsAuthority(ceiling.Authorities, rule)
}

func presetContainsAuthority(authorities []PolicyPresetAuthority, rule PolicyPresetExactRule) bool {
	for _, authority := range authorities {
		if authority.Scheme == rule.Scheme && authority.Host == rule.Host && authority.Port == rule.Port {
			return true
		}
	}
	return false
}

func presetContainsMethod(methods []string, method string) bool {
	for _, candidate := range methods {
		if candidate == method {
			return true
		}
	}
	return false
}

func NormalizePolicyPreset(p PolicyPreset) (PolicyPreset, []byte, string, error) {
	if err := p.Validate(); err != nil {
		return PolicyPreset{}, nil, "", err
	}
	clone := p
	clone.DestinationCeiling.Authorities = append([]PolicyPresetAuthority{}, p.DestinationCeiling.Authorities...)
	clone.MethodCeiling.Methods = append([]string{}, p.MethodCeiling.Methods...)
	clone.BaselineGrants = append([]PolicyPresetExactRule{}, p.BaselineGrants...)
	clone.BaselineTemplates = append([]PolicyPresetPathTemplateRule{}, p.BaselineTemplates...)
	clone.MCPBaselineGrants = append([]PolicyPresetMCPRule{}, p.MCPBaselineGrants...)
	clone.BaselineDenies = append([]PolicyPresetExactRule{}, p.BaselineDenies...)
	clone.GraphQLEndpoints = append([]PolicyPresetExactRule{}, p.GraphQLEndpoints...)
	clone.MCPEndpoints = append([]PolicyPresetExactRule{}, p.MCPEndpoints...)
	sort.Slice(clone.DestinationCeiling.Authorities, func(i, j int) bool {
		a, b := clone.DestinationCeiling.Authorities[i], clone.DestinationCeiling.Authorities[j]
		return fmt.Sprintf("%s/%s/%05d", a.Scheme, a.Host, a.Port) < fmt.Sprintf("%s/%s/%05d", b.Scheme, b.Host, b.Port)
	})
	sort.Strings(clone.MethodCeiling.Methods)
	lessRule := func(a, b PolicyPresetExactRule) bool {
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
		return PolicyPreset{}, nil, "", err
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	return clone, data, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func BuiltinPolicyPreset(origin string) (PolicyPreset, bool) {
	base := PolicyPreset{SchemaVersion: 1, DestinationCeiling: PolicyPresetDestinationCeiling{Mode: "public_https", Authorities: []PolicyPresetAuthority{}}, MethodCeiling: PolicyPresetMethodCeiling{Mode: "all", Methods: []string{}}, BaselineGrants: []PolicyPresetExactRule{}, BaselineTemplates: []PolicyPresetPathTemplateRule{}, MCPBaselineGrants: []PolicyPresetMCPRule{}, BaselineDenies: []PolicyPresetExactRule{}, GraphQLEndpoints: []PolicyPresetExactRule{}, MCPEndpoints: []PolicyPresetExactRule{}}
	switch origin {
	case "builtin/offline":
		base.Name = "offline"
		base.Guardrail = PolicyPresetGuardrailOffline
	case DefaultPolicyPresetOrigin:
		base.Name = "agent-ready"
		base.Guardrail = PolicyPresetGuardrailReviewedExact
		base.BaselineGrants = agentReadyBaselineGrants()
		base.BaselineTemplates = agentReadyBaselineTemplates()
		base.GraphQLEndpoints = agentReadyGraphQLEndpoints()
		base.MCPEndpoints = agentReadyMCPEndpoints()
		base.MCPBaselineGrants = agentReadyMCPBaselineGrants()
	case "builtin/reviewed-exact":
		base.Name = "reviewed-exact"
		base.Guardrail = PolicyPresetGuardrailReviewedExact
	case "builtin/get-only-reviewed":
		base.Name = "get-only-reviewed"
		base.Guardrail = PolicyPresetGuardrailGetOnlyReviewed
		base.MethodCeiling = PolicyPresetMethodCeiling{Mode: "exact", Methods: []string{"GET"}}
	default:
		return PolicyPreset{}, false
	}
	return base, true
}

// nativeToolAuthReadiness is a compile-time compatibility bundle for one
// reviewed native client. Bundle names and process names never become runtime
// authority.
type nativeToolAuthReadiness struct {
	ID               string
	ClientVersion    string
	ContractRevision int
	BaselineGrants   []PolicyPresetExactRule
	GraphQLEndpoints []PolicyPresetExactRule
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

// BuiltinPolicyPresetSnapshot returns the immutable Context-owned portion of a
// built-in preset. Native client readiness is deliberately absent: the trusted
// binary projects its current compatibility overlay at aggregate generation.
func BuiltinPolicyPresetSnapshot(origin string) (PolicyPreset, bool) {
	preset, ok := BuiltinPolicyPreset(origin)
	if !ok || origin != DefaultPolicyPresetOrigin {
		return preset, ok
	}
	return withoutHistoricalNativeToolAuthReadiness(preset), true
}

// ApplyNativeToolAuthReadiness validates an immutable Context snapshot and,
// only for builtin/agent-ready, replaces every historically snapshotted native
// readiness rule with the current compile-time bundle set.
func ApplyNativeToolAuthReadiness(origin string, snapshot PolicyPreset) (PolicyPreset, error) {
	if err := ValidatePolicyPresetOrigin(origin); err != nil {
		return PolicyPreset{}, err
	}
	normalized, _, _, err := NormalizePolicyPreset(snapshot)
	if err != nil {
		return PolicyPreset{}, err
	}
	if origin != DefaultPolicyPresetOrigin {
		return normalized, nil
	}
	if normalized.Name != "agent-ready" || normalized.Guardrail != PolicyPresetGuardrailReviewedExact {
		return PolicyPreset{}, fmt.Errorf("agent-ready snapshot identity is invalid")
	}
	effective := withoutHistoricalNativeToolAuthReadiness(normalized)
	for _, bundle := range nativeToolAuthReadinessBundles() {
		effective.BaselineGrants = append(effective.BaselineGrants, bundle.BaselineGrants...)
		effective.GraphQLEndpoints = append(effective.GraphQLEndpoints, bundle.GraphQLEndpoints...)
	}
	effective, _, _, err = NormalizePolicyPreset(effective)
	return effective, err
}

func withoutHistoricalNativeToolAuthReadiness(preset PolicyPreset) PolicyPreset {
	historicalGrants := make(map[PolicyPresetExactRule]struct{})
	historicalGraphQLEndpoints := make(map[PolicyPresetExactRule]struct{})
	for _, bundle := range nativeToolAuthReadinessHistory() {
		for _, rule := range bundle.BaselineGrants {
			historicalGrants[rule] = struct{}{}
		}
		for _, endpoint := range bundle.GraphQLEndpoints {
			historicalGraphQLEndpoints[endpoint] = struct{}{}
		}
	}
	grants := make([]PolicyPresetExactRule, 0, len(preset.BaselineGrants))
	for _, rule := range preset.BaselineGrants {
		if _, historical := historicalGrants[rule]; !historical {
			grants = append(grants, rule)
		}
	}
	endpoints := make([]PolicyPresetExactRule, 0, len(preset.GraphQLEndpoints))
	for _, endpoint := range preset.GraphQLEndpoints {
		if _, historical := historicalGraphQLEndpoints[endpoint]; !historical {
			endpoints = append(endpoints, endpoint)
		}
	}
	preset.BaselineGrants = grants
	preset.GraphQLEndpoints = endpoints
	return preset
}

// agentReadyBaselineGrants is coupled to reviewed native tool versions. The
// canonical base runtime supplies Claude Code, Codex, and GitHub CLI; TWG and
// pup readiness apply when their pinned versions are supplied by a custom runtime.
// These are exact HTTP or declared semantic grants, not process identity:
// every process in the Context receives the same exact effect decisions.
// Native first-party discovery and control
// plane routes are included; acquisition, file transfer, and self-update stay out.
func agentReadyBaselineGrants() []PolicyPresetExactRule {
	grants := []PolicyPresetExactRule{
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

func agentReadyBaselineTemplates() []PolicyPresetPathTemplateRule {
	return []PolicyPresetPathTemplateRule{{Scheme: "https", Host: "api.anthropic.com", Port: 443, Method: "POST", Path: "/api/eval/{id}", Segments: []string{"api", "eval", "{id}"}}}
}

func agentReadyGraphQLEndpoints() []PolicyPresetExactRule {
	var endpoints []PolicyPresetExactRule
	for _, bundle := range nativeToolAuthReadinessBundles() {
		endpoints = append(endpoints, bundle.GraphQLEndpoints...)
	}
	return endpoints
}

func agentReadyMCPEndpoints() []PolicyPresetExactRule {
	return []PolicyPresetExactRule{{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/ps/mcp"}}
}

func agentReadyMCPBaselineGrants() []PolicyPresetMCPRule {
	methods := []string{"initialize", "notifications/initialized", "ping", "tools/list", "resources/list", "resources/templates/list", "prompts/list"}
	rules := make([]PolicyPresetMCPRule, 0, len(methods))
	for _, method := range methods {
		rules = append(rules, PolicyPresetMCPRule{Scheme: "https", Host: "chatgpt.com", Port: 443, Method: "POST", Path: "/backend-api/ps/mcp", MCPMethod: method})
	}
	return rules
}

func PolicyPresetRevision(p PolicyPreset) (string, error) {
	_, _, revision, err := NormalizePolicyPreset(p)
	return revision, err
}

func DefaultPolicyPresetRevision() string {
	preset, _ := BuiltinPolicyPresetSnapshot(DefaultPolicyPresetOrigin)
	revision, _ := PolicyPresetRevision(preset)
	return revision
}

type PolicyPresetSummary struct {
	Origin              string                `json:"origin"`
	Revision            string                `json:"revision"`
	Guardrail           PolicyPresetGuardrail `json:"guardrail"`
	ImmediateGrantCount int                   `json:"immediate_grant_count"`
	DestinationCeiling  string                `json:"destination_ceiling"`
	DestinationCount    int                   `json:"destination_count"`
	MethodCeiling       string                `json:"method_ceiling"`
	MethodCount         int                   `json:"method_count"`
}

type PolicyPresetResult struct {
	Task        string                `json:"task"`
	Origin      string                `json:"origin,omitempty"`
	Revision    string                `json:"revision,omitempty"`
	Preset      *PolicyPreset         `json:"preset,omitempty"`
	Items       []PolicyPresetSummary `json:"items,omitempty"`
	SourcePath  string                `json:"source_path,omitempty"`
	Scope       string                `json:"scope,omitempty"`
	Limitations []string              `json:"limitations,omitempty"`
}
