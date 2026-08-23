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
	listResult           tobari.ManifestListResult
	listResults          []tobari.ManifestListResult
	showResult           tobari.ManifestReport
	createResult         tobari.ManifestReport
	deleteResult         tobari.ManifestDeleteResult
	useResult            tobari.ManifestReport
	initResult           tobari.ManifestReport
	buildResult          tobari.ManifestReport
	setRuntimeResult     tobari.ManifestReport
	configureResult      tobari.ManifestReport
	configureGitResult   tobari.ManifestReport
	discoverAWSResult    tobari.ManifestAWSBootstrapDiscovery
	baseResult           tobari.ManifestCopySnapshot
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
	lastMode             tobari.ManifestPolicyMode
	lastSourceAccess     tobari.ManifestSourceAccess
	lastComposition      tobari.ManifestCreateComposition
	lastChange           tobari.ManifestShellEnvironmentSetting
	lastChanges          []tobari.ManifestShellEnvironmentSetting
	lastGitChange        tobari.ManifestGitIdentitySetting
	lastRuntimeSelection string
}

func (f *contextRuntimeFake) ManifestCopySnapshot(_ context.Context, name string) (tobari.ManifestCopySnapshot, error) {
	f.baseCalls++
	f.lastName = name
	return f.baseResult.Clone(), f.baseErr
}

func (f *contextRuntimeFake) DiscoverContextAWSBootstraps(context.Context) (tobari.ManifestAWSBootstrapDiscovery, error) {
	f.discoverAWSCalls++
	return f.discoverAWSResult, f.discoverAWSErr
}

func (f *contextRuntimeFake) ListContexts(context.Context) (tobari.ManifestListResult, error) {
	f.listCalls++
	if len(f.listResults) >= f.listCalls {
		return f.listResults[f.listCalls-1], f.listErr
	}
	return f.listResult, f.listErr
}

func (f *contextRuntimeFake) ShowContext(_ context.Context, name string) (tobari.ManifestReport, error) {
	f.showCalls++
	f.lastShowName = name
	return f.showResult, f.showErr
}

func (f *contextRuntimeFake) CreateContext(
	_ context.Context, name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess,
) (tobari.ManifestReport, error) {
	f.createCalls++
	f.lastName, f.lastImage, f.lastMode = name, image, mode
	f.lastSourceAccess = sourceAccess
	return f.createResult, f.createErr
}

func (f *contextRuntimeFake) CreateContextWithComposition(
	_ context.Context, name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
) (tobari.ManifestReport, error) {
	f.createCalls++
	f.lastName, f.lastImage, f.lastMode = name, image, mode
	f.lastSourceAccess, f.lastComposition = sourceAccess, composition.Clone()
	return f.createResult, f.createErr
}

func (f *contextRuntimeFake) DeleteContext(_ context.Context, name string) (tobari.ManifestDeleteResult, error) {
	f.deleteCalls++
	f.lastName = name
	return f.deleteResult, f.deleteErr
}

func (f *contextRuntimeFake) SetDefaultManifest(context.Context, string) (tobari.ManifestReport, error) {
	f.useCalls++
	return f.useResult, f.useErr
}

func (f *contextRuntimeFake) ConfigureContextShell(
	_ context.Context, name string, changes []tobari.ManifestShellEnvironmentSetting,
) (tobari.ManifestReport, error) {
	f.configureCalls++
	f.lastName = name
	f.lastChanges = append([]tobari.ManifestShellEnvironmentSetting(nil), changes...)
	if len(changes) > 0 {
		f.lastChange = changes[0]
	}
	return f.configureResult, f.configureErr
}

func (f *contextRuntimeFake) ConfigureContextGit(
	_ context.Context, name string, change tobari.ManifestGitIdentitySetting,
) (tobari.ManifestReport, error) {
	f.configureGitCalls++
	f.lastName, f.lastGitChange = name, change
	return f.configureGitResult, f.configureGitErr
}

func (f *contextRuntimeFake) InitRuntime(context.Context) (tobari.ManifestReport, error) {
	f.initCalls++
	return f.initResult, f.initErr
}

func (f *contextRuntimeFake) BuildRuntime(context.Context) (tobari.ManifestReport, error) {
	f.buildCalls++
	return f.buildResult, f.buildErr
}

