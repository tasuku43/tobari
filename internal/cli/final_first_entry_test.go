package cli

import (
	"bytes"
	"context"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type firstEntryPairFixture struct {
	order        *[]string
	observation  tobari.FinalDefaultPairObservation
	resolveBody  *tobari.WorkspaceTemplateBody
	session      tobari.WorkspaceSessionRequest
	resolveCalls int
	refreshCalls int
	entryCalls   int
	entryErr     error
}

func (f *firstEntryPairFixture) Observe(context.Context) (tobari.FinalDefaultPairObservation, error) {
	*f.order = append(*f.order, "observe")
	return f.observation.Clone(), nil
}

func (f *firstEntryPairFixture) Resolve(_ context.Context, intent operation.Intent, body *tobari.WorkspaceTemplateBody) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.resolveCalls++
	*f.order = append(*f.order, "resolve")
	if intent.Command != WorkspaceEntryCommandPath || intent.Effect != operation.EffectCreate || intent.Target.Kind != tobari.CurrentDirectoryTargetKind || intent.Target.ParentID != tobari.CurrentDirectoryTargetID {
		panic("root resolution lost its Catalog mutation binding")
	}
	f.resolveBody = body
	return workspaceauthoritycmd.DefaultPairResolution{}, nil
}

func (f *firstEntryPairFixture) RefreshAfterCluster(_ context.Context, resolution workspaceauthoritycmd.DefaultPairResolution, _ workspaceauthoritycmd.FinalClusterReconciliation) (workspaceauthoritycmd.DefaultPairResolution, error) {
	f.refreshCalls++
	*f.order = append(*f.order, "refresh")
	return resolution, nil
}

func (f *firstEntryPairFixture) EnterResolved(_ context.Context, _ workspaceauthoritycmd.DefaultPairResolution, session tobari.WorkspaceSessionRequest, progress tobari.FirstEntryProgressSink, _ io.Reader, _, _ io.Writer) (workspaceauthoritycmd.ContextEntryResult, error) {
	f.entryCalls++
	*f.order = append(*f.order, "entry")
	f.session = session
	if f.entryErr != nil {
		return workspaceauthoritycmd.ContextEntryResult{}, f.entryErr
	}
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageSucceeded})
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageRunning})
	progress(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageSucceeded})
	return workspaceauthoritycmd.ContextEntryResult{Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}, nil
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
	order *[]string
	calls int
	err   error
}

func (f *firstEntryClusterFixture) Reconcile(_ context.Context, intent operation.Intent) (workspaceauthoritycmd.FinalClusterReconciliation, error) {
	f.calls++
	*f.order = append(*f.order, "cluster")
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
	pair := &firstEntryPairFixture{order: &order, observation: observation}
	readiness := &firstEntryReadinessFixture{order: &order}
	cluster := &firstEntryClusterFixture{order: &order}
	reviewer := &firstEntryReviewerFixture{order: &order, action: action}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
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
	if !strings.Contains(stderr.String(), "workspace_entry_interrupted") || !strings.Contains(stderr.String(), "tobari status") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("entry recovery was not preserved: stderr=%q", stderr.String())
	}
}
