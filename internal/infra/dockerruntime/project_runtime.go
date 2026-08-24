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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
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
	projectInteractivePrompt = "\\h:\\w\\$ "
)

func bashSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func projectShellExecEnvironment(
	manifest tobari.WorkspaceManifest, lookup func(string) (string, bool),
) ([]string, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return projectShellSettingsEnvironment(manifest.ShellEnvironment, lookup)
}

func projectShellSettingsEnvironment(
	settings []tobari.ManifestShellEnvironmentSetting, lookup func(string) (string, bool),
) ([]string, error) {
	if lookup == nil {
		return nil, fmt.Errorf("host shell environment lookup is required")
	}
	prompt := projectInteractivePrompt
	resolved := make(map[string]string, len(settings))
	for _, setting := range settings {
		var value string
		var present bool
		switch setting.Source {
		case tobari.ManifestShellEnvironmentInherit:
			value, present = lookup(setting.Variable)
		case tobari.ManifestShellEnvironmentLiteral:
			if setting.Value == nil {
				return nil, fmt.Errorf("literal shell environment %s has no value", setting.Variable)
			}
			value, present = *setting.Value, true
		default:
			return nil, fmt.Errorf("persisted shell environment %s has invalid source %q", setting.Variable, setting.Source)
		}
		if !present {
			continue
		}
		if err := tobari.ValidateContextShellEnvironmentValue(value); err != nil {
			return nil, fmt.Errorf("resolve shell environment %s: %w", setting.Variable, err)
		}
		if setting.Variable == "PS1" {
			prompt = value
			continue
		}
		resolved[setting.Variable] = value
	}
	environment := []string{
		"PS1=" + prompt,
		"PROMPT_COMMAND=PS1=" + bashSingleQuoted(prompt),
	}
	for _, variable := range tobari.ManifestShellEnvironmentVariables() {
		if variable == "PS1" {
			continue
		}
		if value, found := resolved[variable]; found {
			environment = append(environment, variable+"="+value)
		}
	}
	return environment, nil
}

func projectLifetimeCommand() []string {
	return strings.Fields(tobari.RuntimeImageLifetimeCommand)
}

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

// ValidateProjectRuntime checks the image that a new logical Workspace would
// consume without creating state or Docker resources. EnterProject calls this
// before CreateProject so a missing or incompatible image cannot leave a
// durable Workspace that has no possible runtime.
func (r *Runtime) ValidateProjectRuntime(ctx context.Context, state tobari.State) error {
	manifest, _, err := r.activeContext()
	if err != nil {
		return err
	}
	return r.ValidateProjectRuntimeForContext(ctx, state, manifest.ID)
}

func (r *Runtime) ValidateProjectRuntimeForContext(ctx context.Context, state tobari.State, contextID string) error {
	if err := state.Validate(); err != nil {
		return err
	}
	contexts, err := r.ListContexts(ctx)
	if err != nil {
		return err
	}
	if len(contexts.Items) != state.ManifestCount {
		return fault.New(
			fault.KindRejected, "cluster_projection_stale",
			"the shared cluster has not loaded the complete Context catalog", false,
			fault.NextAction{Command: "cluster up", Reason: "Validate and activate the aggregate multi-Context policy projection."},
		)
	}
	manifest, _, err := r.contextByID(contextID)
	if err != nil {
		return err
	}
	image, err := r.resolveContextImageFor(ctx, manifest)
	if err != nil {
		return err
	}
	image = r.resolveBuiltinImageSelector(image)
	return r.validateCompatibleImage(ctx, image)
}

