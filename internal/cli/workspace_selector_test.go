package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type selectorModeFake struct {
	enterErr error
	restored int
	entered  int
}

func (f *selectorModeFake) Enter(io.Reader) (func() error, error) {
	f.entered++
	if f.enterErr != nil {
		return nil, f.enterErr
	}
	return func() error {
		f.restored++
		return nil
	}, nil
}

func testWorkspaceSelection() tobari.WorkspaceSelection {
	return tobari.WorkspaceSelection{
		CWD: "/work/root/app", CanCreate: true,
		Candidates: []tobari.WorkspaceSelectionCandidate{
			{ID: "018bcfe5-687b-7000-8000-000000000000", Root: "/work/root", WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000099", WorkspaceManifestName: "default", Runtime: tobari.RuntimeDiagnosticReady},
			{ID: "018bcfe5-687b-7000-8000-000000000001", Root: "/work", WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000099", WorkspaceManifestName: "default", Runtime: tobari.RuntimeDiagnosticDegraded},
		},
	}
}

func TestWorkspaceSelectorUsesArrowKeysAndRestoresRawMode(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &workspaceSelector{mode: mode, style: true}
	var output bytes.Buffer
	choice, err := selector.Select(
		context.Background(), testWorkspaceSelection(),
		strings.NewReader("\x1b[B\r"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if choice.Kind != tobari.ProjectSelectionUse || choice.ID != "018bcfe5-687b-7000-8000-000000000001" {
		t.Fatalf("choice = %+v", choice)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode calls = entered:%d restored:%d", mode.entered, mode.restored)
	}
	value := output.String()
	for _, want := range []string{"Select a Workspace for /work/root/app", "nearest ancestor", "degraded", "Using existing Workspace", "Working directory /work/root/app"} {
		if !strings.Contains(value, want) {
			t.Fatalf("selector output %q lacks %q", value, want)
		}
	}
	if !strings.Contains(value, "\x1b[") {
		t.Fatalf("rich selector output lacks terminal control sequences: %q", value)
	}
}

func TestWorkspaceSelectorFallsBackToEnglishLineInput(t *testing.T) {
	t.Parallel()
	selector := &workspaceSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	choice, err := selector.Select(
		context.Background(), testWorkspaceSelection(), strings.NewReader("n\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if choice.Kind != tobari.ProjectSelectionCreate || choice.ID != "" {
		t.Fatalf("choice = %+v", choice)
	}
	value := output.String()
	for _, want := range []string{"Select a Workspace for /work/root/app", "1.", "2.", "Create a new Workspace here", "Creating a new Workspace here"} {
		if !strings.Contains(value, want) {
			t.Fatalf("line selector output %q lacks %q", value, want)
		}
	}
	if strings.Contains(value, "\x1b[") {
		t.Fatalf("line selector output unexpectedly contains terminal control sequences: %q", value)
	}
}

func TestWorkspaceSelectorCancelReturnsWithoutSummary(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &workspaceSelector{mode: mode, style: true}
	var output bytes.Buffer
	_, err := selector.Select(context.Background(), testWorkspaceSelection(), strings.NewReader("q"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Select() error = %v, want context.Canceled", err)
	}
	if strings.Contains(output.String(), "Using existing Workspace") || strings.Contains(output.String(), "Creating a new Workspace") {
		t.Fatalf("cancel output contains mutation summary: %q", output.String())
	}
	if mode.restored != 1 {
		t.Fatalf("cancel did not restore raw mode: %d", mode.restored)
	}
}

func TestWorkspaceSelectorDoesNotSelectIncompleteCandidate(t *testing.T) {
	t.Parallel()
	selection := testWorkspaceSelection()
	selection.Candidates[0].Runtime = tobari.RuntimeDiagnosticIncomplete
	selector := &workspaceSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	choice, err := selector.Select(context.Background(), selection, strings.NewReader("1\n2\n"), &output)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if choice.Kind != tobari.ProjectSelectionUse || choice.ID != selection.Candidates[1].ID {
		t.Fatalf("choice = %+v, want second candidate", choice)
	}
	if !strings.Contains(output.String(), "unavailable") {
		t.Fatalf("output lacks unavailable state: %q", output.String())
	}
}

func TestTruncateSelectorPathKeepsBothEnds(t *testing.T) {
	t.Parallel()
	value := truncateSelectorPath("/a/very/long/project/path/that/should/be/shortened", 24)
	if len([]rune(value)) > 24 || !strings.Contains(value, "…") || !strings.HasPrefix(value, "/a/") || !strings.HasSuffix(value, "ened") {
		t.Fatalf("truncated path = %q", value)
	}
}

func TestWorkspaceSelectorScrollsLongCandidateLists(t *testing.T) {
	t.Parallel()
	selection := tobari.WorkspaceSelection{CWD: "/work/a/b/c/d/e/f/g/app", CanCreate: true}
	roots := []string{
		"/work/a/b/c/d/e/f/g", "/work/a/b/c/d/e/f", "/work/a/b/c/d/e",
		"/work/a/b/c/d", "/work/a/b/c", "/work/a/b", "/work/a", "/work",
	}
	for index, root := range roots {
		selection.Candidates = append(selection.Candidates, tobari.WorkspaceSelectionCandidate{
			ID: fmt.Sprintf("018bcfe5-687b-7000-8000-%012x", index), Root: root,
			WorkspaceManifestID: "018bcfe5-687b-7000-8000-000000000099", WorkspaceManifestName: "default",
			Runtime: tobari.RuntimeDiagnosticReady,
		})
	}
	selector := &workspaceSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	_, err := selector.Select(context.Background(), selection, strings.NewReader("\x1b[F\r"), &output)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !strings.Contains(output.String(), "Showing 4-9 of 9 options") {
		t.Fatalf("bounded-window status missing: %q", output.String())
	}
}

func TestWorkspaceSelectorProjectsControlCharactersInPaths(t *testing.T) {
	t.Parallel()
	selection := testWorkspaceSelection()
	selection.CWD = "/work/root/app\nnext"
	selector := &workspaceSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	_, err := selector.Select(context.Background(), selection, strings.NewReader("q\n"), &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Select() error = %v, want context.Canceled", err)
	}
	if strings.Contains(output.String(), "app\nnext") {
		t.Fatalf("selector output contains a raw line break from external path: %q", output.String())
	}
	if !strings.Contains(output.String(), `app\nnext`) {
		t.Fatalf("selector output lacks projected newline: %q", output.String())
	}
}
