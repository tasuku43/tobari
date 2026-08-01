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
	ResolveImageSelector(context.Context, string) (string, error)
	ReadDevContainer(context.Context, string, string) (tobari.DevContainerConfig, error)
	ClusterUp(context.Context) (tobari.State, error)
	LoadState(context.Context) (tobari.State, bool, error)
	InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error)
	ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error)
	ClusterDenials(context.Context, tobari.State, int) ([]tobari.PolicyDenial, error)
	ReadLearnedPolicyRules(context.Context, tobari.State) ([]tobari.LearnedPolicyRule, error)
	ApplyLearnedPolicyRules(
		context.Context, tobari.State, []tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
	) error
	ApplyPolicy(context.Context, tobari.State) error
	ClusterDown(context.Context, tobari.State, bool) error
	Doctor(context.Context, string) (doctor.Report, error)
}

// clusterUpProgressRuntimePort is an optional extension of RuntimePort. The
// base port remains available to non-interactive callers and older fakes;
// production runtimes use it to keep human progress outside application
// policy and Docker output.
type clusterUpProgressRuntimePort interface {
	ClusterUpWithProgress(context.Context, tobari.ClusterUpProgressSink) (tobari.State, error)
}

// legacyNamedRuntimePort is deliberately outside RuntimePort. The old
// name-bound container lifecycle remains only as a migration diagnostic and
// cannot become an authority for the CWD-owned public commands.
type legacyNamedRuntimePort interface {
	Attach(context.Context, tobari.State, string, string, string) (tobari.State, error)
	InspectTobari(context.Context, tobari.State) ([]tobari.ItemStatus, error)
	Exec(context.Context, tobari.Instance, tobari.ExecRequest, io.Reader, io.Writer, io.Writer) (int, error)
	TobariLogs(context.Context, tobari.Instance, tobari.LogRequest) ([]byte, error)
	Detach(context.Context, tobari.State, tobari.Instance, bool) (tobari.State, error)
}

// ProjectRuntimePort is the CWD-owned lifecycle boundary. It is separate from
// RuntimePort so the shared-cluster and policy use cases can be migrated
// without making their test doubles implement unrelated project operations.
type ProjectRuntimePort interface {
	ResolveProject(context.Context, string) (tobari.ProjectInstance, bool, error)
	ResolveOrCreateProject(context.Context, string) (tobari.ProjectInstance, bool, error)
	ListProjects(context.Context) ([]tobari.ProjectInstance, error)
	ProjectHome(context.Context, tobari.ProjectInstance) (string, error)
	IsTerminal(io.Writer) bool
	EnsureProjectRuntime(context.Context, tobari.State, tobari.ProjectInstance) (tobari.ProjectInstance, error)
	InspectProjectRuntime(context.Context, tobari.ProjectInstance) (tobari.RuntimeDiagnostic, error)
	EnterProjectRuntime(context.Context, tobari.ProjectInstance, string, io.Reader, io.Writer, io.Writer) (int, error)
	InsideProject(context.Context) bool
	DeleteProject(context.Context, tobari.ProjectInstance) error
}

// lifecycleRuntimePort serializes shared-cluster and CWD-owned project
// lifecycle operations. It is intentionally separate from the broader
// RuntimePort so observation and legacy compatibility ports cannot acquire a
// lock they do not need.
type lifecycleRuntimePort interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

type ownedPolicy struct{}