func (f *contextRuntimeFake) SetContextRuntime(_ context.Context, name, selection string) (tobari.ManifestReport, error) {
	f.setRuntimeCalls++
	f.lastName, f.lastRuntimeSelection = name, selection
	return f.setRuntimeResult, f.setRuntimeErr
}

func (f *contextRuntimeFake) BuildRuntimeWithProgress(
	_ context.Context, diagnostics io.Writer, progress tobari.RuntimeBuildProgressSink,
) (tobari.ManifestReport, error) {
	f.buildCalls++
	f.buildProgressCalls++
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "synthetic BuildKit output\n")
	}
	if progress != nil {
		progress(tobari.RuntimeBuildProgress{
			Stage: tobari.RuntimeBuildStageBuild, Status: tobari.RuntimeBuildProgressStarted,
			WorkspaceManifestName: "default", Dockerfile: "/config/contexts/default/runtime/Dockerfile",
			PreviousImage: tobari.OfficialRuntimeBase, CandidateImage: "tobari-context-default:0123456789ab",
			Selection: tobari.RuntimeBuildSelectionUnchanged,
		})
	}
	return f.buildResult, f.buildErr
}

func contextReport(task, name string) tobari.ManifestReport {
	authentication := tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	if task == tobari.TaskManifestShow {
		authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerReady, Providers: []tobari.ManifestAuthProvider{}}
	}
	return tobari.ManifestReport{
		Task: task, ManifestState: tobari.ManifestObservationPersisted, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name, Default: task == tobari.TaskManifestDefaultSet,
		Desired:      testWorkspaceManifestRevision("f"),
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode:       tobari.ManifestPolicyModeGuided,
		SourceAccess:     tobari.ManifestSourceAccessReadWrite,
		PolicyRevision:   tobari.DefaultContextPolicyRevision(),
		NativeReadiness:  tobari.ManifestNativeReadinessEnabled,
		MethodPolicy:     tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Runtime: tobari.ManifestRuntimeReport{
			Kind: tobari.ManifestRuntimeKindOfficial, Status: tobari.ManifestRuntimeStatusOfficial,
			Image: tobari.OfficialRuntimeBase, RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1,
		},
		Cluster:        tobari.ManifestClusterStatusNotApplicable,
		Authentication: authentication,
		Stores: tobari.ManifestStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
		},
	}
}

func testWorkspaceManifestRevision(digit string) tobari.WorkspaceManifestRevision {
	digest := "sha256:" + strings.Repeat(digit, 64)
	return tobari.WorkspaceManifestRevision{Generation: 1, Revision: digest, BoundaryRevision: digest, ClusterProjectionRevision: digest, EntryRevision: digest, SessionDefaultsRevision: digest, CreationDefaultsRevision: digest}
}

