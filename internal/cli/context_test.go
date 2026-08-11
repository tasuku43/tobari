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

	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextCLI fakeContextRuntime

func (f *contextCLI) ListContexts(context.Context) (tobari.ContextListResult, error) {
	return f.list, nil
}

func (f *contextCLI) ShowContext(context.Context, string) (tobari.ContextReport, error) {
	f.showCalls++
	return f.report, f.showErr
}

func (f *contextCLI) CreateContext(
	_ context.Context, name, image string, mode tobari.ContextPolicyMode,
) (tobari.ContextReport, error) {
	f.report = contextCLIReport(tobari.TaskContextCreate, name, false, image, mode)
	return f.report, nil
}

func (f *contextCLI) UseContext(context.Context, string) (tobari.ContextReport, error) {
	f.useCalls++
	f.report.Task = tobari.TaskContextUse
	f.report.Active = true
	f.report.Authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) ConfigureContextShell(
	_ context.Context, name string, changes []tobari.ContextShellEnvironmentSetting,
) (tobari.ContextReport, error) {
	f.configureCalls++
	f.lastShellChanges = append([]tobari.ContextShellEnvironmentSetting(nil), changes...)
	if len(changes) > 0 {
		f.lastShellChange = changes[0]
	}
	f.lastShellContext = name
	f.report.Task = tobari.TaskConfigShell
	if name == "" {
		f.report.Active = true
	} else {
		f.report.Name = name
	}
	f.report.Authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	overrides := []tobari.ContextShellEnvironmentSetting{}
	for _, change := range changes {
		if change.Source != tobari.ContextShellEnvironmentDefault {
			overrides = append(overrides, change)
		}
	}
	f.report.ShellEnvironment, _ = tobari.CompleteContextShellEnvironment(overrides)
	return f.report, nil
}

func (f *contextCLI) ConfigureContextGit(
	_ context.Context, name string, change tobari.ContextGitIdentitySetting,
) (tobari.ContextReport, error) {
	f.configureGitCalls++
	f.lastGitChange = change
	f.lastGitContext = name
	f.report.Task = tobari.TaskConfigGit
	if name == "" {
		f.report.Active = true
	} else {
		f.report.Name = name
	}
	f.report.GitIdentity = change
	f.report.Authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) InitRuntime(context.Context) (tobari.ContextReport, error) {
	f.report.Task = tobari.TaskRuntimeInit
	f.report.Authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	return f.report, nil
}

func (f *contextCLI) BuildRuntime(context.Context) (tobari.ContextReport, error) {
	f.buildCalls++
	f.report.Task = tobari.TaskRuntimeBuild
	f.report.Authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	return f.report, f.buildErr
}

func (f *contextCLI) BuildRuntimeWithProgress(
	_ context.Context, diagnostics io.Writer, progress tobari.RuntimeBuildProgressSink,
) (tobari.ContextReport, error) {
	f.buildCalls++
	f.report.Task = tobari.TaskRuntimeBuild
	metadata := tobari.RuntimeBuildProgress{
		ContextName: "default", Dockerfile: "/config/contexts/default/runtime/Dockerfile",
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
		return tobari.ContextReport{}, f.buildErr
	}
	emit(tobari.RuntimeBuildStageBuild, tobari.RuntimeBuildProgressCompleted)
	return f.report, nil
}

type fakeContextRuntime struct {
	list              tobari.ContextListResult
	report            tobari.ContextReport
	useCalls          int
	buildCalls        int
	buildLog          string
	buildErr          error
	configureCalls    int
	configureGitCalls int
	showCalls         int
	showErr           error
	lastShellChange   tobari.ContextShellEnvironmentSetting
	lastShellChanges  []tobari.ContextShellEnvironmentSetting
	lastGitChange     tobari.ContextGitIdentitySetting
	lastShellContext  string
	lastGitContext    string
}

type contextSwitchingWizard struct {
	runtime  *contextCLI
	seenName string
}

func (w *contextSwitchingWizard) switchActive(current tobari.ContextReport) {
	w.seenName = current.Name
	w.runtime.report = contextCLIReport(
		tobari.TaskContextShow, "switched", true,
		tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided,
	)
}

