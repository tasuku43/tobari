package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextCLI fakeContextRuntime

type runtimeCatalogCLI struct {
	buildLog     string
	buildErr     error
	manifest     tobari.RuntimeManifest
	list         []tobari.RuntimeSummary
	listCalls    int
	showCalls    int
	historyCalls int
	buildCalls   int
	lastBuild    string
	createCalls  int
	lastCreate   string
	lastBase     tobari.RuntimeCopySource
}

func testRuntimeManifest() tobari.RuntimeManifest {
	return tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Kind: tobari.RuntimeKindManaged, SourcePath: "/config/runtimes/frontend/source", Revisions: []tobari.RuntimeRevision{}}
}

func readyRuntimeManifest() tobari.RuntimeManifest {
	manifest := testRuntimeManifest()
	manifest.Revisions = []tobari.RuntimeRevision{{
		Ordinal: 1, Revision: "sha256:" + strings.Repeat("a", 64),
		Image: "tobari-runtime-frontend:aaaaaaaaaaaa", ImageDigest: "sha256:" + strings.Repeat("b", 64),
		CreatedAt: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC), SnapshotPath: "/config/runtimes/frontend/snapshots/aaaaaaaaaaaa",
	}}
	return manifest
}

func readyRuntimeManifestWithHistory() tobari.RuntimeManifest {
	manifest := readyRuntimeManifest()
	manifest.Revisions = append(manifest.Revisions, tobari.RuntimeRevision{
		Ordinal: 2, Revision: "sha256:" + strings.Repeat("c", 64),
		Image: "tobari-runtime-frontend:cccccccccccc", ImageDigest: "sha256:" + strings.Repeat("d", 64),
		CreatedAt: time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC), SnapshotPath: "/config/runtimes/frontend/snapshots/cccccccccccc",
	})
	return manifest
}

func runtimeReviewList(manifest tobari.RuntimeManifest) []tobari.RuntimeSummary {
	standard := tobari.RuntimeSummary{ID: tobari.StandardRuntimeID, RuntimeRef: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin, Ready: true, Head: 1, Revision: "sha256:" + strings.Repeat("0", 64)}
	standard.RevisionRef = tobari.RuntimeRevisionRef(standard.ID, standard.Revision)
	return []tobari.RuntimeSummary{
		standard,
		tobari.RuntimeSummaryFrom(manifest),
	}
}

func (f *runtimeCatalogCLI) runtimeManifest() tobari.RuntimeManifest {
	if f.manifest.SchemaVersion != 0 {
		return f.manifest
	}
	return testRuntimeManifest()
}

func (f *runtimeCatalogCLI) ListRuntimes(context.Context) (tobari.RuntimeListResult, error) {
	f.listCalls++
	items := f.list
	if items == nil {
		items = []tobari.RuntimeSummary{}
	}
	return tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: items}, nil
}
func (f *runtimeCatalogCLI) ShowRuntime(context.Context, string) (tobari.RuntimeReport, error) {
	f.showCalls++
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeShow, Runtime: f.runtimeManifest()}, nil
}
func (f *runtimeCatalogCLI) RuntimeHistory(context.Context, string) (tobari.RuntimeReport, error) {
	f.historyCalls++
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeHistory, Runtime: f.runtimeManifest()}, nil
}
func (f *runtimeCatalogCLI) CreateRuntime(_ context.Context, name string, base tobari.RuntimeCopySource) (tobari.RuntimeReport, error) {
	f.createCalls++
	f.lastCreate = name
	f.lastBase = base
	manifest := f.runtimeManifest()
	manifest.Name = name
	manifest.Revisions = []tobari.RuntimeRevision{}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}, nil
}
func (f *runtimeCatalogCLI) BuildManagedRuntime(_ context.Context, name string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	f.buildCalls++
	f.lastBuild = name
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, f.buildLog)
	}
	if f.buildErr != nil {
		return tobari.RuntimeReport{}, f.buildErr
	}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: f.runtimeManifest(), NoChange: true}, nil
}

func (f *contextCLI) ListContexts(context.Context) (tobari.ManifestListResult, error) {
	f.listCalls++
	if len(f.listResults) >= f.listCalls {
		return f.listResults[f.listCalls-1], nil
	}
	if f.list.Task == "" {
		return tobari.ManifestListResult{
			Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
			Items: []tobari.ManifestSummary{},
		}, nil
	}
	return f.list, nil
}

func (f *contextCLI) ShowContext(_ context.Context, name string) (tobari.ManifestReport, error) {
	f.showCalls++
	if f.reports != nil && name != "" {
		if report, ok := f.reports[name]; ok {
			f.report = report
			return report, f.showErr
		}
	}
	return f.report, f.showErr
}

func (f *contextCLI) CreateContext(
	_ context.Context, name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess,
) (tobari.ManifestReport, error) {
	f.createCalls++
	f.report = contextCLIReport(tobari.TaskManifestCreate, name, false, image, mode)
	f.report.SourceAccess = sourceAccess
	return f.report, nil
}

func (f *contextCLI) CreateContextWithComposition(
	_ context.Context, name, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
) (tobari.ManifestReport, error) {
	f.createCalls++
	f.lastComposition = composition.Clone()
	f.report = contextCLIReport(tobari.TaskManifestCreate, name, false, image, mode)
	f.report.SourceAccess = sourceAccess
	f.report.NativeReadiness = composition.NativeReadiness
	if composition.MethodPolicy != nil {
		f.report.MethodPolicy = composition.MethodPolicy.Clone()
	}
	if composition.CopyFrom != nil {
		base := composition.CopyFrom.Clone()
		f.report.ShellEnvironment = base.ShellEnvironment
		f.report.GitIdentity = base.GitIdentity
	}
	f.report.Bootstrap = tobari.ManifestBootstrapReportFrom(composition.Bootstrap)
	return f.report, nil
}

func syntheticContextAWSBootstrap(t *testing.T) tobari.ManifestBootstrapSnapshot {
	t.Helper()
	snapshot, err := tobari.NewContextBootstrapSnapshot(1, tobari.ManifestAWSBootstrap{Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start", SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func (f *contextCLI) PrepareContextAWSBootstrap(context.Context, string) (tobari.ManifestBootstrapSnapshot, error) {
	f.prepareBootstrapCalls++
	return tobari.NewContextBootstrapSnapshot(1, tobari.ManifestAWSBootstrap{Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start", SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"}, AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json"})
}

func (f *contextCLI) PreviewContextAWSBootstrap(_ context.Context, name, _ string) (tobari.ManifestBootstrapPreview, error) {
	snapshot, err := f.PrepareContextAWSBootstrap(context.Background(), "engineering")
	if err != nil {
		return tobari.ManifestBootstrapPreview{}, err
	}
	return tobari.NewContextBootstrapPreview(name, nil, snapshot)
}

func (f *contextCLI) ConfigureContextAWSBootstrap(_ context.Context, name, _, _ string, remove bool) (tobari.ManifestReport, error) {
	f.configureBootstrapCalls++
	f.report.Task = tobari.TaskConfigBootstrapAWS
	if name != "" {
		f.report.Name = name
	} else {
		f.report.Default = true
	}
	if remove {
		f.report.Bootstrap = tobari.ManifestBootstrapReportFrom(nil)
	} else {
		snapshot, err := f.PrepareContextAWSBootstrap(context.Background(), "engineering")
		if err != nil {
			return tobari.ManifestReport{}, err
		}
		f.report.Bootstrap = tobari.ManifestBootstrapReportFrom(&snapshot)
	}
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) PrepareContextEKSBootstrap(_ context.Context, base tobari.ManifestBootstrapSnapshot, _ string) (tobari.ManifestBootstrapSnapshot, error) {
	f.prepareBootstrapCalls++
	return base, nil
}

func (f *contextCLI) PreviewContextEKSBootstrap(context.Context, string, string) (tobari.ManifestBootstrapPreview, error) {
	return tobari.ManifestBootstrapPreview{}, errors.New("EKS preview is not used by this fake")
}

func (f *contextCLI) ConfigureContextEKSBootstrap(_ context.Context, name, kubeContext, _ string, remove bool) (tobari.ManifestReport, error) {
	f.configureBootstrapCalls++
	f.report.Task = tobari.TaskConfigBootstrapEKS
	if name != "" {
		f.report.Name = name
	}
	if remove {
		f.report.Bootstrap = tobari.ManifestBootstrapReport{State: tobari.ManifestBootstrapConfigured, Generation: 2, Revision: "sha256:" + strings.Repeat("b", 64), Adapters: []string{tobari.ManifestBootstrapAdapterAWS}, AWSProfile: "engineering"}
	} else {
		f.report.Bootstrap = tobari.ManifestBootstrapReport{State: tobari.ManifestBootstrapConfigured, Generation: 2, Revision: "sha256:" + strings.Repeat("a", 64), Adapters: []string{tobari.ManifestBootstrapAdapterAWS, tobari.ManifestBootstrapAdapterEKS}, AWSProfile: "engineering", EKSContext: kubeContext}
	}
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) DeleteContext(_ context.Context, name string) (tobari.ManifestDeleteResult, error) {
	f.deleteCalls++
	return tobari.ManifestDeleteResult{
		Task: tobari.TaskManifestDelete, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name,
		Deleted: true, Cluster: tobari.ManifestClusterStatusNotApplicable,
	}, f.deleteErr
}

func (f *contextCLI) SetDefaultManifest(context.Context, string) (tobari.ManifestReport, error) {
	f.useCalls++
	f.report.Task = tobari.TaskManifestDefaultSet
	f.report.Default = true
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) SetContextRuntime(_ context.Context, name, selection string) (tobari.ManifestReport, error) {
	f.setRuntimeCalls++
	f.lastRuntimeContext = name
	f.lastRuntimeSelection = selection
	f.report.Task = tobari.TaskManifestRuntimeSet
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	if name != "" {
		f.report.Name = name
	}
	if selection == tobari.StandardRuntimeName {
		f.report.Runtime = standardContextRuntimeReport(tobari.OfficialRuntimeBase)
		f.report.Image = tobari.OfficialRuntimeBase
	} else {
		runtimeName, ordinal, err := tobari.ParseRuntimeSelection(selection)
		if err != nil {
			return tobari.ManifestReport{}, err
		}
		f.report.Runtime = tobari.ManifestRuntimeReport{
			Kind: tobari.ManifestRuntimeKindManaged, Status: tobari.ManifestRuntimeStatusReady,
			Image:     "tobari-runtime-" + runtimeName + ":aaaaaaaaaaaa",
			RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: runtimeName,
			Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: ordinal,
		}
		f.report.Image = f.report.Runtime.Image
	}
	return f.report, nil
}

func (f *contextCLI) ConfigureContextShell(
	_ context.Context, name string, changes []tobari.ManifestShellEnvironmentSetting,
) (tobari.ManifestReport, error) {
	f.configureCalls++
	f.lastShellChanges = append([]tobari.ManifestShellEnvironmentSetting(nil), changes...)
	if len(changes) > 0 {
		f.lastShellChange = changes[0]
	}
	f.lastShellContext = name
	f.report.Task = tobari.TaskConfigShell
	if name == "" {
		f.report.Default = true
	} else {
		f.report.Name = name
	}
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	overrides := []tobari.ManifestShellEnvironmentSetting{}
	for _, change := range changes {
		if change.Source != tobari.ManifestShellEnvironmentDefault {
			overrides = append(overrides, change)
		}
	}
	f.report.ShellEnvironment, _ = tobari.CompleteContextShellEnvironment(overrides)
	return f.report, nil
}

func (f *contextCLI) ConfigureContextGit(
	_ context.Context, name string, change tobari.ManifestGitIdentitySetting,
) (tobari.ManifestReport, error) {
	f.configureGitCalls++
	f.lastGitChange = change
	f.lastGitContext = name
	f.report.Task = tobari.TaskConfigGit
	if name == "" {
		f.report.Default = true
	} else {
		f.report.Name = name
	}
	f.report.GitIdentity = change
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) InitRuntime(context.Context) (tobari.ManifestReport, error) {
	f.initCalls++
	f.report.Task = tobari.TaskRuntimeInit
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) BuildRuntime(context.Context) (tobari.ManifestReport, error) {
	f.buildCalls++
	f.report.Task = tobari.TaskRuntimeBuild
	f.report.Authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	return f.report, f.buildErr
}

func (f *contextCLI) BuildRuntimeWithProgress(
	_ context.Context, diagnostics io.Writer, progress tobari.RuntimeBuildProgressSink,
) (tobari.ManifestReport, error) {
	f.buildCalls++
	f.report.Task = tobari.TaskRuntimeBuild
	metadata := tobari.RuntimeBuildProgress{
		WorkspaceManifestName: "default", Dockerfile: "/config/contexts/default/runtime/Dockerfile",
		PreviousImage: tobari.OfficialRuntimeBase, CandidateImage: "tobari-context-default:0123456789ab",
		Selection: tobari.RuntimeBuildSelectionUnchanged,
	}
	emit := func(stage tobari.RuntimeBuildStage, status tobari.RuntimeBuildProgressStatus) {
		if progress == nil {
			return
		}
		metadata.Stage, metadata.Status = stage, status
		progress(metadata)
	}
	emit(tobari.RuntimeBuildStagePrepare, tobari.RuntimeBuildProgressStarted)
	emit(tobari.RuntimeBuildStagePrepare, tobari.RuntimeBuildProgressCompleted)
	emit(tobari.RuntimeBuildStageBuild, tobari.RuntimeBuildProgressStarted)
	if diagnostics != nil && f.buildLog != "" {
		_, _ = io.WriteString(diagnostics, f.buildLog)
	}
	if f.buildErr != nil {
		emit(tobari.RuntimeBuildStageBuild, tobari.RuntimeBuildProgressFailed)
		return tobari.ManifestReport{}, f.buildErr
	}
	emit(tobari.RuntimeBuildStageBuild, tobari.RuntimeBuildProgressCompleted)
	return f.report, nil
}

type fakeContextRuntime struct {
	list                    tobari.ManifestListResult
	listResults             []tobari.ManifestListResult
	report                  tobari.ManifestReport
	reports                 map[string]tobari.ManifestReport
	listCalls               int
	initCalls               int
	useCalls                int
	createCalls             int
	buildCalls              int
	buildLog                string
	buildErr                error
	configureCalls          int
	configureGitCalls       int
	configureBootstrapCalls int
	prepareBootstrapCalls   int
	showCalls               int
	showErr                 error
	deleteErr               error
	deleteCalls             int
	setRuntimeCalls         int
	lastShellChange         tobari.ManifestShellEnvironmentSetting
	lastShellChanges        []tobari.ManifestShellEnvironmentSetting
	lastGitChange           tobari.ManifestGitIdentitySetting
	lastShellContext        string
	lastGitContext          string
	lastRuntimeContext      string
	lastRuntimeSelection    string
	base                    tobari.ManifestCopySnapshot
	baseCalls               int
	lastComposition         tobari.ManifestCreateComposition
}

func (f *contextCLI) ManifestCopySnapshot(_ context.Context, name string) (tobari.ManifestCopySnapshot, error) {
	f.baseCalls++
	if f.base.Name == "" || f.base.Name != name {
		return tobari.ManifestCopySnapshot{}, tobari.ErrContextNotFound
	}
	return f.base.Clone(), nil
}

type contextSwitchingWizard struct {
	runtime  *contextCLI
	seenName string
}

func (w *contextSwitchingWizard) switchActive(current tobari.ManifestReport) {
	w.seenName = current.Name
	w.runtime.report = contextCLIReport(
		tobari.TaskManifestShow, "switched", true,
		tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
	)
}

func (w *contextSwitchingWizard) ConfigureShell(
	_ context.Context, current tobari.ManifestReport, _ io.Reader, _ io.Writer,
) ([]tobari.ManifestShellEnvironmentSetting, error) {
	w.switchActive(current)
	return []tobari.ManifestShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ManifestShellEnvironmentInherit,
	}}, nil
}

func (w *contextSwitchingWizard) ConfigureGit(
	_ context.Context, current tobari.ManifestReport, _ io.Reader, _ io.Writer,
) (tobari.ManifestGitIdentitySetting, error) {
	w.switchActive(current)
	return tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityInherit}, nil
}