func (ownedPolicy) Check(_ context.Context, intent operation.Intent) error {
	validCurrentDirectory := intent.Target.Kind == tobari.CurrentDirectoryTargetKind &&
		(intent.Target.ID == "" || intent.Target.ID == tobari.CurrentDirectoryTargetID) &&
		(intent.Target.ParentID == "" || intent.Target.ParentID == tobari.CurrentDirectoryTargetID)
	switch intent.Effect {
	case operation.EffectCreate:
		validCluster := intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ParentID == tobari.ClusterTargetID
		if !validCluster && !validCurrentDirectory {
			return fault.New(fault.KindRejected, "mutation_rejected", "cluster creation scope is not owned by Tobari", false)
		}
	case operation.EffectWrite:
		validCluster := intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ID == tobari.ClusterTargetID
		validTobari := intent.Target.Kind == tobari.TargetKind && intent.Target.ID != ""
		validPolicyCandidate := intent.Target.Kind == tobari.PolicyCandidateKind && intent.Target.ID != ""
		validPolicyCompaction := intent.Target.Kind == tobari.PolicyCompactionKind && intent.Target.ID != ""
		if !validCluster && !validTobari && !validPolicyCandidate && !validPolicyCompaction && !validCurrentDirectory {
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

// IsTerminal reports whether the injected writer is an interactive terminal.
// Terminal ownership remains in the runtime adapter; the CLI uses this only
// to decide whether to attach human progress presentation.
func (s *Service) IsTerminal(writer io.Writer) bool {
	if s == nil || portcheck.IsNil(s.runtime) {
		return false
	}
	return s.runtime.IsTerminal(writer)
}

func (s *Service) projectRuntime() (ProjectRuntimePort, error) {
	if err := s.requireRuntime(); err != nil {
		return nil, err
	}
	project, ok := s.runtime.(ProjectRuntimePort)
	if !ok || portcheck.IsNil(project) {
		return nil, fault.New(
			fault.KindInternal, "missing_runtime",
			"CWD-owned Tobari runtime is not configured", false,
		)
	}
	return project, nil
}

func (s *Service) withLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	if err := s.requireRuntime(); err != nil {
		return err
	}
	lifecycle, ok := s.runtime.(lifecycleRuntimePort)
	if !ok || portcheck.IsNil(lifecycle) {
		return fault.New(
			fault.KindInternal, "missing_runtime",
			"Tobari lifecycle lock is not configured", false,
		)
	}
	return lifecycle.WithLifecycleLock(ctx, action)
}

func (s *Service) legacyNamedRuntime() (legacyNamedRuntimePort, error) {
	if err := s.requireRuntime(); err != nil {
		return nil, err
	}
	legacy, ok := s.runtime.(legacyNamedRuntimePort)
	if !ok || portcheck.IsNil(legacy) {
		return nil, fault.New(
			fault.KindRejected, "legacy_named_lifecycle",
			"named Tobari lifecycle is retired; run tobari from the project directory", false,
		)
	}
	return legacy, nil
}

func (s *Service) validateProjectIntent(intent operation.Intent, effect operation.Effect) error {
	if intent.Command == "" {
		return fault.New(fault.KindContract, "invalid_mutation_contract", "project mutation command is missing", false)
	}
	if intent.Effect != effect || intent.Target.Kind != tobari.CurrentDirectoryTargetKind ||
		(intent.Target.ID != "" && intent.Target.ID != tobari.CurrentDirectoryTargetID) ||
		(intent.Target.ParentID != "" && intent.Target.ParentID != tobari.CurrentDirectoryTargetID) {
		return fault.New(fault.KindContract, "invalid_mutation_contract", "project target binding is invalid", false)
	}
	return nil
}

// EnterProject resolves the current directory, ensures the shared runtime and
// project runtime, then attaches the caller's terminal. It never creates a
// project when the caller has no TTY.
func (s *Service) EnterProject(
	ctx context.Context, intent operation.Intent, in io.Reader, out, errOut io.Writer,
) (int, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return 0, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectCreate); err != nil {
		return 0, err
	}
	if project.InsideProject(ctx) {
		return 0, fault.New(
			fault.KindRejected, "already_inside",
			"This process is already inside a Tobari; nested entry is not supported", false,
			fault.NextAction{Command: "exit", Reason: "Leave the current Tobari before entering another session."},
		)
	}
	if !project.IsTerminal(out) {
		return 0, fault.New(
			fault.KindInvalidInput, "tty_required",
			"tobari requires an interactive terminal", false,
			fault.NextAction{Command: "help tobari", Reason: "Run the root command from a terminal."},
		)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return 0, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	var state tobari.State
	var instance tobari.ProjectInstance
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err = s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		var configured bool
		var stateErr error
		state, configured, stateErr = s.runtime.LoadState(lifecycleContext)
		if stateErr != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "shared cluster state could not be read", false, stateErr,
				fault.NextAction{Command: "cluster status", Reason: "Inspect the configured shared cluster."})
		}
		if !configured {
			return fault.New(
				fault.KindUnavailable, "cluster_not_configured",
				"the shared cluster is not configured; run cluster up before entering a Tobari", false,
				fault.NextAction{Command: "cluster up", Reason: "Create the shared Gateway and OPA cluster explicitly."},
			)
		}
		clusterStatus, statusErr := s.runtime.InspectCluster(lifecycleContext, state)
		if statusErr != nil {
			return fault.Wrap(fault.KindUnavailable, "cluster_status_failed", "the shared cluster could not be inspected", false, statusErr,
				fault.NextAction{Command: "cluster status", Reason: "Inspect the shared cluster before entering a Tobari."})
		}
		if !clusterStatus.Running {
			return fault.New(
				fault.KindUnavailable, "cluster_not_ready",
				"the shared cluster is not ready; repair it with an explicit cluster operation", false,
				fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway and OPA cluster explicitly."},
			)
		}
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			resolved, _, actionErr := project.ResolveOrCreateProject(actionContext, cwd)
			if actionErr != nil {
				return classifyProjectMutationError(actionErr, "tobari", "status", "logical state may need reconciliation")
			}
			if resolved.Incomplete {
				return fault.New(
					fault.KindRejected, "project_state_incomplete",
					"the current Tobari has incomplete logical state and cannot be recreated safely", false,
					fault.NextAction{Command: "delete", Reason: "Review the exact delete command and confirm removal of the incomplete current-directory Tobari."},
				)
			}
			instance, actionErr = project.EnsureProjectRuntime(actionContext, state, resolved)
			if actionErr != nil {
				return classifyProjectMutationError(actionErr, "tobari", "status", "inspect the selected project before retrying")
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}
	code, err := project.EnterProjectRuntime(ctx, instance, cwd, in, out, errOut)
	if err != nil {
		return 0, fault.Wrap(fault.KindInternal, "enter_failed", "Tobari session could not be started", false, err,
			fault.NextAction{Command: "status", Reason: "Inspect the selected project's runtime."})
	}
	return code, nil
}

