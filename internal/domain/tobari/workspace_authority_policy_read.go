package tobari

import (
	"fmt"
	"reflect"
	"sort"
)

const WorkspaceAuthorityPolicyReadSchemaVersion = 3

// PolicyCandidateAuthorityView is one actionable pending candidate joined to
// its final Context, Template, and observing Workspace references. Raw final
// IDs remain private validation evidence rather than public compatibility
// aliases.
type PolicyCandidateAuthorityView struct {
	ID                    string                   `json:"id"`
	Context               string                   `json:"context"`
	Template              string                   `json:"template"`
	ProjectRoot           string                   `json:"project_root"`
	ObservingWorkspace    string                   `json:"observing_workspace"`
	ContextRef            string                   `json:"-"`
	TemplateRef           string                   `json:"-"`
	ObservingWorkspaceRef string                   `json:"-"`
	Effect                PolicyCandidateEffect    `json:"effect"`
	Authority             PolicyCandidateAuthority `json:"-"`
	AttachmentAuthority   *PolicyCandidate         `json:"-"`
	ContextAuthority      ContextBinding           `json:"-"`
	ContextID             ContextID                `json:"context_id"`
	TemplateID            WorkspaceTemplateID      `json:"workspace_template_id"`
	ObservingWorkspaceID  WorkspaceID              `json:"observing_workspace_id"`
	TemplateName          string                   `json:"-"`
	ObservingProjectRoot  string                   `json:"-"`
}

func (v PolicyCandidateAuthorityView) Clone() PolicyCandidateAuthorityView {
	result := v
	result.Effect = v.Effect.Clone()
	result.Authority = v.Authority.Clone()
	if v.AttachmentAuthority != nil {
		attachment := *v.AttachmentAuthority
		result.AttachmentAuthority = &attachment
	}
	return result
}

func (v PolicyCandidateAuthorityView) Validate() error {
	if v.AttachmentAuthority != nil {
		return v.validateAttachment()
	}
	if err := v.Authority.Validate(); err != nil {
		return err
	}
	contextRef, contextErr := ContextRef(v.Authority.ContextID)
	templateRef, templateErr := WorkspaceTemplateRef(v.TemplateID)
	workspaceRef, workspaceErr := WorkspaceRef(v.Authority.ObservingWorkspaceID)
	if contextErr != nil || templateErr != nil || workspaceErr != nil || v.ContextAuthority.Validate() != nil ||
		v.ContextID != v.Authority.ContextID || v.ObservingWorkspaceID != v.Authority.ObservingWorkspaceID ||
		v.ContextAuthority.ID != v.ContextID || v.ContextAuthority.TemplateID != v.TemplateID ||
		ValidateName(v.TemplateName) != nil || ValidateCanonicalRoot(v.ObservingProjectRoot) != nil ||
		v.Context != v.TemplateName || v.Template != v.TemplateName || v.ProjectRoot != v.ObservingProjectRoot ||
		v.ObservingWorkspace != v.ObservingProjectRoot ||
		v.ID != v.Authority.ID || v.ContextRef != contextRef || v.TemplateRef != templateRef ||
		v.ObservingWorkspaceRef != workspaceRef || !reflect.DeepEqual(v.Effect, v.Authority.Effect) {
		return fmt.Errorf("Policy candidate view does not bind its exact final authority")
	}
	return nil
}

