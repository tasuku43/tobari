// Package dockerruntime implements Tobari through the Docker CLI.
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
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const (
	ownerLabel               = "io.tobari.owner"
	ownerValue               = "default"
	componentLabel           = "io.tobari.component"
	tobariIDLabel            = "io.tobari.tobari-id"
	maxLogBytes              = 4 * 1024 * 1024
	defaultLogTail           = 200
	gatewayContainer         = "tobari-gateway"
	opaContainer             = "tobari-opa"
	policyBundleVolume       = "tobari-policy-bundle"
	authBrokerContainer      = "tobari-auth-broker"
	policyTestFailureMessage = "OPA policy tests failed; check Rego syntax and ensure the XDG policy directory is accessible to the Docker Engine VM"
)

var clusterContainers = map[string]string{
	"auth-broker": authBrokerContainer,
	"gateway":     gatewayContainer,
	"opa":         opaContainer,
}

var clusterComponentOrder = []string{"auth-broker", "gateway", "opa"}

var errOwnedResourceMissing = errors.New("owned Docker resource is missing")

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
	configDirectory    string
	stateDirectory     string
	dataDirectory      string
	runner             commandRunner
	images             imageResolver
	browser            hostBrowserOpener
	gitIdentity        hostGitIdentityResolver
	rootKeyLoader      func(context.Context) ([]byte, error)
	hostCLIs           hostCLIResolver
	credentialHost     hostCredentialAcquirer
	policyProjectionMu sync.Mutex
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
		browser:         osHostBrowserOpener{},
		gitIdentity:     newOSHostGitIdentityResolver(),
		hostCLIs:        newPathHostCLIResolver(),
		credentialHost:  newOSHostCredentialAcquirer(),
	}, nil
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
	// This port reports the canonical working directory. Operations that create
	// or select a project root enforce the stricter project-root policy at their
	// own boundary; read-only list presentation must also work outside a valid
	// project root.
	return r.resolveCanonicalRoot(ctx, current)
}

func (r *Runtime) IsTerminal(writer io.Writer) bool {
	return isTerminalFile(writer)
}

func (r *Runtime) IsInputTerminal(reader io.Reader) bool {
	return isTerminalFile(reader)
}

func isTerminalFile(value interface{}) bool {
	return terminal.IsTerminal(value)
}

// ResolveImageSelector applies explicit CLI input before the XDG default.
func (r *Runtime) ResolveImageSelector(ctx context.Context, explicit string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if explicit != "" {
		return r.resolveBuiltinImageSelector(explicit), nil
	}
	if _, err := os.Lstat(r.activeContextPath()); err == nil {
		name, activeErr := r.readActiveContext()
		if activeErr != nil {
			return "", activeErr
		}
		manifest, manifestErr := r.readContextManifestRaw(name)
		if manifestErr != nil {
			return "", manifestErr
		}
		return r.resolveBuiltinImageSelector(manifest.Image), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect active Context: %w", err)
	}
	return r.defaultRuntimeImage(), nil
}

// ClusterUp materializes assets and reconciles the shared Gateway, OPA, and
// Auth Broker.
func (r *Runtime) ClusterUp(ctx context.Context) (tobari.State, error) {
	return r.ClusterUpWithProgress(ctx, nil)
}

// ClusterUpWithProgress materializes assets and reconciles the shared Gateway,
// OPA, and Auth Broker while emitting only fixed, secret-free lifecycle signals.
func (r *Runtime) ClusterUpWithProgress(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) (tobari.State, error) {
	return r.clusterUpWithProgressMode(ctx, progress, false)
}

// ValidateClusterBuildIdentity rejects a compiled resolver/API mismatch before
// the application enters its lifecycle mutation boundary.
func (r *Runtime) ValidateClusterBuildIdentity(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.validateResolverCompatibility()
}

