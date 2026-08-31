package installationmigrationcmd

import (
	"context"
	"errors"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	TaskPlan  = "installation migration plan"
	TaskApply = "installation migration apply"
)

type Port interface {
	PlanInstallationMigration(context.Context) (tobari.InstallationMigrationPlan, error)
	ApplyInstallationMigration(context.Context, string) (tobari.InstallationMigrationResult, error)
}

type mutationPolicy struct{}

func (mutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect != operation.EffectWrite || intent.Target.Kind != tobari.InstallationMigrationPlanReferenceKind || intent.Target.ParentID != "" || tobari.ParseInstallationMigrationPlanRef(intent.Target.ID) != nil {
		return fault.New(fault.KindRejected, "mutation_rejected", "installation migration target is invalid", false)
	}
	return nil
}

type Service struct {
	port    Port
	mutator *execution.Invoker
}

func New(port Port) *Service { return &Service{port: port, mutator: execution.New(mutationPolicy{})} }

func Impact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *Service) Plan(ctx context.Context) (tobari.InstallationMigrationPlan, error) {
	if s == nil || portcheck.IsNil(s.port) {
		return tobari.InstallationMigrationPlan{}, missingPort()
	}
	plan, err := s.port.PlanInstallationMigration(ctx)
	if err != nil {
		return tobari.InstallationMigrationPlan{}, migrationFault(err, false)
	}
	if err := plan.Validate(); err != nil {
		return tobari.InstallationMigrationPlan{}, fault.WithClassification(
			fault.Wrap(fault.KindContract, "invalid_installation_migration_plan", "installation migration plan is invalid", false, err),
			fault.PhaseVerification, fault.ChangeUnknown,
		)
	}
	return plan, nil
}

func (s *Service) Apply(ctx context.Context, intent operation.Intent, planRef string) (tobari.InstallationMigrationResult, error) {
	if s == nil || portcheck.IsNil(s.port) {
		return tobari.InstallationMigrationResult{}, missingPort()
	}
	if err := tobari.ParseInstallationMigrationPlanRef(planRef); err != nil {
		return tobari.InstallationMigrationResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_installation_migration_plan_ref", "installation migration plan reference is invalid", false, err)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskApply, ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.InstallationMigrationPlanReferenceKind, ID: planRef}, ExpectedImpact: Impact()}
	var result tobari.InstallationMigrationResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		value, err := s.port.ApplyInstallationMigration(actionContext, planRef)
		if err != nil {
			return migrationFault(err, true)
		}
		if err := value.Validate(); err != nil || value.PlanRef != planRef {
			return fault.WithClassification(
				fault.Wrap(fault.KindContract, "invalid_installation_migration_result", "installation migration result is invalid", false, err),
				fault.PhaseVerification, fault.ChangeUnknown,
			)
		}
		result = value
		return nil
	})
	return result, err
}

func missingPort() error {
	return fault.New(fault.KindInternal, "missing_runtime", "installation migration runtime is not configured", false)
}

func migrationFault(err error, apply bool) error {
	switch {
	case errors.Is(err, tobari.ErrMigrationNotSupported):
		return fault.Wrap(fault.KindRejected, "installation_migration_not_supported", "installation state is not an exact supported typed authority.json", false, err)
	case errors.Is(err, tobari.ErrMigrationSourceUnsafe):
		return fault.Wrap(fault.KindRejected, "installation_migration_source_rejected", "installation migration source or destination is unsafe", false, err)
	case errors.Is(err, tobari.ErrMigrationSourceChanged):
		return fault.Wrap(fault.KindRejected, "installation_migration_plan_stale", "installation migration source changed after planning", false, err, fault.NextAction{Command: TaskPlan, Reason: "Review a fresh exact migration plan."})
	case errors.Is(err, tobari.ErrMigrationWriteFailed):
		return fault.Wrap(fault.KindUnavailable, "installation_migration_incomplete", "installation migration could not be committed", false, err)
	default:
		phase := fault.PhaseObservation
		change := fault.ChangeNotApplicable
		if apply {
			phase, change = fault.PhasePrecondition, fault.ChangeNone
		}
		return fault.WithClassification(fault.Wrap(fault.KindUnavailable, "installation_migration_failed", "installation migration could not be completed", false, err), phase, change)
	}
}
