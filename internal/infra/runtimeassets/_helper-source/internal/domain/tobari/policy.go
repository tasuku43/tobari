package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	requestIDPattern         = regexp.MustCompile(`^[0-9a-f]{32}$`)
	policyCandidateIDPattern = regexp.MustCompile(`^pcy_[0-9a-f]{32}$`)
	policyTemplateIDPattern  = regexp.MustCompile(`^ptp_[0-9a-f]{32}$`)
	policyDenyRuleIDPattern  = regexp.MustCompile(`^pdr_[0-9a-f]{32}$`)
	learnedRuleIDPattern     = regexp.MustCompile(`^plr_[0-9a-f]{32}$`)
	httpMethodPattern        = regexp.MustCompile(`^[A-Z][A-Z0-9!#$%&'*+.^_` + "`" + `|~-]{0,31}$`)
	graphqlNamePattern       = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)
	mcpMethodPattern         = regexp.MustCompile(`^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*$`)
	mcpToolNamePattern       = regexp.MustCompile(`^[A-Za-z0-9_.:/-]+$`)
	awsServicePattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	awsQueryOperationPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	awsJSONOperationPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*\.[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	policyRevisionPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	PolicyMatchExact             = "exact"
	PolicyMatchPathTemplate      = "path_template"
	PolicyDecisionAllow          = "allow"
	PolicyDecisionDeny           = "deny"
	PolicyProtocolHTTP           = "http"
	PolicyProtocolGraphQL        = "graphql"
	PolicyProtocolMCP            = "mcp"
	PolicyProtocolAWS            = "aws"
	PolicyProtocolKubernetes     = "kubernetes"
	PolicyProtocolGit            = "git"
	PolicyProtocolOCI            = "oci"
	AWSWireProtocolQuery         = "query"
	AWSWireProtocolJSON          = "json"
	GraphQLOperationQuery        = "query"
	GraphQLOperationMutation     = "mutation"
	PolicyStateChangeUnknown     = "unknown"
	PolicyStateChangeNone        = "not_expected"
	PolicyStateChangePossible    = "possible"
	PolicyStateChangeInteractive = "interactive"
)

var (
	policyMatchValues = [...]string{
		PolicyMatchExact,
		PolicyMatchPathTemplate,
	}
	policyProtocolValues = [...]string{
		PolicyProtocolHTTP,
		PolicyProtocolGraphQL,
		PolicyProtocolMCP,
		PolicyProtocolAWS,
		PolicyProtocolKubernetes,
		PolicyProtocolGit,
		PolicyProtocolOCI,
	}
	policyDecisionValues = [...]string{
		PolicyDecisionAllow,
		PolicyDecisionDeny,
	}
	policyStateChangeValues = [...]string{
		PolicyStateChangeNone,
		PolicyStateChangePossible,
		PolicyStateChangeInteractive,
		PolicyStateChangeUnknown,
	}
)

// PolicyMatchValues returns the canonical presentation order for the closed
// policy-rule match vocabulary. Callers receive an independent copy.
func PolicyMatchValues() []string {
	return append([]string{}, policyMatchValues[:]...)
}

// PolicyProtocolValues returns the canonical presentation order for the
// closed policy protocol vocabulary. Callers receive an independent copy.
func PolicyProtocolValues() []string {
	return append([]string{}, policyProtocolValues[:]...)
}

// PolicyDecisionValues returns the canonical presentation order for the
// closed learned-policy decision vocabulary. Callers receive an independent
// copy.
func PolicyDecisionValues() []string {
	return append([]string{}, policyDecisionValues[:]...)
}

// PolicyStateChangeValues returns the canonical presentation order for the
// closed policy state-change evidence vocabulary. Callers receive an
// independent copy.
func PolicyStateChangeValues() []string {
	return append([]string{}, policyStateChangeValues[:]...)
}

func policyVocabularyContains(values []string, value string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validPolicyMatch(value string) bool {
	return policyVocabularyContains(policyMatchValues[:], value)
}

func validPolicyProtocol(value string) bool {
	return policyVocabularyContains(policyProtocolValues[:], value)
}

func validPolicyDecision(value string) bool {
	return policyVocabularyContains(policyDecisionValues[:], value)
}

// PolicyProtocolIdentity identifies one HTTP effect or refines it to exactly
// one bounded protocol coordinate. AWS identity carries no read/write
// semantics; the other refinements derive only their documented signal.
type PolicyProtocolIdentity struct {
	Scheme               string `json:"scheme"`
	Protocol             string `json:"protocol"`
	GraphQLOperationType string `json:"graphql_operation_type,omitempty"`
	GraphQLRootField     string `json:"graphql_root_field,omitempty"`
	MCPMethod            string `json:"mcp_method,omitempty"`
	MCPToolName          string `json:"mcp_tool_name,omitempty"`
	AWSWireProtocol      string `json:"aws_wire_protocol,omitempty"`
	AWSService           string `json:"aws_service,omitempty"`
	AWSOperation         string `json:"aws_operation,omitempty"`
	KubernetesVerb       string `json:"kubernetes_verb,omitempty"`
	KubernetesResource   string `json:"kubernetes_resource,omitempty"`
	KubernetesDryRun     string `json:"kubernetes_dry_run,omitempty"`
	GitService           string `json:"git_service,omitempty"`
	GitRepository        string `json:"git_repository,omitempty"`
	OCIAction            string `json:"oci_action,omitempty"`
	OCIRepository        string `json:"oci_repository,omitempty"`
	OCIObject            string `json:"oci_object,omitempty"`
}

// EffectiveProtocol returns the validated closed protocol value.
func (i PolicyProtocolIdentity) EffectiveProtocol() string {
	return i.Protocol
}

// StateChangePotential is conservative review evidence derived from validated
// wire identity. It is never an independent permission or matching dimension.
func (i PolicyProtocolIdentity) StateChangePotential() string {
	if i.EffectiveProtocol() == PolicyProtocolGraphQL {
		if i.GraphQLOperationType == GraphQLOperationQuery {
			return PolicyStateChangeNone
		}
		if i.GraphQLOperationType == GraphQLOperationMutation {
			return PolicyStateChangePossible
		}
	}
	if i.EffectiveProtocol() == PolicyProtocolKubernetes {
		if i.KubernetesVerb == "connect" {
			return PolicyStateChangeInteractive
		}
		if i.KubernetesDryRun == "all" || i.KubernetesVerb == "get" || i.KubernetesVerb == "list" || i.KubernetesVerb == "watch" {
			return PolicyStateChangeNone
		}
		return PolicyStateChangePossible
	}
	if i.EffectiveProtocol() == PolicyProtocolGit {
		if i.GitService == "upload-pack" {
			return PolicyStateChangeNone
		}
		return PolicyStateChangePossible
	}
	if i.EffectiveProtocol() == PolicyProtocolOCI {
		if i.OCIAction == "list" || i.OCIAction == "pull" || i.OCIAction == "upload_status" {
			return PolicyStateChangeNone
		}
		return PolicyStateChangePossible
	}
	return PolicyStateChangeUnknown
}

func (i PolicyProtocolIdentity) Validate() error {
	if i.Scheme != "http" && i.Scheme != "https" {
		return fmt.Errorf("policy scheme is invalid")
	}
	if !validPolicyProtocol(i.EffectiveProtocol()) {
		return fmt.Errorf("policy protocol is invalid")
	}
	switch i.EffectiveProtocol() {
	case PolicyProtocolHTTP:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("HTTP policy identity cannot contain protocol refinement fields")
		}
	case PolicyProtocolGraphQL:
		if i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("GraphQL policy identity cannot contain MCP fields")
		}
		if i.GraphQLOperationType != GraphQLOperationQuery && i.GraphQLOperationType != GraphQLOperationMutation {
			return fmt.Errorf("GraphQL operation type is invalid")
		}
		if len(i.GraphQLRootField) == 0 || len(i.GraphQLRootField) > 256 || !graphqlNamePattern.MatchString(i.GraphQLRootField) {
			return fmt.Errorf("GraphQL root field is invalid")
		}
	case PolicyProtocolMCP:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("MCP policy identity cannot contain GraphQL fields")
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
	case PolicyProtocolAWS:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("AWS policy identity cannot contain another protocol's fields")
		}
		if !awsServicePattern.MatchString(i.AWSService) {
			return fmt.Errorf("AWS signing service is invalid")
		}
		switch i.AWSWireProtocol {
		case AWSWireProtocolQuery:
			if !awsQueryOperationPattern.MatchString(i.AWSOperation) {
				return fmt.Errorf("AWS Query operation is invalid")
			}
		case AWSWireProtocolJSON:
			if len(i.AWSOperation) > 256 || !awsJSONOperationPattern.MatchString(i.AWSOperation) {
				return fmt.Errorf("AWS JSON operation is invalid")
			}
		default:
			return fmt.Errorf("AWS wire protocol is invalid")
		}
	case PolicyProtocolKubernetes:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.GitService != "" || i.GitRepository != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("Kubernetes policy identity cannot contain another protocol's fields")
		}
		if i.KubernetesVerb != "get" && i.KubernetesVerb != "list" && i.KubernetesVerb != "watch" && i.KubernetesVerb != "create" && i.KubernetesVerb != "update" && i.KubernetesVerb != "patch" && i.KubernetesVerb != "delete" && i.KubernetesVerb != "deletecollection" && i.KubernetesVerb != "connect" {
			return fmt.Errorf("Kubernetes verb is invalid")
		}
		if len(i.KubernetesResource) == 0 || len(i.KubernetesResource) > 1024 || !utf8.ValidString(i.KubernetesResource) || strings.IndexByte(i.KubernetesResource, 0) >= 0 {
			return fmt.Errorf("Kubernetes resource coordinate is invalid")
		}
		for _, character := range i.KubernetesResource {
			if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
				return fmt.Errorf("Kubernetes resource coordinate is invalid")
			}
		}
		if i.KubernetesDryRun != "none" && i.KubernetesDryRun != "empty" && i.KubernetesDryRun != "all" {
			return fmt.Errorf("Kubernetes dry-run mode is invalid")
		}
	case PolicyProtocolGit:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.OCIAction != "" || i.OCIRepository != "" || i.OCIObject != "" {
			return fmt.Errorf("Git policy identity cannot contain another protocol's fields")
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
	case PolicyProtocolOCI:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" || i.MCPMethod != "" || i.MCPToolName != "" || i.AWSWireProtocol != "" || i.AWSService != "" || i.AWSOperation != "" || i.KubernetesVerb != "" || i.KubernetesResource != "" || i.KubernetesDryRun != "" || i.GitService != "" || i.GitRepository != "" {
			return fmt.Errorf("OCI policy identity cannot contain another protocol's fields")
		}
		if i.OCIAction != "list" && i.OCIAction != "pull" && i.OCIAction != "push" && i.OCIAction != "delete" && i.OCIAction != "start_upload" && i.OCIAction != "upload_status" && i.OCIAction != "upload_chunk" && i.OCIAction != "complete_upload" && i.OCIAction != "mount" && i.OCIAction != "cancel_upload" {
			return fmt.Errorf("OCI action is invalid")
		}
		if !validProtocolCoordinate(i.OCIRepository, 1024, true) || !validProtocolCoordinate(i.OCIObject, 1024, false) {
			return fmt.Errorf("OCI coordinate is invalid")
		}
		validCoordinate := false
		switch i.OCIAction {
		case "list":
			validCoordinate = (i.OCIRepository == "" && i.OCIObject == "catalog") || (i.OCIRepository != "" && i.OCIObject == "tags")
		case "pull":
			validCoordinate = i.OCIRepository != "" && (strings.HasPrefix(i.OCIObject, "manifest:") || strings.HasPrefix(i.OCIObject, "blob:") || strings.HasPrefix(i.OCIObject, "referrers:"))
		case "push":
			validCoordinate = i.OCIRepository != "" && strings.HasPrefix(i.OCIObject, "manifest:")
		case "delete":
			validCoordinate = i.OCIRepository != "" && (strings.HasPrefix(i.OCIObject, "manifest:") || strings.HasPrefix(i.OCIObject, "blob:"))
		case "start_upload":
			validCoordinate = i.OCIRepository != "" && i.OCIObject == "upload"
		case "upload_status", "upload_chunk", "cancel_upload":
			validCoordinate = i.OCIRepository != "" && strings.HasPrefix(i.OCIObject, "upload:")
		case "complete_upload":
			validCoordinate = i.OCIRepository != "" && strings.HasPrefix(i.OCIObject, "blob:")
		case "mount":
			mountParts := strings.SplitN(strings.TrimPrefix(i.OCIObject, "mount:"), ":from:", 2)
			validCoordinate = i.OCIRepository != "" && strings.HasPrefix(i.OCIObject, "mount:") && len(mountParts) == 2 && mountParts[0] != "" && mountParts[1] != ""
		}
		if !validCoordinate || strings.HasSuffix(i.OCIObject, ":") {
			return fmt.Errorf("OCI action coordinate is invalid")
		}
	default:
		return fmt.Errorf("policy protocol semantics are not implemented")
	}
	return nil
}