func (v PolicyCandidateAuthorityView) validateAttachment() error {
	attachment := *v.AttachmentAuthority
	if err := attachment.Validate(); err != nil {
		return err
	}
	contextID := ContextID(attachment.WorkspaceManifestID)
	workspaceID := WorkspaceID(attachment.ProjectID)
	contextRef, contextErr := ContextRef(contextID)
	templateRef, templateErr := WorkspaceTemplateRef(v.TemplateID)
	workspaceRef, workspaceErr := WorkspaceRef(workspaceID)
	wantEffect := PolicyCandidateEffect{
		PolicyProtocolIdentity: attachment.PolicyProtocolIdentity,
		Match:                  PolicyMatchExact, Host: attachment.Host, Port: attachment.Port,
		Method: attachment.Method, Path: attachment.Path,
		Segments: []string{}, Examples: []string{attachment.Path},
	}
	if contextErr != nil || templateErr != nil || workspaceErr != nil || v.ContextAuthority.Validate() != nil ||
		attachment.EffectiveDestinationKind() != PolicyDestinationHostLoopback ||
		attachment.EffectiveAuthorityLifetime() != AuthorityLifetimeAttachment ||
		contextID != v.ContextID || workspaceID != v.ObservingWorkspaceID ||
		v.ContextAuthority.ID != contextID || v.ContextAuthority.TemplateID != v.TemplateID ||
		ValidateName(v.TemplateName) != nil || attachment.WorkspaceManifestName != v.TemplateName ||
		ValidateCanonicalRoot(v.ObservingProjectRoot) != nil || attachment.ProjectRoot != v.ObservingProjectRoot ||
		v.Context != v.TemplateName || v.Template != v.TemplateName || v.ProjectRoot != v.ObservingProjectRoot ||
		v.ObservingWorkspace != v.ObservingProjectRoot || v.ID != attachment.ID ||
		v.ContextRef != contextRef || v.TemplateRef != templateRef || v.ObservingWorkspaceRef != workspaceRef ||
		!reflect.DeepEqual(v.Effect, wantEffect) || v.Authority.ID != "" {
		return fmt.Errorf("Policy candidate view does not bind its attachment-local authority")
	}
	return nil
}

// PolicyCandidateAuthorityList is exhaustive for one coherent final-envelope
// observation. CollectionPresent distinguishes a genuinely fresh empty final
// installation from an initialized collection whose pending set is empty.
type PolicyCandidateAuthorityList struct {
	SchemaVersion        int                            `json:"schema_version"`
	Task                 string                         `json:"task"`
	Items                []PolicyCandidateAuthorityView `json:"items"`
	CollectionPresent    bool                           `json:"-"`
	CollectionGeneration uint64                         `json:"-"`
	CollectionRevision   SemanticDigest                 `json:"-"`
}

func NewPolicyCandidateAuthorityList(collection WorkspaceAuthorityCollection, present bool) (PolicyCandidateAuthorityList, error) {
	result, err := newPolicyCandidateAuthorityList(collection, present)
	if err != nil {
		return PolicyCandidateAuthorityList{}, err
	}
	return result, result.Validate()
}

// NewPolicyCandidateAuthorityListWithAttachments joins active attachment-local
// Host Loopback candidates to the same final Context/Template/Workspace
// presentation used by persistent candidates. The attachment authority stays
// out of the final collection and Policy Memory.
func NewPolicyCandidateAuthorityListWithAttachments(
	collection WorkspaceAuthorityCollection,
	present bool,
	attachments []PolicyCandidate,
) (PolicyCandidateAuthorityList, error) {
	return NewPolicyCandidateAuthorityListWithObservations(collection, present, nil, attachments)
}

