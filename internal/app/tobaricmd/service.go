// Package tobaricmd owns shared-cluster and named-Tobari use cases.
package tobaricmd

import (
	"context"
	"errors"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// RuntimePort is the narrow Docker/filesystem boundary required by Tobari tasks.
type RuntimePort interface {
	CurrentDirectory(context.Context) (string, error)
	IsTerminal(io.Writer) bool
	ValidateClusterBuildIdentity(context.Context) error
	ClusterUp(context.Context) (tobari.State, error)
	LoadState(context.Context) (tobari.State, bool, error)
	InspectCluster(context.Context, tobari.State) (tobari.ClusterStatus, error)
	ClusterLogs(context.Context, tobari.State, tobari.LogRequest) ([]byte, error)
	ClusterDenials(context.Context, tobari.State, int) ([]tobari.PolicyDenial, error)
	ReadLearnedPolicyRules(context.Context, tobari.State) ([]tobari.LearnedPolicyRule, error)
	ReadPolicyDenyRules(context.Context, tobari.State) (tobari.PolicyDenyRuleSet, error)
	ApplyLearnedPolicyRules(
		context.Context, tobari.State, []tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
	) (tobari.PolicyActivationReceipt, error)
	ApplyPolicyDenyRules(
		context.Context, tobari.State, []tobari.LearnedPolicyRule,
		[]tobari.PolicyDenyRule, []tobari.PolicyDenyRule,
	) (tobari.PolicyActivationReceipt, error)
	ClusterDown(context.Context, tobari.State, bool) error
}

type policyDecisionSetRuntimePort interface {
	ApplyPolicyDecisionSet(
		context.Context, tobari.State,
		[]tobari.LearnedPolicyRule, []tobari.LearnedPolicyRule,
		[]tobari.PolicyDenyRule, []tobari.PolicyDenyRule,
	) (tobari.PolicyActivationReceipt, error)
}

// clusterUpProgressRuntimePort is an optional extension of RuntimePort.
// Production runtimes use it to keep human progress outside application
// policy and Docker output.
type clusterUpProgressRuntimePort interface {
	ClusterUpWithProgress(context.Context, tobari.ClusterUpProgressSink) (tobari.State, error)
}

// ProjectRuntimePort is the CWD-owned lifecycle boundary. It is separate from
// RuntimePort so shared-cluster and policy test doubles do not implement
// unrelated project operations.
type ProjectRuntimePort interface {
	ResolveProject(context.Context, string) (tobari.ProjectInstance, bool, error)
	CreateProject(context.Context, string) (tobari.ProjectInstance, error)
	ListProjects(context.Context) ([]tobari.ProjectInstance, error)
	ProjectHome(context.Context, tobari.ProjectInstance) (string, error)
	IsTerminal(io.Writer) bool
	IsInputTerminal(io.Reader) bool
	// ValidateProjectRuntime is a read-only precondition for a new Workspace.
	// It must not create logical state or Docker resources.
	ValidateProjectRuntime(context.Context, tobari.State) error
	EnsureProjectRuntime(context.Context, tobari.State, tobari.ProjectInstance) (tobari.ProjectInstance, error)
	InspectProjectRuntime(context.Context, tobari.ProjectInstance) (tobari.RuntimeDiagnostic, error)
	ProjectSessionAttached(context.Context, tobari.ProjectInstance) (bool, error)
	EnterProjectRuntime(context.Context, tobari.ProjectInstance, tobari.ContextManifest, string, io.Reader, io.Writer, io.Writer) (int, error)
	InsideProject(context.Context) bool
	DeleteProject(context.Context, tobari.ProjectInstance) error
}

type contextAwareProjectRuntimePort interface {
	ResolveContext(context.Context, string) (tobari.ContextManifest, error)
	ResolveBoundProject(context.Context, string, tobari.ContextManifest) (tobari.ProjectInstance, bool, error)
	CreateBoundProject(context.Context, string, tobari.ContextManifest) (tobari.ProjectInstance, error)
	ValidateProjectRuntimeForContext(context.Context, tobari.State, string) error
}

type contextObservationProjectRuntimePort interface {
	ObserveContext(context.Context, string) (tobari.ContextObservation, error)
	ObserveBoundProject(context.Context, string, tobari.ContextManifest) (tobari.ProjectInstance, bool, error)
}

// WorkspaceSelector is the presentation boundary for an ambiguous CWD
// selection. Application code owns the typed snapshot and validates the
// returned choice; CLI owns the human interaction implementation.
type WorkspaceSelector interface {
	Select(context.Context, tobari.ProjectSelection, io.Reader, io.Writer) (tobari.ProjectSelectionChoice, error)
}

// lifecycleRuntimePort serializes shared-cluster and CWD-owned project
// lifecycle operations. It is intentionally separate from the broader
// RuntimePort so observation ports cannot acquire a
// lock they do not need.
type lifecycleRuntimePort interface {
	WithLifecycleLock(context.Context, func(context.Context) error) error
}

type activeContextRuntimePort interface {
	ActiveContextName(context.Context) (string, error)
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
		validPolicyCandidate := intent.Target.Kind == tobari.PolicyCandidateKind && intent.Target.ID != ""
		validPolicyRule := intent.Target.Kind == tobari.PolicyRuleKind && intent.Target.ID != ""
		validPolicyCompaction := intent.Target.Kind == tobari.PolicyCompactionKind && intent.Target.ID != ""
		validPolicyDecisionSet := intent.Target.Kind == tobari.PolicyDecisionSetKind &&
			intent.Target.ID == tobari.PolicyDecisionSetID
		if !validCluster && !validPolicyCandidate && !validPolicyRule && !validPolicyCompaction && !validPolicyDecisionSet && !validCurrentDirectory {
			return fault.New(fault.KindRejected, "mutation_rejected", "mutation target is not owned by Tobari", false)
		}
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "mutation effect is not supported", false)
	}
	return nil
}

