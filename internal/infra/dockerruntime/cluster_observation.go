package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// InspectCluster observes exact shared container state.
func (r *Runtime) InspectCluster(ctx context.Context, state tobari.State) (tobari.ClusterStatus, error) {
	if err := state.Validate(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	if err := r.requireNoInterruptedClusterReconcile(ctx); err != nil {
		return tobari.ClusterStatus{}, err
	}
	if _, err := r.runner.Output(ctx, []string{"version", "--format", "{{.Server.Version}}"}, os.Environ()); err != nil {
		return tobari.ClusterStatus{}, fmt.Errorf("Docker Engine is unavailable: %w", err)
	}
	components := make([]tobari.ComponentStatus, 0, len(clusterComponentOrder))
	running := true
	for _, name := range clusterComponentOrder {
		component, err := r.inspectContainer(ctx, name, clusterContainers[name])
		if err != nil {
			return tobari.ClusterStatus{}, err
		}
		if component.State != "running" || (component.Health != "healthy" && component.Health != "none") {
			running = false
		}
		components = append(components, component)
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return tobari.ClusterStatus{}, fmt.Errorf("read CWD-owned projects: %w", err)
	}
	policyIntegrity := r.inspectAggregatePolicyIntegrity(ctx, state)
	principalIntegrity := r.inspectPrincipalRegistryIntegrity(projects)
	gatewayIntegrity := r.inspectGatewayProjectionIntegrity(state)
	brokerState, brokerErr := r.brokerState(ctx)
	if brokerErr != nil {
		brokerState = "unavailable"
	}
	if brokerState != "ready" {
		running = false
	}
	companionState, _, companionErr := r.credentialCompanionStatus(ctx)
	if companionErr != nil {
		companionState = "unavailable"
	}
	if companionState != "ready" {
		running = false
	}
	backend := "unavailable"
	if selected, backendErr := authStorageBackend(); backendErr == nil {
		backend = string(selected)
	}
	return tobari.ClusterStatus{
		Configured: true, Running: running,
		Policy: state.PolicyDirectory, TobariCount: len(projects), ContextCount: state.ContextCount,
		PolicyRevision: state.AggregateRevision, PolicyProjection: policyIntegrity,
		PrincipalRegistry: principalIntegrity, GatewayProjection: gatewayIntegrity,
		AuthProviderProjection: r.inspectAuthProviderProjectionIntegrity(),
		AuthBrokerState:        string(brokerState), CredentialCompanionState: companionState,
		RootKeyBackend: backend,
		Components:     components, RecentError: state.RecentError,
	}, nil
}

func (r *Runtime) inspectAggregatePolicyIntegrity(ctx context.Context, state tobari.State) string {
	contexts, err := r.readAggregateContexts(ctx)
	if err != nil || len(contexts) != state.ContextCount {
		return "invalid"
	}
	if err := requirePrivateDirectory(state.PolicyDirectory); err != nil {
		return "invalid"
	}
	if _, err := readOwnerPolicyFile(filepath.Join(state.PolicyDirectory, "router.rego"), maxPolicyPreflight); err != nil {
		return "invalid"
	}
	data, err := readOwnerPolicyFile(filepath.Join(state.PolicyDirectory, "data.json"), maxPolicyPreflight)
	if err != nil || validateNoDuplicateJSONKeys(data) != nil {
		return "invalid"
	}
	var document struct {
		Contexts map[string]json.RawMessage `json:"tobari_contexts"`
	}
	if err := json.Unmarshal(data, &document); err != nil || len(document.Contexts) != len(contexts) {
		return "invalid"
	}
	for _, item := range contexts {
		if _, exists := document.Contexts[item.manifest.ID]; !exists {
			return "invalid"
		}
	}
	return "valid"
}

func (r *Runtime) inspectPrincipalRegistryIntegrity(projects []tobari.ProjectInstance) string {
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return "invalid"
	}
	byID := make(map[string]tobari.ProjectInstance, len(projects))
	for _, project := range projects {
		byID[project.ID] = project
	}
	for _, binding := range registry.Bindings {
		project, exists := byID[binding.ProjectID]
		if !exists || project.ContextID != binding.ContextID || project.ContextName != binding.ContextName || project.Root != binding.ProjectRoot {
			return "invalid"
		}
		_, network, resourceErr := tobari.ProjectResourceNames(project.ID)
		if resourceErr != nil || network != binding.Network {
			return "invalid"
		}
	}
	return "valid"
}

