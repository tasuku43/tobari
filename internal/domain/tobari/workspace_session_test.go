package tobari

import (
	"reflect"
	"testing"
)

func TestWorkspaceSessionRequestDistinguishesShellAndExactDirectArgv(t *testing.T) {
	shell := NewWorkspaceShellSession()
	if err := shell.Validate(); err != nil || shell.Direct() || shell.Argv() != nil {
		t.Fatalf("shell request = direct:%t argv:%q error:%v", shell.Direct(), shell.Argv(), err)
	}

	input := []string{"claude", "--model", "", "--model"}
	direct, err := NewWorkspaceDirectSession(input)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "changed"
	want := []string{"claude", "--model", "", "--model"}
	if !direct.Direct() || !reflect.DeepEqual(direct.Argv(), want) {
		t.Fatalf("direct request = direct:%t argv:%q, want %q", direct.Direct(), direct.Argv(), want)
	}
	got := direct.Argv()
	got[0] = "mutated"
	if !reflect.DeepEqual(direct.Argv(), want) {
		t.Fatalf("Argv() exposed mutable state: %q", direct.Argv())
	}
}

func TestWorkspaceSessionRequestRejectsMissingOrUnrepresentableExecutable(t *testing.T) {
	for name, argv := range map[string][]string{
		"empty direct":  {},
		"empty command": {"", "argument"},
		"command NUL":   {"bad\x00command"},
		"argument NUL":  {"command", "bad\x00argument"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewWorkspaceDirectSession(argv); err == nil {
				t.Fatalf("NewWorkspaceDirectSession(%q) succeeded", argv)
			}
		})
	}
}

func TestWorkspaceSessionOutcomeKeepsExitAndBoundedCleanupIssues(t *testing.T) {
	outcome := WorkspaceSessionOutcome{ExitCode: 37, CleanupIssues: []WorkspaceAttachmentCleanupIssue{
		WorkspaceCleanupHostLoopback, WorkspaceCleanupInteractiveSession, WorkspaceCleanupPermissionChannel, WorkspaceCleanupServiceExposure,
	}, ServiceCleanupReceipt: &ServiceCleanupReceipt{SchemaVersion: 1, PendingWithdrawnCount: 1, ExposureClosedCount: 2, StreamClosedCount: 3}}
	if err := outcome.Validate(); err != nil {
		t.Fatal(err)
	}
	outcome.CleanupIssues = append(outcome.CleanupIssues, WorkspaceCleanupHostLoopback)
	if err := outcome.Validate(); err == nil {
		t.Fatal("duplicated cleanup issue was accepted")
	}
}
