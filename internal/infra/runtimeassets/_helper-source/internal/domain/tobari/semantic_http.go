package tobari

import (
	"fmt"
	"sort"
	"strings"
)

// SemanticRequestEffect is the complete secret-free request projection shared
// by static Template policy and exact Context Policy Memory. A semantic module
// owns only the refinement stored in Identity; the HTTP transport dimensions
// remain mandatory for every module.
type SemanticRequestEffect struct {
	Scheme   string                 `json:"scheme"`
	Host     string                 `json:"host"`
	Port     int                    `json:"port"`
	Method   string                 `json:"method"`
	Path     string                 `json:"path"`
	Identity PolicyProtocolIdentity `json:"identity"`
}

func (e SemanticRequestEffect) Validate() error {
	if err := e.Identity.Validate(); err != nil {
		return fmt.Errorf("semantic request identity: %w", err)
	}
	if e.Scheme != e.Identity.Scheme {
		return fmt.Errorf("semantic request scheme does not match its module identity")
	}
	if !validNormalizedPolicyHost(e.Host) || e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("semantic request authority is invalid")
	}
	if !httpMethodPattern.MatchString(e.Method) {
		return fmt.Errorf("semantic request method is invalid")
	}
	if err := validatePolicyPath(e.Path); err != nil {
		return fmt.Errorf("semantic request path is invalid")
	}
	module, ok := semanticModuleByProtocol(e.Identity.Protocol)
	if !ok {
		return fmt.Errorf("semantic request module is invalid")
	}
	return module.validateEffect(e)
}

func semanticExactEffectMatches(left, right SemanticRequestEffect) bool {
	if left.Validate() != nil || right.Validate() != nil {
		return false
	}
	return left.Scheme == right.Scheme && left.Host == right.Host && left.Port == right.Port &&
		left.Method == right.Method && left.Path == right.Path && left.Identity.matches(right.Identity)
}

// SemanticRuleAuthority is a public static destination selector. Host and
// Hosts are deliberately exclusive; Hosts is normalized as a semantic set.
type SemanticRuleAuthority struct {
	Scheme string   `json:"scheme"`
	Host   string   `json:"host,omitempty"`
	Hosts  []string `json:"hosts,omitempty"`
	Port   int      `json:"port"`
}

func (a SemanticRuleAuthority) Validate() error {
	if a.Scheme != "https" {
		return fmt.Errorf("semantic static rule authority must use public HTTPS")
	}
	if (a.Host == "") == (a.Hosts == nil) {
		return fmt.Errorf("semantic rule requires exactly one of host or hosts")
	}
	hosts := a.Hosts
	if a.Host != "" {
		hosts = []string{a.Host}
	}
	if len(hosts) == 0 || a.Hosts != nil && len(hosts) < 2 {
		return fmt.Errorf("semantic rule hosts must contain at least two values")
	}
	seen := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		if err := (ManifestPolicyAuthority{Scheme: a.Scheme, Host: host, Port: a.Port}).Validate(); err != nil {
			return fmt.Errorf("semantic rule authority: %w", err)
		}
		if _, duplicate := seen[host]; duplicate {
			return fmt.Errorf("semantic rule host is duplicated")
		}
		seen[host] = struct{}{}
	}
	return nil
}

func (a SemanticRuleAuthority) hosts() []string {
	if a.Host != "" {
		return []string{a.Host}
	}
	return append([]string{}, a.Hosts...)
}

func (a SemanticRuleAuthority) matches(effect SemanticRequestEffect) bool {
	if a.Scheme != effect.Scheme || a.Port != effect.Port {
		return false
	}
	for _, host := range a.hosts() {
		if host == effect.Host {
			return true
		}
	}
	return false
}

func (a SemanticRuleAuthority) canonicalKey() string {
	hosts := a.hosts()
	sort.Strings(hosts)
	return strings.Join([]string{a.Scheme, strings.Join(hosts, "\x1f"), fmt.Sprintf("%d", a.Port)}, "\x00")
}

// SemanticHTTPRule is one generic HTTP static matcher. Path may be exact or
// end in exactly one complete `{id}` segment. It never matches a request that
// has already been classified into a more-specific semantic module.
type SemanticHTTPRule struct {
	SemanticRuleAuthority
	Method string `json:"method"`
	Path   string `json:"path"`
}

func (r SemanticHTTPRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if !httpMethodPattern.MatchString(r.Method) {
		return fmt.Errorf("semantic HTTP method is invalid")
	}
	if strings.Contains(r.Path, "{id}") {
		segments := strings.Split(strings.TrimPrefix(r.Path, "/"), "/")
		if !strings.HasPrefix(r.Path, "/") || len(segments) < 2 || len(segments) > 32 || segments[len(segments)-1] != "{id}" {
			return fmt.Errorf("semantic HTTP path template is invalid")
		}
		wildcards := 0
		for _, segment := range segments {
			if segment == "{id}" {
				wildcards++
				continue
			}
			if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\\%?#`) {
				return fmt.Errorf("semantic HTTP path template is invalid")
			}
		}
		if wildcards != 1 {
			return fmt.Errorf("semantic HTTP path template is invalid")
		}
		return nil
	}
	if err := validatePolicyPath(r.Path); err != nil {
		return fmt.Errorf("semantic HTTP exact path is invalid")
	}
	return nil
}

func (r SemanticHTTPRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleHTTPGeneric ||
		!r.SemanticRuleAuthority.matches(effect) || r.Method != effect.Method {
		return false
	}
	if !strings.Contains(r.Path, "{id}") {
		return r.Path == effect.Path
	}
	return pathTemplateMatches(strings.Split(strings.TrimPrefix(r.Path, "/"), "/"), effect.Path)
}

func (r SemanticHTTPRule) canonicalKey() string {
	return strings.Join([]string{r.SemanticRuleAuthority.canonicalKey(), r.Method, r.Path}, "\x00")
}

func (r SemanticHTTPRule) covers(other SemanticHTTPRule) bool {
	if r.Scheme != other.Scheme || r.Port != other.Port || r.Method != other.Method {
		return false
	}
	coveredHosts := make(map[string]struct{}, len(r.hosts()))
	for _, host := range r.hosts() {
		coveredHosts[host] = struct{}{}
	}
	for _, host := range other.hosts() {
		if _, covered := coveredHosts[host]; !covered {
			return false
		}
	}
	if r.Path == other.Path {
		return true
	}
	if !strings.Contains(r.Path, "{id}") || strings.Contains(other.Path, "{id}") {
		return false
	}
	return pathTemplateMatches(strings.Split(strings.TrimPrefix(r.Path, "/"), "/"), other.Path)
}

type SemanticHTTPRuleSet struct {
	Rules []SemanticHTTPRule `json:"rules"`
}

func (s SemanticHTTPRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic HTTP rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic HTTP rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticHTTPPolicy struct {
	Allow SemanticHTTPRuleSet `json:"allow"`
	Deny  SemanticHTTPRuleSet `json:"deny"`
}

func (p SemanticHTTPPolicy) Validate() error {
	if err := p.Allow.Validate(); err != nil {
		return err
	}
	if err := p.Deny.Validate(); err != nil {
		return err
	}
	for _, allow := range p.Allow.Rules {
		allHostsCovered := true
		for _, host := range allow.hosts() {
			narrowed := allow
			narrowed.Host = host
			narrowed.Hosts = nil
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
			return fmt.Errorf("semantic HTTP Allow is fully shadowed by Deny")
		}
	}
	return nil
}