// EnsureProjectRuntime converges the exact runtime resources of a durable
// logical Tobari. It never removes logical state when Docker is missing or
// cannot be classified safely.
func (r *Runtime) EnsureProjectRuntime(
	ctx context.Context, state tobari.State, instance tobari.Workspace,
) (tobari.Workspace, error) {
	if err := state.Validate(); err != nil {
		return tobari.Workspace{}, err
	}
	if err := instance.Validate(); err != nil {
		return tobari.Workspace{}, err
	}
	if instance.Incomplete {
		return tobari.Workspace{}, fmt.Errorf("project instance state is incomplete; delete the selected Tobari before recreating it")
	}
	var updated tobari.Workspace
	var attempted *tobari.WorkspaceManifestRevision
	phase := "preflight"
	changeState := tobari.ReconciliationChangeNone
	err := r.withProjectLock(ctx, func() (reconcileErr error) {
		if err := r.reconcileProjectJournal(); err != nil {
			return err
		}
		stored, err := r.readProjectInstance(instance.ID)
		if err != nil {
			return err
		}
		if stored.ID != instance.ID || stored.Root != instance.Root || stored.Profile != instance.Profile {
			return fmt.Errorf("project logical state changed before runtime reconciliation")
		}
		if resolved, resolveErr := r.ResolveProjectRoot(ctx, stored.Root); resolveErr != nil || resolved != stored.Root {
			return fmt.Errorf("project root is no longer accessible at its canonical path")
		}
		manifest, _, err := r.contextByID(stored.WorkspaceManifestID)
		if err != nil {
			return err
		}
		if manifest.Name != stored.WorkspaceManifestName {
			return fmt.Errorf("project Context binding is stale")
		}
		attempt := manifest.Desired
		attempted = &attempt
		defer func() {
			if reconcileErr == nil {
				return
			}
			code := "workspace_reconciliation_failed"
			if errors.Is(reconcileErr, context.Canceled) || errors.Is(reconcileErr, context.DeadlineExceeded) {
				code = "workspace_reconciliation_interrupted"
				changeState = tobari.ReconciliationChangeUnknown
			}
			now := time.Now().UTC()
			if r.identities.now != nil {
				now = r.identities.now().UTC()
			}
			stored.LastFailure = &tobari.ReconciliationFailure{
				AttemptedGeneration:       attempted.Generation,
				AttemptedManifestRevision: attempted.Revision,
				AttemptedEntryRevision:    attempted.EntryRevision,
				Phase:                     phase,
				Code:                      code,
				ChangeState:               changeState,
				OccurredAt:                now,
			}
			if recordErr := r.writeProjectInstance(stored); recordErr != nil {
				reconcileErr = fmt.Errorf("%w; record reconciliation failure: %v", reconcileErr, recordErr)
			}
		}()
		// Resolve and atomically replace the narrow Git fallback before any
		// Docker inspection or mutation. A failing host read therefore cannot
		// leave either a partial projection or newly changed Docker resources.
		phase = "workspace_projection"
		changeState = tobari.ReconciliationChangePartial
		if err := r.reconcileProjectGitIdentity(ctx, manifest, stored); err != nil {
			return err
		}
		image, err := r.resolveContextImageFor(ctx, manifest)
		if err != nil {
			return err
		}
		image = r.resolveBuiltinImageSelector(image)
		phase = "runtime_resolution"
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return err
		}
		imageID, err := r.compatibleImageID(ctx, image)
		if err != nil {
			return err
		}
		if err := r.ensurePrivateDirectory(r.projectHomePath(stored.ID)); err != nil {
			return fmt.Errorf("prepare project home: %w", err)
		}
		workspaceRoot, err := r.projectContainerRoot(stored.Root)
		if err != nil {
			return err
		}
		if err := ensureProjectHomeMountTarget(r.projectHomePath(stored.ID), workspaceRoot); err != nil {
			return fmt.Errorf("prepare project mount target: %w", err)
		}
		agentProfile := manifest.AgentProfile
		profile, err := r.ensureSharedProfile(agentProfile)
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
		desired := stored
		desired.Image = image
		authProjection, err := r.reconcileProjectAuth(ctx, desired)
		if err != nil {
			return err
		}
		specHash, err := r.projectSpecHashWithAuthAndSourceAccess(
			state, desired, profile, network, image, imageID, authProjection, manifest.SourceAccess,
		)
		if err != nil {
			return err
		}
		phase = "runtime_reconciliation"
		ready, err := r.projectRuntimeReadyForPrincipalRefresh(
			ctx, stored, container, network, workspaceRoot, specHash, manifest.SourceAccess,
		)
		if err != nil {
			return err
		}
		if !ready {
			if err := r.removeProjectPrincipal(ctx, stored.ID); err != nil {
				return fmt.Errorf("close project principal before network reconciliation: %w", err)
			}
			if err := r.ensureProjectNetwork(ctx, network, stored.ID); err != nil {
				return err
			}
			if err := r.ensureGatewayNetwork(ctx, network); err != nil {
				return err
			}
		}
		if err := r.ensureGatewayNetworkGuard(ctx); err != nil {
			return err
		}
		gatewayIP, err := r.gatewayNetworkAddress(ctx, network)
		if err != nil {
			return err
		}
		subnet, err := r.projectNetworkSubnet(ctx, network)
		if err != nil {
			return err
		}
		if !ready {
			if err := r.ensureProjectContainerWithAuth(
				ctx, state, desired, profile, container, network, gatewayIP, image, specHash, authProjection, manifest.SourceAccess,
			); err != nil {
				return err
			}
		}
		phase = "network_guard"
		if err := r.ensureWorkspaceNetworkGuard(ctx, stored, container, network, subnet, gatewayIP); err != nil {
			return err
		}
		workspaceIP, err := r.workspaceNetworkAddress(ctx, container, network)
		if err != nil {
			return err
		}
		if err := validateProjectNetworkEndpoints(subnet, workspaceIP, gatewayIP); err != nil {
			return networkGuardFailure("Workspace and Gateway endpoints do not match the owned project network", err)
		}
		if err := completeNetworkGuardState().Validate(); err != nil {
			return networkGuardFailure("Workspace network guard state is incomplete", err)
		}
		if err := r.updateProjectPrincipal(ctx, stored, network, workspaceIP, gatewayIP); err != nil {
			return fmt.Errorf("publish guarded project principal: %w", err)
		}
		phase = "readiness"
		if err := r.waitProjectReady(ctx, container); err != nil {
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
		desired.Runtime = tobari.WorkspaceRuntime{ContainerID: containerID, NetworkID: networkID}
		if manifest.RuntimeBinding == nil {
			return fmt.Errorf("Workspace Manifest has no exact Runtime binding")
		}
		now := time.Now().UTC()
		if r.identities.now != nil {
			now = r.identities.now().UTC()
		}
		desired.LastSuccessfulEntry = &tobari.AppliedEntry{
			ManifestGeneration: manifest.Desired.Generation,
			ManifestRevision:   manifest.Desired.Revision,
			EntryRevision:      manifest.Desired.EntryRevision,
			RuntimeID:          manifest.RuntimeBinding.RuntimeID,
			RuntimeRevision:    manifest.RuntimeBinding.Revision,
			ResolvedSpec:       specHash,
			ReconciledAt:       now,
		}
		desired.LastFailure = nil
		if err := r.writeProjectInstance(desired); err != nil {
			return err
		}
		updated = desired
		return nil
	})
	if err != nil {
		return tobari.Workspace{}, err
	}
	return updated, nil
}