// Service coordinates validated tasks without depending on Docker.
type Service struct {
	runtime  RuntimePort
	mutator  *execution.Invoker
	selector WorkspaceSelector
}

func New(runtime RuntimePort) *Service {
	return NewWithWorkspaceSelector(runtime, nil)
}

func NewWithWorkspaceSelector(runtime RuntimePort, selector WorkspaceSelector) *Service {
	return &Service{runtime: runtime, mutator: execution.New(ownedPolicy{}), selector: selector}
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

// IsInputTerminal reports whether the injected input is an interactive
// terminal. RuntimePort deliberately keeps this capability optional so
// read-only and application test doubles do not need to model terminal
// inspection just to use the policy service.
func (s *Service) IsInputTerminal(reader io.Reader) bool {
	if s == nil || portcheck.IsNil(s.runtime) {
		return false
	}
	inputTerminal, ok := s.runtime.(interface {
		IsInputTerminal(io.Reader) bool
	})
	if !ok || portcheck.IsNil(inputTerminal) {
		return false
	}
	return inputTerminal.IsInputTerminal(reader)
}

// IsInteractive reports whether a human-facing workflow may safely read
// commands and confirmations. Both streams must be terminals; a redirected
// input or output must remain on the read-only path.
func (s *Service) IsInteractive(in io.Reader, out io.Writer) bool {
	return s.IsTerminal(out) && s.IsInputTerminal(in)
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
	if !clusterStatus.Running {
		return tobari.State{}, fault.New(
			fault.KindUnavailable, "cluster_not_ready",
			"the shared cluster is not ready; repair it with an explicit cluster operation", false,
			fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared Gateway, OPA, and Auth Broker cluster explicitly."},
		)
	}
	return state, nil
}

func (s *Service) resolveExecutionContext(ctx context.Context, name string) (tobari.ContextManifest, error) {
	if aware, ok := s.runtime.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		manifest, err := aware.ResolveContext(ctx, name)
		if err != nil {
			return tobari.ContextManifest{}, fault.Wrap(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false, err,
				fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
		}
		if err := manifest.Validate(); err != nil {
			return tobari.ContextManifest{}, fault.Wrap(fault.KindContract, "invalid_context_binding", "the selected Context binding is invalid", false, err,
				fault.NextAction{Command: "context list", Reason: "Inspect the Context catalog before selecting a Workspace."})
		}
		return manifest, nil
	}
	if name != "" && name != tobari.DefaultContextName {
		return tobari.ContextManifest{}, fault.New(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false,
			fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
	}
	return tobari.ContextManifest{SchemaVersion: tobari.ContextSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000099",
		Name: tobari.DefaultContextName, AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector,
		PolicyMode: tobari.ContextPolicyModeGuided}, nil
}

func (s *Service) observeExecutionContext(ctx context.Context, name string) (tobari.ContextObservation, error) {
	if aware, ok := s.runtime.(contextObservationProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		observed, err := aware.ObserveContext(ctx, name)
		if err != nil {
			if errors.Is(err, tobari.ErrContextNotFound) {
				return tobari.ContextObservation{}, fault.Wrap(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false, err,
					fault.NextAction{Command: "context list", Reason: "Choose an existing Context."})
			}
			return tobari.ContextObservation{}, fault.Wrap(fault.KindInternal, "context_read_failed", "the selected Context could not be observed safely", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."})
		}
		return observed, nil
	}
	manifest, err := s.resolveExecutionContext(ctx, name)
	if err != nil {
		return tobari.ContextObservation{}, err
	}
	return tobari.ContextObservation{State: tobari.ContextObservationPersisted, Name: manifest.Name, Manifest: &manifest}, nil
}

func observeProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.ContextManifest) (tobari.ProjectInstance, bool, error) {
	if aware, ok := project.(contextObservationProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ObserveBoundProject(ctx, cwd, manifest)
	}
	return resolveProjectForContext(ctx, project, cwd, manifest)
}

func resolveProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.ContextManifest) (tobari.ProjectInstance, bool, error) {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ResolveBoundProject(ctx, cwd, manifest)
	}
	return project.ResolveProject(ctx, cwd)
}

func createProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.ContextManifest) (tobari.ProjectInstance, error) {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.CreateBoundProject(ctx, cwd, manifest)
	}
	return project.CreateProject(ctx, cwd)
}

func validateResolvedProjectContext(instance tobari.ProjectInstance, manifest tobari.ContextManifest) error {
	if instance.ContextID == manifest.ID && instance.ContextName == manifest.Name {
		return nil
	}
	return fault.New(
		fault.KindContract, "context_binding_stale", "Workspace Context binding is stale", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect Context and Workspace state."},
	)
}

func validateProjectRuntimeForContext(ctx context.Context, project ProjectRuntimePort, state tobari.State, manifest tobari.ContextManifest) error {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ValidateProjectRuntimeForContext(ctx, state, manifest.ID)
	}
	return project.ValidateProjectRuntime(ctx, state)
}

func (s *Service) projectSelection(
	ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.ContextManifest,
) (tobari.ProjectSelection, error) {
	instances, err := project.ListProjects(ctx)
	if err != nil {
		return tobari.ProjectSelection{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Workspace state could not be read", false, err)
	}
	byID := make(map[string]tobari.ProjectInstance, len(instances))
	indexes := make([]tobari.RootIndex, 0, len(instances))
	for _, instance := range instances {
		if err := instance.Validate(); err != nil {
			return tobari.ProjectSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection state is invalid", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		if _, exists := byID[instance.ID]; exists {
			return tobari.ProjectSelection{}, fault.New(fault.KindContract, "invalid_workspace_selection", "Workspace selection contains duplicate IDs", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		if instance.ContextID != manifest.ID {
			continue
		}
		if instance.ContextName != manifest.Name {
			return tobari.ProjectSelection{}, fault.New(fault.KindContract, "context_binding_stale", "Workspace Context binding is stale", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect Context and Workspace state."})
		}
		byID[instance.ID] = instance
		indexes = append(indexes, tobari.RootIndex{
			SchemaVersion: tobari.ProjectStateSchemaVersion,
			Root:          instance.Root,
			InstanceID:    instance.ID,
			ContextID:     instance.ContextID,
			ContextName:   instance.ContextName,
		})
	}
	containing, err := tobari.ContainingRoots(cwd, indexes)
	if err != nil {
		return tobari.ProjectSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection scope is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the current directory and local Workspace state."})
	}
	candidates := make([]tobari.ProjectSelectionCandidate, 0, len(containing))
	for _, index := range containing {
		instance, exists := byID[index.InstanceID]
		if !exists || instance.Root != index.Root {
			return tobari.ProjectSelection{}, fault.New(fault.KindContract, "invalid_workspace_selection", "Workspace index and state disagree", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		diagnostic, diagnosticErr := project.InspectProjectRuntime(ctx, instance)
		if diagnosticErr != nil {
			return tobari.ProjectSelection{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "Workspace runtime status could not be read", false, diagnosticErr,
				fault.NextAction{Command: "status", Reason: "Inspect the selected Workspace runtime."})
		}
		candidates = append(candidates, tobari.ProjectSelectionCandidate{
			ID: instance.ID, Root: instance.Root, ContextID: instance.ContextID,
			ContextName: instance.ContextName, Runtime: diagnostic,
		})
	}
	selection := tobari.ProjectSelection{
		CWD: cwd, Candidates: candidates, CanCreate: true,
	}
	for _, candidate := range candidates {
		if candidate.Root == cwd {
			selection.CanCreate = false
			break
		}
	}
	if err := selection.Validate(); err != nil {
		return tobari.ProjectSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
	}
	return selection, nil
}

func (s *Service) chooseWorkspace(
	ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.ContextManifest, in io.Reader, errOut io.Writer,
) (tobari.ProjectSelection, tobari.ProjectSelectionChoice, error) {
	selection, err := s.projectSelection(ctx, project, cwd, manifest)
	if err != nil {
		return tobari.ProjectSelection{}, tobari.ProjectSelectionChoice{}, err
	}
	if !selection.RequiresChoice() {
		if len(selection.Candidates) == 0 {
			return selection, tobari.ProjectSelectionChoice{Kind: tobari.ProjectSelectionCreate}, nil
		}
		return selection, tobari.ProjectSelectionChoice{
			Kind: tobari.ProjectSelectionUse, ID: selection.Candidates[0].ID,
		}, nil
	}
	if s.selector == nil || portcheck.IsNil(s.selector) {
		return tobari.ProjectSelection{}, tobari.ProjectSelectionChoice{}, fault.New(
			fault.KindInternal, "missing_workspace_selector",
			"Workspace selection is not configured", false,
			fault.NextAction{Command: "doctor", Reason: "Configure the Tobari terminal selector."},
		)
	}
	choice, selectErr := s.selector.Select(ctx, selection, in, errOut)
	if selectErr != nil {
		return tobari.ProjectSelection{}, tobari.ProjectSelectionChoice{}, selectErr
	}
	if err := selection.ValidateChoice(choice); err != nil {
		return tobari.ProjectSelection{}, tobari.ProjectSelectionChoice{}, fault.Wrap(
			fault.KindContract, "workspace_selection_invalid", "Workspace selection was invalid", false, err,
			fault.NextAction{Command: "tobari", Reason: "Choose a current Workspace or explicitly create one again."},
		)
	}
	return selection, choice, nil
}

func workspaceSelectionStaleFault() error {
	return fault.New(
		fault.KindRejected, "workspace_selection_stale",
		"Workspace choices changed before entry; no Workspace was modified", true,
		fault.NextAction{Command: "tobari", Reason: "Refresh the Workspace choices and select again."},
	)
}

// EnterProject resolves the current directory, ensures the shared runtime and
// project runtime, then attaches the caller's terminal. It never creates a
// project when the caller has no TTY.
func (s *Service) EnterProject(
	ctx context.Context, intent operation.Intent, in io.Reader, out, errOut io.Writer,
) (int, error) {
	return s.EnterProjectInContext(ctx, intent, "", in, out, errOut)
}

func (s *Service) EnterProjectInContext(
	ctx context.Context, intent operation.Intent, contextName string, in io.Reader, out, errOut io.Writer,
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
	if !project.IsTerminal(out) || !project.IsTerminal(errOut) || !project.IsInputTerminal(in) {
		return 0, fault.New(
			fault.KindInvalidInput, "tty_required",
			"tobari requires an interactive terminal", false,
			fault.NextAction{Command: "help tobari", Reason: "Run the root command from a terminal."},
		)
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	manifest, err := s.resolveExecutionContext(ctx, contextName)
	if err != nil {
		return 0, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return 0, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	if _, err := s.readyCluster(ctx); err != nil {
		return 0, err
	}
	selection, choice, err := s.chooseWorkspace(ctx, project, cwd, manifest, in, errOut)
	if err != nil {
		return 0, err
	}
	var state tobari.State
	var instance tobari.ProjectInstance
	request := execution.Request{
		Intent: intent, ExpectedCommand: intent.Command, ExpectedEffect: operation.EffectCreate,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	err = s.withLifecycleLock(ctx, func(lifecycleContext context.Context) error {
		var readyErr error
		state, readyErr = s.readyCluster(lifecycleContext)
		if readyErr != nil {
			return readyErr
		}
		return s.mutator.Invoke(lifecycleContext, request, func(actionContext context.Context, _ operation.Intent) error {
			var actionErr error
			switch choice.Kind {
			case tobari.ProjectSelectionCreate:
				if actionErr = validateProjectRuntimeForContext(actionContext, project, state, manifest); actionErr != nil {
					return classifyProjectMutationError(actionErr, "tobari", "runtime build", "the selected runtime is not ready for a new Workspace")
				}
				instance, actionErr = createProjectForContext(actionContext, project, cwd, manifest)
				if errors.Is(actionErr, tobari.ErrProjectExists) {
					return workspaceSelectionStaleFault()
				}
			case tobari.ProjectSelectionUse:
				candidate, found := selection.Candidate(choice.ID)
				if !found {
					return workspaceSelectionStaleFault()
				}
				var resolved tobari.ProjectInstance
				var resolvedFound bool
				resolved, resolvedFound, actionErr = resolveProjectForContext(actionContext, project, candidate.Root, manifest)
				if actionErr == nil && (!resolvedFound || resolved.ID != candidate.ID || resolved.Root != candidate.Root) {
					return workspaceSelectionStaleFault()
				}
				instance = resolved
			default:
				return fault.New(fault.KindContract, "workspace_selection_invalid", "Workspace selection choice is invalid", false,
					fault.NextAction{Command: "tobari", Reason: "Choose a current Workspace or explicitly create one again."})
			}
			if actionErr != nil {
				return classifyProjectMutationError(actionErr, "tobari", "status", "logical state may need reconciliation")
			}
			if instance.Incomplete {
				return fault.New(
					fault.KindRejected, "project_state_incomplete",
					"the current Tobari has incomplete logical state and cannot be recreated safely", false,
					fault.NextAction{Command: "delete", Reason: "Review the exact delete command and confirm removal of the incomplete current-directory Tobari."},
				)
			}
			instance, actionErr = project.EnsureProjectRuntime(actionContext, state, instance)
			if actionErr != nil {
				return classifyProjectMutationError(actionErr, "tobari", "status", "inspect the selected project before retrying")
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}
	code, err := project.EnterProjectRuntime(ctx, instance, manifest, cwd, in, out, errOut)
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
	return s.ProjectStatusInContext(ctx, "")
}

func (s *Service) ProjectStatusInContext(ctx context.Context, contextName string) (tobari.ProjectStatus, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectStatus{}, err
	}
	observed, err := s.observeExecutionContext(ctx, contextName)
	if err != nil {
		return tobari.ProjectStatus{}, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	if observed.State != tobari.ContextObservationPersisted {
		result := tobari.ProjectStatus{
			Task: tobari.TaskStatus, ContextState: observed.State, Exists: false,
			Runtime: tobari.RuntimeDiagnosticUnknown, ContextName: observed.Name,
			Attachment: tobari.AttachmentNotApplicable,
		}
		return result, result.Validate()
	}
	manifest := *observed.Manifest
	instance, found, err := observeProjectForContext(ctx, project, cwd, manifest)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	if !found {
		result := tobari.ProjectStatus{
			Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: false, Runtime: tobari.RuntimeDiagnosticUnknown,
			ContextID: manifest.ID, ContextName: manifest.Name, Attachment: tobari.AttachmentNotApplicable,
		}
		return result, result.Validate()
	}
	if err := validateResolvedProjectContext(instance, manifest); err != nil {
		return tobari.ProjectStatus{}, err
	}
	diagnostic, err := project.InspectProjectRuntime(ctx, instance)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "project runtime status could not be read", false, err)
	}
	attached, err := project.ProjectSessionAttached(ctx, instance)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(
			fault.KindInternal, "session_status_failed", "Workspace attachment status could not be read", false, err,
			fault.NextAction{Command: "status", Reason: "Inspect the selected Workspace runtime again."},
		)
	}
	attachment := tobari.AttachmentDetached
	if attached {
		attachment = tobari.AttachmentAttached
	}
	home, err := project.ProjectHome(ctx, instance)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, err)
	}
	result := tobari.ProjectStatus{
		Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: true, Root: instance.Root, ID: instance.ID,
		Home: home, ContextID: instance.ContextID, ContextName: instance.ContextName, Runtime: diagnostic,
		Attachment: attachment,
	}
	if err := result.Validate(); err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "project status is invalid", false, err)
	}
	return result, nil
}

// ProjectList observes every locally indexed logical Tobari and its runtime
// diagnostics. It does not create, repair, or delete any logical entry; the
// infrastructure may only serialize bounded cleanup of a pre-existing
// validated interruption journal.
func (s *Service) ProjectList(ctx context.Context) (tobari.ProjectListResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectListResult{}, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.ProjectListResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	instances, err := project.ListProjects(ctx)
	if err != nil {
		return tobari.ProjectListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	result := tobari.ProjectListResult{Task: tobari.TaskProjectList, Items: make([]tobari.ProjectListItem, 0, len(instances))}
	if len(instances) == 0 {
		return result, result.Validate()
	}
	observed, err := s.observeExecutionContext(ctx, "")
	if err != nil {
		return tobari.ProjectListResult{}, err
	}
	indexes := make([]tobari.RootIndex, 0, len(instances))
	for _, instance := range instances {
		indexes = append(indexes, tobari.RootIndex{
			SchemaVersion: tobari.ProjectStateSchemaVersion,
			Root:          instance.Root,
			InstanceID:    instance.ID,
			ContextID:     instance.ContextID,
			ContextName:   instance.ContextName,
		})
	}
	if observed.State == tobari.ContextObservationPersisted {
		currentIndexes, scopeErr := tobari.RootIndexesForContext(indexes, observed.Manifest.ID)
		if scopeErr != nil {
			return tobari.ProjectListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list Context scope is invalid", false, scopeErr)
		}
		current, found, selectionErr := tobari.NearestRoot(cwd, currentIndexes)
		if selectionErr != nil {
			return tobari.ProjectListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list selection is invalid", false, selectionErr)
		}
		if found {
			result.CurrentID = current.InstanceID
		}
	}
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
			Root: instance.Root, ID: instance.ID, Home: home, ContextID: instance.ContextID,
			ContextName: instance.ContextName, Runtime: diagnostic,
		})
	}
	if err := result.Validate(); err != nil {
		return tobari.ProjectListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list is invalid", false, err)
	}
	return result, nil
}

// DeleteProject removes only the nearest CWD-owned logical Tobari. A detached
// Workspace can be removed normally; an attached session requires force.
func (s *Service) DeleteProject(ctx context.Context, intent operation.Intent, force bool) (tobari.ProjectDeleteResult, error) {
	return s.DeleteProjectInContext(ctx, intent, "", force)
}

func (s *Service) DeleteProjectInContext(ctx context.Context, intent operation.Intent, contextName string, force bool) (tobari.ProjectDeleteResult, error) {
	return s.DeleteProjectWithContextBinding(ctx, intent, contextName, "", force)
}

// DeleteProjectWithContextBinding rejects a changed Context authority when a
// caller previously rendered a destructive preview for expectedContextID.
func (s *Service) DeleteProjectWithContextBinding(
	ctx context.Context,
	intent operation.Intent,
	contextName string,
	expectedContextID string,
	force bool,
) (tobari.ProjectDeleteResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectWrite); err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	manifest, err := s.resolveExecutionContext(ctx, contextName)
	if err != nil {
		return tobari.ProjectDeleteResult{}, err
	}
	if expectedContextID != "" && manifest.ID != expectedContextID {
		return tobari.ProjectDeleteResult{}, fault.New(
			fault.KindContract, "context_binding_stale", "the selected Context changed after the delete preview", false,
			fault.NextAction{Command: "delete", Reason: "Review the newly selected target before retrying force deletion."},
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
		instance, found, resolveErr = resolveProjectForContext(lifecycleContext, project, cwd, manifest)
		if resolveErr != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, resolveErr)
		}
		if !found {
			return fault.New(fault.KindNotFound, "project_not_found", "no Tobari exists for the current directory", false,
				fault.NextAction{Command: "tobari", Reason: "Create a Tobari from the current project directory."})
		}
		if err := validateResolvedProjectContext(instance, manifest); err != nil {
			return err
		}
		home, err = project.ProjectHome(lifecycleContext, instance)
		if err != nil {
			return fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, err)
		}
		if !force {
			attached, attachErr := project.ProjectSessionAttached(lifecycleContext, instance)
			if attachErr != nil {
				return fault.Wrap(
					fault.KindInternal, "session_status_failed",
					"could not determine whether a Workspace session is attached", false, attachErr,
					fault.NextAction{Command: "status", Reason: "Inspect the Workspace runtime before retrying deletion."},
				)
			}
			if attached {
				return fault.New(
					fault.KindRejected, "project_session_attached",
					"an interactive session is attached to this Workspace; exit it before deleting or use --force", false,
					fault.NextAction{Command: "delete", Reason: "Exit the attached session, then retry; use --force only when terminating it is intentional."},
				)
			}
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
	result := tobari.ProjectDeleteResult{Task: tobari.TaskDelete, Deleted: true, Root: instance.Root, ID: instance.ID, Home: home,
		ContextID: instance.ContextID, ContextName: instance.ContextName}
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
	state, err := s.readyCluster(ctx)
	if err != nil {
		return tobari.PolicyCandidateReport{}, err
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
	denyRules, err := s.runtime.ReadPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyCandidateReport{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"policy deny data could not be read safely", false, err,
		)
	}
	items, err := tobari.PolicyCandidatesWithDenyRules(denials, rules, denyRules)
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

// PolicyReview discovers the bounded exact-permission queue for a human host
// review. It is intentionally read-only; policy allow remains the separate
// opaque-reference-bound mutation.
func (s *Service) PolicyReview(
	ctx context.Context, tail int,
) (tobari.PolicyCandidateReport, error) {
	return s.policyCandidates(ctx, tail, tobari.TaskPolicyReview)
}

// ApplyPolicyReviewDecisionSet revalidates every staged opaque candidate
// against fresh retained evidence, then records and activates the complete set
// through one command-owned installation policy target.
func (s *Service) ApplyPolicyReviewDecisionSet(
	ctx context.Context, intent operation.Intent, set tobari.PolicyReviewDecisionSet,
) (tobari.PolicyReviewChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	if err := set.Validate(); err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_review_set",
			"reviewed policy decisions are invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyDecisionSetKind, tobari.PolicyDecisionSetID); err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	runtime, ok := s.runtime.(policyDecisionSetRuntimePort)
	if !ok || portcheck.IsNil(runtime) {
		return tobari.PolicyReviewChange{}, fault.New(
			fault.KindInternal, "missing_runtime", "reviewed policy apply is not configured", false,
		)
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyReviewChange{}, err
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denials, rules, denyRules)
	if err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract", "policy candidates are invalid", false, err,
		)
	}
	byID := make(map[string]tobari.PolicyCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.ID] = candidate
	}
	updatedAllows := append([]tobari.LearnedPolicyRule{}, rules...)
	updatedDenies := append([]tobari.PolicyDenyRule{}, denyRules.Exact...)
	allowCount, denyCount := 0, 0
	receipt := make([]tobari.PolicyReviewAppliedDecision, 0, len(set.Decisions))
	reviewContextID := ""
	for _, decision := range set.Decisions {
		candidate, found := byID[decision.CandidateID]
		if !found {
			return tobari.PolicyReviewChange{}, fault.New(
				fault.KindRejected, "policy_review_changed",
				"the reviewed permission set changed before Apply", false,
				fault.NextAction{Command: "policy review", Reason: "Review the current pending queue again."},
			)
		}
		if reviewContextID == "" {
			reviewContextID = candidate.ContextID
		} else if candidate.ContextID != reviewContextID {
			return tobari.PolicyReviewChange{}, fault.New(
				fault.KindRejected, "policy_review_scope_mixed",
				"one reviewed Apply cannot span multiple Context policy sources", false,
				fault.NextAction{Command: "policy review", Reason: "Apply or discard the current Context decisions before reviewing another Context."},
			)
		}
		if decision.Decision == tobari.PolicyDecisionAllow {
			rule, ruleErr := tobari.NewExactLearnedPolicyRule(candidate)
			if ruleErr != nil {
				return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "policy candidate cannot become an exact rule", false, ruleErr)
			}
			updatedAllows = append(updatedAllows, rule)
			allowCount++
		} else {
			rule, ruleErr := tobari.NewExactPolicyDenyRule(candidate)
			if ruleErr != nil {
				return tobari.PolicyReviewChange{}, fault.Wrap(fault.KindContract, "invalid_candidate_contract", "policy candidate cannot become an exact deny", false, ruleErr)
			}
			updatedDenies = append(updatedDenies, rule)
			denyCount++
		}
		applied, receiptErr := tobari.NewPolicyReviewAppliedDecision(candidate, decision.Decision)
		if receiptErr != nil {
			return tobari.PolicyReviewChange{}, fault.Wrap(
				fault.KindContract, "invalid_candidate_contract",
				"policy candidate cannot become a reviewed receipt", false, receiptErr,
			)
		}
		receipt = append(receipt, applied)
	}
	request := execution.Request{
		Intent: intent, ExpectedCommand: "policy apply-reviewed", ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	activation := tobari.PolicyActivationReceipt{}
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var applyErr error
		activation, applyErr = runtime.ApplyPolicyDecisionSet(
			actionContext, state, rules, updatedAllows, denyRules.Exact, updatedDenies,
		)
		if applyErr != nil {
			return applyErr
		}
		return activation.Validate()
	})
	if err != nil {
		if _, structured := fault.PublicCopy(err); structured {
			return tobari.PolicyReviewChange{}, err
		}
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"reviewed policy activation did not complete; inspect cluster status", false, err,
			fault.NextAction{Command: "cluster status", Reason: "Reconcile OPA and current policy state."},
		)
	}
	result := tobari.PolicyReviewChange{
		Task: tobari.TaskPolicyReviewApply, PolicyDirectory: activation.PolicyDirectory,
		AllowCount: allowCount, DenyCount: denyCount, Applied: true,
		ActiveRevision: activation.ActiveRevision, Decisions: receipt,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyReviewChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_review_result", "reviewed policy result is invalid", false, err,
		)
	}
	return result, nil
}

