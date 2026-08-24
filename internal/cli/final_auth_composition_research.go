//go:build tobari_dev && tobari_research

package cli

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritydoctor"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

// configureFinalContextAuth is the research-only composition edge. The
// release build has no application service, adapter, or reachable Catalog
// path for Broker authentication.
func configureFinalContextAuth(command *CLI, lifetime context.Context, store *workspaceauthoritystore.Store, mutator *workspaceauthoritystore.Mutator, runtime *dockerruntime.Runtime) (workspaceauthoritydoctor.FinalAuthInspector, error) {
	adapter, err := workspaceauthoritystore.NewFinalContextAuthAdapter(mutator, runtime, lifetime)
	if err != nil {
		return nil, err
	}
	command.finalAuth = authcmd.NewFinalContext(store, adapter)
	return adapter, nil
}
