// Package configuratorcmd owns isolated agent-guided configuration authoring.
package configuratorcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type DraftPort interface {
	Reserve(context.Context, tobari.ConfiguratorSeed, tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error)
	Materialize(context.Context, tobari.ConfiguratorDraft) error
	Freeze(context.Context, tobari.ConfiguratorDraft) (tobari.ConfiguratorSubmission, error)
	PendingTask(context.Context, string, tobari.ConfiguratorTask, string) (tobari.ConfiguratorDraft, tobari.ConfiguratorSubmission, bool, bool, error)
	ConfirmTask(context.Context, tobari.ConfiguratorSubmission) error
	CompleteTask(context.Context, tobari.ConfiguratorSubmission) error
	RetireUnmaterializedTask(context.Context, tobari.ConfiguratorDraft) error
	ArmHomeAdoption(context.Context, tobari.ConfiguratorSubmission) error
	PendingHomeAdoption(context.Context, string) (tobari.ConfiguratorSubmission, bool, error)
	AdoptHome(context.Context, tobari.ConfiguratorSubmission, tobari.ContextAuthoritySnapshot, ...func() error) error
}

type RunnerPort interface {
	PrepareConfiguratorRuntime(context.Context, tobari.RuntimeBinding) error
	RunConfigurator(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorIsolation, io.Reader, io.Writer) error
	ApplyConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource, io.Writer) (tobari.RuntimeBinding, error)
}

type RuntimeSourcePublicationPort interface {
	ApplyConfiguratorRuntimeSourceOnly(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource) error
	ConfiguratorRuntimeSourcePublished(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource) (bool, error)
}

type PolicyPublicationPort interface {
	ConfiguratorPolicyPublished(context.Context, tobari.ConfiguratorSubmission) (bool, string, error)
}

type TaskRecovery struct {
	Draft          tobari.ConfiguratorDraft
	Submission     tobari.ConfiguratorSubmission
	Frozen         bool
	ApplyConfirmed bool
	Published      bool
	PendingPlanRef string
}

type SettlementContextFactory func() (context.Context, context.CancelFunc)

const maximumTaskSettlementWindow = 30 * time.Second

func (s *Service) ReconcileTask(ctx context.Context, seed tobari.ConfiguratorSeed) (TaskRecovery, bool, error) {
	if s == nil || s.drafts == nil || seed.Validate() != nil || (seed.Task != tobari.ConfiguratorTaskRuntime && seed.Task != tobari.ConfiguratorTaskPolicy) {
		return TaskRecovery{}, false, fmt.Errorf("Configurator task recovery is invalid")
	}
	scope, err := seed.ConfiguratorScopeKey()
	if err != nil {
		return TaskRecovery{}, false, err
	}
	draft, submission, frozen, confirmed, err := s.drafts.PendingTask(ctx, scope, seed.Task, seed.TargetRuntimeID)
	if err != nil || draft.ID == "" {
		return TaskRecovery{}, false, err
	}
	result := TaskRecovery{Draft: draft, Submission: submission, Frozen: frozen, ApplyConfirmed: confirmed}
	if !frozen || !confirmed {
		return result, true, nil
	}
	var published bool
	switch seed.Task {
	case tobari.ConfiguratorTaskRuntime:
		port, ok := s.runner.(RuntimeSourcePublicationPort)
		if !ok || submission.RuntimeSource == nil {
			return TaskRecovery{}, false, fmt.Errorf("Runtime task publication recovery is unavailable")
		}
		published, err = port.ConfiguratorRuntimeSourcePublished(ctx, submission.Draft, *submission.RuntimeSource)
	case tobari.ConfiguratorTaskPolicy:
		port, ok := s.stager.(PolicyPublicationPort)
		if !ok {
			return TaskRecovery{}, false, fmt.Errorf("Policy task publication recovery is unavailable")
		}
		published, result.PendingPlanRef, err = port.ConfiguratorPolicyPublished(ctx, submission)
	}
	if err != nil {
		return TaskRecovery{}, false, err
	}
	if result.PendingPlanRef != "" {
		planTemplateID, parseErr := tobari.ParseWorkspaceTemplateChangePlanRef(result.PendingPlanRef)
		if parseErr != nil || planTemplateID != submission.Draft.TemplateID || published {
			return TaskRecovery{}, false, errors.Join(tobari.ErrResourceSourceRecoveryRequired, parseErr)
		}
	}
	result.Published = published
	return result, true, nil
}

