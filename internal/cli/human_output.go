package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
)

// humanOutput is the small presentation vocabulary shared by human-facing
// command output. It deliberately knows nothing about command semantics: the
// caller chooses the marker, label, value, and semantic style token.
type humanOutput struct {
	bytes.Buffer
	color bool
}

const humanOutputLabelWidth = 14

func newHumanOutput(color bool) *humanOutput {
	return &humanOutput{color: color}
}

func (o *humanOutput) heading(marker, title string, token styleToken) {
	fmt.Fprintf(&o.Buffer, "%s %s\n", applyStyleToken(o.color, token, marker), applyStyleToken(o.color, styleText, title))
}

func (o *humanOutput) section(title string) {
	o.sectionWithToken(title, styleAccent)
}

func (o *humanOutput) sectionWithToken(title string, token styleToken) {
	if o.Len() > 0 && !strings.HasSuffix(o.String(), "\n\n") {
		_ = o.WriteByte('\n') // #nosec G104 -- bytes.Buffer.WriteByte always returns nil.
	}
	fmt.Fprintln(&o.Buffer, applyStyleToken(o.color, token, title))
}

// row aligns the label before applying color so ANSI escape bytes never affect
// the visible column layout.
func (o *humanOutput) row(label, value string, token styleToken) {
	padded := fmt.Sprintf("%-*s", humanOutputLabelWidth, label)
	fmt.Fprintf(&o.Buffer, "  %s %s\n", applyStyleToken(o.color, styleMuted, padded), applyStyleToken(o.color, token, value))
}

func (o *humanOutput) text(value string) {
	fmt.Fprintln(&o.Buffer, applyStyleToken(o.color, styleText, value))
}

func (o *humanOutput) next(command, reason string) {
	padded := fmt.Sprintf("%-*s", humanOutputLabelWidth, "Next")
	fmt.Fprintf(
		&o.Buffer, "  %s %s %s\n",
		applyStyleToken(o.color, styleMuted, padded),
		applyStyleToken(o.color, styleAccent, recoveryCommand(command)),
		applyStyleToken(o.color, styleText, "— "+escapeTSVCell(reason)),
	)
}

// nextStep keeps the operation label readable while making only the command
// or other explicitly selected next value carry the operation emphasis.
func (o *humanOutput) nextStep(number int, description, value string, token styleToken) {
	fmt.Fprintf(&o.Buffer, "  %d. %s\n", number, applyStyleToken(o.color, styleText, description))
	fmt.Fprintf(&o.Buffer, "     %s\n\n", applyStyleToken(o.color, token, value))
}

func writeStyledCommandLine(output io.Writer, enabled bool, label, before, command, after string) {
	fmt.Fprintf(
		output, "%s %s%s%s\n",
		applyStyleToken(enabled, styleMuted, label),
		applyStyleToken(enabled, styleText, before),
		applyStyleToken(enabled, styleAccent, command),
		applyStyleToken(enabled, styleText, after),
	)
}

func (o *humanOutput) empty(title, detail, command, reason string) {
	o.heading("○", title, styleMuted)
	if detail != "" {
		o.row("Details", detail, styleText)
	}
	if command != "" {
		o.next(command, reason)
	}
}

func (o *humanOutput) bytes() []byte {
	return append([]byte(nil), o.Buffer.Bytes()...)
}

func semanticTextBytes(enabled bool, data []byte) []byte {
	return []byte(applyStyleToken(enabled, styleText, string(data)))
}

// humanStyleAllowed centralizes the interactive/color policy for human
// output. Machine error output never receives ANSI. Terminal ownership stays
// behind the existing runtime port, so the CLI does not perform filesystem or
// process inspection just to decide how to render.
func humanStyleAllowed(ctx context.Context, c *CLI, writer io.Writer) bool {
	if c == nil || writer == nil || c.noColor || invocationErrorFormat(ctx) == errorFormatJSON {
		return false
	}
	return c.tobari != nil && c.tobari.IsTerminal(writer)
}

func humanStatusToken(status string) styleToken {
	switch strings.ToLower(status) {
	case "pass", "ok", "ready", "running", "healthy", "true", "applied", "created", "deleted", "detached":
		return styleSuccess
	case "warn", "warning", "starting", "pending", "pending_build", "unknown", "missing", "degraded", "unreachable", "incomplete", "false":
		return styleWarning
	case "fail", "failed", "error", "unhealthy", "exited", "dead", "rejected":
		return styleDanger
	default:
		return styleMuted
	}
}

func humanOutcomeBoolToken(value bool) styleToken {
	if value {
		return styleSuccess
	}
	return styleWarning
}

func humanBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
