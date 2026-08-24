package tobari

import (
	"reflect"
	"testing"
)

func TestPlanWorkspaceAuthorityClusterReconciliationPersistsEveryCurrentAxisReceipt(t *testing.T) {
	base := workspaceAuthorityCollectionFixture(t)
	contexts := cloneWorkspaceAuthorityContextRecords(base.Contexts)
	contexts[0].ActiveTemplatePolicy = nil
	contexts[0].ActivePolicyMemory = nil
	contexts[0].ActivePolicyMemoryRef = nil
	previous, changed, err := PublishWorkspaceAuthorityCollection(
		base.Templates, contexts, base.Workspaces, base.PendingCandidates, base.DefaultTemplateID, &base,
	)
	if err != nil || !changed {
		t.Fatalf("publish fresh Context: changed=%t err=%v", changed, err)
	}

	transition, err := PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		t.Fatal(err)
	}
	if !transition.Plan.EnvelopeChanged || transition.Next.Generation != previous.Generation+1 ||
		transition.Next.Revision == previous.Revision {
		t.Fatalf("cluster reconciliation did not publish a new envelope: %#v", transition.Plan)
	}
	if err := transition.Plan.ValidateTransition(previous, transition.Next); err != nil {
		t.Fatalf("validate transition: %v", err)
	}
	if err := transition.Plan.ValidateCurrent(transition.Next); err != nil {
		t.Fatalf("validate current: %v", err)
	}
	record := transition.Next.Contexts[0]
	if record.ActiveTemplatePolicy == nil || record.ActivePolicyMemory == nil || record.ActivePolicyMemoryRef == nil {
		t.Fatalf("cluster reconciliation omitted an authority axis receipt: %#v", record)
	}
	if record.ActiveTemplatePolicy.PolicySliceDigest != transition.Next.Templates[0].Current.Slices.PolicySliceDigest ||
		!reflect.DeepEqual(*record.ActivePolicyMemory, record.PolicyMemory) ||
		record.ActivePolicyMemoryRef.Revision != record.PolicyMemory.Revision {
		t.Fatalf("cluster reconciliation did not select current authority: %#v", record)
	}
	projection := transition.Plan.Projection
	if projection.Mode != WorkspacePolicyProjectionCluster || projection.CollectionRevision != transition.Next.Revision ||
		len(projection.Contexts) != 1 || projection.Contexts[0].TemplateReceipt != *record.ActiveTemplatePolicy ||
		projection.Contexts[0].MemoryReceipt != *record.ActivePolicyMemoryRef {
		t.Fatalf("cluster projection does not prove independent axis receipts: %#v", projection)
	}
}

func TestPlanWorkspaceAuthorityClusterReconciliationIsExactAndIdempotent(t *testing.T) {
	previous := workspaceAuthorityCollectionFixture(t)
	transition, err := PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Plan.EnvelopeChanged || !reflect.DeepEqual(transition.Next, previous) {
		t.Fatalf("already-current authority was republished: %#v", transition.Plan)
	}

	tampered := transition.Plan
	tampered.Projection.Contexts[0].MemoryReceipt.Revision = authorityDigest("8")
	if err := tampered.ValidateTransition(previous, transition.Next); err == nil {
		t.Fatal("tampered independent receipt passed transition validation")
	}

	wrongNext := transition.Next.Clone()
	wrongNext.Generation++
	if err := transition.Plan.ValidateTransition(previous, wrongNext); err == nil {
		t.Fatal("wrong final envelope passed exact transition validation")
	}

	missingReceipt := transition.Next.Clone()
	missingReceipt.Contexts[0].ActiveTemplatePolicy = nil
	if err := transition.Plan.ValidateCurrent(missingReceipt); err == nil {
		t.Fatal("missing independent receipt passed terminal validation")
	}
}
