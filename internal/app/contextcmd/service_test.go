package contextcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextRuntimeFake struct {
	listResult           tobari.ContextListResult
	listResults          []tobari.ContextListResult
	showResult           tobari.ContextReport
	createResult         tobari.ContextReport
	deleteResult         tobari.ContextDeleteResult
	useResult            tobari.ContextReport
	initResult           tobari.ContextReport
	buildResult          tobari.ContextReport
	setRuntimeResult     tobari.ContextReport
	configureResult      tobari.ContextReport
	configureGitResult   tobari.ContextReport
	discoverAWSResult    tobari.ContextAWSBootstrapDiscovery
	baseResult           tobari.ContextCreateBase
	listErr              error
	showErr              error
	createErr            error
	deleteErr            error
	useErr               error
	initErr              error
	buildErr             error
	setRuntimeErr        error
	configureErr         error
	configureGitErr      error
	discoverAWSErr       error
	baseErr              error
	createCalls          int
	listCalls            int
	deleteCalls          int
	useCalls             int
	initCalls            int
	buildCalls           int
	setRuntimeCalls      int
	configureCalls       int
	configureGitCalls    int
	discoverAWSCalls     int
	baseCalls            int
	showCalls            int
	buildProgressCalls   int
	lastName             string
	lastShowName         string
	lastImage            string
	lastMode             tobari.ContextPolicyMode
	lastSourceAccess     tobari.ContextSourceAccess
	lastComposition      tobari.ContextCreateComposition
	lastChange           tobari.ContextShellEnvironmentSetting
	lastChanges          []tobari.ContextShellEnvironmentSetting
	lastGitChange        tobari.ContextGitIdentitySetting
	lastRuntimeSelection string
}

func (f *contextRuntimeFake) ContextCreateBase(_ context.Context, name string) (tobari.ContextCreateBase, error) {
	f.baseCalls++
	f.lastName = name
	return f.baseResult.Clone(), f.baseErr
}

func (f *contextRuntimeFake) DiscoverContextAWSBootstraps(context.Context) (tobari.ContextAWSBootstrapDiscovery, error) {
	f.discoverAWSCalls++
	return f.discoverAWSResult, f.discoverAWSErr
}

func (f *contextRuntimeFake) ListContexts(context.Context) (tobari.ContextListResult, error) {
	f.listCalls++
	if len(f.listResults) >= f.listCalls {
		return f.listResults[f.listCalls-1], f.listErr
	}
	return f.listResult, f.listErr
}

func (f *contextRuntimeFake) ShowContext(_ context.Context, name string) (tobari.ContextReport, error) {
	f.showCalls++
	f.lastShowName = name
	return f.showResult, f.showErr
}

func (f *contextRuntimeFake) CreateContext(
	_ context.Context, name, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess,
) (tobari.ContextReport, error) {
	f.createCalls++
	f.lastName, f.lastImage, f.lastMode = name, image, mode
	f.lastSourceAccess = sourceAccess
	return f.createResult, f.createErr
}

func (f *contextRuntimeFake) CreateContextWithComposition(
	_ context.Context, name, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess,
	composition tobari.ContextCreateComposition,
) (tobari.ContextReport, error) {
	f.createCalls++
	f.lastName, f.lastImage, f.lastMode = name, image, mode
	f.lastSourceAccess, f.lastComposition = sourceAccess, composition.Clone()
	return f.createResult, f.createErr
}

func (f *contextRuntimeFake) DeleteContext(_ context.Context, name string) (tobari.ContextDeleteResult, error) {
	f.deleteCalls++
	f.lastName = name
	return f.deleteResult, f.deleteErr
}

func (f *contextRuntimeFake) UseContext(context.Context, string) (tobari.ContextReport, error) {
	f.useCalls++
	return f.useResult, f.useErr
}

func (f *contextRuntimeFake) ConfigureContextShell(
	_ context.Context, name string, changes []tobari.ContextShellEnvironmentSetting,
) (tobari.ContextReport, error) {
	f.configureCalls++
	f.lastName = name
	f.lastChanges = append([]tobari.ContextShellEnvironmentSetting(nil), changes...)
	if len(changes) > 0 {
		f.lastChange = changes[0]
	}
	return f.configureResult, f.configureErr
}

func (f *contextRuntimeFake) ConfigureContextGit(
	_ context.Context, name string, change tobari.ContextGitIdentitySetting,
) (tobari.ContextReport, error) {
	f.configureGitCalls++
	f.lastName, f.lastGitChange = name, change
	return f.configureGitResult, f.configureGitErr
}

