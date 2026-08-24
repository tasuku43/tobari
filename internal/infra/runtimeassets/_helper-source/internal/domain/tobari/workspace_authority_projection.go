package tobari

import (
	"fmt"
	"reflect"
	"sort"
)

const WorkspacePolicyProjectionSchemaVersion = 1

type WorkspacePolicyProjectionMode string

const (
	WorkspacePolicyProjectionHotMemory WorkspacePolicyProjectionMode = "hot_policy_memory"
	WorkspacePolicyProjectionCluster   WorkspacePolicyProjectionMode = "cluster_reconciliation"
)

func (m WorkspacePolicyProjectionMode) Validate() error {
	switch m {
	case WorkspacePolicyProjectionHotMemory, WorkspacePolicyProjectionCluster:
		return nil
	default:
		return fmt.Errorf("Workspace policy projection mode is invalid")
	}
}

// WorkspaceTemplatePolicyAuthority is the complete static policy slice selected
// for one Context. It intentionally omits the overall Template revision: policy
// activation is authoritative by TemplateID plus semantic policy-slice digest,
// while generation and unrelated Template slices remain correlation only.
type WorkspaceTemplatePolicyAuthority struct {
	TemplateID        WorkspaceTemplateID         `json:"workspace_template_id"`
	PolicySliceDigest SemanticDigest              `json:"policy_slice_digest"`
	Boundary          WorkspaceTemplateBoundary   `json:"boundary"`
	Policy            WorkspaceTemplatePolicyBody `json:"policy"`
}

func NewWorkspaceTemplatePolicyAuthority(revision WorkspaceTemplateRevision) (WorkspaceTemplatePolicyAuthority, error) {
	if err := revision.Validate(); err != nil {
		return WorkspaceTemplatePolicyAuthority{}, err
	}
	result := WorkspaceTemplatePolicyAuthority{
		TemplateID: revision.TemplateID, PolicySliceDigest: revision.Slices.PolicySliceDigest,
		Boundary: revision.Body.Boundary.Clone(), Policy: revision.Body.Policy.Clone(),
	}
	return result, result.Validate()
}

func (a WorkspaceTemplatePolicyAuthority) Validate() error {
	if err := a.TemplateID.Validate(); err != nil {
		return err
	}
	if err := a.Boundary.Validate(); err != nil {
		return err
	}
	if err := a.Policy.Validate(a.Boundary); err != nil {
		return err
	}
	boundary, err := semanticIdentity(a.Boundary)
	if err != nil {
		return err
	}
	want, err := semanticIdentity(struct {
		Boundary SemanticDigest
		Body     WorkspaceTemplatePolicyBody
	}{boundary, a.Policy})
	if err != nil {
		return err
	}
	if a.PolicySliceDigest != want {
		return fmt.Errorf("Workspace Template policy authority digest is inconsistent")
	}
	return nil
}

func (a WorkspaceTemplatePolicyAuthority) Clone() WorkspaceTemplatePolicyAuthority {
	result := a
	result.Boundary = a.Boundary.Clone()
	result.Policy = a.Policy.Clone()
	return result
}

// WorkspacePolicyPrincipalAuthority is the stable final identity projected to
// the frozen Gateway/session wire. Network and IP evidence are deliberately
// absent: infrastructure must observe and bind those values from exact owned
// Docker state before publishing a principal row.
type WorkspacePolicyPrincipalAuthority struct {
	ContextID              ContextID                         `json:"context_id"`
	WorkspaceID            WorkspaceID                       `json:"workspace_id"`
	TemplateID             WorkspaceTemplateID               `json:"workspace_template_id"`
	Presentation           string                            `json:"presentation"`
	ProjectRoot            string                            `json:"project_root"`
	AppliedEntry           WorkspaceAppliedEntry             `json:"applied_entry"`
	CreationDefaultsDigest SemanticDigest                    `json:"creation_defaults_digest"`
	CreationDefaults       WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
}

