// Package dockerruntime implements Tobari through the Docker CLI.
package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	ownerLabel       = "io.tobari.owner"
	ownerValue       = "default"
	componentLabel   = "io.tobari.component"
	tobariIDLabel    = "io.tobari.tobari-id"
	maxLogBytes      = 4 * 1024 * 1024
	defaultLogTail   = 200
	gatewayContainer = "tobari-gateway"
	opaContainer     = "tobari-opa"
)

var clusterContainers = map[string]string{"gateway": gatewayContainer, "opa": opaContainer}

type commandRunner interface {
	Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
	Output(context.Context, []string, []string) ([]byte, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	command := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- executable and argv boundary are fixed.
	command.Env, command.Stdin, command.Stdout, command.Stderr = environment, in, out, errOut
	return command.Run()
}

func (osCommandRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- executable and argv boundary are fixed.
	command.Env = environment
	return command.CombinedOutput()
}

// Runtime owns filesystem state and Docker process execution.
type Runtime struct {
	configDirectory string
	stateDirectory  string
	dataDirectory   string
	runner          commandRunner
	// projectStateWriter is nil in production. Tests may use it to inject a
	// durable-state write failure after Docker reconciliation has completed.
	projectStateWriter func(tobari.ProjectInstance) error
}

// New resolves XDG paths without creating them.
func New() (*Runtime, error) {
	configHome, stateHome, err := resolveRuntimeHomes(
		os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_STATE_HOME"), os.UserHomeDir,
	)
	if err != nil {
		return nil, err
	}
	dataHome, err := resolveDataHome(os.Getenv("XDG_DATA_HOME"), os.UserHomeDir)
	if err != nil {
		return nil, err
	}
	return newRuntimeWithData(
		filepath.Join(configHome, "tobari"), filepath.Join(stateHome, "tobari"),
		filepath.Join(dataHome, "tobari"), osCommandRunner{},
	)
}

func resolveRuntimeHomes(configHome, stateHome string, userHome func() (string, error)) (string, string, error) {
	if configHome != "" && stateHome != "" {
		return configHome, stateHome, nil
	}
	home, err := userHome()
	if err != nil {
		return "", "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	return configHome, stateHome, nil
}

func resolveDataHome(dataHome string, userHome func() (string, error)) (string, error) {
	if dataHome != "" {
		return dataHome, nil
	}
	home, err := userHome()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

func newRuntime(configDirectory, stateDirectory string, runner commandRunner) (*Runtime, error) {
	return newRuntimeWithData(
		configDirectory, stateDirectory,
		filepath.Join(filepath.Dir(stateDirectory), "data", filepath.Base(stateDirectory)), runner,
	)
}

func newRuntimeWithData(configDirectory, stateDirectory, dataDirectory string, runner commandRunner) (*Runtime, error) {
	for name, path := range map[string]string{
		"configuration": configDirectory,
		"state":         stateDirectory,
		"data":          dataDirectory,
	} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("%s directory must be canonical and absolute", name)
		}
	}
	if runner == nil {
		return nil, fmt.Errorf("Docker command runner is required")
	}
	return &Runtime{
		configDirectory: configDirectory,
		stateDirectory:  stateDirectory,
		dataDirectory:   dataDirectory,
		runner:          runner,
	}, nil
}

// ResolveRoot resolves symlinks and requires an existing directory. It is kept
// broad for diagnostics and legacy internal utilities; CWD-owned project
// lifecycle uses ResolveProjectRoot below.
func (r *Runtime) ResolveRoot(ctx context.Context, value string) (string, error) {
	return r.resolveCanonicalRoot(ctx, value)
}

// ResolveProjectRoot resolves a project root and rejects host-management paths
// that must never be read-write mounted into an untrusted Tobari.
func (r *Runtime) ResolveProjectRoot(ctx context.Context, value string) (string, error) {
	resolved, err := r.resolveCanonicalRoot(ctx, value)
	if err != nil {
		return "", err
	}
	if err := r.validateProjectRoot(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (r *Runtime) resolveCanonicalRoot(ctx context.Context, value string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("make root absolute: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func (r *Runtime) validateProjectRoot(root string) error {
	if root == string(filepath.Separator) {
		return fmt.Errorf("filesystem root cannot be a Tobari project root")
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home for project-root protection: %w", err)
	}
	home, err := canonicalPathWithMissing(homeDirectory)
	if err != nil {
		return fmt.Errorf("resolve user home for project-root protection: %w", err)
	}
	if root == home || isPathAncestor(root, home) {
		return fmt.Errorf("user home or its ancestor cannot be a Tobari project root")
	}
	protectedPaths := map[string]string{
		"configuration":     r.configDirectory,
		"state":             r.stateDirectory,
		"data":              r.dataDirectory,
		"docker config":     filepath.Join(home, ".docker"),
		"docker socket":     filepath.Join(string(filepath.Separator), "var", "run", "docker.sock"),
		"docker run socket": filepath.Join(string(filepath.Separator), "run", "docker.sock"),
		"docker data":       filepath.Join(string(filepath.Separator), "var", "lib", "docker"),
		"docker runtime":    filepath.Join(string(filepath.Separator), "var", "run", "docker"),
	}
	for name, candidate := range protectedPaths {
		protected, pathErr := canonicalPathWithMissing(candidate)
		if pathErr != nil {
			return fmt.Errorf("resolve protected %s path: %w", name, pathErr)
		}
		for _, protectedPath := range []string{protected, filepath.Clean(candidate)} {
			if isPathAncestor(root, protectedPath) || isPathAncestor(protectedPath, root) {
				return fmt.Errorf("project root overlaps protected %s path", name)
			}
		}
	}
	return nil
}

func isPathAncestor(ancestor, candidate string) bool {
	return ancestor == candidate || (ancestor != string(filepath.Separator) && strings.HasPrefix(candidate, ancestor+string(filepath.Separator))) || ancestor == string(filepath.Separator)
}

func canonicalPathWithMissing(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	missing := make([]string, 0)
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				if !errors.Is(evalErr, os.ErrNotExist) {
					return "", evalErr
				}
				// A dangling management symlink is still a protected lexical
				// path. Preserve it rather than treating the missing target as
				// a discovery failure.
				resolved = current
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (r *Runtime) CurrentDirectory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return r.ResolveProjectRoot(ctx, current)
}

func (r *Runtime) IsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// ResolveImageSelector applies explicit CLI input before the XDG default.
func (r *Runtime) ResolveImageSelector(ctx context.Context, explicit string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if explicit != "" {
		return explicit, nil
	}
	path := filepath.Join(r.configDirectory, "config.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.BuiltinImageSelector, nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect config.json: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("config.json must be a regular owner-only file")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed config.json child.
	if err != nil {
		return "", fmt.Errorf("read config.json: %w", err)
	}
	if len(data) > 64*1024 {
		return "", fmt.Errorf("config.json exceeds 64 KiB")
	}
	var document struct {
		Version      string `json:"version"`
		DefaultImage string `json:"default_image"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("decode config.json: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("config.json contains trailing data")
	}
	if document.Version != "v1" {
		return "", fmt.Errorf("config.json version must be v1")
	}
	if err := tobari.ValidateImageSelector(document.DefaultImage); err != nil {
		return "", fmt.Errorf("config.json default_image: %w", err)
	}
	return document.DefaultImage, nil
}

// ClusterUp materializes assets and reconciles shared Gateway and OPA.
func (r *Runtime) ClusterUp(ctx context.Context) (tobari.State, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, err
	}
	existing, exists, err := r.LoadState(ctx)
	if err != nil {
		return tobari.State{}, err
	}
	state, err := r.prepareState()
	if err != nil {
		return tobari.State{}, err
	}
	if exists {
		if len(existing.Tobari) != 0 {
			return tobari.State{}, fault.New(
				fault.KindRejected, "legacy_named_state",
				"the shared state contains legacy named Tobari records; remove them with the older binary before continuing", false,
			)
		}
		state.RecentError = existing.RecentError
	}
	if err := r.startClusterReconcile(clusterOperationUp); err != nil {
		return tobari.State{}, fmt.Errorf("start cluster reconcile journal: %w", err)
	}
	if err := r.testPolicy(ctx, state); err != nil {
		_ = r.clearClusterJournal()
		return tobari.State{}, fault.Wrap(fault.KindRejected, "policy_test_failed", "OPA policy tests failed", false, err)
	}
	if err := r.writeState(state); err != nil {
		return tobari.State{}, fmt.Errorf("persist Tobari state: %w", err)
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return tobari.State{}, err
	}
	if err := r.buildTobariImage(ctx, state, environment); err != nil {
		return tobari.State{}, err
	}
	var output bytes.Buffer
	err = r.runner.Run(
		ctx,
		[]string{
			"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"up", "-d", "--build", "--wait", "--remove-orphans",
		},
		environment, nil, &output, &output,
	)
	if err != nil {
		_ = r.recordRecentError(state, "Cluster startup did not complete; inspect component logs.")
		return tobari.State{}, fmt.Errorf("docker compose up: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return tobari.State{}, fmt.Errorf("read CWD-owned projects for Gateway reconciliation: %w", err)
	}
	for _, project := range projects {
		_, network, nameErr := tobari.ProjectResourceNames(project.ID)
		if nameErr != nil {
			return tobari.State{}, nameErr
		}
		exists, existsErr := r.projectResourceExists(ctx, "network", network)
		if existsErr != nil {
			return tobari.State{}, existsErr
		}
		if !exists {
			continue
		}
		if err := r.ensureGatewayNetwork(ctx, network); err != nil {
			_ = r.recordRecentError(state, "Gateway did not rejoin every Tobari network; inspect cluster status.")
			return tobari.State{}, err
		}
	}
	if err := r.markClusterRuntimeReconciled(clusterOperationUp); err != nil {
		return tobari.State{}, fmt.Errorf("mark cluster reconcile complete: %w", err)
	}
	state.RecentError = ""
	if err := r.writeState(state); err != nil {
		return tobari.State{}, err
	}
	if err := r.clearClusterJournal(); err != nil {
		return tobari.State{}, fmt.Errorf("clear cluster reconcile journal: %w", err)
	}
	return state, nil
}

func (r *Runtime) buildTobariImage(ctx context.Context, state tobari.State, environment []string) error {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	uid, gid := currentIDs()
	var output bytes.Buffer
	err = r.runner.Run(
		ctx,
		[]string{
			"build",
			"--build-arg", "DEBIAN_IMAGE=" + versions["DEBIAN_IMAGE"],
			"--build-arg", "TOBARI_UID=" + strconv.Itoa(uid),
			"--build-arg", "TOBARI_GID=" + strconv.Itoa(gid),
			"--tag", tobariImage(state),
			"--tag", "tobari-runtime:local",
			filepath.Join(state.RuntimeDirectory, "tobari"),
		},
		environment, nil, &output, &output,
	)
	if err != nil {
		return fmt.Errorf("build Tobari image: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	return nil
}

func tobariImage(state tobari.State) string {
	return "tobari-runtime:" + state.AssetVersion
}

func (r *Runtime) prepareState() (tobari.State, error) {
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.State{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return tobari.State{}, err
	}
	policyDirectory := filepath.Join(r.configDirectory, "policy")
	credentialDirectory := filepath.Join(r.configDirectory, "credentials")
	credentialConfig := filepath.Join(r.configDirectory, "credentials.json")
	for _, directory := range []string{policyDirectory, credentialDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return tobari.State{}, fmt.Errorf("create configuration directory: %w", err)
		}
		if err := os.Chmod(directory, 0o700); err != nil { // #nosec G302 -- owner traversal requires 0700.
			return tobari.State{}, fmt.Errorf("set configuration directory permissions: %w", err)
		}
	}
	for _, name := range []string{"data.json", "tobari.rego", "tobari_test.rego"} {
		if err := initializeFile(filepath.Join(policyDirectory, name), "opa/policy/"+name, 0o600); err != nil {
			return tobari.State{}, err
		}
	}
	if err := initializeBytes(
		credentialConfig, []byte("{\n  \"version\": \"v1\",\n  \"profiles\": {}\n}\n"), 0o600,
	); err != nil {
		return tobari.State{}, err
	}
	if err := initializeBytes(
		filepath.Join(r.configDirectory, "config.json"),
		[]byte("{\n  \"version\": \"v1\",\n  \"default_image\": \"builtin\"\n}\n"), 0o600,
	); err != nil {
		return tobari.State{}, err
	}
	state := tobari.State{
		SchemaVersion: 2, RuntimeDirectory: runtimeDirectory,
		PolicyDirectory: policyDirectory, CredentialConfig: credentialConfig,
		CredentialDir: credentialDirectory, AssetVersion: version,
		ProxyEndpoint: "http://gateway:8080", Tobari: []tobari.Instance{},
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	return state, nil
}

func initializeFile(target, asset string, mode os.FileMode) error {
	data, err := runtimeassets.Read(asset)
	if err != nil {
		return err
	}
	return initializeBytes(target, data, mode)
}

func initializeBytes(target string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s must be a regular file", filepath.Base(target))
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set configuration file permissions: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect configuration file: %w", err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- fixed child and O_EXCL prevent overwrite.
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	return nil
}

func (r *Runtime) statePath() string { return filepath.Join(r.stateDirectory, "state.json") }

func (r *Runtime) writeState(state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	return r.withClusterLock(func() error {
		if err := os.MkdirAll(r.stateDirectory, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(r.stateDirectory, 0o700); err != nil { // #nosec G302 -- shared state is owner-only.
			return err
		}
		return writeAtomicJSON(r.statePath(), state)
	})
}

func (r *Runtime) withClusterLock(action func() error) error {
	if err := os.MkdirAll(r.stateDirectory, 0o700); err != nil {
		return fmt.Errorf("prepare shared state directory: %w", err)
	}
	if err := os.Chmod(r.stateDirectory, 0o700); err != nil { // #nosec G302 -- shared state is owner-only.
		return fmt.Errorf("protect shared state directory: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "cluster.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("cluster lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect cluster lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed state child after lstat.
	if err != nil {
		return fmt.Errorf("open cluster lock: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect cluster lock: %w", err)
	}
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock shared state: %w", lockErr)
		}
		if acquired {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer unlockProjectFile(file)
	return action()
}

// LoadState returns absence separately from corrupt or legacy state.
func (r *Runtime) LoadState(ctx context.Context) (tobari.State, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, false, err
	}
	info, err := os.Lstat(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProjectStateBytes {
		return tobari.State{}, false, fmt.Errorf("Tobari state file is unsafe")
	}
	data, err := os.ReadFile(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	var header struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return tobari.State{}, false, fmt.Errorf("decode Tobari state header: %w", err)
	}
	if header.SchemaVersion == 1 {
		return tobari.State{}, false, fault.New(
			fault.KindRejected, "legacy_state",
			"schema-1 singleton state must be removed with the older Tobari binary before upgrade", false,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state tobari.State
	if err := decoder.Decode(&state); err != nil {
		return tobari.State{}, false, fmt.Errorf("decode Tobari state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.State{}, false, fmt.Errorf("Tobari state contains trailing data")
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, false, err
	}
	return state, true, nil
}

// Attach creates one exact container, internal network, and persistent home.
func (r *Runtime) Attach(ctx context.Context, state tobari.State, name, root, imageSelector string) (tobari.State, error) {
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.State{}, err
	}
	if err := tobari.ValidateImageSelector(imageSelector); err != nil {
		return tobari.State{}, err
	}
	if resolved, err := r.ResolveRoot(ctx, root); err != nil || resolved != root {
		return tobari.State{}, fmt.Errorf("root must be a canonical existing directory")
	}
	digest := sha256.Sum256([]byte("tobari:" + name))
	id := "tbr_" + hex.EncodeToString(digest[:16])
	instance := tobari.Instance{
		ID: id, Name: name, Root: root, Container: "tobari-" + name,
		Network: "tobari-" + name + "-net", HomeVolume: "tobari-" + name + "-home",
		Image: imageSelector,
	}
	if err := instance.Validate(); err != nil {
		return tobari.State{}, err
	}
	image := imageSelector
	if image == tobari.BuiltinImageSelector {
		image = tobariImage(state)
	}
	if err := r.validateCompatibleImage(ctx, image); err != nil {
		return tobari.State{}, err
	}
	labels := []string{
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=tobari",
		"--label", tobariIDLabel + "=" + instance.ID,
	}
	networkArgs := append([]string{"network", "create", "--internal"}, labels...)
	networkArgs = append(networkArgs, instance.Network)
	if output, err := r.runner.Output(ctx, networkArgs, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("create Tobari network: %w: %s", err, boundedDiagnostic(output))
	}
	volumeArgs := append([]string{"volume", "create"}, labels...)
	volumeArgs = append(volumeArgs, instance.HomeVolume)
	if output, err := r.runner.Output(ctx, volumeArgs, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("create Tobari home: %w: %s", err, boundedDiagnostic(output))
	}
	if err := r.verifyOwnedTobari(ctx, "volume", instance.HomeVolume, instance.ID); err != nil {
		return tobari.State{}, err
	}
	if err := r.ensureGatewayNetwork(ctx, instance.Network); err != nil {
		return tobari.State{}, err
	}
	uid, gid := currentIDs()
	args := []string{
		"create", "--name", instance.Container, "--hostname", instance.Name,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--env", "HOME=/var/lib/tobari",
		"--env", "HTTP_PROXY=http://gateway:8080", "--env", "HTTPS_PROXY=http://gateway:8080",
		"--env", "http_proxy=http://gateway:8080", "--env", "https_proxy=http://gateway:8080",
		"--env", "NO_PROXY=", "--env", "no_proxy=",
		"--env", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
		"--env", "REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
		"--mount", "type=bind,src=" + instance.Root + ",dst=/workspace",
		"--mount", "type=volume,src=" + instance.HomeVolume + ",dst=/var/lib/tobari",
		"--mount", "type=volume,src=tobari-public-ca,dst=/run/tobari/ca-public,readonly",
		"--workdir", "/workspace", "--network", instance.Network,
		"--health-cmd", "test -f /tmp/tobari-ready", "--health-interval", "2s",
		"--health-timeout", "2s", "--health-retries", "30",
	}
	args = append(args, labels...)
	args = append(args, image)
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("create Tobari container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(ctx, []string{"start", instance.Container}, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("start Tobari container: %w: %s", err, boundedDiagnostic(output))
	}
	state.Tobari = append(state.Tobari, instance)
	sort.Slice(state.Tobari, func(i, j int) bool { return state.Tobari[i].Name < state.Tobari[j].Name })
	if err := r.writeState(state); err != nil {
		return tobari.State{}, fmt.Errorf("persist attached Tobari: %w", err)
	}
	return state, nil
}

func (r *Runtime) validateCompatibleImage(ctx context.Context, image string) error {
	output, err := r.runner.Output(
		ctx,
		[]string{
			"image", "inspect", "--format",
			`{"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`,
			image,
		},
		os.Environ(),
	)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "image_not_found",
			"selected Tobari image is not available locally; build or pull it explicitly", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Read the compatible image contract."},
		)
	}
	var configuration struct {
		API        string   `json:"api"`
		User       string   `json:"user"`
		Entrypoint []string `json:"entrypoint"`
	}
	expectedEntrypoint := []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}
	if err := json.Unmarshal(bytes.TrimSpace(output), &configuration); err != nil ||
		configuration.API != tobari.RuntimeImageAPI ||
		configuration.User != "tobari" ||
		!equalStrings(configuration.Entrypoint, expectedEntrypoint) {
		return fault.New(
			fault.KindRejected, "incompatible_image",
			"selected image does not preserve the supported Tobari runtime API, user, and entrypoint", false,
			fault.NextAction{Command: "help tobari", Reason: "Extend the documented Tobari runtime base."},
		)
	}
	return nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (r *Runtime) ensureGatewayNetwork(ctx context.Context, network string) error {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", gatewayContainer},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("inspect Gateway networks: %w: %s", err, boundedDiagnostic(output))
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return fmt.Errorf("decode Gateway networks: %w", err)
	}
	if _, connected := networks[network]; connected {
		return nil
	}
	output, err = r.runner.Output(
		ctx,
		[]string{"network", "connect", "--alias", "gateway", network, gatewayContainer},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("connect Gateway to Tobari network: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

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
	components := make([]tobari.ComponentStatus, 0, 2)
	running := true
	for _, name := range []string{"gateway", "opa"} {
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
	return tobari.ClusterStatus{
		Configured: true, Running: running, Proxy: state.ProxyEndpoint,
		Policy: state.PolicyDirectory, TobariCount: len(projects),
		Components: components, RecentError: state.RecentError,
	}, nil
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

// InspectTobari observes each exact state-owned container.
func (r *Runtime) InspectTobari(ctx context.Context, state tobari.State) ([]tobari.ItemStatus, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	items := make([]tobari.ItemStatus, 0, len(state.Tobari))
	for _, instance := range state.Tobari {
		component, err := r.inspectContainer(ctx, instance.Name, instance.Container)
		if err != nil {
			return nil, err
		}
		items = append(items, tobari.ItemStatus{
			ID: instance.ID, Name: instance.Name, Root: instance.Root,
			Image:     instance.ImageSelector(),
			Running:   component.State == "running" && (component.Health == "healthy" || component.Health == "none"),
			Container: instance.Container,
		})
	}
	return items, nil
}

// Exec invokes Docker with exact argv and preserves the child exit code.
func (r *Runtime) Exec(
	ctx context.Context, instance tobari.Instance, request tobari.ExecRequest,
	in io.Reader, out, errOut io.Writer,
) (int, error) {
	if err := instance.Validate(); err != nil {
		return 0, err
	}
	if err := request.Validate(); err != nil {
		return 0, err
	}
	cwd := request.HostCWD
	if cwd == "" {
		cwd = instance.Root
	}
	resolved, err := r.ResolveRoot(ctx, cwd)
	if err != nil {
		return 0, err
	}
	containerCWD, err := tobari.MapHostCWD(instance.Root, resolved)
	if err != nil {
		if request.CWDExplicit {
			return 0, err
		}
		containerCWD = "/workspace"
	}
	args := []string{"exec", "-i"}
	if request.TTY {
		args = append(args, "-t")
	}
	uid, gid := currentIDs()
	args = append(
		args, "--user", strconv.Itoa(uid)+":"+strconv.Itoa(gid),
		"--workdir", containerCWD, instance.Container,
	)
	args = append(args, request.Command...)
	err = r.runner.Run(ctx, args, os.Environ(), in, out, errOut)
	if err == nil {
		return 0, nil
	}
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

func (r *Runtime) ClusterLogs(ctx context.Context, state tobari.State, request tobari.LogRequest) ([]byte, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	if err := request.ValidateCluster(); err != nil {
		return nil, err
	}
	names := []string{request.Component}
	if request.Component == "all" {
		names = []string{"gateway", "opa"}
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

func (r *Runtime) TobariLogs(ctx context.Context, instance tobari.Instance, request tobari.LogRequest) ([]byte, error) {
	if err := instance.Validate(); err != nil {
		return nil, err
	}
	if err := request.ValidateTobari(); err != nil {
		return nil, err
	}
	data, err := r.runner.Output(
		ctx, []string{"logs", "--tail", strconv.Itoa(request.Tail), instance.Container}, os.Environ(),
	)
	if err != nil {
		return nil, fmt.Errorf("read Tobari logs: %w", err)
	}
	if len(data) > maxLogBytes {
		return nil, fmt.Errorf("log output exceeds %d bytes", maxLogBytes)
	}
	return data, nil
}

// Detach removes one exact container and network, preserving home by default.
func (r *Runtime) Detach(
	ctx context.Context, state tobari.State, instance tobari.Instance, purge bool,
) (tobari.State, error) {
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	stored, found := state.Find(instance.ID)
	if !found || stored != instance {
		return tobari.State{}, fmt.Errorf("Tobari target does not match persisted state")
	}
	if err := r.verifyOwnedTobari(ctx, "container", instance.Container, instance.ID); err != nil {
		return tobari.State{}, err
	}
	if output, err := r.runner.Output(ctx, []string{"rm", "-f", instance.Container}, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("remove Tobari container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(
		ctx, []string{"network", "disconnect", instance.Network, gatewayContainer}, os.Environ(),
	); err != nil {
		return tobari.State{}, fmt.Errorf("disconnect Gateway from Tobari network: %w: %s", err, boundedDiagnostic(output))
	}
	if err := r.verifyOwnedTobari(ctx, "network", instance.Network, instance.ID); err != nil {
		return tobari.State{}, err
	}
	if output, err := r.runner.Output(ctx, []string{"network", "rm", instance.Network}, os.Environ()); err != nil {
		return tobari.State{}, fmt.Errorf("remove Tobari network: %w: %s", err, boundedDiagnostic(output))
	}
	if purge {
		if err := r.verifyOwnedTobari(ctx, "volume", instance.HomeVolume, instance.ID); err != nil {
			return tobari.State{}, err
		}
		if output, err := r.runner.Output(ctx, []string{"volume", "rm", instance.HomeVolume}, os.Environ()); err != nil {
			return tobari.State{}, fmt.Errorf("remove Tobari home: %w: %s", err, boundedDiagnostic(output))
		}
	}
	remaining := make([]tobari.Instance, 0, len(state.Tobari)-1)
	for _, candidate := range state.Tobari {
		if candidate.ID != instance.ID {
			remaining = append(remaining, candidate)
		}
	}
	state.Tobari = remaining
	if err := r.writeState(state); err != nil {
		return tobari.State{}, fmt.Errorf("persist detached Tobari: %w", err)
	}
	return state, nil
}

// ClusterDown removes exact shared resources after application-level emptiness validation.
func (r *Runtime) ClusterDown(ctx context.Context, state tobari.State, purge bool) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if len(state.Tobari) != 0 {
		return fmt.Errorf("cluster contains attached Tobari")
	}
	if err := r.startClusterReconcile(clusterOperationDown); err != nil {
		return fmt.Errorf("start cluster reconcile journal: %w", err)
	}
	for _, container := range clusterContainers {
		if err := r.verifyOwned(ctx, "container", container); err != nil {
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
	if purge {
		for _, volume := range []string{"tobari-gateway-ca", "tobari-public-ca"} {
			if err := r.verifyOwned(ctx, "volume", volume); err != nil {
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

func (r *Runtime) verifyOwned(ctx context.Context, kind, name string) error {
	args := []string{"inspect", "--format", `{{index .Config.Labels "` + ownerLabel + `"}}`, name}
	if kind == "volume" {
		args = []string{"volume", "inspect", "--format", `{{index .Labels "` + ownerLabel + `"}}`, name}
	}
	if kind == "network" {
		args = []string{"network", "inspect", "--format", `{{index .Labels "` + ownerLabel + `"}}`, name}
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil {
		return nil
	}
	if strings.TrimSpace(string(output)) != ownerValue {
		return fmt.Errorf("%s %s is not owned by Tobari", kind, name)
	}
	return nil
}

func (r *Runtime) verifyOwnedTobari(ctx context.Context, kind, name, id string) error {
	if err := r.verifyOwned(ctx, kind, name); err != nil {
		return err
	}
	var args []string
	switch kind {
	case "container":
		args = []string{"inspect", "--format", `{{index .Config.Labels "` + tobariIDLabel + `"}}`, name}
	case "volume":
		args = []string{"volume", "inspect", "--format", `{{index .Labels "` + tobariIDLabel + `"}}`, name}
	case "network":
		args = []string{"network", "inspect", "--format", `{{index .Labels "` + tobariIDLabel + `"}}`, name}
	default:
		return fmt.Errorf("unsupported resource kind %s", kind)
	}
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(output)) != id {
		return fmt.Errorf("%s %s does not belong to the selected Tobari", kind, name)
	}
	return nil
}

func (r *Runtime) testPolicy(ctx context.Context, state tobari.State) error {
	return r.testPolicyDirectory(ctx, state.PolicyDirectory)
}

func (r *Runtime) testPolicyDirectory(ctx context.Context, policyDirectory string) error {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	uid, gid := currentIDs()
	mount := "type=bind,src=" + policyDirectory + ",dst=/policy,readonly"
	output, err := r.runner.Output(
		ctx,
		[]string{
			"run", "--rm", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
			"--mount", mount, versions["OPA_IMAGE"], "test", "/policy",
		},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("%w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) composeEnvironment(state tobari.State) ([]string, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return nil, fmt.Errorf("read embedded runtime versions: %w", err)
	}
	uid, gid := currentIDs()
	environment := append([]string{}, os.Environ()...)
	environment = append(
		environment,
		"TOBARI_POLICY_DIR="+state.PolicyDirectory,
		"TOBARI_CREDENTIAL_CONFIG="+state.CredentialConfig,
		"TOBARI_CREDENTIAL_DIR="+state.CredentialDir,
		"TOBARI_ASSET_VERSION="+state.AssetVersion,
		"TOBARI_UID="+strconv.Itoa(uid), "TOBARI_GID="+strconv.Itoa(gid),
		"TOBARI_MITMPROXY_IMAGE="+versions["MITMPROXY_IMAGE"],
		"TOBARI_OPA_IMAGE="+versions["OPA_IMAGE"],
		"TOBARI_DEBIAN_IMAGE="+versions["DEBIAN_IMAGE"],
	)
	return environment, nil
}

func (r *Runtime) recordRecentError(state tobari.State, message string) error {
	state.RecentError = message
	return r.writeState(state)
}

// Doctor reports locally testable prerequisites without repairing them.
func (r *Runtime) Doctor(ctx context.Context, root string) (doctor.Report, error) {
	checks := make([]doctor.Check, 0, 14)
	add := func(name string, status doctor.CheckStatus, detail string) {
		checks = append(checks, doctor.Check{Name: name, Status: status, Detail: detail})
	}
	if _, err := exec.LookPath("docker"); err != nil {
		add("docker_cli", doctor.CheckStatusFail, "docker was not found on PATH")
		return doctor.Report{Checks: checks}, nil
	}
	add("docker_cli", doctor.CheckStatusPass, "docker is available")
	if output, err := r.runner.Output(ctx, []string{"version", "--format", "{{.Server.Version}}"}, os.Environ()); err != nil {
		add("docker_engine", doctor.CheckStatusFail, "Docker Engine is unavailable")
	} else {
		add("docker_engine", doctor.CheckStatusPass, strings.TrimSpace(string(output)))
	}
	if output, err := r.runner.Output(ctx, []string{"context", "show"}, os.Environ()); err != nil {
		add("docker_context", doctor.CheckStatusFail, "Docker context could not be read")
	} else {
		add("docker_context", doctor.CheckStatusPass, strings.TrimSpace(string(output)))
	}
	if output, err := r.runner.Output(ctx, []string{"compose", "version", "--short"}, os.Environ()); err != nil {
		add("docker_compose", doctor.CheckStatusFail, "Docker Compose v2 is unavailable")
	} else {
		add("docker_compose", doctor.CheckStatusPass, strings.TrimSpace(string(output)))
	}
	add("proxy_port", doctor.CheckStatusPass, "Gateway has no host-published port")
	if root != "" {
		if resolved, err := r.ResolveRoot(ctx, root); err != nil {
			add("root", doctor.CheckStatusFail, err.Error())
		} else {
			add("root", doctor.CheckStatusPass, resolved)
			add("root_sharing", doctor.CheckStatusWarn, "path is valid; Docker VM bind sharing is confirmed by attach")
		}
	} else {
		add("root", doctor.CheckStatusWarn, "no root was supplied")
		add("root_sharing", doctor.CheckStatusWarn, "no root was supplied")
	}
	state, exists, stateErr := r.LoadState(ctx)
	if stateErr != nil {
		add("state", doctor.CheckStatusFail, "Tobari state is invalid")
	} else if exists {
		projects, projectErr := r.ListProjects(ctx)
		if projectErr != nil {
			add("state", doctor.CheckStatusFail, "CWD-owned Tobari state is invalid")
		} else {
			add("state", doctor.CheckStatusPass, fmt.Sprintf("cluster has %d CWD-owned Tobari", len(projects)))
		}
		if err := r.testPolicy(ctx, state); err != nil {
			add(
				"policy", doctor.CheckStatusFail,
				"OPA policy tests failed; verify syntax and Docker access to the XDG policy directory",
			)
		} else {
			add("policy", doctor.CheckStatusPass, "OPA policy tests passed")
		}
	} else {
		add("state", doctor.CheckStatusWarn, "cluster is not configured")
		add("policy", doctor.CheckStatusWarn, "policy will be initialized by cluster up")
	}
	if err := r.checkCredentialPermissions(); err != nil {
		add("credentials", doctor.CheckStatusFail, err.Error())
	} else {
		add("credentials", doctor.CheckStatusPass, "credential files have safe Unix modes")
	}
	if detail, status := r.checkCredentialConfig(); status != doctor.CheckStatusPass {
		add("credential_config", status, detail)
	} else {
		add("credential_config", doctor.CheckStatusPass, detail)
	}
	if _, err := r.ResolveImageSelector(ctx, ""); err != nil {
		add("image_config", doctor.CheckStatusFail, err.Error())
	} else {
		add("image_config", doctor.CheckStatusPass, "default image configuration is valid")
	}
	output, err := r.runner.Output(
		ctx, []string{"ps", "-a", "--filter", "label=" + ownerLabel + "=" + ownerValue, "--format", "{{.Names}}"}, os.Environ(),
	)
	if err != nil {
		add("owned_resources", doctor.CheckStatusWarn, "owned Docker resources could not be listed")
	} else if strings.TrimSpace(string(output)) == "" {
		add("owned_resources", doctor.CheckStatusPass, "no residual containers")
	} else {
		add("owned_resources", doctor.CheckStatusWarn, "owned containers exist: "+strings.Join(strings.Fields(string(output)), ","))
	}
	return doctor.Report{Checks: checks}, nil
}

func (r *Runtime) checkCredentialConfig() (string, doctor.CheckStatus) {
	path := filepath.Join(r.configDirectory, "credentials.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "configuration will be initialized by cluster up", doctor.CheckStatusWarn
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "credentials.json must be a regular owner-only file", doctor.CheckStatusFail
	}
	data, err := os.ReadFile(path) // #nosec G304 -- fixed credentials.json child.
	if err != nil || len(data) > 256*1024 {
		return "credentials.json is unreadable or exceeds 256 KiB", doctor.CheckStatusFail
	}
	var document struct {
		Version  string `json:"version"`
		Profiles map[string]struct {
			Type       string   `json:"type"`
			Hosts      []string `json:"hosts"`
			SecretFile string   `json:"secret_file"`
			Header     string   `json:"header"`
		} `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.Version != "v1" || document.Profiles == nil {
		return "credentials.json does not match schema v1", doctor.CheckStatusFail
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "credentials.json contains trailing data", doctor.CheckStatusFail
	}
	for name, profile := range document.Profiles {
		secretName := strings.TrimPrefix(profile.SecretFile, "/run/tobari/credentials/")
		if name == "" || (profile.Type != "bearer" && profile.Type != "header") ||
			len(profile.Hosts) == 0 ||
			!strings.HasPrefix(profile.SecretFile, "/run/tobari/credentials/") ||
			secretName == "" || secretName == "." || secretName == ".." || strings.Contains(secretName, "/") {
			return "credentials.json contains an invalid profile", doctor.CheckStatusFail
		}
		for _, host := range profile.Hosts {
			if host == "" || host != strings.ToLower(strings.TrimSuffix(host, ".")) {
				return "credentials.json contains an invalid host binding", doctor.CheckStatusFail
			}
		}
		if profile.Type == "header" && profile.Header == "" {
			return "header credential profile requires a header name", doctor.CheckStatusFail
		}
	}
	return "credential profile metadata matches schema v1", doctor.CheckStatusPass
}

func (r *Runtime) checkCredentialPermissions() error {
	entries, err := os.ReadDir(filepath.Join(r.configDirectory, "credentials"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("credential file %s is a symbolic link", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("credential file %s must be regular and owner-only", entry.Name())
		}
	}
	return nil
}

func boundedDiagnostic(data []byte) string {
	const maximum = 4096
	data = bytes.TrimSpace(data)
	if len(data) > maximum {
		data = data[:maximum]
	}
	return string(data)
}

func DefaultLogTail() int { return defaultLogTail }
