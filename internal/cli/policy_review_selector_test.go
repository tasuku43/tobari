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
	selector := &policyReviewSelector{mode: &selectorModeFake{}}
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
	selector := &policyReviewSelector{mode: mode}
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

func TestPolicyReviewSelectorFallsBackToLineConfirmation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}}
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
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}}
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
	selector := &policyReviewSelector{mode: &selectorModeFake{}}
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
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}}
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
