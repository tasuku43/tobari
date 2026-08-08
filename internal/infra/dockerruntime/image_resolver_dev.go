//go:build tobari_dev

package dockerruntime

import "context"

const (
	localDevAuthBrokerImage = "tobari-auth-broker:dev"
	localDevGatewayImage    = "tobari-gateway:dev"
	localDevRuntimeImage    = "tobari-runtime:dev"
)

type localDevImageResolver struct{}

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
