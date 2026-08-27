package tobari

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	SemanticModuleHTTPGeneric = "protocols.http.generic"
	SemanticModuleGraphQL     = "protocols.http.graphql"
	SemanticModuleMCP         = "protocols.http.mcp"
	SemanticModuleAWS         = "providers.aws"
	SemanticModuleKubernetes  = "providers.kubernetes"
	SemanticModuleGit         = "protocols.http.git"
	SemanticModuleOCI         = "protocols.http.oci"

	SemanticClassificationMatched   = "matched"
	SemanticClassificationMalformed = "malformed"
)

var semanticModuleIDs = [...]string{
	SemanticModuleHTTPGeneric,
	SemanticModuleGraphQL,
	SemanticModuleMCP,
	SemanticModuleAWS,
	SemanticModuleKubernetes,
	SemanticModuleGit,
	SemanticModuleOCI,
}

// SemanticModuleIDs returns the canonical presentation order of the sealed
// compile-time module registry. Callers receive an independent copy.
func SemanticModuleIDs() []string { return append([]string{}, semanticModuleIDs[:]...) }

type semanticPolicyModule interface {
	id() string
	parent() string
	protocol() string
	validate(PolicyProtocolIdentity) error
	validateEffect(SemanticRequestEffect) error
	matches(PolicyProtocolIdentity, PolicyProtocolIdentity) bool
	appendIdentity([]string, PolicyProtocolIdentity) []string
	stateChange(PolicyProtocolIdentity) string
}

type policyModule struct {
	moduleID         string
	parentModuleID   string
	protocolID       string
	validateFn       func(PolicyProtocolIdentity) error
	validateEffectFn func(SemanticRequestEffect) error
	stateChangeFn    func(PolicyProtocolIdentity) string
	appendFn         func([]string, PolicyProtocolIdentity) []string
}

func (m policyModule) id() string       { return m.moduleID }
func (m policyModule) parent() string   { return m.parentModuleID }
func (m policyModule) protocol() string { return m.protocolID }
func (m policyModule) validate(i PolicyProtocolIdentity) error {
	return m.validateFn(i)
}
func (m policyModule) validateEffect(effect SemanticRequestEffect) error {
	if m.validateEffectFn == nil {
		return nil
	}
	return m.validateEffectFn(effect)
}
func (m policyModule) matches(left, right PolicyProtocolIdentity) bool {
	return left == right
}
func (m policyModule) appendIdentity(material []string, identity PolicyProtocolIdentity) []string {
	return m.appendFn(material, identity)
}
func (m policyModule) stateChange(identity PolicyProtocolIdentity) string {
	return m.stateChangeFn(identity)
}

