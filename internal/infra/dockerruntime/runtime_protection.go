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
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		entries, err := os.ReadDir(r.contextsDirectory())
		if errors.Is(err, os.ErrNotExist) {
			entries = nil
		} else if err != nil {
			return err
		}
		manifestByID := map[string]tobari.WorkspaceManifest{}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			manifest, err := r.readContextManifest(entry.Name())
			if err != nil {
				return err
			}
			manifestByID[manifest.ID] = manifest
			if manifest.RuntimeBinding != nil {
				result.Items = append(result.Items, runtimeManifestProtection(manifest, tobari.RuntimeProtectedByManifestCurrent))
			}
			retained, err := r.readRetainedManifestRevisions(manifest)
			if err != nil {
				return err
			}
			for _, revision := range retained {
				if revision.Desired.Revision == manifest.Desired.Revision || revision.RuntimeBinding == nil {
					continue
				}
				result.Items = append(result.Items, runtimeManifestProtection(revision, tobari.RuntimeProtectedByManifestRetained))
			}
		}
		workspaces, err := r.ListProjects(lockContext)
		if err != nil {
			return err
		}
		for _, workspace := range workspaces {
			if workspace.Incomplete {
				continue
			}
			manifest, ok := manifestByID[workspace.WorkspaceManifestID]
			if !ok {
				return fmt.Errorf("Workspace references an unavailable Manifest")
			}
			if workspace.LastSuccessfulEntry != nil {
				reason := tobari.RuntimeProtectedByWorkspaceApplied
				if workspace.Runtime.ContainerID != "" || workspace.Runtime.NetworkID != "" {
					reason = tobari.RuntimeProtectedByWorkspaceObserved
				}
				result.Items = append(result.Items, tobari.RuntimeProtection{
					RuntimeID: workspace.LastSuccessfulEntry.RuntimeID, RuntimeRevision: workspace.LastSuccessfulEntry.RuntimeRevision,
					Reason: reason, WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
				})
			}
			if manifest.RuntimeBinding != nil && (workspace.LastSuccessfulEntry == nil || workspace.LastSuccessfulEntry.EntryRevision != manifest.Desired.EntryRevision) {
				result.Items = append(result.Items, tobari.RuntimeProtection{
					RuntimeID: manifest.RuntimeBinding.RuntimeID, RuntimeRevision: manifest.RuntimeBinding.Revision,
					Reason: tobari.RuntimeProtectedByWorkspacePending, WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
				})
			}
		}
		return nil
	})
	if err != nil {
		return tobari.RuntimeProtectionInventory{}, err
	}
	sort.Slice(result.Items, func(i, j int) bool {
		left, right := result.Items[i], result.Items[j]
		return left.RuntimeID+left.RuntimeRevision+string(left.Reason)+left.WorkspaceManifestID+left.WorkspaceID <
			right.RuntimeID+right.RuntimeRevision+string(right.Reason)+right.WorkspaceManifestID+right.WorkspaceID
	})
	return result, result.Validate()
}

func runtimeManifestProtection(manifest tobari.WorkspaceManifest, reason tobari.RuntimeProtectionReason) tobari.RuntimeProtection {
	return tobari.RuntimeProtection{
		RuntimeID: manifest.RuntimeBinding.RuntimeID, RuntimeRevision: manifest.RuntimeBinding.Revision,
		Reason: reason, WorkspaceManifestID: manifest.ID,
	}
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
