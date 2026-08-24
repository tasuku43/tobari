//go:build !tobari_research

package cli

import (
	"context"

	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritydoctor"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

func configureFinalContextAuth(*CLI, context.Context, *workspaceauthoritystore.Store, *workspaceauthoritystore.Mutator, *dockerruntime.Runtime) (workspaceauthoritydoctor.FinalAuthInspector, error) {
	return nil, nil
}