func (s *Service) ConfirmTask(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission) error {
	if s == nil || s.drafts == nil || submission.Validate() != nil || submission.Draft.Task == tobari.ConfiguratorTaskAggregate {
		return fmt.Errorf("Configurator task confirmation is invalid")
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return err
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		return s.drafts.ConfirmTask(actionContext, submission)
	})
}

// ApplyRuntimeAssistSource publishes only the exact frozen editable source.
// Building an immutable Runtime revision remains the separate runtime build
// action and cannot be implied by this confirmation.
func (s *Service) ApplyRuntimeAssistSource(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission, settlements ...SettlementContextFactory) error {
	if s == nil || s.drafts == nil || s.runner == nil || submission.Validate() != nil || submission.Draft.Task != tobari.ConfiguratorTaskRuntime || submission.RuntimeSource == nil {
		return fmt.Errorf("Runtime assist source publication is invalid")
	}
	publisher, ok := s.runner.(RuntimeSourcePublicationPort)
	if !ok {
		return fmt.Errorf("Runtime assist source publication is unavailable")
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return err
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		if err := s.drafts.ConfirmTask(actionContext, submission); err != nil {
			return err
		}
		if err := publisher.ApplyConfiguratorRuntimeSourceOnly(actionContext, submission.Draft, *submission.RuntimeSource); err != nil {
			if errors.Is(err, tobari.ErrResourceSourceChanged) {
				return fault.WithClassification(fault.Wrap(
					fault.KindRejected,
					"resource_source_changed",
					"The managed Runtime source changed after the reviewed assist submission was frozen.",
					true,
					err,
					fault.NextAction{Command: "help runtime assist", Reason: "Re-open the exact current Runtime source and review a fresh submission."},
				), fault.PhasePrecondition, fault.ChangeNone)
			}
			return err
		}
		settlementContext, cancel, err := newTaskSettlementContext(firstSettlementFactory(settlements))
		if err != nil {
			return configurationTaskSettlementIncompleteFault(err, submission.Draft.Task)
		}
		defer cancel()
		return s.completePublishedTask(settlementContext, submission)
	})
}

func (s *Service) CompleteTask(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission, settlements ...SettlementContextFactory) error {
	if s == nil || s.drafts == nil || submission.Validate() != nil || submission.Draft.Task == tobari.ConfiguratorTaskAggregate {
		return fmt.Errorf("Configurator task settlement is invalid")
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return err
	}
	settlementContext, cancel, err := newTaskSettlementContext(firstSettlementFactory(settlements))
	if err != nil {
		return configurationTaskSettlementIncompleteFault(err, submission.Draft.Task)
	}
	defer cancel()
	return s.mutator.Invoke(settlementContext, request, func(actionContext context.Context, _ operation.Intent) error {
		return s.completePublishedTask(actionContext, submission)
	})
}

func (s *Service) completePublishedTask(ctx context.Context, submission tobari.ConfiguratorSubmission) error {
	// Publication is already authoritative. Settlement must survive caller
	// cancellation and may never turn that confirmed mutation into replay
	// permission or a change=none failure.
	if err := s.drafts.CompleteTask(ctx, submission); err != nil {
		return configurationTaskSettlementIncompleteFault(err, submission.Draft.Task)
	}
	return nil
}

func configurationTaskSettlementIncompleteFault(err error, task tobari.ConfiguratorTask) error {
	command := "help policy assist"
	if task == tobari.ConfiguratorTaskRuntime {
		command = "help runtime assist"
	}
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"configuration_task_settlement_incomplete",
		"The reviewed configuration source was published, but its local task receipt could not be settled.",
		false,
		err,
		fault.NextAction{Command: command, Reason: "Resume the same exact assistance task to settle its confirmed source publication."},
	), fault.PhaseVerification, fault.ChangeConfirmed)
}

