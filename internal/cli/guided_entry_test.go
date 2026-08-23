package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type guidedEntryRuntime struct {
	policyReviewRuntimeFake
	clusterUpErr   error
	clusterUpCalls int
	configured     bool
	rootErr        error
	rootReads      int
	readiness      map[doctor.CheckID]doctor.Observation
	readinessCalls []doctor.CheckID
}

func (f *guidedEntryRuntime) ObserveDoctorCheck(
	ctx context.Context, root string, id doctor.CheckID,
) (doctor.Observation, error) {
	f.readinessCalls = append(f.readinessCalls, id)
	if observation, ok := f.readiness[id]; ok {
		return observation, nil
	}
	return f.policyReviewRuntimeFake.ObserveDoctorCheck(ctx, root, id)
}

func (f *guidedEntryRuntime) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	return action(ctx)
}

func (f *guidedEntryRuntime) ResolveProjectRoot(_ context.Context, root string) (string, error) {
	f.rootReads++
	return root, f.rootErr
}

func (f *guidedEntryRuntime) ClusterUp(context.Context) (tobari.State, error) {
	f.clusterUpCalls++
	if f.clusterUpErr != nil {
		return tobari.State{}, f.clusterUpErr
	}
	return f.state, nil
}

func (f *guidedEntryRuntime) LoadState(context.Context) (tobari.State, bool, error) {
	return f.state, f.configured, nil
}

func (f *guidedEntryRuntime) InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error) {
	status := tobari.ClusterStatus{
		Configured: true, Running: true, Policy: "/tmp/tobari/policy",
		ManifestCount: 1, PolicyRevision: strings.Repeat("a", 64),
		PolicyProjection: "valid", PrincipalRegistry: "valid", GatewayProjection: "valid",
		Components: validClusterComponentStatuses(),
	}
	if buildIdentityHasBroker() {
		status.AuthProviderProjection = "valid"
		status.AuthBrokerState = "ready"
		status.CredentialCompanionState = "ready"
		status.RootKeyBackend = "xdg_file"
	}
	return status, nil
}

type guidedContextWizard struct {
	selection contextCreateSelection
	err       error
	calls     int
}

func (w *guidedContextWizard) Compose(context.Context, io.Reader, io.Writer) (contextCreateSelection, error) {
	w.calls++
	return w.selection, w.err
}

func (w *guidedContextWizard) ComposeSeeded(_ context.Context, _ io.Reader, _ io.Writer, seed contextCreateWizardSeed) (contextCreateSelection, error) {
	w.calls++
	if w.selection.Name == "" {
		return seed.Selection, w.err
	}
	return w.selection, w.err
}

type guidedFirstUseReviewer struct {
	action recommendedFirstUseAction
	err    error
	calls  int
	draft  tobari.RecommendedFirstUseDraft
}

func (r *guidedFirstUseReviewer) Review(_ context.Context, draft tobari.RecommendedFirstUseDraft, _ io.Reader, _ io.Writer) (recommendedFirstUseAction, error) {
	r.calls++
	r.draft = draft
	return r.action, r.err
}

type guidedRuntimeChoice struct {
	choice  runtimeChoice
	err     error
	calls   int
	runtime *contextCLI
}

func (w *guidedRuntimeChoice) Choose(
	_ context.Context, _ tobari.ManifestReport, _ io.Reader, _ io.Writer,
) (runtimeChoice, error) {
	w.calls++
	if w.choice == runtimeChoiceCustomize && w.runtime != nil {
		w.runtime.report.Runtime = tobari.ManifestRuntimeReport{
			Kind: tobari.ManifestRuntimeKindDockerfile, Status: tobari.ManifestRuntimeStatusPendingBuild,
			Dockerfile: "/tmp/tobari/contexts/coding/runtime/Dockerfile", BaseReference: tobari.OfficialRuntimeBase,
		}
		w.runtime.report.Stores.RuntimeDirectory = "/tmp/tobari/contexts/coding/runtime"
		w.runtime.report.Stores.RuntimeDockerfile = "/tmp/tobari/contexts/coding/runtime/Dockerfile"
	}
	return w.choice, w.err
}

