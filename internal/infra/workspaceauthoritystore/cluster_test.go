package workspaceauthoritystore

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type clusterSettlementFixture struct {
	*finalSettlementFixture
	calls        int
	preflights   int
	preflightErr error
	confirms     int
	onSettle     func()
	previous     tobari.WorkspaceAuthorityCollection
	settled      tobari.WorkspaceAuthorityCollection
	operation    string
	decisionRef  string
}

func (s *clusterSettlementFixture) PreflightFinalClusterAuthority(_ context.Context, plan tobari.WorkspacePolicyProjection) error {
	if err := plan.Validate(); err != nil {
		return err
	}
	s.preflights++
	return s.preflightErr
}

func (s *clusterSettlementFixture) ReconcileFinalClusterAuthority(
	_ context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	operation, decisionRef string,
) error {
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		return err
	}
	if err := transition.Plan.ValidateTransition(previous, next); err != nil {
		return err
	}
	s.calls++
	s.previous = previous.Clone()
	s.settled = next.Clone()
	s.operation = operation
	s.decisionRef = decisionRef
	if s.onSettle != nil {
		s.onSettle()
	}
	return nil
}

func (s *clusterSettlementFixture) ReconcileFinalClusterAuthorityWithIdentity(
	ctx context.Context,
	previous, next tobari.WorkspaceAuthorityCollection,
	operation, decisionRef string,
) (tobari.PolicyProjectionIdentity, error) {
	if err := s.ReconcileFinalClusterAuthority(ctx, previous, next, operation, decisionRef); err != nil {
		return tobari.PolicyProjectionIdentity{}, err
	}
	return tobari.PolicyProjectionIdentity{
		AggregateRevision:  strings.TrimPrefix(string(next.Revision), "sha256:"),
		EvaluatorIdentity:  tobari.PolicyEvaluatorIdentity{SchemaVersion: 1, Version: "test-evaluator", Digest: testPolicyDigest("a")},
		PolicyDataIdentity: tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: testPolicyDigest("b")},
	}, nil
}

func testPolicyDigest(value string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64))
}

func (s *clusterSettlementFixture) ConfirmFinalClusterAuthoritySettled(
	_ context.Context,
	current tobari.WorkspaceAuthorityCollection,
	expected tobari.PolicyProjectionIdentity,
) error {
	s.confirms++
	if !reflect.DeepEqual(current, s.settled) {
		return fmt.Errorf("live final cluster authority differs from settlement")
	}
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(current)
	if err != nil {
		return err
	}
	if err := transition.Plan.ValidateCurrent(current); err != nil {
		return err
	}
	actual := tobari.PolicyProjectionIdentity{
		AggregateRevision:  strings.TrimPrefix(string(current.Revision), "sha256:"),
		EvaluatorIdentity:  expected.EvaluatorIdentity,
		PolicyDataIdentity: expected.PolicyDataIdentity,
	}
	if actual != expected {
		return fmt.Errorf("live final cluster identity differs from settlement")
	}
	return nil
}

func inactiveClusterCollection(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	base := storeCollectionFixture(t)
	contexts := make([]tobari.WorkspaceAuthorityContextRecord, len(base.Contexts))
	for index := range base.Contexts {
		contexts[index] = base.Contexts[index].Clone()
		contexts[index].ActiveTemplatePolicy = nil
		contexts[index].ActivePolicyMemory = nil
		contexts[index].ActivePolicyMemoryRef = nil
	}
	previous, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish inactive final collection: changed=%t err=%v", changed, err)
	}
	return previous
}

func newClusterAdapterFixture(t *testing.T, existing tobari.WorkspaceAuthorityCollection) (*Store, *Mutator, *ClusterAdapter, *clusterSettlementFixture) {
	t.Helper()
	store, mutator, _, _, _ := newMutationFixture(t, &existing)
	base := mutator.settlement.(*finalSettlementFixture)
	settlement := &clusterSettlementFixture{finalSettlementFixture: base}
	mutator.settlement = settlement
	adapter, err := NewClusterAdapter(mutator)
	if err != nil {
		t.Fatal(err)
	}
	return store, mutator, adapter, settlement
}

func TestClusterAdapterPersistsCurrentAxisReceiptsAndReplaysTerminalConfirmation(t *testing.T) {
	previous := inactiveClusterCollection(t)
	store, _, adapter, settlement := newClusterAdapterFixture(t, previous)

	plan, identity, err := adapter.Reconcile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read reconciled final authority: present=%t err=%v", present, err)
	}
	if err := plan.ValidateTransition(previous, current); err != nil {
		t.Fatalf("validate persisted cluster transition: %v", err)
	}
	if len(current.Contexts) == 0 || current.Contexts[0].ActiveTemplatePolicy == nil ||
		current.Contexts[0].ActivePolicyMemory == nil || current.Contexts[0].ActivePolicyMemoryRef == nil {
		t.Fatalf("reconciled envelope omitted independent receipts: %#v", current.Contexts)
	}
	if settlement.calls != 1 || settlement.operation != finalClusterReconciliationOperation ||
		settlement.decisionRef != clusterReconciliationDecisionRef(current.Revision) {
		t.Fatalf("cluster settlement evidence calls=%d operation=%q ref=%q", settlement.calls, settlement.operation, settlement.decisionRef)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("settled cluster identity: %v", err)
	}
	if settlement.preflights != 1 {
		t.Fatalf("fresh cluster preflights=%d, want 1", settlement.preflights)
	}

	replayed, replayedIdentity, err := adapter.Reconcile(context.Background())
	if err != nil || !reflect.DeepEqual(replayed, plan) || replayedIdentity != identity || settlement.calls != 1 || settlement.confirms != 1 {
		t.Fatalf("terminal replay plan=%#v identity=%#v calls=%d confirms=%d err=%v", replayed, replayedIdentity, settlement.calls, settlement.confirms, err)
	}
}

