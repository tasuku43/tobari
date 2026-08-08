package dockerruntime

import "context"

type sharedImageSelection struct {
	Image         string
	RequireDigest bool
}

type imageResolver interface {
	DefaultRuntimeImage() string
	ShouldPullRuntimeImage(string) bool
	GatewayImage(context.Context, *Runtime) (sharedImageSelection, error)
	AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error)
}

func (r *Runtime) imageResolver() imageResolver {
	if r.images != nil {
		return r.images
	}
	return newImageResolver()
}

func (r *Runtime) defaultRuntimeImage() string {
	return r.imageResolver().DefaultRuntimeImage()
}
