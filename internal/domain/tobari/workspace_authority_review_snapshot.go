package tobari

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// PolicyMemoryReviewSnapshot is the one coherent Permission Inbox authority.
// It is derived from one complete final collection, so candidate and source
// rule evidence can never come from different envelope revisions.
type PolicyMemoryReviewSnapshot struct {
	SchemaVersion        int                          `json:"schema_version"`
	Task                 string                       `json:"task"`
	Items                []PolicyMemoryReviewItem     `json:"items"`
	Rules                []PolicyMemoryRuleView       `json:"rules"`
	CollectionPresent    bool                         `json:"-"`
	CollectionGeneration uint64                       `json:"observed_generation"`
	CollectionRevision   SemanticDigest               `json:"observed_revision"`
	Collection           WorkspaceAuthorityCollection `json:"-"`
}

type PolicyMemoryReviewChoice struct {
	ReviewItemID string               `json:"review_item_id"`
	Decision     PolicyMemoryDecision `json:"decision"`
}

type PolicyMemoryReviewChoiceSet struct {
	ObservedGeneration uint64                     `json:"observed_generation"`
	ObservedRevision   SemanticDigest             `json:"observed_revision"`
	Decisions          []PolicyMemoryReviewChoice `json:"decisions"`
}

func (s PolicyMemoryReviewChoiceSet) Validate() error {
	if s.ObservedGeneration == 0 || s.ObservedRevision.Validate() != nil || len(s.Decisions) == 0 || len(s.Decisions) > MaxPolicyReviewDecisions {
		return fmt.Errorf("Permission Inbox choice-set metadata is invalid")
	}
	seen := map[string]struct{}{}
	for _, choice := range s.Decisions {
		if ValidatePolicyReviewItemID(choice.ReviewItemID) != nil || choice.Decision.Validate() != nil {
			return fmt.Errorf("Permission Inbox choice is invalid")
		}
		if _, duplicate := seen[choice.ReviewItemID]; duplicate {
			return fmt.Errorf("Permission Inbox choice is duplicated")
		}
		seen[choice.ReviewItemID] = struct{}{}
	}
	return nil
}

// PolicyMemoryReviewItem is one immutable exact or path-template proposal.
// Complete candidate and source-rule authority stays private; public fields
// contain only final owner facts needed to review the effect.
type PolicyMemoryReviewItem struct {
	ID                   string                     `json:"id"`
	Match                string                     `json:"match"`
	Context              string                     `json:"context"`
	Template             string                     `json:"template"`
	ProjectRoot          string                     `json:"project_root"`
	ObservingWorkspace   string                     `json:"observing_workspace"`
	ContextID            ContextID                  `json:"context_id"`
	TemplateID           WorkspaceTemplateID        `json:"workspace_template_id"`
	ObservingWorkspaceID WorkspaceID                `json:"observing_workspace_id"`
	Rule                 PolicyMemoryRuleBody       `json:"rule"`
	Candidates           []PolicyCandidateAuthority `json:"-"`
	SourceRules          []PolicyMemoryRule         `json:"-"`
}

func (i PolicyMemoryReviewItem) Clone() PolicyMemoryReviewItem {
	result := i
	result.Rule = i.Rule.Clone()
	result.Candidates = clonePolicyCandidateAuthorities(i.Candidates)
	result.SourceRules = clonePolicyMemoryRules(i.SourceRules)
	return result
}

func (i PolicyMemoryReviewItem) Validate() error {
	if ValidatePolicyReviewItemID(i.ID) != nil || (i.Match != PolicyMatchExact && i.Match != PolicyMatchPathTemplate) ||
		i.ContextID.Validate() != nil || i.TemplateID.Validate() != nil || i.ObservingWorkspaceID.Validate() != nil ||
		ValidateName(i.Template) != nil || i.Context != i.Template || ValidateCanonicalRoot(i.ProjectRoot) != nil ||
		ValidateCanonicalRoot(i.ObservingWorkspace) != nil || len(i.Candidates) == 0 {
		return fmt.Errorf("Policy Memory review item metadata is invalid")
	}
	for _, candidate := range i.Candidates {
		if err := candidate.Validate(); err != nil || candidate.ContextID != i.ContextID || candidate.ObservingWorkspaceID != i.ObservingWorkspaceID {
			return fmt.Errorf("Policy Memory review candidate authority is inconsistent")
		}
	}
	for _, rule := range i.SourceRules {
		if err := rule.Validate(i.ContextID); err != nil || rule.Decision != PolicyMemoryAllow || rule.Body.Match != PolicyMatchExact {
			return fmt.Errorf("Policy Memory review source rule authority is inconsistent")
		}
	}
	decision := PolicyMemoryAllow
	if i.Match == PolicyMatchExact {
		if len(i.Candidates) != 1 || len(i.SourceRules) != 0 || i.ID != i.Candidates[0].ID ||
			!reflect.DeepEqual(i.Rule, i.Candidates[0].Effect.RuleBody(i.Candidates[0].ID)) {
			return fmt.Errorf("exact Policy Memory review item is inconsistent")
		}
	} else if len(i.Candidates)+len(i.SourceRules) < 2 || i.Rule.Match != PolicyMatchPathTemplate {
		return fmt.Errorf("template Policy Memory review item has insufficient authority")
	}
	reviewed, err := NewPolicyMemoryReviewedDecision(i.ID, i.Candidates, i.SourceRules, decision, i.Rule)
	if err != nil || reviewed.ReviewItemID != i.ID {
		return fmt.Errorf("Policy Memory review item does not bind a canonical decision")
	}
	return nil
}

