package tobari

import (
	"fmt"
	"strings"
)

type WorkspaceAttachmentCleanupIssue string

const (
	WorkspaceCleanupInteractiveSession WorkspaceAttachmentCleanupIssue = "interactive_session"
	WorkspaceCleanupHostLoopback       WorkspaceAttachmentCleanupIssue = "host_loopback"
	WorkspaceCleanupPermissionChannel  WorkspaceAttachmentCleanupIssue = "permission_channel"
)

func (i WorkspaceAttachmentCleanupIssue) Validate() error {
	switch i {
	case WorkspaceCleanupInteractiveSession, WorkspaceCleanupHostLoopback, WorkspaceCleanupPermissionChannel:
		return nil
	default:
		return fmt.Errorf("Workspace attachment cleanup issue is invalid")
	}
}

// WorkspaceSessionOutcome preserves the authoritative child status while
// carrying bounded secondary attachment cleanup evidence independently. A
// cleanup issue never rewrites ExitCode and carries no raw infrastructure text.
type WorkspaceSessionOutcome struct {
	ExitCode      int
	CleanupIssues []WorkspaceAttachmentCleanupIssue
}

func (o WorkspaceSessionOutcome) Validate() error {
	if o.ExitCode < 0 || o.ExitCode > 255 {
		return fmt.Errorf("Workspace child exit status is invalid")
	}
	seen := map[WorkspaceAttachmentCleanupIssue]struct{}{}
	for _, issue := range o.CleanupIssues {
		if err := issue.Validate(); err != nil {
			return err
		}
		if _, exists := seen[issue]; exists {
			return fmt.Errorf("Workspace attachment cleanup issue is duplicated")
		}
		seen[issue] = struct{}{}
	}
	return nil
}

// WorkspaceSessionRequest selects either the default interactive shell or one
// exact argv for the attachment child. A nil argv means the default shell;
// direct argv is copied at construction so outer layers cannot change command
// meaning after validation.
type WorkspaceSessionRequest struct {
	argv []string
}

// NewWorkspaceShellSession requests the existing interactive Bash attachment.
func NewWorkspaceShellSession() WorkspaceSessionRequest {
	return WorkspaceSessionRequest{}
}

// NewWorkspaceDirectSession requests one exact foreground child argv.
func NewWorkspaceDirectSession(argv []string) (WorkspaceSessionRequest, error) {
	request := WorkspaceSessionRequest{argv: make([]string, len(argv))}
	copy(request.argv, argv)
	if err := request.Validate(); err != nil {
		return WorkspaceSessionRequest{}, err
	}
	return request, nil
}

// Validate rejects an ambiguous empty direct request and values that cannot be
// represented as OS argv. Empty arguments after the executable remain valid.
func (r WorkspaceSessionRequest) Validate() error {
	if r.argv == nil {
		return nil
	}
	if len(r.argv) == 0 {
		return fmt.Errorf("direct Workspace session command is missing")
	}
	if r.argv[0] == "" {
		return fmt.Errorf("direct Workspace session executable is empty")
	}
	for _, argument := range r.argv {
		if strings.IndexByte(argument, 0) >= 0 {
			return fmt.Errorf("direct Workspace session argv contains NUL")
		}
	}
	return nil
}

// Direct reports whether this attachment runs caller-supplied exact argv.
func (r WorkspaceSessionRequest) Direct() bool {
	return r.argv != nil
}

// Argv returns a detached exact argv. It is nil for the default shell session.
func (r WorkspaceSessionRequest) Argv() []string {
	return append([]string(nil), r.argv...)
}