func TestDiscoverAWSBootstrapsPreservesTypedRejectedSourceWithoutMutation(t *testing.T) {
	t.Parallel()
	runtime := &contextRuntimeFake{discoverAWSResult: tobari.ManifestAWSBootstrapDiscovery{
		State: tobari.ManifestBootstrapDiscoveryRejected, Reason: "Host AWS shared config is unsafe.", Candidates: []tobari.ManifestAWSBootstrapCandidate{},
	}}
	service := New(runtime)
	result, err := service.DiscoverAWSBootstraps(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.State != tobari.ManifestBootstrapDiscoveryRejected || runtime.discoverAWSCalls != 1 || runtime.createCalls != 0 || runtime.configureCalls != 0 {
		t.Fatalf("discovery result/calls = %+v / discover=%d create=%d configure=%d", result, runtime.discoverAWSCalls, runtime.createCalls, runtime.configureCalls)
	}
}

func configuredShellContextReport(
	task, name string, active bool, change tobari.ManifestShellEnvironmentSetting,
) tobari.ManifestReport {
	report := contextReport(task, name)
	report.Default = active
	for index := range report.ShellEnvironment {
		if report.ShellEnvironment[index].Variable == change.Variable {
			report.ShellEnvironment[index] = change
			break
		}
	}
	return report
}

func configuredGitContextReport(
	task, name string, active bool, change tobari.ManifestGitIdentitySetting,
) tobari.ManifestReport {
	report := contextReport(task, name)
	report.Default = active
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
		result      func() tobari.ManifestReport
		wantFault   bool
	}{
		{
			name: "explicit active Workspace Manifest is allowed", contextName: "project-tools", result: func() tobari.ManifestReport {
				report := contextReport(tobari.TaskManifestShow, "project-tools")
				report.Default = true
				return report
			},
		},
		{
			name: "omitted Workspace Manifest returns active Workspace Manifest", result: func() tobari.ManifestReport {
				report := contextReport(tobari.TaskManifestShow, "default")
				report.Default = true
				return report
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return contextReport(tobari.TaskConfigShell, "project-tools")
			},
		},
		{
			name: "wrong explicit Workspace Manifest", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return contextReport(tobari.TaskManifestShow, "other")
			},
		},
		{
			name: "omitted Workspace Manifest is not active", wantFault: true, result: func() tobari.ManifestReport {
				return contextReport(tobari.TaskManifestShow, "default")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextRuntimeFake{showResult: test.result()}
			result, err := New(fake).Show(context.Background(), test.contextName)
			if !test.wantFault {
				if err != nil || result.Task != tobari.TaskManifestShow || fake.showCalls != 1 ||
					fake.lastShowName != test.contextName {
					t.Fatalf("Show() result/error/calls/name = %+v / %v / %d / %q", result, err, fake.showCalls, fake.lastShowName)
				}
				return
			}
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_manifest_report" ||
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
	change := tobari.ManifestShellEnvironmentSetting{
		Variable: "PS1", Source: tobari.ManifestShellEnvironmentLiteral, Value: &empty,
	}
	fake := &contextRuntimeFake{
		configureResult: configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, change),
	}
	service := New(fake)
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID},
		Impact: shellContextImpact(),
	}
	result, err := service.ConfigureShell(context.Background(), intent, "project-tools", []tobari.ManifestShellEnvironmentSetting{change})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskConfigShell || fake.configureCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastChange.Value == nil || *fake.lastChange.Value != "" {
		t.Fatalf("result/call = %+v / %d %q %+v", result, fake.configureCalls, fake.lastName, fake.lastChange)
	}

	fake.configureCalls = 0
	change.Variable = "PATH"
	_, err = service.ConfigureShell(context.Background(), intent, "project-tools", []tobari.ManifestShellEnvironmentSetting{change})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_shell_environment" || fake.configureCalls != 0 {
		t.Fatalf("invalid configure = %#v, ok=%t, calls=%d", public, ok, fake.configureCalls)
	}
}

func TestConfigureShellSendsOneAtomicStagedBatch(t *testing.T) {
	changes := []tobari.ManifestShellEnvironmentSetting{
		{Variable: "COLORTERM", Source: tobari.ManifestShellEnvironmentInherit},
		{Variable: "NO_COLOR", Source: tobari.ManifestShellEnvironmentDefault},
	}
	report := contextReport(tobari.TaskConfigShell, "project-tools")
	report.Default = false
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
		Target: operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID},
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
	change := tobari.ManifestShellEnvironmentSetting{
		Variable: "PS1", Source: tobari.ManifestShellEnvironmentLiteral, Value: &empty,
	}
	intent := operation.Intent{
		Command: "config shell", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID},
		Impact: shellContextImpact(),
	}
	tests := []struct {
		name        string
		contextName string
		result      func() tobari.ManifestReport
		wantFault   bool
	}{
		{
			name: "omitted Workspace Manifest returns active Workspace Manifest", result: func() tobari.ManifestReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "default", true, change)
			},
		},
		{
			name: "explicit active Workspace Manifest is allowed", contextName: "project-tools", result: func() tobari.ManifestReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "project-tools", true, change)
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return configuredShellContextReport(tobari.TaskConfigGit, "project-tools", false, change)
			},
		},
		{
			name: "wrong explicit Workspace Manifest", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "other", false, change)
			},
		},
		{
			name: "omitted Workspace Manifest is not active", wantFault: true, result: func() tobari.ManifestReport {
				return configuredShellContextReport(tobari.TaskConfigShell, "default", false, change)
			},
		},
		{
			name: "wrong applied setting", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return contextReport(tobari.TaskConfigShell, "project-tools")
			},
		},
		{
			name: "wrong literal value", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				wrong := change
				wrong.Value = &different
				return configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong cluster outcome", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				report := configuredShellContextReport(tobari.TaskConfigShell, "project-tools", false, change)
				report.Cluster = tobari.ManifestClusterStatusReconciled
				return report
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextRuntimeFake{configureResult: test.result()}
			_, err := New(fake).ConfigureShell(context.Background(), intent, test.contextName, []tobari.ManifestShellEnvironmentSetting{change})
			if !test.wantFault {
				if err != nil || fake.configureCalls != 1 {
					t.Fatalf("ConfigureShell() error = %v, calls = %d", err, fake.configureCalls)
				}
				return
			}
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_manifest_report" ||
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
		Target: operation.TargetRef{Kind: tobari.ManifestShellTargetKind, ID: tobari.ManifestShellTargetID},
		Impact: shellContextImpact(),
	}
	_, err := service.ConfigureShell(context.Background(), intent, "missing", []tobari.ManifestShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ManifestShellEnvironmentInherit,
	}})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "manifest_not_found" || fake.configureCalls != 1 {
		t.Fatalf("missing Workspace Manifest configure = %#v, ok=%t, calls=%d", public, ok, fake.configureCalls)
	}
}

