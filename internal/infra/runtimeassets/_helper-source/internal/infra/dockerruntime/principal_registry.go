package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const projectPrincipalRegistrySchema = 1

var projectPrincipalNetworkPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

// projectPrincipalBinding is written by the host and read by Gateway. The
// endpoint pair belongs to one exact owned project network; neither address is
// a caller-provided project selector.
type projectPrincipalBinding struct {
	// The Go names carry current domain vocabulary. These JSON tags belong to
	// the frozen Gateway schema-v1 wire and are compatibility tokens, not public
	// or domain authority names. The Gateway intentionally has no dual reader.
	ProjectID             string `json:"project_id"`
	WorkspaceManifestID   string `json:"context_id"`
	WorkspaceManifestName string `json:"context"`
	ProjectRoot           string `json:"project_root"`
	WorkspaceIP           string `json:"workspace_ip"`
	GatewayIP             string `json:"gateway_ip"`
	Network               string `json:"network"`
}

type projectPrincipalRegistry struct {
	SchemaVersion int                       `json:"schema_version"`
	Bindings      []projectPrincipalBinding `json:"bindings"`
}

func (r projectPrincipalRegistry) Validate() error {
	if r.SchemaVersion != projectPrincipalRegistrySchema {
		return fmt.Errorf("project principal registry schema version must be %d", projectPrincipalRegistrySchema)
	}
	if r.Bindings == nil {
		return fmt.Errorf("project principal registry bindings must be an array")
	}
	projects := make(map[string]struct{}, len(r.Bindings))
	workspaceAddresses := make(map[string]struct{}, len(r.Bindings))
	gatewayAddresses := make(map[string]struct{}, len(r.Bindings))
	networks := make(map[string]struct{}, len(r.Bindings))
	for _, binding := range r.Bindings {
		if err := tobari.ValidateWorkspaceID(binding.ProjectID); err != nil {
			return fmt.Errorf("project principal project_id: %w", err)
		}
		if err := tobari.ValidateWorkspaceManifestID(binding.WorkspaceManifestID); err != nil {
			return fmt.Errorf("project principal context_id: %w", err)
		}
		if err := tobari.ValidateName(binding.WorkspaceManifestName); err != nil {
			return fmt.Errorf("project principal context: %w", err)
		}
		if !filepath.IsAbs(binding.ProjectRoot) || filepath.Clean(binding.ProjectRoot) != binding.ProjectRoot {
			return fmt.Errorf("project principal project_root is invalid")
		}
		if _, exists := projects[binding.ProjectID]; exists {
			return fmt.Errorf("project principal project IDs must be unique")
		}
		if _, exists := networks[binding.Network]; exists {
			return fmt.Errorf("project principal networks must be unique")
		}
		if !projectPrincipalNetworkPattern.MatchString(binding.Network) {
			return fmt.Errorf("project principal network is invalid")
		}
		workspaceAddress := net.ParseIP(binding.WorkspaceIP)
		if workspaceAddress == nil || workspaceAddress.To4() == nil || !workspaceAddress.IsGlobalUnicast() {
			return fmt.Errorf("project principal Workspace address is not a usable unicast address")
		}
		gatewayAddress := net.ParseIP(binding.GatewayIP)
		if gatewayAddress == nil || gatewayAddress.To4() == nil || !gatewayAddress.IsGlobalUnicast() {
			return fmt.Errorf("project principal gateway address is not a usable unicast address")
		}
		canonicalWorkspace := workspaceAddress.To4().String()
		canonicalGateway := gatewayAddress.To4().String()
		if canonicalWorkspace == canonicalGateway {
			return fmt.Errorf("project principal Workspace and Gateway addresses must be distinct")
		}
		if _, exists := workspaceAddresses[canonicalWorkspace]; exists {
			return fmt.Errorf("project principal Workspace addresses must be unique")
		}
		if _, exists := gatewayAddresses[canonicalGateway]; exists {
			return fmt.Errorf("project principal gateway addresses must be unique")
		}
		if _, exists := gatewayAddresses[canonicalWorkspace]; exists {
			return fmt.Errorf("project principal Workspace and Gateway addresses must not overlap")
		}
		if _, exists := workspaceAddresses[canonicalGateway]; exists {
			return fmt.Errorf("project principal Workspace and Gateway addresses must not overlap")
		}
		projects[binding.ProjectID] = struct{}{}
		workspaceAddresses[canonicalWorkspace] = struct{}{}
		gatewayAddresses[canonicalGateway] = struct{}{}
		networks[binding.Network] = struct{}{}
	}
	return nil
}

