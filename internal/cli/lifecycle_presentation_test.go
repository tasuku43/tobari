package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type lifecyclePresentationAnswer struct {
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	Target        struct {
		Root        string `json:"root"`
		WorkspaceID string `json:"workspace_id"`
		ContextID   string `json:"context_id"`
		ContextName string `json:"context_name"`
		Attachment  string `json:"attachment"`
	} `json:"target"`
	SameRootOtherContext struct {
		Root        string `json:"root"`
		WorkspaceID string `json:"workspace_id"`
		ContextID   string `json:"context_id"`
		ContextName string `json:"context_name"`
	} `json:"same_root_other_context"`
	ExactNextArgv []string `json:"exact_next_argv"`
	ForceDelete   struct {
		Removes       []string `json:"removes"`
		Preserves     []string `json:"preserves"`
		OverridesOnly string   `json:"overrides_only"`
	} `json:"force_delete"`
	RoutineSuccess struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
	NegativeCases         []string `json:"negative_cases"`
	UnsupportedInferences []string `json:"unsupported_inferences"`
}

func TestLifecyclePresentationEvidenceKeepsOneContextBoundTarget(t *testing.T) {
	t.Parallel()
	fixturePath := filepath.Join("testdata", "lifecycle_status_report.json")
	answerPath := filepath.Join("testdata", "lifecycle_status_answer.json")
	goldenPath := filepath.Join("testdata", "lifecycle_status_report.txt")
	for _, path := range []string{fixturePath, answerPath, goldenPath} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("presentation evidence %q is not a regular repository file: info=%v err=%v", path, info, err)
		}
	}

	fixtureData, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var fixture tobari.ProjectStatus
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("decode typed fixture: %v", err)
	}
	if err := fixture.Validate(); err != nil {
		t.Fatalf("typed fixture is invalid: %v", err)
	}

	answerData, err := os.ReadFile(answerPath)
	if err != nil {
		t.Fatal(err)
	}
	var answer lifecyclePresentationAnswer
	if err := json.Unmarshal(answerData, &answer); err != nil {
		t.Fatalf("decode answer key: %v", err)
	}
	if answer.SchemaVersion != 1 || answer.Task != fixture.Task ||
		answer.Target.Root != fixture.Root || answer.Target.WorkspaceID != fixture.ID ||
		answer.Target.ContextID != fixture.ContextID || answer.Target.ContextName != fixture.ContextName ||
		answer.Target.Attachment != string(fixture.Attachment) {
		t.Fatalf("answer target does not match fixture: answer=%+v fixture=%+v", answer.Target, fixture)
	}
	if answer.SameRootOtherContext.Root != fixture.Root ||
		answer.SameRootOtherContext.ContextID == fixture.ContextID ||
		answer.SameRootOtherContext.WorkspaceID == fixture.ID ||
		answer.SameRootOtherContext.ContextName == fixture.ContextName {
		t.Fatalf("same-root Context identities merged: %+v", answer.SameRootOtherContext)
	}
	if !slices.Equal(answer.ExactNextArgv, []string{"tobari", "--context", "toolbox"}) ||
		answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 {
		t.Fatalf("routine-success evidence is incomplete: %+v", answer)
	}
	if !slices.Equal(answer.ForceDelete.Removes, []string{"owned_runtime", "persistent_home", "tool_owned_authentication"}) ||
		!slices.Equal(answer.ForceDelete.Preserves, []string{"project_root", "project_files"}) ||
		answer.ForceDelete.OverridesOnly != "attached_session_guard" {
		t.Fatalf("force-delete impact is incomplete: %+v", answer.ForceDelete)
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	textOutput, err := renderProjectStatus(fixture, successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(textOutput, golden) {
		t.Fatalf("lifecycle status text changed\n--- got ---\n%s--- want ---\n%s", textOutput, golden)
	}
	jsonOutput, err := renderProjectStatus(fixture, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document projectStatusDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 4 || !reflect.DeepEqual(document.Status.NextArgv, answer.ExactNextArgv) ||
		document.Status.ContextID == nil || *document.Status.ContextID != answer.Target.ContextID || document.Status.Attachment != answer.Target.Attachment {
		t.Fatalf("structured status lost target or recovery: %+v", document)
	}

	deleteSpec, found := DefaultCatalog().Lookup("delete")
	if !found {
		t.Fatal("delete is absent from the catalog")
	}
	contract := strings.ToLower(deleteSpec.Agent.Outcome + " " + strings.Join(deleteSpec.Agent.Prerequisites, " "))
	for _, fact := range []string{"persistent home", "tool-owned authentication", "project root", "attached session"} {
		if !strings.Contains(contract, fact) {
			t.Fatalf("delete contract lacks %q: %s", fact, contract)
		}
	}
	for _, unsupported := range answer.UnsupportedInferences {
		if strings.Contains(strings.ToLower(string(golden)), strings.ReplaceAll(unsupported, "_", " ")) {
			t.Fatalf("presentation invented unsupported inference %q", unsupported)
		}
	}
	if len(answer.NegativeCases) != 5 {
		t.Fatalf("negative inference/target cases are incomplete: %v", answer.NegativeCases)
	}
}
