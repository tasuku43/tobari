package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// FinalWorkspaceSessionState is the closed deletion-observation result for
// the canonical WP07 registry. An error means the state is ambiguous and must
// never be interpreted as absence.
type FinalWorkspaceSessionState string

const (
	FinalWorkspaceSessionLive   FinalWorkspaceSessionState = "live"
	FinalWorkspaceSessionAbsent FinalWorkspaceSessionState = "absent"
)

// HasLiveFinalWorkspaceSession exposes only the canonical owner/liveness
// predicate needed by dependent attachment capabilities. Permission-ingestion
// transport, nonce, lease, ACK, and wait-registry authority stay private to
// this file and are never returned to Host Loopback consumers.
func (r *Runtime) HasLiveFinalWorkspaceSession(ctx context.Context) (bool, error) {
	if r == nil {
		return false, fmt.Errorf("Docker runtime is unavailable")
	}
	if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := requireOwnerOnlyRegularFile(filepath.Join(r.configDirectory, "interactive-attachment.lock")); err != nil {
		return false, fmt.Errorf("validate canonical interactive attachment lock: %w", err)
	}
	live := false
	err := r.withInteractiveAttachmentLock(ctx, func() error {
		if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		for _, session := range registry.Sessions {
			if r.permissionSessionActive(session) {
				live = true
				continue
			}
			if permissionSessionLeaseCurrent(session, time.Now()) {
				return fmt.Errorf("canonical interactive attachment liveness is ambiguous")
			}
		}
		return nil
	})
	return live, err
}

// ConfirmNoFinalWorkspaceSessions proves the complete canonical WP07 registry
// has no owner at all. Gateway replacement is installation-wide, so checking
// only the Workspaces present in one candidate collection would incorrectly
// ignore a stale or predecessor owner omitted from that candidate. Missing
// store state is exact absence; every present validated record blocks, while
// unsafe or malformed state remains observation uncertainty.
func (r *Runtime) ConfirmNoFinalWorkspaceSessions(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("Docker runtime is unavailable")
	}
	return r.withInteractiveAttachmentLock(ctx, func() error {
		if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		changed, err := r.reconcileExpiredInteractiveSessionsLocked(&registry, time.Now())
		if err != nil {
			return err
		}
		if changed {
			if err := writeAtomicJSON(r.interactiveAttachmentSessionRegistryPath(), registry); err != nil {
				return err
			}
			var confirmed tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &confirmed); err != nil {
				return err
			}
			if err := confirmed.Validate(); err != nil || !reflect.DeepEqual(confirmed, registry) {
				return fmt.Errorf("canonical interactive attachment cleanup was not published exactly: %w", err)
			}
		}
		if len(registry.Sessions) != 0 {
			return fmt.Errorf("canonical interactive attachment registry is not globally owner-free")
		}
		return nil
	})
}

// observeFinalWorkspaceSessionOwner is the persistent-identity deletion fence.
// It deliberately does not require a current principal projection: absence is
// decided from the canonical registry first, while a present owner is proved
// by its exact lease and authenticated liveness channel.
func (r *Runtime) observeFinalWorkspaceSessionOwner(ctx context.Context, contextID tobari.ContextID, workspaceID tobari.WorkspaceID) (FinalWorkspaceSessionState, *tobari.InteractiveAttachmentSession, error) {
	if err := contextID.Validate(); err != nil {
		return "", nil, err
	}
	if err := workspaceID.Validate(); err != nil {
		return "", nil, err
	}
	var observed *tobari.InteractiveAttachmentSession
	err := r.withInteractiveAttachmentLock(ctx, func() error {
		if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		current := findInteractiveSession(registry, string(contextID), string(workspaceID))
		if current != nil {
			copy := *current
			observed = &copy
		}
		return nil
	})
	if err != nil || observed == nil {
		if err != nil {
			return "", nil, err
		}
		return FinalWorkspaceSessionAbsent, nil, nil
	}
	active := r.permissionSessionActive(*observed)
	if active {
		return FinalWorkspaceSessionLive, observed, nil
	}
	if permissionSessionLeaseCurrent(*observed, time.Now()) {
		return "", nil, fmt.Errorf("canonical interactive attachment liveness is ambiguous")
	}
	return FinalWorkspaceSessionAbsent, observed, nil
}

