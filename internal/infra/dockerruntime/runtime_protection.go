package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ReadRuntimeProtectionInventory returns the complete, lock-consistent graph
// needed by future Runtime retirement. It performs no retirement decision and
// intentionally does not derive last-used from reconciliation timestamps.
func (r *Runtime) ReadRuntimeProtectionInventory(ctx context.Context) (tobari.RuntimeProtectionInventory, error) {
	result := tobari.RuntimeProtectionInventory{Complete: true, Items: []tobari.RuntimeProtection{}}
	err := r.withLifecycleObservation(ctx, func(lockContext context.Context) error {
		entries, err := os.ReadDir(r.contextsDirectory())
		if errors.Is(err, os.ErrNotExist) {
			entries = nil
		} else if err != nil {
			return err
		}
		manifestByID := map[string]tobari.WorkspaceManifest{}
		for _, entry := range entries {
			if entry.Name() == "default.json" && entry.Type().IsRegular() {
				continue
			}
			if entry.Name() == "active.json" {
				return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
			}
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
			}
			manifest, err := r.readContextManifest(entry.Name())
			if err != nil {
				return err
			}
			manifestByID[manifest.ID] = manifest
			if manifest.RuntimeBinding != nil && manifest.RuntimeBinding.RuntimeID != tobari.StandardRuntimeID {
				result.Items = append(result.Items, runtimeManifestProtection(manifest, tobari.RuntimeProtectedByManifestCurrent))
			}
			retained, err := r.readRetainedManifestRevisions(manifest)
			if err != nil {
				return err
			}
			for _, revision := range retained {
				if revision.Desired.Revision == manifest.Desired.Revision || revision.RuntimeBinding == nil || revision.RuntimeBinding.RuntimeID == tobari.StandardRuntimeID {
					continue
				}
				result.Items = append(result.Items, runtimeManifestProtection(revision, tobari.RuntimeProtectedByManifestRetained))
			}
		}
		workspaces, err := r.listProjectsForRuntimeProtection(lockContext)
		if err != nil {
			return err
		}
		for _, workspace := range workspaces {
			if workspace.Incomplete {
				return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryIncomplete}
			}
			manifest, ok := manifestByID[workspace.WorkspaceManifestID]
			if !ok {
				return fmt.Errorf("Workspace references an unavailable Manifest")
			}
			if workspace.Runtime.NetworkID != "" && workspace.Runtime.ContainerID == "" {
				return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
			}
			if workspace.LastSuccessfulEntry != nil {
				observed, observeErr := r.observeWorkspaceRuntimeProtection(lockContext, workspace)
				if observeErr != nil {
					return observeErr
				}
				if protection, ok := runtimeWorkspaceProtection(workspace, observed); ok {
					result.Items = append(result.Items, protection)
				}
			} else if workspace.Runtime.ContainerID != "" || workspace.Runtime.NetworkID != "" {
				return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryMigrationUnverified}
			}
			if manifest.RuntimeBinding != nil && manifest.RuntimeBinding.RuntimeID != tobari.StandardRuntimeID &&
				(workspace.LastSuccessfulEntry == nil || workspace.LastSuccessfulEntry.EntryRevision != manifest.Desired.EntryRevision) {
				result.Items = append(result.Items, tobari.RuntimeProtection{
					RuntimeID: manifest.RuntimeBinding.RuntimeID, RuntimeRevision: manifest.RuntimeBinding.Revision,
					Reason: tobari.RuntimeProtectedByWorkspacePending, WorkspaceManifestID: workspace.WorkspaceManifestID,
					ManifestRevision: manifest.Desired.Revision, WorkspaceID: workspace.ID,
				})
			}
		}
		return nil
	})
	if err != nil {
		return tobari.RuntimeProtectionInventory{}, err
	}
	sort.Slice(result.Items, func(i, j int) bool {
		return runtimeProtectionSortKey(result.Items[i]) < runtimeProtectionSortKey(result.Items[j])
	})
	return result, result.Validate()
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

func (r *Runtime) observeWorkspaceRuntimeProtection(ctx context.Context, workspace tobari.Workspace) (bool, error) {
	if workspace.Runtime.ContainerID == "" {
		return false, nil
	}
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"spec":{{json (index .Config.Labels "` + projectSpecLabel + `")}}}`
	output, err := r.runner.Output(ctx, []string{"container", "inspect", "--format", format, workspace.Runtime.ContainerID}, os.Environ())
	if err != nil {
		if isMissingDockerResource(err, output) {
			return false, nil
		}
		return false, tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
	}
	if len(output) > 4096 {
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
