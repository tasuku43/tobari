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

// policyRuleDecision is deliberately only the opaque current rule ID. The
// reset action resolves the current snapshot again before changing policy.
type policyRuleDecision struct {
	RuleID   string
	Canceled bool
}

type policyRuleSelector struct {
	mode terminal.Mode
}

func newPolicyRuleSelector() *policyRuleSelector {
	return &policyRuleSelector{mode: terminal.New()}
}

func (s *policyRuleSelector) Select(
	ctx context.Context, report tobari.PolicyRuleReport, in io.Reader, out io.Writer,
) (policyRuleDecision, error) {
	if err := report.Validate(); err != nil {
		return policyRuleDecision{}, err
	}
	if len(report.Items) == 0 {
		return policyRuleDecision{Canceled: true}, nil
	}
	if s != nil && s.mode != nil {
		restore, rawErr := s.mode.Enter(in)
		if rawErr == nil {
			decision, selectErr := selectPolicyRulesRaw(ctx, report, in, out)
			restoreErr := restore()
			if selectErr != nil {
				return policyRuleDecision{}, selectErr
			}
			if restoreErr != nil {
				return policyRuleDecision{}, restoreErr
			}
			return decision, nil
		}
	}
	return selectPolicyRulesLine(ctx, report, in, out)
}

func selectPolicyRulesRaw(
	ctx context.Context, report tobari.PolicyRuleReport, in io.Reader, out io.Writer,
) (policyRuleDecision, error) {
	selected := 0
	message := ""
	lineCount := 0
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyRuleDecision{}, err
		}
		if needsRender {
			top := selectorWindowTop(selected, len(report.Items), selectorMaxVisibleOptions)
			currentLines := renderPolicyRulesListRaw(out, report, selected, top, message, lineCount)
			if currentLines < 0 {
				finishPolicyReviewSelector(out, lineCount)
				return policyRuleDecision{}, fmt.Errorf("render policy rule selector")
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyRuleDecision{}, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			selected = (selected - 1 + len(report.Items)) % len(report.Items)
			message = ""
			needsRender = true
		case selectorKeyDown:
			selected = (selected + 1) % len(report.Items)
			message = ""
			needsRender = true
		case selectorKeyHome:
			selected = 0
			message = ""
			needsRender = true
		case selectorKeyEnd:
			selected = len(report.Items) - 1
			message = ""
			needsRender = true
		case selectorKeyNumber:
			if key.index < 0 || key.index >= len(report.Items) {
				message = "That policy decision does not exist."
				needsRender = true
				continue
			}
			selected = key.index
			fallthrough
		case selectorKeyEnter:
			detail := selectPolicyRuleDetailRaw(ctx, report, selected, in, out, lineCount)
			if detail.err != nil {
				return policyRuleDecision{}, detail.err
			}
			if detail.RuleID != "" {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyRuleDecision{RuleID: detail.RuleID}, nil
			}
			if detail.Canceled {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyRuleDecision{Canceled: true}, nil
			}
			finishPolicyReviewSelector(out, detail.Lines)
			lineCount = 0
			message = ""
			needsRender = true
		case selectorKeyCancel:
			finishPolicyReviewSelector(out, lineCount)
			return policyRuleDecision{Canceled: true}, nil
		default:
			message = "Use ↑/↓ to move, Enter to inspect, or q to cancel."
			needsRender = true
		}
	}
}

type policyRuleDetailRawResult struct {
	RuleID   string
	Canceled bool
	Lines    int
	err      error
}

func selectPolicyRuleDetailRaw(
	ctx context.Context, report tobari.PolicyRuleReport, selected int,
	in io.Reader, out io.Writer, previousLines int,
) policyRuleDetailRawResult {
	if selected < 0 || selected >= len(report.Items) {
		return policyRuleDetailRawResult{err: fmt.Errorf("selected policy rule is outside the snapshot")}
	}
	rule := report.Items[selected]
	message := ""
	lineCount := previousLines
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyRuleDetailRawResult{err: err}
		}
		if needsRender {
			currentLines := renderPolicyRuleDetailRaw(out, report, selected, message, lineCount)
			if currentLines < 0 {
				finishPolicyReviewSelector(out, lineCount)
				return policyRuleDetailRawResult{err: fmt.Errorf("render policy rule detail")}
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyRuleDetailRawResult{err: err}
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyReset:
			confirmed, confirmLines, confirmErr := confirmPolicyRuleResetRaw(ctx, rule, in, out, lineCount)
			if confirmErr != nil {
				return policyRuleDetailRawResult{err: confirmErr}
			}
			lineCount = confirmLines
			if confirmed {
				return policyRuleDetailRawResult{RuleID: rule.ID, Lines: lineCount}
			}
			message = ""
			needsRender = true
		case selectorKeyBack, selectorKeyCancel:
			return policyRuleDetailRawResult{Canceled: key.kind == selectorKeyCancel, Lines: lineCount}
		default:
			message = "Press r to reset this decision, or q to go back."
			needsRender = true
		}
	}
}

