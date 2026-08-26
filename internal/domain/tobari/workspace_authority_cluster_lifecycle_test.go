package tobari

import (
	"reflect"
	"strings"
	"testing"
)

func zeroWorkspaceClusterFixture(t *testing.T) WorkspaceAuthorityCollection {
	t.Helper()
	base := workspaceAuthorityCollectionFixture(t)
	value, _, err := PublishWorkspaceAuthorityCollection(base.Templates, base.Contexts, []WorkspaceBinding{}, []PolicyCandidateAuthority{}, base.DefaultTemplateID, &base)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPlanWorkspaceAuthorityClusterDownClearsOnlyActiveReceipts(t *testing.T) {
	previous := zeroWorkspaceClusterFixture(t)
	transition, err := PlanWorkspaceAuthorityClusterDown(previous)
	if err != nil {
		t.Fatal(err)
	}
	if err := transition.Plan.ValidateTransition(previous, transition.Next); err != nil {
		t.Fatal(err)
	}
	if len(transition.Next.Workspaces) != 0 || !reflect.DeepEqual(transition.Next.Templates, previous.Templates) ||
		!reflect.DeepEqual(transition.Next.Contexts[0].Context, previous.Contexts[0].Context) ||
		!reflect.DeepEqual(transition.Next.Contexts[0].PolicyMemory, previous.Contexts[0].PolicyMemory) ||
		transition.Next.Contexts[0].ActiveTemplatePolicy != nil || transition.Next.Contexts[0].ActivePolicyMemory != nil || transition.Next.Contexts[0].ActivePolicyMemoryRef != nil {
		t.Fatalf("cluster down consequence changed retained authority: %#v", transition.Next)
	}
	if err := transition.Plan.ValidateCurrent(transition.Next); err != nil {
		t.Fatal(err)
	}
	replay, err := PlanWorkspaceAuthorityClusterDown(transition.Next)
	if err != nil || replay.Plan.EnvelopeChanged || !reflect.DeepEqual(replay.Next, transition.Next) {
		t.Fatalf("down replay=%#v err=%v", replay, err)
	}
}

func TestPlanWorkspaceAuthorityClusterDownRejectsRemainingWorkspace(t *testing.T) {
	if _, err := PlanWorkspaceAuthorityClusterDown(workspaceAuthorityCollectionFixture(t)); err == nil {
		t.Fatal("cluster down accepted a remaining Workspace")
	}
}

func TestFinalClusterStatusKeepsUnknownDistinctFromAbsence(t *testing.T) {
	status := FinalClusterStatus{SchemaVersion: FinalClusterStatusSchemaVersion, Task: TaskClusterStatus, Authority: FinalClusterAuthorityAbsent, Runtime: FinalClusterRuntimeUnknown, Receipt: FinalClusterReceiptAbsent, Contexts: []FinalClusterContextReceiptObservation{}, Components: []FinalClusterComponentObservation{{Name: "gateway", State: FinalClusterRuntimeUnknown, Identity: FinalClusterEvidenceUnknown, Topology: FinalClusterEvidenceUnknown}, {Name: "opa", State: FinalClusterRuntimeAbsent, Identity: FinalClusterEvidenceAbsent, Topology: FinalClusterEvidenceAbsent}}}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	status.Runtime = ""
	if err := status.Validate(); err == nil {
		t.Fatal("unknown runtime was collapsed to an empty sentinel")
	}
}

func TestFinalClusterStoppedResearchClosureValidates(t *testing.T) {
	status := FinalClusterStatus{
		SchemaVersion:      FinalClusterStatusSchemaVersion,
		Task:               TaskClusterStatus,
		Authority:          FinalClusterAuthorityPresent,
		Generation:         1,
		CollectionRevision: SemanticDigest("sha256:" + strings.Repeat("a", 64)),
		Runtime:            FinalClusterRuntimeStopped,
		Receipt:            FinalClusterReceiptStopped,
		Contexts:           []FinalClusterContextReceiptObservation{},
		Components: []FinalClusterComponentObservation{
			{Name: "gateway", State: FinalClusterRuntimeAbsent, Identity: FinalClusterEvidenceAbsent, Topology: FinalClusterEvidenceAbsent},
			{Name: "opa", State: FinalClusterRuntimeAbsent, Identity: FinalClusterEvidenceAbsent, Topology: FinalClusterEvidenceAbsent},
			{Name: "auth-broker", State: FinalClusterRuntimeAbsent, Identity: FinalClusterEvidenceAbsent, Topology: FinalClusterEvidenceAbsent},
			{Name: "credential-companion", State: FinalClusterRuntimeStopped, Health: "stopped", Identity: FinalClusterEvidenceExact, Topology: FinalClusterEvidenceExact},
		},
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	fresh := status
	fresh.Runtime, fresh.Receipt = FinalClusterRuntimeAbsent, FinalClusterReceiptAbsent
	fresh.Components[3] = FinalClusterComponentObservation{Name: "credential-companion", State: FinalClusterRuntimeAbsent, Identity: FinalClusterEvidenceAbsent, Topology: FinalClusterEvidenceAbsent}
	if err := fresh.Validate(); err != nil {
		t.Fatal(err)
	}
}