func newTaskSettlementContext(factory SettlementContextFactory) (context.Context, context.CancelFunc, error) {
	if factory == nil {
		return nil, nil, fmt.Errorf("bounded task settlement context is required")
	}
	ctx, cancel := factory()
	if ctx == nil || cancel == nil {
		return nil, nil, fmt.Errorf("bounded task settlement context is invalid")
	}
	deadline, bounded := ctx.Deadline()
	remaining := time.Until(deadline)
	if !bounded || ctx.Err() != nil || remaining <= 0 || remaining > maximumTaskSettlementWindow {
		cancel()
		return nil, nil, fmt.Errorf("bounded task settlement context is invalid")
	}
	return ctx, cancel, nil
}

func firstSettlementFactory(values []SettlementContextFactory) SettlementContextFactory {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

type AttachmentPort interface {
	// AcquireConfiguratorAuthorAttachment revalidates the draft's exact authority
	// identity under the catalog fence and returns the live Context/Project
	// lease without reopening a selection-to-attachment race.
	AcquireConfiguratorAuthorAttachment(context.Context, tobari.ConfiguratorDraft) (func() error, error)
	// AcquireConfiguratorPublicationAttachment resumes one exact durable
	// publication receipt across pre-publication and post-authority crash phases.
	AcquireConfiguratorPublicationAttachment(context.Context, tobari.ConfiguratorSubmission) (func() error, error)
}

func (s *Service) ApplyRuntimeSource(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission, diagnostics io.Writer) (tobari.ConfiguratorSubmission, error) {
	if s == nil || s.runner == nil {
		return tobari.ConfiguratorSubmission{}, fmt.Errorf("Configurator Runtime source service is unavailable")
	}
	if err := submission.Validate(); err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	if submission.RuntimeSource == nil || !submission.RuntimeSource.Changed {
		return submission, nil
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	var result tobari.ConfiguratorSubmission
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		binding, err := s.runner.ApplyConfiguratorRuntimeSource(actionContext, submission.Draft, *submission.RuntimeSource, diagnostics)
		if err != nil {
			return err
		}
		applied, err := submission.WithAppliedRuntime(binding)
		if err != nil {
			return err
		}
		result = applied
		return nil
	})
	return result, err
}

type SubmissionPort interface {
	StageConfiguratorSubmission(context.Context, tobari.ConfiguratorSubmission) (tobari.ConfiguratorStage, error)
	DiscardConfiguratorStage(context.Context, tobari.ConfiguratorSubmission, tobari.ConfiguratorStage) error
	PendingConfiguratorStage(context.Context, tobari.WorkspaceTemplateID) (tobari.ConfiguratorPendingStage, bool, error)
	PendingConfiguratorStageForProject(context.Context, string) (tobari.ConfiguratorPendingStage, bool, error)
	BindConfiguratorStagePlan(context.Context, tobari.ConfiguratorPendingStage, string) (tobari.ConfiguratorPendingStage, error)
	ConfirmConfiguratorStageApply(context.Context, tobari.ConfiguratorPendingStage) (tobari.ConfiguratorPendingStage, error)
	ConfirmConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission, tobari.ContextAuthoritySnapshot) error
	BeginConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error
	CompleteConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error
	PendingConfiguratorPublicationForProject(context.Context, string) (tobari.ConfiguratorSubmission, bool, error)
}

func (s *Service) PendingStageForProject(ctx context.Context, projectRoot string) (tobari.ConfiguratorPendingStage, bool, error) {
	if s == nil || s.stager == nil || tobari.ValidateCanonicalRoot(projectRoot) != nil {
		return tobari.ConfiguratorPendingStage{}, false, fmt.Errorf("Configurator Project stage request is invalid")
	}
	return s.stager.PendingConfiguratorStageForProject(ctx, projectRoot)
}

func (s *Service) PendingStage(ctx context.Context, id tobari.WorkspaceTemplateID) (tobari.ConfiguratorPendingStage, bool, error) {
	if s == nil || s.stager == nil || id.Validate() != nil {
		return tobari.ConfiguratorPendingStage{}, false, fmt.Errorf("Configurator pending stage request is invalid")
	}
	return s.stager.PendingConfiguratorStage(ctx, id)
}

