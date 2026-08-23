package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeWorkspaceContainerAuthority struct {
	ContainerID  string
	WorkspaceID  string
	ResolvedSpec string
	RuntimeID    string
	Revision     string
}

type runtimeProtectionObservation struct {
	Inventory  tobari.RuntimeProtectionInventory
	Containers map[string]runtimeWorkspaceContainerAuthority
}

// ReadRuntimeProtectionInventory returns the complete, lock-consistent graph
// needed by future Runtime retirement. It performs no retirement decision and
// intentionally does not derive last-used from reconciliation timestamps.
func (r *Runtime) ReadRuntimeProtectionInventory(ctx context.Context) (tobari.RuntimeProtectionInventory, error) {
	var result runtimeProtectionObservation
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		observationContext, cancel := context.WithTimeout(lockContext, runtimeLifecycleWallBudget)
		defer cancel()
		budget := runtimeLifecycleBudget{remaining: runtimeLifecycleCallBudget}
		var readErr error
		result, readErr = r.readRuntimeProtectionInventoryObserved(observationContext, &budget)
		return readErr
	})
	if err != nil {
		return tobari.RuntimeProtectionInventory{}, err
	}
	return result.Inventory, nil
}

// readRuntimeProtectionInventoryObserved requires the caller to hold the
// lifecycle observation/effect lock. Keeping the join lock outside this helper
// lets lifecycle planning observe catalog, protection, and Docker evidence as
// one coherent zero-write snapshot.
func (r *Runtime) readRuntimeProtectionInventoryObserved(ctx context.Context, budget *runtimeLifecycleBudget) (runtimeProtectionObservation, error) {
	result := runtimeProtectionObservation{Inventory: tobari.RuntimeProtectionInventory{Complete: true, Items: []tobari.RuntimeProtection{}}, Containers: make(map[string]runtimeWorkspaceContainerAuthority)}
	entries, err := os.ReadDir(r.contextsDirectory())
	if errors.Is(err, os.ErrNotExist) {
		entries = nil
	} else if err != nil {
		return runtimeProtectionObservation{}, err
	}
	manifestByID := map[string]tobari.WorkspaceManifest{}
	for _, entry := range entries {
		if entry.Name() == "default.json" && entry.Type().IsRegular() {
			continue
		}
		if entry.Name() == "active.json" {
			return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
		}
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		manifest, err := r.readContextManifest(entry.Name())
		if err != nil {
			return runtimeProtectionObservation{}, err
		}
		manifestByID[manifest.ID] = manifest
		if manifest.RuntimeBinding != nil && manifest.RuntimeBinding.RuntimeID != tobari.StandardRuntimeID {
			result.Inventory.Items = append(result.Inventory.Items, runtimeManifestProtection(manifest, tobari.RuntimeProtectedByManifestCurrent))
		}
		retained, err := r.readRetainedManifestRevisions(manifest)
		if err != nil {
			return runtimeProtectionObservation{}, err
		}
		for _, revision := range retained {
			if revision.Desired.Revision == manifest.Desired.Revision || revision.RuntimeBinding == nil || revision.RuntimeBinding.RuntimeID == tobari.StandardRuntimeID {
				continue
			}
			result.Inventory.Items = append(result.Inventory.Items, runtimeManifestProtection(revision, tobari.RuntimeProtectedByManifestRetained))
		}
	}
	workspaces, err := r.listProjectsForRuntimeProtection(ctx)
	if err != nil {
		return runtimeProtectionObservation{}, err
	}
	for _, workspace := range workspaces {
		if workspace.Incomplete {
			return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
		}
		manifest, ok := manifestByID[workspace.WorkspaceManifestID]
		if !ok {
			return runtimeProtectionObservation{}, fmt.Errorf("Workspace references an unavailable Manifest")
		}
		if workspace.Runtime.NetworkID != "" && workspace.Runtime.ContainerID == "" {
			return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
		}
		if workspace.LastSuccessfulEntry != nil {
			observed, observeErr := r.observeWorkspaceRuntimeProtection(ctx, workspace, budget)
			if observeErr != nil {
				return runtimeProtectionObservation{}, observeErr
			}
			if protection, ok := runtimeWorkspaceProtection(workspace, observed); ok {
				result.Inventory.Items = append(result.Inventory.Items, protection)
				if observed {
					if _, exists := result.Containers[workspace.Runtime.ContainerID]; exists {
						return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
					}
					result.Containers[workspace.Runtime.ContainerID] = runtimeWorkspaceContainerAuthority{ContainerID: workspace.Runtime.ContainerID, WorkspaceID: workspace.ID, ResolvedSpec: workspace.LastSuccessfulEntry.ResolvedSpec, RuntimeID: workspace.LastSuccessfulEntry.RuntimeID, Revision: workspace.LastSuccessfulEntry.RuntimeRevision}
				}
			}
		} else if workspace.Runtime.ContainerID != "" || workspace.Runtime.NetworkID != "" {
			return runtimeProtectionObservation{}, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
		}
		if manifest.RuntimeBinding != nil && manifest.RuntimeBinding.RuntimeID != tobari.StandardRuntimeID &&
			(workspace.LastSuccessfulEntry == nil || workspace.LastSuccessfulEntry.EntryRevision != manifest.Desired.EntryRevision) {
			result.Inventory.Items = append(result.Inventory.Items, tobari.RuntimeProtection{
				RuntimeID: manifest.RuntimeBinding.RuntimeID, RuntimeRevision: manifest.RuntimeBinding.Revision,
				Reason: tobari.RuntimeProtectedByWorkspacePending, WorkspaceManifestID: workspace.WorkspaceManifestID,
				ManifestRevision: manifest.Desired.Revision, WorkspaceID: workspace.ID,
			})
		}
	}
	sort.Slice(result.Inventory.Items, func(i, j int) bool {
		return runtimeProtectionSortKey(result.Inventory.Items[i]) < runtimeProtectionSortKey(result.Inventory.Items[j])
	})
	return result, result.Inventory.Validate()
}

