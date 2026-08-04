//go:build tobari_dev

package dockerruntime

import "context"

const (
	localDevGatewayImage = "tobari-gateway:dev"
	localDevRuntimeImage = "tobari-runtime:dev"
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

func (localDevImageResolver) GatewayImage(context.Context, *Runtime) (gatewayImageSelection, error) {
	return gatewayImageSelection{Image: localDevGatewayImage, RequireDigest: false}, nil
}
