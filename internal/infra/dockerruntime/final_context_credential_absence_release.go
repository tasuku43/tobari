//go:build !tobari_dev || !tobari_research

package dockerruntime

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ConfirmContextCredentialAbsent is the release-surface Context deletion
// prerequisite. Research credential acquisition is not compiled into this
// surface, so exact global absence of the two private auth roots proves the
// requested Context cannot own a Broker credential. Any root presence is
// ambiguity and fails closed without decoding provider or vault contents.
func (r *Runtime) ConfirmContextCredentialAbsent(ctx context.Context, contextID tobari.ContextID) error {
	if r == nil {
		return fmt.Errorf("release Context credential absence authority is unavailable")
	}
	if err := contextID.Validate(); err != nil {
		return err
	}
	return confirmLegacyPathsAbsent(ctx, []string{
		filepath.Join(r.stateDirectory, "auth"),
		filepath.Join(r.configDirectory, "auth"),
	})
}
