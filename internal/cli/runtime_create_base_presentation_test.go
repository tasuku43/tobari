package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimeCreateBaseFixtureSHA256 = "16537101698b82e21f489cb13e1f2202fda7e4fc4132387232481a5897f9822e"
	runtimeCreateBaseAnswerSHA256  = "9f35d9115b7157758fe43c546a898ee2242d9519ea76e353d0c6e4838a3fa464"
)

type runtimeCreateBaseFixture struct {
	SchemaVersion int                     `json:"schema_version"`
	Report        tobari.RuntimeReport    `json:"report"`
	Candidates    []tobari.RuntimeSummary `json:"candidates"`
}

type runtimeCreateBaseAnswer struct {
	SchemaVersion  int      `json:"schema_version"`
	Task           string   `json:"task"`
	NewRuntime     string   `json:"new_runtime"`
	BaseCandidates []string `json:"base_candidates"`
	InitialBase    string   `json:"initial_base"`
	ExactNextArgv  []string `json:"exact_next_argv"`
	RequiredFacts  []string `json:"required_summary_facts"`
	ChooserFacts   []string `json:"chooser_facts"`
	Unsupported    []string `json:"unsupported_inferences"`
	RoutineSuccess struct {
		TaskInvocations             int `json:"task_invocations"`
		ExternalReconstructionSteps int `json:"external_reconstruction_steps"`
	} `json:"routine_success"`
}

func readRuntimeCreateBaseCorpus(t *testing.T) (runtimeCreateBaseFixture, runtimeCreateBaseAnswer) {
	t.Helper()
	read := func(path, want string, target any) {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if got := hex.EncodeToString(digest[:]); got != want {
			t.Fatalf("%s SHA-256 = %s, want %s", path, got, want)
		}
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatal(err)
		}
	}
	var fixture runtimeCreateBaseFixture
	var answer runtimeCreateBaseAnswer
	read("testdata/runtime_create_base_report.json", runtimeCreateBaseFixtureSHA256, &fixture)
	read("testdata/runtime_create_base_answer.json", runtimeCreateBaseAnswerSHA256, &answer)
	return fixture, answer
}

func TestRuntimeCreateBasePinnedPresentationHasNoLineageInference(t *testing.T) {
	fixture, answer := readRuntimeCreateBaseCorpus(t)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 || answer.Task != tobari.TaskRuntimeCreate || answer.NewRuntime != fixture.Report.Runtime.Name {
		t.Fatalf("corpus identity = %+v/%+v", fixture, answer)
	}
	if err := fixture.Report.Validate(); err != nil {
		t.Fatal(err)
	}
	list := tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: fixture.Candidates}
	if err := list.Validate(); err != nil {
		t.Fatal(err)
	}
	if answer.RoutineSuccess.TaskInvocations != 1 || answer.RoutineSuccess.ExternalReconstructionSteps != 0 {
		t.Fatalf("routine success = %+v", answer.RoutineSuccess)
	}

	got, err := renderRuntimeReport("runtime create", fixture.Report, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile("testdata/runtime_create_before.txt")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile("testdata/runtime_create_base_summary.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, after) || !slices.Equal(before, after) {
		t.Fatalf("Runtime create summary changed\n--- got ---\n%s--- after ---\n%s--- before ---\n%s", got, after, before)
	}
	for _, fact := range answer.RequiredFacts {
		if !strings.Contains(string(got), fact) {
			t.Errorf("summary lacks %q: %q", fact, got)
		}
	}
	if !strings.Contains(string(got), strings.Join(answer.ExactNextArgv[1:], " ")) {
		t.Errorf("summary lacks exact next argv %q: %q", answer.ExactNextArgv, got)
	}
	for _, unsupported := range answer.Unsupported {
		if strings.Contains(string(got), unsupported) {
			t.Errorf("summary invented %q: %q", unsupported, got)
		}
	}

	fake := &runtimeCatalogCLI{manifest: fixture.Report.Runtime, list: fixture.Candidates}
	var output bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &bytes.Buffer{}, &output, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}
	selected, err := chooseRuntimeCreateBase(context.Background(), command, answer.NewRuntime)
	if err != nil || selected != answer.InitialBase {
		t.Fatalf("Base chooser = %q/%v", selected, err)
	}
	for _, fact := range answer.ChooserFacts {
		if !strings.Contains(output.String(), fact) {
			t.Errorf("chooser lacks %q: %q", fact, output.String())
		}
	}
	if got := []string{fixture.Candidates[0].Name, fixture.Candidates[1].Name, fixture.Candidates[2].Name}; !slices.Equal(got, answer.BaseCandidates) {
		t.Fatalf("Base candidates = %v, want %v", got, answer.BaseCandidates)
	}
}