// PolicyRules returns the complete current learned-decision inventory. It is
// separate from PolicyReview because covered decisions intentionally disappear
// from the pending denial queue but remain user-manageable state.
func (s *Service) PolicyRules(
	ctx context.Context,
) (tobari.PolicyRuleReport, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyRuleReport{}, err
	}
	items, err := tobari.CurrentPolicyRules(rules, denyRules.Exact)
	if err != nil {
		return tobari.PolicyRuleReport{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_report",
			"current policy rules are invalid", false, err,
		)
	}
	result := tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: state.PolicyDirectory, Items: items,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyRuleReport{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_report",
			"current policy rule report is invalid", false, err,
		)
	}
	return result, nil
}

func (s *Service) loadPolicyState(
	ctx context.Context,
) (tobari.State, []tobari.LearnedPolicyRule, error) {
	state, err := s.readyCluster(ctx)
	if err != nil {
		return tobari.State{}, nil, err
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

func (s *Service) readPolicyDenyRules(
	ctx context.Context, state tobari.State,
) (tobari.PolicyDenyRuleSet, error) {
	denyRules, err := s.runtime.ReadPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyDenyRuleSet{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"policy deny data could not be read safely", false, err,
		)
	}
	if err := denyRules.Validate(); err != nil {
		return tobari.PolicyDenyRuleSet{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny", "policy deny data is invalid", false, err,
		)
	}
	return denyRules, nil
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
) (tobari.PolicyActivationReceipt, error) {
	request := execution.Request{
		Intent: intent, ExpectedCommand: expectedCommand, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	receipt := tobari.PolicyActivationReceipt{}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var actionErr error
		receipt, actionErr = s.runtime.ApplyLearnedPolicyRules(actionContext, state, expected, updated)
		if actionErr == nil {
			return receipt.Validate()
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
	return receipt, err
}

func (s *Service) applyPolicyDenies(
	ctx context.Context, intent operation.Intent, expectedCommand string, state tobari.State,
	expectedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	request := execution.Request{
		Intent: intent, ExpectedCommand: expectedCommand, ExpectedEffect: operation.EffectWrite,
		ExpectedTarget: intent.Target, ExpectedImpact: intent.Impact,
	}
	receipt := tobari.PolicyActivationReceipt{}
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		var actionErr error
		receipt, actionErr = s.runtime.ApplyPolicyDenyRules(
			actionContext, state, expectedAllows, expectedDenies, updatedDenies,
		)
		if actionErr == nil {
			return receipt.Validate()
		}
		if _, structured := fault.PublicCopy(actionErr); structured {
			return actionErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "policy_learning_failed",
			"policy deny activation did not complete; inspect cluster status", false, actionErr,
			fault.NextAction{
				Command: "cluster status",
				Reason:  "Reconcile OPA health and the current policy before another mutation.",
			},
		)
	})
	return receipt, err
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
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyLearningChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denials, rules, denyRules)
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
	activation, err := s.applyLearnedRules(ctx, intent, "policy allow", state, rules, updated)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	result := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: activation.PolicyDirectory,
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

// DenyPolicyCandidate records and activates one exact project-bound denial.
func (s *Service) DenyPolicyCandidate(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyDenyChange, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	if err := tobari.ValidatePolicyCandidateID(id); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_candidate_id",
			"policy candidate ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyCandidateKind, id); err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	denials, err := s.runtime.ClusterDenials(ctx, state, 10_000)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindInternal, "denials_failed", "cluster denials could not be read", false, err,
		)
	}
	candidates, err := tobari.PolicyCandidatesWithDenyRules(denials, rules, denyRules)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
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
		return tobari.PolicyDenyChange{}, fault.New(
			fault.KindInvalidInput, "policy_candidate_not_found",
			"policy candidate is stale, already covered, or outside retained logs", false,
		)
	}
	rule, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_candidate_contract",
			"policy candidate cannot become an exact deny rule", false, err,
		)
	}
	updatedDenies := append(append([]tobari.PolicyDenyRule{}, denyRules.Exact...), rule)
	updatedSet := tobari.PolicyDenyRuleSet{Baseline: denyRules.Baseline, Exact: updatedDenies}
	if err := updatedSet.Validate(); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny", "exact policy deny is invalid", false, err,
		)
	}
	activation, err := s.applyPolicyDenies(ctx, intent, "policy deny", state, rules, denyRules.Exact, updatedDenies)
	if err != nil {
		return tobari.PolicyDenyChange{}, err
	}
	result := tobari.PolicyDenyChange{
		Task: tobari.TaskPolicyDeny, PolicyDirectory: activation.PolicyDirectory,
		TargetID: id, Rule: rule, SourceRuleCount: 1, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyDenyChange{}, fault.Wrap(
			fault.KindContract, "invalid_policy_deny_result",
			"policy deny result is invalid", false, err,
		)
	}
	return result, nil
}

