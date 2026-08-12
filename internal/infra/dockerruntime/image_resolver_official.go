//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type officialImageResolver struct{}

// Release packaging replaces these four values from one validated generated
// component lock. Repository source deliberately contains no published digest
// authority.
var (
	publishedGatewayImage    = "unpublished"
	publishedGatewayAPI      = "1"
	publishedAuthBrokerImage = "unpublished"
	publishedAuthBrokerAPI   = "1"
)

func (officialImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	gatewayAPI, err := selectedImageAPI(publishedGatewayAPI, "Gateway")
	if err != nil {
		return buildidentity.Identity{}, err
	}
	authBrokerAPI, err := selectedImageAPI(publishedAuthBrokerAPI, "Auth Broker")
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

func selectedImageAPI(raw, component string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("injected %s API must be a positive integer", component)
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
	return sharedImageSelection{Image: publishedGatewayImage, RequireDigest: true}, nil
}

func (officialImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return sharedImageSelection{Image: publishedAuthBrokerImage, RequireDigest: true}, nil
}