func guidedContextSelection() contextCreateSelection {
	return contextCreateSelection{
		Name: "coding", SourceAccess: tobari.ManifestSourceAccessReadWrite,
		RuntimeSelection: "standard@1", NativeReadiness: tobari.ManifestNativeReadinessEnabled,
		MethodPolicy: tobari.ManifestMethodPolicy{
			Default:   tobari.ManifestMethodExactReview,
			Overrides: []tobari.ManifestMethodOverride{},
		},
	}
}

func syntheticContextList() tobari.ManifestListResult {
	return tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
		Items: []tobari.ManifestSummary{},
	}
}

func persistedContextList(report tobari.ManifestReport) tobari.ManifestListResult {
	return tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: report.ID, DefaultManifest: report.Name,
		Items: []tobari.ManifestSummary{{
			ID: report.ID, Name: report.Name, ManifestState: tobari.ManifestObservationPersisted, Default: true,
			Desired:      report.Desired,
			AgentProfile: report.AgentProfile, Image: report.Image, PolicyMode: report.PolicyMode,
			SourceAccess: report.SourceAccess, PolicyRevision: report.PolicyRevision, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			MethodPolicy: report.MethodPolicy, RuntimeStatus: report.Runtime.Status, RuntimeSelection: runtimeSelection(report.Runtime),
			Bootstrap: tobari.ManifestBootstrapReport{State: tobari.ManifestBootstrapNotConfigured, Adapters: []string{}},
		}},
	}
}

func runtimeSelection(report tobari.ManifestRuntimeReport) string {
	selection, _ := report.Selection()
	return selection
}

func newGuidedEntryCLI(
	contextRuntime *contextCLI, shared *guidedEntryRuntime, choice runtimeChoiceWizard,
) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextRuntime)
	command.tobari = tobaricmd.New(shared)
	command.runtimeChoice = choice
	command.firstUse = &guidedFirstUseReviewer{action: recommendedFirstUseCustomize}
	return command, stdout, stderr
}

func TestGuidedEntryCreatesContextStartsClusterAndContinuesWithoutRuntimeFork(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	wizard := &guidedContextWizard{selection: guidedContextSelection()}
	choice := &guidedRuntimeChoice{choice: runtimeChoiceCustomize, runtime: contextRuntime}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{
		terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1, AssetVersion: "test"},
	}, configured: true}
	command, stdout, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)
	command.contextCreate = wizard

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || !continueEntry {
		t.Fatalf("guided entry = (%d, %t), stderr = %q", code, continueEntry, stderr.String())
	}
	if contextRuntime.createCalls != 1 || contextRuntime.initCalls != 0 || wizard.calls != 1 || choice.calls != 0 {
		t.Fatalf("create/init/wizard/choice calls = %d/%d/%d/%d", contextRuntime.createCalls, contextRuntime.initCalls, wizard.calls, choice.calls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("guided root wrote child stdout before entry: %q", stdout.String())
	}
	for _, expected := range []string{"✓ Workspace Manifest created: coding", "✓ Shared services ready"} {
		if !strings.Contains(stderr.String(), expected) {
			t.Errorf("guided transcript lacks %q: %q", expected, stderr.String())
		}
	}
}

func TestGuidedFirstUseDockerFailurePrecedesEveryMutation(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{
		policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true},
		readiness: map[doctor.CheckID]doctor.Observation{
			doctor.CheckIDDockerEngine: {Status: doctor.CheckStatusFail, Detail: "synthetic unavailable"},
		},
	}
	command, stdout, stderr := newGuidedEntryCLI(contextRuntime, shared, nil)
	command.firstUse = &guidedFirstUseReviewer{action: recommendedFirstUseStart}
	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitUnavailable || continueEntry || stdout.Len() != 0 ||
		contextRuntime.createCalls != 0 || contextRuntime.initCalls != 0 || shared.clusterUpCalls != 0 {
		t.Fatalf("preflight result=(%d,%t) stdout=%q create/init/cluster=%d/%d/%d stderr=%q",
			code, continueEntry, stdout.String(), contextRuntime.createCalls, contextRuntime.initCalls, shared.clusterUpCalls, stderr.String())
	}
	if !humanOutputHasRow(stderr.String(), "Code", "docker_engine_unavailable") ||
		!humanOutputHasRow(stderr.String(), "Phase", "precondition") ||
		!humanOutputHasRow(stderr.String(), "Change state", "none") {
		t.Fatalf("preflight failure facts = %q", stderr.String())
	}
}