// NewPolicyCandidateAuthorityListWithObservations joins current final
// authority candidates with bounded Gateway observations. Observed persistent
// candidates are read-only evidence and are never added to the durable final
// envelope; only an explicit policy mutation can consume one. Attachment
// candidates remain a separate Host Loopback authority branch.
func NewPolicyCandidateAuthorityListWithObservations(
	collection WorkspaceAuthorityCollection,
	present bool,
	observed []PolicyCandidateAuthority,
	attachments []PolicyCandidate,
) (PolicyCandidateAuthorityList, error) {
	result, err := newPolicyCandidateAuthorityList(collection, present)
	if err != nil {
		return PolicyCandidateAuthorityList{}, err
	}
	if len(observed) == 0 && len(attachments) == 0 {
		return result, result.Validate()
	}
	if !present {
		return PolicyCandidateAuthorityList{}, fmt.Errorf("observed candidates require final authority")
	}
	contexts := make(map[ContextID]ContextBinding, len(collection.Contexts))
	templates := make(map[WorkspaceTemplateID]string, len(collection.Templates))
	workspaces := make(map[WorkspaceID]WorkspaceBinding, len(collection.Workspaces))
	for _, record := range collection.Contexts {
		contexts[record.Context.ID] = record.Context
	}
	for _, template := range collection.Templates {
		templates[template.ID] = template.Name
	}
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ID] = workspace
	}
	seen := make(map[string]struct{}, len(result.Items))
	seenEffects := make(map[string]struct{}, len(result.Items))
	for _, item := range result.Items {
		seen[item.ID] = struct{}{}
		seenEffects[policyCandidateAuthorityEffectKey(item.Authority)] = struct{}{}
	}
	observed = clonePolicyCandidateAuthorities(observed)
	sort.Slice(observed, func(i, j int) bool {
		if observed[i].ID != observed[j].ID {
			return observed[i].ID < observed[j].ID
		}
		return observed[i].ObservingWorkspaceID < observed[j].ObservingWorkspaceID
	})
	for index := range observed {
		candidate := observed[index]
		if err := candidate.Validate(); err != nil {
			return PolicyCandidateAuthorityList{}, err
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			continue
		}
		if _, duplicate := seenEffects[policyCandidateAuthorityEffectKey(candidate)]; duplicate {
			continue
		}
		binding, contextFound := contexts[candidate.ContextID]
		workspace, workspaceFound := workspaces[candidate.ObservingWorkspaceID]
		templateName, templateFound := templates[binding.TemplateID]
		if !contextFound || !workspaceFound || !templateFound || workspace.ContextID != candidate.ContextID {
			return PolicyCandidateAuthorityList{}, fmt.Errorf("observed candidate does not belong to current final authority")
		}
		contextRef, contextErr := ContextRef(candidate.ContextID)
		templateRef, templateErr := WorkspaceTemplateRef(binding.TemplateID)
		workspaceRef, workspaceErr := WorkspaceRef(candidate.ObservingWorkspaceID)
		if contextErr != nil || templateErr != nil || workspaceErr != nil {
			return PolicyCandidateAuthorityList{}, fmt.Errorf("observed candidate references are invalid")
		}
		result.Items = append(result.Items, PolicyCandidateAuthorityView{
			ID: candidate.ID, Context: templateName, Template: templateName, ProjectRoot: workspace.ProjectRoot,
			ObservingWorkspace: workspace.ProjectRoot, ContextRef: contextRef, TemplateRef: templateRef,
			ObservingWorkspaceRef: workspaceRef, Effect: candidate.Effect.Clone(), Authority: candidate.Clone(),
			ContextAuthority: binding, ContextID: candidate.ContextID, TemplateID: binding.TemplateID,
			ObservingWorkspaceID: candidate.ObservingWorkspaceID, TemplateName: templateName,
			ObservingProjectRoot: workspace.ProjectRoot,
		})
		seen[candidate.ID] = struct{}{}
		seenEffects[policyCandidateAuthorityEffectKey(candidate)] = struct{}{}
	}
	for index := range attachments {
		candidate := attachments[index]
		if err := candidate.Validate(); err != nil {
			return PolicyCandidateAuthorityList{}, err
		}
		contextID := ContextID(candidate.WorkspaceManifestID)
		workspaceID := WorkspaceID(candidate.ProjectID)
		binding, contextFound := contexts[contextID]
		workspace, workspaceFound := workspaces[workspaceID]
		templateName, templateFound := templates[binding.TemplateID]
		if !contextFound || !workspaceFound || !templateFound || workspace.ContextID != contextID ||
			candidate.ProjectRoot != workspace.ProjectRoot || candidate.WorkspaceManifestName != templateName {
			return PolicyCandidateAuthorityList{}, fmt.Errorf("attachment-local candidate does not belong to current final authority")
		}
		if _, duplicate := seen[candidate.ID]; duplicate {
			return PolicyCandidateAuthorityList{}, fmt.Errorf("observed candidate IDs are ambiguous")
		}
		contextRef, _ := ContextRef(contextID)
		templateRef, _ := WorkspaceTemplateRef(binding.TemplateID)
		workspaceRef, _ := WorkspaceRef(workspaceID)
		attachment := candidate
		result.Items = append(result.Items, PolicyCandidateAuthorityView{
			ID: candidate.ID, Context: templateName, Template: templateName, ProjectRoot: workspace.ProjectRoot,
			ObservingWorkspace: workspace.ProjectRoot, ContextRef: contextRef, TemplateRef: templateRef,
			ObservingWorkspaceRef: workspaceRef,
			Effect: PolicyCandidateEffect{
				PolicyProtocolIdentity: candidate.PolicyProtocolIdentity,
				Match:                  PolicyMatchExact, Host: candidate.Host, Port: candidate.Port,
				Method: candidate.Method, Path: candidate.Path,
				Segments: []string{}, Examples: []string{candidate.Path},
			},
			AttachmentAuthority: &attachment, ContextAuthority: binding,
			ContextID: contextID, TemplateID: binding.TemplateID, ObservingWorkspaceID: workspaceID,
			TemplateName: templateName, ObservingProjectRoot: workspace.ProjectRoot,
		})
		seen[candidate.ID] = struct{}{}
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].ID < result.Items[j].ID })
	return result, result.Validate()
}

