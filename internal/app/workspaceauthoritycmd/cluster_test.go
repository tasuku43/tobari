package workspaceauthoritycmd

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeFinalClusterPort struct {
	plan     tobari.WorkspaceAuthorityClusterReconciliationPlan
	identity tobari.PolicyProjectionIdentity
	err      error
	calls    int
}

type readyFinalClusterFixture struct {
	err   error
	calls int
}

func (f *readyFinalClusterFixture) Check(context.Context) error {
	f.calls++
	return f.err
}

type fakeFinalClusterDownPort struct {
	plan  tobari.WorkspaceAuthorityClusterDownPlan
	purge bool
	err   error
	calls int
}

func (f *fakeFinalClusterDownPort) Down(_ context.Context, purge bool) (tobari.WorkspaceAuthorityClusterDownPlan, error) {
	f.calls++
	f.purge = purge
	return f.plan, f.err
}

func (f *fakeFinalClusterPort) Reconcile(ctx context.Context, readiness func(context.Context) error) (tobari.WorkspaceAuthorityClusterReconciliationPlan, tobari.PolicyProjectionIdentity, error) {
	f.calls++
	if err := readiness(ctx); err != nil {
		return tobari.WorkspaceAuthorityClusterReconciliationPlan{}, tobari.PolicyProjectionIdentity{}, err
	}
	return f.plan, f.identity, f.err
}

func finalClusterPlanFixture(t *testing.T) tobari.WorkspaceAuthorityClusterReconciliationPlan {
	t.Helper()
	snapshot := snapshotFixture(t, false, false)
	previous, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{snapshot.Template},
		[]tobari.WorkspaceAuthorityContextRecord{{Context: snapshot.Context, PolicyMemory: snapshot.PolicyMemory}},
		[]tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish inactive final authority: changed=%t err=%v", changed, err)
	}
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		t.Fatal(err)
	}
	return transition.Plan
}

func finalClusterIntent() operation.Intent {
	return intent(
		TaskClusterUp, operation.EffectCreate,
		operation.TargetRef{Kind: tobari.ClusterTargetKind, ParentID: tobari.ClusterTargetID},
		FinalClusterUpImpact(),
	)
}

func finalClusterIdentityFixture() tobari.PolicyProjectionIdentity {
	return tobari.PolicyProjectionIdentity{
		AggregateRevision:  strings.TrimPrefix(string(digest("7")), "sha256:"),
		EvaluatorIdentity:  tobari.PolicyEvaluatorIdentity{SchemaVersion: 1, Version: "test-evaluator", Digest: digest("6")},
		PolicyDataIdentity: tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: digest("5")},
	}
}

func finalClusterDownPlanFixture(t *testing.T, purge bool) tobari.WorkspaceAuthorityClusterDownPlan {
	t.Helper()
	snapshot := snapshotFixture(t, false, false)
	previous, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{snapshot.Template},
		[]tobari.WorkspaceAuthorityContextRecord{{Context: snapshot.Context, PolicyMemory: snapshot.PolicyMemory}},
		[]tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil || !changed {
		t.Fatalf("publish final down authority: changed=%t err=%v", changed, err)
	}
	transition, err := tobari.PlanWorkspaceAuthorityClusterDownWithPurge(previous, purge)
	if err != nil {
		t.Fatal(err)
	}
	return transition.Plan
}

func TestFinalClusterDownServiceForwardsAndConfirmsExactPurgeIntent(t *testing.T) {
	port := &fakeFinalClusterDownPort{plan: finalClusterDownPlanFixture(t, true)}
	intent := operation.Intent{
		Command: TaskClusterDown, Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: FinalClusterDownImpact(),
	}
	result, err := NewFinalClusterLifecycleService(port).Down(context.Background(), intent, true)
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || !port.purge || !result.Purged || !result.Plan.Purge || result.Validate() != nil {
		t.Fatalf("down result=%#v calls=%d purge=%t", result, port.calls, port.purge)
	}
}

