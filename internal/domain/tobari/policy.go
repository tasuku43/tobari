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
	requestIDPattern          = regexp.MustCompile(`^[0-9a-f]{32}$`)
	policyCandidateIDPattern  = regexp.MustCompile(`^pcy_[0-9a-f]{32}$`)
	policyCompactionIDPattern = regexp.MustCompile(`^pcx_[0-9a-f]{32}$`)
	policyDenyRuleIDPattern   = regexp.MustCompile(`^pdr_[0-9a-f]{32}$`)
	learnedRuleIDPattern      = regexp.MustCompile(`^plr_[0-9a-f]{32}$`)
	httpMethodPattern         = regexp.MustCompile(`^[A-Z][A-Z0-9!#$%&'*+.^_` + "`" + `|~-]{0,31}$`)
)

const (
	PolicyMatchExact    = "exact"
	PolicyMatchPrefix   = "prefix"
	PolicyDecisionAllow = "allow"
	PolicyDecisionDeny  = "deny"
)

// PolicyDenial is one validated, secret-free Gateway audit decision.
type PolicyDenial struct {
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
	ID                string  `json:"id"`
	ObservedAt        string  `json:"observed_at"`
	ObservationCount  int     `json:"observation_count,omitempty"`
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
	sum := sha256.Sum256([]byte(strings.Join(
		[]string{
			"tobari-policy-candidate-v2", denial.ContextID, denial.ProjectID, denial.Host, strconv.Itoa(denial.Port), denial.Method, denial.Path,
		},
		"\x00",
	)))
	return PolicyCandidate{
		ID: "pcy_" + hex.EncodeToString(sum[:16]), ObservedAt: denial.Timestamp, ObservationCount: 1,
		ContextID: denial.ContextID, ContextName: denial.ContextName,
		ProjectID: denial.ProjectID, ProjectRoot: denial.ProjectRoot,
		Host: denial.Host, Port: denial.Port, Method: denial.Method, Path: denial.Path,
		Reason: denial.Reason, StatusCode: denial.StatusCode,
		CredentialProfile: cloneStringPointer(denial.CredentialProfile),
	}, nil
}

