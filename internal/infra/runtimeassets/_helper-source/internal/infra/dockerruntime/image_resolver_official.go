//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type officialImageResolver struct{}

func (officialImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	return buildidentity.Identity{
		Version: version, Commit: buildidentity.NormalizeCommit(commit),
		ResolverChannel:   buildidentity.ResolverEmbedded,
		CapabilitySurface: capabilitysurface.Compiled(),
		Gateway: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredGatewayAPI, SelectedAPI: buildidentity.RequiredGatewayAPI,
		},
	}, nil
}

func newImageResolver() imageResolver {
	return officialImageResolver{}
}

func (officialImageResolver) DefaultRuntimeImage() (string, error) {
	return runtimeassets.StandardRuntimeImage()
}

func (officialImageResolver) ShouldPullRuntimeImage(string) bool {
	return false
}

func (officialImageResolver) ShouldBuildRuntimeImage(image string) bool {
	selected, err := runtimeassets.StandardRuntimeImage()
	return err == nil && image == selected
}

func (officialImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	version, err := runtimeassets.ComponentVersion("gateway")
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: "tobari-gateway:base-" + version, BuildIfMissing: true}, nil
}

func (officialImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return sharedImageSelection{}, fmt.Errorf("Auth Broker is unavailable on the release surface")
}
