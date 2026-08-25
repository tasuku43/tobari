// Command tobari is the executable entry point for Tobari.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tasuku43/tobari/internal/cli"
)

// Release builds inject both values; repository tasks inject the exact source
// commit while retaining the fixed development version.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	os.Exit(run())
}

func run() int {
	lifetime := context.Background()
	ctx, stop := signal.NotifyContext(lifetime, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cli.IsCredentialCompanionArg0(os.Args[0]) {
		return cli.RunCredentialCompanionContext(ctx, os.Args[1:], os.Stdin)
	}
	command := cli.New(lifetime, os.Stdin, os.Stdout, os.Stderr)
	if os.Getenv("TOBARI_INTEGRATION_FAULT_DIAGNOSTICS") == "true" {
		command.EnableIntegrationFaultDiagnostics()
	}
	command.Version = version
	command.Commit = commit
	return command.RunContext(ctx, os.Args[1:])
}