func sameProjectPrincipalRegistry(left, right projectPrincipalRegistry) bool {
	return left.SchemaVersion == right.SchemaVersion && slices.Equal(left.Bindings, right.Bindings)
}

func (r *Runtime) principalRegistryPath() string {
	return filepath.Join(r.principalRegistryDirectory(), "principals.json")
}

func (r *Runtime) principalRegistryDirectory() string {
	return filepath.Join(r.configDirectory, "principal-registry")
}

func emptyProjectPrincipalRegistry() projectPrincipalRegistry {
	return projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema,
		Bindings:      []projectPrincipalBinding{},
	}
}

func (r *Runtime) readProjectPrincipalRegistry() (projectPrincipalRegistry, error) {
	var registry projectPrincipalRegistry
	if err := readStrictJSON(r.principalRegistryPath(), &registry); err != nil {
		return projectPrincipalRegistry{}, err
	}
	if err := registry.Validate(); err != nil {
		return projectPrincipalRegistry{}, err
	}
	return registry, nil
}

func (r *Runtime) ensureProjectPrincipalRegistry(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.principalRegistryDirectory()); err != nil {
		return fmt.Errorf("prepare principal registry directory: %w", err)
	}
	path := r.principalRegistryPath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := initializeBytes(path, mustJSONBytes(emptyProjectPrincipalRegistry()), 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("inspect principal registry: %w", err)
	}
	_, err := r.readProjectPrincipalRegistry()
	return err
}

func mustJSONBytes(value any) []byte {
	data, err := jsonMarshalIndent(value)
	if err != nil {
		panic(err)
	}
	return append(data, '\n')
}

// jsonMarshalIndent is kept local so registry initialization shares the
// strict JSON shape of writeAtomicJSON without exposing a new public helper.
func jsonMarshalIndent(value any) ([]byte, error) {
	return json.MarshalIndent(value, "", "  ")
}

func (r *Runtime) withPrincipalRegistryLock(ctx context.Context, action func() error) error {
	return r.withConfigFileLock(ctx, "principal-registry.lock", "principal registry", action)
}

func (r *Runtime) withPolicyProjectionLock(ctx context.Context, action func() error) error {
	r.policyProjectionMu.Lock()
	defer r.policyProjectionMu.Unlock()
	return r.withConfigFileLock(ctx, "policy-projection.lock", "policy projection", action)
}

func (r *Runtime) withConfigFileLock(ctx context.Context, filename, purpose string, action func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.configDirectory); err != nil {
		return fmt.Errorf("prepare %s directory: %w", purpose, err)
	}
	path := filepath.Join(r.configDirectory, filename)
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("%s lock is not a regular file", purpose)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s lock: %w", purpose, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed owner-only configuration child.
	if err != nil {
		return fmt.Errorf("open %s lock: %w", purpose, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect %s lock: %w", purpose, err)
	}
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock %s: %w", purpose, lockErr)
		}
		if acquired {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unlockProjectFile(file)
	return action()
}

func (r *Runtime) writeProjectPrincipalRegistry(registry projectPrincipalRegistry) error {
	if err := registry.Validate(); err != nil {
		return err
	}
	return writeAtomicJSON(r.principalRegistryPath(), registry)
}

