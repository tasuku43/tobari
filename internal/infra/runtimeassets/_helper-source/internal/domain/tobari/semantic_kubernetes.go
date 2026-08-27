package tobari

import (
	"fmt"
	"sort"
	"strings"
)

type SemanticKubernetesResourceRule struct {
	Group       string `json:"group"`
	Version     string `json:"version"`
	Resource    string `json:"resource"`
	Namespace   string `json:"namespace,omitempty"`
	Name        string `json:"name,omitempty"`
	Subresource string `json:"subresource,omitempty"`
	Verb        string `json:"verb"`
	DryRun      string `json:"dry_run"`
}

func (r SemanticKubernetesResourceRule) identity(scheme string) PolicyProtocolIdentity {
	return PolicyProtocolIdentity{
		Scheme: scheme, Protocol: PolicyProtocolKubernetes, KubernetesKind: KubernetesRequestResource,
		KubernetesGroup: r.Group, KubernetesVersion: r.Version, KubernetesResource: r.Resource,
		KubernetesNamespace: r.Namespace, KubernetesName: r.Name, KubernetesSubresource: r.Subresource,
		KubernetesVerb: r.Verb, KubernetesDryRun: r.DryRun,
	}
}

type SemanticKubernetesNonResourceRule struct {
	Path string `json:"path"`
	Verb string `json:"verb"`
}

func (r SemanticKubernetesNonResourceRule) identity(scheme string) PolicyProtocolIdentity {
	return PolicyProtocolIdentity{
		Scheme: scheme, Protocol: PolicyProtocolKubernetes, KubernetesKind: KubernetesRequestNonResource,
		KubernetesVerb: r.Verb, KubernetesNonResourcePath: r.Path,
	}
}

type SemanticKubernetesRule struct {
	SemanticRuleAuthority
	Resource    *SemanticKubernetesResourceRule    `json:"resource,omitempty"`
	NonResource *SemanticKubernetesNonResourceRule `json:"non_resource,omitempty"`
}

func (r SemanticKubernetesRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if r.Port != 443 {
		return fmt.Errorf("semantic Kubernetes authority must use the classified port")
	}
	for _, host := range r.hosts() {
		if !strings.HasSuffix(host, ".eks.amazonaws.com") {
			return fmt.Errorf("semantic Kubernetes authority is outside the EKS classifier")
		}
	}
	if (r.Resource == nil) == (r.NonResource == nil) {
		return fmt.Errorf("semantic Kubernetes rule requires exactly one request variant")
	}
	if r.Resource != nil {
		return r.Resource.identity(r.Scheme).Validate()
	}
	return r.NonResource.identity(r.Scheme).Validate()
}

func (r SemanticKubernetesRule) identity() PolicyProtocolIdentity {
	if r.Resource != nil {
		return r.Resource.identity(r.Scheme)
	}
	if r.NonResource != nil {
		return r.NonResource.identity(r.Scheme)
	}
	return PolicyProtocolIdentity{}
}

func (r SemanticKubernetesRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleKubernetes || !r.SemanticRuleAuthority.matches(effect) {
		return false
	}
	return r.identity().matches(effect.Identity)
}

func (r SemanticKubernetesRule) canonicalKey() string {
	hosts := r.hosts()
	sort.Strings(hosts)
	i := r.identity()
	return strings.Join([]string{r.Scheme, strings.Join(hosts, "\x1f"), fmt.Sprintf("%d", r.Port), i.KubernetesKind, i.KubernetesVerb, i.KubernetesGroup, i.KubernetesVersion, i.KubernetesResource, i.KubernetesNamespace, i.KubernetesName, i.KubernetesSubresource, i.KubernetesDryRun, i.KubernetesNonResourcePath}, "\x00")
}

func (r SemanticKubernetesRule) covers(other SemanticKubernetesRule) bool {
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

type SemanticKubernetesRuleSet struct {
	Rules []SemanticKubernetesRule `json:"rules"`
}

func (s SemanticKubernetesRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic Kubernetes rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic Kubernetes rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticKubernetesPolicy struct {
	Allow SemanticKubernetesRuleSet `json:"allow"`
	Deny  SemanticKubernetesRuleSet `json:"deny"`
}

func (p SemanticKubernetesPolicy) Validate() error {
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
			return fmt.Errorf("semantic Kubernetes Allow is fully shadowed by Deny")
		}
	}
	return nil
}
