package tobari

import (
	"fmt"
	"reflect"
	"sort"
)

const WorkspacePolicyProjectionSchemaVersion = 2

type WorkspacePolicyProjectionMode string

const (
	WorkspacePolicyProjectionHotMemory WorkspacePolicyProjectionMode = "hot_policy_memory"
	WorkspacePolicyProjectionCluster   WorkspacePolicyProjectionMode = "cluster_reconciliation"
	WorkspacePolicyProjectionActive    WorkspacePolicyProjectionMode = "active_authority"
	WorkspacePolicyProjectionReviewed  WorkspacePolicyProjectionMode = "reviewed_policy_set"
)

func (m WorkspacePolicyProjectionMode) Validate() error {
	switch m {
	case WorkspacePolicyProjectionHotMemory, WorkspacePolicyProjectionCluster, WorkspacePolicyProjectionActive, WorkspacePolicyProjectionReviewed:
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
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: a.ContextID, TemplateID: a.TemplateID}
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
	ContextID       ContextID                           `json:"context_id"`
	TemplateID      WorkspaceTemplateID                 `json:"workspace_template_id"`
	Presentation    string                              `json:"presentation"`
	TemplatePolicy  WorkspaceTemplatePolicyAuthority    `json:"template_policy"`
	PolicyMemory    PolicyMemoryRevision                `json:"policy_memory"`
	TemplateReceipt TemplatePolicyActivationReceipt     `json:"template_policy_receipt"`
	MemoryReceipt   PolicyMemoryActivationReceipt       `json:"policy_memory_receipt"`
	Principals      []WorkspacePolicyPrincipalAuthority `json:"principals"`
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
	binding := ContextBinding{SchemaVersion: ContextBindingSchemaVersion, ID: c.ContextID, TemplateID: c.TemplateID}
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
	if c.Principals == nil {
		return fmt.Errorf("Context Workspace principal set is unknown")
	}
	previousWorkspace := WorkspaceID("")
	for index := range c.Principals {
		principal := c.Principals[index]
		if previousWorkspace != "" && principal.WorkspaceID <= previousWorkspace {
			return fmt.Errorf("Context Workspace principals must be unique and sorted")
		}
		if err := principal.Validate(); err != nil {
			return err
		}
		if principal.ContextID != c.ContextID || principal.TemplateID != c.TemplateID || principal.Presentation != c.Presentation {
			return fmt.Errorf("Workspace principal crosses Context authority")
		}
		previousWorkspace = principal.WorkspaceID
	}
	return nil
}