func confirmPolicyRuleResetRaw(
	ctx context.Context, rule tobari.PolicyRule, in io.Reader, out io.Writer, previousLines int,
) (bool, int, error) {
	message := "Reset this " + rule.Decision + " decision? Type y to continue; default is no."
	lineCount := renderPolicyRuleDetailRawWithMessage(out, rule, message, previousLines)
	if lineCount < 0 {
		finishPolicyReviewSelector(out, previousLines)
		return false, previousLines, fmt.Errorf("render policy rule reset confirmation")
	}
	for {
		value, err := readSelectorByte(ctx, in)
		if err != nil {
			if errors.Is(err, errSelectorTimeout) {
				continue
			}
			if errors.Is(err, errSelectorEOF) {
				return false, lineCount, nil
			}
			finishPolicyReviewSelector(out, lineCount)
			return false, lineCount, err
		}
		switch value {
		case 'y', 'Y':
			return true, lineCount, nil
		case 'n', 'N', '\r', '\n', 'q', 'Q', 3, 4, 27:
			return false, lineCount, nil
		default:
			message = "Type y to confirm, or n to keep this decision."
			lineCount = renderPolicyRuleDetailRawWithMessage(out, rule, message, lineCount)
			if lineCount < 0 {
				finishPolicyReviewSelector(out, previousLines)
				return false, previousLines, fmt.Errorf("render policy rule reset confirmation")
			}
		}
	}
}

