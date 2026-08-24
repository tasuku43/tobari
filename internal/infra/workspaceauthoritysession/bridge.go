// Package workspaceauthoritysession is the host-only composition seam between
// the final Workspace authority coordinator and dockerruntime's canonical
// WP07 attachment implementation. Keeping this sibling package prevents the
// exposure-helper dockerruntime closure from importing the final store.
package workspaceauthoritysession

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type Bridge struct {
	runtime *dockerruntime.Runtime
}

func New(runtime *dockerruntime.Runtime) (*Bridge, error) {
	if runtime == nil {
		return nil, fmt.Errorf("Docker runtime is required")
	}
	return &Bridge{runtime: runtime}, nil
}

func (b *Bridge) BeginWorkspaceSession(ctx context.Context, binding tobari.WorkspaceSessionBinding) (workspaceauthoritystore.WorkspaceSessionOwner, error) {
	if b == nil || b.runtime == nil {
		return nil, fmt.Errorf("final Workspace session bridge is unavailable")
	}
	return b.runtime.BeginFinalWorkspaceSession(ctx, binding)
}

var _ workspaceauthoritystore.WorkspaceSessionAuthority = (*Bridge)(nil)
