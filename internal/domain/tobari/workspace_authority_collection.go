package tobari

import (
	"fmt"
	"path/filepath"
	"sort"
)

const WorkspaceAuthorityCollectionSchemaVersion = 2

// WorkspaceAuthorityContextRecord is the Context-owned portion of one final
// authority envelope. Template and Workspace values remain normalized so one
// installation snapshot cannot carry divergent copies of either authority.
type WorkspaceAuthorityContextRecord struct {
	Context               ContextBinding                   `json:"context"`
	ContextHome           string                           `json:"context_home,omitempty"`
	CreationDefaults      SemanticDigest                   `json:"creation_defaults_digest,omitempty"`
	PolicyMemory          PolicyMemoryRevision             `json:"policy_memory"`
	ActiveTemplatePolicy  *TemplatePolicyActivationReceipt `json:"active_template_policy,omitempty"`
	ActivePolicyMemory    *PolicyMemoryRevision            `json:"active_policy_memory,omitempty"`
	ActivePolicyMemoryRef *PolicyMemoryActivationReceipt   `json:"active_policy_memory_receipt,omitempty"`
	SupersededAllows      []PolicyMemoryAllowSupersession  `json:"superseded_policy_memory_allows,omitempty"`
	SupersededCandidates  []PolicyCandidateSupersession    `json:"superseded_policy_candidates,omitempty"`
}

type PolicyMemoryAllowSupersession struct {
	SchemaVersion        int                 `json:"schema_version"`
	ContextID            ContextID           `json:"context_id"`
	Rule                 PolicyMemoryRule    `json:"rule"`
	SourceMemoryRevision SemanticDigest      `json:"source_policy_memory_revision"`
	TemplateID           WorkspaceTemplateID `json:"workspace_template_id"`
	TemplateRevision     SemanticDigest      `json:"workspace_template_revision"`
}

// PolicyCandidateSupersession retains the exact ungranted proposal removed
// when a moving-head Template Boundary becomes narrower. It is history only:
// the candidate never enters Policy Memory and cannot authorize a request.
type PolicyCandidateSupersession struct {
	SchemaVersion    int                      `json:"schema_version"`
	ContextID        ContextID                `json:"context_id"`
	Candidate        PolicyCandidateAuthority `json:"candidate"`
	TemplateID       WorkspaceTemplateID      `json:"workspace_template_id"`
	TemplateRevision SemanticDigest           `json:"workspace_template_revision"`
}

func (s PolicyCandidateSupersession) Validate() error {
	if s.SchemaVersion != 1 || s.ContextID.Validate() != nil || s.Candidate.Validate() != nil || s.Candidate.ContextID != s.ContextID || s.TemplateID.Validate() != nil || s.TemplateRevision.Validate() != nil {
		return fmt.Errorf("Policy candidate supersession is invalid")
	}
	return nil
}

func (s PolicyMemoryAllowSupersession) Validate() error {
	if s.SchemaVersion != 1 || s.ContextID.Validate() != nil || s.Rule.Decision != PolicyMemoryAllow || s.Rule.Validate(s.ContextID) != nil || s.SourceMemoryRevision.Validate() != nil || s.TemplateID.Validate() != nil || s.TemplateRevision.Validate() != nil {
		return fmt.Errorf("Policy Memory Allow supersession is invalid")
	}
	return nil
}

func (r WorkspaceAuthorityContextRecord) Clone() WorkspaceAuthorityContextRecord {
	result := r
	result.PolicyMemory = r.PolicyMemory.Clone()
	if r.ActiveTemplatePolicy != nil {
		value := *r.ActiveTemplatePolicy
		result.ActiveTemplatePolicy = &value
	}
	if r.ActivePolicyMemory != nil {
		value := r.ActivePolicyMemory.Clone()
		result.ActivePolicyMemory = &value
	}
	if r.ActivePolicyMemoryRef != nil {
		value := *r.ActivePolicyMemoryRef
		result.ActivePolicyMemoryRef = &value
	}
	if r.SupersededAllows != nil {
		result.SupersededAllows = make([]PolicyMemoryAllowSupersession, len(r.SupersededAllows))
		for index, item := range r.SupersededAllows {
			result.SupersededAllows[index] = item
			result.SupersededAllows[index].Rule = item.Rule.Clone()
		}
	}
	if r.SupersededCandidates != nil {
		result.SupersededCandidates = make([]PolicyCandidateSupersession, len(r.SupersededCandidates))
		for index, item := range r.SupersededCandidates {
			result.SupersededCandidates[index] = item
			result.SupersededCandidates[index].Candidate = item.Candidate.Clone()
		}
	}
	return result
}

