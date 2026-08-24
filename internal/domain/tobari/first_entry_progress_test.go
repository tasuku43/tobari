package tobari

import "testing"

func TestFirstEntryProgressVocabularyIsClosedAndOrdered(t *testing.T) {
	want := []FirstEntryStage{
		FirstEntryCheckRequirements,
		FirstEntryResolveContext,
		FirstEntryPrepareProtection,
		FirstEntryPrepareWorkspace,
		FirstEntryEnterWorkspace,
	}
	stages := FirstEntryStages()
	if len(stages) != len(want) {
		t.Fatalf("stage count = %d, want %d", len(stages), len(want))
	}
	for index, stage := range want {
		if stages[index] != stage || FirstEntryStageIndex(stage) != index {
			t.Fatalf("stage %d = %q, index=%d", index, stages[index], FirstEntryStageIndex(stage))
		}
		for _, state := range []FirstEntryStageState{
			FirstEntryStagePending, FirstEntryStageRunning, FirstEntryStageSucceeded,
			FirstEntryStageSkipped, FirstEntryStageBlocked, FirstEntryStageFailed,
			FirstEntryStageUnknown,
		} {
			if err := (FirstEntryProgress{Stage: stage, State: state}).Validate(); err != nil {
				t.Fatalf("valid progress %q/%q: %v", stage, state, err)
			}
		}
	}
	for _, invalid := range []FirstEntryProgress{
		{Stage: "resolve_manifest", State: FirstEntryStageRunning},
		{Stage: FirstEntryResolveContext, State: "reconcile_required"},
	} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid progress validated: %+v", invalid)
		}
	}
	stages[0] = "mutated"
	if FirstEntryStages()[0] != FirstEntryCheckRequirements {
		t.Fatal("caller mutated the canonical first-entry stage order")
	}
}
