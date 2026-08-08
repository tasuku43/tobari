package dockerruntime

import "context"

type testImageResolver struct {
	runtimeImage string
	pullRuntime  bool
	gateway      sharedImageSelection
	authBroker   sharedImageSelection
}

func (r testImageResolver) DefaultRuntimeImage() string {
	return r.runtimeImage
}

func (r testImageResolver) ShouldPullRuntimeImage(string) bool {
	return r.pullRuntime
}

func (r testImageResolver) GatewayImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return r.gateway, nil
}

func (r testImageResolver) AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error) {
	return r.authBroker, nil
}