// ReviewedDecision applies one explicit human choice to the unchanged item.
func (i PolicyMemoryReviewItem) ReviewedDecision(decision PolicyMemoryDecision) (PolicyMemoryReviewedDecision, error) {
	if err := i.Validate(); err != nil {
		return PolicyMemoryReviewedDecision{}, err
	}
	if i.Match == PolicyMatchPathTemplate && decision != PolicyMemoryAllow {
		return PolicyMemoryReviewedDecision{}, fmt.Errorf("path-template Permission Inbox items can only be allowed")
	}
	return NewPolicyMemoryReviewedDecision(i.ID, i.Candidates, i.SourceRules, decision, i.Rule)
}

func (s PolicyMemoryReviewSnapshot) Clone() PolicyMemoryReviewSnapshot {
	result := s
	result.Items = make([]PolicyMemoryReviewItem, len(s.Items))
	for index := range s.Items {
		result.Items[index] = s.Items[index].Clone()
	}
	result.Rules = make([]PolicyMemoryRuleView, len(s.Rules))
	for index := range s.Rules {
		result.Rules[index] = s.Rules[index].Clone()
	}
	if s.CollectionPresent {
		result.Collection = s.Collection.Clone()
	} else {
		result.Collection = WorkspaceAuthorityCollection{}
	}
	return result
}

