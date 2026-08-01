package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestHumanOutputUsesSemanticTokensAndVisibleAlignment(t *testing.T) {
	output := newHumanOutput(true)
	output.heading("✓", "Ready", colorTokenSuccess)
	output.row("State", "healthy", colorTokenSuccess)
	output.row("Details", "secondary", colorTokenMuted)

	value := output.String()
	for _, want := range []string{
		applyColorToken(true, colorTokenSuccess, "✓"),
		applyColorToken(true, colorTokenSuccess, "healthy"),
		applyColorToken(true, colorTokenMuted, fmt.Sprintf("%-*s", humanOutputLabelWidth, "State")),
		applyColorToken(true, colorTokenMuted, fmt.Sprintf("%-*s", humanOutputLabelWidth, "Details")),
	} {
		if !strings.Contains(value, want) {
			t.Fatalf("human output %q lacks %q", value, want)
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
		applyColorToken(true, colorTokenError, "✗"),
		applyColorToken(true, colorTokenError, "cluster is not running"),
		applyColorToken(true, colorTokenAccent, "tobari cluster up — Start the shared cluster."),
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("colored error %q lacks %q", output, want)
		}
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
	human, err := renderPolicyCandidatesWithColor(empty, "tobari policy allow", successFormatText, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(human), "No policy candidates") || !strings.Contains(string(human), "tobari cluster denials") {
		t.Fatalf("empty human state = %q", human)
	}

	jsonOutput, err := renderPolicyCandidatesWithColor(empty, "tobari policy allow", successFormatJSON, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(jsonOutput), "\x1b[") {
		t.Fatalf("JSON output contains terminal colors: %q", jsonOutput)
	}
}
