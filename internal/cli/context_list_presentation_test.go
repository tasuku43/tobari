package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextListPresentationFixture struct {
	Result            tobari.ManifestListResult `json:"result"`
	RuntimeSelections map[string]string         `json:"runtime_selections"`
}

type contextListPresentationAnswer struct {
	SchemaVersion     int               `json:"schema_version"`
	Task              string            `json:"task"`
	Scope             string            `json:"scope"`
	Current           string            `json:"current"`
	RuntimeSelections map[string]string `json:"runtime_selections"`
	ActionRequired    []string          `json:"action_required"`
	RequiredFacts     []string          `json:"required_facts"`
	DetailOnlyFacts   []string          `json:"detail_only_facts"`
	UnsupportedClaims []string          `json:"unsupported_inferences"`
	RoutineSuccess    struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
}

func TestContextListPresentationEvidenceKeepsExactRuntimeOutsideSchemaOne(t *testing.T) {
	t.Parallel()
	paths := map[string]string{
		"fixture": filepath.Join("testdata", "context_list_report.json"),
		"answer":  filepath.Join("testdata", "context_list_answer.json"),
		"before":  filepath.Join("testdata", "context_list_before.txt"),
		"summary": filepath.Join("testdata", "context_list_summary.txt"),
	}
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("presentation evidence %q is not a regular repository file: info=%v err=%v", path, info, err)
		}
	}

	fixtureData, err := os.ReadFile(paths["fixture"])
	if err != nil {
		t.Fatal(err)
	}
	var fixture contextListPresentationFixture
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("decode typed fixture: %v", err)
	}
	for index := range fixture.Result.Items {
		fixture.Result.Items[index].RuntimeSelection = fixture.RuntimeSelections[fixture.Result.Items[index].Name]
	}
	if err := fixture.Result.Validate(); err != nil {
		t.Fatalf("typed fixture is invalid: %v", err)
	}
	for _, item := range fixture.Result.Items {
		if _, err := item.RoutineSummary(); err != nil {
			t.Fatalf("Workspace Manifest %q routine summary is invalid: %v", item.Name, err)
		}
	}

	answerData, err := os.ReadFile(paths["answer"])
	if err != nil {
		t.Fatal(err)
	}
	var answer contextListPresentationAnswer
	if err := json.Unmarshal(answerData, &answer); err != nil {
		t.Fatalf("decode presentation answer: %v", err)
	}
	if answer.SchemaVersion != 1 || answer.Task != fixture.Result.Task || answer.Scope != "installation" ||
		answer.Current != fixture.Result.DefaultManifest || !mapsEqual(answer.RuntimeSelections, fixture.RuntimeSelections) ||
		answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 {
		t.Fatalf("answer does not match typed fixture: answer=%+v fixture=%+v", answer, fixture)
	}

	before, err := os.ReadFile(paths["before"])
	if err != nil {
		t.Fatal(err)
	}
	summaryGolden, err := os.ReadFile(paths["summary"])
	if err != nil {
		t.Fatal(err)
	}
	textOutput, err := renderContextList(fixture.Result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(textOutput, summaryGolden) {
		t.Fatalf("Workspace Manifest list summary changed\n--- got ---\n%s--- want ---\n%s", textOutput, summaryGolden)
	}
	for _, fact := range answer.RequiredFacts {
		if !strings.Contains(string(summaryGolden), fact) {
			t.Fatalf("summary omits required semantic fact %q: %q", fact, summaryGolden)
		}
	}
	for _, fact := range answer.DetailOnlyFacts {
		if strings.Contains(string(summaryGolden), fact) || !strings.Contains(string(before), fact) {
			t.Fatalf("routine summary did not hide baseline diagnostic %q", fact)
		}
	}
	for _, unsupported := range answer.UnsupportedClaims {
		if strings.Contains(strings.ToLower(string(summaryGolden)), strings.ReplaceAll(unsupported, "_", " ")) {
			t.Fatalf("summary invents unsupported inference %q", unsupported)
		}
	}
	for _, name := range answer.ActionRequired {
		if !strings.Contains(string(summaryGolden), "  "+name+" !") {
			t.Fatalf("summary omits action marker for %q", name)
		}
	}

	jsonOutput, err := renderContextList(fixture.Result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	contexts := document["workspace_manifests"].(map[string]any)
	items := contexts["items"].([]any)
	for _, raw := range items {
		item := raw.(map[string]any)
		if _, found := item["runtime_selection"]; found {
			t.Fatalf("internal display selection changed schema-1 JSON: %v", item)
		}
	}
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
