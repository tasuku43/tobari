package dockerruntime

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

type sharedImageSelection struct {
	Image          string
	RequireDigest  bool
	BuildIfMissing bool
}

type imageResolver interface {
	BuildIdentity(string, string) (buildidentity.Identity, error)
	DefaultRuntimeImage() (string, error)
	ShouldPullRuntimeImage(string) bool
	ShouldBuildRuntimeImage(string) bool
	GatewayImage(context.Context, *Runtime) (sharedImageSelection, error)
	AuthBrokerImage(context.Context, *Runtime) (sharedImageSelection, error)
}

// BuildIdentity returns the public-safe identity compiled into this executable.
func BuildIdentity(version, commit string) (buildidentity.Identity, error) {
	return newImageResolver().BuildIdentity(version, buildidentity.NormalizeCommit(commit))
}

func (r *Runtime) validateResolverCompatibility() error {
	identity, err := r.imageResolver().BuildIdentity("dev", buildidentity.UnknownCommit)
	if err != nil {
		return fault.Wrap(
			fault.KindContract, "runtime_image_api_mismatch",
			"The compiled runtime-image resolver identity is invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the executable and runtime image compatibility contract."},
		)
	}
	if err := identity.Validate(); err != nil {
		return fault.Wrap(
			fault.KindContract, "runtime_image_api_mismatch",
			"The compiled runtime-image resolver identity is invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the executable and runtime image compatibility contract."},
		)
	}
	if identity.APIsCompatible() {
		return nil
	}
	if !identity.CapabilitySurface.IncludesResearch() {
		return fault.New(
			fault.KindContract, "runtime_image_api_mismatch",
			fmt.Sprintf(
				"The resolver selects Gateway API %d, but this source requires Gateway API %d.",
				identity.Gateway.SelectedAPI, identity.Gateway.RequiredAPI,
			), false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed executable and immutable Gateway image pin."},
		)
	}
	return fault.New(
		fault.KindContract, "runtime_image_api_mismatch",
		fmt.Sprintf(
			"The resolver selects Gateway API %d and Auth Broker API %d, but this source requires Gateway API %d and Auth Broker API %d.",
			identity.Gateway.SelectedAPI, identity.AuthBroker.SelectedAPI,
			identity.Gateway.RequiredAPI, identity.AuthBroker.RequiredAPI,
		), false,
		fault.NextAction{Command: "doctor", Reason: "Inspect the installed executable and immutable runtime image pins."},
	)
}

func (r *Runtime) incompatibleComponentAPI(component string, selected, required int, code string) error {
	identity, err := r.imageResolver().BuildIdentity("dev", buildidentity.UnknownCommit)
	if err == nil {
		if buildCommand, binary, ok := identity.DevelopmentRecovery(); ok {
			return fault.New(
				fault.KindContract, code,
				fmt.Sprintf(
					"The development %s image declares API %d, but this source requires API %d. Run `%s`, then retry with `%s cluster up`.",
					component, selected, required, buildCommand, binary,
				), false,
				fault.NextAction{Command: "doctor", Reason: "Inspect the local development image contract."},
			)
		}
	}
	return fault.New(
		fault.KindContract, code,
		fmt.Sprintf(
			"The embedded %s image source declares API %d, but this source requires API %d. Install a compatible Tobari release.",
			component, selected, required,
		), false,
		fault.NextAction{Command: "doctor", Reason: "Inspect the installed executable and immutable runtime image contract."},
	)
}

func (r *Runtime) imageResolver() imageResolver {
	if r.images != nil {
		return r.images
	}
	return newImageResolver()
}

func (r *Runtime) defaultRuntimeImage() (string, error) {
	return r.imageResolver().DefaultRuntimeImage()
}
