package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

// policyReviewDecision is deliberately separate from the selected candidate.
// A canceled review is a successful no-op, while a non-empty ID is the one
// opaque reference that may cross into policy allow.
type policyReviewDecision struct {
	CandidateID string
	Canceled    bool
}

type policyReviewSelector struct {
	mode terminal.Mode
}

func newPolicyReviewSelector() *policyReviewSelector {
	return &policyReviewSelector{mode: terminal.New()}
}

func (s *policyReviewSelector) Select(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
) (policyReviewDecision, error) {
	if err := report.Validate(); err != nil {
		return policyReviewDecision{}, err
	}
	if len(report.Items) == 0 {
		return policyReviewDecision{Canceled: true}, nil
	}

	if s != nil && s.mode != nil {
		restore, rawErr := s.mode.Enter(in)
		if rawErr == nil {
			decision, selectErr := selectPolicyReviewRaw(ctx, report, in, out)
			restoreErr := restore()
			if selectErr != nil {
				return policyReviewDecision{}, selectErr
			}
			if restoreErr != nil {
				return policyReviewDecision{}, restoreErr
			}
			return decision, nil
		}
	}

	return selectPolicyReviewLine(ctx, report, in, out)
}

type policyReviewDetailResult struct {
	CandidateID string
	Back        bool
	Canceled    bool
	Lines       int
}

func selectPolicyReviewRaw(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
) (policyReviewDecision, error) {
	selected := 0
	message := ""
	lineCount := 0
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
		}
		top := selectorWindowTop(selected, len(report.Items), selectorMaxVisibleOptions)
		currentLines := renderPolicyReviewListRaw(out, report, selected, top, message, lineCount)
		if currentLines < 0 {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, fmt.Errorf("render policy review selector")
		}
		lineCount = currentLines
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
		}
		switch key.kind {
		case selectorKeyUp:
			selected = (selected - 1 + len(report.Items)) % len(report.Items)
			message = ""
		case selectorKeyDown:
			selected = (selected + 1) % len(report.Items)
			message = ""
		case selectorKeyHome:
			selected = 0
			message = ""
		case selectorKeyEnd:
			selected = len(report.Items) - 1
			message = ""
		case selectorKeyNumber:
			if key.index < 0 || key.index >= len(report.Items) {
				message = "That permission does not exist."
				continue
			}
			selected = key.index
			fallthrough
		case selectorKeyEnter:
			detail := selectPolicyReviewDetailRaw(ctx, report, selected, in, out, lineCount)
			if detail.err != nil {
				return policyReviewDecision{}, detail.err
			}
			if detail.CandidateID != "" {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyReviewDecision{CandidateID: detail.CandidateID}, nil
			}
			if detail.Canceled {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyReviewDecision{Canceled: true}, nil
			}
			finishPolicyReviewSelector(out, detail.Lines)
			lineCount = 0
			message = ""
		case selectorKeyCancel:
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{Canceled: true}, nil
		default:
			message = "Use ↑/↓ to move, Enter to inspect, or q to cancel."
		}
	}
}

// policyReviewDetailRawResult carries an error without making the public
// decision type represent an internal rendering failure.
type policyReviewDetailRawResult struct {
	policyReviewDetailResult
	err error
}

func selectPolicyReviewDetailRaw(
	ctx context.Context, report tobari.PolicyCandidateReport, selected int,
	in io.Reader, out io.Writer, previousLines int,
) policyReviewDetailRawResult {
	if selected < 0 || selected >= len(report.Items) {
		return policyReviewDetailRawResult{err: fmt.Errorf("selected policy permission is outside the snapshot")}
	}
	candidate := report.Items[selected]
	message := ""
	lineCount := previousLines
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDetailRawResult{err: err}
		}
		currentLines := renderPolicyReviewDetailRaw(out, report, selected, message, lineCount)
		if currentLines < 0 {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDetailRawResult{err: fmt.Errorf("render policy permission detail")}
		}
		lineCount = currentLines
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDetailRawResult{err: err}
		}
		switch key.kind {
		case selectorKeyAllow:
			confirmed, confirmLines, confirmErr := confirmPolicyReviewRaw(ctx, report, selected, in, out, lineCount)
			if confirmErr != nil {
				return policyReviewDetailRawResult{err: confirmErr}
			}
			lineCount = confirmLines
			if confirmed {
				return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
					CandidateID: candidate.ID, Lines: lineCount,
				}}
			}
			message = ""
		case selectorKeyBack, selectorKeyCancel:
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				Back: true, Lines: lineCount,
			}}
		default:
			message = "Press a to allow this exact permission, or q to go back."
		}
	}
}

