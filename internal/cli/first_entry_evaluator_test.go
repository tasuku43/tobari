package cli

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type firstEntryEvaluatorFixture struct {
	SchemaVersion int `json:"schema_version"`
	Rows          []struct {
		ID       string `json:"id"`
		Scenario string `json:"scenario"`
	} `json:"rows"`
}

type firstEntryEvaluatorAnswer struct {
	SchemaVersion int `json:"schema_version"`
	Answers       []struct {
		ID                  string   `json:"id"`
		ExpectedStages      []string `json:"expected_stages"`
		ExpectedNext        string   `json:"expected_next"`
		ExpectedChangeState string   `json:"expected_change_state"`
		RequiredGates       []string `json:"required_gates"`
	} `json:"answers"`
}

func TestFirstEntryEvaluatorHasExactlyTheAcceptedSevenRows(t *testing.T) {
	var fixture firstEntryEvaluatorFixture
	readJSONTestFile(t, "first_entry_evaluator_fixture.json", &fixture)
	var answer firstEntryEvaluatorAnswer
	readJSONTestFile(t, "first_entry_evaluator_answer.json", &answer)

	wantIDs := []string{"E1", "E2", "E3", "E4", "E5", "E6", "E7"}
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 || len(fixture.Rows) != len(wantIDs) || len(answer.Answers) != len(wantIDs) {
		t.Fatalf("evaluator shape fixture=%+v answer=%+v", fixture, answer)
	}
	for index, id := range wantIDs {
		if fixture.Rows[index].ID != id || answer.Answers[index].ID != id || fixture.Rows[index].Scenario == "" || answer.Answers[index].ExpectedStages == nil || answer.Answers[index].RequiredGates == nil {
			t.Fatalf("evaluator row %d = %+v / %+v", index, fixture.Rows[index], answer.Answers[index])
		}
	}

	for index := range fixture.Rows {
		t.Run(fixture.Rows[index].ID+"_"+fixture.Rows[index].Scenario, func(t *testing.T) {
			verifyFirstEntryEvaluatorRow(t, fixture.Rows[index].ID, answer.Answers[index])
		})
	}
}

