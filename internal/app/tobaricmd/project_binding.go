package tobaricmd

import (
	"context"
	"errors"
	"io"

	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

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
	EnterProjectRuntime(context.Context, tobari.ProjectInstance, tobari.ContextManifest, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (int, error)
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

type activeContextRuntimePort interface {
	ActiveContextName(context.Context) (string, error)
}

func (s *Service) projectRuntime() (ProjectRuntimePort, error) {
	if err := s.requireRuntime(); err != nil {
		return nil, err
	}
	project, ok := s.runtime.(ProjectRuntimePort)
	if !ok || portcheck.IsNil(project) {
		return nil, fault.New(
			fault.KindInternal, "missing_runtime",
			"CWD-selected Workspace runtime is not configured", false,
		)
	}
	return project, nil
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
		PolicyMode: tobari.ContextPolicyModeGuided, SourceAccess: tobari.ContextSourceAccessReadWrite}, nil
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
