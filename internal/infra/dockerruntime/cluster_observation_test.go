package dockerruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type clusterNetworkObservationRunner struct {
	networks map[string]map[string]string
}

func (r *clusterNetworkObservationRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return nil
}

func (r *clusterNetworkObservationRunner) Output(
	_ context.Context, arguments, _ []string,
) ([]byte, error) {
	if len(arguments) == 4 && arguments[0] == "inspect" &&
		arguments[1] == "--format" && arguments[2] == "{{json .NetworkSettings.Networks}}" {
		observed, exists := r.networks[arguments[3]]
		if !exists {
			return nil, fmt.Errorf("No such container: %s", arguments[3])
		}
		encoded := make(map[string]map[string]string, len(observed))
		for network, address := range observed {
			encoded[network] = map[string]string{"IPAddress": address}
		}
		return json.Marshal(encoded)
	}
	return nil, fmt.Errorf("unexpected Docker observation: %v", arguments)
}

func healthySharedClusterNetworks() map[string]map[string]string {
	networks := map[string]map[string]string{
		gatewayContainer: {
			"tobari-control": "172.22.0.2",
			"tobari-egress":  "172.23.0.2",
		},
		opaContainer: {
			"tobari-control": "172.22.0.3",
		},
	}
	if brokerRuntimeEnabled {
		networks[authBrokerContainer] = map[string]string{
			"tobari-control": "172.22.0.4",
			"tobari-egress":  "172.23.0.4",
		}
	}
	return networks
}

func cloneClusterNetworks(source map[string]map[string]string) map[string]map[string]string {
	cloned := make(map[string]map[string]string, len(source))
	for container, networks := range source {
		cloned[container] = make(map[string]string, len(networks))
		for network, address := range networks {
			cloned[container][network] = address
		}
	}
	return cloned
}

func TestSharedClusterNetworkIntegrityRejectsDisconnectedComponents(t *testing.T) {
	t.Parallel()
	healthy := healthySharedClusterNetworks()
	runtime := &Runtime{runner: &clusterNetworkObservationRunner{networks: healthy}}
	if got := runtime.inspectSharedClusterNetworkIntegrity(context.Background()); got != "valid" {
		t.Fatalf("healthy shared network integrity = %q, want valid", got)
	}

	tests := []struct {
		name      string
		container string
		network   string
	}{
		{name: "Gateway control", container: gatewayContainer, network: "tobari-control"},
		{name: "Gateway egress", container: gatewayContainer, network: "tobari-egress"},
		{name: "OPA control", container: opaContainer, network: "tobari-control"},
	}
	if brokerRuntimeEnabled {
		tests = append(tests,
			struct {
				name      string
				container string
				network   string
			}{name: "Auth Broker control", container: authBrokerContainer, network: "tobari-control"},
			struct {
				name      string
				container string
				network   string
			}{name: "Auth Broker egress", container: authBrokerContainer, network: "tobari-egress"},
		)
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drifted := cloneClusterNetworks(healthy)
			delete(drifted[test.container], test.network)
			runtime := &Runtime{runner: &clusterNetworkObservationRunner{networks: drifted}}
			if got := runtime.inspectSharedClusterNetworkIntegrity(context.Background()); got != "invalid" {
				t.Fatalf("integrity after disconnecting %s from %s = %q, want invalid", test.container, test.network, got)
			}
		})
	}
}

func TestSharedClusterNetworkIntegrityUsesOnlyReadOnlyDockerInspect(t *testing.T) {
	t.Parallel()
	runner := &clusterNetworkObservationRunner{networks: healthySharedClusterNetworks()}
	runtime := &Runtime{runner: runner}
	if got := runtime.inspectSharedClusterNetworkIntegrity(context.Background()); got != "valid" {
		t.Fatalf("shared network integrity = %q, want valid", got)
	}
	for container := range runner.networks {
		if !slices.Contains([]string{gatewayContainer, opaContainer, authBrokerContainer}, container) {
			t.Fatalf("unexpected shared container observation %q", container)
		}
	}
}

type projectPrincipalObservationRunner struct {
	project            tobari.ProjectInstance
	workspaceAddress   string
	gatewayAddress     string
	gatewayConnected   bool
	workspaceConnected bool
}

func (r *projectPrincipalObservationRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	return fmt.Errorf("project principal observation attempted a Docker mutation")
}