// WorkspaceAuthorityCollection is one atomically published final-authority
// envelope. Revision is an installation-level coherence receipt only; it is
// never a Template, Context, Policy Memory, or Workspace authority identity.
type WorkspaceAuthorityCollection struct {
	SchemaVersion     int                               `json:"schema_version"`
	Generation        uint64                            `json:"generation"`
	Revision          SemanticDigest                    `json:"revision"`
	Templates         []WorkspaceTemplate               `json:"workspace_templates"`
	Contexts          []WorkspaceAuthorityContextRecord `json:"contexts"`
	Workspaces        []WorkspaceBinding                `json:"workspaces"`
	PendingCandidates []PolicyCandidateAuthority        `json:"pending_candidates"`
	DefaultTemplateID *WorkspaceTemplateID              `json:"default_workspace_template_id,omitempty"`
	CurrentContextID  *ContextID                        `json:"current_context_id,omitempty"`
}

type workspaceAuthorityCollectionContent struct {
	Templates         []WorkspaceTemplate
	Contexts          []WorkspaceAuthorityContextRecord
	Workspaces        []WorkspaceBinding
	PendingCandidates []PolicyCandidateAuthority
	DefaultTemplateID *WorkspaceTemplateID
	CurrentContextID  *ContextID
}

func PublishWorkspaceAuthorityCollection(
	templates []WorkspaceTemplate,
	contexts []WorkspaceAuthorityContextRecord,
	workspaces []WorkspaceBinding,
	candidates []PolicyCandidateAuthority,
	defaultTemplateID *WorkspaceTemplateID,
	previous *WorkspaceAuthorityCollection,
) (WorkspaceAuthorityCollection, bool, error) {
	var currentContextID *ContextID
	if previous != nil && previous.CurrentContextID != nil {
		preserved := false
		for _, record := range contexts {
			if record.Context.ID == *previous.CurrentContextID {
				value := *previous.CurrentContextID
				currentContextID = &value
				preserved = true
				break
			}
		}
		if !preserved {
			return WorkspaceAuthorityCollection{}, false, fmt.Errorf("current Context selection cannot be removed implicitly")
		}
	}
	return publishWorkspaceAuthorityCollection(templates, contexts, workspaces, candidates, defaultTemplateID, currentContextID, previous, false)
}

// PublishWorkspaceAuthorityCollectionWithCurrentContext publishes the same
// coherent authority envelope while deliberately changing only the
// installation-owned current Context selection. A nil selection is an
// explicit known absence and is never derived from CWD or Workspace state.
func PublishWorkspaceAuthorityCollectionWithCurrentContext(
	templates []WorkspaceTemplate,
	contexts []WorkspaceAuthorityContextRecord,
	workspaces []WorkspaceBinding,
	candidates []PolicyCandidateAuthority,
	defaultTemplateID *WorkspaceTemplateID,
	currentContextID *ContextID,
	previous *WorkspaceAuthorityCollection,
) (WorkspaceAuthorityCollection, bool, error) {
	return publishWorkspaceAuthorityCollection(templates, contexts, workspaces, candidates, defaultTemplateID, currentContextID, previous, false)
}

func publishWorkspaceAuthorityCollectionForEntry(
	templates []WorkspaceTemplate,
	contexts []WorkspaceAuthorityContextRecord,
	workspaces []WorkspaceBinding,
	candidates []PolicyCandidateAuthority,
	defaultTemplateID *WorkspaceTemplateID,
	previous *WorkspaceAuthorityCollection,
) (WorkspaceAuthorityCollection, bool, error) {
	var currentContextID *ContextID
	if previous != nil && previous.CurrentContextID != nil {
		value := *previous.CurrentContextID
		currentContextID = &value
	}
	return publishWorkspaceAuthorityCollection(templates, contexts, workspaces, candidates, defaultTemplateID, currentContextID, previous, true)
}