func (s *Service) BindStagePlan(ctx context.Context, intent operation.Intent, pending tobari.ConfiguratorPendingStage, planRef string) (tobari.ConfiguratorPendingStage, error) {
	if s == nil || s.stager == nil || pending.Validate() != nil || planRef == "" {
		return tobari.ConfiguratorPendingStage{}, fmt.Errorf("Configurator stage Plan binding is invalid")
	}
	request, err := configuratorRequest(intent, pending.Submission.Draft.Task, pending.Submission.Draft.TargetRuntimeID)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	var result tobari.ConfiguratorPendingStage
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var err error
		result, err = s.stager.BindConfiguratorStagePlan(actionContext, pending, planRef)
		return err
	})
	return result, err
}

func (s *Service) ConfirmStageApply(ctx context.Context, intent operation.Intent, pending tobari.ConfiguratorPendingStage) (tobari.ConfiguratorPendingStage, error) {
	if s == nil || s.stager == nil || pending.Validate() != nil || pending.PlanRef == "" {
		return tobari.ConfiguratorPendingStage{}, fmt.Errorf("Configurator stage Apply confirmation is invalid")
	}
	request, err := configuratorRequest(intent, pending.Submission.Draft.Task, pending.Submission.Draft.TargetRuntimeID)
	if err != nil {
		return tobari.ConfiguratorPendingStage{}, err
	}
	var result tobari.ConfiguratorPendingStage
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var err error
		result, err = s.stager.ConfirmConfiguratorStageApply(actionContext, pending)
		return err
	})
	return result, err
}

func (s *Service) DiscardStage(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission, stage tobari.ConfiguratorStage) error {
	if s == nil || s.stager == nil || stage.ValidateFor(submission) != nil {
		return fmt.Errorf("Configurator staged submission discard is invalid")
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return err
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		return s.stager.DiscardConfiguratorStage(actionContext, submission, stage)
	})
}

type Service struct {
	drafts      DraftPort
	runner      RunnerPort
	stager      SubmissionPort
	attachments AttachmentPort
	mutator     *execution.Invoker
}

func New(drafts DraftPort, runner RunnerPort, stager SubmissionPort, attachments AttachmentPort) *Service {
	return &Service{drafts: drafts, runner: runner, stager: stager, attachments: attachments, mutator: execution.New(ownedPolicy{})}
}

// Stage writes one already-frozen evolve submission to canonical desired
// source. It does not Apply authority; canonical Plan/Apply remains mandatory.
func (s *Service) Stage(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission) (tobari.ConfiguratorStage, error) {
	if s == nil || s.stager == nil {
		return tobari.ConfiguratorStage{}, fmt.Errorf("Configurator submission staging is unavailable")
	}
	if err := submission.Validate(); err != nil || submission.Draft.Purpose != tobari.ConfiguratorPurposeEvolve {
		return tobari.ConfiguratorStage{}, fmt.Errorf("Configurator evolve submission is invalid: %w", err)
	}
	request, err := configuratorRequest(intent, submission.Draft.Task, submission.Draft.TargetRuntimeID)
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	var result tobari.ConfiguratorStage
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) (actionErr error) {
		staged, stageErr := s.stager.StageConfiguratorSubmission(actionContext, submission)
		if stageErr != nil {
			return stageErr
		}
		if err := staged.ValidateFor(submission); err != nil {
			return err
		}
		result = staged
		return nil
	})
	if err != nil {
		return tobari.ConfiguratorStage{}, err
	}
	return result, nil
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch intent.Command {
	case "configure":
		return validateIntent(intent)
	case "runtime assist":
		return validateIntentForTask(intent, tobari.ConfiguratorTaskRuntime, intent.Target.ID)
	case "policy assist":
		return validateIntentForTask(intent, tobari.ConfiguratorTaskPolicy, "")
	default:
		return fmt.Errorf("Configurator mutation command is invalid")
	}
}

func Impact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

