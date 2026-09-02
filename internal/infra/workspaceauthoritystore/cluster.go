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
	PreflightFinalClusterAuthority(context.Context, tobari.WorkspacePolicyProjection) error
	ReconcileFinalClusterAuthorityWithIdentity(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, string, string) (tobari.PolicyProjectionIdentity, error)
	ConfirmFinalClusterAuthoritySettled(context.Context, tobari.WorkspaceAuthorityCollection, tobari.PolicyProjectionIdentity) error
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

func (a *ClusterAdapter) Reconcile(ctx context.Context, readiness func(context.Context) error) (tobari.WorkspaceAuthorityClusterReconciliationPlan, tobari.PolicyProjectionIdentity, error) {
	if a == nil || a.mutator == nil || readiness == nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, fmt.Errorf("final cluster reconciliation adapter is unavailable")
	}
	var settledIdentity tobari.PolicyProjectionIdentity
	committed, err := a.mutator.effectfulMutate(
		ctx,
		finalClusterReconciliationOperation,
		finalClusterAuthorityTarget,
		func(decision effectDecision) bool { return decision.ClusterPlan != nil },
		func(current tobari.WorkspaceAuthorityCollection, active bool) (effectPlan, error) {
			transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(current)
			if err != nil {
				return effectPlan{}, err
			}
			plan := transition.Plan
			if !active {
				// Keep the application-selected Docker profile inside the
				// fresh-action fence. Running it before this adapter could relabel
				// an already-durable interrupted decision as a no-change failure.
				if err := readiness(ctx); err != nil {
					return effectPlan{}, err
				}
				if err := a.settlement.PreflightFinalClusterAuthority(ctx, plan.Projection); err != nil {
					return effectPlan{}, err
				}
			}
			return effectPlan{
				next:     transition.Next,
				decision: effectDecision{ClusterPlan: &plan},
				effect: func(effectContext context.Context) error {
					if err := plan.ValidateTransition(current, transition.Next); err != nil {
						return err
					}
					var err error
					settledIdentity, err = a.settlement.ReconcileFinalClusterAuthorityWithIdentity(
						effectContext, current.Clone(), transition.Next.Clone(),
						finalClusterReconciliationOperation, clusterReconciliationDecisionRef(transition.Next.Revision),
					)
					return err
				},
				finalizeDecision: func(decision effectDecision) (effectDecision, error) {
					if err := settledIdentity.Validate(); err != nil {
						return effectDecision{}, fmt.Errorf("final cluster settlement returned invalid aggregate identity: %w", err)
					}
					identity := settledIdentity
					decision.ClusterProjectionIdentity = &identity
					return decision, nil
				},
			}, nil
		},
	)
	if err != nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, err
	}
	if committed.ClusterPlan == nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, fmt.Errorf("committed final cluster reconciliation plan is unavailable")
	}
	if committed.ClusterProjectionIdentity == nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, fmt.Errorf("committed final cluster reconciliation identity is unavailable")
	}
	if err := committed.ClusterProjectionIdentity.Validate(); err != nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, fmt.Errorf("committed final cluster reconciliation identity is invalid: %w", err)
	}
	return *committed.ClusterPlan, *committed.ClusterProjectionIdentity, nil
}