func TestContextUseReportsReconciliationStatusAndParsesBeforeMutation(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "project-tools", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	fake.report.Cluster = tobari.ManifestClusterStatusReconciled
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "default", "set", "--name", "project-tools", "--format", "yaml"}); code != ExitUsage {
		t.Fatalf("invalid format code = %d, stderr = %q", code, stderr.String())
	}
	if fake.useCalls != 0 {
		t.Fatalf("SetDefaultManifest() calls after invalid format = %d, want 0", fake.useCalls)
	}
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "default", "set", "--name", "project-tools", "--format", "json"}); code != ExitOK {
		t.Fatalf("context use code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		Manifest tobari.ManifestReport `json:"workspace_manifest"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("context use JSON = %q, error = %v", stdout.String(), err)
	}
	if document.Manifest.Cluster != tobari.ManifestClusterStatusReconciled || fake.useCalls != 1 {
		t.Fatalf("context use document/calls = %+v/%d", document.Manifest, fake.useCalls)
	}
}

func TestContextUseDefaultUpdatedContinuesThroughOmittedDefault(t *testing.T) {
	t.Parallel()
	report := contextCLIReport(
		tobari.TaskManifestDefaultSet, "review", true,
		tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
	)
	report.Cluster = tobari.ManifestClusterStatusDefaultManifestUpdated
	output, err := renderContextReport(report, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Next: run `tobari` from a project directory to create or enter a Workspace using the new default Workspace Manifest.") ||
		strings.Contains(string(output), "--manifest review") {
		t.Fatalf("default-updated continuation does not use omitted Workspace Manifest selection: %q", output)
	}
	if routed := assertPublicNextArgvRoutes(t, []string{ProgramName}); routed.Path != "tobari" {
		t.Fatalf("default-updated continuation routes to %q, want root entry", routed.Path)
	}
}

func contextCLIReport(task, name string, active bool, image string, mode tobari.ManifestPolicyMode) tobari.ManifestReport {
	authentication := tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable}
	if task == tobari.TaskManifestShow {
		authentication = tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerReady, Providers: []tobari.ManifestAuthProvider{}}
	}
	return tobari.ManifestReport{
		Task: task, ManifestState: tobari.ManifestObservationPersisted, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name, Default: active, AgentProfile: tobari.DefaultProfile,
		Desired: cliTestManifestRevision("f"),
		Image:   image, PolicyMode: mode, SourceAccess: tobari.ManifestSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(), MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		Cluster:          tobari.ManifestClusterStatusNotApplicable,
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Runtime:          standardContextRuntimeReport(image),
		Authentication:   authentication,
		Stores: tobari.ManifestStorePaths{
			PolicyDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
		},
	}
}

func cliTestManifestRevision(digit string) tobari.WorkspaceManifestRevision {
	digest := "sha256:" + strings.Repeat(digit, 64)
	return tobari.WorkspaceManifestRevision{Generation: 1, Revision: digest, BoundaryRevision: digest, ClusterProjectionRevision: digest, EntryRevision: digest, SessionDefaultsRevision: digest, CreationDefaultsRevision: digest}
}

func contextSummaryFromReport(report tobari.ManifestReport) tobari.ManifestSummary {
	selection, _ := report.Runtime.Selection()
	return tobari.ManifestSummary{
		ID: report.ID, Name: report.Name, ManifestState: report.ManifestState, Default: report.Default,
		Desired:      report.Desired,
		AgentProfile: report.AgentProfile, Image: report.Image, PolicyMode: report.PolicyMode,
		SourceAccess: report.SourceAccess, PolicyRevision: report.PolicyRevision,
		NativeReadiness: report.NativeReadiness, MethodPolicy: report.MethodPolicy.Clone(),
		RuntimeStatus: report.Runtime.Status, RuntimeSelection: selection, Bootstrap: report.Bootstrap,
	}
}

func standardContextRuntimeReport(image string) tobari.ManifestRuntimeReport {
	return tobari.ManifestRuntimeReport{
		Kind: tobari.ManifestRuntimeKindOfficial, Status: tobari.ManifestRuntimeStatusOfficial,
		Image: image, RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
		Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1,
	}
}

