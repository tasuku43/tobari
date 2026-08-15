package dockerruntime

import "context"

import "github.com/tasuku43/tobari/internal/domain/buildidentity"

import "github.com/tasuku43/tobari/internal/domain/capabilityprofile"

type testImageResolver struct {
	runtimeImage string
	pullRuntime  bool
	buildRuntime bool
	gateway      sharedImageSelection
	authBroker   sharedImageSelection
	identity     *buildidentity.Identity
}

func (r testImageResolver) BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	if r.identity != nil {
		return *r.identity, nil
	}
	return buildidentity.Identity{
		Version: version, Commit: buildidentity.NormalizeCommit(commit),
		ResolverChannel: buildidentity.ResolverDevelopment, DevelopmentSource: true,
		CapabilityProfile: capabilityprofile.ProfileStandard,
		Gateway:           buildidentity.Component{RequiredAPI: buildidentity.RequiredGatewayAPI, SelectedAPI: buildidentity.RequiredGatewayAPI},
		AuthBroker:        buildidentity.Component{RequiredAPI: buildidentity.RequiredAuthBrokerAPI, SelectedAPI: buildidentity.RequiredAuthBrokerAPI},
	}, nil
}

func (r testImageResolver) DefaultRuntimeImage() string {
	return r.runtimeImage
}

func (r testImageResolver) ShouldPullRuntimeImage(string) bool {
	return r.pullRuntime
}

func (r testImageResolver) ShouldBuildRuntimeImage(string) bool {
	return r.buildRuntime
}

func (r testImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return r.gateway, nil
}

func (r testImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return r.authBroker, nil
}
