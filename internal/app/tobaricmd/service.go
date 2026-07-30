// Package tobaricmd owns shared-cluster and named-Tobari use cases.
package tobaricmd

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// RuntimePort is the narrow Docker/filesystem boundary required by Tobari tasks.
type RuntimePort interface {
	ResolveRoot(context.Context, string) (string, error)
	CurrentDirectory(context.Context) (string, error)
	IsTerminal(io.Writer) bool
	ClusterUp(context.Context) (tobari.State, error)
	LoadState(context.Context) (tobari.State, bool, error)
	InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error)
	Attach(context.Context, tobari.State, string, string) (tobari.State, error)
	InspectTobari(context.Context, tobari.State) ([]tobari.ItemStatus, error)
	Exec(context.Context, tobari.Instance, tobari.ExecRequest, io.Reader, io.Writer, io.Writer) (int, error)
	ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error)
	TobariLogs(context.Context, tobari.Instance, tobari.LogRequest) ([]byte, error)
	Detach(context.Context, tobari.State, tobari.Instance, bool) (tobari.State, error)
	ClusterDown(context.Context, tobari.State, bool) error
	Doctor(context.Context, string) (doctor.Report, error)
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch intent.Effect {
	case operation.EffectCreate:
		if intent.Target.Kind != tobari.ClusterTargetKind || intent.Target.ParentID != tobari.ClusterTargetID {
			return fault.New(fault.KindRejected, "mutation_rejected", "cluster creation scope is not owned by Tobari", false)
		}
	case operation.EffectWrite:
		validCluster := intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ID == tobari.ClusterTargetID
		validTobari := intent.Target.Kind == tobari.TargetKind && intent.Target.ID != ""
		if !validCluster && !validTobari {
			return fault.New(fault.KindRejected, "mutation_rejected", "mutation target is not owned by Tobari", false)
		}
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "mutation effect is not supported", false)
	}
	return nil
}

// Service coordinates validated tasks without depending on Docker.
type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{})}
}

func (s *Service) requireRuntime() error {
	if s == nil || portcheck.IsNil(s.runtime) {
		return fault.New(fault.KindInternal, "missing_runtime", "Tobari runtime is not configured", false)
	}
	return nil
}

// ClusterUp creates or reconciles the shared enforcement cluster.
func (s *Service) ClusterUp(ctx context.Context, intent operation.Intent) (tobari.ClusterStatus, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	var state tobari.State
	request := execution.Request{
		Intent: intent, ExpectedCommand: "cluster up", ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, actionErr := s.runtime.ClusterUp(actionContext)
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
	if err != nil {
		return tobari.ClusterStatus{}, err
	}
	status, err := s.runtime.InspectCluster(ctx, state)
	if err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "status_failed", "cluster started but status could not be read", false, err)
	}
	status.Task = tobari.TaskClusterUp
	if err := status.Validate(); err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "cluster status is invalid", false, err)
	}
	return status, nil
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
		return tobari.ClusterStatus{Task: tobari.TaskClusterStatus, Components: []tobari.ComponentStatus{}}, nil
	}
	status, err := s.runtime.InspectCluster(ctx, state)
	if err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "status_failed", "cluster status could not be read", false, err)
	}
	status.Task = tobari.TaskClusterStatus
	if err := status.Validate(); err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "cluster status is invalid", false, err)
	}
	return status, nil
}

// Attach creates one named Tobari within the shared cluster.
func (s *Service) Attach(ctx context.Context, intent operation.Intent, name, root string) (tobari.Instance, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.Instance{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_name", "Tobari name is invalid", false, err)
	}
	resolved, err := s.runtime.ResolveRoot(ctx, root)
	if err != nil {
		return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "Tobari root is invalid", false, err)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.Instance{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	if !exists {
		return tobari.Instance{}, fault.New(fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false)
	}
	for _, existing := range state.Tobari {
		if existing.Name == name {
			if existing.Root == resolved {
				return existing, nil
			}
			return tobari.Instance{}, fault.New(fault.KindInvalidInput, "name_conflict", "Tobari name is already attached to another root", false)
		}
		if existing.Root == resolved {
			return tobari.Instance{}, fault.New(fault.KindInvalidInput, "root_conflict", "root is already attached to another Tobari", false)
		}
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "attach", ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	var updated tobari.State
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, actionErr := s.runtime.Attach(actionContext, state, name, resolved)
		updated = created
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "attach_failed",
			"Tobari attachment did not complete; inspect list before retrying", false, actionErr,
			fault.NextAction{Command: "list", Reason: "Reconcile partial Docker state before another attachment."},
		)
	})
	if err != nil {
		return tobari.Instance{}, err
	}
	for _, instance := range updated.Tobari {
		if instance.Name == name {
			return instance, nil
		}
	}
	return tobari.Instance{}, fault.New(fault.KindContract, "invalid_attach_contract", "attached Tobari is absent from confirmed state", false)
}