func TestConfigureGitValidatesInputAndIntentBeforeMutation(t *testing.T) {
	name, email := "Tobari User", "tobari@example.com"
	setting := tobari.ManifestGitIdentitySetting{
		Source: tobari.ManifestGitIdentityLiteral, Name: &name, Email: &email,
	}
	fake := &contextRuntimeFake{
		configureGitResult: configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, setting),
	}
	service := New(fake)
	intent := operation.Intent{
		Command: "config git", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID},
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
	invalid := tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityLiteral, Name: &name}
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
	change := tobari.ManifestGitIdentitySetting{
		Source: tobari.ManifestGitIdentityLiteral, Name: &name, Email: &email,
	}
	intent := operation.Intent{
		Command: "config git", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID},
		Impact: shellContextImpact(),
	}
	tests := []struct {
		name        string
		contextName string
		result      func() tobari.ManifestReport
		wantFault   bool
	}{
		{
			name: "omitted Workspace Manifest returns active Workspace Manifest", result: func() tobari.ManifestReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "default", true, change)
			},
		},
		{
			name: "explicit active Workspace Manifest is allowed", contextName: "project-tools", result: func() tobari.ManifestReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", true, change)
			},
		},
		{
			name: "wrong task", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return configuredGitContextReport(tobari.TaskConfigShell, "project-tools", false, change)
			},
		},
		{
			name: "wrong explicit Workspace Manifest", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "other", false, change)
			},
		},
		{
			name: "omitted Workspace Manifest is not active", wantFault: true, result: func() tobari.ManifestReport {
				return configuredGitContextReport(tobari.TaskConfigGit, "default", false, change)
			},
		},
		{
			name: "wrong applied setting", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				return contextReport(tobari.TaskConfigGit, "project-tools")
			},
		},
		{
			name: "wrong literal name", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				wrong := change
				wrong.Name = &otherName
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong literal email", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				wrong := change
				wrong.Email = &otherEmail
				return configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, wrong)
			},
		},
		{
			name: "wrong cluster outcome", contextName: "project-tools", wantFault: true, result: func() tobari.ManifestReport {
				report := configuredGitContextReport(tobari.TaskConfigGit, "project-tools", false, change)
				report.Cluster = tobari.ManifestClusterStatusDefaultManifestUpdated
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
			if !ok || public.Kind != fault.KindContract || public.Code != "invalid_manifest_report" ||
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
		Target: operation.TargetRef{Kind: tobari.ManifestGitIdentityTargetKind, ID: tobari.ManifestGitIdentityTargetID},
		Impact: shellContextImpact(),
	}
	_, err := service.ConfigureGit(context.Background(), intent, "missing", tobari.ManifestGitIdentitySetting{
		Source: tobari.ManifestGitIdentityInherit,
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "manifest_not_found" || fake.configureGitCalls != 1 {
		t.Fatalf("missing Workspace Manifest Git configure = %#v, ok=%t, calls=%d", public, ok, fake.configureGitCalls)
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
	created := contextReport(tobari.TaskManifestCreate, "project-tools")
	created.PolicyMode = tobari.ManifestPolicyModeAdvanced
	fake := &contextRuntimeFake{createResult: created}
	service := New(fake)
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	result, err := service.Create(
		context.Background(), intent, "project-tools", tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeAdvanced, tobari.ManifestSourceAccessReadWrite,
	)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Name != "project-tools" || fake.createCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastImage != tobari.OfficialRuntimeBase || fake.lastMode != tobari.ManifestPolicyModeAdvanced ||
		fake.lastSourceAccess != tobari.ManifestSourceAccessReadWrite {
		t.Fatalf("result/call = %+v, calls=%d name=%q image=%q mode=%q", result, fake.createCalls, fake.lastName, fake.lastImage, fake.lastMode)
	}
}

func TestCreateRejectsInvalidImageBeforePortCall(t *testing.T) {
	fake := &contextRuntimeFake{createResult: contextReport(tobari.TaskManifestCreate, "project-tools")}
	service := New(fake)
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Create(
		context.Background(), intent, "project-tools", "--pull=always",
		tobari.ManifestPolicyModeGuided, tobari.ManifestSourceAccessReadWrite,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_context" {
		t.Fatalf("Create() fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 0 {
		t.Fatalf("CreateWorkspace Manifest() calls = %d, want 0", fake.createCalls)
	}
}

func TestCreateRejectsInvalidSourceAccessBeforePortCall(t *testing.T) {
	fake := &contextRuntimeFake{createResult: contextReport(tobari.TaskManifestCreate, "project-tools")}
	service := New(fake)
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Create(
		context.Background(), intent, "project-tools", tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeGuided, tobari.ManifestSourceAccess("snapshot"),
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_context" {
		t.Fatalf("Create() fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 0 {
		t.Fatalf("CreateWorkspace Manifest() calls = %d, want 0", fake.createCalls)
	}
}

func TestCreateDuplicateRecoversThroughContextList(t *testing.T) {
	fake := &contextRuntimeFake{createErr: tobari.ErrContextExists}
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).Create(
		context.Background(), intent, "review", tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
		tobari.ManifestSourceAccessReadWrite,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "manifest_exists" || public.Retryable ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "manifest list" ||
		public.NextActions[0].Reason != "List existing Workspace Manifests before choosing another name." {
		t.Fatalf("duplicate Workspace Manifest fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 1 || fake.lastName != "review" {
		t.Fatalf("CreateWorkspace Manifest() calls/name = %d/%q, want 1/review", fake.createCalls, fake.lastName)
	}
}

func TestCreateWithCompositionPreservesTypedMethodSelection(t *testing.T) {
	policy := tobari.ManifestMethodPolicy{
		Default:   tobari.ManifestMethodExactReview,
		Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}},
	}
	report := contextReport(tobari.TaskManifestCreate, "coding")
	report.MethodPolicy = policy.Clone()
	fake := &contextRuntimeFake{createResult: report}
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	result, err := New(fake).CreateWithComposition(
		context.Background(), intent, "coding", tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeGuided, tobari.ManifestSourceAccessReadWrite,
		tobari.ManifestCreateComposition{
			NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			MethodPolicy:    &policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Name != "coding" || fake.createCalls != 1 || fake.lastComposition.MethodPolicy == nil ||
		fake.lastComposition.MethodPolicy.Overrides[0].Decision != tobari.ManifestMethodAllow {
		t.Fatalf("composed create result/call = %+v/%+v", result, fake.lastComposition)
	}
}

func TestCreationBaseAndCreatePreserveReviewedStandaloneComposition(t *testing.T) {
	base := tobari.ManifestCopySnapshot{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ManifestPolicyModeAdvanced,
		Desired:      testWorkspaceManifestRevision("a"),
		SourceAccess: tobari.ManifestSourceAccessReadOnly, NativeReadiness: tobari.ManifestNativeReadinessDisabled,
		MethodPolicy:     tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodDeny, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		RuntimeSelection: "standard@1", ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		RuntimeBinding: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1, Image: tobari.OfficialRuntimeBase},
		GitIdentity:    tobari.DefaultContextGitIdentityReport(),
	}
	report := contextReport(tobari.TaskManifestCreate, "standalone")
	report.ID = "018bcfe5-687b-7000-8000-000000000121"
	report.PolicyMode, report.SourceAccess, report.NativeReadiness = base.PolicyMode, base.SourceAccess, base.NativeReadiness
	report.MethodPolicy, report.ShellEnvironment, report.GitIdentity = base.MethodPolicy.Clone(), base.ShellEnvironment, base.GitIdentity
	fake := &contextRuntimeFake{baseResult: base, createResult: report}
	service := New(fake)
	observed, err := service.CopySnapshot(context.Background(), base.Name)
	if err != nil {
		t.Fatal(err)
	}
	observed.MethodPolicy.Overrides[0].Decision = tobari.ManifestMethodDeny
	if fake.baseResult.MethodPolicy.Overrides[0].Decision != tobari.ManifestMethodAllow {
		t.Fatal("application Base result aliases the infrastructure snapshot")
	}
	observed = base.Clone()
	policy := observed.MethodPolicy.Clone()
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID}, Impact: contextImpact(),
	}
	_, err = service.CreateWithComposition(
		context.Background(), intent, report.Name, tobari.BuiltinImageSelector, observed.PolicyMode, observed.SourceAccess,
		tobari.ManifestCreateComposition{NativeReadiness: observed.NativeReadiness, MethodPolicy: &policy, RuntimeSelection: observed.RuntimeSelection, CopyFrom: &observed},
	)
	if err != nil {
		t.Fatal(err)
	}
	if fake.baseCalls != 1 || fake.createCalls != 1 || fake.lastComposition.CopyFrom == nil || fake.lastComposition.CopyFrom.Revision != base.Revision {
		t.Fatalf("Base observation/create calls = base %d create %d composition %+v", fake.baseCalls, fake.createCalls, fake.lastComposition)
	}
}

func TestCreateMapsChangedBaseToRetryableReviewFault(t *testing.T) {
	base := tobari.ManifestCopySnapshot{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ManifestPolicyModeGuided,
		Desired:      testWorkspaceManifestRevision("a"),
		SourceAccess: tobari.ManifestSourceAccessReadWrite, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
		MethodPolicy:     tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		RuntimeSelection: "standard@1", RuntimeBinding: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1, Image: tobari.OfficialRuntimeBase}, ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(), GitIdentity: tobari.DefaultContextGitIdentityReport(),
	}
	fake := &contextRuntimeFake{createErr: tobari.ErrManifestCopySourceChanged}
	intent := operation.Intent{Command: "manifest create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID}, Impact: contextImpact()}
	policy := base.MethodPolicy.Clone()
	_, err := New(fake).CreateWithComposition(context.Background(), intent, "standalone", tobari.BuiltinImageSelector, base.PolicyMode, base.SourceAccess, tobari.ManifestCreateComposition{NativeReadiness: base.NativeReadiness, MethodPolicy: &policy, RuntimeSelection: base.RuntimeSelection, CopyFrom: &base})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "manifest_copy_source_changed" || !public.Retryable || len(public.NextActions) != 1 || public.NextActions[0].Command != "manifest list" {
		t.Fatalf("changed Base fault = %#v, ok=%t", public, ok)
	}
}

func TestCreateFirstWithCompositionRevalidatesKnownEmptyInsideLifecycle(t *testing.T) {
	empty := tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
		Items: []tobari.ManifestSummary{},
	}
	report := contextReport(tobari.TaskManifestCreate, tobari.DefaultManifestName)
	fake := &contextRuntimeFake{listResult: empty, createResult: report}
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).CreateFirstWithComposition(
		context.Background(), intent, tobari.DefaultManifestName, tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeGuided, tobari.ManifestSourceAccessReadWrite,
		tobari.ManifestCreateComposition{NativeReadiness: tobari.ManifestNativeReadinessEnabled, RuntimeSelection: "standard@1"},
	)
	if err != nil || fake.listCalls != 1 || fake.createCalls != 1 {
		t.Fatalf("first create error/calls = %v, list=%d create=%d", err, fake.listCalls, fake.createCalls)
	}
}

func TestCreateFirstWithCompositionRejectsConcurrentCollectionChange(t *testing.T) {
	persisted := tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: "018bcfe5-687b-7000-8000-000000000099", DefaultManifest: "other",
		Items: []tobari.ManifestSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000099", Name: "other", ManifestState: tobari.ManifestObservationPersisted,
			Default: true, AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
			Desired:    testWorkspaceManifestRevision("e"),
			PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite,
			PolicyRevision: tobari.DefaultContextPolicyRevision(), NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			MethodPolicy:  tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
			RuntimeStatus: tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: "standard@1",
			Bootstrap: tobari.ManifestBootstrapReport{State: tobari.ManifestBootstrapNotConfigured, Adapters: []string{}},
		}},
	}
	fake := &contextRuntimeFake{listResult: persisted}
	intent := operation.Intent{
		Command: "manifest create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ParentID: tobari.ManifestCatalogTargetID},
		Impact: contextImpact(),
	}
	_, err := New(fake).CreateFirstWithComposition(
		context.Background(), intent, tobari.DefaultManifestName, tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeGuided, tobari.ManifestSourceAccessReadWrite,
		tobari.ManifestCreateComposition{NativeReadiness: tobari.ManifestNativeReadinessEnabled, RuntimeSelection: "standard@1"},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "manifest_collection_changed" || !public.Retryable ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "manifest list" || fake.createCalls != 0 {
		t.Fatalf("concurrent collection fault/calls = %#v, ok=%t create=%d", public, ok, fake.createCalls)
	}
}