func (i PolicyProtocolIdentity) matches(other PolicyProtocolIdentity) bool {
	return i.EffectiveProtocol() == other.EffectiveProtocol() &&
		i.Scheme == other.Scheme &&
		i.GraphQLOperationType == other.GraphQLOperationType &&
		i.GraphQLRootField == other.GraphQLRootField &&
		i.MCPMethod == other.MCPMethod &&
		i.MCPToolName == other.MCPToolName &&
		i.AWSWireProtocol == other.AWSWireProtocol &&
		i.AWSService == other.AWSService &&
		i.AWSOperation == other.AWSOperation &&
		i.KubernetesVerb == other.KubernetesVerb &&
		i.KubernetesResource == other.KubernetesResource &&
		i.KubernetesDryRun == other.KubernetesDryRun &&
		i.GitService == other.GitService &&
		i.GitRepository == other.GitRepository &&
		i.OCIAction == other.OCIAction &&
		i.OCIRepository == other.OCIRepository &&
		i.OCIObject == other.OCIObject
}

func appendPolicyProtocolIdentity(material []string, identity PolicyProtocolIdentity) []string {
	material = append(material, identity.Scheme)
	if identity.EffectiveProtocol() == PolicyProtocolHTTP {
		return material
	}
	if identity.EffectiveProtocol() == PolicyProtocolGraphQL {
		return append(material, PolicyProtocolGraphQL, identity.GraphQLOperationType, identity.GraphQLRootField)
	}
	if identity.EffectiveProtocol() == PolicyProtocolMCP {
		return append(material, PolicyProtocolMCP, identity.MCPMethod, identity.MCPToolName)
	}
	if identity.EffectiveProtocol() == PolicyProtocolKubernetes {
		return append(material, PolicyProtocolKubernetes, identity.KubernetesVerb, identity.KubernetesResource, identity.KubernetesDryRun)
	}
	if identity.EffectiveProtocol() == PolicyProtocolGit {
		return append(material, PolicyProtocolGit, identity.GitService, identity.GitRepository)
	}
	if identity.EffectiveProtocol() == PolicyProtocolOCI {
		return append(material, PolicyProtocolOCI, identity.OCIAction, identity.OCIRepository, identity.OCIObject)
	}
	return append(material, PolicyProtocolAWS, identity.AWSWireProtocol, identity.AWSService, identity.AWSOperation)
}

func validProtocolCoordinate(value string, maxBytes int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || len(value) > maxBytes || !utf8.ValidString(value) || strings.IndexByte(value, 0) >= 0 {
		return false
	}
	for _, character := range value {
		if character < 32 || character == 127 || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

// GraphQLEndpoint is one trusted exact transport location where Gateway may
// classify a request body as GraphQL. It carries no provider or CLI semantics.
type GraphQLEndpoint struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Path   string `json:"path"`
}

func (e GraphQLEndpoint) Validate() error {
	if e.Scheme != "http" && e.Scheme != "https" {
		return fmt.Errorf("GraphQL endpoint scheme is invalid")
	}
	if !validNormalizedPolicyHost(e.Host) {
		return fmt.Errorf("GraphQL endpoint host is invalid")
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("GraphQL endpoint port is invalid")
	}
	if err := validateGraphQLEndpointPath(e.Path); err != nil {
		return fmt.Errorf("GraphQL endpoint path is invalid")
	}
	return nil
}

func validNormalizedPolicyHost(host string) bool {
	if len(host) == 0 || len(host) > 253 || host != strings.ToLower(host) || strings.HasSuffix(host, ".") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return false
			}
		}
	}
	return true
}

func validateGraphQLEndpointPath(path string) error {
	if err := validatePolicyPath(path); err != nil {
		return err
	}
	if path == "/" {
		return nil
	}
	if strings.ContainsAny(path, `%\\?#`) || strings.Contains(path, "//") {
		return fmt.Errorf("GraphQL endpoint path is not normalized")
	}
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("GraphQL endpoint path is not normalized")
		}
	}
	return nil
}

func (e GraphQLEndpoint) Matches(scheme, host string, port int, path string) bool {
	return e.Scheme == scheme && e.Host == host && e.Port == port && e.Path == path
}