// PrepareRuntime makes only immutable execution material available after the
// user chooses an agent and before its container starts. It grants no
// configuration or cluster authority.
func (s *Service) PrepareRuntime(ctx context.Context, intent operation.Intent, seed tobari.ConfiguratorSeed) error {
	if s == nil || s.runner == nil {
		return fmt.Errorf("Configurator service is unavailable")
	}
	if err := seed.Validate(); err != nil {
		return err
	}
	request, err := configuratorRequest(intent, seed.Task, seed.TargetRuntimeID)
	if err != nil {
		return err
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		return s.runner.PrepareConfiguratorRuntime(actionContext, seed.Runtime())
	})
}

func (s *Service) AcquirePublicationAttachment(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission) (func() error, error) {
	if s == nil || s.attachments == nil || submission.Validate() != nil || !submission.Draft.NeedsHomeAdoption() {
		return nil, fmt.Errorf("Configurator attachment lease request is invalid")
	}
	request := execution.Request{Intent: intent, ExpectedCommand: "configure", ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, ExpectedImpact: Impact()}
	var release func() error
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var err error
		release, err = s.attachments.AcquireConfiguratorPublicationAttachment(actionContext, submission)
		return err
	})
	if err != nil {
		return nil, err
	}
	return release, nil
}

// Author creates or resumes a working copy, hands the terminal to the selected
// agent, then freezes one immutable submission without activating it.
func (s *Service) Author(
	ctx context.Context,
	intent operation.Intent,
	seed tobari.ConfiguratorSeed,
	agent tobari.ConfiguratorAgent,
	in io.Reader,
	visible io.Writer,
	settlements ...SettlementContextFactory,
) (tobari.ConfiguratorSubmission, error) {
	if s == nil || s.drafts == nil || s.runner == nil || s.attachments == nil {
		return tobari.ConfiguratorSubmission{}, fmt.Errorf("Configurator service is unavailable")
	}
	if err := validateIntentForTask(intent, seed.Task, seed.TargetRuntimeID); err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	if err := agent.Validate(); err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	if err := seed.Validate(); err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	request, err := configuratorRequest(intent, seed.Task, seed.TargetRuntimeID)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	var submission tobari.ConfiguratorSubmission
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) (actionErr error) {
		draft, err := s.drafts.Reserve(actionContext, seed, agent)
		if err != nil {
			return err
		}
		if err := draft.Validate(); err != nil {
			return err
		}
		var adoptionContextIDs []tobari.ContextID
		if draft.AdoptionContextID != "" {
			adoptionContextIDs = append(adoptionContextIDs, draft.AdoptionContextID)
		}
		expected, err := tobari.NewConfiguratorDraft(seed, agent, draft.TemplateID, adoptionContextIDs...)
		if err != nil || expected != draft {
			return fmt.Errorf("Configurator draft does not match its requested seed and agent: %w", err)
		}
		release, err := s.attachments.AcquireConfiguratorAuthorAttachment(actionContext, draft)
		if err != nil {
			if draft.Task == tobari.ConfiguratorTaskRuntime && (errors.Is(err, tobari.ErrContextBindingNotFound) || errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale)) {
				settlementContext, cancel, contextErr := newTaskSettlementContext(firstSettlementFactory(settlements))
				if contextErr != nil {
					return contextErr
				}
				defer cancel()
				if retireErr := s.drafts.RetireUnmaterializedTask(settlementContext, draft); retireErr != nil {
					return configurationTaskRetirementIncompleteFault(errors.Join(err, retireErr))
				}
				return runtimeAssistMaterialRetiredFault(err)
			}
			return err
		}
		defer func() { actionErr = errors.Join(actionErr, release()) }()
		if err := s.drafts.Materialize(actionContext, draft); err != nil {
			if errors.Is(err, tobari.ErrConfiguratorTaskRetirementIncomplete) && draft.Task == tobari.ConfiguratorTaskRuntime {
				return configurationTaskRetirementIncompleteFault(err)
			}
			if errors.Is(err, tobari.ErrResourceSourceChanged) && draft.Task == tobari.ConfiguratorTaskRuntime {
				return runtimeAssistMaterialRetiredFault(err)
			}
			return err
		}
		if err := s.runner.PrepareConfiguratorRuntime(actionContext, seed.Runtime()); err != nil {
			return materialRetainedFault(
				draft.Task,
				"Configuration stopped after its exact Runtime image could not be prepared; the managed task material is retained.",
				err,
			)
		}
		isolation := tobari.DirectEgressConfiguratorIsolation()
		if err := s.runner.RunConfigurator(actionContext, draft, isolation, in, visible); err != nil {
			if errors.Is(err, tobari.ErrConfiguratorTransientCleanupUnknown) {
				return CleanupIncompleteFault(draft.Task, err)
			}
			if errors.Is(err, tobari.ErrNativeLoginBridgeUnavailable) {
				return materialRetainedFault(draft.Task, "Configuration stopped after its native-login bridge became unavailable; the managed Home is retained.", err)
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return materialRetainedFault(draft.Task, "Configuration was canceled after its managed Home was materialized; transient Configurator resources were removed.", err)
			}
			return err
		}
		frozen, err := s.drafts.Freeze(actionContext, draft)
		if err != nil {
			return err
		}
		if err := frozen.Validate(); err != nil || frozen.Draft != draft {
			return fmt.Errorf("Configurator submission is invalid: %w", err)
		}
		submission = frozen
		return nil
	})
	return submission, err
}

