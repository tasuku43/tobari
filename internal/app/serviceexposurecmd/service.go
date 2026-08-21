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
	ListServiceExposures(context.Context) (tobari.ServiceExposureList, error)
	StopServiceExposure(context.Context, string) error
	ListServiceRequests(context.Context) (tobari.ServiceRequestList, error)
	AllowServiceRequest(context.Context, string) (tobari.ServiceExposure, error)
	DenyServiceRequest(context.Context, string) error
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

func (s *Service) List(ctx context.Context) (tobari.ServiceExposureList, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceExposureList{}, err
	}
	result, err := s.port.ListServiceExposures(ctx)
	if err != nil {
		return tobari.ServiceExposureList{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceExposureList{}, fault.Wrap(fault.KindContract, "invalid_service_exposure_list", "service exposure list is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Pending(ctx context.Context) (tobari.ServiceRequestList, error) {
	if err := s.requirePort(); err != nil {
		return tobari.ServiceRequestList{}, err
	}
	result, err := s.port.ListServiceRequests(ctx)
	if err != nil {
		return tobari.ServiceRequestList{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.ServiceRequestList{}, fault.Wrap(fault.KindContract, "invalid_service_request_list", "service request list is invalid", false, err)
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
