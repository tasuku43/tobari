package cli

import (
	"fmt"
	"io"
)

const (
	selectorAlternateScreenEnter = "\x1b[?1049h"
	selectorAlternateScreenExit  = "\x1b[?1049l"
	selectorCursorHome           = "\x1b[H"
	selectorEraseDisplay         = "\x1b[2J"
	selectorCursorHide           = "\x1b[?25l"
	selectorCursorShow           = "\x1b[?25h"
)

// renderSelectorScreen redraws inside the terminal's alternate screen rather
// than moving upward by a logical line count. A terminal may wrap one logical
// row into multiple physical rows, so line-count cursor movement cannot locate
// the prior frame reliably.
func renderSelectorScreen(out io.Writer, lines []string, previousLines int) (int, error) {
	transition := selectorCursorHome + selectorEraseDisplay
	if previousLines <= 0 {
		transition = selectorAlternateScreenEnter + selectorCursorHide + transition
	}
	if _, err := io.WriteString(out, transition); err != nil {
		return 0, err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(out, "\x1b[2K\r%s\n", line); err != nil {
			return 0, err
		}
	}
	return len(lines), nil
}

func finishSelectorScreen(out io.Writer, _ int) {
	_, _ = io.WriteString(out, selectorAlternateScreenExit+selectorCursorShow)
}
