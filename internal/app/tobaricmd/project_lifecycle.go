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
) (tobari.WorkspaceSessionOutcome, error) {
	return s.EnterProjectInContext(ctx, intent, "", in, out, errOut)
}

func (s *Service) EnterProjectInContext(
	ctx context.Context, intent operation.Intent, contextName string, in io.Reader, out, errOut io.Writer,
) (tobari.WorkspaceSessionOutcome, error) {
	return s.EnterProjectSessionInContext(
		ctx, intent, contextName, tobari.NewWorkspaceShellSession(), in, out, errOut,
	)
}

// EnterProjectSessionInContext enters the selected reusable Workspace with
// either its default Bash child or one exact direct child argv.
func (s *Service) EnterProjectSessionInContext(
	ctx context.Context, intent operation.Intent, contextName string, session tobari.WorkspaceSessionRequest,
	in io.Reader, out, errOut io.Writer,
) (tobari.WorkspaceSessionOutcome, error) {
	empty := tobari.WorkspaceSessionOutcome{}
	project, err := s.projectRuntime()
	if err != nil {
		return empty, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectCreate); err != nil {
		return empty, err
	}
	if err := session.Validate(); err != nil {
		return empty, fault.Wrap(
			fault.KindInvalidInput, "invalid_arguments", "Workspace session command is invalid", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Supply one non-empty command after the positional-only marker."},
		)
	}
	if project.InsideProject(ctx) {
		return empty, fault.New(
			fault.KindRejected, "already_inside",
			"This process is already inside a Workspace; nested entry is not supported", false,
			fault.NextAction{Command: "help tobari", Reason: "Exit the current Workspace session before entering another."},
		)
	}
	if !project.IsTerminal(out) || !project.IsTerminal(errOut) || !project.IsInputTerminal(in) {
		return empty, fault.New(
			fault.KindInvalidInput, "tty_required",
			"tobari requires an interactive terminal", false,
			fault.NextAction{Command: "help tobari", Reason: "Run the root command from a terminal."},
		)
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	manifest, err := s.resolveExecutionContext(ctx, contextName)
	if err != nil {
		return empty, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return empty, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	if _, err := s.readyCluster(ctx); err != nil {
		return empty, err
	}
	selection, choice, err := s.chooseWorkspace(ctx, project, cwd, manifest, in, errOut)
	if err != nil {
		return empty, err
	}
	var state tobari.State
	var instance tobari.Workspace
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
				var resolved tobari.Workspace
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
		return empty, err
	}
	outcome, err := project.EnterProjectRuntime(ctx, instance, manifest, cwd, session, in, out, errOut)
	if validationErr := outcome.Validate(); validationErr != nil {
		return empty, fault.Wrap(fault.KindContract, "invalid_workspace_session_outcome", "Workspace session result is invalid", false, validationErr)
	}
	if err != nil {
		return outcome, fault.Wrap(fault.KindInternal, "enter_failed", "Workspace session could not be started", false, err,
			fault.NextAction{Command: "status", Reason: "Inspect the selected project's runtime."})
	}
	return outcome, nil
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

// WorkspaceStatus observes the nearest CWD-owned Workspace without
// creating or repairing it.
func (s *Service) WorkspaceStatus(ctx context.Context) (tobari.WorkspaceStatus, error) {
	return s.ProjectStatusInContext(ctx, "")
}

func (s *Service) ProjectStatusInContext(ctx context.Context, contextName string) (tobari.WorkspaceStatus, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.WorkspaceStatus{}, err
	}
	observed, err := s.observeExecutionContext(ctx, contextName)
	if err != nil {
		return tobari.WorkspaceStatus{}, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	if observed.State != tobari.ManifestObservationPersisted {
		result := tobari.WorkspaceStatus{
			Task: tobari.TaskStatus, ManifestState: observed.State, Exists: false,
			Runtime: tobari.RuntimeDiagnosticUnknown, WorkspaceManifestName: observed.Name,
			Attachment: tobari.AttachmentNotApplicable,
			Bootstrap:  tobari.WorkspaceBootstrapReport{State: tobari.WorkspaceBootstrapNotConfigured},
		}
		return result, result.Validate()
	}
	manifest := *observed.Manifest
	next, err := tobari.NewDesiredEntry(manifest)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindContract, "invalid_desired_entry", "Manifest desired entry is invalid", false, err)
	}
	runtimeSelection, err := manifest.RuntimeSelection()
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindContract, "invalid_runtime_binding", "Context Runtime selection is invalid", false, err)
	}
	instance, found, err := observeProjectForContext(ctx, project, cwd, manifest)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	if !found {
		bootstrap, bootstrapErr := tobari.ResolveWorkspaceBootstrapReport("", manifest.Bootstrap)
		if bootstrapErr != nil {
			return tobari.WorkspaceStatus{}, bootstrapErr
		}
		result := tobari.WorkspaceStatus{
			Task: tobari.TaskStatus, ManifestState: tobari.ManifestObservationPersisted, Exists: false, Runtime: tobari.RuntimeDiagnosticUnknown,
			WorkspaceManifestID: manifest.ID, WorkspaceManifestName: manifest.Name, Attachment: tobari.AttachmentNotApplicable,
			Bootstrap: bootstrap, RuntimeSelection: runtimeSelection, Next: &next,
		}
		return result, result.Validate()
	}
	if err := validateResolvedProjectContext(instance, manifest); err != nil {
		return tobari.WorkspaceStatus{}, err
	}
	diagnostic, err := project.InspectProjectRuntime(ctx, instance)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "project runtime status could not be read", false, err)
	}
	attached, err := project.ProjectSessionAttached(ctx, instance)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(
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
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, err)
	}
	result := tobari.WorkspaceStatus{
		Task: tobari.TaskStatus, ManifestState: tobari.ManifestObservationPersisted, Exists: true, Root: instance.Root, ID: instance.ID,
		Home: home, WorkspaceManifestID: instance.WorkspaceManifestID, WorkspaceManifestName: instance.WorkspaceManifestName, Runtime: diagnostic,
		Attachment: attachment, RuntimeSelection: runtimeSelection,
		Current: instance.LastSuccessfulEntry, Next: &next, LastFailure: instance.LastFailure,
	}
	result.Adoption, err = instance.AdoptionState(manifest.Desired)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindContract, "invalid_adoption_state", "Workspace adoption state is invalid", false, err)
	}
	result.Bootstrap, err = tobari.ResolveWorkspaceBootstrapReport(instance.CreationApplied.BootstrapRevision, manifest.Bootstrap)
	if err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindContract, "invalid_bootstrap_status", "Workspace bootstrap status is invalid", false, err)
	}
	if err := result.Validate(); err != nil {
		return tobari.WorkspaceStatus{}, fault.Wrap(fault.KindContract, "invalid_status_contract", "project status is invalid", false, err)
	}
	return result, nil
}

