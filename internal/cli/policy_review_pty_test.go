package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	policyReviewPTYChildEnv = "TOBARI_POLICY_REVIEW_PTY_CHILD"
	policyReviewPTYCaseEnv  = "TOBARI_POLICY_REVIEW_PTY_CASE"
)

// TestPolicyReviewPTYChild is the process body for the real-PTY test below.
// It deliberately uses the existing fake runtime, so the test exercises the
// production CLI orchestration and terminal capability without Docker state.
func TestPolicyReviewPTYChild(t *testing.T) {
	if os.Getenv(policyReviewPTYChildEnv) != "1" {
		return
	}

	caseName := os.Getenv(policyReviewPTYCaseEnv)
	isPolicyRules := strings.HasPrefix(caseName, "policy-rules")
	terminal := caseName != "json" && caseName != "policy-rules-json"
	runtimeFake, candidateID := newPolicyReviewPTYRuntime(terminal)
	if isPolicyRules {
		runtimeFake, candidateID = newPolicyRulesPTYRuntime(caseName != "policy-rules-json")
	}
	command := newCLI(os.Stdin, os.Stdout, os.Stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtimeFake)
	args := []string{"policy", "review"}
	if isPolicyRules {
		args = []string{"policy", "rules"}
	}
	if caseName == "json" || caseName == "policy-rules-json" {
		args = append(args, "--format", "json")
	}
	code := command.RunContext(context.Background(), args)
	if isPolicyRules {
		fmt.Fprintf(os.Stderr,
			"POLICY_RULES_E2E case=%s code=%d apply_calls=%d rules=%d rule=%s\n",
			caseName, code, runtimeFake.applyCalls, len(runtimeFake.rules), candidateID,
		)
		if code != ExitOK {
			t.Fatalf("policy rules returned %d", code)
		}
		return
	}

	sourceCandidate := ""
	if len(runtimeFake.rules) > 0 && len(runtimeFake.rules[0].SourceCandidates) > 0 {
		sourceCandidate = runtimeFake.rules[0].SourceCandidates[0]
	}
	denyCandidate := ""
	if len(runtimeFake.denyRules) > 0 && len(runtimeFake.denyRules[0].SourceCandidates) > 0 {
		denyCandidate = runtimeFake.denyRules[0].SourceCandidates[0]
	}
	fmt.Fprintf(os.Stderr,
		"POLICY_REVIEW_E2E case=%s code=%d apply_calls=%d deny_calls=%d candidate=%s source_candidate=%s deny_candidate=%s\n",
		caseName, code, runtimeFake.applyCalls, runtimeFake.denyCalls,
		candidateID, sourceCandidate, denyCandidate,
	)
	if code != ExitOK {
		t.Fatalf("policy review returned %d", code)
	}
}

func TestPolicyReviewRealPTYAndReadOnlyE2E(t *testing.T) {
	if os.Getenv(policyReviewPTYChildEnv) == "1" {
		return
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the supported raw-terminal test requires a Unix PTY")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 PTY helper is required: %v", err)
	}

	candidateID := policyReviewPTYCandidateID()

	t.Run("allow through real PTY", func(t *testing.T) {
		output := runPolicyReviewPTYChild(t, "allow", "1ap")
		for _, want := range []string{
			"Tobari · Permission Inbox",
			"1 pending permission in 1 Tobari",
			"default · /workspace/project",
			"POST   api.example.com:443/repos/example/issues",
			"Selected",
			"Observed 1 time · Latest 2026-08-02T10:00:00Z",
			"Permission 1 of 1",
			"This decision applies only to this Tobari in this Context.",
			"[a] Allow exact",
			"Reviewed permissions applied",
			"\x1b[?25h",
			"POLICY_REVIEW_E2E case=allow code=0 apply_calls=1 deny_calls=0",
			"source_candidate=" + candidateID,
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("PTY output lacks %q: %q", want, output)
			}
		}
	})

	t.Run("deny through real PTY", func(t *testing.T) {
		output := runPolicyReviewPTYChild(t, "deny", "1dp")
		for _, want := range []string{
			"Tobari · Permission Inbox",
			"Permission 1 of 1",
			"Reviewed permissions applied",
			"\x1b[?25h",
			"POLICY_REVIEW_E2E case=deny code=0 apply_calls=1 deny_calls=1",
			"deny_candidate=" + candidateID,
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("PTY output lacks %q: %q", want, output)
			}
		}
	})

	for _, test := range []struct {
		name  string
		input string
	}{
		{name: "cancel", input: "q"},
		{name: "invalid then cancel", input: "9q"},
		{name: "list allow key then cancel", input: "aq"},
		{name: "list deny key then cancel", input: "dq"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := runPolicyReviewPTYChild(t, test.name, test.input)
			if !strings.Contains(output, "Permission review canceled") ||
				!strings.Contains(output, "\x1b[?25h") ||
				!strings.Contains(output, "apply_calls=0 deny_calls=0") {
				t.Fatalf("non-mutating PTY output = %q", output)
			}
			if strings.Contains(output, "Permission allowed") || strings.Contains(output, "Permission denied") {
				t.Fatalf("non-mutating PTY output reports a policy change: %q", output)
			}
		})
	}

	t.Run("redirected JSON stays read-only", func(t *testing.T) {
		output := runPolicyReviewJSONChild(t)
		for _, want := range []string{
			`"policy_review"`,
			candidateID,
			"POLICY_REVIEW_E2E case=json code=0 apply_calls=0 deny_calls=0",
		} {
			if !strings.Contains(output, want) {
				t.Fatalf("redirected JSON output lacks %q: %q", want, output)
			}
		}
	})
}

