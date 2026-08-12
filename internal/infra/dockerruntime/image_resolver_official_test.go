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
