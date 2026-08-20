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

type contextShowAnswer struct {
	SchemaVersion   int    `json:"schema_version"`
	Task            string `json:"task"`
	SelectedContext struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Active bool   `json:"active"`
	} `json:"selected_context"`
	ExactNextArgv        []string `json:"exact_next_argv"`
	RequiredSummaryFacts []string `json:"required_summary_facts"`
	DetailOnlyFacts      []string `json:"detail_only_facts"`
	RoutineSuccess       struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
	UnsupportedInferences []string `json:"unsupported_inferences"`
}

func TestContextShowPresentationEvidenceUsesOneTypedFixture(t *testing.T) {
	t.Parallel()
	paths := map[string]string{
		"fixture": filepath.Join("testdata", "context_show_report.json"),
		"answer":  filepath.Join("testdata", "context_show_answer.json"),
		"before":  filepath.Join("testdata", "context_show_before.txt"),
		"summary": filepath.Join("testdata", "context_show_summary.txt"),
		"details": filepath.Join("testdata", "context_show_details.txt"),
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
	var fixture tobari.ContextReport
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("decode typed fixture: %v", err)
	}
	if err := fixture.Validate(); err != nil {
		t.Fatalf("typed fixture is invalid: %v", err)
	}

	answerData, err := os.ReadFile(paths["answer"])
	if err != nil {
		t.Fatal(err)
	}
	var answer contextShowAnswer
	if err := json.Unmarshal(answerData, &answer); err != nil {
		t.Fatalf("decode presentation answer: %v", err)
	}
	if answer.SchemaVersion != 1 || answer.Task != fixture.Task ||
		answer.SelectedContext.ID != fixture.ID || answer.SelectedContext.Name != fixture.Name ||
		answer.SelectedContext.Active != fixture.Active ||
		!slices.Equal(answer.ExactNextArgv, []string{ProgramName, "--context", fixture.Name}) ||
		answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 {
		t.Fatalf("answer does not match typed fixture: answer=%+v fixture=%+v", answer, fixture)
	}

	beforeGolden, err := os.ReadFile(paths["before"])
	if err != nil {
		t.Fatal(err)
	}
	summaryGolden, err := os.ReadFile(paths["summary"])
	if err != nil {
		t.Fatal(err)
	}
	detailsGolden, err := os.ReadFile(paths["details"])
	if err != nil {
		t.Fatal(err)
	}
	if got := renderContextShowSummaryText(fixture, false); !slices.Equal(got, summaryGolden) {
		t.Fatalf("Context show summary changed\n--- got ---\n%s--- want ---\n%s", got, summaryGolden)
	}
	if got := renderContextShowDetailsText(fixture, false); !slices.Equal(got, detailsGolden) {
		t.Fatalf("Context show details changed\n--- got ---\n%s--- want ---\n%s", got, detailsGolden)
	}

	summary := string(summaryGolden)
	details := string(detailsGolden)
	before := string(beforeGolden)
	for _, fact := range answer.RequiredSummaryFacts {
		if !strings.Contains(summary, fact) {
			t.Fatalf("summary omits required semantic fact %q: %q", fact, summary)
		}
	}
	for _, fact := range answer.DetailOnlyFacts {
		if strings.Contains(summary, fact) || !strings.Contains(details, fact) || !strings.Contains(before, fact) {
			t.Fatalf("diagnostic fact %q is not preserved from the flat baseline into details: before=%q summary=%q details=%q", fact, before, summary, details)
		}
	}
	for _, unsupported := range answer.UnsupportedInferences {
		forbidden := strings.ReplaceAll(unsupported, "_", " ")
		if strings.Contains(strings.ToLower(summary), forbidden) || strings.Contains(strings.ToLower(details), forbidden) {
			t.Fatalf("presentation invents unsupported inference %q", unsupported)
		}
	}
}
