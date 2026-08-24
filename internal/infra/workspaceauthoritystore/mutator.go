package workspaceauthoritystore

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// LifecycleAuthority is the existing installation-wide serialization
// boundary. The final-authority adapter deliberately does not own a second
// lock or recovery concept.
type LifecycleAuthority interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

// DeletionAuthority owns the external prerequisites that cannot be inferred
// from the final envelope. Implementations must make Workspace retirement
// receipt-idempotent because a confirmed external retirement can precede an
// interrupted envelope publication.
type DeletionAuthority interface {
	ConfirmWorkspaceRetirementAllowed(context.Context, tobari.WorkspaceBinding, bool) error
	RetireWorkspace(context.Context, tobari.WorkspaceBinding, bool, string) error
	ConfirmWorkspaceRetired(context.Context, tobari.WorkspaceBinding, string) error
	ConfirmContextCredentialAbsent(context.Context, tobari.ContextID) error
}

// PolicyMemoryActivationAuthority owns the effectful aggregate policy
// projection. The envelope records an active receipt only after this boundary
// confirms the exact Context-owned revision; failures preserve the prior
// current and last-successful active authority.
type PolicyMemoryActivationAuthority interface {
	ActivatePolicyMemory(context.Context, tobari.ContextAuthoritySnapshot, tobari.PolicyMemoryRevision) (tobari.PolicyMemoryActivationReceipt, error)
	ConfirmPolicyMemoryActive(context.Context, tobari.ContextAuthoritySnapshot, tobari.PolicyMemoryActivationReceipt) error
}

// Mutator publishes complete final-authority envelopes behind the existing
// lifecycle authority. It is dormant until the atomic WP11 reader cutover.
type Mutator struct {
	store      *Store
	lifecycle  LifecycleAuthority
	deletion   DeletionAuthority
	activation PolicyMemoryActivationAuthority
	clock      func() time.Time
	entropy    io.Reader

	rename func(string, string) error
	sync   func(string) error
}

const effectDecisionSchemaVersion = 1

type effectDecision struct {
	SchemaVersion      int                                      `json:"schema_version"`
	Operation          string                                   `json:"operation"`
	Target             string                                   `json:"target"`
	PreviousGeneration uint64                                   `json:"previous_generation"`
	PreviousRevision   tobari.SemanticDigest                    `json:"previous_revision"`
	NextGeneration     uint64                                   `json:"next_generation"`
	NextRevision       tobari.SemanticDigest                    `json:"next_revision"`
	WorkspaceID        *tobari.WorkspaceID                      `json:"workspace_id,omitempty"`
	Workspace          *tobari.WorkspaceBinding                 `json:"workspace,omitempty"`
	Force              *bool                                    `json:"force,omitempty"`
	Candidate          *tobari.PolicyCandidateAuthority         `json:"candidate,omitempty"`
	RuleID             string                                   `json:"rule_id,omitempty"`
	Decision           tobari.PolicyMemoryDecision              `json:"decision,omitempty"`
	PreviousMemory     *tobari.PolicyMemoryRevision             `json:"previous_policy_memory,omitempty"`
	EntryPlan          *tobari.WorkspaceEntryReconciliationPlan `json:"workspace_entry_plan,omitempty"`
}

func (d effectDecision) validate() error {
	if d.SchemaVersion != effectDecisionSchemaVersion || d.Operation == "" || d.Target == "" || d.PreviousGeneration == 0 {
		return fmt.Errorf("final-authority effect decision metadata is invalid")
	}
	if err := d.PreviousRevision.Validate(); err != nil {
		return err
	}
	if err := d.NextRevision.Validate(); err != nil {
		return err
	}
	switch d.Operation {
	case "workspace-delete", "workspace-delete-force":
		if d.NextGeneration != d.PreviousGeneration+1 || d.WorkspaceID == nil || d.WorkspaceID.Validate() != nil || d.Workspace == nil || d.Workspace.ID != *d.WorkspaceID || d.Force == nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil {
			return fmt.Errorf("Workspace delete effect decision is invalid")
		}
	case "policy-allow", "policy-deny":
		if d.NextGeneration != d.PreviousGeneration+1 || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate == nil || d.Candidate.Validate() != nil || d.RuleID == "" || d.PreviousMemory == nil || d.PreviousMemory.Validate() != nil || d.Decision.Validate() != nil || d.EntryPlan != nil {
			return fmt.Errorf("Policy candidate effect decision is invalid")
		}
	case "policy-reset":
		if d.NextGeneration != d.PreviousGeneration+1 || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID == "" || d.PreviousMemory == nil || d.PreviousMemory.Validate() != nil || d.Decision != "" || d.EntryPlan != nil {
			return fmt.Errorf("Policy reset effect decision is invalid")
		}
	case "context-entry":
		contextID, err := tobari.ParseContextRef(d.Target)
		if err != nil || d.EntryPlan == nil || d.EntryPlan.Workspace.ContextID != contextID || d.EntryPlan.Applied.ContextID != contextID || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil {
			return fmt.Errorf("Context entry effect decision is invalid")
		}
		binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: d.EntryPlan.Workspace.ProjectRoot, TemplateID: d.EntryPlan.Applied.TemplateID}
		if d.EntryPlan.Workspace.ValidateFor(binding) != nil || d.EntryPlan.Applied.ValidateFor(binding) != nil || d.EntryPlan.Workspace.LastSuccessfulEntry == nil || *d.EntryPlan.Workspace.LastSuccessfulEntry != d.EntryPlan.Applied {
			return fmt.Errorf("Context entry effect decision plan is invalid")
		}
		if d.NextGeneration != d.PreviousGeneration && d.NextGeneration != d.PreviousGeneration+1 {
			return fmt.Errorf("Context entry envelope transition is invalid")
		}
		if d.NextGeneration == d.PreviousGeneration && d.NextRevision != d.PreviousRevision {
			return fmt.Errorf("Context entry no-op transition changed revision")
		}
	default:
		return fmt.Errorf("final-authority effect decision operation is invalid")
	}
	return nil
}

type effectPlan struct {
	next     tobari.WorkspaceAuthorityCollection
	decision effectDecision
	effect   func(context.Context) error
}

