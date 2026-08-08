//go:build !tobari_dev

package dockerruntime

import (
	"context"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type officialImageResolver struct{}

func newImageResolver() imageResolver {
	return officialImageResolver{}
}

func (officialImageResolver) DefaultRuntimeImage() string {
	return tobari.OfficialRuntimeBase
}

func (officialImageResolver) ShouldPullRuntimeImage(image string) bool {
	return image == tobari.OfficialRuntimeBase
}

func (officialImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: versions["GATEWAY_IMAGE"], RequireDigest: true}, nil
}

func (officialImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: versions["AUTH_BROKER_IMAGE"], RequireDigest: true}, nil
}