func (l PolicyCandidateAuthorityList) Clone() PolicyCandidateAuthorityList {
	result := l
	result.Items = make([]PolicyCandidateAuthorityView, len(l.Items))
	for index := range l.Items {
		result.Items[index] = l.Items[index].Clone()
	}
	return result
}

func (l PolicyCandidateAuthorityList) Validate() error {
	if l.SchemaVersion != WorkspaceAuthorityPolicyReadSchemaVersion || l.Task != TaskPolicyCandidates || l.Items == nil {
		return fmt.Errorf("Policy candidate list metadata is invalid")
	}
	if !l.CollectionPresent {
		if l.CollectionGeneration != 0 || l.CollectionRevision != "" || len(l.Items) != 0 {
			return fmt.Errorf("fresh final Policy candidate list carries initialized authority")
		}
		return nil
	}
	if l.CollectionGeneration == 0 || l.CollectionRevision.Validate() != nil {
		return fmt.Errorf("Policy candidate list collection receipt is invalid")
	}
	previous := ""
	for _, item := range l.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if previous != "" && item.ID <= previous {
			return fmt.Errorf("Policy candidate views must be unique and sorted")
		}
		previous = item.ID
	}
	return nil
}

func (l PolicyCandidateAuthorityList) ValidateFor(collection WorkspaceAuthorityCollection, present bool) error {
	if err := l.Validate(); err != nil {
		return err
	}
	want, err := newPolicyCandidateAuthorityList(collection, present)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(l, want) {
		return fmt.Errorf("Policy candidate list does not match its complete final collection")
	}
	return nil
}