// ProjectList observes every locally indexed Workspace and its runtime
// diagnostics. It does not create, repair, or delete any logical entry; the
// infrastructure may only serialize bounded cleanup of a pre-existing
// validated interruption journal.
func (s *Service) ProjectList(ctx context.Context) (tobari.WorkspaceListResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.WorkspaceListResult{}, err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	instances, err := project.ListProjects(ctx)
	if err != nil {
		return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project state could not be read", false, err)
	}
	result := tobari.WorkspaceListResult{Task: tobari.TaskWorkspaceList, Items: make([]tobari.WorkspaceListItem, 0, len(instances))}
	if len(instances) == 0 {
		return result, result.Validate()
	}
	observed, err := s.observeExecutionContext(ctx, "")
	if err != nil {
		return tobari.WorkspaceListResult{}, err
	}
	indexes := make([]tobari.RootIndex, 0, len(instances))
	manifests := make(map[string]tobari.WorkspaceManifest, len(instances))
	for _, instance := range instances {
		manifest, manifestErr := readWorkspaceManifestByID(ctx, s.runtime, instance.WorkspaceManifestID)
		if manifestErr != nil {
			return tobari.WorkspaceListResult{}, manifestErr
		}
		manifests[instance.ID] = manifest
		indexes = append(indexes, tobari.RootIndex{
			SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
			Root:                  instance.Root,
			InstanceID:            instance.ID,
			WorkspaceManifestID:   instance.WorkspaceManifestID,
			WorkspaceManifestName: instance.WorkspaceManifestName,
		})
	}
	if observed.State == tobari.ManifestObservationPersisted {
		currentIndexes, scopeErr := tobari.RootIndexesForContext(indexes, observed.Manifest.ID)
		if scopeErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list Context scope is invalid", false, scopeErr)
		}
		current, found, selectionErr := tobari.NearestRoot(cwd, currentIndexes)
		if selectionErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list selection is invalid", false, selectionErr)
		}
		if found {
			result.CurrentID = current.InstanceID
		}
	}
	for _, instance := range instances {
		manifest := manifests[instance.ID]
		next, nextErr := tobari.NewDesiredEntry(manifest)
		if nextErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindContract, "invalid_desired_entry", "Manifest desired entry is invalid", false, nextErr)
		}
		adoption, adoptionErr := instance.AdoptionState(manifest.Desired)
		if adoptionErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindContract, "invalid_adoption_state", "Workspace adoption state is invalid", false, adoptionErr)
		}
		diagnostic, diagnosticErr := project.InspectProjectRuntime(ctx, instance)
		if diagnosticErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "project runtime status could not be read", false, diagnosticErr)
		}
		home, homeErr := project.ProjectHome(ctx, instance)
		if homeErr != nil {
			return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindInternal, "state_read_failed", "project home path could not be resolved", false, homeErr)
		}
		result.Items = append(result.Items, tobari.WorkspaceListItem{
			Root: instance.Root, ID: instance.ID, Home: home, WorkspaceManifestID: instance.WorkspaceManifestID,
			WorkspaceManifestName: manifest.Name, Runtime: diagnostic, Adoption: adoption,
			Current: instance.LastSuccessfulEntry, Next: next, LastFailure: instance.LastFailure,
		})
	}
	if err := result.Validate(); err != nil {
		return tobari.WorkspaceListResult{}, fault.Wrap(fault.KindContract, "invalid_list_contract", "project list is invalid", false, err)
	}
	return result, nil
}