func (r *Runtime) clusterUpWithProgressMode(
	ctx context.Context, progress tobari.ClusterUpProgressSink, forceRecreate bool,
) (tobari.State, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, err
	}
	if err := r.validateResolverCompatibility(); err != nil {
		return tobari.State{}, err
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressStarted,
	})
	existing, exists, err := r.LoadState(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	authBrokerSelection, err := r.selectAuthBrokerImage(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	state, err := r.prepareState(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	if exists {
		state.RecentError = existing.RecentError
	}
	recordAttemptError := func(message string) {
		if exists {
			_ = r.recordRecentError(existing, message)
		}
	}
	activationAttempted := false
	activationCommitted := false
	var gatewayImage string
	defer func() {
		if !activationAttempted || activationCommitted || !exists || existing.AggregateRevision == state.AggregateRevision {
			return
		}
		rollbackContext, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		rollbackEnvironment, environmentErr := r.composeEnvironment(existing)
		if environmentErr != nil {
			return
		}
		rollbackEnvironment = replaceEnvironmentValue(
			rollbackEnvironment, "TOBARI_AUTH_BROKER_IMAGE", authBrokerSelection.Image,
		)
		rollbackEnvironment = replaceEnvironmentValue(
			rollbackEnvironment, "TOBARI_GATEWAY_IMAGE", gatewayImage,
		)
		_ = r.runner.Run(
			rollbackContext,
			[]string{"compose", "--project-directory", existing.RuntimeDirectory,
				"-f", filepath.Join(existing.RuntimeDirectory, "compose.yaml"),
				"up", "-d", "--no-build", "--remove-orphans", "--force-recreate", "--wait"},
			rollbackEnvironment, nil, io.Discard, io.Discard,
		)
		_ = r.ensureGatewayNetworkGuard(rollbackContext)
	}()
	var environment []string
	environment, err = r.composeEnvironment(state)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	if err = r.verifyAuthBrokerImage(ctx, authBrokerSelection.Image, authBrokerSelection.RequireDigest); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE", authBrokerSelection.Image)
	gatewayImage, err = r.prepareGatewayImage(ctx)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, err
	}
	environment = replaceEnvironmentValue(environment, "TOBARI_GATEWAY_IMAGE", gatewayImage)
	if err := r.startClusterReconcile(clusterOperationUp); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.State{}, fmt.Errorf("start cluster reconcile journal: %w", err)
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressCompleted,
	})

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPolicy, func() error {
		if err := r.testPolicy(ctx, state); err != nil {
			_ = r.clearClusterJournal()
			return fault.Wrap(fault.KindRejected, "policy_test_failed", policyTestFailureMessage, false, err)
		}
		return r.preparePolicyBundle(ctx, state)
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressPrepareImages, func() error {
		return r.prepareContextImages(ctx)
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressStartServices, func() error {
		activationAttempted = true
		var output bytes.Buffer
		composeUpArgs := []string{"compose", "--project-directory", state.RuntimeDirectory,
			"-f", filepath.Join(state.RuntimeDirectory, "compose.yaml"),
			"up", "-d", "--no-build", "--remove-orphans"}
		if forceRecreate {
			composeUpArgs = append(composeUpArgs, "--force-recreate")
		}
		err := r.runner.Run(
			ctx,
			composeUpArgs,
			environment, nil, &output, &output,
		)
		if err != nil {
			recordAttemptError("Cluster startup did not complete; inspect component logs.")
			return fmt.Errorf("docker compose up: %w: %s", err, boundedDiagnostic(output.Bytes()))
		}
		if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
			recordAttemptError("Gateway network guard did not become ready; inspect Docker kernel support.")
			return err
		}
		rootKey, err := r.unlockAuthBroker(ctx)
		if err != nil {
			recordAttemptError("Auth Broker did not unlock; inspect root-key and broker state.")
			return err
		}
		defer clear(rootKey)
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressConnectNetworks, func() error {
		for _, sharedNetwork := range []string{"tobari-control", "tobari-egress"} {
			if err := r.ensureGatewayNetwork(ctx, sharedNetwork); err != nil {
				recordAttemptError("Gateway did not rejoin the shared cluster network; inspect cluster status.")
				return err
			}
			if err := r.ensureAuthBrokerNetwork(ctx, sharedNetwork); err != nil {
				recordAttemptError("Auth Broker did not rejoin the shared cluster network; inspect cluster status.")
				return err
			}
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressWaitForHealth, func() error {
		if err := r.waitForClusterReady(ctx, progress); err != nil {
			recordAttemptError("Cluster components did not become healthy; inspect component status.")
			return err
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressReconcileProjects, func() error {
		projects, err := r.ListProjects(ctx)
		if err != nil {
			return fmt.Errorf("read CWD-owned projects for Gateway reconciliation: %w", err)
		}
		if err := r.syncProjectPrincipalRegistry(ctx, projects); err != nil {
			recordAttemptError("Gateway did not rejoin every Tobari network; inspect cluster status.")
			return err
		}
		return nil
	}); err != nil {
		return tobari.State{}, err
	}

	if err := runClusterUpProgressStep(progress, tobari.ClusterUpProgressFinalize, func() error {
		if err := r.markClusterRuntimeReconciled(clusterOperationUp); err != nil {
			return fmt.Errorf("mark cluster reconcile complete: %w", err)
		}
		state.RecentError = ""
		if err := r.writeState(state); err != nil {
			return err
		}
		if err := r.clearClusterJournal(); err != nil {
			return fmt.Errorf("clear cluster reconcile journal: %w", err)
		}
		activationCommitted = true
		return nil
	}); err != nil {
		return tobari.State{}, err
	}
	return state, nil
}

func runClusterUpProgressStep(
	progress tobari.ClusterUpProgressSink, step tobari.ClusterUpProgressStep, action func() error,
) error {
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressStarted})
	if err := action(); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressFailed})
		return err
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressCompleted})
	return nil
}

