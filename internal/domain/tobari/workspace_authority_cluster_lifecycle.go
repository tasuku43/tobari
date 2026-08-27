package tobari

import (
	"fmt"
	"reflect"
)

const (
	FinalClusterStatusSchemaVersion = 3
	FinalClusterUpSchemaVersion     = 3
	FinalClusterDownSchemaVersion   = 3
)

type FinalClusterAuthorityState string

const (
	FinalClusterAuthorityAbsent  FinalClusterAuthorityState = "absent"
	FinalClusterAuthorityPresent FinalClusterAuthorityState = "present"
)

type FinalClusterRuntimeState string

const (
	FinalClusterRuntimeAbsent    FinalClusterRuntimeState = "absent"
	FinalClusterRuntimeStopped   FinalClusterRuntimeState = "stopped"
	FinalClusterRuntimeRunning   FinalClusterRuntimeState = "running"
	FinalClusterRuntimeUnhealthy FinalClusterRuntimeState = "unhealthy"
	FinalClusterRuntimeDrifted   FinalClusterRuntimeState = "drifted"
	FinalClusterRuntimeUnknown   FinalClusterRuntimeState = "unknown"
)

func (s FinalClusterRuntimeState) Validate() error {
	switch s {
	case FinalClusterRuntimeAbsent, FinalClusterRuntimeStopped, FinalClusterRuntimeRunning,
		FinalClusterRuntimeUnhealthy, FinalClusterRuntimeDrifted, FinalClusterRuntimeUnknown:
		return nil
	default:
		return fmt.Errorf("final cluster runtime state is invalid")
	}
}

type FinalClusterReceiptState string

const (
	FinalClusterReceiptAbsent  FinalClusterReceiptState = "absent"
	FinalClusterReceiptActive  FinalClusterReceiptState = "active"
	FinalClusterReceiptStopped FinalClusterReceiptState = "stopped"
	FinalClusterReceiptDrifted FinalClusterReceiptState = "drifted"
	FinalClusterReceiptUnknown FinalClusterReceiptState = "unknown"
)

func (s FinalClusterReceiptState) Validate() error {
	switch s {
	case FinalClusterReceiptAbsent, FinalClusterReceiptActive, FinalClusterReceiptStopped,
		FinalClusterReceiptDrifted, FinalClusterReceiptUnknown:
		return nil
	default:
		return fmt.Errorf("final cluster receipt state is invalid")
	}
}

type FinalClusterComponentObservation struct {
	Name     string                    `json:"name"`
	State    FinalClusterRuntimeState  `json:"state"`
	Health   string                    `json:"health,omitempty"`
	Identity FinalClusterEvidenceState `json:"identity"`
	Topology FinalClusterEvidenceState `json:"topology"`
}

type FinalClusterEvidenceState string

const (
	FinalClusterEvidenceAbsent  FinalClusterEvidenceState = "absent"
	FinalClusterEvidenceExact   FinalClusterEvidenceState = "exact"
	FinalClusterEvidenceDrifted FinalClusterEvidenceState = "drifted"
	FinalClusterEvidenceUnknown FinalClusterEvidenceState = "unknown"
)

func (s FinalClusterEvidenceState) Validate() error {
	switch s {
	case FinalClusterEvidenceAbsent, FinalClusterEvidenceExact, FinalClusterEvidenceDrifted, FinalClusterEvidenceUnknown:
		return nil
	}
	return fmt.Errorf("final cluster evidence state is invalid")
}

func (o FinalClusterComponentObservation) Validate() error {
	if o.Name != "gateway" && o.Name != "opa" && o.Name != "auth-broker" && o.Name != "credential-companion" || o.State.Validate() != nil || o.Identity.Validate() != nil || o.Topology.Validate() != nil {
		return fmt.Errorf("final cluster component observation is invalid")
	}
	if o.State == FinalClusterRuntimeAbsent {
		if o.Health != "" || o.Identity != FinalClusterEvidenceAbsent || o.Topology != FinalClusterEvidenceAbsent {
			return fmt.Errorf("absent final cluster component contains runtime identity")
		}
	}
	return nil
}

type FinalClusterContextReceiptObservation struct {
	ContextID      ContextID                        `json:"context_id"`
	TemplatePolicy *TemplatePolicyActivationReceipt `json:"template_policy,omitempty"`
	PolicyMemory   *PolicyMemoryActivationReceipt   `json:"policy_memory,omitempty"`
}

func (o FinalClusterContextReceiptObservation) Validate() error {
	if o.ContextID.Validate() != nil {
		return fmt.Errorf("final cluster Context receipt identity is invalid")
	}
	if (o.TemplatePolicy == nil) != (o.PolicyMemory == nil) {
		return fmt.Errorf("final cluster Context has a partial active receipt")
	}
	return nil
}

