// Package realmcmd owns Tobari's single-realm lifecycle use cases.
package realmcmd

import (
	"context"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

// RuntimePort is the narrow Docker/filesystem boundary required by the realm tasks.
type RuntimePort interface {
	ResolveRoot(context.Context, string) (string, error)
	CurrentDirectory(context.Context) (string, error)
	IsTerminal(io.Writer) bool
	Up(context.Context, string) (realm.State, error)
	LoadState(context.Context) (realm.State, bool, error)
	Inspect(context.Context, realm.State) (realm.Status, error)
	Exec(context.Context, realm.State, realm.ExecRequest, io.Reader, io.Writer, io.Writer) (int, error)
	Logs(context.Context, realm.State, realm.LogRequest) ([]byte, error)
	Down(context.Context, realm.State, bool) error
	Doctor(context.Context, string) (doctor.Report, error)
}

type ownedRealmPolicy struct{}

func (ownedRealmPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Target.Kind != realm.TargetKind {
		return fault.New(fault.KindRejected, "mutation_rejected", "realm target kind is not owned by Tobari", false)
	}
	if intent.Effect == operation.EffectCreate && intent.Target.ParentID != realm.TargetID {
		return fault.New(fault.KindRejected, "mutation_rejected", "realm creation scope is not owned by Tobari", false)
	}
	if intent.Effect == operation.EffectWrite && intent.Target.ID != realm.TargetID {
		return fault.New(fault.KindRejected, "mutation_rejected", "realm target is not owned by Tobari", false)
	}
	return nil
}

// Service coordinates validated tasks without depending on Docker.
type Service struct {
	runtime RuntimePort
	mutator *execution.Invoker
}

// New creates the single-realm use case service.
func New(runtime RuntimePort) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedRealmPolicy{})}
}

// Up reconciles the one realm for root.
func (s *Service) Up(ctx context.Context, intent operation.Intent, root string) (realm.Status, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return realm.Status{}, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
	}
	resolved, err := s.runtime.ResolveRoot(ctx, root)
	if err != nil {
		return realm.Status{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "realm root is invalid", false, err)
	}
	var state realm.State
	request := execution.Request{
		Intent: intent, ExpectedCommand: "up", ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	if err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, actionErr := s.runtime.Up(actionContext, resolved)
		state = created
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable,
			"realm_start_failed",
			"Realm startup did not complete; inspect status before retrying",
			false,
			actionErr,
			fault.NextAction{Command: "status", Reason: "Reconcile partial Docker state before another startup."},
		)
	}); err != nil {
		return realm.Status{}, err
	}
	status, err := s.runtime.Inspect(ctx, state)
	if err != nil {
		return realm.Status{}, fault.Wrap(fault.KindInternal, "status_failed", "realm started but status could not be read", false, err)
	}
	status.Task = realm.TaskUp
	if err := status.Validate(); err != nil {
		return realm.Status{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "realm status is invalid", false, err)
	}
	return status, nil
}

// Status observes the one realm without repairing it.
func (s *Service) Status(ctx context.Context) (realm.Status, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return realm.Status{}, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return realm.Status{}, fault.Wrap(fault.KindInternal, "state_read_failed", "realm state could not be read", false, err)
	}
	if !exists {
		return realm.Status{Task: realm.TaskStatus, Components: []realm.ComponentStatus{}}, nil
	}
	status, err := s.runtime.Inspect(ctx, state)
	if err != nil {
		return realm.Status{}, fault.Wrap(fault.KindInternal, "status_failed", "realm status could not be read", false, err)
	}
	status.Task = realm.TaskStatus
	if err := status.Validate(); err != nil {
		return realm.Status{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "realm status is invalid", false, err)
	}
	return status, nil
}

// Exec runs one argv inside the configured Realm and returns the child status.
func (s *Service) Exec(
	ctx context.Context,
	request realm.ExecRequest,
	in io.Reader,
	out, errOut io.Writer,
) (int, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return 0, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
	}
	if err := request.Validate(); err != nil {
		return 0, fault.Wrap(fault.KindInvalidInput, "invalid_exec_request", "realm command is invalid", false, err)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return 0, fault.Wrap(fault.KindInternal, "state_read_failed", "realm state could not be read", false, err)
	}
	if !exists {
		return 0, fault.New(fault.KindUnavailable, "realm_not_running", "realm is not configured", false)
	}
	if request.HostCWD == "" && request.Interactive {
		current, currentErr := s.runtime.CurrentDirectory(ctx)
		if currentErr == nil {
			request.HostCWD = current
		}
	}
	request.TTY = request.TTY && s.runtime.IsTerminal(out)
	code, err := s.runtime.Exec(ctx, state, request, in, out, errOut)
	if err != nil {
		return 0, fault.Wrap(fault.KindInternal, "exec_failed", "realm command could not be started", false, err)
	}
	return code, nil
}

// Logs returns a bounded redacted component-log window.
func (s *Service) Logs(ctx context.Context, request realm.LogRequest) ([]byte, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return nil, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
	}
	if err := request.Validate(); err != nil {
		return nil, fault.Wrap(fault.KindInvalidInput, "invalid_log_request", "log request is invalid", false, err)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "state_read_failed", "realm state could not be read", false, err)
	}
	if !exists {
		return nil, fault.New(fault.KindUnavailable, "realm_not_running", "realm is not configured", false)
	}
	output, err := s.runtime.Logs(ctx, state, request)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "logs_failed", "realm logs could not be read", false, err)
	}
	return output, nil
}

// Down removes transient owned resources and optionally the persistent home.
func (s *Service) Down(ctx context.Context, intent operation.Intent, purge bool) (realm.Status, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return realm.Status{}, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return realm.Status{}, fault.Wrap(fault.KindInternal, "state_read_failed", "realm state could not be read", false, err)
	}
	if !exists {
		return realm.Status{Task: realm.TaskDown, Components: []realm.ComponentStatus{}}, nil
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "down", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	if err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		actionErr := s.runtime.Down(actionContext, state, purge)
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable,
			"realm_stop_failed",
			"Realm cleanup did not complete; inspect status before retrying",
			false,
			actionErr,
			fault.NextAction{Command: "status", Reason: "Reconcile remaining Docker state before another cleanup."},
		)
	}); err != nil {
		return realm.Status{}, err
	}
	return realm.Status{Task: realm.TaskDown, Components: []realm.ComponentStatus{}}, nil
}

// Doctor runs bounded host and policy diagnostics.
func (s *Service) Doctor(ctx context.Context, root string) (doctor.Report, error) {
	if s == nil || portcheck.IsNil(s.runtime) {
		return doctor.Report{}, fault.New(fault.KindInternal, "missing_runtime", "realm runtime is not configured", false)
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