func TestDeleteMapsCurrentWorkspaceAndConfirmedOutcomes(t *testing.T) {
	intent := operation.Intent{
		Command: "manifest delete", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestCatalogTargetKind, ID: tobari.ManifestCatalogTargetID},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes,
		},
	}
	fake := &contextRuntimeFake{deleteErr: tobari.ErrContextActive}
	_, err := New(fake).Delete(context.Background(), intent, "coding")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "manifest_is_default" {
		t.Fatalf("current Workspace Manifest delete fault = %#v, %v", public, err)
	}
	fake.deleteErr = tobari.ErrContextHasWorkspaces
	_, err = New(fake).Delete(context.Background(), intent, "coding")
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "manifest_has_workspaces" {
		t.Fatalf("Workspace-bound Workspace Manifest delete fault = %#v, %v", public, err)
	}
	fake.deleteErr = nil
	fake.deleteResult = tobari.ManifestDeleteResult{
		Task: tobari.TaskManifestDelete, ID: "018bcfe5-687b-7000-8000-000000000099",
		Name: "coding", Deleted: true, Cluster: tobari.ManifestClusterStatusNotApplicable,
	}
	result, err := New(fake).Delete(context.Background(), intent, "coding")
	if err != nil || !result.Deleted || fake.deleteCalls != 3 {
		t.Fatalf("confirmed Workspace Manifest delete = %+v, calls=%d, error=%v", result, fake.deleteCalls, err)
	}
}

