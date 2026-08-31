package dockerruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	configuratorCleanupTimeout  = 10 * time.Second
	configuratorCleanupAttempts = 3
	configuratorMemoryLimit     = "2g"
	configuratorInitialPrompt   = "Begin the Tobari Configurator workflow now. Read and follow AGENTS.md in this directory, inspect observed.json and the current editable configuration sources, then introduce what you can help configure and ask the user what they want to achieve. Do not edit until you understand their intent."
)

// PrepareConfiguratorRuntime creates only immutable Runtime material. It is
// safe to run before agent selection and creates no Context, cluster, network,
// Workspace, or configuration authority.
func (r *Runtime) PrepareConfiguratorRuntime(ctx context.Context, binding tobari.RuntimeBinding) error {
	if err := r.requireConfiguratorNativeLoginControl(ctx); err != nil {
		return err
	}
	_, err := r.resolveConfiguratorRuntime(ctx, binding, true)
	return err
}

func (r *Runtime) requireConfiguratorNativeLoginControl(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || r.runner == nil {
		return fmt.Errorf("%w: Docker runner is unavailable", tobari.ErrNativeLoginBridgeUnavailable)
	}
	if _, ok := r.runner.(workspaceBrowserControlRunner); !ok {
		return fmt.Errorf("%w: Docker runner has no native-login control capability", tobari.ErrNativeLoginBridgeUnavailable)
	}
	return nil
}

func (r *Runtime) resolveConfiguratorRuntime(ctx context.Context, binding tobari.RuntimeBinding, prepare bool) (string, error) {
	if err := binding.Validate(); err != nil {
		return "", err
	}
	if binding.RuntimeID == tobari.StandardRuntimeID {
		if err := r.validateExactStandardRuntimeBinding(binding); err != nil {
			return "", err
		}
		image := binding.Image
		canonical, err := r.defaultRuntimeImage()
		if err != nil {
			return "", err
		}
		if prepare {
			if image == canonical && r.imageResolver().ShouldPullRuntimeImage(image) {
				if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
					return "", err
				}
			}
			if image == canonical && r.imageResolver().ShouldBuildRuntimeImage(image) {
				if err := r.ensureLocalBaseRuntimeImage(ctx, image); err != nil {
					return "", err
				}
			}
			if err := r.validateCompatibleImage(ctx, image); err != nil {
				return "", err
			}
			return image, nil
		}
	} else if prepare {
		if err := r.PrepareWorkspaceRuntimeMaterial(ctx, binding); err != nil {
			return "", err
		}
		return binding.Image, nil
	}
	// Tests may inject the final material resolver, but production always uses
	// the same lifecycle/store-fenced immutable resolution as Context login.
	if r.finalWorkspaceRuntimeMaterial != nil {
		resolved, _, imageID, err := r.resolveFinalWorkspaceRuntimeMaterial(ctx, binding)
		if err != nil || resolved != binding {
			return "", fmt.Errorf("Context Runtime binding changed before Configurator start: %w", err)
		}
		if !imageIDPattern.MatchString(imageID) {
			return "", fmt.Errorf("Configurator Runtime image identity is not immutable")
		}
		if err := r.validateCompatibleImage(ctx, imageID); err != nil {
			return "", err
		}
		return imageID, nil
	}
	var imageID string
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		return r.withRuntimeStoreLock(lockContext, func() error {
			var resolveErr error
			imageID, resolveErr = r.resolveFinalContextLoginRuntimeImage(lockContext, binding)
			return resolveErr
		})
	})
	if err != nil {
		return "", err
	}
	if !imageIDPattern.MatchString(imageID) {
		return "", fmt.Errorf("Configurator Runtime image identity is not immutable")
	}
	return imageID, nil
}