func TestRecommendedFirstUseStartCreatesExactDraftWithoutWizard(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{
		terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1, AssetVersion: "test"},
	}, configured: true}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	reviewer := &guidedFirstUseReviewer{action: recommendedFirstUseStart}
	wizard := &guidedContextWizard{}
	command.firstUse, command.contextCreate = reviewer, wizard
	session, err := tobari.NewWorkspaceDirectSession([]string{"claude", "--flag", ""})
	if err != nil {
		t.Fatal(err)
	}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "", session)
	if code != ExitOK || !continueEntry || reviewer.calls != 1 || wizard.calls != 0 ||
		contextRuntime.listCalls != 2 || contextRuntime.createCalls != 1 || shared.clusterUpCalls != 1 {
		t.Fatalf("start result/calls = (%d,%t), review=%d wizard=%d list=%d create=%d cluster=%d stderr=%q", code, continueEntry, reviewer.calls, wizard.calls, contextRuntime.listCalls, contextRuntime.createCalls, shared.clusterUpCalls, stderr.String())
	}
	if reviewer.draft.WorkspaceManifestName != tobari.DefaultManifestName || reviewer.draft.ProjectRoot != "/tmp/project" ||
		reviewer.draft.Session.Executable != "claude" || contextRuntime.report.Name != tobari.DefaultManifestName {
		t.Fatalf("reviewed/created draft = %+v / %+v", reviewer.draft, contextRuntime.report)
	}
}

func TestRecommendedFirstUseCancelHasNoLaterSideEffects(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	command.firstUse = &guidedFirstUseReviewer{action: recommendedFirstUseCancel}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitCanceled || continueEntry || contextRuntime.createCalls != 0 || contextRuntime.prepareBootstrapCalls != 0 || shared.clusterUpCalls != 0 {
		t.Fatalf("cancel result/calls = (%d,%t), create=%d cluster=%d stderr=%q", code, continueEntry, contextRuntime.createCalls, shared.clusterUpCalls, stderr.String())
	}
}

func TestRecommendedFirstUseInvalidRootFailsBeforeReviewOrMutation(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}, rootErr: errors.New("protected root")}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	reviewer := &guidedFirstUseReviewer{action: recommendedFirstUseStart}
	command.firstUse = reviewer

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitUsage || continueEntry || reviewer.calls != 0 || contextRuntime.createCalls != 0 ||
		contextRuntime.prepareBootstrapCalls != 0 || shared.clusterUpCalls != 0 ||
		!humanOutputHasRow(stderr.String(), "Code", "invalid_root") {
		t.Fatalf("invalid root result/calls = (%d,%t), review=%d create=%d host=%d cluster=%d stderr=%q", code, continueEntry, reviewer.calls, contextRuntime.createCalls, contextRuntime.prepareBootstrapCalls, shared.clusterUpCalls, stderr.String())
	}
}

func TestRecommendedFirstUseReviewFailureHasZeroMutation(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	command.firstUse = &guidedFirstUseReviewer{err: errors.New("render failed")}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitInternal || continueEntry || contextRuntime.createCalls != 0 ||
		contextRuntime.prepareBootstrapCalls != 0 || shared.clusterUpCalls != 0 ||
		!humanOutputHasRow(stderr.String(), "Code", "first_use_review_failed") {
		t.Fatalf("review failure result/calls = (%d,%t), create=%d host=%d cluster=%d stderr=%q", code, continueEntry, contextRuntime.createCalls, contextRuntime.prepareBootstrapCalls, shared.clusterUpCalls, stderr.String())
	}
}

