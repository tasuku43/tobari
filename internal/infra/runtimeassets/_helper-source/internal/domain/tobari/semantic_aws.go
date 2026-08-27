package tobari

import (
	"fmt"
	"sort"
	"strings"
)

type SemanticAWSRule struct {
	SemanticRuleAuthority
	WireProtocol    string   `json:"wire_protocol"`
	Service         string   `json:"service,omitempty"`
	Services        []string `json:"services,omitempty"`
	ProtocolVersion string   `json:"protocol_version,omitempty"`
	TargetNamespace string   `json:"target_namespace,omitempty"`
	Operation       string   `json:"operation"`
}

func (r SemanticAWSRule) Validate() error {
	if err := r.SemanticRuleAuthority.Validate(); err != nil {
		return err
	}
	if r.Port != 443 {
		return fmt.Errorf("semantic AWS authority must use the classified port")
	}
	for _, host := range r.hosts() {
		if !strings.HasSuffix(host, ".amazonaws.com") {
			return fmt.Errorf("semantic AWS authority is outside the classifier")
		}
	}
	if (r.Service == "") == (r.Services == nil) {
		return fmt.Errorf("semantic AWS rule requires exactly one of service or services")
	}
	services := r.services()
	if len(services) == 0 || r.Services != nil && len(services) < 2 {
		return fmt.Errorf("semantic AWS services must contain at least two values")
	}
	seen := make(map[string]struct{}, len(services))
	for _, service := range services {
		if !awsServicePattern.MatchString(service) {
			return fmt.Errorf("semantic AWS service is invalid")
		}
		if _, duplicate := seen[service]; duplicate {
			return fmt.Errorf("semantic AWS service is duplicated")
		}
		seen[service] = struct{}{}
	}
	operation := strings.TrimSuffix(r.Operation, "*")
	if operation == "" || strings.Count(r.Operation, "*") > 1 || strings.Contains(operation, "*") || !awsQueryOperationPattern.MatchString(operation) {
		return fmt.Errorf("semantic AWS operation matcher is invalid")
	}
	switch r.WireProtocol {
	case AWSWireProtocolQuery:
		if !awsProtocolVersionPattern.MatchString(r.ProtocolVersion) || r.TargetNamespace != "" {
			return fmt.Errorf("semantic AWS Query projection is invalid")
		}
	case AWSWireProtocolJSON:
		if r.ProtocolVersion != "" || len(r.TargetNamespace)+1+len(operation) > 256 || !awsTargetNamespacePattern.MatchString(r.TargetNamespace) {
			return fmt.Errorf("semantic AWS JSON projection is invalid")
		}
	default:
		return fmt.Errorf("semantic AWS wire protocol is invalid")
	}
	return nil
}

func (r SemanticAWSRule) services() []string {
	if r.Service != "" {
		return []string{r.Service}
	}
	return append([]string{}, r.Services...)
}

func (r SemanticAWSRule) operationMatches(operation string) bool {
	if !strings.HasSuffix(r.Operation, "*") {
		return r.Operation == operation
	}
	return strings.HasPrefix(operation, strings.TrimSuffix(r.Operation, "*"))
}

func (r SemanticAWSRule) Matches(effect SemanticRequestEffect) bool {
	if err := r.Validate(); err != nil {
		return false
	}
	if err := effect.Validate(); err != nil || effect.Identity.SemanticModuleID() != SemanticModuleAWS || !r.SemanticRuleAuthority.matches(effect) {
		return false
	}
	identity := effect.Identity
	if r.WireProtocol != identity.AWSWireProtocol || r.ProtocolVersion != identity.AWSProtocolVersion || r.TargetNamespace != identity.AWSTargetNamespace || !r.operationMatches(identity.AWSOperation) {
		return false
	}
	for _, service := range r.services() {
		if service == identity.AWSService {
			return true
		}
	}
	return false
}

func (r SemanticAWSRule) canonicalKey() string {
	hosts := r.hosts()
	services := r.services()
	sort.Strings(hosts)
	sort.Strings(services)
	return strings.Join([]string{r.Scheme, strings.Join(hosts, "\x1f"), fmt.Sprintf("%d", r.Port), r.WireProtocol, strings.Join(services, "\x1f"), r.ProtocolVersion, r.TargetNamespace, r.Operation}, "\x00")
}

func (r SemanticAWSRule) covers(other SemanticAWSRule) bool {
	if r.Scheme != other.Scheme || r.Port != other.Port || r.WireProtocol != other.WireProtocol || r.ProtocolVersion != other.ProtocolVersion || r.TargetNamespace != other.TargetNamespace {
		return false
	}
	if strings.HasSuffix(r.Operation, "*") {
		if !strings.HasPrefix(strings.TrimSuffix(other.Operation, "*"), strings.TrimSuffix(r.Operation, "*")) {
			return false
		}
	} else if r.Operation != other.Operation {
		return false
	}
	coveredHosts := make(map[string]struct{}, len(r.hosts()))
	for _, host := range r.hosts() {
		coveredHosts[host] = struct{}{}
	}
	for _, host := range other.hosts() {
		if _, ok := coveredHosts[host]; !ok {
			return false
		}
	}
	coveredServices := make(map[string]struct{}, len(r.services()))
	for _, service := range r.services() {
		coveredServices[service] = struct{}{}
	}
	for _, service := range other.services() {
		if _, ok := coveredServices[service]; !ok {
			return false
		}
	}
	return true
}

type SemanticAWSRuleSet struct {
	Rules []SemanticAWSRule `json:"rules"`
}

func (s SemanticAWSRuleSet) Validate() error {
	if s.Rules == nil {
		return fmt.Errorf("semantic AWS rule collection must be explicit")
	}
	seen := make(map[string]struct{}, len(s.Rules))
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		key := rule.canonicalKey()
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("semantic AWS rule is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

type SemanticAWSPolicy struct {
	Allow SemanticAWSRuleSet `json:"allow"`
	Deny  SemanticAWSRuleSet `json:"deny"`
}

func (p SemanticAWSPolicy) Validate() error {
	if err := p.Allow.Validate(); err != nil {
		return err
	}
	if err := p.Deny.Validate(); err != nil {
		return err
	}
	for _, allow := range p.Allow.Rules {
		fullyCovered := true
		for _, host := range allow.hosts() {
			for _, service := range allow.services() {
				narrowed := allow
				narrowed.Host, narrowed.Hosts = host, nil
				narrowed.Service, narrowed.Services = service, nil
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
			if !fullyCovered {
				break
			}
		}
		if fullyCovered {
			return fmt.Errorf("semantic AWS Allow is fully shadowed by Deny")
		}
	}
	return nil
}