func (f *contextRuntimeFake) InitRuntime(context.Context) (tobari.ContextReport, error) {
	f.initCalls++
	return f.initResult, f.initErr
}

func (f *contextRuntimeFake) BuildRuntime(context.Context) (tobari.ContextReport, error) {
	f.buildCalls++
	return f.buildResult, f.buildErr
}

func (f *contextRuntimeFake) SetContextRuntime(_ context.Context, name, selection string) (tobari.ContextReport, error) {
	f.setRuntimeCalls++
	f.lastName, f.lastRuntimeSelection = name, selection
	return f.setRuntimeResult, f.setRuntimeErr
}

func (f *contextRuntimeFake) BuildRuntimeWithProgress(
	_ context.Context, diagnostics io.Writer, progress tobari.RuntimeBuildProgressSink,
) (tobari.ContextReport, error) {
	f.buildCalls++
	f.buildProgressCalls++
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "synthetic BuildKit output\n")
	}
	if progress != nil {
		progress(tobari.RuntimeBuildProgress{
			Stage: tobari.RuntimeBuildStageBuild, Status: tobari.RuntimeBuildProgressStarted,
			ContextName: "default", Dockerfile: "/config/contexts/default/runtime/Dockerfile",
			PreviousImage: tobari.OfficialRuntimeBase, CandidateImage: "tobari-context-default:0123456789ab",
			Selection: tobari.RuntimeBuildSelectionUnchanged,
		})
	}
	return f.buildResult, f.buildErr
}

func contextReport(task, name string) tobari.ContextReport {
	authentication := tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	if task == tobari.TaskContextShow {
		authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerReady, Providers: []tobari.ContextAuthProvider{}}
	}
	return tobari.ContextReport{
		Task: task, ContextState: tobari.ContextObservationPersisted, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name, Active: task == tobari.TaskContextUse,
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode:       tobari.ContextPolicyModeGuided,
		SourceAccess:     tobari.ContextSourceAccessReadWrite,
		PolicyRevision:   tobari.DefaultContextPolicyRevision(),
		NativeReadiness:  tobari.ContextNativeReadinessEnabled,
		MethodPolicy:     tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview, Overrides: []tobari.ContextMethodOverride{}},
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Runtime: tobari.ContextRuntimeReport{
			Kind: tobari.ContextRuntimeKindOfficial, Status: tobari.ContextRuntimeStatusOfficial,
			Image: tobari.OfficialRuntimeBase, RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1,
		},
		Cluster:        tobari.ContextClusterStatusNotApplicable,
		Authentication: authentication,
		Stores: tobari.ContextStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
		},
	}
}

func TestDiscoverAWSBootstrapsPreservesTypedRejectedSourceWithoutMutation(t *testing.T) {
	t.Parallel()
	runtime := &contextRuntimeFake{discoverAWSResult: tobari.ContextAWSBootstrapDiscovery{
		State: tobari.ContextBootstrapDiscoveryRejected, Reason: "Host AWS shared config is unsafe.", Candidates: []tobari.ContextAWSBootstrapCandidate{},
	}}
	service := New(runtime)
	result, err := service.DiscoverAWSBootstraps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ContextBootstrapDiscoveryRejected || runtime.discoverAWSCalls != 1 || runtime.createCalls != 0 || runtime.configureCalls != 0 {
		t.Fatalf("discovery result/calls = %+v / discover=%d create=%d configure=%d", result, runtime.discoverAWSCalls, runtime.createCalls, runtime.configureCalls)
	}
}

func configuredShellContextReport(
	task, name string, active bool, change tobari.ContextShellEnvironmentSetting,
) tobari.ContextReport {
	report := contextReport(task, name)
	report.Active = active
	for index := range report.ShellEnvironment {
		if report.ShellEnvironment[index].Variable == change.Variable {
			report.ShellEnvironment[index] = change
			break
		}
	}
	return report
}

func configuredGitContextReport(
	task, name string, active bool, change tobari.ContextGitIdentitySetting,
) tobari.ContextReport {
	report := contextReport(task, name)
	report.Active = active
	report.GitIdentity = change
	return report
}

func TestContextReadsPreserveCallerCancellation(t *testing.T) {
	for _, canceled := range []error{context.Canceled, context.DeadlineExceeded} {
		canceled := canceled
		t.Run(canceled.Error(), func(t *testing.T) {
			service := New(&contextRuntimeFake{listErr: canceled, showErr: canceled})
			if _, err := service.List(context.Background()); !errors.Is(err, canceled) {
				t.Fatalf("List() error = %v, want %v", err, canceled)
			}
			if _, err := service.Show(context.Background(), "default"); !errors.Is(err, canceled) {
				t.Fatalf("Show() error = %v, want %v", err, canceled)
			}
		})
	}
}

