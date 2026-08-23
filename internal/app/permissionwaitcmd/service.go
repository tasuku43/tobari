// Package permissionwaitcmd owns read-only observation of one attachment-local
// reviewed permission disposition.
package permissionwaitcmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// Observer is the complete effectful boundary for permission wait. It can
// observe and consume only attachment-local wait side state; it has no policy,
// Workspace, cluster, Docker, process, unrestricted filesystem, general or
// external network, or retry mutation. Infrastructure owns the private
// attachment-local Unix transport without exposing it through this port.
type Observer interface {
	WaitPermission(context.Context, string) (tobari.PermissionWaitResult, error)
}

type Service struct {
	observer Observer
}

func New(observer Observer) *Service { return &Service{observer: observer} }

func (s *Service) Wait(ctx context.Context, id string) (tobari.PermissionWaitResult, error) {
	if s == nil || portcheck.IsNil(s.observer) {
		return "", fault.New(fault.KindInternal, "missing_permission_wait_observer", "permission wait observer is not configured", false)
	}
	if err := tobari.ValidatePermissionWaitID(id); err != nil {
		return "", fault.Wrap(fault.KindInvalidInput, "invalid_permission_wait", "permission wait ID is invalid", false, err)
	}
	result, err := s.observer.WaitPermission(ctx, id)
	if err != nil {
		return "", err
	}
	if err := result.Validate(); err != nil {
		return "", fault.Wrap(fault.KindContract, "invalid_permission_wait_result", "permission wait result is invalid", false, err)
	}
	return result, nil
}