// MCPEndpoint is one trusted exact transport location where Gateway may
// classify a bounded JSON-RPC request body as MCP.
type MCPEndpoint struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Path   string `json:"path"`
}

func (e MCPEndpoint) Validate() error {
	if e.Scheme != "http" && e.Scheme != "https" {
		return fmt.Errorf("MCP endpoint scheme is invalid")
	}
	if !validNormalizedPolicyHost(e.Host) {
		return fmt.Errorf("MCP endpoint host is invalid")
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("MCP endpoint port is invalid")
	}
	if err := validateGraphQLEndpointPath(e.Path); err != nil {
		return fmt.Errorf("MCP endpoint path is invalid")
	}
	return nil
}

// PolicyDenial is one validated, secret-free Gateway audit decision.
type PolicyDenial struct {
	PolicyProtocolIdentity
	Timestamp             string `json:"timestamp"`
	RequestID             string `json:"request_id"`
	WorkspaceManifestID   string `json:"workspace_manifest_id"`
	WorkspaceManifestName string `json:"workspace_manifest"`
	ProjectID             string `json:"workspace_id"`
	ProjectRoot           string `json:"project_root"`
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Method                string `json:"method"`
	Path                  string `json:"path"`
	Reason                string `json:"reason"`
	StatusCode            int    `json:"status_code"`
	Learnable             bool   `json:"learnable"`
	DestinationKind       string `json:"destination_kind,omitempty"`
	AuthorityLifetime     string `json:"authority_lifetime,omitempty"`
	AttachmentEpochID     string `json:"attachment_epoch_id,omitempty"`
}

// DenialRead is one bounded Gateway-log projection. UnparsedLines counts
// denial-shaped records that could not safely enter the typed collection.
type DenialRead struct {
	Items         []PolicyDenial
	UnparsedLines int
}

func (r DenialRead) Validate() error {
	if r.Items == nil {
		return fmt.Errorf("denial read collection is unknown")
	}
	if r.UnparsedLines < 0 {
		return fmt.Errorf("denial read unparsed line count is invalid")
	}
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (d PolicyDenial) EffectiveDestinationKind() string {
	if d.DestinationKind == "" {
		return PolicyDestinationExternal
	}
	return d.DestinationKind
}

func (d PolicyDenial) EffectiveAuthorityLifetime() string {
	if d.AuthorityLifetime == "" {
		return AuthorityLifetimePersistent
	}
	return d.AuthorityLifetime
}

// Validate rejects audit-shaped data that cannot be safely interpreted as one
// denied HTTP boundary effect.
func (d PolicyDenial) Validate() error {
	if err := d.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("denial protocol identity: %w", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, d.Timestamp); err != nil {
		return fmt.Errorf("denial timestamp is invalid")
	}
	if !requestIDPattern.MatchString(d.RequestID) {
		return fmt.Errorf("denial request ID is invalid")
	}
	if err := validatePolicyScope(d.WorkspaceManifestID, d.WorkspaceManifestName, d.ProjectRoot); err != nil {
		return fmt.Errorf("denial scope: %w", err)
	}
	if err := ValidateWorkspaceID(d.ProjectID); err != nil {
		return fmt.Errorf("denial project ID is invalid")
	}
	if len(d.Host) == 0 || len(d.Host) > 253 || containsSpaceOrControl(d.Host) {
		return fmt.Errorf("denial host is invalid")
	}
	if d.Port < 1 || d.Port > 65535 {
		return fmt.Errorf("denial port is invalid")
	}
	if !httpMethodPattern.MatchString(d.Method) {
		return fmt.Errorf("denial method is invalid")
	}
	if err := validatePolicyPath(d.Path); err != nil {
		return fmt.Errorf("denial path is invalid")
	}
	if len(d.Reason) == 0 || len(d.Reason) > 1024 {
		return fmt.Errorf("denial reason is invalid")
	}
	if d.StatusCode < 400 || d.StatusCode > 599 {
		return fmt.Errorf("denial status code is invalid")
	}
	if d.EffectiveDestinationKind() == PolicyDestinationHostLoopback {
		if d.EffectiveAuthorityLifetime() != AuthorityLifetimeAttachment || ValidateAttachmentEpochID(d.AttachmentEpochID) != nil ||
			d.Host != HostLoopbackHostname || d.Scheme != "http" || ValidateHostLoopbackPort(d.Port) != nil {
			return fmt.Errorf("denial Host Loopback authority is invalid")
		}
	} else if d.EffectiveDestinationKind() != PolicyDestinationExternal || d.EffectiveAuthorityLifetime() != AuthorityLifetimePersistent || d.AttachmentEpochID != "" {
		return fmt.Errorf("denial destination authority is invalid")
	}
	return nil
}

func validatePolicyScope(contextID, contextName, projectRoot string) error {
	if err := ValidateWorkspaceManifestID(contextID); err != nil {
		return fmt.Errorf("Workspace Manifest ID is invalid")
	}
	if err := ValidateName(contextName); err != nil {
		return fmt.Errorf("Workspace Manifest name is invalid")
	}
	if !filepath.IsAbs(projectRoot) || filepath.Clean(projectRoot) != projectRoot {
		return fmt.Errorf("project root is invalid")
	}
	return nil
}

func validatePolicyPath(value string) error {
	if len(value) == 0 || len(value) > 4096 || !strings.HasPrefix(value, "/") {
		return fmt.Errorf("path must be an absolute HTTP path")
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0 {
		return fmt.Errorf("path contains a control character")
	}
	return nil
}

func containsSpaceOrControl(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return r <= ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0
}

// PolicyProjectionIdentity is the public, path-free identity of the active
// aggregate policy. The evaluator and canonical typed policy-data identities
// are deliberately separate so either kind of drift remains auditable.
type PolicyProjectionIdentity struct {
	AggregateRevision  string                  `json:"aggregate_revision"`
	EvaluatorIdentity  PolicyEvaluatorIdentity `json:"evaluator_identity"`
	PolicyDataIdentity PolicyDataIdentity      `json:"policy_data_identity"`
}

func (i PolicyProjectionIdentity) Validate() error {
	if !policyRevisionPattern.MatchString(i.AggregateRevision) {
		return fmt.Errorf("aggregate policy revision is invalid")
	}
	if err := i.EvaluatorIdentity.Validate(); err != nil {
		return fmt.Errorf("aggregate evaluator identity: %w", err)
	}
	if err := i.PolicyDataIdentity.Validate(); err != nil {
		return fmt.Errorf("aggregate policy-data identity: %w", err)
	}
	return nil
}

func (s State) PolicyProjectionIdentity() PolicyProjectionIdentity {
	return PolicyProjectionIdentity{
		AggregateRevision:  s.AggregateRevision,
		EvaluatorIdentity:  s.EvaluatorIdentity,
		PolicyDataIdentity: s.PolicyDataIdentity,
	}
}

// DenialReport preserves the exact local cluster scope and requested bounded
// Gateway-log window, including a valid empty result.
type DenialReport struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	WindowLines   int            `json:"window_lines"`
	UnparsedLines int            `json:"unparsed_lines"`
	Items         []PolicyDenial `json:"items"`
}

// Validate binds denial evidence to the cluster-denials task and its scope.
func (r DenialReport) Validate() error {
	if r.Task != TaskClusterDenials {
		return fmt.Errorf("denial report task identity is invalid")
	}
	if err := r.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("denial report projection identity: %w", err)
	}
	if r.WindowLines < 1 || r.WindowLines > 10_000 {
		return fmt.Errorf("denial report window is invalid")
	}
	if r.UnparsedLines < 0 || r.UnparsedLines > r.WindowLines {
		return fmt.Errorf("denial report unparsed line count is invalid")
	}
	if r.Items == nil {
		return fmt.Errorf("denial report collection is unknown")
	}
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// PolicyCandidate is one exact permission proposal derived from one retained
// validated denial. ID is opaque and remains stable for the same exact effect.
type PolicyCandidate struct {
	PolicyProtocolIdentity
	ID                    string `json:"id"`
	ObservedAt            string `json:"observed_at"`
	ObservationCount      int    `json:"observation_count"`
	WorkspaceManifestID   string `json:"workspace_manifest_id"`
	WorkspaceManifestName string `json:"workspace_manifest"`
	ProjectID             string `json:"workspace_id"`
	ProjectRoot           string `json:"project_root"`
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Method                string `json:"method"`
	Path                  string `json:"path"`
	Reason                string `json:"reason"`
	StatusCode            int    `json:"status_code"`
	DestinationKind       string `json:"destination_kind,omitempty"`
	AuthorityLifetime     string `json:"authority_lifetime,omitempty"`
	AttachmentEpochID     string `json:"attachment_epoch_id,omitempty"`
}

func (c PolicyCandidate) EffectiveDestinationKind() string {
	if c.DestinationKind == "" {
		return PolicyDestinationExternal
	}
	return c.DestinationKind
}

func (c PolicyCandidate) EffectiveAuthorityLifetime() string {
	if c.AuthorityLifetime == "" {
		return AuthorityLifetimePersistent
	}
	return c.AuthorityLifetime
}

// NewPolicyCandidate derives a kind-specific opaque reference without exposing
// a reversible encoding of the exact host, method, and path.
func NewPolicyCandidate(denial PolicyDenial) (PolicyCandidate, error) {
	if err := denial.Validate(); err != nil {
		return PolicyCandidate{}, err
	}
	if !denial.Learnable {
		return PolicyCandidate{}, fmt.Errorf("denial cannot be resolved by an exact learned rule")
	}
	material := appendPolicyProtocolIdentity(
		[]string{
			"tobari-policy-candidate-v1", denial.WorkspaceManifestID, denial.ProjectID, denial.Host, strconv.Itoa(denial.Port), denial.Method, denial.Path,
		},
		denial.PolicyProtocolIdentity,
	)
	if denial.EffectiveDestinationKind() == PolicyDestinationHostLoopback {
		material = append(material, denial.EffectiveDestinationKind(), denial.EffectiveAuthorityLifetime(), denial.AttachmentEpochID)
	}
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return PolicyCandidate{
		PolicyProtocolIdentity: denial.PolicyProtocolIdentity,
		ID:                     "pcy_" + hex.EncodeToString(sum[:16]), ObservedAt: denial.Timestamp, ObservationCount: 1,
		WorkspaceManifestID: denial.WorkspaceManifestID, WorkspaceManifestName: denial.WorkspaceManifestName,
		ProjectID: denial.ProjectID, ProjectRoot: denial.ProjectRoot,
		Host: denial.Host, Port: denial.Port, Method: denial.Method, Path: denial.Path,
		Reason: denial.Reason, StatusCode: denial.StatusCode,
		DestinationKind: denial.DestinationKind, AuthorityLifetime: denial.AuthorityLifetime,
		AttachmentEpochID: denial.AttachmentEpochID,
	}, nil
}

// EffectiveObservationCount returns the required retained observation count.
func (c PolicyCandidate) EffectiveObservationCount() int {
	return c.ObservationCount
}

func ValidatePolicyCandidateID(id string) error {
	if !policyCandidateIDPattern.MatchString(id) {
		return fmt.Errorf("policy candidate ID is invalid")
	}
	return nil
}

// ValidatePolicyRuleID accepts either kind of current CLI-owned learned
// decision. Baseline host policy has no policy-rule reference and cannot be
// reset through this path.
func ValidatePolicyRuleID(id string) error {
	if !learnedRuleIDPattern.MatchString(id) && !policyDenyRuleIDPattern.MatchString(id) {
		return fmt.Errorf("policy rule ID is invalid")
	}
	return nil
}

func (c PolicyCandidate) Validate() error {
	if err := c.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("policy candidate protocol identity: %w", err)
	}
	if err := ValidatePolicyCandidateID(c.ID); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, c.ObservedAt); err != nil {
		return fmt.Errorf("policy candidate timestamp is invalid")
	}
	if c.ObservationCount < 1 {
		return fmt.Errorf("policy candidate observation count is invalid")
	}
	if err := validatePolicyScope(c.WorkspaceManifestID, c.WorkspaceManifestName, c.ProjectRoot); err != nil {
		return fmt.Errorf("policy candidate scope: %w", err)
	}
	if err := ValidateWorkspaceID(c.ProjectID); err != nil {
		return fmt.Errorf("policy candidate project ID is invalid")
	}
	if len(c.Host) == 0 || len(c.Host) > 253 || containsSpaceOrControl(c.Host) {
		return fmt.Errorf("policy candidate host is invalid")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("policy candidate port is invalid")
	}
	if !httpMethodPattern.MatchString(c.Method) {
		return fmt.Errorf("policy candidate method is invalid")
	}
	if err := validatePolicyPath(c.Path); err != nil {
		return fmt.Errorf("policy candidate path is invalid")
	}
	if len(c.Reason) == 0 || len(c.Reason) > 1024 {
		return fmt.Errorf("policy candidate reason is invalid")
	}
	if c.StatusCode < 400 || c.StatusCode > 599 {
		return fmt.Errorf("policy candidate status is invalid")
	}
	denial := PolicyDenial{PolicyProtocolIdentity: c.PolicyProtocolIdentity, Timestamp: c.ObservedAt, RequestID: strings.Repeat("0", 32), WorkspaceManifestID: c.WorkspaceManifestID, WorkspaceManifestName: c.WorkspaceManifestName, ProjectID: c.ProjectID, ProjectRoot: c.ProjectRoot, Host: c.Host, Port: c.Port, Method: c.Method, Path: c.Path, Reason: c.Reason, StatusCode: c.StatusCode, Learnable: true, DestinationKind: c.DestinationKind, AuthorityLifetime: c.AuthorityLifetime, AttachmentEpochID: c.AttachmentEpochID}
	if err := denial.Validate(); err != nil {
		return fmt.Errorf("policy candidate authority: %w", err)
	}
	return nil
}

