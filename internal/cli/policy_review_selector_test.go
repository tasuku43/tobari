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
				ObservedAt: "2026-08-02T10:00:00Z", ObservationCount: 3,
				ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
				ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
				Host: "api.github.com", Port: 443, Method: "POST", Path: "/repos/example/issues",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
			{
				ID:         "pcy_abcdef0123456789abcdef0123456789",
				ObservedAt: "2026-08-02T10:01:00Z",
				ContextID:  "01912345-6789-7abc-8def-0123456789ae", ContextName: "restricted",
				ProjectID: "01912345-6789-7abc-8def-0123456789ac", ProjectRoot: "/workspace/project",
				Host: "registry.npmjs.org", Port: 443, Method: "GET", Path: "/package/example",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
		},
	}
}

func TestPolicyReviewSelectorRawDetailActionConfirmsAndPreservesOpaqueID(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(),
		strings.NewReader("\x1b[B\ra"), &output,
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
		!strings.Contains(output.String(), "This decision applies only to this Tobari in this Context.") ||
		!strings.Contains(output.String(), "restricted") || !strings.Contains(output.String(), "/workspace/project") {
		t.Fatalf("rich review output = %q", output.String())
	}
	if strings.Contains(output.String(), "Type y") || strings.Contains(output.String(), "Allow this exact permission?") {
		t.Fatalf("detail action triggered a redundant confirmation: %q", output.String())
	}
	if !strings.Contains(output.String(), "\x1b[?25h") {
		t.Fatalf("cursor restore missing: %q", output.String())
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
		applyStyleToken(true, styleWarning, "2 pending permissions in 2 Tobari"),
		applyStyleToken(true, styleText, "default · /workspace/project"),
		"POST   api.github.com:443/repos/example/issues",
		applyStyleToken(true, styleMuted, "3×"),
		applyStyleToken(true, styleMuted, "Selected"),
		"Latest 2026-08-02T10:00:00Z",
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
		applyStyleToken(true, styleDanger, "403"),
		"3 times",
		"2026-08-02T10:00:00Z",
		applyStyleToken(true, styleAccent, "[a] Allow exact"),
		applyStyleToken(true, styleAccent, "[d] Deny exact"),
		applyStyleToken(true, styleMuted, "[q] Back"),
	} {
		if !strings.Contains(detailOutput.String(), want) {
			t.Fatalf("colored detail output %q lacks %q", detailOutput.String(), want)
		}
	}
}

func TestPolicyReviewSelectorGroupsByStableScopeAndKeepsEffectOrderWithinGroup(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	first := report.Items[0]
	secondInFirstScope := first
	secondInFirstScope.ID = "pcy_11111111111111111111111111111111"
	secondInFirstScope.ObservedAt = "2026-08-02T10:02:00Z"
	secondInFirstScope.ObservationCount = 2
	secondInFirstScope.Method = "GET"
	secondInFirstScope.Path = "/notifications"
	report.Items = []tobari.PolicyCandidate{first, report.Items[1], secondInFirstScope}

	display := groupPolicyReviewReport(report)
	if got := []string{display.Items[0].ID, display.Items[1].ID, display.Items[2].ID}; got[0] != first.ID || got[1] != secondInFirstScope.ID || got[2] != report.Items[1].ID {
		t.Fatalf("grouped order = %v", got)
	}

	var output bytes.Buffer
	if lines := renderPolicyReviewListRaw(&output, report, 1, 0, "", 0, false); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, output.String())
	}
	text := output.String()
	if strings.Count(text, "default · /workspace/project") != 1 ||
		strings.Count(text, "restricted · /workspace/project") != 1 {
		t.Fatalf("scope headings were not grouped once: %q", text)
	}
	firstEffect := strings.Index(text, "POST   api.github.com:443/repos/example/issues")
	secondEffect := strings.Index(text, "GET    api.github.com:443/notifications")
	restricted := strings.Index(text, "restricted · /workspace/project")
	if firstEffect < 0 || secondEffect <= firstEffect || restricted <= secondEffect {
		t.Fatalf("effect order is not stable within grouped scopes: %q", text)
	}
	for _, want := range []string{
		"3 pending permissions in 2 Tobari",
		"Selected",
		"GET api.github.com:443/notifications",
		"Observed 2 times · Latest 2026-08-02T10:02:00Z",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("grouped list %q lacks %q", text, want)
		}
	}
}

func TestPolicyReviewSelectorDoesNotGroupMatchingDisplayLabelsAcrossTypedScopes(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	report.Items[1].ContextName = report.Items[0].ContextName
	report.Items[1].ProjectRoot = report.Items[0].ProjectRoot

	var output bytes.Buffer
	if lines := renderPolicyReviewListRaw(&output, report, 0, 0, "", 0, false); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, output.String())
	}
	if got := strings.Count(output.String(), "default · /workspace/project"); got != 2 {
		t.Fatalf("matching display labels produced %d headings, want 2: %q", got, output.String())
	}
	if !strings.Contains(output.String(), "2 pending permissions in 2 Tobari") {
		t.Fatalf("typed scope count was inferred from labels: %q", output.String())
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

func TestPolicyReviewSelectorLineDetailActionConfirmsExactAllow(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("2\na\n"), &output,
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
	for _, want := range []string{"1.", "2.", "Choose [a] to allow exact", "Context   restricted"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("line review output %q lacks %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "[y/N]") || strings.Contains(output.String(), "Allow this exact permission?") {
		t.Fatalf("line action triggered a redundant confirmation: %q", output.String())
	}
}

func TestPolicyReviewSelectorLineDetailActionConfirmsExactDeny(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("1\nd\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_0123456789abcdef0123456789abcdef" ||
		decision.Action != policyReviewActionDeny || decision.Canceled {
		t.Fatalf("decision = %+v", decision)
	}
	if !strings.Contains(output.String(), "Choose [a] to allow exact") || !strings.Contains(output.String(), "Context   default") {
		t.Fatalf("deny detail action missing exact scope: %q", output.String())
	}
	if strings.Contains(output.String(), "[y/N]") || strings.Contains(output.String(), "Deny this exact permission?") {
		t.Fatalf("deny action triggered a redundant confirmation: %q", output.String())
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