func runtimeProtectionSortKey(item tobari.RuntimeProtection) string {
	return item.RuntimeID + "\x00" + item.RuntimeRevision + "\x00" + string(item.Reason) + "\x00" +
		item.WorkspaceManifestID + "\x00" + item.ManifestRevision + "\x00" + item.WorkspaceID
}

func runtimeWorkspaceProtection(workspace tobari.Workspace, observed bool) (tobari.RuntimeProtection, bool) {
	if workspace.LastSuccessfulEntry == nil || workspace.LastSuccessfulEntry.RuntimeID == tobari.StandardRuntimeID {
		return tobari.RuntimeProtection{}, false
	}
	reason := tobari.RuntimeProtectedByWorkspaceApplied
	if observed {
		reason = tobari.RuntimeProtectedByWorkspaceObserved
	}
	return tobari.RuntimeProtection{
		RuntimeID: workspace.LastSuccessfulEntry.RuntimeID, RuntimeRevision: workspace.LastSuccessfulEntry.RuntimeRevision,
		Reason: reason, WorkspaceManifestID: workspace.WorkspaceManifestID,
		ManifestRevision: workspace.LastSuccessfulEntry.ManifestRevision, WorkspaceID: workspace.ID,
	}, true
}

func runtimeManifestProtection(manifest tobari.WorkspaceManifest, reason tobari.RuntimeProtectionReason) tobari.RuntimeProtection {
	return tobari.RuntimeProtection{
		RuntimeID: manifest.RuntimeBinding.RuntimeID, RuntimeRevision: manifest.RuntimeBinding.Revision,
		Reason: reason, WorkspaceManifestID: manifest.ID, ManifestRevision: manifest.Desired.Revision,
	}
}

type workspaceRuntimeObservation struct {
	ID        string `json:"id"`
	Owner     string `json:"owner"`
	Component string `json:"component"`
	Workspace string `json:"workspace"`
	Role      string `json:"role"`
	Spec      string `json:"spec"`
}

func (r *Runtime) observeWorkspaceRuntimeProtection(ctx context.Context, workspace tobari.Workspace, budget *runtimeLifecycleBudget) (bool, error) {
	if workspace.Runtime.ContainerID == "" {
		return false, nil
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	output, diagnostic, err := budget.run(ctx, r.runner, []string{"container", "inspect", "--format", format, workspace.Runtime.ContainerID}, os.Environ(), 4096)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		if isMissingRuntimeContainerInspect(err, diagnostic, workspace.Runtime.ContainerID) {
			return false, nil
		}
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	var observed workspaceRuntimeObservation
	if err := decodeStrictJSON(output, &observed); err != nil {
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	if observed.ID != workspace.Runtime.ContainerID || observed.Owner != ownerValue || observed.Component != "tobari" ||
		observed.Workspace != workspace.ID || observed.Role != projectWorkRole || workspace.LastSuccessfulEntry == nil ||
		observed.Spec != workspace.LastSuccessfulEntry.ResolvedSpec {
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	return true, nil
}

func isMissingRuntimeContainerInspect(err error, diagnostic []byte, containerID string) bool {
	if err == nil || !runtimeLifecycleContainerID.MatchString(containerID) {
		return false
	}
	message := strings.TrimSuffix(string(diagnostic), "\n")
	message = strings.TrimSuffix(message, "\r")
	accepted := []string{
		"Error: No such container: " + containerID,
		"Error response from daemon: No such container: " + containerID,
		"Error: No such object: " + containerID,
		"Error response from daemon: No such object: " + containerID,
	}
	return slices.Contains(accepted, message)
}

func (r *Runtime) readRetainedManifestRevisions(current tobari.WorkspaceManifest) ([]tobari.WorkspaceManifest, error) {
	entries, err := os.ReadDir(r.manifestRevisionsDirectory(current.Name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("Manifest retained revision inventory is unavailable")
	}
	if err != nil {
		return nil, err
	}
	result := make([]tobari.WorkspaceManifest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Manifest revision store contains an unsafe entry")
		}
		manifest, err := readWorkspaceManifestRevision(filepath.Join(r.manifestRevisionsDirectory(current.Name), entry.Name()))
		if err != nil {
			return nil, err
		}
		if manifest.ID != current.ID || manifest.Name != current.Name || manifest.Desired.BoundaryRevision != current.Desired.BoundaryRevision {
			return nil, fmt.Errorf("retained Manifest revision ownership is invalid")
		}
		result = append(result, manifest)
	}
	return result, nil
}
