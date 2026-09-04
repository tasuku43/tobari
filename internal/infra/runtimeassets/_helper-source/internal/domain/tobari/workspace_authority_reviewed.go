package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const PolicyMemoryReviewedSetSchemaVersion = 1

// PolicyMemoryReviewedDecision is one complete, immutable review item. Exact
// decisions consume one pending candidate. A path-template Allow may also
// replace exact Allow rules that were part of the reviewed proposal; their
// complete authority is retained so Apply never rediscovers proposal inputs.
type PolicyMemoryReviewedDecision struct {
	ReviewItemID   string                     `json:"review_item_id"`
	Candidates     []PolicyCandidateAuthority `json:"candidates"`
	SourceRules    []PolicyMemoryRule         `json:"source_rules"`
	Decision       PolicyMemoryDecision       `json:"decision"`
	Rule           PolicyMemoryRuleBody       `json:"rule"`
	ProposalDigest SemanticDigest             `json:"proposal_digest"`
	Digest         SemanticDigest             `json:"digest"`
}

func NewPolicyMemoryReviewedDecision(
	reviewItemID string,
	candidates []PolicyCandidateAuthority,
	sourceRules []PolicyMemoryRule,
	decision PolicyMemoryDecision,
	rule PolicyMemoryRuleBody,
) (PolicyMemoryReviewedDecision, error) {
	result := PolicyMemoryReviewedDecision{
		ReviewItemID: reviewItemID,
		Candidates:   clonePolicyCandidateAuthorities(candidates),
		SourceRules:  clonePolicyMemoryRules(sourceRules),
		Decision:     decision,
		Rule:         rule.Clone(),
	}
	var err error
	result.ProposalDigest, err = policyMemoryReviewedProposalDigest(result)
	if err != nil {
		return PolicyMemoryReviewedDecision{}, err
	}
	result.Digest, err = policyMemoryReviewedDecisionDigest(result)
	if err != nil {
		return PolicyMemoryReviewedDecision{}, err
	}
	return result, result.Validate()
}

func (d PolicyMemoryReviewedDecision) Validate() error {
	if len(d.Candidates) == 0 || len(d.Candidates) > MaxPolicyReviewDecisions || d.Decision.Validate() != nil {
		return fmt.Errorf("reviewed Policy Memory decision metadata is invalid")
	}
	contextID := d.Candidates[0].ContextID
	previousCandidate := ""
	for _, candidate := range d.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if candidate.Effect.Host == HostLoopbackHostname {
			return fmt.Errorf("attachment-local policy candidate cannot enter persistent Policy Memory")
		}
		if candidate.ContextID != contextID ||
			previousCandidate != "" && candidate.ID <= previousCandidate {
			return fmt.Errorf("reviewed Policy Memory candidates must share one owner and be unique and sorted")
		}
		previousCandidate = candidate.ID
	}
	previousRule := ""
	for _, rule := range d.SourceRules {
		if err := rule.Validate(contextID); err != nil {
			return err
		}
		if rule.Decision != PolicyMemoryAllow || rule.Body.Match != PolicyMatchExact ||
			previousRule != "" && rule.ID <= previousRule {
			return fmt.Errorf("reviewed Policy Memory source rules must be unique sorted exact Allows")
		}
		previousRule = rule.ID
	}
	if err := d.Rule.Validate(d.Decision); err != nil {
		return err
	}
	if d.Rule.Match == PolicyMatchExact {
		if len(d.Candidates) != 1 || len(d.SourceRules) != 0 ||
			!reflect.DeepEqual(d.Rule, d.Candidates[0].Effect.RuleBody(d.Candidates[0].ID)) || d.ReviewItemID != d.Candidates[0].ID {
			return fmt.Errorf("reviewed exact Policy Memory rule differs from its candidate")
		}
	} else if err := d.validatePathTemplate(); err != nil {
		return err
	}
	wantProposal, err := policyMemoryReviewedProposalDigest(d)
	if err != nil {
		return err
	}
	if d.ProposalDigest != wantProposal {
		return fmt.Errorf("reviewed Policy Memory proposal digest is inconsistent")
	}
	wantItemID, err := policyMemoryReviewedItemID(d)
	if err != nil || ValidatePolicyReviewItemID(d.ReviewItemID) != nil || d.ReviewItemID != wantItemID {
		return fmt.Errorf("reviewed Policy Memory item ID does not bind its unchanged proposal authority")
	}
	want, err := policyMemoryReviewedDecisionDigest(d)
	if err != nil {
		return err
	}
	if d.Digest != want {
		return fmt.Errorf("reviewed Policy Memory decision digest is inconsistent")
	}
	return nil
}

func (d PolicyMemoryReviewedDecision) validatePathTemplate() error {
	if d.Decision != PolicyMemoryAllow || d.Rule.Match != PolicyMatchPathTemplate {
		return fmt.Errorf("reviewed Policy Memory template decision must be Allow")
	}
	examples := make([]string, 0, len(d.Candidates)+len(d.SourceRules))
	sources := make([]string, 0, len(d.Candidates)+len(d.SourceRules))
	for _, candidate := range d.Candidates {
		if !reviewedTemplateIdentityMatchesEffect(d.Rule, candidate.Effect) || !pathTemplateMatches(d.Rule.Segments, candidate.Effect.Path) {
			return fmt.Errorf("reviewed Policy Memory template does not cover one pending candidate")
		}
		examples = append(examples, candidate.Effect.Path)
		sources = append(sources, candidate.ID)
	}
	for _, source := range d.SourceRules {
		if !reviewedTemplateIdentityMatchesBody(d.Rule, source.Body) || !pathTemplateMatches(d.Rule.Segments, source.Body.Path) {
			return fmt.Errorf("reviewed Policy Memory template does not cover one source rule")
		}
		examples = append(examples, source.Body.Path)
		sources = append(sources, source.Body.SourceCandidates...)
	}
	sort.Strings(examples)
	examples = uniqueSortedStrings(examples)
	sort.Strings(sources)
	sources = uniqueSortedStrings(sources)
	if len(examples) < 2 || !reflect.DeepEqual(examples, d.Rule.Examples) || !reflect.DeepEqual(sources, d.Rule.SourceCandidates) {
		return fmt.Errorf("reviewed Policy Memory template evidence is not mechanically derived from its exact sources")
	}
	return nil
}

func reviewedTemplateIdentityMatchesEffect(template PolicyMemoryRuleBody, effect PolicyCandidateEffect) bool {
	return template.PolicyProtocolIdentity == effect.PolicyProtocolIdentity &&
		template.Host == effect.Host && template.Port == effect.Port && template.Method == effect.Method
}

func reviewedTemplateIdentityMatchesBody(template, exact PolicyMemoryRuleBody) bool {
	return template.PolicyProtocolIdentity == exact.PolicyProtocolIdentity &&
		template.Host == exact.Host && template.Port == exact.Port && template.Method == exact.Method
}

func (d PolicyMemoryReviewedDecision) Clone() PolicyMemoryReviewedDecision {
	result := d
	result.Candidates = clonePolicyCandidateAuthorities(d.Candidates)
	result.SourceRules = clonePolicyMemoryRules(d.SourceRules)
	result.Rule = d.Rule.Clone()
	return result
}

func (d PolicyMemoryReviewedDecision) ContextID() ContextID { return d.Candidates[0].ContextID }

