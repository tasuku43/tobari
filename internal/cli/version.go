package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/operation"
)

func runVersion(ctx context.Context, c *CLI, _ CommandSpec, _ operation.Intent, _ ParsedInputs) int {
	style := humanStyleAllowed(ctx, c, c.Out)
	if style {
		output := newHumanOutput(true)
		output.heading("•", "Tobari", styleAccent)
		output.row("Version", c.Version, styleText)
		if c.Commit != "" {
			output.row("Commit", c.Commit, styleMuted)
		}
		return c.emitResult(ctx, output.bytes())
	}
	if c.Commit == "" {
		return c.emitResult(ctx, []byte(fmt.Sprintf(
			"%s %s\n",
			applyStyleToken(style, styleText, ProgramName),
			applyStyleToken(style, styleText, c.Version),
		)))
	}
	return c.emitResult(ctx, []byte(fmt.Sprintf(
		"%s %s (%s)\n",
		applyStyleToken(style, styleText, ProgramName),
		applyStyleToken(style, styleText, c.Version),
		applyStyleToken(style, styleMuted, c.Commit),
	)))
}