func (c WorkspacePolicyProjectionContext) Clone() WorkspacePolicyProjectionContext {
	result := c
	result.TemplatePolicy = c.TemplatePolicy.Clone()
	result.PolicyMemory = c.PolicyMemory.Clone()
	result.Principals = make([]WorkspacePolicyPrincipalAuthority, len(c.Principals))
	for index := range c.Principals {
		result.Principals[index] = c.Principals[index]
		result.Principals[index].CreationDefaults = c.Principals[index].CreationDefaults.Clone()
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
	TargetContextIDs   []ContextID                        `json:"target_context_ids,omitempty"`
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
		if p.TargetContextID == nil || p.TargetContextID.Validate() != nil || p.TargetContextIDs != nil {
			return fmt.Errorf("hot Workspace policy projection target is invalid")
		}
	} else if p.Mode == WorkspacePolicyProjectionReviewed {
		if p.TargetContextID != nil || p.TargetContextIDs == nil || len(p.TargetContextIDs) == 0 || len(p.TargetContextIDs) > MaxPolicyReviewDecisions {
			return fmt.Errorf("reviewed Workspace policy projection targets are invalid")
		}
		previousTarget := ContextID("")
		for _, target := range p.TargetContextIDs {
			if target.Validate() != nil || previousTarget != "" && target <= previousTarget {
				return fmt.Errorf("reviewed Workspace policy projection targets must be unique and sorted")
			}
			previousTarget = target
		}
	} else if p.TargetContextID != nil || p.TargetContextIDs != nil {
		return fmt.Errorf("non-hot Workspace policy projection cannot carry a hot target")
	}
	if p.Contexts == nil {
		return fmt.Errorf("Workspace policy projection Context set is unknown")
	}
	previous := ContextID("")
	workspaces := map[WorkspaceID]struct{}{}
	targetsFound := make(map[ContextID]bool, len(p.TargetContextIDs)+1)
	if p.TargetContextID != nil {
		targetsFound[*p.TargetContextID] = false
	}
	for _, target := range p.TargetContextIDs {
		targetsFound[target] = false
	}
	for _, context := range p.Contexts {
		if err := context.Validate(); err != nil {
			return err
		}
		if previous != "" && context.ContextID <= previous {
			return fmt.Errorf("Workspace policy projection Contexts must be unique and sorted")
		}
		for _, principal := range context.Principals {
			if _, exists := workspaces[principal.WorkspaceID]; exists {
				return fmt.Errorf("Workspace policy projection principal is duplicated")
			}
			workspaces[principal.WorkspaceID] = struct{}{}
		}
		if _, selected := targetsFound[context.ContextID]; selected {
			targetsFound[context.ContextID] = true
		}
		previous = context.ContextID
	}
	for _, found := range targetsFound {
		if !found {
			return fmt.Errorf("Workspace policy projection target is absent from complete content")
		}
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
	if p.TargetContextIDs != nil {
		result.TargetContextIDs = append([]ContextID{}, p.TargetContextIDs...)
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
		TargetContextIDs   []ContextID
		ContentDigest      SemanticDigest
	}{p.Mode, p.CollectionRevision, p.TargetContextID, p.TargetContextIDs, p.ContentDigest})
}

func BuildHotWorkspacePolicyProjection(collection WorkspaceAuthorityCollection, target ContextID) (WorkspacePolicyProjection, error) {
	if err := target.Validate(); err != nil {
		return WorkspacePolicyProjection{}, err
	}
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionHotMemory, target, nil)
}

func BuildClusterWorkspacePolicyProjection(collection WorkspaceAuthorityCollection) (WorkspacePolicyProjection, error) {
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionCluster, "", nil)
}

// BuildActiveWorkspacePolicyProjection selects every Context's independently
// active Template-policy and Policy-Memory axes without authorizing a new hot
// target or adopting Template.Current. It is the complete authority used when
// a Context deletion removes one axis owner from the global projection.
func BuildActiveWorkspacePolicyProjection(collection WorkspaceAuthorityCollection) (WorkspacePolicyProjection, error) {
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionActive, "", nil)
}

