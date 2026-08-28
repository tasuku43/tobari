package workspaceauthoritystore

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterDownSettlementFixture struct {
	*finalSettlementFixture
	calls       int
	confirms    int
	purge       bool
	operation   string
	decisionRef string
	err         error
	settled     tobari.WorkspaceAuthorityCollection
}

func (s *finalClusterDownSettlementFixture) SettleFinalClusterDown(
	_ context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	operation, decisionRef string,
	purge bool,
) error {
	transition, err := tobari.PlanWorkspaceAuthorityClusterDownWithPurge(previous, purge)
	if err != nil || transition.Plan.ValidateTransition(previous, next) != nil {
		return errors.New("invalid final cluster down transition")
	}
	s.calls++
	s.purge = purge
	s.operation = operation
	s.decisionRef = decisionRef
	s.settled = next.Clone()
	return s.err
}

func (s *finalClusterDownSettlementFixture) ConfirmFinalClusterDownSettled(_ context.Context, current tobari.WorkspaceAuthorityCollection) error {
	s.confirms++
	if !reflect.DeepEqual(current, s.settled) {
		return errors.New("final cluster down settlement changed")
	}
	return nil
}

type finalClusterDownObserverFixture struct{}

func (finalClusterDownObserverFixture) ObserveFinalCluster(context.Context, tobari.WorkspaceAuthorityCollection, bool) (tobari.FinalClusterStatus, error) {
	return tobari.FinalClusterStatus{}, errors.New("unexpected final cluster observation")
}

func finalClusterDownCollectionFixture(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	base := storeCollectionFixture(t)
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, base.Contexts, []tobari.WorkspaceBinding{},
		[]tobari.PolicyCandidateAuthority{}, base.DefaultTemplateID, &base,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func newFinalClusterDownAdapterFixture(t *testing.T, collection tobari.WorkspaceAuthorityCollection) (*Mutator, *ClusterLifecycleAdapter, *finalClusterDownSettlementFixture) {
	t.Helper()
	store, mutator, _, _, _ := newMutationFixture(t, &collection)
	settlement := &finalClusterDownSettlementFixture{finalSettlementFixture: mutator.settlement.(*finalSettlementFixture)}
	mutator.settlement = settlement
	adapter, err := NewClusterLifecycleAdapter(store, mutator, finalClusterDownObserverFixture{})
	if err != nil {
		t.Fatal(err)
	}
	return mutator, adapter, settlement
}

func TestFinalClusterDownAdapterPersistsExactPurgeMode(t *testing.T) {
	previous := finalClusterDownCollectionFixture(t)
	_, adapter, settlement := newFinalClusterDownAdapterFixture(t, previous)
	plan, err := adapter.Down(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Purge || settlement.calls != 1 || !settlement.purge || settlement.operation != finalClusterDownOperation ||
		!strings.Contains(settlement.decisionRef, ":purge:") {
		t.Fatalf("plan=%#v settlement=%#v", plan, settlement)
	}
}

func TestFinalClusterDownAdapterRejectsDifferentModeDuringRecovery(t *testing.T) {
	previous := finalClusterDownCollectionFixture(t)
	mutator, adapter, settlement := newFinalClusterDownAdapterFixture(t, previous)
	settlement.err = errors.New("synthetic interrupted purge")
	if _, err := adapter.Down(context.Background(), true); err == nil {
		t.Fatal("interrupted purge was reported complete")
	}
	decision, active, err := mutator.readEffectDecision()
	if err != nil || !active || decision.ClusterDownPlan == nil || !decision.ClusterDownPlan.Purge {
		t.Fatalf("purge decision=%#v active=%t err=%v", decision, active, err)
	}
	if _, err := adapter.Down(context.Background(), false); err == nil {
		t.Fatal("normal down adopted an interrupted purge decision")
	}
	if settlement.calls != 1 {
		t.Fatalf("different mode reached settlement: calls=%d", settlement.calls)
	}
}

func TestFinalClusterDownAdapterAllowsRetainedDownAfterCompletedPurge(t *testing.T) {
	previous := finalClusterDownCollectionFixture(t)
	_, adapter, settlement := newFinalClusterDownAdapterFixture(t, previous)
	if _, err := adapter.Down(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.Down(context.Background(), false)
	if err != nil {
		t.Fatalf("retained down after completed purge: %v", err)
	}
	if plan.Purge || settlement.calls != 2 || settlement.purge {
		t.Fatalf("plan=%#v settlement=%#v", plan, settlement)
	}
}
