package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	projectIDLabel   = "io.tobari.id"
	projectRoleLabel = "io.tobari.role"
	projectWorkRole  = "work"
	projectNetRole   = "network"
)

// EnsureProjectRuntime converges the exact runtime resources of a durable
// logical Tobari. It never removes logical state when Docker is missing or
// cannot be classified safely.
func (r *Runtime) EnsureProjectRuntime(
	ctx context.Context, state tobari.State, instance tobari.ProjectInstance,
) (tobari.ProjectInstance, error) {
	if err := state.Validate(); err != nil {
		return tobari.ProjectInstance{}, err
	}
	if err := instance.Validate(); err != nil {
		return tobari.ProjectInstance{}, err
	}
	var updated tobari.ProjectInstance
	err := r.withProjectLock(ctx, func() error {
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored != instance {
			return fmt.Errorf("project state changed before runtime reconciliation")
		}
		if resolved, resolveErr := r.ResolveRoot(ctx, stored.Root); resolveErr != nil || resolved != stored.Root {
			return fmt.Errorf("project root is no longer accessible at its canonical path")
		}
		if err := r.ensurePrivateDirectory(r.projectHomePath(stored.ID)); err != nil {
			return fmt.Errorf("prepare project home: %w", err)
		}
		profile, err := r.ensureSharedProfile(stored.Profile)
		if err != nil {
			return err
		}
		if err := r.ensureProjectAgentState(stored.ID, profile); err != nil {
			return err
		}
		container, network, err := tobari.ProjectResourceNames(stored.ID)
		if err != nil {
			return err
		}
		image := stored.Image
		if image == tobari.BuiltinImageSelector {
			image = tobariImage(state)
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return err
		}
		if err := r.ensureProjectNetwork(ctx, network, stored.ID); err != nil {
			return err
		}
		if err := r.ensureGatewayNetwork(ctx, network); err != nil {
			return err
		}
		if err := r.ensureProjectContainer(ctx, state, stored, profile, container, network, image); err != nil {
			return err
		}
		containerID, err := r.projectResourceID(ctx, "container", container)
		if err != nil {
			return err
		}
		networkID, err := r.projectResourceID(ctx, "network", network)
		if err != nil {
			return err
		}
		stored.Runtime = tobari.ProjectRuntime{ContainerID: containerID, NetworkID: networkID}
		if err := r.writeProjectInstance(stored); err != nil {
			return err
		}
		updated = stored
		return nil
	})
	if err != nil {
		return tobari.ProjectInstance{}, err
	}
	return updated, nil
}

// InspectProjectRuntime describes recoverable runtime health without changing
// state. A missing container is not a missing logical Tobari.
func (r *Runtime) InspectProjectRuntime(
	ctx context.Context, instance tobari.ProjectInstance,
) (tobari.RuntimeDiagnostic, error) {
	if err := instance.Validate(); err != nil {
		return tobari.RuntimeDiagnosticUnknown, err
	}
	container, network, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		return tobari.RuntimeDiagnosticUnknown, err
	}
	containerExists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return tobari.RuntimeDiagnosticUnreachable, nil
	}
	if !containerExists {
		return tobari.RuntimeDiagnosticMissing, nil
	}
	networkExists, err := r.projectResourceExists(ctx, "network", network)
	if err != nil {
		return tobari.RuntimeDiagnosticUnreachable, nil
	}
	if !networkExists {
		return tobari.RuntimeDiagnosticDegraded, nil
	}
	component, err := r.inspectContainer(ctx, projectWorkRole, container)
	if err != nil {
		return tobari.RuntimeDiagnosticUnknown, err
	}
	if component.State != "running" || (component.Health != "healthy" && component.Health != "none") {
		return tobari.RuntimeDiagnosticDegraded, nil
	}
	return tobari.RuntimeDiagnosticReady, nil
}

