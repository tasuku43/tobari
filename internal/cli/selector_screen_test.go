package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

type sizedSelectorBuffer struct {
	bytes.Buffer
	rows    int
	columns int
}

func (b *sizedSelectorBuffer) SelectorTerminalSize() (int, int) { return b.rows, b.columns }

func TestSelectorScreenRedrawIsInlineAndIndependentFromWrappedPhysicalRows(t *testing.T) {
	t.Parallel()
	long := "GET https://example.com/" + strings.Repeat("wrapped-segment/", 20)
	lines := []string{"Tobari · Permission Inbox", long, "Selected"}

	initial := &sizedSelectorBuffer{rows: 24, columns: 40}
	state, err := renderSelectorScreen(initial, lines, 0)
	wantRows := selectorFrameRows(lines, initial.columns)
	stateRows, stateColumns, known, appendOnly := decodeSelectorState(state)
	if err != nil || stateRows != initial.rows || stateColumns != initial.columns || !known || appendOnly {
		t.Fatalf("initial selector state=%d (%d×%d known=%t append=%t) error=%v", state, stateRows, stateColumns, known, appendOnly, err)
	}
	initialPrefix := selectorCursorHide + "\r" + strings.Repeat("\r\n", wantRows) + fmt.Sprintf(selectorCursorUp, wantRows) + selectorCursorSave + selectorEraseBelow
	if !strings.HasPrefix(initial.String(), initialPrefix) {
		t.Fatalf("initial selector transition = %q", initial.String())
	}
	if strings.Contains(initial.String(), "\x1b[?1049") {
		t.Fatalf("inline selector entered the terminal alternate screen: %q", initial.String())
	}

	redraw := &sizedSelectorBuffer{rows: 24, columns: 40}
	state, err = renderSelectorScreen(redraw, lines, state)
	if err != nil {
		t.Fatalf("selector redraw state=%d error=%v", state, err)
	}
	redrawPrefix := selectorCursorHide + selectorCursorRestore + "\r" + selectorEraseBelow +
		strings.Repeat("\r\n", wantRows) + fmt.Sprintf(selectorCursorUp, wantRows) + selectorCursorSave + selectorEraseBelow
	if !strings.HasPrefix(redraw.String(), redrawPrefix) {
		t.Fatalf("selector redraw did not reserve wrapped physical rows: %q", redraw.String())
	}
	if strings.Contains(redraw.String(), "\x1b[?1049") {
		t.Fatalf("inline selector redraw entered the terminal alternate screen: %q", redraw.String())
	}
	if strings.Count(redraw.String(), "Tobari · Permission Inbox") != 1 ||
		!strings.Contains(redraw.String(), long) {
		t.Fatalf("selector redraw frame = %q", redraw.String())
	}
}

func TestSelectorScreenFinishLeavesInlineFrameAndRestoresCursor(t *testing.T) {
	t.Parallel()
	for _, previousLines := range []int{0, 1, 40} {
		var output bytes.Buffer
		if err := finishSelectorScreen(&output, previousLines); err != nil {
			t.Fatal(err)
		}
		if output.String() != selectorCursorShow+"\r" {
			t.Fatalf("finish(%d) = %q", previousLines, output.String())
		}
	}
}

func TestSelectorScreenReservesBeforeSavingAtViewportBottom(t *testing.T) {
	t.Parallel()
	model := newSelectorScreenModel(8, 24)
	out := &selectorModelWriter{model: model, rows: 8, columns: 24}
	lines := []string{"Tobari selector", "one line that wraps here", "Selected"}
	model.row = model.rows - 1

	state, err := renderSelectorScreen(out, lines, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := model.visible()
	if strings.Count(first, "Tobari selector") != 1 || strings.Count(first, "Selected") != 1 {
		t.Fatalf("initial screen state = %q", first)
	}
	if _, err := renderSelectorScreen(out, lines, state); err != nil {
		t.Fatal(err)
	}
	redrawn := model.visible()
	if strings.Count(redrawn, "Tobari selector") != 1 || strings.Count(redrawn, "Selected") != 1 {
		t.Fatalf("redrawn screen duplicated a scrolled frame = %q", redrawn)
	}
}

func TestSelectorScreenResizeDoesNotRestoreTheOldViewportOrigin(t *testing.T) {
	t.Parallel()
	model := newSelectorScreenModel(10, 30)
	out := &selectorModelWriter{model: model, rows: 10, columns: 30}
	lines := []string{"Tobari selector", "one line that wraps after resize", "Selected"}
	state, err := renderSelectorScreen(out, lines, 0)
	if err != nil {
		t.Fatal(err)
	}
	out.Buffer.Reset()
	out.rows, out.columns = 7, 18
	if _, err := renderSelectorScreen(out, lines, state); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), selectorCursorRestore) || !strings.Contains(out.String(), selectorCursorSave) {
		t.Fatalf("resize reused an old origin instead of appending and re-anchoring: %q", out.String())
	}
}

