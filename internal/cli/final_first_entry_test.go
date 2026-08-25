package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type firstEntryProcessLifetimeMarker struct{}

type firstEntryPairFixture struct {
	order          *[]string
	observation    tobari.FinalDefaultPairObservation
	resolution     workspaceauthoritycmd.DefaultPairResolution
	resolveBody    *tobari.WorkspaceTemplateBody
	session        tobari.WorkspaceSessionRequest
	outcome        tobari.WorkspaceSessionOutcome
	resolveCalls   int
	refreshCalls   int
	refreshContext context.Context
	refreshCtxErr  error
	entryCalls     int
	cancelAt       string
	cancel         context.CancelFunc
	resolveErr     error
	refreshErr     error
	entryErr       error
}

func (f *firstEntryPairFixture) Observe(context.Context) (tobari.FinalDefaultPairObservation, error) {
	*f.order = append(*f.order, "observe")
	return f.observation.Clone(), nil
}

func (f *firstEntryPairFixture) Resolve(_ context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.resolveCalls++
	*f.order = append(*f.order, "resolve")
	if f.cancelAt == "resolve" && f.cancel != nil {
		f.cancel()
	}
	if intent.Command != WorkspaceEntryCommandPath || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.CurrentDirectoryTargetKind || intent.Target.ParentID != tobari.CurrentDirectoryTargetID {
		panic("root resolution lost its Catalog mutation binding")
	}
	f.resolveBody = body
	return f.resolution, f.resolveErr
}

func (f *firstEntryPairFixture) RefreshAfterCluster(ctx context.Context, resolution workspaceauthoritycmd.DefaultPairResolution, _ workspaceauthoritycmd.FinalClusterReconciliation) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.refreshCalls++
	f.refreshContext = ctx
	f.refreshCtxErr = ctx.Err()
	*f.order = append(*f.order, "refresh")
	if f.cancelAt == "refresh" && f.cancel != nil {
		f.cancel()
	}
	return resolution, f.refreshErr
}

func (f *firstEntryPairFixture) EnterResolved(_ context.Context, _ workspaceauthoritycmd.DefaultPairResolution, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, _ io.Reader, _, _ io.Writer) (workspaceauthoritycmd.ContextEntryResult, error) {
	f.entryCalls++
	*f.order = append(*f.order, "entry")
	f.session = session
	if f.cancelAt == "entry" && f.cancel != nil {
		f.cancel()
	}
	if f.entryErr != nil {
		return workspaceauthoritycmd.ContextEntryResult{}, f.entryErr
	}
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageSucceeded})
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageRunning})
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageSucceeded})
	return workspaceauthoritycmd.ContextEntryResult{Outcome: f.outcome}, nil
}

type firstEntryReadinessFixture struct {
	order  *[]string
	calls  int
	err    error
	cancel context.CancelFunc
}

func (f *firstEntryReadinessFixture) Check(context.Context) error {
	f.calls++
	*f.order = append(*f.order, "readiness")
	if f.cancel != nil {
		f.cancel()
	}
	return f.err
}

type firstEntryClusterFixture struct {
	order  *[]string
	calls  int
	cancel context.CancelFunc
	err    error
}

func (f *firstEntryClusterFixture) Reconcile(_ context.Context, intent operation.Intent) (workspaceauthoritycmd.FinalClusterReconciliation, error) {
	f.calls++
	*f.order = append(*f.order, "cluster")
	if f.cancel != nil {
		f.cancel()
	}
	if intent.Command != "cluster up" || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.ClusterTargetKind || intent.Target.ParentID != tobari.ClusterTargetID {
		panic("root cluster call did not consume the canonical Catalog contract")
	}
	return workspaceauthoritycmd.FinalClusterReconciliation{}, f.err
}

type firstEntryReviewerFixture struct {
	order  *[]string
	action recommendedFirstUseAction
	calls  int
}

func (f *firstEntryReviewerFixture) Review(context.Context, tobari.RecommendedFirstUseDraft, io.Reader, io.Writer) (recommendedFirstUseAction, error) {
	f.calls++
	*f.order = append(*f.order, "review")
	return f.action, nil
}

