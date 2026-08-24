package tobari

import (
	"fmt"
	"sort"
)

const WorkspaceAuthorityCollectionSchemaVersion = 1

// WorkspaceAuthorityContextRecord is the Context-owned portion of one final
// authority envelope. Template and Workspace values remain normalized so one
// installation snapshot cannot carry divergent copies of either authority.
type WorkspaceAuthorityContextRecord struct {
	Context               ContextBinding                   `json:"context"`
	PolicyMemory          PolicyMemoryRevision             `json:"policy_memory"`
	ActiveTemplatePolicy  *TemplatePolicyActivationReceipt `json:"active_template_policy,omitempty"`
	ActivePolicyMemory    *PolicyMemoryRevision            `json:"active_policy_memory,omitempty"`
	ActivePolicyMemoryRef *PolicyMemoryActivationReceipt   `json:"active_policy_memory_receipt,omitempty"`
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
}

type workspaceAuthorityCollectionContent struct {
	Templates         []WorkspaceTemplate
	Contexts          []WorkspaceAuthorityContextRecord
	Workspaces        []WorkspaceBinding
	PendingCandidates []PolicyCandidateAuthority
	DefaultTemplateID *WorkspaceTemplateID
}

func PublishWorkspaceAuthorityCollection(
	templates []WorkspaceTemplate,
	contexts []WorkspaceAuthorityContextRecord,
	workspaces []WorkspaceBinding,
	candidates []PolicyCandidateAuthority,
	defaultTemplateID *WorkspaceTemplateID,
	previous *WorkspaceAuthorityCollection,
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
	if defaultTemplateID != nil {
		value := *defaultTemplateID
		result.DefaultTemplateID = &value
	}
	sort.Slice(result.Templates, func(i, j int) bool { return result.Templates[i].ID < result.Templates[j].ID })
	sort.Slice(result.Contexts, func(i, j int) bool { return result.Contexts[i].Context.ID < result.Contexts[j].Context.ID })
	sort.Slice(result.Workspaces, func(i, j int) bool { return result.Workspaces[i].ID < result.Workspaces[j].ID })
	sort.Slice(result.PendingCandidates, func(i, j int) bool { return result.PendingCandidates[i].ID < result.PendingCandidates[j].ID })
	if previous != nil {
		if err := previous.Validate(); err != nil {
			return WorkspaceAuthorityCollection{}, false, err
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

func workspaceAuthorityCollectionRevision(collection WorkspaceAuthorityCollection) (SemanticDigest, error) {
	content := workspaceAuthorityCollectionContent{
		Templates: collection.Templates, Contexts: collection.Contexts, Workspaces: collection.Workspaces,
		PendingCandidates: collection.PendingCandidates, DefaultTemplateID: collection.DefaultTemplateID,
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
		records[record.Context.ID] = record
		previousContext = record.Context.ID
	}
	if err := ValidateContextBindings(bindings); err != nil {
		return err
	}

	workspacesByContext := make(map[ContextID]WorkspaceBinding, len(c.Workspaces))
	workspacesByID := make(map[WorkspaceID]WorkspaceBinding, len(c.Workspaces))
	workspacesByHome := make(map[string]WorkspaceID, len(c.Workspaces))
	previousWorkspace := WorkspaceID("")
	for _, workspace := range c.Workspaces {
		if previousWorkspace != "" && workspace.ID <= previousWorkspace {
			return fmt.Errorf("Workspaces must be unique and sorted")
		}
		record, exists := records[workspace.ContextID]
		if !exists {
			return fmt.Errorf("Workspace refers to an unavailable Context")
		}
		if _, exists := workspacesByContext[workspace.ContextID]; exists {
			return fmt.Errorf("one Context may have at most one Workspace")
		}
		if err := workspace.ValidateFor(record.Context); err != nil {
			return err
		}
		if previous, exists := workspacesByHome[workspace.Home]; exists && previous != workspace.ID {
			return fmt.Errorf("one Workspace home may belong to only one Workspace")
		}
		workspacesByContext[workspace.ContextID] = workspace
		workspacesByID[workspace.ID] = workspace
		workspacesByHome[workspace.Home] = workspace.ID
		previousWorkspace = workspace.ID
	}

	for _, record := range c.Contexts {
		template := templates[record.Context.TemplateID]
		var workspace *WorkspaceBinding
		if value, exists := workspacesByContext[record.Context.ID]; exists {
			copy := value
			workspace = &copy
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
		snapshot := ContextAuthoritySnapshot{
			Context: record.Context, Template: template, PolicyMemory: record.PolicyMemory,
			ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
			ActivePolicyMemoryRef: record.ActivePolicyMemoryRef, Workspace: workspace,
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
	want, err := workspaceAuthorityCollectionRevision(c)
	if err != nil {
		return err
	}
	if c.Revision != want {
		return fmt.Errorf("Workspace authority collection revision does not bind its complete content")
	}
	return nil
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
	workspaces := make(map[ContextID]WorkspaceBinding, len(c.Workspaces))
	for _, workspace := range c.Workspaces {
		workspaces[workspace.ContextID] = workspace
	}
	result := make([]ContextAuthoritySnapshot, len(c.Contexts))
	for index, record := range c.Contexts {
		result[index] = ContextAuthoritySnapshot{
			Context: record.Context, Template: templates[record.Context.TemplateID].Clone(), PolicyMemory: record.PolicyMemory.Clone(),
			ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
			ActivePolicyMemoryRef: record.ActivePolicyMemoryRef,
		}
		if workspace, exists := workspaces[record.Context.ID]; exists {
			value := workspace
			if workspace.LastSuccessfulEntry != nil {
				entry := *workspace.LastSuccessfulEntry
				value.LastSuccessfulEntry = &entry
			}
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
	result := append([]WorkspaceBinding{}, values...)
	for index := range result {
		if values[index].LastSuccessfulEntry != nil {
			entry := *values[index].LastSuccessfulEntry
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
