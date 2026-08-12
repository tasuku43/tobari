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
	policyDenyRuleIDPattern  = regexp.MustCompile(`^pdr_[0-9a-f]{32}$`)
	learnedRuleIDPattern     = regexp.MustCompile(`^plr_[0-9a-f]{32}$`)
	httpMethodPattern        = regexp.MustCompile(`^[A-Z][A-Z0-9!#$%&'*+.^_` + "`" + `|~-]{0,31}$`)
	graphqlNamePattern       = regexp.MustCompile(`^[_A-Za-z][_0-9A-Za-z]*$`)
	policyRevisionPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const (
	PolicyMatchExact         = "exact"
	PolicyDecisionAllow      = "allow"
	PolicyDecisionDeny       = "deny"
	PolicyProtocolHTTP       = "http"
	PolicyProtocolGraphQL    = "graphql"
	GraphQLOperationQuery    = "query"
	GraphQLOperationMutation = "mutation"
)

// PolicyProtocolIdentity identifies one HTTP effect or refines it to exactly
// one GraphQL root coordinate.
type PolicyProtocolIdentity struct {
	Protocol             string `json:"protocol"`
	GraphQLOperationType string `json:"graphql_operation_type,omitempty"`
	GraphQLRootField     string `json:"graphql_root_field,omitempty"`
}

// EffectiveProtocol returns the validated closed protocol value.
func (i PolicyProtocolIdentity) EffectiveProtocol() string {
	return i.Protocol
}

func (i PolicyProtocolIdentity) Validate() error {
	switch i.EffectiveProtocol() {
	case PolicyProtocolHTTP:
		if i.GraphQLOperationType != "" || i.GraphQLRootField != "" {
			return fmt.Errorf("HTTP policy identity cannot contain GraphQL fields")
		}
	case PolicyProtocolGraphQL:
		if i.GraphQLOperationType != GraphQLOperationQuery && i.GraphQLOperationType != GraphQLOperationMutation {
			return fmt.Errorf("GraphQL operation type is invalid")
		}
		if len(i.GraphQLRootField) == 0 || len(i.GraphQLRootField) > 256 || !graphqlNamePattern.MatchString(i.GraphQLRootField) {
			return fmt.Errorf("GraphQL root field is invalid")
		}
	default:
		return fmt.Errorf("policy protocol is invalid")
	}
	return nil
}

func (i PolicyProtocolIdentity) matches(other PolicyProtocolIdentity) bool {
	return i.EffectiveProtocol() == other.EffectiveProtocol() &&
		i.GraphQLOperationType == other.GraphQLOperationType &&
		i.GraphQLRootField == other.GraphQLRootField
}

