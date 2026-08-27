package tobari

import (
	"fmt"
	"strings"
)

type SemanticMCPRule struct {
	SemanticRuleAuthority
	Path     string `json:"path"`
	Method   string `json:"method"`
	ToolName string `json:"tool_name,omitempty"`
}

func (r SemanticMCPRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if err := validateGraphQLEndpointPath(r.Path); err != nil {
		return fmt.Errorf("semantic MCP path is invalid")
	}
	return (PolicyProtocolIdentity{
		Scheme: r.Scheme, Protocol: PolicyProtocolMCP, MCPMethod: r.Method, MCPToolName: r.ToolName,
	}).Validate()
}

func (r SemanticMCPRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleMCP {
		return false
	}
	return r.SemanticRuleAuthority.matches(effect) && effect.Method == "POST" && r.Path == effect.Path &&
		r.Method == effect.Identity.MCPMethod && r.ToolName == effect.Identity.MCPToolName
}

func (r SemanticMCPRule) canonicalKey() string {
	return strings.Join([]string{r.SemanticRuleAuthority.canonicalKey(), r.Path, r.Method, r.ToolName}, "\x00")
}

func (r SemanticMCPRule) covers(other SemanticMCPRule) bool {
	if r.Scheme != other.Scheme || r.Port != other.Port || r.Path != other.Path || r.Method != other.Method || r.ToolName != other.ToolName {
		return false
	}
	covered := make(map[string]struct{}, len(r.hosts()))
	for _, host := range r.hosts() {
		covered[host] = struct{}{}
	}
	for _, host := range other.hosts() {
		if _, ok := covered[host]; !ok {
			return false
		}
	}
	return true
}

type SemanticMCPRuleSet struct {
	Rules []SemanticMCPRule `json:"rules"`
}

func (s SemanticMCPRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic MCP rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic MCP rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticMCPPolicy struct {
	Endpoints []SemanticHTTPEndpoint `json:"endpoints"`
	Allow     SemanticMCPRuleSet     `json:"allow"`
	Deny      SemanticMCPRuleSet     `json:"deny"`
}

func (p SemanticMCPPolicy) Validate() error {
	if p.Endpoints == nil {
		return fmt.Errorf("semantic MCP endpoint collection must be explicit")
	}
	seenEndpoints := make(map[string]struct{}, len(p.Endpoints))
	for _, endpoint := range p.Endpoints {
		if err := endpoint.Validate(); err != nil {
			return err
		}
		key := endpoint.canonicalKey()
		if _, duplicate := seenEndpoints[key]; duplicate {
			return fmt.Errorf("semantic MCP endpoint is duplicated")
		}
		seenEndpoints[key] = struct{}{}
	}
	if err := p.Allow.Validate(); err != nil {
		return err
	}
	if err := p.Deny.Validate(); err != nil {
		return err
	}
	for _, rules := range [][]SemanticMCPRule{p.Allow.Rules, p.Deny.Rules} {
		for _, rule := range rules {
			for _, ruleHost := range rule.hosts() {
				declared := false
				for _, endpoint := range p.Endpoints {
					if endpoint.declaresHost(rule.SemanticRuleAuthority, rule.Path, ruleHost) {
						declared = true
						break
					}
				}
				if !declared {
					return fmt.Errorf("semantic MCP rule has no declared endpoint")
				}
			}
		}
	}
	for _, allow := range p.Allow.Rules {
		allHostsCovered := true
		for _, host := range allow.hosts() {
			narrowed := allow
			narrowed.Host, narrowed.Hosts = host, nil
			hostCovered := false
			for _, deny := range p.Deny.Rules {
				if deny.covers(narrowed) {
					hostCovered = true
					break
				}
			}
			if !hostCovered {
				allHostsCovered = false
				break
			}
		}
		if allHostsCovered {
			return fmt.Errorf("semantic MCP Allow is fully shadowed by Deny")
		}
	}
	return nil
}
