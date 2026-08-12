package dockerruntime

import (
	"bytes"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestIdentityIssuerPreservesUUIDv7AndProjectRequest(t *testing.T) {
	t.Parallel()
	issuer := identityIssuer{
		now:     func() time.Time { return time.UnixMilli(1_700_000_000_123) },
		entropy: bytes.NewReader(make([]byte, 20)),
	}

	contextID, err := issuer.newContextID()
	if err != nil {
		t.Fatal(err)
	}
	if contextID != "018bcfe5-687b-7000-8000-000000000000" {
		t.Fatalf("Context ID = %q", contextID)
	}

	request := tobari.ProjectInstanceRequest{
		Root:        "/workspace/project",
		ContextID:   contextID,
		ContextName: tobari.DefaultContextName,
		Image:       tobari.BuiltinImageSelector,
	}
	project, err := issuer.newProjectInstance(request)
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != contextID || project.Root != request.Root ||
		project.ContextID != request.ContextID || project.ContextName != request.ContextName ||
		project.Image != request.Image || project.Runtime != (tobari.ProjectRuntime{}) {
		t.Fatalf("project = %+v", project)
	}
}
