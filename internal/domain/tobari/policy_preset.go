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
	PolicyPresetSchemaVersion = 1
	DefaultPolicyPresetOrigin = "builtin/reviewed-exact"

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
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Method string `json:"method"`
	Path   string `json:"path"`
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
	BaselineDenies     []PolicyPresetExactRule        `json:"baseline_denies"`
	GraphQLEndpoints   []PolicyPresetExactRule        `json:"graphql_endpoints"`
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
	if p.DestinationCeiling.Authorities == nil || p.MethodCeiling.Methods == nil || p.BaselineGrants == nil || p.BaselineDenies == nil || p.GraphQLEndpoints == nil {
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
	for _, rules := range [][]PolicyPresetExactRule{p.BaselineGrants, p.BaselineDenies, p.GraphQLEndpoints} {
		seen = map[string]struct{}{}
		for _, rule := range rules {
			if err := rule.Validate(); err != nil {
				return err
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
	clone.BaselineDenies = append([]PolicyPresetExactRule{}, p.BaselineDenies...)
	clone.GraphQLEndpoints = append([]PolicyPresetExactRule{}, p.GraphQLEndpoints...)
	sort.Slice(clone.DestinationCeiling.Authorities, func(i, j int) bool {
		a, b := clone.DestinationCeiling.Authorities[i], clone.DestinationCeiling.Authorities[j]
		return fmt.Sprintf("%s/%s/%05d", a.Scheme, a.Host, a.Port) < fmt.Sprintf("%s/%s/%05d", b.Scheme, b.Host, b.Port)
	})
	sort.Strings(clone.MethodCeiling.Methods)
	lessRule := func(a, b PolicyPresetExactRule) bool {
		return fmt.Sprintf("%s/%s/%05d/%s/%s", a.Scheme, a.Host, a.Port, a.Method, a.Path) < fmt.Sprintf("%s/%s/%05d/%s/%s", b.Scheme, b.Host, b.Port, b.Method, b.Path)
	}
	sort.Slice(clone.BaselineGrants, func(i, j int) bool { return lessRule(clone.BaselineGrants[i], clone.BaselineGrants[j]) })
	sort.Slice(clone.BaselineDenies, func(i, j int) bool { return lessRule(clone.BaselineDenies[i], clone.BaselineDenies[j]) })
	sort.Slice(clone.GraphQLEndpoints, func(i, j int) bool { return lessRule(clone.GraphQLEndpoints[i], clone.GraphQLEndpoints[j]) })
	data, err := json.MarshalIndent(clone, "", "  ")
	if err != nil {
		return PolicyPreset{}, nil, "", err
	}
	data = append(data, '\n')
	digest := sha256.Sum256(data)
	return clone, data, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func BuiltinPolicyPreset(origin string) (PolicyPreset, bool) {
	base := PolicyPreset{SchemaVersion: 1, DestinationCeiling: PolicyPresetDestinationCeiling{Mode: "public_https", Authorities: []PolicyPresetAuthority{}}, MethodCeiling: PolicyPresetMethodCeiling{Mode: "all", Methods: []string{}}, BaselineGrants: []PolicyPresetExactRule{}, BaselineDenies: []PolicyPresetExactRule{}, GraphQLEndpoints: []PolicyPresetExactRule{}}
	switch origin {
	case "builtin/offline":
		base.Name = "offline"
		base.Guardrail = PolicyPresetGuardrailOffline
	case DefaultPolicyPresetOrigin:
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

func PolicyPresetRevision(p PolicyPreset) (string, error) {
	_, _, revision, err := NormalizePolicyPreset(p)
	return revision, err
}

func DefaultPolicyPresetRevision() string {
	preset, _ := BuiltinPolicyPreset(DefaultPolicyPresetOrigin)
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
