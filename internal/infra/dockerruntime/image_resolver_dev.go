//go:build tobari_dev

package dockerruntime

import "context"

import "github.com/tasuku43/tobari/internal/domain/buildidentity"

import "github.com/tasuku43/tobari/internal/infra/runtimeassets"

const (
	localDevRuntimeImage = "tobari-runtime:dev"
)

type localDevImageResolver struct{}

func (localDevImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	return buildidentity.Identity{
		Version: version, Commit: buildidentity.NormalizeCommit(commit),
		ResolverChannel: buildidentity.ResolverDevelopment, DevelopmentSource: true,
		Gateway: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredGatewayAPI,
			SelectedAPI: buildidentity.RequiredGatewayAPI,
		},
		AuthBroker: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredAuthBrokerAPI,
			SelectedAPI: buildidentity.RequiredAuthBrokerAPI,
		},
	}, nil
}

func newImageResolver() imageResolver {
	return localDevImageResolver{}
}

func (localDevImageResolver) DefaultRuntimeImage() string {
	return localDevRuntimeImage
}

func (localDevImageResolver) ShouldPullRuntimeImage(string) bool {
	return false
}

func (localDevImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	version, err := runtimeassets.ComponentVersion("gateway")
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: "tobari-gateway:dev-" + version, RequireDigest: false}, nil
}

func (localDevImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	version, err := runtimeassets.ComponentVersion("authbroker")
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: "tobari-auth-broker:dev-" + version, RequireDigest: false}, nil
}