func TestClusterAdapterBindPreflightFailurePrecedesDurableDecision(t *testing.T) {
	previous := inactiveClusterCollection(t)
	store, mutator, adapter, settlement := newClusterAdapterFixture(t, previous)
	settlement.preflightErr = fault.WithClassification(fault.New(
		fault.KindRejected, "cluster_resource_conflict", "Docker bind source is unavailable", false,
		fault.NextAction{Command: "doctor", Reason: "Share the exact host path."},
	), fault.PhasePrecondition, fault.ChangeNone)

	if _, _, err := adapter.Reconcile(context.Background()); err == nil {
		t.Fatal("bind preflight failure was reported as complete")
	} else if public, ok := fault.PublicCopy(err); !ok || public.Code != "cluster_resource_conflict" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("bind preflight fault = %#v, public=%t", public, ok)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || !reflect.DeepEqual(current, previous) {
		t.Fatalf("preflight changed final authority: present=%t current=%#v err=%v", present, current, err)
	}
	if settlement.preflights != 1 || settlement.calls != 0 {
		t.Fatalf("preflight calls=%d settlement calls=%d", settlement.preflights, settlement.calls)
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || active {
		t.Fatalf("preflight published durable decision: active=%t err=%v", active, err)
	}
	if _, err := os.Lstat(mutationStagePath(store.root)); !os.IsNotExist(err) {
		t.Fatalf("preflight published durable stage: %v", err)
	}
}

func TestClusterAdapterRecoversPostEffectPreEnvelopeAndBlocksUnrelatedMutation(t *testing.T) {
	previous := inactiveClusterCollection(t)
	store, mutator, adapter, settlement := newClusterAdapterFixture(t, previous)
	realRename := mutator.rename
	interrupted := false
	mutator.rename = func(source, target string) error {
		if source == mutationStagePath(store.root) && target == store.root+"/"+authorityFileName && !interrupted {
			interrupted = true
			return fmt.Errorf("injected post-effect publication interruption")
		}
		return realRename(source, target)
	}

	if _, _, err := adapter.Reconcile(context.Background()); err == nil {
		t.Fatal("post-effect/pre-envelope interruption was reported as complete")
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || !reflect.DeepEqual(current, previous) || settlement.calls != 1 {
		t.Fatalf("interrupted authority current=%#v present=%t calls=%d err=%v", current, present, settlement.calls, err)
	}
	if _, active, err := mutator.readEffectDecision(); err != nil || !active {
		t.Fatalf("durable cluster decision active=%t err=%v", active, err)
	}
	if _, err := os.Lstat(mutationStagePath(store.root)); err != nil {
		t.Fatalf("durable cluster stage is unavailable: %v", err)
	}
	if _, err := mutator.seedWorkspaceTemplateForLegacyMigration(context.Background(), "blocked", current.Templates[0].Current.Body); err == nil {
		t.Fatal("unrelated final-authority mutation entered while cluster decision was active")
	}

	mutator.rename = realRename
	settlement.preflightErr = fmt.Errorf("active durable decision must not repeat fresh preflight")
	plan, _, err := adapter.Reconcile(context.Background())
	if err != nil || settlement.calls != 2 || settlement.preflights != 1 {
		t.Fatalf("cluster recovery plan=%#v calls=%d preflights=%d err=%v", plan, settlement.calls, settlement.preflights, err)
	}
	current, present, err = store.ReadComplete(context.Background())
	if err != nil || !present || plan.ValidateTransition(previous, current) != nil {
		t.Fatalf("recovered final authority present=%t current=%#v err=%v", present, current, err)
	}
	if _, err := os.Lstat(mutationStagePath(store.root)); !os.IsNotExist(err) {
		t.Fatalf("cluster stage remained after recovery: %v", err)
	}
}

func TestClusterAdapterPublishesConfirmedEffectAfterCallerCancellation(t *testing.T) {
	previous := inactiveClusterCollection(t)
	store, _, adapter, settlement := newClusterAdapterFixture(t, previous)
	ctx, cancel := context.WithCancel(context.Background())
	settlement.onSettle = cancel

	plan, _, err := adapter.Reconcile(ctx)
	if err != nil {
		t.Fatalf("confirmed cluster effect became replay permission: %v", err)
	}
	current, present, err := store.ReadComplete(context.Background())
	if err != nil || !present {
		t.Fatalf("read post-cancellation final authority: present=%t err=%v", present, err)
	}
	if err := plan.ValidateTransition(previous, current); err != nil {
		t.Fatalf("post-cancellation final publication is invalid: %v", err)
	}
	if settlement.calls != 1 {
		t.Fatalf("confirmed cluster effect was repeated: calls=%d", settlement.calls)
	}
}

func TestNewClusterAdapterRequiresClusterSettlementAuthority(t *testing.T) {
	current := storeCollectionFixture(t)
	_, mutator, _, _, _ := newMutationFixture(t, &current)
	if _, err := NewClusterAdapter(mutator); err == nil {
		t.Fatal("adapter accepted settlement without exact cluster recovery authority")
	}
}
