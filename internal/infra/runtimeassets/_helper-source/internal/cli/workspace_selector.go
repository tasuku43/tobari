package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const (
	selectorMaxVisibleOptions = 6
	selectorPathWidth         = 72
)

var errSelectorTimeout = errors.New("selector input timeout")
var errSelectorEOF = errors.New("selector input reached EOF")

type workspaceSelector struct {
	mode  terminal.Mode
	style bool
}

func newWorkspaceSelector() *workspaceSelector {
	return newWorkspaceSelectorWithStyle(true)
}

func newWorkspaceSelectorWithStyle(enabled bool) *workspaceSelector {
	return &workspaceSelector{mode: terminal.New(), style: enabled}
}

func (s *workspaceSelector) Select(
	ctx context.Context, selection tobari.WorkspaceSelection, in io.Reader, out io.Writer,
) (tobari.ProjectSelectionChoice, error) {
	if err := selection.Validate(); err != nil {
		return tobari.ProjectSelectionChoice{}, err
	}
	if !selection.RequiresChoice() {
		if len(selection.Candidates) == 0 {
			return tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}, nil
		}
		return tobari.ProjectSelectionChoice{
			Kind: tobari.ProjectSelectionUse, ID: selection.Candidates[0].ID,
		}, nil
	}

	if s != nil && s.mode != nil {
		restore, rawErr := s.mode.Enter(in)
		if rawErr == nil {
			choice, selectErr := selectWorkspaceRaw(ctx, selection, in, out, s.style)
			restoreErr := restore()
			if selectErr != nil {
				return tobari.ProjectSelectionChoice{}, selectErr
			}
			if restoreErr != nil {
				return tobari.ProjectSelectionChoice{}, restoreErr
			}
			if err := writeWorkspaceSelectionSummary(out, selection, choice); err != nil {
				return tobari.ProjectSelectionChoice{}, err
			}
			return choice, nil
		}
	}

	choice, err := selectWorkspaceLine(ctx, selection, in, out)
	if err != nil {
		return tobari.ProjectSelectionChoice{}, err
	}
	if err := writeWorkspaceSelectionSummary(out, selection, choice); err != nil {
		return tobari.ProjectSelectionChoice{}, err
	}
	return choice, nil
}

type selectorKeyKind uint8

const (
	selectorKeyNone selectorKeyKind = iota
	selectorKeyUp
	selectorKeyDown
	selectorKeyHome
	selectorKeyEnd
	selectorKeyPageUp
	selectorKeyPageDown
	selectorKeyEnter
	selectorKeyCreate
	selectorKeyAllow
	selectorKeyTemplate
	selectorKeyExact
	selectorKeyDeny
	selectorKeyClear
	selectorKeyReset
	selectorKeyApply
	selectorKeyConfirm
	selectorKeyInherit
	selectorKeyLiteral
	selectorKeyBack
	selectorKeyCancel
	selectorKeyNumber
	selectorKeyInvalid
)

type selectorKey struct {
	kind  selectorKeyKind
	index int
}

func selectWorkspaceRaw(
	ctx context.Context, selection tobari.WorkspaceSelection, in io.Reader, out io.Writer,
	style bool,
) (tobari.ProjectSelectionChoice, error) {
	options := workspaceSelectorOptions(selection)
	selected := firstSelectableOption(options)
	message := ""
	lineCount := 0
	for {
		if err := ctx.Err(); err != nil {
			finishWorkspaceSelector(out, lineCount)
			return tobari.ProjectSelectionChoice{}, err
		}
		top := selectorWindowTop(selected, len(options), selectorMaxVisibleOptions)
		currentLines := renderWorkspaceSelector(out, selection, options, selected, top, message, lineCount, style)
		if currentLines < 0 {
			finishWorkspaceSelector(out, lineCount)
			return tobari.ProjectSelectionChoice{}, fmt.Errorf("render Workspace selector")
		}
		lineCount = currentLines
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishWorkspaceSelector(out, lineCount)
			return tobari.ProjectSelectionChoice{}, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = moveSelectableOption(options, selected, -1)
			message = ""
		case selectorKeyDown:
			selected = moveSelectableOption(options, selected, 1)
			message = ""
		case selectorKeyHome:
			selected = firstSelectableOption(options)
			message = ""
		case selectorKeyEnd:
			selected = lastSelectableOption(options)
			message = ""
		case selectorKeyCreate:
			if !selection.CanCreate {
				message = "A Workspace already exists at the current directory."
				continue
			}
			finishWorkspaceSelector(out, lineCount)
			return tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}, nil
		case selectorKeyNumber:
			if key.index < 0 || key.index >= len(options) {
				message = "That Workspace option does not exist."
				continue
			}
			selected = key.index
			message = ""
			if !options[selected].selectable {
				message = "That Workspace is unavailable."
				continue
			}
			finishWorkspaceSelector(out, lineCount)
			return workspaceChoice(options[selected]), nil
		case selectorKeyEnter:
			if !options[selected].selectable {
				message = "That Workspace is unavailable."
				continue
			}
			finishWorkspaceSelector(out, lineCount)
			return workspaceChoice(options[selected]), nil
		case selectorKeyCancel:
			finishWorkspaceSelector(out, lineCount)
			return tobari.ProjectSelectionChoice{}, context.Canceled
		default:
			message = "Use ↑/↓ to move, Enter to select, n to create, or q to cancel."
		}
	}
}

