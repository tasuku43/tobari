package tobari

import (
	"fmt"
	"strings"
)

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
