// Package runtimecmd owns installation-wide reusable Runtime tasks.
package runtimecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type RuntimePort interface {
	ListRuntimes(context.Context) (tobari.RuntimeListResult, error)
	ShowRuntime(context.Context, string) (tobari.RuntimeReport, error)
	RuntimeHistory(context.Context, string) (tobari.RuntimeReport, error)
	CreateRuntime(context.Context, string, tobari.RuntimeCopySource) (tobari.RuntimeReport, error)
	ResolveRuntimeReference(context.Context, string) (tobari.RuntimeManifest, error)
	BuildManagedRuntimeByReference(context.Context, string, io.Writer) (tobari.RuntimeReport, error)
	ReadRuntimeLifecycleSnapshot(context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error)
	ReadRuntimeBuildRecovery(context.Context) (tobari.RuntimeBuildRecovery, bool, error)
	RecoverRuntimeBuildByReference(context.Context, string, tobari.RuntimeBuildRecoveryKind) error
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.RuntimeCatalogTargetKind &&
		intent.Target.ParentID == tobari.RuntimeCatalogTargetID && intent.Target.ID == "" {
		return nil
	}
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.RuntimeCatalogTargetKind &&
		intent.Target.ID == tobari.RuntimeCatalogTargetID {
		return nil
	}
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.RuntimeReferenceKind && intent.Target.ID != "" {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Runtime mutation target is not owned by Tobari", false)
}

type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}

func (s *Service) PlanPrune(ctx context.Context) (tobari.RuntimePrunePlan, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimePrunePlan{}, err
	}
	snapshot, observedAt, err := s.runtime.ReadRuntimeLifecycleSnapshot(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.RuntimePrunePlan{}, err
		}
		return tobari.RuntimePrunePlan{}, fault.Wrap(fault.KindRejected, "runtime_retirement_observation_unknown", "Runtime lifecycle could not be observed completely", false, err)
	}
	plan, err := tobari.PlanRuntimePrune(snapshot, observedAt)
	if err != nil {
		return tobari.RuntimePrunePlan{}, fault.Wrap(fault.KindContract, "invalid_runtime_prune_plan", "Runtime prune plan is invalid", false, err)
	}
	return plan, nil
}

func (s *Service) ReviewRecovery(ctx context.Context) (tobari.RuntimeBuildRecovery, bool, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeBuildRecovery{}, false, err
	}
	recovery, found, err := s.runtime.ReadRuntimeBuildRecovery(ctx)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return tobari.RuntimeBuildRecovery{}, false, err
	}
	if err != nil {
		return tobari.RuntimeBuildRecovery{}, false, fault.Wrap(fault.KindRejected, "runtime_recovery_observation_unknown", "Runtime build recovery authority could not be observed completely", false, err, fault.NextAction{Command: "review runtimes", Reason: "Retry the trusted-host read-only review."})
	}
	if !found {
		return tobari.RuntimeBuildRecovery{}, false, nil
	}
	if err := recovery.Validate(); err != nil {
		return tobari.RuntimeBuildRecovery{}, false, fault.Wrap(fault.KindContract, "runtime_recovery_contract_invalid", "Runtime build recovery is invalid", false, err)
	}
	return recovery, true, nil
}

func (s *Service) Recover(ctx context.Context, intent operation.Intent, recovery tobari.RuntimeBuildRecovery) (tobari.RuntimeReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := recovery.Validate(); err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_recovery", "Runtime build recovery target is invalid", false, err, fault.NextAction{Command: "review runtimes", Reason: "Restart from current recovery authority."})
	}
	request := execution.Request{Intent: intent, ExpectedCommand: "runtime build", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: recovery.RuntimeRef}, ExpectedImpact: intent.Impact}
	var result tobari.RuntimeReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		if err := s.runtime.RecoverRuntimeBuildByReference(actionContext, recovery.RuntimeRef, recovery.Kind); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return fault.Wrap(fault.KindRejected, "runtime_recovery_failed", "Runtime build recovery remains incomplete", false, err, fault.NextAction{Command: "review runtimes", Reason: "Re-observe the retained journal before another mutation."})
		}
		manifest, err := s.runtime.ResolveRuntimeReference(actionContext, recovery.RuntimeRef)
		if err != nil || manifest.ID != recovery.RuntimeID || manifest.Name != recovery.Name || manifest.Kind != tobari.RuntimeKindManaged {
			if err == nil {
				err = fmt.Errorf("recovered Runtime identity changed")
			}
			return fault.Wrap(fault.KindContract, "runtime_recovery_contract_invalid", "Recovered Runtime identity is invalid", false, err, fault.NextAction{Command: "review runtimes", Reason: "Reconcile the current Runtime catalog."})
		}
		result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, NoChange: true}
		if recovery.Kind == tobari.RuntimeBuildRecoveryPublication {
			result.NoChange = false
			result.Built = true
		}
		if err := result.Validate(); err != nil {
			return fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime recovery report is invalid", false, err)
		}
		return nil
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	result, err = tobari.RuntimeReportWithReferences(result)
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime recovery report is invalid", false, err)
	}
	return result, nil
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Runtime catalog is not configured", false)
	}
	return nil
}