func (r *Runtime) updateProjectPrincipal(ctx context.Context, project tobari.Workspace, network, workspaceIP, gatewayIP string) error {
	if err := project.Validate(); err != nil {
		return err
	}
	binding := projectPrincipalBinding{
		ProjectID: project.ID, WorkspaceManifestID: project.WorkspaceManifestID, WorkspaceManifestName: project.WorkspaceManifestName,
		ProjectRoot: project.Root, Network: network, WorkspaceIP: workspaceIP, GatewayIP: gatewayIP,
	}
	registry := emptyProjectPrincipalRegistry()
	registry.Bindings = append(registry.Bindings, binding)
	if err := registry.Validate(); err != nil {
		return err
	}
	return r.withPrincipalRegistryLock(ctx, func() error {
		current, err := r.readProjectPrincipalRegistry()
		if errors.Is(err, os.ErrNotExist) {
			current = emptyProjectPrincipalRegistry()
		} else if err != nil {
			return fmt.Errorf("read project principal registry: %w", err)
		}
		filtered := make([]projectPrincipalBinding, 0, len(current.Bindings)+1)
		for _, existing := range current.Bindings {
			if existing.ProjectID == project.ID || existing.Network == network || existing.WorkspaceIP == workspaceIP || existing.GatewayIP == gatewayIP {
				continue
			}
			filtered = append(filtered, existing)
		}
		filtered = append(filtered, binding)
		sort.Slice(filtered, func(i, j int) bool { return filtered[i].ProjectID < filtered[j].ProjectID })
		current.Bindings = filtered
		return r.writeProjectPrincipalRegistry(current)
	})
}

func (r *Runtime) removeProjectPrincipal(ctx context.Context, projectID string) error {
	if err := tobari.ValidateWorkspaceID(projectID); err != nil {
		return err
	}
	return r.withPrincipalRegistryLock(ctx, func() error {
		registry, err := r.readProjectPrincipalRegistry()
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read project principal registry: %w", err)
		}
		filtered := registry.Bindings[:0]
		for _, binding := range registry.Bindings {
			if binding.ProjectID != projectID {
				filtered = append(filtered, binding)
			}
		}
		registry.Bindings = filtered
		return r.writeProjectPrincipalRegistry(registry)
	})
}

func (r *Runtime) replaceProjectPrincipalRegistry(ctx context.Context, bindings []projectPrincipalBinding) error {
	registry := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: bindings}
	if err := registry.Validate(); err != nil {
		return err
	}
	sort.Slice(registry.Bindings, func(i, j int) bool { return registry.Bindings[i].ProjectID < registry.Bindings[j].ProjectID })
	return r.withPrincipalRegistryLock(ctx, func() error {
		return r.writeProjectPrincipalRegistry(registry)
	})
}

func (r *Runtime) replaceProjectPrincipalRegistryIfCurrent(
	ctx context.Context, expected projectPrincipalRegistry, bindings []projectPrincipalBinding,
) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	candidate := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: append([]projectPrincipalBinding{}, bindings...)}
	sort.Slice(candidate.Bindings, func(i, j int) bool { return candidate.Bindings[i].ProjectID < candidate.Bindings[j].ProjectID })
	if err := candidate.Validate(); err != nil {
		return err
	}
	return r.withPrincipalRegistryLock(ctx, func() error {
		current, err := r.readProjectPrincipalRegistry()
		if err != nil {
			return err
		}
		if !sameProjectPrincipalRegistry(current, expected) {
			return fmt.Errorf("project principal registry changed before exact replacement")
		}
		return r.writeProjectPrincipalRegistry(candidate)
	})
}

func (r *Runtime) containerNetworkAddressIfConnected(
	ctx context.Context, container, network, purpose string,
) (string, bool, error) {
	networks, err := r.containerNetworkAddresses(ctx, container, purpose)
	if err != nil {
		return "", false, err
	}
	address, connected := networks[network]
	return address, connected && address != "", nil
}