func (a WorkspacePolicyPrincipalAuthority) Validate() error {
	if err := a.ContextID.Validate(); err != nil {
		return err
	}
	if err := a.WorkspaceID.Validate(); err != nil {
		return err
	}
	if err := a.TemplateID.Validate(); err != nil {
		return err
	}
	if err := ValidateName(a.Presentation); err != nil {
		return fmt.Errorf("Workspace principal presentation: %w", err)
	}
	if err := ValidateCanonicalRoot(a.ProjectRoot); err != nil {
		return err
	}
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: a.ContextID, ProjectRoot: a.ProjectRoot, TemplateID: a.TemplateID}
	if err := a.AppliedEntry.ValidateFor(binding); err != nil {
		return err
	}
	want, err := semanticIdentity(a.CreationDefaults)
	if err != nil {
		return err
	}
	if a.CreationDefaultsDigest != want {
		return fmt.Errorf("Workspace principal creation defaults authority is inconsistent")
	}
	return a.CreationDefaults.Validate()
}

type WorkspacePolicyProjectionContext struct {
	ContextID       ContextID                          `json:"context_id"`
	TemplateID      WorkspaceTemplateID                `json:"workspace_template_id"`
	Presentation    string                             `json:"presentation"`
	ProjectRoot     string                             `json:"project_root"`
	TemplatePolicy  WorkspaceTemplatePolicyAuthority   `json:"template_policy"`
	PolicyMemory    PolicyMemoryRevision               `json:"policy_memory"`
	TemplateReceipt TemplatePolicyActivationReceipt    `json:"template_policy_receipt"`
	MemoryReceipt   PolicyMemoryActivationReceipt      `json:"policy_memory_receipt"`
	Principal       *WorkspacePolicyPrincipalAuthority `json:"principal,omitempty"`
}

func (c WorkspacePolicyProjectionContext) Validate() error {
	if err := c.ContextID.Validate(); err != nil {
		return err
	}
	if err := c.TemplateID.Validate(); err != nil {
		return err
	}
	if err := ValidateName(c.Presentation); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(c.ProjectRoot); err != nil {
		return err
	}
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: c.ContextID, ProjectRoot: c.ProjectRoot, TemplateID: c.TemplateID}
	if err := c.TemplatePolicy.Validate(); err != nil || c.TemplatePolicy.TemplateID != c.TemplateID {
		return fmt.Errorf("Context Template policy authority is inconsistent: %w", err)
	}
	if c.PolicyMemory.ContextID != c.ContextID {
		return fmt.Errorf("Context Policy Memory authority is inconsistent")
	}
	if err := c.PolicyMemory.Validate(); err != nil {
		return err
	}
	if err := c.PolicyMemory.validateInsideBoundary(c.TemplatePolicy.Boundary); err != nil {
		return fmt.Errorf("Context Policy Memory exceeds selected Template policy authority: %w", err)
	}
	if c.TemplateReceipt.ContextID != binding.ID || c.TemplateReceipt.TemplateID != binding.TemplateID || c.TemplateReceipt.PolicySliceDigest != c.TemplatePolicy.PolicySliceDigest {
		return fmt.Errorf("Context Template policy receipt is inconsistent")
	}
	if err := c.MemoryReceipt.ValidateFor(binding, c.PolicyMemory); err != nil {
		return err
	}
	if c.Principal != nil {
		if err := c.Principal.Validate(); err != nil {
			return err
		}
		if c.Principal.ContextID != c.ContextID || c.Principal.TemplateID != c.TemplateID || c.Principal.Presentation != c.Presentation || c.Principal.ProjectRoot != c.ProjectRoot {
			return fmt.Errorf("Workspace principal crosses Context authority")
		}
	}
	return nil
}

