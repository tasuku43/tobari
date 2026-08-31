// Package dockerruntime implements Tobari through the Docker CLI.
package dockerruntime

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/companionruntime"
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
	authRuntimeVolume        = "tobari-auth-runtime"
	authBrokerContainer      = "tobari-auth-broker"
	policyTestFailureMessage = "built-in policy checks failed; reconcile the typed policy data and owned Docker policy bundle"
)

func finalRetainedVolumeNames() []string {
	return []string{"tobari-gateway-ca", policyBundleVolume, "tobari-public-ca"}
}

func finalFreshVolumeNames() []string {
	volumes := finalRetainedVolumeNames()
	if brokerRuntimeEnabled {
		volumes = append([]string{authRuntimeVolume}, volumes...)
	}
	return volumes
}

var errOwnedResourceMissing = errors.New("owned Docker resource is missing")

type commandRunner interface {
	Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
	Output(context.Context, []string, []string) ([]byte, error)
}

// workspaceBrowserControlRunner opts a runner into the concurrent, long-lived
// Docker exec used only for attachment-scoped browser requests. Keeping this
// separate from commandRunner prevents ordinary test and inspection runners
// from accidentally being treated as interactive control transports.
type workspaceBrowserControlRunner interface {
	RunWorkspaceBrowserControl(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
}

type workspaceServiceControlRunner interface {
	RunWorkspaceServiceControl(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
	RunWorkspaceServiceStream(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
}

type workspacePermissionControlRunner interface {
	RunWorkspacePermissionControl(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error
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

func (runner osCommandRunner) RunWorkspaceBrowserControl(
	ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer,
) error {
	return runner.Run(ctx, args, environment, in, out, errOut)
}

func (runner osCommandRunner) RunWorkspaceServiceControl(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	return runner.Run(ctx, args, environment, in, out, errOut)
}

func (runner osCommandRunner) RunWorkspaceServiceStream(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	return runner.Run(ctx, args, environment, in, out, errOut)
}

func (runner osCommandRunner) RunWorkspacePermissionControl(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	return runner.Run(ctx, args, environment, in, out, errOut)
}

// Runtime owns filesystem state and Docker process execution.
type Runtime struct {
	lifetimeContext             context.Context
	configDirectory             string
	stateDirectory              string
	dataDirectory               string
	shortTemporaryDirectory     string
	hostHomeDirectory           string
	runner                      commandRunner
	images                      imageResolver
	browser                     hostBrowserOpener
	serviceBrowser              serviceBrowserDispatcher
	gitIdentity                 hostGitIdentityResolver
	companion                   companionruntime.Launcher
	companionEntropy            io.Reader
	rootKeyLoader               func(context.Context) ([]byte, error)
	hostCLIs                    hostCLIResolver
	credentialHost              hostCredentialAcquirer
	lifecycleLockAttempt        func()
	runtimeStoreLockAttempt     func()
	runtimeBuildCleanup         func(runtimeBuildJournal) error
	runtimeBuildCompletionWrite func(runtimeBuildJournal) error
	runtimeBuildJournalRemove   func(string) error
	runtimeBuildJournalWrite    func(runtimeBuildJournal, runtimeBuildJournal) error
	runtimeBuildManifestWrite   func(string, any) error
	runtimeBuildRehash          func(context.Context, string) (string, error)
	runtimeBuildRehashBoundary  func(string, bool) error
	runtimeBuildSnapshotSync    func(string) error
	runtimeBuildDirectorySync   func(string) error
	runtimeBuildRename          func(string, string) error
	runtimeBuildFreeze          func(string) error
	runtimeBuildSnapshotRemove  func(string) error
	runtimeBuildRecoveryTimeout time.Duration
	// configuratorRuntimeAfterPromotion is nil in production. Focused tests
	// mutate canonical editable source after promotion to prove the build stays
	// bound to the immutable frozen Configurator snapshot.
	configuratorRuntimeAfterPromotion    func()
	runtimePruneJournalWrite             func(*runtimePruneJournal, runtimePruneJournal) error
	runtimePruneReceiptWrite             func(runtimePruneReceiptStore) error
	runtimePruneJournalRemove            func(string) error
	runtimePruneBeforeRemove             func(tobari.RuntimePruneCandidate) error
	runtimePruneAfterBuildCleanup        func(tobari.RuntimePruneCandidate) error
	runtimeDeleteJournalWrite            func(*runtimeDeleteJournal, runtimeDeleteJournal) error
	runtimeDeleteReceiptWrite            func(tobari.RuntimeDeleteResult) error
	runtimeDeleteJournalRemove           func(string) error
	runtimeDeleteBeforeImageRemove       func(tobari.RuntimePruneCandidate) error
	runtimeDeleteBeforeQuarantine        func(string, string) error
	runtimeDeleteRename                  func(string, string) error
	runtimeDeleteQuarantineRemove        func(string) error
	runtimeCreateBoundary                func(string) error
	runtimeCreateFileSync                func(string) error
	runtimeInstallationMigrationBoundary func(string) error
	// claudeContainerLogin is nil in production. Tests may replace the
	// isolated Context-runtime acquisition without granting the generic host
	// credential adapter authority over Claude's native state.
	claudeContainerLogin func(context.Context, string, io.Reader, io.Writer) (hostCredentialPayload, error)
	// pupContainerLogin and pupRelayFactory are nil in production. Tests may
	// replace the isolated Context-runtime acquisition and loopback adapter.
	pupContainerLogin func(context.Context, string, io.Reader, io.Writer) (hostCredentialPayload, error)
	pupRelayFactory   pupLoginRelayFactory
	hostLoginProfiles hostLoginProfileReader
	identities        identityIssuer
	// permissionIngestionTransport is selected by the trusted host binary from
	// its closed support profile. Tests may select a member directly; no public
	// input or runtime probe can change it.
	permissionIngestionTransport tobari.PermissionSessionTransport
	policyProjectionMu           sync.Mutex
	// attachmentGrantVisibility is nil in production. Focused tests replace
	// only the bounded Gateway bind-mount visibility confirmation.
	attachmentGrantVisibility func(context.Context, []byte) error
	// projectStateWriter is nil in production. Tests may use it to inject a
	// durable-state write failure after Docker reconciliation has completed.
	projectStateWriter func(tobari.Workspace) error
	// clusterStateWriteHook is nil in production. Tests use it to distinguish
	// failures before and after the atomic shared-state publication boundary.
	clusterStateWriteHook   func(tobari.State, func() error) error
	clusterJournalClearHook func() error
	// finalSessionAfterLiveness is nil in production. Tests use it to move the
	// principal projection between the two complete Run-start observations.
	finalSessionAfterLiveness func()
	// finalProjectionAfterFirstObservation is nil in production. Tests use it
	// to move Docker network/principal evidence between the two complete dormant
	// final-authority observations.
	finalProjectionAfterFirstObservation func()
	// finalProjectionBeforeEffect is nil in production. Tests use it to move
	// Docker evidence after artifact construction but before the final complete
	// observation fence that authorizes an activation effect.
	finalProjectionBeforeEffect func()
	// finalPolicyRootSync is nil in production. Tests replace the first-use
	// activation-root parent durability barrier without weakening production
	// directory fsync behavior.
	finalPolicyRootSync func(string) error
	// policyBeforeBundleAssembly is nil in production. Focused publication
	// tests use it to inject a host-data replacement between the first complete
	// aggregate verification and the byte-authoritative bundle assembly.
	policyBeforeBundleAssembly func()
	// finalPolicyActivationLimit is zero in production. Tests lower the exact
	// task-owned journal/receipt byte ceiling to prove rejection precedes OPA.
	finalPolicyActivationLimit int64
	// finalPolicyAfterApply is nil in production. Tests use it to interrupt the
	// dormant final-authority activation after OPA confirmed the exact revision
	// but before the private active receipt is published.
	finalPolicyAfterApply func() error
	// finalGatewayAfterFirstSessionFence is nil in production. Tests use it to
	// publish a canonical attachment between the two global zero-owner fences.
	finalGatewayAfterFirstSessionFence func()
	// finalGatewayAfterEffect is nil in production. Tests interrupt exact
	// Gateway-settlement effect boundaries while leaving the durable decision
	// available to the same initiating action.
	finalGatewayAfterEffect func(string) error
	// finalGatewayReplaceComponents is nil in production. Lifecycle tests use it
	// to model one exact Compose replacement while retaining the concrete
	// settlement journal, receipt publication, and recovery phases.
	finalGatewayReplaceComponents func(context.Context, finalGatewaySettlementCandidate) error
	// The final Workspace hooks are nil in production. Focused tests replace
	// only the exact WP03 material and Docker effect/observation boundaries;
	// filesystem decisions and durable receipts remain the concrete code path.
	finalWorkspaceRuntimeMaterial          func(context.Context, tobari.RuntimeBinding) (tobari.RuntimeBinding, string, string, error)
	finalWorkspaceDockerReconcile          func(context.Context, tobari.WorkspaceEntryReconciliationPlan, finalWorkspaceRuntimeSpec) (string, error)
	finalWorkspaceDockerObserve            func(context.Context, tobari.WorkspaceEntryReconciliationPlan) (finalWorkspaceContainerObservation, error)
	finalWorkspaceAfterContainerRetirement func() error
	finalWorkspaceAfterHomeRetirement      func() error
	// finalRuntimeProtectionSource is injected by the host composition root.
	// It exposes only a domain-validated final-owner projection; dockerruntime
	// never receives or rediscovers the final Store path.
	finalRuntimeProtectionSource FinalRuntimeProtectionSource
}

// ResourceSourceRoot returns Tobari's exact XDG configuration root. It is
// consumed only by the concept-separated source adapter at composition.
func (r *Runtime) ResourceSourceRoot() (string, error) {
	if r == nil || r.configDirectory == "" || !filepath.IsAbs(r.configDirectory) {
		return "", fmt.Errorf("resource source configuration root is unavailable")
	}
	return r.configDirectory, nil
}

// ConfiguratorRoot returns the owner-only non-authoritative draft root. It is
// disjoint from desired source, active authority, and Workspace Home state.
func (r *Runtime) ConfiguratorRoot() (string, error) {
	if r == nil || r.stateDirectory == "" || !filepath.IsAbs(r.stateDirectory) || filepath.Clean(r.stateDirectory) != r.stateDirectory {
		return "", fmt.Errorf("Configurator state root is unavailable")
	}
	return filepath.Join(r.stateDirectory, "configurator"), nil
}

// ContextHomeRoot returns the owner-only root whose exact Context-ID children
// are shared by Configurator and Workspace. It is never mounted as a whole.
func (r *Runtime) ContextHomeRoot() (string, error) {
	if r == nil || r.stateDirectory == "" || !filepath.IsAbs(r.stateDirectory) || filepath.Clean(r.stateDirectory) != r.stateDirectory {
		return "", fmt.Errorf("Context Home root is unavailable")
	}
	return filepath.Join(r.stateDirectory, "workspace-authority-runtime", "contexts"), nil
}

// SetInstallationMigrationBoundaryForTest installs a process-death boundary
// used only by cross-component crash tests. It is intentionally unavailable
// through any application or CLI port.
func (r *Runtime) SetInstallationMigrationBoundaryForTest(boundary func(string) error) {
	if r != nil {
		r.runtimeInstallationMigrationBoundary = boundary
	}
}

// New resolves XDG paths without creating them.
func New(lifetime context.Context) (*Runtime, error) {
	if lifetime == nil {
		return nil, fmt.Errorf("runtime lifetime context is required")
	}
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
	configDirectory, stateDirectory, dataDirectory :=
		filepath.Join(configHome, "tobari"), filepath.Join(stateHome, "tobari"), filepath.Join(dataHome, "tobari")
	for _, directory := range []struct {
		name string
		path *string
	}{
		{name: "configuration", path: &configDirectory},
		{name: "state", path: &stateDirectory},
		{name: "data", path: &dataDirectory},
	} {
		resolved, resolveErr := canonicalPathWithMissingStrict(*directory.path)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve canonical %s directory: %w", directory.name, resolveErr)
		}
		if strings.Contains(resolved, ",") || strings.IndexFunc(resolved, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("canonical %s directory contains unsupported Docker bind syntax", directory.name)
		}
		*directory.path = resolved
	}
	runtime, err := newRuntimeWithData(
		configDirectory, stateDirectory, dataDirectory, osCommandRunner{},
	)
	if err != nil {
		return nil, err
	}
	runtime.permissionIngestionTransport = permissionSessionTransportForGOOS(goruntime.GOOS)
	runtime.lifetimeContext = lifetime
	return runtime, nil
}

// FinalWorkspaceAuthorityRoot returns the configured owner root for the final
// Workspace authority envelope. The composition root uses this path to
// construct and inject the final-only Store; Runtime never opens or
// rediscovers the Store itself.
func (r *Runtime) FinalWorkspaceAuthorityRoot() (string, error) {
	if r == nil || r.stateDirectory == "" || !filepath.IsAbs(r.stateDirectory) || filepath.Clean(r.stateDirectory) != r.stateDirectory {
		return "", fmt.Errorf("runtime state directory is unavailable")
	}
	return filepath.Join(r.stateDirectory, "authority"), nil
}

// lifetimeParent selects the process-lifetime context supplied by the command
// composition root. Tests that construct Runtime directly retain their caller
// context. Cleanup and attachment control derive their own finite deadlines
// from this parent so child cancellation cannot abandon host authority.
func (r *Runtime) lifetimeParent(caller context.Context) context.Context {
	if r != nil && r.lifetimeContext != nil {
		return r.lifetimeContext
	}
	return caller
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
	shortTemporaryDirectory, err := canonicalPathWithMissingStrict("/tmp")
	if err != nil {
		return nil, fmt.Errorf("resolve canonical short temporary directory: %w", err)
	}
	if strings.Contains(shortTemporaryDirectory, ",") || strings.IndexFunc(shortTemporaryDirectory, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("canonical short temporary directory contains unsupported Docker bind syntax")
	}
	return &Runtime{
		configDirectory:              configDirectory,
		stateDirectory:               stateDirectory,
		dataDirectory:                dataDirectory,
		shortTemporaryDirectory:      shortTemporaryDirectory,
		runner:                       runner,
		browser:                      osHostBrowserOpener{},
		serviceBrowser:               osServiceBrowserDispatcher{},
		gitIdentity:                  newOSHostGitIdentityResolver(),
		companion:                    companionruntime.NewOSLauncher(),
		companionEntropy:             rand.Reader,
		hostCLIs:                     newPathHostCLIResolver(),
		credentialHost:               newOSHostCredentialAcquirer(),
		hostLoginProfiles:            osHostLoginProfileReader{},
		identities:                   identityIssuer{now: time.Now, entropy: rand.Reader},
		permissionIngestionTransport: tobari.PermissionSessionTransportUnix,
	}, nil
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
