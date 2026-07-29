// Package dockerruntime implements the Tobari runtime through the Docker CLI.
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
	"time"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/realm"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	ownerLabel     = "io.tobari.owner"
	ownerValue     = "default"
	maxLogBytes    = 4 * 1024 * 1024
	defaultLogTail = 200
)

var componentContainers = map[string]string{
	"gateway": "tobari-gateway",
	"opa":     "tobari-opa",
	"realm":   "tobari-realm",
}

type commandRunner interface {
	Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
	Output(context.Context, []string, []string) ([]byte, error)
}

type osCommandRunner struct{}

func (osCommandRunner) Run(
	ctx context.Context,
	args, environment []string,
	in io.Reader,
	out, errOut io.Writer,
) error {
	command := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- executable is fixed and every value is passed as one exact argv element, never through a shell.
	command.Env = environment
	command.Stdin = in
	command.Stdout = out
	command.Stderr = errOut
	return command.Run()
}

func (osCommandRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	command := exec.CommandContext(ctx, "docker", args...) // #nosec G204 -- executable is fixed and every value is passed as one exact argv element, never through a shell.
	command.Env = environment
	return command.CombinedOutput()
}

// Runtime owns filesystem state and Docker process execution.
type Runtime struct {
	configDirectory string
	stateDirectory  string
	runner          commandRunner
}

// New resolves XDG paths without creating them.
func New() (*Runtime, error) {
	configHome, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user configuration directory: %w", err)
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf("resolve user state directory: %w", homeErr)
		}
		stateHome = filepath.Join(home, ".local", "state")
	}
	return newRuntime(
		filepath.Join(configHome, "tobari"),
		filepath.Join(stateHome, "tobari"),
		osCommandRunner{},
	)
}

func newRuntime(configDirectory, stateDirectory string, runner commandRunner) (*Runtime, error) {
	for name, path := range map[string]string{"configuration": configDirectory, "state": stateDirectory} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("%s directory must be canonical and absolute", name)
		}
	}
	if runner == nil {
		return nil, fmt.Errorf("Docker command runner is required")
	}
	return &Runtime{configDirectory: configDirectory, stateDirectory: stateDirectory, runner: runner}, nil
}

