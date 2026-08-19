package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

// policyReviewDecision is deliberately separate from the selected candidate.
// A canceled review is a successful no-op, while a non-empty ID is the one
// opaque reference that may cross into one exact policy action.
type policyReviewDecision struct {
	CandidateID string
	SelectedID  string
	Action      policyReviewAction
	Apply       bool
	Refresh     bool
	// AutomaticRefresh distinguishes the watch timer from an explicit r key so
	// unchanged timer polls can remain silent while manual refresh reports.
	AutomaticRefresh bool
	Clear            bool
	Canceled         bool
	keepScreen       bool
	screenLines      int
	frameNotice      string
}

type policyReviewAction uint8

const (
	policyReviewActionNone policyReviewAction = iota
	policyReviewActionAllow
	policyReviewActionAllowTemplate
	policyReviewActionDeny
)

type policyReviewSelector struct {
	mode        terminal.Mode
	style       bool
	lineReader  *bufio.Reader
	staged      map[string]policyReviewAction
	stagedOrder []string
	selectedID  string
	notice      string
	watch       bool
	ticker      policyReviewRefreshTicker
	screenLines int
	rendered    bool
	lastReport  tobari.PolicyCandidateReport
	lastID      string
	lastNotice  string
	lastStaged  map[string]policyReviewAction
}

const (
	policyReviewRefreshInterval   = time.Second
	policyReviewRefreshMaxBackoff = 8 * time.Second
	// Compact list labels fit the width of "Allow exact". Keeping this column
	// fixed prevents the HTTP effect from moving as decisions change.
	policyReviewListStateWidth = 11
)

// policyReviewRefreshTicker is the narrow scheduling boundary for automatic
// inbox refresh. Raw terminal reads remain bounded by the terminal adapter;
// this ticker only decides whether one of those bounded wakeups is due.
type policyReviewRefreshTicker interface {
	Ready(context.Context) bool
	Succeeded()
	Failed()
	Delay() time.Duration
}

type intervalPolicyReviewRefreshTicker struct {
	now   func() time.Time
	next  time.Time
	delay time.Duration
}

func newIntervalPolicyReviewRefreshTicker() *intervalPolicyReviewRefreshTicker {
	return &intervalPolicyReviewRefreshTicker{now: time.Now, delay: policyReviewRefreshInterval}
}

func (t *intervalPolicyReviewRefreshTicker) Ready(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	now := t.now()
	if t.next.IsZero() {
		t.next = now.Add(t.delay)
		return false
	}
	if now.Before(t.next) {
		return false
	}
	t.next = now.Add(t.delay)
	return true
}

func (t *intervalPolicyReviewRefreshTicker) Succeeded() {
	t.delay = policyReviewRefreshInterval
	t.next = t.now().Add(t.delay)
}

func (t *intervalPolicyReviewRefreshTicker) Failed() {
	if t.delay < policyReviewRefreshMaxBackoff {
		t.delay *= 2
		if t.delay > policyReviewRefreshMaxBackoff {
			t.delay = policyReviewRefreshMaxBackoff
		}
	}
	t.next = t.now().Add(t.delay)
}

func (t *intervalPolicyReviewRefreshTicker) Delay() time.Duration { return t.delay }

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

func (s *policyReviewSelector) EnableWatch(ticker policyReviewRefreshTicker) {
	if s == nil {
		return
	}
	s.watch = true
	if ticker == nil {
		ticker = s.ticker
	}
	if ticker == nil {
		ticker = newIntervalPolicyReviewRefreshTicker()
	}
	s.ticker = ticker
}

func (s *policyReviewSelector) RefreshSucceeded() {
	if s != nil && s.ticker != nil {
		s.ticker.Succeeded()
	}
}

func (s *policyReviewSelector) RefreshFailed() time.Duration {
	if s == nil || s.ticker == nil {
		return 0
	}
	s.ticker.Failed()
	return s.ticker.Delay()
}

func (s *policyReviewSelector) Stage(candidateID string, action policyReviewAction) {
	if s == nil {
		return
	}
	if s.staged == nil {
		s.staged = map[string]policyReviewAction{}
	}
	if _, exists := s.staged[candidateID]; !exists {
		s.stagedOrder = append(s.stagedOrder, candidateID)
	}
	s.staged[candidateID] = action
	label := "Allow exact"
	if action == policyReviewActionAllowTemplate {
		label = "Allow template"
	}
	if action == policyReviewActionDeny {
		label = "Deny exact"
	}
	s.notice = fmt.Sprintf("Staged %s · %d decision%s ready to apply.", label, len(s.staged), pluralSuffix(len(s.staged)))
}

func (s *policyReviewSelector) Clear(candidateID string) {
	if s == nil || s.staged == nil {
		return
	}
	if _, found := s.staged[candidateID]; !found {
		s.notice = "That permission has no staged decision."
		return
	}
	delete(s.staged, candidateID)
	kept := s.stagedOrder[:0]
	for _, id := range s.stagedOrder {
		if id != candidateID {
			kept = append(kept, id)
		}
	}
	s.stagedOrder = kept
	s.notice = fmt.Sprintf("Decision cleared · %d decision%s ready to apply.", len(s.staged), pluralSuffix(len(s.staged)))
}