func policyMemoryReviewedProposalDigest(d PolicyMemoryReviewedDecision) (SemanticDigest, error) {
	if len(d.Candidates) == 0 {
		return "", fmt.Errorf("reviewed Policy Memory proposal has no candidate authority")
	}
	return semanticIdentity(struct {
		ContextID   ContextID
		Candidates  []PolicyCandidateAuthority
		SourceRules []PolicyMemoryRule
		Rule        PolicyMemoryRuleBody
	}{d.Candidates[0].ContextID, d.Candidates, d.SourceRules, d.Rule})
}

func policyMemoryReviewedDecisionDigest(d PolicyMemoryReviewedDecision) (SemanticDigest, error) {
	return semanticIdentity(struct {
		ReviewItemID   string
		ProposalDigest SemanticDigest
		Decision       PolicyMemoryDecision
	}{d.ReviewItemID, d.ProposalDigest, d.Decision})
}

func policyMemoryReviewedItemID(d PolicyMemoryReviewedDecision) (string, error) {
	if len(d.Candidates) == 0 {
		return "", fmt.Errorf("reviewed Policy Memory item has no candidate authority")
	}
	if len(d.Candidates) == 1 && len(d.SourceRules) == 0 && d.Rule.Match == PolicyMatchExact {
		return d.Candidates[0].ID, nil
	}
	return PolicyMemoryReviewedPathTemplateItemID(d.Candidates[0].ContextID, d.Rule)
}

// PolicyMemoryReviewedPathTemplateItemID derives the stable opaque identity
// displayed and selected by review UI. Complete candidate/source evidence is
// deliberately excluded and is bound separately by ProposalDigest.
func PolicyMemoryReviewedPathTemplateItemID(
	contextID ContextID,
	rule PolicyMemoryRuleBody,
) (string, error) {
	if contextID.Validate() != nil || rule.Match != PolicyMatchPathTemplate ||
		rule.PolicyProtocolIdentity.Validate() != nil || !validNormalizedPolicyHost(rule.Host) || rule.Port < 1 || rule.Port > 65535 ||
		!httpMethodPattern.MatchString(rule.Method) || validatePathTemplate(rule.Path, rule.Segments) != nil {
		return "", fmt.Errorf("reviewed Policy Memory path-template item scope is invalid")
	}
	material := appendPolicyProtocolIdentity([]string{
		"tobari-policy-path-template-v2", string(contextID),
		rule.Host, strconv.Itoa(rule.Port), rule.Method, rule.Path,
	}, rule.PolicyProtocolIdentity)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return "ptp_" + hex.EncodeToString(sum[:16]), nil
}

func legacyPolicyMemoryReviewedPathTemplateItemID(contextID ContextID, workspaceID WorkspaceID, rule PolicyMemoryRuleBody) (string, error) {
	if contextID.Validate() != nil || workspaceID.Validate() != nil || rule.Match != PolicyMatchPathTemplate ||
		rule.PolicyProtocolIdentity.Validate() != nil || !validNormalizedPolicyHost(rule.Host) || rule.Port < 1 || rule.Port > 65535 ||
		!httpMethodPattern.MatchString(rule.Method) || validatePathTemplate(rule.Path, rule.Segments) != nil {
		return "", fmt.Errorf("legacy reviewed Policy Memory path-template item scope is invalid")
	}
	material := appendPolicyProtocolIdentity([]string{
		"tobari-policy-path-template-v1", string(contextID), string(workspaceID),
		rule.Host, strconv.Itoa(rule.Port), rule.Method, rule.Path,
	}, rule.PolicyProtocolIdentity)
	sum := sha256.Sum256([]byte(strings.Join(material, "\x00")))
	return "ptp_" + hex.EncodeToString(sum[:16]), nil
}