func TestSelectorScreenUnknownSizeStaysAppendOnly(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	state, err := renderSelectorScreen(&out, []string{"Tobari · Unknown size", "Selected"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _, known, appendOnly := decodeSelectorState(state)
	if known || !appendOnly || strings.Contains(out.String(), selectorCursorSave) {
		t.Fatalf("unknown terminal size guessed a restorable viewport: state=%d output=%q", state, out.String())
	}
	out.Reset()
	if _, err := renderSelectorScreen(&out, []string{"Tobari · Unknown size", "Selected"}, state); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), selectorCursorRestore) {
		t.Fatalf("unknown terminal size restored an unproven origin: %q", out.String())
	}
}

func TestSelectorDisplayWidthConservativelyBudgetsUnicodeClusters(t *testing.T) {
	t.Parallel()
	for value, minimum := range map[string]int{"·": 2, "⚠️": 4, "1️⃣": 5, "👩‍💻": 6} {
		if got := selectorDisplayCells(value); got < minimum {
			t.Fatalf("display cells for %q = %d, want at least %d", value, got, minimum)
		}
	}
}

func TestSelectorFinishFailureIsNotASelectionSuccess(t *testing.T) {
	t.Parallel()
	out := &selectorFailingWriter{failOn: selectorCursorShow}
	_, err := selectConfigurationWizardRaw(context.Background(), strings.NewReader("\r"), out, configurationWizardMenu{
		title: "Choose", prompt: "Next", options: []configurationWizardOption{{label: "Apply"}},
	}, false)
	if err == nil || !strings.Contains(err.Error(), "selector output failed") {
		t.Fatalf("selection error = %v", err)
	}
}

func TestSelectorRenderFailureHasOneCleanupOwner(t *testing.T) {
	t.Parallel()
	out := &selectorFailingWriter{failOn: "Choose"}
	_, err := selectConfigurationWizardRaw(context.Background(), strings.NewReader("\r"), out, configurationWizardMenu{
		title: "Choose", prompt: "Next", options: []configurationWizardOption{{label: "Apply"}},
	}, false)
	if err == nil {
		t.Fatal("render failure unexpectedly selected an action")
	}
	if got := strings.Count(out.String(), selectorCursorShow); got != 1 {
		t.Fatalf("render failure cursor cleanup count=%d output=%q", got, out.String())
	}
}

type selectorFailingWriter struct {
	bytes.Buffer
	failOn string
}

func (w *selectorFailingWriter) Write(value []byte) (int, error) {
	if strings.Contains(string(value), w.failOn) {
		return 0, errors.New("selector output failed")
	}
	return w.Buffer.Write(value)
}

func (w *selectorFailingWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

type selectorModelWriter struct {
	bytes.Buffer
	model         *selectorScreenModel
	rows, columns int
}

func (w *selectorModelWriter) SelectorTerminalSize() (int, int) { return w.rows, w.columns }
func (w *selectorModelWriter) Write(value []byte) (int, error) {
	_, _ = w.Buffer.Write(value)
	w.model.write(string(value))
	return len(value), nil
}

func (w *selectorModelWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

type selectorScreenModel struct {
	rows, columns int
	row, column   int
	savedRow      int
	cells         [][]rune
}

func newSelectorScreenModel(rows, columns int) *selectorScreenModel {
	m := &selectorScreenModel{rows: rows, columns: columns, cells: make([][]rune, rows)}
	for index := range m.cells {
		m.cells[index] = make([]rune, columns)
	}
	return m
}

func (m *selectorScreenModel) write(value string) {
	for index := 0; index < len(value); {
		if value[index] == '\x1b' && index+1 < len(value) && value[index+1] == '[' {
			end := index + 2
			for end < len(value) && (value[end] < 0x40 || value[end] > 0x7e) {
				end++
			}
			if end >= len(value) {
				return
			}
			m.control(value[index+2:end], value[end])
			index = end + 1
			continue
		}
		switch value[index] {
		case '\r':
			m.column = 0
		case '\n':
			m.nextRow()
		default:
			if m.column >= m.columns {
				m.column = 0
				m.nextRow()
			}
			m.cells[m.row][m.column] = rune(value[index])
			m.column++
		}
		index++
	}
}

func (m *selectorScreenModel) control(parameters string, final byte) {
	switch final {
	case 's':
		m.savedRow = m.row
	case 'u':
		m.row = m.savedRow
		m.column = 0
	case 'A':
		count, _ := strconv.Atoi(parameters)
		m.row -= count
		if m.row < 0 {
			m.row = 0
		}
	case 'J':
		for row := m.row; row < m.rows; row++ {
			start := 0
			if row == m.row {
				start = m.column
			}
			for column := start; column < m.columns; column++ {
				m.cells[row][column] = 0
			}
		}
	case 'K':
		for column := range m.cells[m.row] {
			m.cells[m.row][column] = 0
		}
	}
}

func (m *selectorScreenModel) nextRow() {
	m.row++
	if m.row < m.rows {
		return
	}
	copy(m.cells, m.cells[1:])
	m.cells[m.rows-1] = make([]rune, m.columns)
	m.row = m.rows - 1
}

func (m *selectorScreenModel) visible() string {
	var lines []string
	for _, row := range m.cells {
		lines = append(lines, strings.TrimRight(string(row), "\x00 "))
	}
	return strings.Join(lines, "\n")
}
