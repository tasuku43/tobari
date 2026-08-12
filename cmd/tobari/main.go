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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.New(os.Stdin, os.Stdout, os.Stderr)
	command.Version = version
	command.Commit = commit
	return command.RunContext(ctx, os.Args[1:])
}