// ResetPolicyRule removes one current learned decision and returns the exact
// effect to default deny. It never creates a replacement Allow or Deny.
func (s *Service) ResetPolicyRule(
	ctx context.Context, intent operation.Intent, id string,
) (tobari.PolicyRuleReset, error) {
	if err := s.requireRuntime(); err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	if err := tobari.ValidatePolicyRuleID(id); err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindInvalidInput, "invalid_policy_rule_id",
			"policy rule ID is invalid", false, err,
		)
	}
	if err := validatePolicyMutationTarget(intent, tobari.PolicyRuleKind, id); err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	state, rules, err := s.loadPolicyState(ctx)
	if err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	denyRules, err := s.readPolicyDenyRules(ctx, state)
	if err != nil {
		return tobari.PolicyRuleReset{}, err
	}
	updatedRules, updatedDenies, removed, err := tobari.RemovePolicyRule(rules, denyRules.Exact, id)
	if err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindInvalidInput, "policy_rule_not_found",
			"policy rule is stale, baseline-owned, or no longer current", false, err,
		)
	}
	activation := tobari.PolicyActivationReceipt{}
	if removed.Decision == tobari.PolicyDecisionAllow {
		activation, err = s.applyLearnedRules(ctx, intent, "policy reset", state, rules, updatedRules)
		if err != nil {
			return tobari.PolicyRuleReset{}, err
		}
	} else {
		activation, err = s.applyPolicyDenies(ctx, intent, "policy reset", state, rules, denyRules.Exact, updatedDenies)
		if err != nil {
			return tobari.PolicyRuleReset{}, err
		}
	}
	result := tobari.PolicyRuleReset{
		Task: tobari.TaskPolicyReset, PolicyDirectory: activation.PolicyDirectory,
		TargetID: id, Decision: removed.Decision, Applied: true,
	}
	if err := result.Validate(); err != nil {
		return tobari.PolicyRuleReset{}, fault.Wrap(
			fault.KindContract, "invalid_policy_rule_reset_result",
			"policy rule reset result is invalid", false, err,
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
	activation, err := s.applyLearnedRules(ctx, intent, "policy compact", state, rules, updated)
	if err != nil {
		return tobari.PolicyLearningChange{}, err
	}
	result := tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyCompact, PolicyDirectory: activation.PolicyDirectory,
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
