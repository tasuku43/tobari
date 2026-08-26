package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type readyPolicyReviewTicker struct {
	ready    bool
	failures int
}

func (t *readyPolicyReviewTicker) Ready(context.Context) bool { return t.ready }
func (t *readyPolicyReviewTicker) Succeeded()                 { t.failures = 0 }
func (t *readyPolicyReviewTicker) Failed()                    { t.failures++ }
func (t *readyPolicyReviewTicker) Delay() time.Duration {
	return time.Duration(t.failures+1) * time.Second
}

type timeoutThenPolicyReviewReader struct {
	remaining io.Reader
	timedOut  bool
}

func (r *timeoutThenPolicyReviewReader) Read(value []byte) (int, error) {
	if !r.timedOut {
		r.timedOut = true
		return 0, nil
	}
	return r.remaining.Read(value)
}

func testPolicyReviewReport() tobari.PolicyCandidateReport {
	return tobari.PolicyCandidateReport{
		Task:                     tobari.TaskPolicyReview,
		PolicyProjectionIdentity: testCLIProjectionIdentity(strings.Repeat("a", 64)),
		WindowLines:              100,
		Items: []tobari.PolicyCandidate{
			{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, ID: "pcy_0123456789abcdef0123456789abcdef",
				ObservedAt: "2026-08-02T10:00:00Z", ObservationCount: 3,
				WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad", WorkspaceManifestName: "default",
				ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
				Host: "api.github.com", Port: 443, Method: "POST", Path: "/repos/example/issues",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
			{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, ID: "pcy_abcdef0123456789abcdef0123456789",
				ObservedAt: "2026-08-02T10:01:00Z", ObservationCount: 1,
				WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ae", WorkspaceManifestName: "restricted",
				ProjectID: "01912345-6789-7abc-8def-0123456789ac", ProjectRoot: "/workspace/project",
				Host: "registry.npmjs.org", Port: 443, Method: "GET", Path: "/package/example",
				Reason: "request did not match an allow rule", StatusCode: 403,
			},
		},
	}
}

func TestPolicyReviewSelectorRawDetailActionConfirmsAndPreservesOpaqueID(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(),
		strings.NewReader("\x1b[B\ra"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_abcdef0123456789abcdef0123456789" ||
		decision.Action != policyReviewActionAllow || decision.Canceled {
		t.Fatalf("decision = %+v", decision)
	}
	if !strings.Contains(output.String(), "Tobari · Permission Inbox") ||
		!strings.Contains(output.String(), "Permission 2 of 2") ||
		!strings.Contains(output.String(), "This decision applies only to this Workspace in this Workspace Manifest.") ||
		!strings.Contains(output.String(), "restricted") || !strings.Contains(output.String(), "/workspace/project") {
		t.Fatalf("rich review output = %q", output.String())
	}
	if strings.Contains(output.String(), "Type y") || strings.Contains(output.String(), "Allow this exact permission?") {
		t.Fatalf("detail action triggered a redundant confirmation: %q", output.String())
	}
	if !strings.Contains(output.String(), "\x1b[?25h") {
		t.Fatalf("cursor restore missing: %q", output.String())
	}
}

func TestPolicyReviewSelectorRawQuickStagesExactAndAdvancesWithoutWrap(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	report.Items[1].WorkspaceManifestID = report.Items[0].WorkspaceManifestID
	report.Items[1].WorkspaceManifestName = report.Items[0].WorkspaceManifestName
	report.Items[1].ProjectID = report.Items[0].ProjectID
	report.Items[1].ProjectRoot = report.Items[0].ProjectRoot
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: false, staged: map[string]policyReviewAction{}}
	var output bytes.Buffer

	first, err := selector.Select(context.Background(), report, strings.NewReader("a"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if first.CandidateID != report.Items[0].ID || first.SelectedID != report.Items[1].ID || first.Action != policyReviewActionAllow {
		t.Fatalf("first quick stage = %+v", first)
	}
	selector.Stage(first.CandidateID, first.Action)
	selector.selectedID = first.SelectedID

	second, err := selector.Select(context.Background(), report, strings.NewReader("d"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if second.CandidateID != report.Items[1].ID || second.SelectedID != report.Items[1].ID || second.Action != policyReviewActionDeny {
		t.Fatalf("second quick stage wrapped or chose wrong row: %+v", second)
	}
	selector.Stage(second.CandidateID, second.Action)
	selector.selectedID = second.SelectedID

	overwrite, err := selector.Select(context.Background(), report, strings.NewReader("a"), &output)
	if err != nil {
		t.Fatal(err)
	}
	selector.Stage(overwrite.CandidateID, overwrite.Action)
	if selector.staged[report.Items[1].ID] != policyReviewActionAllow || len(selector.stagedOrder) != 2 {
		t.Fatalf("overwrite duplicated or lost staging: staged=%+v order=%+v", selector.staged, selector.stagedOrder)
	}

	clear, err := selector.Select(context.Background(), report, strings.NewReader("x"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if !clear.Clear || clear.CandidateID != report.Items[1].ID {
		t.Fatalf("clear decision = %+v", clear)
	}
	selector.Clear(clear.CandidateID)
	if _, found := selector.staged[report.Items[1].ID]; found || len(selector.stagedOrder) != 1 {
		t.Fatalf("clear retained staging: staged=%+v order=%+v", selector.staged, selector.stagedOrder)
	}
	for _, want := range []string{"Pending", "Allow exact", "Deny exact", "a allow exact", "x clear", "Reason:"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("quick-stage output %q lacks %q", output.String(), want)
		}
	}
}

func TestPolicyReviewSelectorWatchEmptyWaitsAndRequestsRefresh(t *testing.T) {
	t.Parallel()
	ticker := &readyPolicyReviewTicker{ready: true}
	selector := &policyReviewSelector{
		mode: &selectorModeFake{}, style: false, staged: map[string]policyReviewAction{}, watch: true, ticker: ticker,
	}
	report := tobari.PolicyCandidateReport{Task: tobari.TaskPolicyReview, PolicyProjectionIdentity: testCLIProjectionIdentity(strings.Repeat("a", 64)), WindowLines: 10_000, UnparsedLines: 2, Items: []tobari.PolicyCandidate{}}
	var output bytes.Buffer
	decision, err := selector.Select(context.Background(), report, &timeoutThenPolicyReviewReader{remaining: strings.NewReader("q")}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Refresh || decision.Canceled {
		t.Fatalf("watch decision = %+v", decision)
	}
	for _, want := range []string{"No requests need review.", "Watching for denied requests…", "Press q to stop.", "2 denial-shaped Gateway lines skipped"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("watch empty output %q lacks %q", output.String(), want)
		}
	}
}

func TestPolicyReviewSelectorWatchKeepsOneUnchangedAlternateScreen(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &policyReviewSelector{
		mode: mode, style: false, staged: map[string]policyReviewAction{}, watch: true,
		ticker: &readyPolicyReviewTicker{ready: true},
	}
	report := testPolicyReviewReport()
	var output bytes.Buffer

	for refresh := 0; refresh < 2; refresh++ {
		decision, err := selector.Select(
			context.Background(), report,
			&timeoutThenPolicyReviewReader{remaining: strings.NewReader("q")}, &output,
		)
		if err != nil {
			t.Fatal(err)
		}
		if !decision.Refresh || !decision.AutomaticRefresh {
			t.Fatalf("refresh %d decision = %+v", refresh, decision)
		}
		selector.RefreshSucceeded()
	}
	decision, err := selector.Select(context.Background(), report, strings.NewReader("q"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled {
		t.Fatalf("stop decision = %+v", decision)
	}
	if got := strings.Count(output.String(), selectorAlternateScreenEnter); got != 1 {
		t.Fatalf("alternate-screen entries = %d, output=%q", got, output.String())
	}
	if got := strings.Count(output.String(), selectorAlternateScreenExit); got != 1 {
		t.Fatalf("alternate-screen exits = %d, output=%q", got, output.String())
	}
	if got := strings.Count(output.String(), "Tobari · Permission Inbox"); got != 1 {
		t.Fatalf("unchanged watch renders = %d, output=%q", got, output.String())
	}
	if mode.entered != 3 || mode.restored != 3 {
		t.Fatalf("raw mode calls = entered:%d restored:%d", mode.entered, mode.restored)
	}
}

func TestPolicyReviewSelectorWatchRedrawsChangedSnapshotInsideExistingScreen(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{
		mode: &selectorModeFake{}, style: false, staged: map[string]policyReviewAction{}, watch: true,
		ticker: &readyPolicyReviewTicker{ready: true},
	}
	report := testPolicyReviewReport()
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), report,
		&timeoutThenPolicyReviewReader{remaining: strings.NewReader("q")}, &output,
	)
	if err != nil || !decision.Refresh {
		t.Fatalf("initial refresh decision=%+v error=%v", decision, err)
	}
	selector.RefreshSucceeded()
	report.Items[0].ObservationCount++
	decision, err = selector.Select(context.Background(), report, strings.NewReader("q"), &output)
	if err != nil || !decision.Canceled {
		t.Fatalf("stop decision=%+v error=%v", decision, err)
	}
	if got := strings.Count(output.String(), selectorAlternateScreenEnter); got != 1 {
		t.Fatalf("alternate-screen entries = %d, output=%q", got, output.String())
	}
	if got := strings.Count(output.String(), "Tobari · Permission Inbox"); got != 2 {
		t.Fatalf("changed watch renders = %d, output=%q", got, output.String())
	}
}

func TestPolicyReviewSelectorWatchRequiresRawMode(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{
		mode: &selectorModeFake{enterErr: errors.New("raw unavailable")}, style: false,
		staged: map[string]policyReviewAction{}, watch: true, ticker: &readyPolicyReviewTicker{ready: true},
	}
	_, err := selector.Select(context.Background(), testPolicyReviewReport(), strings.NewReader("q"), io.Discard)
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "policy_review_watch_requires_tty" {
		t.Fatalf("watch raw-mode error = %v", err)
	}
}

func TestPolicyReviewSelectorWatchClosesExistingScreenWhenRawModeBecomesUnavailable(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &policyReviewSelector{
		mode: mode, style: false, staged: map[string]policyReviewAction{}, watch: true,
		ticker: &readyPolicyReviewTicker{ready: true},
	}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(),
		&timeoutThenPolicyReviewReader{remaining: strings.NewReader("q")}, &output,
	)
	if err != nil || !decision.Refresh {
		t.Fatalf("initial refresh decision=%+v error=%v", decision, err)
	}
	mode.enterErr = errors.New("raw unavailable after refresh")
	_, err = selector.Select(context.Background(), testPolicyReviewReport(), strings.NewReader("q"), &output)
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "policy_review_watch_requires_tty" {
		t.Fatalf("second raw-mode error = %v", err)
	}
	if got := strings.Count(output.String(), selectorAlternateScreenExit); got != 1 {
		t.Fatalf("screen exits=%d output=%q", got, output.String())
	}
	if selector.rendered || selector.screenLines != 0 {
		t.Fatalf("selector retained closed frame: %+v", selector)
	}
}

func TestPolicyReviewSelectorWatchRestoresRawModeOnWriterFailureAndCancellation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		ctx  func() context.Context
		out  io.Writer
	}{
		{
			name: "writer failure",
			ctx:  context.Background,
			out:  errorWriter{err: io.ErrClosedPipe},
		},
		{
			name: "cancellation",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			out: io.Discard,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			mode := &selectorModeFake{}
			selector := &policyReviewSelector{
				mode: mode, style: false, staged: map[string]policyReviewAction{},
				watch: true, ticker: &readyPolicyReviewTicker{ready: true},
			}
			_, err := selector.Select(test.ctx(), testPolicyReviewReport(), strings.NewReader("q"), test.out)
			if err == nil {
				t.Fatal("watch selector unexpectedly succeeded")
			}
			if mode.entered != 1 || mode.restored != 1 {
				t.Fatalf("raw mode calls = entered:%d restored:%d", mode.entered, mode.restored)
			}
		})
	}
}

func TestPolicyReviewRefreshTickerUsesInjectedClockAndBoundedBackoff(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	ticker := newIntervalPolicyReviewRefreshTicker()
	ticker.now = func() time.Time { return now }
	if ticker.Ready(context.Background()) {
		t.Fatal("new ticker refreshed before its first interval")
	}
	now = now.Add(policyReviewRefreshInterval)
	if !ticker.Ready(context.Background()) {
		t.Fatal("ticker did not refresh at the injected deadline")
	}
	for index := 0; index < 10; index++ {
		ticker.Failed()
	}
	if ticker.Delay() != policyReviewRefreshMaxBackoff {
		t.Fatalf("bounded backoff = %s, want %s", ticker.Delay(), policyReviewRefreshMaxBackoff)
	}
	ticker.Succeeded()
	if ticker.Delay() != policyReviewRefreshInterval {
		t.Fatalf("success delay = %s, want %s", ticker.Delay(), policyReviewRefreshInterval)
	}
}

func TestPolicyReviewRefreshKeepsSelectedIDWhenNewCandidateArrives(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	selector := &policyReviewSelector{staged: map[string]policyReviewAction{}, selectedID: report.Items[1].ID}
	selector.Stage(report.Items[1].ID, policyReviewActionDeny)
	newItem := report.Items[0]
	newItem.ID = "pcy_11111111111111111111111111111111"
	newItem.Path = "/new"
	fresh := report
	fresh.Items = append([]tobari.PolicyCandidate{newItem}, report.Items...)
	if removed := selector.Reconcile(fresh); removed != 0 {
		t.Fatalf("new arrival removed %d staged decisions", removed)
	}
	grouped := groupPolicyReviewReport(fresh)
	selected := policyReviewSelectedIndex(grouped.Items, selector.selectedID)
	if grouped.Items[selected].ID != report.Items[1].ID || selector.staged[report.Items[1].ID] != policyReviewActionDeny {
		t.Fatalf("new arrival stole selection or staging: selected=%q staged=%+v", grouped.Items[selected].ID, selector.staged)
	}
}

func TestPolicyReviewSelectorRawTemplateDetailOffersExplicitFutureAndExactChoices(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	makeCandidate := func(path, timestamp, requestID string) tobari.PolicyCandidate {
		candidate, candidateErr := tobari.NewPolicyCandidate(tobari.PolicyDenial{
			PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
			Timestamp:              timestamp, RequestID: requestID,
			WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad", WorkspaceManifestName: "default",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/project",
			Host: "api.example.com", Port: 443, Method: "GET", Path: path,
			Reason: "request did not match an allow rule", StatusCode: 403, Learnable: true,
		})
		if candidateErr != nil {
			t.Fatal(candidateErr)
		}
		return candidate
	}
	first := makeCandidate("/items/123", "2026-08-02T10:00:00Z", "7185da2688d7469aae9cd9068e920b0b")
	second := makeCandidate("/items/456", "2026-08-02T10:01:00Z", "8185da2688d7469aae9cd9068e920b0b")
	report.Items = []tobari.PolicyCandidate{first, second}
	var err error
	report.ReviewItems, err = tobari.PolicyReviewItems(report.Items, []tobari.LearnedPolicyRule{})
	if err != nil || len(report.ReviewItems) != 1 || report.ReviewItems[0].Template == nil {
		t.Fatalf("review items = %+v, error = %v", report.ReviewItems, err)
	}
	var pendingList, stagedList bytes.Buffer
	if lines := renderPolicyReviewListRaw(&pendingList, report, 0, 0, "", 0, false); lines <= 0 {
		t.Fatalf("pending template list lines = %d, output = %q", lines, pendingList.String())
	}
	if lines := renderPolicyReviewListRaw(
		&stagedList, report, 0, 0, "", 0, false,
		map[string]policyReviewAction{report.ReviewItems[0].ID: policyReviewActionAllowTemplate},
	); lines <= 0 {
		t.Fatalf("staged template list lines = %d, output = %q", lines, stagedList.String())
	}
	if !strings.Contains(pendingList.String(), "Review {id}  GET") ||
		!strings.Contains(stagedList.String(), "Allow {id}   GET") {
		t.Fatalf("compact template states = pending:%q staged:%q", pendingList.String(), stagedList.String())
	}
	if strings.Contains(pendingList.String(), "Pending · Suggested") ||
		strings.Contains(stagedList.String(), "Allow template") {
		t.Fatalf("list retained long template states = pending:%q staged:%q", pendingList.String(), stagedList.String())
	}

	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(context.Background(), report, strings.NewReader("\rt"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if decision.CandidateID != report.ReviewItems[0].ID || decision.Action != policyReviewActionAllowTemplate {
		t.Fatalf("template decision = %+v", decision)
	}
	for _, want := range []string{"Review {id}", "/items/{id}", "2 distinct values", "[t] Allow template", "[e] Allow observed exact", "future non-empty values"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("template raw output %q lacks %q", output.String(), want)
		}
	}
}

func TestPolicyReviewSelectorBackThenCancelDoesNotSelect(t *testing.T) {
	t.Parallel()
	mode := &selectorModeFake{}
	selector := &policyReviewSelector{mode: mode, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("\rqq"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" {
		t.Fatalf("decision = %+v", decision)
	}
	if mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("raw mode calls = entered:%d restored:%d", mode.entered, mode.restored)
	}
	if strings.Contains(output.String(), "Permission allowed") {
		t.Fatalf("cancel output contains an allow result: %q", output.String())
	}
}

func TestPolicyReviewSelectorRawUsesSemanticColor(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()

	var listOutput bytes.Buffer
	if lines := renderPolicyReviewListRaw(&listOutput, report, 0, 0, "", 0, true); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, listOutput.String())
	}
	for _, want := range []string{
		applyStyleToken(true, styleAccent, "Tobari · Permission Inbox"),
		applyStyleToken(true, styleWarning, "2 pending permissions in 2 Workspaces"),
		applyStyleToken(true, styleText, "default · /workspace/project"),
		"POST   https://api.github.com:443/repos/example/issues",
		applyStyleToken(true, styleMuted, "3×"),
		applyStyleToken(true, styleMuted, "Selected"),
		"Latest 2026-08-02T10:00:00Z",
		"❯ ",
	} {
		if !strings.Contains(listOutput.String(), want) {
			t.Fatalf("colored list output %q lacks %q", listOutput.String(), want)
		}
	}

	var detailOutput bytes.Buffer
	if lines := renderPolicyReviewDetailRaw(&detailOutput, report, 0, "", 0, true); lines <= 0 {
		t.Fatalf("detail render lines = %d, output = %q", lines, detailOutput.String())
	}
	for _, want := range []string{
		applyStyleToken(true, styleAccent, "Permission 1 of 2"),
		"https://api.github.com:443 POST /repos/example/issues",
		applyStyleToken(true, styleDanger, "403"),
		"3 times",
		"2026-08-02T10:00:00Z",
		applyStyleToken(true, styleAccent, "[a] Allow exact"),
		applyStyleToken(true, styleAccent, "[d] Deny exact"),
		applyStyleToken(true, styleMuted, "[q] Back"),
	} {
		if !strings.Contains(detailOutput.String(), want) {
			t.Fatalf("colored detail output %q lacks %q", detailOutput.String(), want)
		}
	}

	var finalOutput bytes.Buffer
	staged := map[string]policyReviewAction{
		report.Items[0].ID: policyReviewActionAllow,
		report.Items[1].ID: policyReviewActionDeny,
	}
	stagedOrder := []string{report.Items[0].ID, report.Items[1].ID}
	if lines := renderPolicyReviewFinalRaw(&finalOutput, report, staged, stagedOrder, "", 0, true); lines <= 0 {
		t.Fatalf("final render lines = %d, output = %q", lines, finalOutput.String())
	}
	for _, want := range []string{
		applyStyleToken(true, styleSuccess, "1. Allow exact"),
		applyStyleToken(true, styleWarning, "2. Deny exact"),
		applyStyleToken(true, styleAccent, "[y] Apply"),
	} {
		if !strings.Contains(finalOutput.String(), want) {
			t.Fatalf("colored final output %q lacks %q", finalOutput.String(), want)
		}
	}
}

func TestPolicyReviewSelectorGroupsByStableScopeAndKeepsEffectOrderWithinGroup(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	first := report.Items[0]
	secondInFirstScope := first
	secondInFirstScope.ID = "pcy_11111111111111111111111111111111"
	secondInFirstScope.ObservedAt = "2026-08-02T10:02:00Z"
	secondInFirstScope.ObservationCount = 2
	secondInFirstScope.Method = "GET"
	secondInFirstScope.Path = "/notifications"
	report.Items = []tobari.PolicyCandidate{first, report.Items[1], secondInFirstScope}

	display := groupPolicyReviewReport(report)
	if got := []string{display.Items[0].ID, display.Items[1].ID, display.Items[2].ID}; got[0] != first.ID || got[1] != secondInFirstScope.ID || got[2] != report.Items[1].ID {
		t.Fatalf("grouped order = %v", got)
	}

	var output bytes.Buffer
	if lines := renderPolicyReviewListRaw(&output, report, 1, 0, "", 0, false); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, output.String())
	}
	text := output.String()
	if strings.Count(text, "default · /workspace/project") != 1 ||
		strings.Count(text, "restricted · /workspace/project") != 1 {
		t.Fatalf("scope headings were not grouped once: %q", text)
	}
	firstEffect := strings.Index(text, "POST   https://api.github.com:443/repos/example/issues")
	secondEffect := strings.Index(text, "GET    https://api.github.com:443/notifications")
	restricted := strings.Index(text, "restricted · /workspace/project")
	if firstEffect < 0 || secondEffect <= firstEffect || restricted <= secondEffect {
		t.Fatalf("effect order is not stable within grouped scopes: %q", text)
	}
	for _, want := range []string{
		"3 pending permissions in 2 Workspaces",
		"Selected",
		"GET https://api.github.com:443/notifications",
		"Observed 2 times · Latest 2026-08-02T10:02:00Z",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("grouped list %q lacks %q", text, want)
		}
	}
}