func confirmPolicyReviewRaw(
	ctx context.Context, report tobari.PolicyCandidateReport, selected int,
	in io.Reader, out io.Writer, previousLines int,
) (bool, int, error) {
	message := "Allow this exact permission? Type y to continue; default is no."
	lineCount := previousLines
	for {
		currentLines := renderPolicyReviewDetailRaw(out, report, selected, message, lineCount)
		if currentLines < 0 {
			finishPolicyReviewSelector(out, lineCount)
			return false, lineCount, fmt.Errorf("render policy permission confirmation")
		}
		lineCount = currentLines
		value, err := readSelectorByte(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return false, lineCount, err
		}
		switch value {
		case 'y', 'Y':
			return true, lineCount, nil
		case 'n', 'N', '\r', '\n', 'q', 'Q', 3, 4, 27:
			return false, lineCount, nil
		default:
			message = "Type y to allow, or n to keep this permission blocked."
		}
	}
}

func renderPolicyReviewListRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected, top int, message string, previousLines int,
) int {
	lines := []string{
		"Tobari · Permission Inbox",
		"",
		fmt.Sprintf("%d pending permission%s", len(report.Items), pluralSuffix(len(report.Items))),
		"",
	}
	end := top + selectorMaxVisibleOptions
	if end > len(report.Items) {
		end = len(report.Items)
	}
	for index := top; index < end; index++ {
		candidate := report.Items[index]
		prefix := "  "
		if index == selected {
			prefix = applyColorToken(true, colorTokenAccent, "❯ ")
		}
		line := prefix + policyReviewCandidateRequest(candidate)
		if index != selected {
			line = applyColorToken(true, colorTokenMuted, line)
		}
		lines = append(lines, line)
	}
	if top > 0 || end < len(report.Items) {
		lines = append(lines, fmt.Sprintf("  Showing %d-%d of %d", top+1, end, len(report.Items)))
	}
	lines = append(lines, "", "↑/↓ move   Enter inspect   q cancel")
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyColorToken(true, colorTokenWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func renderPolicyReviewDetailRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected int, message string, previousLines int,
) int {
	candidate := report.Items[selected]
	lines := []string{
		"Tobari · Permission Inbox",
		"",
		fmt.Sprintf("Permission %d of %d", selected+1, len(report.Items)),
		"",
		"Scope     Current Tobari only",
		"Request   " + policyReviewCandidateRequest(candidate),
		"Reason    " + safeExternalText(candidate.Reason),
		fmt.Sprintf("Status    %d", candidate.StatusCode),
		"Observed  " + safeExternalText(candidate.ObservedAt),
		"",
		"This allows exactly this host, port, method, and path.",
		"",
		"[a] Allow this permission   [q] Back",
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyColorToken(true, colorTokenWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func renderPolicyReviewScreen(out io.Writer, lines []string, previousLines int) int {
	for index, line := range lines {
		if index == 0 && previousLines > 0 {
			if _, err := fmt.Fprintf(out, "\x1b[%dA", previousLines); err != nil {
				return -1
			}
		} else if index == 0 {
			if _, err := io.WriteString(out, "\x1b[?25l"); err != nil {
				return -1
			}
		}
		if _, err := fmt.Fprintf(out, "\x1b[2K\r%s\n", line); err != nil {
			return -1
		}
	}
	return len(lines)
}

func finishPolicyReviewSelector(out io.Writer, lines int) {
	if lines > 0 {
		_, _ = fmt.Fprintf(out, "\x1b[%dA", lines)
		for index := 0; index < lines; index++ {
			_, _ = io.WriteString(out, "\x1b[2K\r")
			if index < lines-1 {
				_, _ = io.WriteString(out, "\n")
			}
		}
		_, _ = io.WriteString(out, "\n")
	}
	_, _ = io.WriteString(out, "\x1b[?25h")
}

func selectPolicyReviewLine(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
) (policyReviewDecision, error) {
	reader := bufio.NewReader(in)
	for {
		if err := ctx.Err(); err != nil {
			return policyReviewDecision{}, err
		}
		if err := writePolicyReviewListLine(out, report); err != nil {
			return policyReviewDecision{}, err
		}
		if _, err := fmt.Fprintln(out, "\nChoose a number to inspect, or q to cancel:"); err != nil {
			return policyReviewDecision{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return policyReviewDecision{}, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		if value == "q" || value == "quit" || value == "esc" {
			return policyReviewDecision{Canceled: true}, nil
		}
		index, parseErr := strconv.Atoi(value)
		if parseErr != nil || index < 1 || index > len(report.Items) {
			if writeErr := writeSelectorLine(out, "Enter a listed number or q."); writeErr != nil {
				return policyReviewDecision{}, writeErr
			}
			if errors.Is(err, io.EOF) {
				return policyReviewDecision{Canceled: true}, nil
			}
			continue
		}

		detail, detailErr := selectPolicyReviewDetailLine(ctx, report, index-1, reader, out)
		if detailErr != nil {
			return policyReviewDecision{}, detailErr
		}
		if detail.CandidateID != "" || detail.Canceled {
			return policyReviewDecision{CandidateID: detail.CandidateID, Canceled: detail.Canceled}, nil
		}
		if errors.Is(err, io.EOF) {
			return policyReviewDecision{Canceled: true}, nil
		}
	}
}

func writePolicyReviewListLine(out io.Writer, report tobari.PolicyCandidateReport) error {
	if _, err := fmt.Fprintln(out, "Tobari · Permission Inbox"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%d pending permission%s\n\n", len(report.Items), pluralSuffix(len(report.Items))); err != nil {
		return err
	}
	for index, candidate := range report.Items {
		if _, err := fmt.Fprintf(out, "  %d. %s\n", index+1, policyReviewCandidateRequest(candidate)); err != nil {
			return err
		}
	}
	return nil
}

func selectPolicyReviewDetailLine(
	ctx context.Context, report tobari.PolicyCandidateReport, selected int,
	reader *bufio.Reader, out io.Writer,
) (policyReviewDetailResult, error) {
	candidate := report.Items[selected]
	if err := writePolicyReviewDetailLines(out, report, selected); err != nil {
		return policyReviewDetailResult{}, err
	}
	for {
		if _, err := fmt.Fprintln(out, "\nChoose [a] to allow this exact permission, or [q] to go back:"); err != nil {
			return policyReviewDetailResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return policyReviewDetailResult{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return policyReviewDetailResult{}, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		switch value {
		case "q", "quit", "esc", "b", "back":
			return policyReviewDetailResult{Back: true}, nil
		case "a", "allow":
			if _, writeErr := fmt.Fprintln(out, "Allow exactly this permission? [y/N]"); writeErr != nil {
				return policyReviewDetailResult{}, writeErr
			}
			confirmation, confirmationErr := reader.ReadString('\n')
			if confirmationErr != nil && !errors.Is(confirmationErr, io.EOF) {
				return policyReviewDetailResult{}, confirmationErr
			}
			if strings.EqualFold(strings.TrimSpace(confirmation), "y") || strings.EqualFold(strings.TrimSpace(confirmation), "yes") {
				return policyReviewDetailResult{CandidateID: candidate.ID}, nil
			}
			if errors.Is(confirmationErr, io.EOF) {
				return policyReviewDetailResult{Canceled: true}, nil
			}
			if _, writeErr := fmt.Fprintln(out, "Kept blocked. Choose [a] to allow or [q] to go back."); writeErr != nil {
				return policyReviewDetailResult{}, writeErr
			}
		default:
			if _, writeErr := fmt.Fprintln(out, "Use a to allow this exact permission, or q to go back."); writeErr != nil {
				return policyReviewDetailResult{}, writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return policyReviewDetailResult{Canceled: true}, nil
		}
	}
}

func writePolicyReviewDetailLines(out io.Writer, report tobari.PolicyCandidateReport, selected int) error {
	candidate := report.Items[selected]
	return writeSelectorLines(out,
		"Permission "+strconv.Itoa(selected+1)+" of "+strconv.Itoa(len(report.Items)),
		"",
		"Scope     Current Tobari only",
		"Request   "+policyReviewCandidateRequest(candidate),
		"Reason    "+safeExternalText(candidate.Reason),
		fmt.Sprintf("Status    %d", candidate.StatusCode),
		"Observed  "+safeExternalText(candidate.ObservedAt),
		"",
		"This allows exactly this host, port, method, and path.",
	)
}

func policyReviewCandidateRequest(candidate tobari.PolicyCandidate) string {
	return fmt.Sprintf(
		"%s:%d %s %s",
		safeExternalText(candidate.Host), candidate.Port,
		safeExternalText(candidate.Method), safeExternalText(candidate.Path),
	)
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}