var semanticModules = [...]semanticPolicyModule{
	policyModule{
		moduleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolHTTP,
		validateFn: func(i PolicyProtocolIdentity) error {
			if policyIdentityHasRefinement(i) {
				return fmt.Errorf("HTTP policy identity cannot contain semantic refinement fields")
			}
			return nil
		},
		stateChangeFn: func(PolicyProtocolIdentity) string { return PolicyStateChangeUnknown },
		appendFn:      func(material []string, _ PolicyProtocolIdentity) []string { return material },
	},
	policyModule{
		moduleID: SemanticModuleGraphQL, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolGraphQL,
		validateFn: func(i PolicyProtocolIdentity) error {
			if i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || kubernetesIdentityFieldsSet(i) || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
				return fmt.Errorf("GraphQL policy identity contains another module's fields")
			}
			if i.GraphQLOperationType != GraphQLOperationQuery && i.GraphQLOperationType != GraphQLOperationMutation {
				return fmt.Errorf("GraphQL operation type is invalid")
			}
			if len(i.GraphQLRootField) == 0 || len(i.GraphQLRootField) > 256 || !graphqlNamePattern.MatchString(i.GraphQLRootField) {
				return fmt.Errorf("GraphQL root field is invalid")
			}
			return nil
		},
		stateChangeFn: func(i PolicyProtocolIdentity) string {
			if i.GraphQLOperationType == GraphQLOperationQuery {
				return PolicyStateChangeNone
			}
			return PolicyStateChangePossible
		},
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolGraphQL, i.GraphQLOperationType, i.GraphQLRootField)
		},
	},
	policyModule{
		moduleID: SemanticModuleMCP, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolMCP,
		validateFn: func(i PolicyProtocolIdentity) error {
			if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || kubernetesIdentityFieldsSet(i) || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
				return fmt.Errorf("MCP policy identity contains another module's fields")
			}
			if len(i.MCPMethod) == 0 || len(i.MCPMethod) > 128 || !mcpMethodPattern.MatchString(i.MCPMethod) {
				return fmt.Errorf("MCP method is invalid")
			}
			if i.MCPMethod == "tools/call" {
				if len(i.MCPToolName) == 0 || len(i.MCPToolName) > 256 || !mcpToolNamePattern.MatchString(i.MCPToolName) {
					return fmt.Errorf("MCP tool name is invalid")
				}
			} else if i.MCPToolName != "" {
				return fmt.Errorf("MCP tool name is only valid for tools/call")
			}
			return nil
		},
		validateEffectFn: func(effect SemanticRequestEffect) error {
			if effect.Method != "POST" {
				return fmt.Errorf("MCP transport method must be POST")
			}
			return nil
		},
		stateChangeFn: func(PolicyProtocolIdentity) string { return PolicyStateChangeUnknown },
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolMCP, i.MCPMethod, i.MCPToolName)
		},
	},
	policyModule{
		moduleID: SemanticModuleAWS, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolAWS,
		validateFn: func(i PolicyProtocolIdentity) error {
			if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || kubernetesIdentityFieldsSet(i) || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
				return fmt.Errorf("AWS policy identity contains another module's fields")
			}
			if !awsServicePattern.MatchString(i.AWSService) {
				return fmt.Errorf("AWS signing service is invalid")
			}
			switch i.AWSWireProtocol {
			case AWSWireProtocolQuery:
				if !awsProtocolVersionPattern.MatchString(i.AWSProtocolVersion) || i.AWSTargetNamespace != "" || !awsQueryOperationPattern.MatchString(i.AWSOperation) {
					return fmt.Errorf("AWS Query operation is invalid")
				}
			case AWSWireProtocolJSON:
				if i.AWSProtocolVersion != "" || len(i.AWSTargetNamespace)+1+len(i.AWSOperation) > 256 || !awsTargetNamespacePattern.MatchString(i.AWSTargetNamespace) || !awsQueryOperationPattern.MatchString(i.AWSOperation) {
					return fmt.Errorf("AWS JSON operation is invalid")
				}
			default:
				return fmt.Errorf("AWS wire protocol is invalid")
			}
			return nil
		},
		validateEffectFn: func(effect SemanticRequestEffect) error {
			if effect.Method != "POST" || effect.Path != "/" {
				return fmt.Errorf("AWS RPC transport coordinates are invalid")
			}
			return nil
		},
		stateChangeFn: func(PolicyProtocolIdentity) string { return PolicyStateChangeUnknown },
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolAWS, i.AWSWireProtocol, i.AWSService, i.AWSProtocolVersion, i.AWSTargetNamespace, i.AWSOperation)
		},
	},
	policyModule{
		moduleID: SemanticModuleKubernetes, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolKubernetes,
		validateFn:       validateKubernetesModuleIdentity,
		validateEffectFn: validateKubernetesModuleEffect,
		stateChangeFn: func(i PolicyProtocolIdentity) string {
			if i.KubernetesVerb == "connect" {
				return PolicyStateChangeInteractive
			}
			if i.KubernetesDryRun == "all" || i.KubernetesVerb == "get" || i.KubernetesVerb == "list" || i.KubernetesVerb == "watch" {
				return PolicyStateChangeNone
			}
			return PolicyStateChangePossible
		},
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolKubernetes, i.KubernetesKind, i.KubernetesVerb, i.KubernetesGroup, i.KubernetesVersion, i.KubernetesResource, i.KubernetesNamespace, i.KubernetesName, i.KubernetesSubresource, i.KubernetesDryRun, i.KubernetesNonResourcePath)
		},
	},
	policyModule{
		moduleID: SemanticModuleGit, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolGit,
		validateFn:       validateGitModuleIdentity,
		validateEffectFn: validateGitModuleEffect,
		stateChangeFn: func(i PolicyProtocolIdentity) string {
			if i.GitService == "upload-pack" {
				return PolicyStateChangeNone
			}
			return PolicyStateChangePossible
		},
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolGit, i.GitService, i.GitRepository)
		},
	},
	policyModule{
		moduleID: SemanticModuleOCI, parentModuleID: SemanticModuleHTTPGeneric, protocolID: PolicyProtocolOCI,
		validateFn:       validateOCIModuleIdentity,
		validateEffectFn: validateOCIModuleEffect,
		stateChangeFn: func(i PolicyProtocolIdentity) string {
			if i.OCIAction == "list" || i.OCIAction == "pull" || i.OCIAction == "upload_status" {
				return PolicyStateChangeNone
			}
			return PolicyStateChangePossible
		},
		appendFn: func(material []string, i PolicyProtocolIdentity) []string {
			return append(material, PolicyProtocolOCI, i.OCIAction, i.OCIRepository, i.OCIObject)
		},
	},
}

