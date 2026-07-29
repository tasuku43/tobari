// Package systemdoctor provides the local, offline doctor adapter.
package systemdoctor

import (
	"context"
	"fmt"
	"runtime"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/doctor"
)

// Inspector reports properties of the running binary without network access.
type Inspector struct{}

// New creates the default system inspector.
func New() *Inspector {
	return &Inspector{}
}

// Inspect implements doctorcmd.InspectorPort by structural typing.
func (i *Inspector) Inspect(ctx context.Context) (doctor.Report, error) {
	if ctx == nil {
		return doctor.Report{}, fmt.Errorf("inspection context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Report{}, err
	}
	return doctor.Report{Checks: []doctor.Check{
		{
			Name:   "runtime",
			Status: doctor.CheckStatusPass,
			Detail: fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
		},
	}}, nil
}
