package tobaricmd

import (
	"context"
	"errors"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

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
	return s.EnterProjectInContext(ctx, intent, "", in, out, errOut)
}

func (s *Service) EnterProjectInContext(
	ctx context.Context, intent operation.Intent, contextName string, in io.Reader, out, errOut io.Writer,
) (int, error) {
	return s.EnterProjectSessionInContext(
		ctx, intent, contextName, tobari.NewWorkspaceShellSession(), in, out, errOut,
	)
}

// EnterProjectSessionInContext enters the selected reusable Workspace with
// either its default Bash child or one exact direct child argv.
func (s *Service) EnterProjectSessionInContext(
	ctx context.Context, intent operation.Intent, contextName string, session tobari.WorkspaceSessionRequest,
	in io.Reader, out, errOut io.Writer,
) (int, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return 0, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectCreate); err != nil {
		return 0, err
	}
	if err := session.Validate(); err != nil {
		return 0, fault.Wrap(
			fault.KindInvalidInput, "invalid_arguments", "Workspace session command is invalid", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Supply one non-empty command after the positional-only marker."},
		)
	}
	if project.InsideProject(ctx) {
		return 0, fault.New(
			fault.KindRejected, "already_inside",
			"This process is already inside a Workspace; nested entry is not supported", false,
			fault.NextAction{Command: "exit", Reason: "Leave the current Workspace before entering another session."},
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
					"the current Workspace has incomplete logical state and cannot be recreated safely", false,
					fault.NextAction{Command: "delete", Reason: "Review the exact delete command and confirm removal of the incomplete current-directory Workspace."},
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
	code, err := project.EnterProjectRuntime(ctx, instance, manifest, cwd, session, in, out, errOut)
	if err != nil {
		return 0, fault.Wrap(fault.KindInternal, "enter_failed", "Workspace session could not be started", false, err,
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

// ProjectStatus observes the nearest CWD-owned Workspace without
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
			Bootstrap:  tobari.WorkspaceBootstrapReport{State: tobari.WorkspaceBootstrapNotConfigured},
		}
		return result, result.Validate()
	}
	manifest := *observed.Manifest
	runtimeSelection, err := manifest.RuntimeSelection()
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindContract, "invalid_runtime_binding", "Context Runtime selection is invalid", false, err)
	}
	instance, found, err := observeProjectForContext(ctx, project, cwd, manifest)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	if !found {
		bootstrap, bootstrapErr := tobari.ResolveWorkspaceBootstrapReport("", manifest.Bootstrap)
		if bootstrapErr != nil {
			return tobari.ProjectStatus{}, bootstrapErr
		}
		result := tobari.ProjectStatus{
			Task: tobari.TaskStatus, ContextState: tobari.ContextObservationPersisted, Exists: false, Runtime: tobari.RuntimeDiagnosticUnknown,
			ContextID: manifest.ID, ContextName: manifest.Name, Attachment: tobari.AttachmentNotApplicable,
			Bootstrap: bootstrap, RuntimeSelection: runtimeSelection,
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
		Attachment: attachment, RuntimeSelection: runtimeSelection,
	}
	result.Bootstrap, err = tobari.ResolveWorkspaceBootstrapReport(instance.BootstrapRevision, manifest.Bootstrap)
	if err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_status", "Workspace bootstrap status is invalid", false, err)
	}
	if err := result.Validate(); err != nil {
		return tobari.ProjectStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "project status is invalid", false, err)
	}
	return result, nil
}

// ProjectList observes every locally indexed Workspace and its runtime
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

// DeleteProject removes only the nearest CWD-owned Workspace. A detached
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
			return fault.New(fault.KindNotFound, "project_not_found", "no Workspace exists for the current directory", false,
				fault.NextAction{Command: "tobari", Reason: "Create a Workspace from the current project directory."})
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
