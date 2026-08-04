package dockerruntime

import "context"

type gatewayImageSelection struct {
	Image         string
	RequireDigest bool
}

type imageResolver interface {
	DefaultRuntimeImage() string
	ShouldPullRuntimeImage(string) bool
	GatewayImage(context.Context, *Runtime) (gatewayImageSelection, error)
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