func (s *policyReviewSelector) ClearAll() {
	if s == nil {
		return
	}
	s.staged = map[string]policyReviewAction{}
	s.stagedOrder = nil
}

func (s *policyReviewSelector) Reconcile(report tobari.PolicyCandidateReport) int {
	if s == nil {
		return 0
	}
	report = groupPolicyReviewReport(report)
	current := make(map[string]struct{}, len(report.Items))
	for _, candidate := range report.Items {
		current[candidate.ID] = struct{}{}
	}
	kept := make([]string, 0, len(s.stagedOrder))
	removed := 0
	for _, id := range s.stagedOrder {
		if _, found := current[id]; found {
			kept = append(kept, id)
			continue
		}
		delete(s.staged, id)
		removed++
	}
	s.stagedOrder = kept
	return removed
}

func (s *policyReviewSelector) OrderedDecisions() []policyReviewDecision {
	if s == nil {
		return nil
	}
	result := make([]policyReviewDecision, 0, len(s.stagedOrder))
	for _, id := range s.stagedOrder {
		if action, found := s.staged[id]; found {
			result = append(result, policyReviewDecision{CandidateID: id, Action: action})
		}
	}
	return result
}

func (s *policyReviewSelector) Select(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
) (policyReviewDecision, error) {
	if err := report.Validate(); err != nil {
		return policyReviewDecision{}, err
	}
	report = groupPolicyReviewReport(report)
	if len(report.Items) == 0 && (s == nil || !s.watch) {
		return policyReviewDecision{Canceled: true}, nil
	}

	if s != nil && s.mode != nil {
		restore, rawErr := s.mode.Enter(in)
		if rawErr == nil {
			previousLines := 0
			renderOnEntry := true
			if s.watch && s.rendered {
				previousLines = s.screenLines
				renderOnEntry = s.watchFrameChanged(report)
			}
			decision, selectErr := selectPolicyReviewRawWithWatch(
				ctx, report, in, out, s.style, s.staged, s.stagedOrder, s.selectedID, s.notice,
				s.watch, s.ticker, previousLines, renderOnEntry,
			)
			restoreErr := restore()
			if selectErr != nil {
				s.resetWatchFrame()
				return policyReviewDecision{}, selectErr
			}
			if restoreErr != nil {
				if decision.keepScreen {
					finishPolicyReviewSelector(out, decision.screenLines)
				}
				s.resetWatchFrame()
				return policyReviewDecision{}, restoreErr
			}
			if decision.SelectedID != "" {
				s.selectedID = decision.SelectedID
			}
			if decision.keepScreen {
				s.screenLines = decision.screenLines
				s.rememberWatchFrame(report, decision.SelectedID, decision.frameNotice)
			} else {
				s.resetWatchFrame()
			}
			return decision, nil
		}
		if s.watch && s.rendered {
			finishPolicyReviewSelector(out, s.screenLines)
			s.resetWatchFrame()
		}
	}

	if s.watch {
		return policyReviewDecision{}, fault.New(
			fault.KindInvalidInput, "policy_review_watch_requires_tty",
			"policy review --watch requires an interactive raw terminal and text output", false,
			fault.NextAction{Command: "help policy review", Reason: "Run watch with text output in an interactive raw terminal."},
		)
	}
	if s.lineReader == nil {
		s.lineReader = bufio.NewReader(in)
	}
	decision, err := selectPolicyReviewLine(ctx, report, s.lineReader, out, s.staged, s.stagedOrder, s.notice)
	if err == nil && decision.SelectedID != "" {
		s.selectedID = decision.SelectedID
	}
	return decision, err
}

func (s *policyReviewSelector) watchFrameChanged(report tobari.PolicyCandidateReport) bool {
	if s == nil || !s.rendered {
		return true
	}
	report = groupPolicyReviewReport(report)
	return !reflect.DeepEqual(s.lastReport, report) || s.lastID != s.selectedID ||
		s.lastNotice != s.notice || !maps.Equal(s.lastStaged, s.staged)
}

func (s *policyReviewSelector) rememberWatchFrame(
	report tobari.PolicyCandidateReport, selectedID, notice string,
) {
	if s == nil {
		return
	}
	s.rendered = true
	s.lastReport = groupPolicyReviewReport(report)
	s.lastID = selectedID
	s.lastNotice = notice
	s.lastStaged = maps.Clone(s.staged)
}

func (s *policyReviewSelector) resetWatchFrame() {
	if s == nil {
		return
	}
	s.screenLines = 0
	s.rendered = false
	s.lastReport = tobari.PolicyCandidateReport{}
	s.lastID = ""
	s.lastNotice = ""
	s.lastStaged = nil
}

type policyReviewDetailResult struct {
	CandidateID string
	Action      policyReviewAction
	Back        bool
	Canceled    bool
	Lines       int
}

type policyReviewFinalResult struct {
	Apply    bool
	Back     bool
	Canceled bool
	Lines    int
	err      error
}

func policyReviewStagedCount(staged []map[string]policyReviewAction) int {
	if len(staged) == 0 || staged[0] == nil {
		return 0
	}
	return len(staged[0])
}