func selectWorkspaceLine(
	ctx context.Context, selection tobari.WorkspaceSelection, in io.Reader, out io.Writer,
) (tobari.ProjectSelectionChoice, error) {
	options := workspaceSelectorOptions(selection)
	if _, err := fmt.Fprintf(out, "Select a Workspace for %s\n\n", safeExternalText(selection.CWD)); err != nil {
		return tobari.ProjectSelectionChoice{}, err
	}
	for index, option := range options {
		if _, err := fmt.Fprintf(out, "  %d. %s\n", index+1, lineWorkspaceOption(option)); err != nil {
			return tobari.ProjectSelectionChoice{}, err
		}
	}
	if _, err := fmt.Fprintln(out, "\nChoose [1], n to create, or q to cancel:"); err != nil {
		return tobari.ProjectSelectionChoice{}, err
	}
	reader := bufio.NewReader(in)
	for {
		if err := ctx.Err(); err != nil {
			return tobari.ProjectSelectionChoice{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return tobari.ProjectSelectionChoice{}, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		if value == "" {
			value = "1"
		}
		if value == "q" || value == "quit" || value == "esc" {
			return tobari.ProjectSelectionChoice{}, context.Canceled
		}
		if value == "n" || value == "new" {
			if selection.CanCreate {
				return tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}, nil
			}
			if _, writeErr := fmt.Fprintln(out, "A Workspace already exists at the current directory. Choose an existing Workspace or q to cancel."); writeErr != nil {
				return tobari.ProjectSelectionChoice{}, writeErr
			}
			if errors.Is(err, io.EOF) {
				return tobari.ProjectSelectionChoice{}, context.Canceled
			}
			continue
		}
		index, parseErr := strconv.Atoi(value)
		if parseErr == nil && index >= 1 && index <= len(options) {
			option := options[index-1]
			if option.selectable {
				return workspaceChoice(option), nil
			}
			if _, writeErr := fmt.Fprintln(out, "That Workspace is unavailable. Choose another option or q to cancel."); writeErr != nil {
				return tobari.ProjectSelectionChoice{}, writeErr
			}
		} else if writeErr := writeSelectorLine(out, "Enter a listed number, n, or q."); writeErr != nil {
			return tobari.ProjectSelectionChoice{}, writeErr
		}
		if errors.Is(err, io.EOF) {
			return tobari.ProjectSelectionChoice{}, context.Canceled
		}
	}
}

type workspaceSelectorOption struct {
	candidate  *tobari.WorkspaceSelectionCandidate
	create     bool
	selectable bool
	nearest    bool
}

func workspaceSelectorOptions(selection tobari.WorkspaceSelection) []workspaceSelectorOption {
	options := make([]workspaceSelectorOption, 0, len(selection.Candidates)+1)
	for index := range selection.Candidates {
		candidate := &selection.Candidates[index]
		options = append(options, workspaceSelectorOption{
			candidate: candidate, selectable: candidate.Runtime != tobari.RuntimeDiagnosticIncomplete,
			nearest: index == 0,
		})
	}
	if selection.CanCreate {
		options = append(options, workspaceSelectorOption{create: true, selectable: true})
	}
	return options
}

func workspaceChoice(option workspaceSelectorOption) tobari.ProjectSelectionChoice {
	if option.create {
		return tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}
	}
	return tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionUse, ID: option.candidate.ID}
}

func firstSelectableOption(options []workspaceSelectorOption) int {
	for index, option := range options {
		if option.selectable {
			return index
		}
	}
	return 0
}

func lastSelectableOption(options []workspaceSelectorOption) int {
	for index := len(options) - 1; index >= 0; index-- {
		if options[index].selectable {
			return index
		}
	}
	return 0
}