func NewMutator(store *Store, lifecycle LifecycleAuthority, deletion DeletionAuthority, activation PolicyMemoryActivationAuthority) (*Mutator, error) {
	if store == nil || store.root == "" {
		return nil, fmt.Errorf("final Workspace authority store is required")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("installation lifecycle authority is required")
	}
	return &Mutator{
		store: store, lifecycle: lifecycle, deletion: deletion, activation: activation,
		clock: time.Now, entropy: rand.Reader, rename: os.Rename, sync: syncMutationDirectory,
	}, nil
}

func (m *Mutator) CreateWorkspaceTemplate(ctx context.Context, name string, body tobari.WorkspaceTemplateBody) (created tobari.WorkspaceTemplate, resultErr error) {
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		for _, existing := range current.Templates {
			if existing.Name == name {
				return current, false, tobari.ErrWorkspaceTemplateExists
			}
		}
		id, err := tobari.IssueWorkspaceTemplateID(m.clock().UTC(), m.entropy)
		if err != nil {
			return current, false, err
		}
		revision, err := tobari.NewWorkspaceTemplateRevision(id, 1, body)
		if err != nil {
			return current, false, err
		}
		created = tobari.WorkspaceTemplate{
			SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: id, Name: name,
			Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()},
		}
		templates := append(cloneTemplates(current.Templates), created.Clone())
		next, changed, err := publishCollection(current, present, templates, current.Contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
		return next, changed, err
	})
	return created.Clone(), resultErr
}

func (m *Mutator) CopyWorkspaceTemplateByRevisionReference(ctx context.Context, revisionRef, name string) (publication tobari.WorkspaceTemplateCopyPublication, resultErr error) {
	sourceID, sourceDigest, err := tobari.ParseWorkspaceTemplateRevisionRef(revisionRef)
	if err != nil {
		return publication, err
	}
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		for _, existing := range current.Templates {
			if existing.Name == name {
				return current, false, tobari.ErrWorkspaceTemplateExists
			}
		}
		var source *tobari.WorkspaceTemplateRevision
		for _, template := range current.Templates {
			if template.ID != sourceID {
				continue
			}
			for index := len(template.Retained) - 1; index >= 0; index-- {
				if template.Retained[index].Revision == sourceDigest {
					value := template.Retained[index].Clone()
					source = &value
					break
				}
			}
		}
		if source == nil {
			return current, false, tobari.ErrWorkspaceTemplateRevisionNotFound
		}
		id, err := tobari.IssueWorkspaceTemplateID(m.clock().UTC(), m.entropy)
		if err != nil {
			return current, false, err
		}
		created, err := tobari.CopyWorkspaceTemplateRevision(id, name, *source)
		if err != nil {
			return current, false, err
		}
		publication = tobari.WorkspaceTemplateCopyPublication{Source: source.Clone(), Created: created.Clone()}
		templates := append(cloneTemplates(current.Templates), created)
		next, changed, err := publishCollection(current, present, templates, current.Contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
		return next, changed, err
	})
	return publication, resultErr
}

func (m *Mutator) SetDefaultWorkspaceTemplateByReference(ctx context.Context, ref string) (result tobari.WorkspaceTemplateSelectionResult, resultErr error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return result, err
	}
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		if !templateExists(current, id) {
			return current, false, tobari.ErrWorkspaceTemplateNotFound
		}
		result = tobari.WorkspaceTemplateSelectionResult{TemplateID: id, Selected: true}
		next, changed, err := publishCollection(current, present, current.Templates, current.Contexts, current.Workspaces, current.PendingCandidates, &id)
		return next, changed, err
	})
	return result, resultErr
}

func (m *Mutator) DeleteWorkspaceTemplateByReference(ctx context.Context, ref string) (result tobari.WorkspaceTemplateDeleteResult, resultErr error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return result, err
	}
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		if !templateExists(current, id) {
			return current, false, tobari.ErrWorkspaceTemplateNotFound
		}
		if current.DefaultTemplateID != nil && *current.DefaultTemplateID == id {
			return current, false, tobari.ErrWorkspaceTemplateProtected
		}
		for _, record := range current.Contexts {
			if record.Context.TemplateID == id {
				return current, false, tobari.ErrWorkspaceTemplateProtected
			}
		}
		templates := make([]tobari.WorkspaceTemplate, 0, len(current.Templates)-1)
		for _, template := range current.Templates {
			if template.ID != id {
				templates = append(templates, template.Clone())
			}
		}
		result = tobari.WorkspaceTemplateDeleteResult{TemplateID: id, Deleted: true}
		next, changed, err := publishCollection(current, present, templates, current.Contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
		return next, changed, err
	})
	return result, resultErr
}

func (m *Mutator) CreateContextByTemplateReference(ctx context.Context, templateRef, projectRoot string) (created tobari.ContextAuthoritySnapshot, resultErr error) {
	templateID, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return created, err
	}
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		var template *tobari.WorkspaceTemplate
		for index := range current.Templates {
			if current.Templates[index].ID == templateID {
				value := current.Templates[index].Clone()
				template = &value
				break
			}
		}
		if template == nil {
			return current, false, tobari.ErrWorkspaceTemplateNotFound
		}
		for _, record := range current.Contexts {
			if record.Context.ProjectRoot == projectRoot && record.Context.TemplateID == templateID {
				return current, false, tobari.ErrContextBindingExists
			}
		}
		id, err := tobari.IssueContextID(m.clock().UTC(), m.entropy)
		if err != nil {
			return current, false, err
		}
		binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: id, ProjectRoot: projectRoot, TemplateID: templateID}
		memory, _, err := tobari.PublishPolicyMemory(id, []tobari.PolicyMemoryRule{}, nil)
		if err != nil {
			return current, false, err
		}
		record := tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory}
		contexts := append(cloneContextRecords(current.Contexts), record)
		next, changed, err := publishCollection(current, present, current.Templates, contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
		if err == nil {
			created = tobari.ContextAuthoritySnapshot{Context: binding, Template: template.Clone(), PolicyMemory: memory.Clone()}
		}
		return next, changed, err
	})
	return created.Clone(), resultErr
}