func policyReviewStagedLabel(candidateID string, staged []map[string]policyReviewAction) string {
	if len(staged) == 0 || staged[0] == nil {
		return "Pending"
	}
	switch staged[0][candidateID] {
	case policyReviewActionAllow:
		return "Allow exact"
	case policyReviewActionAllowTemplate:
		return "Allow template"
	case policyReviewActionDeny:
		return "Deny exact"
	default:
		return "Pending"
	}
}

func policyReviewListState(
	report tobari.PolicyCandidateReport, candidate tobari.PolicyCandidate, staged []map[string]policyReviewAction,
) string {
	state := policyReviewStagedLabel(candidate.ID, staged)
	if state == "Allow template" {
		return "Allow {id}"
	}
	if state == "Pending" && policyReviewTemplateByID(report, candidate.ID) != nil {
		return "Review {id}"
	}
	return state
}

func policyReviewStagedStyle(candidateID string, staged []map[string]policyReviewAction) styleToken {
	if len(staged) == 0 || staged[0] == nil {
		return styleMuted
	}
	return policyReviewActionStyle(staged[0][candidateID])
}

func policyReviewActionStyle(action policyReviewAction) styleToken {
	switch action {
	case policyReviewActionAllow, policyReviewActionAllowTemplate:
		return styleSuccess
	case policyReviewActionDeny:
		return styleWarning
	default:
		return styleMuted
	}
}

func policyReviewActionLabel(action policyReviewAction) string {
	if action == policyReviewActionDeny {
		return "Deny exact"
	}
	if action == policyReviewActionAllowTemplate {
		return "Allow template"
	}
	return "Allow exact"
}

func policyReviewActionLabelFor(report tobari.PolicyCandidateReport, id string, action policyReviewAction) string {
	if policyReviewTemplateByID(report, id) == nil {
		return policyReviewActionLabel(action)
	}
	if action == policyReviewActionDeny {
		return "Deny pending exact"
	}
	if action == policyReviewActionAllowTemplate {
		return "Allow template"
	}
	return "Allow observed exact"
}

func selectPolicyReviewFinalRaw(
	ctx context.Context, report tobari.PolicyCandidateReport,
	staged map[string]policyReviewAction, stagedOrder []string,
	in io.Reader, out io.Writer, previousLines int, style bool,
) policyReviewFinalResult {
	message := ""
	lineCount := previousLines
	needsRender := true
	for {
		if err := ctx.Err(); err != nil {
			return policyReviewFinalResult{Lines: lineCount, err: err}
		}
		if needsRender {
			lineCount = renderPolicyReviewFinalRaw(out, report, staged, stagedOrder, message, lineCount, style)
			if lineCount < 0 {
				return policyReviewFinalResult{err: fmt.Errorf("render final policy review")}
			}
			needsRender = false
		}
		key, err := readSelectorKey(ctx, in)
		if err != nil {
			return policyReviewFinalResult{Lines: lineCount, err: err}
		}
		switch key.kind {
		case selectorKeyConfirm:
			return policyReviewFinalResult{Apply: true, Lines: lineCount}
		case selectorKeyBack:
			return policyReviewFinalResult{Back: true, Lines: lineCount}
		case selectorKeyCancel:
			return policyReviewFinalResult{Canceled: true, Lines: lineCount}
		default:
			message = "Press y to Apply, b to go back, or q to cancel."
			needsRender = true
		}
	}
}