// FinalClusterStatus is one bounded, task-owned observation of final authority
// and the exact selected Gateway/OPA runtime. Unknown is data, never absence.
type FinalClusterStatus struct {
	SchemaVersion      int                        `json:"schema_version"`
	Task               string                     `json:"task"`
	Authority          FinalClusterAuthorityState `json:"authority"`
	Generation         uint64                     `json:"generation,omitempty"`
	CollectionRevision SemanticDigest             `json:"collection_revision,omitempty"`
	// The active aggregate identity is nullable while the final authority is
	// absent, unconfigured, stopped, or otherwise not active. When Receipt is
	// active all three values are required and are copied from the same active
	// publication receipt; no path is part of this public observation.
	AggregateRevision  *string                                 `json:"aggregate_revision"`
	EvaluatorIdentity  *PolicyEvaluatorIdentity                `json:"evaluator_identity"`
	PolicyDataIdentity *PolicyDataIdentity                     `json:"policy_data_identity"`
	TemplateCount      int                                     `json:"template_count"`
	ContextCount       int                                     `json:"context_count"`
	WorkspaceCount     int                                     `json:"workspace_count"`
	Runtime            FinalClusterRuntimeState                `json:"runtime"`
	Receipt            FinalClusterReceiptState                `json:"receipt"`
	Contexts           []FinalClusterContextReceiptObservation `json:"contexts"`
	Components         []FinalClusterComponentObservation      `json:"components"`
}

// ActivePolicyProjectionIdentity returns the exact aggregate identity carried
// by an active status observation. Inactive observations deliberately expose
// the same public keys as explicit nulls, but cannot be used as authority for
// a denial or mutation result.
func (s FinalClusterStatus) ActivePolicyProjectionIdentity() (PolicyProjectionIdentity, error) {
	if err := s.Validate(); err != nil {
		return PolicyProjectionIdentity{}, err
	}
	if s.Authority != FinalClusterAuthorityPresent || s.Receipt != FinalClusterReceiptActive ||
		s.AggregateRevision == nil || s.EvaluatorIdentity == nil || s.PolicyDataIdentity == nil {
		return PolicyProjectionIdentity{}, fmt.Errorf("final cluster status has no active aggregate identity")
	}
	identity := PolicyProjectionIdentity{
		AggregateRevision:  *s.AggregateRevision,
		EvaluatorIdentity:  *s.EvaluatorIdentity,
		PolicyDataIdentity: *s.PolicyDataIdentity,
	}
	return identity, identity.Validate()
}