// InspectProjectRuntime describes recoverable runtime health without changing
// state. A missing container is not a missing logical Tobari.
func (r *Runtime) InspectProjectRuntime(
	ctx context.Context, instance tobari.Workspace,
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

// ProjectSessionAttached observes active Docker exec sessions for the exact
// work container. Attachment is transient runtime state; it is never persisted
// in the logical Workspace record.
func (r *Runtime) ProjectSessionAttached(ctx context.Context, instance tobari.Workspace) (bool, error) {
	if err := instance.Validate(); err != nil {
		return false, err
	}
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		return false, err
	}
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .ExecIDs}}", container},
		os.Environ(),
	)
	if err != nil {
		if isMissingDockerResource(err, output) {
			return false, nil
		}
		return false, fmt.Errorf("inspect project session: %w: %s", err, boundedDiagnostic(output))
	}
	var execIDs []string
	if err := json.Unmarshal(bytes.TrimSpace(output), &execIDs); err != nil {
		return false, fmt.Errorf("decode project session IDs: %w", err)
	}
	for _, execID := range execIDs {
		if strings.TrimSpace(execID) != "" {
			return true, nil
		}
	}
	return false, nil
}

// EnterProjectRuntime attaches the caller's streams to the ready work
// container, maps the host directory below its root, and preserves child exit
// status.
func (r *Runtime) EnterProjectRuntime(
	ctx context.Context, instance tobari.Workspace, manifest tobari.WorkspaceManifest, cwd string,
	session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer,
) (outcome tobari.WorkspaceSessionOutcome, resultErr error) {
	if err := session.Validate(); err != nil {
		return outcome, err
	}
	if err := manifest.Validate(); err != nil {
		return outcome, err
	}
	principal, err := legacyInteractiveWorkspacePrincipal(instance)
	if err != nil {
		return outcome, err
	}
	if manifest.ID != principal.contextID {
		return outcome, fmt.Errorf("Context shell environment does not belong to the selected Workspace")
	}
	container, _, err := tobari.ProjectResourceNames(principal.workspaceID)
	if err != nil {
		return outcome, err
	}
	interactiveAttachment, err := r.beginInteractiveWorkspaceAttachment(ctx, instance)
	if err != nil {
		return outcome, err
	}
	defer func() {
		if cleanupErr := interactiveAttachment.Close(ctx); cleanupErr != nil {
			outcome.CleanupIssues = append(outcome.CleanupIssues, tobari.WorkspaceCleanupInteractiveSession)
		}
	}()
	return r.runWorkspaceSession(
		ctx, principal, manifest.ShellEnvironment, cwd, container, session,
		interactiveAttachment, in, out, errOut,
	)
}

// runWorkspaceSession reuses one attachment owner already acquired under the
// installation lifecycle order. Both the predecessor wrapper and the dormant
// final-identity bridge enter here; neither can begin a second WP07 session.
func (r *Runtime) runWorkspaceSession(
	ctx context.Context, principal interactiveWorkspacePrincipal,
	shellSettings []tobari.ManifestShellEnvironmentSetting, cwd string,
	container string,
	session tobari.WorkspaceSessionRequest, interactiveAttachment *interactiveWorkspaceAttachment,
	in io.Reader, out, errOut io.Writer,
) (outcome tobari.WorkspaceSessionOutcome, resultErr error) {
	return r.runWorkspaceSessionWithHandoff(ctx, principal, shellSettings, cwd, container, session, interactiveAttachment, in, out, errOut, nil)
}