func (m *Mutator) DeleteContextByReference(ctx context.Context, ref string) (result tobari.ContextDeleteResult, resultErr error) {
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return result, err
	}
	resultErr = m.mutate(ctx, func(current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		index := contextRecordIndex(current, id)
		if index < 0 {
			return current, false, tobari.ErrContextBindingNotFound
		}
		for _, workspace := range current.Workspaces {
			if workspace.ContextID == id {
				return current, false, tobari.ErrContextBindingProtected
			}
		}
		if m.deletion == nil {
			return current, false, fmt.Errorf("Context credential deletion authority is unavailable")
		}
		if err := m.deletion.ConfirmContextCredentialAbsent(ctx, id); err != nil {
			return current, false, err
		}
		contexts := append(cloneContextRecords(current.Contexts[:index]), cloneContextRecords(current.Contexts[index+1:])...)
		candidates := make([]tobari.PolicyCandidateAuthority, 0, len(current.PendingCandidates))
		for _, candidate := range current.PendingCandidates {
			if candidate.ContextID != id {
				candidates = append(candidates, candidate.Clone())
			}
		}
		result = tobari.ContextDeleteResult{ContextID: id, Deleted: true}
		next, changed, err := publishCollection(current, present, current.Templates, contexts, current.Workspaces, candidates, current.DefaultTemplateID)
		return next, changed, err
	})
	return result, resultErr
}

func (m *Mutator) DeleteWorkspaceByReference(ctx context.Context, ref string, force bool) (result tobari.WorkspaceAuthorityDeleteResult, resultErr error) {
	id, err := tobari.ParseWorkspaceRef(ref)
	if err != nil {
		return result, err
	}
	operation := "workspace-delete"
	if force {
		operation = "workspace-delete-force"
	}
	committedDecision, resultErr := m.effectfulMutate(ctx, operation, ref, func(current tobari.WorkspaceAuthorityCollection, recovering bool) (effectPlan, error) {
		index := -1
		for candidateIndex := range current.Workspaces {
			if current.Workspaces[candidateIndex].ID == id {
				index = candidateIndex
				break
			}
		}
		if index < 0 {
			return effectPlan{}, tobari.ErrWorkspaceBindingNotFound
		}
		if m.deletion == nil {
			return effectPlan{}, fmt.Errorf("Workspace retirement authority is unavailable")
		}
		workspace := current.Workspaces[index]
		if !recovering {
			if err := m.deletion.ConfirmWorkspaceRetirementAllowed(ctx, workspace, force); err != nil {
				return effectPlan{}, err
			}
		}
		workspaces := append(cloneWorkspaceBindings(current.Workspaces[:index]), cloneWorkspaceBindings(current.Workspaces[index+1:])...)
		candidates := make([]tobari.PolicyCandidateAuthority, 0, len(current.PendingCandidates))
		for _, candidate := range current.PendingCandidates {
			if candidate.ObservingWorkspaceID != id {
				candidates = append(candidates, candidate.Clone())
			}
		}
		result = tobari.WorkspaceAuthorityDeleteResult{WorkspaceID: id, Deleted: true}
		next, changed, err := publishCollection(current, true, current.Templates, current.Contexts, workspaces, candidates, current.DefaultTemplateID)
		if err != nil {
			return effectPlan{}, err
		}
		if !changed {
			return effectPlan{}, fmt.Errorf("Workspace deletion did not change authority")
		}
		workspaceID := id
		workspaceEvidence := cloneWorkspaceBindings([]tobari.WorkspaceBinding{workspace})[0]
		forceValue := force
		return effectPlan{
			next:     next,
			decision: effectDecision{WorkspaceID: &workspaceID, Workspace: &workspaceEvidence, Force: &forceValue},
			effect: func(effectContext context.Context) error {
				decisionRef := workspaceRetirementDecisionRef(workspace.ID, next.Revision)
				if err := m.deletion.RetireWorkspace(effectContext, workspace, force, decisionRef); err != nil {
					return err
				}
				return m.deletion.ConfirmWorkspaceRetired(effectContext, workspace, decisionRef)
			},
		}, nil
	})
	if resultErr == nil && !result.Deleted && committedDecision.WorkspaceID != nil {
		result = tobari.WorkspaceAuthorityDeleteResult{WorkspaceID: *committedDecision.WorkspaceID, Deleted: true}
	}
	return result, resultErr
}

func (m *Mutator) AllowPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return m.applyPolicyCandidate(ctx, ref, tobari.PolicyMemoryAllow)
}

func (m *Mutator) DenyPolicyCandidateByReference(ctx context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return m.applyPolicyCandidate(ctx, ref, tobari.PolicyMemoryDeny)
}

func (m *Mutator) applyPolicyCandidate(ctx context.Context, ref string, decision tobari.PolicyMemoryDecision) (publication tobari.PolicyCandidatePublication, resultErr error) {
	if err := tobari.ValidatePolicyCandidateID(ref); err != nil {
		return publication, err
	}
	operation := "policy-allow"
	if decision == tobari.PolicyMemoryDeny {
		operation = "policy-deny"
	}
	committedDecision, resultErr := m.effectfulMutate(ctx, operation, ref, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
		candidateIndex := -1
		for index := range current.PendingCandidates {
			if current.PendingCandidates[index].ID == ref {
				candidateIndex = index
				break
			}
		}
		if candidateIndex < 0 {
			return effectPlan{}, tobari.ErrPolicyMemoryTargetNotFound
		}
		candidate := current.PendingCandidates[candidateIndex].Clone()
		recordIndex := contextRecordIndex(current, candidate.ContextID)
		if recordIndex < 0 {
			return effectPlan{}, fmt.Errorf("Policy candidate Context is unavailable")
		}
		previous := current.Contexts[recordIndex].PolicyMemory.Clone()
		rule, err := tobari.NewPolicyMemoryRule(candidate.ContextID, decision, candidate.Effect.RuleBody(candidate.ID))
		if err != nil {
			return effectPlan{}, err
		}
		rules := append([]tobari.PolicyMemoryRule{}, previous.Rules...)
		rules = append(rules, rule)
		memory, changed, err := tobari.PublishPolicyMemory(candidate.ContextID, rules, &previous)
		if err != nil || !changed {
			if err == nil {
				err = fmt.Errorf("Policy candidate did not change authority")
			}
			return effectPlan{}, err
		}
		contexts := cloneContextRecords(current.Contexts)
		contexts[recordIndex].PolicyMemory = memory
		activationEffect, err := m.preparePolicyMemoryActivation(current, &contexts[recordIndex])
		if err != nil {
			return effectPlan{}, err
		}
		candidates := append(clonePolicyCandidates(current.PendingCandidates[:candidateIndex]), clonePolicyCandidates(current.PendingCandidates[candidateIndex+1:])...)
		next, collectionChanged, err := publishCollection(current, true, current.Templates, contexts, current.Workspaces, candidates, current.DefaultTemplateID)
		if err != nil {
			return effectPlan{}, err
		}
		snapshot, err := snapshotForContext(next, candidate.ContextID)
		if err != nil {
			return effectPlan{}, err
		}
		publication = tobari.PolicyCandidatePublication{
			Candidate: candidate, RuleID: rule.ID, Previous: previous,
			Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true},
		}
		if err := publication.ValidateFor(ref, decision); err != nil {
			return effectPlan{}, err
		}
		if !collectionChanged {
			return effectPlan{}, fmt.Errorf("Policy candidate did not change final authority")
		}
		candidateEvidence := candidate.Clone()
		previousEvidence := previous.Clone()
		return effectPlan{
			next:     next,
			decision: effectDecision{Candidate: &candidateEvidence, RuleID: rule.ID, Decision: decision, PreviousMemory: &previousEvidence},
			effect:   activationEffect,
		}, nil
	})
	if resultErr == nil && publication.Candidate.ID == "" {
		resultErr = m.recoverCandidatePublication(ctx, committedDecision, ref, decision, &publication)
	}
	return publication, resultErr
}

