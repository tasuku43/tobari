// Package contextcmd owns the host-side Context composition workflow.
package contextcmd

import (
	"context"
	"errors"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// RuntimePort is the smallest boundary needed to inspect and change the
// host-owned active Context. It deliberately exposes no secret-reading API.
type RuntimePort interface {
	ListContexts(context.Context) (tobari.ContextListResult, error)
	ShowContext(context.Context, string) (tobari.ContextReport, error)
	CreateContext(context.Context, string, string, tobari.ContextPolicyMode) (tobari.ContextReport, error)
	UseContext(context.Context, string) (tobari.ContextReport, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate &&
		intent.Target.Kind == tobari.ContextCatalogTargetKind &&
		intent.Target.ParentID == tobari.ContextCatalogTargetID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite &&
		intent.Target.Kind == tobari.ContextTargetKind &&
		intent.Target.ID == tobari.ActiveContextTargetID {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Context mutation target is not owned by Tobari", false)
}

// Service coordinates Context tasks without depending on the filesystem or
// Docker implementation.
type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Context runtime is not configured", false)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (tobari.ContextListResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextListResult{}, err
	}
	result, err := s.runtime.ListContexts(ctx)
	if err != nil {
		return tobari.ContextListResult{}, fault.Wrap(
			fault.KindInternal, "context_read_failed", "Context list could not be read", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."},
		)
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextListResult{}, fault.Wrap(
			fault.KindContract, "invalid_context_list", "Context list is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."},
		)
	}
	return result, nil
}

func (s *Service) Show(ctx context.Context, name string) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if name != "" {
		if err := tobari.ValidateName(name); err != nil {
			return tobari.ContextReport{}, fault.Wrap(
				fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
				fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
			)
		}
	}
	result, err := s.runtime.ShowContext(ctx, name)
	if err != nil {
		return tobari.ContextReport{}, s.readError(err)
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindContract, "invalid_context_report", "Context report is invalid", false, err,
			fault.NextAction{Command: "context list", Reason: "Inspect the local Context collection."},
		)
	}
	return result, nil
}

func (s *Service) Create(
	ctx context.Context, intent operation.Intent, name, image string, mode tobari.ContextPolicyMode,
) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := validateCreateInput(name, image, mode); err != nil {
		return tobari.ContextReport{}, err
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextCatalogTargetKind, ParentID: tobari.ContextCatalogTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, createErr := s.runtime.CreateContext(actionContext, name, image, mode)
		if errors.Is(createErr, tobari.ErrContextExists) {
			return fault.New(
				fault.KindRejected, "context_exists", "the named Context already exists", false,
				fault.NextAction{Command: "context show", Reason: "Inspect the existing Context before choosing another name."},
			)
		}
		if createErr != nil {
			return fault.Wrap(fault.KindRejected, "context_create_failed", "Context could not be created", false, createErr,
				fault.NextAction{Command: "context list", Reason: "Inspect the local Context collection."})
		}
		result = created
		return nil
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (s *Service) Use(ctx context.Context, intent operation.Intent, name string) (tobari.ContextReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ContextReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_context_name", "Context name is invalid", false, err,
			fault.NextAction{Command: "context list", Reason: "Choose a named Context from the local collection."},
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.ContextTargetKind, ID: tobari.ActiveContextTargetID},
		ExpectedImpact: intent.Impact,
	}
	var result tobari.ContextReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		used, useErr := s.runtime.UseContext(actionContext, name)
		if errors.Is(useErr, tobari.ErrContextNotFound) {
			return fault.New(
				fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
				fault.NextAction{Command: "context list", Reason: "Choose an existing Context or create it first."},
			)
		}
		if useErr != nil {
			return fault.Wrap(fault.KindRejected, "context_use_failed", "Context selection could not be changed", false, useErr,
				fault.NextAction{Command: "context show", Reason: "Inspect the active Context selection."})
		}
		result = used
		return nil
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func validateCreateInput(name, image string, mode tobari.ContextPolicyMode) error {
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion, Name: name,
		AgentProfile: tobari.DefaultProfile, Image: image, PolicyMode: mode,
	}
	if err := manifest.Validate(); err != nil {
		return fault.Wrap(
			fault.KindInvalidInput, "invalid_context", "Context definition is invalid", false, err,
			fault.NextAction{Command: "help context create", Reason: "Correct the Context name, image, or policy mode."},
		)
	}
	return nil
}

func (s *Service) readError(err error) error {
	if errors.Is(err, tobari.ErrContextNotFound) {
		return fault.New(fault.KindNotFound, "context_not_found", "the named Context does not exist", false,
			fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
	}
	return fault.Wrap(fault.KindInternal, "context_read_failed", "Context could not be read", false, err,
		fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."})
}