func appendPolicyProtocolIdentity(material []string, identity PolicyProtocolIdentity) []string {
	if identity.EffectiveProtocol() == PolicyProtocolHTTP {
		return material
	}
	return append(material, PolicyProtocolGraphQL, identity.GraphQLOperationType, identity.GraphQLRootField)
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

// PolicyDenial is one validated, secret-free Gateway audit decision.
type PolicyDenial struct {
	PolicyProtocolIdentity
	Timestamp         string  `json:"timestamp"`
	RequestID         string  `json:"request_id"`
	ContextID         string  `json:"context_id"`
	ContextName       string  `json:"context"`
	ProjectID         string  `json:"project_id"`
	ProjectRoot       string  `json:"project_root"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Reason            string  `json:"reason"`
	StatusCode        int     `json:"status_code"`
	Learnable         bool    `json:"learnable"`
	CredentialProfile *string `json:"credential_profile"`
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
	if err := validatePolicyScope(d.ContextID, d.ContextName, d.ProjectRoot); err != nil {
		return fmt.Errorf("denial scope: %w", err)
	}
	if err := ValidateProjectID(d.ProjectID); err != nil {
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
	if err := validateOptionalCredentialProfile(d.CredentialProfile); err != nil {
		return err
	}
	return nil
}

func validatePolicyScope(contextID, contextName, projectRoot string) error {
	if err := ValidateContextID(contextID); err != nil {
		return fmt.Errorf("Context ID is invalid")
	}
	if err := ValidateName(contextName); err != nil {
		return fmt.Errorf("Context name is invalid")
	}
	if !filepath.IsAbs(projectRoot) || filepath.Clean(projectRoot) != projectRoot {
		return fmt.Errorf("project root is invalid")
	}
	return nil
}

func validateOptionalCredentialProfile(value *string) error {
	if value == nil {
		return nil
	}
	if *value == "" || len(*value) > 256 || !utf8.ValidString(*value) ||
		strings.IndexFunc(*value, func(r rune) bool {
			return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
		}) >= 0 {
		return fmt.Errorf("credential profile is invalid")
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

// DenialReport preserves the exact local cluster scope and requested bounded
// Gateway-log window, including a valid empty result.
type DenialReport struct {
	Task            string         `json:"task"`
	PolicyDirectory string         `json:"policy"`
	WindowLines     int            `json:"window_lines"`
	Items           []PolicyDenial `json:"items"`
}

// Validate binds denial evidence to the cluster-denials task and its scope.
func (r DenialReport) Validate() error {
	if r.Task != TaskClusterDenials {
		return fmt.Errorf("denial report task identity is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("denial report policy directory is invalid")
	}
	if r.WindowLines < 1 || r.WindowLines > 10_000 {
		return fmt.Errorf("denial report window is invalid")
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
	ID                string  `json:"id"`
	ObservedAt        string  `json:"observed_at"`
	ObservationCount  int     `json:"observation_count"`
	ContextID         string  `json:"context_id"`
	ContextName       string  `json:"context"`
	ProjectID         string  `json:"project_id"`
	ProjectRoot       string  `json:"project_root"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	Method            string  `json:"method"`
	Path              string  `json:"path"`
	Reason            string  `json:"reason"`
	StatusCode        int     `json:"status_code"`
	CredentialProfile *string `json:"credential_profile"`
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
			"tobari-policy-candidate-v1", denial.ContextID, denial.ProjectID, denial.Host, strconv.Itoa(denial.Port), denial.Method, denial.Path,
		},
		denial.PolicyProtocolIdentity,
	)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return PolicyCandidate{
		PolicyProtocolIdentity: denial.PolicyProtocolIdentity,
		ID:                     "pcy_" + hex.EncodeToString(sum[:16]), ObservedAt: denial.Timestamp, ObservationCount: 1,
		ContextID: denial.ContextID, ContextName: denial.ContextName,
		ProjectID: denial.ProjectID, ProjectRoot: denial.ProjectRoot,
		Host: denial.Host, Port: denial.Port, Method: denial.Method, Path: denial.Path,
		Reason: denial.Reason, StatusCode: denial.StatusCode,
		CredentialProfile: cloneStringPointer(denial.CredentialProfile),
	}, nil
}

