package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/contextcmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextCLI fakeContextRuntime

func (f *contextCLI) ListContexts(context.Context) (tobari.ContextListResult, error) {
	return f.list, nil
}

func (f *contextCLI) ShowContext(context.Context, string) (tobari.ContextReport, error) {
	return f.report, nil
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
	return f.report, nil
}

func (f *contextCLI) InitRuntime(context.Context) (tobari.ContextReport, error) {
	f.report.Task = tobari.TaskRuntimeInit
	return f.report, nil
}

func (f *contextCLI) BuildRuntime(context.Context) (tobari.ContextReport, error) {
	f.buildCalls++
	f.report.Task = tobari.TaskRuntimeBuild
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
	list       tobari.ContextListResult
	report     tobari.ContextReport
	useCalls   int
	buildCalls int
	buildLog   string
	buildErr   error
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
	return tobari.ContextReport{
		Task: task, Name: name, Active: active, AgentProfile: tobari.DefaultProfile,
		Image: image, PolicyMode: mode, Cluster: tobari.ContextClusterStatusNotApplicable,
		Runtime: tobari.ContextRuntimeReport{Kind: tobari.ContextRuntimeKindOfficial, Status: tobari.ContextRuntimeStatusOfficial},
		Stores: tobari.ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials"),
		},
	}
}

func TestContextCommandsRenderActiveContextAndRuntimeImage(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided)}
	fake.list = tobari.ContextListResult{
		Task: tobari.TaskContextList, Active: "default",
		Items: []tobari.ContextSummary{{Name: "default", Active: true, AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase, PolicyMode: tobari.ContextPolicyModeGuided}},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"context", "list"}); code != ExitOK {
		t.Fatalf("context list code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Active Context: default") || !strings.Contains(stdout.String(), "image="+tobari.OfficialRuntimeBase) {
		t.Fatalf("context list output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "show"}); code != ExitOK {
		t.Fatalf("context show code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime: official (official)") ||
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
	if initDocument.SchemaVersion != 2 || initDocument.Context.Task != tobari.TaskRuntimeInit {
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
		Task:         tobari.TaskRuntimeInit,
		Name:         "default",
		Active:       true,
		AgentProfile: tobari.DefaultProfile,
		Image:        tobari.OfficialRuntimeBase,
		PolicyMode:   tobari.ContextPolicyModeGuided,
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
		Cluster: tobari.ContextClusterStatusNotApplicable,
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