func (c WorkspacePolicyProjectionContext) Clone() WorkspacePolicyProjectionContext {
	result := c
	result.TemplatePolicy = c.TemplatePolicy.Clone()
	result.PolicyMemory = c.PolicyMemory.Clone()
	if c.Principal != nil {
		principal := *c.Principal
		principal.CreationDefaults = c.Principal.CreationDefaults.Clone()
		result.Principal = &principal
	}
	return result
}

// WorkspacePolicyProjection is the complete ordered authority for one global
// OPA/Gateway publication. Its digest is private task evidence and cannot be
// replaced by a collection generation or by independent per-Context receipts.
type WorkspacePolicyProjection struct {
	SchemaVersion      int                                `json:"schema_version"`
	Mode               WorkspacePolicyProjectionMode      `json:"mode"`
	CollectionRevision SemanticDigest                     `json:"collection_revision"`
	TargetContextID    *ContextID                         `json:"target_context_id,omitempty"`
	Contexts           []WorkspacePolicyProjectionContext `json:"contexts"`
	ContentDigest      SemanticDigest                     `json:"content_digest"`
	PlanDigest         SemanticDigest                     `json:"plan_digest"`
}

func (p WorkspacePolicyProjection) Validate() error {
	if p.SchemaVersion != WorkspacePolicyProjectionSchemaVersion {
		return fmt.Errorf("Workspace policy projection schema version is invalid")
	}
	if err := p.Mode.Validate(); err != nil {
		return err
	}
	if err := p.CollectionRevision.Validate(); err != nil {
		return err
	}
	if p.Mode == WorkspacePolicyProjectionHotMemory {
		if p.TargetContextID == nil || p.TargetContextID.Validate() != nil {
			return fmt.Errorf("hot Workspace policy projection target is invalid")
		}
	} else if p.TargetContextID != nil {
		return fmt.Errorf("cluster Workspace policy projection cannot carry a hot target")
	}
	if p.Contexts == nil {
		return fmt.Errorf("Workspace policy projection Context set is unknown")
	}
	previous := ContextID("")
	workspaces := map[WorkspaceID]struct{}{}
	targetFound := p.Mode != WorkspacePolicyProjectionHotMemory
	for _, context := range p.Contexts {
		if err := context.Validate(); err != nil {
			return err
		}
		if previous != "" && context.ContextID <= previous {
			return fmt.Errorf("Workspace policy projection Contexts must be unique and sorted")
		}
		if context.Principal != nil {
			if _, exists := workspaces[context.Principal.WorkspaceID]; exists {
				return fmt.Errorf("Workspace policy projection principal is duplicated")
			}
			workspaces[context.Principal.WorkspaceID] = struct{}{}
		}
		if p.TargetContextID != nil && context.ContextID == *p.TargetContextID {
			targetFound = true
		}
		previous = context.ContextID
	}
	if !targetFound {
		return fmt.Errorf("hot Workspace policy projection target is absent from complete content")
	}
	wantContent, err := workspacePolicyProjectionContentDigest(p)
	if err != nil {
		return err
	}
	if p.ContentDigest != wantContent {
		return fmt.Errorf("Workspace policy projection content digest is inconsistent")
	}
	wantPlan, err := workspacePolicyProjectionPlanDigest(p)
	if err != nil {
		return err
	}
	if p.PlanDigest != wantPlan {
		return fmt.Errorf("Workspace policy projection plan digest does not bind its complete selected authority")
	}
	return nil
}

func (p WorkspacePolicyProjection) Clone() WorkspacePolicyProjection {
	result := p
	result.Contexts = make([]WorkspacePolicyProjectionContext, len(p.Contexts))
	for index := range p.Contexts {
		result.Contexts[index] = p.Contexts[index].Clone()
	}
	if p.TargetContextID != nil {
		target := *p.TargetContextID
		result.TargetContextID = &target
	}
	return result
}

