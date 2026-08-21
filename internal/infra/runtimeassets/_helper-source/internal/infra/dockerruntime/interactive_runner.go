package dockerruntime

import (
	"context"
	"io"
)

// interactiveCommandRunner is deliberately narrower than commandRunner. It
// is used only for the attached Workspace shell when a terminal presentation
// relay is explicitly enabled; inspection and mutation commands remain on the
// ordinary Docker stream path.
type interactiveCommandRunner interface {
	RunInteractive(context.Context, []string, []string, io.Reader, io.Writer, io.Writer, bool) error
}
