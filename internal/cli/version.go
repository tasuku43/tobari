package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

func runVersion(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	if c.Commit == "" {
		return c.emitResult(ctx, []byte(fmt.Sprintf("%s %s\n", ProgramName, c.Version)))
	}
	return c.emitResult(ctx, []byte(fmt.Sprintf("%s %s (%s)\n", ProgramName, c.Version, c.Commit)))
}