// RunConfigurator starts the selected pinned agent from the exact seed Runtime
// with direct egress. Its only writable data mount is the complete managed Home.
func (r *Runtime) RunConfigurator(ctx context.Context, draft tobari.ConfiguratorDraft, isolation tobari.ConfiguratorIsolation, in io.Reader, visible io.Writer) (resultErr error) {
	if err := draft.Validate(); err != nil {
		return err
	}
	if err := isolation.Validate(); err != nil {
		return err
	}
	if err := r.requireConfiguratorNativeLoginControl(ctx); err != nil {
		return err
	}
	home, err := r.configuratorHome(draft)
	if err != nil {
		return err
	}
	if err := validateConfiguratorDirectory(home); err != nil {
		return fmt.Errorf("Configurator managed Home: %w", err)
	}
	relative, err := tobari.ConfiguratorWorkingDirectory(draft)
	if err != nil {
		return err
	}
	workdir := filepath.Join(home, filepath.FromSlash(relative))
	if err := validateConfiguratorDirectory(workdir); err != nil {
		return fmt.Errorf("Configurator working directory: %w", err)
	}
	sourceRelative, err := tobari.ConfiguratorSourceDirectory(draft)
	if err != nil {
		return err
	}
	requiredFiles := []string{filepath.Join(workdir, "observed.json"), filepath.Join(workdir, "AGENTS.md"), filepath.Join(workdir, "CLAUDE.md")}
	if draft.Task != tobari.ConfiguratorTaskRuntime {
		templateDir := filepath.Join(home, filepath.FromSlash(sourceRelative), "templates", string(draft.TemplateID))
		requiredFiles = append(requiredFiles, filepath.Join(templateDir, "template.yaml"), filepath.Join(templateDir, "policy.yaml"))
	} else {
		requiredFiles = append(requiredFiles, filepath.Join(workdir, "runtime", "source", "Dockerfile"))
	}
	for _, path := range requiredFiles {
		if err := validateConfiguratorFile(path); err != nil {
			return fmt.Errorf("Configurator working file: %w", err)
		}
	}
	if strings.ContainsAny(home+relative, ",\x00\r\n") {
		return fmt.Errorf("Configurator mount path is unsupported")
	}
	image, err := r.resolveConfiguratorRuntime(ctx, draft.Runtime, false)
	if err != nil {
		return err
	}
	opener, err := r.ensureConfiguratorBrowserOpener(ctx)
	if err != nil {
		return fmt.Errorf("prepare Configurator native-login opener: %w", err)
	}
	name, err := randomConfiguratorContainerName()
	if err != nil {
		return err
	}
	network := name + "-egress"
	var networkID, containerID string
	defer func() {
		cleanupContext, cancel := context.WithTimeout(r.lifetimeParent(ctx), configuratorCleanupTimeout)
		defer cancel()
		if containerID != "" {
			resultErr = errors.Join(resultErr, r.cleanupConfiguratorResource(cleanupContext, "container", []string{"container", "rm", "--force", containerID}))
		}
		if networkID != "" {
			resultErr = errors.Join(resultErr, r.cleanupConfiguratorResource(cleanupContext, "network", []string{"network", "rm", networkID}))
		}
	}()
	networkOutput, err := r.runner.Output(ctx, []string{"network", "create", "--driver", "bridge", "--label", ownerLabel + "=" + ownerValue, "--label", componentLabel + "=configurator", network}, os.Environ())
	if err != nil {
		return fmt.Errorf("create Configurator egress network: %w", err)
	}
	networkID, err = exactDockerResourceID(networkOutput)
	if err != nil {
		return fmt.Errorf("capture Configurator egress network identity: %w", err)
	}
	if err := r.confirmConfiguratorNetwork(ctx, networkID); err != nil {
		return err
	}
	agentExecutable := "/usr/local/bin/" + string(draft.Agent)
	uid, gid := currentIDs()
	containerWorkdir := "/var/lib/tobari/" + relative
	createArgs := []string{
		"container", "create", "--name", name,
		"--label", ownerLabel + "=" + ownerValue, "--label", componentLabel + "=configurator",
		"--interactive", "--tty", "--network", "none", "--read-only", "--log-driver", "none",
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "512",
		"--memory", configuratorMemoryLimit, "--memory-swap", configuratorMemoryLimit, "--cpus", "2",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--hostname", "tobari-configurator", "--workdir", containerWorkdir,
		"--mount", "type=bind,src=" + home + ",dst=/var/lib/tobari",
		"--mount", "type=bind,src=" + opener + ",dst=" + workspaceBrowserOpenerPath + ",readonly",
		"--mount", "type=bind,src=" + opener + ",dst=/usr/local/bin/xdg-open,readonly",
		"--env", "HOME=/var/lib/tobari", "--env", "DISABLE_AUTOUPDATER=1", "--env", "NO_COLOR=1",
		"--env", "TOBARI_CONFIGURATOR=1", "--env", "TOBARI_CONTEXT_ID=" + string(draft.ContextID),
	}
	if draft.Agent == tobari.ConfiguratorAgentCodex {
		createArgs = append(createArgs, "--env", "CODEX_HOME=/var/lib/tobari/.codex")
	}
	createArgs = append(createArgs, "--entrypoint", "/usr/bin/tini", image, "--", "/usr/bin/sleep", "infinity")
	containerOutput, err := r.runner.Output(ctx, createArgs, os.Environ())
	if err != nil {
		return fmt.Errorf("create Configurator container: %w", err)
	}
	containerID, err = exactDockerResourceID(containerOutput)
	if err != nil {
		return fmt.Errorf("capture Configurator container identity: %w", err)
	}
	if err := r.confirmConfiguratorContainer(ctx, containerID, home, opener, image, containerWorkdir); err != nil {
		return err
	}
	if err := r.runner.Run(ctx, []string{"network", "disconnect", "none", containerID}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("detach Configurator from Docker's none network: %w", err)
	}
	if err := r.runner.Run(ctx, []string{"network", "connect", networkID, containerID}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("attach verified Configurator egress network: %w", err)
	}
	if err := r.runner.Run(ctx, []string{"container", "start", containerID}, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("start Configurator container: %w", err)
	}
	if err := r.confirmConfiguratorNetworkAttachment(ctx, containerID, networkID); err != nil {
		return err
	}
	sessionContext, cancelSession := context.WithCancel(ctx)
	defer cancelSession()
	bridge := newConfiguratorLoginBridge(sessionContext, r, containerID, draft.Agent)
	defer bridge.close()
	browserChannel, err := r.startWorkspaceBrowserChannel(sessionContext, bridge, containerID)
	if err != nil {
		return fmt.Errorf("start Configurator native-login bridge: %w", err)
	}
	defer browserChannel.close()
	agentArgs := []string{"exec", "-i", "-t", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid)}
	for _, environment := range browserChannel.environment() {
		agentArgs = append(agentArgs, "--env", environment)
	}
	// Both pinned agents accept one positional prompt while retaining their
	// native interactive interface. Tobari supplies a fixed conversation opener
	// that asks the isolated Configurator to explain itself before waiting for
	// user input; AGENTS.md remains the complete purpose- and language-aware
	// instruction.
	agentArgs = append(agentArgs, "--workdir", containerWorkdir, containerID, agentExecutable, configuratorPrompt(draft.Task))
	runAgent := func() error { return r.runner.Run(sessionContext, agentArgs, os.Environ(), in, visible, visible) }
	if interactive, ok := r.runner.(interactiveCommandRunner); ok && isTerminalFile(in) && isTerminalFile(visible) {
		runAgent = func() error {
			return interactive.RunInteractive(sessionContext, agentArgs, os.Environ(), in, visible, visible, true)
		}
	}
	if err := runWithAttachedBrowserControl(ctx, cancelSession, browserChannel, runAgent); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return fmt.Errorf("Configurator agent exited unsuccessfully: %w", err)
	}
	return nil
}

