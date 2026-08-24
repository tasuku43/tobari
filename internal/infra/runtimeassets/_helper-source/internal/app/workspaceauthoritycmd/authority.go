// Package workspaceauthoritycmd owns the final Workspace Template, Context,
// Policy Memory, and Workspace application tasks before public Catalog wiring.
package workspaceauthoritycmd

import (
	"fmt"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	TaskTemplateList         = "template list"
	TaskTemplateShow         = "template show"
	TaskTemplateCreate       = "template create"
	TaskTemplateCopy         = "template copy"
	TaskTemplateDefaultSet   = "template default set"
	TaskTemplateDelete       = "template delete"
	TaskTemplateConfigShell  = "config shell"
	TaskTemplateConfigGit    = "config git"
	TaskTemplateBootstrapAWS = "config bootstrap aws"
	TaskTemplateBootstrapEKS = "config bootstrap kubernetes eks"
	TaskTemplateRuntimeSet   = "template runtime set"
	TaskContextList          = "context list"
	TaskContextShow          = "context show"
	TaskContextCreate        = "context create"
	TaskContextEnter         = "context enter"
	TaskContextDelete        = "context delete"
	TaskWorkspaceList        = "workspace list"
	TaskWorkspaceStatus      = "workspace status"
	TaskWorkspaceDelete      = "workspace delete"
	TaskClusterUp            = "cluster up"
	TaskClusterDown          = "cluster down"
	TaskPolicyAllow          = "policy allow"
	TaskPolicyDeny           = "policy deny"
	TaskPolicyReset          = "policy reset"
	TaskPolicyApply          = "policy apply-reviewed"
)

type ContextSnapshot = tobari.ContextAuthoritySnapshot

type TemplateView struct {
	TemplateRef        string
	CurrentRevisionRef string
	Template           tobari.WorkspaceTemplate
}

func NewTemplateView(template tobari.WorkspaceTemplate) (TemplateView, error) {
	if err := template.Validate(); err != nil {
		return TemplateView{}, err
	}
	templateRef, err := tobari.WorkspaceTemplateRef(template.ID)
	if err != nil {
		return TemplateView{}, err
	}
	revisionRef, err := tobari.WorkspaceTemplateRevisionRef(template.ID, template.Current.Revision)
	if err != nil {
		return TemplateView{}, err
	}
	return TemplateView{TemplateRef: templateRef, CurrentRevisionRef: revisionRef, Template: template.Clone()}, nil
}

func (v TemplateView) Validate() error {
	want, err := NewTemplateView(v.Template)
	if err != nil {
		return err
	}
	if v.TemplateRef != want.TemplateRef || v.CurrentRevisionRef != want.CurrentRevisionRef {
		return fmt.Errorf("Template view references are inconsistent")
	}
	return nil
}

type TemplateList struct{ Items []TemplateView }

func NewTemplateList(templates []tobari.WorkspaceTemplate) (TemplateList, error) {
	if err := tobari.ValidateWorkspaceTemplateAuthorities(templates); err != nil {
		return TemplateList{}, err
	}
	items := make([]TemplateView, len(templates))
	seenIDs := make(map[tobari.WorkspaceTemplateID]struct{}, len(templates))
	for index, template := range templates {
		if _, exists := seenIDs[template.ID]; exists {
			return TemplateList{}, fmt.Errorf("Template collection contains a duplicate ID")
		}
		view, err := NewTemplateView(template)
		if err != nil {
			return TemplateList{}, err
		}
		items[index] = view
		seenIDs[template.ID] = struct{}{}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Template.Name == items[j].Template.Name {
			return items[i].Template.ID < items[j].Template.ID
		}
		return items[i].Template.Name < items[j].Template.Name
	})
	return TemplateList{Items: items}, nil
}

type ContextView struct {
	ContextRef string
	Snapshot   ContextSnapshot
}

func NewContextView(snapshot ContextSnapshot) (ContextView, error) {
	if err := snapshot.Validate(); err != nil {
		return ContextView{}, err
	}
	reference, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		return ContextView{}, err
	}
	return ContextView{ContextRef: reference, Snapshot: snapshot.Clone()}, nil
}

func (v ContextView) Validate() error {
	want, err := NewContextView(v.Snapshot)
	if err != nil {
		return err
	}
	if v.ContextRef != want.ContextRef {
		return fmt.Errorf("Context view reference is inconsistent")
	}
	return nil
}

type ContextList struct{ Items []ContextView }

