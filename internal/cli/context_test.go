package cli

import (
	"bytes"
	"context"
	"encoding/json"
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
	f.report.Task = tobari.TaskContextUse
	f.report.Active = true
	return f.report, nil
}

func (f *contextCLI) InitRuntime(context.Context) (tobari.ContextReport, error) {
	f.report.Task = tobari.TaskRuntimeInit
	return f.report, nil
}

func (f *contextCLI) BuildRuntime(context.Context) (tobari.ContextReport, error) {
	f.report.Task = tobari.TaskRuntimeBuild
	return f.report, nil
}

type fakeContextRuntime struct {
	list   tobari.ContextListResult
	report tobari.ContextReport
}

func contextCLIReport(task, name string, active bool, image string, mode tobari.ContextPolicyMode) tobari.ContextReport {
	return tobari.ContextReport{
		Task: task, Name: name, Active: active, AgentProfile: tobari.DefaultProfile,
		Image: image, PolicyMode: mode,
		Runtime: tobari.ContextRuntimeReport{Kind: tobari.ContextRuntimeKindOfficial, Status: tobari.ContextRuntimeStatusOfficial},
		Stores: tobari.ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials"),
		},
	}
}

func TestContextCommandsRenderActiveContextAndRuntimeImage(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, "builtin", tobari.ContextPolicyModeGuided)}
	fake.list = tobari.ContextListResult{
		Task: tobari.TaskContextList, Active: "default",
		Items: []tobari.ContextSummary{{Name: "default", Active: true, AgentProfile: tobari.DefaultProfile, Image: "builtin", PolicyMode: tobari.ContextPolicyModeGuided}},
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.context = contextcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"context", "list"}); code != ExitOK {
		t.Fatalf("context list code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Active Context: default") || !strings.Contains(stdout.String(), "image=builtin") {
		t.Fatalf("context list output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"context", "create", "--name", "project-tools", "--image", "tobari-runtime:local", "--mode", "advanced", "--format", "json"}); code != ExitOK {
		t.Fatalf("context create code = %d, stderr = %q", code, stderr.String())
	}
	var document struct {
		Context tobari.ContextReport `json:"context"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("context create JSON = %q, error = %v", stdout.String(), err)
	}
	if document.Context.Name != "project-tools" || document.Context.Image != "tobari-runtime:local" || document.Context.PolicyMode != tobari.ContextPolicyModeAdvanced {
		t.Fatalf("context create document = %+v", document.Context)
	}
}

func TestRuntimeCommandsUseTheActiveContextWithoutAName(t *testing.T) {
	fake := &contextCLI{report: contextCLIReport(tobari.TaskContextShow, "default", true, "builtin", tobari.ContextPolicyModeGuided)}
	fake.report.Runtime = tobari.ContextRuntimeReport{
		Kind: tobari.ContextRuntimeKindDockerfile, Status: tobari.ContextRuntimeStatusPendingBuild,
		Dockerfile: "/config/contexts/default/runtime/Dockerfile",
	}
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

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "init"}); code != ExitOK {
		t.Fatalf("runtime init text code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Next: edit /config/contexts/default/runtime/Dockerfile, then run `tobari runtime build`.") {
		t.Fatalf("runtime init text output = %q", stdout.String())
	}

	stdout.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "build"}); code != ExitOK {
		t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Runtime") || !strings.Contains(stdout.String(), "Next: run `tobari` from a project directory.") {
		t.Fatalf("runtime build output = %q", stdout.String())
	}
}
