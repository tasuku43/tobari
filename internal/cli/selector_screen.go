package cli

import (
	"fmt"
	"io"
	"strings"
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

func renderPolicyReviewScreen(out io.Writer, lines []string, previousLines int) int {
	lineCount, err := renderSelectorScreen(out, lines, previousLines)
	if err != nil {
		return -1
	}
	return lineCount
}

func finishPolicyReviewSelector(out io.Writer, lines int) {
	finishSelectorScreen(out, lines)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

const selectorDetailLabelWidth = 9

func selectorTitle(enabled bool, value string) string {
	return applyStyleToken(enabled, styleAccent, value)
}

func selectorHelp(enabled bool, value string) string {
	return applyStyleToken(enabled, styleMuted, value)
}

func selectorDetail(enabled bool, label, value string, token styleToken) string {
	return applyStyleToken(enabled, styleMuted, fmt.Sprintf("%-*s", selectorDetailLabelWidth, label)) +
		" " + applyStyleToken(enabled, token, value)
}

func styleAction(enabled bool, value string, token styleToken) string {
	return applyStyleToken(enabled, token, value)
}

func selectorActions(actions ...string) string {
	return strings.Join(actions, "   ")
}
