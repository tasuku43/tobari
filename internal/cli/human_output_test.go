package cli

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestHumanOutputUsesSemanticTokensAndVisibleAlignment(t *testing.T) {
	output := newHumanOutput(true)
	output.heading("✓", "Ready", styleSuccess)
	output.row("State", "healthy", styleSuccess)
	output.row("Details", "secondary", styleMuted)

	value := output.String()
	for _, want := range []string{
		applyStyleToken(true, styleSuccess, "✓"),
		applyStyleToken(true, styleSuccess, "healthy"),
		applyStyleToken(true, styleMuted, fmt.Sprintf("%-*s", humanOutputLabelWidth, "State")),
		applyStyleToken(true, styleMuted, fmt.Sprintf("%-*s", humanOutputLabelWidth, "Details")),
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("human output %q lacks %q", value, want)
		}
	}
	for _, forbidden := range []string{
		applyStyleToken(true, styleAccent, "Ready"),
		applyStyleToken(true, styleSuccess, "Ready"),
	} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("human output %q overstyles heading as %q", value, forbidden)
		}
	}
}

func TestSemanticStyleTokensRenderWithAndWithoutTerminalStyles(t *testing.T) {
	t.Parallel()
	want := []styleToken{
		styleText, styleMuted, styleAccent,
		styleSuccess, styleWarning, styleDanger,
	}
	if !reflect.DeepEqual(semanticStyleTokens, want) {
		t.Fatalf("semantic style tokens = %#v, want %#v", semanticStyleTokens, want)
	}
	for _, token := range semanticStyleTokens {
		plain := applyStyleToken(false, token, "value")
		if plain != "value" || strings.Contains(plain, "\x1b[") {
			t.Fatalf("disabled token %s = %q", token, plain)
		}
		styled := applyStyleToken(true, token, "value")
		if stripANSIStyles(styled) != "value" {
			t.Fatalf("enabled token %s changed content: %q", token, styled)
		}
		if token == styleText && strings.Contains(styled, "\x1b[") {
			t.Fatalf("text token must preserve the terminal default: %q", styled)
		}
		if token != styleText && !strings.Contains(styled, "\x1b[") {
			t.Fatalf("enabled token %s has no shared terminal style: %q", token, styled)
		}
	}
	if got := ansiStyleTokens[styleMuted]; strings.Contains(got, "[2;") {
		t.Fatalf("muted token must not use dim/faint styling: %q", got)
	}
	if got, want := ansiStyleTokens[styleAccent], "\x1b[1;38;5;38m"; got != want {
		t.Fatalf("accent token = %q, want calmer emphasized cyan %q", got, want)
	}
	for _, token := range []styleToken{styleSuccess, styleWarning, styleDanger} {
		if got := ansiStyleTokens[token]; strings.Contains(got, "[1;") || strings.Contains(got, "[2;") {
			t.Fatalf("state token %s should not add emphasis: %q", token, got)
		}
	}
}

func stripANSIStyles(value string) string {
	for _, prefix := range ansiStyleTokens {
		value = strings.ReplaceAll(value, prefix, "")
	}
	return strings.ReplaceAll(value, ansiStyleReset, "")
}

func TestNoColorSuppressesANSIWithoutRemovingStateMeaning(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	runtime := &policyReviewRuntimeFake{terminal: true}
	command, stdout, stderr := newTestCLI(passingInspector("ready"))
	command.tobari = tobaricmd.New(runtime)
	command.noColor = noColorFromEnvironment()

	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("doctor exit = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("NO_COLOR output contains ANSI: %q", output)
	}
	for _, want := range []string{"✓ Environment check", "docker_cli", "pass", "ready"} {
		if !strings.Contains(output, want) {
			t.Fatalf("NO_COLOR output %q lacks semantic canary %q", output, want)
		}
	}
	if code := runCLI(command, []string{"missing"}); code != ExitUsage {
		t.Fatalf("missing command exit = %d", code)
	}
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("NO_COLOR stderr contains ANSI: %q", stderr.String())
	}
	for _, want := range []string{"Command failed", "Unknown command", "Next"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("NO_COLOR stderr %q lacks semantic canary %q", stderr.String(), want)
		}
	}
}

func TestNewCapturesNoColorForEveryOutputStream(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	command := New(strings.NewReader(""), &strings.Builder{}, &strings.Builder{})
	if !command.noColor {
		t.Fatal("New did not propagate the presence-only NO_COLOR policy")
	}
}