func emitClusterUpProgress(
	progress tobari.ClusterUpProgressSink, event tobari.ClusterUpProgress,
) {
	if progress == nil {
		return
	}
	if err := event.Validate(); err != nil {
		return
	}
	progress(event)
}

func (r *Runtime) waitForClusterReady(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) error {
	const attempts = 60
	for attempt := 0; attempt < attempts; attempt++ {
		ready := true
		statuses := make([]tobari.ComponentStatus, 0, len(clusterContainers))
		for _, name := range clusterComponentOrder {
			component, err := r.inspectContainer(ctx, name, clusterContainers[name])
			if err != nil {
				return err
			}
			statuses = append(statuses, component)
			if component.State != "running" || component.Health != "healthy" {
				ready = false
			}
		}
		if brokerState, err := r.brokerState(ctx); err != nil || brokerState != "ready" {
			ready = false
		}
		if ready {
			return nil
		}
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressUpdated,
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("cluster components did not become healthy")
}

func (r *Runtime) prepareContextImages(ctx context.Context) error {
	list, err := r.ListContexts(ctx)
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		manifest, _, err := r.resolveContext(item.Name)
		if err != nil {
			return err
		}
		image := r.resolveBuiltinImageSelector(manifest.Image)
		if r.imageResolver().ShouldPullRuntimeImage(image) {
			if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
				return err
			}
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return fmt.Errorf("Context %q runtime image: %w", manifest.Name, err)
		}
	}
	return nil
}

// prepareActiveContextImage preserves the focused single-Context image check;
// shared cluster reconciliation calls prepareContextImages instead.
func (r *Runtime) prepareActiveContextImage(ctx context.Context) error {
	manifest, _, err := r.activeContext()
	if err != nil {
		return err
	}
	image := r.resolveBuiltinImageSelector(manifest.Image)
	if r.imageResolver().ShouldPullRuntimeImage(image) {
		if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
			return err
		}
	}
	return r.validateCompatibleImage(ctx, image)
}

func (r *Runtime) pullOfficialRuntimeImage(ctx context.Context, image string) error {
	var output bytes.Buffer
	err := r.runner.Run(
		ctx,
		[]string{"image", "pull", image},
		os.Environ(), nil, &output, &output,
	)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "runtime_image_unavailable",
			"official Tobari runtime image is not available; inspect Docker registry access before startup", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker registry access and the selected Context image."},
		)
	}
	return nil
}

func (r *Runtime) resolveBuiltinImageSelector(image string) string {
	if image == tobari.BuiltinImageSelector {
		return r.defaultRuntimeImage()
	}
	return image
}