func (m *Mutator) ResetPolicyMemoryRuleByReference(ctx context.Context, ref string) (publication tobari.PolicyRuleResetPublication, resultErr error) {
	if err := tobari.ValidatePolicyMemoryRuleID(ref); err != nil {
		return publication, err
	}
	committedDecision, resultErr := m.effectfulMutate(ctx, "policy-reset", ref, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
		recordIndex, ruleIndex := -1, -1
		for candidateRecord := range current.Contexts {
			for candidateRule := range current.Contexts[candidateRecord].PolicyMemory.Rules {
				if current.Contexts[candidateRecord].PolicyMemory.Rules[candidateRule].ID == ref {
					if recordIndex >= 0 {
						return effectPlan{}, fmt.Errorf("Policy Memory rule authority is ambiguous")
					}
					recordIndex, ruleIndex = candidateRecord, candidateRule
				}
			}
		}
		if recordIndex < 0 {
			return effectPlan{}, tobari.ErrPolicyMemoryTargetNotFound
		}
		previous := current.Contexts[recordIndex].PolicyMemory.Clone()
		rules := append([]tobari.PolicyMemoryRule{}, previous.Rules[:ruleIndex]...)
		rules = append(rules, previous.Rules[ruleIndex+1:]...)
		memory, changed, err := tobari.PublishPolicyMemory(previous.ContextID, rules, &previous)
		if err != nil || !changed {
			if err == nil {
				err = fmt.Errorf("Policy Memory reset did not change authority")
			}
			return effectPlan{}, err
		}
		contexts := cloneContextRecords(current.Contexts)
		contexts[recordIndex].PolicyMemory = memory
		activationEffect, err := m.preparePolicyMemoryActivation(current, &contexts[recordIndex])
		if err != nil {
			return effectPlan{}, err
		}
		next, collectionChanged, err := publishCollection(current, true, current.Templates, contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
		if err != nil {
			return effectPlan{}, err
		}
		snapshot, err := snapshotForContext(next, previous.ContextID)
		if err != nil {
			return effectPlan{}, err
		}
		publication = tobari.PolicyRuleResetPublication{
			RuleID: ref, RemovedFrom: previous,
			Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true},
		}
		if err := publication.ValidateFor(ref); err != nil {
			return effectPlan{}, err
		}
		if !collectionChanged {
			return effectPlan{}, fmt.Errorf("Policy reset did not change final authority")
		}
		previousEvidence := previous.Clone()
		return effectPlan{
			next:     next,
			decision: effectDecision{RuleID: ref, PreviousMemory: &previousEvidence},
			effect:   activationEffect,
		}, nil
	})
	if resultErr == nil && publication.RuleID == "" {
		resultErr = m.recoverResetPublication(ctx, committedDecision, ref, &publication)
	}
	return publication, resultErr
}

func (m *Mutator) preparePolicyMemoryActivation(current tobari.WorkspaceAuthorityCollection, record *tobari.WorkspaceAuthorityContextRecord) (func(context.Context) error, error) {
	if m.activation == nil {
		return nil, fmt.Errorf("Policy Memory activation authority is unavailable")
	}
	templateIndex := -1
	for index := range current.Templates {
		if current.Templates[index].ID == record.Context.TemplateID {
			templateIndex = index
			break
		}
	}
	if templateIndex < 0 {
		return nil, fmt.Errorf("Policy Memory Template authority is unavailable")
	}
	var workspace *tobari.WorkspaceBinding
	for index := range current.Workspaces {
		if current.Workspaces[index].ContextID == record.Context.ID {
			value := cloneWorkspaceBindings(current.Workspaces[index : index+1])[0]
			workspace = &value
			break
		}
	}
	desired := tobari.ContextAuthoritySnapshot{
		Context: record.Context, Template: current.Templates[templateIndex].Clone(), PolicyMemory: record.PolicyMemory.Clone(),
		ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemory: record.ActivePolicyMemory,
		ActivePolicyMemoryRef: record.ActivePolicyMemoryRef, Workspace: workspace,
	}
	if err := desired.Validate(); err != nil {
		return nil, err
	}
	expectedReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: record.Context.ID, Revision: record.PolicyMemory.Revision}
	active := record.PolicyMemory.Clone()
	record.ActivePolicyMemory = &active
	record.ActivePolicyMemoryRef = &expectedReceipt
	return func(ctx context.Context) error {
		receipt, err := m.activation.ActivatePolicyMemory(ctx, desired.Clone(), active.Clone())
		if err != nil {
			return err
		}
		if err := receipt.ValidateFor(record.Context, active); err != nil || receipt != expectedReceipt {
			return fmt.Errorf("Policy Memory activation returned another authority: %w", err)
		}
		return nil
	}, nil
}