func verifyFirstEntryEvaluatorRow(t *testing.T, id string, answer struct {
	ID                  string   `json:"id"`
	ExpectedStages      []string `json:"expected_stages"`
	ExpectedNext        string   `json:"expected_next"`
	ExpectedChangeState string   `json:"expected_change_state"`
	RequiredGates       []string `json:"required_gates"`
}) {
	t.Helper()
	switch id {
	case "E1":
		command, pair, readiness, cluster, reviewer, stdout, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
		if code := command.RunContext(context.Background(), []string{"--", "claude"}); code != ExitOK {
			t.Fatalf("fresh entry exit=%d stderr=%q", code, stderr.String())
		}
		if reviewer.calls != 1 || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 1 || pair.entryCalls != 1 || stdout.Len() != 0 || !reflect.DeepEqual(*order, []string{"observe", "review", "readiness", "template body", "resolve", "cluster", "refresh", "entry"}) {
			t.Fatalf("fresh boundary order=%v review=%d readiness=%d cluster=%d resolve=%d entry=%d", *order, reviewer.calls, readiness.calls, cluster.calls, pair.resolveCalls, pair.entryCalls)
		}
		assertEvaluatorStages(t, answer.ExpectedStages)
	case "E2":
		command, pair, readiness, cluster, reviewer, stdout, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
		readiness.err = fault.WithClassification(
			fault.New(fault.KindUnavailable, "docker_engine_unavailable", "synthetic selected engine is stopped", false),
			fault.PhasePrecondition, fault.ChangeNone,
		)
		if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
			t.Fatalf("stopped engine exit=%d stderr=%q", code, stderr.String())
		}
		if !reflect.DeepEqual(*order, []string{"observe", "review", "readiness"}) || reviewer.calls != 1 || pair.resolveCalls != 0 || cluster.calls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "tobari "+answer.ExpectedNext) || !humanOutputHasRow(stderr.String(), "Change state", answer.ExpectedChangeState) {
			t.Fatalf("stopped engine crossed boundary order=%v resolve=%d cluster=%d stdout=%q stderr=%q", *order, pair.resolveCalls, cluster.calls, stdout.String(), stderr.String())
		}
		command, pair, _, cluster, _, _, stderr, order = newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
		if code := command.RunContext(context.Background(), nil); code != ExitOK {
			t.Fatalf("stopped cluster convergence exit=%d stderr=%q", code, stderr.String())
		}
		if cluster.calls != 1 || pair.entryCalls != 1 || !reflect.DeepEqual(*order, []string{"observe", "readiness", "resolve", "cluster", "refresh", "entry"}) {
			t.Fatalf("stopped cluster convergence order=%v cluster=%d entry=%d", *order, cluster.calls, pair.entryCalls)
		}
	case "E3":
		command, pair, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
		cluster.err = fault.WithClassification(
			fault.New(fault.KindContract, "unclassified_mutation_outcome", "synthetic cluster outcome is unknown", false),
			fault.PhaseMutation, fault.ChangeUnknown,
		)
		if code := command.RunContext(context.Background(), nil); code != ExitContract {
			t.Fatalf("unknown cluster exit=%d stderr=%q", code, stderr.String())
		}
		if pair.entryCalls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "tobari "+answer.ExpectedNext) || !humanOutputHasRow(stderr.String(), "Change state", answer.ExpectedChangeState) || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
			t.Fatalf("unknown cluster entry=%d stdout=%q stderr=%q", pair.entryCalls, stdout.String(), stderr.String())
		}
	case "E4":
		spec, found := DefaultCatalog().Lookup(answer.ExpectedNext)
		if !found {
			t.Fatalf("explicit entry path %q is absent", answer.ExpectedNext)
		}
		var exactReference, positionalOnly bool
		for _, input := range spec.Agent.Inputs {
			exactReference = exactReference || input.Name == "--id" && input.Required && input.ReferenceKind == tobari.ContextReferenceKind
			positionalOnly = positionalOnly || input.Name == "command" && input.PositionalOnly && input.Cardinality == InputCardinalityRepeatable
		}
		if !exactReference || !positionalOnly {
			t.Fatalf("explicit entry contract exact_ref=%t positional_only=%t", exactReference, positionalOnly)
		}
		snapshot := finalCurrentContextEntrySnapshotFixture(t)
		contextRef, err := tobari.ContextRef(snapshot.Context.ID)
		if err != nil {
			t.Fatal(err)
		}
		port := &finalFirstEntryFixture{publication: tobari.ContextEntryPublication{
			Snapshot: snapshot,
			Outcome:  tobari.WorkspaceSessionOutcome{ExitCode: 23},
		}}
		var stdout, stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
		command.finalContexts = workspaceauthoritycmd.NewContextService(port)
		argv := []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox", "echo unchanged"}
		invocation := append([]string{"context", "enter", "--id", contextRef, "--"}, argv...)
		if code := command.RunContext(context.Background(), invocation); code != 23 {
			t.Fatalf("explicit entry child exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if port.calls != 1 || !port.session.Direct() || !reflect.DeepEqual(port.session.Argv(), argv) || stdout.Len() != 0 {
			t.Fatalf("explicit entry calls=%d direct=%t argv=%q stdout=%q", port.calls, port.session.Direct(), port.session.Argv(), stdout.String())
		}
	case "E5":
		assertEvaluatorStages(t, answer.ExpectedStages)
		if answer.ExpectedNext != "status_schema_3_or_catalog_fault" || answer.ExpectedChangeState != "typed" {
			t.Fatalf("truthful progress answer=%+v", answer)
		}
		if err := validateCausalRecoveryGraph(DefaultCatalog()); err != nil {
			t.Fatal(err)
		}
	case "E6":
		command, pair, _, cluster, reviewer, _, stderr, order := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
		for attempt := 0; attempt < 2; attempt++ {
			if code := command.RunContext(context.Background(), nil); code != ExitOK {
				t.Fatalf("convergence attempt %d exit=%d stderr=%q", attempt+1, code, stderr.String())
			}
		}
		if reviewer.calls != 0 || pair.resolveBody != nil || pair.resolveCalls != 2 || cluster.calls != 2 || pair.entryCalls != 2 || len(*order) != 12 || answer.ExpectedNext != WorkspaceEntryCommandPath || answer.ExpectedChangeState != "known_safe" {
			t.Fatalf("convergence order=%v review=%d body=%v resolve=%d cluster=%d entry=%d", *order, reviewer.calls, pair.resolveBody, pair.resolveCalls, cluster.calls, pair.entryCalls)
		}
	case "E7":
		wantGates := []string{"task check", "task security", "task public:check", "task release:check"}
		if !reflect.DeepEqual(answer.RequiredGates, wantGates) {
			t.Fatalf("repository gates=%v want=%v", answer.RequiredGates, wantGates)
		}
	default:
		t.Fatalf("unaccepted evaluator row %q", id)
	}
}

func assertEvaluatorStages(t *testing.T, want []string) {
	t.Helper()
	stages := tobari.FirstEntryStages()
	got := make([]string, len(stages))
	for index, stage := range stages {
		got[index] = string(stage)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("first-entry stages=%v want=%v", got, want)
	}
}
