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

type contextCreateAnswer struct {
	SchemaVersion   int    `json:"schema_version"`
	Task            string `json:"task"`
	SelectedContext struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Default bool   `json:"default"`
	} `json:"selected_context"`
	ExactNextArgv        []string `json:"exact_next_argv"`
	ExactDetailsArgv     []string `json:"exact_details_argv"`
	RequiredSummaryFacts []string `json:"required_summary_facts"`
	DetailOnlyFacts      []string `json:"detail_only_facts"`
	RoutineSuccess       struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
	UnsupportedInferences []string `json:"unsupported_inferences"`
}

func TestContextCreatePresentationUsesContextShowStructure(t *testing.T) {
	t.Parallel()
	paths := map[string]string{
		"fixture": filepath.Join("testdata", "context_create_report.json"),
		"answer":  filepath.Join("testdata", "context_create_answer.json"),
		"before":  filepath.Join("testdata", "context_create_before.txt"),
		"summary": filepath.Join("testdata", "context_create_summary.txt"),
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
	var fixture tobari.ManifestReport
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
	var answer contextCreateAnswer
	if err := json.Unmarshal(answerData, &answer); err != nil {
		t.Fatalf("decode presentation answer: %v", err)
	}
	exactDetailsArgv := append([]string{ProgramName}, strings.Fields(contextShowDetailsCommand(fixture))...)
	if answer.SchemaVersion != 1 || answer.Task != fixture.Task ||
		answer.SelectedContext.ID != fixture.ID || answer.SelectedContext.Name != fixture.Name ||
		answer.SelectedContext.Default != fixture.Default ||
		!slices.Equal(expectedSurfaceArgv(answer.ExactNextArgv), contextCreateNextArgv(fixture)) ||
		!slices.Equal(expectedSurfaceArgv(answer.ExactDetailsArgv), exactDetailsArgv) ||
		answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 {
		t.Fatalf("answer does not match typed fixture: answer=%+v fixture=%+v", answer, fixture)
	}
	if routed := assertPublicNextArgvRoutes(t, expectedSurfaceArgv(answer.ExactNextArgv)); routed.Path != "tobari" {
		t.Fatalf("create continuation routes to %q, want root entry", routed.Path)
	}
	if routed := assertPublicNextArgvRoutes(t, expectedSurfaceArgv(answer.ExactDetailsArgv)); routed.Path != "manifest show" {
		t.Fatalf("details continuation routes to %q, want context show", routed.Path)
	}

	before, err := os.ReadFile(paths["before"])
	if err != nil {
		t.Fatal(err)
	}
	summary, err := os.ReadFile(paths["summary"])
	if err != nil {
		t.Fatal(err)
	}
	if got := renderContextCreateSummaryText(fixture, false); !slices.Equal(got, []byte(expectedSurfaceText(string(summary)))) {
		t.Fatalf("Workspace Manifest create summary changed\n--- got ---\n%s--- want ---\n%s", got, summary)
	}
	if got := renderContextReportText(fixture, false); !slices.Equal(got, []byte(expectedSurfaceText(string(summary)))) {
		t.Fatalf("Workspace Manifest create did not route through the structured summary\n--- got ---\n%s--- want ---\n%s", got, summary)
	}

	for _, fact := range answer.RequiredSummaryFacts {
		if !strings.Contains(expectedSurfaceText(string(summary)), expectedSurfaceText(fact)) {
			t.Fatalf("summary omits required semantic fact %q: %q", fact, summary)
		}
	}
	detailsFixture := fixture
	detailsFixture.Task = tobari.TaskManifestShow
	detailsFixture.Authentication = tobari.ManifestAuthentication{
		Mode: tobari.ManifestAuthenticationModeNative, Providers: []tobari.ManifestAuthProvider{},
	}
	if err := detailsFixture.Validate(); err != nil {
		t.Fatalf("derived details fixture is invalid: %v", err)
	}
	details := renderContextShowDetailsText(detailsFixture, false)
	for _, fact := range answer.DetailOnlyFacts {
		if strings.Contains(expectedSurfaceText(string(summary)), expectedSurfaceText(fact)) || !strings.Contains(expectedSurfaceText(string(before)), expectedSurfaceText(fact)) || !strings.Contains(expectedSurfaceText(string(details)), expectedSurfaceText(fact)) {
			t.Fatalf("diagnostic fact %q was not isolated behind the details command: before=%q summary=%q details=%q", fact, before, summary, details)
		}
	}
	for _, unsupported := range answer.UnsupportedInferences {
		forbidden := strings.ReplaceAll(unsupported, "_", " ")
		if strings.Contains(strings.ToLower(string(summary)), forbidden) {
			t.Fatalf("presentation invents unsupported inference %q", unsupported)
		}
	}
	if strings.Contains(string(summary), "Authentication") || strings.Contains(string(summary), "Auth status") {
		t.Fatalf("create summary inferred unobserved authentication state: %q", summary)
	}
}