func newFirstEntryCLI(t *testing.T, fresh, interactive bool, action recommendedFirstUseAction) (*CLI, *firstEntryPairFixture, *firstEntryReadinessFixture, *firstEntryClusterFixture, *firstEntryReviewerFixture, *bytes.Buffer, *bytes.Buffer, *[]string) {
	t.Helper()
	order := []string{}
	observation := tobari.FinalDefaultPairObservation{SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, ProjectRoot: "/workspace/example", CollectionPresent: !fresh}
	pair := &firstEntryPairFixture{order: &order, observation: observation, outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}
	readiness := &firstEntryReadinessFixture{order: &order}
	cluster := &firstEntryClusterFixture{order: &order}
	reviewer := &firstEntryReviewerFixture{order: &order, action: action}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.processLifetime = context.WithValue(context.Background(), firstEntryProcessLifetimeMarker{}, "injected")
	command.finalDefaultPair = pair
	command.finalEntryReadiness = readiness
	command.finalCluster = cluster
	command.firstUse = reviewer
	command.firstUseInteractive = func(io.Reader, io.Writer, io.Writer) bool { return interactive }
	command.firstUseTemplateBody = func(context.Context) (tobari.WorkspaceTemplateBody, error) {
		order = append(order, "template body")
		return tobari.WorkspaceTemplateBody{}, nil
	}
	command.firstUseCustomize = func(context.Context, tobari.RecommendedFirstUseDraft) (tobari.WorkspaceTemplateBody, error) {
		order = append(order, "customize")
		return tobari.WorkspaceTemplateBody{}, nil
	}
	return command, pair, readiness, cluster, reviewer, &stdout, &stderr, &order
}

