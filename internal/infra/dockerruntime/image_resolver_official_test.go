//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestOfficialImageResolverReturnsPinnedAuthBrokerSelection(t *testing.T) {
	t.Parallel()
	selection, err := (officialImageResolver{}).AuthBrokerImage(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		t.Fatal(err)
	}
	if selection.Image != versions["AUTH_BROKER_IMAGE"] || !selection.RequireDigest || selection.Image == "unpublished" {
		t.Fatalf("Auth Broker selection = %+v", selection)
	}
}

func TestOfficialImageResolverIdentityKeepsHistoricalPinsSeparateFromSource(t *testing.T) {
	t.Parallel()
	identity, err := (officialImageResolver{}).BuildIdentity("1.2.3", "0123456789abcdef0123456789abcdef01234567")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ResolverChannel != buildidentity.ResolverPublished || identity.DevelopmentSource ||
		identity.Gateway.RequiredAPI != 4 || identity.Gateway.SelectedAPI != 3 ||
		identity.AuthBroker.RequiredAPI != 3 || identity.AuthBroker.SelectedAPI != 2 ||
		identity.APIsCompatible() {
		t.Fatalf("official identity = %+v", identity)
	}
	if build, binary, ok := identity.DevelopmentRecovery(); ok || build != "" || binary != "" {
		t.Fatalf("official identity advertised contributor recovery = %q %q", build, binary)
	}
}
