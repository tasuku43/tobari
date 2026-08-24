// Package serviceexposurecmd owns attachment-scoped Workspace service review.
package serviceexposurecmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type Port interface {
	RequestService(context.Context, int) (tobari.ServiceExposure, error)
	AttachmentServiceStatus(context.Context) (tobari.ServiceAttachmentStatus, error)
	StopServiceExposure(context.Context, string) error
	ReviewServiceRequests(context.Context) (tobari.ServiceReviewSnapshot, error)
	ServiceStatus(context.Context) (tobari.ServiceStatusSnapshot, error)
	AllowServiceRequest(context.Context, string) (tobari.ServiceExposure, error)
	DenyServiceRequest(context.Context, string) error
	OpenServiceExposure(context.Context, string) (tobari.ServiceOpenResult, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ServiceAttachmentServicesKind && intent.Target.ParentID == tobari.ServiceAttachmentServicesTargetID && intent.Target.ID == "":
		return nil
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ServiceExposureKind && intent.Target.ParentID != "" && intent.Target.ID == "":
		return tobari.ValidateServiceRequestID(intent.Target.ParentID)
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ServiceRequestKind && intent.Target.ID != "":
		return tobari.ValidateServiceRequestID(intent.Target.ID)
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ServiceExposureKind && intent.Target.ID != "":
		return tobari.ValidateServiceExposureID(intent.Target.ID)
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "service exposure mutation target is not owned by Tobari", false)
	}
}

type Service struct {
	port    Port
	mutator *execution.Invoker
}

func New(port Port) *Service { return &Service{port: port, mutator: execution.New(ownedPolicy{})} }

func (s *Service) requirePort() error {
	if s == nil || portcheck.IsNil(s.port) {
		return fault.New(fault.KindInternal, "missing_runtime", "service exposure runtime is not configured", false)
	}
	return nil
}

func (s *Service) Request(ctx context.Context, intent operation.Intent, port int) (tobari.ServiceExposure, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceExposure{}, err
	}
	if err := tobari.ValidateServicePort(port); err != nil {
		return tobari.ServiceExposure{}, fault.Wrap(fault.KindInvalidInput, "invalid_service_port", "Workspace service port is invalid", false, err)
	}
	target := operation.TargetRef{Kind: tobari.ServiceAttachmentServicesKind, ParentID: tobari.ServiceAttachmentServicesTargetID}
	intent.Target = target
	var result tobari.ServiceExposure
	err := s.mutator.Invoke(ctx, mutationRequest(intent, target), func(actionContext context.Context, _ operation.Intent) error {
		var err error
		result, err = s.port.RequestService(actionContext, port)
		return err
	})
	if err != nil {
		return tobari.ServiceExposure{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceExposure{}, fault.Wrap(fault.KindContract, "invalid_service_exposure", "service exposure result is invalid", false, err)
	}
	return result, nil
}

func (s *Service) AttachmentStatus(ctx context.Context) (tobari.ServiceAttachmentStatus, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceAttachmentStatus{}, err
	}
	result, err := s.port.AttachmentServiceStatus(ctx)
	if err != nil {
		return tobari.ServiceAttachmentStatus{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceAttachmentStatus{}, fault.Wrap(fault.KindContract, "invalid_service_attachment_status", "service attachment status is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Review(ctx context.Context) (tobari.ServiceReviewSnapshot, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceReviewSnapshot{}, err
	}
	result, err := s.port.ReviewServiceRequests(ctx)
	if err != nil {
		return tobari.ServiceReviewSnapshot{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceReviewSnapshot{}, fault.Wrap(fault.KindContract, "invalid_service_review", "service review snapshot is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context) (tobari.ServiceStatusSnapshot, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceStatusSnapshot{}, err
	}
	result, err := s.port.ServiceStatus(ctx)
	if err != nil {
		return tobari.ServiceStatusSnapshot{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceStatusSnapshot{}, fault.Wrap(fault.KindContract, "invalid_service_status", "service status snapshot is invalid", false, err)
	}
	return result, nil
}

func mutationRequest(intent operation.Intent, target operation.TargetRef) execution.Request {
	return execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: intent.Effect, ExpectedTarget: target, ExpectedImpact: intent.Impact}
}

func (s *Service) Allow(ctx context.Context, intent operation.Intent, requestID string) (tobari.ServiceExposure, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceExposure{}, err
	}
	target := operation.TargetRef{Kind: tobari.ServiceExposureKind, ParentID: requestID}
	intent.Target = target
	var result tobari.ServiceExposure
	err := s.mutator.Invoke(ctx, mutationRequest(intent, target), func(actionContext context.Context, _ operation.Intent) error {
		var err error
		result, err = s.port.AllowServiceRequest(actionContext, requestID)
		return err
	})
	if err != nil {
		return tobari.ServiceExposure{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceExposure{}, fault.Wrap(fault.KindContract, "invalid_service_exposure", "service exposure result is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Deny(ctx context.Context, intent operation.Intent, requestID string) error {
	if err := s.requirePort(); err != nil {
		return err
	}
	target := operation.TargetRef{Kind: tobari.ServiceRequestKind, ID: requestID}
	intent.Target = target
	return s.mutator.Invoke(ctx, mutationRequest(intent, target), func(actionContext context.Context, _ operation.Intent) error {
		return s.port.DenyServiceRequest(actionContext, requestID)
	})
}

func (s *Service) Stop(ctx context.Context, intent operation.Intent, exposureID string) error {
	if err := s.requirePort(); err != nil {
		return err
	}
	target := operation.TargetRef{Kind: tobari.ServiceExposureKind, ID: exposureID}
	intent.Target = target
	return s.mutator.Invoke(ctx, mutationRequest(intent, target), func(actionContext context.Context, _ operation.Intent) error {
		return s.port.StopServiceExposure(actionContext, exposureID)
	})
}

func (s *Service) Open(ctx context.Context, intent operation.Intent, exposureID string) (tobari.ServiceOpenResult, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceOpenResult{}, err
	}
	target := operation.TargetRef{Kind: tobari.ServiceExposureKind, ID: exposureID}
	intent.Target = target
	var result tobari.ServiceOpenResult
	err := s.mutator.Invoke(ctx, mutationRequest(intent, target), func(actionContext context.Context, _ operation.Intent) error {
		var err error
		result, err = s.port.OpenServiceExposure(actionContext, exposureID)
		return err
	})
	if err != nil {
		return tobari.ServiceOpenResult{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceOpenResult{}, fault.Wrap(fault.KindContract, "invalid_service_open_result", "service open result is invalid", false, err)
	}
	return result, nil
}
