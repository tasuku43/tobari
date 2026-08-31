package dockerruntime

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterBindProbeRunner struct {
	args        []string
	err         error
	output      string
	hasDeadline bool
	buffered    int
}

func (r *finalClusterBindProbeRunner) Run(ctx context.Context, args, _ []string, _ io.Reader, _, errorOutput io.Writer) error {
	r.args = append([]string{}, args...)
	_, r.hasDeadline = ctx.Deadline()
	if r.output != "" {
		_, _ = io.WriteString(errorOutput, r.output)
	}
	if bounded, ok := errorOutput.(*boundedBuffer); ok {
		r.buffered = bounded.buffer.Len()
	}
	if r.err != nil && r.output == "" {
		_, _ = io.WriteString(errorOutput, "synthetic bind type mismatch")
	}
	return r.err
}

func (*finalClusterBindProbeRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("bind preflight must not use unbounded Docker output")
}

func TestFinalClusterBindPreflightRejectsInvisibleSourceWithoutNamedMutation(t *testing.T) {
	root := t.TempDir()
	runner := &finalClusterBindProbeRunner{err: errors.New("synthetic Docker bind failure")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := appliedRuntimeState(t, root, tobari.SharedClusterProfileLoopbackTCP)
	state.GatewayConfig = filepath.Join(runtime.stateDirectory, "projection", "gateway.json")
	state.PolicyDirectory = filepath.Join(runtime.stateDirectory, "projection", "policy")
	state.RuntimeDirectory = filepath.Join(runtime.stateDirectory, "runtime", state.AssetVersion)
	if err := state.Validate(); err != nil {
		t.Fatal(err)
	}
	gatewayImage := "sha256:" + strings.Repeat("a", 64)
	err = runtime.preflightFinalClusterBindSources(context.Background(), state, tobari.SharedClusterProfileLoopbackTCP, gatewayImage)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "cluster_resource_conflict" || public.Kind != fault.KindRejected || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("bind preflight fault = %#v, public=%t", public, ok)
	}
	requiredSources := []string{
		"run", "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true", "--entrypoint", "/usr/local/bin/python3", gatewayImage,
		"type=bind,src=" + state.GatewayConfig + ",dst=/run/tobari/config/gateway.json,readonly",
		"type=bind,src=" + state.PolicyDirectory + ",dst=/run/tobari/policy,readonly",
		"type=bind,src=" + runtime.principalRegistryDirectory() + ",dst=/run/tobari/principal-registry,readonly",
		"type=bind,src=" + runtime.hostLoopbackDirectory() + ",dst=/run/tobari/host-loopback,readonly",
		"type=bind,src=" + runtime.interactiveAttachmentDirectory() + ",dst=/run/tobari/interactive-attachments,readonly",
	}
	if brokerRuntimeEnabled {
		requiredSources = append(requiredSources,
			"type=bind,src="+runtime.authProviderProjectionDirectory()+",dst=/run/tobari/auth,readonly",
			"type=bind,src="+runtime.authContextsDirectory()+",dst=/run/tobari/auth-contexts,readonly",
		)
	}
	for _, required := range requiredSources {
		if !slices.Contains(runner.args, required) {
			t.Fatalf("bind preflight argv omits %q: %v", required, runner.args)
		}
	}
	if !runner.hasDeadline {
		t.Fatal("bind preflight Docker probe has no finite deadline")
	}
	for _, forbidden := range []string{"compose", "network create", "volume create"} {
		if strings.Contains(strings.Join(runner.args, " "), forbidden) {
			t.Fatalf("bind preflight performed named mutation %q: %v", forbidden, runner.args)
		}
	}
}

func TestFinalClusterBindPreflightIncludesUnixPermissionSource(t *testing.T) {
	root := t.TempDir()
	runner := &finalClusterBindProbeRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := appliedRuntimeState(t, root, tobari.SharedClusterProfileUnix)
	gatewayImage := "sha256:" + strings.Repeat("a", 64)
	if err := runtime.preflightFinalClusterBindSources(context.Background(), state, tobari.SharedClusterProfileUnix, gatewayImage); err != nil {
		t.Fatal(err)
	}
	want := "type=bind,src=" + runtime.interactiveAttachmentSocketDirectory() + ",dst=/run/tobari/permission-ingestion,readonly"
	if !slices.Contains(runner.args, want) {
		t.Fatalf("Unix bind preflight argv omits %q: %v", want, runner.args)
	}
}

func TestFinalClusterBindPreflightBoundsTimeoutAndDiagnosticOutput(t *testing.T) {
	root := t.TempDir()
	runner := &finalClusterBindProbeRunner{
		err: context.DeadlineExceeded, output: strings.Repeat("x", finalClusterBindPreflightOutput+1024),
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	state := appliedRuntimeState(t, root, tobari.SharedClusterProfileLoopbackTCP)
	gatewayImage := "sha256:" + strings.Repeat("a", 64)
	err = runtime.preflightFinalClusterBindSources(context.Background(), state, tobari.SharedClusterProfileLoopbackTCP, gatewayImage)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "cluster_resource_conflict" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("bounded bind preflight fault=%#v public=%t", public, ok)
	}
	if !runner.hasDeadline || runner.buffered > finalClusterBindPreflightOutput {
		t.Fatalf("bind preflight deadline=%t buffered=%d limit=%d", runner.hasDeadline, runner.buffered, finalClusterBindPreflightOutput)
	}
}
