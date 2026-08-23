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
	principalIntegrity := r.inspectPrincipalRegistryIntegrity(ctx, projects)
	gatewayIntegrity := r.inspectGatewayProjectionIntegrity(ctx, state)
	status := tobari.ClusterStatus{
		Configured: true, Running: running,
		Policy: state.PolicyDirectory, TobariCount: len(projects), ManifestCount: state.ManifestCount,
		PolicyRevision: state.AggregateRevision, PolicyProjection: policyIntegrity,
		PrincipalRegistry: principalIntegrity, GatewayProjection: gatewayIntegrity,
		Components: components, RecentError: state.RecentError,
	}
	if brokerRuntimeEnabled {
		brokerState, brokerErr := r.brokerState(ctx)
		if brokerErr != nil {
			brokerState = "unavailable"
		}
		if brokerState != "ready" {
			status.Running = false
		}
		companionState, _, companionErr := r.credentialCompanionStatus(ctx)
		if companionErr != nil {
			companionState = "unavailable"
		}
		if companionState != "ready" {
			status.Running = false
		}
		backend := "unavailable"
		if selected, backendErr := authStorageBackend(); backendErr == nil {
			backend = string(selected)
		}
		status.AuthProviderProjection = r.inspectAuthProviderProjectionIntegrity()
		status.AuthBrokerState = string(brokerState)
		status.CredentialCompanionState = companionState
		status.RootKeyBackend = backend
	}
	return status, nil
}

func (r *Runtime) inspectAggregatePolicyIntegrity(ctx context.Context, state tobari.State) string {
	contexts, err := r.readAggregateContexts(ctx)
	if err != nil || len(contexts) != state.ManifestCount {
		return "invalid"
	}
	desiredRevision, err := aggregateRevision(contexts)
	if err != nil || desiredRevision != state.AggregateRevision {
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
		Tobari   struct {
			AggregateSchemaVersion int    `json:"aggregate_schema_version"`
			AggregateRevision      string `json:"aggregate_revision"`
		} `json:"tobari"`
	}
	if err := json.Unmarshal(data, &document); err != nil || len(document.Contexts) != len(contexts) ||
		document.Tobari.AggregateSchemaVersion != aggregateSchemaVersion ||
		document.Tobari.AggregateRevision != state.AggregateRevision {
		return "invalid"
	}
	for _, item := range contexts {
		if _, exists := document.Contexts[item.manifest.ID]; !exists {
			return "invalid"
		}
	}
	return "valid"
}

func (r *Runtime) inspectPrincipalRegistryIntegrity(ctx context.Context, projects []tobari.Workspace) string {
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return "invalid"
	}
	byID := make(map[string]tobari.Workspace, len(projects))
	for _, project := range projects {
		byID[project.ID] = project
	}
	bindings := make(map[string]projectPrincipalBinding, len(registry.Bindings))
	for _, binding := range registry.Bindings {
		project, exists := byID[binding.ProjectID]
		if !exists || project.WorkspaceManifestID != binding.WorkspaceManifestID || project.WorkspaceManifestName != binding.WorkspaceManifestName || project.Root != binding.ProjectRoot {
			return "invalid"
		}
		_, network, resourceErr := tobari.ProjectResourceNames(project.ID)
		if resourceErr != nil || network != binding.Network {
			return "invalid"
		}
		bindings[binding.ProjectID] = binding
	}
	for _, project := range projects {
		observed, ready, observeErr := r.observeProjectPrincipalRuntime(ctx, project)
		stored, registered := bindings[project.ID]
		if observeErr != nil || ready != registered {
			return "invalid"
		}
		if ready && observed != stored {
			return "invalid"
		}
	}
	return "valid"
}