func classifyProjectMutationError(err error, command, recovery, message string) error {
	if _, structured := fault.PublicCopy(err); structured {
		return err
	}
	return fault.Wrap(
		fault.KindUnavailable, "runtime_reconcile_failed", message, false, err,
		fault.NextAction{Command: recovery, Reason: "Inspect the logical project and recoverable runtime state."},
	)
}

// ProjectStatus observes the nearest CWD-owned logical Tobari without
// creating or repairing it.
func (s *Service) ProjectStatus(ctx context.Context) (tobari.ProjectStatus, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectStatus{}, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	instance, found, err := project.ResolveProject(ctx, cwd)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	if !found {
		result := tobari.ProjectStatus{Task: tobari.TaskStatus, Exists: false, Runtime: tobari.RuntimeDiagnosticUnknown}
		return result, result.Validate()
	}
	diagnostic, err := project.InspectProjectRuntime(ctx, instance)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "project runtime status could not be read", false, err)
	}
	home, err := project.ProjectHome(ctx, instance)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, err)
	}
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, Exists: true, Root: instance.Root, ID: instance.ID,
		Home: home, Runtime: diagnostic,
	}
	if err := result.Validate(); err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "project status is invalid", false, err)
	}
	return result, nil
}

// ProjectList observes every locally indexed logical Tobari and its runtime
// diagnostics. It does not create, repair, or delete any entry.
func (s *Service) ProjectList(ctx context.Context) (tobari.ProjectListResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectListResult{}, err
	}
	instances, err := project.ListProjects(ctx)
	if err != nil {
		return tobari.ProjectListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	result := tobari.ProjectListResult{Task: tobari.TaskProjectList, Items: make([]tobari.ProjectListItem, 0, len(instances))}
	for _, instance := range instances {
		diagnostic, diagnosticErr := project.InspectProjectRuntime(ctx, instance)
		if diagnosticErr != nil {
			return tobari.ProjectListResult{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "project runtime status could not be read", false, diagnosticErr)
		}
		home, homeErr := project.ProjectHome(ctx, instance)
		if homeErr != nil {
			return tobari.ProjectListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, homeErr)
		}
		result.Items = append(result.Items, tobari.ProjectListItem{
			Root: instance.Root, ID: instance.ID, Home: home, Runtime: diagnostic,
		})
	}
	if err := result.Validate(); err != nil {
		return tobari.ProjectListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list is invalid", false, err)
	}
	return result, nil
}

