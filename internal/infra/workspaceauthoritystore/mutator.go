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

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const finalMutationSelectionSettlementTimeout = 30 * time.Second

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
	PrepareWorkspaceRetirement(context.Context, tobari.WorkspaceBinding, bool, string) error
	CompleteWorkspaceRetirement(context.Context, tobari.WorkspaceBinding, bool, string) error
	ConfirmWorkspaceRetired(context.Context, tobari.WorkspaceBinding, string) error
}

// ContextCredentialAbsenceAuthority is separate from Workspace retirement so
// the final research adapter, not a predecessor deletion adapter, owns the
// exhaustive Context-scoped Broker inventory prerequisite.
type ContextCredentialAbsenceAuthority interface {
	ConfirmContextCredentialAbsent(context.Context, tobari.ContextID) error
}

// WorkspaceTemplateRuntimeRevisionAuthority resolves one unchanged managed
// Runtime revision reference through the coherent WP03 lifecycle authority.
// It is invoked only while the installation lifecycle lock is held.
type WorkspaceTemplateRuntimeRevisionAuthority interface {
	ResolveWorkspaceTemplateRuntimeRevision(context.Context, string) (tobari.RuntimeBinding, error)
}

// WorkspaceTemplateBootstrapSourceAuthority normalizes only the fixed host
// AWS/kubeconfig sources. It receives no Template name or store path and is
// called while the installation lifecycle lock protects the exact current
// Template body used to derive the next revision.
type WorkspaceTemplateBootstrapSourceAuthority interface {
	PrepareFinalTemplateAWSBootstrap(context.Context, string) (tobari.ManifestBootstrapSnapshot, error)
	PrepareFinalTemplateEKSBootstrap(context.Context, tobari.ManifestBootstrapSnapshot, string) (tobari.ManifestBootstrapSnapshot, error)
}

// PolicyMemoryActivationAuthority owns the effectful aggregate policy
// projection. The envelope records an active receipt only after this boundary
// confirms the exact Context-owned revision; failures preserve the prior
// current and last-successful active authority.
type PolicyMemoryActivationAuthority interface {
	ActivatePolicyMemory(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID) (tobari.PolicyMemoryActivationReceipt, error)
	ConfirmPolicyMemoryActive(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID, tobari.PolicyMemoryActivationReceipt) error
}

// FinalAuthoritySettlementAuthority owns the one shared Gateway/principal/OPA
// settlement beneath the installation lifecycle decision. It receives exact
// complete previous/next envelopes and one unchanged decision reference; its
// effect class and recovery are infrastructure-private.
type FinalAuthoritySettlementAuthority interface {
	SettleFinalAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.ContextID, string, string) error
	ConfirmFinalAuthoritySettled(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID) error
	SettleFinalContextDeletion(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.ContextID, string, string) error
	ConfirmFinalContextDeletionSettled(context.Context, tobari.WorkspaceAuthorityCollection, tobari.ContextID) error
	SettleFinalReviewedPolicyAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.PolicyMemoryReviewedDecisionSet, string, string) (tobari.PolicyMemoryReviewedSettlementReceipt, error)
	ConfirmFinalReviewedPolicyAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.PolicyMemoryReviewedDecisionSet) (tobari.PolicyMemoryReviewedSettlementReceipt, error)
}

// Mutator publishes complete final-authority envelopes behind the existing
// lifecycle authority. It is dormant until the atomic WP11 reader cutover.
type Mutator struct {
	lifetime          context.Context
	store             *Store
	lifecycle         LifecycleAuthority
	deletion          DeletionAuthority
	activation        PolicyMemoryActivationAuthority
	settlement        FinalAuthoritySettlementAuthority
	runtimeRevision   WorkspaceTemplateRuntimeRevisionAuthority
	bootstrapSource   WorkspaceTemplateBootstrapSourceAuthority
	researchAuth      FinalContextCredentialAuthority
	credentialAbsence ContextCredentialAbsenceAuthority
	clock             func() time.Time
	entropy           io.Reader

	rename func(string, string) error
	sync   func(string) error
}

const effectDecisionSchemaVersion = 1
const maxEffectDecisionBytes = 8 << 20

type effectDecision struct {
	SchemaVersion       int                                                 `json:"schema_version"`
	Operation           string                                              `json:"operation"`
	Target              string                                              `json:"target"`
	PreviousGeneration  uint64                                              `json:"previous_generation"`
	PreviousRevision    tobari.SemanticDigest                               `json:"previous_revision"`
	NextGeneration      uint64                                              `json:"next_generation"`
	NextRevision        tobari.SemanticDigest                               `json:"next_revision"`
	ContextID           *tobari.ContextID                                   `json:"context_id,omitempty"`
	WorkspaceID         *tobari.WorkspaceID                                 `json:"workspace_id,omitempty"`
	Workspace           *tobari.WorkspaceBinding                            `json:"workspace,omitempty"`
	Force               *bool                                               `json:"force,omitempty"`
	Candidate           *tobari.PolicyCandidateAuthority                    `json:"candidate,omitempty"`
	RuleID              string                                              `json:"rule_id,omitempty"`
	Decision            tobari.PolicyMemoryDecision                         `json:"decision,omitempty"`
	PreviousMemory      *tobari.PolicyMemoryRevision                        `json:"previous_policy_memory,omitempty"`
	EntryPlan           *tobari.WorkspaceEntryReconciliationPlan            `json:"workspace_entry_plan,omitempty"`
	ReviewedSet         *tobari.PolicyMemoryReviewedDecisionSet             `json:"reviewed_set,omitempty"`
	ReviewedPublication *reviewedTerminalPublication                        `json:"reviewed_publication,omitempty"`
	AuthDecision        *authbroker.ContextAuthDecisionAuthority            `json:"auth_decision,omitempty"`
	AuthResult          *authbroker.ContextMutationObservation              `json:"auth_result,omitempty"`
	ClusterPlan         *tobari.WorkspaceAuthorityClusterReconciliationPlan `json:"cluster_plan,omitempty"`
	ClusterDownPlan     *tobari.WorkspaceAuthorityClusterDownPlan           `json:"cluster_down_plan,omitempty"`
}

type reviewedTerminalAppliedDecision struct {
	ReviewItemID          string                      `json:"review_item_id"`
	RuleID                string                      `json:"rule_id"`
	Decision              tobari.PolicyMemoryDecision `json:"decision"`
	Match                 string                      `json:"match"`
	ContextRef            string                      `json:"context_ref"`
	TemplateRef           string                      `json:"template_ref"`
	ObservingWorkspaceRef string                      `json:"observing_workspace_ref"`
	ContextID             tobari.ContextID            `json:"context_id"`
	TemplateID            tobari.WorkspaceTemplateID  `json:"template_id"`
	ObservingWorkspaceID  tobari.WorkspaceID          `json:"observing_workspace_id"`
	ConsumedCandidates    []string                    `json:"consumed_candidates"`
	ReplacedSourceRules   []string                    `json:"replaced_source_rules"`
}