func (r *Runtime) observeProjectPrincipalRuntime(
	ctx context.Context, project tobari.Workspace,
) (projectPrincipalBinding, bool, error) {
	if err := project.Validate(); err != nil {
		return projectPrincipalBinding{}, false, err
	}
	if project.Incomplete {
		return projectPrincipalBinding{}, false, nil
	}
	container, network, err := tobari.ProjectResourceNames(project.ID)
	if err != nil {
		return projectPrincipalBinding{}, false, err
	}
	networkExists, err := r.projectResourceExists(ctx, "network", network)
	if err != nil || !networkExists {
		return projectPrincipalBinding{}, false, err
	}
	if err := r.verifyOwnedProjectResource(ctx, "network", network, project.ID, projectNetRole); err != nil {
		return projectPrincipalBinding{}, false, err
	}
	containerExists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil || !containerExists {
		return projectPrincipalBinding{}, false, err
	}
	if err := r.verifyOwnedProjectResource(ctx, "container", container, project.ID, projectWorkRole); err != nil {
		return projectPrincipalBinding{}, false, err
	}
	component, err := r.inspectContainer(ctx, projectWorkRole, container)
	if err != nil {
		return projectPrincipalBinding{}, false, err
	}
	if component.State != "running" || component.Health != "healthy" {
		return projectPrincipalBinding{}, false, nil
	}
	gatewayAddress, gatewayConnected, err := r.containerNetworkAddressIfConnected(ctx, gatewayContainer, network, "Gateway")
	if err != nil {
		return projectPrincipalBinding{}, false, err
	}
	workspaceAddress, workspaceConnected, err := r.containerNetworkAddressIfConnected(ctx, container, network, "Workspace")
	if err != nil {
		return projectPrincipalBinding{}, false, err
	}
	if !gatewayConnected || !workspaceConnected {
		return projectPrincipalBinding{}, false, nil
	}
	subnet, err := r.projectNetworkSubnet(ctx, network)
	if err != nil {
		return projectPrincipalBinding{}, false, err
	}
	if err := validateProjectNetworkEndpoints(subnet, workspaceAddress, gatewayAddress); err != nil {
		return projectPrincipalBinding{}, false, err
	}
	return projectPrincipalBinding{
		ProjectID: project.ID, WorkspaceManifestID: project.WorkspaceManifestID, WorkspaceManifestName: project.WorkspaceManifestName,
		ProjectRoot: project.Root, WorkspaceIP: workspaceAddress, GatewayIP: gatewayAddress, Network: network,
	}, true, nil
}

func (r *Runtime) inspectGatewayProjectionIntegrity(ctx context.Context, state tobari.State) string {
	if _, status := r.checkGatewayConfigAt(state.GatewayConfig); status != doctor.CheckStatusPass {
		return "invalid"
	}
	if r.inspectSharedClusterNetworkIntegrity(ctx) != "valid" {
		return "invalid"
	}
	return "valid"
}

func (r *Runtime) inspectSharedClusterNetworkIntegrity(ctx context.Context) string {
	type requiredNetworks struct {
		container string
		networks  []string
	}
	required := []requiredNetworks{
		{container: gatewayContainer, networks: []string{"tobari-control", "tobari-egress"}},
		{container: opaContainer, networks: []string{"tobari-control"}},
	}
	if brokerRuntimeEnabled {
		required = append(required, requiredNetworks{
			container: authBrokerContainer,
			networks:  []string{"tobari-control", "tobari-egress"},
		})
	}
	for _, component := range required {
		observed, err := r.containerNetworkAddresses(ctx, component.container, "shared cluster")
		if err != nil {
			return "invalid"
		}
		for _, network := range component.networks {
			if observed[network] == "" {
				return "invalid"
			}
		}
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
	journal, journalExists, err := r.readClusterJournal()
	if err != nil {
		return fmt.Errorf("read interrupted cluster reconcile before down: %w", err)
	}
	if journalExists && journal.Operation == clusterOperationUp {
		if err := r.recoverInterruptedClusterUp(ctx, state, true); err != nil {
			return fmt.Errorf("recover interrupted cluster activation before down: %w", err)
		}
	} else if journalExists && journal.Operation != clusterOperationDown {
		return fmt.Errorf("interrupted cluster reconcile is not recoverable by down")
	}
	current, exists, err := r.LoadState(ctx)
	if err != nil {
		return err
	}
	if !exists || current != state {
		return fmt.Errorf("shared-cluster state changed during down recovery")
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
	composeArgs := []string{"compose", "--project-directory", state.RuntimeDirectory}
	composeArgs = append(composeArgs, composeFileArgs(state.RuntimeDirectory)...)
	composeArgs = append(composeArgs, "down", "--remove-orphans")
	err = r.runner.Run(
		ctx,
		composeArgs,
		environment, nil, &output, &output,
	)
	if err != nil {
		_ = r.recordRecentError(state, "Cluster cleanup did not complete; inspect component logs.")
		return fmt.Errorf("docker compose down: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if brokerRuntimeEnabled {
		if err := r.waitForCredentialCompanionStopped(ctx); err != nil {
			return err
		}
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