func (m *Mutator) effectfulMutate(ctx context.Context, operation, target string, planner func(tobari.WorkspaceAuthorityCollection, bool) (effectPlan, error)) (committed effectDecision, resultErr error) {
	if m == nil || m.store == nil || m.lifecycle == nil || m.rename == nil || m.sync == nil {
		return committed, fmt.Errorf("final Workspace authority mutator is unavailable")
	}
	resultErr = m.lifecycle.WithLifecycleLock(ctx, func(lockedContext context.Context) error {
		if err := lockedContext.Err(); err != nil {
			return err
		}
		if err := validateMutationDirectory(filepath.Dir(m.store.root), 0o700); err != nil {
			return fmt.Errorf("validate final Workspace authority parent: %w", err)
		}
		if err := m.reconcileDecisionArtifacts(); err != nil {
			return err
		}
		decision, active, err := m.readEffectDecision()
		if err != nil {
			return err
		}
		current, present, err := m.store.ReadComplete(lockedContext)
		if err != nil {
			return err
		}
		if !present {
			return fmt.Errorf("effectful final-authority mutation requires an existing complete envelope")
		}
		terminal, terminalPresent, err := m.readTerminalEffectDecision()
		if err != nil {
			return err
		}
		if !active && terminalPresent && terminal.Operation == operation && terminal.Target == target && m.terminalConsequenceCurrent(current, terminal) == nil {
			if err := m.confirmCommittedEffect(lockedContext, current, terminal); err != nil {
				return err
			}
			committed = terminal
			return nil
		}
		if active {
			if decision.Operation != operation || decision.Target != target {
				return fmt.Errorf("another final-authority mutation requires exact same-target recovery")
			}
			if current.Generation == decision.NextGeneration && current.Revision == decision.NextRevision {
				if err := m.confirmCommittedEffect(lockedContext, current, decision); err != nil {
					return err
				}
				committed = decision
				return m.clearEffectDecision()
			}
			if current.Generation != decision.PreviousGeneration || current.Revision != decision.PreviousRevision {
				return fmt.Errorf("active final-authority mutation crosses unexpected envelope authority")
			}
		}

		plan, err := planner(current.Clone(), active)
		if err != nil {
			return err
		}
		encoded, err := EncodeComplete(plan.next)
		if err != nil {
			return err
		}
		complete := plan.decision
		complete.SchemaVersion = effectDecisionSchemaVersion
		complete.Operation = operation
		complete.Target = target
		complete.PreviousGeneration = current.Generation
		complete.PreviousRevision = current.Revision
		complete.NextGeneration = plan.next.Generation
		complete.NextRevision = plan.next.Revision
		if err := complete.validate(); err != nil {
			return err
		}
		if active {
			if !reflect.DeepEqual(decision, complete) {
				return fmt.Errorf("same-target recovery does not match the durable effect decision")
			}
			if err := m.validatePreparedStage(encoded); err != nil {
				return err
			}
		} else {
			if err := m.reconcileStage(); err != nil {
				return err
			}
			if err := m.prepareEffectStage(encoded); err != nil {
				return err
			}
			if err := m.writeEffectDecision(complete); err != nil {
				return err
			}
		}
		if err := plan.effect(lockedContext); err != nil {
			return err
		}
		// Once the external authority confirms the exact decision, cancellation
		// cannot turn success into replay permission. Process death remains
		// recoverable from the durable decision.
		if err := m.publishPreparedEffect(current, plan.next, encoded); err != nil {
			return err
		}
		committed = complete
		return m.clearEffectDecision()
	})
	return committed, resultErr
}

func (m *Mutator) terminalConsequenceCurrent(current tobari.WorkspaceAuthorityCollection, decision effectDecision) error {
	switch decision.Operation {
	case "workspace-delete", "workspace-delete-force":
		for _, workspace := range current.Workspaces {
			if workspace.ID == *decision.WorkspaceID {
				return fmt.Errorf("terminal Workspace delete target is present")
			}
		}
		return nil
	case "policy-allow", "policy-deny":
		if decision.Candidate == nil || decision.PreviousMemory == nil {
			return fmt.Errorf("terminal Policy candidate evidence is incomplete")
		}
		rule, err := tobari.NewPolicyMemoryRule(decision.Candidate.ContextID, decision.Decision, decision.Candidate.Effect.RuleBody(decision.Candidate.ID))
		if err != nil || rule.ID != decision.RuleID {
			return fmt.Errorf("terminal Policy candidate resulting rule is invalid")
		}
		rules := append([]tobari.PolicyMemoryRule{}, decision.PreviousMemory.Rules...)
		rules = append(rules, rule)
		want, changed, err := tobari.PublishPolicyMemory(decision.PreviousMemory.ContextID, rules, decision.PreviousMemory)
		if err != nil || !changed {
			return fmt.Errorf("terminal Policy candidate resulting memory is invalid")
		}
		return terminalPolicyMemoryCurrent(current, want)
	case "policy-reset":
		if decision.PreviousMemory == nil {
			return fmt.Errorf("terminal Policy reset evidence is incomplete")
		}
		rules := make([]tobari.PolicyMemoryRule, 0, len(decision.PreviousMemory.Rules)-1)
		found := false
		for _, rule := range decision.PreviousMemory.Rules {
			if rule.ID == decision.RuleID {
				found = true
				continue
			}
			rules = append(rules, rule.Clone())
		}
		if !found {
			return fmt.Errorf("terminal Policy reset rule was absent")
		}
		want, changed, err := tobari.PublishPolicyMemory(decision.PreviousMemory.ContextID, rules, decision.PreviousMemory)
		if err != nil || !changed {
			return fmt.Errorf("terminal Policy reset resulting memory is invalid")
		}
		return terminalPolicyMemoryCurrent(current, want)
	default:
		return fmt.Errorf("terminal effect operation is invalid")
	}
}

func terminalPolicyMemoryCurrent(current tobari.WorkspaceAuthorityCollection, want tobari.PolicyMemoryRevision) error {
	index := contextRecordIndex(current, want.ContextID)
	if index < 0 {
		return fmt.Errorf("terminal Policy Memory Context is absent")
	}
	record := current.Contexts[index]
	if record.PolicyMemory.Generation != want.Generation || record.PolicyMemory.Revision != want.Revision || record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil || record.ActivePolicyMemory.Generation != want.Generation || record.ActivePolicyMemory.Revision != want.Revision || record.ActivePolicyMemoryRef.Revision != want.Revision {
		return fmt.Errorf("terminal Policy Memory consequence is no longer current and active")
	}
	return nil
}