func policyIdentityHasRefinement(i PolicyProtocolIdentity) bool {
	return i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" ||
		i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || i.KubernetesKind != "" || i.KubernetesVerb != "" ||
		i.KubernetesGroup != "" || i.KubernetesVersion != "" || i.KubernetesResource != "" || i.KubernetesNamespace != "" || i.KubernetesName != "" || i.KubernetesSubresource != "" || i.KubernetesDryRun != "" || i.KubernetesNonResourcePath != "" || i.GitService != "" || i.GitRepository != "" ||
		i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != ""
}

func semanticModuleByProtocol(protocol string) (semanticPolicyModule, bool) {
	for _, module := range semanticModules {
		if module.protocol() == protocol {
			return module, true
		}
	}
	return nil, false
}

func semanticModuleByID(id string) (semanticPolicyModule, bool) {
	for _, module := range semanticModules {
		if module.id() == id {
			return module, true
		}
	}
	return nil, false
}

// SemanticModuleID returns the selected module identity after validation.
func (i PolicyProtocolIdentity) SemanticModuleID() string {
	if err := i.Validate(); err != nil {
		return ""
	}
	module, ok := semanticModuleByProtocol(i.Protocol)
	if !ok {
		return ""
	}
	return module.id()
}

func validateSemanticModuleIdentity(i PolicyProtocolIdentity) error {
	if i.Scheme != "http" && i.Scheme != "https" {
		return fmt.Errorf("policy scheme is invalid")
	}
	module, ok := semanticModuleByProtocol(i.Protocol)
	if !ok {
		return fmt.Errorf("policy semantic module is invalid")
	}
	return module.validate(i)
}

func semanticModuleStateChange(i PolicyProtocolIdentity) string {
	module, ok := semanticModuleByProtocol(i.Protocol)
	if !ok {
		return PolicyStateChangeUnknown
	}
	return module.stateChange(i)
}

func semanticModuleMatches(left, right PolicyProtocolIdentity) bool {
	module, ok := semanticModuleByProtocol(left.Protocol)
	if !ok || right.Protocol != left.Protocol {
		return false
	}
	return module.matches(left, right)
}

func appendSemanticModuleIdentity(material []string, identity PolicyProtocolIdentity) []string {
	material = append(material, identity.Scheme)
	module, ok := semanticModuleByProtocol(identity.Protocol)
	if !ok {
		return append(material, "invalid")
	}
	return module.appendIdentity(material, identity)
}