// DeleteProject removes only the nearest CWD-owned logical Tobari after the
// caller has completed the explicit destructive confirmation.
func (s *Service) DeleteProject(ctx context.Context, intent operation.Intent, force bool) (tobari.ProjectDeleteResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectWrite); err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	if !force {
		return tobari.ProjectDeleteResult{}, fault.New(
			fault.KindRejected, "confirmation_required",
			"deleting a Tobari requires explicit confirmation; use --force", false,
			fault.NextAction{Command: "delete --force", Reason: "Confirm removal of the current directory's Tobari."},
		)
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.ProjectDeleteResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	var instance tobari.ProjectInstance
	var home string
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err = s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		var found bool
		var resolveErr error
		instance, found, resolveErr = project.ResolveProject(lifecycleContext, cwd)
		if resolveErr != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, resolveErr)
		}
		if !found {
			return fault.New(fault.KindNotFound, "project_not_found", "no Tobari exists for the current directory", false,
				fault.NextAction{Command: "tobari", Reason: "Create a Tobari from the current project directory."})
		}
		home, err = project.ProjectHome(lifecycleContext, instance)
		if err != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, err)
		}
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			if actionErr := project.DeleteProject(actionContext, instance); actionErr != nil {
				return classifyProjectMutationError(actionErr, "delete", "status", "deletion did not complete; retry delete after inspecting status")
			}
			return nil
		})
	})
	if err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	result := tobari.ProjectDeleteResult{Task: tobari.TaskDelete, Deleted: true, Root: instance.Root, ID: instance.ID, Home: home}
	if err := result.Validate(); err != nil {
		return tobari.ProjectDeleteResult{}, fault.Wrap(fault.KindContract, "invalid_delete_contract", "project delete result is invalid", false, err)
	}
	return result, nil
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
		return tobari.ClusterStatus{Task: tobari.TaskClusterStatus, Components: []tobari.ComponentStatus{}}, nil
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
func (s *Service) Attach(
	ctx context.Context, intent operation.Intent, name, root, image, devcontainer string,
) (tobari.Instance, error) {
	legacy, err := s.legacyNamedRuntime()
	if err != nil {
		return tobari.Instance{}, err
	}
	if err := ctx.Err(); err != nil {
		return tobari.Instance{}, err
	}
	if image != "" && devcontainer != "" {
		return tobari.Instance{}, fault.New(
			fault.KindInvalidInput, "invalid_devcontainer",
			"--image and --devcontainer cannot be used together", false,
		)
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_name", "Tobari name is invalid", false, err)
	}
	if image != "" {
		if err := tobari.ValidateImageSelector(image); err != nil {
			return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_image", "Tobari image selector is invalid", false, err)
		}
	}
	resolved, err := s.runtime.ResolveRoot(ctx, root)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return tobari.Instance{}, contextErr
		}
		return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "Tobari root is invalid", false, err)
	}
	if devcontainer != "" {
		config, readErr := s.runtime.ReadDevContainer(ctx, resolved, devcontainer)
		if readErr != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return tobari.Instance{}, contextErr
			}
			return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_devcontainer", "Dev Container configuration is invalid", false, readErr)
		}
		if len(config.UnsupportedProperties()) != 0 {
			return tobari.Instance{}, fault.New(
				fault.KindUnsupported, "unsupported_devcontainer",
				"Dev Container configuration contains runtime properties outside Tobari's image-based subset", false,
			)
		}
		if err := config.Validate(); err != nil {
			return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_devcontainer", "Dev Container configuration is invalid", false, err)
		}
		image = config.Image
	} else {
		image, err = s.runtime.ResolveImageSelector(ctx, image)
		if err != nil {
			if contextErr := ctx.Err(); contextErr != nil {
				return tobari.Instance{}, contextErr
			}
			if _, structured := fault.PublicCopy(err); structured {
				return tobari.Instance{}, err
			}
			return tobari.Instance{}, fault.Wrap(fault.KindRejected, "invalid_image_config", "default Tobari image configuration is invalid", false, err)
		}
	}
	if err := tobari.ValidateImageSelector(image); err != nil {
		return tobari.Instance{}, fault.Wrap(fault.KindInvalidInput, "invalid_image", "Tobari image selector is invalid", false, err)
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
				if existing.ImageSelector() == image {
					existing.Image = existing.ImageSelector()
					return existing, nil
				}
				return tobari.Instance{}, fault.New(fault.KindInvalidInput, "image_conflict", "Tobari name and root are already attached with another image", false)
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
		created, actionErr := legacy.Attach(actionContext, state, name, resolved, image)
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
	legacy, err := s.legacyNamedRuntime()
	if err != nil {
		return tobari.ListResult{}, err
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.ListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err)
	}
	result := tobari.ListResult{Task: tobari.TaskList, Items: []tobari.ItemStatus{}}
	if exists {
		result.Items, err = legacy.InspectTobari(ctx, state)
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
	legacy, err := s.legacyNamedRuntime()
	if err != nil {
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
	code, err := legacy.Exec(ctx, instance, request, in, out, errOut)
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
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.DenialReport{}, fault.Wrap(
			fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err,
		)
	}
	if !exists {
		return tobari.DenialReport{}, fault.New(
			fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false,
		)
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

func (s *Service) policyCandidates(
	ctx context.Context, tail int, task string,
) (tobari.PolicyCandidateReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyCandidateReport{}, err
	}
	request := tobari.LogRequest{Component: "gateway", Tail: tail}
	if err := request.ValidateCluster(); err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_candidate_request",
			"policy candidate request is invalid", false, err,
		)
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err,
		)
	}
	if !exists {
		return tobari.PolicyCandidateReport{}, fault.New(
			fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false,
		)
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, tail)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	rules, err := s.runtime.ReadLearnedPolicyRules(ctx, state)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"learned policy data could not be read safely", false, err,
		)
	}
	items, err := tobari.PolicyCandidates(denials, rules)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidates are invalid", false, err,
		)
	}
	result := tobari.PolicyCandidateReport{
		Task: task, PolicyDirectory: state.PolicyDirectory, WindowLines: tail, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate result is invalid", false, err,
		)
	}
	return result, nil
}