// PolicyCandidateReport preserves the bounded retained-log scope and supports
// both machine discovery and the human tail projection.
type PolicyCandidateReport struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	WindowLines   int                `json:"window_lines"`
	UnparsedLines int                `json:"unparsed_lines"`
	Items         []PolicyCandidate  `json:"items"`
	ReviewItems   []PolicyReviewItem `json:"-"`
}

func (r PolicyCandidateReport) Validate() error {
	if r.Task != TaskPolicyCandidates && r.Task != TaskPolicyReview {
		return fmt.Errorf("policy candidate report task identity is invalid")
	}
	if err := r.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("policy candidate projection identity: %w", err)
	}
	if r.WindowLines < 1 || r.WindowLines > 10_000 {
		return fmt.Errorf("policy candidate window is invalid")
	}
	if r.UnparsedLines < 0 || r.UnparsedLines > r.WindowLines {
		return fmt.Errorf("policy candidate unparsed line count is invalid")
	}
	if r.Items == nil {
		return fmt.Errorf("policy candidate collection is unknown")
	}
	seen := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if seen[item.ID] {
			return fmt.Errorf("policy candidate IDs must be unique")
		}
		seen[item.ID] = true
	}
	if r.ReviewItems != nil {
		seenReviewItems := make(map[string]bool, len(r.ReviewItems))
		for _, item := range r.ReviewItems {
			if err := item.Validate(); err != nil {
				return err
			}
			if seenReviewItems[item.ID] {
				return fmt.Errorf("policy review item IDs must be unique")
			}
			seenReviewItems[item.ID] = true
		}
	}
	return nil
}