func renderPolicyReviewFinalRaw(
	out io.Writer, report tobari.PolicyCandidateReport,
	staged map[string]policyReviewAction, stagedOrder []string,
	message string, previousLines int, style bool,
) int {
	lines := []string{
		selectorTitle(style, "Tobari · Review staged permissions"),
		"",
		applyStyleToken(style, styleWarning, policyReviewFinalCountText(report, staged)),
		"",
	}
	for index, id := range stagedOrder {
		action, found := staged[id]
		candidate, current := policyReviewCandidateByID(report, id)
		if !found || !current {
			continue
		}
		lines = append(lines,
			applyStyleToken(style, policyReviewActionStyle(action), fmt.Sprintf("%d. %s", index+1, policyReviewActionLabelFor(report, id, action))),
			selectorDetail(style, "Context", safeExternalText(candidate.ContextName)+" · "+candidate.ContextID, styleText),
			selectorDetail(style, "Project", safeExternalText(candidate.ProjectRoot)+" · "+candidate.ProjectID, styleText),
			selectorDetail(style, "Effect", policyReviewCandidateEffect(candidate), styleText),
			selectorDetail(style, "Candidate", candidate.ID, styleText),
			"",
		)
	}
	lines = append(lines,
		selectorHelp(style, "Staging grants no authority. Apply performs one trusted-host policy mutation."),
		"",
		selectorActions(
			styleAction(style, "[y] Apply", styleAccent),
			styleAction(style, "[b] Back", styleMuted),
			styleAction(style, "[q] Cancel all", styleMuted),
		),
	)
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func policyReviewFinalCountText(report tobari.PolicyCandidateReport, staged map[string]policyReviewAction) string {
	label := "exact decision"
	for id, action := range staged {
		if action == policyReviewActionAllowTemplate && policyReviewTemplateByID(report, id) != nil {
			label = "reviewed decision"
			break
		}
	}
	return fmt.Sprintf("%d %s%s ready for one Apply", len(staged), label, pluralSuffix(len(staged)))
}

func selectPolicyReviewRaw(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
	style bool, staged map[string]policyReviewAction, stagedOrder []string, selectedID string, notice ...string,
) (policyReviewDecision, error) {
	message := ""
	if len(notice) > 0 {
		message = notice[0]
	}
	return selectPolicyReviewRawWithWatch(ctx, report, in, out, style, staged, stagedOrder, selectedID, message, false, nil, 0, true)
}

func selectPolicyReviewRawWithWatch(
	ctx context.Context, report tobari.PolicyCandidateReport, in io.Reader, out io.Writer,
	style bool, staged map[string]policyReviewAction, stagedOrder []string, selectedID, notice string,
	watch bool, ticker policyReviewRefreshTicker, previousLines int, renderOnEntry bool,
) (policyReviewDecision, error) {
	report = groupPolicyReviewReport(report)
	selected := policyReviewSelectedIndex(report.Items, selectedID)
	message := notice
	lineCount := previousLines
	needsRender := renderOnEntry
	for {
		if err := ctx.Err(); err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
		}
		if needsRender {
			top := selectorWindowTop(selected, len(report.Items), selectorMaxVisibleOptions)
			currentLines := renderPolicyReviewListRaw(out, report, selected, top, message, lineCount, style, staged)
			if currentLines < 0 {
				finishPolicyReviewSelector(out, lineCount)
				return policyReviewDecision{}, fmt.Errorf("render policy review selector")
			}
			lineCount = currentLines
			needsRender = false
		}
		key, err := readSelectorKeyOnce(ctx, in)
		if errors.Is(err, errSelectorTimeout) || (err == nil && key.kind == selectorKeyNone) {
			if watch && ticker != nil && ticker.Ready(ctx) {
				return policyReviewDecision{
					SelectedID: candidateIDAt(report.Items, selected), Refresh: true, AutomaticRefresh: true,
					keepScreen: true, screenLines: lineCount, frameNotice: message,
				}, nil
			}
			continue
		}
		if err != nil {
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{}, err
		}
		switch key.kind {
		case selectorKeyNone:
			continue
		case selectorKeyUp:
			if len(report.Items) == 0 {
				continue
			}
			selected = (selected - 1 + len(report.Items)) % len(report.Items)
			message = ""
			needsRender = true
		case selectorKeyDown:
			if len(report.Items) == 0 {
				continue
			}
			selected = (selected + 1) % len(report.Items)
			message = ""
			needsRender = true
		case selectorKeyHome:
			if len(report.Items) == 0 {
				continue
			}
			selected = 0
			message = ""
			needsRender = true
		case selectorKeyEnd:
			if len(report.Items) == 0 {
				continue
			}
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
			if len(report.Items) == 0 {
				message = "No requests need review. Watching for denied requests…"
				needsRender = true
				continue
			}
			detail := selectPolicyReviewDetailRaw(ctx, report, selected, in, out, lineCount, style)
			if detail.err != nil {
				return policyReviewDecision{}, detail.err
			}
			if detail.CandidateID != "" {
				decision := policyReviewDecision{
					CandidateID: detail.CandidateID, SelectedID: candidateIDAt(report.Items, selected), Action: detail.Action,
				}
				if watch {
					decision.keepScreen = true
					decision.screenLines = detail.Lines
					decision.frameNotice = message
				} else {
					finishPolicyReviewSelector(out, detail.Lines)
				}
				return decision, nil
			}
			if detail.Canceled {
				finishPolicyReviewSelector(out, detail.Lines)
				return policyReviewDecision{SelectedID: candidateIDAt(report.Items, selected), Canceled: true}, nil
			}
			lineCount = detail.Lines
			message = ""
			needsRender = true
		case selectorKeyAllow, selectorKeyDeny:
			if len(report.Items) == 0 {
				continue
			}
			candidate := report.Items[selected]
			action := policyReviewActionAllow
			if key.kind == selectorKeyDeny {
				action = policyReviewActionDeny
			}
			selectedID := candidate.ID
			if next := policyReviewNextUndecidedID(report.Items, selected, staged, candidate.ID); next != "" {
				selectedID = next
			}
			decision := policyReviewDecision{CandidateID: candidate.ID, SelectedID: selectedID, Action: action}
			if watch {
				decision.keepScreen = true
				decision.screenLines = lineCount
				decision.frameNotice = message
			} else {
				finishPolicyReviewSelector(out, lineCount)
			}
			return decision, nil
		case selectorKeyClear:
			if len(report.Items) == 0 {
				continue
			}
			decision := policyReviewDecision{CandidateID: report.Items[selected].ID, SelectedID: report.Items[selected].ID, Clear: true}
			if watch {
				decision.keepScreen = true
				decision.screenLines = lineCount
				decision.frameNotice = message
			} else {
				finishPolicyReviewSelector(out, lineCount)
			}
			return decision, nil
		case selectorKeyCancel:
			finishPolicyReviewSelector(out, lineCount)
			return policyReviewDecision{SelectedID: candidateIDAt(report.Items, selected), Canceled: true}, nil
		case selectorKeyReset:
			decision := policyReviewDecision{SelectedID: candidateIDAt(report.Items, selected), Refresh: true}
			if watch {
				decision.keepScreen = true
				decision.screenLines = lineCount
				decision.frameNotice = message
			} else {
				finishPolicyReviewSelector(out, lineCount)
			}
			return decision, nil
		case selectorKeyApply:
			if len(staged) == 0 {
				message = "Inspect a permission and stage Allow exact or Deny exact first."
				needsRender = true
				continue
			}
			final := selectPolicyReviewFinalRaw(ctx, report, staged, stagedOrder, in, out, lineCount, style)
			if final.err != nil {
				return policyReviewDecision{}, final.err
			}
			if final.Back {
				lineCount = final.Lines
				message = "Staged decisions unchanged."
				needsRender = true
				continue
			}
			finishPolicyReviewSelector(out, final.Lines)
			if final.Canceled {
				return policyReviewDecision{SelectedID: candidateIDAt(report.Items, selected), Canceled: true}, nil
			}
			if final.Apply {
				return policyReviewDecision{SelectedID: candidateIDAt(report.Items, selected), Apply: true}, nil
			}
		default:
			message = "Use ↑/↓ to move, a/d to stage exact, x to clear, Enter to inspect, r to refresh, or q to cancel."
			needsRender = true
		}
	}
}

