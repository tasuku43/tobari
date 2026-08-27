package tobari

import (
	"fmt"
	"sort"
	"strings"
)

type SemanticOCIRule struct {
	SemanticRuleAuthority
	Action     string `json:"action"`
	Repository string `json:"repository"`
	Object     string `json:"object"`
}

func (r SemanticOCIRule) identity() PolicyProtocolIdentity {
	return PolicyProtocolIdentity{
		Scheme: r.Scheme, Protocol: PolicyProtocolOCI,
		OCIAction: r.Action, OCIRepository: r.Repository, OCIObject: r.Object,
	}
}

func (r SemanticOCIRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	return r.identity().Validate()
}

func (r SemanticOCIRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleOCI || !r.SemanticRuleAuthority.matches(effect) {
		return false
	}
	return r.identity().matches(effect.Identity)
}

func (r SemanticOCIRule) canonicalKey() string {
	hosts := r.hosts()
	sort.Strings(hosts)
	return strings.Join([]string{r.Scheme, strings.Join(hosts, "\x1f"), fmt.Sprintf("%d", r.Port), r.Action, r.Repository, r.Object}, "\x00")
}

func (r SemanticOCIRule) covers(other SemanticOCIRule) bool {
	if r.Scheme != other.Scheme || r.Port != other.Port || !r.identity().matches(other.identity()) {
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

type SemanticOCIRuleSet struct {
	Rules []SemanticOCIRule `json:"rules"`
}

func (s SemanticOCIRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic OCI rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic OCI rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticOCIPolicy struct {
	Allow SemanticOCIRuleSet `json:"allow"`
	Deny  SemanticOCIRuleSet `json:"deny"`
}

func (p SemanticOCIPolicy) Validate() error {
	if err := p.Allow.Validate(); err != nil {
		return err
	}
	if err := p.Deny.Validate(); err != nil {
		return err
	}
	for _, allow := range p.Allow.Rules {
		fullyCovered := true
		for _, host := range allow.hosts() {
			narrowed := allow
			narrowed.Host, narrowed.Hosts = host, nil
			covered := false
			for _, deny := range p.Deny.Rules {
				if deny.covers(narrowed) {
					covered = true
					break
				}
			}
			if !covered {
				fullyCovered = false
				break
			}
		}
		if fullyCovered {
			return fmt.Errorf("semantic OCI Allow is fully shadowed by Deny")
		}
	}
	return nil
}