func (r *Runtime) runWorkspaceSessionWithHandoff(
	ctx context.Context, principal interactiveWorkspacePrincipal,
	shellSettings []tobari.ManifestShellEnvironmentSetting, cwd string,
	container string,
	session tobari.WorkspaceSessionRequest, interactiveAttachment *interactiveWorkspaceAttachment,
	in io.Reader, out, errOut io.Writer, handoff func(),
) (outcome tobari.WorkspaceSessionOutcome, resultErr error) {
	if err := principal.validate(); err != nil {
		return outcome, err
	}
	if err := session.Validate(); err != nil {
		return outcome, err
	}
	if interactiveAttachment == nil ||
		interactiveAttachment.session.WorkspaceManifestID != principal.contextID ||
		interactiveAttachment.session.WorkspaceID != principal.workspaceID {
		return outcome, fmt.Errorf("interactive attachment does not belong to the exact Workspace principal")
	}
	extraEnvironment := []string{}
	hostLoopbackAttachment, hostLoopbackErr := r.beginHostLoopbackAttachmentForPrincipal(ctx, principal, interactiveAttachment.session.AttachmentID)
	if hostLoopbackErr != nil {
		return outcome, fmt.Errorf("establish Host Loopback attachment: %w", hostLoopbackErr)
	}
	defer func() {
		if cleanupErr := hostLoopbackAttachment.Close(ctx); cleanupErr != nil {
			outcome.CleanupIssues = append(outcome.CleanupIssues, tobari.WorkspaceCleanupHostLoopback)
		}
	}()
	projection := tobari.NewHostLoopbackCapabilityProjection()
	encoded, encodeErr := json.Marshal(projection)
	if encodeErr != nil {
		return outcome, encodeErr
	}
	extraEnvironment = append(extraEnvironment, "TOBARI_CAPABILITIES_JSON="+string(encoded))
	if strings.TrimSpace(container) == "" {
		return outcome, fmt.Errorf("Workspace session container is missing")
	}
	permissionChannel, permissionChannelErr := r.startWorkspacePermissionChannel(ctx, interactiveAttachment, container)
	if permissionChannelErr != nil {
		return outcome, fmt.Errorf("establish permission wait channel: %w", permissionChannelErr)
	}
	defer func() {
		if cleanupErr := permissionChannel.Close(); cleanupErr != nil {
			outcome.CleanupIssues = append(outcome.CleanupIssues, tobari.WorkspaceCleanupPermissionChannel)
		}
	}()
	extraEnvironment = append(extraEnvironment, permissionChannel.environment()...)
	code, serviceReceipt, resultErr := r.enterProjectRuntime(
		ctx, principal, shellSettings, cwd, container, session,
		extraEnvironment, in, out, errOut, handoff,
	)
	outcome.ExitCode = code
	outcome.ServiceCleanupReceipt = serviceReceipt
	if serviceReceipt == nil && resultErr == nil {
		outcome.CleanupIssues = append(outcome.CleanupIssues, tobari.WorkspaceCleanupServiceExposure)
	}
	return outcome, resultErr
}

