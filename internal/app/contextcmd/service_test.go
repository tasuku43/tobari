package contextcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextRuntimeFake struct {
	listResult         tobari.ContextListResult
	showResult         tobari.ContextReport
	createResult       tobari.ContextReport
	useResult          tobari.ContextReport
	initResult         tobari.ContextReport
	buildResult        tobari.ContextReport
	createErr          error
	useErr             error
	initErr            error
	buildErr           error
	createCalls        int
	useCalls           int
	initCalls          int
	buildCalls         int
	buildProgressCalls int
	lastName           string
	lastImage          string
	lastMode           tobari.ContextPolicyMode
}

func (f *contextRuntimeFake) ListContexts(context.Context) (tobari.ContextListResult, error) {
	return f.listResult, nil
}

func (f *contextRuntimeFake) ShowContext(context.Context, string) (tobari.ContextReport, error) {
	return f.showResult, nil
}

func (f *contextRuntimeFake) CreateContext(
	_ context.Context, name, image string, mode tobari.ContextPolicyMode,
) (tobari.ContextReport, error) {
	f.createCalls++
	f.lastName, f.lastImage, f.lastMode = name, image, mode
	return f.createResult, f.createErr
}

func (f *contextRuntimeFake) UseContext(context.Context, string) (tobari.ContextReport, error) {
	f.useCalls++
	return f.useResult, f.useErr
}

func (f *contextRuntimeFake) InitRuntime(context.Context) (tobari.ContextReport, error) {
	f.initCalls++
	return f.initResult, f.initErr
}

func (f *contextRuntimeFake) BuildRuntime(context.Context) (tobari.ContextReport, error) {
	f.buildCalls++
	return f.buildResult, f.buildErr
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
	return tobari.ContextReport{
		Task: task, ID: "018bcfe5-687b-7000-8000-000000000099", Name: name, Active: task == tobari.TaskContextUse,
		AgentProfile: tobari.DefaultProfile, Image: tobari.OfficialRuntimeBase,
		PolicyMode: tobari.ContextPolicyModeGuided,
		Cluster:    tobari.ContextClusterStatusNotApplicable,
		Stores: tobari.ContextStorePaths{
			PolicyDirectory:     filepath.Join(string(filepath.Separator), "config", "contexts", name, "policy"),
			CredentialConfig:    filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials.json"),
			CredentialDirectory: filepath.Join(string(filepath.Separator), "config", "contexts", name, "credentials"),
		},
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
	fake := &contextRuntimeFake{createResult: contextReport(tobari.TaskContextCreate, "project-tools")}
	service := New(fake)
	intent := operation.Intent{
		Command: "context create", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		Impact: contextImpact(),
	}
	result, err := service.Create(context.Background(), intent, "project-tools", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeAdvanced)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if result.Name != "project-tools" || fake.createCalls != 1 || fake.lastName != "project-tools" ||
		fake.lastImage != tobari.OfficialRuntimeBase || fake.lastMode != tobari.ContextPolicyModeAdvanced {
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
	_, err := service.Create(context.Background(), intent, "project-tools", "--pull=always", tobari.ContextPolicyModeGuided)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_context" {
		t.Fatalf("Create() fault = %#v, ok=%t", public, ok)
	}
	if fake.createCalls != 0 {
		t.Fatalf("CreateContext() calls = %d, want 0", fake.createCalls)
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
