package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectorScreenRedrawIsIndependentFromWrappedPhysicalRows(t *testing.T) {
	t.Parallel()
	long := "GET https://example.com/" + strings.Repeat("wrapped-segment/", 20)
	lines := []string{"Tobari · Permission Inbox", long, "Selected"}

	var initial bytes.Buffer
	count, err := renderSelectorScreen(&initial, lines, 0)
	if err != nil || count != len(lines) {
		t.Fatalf("initial selector render count=%d error=%v", count, err)
	}
	initialPrefix := selectorAlternateScreenEnter + selectorCursorHide + selectorCursorHome + selectorEraseDisplay
	if !strings.HasPrefix(initial.String(), initialPrefix) {
		t.Fatalf("initial selector transition = %q", initial.String())
	}

	var redraw bytes.Buffer
	count, err = renderSelectorScreen(&redraw, lines, 3)
	if err != nil || count != len(lines) {
		t.Fatalf("selector redraw count=%d error=%v", count, err)
	}
	if !strings.HasPrefix(redraw.String(), selectorCursorHome+selectorEraseDisplay) ||
		strings.Contains(redraw.String(), selectorAlternateScreenEnter) ||
		strings.Contains(redraw.String(), "\x1b[3A") {
		t.Fatalf("selector redraw depends on logical rows: %q", redraw.String())
	}
	if strings.Count(redraw.String(), "Tobari · Permission Inbox") != 1 ||
		!strings.Contains(redraw.String(), long) {
		t.Fatalf("selector redraw frame = %q", redraw.String())
	}
}

func TestSelectorScreenFinishAlwaysRestoresMainScreenAndCursor(t *testing.T) {
	t.Parallel()
	for _, previousLines := range []int{0, 1, 40} {
		var output bytes.Buffer
		finishSelectorScreen(&output, previousLines)
		if output.String() != selectorAlternateScreenExit+selectorCursorShow {
			t.Fatalf("finish(%d) = %q", previousLines, output.String())
		}
	}
}
