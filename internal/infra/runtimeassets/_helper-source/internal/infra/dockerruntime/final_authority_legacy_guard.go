package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type preReleaseLegacyAuthorityInventory struct {
	legacyOnly              []string
	firstInitializationOnly []string
}

// preReleaseLegacyAuthorityPaths is the closed physical inventory for the
// pre-release clean break. Runtime lifecycle roots are intentionally absent:
// WP03 remains valid final authority and is not predecessor Workspace state.
// The second set is reused by final adapters, so only a clean first envelope
// may establish their lineage; their owning adapters validate exact schemas
// after initialization. The research Workspace authentication registry is
// created lazily by final Workspace entry, not by initial cluster bootstrap.
func (r *Runtime) preReleaseLegacyAuthorityPaths() preReleaseLegacyAuthorityInventory {
	return preReleaseLegacyAuthorityInventory{
		legacyOnly: []string{
			r.contextsDirectory(),
			r.rootsDirectory(),
			r.instancesDirectory(),
			r.statePath(),
			r.projectJournalPath(),
			r.clusterJournalPath(),
			filepath.Join(r.configDirectory, "migrations"),
			filepath.Join(r.stateDirectory, "migrations"),
		},
		firstInitializationOnly: []string{
			r.aggregateRoot(),
			r.principalRegistryDirectory(),
			filepath.Join(r.stateDirectory, "auth"),
			filepath.Join(r.stateDirectory, "auth", "projects"),
			filepath.Join(r.configDirectory, "auth"),
			filepath.Join(r.dataDirectory, "profiles"),
			r.hostLoopbackDirectory(),
			r.interactiveAttachmentDirectory(),
			filepath.Join(r.stateDirectory, "service-exposure"),
		},
	}
}

// ConfirmNoPreReleaseLegacyAuthority is the non-decoding clean-break guard for
// the final Workspace authority store. Legacy-only roots are forbidden for the
// lifetime of the final installation. Physical roots reused by reviewed final
// cluster/auth operations must be absent only before the first final envelope
// is published; afterward their owning adapters validate exact final schemas.
// This observation never creates, reads, moves, or removes legacy contents.
func (r *Runtime) ConfirmNoPreReleaseLegacyAuthority(ctx context.Context, finalInitialized bool) error {
	if r == nil {
		return fmt.Errorf("pre-release legacy authority guard is unavailable")
	}
	inventory := r.preReleaseLegacyAuthorityPaths()
	if err := confirmLegacyPathsAbsent(ctx, inventory.legacyOnly); err != nil {
		return err
	}
	if finalInitialized {
		return nil
	}
	return confirmLegacyPathsAbsent(ctx, inventory.firstInitializationOnly)
}

func confirmLegacyPathsAbsent(ctx context.Context, paths []string) error {
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("unsupported pre-release authority is present at a declared legacy root")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("declared pre-release authority root is ambiguous: %w", err)
		}
	}
	return nil
}
