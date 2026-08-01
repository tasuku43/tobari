package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/operation"
)

func runVersion(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	color := humanColorAllowed(ctx, c, c.Out)
	if color {
		output := newHumanOutput(true)
		output.heading("•", "Tobari", colorTokenAccent)
		output.row("Version", c.Version, colorTokenSuccess)
		if c.Commit != "" {
			output.row("Commit", c.Commit, colorTokenMuted)
		}
		return c.emitResult(ctx, output.bytes())
	}
	if c.Commit == "" {
		return c.emitResult(ctx, []byte(fmt.Sprintf("%s %s\n", applyColorToken(color, colorTokenAccent, ProgramName), applyColorToken(color, colorTokenSuccess, c.Version))))
	}
	return c.emitResult(ctx, []byte(fmt.Sprintf("%s %s (%s)\n", applyColorToken(color, colorTokenAccent, ProgramName), applyColorToken(color, colorTokenSuccess, c.Version), applyColorToken(color, colorTokenMuted, c.Commit))))
}