// MaterialRetainedFault preserves the confirmed managed Home while routing
// recovery through the same explicit task instead of ambient Project state.
func MaterialRetainedFault(task tobari.ConfiguratorTask, err error) error {
	return materialRetainedFault(task, "Configuration stopped after immutable execution Runtime preparation; its managed Home may also be retained.", err)
}

func materialRetainedFault(task tobari.ConfiguratorTask, message string, err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindCanceled,
		"configuration_material_retained",
		message,
		false,
		err,
		configurationTaskRecovery(task, false),
	), fault.PhaseMutation, fault.ChangeConfirmed)
}

// CleanupIncompleteFault preserves the strongest known partial cleanup state
// and routes reconciliation through the same explicit assistance task.
func CleanupIncompleteFault(task tobari.ConfiguratorTask, err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"configuration_cleanup_incomplete",
		"Configuration stopped and its managed Home is retained, but bounded cleanup could not confirm removal of every transient Configurator resource.",
		false,
		err,
		configurationTaskRecovery(task, true),
	), fault.PhaseMutation, fault.ChangePartial)
}

func configurationTaskRecovery(task tobari.ConfiguratorTask, cleanup bool) fault.NextAction {
	switch task {
	case tobari.ConfiguratorTaskRuntime:
		reason := "Resume the same exact retained Runtime assistance task with its original Runtime reference."
		if cleanup {
			reason = "Repair Docker health, then resume the same exact Runtime assistance task with its original Runtime reference."
		}
		return fault.NextAction{Command: "help runtime assist", Reason: reason}
	case tobari.ConfiguratorTaskPolicy:
		reason := "Resume the same exact retained Policy assistance task with its original Context reference."
		if cleanup {
			reason = "Repair Docker health, then resume the same exact Policy assistance task with its original Context reference."
		}
		return fault.NextAction{Command: "help policy assist", Reason: reason}
	default:
		reason := "Reconcile current authority before resuming the retained Project configuration material."
		if cleanup {
			reason = "Reconcile current Project authority and Docker health before resuming configuration."
		}
		return fault.NextAction{Command: "status", Reason: reason}
	}
}

func runtimeAssistMaterialRetiredFault(err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindRejected,
		"runtime_assist_material_retired",
		"The managed Runtime source changed before the isolated agent started; the stale task material was retired.",
		true,
		err,
		fault.NextAction{Command: "help runtime assist", Reason: "Retry from the exact current managed Runtime source."},
	), fault.PhaseMutation, fault.ChangeConfirmed)
}

func configurationTaskRetirementIncompleteFault(err error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"configuration_task_retirement_incomplete",
		"The stale Runtime assistance task could not be retired completely.",
		false,
		err,
		fault.NextAction{Command: "help runtime assist", Reason: "Resume the same exact task so its retained material can finish retirement."},
	), fault.PhaseMutation, fault.ChangePartial)
}