func TestFinalClusterLifecycleRejectsAbsentAuthorityBeforeMutation(t *testing.T) {
	up := &fakeFinalClusterPort{err: tobari.ErrFinalAuthorityNotFound}
	if _, err := NewFinalClusterService(up, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent()); err == nil {
		t.Fatal("cluster up accepted absent final authority")
	} else if public, ok := fault.PublicCopy(err); !ok || public.Code != "authority_not_found" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("cluster up absent-authority fault = %+v, structured=%t", public, ok)
	}

	down := &fakeFinalClusterDownPort{err: tobari.ErrFinalAuthorityNotFound}
	intent := operation.Intent{
		Command: TaskClusterDown, Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID},
		Impact: FinalClusterDownImpact(),
	}
	if _, err := NewFinalClusterLifecycleService(down).Down(context.Background(), intent, false); err == nil {
		t.Fatal("cluster down accepted absent final authority")
	} else if public, ok := fault.PublicCopy(err); !ok || public.Code != "authority_not_found" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("cluster down absent-authority fault = %+v, structured=%t", public, ok)
	}
}

func TestFinalClusterServiceBindsFixedTargetAndReturnsExactReceipts(t *testing.T) {
	port := &fakeFinalClusterPort{plan: finalClusterPlanFixture(t), identity: finalClusterIdentityFixture()}
	result, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
	if err != nil {
		t.Fatal(err)
	}
	if port.calls != 1 || result.Task != tobari.TaskClusterUp || result.Generation != port.plan.NextGeneration ||
		result.CollectionRevision != port.plan.NextRevision || result.ContentDigest != port.plan.Projection.ContentDigest ||
		result.PlanDigest != port.plan.Projection.PlanDigest || result.PolicyProjectionIdentity != port.identity || !result.Applied || len(result.Contexts) != 1 {
		t.Fatalf("final cluster result=%#v calls=%d", result, port.calls)
	}
	want := port.plan.Projection.Contexts[0]
	got := result.Contexts[0]
	if got.ContextID != want.ContextID || got.WorkspaceTemplateID != want.TemplateID ||
		got.TemplatePolicy != want.TemplateReceipt || got.PolicyMemory != want.MemoryReceipt {
		t.Fatalf("independent receipts=%#v want projection=%#v", got, want)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("validated final cluster result: %v", err)
	}

	// The task result owns a clone rather than retaining adapter-owned slice
	// aliases that could relabel a confirmed publication after return.
	port.plan.Projection.Contexts[0].MemoryReceipt.Revision = digest("9")
	if err := result.Validate(); err != nil {
		t.Fatalf("adapter alias changed confirmed result: %v", err)
	}
}

func TestFinalClusterServiceRejectsEveryMutationDimensionBeforeAdapter(t *testing.T) {
	valid := finalClusterIntent()
	tests := map[string]func(*operation.Intent){
		"command": func(value *operation.Intent) { value.Command = "cluster down" },
		"effect": func(value *operation.Intent) {
			value.Effect = operation.EffectWrite
			value.Target = operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID}
		},
		"kind":   func(value *operation.Intent) { value.Target.Kind = tobari.PolicyDecisionSetKind },
		"scope":  func(value *operation.Intent) { value.Target.ParentID = "another-cluster" },
		"impact": func(value *operation.Intent) { value.Impact.AccessChange = operation.DeclarationYes },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			port := &fakeFinalClusterPort{plan: finalClusterPlanFixture(t), identity: finalClusterIdentityFixture()}
			request := valid
			mutate(&request)
			if _, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), request); err == nil || port.calls != 0 {
				t.Fatalf("invalid %s reached adapter: calls=%d err=%v", name, port.calls, err)
			}
		})
	}
}

func TestFinalClusterServiceRejectsInvalidAdapterResultAsUnknownConfirmedBoundary(t *testing.T) {
	plan := finalClusterPlanFixture(t)
	plan.Projection.Contexts[0].MemoryReceipt.Revision = digest("8")
	port := &fakeFinalClusterPort{plan: plan, identity: finalClusterIdentityFixture()}
	_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_cluster_reconciliation_result" ||
		public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeUnknown || port.calls != 1 {
		t.Fatalf("invalid result fault=%#v ok=%t calls=%d", public, ok, port.calls)
	}
}