func (w *contextSwitchingWizard) ConfigureShell(
	_ context.Context, current tobari.ContextReport, _ io.Reader, _ io.Writer,
) ([]tobari.ContextShellEnvironmentSetting, error) {
	w.switchActive(current)
	return []tobari.ContextShellEnvironmentSetting{{
		Variable: "PS1", Source: tobari.ContextShellEnvironmentInherit,
	}}, nil
}

func (w *contextSwitchingWizard) ConfigureGit(
	_ context.Context, current tobari.ContextReport, _ io.Reader, _ io.Writer,
) (tobari.ContextGitIdentitySetting, error) {
	w.switchActive(current)
	return tobari.ContextGitIdentitySetting{Source: tobari.ContextGitIdentityInherit}, nil
}

func TestContextUseReportsReconciliationStatusAndParsesBeforeMutation(t *testing.T) {
	t.Parallel()
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "project-tools", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	fake.report.Cluster = tobari.ContextClusterStatusReconciled
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"context", "use", "--name", "project-tools", "--format", "yaml"}); code != ExitUsage {
		t.Fatalf("invalid format code = %d, stderr = %q", code, stderr.String())
	}
	if fake.useCalls != 0 {
		t.Fatalf("UseContext() calls after invalid format = %d, want 0", fake.useCalls)
	}
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "use", "--name", "project-tools", "--format", "json"}); code != ExitOK {
		t.Fatalf("context use code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		Context tobari.ContextReport `json:"context"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("context use JSON = %q, error = %v", stdout.String(), err)
	}
	if document.Context.Cluster != tobari.ContextClusterStatusReconciled || fake.useCalls != 1 {
		t.Fatalf("context use document/calls = %+v/%d", document.Context, fake.useCalls)
	}
}

func contextCLIReport(task, name string, active bool, image string, mode tobari.ContextPolicyMode) tobari.ContextReport {
	authentication := tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable}
	if task == tobari.TaskContextShow {
		authentication = tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerReady, Providers: []tobari.ContextAuthProvider{}}
	}
	return tobari.ContextReport{
		Task: task, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name, Active: active, AgentProfile: tobari.DefaultProfile,
		Image: image, PolicyMode: mode, Cluster: tobari.ContextClusterStatusNotApplicable,
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Runtime:          tobari.ContextRuntimeReport{Kind: tobari.ContextRuntimeKindOfficial, Status: tobari.ContextRuntimeStatusOfficial},
		Authentication:   authentication,
		Stores: tobari.ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials"),
		},
	}
}

