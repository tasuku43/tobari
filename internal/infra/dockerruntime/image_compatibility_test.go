package dockerruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestClusterUpRejectsPublishedResolverAPIMismatchBeforeRuntimeCalls(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	runtime, err := newRuntimeWithData(t.TempDir()+"/config", t.TempDir()+"/state", t.TempDir()+"/data", runner)
	if err != nil {
		t.Fatal(err)
	}
	identity := buildidentity.Identity{
		Version: "dev", Commit: buildidentity.UnknownCommit,
		ResolverChannel:   buildidentity.ResolverPublished,
		CapabilityProfile: capabilityprofile.ProfileStandard,
		Gateway:           buildidentity.Component{RequiredAPI: 1, SelectedAPI: 2},
	}
	runtime.images = testImageResolver{identity: &identity}
	var progress []tobari.ClusterUpProgress
	_, err = runtime.ClusterUpWithProgress(context.Background(), func(event tobari.ClusterUpProgress) {
		progress = append(progress, event)
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "runtime_image_api_mismatch" || public.Retryable {
		t.Fatalf("fault = %#v, error = %v", public, err)
	}
	if !strings.Contains(public.Message, "Gateway API 2") ||
		!strings.Contains(public.Message, "source requires Gateway API 1") ||
		strings.Contains(public.Message, "task build") || strings.Contains(public.Message, "bin/tobari") {
		t.Fatalf("published mismatch message = %q", public.Message)
	}
	if len(progress) != 0 || len(runner.runs) != 0 || len(runner.outputs) != 0 {
		t.Fatalf("mismatch crossed preflight: progress=%+v runs=%+v outputs=%+v", progress, runner.runs, runner.outputs)
	}
}

func TestComponentAPIMismatchRecoveryIsChannelSpecific(t *testing.T) {
	t.Parallel()
	development := buildidentity.Identity{
		Version: "dev", Commit: buildidentity.UnknownCommit,
		ResolverChannel: buildidentity.ResolverDevelopment, DevelopmentSource: true,
		CapabilityProfile: capabilityprofile.ProfileStandard,
		Gateway:           buildidentity.Component{RequiredAPI: 1, SelectedAPI: 1},
	}
	runtime := &Runtime{images: testImageResolver{identity: &development}}
	public, ok := fault.PublicCopy(runtime.incompatibleComponentAPI("Gateway", 2, 1, "gateway_image_incompatible"))
	if !ok || !strings.Contains(public.Message, "task build") || !strings.Contains(public.Message, "bin/tobari cluster up") {
		t.Fatalf("development recovery = %#v", public)
	}

	published := development
	published.ResolverChannel = buildidentity.ResolverPublished
	published.DevelopmentSource = false
	runtime.images = testImageResolver{identity: &published}
	public, ok = fault.PublicCopy(runtime.incompatibleComponentAPI("Gateway", 2, 1, "gateway_image_incompatible"))
	if !ok || strings.Contains(public.Message, "task build") || strings.Contains(public.Message, "bin/tobari") {
		t.Fatalf("published recovery leaked repository commands = %#v", public)
	}
}