func (m *Mutator) confirmCommittedEffect(ctx context.Context, current tobari.WorkspaceAuthorityCollection, decision effectDecision) error {
	switch decision.Operation {
	case "workspace-delete", "workspace-delete-force":
		contextIndex := contextRecordIndex(current, decision.Workspace.ContextID)
		if contextIndex < 0 || decision.Workspace.ValidateFor(current.Contexts[contextIndex].Context) != nil {
			return fmt.Errorf("terminal Workspace retirement evidence crosses Context authority")
		}
		decisionRef := workspaceRetirementDecisionRef(*decision.WorkspaceID, decision.NextRevision)
		return m.deletion.ConfirmWorkspaceRetired(ctx, *decision.Workspace, decisionRef)
	case "policy-allow", "policy-deny", "policy-reset":
		if m.activation == nil || decision.PreviousMemory == nil {
			return fmt.Errorf("Policy Memory activation recovery authority is unavailable")
		}
		snapshot, err := snapshotForContext(current, decision.PreviousMemory.ContextID)
		if err != nil {
			return err
		}
		if snapshot.ActivePolicyMemoryRef == nil {
			return fmt.Errorf("committed Policy Memory has no activation receipt")
		}
		return m.activation.ConfirmPolicyMemoryActive(ctx, snapshot, *snapshot.ActivePolicyMemoryRef)
	default:
		return fmt.Errorf("final-authority effect recovery operation is invalid")
	}
}

func (m *Mutator) recoverCandidatePublication(ctx context.Context, decision effectDecision, ref string, expected tobari.PolicyMemoryDecision, target *tobari.PolicyCandidatePublication) error {
	if decision.Candidate == nil || decision.PreviousMemory == nil || decision.RuleID == "" || decision.Decision != expected || decision.Target != ref {
		return fmt.Errorf("committed Policy candidate recovery evidence is incomplete")
	}
	current, present, err := m.store.ReadComplete(ctx)
	if err != nil || !present {
		return fmt.Errorf("read committed Policy candidate authority: %w", err)
	}
	snapshot, err := snapshotForContext(current, decision.Candidate.ContextID)
	if err != nil {
		return err
	}
	*target = tobari.PolicyCandidatePublication{
		Candidate: decision.Candidate.Clone(), RuleID: decision.RuleID, Previous: decision.PreviousMemory.Clone(),
		Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: decision.PreviousMemory.Revision, Changed: true},
	}
	return target.ValidateFor(ref, expected)
}

func (m *Mutator) recoverResetPublication(ctx context.Context, decision effectDecision, ref string, target *tobari.PolicyRuleResetPublication) error {
	if decision.PreviousMemory == nil || decision.RuleID != ref || decision.Target != ref {
		return fmt.Errorf("committed Policy reset recovery evidence is incomplete")
	}
	current, present, err := m.store.ReadComplete(ctx)
	if err != nil || !present {
		return fmt.Errorf("read committed Policy reset authority: %w", err)
	}
	snapshot, err := snapshotForContext(current, decision.PreviousMemory.ContextID)
	if err != nil {
		return err
	}
	*target = tobari.PolicyRuleResetPublication{
		RuleID: ref, RemovedFrom: decision.PreviousMemory.Clone(),
		Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: decision.PreviousMemory.Revision, Changed: true},
	}
	return target.ValidateFor(ref)
}

func workspaceRetirementDecisionRef(id tobari.WorkspaceID, revision tobari.SemanticDigest) string {
	return "workspace-retirement:" + string(id) + ":" + string(revision)
}

func (m *Mutator) effectDecisionPath() string     { return m.store.root + ".wp11-mutation-decision.json" }
func (m *Mutator) effectDecisionTempPath() string { return m.effectDecisionPath() + ".tmp" }
func (m *Mutator) effectDecisionDonePath() string { return m.effectDecisionPath() + ".done" }

func (m *Mutator) readTerminalEffectDecision() (effectDecision, bool, error) {
	path := m.effectDecisionDonePath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return effectDecision{}, false, nil
	} else if err != nil {
		return effectDecision{}, false, err
	}
	data, err := readAuthorityFile(path)
	if err != nil {
		return effectDecision{}, false, fmt.Errorf("read terminal final-authority effect decision: %w", err)
	}
	var decision effectDecision
	if err := decodeStrictJSON(data, &decision); err != nil {
		return effectDecision{}, false, fmt.Errorf("decode terminal final-authority effect decision: %w", err)
	}
	if err := decision.validate(); err != nil {
		return effectDecision{}, false, err
	}
	return decision, true, nil
}

func (m *Mutator) readEffectDecision() (effectDecision, bool, error) {
	path := m.effectDecisionPath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return effectDecision{}, false, nil
	} else if err != nil {
		return effectDecision{}, false, err
	}
	data, err := readAuthorityFile(path)
	if err != nil {
		return effectDecision{}, false, fmt.Errorf("read final-authority effect decision: %w", err)
	}
	var decision effectDecision
	if err := decodeStrictJSON(data, &decision); err != nil {
		return effectDecision{}, false, fmt.Errorf("decode final-authority effect decision: %w", err)
	}
	if err := decision.validate(); err != nil {
		return effectDecision{}, false, err
	}
	return decision, true, nil
}

func (m *Mutator) writeEffectDecision(decision effectDecision) error {
	if err := decision.validate(); err != nil {
		return err
	}
	buffer := boundedJSONBuffer{maximum: MaxAuthorityBytes}
	if err := writeJSONValue(&buffer, reflect.ValueOf(decision)); err != nil {
		return fmt.Errorf("encode final-authority effect decision: %w", err)
	}
	if _, present, err := m.readEffectDecision(); err != nil || present {
		if err == nil {
			err = fmt.Errorf("final-authority effect decision already exists")
		}
		return err
	}
	temporary := m.effectDecisionTempPath()
	if err := writeMutationFile(temporary, buffer.Bytes()); err != nil {
		return err
	}
	parent := filepath.Dir(temporary)
	if err := m.sync(parent); err != nil {
		return err
	}
	if err := m.rename(temporary, m.effectDecisionPath()); err != nil {
		return err
	}
	return m.sync(parent)
}