func workspacePolicyProjectionContentDigest(p WorkspacePolicyProjection) (SemanticDigest, error) {
	// Operation route and collection revision are plan preconditions, not live
	// OPA/Gateway content. Identical chosen axes share one content identity.
	return semanticIdentity(p.Contexts)
}

func workspacePolicyProjectionPlanDigest(p WorkspacePolicyProjection) (SemanticDigest, error) {
	return semanticIdentity(struct {
		Mode               WorkspacePolicyProjectionMode
		CollectionRevision SemanticDigest
		TargetContextID    *ContextID
		ContentDigest      SemanticDigest
	}{p.Mode, p.CollectionRevision, p.TargetContextID, p.ContentDigest})
}

func BuildHotWorkspacePolicyProjection(collection WorkspaceAuthorityCollection, target ContextID) (WorkspacePolicyProjection, error) {
	if err := target.Validate(); err != nil {
		return WorkspacePolicyProjection{}, err
	}
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionHotMemory, target)
}

func BuildClusterWorkspacePolicyProjection(collection WorkspaceAuthorityCollection) (WorkspacePolicyProjection, error) {
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionCluster, "")
}

// NewWorkspacePolicyProjection validates one already selected complete set of
// Context authorities and derives both route-independent content identity and
// exact request/plan identity. Infrastructure uses it only when preserving the
// current active axes rather than selecting Template.Current implicitly.
func NewWorkspacePolicyProjection(mode WorkspacePolicyProjectionMode, collectionRevision SemanticDigest, target *ContextID, contexts []WorkspacePolicyProjectionContext) (WorkspacePolicyProjection, error) {
	result := WorkspacePolicyProjection{
		SchemaVersion: WorkspacePolicyProjectionSchemaVersion, Mode: mode, CollectionRevision: collectionRevision,
		TargetContextID: target, Contexts: make([]WorkspacePolicyProjectionContext, len(contexts)),
	}
	for index := range contexts {
		result.Contexts[index] = contexts[index].Clone()
	}
	if target != nil {
		copyTarget := *target
		result.TargetContextID = &copyTarget
	}
	var err error
	result.ContentDigest, err = workspacePolicyProjectionContentDigest(result)
	if err != nil {
		return WorkspacePolicyProjection{}, err
	}
	result.PlanDigest, err = workspacePolicyProjectionPlanDigest(result)
	if err != nil {
		return WorkspacePolicyProjection{}, err
	}
	return result, result.Validate()
}