func TestShowCorrelatesTaskAndSelectedContext(t *testing.T) {
	tests := []struct {
		name        string
		contextName string
		result      func() tobari.ContextReport
		wantFault   bool
	}{
		{
			name: "explicit active Context is allowed", contextName: "project-tools", result: func() tobari.ContextReport {
				report := contextReport(tobari.TaskContextShow, "project-tools")
				report.Active = true
				return report
			},
		},
		{
			name: "omitted Context returns active Context", result: func() tobari.ContextReport {
				report := contextReport(tobari.TaskContextShow, "default")
				report.Active = true
				return report
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return contextReport(tobari.TaskConfigShell, "project-tools")
			},
		},
		{
			name: "wrong explicit Context", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return contextReport(tobari.TaskContextShow, "other")
			},
		},
		{
			name: "omitted Context is not active", wantFault: true, result: func() tobari.ContextReport {
				return contextReport(tobari.TaskContextShow, "default")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextRuntimeFake{showResult: test.result()}
			result, err := New(fake).Show(context.Background(), test.contextName)
			if !test.wantFault {
				if err != nil || result.Task != tobari.TaskContextShow || fake.showCalls != 1 ||
					fake.lastShowName != test.contextName {
					t.Fatalf("Show() result/error/calls/name = %+v / %v / %d / %q", result, err, fake.showCalls, fake.lastShowName)
				}
				return
			}
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_context_report" ||
				public.Retryable || fake.showCalls != 1 {
				t.Fatalf("Show() fault = %+v, ok=%t, calls=%d", public, ok, fake.showCalls)
			}
		})
	}
}

func shellContextImpact() operation.Impact {
	return operation.Impact{
		Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
		AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
	}
}

func TestConfigureShellValidatesBeforeMutationAndPreservesLiteralEmpty(t *testing.T) {
	empty := ""
	change := tobari.ContextShellEnvironmentSetting{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentLiteral, Value: &empty,
	}
	fake := &contextRuntimeFake{
		configureResult: configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, change),
	}
	service := New(fake)
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID},
		Impact: shellContextImpact(),
	}
	result, err := service.ConfigureShell(context.Background(), intent, "project-tools", []tobari.ContextShellEnvironmentSetting{change})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskConfigShell || fake.configureCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastChange.Value == nil || *fake.lastChange.Value != "" {
		t.Fatalf("result/call = %+v / %d %q %+v", result, fake.configureCalls, fake.lastName, fake.lastChange)
	}

	fake.configureCalls = 0
	change.Variable = "PATH"
	_, err = service.ConfigureShell(context.Background(), intent, "project-tools", []tobari.ContextShellEnvironmentSetting{change})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_shell_environment" || fake.configureCalls != 0 {
		t.Fatalf("invalid configure = %#v, ok=%t, calls=%d", public, ok, fake.configureCalls)
	}
}

func TestConfigureShellSendsOneAtomicStagedBatch(t *testing.T) {
	changes := []tobari.ContextShellEnvironmentSetting{
		{Variable: "COLORTERM", Source: tobari.ContextShellEnvironmentInherit},
		{Variable: "NO_COLOR", Source: tobari.ContextShellEnvironmentDefault},
	}
	report := contextReport(tobari.TaskConfigShell, "project-tools")
	report.Active = false
	for _, change := range changes {
		for index := range report.ShellEnvironment {
			if report.ShellEnvironment[index].Variable == change.Variable {
				report.ShellEnvironment[index] = change
			}
		}
	}
	fake := &contextRuntimeFake{configureResult: report}
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID},
		Impact: shellContextImpact(),
	}
	if _, err := New(fake).ConfigureShell(context.Background(), intent, "project-tools", changes); err != nil {
		t.Fatalf("ConfigureShell() error = %v", err)
	}
	if fake.configureCalls != 1 || len(fake.lastChanges) != 2 {
		t.Fatalf("configure calls/changes = %d/%+v", fake.configureCalls, fake.lastChanges)
	}
}

