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
	runtimeFake, candidateID := newPolicyReviewPTYRuntime(caseName != "json")
	command := newCLI(os.Stdin, os.Stdout, os.Stderr, DefaultCatalog(), nil)
	command.tobari = tobaricmd.New(runtimeFake)
	args := []string{"policy", "review"}
	if caseName == "json" {
		args = append(args, "--format", "json")
	}
	code := command.RunContext(context.Background(), args)

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
		output := runPolicyReviewPTYChild(t, "allow", "1ay")
		for _, want := range []string{
			"Tobari · Permission Inbox",
			"Permission 1 of 1",
			"This allows exactly this host, port, method, and path.",
			"No pending network permissions",
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
		output := runPolicyReviewPTYChild(t, "deny", "1dy")
		for _, want := range []string{
			"Tobari · Permission Inbox",
			"Permission 1 of 1",
			"No pending network permissions",
			"\x1b[?25h",
			"POLICY_REVIEW_E2E case=deny code=0 apply_calls=0 deny_calls=1",
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
	}{{name: "cancel", input: "q"}, {name: "invalid then cancel", input: "9q"}} {
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

func runPolicyReviewPTYChild(t *testing.T, caseName, input string) string {
	t.Helper()
	command := exec.Command("python3", "-c", policyReviewPTYPython,
		os.Args[0], "-test.run=^TestPolicyReviewPTYChild$", "-test.v", input)
	command.Env = append(os.Environ(),
		policyReviewPTYChildEnv+"=1",
		policyReviewPTYCaseEnv+"="+caseName,
	)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("PTY child failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	return stdout.String() + stderr.String()
}

const policyReviewPTYPython = `
import errno
import os
import pty
import select
import sys
import time

args = sys.argv[1:-1]
payload = sys.argv[-1].encode()
pid, master = pty.fork()
if pid == 0:
    os.execv(args[0], args)

time.sleep(1.0)
os.write(master, payload)
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

func newPolicyReviewPTYRuntime(terminal bool) (*policyReviewRuntimeApplyingFake, string) {
	denial := tobari.PolicyDenial{
		Timestamp: "2026-08-02T10:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", Host: "api.example.com", Port: 443,
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

func policyReviewPTYCandidateID() string {
	_, candidateID := newPolicyReviewPTYRuntime(false)
	return candidateID
}