func TestRecommendedFirstUseStartRejectsConcurrentContextChange(t *testing.T) {
	other := contextCLIReport(tobari.TaskManifestShow, "other", true, tobari.BuiltinImageSelector, tobari.ManifestPolicyModeGuided)
	contextRuntime := &contextCLI{listResults: []tobari.ManifestListResult{syntheticContextList(), persistedContextList(other)}}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	command.firstUse = &guidedFirstUseReviewer{action: recommendedFirstUseStart}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitRejected || continueEntry || contextRuntime.createCalls != 0 || shared.clusterUpCalls != 0 ||
		!humanOutputHasRow(stderr.String(), "Code", "manifest_collection_changed") {
		t.Fatalf("race result/calls = (%d,%t), create=%d cluster=%d stderr=%q", code, continueEntry, contextRuntime.createCalls, shared.clusterUpCalls, stderr.String())
	}
}

func TestDirectEntryRejectsEmptyExecutableBeforeGuidedSetup(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	wizard := &guidedContextWizard{selection: guidedContextSelection()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{
		terminal: true,
	}, configured: false}
	command, stdout, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	command.contextCreate = wizard

	if code := command.RunContext(context.Background(), []string{"--", ""}); code != ExitUsage {
		t.Fatalf("empty direct executable code = %d, stderr = %q", code, stderr.String())
	}
	if contextRuntime.listCalls != 0 || wizard.calls != 0 || shared.clusterUpCalls != 0 {
		t.Fatalf(
			"invalid direct entry side effects: context reads=%d wizard=%d cluster up=%d",
			contextRuntime.listCalls, wizard.calls, shared.clusterUpCalls,
		)
	}
	if stdout.Len() != 0 || !humanOutputHasRow(stderr.String(), "Code", "invalid_arguments") {
		t.Fatalf("empty direct executable output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestGuidedEntryRetainsContextWhenClusterStartupFails(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{
		policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true},
		clusterUpErr:            errors.New("synthetic cluster failure"),
	}
	choice := &guidedRuntimeChoice{choice: runtimeChoiceStandard}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)
	command.contextCreate = &guidedContextWizard{selection: guidedContextSelection()}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitUnavailable || continueEntry {
		t.Fatalf("cluster failure entry = (%d, %t), stderr = %q", code, continueEntry, stderr.String())
	}
	if contextRuntime.createCalls != 1 || contextRuntime.initCalls != 0 || choice.calls != 0 {
		t.Fatalf("partial success create/init/choice = %d/%d/%d", contextRuntime.createCalls, contextRuntime.initCalls, choice.calls)
	}
	if !strings.Contains(stderr.String(), "✓ Workspace Manifest created: coding") || !humanOutputHasRow(stderr.String(), "Code", "cluster_start_failed") {
		t.Fatalf("partial-success recovery is unclear: %q", stderr.String())
	}
}

func TestGuidedEntryContextCancellationPerformsNoMutation(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})
	command.contextCreate = &guidedContextWizard{err: context.Canceled}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitCanceled || continueEntry || contextRuntime.createCalls != 0 || shared.clusterUpCalls != 0 {
		t.Fatalf("canceled guided entry = (%d, %t), create/cluster = %d/%d, stderr = %q", code, continueEntry, contextRuntime.createCalls, shared.clusterUpCalls, stderr.String())
	}
}