func clonePolicyMemoryRules(values []PolicyMemoryRule) []PolicyMemoryRule {
	result := make([]PolicyMemoryRule, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

// PolicyMemoryReviewedDecisionSet is the complete non-empty content of the
// fixed policy-decision-set target. Decisions are canonical and Apply consumes
// this exact value rather than rereading an ambient Permission Inbox queue.
type PolicyMemoryReviewedDecisionSet struct {
	SchemaVersion      int                            `json:"schema_version"`
	TargetID           string                         `json:"target_id"`
	ObservedGeneration uint64                         `json:"observed_generation"`
	ObservedRevision   SemanticDigest                 `json:"observed_revision"`
	Decisions          []PolicyMemoryReviewedDecision `json:"decisions"`
	Digest             SemanticDigest                 `json:"digest"`
}

func NewPolicyMemoryReviewedDecisionSet(
	observed WorkspaceAuthorityCollection,
	decisions []PolicyMemoryReviewedDecision,
) (PolicyMemoryReviewedDecisionSet, error) {
	return newPolicyMemoryReviewedDecisionSet(observed, nil, decisions)
}

// NewPolicyMemoryReviewedDecisionSetWithObservations binds decisions to one
// coherent final collection plus the non-durable live candidates joined into
// its read-only Permission Inbox snapshot. Apply must still freshly reobserve
// these candidates before any authority change.
func NewPolicyMemoryReviewedDecisionSetWithObservations(
	observed WorkspaceAuthorityCollection,
	live []PolicyCandidateAuthority,
	decisions []PolicyMemoryReviewedDecision,
) (PolicyMemoryReviewedDecisionSet, error) {
	return newPolicyMemoryReviewedDecisionSet(observed, live, decisions)
}

func newPolicyMemoryReviewedDecisionSet(
	observed WorkspaceAuthorityCollection,
	live []PolicyCandidateAuthority,
	decisions []PolicyMemoryReviewedDecision,
) (PolicyMemoryReviewedDecisionSet, error) {
	if err := observed.Validate(); err != nil {
		return PolicyMemoryReviewedDecisionSet{}, err
	}
	result := PolicyMemoryReviewedDecisionSet{
		SchemaVersion: PolicyMemoryReviewedSetSchemaVersion, TargetID: PolicyDecisionSetID,
		ObservedGeneration: observed.Generation, ObservedRevision: observed.Revision,
		Decisions: make([]PolicyMemoryReviewedDecision, len(decisions)),
	}
	for index := range decisions {
		result.Decisions[index] = decisions[index].Clone()
	}
	sort.Slice(result.Decisions, func(i, j int) bool {
		return result.Decisions[i].ReviewItemID < result.Decisions[j].ReviewItemID
	})
	var err error
	result.Digest, err = policyMemoryReviewedDecisionSetDigest(result)
	if err != nil {
		return PolicyMemoryReviewedDecisionSet{}, err
	}
	if err := result.Validate(); err != nil {
		return PolicyMemoryReviewedDecisionSet{}, err
	}
	if _, err := validatePolicyMemoryReviewedSources(observed, result, live); err != nil {
		return PolicyMemoryReviewedDecisionSet{}, err
	}
	return result, nil
}

func (s PolicyMemoryReviewedDecisionSet) Validate() error {
	if s.SchemaVersion != PolicyMemoryReviewedSetSchemaVersion || s.TargetID != PolicyDecisionSetID ||
		s.ObservedGeneration == 0 || s.ObservedRevision.Validate() != nil ||
		len(s.Decisions) == 0 || len(s.Decisions) > MaxPolicyReviewDecisions {
		return fmt.Errorf("reviewed Policy Memory decision-set metadata is invalid")
	}
	seenCandidates := map[string]struct{}{}
	seenRules := map[string]struct{}{}
	seenResults := map[string]struct{}{}
	seenItems := map[string]struct{}{}
	previousItem := ""
	for _, decision := range s.Decisions {
		if err := decision.Validate(); err != nil {
			return err
		}
		if previousItem != "" && decision.ReviewItemID <= previousItem {
			return fmt.Errorf("reviewed Policy Memory decisions must use canonical ReviewItemID order")
		}
		if _, duplicate := seenItems[decision.ReviewItemID]; duplicate {
			return fmt.Errorf("reviewed Policy Memory decision set contains a duplicate review item")
		}
		seenItems[decision.ReviewItemID] = struct{}{}
		previousItem = decision.ReviewItemID
		for _, candidate := range decision.Candidates {
			if _, duplicate := seenCandidates[candidate.ID]; duplicate {
				return fmt.Errorf("reviewed Policy Memory decisions duplicate one pending candidate")
			}
			seenCandidates[candidate.ID] = struct{}{}
		}
		for _, rule := range decision.SourceRules {
			if _, duplicate := seenRules[rule.ID]; duplicate {
				return fmt.Errorf("reviewed Policy Memory decisions duplicate one source rule")
			}
			seenRules[rule.ID] = struct{}{}
		}
		result, err := NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
		if err != nil {
			return err
		}
		if _, duplicate := seenResults[result.ID]; duplicate {
			return fmt.Errorf("reviewed Policy Memory decisions produce a duplicate rule")
		}
		seenResults[result.ID] = struct{}{}
	}
	want, err := policyMemoryReviewedDecisionSetDigest(s)
	if err != nil {
		return err
	}
	if s.Digest != want {
		return fmt.Errorf("reviewed Policy Memory decision-set digest is inconsistent")
	}
	return nil
}

func (s PolicyMemoryReviewedDecisionSet) Clone() PolicyMemoryReviewedDecisionSet {
	result := s
	result.Decisions = make([]PolicyMemoryReviewedDecision, len(s.Decisions))
	for index := range s.Decisions {
		result.Decisions[index] = s.Decisions[index].Clone()
	}
	return result
}

func (s PolicyMemoryReviewedDecisionSet) TargetContextIDs() ([]ContextID, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return policyMemoryReviewedTargetContexts(s), nil
}

func policyMemoryReviewedDecisionSetDigest(s PolicyMemoryReviewedDecisionSet) (SemanticDigest, error) {
	return semanticIdentity(struct {
		TargetID           string
		ObservedGeneration uint64
		ObservedRevision   SemanticDigest
		Decisions          []PolicyMemoryReviewedDecision
	}{s.TargetID, s.ObservedGeneration, s.ObservedRevision, s.Decisions})
}

// NormalizePolicyMemoryReviewedDecisionSetAliases upgrades the retained v1
// path-template review identity to its Context-owned v2 semantic alias. It is
// intentionally strict: every predecessor digest must first bind the exact
// predecessor value, so arbitrary invalid review evidence cannot be repaired.
func NormalizePolicyMemoryReviewedDecisionSetAliases(s PolicyMemoryReviewedDecisionSet) (PolicyMemoryReviewedDecisionSet, bool, error) {
	if err := s.Validate(); err == nil {
		return s.Clone(), false, nil
	}
	if s.SchemaVersion != PolicyMemoryReviewedSetSchemaVersion || s.TargetID != PolicyDecisionSetID ||
		s.ObservedGeneration == 0 || s.ObservedRevision.Validate() != nil || len(s.Decisions) == 0 || len(s.Decisions) > MaxPolicyReviewDecisions {
		return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("legacy reviewed Policy Memory decision-set metadata is invalid")
	}
	wantLegacySetDigest, err := policyMemoryReviewedDecisionSetDigest(s)
	if err != nil || s.Digest != wantLegacySetDigest {
		return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("legacy reviewed Policy Memory decision-set digest is inconsistent")
	}
	result := s.Clone()
	changed := false
	for index, legacy := range s.Decisions {
		if err := legacy.Validate(); err == nil {
			continue
		}
		if legacy.Rule.Match != PolicyMatchPathTemplate || len(legacy.Candidates) == 0 {
			return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("reviewed Policy Memory decision is not a recognized legacy alias")
		}
		workspaceID := legacy.Candidates[0].ObservingWorkspaceID
		for _, candidate := range legacy.Candidates {
			if candidate.ObservingWorkspaceID != workspaceID {
				return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("legacy reviewed Policy Memory candidates cross Workspace scope")
			}
		}
		legacyID, legacyIDErr := legacyPolicyMemoryReviewedPathTemplateItemID(legacy.ContextID(), workspaceID, legacy.Rule)
		wantProposal, proposalErr := policyMemoryReviewedProposalDigest(legacy)
		wantDecision, decisionErr := policyMemoryReviewedDecisionDigest(legacy)
		if legacyIDErr != nil || proposalErr != nil || decisionErr != nil || legacy.ReviewItemID != legacyID ||
			legacy.ProposalDigest != wantProposal || legacy.Digest != wantDecision {
			return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("legacy reviewed Policy Memory decision authority is inconsistent")
		}
		currentID, currentIDErr := PolicyMemoryReviewedPathTemplateItemID(legacy.ContextID(), legacy.Rule)
		if currentIDErr != nil {
			return PolicyMemoryReviewedDecisionSet{}, false, currentIDErr
		}
		current, currentErr := NewPolicyMemoryReviewedDecision(currentID, legacy.Candidates, legacy.SourceRules, legacy.Decision, legacy.Rule)
		if currentErr != nil {
			return PolicyMemoryReviewedDecisionSet{}, false, currentErr
		}
		result.Decisions[index] = current
		changed = true
	}
	if !changed {
		return PolicyMemoryReviewedDecisionSet{}, false, fmt.Errorf("reviewed Policy Memory decision set is invalid and has no legacy alias")
	}
	sort.Slice(result.Decisions, func(i, j int) bool { return result.Decisions[i].ReviewItemID < result.Decisions[j].ReviewItemID })
	result.Digest, err = policyMemoryReviewedDecisionSetDigest(result)
	if err != nil {
		return PolicyMemoryReviewedDecisionSet{}, false, err
	}
	if err := result.Validate(); err != nil {
		return PolicyMemoryReviewedDecisionSet{}, false, err
	}
	return result, true, nil
}

// PolicyMemoryReviewedSettlementReceipt is the route-independent confirmed
// global consequence. It binds the exact reviewed set and selected projection;
// OPA-only versus Gateway replacement remains private recovery mechanism.
type PolicyMemoryReviewedSettlementReceipt struct {
	DecisionSetDigest SemanticDigest `json:"decision_set_digest"`
	PlanDigest        SemanticDigest `json:"plan_digest"`
	ContentDigest     SemanticDigest `json:"content_digest"`
	AggregateRevision string         `json:"aggregate_revision"`
	PolicyArtifact    SemanticDigest `json:"policy_artifact_digest"`
	GatewayArtifact   SemanticDigest `json:"gateway_artifact_digest"`
	PrincipalDigest   SemanticDigest `json:"principal_digest"`
}

func (r PolicyMemoryReviewedSettlementReceipt) Validate() error {
	if r.DecisionSetDigest.Validate() != nil || r.PlanDigest.Validate() != nil || r.ContentDigest.Validate() != nil ||
		!aggregateRevisionPattern.MatchString(r.AggregateRevision) || r.PolicyArtifact.Validate() != nil ||
		r.GatewayArtifact.Validate() != nil || r.PrincipalDigest.Validate() != nil {
		return fmt.Errorf("reviewed Policy Memory settlement receipt is incomplete")
	}
	return nil
}

type PolicyMemoryReviewedContextChange struct {
	ContextID ContextID            `json:"context_id"`
	Previous  PolicyMemoryRevision `json:"previous"`
	Current   PolicyMemoryRevision `json:"current"`
}

// PolicyMemoryReviewedAppliedDecision is the semantic public-result boundary.
// Its order exactly follows the reviewed set's canonical ReviewItemID order,
// so presentation never searches resulting memories or reconstructs identity.
type PolicyMemoryReviewedAppliedDecision struct {
	ReviewItemID          string               `json:"review_item_id"`
	RuleID                string               `json:"rule_id"`
	Decision              PolicyMemoryDecision `json:"decision"`
	Match                 string               `json:"match"`
	ContextRef            string               `json:"-"`
	TemplateRef           string               `json:"-"`
	ObservingWorkspaceRef string               `json:"-"`
	ContextID             ContextID            `json:"-"`
	TemplateID            WorkspaceTemplateID  `json:"-"`
	ObservingWorkspaceID  WorkspaceID          `json:"-"`
	ConsumedCandidates    []string             `json:"-"`
	ReplacedSourceRules   []string             `json:"-"`
}

// PolicyMemoryReviewedResult is the route-independent public consequence.
// Complete collections, owner refs, Docker settlement evidence, and consumed
// source authority remain private in PolicyMemoryReviewedSetPublication.
type PolicyMemoryReviewedResult struct {
	SchemaVersion  int                                  `json:"schema_version"`
	Task           string                               `json:"task"`
	AllowCount     int                                  `json:"allow_count"`
	DenyCount      int                                  `json:"deny_count"`
	Applied        bool                                 `json:"applied"`
	ActiveRevision string                               `json:"active_revision"`
	Decisions      []PolicyMemoryReviewedResultDecision `json:"decisions"`
}

type PolicyMemoryReviewedResultDecision struct {
	ReviewItemID string               `json:"review_item_id"`
	RuleID       string               `json:"rule_id"`
	Decision     PolicyMemoryDecision `json:"decision"`
	Match        string               `json:"match"`
}

func NewPolicyMemoryReviewedResult(publication PolicyMemoryReviewedSetPublication) (PolicyMemoryReviewedResult, error) {
	if err := publication.Validate(); err != nil {
		return PolicyMemoryReviewedResult{}, err
	}
	result := PolicyMemoryReviewedResult{SchemaVersion: WorkspaceAuthorityPolicyReadSchemaVersion, Task: TaskPolicyReviewApply, AllowCount: publication.AllowCount, DenyCount: publication.DenyCount, Applied: publication.Applied, ActiveRevision: publication.ActiveRevision, Decisions: make([]PolicyMemoryReviewedResultDecision, len(publication.AppliedDecisions))}
	for index, decision := range publication.AppliedDecisions {
		result.Decisions[index] = PolicyMemoryReviewedResultDecision{ReviewItemID: decision.ReviewItemID, RuleID: decision.RuleID, Decision: decision.Decision, Match: decision.Match}
	}
	return result, result.Validate()
}

func (r PolicyMemoryReviewedResult) Validate() error {
	if r.SchemaVersion != WorkspaceAuthorityPolicyReadSchemaVersion || r.Task != TaskPolicyReviewApply || !r.Applied || !aggregateRevisionPattern.MatchString(r.ActiveRevision) || r.Decisions == nil || r.AllowCount < 0 || r.DenyCount < 0 || r.AllowCount+r.DenyCount != len(r.Decisions) {
		return fmt.Errorf("reviewed Policy Memory result metadata is invalid")
	}
	allow, deny := 0, 0
	previous := ""
	for _, decision := range r.Decisions {
		if ValidatePolicyReviewItemID(decision.ReviewItemID) != nil || ValidatePolicyMemoryRuleID(decision.RuleID) != nil || decision.Decision.Validate() != nil || (decision.Match != PolicyMatchExact && decision.Match != PolicyMatchPathTemplate) || previous != "" && decision.ReviewItemID <= previous {
			return fmt.Errorf("reviewed Policy Memory result decision is invalid")
		}
		if decision.Decision == PolicyMemoryAllow {
			allow++
		} else {
			deny++
		}
		previous = decision.ReviewItemID
	}
	if allow != r.AllowCount || deny != r.DenyCount {
		return fmt.Errorf("reviewed Policy Memory result counts are inconsistent")
	}
	return nil
}

func (d PolicyMemoryReviewedAppliedDecision) Clone() PolicyMemoryReviewedAppliedDecision {
	result := d
	result.ConsumedCandidates = append([]string{}, d.ConsumedCandidates...)
	result.ReplacedSourceRules = append([]string{}, d.ReplacedSourceRules...)
	return result
}

func (c PolicyMemoryReviewedContextChange) Clone() PolicyMemoryReviewedContextChange {
	return PolicyMemoryReviewedContextChange{ContextID: c.ContextID, Previous: c.Previous.Clone(), Current: c.Current.Clone()}
}

// PolicyMemoryReviewedSetPublication is an exhaustive task-owned result. The
// complete collections are internal validation evidence and are not public
// presentation fields.
type PolicyMemoryReviewedSetPublication struct {
	SchemaVersion      int                                   `json:"schema_version"`
	Task               string                                `json:"task"`
	TargetID           string                                `json:"-"`
	DecisionSet        PolicyMemoryReviewedDecisionSet       `json:"-"`
	Previous           WorkspaceAuthorityCollection          `json:"-"`
	Next               WorkspaceAuthorityCollection          `json:"-"`
	LiveCandidates     []PolicyCandidateAuthority            `json:"-"`
	Changes            []PolicyMemoryReviewedContextChange   `json:"-"`
	AppliedDecisions   []PolicyMemoryReviewedAppliedDecision `json:"decisions"`
	Settlement         PolicyMemoryReviewedSettlementReceipt `json:"-"`
	PreviousGeneration uint64                                `json:"-"`
	PreviousRevision   SemanticDigest                        `json:"-"`
	NextGeneration     uint64                                `json:"-"`
	NextRevision       SemanticDigest                        `json:"-"`
	ActiveRevision     string                                `json:"active_revision"`
	AllowCount         int                                   `json:"allow_count"`
	DenyCount          int                                   `json:"deny_count"`
	Applied            bool                                  `json:"applied"`
	Changed            bool                                  `json:"-"`
}

func NewPolicyMemoryReviewedSetPublication(
	previous, next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	settlement PolicyMemoryReviewedSettlementReceipt,
) (PolicyMemoryReviewedSetPublication, error) {
	return NewPolicyMemoryReviewedSetPublicationWithLiveSources(previous, next, set, nil, settlement)
}

// NewPolicyMemoryReviewedSetPublicationWithLiveSources retains the private
// fresh evidence needed to validate a full publication whose reviewed set
// consumed non-durable Gateway observations.
func NewPolicyMemoryReviewedSetPublicationWithLiveSources(
	previous, next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	live []PolicyCandidateAuthority,
	settlement PolicyMemoryReviewedSettlementReceipt,
) (PolicyMemoryReviewedSetPublication, error) {
	changes, applied, err := validatePolicyMemoryReviewedTransition(previous, next, set, live)
	if err != nil {
		return PolicyMemoryReviewedSetPublication{}, err
	}
	if err := settlement.Validate(); err != nil {
		return PolicyMemoryReviewedSetPublication{}, err
	}
	allowCount, denyCount := 0, 0
	for _, decision := range set.Decisions {
		if decision.Decision == PolicyMemoryAllow {
			allowCount++
		} else {
			denyCount++
		}
	}
	result := PolicyMemoryReviewedSetPublication{
		SchemaVersion: PolicyMemoryReviewedSetSchemaVersion, Task: TaskPolicyReviewApply, TargetID: PolicyDecisionSetID,
		DecisionSet: set.Clone(), Previous: previous.Clone(), Next: next.Clone(), LiveCandidates: clonePolicyCandidateAuthorities(live), Changes: changes,
		AppliedDecisions: applied, Settlement: settlement, ActiveRevision: settlement.AggregateRevision,
		PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision,
		NextGeneration: next.Generation, NextRevision: next.Revision,
		AllowCount: allowCount, DenyCount: denyCount, Applied: true, Changed: true,
	}
	return result, result.Validate()
}

func (p PolicyMemoryReviewedSetPublication) Validate() error {
	if p.SchemaVersion != PolicyMemoryReviewedSetSchemaVersion || p.Task != TaskPolicyReviewApply ||
		p.TargetID != PolicyDecisionSetID || p.DecisionSet.TargetID != p.TargetID || !p.Applied || !p.Changed ||
		p.ActiveRevision != p.Settlement.AggregateRevision {
		return fmt.Errorf("reviewed Policy Memory publication metadata is invalid")
	}
	if err := p.DecisionSet.Validate(); err != nil {
		return err
	}
	if err := p.Settlement.Validate(); err != nil {
		return err
	}
	allowCount, denyCount := 0, 0
	for _, decision := range p.DecisionSet.Decisions {
		if decision.Decision == PolicyMemoryAllow {
			allowCount++
		} else {
			denyCount++
		}
	}
	if p.AllowCount != allowCount || p.DenyCount != denyCount || p.AllowCount+p.DenyCount != len(p.DecisionSet.Decisions) {
		return fmt.Errorf("reviewed Policy Memory publication counts do not bind the exact reviewed set")
	}
	if p.PreviousGeneration != p.DecisionSet.ObservedGeneration || p.PreviousRevision != p.DecisionSet.ObservedRevision ||
		p.NextGeneration != p.PreviousGeneration+1 || p.NextRevision.Validate() != nil {
		return fmt.Errorf("reviewed Policy Memory publication envelope receipt is inconsistent")
	}
	emptyCollection := WorkspaceAuthorityCollection{}
	previousEmpty := reflect.DeepEqual(p.Previous, emptyCollection)
	nextEmpty := reflect.DeepEqual(p.Next, emptyCollection)
	if previousEmpty != nextEmpty {
		return fmt.Errorf("reviewed Policy Memory terminal publication is only partially compact")
	}
	compact := previousEmpty
	if compact {
		if len(p.LiveCandidates) != 0 {
			return fmt.Errorf("reviewed Policy Memory terminal publication retained live candidate evidence")
		}
		if err := validatePolicyMemoryReviewedChanges(p.DecisionSet, p.Changes); err != nil {
			return err
		}
		if err := validatePolicyMemoryReviewedApplied(p.DecisionSet, p.AppliedDecisions); err != nil {
			return err
		}
		if p.Settlement.DecisionSetDigest != p.DecisionSet.Digest {
			return fmt.Errorf("reviewed Policy Memory terminal settlement does not bind its decision set")
		}
		return nil
	}
	wantChanges, wantApplied, err := validatePolicyMemoryReviewedTransition(p.Previous, p.Next, p.DecisionSet, p.LiveCandidates)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(wantChanges, p.Changes) {
		return fmt.Errorf("reviewed Policy Memory publication changed-Context result is not exhaustive")
	}
	if !reflect.DeepEqual(wantApplied, p.AppliedDecisions) {
		return fmt.Errorf("reviewed Policy Memory applied decision does not bind its exact input and result")
	}
	projection, err := BuildReviewedWorkspacePolicyProjection(p.Next, policyMemoryReviewedTargetContexts(p.DecisionSet))
	if err != nil {
		return fmt.Errorf("reviewed Policy Memory publication projection: %w", err)
	}
	if p.Settlement.DecisionSetDigest != p.DecisionSet.Digest || p.Settlement.PlanDigest != projection.PlanDigest ||
		p.Settlement.ContentDigest != projection.ContentDigest {
		return fmt.Errorf("reviewed Policy Memory settlement does not bind the exact reviewed set and next authority")
	}
	return nil
}

// NormalizePolicyMemoryReviewedSetPublicationAliases upgrades a compact
// terminal publication whose complete predecessor review authority was v1.
func NormalizePolicyMemoryReviewedSetPublicationAliases(p PolicyMemoryReviewedSetPublication) (PolicyMemoryReviewedSetPublication, bool, error) {
	if err := p.Validate(); err == nil {
		return p.Clone(), false, nil
	}
	set, changed, err := NormalizePolicyMemoryReviewedDecisionSetAliases(p.DecisionSet)
	if err != nil || !changed {
		return PolicyMemoryReviewedSetPublication{}, false, fmt.Errorf("normalize legacy reviewed publication decision set: %w", err)
	}
	if p.SchemaVersion != PolicyMemoryReviewedSetSchemaVersion || p.Task != TaskPolicyReviewApply || p.TargetID != PolicyDecisionSetID ||
		!p.Applied || !p.Changed || p.ActiveRevision != p.Settlement.AggregateRevision || p.PreviousGeneration != p.DecisionSet.ObservedGeneration ||
		p.PreviousRevision != p.DecisionSet.ObservedRevision || p.NextGeneration != p.PreviousGeneration+1 || p.NextRevision.Validate() != nil ||
		!reflect.DeepEqual(p.Previous, WorkspaceAuthorityCollection{}) || !reflect.DeepEqual(p.Next, WorkspaceAuthorityCollection{}) || len(p.LiveCandidates) != 0 ||
		p.Settlement.Validate() != nil || p.Settlement.DecisionSetDigest != p.DecisionSet.Digest || p.AllowCount+p.DenyCount != len(p.DecisionSet.Decisions) {
		return PolicyMemoryReviewedSetPublication{}, false, fmt.Errorf("legacy reviewed Policy Memory terminal publication metadata is invalid")
	}
	allow, deny := 0, 0
	for _, decision := range p.DecisionSet.Decisions {
		if decision.Decision == PolicyMemoryAllow {
			allow++
		} else {
			deny++
		}
	}
	if allow != p.AllowCount || deny != p.DenyCount || len(p.AppliedDecisions) != len(p.DecisionSet.Decisions) {
		return PolicyMemoryReviewedSetPublication{}, false, fmt.Errorf("legacy reviewed Policy Memory publication counts are inconsistent")
	}
	if err := validatePolicyMemoryReviewedChanges(set, p.Changes); err != nil {
		return PolicyMemoryReviewedSetPublication{}, false, err
	}
	result := p.Clone()
	result.Previous = WorkspaceAuthorityCollection{}
	result.Next = WorkspaceAuthorityCollection{}
	result.LiveCandidates = nil
	result.DecisionSet = set
	result.Settlement.DecisionSetDigest = set.Digest
	result.AppliedDecisions = make([]PolicyMemoryReviewedAppliedDecision, len(p.AppliedDecisions))
	for index, legacyDecision := range p.DecisionSet.Decisions {
		item := p.AppliedDecisions[index]
		legacyAlias := legacyDecision.Validate() != nil
		rule, ruleErr := NewPolicyMemoryRule(legacyDecision.ContextID(), legacyDecision.Decision, legacyDecision.Rule)
		contextRef, contextErr := ContextRef(item.ContextID)
		templateRef, templateErr := WorkspaceTemplateRef(item.TemplateID)
		workspaceID, workspaceRef := WorkspaceID(""), ""
		var workspaceErr error
		if legacyAlias || legacyDecision.Rule.Match == PolicyMatchExact {
			workspaceID = legacyDecision.Candidates[0].ObservingWorkspaceID
			workspaceRef, workspaceErr = WorkspaceRef(workspaceID)
		}
		candidateIDs := make([]string, len(legacyDecision.Candidates))
		for sourceIndex := range legacyDecision.Candidates {
			candidateIDs[sourceIndex] = legacyDecision.Candidates[sourceIndex].ID
		}
		ruleIDs := make([]string, len(legacyDecision.SourceRules))
		for sourceIndex := range legacyDecision.SourceRules {
			ruleIDs[sourceIndex] = legacyDecision.SourceRules[sourceIndex].ID
		}
		if ruleErr != nil || contextErr != nil || templateErr != nil || workspaceErr != nil || item.ContextID != legacyDecision.ContextID() ||
			item.ObservingWorkspaceID != workspaceID || item.ReviewItemID != legacyDecision.ReviewItemID ||
			item.RuleID != rule.ID || item.Decision != legacyDecision.Decision || item.Match != legacyDecision.Rule.Match ||
			item.ContextRef != contextRef || item.TemplateRef != templateRef || item.ObservingWorkspaceRef != workspaceRef ||
			!reflect.DeepEqual(item.ConsumedCandidates, candidateIDs) || !reflect.DeepEqual(item.ReplacedSourceRules, ruleIDs) {
			return PolicyMemoryReviewedSetPublication{}, false, fmt.Errorf("legacy reviewed Policy Memory applied decision is inconsistent")
		}
		currentID := legacyDecision.ReviewItemID
		if legacyAlias {
			currentID, err = PolicyMemoryReviewedPathTemplateItemID(legacyDecision.ContextID(), legacyDecision.Rule)
			if err != nil {
				return PolicyMemoryReviewedSetPublication{}, false, err
			}
		}
		currentIndex := sort.Search(len(set.Decisions), func(candidate int) bool { return set.Decisions[candidate].ReviewItemID >= currentID })
		if currentIndex == len(set.Decisions) || set.Decisions[currentIndex].ReviewItemID != currentID || result.AppliedDecisions[currentIndex].ReviewItemID != "" {
			return PolicyMemoryReviewedSetPublication{}, false, fmt.Errorf("legacy reviewed Policy Memory applied decision alias is ambiguous")
		}
		currentItem := item.Clone()
		currentItem.ReviewItemID = currentID
		if legacyDecision.Rule.Match == PolicyMatchPathTemplate {
			currentItem.ObservingWorkspaceID = ""
			currentItem.ObservingWorkspaceRef = ""
		}
		result.AppliedDecisions[currentIndex] = currentItem
	}
	if err := result.Validate(); err != nil {
		return PolicyMemoryReviewedSetPublication{}, false, err
	}
	return result, true, nil
}

// ValidatePolicyMemoryReviewedTransition proves that next is exactly the
// result of applying one confirmed reviewed set to previous. It is the shared
// pre-effect authority boundary for storage and concrete global settlement.
func ValidatePolicyMemoryReviewedTransition(
	previous, next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
) error {
	_, _, err := validatePolicyMemoryReviewedTransition(previous, next, set, nil)
	return err
}

// ValidatePolicyMemoryReviewedTransitionWithLiveSources proves the same exact
// state transition while also binding every non-durable candidate to freshly
// observed private authority.
func ValidatePolicyMemoryReviewedTransitionWithLiveSources(
	previous, next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	live []PolicyCandidateAuthority,
) error {
	_, _, err := validatePolicyMemoryReviewedTransition(previous, next, set, live)
	return err
}

func validatePolicyMemoryReviewedTransition(
	previous, next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	live []PolicyCandidateAuthority,
) ([]PolicyMemoryReviewedContextChange, []PolicyMemoryReviewedAppliedDecision, error) {
	if err := previous.Validate(); err != nil {
		return nil, nil, fmt.Errorf("reviewed Policy Memory predecessor authority: %w", err)
	}
	if err := next.Validate(); err != nil {
		return nil, nil, fmt.Errorf("reviewed Policy Memory next authority: %w", err)
	}
	if err := set.Validate(); err != nil {
		return nil, nil, err
	}
	if set.ObservedGeneration != previous.Generation || set.ObservedRevision != previous.Revision {
		return nil, nil, fmt.Errorf("reviewed Policy Memory transition does not consume its confirmed collection snapshot")
	}
	if next.Generation != previous.Generation+1 ||
		!reflect.DeepEqual(previous.Templates, next.Templates) ||
		!reflect.DeepEqual(previous.Workspaces, next.Workspaces) ||
		!reflect.DeepEqual(previous.DefaultTemplateID, next.DefaultTemplateID) ||
		len(previous.Contexts) != len(next.Contexts) {
		return nil, nil, fmt.Errorf("reviewed Policy Memory transition changed unrelated aggregate authority")
	}
	consumed, err := validatePolicyMemoryReviewedSources(previous, set, live)
	if err != nil {
		return nil, nil, err
	}
	wantPending := make([]PolicyCandidateAuthority, 0, len(previous.PendingCandidates)-len(consumed))
	for _, candidate := range previous.PendingCandidates {
		if _, remove := consumed[candidate.ID]; !remove {
			wantPending = append(wantPending, candidate.Clone())
		}
	}
	if !reflect.DeepEqual(wantPending, next.PendingCandidates) {
		return nil, nil, fmt.Errorf("reviewed Policy Memory transition changed pending authority beyond the reviewed set")
	}
	evidence := PolicyMemoryReviewedSetPublication{Previous: previous, Next: next, DecisionSet: set}
	changes, err := evidence.validateContextTransitions()
	if err != nil {
		return nil, nil, err
	}
	applied, err := policyMemoryReviewedAppliedDecisions(next, set)
	if err != nil {
		return nil, nil, err
	}
	return changes, applied, nil
}

// CompactTerminal retains the bounded exact task result needed after result-
// delivery interruption while dropping complete installation collections.
// Recovery must separately confirm the stored global settlement receipt
// against live authority before returning this publication.
func (p PolicyMemoryReviewedSetPublication) CompactTerminal() (PolicyMemoryReviewedSetPublication, error) {
	if err := p.Validate(); err != nil {
		return PolicyMemoryReviewedSetPublication{}, err
	}
	result := p.Clone()
	result.Previous = WorkspaceAuthorityCollection{}
	result.Next = WorkspaceAuthorityCollection{}
	result.LiveCandidates = nil
	return result, result.Validate()
}

func (p PolicyMemoryReviewedSetPublication) validateAppliedDecisions() error {
	want, err := policyMemoryReviewedAppliedDecisions(p.Next, p.DecisionSet)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(want, p.AppliedDecisions) {
		return fmt.Errorf("reviewed Policy Memory applied decision does not bind its exact input and result")
	}
	return nil
}

func validatePolicyMemoryReviewedChanges(
	set PolicyMemoryReviewedDecisionSet,
	changes []PolicyMemoryReviewedContextChange,
) error {
	decisionsByContext := map[ContextID][]PolicyMemoryReviewedDecision{}
	for _, decision := range set.Decisions {
		decisionsByContext[decision.ContextID()] = append(decisionsByContext[decision.ContextID()], decision)
	}
	if len(changes) != len(decisionsByContext) {
		return fmt.Errorf("reviewed Policy Memory terminal changes are not exhaustive")
	}
	previousContext := ContextID("")
	for _, change := range changes {
		if change.ContextID.Validate() != nil || change.Previous.Validate() != nil || change.Current.Validate() != nil ||
			change.Previous.ContextID != change.ContextID || change.Current.ContextID != change.ContextID ||
			previousContext != "" && change.ContextID <= previousContext {
			return fmt.Errorf("reviewed Policy Memory terminal change metadata is invalid")
		}
		decisions := decisionsByContext[change.ContextID]
		if len(decisions) == 0 {
			return fmt.Errorf("reviewed Policy Memory terminal change has no reviewed input")
		}
		rules := clonePolicyMemoryRules(change.Previous.Rules)
		remove := map[string]PolicyMemoryRule{}
		for _, decision := range decisions {
			for _, source := range decision.SourceRules {
				remove[source.ID] = source
			}
		}
		kept := rules[:0]
		for _, rule := range rules {
			if source, drop := remove[rule.ID]; drop {
				if !reflect.DeepEqual(source, rule) {
					return fmt.Errorf("reviewed Policy Memory terminal source rule changed")
				}
				delete(remove, rule.ID)
				continue
			}
			kept = append(kept, rule)
		}
		if len(remove) != 0 {
			return fmt.Errorf("reviewed Policy Memory terminal source rule is absent")
		}
		rules = kept
		for _, decision := range decisions {
			rule, err := NewPolicyMemoryRule(change.ContextID, decision.Decision, decision.Rule)
			if err != nil {
				return err
			}
			rules = append(rules, rule)
		}
		want, changed, err := PublishPolicyMemory(change.ContextID, rules, &change.Previous)
		if err != nil || !changed || !reflect.DeepEqual(want, change.Current) {
			return fmt.Errorf("reviewed Policy Memory terminal change is inconsistent: %w", err)
		}
		delete(decisionsByContext, change.ContextID)
		previousContext = change.ContextID
	}
	if len(decisionsByContext) != 0 {
		return fmt.Errorf("reviewed Policy Memory terminal result omitted one Context")
	}
	return nil
}

func validatePolicyMemoryReviewedApplied(
	set PolicyMemoryReviewedDecisionSet,
	applied []PolicyMemoryReviewedAppliedDecision,
) error {
	if len(applied) != len(set.Decisions) {
		return fmt.Errorf("reviewed Policy Memory applied result is not exhaustive")
	}
	for index, decision := range set.Decisions {
		item := applied[index]
		rule, err := NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
		if err != nil {
			return err
		}
		contextRef, contextErr := ContextRef(item.ContextID)
		templateRef, templateErr := WorkspaceTemplateRef(item.TemplateID)
		workspaceRef, workspaceID := "", WorkspaceID("")
		var workspaceErr error
		if decision.Rule.Match == PolicyMatchExact {
			workspaceID = decision.Candidates[0].ObservingWorkspaceID
			workspaceRef, workspaceErr = WorkspaceRef(workspaceID)
		}
		candidateIDs := make([]string, len(decision.Candidates))
		for sourceIndex := range decision.Candidates {
			candidateIDs[sourceIndex] = decision.Candidates[sourceIndex].ID
		}
		ruleIDs := make([]string, len(decision.SourceRules))
		for sourceIndex := range decision.SourceRules {
			ruleIDs[sourceIndex] = decision.SourceRules[sourceIndex].ID
		}
		if contextErr != nil || templateErr != nil || workspaceErr != nil || item.ContextID != decision.ContextID() ||
			item.ObservingWorkspaceID != workspaceID || item.ReviewItemID != decision.ReviewItemID ||
			item.RuleID != rule.ID || item.Decision != decision.Decision || item.Match != decision.Rule.Match ||
			item.ContextRef != contextRef || item.TemplateRef != templateRef || item.ObservingWorkspaceRef != workspaceRef ||
			!reflect.DeepEqual(item.ConsumedCandidates, candidateIDs) || !reflect.DeepEqual(item.ReplacedSourceRules, ruleIDs) {
			return fmt.Errorf("reviewed Policy Memory applied decision does not bind its exact input and result")
		}
	}
	return nil
}

func policyMemoryReviewedAppliedDecisions(
	next WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
) ([]PolicyMemoryReviewedAppliedDecision, error) {
	result := make([]PolicyMemoryReviewedAppliedDecision, len(set.Decisions))
	for index, decision := range set.Decisions {
		rule, err := NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
		if err != nil {
			return nil, err
		}
		candidateIDs := make([]string, len(decision.Candidates))
		for sourceIndex := range decision.Candidates {
			candidateIDs[sourceIndex] = decision.Candidates[sourceIndex].ID
		}
		ruleIDs := make([]string, len(decision.SourceRules))
		for sourceIndex := range decision.SourceRules {
			ruleIDs[sourceIndex] = decision.SourceRules[sourceIndex].ID
		}
		var templateID WorkspaceTemplateID
		for _, record := range next.Contexts {
			if record.Context.ID == decision.ContextID() {
				templateID = record.Context.TemplateID
				break
			}
		}
		contextRef, contextErr := ContextRef(decision.ContextID())
		templateRef, templateErr := WorkspaceTemplateRef(templateID)
		workspaceRef, workspaceID := "", WorkspaceID("")
		var workspaceErr error
		if decision.Rule.Match == PolicyMatchExact {
			workspaceID = decision.Candidates[0].ObservingWorkspaceID
			workspaceRef, workspaceErr = WorkspaceRef(workspaceID)
		}
		if contextErr != nil || templateErr != nil || workspaceErr != nil {
			return nil, fmt.Errorf("reviewed Policy Memory applied references are invalid")
		}
		result[index] = PolicyMemoryReviewedAppliedDecision{
			ReviewItemID: decision.ReviewItemID, RuleID: rule.ID, Decision: decision.Decision,
			Match: decision.Rule.Match, ContextRef: contextRef, TemplateRef: templateRef,
			ObservingWorkspaceRef: workspaceRef, ConsumedCandidates: candidateIDs, ReplacedSourceRules: ruleIDs,
			ContextID: decision.ContextID(), TemplateID: templateID,
			ObservingWorkspaceID: workspaceID,
		}
	}
	return result, nil
}

// ValidatePolicyMemoryReviewedLiveSources proves that every decision source is
// either unchanged durable authority or an exact member of one freshly read
// live observation set. The caller owns collection-receipt coherence.
func ValidatePolicyMemoryReviewedLiveSources(
	collection WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	live []PolicyCandidateAuthority,
) error {
	_, err := validatePolicyMemoryReviewedSources(collection, set, live)
	return err
}

func validatePolicyMemoryReviewedSources(
	collection WorkspaceAuthorityCollection,
	set PolicyMemoryReviewedDecisionSet,
	live []PolicyCandidateAuthority,
) (map[string]struct{}, error) {
	pending := make(map[string]PolicyCandidateAuthority, len(collection.PendingCandidates))
	for _, candidate := range collection.PendingCandidates {
		pending[candidate.ID] = candidate
	}
	observed := make(map[string]PolicyCandidateAuthority, len(live))
	for _, candidate := range live {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("live reviewed Policy Memory candidate is invalid: %w", err)
		}
		if durable, exists := pending[candidate.ID]; exists && !reflect.DeepEqual(durable, candidate) {
			return nil, fmt.Errorf("live reviewed Policy Memory candidate conflicts with pending authority")
		}
		if _, duplicate := observed[candidate.ID]; duplicate {
			return nil, fmt.Errorf("live reviewed Policy Memory candidate is duplicated")
		}
		observed[candidate.ID] = candidate
	}
	workspaces := make(map[WorkspaceID]ContextID, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ID] = workspace.ContextID
	}
	memoryByContext := make(map[ContextID]map[string]PolicyMemoryRule, len(collection.Contexts))
	for _, record := range collection.Contexts {
		memoryByContext[record.Context.ID] = make(map[string]PolicyMemoryRule, len(record.PolicyMemory.Rules))
		for _, rule := range record.PolicyMemory.Rules {
			memoryByContext[record.Context.ID][rule.ID] = rule
		}
	}
	consumed := map[string]struct{}{}
	usedObserved := map[string]struct{}{}
	for _, decision := range set.Decisions {
		for _, reviewed := range decision.Candidates {
			candidate, exists := pending[reviewed.ID]
			if !exists {
				candidate, exists = observed[reviewed.ID]
				if exists {
					usedObserved[reviewed.ID] = struct{}{}
				}
			}
			if !exists || !reflect.DeepEqual(candidate, reviewed) {
				return nil, fmt.Errorf("reviewed Policy Memory decision no longer matches pending or live authority")
			}
			if workspaces[candidate.ObservingWorkspaceID] != candidate.ContextID {
				return nil, fmt.Errorf("reviewed Policy Memory decision crosses its observing Workspace")
			}
			for pendingID, pendingCandidate := range pending {
				if policyCandidateAuthoritiesShareEffect(candidate, pendingCandidate) {
					consumed[pendingID] = struct{}{}
				}
			}
		}
		for _, reviewed := range decision.SourceRules {
			rule, exists := memoryByContext[decision.ContextID()][reviewed.ID]
			if !exists || !reflect.DeepEqual(rule, reviewed) {
				return nil, fmt.Errorf("reviewed Policy Memory source rule changed after confirmation")
			}
		}
	}
	if len(usedObserved) != len(observed) {
		return nil, fmt.Errorf("live reviewed Policy Memory evidence contains an unselected candidate")
	}
	return consumed, nil
}