// compactExactExpiredFinalWorkspaceSession removes only one exact expired,
// socket-absent target after the container-owning exec has ended. It never
// removes a current, live, changed, foreign, or ambiguous owner.
func (r *Runtime) compactExactExpiredFinalWorkspaceSession(ctx context.Context, contextID tobari.ContextID, workspaceID tobari.WorkspaceID, observed *tobari.InteractiveAttachmentSession) error {
	return r.withInteractiveAttachmentLock(ctx, func() error {
		if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
			if errors.Is(err, os.ErrNotExist) && observed == nil {
				return nil
			}
			return err
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		current := findInteractiveSession(registry, string(contextID), string(workspaceID))
		if current == nil {
			return nil
		}
		if observed == nil || !sameInteractiveSessionAuthority(*current, *observed) {
			return fmt.Errorf("canonical interactive attachment changed during retirement")
		}
		if permissionSessionLeaseCurrent(*current, time.Now()) || r.permissionSessionActive(*current) {
			return fmt.Errorf("canonical interactive attachment is not exactly expired and absent")
		}
		filtered := make([]tobari.InteractiveAttachmentSession, 0, len(registry.Sessions)-1)
		for _, session := range registry.Sessions {
			if session.WorkspaceManifestID == string(contextID) && session.WorkspaceID == string(workspaceID) {
				continue
			}
			filtered = append(filtered, session)
		}
		registry.Sessions = filtered
		return writeAtomicJSON(r.interactiveAttachmentSessionRegistryPath(), registry)
	})
}

func (r *Runtime) waitFinalWorkspaceSessionOwnerAbsent(ctx context.Context, contextID tobari.ContextID, workspaceID tobari.WorkspaceID) error {
	deadline := time.Now().Add(tobari.PermissionSessionLease + permissionSessionHeartbeat + permissionSessionCleanup)
	for time.Now().Before(deadline) {
		state, observed, err := r.observeFinalWorkspaceSessionOwner(ctx, contextID, workspaceID)
		if err == nil && state == FinalWorkspaceSessionAbsent {
			if observed == nil {
				return nil
			}
			if err := r.compactExactExpiredFinalWorkspaceSession(ctx, contextID, workspaceID, observed); err == nil {
				return nil
			}
		}
		if err != nil && !strings.Contains(err.Error(), "liveness is ambiguous") {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return fmt.Errorf("selected canonical interactive attachment did not become absent after its container stopped")
}

// FinalWorkspaceSessionOwner is the narrow host-runtime owner returned to the
// dormant final-authority bridge. It retains one canonical WP07 attachment;
// Run reuses it and therefore cannot create a second registry record, epoch,
// nonce, lease, heartbeat, or wait registry.
type FinalWorkspaceSessionOwner struct {
	runtime    *Runtime
	binding    tobari.WorkspaceSessionBinding
	principal  interactiveWorkspacePrincipal
	attachment *interactiveWorkspaceAttachment
	runMu      sync.Mutex
	runStarted bool
}

type finalWorkspaceContainerObservation struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	Workspace string `json:"workspace"`
	Role      string `json:"role"`
	Spec      string `json:"spec"`
	Running   bool   `json:"running"`
	Health    string `json:"health"`
}

func (o finalWorkspaceContainerObservation) validateFor(workspaceID tobari.WorkspaceID, resolvedSpec tobari.SemanticDigest, exactContainerID string) error {
	if o.Owner != ownerValue || o.Component != "tobari" || o.Workspace != string(workspaceID) || o.Role != projectWorkRole ||
		o.Spec != string(resolvedSpec) || !o.Running || o.Health != "healthy" || !runtimeLifecycleContainerID.MatchString(o.ID) || exactContainerID != "" && o.ID != exactContainerID {
		return fmt.Errorf("final Workspace container does not match its exact AppliedEntry authority")
	}
	return nil
}

func (r *Runtime) confirmFinalWorkspaceContainer(ctx context.Context, binding tobari.WorkspaceSessionBinding) error {
	if err := binding.Validate(); err != nil {
		return err
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}},` +
		`"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, binding.ContainerID}, os.Environ())
	if err != nil {
		return fmt.Errorf("observe exact final Workspace container: %w: %s", err, boundedDiagnostic(output))
	}
	var observed finalWorkspaceContainerObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return fmt.Errorf("decode exact final Workspace container: %w", err)
	}
	return observed.validateFor(binding.WorkspaceID, binding.AppliedEntry.ResolvedSpec, binding.ContainerID)
}

// BeginFinalWorkspaceSession validates the complete final binding and exact
// owned Docker observation before borrowing or issuing the canonical WP07
// owner. TemplateID and resolved-spec authority stop here and never enter the
// frozen private wire or become session selectors.
func (r *Runtime) BeginFinalWorkspaceSession(ctx context.Context, binding tobari.WorkspaceSessionBinding) (*FinalWorkspaceSessionOwner, error) {
	if r == nil {
		return nil, fmt.Errorf("Docker runtime is unavailable")
	}
	principal, err := finalInteractiveWorkspacePrincipal(binding)
	if err != nil {
		return nil, err
	}
	if err := r.confirmFinalWorkspaceContainer(ctx, binding); err != nil {
		return nil, err
	}
	attachment, err := r.beginInteractiveWorkspaceAttachmentForPrincipal(ctx, principal)
	if err != nil {
		return nil, err
	}
	return &FinalWorkspaceSessionOwner{
		runtime: r, binding: binding.Clone(), principal: principal, attachment: attachment,
	}, nil
}