func (r *Runtime) inspectGatewayProjectionIntegrity(state tobari.State) string {
	if _, status := r.checkGatewayConfigAt(state.GatewayConfig); status != doctor.CheckStatusPass {
		return "invalid"
	}
	return "valid"
}

func (r *Runtime) inspectContainer(ctx context.Context, component, container string) (tobari.ComponentStatus, error) {
	output, err := r.runner.Output(
		ctx,
		[]string{
			"inspect", "--format",
			`{"state":"{{.State.Status}}","health":"{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}"}`,
			container,
		},
		os.Environ(),
	)
	status := tobari.ComponentStatus{Name: component, State: "absent", Health: "none"}
	if err != nil {
		return status, nil
	}
	var observed struct {
		State  string `json:"state"`
		Health string `json:"health"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &observed); err != nil {
		return tobari.ComponentStatus{}, fmt.Errorf("decode Docker status for %s: %w", component, err)
	}
	status.State, status.Health = observed.State, observed.Health
	return status, nil
}

func (r *Runtime) ClusterLogs(ctx context.Context, state tobari.State, request tobari.LogRequest) ([]byte, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	if err := request.ValidateCluster(); err != nil {
		return nil, err
	}
	names := []string{request.Component}
	if request.Component == "all" {
		names = clusterComponentOrder
	}
	var output bytes.Buffer
	for _, name := range names {
		data, err := r.runner.Output(
			ctx, []string{"logs", "--tail", strconv.Itoa(request.Tail), clusterContainers[name]}, os.Environ(),
		)
		if err != nil {
			return nil, fmt.Errorf("read %s logs: %w", name, err)
		}
		fmt.Fprintf(&output, "== %s ==\n", name)
		_, _ = output.Write(data)
		if len(data) == 0 || data[len(data)-1] != '\n' {
			_ = output.WriteByte('\n')
		}
		if output.Len() > maxLogBytes {
			return nil, fmt.Errorf("log output exceeds %d bytes", maxLogBytes)
		}
	}
	return output.Bytes(), nil
}

// ClusterDown removes exact shared resources after application-level emptiness validation.
func (r *Runtime) ClusterDown(ctx context.Context, state tobari.State, purge bool) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := r.startClusterReconcile(clusterOperationDown); err != nil {
		return fmt.Errorf("start cluster reconcile journal: %w", err)
	}
	for _, container := range clusterContainers {
		if err := r.verifyOwned(ctx, "container", container); err != nil && !errors.Is(err, errOwnedResourceMissing) {
			return err
		}
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	err = r.runner.Run(
		ctx,
		[]string{
			"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"down", "--remove-orphans",
		},
		environment, nil, &output, &output,
	)
	if err != nil {
		_ = r.recordRecentError(state, "Cluster cleanup did not complete; inspect component logs.")
		return fmt.Errorf("docker compose down: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if err := r.waitForCredentialCompanionStopped(ctx); err != nil {
		return err
	}
	if err := r.replaceProjectPrincipalRegistry(ctx, []projectPrincipalBinding{}); err != nil {
		return fmt.Errorf("clear project principal registry: %w", err)
	}
	if purge {
		for _, volume := range []string{"tobari-gateway-ca", "tobari-public-ca", policyBundleVolume} {
			if err := r.verifyOwned(ctx, "volume", volume); errors.Is(err, errOwnedResourceMissing) {
				continue
			} else if err != nil {
				return err
			}
			if output, err := r.runner.Output(ctx, []string{"volume", "rm", volume}, os.Environ()); err != nil {
				return fmt.Errorf("remove owned volume %s: %w: %s", volume, err, boundedDiagnostic(output))
			}
		}
	}
	if err := r.markClusterRuntimeReconciled(clusterOperationDown); err != nil {
		return fmt.Errorf("mark cluster reconcile complete: %w", err)
	}
	if err := r.clearClusterJournal(); err != nil {
		return fmt.Errorf("clear cluster reconcile journal: %w", err)
	}
	if err := os.Remove(r.statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove Tobari state: %w", err)
	}
	return nil
}

func DefaultLogTail() int { return defaultLogTail }
