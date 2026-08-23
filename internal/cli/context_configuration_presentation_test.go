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

type contextConfigurationAnswer struct {
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	FixedTarget   struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"fixed_target"`
	SelectedContext struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"selected_context"`
	ExplicitEmptyShell struct {
		Variable     string `json:"variable"`
		Source       string `json:"source"`
		Value        string `json:"value"`
		ValuePresent bool   `json:"value_present"`
	} `json:"explicit_empty_shell"`
	GitIdentity struct {
		Source string `json:"source"`
		Name   string `json:"name"`
		Email  string `json:"email"`
	} `json:"git_identity"`
	ExactNextArgv  []string `json:"exact_next_argv"`
	RoutineSuccess struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
	UnsupportedInferences []string `json:"unsupported_inferences"`
}

func TestContextConfigurationPresentationEvidenceUsesOneTypedFixture(t *testing.T) {
	t.Parallel()
	fixturePath := filepath.Join("testdata", "context_configuration_report.json")
	answerPath := filepath.Join("testdata", "context_configuration_answer.json")
	goldenPath := filepath.Join("testdata", "context_configuration_report.txt")
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
	var fixture tobari.ManifestReport
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
	var answer contextConfigurationAnswer
	if err := json.Unmarshal(answerData, &answer); err != nil {
		t.Fatalf("decode presentation answer: %v", err)
	}
	if answer.SchemaVersion != 1 || answer.Task != fixture.Task ||
		answer.FixedTarget.Kind != tobari.ManifestGitIdentityTargetKind ||
		answer.FixedTarget.ID != tobari.ManifestGitIdentityTargetID ||
		answer.SelectedContext.ID != fixture.ID || answer.SelectedContext.Name != fixture.Name {
		t.Fatalf("answer target/task does not match fixture: answer=%+v fixture=%+v", answer, fixture)
	}
	setting, found := contextShellSetting(fixture.ShellEnvironment, answer.ExplicitEmptyShell.Variable)
	if !found || setting.Value == nil || !answer.ExplicitEmptyShell.ValuePresent ||
		string(setting.Source) != answer.ExplicitEmptyShell.Source || *setting.Value != answer.ExplicitEmptyShell.Value {
		t.Fatalf("explicit-empty answer does not match fixture: answer=%+v setting=%+v", answer.ExplicitEmptyShell, setting)
	}
	if fixture.GitIdentity.Name == nil || fixture.GitIdentity.Email == nil ||
		string(fixture.GitIdentity.Source) != answer.GitIdentity.Source ||
		*fixture.GitIdentity.Name != answer.GitIdentity.Name || *fixture.GitIdentity.Email != answer.GitIdentity.Email {
		t.Fatalf("Git identity answer does not match fixture: answer=%+v fixture=%+v", answer.GitIdentity, fixture.GitIdentity)
	}
	if !slices.Equal(expectedSurfaceArgv(answer.ExactNextArgv), []string{ProgramName}) ||
		answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 ||
		!slices.Equal(answer.UnsupportedInferences, []string{
			"authentication", "signing", "provider_account", "command_authority_from_display_text",
		}) {
		t.Fatalf("answer workflow evidence is incomplete: %+v", answer)
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := renderContextReportText(fixture, false); !slices.Equal(got, []byte(expectedSurfaceText(string(golden)))) {
		t.Fatalf("Workspace Manifest configuration text changed\n--- got ---\n%s--- want ---\n%s", got, golden)
	}
	jsonOutput, err := renderContextReport(fixture, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document contextReportDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatalf("decode rendered JSON: %v", err)
	}
	if document.SchemaVersion != 2 || document.Manifest.Task != answer.Task || document.Manifest.ID == nil ||
		*document.Manifest.ID != answer.SelectedContext.ID || document.Manifest.Name != answer.SelectedContext.Name {
		t.Fatalf("rendered JSON lost semantic identity: %+v", document)
	}
	text := expectedSurfaceText(string(golden))
	if !strings.Contains(text, "Shell NO_COLOR: literal \"\"") ||
		!strings.Contains(text, expectedSurfaceText("Next: re-enter a matching Workspace with `tobari`")) {
		t.Fatalf("text does not preserve explicit-empty or exact-next-command evidence: %q", text)
	}
	for _, unsupported := range answer.UnsupportedInferences {
		forbidden := strings.ReplaceAll(unsupported, "_", " ")
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("presentation invented unsupported inference %q: %q", unsupported, text)
		}
	}
	for _, forbidden := range []string{"authenticated", "signed identity", "role=assistant"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("presentation invented unsupported claim %q: %q", forbidden, text)
		}
	}
}
