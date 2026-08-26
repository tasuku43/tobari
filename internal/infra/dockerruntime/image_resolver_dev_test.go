//go:build tobari_dev

package dockerruntime

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestLocalDevImageResolverSelectsAllLocalImagesWithoutPulling(t *testing.T) {
	t.Parallel()
	resolver := localDevImageResolver{}
	got, err := resolver.DefaultRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	if expected, expectedErr := runtimeassets.StandardRuntimeImage(); expectedErr != nil || got != expected || resolver.ShouldPullRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t", got, resolver.ShouldPullRuntimeImage(got))
	}
	if resolver.ShouldBuildRuntimeImage(got) {
		t.Fatalf("development resolver unexpectedly owns standard Runtime build: %q", got)
	}
	gatewayVersion, err := runtimeassets.ComponentVersion("gateway")
	if err != nil {
		t.Fatal(err)
	}
	authVersion, err := runtimeassets.ComponentVersion("authbroker")
	if err != nil {
		t.Fatal(err)
	}
	gatewayPrefix := "tobari-gateway:dev-"
	if brokerRuntimeEnabled {
		gatewayPrefix = "tobari-gateway-experimental:dev-"
	}
	selections := map[string]string{"gateway": gatewayPrefix + gatewayVersion}
	if capabilitysurface.Compiled().IncludesResearch() {
		selections["auth broker"] = "tobari-auth-broker:dev-" + authVersion
	}
	for name, want := range selections {
		var selection sharedImageSelection
		var err error
		if name == "gateway" {
			selection, err = resolver.GatewayImage(context.Background(), nil)
		} else {
			selection, err = resolver.AuthBrokerImage(context.Background(), nil)
			if !capabilitysurface.Compiled().IncludesResearch() {
				if err == nil {
					t.Fatalf("standard development resolver unexpectedly selected Auth Broker image: %+v", selection)
				}
				continue
			}
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
		!identity.APIsCompatible() {
		t.Fatalf("development identity = %+v", identity)
	}
	if capabilitysurface.Compiled().IncludesResearch() {
		if identity.AuthBroker.RequiredAPI != buildidentity.RequiredAuthBrokerAPI || identity.AuthBroker.SelectedAPI != buildidentity.RequiredAuthBrokerAPI {
			t.Fatalf("research development identity = %+v", identity)
		}
	} else if identity.AuthBroker != (buildidentity.Component{}) {
		t.Fatalf("release development identity unexpectedly includes Auth Broker = %+v", identity.AuthBroker)
	}
	build, binary, ok := identity.DevelopmentRecovery()
	wantBuild, wantBinary := buildidentity.ReleaseDevelopmentBuildCommand, buildidentity.ReleaseDevelopmentBinary
	if identity.CapabilitySurface.IncludesResearch() {
		wantBuild, wantBinary = buildidentity.ResearchDevelopmentBuildCommand, buildidentity.ResearchDevelopmentBinary
	}
	if !ok || build != wantBuild || binary != wantBinary {
		t.Fatalf("development recovery = %q %q %t", build, binary, ok)
	}
}
