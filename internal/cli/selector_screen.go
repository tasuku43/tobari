package cli

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const (
	selectorCursorSave     = "\x1b[s"
	selectorCursorRestore  = "\x1b[u"
	selectorEraseBelow     = "\x1b[0J"
	selectorCursorHide     = "\x1b[?25l"
	selectorCursorShow     = "\x1b[?25h"
	selectorCursorUp       = "\x1b[%dA"
	selectorStateFrameMask = 1<<10 - 1
	selectorStateRowsMask  = 1<<8 - 1
	selectorStateColsMask  = 1<<11 - 1
	selectorStateRowsShift = 10
	selectorStateColsShift = 18
	selectorStateAppend    = 1 << 29
	selectorStateKnown     = 1 << 30
)

type selectorTerminalSizeWriter interface {
	SelectorTerminalSize() (rows, columns int)
}

// renderSelectorScreen keeps Tobari-owned interaction inline in the main
// terminal history. Before saving an origin it reserves the frame's physical
// rows, including terminal wrapping. Any scroll therefore happens before the
// origin is saved; redraw cannot restore a cursor position that has already
// moved into scrollback. A frame taller than the viewport degrades to complete
// append-only frames instead of corrupting the visible screen.
func renderSelectorScreen(out io.Writer, lines []string, previousLines int) (int, error) {
	rows, columns, sizeKnown := selectorTerminalSize(out)
	frameRows := selectorLogicalRows(lines)
	if sizeKnown {
		frameRows = selectorFrameRows(lines, columns)
	}
	stable := sizeKnown && frameRows < rows && frameRows <= selectorStateFrameMask
	previousRows, previousColumns, previousKnown, previousAppend := decodeSelectorState(previousLines)
	sameViewport := stable && previousKnown && !previousAppend && previousRows == rows && previousColumns == columns
	transition := selectorCursorHide
	if previousLines > 0 && sameViewport {
		transition += selectorCursorRestore + "\r" + selectorEraseBelow
	} else if previousLines > 0 {
		transition += "\r\n"
	} else {
		transition += "\r"
	}
	if stable {
		transition += strings.Repeat("\r\n", frameRows) + fmt.Sprintf(selectorCursorUp, frameRows) + selectorCursorSave + selectorEraseBelow
	} else if previousLines == 0 {
		transition += selectorEraseBelow
	}
	if _, err := io.WriteString(out, transition); err != nil {
		return 0, err
	}
	for _, line := range lines {
		for _, segment := range strings.Split(line, "\n") {
			if _, err := io.WriteString(out, "\x1b[2K\r"+segment+"\r\n"); err != nil {
				return 0, err
			}
		}
	}
	if !stable {
		return encodeSelectorState(frameRows, rows, columns, sizeKnown, true), nil
	}
	return encodeSelectorState(frameRows, rows, columns, true, false), nil
}

func finishSelectorScreen(out io.Writer, _ int) error {
	_, err := io.WriteString(out, selectorCursorShow+"\r")
	return err
}

func renderPolicyReviewScreen(out io.Writer, lines []string, previousLines int) int {
	lineCount, err := renderSelectorScreen(out, lines, previousLines)
	if err != nil {
		return -1
	}
	return lineCount
}

func finishPolicyReviewSelector(out io.Writer, lines int) error {
	return finishSelectorScreen(out, lines)
}

func selectorTerminalSize(out io.Writer) (rows, columns int, known bool) {
	if sized, ok := out.(selectorTerminalSizeWriter); ok {
		rows, columns = sized.SelectorTerminalSize()
	} else if detectedRows, detectedColumns, err := terminal.Size(out); err == nil {
		rows, columns = detectedRows, detectedColumns
	}
	known = rows >= 2 && rows <= selectorStateRowsMask && columns >= 1 && columns <= selectorStateColsMask
	return rows, columns, known
}

func selectorFrameRows(lines []string, columns int) int {
	rows := 0
	for _, line := range lines {
		for _, segment := range strings.Split(line, "\n") {
			cells := selectorDisplayCells(segment)
			physical := (cells + columns - 1) / columns
			if physical < 1 {
				physical = 1
			}
			rows += physical
		}
	}
	if rows < 1 {
		return 1
	}
	return rows
}

func selectorLogicalRows(lines []string) int {
	rows := 0
	for _, line := range lines {
		rows += len(strings.Split(line, "\n"))
	}
	if rows < 1 {
		return 1
	}
	return rows
}

func selectorDisplayCells(value string) int {
	cells := 0
	for index := 0; index < len(value); {
		if value[index] == '\x1b' && index+1 < len(value) && value[index+1] == '[' {
			index += 2
			for index < len(value) {
				last := value[index]
				index++
				if last >= 0x40 && last <= 0x7e {
					break
				}
			}
			continue
		}
		r, size := rune(value[index]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(value[index:])
		}
		index += size
		switch {
		case r == '\t':
			cells += 8 - cells%8
		case r < 0x20 || r == 0x7f:
		case r < utf8.RuneSelf:
			cells++
		default:
			// Every non-ASCII code point consumes a conservative two-cell
			// budget. Combining marks, variation selectors, ZWJ sequences,
			// emoji presentation, and East Asian Ambiguous characters normally
			// consume fewer cells; over-reservation is safe and under-reservation
			// could scroll a saved origin out of the viewport.
			cells += 2
		}
	}
	return cells
}

func encodeSelectorState(frameRows, rows, columns int, known, appendOnly bool) int {
	if frameRows < 1 {
		frameRows = 1
	}
	if frameRows > selectorStateFrameMask {
		frameRows = selectorStateFrameMask
	}
	state := frameRows
	if appendOnly {
		state |= selectorStateAppend
	}
	if known && rows <= selectorStateRowsMask && columns <= selectorStateColsMask {
		state |= selectorStateKnown
		state |= rows << selectorStateRowsShift
		state |= columns << selectorStateColsShift
	}
	return state
}

func decodeSelectorState(state int) (rows, columns int, known, appendOnly bool) {
	if state <= 0 {
		return 0, 0, false, false
	}
	known = state&selectorStateKnown != 0
	appendOnly = state&selectorStateAppend != 0
	rows = state >> selectorStateRowsShift & selectorStateRowsMask
	columns = state >> selectorStateColsShift & selectorStateColsMask
	return rows, columns, known, appendOnly
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