func configuratorPrompt(task tobari.ConfiguratorTask) string {
	switch task {
	case tobari.ConfiguratorTaskRuntime:
		return "Begin this Tobari Runtime assistance now. Read and follow AGENTS.md, inspect runtime/source/Dockerfile and the bounded build context, explain the exact source you can edit, then ask the first concrete question about required tools or commands. Do not edit until you understand the user's need."
	case tobari.ConfiguratorTaskPolicy:
		return "Begin this Tobari policy assistance now. Read and follow AGENTS.md, observed.json, and the policy reference material, explain the exact policy source and read-only evidence available, then ask the first concrete question about operations that should be reusable. Do not edit until you understand the user's need."
	default:
		return configuratorInitialPrompt
	}
}

func (r *Runtime) cleanupConfiguratorResource(ctx context.Context, kind string, args []string) error {
	var cleanupErr error
	for attempt := 0; attempt < configuratorCleanupAttempts; attempt++ {
		if err := r.runner.Run(ctx, args, os.Environ(), nil, io.Discard, io.Discard); err == nil {
			return nil
		} else {
			cleanupErr = errors.Join(cleanupErr, err)
		}
		if ctx.Err() != nil {
			break
		}
	}
	return fmt.Errorf("%w: remove Configurator %s by immutable ID after %d bounded attempts: %v", tobari.ErrConfiguratorTransientCleanupUnknown, kind, configuratorCleanupAttempts, cleanupErr)
}

