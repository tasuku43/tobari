// Package doctorcmd implements the read-only doctor use case.
package doctorcmd

import (
	"context"
	"fmt"

	"github.com/tasuku43/agentic-cli-foundry/internal/app/portcheck"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/doctor"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

// InspectorPort is the smallest infrastructure capability needed by doctor.
// Infrastructure adapters satisfy it structurally and do not import app.
type InspectorPort interface {
	Inspect(context.Context) (doctor.Report, error)
}

// Service coordinates the doctor inspection.
type Service struct {
	inspector InspectorPort
}

// New creates a doctor service.
func New(inspector InspectorPort) *Service {
	return &Service{inspector: inspector}
}

// Run validates the declared read intent before crossing the infrastructure
// boundary.
func (s *Service) Run(ctx context.Context, intent operation.Intent) (doctor.Report, error) {
	if ctx == nil {
		return doctor.Report{}, fmt.Errorf("doctor context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Report{}, err
	}
	if err := intent.Validate(); err != nil {
		return doctor.Report{}, fmt.Errorf("doctor intent: %w", err)
	}
	if intent.Command != "doctor" || intent.Effect != operation.EffectRead {
		return doctor.Report{}, fmt.Errorf("doctor requires the doctor read intent")
	}
	if s == nil || portcheck.IsNil(s.inspector) {
		return doctor.Report{}, fmt.Errorf("doctor inspector is not configured")
	}

	report, err := s.inspector.Inspect(ctx)
	if contextErr := ctx.Err(); contextErr != nil {
		return doctor.Report{}, contextErr
	}
	if err != nil {
		return doctor.Report{}, fmt.Errorf("inspect system: %w", err)
	}
	if err := report.Validate(); err != nil {
		return doctor.Report{}, fmt.Errorf("invalid doctor report: %w", err)
	}
	return report, nil
}