func TestConfigureShellRejectsSemanticallyMismatchedResult(t *testing.T) {
	empty, different := "", "different"
	change := tobari.ContextShellEnvironmentSetting{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentLiteral, Value: &empty,
	}
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID},
		Impact: shellContextImpact(),
	}
	tests := []struct {
		name        string
		contextName string
		result      func() tobari.ContextReport
		wantFault   bool
	}{
		{
			name: "omitted Context returns active Context", result: func() tobari.ContextReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "default", true, change)
			},
		},
		{
			name: "explicit active Context is allowed", contextName: "project-tools", result: func() tobari.ContextReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "project-tools", true, change)
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return configuredShellContextReport(tobari.TaskConfigGit, "project-tools", false, change)
			},
		},
		{
			name: "wrong explicit Context", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "other", false, change)
			},
		},
		{
			name: "omitted Context is not active", wantFault: true, result: func() tobari.ContextReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "default", false, change)
			},
		},
		{
			name: "wrong applied setting", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return contextReport(tobari.TaskConfigShell, "project-tools")
			},
		},
		{
			name: "wrong literal value", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				wrong := change
				wrong.Value = &different
				return configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong cluster outcome", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				report := configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, change)
				report.Cluster = tobari.ContextClusterStatusReconciled
				return report
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextRuntimeFake{configureResult: test.result()}
			_, err := New(fake).ConfigureShell(context.Background(), intent, test.contextName, []tobari.ContextShellEnvironmentSetting{change})
			if !test.wantFault {
				if err != nil || fake.configureCalls != 1 {
					t.Fatalf("ConfigureShell() error = %v, calls = %d", err, fake.configureCalls)
				}
				return
			}
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_context_report" ||
				public.Retryable || fake.configureCalls != 1 {
				t.Fatalf("ConfigureShell() fault = %+v, ok=%t, calls=%d", public, ok, fake.configureCalls)
			}
		})
	}
}

func TestConfigureShellMapsMissingContext(t *testing.T) {
	fake := &contextRuntimeFake{configureErr: tobari.ErrContextNotFound}
	service := New(fake)
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextShellTargetKind, ID: tobari.ContextShellTargetID},
		Impact: shellContextImpact(),
	}
	_, err := service.ConfigureShell(context.Background(), intent, "missing", []tobari.ContextShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentInherit,
	}})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "context_not_found" || fake.configureCalls != 1 {
		t.Fatalf("missing Context configure = %#v, ok=%t, calls=%d", public, ok, fake.configureCalls)
	}
}

func TestConfigureGitValidatesInputAndIntentBeforeMutation(t *testing.T) {
	name, email := "Tobari User", "tobari@example.com"
	setting := tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityLiteral, Name: &name, Email: &email,
	}
	fake := &contextRuntimeFake{
		configureGitResult: configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, setting),
	}
	service := New(fake)
	intent := operation.Intent{
		Command: "config git", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID},
		Impact: shellContextImpact(),
	}
	result, err := service.ConfigureGit(context.Background(), intent, "project-tools", setting)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskConfigGit || fake.configureGitCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastGitChange.Name == nil || *fake.lastGitChange.Name != name ||
		fake.lastGitChange.Email == nil || *fake.lastGitChange.Email != email {
		t.Fatalf("result/call = %+v / %d %q %+v", result, fake.configureGitCalls, fake.lastName, fake.lastGitChange)
	}

	fake.configureGitCalls = 0
	invalid := tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityLiteral, Name: &name}
	_, err = service.ConfigureGit(context.Background(), intent, "project-tools", invalid)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_git_identity" || fake.configureGitCalls != 0 {
		t.Fatalf("invalid Git configuration = %#v, ok=%t, calls=%d", public, ok, fake.configureGitCalls)
	}

	wrongIntent := intent
	wrongIntent.Target.ID = "other-target"
	_, err = service.ConfigureGit(context.Background(), wrongIntent, "project-tools", setting)
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "invalid_mutation_contract" || fake.configureGitCalls != 0 {
		t.Fatalf("invalid Git intent = %#v, ok=%t, calls=%d", public, ok, fake.configureGitCalls)
	}
}

