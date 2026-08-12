//go:build tobari_dev

package dockerruntime

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestLocalDevImageResolverSelectsAllLocalImagesWithoutPulling(t *testing.T) {
	t.Parallel()
	resolver := localDevImageResolver{}
	if got := resolver.DefaultRuntimeImage(); got != localDevRuntimeImage || resolver.ShouldPullRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t", got, resolver.ShouldPullRuntimeImage(got))
	}
	gatewayVersion, err := runtimeassets.ComponentVersion("gateway")
	if err != nil {
		t.Fatal(err)
	}
	authVersion, err := runtimeassets.ComponentVersion("authbroker")
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"gateway":     "tobari-gateway:dev-" + gatewayVersion,
		"auth broker": "tobari-auth-broker:dev-" + authVersion,
	} {
		var selection sharedImageSelection
		var err error
		if name == "gateway" {
			selection, err = resolver.GatewayImage(context.Background(), nil)
		} else {
			selection, err = resolver.AuthBrokerImage(context.Background(), nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		if selection.Image != want || selection.RequireDigest {
			t.Errorf("%s selection = %+v, want local %q", name, selection, want)
		}
	}
}

func TestLocalDevImageResolverIdentityCannotCrossToPublishedPins(t *testing.T) {
	t.Parallel()
	identity, err := (localDevImageResolver{}).BuildIdentity("dev", "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ResolverChannel != buildidentity.ResolverDevelopment || !identity.DevelopmentSource ||
		identity.Gateway.RequiredAPI != buildidentity.RequiredGatewayAPI || identity.Gateway.SelectedAPI != buildidentity.RequiredGatewayAPI ||
		identity.AuthBroker.RequiredAPI != buildidentity.RequiredAuthBrokerAPI || identity.AuthBroker.SelectedAPI != buildidentity.RequiredAuthBrokerAPI ||
		!identity.APIsCompatible() {
		t.Fatalf("development identity = %+v", identity)
	}
	build, binary, ok := identity.DevelopmentRecovery()
	if !ok || build != "task build" || binary != "bin/tobari" {
		t.Fatalf("development recovery = %q %q %t", build, binary, ok)
	}
}