func (s PolicyMemoryReviewSnapshot) Validate() error {
	if s.SchemaVersion != WorkspaceAuthorityPolicyReadSchemaVersion || s.Task != TaskPolicyReview || s.Items == nil || s.Rules == nil {
		return fmt.Errorf("Permission Inbox snapshot metadata is invalid")
	}
	if !s.CollectionPresent {
		if s.CollectionGeneration != 0 || s.CollectionRevision != "" || len(s.Items) != 0 || len(s.Rules) != 0 || !reflect.DeepEqual(s.Collection, WorkspaceAuthorityCollection{}) {
			return fmt.Errorf("fresh Permission Inbox carries initialized authority")
		}
		return nil
	}
	if err := s.Collection.Validate(); err != nil || s.CollectionGeneration != s.Collection.Generation || s.CollectionRevision != s.Collection.Revision {
		return fmt.Errorf("Permission Inbox collection receipt is invalid")
	}
	previous := ""
	for _, item := range s.Items {
		if err := item.Validate(); err != nil || previous != "" && item.ID <= previous {
			return fmt.Errorf("Permission Inbox items must be valid, unique, and sorted")
		}
		previous = item.ID
	}
	for _, rule := range s.Rules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (s PolicyMemoryReviewSnapshot) ValidateFor(collection WorkspaceAuthorityCollection, present bool) error {
	if err := s.Validate(); err != nil {
		return err
	}
	want, err := NewPolicyMemoryReviewSnapshot(collection, present)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(s, want) {
		return fmt.Errorf("Permission Inbox snapshot does not match its complete final collection")
	}
	return nil
}

func (s PolicyMemoryReviewSnapshot) ReviewedSet(choices map[string]PolicyMemoryDecision) (PolicyMemoryReviewedDecisionSet, error) {
	if err := s.Validate(); err != nil || !s.CollectionPresent {
		return PolicyMemoryReviewedDecisionSet{}, fmt.Errorf("Permission Inbox snapshot is not initialized")
	}
	if len(choices) == 0 {
		return PolicyMemoryReviewedDecisionSet{}, fmt.Errorf("Permission Inbox decision set is empty")
	}
	items := make(map[string]PolicyMemoryReviewItem, len(s.Items))
	for _, item := range s.Items {
		items[item.ID] = item
	}
	decisions := make([]PolicyMemoryReviewedDecision, 0, len(choices))
	for id, choice := range choices {
		item, found := items[id]
		if !found {
			return PolicyMemoryReviewedDecisionSet{}, fmt.Errorf("Permission Inbox choice is outside the unchanged snapshot")
		}
		decision, err := item.ReviewedDecision(choice)
		if err != nil {
			return PolicyMemoryReviewedDecisionSet{}, err
		}
		decisions = append(decisions, decision)
	}
	return NewPolicyMemoryReviewedDecisionSet(s.Collection, decisions)
}

func (s PolicyMemoryReviewSnapshot) ReviewedChoiceSet(choices PolicyMemoryReviewChoiceSet) (PolicyMemoryReviewedDecisionSet, error) {
	if err := choices.Validate(); err != nil || choices.ObservedGeneration != s.CollectionGeneration || choices.ObservedRevision != s.CollectionRevision {
		return PolicyMemoryReviewedDecisionSet{}, fmt.Errorf("Permission Inbox choices do not bind the unchanged snapshot")
	}
	selected := make(map[string]PolicyMemoryDecision, len(choices.Decisions))
	for _, choice := range choices.Decisions {
		selected[choice.ReviewItemID] = choice.Decision
	}
	return s.ReviewedSet(selected)
}

func NewPolicyMemoryReviewSnapshot(collection WorkspaceAuthorityCollection, present bool) (PolicyMemoryReviewSnapshot, error) {
	result := PolicyMemoryReviewSnapshot{SchemaVersion: WorkspaceAuthorityPolicyReadSchemaVersion, Task: TaskPolicyReview, Items: []PolicyMemoryReviewItem{}, Rules: []PolicyMemoryRuleView{}, CollectionPresent: present}
	if !present {
		return result, result.Validate()
	}
	if err := collection.Validate(); err != nil {
		return PolicyMemoryReviewSnapshot{}, err
	}
	rules, err := NewPolicyMemoryRuleList(collection, true)
	if err != nil {
		return PolicyMemoryReviewSnapshot{}, err
	}
	result.CollectionGeneration, result.CollectionRevision, result.Collection = collection.Generation, collection.Revision, collection.Clone()
	result.Rules = rules.Items
	result.Items, err = policyMemoryReviewItems(collection)
	if err != nil {
		return PolicyMemoryReviewSnapshot{}, err
	}
	return result, result.Validate()
}

type finalReviewEvidence struct {
	path      string
	candidate *PolicyCandidateAuthority
	rule      *PolicyMemoryRule
}

func policyMemoryReviewItems(collection WorkspaceAuthorityCollection) ([]PolicyMemoryReviewItem, error) {
	templates := map[WorkspaceTemplateID]WorkspaceTemplate{}
	contexts := map[ContextID]WorkspaceAuthorityContextRecord{}
	workspaces := map[WorkspaceID]WorkspaceBinding{}
	for _, template := range collection.Templates {
		templates[template.ID] = template
	}
	for _, record := range collection.Contexts {
		contexts[record.Context.ID] = record
	}
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ID] = workspace
	}

	consumed := map[string]struct{}{}
	items := []PolicyMemoryReviewItem{}
	for _, seed := range collection.PendingCandidates {
		if _, found := consumed[seed.ID]; found || seed.Effect.EffectiveProtocol() != PolicyProtocolHTTP {
			continue
		}
		evidence := []finalReviewEvidence{{path: seed.Effect.Path, candidate: clonePolicyCandidatePointer(seed)}}
		for _, candidate := range collection.PendingCandidates {
			if candidate.ID == seed.ID || candidate.ContextID != seed.ContextID || candidate.ObservingWorkspaceID != seed.ObservingWorkspaceID || !reviewedTemplateIdentityMatchesEffect(seed.Effect.RuleBody(seed.ID), candidate.Effect) {
				continue
			}
			evidence = append(evidence, finalReviewEvidence{path: candidate.Effect.Path, candidate: clonePolicyCandidatePointer(candidate)})
		}
		for _, rule := range contexts[seed.ContextID].PolicyMemory.Rules {
			if rule.Decision != PolicyMemoryAllow || rule.Body.Match != PolicyMatchExact || !reviewedTemplateIdentityMatchesBody(seed.Effect.RuleBody(seed.ID), rule.Body) {
				continue
			}
			cloned := rule.Clone()
			evidence = append(evidence, finalReviewEvidence{path: rule.Body.Path, rule: &cloned})
		}
		templatesFound := map[string][]string{}
		for left := 0; left < len(evidence); left++ {
			for right := left + 1; right < len(evidence); right++ {
				path, segments, ok := inferPathTemplate(evidence[left].path, evidence[right].path)
				if ok {
					templatesFound[path] = segments
				}
			}
		}
		if len(templatesFound) != 1 {
			continue
		}
		var templatePath string
		var segments []string
		for templatePath, segments = range templatesFound {
		}
		candidates := []PolicyCandidateAuthority{}
		sourceRules := []PolicyMemoryRule{}
		examples := []string{}
		sourceCandidates := []string{}
		for _, source := range evidence {
			if !pathTemplateMatches(segments, source.path) {
				continue
			}
			examples = append(examples, source.path)
			if source.candidate != nil {
				candidates = append(candidates, source.candidate.Clone())
				sourceCandidates = append(sourceCandidates, source.candidate.ID)
			}
			if source.rule != nil {
				sourceRules = append(sourceRules, source.rule.Clone())
				sourceCandidates = append(sourceCandidates, source.rule.Body.SourceCandidates...)
			}
		}
		if len(candidates) == 0 || len(candidates)+len(sourceRules) < 2 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
		sort.Slice(sourceRules, func(i, j int) bool { return sourceRules[i].ID < sourceRules[j].ID })
		sort.Strings(examples)
		examples = uniqueSortedStrings(examples)
		sort.Strings(sourceCandidates)
		sourceCandidates = uniqueSortedStrings(sourceCandidates)
		rule := seed.Effect.RuleBody(seed.ID)
		rule.Match, rule.Path, rule.Segments, rule.Examples, rule.SourceCandidates = PolicyMatchPathTemplate, templatePath, append([]string{}, segments...), examples, sourceCandidates
		id, err := PolicyMemoryReviewedPathTemplateItemID(seed.ContextID, seed.ObservingWorkspaceID, rule)
		if err != nil {
			return nil, err
		}
		item, err := newPolicyMemoryReviewItem(collection, id, PolicyMatchPathTemplate, candidates, sourceRules, rule, templates, contexts, workspaces)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		for _, candidate := range candidates {
			consumed[candidate.ID] = struct{}{}
		}
	}
	for _, candidate := range collection.PendingCandidates {
		if _, found := consumed[candidate.ID]; found {
			continue
		}
		item, err := newPolicyMemoryReviewItem(collection, candidate.ID, PolicyMatchExact, []PolicyCandidateAuthority{candidate}, nil, candidate.Effect.RuleBody(candidate.ID), templates, contexts, workspaces)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

func clonePolicyCandidatePointer(value PolicyCandidateAuthority) *PolicyCandidateAuthority {
	cloned := value.Clone()
	return &cloned
}

func newPolicyMemoryReviewItem(_ WorkspaceAuthorityCollection, id, match string, candidates []PolicyCandidateAuthority, sourceRules []PolicyMemoryRule, rule PolicyMemoryRuleBody, templates map[WorkspaceTemplateID]WorkspaceTemplate, contexts map[ContextID]WorkspaceAuthorityContextRecord, workspaces map[WorkspaceID]WorkspaceBinding) (PolicyMemoryReviewItem, error) {
	contextRecord, found := contexts[candidates[0].ContextID]
	if !found {
		return PolicyMemoryReviewItem{}, fmt.Errorf("Permission Inbox Context is missing")
	}
	template, found := templates[contextRecord.Context.TemplateID]
	workspace, workspaceFound := workspaces[candidates[0].ObservingWorkspaceID]
	if !found || !workspaceFound {
		return PolicyMemoryReviewItem{}, fmt.Errorf("Permission Inbox owner is missing")
	}
	item := PolicyMemoryReviewItem{ID: id, Match: match, Context: template.Name, Template: template.Name, ProjectRoot: contextRecord.Context.ProjectRoot, ObservingWorkspace: workspace.ProjectRoot, ContextID: contextRecord.Context.ID, TemplateID: template.ID, ObservingWorkspaceID: workspace.ID, Rule: rule.Clone(), Candidates: clonePolicyCandidateAuthorities(candidates), SourceRules: clonePolicyMemoryRules(sourceRules)}
	if err := item.Validate(); err != nil {
		return PolicyMemoryReviewItem{}, err
	}
	return item, nil
}

// finalReviewIdentityKey is deliberately unused as a selector; it only keeps
// future diagnostics explicit about all dimensions that make two effects
// compatible for one path-template proposal.
func finalReviewIdentityKey(candidate PolicyCandidateAuthority) string {
	return strings.Join(appendPolicyProtocolIdentity([]string{string(candidate.ContextID), string(candidate.ObservingWorkspaceID), candidate.Effect.Host, fmt.Sprint(candidate.Effect.Port), candidate.Effect.Method}, candidate.Effect.PolicyProtocolIdentity), "\x00")
}
