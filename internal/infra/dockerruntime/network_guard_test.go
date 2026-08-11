package dockerruntime

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

func TestNetworkGuardHelperUsesOneFixedCapabilityOnly(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte("tobari-network-guard v1 workspace\n")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	imageID := "sha256:" + strings.Repeat("a", 64)
	if err := runtime.runNetworkGuardHelper(
		context.Background(), imageID, "tobari-work", "workspace", "172.29.0.2", "172.29.0.0/24",
	); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"run", "--rm", "--network", "container:tobari-work", "--user", "0:0",
		"--read-only", "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
		"--security-opt", "no-new-privileges:true", "--entrypoint", networkGuardEntrypoint,
		imageID, "workspace", "172.29.0.2", "172.29.0.0/24",
	}
	if len(runner.outputs) != 1 || !slices.Equal(runner.outputs[0].args, want) {
		t.Fatalf("network guard argv = %v, want %v", runner.outputs, want)
	}
	joined := strings.Join(want, " ")
	for _, forbidden := range []string{"--privileged", "--mount", "--volume", "/var/run/docker.sock", "--pid", "--ipc"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("network guard argv contains %q: %v", forbidden, want)
		}
	}
}

func TestNetworkGuardHelperRejectsUnverifiedCompletion(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte("completed\n")}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.runNetworkGuardHelper(
		context.Background(), "sha256:"+strings.Repeat("b", 64), gatewayContainer, "gateway",
	)
	if err == nil || !strings.Contains(err.Error(), "verification output") {
		t.Fatalf("runNetworkGuardHelper() error = %v", err)
	}
}

func TestProjectNetworkEndpointValidationIsExact(t *testing.T) {
	t.Parallel()
	if err := validateProjectNetworkEndpoints("172.29.0.0/24", "172.29.0.3", "172.29.0.2"); err != nil {
		t.Fatal(err)
	}
	for _, endpoints := range [][3]string{
		{"172.29.0.1/24", "172.29.0.3", "172.29.0.2"},
		{"172.29.0.0/24", "172.29.1.3", "172.29.0.2"},
		{"172.29.0.0/24", "172.29.0.2", "172.29.0.2"},
		{"172.29.0.0/24", "not-an-address", "172.29.0.2"},
	} {
		if err := validateProjectNetworkEndpoints(endpoints[0], endpoints[1], endpoints[2]); err == nil {
			t.Fatalf("validateProjectNetworkEndpoints%v accepted invalid state", endpoints)
		}
	}
}

func TestGatewayGuardFailureHasStableDoctorRecovery(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputErr: context.DeadlineExceeded}
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	err = runtime.ensureGatewayNetworkGuard(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "network_guard_failed" || public.Retryable || len(public.NextActions) != 1 || public.NextActions[0].Command != "doctor" {
		t.Fatalf("network guard fault = %#v, error = %v", public, err)
	}
}
