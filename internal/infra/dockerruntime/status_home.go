package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ObserveStatusRuntime reads one exact Template-bound Runtime revision. It
// performs at most one Docker image inspection and never invokes lifecycle
// recovery, pruning, restore, build, or mutation.
func (r *Runtime) ObserveStatusRuntime(ctx context.Context, binding tobari.RuntimeBinding) (result tobari.StatusRuntimeObservation, resultErr error) {
	result = tobari.StatusRuntimeObservation{Authority: tobari.StatusRuntimeAuthorityUnknown, Availability: tobari.RuntimeAvailabilityUnknown, Compatibility: tobari.StatusNativeUnknown}
	if r == nil || binding.Validate() != nil {
		return result, fmt.Errorf("status Runtime binding is invalid")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	manifest, err := r.standardRuntimeManifest()
	if err != nil {
		return result, err
	}
	if binding.RuntimeID != tobari.StandardRuntimeID {
		var err error
		manifest, err = r.resolveManagedRuntimeReferenceUnlocked(tobari.RuntimeRef(binding.RuntimeID))
		if errors.Is(err, tobari.ErrRuntimeNotFound) {
			result.Authority = tobari.StatusRuntimeAuthorityNotReady
			return result, nil
		}
		if err != nil {
			return result, err
		}
		defer func() {
			if resultErr != nil {
				return
			}
			confirmed, err := r.resolveManagedRuntimeReferenceUnlocked(tobari.RuntimeRef(binding.RuntimeID))
			if err != nil || !reflect.DeepEqual(confirmed, manifest) {
				resultErr = fmt.Errorf("status Runtime authority changed during observation")
			}
		}()
	}
	expected, err := manifest.Binding(binding.Ordinal)
	if err != nil || !reflect.DeepEqual(expected, binding) {
		result.Authority = tobari.StatusRuntimeAuthorityNotReady
		return result, nil
	}
	result.Authority = tobari.StatusRuntimeAuthorityReady

	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},"runtime":{{json (index .Config.Labels "` + managedRuntimeIDLabel + `")}},` +
		`"revision":{{json (index .Config.Labels "` + managedRuntimeRevisionLabel + `")}},"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},` +
		`"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`
	stdout, stderr := &boundedBuffer{limit: 4096}, &boundedBuffer{limit: 4096}
	inspectErr := r.runner.Run(ctx, []string{"image", "inspect", "--format", format, binding.Image}, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return result, fmt.Errorf("status Runtime evidence exceeds its bound")
	}
	if inspectErr != nil {
		if isMissingRuntimeImageInspect(inspectErr, stderr.buffer.Bytes(), binding.Image) || isMissingRuntimeImageInspect(inspectErr, stdout.buffer.Bytes(), binding.Image) {
			result.Availability = tobari.RuntimeAvailabilityMissing
			return result, nil
		}
		return result, nil
	}
	var observed struct {
		ID, Owner, Component, Runtime, Revision, API, Lifetime, User string
		Entrypoint                                                   []string `json:"entrypoint"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(stdout.buffer.Bytes()), &observed); err != nil || tobari.ValidateDigest(observed.ID) != nil {
		return result, fmt.Errorf("status Runtime evidence is invalid")
	}
	if manifest.Kind == tobari.RuntimeKindManaged {
		revision := manifest.Revisions[binding.Ordinal-1]
		if observed.Owner != ownerValue || observed.Component != managedRuntimeComponentLabel || observed.Runtime != binding.RuntimeID || observed.Revision != binding.Revision || observed.ID != revision.ImageDigest {
			result.Availability = tobari.RuntimeAvailabilityMismatched
			return result, nil
		}
	}
	result.Availability = tobari.RuntimeAvailabilityAvailable
	if observed.API == tobari.RuntimeImageAPI && observed.Lifetime == tobari.RuntimeImageLifetimeCommand && observed.User == "tobari" && equalStrings(observed.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		result.Compatibility = tobari.StatusNativeCompatible
	} else {
		result.Compatibility = tobari.StatusNativeIncompatible
	}
	return result, nil
}

// ObserveStatusWorkspace performs one exact container inspection selected by
// WorkspaceID and validates final labels before returning semantic state.
func (r *Runtime) ObserveStatusWorkspace(ctx context.Context, snapshot tobari.ContextAuthoritySnapshot) (tobari.StatusWorkspaceObservation, error) {
	result := tobari.StatusWorkspaceObservation{State: tobari.StatusWorkspaceRuntimeUnknown}
	if r == nil || snapshot.Validate() != nil || snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil {
		return result, fmt.Errorf("status Workspace authority is incomplete")
	}
	container, _, err := tobari.ProjectResourceNames(string(snapshot.Workspace.ID))
	if err != nil {
		return result, err
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}},` +
		`"running":{{json .State.Running}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}}}`
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, container}, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) || bytes.Contains(output, []byte("No such object")) {
			result.State = tobari.StatusWorkspaceRuntimeAbsent
		}
		return result, nil
	}
	var observed finalWorkspaceContainerObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return result, fmt.Errorf("status Workspace observation is invalid: %w", err)
	}
	applied := snapshot.Workspace.LastSuccessfulEntry
	if observed.Owner != ownerValue || observed.Component != "tobari" || observed.Workspace != string(snapshot.Workspace.ID) || observed.Role != projectWorkRole {
		return result, fmt.Errorf("status Workspace container has foreign ownership")
	}
	if observed.Spec != string(applied.ResolvedSpec) {
		result.State = tobari.StatusWorkspaceRuntimeDrifted
		return result, nil
	}
	if !observed.Running {
		result.State = tobari.StatusWorkspaceRuntimeStopped
		return result, nil
	}
	if observed.Health != "healthy" {
		result.State = tobari.StatusWorkspaceRuntimeDrifted
		return result, nil
	}
	result.State = tobari.StatusWorkspaceRuntimeRunning
	return result, nil
}

