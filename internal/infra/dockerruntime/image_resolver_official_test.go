//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"testing"
)

func TestOfficialImageResolverReturnsLinkInjectedAuthBrokerAuthority(t *testing.T) {
	t.Parallel()
	selection, err := (officialImageResolver{}).AuthBrokerImage(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Image != publishedAuthBrokerImage || !selection.RequireDigest || selection.Image != "unpublished" {
		t.Fatalf("Auth Broker selection = %+v", selection)
	}
}

func TestOfficialImageResolverReturnsLocalBaseAuthority(t *testing.T) {
	t.Parallel()
	resolver := officialImageResolver{}
	if got := resolver.DefaultRuntimeImage(); got != localBaseRuntimeImage || got != "tobari-runtime:base" || resolver.ShouldPullRuntimeImage(got) || !resolver.ShouldBuildRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t build=%t", got, resolver.ShouldPullRuntimeImage(got), resolver.ShouldBuildRuntimeImage(got))
	}
}
