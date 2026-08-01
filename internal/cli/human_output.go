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
// caller chooses the marker, label, value, and semantic color token.
type humanOutput struct {
	bytes.Buffer
	color bool
}

const humanOutputLabelWidth = 14

func newHumanOutput(color bool) *humanOutput {
	return &humanOutput{color: color}
}

func (o *humanOutput) heading(marker, title string, token colorToken) {
	fmt.Fprintf(&o.Buffer, "%s %s\n", applyColorToken(o.color, token, marker), applyColorToken(o.color, colorTokenAccent, title))
}

func (o *humanOutput) section(title string) {
	if o.Len() > 0 && !strings.HasSuffix(o.String(), "\n\n") {
		o.WriteByte('\n')
	}
	fmt.Fprintln(&o.Buffer, applyColorToken(o.color, colorTokenAccent, title))
}

// row aligns the label before applying color so ANSI escape bytes never affect
// the visible column layout.
func (o *humanOutput) row(label, value string, token colorToken) {
	padded := fmt.Sprintf("%-*s", humanOutputLabelWidth, label)
	fmt.Fprintf(&o.Buffer, "  %s %s\n", applyColorToken(o.color, colorTokenMuted, padded), applyColorToken(o.color, token, value))
}

func (o *humanOutput) text(value string) {
	fmt.Fprintln(&o.Buffer, value)
}

func (o *humanOutput) next(command, reason string) {
	o.row("Next", recoveryCommand(command)+" — "+escapeTSVCell(reason), colorTokenAccent)
}

func (o *humanOutput) empty(title, detail, command, reason string) {
	o.heading("○", title, colorTokenMuted)
	if detail != "" {
		o.row("Details", detail, colorTokenMuted)
	}
	if command != "" {
		o.next(command, reason)
	}
}

func (o *humanOutput) bytes() []byte {
	return append([]byte(nil), o.Buffer.Bytes()...)
}

// humanColorAllowed centralizes the interactive/color policy for human
// output. Machine error output never receives ANSI. Terminal ownership stays
// behind the existing runtime port, so the CLI does not perform filesystem or
// process inspection just to decide how to render.
func humanColorAllowed(ctx context.Context, c *CLI, writer io.Writer) bool {
	if c == nil || writer == nil || invocationErrorFormat(ctx) == errorFormatJSON {
		return false
	}
	return c.tobari != nil && c.tobari.IsTerminal(writer)
}

func humanStatusToken(status string) colorToken {
	switch strings.ToLower(status) {
	case "pass", "ok", "ready", "running", "healthy", "true", "applied", "created", "deleted", "detached":
		return colorTokenSuccess
	case "warn", "warning", "starting", "pending", "unknown", "missing", "degraded", "unreachable", "incomplete", "false":
		return colorTokenWarning
	case "fail", "failed", "error", "unhealthy", "exited", "dead", "rejected":
		return colorTokenError
	default:
		return colorTokenMuted
	}
}

func humanBoolToken(value bool) colorToken {
	if value {
		return colorTokenSuccess
	}
	return colorTokenMuted
}

func humanBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
