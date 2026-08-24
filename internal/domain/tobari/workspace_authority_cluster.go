package tobari

import (
	"fmt"
	"reflect"
)

const WorkspaceAuthorityClusterReconciliationPlanSchemaVersion = 1

// WorkspaceAuthorityClusterReconciliationPlan binds one cluster-wide runtime
// settlement to the exact complete final-authority envelope it will publish.
// The projection carries the independently selected Template-policy and
// Policy-Memory receipts for every Context; the collection revision remains a
// separate global coherence receipt.
type WorkspaceAuthorityClusterReconciliationPlan struct {
	SchemaVersion      int                       `json:"schema_version"`
	PreviousGeneration uint64                    `json:"previous_generation"`
	PreviousRevision   SemanticDigest            `json:"previous_revision"`
	NextGeneration     uint64                    `json:"next_generation"`
	NextRevision       SemanticDigest            `json:"next_revision"`
	EnvelopeChanged    bool                      `json:"envelope_changed"`
	Projection         WorkspacePolicyProjection `json:"projection"`
}

func (p WorkspaceAuthorityClusterReconciliationPlan) Validate() error {
	if p.SchemaVersion != WorkspaceAuthorityClusterReconciliationPlanSchemaVersion || p.PreviousGeneration == 0 {
		return fmt.Errorf("Workspace authority cluster reconciliation plan metadata is invalid")
	}
	if err := p.PreviousRevision.Validate(); err != nil {
		return err
	}
	if err := p.NextRevision.Validate(); err != nil {
		return err
	}
	if p.EnvelopeChanged {
		if p.NextGeneration != p.PreviousGeneration+1 || p.NextRevision == p.PreviousRevision {
			return fmt.Errorf("Workspace authority cluster reconciliation transition is invalid")
		}
	} else if p.NextGeneration != p.PreviousGeneration || p.NextRevision != p.PreviousRevision {
		return fmt.Errorf("Workspace authority cluster reconciliation no-op is invalid")
	}
	if err := p.Projection.Validate(); err != nil {
		return err
	}
	if p.Projection.Mode != WorkspacePolicyProjectionCluster || p.Projection.CollectionRevision != p.NextRevision {
		return fmt.Errorf("Workspace authority cluster projection is inconsistent")
	}
	return nil
}

func (p WorkspaceAuthorityClusterReconciliationPlan) ValidateTransition(previous, next WorkspaceAuthorityCollection) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := next.Validate(); err != nil {
		return err
	}
	if previous.Generation != p.PreviousGeneration || previous.Revision != p.PreviousRevision {
		return fmt.Errorf("Workspace authority cluster plan does not bind the envelope transition")
	}
	if err := p.ValidateCurrent(next); err != nil {
		return err
	}
	want, err := PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(want.Next, next) || !reflect.DeepEqual(want.Plan, p) {
		return fmt.Errorf("Workspace authority cluster plan does not select current Context authority")
	}
	return nil
}

// ValidateCurrent proves both the global envelope receipt and every
// independently active Context receipt. This is the terminal replay check: a
// matching generation alone cannot stand in for either policy axis.
func (p WorkspaceAuthorityClusterReconciliationPlan) ValidateCurrent(current WorkspaceAuthorityCollection) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if current.Generation != p.NextGeneration || current.Revision != p.NextRevision {
		return fmt.Errorf("Workspace authority cluster plan does not bind the current envelope")
	}
	projection, err := BuildClusterWorkspacePolicyProjection(current)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(projection, p.Projection) {
		return fmt.Errorf("Workspace authority cluster projection is no longer current")
	}
	if len(current.Contexts) != len(projection.Contexts) {
		return fmt.Errorf("Workspace authority cluster active receipt set is incomplete")
	}
	for index, record := range current.Contexts {
		item := projection.Contexts[index]
		if record.Context.ID != item.ContextID || record.ActiveTemplatePolicy == nil ||
			record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil ||
			*record.ActiveTemplatePolicy != item.TemplateReceipt ||
			!reflect.DeepEqual(*record.ActivePolicyMemory, item.PolicyMemory) ||
			*record.ActivePolicyMemoryRef != item.MemoryReceipt {
			return fmt.Errorf("Workspace authority cluster Context receipt is not current")
		}
	}
	return nil
}

// WorkspaceAuthorityClusterTransition is the in-memory mutation product. Only
// Plan is durable decision authority; Next is staged through the existing
// final-envelope publication mechanism.
type WorkspaceAuthorityClusterTransition struct {
	Plan WorkspaceAuthorityClusterReconciliationPlan
	Next WorkspaceAuthorityCollection
}

func PlanWorkspaceAuthorityClusterReconciliation(previous WorkspaceAuthorityCollection) (WorkspaceAuthorityClusterTransition, error) {
	if err := previous.Validate(); err != nil {
		return WorkspaceAuthorityClusterTransition{}, err
	}
	templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(previous.Templates))
	for _, template := range previous.Templates {
		templates[template.ID] = template
	}
	contexts := make([]WorkspaceAuthorityContextRecord, len(previous.Contexts))
	for index, record := range previous.Contexts {
		contexts[index] = record.Clone()
		template, found := templates[record.Context.TemplateID]
		if !found {
			return WorkspaceAuthorityClusterTransition{}, fmt.Errorf("Context Template authority is unavailable")
		}
		templateReceipt := TemplatePolicyActivationReceipt{
			ContextID: record.Context.ID, TemplateID: template.ID,
			PolicySliceDigest: template.Current.Slices.PolicySliceDigest,
		}
		if err := templateReceipt.ValidateFor(record.Context, template.Current); err != nil {
			return WorkspaceAuthorityClusterTransition{}, err
		}
		memory := record.PolicyMemory.Clone()
		memoryReceipt := PolicyMemoryActivationReceipt{ContextID: record.Context.ID, Revision: memory.Revision}
		if err := memoryReceipt.ValidateFor(record.Context, memory); err != nil {
			return WorkspaceAuthorityClusterTransition{}, err
		}
		contexts[index].ActiveTemplatePolicy = &templateReceipt
		contexts[index].ActivePolicyMemory = &memory
		contexts[index].ActivePolicyMemoryRef = &memoryReceipt
	}
	next, changed, err := PublishWorkspaceAuthorityCollection(
		previous.Templates, contexts, previous.Workspaces, previous.PendingCandidates, previous.DefaultTemplateID, &previous,
	)
	if err != nil {
		return WorkspaceAuthorityClusterTransition{}, err
	}
	projection, err := BuildClusterWorkspacePolicyProjection(next)
	if err != nil {
		return WorkspaceAuthorityClusterTransition{}, err
	}
	plan := WorkspaceAuthorityClusterReconciliationPlan{
		SchemaVersion:      WorkspaceAuthorityClusterReconciliationPlanSchemaVersion,
		PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision,
		NextGeneration: next.Generation, NextRevision: next.Revision, EnvelopeChanged: changed,
		Projection: projection,
	}
	if err := plan.Validate(); err != nil {
		return WorkspaceAuthorityClusterTransition{}, err
	}
	return WorkspaceAuthorityClusterTransition{Plan: plan, Next: next}, nil
}