type reviewedTerminalPublication struct {
	SchemaVersion      int                                          `json:"schema_version"`
	Task               string                                       `json:"task"`
	TargetID           string                                       `json:"target_id"`
	DecisionSet        tobari.PolicyMemoryReviewedDecisionSet       `json:"decision_set"`
	Changes            []tobari.PolicyMemoryReviewedContextChange   `json:"changes"`
	AppliedDecisions   []reviewedTerminalAppliedDecision            `json:"applied_decisions"`
	Settlement         tobari.PolicyMemoryReviewedSettlementReceipt `json:"settlement"`
	PreviousGeneration uint64                                       `json:"previous_generation"`
	PreviousRevision   tobari.SemanticDigest                        `json:"previous_revision"`
	NextGeneration     uint64                                       `json:"next_generation"`
	NextRevision       tobari.SemanticDigest                        `json:"next_revision"`
	ActiveRevision     string                                       `json:"active_revision"`
	AllowCount         int                                          `json:"allow_count"`
	DenyCount          int                                          `json:"deny_count"`
	Applied            bool                                         `json:"applied"`
	Changed            bool                                         `json:"changed"`
}

func newReviewedTerminalPublication(publication tobari.PolicyMemoryReviewedSetPublication) (reviewedTerminalPublication, error) {
	compact, err := publication.CompactTerminal()
	if err != nil {
		return reviewedTerminalPublication{}, err
	}
	result := reviewedTerminalPublication{
		SchemaVersion: compact.SchemaVersion, Task: compact.Task, TargetID: compact.TargetID,
		DecisionSet: compact.DecisionSet.Clone(), Changes: cloneReviewedContextChanges(compact.Changes),
		Settlement: compact.Settlement, PreviousGeneration: compact.PreviousGeneration, PreviousRevision: compact.PreviousRevision,
		NextGeneration: compact.NextGeneration, NextRevision: compact.NextRevision, ActiveRevision: compact.ActiveRevision,
		AllowCount: compact.AllowCount, DenyCount: compact.DenyCount, Applied: compact.Applied, Changed: compact.Changed,
		AppliedDecisions: make([]reviewedTerminalAppliedDecision, len(compact.AppliedDecisions)),
	}
	for index, item := range compact.AppliedDecisions {
		result.AppliedDecisions[index] = reviewedTerminalAppliedDecision{
			ReviewItemID: item.ReviewItemID, RuleID: item.RuleID, Decision: item.Decision, Match: item.Match,
			ContextRef: item.ContextRef, TemplateRef: item.TemplateRef, ObservingWorkspaceRef: item.ObservingWorkspaceRef,
			ContextID: item.ContextID, TemplateID: item.TemplateID, ObservingWorkspaceID: item.ObservingWorkspaceID,
			ConsumedCandidates: append([]string{}, item.ConsumedCandidates...), ReplacedSourceRules: append([]string{}, item.ReplacedSourceRules...),
		}
	}
	return result, result.validate()
}

func (r reviewedTerminalPublication) publication() tobari.PolicyMemoryReviewedSetPublication {
	result := tobari.PolicyMemoryReviewedSetPublication{
		SchemaVersion: r.SchemaVersion, Task: r.Task, TargetID: r.TargetID, DecisionSet: r.DecisionSet.Clone(),
		Changes: cloneReviewedContextChanges(r.Changes), Settlement: r.Settlement,
		PreviousGeneration: r.PreviousGeneration, PreviousRevision: r.PreviousRevision,
		NextGeneration: r.NextGeneration, NextRevision: r.NextRevision, ActiveRevision: r.ActiveRevision,
		AllowCount: r.AllowCount, DenyCount: r.DenyCount, Applied: r.Applied, Changed: r.Changed,
		AppliedDecisions: make([]tobari.PolicyMemoryReviewedAppliedDecision, len(r.AppliedDecisions)),
	}
	for index, item := range r.AppliedDecisions {
		result.AppliedDecisions[index] = tobari.PolicyMemoryReviewedAppliedDecision{
			ReviewItemID: item.ReviewItemID, RuleID: item.RuleID, Decision: item.Decision, Match: item.Match,
			ContextRef: item.ContextRef, TemplateRef: item.TemplateRef, ObservingWorkspaceRef: item.ObservingWorkspaceRef,
			ContextID: item.ContextID, TemplateID: item.TemplateID, ObservingWorkspaceID: item.ObservingWorkspaceID,
			ConsumedCandidates: append([]string{}, item.ConsumedCandidates...), ReplacedSourceRules: append([]string{}, item.ReplacedSourceRules...),
		}
	}
	return result
}

func (r reviewedTerminalPublication) validate() error { return r.publication().Validate() }

