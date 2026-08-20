package dockerruntime

import (
	"io"
	"strings"

	"github.com/tasuku43/tobari/internal/infra/terminal"
	"github.com/tasuku43/tobari/internal/infra/terminalstyle"
)

func structuredOutputColorEnabled(in io.Reader, out io.Writer, shellEnvironment []string) bool {
	if !terminal.IsTerminal(in) || !terminal.IsTerminal(out) || terminalstyle.NoColorRequested() {
		return false
	}
	for _, value := range shellEnvironment {
		if strings.HasPrefix(value, "NO_COLOR=") {
			return false
		}
	}
	return true
}