func TestGuidedEntryBlocksPendingRuntimeBeforeWorkspaceMutation(t *testing.T) {
	report := contextCLIReport(tobari.TaskManifestShow, "coding", true, tobari.BuiltinImageSelector, tobari.ManifestPolicyModeGuided)
	report.Runtime = tobari.ManifestRuntimeReport{
		Kind: tobari.ManifestRuntimeKindDockerfile, Status: tobari.ManifestRuntimeStatusPendingBuild,
		Dockerfile: filepath.Join("/tmp", "tobari", "runtime", "Dockerfile"), BaseReference: tobari.OfficialRuntimeBase,
	}
	report.Stores.RuntimeDirectory = filepath.Dir(report.Runtime.Dockerfile)
	report.Stores.RuntimeDockerfile = report.Runtime.Dockerfile
	contextRuntime := &contextCLI{report: report, list: persistedContextList(report)}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: true}}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitRejected || continueEntry {
		t.Fatalf("pending runtime entry = (%d, %t), stderr = %q", code, continueEntry, stderr.String())
	}
	if !humanOutputHasRow(stderr.String(), "Code", "runtime_build_required") ||
		!strings.Contains(stderr.String(), "tobari runtime build") {
		t.Fatalf("pending runtime recovery = %q", stderr.String())
	}
}

func TestGuidedEntrySkipsObservationAndMutationWithoutInteractiveStreams(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{terminal: false}}
	command, _, _ := newGuidedEntryCLI(contextRuntime, shared, &guidedRuntimeChoice{})

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || !continueEntry || contextRuntime.listCalls != 0 || contextRuntime.createCalls != 0 {
		t.Fatalf("non-interactive preflight = (%d, %t), list/create = %d/%d", code, continueEntry, contextRuntime.listCalls, contextRuntime.createCalls)
	}
}

func TestGuidedEntryStandardChoiceContinuesToExistingWorkspaceEntry(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	choice := &guidedRuntimeChoice{choice: runtimeChoiceStandard}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{
		terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1, AssetVersion: "test"},
	}, configured: true}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)
	command.contextCreate = &guidedContextWizard{selection: guidedContextSelection()}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || !continueEntry || contextRuntime.initCalls != 0 || choice.calls != 0 {
		t.Fatalf("guided standard entry = (%d, %t), init/choice = %d/%d, stderr = %q", code, continueEntry, contextRuntime.initCalls, choice.calls, stderr.String())
	}
}

func TestGuidedEntryReconcilesMissingClusterForStandaloneContext(t *testing.T) {
	report := contextCLIReport(tobari.TaskManifestShow, "coding", true, tobari.BuiltinImageSelector, tobari.ManifestPolicyModeGuided)
	contextRuntime := &contextCLI{report: report, list: persistedContextList(report)}
	shared := &guidedEntryRuntime{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1, AssetVersion: "test"},
		},
		configured: false,
	}
	choice := &guidedRuntimeChoice{}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || !continueEntry || shared.clusterUpCalls != 1 || choice.calls != 0 {
		t.Fatalf("standalone continuation = (%d, %t), cluster/choice = %d/%d, stderr = %q", code, continueEntry, shared.clusterUpCalls, choice.calls, stderr.String())
	}
	if !strings.Contains(stderr.String(), "✓ Shared services ready") {
		t.Fatalf("standalone continuation hides cluster completion: %q", stderr.String())
	}
}

func TestRuntimeChoiceWizardDefaultsToStandardAndOffersCustomization(t *testing.T) {
	report := contextCLIReport(tobari.TaskManifestShow, "coding", true, tobari.BuiltinImageSelector, tobari.ManifestPolicyModeGuided)
	wizard := &terminalRuntimeChoiceWizard{chooser: &terminalContextConfigurationWizard{mode: nil, style: false}}
	var output bytes.Buffer
	choice, err := wizard.Choose(context.Background(), report, strings.NewReader("\n"), &output)
	if err != nil || choice != runtimeChoiceStandard {
		t.Fatalf("default runtime choice = (%d, %v)", choice, err)
	}
	for _, expected := range []string{
		"standard Tobari runtime selected", "Enter with the standard runtime",
		"Customize before creating the first Workspace",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("runtime choice output lacks %q: %q", expected, output.String())
		}
	}

	choice, err = wizard.Choose(context.Background(), report, strings.NewReader("2\n"), io.Discard)
	if err != nil || choice != runtimeChoiceCustomize {
		t.Fatalf("custom runtime choice = (%d, %v)", choice, err)
	}
}
