package tobari

import "testing"

func TestStatusHomeSurfacesMutationRecoveryBeforeOrdinaryEntryGuidance(t *testing.T) {
	contextRef, err := ContextRef(ContextID("01912345-6789-7abc-8def-0123456789ad"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		observation FinalAuthorityMutationObservation
		wantPath    string
	}{
		{name: "pending context entry", observation: FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "context-entry", Target: contextRef}, wantPath: "tobari"},
		{name: "pending context delete", observation: FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "context-delete", Target: contextRef}, wantPath: "help context delete"},
		{name: "pending workspace delete", observation: FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "workspace-delete", Target: "wsp1_01912345-6789-7abc-8def-0123456789ae"}, wantPath: "help workspace delete"},
		{name: "pending policy Apply", observation: FinalAuthorityMutationObservation{ActiveDecision: true, Operation: "policy-apply-reviewed", Target: PolicyDecisionSetID}, wantPath: "help review permissions"},
		{name: "stage without decision", observation: FinalAuthorityMutationObservation{StagePresent: true}, wantPath: "doctor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := NewStatusHomeSnapshot(
				WorkspaceAuthorityCollection{}, false, "/workspace/example",
				StatusHomeLiveEvidence{MutationRecovery: &test.observation},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(snapshot.Attention) != 1 || snapshot.Attention[0].Kind != "mutation_recovery" || snapshot.Attention[0].Path != test.wantPath || snapshot.Next.Path == nil || *snapshot.Next.Path != test.wantPath {
				t.Fatalf("snapshot attention=%+v next=%+v", snapshot.Attention, snapshot.Next)
			}
			if snapshot.Next.Reason == "" {
				t.Fatal("mutation recovery guidance has no reason")
			}
		})
	}

}