// EffectiveObservationCount keeps candidates decoded from the additive legacy
// shape meaningful. Newly derived candidates always carry an explicit count.
func (c PolicyCandidate) EffectiveObservationCount() int {
	if c.ObservationCount == 0 {
		return 1
	}
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
	if err := ValidatePolicyCandidateID(c.ID); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, c.ObservedAt); err != nil {
		return fmt.Errorf("policy candidate timestamp is invalid")
	}
	if c.ObservationCount < 0 {
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
	if r.Task != TaskPolicyCandidates && r.Task != TaskPolicyTail && r.Task != TaskPolicyReview {
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
	material := strings.Join(
		[]string{
			"tobari-policy-deny-v2", contextID, projectID, host, strconv.Itoa(port), method, path,
			strings.Join(sourceCandidates, "\x1f"),
		},
		"\x00",
	)
	sum := sha256.Sum256([]byte(material))
	return "pdr_" + hex.EncodeToString(sum[:16])
}

// NewExactPolicyDenyRule binds a rejection to one exact candidate.
func NewExactPolicyDenyRule(candidate PolicyCandidate) (PolicyDenyRule, error) {
	if err := candidate.Validate(); err != nil {
		return PolicyDenyRule{}, err
	}
	rule := PolicyDenyRule{
		ContextID: candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port,
		Method: candidate.Method, Path: candidate.Path,
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = policyDenyRuleID(
		rule.ContextID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.SourceCandidates,
	)
	return rule, nil
}

func (r PolicyDenyRule) Validate() error {
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
	if r.ID != policyDenyRuleID(r.ContextID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.SourceCandidates) {
		return fmt.Errorf("policy deny rule ID does not bind its content")
	}
	return nil
}

func (r PolicyDenyRule) Matches(contextID, projectID, host string, port int, method, path string) bool {
	return r.ContextID == contextID && r.ProjectID == projectID && r.Host == host && r.Port == port &&
		r.Method == method && r.Path == path
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
		if rule.Matches(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path) {
			return true
		}
	}
	return false
}

// LearnedPolicyRule is the only data.json member owned by policy-learning
// commands. Examples are retained so compaction remains testable.
type LearnedPolicyRule struct {
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
	material := strings.Join(
		[]string{
			"tobari-learned-rule-v2", match, contextID, projectID, host, strconv.Itoa(port), method, path,
			strings.Join(examples, "\x1f"), strings.Join(sourceCandidates, "\x1f"),
		},
		"\x00",
	)
	sum := sha256.Sum256([]byte(material))
	return "plr_" + hex.EncodeToString(sum[:16])
}

func NewExactLearnedPolicyRule(candidate PolicyCandidate) (LearnedPolicyRule, error) {
	if err := candidate.Validate(); err != nil {
		return LearnedPolicyRule{}, err
	}
	rule := LearnedPolicyRule{
		Match: PolicyMatchExact, ContextID: candidate.ContextID, ContextName: candidate.ContextName,
		ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port, Method: candidate.Method,
		Path: candidate.Path, Examples: []string{candidate.Path},
		SourceCandidates: []string{candidate.ID},
	}
	rule.ID = learnedRuleID(
		rule.Match, rule.ContextID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.Examples, rule.SourceCandidates,
	)
	return rule, nil
}

func (r LearnedPolicyRule) Validate() error {
	if !learnedRuleIDPattern.MatchString(r.ID) {
		return fmt.Errorf("learned rule ID is invalid")
	}
	if r.Match != PolicyMatchExact && r.Match != PolicyMatchPrefix {
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
	if r.Match == PolicyMatchPrefix && !strings.HasSuffix(r.Path, "/") {
		return fmt.Errorf("learned prefix rule must end at a path boundary")
	}
	if r.Examples == nil || r.SourceCandidates == nil {
		return fmt.Errorf("learned rule evidence is unknown")
	}
	minimum := 1
	if r.Match == PolicyMatchPrefix {
		minimum = 3
	}
	if len(r.Examples) < minimum || len(r.SourceCandidates) < minimum {
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
		if r.Match == PolicyMatchPrefix && !strings.HasPrefix(example, r.Path) {
			return fmt.Errorf("prefix learned rule example is outside its path")
		}
	}
	if r.ID != learnedRuleID(
		r.Match, r.ContextID, r.ProjectID, r.Host, r.Port, r.Method, r.Path, r.Examples, r.SourceCandidates,
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
		ID: rule.ID, Decision: PolicyDecisionAllow, Match: rule.Match,
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
		ID: rule.ID, Decision: PolicyDecisionDeny, Match: PolicyMatchExact,
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
			ID: r.ID, Match: r.Match, ContextID: r.ContextID, ContextName: r.ContextName,
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
		ID: r.ID, ContextID: r.ContextID, ContextName: r.ContextName,
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
	if r.ContextID != contextID || r.ProjectID != projectID || r.Host != host || r.Port != port || r.Method != method {
		return false
	}
	switch r.Match {
	case PolicyMatchExact:
		return r.Path == path
	case PolicyMatchPrefix:
		return strings.HasPrefix(path, r.Path)
	default:
		return false
	}
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
			if rule.Matches(denial.ContextID, denial.ProjectID, denial.Host, denial.Port, denial.Method, denial.Path) {
				return true
			}
		}
		return false
	}
	type effectKey struct {
		contextID string
		projectID string
		host      string
		port      int
		method    string
		path      string
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
			method: denial.Method, path: denial.Path,
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

// PolicyCompaction binds one current exact source-rule set to one path prefix.
type PolicyCompaction struct {
	ID            string   `json:"id"`
	ContextID     string   `json:"context_id"`
	ContextName   string   `json:"context"`
	ProjectID     string   `json:"project_id"`
	ProjectRoot   string   `json:"project_root"`
	Host          string   `json:"host"`
	Port          int      `json:"port"`
	Method        string   `json:"method"`
	PathPrefix    string   `json:"path_prefix"`
	SourceRuleIDs []string `json:"source_rule_ids"`
	Examples      []string `json:"examples"`
	OutsideCanary string   `json:"outside_canary"`
}

func ValidatePolicyCompactionID(id string) error {
	if !policyCompactionIDPattern.MatchString(id) {
		return fmt.Errorf("policy compaction ID is invalid")
	}
	return nil
}

func compactionID(contextID, projectID, host string, port int, method, prefix string, sourceRuleIDs []string) string {
	material := strings.Join(
		[]string{
			"tobari-policy-compaction-v2", contextID, projectID, host, strconv.Itoa(port), method, prefix,
			strings.Join(sourceRuleIDs, "\x1f"),
		},
		"\x00",
	)
	sum := sha256.Sum256([]byte(material))
	return "pcx_" + hex.EncodeToString(sum[:16])
}

func outsidePrefixCanary(prefix string) string {
	return strings.TrimSuffix(prefix, "/") + "-outside-tobari-canary"
}

func (c PolicyCompaction) Validate() error {
	if err := ValidatePolicyCompactionID(c.ID); err != nil {
		return err
	}
	if err := validatePolicyScope(c.ContextID, c.ContextName, c.ProjectRoot); err != nil {
		return fmt.Errorf("policy compaction scope: %w", err)
	}
	if err := ValidateProjectID(c.ProjectID); err != nil {
		return fmt.Errorf("policy compaction project ID is invalid")
	}
	if len(c.Host) == 0 || len(c.Host) > 253 || containsSpaceOrControl(c.Host) {
		return fmt.Errorf("policy compaction host is invalid")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("policy compaction port is invalid")
	}
	if !httpMethodPattern.MatchString(c.Method) {
		return fmt.Errorf("policy compaction method is invalid")
	}
	if !validCompactionPrefix(c.PathPrefix) {
		return fmt.Errorf("policy compaction prefix is unsafe")
	}
	if len(c.SourceRuleIDs) < 3 || len(c.Examples) < 3 {
		return fmt.Errorf("policy compaction has insufficient evidence")
	}
	previous := ""
	for index, id := range c.SourceRuleIDs {
		if !learnedRuleIDPattern.MatchString(id) || index > 0 && id <= previous {
			return fmt.Errorf("policy compaction source rules must be unique and sorted")
		}
		previous = id
	}
	if err := validateSortedUniquePaths(c.Examples); err != nil {
		return err
	}
	for _, example := range c.Examples {
		if !strings.HasPrefix(example, c.PathPrefix) {
			return fmt.Errorf("policy compaction example is outside its prefix")
		}
	}
	if c.OutsideCanary != outsidePrefixCanary(c.PathPrefix) {
		return fmt.Errorf("policy compaction boundary canary is invalid")
	}
	if c.ID != compactionID(c.ContextID, c.ProjectID, c.Host, c.Port, c.Method, c.PathPrefix, c.SourceRuleIDs) {
		return fmt.Errorf("policy compaction ID does not bind its source rules")
	}
	return nil
}

// PolicyCompactionReport is exhaustive for the current learned-rule file.
type PolicyCompactionReport struct {
	Task            string             `json:"task"`
	PolicyDirectory string             `json:"policy"`
	Items           []PolicyCompaction `json:"items"`
}

func (r PolicyCompactionReport) Validate() error {
	if r.Task != TaskPolicyCompactions {
		return fmt.Errorf("policy compaction report task identity is invalid")
	}
	if !filepath.IsAbs(r.PolicyDirectory) || filepath.Clean(r.PolicyDirectory) != r.PolicyDirectory {
		return fmt.Errorf("policy compaction policy directory is invalid")
	}
	if r.Items == nil {
		return fmt.Errorf("policy compaction collection is unknown")
	}
	seen := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if seen[item.ID] {
			return fmt.Errorf("policy compaction IDs must be unique")
		}
		seen[item.ID] = true
	}
	return nil
}

func exactRuleDirectory(path string) string {
	index := strings.LastIndex(path, "/")
	if index < 0 {
		return ""
	}
	return path[:index+1]
}

func validCompactionPrefix(prefix string) bool {
	if !safeCompactionRequestPath(prefix) || !strings.HasSuffix(prefix, "/") {
		return false
	}
	trimmed := strings.Trim(prefix, "/")
	return trimmed != "" && strings.Contains(trimmed, "/")
}

func safeCompactionRequestPath(path string) bool {
	if err := validatePolicyPath(path); err != nil ||
		strings.ContainsAny(path, `%\`) || strings.Contains(path, "//") {
		return false
	}
	for _, segment := range strings.Split(strings.Trim(path, "/"), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
}

func PolicyCompactions(rules []LearnedPolicyRule) ([]PolicyCompaction, error) {
	if err := ValidateLearnedPolicyRules(rules); err != nil {
		return nil, err
	}
	groups := make(map[string][]LearnedPolicyRule)
	for _, rule := range rules {
		if rule.Match != PolicyMatchExact || !safeCompactionRequestPath(rule.Path) {
			continue
		}
		prefix := exactRuleDirectory(rule.Path)
		if !validCompactionPrefix(prefix) {
			continue
		}
		key := rule.ContextID + "\x00" + rule.ProjectID + "\x00" + rule.Host + "\x00" + strconv.Itoa(rule.Port) + "\x00" + rule.Method + "\x00" + prefix
		groups[key] = append(groups[key], rule)
	}
	items := make([]PolicyCompaction, 0)
	for _, group := range groups {
		if len(group) < 3 {
			continue
		}
		sourceRuleIDs := make([]string, 0, len(group))
		examples := make([]string, 0, len(group))
		for _, rule := range group {
			sourceRuleIDs = append(sourceRuleIDs, rule.ID)
			examples = append(examples, rule.Path)
		}
		sort.Strings(sourceRuleIDs)
		sort.Strings(examples)
		examples = uniqueStrings(examples)
		if len(examples) < 3 {
			continue
		}
		prefix := exactRuleDirectory(group[0].Path)
		item := PolicyCompaction{
			ContextID: group[0].ContextID, ContextName: group[0].ContextName,
			ProjectID: group[0].ProjectID, ProjectRoot: group[0].ProjectRoot, Host: group[0].Host, Port: group[0].Port, Method: group[0].Method, PathPrefix: prefix,
			SourceRuleIDs: sourceRuleIDs, Examples: examples,
			OutsideCanary: outsidePrefixCanary(prefix),
		}
		item.ID = compactionID(item.ContextID, item.ProjectID, item.Host, item.Port, item.Method, item.PathPrefix, item.SourceRuleIDs)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

func CompactLearnedPolicyRules(
	rules []LearnedPolicyRule, id string,
) ([]LearnedPolicyRule, PolicyCompaction, LearnedPolicyRule, error) {
	if err := ValidatePolicyCompactionID(id); err != nil {
		return nil, PolicyCompaction{}, LearnedPolicyRule{}, err
	}
	compactions, err := PolicyCompactions(rules)
	if err != nil {
		return nil, PolicyCompaction{}, LearnedPolicyRule{}, err
	}
	var selected PolicyCompaction
	found := false
	for _, candidate := range compactions {
		if candidate.ID == id {
			selected, found = candidate, true
			break
		}
	}
	if !found {
		return nil, PolicyCompaction{}, LearnedPolicyRule{}, fmt.Errorf("policy compaction is not current")
	}
	sourceSet := make(map[string]bool, len(selected.SourceRuleIDs))
	for _, source := range selected.SourceRuleIDs {
		sourceSet[source] = true
	}
	sourceCandidates := make([]string, 0)
	remaining := make([]LearnedPolicyRule, 0, len(rules)-len(sourceSet)+1)
	for _, rule := range rules {
		if sourceSet[rule.ID] {
			sourceCandidates = append(sourceCandidates, rule.SourceCandidates...)
			continue
		}
		remaining = append(remaining, rule)
	}
	sort.Strings(sourceCandidates)
	sourceCandidates = uniqueStrings(sourceCandidates)
	prefixRule := LearnedPolicyRule{
		Match: PolicyMatchPrefix, ContextID: selected.ContextID, ContextName: selected.ContextName,
		ProjectID: selected.ProjectID, ProjectRoot: selected.ProjectRoot, Host: selected.Host, Port: selected.Port, Method: selected.Method,
		Path: selected.PathPrefix, Examples: append([]string{}, selected.Examples...),
		SourceCandidates: sourceCandidates,
	}
	prefixRule.ID = learnedRuleID(
		prefixRule.Match, prefixRule.ContextID, prefixRule.ProjectID, prefixRule.Host, prefixRule.Port, prefixRule.Method, prefixRule.Path,
		prefixRule.Examples, prefixRule.SourceCandidates,
	)
	if err := prefixRule.Validate(); err != nil {
		return nil, PolicyCompaction{}, LearnedPolicyRule{}, err
	}
	remaining = append(remaining, prefixRule)
	sort.Slice(remaining, func(i, j int) bool { return remaining[i].ID < remaining[j].ID })
	if err := ValidateLearnedPolicyRules(remaining); err != nil {
		return nil, PolicyCompaction{}, LearnedPolicyRule{}, err
	}
	return remaining, selected, prefixRule, nil
}

// PolicyLearningChange is a confirmed exact approval or compaction result.
type PolicyLearningChange struct {
	Task            string            `json:"task"`
	PolicyDirectory string            `json:"policy"`
	TargetID        string            `json:"target_id"`
	Rule            LearnedPolicyRule `json:"rule"`
	SourceRuleCount int               `json:"source_rule_count"`
	Applied         bool              `json:"applied"`
}

func (c PolicyLearningChange) Validate() error {
	switch c.Task {
	case TaskPolicyAllow:
		if err := ValidatePolicyCandidateID(c.TargetID); err != nil {
			return err
		}
		if c.Rule.Match != PolicyMatchExact || c.SourceRuleCount != 1 {
			return fmt.Errorf("policy allow result is inconsistent")
		}
	case TaskPolicyCompact:
		if err := ValidatePolicyCompactionID(c.TargetID); err != nil {
			return err
		}
		if c.Rule.Match != PolicyMatchPrefix || c.SourceRuleCount < 3 {
			return fmt.Errorf("policy compact result is inconsistent")
		}
	default:
		return fmt.Errorf("policy learning result task identity is invalid")
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