// PolicyCandidates discovers pending exact-rule proposals from retained denials.
func (s *Service) PolicyCandidates(
	ctx context.Context, tail int,
) (tobari.PolicyCandidateReport, error) {
	return s.policyCandidates(ctx, tail, tobari.TaskPolicyCandidates)
}

// PolicyTail returns the same queue with a distinct human-review task identity.
func (s *Service) PolicyTail(
	ctx context.Context, tail int,
) (tobari.PolicyCandidateReport, error) {
	return s.policyCandidates(ctx, tail, tobari.TaskPolicyTail)
}

func (s *Service) loadPolicyState(
	ctx context.Context,
) (tobari.State, []tobari.LearnedPolicyRule, error) {
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.State{}, nil, fault.Wrap(
			fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err,
		)
	}
	if !exists {
		return tobari.State{}, nil, fault.New(
			fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false,
		)
	}
	rules, err := s.runtime.ReadLearnedPolicyRules(ctx, state)
	if err != nil {
		return tobari.State{}, nil, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"learned policy data could not be read safely", false, err,
		)
	}
	return state, rules, nil
}

func validatePolicyMutationTarget(intent operation.Intent, kind, id string) error {
	if intent.Target.Kind != kind || intent.Target.ID != id {
		return fault.New(
			fault.KindContract, "invalid_mutation_contract",
			"policy mutation target does not match the consumed opaque ID", false,
		)
	}
	return nil
}

func (s *Service) applyLearnedRules(
	ctx context.Context, intent operation.Intent, expectedCommand string,
	state tobari.State, expected, updated []tobari.LearnedPolicyRule,
) error {
	request := execution.Request{
		Intent: intent, ExpectedCommand: expectedCommand, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	return s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		actionErr := s.runtime.ApplyLearnedPolicyRules(actionContext, state, expected, updated)
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"learned policy activation did not complete; inspect cluster status", false, actionErr,
			fault.NextAction{
				Command: "cluster status",
				Reason:  "Reconcile OPA health and the current policy before another mutation.",
			},
		)
	})
}

// AllowPolicyCandidate records and activates one exact retained denial.
func (s *Service) AllowPolicyCandidate(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyLearningChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	if err := tobari.ValidatePolicyCandidateID(id); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_candidate_id",
			"policy candidate ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyCandidateKind, id); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidates(denials, rules)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidates are invalid", false, err,
		)
	}
	var candidate tobari.PolicyCandidate
	found := false
	for _, item := range candidates {
		if item.ID == id {
			candidate, found = item, true
			break
		}
	}
	if !found {
		return tobari.PolicyLearningChange{}, fault.New(
			fault.KindInvalidInput, "policy_candidate_not_found",
			"policy candidate is stale, already covered, or outside retained logs", false,
		)
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate cannot become an exact rule", false, err,
		)
	}
	updated := append(append([]tobari.LearnedPolicyRule{}, rules...), rule)
	if err := tobari.ValidateLearnedPolicyRules(updated); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_learned_policy",
			"exact learned policy is invalid", false, err,
		)
	}
	if err := s.applyLearnedRules(ctx, intent, "policy allow", state, rules, updated); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	result := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: state.PolicyDirectory,
		TargetID: id, Rule: rule, SourceRuleCount: 1, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_learning_result",
			"policy allow result is invalid", false, err,
		)
	}
	return result, nil
}

