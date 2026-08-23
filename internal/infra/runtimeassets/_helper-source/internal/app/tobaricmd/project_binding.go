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
	ResolveProject(context.Context, string) (tobari.Workspace, bool, error)
	CreateProject(context.Context, string) (tobari.Workspace, error)
	ListProjects(context.Context) ([]tobari.Workspace, error)
	ProjectHome(context.Context, tobari.Workspace) (string, error)
	IsTerminal(io.Writer) bool
	IsInputTerminal(io.Reader) bool
	// ValidateProjectRuntime is a read-only precondition for a new Workspace.
	// It must not create logical state or Docker resources.
	ValidateProjectRuntime(context.Context, tobari.State) error
	EnsureProjectRuntime(context.Context, tobari.State, tobari.Workspace) (tobari.Workspace, error)
	InspectProjectRuntime(context.Context, tobari.Workspace) (tobari.RuntimeDiagnostic, error)
	ProjectSessionAttached(context.Context, tobari.Workspace) (bool, error)
	EnterProjectRuntime(context.Context, tobari.Workspace, tobari.WorkspaceManifest, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.WorkspaceSessionOutcome, error)
	InsideProject(context.Context) bool
	DeleteProject(context.Context, tobari.Workspace) error
}

type contextAwareProjectRuntimePort interface {
	ResolveContext(context.Context, string) (tobari.WorkspaceManifest, error)
	ResolveBoundProject(context.Context, string, tobari.WorkspaceManifest) (tobari.Workspace, bool, error)
	CreateBoundProject(context.Context, string, tobari.WorkspaceManifest) (tobari.Workspace, error)
	ValidateProjectRuntimeForContext(context.Context, tobari.State, string) error
}

type contextObservationProjectRuntimePort interface {
	ObserveContext(context.Context, string) (tobari.ManifestObservation, error)
	ObserveBoundProject(context.Context, string, tobari.WorkspaceManifest) (tobari.Workspace, bool, error)
}

// manifestIdentityReader resolves authority by stable ID for Workspace list
// projections. It is deliberately separate from name/default selection.
type manifestIdentityReader interface {
	ReadWorkspaceManifestByID(context.Context, string) (tobari.WorkspaceManifest, error)
}

func readWorkspaceManifestByID(ctx context.Context, runtime RuntimePort, id string) (tobari.WorkspaceManifest, error) {
	reader, ok := runtime.(manifestIdentityReader)
	if !ok || portcheck.IsNil(reader) {
		return tobari.WorkspaceManifest{}, fault.New(fault.KindInternal, "missing_runtime", "Manifest identity reader is unavailable", false)
	}
	manifest, err := reader.ReadWorkspaceManifestByID(ctx, id)
	if err != nil {
		return tobari.WorkspaceManifest{}, fault.Wrap(fault.KindInternal, "manifest_read_failed", "Workspace Manifest could not be read by stable identity", false, err)
	}
	if manifest.ID != id {
		return tobari.WorkspaceManifest{}, fault.New(fault.KindContract, "manifest_identity_mismatch", "Workspace Manifest identity changed while reading Workspace state", false)
	}
	return manifest, nil
}

// WorkspaceSelector is the presentation boundary for an ambiguous CWD
// selection. Application code owns the typed snapshot and validates the
// returned choice; CLI owns the human interaction implementation.
type WorkspaceSelector interface {
	Select(context.Context, tobari.WorkspaceSelection, io.Reader, io.Writer) (tobari.ProjectSelectionChoice, error)
}

type activeContextRuntimePort interface {
	DefaultManifestName(context.Context) (string, error)
}

type projectRootRuntimePort interface {
	ResolveProjectRoot(context.Context, string) (string, error)
}

