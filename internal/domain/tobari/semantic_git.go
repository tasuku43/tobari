package tobari

import (
	"fmt"
	"sort"
	"strings"
)

type SemanticGitRule struct {
	SemanticRuleAuthority
	Service    string `json:"service"`
	Repository string `json:"repository"`
}

func (r SemanticGitRule) identity() PolicyProtocolIdentity {
	return PolicyProtocolIdentity{
		Scheme: r.Scheme, Protocol: PolicyProtocolGit,
		GitService: r.Service, GitRepository: r.Repository,
	}
}

func (r SemanticGitRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if err := r.identity().Validate(); err != nil {
		return err
	}
	return nil
}

func (r SemanticGitRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleGit || !r.SemanticRuleAuthority.matches(effect) {
		return false
	}
	return r.identity().matches(effect.Identity)
}

func (r SemanticGitRule) canonicalKey() string {
	hosts := r.hosts()
	sort.Strings(hosts)
	return strings.Join([]string{r.Scheme, strings.Join(hosts, "\x1f"), fmt.Sprintf("%d", r.Port), r.Service, r.Repository}, "\x00")
}

func (r SemanticGitRule) covers(other SemanticGitRule) bool {
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

type SemanticGitRuleSet struct {
	Rules []SemanticGitRule `json:"rules"`
}

func (s SemanticGitRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic Git rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic Git rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticGitPolicy struct {
	Allow SemanticGitRuleSet `json:"allow"`
	Deny  SemanticGitRuleSet `json:"deny"`
}

func (p SemanticGitPolicy) Validate() error {
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
			return fmt.Errorf("semantic Git Allow is fully shadowed by Deny")
		}
	}
	return nil
}