func policyReviewNextUndecidedID(
	items []tobari.PolicyCandidate, selected int, staged map[string]policyReviewAction, currentID string,
) string {
	for index := selected + 1; index < len(items); index++ {
		id := items[index].ID
		if id == currentID {
			continue
		}
		if _, decided := staged[id]; !decided {
			return id
		}
	}
	return ""
}

func policyReviewSelectedIndex(items []tobari.PolicyCandidate, selectedID string) int {
	for index, item := range items {
		if item.ID == selectedID {
			return index
		}
	}
	return 0
}

func candidateIDAt(items []tobari.PolicyCandidate, index int) string {
	if index < 0 || index >= len(items) {
		return ""
	}
	return items[index].ID
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
			if policyReviewTemplateByID(report, candidate.ID) != nil {
				message = "Press t to allow the template, e to allow observed exact paths, d to deny pending exact paths, or q to go back."
				needsRender = true
				continue
			}
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				CandidateID: candidate.ID, Action: policyReviewActionAllow, Lines: lineCount,
			}}
		case selectorKeyTemplate:
			if policyReviewTemplateByID(report, candidate.ID) == nil {
				message = "This exact permission has no template proposal."
				needsRender = true
				continue
			}
			return policyReviewDetailRawResult{policyReviewDetailResult: policyReviewDetailResult{
				CandidateID: candidate.ID, Action: policyReviewActionAllowTemplate, Lines: lineCount,
			}}
		case selectorKeyExact:
			if policyReviewTemplateByID(report, candidate.ID) == nil {
				message = "Press a to allow exact, d to deny exact, or q to go back."
				needsRender = true
				continue
			}
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
			if policyReviewTemplateByID(report, candidate.ID) != nil {
				message = "Press t to allow the template, e to allow observed exact paths, d to deny pending exact paths, or q to go back."
			} else {
				message = "Press a to allow exact, d to deny exact, or q to go back."
			}
			needsRender = true
		}
	}
}

func renderPolicyReviewListRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected, top int, message string, previousLines int,
	style bool, staged ...map[string]policyReviewAction,
) int {
	report = groupPolicyReviewReport(report)
	if len(report.Items) == 0 {
		lines := []string{
			selectorTitle(style, "Tobari · Permission Inbox"),
			"",
			applyStyleToken(style, styleText, "No requests need review."),
			"",
			applyStyleToken(style, styleMuted, "Watching for denied requests…"),
			applyStyleToken(style, styleMuted, "Press q to stop."),
		}
		if message != "" {
			lines = append(lines, "", applyStyleToken(style, styleWarning, "! "+message))
		}
		return renderPolicyReviewScreen(out, lines, previousLines)
	}
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
		state := fmt.Sprintf("%-*s", policyReviewListStateWidth, policyReviewListState(report, candidate, staged))
		lines = append(lines, prefix+applyStyleToken(style, policyReviewStagedStyle(candidate.ID, staged), state)+"  "+
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
			policyReviewObservationText(report, selectedCandidate), safeExternalText(selectedCandidate.ObservedAt),
		)),
		"  "+applyStyleToken(style, styleDanger, "Reason: "+safeExternalText(selectedCandidate.Reason)),
		"",
	)
	help := "↑/↓ move   a allow exact   d deny exact   x clear   Enter inspect   r refresh   q cancel"
	if policyReviewStagedCount(staged) > 0 {
		help = "↑/↓ move   a allow exact   d deny exact   x clear   Enter inspect   p review staged   r refresh   q cancel"
	}
	lines = append(lines, selectorHelp(style, help))
	stagedNotice := strings.HasPrefix(message, "Staged Allow") || strings.HasPrefix(message, "Staged Deny")
	if policyReviewAllVisibleStaged(report.Items, staged) && (message == "" || stagedNotice) {
		message = "All visible permissions have a staged decision. Press p to review and apply."
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func policyReviewAllVisibleStaged(items []tobari.PolicyCandidate, staged []map[string]policyReviewAction) bool {
	if len(items) == 0 || len(staged) == 0 || staged[0] == nil {
		return false
	}
	for _, item := range items {
		if _, found := staged[0][item.ID]; !found {
			return false
		}
	}
	return true
}

func groupPolicyReviewReport(report tobari.PolicyCandidateReport) tobari.PolicyCandidateReport {
	reviewItems := report.ReviewItems
	if reviewItems == nil {
		var err error
		reviewItems, err = tobari.PolicyReviewItems(report.Items, []tobari.LearnedPolicyRule{})
		if err != nil {
			return report
		}
	}
	displayItems := make([]tobari.PolicyCandidate, 0, len(reviewItems))
	for _, item := range reviewItems {
		if item.Candidate != nil {
			displayItems = append(displayItems, *item.Candidate)
			continue
		}
		if item.Template == nil || len(item.Template.PendingCandidates) == 0 {
			continue
		}
		proposal := item.Template
		candidate := proposal.PendingCandidates[len(proposal.PendingCandidates)-1]
		candidate.ID = proposal.ID
		candidate.Path = proposal.Path
		candidate.ObservationCount = len(proposal.Examples)
		displayItems = append(displayItems, candidate)
	}
	report.Items = displayItems
	report.ReviewItems = reviewItems
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

func policyReviewTemplateByID(report tobari.PolicyCandidateReport, id string) *tobari.PolicyPathTemplateProposal {
	for _, item := range report.ReviewItems {
		if item.ID == id {
			return item.Template
		}
	}
	return nil
}

func policyReviewObservationText(report tobari.PolicyCandidateReport, candidate tobari.PolicyCandidate) string {
	if proposal := policyReviewTemplateByID(report, candidate.ID); proposal != nil {
		return fmt.Sprintf("%d distinct values", len(proposal.Examples))
	}
	return policyCandidateObservationText(candidate)
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
		"%-6s %s://%s:%d%s",
		safeExternalText(candidate.Method), safeExternalText(candidate.Scheme), safeExternalText(candidate.Host), candidate.Port, safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		effect += " · GraphQL " + coordinate
	}
	if candidate.EffectiveProtocol() == tobari.PolicyProtocolMCP {
		effect += " · MCP " + safeExternalText(candidate.MCPMethod)
		if candidate.MCPToolName != "" {
			effect += " · " + safeExternalText(candidate.MCPToolName)
		}
	}
	return effect
}

func policyReviewCandidateEffect(candidate tobari.PolicyCandidate) string {
	effect := fmt.Sprintf(
		"%s %s://%s:%d%s",
		safeExternalText(candidate.Method), safeExternalText(candidate.Scheme), safeExternalText(candidate.Host), candidate.Port, safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		effect += " · GraphQL " + coordinate
	}
	if candidate.EffectiveProtocol() == tobari.PolicyProtocolMCP {
		effect += " · MCP " + safeExternalText(candidate.MCPMethod)
		if candidate.MCPToolName != "" {
			effect += " · " + safeExternalText(candidate.MCPToolName)
		}
	}
	return effect
}

func renderPolicyReviewDetailRaw(
	out io.Writer, report tobari.PolicyCandidateReport, selected int, message string, previousLines int,
	style bool,
) int {
	candidate := report.Items[selected]
	proposal := policyReviewTemplateByID(report, candidate.ID)
	lines := []string{
		selectorTitle(style, "Tobari · Permission Inbox"),
		"",
		applyStyleToken(style, styleAccent, fmt.Sprintf("Permission %d of %d", selected+1, len(report.Items))),
		"",
		selectorDetail(style, "Context", safeExternalText(candidate.ContextName), styleText),
		selectorDetail(style, "Tobari", safeExternalText(candidate.ProjectRoot), styleText),
		selectorDetail(style, "Request", policyReviewCandidateRequest(candidate), styleText),
		selectorDetail(style, "Authority", policyReviewCandidateAuthority(candidate), styleText),
		selectorDetail(style, "Reason", safeExternalText(candidate.Reason), styleDanger),
		selectorDetail(style, "Status", fmt.Sprintf("%d", candidate.StatusCode), styleDanger),
		selectorDetail(style, "Observed", policyReviewObservationText(report, candidate), styleText),
		selectorDetail(style, "Latest", safeExternalText(candidate.ObservedAt), styleText),
		"",
	}
	if proposal != nil {
		lines = append(lines, selectorDetail(style, "Examples", strings.Join(proposal.Examples, ", "), styleText), "",
			selectorHelp(style, "Allow template includes future non-empty values in exactly the {id} segment."), "",
			selectorActions(
				styleAction(style, "[t] Allow template", styleAccent),
				styleAction(style, "[e] Allow observed exact", styleAccent),
				styleAction(style, "[d] Deny pending exact", styleAccent),
				styleAction(style, "[q] Back", styleMuted),
			))
	} else {
		help := "This decision applies only to this Tobari in this Context."
		if candidate.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			help = "This decision applies only while the current Host Loopback attachment remains active."
		}
		lines = append(lines, selectorHelp(style, help), "",
			selectorActions(
				styleAction(style, "[a] Allow exact", styleAccent),
				styleAction(style, "[d] Deny exact", styleAccent),
				styleAction(style, "[q] Back", styleMuted),
			))
	}
	if message == "" {
		lines = append(lines, "")
	} else {
		lines = append(lines, applyStyleToken(style, styleWarning, "! "+message))
	}
	return renderPolicyReviewScreen(out, lines, previousLines)
}

