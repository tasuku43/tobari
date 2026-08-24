package dockerruntime

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ConfirmFinalClusterAuthoritySettled proves the exact live aggregate and
// every independently selected Context receipt for terminal replay.
func (r *Runtime) ConfirmFinalClusterAuthoritySettled(ctx context.Context, current tobari.WorkspaceAuthorityCollection) error {
	plan, err := tobari.BuildClusterWorkspacePolicyProjection(current)
	if err != nil {
		return err
	}
	return r.confirmFinalPolicyProjection(ctx, plan)
}