func (s FinalClusterStatus) Validate() error {
	if s.SchemaVersion != FinalClusterStatusSchemaVersion || s.Task != TaskClusterStatus ||
		s.Runtime.Validate() != nil || s.Receipt.Validate() != nil || s.Contexts == nil || s.Components == nil {
		return fmt.Errorf("final cluster status metadata is invalid")
	}
	hasAggregateIdentity := s.AggregateRevision != nil || s.EvaluatorIdentity != nil || s.PolicyDataIdentity != nil
	if s.Authority == FinalClusterAuthorityAbsent {
		if s.Generation != 0 || s.CollectionRevision != "" || s.TemplateCount != 0 || s.ContextCount != 0 ||
			s.WorkspaceCount != 0 || len(s.Contexts) != 0 || hasAggregateIdentity || s.Receipt != FinalClusterReceiptAbsent && s.Receipt != FinalClusterReceiptUnknown && s.Receipt != FinalClusterReceiptDrifted {
			return fmt.Errorf("absent final cluster status contains authority")
		}
	} else if s.Authority != FinalClusterAuthorityPresent || s.Generation == 0 || s.CollectionRevision.Validate() != nil ||
		s.TemplateCount < 0 || s.ContextCount < 0 || s.WorkspaceCount < 0 || len(s.Contexts) != s.ContextCount {
		return fmt.Errorf("present final cluster status is incomplete")
	}
	if s.Authority == FinalClusterAuthorityPresent && s.Receipt == FinalClusterReceiptActive {
		if s.AggregateRevision == nil || s.EvaluatorIdentity == nil || s.PolicyDataIdentity == nil {
			return fmt.Errorf("active final cluster status omits aggregate identity")
		}
		identity := PolicyProjectionIdentity{AggregateRevision: *s.AggregateRevision, EvaluatorIdentity: *s.EvaluatorIdentity, PolicyDataIdentity: *s.PolicyDataIdentity}
		if err := identity.Validate(); err != nil {
			return fmt.Errorf("active final cluster status aggregate identity is invalid: %w", err)
		}
	} else if hasAggregateIdentity {
		return fmt.Errorf("inactive final cluster status contains aggregate identity")
	}
	for _, item := range s.Contexts {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	for _, item := range s.Components {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// WorkspaceAuthorityClusterDownPlan clears active axes in one exact envelope
// transition and binds whether exact shared volumes are purged. It cannot
// authorize removal while a Workspace remains.
type WorkspaceAuthorityClusterDownPlan struct {
	SchemaVersion      int            `json:"schema_version"`
	Purge              bool           `json:"purge"`
	PreviousGeneration uint64         `json:"previous_generation"`
	PreviousRevision   SemanticDigest `json:"previous_revision"`
	NextGeneration     uint64         `json:"next_generation"`
	NextRevision       SemanticDigest `json:"next_revision"`
	EnvelopeChanged    bool           `json:"envelope_changed"`
}

func (p WorkspaceAuthorityClusterDownPlan) Validate() error {
	if p.SchemaVersion != 1 || p.PreviousGeneration == 0 || p.PreviousRevision.Validate() != nil || p.NextRevision.Validate() != nil {
		return fmt.Errorf("final cluster down plan metadata is invalid")
	}
	if p.EnvelopeChanged {
		if p.NextGeneration != p.PreviousGeneration+1 || p.NextRevision == p.PreviousRevision {
			return fmt.Errorf("final cluster down transition is invalid")
		}
	} else if p.NextGeneration != p.PreviousGeneration || p.NextRevision != p.PreviousRevision {
		return fmt.Errorf("final cluster down no-op is invalid")
	}
	return nil
}

type WorkspaceAuthorityClusterDownTransition struct {
	Plan WorkspaceAuthorityClusterDownPlan
	Next WorkspaceAuthorityCollection
}

func PlanWorkspaceAuthorityClusterDown(previous WorkspaceAuthorityCollection) (WorkspaceAuthorityClusterDownTransition, error) {
	return PlanWorkspaceAuthorityClusterDownWithPurge(previous, false)
}

// PlanWorkspaceAuthorityClusterDownWithPurge binds the caller's exact volume
// retention choice into the durable effect plan.
func PlanWorkspaceAuthorityClusterDownWithPurge(previous WorkspaceAuthorityCollection, purge bool) (WorkspaceAuthorityClusterDownTransition, error) {
	if err := previous.Validate(); err != nil {
		return WorkspaceAuthorityClusterDownTransition{}, err
	}
	if len(previous.Workspaces) != 0 {
		return WorkspaceAuthorityClusterDownTransition{}, fmt.Errorf("final cluster down requires zero Workspaces")
	}
	contexts := make([]WorkspaceAuthorityContextRecord, len(previous.Contexts))
	for i := range previous.Contexts {
		contexts[i] = previous.Contexts[i].Clone()
		contexts[i].ActiveTemplatePolicy = nil
		contexts[i].ActivePolicyMemory = nil
		contexts[i].ActivePolicyMemoryRef = nil
	}
	next, changed, err := PublishWorkspaceAuthorityCollection(previous.Templates, contexts, previous.Workspaces, previous.PendingCandidates, previous.DefaultTemplateID, &previous)
	if err != nil {
		return WorkspaceAuthorityClusterDownTransition{}, err
	}
	plan := WorkspaceAuthorityClusterDownPlan{SchemaVersion: 1, Purge: purge, PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision, NextGeneration: next.Generation, NextRevision: next.Revision, EnvelopeChanged: changed}
	if err := plan.ValidateTransition(previous, next); err != nil {
		return WorkspaceAuthorityClusterDownTransition{}, err
	}
	return WorkspaceAuthorityClusterDownTransition{Plan: plan, Next: next}, nil
}

func (p WorkspaceAuthorityClusterDownPlan) ValidateTransition(previous, next WorkspaceAuthorityCollection) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if previous.Validate() != nil || next.Validate() != nil || len(previous.Workspaces) != 0 || len(next.Workspaces) != 0 ||
		p.PreviousGeneration != previous.Generation || p.PreviousRevision != previous.Revision || p.NextGeneration != next.Generation || p.NextRevision != next.Revision {
		return fmt.Errorf("final cluster down plan does not bind its envelope transition")
	}
	for _, record := range next.Contexts {
		if record.ActiveTemplatePolicy != nil || record.ActivePolicyMemory != nil || record.ActivePolicyMemoryRef != nil {
			return fmt.Errorf("final cluster down retained active Context authority")
		}
	}
	want, changed, err := PublishWorkspaceAuthorityCollection(previous.Templates, next.Contexts, previous.Workspaces, previous.PendingCandidates, previous.DefaultTemplateID, &previous)
	if err != nil || changed != p.EnvelopeChanged || !reflect.DeepEqual(want, next) {
		return fmt.Errorf("final cluster down changed preserved authority")
	}
	return nil
}

func (p WorkspaceAuthorityClusterDownPlan) ValidateCurrent(current WorkspaceAuthorityCollection) error {
	if err := current.Validate(); err != nil {
		return err
	}
	if current.Generation != p.NextGeneration || current.Revision != p.NextRevision || len(current.Workspaces) != 0 {
		return fmt.Errorf("final cluster down consequence is not current")
	}
	for _, record := range current.Contexts {
		if record.ActiveTemplatePolicy != nil || record.ActivePolicyMemory != nil || record.ActivePolicyMemoryRef != nil {
			return fmt.Errorf("final cluster down consequence retained active receipts")
		}
	}
	return nil
}