func (r *Runtime) enterProjectRuntime(
	ctx context.Context, principal interactiveWorkspacePrincipal,
	shellSettings []tobari.ManifestShellEnvironmentSetting, cwd string,
	container string,
	session tobari.WorkspaceSessionRequest, extraEnvironment []string, in io.Reader, out, errOut io.Writer, handoff func(),
) (code int, serviceReceipt *tobari.ServiceCleanupReceipt, resultErr error) {
	if err := principal.validate(); err != nil {
		return 0, nil, err
	}
	resolved, err := r.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return 0, nil, err
	}
	workdir, err := r.projectContainerCWD(principal.projectRoot, resolved)
	if err != nil {
		return 0, nil, err
	}
	if strings.TrimSpace(container) == "" {
		return 0, nil, fmt.Errorf("Workspace session container is missing")
	}
	uid, gid := currentIDs()
	shellEnvironment, err := projectShellSettingsEnvironment(shellSettings, os.LookupEnv)
	if err != nil {
		return 0, nil, err
	}
	// A direct child remains runnable with redirected streams. Docker rejects
	// `exec -t` when the caller has no terminal, before the exact child starts;
	// allocate its container TTY only for an actual attached terminal pair.
	args := []string{"exec", "-i"}
	if isTerminalFile(in) && isTerminalFile(out) {
		args = append(args, "-t")
	}
	args = append(args, "--user", strconv.Itoa(uid)+":"+strconv.Itoa(gid))
	bridge := newWorkspaceLoginBridge(ctx, r, container, principal.workspaceID)
	defer bridge.close()
	browserChannel, err := r.startWorkspaceBrowserChannel(ctx, bridge, container)
	if err != nil {
		return 0, nil, err
	}
	defer browserChannel.close()
	serviceController, err := r.startWorkspaceServiceControllerForPrincipal(ctx, principal, container)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		receipt, cleanupErr := serviceController.CloseWithReceipt(ctx)
		if cleanupErr == nil {
			serviceReceipt = &receipt
		}
	}()
	for _, environment := range browserChannel.environment() {
		args = append(args, "--env", environment)
	}
	for _, environment := range serviceController.environment() {
		args = append(args, "--env", environment)
	}
	for _, environment := range shellEnvironment {
		args = append(args, "--env", environment)
	}
	for _, environment := range extraEnvironment {
		args = append(args, "--env", environment)
	}
	childArgv := session.Argv()
	if !session.Direct() {
		childArgv = []string{"/bin/bash"}
	}
	args = append(args, "--workdir", workdir, container)
	args = append(args, childArgv...)
	// Keep the existing direct stream runner as the compatibility path. The
	// interactive runner is selected only for a real terminal presentation
	// session, after the child environment and all attachment setup are ready.
	run := func() error { return r.runner.Run(ctx, args, os.Environ(), in, out, errOut) }
	if structuredOutputColorEnabled(in, out, shellEnvironment) {
		if interactive, ok := r.runner.(interactiveCommandRunner); ok {
			run = func() error {
				return interactive.RunInteractive(ctx, args, os.Environ(), in, out, errOut, true)
			}
		}
	}
	if handoff != nil {
		handoff()
	}
	if err := run(); err == nil {
		return 0, serviceReceipt, nil
	} else {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return exitError.ExitCode(), serviceReceipt, nil
		}
		type exitCoder interface{ ExitCode() int }
		var coded exitCoder
		if errors.As(err, &coded) {
			return coded.ExitCode(), serviceReceipt, nil
		}
		return 0, serviceReceipt, err
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
func (r *Runtime) ProjectHome(ctx context.Context, instance tobari.Workspace) (string, error) {
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
func (r *Runtime) DeleteProject(ctx context.Context, instance tobari.Workspace) error {
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
			WorkspaceManifestID: stored.WorkspaceManifestID,
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
		if err := os.Remove(r.projectAuthRegistryPath(stored.ID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove Workspace authentication file registry: %w", err)
		}
		journal.Phase = projectPhaseInstance
		if err := r.writeProjectJournal(journal); err != nil {
			return err
		}
		if err := r.removeProjectRootIndexFor(stored.Root, stored.WorkspaceManifestID); err != nil {
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

// projectRuntimeReadyForPrincipalRefresh observes the complete endpoint-owning
// runtime shape without mutating Docker. A ready result lets ordinary re-entry
// keep the existing fail-closed principal in place while guards are rechecked;
// drift takes the slower path that closes authority before changing resources.
func (r *Runtime) projectRuntimeReadyForPrincipalRefresh(
	ctx context.Context,
	instance tobari.Workspace,
	container, network, workspaceRoot, specHash string,
	sourceAccess tobari.ManifestSourceAccess,
) (bool, error) {
	networkExists, err := r.projectResourceExists(ctx, "network", network)
	if err != nil {
		return false, err
	}
	if !networkExists {
		return false, nil
	}
	if err := r.verifyOwnedProjectResource(ctx, "network", network, instance.ID, projectNetRole); err != nil {
		return false, err
	}
	gatewayIP, gatewayConnected, err := r.containerNetworkAddressIfConnected(ctx, gatewayContainer, network, "Gateway")
	if err != nil {
		return false, err
	}
	if !gatewayConnected {
		return false, nil
	}
	containerExists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return false, err
	}
	if !containerExists {
		return false, nil
	}
	if err := r.verifyOwnedProjectResource(ctx, "container", container, instance.ID, projectWorkRole); err != nil {
		return false, err
	}
	observedSpec, err := r.projectContainerSpecHash(ctx, container)
	if err != nil {
		return false, err
	}
	if observedSpec != specHash {
		return false, nil
	}
	observedDNS, err := r.projectContainerDNS(ctx, container)
	if err != nil {
		return false, err
	}
	if len(observedDNS) != 1 || observedDNS[0] != gatewayIP {
		return false, nil
	}
	observedAccess, err := r.projectContainerSourceAccess(ctx, container, instance.Root, workspaceRoot)
	if err != nil {
		return false, err
	}
	if observedAccess != sourceAccess {
		return false, nil
	}
	workspaceIP, workspaceConnected, err := r.containerNetworkAddressIfConnected(ctx, container, network, "Workspace")
	if err != nil {
		return false, err
	}
	if !workspaceConnected {
		return false, nil
	}
	component, err := r.inspectContainer(ctx, projectWorkRole, container)
	if err != nil {
		return false, err
	}
	if component.State != "running" || component.Health != "healthy" {
		return false, nil
	}
	subnet, err := r.projectNetworkSubnet(ctx, network)
	if err != nil {
		return false, err
	}
	if err := validateProjectNetworkEndpoints(subnet, workspaceIP, gatewayIP); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Runtime) ensureProjectContainer(
	ctx context.Context, state tobari.State, instance tobari.Workspace,
	profile, container, network, gatewayIP, image, specHash string,
) error {
	if err := r.ensureProjectContainerWithAuth(
		ctx, state, instance, profile, container, network, gatewayIP, image, specHash,
		projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}}, tobari.ManifestSourceAccessReadWrite,
	); err != nil {
		return err
	}
	return r.waitProjectReady(ctx, container)
}

func (r *Runtime) ensureProjectContainerWithAuth(
	ctx context.Context, state tobari.State, instance tobari.Workspace,
	profile, container, network, gatewayIP, image, specHash string,
	auth projectAuthProjection,
	sourceAccess tobari.ManifestSourceAccess,
) error {
	if err := sourceAccess.Validate(); err != nil {
		return err
	}
	workspaceRoot, err := r.projectContainerRoot(instance.Root)
	if err != nil {
		return err
	}
	gitDirectory, err := r.projectGitDirectory(instance.ID)
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
		} else {
			observedDNS, dnsErr := r.projectContainerDNS(ctx, container)
			if dnsErr != nil {
				return dnsErr
			}
			if len(observedDNS) != 1 || observedDNS[0] != gatewayIP {
				if output, removeErr := r.runner.Output(ctx, []string{"rm", "-f", container}, os.Environ()); removeErr != nil {
					return fmt.Errorf("remove project container with stale DNS: %w: %s", removeErr, boundedDiagnostic(output))
				}
				exists = false
			}
		}
	}
	if exists {
		component, inspectErr := r.inspectContainer(ctx, projectWorkRole, container)
		if inspectErr != nil {
			return inspectErr
		}
		observedAccess, inspectErr := r.projectContainerSourceAccess(ctx, container, instance.Root, workspaceRoot)
		if inspectErr != nil {
			return inspectErr
		}
		if observedAccess != sourceAccess {
			if output, removeErr := r.runner.Output(ctx, []string{"rm", "-f", container}, os.Environ()); removeErr != nil {
				return fmt.Errorf("remove project container with stale source access: %w: %s", removeErr, boundedDiagnostic(output))
			}
			exists = false
		}
		if exists {
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
	}
	uid, gid := currentIDs()
	sourceMount := "type=bind,src=" + instance.Root + ",dst=" + workspaceRoot
	if sourceAccess == tobari.ManifestSourceAccessReadOnly {
		sourceMount += ",readonly"
	}
	args := []string{
		"create", "--name", container, "--hostname", container,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges:true",
		"--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		"--tmpfs", "/tmp:size=512m,mode=1777", "--tmpfs", "/run:size=16m,mode=1777",
		"--env", "HOME=/var/lib/tobari",
		"--env", "TOBARI_INSIDE=1", "--env", "TOBARI_ID=" + instance.ID, "--env", "TOBARI_ROOT=" + workspaceRoot,
		"--env", "TOBARI_CONTEXT_ID=" + instance.WorkspaceManifestID,
		"--env", "TOBARI_PROFILE=/opt/tobari/profile",
		"--env", "SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
		"--env", "REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
		"--env", "GIT_CONFIG_SYSTEM=" + projectGitContainerConfig,
		"--mount", "type=bind,src=" + r.projectHomePath(instance.ID) + ",dst=/var/lib/tobari",
		"--mount", sourceMount,
		"--mount", "type=bind,src=" + gitDirectory + ",dst=" + projectGitContainerDirectory + ",readonly",
		"--mount", "type=bind,src=" + profile + ",dst=/opt/tobari/profile,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "skills") + ",dst=/var/lib/tobari/.claude/skills,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "agents") + ",dst=/var/lib/tobari/.claude/agents,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "commands") + ",dst=/var/lib/tobari/.claude/commands,readonly",
		"--mount", "type=bind,src=" + filepath.Join(profile, "claude", "plugins.lock") + ",dst=/var/lib/tobari/.claude/plugins.lock,readonly",
		"--mount", "type=bind,src=" + filepath.Join(state.RuntimeDirectory, "browser", "tobari-open") + ",dst=/run/tobari-open,readonly",
		"--mount", "type=bind,src=" + filepath.Join(state.RuntimeDirectory, "browser", "tobari-open") + ",dst=/usr/local/bin/xdg-open,readonly",
		"--mount", "type=bind,src=" + filepath.Join(state.RuntimeDirectory, "helpers", "tobari-expose") + ",dst=/usr/local/bin/tobari-expose,readonly",
		"--mount", "type=bind,src=" + filepath.Join(state.RuntimeDirectory, "helpers", "tobari-permission") + ",dst=/usr/local/bin/tobari-permission,readonly",
		"--mount", "type=volume,src=tobari-public-ca,dst=/run/tobari/ca-public,readonly",
		"--workdir", workspaceRoot, "--network", network, "--dns", gatewayIP,
		"--health-cmd", "test -f /tmp/tobari-ready", "--health-interval", "2s",
		"--health-timeout", "2s", "--health-retries", "30",
		"--label", ownerLabel + "=" + ownerValue,
		"--label", componentLabel + "=tobari",
		"--label", projectIDLabel + "=" + instance.ID,
		"--label", projectRoleLabel + "=" + projectWorkRole,
		"--label", projectSpecLabel + "=" + specHash,
	}
	for _, environment := range auth.Environment {
		args = append(args, "--env", environment)
	}
	args = append(args, projectResourceDockerArgs()...)
	args = append(args, image)
	args = append(args, projectLifetimeCommand()...)
	if output, err := r.runner.Output(ctx, args, os.Environ()); err != nil {
		return fmt.Errorf("create project container: %w: %s", err, boundedDiagnostic(output))
	}
	if output, err := r.runner.Output(ctx, []string{"start", container}, os.Environ()); err != nil {
		return fmt.Errorf("start project container: %w: %s", err, boundedDiagnostic(output))
	}
	return nil
}

