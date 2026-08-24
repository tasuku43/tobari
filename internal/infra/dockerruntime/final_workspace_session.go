package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
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
	return o.runtime.runWorkspaceSession(
		ctx, o.principal, o.binding.SessionDefaults.ShellEnvironment,
		o.binding.ProjectRoot, o.binding.ContainerID, request,
		o.attachment, in, out, errOut,
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