// PolicyDenyRule is one exact project-bound deny created by rejecting a
// candidate. It is CLI-owned policy data and intentionally carries the same
// dimensions as the candidate it resolves.
type PolicyDenyRule struct {
	PolicyProtocolIdentity
	ID string `json:"id"`
	// Persisted learned-policy data is also the OPA schema-v1 wire. Keep its
	// frozen tokens separate from the Workspace-named public PolicyRule.
	WorkspaceManifestID   string   `json:"context_id"`
	WorkspaceManifestName string   `json:"context"`
	ProjectID             string   `json:"project_id"`
	ProjectRoot           string   `json:"project_root"`
	Host                  string   `json:"host"`
	Port                  int      `json:"port"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	SourceCandidates      []string `json:"source_candidates"`
}

func policyDenyRuleIDWithIdentity(
	contextID, projectID, host string, port int, method, path string, sourceCandidates []string,
	identity PolicyProtocolIdentity,
) string {
	material := appendPolicyProtocolIdentity(
		[]string{
			"tobari-policy-deny-v1", contextID, projectID, host, strconv.Itoa(port), method, path,
			strings.Join(sourceCandidates, "\x1f"),
		},
		identity,
	)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return "pdr_" + hex.EncodeToString(sum[:16])
}

// NewExactPolicyDenyRule binds a rejection to one exact candidate.
func NewExactPolicyDenyRule(candidate PolicyCandidate) (PolicyDenyRule, error) {
	if err := candidate.Validate(); err != nil {
		return PolicyDenyRule{}, err
	}
	if candidate.EffectiveDestinationKind() != PolicyDestinationExternal || candidate.EffectiveAuthorityLifetime() != AuthorityLifetimePersistent {
		return PolicyDenyRule{}, fmt.Errorf("attachment candidate cannot become a persistent deny rule")
	}
	rule := PolicyDenyRule{
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		WorkspaceManifestID:    candidate.WorkspaceManifestID, WorkspaceManifestName: candidate.WorkspaceManifestName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port,
		Method: candidate.Method, Path: candidate.Path,
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = policyDenyRuleIDWithIdentity(
		rule.WorkspaceManifestID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.SourceCandidates,
		rule.PolicyProtocolIdentity,
	)
	return rule, nil
}

func (r PolicyDenyRule) Validate() error {
	if err := r.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("policy deny rule protocol identity: %w", err)
	}
	if !policyDenyRuleIDPattern.MatchString(r.ID) {
		return fmt.Errorf("policy deny rule ID is invalid")
	}
	if err := validatePolicyScope(r.WorkspaceManifestID, r.WorkspaceManifestName, r.ProjectRoot); err != nil {
		return fmt.Errorf("policy deny rule scope: %w", err)
	}
	if err := ValidateWorkspaceID(r.ProjectID); err != nil {
		return fmt.Errorf("policy deny rule project ID is invalid")
	}
	if len(r.Host) == 0 || len(r.Host) > 253 || containsSpaceOrControl(r.Host) {
		return fmt.Errorf("policy deny rule host is invalid")
	}
	if r.Port < 1 || r.Port > 65535 {
		return fmt.Errorf("policy deny rule port is invalid")
	}
	if !httpMethodPattern.MatchString(r.Method) {
		return fmt.Errorf("policy deny rule method is invalid")
	}
	if err := validatePolicyPath(r.Path); err != nil {
		return fmt.Errorf("policy deny rule path is invalid")
	}
	if err := validateSortedUniqueCandidateIDs(r.SourceCandidates); err != nil {
		return fmt.Errorf("policy deny rule sources: %w", err)
	}
	if len(r.SourceCandidates) != 1 {
		return fmt.Errorf("exact policy deny rule must have one source candidate")
	}
	if r.ID != policyDenyRuleIDWithIdentity(
		r.WorkspaceManifestID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.SourceCandidates, r.PolicyProtocolIdentity,
	) {
		return fmt.Errorf("policy deny rule ID does not bind its content")
	}
	return nil
}

func (r PolicyDenyRule) MatchesIdentity(
	contextID, projectID, host string, port int, method, path string, identity PolicyProtocolIdentity,
) bool {
	return r.WorkspaceManifestID == contextID && r.ProjectID == projectID && r.Host == host && r.Port == port &&
		r.Method == method && r.Path == path && r.PolicyProtocolIdentity.matches(identity)
}

// PolicyDenyRuleSet is the current exact-deny projection used to remove
// already decided effects from the review queue.
type PolicyDenyRuleSet struct {
	Exact []PolicyDenyRule `json:"exact"`
}

func (s PolicyDenyRuleSet) Validate() error {
	if s.Exact == nil {
		return fmt.Errorf("policy deny rule collections are unknown")
	}
	seen := make(map[string]bool, len(s.Exact))
	for _, rule := range s.Exact {
		if err := rule.Validate(); err != nil {
			return err
		}
		if seen[rule.ID] {
			return fmt.Errorf("policy deny rule IDs must be unique")
		}
		seen[rule.ID] = true
	}
	return nil
}

func (s PolicyDenyRuleSet) Matches(denial PolicyDenial) bool {
	for _, rule := range s.Exact {
		if rule.MatchesIdentity(
			denial.WorkspaceManifestID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path,
			denial.PolicyProtocolIdentity,
		) {
			return true
		}
	}
	return false
}

// LearnedPolicyRule is one exact allow.json rule owned by policy-learning commands.
type LearnedPolicyRule struct {
	PolicyProtocolIdentity
	ID                    string   `json:"id"`
	Match                 string   `json:"match"`
	WorkspaceManifestID   string   `json:"context_id"`
	WorkspaceManifestName string   `json:"context"`
	ProjectID             string   `json:"project_id"`
	ProjectRoot           string   `json:"project_root"`
	Host                  string   `json:"host"`
	Port                  int      `json:"port"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	Segments              []string `json:"segments,omitempty"`
	Examples              []string `json:"examples"`
	SourceCandidates      []string `json:"source_candidates"`
}

func learnedRuleIDWithIdentity(
	match, contextID, projectID, host string, port int, method, path string, examples, sourceCandidates []string,
	identity PolicyProtocolIdentity,
) string {
	material := appendPolicyProtocolIdentity(
		[]string{
			"tobari-learned-rule-v2", match, contextID, projectID, host, strconv.Itoa(port), method, path,
			strings.Join(examples, "\x1f"), strings.Join(sourceCandidates, "\x1f"),
		},
		identity,
	)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return "plr_" + hex.EncodeToString(sum[:16])
}

func NewExactLearnedPolicyRule(candidate PolicyCandidate) (LearnedPolicyRule, error) {
	if err := candidate.Validate(); err != nil {
		return LearnedPolicyRule{}, err
	}
	if candidate.EffectiveDestinationKind() != PolicyDestinationExternal || candidate.EffectiveAuthorityLifetime() != AuthorityLifetimePersistent {
		return LearnedPolicyRule{}, fmt.Errorf("attachment candidate cannot become a persistent learned rule")
	}
	rule := LearnedPolicyRule{
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		Match:                  PolicyMatchExact, WorkspaceManifestID: candidate.WorkspaceManifestID, WorkspaceManifestName: candidate.WorkspaceManifestName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port, Method: candidate.Method,
		Path: candidate.Path, Examples: []string{candidate.Path},
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = learnedRuleIDWithIdentity(
		rule.Match, rule.WorkspaceManifestID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.Examples, rule.SourceCandidates,
		rule.PolicyProtocolIdentity,
	)
	return rule, nil
}

func (r LearnedPolicyRule) Validate() error {
	if err := r.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("learned rule protocol identity: %w", err)
	}
	if !learnedRuleIDPattern.MatchString(r.ID) {
		return fmt.Errorf("learned rule ID is invalid")
	}
	if !validPolicyMatch(r.Match) {
		return fmt.Errorf("learned rule match is invalid")
	}
	if err := validatePolicyScope(r.WorkspaceManifestID, r.WorkspaceManifestName, r.ProjectRoot); err != nil {
		return fmt.Errorf("learned rule scope: %w", err)
	}
	if err := ValidateWorkspaceID(r.ProjectID); err != nil {
		return fmt.Errorf("learned rule project ID is invalid")
	}
	if len(r.Host) == 0 || len(r.Host) > 253 || containsSpaceOrControl(r.Host) {
		return fmt.Errorf("learned rule host is invalid")
	}
	if r.Port < 1 || r.Port > 65535 {
		return fmt.Errorf("learned rule port is invalid")
	}
	if !httpMethodPattern.MatchString(r.Method) {
		return fmt.Errorf("learned rule method is invalid")
	}
	if r.Match == PolicyMatchExact {
		if err := validatePolicyPath(r.Path); err != nil {
			return fmt.Errorf("learned rule path is invalid")
		}
	} else if err := validatePathTemplate(r.Path, r.Segments); err != nil {
		return fmt.Errorf("learned rule path template is invalid: %w", err)
	}
	if r.Examples == nil || r.SourceCandidates == nil {
		return fmt.Errorf("learned rule evidence is unknown")
	}
	minimumEvidence := 1
	if r.Match == PolicyMatchPathTemplate {
		minimumEvidence = 2
	}
	if len(r.Examples) < minimumEvidence || len(r.SourceCandidates) < minimumEvidence {
		return fmt.Errorf("learned rule has insufficient evidence")
	}
	if err := validateSortedUniquePaths(r.Examples); err != nil {
		return fmt.Errorf("learned rule examples: %w", err)
	}
	if err := validateSortedUniqueCandidateIDs(r.SourceCandidates); err != nil {
		return fmt.Errorf("learned rule sources: %w", err)
	}
	for _, example := range r.Examples {
		if r.Match == PolicyMatchExact && example != r.Path {
			return fmt.Errorf("exact learned rule example must equal its path")
		}
		if r.Match == PolicyMatchPathTemplate && !pathTemplateMatches(r.Segments, example) {
			return fmt.Errorf("path-template learned rule example must match its template")
		}
	}
	if r.Match == PolicyMatchExact && len(r.Segments) != 0 {
		return fmt.Errorf("exact learned rule cannot contain template segments")
	}
	if r.Match == PolicyMatchPathTemplate && r.EffectiveProtocol() != PolicyProtocolHTTP {
		return fmt.Errorf("non-HTTP learned rules cannot use a path template")
	}
	if r.ID != learnedRuleIDWithIdentity(
		r.Match, r.WorkspaceManifestID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.Examples, r.SourceCandidates,
		r.PolicyProtocolIdentity,
	) {
		return fmt.Errorf("learned rule ID does not bind its content")
	}
	return nil
}