func buildWorkspacePolicyProjection(collection WorkspaceAuthorityCollection, mode WorkspacePolicyProjectionMode, target ContextID) (WorkspacePolicyProjection, error) {
	if err := collection.Validate(); err != nil {
		return WorkspacePolicyProjection{}, err
	}
	if err := mode.Validate(); err != nil {
		return WorkspacePolicyProjection{}, err
	}
	templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(collection.Templates))
	for _, template := range collection.Templates {
		templates[template.ID] = template
	}
	workspaces := make(map[ContextID]WorkspaceBinding, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ContextID] = workspace
	}
	contexts := make([]WorkspacePolicyProjectionContext, 0, len(collection.Contexts))
	targetFound := mode != WorkspacePolicyProjectionHotMemory
	for _, record := range collection.Contexts {
		template := templates[record.Context.TemplateID]
		var revision WorkspaceTemplateRevision
		var memory PolicyMemoryRevision
		if mode == WorkspacePolicyProjectionCluster {
			revision = template.Current.Clone()
			memory = record.PolicyMemory.Clone()
		} else {
			if record.ActiveTemplatePolicy == nil || record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil {
				return WorkspacePolicyProjection{}, fmt.Errorf("hot Policy Memory projection requires both active axes for every Context")
			}
			var found bool
			for _, retained := range template.Retained {
				if retained.Slices.PolicySliceDigest != record.ActiveTemplatePolicy.PolicySliceDigest {
					continue
				}
				if !found {
					revision, found = retained.Clone(), true
					continue
				}
				left, _ := NewWorkspaceTemplatePolicyAuthority(revision)
				right, _ := NewWorkspaceTemplatePolicyAuthority(retained)
				if !reflect.DeepEqual(left, right) {
					return WorkspacePolicyProjection{}, fmt.Errorf("active Template policy digest is ambiguous")
				}
			}
			if !found || record.ActiveTemplatePolicy.ValidateFor(record.Context, revision) != nil {
				return WorkspacePolicyProjection{}, fmt.Errorf("active Template policy revision is unavailable")
			}
			if record.Context.ID == target {
				targetFound = true
				memory = record.PolicyMemory.Clone()
			} else {
				memory = record.ActivePolicyMemory.Clone()
			}
			if record.Context.ID != target && record.ActivePolicyMemoryRef.ValidateFor(record.Context, memory) != nil {
				return WorkspacePolicyProjection{}, fmt.Errorf("active Policy Memory authority is unavailable")
			}
		}
		policy, err := NewWorkspaceTemplatePolicyAuthority(revision)
		if err != nil {
			return WorkspacePolicyProjection{}, err
		}
		item := WorkspacePolicyProjectionContext{
			ContextID: record.Context.ID, TemplateID: template.ID, Presentation: template.Name,
			ProjectRoot: record.Context.ProjectRoot, TemplatePolicy: policy, PolicyMemory: memory,
			TemplateReceipt: TemplatePolicyActivationReceipt{ContextID: record.Context.ID, TemplateID: template.ID, PolicySliceDigest: policy.PolicySliceDigest},
			MemoryReceipt:   PolicyMemoryActivationReceipt{ContextID: record.Context.ID, Revision: memory.Revision},
		}
		if workspace, exists := workspaces[record.Context.ID]; exists && workspace.LastSuccessfulEntry != nil {
			var creation WorkspaceTemplateCreationDefaults
			creationFound := false
			for _, retained := range template.Retained {
				if retained.Slices.CreationDefaultsDigest != workspace.CreationDefaults {
					continue
				}
				if !creationFound {
					creation, creationFound = retained.Body.CreationDefaults.Clone(), true
					continue
				}
				if !reflect.DeepEqual(creation, retained.Body.CreationDefaults) {
					return WorkspacePolicyProjection{}, fmt.Errorf("Workspace creation defaults digest is ambiguous")
				}
			}
			if !creationFound {
				return WorkspacePolicyProjection{}, fmt.Errorf("Workspace creation defaults authority is unavailable")
			}
			item.Principal = &WorkspacePolicyPrincipalAuthority{
				ContextID: record.Context.ID, WorkspaceID: workspace.ID, TemplateID: template.ID,
				Presentation: template.Name, ProjectRoot: record.Context.ProjectRoot, AppliedEntry: *workspace.LastSuccessfulEntry,
				CreationDefaultsDigest: workspace.CreationDefaults, CreationDefaults: creation,
			}
		}
		contexts = append(contexts, item)
	}
	if !targetFound {
		return WorkspacePolicyProjection{}, fmt.Errorf("hot Policy Memory target Context is unavailable")
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].ContextID < contexts[j].ContextID })
	result := WorkspacePolicyProjection{
		SchemaVersion: WorkspacePolicyProjectionSchemaVersion, Mode: mode,
		CollectionRevision: collection.Revision, Contexts: contexts,
	}
	if mode == WorkspacePolicyProjectionHotMemory {
		value := target
		result.TargetContextID = &value
	}
	contentDigest, err := workspacePolicyProjectionContentDigest(result)
	if err != nil {
		return WorkspacePolicyProjection{}, err
	}
	result.ContentDigest = contentDigest
	planDigest, err := workspacePolicyProjectionPlanDigest(result)
	if err != nil {
		return WorkspacePolicyProjection{}, err
	}
	result.PlanDigest = planDigest
	return result, result.Validate()
}
