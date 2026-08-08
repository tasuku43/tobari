//go:build tobari_dev

package dockerruntime

import (
	"context"
	"testing"
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
