package dockerruntime

import "context"

type testImageResolver struct {
	runtimeImage string
	pullRuntime  bool
	gateway      gatewayImageSelection
}

func (r testImageResolver) DefaultRuntimeImage() string {
	return r.runtimeImage
}

func (r testImageResolver) ShouldPullRuntimeImage(string) bool {
	return r.pullRuntime
}

func (r testImageResolver) GatewayImage(context.Context, *Runtime) (gatewayImageSelection, error) {
	return r.gateway, nil
}