// EnterProjectRuntime attaches the caller's streams to the ready work
// container, maps the host directory below its root, and preserves child exit
// status.
func (r *Runtime) EnterProjectRuntime(
	ctx context.Context, instance tobari.ProjectInstance, cwd string,
	in io.Reader, out, errOut io.Writer,
) (int, error) {
	if err := instance.Validate(); err != nil {
		return 0, err
	}
	resolved, err := r.ResolveRoot(ctx, cwd)
	if err != nil {
		return 0, err
	}
	workdir, err := tobari.MapHostCWD(instance.Root, resolved)
	if err != nil {
		return 0, err
	}
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		return 0, err
	}
	uid, gid := currentIDs()
	args := []string{
		"exec", "-i", "-t", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--workdir", workdir, container, "/bin/bash",
	}
	if err := r.runner.Run(ctx, args, os.Environ(), in, out, errOut); err == nil {
		return 0, nil
	} else {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode(), nil
		}
		type exitCoder interface{ ExitCode() int }
		var coded exitCoder
		if errors.As(err, &coded) {
			return coded.ExitCode(), nil
		}
		return 0, err
	}
}

// InsideProject reports whether the current process is already executing in a
// Tobari work container.
func (r *Runtime) InsideProject(context.Context) bool {
	return os.Getenv("TOBARI_INSIDE") == "1"
}

// ProjectHome returns the exact per-project XDG home path for presentation and
// deletion diagnostics. The path is derived only from the validated logical
// ID and never from a user-supplied Docker identifier.
func (r *Runtime) ProjectHome(ctx context.Context, instance tobari.ProjectInstance) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := instance.Validate(); err != nil {
		return "", err
	}
	return r.projectHomePath(instance.ID), nil
}

// DeleteProject removes only exact owned runtime resources and the selected
// instance's state and home. Shared cluster and profile data are never targets.
func (r *Runtime) DeleteProject(ctx context.Context, instance tobari.ProjectInstance) error {
	if err := instance.Validate(); err != nil {
		return err
	}
	return r.withProjectLock(ctx, func() error {
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored != instance {
			return fmt.Errorf("project state changed before deletion")
		}
		container, network, err := tobari.ProjectResourceNames(stored.ID)
		if err != nil {
			return err
		}
		containerExists, err := r.projectResourceExists(ctx, "container", container)
		if err != nil {
			return err
		}
		if containerExists {
			if err := r.verifyOwnedProjectResource(ctx, "container", container, stored.ID, projectWorkRole); err != nil {
				return err
			}
			if output, removeErr := r.runner.Output(ctx, []string{"rm", "-f", container}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove project container: %w: %s", removeErr, boundedDiagnostic(output))
			}
		}
		networkExists, err := r.projectResourceExists(ctx, "network", network)
		if err != nil {
			return err
		}
		if networkExists {
			if err := r.verifyOwnedProjectResource(ctx, "network", network, stored.ID, projectNetRole); err != nil {
				return err
			}
			if output, disconnectErr := r.runner.Output(
				ctx, []string{"network", "disconnect", "-f", network, gatewayContainer}, os.Environ(),
			); disconnectErr != nil {
				return fmt.Errorf("disconnect Gateway from project network: %w: %s", disconnectErr, boundedDiagnostic(output))
			}
			if output, removeErr := r.runner.Output(ctx, []string{"network", "rm", network}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove project network: %w: %s", removeErr, boundedDiagnostic(output))
			}
		}
		if err := r.removeProjectRecords(stored); err != nil {
			return err
		}
		return nil
	})
}