func TestContextShellConfigurePreservesSourceAndExplicitEmptyValue(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	if code := command.RunContext(context.Background(), []string{
		"config", "shell", "--variable", "PS1", "--source", "literal", "--value=", "--format", "json",
	}); code != ExitOK {
		t.Fatalf("config shell code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		SchemaVersion int                   `json:"schema_version"`
		Manifest      tobari.ManifestReport `json:"workspace_manifest"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("config shell JSON = %q, error = %v", stdout.String(), err)
	}
	if document.SchemaVersion != 2 || fake.configureCalls != 1 || fake.lastShellChange.Value == nil ||
		*fake.lastShellChange.Value != "" || document.Manifest.Task != tobari.TaskConfigShell {
		t.Fatalf("configure document/call = %+v / %d %+v", document, fake.configureCalls, fake.lastShellChange)
	}
	if fake.showCalls != 0 {
		t.Fatalf("direct config shell unexpectedly inspected the Workspace Manifest %d times", fake.showCalls)
	}

	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{
		"config", "shell", "--variable", "PATH", "--source", "inherit",
	}); code != ExitUsage {
		t.Fatalf("unlisted variable code = %d, stderr = %q", code, stderr.String())
	}
	if fake.configureCalls != 1 {
		t.Fatalf("unlisted variable reached mutation, calls = %d", fake.configureCalls)
	}
}

func TestConfigNamespacePublishesCompositionCommands(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{"config shell", "config git", "config bootstrap aws", "config bootstrap kubernetes eks"} {
		spec, found := catalog.Lookup(path)
		if !found || spec.Role != RoleAct || spec.Effect.String() != "write" ||
			spec.Agent.FixedTarget == nil || spec.Agent.FixedTarget.Scope != FixedTargetScopeToolLocal {
			t.Fatalf("%s contract = %+v", path, spec)
		}
	}
	selected, exact := catalog.Select("config")
	if exact || len(selected) != 4 {
		t.Fatalf("config namespace selection exact=%t commands=%+v", exact, selected)
	}
}

func TestContextCatalogKeepsBoundaryCreationOnlyAndDeclaresMutableComponents(t *testing.T) {
	catalog := DefaultCatalog()
	creationOnlyBoundaryInputs := map[string]struct{}{
		"--mode": {}, "--source-access": {}, "--native-readiness": {},
	}
	for _, spec := range catalog.Commands() {
		if spec.Effect.String() != "write" {
			continue
		}
		for _, input := range spec.Agent.Inputs {
			if _, boundary := creationOnlyBoundaryInputs[input.Name]; boundary {
				t.Fatalf("existing-Workspace Manifest write %q accepts creation-time Boundary input %q", spec.Path, input.Name)
			}
		}
	}

	wantTargets := map[string]string{
		"manifest runtime set":            tobari.ManifestRuntimeBindingTargetKind,
		"config shell":                    tobari.ManifestShellTargetKind,
		"config git":                      tobari.ManifestGitIdentityTargetKind,
		"config bootstrap aws":            tobari.ManifestBootstrapTargetKind,
		"config bootstrap kubernetes eks": tobari.ManifestBootstrapTargetKind,
	}
	for path, targetKind := range wantTargets {
		spec, found := catalog.Lookup(path)
		if !found || spec.Effect.String() != "write" || spec.Agent.Mutation == nil ||
			spec.Agent.Mutation.TargetKind != targetKind || spec.Agent.FixedTarget == nil ||
			spec.Agent.FixedTarget.Kind != targetKind {
			t.Fatalf("mutable Workspace Manifest component %q contract = %+v", path, spec)
		}
	}
}

func TestConfigBootstrapAWSDirectIsFutureWorkspaceOnlyAndConflictsFailBeforeMutation(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"config", "bootstrap", "aws", "--profile", "engineering", "--format", "json"}); code != ExitOK {
		t.Fatalf("config bootstrap code = %d, stderr = %q", code, stderr.String())
	}
	if fake.configureBootstrapCalls != 1 || fake.showCalls != 0 || !strings.Contains(stdout.String(), `"state":"configured"`) {
		t.Fatalf("bootstrap calls=%d show=%d output=%q", fake.configureBootstrapCalls, fake.showCalls, stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"config", "bootstrap", "aws", "--profile", "engineering", "--remove"}); code != ExitUsage {
		t.Fatalf("conflicting bootstrap action code = %d, stderr = %q", code, stderr.String())
	}
	if fake.configureBootstrapCalls != 1 {
		t.Fatalf("conflicting action reached mutation: %d", fake.configureBootstrapCalls)
	}
}

func TestConfigBootstrapEKSDirectUsesClosedContextTarget(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"config", "bootstrap", "kubernetes", "eks", "--kube-context", "engineering", "--format", "json"}); code != ExitOK {
		t.Fatalf("EKS bootstrap code = %d, stderr = %q", code, stderr.String())
	}
	if fake.configureBootstrapCalls != 1 || !strings.Contains(stdout.String(), `"task":"config.bootstrap.kubernetes.eks"`) || !strings.Contains(stdout.String(), `"kubernetes_eks_context":"engineering"`) {
		t.Fatalf("EKS bootstrap calls=%d output=%s", fake.configureBootstrapCalls, stdout.String())
	}
}

func TestContextCreateDirectCanSnapshotOneAWSProfile(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--name", "coding", "--runtime", "standard", "--mode", "guided", "--source-access", "read-write", "--native-readiness", "enabled", "--bootstrap-aws-profile", "engineering", "--format", "json"}); code != ExitOK {
		t.Fatalf("context create with bootstrap code = %d, stderr = %q", code, stderr.String())
	}
	if fake.prepareBootstrapCalls != 1 || fake.createCalls != 1 || !strings.Contains(stdout.String(), `"aws_profile":"engineering"`) {
		t.Fatalf("prepare/create=%d/%d output=%q", fake.prepareBootstrapCalls, fake.createCalls, stdout.String())
	}
}

func TestConfigGitDirectPreservesLiteralPairWithoutWizardRead(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	code := command.RunContext(context.Background(), []string{
		"config", "git", "--manifest", "work", "--source", "literal",
		"--name", "Tobari User", "--email", "tobari@example.com", "--format", "json",
	})
	if code != ExitOK {
		t.Fatalf("config git code = %d, stderr = %q", code, stderr.String())
	}
	if fake.configureGitCalls != 1 || fake.showCalls != 0 || fake.lastGitChange.Name == nil ||
		fake.lastGitChange.Email == nil || *fake.lastGitChange.Name != "Tobari User" ||
		*fake.lastGitChange.Email != "tobari@example.com" {
		t.Fatalf("direct Git call/show/change = %d/%d/%+v", fake.configureGitCalls, fake.showCalls, fake.lastGitChange)
	}
	var document contextReportDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("config git JSON = %q, error = %v", stdout.String(), err)
	}
	if document.SchemaVersion != 2 || document.Manifest.Task != tobari.TaskConfigGit ||
		document.Manifest.GitIdentity.Source != tobari.ManifestGitIdentityLiteral {
		t.Fatalf("config git document = %+v", document)
	}
}

func TestConfigDirectSourceOnlyModesNeverPrompt(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantShell tobari.ManifestShellEnvironmentSource
		wantGit   tobari.ManifestGitIdentitySource
	}{
		{name: "shell default", args: []string{"config", "shell", "--variable", "PS1", "--source", "default"}, wantShell: tobari.ManifestShellEnvironmentDefault},
		{name: "shell inherit", args: []string{"config", "shell", "--variable", "PS1", "--source", "inherit"}, wantShell: tobari.ManifestShellEnvironmentInherit},
		{name: "Git default", args: []string{"config", "git", "--source", "default"}, wantGit: tobari.ManifestGitIdentityDefault},
		{name: "Git inherit", args: []string{"config", "git", "--source", "inherit"}, wantGit: tobari.ManifestGitIdentityInherit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitOK {
				t.Fatalf("direct source-only code = %d, stderr = %q", code, stderr.String())
			}
			if fake.showCalls != 0 {
				t.Fatalf("direct source-only mode inspected Workspace Manifest %d times", fake.showCalls)
			}
			if test.wantShell != "" && (fake.configureCalls != 1 || fake.lastShellChange.Source != test.wantShell || fake.lastShellChange.Value != nil) {
				t.Fatalf("shell direct call/change = %d/%+v", fake.configureCalls, fake.lastShellChange)
			}
			if test.wantGit != "" && (fake.configureGitCalls != 1 || fake.lastGitChange.Source != test.wantGit || fake.lastGitChange.Name != nil || fake.lastGitChange.Email != nil) {
				t.Fatalf("Git direct call/change = %d/%+v", fake.configureGitCalls, fake.lastGitChange)
			}
		})
	}
}

func TestConfigPartialInputsFailBeforeInspectionOrMutation(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "shell variable only", args: []string{"config", "shell", "--variable", "PS1"}},
		{name: "git pair without source", args: []string{"config", "git", "--name", "Tobari User", "--email", "tobari@example.com"}},
		{name: "literal source without pair", args: []string{"config", "git", "--source", "literal"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitUsage {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			if fake.showCalls != 0 || fake.configureCalls != 0 || fake.configureGitCalls != 0 || stdout.Len() != 0 {
				t.Fatalf("show/shell/Git/stdout = %d/%d/%d/%q", fake.showCalls, fake.configureCalls, fake.configureGitCalls, stdout.String())
			}
		})
	}
}

func TestConfigRejectsExplicitEmptyContextBeforeInspectionOrMutation(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode string
	}{
		{
			name: "shell command-local equals", wantCode: "invalid_manifest_name",
			args: []string{"config", "shell", "--manifest=", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "shell command-local separated", wantCode: "invalid_manifest_name",
			args: []string{"config", "shell", "--manifest", "", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "Git command-local equals", wantCode: "invalid_manifest_name",
			args: []string{"config", "git", "--manifest=", "--source", "default"},
		},
		{
			name: "Git command-local separated", wantCode: "invalid_manifest_name",
			args: []string{"config", "git", "--manifest", "", "--source", "default"},
		},
		{
			name: "global equals", wantCode: "invalid_root_options",
			args: []string{"--manifest=", "config", "shell", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "global separated", wantCode: "invalid_root_options",
			args: []string{"--manifest", "", "config", "git", "--source", "default"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{
				report: contextCLIReport(
					tobari.TaskManifestShow, "default", true,
					tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
				),
			}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitUsage {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			if fake.showCalls != 0 || fake.configureCalls != 0 || fake.configureGitCalls != 0 || stdout.Len() != 0 ||
				!humanOutputHasRow(stderr.String(), "Code", test.wantCode) {
				t.Fatalf(
					"show/shell/Git/stdout/stderr = %d/%d/%d/%q/%q",
					fake.showCalls, fake.configureCalls, fake.configureGitCalls, stdout.String(), stderr.String(),
				)
			}
		})
	}
}

func TestConfigOmittedSettingsRequireTextTerminalWithoutInspectingContext(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "shell non terminal", args: []string{"config", "shell", "--manifest", "work"}},
		{name: "Git non terminal", args: []string{"config", "git"}},
		{name: "shell JSON", args: []string{"config", "shell", "--format", "json"}},
		{name: "Git JSON", args: []string{"config", "git", "--format", "json"}},
		{name: "Git JSON errors", args: []string{"--error-format=json", "config", "git"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: strings.Contains(test.name, "JSON")})
			if code := command.RunContext(context.Background(), test.args); code != ExitUsage {
				t.Fatalf("code = %d, stderr = %q", code, stderr.String())
			}
			if fake.showCalls != 0 || fake.configureCalls != 0 || fake.configureGitCalls != 0 || stdout.Len() != 0 ||
				!strings.Contains(stderr.String(), "configuration_wizard_unavailable") {
				t.Fatalf("show/shell/Git/stdout/stderr = %d/%d/%d/%q/%q", fake.showCalls, fake.configureCalls, fake.configureGitCalls, stdout.String(), stderr.String())
			}
			if strings.Contains(test.name, "JSON errors") && !json.Valid(stderr.Bytes()) {
				t.Fatalf("JSON-error wizard refusal stderr = %q", stderr.String())
			}
		})
	}
}

func TestConfigGitLineWizardAppliesOnStderrAndEmitsReportOnStdout(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("l\nTobari User\ntobari@example.com\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "git", "--manifest", "work"}); code != ExitOK {
		t.Fatalf("wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureGitCalls != 1 || fake.lastGitChange.Source != tobari.ManifestGitIdentityLiteral {
		t.Fatalf("wizard show/config/change = %d/%d/%+v", fake.showCalls, fake.configureGitCalls, fake.lastGitChange)
	}
	for _, want := range []string{"Tobari · Git identity", "Workspace Manifest: work", "Only user.name and user.email are projected.", "Apply this change?"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("wizard stderr = %q, missing %q", stderr.String(), want)
		}
	}
	for _, prompt := range []string{"Tobari · Git identity", "Only user.name and user.email are projected.", "Apply this change?"} {
		if strings.Contains(stdout.String(), prompt) {
			t.Fatalf("wizard prompt %q leaked to stdout: %q", prompt, stdout.String())
		}
	}
	if !strings.Contains(stdout.String(), "Workspace Manifest: work") || !strings.Contains(stdout.String(), "Git identity: literal") {
		t.Fatalf("confirmed report stdout = %q", stdout.String())
	}
}

func TestConfigShellLineWizardStagesMultipleSettingsInOneMutation(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\nh\n2\nd\np\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "shell", "--manifest", "work"}); code != ExitOK {
		t.Fatalf("wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureCalls != 1 || len(fake.lastShellChanges) != 2 {
		t.Fatalf("wizard show/config/changes = %d/%d/%+v", fake.showCalls, fake.configureCalls, fake.lastShellChanges)
	}
	if fake.lastShellChanges[0].Variable != "COLORTERM" || fake.lastShellChanges[0].Source != tobari.ManifestShellEnvironmentInherit ||
		fake.lastShellChanges[1].Variable != "NO_COLOR" || fake.lastShellChanges[1].Source != tobari.ManifestShellEnvironmentDefault {
		t.Fatalf("wizard changes = %+v", fake.lastShellChanges)
	}
	for _, want := range []string{"Tobari · Shell configuration", "COLORTERM", "NO_COLOR", "Apply 2 changes"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("wizard stderr = %q, missing %q", stderr.String(), want)
		}
	}
	if !strings.Contains(stdout.String(), "Workspace Manifest: work") {
		t.Fatalf("confirmed report stdout = %q", stdout.String())
	}
}

func TestConfigWizardBindsApplyToTheContextShownBeforeActiveSelectionChanges(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "shell", args: []string{"config", "shell"}},
		{name: "Git", args: []string{"config", "git"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{
				report: contextCLIReport(
					tobari.TaskManifestShow, "shown", true,
					tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
				),
			}
			wizard := &contextSwitchingWizard{runtime: fake}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
			command.config = wizard
			command.noColor = true

			if code := command.RunContext(context.Background(), test.args); code != ExitOK {
				t.Fatalf("wizard code = %d, stderr = %q", code, stderr.String())
			}
			if wizard.seenName != "shown" || fake.showCalls != 1 {
				t.Fatalf("wizard saw Workspace Manifest %q after %d reads", wizard.seenName, fake.showCalls)
			}
			configuredContext, configuredCalls := fake.lastShellContext, fake.configureCalls
			if test.name == "Git" {
				configuredContext, configuredCalls = fake.lastGitContext, fake.configureGitCalls
			}
			if configuredContext != "shown" || configuredCalls != 1 || configuredContext == "switched" {
				t.Fatalf("configured Workspace Manifest/calls = %q/%d, want shown/1", configuredContext, configuredCalls)
			}
		})
	}
}

func TestConfigWizardCancellationLeavesConfigurationUnchanged(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("q\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "git", "--manifest", "work"}); code != ExitCanceled {
		t.Fatalf("canceled wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureCalls != 0 || fake.configureGitCalls != 0 || stdout.Len() != 0 {
		t.Fatalf(
			"canceled wizard show/shell/Git/stdout = %d/%d/%d/%q",
			fake.showCalls, fake.configureCalls, fake.configureGitCalls, stdout.String(),
		)
	}
	if !humanOutputHasRow(stderr.String(), "Code", "operation_canceled") {
		t.Fatalf("canceled wizard stderr = %q", stderr.String())
	}
}

func TestConfigWizardPreservesCancellationWhileLoadingCurrentState(t *testing.T) {
	fake := &contextCLI{
		report:  contextCLIReport(tobari.TaskManifestShow, "work", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided),
		showErr: context.Canceled,
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "shell", "--manifest", "work"}); code != ExitCanceled {
		t.Fatalf("pre-wizard cancellation code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureCalls != 0 || fake.configureGitCalls != 0 || stdout.Len() != 0 {
		t.Fatalf(
			"pre-wizard cancellation show/shell/Git/stdout = %d/%d/%d/%q",
			fake.showCalls, fake.configureCalls, fake.configureGitCalls, stdout.String(),
		)
	}
	if !humanOutputHasRow(stderr.String(), "Code", "operation_canceled") {
		t.Fatalf("pre-wizard cancellation stderr = %q", stderr.String())
	}
}

func TestManifestReportJSONSchemaTwoDeclaresExactKeys(t *testing.T) {
	report := contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)
	encoded, err := renderContextReport(report, successFormatJSON, false)
	if err != nil {
		t.Fatalf("renderWorkspace ManifestReport() error = %v", err)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &outer); err != nil {
		t.Fatalf("JSON = %q, error = %v", encoded, err)
	}
	var version int
	if err := json.Unmarshal(outer["schema_version"], &version); err != nil || version != 2 {
		t.Fatalf("schema version = %d, error = %v", version, err)
	}
	var contextFields map[string]json.RawMessage
	if err := json.Unmarshal(outer["workspace_manifest"], &contextFields); err != nil {
		t.Fatalf("Workspace Manifest envelope = %q, error = %v", outer["workspace_manifest"], err)
	}
	want := []string{"agent_profile", "authentication", "bootstrap", "cluster", "default", "desired", "git_identity", "image", "method_policy", "name", "native_readiness", "policy_mode", "policy_revision", "runtime", "shell_environment", "source_access", "stores", "task", "workspace_manifest_id", "workspace_manifest_state"}
	got := make([]string, 0, len(contextFields))
	for name := range contextFields {
		got = append(got, name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Workspace Manifest JSON keys = %v, want %v", got, want)
	}
	spec, _ := DefaultCatalog().Lookup("config git")
	declared := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		declared = append(declared, field.Name)
	}
	sort.Strings(declared)
	if !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared Workspace Manifest output fields = %v, want %v", declared, want)
	}
}

func TestAbsentManifestCannotRenderAsAnAuthorityReport(t *testing.T) {
	t.Parallel()
	syntheticReport := tobari.ManifestReport{
		Task: tobari.TaskManifestShow, ManifestState: tobari.ManifestObservationAbsent,
		Name: tobari.DefaultManifestName, Default: true, AgentProfile: tobari.DefaultProfile,
		Image: tobari.OfficialRuntimeBase, PolicyMode: tobari.ManifestPolicyModeGuided,
		SourceAccess:     tobari.ManifestSourceAccessReadWrite,
		MethodPolicy:     tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Runtime:          standardContextRuntimeReport(tobari.OfficialRuntimeBase),
		Cluster:          tobari.ManifestClusterStatusNotApplicable,
		Authentication: tobari.ManifestAuthentication{
			BrokerState: tobari.ManifestAuthBrokerUnavailable, Providers: []tobari.ManifestAuthProvider{},
		},
	}
	if _, err := renderContextReport(syntheticReport, successFormatJSON, false); err == nil {
		t.Fatal("absent Manifest rendered as authority")
	}
}

func TestContextCreateRendersRequiresReconcileAndExecutableRootContinuation(t *testing.T) {
	t.Parallel()
	report := contextCLIReport(
		tobari.TaskManifestCreate, "review", false,
		tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
	)
	report.Cluster = tobari.ManifestClusterStatusRequiresReconcile
	output, err := renderContextReport(report, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Workspace Manifest review created") ||
		!strings.Contains(string(output), "Cluster        requires_reconcile") ||
		!strings.Contains(string(output), "Next           tobari --manifest review") {
		t.Fatalf("Workspace Manifest create hides required cluster reconciliation: %q", output)
	}
	if routed := assertPublicNextArgvRoutes(t, contextCreateNextArgv(report)); routed.Path != "tobari" {
		t.Fatalf("Workspace Manifest create recovery routes to %q", routed.Path)
	}
}

func TestContextCreateRendersAbsentClusterAndExecutableRootContinuation(t *testing.T) {
	t.Parallel()
	report := contextCLIReport(
		tobari.TaskManifestCreate, tobari.DefaultManifestName, true,
		tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided,
	)
	report.Cluster = tobari.ManifestClusterStatusNotApplicable
	output, err := renderContextReport(report, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Workspace Manifest default created") ||
		!strings.Contains(string(output), "Cluster        not_applicable") ||
		!strings.Contains(string(output), "Next           tobari —") ||
		strings.Contains(string(output), "Next           tobari --manifest") {
		t.Fatalf("Workspace Manifest create hides absent-cluster recovery: %q", output)
	}
	if routed := assertPublicNextArgvRoutes(t, contextCreateNextArgv(report)); routed.Path != "tobari" {
		t.Fatalf("Workspace Manifest create recovery routes to %q", routed.Path)
	}
}

func TestContextExistsCatalogRecoveryRoutesToListContainingNonActiveDuplicate(t *testing.T) {
	t.Parallel()
	create, found := DefaultCatalog().Lookup("manifest create")
	if !found {
		t.Fatal("context create is absent from the catalog")
	}
	var duplicateError *CommandError
	for index := range create.Agent.Errors {
		if create.Agent.Errors[index].Code == "manifest_exists" {
			duplicateError = &create.Agent.Errors[index]
			break
		}
	}
	if duplicateError == nil || duplicateError.Kind != fault.KindRejected || duplicateError.Retryable ||
		len(duplicateError.NextActions) != 1 || duplicateError.NextActions[0].Command != "manifest list" ||
		duplicateError.NextActions[0].Reason != "List existing Workspace Manifests before choosing another name." {
		t.Fatalf("manifest_exists catalog error = %+v", duplicateError)
	}
	if routed := assertPublicNextArgvRoutes(t, []string{ProgramName, "manifest", "list"}); routed.Path != "manifest list" {
		t.Fatalf("manifest_exists recovery routes to %q", routed.Path)
	}

	result := tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: "018bcfe5-687b-7000-8000-000000000099", DefaultManifest: "default",
		Items: []tobari.ManifestSummary{
			{
				ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default",
				ManifestState: tobari.ManifestObservationPersisted, Default: true,
				Desired:      cliTestManifestRevision("9"),
				AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
				PolicyMode:     tobari.ManifestPolicyModeGuided,
				SourceAccess:   tobari.ManifestSourceAccessReadWrite,
				PolicyRevision: tobari.DefaultContextPolicyRevision(),
				MethodPolicy:   tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
				RuntimeStatus:  tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: tobari.StandardRuntimeName + "@1",
			},
			{
				ID: "018bcfe5-687b-7000-8000-000000000100", Name: "review",
				ManifestState: tobari.ManifestObservationPersisted, Default: false,
				Desired:      cliTestManifestRevision("8"),
				AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
				PolicyMode:     tobari.ManifestPolicyModeGuided,
				SourceAccess:   tobari.ManifestSourceAccessReadOnly,
				PolicyRevision: tobari.DefaultContextPolicyRevision(),
				MethodPolicy:   tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
				RuntimeStatus:  tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: tobari.StandardRuntimeName + "@1",
			},
		},
	}
	output, err := renderContextList(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "review") || strings.Contains(string(output), "* review") {
		t.Fatalf("context list did not expose the non-active duplicate truthfully: %q", output)
	}
}

func TestContextListResultFirstCardsKeepSchemaOneJSONProjectionUnchanged(t *testing.T) {
	t.Parallel()
	result := tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: "018bcfe5-687b-7000-8000-000000000099", DefaultManifest: "default",
		Items: []tobari.ManifestSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default",
			ManifestState: tobari.ManifestObservationPersisted, Default: true,
			Desired:      cliTestManifestRevision("d"),
			AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
			PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite,
			PolicyRevision:  tobari.DefaultContextPolicyRevision(),
			NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			MethodPolicy: tobari.ManifestMethodPolicy{
				Default:   tobari.ManifestMethodExactReview,
				Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}},
			},
			RuntimeStatus: tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: tobari.StandardRuntimeName + "@1",
		}},
	}
	textOutput, err := renderContextList(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(textOutput), "\t") || !strings.Contains(string(textOutput), "Access GET allowed") ||
		!strings.Contains(string(textOutput), "Runtime    standard@1") {
		t.Fatalf("Workspace Manifest cards hid effective Access or exact Runtime: %q", textOutput)
	}
	jsonOutput, err := renderContextList(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		SchemaVersion      int `json:"schema_version"`
		WorkspaceManifests struct {
			DefaultManifest string `json:"default_manifest"`
			Items           []struct {
				Name         string                      `json:"name"`
				SourceAccess tobari.ManifestSourceAccess `json:"source_access"`
				MethodPolicy tobari.ManifestMethodPolicy `json:"method_policy"`
			} `json:"items"`
		} `json:"workspace_manifests"`
	}
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 2 || document.WorkspaceManifests.DefaultManifest != "default" || len(document.WorkspaceManifests.Items) != 1 ||
		document.WorkspaceManifests.Items[0].Name != "default" || document.WorkspaceManifests.Items[0].SourceAccess != tobari.ManifestSourceAccessReadWrite ||
		len(document.WorkspaceManifests.Items[0].MethodPolicy.Overrides) != 1 {
		t.Fatalf("Workspace Manifest list JSON changed with text layout: %+v", document)
	}
}

func TestContextListMarksRuntimeActionWithoutInventingAReadyRevision(t *testing.T) {
	t.Parallel()
	result := tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: "018bcfe5-687b-7000-8000-000000000099", DefaultManifest: "default",
		Items: []tobari.ManifestSummary{{
			ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default",
			ManifestState: tobari.ManifestObservationPersisted, Default: true,
			Desired:      cliTestManifestRevision("c"),
			AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
			PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite,
			PolicyRevision: tobari.DefaultContextPolicyRevision(),
			MethodPolicy:   tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
			RuntimeStatus:  tobari.ManifestRuntimeStatusPendingBuild, RuntimeSelection: "context-owned Dockerfile",
		}},
	}
	output, err := renderContextList(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "* default !") || !strings.Contains(string(output), "Runtime    context-owned Dockerfile") ||
		strings.Contains(string(output), "standard@1") {
		t.Fatalf("Workspace Manifest list action marker or pending Runtime is untruthful: %q", output)
	}
}

func TestSyntheticManifestAuthStatusRecoveryOmitsAbsentSelector(t *testing.T) {
	if len(authCommandSpecs()) == 0 {
		t.Skip("Broker recovery exists only in the experimental profile")
	}
	t.Parallel()
	report := tobari.ManifestReport{ManifestState: tobari.ManifestObservationAbsent}
	if routed := assertPublicNextArgvRoutes(t, contextAuthStatusNextArgv(report)); routed.Path != "auth status" {
		t.Fatalf("synthetic Workspace Manifest recovery routes to %q", routed.Path)
	}
	if got := contextAuthStatusNextArgv(report); !reflect.DeepEqual(got, []string{"tobari", "auth", "status"}) {
		t.Fatalf("synthetic Workspace Manifest recovery claims an absent selector: %v", got)
	}
}

func TestContextShowReportsBrokerFirstRouting(t *testing.T) {
	if len(authCommandSpecs()) == 0 {
		t.Skip("Broker routing exists only in the experimental profile")
	}
	report := contextCLIReport(
		tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase,
		tobari.ManifestPolicyModeGuided,
	)
	output, err := renderContextReport(report, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Manifest struct {
			Authentication struct {
				DeclaredBindings   string `json:"declared_bindings"`
				UndeclaredBindings string `json:"undeclared_bindings"`
			} `json:"authentication"`
		} `json:"workspace_manifest"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.Manifest.Authentication.DeclaredBindings != "broker_required" ||
		document.Manifest.Authentication.UndeclaredBindings != "workspace_owned_compatibility" {
		t.Fatalf("authentication routes = %+v", document.Manifest.Authentication)
	}
}

func TestContextCommandsRenderActiveContextAndRuntimeImage(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	fake.list = tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: "018bcfe5-687b-7000-8000-000000000099", DefaultManifest: "default",
		Items: []tobari.ManifestSummary{{ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default", ManifestState: tobari.ManifestObservationPersisted, Default: true, Desired: cliTestManifestRevision("b"), AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase, PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite, PolicyRevision: tobari.DefaultContextPolicyRevision(), MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}, RuntimeStatus: tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: tobari.StandardRuntimeName + "@1"}},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "list"}); code != ExitOK {
		t.Fatalf("context list code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Workspace Manifests") || !strings.Contains(stdout.String(), "* default") ||
		!strings.Contains(stdout.String(), "Access     Read-write · routine clients ready · other exact review · private denied") ||
		!strings.Contains(stdout.String(), "Runtime    standard@1") ||
		strings.Contains(stdout.String(), "Image") || strings.Contains(stdout.String(), "Profile") {
		t.Fatalf("context list output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "show"}); code != ExitOK {
		t.Fatalf("context show code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Selected       standard@1") ||
		!strings.Contains(stdout.String(), "Project files  Read-write · changes affect this project directly") ||
		!strings.Contains(stdout.String(), "Details        tobari manifest show --details") ||
		!strings.Contains(stdout.String(), "Next           tobari") {
		t.Fatalf("context show output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--name", "project-tools", "--runtime", "standard", "--mode", "advanced", "--source-access", "read-write", "--native-readiness", "enabled", "--format", "json"}); code != ExitOK {
		t.Fatalf("context create code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		Manifest tobari.ManifestReport `json:"workspace_manifest"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("context create JSON = %q, error = %v", stdout.String(), err)
	}
	if document.Manifest.Name != "project-tools" || document.Manifest.Image != tobari.BuiltinImageSelector ||
		document.Manifest.PolicyMode != tobari.ManifestPolicyModeAdvanced ||
		document.Manifest.SourceAccess != tobari.ManifestSourceAccessReadWrite {
		t.Fatalf("context create document = %+v", document.Manifest)
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{
		"manifest", "create", "--name", "review", "--runtime", "standard", "--mode", "guided",
		"--source-access", "read-only", "--native-readiness", "enabled", "--format", "json",
	}); code != ExitOK {
		t.Fatalf("read-only context create code = %d, stderr = %q", code, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil ||
		document.Manifest.SourceAccess != tobari.ManifestSourceAccessReadOnly {
		t.Fatalf("read-only context create document = %+v, error = %v", document.Manifest, err)
	}
}

func TestContextCreateIncompleteNonInteractiveInputFailsAndCompleteDirectInputDoesNotPrompt(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "create"}); code != ExitUsage {
		t.Fatalf("non-interactive wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 0 || !strings.Contains(stderr.String(), "manifest_create_wizard_unavailable") {
		t.Fatalf("non-interactive wizard mutated or hid recovery: calls=%d stderr=%q", fake.createCalls, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--name", "partial"}); code != ExitUsage {
		t.Fatalf("partial direct create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 0 || !strings.Contains(stderr.String(), "manifest_create_wizard_unavailable") {
		t.Fatalf("partial direct create mutated or hid recovery: calls=%d stderr=%q", fake.createCalls, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{
		"manifest", "create", "--name", "direct", "--runtime", "standard", "--mode", "guided",
		"--source-access", "read-write", "--native-readiness", "enabled",
	}); code != ExitOK {
		t.Fatalf("direct create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 1 || !strings.Contains(stdout.String(), "Workspace Manifest direct created") {
		t.Fatalf("direct create calls/output = %d/%q", fake.createCalls, stdout.String())
	}
}

func testContextCreateBase() tobari.ManifestCopySnapshot {
	return tobari.ManifestCopySnapshot{
		ID: "018bcfe5-687b-7000-8000-000000000120", Name: "engineering",
		Revision: "sha256:" + strings.Repeat("a", 64), PolicyMode: tobari.ManifestPolicyModeAdvanced,
		Desired:      cliTestManifestRevision("a"),
		SourceAccess: tobari.ManifestSourceAccessReadOnly, NativeReadiness: tobari.ManifestNativeReadinessDisabled,
		MethodPolicy:     tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodDeny, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		RuntimeSelection: "standard@1", RuntimeBinding: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("0", 64), Ordinal: 1, Image: tobari.OfficialRuntimeBase}, ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(), GitIdentity: tobari.DefaultContextGitIdentityReport(),
	}
}

func testContextCreateBaseList(base tobari.ManifestCopySnapshot) tobari.ManifestListResult {
	return tobari.ManifestListResult{
		Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
		DefaultManifestID: base.ID, DefaultManifest: base.Name,
		Items: []tobari.ManifestSummary{{
			ID: base.ID, Name: base.Name, ManifestState: tobari.ManifestObservationPersisted, Default: true,
			Desired:      base.Desired,
			AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
			PolicyMode: base.PolicyMode, SourceAccess: base.SourceAccess, PolicyRevision: tobari.DefaultContextPolicyRevision(),
			NativeReadiness: base.NativeReadiness, MethodPolicy: base.MethodPolicy.Clone(),
			RuntimeStatus: tobari.ManifestRuntimeStatusOfficial, RuntimeSelection: base.RuntimeSelection,
			Bootstrap: tobari.ManifestBootstrapReportFrom(nil),
		}},
	}
}

func TestContextCreateExplicitBaseIsDirectAndProducesStandaloneContext(t *testing.T) {
	t.Parallel()
	base := testContextCreateBase()
	fake := &contextCLI{base: base}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--copy-from", base.Name, "--name", "standalone", "--format", "json"}); code != ExitOK {
		t.Fatalf("explicit Base create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.baseCalls != 1 || fake.createCalls != 1 || fake.listCalls != 0 || fake.lastComposition.CopyFrom == nil ||
		fake.lastComposition.CopyFrom.Revision != base.Revision || fake.report.ID == base.ID ||
		fake.report.PolicyMode != base.PolicyMode || fake.report.SourceAccess != base.SourceAccess {
		t.Fatalf("explicit Base create calls/composition/report = base:%d list:%d create:%d %+v %+v", fake.baseCalls, fake.listCalls, fake.createCalls, fake.lastComposition, fake.report)
	}
	for _, forbidden := range []string{`"base_context"`, `"parent_context"`, `"inherits_from"`} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("standalone Workspace Manifest output persisted inferred lineage %s: %s", forbidden, stdout.String())
		}
	}
}

func TestContextCreateMissingExplicitBaseFailsBeforeMutation(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--copy-from", "missing", "--name", "standalone"}); code != ExitNotFound {
		t.Fatalf("missing Base create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.baseCalls != 1 || fake.createCalls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "manifest_copy_source_not_found") {
		t.Fatalf("missing Base mutation/output = base:%d create:%d stdout:%q stderr:%q", fake.baseCalls, fake.createCalls, stdout.String(), stderr.String())
	}
}

func TestContextCreateExplicitBaseAppliesSuppliedDraftOverrides(t *testing.T) {
	t.Parallel()
	base := testContextCreateBase()
	fake := &contextCLI{base: base}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{
		"manifest", "create", "--copy-from", base.Name, "--name", "overridden",
		"--mode", "guided", "--source-access", "read-write", "--native-readiness", "enabled",
	}); code != ExitOK {
		t.Fatalf("overridden Base create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.report.PolicyMode != tobari.ManifestPolicyModeGuided || fake.report.SourceAccess != tobari.ManifestSourceAccessReadWrite ||
		fake.lastComposition.NativeReadiness != tobari.ManifestNativeReadinessEnabled || fake.lastComposition.CopyFrom == nil ||
		fake.lastComposition.MethodPolicy == nil || fake.lastComposition.MethodPolicy.Default != base.MethodPolicy.Default {
		t.Fatalf("Base overrides lost reviewed values: report=%+v composition=%+v", fake.report, fake.lastComposition)
	}
}

func TestContextCreateOmittedBaseChoosesCurrentBeforeNameAndShowsInitializer(t *testing.T) {
	base := testContextCreateBase()
	fake := &contextCLI{base: base, list: testContextCreateBaseList(base)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\nstandalone\n1\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	if code := command.RunContext(context.Background(), []string{"manifest", "create"}); code != ExitOK {
		t.Fatalf("implicit Base create code = %d, stderr = %q", code, stderr.String())
	}
	basePosition, namePosition := strings.Index(stderr.String(), "Create Manifest · Copy"), strings.Index(stderr.String(), "Workspace Manifest name")
	if basePosition < 0 || namePosition < 0 || basePosition >= namePosition ||
		!strings.Contains(stderr.String(), "engineering (draft initializer only)") ||
		!strings.Contains(stderr.String(), "no lineage or inheritance") {
		t.Fatalf("implicit Base ordering/review = %q", stderr.String())
	}
	if fake.listCalls != 1 || fake.baseCalls != 1 || fake.createCalls != 1 || fake.lastComposition.CopyFrom == nil || fake.lastComposition.CopyFrom.Name != base.Name {
		t.Fatalf("implicit Base calls/composition = list:%d base:%d create:%d %+v", fake.listCalls, fake.baseCalls, fake.createCalls, fake.lastComposition)
	}
}

func TestContextCreateCatalogUsesBaseWithoutLineageOrFromAlias(t *testing.T) {
	command, ok := DefaultCatalog().Lookup("manifest create")
	if !ok {
		t.Fatal("context create is absent")
	}
	var base CommandInput
	found := false
	for _, input := range command.Agent.Inputs {
		if input.Name == "--copy-from" {
			base, found = input, true
		}
	}
	if !found || base.Required || base.ReferenceKind != "" || base.Completion != InputCompletionContextName || base.Cardinality != InputCardinalitySingle {
		t.Fatalf("--copy-from catalog input = %+v, found=%t", base, found)
	}
	for _, input := range command.Agent.Inputs {
		if input.Name == "--from" {
			t.Fatal("context create exposed the rejected --from lineage vocabulary")
		}
	}
	if strings.Contains(command.Args, "--from") {
		t.Fatal("context create exposed the rejected --from lineage vocabulary")
	}
}

func TestContextCreatePartialInteractiveInputPrefillsNameAndReviewsOmittedStages(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("2\n1\n1\n1\n1\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})

	if code := command.RunContext(context.Background(), []string{"manifest", "create", "--name", "sre3"}); code != ExitOK {
		t.Fatalf("partial interactive create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 1 || fake.prepareBootstrapCalls != 0 || fake.report.Name != "sre3" || fake.report.SourceAccess != tobari.ManifestSourceAccessReadOnly {
		t.Fatalf("partial interactive create = calls %d report %+v", fake.createCalls, fake.report)
	}
	if strings.Contains(stderr.String(), "Workspace Manifest name:") {
		t.Fatalf("supplied Name stage was replayed: %q", stderr.String())
	}
	for _, required := range []string{"Project source access", "Network access", "Ready Runtime revision", "Workspace bootstrap", "Review & Create"} {
		if !strings.Contains(stderr.String(), required) {
			t.Errorf("partial interactive flow lacks %q: %q", required, stderr.String())
		}
	}
	if !strings.Contains(stdout.String(), "Workspace Manifest sre3 created") {
		t.Fatalf("partial interactive success = %q", stdout.String())
	}
}

type canceledContextCreateWizard struct{}

func (canceledContextCreateWizard) Compose(context.Context, io.Reader, io.Writer) (contextCreateSelection, error) {
	return contextCreateSelection{}, context.Canceled
}

func TestContextCreateWizardCancellationPerformsZeroMutation(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.contextCreate = canceledContextCreateWizard{}

	if code := command.RunContext(context.Background(), []string{"manifest", "create"}); code != ExitCanceled {
		t.Fatalf("canceled create wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 0 || fake.prepareBootstrapCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("canceled create mutated: create/prepare/stdout = %d/%d/%q", fake.createCalls, fake.prepareBootstrapCalls, stdout.String())
	}
}

func TestContextDeleteRendersConfirmedOutcomeAndAppearsInNamespaceHelp(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"manifest", "delete", "--name", "coding"}); code != ExitOK {
		t.Fatalf("context delete code = %d, stderr = %q", code, stderr.String())
	}
	if fake.deleteCalls != 1 || !strings.Contains(stdout.String(), "Workspace Manifest deleted: coding") ||
		!strings.Contains(stdout.String(), "Preserved: project files and shared runtime images") {
		t.Fatalf("context delete calls/output = %d/%q", fake.deleteCalls, stdout.String())
	}
	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "--help"}); code != ExitOK {
		t.Fatalf("context help code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "delete       Delete one unused non-default Workspace Manifest") {
		t.Fatalf("context help lacks delete: %q", stdout.String())
	}
}

func TestRuntimeBuildFailureKeepsDockerErrorAndEndsWithActionableSummary(t *testing.T) {
	fake := &contextCLI{
		report:   contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided),
		buildLog: "#7 [2/2] RUN gh --version\n > [2/2] RUN gh --version:\n/bin/sh: gh: not found\nERROR: process failed\n",
		buildErr: errors.New("synthetic Docker build failure"),
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{buildLog: fake.buildLog, buildErr: fake.buildErr})

	code := command.RunContext(context.Background(), []string{"runtime", "build", "--name", "frontend"})
	if code != ExitRejected {
		t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("runtime build failure stdout = %q", stdout.String())
	}
	for _, retained := range []string{
		"RUN gh --version",
		"/bin/sh: gh: not found",
		"Runtime could not be built",
		"runtime_build_failed",
		"tobari runtime show",
		"unchanged Runtime history and source path",
	} {
		if !strings.Contains(stderr.String(), retained) {
			t.Fatalf("runtime build stderr = %q, missing %q", stderr.String(), retained)
		}
	}
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("non-TTY runtime build stderr contains ANSI: %q", stderr.String())
	}
}

func TestRuntimeCreateOutputDeclaresSourcePermissionAndSizeRules(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{})
	if code := command.RunContext(context.Background(), []string{"runtime", "create", "--name", "frontend"}); code != ExitOK {
		t.Fatalf("runtime create code = %d, stderr = %q", code, stderr.String())
	}
	for _, want := range []string{"Source rules", "no group/other permissions", "1,024 files", "256 directories", "32 MiB/file", "64 MiB total"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("runtime create output = %q, missing %q", stdout.String(), want)
		}
	}
}

func TestRuntimeCreateExplicitAndNonInteractiveBaseSelection(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		base tobari.RuntimeCopySource
	}{
		{name: "explicit managed", args: []string{"runtime", "create", "--copy-source-from", "frontend", "--name", "mobile"}, base: "frontend"},
		{name: "redirected omission", args: []string{"runtime", "create", "--name", "mobile"}, base: tobari.RuntimeCopySource(tobari.StandardRuntimeName)},
		{name: "JSON omission", args: []string{"runtime", "create", "--name", "mobile", "--format", "json"}, base: tobari.RuntimeCopySource(tobari.StandardRuntimeName)},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &runtimeCatalogCLI{list: runtimeReviewList(readyRuntimeManifest())}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitOK {
				t.Fatalf("runtime create code = %d, stderr = %q", code, stderr.String())
			}
			if fake.createCalls != 1 || fake.lastCreate != "mobile" || fake.lastBase != test.base || fake.listCalls != 0 {
				t.Fatalf("create/list calls = %d %q %q / %d", fake.createCalls, fake.lastCreate, fake.lastBase, fake.listCalls)
			}
			if strings.Contains(stdout.String(), `"base"`) || strings.Contains(stdout.String(), "CopyFrom:") {
				t.Fatalf("Runtime result persisted or presented lineage: %q", stdout.String())
			}
		})
	}
}

func TestRuntimeCreateInteractiveBaseSelectionAndStandardOnlySkip(t *testing.T) {
	manifest := readyRuntimeManifest()
	for _, test := range []struct {
		name     string
		items    []tobari.RuntimeSummary
		input    string
		raw      bool
		wantBase tobari.RuntimeCopySource
		wantMenu bool
	}{
		{name: "standard only", items: runtimeReviewList(manifest)[:1], wantBase: tobari.RuntimeCopySource(tobari.StandardRuntimeName)},
		{name: "line managed", items: runtimeReviewList(manifest), input: "2\n", wantBase: "frontend", wantMenu: true},
		{name: "raw managed", items: runtimeReviewList(manifest), input: "\x1b[B\n", raw: true, wantBase: "frontend", wantMenu: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &runtimeCatalogCLI{manifest: manifest, list: test.items}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(test.input), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
			chooser := &terminalContextConfigurationWizard{mode: nil, style: false}
			if test.raw {
				chooser.mode = &selectorModeFake{}
			}
			command.config = chooser
			if code := command.RunContext(context.Background(), []string{"runtime", "create", "--name", "mobile"}); code != ExitOK {
				t.Fatalf("runtime create code = %d, stderr = %q", code, stderr.String())
			}
			if fake.listCalls != 1 || fake.createCalls != 1 || fake.lastBase != test.wantBase {
				t.Fatalf("interactive create calls/base = list %d create %d base %q", fake.listCalls, fake.createCalls, fake.lastBase)
			}
			shown := strings.Contains(stderr.String(), "Tobari · Create Runtime · Base")
			if shown != test.wantMenu {
				t.Fatalf("Base menu shown = %t, want %t: %q", shown, test.wantMenu, stderr.String())
			}
		})
	}
}

func TestRuntimeCreateBaseChooserCancellationCreatesNothing(t *testing.T) {
	manifest := readyRuntimeManifest()
	fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("q\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}
	if code := command.RunContext(context.Background(), []string{"runtime", "create", "--name", "mobile"}); code != ExitCanceled {
		t.Fatalf("canceled Runtime create code = %d, stderr = %q", code, stderr.String())
	}
	if fake.createCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("canceled Runtime create calls/output = %d/%q", fake.createCalls, stdout.String())
	}
}

func TestRuntimeBuildSourceValidationFailureIsActionableInTextAndJSON(t *testing.T) {
	message := "Runtime source file \"bin/tool\" is 33554433 bytes; the limit is 33554432 bytes (32 MiB)."
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "text", args: []string{"runtime", "build", "--name", "frontend"}, want: []string{"runtime_source_invalid", message, "tobari runtime show", "unchanged Runtime source path and history"}},
		{name: "json", args: []string{"--error-format", "json", "runtime", "build", "--name", "frontend"}, want: []string{`"code":"runtime_source_invalid"`, `"kind":"rejected"`, `"message":"Runtime source file \"bin/tool\" is 33554433 bytes; the limit is 33554432 bytes (32 MiB)."`, `"command":"runtime show"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &runtimeCatalogCLI{buildErr: fault.New(fault.KindRejected, "runtime_source_invalid", message, false)}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitRejected {
				t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Fatalf("runtime build stdout = %q", stdout.String())
			}
			for _, want := range test.want {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("runtime build stderr = %q, missing %q", stderr.String(), want)
				}
			}
		})
	}
}

func TestRuntimeBuildDiagnosticStreamProjectsTerminalControls(t *testing.T) {
	fake := &contextCLI{
		report:   contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided),
		buildLog: "RUN tool\\literal\tvalue\x1b[31m\u202etest\nERROR: tool not found\n",
		buildErr: errors.New("synthetic Docker build failure"),
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{buildLog: fake.buildLog, buildErr: fake.buildErr})
	if code := command.RunContext(context.Background(), []string{"runtime", "build", "--name", "frontend"}); code != ExitRejected {
		t.Fatalf("runtime build code = %d", code)
	}
	value := stderr.String()
	for _, projected := range []string{`tool\\literal\tvalue\u001B[31m\u202Etest`, "ERROR: tool not found"} {
		if !strings.Contains(value, projected) {
			t.Fatalf("projected stderr = %q, missing %q", value, projected)
		}
	}
	if strings.Contains(value, "\x1b") || strings.Contains(value, "\u202e") {
		t.Fatalf("projected stderr retains terminal controls: %q", value)
	}
}

func TestRuntimeBuildFailureDetailsCoverDockerFailureClasses(t *testing.T) {
	tests := []struct {
		name      string
		log       string
		wantStep  string
		wantError string
	}{
		{
			name:     "Dockerfile syntax",
			log:      "ERROR: failed to solve: failed to read dockerfile: dockerfile parse error on line 4\n",
			wantStep: "Parse Dockerfile", wantError: "dockerfile parse error",
		},
		{
			name:     "RUN command",
			log:      "#7 [2/2] RUN gh --version\n/bin/sh: gh: not found\n#7 ERROR: process failed\n",
			wantStep: "RUN gh --version", wantError: "/bin/sh: gh: not found",
		},
		{
			name:     "base image",
			log:      "#5 [internal] load metadata for example.invalid/missing:latest\n#5 ERROR: failed to resolve source metadata\n",
			wantStep: "load metadata for example.invalid/missing:latest", wantError: "failed to resolve source metadata",
		},
		{
			name:     "daemon",
			log:      "ERROR: Cannot connect to the Docker daemon at unix:///var/run/docker.sock\n",
			wantStep: "Connect to Docker daemon", wantError: "Cannot connect to the Docker daemon",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			step, diagnostic := runtimeBuildFailureDetails(tobari.RuntimeBuildStageBuild, []byte(test.log))
			if !strings.Contains(step, test.wantStep) || !strings.Contains(diagnostic, test.wantError) {
				t.Fatalf("details = %q / %q", step, diagnostic)
			}
		})
	}
}

func TestRuntimeInitIsRetiredAndOmittedBuildSelectorRequiresInteractiveReview(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	if code := command.RunContext(context.Background(), []string{"runtime", "init"}); code != ExitUsage || !strings.Contains(stderr.String(), "Unknown command") {
		t.Fatalf("runtime init retirement = %d/%q", code, stderr.String())
	}
	stderr.Reset()
	fake := &runtimeCatalogCLI{manifest: readyRuntimeManifest()}
	command.runtime = runtimecmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitUsage || !strings.Contains(stderr.String(), "runtime_review_unavailable") {
		t.Fatalf("runtime build selection = %d/%q", code, stderr.String())
	}
	if fake.listCalls != 0 || fake.buildCalls != 0 {
		t.Fatalf("unavailable Review read/build calls = %d/%d, want zero", fake.listCalls, fake.buildCalls)
	}
}

func TestRuntimeBuildReviewSelectsAndConfirmsBeforeBuild(t *testing.T) {
	manifest := readyRuntimeManifest()
	fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitOK {
		t.Fatalf("runtime build Review code = %d, stderr = %q", code, stderr.String())
	}
	if fake.buildCalls != 1 || fake.lastBuild != "frontend" || fake.listCalls != 1 || fake.showCalls != 1 {
		t.Fatalf("runtime Review calls = list %d show %d build %d/%q", fake.listCalls, fake.showCalls, fake.buildCalls, fake.lastBuild)
	}
	for _, want := range []string{"Tobari · Build Runtime", "Runtime: frontend", "Source: /config/runtimes/frontend/source", "No Workspace Manifest Runtime binding will change.", "Build Runtime"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("runtime Review stderr = %q, missing %q", stderr.String(), want)
		}
	}
	if !strings.Contains(stdout.String(), "Runtime frontend") || !strings.Contains(stdout.String(), "unchanged") {
		t.Fatalf("runtime build confirmed stdout = %q", stdout.String())
	}
}

func TestRuntimeBuildReviewCancellationPerformsZeroBuild(t *testing.T) {
	manifest := readyRuntimeManifest()
	fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n3\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitCanceled {
		t.Fatalf("canceled runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if fake.buildCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("canceled runtime Review build/stdout = %d/%q", fake.buildCalls, stdout.String())
	}
}

func TestRuntimeBuildReviewRejectsCatalogWithoutManagedRuntimeBeforeBuild(t *testing.T) {
	standard := runtimeReviewList(readyRuntimeManifest())[:1]
	fake := &runtimeCatalogCLI{manifest: readyRuntimeManifest(), list: standard}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitNotFound || !strings.Contains(stderr.String(), "managed_runtime_not_found") {
		t.Fatalf("managed Runtime empty state code/stderr = %d/%q", code, stderr.String())
	}
	if fake.buildCalls != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "tobari help runtime create") {
		t.Fatalf("managed Runtime empty state build/stdout/stderr = %d/%q/%q", fake.buildCalls, stdout.String(), stderr.String())
	}
}

func TestRuntimeBuildFullySpecifiedRemainsDirect(t *testing.T) {
	manifest := readyRuntimeManifest()
	fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)

	if code := command.RunContext(context.Background(), []string{"runtime", "build", "--name", "frontend"}); code != ExitOK {
		t.Fatalf("direct runtime build code = %d, stderr = %q", code, stderr.String())
	}
	if fake.buildCalls != 1 || fake.listCalls != 0 || fake.showCalls != 0 {
		t.Fatalf("direct runtime build calls = build/list/show %d/%d/%d", fake.buildCalls, fake.listCalls, fake.showCalls)
	}
	if !strings.Contains(stdout.String(), "tobari manifest runtime set --runtime frontend@1") {
		t.Fatalf("direct runtime build output lacks exact selection handoff: %q", stdout.String())
	}
}

func TestContextRuntimeReviewSelectsRevisionAndAppliesOnce(t *testing.T) {
	manifest := readyRuntimeManifest()
	contextFake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n2\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set"}); code != ExitOK {
		t.Fatalf("Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.setRuntimeCalls != 1 || contextFake.lastRuntimeContext != "web" || contextFake.lastRuntimeSelection != "frontend@1" {
		t.Fatalf("Workspace Manifest Runtime Apply = %d/%q/%q", contextFake.setRuntimeCalls, contextFake.lastRuntimeContext, contextFake.lastRuntimeSelection)
	}
	for _, want := range []string{
		"Tobari · Set Workspace Manifest Runtime · Review",
		"Workspace Manifest: web · current",
		"Runtime: standard@1 → frontend@1",
		"next Workspace entry",
		"Apply change",
		"Back to Runtime list",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("Workspace Manifest Runtime Review stderr = %q, missing %q", stderr.String(), want)
		}
	}
	if strings.Contains(stderr.String(), "Apply is unavailable") || strings.Count(stderr.String(), "Apply change") != 1 {
		t.Fatalf("Workspace Manifest Runtime Review did not isolate Apply to one Review state: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "frontend@1") ||
		!strings.Contains(stdout.String(), applyStyleToken(true, styleAccent, "`tobari`")) ||
		!strings.Contains(stdout.String(), "from the project directory to adopt the selected Runtime on entry") {
		t.Fatalf("Workspace Manifest Runtime confirmed stdout = %q", stdout.String())
	}
}

func TestContextRuntimeSetNonCurrentHandoffKeepsExactContext(t *testing.T) {
	manifest := readyRuntimeManifest()
	report := contextCLIReport(tobari.TaskManifestShow, "web", false, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)
	contextFake := &contextCLI{report: report}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set", "--manifest", "web", "--runtime", manifest.Name + "@1"}); code != ExitOK {
		t.Fatalf("non-current Workspace Manifest Runtime set code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Next: run `tobari --manifest web` from the project directory") {
		t.Fatalf("non-current Workspace Manifest Runtime handoff = %q", stdout.String())
	}
}

func TestContextRuntimeReviewLoadsCompleteSuccessfulHistoryForRollback(t *testing.T) {
	manifest := readyRuntimeManifestWithHistory()
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(runtimeFake)

	choices, err := loadReadyRuntimeChoices(context.Background(), command)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"standard", "frontend@2", "frontend@1"}
	got := make([]string, 0, len(choices))
	for _, choice := range choices {
		got = append(got, choice.selection)
	}
	if !reflect.DeepEqual(got, want) || runtimeFake.historyCalls != 1 {
		t.Fatalf("ready Runtime choices/history calls = %v/%d, want %v/1", got, runtimeFake.historyCalls, want)
	}
}

func TestContextRuntimeReviewCanChangeOmittedContextBeforeApply(t *testing.T) {
	manifest := readyRuntimeManifest()
	web := contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)
	review := contextCLIReport(tobari.TaskManifestShow, "review", false, manifest.Revisions[0].Image, tobari.ManifestPolicyModeGuided)
	review.ID = "018bcfe5-687b-7000-8000-000000000100"
	review.Runtime = tobari.ManifestRuntimeReport{
		Kind: tobari.ManifestRuntimeKindManaged, Status: tobari.ManifestRuntimeStatusReady,
		Image: manifest.Revisions[0].Image, RuntimeID: manifest.ID, Name: manifest.Name,
		Revision: manifest.Revisions[0].Revision, Ordinal: 1,
	}
	contextFake := &contextCLI{
		report:  web,
		reports: map[string]tobari.ManifestReport{"web": web, "review": review},
		list: tobari.ManifestListResult{
			Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationPersisted,
			DefaultManifestID: web.ID, DefaultManifest: "web",
			Items: []tobari.ManifestSummary{contextSummaryFromReport(web), contextSummaryFromReport(review)},
		},
	}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("2\n2\n\n2\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set"}); code != ExitOK {
		t.Fatalf("change-Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.listCalls != 1 || contextFake.lastRuntimeContext != "review" || contextFake.lastRuntimeSelection != tobari.StandardRuntimeName {
		t.Fatalf("change-Workspace Manifest Apply = list %d context %q Runtime %q", contextFake.listCalls, contextFake.lastRuntimeContext, contextFake.lastRuntimeSelection)
	}
	for _, want := range []string{"Persisted Workspace Manifest", "review — persisted", "Workspace Manifest: review", "Runtime: frontend@1 → standard@1", "Tobari · Set Workspace Manifest Runtime · Review"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("change-Workspace Manifest Review stderr = %q, missing %q", stderr.String(), want)
		}
	}
}

func TestContextRuntimeReviewKeepsExplicitContextFixed(t *testing.T) {
	manifest := readyRuntimeManifest()
	contextFake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n2\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set", "--manifest", "web"}); code != ExitOK {
		t.Fatalf("explicit-Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.listCalls != 0 || contextFake.lastRuntimeContext != "web" || contextFake.lastRuntimeSelection != "frontend@1" {
		t.Fatalf("explicit-Workspace Manifest Apply = list %d context %q Runtime %q", contextFake.listCalls, contextFake.lastRuntimeContext, contextFake.lastRuntimeSelection)
	}
	if strings.Contains(stderr.String(), "Change Workspace Manifest") || !strings.Contains(stderr.String(), "Runtime: standard@1 → frontend@1") {
		t.Fatalf("explicit Workspace Manifest was not fixed through Review: %q", stderr.String())
	}
}

func TestContextRuntimeReviewBackReopensRuntimeListWithoutApplying(t *testing.T) {
	manifest := readyRuntimeManifest()
	contextFake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n2\n2\n2\n3\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set"}); code != ExitCanceled {
		t.Fatalf("back then cancel Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.setRuntimeCalls != 0 || stdout.Len() != 0 {
		t.Fatalf("back then cancel mutation/stdout = %d/%q", contextFake.setRuntimeCalls, stdout.String())
	}
	if strings.Count(stderr.String(), "Ready Runtime revision:") != 2 || strings.Count(stderr.String(), "Tobari · Set Workspace Manifest Runtime · Review") != 1 {
		t.Fatalf("Back did not return from one Review to the Runtime list: %q", stderr.String())
	}
}

func TestContextRuntimeReviewUnchangedSelectionCannotApply(t *testing.T) {
	manifest := readyRuntimeManifest()
	contextFake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n\n3\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.config = &terminalContextConfigurationWizard{mode: nil, style: false}

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set"}); code != ExitCanceled {
		t.Fatalf("unchanged Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.setRuntimeCalls != 0 || stdout.Len() != 0 || strings.Contains(stderr.String(), "Apply") || strings.Contains(stderr.String(), "· Review") || strings.Count(stderr.String(), "Tobari · Set Workspace Manifest Runtime\n") != 2 {
		t.Fatalf("unchanged Review mutation/stdout/stderr = %d/%q/%q", contextFake.setRuntimeCalls, stdout.String(), stderr.String())
	}
}

func TestContextRuntimeOmittedSelectorRejectsNonInteractiveAndDirectModeSkipsReads(t *testing.T) {
	manifest := readyRuntimeManifest()
	contextFake := &contextCLI{report: contextCLIReport(tobari.TaskManifestShow, "web", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)}
	runtimeFake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(contextFake)
	command.runtime = runtimecmd.New(runtimeFake)

	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set"}); code != ExitUsage || !strings.Contains(stderr.String(), "runtime_review_unavailable") {
		t.Fatalf("non-interactive Workspace Manifest Runtime code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.showCalls != 0 || contextFake.setRuntimeCalls != 0 || runtimeFake.listCalls != 0 {
		t.Fatalf("non-interactive Review calls = show/set/list %d/%d/%d", contextFake.showCalls, contextFake.setRuntimeCalls, runtimeFake.listCalls)
	}
	stderr.Reset()
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set", "--format", "json"}); code != ExitUsage || !strings.Contains(stderr.String(), "runtime_review_unavailable") {
		t.Fatalf("JSON Workspace Manifest Runtime Review code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.showCalls != 0 || contextFake.setRuntimeCalls != 0 || runtimeFake.listCalls != 0 {
		t.Fatalf("JSON Review calls = show/set/list %d/%d/%d", contextFake.showCalls, contextFake.setRuntimeCalls, runtimeFake.listCalls)
	}

	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "runtime", "set", "--runtime", "frontend@1"}); code != ExitOK {
		t.Fatalf("direct Workspace Manifest Runtime code = %d, stderr = %q", code, stderr.String())
	}
	if contextFake.setRuntimeCalls != 1 || contextFake.showCalls != 0 || runtimeFake.listCalls != 0 {
		t.Fatalf("direct Workspace Manifest Runtime calls = set/show/list %d/%d/%d", contextFake.setRuntimeCalls, contextFake.showCalls, runtimeFake.listCalls)
	}
}

func runtimeInitReportFixture() tobari.ManifestReport {
	return tobari.ManifestReport{
		Task:           tobari.TaskRuntimeInit,
		ManifestState:  tobari.ManifestObservationPersisted,
		ID:             "018bcfe5-687b-7000-8000-000000000099",
		Name:           "default",
		Default:        true,
		Desired:        cliTestManifestRevision("e"),
		AgentProfile:   tobari.DefaultProfile,
		Image:          tobari.OfficialRuntimeBase,
		PolicyMode:     tobari.ManifestPolicyModeGuided,
		SourceAccess:   tobari.ManifestSourceAccessReadWrite,
		PolicyRevision: tobari.DefaultContextPolicyRevision(), MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Stores: tobari.ManifestStorePaths{
			PolicyDirectory: "/config/contexts/default/policy",
		},
		Runtime: tobari.ManifestRuntimeReport{
			Kind:          tobari.ManifestRuntimeKindDockerfile,
			Status:        tobari.ManifestRuntimeStatusPendingBuild,
			Dockerfile:    "/config/contexts/default/runtime/Dockerfile",
			BaseReference: tobari.OfficialRuntimeBase,
			SourceDigest:  "sha256:" + strings.Repeat("a", 64),
			ImageDigest:   "sha256:" + strings.Repeat("b", 64),
		},
		Cluster:        tobari.ManifestClusterStatusNotApplicable,
		Authentication: tobari.ManifestAuthentication{BrokerState: tobari.ManifestAuthBrokerNotApplicable},
	}
}

func TestRuntimeInitTextSnapshotPrioritizesNextActions(t *testing.T) {
	output, err := renderContextReport(runtimeInitReportFixture(), successFormatText, false)
	if err != nil {
		t.Fatalf("renderWorkspace ManifestReport() error = %v", err)
	}
	want := "✓ Runtime Dockerfile created\n\n" +
		"Next\n" +
		"  1. Edit the Dockerfile\n" +
		"     /config/contexts/default/runtime/Dockerfile\n\n" +
		"  2. Build the runtime\n" +
		"     tobari runtime build\n\n" +
		"Details\n" +
		"  Workspace Manifest default\n" +
		"  Base image     tobari-runtime:base\n" +
		"  Status         pending_build\n"
	if got := string(output); got != want {
		t.Fatalf("runtime init text = %q, want snapshot %q", got, want)
	}
	if strings.Index(string(output), "Next\n") > strings.Index(string(output), "Details\n") {
		t.Fatalf("Next section was rendered after Details: %q", output)
	}
	for _, omitted := range []string{
		"Agent profile:", "Policy mode:", "Runtime source digest:",
		"Runtime image digest:", "Policy:",
	} {
		if strings.Contains(string(output), omitted) {
			t.Fatalf("runtime init primary output contains diagnostic %q: %q", omitted, output)
		}
	}
}

func TestRuntimeInitTextColorDisabledRetainsPriorityAndValueEmphasis(t *testing.T) {
	fixture := runtimeInitReportFixture()
	plain := string(renderContextReportText(fixture, false))
	if strings.Contains(plain, "\x1b[") {
		t.Fatalf("color-disabled runtime init output contains ANSI: %q", plain)
	}
	if strings.Index(plain, "Next\n") > strings.Index(plain, "Details\n") {
		t.Fatalf("color-disabled output loses section priority: %q", plain)
	}

	styled := string(renderContextReportText(fixture, true))
	if !strings.Contains(styled, applyStyleToken(true, styleAccent, "tobari runtime build")) {
		t.Fatalf("styled output does not accent the next command: %q", styled)
	}
	for _, ordinary := range []string{"Runtime Dockerfile created", fixture.Runtime.Dockerfile, fixture.Name, fixture.Runtime.BaseReference} {
		for _, token := range []styleToken{styleMuted, styleAccent, styleSuccess, styleWarning, styleDanger} {
			if strings.Contains(styled, applyStyleToken(true, token, ordinary)) {
				t.Fatalf("styled output applies %s to ordinary value %q: %q", token, ordinary, styled)
			}
		}
	}
}

func TestContextShowPrioritizesBoundaryAndExpandsDiagnostics(t *testing.T) {
	fixture := contextCLIReport(tobari.TaskManifestShow, "default", true, "tobari-runtime-frontend:aaaaaaaaaaaa", tobari.ManifestPolicyModeGuided)
	fixture.Runtime = tobari.ManifestRuntimeReport{
		Kind: tobari.ManifestRuntimeKindManaged, Status: tobari.ManifestRuntimeStatusReady,
		Image: fixture.Image, RuntimeID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend",
		Revision: "sha256:" + strings.Repeat("a", 64), Ordinal: 4,
	}
	fixture.Authentication = tobari.ManifestAuthentication{
		BrokerState: tobari.ManifestAuthBrokerReady,
		Providers: []tobari.ManifestAuthProvider{{
			Provider: "github", State: tobari.ManifestAuthProviderNotConfigured,
		}},
	}
	fake := &contextCLI{report: fixture}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	if code := command.RunContext(context.Background(), []string{"manifest", "show"}); code != ExitOK {
		t.Fatalf("context show code = %d, stderr = %q", code, stderr.String())
	}
	for _, retained := range []string{
		"Workspace Manifest default",
		"Boundary · fixed for this Workspace Manifest",
		"Project files  Read-write · changes affect this project directly",
		"Routine clients Ready",
		"Other requests Exact review",
		"Private targets Denied",
		"Runtime binding · adopted on next Workspace entry",
		"Selected       frontend@4",
		"Workspace defaults",
		"Later entries and sessions",
		"New Workspace homes only · existing homes unchanged",
		"Login ownership",
		"Details        tobari manifest show --details",
		"Next           tobari",
	} {
		if !strings.Contains(stdout.String(), retained) {
			t.Fatalf("context show output = %q, missing primary fact %q", stdout.String(), retained)
		}
	}
	for _, diagnostic := range []string{
		"/config/contexts/default/policy",
		"018bcfe5-687b-7000-8000-000000000077",
		"sha256:" + strings.Repeat("a", 64),
	} {
		if strings.Contains(stdout.String(), diagnostic) {
			t.Fatalf("context show primary output = %q, contains diagnostic %q", stdout.String(), diagnostic)
		}
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "show", "--details"}); code != ExitOK {
		t.Fatalf("context show --details code = %d, stderr = %q", code, stderr.String())
	}
	for _, retained := range []string{
		"Boundary · fixed for this Workspace Manifest",
		"Runtime binding · adopted on next Workspace entry",
		"Workspace defaults",
		"Login ownership",
		"Stores and revisions",
		"/config/contexts/default/policy",
		"018bcfe5-687b-7000-8000-000000000077",
		"sha256:" + strings.Repeat("a", 64),
	} {
		if !strings.Contains(stdout.String(), retained) {
			t.Fatalf("context show --details output = %q, missing diagnostic %q", stdout.String(), retained)
		}
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "show", "--format=json"}); code != ExitOK {
		t.Fatalf("context show JSON code = %d, stderr = %q", code, stderr.String())
	}
	compactJSON := append([]byte(nil), stdout.Bytes()...)
	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"manifest", "show", "--details", "--format=json"}); code != ExitOK {
		t.Fatalf("context show detailed JSON code = %d, stderr = %q", code, stderr.String())
	}
	if !bytes.Equal(stdout.Bytes(), compactJSON) {
		t.Fatalf("--details changed complete JSON\n--- got ---\n%s--- want ---\n%s", stdout.Bytes(), compactJSON)
	}
}

func TestContextShowNamesConfiguredNewWorkspaceSetupWithoutClaimingCurrentState(t *testing.T) {
	fixture := contextCLIReport(tobari.TaskManifestShow, "default", true, tobari.OfficialRuntimeBase, tobari.ManifestPolicyModeGuided)
	fixture.Bootstrap = tobari.ManifestBootstrapReport{
		State: tobari.ManifestBootstrapConfigured, Generation: 3,
		Revision:   "sha256:" + strings.Repeat("c", 64),
		Adapters:   []string{tobari.ManifestBootstrapAdapterAWS, tobari.ManifestBootstrapAdapterEKS},
		AWSProfile: "engineering", EKSContext: "platform",
	}

	for _, rendered := range [][]byte{
		renderContextShowSummaryText(fixture, false),
		renderContextShowDetailsText(fixture, false),
	} {
		text := string(rendered)
		for _, want := range []string{
			"New Workspace homes only · existing homes unchanged",
			"AWS            engineering",
			"Kubernetes EKS platform",
		} {
			if !strings.Contains(text, want) {
				t.Fatalf("configured new-Workspace setup = %q, missing %q", text, want)
			}
		}
		if strings.Contains(text, "Bootstrap") || strings.Contains(text, "currently applied") {
			t.Fatalf("configured new-Workspace setup invents current state: %q", text)
		}
	}
}