// DeleteProject removes only the nearest CWD-owned Workspace. A detached
// Workspace can be removed normally; an attached session requires force.
func (s *Service) DeleteProject(ctx context.Context, intent operation.Intent, force bool) (tobari.WorkspaceDeleteResult, error) {
	return s.DeleteProjectInContext(ctx, intent, "", force)
}

func (s *Service) DeleteProjectInContext(ctx context.Context, intent operation.Intent, contextName string, force bool) (tobari.WorkspaceDeleteResult, error) {
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
) (tobari.WorkspaceDeleteResult, error) {
	project, err := s.projectRuntime()
	if err != nil {
		return tobari.WorkspaceDeleteResult{}, err
	}
	if err := s.validateProjectIntent(intent, operation.EffectWrite); err != nil {
		return tobari.WorkspaceDeleteResult{}, err
	}
	manifest, err := s.resolveExecutionContext(ctx, contextName)
	if err != nil {
		return tobari.WorkspaceDeleteResult{}, err
	}
	if expectedContextID != "" && manifest.ID != expectedContextID {
		return tobari.WorkspaceDeleteResult{}, fault.New(
			fault.KindContract, "context_binding_stale", "the selected Context changed after the delete preview", false,
			fault.NextAction{Command: "delete", Reason: "Review the newly selected target before retrying force deletion."},
		)
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return tobari.WorkspaceDeleteResult{}, fault.Wrap(fault.KindInvalidInput, "invalid_root", "current directory could not be resolved", false, err)
	}
	var instance tobari.Workspace
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
		return tobari.WorkspaceDeleteResult{}, err
	}
	result := tobari.WorkspaceDeleteResult{Task: tobari.TaskDelete, Deleted: true, Root: instance.Root, ID: instance.ID, Home: home,
		WorkspaceManifestID: instance.WorkspaceManifestID, WorkspaceManifestName: instance.WorkspaceManifestName}
	if err := result.Validate(); err != nil {
		return tobari.WorkspaceDeleteResult{}, fault.Wrap(fault.KindContract, "invalid_delete_contract", "project delete result is invalid", false, err)
	}
	return result, nil
}