// ValidateSemanticModuleRegistry proves that the compiled refinement graph is
// closed, acyclic, uniquely keyed, and has exactly one protocol mapping.
func ValidateSemanticModuleRegistry() error {
	byID := make(map[string]semanticPolicyModule, len(semanticModules))
	byProtocol := make(map[string]struct{}, len(semanticModules))
	for _, module := range semanticModules {
		if module.id() == "" || module.protocol() == "" {
			return fmt.Errorf("semantic module identity is empty")
		}
		if _, duplicate := byID[module.id()]; duplicate {
			return fmt.Errorf("semantic module ID %q is duplicated", module.id())
		}
		if _, duplicate := byProtocol[module.protocol()]; duplicate {
			return fmt.Errorf("semantic module protocol %q is duplicated", module.protocol())
		}
		byID[module.id()] = module
		byProtocol[module.protocol()] = struct{}{}
	}
	if len(byID) != len(semanticModuleIDs) {
		return fmt.Errorf("semantic module registry inventory diverges from its public IDs")
	}
	for _, id := range semanticModuleIDs {
		if _, ok := byID[id]; !ok {
			return fmt.Errorf("semantic module registry omits public ID %q", id)
		}
	}
	for _, module := range semanticModules {
		if module.id() != SemanticModuleHTTPGeneric && module.parent() == "" {
			return fmt.Errorf("semantic module %q is an extra refinement root", module.id())
		}
		seen := map[string]struct{}{module.id(): {}}
		for parent := module.parent(); parent != ""; {
			if _, cycle := seen[parent]; cycle {
				return fmt.Errorf("semantic module refinement graph contains a cycle")
			}
			seen[parent] = struct{}{}
			parentModule, ok := byID[parent]
			if !ok {
				return fmt.Errorf("semantic module parent %q is unknown", parent)
			}
			parent = parentModule.parent()
		}
	}
	if root, ok := byID[SemanticModuleHTTPGeneric]; !ok || root.parent() != "" {
		return fmt.Errorf("generic HTTP must be the semantic module root")
	}
	return nil
}

// SemanticClassificationClaim records only a positive wire claim. A parser
// that recognized its admission signal but could not produce a valid bounded
// projection must emit malformed; omitting that claim would permit an unsafe
// fallback to generic HTTP.
type SemanticClassificationClaim struct {
	ModuleID string
	State    string
}

// SelectSemanticModuleClaims rejects every malformed classified request before
// applying the refinement graph to successfully projected candidates.
func SelectSemanticModuleClaims(claims []SemanticClassificationClaim) (string, error) {
	if len(claims) == 0 {
		return "", fmt.Errorf("semantic module classification claims are empty")
	}
	candidates := make([]string, 0, len(claims))
	for _, claim := range claims {
		if _, ok := semanticModuleByID(claim.ModuleID); !ok {
			return "", fmt.Errorf("semantic module claim %q is unknown", claim.ModuleID)
		}
		switch claim.State {
		case SemanticClassificationMatched:
			candidates = append(candidates, claim.ModuleID)
		case SemanticClassificationMalformed:
			return "", fmt.Errorf("semantic module %q classified request is malformed", claim.ModuleID)
		default:
			return "", fmt.Errorf("semantic module claim state is invalid")
		}
	}
	return SelectSemanticModule(candidates)
}

func semanticModuleRefines(candidate, ancestor string) bool {
	for current := candidate; current != ""; {
		if current == ancestor {
			return true
		}
		module, ok := semanticModuleByID(current)
		if !ok {
			return false
		}
		current = module.parent()
	}
	return false
}

// SelectSemanticModule requires one unique most-specific compiled candidate.
// Candidate ordering is deliberately irrelevant.
func SelectSemanticModule(candidates []string) (string, error) {
	if err := ValidateSemanticModuleRegistry(); err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("semantic module candidates are empty")
	}
	unique := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, ok := semanticModuleByID(candidate); !ok {
			return "", fmt.Errorf("semantic module candidate %q is unknown", candidate)
		}
		unique[candidate] = struct{}{}
	}
	mostSpecific := make([]string, 0, len(unique))
	for candidate := range unique {
		refinedByAnother := false
		for other := range unique {
			if other != candidate && semanticModuleRefines(other, candidate) {
				refinedByAnother = true
				break
			}
		}
		if !refinedByAnother {
			mostSpecific = append(mostSpecific, candidate)
		}
	}
	sort.Strings(mostSpecific)
	if len(mostSpecific) != 1 {
		return "", fmt.Errorf("semantic module classification is ambiguous: %s", strings.Join(mostSpecific, ","))
	}
	return mostSpecific[0], nil
}

