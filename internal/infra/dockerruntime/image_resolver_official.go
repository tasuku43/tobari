//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type officialImageResolver struct{}

func (officialImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return buildidentity.Identity{}, err
	}
	gatewayAPI, err := selectedImageAPI(versions, "GATEWAY_IMAGE_API")
	if err != nil {
		return buildidentity.Identity{}, err
	}
	authBrokerAPI, err := selectedImageAPI(versions, "AUTH_BROKER_IMAGE_API")
	if err != nil {
		return buildidentity.Identity{}, err
	}
	return buildidentity.Identity{
		Version: version, Commit: buildidentity.NormalizeCommit(commit),
		ResolverChannel: buildidentity.ResolverPublished,
		Gateway: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredGatewayAPI, SelectedAPI: gatewayAPI,
		},
		AuthBroker: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredAuthBrokerAPI, SelectedAPI: authBrokerAPI,
		},
	}, nil
}

func selectedImageAPI(versions map[string]string, key string) (int, error) {
	value, err := strconv.Atoi(versions[key])
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("embedded versions.env %s must be a positive integer", key)
	}
	return value, nil
}

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