func (r *Runtime) projectContainerSourceAccess(
	ctx context.Context, container, root, workspaceRoot string,
) (tobari.ManifestSourceAccess, error) {
	output, err := r.runner.Output(
		ctx, []string{"inspect", "--format", "{{json .Mounts}}", container}, os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("inspect project source mount: %w: %s", err, boundedDiagnostic(output))
	}
	var mounts []struct {
		Type        string `json:"Type"`
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &mounts); err != nil {
		return "", fmt.Errorf("decode project source mount: %w", err)
	}
	found := false
	access := tobari.ManifestSourceAccessReadOnly
	for _, mount := range mounts {
		if mount.Type != "bind" || mount.Source != root {
			continue
		}
		if mount.Destination != workspaceRoot {
			if mount.RW {
				return "", fmt.Errorf("project source has a writable alias")
			}
			continue
		}
		if found {
			return "", fmt.Errorf("project source mount is duplicated")
		}
		found = true
		if mount.RW {
			access = tobari.ManifestSourceAccessReadWrite
		}
	}
	if !found {
		return "", fmt.Errorf("project source mount is absent")
	}
	return access, nil
}

func (r *Runtime) projectContainerDNS(ctx context.Context, container string) ([]string, error) {
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{json .HostConfig.Dns}}", container},
		os.Environ(),
	)
	if err != nil {
		return nil, fmt.Errorf("inspect project DNS: %w: %s", err, boundedDiagnostic(output))
	}
	var addresses []string
	if err := json.Unmarshal(bytes.TrimSpace(output), &addresses); err != nil {
		return nil, fmt.Errorf("decode project DNS: %w", err)
	}
	return addresses, nil
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
	Image           string                      `json:"image"`
	ImageID         string                      `json:"image_id"`
	RuntimeAPI      string                      `json:"runtime_api"`
	AssetVersion    string                      `json:"asset_version"`
	WorkspaceRoot   string                      `json:"workspace_root"`
	Root            string                      `json:"root"`
	SourceAccess    tobari.ManifestSourceAccess `json:"source_access"`
	Network         string                      `json:"network"`
	NetworkGuard    string                      `json:"network_guard"`
	User            string                      `json:"user"`
	Environment     []string                    `json:"environment"`
	AuthFiles       []string                    `json:"auth_files"`
	Mounts          []string                    `json:"mounts"`
	ReadOnly        bool                        `json:"read_only"`
	Capabilities    string                      `json:"capabilities"`
	Security        string                      `json:"security"`
	Resources       []string                    `json:"resources"`
	LifetimeCommand []string                    `json:"lifetime_command"`
	HealthCommand   string                      `json:"health_command"`
	HealthInterval  string                      `json:"health_interval"`
	ProfileDigest   string                      `json:"profile_digest"`
}