func newPolicyCandidateAuthorityList(collection WorkspaceAuthorityCollection, present bool) (PolicyCandidateAuthorityList, error) {
	// Keep the exact constructor logic available to ValidateFor without
	// recursively validating the result against itself.
	result := PolicyCandidateAuthorityList{SchemaVersion: WorkspaceAuthorityPolicyReadSchemaVersion, Task: TaskPolicyCandidates, Items: []PolicyCandidateAuthorityView{}, CollectionPresent: present}
	if !present {
		return result, nil
	}
	if err := collection.Validate(); err != nil {
		return PolicyCandidateAuthorityList{}, err
	}
	contexts := make(map[ContextID]WorkspaceTemplateID, len(collection.Contexts))
	contextAuthority := make(map[ContextID]ContextBinding, len(collection.Contexts))
	templateNames := make(map[WorkspaceTemplateID]string, len(collection.Templates))
	workspaceRoots := make(map[WorkspaceID]string, len(collection.Workspaces))
	for _, template := range collection.Templates {
		templateNames[template.ID] = template.Name
	}
	for _, record := range collection.Contexts {
		contexts[record.Context.ID] = record.Context.TemplateID
		contextAuthority[record.Context.ID] = record.Context
	}
	for _, workspace := range collection.Workspaces {
		workspaceRoots[workspace.ID] = workspace.ProjectRoot
	}
	result.CollectionGeneration, result.CollectionRevision = collection.Generation, collection.Revision
	result.Items = make([]PolicyCandidateAuthorityView, 0, len(collection.PendingCandidates))
	seenEffects := make(map[string]struct{}, len(collection.PendingCandidates))
	for _, candidate := range collection.PendingCandidates {
		if _, duplicate := seenEffects[policyCandidateAuthorityEffectKey(candidate)]; duplicate {
			continue
		}
		contextRef, _ := ContextRef(candidate.ContextID)
		templateRef, _ := WorkspaceTemplateRef(contexts[candidate.ContextID])
		workspaceRef, _ := WorkspaceRef(candidate.ObservingWorkspaceID)
		name := templateNames[contexts[candidate.ContextID]]
		root := workspaceRoots[candidate.ObservingWorkspaceID]
		result.Items = append(result.Items, PolicyCandidateAuthorityView{
			ID: candidate.ID, Context: name, Template: name, ProjectRoot: root, ObservingWorkspace: root,
			ContextRef: contextRef, TemplateRef: templateRef, ObservingWorkspaceRef: workspaceRef,
			Effect: candidate.Effect.Clone(), Authority: candidate.Clone(), ContextAuthority: contextAuthority[candidate.ContextID],
			ContextID: candidate.ContextID, TemplateID: contexts[candidate.ContextID], ObservingWorkspaceID: candidate.ObservingWorkspaceID,
			TemplateName: name, ObservingProjectRoot: root,
		})
		seenEffects[policyCandidateAuthorityEffectKey(candidate)] = struct{}{}
	}
	return result, nil
}

// PolicyMemoryRuleView is one current Context-owned remembered rule. A rule's
// source candidate IDs are historical evidence, not actionable references;
// the final Context and Template references are the complete current owner
// dimensions that can be derived from the envelope without predecessor data.
type PolicyMemoryRuleView struct {
	ID               string               `json:"id"`
	Decision         PolicyMemoryDecision `json:"decision"`
	Match            string               `json:"match"`
	Context          string               `json:"context"`
	Template         string               `json:"template"`
	ContextRef       string               `json:"-"`
	TemplateRef      string               `json:"-"`
	Body             PolicyMemoryRuleBody `json:"body"`
	Rule             PolicyMemoryRule     `json:"-"`
	ContextAuthority ContextBinding       `json:"-"`
	ContextID        ContextID            `json:"context_id"`
	TemplateID       WorkspaceTemplateID  `json:"workspace_template_id"`
	TemplateName     string               `json:"-"`
}

func (v PolicyMemoryRuleView) Clone() PolicyMemoryRuleView {
	result := v
	result.Body = v.Body.Clone()
	result.Rule = v.Rule.Clone()
	return result
}

func (v PolicyMemoryRuleView) Validate() error {
	if err := v.Rule.Validate(v.ContextID); err != nil {
		return err
	}
	contextRef, contextErr := ContextRef(v.ContextID)
	templateRef, templateErr := WorkspaceTemplateRef(v.TemplateID)
	if contextErr != nil || templateErr != nil || v.ContextAuthority.Validate() != nil ||
		v.ContextAuthority.ID != v.ContextID || v.ContextAuthority.TemplateID != v.TemplateID ||
		ValidateName(v.TemplateName) != nil ||
		v.Context != v.TemplateName || v.Template != v.TemplateName ||
		v.ID != v.Rule.ID || v.Decision != v.Rule.Decision ||
		v.Match != v.Rule.Body.Match || v.ContextRef != contextRef || v.TemplateRef != templateRef ||
		!reflect.DeepEqual(v.Body, v.Rule.Body) {
		return fmt.Errorf("Policy Memory rule view does not bind its exact final authority")
	}
	return nil
}

