package workspaceauthoritystore

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// StatusHomeRuntime is kept explicit because the attachment seam's concrete
// enum is infrastructure-owned. Implementations map it to the domain summary
// without exposing registry records or transport details.
type StatusHomeRuntime interface {
	ObserveFinalCluster(context.Context, tobari.WorkspaceAuthorityCollection, bool) (tobari.FinalClusterStatus, error)
	ObserveStatusRuntime(context.Context, tobari.RuntimeBinding) (tobari.StatusRuntimeObservation, error)
	ObserveStatusWorkspace(context.Context, tobari.ContextAuthoritySnapshot) (tobari.StatusWorkspaceObservation, error)
	ObserveStatusAttachment(context.Context, tobari.WorkspaceSessionIdentity) (tobari.StatusAttachmentState, error)
	ObserveStatusServices(context.Context, tobari.ContextID, tobari.WorkspaceID) (tobari.ServiceSummary, error)
}

type StatusHomeAdapter struct {
	store   *Store
	root    FinalCanonicalProjectRootAuthority
	runtime StatusHomeRuntime
}

func NewStatusHomeAdapter(store *Store, root FinalCanonicalProjectRootAuthority, runtime StatusHomeRuntime) (*StatusHomeAdapter, error) {
	if store == nil || store.legacyGuard == nil || root == nil || runtime == nil {
		return nil, fmt.Errorf("pure status-home authority is unavailable")
	}
	return &StatusHomeAdapter{store: store, root: root, runtime: runtime}, nil
}

func (a *StatusHomeAdapter) ObserveStatusHome(ctx context.Context) (tobari.StatusHomeObservation, error) {
	first, err := a.observeStatusHomeAttempt(ctx)
	if err == nil {
		return first, nil
	}
	second, retryErr := a.observeStatusHomeAttempt(ctx)
	if retryErr != nil {
		return tobari.StatusHomeObservation{}, fmt.Errorf("status snapshot changed after bounded retry: %w", retryErr)
	}
	return second, nil
}

func (a *StatusHomeAdapter) observeStatusHomeAttempt(ctx context.Context) (tobari.StatusHomeObservation, error) {
	cwd, err := a.root.CurrentDirectory(ctx)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	cwdRoot, err := a.root.ResolveProjectRoot(ctx, cwd)
	if err != nil || tobari.ValidateCanonicalRoot(cwdRoot) != nil {
		return tobari.StatusHomeObservation{}, fmt.Errorf("canonical status Project root is unavailable: %w", err)
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	recovery, err := a.store.ObserveMutationRecovery(ctx)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	root, err := nearestStatusProjectRoot(collection, present, cwdRoot)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	result := tobari.StatusHomeObservation{Collection: collection.Clone(), Present: present, ProjectRoot: root}
	if recovery.ActiveDecision || recovery.StagePresent {
		result.Live.MutationRecovery = &recovery
	}
	selected, err := statusSelectedSnapshot(collection, present, root)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	if selected != nil {
		cluster, err := a.runtime.ObserveFinalCluster(ctx, collection, present)
		if err != nil {
			return tobari.StatusHomeObservation{}, err
		}
		result.Live.Cluster = &cluster
		result.Live.Runtime, err = a.runtime.ObserveStatusRuntime(ctx, selected.Template.Current.Body.EntryDefaults.Runtime)
		if err != nil {
			return tobari.StatusHomeObservation{}, err
		}
		if selected.Workspace != nil && selected.Workspace.LastSuccessfulEntry != nil {
			result.Live.Workspace, err = a.runtime.ObserveStatusWorkspace(ctx, *selected)
			if err != nil {
				return tobari.StatusHomeObservation{}, err
			}
			identity, identityErr := tobari.NewWorkspaceSessionIdentity(*selected)
			if identityErr != nil {
				return tobari.StatusHomeObservation{}, identityErr
			}
			result.Live.Attachment, err = a.runtime.ObserveStatusAttachment(ctx, identity)
			if err != nil {
				return tobari.StatusHomeObservation{}, err
			}
			services, serviceErr := a.runtime.ObserveStatusServices(ctx, selected.Context.ID, selected.Workspace.ID)
			if serviceErr != nil {
				return tobari.StatusHomeObservation{}, serviceErr
			}
			result.Live.Services = &services
		}
	}
	if err := a.store.ConfirmSelected(ctx, collection, present); err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	cwdAfter, err := a.root.CurrentDirectory(ctx)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	cwdRootAfter, err := a.root.ResolveProjectRoot(ctx, cwdAfter)
	if err != nil {
		return tobari.StatusHomeObservation{}, err
	}
	rootAfter, err := nearestStatusProjectRoot(collection, present, cwdRootAfter)
	if err != nil || rootAfter != root {
		return tobari.StatusHomeObservation{}, fmt.Errorf("canonical Project root changed during status")
	}
	return result, nil
}

// nearestStatusProjectRoot selects root authority before applying the default
// Template. Existing Context roots may anchor a CWD descendant; siblings and
// the default Template cannot redirect that root selection.
func nearestStatusProjectRoot(collection tobari.WorkspaceAuthorityCollection, present bool, cwdRoot string) (string, error) {
	if tobari.ValidateCanonicalRoot(cwdRoot) != nil {
		return "", fmt.Errorf("status CWD root is invalid")
	}
	if !present {
		return cwdRoot, nil
	}
	if err := collection.Validate(); err != nil {
		return "", err
	}
	selected := cwdRoot
	selectedDepth := -1
	for _, record := range collection.Contexts {
		candidate := record.Context.ProjectRoot
		relative, err := filepath.Rel(candidate, cwdRoot)
		if err != nil {
			return "", fmt.Errorf("compare status Project roots: %w", err)
		}
		if relative != "." && (relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			continue
		}
		depth := len(strings.Split(filepath.Clean(candidate), string(filepath.Separator)))
		if depth > selectedDepth {
			selected, selectedDepth = candidate, depth
		}
	}
	return selected, nil
}

func statusSelectedSnapshot(collection tobari.WorkspaceAuthorityCollection, present bool, root string) (*tobari.ContextAuthoritySnapshot, error) {
	if !present || collection.DefaultTemplateID == nil {
		return nil, nil
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.ProjectRoot == root && snapshot.Context.TemplateID == *collection.DefaultTemplateID {
			copy := snapshot.Clone()
			return &copy, nil
		}
	}
	return nil, nil
}