func projectSourceMountSpec(root, workspaceRoot string, sourceAccess tobari.ManifestSourceAccess) string {
	mount := "bind:" + root + "->" + workspaceRoot
	if sourceAccess == tobari.ManifestSourceAccessReadOnly {
		mount += ":ro"
	}
	return mount
}

func (r *Runtime) projectSpecHash(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
) (string, error) {
	return r.projectSpecHashWithAuth(
		state, instance, profile, network, image, imageID,
		projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}},
	)
}

func (r *Runtime) projectSpecHashWithAuth(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
	auth projectAuthProjection,
) (string, error) {
	return r.projectSpecHashWithAuthAndSourceAccess(
		state, instance, profile, network, image, imageID, auth, tobari.ManifestSourceAccessReadWrite,
	)
}

func (r *Runtime) projectSpecHashWithAuthAndSourceAccess(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
	auth projectAuthProjection, sourceAccess tobari.ManifestSourceAccess,
) (string, error) {
	return r.projectSpecHashWithAuthAndCommand(
		state, instance, profile, network, image, imageID, auth, projectLifetimeCommand(), sourceAccess,
	)
}

func (r *Runtime) projectSpecHashWithCommand(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
	command []string,
) (string, error) {
	return r.projectSpecHashWithAuthAndCommand(
		state, instance, profile, network, image, imageID,
		projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}, Providers: []projectAuthProviderBinding{}}, command,
		tobari.ManifestSourceAccessReadWrite,
	)
}

func (r *Runtime) projectSpecHashWithAuthAndCommand(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
	auth projectAuthProjection,
	command []string,
	sourceAccess tobari.ManifestSourceAccess,
) (string, error) {
	spec, err := r.projectRuntimeSpecWithAuthAndCommand(
		state, instance, profile, network, image, imageID, auth, command, sourceAccess,
	)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:]), nil
}