func (s *Service) List(ctx context.Context) (tobari.RuntimeListResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeListResult{}, err
	}
	result, err := s.runtime.ListRuntimes(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return tobari.RuntimeListResult{}, err
		}
		return tobari.RuntimeListResult{}, fault.Wrap(fault.KindInternal, "runtime_read_failed", "Runtime catalog could not be read", false, err, fault.NextAction{Command: "doctor", Reason: "Inspect the host Runtime store."})
	}
	if err := result.Validate(); err != nil {
		return tobari.RuntimeListResult{}, fault.Wrap(fault.KindContract, "invalid_runtime_list", "Runtime list is invalid", false, err)
	}
	result, err = runtimeListWithReferences(result)
	if err != nil {
		return tobari.RuntimeListResult{}, fault.Wrap(fault.KindContract, "invalid_runtime_list", "Runtime list is invalid", false, err)
	}
	return result, nil
}

func runtimeListWithReferences(result tobari.RuntimeListResult) (tobari.RuntimeListResult, error) {
	for index := range result.Items {
		result.Items[index].RuntimeRef = tobari.RuntimeRef(result.Items[index].ID)
		// Restore publishes this only when its consumer lands in the Catalog.
		result.Items[index].RevisionRef = ""
	}
	return result, result.Validate()
}

func (s *Service) Show(ctx context.Context, name string) (tobari.RuntimeReport, error) {
	return s.read(ctx, name, false)
}

func (s *Service) History(ctx context.Context, name string) (tobari.RuntimeReport, error) {
	return s.read(ctx, name, true)
}

func (s *Service) read(ctx context.Context, name string, history bool) (tobari.RuntimeReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_name", "Runtime name is invalid", false, err, fault.NextAction{Command: "runtime list", Reason: "Choose a Runtime from the local catalog."})
	}
	var result tobari.RuntimeReport
	var err error
	if history {
		result, err = s.runtime.RuntimeHistory(ctx, name)
	} else {
		result, err = s.runtime.ShowRuntime(ctx, name)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return tobari.RuntimeReport{}, err
	}
	if errors.Is(err, tobari.ErrRuntimeNotFound) {
		return tobari.RuntimeReport{}, fault.New(fault.KindNotFound, "runtime_not_found", "the named Runtime does not exist", false, fault.NextAction{Command: "runtime list", Reason: "Choose an existing Runtime."})
	}
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInternal, "runtime_read_failed", "Runtime could not be read", false, err, fault.NextAction{Command: "doctor", Reason: "Inspect the host Runtime store."})
	}
	if err := result.Validate(); err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime report is invalid", false, err)
	}
	result, err = tobari.RuntimeReportWithReferences(result)
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime report is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Create(ctx context.Context, intent operation.Intent, name, baseValue string) (tobari.RuntimeReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_name", "Runtime name is invalid", false, err)
	}
	base, err := tobari.ParseRuntimeCopySource(baseValue)
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_copy_source", "Runtime source Base is invalid", false, err, fault.NextAction{Command: "runtime list", Reason: "Choose standard or an existing managed Runtime name."})
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}, ExpectedImpact: intent.Impact}
	var result tobari.RuntimeReport
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, err := s.runtime.CreateRuntime(actionContext, name, base)
		if errors.Is(err, tobari.ErrRuntimeExists) {
			return fault.New(fault.KindRejected, "runtime_exists", "the named Runtime already exists", false, fault.NextAction{Command: "runtime show", Reason: "Inspect the existing Runtime before editing it."})
		}
		if errors.Is(err, tobari.ErrRuntimeNotFound) {
			return fault.New(fault.KindNotFound, "runtime_copy_source_not_found", "the named Runtime source Base does not exist", false, fault.NextAction{Command: "runtime list", Reason: "Choose standard or an existing managed Runtime name."})
		}
		if err != nil {
			if structured, ok := fault.PublicCopy(err); ok {
				return structured
			}
			return fault.Wrap(fault.KindRejected, "runtime_create_failed", "Runtime source could not be created", false, err, fault.NextAction{Command: "runtime list", Reason: "Inspect the local Runtime catalog."})
		}
		if err := created.Validate(); err != nil || created.Task != tobari.TaskRuntimeCreate || created.Runtime.Name != name ||
			created.Runtime.Kind != tobari.RuntimeKindManaged || len(created.Runtime.Revisions) != 0 || !created.Created {
			if err == nil {
				err = fmt.Errorf("created Runtime does not match the request")
			}
			return fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime creation report is invalid", false, err)
		}
		result = created
		return nil
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	result, err = tobari.RuntimeReportWithReferences(result)
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime creation report is invalid", false, err)
	}
	return result, nil
}

