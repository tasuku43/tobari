package workspaceauthoritysession

import (
	"testing"

	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
)

func TestBridgeRequiresOneCanonicalDockerRuntime(t *testing.T) {
	if bridge, err := New(nil); err == nil || bridge != nil {
		t.Fatal("nil Docker runtime created a final session bridge")
	}
	bridge, err := New(&dockerruntime.Runtime{})
	if err != nil || bridge == nil {
		t.Fatalf("host-only final session bridge = %#v, %v", bridge, err)
	}
}
