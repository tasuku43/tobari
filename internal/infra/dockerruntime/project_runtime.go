package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	projectIDLabel           = "io.tobari.id"
	projectRoleLabel         = "io.tobari.role"
	projectSpecLabel         = "io.tobari.spec-hash"
	projectWorkRole          = "work"
	projectNetRole           = "network"
	projectLocalSettingsFile = "settings.local.json"
	projectCPULimit          = "2.0"
	projectMemoryLimit       = "4g"
	projectPIDsLimit         = "512"
	projectLogDriver         = "json-file"
	projectLogMaxSize        = "10m"
	projectLogMaxFiles       = "3"
)

func projectResourceDockerArgs() []string {
	return []string{
		"--cpus", projectCPULimit,
		"--memory", projectMemoryLimit,
		"--memory-swap", projectMemoryLimit,
		"--pids-limit", projectPIDsLimit,
		"--log-driver", projectLogDriver,
		"--log-opt", "max-size=" + projectLogMaxSize,
		"--log-opt", "max-file=" + projectLogMaxFiles,
	}
}

func projectResourceHashFields() []string {
	return []string{
		"cpus=" + projectCPULimit,
		"memory=" + projectMemoryLimit,
		"memory-swap=" + projectMemoryLimit,
		"pids-limit=" + projectPIDsLimit,
		"log-driver=" + projectLogDriver,
		"log-max-size=" + projectLogMaxSize,
		"log-max-files=" + projectLogMaxFiles,
	}
}

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
	if instance.Incomplete {
		return tobari.ProjectInstance{}, fmt.Errorf("project instance state is incomplete; delete the selected Tobari before recreating it")
	}
	var updated tobari.ProjectInstance
	err := r.withProjectLock(ctx, func() error {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored.ID != instance.ID || stored.Root != instance.Root || stored.Profile != instance.Profile || stored.Image != instance.Image {
			return fmt.Errorf("project logical state changed before runtime reconciliation")
		}
		if resolved, resolveErr := r.ResolveProjectRoot(ctx, stored.Root); resolveErr != nil || resolved != stored.Root {
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
		imageID, err := r.compatibleImageID(ctx, image)
		if err != nil {
			return err
		}
		specHash, err := r.projectSpecHash(state, stored, profile, network, image, imageID)
		if err != nil {
			return err
		}
		if err := r.ensureProjectNetwork(ctx, network, stored.ID); err != nil {
			return err
		}
		if err := r.ensureGatewayProjectNetwork(ctx, network, stored.ID); err != nil {
			return err
		}
		if err := r.ensureProjectContainer(ctx, state, stored, profile, container, network, image, specHash); err != nil {
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
	if instance.Incomplete {
		return tobari.RuntimeDiagnosticIncomplete, nil
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
	if component.State != "running" || component.Health != "healthy" {
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
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return 0, err
	}
	workdir, err := tobari.MapProjectCWD(instance.Root, resolved)
	if err != nil {
		return 0, err
	}
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		return 0, err
	}
	uid, gid := currentIDs()
	args := []string{
		// Docker's attached exec path owns the PTY resize and terminal signal
		// forwarding; inherit the caller's streams without a shell wrapper.
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
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil && !(instance.Incomplete && errors.Is(err, os.ErrNotExist)) {
			return err
		}
		if instance.Incomplete && errors.Is(err, os.ErrNotExist) {
			stored = instance
		}
		if stored.ID != instance.ID || stored.Root != instance.Root || stored.Profile != instance.Profile || stored.Image != instance.Image {
			return fmt.Errorf("project logical state changed before deletion")
		}
		journal := projectJournal{
			SchemaVersion: projectJournalSchema, Operation: projectOpDelete,
			ProjectID: stored.ID, Root: stored.Root, Phase: projectPhaseStarted,
		}
		if err := r.writeProjectJournal(journal); err != nil {
			return err
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
			if err := r.disconnectGatewayIfConnected(ctx, network); err != nil {
				return err
			}
			if output, removeErr := r.runner.Output(ctx, []string{"network", "rm", network}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove project network: %w: %s", removeErr, boundedDiagnostic(output))
			}
		}
		if err := r.removeProjectPrincipal(ctx, stored.ID); err != nil {
			return fmt.Errorf("remove project principal: %w", err)
		}
		journal.Phase = projectPhaseRuntime
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.removeProjectInstanceDirectory(stored.ID); err != nil {
			return err
		}
		journal.Phase = projectPhaseInstance
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.removeProjectRootIndex(stored.Root); err != nil {
			return err
		}
		return r.clearProjectJournal()
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
	profile, container, network, image, specHash string,
) error {
	workspaceRoot, err := tobari.ProjectWorkspaceRoot(instance.Root)
	if err != nil {
		return err
	}
	exists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return err
	}
	if exists {
		if err := r.verifyOwnedProjectResource(ctx, "container", container, instance.ID, projectWorkRole); err != nil {
			return err
		}
		observedSpec, specErr := r.projectContainerSpecHash(ctx, container)
		if specErr != nil {
			return specErr
		}
		if observedSpec != specHash {
			if output, removeErr := r.runner.Output(ctx, []string{"rm", "-f", container}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove drifted project container: %w: %s", removeErr, boundedDiagnostic(output))
			}
			exists = false
		}
	}
	if exists {
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
		return r.waitProjectReady(ctx, container)
	}
	uid, gid := currentIDs()
	args := []string{
		"create", "--name", container, "--hostname", container,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--env", "HOME=/var/lib/tobari",
		"--env", "TOBARI_INSIDE=1", "--env", "TOBARI_ID=" + instance.ID, "--env", "TOBARI_ROOT=" + workspaceRoot,
		"--env", "TOBARI_PROFILE=/opt/tobari/profile",
		"--env", "HTTP_PROXY=http://gateway:8080", "--env", "HTTPS_PROXY=http://gateway:8080",
		"--env", "http_proxy=http://gateway:8080", "--env", "https_proxy=http://gateway:8080",
		"--env", "NO_PROXY=", "--env", "no_proxy=",
		"--env", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
		"--env", "REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
		"--mount", "type=bind,src=" + instance.Root + ",dst=" + workspaceRoot,
		"--mount", "type=bind,src=" + r.projectHomePath(instance.ID) + ",dst=/var/lib/tobari",
		"--mount", "type=bind,src=" + profile + ",dst=/opt/tobari/profile,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "skills") + ",dst=/var/lib/tobari/.claude/skills,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "agents") + ",dst=/var/lib/tobari/.claude/agents,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "commands") + ",dst=/var/lib/tobari/.claude/commands,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "plugins.lock") + ",dst=/var/lib/tobari/.claude/plugins.lock,readonly",
		"--mount", "type=volume,src=tobari-public-ca,dst=/run/tobari/ca-public,readonly",
		"--workdir", workspaceRoot, "--network", network,
		"--health-cmd", "test -f /tmp/tobari-ready", "--health-interval", "2s",
		"--health-timeout", "2s", "--health-retries", "30",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=tobari",
		"--label", projectIDLabel + "=" + instance.ID,
		"--label", projectRoleLabel + "=" + projectWorkRole,
		"--label", projectSpecLabel + "=" + specHash,
	}
	args = append(args, projectResourceDockerArgs()...)
	args = append(args, image)
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("create project container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(ctx, []string{"start", container}, os.Environ()); err != nil {
		return fmt.Errorf("start project container: %w: %s", err, boundedDiagnostic(output))
	}
	return r.waitProjectReady(ctx, container)
}

func (r *Runtime) waitProjectReady(ctx context.Context, container string) error {
	const attempts = 60
	for attempt := 0; attempt < attempts; attempt++ {
		output, err := r.runner.Output(
			ctx,
			[]string{"inspect", "--format", `{"state":"{{.State.Status}}","health":"{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}"}`, container},
			os.Environ(),
		)
		if err != nil {
			return fmt.Errorf("inspect project readiness: %w: %s", err, boundedDiagnostic(output))
		}
		var observed struct {
			State  string `json:"state"`
			Health string `json:"health"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(output), &observed); err != nil {
			return fmt.Errorf("decode project readiness: %w", err)
		}
		if observed.State == "exited" || observed.State == "dead" {
			return fmt.Errorf("project container exited")
		}
		switch observed.Health {
		case "healthy":
			return nil
		case "unhealthy":
			return fmt.Errorf("project container is unhealthy")
		case "none":
			return fmt.Errorf("project container has no readiness healthcheck")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("project container did not become healthy")
}

func (r *Runtime) compatibleImageID(ctx context.Context, image string) (string, error) {
	output, err := r.runner.Output(ctx, []string{"image", "inspect", "--format", "{{.Id}}", image}, os.Environ())
	if err != nil {
		return "", fmt.Errorf("inspect compatible image identity: %w: %s", err, boundedDiagnostic(output))
	}
	imageID := strings.TrimSpace(string(output))
	if imageID == "" {
		return "", fmt.Errorf("compatible image identity is empty")
	}
	return imageID, nil
}

type projectRuntimeSpec struct {
	Image          string   `json:"image"`
	ImageID        string   `json:"image_id"`
	RuntimeAPI     string   `json:"runtime_api"`
	AssetVersion   string   `json:"asset_version"`
	WorkspaceRoot  string   `json:"workspace_root"`
	Root           string   `json:"root"`
	Network        string   `json:"network"`
	User           string   `json:"user"`
	Environment    []string `json:"environment"`
	Mounts         []string `json:"mounts"`
	ReadOnly       bool     `json:"read_only"`
	Capabilities   string   `json:"capabilities"`
	Security       string   `json:"security"`
	Resources      []string `json:"resources"`
	HealthCommand  string   `json:"health_command"`
	HealthInterval string   `json:"health_interval"`
	ProfileDigest  string   `json:"profile_digest"`
}

func (r *Runtime) projectSpecHash(
	state tobari.State, instance tobari.ProjectInstance, profile, network, image, imageID string,
) (string, error) {
	workspaceRoot, err := tobari.ProjectWorkspaceRoot(instance.Root)
	if err != nil {
		return "", err
	}
	profileDigest, err := r.projectProfileDigest(profile)
	if err != nil {
		return "", err
	}
	uid, gid := currentIDs()
	spec := projectRuntimeSpec{
		Image: image, ImageID: imageID, RuntimeAPI: tobari.RuntimeImageAPI, AssetVersion: state.AssetVersion,
		WorkspaceRoot: workspaceRoot, Root: instance.Root, Network: network,
		User: strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		Environment: []string{
			"HOME=/var/lib/tobari", "TOBARI_INSIDE=1", "TOBARI_ID=" + instance.ID,
			"TOBARI_ROOT=" + workspaceRoot, "TOBARI_PROFILE=/opt/tobari/profile",
			"HTTP_PROXY=http://gateway:8080", "HTTPS_PROXY=http://gateway:8080",
			"http_proxy=http://gateway:8080", "https_proxy=http://gateway:8080",
			"NO_PROXY=", "no_proxy=", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
			"REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
		},
		Mounts: []string{
			"bind:" + instance.Root + "->" + workspaceRoot,
			"bind:" + r.projectHomePath(instance.ID) + "->/var/lib/tobari",
			"bind:" + profile + "->/opt/tobari/profile:ro",
			"bind:" + filepath.Join(profile, "claude", "skills") + "->/var/lib/tobari/.claude/skills:ro",
			"bind:" + filepath.Join(profile, "claude", "agents") + "->/var/lib/tobari/.claude/agents:ro",
			"bind:" + filepath.Join(profile, "claude", "commands") + "->/var/lib/tobari/.claude/commands:ro",
			"bind:" + filepath.Join(profile, "claude", "plugins.lock") + "->/var/lib/tobari/.claude/plugins.lock:ro",
			"volume:tobari-public-ca->/run/tobari/ca-public:ro",
		},
		ReadOnly: true, Capabilities: "ALL", Security: "no-new-privileges:true",
		Resources:     projectResourceHashFields(),
		HealthCommand: "test -f /tmp/tobari-ready", HealthInterval: "2s", ProfileDigest: profileDigest,
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:]), nil
}

func (r *Runtime) projectProfileDigest(profile string) (string, error) {
	hash := sha256.New()
	for _, path := range []string{
		filepath.Join(profile, "claude", "plugins.lock"),
		filepath.Join(profile, "common", "settings.json"),
	} {
		data, err := os.ReadFile(path) // #nosec G304 -- exact runtime-owned profile files.
		if err != nil {
			return "", fmt.Errorf("read shared profile revision: %w", err)
		}
		_, _ = hash.Write([]byte(filepath.Base(filepath.Dir(path))))
		_, _ = hash.Write(data)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}

func (r *Runtime) projectContainerSpecHash(ctx context.Context, container string) (string, error) {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", `{{index .Config.Labels "` + projectSpecLabel + `"}}`, container},
		os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("inspect project spec hash: %w: %s", err, boundedDiagnostic(output))
	}
	return strings.TrimSpace(string(output)), nil
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

func (r *Runtime) disconnectGatewayIfConnected(ctx context.Context, network string) error {
	exists, err := r.projectResourceExists(ctx, "container", gatewayContainer)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	output, err := r.runner.Output(
		ctx, []string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", gatewayContainer}, os.Environ(),
	)
	if err != nil {
		if isMissingDockerResource(err, output) {
			return nil
		}
		return fmt.Errorf("inspect Gateway networks for deletion: %w: %s", err, boundedDiagnostic(output))
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return fmt.Errorf("decode Gateway networks for deletion: %w", err)
	}
	if _, connected := networks[network]; !connected {
		return nil
	}
	if output, disconnectErr := r.runner.Output(
		ctx, []string{"network", "disconnect", "-f", network, gatewayContainer}, os.Environ(),
	); disconnectErr != nil && !isMissingDockerResource(disconnectErr, output) {
		return fmt.Errorf("disconnect Gateway from project network: %w: %s", disconnectErr, boundedDiagnostic(output))
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
	if err := r.ensurePrivateDirectory(r.dataDirectory); err != nil {
		return "", fmt.Errorf("prepare shared profile data directory: %w", err)
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
	base, err := decodeSettingsObject(baseSettings)
	if err != nil {
		return fmt.Errorf("shared agent settings are invalid: %w", err)
	}
	settingsPath := filepath.Join(claude, "settings.json")
	localPath := filepath.Join(claude, projectLocalSettingsFile)
	local, localFound, err := readProjectSettings(localPath, "per-project local agent settings")
	if err != nil {
		return err
	}
	if !localFound {
		// Older Tobari versions used settings.json for both local input and
		// generated effective settings. Preserve only values that differ from
		// the shared source as local overrides, then keep the effective file
		// generated from the current shared source on every reconciliation.
		legacy, legacyFound, legacyErr := readProjectSettings(settingsPath, "per-project agent settings")
		if legacyErr != nil {
			return legacyErr
		}
		if legacyFound {
			for key, value := range legacy {
				shared, sharedFound := base[key]
				if !sharedFound || !bytes.Equal(shared, value) {
					local[key] = value
				}
			}
			if len(local) != 0 {
				if err := writeAtomicJSON(localPath, local); err != nil {
					return fmt.Errorf("persist migrated local agent settings: %w", err)
				}
			}
		}
	}
	for key, value := range local {
		base[key] = value
	}
	return writeAtomicJSON(settingsPath, base)
}

func readProjectSettings(path, description string) (map[string]json.RawMessage, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, false, fmt.Errorf("%s path is unsafe", description)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- caller supplies only runtime-owned settings paths.
	if err != nil {
		return nil, false, err
	}
	settings, err := decodeSettingsObject(data)
	if err != nil {
		return nil, false, fmt.Errorf("%s are invalid: %w", description, err)
	}
	return settings, true, nil
}

func decodeSettingsObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	return object, nil
}

func (r *Runtime) removeProjectInstanceDirectory(id string) error {
	directory, err := r.projectDirectory(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(directory); err != nil { // #nosec G301 -- exact validated instance directory after owned resource cleanup.
		return fmt.Errorf("remove project instance state: %w", err)
	}
	return syncDirectoryIfPresent(filepath.Dir(directory))
}

func (r *Runtime) removeProjectRootIndex(root string) error {
	indexPath, err := r.rootIndexPath(root)
	if err != nil {
		return err
	}
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove project root index: %w", err)
	}
	return syncDirectoryIfPresent(filepath.Dir(indexPath))
}

func syncDirectory(path string) error {
	directory, err := os.Open(path) // #nosec G304 -- callers pass only runtime-owned state directories.
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func syncDirectoryIfPresent(path string) error {
	if err := syncDirectory(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func isMissingDockerResource(err error, output []byte) bool {
	diagnostic := strings.ToLower(err.Error() + " " + string(output))
	return strings.Contains(diagnostic, "no such") || strings.Contains(diagnostic, "not found")
}