// BuildWorkspaceRetirementPolicyProjection preserves the last verified live
// policy axes while removing one exact Workspace principal. It is the bounded
// recovery path for authority written by older binaries that lost an active
// Template or Memory receipt after publishing desired Template state. The
// live projection is trusted input from the infrastructure-owned activation
// receipt; desired Template and Policy Memory state are never inferred here.
func BuildWorkspaceRetirementPolicyProjection(active WorkspacePolicyProjection, next WorkspaceAuthorityCollection, workspace WorkspaceBinding) (WorkspacePolicyProjection, error) {
	if err := active.Validate(); err != nil {
		return WorkspacePolicyProjection{}, fmt.Errorf("active Workspace retirement projection: %w", err)
	}
	if err := next.Validate(); err != nil {
		return WorkspacePolicyProjection{}, err
	}
	contextIndex := -1
	for index, record := range next.Contexts {
		if record.Context.ID == workspace.ContextID {
			contextIndex = index
			break
		}
	}
	if contextIndex < 0 || workspace.ValidateFor(next.Contexts[contextIndex].Context) != nil {
		return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection crosses Context authority")
	}
	for _, retained := range next.Workspaces {
		if retained.ID == workspace.ID {
			return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection retains its target")
		}
	}

	contextsByID := make(map[ContextID]ContextBinding, len(next.Contexts))
	for _, record := range next.Contexts {
		contextsByID[record.Context.ID] = record.Context
	}
	contexts := make([]WorkspacePolicyProjectionContext, len(active.Contexts))
	targetFound := false
	principalFound := false
	for index, item := range active.Contexts {
		binding, found := contextsByID[item.ContextID]
		if !found || binding.TemplateID != item.TemplateID {
			return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection contains foreign Context authority")
		}
		contexts[index] = item.Clone()
		if item.ContextID == workspace.ContextID {
			targetFound = true
			retained := make([]WorkspacePolicyPrincipalAuthority, 0, len(item.Principals))
			for _, principal := range item.Principals {
				if principal.WorkspaceID != workspace.ID {
					retained = append(retained, principal)
					continue
				}
				if !workspacePrincipalMatchesBinding(principal, workspace) {
					return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection principal differs from its target")
				}
				principalFound = true
			}
			contexts[index].Principals = retained
		} else {
			for _, principal := range item.Principals {
				if principal.WorkspaceID == workspace.ID {
					return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection rebinds its target principal")
				}
			}
		}
	}
	if !targetFound {
		return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection target Context is not active")
	}
	if !principalFound && (active.Mode != WorkspacePolicyProjectionActive || active.CollectionRevision != next.Revision) {
		return WorkspacePolicyProjection{}, fmt.Errorf("Workspace retirement projection has no exact target principal")
	}
	return NewWorkspacePolicyProjection(WorkspacePolicyProjectionActive, next.Revision, nil, contexts)
}

func workspacePrincipalMatchesBinding(principal WorkspacePolicyPrincipalAuthority, workspace WorkspaceBinding) bool {
	return workspace.LastSuccessfulEntry != nil &&
		principal.ContextID == workspace.ContextID && principal.WorkspaceID == workspace.ID &&
		principal.TemplateID == workspace.LastSuccessfulEntry.TemplateID && principal.ProjectRoot == workspace.ProjectRoot &&
		principal.AppliedEntry == *workspace.LastSuccessfulEntry && principal.CreationDefaultsDigest == workspace.CreationDefaults
}