func TestContextShellConfigurePreservesSourceAndExplicitEmptyValue(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	if code := command.RunContext(context.Background(), []string{
		"config", "shell", "--variable", "PS1", "--source", "literal", "--value=", "--format", "json",
	}); code != ExitOK {
		t.Fatalf("config shell code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		SchemaVersion int                  `json:"schema_version"`
		Context       tobari.ContextReport `json:"context"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("config shell JSON = %q, error = %v", stdout.String(), err)
	}
	if document.SchemaVersion != 7 || fake.configureCalls != 1 || fake.lastShellChange.Value == nil ||
		*fake.lastShellChange.Value != "" || document.Context.Task != tobari.TaskConfigShell {
		t.Fatalf("configure document/call = %+v / %d %+v", document, fake.configureCalls, fake.lastShellChange)
	}
	if fake.showCalls != 0 {
		t.Fatalf("direct config shell unexpectedly inspected the Context %d times", fake.showCalls)
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

func TestConfigNamespaceReplacesContextShellConfigure(t *testing.T) {
	catalog := DefaultCatalog()
	if _, found := catalog.Lookup("context shell configure"); found {
		t.Fatal("retired context shell configure path remains public")
	}
	for _, path := range []string{"config shell", "config git"} {
		spec, found := catalog.Lookup(path)
		if !found || spec.Role != RoleAct || spec.Effect.String() != "write" ||
			spec.Agent.FixedTarget == nil || spec.Agent.FixedTarget.Scope != FixedTargetScopeToolLocal {
			t.Fatalf("%s contract = %+v", path, spec)
		}
	}
	selected, exact := catalog.Select("config")
	if exact || len(selected) != 2 {
		t.Fatalf("config namespace selection exact=%t commands=%+v", exact, selected)
	}
}

func TestConfigGitDirectPreservesLiteralPairWithoutWizardRead(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	code := command.RunContext(context.Background(), []string{
		"config", "git", "--context", "work", "--source", "literal",
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
	if document.SchemaVersion != 7 || document.Context.Task != tobari.TaskConfigGit ||
		document.Context.GitIdentity.Source != tobari.ContextGitIdentityLiteral {
		t.Fatalf("config git document = %+v", document)
	}
}

func TestConfigDirectSourceOnlyModesNeverPrompt(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantShell tobari.ContextShellEnvironmentSource
		wantGit   tobari.ContextGitIdentitySource
	}{
		{name: "shell default", args: []string{"config", "shell", "--variable", "PS1", "--source", "default"}, wantShell: tobari.ContextShellEnvironmentDefault},
		{name: "shell inherit", args: []string{"config", "shell", "--variable", "PS1", "--source", "inherit"}, wantShell: tobari.ContextShellEnvironmentInherit},
		{name: "Git default", args: []string{"config", "git", "--source", "default"}, wantGit: tobari.ContextGitIdentityDefault},
		{name: "Git inherit", args: []string{"config", "git", "--source", "inherit"}, wantGit: tobari.ContextGitIdentityInherit},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
			command.context = contextcmd.New(fake)
			if code := command.RunContext(context.Background(), test.args); code != ExitOK {
				t.Fatalf("direct source-only code = %d, stderr = %q", code, stderr.String())
			}
			if fake.showCalls != 0 {
				t.Fatalf("direct source-only mode inspected Context %d times", fake.showCalls)
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
			fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
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
			name: "shell command-local equals", wantCode: "invalid_context_name",
			args: []string{"config", "shell", "--context=", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "shell command-local separated", wantCode: "invalid_context_name",
			args: []string{"config", "shell", "--context", "", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "Git command-local equals", wantCode: "invalid_context_name",
			args: []string{"config", "git", "--context=", "--source", "default"},
		},
		{
			name: "Git command-local separated", wantCode: "invalid_context_name",
			args: []string{"config", "git", "--context", "", "--source", "default"},
		},
		{
			name: "global equals", wantCode: "invalid_root_options",
			args: []string{"--context=", "config", "shell", "--variable", "PS1", "--source", "inherit"},
		},
		{
			name: "global separated", wantCode: "invalid_root_options",
			args: []string{"--context", "", "config", "git", "--source", "default"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{
				report: contextCLIReport(
					tobari.TaskContextShow, "default", true,
					tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided,
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
		{name: "shell non terminal", args: []string{"config", "shell", "--context", "work"}},
		{name: "Git non terminal", args: []string{"config", "git"}},
		{name: "shell JSON", args: []string{"config", "shell", "--format", "json"}},
		{name: "Git JSON", args: []string{"config", "git", "--format", "json"}},
		{name: "Git JSON errors", args: []string{"--error-format=json", "config", "git"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
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
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("l\nTobari User\ntobari@example.com\n\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "git", "--context", "work"}); code != ExitOK {
		t.Fatalf("wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureGitCalls != 1 || fake.lastGitChange.Source != tobari.ContextGitIdentityLiteral {
		t.Fatalf("wizard show/config/change = %d/%d/%+v", fake.showCalls, fake.configureGitCalls, fake.lastGitChange)
	}
	for _, want := range []string{"Tobari · Git identity", "Context: work", "Only user.name and user.email are projected.", "Apply this change?"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("wizard stderr = %q, missing %q", stderr.String(), want)
		}
	}
	for _, prompt := range []string{"Tobari · Git identity", "Only user.name and user.email are projected.", "Apply this change?"} {
		if strings.Contains(stdout.String(), prompt) {
			t.Fatalf("wizard prompt %q leaked to stdout: %q", prompt, stdout.String())
		}
	}
	if !strings.Contains(stdout.String(), "Context: work") || !strings.Contains(stdout.String(), "Git identity: literal") {
		t.Fatalf("confirmed report stdout = %q", stdout.String())
	}
}

func TestConfigShellLineWizardStagesMultipleSettingsInOneMutation(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("1\nh\n2\nd\np\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "shell", "--context", "work"}); code != ExitOK {
		t.Fatalf("wizard code = %d, stderr = %q", code, stderr.String())
	}
	if fake.showCalls != 1 || fake.configureCalls != 1 || len(fake.lastShellChanges) != 2 {
		t.Fatalf("wizard show/config/changes = %d/%d/%+v", fake.showCalls, fake.configureCalls, fake.lastShellChanges)
	}
	if fake.lastShellChanges[0].Variable != "COLORTERM" || fake.lastShellChanges[0].Source != tobari.ContextShellEnvironmentInherit ||
		fake.lastShellChanges[1].Variable != "NO_COLOR" || fake.lastShellChanges[1].Source != tobari.ContextShellEnvironmentDefault {
		t.Fatalf("wizard changes = %+v", fake.lastShellChanges)
	}
	for _, want := range []string{"Tobari · Shell configuration", "COLORTERM", "NO_COLOR", "Apply 2 changes"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("wizard stderr = %q, missing %q", stderr.String(), want)
		}
	}
	if !strings.Contains(stdout.String(), "Context: work") {
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
					tobari.TaskContextShow, "shown", true,
					tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided,
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
				t.Fatalf("wizard saw Context %q after %d reads", wizard.seenName, fake.showCalls)
			}
			configuredContext, configuredCalls := fake.lastShellContext, fake.configureCalls
			if test.name == "Git" {
				configuredContext, configuredCalls = fake.lastGitContext, fake.configureGitCalls
			}
			if configuredContext != "shown" || configuredCalls != 1 || configuredContext == "switched" {
				t.Fatalf("configured Context/calls = %q/%d, want shown/1", configuredContext, configuredCalls)
			}
		})
	}
}

func TestConfigWizardCancellationLeavesConfigurationUnchanged(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("q\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "git", "--context", "work"}); code != ExitCanceled {
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
		report:  contextCLIReport(tobari.TaskContextShow, "work", false, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided),
		showErr: context.Canceled,
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("must not be read"), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	command.tobari = tobaricmd.New(&policyReviewRuntimeFake{terminal: true})
	command.noColor = true

	if code := command.RunContext(context.Background(), []string{"config", "shell", "--context", "work"}); code != ExitCanceled {
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

func TestContextReportJSONSchemaSevenDeclaresExactContextKeys(t *testing.T) {
	report := contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)
	encoded, err := renderContextReport(report, successFormatJSON, false)
	if err != nil {
		t.Fatalf("renderContextReport() error = %v", err)
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &outer); err != nil {
		t.Fatalf("JSON = %q, error = %v", encoded, err)
	}
	var version int
	if err := json.Unmarshal(outer["schema_version"], &version); err != nil || version != 7 {
		t.Fatalf("schema version = %d, error = %v", version, err)
	}
	var contextFields map[string]json.RawMessage
	if err := json.Unmarshal(outer["context"], &contextFields); err != nil {
		t.Fatalf("context envelope = %q, error = %v", outer["context"], err)
	}
	want := []string{"active", "agent_profile", "authentication", "cluster", "git_identity", "id", "image", "name", "policy_mode", "runtime", "shell_environment", "stores", "task"}
	got := make([]string, 0, len(contextFields))
	for name := range contextFields {
		got = append(got, name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Context JSON keys = %v, want %v", got, want)
	}
	spec, _ := DefaultCatalog().Lookup("config git")
	declared := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		declared = append(declared, field.Name)
	}
	sort.Strings(declared)
	if !reflect.DeepEqual(declared, want) {
		t.Fatalf("declared Context output fields = %v, want %v", declared, want)
	}
}

func TestContextCommandsRenderActiveContextAndRuntimeImage(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	fake.list = tobari.ContextListResult{
		Task: tobari.TaskContextList, Active: "default",
		Items: []tobari.ContextSummary{{ID: "018bcfe5-687b-7000-8000-000000000099", Name: "default", Active: true, AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase, PolicyMode: tobari.ContextPolicyModeGuided}},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"context", "list"}); code != ExitOK {
		t.Fatalf("context list code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Current Context: default") || !strings.Contains(stdout.String(), "image="+tobari.OfficialRuntimeBase) {
		t.Fatalf("context list output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "show"}); code != ExitOK {
		t.Fatalf("context show code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime: official (official)") ||
		!strings.Contains(stdout.String(), "Shell PS1: default") ||
		!strings.Contains(stdout.String(), "run `tobari runtime init`") {
		t.Fatalf("context show output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "create", "--name", "project-tools", "--image", tobari.OfficialRuntimeBase, "--mode", "advanced", "--format", "json"}); code != ExitOK {
		t.Fatalf("context create code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		Context tobari.ContextReport `json:"context"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("context create JSON = %q, error = %v", stdout.String(), err)
	}
	if document.Context.Name != "project-tools" || document.Context.Image != tobari.OfficialRuntimeBase || document.Context.PolicyMode != tobari.ContextPolicyModeAdvanced {
		t.Fatalf("context create document = %+v", document.Context)
	}
}

func TestRuntimeBuildFailureKeepsDockerErrorAndEndsWithActionableSummary(t *testing.T) {
	fake := &contextCLI{
		report:   contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided),
		buildLog: "#7 [2/2] RUN gh --version\n > [2/2] RUN gh --version:\n/bin/sh: gh: not found\nERROR: process failed\n",
		buildErr: errors.New("synthetic Docker build failure"),
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	code := command.RunContext(context.Background(), []string{"runtime", "build"})
	if code != ExitRejected {
		t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("runtime build failure stdout = %q", stdout.String())
	}
	for _, retained := range []string{
		"Building runtime for context \"default\"...",
		"RUN gh --version",
		"/bin/sh: gh: not found",
		"× Runtime build failed",
		"Failed step:\n  RUN gh --version",
		"Error:\n  /bin/sh: gh: not found",
		"/config/contexts/default/runtime/Dockerfile",
		"The previously selected runtime is unchanged.",
		"Docker build cache may contain intermediate layers",
		"tobari runtime build",
	} {
		if !strings.Contains(stderr.String(), retained) {
			t.Fatalf("runtime build stderr = %q, missing %q", stderr.String(), retained)
		}
	}
	if strings.Contains(stderr.String(), "\x1b[") {
		t.Fatalf("non-TTY runtime build stderr contains ANSI: %q", stderr.String())
	}
}

func TestRuntimeBuildDiagnosticStreamProjectsTerminalControls(t *testing.T) {
	fake := &contextCLI{
		report:   contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided),
		buildLog: "RUN tool\\literal\tvalue\x1b[31m\u202etest\nERROR: tool not found\n",
		buildErr: errors.New("synthetic Docker build failure"),
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitRejected {
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

func TestRuntimeCommandsUseTheActiveContextWithoutAName(t *testing.T) {
	fake := &contextCLI{report: runtimeInitReportFixture()}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	if code := command.RunContext(context.Background(), []string{"runtime", "init", "--format", "json"}); code != ExitOK {
		t.Fatalf("runtime init code = %d, stderr = %q", code, stderr.String())
	}
	var initDocument struct {
		SchemaVersion int                  `json:"schema_version"`
		Context       tobari.ContextReport `json:"context"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &initDocument); err != nil {
		t.Fatalf("runtime init JSON = %q, error = %v", stdout.String(), err)
	}
	if initDocument.SchemaVersion != 7 || initDocument.Context.Task != tobari.TaskRuntimeInit {
		t.Fatalf("runtime init document = %+v", initDocument)
	}
	for _, retained := range []string{
		"/config/contexts/default/policy",
		"/config/contexts/default/credentials.json",
		"/config/contexts/default/credentials",
		"sha256:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("b", 64),
	} {
		if !strings.Contains(stdout.String(), retained) {
			t.Fatalf("runtime init JSON = %q, missing retained diagnostic %q", stdout.String(), retained)
		}
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "init"}); code != ExitOK {
		t.Fatalf("runtime init text code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime Dockerfile created") ||
		!strings.Contains(stdout.String(), "tobari runtime build") {
		t.Fatalf("runtime init text output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitOK {
		t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime") ||
		!strings.Contains(stdout.String(), "existing Workspaces keep their home") ||
		!strings.Contains(stdout.String(), "Next: run `tobari` from a project directory.") {
		t.Fatalf("runtime build output = %q", stdout.String())
	}
}

func runtimeInitReportFixture() tobari.ContextReport {
	return tobari.ContextReport{
		Task:             tobari.TaskRuntimeInit,
		ID:               "018bcfe5-687b-7000-8000-000000000099",
		Name:             "default",
		Active:           true,
		AgentProfile:     tobari.DefaultProfile,
		Image:            tobari.OfficialRuntimeBase,
		PolicyMode:       tobari.ContextPolicyModeGuided,
		ShellEnvironment: tobari.DefaultContextShellEnvironmentReport(),
		GitIdentity:      tobari.DefaultContextGitIdentityReport(),
		Stores: tobari.ContextStorePaths{
			PolicyDirectory:     "/config/contexts/default/policy",
			CredentialConfig:    "/config/contexts/default/credentials.json",
			CredentialDirectory: "/config/contexts/default/credentials",
		},
		Runtime: tobari.ContextRuntimeReport{
			Kind:          tobari.ContextRuntimeKindDockerfile,
			Status:        tobari.ContextRuntimeStatusPendingBuild,
			Dockerfile:    "/config/contexts/default/runtime/Dockerfile",
			BaseReference: tobari.OfficialRuntimeBase,
			SourceDigest:  "sha256:" + strings.Repeat("a", 64),
			ImageDigest:   "sha256:" + strings.Repeat("b", 64),
		},
		Cluster:        tobari.ContextClusterStatusNotApplicable,
		Authentication: tobari.ContextAuthentication{BrokerState: tobari.ContextAuthBrokerNotApplicable},
	}
}

func TestRuntimeInitTextSnapshotPrioritizesNextActions(t *testing.T) {
	output, err := renderContextReport(runtimeInitReportFixture(), successFormatText, false)
	if err != nil {
		t.Fatalf("renderContextReport() error = %v", err)
	}
	want := "✓ Runtime Dockerfile created\n\n" +
		"Next\n" +
		"  1. Edit the Dockerfile\n" +
		"     /config/contexts/default/runtime/Dockerfile\n\n" +
		"  2. Build the runtime\n" +
		"     tobari runtime build\n\n" +
		"Details\n" +
		"  Context        default\n" +
		"  Base image     ghcr.io/tasuku43/tobari/runtime:latest\n" +
		"  Status         pending_build\n"
	if got := string(output); got != want {
		t.Fatalf("runtime init text = %q, want snapshot %q", got, want)
	}
	if strings.Index(string(output), "Next\n") > strings.Index(string(output), "Details\n") {
		t.Fatalf("Next section was rendered after Details: %q", output)
	}
	for _, omitted := range []string{
		"Agent profile:", "Policy mode:", "Runtime source digest:",
		"Runtime image digest:", "Policy:", "Credential metadata:", "Credential directory:",
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

func TestContextShowRetainsRuntimeAndStoreDiagnostics(t *testing.T) {
	fixture := runtimeInitReportFixture()
	fixture.Task = tobari.TaskContextShow
	fixture.Authentication = tobari.ContextAuthentication{
		BrokerState: tobari.ContextAuthBrokerReady,
		Providers: []tobari.ContextAuthProvider{{
			Provider: "github", State: tobari.ContextAuthProviderNotConfigured,
		}},
	}
	fake := &contextCLI{report: fixture}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)

	if code := command.RunContext(context.Background(), []string{"context", "show"}); code != ExitOK {
		t.Fatalf("context show code = %d, stderr = %q", code, stderr.String())
	}
	for _, retained := range []string{
		"/config/contexts/default/policy",
		"/config/contexts/default/credentials.json",
		"/config/contexts/default/credentials",
		"sha256:" + strings.Repeat("a", 64),
		"sha256:" + strings.Repeat("b", 64),
	} {
		if !strings.Contains(stdout.String(), retained) {
			t.Fatalf("context show output = %q, missing retained diagnostic %q", stdout.String(), retained)
		}
	}
}