func TestPolicyReviewSelectorKeepsEffectColumnFixedAcrossStateCombinations(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	base := report.Items[0]
	base.Method = "GET"
	base.Host = "google.com"
	base.Path = "/"
	base.ObservationCount = 3
	pending := base
	pending.ID = "pcy_11111111111111111111111111111111"
	pending.Host = "google2.com"
	pending.ObservationCount = 5
	allowed := base
	allowed.ID = "pcy_22222222222222222222222222222222"
	allowed.Host = "google3.com"
	allowed.ObservationCount = 1
	report.Items = []tobari.PolicyCandidate{base, pending, allowed}
	report.ReviewItems = nil
	firstStaged := map[string]policyReviewAction{
		base.ID:    policyReviewActionDeny,
		allowed.ID: policyReviewActionAllow,
	}
	secondStaged := map[string]policyReviewAction{
		base.ID:    policyReviewActionDeny,
		pending.ID: policyReviewActionDeny,
	}

	var firstOutput, secondOutput bytes.Buffer
	if lines := renderPolicyReviewListRaw(&firstOutput, report, 1, 0, "", 0, false, firstStaged); lines <= 0 {
		t.Fatalf("first list render lines = %d, output = %q", lines, firstOutput.String())
	}
	if lines := renderPolicyReviewListRaw(&secondOutput, report, 2, 0, "", 0, false, secondStaged); lines <= 0 {
		t.Fatalf("second list render lines = %d, output = %q", lines, secondOutput.String())
	}

	effectRows := func(output string) []string {
		rows := make([]string, 0, len(report.Items))
		for _, line := range strings.Split(output, "\n") {
			if strings.Contains(line, "https://google") && strings.Contains(line, "×") {
				rows = append(rows, line)
			}
		}
		return rows
	}
	firstRows := effectRows(firstOutput.String())
	secondRows := effectRows(secondOutput.String())
	if len(firstRows) != 3 || len(secondRows) != 3 {
		t.Fatalf("effect rows = first:%q second:%q, want 3 rows each", firstRows, secondRows)
	}
	methodColumn := utf8.RuneCountInString(firstRows[0][:strings.Index(firstRows[0], "GET")])
	for _, row := range append(firstRows, secondRows...) {
		if column := utf8.RuneCountInString(row[:strings.Index(row, "GET")]); column != methodColumn {
			t.Fatalf("effect column moved from %d to %d: first=%q second=%q", methodColumn, column, firstRows, secondRows)
		}
	}
	for _, want := range []string{
		"    Deny exact   GET",
		"  ❯ Pending      GET",
		"    Allow exact  GET",
	} {
		if !strings.Contains(firstOutput.String(), want) {
			t.Fatalf("fixed-width list output %q lacks %q", firstOutput.String(), want)
		}
	}
}