func TestPolicyReviewDirectDetailActionRealPTYAndCancellation(t *testing.T) {
	if os.Getenv(policyReviewPTYChildEnv) == "1" {
		return
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the supported raw-terminal test requires a Unix PTY")
	}

	for _, test := range []struct {
		name     string
		input    string
		marker   string
		wantText []string
	}{
		{
			name:     "delayed-allow",
			input:    "1|a|p",
			marker:   "case=delayed-allow code=0 apply_calls=1 deny_calls=0",
			wantText: []string{"source_candidate=" + policyReviewPTYCandidateID(), "Reviewed permissions applied"},
		},
		{
			name:     "delayed-deny",
			input:    "1|d|p",
			marker:   "case=delayed-deny code=0 apply_calls=1 deny_calls=1",
			wantText: []string{"deny_candidate=" + policyReviewPTYCandidateID(), "Reviewed permissions applied"},
		},
		{
			name:     "back-then-cancel",
			input:    "1|q|q",
			marker:   "case=back-then-cancel code=0 apply_calls=0 deny_calls=0",
			wantText: []string{"Permission review canceled", "Changed", "No permissions changed."},
		},
		{
			name:     "invalid-detail-key-then-cancel",
			input:    "1|x|q|q",
			marker:   "case=invalid-detail-key-then-cancel code=0 apply_calls=0 deny_calls=0",
			wantText: []string{"Press a to allow exact, d to deny exact, or q to go back.", "Permission review canceled", "No permissions changed."},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			output := runPolicyReviewPTYChild(t, test.name, test.input)
			for _, want := range append([]string{
				"PTY_META rows=40 cols=120",
				"\x1b[?25h",
				"Tobari · Permission Inbox",
				test.marker,
			}, test.wantText...) {
				if !strings.Contains(output, want) {
					t.Fatalf("PTY output lacks %q: %q", want, output)
				}
			}
			if strings.Contains(output, "undeclared_fault_contract") {
				t.Fatalf("PTY output contains an undeclared fault: %q", output)
			}
			if test.name == "delayed-allow" || test.name == "delayed-deny" {
				if got := strings.Count(output, "Tobari · Permission Inbox"); got < 2 {
					t.Fatalf("direct detail action did not return to the staged queue, redraws=%d output=%q", got, output)
				}
				if strings.Contains(output, "Type y") || strings.Contains(output, "this exact permission?") {
					t.Fatalf("direct detail action requested redundant confirmation: %q", output)
				}
			}
		})
	}
}

func TestPolicyRulesRealPTYResetAndReadOnlyE2E(t *testing.T) {
	if os.Getenv(policyReviewPTYChildEnv) == "1" {
		return
	}
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("the supported raw-terminal test requires a Unix PTY")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 PTY helper is required: %v", err)
	}

	ruleID := policyRulesPTYRuleID()
	output := runPolicyReviewPTYChild(t, "policy-rules-reset", "1ry")
	for _, want := range []string{
		"Tobari · Policy decisions",
		"Reset returns this exact effect to default deny.",
		"Policy decision reset",
		"No learned policy decisions",
		"\x1b[?25h",
		"POLICY_RULES_E2E case=policy-rules-reset code=0 apply_calls=1 rules=0 rule=" + ruleID,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("policy rules PTY output lacks %q: %q", want, output)
		}
	}

	output = runPolicyRulesJSONChild(t)
	for _, want := range []string{
		`"policy_rules"`, ruleID,
		"POLICY_RULES_E2E case=policy-rules-json code=0 apply_calls=0 rules=1 rule=" + ruleID,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("policy rules JSON output lacks %q: %q", want, output)
		}
	}
}

