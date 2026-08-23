package dockerruntime

import (
	"bytes"
	"strings"
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

	contextID, err := issuer.newWorkspaceManifestID()
	if err != nil {
		t.Fatal(err)
	}
	if contextID != "018bcfe5-687b-7000-8000-000000000000" {
		t.Fatalf("Context ID = %q", contextID)
	}

	request := tobari.ProjectInstanceRequest{
		Root:                     "/workspace/project",
		WorkspaceManifestID:      contextID,
		WorkspaceManifestName:    tobari.DefaultManifestName,
		Image:                    tobari.BuiltinImageSelector,
		CreationDefaultsRevision: "sha256:" + strings.Repeat("a", 64),
		CreatedAt:                time.UnixMilli(1_700_000_000_123).UTC(),
	}
	project, err := issuer.newProjectInstance(request)
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != contextID || project.Root != request.Root ||
		project.WorkspaceManifestID != request.WorkspaceManifestID || project.WorkspaceManifestName != request.WorkspaceManifestName ||
		project.Image != request.Image || project.Runtime != (tobari.WorkspaceRuntime{}) {
		t.Fatalf("project = %+v", project)
	}
}
