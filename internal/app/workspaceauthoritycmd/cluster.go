package workspaceauthoritycmd

import (
	"context"
	"fmt"
	"reflect"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const FinalClusterReconciliationSchemaVersion = 2

// FinalClusterContextActivation is one exact pair of independently active
// Template-policy and Policy-Memory receipts. It does not collapse those axes
// into the collection revision or the global projection digest.
type FinalClusterContextActivation struct {
	ContextID           tobari.ContextID                       `json:"context_id"`
	WorkspaceTemplateID tobari.WorkspaceTemplateID             `json:"workspace_template_id"`
	TemplatePolicy      tobari.TemplatePolicyActivationReceipt `json:"template_policy"`
	PolicyMemory        tobari.PolicyMemoryActivationReceipt   `json:"policy_memory"`
}

// FinalClusterReconciliation is the task-owned confirmed result of one final
// cluster reconcile. The private plan remains available for contract
// validation without exposing durable recovery authority as presentation.
type FinalClusterReconciliation struct {
	SchemaVersion      int                                                `json:"schema_version"`
	Task               string                                             `json:"task"`
	Generation         uint64                                             `json:"generation"`
	CollectionRevision tobari.SemanticDigest                              `json:"collection_revision"`
	ContentDigest      tobari.SemanticDigest                              `json:"content_digest"`
	PlanDigest         tobari.SemanticDigest                              `json:"plan_digest"`
	EnvelopeChanged    bool                                               `json:"envelope_changed"`
	Applied            bool                                               `json:"applied"`
	Contexts           []FinalClusterContextActivation                    `json:"contexts"`
	Plan               tobari.WorkspaceAuthorityClusterReconciliationPlan `json:"-"`
}

func NewFinalClusterReconciliation(plan tobari.WorkspaceAuthorityClusterReconciliationPlan) (FinalClusterReconciliation, error) {
	plan.Projection = plan.Projection.Clone()
	if err := plan.Validate(); err != nil {
		return FinalClusterReconciliation{}, err
	}
	contexts := make([]FinalClusterContextActivation, len(plan.Projection.Contexts))
	for index, item := range plan.Projection.Contexts {
		contexts[index] = FinalClusterContextActivation{
			ContextID: item.ContextID, WorkspaceTemplateID: item.TemplateID,
			TemplatePolicy: item.TemplateReceipt, PolicyMemory: item.MemoryReceipt,
		}
	}
	result := FinalClusterReconciliation{
		SchemaVersion: FinalClusterReconciliationSchemaVersion,
		Task:          tobari.TaskClusterUp,
		Generation:    plan.NextGeneration, CollectionRevision: plan.NextRevision,
		ContentDigest: plan.Projection.ContentDigest, PlanDigest: plan.Projection.PlanDigest,
		EnvelopeChanged: plan.EnvelopeChanged, Applied: true, Contexts: contexts, Plan: plan,
	}
	return result, result.Validate()
}

func (r FinalClusterReconciliation) Validate() error {
	if r.SchemaVersion != FinalClusterReconciliationSchemaVersion || r.Task != tobari.TaskClusterUp || !r.Applied {
		return fmt.Errorf("final cluster reconciliation result metadata is invalid")
	}
	if err := r.Plan.Validate(); err != nil {
		return err
	}
	if r.Generation != r.Plan.NextGeneration || r.CollectionRevision != r.Plan.NextRevision ||
		r.ContentDigest != r.Plan.Projection.ContentDigest || r.PlanDigest != r.Plan.Projection.PlanDigest ||
		r.EnvelopeChanged != r.Plan.EnvelopeChanged {
		return fmt.Errorf("final cluster reconciliation result does not bind its exact consequence")
	}
	want := make([]FinalClusterContextActivation, len(r.Plan.Projection.Contexts))
	for index, item := range r.Plan.Projection.Contexts {
		want[index] = FinalClusterContextActivation{
			ContextID: item.ContextID, WorkspaceTemplateID: item.TemplateID,
			TemplatePolicy: item.TemplateReceipt, PolicyMemory: item.MemoryReceipt,
		}
	}
	if r.Contexts == nil || !reflect.DeepEqual(r.Contexts, want) {
		return fmt.Errorf("final cluster reconciliation result omits an active Context receipt")
	}
	return nil
}

type FinalClusterReconciliationPort interface {
	Reconcile(context.Context) (tobari.WorkspaceAuthorityClusterReconciliationPlan, error)
}

type finalClusterMutationPolicy struct{}

func (finalClusterMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ClusterTargetKind &&
		intent.Target.ParentID == tobari.ClusterTargetID && intent.Target.ID == "" {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "final cluster reconciliation target is not owned by Tobari", false)
}

type FinalClusterService struct {
	reconcile FinalClusterReconciliationPort
	mutator   *execution.Invoker
}

func NewFinalClusterService(port any) *FinalClusterService {
	service := &FinalClusterService{mutator: execution.New(finalClusterMutationPolicy{})}
	service.reconcile, _ = port.(FinalClusterReconciliationPort)
	return service
}

func FinalClusterUpImpact() operation.Impact {
	return operation.Impact{
		Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
		AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
	}
}

func (s *FinalClusterService) Reconcile(ctx context.Context, intent operation.Intent) (FinalClusterReconciliation, error) {
	if s == nil || portcheck.IsNil(s.reconcile) {
		return FinalClusterReconciliation{}, missingPort("final cluster reconciliation")
	}
	target := operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID}
	request := execution.Request{
		Intent: intent, ExpectedCommand: TaskClusterUp, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: target, ExpectedImpact: FinalClusterUpImpact(),
	}
	var result FinalClusterReconciliation
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		plan, err := s.reconcile.Reconcile(actionContext)
		if err != nil {
			if classified, ok := preReleaseLegacyMutationFault(err); ok {
				return classified
			}
			return err
		}
		confirmed, err := NewFinalClusterReconciliation(plan)
		if err != nil {
			return contractFault("invalid_cluster_reconciliation_result", "final cluster reconciliation result is invalid", err)
		}
		result = confirmed
		return nil
	})
	return result, err
}