func kubernetesIdentityFieldsSet(i PolicyProtocolIdentity) bool {
	return i.KubernetesKind != "" || i.KubernetesVerb != "" || i.KubernetesGroup != "" || i.KubernetesVersion != "" ||
		i.KubernetesResource != "" || i.KubernetesNamespace != "" || i.KubernetesName != "" || i.KubernetesSubresource != "" ||
		i.KubernetesDryRun != "" || i.KubernetesNonResourcePath != ""
}

func validKubernetesSegment(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	if len(value) > 253 || !utf8.ValidString(value) || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func validKubernetesVerb(value string) bool {
	return value == "get" || value == "list" || value == "watch" || value == "create" || value == "update" || value == "patch" || value == "delete" || value == "deletecollection" || value == "connect"
}

func validateKubernetesModuleIdentity(i PolicyProtocolIdentity) error {
	if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
		return fmt.Errorf("Kubernetes policy identity contains another module's fields")
	}
	if !validKubernetesVerb(i.KubernetesVerb) {
		return fmt.Errorf("Kubernetes verb is invalid")
	}
	switch i.KubernetesKind {
	case KubernetesRequestResource:
		if !validKubernetesSegment(i.KubernetesGroup, true) || !validKubernetesSegment(i.KubernetesVersion, false) || !validKubernetesSegment(i.KubernetesResource, false) ||
			!validKubernetesSegment(i.KubernetesNamespace, true) || !validKubernetesSegment(i.KubernetesName, true) || !validKubernetesSegment(i.KubernetesSubresource, true) || i.KubernetesNonResourcePath != "" {
			return fmt.Errorf("Kubernetes resource projection is invalid")
		}
		if i.KubernetesSubresource != "" && i.KubernetesName == "" {
			return fmt.Errorf("Kubernetes subresource requires a named resource")
		}
		if i.KubernetesDryRun != "none" && i.KubernetesDryRun != "empty" && i.KubernetesDryRun != "all" {
			return fmt.Errorf("Kubernetes dry-run mode is invalid")
		}
		validShape := false
		switch i.KubernetesVerb {
		case "get":
			interactive := i.KubernetesSubresource == "attach" || i.KubernetesSubresource == "exec" || i.KubernetesSubresource == "portforward" || i.KubernetesSubresource == "proxy"
			validShape = i.KubernetesName != "" && !interactive && i.KubernetesDryRun == "none"
		case "list", "watch":
			validShape = i.KubernetesName == "" && i.KubernetesSubresource == "" && i.KubernetesDryRun == "none"
		case "create":
			validShape = i.KubernetesName == "" && i.KubernetesSubresource == ""
		case "update", "patch", "delete":
			validShape = i.KubernetesName != ""
		case "deletecollection":
			validShape = i.KubernetesName == "" && i.KubernetesSubresource == ""
		case "connect":
			validShape = i.KubernetesName != "" && (i.KubernetesSubresource == "attach" || i.KubernetesSubresource == "exec" || i.KubernetesSubresource == "portforward" || i.KubernetesSubresource == "proxy") && i.KubernetesDryRun == "none"
		}
		if !validShape {
			return fmt.Errorf("Kubernetes verb and resource shape are inconsistent")
		}
	case KubernetesRequestNonResource:
		if i.KubernetesVerb != "get" || i.KubernetesGroup != "" || i.KubernetesVersion != "" || i.KubernetesResource != "" || i.KubernetesNamespace != "" || i.KubernetesName != "" || i.KubernetesSubresource != "" || i.KubernetesDryRun != "" || !validKubernetesNonResourcePath(i.KubernetesNonResourcePath) {
			return fmt.Errorf("Kubernetes non-resource projection is invalid")
		}
	default:
		return fmt.Errorf("Kubernetes request kind is invalid")
	}
	return nil
}

