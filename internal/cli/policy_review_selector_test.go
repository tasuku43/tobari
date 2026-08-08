package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func testPolicyReviewReport() tobari.PolicyCandidateReport {
	return tobari.PolicyCandidateReport{
		Task:            tobari.TaskPolicyReview,
		PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines:     100,
		Items: []tobari.PolicyCandidate{
			{
				ID:         "pcy_0123456789abcdef0123456789abcdef",
				ObservedAt: "2026-08-02T10:00:00Z", ProjectID: "01912345-6789-7abc-8def-0123456789ab",
				Host: "api.github.com", Port: 443, Method: "POST", Path: "/repos/example/issues",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
			{
				ID:         "pcy_abcdef0123456789abcdef0123456789",
				ObservedAt: "2026-08-02T10:01:00Z", ProjectID: "01912345-6789-7abc-8def-0123456789ab",
				Host: "registry.npmjs.org", Port: 443, Method: "GET", Path: "/package/example",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
		},
	}
}

func TestPolicyReviewSelectorRawInspectConfirmAndPreservesOpaqueID(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(),
		strings.NewReader("\x1b[B\ray"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_abcdef0123456789abcdef0123456789" ||
		decision.Action != policyReviewActionAllow || decision.Canceled {
		t.Fatalf("decision = %+v", decision)
	}
	if !strings.Contains(output.String(), "Tobari · Permission Inbox") ||
		!strings.Contains(output.String(), "Permission 2 of 2") ||
		!strings.Contains(output.String(), "This allows exactly this host, port, method, and path.") {
		t.Fatalf("rich review output = %q", output.String())
	}
	if !strings.Contains(output.String(), "Allow this exact permission?") ||
		!strings.Contains(output.String(), "\x1b[?25h") {
		t.Fatalf("confirmation or cursor restore missing: %q", output.String())
	}
}

func TestPolicyReviewSelectorBackThenCancelDoesNotSelect(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &policyReviewSelector{mode: mode, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("\rqq"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" {
		t.Fatalf("decision = %+v", decision)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode calls = entered:%d restored:%d", mode.entered, mode.restored)
	}
	if strings.Contains(output.String(), "Permission allowed") {
		t.Fatalf("cancel output contains an allow result: %q", output.String())
	}
}

func TestPolicyReviewSelectorRawUsesSemanticColor(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()

	var listOutput bytes.Buffer
	if lines := renderPolicyReviewListRaw(&listOutput, report, 0, 0, "", 0, true); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, listOutput.String())
	}
	for _, want := range []string{
		applyStyleToken(true, styleAccent, "Tobari · Permission Inbox"),
		applyStyleToken(true, styleWarning, "2 pending permissions"),
		"❯ ",
	} {
		if !strings.Contains(listOutput.String(), want) {
			t.Fatalf("colored list output %q lacks %q", listOutput.String(), want)
		}
	}

	var detailOutput bytes.Buffer
	if lines := renderPolicyReviewDetailRaw(&detailOutput, report, 0, "", 0, true); lines <= 0 {
		t.Fatalf("detail render lines = %d, output = %q", lines, detailOutput.String())
	}
	for _, want := range []string{
		applyStyleToken(true, styleAccent, "Permission 1 of 2"),
		"api.github.com:443 POST /repos/example/issues",
		applyStyleToken(true, styleWarning, "403"),
		applyStyleToken(true, styleAccent, "[a] Allow"),
		applyStyleToken(true, styleAccent, "[d] Deny"),
		applyStyleToken(true, styleMuted, "[q] Back"),
	} {
		if !strings.Contains(detailOutput.String(), want) {
			t.Fatalf("colored detail output %q lacks %q", detailOutput.String(), want)
		}
	}
}

func TestPolicyReviewSelectorFinishDoesNotLeaveClearedRowsBehind(t *testing.T) {
	t.Parallel()
	var reviewOutput, workspaceOutput bytes.Buffer

	finishPolicyReviewSelector(&reviewOutput, 3)
	finishWorkspaceSelector(&workspaceOutput, 3)

	want := "\x1b[3A\r\x1b[J\x1b[?25h"
	if reviewOutput.String() != want {
		t.Fatalf("policy review finish = %q, want %q", reviewOutput.String(), want)
	}
	if workspaceOutput.String() != reviewOutput.String() {
		t.Fatalf("selector finish differs: workspace=%q review=%q", workspaceOutput.String(), reviewOutput.String())
	}
	if strings.Contains(reviewOutput.String(), "\n") {
		t.Fatalf("selector finish writes blank rows: %q", reviewOutput.String())
	}
}

func TestPolicyReviewSelectorFallsBackToLineConfirmation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("2\na\ny\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_abcdef0123456789abcdef0123456789" || decision.Action != policyReviewActionAllow {
		t.Fatalf("decision = %+v", decision)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("line fallback contains terminal controls: %q", output.String())
	}
	for _, want := range []string{"1.", "2.", "Choose [a] to allow", "Allow exactly this permission? [y/N]"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("line review output %q lacks %q", output.String(), want)
		}
	}
}

func TestPolicyReviewSelectorLineCanConfirmExactDeny(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("1\nd\ny\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_0123456789abcdef0123456789abcdef" ||
		decision.Action != policyReviewActionDeny || decision.Canceled {
		t.Fatalf("decision = %+v", decision)
	}
	if !strings.Contains(output.String(), "Deny exactly this permission? [y/N]") {
		t.Fatalf("deny confirmation missing: %q", output.String())
	}
}

func TestPolicyReviewSelectorRawEOFIsSafeCancellation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader(""), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" || decision.Action != policyReviewActionNone {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPolicyReviewSelectorEOFIsSafeCancellation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader(""), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" {
		t.Fatalf("decision = %+v", decision)
	}
}