func (r *projectPrincipalObservationRunner) Output(
	_ context.Context, arguments, _ []string,
) ([]byte, error) {
	container, network, err := tobari.ProjectResourceNames(r.project.ID)
	if err != nil {
		return nil, err
	}
	joined := strings.Join(arguments, " ")
	switch {
	case len(arguments) == 5 && arguments[0] == "network" && arguments[1] == "inspect" &&
		arguments[2] == "--format" && strings.Contains(arguments[3], ".IPAM.Config") && arguments[4] == network:
		return []byte(`[{"Subnet":"172.29.0.0/16"}]`), nil
	case len(arguments) == 5 && arguments[0] == "network" && arguments[1] == "inspect" && arguments[4] == network:
		switch {
		case strings.Contains(joined, ownerLabel):
			return []byte(ownerValue), nil
		case strings.Contains(joined, projectIDLabel):
			return []byte(r.project.ID), nil
		case strings.Contains(joined, projectRoleLabel):
			return []byte(projectNetRole), nil
		default:
			return []byte("network-id"), nil
		}
	case len(arguments) == 4 && arguments[0] == "inspect" && arguments[3] == container:
		switch {
		case strings.Contains(joined, ".State.Status"):
			return []byte(`{"state":"running","health":"healthy"}`), nil
		case strings.Contains(joined, ".NetworkSettings.Networks"):
			networks := map[string]map[string]string{}
			if r.workspaceConnected {
				networks[network] = map[string]string{"IPAddress": r.workspaceAddress}
			}
			return json.Marshal(networks)
		case strings.Contains(joined, ownerLabel):
			return []byte(ownerValue), nil
		case strings.Contains(joined, projectIDLabel):
			return []byte(r.project.ID), nil
		case strings.Contains(joined, projectRoleLabel):
			return []byte(projectWorkRole), nil
		default:
			return []byte("container-id"), nil
		}
	case len(arguments) == 4 && arguments[0] == "inspect" && arguments[3] == gatewayContainer &&
		strings.Contains(joined, ".NetworkSettings.Networks"):
		networks := map[string]map[string]string{}
		if r.gatewayConnected {
			networks[network] = map[string]string{"IPAddress": r.gatewayAddress}
		}
		return json.Marshal(networks)
	default:
		return nil, fmt.Errorf("unexpected Docker observation: %v", arguments)
	}
}

func TestPrincipalRegistryIntegrityTracksLiveOwnedEndpoints(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	project := principalTestProject(t, projectRoot)
	_, network, err := tobari.ProjectResourceNames(project.ID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &projectPrincipalObservationRunner{
		project: project, workspaceAddress: "172.29.0.3", gatewayAddress: "172.29.0.2",
		gatewayConnected: true, workspaceConnected: true,
	}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensurePrivateDirectory(runtime.principalRegistryDirectory()); err != nil {
		t.Fatal(err)
	}
	registry := projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema,
		Bindings: []projectPrincipalBinding{{
			ProjectID: project.ID, ContextID: project.ContextID, ContextName: project.ContextName,
			ProjectRoot: project.Root, WorkspaceIP: runner.workspaceAddress,
			GatewayIP: runner.gatewayAddress, Network: network,
		}},
	}
	if err := runtime.writeProjectPrincipalRegistry(registry); err != nil {
		t.Fatal(err)
	}
	if got := runtime.inspectPrincipalRegistryIntegrity(context.Background(), []tobari.ProjectInstance{project}); got != "valid" {
		t.Fatalf("matching live principal registry = %q, want valid", got)
	}
	runner.gatewayConnected = false
	if got := runtime.inspectPrincipalRegistryIntegrity(context.Background(), []tobari.ProjectInstance{project}); got != "invalid" {
		t.Fatalf("disconnected live principal registry = %q, want invalid", got)
	}
	runner.gatewayConnected = true
	registry.Bindings = []projectPrincipalBinding{}
	if err := runtime.writeProjectPrincipalRegistry(registry); err != nil {
		t.Fatal(err)
	}
	if got := runtime.inspectPrincipalRegistryIntegrity(context.Background(), []tobari.ProjectInstance{project}); got != "invalid" {
		t.Fatalf("missing live principal registry binding = %q, want invalid", got)
	}
}