func validKubernetesNonResourcePath(path string) bool {
	if len(path) == 0 || len(path) > 1024 || !utf8.ValidString(path) || !strings.HasPrefix(path, "/") || path == "/" || strings.HasSuffix(path, "/") || strings.Contains(path, "//") || strings.ContainsAny(path, "%\\?#") {
		return false
	}
	parts := strings.Split(path[1:], "/")
	for _, part := range parts {
		if !validKubernetesSegment(part, false) {
			return false
		}
	}
	for _, character := range path {
		if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	if parts[0] == "api" {
		return len(parts) == 1 || len(parts) == 2
	}
	if parts[0] == "apis" {
		return len(parts) == 1 || len(parts) == 3
	}
	return parts[0] == "healthz" || parts[0] == "livez" || parts[0] == "openapi" || parts[0] == "readyz" || parts[0] == "version"
}

func kubernetesResourcePath(i PolicyProtocolIdentity) string {
	var segments []string
	if i.KubernetesGroup == "" {
		segments = []string{"api", i.KubernetesVersion}
	} else {
		segments = []string{"apis", i.KubernetesGroup, i.KubernetesVersion}
	}
	if i.KubernetesNamespace != "" {
		segments = append(segments, "namespaces", i.KubernetesNamespace)
	}
	segments = append(segments, i.KubernetesResource)
	if i.KubernetesName != "" {
		segments = append(segments, i.KubernetesName)
	}
	if i.KubernetesSubresource != "" {
		segments = append(segments, i.KubernetesSubresource)
	}
	return "/" + strings.Join(segments, "/")
}

func validateKubernetesModuleEffect(effect SemanticRequestEffect) error {
	i := effect.Identity
	if i.KubernetesKind == KubernetesRequestNonResource {
		if effect.Method != "GET" || effect.Path != i.KubernetesNonResourcePath {
			return fmt.Errorf("Kubernetes non-resource transport coordinates are invalid")
		}
		return nil
	}
	expectedMethod := map[string]string{"get": "GET", "list": "GET", "watch": "GET", "create": "POST", "update": "PUT", "patch": "PATCH", "delete": "DELETE", "deletecollection": "DELETE"}[i.KubernetesVerb]
	if i.KubernetesVerb == "connect" {
		if effect.Method != "GET" && effect.Method != "POST" {
			return fmt.Errorf("Kubernetes connect transport method is invalid")
		}
	} else if effect.Method != expectedMethod {
		return fmt.Errorf("Kubernetes transport method is inconsistent with its verb")
	}
	if effect.Path != kubernetesResourcePath(i) {
		return fmt.Errorf("Kubernetes transport path is inconsistent with its resource projection")
	}
	return nil
}

func validateGitModuleIdentity(i PolicyProtocolIdentity) error {
	if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || kubernetesIdentityFieldsSet(i) || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
		return fmt.Errorf("Git policy identity contains another module's fields")
	}
	if i.GitService != "upload-pack" && i.GitService != "receive-pack" {
		return fmt.Errorf("Git service is invalid")
	}
	if len(i.GitRepository) < 2 || len(i.GitRepository) > 1024 || i.GitRepository[0] != '/' || !utf8.ValidString(i.GitRepository) || strings.Contains(i.GitRepository, "//") || strings.ContainsAny(i.GitRepository, "%\\") {
		return fmt.Errorf("Git repository path is invalid")
	}
	for _, segment := range strings.Split(i.GitRepository[1:], "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("Git repository path is invalid")
		}
	}
	for _, character := range i.GitRepository {
		if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
			return fmt.Errorf("Git repository path is invalid")
		}
	}
	return nil
}

func validateGitModuleEffect(effect SemanticRequestEffect) error {
	i := effect.Identity
	discoveryPath := i.GitRepository + "/info/refs"
	rpcPath := i.GitRepository + "/git-" + i.GitService
	if (effect.Method == "GET" && effect.Path == discoveryPath) || (effect.Method == "POST" && effect.Path == rpcPath) {
		return nil
	}
	return fmt.Errorf("Git Smart HTTP transport coordinates are invalid")
}