// CurrentProjectRoot resolves the exact protected project root before a
// first-use review. It is read-only and deliberately performs no Workspace or
// Docker preparation.
func (s *Service) CurrentProjectRoot(ctx context.Context) (string, error) {
	if err := s.requireRuntime(); err != nil {
		return "", err
	}
	cwd, err := s.runtime.CurrentDirectory(ctx)
	if err != nil {
		return "", fault.Wrap(fault.KindInvalidInput, "invalid_root", "the current project root is unavailable", false, err)
	}
	resolver, ok := s.runtime.(projectRootRuntimePort)
	if !ok || portcheck.IsNil(resolver) {
		return "", fault.New(fault.KindInternal, "missing_runtime", "project-root validation is unavailable", false)
	}
	root, err := resolver.ResolveProjectRoot(ctx, cwd)
	if err != nil {
		return "", fault.Wrap(fault.KindInvalidInput, "invalid_root", "the current project root is not eligible for a Workspace", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Run Tobari from an accessible project directory outside protected host paths."})
	}
	if err := tobari.ValidateCanonicalRoot(root); err != nil {
		return "", fault.Wrap(fault.KindContract, "invalid_root", "the resolved project root is invalid", false, err)
	}
	return root, nil
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

func (s *Service) resolveExecutionContext(ctx context.Context, name string) (tobari.WorkspaceManifest, error) {
	if aware, ok := s.runtime.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		manifest, err := aware.ResolveContext(ctx, name)
		if err != nil {
			return tobari.WorkspaceManifest{}, fault.Wrap(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false, err,
				fault.NextAction{Command: "manifest list", Reason: "Choose an existing Context."})
		}
		if err := manifest.Validate(); err != nil {
			return tobari.WorkspaceManifest{}, fault.Wrap(fault.KindContract, "invalid_context_binding", "the selected Context binding is invalid", false, err,
				fault.NextAction{Command: "manifest list", Reason: "Inspect the Context catalog before selecting a Workspace."})
		}
		return manifest, nil
	}
	if name != "" && name != tobari.DefaultManifestName {
		return tobari.WorkspaceManifest{}, fault.New(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false,
			fault.NextAction{Command: "manifest list", Reason: "Choose an existing Context."})
	}
	return tobari.WorkspaceManifest{SchemaVersion: tobari.WorkspaceManifestSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000099",
		Name: tobari.DefaultManifestName, AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector,
		PolicyMode: tobari.ManifestPolicyModeGuided, SourceAccess: tobari.ManifestSourceAccessReadWrite}, nil
}

func (s *Service) observeExecutionContext(ctx context.Context, name string) (tobari.ManifestObservation, error) {
	if aware, ok := s.runtime.(contextObservationProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		observed, err := aware.ObserveContext(ctx, name)
		if err != nil {
			if errors.Is(err, tobari.ErrContextNotFound) {
				return tobari.ManifestObservation{}, fault.Wrap(fault.KindNotFound, "context_not_found", "the selected Context is unavailable", false, err,
					fault.NextAction{Command: "manifest list", Reason: "Choose an existing Context."})
			}
			return tobari.ManifestObservation{}, fault.Wrap(fault.KindInternal, "context_read_failed", "the selected Context could not be observed safely", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect the host Context stores."})
		}
		return observed, nil
	}
	manifest, err := s.resolveExecutionContext(ctx, name)
	if err != nil {
		return tobari.ManifestObservation{}, err
	}
	return tobari.ManifestObservation{State: tobari.ManifestObservationPersisted, Name: manifest.Name, Manifest: &manifest}, nil
}

func observeProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, bool, error) {
	if aware, ok := project.(contextObservationProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ObserveBoundProject(ctx, cwd, manifest)
	}
	return resolveProjectForContext(ctx, project, cwd, manifest)
}

func resolveProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, bool, error) {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ResolveBoundProject(ctx, cwd, manifest)
	}
	return project.ResolveProject(ctx, cwd)
}

func createProjectForContext(ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.WorkspaceManifest) (tobari.Workspace, error) {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.CreateBoundProject(ctx, cwd, manifest)
	}
	return project.CreateProject(ctx, cwd)
}

func validateResolvedProjectContext(instance tobari.Workspace, manifest tobari.WorkspaceManifest) error {
	if instance.WorkspaceManifestID == manifest.ID && instance.WorkspaceManifestName == manifest.Name {
		return nil
	}
	return fault.New(
		fault.KindContract, "context_binding_stale", "Workspace Context binding is stale", false,
		fault.NextAction{Command: "doctor", Reason: "Inspect Context and Workspace state."},
	)
}

func validateProjectRuntimeForContext(ctx context.Context, project ProjectRuntimePort, state tobari.State, manifest tobari.WorkspaceManifest) error {
	if aware, ok := project.(contextAwareProjectRuntimePort); ok && !portcheck.IsNil(aware) {
		return aware.ValidateProjectRuntimeForContext(ctx, state, manifest.ID)
	}
	return project.ValidateProjectRuntime(ctx, state)
}

