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
// opaque reference that may cross into one exact policy action.
type policyReviewDecision struct {
	CandidateID string
	Action      policyReviewAction
	Apply       bool
	Canceled    bool
}

type policyReviewAction uint8

const (
	policyReviewActionNone policyReviewAction = iota
	policyReviewActionAllow
	policyReviewActionDeny
)

type policyReviewSelector struct {
	mode       terminal.Mode
	style      bool
	lineReader *bufio.Reader
	staged     map[string]policyReviewAction
	notice     string
}

type policyReviewScopeKey struct {
	ContextID string
	ProjectID string
}

func newPolicyReviewSelector() *policyReviewSelector {
	return newPolicyReviewSelectorWithStyle(true)
}

func newPolicyReviewSelectorWithStyle(enabled bool) *policyReviewSelector {
	return &policyReviewSelector{mode: terminal.New(), style: enabled, staged: map[string]policyReviewAction{}}
}

func (s *policyReviewSelector) Stage(candidateID string, action policyReviewAction) {
	if s == nil {
		return
	}
	if s.staged == nil {
		s.staged = map[string]policyReviewAction{}
	}
	s.staged[candidateID] = action
	label := "Allow exact"
	if action == policyReviewActionDeny {
		label = "Deny exact"
	}
	s.notice = fmt.Sprintf("Staged %s · %d decision%s ready to apply.", label, len(s.staged), pluralSuffix(len(s.staged)))
}

func (s *policyReviewSelector) Select(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
) (policyReviewDecision, error) {
	if err := report.Validate(); err != nil {
		return policyReviewDecision{}, err
	}
	report = groupPolicyReviewReport(report)
	if len(report.Items) == 0 {
		return policyReviewDecision{Canceled: true}, nil
	}

	if s != nil && s.mode != nil {
		restore, rawErr := s.mode.Enter(in)
		if rawErr == nil {
			decision, selectErr := selectPolicyReviewRaw(ctx, report, in, out, s.style, s.notice)
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

	if s.lineReader == nil {
		s.lineReader = bufio.NewReader(in)
	}
	return selectPolicyReviewLine(ctx, report, s.lineReader, out, len(s.staged), s.notice)
}

type policyReviewDetailResult struct {
	CandidateID string
	Action      policyReviewAction
	Back        bool
	Canceled    bool
	Lines       int
}

func selectPolicyReviewRaw(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
	style bool, notice ...string,
) (policyReviewDecision, error) {
	selected := 0
	message := ""
	if len(notice) > 0 {
		message = notice[0]
	}
	lineCount := 0
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
		}
		if needsRender {
			top := selectorWindowTop(selected, len(report.Items), selectorMaxVisibleOptions)
			currentLines := renderPolicyReviewListRaw(out, report, selected, top, message, lineCount, style)
			if currentLines < 0 {
				finishPolicyReviewSelector(out, lineCount)
				return policyReviewDecision{}, fmt.Errorf("render policy review selector")
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
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
				message = "That permission does not exist."
				needsRender = true
				continue
			}
			selected = key.index
			fallthrough
		case selectorKeyEnter:
			detail := selectPolicyReviewDetailRaw(ctx, report, selected, in, out, lineCount, style)
			if detail.err != nil {
				return policyReviewDecision{}, detail.err
			}
			if detail.CandidateID != "" {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyReviewDecision{CandidateID: detail.CandidateID, Action: detail.Action}, nil
			}
			if detail.Canceled {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyReviewDecision{Canceled: true}, nil
			}
			finishPolicyReviewSelector(out, detail.Lines)
			lineCount = 0
			message = ""
			needsRender = true
		case selectorKeyCancel:
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{Canceled: true}, nil
		case selectorKeyApply:
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{Apply: true}, nil
		default:
			message = "Use ↑/↓ to move, Enter to inspect, or q to cancel."
			needsRender = true
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
	in io.Reader, out io.Writer, previousLines int, style bool,
) policyReviewDetailRawResult {
	if selected < 0 || selected >= len(report.Items) {
		return policyReviewDetailRawResult{err: fmt.Errorf("selected policy permission is outside the snapshot")}
	}
	candidate := report.Items[selected]
	message := ""
	lineCount := previousLines
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDetailRawResult{err: err}
		}
		if needsRender {
			currentLines := renderPolicyReviewDetailRaw(out, report, selected, message, lineCount, style)
			if currentLines < 0 {
				finishPolicyReviewSelector(out, lineCount)
				return policyReviewDetailRawResult{err: fmt.Errorf("render policy permission detail")}
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDetailRawResult{err: err}
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyAllow:
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				CandidateID: candidate.ID, Action: policyReviewActionAllow, Lines: lineCount,
			}}
		case selectorKeyDeny:
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				CandidateID: candidate.ID, Action: policyReviewActionDeny, Lines: lineCount,
			}}
		case selectorKeyBack, selectorKeyCancel:
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				Back: true, Lines: lineCount,
			}}
		default:
			message = "Press a to allow exact, d to deny exact, or q to go back."
			needsRender = true
		}
	}
}

func renderPolicyReviewListRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected, top int, message string, previousLines int,
	style bool,
) int {
	report = groupPolicyReviewReport(report)
	lines := []string{
		selectorTitle(style, "Tobari · Permission Inbox"),
		"",
		applyStyleToken(style, styleWarning, fmt.Sprintf(
			"%d pending permission%s in %d Tobari",
			len(report.Items), pluralSuffix(len(report.Items)), policyReviewScopeCount(report.Items),
		)),
		"",
	}
	end := top + selectorMaxVisibleOptions
	if end > len(report.Items) {
		end = len(report.Items)
	}
	for index := top; index < end; index++ {
		candidate := report.Items[index]
		if index == top || !samePolicyReviewScope(report.Items[index-1], candidate) {
			if index > top {
				lines = append(lines, "")
			}
			lines = append(lines, applyStyleToken(style, styleText, policyReviewScopeHeading(candidate)))
		}
		prefix := "    "
		if index == selected {
			prefix = "  " + applyStyleToken(style, styleText, "❯ ")
		}
		lines = append(lines, prefix+
			applyStyleToken(style, styleText, policyReviewCandidateListEffect(candidate))+"  "+
			applyStyleToken(style, styleMuted, fmt.Sprintf("%d×", candidate.EffectiveObservationCount())))
	}
	if top > 0 || end < len(report.Items) {
		lines = append(lines, applyStyleToken(style, styleMuted, fmt.Sprintf("  Showing %d-%d of %d", top+1, end, len(report.Items))))
	}
	selectedCandidate := report.Items[selected]
	lines = append(lines,
		"",
		applyStyleToken(style, styleMuted, "Selected"),
		"  "+applyStyleToken(style, styleText, policyReviewCandidateEffect(selectedCandidate)),
		"  "+applyStyleToken(style, styleMuted, fmt.Sprintf(
			"Observed %s · Latest %s",
			policyCandidateObservationText(selectedCandidate), safeExternalText(selectedCandidate.ObservedAt),
		)),
		"",
		selectorHelp(style, "↑/↓ move   Enter inspect   p apply staged   q cancel"),
	)
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func groupPolicyReviewReport(report tobari.PolicyCandidateReport) tobari.PolicyCandidateReport {
	groups := make([][]tobari.PolicyCandidate, 0, len(report.Items))
	groupIndexes := make(map[policyReviewScopeKey]int, len(report.Items))
	for _, candidate := range report.Items {
		key := policyReviewScopeKey{ContextID: candidate.ContextID, ProjectID: candidate.ProjectID}
		groupIndex, found := groupIndexes[key]
		if !found {
			groupIndex = len(groups)
			groupIndexes[key] = groupIndex
			groups = append(groups, []tobari.PolicyCandidate{})
		}
		groups[groupIndex] = append(groups[groupIndex], candidate)
	}
	report.Items = make([]tobari.PolicyCandidate, 0, len(report.Items))
	for _, group := range groups {
		report.Items = append(report.Items, group...)
	}
	return report
}

func policyReviewScopeCount(items []tobari.PolicyCandidate) int {
	seen := make(map[policyReviewScopeKey]struct{}, len(items))
	for _, candidate := range items {
		seen[policyReviewScopeKey{ContextID: candidate.ContextID, ProjectID: candidate.ProjectID}] = struct{}{}
	}
	return len(seen)
}

func samePolicyReviewScope(left, right tobari.PolicyCandidate) bool {
	return left.ContextID == right.ContextID && left.ProjectID == right.ProjectID
}

func policyReviewScopeHeading(candidate tobari.PolicyCandidate) string {
	return safeExternalText(candidate.ContextName) + " · " + safeExternalText(candidate.ProjectRoot)
}

func policyReviewCandidateListEffect(candidate tobari.PolicyCandidate) string {
	effect := fmt.Sprintf(
		"%-6s %s:%d%s",
		safeExternalText(candidate.Method), safeExternalText(candidate.Host), candidate.Port, safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		effect += " · GraphQL " + coordinate
	}
	return effect
}

func policyReviewCandidateEffect(candidate tobari.PolicyCandidate) string {
	effect := fmt.Sprintf(
		"%s %s:%d%s",
		safeExternalText(candidate.Method), safeExternalText(candidate.Host), candidate.Port, safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		effect += " · GraphQL " + coordinate
	}
	return effect
}

func renderPolicyReviewDetailRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected int, message string, previousLines int,
	style bool,
) int {
	candidate := report.Items[selected]
	lines := []string{
		selectorTitle(style, "Tobari · Permission Inbox"),
		"",
		applyStyleToken(style, styleAccent, fmt.Sprintf("Permission %d of %d", selected+1, len(report.Items))),
		"",
		selectorDetail(style, "Context", safeExternalText(candidate.ContextName), styleText),
		selectorDetail(style, "Tobari", safeExternalText(candidate.ProjectRoot), styleText),
		selectorDetail(style, "Request", policyReviewCandidateRequest(candidate), styleText),
		selectorDetail(style, "Reason", safeExternalText(candidate.Reason), styleDanger),
		selectorDetail(style, "Status", fmt.Sprintf("%d", candidate.StatusCode), styleDanger),
		selectorDetail(style, "Observed", policyCandidateObservationText(candidate), styleText),
		selectorDetail(style, "Latest", safeExternalText(candidate.ObservedAt), styleText),
		"",
		selectorHelp(style, "This decision applies only to this Tobari in this Context."),
		"",
		selectorActions(
			styleAction(style, "[a] Allow exact", styleAccent),
			styleAction(style, "[d] Deny exact", styleAccent),
			styleAction(style, "[q] Back", styleMuted),
		),
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
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
	finishSelectorScreen(out, lines)
}

func selectPolicyReviewLine(
	ctx context.Context, report tobari.PolicyCandidateReport, reader *bufio.Reader, out io.Writer,
	stagedCount int, notice ...string,
) (policyReviewDecision, error) {
	for {
		if err := ctx.Err(); err != nil {
			return policyReviewDecision{}, err
		}
		if err := writePolicyReviewListLine(out, report); err != nil {
			return policyReviewDecision{}, err
		}
		if stagedCount > 0 {
			message := fmt.Sprintf("%d staged decision%s ready to apply.", stagedCount, pluralSuffix(stagedCount))
			if len(notice) > 0 && notice[0] != "" {
				message = notice[0]
			}
			if _, err := fmt.Fprintln(out, "\n"+message); err != nil {
				return policyReviewDecision{}, err
			}
		}
		if _, err := fmt.Fprintln(out, "\nChoose a number to inspect, p to apply staged decisions, or q to cancel:"); err != nil {
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
		if value == "p" || value == "apply" {
			return policyReviewDecision{Apply: true}, nil
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
			return policyReviewDecision{CandidateID: detail.CandidateID, Action: detail.Action, Canceled: detail.Canceled}, nil
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
		if _, err := fmt.Fprintf(out, "  %d. %s  %s\n     %s\n", index+1,
			safeExternalText(candidate.ContextName), safeExternalText(candidate.ProjectRoot), policyReviewCandidateRequest(candidate)); err != nil {
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
		if _, err := fmt.Fprintln(out, "\nChoose [a] to allow exact, [d] to deny exact, or [q] to go back:"); err != nil {
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
		case "a", "allow", "d", "deny", "reject":
			action := policyReviewActionAllow
			if value == "d" || value == "deny" || value == "reject" {
				action = policyReviewActionDeny
			}
			return policyReviewDetailResult{CandidateID: candidate.ID, Action: action}, nil
		default:
			if _, writeErr := fmt.Fprintln(out, "Use a to allow exact, d to deny exact, or q to go back."); writeErr != nil {
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
		"Context   "+safeExternalText(candidate.ContextName),
		"Tobari    "+safeExternalText(candidate.ProjectRoot),
		"Request   "+policyReviewCandidateRequest(candidate),
		"Reason    "+safeExternalText(candidate.Reason),
		fmt.Sprintf("Status    %d", candidate.StatusCode),
		"Observed  "+policyCandidateObservationText(candidate),
		"Latest    "+safeExternalText(candidate.ObservedAt),
		"",
		"This decision applies only to this Tobari in this Context.",
	)
}

func policyReviewCandidateRequest(candidate tobari.PolicyCandidate) string {
	request := fmt.Sprintf(
		"%s:%d %s %s",
		safeExternalText(candidate.Host), candidate.Port,
		safeExternalText(candidate.Method), safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		request += " · GraphQL " + coordinate
	}
	return request
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