// PolicyCompactions discovers every current bounded exact-to-prefix proposal.
func (s *Service) PolicyCompactions(
	ctx context.Context,
) (tobari.PolicyCompactionReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyCompactionReport{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyCompactionReport{}, err
	}
	items, err := tobari.PolicyCompactions(rules)
	if err != nil {
		return tobari.PolicyCompactionReport{}, fault.Wrap(
			fault.KindContract, "invalid_compaction_contract",
			"policy compactions are invalid", false, err,
		)
	}
	result := tobari.PolicyCompactionReport{
		Task: tobari.TaskPolicyCompactions, PolicyDirectory: state.PolicyDirectory, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyCompactionReport{}, fault.Wrap(
			fault.KindContract, "invalid_compaction_contract",
			"policy compaction result is invalid", false, err,
		)
	}
	return result, nil
}

// CompactPolicy records and activates one current exact-rule compaction.
func (s *Service) CompactPolicy(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyLearningChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	if err := tobari.ValidatePolicyCompactionID(id); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_compaction_id",
			"policy compaction ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyCompactionKind, id); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	updated, selected, rule, err := tobari.CompactLearnedPolicyRules(rules, id)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInvalidInput, "policy_compaction_not_found",
			"policy compaction is stale or no longer safe", false, err,
		)
	}
	if err := s.applyLearnedRules(ctx, intent, "policy compact", state, rules, updated); err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	result := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyCompact, PolicyDirectory: state.PolicyDirectory,
		TargetID: id, Rule: rule, SourceRuleCount: len(selected.SourceRuleIDs), Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_learning_result",
			"policy compact result is invalid", false, err,
		)
	}
	return result, nil
}

// ApplyPolicy tests the trusted-host policy and activates it in the exact owned
// OPA component.
func (s *Service) ApplyPolicy(
	ctx context.Context, intent operation.Intent,
) (tobari.PolicyActivation, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyActivation{}, err
	}
	state, exists, err := s.runtime.LoadState(ctx)
	if err != nil {
		return tobari.PolicyActivation{}, fault.Wrap(
			fault.KindInternal, "state_read_failed", "Tobari state could not be read", false, err,
		)
	}
	if !exists {
		return tobari.PolicyActivation{}, fault.New(
			fault.KindUnavailable, "cluster_not_running", "cluster is not configured", false,
		)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "policy apply", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		actionErr := s.runtime.ApplyPolicy(actionContext, state)
		if actionErr == nil {
			return nil
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "policy_apply_failed",
			"Policy activation did not complete; inspect cluster status", false, actionErr,
			fault.NextAction{
				Command: "cluster status",
				Reason:  "Reconcile OPA health before applying policy again.",
			},
		)
	})
	if err != nil {
		return tobari.PolicyActivation{}, err
	}
	result := tobari.PolicyActivation{
		Task: tobari.TaskPolicyApply, PolicyDirectory: state.PolicyDirectory, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyActivation{}, fault.Wrap(
			fault.KindContract, "invalid_policy_activation",
			"policy activation result is invalid", false, err,
		)
	}
	return result, nil
}

// TobariLogs returns a bounded log window for one exact Tobari.
func (s *Service) TobariLogs(ctx context.Context, id string, tail int) ([]byte, error) {
	legacy, err := s.legacyNamedRuntime()
	if err != nil {
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
	output, err := legacy.TobariLogs(ctx, instance, request)
	if err != nil {
		return nil, fault.Wrap(fault.KindInternal, "logs_failed", "Tobari logs could not be read", false, err)
	}
	return output, nil
}

// Detach removes one exact referenced Tobari.
func (s *Service) Detach(ctx context.Context, intent operation.Intent, id string, purge bool) error {
	legacy, err := s.legacyNamedRuntime()
	if err != nil {
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
		_, actionErr := legacy.Detach(actionContext, state, instance, purge)
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
					fault.KindRejected, "cluster_not_empty", "delete every CWD-owned Tobari before removing the cluster", false,
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
		if len(state.Tobari) != 0 {
			return fault.New(
				fault.KindRejected, "legacy_named_state",
				"the shared state contains legacy named Tobari records; remove them with the older binary before continuing", false,
			)
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