func validateOCIModuleIdentity(i PolicyProtocolIdentity) error {
	if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSProtocolVersion != "" || i.AWSTargetNamespace != "" || i.AWSOperation != "" || kubernetesIdentityFieldsSet(i) || i.GitService != "" || i.GitRepository != "" {
		return fmt.Errorf("OCI policy identity contains another module's fields")
	}
	if i.OCIAction != "list" && i.OCIAction != "pull" && i.OCIAction != "push" && i.OCIAction != "delete" && i.OCIAction != "start_upload" && i.OCIAction != "upload_status" && i.OCIAction != "upload_chunk" && i.OCIAction != "complete_upload" && i.OCIAction != "mount" && i.OCIAction != "cancel_upload" {
		return fmt.Errorf("OCI action is invalid")
	}
	if !validOCIRepository(i.OCIRepository, true) || !validProtocolCoordinate(i.OCIObject, 1024, false) {
		return fmt.Errorf("OCI coordinate is invalid")
	}
	validCoordinate := false
	switch i.OCIAction {
	case "list":
		validCoordinate = (i.OCIRepository == "" && i.OCIObject == "catalog") || (i.OCIRepository != "" && i.OCIObject == "tags")
	case "pull":
		validCoordinate = i.OCIRepository != "" && validOCIPathObject(i.OCIObject, "manifest:", "blob:", "referrers:")
	case "push":
		validCoordinate = i.OCIRepository != "" && validOCIPathObject(i.OCIObject, "manifest:")
	case "delete":
		validCoordinate = i.OCIRepository != "" && validOCIPathObject(i.OCIObject, "manifest:", "blob:")
	case "start_upload":
		validCoordinate = i.OCIRepository != "" && i.OCIObject == "upload"
	case "upload_status", "upload_chunk", "cancel_upload":
		validCoordinate = i.OCIRepository != "" && validOCIPathObject(i.OCIObject, "upload:")
	case "complete_upload":
		if strings.HasPrefix(i.OCIObject, "blob:") {
			_, validCoordinate = decodeCanonicalOCIQuotedToken(strings.TrimPrefix(i.OCIObject, "blob:"))
			validCoordinate = i.OCIRepository != "" && validCoordinate
		} else if strings.HasPrefix(i.OCIObject, "upload:") {
			parts := strings.SplitN(strings.TrimPrefix(i.OCIObject, "upload:"), ":blob:", 2)
			if len(parts) == 2 {
				_, sessionValid := decodeCanonicalOCIQuotedToken(parts[0])
				_, digestValid := decodeCanonicalOCIQuotedToken(parts[1])
				validCoordinate = i.OCIRepository != "" && sessionValid && digestValid
			}
		}
	case "mount":
		mountParts := strings.SplitN(strings.TrimPrefix(i.OCIObject, "mount:"), ":from:", 2)
		if i.OCIRepository != "" && strings.HasPrefix(i.OCIObject, "mount:") && len(mountParts) == 2 {
			_, digestValid := decodeCanonicalOCIQuotedToken(mountParts[0])
			source, sourceValid := decodeCanonicalOCIQuotedToken(mountParts[1])
			validCoordinate = digestValid && sourceValid && validOCIRepository(source, false)
		}
	}
	if !validCoordinate || strings.HasSuffix(i.OCIObject, ":") {
		return fmt.Errorf("OCI action coordinate is invalid")
	}
	return nil
}

func validOCIRepository(repository string, allowEmpty bool) bool {
	if repository == "" {
		return allowEmpty
	}
	if len(repository) > 1024 || !utf8.ValidString(repository) || strings.ContainsAny(repository, `%\\`) {
		return false
	}
	for _, segment := range strings.Split(repository, "/") {
		if !validOCIPathSegment(segment) {
			return false
		}
	}
	return true
}

