//go:build tobari_dev

package dockerruntime

import (
	"context"
	"fmt"
)

import "github.com/tasuku43/tobari/internal/domain/buildidentity"

import "github.com/tasuku43/tobari/internal/domain/capabilityprofile"

import "github.com/tasuku43/tobari/internal/infra/runtimeassets"

const (
	localDevRuntimeImage = "tobari-runtime:dev"
)

type localDevImageResolver struct{}

func (localDevImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	profile := capabilityprofile.Compiled()
	identity := buildidentity.Identity{
		Version: version, Commit: buildidentity.NormalizeCommit(commit),
		ResolverChannel: buildidentity.ResolverDevelopment, DevelopmentSource: true,
		CapabilityProfile: profile,
		Gateway: buildidentity.Component{
			RequiredAPI: buildidentity.RequiredGatewayAPI,
			SelectedAPI: buildidentity.RequiredGatewayAPI,
		},
	}
	if profile.IncludesExperimental() {
		identity.AuthBroker = buildidentity.Component{
			RequiredAPI: buildidentity.RequiredAuthBrokerAPI,
			SelectedAPI: buildidentity.RequiredAuthBrokerAPI,
		}
	}
	return identity, nil
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

func (localDevImageResolver) ShouldBuildRuntimeImage(string) bool {
	return false
}

func (localDevImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	version, err := runtimeassets.ComponentVersion("gateway")
	if err != nil {
		return sharedImageSelection{}, err
	}
	prefix := "tobari-gateway:dev-"
	if capabilityprofile.Compiled().IncludesExperimental() {
		prefix = "tobari-gateway-experimental:dev-"
	}
	return sharedImageSelection{Image: prefix + version, RequireDigest: false}, nil
}

func (localDevImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	if !capabilityprofile.Compiled().IncludesExperimental() {
		return sharedImageSelection{}, fmt.Errorf("Auth Broker is unavailable in the standard development profile")
	}
	version, err := runtimeassets.ComponentVersion("authbroker")
	if err != nil {
		return sharedImageSelection{}, err
	}
	return sharedImageSelection{Image: "tobari-auth-broker:dev-" + version, RequireDigest: false}, nil
}
