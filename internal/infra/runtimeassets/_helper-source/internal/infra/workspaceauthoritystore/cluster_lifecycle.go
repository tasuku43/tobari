package workspaceauthoritystore

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const finalClusterDownOperation = "cluster-down"

type finalClusterRuntimeObserver interface {
	ObserveFinalCluster(context.Context, tobari.WorkspaceAuthorityCollection, bool) (tobari.FinalClusterStatus, error)
}

type finalClusterDownSettlementAuthority interface {
	SettleFinalClusterDown(context.Context, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, string, string, bool) error
	ConfirmFinalClusterDownSettled(context.Context, tobari.WorkspaceAuthorityCollection) error
}

// ClusterLifecycleAdapter keeps complete final-envelope selection in the Store;
// the runtime receives typed authority, never a store path.
type ClusterLifecycleAdapter struct {
	store      *Store
	mutator    *Mutator
	observer   finalClusterRuntimeObserver
	settlement finalClusterDownSettlementAuthority
}

func NewClusterLifecycleAdapter(store *Store, mutator *Mutator, runtime any) (*ClusterLifecycleAdapter, error) {
	observer, ok := runtime.(finalClusterRuntimeObserver)
	if !ok || store == nil {
		return nil, fmt.Errorf("final cluster status authority is unavailable")
	}
	adapter := &ClusterLifecycleAdapter{store: store, mutator: mutator, observer: observer}
	if mutator != nil {
		settlement, ok := mutator.settlement.(finalClusterDownSettlementAuthority)
		if !ok {
			return nil, fmt.Errorf("final cluster down settlement authority is unavailable")
		}
		adapter.settlement = settlement
	}
	return adapter, nil
}

func (a *ClusterLifecycleAdapter) Observe(ctx context.Context) (tobari.FinalClusterStatus, error) {
	if a == nil || a.store == nil || a.observer == nil {
		return tobari.FinalClusterStatus{}, fmt.Errorf("final cluster status adapter is unavailable")
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalClusterStatus{}, err
	}
	status, err := a.observer.ObserveFinalCluster(ctx, collection, present)
	if err != nil {
		return tobari.FinalClusterStatus{}, err
	}
	if err := a.store.ConfirmSelected(ctx, collection, present); err != nil {
		return tobari.FinalClusterStatus{}, fmt.Errorf("confirm final cluster status authority: %w", err)
	}
	return status, nil
}

func (a *ClusterLifecycleAdapter) Down(ctx context.Context, purge bool) (tobari.WorkspaceAuthorityClusterDownPlan, error) {
	if a == nil || a.mutator == nil || a.settlement == nil {
		return tobari.WorkspaceAuthorityClusterDownPlan{}, fmt.Errorf("final cluster down adapter is unavailable")
	}
	committed, err := a.mutator.effectfulMutate(ctx, finalClusterDownOperation, finalClusterAuthorityTarget,
		func(d effectDecision) bool { return d.ClusterDownPlan != nil && d.ClusterDownPlan.Purge == purge },
		func(current tobari.WorkspaceAuthorityCollection, _ bool) (effectPlan, error) {
			transition, err := tobari.PlanWorkspaceAuthorityClusterDownWithPurge(current, purge)
			if err != nil {
				return effectPlan{}, err
			}
			plan := transition.Plan
			return effectPlan{next: transition.Next, decision: effectDecision{ClusterDownPlan: &plan}, effect: func(effectContext context.Context) error {
				if err := plan.ValidateTransition(current, transition.Next); err != nil {
					return err
				}
				return a.settlement.SettleFinalClusterDown(effectContext, current.Clone(), transition.Next.Clone(), finalClusterDownOperation, clusterDownDecisionRef(transition.Next.Revision, purge), purge)
			}}, nil
		})
	if err != nil {
		return tobari.WorkspaceAuthorityClusterDownPlan{}, err
	}
	if committed.ClusterDownPlan == nil {
		return tobari.WorkspaceAuthorityClusterDownPlan{}, fmt.Errorf("committed final cluster down plan is unavailable")
	}
	return *committed.ClusterDownPlan, nil
}

func clusterDownDecisionRef(revision tobari.SemanticDigest, purge bool) string {
	mode := "retain"
	if purge {
		mode = "purge"
	}
	return "cluster-down:" + mode + ":" + string(revision)
}