func TestPolicyReviewSelectorDoesNotGroupMatchingDisplayLabelsAcrossTypedScopes(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	report.Items[1].WorkspaceManifestName = report.Items[0].WorkspaceManifestName
	report.Items[1].ProjectRoot = report.Items[0].ProjectRoot

	var output bytes.Buffer
	if lines := renderPolicyReviewListRaw(&output, report, 0, 0, "", 0, false); lines <= 0 {
		t.Fatalf("list render lines = %d, output = %q", lines, output.String())
	}
	if got := strings.Count(output.String(), "default · /workspace/project"); got != 2 {
		t.Fatalf("matching display labels produced %d headings, want 2: %q", got, output.String())
	}
	if !strings.Contains(output.String(), "2 pending permissions in 2 Workspaces") {
		t.Fatalf("typed scope count was inferred from labels: %q", output.String())
	}
}

func TestPolicyReviewSelectorFinishRestoresMainScreenWithoutLogicalRowMovement(t *testing.T) {
	t.Parallel()
	var reviewOutput, workspaceOutput bytes.Buffer

	finishPolicyReviewSelector(&reviewOutput, 3)
	finishWorkspaceSelector(&workspaceOutput, 3)

	want := selectorAlternateScreenExit + selectorCursorShow
	if reviewOutput.String() != want {
		t.Fatalf("review permissions finish = %q, want %q", reviewOutput.String(), want)
	}
	if workspaceOutput.String() != reviewOutput.String() {
		t.Fatalf("selector finish differs: workspace=%q review=%q", workspaceOutput.String(), reviewOutput.String())
	}
	if strings.Contains(reviewOutput.String(), "\n") || strings.Contains(reviewOutput.String(), "\x1b[3A") {
		t.Fatalf("selector finish writes or moves by logical rows: %q", reviewOutput.String())
	}
}

