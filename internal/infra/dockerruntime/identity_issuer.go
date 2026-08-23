package dockerruntime

import (
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// identityIssuer owns the production clock and entropy boundary for stable
// Context and project identities. Domain constructors remain deterministic.
type identityIssuer struct {
	now     func() time.Time
	entropy io.Reader
}

func (i identityIssuer) newRuntimeBuildAttemptID() (string, error) {
	if i.entropy == nil {
		return "", fmt.Errorf("identity entropy source is required")
	}
	var raw [32]byte
	if _, err := io.ReadFull(i.entropy, raw[:]); err != nil {
		return "", fmt.Errorf("read Runtime build attempt entropy: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func (i identityIssuer) newWorkspaceManifestID() (string, error) {
	if i.now == nil {
		return "", fmt.Errorf("identity clock is required")
	}
	if i.entropy == nil {
		return "", fmt.Errorf("identity entropy source is required")
	}
	return tobari.NewWorkspaceManifestID(i.now().UTC(), i.entropy)
}

func (i identityIssuer) newRuntimeID() (string, error) {
	if i.now == nil {
		return "", fmt.Errorf("identity clock is required")
	}
	if i.entropy == nil {
		return "", fmt.Errorf("identity entropy source is required")
	}
	return tobari.NewRuntimeID(i.now().UTC(), i.entropy)
}

func (i identityIssuer) newProjectInstance(request tobari.ProjectInstanceRequest) (tobari.Workspace, error) {
	if i.now == nil {
		return tobari.Workspace{}, fmt.Errorf("identity clock is required")
	}
	if i.entropy == nil {
		return tobari.Workspace{}, fmt.Errorf("identity entropy source is required")
	}
	now := i.now().UTC()
	request.CreatedAt = now
	return tobari.NewProjectInstance(now, i.entropy, request)
}