func NewContextList(snapshots []ContextSnapshot) (ContextList, error) {
	if snapshots == nil {
		return ContextList{}, fmt.Errorf("Context collection is unknown")
	}
	items := make([]ContextView, len(snapshots))
	bindings := make([]tobari.ContextBinding, len(snapshots))
	templates := make([]tobari.WorkspaceTemplate, len(snapshots))
	workspaceIDs := make(map[tobari.WorkspaceID]struct{}, len(snapshots))
	for index, snapshot := range snapshots {
		view, err := NewContextView(snapshot)
		if err != nil {
			return ContextList{}, err
		}
		items[index] = view
		bindings[index] = view.Snapshot.Context
		templates[index] = view.Snapshot.Template
		if view.Snapshot.Workspace != nil {
			if _, exists := workspaceIDs[view.Snapshot.Workspace.ID]; exists {
				return ContextList{}, fmt.Errorf("Context collection contains a duplicate Workspace ID")
			}
			workspaceIDs[view.Snapshot.Workspace.ID] = struct{}{}
		}
	}
	if err := tobari.ValidateContextBindings(bindings); err != nil {
		return ContextList{}, fmt.Errorf("Context collection bindings are inconsistent: %w", err)
	}
	if err := tobari.ValidateWorkspaceTemplateAuthorities(templates); err != nil {
		return ContextList{}, fmt.Errorf("Context collection Templates are inconsistent: %w", err)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Snapshot.Context.ProjectRoot == items[j].Snapshot.Context.ProjectRoot {
			return items[i].Snapshot.Context.ID < items[j].Snapshot.Context.ID
		}
		return items[i].Snapshot.Context.ProjectRoot < items[j].Snapshot.Context.ProjectRoot
	})
	return ContextList{Items: items}, nil
}

type WorkspaceView struct {
	WorkspaceRef string
	Snapshot     ContextSnapshot
}

func NewWorkspaceView(snapshot ContextSnapshot) (WorkspaceView, error) {
	if err := snapshot.Validate(); err != nil {
		return WorkspaceView{}, err
	}
	if snapshot.Workspace == nil {
		return WorkspaceView{}, fmt.Errorf("Workspace view has no Workspace")
	}
	reference, err := tobari.WorkspaceRef(snapshot.Workspace.ID)
	if err != nil {
		return WorkspaceView{}, err
	}
	return WorkspaceView{WorkspaceRef: reference, Snapshot: snapshot.Clone()}, nil
}

func (v WorkspaceView) Validate() error {
	want, err := NewWorkspaceView(v.Snapshot)
	if err != nil {
		return err
	}
	if v.WorkspaceRef != want.WorkspaceRef {
		return fmt.Errorf("Workspace view reference is inconsistent")
	}
	return nil
}

type WorkspaceList struct{ Items []WorkspaceView }

func NewWorkspaceList(snapshots []ContextSnapshot) (WorkspaceList, error) {
	if snapshots == nil {
		return WorkspaceList{}, fmt.Errorf("Workspace collection is unknown")
	}
	items := make([]WorkspaceView, len(snapshots))
	workspaceIDs := make(map[tobari.WorkspaceID]struct{}, len(snapshots))
	contextIDs := make(map[tobari.ContextID]struct{}, len(snapshots))
	bindings := make([]tobari.ContextBinding, len(snapshots))
	templates := make([]tobari.WorkspaceTemplate, len(snapshots))
	for index, snapshot := range snapshots {
		view, err := NewWorkspaceView(snapshot)
		if err != nil {
			return WorkspaceList{}, err
		}
		if _, exists := workspaceIDs[view.Snapshot.Workspace.ID]; exists {
			return WorkspaceList{}, fmt.Errorf("Workspace collection contains a duplicate ID")
		}
		if _, exists := contextIDs[view.Snapshot.Context.ID]; exists {
			return WorkspaceList{}, fmt.Errorf("Workspace collection contains more than one Workspace for a Context")
		}
		items[index] = view
		bindings[index] = view.Snapshot.Context
		templates[index] = view.Snapshot.Template
		workspaceIDs[view.Snapshot.Workspace.ID] = struct{}{}
		contextIDs[view.Snapshot.Context.ID] = struct{}{}
	}
	if err := tobari.ValidateContextBindings(bindings); err != nil {
		return WorkspaceList{}, fmt.Errorf("Workspace collection Context bindings are inconsistent: %w", err)
	}
	if err := tobari.ValidateWorkspaceTemplateAuthorities(templates); err != nil {
		return WorkspaceList{}, fmt.Errorf("Workspace collection Templates are inconsistent: %w", err)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Snapshot.Workspace.ID < items[j].Snapshot.Workspace.ID })
	return WorkspaceList{Items: items}, nil
}

type TemplateSelectionResult = tobari.WorkspaceTemplateSelectionResult
type TemplateDeleteResult = tobari.WorkspaceTemplateDeleteResult
type ContextDeleteResult = tobari.ContextDeleteResult
type WorkspaceDeleteResult = tobari.WorkspaceAuthorityDeleteResult
type PolicyMemoryPublication = tobari.PolicyMemoryPublication