// ResolveRoot resolves symlinks and requires an existing directory.
func (r *Runtime) ResolveRoot(ctx context.Context, value string) (string, error) {
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

// CurrentDirectory returns the canonical host working directory.
func (r *Runtime) CurrentDirectory(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	current, err := os.Getwd()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// IsTerminal reports whether writer is an attached character device.
func (r *Runtime) IsTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Up materializes assets and reconciles the three-container runtime.
func (r *Runtime) Up(ctx context.Context, root string) (realm.State, error) {
	if err := ctx.Err(); err != nil {
		return realm.State{}, err
	}
	reloadPolicy := false
	if existing, exists, err := r.LoadState(ctx); err != nil {
		return realm.State{}, err
	} else if exists && existing.Root != root {
		return realm.State{}, fault.New(
			fault.KindInvalidInput,
			"root_conflict",
			"a Tobari realm is already configured for another root",
			false,
		)
	} else {
		reloadPolicy = exists
	}
	state, err := r.prepareState(root)
	if err != nil {
		return realm.State{}, err
	}
	if err := r.testPolicy(ctx, state); err != nil {
		return realm.State{}, fault.Wrap(
			fault.KindRejected,
			"policy_test_failed",
			"OPA policy tests failed",
			false,
			err,
		)
	}
	if err := r.writeState(state); err != nil {
		return realm.State{}, fmt.Errorf("persist realm state: %w", err)
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return realm.State{}, err
	}
	var output bytes.Buffer
	if err := r.runner.Run(
		ctx,
		[]string{
			"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"up", "-d", "--build", "--wait", "--remove-orphans",
		},
		environment,
		nil,
		&output,
		&output,
	); err != nil {
		if stateErr := r.recordRecentError(state, "Realm startup did not complete; inspect component logs."); stateErr != nil {
			return realm.State{}, fmt.Errorf("docker compose up failed and recent error could not be persisted: %w", stateErr)
		}
		return realm.State{}, fmt.Errorf("docker compose up: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if reloadPolicy {
		if err := r.restartOPA(ctx); err != nil {
			if stateErr := r.recordRecentError(state, "OPA policy reload did not complete; inspect OPA logs."); stateErr != nil {
				return realm.State{}, fmt.Errorf("OPA reload failed and recent error could not be persisted: %w", stateErr)
			}
			return realm.State{}, err
		}
	}
	return state, nil
}

func (r *Runtime) recordRecentError(state realm.State, message string) error {
	state.RecentError = message
	return r.writeState(state)
}

func (r *Runtime) restartOPA(ctx context.Context) error {
	output, err := r.runner.Output(ctx, []string{"restart", "tobari-opa"}, os.Environ())
	if err != nil {
		return fmt.Errorf("restart OPA after policy reconciliation: %w: %s", err, boundedDiagnostic(output))
	}
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		health, inspectErr := r.runner.Output(
			ctx,
			[]string{"inspect", "--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}", "tobari-opa"},
			os.Environ(),
		)
		if inspectErr == nil && strings.TrimSpace(string(health)) == "healthy" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("OPA did not become healthy after policy reconciliation")
		case <-ticker.C:
		}
	}
}

func (r *Runtime) prepareState(root string) (realm.State, error) {
	version, err := runtimeassets.Version()
	if err != nil {
		return realm.State{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return realm.State{}, err
	}
	policyDirectory := filepath.Join(r.configDirectory, "policy")
	credentialDirectory := filepath.Join(r.configDirectory, "credentials")
	credentialConfig := filepath.Join(r.configDirectory, "credentials.json")
	if err := os.MkdirAll(policyDirectory, 0o700); err != nil {
		return realm.State{}, fmt.Errorf("create policy directory: %w", err)
	}
	if err := os.Chmod(policyDirectory, 0o700); err != nil { // #nosec G302 -- this is a private directory; owner traversal requires 0700 rather than a regular file's 0600.
		return realm.State{}, fmt.Errorf("set policy directory permissions: %w", err)
	}
	if err := os.MkdirAll(credentialDirectory, 0o700); err != nil {
		return realm.State{}, fmt.Errorf("create credential directory: %w", err)
	}
	for _, name := range []string{"data.json", "tobari.rego", "tobari_test.rego"} {
		if err := initializeFile(
			filepath.Join(policyDirectory, name),
			"opa/policy/"+name,
			0o600,
		); err != nil {
			return realm.State{}, err
		}
	}
	if err := initializeBytes(
		credentialConfig,
		[]byte("{\n  \"version\": \"v1\",\n  \"profiles\": {}\n}\n"),
		0o600,
	); err != nil {
		return realm.State{}, err
	}
	state := realm.State{
		SchemaVersion: 1, Root: root, RuntimeDirectory: runtimeDirectory,
		PolicyDirectory: policyDirectory, CredentialConfig: credentialConfig,
		CredentialDir: credentialDirectory, AssetVersion: version,
		ProxyEndpoint: "http://tobari-gateway:8080",
	}
	if err := state.Validate(); err != nil {
		return realm.State{}, err
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
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- target is one fixed Tobari configuration child and O_EXCL prevents overwrite or final-component symlink traversal.
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

func (r *Runtime) statePath() string {
	return filepath.Join(r.stateDirectory, "state.json")
}

func (r *Runtime) writeState(state realm.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(r.stateDirectory, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := r.statePath() + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, r.statePath()); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

// LoadState returns absence separately from corrupt state.
func (r *Runtime) LoadState(ctx context.Context) (realm.State, bool, error) {
	if err := ctx.Err(); err != nil {
		return realm.State{}, false, err
	}
	data, err := os.ReadFile(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return realm.State{}, false, nil
	}
	if err != nil {
		return realm.State{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state realm.State
	if err := decoder.Decode(&state); err != nil {
		return realm.State{}, false, fmt.Errorf("decode realm state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return realm.State{}, false, fmt.Errorf("realm state contains trailing data")
	}
	if err := state.Validate(); err != nil {
		return realm.State{}, false, err
	}
	return state, true, nil
}

// Inspect observes exact container state.
func (r *Runtime) Inspect(ctx context.Context, state realm.State) (realm.Status, error) {
	if err := state.Validate(); err != nil {
		return realm.Status{}, err
	}
	if _, err := r.runner.Output(ctx, []string{"version", "--format", "{{.Server.Version}}"}, os.Environ()); err != nil {
		return realm.Status{}, fmt.Errorf("Docker Engine is unavailable: %w", err)
	}
	components := make([]realm.ComponentStatus, 0, len(componentContainers))
	running := true
	for _, name := range []string{"gateway", "opa", "realm"} {
		container := componentContainers[name]
		output, err := r.runner.Output(
			ctx,
			[]string{
				"inspect", "--format",
				`{"state":"{{.State.Status}}","health":"{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}"}`,
				container,
			},
			os.Environ(),
		)
		component := realm.ComponentStatus{Name: name, State: "absent", Health: "none"}
		if err == nil {
			var observed struct {
				State  string `json:"state"`
				Health string `json:"health"`
			}
			if decodeErr := json.Unmarshal(bytes.TrimSpace(output), &observed); decodeErr != nil {
				return realm.Status{}, fmt.Errorf("decode Docker status for %s: %w", name, decodeErr)
			}
			component.State, component.Health = observed.State, observed.Health
		}
		if component.State != "running" || (component.Health != "healthy" && component.Health != "none") {
			running = false
		}
		components = append(components, component)
	}
	return realm.Status{
		Configured: true, Running: running, Root: state.Root,
		Proxy: state.ProxyEndpoint, Policy: state.PolicyDirectory,
		Components: components, RecentError: state.RecentError,
	}, nil
}

// Exec invokes Docker with exact argv and preserves the child exit code.
func (r *Runtime) Exec(
	ctx context.Context,
	state realm.State,
	request realm.ExecRequest,
	in io.Reader,
	out, errOut io.Writer,
) (int, error) {
	if err := state.Validate(); err != nil {
		return 0, err
	}
	if err := request.Validate(); err != nil {
		return 0, err
	}
	cwd := request.HostCWD
	if cwd == "" {
		cwd = state.Root
	}
	resolved, err := r.ResolveRoot(ctx, cwd)
	if err != nil {
		return 0, err
	}
	containerCWD, err := realm.MapHostCWD(state.Root, resolved)
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
	args = append(args, "--user", "tobari", "--workdir", containerCWD, "tobari-realm")
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

// Logs returns a bounded window from exact component containers.
func (r *Runtime) Logs(ctx context.Context, state realm.State, request realm.LogRequest) ([]byte, error) {
	if err := state.Validate(); err != nil {
		return nil, err
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	names := []string{request.Component}
	if request.Component == "all" {
		names = []string{"gateway", "opa", "realm"}
	}
	var output bytes.Buffer
	for _, name := range names {
		container := componentContainers[name]
		data, err := r.runner.Output(
			ctx,
			[]string{"logs", "--tail", strconv.Itoa(request.Tail), container},
			os.Environ(),
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

// Down removes exact compose resources and optionally persistent volumes.
func (r *Runtime) Down(ctx context.Context, state realm.State, purge bool) error {
	if err := state.Validate(); err != nil {
		return err
	}
	for _, container := range componentContainers {
		if err := r.verifyOwned(ctx, "container", container); err != nil {
			return err
		}
	}
	environment, err := r.composeEnvironment(state)
	if err != nil {
		return err
	}
	var output bytes.Buffer
	if err := r.runner.Run(
		ctx,
		[]string{
			"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"down", "--remove-orphans",
		},
		environment,
		nil,
		&output,
		&output,
	); err != nil {
		if stateErr := r.recordRecentError(state, "Realm cleanup did not complete; inspect component logs."); stateErr != nil {
			return fmt.Errorf("docker compose down failed and recent error could not be persisted: %w", stateErr)
		}
		return fmt.Errorf("docker compose down: %w: %s", err, boundedDiagnostic(output.Bytes()))
	}
	if purge {
		for _, volume := range []string{"tobari-realm-home", "tobari-gateway-ca", "tobari-public-ca"} {
			if err := r.verifyOwned(ctx, "volume", volume); err != nil {
				return err
			}
			if _, err := r.runner.Output(ctx, []string{"volume", "rm", volume}, os.Environ()); err != nil {
				return fmt.Errorf("remove owned volume %s: %w", volume, err)
			}
		}
	}
	if err := os.Remove(r.statePath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove realm state: %w", err)
	}
	return nil
}

func (r *Runtime) verifyOwned(ctx context.Context, kind, name string) error {
	args := []string{"inspect", "--format", `{{index .Config.Labels "` + ownerLabel + `"}}`, name}
	if kind == "volume" {
		args = []string{"volume", "inspect", "--format", `{{index .Labels "` + ownerLabel + `"}}`, name}
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

func (r *Runtime) testPolicy(ctx context.Context, state realm.State) error {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	uid, gid := currentIDs()
	mount := "type=bind,src=" + state.PolicyDirectory + ",dst=/policy,readonly"
	output, err := r.runner.Output(
		ctx,
		[]string{
			"run", "--rm",
			"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
			"--mount", mount,
			versions["OPA_IMAGE"], "test", "/policy",
		},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("%w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) composeEnvironment(state realm.State) ([]string, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return nil, fmt.Errorf("read embedded runtime versions: %w", err)
	}
	uid, gid := currentIDs()
	environment := append([]string{}, os.Environ()...)
	environment = append(
		environment,
		"TOBARI_ROOT="+state.Root,
		"TOBARI_POLICY_DIR="+state.PolicyDirectory,
		"TOBARI_CREDENTIAL_CONFIG="+state.CredentialConfig,
		"TOBARI_CREDENTIAL_DIR="+state.CredentialDir,
		"TOBARI_ASSET_VERSION="+state.AssetVersion,
		"TOBARI_UID="+strconv.Itoa(uid),
		"TOBARI_GID="+strconv.Itoa(gid),
		"TOBARI_MITMPROXY_IMAGE="+versions["MITMPROXY_IMAGE"],
		"TOBARI_OPA_IMAGE="+versions["OPA_IMAGE"],
		"TOBARI_DEBIAN_IMAGE="+versions["DEBIAN_IMAGE"],
	)
	return environment, nil
}

// Doctor reports all locally testable prerequisites without repairing them.
func (r *Runtime) Doctor(ctx context.Context, root string) (doctor.Report, error) {
	checks := make([]doctor.Check, 0, 13)
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
	add("proxy_port", doctor.CheckStatusPass, "Gateway has no host-published port, so there is no host port conflict")
	if root != "" {
		if resolved, err := r.ResolveRoot(ctx, root); err != nil {
			add("root", doctor.CheckStatusFail, err.Error())
		} else {
			add("root", doctor.CheckStatusPass, resolved)
			add("root_sharing", doctor.CheckStatusWarn, "path is valid; Docker VM bind sharing is confirmed by up")
		}
	} else {
		add("root", doctor.CheckStatusWarn, "no root was supplied")
		add("root_sharing", doctor.CheckStatusWarn, "no root was supplied")
	}
	state, exists, stateErr := r.LoadState(ctx)
	if stateErr != nil {
		add("state", doctor.CheckStatusFail, "realm state is invalid")
	} else if exists {
		add("state", doctor.CheckStatusPass, "realm state is configured")
		if err := r.testPolicy(ctx, state); err != nil {
			add("policy", doctor.CheckStatusFail, "OPA policy tests failed")
		} else {
			add("policy", doctor.CheckStatusPass, "OPA policy tests passed")
		}
	} else {
		add("state", doctor.CheckStatusWarn, "realm is not configured")
		add("policy", doctor.CheckStatusWarn, "policy will be initialized by up")
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
	output, err := r.runner.Output(
		ctx,
		[]string{"ps", "-a", "--filter", "label=" + ownerLabel + "=" + ownerValue, "--format", "{{.Names}}"},
		os.Environ(),
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
		return "configuration will be initialized by up", doctor.CheckStatusWarn
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return "credentials.json must be a regular owner-only file", doctor.CheckStatusFail
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed credentials.json child of Tobari's user configuration directory.
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
			secretName == "" || secretName == "." || secretName == ".." ||
			strings.Contains(secretName, "/") {
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

// DefaultLogTail is shared with the CLI catalog default.
func DefaultLogTail() int {
	return defaultLogTail
}
