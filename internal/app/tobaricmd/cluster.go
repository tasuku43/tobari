package tobaricmd

import (
	"context"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// clusterUpProgressRuntimePort is an optional extension of RuntimePort.
// Production runtimes use it to keep human progress outside application
// policy and Docker output.
type clusterUpProgressRuntimePort interface {
	ClusterUpWithProgress(context.Context, tobari.ClusterUpProgressSink) (tobari.State, error)
}

func (s *Service) readyCluster(ctx context.Context) (tobari.State, error) {
	state, configured, stateErr := s.runtime.LoadState(ctx)
	if stateErr != nil {
		return tobari.State{}, fault.Wrap(fault.KindInternal, "state_read_failed", "shared cluster state could not be read", false, stateErr,
			fault.NextAction{Command: "cluster status", Reason: "Inspect the configured shared cluster."})
	}
	if !configured {
		return tobari.State{}, fault.New(
			fault.KindUnavailable, "cluster_not_configured",
			"the shared cluster is not configured; run cluster up before entering a Tobari", false,
			fault.NextAction{Command: "cluster up", Reason: "Create the shared Gateway, OPA, and Auth Broker cluster explicitly."},
		)
	}
	clusterStatus, statusErr := s.runtime.InspectCluster(ctx, state)
	if statusErr != nil {
		return tobari.State{}, fault.Wrap(fault.KindUnavailable, "cluster_status_failed", "the shared cluster could not be inspected", false, statusErr,
			fault.NextAction{Command: "cluster status", Reason: "Inspect the shared cluster before entering a Tobari."})
	}
	if !clusterStatus.Running || clusterStatus.PolicyProjection != "valid" ||
		clusterStatus.PrincipalRegistry != "valid" || clusterStatus.GatewayProjection != "valid" {
		return tobari.State{}, fault.New(
			fault.KindUnavailable, "cluster_not_ready",
			"the shared cluster is not ready or its projection is stale; repair it with an explicit cluster operation", false,
			fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster explicitly."},
		)
	}
	return state, nil
}

// ClusterUp creates or reconciles the shared enforcement cluster.
func (s *Service) ClusterUp(ctx context.Context, intent operation.Intent) (tobari.ClusterStatus, error) {
	return s.clusterUp(ctx, intent, nil)
}

// ClusterUpWithProgress creates or reconciles the shared enforcement cluster
// and forwards bounded startup signals to an optional human presentation sink.
// The sink cannot affect mutation policy, runtime calls, or the returned
// status.
func (s *Service) ClusterUpWithProgress(
	ctx context.Context, intent operation.Intent, progress tobari.ClusterUpProgressSink,
) (tobari.ClusterStatus, error) {
	return s.clusterUp(ctx, intent, progress)
}

func (s *Service) clusterUp(
	ctx context.Context, intent operation.Intent, progress tobari.ClusterUpProgressSink,
) (tobari.ClusterStatus, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	if err := s.runtime.ValidateClusterBuildIdentity(ctx); err != nil {
		return tobari.ClusterStatus{}, err
	}
	var state tobari.State
	request := execution.Request{
		Intent: intent, ExpectedCommand: "cluster up", ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err := s.withLifecycleLock(ctx, func(actionContext context.Context) error {
		return s.mutator.Invoke(actionContext, request, func(actionContext context.Context, _ operation.Intent) error {
			created, actionErr := s.clusterUpRuntime(actionContext, progress)
			state = created
			if actionErr == nil {
				return nil
			}
			if _, structured := fault.PublicCopy(actionErr); structured {
				return actionErr
			}
			return fault.Wrap(
				fault.KindUnavailable, "cluster_start_failed",
				"Cluster startup did not complete; inspect status before retrying", false, actionErr,
				fault.NextAction{Command: "cluster status", Reason: "Reconcile partial Docker state before another startup."},
			)
		})
	})
	if err != nil {
		return tobari.ClusterStatus{}, err
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressStarted,
	})
	status, err := s.runtime.InspectCluster(ctx, state)
	if err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressFailed,
		})
		if structured, ok := fault.PublicCopy(err); ok {
			return tobari.ClusterStatus{}, structured
		}
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "status_failed", "cluster started but status could not be read", false, err)
	}
	status.Task = tobari.TaskClusterUp
	if err := status.Validate(); err != nil {
		emitClusterUpProgress(progress, tobari.ClusterUpProgress{
			Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressFailed,
		})
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "cluster status is invalid", false, err)
	}
	emitClusterUpProgress(progress, tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressCompleted,
	})
	return status, nil
}