func runPolicyReviewPTYChild(t *testing.T, caseName, input string) string {
	t.Helper()
	output, err := runPolicyReviewPTYChildResult(t, caseName, input)
	if err != nil {
		t.Fatalf("PTY child failed: %v\noutput=%q", err, output)
	}
	return output
}

func runPolicyReviewPTYChildResult(t *testing.T, caseName, input string) (string, error) {
	t.Helper()
	command := exec.Command("python3", "-c", policyReviewPTYPython,
		os.Args[0], "-test.run=^TestPolicyReviewPTYChild$", "-test.v", input)
	command.Env = append(os.Environ(),
		policyReviewPTYChildEnv+"=1",
		policyReviewPTYCaseEnv+"="+caseName,
		"TERM=xterm-256color",
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String() + stderr.String(), err
}

const policyReviewPTYPython = `
import errno
import fcntl
import os
import pty
import select
import struct
import sys
import termios
import time

args = sys.argv[1:-1]
payload = sys.argv[-1]
pid, master = pty.fork()
if pid == 0:
    os.execv(args[0], args)

fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
os.write(1, b"PTY_META rows=40 cols=120\n")
time.sleep(1.0)
for index, part in enumerate(payload.split("|")):
    try:
        os.write(master, part.encode())
    except OSError as error:
        if error.errno == errno.EIO:
            break
        raise
    if index + 1 < len(payload.split("|")):
        time.sleep(0.75)
status = None
while status is None:
    ready, _, _ = select.select([master], [], [], 0.1)
    if master in ready:
        try:
            data = os.read(master, 4096)
        except OSError as error:
            if error.errno == errno.EIO:
                data = b""
            else:
                raise
        if data:
            os.write(1, data)
    waited, status = os.waitpid(pid, os.WNOHANG)
    if waited == 0:
        status = None

while True:
    try:
        data = os.read(master, 4096)
    except OSError as error:
        if error.errno == errno.EIO:
            break
        raise
    if not data:
        break
    os.write(1, data)

if os.WIFEXITED(status):
    raise SystemExit(os.WEXITSTATUS(status))
raise SystemExit(128 + os.WTERMSIG(status))
`

func runPolicyReviewJSONChild(t *testing.T) string {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestPolicyReviewPTYChild$", "-test.v")
	command.Env = append(os.Environ(),
		policyReviewPTYChildEnv+"=1",
		policyReviewPTYCaseEnv+"=json",
	)
	command.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("JSON child failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}

func runPolicyRulesJSONChild(t *testing.T) string {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestPolicyReviewPTYChild$", "-test.v")
	command.Env = append(os.Environ(),
		policyReviewPTYChildEnv+"=1",
		policyReviewPTYCaseEnv+"=policy-rules-json",
	)
	command.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("policy rules JSON child failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}

func newPolicyReviewPTYRuntime(terminal bool) (*policyReviewRuntimeApplyingFake, string) {
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		panic(err)
	}
	return &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			denials:  []tobari.PolicyDenial{denial},
			terminal: terminal,
		},
	}, candidate.ID
}

func newPolicyRulesPTYRuntime(terminal bool) (*policyReviewRuntimeApplyingFake, string) {
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "8185da2688d7469aae9cd9068e920b0b",
		ContextID: "01912345-6789-7abc-8def-0123456789ad", ContextName: "default",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project", Host: "api.example.com", Port: 443,
		Method: "POST", Path: "/repos/example/issues", Reason: "request did not match an allow rule",
		StatusCode: 403, Learnable: true,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		panic(err)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		panic(err)
	}
	return &policyReviewRuntimeApplyingFake{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			state:    tobari.State{PolicyDirectory: "/tmp/policy"},
			rules:    []tobari.LearnedPolicyRule{rule},
			terminal: terminal,
		},
	}, rule.ID
}

func policyReviewPTYCandidateID() string {
	_, candidateID := newPolicyReviewPTYRuntime(false)
	return candidateID
}

func policyRulesPTYRuleID() string {
	_, ruleID := newPolicyRulesPTYRuntime(false)
	return ruleID
}
