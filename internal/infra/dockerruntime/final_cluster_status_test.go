package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterStatusRunner struct{ unknown bool }

func (r finalClusterStatusRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}
func (r finalClusterStatusRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if r.unknown {
		return []byte("Docker unavailable"), errors.New("Docker unavailable")
	}
	name := "resource"
	if len(args) != 0 {
		name = args[len(args)-1]
	}
	return []byte("Error: No such object: " + name), errors.New("No such object")
}

func newFinalClusterStatusRuntime(t *testing.T, runner commandRunner) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestFinalClusterStatusFreshAbsenceAndSurfaceSelectedComponents(t *testing.T) {
	runtime := newFinalClusterStatusRuntime(t, finalClusterStatusRunner{})
	status, err := runtime.ObserveFinalCluster(context.Background(), tobari.WorkspaceAuthorityCollection{}, false)
	if err != nil {
		t.Fatal(err)
	}
	wantComponents := 2
	if brokerRuntimeEnabled {
		wantComponents = 4
	}
	if status.Runtime != tobari.FinalClusterRuntimeAbsent || status.Receipt != tobari.FinalClusterReceiptAbsent || len(status.Components) != wantComponents {
		t.Fatalf("fresh status=%#v", status)
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
	if status.Components[0].Name != "gateway" || status.Components[1].Name != "opa" {
		t.Fatalf("surface order=%#v", status.Components)
	}
	if !brokerRuntimeEnabled && (strings.Contains(status.Components[0].Name, "broker") || len(status.Components) != 2) {
		t.Fatalf("release exposed research surface: %#v", status.Components)
	}
	if brokerRuntimeEnabled && (status.Components[2].Name != "auth-broker" || status.Components[3].Name != "credential-companion") {
		t.Fatalf("research surface=%#v", status.Components)
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"container_id", "image_id", "address", "tobari-control", "tobari-egress"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public status leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestFinalClusterComponentStatusDoesNotExportPrivateRuntimeIdentity(t *testing.T) {
	privateContainerID := strings.Repeat("a", 64)
	privateImageID := "sha256:" + strings.Repeat("b", 64)
	privateAddress := "172.31.8.9"
	row := finalComponentStatus("gateway", appliedClusterComponentObservation{
		ContainerID: privateContainerID,
		Owner:       ownerValue,
		Component:   "gateway",
		Role:        gatewayRole,
		ImageID:     privateImageID,
		State:       "running",
		Health:      "healthy",
		NetworkAddresses: map[string]string{
			"tobari-control": privateAddress,
		},
	}, false, nil)
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"container_id", "image_id", "network_addresses", privateContainerID, privateImageID, privateAddress, "tobari-control"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public component leaked private runtime value %q: %s", forbidden, encoded)
		}
	}
}

func TestFinalClusterStatusKeepsDockerUncertaintyUnknown(t *testing.T) {
	runtime := newFinalClusterStatusRuntime(t, finalClusterStatusRunner{unknown: true})
	status, err := runtime.ObserveFinalCluster(context.Background(), tobari.WorkspaceAuthorityCollection{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if status.Runtime != tobari.FinalClusterRuntimeUnknown {
		t.Fatalf("unknown Docker status=%#v", status)
	}
}

func TestFinalClusterStatusReportsInterruptedJournalWithoutRepair(t *testing.T) {
	runtime := newFinalClusterStatusRuntime(t, finalClusterStatusRunner{})
	if err := os.MkdirAll(runtime.finalGatewaySettlementRoot(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(runtime.finalClusterBootstrapJournalPath(), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runtime.finalClusterBootstrapJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	status, err := runtime.ObserveFinalCluster(context.Background(), tobari.WorkspaceAuthorityCollection{}, false)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(runtime.finalClusterBootstrapJournalPath())
	if err != nil {
		t.Fatal(err)
	}
	if status.Runtime != tobari.FinalClusterRuntimeUnknown || status.Receipt != tobari.FinalClusterReceiptUnknown || string(before) != string(after) {
		t.Fatalf("interrupted status=%#v mutation=%t", status, string(before) != string(after))
	}
}