func validOCIPathSegment(segment string) bool {
	if segment == "" || len(segment) > 512 || !utf8.ValidString(segment) || segment == "." || segment == ".." || strings.ContainsAny(segment, `%\\/`) {
		return false
	}
	for _, character := range segment {
		if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func validOCIPathObject(object string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(object, prefix) {
			return validOCIPathSegment(strings.TrimPrefix(object, prefix))
		}
	}
	return false
}

func decodeCanonicalOCIQuotedToken(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	decodedBytes := make([]byte, 0, len(token))
	for index := 0; index < len(token); index++ {
		if token[index] != '%' {
			decodedBytes = append(decodedBytes, token[index])
			continue
		}
		if index+2 >= len(token) {
			return "", false
		}
		high, highOK := hexadecimalNibble(token[index+1])
		low, lowOK := hexadecimalNibble(token[index+2])
		if !highOK || !lowOK {
			return "", false
		}
		decodedBytes = append(decodedBytes, high<<4|low)
		index += 2
	}
	decoded := string(decodedBytes)
	if len(decoded) > 512 || !utf8.ValidString(decoded) || canonicalOCIQuotedToken(decoded) != token {
		return "", false
	}
	return decoded, true
}

func hexadecimalNibble(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	default:
		return 0, false
	}
}

func canonicalOCIQuotedToken(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var encoded strings.Builder
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("-._~", rune(character)) {
			encoded.WriteByte(character)
			continue
		}
		encoded.WriteByte('%')
		encoded.WriteByte(hexadecimal[character>>4])
		encoded.WriteByte(hexadecimal[character&0x0f])
	}
	return encoded.String()
}

func validateOCIModuleEffect(effect SemanticRequestEffect) error {
	i := effect.Identity
	base := "/v2/" + i.OCIRepository
	objectValue := func(prefix string) string { return strings.TrimPrefix(i.OCIObject, prefix) }
	valid := false
	switch i.OCIAction {
	case "list":
		valid = effect.Method == "GET" && ((i.OCIRepository == "" && effect.Path == "/v2/_catalog") || (i.OCIRepository != "" && effect.Path == base+"/tags/list"))
	case "pull":
		switch {
		case strings.HasPrefix(i.OCIObject, "manifest:"):
			valid = (effect.Method == "GET" || effect.Method == "HEAD") && effect.Path == base+"/manifests/"+objectValue("manifest:")
		case strings.HasPrefix(i.OCIObject, "blob:"):
			valid = (effect.Method == "GET" || effect.Method == "HEAD") && effect.Path == base+"/blobs/"+objectValue("blob:")
		case strings.HasPrefix(i.OCIObject, "referrers:"):
			valid = effect.Method == "GET" && effect.Path == base+"/referrers/"+objectValue("referrers:")
		}
	case "push":
		valid = effect.Method == "PUT" && effect.Path == base+"/manifests/"+objectValue("manifest:")
	case "delete":
		if strings.HasPrefix(i.OCIObject, "manifest:") {
			valid = effect.Method == "DELETE" && effect.Path == base+"/manifests/"+objectValue("manifest:")
		} else if strings.HasPrefix(i.OCIObject, "blob:") {
			valid = effect.Method == "DELETE" && effect.Path == base+"/blobs/"+objectValue("blob:")
		}
	case "start_upload", "mount":
		valid = effect.Method == "POST" && (effect.Path == base+"/blobs/uploads" || effect.Path == base+"/blobs/uploads/")
	case "upload_status", "upload_chunk", "cancel_upload":
		expectedMethod := map[string]string{"upload_status": "GET", "upload_chunk": "PATCH", "cancel_upload": "DELETE"}[i.OCIAction]
		valid = effect.Method == expectedMethod && effect.Path == base+"/blobs/uploads/"+objectValue("upload:")
	case "complete_upload":
		if strings.HasPrefix(i.OCIObject, "blob:") {
			valid = effect.Method == "POST" && (effect.Path == base+"/blobs/uploads" || effect.Path == base+"/blobs/uploads/")
		} else if strings.HasPrefix(i.OCIObject, "upload:") {
			parts := strings.SplitN(strings.TrimPrefix(i.OCIObject, "upload:"), ":blob:", 2)
			if len(parts) == 2 {
				session, sessionValid := decodeCanonicalOCIQuotedToken(parts[0])
				valid = sessionValid && effect.Method == "PUT" && effect.Path == base+"/blobs/uploads/"+session
			}
		}
	}
	if !valid {
		return fmt.Errorf("OCI Distribution transport coordinates are invalid")
	}
	return nil
}