func (s *Service) projectSelection(
	ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.WorkspaceManifest,
) (tobari.WorkspaceSelection, error) {
	instances, err := project.ListProjects(ctx)
	if err != nil {
		return tobari.WorkspaceSelection{}, fault.Wrap(fault.KindInternal, "state_read_failed", "Workspace state could not be read", false, err)
	}
	byID := make(map[string]tobari.Workspace, len(instances))
	indexes := make([]tobari.RootIndex, 0, len(instances))
	for _, instance := range instances {
		if err := instance.Validate(); err != nil {
			return tobari.WorkspaceSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection state is invalid", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		if _, exists := byID[instance.ID]; exists {
			return tobari.WorkspaceSelection{}, fault.New(fault.KindContract, "invalid_workspace_selection", "Workspace selection contains duplicate IDs", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		if instance.WorkspaceManifestID != manifest.ID {
			continue
		}
		if instance.WorkspaceManifestName != manifest.Name {
			return tobari.WorkspaceSelection{}, fault.New(fault.KindContract, "context_binding_stale", "Workspace Context binding is stale", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect Context and Workspace state."})
		}
		byID[instance.ID] = instance
		indexes = append(indexes, tobari.RootIndex{
			SchemaVersion:         tobari.WorkspaceStateSchemaVersion,
			Root:                  instance.Root,
			InstanceID:            instance.ID,
			WorkspaceManifestID:   instance.WorkspaceManifestID,
			WorkspaceManifestName: instance.WorkspaceManifestName,
		})
	}
	containing, err := tobari.ContainingRoots(cwd, indexes)
	if err != nil {
		return tobari.WorkspaceSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection scope is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the current directory and local Workspace state."})
	}
	candidates := make([]tobari.WorkspaceSelectionCandidate, 0, len(containing))
	for _, index := range containing {
		instance, exists := byID[index.InstanceID]
		if !exists || instance.Root != index.Root {
			return tobari.WorkspaceSelection{}, fault.New(fault.KindContract, "invalid_workspace_selection", "Workspace index and state disagree", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
		}
		diagnostic, diagnosticErr := project.InspectProjectRuntime(ctx, instance)
		if diagnosticErr != nil {
			return tobari.WorkspaceSelection{}, fault.Wrap(fault.KindInternal, "runtime_status_failed", "Workspace runtime status could not be read", false, diagnosticErr,
				fault.NextAction{Command: "status", Reason: "Inspect the selected Workspace runtime."})
		}
		candidates = append(candidates, tobari.WorkspaceSelectionCandidate{
			ID: instance.ID, Root: instance.Root, WorkspaceManifestID: instance.WorkspaceManifestID,
			WorkspaceManifestName: instance.WorkspaceManifestName, Runtime: diagnostic,
		})
	}
	selection := tobari.WorkspaceSelection{
		CWD: cwd, Candidates: candidates, CanCreate: true,
	}
	for _, candidate := range candidates {
		if candidate.Root == cwd {
			selection.CanCreate = false
			break
		}
	}
	if err := selection.Validate(); err != nil {
		return tobari.WorkspaceSelection{}, fault.Wrap(fault.KindContract, "invalid_workspace_selection", "Workspace selection is invalid", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect local Workspace state."})
	}
	return selection, nil
}

func (s *Service) chooseWorkspace(
	ctx context.Context, project ProjectRuntimePort, cwd string, manifest tobari.WorkspaceManifest, in io.Reader, errOut io.Writer,
) (tobari.WorkspaceSelection, tobari.ProjectSelectionChoice, error) {
	selection, err := s.projectSelection(ctx, project, cwd, manifest)
	if err != nil {
		return tobari.WorkspaceSelection{}, tobari.ProjectSelectionChoice{}, err
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
		return tobari.WorkspaceSelection{}, tobari.ProjectSelectionChoice{}, fault.New(
			fault.KindInternal, "missing_workspace_selector",
			"Workspace selection is not configured", false,
			fault.NextAction{Command: "doctor", Reason: "Configure the Tobari terminal selector."},
		)
	}
	choice, selectErr := s.selector.Select(ctx, selection, in, errOut)
	if selectErr != nil {
		return tobari.WorkspaceSelection{}, tobari.ProjectSelectionChoice{}, selectErr
	}
	if err := selection.ValidateChoice(choice); err != nil {
		return tobari.WorkspaceSelection{}, tobari.ProjectSelectionChoice{}, fault.Wrap(
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