// EffectiveObservationCount returns the required retained observation count.
func (c PolicyCandidate) EffectiveObservationCount() int {
	return c.ObservationCount
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
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
	if err := validatePolicyScope(c.ContextID, c.ContextName, c.ProjectRoot); err != nil {
		return fmt.Errorf("policy candidate scope: %w", err)
	}
	if err := ValidateProjectID(c.ProjectID); err != nil {
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
	if err := validateOptionalCredentialProfile(c.CredentialProfile); err != nil {
		return err
	}
	return nil
}

// PolicyCandidateReport preserves the bounded retained-log scope and supports
// both machine discovery and the human tail projection.
type PolicyCandidateReport struct {
	Task            string            `json:"task"`
	PolicyDirectory string            `json:"policy"`
	WindowLines     int               `json:"window_lines"`
	Items           []PolicyCandidate `json:"items"`
}

func (r PolicyCandidateReport) Validate() error {
	if r.Task != TaskPolicyCandidates && r.Task != TaskPolicyReview {
		return fmt.Errorf("policy candidate report task identity is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("policy candidate policy directory is invalid")
	}
	if r.WindowLines < 1 || r.WindowLines > 10_000 {
		return fmt.Errorf("policy candidate window is invalid")
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
	return nil
}

// PolicyBaselineDenyRule is a trusted host-authored deny matcher. It may be
// broader than one project or port because the host owns its policy source.
// Baseline denies are terminal policy decisions and never become candidates.
type PolicyBaselineDenyRule struct {
	ContextID  string `json:"context_id,omitempty"`
	Host       string `json:"host"`
	Method     string `json:"method"`
	PathPrefix string `json:"path_prefix"`
}

func (r PolicyBaselineDenyRule) Validate() error {
	if r.ContextID != "" {
		if err := ValidateContextID(r.ContextID); err != nil {
			return fmt.Errorf("baseline deny Context is invalid")
		}
	}
	if len(r.Host) == 0 || len(r.Host) > 253 || containsSpaceOrControl(r.Host) {
		return fmt.Errorf("baseline deny host is invalid")
	}
	if !httpMethodPattern.MatchString(r.Method) {
		return fmt.Errorf("baseline deny method is invalid")
	}
	if err := validatePolicyPath(r.PathPrefix); err != nil {
		return fmt.Errorf("baseline deny path prefix is invalid")
	}
	return nil
}

func (r PolicyBaselineDenyRule) Matches(contextID, host, method, path string) bool {
	return (r.ContextID == "" || r.ContextID == contextID) && r.Host == host && r.Method == method && strings.HasPrefix(path, r.PathPrefix)
}

// PolicyDenyRule is one exact project-bound deny created by rejecting a
// candidate. It is CLI-owned policy data and intentionally carries the same
// dimensions as the candidate it resolves.
type PolicyDenyRule struct {
	PolicyProtocolIdentity
	ID               string   `json:"id"`
	ContextID        string   `json:"context_id"`
	ContextName      string   `json:"context"`
	ProjectID        string   `json:"project_id"`
	ProjectRoot      string   `json:"project_root"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	SourceCandidates []string `json:"source_candidates"`
}

func policyDenyRuleID(contextID, projectID, host string, port int, method, path string, sourceCandidates []string) string {
	return policyDenyRuleIDWithIdentity(
		contextID, projectID, host, port, method, path, sourceCandidates,
		PolicyProtocolIdentity{Protocol: PolicyProtocolHTTP},
	)
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
	rule := PolicyDenyRule{
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		ContextID:              candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port,
		Method: candidate.Method, Path: candidate.Path,
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = policyDenyRuleIDWithIdentity(
		rule.ContextID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.SourceCandidates,
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
	if err := validatePolicyScope(r.ContextID, r.ContextName, r.ProjectRoot); err != nil {
		return fmt.Errorf("policy deny rule scope: %w", err)
	}
	if err := ValidateProjectID(r.ProjectID); err != nil {
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
		r.ContextID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.SourceCandidates, r.PolicyProtocolIdentity,
	) {
		return fmt.Errorf("policy deny rule ID does not bind its content")
	}
	return nil
}

func (r PolicyDenyRule) Matches(contextID, projectID, host string, port int, method, path string) bool {
	return r.MatchesIdentity(
		contextID, projectID, host, port, method, path,
		PolicyProtocolIdentity{Protocol: PolicyProtocolHTTP},
	)
}

func (r PolicyDenyRule) MatchesIdentity(
	contextID, projectID, host string, port int, method, path string, identity PolicyProtocolIdentity,
) bool {
	return r.ContextID == contextID && r.ProjectID == projectID && r.Host == host && r.Port == port &&
		r.Method == method && r.Path == path && r.PolicyProtocolIdentity.matches(identity)
}

// PolicyDenyRuleSet is the current effective deny projection used to remove
// both baseline and exact-denied effects from the review queue.
type PolicyDenyRuleSet struct {
	Baseline []PolicyBaselineDenyRule `json:"baseline"`
	Exact    []PolicyDenyRule         `json:"exact"`
}

func (s PolicyDenyRuleSet) Validate() error {
	if s.Baseline == nil || s.Exact == nil {
		return fmt.Errorf("policy deny rule collections are unknown")
	}
	seen := make(map[string]bool, len(s.Exact))
	for _, rule := range s.Baseline {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
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
	for _, rule := range s.Baseline {
		if rule.Matches(denial.ContextID, denial.Host, denial.Method, denial.Path) {
			return true
		}
	}
	for _, rule := range s.Exact {
		if rule.MatchesIdentity(
			denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path,
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
	ID               string   `json:"id"`
	Match            string   `json:"match"`
	ContextID        string   `json:"context_id"`
	ContextName      string   `json:"context"`
	ProjectID        string   `json:"project_id"`
	ProjectRoot      string   `json:"project_root"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	Examples         []string `json:"examples"`
	SourceCandidates []string `json:"source_candidates"`
}

func learnedRuleID(
	match, contextID, projectID, host string, port int, method, path string, examples, sourceCandidates []string,
) string {
	return learnedRuleIDWithIdentity(
		match, contextID, projectID, host, port, method, path, examples, sourceCandidates,
		PolicyProtocolIdentity{Protocol: PolicyProtocolHTTP},
	)
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
	rule := LearnedPolicyRule{
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		Match:                  PolicyMatchExact, ContextID: candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port, Method: candidate.Method,
		Path: candidate.Path, Examples: []string{candidate.Path},
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = learnedRuleIDWithIdentity(
		rule.Match, rule.ContextID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.Examples, rule.SourceCandidates,
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
	if r.Match != PolicyMatchExact {
		return fmt.Errorf("learned rule match is invalid")
	}
	if err := validatePolicyScope(r.ContextID, r.ContextName, r.ProjectRoot); err != nil {
		return fmt.Errorf("learned rule scope: %w", err)
	}
	if err := ValidateProjectID(r.ProjectID); err != nil {
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
	if err := validatePolicyPath(r.Path); err != nil {
		return fmt.Errorf("learned rule path is invalid")
	}
	if r.Examples == nil || r.SourceCandidates == nil {
		return fmt.Errorf("learned rule evidence is unknown")
	}
	if len(r.Examples) < 1 || len(r.SourceCandidates) < 1 {
		return fmt.Errorf("learned rule has insufficient evidence")
	}
	if err := validateSortedUniquePaths(r.Examples); err != nil {
		return fmt.Errorf("learned rule examples: %w", err)
	}
	if err := validateSortedUniqueCandidateIDs(r.SourceCandidates); err != nil {
		return fmt.Errorf("learned rule sources: %w", err)
	}
	for _, example := range r.Examples {
		if example != r.Path {
			return fmt.Errorf("exact learned rule example must equal its path")
		}
	}
	if r.ID != learnedRuleIDWithIdentity(
		r.Match, r.ContextID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.Examples, r.SourceCandidates,
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
	ID               string   `json:"id"`
	Decision         string   `json:"decision"`
	Match            string   `json:"match"`
	ContextID        string   `json:"context_id"`
	ContextName      string   `json:"context"`
	ProjectID        string   `json:"project_id"`
	ProjectRoot      string   `json:"project_root"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	Examples         []string `json:"examples"`
	SourceCandidates []string `json:"source_candidates"`
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
		ContextID: rule.ContextID, ContextName: rule.ContextName,
		ProjectID: rule.ProjectID, ProjectRoot: rule.ProjectRoot, Host: rule.Host, Port: rule.Port, Method: rule.Method,
		Path: rule.Path, Examples: append([]string{}, rule.Examples...),
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
		ContextID: rule.ContextID, ContextName: rule.ContextName,
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
	if r.Decision != PolicyDecisionAllow && r.Decision != PolicyDecisionDeny {
		return fmt.Errorf("policy rule decision is invalid")
	}
	if r.Decision == PolicyDecisionAllow {
		learned := LearnedPolicyRule{
			PolicyProtocolIdentity: r.PolicyProtocolIdentity,
			ID:                     r.ID, Match: r.Match, ContextID: r.ContextID, ContextName: r.ContextName,
			ProjectID: r.ProjectID, ProjectRoot: r.ProjectRoot, Host: r.Host, Port: r.Port,
			Method: r.Method, Path: r.Path, Examples: r.Examples,
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
		ID:                     r.ID, ContextID: r.ContextID, ContextName: r.ContextName,
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
	denySet := PolicyDenyRuleSet{Baseline: []PolicyBaselineDenyRule{}, Exact: denies}
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
	Task            string       `json:"task"`
	PolicyDirectory string       `json:"policy"`
	Items           []PolicyRule `json:"items"`
}

func (r PolicyRuleReport) Validate() error {
	if r.Task != TaskPolicyRules {
		return fmt.Errorf("policy rule report task identity is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("policy rule report policy directory is invalid")
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
	denySet := PolicyDenyRuleSet{Baseline: []PolicyBaselineDenyRule{}, Exact: denies}
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
	Task            string `json:"task"`
	PolicyDirectory string `json:"policy"`
	TargetID        string `json:"target_id"`
	Decision        string `json:"decision"`
	Applied         bool   `json:"applied"`
}

func (r PolicyRuleReset) Validate() error {
	if r.Task != TaskPolicyReset {
		return fmt.Errorf("policy rule reset task identity is invalid")
	}
	if err := ValidatePolicyRuleID(r.TargetID); err != nil {
		return err
	}
	if r.Decision != PolicyDecisionAllow && r.Decision != PolicyDecisionDeny {
		return fmt.Errorf("policy rule reset decision is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("policy rule reset policy directory is invalid")
	}
	if !r.Applied {
		return fmt.Errorf("policy rule reset is not applied")
	}
	return nil
}

func (r LearnedPolicyRule) Matches(contextID, projectID, host string, port int, method, path string) bool {
	return r.MatchesIdentity(
		contextID, projectID, host, port, method, path,
		PolicyProtocolIdentity{Protocol: PolicyProtocolHTTP},
	)
}

func (r LearnedPolicyRule) MatchesIdentity(
	contextID, projectID, host string, port int, method, path string, identity PolicyProtocolIdentity,
) bool {
	if r.ContextID != contextID || r.ProjectID != projectID || r.Host != host || r.Port != port || r.Method != method {
		return false
	}
	if !r.PolicyProtocolIdentity.matches(identity) {
		return false
	}
	return r.Match == PolicyMatchExact && r.Path == path
}

func PolicyCandidates(
	denials []PolicyDenial, rules []LearnedPolicyRule,
) ([]PolicyCandidate, error) {
	return PolicyCandidatesWithDenyRules(denials, rules, PolicyDenyRuleSet{
		Baseline: []PolicyBaselineDenyRule{}, Exact: []PolicyDenyRule{},
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
		for _, rule := range rules {
			if rule.MatchesIdentity(
				denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path,
				denial.PolicyProtocolIdentity,
			) {
				return true
			}
		}
		return false
	}
	type effectKey struct {
		contextID            string
		projectID            string
		host                 string
		port                 int
		method               string
		path                 string
		protocol             string
		graphqlOperationType string
		graphqlRootField     string
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
		if !denial.Learnable || covered(denial) || denyRules.Matches(denial) {
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
			contextID: denial.ContextID, projectID: denial.ProjectID, host: denial.Host, port: denial.Port,
			method: denial.Method, path: denial.Path, protocol: denial.EffectiveProtocol(),
			graphqlOperationType: denial.GraphQLOperationType, graphqlRootField: denial.GraphQLRootField,
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
	PolicyDirectory string
	ActiveRevision  string
}

func (r PolicyActivationReceipt) Validate() error {
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("policy activation directory is invalid")
	}
	if !policyRevisionPattern.MatchString(r.ActiveRevision) {
		return fmt.Errorf("policy activation revision is invalid")
	}
	return nil
}

// PolicyLearningChange is a confirmed exact approval result.
type PolicyLearningChange struct {
	Task            string            `json:"task"`
	PolicyDirectory string            `json:"policy"`
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
	if !filepath.IsAbs(c.PolicyDirectory) || filepath.Clean(c.PolicyDirectory) != c.PolicyDirectory {
		return fmt.Errorf("policy learning directory is invalid")
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
	Task            string         `json:"task"`
	PolicyDirectory string         `json:"policy"`
	TargetID        string         `json:"target_id"`
	Rule            PolicyDenyRule `json:"rule"`
	SourceRuleCount int            `json:"source_rule_count"`
	Applied         bool           `json:"applied"`
}

const MaxPolicyReviewDecisions = 128

// PolicyReviewDecision is one explicit exact choice retained from a validated
// review detail screen. CandidateID remains opaque and unchanged.
type PolicyReviewDecision struct {
	CandidateID string `json:"candidate_id"`
	Decision    string `json:"decision"`
}

// PolicyReviewDecisionSet is the bounded typed payload of one final human
// Apply. It is content of the command-owned target, not a collection of public
// mutation targets.
type PolicyReviewDecisionSet struct {
	Decisions []PolicyReviewDecision `json:"decisions"`
}

// PolicyReviewAppliedDecision is one exact secret-free decision repeated in
// the confirmed receipt. Its fields are copied from the freshly revalidated
// candidate; presentation must not reconstruct authority from labels or order.
type PolicyReviewAppliedDecision struct {
	PolicyProtocolIdentity
	CandidateID string `json:"candidate_id"`
	Decision    string `json:"decision"`
	ContextID   string `json:"context_id"`
	ContextName string `json:"context"`
	ProjectID   string `json:"project_id"`
	ProjectRoot string `json:"project_root"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Method      string `json:"method"`
	Path        string `json:"path"`
}

// NewPolicyReviewAppliedDecision copies only the candidate dimensions that
// define the exact policy effect. Observation prose, credentials, headers,
// queries, and bodies are intentionally absent from the receipt type.
func NewPolicyReviewAppliedDecision(candidate PolicyCandidate, decision string) (PolicyReviewAppliedDecision, error) {
	if err := candidate.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	receipt := PolicyReviewAppliedDecision{
		PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		CandidateID:            candidate.ID, Decision: decision,
		ContextID: candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot,
		Host: candidate.Host, Port: candidate.Port, Method: candidate.Method, Path: candidate.Path,
	}
	receipt.Protocol = candidate.EffectiveProtocol()
	if err := receipt.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	return receipt, nil
}

func (d PolicyReviewAppliedDecision) Validate() error {
	if err := d.PolicyProtocolIdentity.Validate(); err != nil {
		return fmt.Errorf("policy review receipt protocol identity: %w", err)
	}
	if err := ValidatePolicyCandidateID(d.CandidateID); err != nil {
		return err
	}
	if d.Decision != PolicyDecisionAllow && d.Decision != PolicyDecisionDeny {
		return fmt.Errorf("policy review receipt decision is invalid")
	}
	if err := validatePolicyScope(d.ContextID, d.ContextName, d.ProjectRoot); err != nil {
		return fmt.Errorf("policy review receipt scope: %w", err)
	}
	if err := ValidateProjectID(d.ProjectID); err != nil {
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
	if err := validatePolicyPath(d.Path); err != nil {
		return fmt.Errorf("policy review receipt path is invalid")
	}
	return nil
}

func (s PolicyReviewDecisionSet) Validate() error {
	if len(s.Decisions) == 0 || len(s.Decisions) > MaxPolicyReviewDecisions {
		return fmt.Errorf("policy review decision count must be between 1 and %d", MaxPolicyReviewDecisions)
	}
	seen := make(map[string]struct{}, len(s.Decisions))
	for _, decision := range s.Decisions {
		if err := ValidatePolicyCandidateID(decision.CandidateID); err != nil {
			return err
		}
		if decision.Decision != PolicyDecisionAllow && decision.Decision != PolicyDecisionDeny {
			return fmt.Errorf("policy review decision is invalid")
		}
		if _, duplicate := seen[decision.CandidateID]; duplicate {
			return fmt.Errorf("policy review decision set contains a duplicate candidate")
		}
		seen[decision.CandidateID] = struct{}{}
	}
	return nil
}

// PolicyReviewChange is emitted only after the complete reviewed set is active.
type PolicyReviewChange struct {
	Task            string                        `json:"task"`
	PolicyDirectory string                        `json:"policy"`
	AllowCount      int                           `json:"allow_count"`
	DenyCount       int                           `json:"deny_count"`
	Applied         bool                          `json:"applied"`
	ActiveRevision  string                        `json:"active_revision"`
	Decisions       []PolicyReviewAppliedDecision `json:"decisions"`
}

func (c PolicyReviewChange) Validate() error {
	if c.Task != TaskPolicyReviewApply || c.AllowCount < 0 || c.DenyCount < 0 ||
		c.AllowCount+c.DenyCount == 0 || c.AllowCount+c.DenyCount > MaxPolicyReviewDecisions || !c.Applied {
		return fmt.Errorf("policy review result is inconsistent")
	}
	if !filepath.IsAbs(c.PolicyDirectory) || filepath.Clean(c.PolicyDirectory) != c.PolicyDirectory {
		return fmt.Errorf("policy review result directory is invalid")
	}
	if !policyRevisionPattern.MatchString(c.ActiveRevision) {
		return fmt.Errorf("policy review active revision is invalid")
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
		if _, duplicate := seen[decision.CandidateID]; duplicate {
			return fmt.Errorf("policy review receipt contains a duplicate candidate")
		}
		seen[decision.CandidateID] = struct{}{}
		if decision.Decision == PolicyDecisionAllow {
			allowCount++
		} else {
			denyCount++
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
	if !filepath.IsAbs(c.PolicyDirectory) || filepath.Clean(c.PolicyDirectory) != c.PolicyDirectory {
		return fmt.Errorf("policy deny directory is invalid")
	}
	if err := c.Rule.Validate(); err != nil {
		return err
	}
	return nil
}