func (m *Mutator) clearEffectDecision() error {
	path := m.effectDecisionPath()
	done := m.effectDecisionDonePath()
	if info, err := os.Lstat(done); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
			return fmt.Errorf("terminal final-authority effect decision is unsafe")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := m.rename(path, done); err != nil {
		return fmt.Errorf("retire final-authority effect decision: %w", err)
	}
	parent := filepath.Dir(path)
	return m.sync(parent)
}

func (m *Mutator) reconcileDecisionArtifacts() error {
	for _, path := range []string{m.effectDecisionTempPath()} {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
			return fmt.Errorf("final-authority effect decision artifact is unsafe")
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := m.sync(filepath.Dir(path)); err != nil {
			return err
		}
	}
	if _, _, err := m.readTerminalEffectDecision(); err != nil {
		return err
	}
	return nil
}

func (m *Mutator) prepareEffectStage(encoded []byte) error {
	stage := m.store.root + ".wp11-mutation-stage"
	if err := writeMutationFile(stage, encoded); err != nil {
		return err
	}
	return m.sync(filepath.Dir(stage))
}

func (m *Mutator) validatePreparedStage(encoded []byte) error {
	data, err := readAuthorityFile(m.store.root + ".wp11-mutation-stage")
	if err != nil {
		return fmt.Errorf("read durable final-authority mutation stage: %w", err)
	}
	if !bytes.Equal(data, encoded) {
		return fmt.Errorf("durable final-authority mutation stage does not match the active decision")
	}
	return nil
}

func (m *Mutator) publishPreparedEffect(previous, next tobari.WorkspaceAuthorityCollection, encoded []byte) error {
	stage := m.store.root + ".wp11-mutation-stage"
	if err := m.rename(stage, filepath.Join(m.store.root, authorityFileName)); err != nil {
		return m.classifyPublication(previous, true, next, encoded, err)
	}
	if err := m.sync(m.store.root); err != nil {
		return m.classifyPublication(previous, true, next, encoded, err)
	}
	return m.classifyPublication(previous, true, next, encoded, nil)
}

type collectionMutation func(tobari.WorkspaceAuthorityCollection, bool) (tobari.WorkspaceAuthorityCollection, bool, error)

func (m *Mutator) mutate(ctx context.Context, change collectionMutation) error {
	if m == nil || m.store == nil || m.lifecycle == nil || m.clock == nil || m.entropy == nil || m.rename == nil || m.sync == nil {
		return fmt.Errorf("final Workspace authority mutator is unavailable")
	}
	return m.lifecycle.WithLifecycleLock(ctx, func(lockedContext context.Context) error {
		if err := lockedContext.Err(); err != nil {
			return err
		}
		if err := validateMutationDirectory(filepath.Dir(m.store.root), 0o700); err != nil {
			return fmt.Errorf("validate final Workspace authority parent: %w", err)
		}
		if err := m.reconcileDecisionArtifacts(); err != nil {
			return err
		}
		if _, active, err := m.readEffectDecision(); err != nil {
			return err
		} else if active {
			return fmt.Errorf("final-authority mutation requires exact active-decision recovery")
		}
		if err := m.reconcileStage(); err != nil {
			return err
		}
		current, present, err := m.store.ReadComplete(lockedContext)
		if err != nil {
			return err
		}
		if !present {
			current.Templates = []tobari.WorkspaceTemplate{}
			current.Contexts = []tobari.WorkspaceAuthorityContextRecord{}
			current.Workspaces = []tobari.WorkspaceBinding{}
			current.PendingCandidates = []tobari.PolicyCandidateAuthority{}
		}
		next, changed, err := change(current.Clone(), present)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		encoded, err := EncodeComplete(next)
		if err != nil {
			return err
		}
		if err := lockedContext.Err(); err != nil {
			return err
		}
		return m.publish(current, present, next, encoded)
	})
}

func publishCollection(current tobari.WorkspaceAuthorityCollection, present bool, templates []tobari.WorkspaceTemplate, contexts []tobari.WorkspaceAuthorityContextRecord, workspaces []tobari.WorkspaceBinding, candidates []tobari.PolicyCandidateAuthority, defaultID *tobari.WorkspaceTemplateID) (tobari.WorkspaceAuthorityCollection, bool, error) {
	if present {
		return tobari.PublishWorkspaceAuthorityCollection(templates, contexts, workspaces, candidates, defaultID, &current)
	}
	return tobari.PublishWorkspaceAuthorityCollection(templates, contexts, workspaces, candidates, defaultID, nil)
}

func (m *Mutator) publish(previous tobari.WorkspaceAuthorityCollection, present bool, next tobari.WorkspaceAuthorityCollection, encoded []byte) error {
	parent := filepath.Dir(m.store.root)
	if err := validateMutationDirectory(parent, 0o700); err != nil {
		return fmt.Errorf("validate final Workspace authority parent: %w", err)
	}
	stage := m.store.root + ".wp11-mutation-stage"
	if present {
		if err := writeMutationFile(stage, encoded); err != nil {
			return err
		}
		if err := m.rename(stage, filepath.Join(m.store.root, authorityFileName)); err != nil {
			return m.classifyPublication(previous, present, next, encoded, err)
		}
		if err := m.sync(m.store.root); err != nil {
			return m.classifyPublication(previous, present, next, encoded, err)
		}
		return m.classifyPublication(previous, present, next, encoded, nil)
	}
	if err := os.Mkdir(stage, 0o700); err != nil {
		return fmt.Errorf("create final Workspace authority stage: %w", err)
	}
	if err := writeMutationFile(filepath.Join(stage, authorityFileName), encoded); err != nil {
		return err
	}
	if err := m.sync(stage); err != nil {
		return err
	}
	if err := m.rename(stage, m.store.root); err != nil {
		return m.classifyPublication(previous, present, next, encoded, err)
	}
	if err := m.sync(parent); err != nil {
		return m.classifyPublication(previous, present, next, encoded, err)
	}
	return m.classifyPublication(previous, present, next, encoded, nil)
}