func cloneReviewedContextChanges(values []tobari.PolicyMemoryReviewedContextChange) []tobari.PolicyMemoryReviewedContextChange {
	result := make([]tobari.PolicyMemoryReviewedContextChange, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
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
	if d.ClusterDownPlan != nil && d.Operation != finalClusterDownOperation {
		return fmt.Errorf("final cluster down authority appears on another effect decision")
	}
	switch d.Operation {
	case finalClusterReconciliationOperation:
		if d.ClusterDownPlan != nil || d.Target != finalClusterAuthorityTarget || d.ClusterPlan == nil || d.ClusterPlan.Validate() != nil ||
			d.ClusterPlan.PreviousGeneration != d.PreviousGeneration || d.ClusterPlan.PreviousRevision != d.PreviousRevision ||
			d.ClusterPlan.NextGeneration != d.NextGeneration || d.ClusterPlan.NextRevision != d.NextRevision ||
			d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil ||
			d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet != nil ||
			d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("final cluster reconciliation effect decision is invalid")
		}
	case finalClusterDownOperation:
		if d.Target != finalClusterAuthorityTarget || d.ClusterPlan != nil || d.ClusterDownPlan == nil || d.ClusterDownPlan.Validate() != nil ||
			d.ClusterDownPlan.PreviousGeneration != d.PreviousGeneration || d.ClusterDownPlan.PreviousRevision != d.PreviousRevision ||
			d.ClusterDownPlan.NextGeneration != d.NextGeneration || d.ClusterDownPlan.NextRevision != d.NextRevision ||
			d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil ||
			d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet != nil ||
			d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("final cluster down effect decision is invalid")
		}
	case "workspace-delete", "workspace-delete-force":
		if d.ClusterPlan != nil || d.NextGeneration != d.PreviousGeneration+1 || d.ContextID != nil || d.WorkspaceID == nil || d.WorkspaceID.Validate() != nil || d.Workspace == nil || d.Workspace.ID != *d.WorkspaceID || d.Force == nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("Workspace delete effect decision is invalid")
		}
	case "context-delete":
		contextID, err := tobari.ParseContextRef(d.Target)
		if d.ClusterPlan != nil || err != nil || d.NextGeneration != d.PreviousGeneration+1 || d.ContextID == nil || *d.ContextID != contextID || d.ContextID.Validate() != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("Context delete effect decision is invalid")
		}
	case "policy-allow", "policy-deny":
		if d.ClusterPlan != nil || d.NextGeneration != d.PreviousGeneration+1 || d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate == nil || d.Candidate.Validate() != nil || d.RuleID == "" || d.PreviousMemory == nil || d.PreviousMemory.Validate() != nil || d.Decision.Validate() != nil || d.EntryPlan != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("Policy candidate effect decision is invalid")
		}
	case "policy-reset":
		if d.ClusterPlan != nil || d.NextGeneration != d.PreviousGeneration+1 || d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID == "" || d.PreviousMemory == nil || d.PreviousMemory.Validate() != nil || d.Decision != "" || d.EntryPlan != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
			return fmt.Errorf("Policy reset effect decision is invalid")
		}
	case "context-entry":
		contextID, err := tobari.ParseContextRef(d.Target)
		if d.ClusterPlan != nil || err != nil || d.EntryPlan == nil || d.EntryPlan.Workspace.ContextID != contextID || d.EntryPlan.Applied.ContextID != contextID || d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil || d.AuthDecision != nil || d.AuthResult != nil {
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
	case "policy-apply-reviewed":
		if d.ClusterPlan != nil || d.Target != tobari.PolicyDecisionSetID || d.NextGeneration != d.PreviousGeneration+1 || d.ContextID != nil ||
			d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID != "" ||
			d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet == nil || d.ReviewedSet.Validate() != nil || d.AuthDecision != nil || d.AuthResult != nil ||
			d.ReviewedSet.ObservedGeneration != d.PreviousGeneration || d.ReviewedSet.ObservedRevision != d.PreviousRevision {
			return fmt.Errorf("reviewed Policy Memory effect decision is invalid")
		}
		if d.ReviewedPublication != nil {
			if err := d.ReviewedPublication.validate(); err != nil || !reflect.DeepEqual(d.ReviewedPublication.DecisionSet, *d.ReviewedSet) ||
				d.ReviewedPublication.PreviousGeneration != d.PreviousGeneration || d.ReviewedPublication.PreviousRevision != d.PreviousRevision ||
				d.ReviewedPublication.NextGeneration != d.NextGeneration || d.ReviewedPublication.NextRevision != d.NextRevision {
				return fmt.Errorf("reviewed Policy Memory terminal publication is invalid: %w", err)
			}
		}
	case "research-auth-login", "research-auth-import", "research-auth-logout":
		contextID, err := tobari.ParseContextRef(d.Target)
		if d.ClusterPlan != nil || err != nil || d.NextGeneration != d.PreviousGeneration || d.NextRevision != d.PreviousRevision || d.AuthDecision == nil || d.AuthDecision.Context.ContextID != contextID || d.AuthDecision.Context.ContextRef != d.Target || d.ContextID != nil || d.WorkspaceID != nil || d.Workspace != nil || d.Force != nil || d.Candidate != nil || d.RuleID != "" || d.Decision != "" || d.PreviousMemory != nil || d.EntryPlan != nil || d.ReviewedSet != nil || d.ReviewedPublication != nil {
			return fmt.Errorf("final Context authentication effect decision is invalid")
		}
		wantTask := map[string]string{"research-auth-login": authbroker.TaskLogin, "research-auth-import": authbroker.TaskImport, "research-auth-logout": authbroker.TaskLogout}[d.Operation]
		if d.AuthDecision.Task != wantTask || d.AuthDecision.Validate() != nil {
			return fmt.Errorf("final Context authentication decision authority is invalid")
		}
		if d.AuthResult != nil {
			if err := d.AuthResult.ValidateFor(wantTask, d.Target, d.AuthDecision.Provider); err != nil || !reflect.DeepEqual(d.AuthResult.Decision, *d.AuthDecision) {
				return fmt.Errorf("final Context authentication terminal result is invalid: %w", err)
			}
		}
	default:
		return fmt.Errorf("final-authority effect decision operation is invalid")
	}
	return nil
}

type effectPlan struct {
	next             tobari.WorkspaceAuthorityCollection
	decision         effectDecision
	effect           func(context.Context) error
	finalizeDecision func(effectDecision) (effectDecision, error)
}

func NewMutator(
	lifetime context.Context,
	store *Store,
	lifecycle LifecycleAuthority,
	runtimeRevision WorkspaceTemplateRuntimeRevisionAuthority,
	deletion DeletionAuthority,
	activation PolicyMemoryActivationAuthority,
	settlement FinalAuthoritySettlementAuthority,
) (*Mutator, error) {
	if lifetime == nil {
		return nil, fmt.Errorf("process lifetime context is required")
	}
	if store == nil || store.root == "" {
		return nil, fmt.Errorf("final Workspace authority store is required")
	}
	if store.legacyGuard == nil {
		return nil, fmt.Errorf("final Workspace authority mutator requires the final-only guarded Store")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("installation lifecycle authority is required")
	}
	if runtimeRevision == nil {
		return nil, fmt.Errorf("Workspace Template Runtime revision authority is required")
	}
	mutator := &Mutator{
		lifetime: lifetime, store: store, lifecycle: lifecycle, runtimeRevision: runtimeRevision,
		deletion: deletion, activation: activation, settlement: settlement,
		clock: time.Now, entropy: rand.Reader, rename: os.Rename, sync: syncMutationDirectory,
	}
	if absence, ok := deletion.(ContextCredentialAbsenceAuthority); ok {
		mutator.credentialAbsence = absence
	}
	if bootstrap, ok := runtimeRevision.(WorkspaceTemplateBootstrapSourceAuthority); ok {
		mutator.bootstrapSource = bootstrap
	}
	return mutator, nil
}

func (m *Mutator) CreateWorkspaceTemplate(ctx context.Context, name string, body tobari.WorkspaceTemplateBody) (created tobari.WorkspaceTemplate, resultErr error) {
	if err := tobari.ValidateName(name); err != nil {
		return created, err
	}
	if err := body.Validate(); err != nil {
		return created, err
	}
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
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

// InitializeFinalDefaultPair publishes the bare command's complete initial
// authority in one envelope. An initialized collection without a selected
// default is never repaired by display-name lookup; the user must select one
// exact Template reference explicitly.
func (m *Mutator) InitializeFinalDefaultPair(ctx context.Context, projectRoot string, freshBody tobari.WorkspaceTemplateBody) (publication tobari.FinalDefaultPairPublication, resultErr error) {
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return publication, err
	}
	if err := freshBody.Validate(); err != nil {
		return publication, err
	}
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		previous, err := tobari.NewFinalDefaultPairObservation(current, present, projectRoot)
		if err != nil {
			return current, false, err
		}
		var next tobari.WorkspaceAuthorityCollection
		var changed bool
		if !present {
			templateID, err := tobari.IssueWorkspaceTemplateID(m.clock().UTC(), m.entropy)
			if err != nil {
				return current, false, err
			}
			revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, freshBody)
			if err != nil {
				return current, false, err
			}
			template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
			contextID, err := tobari.IssueContextID(m.clock().UTC(), m.entropy)
			if err != nil {
				return current, false, err
			}
			memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
			if err != nil {
				return current, false, err
			}
			binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: projectRoot, TemplateID: templateID}
			record := tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory}
			next, changed, err = publishCollection(current, false, []tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, &templateID)
			if err != nil {
				return current, false, err
			}
		} else {
			if current.DefaultTemplateID == nil {
				return current, false, tobari.ErrDefaultTemplateSelectionRequired
			}
			if previous.DefaultTemplate == nil {
				return current, false, fmt.Errorf("selected default Template is unavailable")
			}
			if previous.Context != nil {
				next, changed = current.Clone(), false
			} else {
				contextID, err := tobari.IssueContextID(m.clock().UTC(), m.entropy)
				if err != nil {
					return current, false, err
				}
				memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
				if err != nil {
					return current, false, err
				}
				binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: projectRoot, TemplateID: previous.DefaultTemplate.ID}
				contexts := append(cloneContextRecords(current.Contexts), tobari.WorkspaceAuthorityContextRecord{Context: binding, PolicyMemory: memory})
				next, changed, err = publishCollection(current, true, current.Templates, contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID)
				if err != nil {
					return current, false, err
				}
			}
		}
		confirmed, err := tobari.NewFinalDefaultPairObservation(next, true, projectRoot)
		if err != nil {
			return current, false, err
		}
		publication = tobari.FinalDefaultPairPublication{Previous: previous.Clone(), Current: confirmed, Changed: changed}
		if err := publication.ValidateFor(projectRoot, freshBody); err != nil {
			return current, false, err
		}
		return next, changed, nil
	})
	return publication, resultErr
}

