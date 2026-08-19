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
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type guidedEntryRuntime struct {
	policyReviewRuntimeFake
	clusterUpErr   error
	clusterUpCalls int
	configured     bool
}

func (f *guidedEntryRuntime) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	return action(ctx)
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
		ContextCount: 1, PolicyRevision: strings.Repeat("a", 64),
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

type guidedRuntimeChoice struct {
	choice  runtimeChoice
	err     error
	calls   int
	runtime *contextCLI
}

func (w *guidedRuntimeChoice) Choose(
	_ context.Context, _ tobari.ContextReport, _ io.Reader, _ io.Writer,
) (runtimeChoice, error) {
	w.calls++
	if w.choice == runtimeChoiceCustomize && w.runtime != nil {
		w.runtime.report.Runtime = tobari.ContextRuntimeReport{
			Kind: tobari.ContextRuntimeKindDockerfile, Status: tobari.ContextRuntimeStatusPendingBuild,
			Dockerfile: "/tmp/tobari/contexts/coding/runtime/Dockerfile", BaseReference: tobari.OfficialRuntimeBase,
		}
		w.runtime.report.Stores.RuntimeDirectory = "/tmp/tobari/contexts/coding/runtime"
		w.runtime.report.Stores.RuntimeDockerfile = "/tmp/tobari/contexts/coding/runtime/Dockerfile"
	}
	return w.choice, w.err
}

func guidedContextSelection() contextCreateSelection {
	return contextCreateSelection{
		Name: "coding", SourceAccess: tobari.ContextSourceAccessReadWrite,
		MethodPolicy: tobari.PolicyPresetMethodPolicy{
			Default:   tobari.PolicyPresetMethodExactReview,
			Overrides: []tobari.PolicyPresetMethodOverride{},
		},
	}
}

func syntheticContextList() tobari.ContextListResult {
	return tobari.ContextListResult{
		Task: tobari.TaskContextList, ContextState: tobari.ContextObservationSyntheticDefault,
		Active: tobari.DefaultContextName, Items: []tobari.ContextSummary{},
	}
}

func persistedContextList(report tobari.ContextReport) tobari.ContextListResult {
	return tobari.ContextListResult{
		Task: tobari.TaskContextList, ContextState: tobari.ContextObservationPersisted,
		Active: report.Name,
		Items: []tobari.ContextSummary{{
			ID: report.ID, Name: report.Name, ContextState: tobari.ContextObservationPersisted, Active: true,
			AgentProfile: report.AgentProfile, Image: report.Image, PolicyMode: report.PolicyMode,
			SourceAccess: report.SourceAccess, PolicyPresetOrigin: report.PolicyPresetOrigin,
			PolicyPresetRevision: report.PolicyPresetRevision, NativeReadiness: tobari.ContextNativeReadinessEnabled,
			MethodPolicy: report.MethodPolicy, RuntimeStatus: report.Runtime.Status,
			Bootstrap: tobari.ContextBootstrapReport{State: tobari.ContextBootstrapNotConfigured, Adapters: []string{}},
		}},
	}
}

func newGuidedEntryCLI(
	contextRuntime *contextCLI, shared *guidedEntryRuntime, choice runtimeChoiceWizard,
) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextRuntime)
	command.tobari = tobaricmd.New(shared)
	command.runtimeChoice = choice
	return command, stdout, stderr
}

func TestGuidedEntryCreatesContextStartsClusterAndStagesCustomRuntime(t *testing.T) {
	contextRuntime := &contextCLI{list: syntheticContextList()}
	wizard := &guidedContextWizard{selection: guidedContextSelection()}
	choice := &guidedRuntimeChoice{choice: runtimeChoiceCustomize, runtime: contextRuntime}
	shared := &guidedEntryRuntime{policyReviewRuntimeFake: policyReviewRuntimeFake{
		terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ContextCount: 1, AssetVersion: "test"},
	}, configured: true}
	command, stdout, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)
	command.contextCreate = wizard

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || continueEntry {
		t.Fatalf("guided custom entry = (%d, %t), stderr = %q", code, continueEntry, stderr.String())
	}
	if contextRuntime.createCalls != 1 || contextRuntime.initCalls != 1 || wizard.calls != 1 || choice.calls != 1 {
		t.Fatalf("create/init/wizard/choice calls = %d/%d/%d/%d", contextRuntime.createCalls, contextRuntime.initCalls, wizard.calls, choice.calls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("guided root wrote child stdout before entry: %q", stdout.String())
	}
	for _, expected := range []string{
		"✓ Context created: coding", "✓ Shared services ready", "✓ Runtime recipe created",
		"/tmp/tobari/contexts/coding/runtime/Dockerfile", "tobari runtime build", "After the build succeeds:",
	} {
		if !strings.Contains(stderr.String(), expected) {
			t.Errorf("guided transcript lacks %q: %q", expected, stderr.String())
		}
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
	if !strings.Contains(stderr.String(), "✓ Context created: coding") || !humanOutputHasRow(stderr.String(), "Code", "cluster_start_failed") {
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
	report := contextCLIReport(tobari.TaskContextShow, "coding", true, tobari.BuiltinImageSelector, tobari.ContextPolicyModeGuided)
	report.Runtime = tobari.ContextRuntimeReport{
		Kind: tobari.ContextRuntimeKindDockerfile, Status: tobari.ContextRuntimeStatusPendingBuild,
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
		terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ContextCount: 1, AssetVersion: "test"},
	}, configured: true}
	command, _, stderr := newGuidedEntryCLI(contextRuntime, shared, choice)
	command.contextCreate = &guidedContextWizard{selection: guidedContextSelection()}

	code, continueEntry := prepareGuidedProjectEntry(context.Background(), command, "")
	if code != ExitOK || !continueEntry || contextRuntime.initCalls != 0 || choice.calls != 1 {
		t.Fatalf("guided standard entry = (%d, %t), init/choice = %d/%d, stderr = %q", code, continueEntry, contextRuntime.initCalls, choice.calls, stderr.String())
	}
}

func TestGuidedEntryReconcilesMissingClusterForStandaloneContext(t *testing.T) {
	report := contextCLIReport(tobari.TaskContextShow, "coding", true, tobari.BuiltinImageSelector, tobari.ContextPolicyModeGuided)
	contextRuntime := &contextCLI{report: report, list: persistedContextList(report)}
	shared := &guidedEntryRuntime{
		policyReviewRuntimeFake: policyReviewRuntimeFake{
			terminal: true, state: tobari.State{SchemaVersion: 1, RuntimeDirectory: "/tmp/tobari/runtime", PolicyDirectory: "/tmp/tobari/policy", GatewayConfig: "/tmp/tobari/gateway.json", AggregateRevision: strings.Repeat("a", 64), ContextCount: 1, AssetVersion: "test"},
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
	report := contextCLIReport(tobari.TaskContextShow, "coding", true, tobari.BuiltinImageSelector, tobari.ContextPolicyModeGuided)
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