func renderPolicyRulesListRaw(
	out io.Writer, report tobari.PolicyRuleReport, selected, top int, message string, previousLines int,
) int {
	lines := []string{
		"Tobari · Policy decisions",
		"",
		fmt.Sprintf("%d learned decision%s", len(report.Items), pluralSuffix(len(report.Items))),
		"",
	}
	end := top + selectorMaxVisibleOptions
	if end > len(report.Items) {
		end = len(report.Items)
	}
	for index := top; index < end; index++ {
		rule := report.Items[index]
		prefix := "  "
		if index == selected {
			prefix = applyColorToken(true, colorTokenAccent, "❯ ")
		}
		line := prefix + strings.ToUpper(rule.Decision) + " " + policyRuleRequest(rule)
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

func renderPolicyRuleDetailRaw(
	out io.Writer, report tobari.PolicyRuleReport, selected int, message string, previousLines int,
) int {
	rule := report.Items[selected]
	lines := []string{
		"Tobari · Policy decisions",
		"",
		fmt.Sprintf("Decision %d of %d", selected+1, len(report.Items)),
		"",
		"Decision  " + strings.ToUpper(rule.Decision),
		"Scope     Current Context only",
		"Request   " + policyRuleRequest(rule),
		"Match     " + safeExternalText(rule.Match),
		"Rule ID   " + rule.ID,
		"Sources   " + strings.Join(rule.SourceCandidates, ", "),
		"",
		"Reset returns this exact effect to default deny.",
		"",
		"[r] Reset   [q] Back",
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyColorToken(true, colorTokenWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func renderPolicyRuleDetailRawWithMessage(
	out io.Writer, rule tobari.PolicyRule, message string, previousLines int,
) int {
	lines := []string{
		"Tobari · Policy decisions",
		"",
		"Decision reset confirmation",
		"",
		"Decision  " + strings.ToUpper(rule.Decision),
		"Request   " + policyRuleRequest(rule),
		"Rule ID   " + rule.ID,
		"",
		"Reset returns this exact effect to default deny.",
		"",
		applyColorToken(true, colorTokenWarning, "! "+message),
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func selectPolicyRulesLine(
	ctx context.Context, report tobari.PolicyRuleReport, in io.Reader, out io.Writer,
) (policyRuleDecision, error) {
	reader := bufio.NewReader(in)
	for {
		if err := ctx.Err(); err != nil {
			return policyRuleDecision{}, err
		}
		if err := writePolicyRulesListLine(out, report); err != nil {
			return policyRuleDecision{}, err
		}
		if _, err := fmt.Fprintln(out, "\nChoose a number to inspect, or q to cancel:"); err != nil {
			return policyRuleDecision{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return policyRuleDecision{}, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		if value == "q" || value == "quit" || value == "esc" {
			return policyRuleDecision{Canceled: true}, nil
		}
		index, parseErr := strconv.Atoi(value)
		if parseErr != nil || index < 1 || index > len(report.Items) {
			if writeErr := writeSelectorLine(out, "Enter a listed number or q."); writeErr != nil {
				return policyRuleDecision{}, writeErr
			}
			if errors.Is(err, io.EOF) {
				return policyRuleDecision{Canceled: true}, nil
			}
			continue
		}
		detail, detailErr := selectPolicyRuleDetailLine(ctx, report, index-1, reader, out)
		if detailErr != nil {
			return policyRuleDecision{}, detailErr
		}
		if detail.RuleID != "" || detail.Canceled {
			return policyRuleDecision{RuleID: detail.RuleID, Canceled: detail.Canceled}, nil
		}
		if errors.Is(err, io.EOF) {
			return policyRuleDecision{Canceled: true}, nil
		}
	}
}

func writePolicyRulesListLine(out io.Writer, report tobari.PolicyRuleReport) error {
	if _, err := fmt.Fprint(out, "Tobari · Policy decisions\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "%d learned decision%s\n\n", len(report.Items), pluralSuffix(len(report.Items))); err != nil {
		return err
	}
	for index, rule := range report.Items {
		if _, err := fmt.Fprintf(out, "  %d. %s %s\n", index+1, strings.ToUpper(rule.Decision), policyRuleRequest(rule)); err != nil {
			return err
		}
	}
	return nil
}

type policyRuleDetailLineResult struct {
	RuleID   string
	Canceled bool
}

func selectPolicyRuleDetailLine(
	ctx context.Context, report tobari.PolicyRuleReport, selected int,
	reader *bufio.Reader, out io.Writer,
) (policyRuleDetailLineResult, error) {
	rule := report.Items[selected]
	if err := writeSelectorLines(out,
		"Decision "+strconv.Itoa(selected+1)+" of "+strconv.Itoa(len(report.Items)),
		"",
		"Decision  "+strings.ToUpper(rule.Decision),
		"Scope     Current Context only",
		"Request   "+policyRuleRequest(rule),
		"Match     "+safeExternalText(rule.Match),
		"Rule ID   "+rule.ID,
		"Sources   "+strings.Join(rule.SourceCandidates, ", "),
		"",
		"Reset returns this exact effect to default deny.",
	); err != nil {
		return policyRuleDetailLineResult{}, err
	}
	for {
		if _, err := fmt.Fprintln(out, "\nChoose [r] to reset this decision, or [q] to go back:"); err != nil {
			return policyRuleDetailLineResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return policyRuleDetailLineResult{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return policyRuleDetailLineResult{}, err
		}
		value := strings.ToLower(strings.TrimSpace(line))
		switch value {
		case "q", "quit", "esc", "b", "back":
			return policyRuleDetailLineResult{}, nil
		case "r", "reset":
			if _, writeErr := fmt.Fprintln(out, "Reset this "+rule.Decision+" decision? [y/N]"); writeErr != nil {
				return policyRuleDetailLineResult{}, writeErr
			}
			confirmation, confirmationErr := reader.ReadString('\n')
			if confirmationErr != nil && !errors.Is(confirmationErr, io.EOF) {
				return policyRuleDetailLineResult{}, confirmationErr
			}
			if strings.EqualFold(strings.TrimSpace(confirmation), "y") || strings.EqualFold(strings.TrimSpace(confirmation), "yes") {
				return policyRuleDetailLineResult{RuleID: rule.ID}, nil
			}
			if errors.Is(confirmationErr, io.EOF) {
				return policyRuleDetailLineResult{Canceled: true}, nil
			}
			if _, writeErr := fmt.Fprintln(out, "Kept decision. Choose r to reset or q to go back."); writeErr != nil {
				return policyRuleDetailLineResult{}, writeErr
			}
		default:
			if _, writeErr := fmt.Fprintln(out, "Use r to reset or q to go back."); writeErr != nil {
				return policyRuleDetailLineResult{}, writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return policyRuleDetailLineResult{Canceled: true}, nil
		}
	}
}

func policyRuleRequest(rule tobari.PolicyRule) string {
	return fmt.Sprintf(
		"%s:%d %s %s",
		safeExternalText(rule.Host), rule.Port,
		safeExternalText(rule.Method), safeExternalText(rule.Path),
	)
}
