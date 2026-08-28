package workspaceauthoritystore

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type FinalCanonicalProjectRootAuthority interface {
	CurrentDirectory(context.Context) (string, error)
	ResolveProjectRoot(context.Context, string) (string, error)
}

type finalWorkspacePresenceAuthority interface {
	InsideProject(context.Context) bool
}

// DefaultPairAdapter joins only the final Store and the non-creating canonical
// CWD resolver. It has no predecessor project, Manifest, or name fallback.
type DefaultPairAdapter struct {
	store *Store
	root  FinalCanonicalProjectRootAuthority
}

func NewDefaultPairAdapter(store *Store, root FinalCanonicalProjectRootAuthority) (*DefaultPairAdapter, error) {
	if store == nil || store.legacyGuard == nil {
		return nil, fmt.Errorf("final-only guarded Store is required")
	}
	if root == nil {
		return nil, fmt.Errorf("canonical Project-root authority is required")
	}
	return &DefaultPairAdapter{store: store, root: root}, nil
}

func (a *DefaultPairAdapter) ObserveFinalCanonicalProjectRoot(ctx context.Context) (string, error) {
	if a == nil || a.root == nil {
		return "", fmt.Errorf("canonical Project-root authority is unavailable")
	}
	cwd, err := a.root.CurrentDirectory(ctx)
	if err != nil {
		return "", err
	}
	root, err := a.root.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return "", err
	}
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return "", err
	}
	return root, nil
}

// InsideFinalWorkspace keeps the process-environment observation inside
// infrastructure while allowing final root entry to reject recursion before
// selection, review, state, or Docker work.
func (a *DefaultPairAdapter) InsideFinalWorkspace(ctx context.Context) bool {
	if a == nil || a.root == nil {
		return false
	}
	presence, ok := a.root.(finalWorkspacePresenceAuthority)
	return ok && presence.InsideProject(ctx)
}

func (a *DefaultPairAdapter) ObserveFinalDefaultPairSelection(ctx context.Context) (tobari.FinalDefaultPairSelection, error) {
	if a == nil || a.store == nil || a.root == nil {
		return tobari.FinalDefaultPairSelection{}, fmt.Errorf("final default-pair selection authority is unavailable")
	}
	cwd, err := a.root.CurrentDirectory(ctx)
	if err != nil {
		return tobari.FinalDefaultPairSelection{}, err
	}
	canonicalCWD, err := a.root.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return tobari.FinalDefaultPairSelection{}, err
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairSelection{}, err
	}
	return tobari.NewFinalDefaultPairSelection(collection, present, canonicalCWD)
}

func (a *DefaultPairAdapter) ObserveFinalDefaultPair(ctx context.Context, projectRoot string) (tobari.FinalDefaultPairObservation, error) {
	if a == nil || a.store == nil {
		return tobari.FinalDefaultPairObservation{}, fmt.Errorf("final default-pair Store is unavailable")
	}
	collection, present, err := a.store.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalDefaultPairObservation{}, err
	}
	return tobari.NewFinalDefaultPairObservation(collection, present, projectRoot)
}

func (a *DefaultPairAdapter) ObserveMutationRecovery(ctx context.Context) (tobari.FinalAuthorityMutationObservation, error) {
	if a == nil || a.store == nil {
		return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("final mutation recovery authority is unavailable")
	}
	return a.store.ObserveMutationRecovery(ctx)
}