func TestFinalClusterServicePreservesLegacyAndUnknownMutationClassification(t *testing.T) {
	for name, sentinel := range map[string]error{
		"pre-release envelope": tobari.ErrPreReleaseLegacyAuthority,
		"executable policy":    fmt.Errorf("%w: %w", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy),
	} {
		t.Run(name, func(t *testing.T) {
			port := &fakeFinalClusterPort{err: sentinel}
			_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhasePrecondition ||
				public.ChangeState != fault.ChangeNone || port.calls != 1 {
				t.Fatalf("legacy fault=%#v ok=%t calls=%d", public, ok, port.calls)
			}
		})
	}
	t.Run("unclassified adapter failure", func(t *testing.T) {
		port := &fakeFinalClusterPort{err: errors.New("unknown settlement result")}
		_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "unclassified_mutation_outcome" || public.Phase != fault.PhaseMutation ||
			public.ChangeState != fault.ChangeUnknown || port.calls != 1 {
			t.Fatalf("unknown fault=%#v ok=%t calls=%d", public, ok, port.calls)
		}
	})
	t.Run("fresh preflight cancellation", func(t *testing.T) {
		port := &fakeFinalClusterPort{err: context.Canceled}
		_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "operation_canceled" || public.Kind != fault.KindCanceled || public.Phase != fault.PhasePrecondition ||
			public.ChangeState != fault.ChangeNone || !public.Retryable || port.calls != 1 {
			t.Fatalf("preflight cancellation=%#v ok=%t calls=%d", public, ok, port.calls)
		}
	})
	t.Run("post-decision cancellation preserves recovery classification", func(t *testing.T) {
		structured := fault.WithClassification(fault.Wrap(
			fault.KindUnavailable, "final_authority_mutation_interrupted", "durable decision retained", false, context.Canceled,
			fault.NextAction{Command: "status", Reason: "Recover the retained decision."},
		), fault.PhaseMutation, fault.ChangePartial)
		port := &fakeFinalClusterPort{err: structured}
		_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "final_authority_mutation_interrupted" || public.Kind != fault.KindUnavailable ||
			public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial || port.calls != 1 {
			t.Fatalf("post-decision cancellation=%#v ok=%t calls=%d", public, ok, port.calls)
		}
	})
	t.Run("active decision recovery sentinel precedes cancellation", func(t *testing.T) {
		port := &fakeFinalClusterPort{err: errors.Join(tobari.ErrFinalAuthorityMutationRecoveryRequired, context.Canceled)}
		_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "final_authority_mutation_recovery_required" || public.Kind != fault.KindUnavailable ||
			public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || port.calls != 1 {
			t.Fatalf("active recovery cancellation=%#v ok=%t calls=%d", public, ok, port.calls)
		}
	})
	t.Run("structured adapter failure", func(t *testing.T) {
		structured := fault.WithClassification(fault.New(
			fault.KindRejected,
			"cluster_resource_conflict",
			"Fresh shared-cluster resources are present or could not be proved absent.",
			false,
			fault.NextAction{Command: "doctor", Reason: "Inspect exact Docker and Tobari ownership state before another cluster activation."},
		), fault.PhasePrecondition, fault.ChangeNone)
		port := &fakeFinalClusterPort{err: structured}
		_, err := NewFinalClusterService(port, &readyFinalClusterFixture{}).Reconcile(context.Background(), finalClusterIntent())
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "cluster_resource_conflict" || public.Phase != fault.PhasePrecondition ||
			public.ChangeState != fault.ChangeNone || port.calls != 1 {
			t.Fatalf("structured fault=%#v ok=%t calls=%d", public, ok, port.calls)
		}
	})
}

func TestFinalClusterReconciliationValidationRejectsRelabeledConsequence(t *testing.T) {
	result, err := NewFinalClusterReconciliation(finalClusterPlanFixture(t), finalClusterIdentityFixture())
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(*FinalClusterReconciliation){
		"schema":   func(value *FinalClusterReconciliation) { value.SchemaVersion = 1 },
		"task":     func(value *FinalClusterReconciliation) { value.Task = tobari.TaskClusterStatus },
		"revision": func(value *FinalClusterReconciliation) { value.CollectionRevision = digest("8") },
		"content":  func(value *FinalClusterReconciliation) { value.ContentDigest = digest("8") },
		"plan":     func(value *FinalClusterReconciliation) { value.PlanDigest = digest("8") },
		"receipt":  func(value *FinalClusterReconciliation) { value.Contexts[0].PolicyMemory.Revision = digest("8") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			copy := result
			copy.Contexts = append([]FinalClusterContextActivation{}, result.Contexts...)
			copy.Plan = result.Plan
			copy.Plan.Projection = result.Plan.Projection.Clone()
			mutate(&copy)
			if err := copy.Validate(); err == nil {
				t.Fatalf("relabeled %s consequence validated: %#v", name, copy)
			}
		})
	}

	copy := result
	copy.Contexts = []FinalClusterContextActivation{}
	if reflect.DeepEqual(copy.Contexts, result.Contexts) || copy.Validate() == nil {
		t.Fatal("missing Context receipt set validated")
	}
}