func policyReviewCandidateAuthority(candidate tobari.PolicyCandidate) string {
	if candidate.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
		return "Host Loopback · attachment-scoped · Workspace audience"
	}
	return "external service · persistent learned policy"
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

func selectPolicyReviewLine(
	ctx context.Context, report tobari.PolicyCandidateReport, reader *bufio.Reader, out io.Writer,
	staged map[string]policyReviewAction, stagedOrder []string, notice ...string,
) (policyReviewDecision, error) {
	for {
		if err := ctx.Err(); err != nil {
			return policyReviewDecision{}, err
		}
		if err := writePolicyReviewListLine(out, report, staged); err != nil {
			return policyReviewDecision{}, err
		}
		message := ""
		if len(notice) > 0 && notice[0] != "" {
			message = notice[0]
		} else if len(staged) > 0 {
			message = fmt.Sprintf("%d staged decision%s ready to review.", len(staged), pluralSuffix(len(staged)))
		}
		if message != "" {
			if _, err := fmt.Fprintln(out, "\n"+message); err != nil {
				return policyReviewDecision{}, err
			}
		}
		prompt := "\nChoose a number to inspect, r to refresh, or q to cancel:"
		if len(staged) > 0 {
			prompt = "\nChoose a number to inspect, r to refresh, p to review staged decisions, or q to cancel:"
		}
		if _, err := fmt.Fprintln(out, prompt); err != nil {
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
		if value == "r" || value == "refresh" {
			return policyReviewDecision{Refresh: true}, nil
		}
		if value == "p" || value == "apply" {
			if len(staged) == 0 {
				if writeErr := writeSelectorLine(out, "Inspect a permission and stage Allow exact or Deny exact first."); writeErr != nil {
					return policyReviewDecision{}, writeErr
				}
				continue
			}
			final, finalErr := selectPolicyReviewFinalLine(ctx, report, staged, stagedOrder, reader, out)
			if finalErr != nil {
				return policyReviewDecision{}, finalErr
			}
			if final.Back {
				continue
			}
			if final.Canceled {
				return policyReviewDecision{Canceled: true}, nil
			}
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
			return policyReviewDecision{
				CandidateID: detail.CandidateID, SelectedID: candidateIDAt(report.Items, index-1),
				Action: detail.Action, Canceled: detail.Canceled,
			}, nil
		}
		if errors.Is(err, io.EOF) {
			return policyReviewDecision{Canceled: true}, nil
		}
	}
}

func writePolicyReviewListLine(out io.Writer, report tobari.PolicyCandidateReport, staged map[string]policyReviewAction) error {
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
		state := policyReviewLineStagedLabel(candidate.ID, staged)
		if state == "Undecided" && policyReviewTemplateByID(report, candidate.ID) != nil {
			state = "Suggested"
		}
		if _, err := fmt.Fprintf(out, "  %d. %-12s  %s  %s\n     %s\n", index+1,
			state,
			safeExternalText(candidate.ContextName), safeExternalText(candidate.ProjectRoot), policyReviewCandidateRequest(candidate)); err != nil {
			return err
		}
	}
	return nil
}

func policyReviewLineStagedLabel(candidateID string, staged map[string]policyReviewAction) string {
	switch staged[candidateID] {
	case policyReviewActionAllow:
		return "Staged Allow"
	case policyReviewActionAllowTemplate:
		return "Staged Template"
	case policyReviewActionDeny:
		return "Staged Deny"
	default:
		return "Undecided"
	}
}

func selectPolicyReviewFinalLine(
	ctx context.Context, report tobari.PolicyCandidateReport,
	staged map[string]policyReviewAction, stagedOrder []string,
	reader *bufio.Reader, out io.Writer,
) (policyReviewFinalResult, error) {
	if err := writePolicyReviewFinalLines(out, report, staged, stagedOrder); err != nil {
		return policyReviewFinalResult{}, err
	}
	for {
		if _, err := fmt.Fprintln(out, "\nChoose [y] to Apply, [b] to go back, or [q] to cancel all:"); err != nil {
			return policyReviewFinalResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return policyReviewFinalResult{}, err
		}
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return policyReviewFinalResult{}, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "apply":
			return policyReviewFinalResult{Apply: true}, nil
		case "b", "back":
			return policyReviewFinalResult{Back: true}, nil
		case "q", "quit", "cancel", "esc":
			return policyReviewFinalResult{Canceled: true}, nil
		default:
			if _, writeErr := fmt.Fprintln(out, "Use y to Apply, b to go back, or q to cancel all."); writeErr != nil {
				return policyReviewFinalResult{}, writeErr
			}
		}
		if errors.Is(err, io.EOF) {
			return policyReviewFinalResult{Canceled: true}, nil
		}
	}
}