func validateSortedUniquePaths(values []string) error {
	previous := ""
	for index, value := range values {
		if err := validatePolicyPath(value); err != nil {
			return err
		}
		if index > 0 && value <= previous {
			return fmt.Errorf("paths must be unique and sorted")
		}
		previous = value
	}
	return nil
}

func validateSortedUniqueCandidateIDs(values []string) error {
	previous := ""
	for index, value := range values {
		if err := ValidatePolicyCandidateID(value); err != nil {
			return err
		}
		if index > 0 && value <= previous {
			return fmt.Errorf("candidate IDs must be unique and sorted")
		}
		previous = value
	}
	return nil
}

func ValidateLearnedPolicyRules(rules []LearnedPolicyRule) error {
	if rules == nil {
		return fmt.Errorf("learned rule collection is unknown")
	}
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if err := rule.Validate(); err != nil {
			return err
		}
		if seen[rule.ID] {
			return fmt.Errorf("learned rule IDs must be unique")
		}
		seen[rule.ID] = true
	}
	return nil
}

// PolicyRule is the presentation-independent current decision shape shared by
// learned Allow and exact learned Deny inventory. Baseline host policy is not
// represented because it has no reversible CLI-owned decision.
type PolicyRule struct {
	PolicyProtocolIdentity
	ID                    string   `json:"id"`
	Decision              string   `json:"decision"`
	Match                 string   `json:"match"`
	WorkspaceManifestID   string   `json:"workspace_manifest_id"`
	WorkspaceManifestName string   `json:"workspace_manifest"`
	ProjectID             string   `json:"workspace_id"`
	ProjectRoot           string   `json:"project_root"`
	Host                  string   `json:"host"`
	Port                  int      `json:"port"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	Segments              []string `json:"segments,omitempty"`
	Examples              []string `json:"examples"`
	SourceCandidates      []string `json:"source_candidates"`
}

// NewPolicyRuleFromLearned converts one validated learned Allow rule into the
// common inventory shape without changing any opaque evidence.
func NewPolicyRuleFromLearned(rule LearnedPolicyRule) (PolicyRule, error) {
	if err := rule.Validate(); err != nil {
		return PolicyRule{}, err
	}
	result := PolicyRule{
		PolicyProtocolIdentity: rule.PolicyProtocolIdentity,
		ID:                     rule.ID, Decision: PolicyDecisionAllow, Match: rule.Match,
		WorkspaceManifestID: rule.WorkspaceManifestID, WorkspaceManifestName: rule.WorkspaceManifestName,
		ProjectID: rule.ProjectID, ProjectRoot: rule.ProjectRoot, Host: rule.Host, Port: rule.Port, Method: rule.Method,
		Path: rule.Path, Segments: append([]string{}, rule.Segments...), Examples: append([]string{}, rule.Examples...),
		SourceCandidates: append([]string{}, rule.SourceCandidates...),
	}
	if err := result.Validate(); err != nil {
		return PolicyRule{}, err
	}
	return result, nil
}

// NewPolicyRuleFromDeny converts one validated exact learned Deny rule into
// the common inventory shape. Deny decisions have no positive examples, so
// their examples collection is explicitly empty rather than unknown.
func NewPolicyRuleFromDeny(rule PolicyDenyRule) (PolicyRule, error) {
	if err := rule.Validate(); err != nil {
		return PolicyRule{}, err
	}
	result := PolicyRule{
		PolicyProtocolIdentity: rule.PolicyProtocolIdentity,
		ID:                     rule.ID, Decision: PolicyDecisionDeny, Match: PolicyMatchExact,
		WorkspaceManifestID: rule.WorkspaceManifestID, WorkspaceManifestName: rule.WorkspaceManifestName,
		ProjectID: rule.ProjectID, ProjectRoot: rule.ProjectRoot, Host: rule.Host, Port: rule.Port, Method: rule.Method,
		Path: rule.Path, Examples: []string{},
		SourceCandidates: append([]string{}, rule.SourceCandidates...),
	}
	if err := result.Validate(); err != nil {
		return PolicyRule{}, err
	}
	return result, nil
}

func (r PolicyRule) Validate() error {
	if err := ValidatePolicyRuleID(r.ID); err != nil {
		return err
	}
	if !validPolicyDecision(r.Decision) {
		return fmt.Errorf("policy rule decision is invalid")
	}
	if r.Decision == PolicyDecisionAllow {
		learned := LearnedPolicyRule{
			PolicyProtocolIdentity: r.PolicyProtocolIdentity,
			ID:                     r.ID, Match: r.Match, WorkspaceManifestID: r.WorkspaceManifestID, WorkspaceManifestName: r.WorkspaceManifestName,
			ProjectID: r.ProjectID, ProjectRoot: r.ProjectRoot, Host: r.Host, Port: r.Port,
			Method: r.Method, Path: r.Path, Segments: r.Segments, Examples: r.Examples,
			SourceCandidates: r.SourceCandidates,
		}
		if err := learned.Validate(); err != nil {
			return fmt.Errorf("policy rule allow: %w", err)
		}
		return nil
	}
	if r.Match != PolicyMatchExact || r.Examples == nil {
		return fmt.Errorf("policy rule deny shape is invalid")
	}
	deny := PolicyDenyRule{
		PolicyProtocolIdentity: r.PolicyProtocolIdentity,
		ID:                     r.ID, WorkspaceManifestID: r.WorkspaceManifestID, WorkspaceManifestName: r.WorkspaceManifestName,
		ProjectID: r.ProjectID, ProjectRoot: r.ProjectRoot, Host: r.Host, Port: r.Port,
		Method: r.Method, Path: r.Path, SourceCandidates: r.SourceCandidates,
	}
	if err := deny.Validate(); err != nil {
		return fmt.Errorf("policy rule deny: %w", err)
	}
	return nil
}

// CurrentPolicyRules returns the complete learned-decision inventory for one
// validated policy snapshot. Baseline host rules are intentionally excluded
// because this report is the reversible CLI-owned decision surface.
func CurrentPolicyRules(learned []LearnedPolicyRule, denies []PolicyDenyRule) ([]PolicyRule, error) {
	if err := ValidateLearnedPolicyRules(learned); err != nil {
		return nil, err
	}
	denySet := PolicyDenyRuleSet{Exact: denies}
	if err := denySet.Validate(); err != nil {
		return nil, err
	}
	items := make([]PolicyRule, 0, len(learned)+len(denies))
	for _, rule := range learned {
		item, err := NewPolicyRuleFromLearned(rule)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	for _, rule := range denies {
		item, err := NewPolicyRuleFromDeny(rule)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Decision != items[j].Decision {
			return items[i].Decision < items[j].Decision
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

// PolicyRuleReport is exhaustive for the current learned-rule file at one
// observation point. An empty Items collection is known and valid.
type PolicyRuleReport struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	Items []PolicyRule `json:"items"`
}

func (r PolicyRuleReport) Validate() error {
	if r.Task != TaskPolicyRules {
		return fmt.Errorf("policy rule report task identity is invalid")
	}
	if err := r.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("policy rule report projection identity: %w", err)
	}
	if r.Items == nil {
		return fmt.Errorf("policy rule collection is unknown")
	}
	seen := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if seen[item.ID] {
			return fmt.Errorf("policy rule IDs must be unique")
		}
		seen[item.ID] = true
	}
	return nil
}

// RemovePolicyRule removes one current learned decision and returns the
// resulting collections plus the removed decision. It never touches baseline
// host policy and never synthesizes a replacement authorization.
func RemovePolicyRule(
	learned []LearnedPolicyRule, denies []PolicyDenyRule, id string,
) ([]LearnedPolicyRule, []PolicyDenyRule, PolicyRule, error) {
	if err := ValidatePolicyRuleID(id); err != nil {
		return nil, nil, PolicyRule{}, err
	}
	if err := ValidateLearnedPolicyRules(learned); err != nil {
		return nil, nil, PolicyRule{}, err
	}
	denySet := PolicyDenyRuleSet{Exact: denies}
	if err := denySet.Validate(); err != nil {
		return nil, nil, PolicyRule{}, err
	}
	updatedLearned := make([]LearnedPolicyRule, 0, len(learned))
	var removed PolicyRule
	found := false
	for _, rule := range learned {
		if rule.ID != id {
			updatedLearned = append(updatedLearned, rule)
			continue
		}
		var err error
		removed, err = NewPolicyRuleFromLearned(rule)
		if err != nil {
			return nil, nil, PolicyRule{}, err
		}
		found = true
	}
	updatedDenies := make([]PolicyDenyRule, 0, len(denies))
	for _, rule := range denies {
		if rule.ID != id {
			updatedDenies = append(updatedDenies, rule)
			continue
		}
		if found {
			return nil, nil, PolicyRule{}, fmt.Errorf("policy rule ID is duplicated across decisions")
		}
		var err error
		removed, err = NewPolicyRuleFromDeny(rule)
		if err != nil {
			return nil, nil, PolicyRule{}, err
		}
		found = true
	}
	if !found {
		return nil, nil, PolicyRule{}, fmt.Errorf("policy rule is not current")
	}
	return updatedLearned, updatedDenies, removed, nil
}

// PolicyRuleReset is the confirmed result of returning one learned decision
// to the initialized default-deny behavior.
type PolicyRuleReset struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	TargetID string `json:"target_id"`
	Decision string `json:"decision"`
	Applied  bool   `json:"applied"`
}

func (r PolicyRuleReset) Validate() error {
	if r.Task != TaskPolicyReset {
		return fmt.Errorf("policy rule reset task identity is invalid")
	}
	if err := ValidatePolicyRuleID(r.TargetID); err != nil {
		return err
	}
	if !validPolicyDecision(r.Decision) {
		return fmt.Errorf("policy rule reset decision is invalid")
	}
	if err := r.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("policy rule reset projection identity: %w", err)
	}
	if !r.Applied {
		return fmt.Errorf("policy rule reset is not applied")
	}
	return nil
}

func (r LearnedPolicyRule) MatchesIdentity(
	contextID, projectID, host string, port int, method, path string, identity PolicyProtocolIdentity,
) bool {
	if r.WorkspaceManifestID != contextID || r.ProjectID != projectID || r.Host != host || r.Port != port || r.Method != method {
		return false
	}
	if !r.PolicyProtocolIdentity.matches(identity) {
		return false
	}
	switch r.Match {
	case PolicyMatchExact:
		return r.Path == path
	case PolicyMatchPathTemplate:
		return pathTemplateMatches(r.Segments, path)
	default:
		return false
	}
}

func PolicyCandidates(
	denials []PolicyDenial, rules []LearnedPolicyRule,
) ([]PolicyCandidate, error) {
	return PolicyCandidatesWithDenyRules(denials, rules, PolicyDenyRuleSet{
		Exact: []PolicyDenyRule{},
	})
}

// PolicyCandidatesWithDenyRules returns only learnable retained denials that
// are not already covered by an allow or an effective deny rule.
func PolicyCandidatesWithDenyRules(
	denials []PolicyDenial, rules []LearnedPolicyRule, denyRules PolicyDenyRuleSet,
) ([]PolicyCandidate, error) {
	if denials == nil {
		return nil, fmt.Errorf("denial collection is unknown")
	}
	if err := ValidateLearnedPolicyRules(rules); err != nil {
		return nil, err
	}
	if err := denyRules.Validate(); err != nil {
		return nil, err
	}
	covered := func(denial PolicyDenial) bool {
		if denial.EffectiveDestinationKind() == PolicyDestinationHostLoopback {
			return false
		}
		for _, rule := range rules {
			if rule.MatchesIdentity(
				denial.WorkspaceManifestID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path,
				denial.PolicyProtocolIdentity,
			) {
				return true
			}
		}
		return false
	}
	type effectKey struct {
		contextID         string
		projectID         string
		host              string
		port              int
		method            string
		path              string
		protocol          string
		protocolKey       string
		destinationKind   string
		attachmentEpochID string
	}
	type aggregate struct {
		candidate   PolicyCandidate
		observedAt  time.Time
		latestIndex int
	}
	aggregates := make(map[effectKey]aggregate, len(denials))
	for index, denial := range denials {
		if err := denial.Validate(); err != nil {
			return nil, err
		}
		persistentDenyCovered := denial.EffectiveDestinationKind() != PolicyDestinationHostLoopback && denyRules.Matches(denial)
		if !denial.Learnable || covered(denial) || persistentDenyCovered {
			continue
		}
		candidate, err := NewPolicyCandidate(denial)
		if err != nil {
			return nil, err
		}
		observedAt, err := time.Parse(time.RFC3339Nano, denial.Timestamp)
		if err != nil {
			return nil, err
		}
		key := effectKey{
			contextID: denial.WorkspaceManifestID, projectID: denial.ProjectID, host: denial.Host, port: denial.Port,
			method: denial.Method, path: denial.Path, protocol: denial.EffectiveProtocol(),
			protocolKey:     strings.Join(appendPolicyProtocolIdentity(nil, denial.PolicyProtocolIdentity), "\x00"),
			destinationKind: denial.EffectiveDestinationKind(), attachmentEpochID: denial.AttachmentEpochID,
		}
		current, found := aggregates[key]
		if !found {
			aggregates[key] = aggregate{candidate: candidate, observedAt: observedAt, latestIndex: index}
			continue
		}
		count := current.candidate.EffectiveObservationCount() + 1
		if observedAt.After(current.observedAt) || observedAt.Equal(current.observedAt) {
			candidate.ObservationCount = count
			aggregates[key] = aggregate{candidate: candidate, observedAt: observedAt, latestIndex: index}
			continue
		}
		current.candidate.ObservationCount = count
		aggregates[key] = current
	}
	ordered := make([]aggregate, 0, len(aggregates))
	for _, item := range aggregates {
		ordered = append(ordered, item)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].observedAt.Equal(ordered[j].observedAt) {
			return ordered[i].latestIndex < ordered[j].latestIndex
		}
		return ordered[i].observedAt.Before(ordered[j].observedAt)
	})
	items := make([]PolicyCandidate, len(ordered))
	for index, item := range ordered {
		items[index] = item.candidate
	}
	return items, nil
}

// PolicyActivationReceipt is the minimal authoritative result of a confirmed
// aggregate activation. It is produced inside the activation boundary so a
// caller never needs a fallible post-success state reload.
type PolicyActivationReceipt struct {
	ActiveRevision     string
	EvaluatorIdentity  PolicyEvaluatorIdentity
	PolicyDataIdentity PolicyDataIdentity
}

// ValidateAggregate proves the receipt came from the shared aggregate
// activation boundary. Persistent policy mutations must never accept the
// attachment-shaped empty identity pair.
func (r PolicyActivationReceipt) ValidateAggregate() error {
	if !policyRevisionPattern.MatchString(r.ActiveRevision) {
		return fmt.Errorf("aggregate policy activation revision is invalid")
	}
	if r.EvaluatorIdentity == (PolicyEvaluatorIdentity{}) || r.PolicyDataIdentity == (PolicyDataIdentity{}) {
		return fmt.Errorf("aggregate policy activation requires evaluator and policy-data identities")
	}
	if err := (PolicyProjectionIdentity{
		AggregateRevision:  r.ActiveRevision,
		EvaluatorIdentity:  r.EvaluatorIdentity,
		PolicyDataIdentity: r.PolicyDataIdentity,
	}).Validate(); err != nil {
		return err
	}
	return nil
}

// ValidateAttachment proves the receipt belongs to the separate Host
// Loopback attachment registry. It intentionally carries no aggregate policy
// identity because attachment state is not shared-cluster policy authority.
func (r PolicyActivationReceipt) ValidateAttachment() error {
	if !policyRevisionPattern.MatchString(r.ActiveRevision) {
		return fmt.Errorf("attachment activation revision is invalid")
	}
	if r.EvaluatorIdentity != (PolicyEvaluatorIdentity{}) || r.PolicyDataIdentity != (PolicyDataIdentity{}) {
		return fmt.Errorf("attachment activation cannot carry aggregate identities")
	}
	return nil
}

// PolicyLearningChange is a confirmed exact approval result.
type PolicyLearningChange struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	TargetID        string            `json:"target_id"`
	Rule            LearnedPolicyRule `json:"rule"`
	SourceRuleCount int               `json:"source_rule_count"`
	Applied         bool              `json:"applied"`
}

func (c PolicyLearningChange) Validate() error {
	if c.Task != TaskPolicyAllow {
		return fmt.Errorf("policy learning result task identity is invalid")
	}
	if err := ValidatePolicyCandidateID(c.TargetID); err != nil {
		return err
	}
	if c.Rule.Match != PolicyMatchExact || c.SourceRuleCount != 1 {
		return fmt.Errorf("policy allow result is inconsistent")
	}
	if err := c.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("policy learning projection identity: %w", err)
	}
	if err := c.Rule.Validate(); err != nil {
		return err
	}
	if !c.Applied {
		return fmt.Errorf("policy learning result is not applied")
	}
	return nil
}

// PolicyDenyChange is the confirmed result of rejecting one exact candidate.
type PolicyDenyChange struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	TargetID        string         `json:"target_id"`
	Rule            PolicyDenyRule `json:"rule"`
	SourceRuleCount int            `json:"source_rule_count"`
	Applied         bool           `json:"applied"`
}

const MaxPolicyReviewDecisions = 128

// PolicyReviewDecision is one explicit choice retained from a validated review
// detail screen. ReviewItemID remains opaque and Match states whether an Allow
// accepts the proposed template or keeps authority exact.
type PolicyReviewDecision struct {
	ReviewItemID string `json:"review_item_id"`
	Decision     string `json:"decision"`
	Match        string `json:"match"`
}

// PolicyReviewDecisionSet is the bounded typed payload of one final human
// Apply. It is content of the command-owned target, not a collection of public
// mutation targets.
type PolicyReviewDecisionSet struct {
	Decisions []PolicyReviewDecision `json:"decisions"`
}

// PolicyReviewAppliedDecision is one secret-free stored rule repeated in the
// confirmed receipt. RuleID identifies the actual exact or template decision;
// ReviewItemID identifies the freshly revalidated detail choice that created it.
type PolicyReviewAppliedDecision struct {
	PolicyProtocolIdentity
	RuleID                string   `json:"rule_id"`
	ReviewItemID          string   `json:"review_item_id"`
	Decision              string   `json:"decision"`
	Match                 string   `json:"match"`
	WorkspaceManifestID   string   `json:"workspace_manifest_id"`
	WorkspaceManifestName string   `json:"workspace_manifest"`
	ProjectID             string   `json:"workspace_id"`
	ProjectRoot           string   `json:"project_root"`
	Host                  string   `json:"host"`
	Port                  int      `json:"port"`
	Method                string   `json:"method"`
	Path                  string   `json:"path"`
	SourceCandidates      []string `json:"source_candidates"`
	DestinationKind       string   `json:"destination_kind,omitempty"`
	AuthorityLifetime     string   `json:"authority_lifetime,omitempty"`
	AttachmentEpochID     string   `json:"attachment_epoch_id,omitempty"`
}

func (d PolicyReviewAppliedDecision) Validate() error {
	if err := d.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("policy review receipt protocol identity: %w", err)
	}
	attachment := d.DestinationKind == PolicyDestinationHostLoopback
	if attachment {
		if !attachmentGrantPattern.MatchString(d.RuleID) || d.AuthorityLifetime != AuthorityLifetimeAttachment || ValidateAttachmentEpochID(d.AttachmentEpochID) != nil || d.Host != HostLoopbackHostname || ValidateHostLoopbackPort(d.Port) != nil {
			return fmt.Errorf("policy review attachment receipt is invalid")
		}
	} else {
		if err := ValidatePolicyRuleID(d.RuleID); err != nil {
			return err
		}
		if d.DestinationKind != "" || d.AuthorityLifetime != "" || d.AttachmentEpochID != "" {
			return fmt.Errorf("persistent policy review receipt has attachment authority")
		}
	}
	if err := ValidatePolicyReviewItemID(d.ReviewItemID); err != nil {
		return err
	}
	if !validPolicyDecision(d.Decision) {
		return fmt.Errorf("policy review receipt decision is invalid")
	}
	if !validPolicyMatch(d.Match) {
		return fmt.Errorf("policy review receipt match is invalid")
	}
	if d.Decision == PolicyDecisionDeny && d.Match != PolicyMatchExact {
		return fmt.Errorf("policy review deny receipt must remain exact")
	}
	if err := validatePolicyScope(d.WorkspaceManifestID, d.WorkspaceManifestName, d.ProjectRoot); err != nil {
		return fmt.Errorf("policy review receipt scope: %w", err)
	}
	if err := ValidateWorkspaceID(d.ProjectID); err != nil {
		return fmt.Errorf("policy review receipt project ID is invalid")
	}
	if len(d.Host) == 0 || len(d.Host) > 253 || containsSpaceOrControl(d.Host) {
		return fmt.Errorf("policy review receipt host is invalid")
	}
	if d.Port < 1 || d.Port > 65535 {
		return fmt.Errorf("policy review receipt port is invalid")
	}
	if !httpMethodPattern.MatchString(d.Method) {
		return fmt.Errorf("policy review receipt method is invalid")
	}
	if d.Match == PolicyMatchExact {
		if err := validatePolicyPath(d.Path); err != nil {
			return fmt.Errorf("policy review receipt path is invalid")
		}
	} else if !strings.Contains(d.Path, PolicyPathTemplatePlaceholder) {
		return fmt.Errorf("policy review receipt template path is invalid")
	}
	if err := validateSortedUniqueCandidateIDs(d.SourceCandidates); err != nil {
		return fmt.Errorf("policy review receipt sources: %w", err)
	}
	return nil
}

func (s PolicyReviewDecisionSet) Validate() error {
	if len(s.Decisions) == 0 || len(s.Decisions) > MaxPolicyReviewDecisions {
		return fmt.Errorf("policy review decision count must be between 1 and %d", MaxPolicyReviewDecisions)
	}
	seen := make(map[string]struct{}, len(s.Decisions))
	for _, decision := range s.Decisions {
		if err := ValidatePolicyReviewItemID(decision.ReviewItemID); err != nil {
			return err
		}
		if !validPolicyDecision(decision.Decision) {
			return fmt.Errorf("policy review decision is invalid")
		}
		if !validPolicyMatch(decision.Match) {
			return fmt.Errorf("policy review decision match is invalid")
		}
		if decision.Decision == PolicyDecisionDeny && decision.Match != PolicyMatchExact {
			return fmt.Errorf("policy review deny decision must remain exact")
		}
		if _, duplicate := seen[decision.ReviewItemID]; duplicate {
			return fmt.Errorf("policy review decision set contains a duplicate review item")
		}
		seen[decision.ReviewItemID] = struct{}{}
	}
	return nil
}

// PolicyReviewChange is emitted only after the complete reviewed set is active.
type PolicyReviewChange struct {
	Task string `json:"task"`
	PolicyProjectionIdentity
	AllowCount     int                           `json:"allow_count"`
	DenyCount      int                           `json:"deny_count"`
	Applied        bool                          `json:"applied"`
	ActiveRevision string                        `json:"active_revision"`
	Decisions      []PolicyReviewAppliedDecision `json:"decisions"`
}

func (c PolicyReviewChange) Validate() error {
	if c.Task != TaskPolicyReviewApply || c.AllowCount < 0 || c.DenyCount < 0 ||
		c.AllowCount+c.DenyCount == 0 || c.AllowCount+c.DenyCount > MaxPolicyReviewDecisions || !c.Applied {
		return fmt.Errorf("policy review result is inconsistent")
	}
	if !policyRevisionPattern.MatchString(c.ActiveRevision) {
		return fmt.Errorf("policy review active revision is invalid")
	}
	identitiesEmpty := c.PolicyProjectionIdentity == (PolicyProjectionIdentity{})
	if !identitiesEmpty && (c.AggregateRevision != c.ActiveRevision || c.PolicyProjectionIdentity.Validate() != nil) {
		return fmt.Errorf("policy review projection identity is invalid")
	}
	if len(c.Decisions) != c.AllowCount+c.DenyCount {
		return fmt.Errorf("policy review receipt count is inconsistent")
	}
	allowCount, denyCount := 0, 0
	seen := make(map[string]struct{}, len(c.Decisions))
	for _, decision := range c.Decisions {
		if err := decision.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[decision.RuleID]; duplicate {
			return fmt.Errorf("policy review receipt contains a duplicate rule")
		}
		seen[decision.RuleID] = struct{}{}
		if decision.Decision == PolicyDecisionAllow {
			allowCount++
		} else {
			denyCount++
		}
	}
	if identitiesEmpty {
		for _, decision := range c.Decisions {
			if decision.DestinationKind != PolicyDestinationHostLoopback {
				return fmt.Errorf("persistent policy review result is missing projection identity")
			}
		}
	}
	if allowCount != c.AllowCount || denyCount != c.DenyCount {
		return fmt.Errorf("policy review receipt decisions do not match counts")
	}
	return nil
}

func (c PolicyDenyChange) Validate() error {
	if c.Task != TaskPolicyDeny {
		return fmt.Errorf("policy deny result task identity is invalid")
	}
	if err := ValidatePolicyCandidateID(c.TargetID); err != nil {
		return err
	}
	if c.SourceRuleCount != 1 || !c.Applied {
		return fmt.Errorf("policy deny result is inconsistent")
	}
	if err := c.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("policy deny projection identity: %w", err)
	}
	if err := c.Rule.Validate(); err != nil {
		return err
	}
	return nil
}