// List returns every configured Tobari in the exact local scope.
func (s *Service) List(ctx context.Context) (tobari.ListResult, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ListResult{}, err
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.ListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	result := tobari.ListResult{Task: tobari.TaskList, Items: []tobari.ItemStatus{}}
	if exists {
		result.Items, err = s.runtime.InspectTobari(ctx, state)
		if err != nil {
			return tobari.ListResult{}, fault.Wrap(fault.KindInternal, "list_failed", "Tobari list could not be observed", false, err)
		}
	}
	if err := result.Validate(); err != nil {
		return tobari.ListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "Tobari list is invalid", false, err)
	}
	return result, nil
}

func (s *Service) loadInstance(ctx context.Context, id string) (tobari.State, tobari.Instance, error) {
	if err := tobari.ValidateID(id); err != nil {
		return tobari.State{}, tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_tobari_id", "Tobari ID is invalid", false, err)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.State{}, tobari.Instance{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	if !exists {
		return tobari.State{}, tobari.Instance{}, fault.New(fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false)
	}
	instance, found := state.Find(id)
	if !found {
		return tobari.State{}, tobari.Instance{}, fault.New(fault.KindInvalidInput, "tobari_not_found", "Tobari ID is not configured", false)
	}
	return state, instance, nil
}

// Exec runs exact argv inside one opaque-ID-bound Tobari.
func (s *Service) Exec(
	ctx context.Context, id string, request tobari.ExecRequest,
	in io.Reader, out, errOut io.Writer,
) (int, error) {
	if err := s.requireRuntime(); err != nil {
		return 0, err
	}
	if err := request.Validate(); err != nil {
		return 0, fault.Wrap(fault.KindInvalidInput, "invalid_exec_request", "Tobari command is invalid", false, err)
	}
	_, instance, err := s.loadInstance(ctx, id)
	if err != nil {
		return 0, err
	}
	if request.HostCWD == "" && request.Interactive {
		current, currentErr := s.runtime.CurrentDirectory(ctx)
		if currentErr == nil {
			request.HostCWD = current
		}
	}
	request.TTY = request.TTY && s.runtime.IsTerminal(out)
	code, err := s.runtime.Exec(ctx, instance, request, in, out, errOut)
	if err != nil {
		return 0, fault.Wrap(fault.KindInternal, "exec_failed", "Tobari command could not be started", false, err)
	}
	return code, nil
}

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

// TobariLogs returns a bounded log window for one exact Tobari.
func (s *Service) TobariLogs(ctx context.Context, id string, tail int) ([]byte, error) {
	if err := s.requireRuntime(); err != nil {
		return nil, err
	}
	request := tobari.LogRequest{Component: "tobari", Tail: tail}
	if err := request.ValidateTobari(); err != nil {
		return nil, fault.Wrap(fault.KindInvalidInput, "invalid_log_request", "Tobari log request is invalid", false, err)
	}
	_, instance, err := s.loadInstance(ctx, id)
	if err != nil {
		return nil, err
	}
	output, err := s.runtime.TobariLogs(ctx, instance, request)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "logs_failed", "Tobari logs could not be read", false, err)
	}
	return output, nil
}

// Detach removes one exact referenced Tobari.
func (s *Service) Detach(ctx context.Context, intent operation.Intent, id string, purge bool) error {
	if err := s.requireRuntime(); err != nil {
		return err
	}
	state, instance, err := s.loadInstance(ctx, id)
	if err != nil {
		return err
	}
	if intent.Target.ID != id {
		return fault.New(fault.KindContract, "invalid_mutation_contract", "detach target does not match the consumed Tobari ID", false)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "detach", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		_, actionErr := s.runtime.Detach(actionContext, state, instance, purge)
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "detach_failed",
			"Tobari detachment did not complete; inspect list before retrying", false, actionErr,
			fault.NextAction{Command: "list", Reason: "Reconcile remaining Docker state before another detachment."},
		)
	})
}

// ClusterDown removes shared resources only after every Tobari is detached.
func (s *Service) ClusterDown(ctx context.Context, intent operation.Intent, purge bool) (tobari.ClusterStatus, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.ClusterStatus{}, err
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.ClusterStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	if !exists {
		return tobari.ClusterStatus{Task: tobari.TaskClusterDown, Components: []tobari.ComponentStatus{}}, nil
	}
	if len(state.Tobari) != 0 {
		return tobari.ClusterStatus{}, fault.New(fault.KindRejected, "cluster_not_empty", "detach every Tobari before removing the cluster", false)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "cluster down", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
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
	if err != nil {
		return tobari.ClusterStatus{}, err
	}
	return tobari.ClusterStatus{Task: tobari.TaskClusterDown, Components: []tobari.ComponentStatus{}}, nil
}

func (s *Service) Doctor(ctx context.Context, root string) (doctor.Report, error) {
	if err := s.requireRuntime(); err != nil {
		return doctor.Report{}, err
	}
	report, err := s.runtime.Doctor(ctx, root)
	if err != nil {
		return doctor.Report{}, fault.Wrap(fault.KindInternal, "doctor_failed", "Tobari diagnostics could not run", false, err)
	}
	if err := report.Validate(); err != nil {
		return doctor.Report{}, fault.Wrap(fault.KindContract, "invalid_doctor_contract", "Tobari diagnostic report is invalid", false, err)
	}
	return report, nil
}
