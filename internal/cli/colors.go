package cli

// colorToken is a semantic presentation role rather than a raw ANSI color.
// Keeping roles here lets future CLI surfaces share the same visual language
// without coupling their meaning to a terminal escape sequence.
type colorToken string

const (
	colorTokenMuted    colorToken = "muted"
	colorTokenAccent   colorToken = "accent"
	colorTokenSelected colorToken = "selected"
	colorTokenSuccess  colorToken = "success"
	colorTokenWarning  colorToken = "warning"
	colorTokenError    colorToken = "error"
)

const ansiReset = "\x1b[0m"

var ansiColorTokens = map[colorToken]string{
	colorTokenMuted:    "\x1b[2m",
	colorTokenAccent:   "\x1b[36m",
	colorTokenSelected: "\x1b[96m",
	colorTokenSuccess:  "\x1b[32m",
	colorTokenWarning:  "\x1b[33m",
	colorTokenError:    "\x1b[31m",
}

func applyColorToken(enabled bool, token colorToken, value string) string {
	if !enabled || value == "" {
		return value
	}
	code, ok := ansiColorTokens[token]
	if !ok {
		return value
	}
	return code + value + ansiReset
}