func TestPolicyReviewSelectorLineDetailActionConfirmsExactAllow(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("2\na\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_abcdef0123456789abcdef0123456789" || decision.Action != policyReviewActionAllow {
		t.Fatalf("decision = %+v", decision)
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("line fallback contains terminal controls: %q", output.String())
	}
	for _, want := range []string{"1.", "2.", "Choose [a] to allow exact", "Workspace Manifest   restricted"} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("line review output %q lacks %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "[y/N]") || strings.Contains(output.String(), "Allow this exact permission?") {
		t.Fatalf("line action triggered a redundant confirmation: %q", output.String())
	}
}

func TestPolicyReviewSelectorLineDetailActionConfirmsExactDeny(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader("1\nd\n"), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if decision.CandidateID != "pcy_0123456789abcdef0123456789abcdef" ||
		decision.Action != policyReviewActionDeny || decision.Canceled {
		t.Fatalf("decision = %+v", decision)
	}
	if !strings.Contains(output.String(), "Choose [a] to allow exact") || !strings.Contains(output.String(), "Workspace Manifest   default") {
		t.Fatalf("deny detail action missing exact scope: %q", output.String())
	}
	if strings.Contains(output.String(), "[y/N]") || strings.Contains(output.String(), "Deny this exact permission?") {
		t.Fatalf("deny action triggered a redundant confirmation: %q", output.String())
	}
}

func TestPolicyReviewSelectorRawEOFIsSafeCancellation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader(""), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" || decision.Action != policyReviewActionNone {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPolicyReviewSelectorEOFIsSafeCancellation(t *testing.T) {
	t.Parallel()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: true}
	var output bytes.Buffer
	decision, err := selector.Select(
		context.Background(), testPolicyReviewReport(), strings.NewReader(""), &output,
	)
	if err != nil {
		t.Fatalf("Select() error = %v", err)
	}
	if !decision.Canceled || decision.CandidateID != "" {
		t.Fatalf("decision = %+v", decision)
	}
}