func TestConfigureGitRejectsSemanticallyMismatchedResult(t *testing.T) {
	name, email := "Tobari User", "tobari@example.com"
	otherName, otherEmail := "Other User", "other@example.com"
	change := tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityLiteral, Name: &name, Email: &email,
	}
	intent := operation.Intent{
		Command: "config git", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID},
		Impact: shellContextImpact(),
	}
	tests := []struct {
		name        string
		contextName string
		result      func() tobari.ContextReport
		wantFault   bool
	}{
		{
			name: "omitted Context returns active Context", result: func() tobari.ContextReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "default", true, change)
			},
		},
		{
			name: "explicit active Context is allowed", contextName: "project-tools", result: func() tobari.ContextReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", true, change)
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return configuredGitContextReport(tobari.TaskConfigShell, "project-tools", false, change)
			},
		},
		{
			name: "wrong explicit Context", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "other", false, change)
			},
		},
		{
			name: "omitted Context is not active", wantFault: true, result: func() tobari.ContextReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "default", false, change)
			},
		},
		{
			name: "wrong applied setting", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				return contextReport(tobari.TaskConfigGit, "project-tools")
			},
		},
		{
			name: "wrong literal name", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				wrong := change
				wrong.Name = &otherName
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong literal email", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				wrong := change
				wrong.Email = &otherEmail
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong cluster outcome", contextName: "project-tools", wantFault: true, result: func() tobari.ContextReport {
				report := configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, change)
				report.Cluster = tobari.ContextClusterStatusDefaultUpdated
				return report
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextRuntimeFake{configureGitResult: test.result()}
			_, err := New(fake).ConfigureGit(context.Background(), intent, test.contextName, change)
			if !test.wantFault {
				if err != nil || fake.configureGitCalls != 1 {
					t.Fatalf("ConfigureGit() error = %v, calls = %d", err, fake.configureGitCalls)
				}
				return
			}
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_context_report" ||
				public.Retryable || fake.configureGitCalls != 1 {
				t.Fatalf("ConfigureGit() fault = %+v, ok=%t, calls=%d", public, ok, fake.configureGitCalls)
			}
		})
	}
}