func (o *FinalWorkspaceSessionOwner) Run(
	ctx context.Context, request tobari.WorkspaceSessionRequest,
	in io.Reader, out, errOut io.Writer,
) (tobari.WorkspaceSessionOutcome, error) {
	return o.run(ctx, request, in, out, errOut, nil)
}

// RunWithHandoff reports the exact point after all Tobari-owned attachment
// setup and immediately before the child runner takes stream ownership.
func (o *FinalWorkspaceSessionOwner) RunWithHandoff(
	ctx context.Context, request tobari.WorkspaceSessionRequest,
	in io.Reader, out, errOut io.Writer, handoff func(),
) (tobari.WorkspaceSessionOutcome, error) {
	return o.run(ctx, request, in, out, errOut, handoff)
}

func (o *FinalWorkspaceSessionOwner) run(
	ctx context.Context, request tobari.WorkspaceSessionRequest,
	in io.Reader, out, errOut io.Writer, handoff func(),
) (tobari.WorkspaceSessionOutcome, error) {
	if o == nil || o.runtime == nil || o.attachment == nil {
		return tobari.WorkspaceSessionOutcome{}, fmt.Errorf("final Workspace session owner is unavailable")
	}
	o.runMu.Lock()
	if o.runStarted {
		o.runMu.Unlock()
		return tobari.WorkspaceSessionOutcome{}, fmt.Errorf("final Workspace session owner already ran")
	}
	o.runStarted = true
	o.runMu.Unlock()
	if err := o.binding.Validate(); err != nil {
		return tobari.WorkspaceSessionOutcome{}, err
	}
	if err := o.runtime.confirmFinalWorkspaceContainer(ctx, o.binding); err != nil {
		return tobari.WorkspaceSessionOutcome{}, err
	}
	if err := o.runtime.confirmExactInteractiveWorkspaceAttachment(ctx, o.principal, o.attachment.session); err != nil {
		return tobari.WorkspaceSessionOutcome{}, fmt.Errorf("confirm exact final Workspace session owner: %w", err)
	}
	return o.runtime.runWorkspaceSessionWithHandoff(
		ctx, o.principal, o.binding.SessionDefaults.ShellEnvironment,
		o.binding.ProjectRoot, o.binding.ContainerID, request,
		o.attachment, in, out, errOut, handoff,
	)
}

func (o *FinalWorkspaceSessionOwner) Close(ctx context.Context) error {
	if o == nil || o.attachment == nil {
		return nil
	}
	return o.attachment.Close(ctx)
}

// ObserveFinalWorkspaceSession reads the one canonical WP07 registry under
// its existing lock. Absent means no exact record exists. Any malformed,
// stale, expired, replaced, or unresponsive authority returns an error so a
// Workspace deletion adapter cannot mistake observation failure for zero live
// owners.
func (r *Runtime) ObserveFinalWorkspaceSession(ctx context.Context, identity tobari.WorkspaceSessionIdentity) (FinalWorkspaceSessionState, error) {
	if r == nil {
		return "", fmt.Errorf("Docker runtime is unavailable")
	}
	principal, err := finalInteractiveWorkspacePrincipalFromIdentity(identity)
	if err != nil {
		return "", err
	}
	var observed *tobari.InteractiveAttachmentSession
	err = r.withInteractiveAttachmentLock(ctx, func() error {
		if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := requireOwnerOnlyRegularFile(r.interactiveAttachmentSessionRegistryPath()); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		current := findInteractiveSession(registry, principal.contextID, principal.workspaceID)
		if current == nil {
			return nil
		}
		copy := *current
		observed = &copy
		return nil
	})
	if err != nil {
		return "", err
	}
	if observed == nil {
		return FinalWorkspaceSessionAbsent, nil
	}
	fingerprint, err := r.exactFrozenPrincipalFingerprint(principal)
	if err != nil {
		return "", fmt.Errorf("bind canonical interactive attachment principal: %w", err)
	}
	if observed.FrozenPrincipalFingerprint != fingerprint || !permissionSessionLeaseCurrent(*observed, time.Now()) {
		return "", fmt.Errorf("canonical interactive attachment authority is stale or ambiguous")
	}
	if !r.permissionSessionActive(*observed) {
		return "", fmt.Errorf("canonical interactive attachment liveness is ambiguous")
	}
	err = r.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		current := findInteractiveSession(registry, principal.contextID, principal.workspaceID)
		if current == nil || !sameInteractiveSessionAuthority(*current, *observed) {
			return fmt.Errorf("canonical interactive attachment changed during liveness observation")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return FinalWorkspaceSessionLive, nil
}