func TestFinalRootFreshStartComposesReviewFiveCheckpointsAndExactDirectArgv(t *testing.T) {
	command, pair, readiness, cluster, reviewer, stdout, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseStart)
	argv := []string{"claude", "--model", "", "--flag=-value"}
	if code := command.RunContext(context.Background(), append([]string{"--"}, argv...)); code != ExitOK {
		t.Fatalf("root exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "review", "readiness", "template body", "resolve", "cluster", "refresh", "entry"}) {
		t.Fatalf("root order = %v", *order)
	}
	if reviewer.calls != 1 || readiness.calls != 1 || cluster.calls != 1 || pair.resolveCalls != 1 || pair.refreshCalls != 1 || pair.entryCalls != 1 || pair.resolveBody == nil {
		t.Fatalf("root calls reviewer=%d readiness=%d cluster=%d resolve=%d refresh=%d entry=%d body=%v", reviewer.calls, readiness.calls, cluster.calls, pair.resolveCalls, pair.refreshCalls, pair.entryCalls, pair.resolveBody)
	}
	if !pair.session.Direct() || !reflect.DeepEqual(pair.session.Argv(), argv) {
		t.Fatalf("direct argv = %#v", pair.session.Argv())
	}
	wantProgress := "✓ Check requirements\n✓ Save setup\n✓ Prepare protection\n✓ Prepare Workspace\n✓ Enter Workspace\n"
	if stderr.String() != wantProgress || stdout.Len() != 0 {
		t.Fatalf("root streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootExistingPairSkipsReviewAndFreshBody(t *testing.T) {
	command, pair, _, _, reviewer, _, stderr, order := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("existing root exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "readiness", "resolve", "cluster", "refresh", "entry"}) || reviewer.calls != 0 || pair.resolveBody != nil {
		t.Fatalf("existing root order=%v review=%d body=%v", *order, reviewer.calls, pair.resolveBody)
	}
	if !strings.Contains(stderr.String(), "✓ Use Context\n") || strings.Contains(stderr.String(), "Save setup") {
		t.Fatalf("existing progress = %q", stderr.String())
	}
}

func TestFinalRootCustomizePublishesOnlyTheReviewedBody(t *testing.T) {
	command, pair, _, _, _, _, stderr, order := newFirstEntryCLI(t, true, true, recommendedFirstUseCustomize)
	if code := command.RunContext(context.Background(), nil); code != ExitOK {
		t.Fatalf("customized root exit=%d stderr=%q", code, stderr.String())
	}
	if !reflect.DeepEqual(*order, []string{"observe", "review", "customize", "readiness", "resolve", "cluster", "refresh", "entry"}) || pair.resolveBody == nil {
		t.Fatalf("customized root order=%v body=%v", *order, pair.resolveBody)
	}
}

func TestFinalRootFreshNoninteractiveAndCancelMakeZeroReadinessOrMutation(t *testing.T) {
	for _, test := range []struct {
		name        string
		interactive bool
		action      recommendedFirstUseAction
		want        int
	}{
		{name: "redirected", interactive: false, action: recommendedFirstUseStart, want: ExitRejected},
		{name: "cancel", interactive: true, action: recommendedFirstUseCancel, want: ExitCanceled},
	} {
		t.Run(test.name, func(t *testing.T) {
			command, pair, readiness, cluster, reviewer, stdout, stderr, _ := newFirstEntryCLI(t, true, test.interactive, test.action)
			if code := command.RunContext(context.Background(), nil); code != test.want {
				t.Fatalf("exit=%d want=%d stderr=%q", code, test.want, stderr.String())
			}
			if readiness.calls != 0 || cluster.calls != 0 || pair.resolveCalls != 0 || pair.entryCalls != 0 || stdout.Len() != 0 {
				t.Fatalf("redirected/cancel crossed setup: readiness=%d cluster=%d resolve=%d entry=%d stdout=%q", readiness.calls, cluster.calls, pair.resolveCalls, pair.entryCalls, stdout.String())
			}
			if test.interactive && reviewer.calls != 1 || !test.interactive && reviewer.calls != 0 {
				t.Fatalf("review calls = %d", reviewer.calls)
			}
		})
	}
}

func TestFinalRootCallerCancellationBeforeHandoffExits130(t *testing.T) {
	command, pair, readiness, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	readiness.cancel = cancel
	readiness.err = context.Canceled
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("canceled root exit=%d stderr=%q", code, stderr.String())
	}
	if pair.resolveCalls != 0 || cluster.calls != 0 || pair.entryCalls != 0 {
		t.Fatalf("canceled root crossed mutation: resolve=%d cluster=%d entry=%d", pair.resolveCalls, cluster.calls, pair.entryCalls)
	}
}

func TestFinalRootCallerCancellationAtEachCanonicalPreHandoffBoundaryExits130(t *testing.T) {
	for _, boundary := range []string{"resolve", "cluster", "refresh", "entry"} {
		t.Run(boundary, func(t *testing.T) {
			command, pair, _, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch boundary {
			case "resolve":
				pair.cancelAt, pair.cancel, pair.resolveErr = boundary, cancel, context.Canceled
			case "cluster":
				cluster.cancel, cluster.err = cancel, context.Canceled
			case "refresh":
				pair.cancelAt, pair.cancel, pair.refreshErr = boundary, cancel, context.Canceled
			case "entry":
				pair.cancelAt, pair.cancel, pair.entryErr = boundary, cancel, context.Canceled
			}
			if code := command.RunContext(ctx, nil); code != ExitInterrupted {
				t.Fatalf("%s cancellation exit=%d stderr=%q", boundary, code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "operation_canceled") || !strings.Contains(stderr.String(), "Canceled") {
				t.Fatalf("%s cancellation classification=%q", boundary, stderr.String())
			}
		})
	}
}

func TestFinalRootCallerCancellationPreservesUnknownMutationClassificationAndExits130(t *testing.T) {
	command, _, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster.cancel = cancel
	cluster.err = fault.WithClassification(
		fault.New(fault.KindContract, "unclassified_mutation_outcome", "synthetic unknown cluster outcome", false),
		fault.PhaseMutation, fault.ChangeUnknown,
	)
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("unknown cancellation exit=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "unclassified_mutation_outcome") || !humanOutputHasRow(stderr.String(), "Change state", "unknown") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("unknown cancellation stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootLateCancellationAfterResolutionPreservesConfirmedAuthorityAndStops(t *testing.T) {
	command, pair, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pair.cancelAt, pair.cancel = "resolve", cancel
	pair.resolution.AuthorityChanged = true
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("late resolution cancellation exit=%d stderr=%q", code, stderr.String())
	}
	if cluster.calls != 0 || pair.refreshCalls != 0 || pair.entryCalls != 0 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "default_pair_initialized") || !humanOutputHasRow(stderr.String(), "Change state", "partial") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("late resolution cancellation cluster=%d refresh=%d entry=%d stdout=%q stderr=%q", cluster.calls, pair.refreshCalls, pair.entryCalls, stdout.String(), stderr.String())
	}
}

func TestFinalRootLateCancellationAfterClusterUsesBoundedClassificationAndStopsBeforeEntry(t *testing.T) {
	command, pair, _, cluster, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cluster.cancel = cancel
	if code := command.RunContext(ctx, nil); code != ExitInterrupted {
		t.Fatalf("late cluster cancellation exit=%d stderr=%q", code, stderr.String())
	}
	var deadline time.Time
	var hasDeadline bool
	var settlementParent any
	if pair.refreshContext != nil {
		deadline, hasDeadline = pair.refreshContext.Deadline()
		settlementParent = pair.refreshContext.Value(firstEntryProcessLifetimeMarker{})
	}
	remaining := time.Until(deadline)
	if pair.refreshCalls != 1 || pair.refreshCtxErr != nil || pair.entryCalls != 0 || stdout.Len() != 0 ||
		!hasDeadline || remaining <= 0 || remaining > firstEntryClassificationTimeout || settlementParent != "injected" ||
		!strings.Contains(stderr.String(), "default_pair_initialized") || !humanOutputHasRow(stderr.String(), "Change state", "partial") || !humanOutputHasRow(stderr.String(), "Retryable", "no") {
		t.Fatalf("late cluster cancellation refresh=%d refresh_ctx=%v deadline=%v has_deadline=%v remaining=%v parent=%v entry=%d stdout=%q stderr=%q", pair.refreshCalls, pair.refreshCtxErr, deadline, hasDeadline, remaining, settlementParent, pair.entryCalls, stdout.String(), stderr.String())
	}
}

func TestFinalRootPreservesCanonicalClusterRecoveryFault(t *testing.T) {
	command, pair, _, cluster, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	cluster.err = fault.New(fault.KindUnavailable, "cluster_reconcile_interrupted", "synthetic interrupted cluster reconcile", false)
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("cluster fault exit=%d stderr=%q", code, stderr.String())
	}
	if pair.entryCalls != 0 || !strings.Contains(stderr.String(), "cluster_reconcile_interrupted") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("cluster recovery was not preserved: entry=%d stderr=%q", pair.entryCalls, stderr.String())
	}
}