func (s *Service) clusterUpRuntime(
	ctx context.Context, progress tobari.ClusterUpProgressSink,
) (tobari.State, error) {
	if runtime, ok := s.runtime.(clusterUpProgressRuntimePort); ok {
		return runtime.ClusterUpWithProgress(ctx, progress)
	}
	return s.runtime.ClusterUp(ctx)
}

func emitClusterUpProgress(
	progress tobari.ClusterUpProgressSink, event tobari.ClusterUpProgress,
) {
	if progress == nil {
		return
	}
	if err := event.Validate(); err != nil {
		return
	}
	progress(event)
}

// ClusterStatus observes shared enforcement without repairing it.
func (s *Service) ClusterStatus(ctx context.Context) (tobari.ClusterStatus, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	if !exists {
		return tobari.UnconfiguredClusterStatus(tobari.TaskClusterStatus), nil
	}
	status, err := s.runtime.InspectCluster(ctx, state)
	if err != nil {
		if structured, ok := fault.PublicCopy(err); ok {
			return tobari.ClusterStatus{}, structured
		}
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "status_failed", "cluster status could not be read", false, err)
	}
	status.Task = tobari.TaskClusterStatus
	if err := status.Validate(); err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "cluster status is invalid", false, err)
	}
	return status, nil
}

// Attach creates one named Tobari within the shared cluster.

// List returns every configured Tobari in the exact local scope.

// Exec runs exact argv inside one opaque-ID-bound Tobari.

// ClusterLogs returns a bounded shared log window.
func (s *Service) ClusterLogs(ctx context.Context, request tobari.LogRequest) ([]byte, error) {
	if err := s.requireRuntime(); err != nil {
		return nil, err
	}
	if err := request.ValidateCluster(); err != nil {
		return nil, fault.Wrap(fault.KindInvalidInput, "invalid_log_request", "cluster log request is invalid", false, err)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	if !exists {
		return nil, fault.New(fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false)
	}
	output, err := s.runtime.ClusterLogs(ctx, state, request)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "logs_failed", "cluster logs could not be read", false, err)
	}
	return output, nil
}

// ClusterDenials returns one typed bounded window of policy-learning evidence.
func (s *Service) ClusterDenials(ctx context.Context, tail int) (tobari.DenialReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.DenialReport{}, err
	}
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return tobari.DenialReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_denial_request", "denial request is invalid", false, err,
		)
	}
	state, err := s.readyCluster(ctx)
	if err != nil {
		return tobari.DenialReport{}, err
	}
	items, err := s.runtime.ClusterDenials(ctx, state, tail)
	if err != nil {
		return tobari.DenialReport{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	result := tobari.DenialReport{
		Task: tobari.TaskClusterDenials, PolicyDirectory: state.PolicyDirectory,
		WindowLines: tail, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.DenialReport{}, fault.Wrap(
			fault.KindContract, "invalid_denial_contract", "cluster denial result is invalid", false, err,
		)
	}
	return result, nil
}

// ClusterDown removes shared resources only after every logical Workspace is deleted.
func (s *Service) ClusterDown(ctx context.Context, intent operation.Intent, purge bool) (tobari.ClusterStatus, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	var state tobari.State
	var exists bool
	request := execution.Request{
		Intent: intent, ExpectedCommand: "cluster down", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err := s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		if project, ok := s.runtime.(ProjectRuntimePort); ok && !portcheck.IsNil(project) {
			projects, projectErr := project.ListProjects(lifecycleContext)
			if projectErr != nil {
				return fault.Wrap(
					fault.KindInternal, "state_read_failed", "CWD-owned Tobari state could not be read", false, projectErr,
				)
			}
			if len(projects) != 0 {
				return fault.New(
					fault.KindRejected, "cluster_not_empty", "delete every logical Workspace before removing the cluster", false,
				)
			}
		}
		var loadErr error
		state, exists, loadErr = s.runtime.LoadState(lifecycleContext)
		if loadErr != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, loadErr)
		}
		if !exists {
			return nil
		}
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			actionErr := s.runtime.ClusterDown(actionContext, state, purge)
			if actionErr == nil {
				return nil
			}
			if _, structured := fault.PublicCopy(actionErr); structured {
				return actionErr
			}
			return fault.Wrap(
				fault.KindUnavailable, "cluster_stop_failed",
				"Cluster cleanup did not complete; inspect status before retrying", false, actionErr,
				fault.NextAction{Command: "cluster status", Reason: "Reconcile remaining Docker state before another cleanup."},
			)
		})
	})
	if err != nil {
		return tobari.ClusterStatus{}, err
	}
	return tobari.UnconfiguredClusterStatus(tobari.TaskClusterDown), nil
}