func (r *Runtime) confirmConfiguratorNetwork(ctx context.Context, networkID string) error {
	output, err := r.runner.Output(ctx, []string{"network", "inspect", "--format", `{"id":{{json .Id}},"driver":{{json .Driver}},"internal":{{json .Internal}},"owner":{{json (index .Labels "` + ownerLabel + `")}},"component":{{json (index .Labels "` + componentLabel + `")}}}`, networkID}, os.Environ())
	if err != nil {
		return fmt.Errorf("inspect Configurator egress network: %w", err)
	}
	var observed struct {
		ID        string `json:"id"`
		Driver    string `json:"driver"`
		Internal  bool   `json:"internal"`
		Owner     string `json:"owner"`
		Component string `json:"component"`
	}
	if json.Unmarshal(output, &observed) != nil || observed.ID != networkID || observed.Driver != "bridge" || observed.Internal || observed.Owner != ownerValue || observed.Component != "configurator" {
		return fmt.Errorf("Configurator egress network does not match its owned direct-egress contract")
	}
	return nil
}

func (r *Runtime) confirmConfiguratorContainer(ctx context.Context, containerID, home, opener, image, workdir string) error {
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", `{"id":{{json .Id}},"image_id":{{json .Image}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},"component":{{json (index .Config.Labels "` + componentLabel + `")}},"mounts":{{json .Mounts}},"tmpfs":{{json .HostConfig.Tmpfs}},"network_mode":{{json .HostConfig.NetworkMode}},"read_only":{{json .HostConfig.ReadonlyRootfs}},"privileged":{{json .HostConfig.Privileged}},"cap_add":{{json .HostConfig.CapAdd}},"cap_drop":{{json .HostConfig.CapDrop}},"security_opt":{{json .HostConfig.SecurityOpt}},"pids_limit":{{json .HostConfig.PidsLimit}},"memory":{{json .HostConfig.Memory}},"memory_swap":{{json .HostConfig.MemorySwap}},"nano_cpus":{{json .HostConfig.NanoCpus}},"log_driver":{{json .HostConfig.LogConfig.Type}},"user":{{json .Config.User}},"hostname":{{json .Config.Hostname}},"image":{{json .Config.Image}},"workdir":{{json .Config.WorkingDir}},"tty":{{json .Config.Tty}},"open_stdin":{{json .Config.OpenStdin}},"entrypoint":{{json .Config.Entrypoint}},"cmd":{{json .Config.Cmd}}}`, containerID}, os.Environ())
	if err != nil {
		return fmt.Errorf("inspect Configurator container isolation: %w", err)
	}
	var observed struct {
		ID        string `json:"id"`
		ImageID   string `json:"image_id"`
		Owner     string `json:"owner"`
		Component string `json:"component"`
		Mounts    []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
			RW          bool   `json:"RW"`
		} `json:"mounts"`
		Tmpfs       map[string]string `json:"tmpfs"`
		NetworkMode string            `json:"network_mode"`
		ReadOnly    bool              `json:"read_only"`
		Privileged  bool              `json:"privileged"`
		CapAdd      []string          `json:"cap_add"`
		CapDrop     []string          `json:"cap_drop"`
		SecurityOpt []string          `json:"security_opt"`
		PidsLimit   int64             `json:"pids_limit"`
		Memory      int64             `json:"memory"`
		MemorySwap  int64             `json:"memory_swap"`
		NanoCPUs    int64             `json:"nano_cpus"`
		LogDriver   string            `json:"log_driver"`
		User        string            `json:"user"`
		Hostname    string            `json:"hostname"`
		Image       string            `json:"image"`
		Workdir     string            `json:"workdir"`
		TTY         bool              `json:"tty"`
		OpenStdin   bool              `json:"open_stdin"`
		Entrypoint  []string          `json:"entrypoint"`
		Command     []string          `json:"cmd"`
	}
	if err := json.Unmarshal(output, &observed); err != nil {
		return fmt.Errorf("decode Configurator container isolation: %w", err)
	}
	uid, gid := currentIDs()
	exactSecurityOpt := len(observed.SecurityOpt) == 1 && (observed.SecurityOpt[0] == "no-new-privileges" || observed.SecurityOpt[0] == "no-new-privileges:true")
	if observed.ID != containerID || observed.ImageID != image || observed.Owner != ownerValue || observed.Component != "configurator" || !exactConfiguratorMounts(observed.Mounts, home, opener) || len(observed.Tmpfs) != 2 || observed.Tmpfs["/tmp"] != "size=512m,mode=1777" || observed.Tmpfs["/run"] != "size=16m,mode=1777" || observed.NetworkMode != "none" || !observed.ReadOnly || observed.Privileged || len(observed.CapAdd) != 0 || len(observed.CapDrop) != 1 || observed.CapDrop[0] != "ALL" || !exactSecurityOpt || observed.PidsLimit != 512 || observed.Memory != 2<<30 || observed.MemorySwap != 2<<30 || observed.NanoCPUs != 2_000_000_000 || observed.LogDriver != "none" || observed.User != strconv.Itoa(uid)+":"+strconv.Itoa(gid) || observed.Hostname != "tobari-configurator" || observed.Image != image || observed.Workdir != workdir || !observed.TTY || !observed.OpenStdin || len(observed.Entrypoint) != 1 || observed.Entrypoint[0] != "/usr/bin/tini" || len(observed.Command) != 3 || observed.Command[0] != "--" || observed.Command[1] != "/usr/bin/sleep" || observed.Command[2] != "infinity" {
		return fmt.Errorf("Configurator container isolation differs from the one-mutable-Home native-login contract")
	}
	return nil
}