func publishWorkspaceAuthorityCollection(
	templates []WorkspaceTemplate,
	contexts []WorkspaceAuthorityContextRecord,
	workspaces []WorkspaceBinding,
	candidates []PolicyCandidateAuthority,
	defaultTemplateID *WorkspaceTemplateID,
	currentContextID *ContextID,
	previous *WorkspaceAuthorityCollection,
	allowContextHomeInitialization bool,
) (WorkspaceAuthorityCollection, bool, error) {
	if templates == nil || contexts == nil || workspaces == nil || candidates == nil {
		return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Workspace authority collections must be explicit")
	}
	result := WorkspaceAuthorityCollection{
		SchemaVersion:     WorkspaceAuthorityCollectionSchemaVersion,
		Generation:        1,
		Templates:         cloneWorkspaceTemplates(templates),
		Contexts:          cloneWorkspaceAuthorityContextRecords(contexts),
		Workspaces:        cloneWorkspaceBindings(workspaces),
		PendingCandidates: clonePolicyCandidateAuthorities(candidates),
	}
	bindContextHomeAuthority(result.Contexts, result.Workspaces)
	if defaultTemplateID != nil {
		value := *defaultTemplateID
		result.DefaultTemplateID = &value
	}
	if currentContextID != nil {
		value := *currentContextID
		result.CurrentContextID = &value
	}
	sort.Slice(result.Templates, func(i, j int) bool { return result.Templates[i].ID < result.Templates[j].ID })
	sort.Slice(result.Contexts, func(i, j int) bool { return result.Contexts[i].Context.ID < result.Contexts[j].Context.ID })
	sort.Slice(result.Workspaces, func(i, j int) bool { return result.Workspaces[i].ID < result.Workspaces[j].ID })
	sort.Slice(result.PendingCandidates, func(i, j int) bool { return result.PendingCandidates[i].ID < result.PendingCandidates[j].ID })
	if previous != nil {
		if err := previous.Validate(); err != nil {
			return WorkspaceAuthorityCollection{}, false, err
		}
		currentContexts := make(map[ContextID]WorkspaceAuthorityContextRecord, len(result.Contexts))
		for _, record := range result.Contexts {
			currentContexts[record.Context.ID] = record
		}
		for _, before := range previous.Contexts {
			after, retained := currentContexts[before.Context.ID]
			if !retained {
				continue
			}
			if before.ContextHome == "" {
				if after.ContextHome != "" && !allowContextHomeInitialization {
					return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Context Home initialization requires an exact Workspace entry plan")
				}
				continue
			}
			if after.ContextHome != before.ContextHome || after.CreationDefaults != before.CreationDefaults {
				return WorkspaceAuthorityCollection{}, false, fmt.Errorf("Context Home creation authority cannot change")
			}
		}
		result.Generation = previous.Generation + 1
	}
	revision, err := workspaceAuthorityCollectionRevision(result)
	if err != nil {
		return WorkspaceAuthorityCollection{}, false, err
	}
	if previous != nil && previous.Revision == revision {
		return previous.Clone(), false, nil
	}
	result.Revision = revision
	return result, true, result.Validate()
}

// bindContextHomeAuthority promotes the first Workspace's create-once Home
// facts into Context authority. Once present, those facts survive even when
// the Context temporarily owns no Workspaces.
func bindContextHomeAuthority(contexts []WorkspaceAuthorityContextRecord, workspaces []WorkspaceBinding) {
	indexes := make(map[ContextID]int, len(contexts))
	for index := range contexts {
		indexes[contexts[index].Context.ID] = index
	}
	for _, workspace := range workspaces {
		index, found := indexes[workspace.ContextID]
		if !found || contexts[index].ContextHome != "" || contexts[index].CreationDefaults != "" {
			continue
		}
		contexts[index].ContextHome = workspace.Home
		contexts[index].CreationDefaults = workspace.CreationDefaults
	}
}

