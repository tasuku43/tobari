package workspaceauthoritystore

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	finalClusterReconciliationOperation = "cluster-reconcile"
	finalClusterAuthorityTarget         = "final-cluster-authority"
)

// ClusterAdapter owns the durable parent mutation for final-only cluster
// reconciliation. It deliberately reuses Mutator's lifecycle, decision,
// staging, publication, and terminal-replay machinery.
type ClusterAdapter struct {
	mutator    *Mutator
	settlement finalClusterSettlementAuthority
}

type finalClusterSettlementAuthority interface {
	ReconcileFinalClusterAuthority(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, string, string) error
	ConfirmFinalClusterAuthoritySettled(context.Context, tobari.WorkspaceAuthorityCollection) error
}

func NewClusterAdapter(mutator *Mutator) (*ClusterAdapter, error) {
	if mutator == nil || mutator.store == nil || mutator.lifecycle == nil || mutator.settlement == nil {
		return nil, fmt.Errorf("final cluster reconciliation adapter is unavailable")
	}
	settlement, ok := mutator.settlement.(finalClusterSettlementAuthority)
	if !ok {
		return nil, fmt.Errorf("final cluster reconciliation settlement authority is unavailable")
	}
	return &ClusterAdapter{mutator: mutator, settlement: settlement}, nil
}

func (a *ClusterAdapter) Reconcile(ctx context.Context) (tobari.WorkspaceAuthorityClusterReconciliationPlan, error) {
	if a == nil || a.mutator == nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, fmt.Errorf("final cluster reconciliation adapter is unavailable")
	}
	committed, err := a.mutator.effectfulMutate(
		ctx,
		finalClusterReconciliationOperation,
		finalClusterAuthorityTarget,
		func(decision effectDecision) bool { return decision.ClusterPlan != nil },
		func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
			transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(current)
			if err != nil {
				return effectPlan{}, err
			}
			plan := transition.Plan
			return effectPlan{
				next:     transition.Next,
				decision: effectDecision{ClusterPlan: &plan},
				effect: func(effectContext context.Context) error {
					if err := plan.ValidateTransition(current, transition.Next); err != nil {
						return err
					}
					return a.settlement.ReconcileFinalClusterAuthority(
						effectContext, current.Clone(), transition.Next.Clone(),
						finalClusterReconciliationOperation, clusterReconciliationDecisionRef(transition.Next.Revision),
					)
				},
			}, nil
		},
	)
	if err != nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, err
	}
	if committed.ClusterPlan == nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, fmt.Errorf("committed final cluster reconciliation plan is unavailable")
	}
	return *committed.ClusterPlan, nil
}
