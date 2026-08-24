package dockerruntime

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// PrepareFinalTemplateAWSBootstrap exposes only the fixed, bounded host-source
// normalizer to the final Template mutator. It performs no Manifest selection
// and receives no final Store path.
func (r *Runtime) PrepareFinalTemplateAWSBootstrap(ctx context.Context, profile string) (tobari.ManifestBootstrapSnapshot, error) {
	return r.PrepareContextAWSBootstrap(ctx, profile)
}

// PrepareFinalTemplateEKSBootstrap composes the fixed host kubeconfig source
// with the exact current final Template AWS authority supplied under the
// installation lifecycle lock.
func (r *Runtime) PrepareFinalTemplateEKSBootstrap(ctx context.Context, base tobari.ManifestBootstrapSnapshot, name string) (tobari.ManifestBootstrapSnapshot, error) {
	return r.PrepareContextEKSBootstrap(ctx, base, name)
}