func (r *Runtime) ensureProjectNetwork(ctx context.Context, network, id string) error {
	exists, err := r.projectResourceExists(ctx, "network", network)
	if err != nil {
		return err
	}
	if exists {
		return r.verifyOwnedProjectResource(ctx, "network", network, id, projectNetRole)
	}
	args := []string{
		"network", "create", "--internal",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=tobari",
		"--label", projectIDLabel + "=" + id,
		"--label", projectRoleLabel + "=" + projectNetRole,
		network,
	}
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("create project network: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) ensureProjectContainer(
	ctx context.Context, state tobari.State, instance tobari.ProjectInstance,
	profile, container, network, image string,
) error {
	exists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return err
	}
	if exists {
		if err := r.verifyOwnedProjectResource(ctx, "container", container, instance.ID, projectWorkRole); err != nil {
			return err
		}
		component, inspectErr := r.inspectContainer(ctx, projectWorkRole, container)
		if inspectErr != nil {
			return inspectErr
		}
		if err := r.ensureProjectContainerNetwork(ctx, container, network); err != nil {
			return err
		}
		if component.State != "running" {
			if output, startErr := r.runner.Output(ctx, []string{"start", container}, os.Environ()); startErr != nil {
				return fmt.Errorf("start project container: %w: %s", startErr, boundedDiagnostic(output))
			}
		}
		return nil
	}
	uid, gid := currentIDs()
	args := []string{
		"create", "--name", container, "--hostname", container,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--env", "HOME=/var/lib/tobari",
		"--env", "TOBARI_INSIDE=1", "--env", "TOBARI_ID=" + instance.ID, "--env", "TOBARI_ROOT=/workspace",
		"--env", "TOBARI_PROFILE=/opt/tobari/profile",
		"--env", "HTTP_PROXY=http://gateway:8080", "--env", "HTTPS_PROXY=http://gateway:8080",
		"--env", "http_proxy=http://gateway:8080", "--env", "https_proxy=http://gateway:8080",
		"--env", "NO_PROXY=", "--env", "no_proxy=",
		"--env", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
		"--env", "REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
		"--mount", "type=bind,src=" + instance.Root + ",dst=/workspace",
		"--mount", "type=bind,src=" + r.projectHomePath(instance.ID) + ",dst=/var/lib/tobari",
		"--mount", "type=bind,src=" + profile + ",dst=/opt/tobari/profile,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "skills") + ",dst=/var/lib/tobari/.claude/skills,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "agents") + ",dst=/var/lib/tobari/.claude/agents,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "commands") + ",dst=/var/lib/tobari/.claude/commands,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "plugins.lock") + ",dst=/var/lib/tobari/.claude/plugins.lock,readonly",
		"--mount", "type=volume,src=tobari-public-ca,dst=/run/tobari/ca-public,readonly",
		"--workdir", "/workspace", "--network", network,
		"--health-cmd", "test -f /tmp/tobari-ready", "--health-interval", "2s",
		"--health-timeout", "2s", "--health-retries", "30",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=tobari",
		"--label", projectIDLabel + "=" + instance.ID,
		"--label", projectRoleLabel + "=" + projectWorkRole,
		image,
	}
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("create project container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(ctx, []string{"start", container}, os.Environ()); err != nil {
		return fmt.Errorf("start project container: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) ensureProjectContainerNetwork(ctx context.Context, container, network string) error {
	output, err := r.runner.Output(
		ctx, []string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", container}, os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("inspect project container networks: %w: %s", err, boundedDiagnostic(output))
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return fmt.Errorf("decode project container networks: %w", err)
	}
	if _, connected := networks[network]; connected {
		return nil
	}
	if output, err := r.runner.Output(
		ctx, []string{"network", "connect", network, container}, os.Environ(),
	); err != nil {
		return fmt.Errorf("connect project container to network: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) projectResourceExists(ctx context.Context, kind, name string) (bool, error) {
	args := []string{"inspect", "--format", "{{.Id}}", name}
	if kind == "network" {
		args = []string{"network", "inspect", "--format", "{{.Id}}", name}
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err == nil {
		return true, nil
	}
	diagnostic := strings.ToLower(err.Error() + " " + string(output))
	if strings.Contains(diagnostic, "no such") || strings.Contains(diagnostic, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("inspect project %s: %w", kind, err)
}

func (r *Runtime) projectResourceID(ctx context.Context, kind, name string) (string, error) {
	args := []string{"inspect", "--format", "{{.Id}}", name}
	if kind == "network" {
		args = []string{"network", "inspect", "--format", "{{.Id}}", name}
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil || strings.TrimSpace(string(output)) == "" {
		return "", fmt.Errorf("read project %s identifier: %w", kind, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (r *Runtime) verifyOwnedProjectResource(ctx context.Context, kind, name, id, role string) error {
	if err := r.verifyOwned(ctx, kind, name); err != nil {
		return err
	}
	for label, expected := range map[string]string{projectIDLabel: id, projectRoleLabel: role} {
		args := []string{"inspect", "--format", `{{index .Config.Labels "` + label + `"}}`, name}
		if kind == "network" {
			args = []string{"network", "inspect", "--format", `{{index .Labels "` + label + `"}}`, name}
		}
		output, err := r.runner.Output(ctx, args, os.Environ())
		if err != nil {
			return fmt.Errorf("inspect project %s ownership: %w", kind, err)
		}
		if strings.TrimSpace(string(output)) != expected {
			return fmt.Errorf("%s %s does not belong to the selected Tobari", kind, name)
		}
	}
	return nil
}

func (r *Runtime) ensureSharedProfile(profile string) (string, error) {
	if profile != tobari.DefaultProfile {
		return "", fmt.Errorf("unsupported project profile")
	}
	directory := filepath.Join(r.dataDirectory, "profiles", profile)
	for _, child := range []string{
		directory,
		filepath.Join(directory, "claude"),
		filepath.Join(directory, "claude", "skills"),
		filepath.Join(directory, "claude", "agents"),
		filepath.Join(directory, "claude", "commands"),
		filepath.Join(directory, "common"),
	} {
		if err := r.ensurePrivateDirectory(child); err != nil {
			return "", fmt.Errorf("prepare shared agent profile: %w", err)
		}
	}
	if err := initializeBytes(filepath.Join(directory, "claude", "plugins.lock"), []byte("{}\n"), 0o600); err != nil {
		return "", err
	}
	if err := initializeBytes(filepath.Join(directory, "common", "settings.json"), []byte("{}\n"), 0o600); err != nil {
		return "", err
	}
	return directory, nil
}

func (r *Runtime) ensureProjectAgentState(id, profile string) error {
	home := r.projectHomePath(id)
	claude := filepath.Join(home, ".claude")
	for _, directory := range []string{
		claude,
		filepath.Join(claude, "skills"),
		filepath.Join(claude, "agents"),
		filepath.Join(claude, "commands"),
	} {
		if err := r.ensurePrivateDirectory(directory); err != nil {
			return err
		}
	}
	baseSettings, err := os.ReadFile(filepath.Join(profile, "common", "settings.json")) // #nosec G304 -- profile path is runtime-owned.
	if err != nil {
		return err
	}
	return initializeBytes(filepath.Join(claude, "settings.json"), baseSettings, 0o600)
}

func (r *Runtime) removeProjectRecords(instance tobari.ProjectInstance) error {
	indexPath, err := r.rootIndexPath(instance.Root)
	if err != nil {
		return err
	}
	var index tobari.RootIndex
	if err := readStrictJSON(indexPath, &index); err != nil {
		return err
	}
	if index.InstanceID != instance.ID || index.Root != instance.Root {
		return fmt.Errorf("root index no longer identifies selected Tobari")
	}
	directory, err := r.projectDirectory(instance.ID)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(directory); err != nil { // #nosec G301 -- exact validated instance directory after owned resource cleanup.
		return fmt.Errorf("remove project instance state: %w", err)
	}
	if err := os.Remove(indexPath); err != nil {
		return fmt.Errorf("remove project root index: %w", err)
	}
	if err := syncDirectory(filepath.Dir(indexPath)); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(directory))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