type PolicyMemoryRuleList struct {
	SchemaVersion        int                    `json:"schema_version"`
	Task                 string                 `json:"task"`
	Items                []PolicyMemoryRuleView `json:"items"`
	CollectionPresent    bool                   `json:"-"`
	CollectionGeneration uint64                 `json:"-"`
	CollectionRevision   SemanticDigest         `json:"-"`
}

func NewPolicyMemoryRuleList(collection WorkspaceAuthorityCollection, present bool) (PolicyMemoryRuleList, error) {
	result, err := newPolicyMemoryRuleList(collection, present)
	if err != nil {
		return PolicyMemoryRuleList{}, err
	}
	return result, result.Validate()
}

func newPolicyMemoryRuleList(collection WorkspaceAuthorityCollection, present bool) (PolicyMemoryRuleList, error) {
	result := PolicyMemoryRuleList{SchemaVersion: WorkspaceAuthorityPolicyReadSchemaVersion, Task: TaskPolicyRules, Items: []PolicyMemoryRuleView{}, CollectionPresent: present}
	if !present {
		return result, nil
	}
	if err := collection.Validate(); err != nil {
		return PolicyMemoryRuleList{}, err
	}
	result.CollectionGeneration, result.CollectionRevision = collection.Generation, collection.Revision
	templateNames := make(map[WorkspaceTemplateID]string, len(collection.Templates))
	for _, template := range collection.Templates {
		templateNames[template.ID] = template.Name
	}
	for _, record := range collection.Contexts {
		contextRef, _ := ContextRef(record.Context.ID)
		templateRef, _ := WorkspaceTemplateRef(record.Context.TemplateID)
		for _, rule := range record.PolicyMemory.Rules {
			name := templateNames[record.Context.TemplateID]
			result.Items = append(result.Items, PolicyMemoryRuleView{
				ID: rule.ID, Decision: rule.Decision, Match: rule.Body.Match,
				Context: name, Template: name,
				ContextRef: contextRef, TemplateRef: templateRef, Body: rule.Body.Clone(), Rule: rule.Clone(),
				ContextAuthority: record.Context, ContextID: record.Context.ID, TemplateID: record.Context.TemplateID, TemplateName: name,
			})
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].ContextRef == result.Items[j].ContextRef {
			return result.Items[i].ID < result.Items[j].ID
		}
		return result.Items[i].ContextRef < result.Items[j].ContextRef
	})
	return result, nil
}

func (l PolicyMemoryRuleList) Clone() PolicyMemoryRuleList {
	result := l
	result.Items = make([]PolicyMemoryRuleView, len(l.Items))
	for index := range l.Items {
		result.Items[index] = l.Items[index].Clone()
	}
	return result
}

func (l PolicyMemoryRuleList) Validate() error {
	if l.SchemaVersion != WorkspaceAuthorityPolicyReadSchemaVersion || l.Task != TaskPolicyRules || l.Items == nil {
		return fmt.Errorf("Policy Memory rule list metadata is invalid")
	}
	if !l.CollectionPresent {
		if l.CollectionGeneration != 0 || l.CollectionRevision != "" || len(l.Items) != 0 {
			return fmt.Errorf("fresh final Policy Memory rule list carries initialized authority")
		}
		return nil
	}
	if l.CollectionGeneration == 0 || l.CollectionRevision.Validate() != nil {
		return fmt.Errorf("Policy Memory rule list collection receipt is invalid")
	}
	previous := ""
	for _, item := range l.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		key := item.ContextRef + "\x00" + item.ID
		if previous != "" && key <= previous {
			return fmt.Errorf("Policy Memory rule views must be unique and sorted")
		}
		previous = key
	}
	return nil
}

func (l PolicyMemoryRuleList) ValidateFor(collection WorkspaceAuthorityCollection, present bool) error {
	if err := l.Validate(); err != nil {
		return err
	}
	want, err := newPolicyMemoryRuleList(collection, present)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(l, want) {
		return fmt.Errorf("Policy Memory rule list does not match its complete final collection")
	}
	return nil
}
