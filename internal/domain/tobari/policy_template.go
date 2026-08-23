package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const PolicyPathTemplatePlaceholder = "{id}"

// PolicyPathTemplateProposal is one typed, inert review proposal derived from
// at least two distinct compatible HTTP paths. Its ID binds authority, not the
// growing evidence set, so another compatible example cannot change scope.
type PolicyPathTemplateProposal struct {
	PolicyProtocolIdentity
	ID                    string            `json:"id"`
	WorkspaceManifestID   string            `json:"workspace_manifest_id"`
	WorkspaceManifestName string            `json:"workspace_manifest"`
	ProjectID             string            `json:"workspace_id"`
	ProjectRoot           string            `json:"project_root"`
	Host                  string            `json:"host"`
	Port                  int               `json:"port"`
	Method                string            `json:"method"`
	Path                  string            `json:"path"`
	Segments              []string          `json:"segments"`
	Examples              []string          `json:"examples"`
	SourceCandidates      []string          `json:"source_candidates"`
	SourceRuleIDs         []string          `json:"source_rule_ids"`
	PendingCandidates     []PolicyCandidate `json:"pending_candidates"`
}

// PolicyReviewItem is the domain-owned unit shown by Permission Inbox. Exact
// items carry one pending candidate; template items carry one proposal.
type PolicyReviewItem struct {
	ID        string                      `json:"id"`
	Match     string                      `json:"match"`
	Candidate *PolicyCandidate            `json:"candidate,omitempty"`
	Template  *PolicyPathTemplateProposal `json:"template,omitempty"`
}

func ValidatePolicyReviewItemID(id string) error {
	if policyCandidateIDPattern.MatchString(id) || policyTemplateIDPattern.MatchString(id) {
		return nil
	}
	return fmt.Errorf("policy review item ID is invalid")
}

func (i PolicyReviewItem) Validate() error {
	if err := ValidatePolicyReviewItemID(i.ID); err != nil {
		return err
	}
	if !validPolicyMatch(i.Match) {
		return fmt.Errorf("policy review item match is invalid")
	}
	switch i.Match {
	case PolicyMatchExact:
		if i.Candidate == nil || i.Template != nil || i.ID != i.Candidate.ID {
			return fmt.Errorf("exact policy review item is invalid")
		}
		return i.Candidate.Validate()
	case PolicyMatchPathTemplate:
		if i.Template == nil || i.Candidate != nil || i.ID != i.Template.ID {
			return fmt.Errorf("template policy review item is invalid")
		}
		return i.Template.Validate()
	default:
		return fmt.Errorf("policy review item match semantics are not implemented")
	}
}

func (p PolicyPathTemplateProposal) Validate() error {
	if !policyTemplateIDPattern.MatchString(p.ID) {
		return fmt.Errorf("policy path-template proposal ID is invalid")
	}
	if err := p.PolicyProtocolIdentity.Validate(); err != nil || p.EffectiveProtocol() != PolicyProtocolHTTP {
		return fmt.Errorf("policy path-template proposal protocol is invalid")
	}
	if err := validatePolicyScope(p.WorkspaceManifestID, p.WorkspaceManifestName, p.ProjectRoot); err != nil {
		return fmt.Errorf("policy path-template proposal scope: %w", err)
	}
	if err := ValidateWorkspaceID(p.ProjectID); err != nil || !validNormalizedPolicyHost(p.Host) || p.Port < 1 || p.Port > 65535 || !httpMethodPattern.MatchString(p.Method) {
		return fmt.Errorf("policy path-template proposal request identity is invalid")
	}
	if err := validatePathTemplate(p.Path, p.Segments); err != nil {
		return err
	}
	if len(p.Examples) < 2 || len(p.SourceCandidates) < 2 || len(p.PendingCandidates) < 1 {
		return fmt.Errorf("policy path-template proposal has insufficient evidence")
	}
	if err := validateSortedUniquePaths(p.Examples); err != nil {
		return fmt.Errorf("policy path-template proposal examples: %w", err)
	}
	if err := validateSortedUniqueCandidateIDs(p.SourceCandidates); err != nil {
		return fmt.Errorf("policy path-template proposal sources: %w", err)
	}
	if err := validateSortedUniqueRuleIDs(p.SourceRuleIDs); err != nil {
		return fmt.Errorf("policy path-template proposal source rules: %w", err)
	}
	seenPending := map[string]struct{}{}
	for _, candidate := range p.PendingCandidates {
		if err := candidate.Validate(); err != nil || !p.matchesCandidate(candidate) || !pathTemplateMatches(p.Segments, candidate.Path) {
			return fmt.Errorf("policy path-template proposal pending candidate is invalid")
		}
		if _, duplicate := seenPending[candidate.ID]; duplicate {
			return fmt.Errorf("policy path-template proposal pending candidates are duplicated")
		}
		seenPending[candidate.ID] = struct{}{}
	}
	for _, example := range p.Examples {
		if !pathTemplateMatches(p.Segments, example) {
			return fmt.Errorf("policy path-template proposal example is outside its template")
		}
	}
	if p.ID != pathTemplateProposalID(p) {
		return fmt.Errorf("policy path-template proposal ID does not bind its authority")
	}
	return nil
}