func TestPolicyReviewSelectorLineShowsTypedStagedStateAndDoesNotAdvertiseEmptyApply(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	selector := &policyReviewSelector{mode: &selectorModeFake{enterErr: errors.New("raw mode unavailable")}, style: false}
	var emptyOutput bytes.Buffer
	decision, err := selector.Select(
		context.Background(), report, strings.NewReader("p\nq\n"), &emptyOutput,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled || decision.Apply {
		t.Fatalf("empty Apply was accepted: %+v", decision)
	}
	if strings.Contains(emptyOutput.String(), "apply staged") {
		t.Fatalf("empty Apply was advertised: %q", emptyOutput.String())
	}

	selector.Stage(report.Items[0].ID, policyReviewActionAllow)
	selector.Stage(report.Items[1].ID, policyReviewActionDeny)
	var stagedOutput bytes.Buffer
	decision, err = selector.Select(
		context.Background(), report, strings.NewReader("q\n"), &stagedOutput,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled {
		t.Fatalf("staged cancellation = %+v", decision)
	}
	for _, want := range []string{"Staged Allow", "Staged Deny", "review staged"} {
		if !strings.Contains(stagedOutput.String(), want) {
			t.Fatalf("staged list %q lacks %q", stagedOutput.String(), want)
		}
	}
}

func TestPolicyReviewSelectorKeepsSelectionAndStageByCandidateIDAcrossReorder(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	wantID := report.Items[1].ID
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: false}
	var first bytes.Buffer
	decision, err := selector.Select(context.Background(), report, strings.NewReader("\x1b[B\ra"), &first)
	if err != nil {
		t.Fatal(err)
	}
	if decision.CandidateID != wantID || decision.SelectedID != wantID {
		t.Fatalf("selected decision = %+v", decision)
	}
	selector.Stage(decision.CandidateID, decision.Action)

	reordered := report
	reordered.Items = []tobari.PolicyCandidate{report.Items[1], report.Items[0]}
	var second bytes.Buffer
	decision, err = selector.Select(context.Background(), reordered, strings.NewReader("q"), &second)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled || decision.SelectedID != wantID {
		t.Fatalf("reordered selection = %+v", decision)
	}
	for _, want := range []string{"Staged Allow", "GET    https://registry.npmjs.org:443/package/example", "Selected", "GET https://registry.npmjs.org:443/package/example"} {
		if !strings.Contains(second.String(), want) {
			t.Fatalf("reordered output %q lacks %q", second.String(), want)
		}
	}

	removed := report
	removed.Items = []tobari.PolicyCandidate{report.Items[0]}
	if got := selector.Reconcile(removed); got != 1 {
		t.Fatalf("removed staged count = %d, want 1", got)
	}
	var fallback bytes.Buffer
	decision, err = selector.Select(context.Background(), removed, strings.NewReader("q"), &fallback)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled || decision.SelectedID != removed.Items[0].ID {
		t.Fatalf("fallback selection = %+v", decision)
	}
	if strings.Contains(fallback.String(), "❯ Staged Allow") || !strings.Contains(fallback.String(), "Selected") ||
		!strings.Contains(fallback.String(), "POST https://api.github.com:443/repos/example/issues") {
		t.Fatalf("fallback output = %q", fallback.String())
	}
}

func TestPolicyReviewSelectorReconcileDoesNotTransferStageToMatchingLabels(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	selector := &policyReviewSelector{}
	selector.Stage(report.Items[0].ID, policyReviewActionDeny)

	replacement := report.Items[0]
	replacement.ID = "pcy_11111111111111111111111111111111"
	refreshed := report
	refreshed.Items = []tobari.PolicyCandidate{replacement, report.Items[1]}
	if got := selector.Reconcile(refreshed); got != 1 {
		t.Fatalf("removed count = %d, want 1", got)
	}
	if got := selector.OrderedDecisions(); len(got) != 0 {
		t.Fatalf("stage transferred to matching labels: %+v", got)
	}
}

func TestPolicyReviewSelectorFinalReviewRequiresExplicitConfirmation(t *testing.T) {
	t.Parallel()
	report := testPolicyReviewReport()
	selector := &policyReviewSelector{mode: &selectorModeFake{}, style: false}
	selector.Stage(report.Items[0].ID, policyReviewActionAllow)
	selector.Stage(report.Items[1].ID, policyReviewActionDeny)

	var canceled bytes.Buffer
	decision, err := selector.Select(context.Background(), report, strings.NewReader("pq"), &canceled)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled || decision.Apply {
		t.Fatalf("final cancel = %+v", decision)
	}
	for _, want := range []string{"Review staged permissions", report.Items[0].ID, report.Items[1].ID, "Staging grants no authority", "[y] Apply"} {
		if !strings.Contains(canceled.String(), want) {
			t.Fatalf("final review %q lacks %q", canceled.String(), want)
		}
	}

	var backed bytes.Buffer
	decision, err = selector.Select(context.Background(), report, strings.NewReader("pbq"), &backed)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Canceled || decision.Apply {
		t.Fatalf("back then cancel = %+v", decision)
	}

	var confirmed bytes.Buffer
	decision, err = selector.Select(context.Background(), report, strings.NewReader("py"), &confirmed)
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Apply || decision.Canceled {
		t.Fatalf("explicit confirmation = %+v", decision)
	}
}