func (r *Runtime) ObserveStatusAttachment(ctx context.Context, identity tobari.WorkspaceSessionIdentity) (tobari.StatusAttachmentState, error) {
	if r == nil {
		return tobari.StatusAttachmentUnknown, fmt.Errorf("status attachment runtime is unavailable")
	}
	principal, err := finalInteractiveWorkspacePrincipalFromIdentity(identity)
	if err != nil {
		return tobari.StatusAttachmentUnknown, err
	}
	before, err := r.readStatusAttachmentRegistry(ctx)
	if err != nil {
		return tobari.StatusAttachmentUnknown, err
	}
	if before == nil {
		return tobari.StatusAttachmentDetached, nil
	}
	observed := findInteractiveSession(*before, principal.contextID, principal.workspaceID)
	if observed == nil {
		return tobari.StatusAttachmentDetached, nil
	}
	fingerprint, err := r.exactFrozenPrincipalFingerprint(principal)
	if err != nil || observed.FrozenPrincipalFingerprint != fingerprint || !permissionSessionLeaseCurrent(*observed, time.Now()) || !r.permissionSessionActive(*observed) {
		return tobari.StatusAttachmentUnknown, fmt.Errorf("status attachment authority is stale or ambiguous")
	}
	after, err := r.readStatusAttachmentRegistry(ctx)
	if err != nil || after == nil {
		return tobari.StatusAttachmentUnknown, fmt.Errorf("status attachment authority changed during observation")
	}
	confirmed := findInteractiveSession(*after, principal.contextID, principal.workspaceID)
	if confirmed == nil || !sameInteractiveSessionAuthority(*confirmed, *observed) {
		return tobari.StatusAttachmentUnknown, fmt.Errorf("status attachment authority changed during observation")
	}
	return tobari.StatusAttachmentAttached, nil
}

func (r *Runtime) readStatusAttachmentRegistry(ctx context.Context) (*tobari.InteractiveAttachmentSessionRegistry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := requirePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	path := r.interactiveAttachmentSessionRegistryPath()
	if err := requireOwnerOnlyRegularFile(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(path, &registry); err != nil {
		return nil, err
	}
	if err := registry.Validate(); err != nil {
		return nil, err
	}
	return &registry, nil
}
