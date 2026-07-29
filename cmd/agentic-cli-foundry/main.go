// Command agentic-cli-foundry is the executable entry point for Agentic CLI Foundry.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tasuku43/agentic-cli-foundry/internal/cli"
)

// Release builds inject both values with -ldflags.
var (
	version = "dev"
	commit  = ""
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	command := cli.New(os.Stdin, os.Stdout, os.Stderr)
	command.Version = version
	command.Commit = commit
	return command.RunContext(ctx, os.Args[1:])
}
