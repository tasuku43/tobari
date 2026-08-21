// Command tobari-expose is the fixed Workspace service helper entry point.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tasuku43/tobari/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.NewExposureHelper(os.Stdin, os.Stdout, os.Stderr)
	os.Exit(command.RunContext(ctx, os.Args[1:]))
}
