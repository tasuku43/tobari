// Package policypresetcmd owns local policy-preset discovery and creation.
package policypresetcmd

import (
	"context"
	"errors"
	"strings"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type RuntimePort interface {
	ListPolicyPresets(context.Context) (tobari.PolicyPresetResult, error)
	ShowPolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error)
	ValidatePolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error)
	InitPolicyPreset(context.Context, string) (tobari.PolicyPresetResult, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.PolicyPresetCatalogTargetKind && intent.Target.ParentID == tobari.PolicyPresetCatalogTargetID && intent.Target.ID == "" {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Policy preset mutation target is not owned by Tobari", false)
}

type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}
func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Policy preset runtime is not configured", false)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (tobari.PolicyPresetResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	return s.runtime.ListPolicyPresets(ctx)
}
func (s *Service) Show(ctx context.Context, origin string) (tobari.PolicyPresetResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if err := tobari.ValidatePolicyPresetOrigin(origin); err != nil {
		return tobari.PolicyPresetResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_policy_preset", "Policy preset selector is invalid", false, err)
	}
	return s.runtime.ShowPolicyPreset(ctx, origin)
}
func (s *Service) Validate(ctx context.Context, origin string) (tobari.PolicyPresetResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if !strings.HasPrefix(origin, "custom/") {
		return tobari.PolicyPresetResult{}, fault.New(fault.KindInvalidInput, "invalid_policy_preset", "Only custom policy preset sources can be validated", false)
	}
	if err := tobari.ValidatePolicyPresetOrigin(origin); err != nil {
		return tobari.PolicyPresetResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_policy_preset", "Custom policy preset selector is invalid", false, err)
	}
	return s.runtime.ValidatePolicyPreset(ctx, origin)
}
func (s *Service) Init(ctx context.Context, intent operation.Intent, name string) (tobari.PolicyPresetResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if err := tobari.ValidatePolicyPresetOrigin("custom/" + name); err != nil {
		return tobari.PolicyPresetResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_policy_preset", "Custom policy preset name is invalid", false, err)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate, ExpectedTarget: operation.TargetRef{Kind: tobari.PolicyPresetCatalogTargetKind, ParentID: tobari.PolicyPresetCatalogTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.PolicyPresetResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, createErr := s.runtime.InitPolicyPreset(actionContext, name)
		if createErr != nil {
			if errors.Is(createErr, context.Canceled) || errors.Is(createErr, context.DeadlineExceeded) {
				return createErr
			}
			return fault.Wrap(fault.KindRejected, "policy_preset_init_failed", "Custom policy preset could not be created", false, createErr)
		}
		result = created
		return nil
	})
	return result, err
}