func (r *Runtime) confirmConfiguratorNetworkAttachment(ctx context.Context, containerID, networkID string) error {
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},"component":{{json (index .Config.Labels "` + componentLabel + `")}},"networks":{{json .NetworkSettings.Networks}}}`, containerID}, os.Environ())
	if err != nil {
		return fmt.Errorf("inspect Configurator network attachment: %w", err)
	}
	var observed struct {
		ID        string `json:"id"`
		Owner     string `json:"owner"`
		Component string `json:"component"`
		Networks  map[string]struct {
			NetworkID string `json:"NetworkID"`
		} `json:"networks"`
	}
	if json.Unmarshal(output, &observed) != nil || observed.ID != containerID || observed.Owner != ownerValue || observed.Component != "configurator" || len(observed.Networks) != 1 {
		return fmt.Errorf("Configurator network attachment differs from its exact owned role")
	}
	for _, network := range observed.Networks {
		if network.NetworkID != networkID {
			return fmt.Errorf("Configurator network attachment changed identity")
		}
	}
	return nil
}

func exactDockerResourceID(output []byte) (string, error) {
	id := strings.TrimSpace(string(output))
	if len(id) != sha256.Size*2 {
		return "", fmt.Errorf("Docker returned a non-canonical resource identity")
	}
	for _, character := range id {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return "", fmt.Errorf("Docker returned a non-canonical resource identity")
		}
	}
	return id, nil
}

func exactConfiguratorMounts(mounts []struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}, home, opener string) bool {
	want := map[string]struct {
		source string
		rw     bool
	}{
		"/var/lib/tobari":          {source: home, rw: true},
		workspaceBrowserOpenerPath: {source: opener, rw: false},
		"/usr/local/bin/xdg-open":  {source: opener, rw: false},
	}
	if len(mounts) != len(want) {
		return false
	}
	for _, mount := range mounts {
		expected, ok := want[mount.Destination]
		if !ok || mount.Type != "bind" || mount.Source != expected.source || mount.RW != expected.rw {
			return false
		}
		delete(want, mount.Destination)
	}
	return len(want) == 0
}

func (r *Runtime) ensureConfiguratorBrowserOpener(ctx context.Context) (string, error) {
	var opener string
	err := r.withRuntimeStoreLock(ctx, func() error {
		version, err := runtimeassets.Version()
		if err != nil {
			return err
		}
		runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
		if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
			return err
		}
		opener = filepath.Join(runtimeDirectory, "browser", "tobari-open")
		info, err := os.Lstat(opener)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
			return fmt.Errorf("Configurator browser opener is unavailable or unsafe: %w", err)
		}
		expected, err := runtimeassets.Read("browser/tobari-open")
		if err != nil {
			return err
		}
		actual, err := os.ReadFile(opener) // #nosec G304 -- exact owner-only embedded asset path.
		if err != nil || !bytes.Equal(actual, expected) {
			return fmt.Errorf("Configurator browser opener differs from this Tobari build: %w", err)
		}
		return nil
	})
	return opener, err
}

func (r *Runtime) AcquireConfiguratorAttachment(ctx context.Context, draft tobari.ConfiguratorDraft) (func() error, error) {
	if err := draft.Validate(); err != nil {
		return nil, err
	}
	if draft.UsesInstallationHome() {
		return r.acquireConfiguratorAttachmentKeys(ctx, "runtime-assist-home-"+string(draft.Agent), "runtime-assist-target-"+draft.TargetRuntimeID)
	}
	contextID := draft.ContextID
	if contextID == "" {
		contextID = draft.AdoptionContextID
	}
	return r.acquireConfiguratorAttachmentKeys(ctx, "context-"+string(contextID), configuratorProjectAttachmentKey(draft.ProjectRoot))
}

// AcquireWorkspaceEntryAttachment uses the same exact Context/Project fence
// as Configurator entry. Its owner retains the returned release function until
// the interactive Workspace session closes.
func (r *Runtime) AcquireWorkspaceEntryAttachment(ctx context.Context, contextID tobari.ContextID, projectRoot string) (func() error, error) {
	if err := contextID.Validate(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return nil, err
	}
	return r.acquireSharedWorkspaceAttachmentKeys(ctx, "context-"+string(contextID), configuratorProjectAttachmentKey(projectRoot))
}

const workspaceSessionAttachmentKey = "workspace-sessions-global"

func (r *Runtime) acquireWorkspaceRetirementAttachment(ctx context.Context, contextID tobari.ContextID, projectRoot string) (func() error, error) {
	if err := contextID.Validate(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return nil, err
	}
	return r.acquireConfiguratorAttachmentKeys(ctx, "context-"+string(contextID), configuratorProjectAttachmentKey(projectRoot))
}

func (r *Runtime) acquireNoWorkspaceSessionsFence(ctx context.Context) (func() error, error) {
	return r.acquireConfiguratorAttachmentKeys(ctx, workspaceSessionAttachmentKey)
}

// AcquireWorkspaceReconciliationFence excludes every already-live Workspace
// borrower while entry repairs Docker-owned runtime material. Context entry
// acquires this only after read-only confirmation misses and releases it before
// Gateway settlement, whose own no-session proof uses the same key.
func (r *Runtime) AcquireWorkspaceReconciliationFence(ctx context.Context) (func() error, error) {
	return r.acquireNoWorkspaceSessionsFence(ctx)
}

func (r *Runtime) acquireWorkspaceSessionAttachment(ctx context.Context) (func() error, error) {
	return r.acquireSharedWorkspaceAttachmentKeys(ctx, workspaceSessionAttachmentKey)
}

func configuratorProjectAttachmentKey(projectRoot string) string {
	digest := sha256.Sum256([]byte(projectRoot))
	return "project-" + hex.EncodeToString(digest[:])
}

func (r *Runtime) acquireConfiguratorAttachmentKeys(ctx context.Context, keys ...string) (func() error, error) {
	return r.acquireAttachmentKeys(ctx, tryLockProjectFile, keys...)
}

func (r *Runtime) acquireSharedWorkspaceAttachmentKeys(ctx context.Context, keys ...string) (func() error, error) {
	return r.acquireAttachmentKeys(ctx, tryLockSharedProjectFile, keys...)
}

func (r *Runtime) acquireAttachmentKeys(ctx context.Context, tryLock func(*os.File) (bool, error), keys ...string) (func() error, error) {
	root, err := r.ConfiguratorRoot()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "attachments")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := os.Chmod(dir, 0o700); err != nil { // #nosec G302 -- exact owner-only metadata directory.
		return nil, err
	}
	if err := requirePrivateDirectory(dir); err != nil {
		return nil, err
	}
	files := make([]*os.File, 0, len(keys))
	release := func() error {
		var releaseErr error
		for index := len(files) - 1; index >= 0; index-- {
			unlockProjectFile(files[index])
			releaseErr = errors.Join(releaseErr, files[index].Close())
		}
		return releaseErr
	}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			_ = release()
			return nil, err
		}
		path := filepath.Join(dir, key+".lock")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- validated fixed-key private Configurator lock.
		if err != nil {
			_ = release()
			return nil, err
		}
		locked, err := tryLock(file)
		if err != nil || !locked {
			_ = file.Close()
			_ = release()
			return nil, errors.Join(tobari.ErrContextBindingProtected, err)
		}
		files = append(files, file)
	}
	return func() error {
		return release()
	}, nil
}

// AcquireTombstonedContextHomeRetirement holds the Context attachment lease
// through source and Home cleanup. A point-in-time confirmation would race a
// new Configurator attachment immediately after the check.
func (r *Runtime) AcquireTombstonedContextHomeRetirement(ctx context.Context, id tobari.ContextID) (func() error, error) {
	if id.Validate() != nil {
		return nil, fmt.Errorf("tombstoned Context Home retirement target is invalid")
	}
	return r.acquireConfiguratorAttachmentKeys(ctx, "context-"+string(id))
}

func (r *Runtime) AcquireContextHomeRetirement(ctx context.Context, id tobari.ContextID) (func() error, error) {
	if id.Validate() != nil {
		return nil, fmt.Errorf("Context Home retirement target is invalid")
	}
	return r.acquireConfiguratorAttachmentKeys(ctx, "context-"+string(id))
}

func (r *Runtime) configuratorHome(draft tobari.ConfiguratorDraft) (string, error) {
	if draft.UsesInstallationHome() {
		root, err := r.ConfiguratorRoot()
		if err != nil {
			return "", err
		}
		return filepath.Join(root, ".runtime-assist-homes", string(draft.Agent), "home"), nil
	}
	if draft.ContextID != "" {
		return r.finalContextHome(draft.ContextID)
	}
	root, err := r.ConfiguratorRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, draft.ID, "home"), nil
}

func validateConfiguratorDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("directory must be owner-only and must not be a symlink")
	}
	return nil
}

func validateConfiguratorFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > 4<<20 {
		return fmt.Errorf("file must be bounded, owner-only, regular, and must not be a symlink")
	}
	return nil
}

func randomConfiguratorContainerName() (string, error) {
	var value [12]byte
	if _, err := io.ReadFull(rand.Reader, value[:]); err != nil {
		return "", err
	}
	return "tobari-configurator-" + hex.EncodeToString(value[:]), nil
}
