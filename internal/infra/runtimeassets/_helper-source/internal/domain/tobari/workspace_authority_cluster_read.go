package tobari

import (
	"errors"
	"fmt"
)

const FinalClusterDenialSchemaVersion = 5

var (
	ErrFinalClusterNotRunning         = errors.New("final cluster is not running with exact active authority")
	ErrFinalClusterObservationChanged = errors.New("final cluster observation changed during the read")
)

// FinalClusterDenial binds one bounded Gateway denial to the complete final
// Context, Template, and Workspace authority that owned the observed request.
// The retained PolicyDenial is private validation/effect evidence; public
// projections use the final owner fields below rather than predecessor names.
type FinalClusterDenial struct {
	ContextID           ContextID           `json:"context_id"`
	WorkspaceTemplateID WorkspaceTemplateID `json:"workspace_template_id"`
	TemplateName        string              `json:"template_name"`
	WorkspaceID         WorkspaceID         `json:"workspace_id"`
	ProjectRoot         string              `json:"project_root"`
	Denial              PolicyDenial        `json:"-"`
}

func (d FinalClusterDenial) Validate() error {
	if err := d.ContextID.Validate(); err != nil {
		return err
	}
	if err := d.WorkspaceTemplateID.Validate(); err != nil {
		return err
	}
	if err := ValidateName(d.TemplateName); err != nil {
		return err
	}
	if err := d.WorkspaceID.Validate(); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(d.ProjectRoot); err != nil {
		return err
	}
	if err := d.Denial.Validate(); err != nil {
		return err
	}
	if d.Denial.WorkspaceManifestID != string(d.ContextID) ||
		d.Denial.WorkspaceManifestName != d.TemplateName ||
		d.Denial.ProjectID != string(d.WorkspaceID) || d.Denial.ProjectRoot != d.ProjectRoot {
		return fmt.Errorf("final cluster denial does not match its owner authority")
	}
	return nil
}

type FinalClusterDenialWindow struct {
	SchemaVersion int    `json:"schema_version"`
	Task          string `json:"task"`
	PolicyProjectionIdentity
	WindowLines   int                  `json:"window_lines"`
	UnparsedLines int                  `json:"unparsed_lines"`
	Items         []FinalClusterDenial `json:"items"`
}

func NewFinalClusterDenialWindow(collection WorkspaceAuthorityCollection, tail int, read DenialRead, identity PolicyProjectionIdentity) (FinalClusterDenialWindow, error) {
	if err := collection.Validate(); err != nil {
		return FinalClusterDenialWindow{}, err
	}
	if err := identity.Validate(); err != nil {
		return FinalClusterDenialWindow{}, fmt.Errorf("final cluster denial projection identity is invalid: %w", err)
	}
	if tail < 1 {
		return FinalClusterDenialWindow{}, fmt.Errorf("final cluster denial window is invalid")
	}
	if err := read.Validate(); err != nil {
		return FinalClusterDenialWindow{}, fmt.Errorf("final cluster denial window is invalid")
	}
	templates := make(map[WorkspaceTemplateID]WorkspaceTemplate, len(collection.Templates))
	for _, template := range collection.Templates {
		templates[template.ID] = template
	}
	contexts := make(map[ContextID]ContextBinding, len(collection.Contexts))
	for _, record := range collection.Contexts {
		contexts[record.Context.ID] = record.Context
	}
	workspaces := make(map[WorkspaceID]WorkspaceBinding, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaces[workspace.ID] = workspace
	}
	result := FinalClusterDenialWindow{SchemaVersion: FinalClusterDenialSchemaVersion, Task: TaskClusterDenials, PolicyProjectionIdentity: identity, WindowLines: tail, UnparsedLines: read.UnparsedLines, Items: make([]FinalClusterDenial, len(read.Items))}
	for index, denial := range read.Items {
		contextID, err := ParseContextID(denial.WorkspaceManifestID)
		if err != nil {
			return FinalClusterDenialWindow{}, fmt.Errorf("denial Context authority is invalid: %w", err)
		}
		workspaceID, err := ParseWorkspaceID(denial.ProjectID)
		if err != nil {
			return FinalClusterDenialWindow{}, fmt.Errorf("denial Workspace authority is invalid: %w", err)
		}
		contextBinding, contextFound := contexts[contextID]
		workspace, workspaceFound := workspaces[workspaceID]
		template, templateFound := templates[contextBinding.TemplateID]
		if !contextFound || !workspaceFound || !templateFound || workspace.ContextID != contextID || workspace.ProjectRoot != contextBinding.ProjectRoot {
			return FinalClusterDenialWindow{}, fmt.Errorf("denial owner is absent from final authority")
		}
		item := FinalClusterDenial{ContextID: contextID, WorkspaceTemplateID: template.ID, TemplateName: template.Name, WorkspaceID: workspaceID, ProjectRoot: contextBinding.ProjectRoot, Denial: denial}
		if err := item.Validate(); err != nil {
			return FinalClusterDenialWindow{}, err
		}
		result.Items[index] = item
	}
	return result, result.Validate()
}

func (w FinalClusterDenialWindow) Validate() error {
	if w.SchemaVersion != FinalClusterDenialSchemaVersion || w.Task != TaskClusterDenials || w.WindowLines < 1 || w.UnparsedLines < 0 || w.Items == nil {
		return fmt.Errorf("final cluster denial window is invalid")
	}
	if err := w.PolicyProjectionIdentity.Validate(); err != nil {
		return fmt.Errorf("final cluster denial projection identity is invalid: %w", err)
	}
	for _, item := range w.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}