func (r *Runtime) containerNetworkAddresses(
	ctx context.Context, container, purpose string,
) (map[string]string, error) {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", container},
		os.Environ(),
	)
	if err != nil {
		return nil, fmt.Errorf("inspect %s networks: %w: %s", purpose, err, boundedDiagnostic(output))
	}
	var networks map[string]struct {
		IPAddress string `json:"IPAddress"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return nil, fmt.Errorf("decode %s networks: %w", purpose, err)
	}
	addresses := make(map[string]string, len(networks))
	for network, endpoint := range networks {
		if endpoint.IPAddress == "" {
			continue
		}
		address := net.ParseIP(endpoint.IPAddress)
		if address == nil || !address.IsGlobalUnicast() {
			return nil, fmt.Errorf("%s network address is not a usable unicast address", purpose)
		}
		addresses[network] = address.String()
	}
	return addresses, nil
}

func (r *Runtime) containerNetworkAddress(ctx context.Context, container, network, purpose string) (string, error) {
	address, connected, err := r.containerNetworkAddressIfConnected(ctx, container, network, purpose)
	if err != nil {
		return "", err
	}
	if !connected {
		return "", fmt.Errorf("%s is not attached to project network %s", purpose, network)
	}
	return address, nil
}

func (r *Runtime) gatewayNetworkAddress(ctx context.Context, network string) (string, error) {
	return r.containerNetworkAddress(ctx, gatewayContainer, network, "Gateway")
}

func (r *Runtime) workspaceNetworkAddress(ctx context.Context, container, network string) (string, error) {
	return r.containerNetworkAddress(ctx, container, network, "Workspace")
}

func (r *Runtime) syncProjectPrincipalRegistry(ctx context.Context, projects []tobari.Workspace) error {
	expected, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return err
	}
	_, err = r.syncProjectPrincipalRegistryFrom(ctx, projects, expected, nil)
	return err
}

func (r *Runtime) syncProjectPrincipalRegistryFrom(
	ctx context.Context, projects []tobari.Workspace, expected projectPrincipalRegistry,
	beforeReplace func(projectPrincipalRegistry) error,
) (projectPrincipalRegistry, error) {
	bindings := make([]projectPrincipalBinding, 0, len(projects))
	for _, project := range projects {
		if err := project.Validate(); err != nil {
			return projectPrincipalRegistry{}, err
		}
		container, network, err := tobari.ProjectResourceNames(project.ID)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		exists, err := r.projectResourceExists(ctx, "network", network)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		if !exists {
			continue
		}
		if err := r.verifyOwnedProjectResource(ctx, "network", network, project.ID, projectNetRole); err != nil {
			return projectPrincipalRegistry{}, err
		}
		if err := r.ensureGatewayNetwork(ctx, network); err != nil {
			return projectPrincipalRegistry{}, err
		}
		containerExists, err := r.projectResourceExists(ctx, "container", container)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		if !containerExists {
			continue
		}
		if err := r.verifyOwnedProjectResource(ctx, "container", container, project.ID, projectWorkRole); err != nil {
			return projectPrincipalRegistry{}, err
		}
		component, err := r.inspectContainer(ctx, projectWorkRole, container)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		if component.State != "running" {
			continue
		}
		subnet, err := r.projectNetworkSubnet(ctx, network)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		gatewayAddress, err := r.gatewayNetworkAddress(ctx, network)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		if err := r.ensureWorkspaceNetworkGuard(ctx, project, container, network, subnet, gatewayAddress); err != nil {
			return projectPrincipalRegistry{}, err
		}
		workspaceAddress, err := r.workspaceNetworkAddress(ctx, container, network)
		if err != nil {
			return projectPrincipalRegistry{}, err
		}
		if err := validateProjectNetworkEndpoints(subnet, workspaceAddress, gatewayAddress); err != nil {
			return projectPrincipalRegistry{}, err
		}
		bindings = append(bindings, projectPrincipalBinding{
			ProjectID: project.ID, WorkspaceManifestID: project.WorkspaceManifestID, WorkspaceManifestName: project.WorkspaceManifestName,
			ProjectRoot: project.Root, WorkspaceIP: workspaceAddress, GatewayIP: gatewayAddress, Network: network,
		})
	}
	candidate := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: bindings}
	sort.Slice(candidate.Bindings, func(i, j int) bool { return candidate.Bindings[i].ProjectID < candidate.Bindings[j].ProjectID })
	if err := candidate.Validate(); err != nil {
		return projectPrincipalRegistry{}, err
	}
	if beforeReplace != nil {
		if err := beforeReplace(candidate); err != nil {
			return projectPrincipalRegistry{}, err
		}
	}
	if err := r.replaceProjectPrincipalRegistryIfCurrent(ctx, expected, candidate.Bindings); err != nil {
		return projectPrincipalRegistry{}, err
	}
	return candidate, nil
}