func (m *Mutator) CopyWorkspaceTemplateByRevisionReference(ctx context.Context, revisionRef, name string) (publication tobari.WorkspaceTemplateCopyPublication, resultErr error) {
	sourceID, sourceDigest, err := tobari.ParseWorkspaceTemplateRevisionRef(revisionRef)
	if err != nil {
		return publication, err
	}
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
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

func (m *Mutator) UpdateWorkspaceTemplateByReference(
	ctx context.Context, templateRef string, change tobari.WorkspaceTemplateChange,
) (publication tobari.WorkspaceTemplateRevisionPublication, resultErr error) {
	if err := change.Validate(); err != nil {
		return publication, err
	}
	change = change.Clone()
	publication, _, resultErr = m.updateWorkspaceTemplateByReference(ctx, templateRef, func(lockedContext context.Context, _ tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateChange, *tobari.RuntimeBinding, error) {
		var resolvedRuntime *tobari.RuntimeBinding
		if change.Kind == tobari.WorkspaceTemplateChangeRuntime {
			resolved, err := m.runtimeRevision.ResolveWorkspaceTemplateRuntimeRevision(lockedContext, change.RuntimeRevisionRef)
			if err != nil {
				return tobari.WorkspaceTemplateChange{}, nil, err
			}
			resolvedRuntime = &resolved
		}
		return change.Clone(), resolvedRuntime, nil
	})
	return publication, resultErr
}

func (m *Mutator) UpdateWorkspaceTemplateBootstrapByReference(
	ctx context.Context,
	templateRef string,
	request tobari.WorkspaceTemplateBootstrapRequest,
) (publication tobari.WorkspaceTemplateRevisionPublication, resolvedChange tobari.WorkspaceTemplateChange, resultErr error) {
	if err := request.Validate(); err != nil {
		return publication, resolvedChange, err
	}
	if m == nil || m.bootstrapSource == nil {
		return publication, resolvedChange, fmt.Errorf("final Template bootstrap source authority is unavailable")
	}
	return m.updateWorkspaceTemplateByReference(ctx, templateRef, func(lockedContext context.Context, current tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateChange, *tobari.RuntimeBinding, error) {
		change, err := m.resolveWorkspaceTemplateBootstrapChange(lockedContext, current, request)
		return change, nil, err
	})
}

type workspaceTemplateChangeResolver func(context.Context, tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateChange, *tobari.RuntimeBinding, error)

func (m *Mutator) updateWorkspaceTemplateByReference(
	ctx context.Context,
	templateRef string,
	resolve workspaceTemplateChangeResolver,
) (publication tobari.WorkspaceTemplateRevisionPublication, resolvedChange tobari.WorkspaceTemplateChange, resultErr error) {
	id, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return publication, resolvedChange, err
	}
	resultErr = m.mutate(ctx, func(lockedContext context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
		if !present {
			return current, false, tobari.ErrWorkspaceTemplateNotFound
		}
		templates := cloneTemplates(current.Templates)
		for index := range templates {
			if templates[index].ID != id {
				continue
			}
			previous := templates[index].Current.Clone()
			change, resolvedRuntime, err := resolve(lockedContext, previous.Body.Clone())
			if err != nil {
				return current, false, err
			}
			if err := change.Validate(); err != nil {
				return current, false, err
			}
			resolvedChange = change.Clone()
			nextBody, err := tobari.ApplyWorkspaceTemplateChange(previous.Body, change, resolvedRuntime)
			if err != nil {
				return current, false, err
			}
			nextRevision, changed, err := tobari.AdvanceWorkspaceTemplateRevision(previous, nextBody)
			if err != nil {
				return current, false, err
			}
			if changed {
				templates[index].Current = nextRevision.Clone()
				templates[index].Retained = append(templates[index].Retained, nextRevision.Clone())
			}
			publication = tobari.WorkspaceTemplateRevisionPublication{
				Template: templates[index].Clone(), Previous: previous, Current: nextRevision.Clone(),
				ResolvedRuntime: resolvedRuntime, Changed: changed,
			}
			if !changed {
				return current, false, nil
			}
			next, published, err := publishCollection(
				current, true, templates, current.Contexts, current.Workspaces, current.PendingCandidates, current.DefaultTemplateID,
			)
			return next, published, err
		}
		return current, false, tobari.ErrWorkspaceTemplateNotFound
	})
	if resultErr == nil {
		resultErr = publication.ValidateFor(templateRef, resolvedChange)
	}
	return publication, resolvedChange.Clone(), resultErr
}

func (m *Mutator) resolveWorkspaceTemplateBootstrapChange(
	ctx context.Context,
	current tobari.WorkspaceTemplateBody,
	request tobari.WorkspaceTemplateBootstrapRequest,
) (tobari.WorkspaceTemplateChange, error) {
	if err := request.Validate(); err != nil {
		return tobari.WorkspaceTemplateChange{}, err
	}
	bootstrap := current.CreationDefaults.Bootstrap
	switch request.Kind {
	case tobari.WorkspaceTemplateChangeBootstrapAWS:
		if request.Action == tobari.WorkspaceTemplateBootstrapRemove {
			return tobari.WorkspaceTemplateChange{Kind: request.Kind}, nil
		}
		profile := request.Selector
		if request.Action == tobari.WorkspaceTemplateBootstrapRefresh {
			if bootstrap == nil {
				return tobari.WorkspaceTemplateChange{}, tobari.ErrContextBootstrapNotConfigured
			}
			profile = bootstrap.AWS.Profile
		}
		snapshot, err := m.bootstrapSource.PrepareFinalTemplateAWSBootstrap(ctx, profile)
		if err != nil {
			return tobari.WorkspaceTemplateChange{}, err
		}
		if err := snapshot.Validate(); err != nil || snapshot.EKS != nil || snapshot.AWS.Profile != profile {
			return tobari.WorkspaceTemplateChange{}, fmt.Errorf("resolved final Template AWS bootstrap is invalid")
		}
		if bootstrap != nil && bootstrap.EKS != nil && !reflect.DeepEqual(snapshot.AWS, bootstrap.AWS) {
			return tobari.WorkspaceTemplateChange{}, tobari.ErrContextBootstrapDependency
		}
		aws := snapshot.AWS.Clone()
		return tobari.WorkspaceTemplateChange{Kind: request.Kind, AWS: &aws}, nil
	case tobari.WorkspaceTemplateChangeBootstrapEKS:
		if request.Action == tobari.WorkspaceTemplateBootstrapRemove {
			return tobari.WorkspaceTemplateChange{Kind: request.Kind}, nil
		}
		if bootstrap == nil {
			return tobari.WorkspaceTemplateChange{}, tobari.ErrContextBootstrapNotConfigured
		}
		selector := request.Selector
		if request.Action == tobari.WorkspaceTemplateBootstrapRefresh {
			if bootstrap.EKS == nil {
				return tobari.WorkspaceTemplateChange{}, tobari.ErrContextBootstrapNotConfigured
			}
			selector = bootstrap.EKS.WorkspaceManifestName
		}
		base, err := tobari.NewContextBootstrapSnapshot(bootstrap.Generation, bootstrap.AWS.Clone())
		if err != nil {
			return tobari.WorkspaceTemplateChange{}, err
		}
		snapshot, err := m.bootstrapSource.PrepareFinalTemplateEKSBootstrap(ctx, base, selector)
		if err != nil {
			return tobari.WorkspaceTemplateChange{}, err
		}
		if err := snapshot.Validate(); err != nil || snapshot.EKS == nil || snapshot.AWS.Profile != bootstrap.AWS.Profile || snapshot.EKS.WorkspaceManifestName != selector {
			return tobari.WorkspaceTemplateChange{}, fmt.Errorf("resolved final Template EKS bootstrap is invalid")
		}
		eks := *snapshot.EKS
		return tobari.WorkspaceTemplateChange{Kind: request.Kind, EKS: &eks}, nil
	default:
		return tobari.WorkspaceTemplateChange{}, fmt.Errorf("Template bootstrap request kind is invalid")
	}
}