func BuildReviewedWorkspacePolicyProjection(collection WorkspaceAuthorityCollection, targets []ContextID) (WorkspacePolicyProjection, error) {
	if len(targets) == 0 || len(targets) > MaxPolicyReviewDecisions {
		return WorkspacePolicyProjection{}, fmt.Errorf("reviewed Workspace policy projection target set is invalid")
	}
	copyTargets := append([]ContextID{}, targets...)
	for index, target := range copyTargets {
		if target.Validate() != nil || index > 0 && target <= copyTargets[index-1] {
			return WorkspacePolicyProjection{}, fmt.Errorf("reviewed Workspace policy projection targets must be unique and sorted")
		}
	}
	return buildWorkspacePolicyProjection(collection, WorkspacePolicyProjectionReviewed, "", copyTargets)
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

func buildWorkspacePolicyProjection(collection WorkspaceAuthorityCollection, mode WorkspacePolicyProjectionMode, target ContextID, reviewedTargets []ContextID) (WorkspacePolicyProjection, error) {
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
	workspaces := make(map[ContextID][]WorkspaceBinding, len(collection.Contexts))
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ContextID] = append(workspaces[workspace.ContextID], workspace)
	}
	contexts := make([]WorkspacePolicyProjectionContext, 0, len(collection.Contexts))
	targetSet := make(map[ContextID]struct{}, len(reviewedTargets))
	for _, reviewedTarget := range reviewedTargets {
		targetSet[reviewedTarget] = struct{}{}
	}
	targetFound := mode != WorkspacePolicyProjectionHotMemory
	reviewedFound := make(map[ContextID]bool, len(reviewedTargets))
	for _, reviewedTarget := range reviewedTargets {
		reviewedFound[reviewedTarget] = false
	}
	for _, record := range collection.Contexts {
		template := templates[record.Context.TemplateID]
		var revision WorkspaceTemplateRevision
		var memory PolicyMemoryRevision
		if mode == WorkspacePolicyProjectionCluster {
			revision = template.Current.Clone()
			memory = record.PolicyMemory.Clone()
		} else {
			inactive := record.ActiveTemplatePolicy == nil && record.ActivePolicyMemory == nil && record.ActivePolicyMemoryRef == nil
			if inactive {
				// An intentionally new Context owns durable desired authority but
				// contributes no executable global policy until cluster
				// reconciliation activates both axes together.
				continue
			}
			if record.ActiveTemplatePolicy == nil || record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil {
				return WorkspacePolicyProjection{}, fmt.Errorf("Workspace policy projection has a partial active Context authority")
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
			_, reviewedTarget := targetSet[record.Context.ID]
			selectingCurrent := mode == WorkspacePolicyProjectionHotMemory && record.Context.ID == target || mode == WorkspacePolicyProjectionReviewed && reviewedTarget
			if selectingCurrent {
				targetFound = true
				if reviewedTarget {
					reviewedFound[record.Context.ID] = true
				}
				memory = record.PolicyMemory.Clone()
			} else {
				memory = record.ActivePolicyMemory.Clone()
			}
			if !selectingCurrent && record.ActivePolicyMemoryRef.ValidateFor(record.Context, memory) != nil {
				return WorkspacePolicyProjection{}, fmt.Errorf("active Policy Memory authority is unavailable")
			}
		}
		policy, err := NewWorkspaceTemplatePolicyAuthority(revision)
		if err != nil {
			return WorkspacePolicyProjection{}, err
		}
		item := WorkspacePolicyProjectionContext{
			ContextID: record.Context.ID, TemplateID: template.ID, Presentation: template.Name,
			TemplatePolicy: policy, PolicyMemory: memory,
			TemplateReceipt: TemplatePolicyActivationReceipt{ContextID: record.Context.ID, TemplateID: template.ID, PolicySliceDigest: policy.PolicySliceDigest},
			MemoryReceipt:   PolicyMemoryActivationReceipt{ContextID: record.Context.ID, Revision: memory.Revision},
			Principals:      []WorkspacePolicyPrincipalAuthority{},
		}
		for _, workspace := range workspaces[record.Context.ID] {
			if workspace.LastSuccessfulEntry == nil {
				continue
			}
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
			item.Principals = append(item.Principals, WorkspacePolicyPrincipalAuthority{
				ContextID: record.Context.ID, WorkspaceID: workspace.ID, TemplateID: template.ID,
				Presentation: template.Name, ProjectRoot: workspace.ProjectRoot, AppliedEntry: *workspace.LastSuccessfulEntry,
				CreationDefaultsDigest: workspace.CreationDefaults, CreationDefaults: creation,
			})
		}
		sort.Slice(item.Principals, func(i, j int) bool { return item.Principals[i].WorkspaceID < item.Principals[j].WorkspaceID })
		contexts = append(contexts, item)
	}
	if !targetFound {
		return WorkspacePolicyProjection{}, fmt.Errorf("hot Policy Memory target Context is unavailable")
	}
	for _, found := range reviewedFound {
		if !found {
			return WorkspacePolicyProjection{}, fmt.Errorf("reviewed Policy Memory target Context is unavailable")
		}
	}
	sort.Slice(contexts, func(i, j int) bool { return contexts[i].ContextID < contexts[j].ContextID })
	result := WorkspacePolicyProjection{
		SchemaVersion: WorkspacePolicyProjectionSchemaVersion, Mode: mode,
		CollectionRevision: collection.Revision, Contexts: contexts,
	}
	if mode == WorkspacePolicyProjectionHotMemory {
		value := target
		result.TargetContextID = &value
	} else if mode == WorkspacePolicyProjectionReviewed {
		result.TargetContextIDs = append([]ContextID{}, reviewedTargets...)
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
