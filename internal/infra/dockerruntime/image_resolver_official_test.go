//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"testing"
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
	if got := resolver.DefaultRuntimeImage(); got != localBaseRuntimeImage || localBaseRuntimeImage != "tobari-runtime:base" || resolver.ShouldPullRuntimeImage(got) || !resolver.ShouldBuildRuntimeImage(got) {
		t.Fatalf("runtime selection = %q pull=%t build=%t", got, resolver.ShouldPullRuntimeImage(got), resolver.ShouldBuildRuntimeImage(got))
	}
}
