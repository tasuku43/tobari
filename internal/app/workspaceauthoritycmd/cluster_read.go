package workspaceauthoritycmd

import (
	"context"
	"errors"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type FinalClusterReadPort interface {
	ReadLogs(context.Context, tobari.LogRequest) ([]byte, error)
	ReadDenials(context.Context, int) (tobari.FinalClusterDenialWindow, error)
}

type FinalClusterReadService struct{ port FinalClusterReadPort }

func NewFinalClusterReadService(port FinalClusterReadPort) *FinalClusterReadService {
	return &FinalClusterReadService{port: port}
}

func (s *FinalClusterReadService) Logs(ctx context.Context, request tobari.LogRequest) ([]byte, error) {
	if s == nil || portcheck.IsNil(s.port) {
		return nil, missingPort("final cluster logs")
	}
	if err := request.ValidateCluster(); err != nil {
		return nil, fault.Wrap(fault.KindInvalidInput, "invalid_log_request", "cluster log request is invalid", false, err)
	}
	result, err := s.port.ReadLogs(ctx, request)
	if err != nil {
		return nil, finalClusterReadFault("logs_failed", "cluster logs could not be read", err)
	}
	return append([]byte{}, result...), nil
}

func (s *FinalClusterReadService) Denials(ctx context.Context, tail int) (tobari.FinalClusterDenialWindow, error) {
	if s == nil || portcheck.IsNil(s.port) {
		return tobari.FinalClusterDenialWindow{}, missingPort("final cluster denials")
	}
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return tobari.FinalClusterDenialWindow{}, fault.Wrap(fault.KindInvalidInput, "invalid_denial_request", "denial request is invalid", false, err)
	}
	result, err := s.port.ReadDenials(ctx, tail)
	if err != nil {
		return tobari.FinalClusterDenialWindow{}, finalClusterReadFault("denials_failed", "cluster denials could not be read", err)
	}
	if err := result.Validate(); err != nil {
		return tobari.FinalClusterDenialWindow{}, contractFault("invalid_denial_contract", "final cluster denial result is invalid", err)
	}
	return result, nil
}

func finalClusterReadFault(code, message string, err error) error {
	if errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		return fault.Wrap(fault.KindRejected, "legacy_state_present", "pre-release legacy authority blocks final cluster reads", false, err)
	}
	if errors.Is(err, tobari.ErrFinalClusterNotRunning) || errors.Is(err, tobari.ErrFinalClusterObservationChanged) {
		return fault.Wrap(fault.KindUnavailable, "cluster_not_running", "final cluster authority is not exactly active", false, err)
	}
	return fault.Wrap(fault.KindInternal, code, message, false, err)
}