func moveSelectableOption(options []workspaceSelectorOption, selected, delta int) int {
	if len(options) == 0 {
		return 0
	}
	for attempts := 0; attempts < len(options); attempts++ {
		selected = (selected + delta + len(options)) % len(options)
		if options[selected].selectable {
			return selected
		}
	}
	return selected
}

func selectorWindowTop(selected, optionCount, window int) int {
	if optionCount <= window {
		return 0
	}
	top := selected - window/2
	if top < 0 {
		top = 0
	}
	if top > optionCount-window {
		top = optionCount - window
	}
	return top
}

func renderWorkspaceSelector(
	out io.Writer, selection tobari.WorkspaceSelection, options []workspaceSelectorOption,
	selected, top int, message string, previousLines int, style bool,
) int {
	lines := []string{
		"Select a Workspace for " + safeExternalText(selection.CWD),
		"",
		"Use ↑/↓ to move, Enter to select, n to create, q/Esc to cancel.",
		"",
	}
	end := top + selectorMaxVisibleOptions
	if end > len(options) {
		end = len(options)
	}
	for index := top; index < end; index++ {
		lines = append(lines, ansiWorkspaceOption(options[index], index == selected, style))
	}
	if top > 0 || end < len(options) {
		lines = append(lines, fmt.Sprintf("  Showing %d-%d of %d options", top+1, end, len(options)))
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	}
	lineCount, err := renderSelectorScreen(out, lines, previousLines)
	if err != nil {
		return -1
	}
	return lineCount
}

func ansiWorkspaceOption(option workspaceSelectorOption, selected, style bool) string {
	marker := "●"
	if option.create {
		marker = "＋"
	}
	prefix := "  "
	if selected {
		prefix = applyStyleToken(style, styleText, "❯ ")
	}
	if option.create {
		return prefix + applyStyleToken(style, styleText, marker+" Create a new Workspace here")
	}
	path := truncateSelectorPath(option.candidate.Root, selectorPathWidth)
	status := string(option.candidate.Runtime)
	detail := status + " · ancestor"
	if option.nearest {
		detail = status + " · nearest ancestor"
	}
	if option.candidate.Runtime == tobari.RuntimeDiagnosticIncomplete {
		detail += " · unavailable"
	}
	pathText := applyStyleToken(style, styleText, path)
	statusText := applyStyleToken(style, humanStatusToken(status), detail)
	return prefix + applyStyleToken(style, styleMuted, marker) + " " + pathText + "  " + statusText
}

func lineWorkspaceOption(option workspaceSelectorOption) string {
	if option.create {
		return "Create a new Workspace here"
	}
	detail := string(option.candidate.Runtime) + " · ancestor"
	if option.nearest {
		detail = string(option.candidate.Runtime) + " · nearest ancestor"
	}
	if !option.selectable {
		detail += " · unavailable"
	}
	return truncateSelectorPath(option.candidate.Root, selectorPathWidth) + "  " + detail
}

