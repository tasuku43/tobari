//go:build tobari_dev

package dockerruntime

import "context"

import "github.com/tasuku43/tobari/internal/domain/buildidentity"

const (
	localDevAuthBrokerImage = "tobari-auth-broker:dev"
	localDevGatewayImage    = "tobari-gateway:dev"
	localDevRuntimeImage    = "tobari-runtime:dev"
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
	return sharedImageSelection{Image: localDevGatewayImage, RequireDigest: false}, nil
}

func (localDevImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return sharedImageSelection{Image: localDevAuthBrokerImage, RequireDigest: false}, nil
}