func validateSortedUniqueRuleIDs(values []string) error {
	previous := ""
	for index, value := range values {
		if !learnedRuleIDPattern.MatchString(value) {
			return fmt.Errorf("learned rule ID is invalid")
		}
		if index > 0 && value <= previous {
			return fmt.Errorf("learned rule IDs must be unique and sorted")
		}
		previous = value
	}
	return nil
}

func (p PolicyPathTemplateProposal) matchesCandidate(candidate PolicyCandidate) bool {
	return p.WorkspaceManifestID == candidate.WorkspaceManifestID && p.WorkspaceManifestName == candidate.WorkspaceManifestName &&
		p.ProjectID == candidate.ProjectID && p.ProjectRoot == candidate.ProjectRoot && p.Host == candidate.Host &&
		p.Port == candidate.Port && p.Method == candidate.Method && p.PolicyProtocolIdentity.matches(candidate.PolicyProtocolIdentity)
}

func rawPathSegments(path string) ([]string, error) {
	if err := validatePolicyPath(path); err != nil {
		return nil, err
	}
	if path == "/" || strings.ContainsAny(path, "{}\\") {
		return nil, fmt.Errorf("path is not safe template evidence")
	}
	segments := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil, fmt.Errorf("path contains an empty or dot segment")
		}
		if strings.Contains(segment, "%") {
			return nil, fmt.Errorf("path contains an unsafe encoded segment")
		}
	}
	return segments, nil
}

func inferPathTemplate(left, right string) (string, []string, bool) {
	leftSegments, leftErr := rawPathSegments(left)
	rightSegments, rightErr := rawPathSegments(right)
	if leftErr != nil || rightErr != nil || len(leftSegments) != len(rightSegments) || len(leftSegments) < 2 {
		return "", nil, false
	}
	segments := append([]string{}, leftSegments...)
	changed := -1
	for index := range segments {
		if leftSegments[index] == rightSegments[index] {
			continue
		}
		if changed >= 0 {
			return "", nil, false
		}
		changed = index
		segments[index] = PolicyPathTemplatePlaceholder
	}
	if changed < 0 {
		return "", nil, false
	}
	return "/" + strings.Join(segments, "/"), segments, true
}

func validatePathTemplate(path string, segments []string) error {
	if len(segments) < 2 || path != "/"+strings.Join(segments, "/") {
		return fmt.Errorf("path template shape is invalid")
	}
	placeholders := 0
	for _, segment := range segments {
		if segment == PolicyPathTemplatePlaceholder {
			placeholders++
			continue
		}
		if _, err := rawPathSegments("/fixed/" + segment); err != nil {
			return fmt.Errorf("path template literal is unsafe")
		}
	}
	if placeholders != 1 {
		return fmt.Errorf("path template must contain exactly one placeholder")
	}
	return nil
}

func pathTemplateMatches(templateSegments []string, path string) bool {
	if err := validatePathTemplate("/"+strings.Join(templateSegments, "/"), templateSegments); err != nil {
		return false
	}
	segments, err := rawPathSegments(path)
	if err != nil || len(segments) != len(templateSegments) {
		return false
	}
	for index := range segments {
		if templateSegments[index] != PolicyPathTemplatePlaceholder && templateSegments[index] != segments[index] {
			return false
		}
	}
	return true
}

