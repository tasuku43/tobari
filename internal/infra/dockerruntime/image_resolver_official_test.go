//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func TestOfficialImageResolverReturnsFailClosedAuthBrokerBootstrapSelection(t *testing.T) {
	t.Parallel()
	selection, err := (officialImageResolver{}).AuthBrokerImage(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Image != runtimeassets.UnpublishedAuthBrokerImage || !selection.RequireDigest {
		t.Fatalf("Auth Broker selection = %+v", selection)
	}
}