func truncateSelectorPath(value string, width int) string {
	value = safeExternalText(value)
	if width <= 0 || utf8.RuneCountInString(value) <= width {
		return value
	}
	runes := []rune(value)
	left := width / 2
	right := width - left - 1
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func writeWorkspaceSelectionSummary(out io.Writer, selection tobari.WorkspaceSelection, choice tobari.ProjectSelectionChoice) error {
	if choice.Kind == tobari.ProjectSelectionCreate {
		return writeSelectorLines(out,
			"Creating a new Workspace here",
			"  Root              "+safeExternalText(selection.CWD),
		)
	}
	candidate, found := selection.Candidate(choice.ID)
	if !found {
		return fmt.Errorf("selected Workspace is absent from the snapshot")
	}
	return writeSelectorLines(out,
		"Using existing Workspace",
		"  Root              "+safeExternalText(candidate.Root),
		"  Working directory "+safeExternalText(selection.CWD),
	)
}

func writeSelectorLines(out io.Writer, lines ...string) error {
	for _, line := range lines {
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func writeSelectorLine(out io.Writer, line string) error {
	_, err := fmt.Fprintln(out, line)
	return err
}

func finishWorkspaceSelector(out io.Writer, lines int) {
	finishSelectorScreen(out, lines)
}

func readSelectorKey(ctx context.Context, in io.Reader) (selectorKey, error) {
	for {
		key, err := readSelectorKeyOnce(ctx, in)
		if errors.Is(err, errSelectorTimeout) || (err == nil && key.kind == selectorKeyNone) {
			continue
		}
		return key, err
	}
}

func readSelectorKeyOnce(ctx context.Context, in io.Reader) (selectorKey, error) {
	value, err := readSelectorByte(ctx, in)
	if errors.Is(err, errSelectorTimeout) {
		return selectorKey{}, errSelectorTimeout
	}
	if errors.Is(err, errSelectorEOF) {
		return selectorKey{kind: selectorKeyCancel}, nil
	}
	if err != nil {
		return selectorKey{}, err
	}
	switch value {
	case 0:
		return selectorKey{kind: selectorKeyNone}, nil
	case '\r', '\n':
		return selectorKey{kind: selectorKeyEnter}, nil
	case 'n', 'N':
		return selectorKey{kind: selectorKeyCreate}, nil
	case 'a', 'A':
		return selectorKey{kind: selectorKeyAllow}, nil
	case 't', 'T':
		return selectorKey{kind: selectorKeyTemplate}, nil
	case 'e', 'E':
		return selectorKey{kind: selectorKeyExact}, nil
	case 'd', 'D':
		return selectorKey{kind: selectorKeyDeny}, nil
	case 'x', 'X':
		return selectorKey{kind: selectorKeyClear}, nil
	case 'r', 'R':
		return selectorKey{kind: selectorKeyReset}, nil
	case 'p', 'P':
		return selectorKey{kind: selectorKeyApply}, nil
	case 'y', 'Y':
		return selectorKey{kind: selectorKeyConfirm}, nil
	case 'h', 'H', 'i', 'I':
		return selectorKey{kind: selectorKeyInherit}, nil
	case 'l', 'L':
		return selectorKey{kind: selectorKeyLiteral}, nil
	case 'b', 'B':
		return selectorKey{kind: selectorKeyBack}, nil
	case 'q', 'Q', 3, 4:
		return selectorKey{kind: selectorKeyCancel}, nil
	case '\x1b':
		next, nextErr := readSelectorByte(ctx, in)
		if errors.Is(nextErr, errSelectorTimeout) {
			return selectorKey{kind: selectorKeyCancel}, nil
		}
		if errors.Is(nextErr, errSelectorEOF) {
			return selectorKey{kind: selectorKeyCancel}, nil
		}
		if nextErr != nil {
			return selectorKey{}, nextErr
		}
		if next != '[' && next != 'O' {
			return selectorKey{kind: selectorKeyCancel}, nil
		}
		code, codeErr := readSelectorByte(ctx, in)
		if errors.Is(codeErr, errSelectorTimeout) {
			return selectorKey{kind: selectorKeyCancel}, nil
		}
		if errors.Is(codeErr, errSelectorEOF) {
			return selectorKey{kind: selectorKeyCancel}, nil
		}
		if codeErr != nil {
			return selectorKey{}, codeErr
		}
		switch code {
		case 'A':
			return selectorKey{kind: selectorKeyUp}, nil
		case 'B':
			return selectorKey{kind: selectorKeyDown}, nil
		case 'H':
			return selectorKey{kind: selectorKeyHome}, nil
		case 'F':
			return selectorKey{kind: selectorKeyEnd}, nil
		case '5', '6':
			terminator, terminatorErr := readSelectorByte(ctx, in)
			if terminatorErr != nil || terminator != '~' {
				return selectorKey{kind: selectorKeyInvalid}, nil
			}
			if code == '5' {
				return selectorKey{kind: selectorKeyPageUp}, nil
			}
			return selectorKey{kind: selectorKeyPageDown}, nil
		default:
			return selectorKey{kind: selectorKeyInvalid}, nil
		}
	default:
		if value >= '1' && value <= '9' {
			return selectorKey{kind: selectorKeyNumber, index: int(value - '1')}, nil
		}
		return selectorKey{kind: selectorKeyInvalid}, nil
	}
}

func readSelectorByte(ctx context.Context, in io.Reader) (byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var value [1]byte
		n, err := in.Read(value[:])
		if n > 0 {
			return value[0], nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				// Unix raw mode uses VMIN=0/VTIME=1 so the read can
				// poll for context cancellation. Go maps the resulting
				// zero-byte terminal read to io.EOF; it is a timeout, not
				// a closed interactive session. A real terminal still
				// cancels explicitly through q or Ctrl-D.
				if terminal.IsCharDevice(in) {
					return 0, errSelectorTimeout
				}
				return 0, errSelectorEOF
			}
			return 0, err
		}
		if n == 0 {
			return 0, errSelectorTimeout
		}
	}
}