func (s *Service) ArmHomeAdoption(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission) error {
	if s == nil || s.drafts == nil {
		return fmt.Errorf("Configurator service is unavailable")
	}
	if err := submission.Validate(); err != nil || !submission.Draft.NeedsHomeAdoption() {
		return fmt.Errorf("Configurator Home adoption arm is invalid: %w", err)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: "configure", ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, ExpectedImpact: Impact()}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		if s.stager == nil {
			return fmt.Errorf("Configurator publication barrier is unavailable")
		}
		if err := s.stager.BeginConfiguratorPublication(actionContext, submission); err != nil {
			return err
		}
		return s.drafts.ArmHomeAdoption(actionContext, submission)
	})
}

func (s *Service) PendingHomeAdoption(ctx context.Context, projectRoot string) (tobari.ConfiguratorSubmission, bool, error) {
	if s == nil || s.drafts == nil {
		return tobari.ConfiguratorSubmission{}, false, fmt.Errorf("Configurator service is unavailable")
	}
	submission, present, err := s.drafts.PendingHomeAdoption(ctx, projectRoot)
	if err != nil || present {
		return submission, present, err
	}
	if s.stager == nil {
		return tobari.ConfiguratorSubmission{}, false, fmt.Errorf("Configurator publication recovery is unavailable")
	}
	return s.stager.PendingConfiguratorPublicationForProject(ctx, projectRoot)
}

func (s *Service) AdoptHome(ctx context.Context, intent operation.Intent, submission tobari.ConfiguratorSubmission, snapshot tobari.ContextAuthoritySnapshot) error {
	if s == nil || s.drafts == nil {
		return fmt.Errorf("Configurator service is unavailable")
	}
	if err := submission.Validate(); err != nil || snapshot.Validate() != nil {
		return fmt.Errorf("Configurator Home adoption publication is invalid: %w", err)
	}
	request := execution.Request{Intent: intent, ExpectedCommand: "configure", ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, ExpectedImpact: Impact()}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		if s.stager == nil {
			return fmt.Errorf("Configurator publication verifier is unavailable")
		}
		if err := s.stager.BeginConfiguratorPublication(actionContext, submission); err != nil {
			return err
		}
		if err := s.stager.ConfirmConfiguratorPublication(actionContext, submission, snapshot); err != nil {
			return err
		}
		return s.drafts.AdoptHome(actionContext, submission, snapshot, func() error {
			return s.stager.CompleteConfiguratorPublication(actionContext, submission)
		})
	})
}

func validateIntent(intent operation.Intent) error {
	return validateIntentForTask(intent, tobari.ConfiguratorTaskAggregate, "")
}

func validateIntentForTask(intent operation.Intent, task tobari.ConfiguratorTask, targetRuntimeID string) error {
	if intent.Effect != operation.EffectWrite || intent.Impact != Impact() || intent.Target.ParentID != "" {
		return fmt.Errorf("Configurator intent is invalid")
	}
	switch task {
	case tobari.ConfiguratorTaskAggregate:
		if intent.Command != "configure" || intent.Target.Kind != tobari.ProjectConfigurationTargetKind || intent.Target.ID != tobari.ProjectConfigurationTargetID {
			return fmt.Errorf("aggregate Configurator intent is invalid")
		}
	case tobari.ConfiguratorTaskRuntime:
		if intent.Command != "runtime assist" || intent.Target.Kind != tobari.RuntimeReferenceKind || intent.Target.ID != tobari.RuntimeRef(targetRuntimeID) {
			return fmt.Errorf("Runtime assist intent is invalid")
		}
	case tobari.ConfiguratorTaskPolicy:
		if intent.Command != "policy assist" || intent.Target.Kind != tobari.ContextReferenceKind || intent.Target.ID == "" {
			return fmt.Errorf("policy assist intent is invalid")
		}
		if _, err := tobari.ParseContextRef(intent.Target.ID); err != nil {
			return fmt.Errorf("policy assist intent is invalid: %w", err)
		}
	default:
		return fmt.Errorf("Configurator task intent is invalid")
	}
	return intent.Validate()
}

func configuratorRequest(intent operation.Intent, task tobari.ConfiguratorTask, targetRuntimeID string) (execution.Request, error) {
	if err := validateIntentForTask(intent, task, targetRuntimeID); err != nil {
		return execution.Request{}, err
	}
	return execution.Request{Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite, ExpectedTarget: intent.Target, ExpectedImpact: Impact()}, nil
}