func workspaceAuthorityCollectionRevision(collection WorkspaceAuthorityCollection) (SemanticDigest, error) {
	contexts := cloneWorkspaceAuthorityContextRecords(collection.Contexts)
	workspaceOwners := make(map[ContextID]struct{}, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaceOwners[workspace.ContextID] = struct{}{}
	}
	for index := range contexts {
		if _, found := workspaceOwners[contexts[index].Context.ID]; found {
			// While a Workspace exists these values are validated mirrors of
			// Workspace authority. Excluding the mirrors preserves the schema-v2
			// collection revision for compatible reads; once the last Workspace
			// is removed, the retained Context-owned values become revision-bearing.
			contexts[index].ContextHome = ""
			contexts[index].CreationDefaults = ""
		}
	}
	content := workspaceAuthorityCollectionContent{
		Templates: collection.Templates, Contexts: contexts, Workspaces: collection.Workspaces,
		PendingCandidates: collection.PendingCandidates, DefaultTemplateID: collection.DefaultTemplateID,
		CurrentContextID: collection.CurrentContextID,
	}
	return semanticIdentity(content)
}

func (c WorkspaceAuthorityCollection) Validate() error {
	if c.SchemaVersion != WorkspaceAuthorityCollectionSchemaVersion || c.Generation == 0 {
		return fmt.Errorf("Workspace authority collection metadata is invalid")
	}
	if c.Templates == nil || c.Contexts == nil || c.Workspaces == nil || c.PendingCandidates == nil {
		return fmt.Errorf("Workspace authority collection is incomplete")
	}
	if err := ValidateWorkspaceTemplateAuthorities(c.Templates); err != nil {
		return err
	}
	templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(c.Templates))
	previousTemplate := WorkspaceTemplateID("")
	for _, template := range c.Templates {
		if previousTemplate != "" && template.ID <= previousTemplate {
			return fmt.Errorf("Workspace Templates must be unique and sorted")
		}
		templates[template.ID] = template
		previousTemplate = template.ID
	}

	bindings := make([]ContextBinding, len(c.Contexts))
	records := make(map[ContextID]WorkspaceAuthorityContextRecord, len(c.Contexts))
	previousContext := ContextID("")
	for index, record := range c.Contexts {
		if previousContext != "" && record.Context.ID <= previousContext {
			return fmt.Errorf("Contexts must be unique and sorted")
		}
		if _, exists := templates[record.Context.TemplateID]; !exists {
			return fmt.Errorf("Context refers to an unavailable Workspace Template")
		}
		bindings[index] = record.Context
		if (record.ContextHome == "") != (record.CreationDefaults == "") {
			return fmt.Errorf("Context Home authority is incomplete")
		}
		if record.ContextHome != "" {
			if !filepath.IsAbs(record.ContextHome) || filepath.Clean(record.ContextHome) != record.ContextHome {
				return fmt.Errorf("Context Home authority is invalid")
			}
			if err := record.CreationDefaults.Validate(); err != nil {
				return fmt.Errorf("Context Home creation authority is invalid: %w", err)
			}
		}
		records[record.Context.ID] = record
		previousContext = record.Context.ID
	}
	if err := ValidateContextBindings(bindings); err != nil {
		return err
	}

	workspacesByContext := make(map[ContextID][]WorkspaceBinding, len(c.Contexts))
	workspacesByID := make(map[WorkspaceID]WorkspaceBinding, len(c.Workspaces))
	workspacesByHome := make(map[string]ContextID, len(c.Contexts))
	workspacesByContextRoot := make(map[string]WorkspaceID, len(c.Workspaces))
	previousWorkspace := WorkspaceID("")
	for _, record := range c.Contexts {
		if record.ContextHome == "" {
			continue
		}
		if previous, exists := workspacesByHome[record.ContextHome]; exists && previous != record.Context.ID {
			return fmt.Errorf("one Context Home may belong to only one Context")
		}
		workspacesByHome[record.ContextHome] = record.Context.ID
	}
	for _, workspace := range c.Workspaces {
		if previousWorkspace != "" && workspace.ID <= previousWorkspace {
			return fmt.Errorf("Workspaces must be unique and sorted")
		}
		record, exists := records[workspace.ContextID]
		if !exists {
			return fmt.Errorf("Workspace refers to an unavailable Context")
		}
		if err := workspace.ValidateFor(record.Context); err != nil {
			return err
		}
		if record.ContextHome == "" || record.CreationDefaults == "" || workspace.Home != record.ContextHome || workspace.CreationDefaults != record.CreationDefaults {
			return fmt.Errorf("Workspace does not match its Context Home creation authority")
		}
		pair := string(workspace.ContextID) + "\x00" + workspace.ProjectRoot
		if previous, exists := workspacesByContextRoot[pair]; exists && previous != workspace.ID {
			return fmt.Errorf("one Context may have at most one Workspace at one Project root")
		}
		if previous, exists := workspacesByHome[workspace.Home]; exists && previous != workspace.ContextID {
			return fmt.Errorf("one Context Home may belong to only one Context")
		}
		workspacesByContext[workspace.ContextID] = append(workspacesByContext[workspace.ContextID], workspace)
		workspacesByID[workspace.ID] = workspace
		workspacesByHome[workspace.Home] = workspace.ContextID
		workspacesByContextRoot[pair] = workspace.ID
		previousWorkspace = workspace.ID
	}

	for _, record := range c.Contexts {
		template := templates[record.Context.TemplateID]
		if record.CreationDefaults != "" {
			creationFound := false
			for _, revision := range template.Retained {
				if revision.Slices.CreationDefaultsDigest == record.CreationDefaults {
					creationFound = true
					break
				}
			}
			if !creationFound {
				return fmt.Errorf("Context Home creation defaults have no retained Template revision")
			}
		}
		currentRules := make(map[string]struct{}, len(record.PolicyMemory.Rules))
		for _, rule := range record.PolicyMemory.Rules {
			currentRules[rule.ID] = struct{}{}
		}
		for _, supersession := range record.SupersededAllows {
			if err := supersession.Validate(); err != nil || supersession.ContextID != record.Context.ID || supersession.TemplateID != template.ID {
				return fmt.Errorf("Policy Memory Allow supersession crosses authority")
			}
			if _, exists := currentRules[supersession.Rule.ID]; exists {
				return fmt.Errorf("superseded Policy Memory Allow remains current")
			}
		}
		for _, supersession := range record.SupersededCandidates {
			if err := supersession.Validate(); err != nil || supersession.ContextID != record.Context.ID || supersession.TemplateID != template.ID {
				return fmt.Errorf("Policy candidate supersession crosses authority")
			}
			for _, pending := range c.PendingCandidates {
				if pending.ID == supersession.Candidate.ID {
					return fmt.Errorf("superseded Policy candidate remains pending")
				}
			}
		}
		workspaces := workspacesByContext[record.Context.ID]
		for _, value := range workspaces {
			creationFound := false
			for _, revision := range template.Retained {
				if revision.Slices.CreationDefaultsDigest == value.CreationDefaults {
					creationFound = true
					break
				}
			}
			if !creationFound {
				return fmt.Errorf("Workspace creation defaults have no retained Template revision")
			}
		}
		if len(workspaces) > 1 {
			home := workspaces[0].Home
			creationDefaults := workspaces[0].CreationDefaults
			for _, workspace := range workspaces[1:] {
				if workspace.Home != home {
					return fmt.Errorf("Workspaces owned by one Context must share one Context Home")
				}
				if workspace.CreationDefaults != creationDefaults {
					return fmt.Errorf("Workspaces owned by one Context must share create-once Home defaults")
				}
			}
		}
		snapshot := ContextAuthoritySnapshot{
			Context: record.Context, Template: template, PolicyMemory: record.PolicyMemory,
			ContextHome: record.ContextHome, ContextCreationDefaults: record.CreationDefaults,
			ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
			ActivePolicyMemoryRef: record.ActivePolicyMemoryRef, Workspaces: cloneWorkspaceBindings(workspaces),
		}
		if err := snapshot.Validate(); err != nil {
			return err
		}
	}

	previousCandidate := ""
	for _, candidate := range c.PendingCandidates {
		if previousCandidate != "" && candidate.ID <= previousCandidate {
			return fmt.Errorf("Policy candidates must be unique and sorted")
		}
		if err := candidate.Validate(); err != nil {
			return err
		}
		record, contextExists := records[candidate.ContextID]
		workspace, workspaceExists := workspacesByID[candidate.ObservingWorkspaceID]
		if !contextExists || !workspaceExists || workspace.ContextID != candidate.ContextID {
			return fmt.Errorf("Policy candidate crosses Context or observing Workspace authority")
		}
		for _, existingRule := range record.PolicyMemory.Rules {
			index := sort.SearchStrings(existingRule.Body.SourceCandidates, candidate.ID)
			if index < len(existingRule.Body.SourceCandidates) && existingRule.Body.SourceCandidates[index] == candidate.ID {
				return fmt.Errorf("pending Policy candidate was already consumed by current Policy Memory")
			}
		}
		rule, err := NewPolicyMemoryRule(candidate.ContextID, PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
		if err != nil {
			return err
		}
		candidateMemory, _, err := PublishPolicyMemory(candidate.ContextID, []PolicyMemoryRule{rule}, nil)
		if err != nil {
			return err
		}
		if err := candidateMemory.ValidateFor(record.Context, templates[record.Context.TemplateID].Current); err != nil {
			return fmt.Errorf("Policy candidate exceeds its Context Template Boundary: %w", err)
		}
		previousCandidate = candidate.ID
	}

	if c.DefaultTemplateID != nil {
		if err := c.DefaultTemplateID.Validate(); err != nil {
			return err
		}
		if _, exists := templates[*c.DefaultTemplateID]; !exists {
			return fmt.Errorf("default Workspace Template is unavailable")
		}
	}
	if c.CurrentContextID != nil {
		if err := c.CurrentContextID.Validate(); err != nil {
			return err
		}
		if _, exists := records[*c.CurrentContextID]; !exists {
			return fmt.Errorf("current Context is unavailable")
		}
	}
	want, err := workspaceAuthorityCollectionRevision(c)
	if err != nil {
		return err
	}
	if c.Revision != want {
		return fmt.Errorf("Workspace authority collection revision does not bind its complete content")
	}
	return nil
}