func (m *Mutator) classifyPublication(previous tobari.WorkspaceAuthorityCollection, previousPresent bool, next tobari.WorkspaceAuthorityCollection, encoded []byte, effectErr error) error {
	observed, present, readErr := m.readPublishedComplete()
	if readErr == nil && present && observed.Generation == next.Generation && observed.Revision == next.Revision {
		actual, fileErr := readAuthorityFile(filepath.Join(m.store.root, authorityFileName))
		if fileErr == nil && bytes.Equal(actual, encoded) {
			return nil
		}
	}
	if readErr == nil && previousPresent && present && observed.Generation == previous.Generation && observed.Revision == previous.Revision {
		return fmt.Errorf("publish final Workspace authority had no effect: %w", effectErr)
	}
	if readErr == nil && !previousPresent && !present {
		return fmt.Errorf("publish final Workspace authority had no effect: %w", effectErr)
	}
	if effectErr == nil {
		effectErr = fmt.Errorf("exact publication read-back failed")
	}
	return fmt.Errorf("classify final Workspace authority publication: %w", errors.Join(effectErr, readErr))
}

// readPublishedComplete is the bounded post-effect classifier. It deliberately
// has no cancellation point: after rename, the adapter must distinguish exact
// success from no effect before returning replay guidance. Ordinary reads keep
// using Store.ReadComplete and propagate their caller context.
func (m *Mutator) readPublishedComplete() (tobari.WorkspaceAuthorityCollection, bool, error) {
	rootInfo, err := os.Lstat(m.store.root)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceAuthorityCollection{}, false, nil
	}
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() || rootInfo.Mode().Perm() != 0o700 || !ownedByCurrentUser(rootInfo) {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority root is unsafe after publication")
	}
	entries, err := os.ReadDir(m.store.root)
	if err != nil || len(entries) != 1 || entries[0].Name() != authorityFileName {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority root is partial after publication")
	}
	data, err := readAuthorityFile(filepath.Join(m.store.root, authorityFileName))
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	var collection tobari.WorkspaceAuthorityCollection
	if err := decodeStrictJSON(data, &collection); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := validateCollectionBounds(collection); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := collection.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	return collection, true, nil
}

func (m *Mutator) reconcileStage() error {
	stage := m.store.root + ".wp11-mutation-stage"
	info, err := os.Lstat(stage)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect final Workspace authority mutation stage: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(info) {
		return fmt.Errorf("final Workspace authority mutation stage is unsafe")
	}
	if info.IsDir() {
		if info.Mode().Perm() != 0o700 {
			return fmt.Errorf("final Workspace authority mutation stage directory is unsafe")
		}
		entries, err := os.ReadDir(stage)
		if err != nil {
			return err
		}
		if len(entries) > 1 || (len(entries) == 1 && entries[0].Name() != authorityFileName) {
			return fmt.Errorf("final Workspace authority mutation stage is foreign or mixed")
		}
		if len(entries) == 1 {
			path := filepath.Join(stage, authorityFileName)
			if _, err := readAuthorityFile(path); err != nil {
				// A process may stop after creating or while writing the owned
				// fixed child. Only the exact real owner-only child is reclaimed.
				child, inspectErr := os.Lstat(path)
				if inspectErr != nil || child.Mode()&os.ModeSymlink != 0 || !child.Mode().IsRegular() || child.Mode().Perm() != 0o600 || !ownedByCurrentUser(child) {
					return fmt.Errorf("final Workspace authority mutation stage child is unsafe")
				}
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("reconcile final Workspace authority mutation stage file: %w", err)
			}
		}
		if err := os.Remove(stage); err != nil {
			return fmt.Errorf("reconcile final Workspace authority mutation stage directory: %w", err)
		}
		return m.sync(filepath.Dir(stage))
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("final Workspace authority mutation stage file is unsafe")
	}
	// A partial regular file at the one reserved stage path is safely replaced
	// under the installation lock; writeMutationFile truncates it before use.
	return nil
}

func writeMutationFile(path string, data []byte) (resultErr error) {
	info, err := os.Lstat(path)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info)) {
		return fmt.Errorf("final Workspace authority mutation stage file is unsafe")
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G304 -- exact reserved stage path under the lifecycle lock.
	if err != nil {
		return fmt.Errorf("open final Workspace authority mutation stage: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write final Workspace authority mutation stage: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync final Workspace authority mutation stage: %w", err)
	}
	return nil
}

func validateMutationDirectory(path string, mode os.FileMode) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != mode || !ownedByCurrentUser(info) {
		return fmt.Errorf("directory must be real and owner-only")
	}
	return nil
}

func syncMutationDirectory(path string) (resultErr error) {
	directory, err := os.Open(path) // #nosec G304 -- caller validates or owns the exact directory.
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := directory.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	return directory.Sync()
}

func templateExists(collection tobari.WorkspaceAuthorityCollection, id tobari.WorkspaceTemplateID) bool {
	for _, template := range collection.Templates {
		if template.ID == id {
			return true
		}
	}
	return false
}

func contextRecordIndex(collection tobari.WorkspaceAuthorityCollection, id tobari.ContextID) int {
	for index := range collection.Contexts {
		if collection.Contexts[index].Context.ID == id {
			return index
		}
	}
	return -1
}

func snapshotForContext(collection tobari.WorkspaceAuthorityCollection, id tobari.ContextID) (tobari.ContextAuthoritySnapshot, error) {
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.ID == id {
			return snapshot.Clone(), nil
		}
	}
	return tobari.ContextAuthoritySnapshot{}, tobari.ErrContextBindingNotFound
}

func cloneContextRecords(values []tobari.WorkspaceAuthorityContextRecord) []tobari.WorkspaceAuthorityContextRecord {
	result := make([]tobari.WorkspaceAuthorityContextRecord, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}

func cloneWorkspaceBindings(values []tobari.WorkspaceBinding) []tobari.WorkspaceBinding {
	result := make([]tobari.WorkspaceBinding, len(values))
	copy(result, values)
	for index := range result {
		if values[index].LastSuccessfulEntry != nil {
			entry := *values[index].LastSuccessfulEntry
			result[index].LastSuccessfulEntry = &entry
		}
	}
	return result
}

func clonePolicyCandidates(values []tobari.PolicyCandidateAuthority) []tobari.PolicyCandidateAuthority {
	result := make([]tobari.PolicyCandidateAuthority, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}