func (m *Mutator) SetDefaultWorkspaceTemplateByReference(ctx context.Context, ref string) (result tobari.WorkspaceTemplateSelectionResult, resultErr error) {
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil {
		return result, err
	}
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
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
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
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
	resultErr = m.mutate(ctx, func(_ context.Context, current tobari.WorkspaceAuthorityCollection, present bool) (tobari.WorkspaceAuthorityCollection, bool, error) {
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
	committedDecision, resultErr := m.effectfulMutate(ctx, "context-delete", ref, nil, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
		index := contextRecordIndex(current, id)
		if index < 0 {
			return effectPlan{}, tobari.ErrContextBindingNotFound
		}
		for _, workspace := range current.Workspaces {
			if workspace.ContextID == id {
				return effectPlan{}, tobari.ErrContextBindingProtected
			}
		}
		if m.credentialAbsence == nil {
			return effectPlan{}, fmt.Errorf("Context credential deletion authority is unavailable")
		}
		if err := m.credentialAbsence.ConfirmContextCredentialAbsent(ctx, id); err != nil {
			return effectPlan{}, err
		}
		contexts := append(cloneContextRecords(current.Contexts[:index]), cloneContextRecords(current.Contexts[index+1:])...)
		candidates := make([]tobari.PolicyCandidateAuthority, 0, len(current.PendingCandidates))
		for _, candidate := range current.PendingCandidates {
			if candidate.ContextID != id {
				candidates = append(candidates, candidate.Clone())
			}
		}
		result = tobari.ContextDeleteResult{ContextID: id, Deleted: true}
		next, changed, err := publishCollection(current, true, current.Templates, contexts, current.Workspaces, candidates, current.DefaultTemplateID)
		if err != nil {
			return effectPlan{}, err
		}
		if !changed {
			return effectPlan{}, fmt.Errorf("Context deletion did not change authority")
		}
		contextID := id
		return effectPlan{
			next:     next,
			decision: effectDecision{ContextID: &contextID},
			effect: func(effectContext context.Context) error {
				if m.settlement == nil {
					return fmt.Errorf("final Gateway settlement authority is unavailable")
				}
				decisionRef := contextDeletionSettlementDecisionRef(id, next.Revision)
				return m.settlement.SettleFinalContextDeletion(effectContext, current.Clone(), next.Clone(), id, "context-delete", decisionRef)
			},
		}, nil
	})
	if resultErr == nil && !result.Deleted && committedDecision.ContextID != nil {
		result = tobari.ContextDeleteResult{ContextID: *committedDecision.ContextID, Deleted: true}
	}
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
	committedDecision, resultErr := m.effectfulMutate(ctx, operation, ref, nil, func(current tobari.WorkspaceAuthorityCollection, recovering bool) (effectPlan, error) {
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
				if err := m.deletion.PrepareWorkspaceRetirement(effectContext, workspace, force, decisionRef); err != nil {
					return err
				}
				if m.settlement == nil {
					return fmt.Errorf("final Gateway settlement authority is unavailable")
				}
				if err := m.settlement.SettleFinalAuthority(effectContext, current.Clone(), next.Clone(), workspace.ContextID, operation, decisionRef); err != nil {
					return err
				}
				if err := m.deletion.CompleteWorkspaceRetirement(effectContext, workspace, force, decisionRef); err != nil {
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
	committedDecision, resultErr := m.effectfulMutate(ctx, operation, ref, nil, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
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
		expectedReceipt, err := m.preparePolicyMemoryActivation(&contexts[recordIndex])
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
		activationEffect := m.policyMemoryActivationEffect(current, next, candidate.ContextID, operation, ref, expectedReceipt)
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
	committedDecision, resultErr := m.effectfulMutate(ctx, "policy-reset", ref, nil, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
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
		expectedReceipt, err := m.preparePolicyMemoryActivation(&contexts[recordIndex])
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
		activationEffect := m.policyMemoryActivationEffect(current, next, previous.ContextID, "policy-reset", ref, expectedReceipt)
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

func (m *Mutator) ApplyReviewedPolicyMemory(
	ctx context.Context,
	set tobari.PolicyMemoryReviewedDecisionSet,
) (publication tobari.PolicyMemoryReviewedSetPublication, resultErr error) {
	if err := set.Validate(); err != nil {
		return publication, err
	}
	requestMatches := func(decision effectDecision) bool {
		return decision.ReviewedSet != nil && decision.ReviewedSet.Digest == set.Digest && reflect.DeepEqual(*decision.ReviewedSet, set)
	}
	committed, resultErr := m.effectfulMutate(ctx, "policy-apply-reviewed", tobari.PolicyDecisionSetID, requestMatches, func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
		if current.Generation != set.ObservedGeneration || current.Revision != set.ObservedRevision {
			return effectPlan{}, tobari.ErrPolicyReviewChanged
		}
		revalidated, err := tobari.NewPolicyMemoryReviewedDecisionSet(current, set.Decisions)
		if err != nil || !reflect.DeepEqual(revalidated, set) {
			return effectPlan{}, tobari.ErrPolicyReviewChanged
		}
		contexts := cloneContextRecords(current.Contexts)
		decisionsByContext := make(map[tobari.ContextID][]tobari.PolicyMemoryReviewedDecision)
		consumed := make(map[string]struct{})
		for _, decision := range set.Decisions {
			decisionsByContext[decision.ContextID()] = append(decisionsByContext[decision.ContextID()], decision)
			for _, candidate := range decision.Candidates {
				consumed[candidate.ID] = struct{}{}
			}
		}
		for index := range contexts {
			decisions := decisionsByContext[contexts[index].Context.ID]
			if len(decisions) == 0 {
				continue
			}
			if contexts[index].ActiveTemplatePolicy == nil || contexts[index].ActivePolicyMemory == nil || contexts[index].ActivePolicyMemoryRef == nil ||
				!reflect.DeepEqual(*contexts[index].ActivePolicyMemory, contexts[index].PolicyMemory) ||
				contexts[index].ActivePolicyMemoryRef.ValidateFor(contexts[index].Context, contexts[index].PolicyMemory) != nil {
				return effectPlan{}, tobari.ErrPolicyReviewChanged
			}
			rules := clonePolicyMemoryRules(contexts[index].PolicyMemory.Rules)
			remove := make(map[string]struct{})
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
				rule, err := tobari.NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
				if err != nil {
					return effectPlan{}, err
				}
				rules = append(rules, rule)
			}
			memory, changed, err := tobari.PublishPolicyMemory(contexts[index].Context.ID, rules, &contexts[index].PolicyMemory)
			if err != nil || !changed {
				return effectPlan{}, fmt.Errorf("reviewed Policy Memory did not change exact Context authority: %w", err)
			}
			contexts[index].PolicyMemory = memory
			active := memory.Clone()
			receipt := tobari.PolicyMemoryActivationReceipt{ContextID: contexts[index].Context.ID, Revision: memory.Revision}
			contexts[index].ActivePolicyMemory = &active
			contexts[index].ActivePolicyMemoryRef = &receipt
			delete(decisionsByContext, contexts[index].Context.ID)
		}
		if len(decisionsByContext) != 0 {
			return effectPlan{}, tobari.ErrPolicyReviewChanged
		}
		candidates := make([]tobari.PolicyCandidateAuthority, 0, len(current.PendingCandidates)-len(consumed))
		for _, candidate := range current.PendingCandidates {
			if _, remove := consumed[candidate.ID]; !remove {
				candidates = append(candidates, candidate.Clone())
			}
		}
		next, changed, err := publishCollection(current, true, current.Templates, contexts, current.Workspaces, candidates, current.DefaultTemplateID)
		if err != nil || !changed {
			return effectPlan{}, fmt.Errorf("reviewed Policy Memory did not change final authority: %w", err)
		}
		if err := tobari.ValidatePolicyMemoryReviewedTransition(current, next, set); err != nil {
			return effectPlan{}, fmt.Errorf("reviewed Policy Memory transition: %w", err)
		}
		if m.settlement == nil {
			return effectPlan{}, fmt.Errorf("reviewed Policy Memory settlement authority is unavailable")
		}
		setEvidence := set.Clone()
		var settlement tobari.PolicyMemoryReviewedSettlementReceipt
		decisionRef := reviewedPolicySettlementDecisionRef(set.Digest, next.Revision)
		return effectPlan{
			next:     next,
			decision: effectDecision{ReviewedSet: &setEvidence},
			effect: func(effectContext context.Context) error {
				var effectErr error
				settlement, effectErr = m.settlement.SettleFinalReviewedPolicyAuthority(
					effectContext, current.Clone(), next.Clone(), set.Clone(), "policy-apply-reviewed", decisionRef,
				)
				return effectErr
			},
			finalizeDecision: func(decision effectDecision) (effectDecision, error) {
				full, err := tobari.NewPolicyMemoryReviewedSetPublication(current, next, set, settlement)
				if err != nil {
					return effectDecision{}, err
				}
				terminal, err := newReviewedTerminalPublication(full)
				if err != nil {
					return effectDecision{}, err
				}
				if decision.ReviewedPublication != nil && !reflect.DeepEqual(*decision.ReviewedPublication, terminal) {
					return effectDecision{}, fmt.Errorf("reviewed Policy Memory recovery result changed")
				}
				publication = full
				decision.ReviewedPublication = &terminal
				return decision, nil
			},
		}, nil
	})
	if resultErr == nil && publication.SchemaVersion == 0 {
		resultErr = m.recoverReviewedPublication(ctx, committed, &publication)
	}
	return publication, resultErr
}

func (m *Mutator) preparePolicyMemoryActivation(record *tobari.WorkspaceAuthorityContextRecord) (tobari.PolicyMemoryActivationReceipt, error) {
	if m.activation == nil {
		return tobari.PolicyMemoryActivationReceipt{}, fmt.Errorf("Policy Memory activation authority is unavailable")
	}
	expectedReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: record.Context.ID, Revision: record.PolicyMemory.Revision}
	active := record.PolicyMemory.Clone()
	record.ActivePolicyMemory = &active
	record.ActivePolicyMemoryRef = &expectedReceipt
	return expectedReceipt, nil
}

func (m *Mutator) policyMemoryActivationEffect(previous, collection tobari.WorkspaceAuthorityCollection, contextID tobari.ContextID, operation, target string, expectedReceipt tobari.PolicyMemoryActivationReceipt) func(context.Context) error {
	return func(ctx context.Context) error {
		if m.settlement == nil {
			return fmt.Errorf("final Gateway settlement authority is unavailable")
		}
		decisionRef := policySettlementDecisionRef(operation, target, collection.Revision)
		if err := m.settlement.SettleFinalAuthority(ctx, previous.Clone(), collection.Clone(), contextID, operation, decisionRef); err != nil {
			return err
		}
		snapshot, snapshotErr := snapshotForContext(collection, contextID)
		if snapshotErr != nil || snapshot.ActivePolicyMemory == nil {
			return fmt.Errorf("Policy Memory activation candidate is incomplete: %w", snapshotErr)
		}
		if err := expectedReceipt.ValidateFor(snapshot.Context, *snapshot.ActivePolicyMemory); err != nil {
			return fmt.Errorf("Policy Memory activation candidate is invalid: %w", err)
		}
		return nil
	}
}

func (m *Mutator) effectfulMutate(
	ctx context.Context,
	operation, target string,
	requestMatches func(effectDecision) bool,
	planner func(tobari.WorkspaceAuthorityCollection, bool) (effectPlan, error),
) (committed effectDecision, resultErr error) {
	if m == nil || m.store == nil || m.lifecycle == nil || m.rename == nil || m.sync == nil {
		return committed, fmt.Errorf("final Workspace authority mutator is unavailable")
	}
	// Reject pre-existing clean-break residue before the concrete lifecycle
	// authority creates its state directory and lock. The lock-held read and
	// ConfirmSelected fences below remain authoritative for concurrent drift.
	if _, _, err := m.store.ReadComplete(ctx); err != nil {
		return committed, err
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
		terminalMatches := terminal.Operation == operation && terminal.Target == target &&
			(requestMatches == nil || requestMatches(terminal))
		terminalReplayable := !isResearchAuthOperation(terminal.Operation) || terminal.Operation == "research-auth-logout"
		if !active && terminalPresent && terminalMatches && terminalReplayable && m.terminalConsequenceCurrent(current, terminal) == nil {
			if err := m.confirmCommittedEffect(lockedContext, current, terminal); err != nil {
				return err
			}
			committed = terminal
			return nil
		}
		if active {
			if decision.Operation != operation || decision.Target != target || requestMatches != nil && !requestMatches(decision) {
				return fmt.Errorf("another final-authority mutation requires exact same-target recovery")
			}
			if isResearchAuthOperation(decision.Operation) && decision.AuthResult != nil {
				if err := m.confirmCommittedEffect(lockedContext, current, decision); err != nil {
					return err
				}
				committed = decision
				return m.clearEffectDecision()
			}
			clusterNoop := decision.Operation == finalClusterReconciliationOperation && decision.ClusterPlan != nil && !decision.ClusterPlan.EnvelopeChanged ||
				decision.Operation == finalClusterDownOperation && decision.ClusterDownPlan != nil && !decision.ClusterDownPlan.EnvelopeChanged
			if !isResearchAuthOperation(decision.Operation) && !clusterNoop && current.Generation == decision.NextGeneration && current.Revision == decision.NextRevision {
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

		if err := m.store.ConfirmSelected(lockedContext, current, true); err != nil {
			return fmt.Errorf("confirm final Workspace authority selection before planning: %w", err)
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
		if active && decision.ReviewedPublication != nil {
			value, err := newReviewedTerminalPublication(decision.ReviewedPublication.publication())
			if err != nil {
				return err
			}
			complete.ReviewedPublication = &value
		}
		if err := complete.validate(); err != nil {
			return err
		}
		noEnvelopeEffect := plan.next.Generation == current.Generation && plan.next.Revision == current.Revision
		if noEnvelopeEffect && !isResearchAuthOperation(operation) && operation != finalClusterReconciliationOperation && operation != finalClusterDownOperation {
			return fmt.Errorf("only final Context authentication or exact cluster reconciliation may retain the envelope authority")
		}
		if active {
			if !reflect.DeepEqual(decision, complete) {
				return fmt.Errorf("same-target recovery does not match the durable effect decision")
			}
			if !noEnvelopeEffect {
				if err := m.validatePreparedStage(encoded); err != nil {
					return err
				}
			}
		} else {
			if err := m.reconcileStage(); err != nil {
				return err
			}
			if !noEnvelopeEffect {
				if err := m.prepareEffectStage(encoded); err != nil {
					return err
				}
			}
			if err := m.writeEffectDecision(complete); err != nil {
				return err
			}
		}
		if err := m.store.ConfirmSelected(lockedContext, current, true); err != nil {
			return fmt.Errorf("confirm final Workspace authority selection before external effect: %w", err)
		}
		if err := plan.effect(lockedContext); err != nil {
			return err
		}
		completionContext, cancelCompletion := context.WithTimeout(m.lifetime, finalMutationSelectionSettlementTimeout)
		defer cancelCompletion()
		if plan.finalizeDecision != nil {
			finalized, err := plan.finalizeDecision(complete)
			if err != nil {
				return err
			}
			if err := finalized.validate(); err != nil {
				return err
			}
			if complete.ReviewedPublication == nil || !reflect.DeepEqual(complete, finalized) {
				if err := m.replaceEffectDecision(finalized); err != nil {
					return err
				}
			}
			complete = finalized
		}
		// Once the external authority confirms the exact decision, cancellation
		// cannot turn success into replay permission. Process death remains
		// recoverable from the durable decision.
		if !noEnvelopeEffect {
			if err := m.store.ConfirmSelected(completionContext, current, true); err != nil {
				return fmt.Errorf("confirm final Workspace authority selection before publication: %w", err)
			}
			if err := m.publishPreparedEffect(current, plan.next, encoded); err != nil {
				return err
			}
		} else if err := m.store.ConfirmSelected(completionContext, current, true); err != nil {
			return fmt.Errorf("confirm final Workspace authority selection before terminal publication: %w", err)
		}
		committed = complete
		return m.clearEffectDecision()
	})
	return committed, resultErr
}

func isResearchAuthOperation(operation string) bool {
	return operation == "research-auth-login" || operation == "research-auth-import" || operation == "research-auth-logout"
}

func (m *Mutator) terminalConsequenceCurrent(current tobari.WorkspaceAuthorityCollection, decision effectDecision) error {
	switch decision.Operation {
	case finalClusterReconciliationOperation:
		if decision.ClusterPlan == nil {
			return fmt.Errorf("terminal final cluster reconciliation evidence is incomplete")
		}
		return decision.ClusterPlan.ValidateCurrent(current)
	case finalClusterDownOperation:
		if decision.ClusterDownPlan == nil {
			return fmt.Errorf("terminal final cluster down evidence is incomplete")
		}
		return decision.ClusterDownPlan.ValidateCurrent(current)
	case "research-auth-login", "research-auth-import", "research-auth-logout":
		if decision.AuthDecision == nil || decision.AuthResult == nil {
			return fmt.Errorf("terminal final Context authentication evidence is incomplete")
		}
		snapshot, err := snapshotForContext(current, decision.AuthDecision.Context.ContextID)
		if err != nil {
			return err
		}
		authority, err := authbroker.NewContextAuthenticationAuthority(snapshot, decision.Target)
		if err != nil || authority != decision.AuthDecision.Context || decision.AuthResult.Authority != authority {
			return fmt.Errorf("terminal final Context authentication authority is no longer current")
		}
		return nil
	case "context-delete":
		for _, record := range current.Contexts {
			if record.Context.ID == *decision.ContextID {
				return fmt.Errorf("terminal Context delete target is present")
			}
		}
		return nil
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
	case "policy-apply-reviewed":
		if decision.ReviewedSet == nil || decision.ReviewedPublication == nil {
			return fmt.Errorf("terminal reviewed Policy Memory evidence is incomplete")
		}
		for _, change := range decision.ReviewedPublication.Changes {
			if err := terminalPolicyMemoryCurrent(current, change.Current); err != nil {
				return err
			}
		}
		return nil
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
	case finalClusterReconciliationOperation:
		settlement, ok := m.settlement.(finalClusterSettlementAuthority)
		if !ok || decision.ClusterPlan == nil {
			return fmt.Errorf("final cluster reconciliation recovery authority is unavailable")
		}
		if err := decision.ClusterPlan.ValidateCurrent(current); err != nil {
			return err
		}
		return settlement.ConfirmFinalClusterAuthoritySettled(ctx, current.Clone())
	case finalClusterDownOperation:
		settlement, ok := m.settlement.(finalClusterDownSettlementAuthority)
		if !ok || decision.ClusterDownPlan == nil {
			return fmt.Errorf("final cluster down recovery authority is unavailable")
		}
		if err := decision.ClusterDownPlan.ValidateCurrent(current); err != nil {
			return err
		}
		return settlement.ConfirmFinalClusterDownSettled(ctx, current.Clone())
	case "research-auth-login", "research-auth-import", "research-auth-logout":
		if m.researchAuth == nil || decision.AuthDecision == nil || decision.AuthResult == nil {
			return fmt.Errorf("final Context authentication recovery authority is unavailable")
		}
		target, err := m.researchAuth.ResolveFinalContextProvider(ctx, decision.AuthDecision.Context, decision.AuthDecision.Provider)
		if err != nil {
			return err
		}
		providerAuthority, err := target.DecisionProvider()
		if err != nil || !reflect.DeepEqual(providerAuthority, decision.AuthDecision.ProviderAuthority) {
			return fmt.Errorf("final Context reviewed provider authority differs from the confirmed decision")
		}
		provider, backend, state, err := m.researchAuth.ObserveFinalContextProvider(ctx, target)
		if err != nil {
			return err
		}
		observed := authbroker.ContextMutationObservation{
			Authority: decision.AuthDecision.Context, Decision: *decision.AuthDecision, Provider: provider,
			StorageBackend: backend, BrokerState: state, Changed: decision.AuthResult.Changed, DecisionRef: decision.AuthResult.DecisionRef,
		}
		if err := observed.ValidateFor(decision.AuthDecision.Task, decision.Target, decision.AuthDecision.Provider); err != nil || !reflect.DeepEqual(observed, *decision.AuthResult) {
			return fmt.Errorf("final Context authentication consequence differs from the confirmed result: %w", err)
		}
		return nil
	case "context-delete":
		if m.settlement == nil {
			return fmt.Errorf("final Context deletion settlement recovery authority is unavailable")
		}
		return m.settlement.ConfirmFinalContextDeletionSettled(ctx, current.Clone(), *decision.ContextID)
	case "workspace-delete", "workspace-delete-force":
		contextIndex := contextRecordIndex(current, decision.Workspace.ContextID)
		if contextIndex < 0 || decision.Workspace.ValidateFor(current.Contexts[contextIndex].Context) != nil {
			return fmt.Errorf("terminal Workspace retirement evidence crosses Context authority")
		}
		decisionRef := workspaceRetirementDecisionRef(*decision.WorkspaceID, decision.NextRevision)
		if m.settlement == nil {
			return fmt.Errorf("final Gateway settlement recovery authority is unavailable")
		}
		if err := m.settlement.ConfirmFinalAuthoritySettled(ctx, current.Clone(), decision.Workspace.ContextID); err != nil {
			return err
		}
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
		return m.activation.ConfirmPolicyMemoryActive(ctx, current.Clone(), snapshot.Context.ID, *snapshot.ActivePolicyMemoryRef)
	case "policy-apply-reviewed":
		if m.settlement == nil || decision.ReviewedSet == nil || decision.ReviewedPublication == nil {
			return fmt.Errorf("reviewed Policy Memory recovery authority is unavailable")
		}
		observed, err := m.settlement.ConfirmFinalReviewedPolicyAuthority(ctx, current.Clone(), decision.ReviewedSet.Clone())
		if err != nil {
			return err
		}
		want := decision.ReviewedPublication.Settlement
		if observed.DecisionSetDigest != want.DecisionSetDigest || observed.ContentDigest != want.ContentDigest ||
			observed.AggregateRevision != want.AggregateRevision || observed.PolicyArtifact != want.PolicyArtifact ||
			observed.GatewayArtifact != want.GatewayArtifact || observed.PrincipalDigest != want.PrincipalDigest {
			return fmt.Errorf("reviewed Policy Memory live settlement differs from the confirmed result")
		}
		return nil
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

func (m *Mutator) recoverReviewedPublication(
	_ context.Context,
	decision effectDecision,
	target *tobari.PolicyMemoryReviewedSetPublication,
) error {
	if decision.Operation != "policy-apply-reviewed" || decision.Target != tobari.PolicyDecisionSetID ||
		decision.ReviewedSet == nil || decision.ReviewedPublication == nil {
		return fmt.Errorf("committed reviewed Policy Memory recovery evidence is incomplete")
	}
	*target = decision.ReviewedPublication.publication()
	return target.Validate()
}

func workspaceRetirementDecisionRef(id tobari.WorkspaceID, revision tobari.SemanticDigest) string {
	return "workspace-retirement:" + string(id) + ":" + string(revision)
}

func policySettlementDecisionRef(operation, target string, revision tobari.SemanticDigest) string {
	return "policy-settlement:" + operation + ":" + target + ":" + string(revision)
}

func reviewedPolicySettlementDecisionRef(set, revision tobari.SemanticDigest) string {
	return "policy-reviewed-settlement:" + string(set) + ":" + string(revision)
}

func contextDeletionSettlementDecisionRef(id tobari.ContextID, revision tobari.SemanticDigest) string {
	return "context-deletion-settlement:" + string(id) + ":" + string(revision)
}

func clusterReconciliationDecisionRef(revision tobari.SemanticDigest) string {
	return "cluster-reconciliation:" + string(revision)
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
	data, err := readEffectDecisionFile(path)
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
	data, err := readEffectDecisionFile(path)
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
	buffer := boundedJSONBuffer{maximum: maxEffectDecisionBytes}
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

func (m *Mutator) replaceEffectDecision(decision effectDecision) error {
	if err := decision.validate(); err != nil {
		return err
	}
	buffer := boundedJSONBuffer{maximum: maxEffectDecisionBytes}
	if err := writeJSONValue(&buffer, reflect.ValueOf(decision)); err != nil {
		return fmt.Errorf("encode confirmed final-authority effect decision: %w", err)
	}
	temporary := m.effectDecisionTempPath()
	if err := writeMutationFile(temporary, buffer.Bytes()); err != nil {
		return err
	}
	parent := filepath.Dir(temporary)
	if err := m.sync(parent); err != nil {
		return err
	}
	effectErr := m.rename(temporary, m.effectDecisionPath())
	if effectErr == nil {
		effectErr = m.sync(parent)
	}
	observed, present, readErr := m.readEffectDecision()
	if readErr == nil && present && reflect.DeepEqual(observed, decision) {
		return nil
	}
	if effectErr == nil {
		effectErr = fmt.Errorf("exact confirmed decision read-back failed")
	}
	return fmt.Errorf("publish confirmed final-authority effect decision: %w", errors.Join(effectErr, readErr))
}

func readEffectDecisionFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxEffectDecisionBytes {
		return nil, fmt.Errorf("final-authority effect decision must contain 1..%d safe bytes", maxEffectDecisionBytes)
	}
	return readAuthorityFile(path)
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

type collectionMutation func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (tobari.WorkspaceAuthorityCollection, bool, error)

func (m *Mutator) mutate(ctx context.Context, change collectionMutation) error {
	if m == nil || m.store == nil || m.lifecycle == nil || m.clock == nil || m.entropy == nil || m.rename == nil || m.sync == nil {
		return fmt.Errorf("final Workspace authority mutator is unavailable")
	}
	// Keep routine clean-break rejection zero-mutation. The lifecycle authority
	// may create its lock, so selection is observed once before entering it and
	// then re-observed under the lock below.
	if _, _, err := m.store.ReadComplete(ctx); err != nil {
		return err
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
		if err := m.store.ConfirmSelected(lockedContext, current, present); err != nil {
			return fmt.Errorf("confirm final Workspace authority selection before planning: %w", err)
		}
		next, changed, err := change(lockedContext, current.Clone(), present)
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
		if err := m.store.ConfirmSelected(lockedContext, current, present); err != nil {
			return fmt.Errorf("confirm final Workspace authority selection before publication: %w", err)
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
	for index, value := range values {
		if value.LastSuccessfulEntry != nil {
			entry := *value.LastSuccessfulEntry
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

func clonePolicyMemoryRules(values []tobari.PolicyMemoryRule) []tobari.PolicyMemoryRule {
	result := make([]tobari.PolicyMemoryRule, len(values))
	for index := range values {
		result[index] = values[index].Clone()
	}
	return result
}