func SupersedePolicyMemoryAllowsOutsideBoundary(record WorkspaceAuthorityContextRecord, template WorkspaceTemplateRevision) (WorkspaceAuthorityContextRecord, int, error) {
	if err := record.Context.Validate(); err != nil || template.Validate() != nil || record.Context.TemplateID != template.TemplateID {
		return WorkspaceAuthorityContextRecord{}, 0, fmt.Errorf("Policy Memory supersession target is invalid")
	}
	result := record.Clone()
	remaining := make([]PolicyMemoryRule, 0, len(record.PolicyMemory.Rules))
	removed := make([]PolicyMemoryRule, 0)
	for _, rule := range record.PolicyMemory.Rules {
		if rule.Decision != PolicyMemoryAllow {
			remaining = append(remaining, rule.Clone())
			continue
		}
		trial, _, err := PublishPolicyMemory(record.Context.ID, []PolicyMemoryRule{rule.Clone()}, nil)
		if err != nil {
			return WorkspaceAuthorityContextRecord{}, 0, err
		}
		if err := trial.ValidateFor(record.Context, template); err != nil {
			removed = append(removed, rule.Clone())
		} else {
			remaining = append(remaining, rule.Clone())
		}
	}
	if len(removed) == 0 {
		return result, 0, nil
	}
	next, changed, err := PublishPolicyMemory(record.Context.ID, remaining, &record.PolicyMemory)
	if err != nil || !changed {
		return WorkspaceAuthorityContextRecord{}, 0, fmt.Errorf("Policy Memory supersession publication failed: %w", err)
	}
	result.PolicyMemory = next
	for _, rule := range removed {
		result.SupersededAllows = append(result.SupersededAllows, PolicyMemoryAllowSupersession{SchemaVersion: 1, ContextID: record.Context.ID, Rule: rule.Clone(), SourceMemoryRevision: record.PolicyMemory.Revision, TemplateID: template.TemplateID, TemplateRevision: template.Revision})
	}
	result.ActivePolicyMemory = nil
	result.ActivePolicyMemoryRef = nil
	return result, len(removed), nil
}