func (p PolicyMemoryReviewedSetPublication) validateContextTransitions() ([]PolicyMemoryReviewedContextChange, error) {
	decisionsByContext := map[ContextID][]PolicyMemoryReviewedDecision{}
	for _, decision := range p.DecisionSet.Decisions {
		decisionsByContext[decision.ContextID()] = append(decisionsByContext[decision.ContextID()], decision)
	}
	wantChanges := make([]PolicyMemoryReviewedContextChange, 0, len(decisionsByContext))
	for index := range p.Previous.Contexts {
		before, after := p.Previous.Contexts[index], p.Next.Contexts[index]
		if before.Context != after.Context {
			return nil, fmt.Errorf("reviewed Policy Memory publication changed Context binding")
		}
		decisions := decisionsByContext[before.Context.ID]
		if len(decisions) == 0 {
			if !reflect.DeepEqual(before, after) {
				return nil, fmt.Errorf("reviewed Policy Memory publication changed an unreviewed Context")
			}
			continue
		}
		if before.ActiveTemplatePolicy == nil || before.ActivePolicyMemory == nil || before.ActivePolicyMemoryRef == nil ||
			!reflect.DeepEqual(*before.ActivePolicyMemory, before.PolicyMemory) ||
			before.ActivePolicyMemoryRef.ValidateFor(before.Context, before.PolicyMemory) != nil {
			return nil, fmt.Errorf("reviewed Policy Memory target is not exactly active")
		}
		rules := clonePolicyMemoryRules(before.PolicyMemory.Rules)
		remove := map[string]struct{}{}
		for _, decision := range decisions {
			for _, source := range decision.SourceRules {
				remove[source.ID] = struct{}{}
			}
		}
		kept := rules[:0]
		for _, rule := range rules {
			if _, drop := remove[rule.ID]; !drop {
				kept = append(kept, rule)
			}
		}
		rules = kept
		for _, decision := range decisions {
			rule, err := NewPolicyMemoryRule(before.Context.ID, decision.Decision, decision.Rule)
			if err != nil {
				return nil, err
			}
			rules = append(rules, rule)
		}
		want, changed, err := PublishPolicyMemory(before.Context.ID, rules, &before.PolicyMemory)
		if err != nil || !changed || !reflect.DeepEqual(want, after.PolicyMemory) || after.ActivePolicyMemory == nil ||
			after.ActivePolicyMemoryRef == nil || !reflect.DeepEqual(*after.ActivePolicyMemory, want) ||
			after.ActivePolicyMemoryRef.ValidateFor(after.Context, want) != nil ||
			!reflect.DeepEqual(before.ActiveTemplatePolicy, after.ActiveTemplatePolicy) {
			return nil, fmt.Errorf("reviewed Policy Memory Context transition is inconsistent: %w", err)
		}
		wantChanges = append(wantChanges, PolicyMemoryReviewedContextChange{
			ContextID: before.Context.ID,
			Previous:  before.PolicyMemory.Clone(),
			Current:   want.Clone(),
		})
	}
	if len(wantChanges) != len(decisionsByContext) {
		return nil, fmt.Errorf("reviewed Policy Memory publication omitted one target Context")
	}
	return wantChanges, nil
}

func (p PolicyMemoryReviewedSetPublication) Clone() PolicyMemoryReviewedSetPublication {
	result := p
	result.DecisionSet = p.DecisionSet.Clone()
	result.Previous = p.Previous.Clone()
	result.Next = p.Next.Clone()
	result.LiveCandidates = clonePolicyCandidateAuthorities(p.LiveCandidates)
	result.Changes = make([]PolicyMemoryReviewedContextChange, len(p.Changes))
	for index := range p.Changes {
		result.Changes[index] = p.Changes[index].Clone()
	}
	result.AppliedDecisions = make([]PolicyMemoryReviewedAppliedDecision, len(p.AppliedDecisions))
	for index := range p.AppliedDecisions {
		result.AppliedDecisions[index] = p.AppliedDecisions[index].Clone()
	}
	return result
}

func policyMemoryReviewedTargetContexts(set PolicyMemoryReviewedDecisionSet) []ContextID {
	seen := map[ContextID]struct{}{}
	for _, decision := range set.Decisions {
		seen[decision.ContextID()] = struct{}{}
	}
	result := make([]ContextID, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
