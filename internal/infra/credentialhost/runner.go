package credentialhost

import (
	"context"
	"io"
	"os/exec"
	"time"
)

// Command is the complete trusted-host process boundary. Callers provide no
// shell string, and Driver constructs every argument and environment entry.
type Command struct {
	Path   string
	Args   []string
	Env    []string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// CommandRunner permits deterministic process tests without network access.
type CommandRunner interface {
	Run(context.Context, Command) error
}

// ExecRunner executes a validated absolute binary path without a shell.
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Path, command.Args...) // #nosec G204 -- Driver validates and digest-binds the absolute executable; argv is fixed.
	process.Env = append([]string(nil), command.Env...)
	process.Dir = command.Dir
	process.Stdin = command.Stdin
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	process.WaitDelay = 2 * time.Second
	return process.Run()
}