// SupersedePolicyCandidatesOutsideBoundary removes only pending, ungranted
// candidate proposals that the new Boundary can no longer admit. Exact
// evidence is retained on its Context owner; no Policy Memory revision or
// active receipt is changed by this transition.
func SupersedePolicyCandidatesOutsideBoundary(records []WorkspaceAuthorityContextRecord, candidates []PolicyCandidateAuthority, template WorkspaceTemplateRevision) ([]WorkspaceAuthorityContextRecord, []PolicyCandidateAuthority, int, error) {
	if template.Validate() != nil || records == nil || candidates == nil {
		return nil, nil, 0, fmt.Errorf("Policy candidate supersession input is invalid")
	}
	resultRecords := cloneWorkspaceAuthorityContextRecords(records)
	indexByContext := make(map[ContextID]int, len(resultRecords))
	for index := range resultRecords {
		indexByContext[resultRecords[index].Context.ID] = index
	}
	remaining := make([]PolicyCandidateAuthority, 0, len(candidates))
	removed := 0
	for _, candidate := range candidates {
		index, exists := indexByContext[candidate.ContextID]
		if !exists || resultRecords[index].Context.TemplateID != template.TemplateID {
			remaining = append(remaining, candidate.Clone())
			continue
		}
		rule, err := NewPolicyMemoryRule(candidate.ContextID, PolicyMemoryAllow, candidate.Effect.RuleBody(candidate.ID))
		if err != nil {
			return nil, nil, 0, err
		}
		trial, _, err := PublishPolicyMemory(candidate.ContextID, []PolicyMemoryRule{rule}, nil)
		if err != nil {
			return nil, nil, 0, err
		}
		if err := trial.ValidateFor(resultRecords[index].Context, template); err == nil {
			remaining = append(remaining, candidate.Clone())
			continue
		}
		resultRecords[index].SupersededCandidates = append(resultRecords[index].SupersededCandidates, PolicyCandidateSupersession{
			SchemaVersion: 1, ContextID: candidate.ContextID, Candidate: candidate.Clone(),
			TemplateID: template.TemplateID, TemplateRevision: template.Revision,
		})
		removed++
	}
	return resultRecords, remaining, removed, nil
}