func TestUseMapsMissingContextAndDoesNotHidePortError(t *testing.T) {
	fake := &contextRuntimeFake{useErr: tobari.ErrContextNotFound}
	service := New(fake)
	intent := operation.Intent{
		Command: "manifest default set", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestTargetKind, ID: tobari.DefaultManifestSelectionTargetID},
		Impact: contextImpact(),
	}
	_, err := service.Use(context.Background(), intent, "missing")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindNotFound || public.Code != "manifest_not_found" {
		t.Fatalf("Use() fault = %#v, ok=%t", public, ok)
	}
	if fake.useCalls != 1 {
		t.Fatalf("SetDefaultManifest() calls = %d, want 1", fake.useCalls)
	}

	fake.useErr = errors.New("private runtime failure")
	_, err = service.Use(context.Background(), intent, "missing")
	public, ok = fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindRejected || public.Code != "manifest_default_set_failed" {
		t.Fatalf("Use() runtime fault = %#v, ok=%t", public, ok)
	}
}

func TestRuntimeBuildUsesActiveContextFixedTarget(t *testing.T) {
	fake := &contextRuntimeFake{buildResult: contextReport(tobari.TaskRuntimeBuild, "default")}
	service := New(fake)
	intent := operation.Intent{
		Command: "runtime build", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
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
		Target: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
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
		Target: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID},
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
	result := contextReport(tobari.TaskManifestRuntimeSet, "coding")
	result.Runtime = tobari.ManifestRuntimeReport{Kind: tobari.ManifestRuntimeKindManaged, Status: tobari.ManifestRuntimeStatusReady, Image: "tobari-runtime-frontend:aaaaaaaaaaaa", RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4}
	result.Image = result.Runtime.Image
	fake := &contextRuntimeFake{setRuntimeResult: result}
	service := New(fake)
	intent := operation.Intent{Command: "manifest runtime set", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ManifestRuntimeBindingTargetKind, ID: tobari.ManifestRuntimeBindingTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	updated, err := service.SetRuntime(context.Background(), intent, "coding", "frontend@4")
	if err != nil || updated.Runtime.Ordinal != 4 || fake.setRuntimeCalls != 1 || fake.lastName != "coding" || fake.lastRuntimeSelection != "frontend@4" {
		t.Fatalf("set Runtime = %+v/%v fake=%+v", updated, err, fake)
	}
}
