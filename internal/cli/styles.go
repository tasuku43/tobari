package cli

// styleToken is a semantic presentation role rather than a concrete color or
// emphasis. Human renderers select meaning; this shared layer alone decides
// how that meaning appears on a terminal.
type styleToken string

const (
	styleText    styleToken = "text"
	styleMuted   styleToken = "muted"
	styleAccent  styleToken = "accent"
	styleSuccess styleToken = "success"
	styleWarning styleToken = "warning"
	styleDanger  styleToken = "danger"
)

var semanticStyleTokens = []styleToken{
	styleText,
	styleMuted,
	styleAccent,
	styleSuccess,
	styleWarning,
	styleDanger,
}

const ansiStyleReset = "\x1b[0m"

// ansiStyleTokens is deliberately private to the shared presentation layer.
// text uses the terminal default. muted stays secondary without the dim/faint
// attribute, which is difficult to read on common terminal backgrounds.
var ansiStyleTokens = map[styleToken]string{
	styleText:    "",
	styleMuted:   "\x1b[38;5;250m",
	styleAccent:  "\x1b[1;38;5;45m",
	styleSuccess: "\x1b[38;5;42m",
	styleWarning: "\x1b[38;5;214m",
	styleDanger:  "\x1b[38;5;203m",
}

func applyStyleToken(enabled bool, token styleToken, value string) string {
	if !enabled || value == "" {
		return value
	}
	code, ok := ansiStyleTokens[token]
	if !ok || code == "" {
		return value
	}
	return code + value + ansiStyleReset
}
