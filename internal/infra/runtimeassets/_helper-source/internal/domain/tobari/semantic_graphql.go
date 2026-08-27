package tobari

import (
	"fmt"
	"strings"
)

// SemanticHTTPEndpoint declares one trusted classification location. Hosts is
// finite and exact; endpoint declarations do not grant request authority.
type SemanticHTTPEndpoint struct {
	SemanticRuleAuthority
	Path string `json:"path"`
}

func (e SemanticHTTPEndpoint) Validate() error {
	if err := e.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if err := validateGraphQLEndpointPath(e.Path); err != nil {
		return fmt.Errorf("semantic HTTP endpoint path is invalid")
	}
	return nil
}

func (e SemanticHTTPEndpoint) canonicalKey() string {
	return e.SemanticRuleAuthority.canonicalKey() + "\x00" + e.Path
}

func (e SemanticHTTPEndpoint) declaresHost(ruleAuthority SemanticRuleAuthority, path, requestedHost string) bool {
	if e.Scheme != ruleAuthority.Scheme || e.Port != ruleAuthority.Port || e.Path != path {
		return false
	}
	for _, host := range e.hosts() {
		if host == requestedHost {
			return true
		}
	}
	return false
}

type SemanticGraphQLRule struct {
	SemanticRuleAuthority
	Path          string `json:"path"`
	OperationType string `json:"operation_type"`
	RootField     string `json:"root_field"`
}

func (r SemanticGraphQLRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if err := validateGraphQLEndpointPath(r.Path); err != nil {
		return fmt.Errorf("semantic GraphQL path is invalid")
	}
	return (PolicyProtocolIdentity{
		Scheme: r.Scheme, Protocol: PolicyProtocolGraphQL,
		GraphQLOperationType: r.OperationType, GraphQLRootField: r.RootField,
	}).Validate()
}

func (r SemanticGraphQLRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleGraphQL {
		return false
	}
	return r.SemanticRuleAuthority.matches(effect) && effect.Method == "POST" && r.Path == effect.Path &&
		r.OperationType == effect.Identity.GraphQLOperationType && r.RootField == effect.Identity.GraphQLRootField
}

func (r SemanticGraphQLRule) canonicalKey() string {
	return strings.Join([]string{r.SemanticRuleAuthority.canonicalKey(), r.Path, r.OperationType, r.RootField}, "\x00")
}

func (r SemanticGraphQLRule) covers(other SemanticGraphQLRule) bool {
	if r.Scheme != other.Scheme || r.Port != other.Port || r.Path != other.Path || r.OperationType != other.OperationType || r.RootField != other.RootField {
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

type SemanticGraphQLRuleSet struct {
	Rules []SemanticGraphQLRule `json:"rules"`
}

func (s SemanticGraphQLRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic GraphQL rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic GraphQL rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticGraphQLPolicy struct {
	Endpoints []SemanticHTTPEndpoint `json:"endpoints"`
	Allow     SemanticGraphQLRuleSet `json:"allow"`
	Deny      SemanticGraphQLRuleSet `json:"deny"`
}

func (p SemanticGraphQLPolicy) Validate() error {
	if p.Endpoints == nil {
		return fmt.Errorf("semantic GraphQL endpoint collection must be explicit")
	}
	seenEndpoints := make(map[string]struct{}, len(p.Endpoints))
	for _, endpoint := range p.Endpoints {
		if err := endpoint.Validate(); err != nil {
			return err
		}
		key := endpoint.canonicalKey()
		if _, duplicate := seenEndpoints[key]; duplicate {
			return fmt.Errorf("semantic GraphQL endpoint is duplicated")
		}
		seenEndpoints[key] = struct{}{}
	}
	if err := p.Allow.Validate(); err != nil {
		return err
	}
	if err := p.Deny.Validate(); err != nil {
		return err
	}
	for _, rules := range [][]SemanticGraphQLRule{p.Allow.Rules, p.Deny.Rules} {
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
					return fmt.Errorf("semantic GraphQL rule has no declared endpoint")
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
			return fmt.Errorf("semantic GraphQL Allow is fully shadowed by Deny")
		}
	}
	return nil
}
