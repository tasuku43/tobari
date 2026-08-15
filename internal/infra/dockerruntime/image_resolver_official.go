//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
)

type officialImageResolver struct{}

// Release packaging replaces the two service identities and APIs from one
// validated generated component lock. The base runtime remains a local image;
// packaging gives its embedded recipe a source-derived tag.
var (
	publishedGatewayImage    = "unpublished"
	publishedGatewayAPI      = "1"
	publishedAuthBrokerImage = "unpublished"
	publishedAuthBrokerAPI   = "1"
	localBaseRuntimeImage    = "tobari-runtime:base"
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
		ResolverChannel:   buildidentity.ResolverPublished,
		CapabilityProfile: capabilityprofile.Compiled(),
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
	return localBaseRuntimeImage
}

func (officialImageResolver) ShouldPullRuntimeImage(string) bool {
	return false
}

func (officialImageResolver) ShouldBuildRuntimeImage(image string) bool {
	return image == localBaseRuntimeImage
}

func (officialImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return sharedImageSelection{Image: publishedGatewayImage, RequireDigest: true}, nil
}

func (officialImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return sharedImageSelection{Image: publishedAuthBrokerImage, RequireDigest: true}, nil
}