func (r *Runtime) projectRuntimeSpecWithAuthAndCommand(
	state tobari.State, instance tobari.Workspace, profile, network, image, imageID string,
	auth projectAuthProjection,
	command []string,
	sourceAccess tobari.ManifestSourceAccess,
) (projectRuntimeSpec, error) {
	if err := sourceAccess.Validate(); err != nil {
		return projectRuntimeSpec{}, err
	}
	workspaceRoot, err := r.projectContainerRoot(instance.Root)
	if err != nil {
		return projectRuntimeSpec{}, err
	}
	gitDirectory, err := r.projectGitDirectory(instance.ID)
	if err != nil {
		return projectRuntimeSpec{}, err
	}
	profileDigest, err := r.projectProfileDigest(profile)
	if err != nil {
		return projectRuntimeSpec{}, err
	}
	uid, gid := currentIDs()
	spec := projectRuntimeSpec{
		Image: image, ImageID: imageID, RuntimeAPI: tobari.RuntimeImageAPI, AssetVersion: state.AssetVersion,
		WorkspaceRoot: workspaceRoot, Root: instance.Root, SourceAccess: sourceAccess, Network: network,
		NetworkGuard: tobari.NetworkGuardRevision,
		User:         strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		Environment: []string{
			"HOME=/var/lib/tobari", "TOBARI_INSIDE=1", "TOBARI_ID=" + instance.ID,
			"TOBARI_CONTEXT_ID=" + instance.WorkspaceManifestID,
			"TOBARI_ROOT=" + workspaceRoot, "TOBARI_PROFILE=/opt/tobari/profile",
			"SSL_CERT_FILE=/tmp/tobari-ca-bundle.pem",
			"REQUESTS_CA_BUNDLE=/tmp/tobari-ca-bundle.pem", "GIT_SSL_CAINFO=/tmp/tobari-ca-bundle.pem",
			"GIT_CONFIG_SYSTEM=" + projectGitContainerConfig,
		},
		AuthFiles: []string{},
		Mounts: []string{
			"bind:" + r.projectHomePath(instance.ID) + "->/var/lib/tobari",
			projectSourceMountSpec(instance.Root, workspaceRoot, sourceAccess),
			"bind:" + gitDirectory + "->" + projectGitContainerDirectory + ":ro",
			"bind:" + profile + "->/opt/tobari/profile:ro",
			"bind:" + filepath.Join(profile, "claude", "skills") + "->/var/lib/tobari/.claude/skills:ro",
			"bind:" + filepath.Join(profile, "claude", "agents") + "->/var/lib/tobari/.claude/agents:ro",
			"bind:" + filepath.Join(profile, "claude", "commands") + "->/var/lib/tobari/.claude/commands:ro",
			"bind:" + filepath.Join(profile, "claude", "plugins.lock") + "->/var/lib/tobari/.claude/plugins.lock:ro",
			"bind:" + filepath.Join(state.RuntimeDirectory, "browser", "tobari-open") + "->/run/tobari-open:ro",
			"bind:" + filepath.Join(state.RuntimeDirectory, "browser", "tobari-open") + "->/usr/local/bin/xdg-open:ro",
			"bind:" + filepath.Join(state.RuntimeDirectory, "helpers", "tobari-expose") + "->/usr/local/bin/tobari-expose:ro",
			"bind:" + filepath.Join(state.RuntimeDirectory, "helpers", "tobari-permission") + "->/usr/local/bin/tobari-permission:ro",
			"volume:tobari-public-ca->/run/tobari/ca-public:ro",
		},
		ReadOnly: true, Capabilities: "ALL", Security: "no-new-privileges:true",
		Resources:       projectResourceHashFields(),
		LifetimeCommand: append([]string(nil), command...),
		HealthCommand:   "test -f /tmp/tobari-ready", HealthInterval: "2s", ProfileDigest: profileDigest,
	}
	spec.Environment = append(spec.Environment, auth.Environment...)
	for _, file := range auth.Files {
		spec.AuthFiles = append(spec.AuthFiles, file.Path+"="+file.Digest)
	}
	sort.Strings(spec.Environment)
	sort.Strings(spec.AuthFiles)
	return spec, nil
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
	if err := tobari.ValidateName(profile); err != nil {
		return "", fmt.Errorf("unsupported agent profile: %w", err)
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
		local = map[string]json.RawMessage{}
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
	manifest, _, err := r.activeContext()
	if err != nil {
		return err
	}
	return r.removeProjectRootIndexFor(root, manifest.ID)
}

func (r *Runtime) removeProjectRootIndexFor(root, contextID string) error {
	indexPath, err := r.rootIndexPath(root, contextID)
	if err != nil {
		return err
	}
	if err := os.Remove(indexPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove project root index: %w", err)
	}
	return syncDirectoryIfPresent(filepath.Dir(indexPath))
}

func syncDirectory(path string) (resultErr error) {
	directory, err := os.Open(path) // #nosec G304 -- callers pass only runtime-owned state directories.
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := directory.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
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