func pathTemplateProposalID(proposal PolicyPathTemplateProposal) string {
	material := appendPolicyProtocolIdentity([]string{
		"tobari-policy-path-template-v1", proposal.WorkspaceManifestID, proposal.ProjectID, proposal.Host,
		strconv.Itoa(proposal.Port), proposal.Method, proposal.Path,
	}, proposal.PolicyProtocolIdentity)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return "ptp_" + hex.EncodeToString(sum[:16])
}

type pathTemplateEvidence struct {
	path         string
	candidate    *PolicyCandidate
	sourceIDs    []string
	sourceRuleID string
	baseKey      string
	pendingIndex int
	identity     PolicyPathTemplateProposal
}

// PolicyReviewItems derives the typed Permission Inbox projection from pending
// exact candidates and current exact allows. Current templates are accepted as
// coverage but never used as evidence for another inferred template.
func PolicyReviewItems(candidates []PolicyCandidate, rules []LearnedPolicyRule) ([]PolicyReviewItem, error) {
	if candidates == nil {
		return nil, fmt.Errorf("policy review candidates are unknown")
	}
	if err := ValidateLearnedPolicyRules(rules); err != nil {
		return nil, err
	}
	evidenceByBase := map[string][]pathTemplateEvidence{}
	for _, rule := range rules {
		if rule.Match != PolicyMatchExact || rule.EffectiveProtocol() != PolicyProtocolHTTP {
			continue
		}
		identity := proposalIdentityFromRule(rule)
		base := templateBaseKey(identity)
		evidenceByBase[base] = append(evidenceByBase[base], pathTemplateEvidence{
			path: rule.Path, sourceIDs: append([]string{}, rule.SourceCandidates...), sourceRuleID: rule.ID,
			baseKey: base, pendingIndex: -1, identity: identity,
		})
	}
	for index := range candidates {
		candidate := candidates[index]
		if err := candidate.Validate(); err != nil {
			return nil, err
		}
		if candidate.EffectiveProtocol() != PolicyProtocolHTTP || candidate.EffectiveDestinationKind() == PolicyDestinationHostLoopback {
			continue
		}
		identity := proposalIdentityFromCandidate(candidate)
		base := templateBaseKey(identity)
		evidenceByBase[base] = append(evidenceByBase[base], pathTemplateEvidence{
			path: candidate.Path, candidate: &candidates[index], sourceIDs: []string{candidate.ID},
			baseKey: base, pendingIndex: index, identity: identity,
		})
	}

	type proposalGroup struct {
		template string
		segments []string
		evidence map[string]pathTemplateEvidence
	}
	groups := map[string]*proposalGroup{}
	for base, evidence := range evidenceByBase {
		for left := 0; left < len(evidence); left++ {
			for right := left + 1; right < len(evidence); right++ {
				if evidence[left].identity.WorkspaceManifestName != evidence[right].identity.WorkspaceManifestName ||
					evidence[left].identity.ProjectRoot != evidence[right].identity.ProjectRoot {
					return nil, fmt.Errorf("policy template evidence has inconsistent stable scope facts")
				}
				template, segments, ok := inferPathTemplate(evidence[left].path, evidence[right].path)
				if !ok {
					continue
				}
				key := base + "\x00" + template
				group := groups[key]
				if group == nil {
					group = &proposalGroup{template: template, segments: segments, evidence: map[string]pathTemplateEvidence{}}
					groups[key] = group
				}
				group.evidence[evidence[left].path] = evidence[left]
				group.evidence[evidence[right].path] = evidence[right]
			}
		}
	}

	membership := map[string]int{}
	eligible := make([]*proposalGroup, 0, len(groups))
	for _, group := range groups {
		pending := false
		for _, item := range group.evidence {
			pending = pending || item.candidate != nil
		}
		if len(group.evidence) < 2 || !pending {
			continue
		}
		eligible = append(eligible, group)
		for _, item := range group.evidence {
			membership[item.baseKey+"\x00"+item.path]++
		}
	}

	type orderedReviewItem struct {
		index int
		item  PolicyReviewItem
	}
	ordered := make([]orderedReviewItem, 0, len(candidates))
	consumed := map[string]struct{}{}
	for _, group := range eligible {
		ambiguous := false
		for _, item := range group.evidence {
			if membership[item.baseKey+"\x00"+item.path] > 1 {
				ambiguous = true
			}
		}
		if ambiguous {
			continue
		}
		var proposal PolicyPathTemplateProposal
		proposal.Path, proposal.Segments = group.template, append([]string{}, group.segments...)
		firstPending := len(candidates)
		for _, item := range group.evidence {
			proposal = mergeProposalEvidence(proposal, item)
			if item.candidate != nil {
				consumed[item.candidate.ID] = struct{}{}
				if item.pendingIndex < firstPending {
					firstPending = item.pendingIndex
				}
			}
		}
		sort.Strings(proposal.Examples)
		sort.Strings(proposal.SourceCandidates)
		proposal.SourceCandidates = uniqueSortedStrings(proposal.SourceCandidates)
		sort.Strings(proposal.SourceRuleIDs)
		proposal.SourceRuleIDs = uniqueSortedStrings(proposal.SourceRuleIDs)
		sort.Slice(proposal.PendingCandidates, func(i, j int) bool {
			if proposal.PendingCandidates[i].ObservedAt == proposal.PendingCandidates[j].ObservedAt {
				return proposal.PendingCandidates[i].ID < proposal.PendingCandidates[j].ID
			}
			return proposal.PendingCandidates[i].ObservedAt < proposal.PendingCandidates[j].ObservedAt
		})
		proposal.ID = pathTemplateProposalID(proposal)
		item := PolicyReviewItem{ID: proposal.ID, Match: PolicyMatchPathTemplate, Template: &proposal}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		ordered = append(ordered, orderedReviewItem{index: firstPending, item: item})
	}
	for index := range candidates {
		if _, found := consumed[candidates[index].ID]; found {
			continue
		}
		candidate := candidates[index]
		item := PolicyReviewItem{ID: candidate.ID, Match: PolicyMatchExact, Candidate: &candidate}
		ordered = append(ordered, orderedReviewItem{index: index, item: item})
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	result := make([]PolicyReviewItem, len(ordered))
	for index, item := range ordered {
		result[index] = item.item
	}
	return result, nil
}

func proposalIdentityFromCandidate(candidate PolicyCandidate) PolicyPathTemplateProposal {
	return PolicyPathTemplateProposal{PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		WorkspaceManifestID: candidate.WorkspaceManifestID, WorkspaceManifestName: candidate.WorkspaceManifestName, ProjectID: candidate.ProjectID,
		ProjectRoot: candidate.ProjectRoot, Host: candidate.Host, Port: candidate.Port, Method: candidate.Method}
}

func proposalIdentityFromRule(rule LearnedPolicyRule) PolicyPathTemplateProposal {
	return PolicyPathTemplateProposal{PolicyProtocolIdentity: rule.PolicyProtocolIdentity,
		WorkspaceManifestID: rule.WorkspaceManifestID, WorkspaceManifestName: rule.WorkspaceManifestName, ProjectID: rule.ProjectID,
		ProjectRoot: rule.ProjectRoot, Host: rule.Host, Port: rule.Port, Method: rule.Method}
}

func templateBaseKey(p PolicyPathTemplateProposal) string {
	parts := appendPolicyProtocolIdentity([]string{p.WorkspaceManifestID, p.ProjectID, p.Host, strconv.Itoa(p.Port), p.Method}, p.PolicyProtocolIdentity)
	return strings.Join(parts, "\x00")
}

func mergeProposalEvidence(proposal PolicyPathTemplateProposal, item pathTemplateEvidence) PolicyPathTemplateProposal {
	if proposal.WorkspaceManifestID == "" {
		identity := item.identity
		proposal.PolicyProtocolIdentity = identity.PolicyProtocolIdentity
		proposal.WorkspaceManifestID, proposal.WorkspaceManifestName = identity.WorkspaceManifestID, identity.WorkspaceManifestName
		proposal.ProjectID, proposal.ProjectRoot = identity.ProjectID, identity.ProjectRoot
		proposal.Host, proposal.Port, proposal.Method = identity.Host, identity.Port, identity.Method
	}
	proposal.Examples = append(proposal.Examples, item.path)
	proposal.SourceCandidates = append(proposal.SourceCandidates, item.sourceIDs...)
	if item.sourceRuleID != "" {
		proposal.SourceRuleIDs = append(proposal.SourceRuleIDs, item.sourceRuleID)
	}
	if item.candidate != nil {
		proposal.PendingCandidates = append(proposal.PendingCandidates, *item.candidate)
	}
	return proposal
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := values[:1]
	for _, value := range values[1:] {
		if value != result[len(result)-1] {
			result = append(result, value)
		}
	}
	return result
}

// NewPathTemplateLearnedPolicyRule creates authority only from a validated
// inert proposal chosen on its detail screen.
func NewPathTemplateLearnedPolicyRule(proposal PolicyPathTemplateProposal) (LearnedPolicyRule, error) {
	if err := proposal.Validate(); err != nil {
		return LearnedPolicyRule{}, err
	}
	rule := LearnedPolicyRule{PolicyProtocolIdentity: proposal.PolicyProtocolIdentity,
		Match: PolicyMatchPathTemplate, WorkspaceManifestID: proposal.WorkspaceManifestID, WorkspaceManifestName: proposal.WorkspaceManifestName,
		ProjectID: proposal.ProjectID, ProjectRoot: proposal.ProjectRoot, Host: proposal.Host, Port: proposal.Port,
		Method: proposal.Method, Path: proposal.Path, Segments: append([]string{}, proposal.Segments...),
		Examples: append([]string{}, proposal.Examples...), SourceCandidates: append([]string{}, proposal.SourceCandidates...)}
	rule.ID = learnedRuleIDWithIdentity(rule.Match, rule.WorkspaceManifestID, rule.ProjectID, rule.Host, rule.Port, rule.Method, rule.Path, rule.Examples, rule.SourceCandidates, rule.PolicyProtocolIdentity)
	if err := rule.Validate(); err != nil {
		return LearnedPolicyRule{}, err
	}
	return rule, nil
}

func NewPolicyReviewAppliedAllow(reviewItemID string, rule LearnedPolicyRule) (PolicyReviewAppliedDecision, error) {
	if err := rule.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	receipt := PolicyReviewAppliedDecision{PolicyProtocolIdentity: rule.PolicyProtocolIdentity,
		RuleID: rule.ID, ReviewItemID: reviewItemID, Decision: PolicyDecisionAllow, Match: rule.Match,
		WorkspaceManifestID: rule.WorkspaceManifestID, WorkspaceManifestName: rule.WorkspaceManifestName, ProjectID: rule.ProjectID, ProjectRoot: rule.ProjectRoot,
		Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path,
		SourceCandidates: append([]string{}, rule.SourceCandidates...)}
	if err := receipt.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	return receipt, nil
}

func NewPolicyReviewAppliedDeny(reviewItemID string, rule PolicyDenyRule) (PolicyReviewAppliedDecision, error) {
	if err := rule.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	receipt := PolicyReviewAppliedDecision{PolicyProtocolIdentity: rule.PolicyProtocolIdentity,
		RuleID: rule.ID, ReviewItemID: reviewItemID, Decision: PolicyDecisionDeny, Match: PolicyMatchExact,
		WorkspaceManifestID: rule.WorkspaceManifestID, WorkspaceManifestName: rule.WorkspaceManifestName, ProjectID: rule.ProjectID, ProjectRoot: rule.ProjectRoot,
		Host: rule.Host, Port: rule.Port, Method: rule.Method, Path: rule.Path,
		SourceCandidates: append([]string{}, rule.SourceCandidates...)}
	if err := receipt.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	return receipt, nil
}

func NewPolicyReviewAppliedAttachment(candidate PolicyCandidate, grant AttachmentGrant) (PolicyReviewAppliedDecision, error) {
	if err := candidate.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	if err := grant.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	if candidate.ID != grant.SourceCandidate {
		return PolicyReviewAppliedDecision{}, fmt.Errorf("attachment grant source does not match review item")
	}
	receipt := PolicyReviewAppliedDecision{PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
		RuleID: grant.ID, ReviewItemID: candidate.ID, Decision: grant.Decision, Match: PolicyMatchExact,
		WorkspaceManifestID: candidate.WorkspaceManifestID, WorkspaceManifestName: candidate.WorkspaceManifestName, ProjectID: candidate.ProjectID, ProjectRoot: candidate.ProjectRoot,
		Host: candidate.Host, Port: candidate.Port, Method: candidate.Method, Path: candidate.Path,
		SourceCandidates: []string{candidate.ID}, DestinationKind: PolicyDestinationHostLoopback,
		AuthorityLifetime: AuthorityLifetimeAttachment, AttachmentEpochID: candidate.AttachmentEpochID}
	if err := receipt.Validate(); err != nil {
		return PolicyReviewAppliedDecision{}, err
	}
	return receipt, nil
}