func (c WorkspaceAuthorityCollection) Clone() WorkspaceAuthorityCollection {
	result := c
	result.Templates = cloneWorkspaceTemplates(c.Templates)
	result.Contexts = cloneWorkspaceAuthorityContextRecords(c.Contexts)
	result.Workspaces = cloneWorkspaceBindings(c.Workspaces)
	result.PendingCandidates = clonePolicyCandidateAuthorities(c.PendingCandidates)
	if c.DefaultTemplateID != nil {
		value := *c.DefaultTemplateID
		result.DefaultTemplateID = &value
	}
	if c.CurrentContextID != nil {
		value := *c.CurrentContextID
		result.CurrentContextID = &value
	}
	return result
}

func (c WorkspaceAuthorityCollection) ContextSnapshots() ([]ContextAuthoritySnapshot, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(c.Templates))
	for _, template := range c.Templates {
		templates[template.ID] = template
	}
	workspaces := make(map[ContextID][]WorkspaceBinding, len(c.Contexts))
	for _, workspace := range c.Workspaces {
		workspaces[workspace.ContextID] = append(workspaces[workspace.ContextID], workspace)
	}
	result := make([]ContextAuthoritySnapshot, len(c.Contexts))
	for index, record := range c.Contexts {
		result[index] = ContextAuthoritySnapshot{
			Context: record.Context, Template: templates[record.Context.TemplateID].Clone(), PolicyMemory: record.PolicyMemory.Clone(),
			ContextHome: record.ContextHome, ContextCreationDefaults: record.CreationDefaults,
			ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
			ActivePolicyMemoryRef: record.ActivePolicyMemoryRef, Workspaces: cloneWorkspaceBindings(workspaces[record.Context.ID]),
		}
		if len(result[index].Workspaces) == 1 {
			value := result[index].Workspaces[0]
			result[index].Workspace = &value
		}
		result[index] = result[index].Clone()
	}
	return result, nil
}

func cloneWorkspaceTemplates(values []WorkspaceTemplate) []WorkspaceTemplate {
	result := make([]WorkspaceTemplate, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}

func cloneWorkspaceAuthorityContextRecords(values []WorkspaceAuthorityContextRecord) []WorkspaceAuthorityContextRecord {
	result := make([]WorkspaceAuthorityContextRecord, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}

func cloneWorkspaceBindings(values []WorkspaceBinding) []WorkspaceBinding {
	result := make([]WorkspaceBinding, len(values))
	for index, value := range values {
		result[index] = value
		if value.LastSuccessfulEntry != nil {
			entry := *value.LastSuccessfulEntry
			result[index].LastSuccessfulEntry = &entry
		}
	}
	return result
}

func clonePolicyCandidateAuthorities(values []PolicyCandidateAuthority) []PolicyCandidateAuthority {
	result := make([]PolicyCandidateAuthority, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}