func TestConfigureGitMapsMissingContext(t *testing.T) {
	fake := &contextRuntimeFake{configureGitErr: tobari.ErrContextNotFound}
	service := New(fake)
	intent := operation.Intent{
		Command: "config git", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextGitIdentityTargetKind, ID: tobari.ContextGitIdentityTargetID},
		Impact: shellContextImpact(),
	}
	_, err := service.ConfigureGit(context.Background(), intent, "missing", tobari.ContextGitIdentitySetting{
		Source: tobari.ContextGitIdentityInherit,
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "context_not_found" || fake.configureGitCalls != 1 {
		t.Fatalf("missing Context Git configure = %#v, ok=%t, calls=%d", public, ok, fake.configureGitCalls)
	}
}

func contextImpact() operation.Impact {
	return operation.Impact{
		Cardinality:  operation.CardinalityOne,
		Notification: operation.DeclarationNo,
		AccessChange: operation.DeclarationYes,
		Destructive:  operation.DeclarationNo,
	}
}

func TestCreateValidatesIntentAndPassesRuntimeImageToPort(t *testing.T) {
	created := contextReport(tobari.TaskContextCreate, "project-tools")
	created.PolicyMode = tobari.ContextPolicyModeAdvanced
	fake := &contextRuntimeFake{createResult: created}
	service := New(fake)
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	result, err := service.Create(
		context.Background(), intent, "project-tools", tobari.OfficialRuntimeBase,
		tobari.ContextPolicyModeAdvanced, tobari.ContextSourceAccessReadWrite,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Name != "project-tools" || fake.createCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastImage != tobari.OfficialRuntimeBase || fake.lastMode != tobari.ContextPolicyModeAdvanced ||
		fake.lastSourceAccess != tobari.ContextSourceAccessReadWrite {
		t.Fatalf("result/call = %+v, calls=%d name=%q image=%q mode=%q", result, fake.createCalls, fake.lastName, fake.lastImage, fake.lastMode)
	}
}

func TestCreateRejectsInvalidImageBeforePortCall(t *testing.T) {
	fake := &contextRuntimeFake{createResult: contextReport(tobari.TaskContextCreate, "project-tools")}
	service := New(fake)
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Create(
		context.Background(), intent, "project-tools", "--pull=always",
		tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_context" {
		t.Fatalf("Create() fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 0 {
		t.Fatalf("CreateContext() calls = %d, want 0", fake.createCalls)
	}
}

func TestCreateRejectsInvalidSourceAccessBeforePortCall(t *testing.T) {
	fake := &contextRuntimeFake{createResult: contextReport(tobari.TaskContextCreate, "project-tools")}
	service := New(fake)
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Create(
		context.Background(), intent, "project-tools", tobari.OfficialRuntimeBase,
		tobari.ContextPolicyModeGuided, tobari.ContextSourceAccess("snapshot"),
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_context" {
		t.Fatalf("Create() fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 0 {
		t.Fatalf("CreateContext() calls = %d, want 0", fake.createCalls)
	}
}

func TestCreateDuplicateRecoversThroughContextList(t *testing.T) {
	fake := &contextRuntimeFake{createErr: tobari.ErrContextExists}
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).Create(
		context.Background(), intent, "review", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided,
		tobari.ContextSourceAccessReadWrite,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "context_exists" || public.Retryable ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "context list" ||
		public.NextActions[0].Reason != "List existing Contexts before choosing another name." {
		t.Fatalf("duplicate Context fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 1 || fake.lastName != "review" {
		t.Fatalf("CreateContext() calls/name = %d/%q, want 1/review", fake.createCalls, fake.lastName)
	}
}

func TestCreateWithCompositionPreservesTypedMethodSelection(t *testing.T) {
	policy := tobari.ContextMethodPolicy{
		Default:   tobari.ContextMethodExactReview,
		Overrides: []tobari.ContextMethodOverride{{Method: "GET", Decision: tobari.ContextMethodAllow}},
	}
	report := contextReport(tobari.TaskContextCreate, "coding")
	report.MethodPolicy = policy.Clone()
	fake := &contextRuntimeFake{createResult: report}
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	result, err := New(fake).CreateWithComposition(
		context.Background(), intent, "coding", tobari.OfficialRuntimeBase,
		tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
		tobari.ContextCreateComposition{
			NativeReadiness: tobari.ContextNativeReadinessEnabled,
			MethodPolicy:    &policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "coding" || fake.createCalls != 1 || fake.lastComposition.MethodPolicy == nil ||
		fake.lastComposition.MethodPolicy.Overrides[0].Decision != tobari.ContextMethodAllow {
		t.Fatalf("composed create result/call = %+v/%+v", result, fake.lastComposition)
	}
}

func TestCreationBaseAndCreatePreserveReviewedStandaloneComposition(t *testing.T) {
	base := tobari.ContextCreateBase{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ContextPolicyModeAdvanced,
		SourceAccess: tobari.ContextSourceAccessReadOnly, NativeReadiness: tobari.ContextNativeReadinessDisabled,
		MethodPolicy:     tobari.ContextMethodPolicy{Default: tobari.ContextMethodDeny, Overrides: []tobari.ContextMethodOverride{{Method: "GET", Decision: tobari.ContextMethodAllow}}},
		RuntimeSelection: "standard@1", ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity: tobari.DefaultContextGitIdentityReport(),
	}
	report := contextReport(tobari.TaskContextCreate, "standalone")
	report.ID = "018bcfe5-687b-7000-8000-000000000121"
	report.PolicyMode, report.SourceAccess, report.NativeReadiness = base.PolicyMode, base.SourceAccess, base.NativeReadiness
	report.MethodPolicy, report.ShellEnvironment, report.GitIdentity = base.MethodPolicy.Clone(), base.ShellEnvironment, base.GitIdentity
	fake := &contextRuntimeFake{baseResult: base, createResult: report}
	service := New(fake)
	observed, err := service.CreationBase(context.Background(), base.Name)
	if err != nil {
		t.Fatal(err)
	}
	observed.MethodPolicy.Overrides[0].Decision = tobari.ContextMethodDeny
	if fake.baseResult.MethodPolicy.Overrides[0].Decision != tobari.ContextMethodAllow {
		t.Fatal("application Base result aliases the infrastructure snapshot")
	}
	observed = base.Clone()
	policy := observed.MethodPolicy.Clone()
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID}, Impact: contextImpact(),
	}
	_, err = service.CreateWithComposition(
		context.Background(), intent, report.Name, tobari.BuiltinImageSelector, observed.PolicyMode, observed.SourceAccess,
		tobari.ContextCreateComposition{NativeReadiness: observed.NativeReadiness, MethodPolicy: &policy, RuntimeSelection: observed.RuntimeSelection, Base: &observed},
	)
	if err != nil {
		t.Fatal(err)
	}
	if fake.baseCalls != 1 || fake.createCalls != 1 || fake.lastComposition.Base == nil || fake.lastComposition.Base.Revision != base.Revision {
		t.Fatalf("Base observation/create calls = base %d create %d composition %+v", fake.baseCalls, fake.createCalls, fake.lastComposition)
	}
}

func TestCreateMapsChangedBaseToRetryableReviewFault(t *testing.T) {
	base := tobari.ContextCreateBase{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ContextPolicyModeGuided,
		SourceAccess: tobari.ContextSourceAccessReadWrite, NativeReadiness: tobari.ContextNativeReadinessEnabled,
		MethodPolicy:     tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview, Overrides: []tobari.ContextMethodOverride{}},
		RuntimeSelection: "standard@1", ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(), GitIdentity: tobari.DefaultContextGitIdentityReport(),
	}
	fake := &contextRuntimeFake{createErr: tobari.ErrContextBaseChanged}
	intent := operation.Intent{Command: "context create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID}, Impact: contextImpact()}
	policy := base.MethodPolicy.Clone()
	_, err := New(fake).CreateWithComposition(context.Background(), intent, "standalone", tobari.BuiltinImageSelector, base.PolicyMode, base.SourceAccess, tobari.ContextCreateComposition{NativeReadiness: base.NativeReadiness, MethodPolicy: &policy, RuntimeSelection: base.RuntimeSelection, Base: &base})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "context_base_changed" || !public.Retryable || len(public.NextActions) != 1 || public.NextActions[0].Command != "context list" {
		t.Fatalf("changed Base fault = %#v, ok=%t", public, ok)
	}
}

func TestCreateFirstWithCompositionRevalidatesKnownEmptyInsideLifecycle(t *testing.T) {
	empty := tobari.ContextListResult{
		Task: tobari.TaskContextList, ContextState: tobari.ContextObservationSyntheticDefault,
		Active: tobari.DefaultContextName, Items: []tobari.ContextSummary{},
	}
	report := contextReport(tobari.TaskContextCreate, tobari.DefaultContextName)
	fake := &contextRuntimeFake{listResult: empty, createResult: report}
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).CreateFirstWithComposition(
		context.Background(), intent, tobari.DefaultContextName, tobari.OfficialRuntimeBase,
		tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
		tobari.ContextCreateComposition{NativeReadiness: tobari.ContextNativeReadinessEnabled, RuntimeSelection: "standard@1"},
	)
	if err != nil || fake.listCalls != 1 || fake.createCalls != 1 {
		t.Fatalf("first create error/calls = %v, list=%d create=%d", err, fake.listCalls, fake.createCalls)
	}
}

func TestCreateFirstWithCompositionRejectsConcurrentCollectionChange(t *testing.T) {
	persisted := tobari.ContextListResult{
		Task: tobari.TaskContextList, ContextState: tobari.ContextObservationPersisted, Active: "other",
		Items: []tobari.ContextSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000099", Name: "other", ContextState: tobari.ContextObservationPersisted,
			Active: true, AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
			PolicyMode: tobari.ContextPolicyModeGuided, SourceAccess: tobari.ContextSourceAccessReadWrite,
			PolicyRevision: tobari.DefaultContextPolicyRevision(), NativeReadiness: tobari.ContextNativeReadinessEnabled,
			MethodPolicy:  tobari.ContextMethodPolicy{Default: tobari.ContextMethodExactReview, Overrides: []tobari.ContextMethodOverride{}},
			RuntimeStatus: tobari.ContextRuntimeStatusOfficial, RuntimeSelection: "standard@1",
			Bootstrap: tobari.ContextBootstrapReport{State: tobari.ContextBootstrapNotConfigured, Adapters: []string{}},
		}},
	}
	fake := &contextRuntimeFake{listResult: persisted}
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).CreateFirstWithComposition(
		context.Background(), intent, tobari.DefaultContextName, tobari.OfficialRuntimeBase,
		tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite,
		tobari.ContextCreateComposition{NativeReadiness: tobari.ContextNativeReadinessEnabled, RuntimeSelection: "standard@1"},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "context_collection_changed" || !public.Retryable ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "context list" || fake.createCalls != 0 {
		t.Fatalf("concurrent collection fault/calls = %#v, ok=%t create=%d", public, ok, fake.createCalls)
	}
}

func TestDeleteMapsCurrentWorkspaceAndConfirmedOutcomes(t *testing.T) {
	intent := operation.Intent{
		Command: "context delete", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ID: tobari.ContextCatalogTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes,
		},
	}
	fake := &contextRuntimeFake{deleteErr: tobari.ErrContextActive}
	_, err := New(fake).Delete(context.Background(), intent, "coding")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "context_is_current" {
		t.Fatalf("current Context delete fault = %#v, %v", public, err)
	}
	fake.deleteErr = tobari.ErrContextHasWorkspaces
	_, err = New(fake).Delete(context.Background(), intent, "coding")
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "context_has_workspaces" {
		t.Fatalf("Workspace-bound Context delete fault = %#v, %v", public, err)
	}
	fake.deleteErr = nil
	fake.deleteResult = tobari.ContextDeleteResult{
		Task: tobari.TaskContextDelete, ID: "018bcfe5-687b-7000-8000-000000000099",
		Name: "coding", Deleted: true, Cluster: tobari.ContextClusterStatusNotApplicable,
	}
	result, err := New(fake).Delete(context.Background(), intent, "coding")
	if err != nil || !result.Deleted || fake.deleteCalls != 3 {
		t.Fatalf("confirmed Context delete = %+v, calls=%d, error=%v", result, fake.deleteCalls, err)
	}
}

func TestUseMapsMissingContextAndDoesNotHidePortError(t *testing.T) {
	fake := &contextRuntimeFake{useErr: tobari.ErrContextNotFound}
	service := New(fake)
	intent := operation.Intent{
		Command: "context use", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Use(context.Background(), intent, "missing")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "context_not_found" {
		t.Fatalf("Use() fault = %#v, ok=%t", public, ok)
	}
	if fake.useCalls != 1 {
		t.Fatalf("UseContext() calls = %d, want 1", fake.useCalls)
	}

	fake.useErr = errors.New("private runtime failure")
	_, err = service.Use(context.Background(), intent, "missing")
	public, ok = fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "context_use_failed" {
		t.Fatalf("Use() runtime fault = %#v, ok=%t", public, ok)
	}
}

func TestRuntimeBuildUsesActiveContextFixedTarget(t *testing.T) {
	fake := &contextRuntimeFake{buildResult: contextReport(tobari.TaskRuntimeBuild, "default")}
	service := New(fake)
	intent := operation.Intent{
		Command: "runtime build", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
		Impact: contextImpact(),
	}
	result, err := service.BuildRuntime(context.Background(), intent)
	if err != nil {
		t.Fatalf("BuildRuntime() error = %v", err)
	}
	if result.Task != tobari.TaskRuntimeBuild || fake.buildCalls != 1 {
		t.Fatalf("result/calls = %+v/%d", result, fake.buildCalls)
	}
}

func TestRuntimeBuildForwardsPurposeBoundDiagnosticsAndProgress(t *testing.T) {
	fake := &contextRuntimeFake{buildResult: contextReport(tobari.TaskRuntimeBuild, "default")}
	service := New(fake)
	intent := operation.Intent{
		Command: "runtime build", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
		Impact: contextImpact(),
	}
	var diagnostics bytes.Buffer
	var events []tobari.RuntimeBuildProgress
	result, err := service.BuildRuntimeWithProgress(
		context.Background(), intent, &diagnostics,
		func(event tobari.RuntimeBuildProgress) { events = append(events, event) },
	)
	if err != nil {
		t.Fatalf("BuildRuntimeWithProgress() error = %v", err)
	}
	if result.Task != tobari.TaskRuntimeBuild || fake.buildProgressCalls != 1 ||
		diagnostics.String() != "synthetic BuildKit output\n" || len(events) != 1 {
		t.Fatalf("result/calls/diagnostics/events = %+v/%d/%q/%+v", result, fake.buildProgressCalls, diagnostics.String(), events)
	}
}

func TestRuntimeBuildMapsMissingRecipeBeforePromotion(t *testing.T) {
	fake := &contextRuntimeFake{buildErr: tobari.ErrRuntimeRecipeMissing}
	service := New(fake)
	intent := operation.Intent{
		Command: "runtime build", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
		Impact: contextImpact(),
	}
	_, err := service.BuildRuntime(context.Background(), intent)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "runtime_recipe_missing" {
		t.Fatalf("BuildRuntime() fault = %#v, ok=%t", public, ok)
	}
	if fake.buildCalls != 1 {
		t.Fatalf("BuildRuntime() calls = %d, want 1", fake.buildCalls)
	}
}

func TestContextRuntimeSetPinsExplicitReadyRevision(t *testing.T) {
	result := contextReport(tobari.TaskContextRuntimeSet, "coding")
	result.Runtime = tobari.ContextRuntimeReport{Kind: tobari.ContextRuntimeKindManaged, Status: tobari.ContextRuntimeStatusReady, Image: "tobari-runtime-frontend:aaaaaaaaaaaa", RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4}
	result.Image = result.Runtime.Image
	fake := &contextRuntimeFake{setRuntimeResult: result}
	service := New(fake)
	intent := operation.Intent{Command: "context runtime set", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ContextRuntimeBindingTargetKind, ID: tobari.ContextRuntimeBindingTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	updated, err := service.SetRuntime(context.Background(), intent, "coding", "frontend@4")
	if err != nil || updated.Runtime.Ordinal != 4 || fake.setRuntimeCalls != 1 || fake.lastName != "coding" || fake.lastRuntimeSelection != "frontend@4" {
		t.Fatalf("set Runtime = %+v/%v fake=%+v", updated, err, fake)
	}
}
