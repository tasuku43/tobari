//go:build tobari_dev

package dockerruntime

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
)

func TestLocalDevImageResolverSelectsAllLocalImagesWithoutPulling(t *testing.T) {
	t.Parallel()
	resolver := localDevImageResolver{}
	if got := resolver.DefaultRuntimeImage(); got != localDevRuntimeImage || resolver.ShouldPullRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t", got, resolver.ShouldPullRuntimeImage(got))
	}
	for name, want := range map[string]string{
		"gateway":     localDevGatewayImage,
		"auth broker": localDevAuthBrokerImage,
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
		identity.Gateway.RequiredAPI != 4 || identity.Gateway.SelectedAPI != 4 ||
		identity.AuthBroker.RequiredAPI != 3 || identity.AuthBroker.SelectedAPI != 3 ||
		!identity.APIsCompatible() {
		t.Fatalf("development identity = %+v", identity)
	}
	build, binary, ok := identity.DevelopmentRecovery()
	if !ok || build != "task build:dev" || binary != "bin/tobari-dev" {
		t.Fatalf("development recovery = %q %q %t", build, binary, ok)
	}
}