func TestFinalRootPreservesCanonicalWorkspaceEntryRecoveryFault(t *testing.T) {
	command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	pair.entryErr = fault.WithClassification(
		fault.New(fault.KindUnavailable, "workspace_entry_interrupted", "synthetic interrupted Workspace entry", false),
		fault.PhaseMutation, fault.ChangePartial,
	)
	if code := command.RunContext(context.Background(), nil); code != ExitUnavailable {
		t.Fatalf("entry fault exit=%d stderr=%q", code, stderr.String())
	}
	spec, found := command.catalog.Lookup("status")
	if !found {
		t.Fatal("status recovery command is absent from the Catalog")
	}
	expectedInvocation := recoveryCommandForProgram(spec.programName(), spec.Path)
	if !strings.Contains(stderr.String(), "workspace_entry_interrupted") || !evaluatorNextHasInvocation(stderr.String(), expectedInvocation) || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("entry recovery was not preserved: stderr=%q", stderr.String())
	}
}

func TestFinalRootPreservesChildStatusAndReportsSecondaryCleanup(t *testing.T) {
	command, pair, _, _, _, stdout, stderr, _ := newFirstEntryCLI(t, false, false, recommendedFirstUseStart)
	pair.outcome = tobari.WorkspaceSessionOutcome{ExitCode: 23, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{tobari.WorkspaceCleanupHostLoopback}}
	if code := command.RunContext(context.Background(), []string{"--", "claude"}); code != 23 {
		t.Fatalf("child exit=%d stderr=%q", code, stderr.String())
	}
	if stdout.Len() != 0 || strings.Count(stderr.String(), "Tobari cleanup needs attention") != 1 || !strings.Contains(stderr.String(), "Next: tobari status") || strings.Contains(stderr.String(), "Command failed") {
		t.Fatalf("child boundary stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestFinalRootShowsWorkspaceOwnedCredentialHintOnlyForFreshShell(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		fresh          bool
		freshWorkspace bool
		want           int
	}{
		{name: "fresh shell", fresh: true, freshWorkspace: true, want: 1},
		{name: "fresh direct command", args: []string{"--", "claude"}, fresh: true, freshWorkspace: true, want: 0},
		{name: "existing Workspace shell", fresh: false, freshWorkspace: false, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, pair, _, _, _, _, stderr, _ := newFirstEntryCLI(t, tt.fresh, true, recommendedFirstUseStart)
			pair.resolution.Observation.Context = &tobari.ContextAuthoritySnapshot{}
			if !tt.freshWorkspace {
				pair.resolution.Observation.Context.Workspace = &tobari.WorkspaceBinding{}
			}
			if code := command.RunContext(context.Background(), tt.args); code != ExitOK {
				t.Fatalf("entry exit=%d stderr=%q", code, stderr.String())
			}
			if count := strings.Count(stderr.String(), "Credentials stay in this Workspace; sign in with the tool when needed."); count != tt.want {
				t.Fatalf("credential guidance count=%d want=%d stderr=%q", count, tt.want, stderr.String())
			}
		})
	}
}
