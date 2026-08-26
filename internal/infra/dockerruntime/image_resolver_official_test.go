//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestOfficialImageResolverRejectsAuthBrokerInStandardProfile(t *testing.T) {
	t.Parallel()
	if _, err := (officialImageResolver{}).AuthBrokerImage(context.Background(), nil); err == nil {
		t.Fatal("standard resolver exposed an Auth Broker image")
	}
}

func TestOfficialImageResolverReturnsLocalBaseAuthority(t *testing.T) {
	t.Parallel()
	resolver := officialImageResolver{}
	got, err := resolver.DefaultRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	want, err := runtimeassets.StandardRuntimeImage()
	if err != nil {
		t.Fatal(err)
	}
	if got != want || resolver.ShouldPullRuntimeImage(got) || !resolver.ShouldBuildRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t build=%t", got, resolver.ShouldPullRuntimeImage(got), resolver.ShouldBuildRuntimeImage(got))
	}
}

func TestOfficialImageResolverReturnsEmbeddedLocalGatewayAuthority(t *testing.T) {
	t.Parallel()
	selection, err := (officialImageResolver{}).GatewayImage(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(selection.Image, "tobari-gateway:base-") || selection.RequireDigest || !selection.BuildIfMissing {
		t.Fatalf("Gateway selection = %+v", selection)
	}
}