func writePolicyReviewFinalLines(
	out io.Writer, report tobari.PolicyCandidateReport,
	staged map[string]policyReviewAction, stagedOrder []string,
) error {
	if _, err := fmt.Fprintf(out, "Tobari · Review staged permissions\n\n%s\n", policyReviewFinalCountText(report, staged)); err != nil {
		return err
	}
	for index, id := range stagedOrder {
		action, found := staged[id]
		candidate, current := policyReviewCandidateByID(report, id)
		if !found || !current {
			continue
		}
		if _, err := fmt.Fprintf(out,
			"\n%d. %s\n   Context   %s · %s\n   Project   %s · %s\n   Effect    %s\n   Candidate %s\n",
			index+1, policyReviewActionLabelFor(report, id, action), safeExternalText(candidate.ContextName), candidate.ContextID,
			safeExternalText(candidate.ProjectRoot), candidate.ProjectID, policyReviewCandidateEffect(candidate), candidate.ID,
		); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "\nStaging grants no authority. Apply performs one trusted-host policy mutation.")
	return err
}

func selectPolicyReviewDetailLine(
	ctx context.Context, report tobari.PolicyCandidateReport, selected int,
	reader *bufio.Reader, out io.Writer,
) (policyReviewDetailResult, error) {
	candidate := report.Items[selected]
	proposal := policyReviewTemplateByID(report, candidate.ID)
	if err := writePolicyReviewDetailLines(out, report, selected); err != nil {
		return policyReviewDetailResult{}, err
	}
	for {
		prompt := "\nChoose [a] to allow exact, [d] to deny exact, or [q] to go back:"
		if proposal != nil {
			prompt = "\nChoose [t] to allow the template, [e] to allow observed exact paths, [d] to deny pending exact paths, or [q] to go back:"
		}
		if _, err := fmt.Fprintln(out, prompt); err != nil {
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
		case "t", "template":
			if proposal == nil {
				if _, writeErr := fmt.Fprintln(out, "This exact permission has no template proposal."); writeErr != nil {
					return policyReviewDetailResult{}, writeErr
				}
				continue
			}
			return policyReviewDetailResult{CandidateID: candidate.ID, Action: policyReviewActionAllowTemplate}, nil
		case "a", "allow", "e", "exact", "d", "deny", "reject":
			if (value == "e" || value == "exact") && proposal == nil {
				continue
			}
			action := policyReviewActionAllow
			if value == "d" || value == "deny" || value == "reject" {
				action = policyReviewActionDeny
			}
			return policyReviewDetailResult{CandidateID: candidate.ID, Action: action}, nil
		default:
			message := "Use a to allow exact, d to deny exact, or q to go back."
			if proposal != nil {
				message = "Use t to allow the template, e to allow observed exact paths, d to deny pending exact paths, or q to go back."
			}
			if _, writeErr := fmt.Fprintln(out, message); writeErr != nil {
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
	lines := []string{
		"Permission " + strconv.Itoa(selected+1) + " of " + strconv.Itoa(len(report.Items)),
		"",
		"Context   " + safeExternalText(candidate.ContextName),
		"Tobari    " + safeExternalText(candidate.ProjectRoot),
		"Request   " + policyReviewCandidateRequest(candidate),
		"Authority " + policyReviewCandidateAuthority(candidate),
		"Reason    " + safeExternalText(candidate.Reason),
		fmt.Sprintf("Status    %d", candidate.StatusCode),
		"Observed  " + policyReviewObservationText(report, candidate),
		"Latest    " + safeExternalText(candidate.ObservedAt),
		"",
	}
	if proposal := policyReviewTemplateByID(report, candidate.ID); proposal != nil {
		lines = append(lines, "Examples  "+strings.Join(proposal.Examples, ", "), "",
			"Allow template includes future non-empty values in exactly the {id} segment.")
	} else {
		help := "This decision applies only to this Tobari in this Context."
		if candidate.EffectiveDestinationKind() == tobari.PolicyDestinationHostLoopback {
			help = "This decision applies only while the current Host Loopback attachment remains active."
		}
		lines = append(lines, help)
	}
	return writeSelectorLines(out, lines...)
}

func policyReviewCandidateRequest(candidate tobari.PolicyCandidate) string {
	request := fmt.Sprintf(
		"%s://%s:%d %s %s",
		safeExternalText(candidate.Scheme), safeExternalText(candidate.Host), candidate.Port,
		safeExternalText(candidate.Method), safeExternalText(candidate.Path),
	)
	if coordinate := policyGraphQLCoordinate(candidate.PolicyProtocolIdentity); coordinate != "" {
		request += " · GraphQL " + coordinate
	}
	if candidate.EffectiveProtocol() == tobari.PolicyProtocolMCP {
		request += " · MCP " + safeExternalText(candidate.MCPMethod)
		if candidate.MCPToolName != "" {
			request += " · " + safeExternalText(candidate.MCPToolName)
		}
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
