package tobari

import "testing"

func TestRuntimeBuildProgressValidatesTaskMetadata(t *testing.T) {
	valid := RuntimeBuildProgress{
		Stage: RuntimeBuildStageBuild, Status: RuntimeBuildProgressStarted,
		WorkspaceManifestName: "default", Dockerfile: "/config/contexts/default/runtime/Dockerfile",
		PreviousImage: testRuntimeImage, CandidateImage: "tobari-context-default:0123456789ab",
		Selection: RuntimeBuildSelectionUnchanged,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid progress error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*RuntimeBuildProgress)
	}{
		{name: "stage", mutate: func(value *RuntimeBuildProgress) { value.Stage = "unknown" }},
		{name: "status", mutate: func(value *RuntimeBuildProgress) { value.Status = "unknown" }},
		{name: "selection", mutate: func(value *RuntimeBuildProgress) { value.Selection = "unknown" }},
		{name: "context", mutate: func(value *RuntimeBuildProgress) { value.WorkspaceManifestName = "../outside" }},
		{name: "Dockerfile", mutate: func(value *RuntimeBuildProgress) { value.Dockerfile = "relative/Dockerfile" }},
		{name: "previous image", mutate: func(value *RuntimeBuildProgress) { value.PreviousImage = "--pull" }},
		{name: "unresolved previous image", mutate: func(value *RuntimeBuildProgress) { value.PreviousImage = BuiltinImageSelector }},
		{name: "candidate image", mutate: func(value *RuntimeBuildProgress) { value.CandidateImage = BuiltinImageSelector }},
		{name: "premature promotion", mutate: func(value *RuntimeBuildProgress) { value.Selection = RuntimeBuildSelectionPromoted }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := valid
			test.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatalf("progress unexpectedly valid: %+v", value)
			}
		})
	}
	promoted := valid
	promoted.Stage = RuntimeBuildStagePromote
	promoted.Status = RuntimeBuildProgressCompleted
	promoted.Selection = RuntimeBuildSelectionPromoted
	if err := promoted.Validate(); err != nil {
		t.Fatalf("completed promotion error = %v", err)
	}
}