func (s *Service) Build(ctx context.Context, intent operation.Intent, runtimeRef string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := tobari.ValidateRuntimeRef(runtimeRef); err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindInvalidInput, "invalid_runtime_ref", "Runtime reference is invalid", false, err, fault.NextAction{Command: "runtime list", Reason: "Use one Runtime reference unchanged."})
	}
	if runtimeRef == tobari.StandardRuntimeID {
		return tobari.RuntimeReport{}, fault.New(fault.KindRejected, "runtime_not_managed", "the built-in standard Runtime cannot be built", false, fault.NextAction{Command: "runtime list", Reason: "Choose a managed Runtime."})
	}
	request := execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: runtimeRef}, ExpectedImpact: intent.Impact}
	var result tobari.RuntimeReport
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		manifest, err := s.runtime.ResolveRuntimeReference(actionContext, runtimeRef)
		if errors.Is(err, tobari.ErrRuntimeNotFound) {
			return fault.New(fault.KindNotFound, "runtime_not_found", "the referenced Runtime does not exist", false, fault.NextAction{Command: "runtime list", Reason: "Discover the current Runtime catalog."})
		}
		if err != nil {
			return fault.Wrap(fault.KindRejected, "runtime_reference_unresolved", "Runtime reference could not be resolved", false, err, fault.NextAction{Command: "runtime list", Reason: "Discover the current Runtime catalog."})
		}
		if manifest.Kind != tobari.RuntimeKindManaged {
			return fault.New(fault.KindRejected, "runtime_not_managed", "the built-in standard Runtime cannot be built", false, fault.NextAction{Command: "runtime list", Reason: "Choose a managed Runtime."})
		}
		built, err := s.runtime.BuildManagedRuntimeByReference(actionContext, runtimeRef, diagnostics)
		if errors.Is(err, tobari.ErrRuntimeNotFound) {
			return fault.New(fault.KindNotFound, "runtime_not_found", "the referenced Runtime does not exist", false, fault.NextAction{Command: "runtime list", Reason: "Choose an existing managed Runtime."})
		}
		if err != nil {
			if structured, ok := fault.PublicCopy(err); ok {
				return structured
			}
			return fault.Wrap(fault.KindRejected, "runtime_build_failed", "Runtime could not be built", false, err, fault.NextAction{Command: "runtime show", Reason: "Inspect the unchanged Runtime history and source path."})
		}
		if err := built.Validate(); err != nil || built.Task != tobari.TaskRuntimeBuildV1 || built.Runtime.ID != manifest.ID || (!built.Built && !built.NoChange) {
			if err == nil {
				err = fmt.Errorf("built Runtime does not match the request")
			}
			return fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime build report is invalid", false, err)
		}
		result = built
		return nil
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	result, err = tobari.RuntimeReportWithReferences(result)
	if err != nil {
		return tobari.RuntimeReport{}, fault.Wrap(fault.KindContract, "invalid_runtime_report", "Runtime build report is invalid", false, err)
	}
	return result, nil
}
