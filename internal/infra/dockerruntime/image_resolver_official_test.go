//go:build !tobari_dev

package dockerruntime

import (
	"context"
	"testing"

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
