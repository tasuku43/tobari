package dockerruntime

import (
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

func (i identityIssuer) newContextID() (string, error) {
	if i.now == nil {
		return "", fmt.Errorf("identity clock is required")
	}
	if i.entropy == nil {
		return "", fmt.Errorf("identity entropy source is required")
	}
	return tobari.NewContextID(i.now().UTC(), i.entropy)
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

func (i identityIssuer) newProjectInstance(request tobari.ProjectInstanceRequest) (tobari.ProjectInstance, error) {
	if i.now == nil {
		return tobari.ProjectInstance{}, fmt.Errorf("identity clock is required")
	}
	if i.entropy == nil {
		return tobari.ProjectInstance{}, fmt.Errorf("identity entropy source is required")
	}
	return tobari.NewProjectInstance(i.now().UTC(), i.entropy, request)
}
