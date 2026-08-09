// Command tobari is the executable entry point for Tobari.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tasuku43/tobari/internal/cli"
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
	if cli.IsCredentialCompanionArg0(os.Args[0]) {
		return cli.RunCredentialCompanionContext(ctx, os.Args[1:], os.Stdin)
	}
	command := cli.New(os.Stdin, os.Stdout, os.Stderr)
	command.Version = version
	command.Commit = commit
	return command.RunContext(ctx, os.Args[1:])
}