func (r *Runtime) prepareState(ctx context.Context) (tobari.State, error) {
	for name, path := range map[string]string{
		"configuration": r.configDirectory,
		"state":         r.stateDirectory,
		"data":          r.dataDirectory,
	} {
		if err := r.ensurePrivateDirectory(path); err != nil {
			return tobari.State{}, fmt.Errorf("prepare %s directory: %w", name, err)
		}
	}
	if _, err := r.prepareAuthProjection(); err != nil {
		return tobari.State{}, fmt.Errorf("prepare Auth Broker provider projection: %w", err)
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.State{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return tobari.State{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.State{}, fmt.Errorf("prepare Context catalog: %w", err)
	}
	if err := r.withPolicyProjectionLock(ctx, func() error {
		return r.recoverAllPolicySourceTransactions(ctx)
	}); err != nil {
		return tobari.State{}, fmt.Errorf("recover interrupted Context policy source transaction: %w", err)
	}
	projection, err := r.buildAggregateProjection(ctx)
	if err != nil {
		return tobari.State{}, fmt.Errorf("prepare aggregate Context projection: %w", err)
	}
	if err := r.ensureProjectPrincipalRegistry(ctx); err != nil {
		return tobari.State{}, fmt.Errorf("validate project principal registry: %w", err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: runtimeDirectory,
		AggregateRevision: projection.Revision, ContextCount: projection.ContextCount,
		PolicyDirectory: projection.PolicyDirectory, CredentialConfig: projection.CredentialConfig,
		CredentialDir: projection.CredentialDirectory, AssetVersion: version,
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
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return fmt.Errorf("prepare shared state directory: %w", err)
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

// LoadState returns absence separately from corrupt state.
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

func (r *Runtime) validateCompatibleImage(ctx context.Context, image string) error {
	output, err := r.runner.Output(
		ctx,
		[]string{
			"image", "inspect", "--format",
			`{"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`,
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
		Lifetime   string   `json:"lifetime"`
		User       string   `json:"user"`
		Entrypoint []string `json:"entrypoint"`
	}
	expectedEntrypoint := []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}
	if err := json.Unmarshal(bytes.TrimSpace(output), &configuration); err != nil ||
		configuration.API != tobari.RuntimeImageAPI ||
		configuration.Lifetime != tobari.RuntimeImageLifetimeCommand ||
		configuration.User != "tobari" ||
		!equalStrings(configuration.Entrypoint, expectedEntrypoint) {
		return fault.New(
			fault.KindRejected, "incompatible_image",
			"selected image does not preserve the supported Tobari runtime API, lifetime command, user, and entrypoint", false,
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
	return r.ensureClusterContainerNetwork(ctx, gatewayContainer, "Gateway", "gateway", network)
}

func (r *Runtime) ensureAuthBrokerNetwork(ctx context.Context, network string) error {
	return r.ensureClusterContainerNetwork(ctx, authBrokerContainer, "Auth Broker", "auth-broker", network)
}

func (r *Runtime) ensureClusterContainerNetwork(
	ctx context.Context,
	container string,
	component string,
	alias string,
	network string,
) error {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", container},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("inspect %s networks: %w: %s", component, err, boundedDiagnostic(output))
	}
	var networks map[string]json.RawMessage
	if err := json.Unmarshal(bytes.TrimSpace(output), &networks); err != nil {
		return fmt.Errorf("decode %s networks: %w", component, err)
	}
	if _, connected := networks[network]; connected {
		return nil
	}
	output, err = r.runner.Output(
		ctx,
		[]string{"network", "connect", "--alias", alias, network, container},
		os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("connect %s to Tobari network: %w: %s", component, err, boundedDiagnostic(output))
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
	credentialIntegrity := r.inspectCredentialProjectionIntegrity(state)
	brokerState, brokerErr := r.brokerState(ctx)
	if brokerErr != nil {
		brokerState = "unavailable"
	}
	if brokerState != "ready" {
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
		PrincipalRegistry: principalIntegrity, CredentialProjection: credentialIntegrity,
		AuthProviderProjection: r.inspectAuthProviderProjectionIntegrity(),
		AuthBrokerState:        string(brokerState),
		RootKeyBackend:         backend,
		Components:             components, RecentError: state.RecentError,
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

func (r *Runtime) inspectCredentialProjectionIntegrity(state tobari.State) string {
	if _, status := r.checkCredentialConfigAt(state.CredentialConfig); status != doctor.CheckStatusPass {
		return "invalid"
	}
	if err := requirePrivateDirectory(state.CredentialDir); err != nil {
		return "invalid"
	}
	data, err := readOwnerPolicyFile(state.CredentialConfig, 256*1024)
	if err != nil {
		return "invalid"
	}
	var document struct {
		Contexts map[string]struct {
			Profiles map[string]struct {
				SecretFile string `json:"secret_file"`
			} `json:"profiles"`
		} `json:"contexts"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return "invalid"
	}
	const prefix = "/run/tobari/credentials/"
	for _, projected := range document.Contexts {
		for _, profile := range projected.Profiles {
			relative := strings.TrimPrefix(profile.SecretFile, prefix)
			if relative == profile.SecretFile {
				return "invalid"
			}
			if _, err := readOwnerPolicyFile(filepath.Join(state.CredentialDir, filepath.FromSlash(relative)), 64*1024); err != nil {
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
		if isMissingDockerResource(err, output) {
			return errOwnedResourceMissing
		}
		return fmt.Errorf("inspect %s %s ownership: %w: %s", kind, name, err, boundedDiagnostic(output))
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
		"TOBARI_PRINCIPAL_DIR="+r.principalRegistryDirectory(),
		"TOBARI_AUTH_PROVIDER_CONFIG="+r.authProviderProjectionPath(),
		"TOBARI_AUTH_CONTEXTS_DIR="+r.authContextsDirectory(),
		"TOBARI_AUTH_RUNTIME_DIR="+r.authRuntimeDirectory(),
		"TOBARI_ASSET_VERSION="+state.AssetVersion,
		"TOBARI_UID="+strconv.Itoa(uid), "TOBARI_GID="+strconv.Itoa(gid),
		"TOBARI_MITMPROXY_IMAGE="+versions["MITMPROXY_IMAGE"],
		"TOBARI_GATEWAY_IMAGE="+versions["GATEWAY_IMAGE"],
		"TOBARI_AUTH_BROKER_IMAGE="+versions["AUTH_BROKER_IMAGE"],
		"TOBARI_OPA_IMAGE="+versions["OPA_IMAGE"],
		"TOBARI_DEBIAN_IMAGE="+versions["DEBIAN_IMAGE"],
	)
	return environment, nil
}

func replaceEnvironmentValue(environment []string, key, value string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, prefix+value)
}

func (r *Runtime) recordRecentError(state tobari.State, message string) error {
	state.RecentError = message
	return r.writeState(state)
}

func (r *Runtime) addPolicyDataDiagnostic(
	ctx context.Context, add func(string, doctor.CheckStatus, string), policyDirectory string,
) {
	if strings.HasPrefix(policyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		contexts, err := r.readAggregateContexts(ctx)
		if err != nil || len(contexts) == 0 {
			add("policy_data", doctor.CheckStatusFail, "Context policy data is invalid or unsafe: "+fmt.Sprint(err))
			return
		}
		add("policy_data", doctor.CheckStatusPass, fmt.Sprintf("learned policy data is safe across %d Contexts", len(contexts)))
		return
	}
	if _, err := readPolicyData(policyDirectory); err != nil {
		add(
			"policy_data", doctor.CheckStatusFail,
			"learned policy data is invalid or unsafe; inspect the active Context policy data: "+err.Error(),
		)
		return
	}
	add("policy_data", doctor.CheckStatusPass, "learned policy data is safe for guided review")
}

func (r *Runtime) checkCredentialConfig() (string, doctor.CheckStatus) {
	paths, err := r.diagnosticContextStores()
	if err != nil {
		return "active Context could not be inspected", doctor.CheckStatusFail
	}
	return r.checkCredentialConfigAt(paths.CredentialConfig)
}

func (r *Runtime) checkCredentialConfigAt(path string) (string, doctor.CheckStatus) {
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
	type credentialProfile struct {
		Type       string   `json:"type"`
		Hosts      []string `json:"hosts"`
		Projects   []string `json:"projects"`
		SecretFile string   `json:"secret_file"`
		Header     string   `json:"header"`
	}
	validateProfiles := func(profiles map[string]credentialProfile, contextID string) (string, doctor.CheckStatus) {
		for name, profile := range profiles {
			secretName := strings.TrimPrefix(profile.SecretFile, "/run/tobari/credentials/")
			if contextID != "" {
				secretName = strings.TrimPrefix(secretName, contextID+"/")
			}
			if name == "" || (profile.Type != "bearer" && profile.Type != "header") ||
				len(profile.Hosts) == 0 ||
				profile.Projects == nil ||
				!strings.HasPrefix(profile.SecretFile, "/run/tobari/credentials/") ||
				(contextID != "" && !strings.HasPrefix(profile.SecretFile, "/run/tobari/credentials/"+contextID+"/")) ||
				secretName == "" || secretName == "." || secretName == ".." || strings.Contains(secretName, "/") {
				return "credentials.json contains an invalid profile", doctor.CheckStatusFail
			}
			seenProjects := make(map[string]struct{}, len(profile.Projects))
			for _, projectID := range profile.Projects {
				if err := tobari.ValidateProjectID(projectID); err != nil {
					return "credentials.json contains an invalid project binding", doctor.CheckStatusFail
				}
				if _, exists := seenProjects[projectID]; exists {
					return "credentials.json contains duplicate project bindings", doctor.CheckStatusFail
				}
				seenProjects[projectID] = struct{}{}
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
		return "", doctor.CheckStatusPass
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		return "credentials.json is not valid JSON", doctor.CheckStatusFail
	}
	var version string
	if raw, ok := shape["version"]; !ok || json.Unmarshal(raw, &version) != nil || version != "v1" {
		return "credentials.json must use schema V1", doctor.CheckStatusFail
	}
	_, hasProfiles := shape["profiles"]
	_, hasContexts := shape["contexts"]
	switch {
	case hasProfiles && !hasContexts:
		var document struct {
			Version  string                       `json:"version"`
			Profiles map[string]credentialProfile `json:"profiles"`
		}
		if err := decodeStrictJSON(data, &document); err != nil || document.Version != "v1" || document.Profiles == nil {
			return "credentials.json does not match Context credential schema V1", doctor.CheckStatusFail
		}
		if detail, status := validateProfiles(document.Profiles, ""); status != doctor.CheckStatusPass {
			return detail, status
		}
		return "credential profile metadata matches Context credential schema V1", doctor.CheckStatusPass
	case hasContexts && !hasProfiles:
		var document struct {
			Version  string `json:"version"`
			Contexts map[string]struct {
				Name             string                       `json:"name"`
				Profiles         map[string]credentialProfile `json:"profiles"`
				GraphQLEndpoints []tobari.GraphQLEndpoint     `json:"graphql_endpoints"`
			} `json:"contexts"`
		}
		if err := decodeStrictJSON(data, &document); err != nil || document.Version != "v1" || document.Contexts == nil {
			return "credentials.json does not match aggregate credential schema V1", doctor.CheckStatusFail
		}
		for contextID, projected := range document.Contexts {
			if err := tobari.ValidateContextID(contextID); err != nil || tobari.ValidateName(projected.Name) != nil || projected.Profiles == nil {
				return "credentials.json contains an invalid Context projection", doctor.CheckStatusFail
			}
			seenEndpoints := make(map[tobari.GraphQLEndpoint]struct{}, len(projected.GraphQLEndpoints))
			for _, endpoint := range projected.GraphQLEndpoints {
				if err := endpoint.Validate(); err != nil {
					return "credentials.json contains an invalid GraphQL endpoint projection", doctor.CheckStatusFail
				}
				if _, duplicate := seenEndpoints[endpoint]; duplicate {
					return "credentials.json contains duplicate GraphQL endpoint projections", doctor.CheckStatusFail
				}
				seenEndpoints[endpoint] = struct{}{}
			}
			if detail, status := validateProfiles(projected.Profiles, contextID); status != doctor.CheckStatusPass {
				return detail, status
			}
		}
		return "credential profile metadata matches aggregate credential schema V1", doctor.CheckStatusPass
	default:
		return "credentials.json does not match a current credential schema", doctor.CheckStatusFail
	}
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("JSON contains trailing data")
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