func TestNonTTYOutputRemainsANSIStyleFree(t *testing.T) {
	runtime := &policyReviewRuntimeFake{terminal: false}
	command, stdout, stderr := newTestCLI(passingInspector("ready"))
	command.tobari = tobaricmd.New(runtime)
	if code := runCLI(command, []string{"doctor"}); code != ExitOK {
		t.Fatalf("doctor exit = %d, stderr = %q", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("non-TTY output contains ANSI: %q", stdout.String())
	}
	if code := runCLI(command, []string{"missing"}); code != ExitUsage {
		t.Fatalf("missing command exit = %d", code)
	}
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("non-TTY error output contains ANSI: %q", stderr.String())
	}
}

func TestStateMeaningDoesNotDependOnColor(t *testing.T) {
	output := newHumanOutput(false)
	output.heading("✓", "Ready", styleSuccess)
	output.row("State", "healthy", styleSuccess)
	output.heading("!", "Needs attention", styleWarning)
	output.row("State", "pending", styleWarning)
	output.heading("✗", "Failed", styleDanger)
	output.row("State", "rejected", styleDanger)
	value := output.String()
	if strings.Contains(value, "\x1b[") {
		t.Fatalf("color-free state output contains ANSI: %q", value)
	}
	for _, want := range []string{
		"✓ Ready", "healthy", "! Needs attention", "pending", "✗ Failed", "rejected",
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("color-free state output %q lacks %q", value, want)
		}
	}
}

func TestDoctorStylesOnlyCheckStatesAndFailureMarker(t *testing.T) {
	t.Parallel()
	report := doctor.Report{Checks: []doctor.Check{
		{Name: "runtime", Status: doctor.CheckStatusPass, Detail: "runtime is available"},
		{Name: "configuration", Status: doctor.CheckStatusWarn, Detail: "using defaults"},
		{Name: "policy", Status: doctor.CheckStatusFail, Detail: "policy test failed"},
	}}
	output, err := renderDoctorReportWithColor(report, successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	value := string(output)
	for _, want := range []string{
		applyStyleToken(true, styleDanger, "✗"),
		applyStyleToken(true, styleSuccess, "pass  "),
		applyStyleToken(true, styleWarning, "warn  "),
		applyStyleToken(true, styleDanger, "fail  "),
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("doctor output %q lacks %q", value, want)
		}
	}
	for _, ordinary := range []string{"Environment check", "runtime", "configuration", "policy", "runtime is available", "using defaults", "policy test failed"} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(value, applyStyleToken(true, token, ordinary)) {
				t.Fatalf("doctor ordinary text %q used %s: %q", ordinary, token, value)
			}
		}
	}
}

func TestHumanErrorUsesSemanticTokensAndExactRecovery(t *testing.T) {
	payload := errorPayload{
		Kind:        fault.KindUnavailable,
		Code:        "cluster_not_running",
		Message:     "cluster is not running",
		Retryable:   false,
		NextActions: []fault.NextAction{{Command: "cluster up", Reason: "Start the shared cluster."}},
	}
	output := string(renderTextErrorWithColor(payload, true))
	for _, want := range []string{
		applyStyleToken(true, styleDanger, "✗"),
		applyStyleToken(true, styleDanger, "cluster is not running"),
		applyStyleToken(true, styleAccent, expectedSurfaceText("tobari cluster up")),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("colored error %q lacks %q", output, want)
		}
	}
	if strings.Contains(output, applyStyleToken(true, styleAccent, "— Start the shared cluster.")) {
		t.Fatalf("colored error accents recovery explanation: %q", output)
	}
}

func TestHumanHelpAndEmptyStateKeepMachineProjectionUnstyled(t *testing.T) {
	command, found := DefaultCatalog().Lookup("cluster status")
	if !found {
		t.Fatal("cluster status is absent from the catalog")
	}
	if output := renderCommandHelpWithColor(command, true); !strings.Contains(string(output), "\x1b[") {
		t.Fatalf("human help is not styled: %q", output)
	}

	empty := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines: 200, Items: []tobari.PolicyCandidate{},
	}
	human, err := renderPolicyCandidatesWithColor(empty, expectedSurfaceText("tobari policy allow"), successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(human), "No policy candidates") || !strings.Contains(string(human), expectedSurfaceText("tobari cluster denials")) {
		t.Fatalf("empty human state = %q", human)
	}

	jsonOutput, err := renderPolicyCandidatesWithColor(empty, expectedSurfaceText("tobari policy allow"), successFormatJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonOutput), "\x1b[") {
		t.Fatalf("JSON output contains terminal colors: %q", jsonOutput)
	}
}
