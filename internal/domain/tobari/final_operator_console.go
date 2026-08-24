package tobari

import "fmt"

// FinalOperatorConsoleWorkspace is the semantic Workspace row shown by the
// research-only local console. It carries no Docker selector or predecessor
// Manifest identity.
type FinalOperatorConsoleWorkspace struct {
	WorkspaceID WorkspaceID `json:"workspace_id"`
	ContextID   ContextID   `json:"context_id"`
	Context     string      `json:"context"`
	ProjectRoot string      `json:"project_root"`
	Applied     bool        `json:"applied"`
}

func (w FinalOperatorConsoleWorkspace) Validate() error {
	if w.WorkspaceID.Validate() != nil || w.ContextID.Validate() != nil || ValidateName(w.Context) != nil || ValidateCanonicalRoot(w.ProjectRoot) != nil {
		return fmt.Errorf("final operator-console Workspace is invalid")
	}
	return nil
}

// FinalOperatorConsoleSnapshot joins one final cluster observation with one
// coherent Permission Inbox envelope. The collection receipt must be exact;
// the browser never reconstructs owner relationships from presentation rows.
type FinalOperatorConsoleSnapshot struct {
	Task       string                          `json:"task"`
	Cluster    FinalClusterStatus              `json:"cluster"`
	Workspaces []FinalOperatorConsoleWorkspace `json:"workspaces"`
	Review     PolicyMemoryReviewSnapshot      `json:"review"`
}

func NewFinalOperatorConsoleSnapshot(cluster FinalClusterStatus, review PolicyMemoryReviewSnapshot) (FinalOperatorConsoleSnapshot, error) {
	result := FinalOperatorConsoleSnapshot{Task: TaskOperatorConsoleSnapshot, Cluster: cluster, Workspaces: []FinalOperatorConsoleWorkspace{}, Review: review.Clone()}
	if review.CollectionPresent {
		contextNames := map[ContextID]string{}
		templateNames := map[WorkspaceTemplateID]string{}
		for _, template := range review.Collection.Templates {
			templateNames[template.ID] = template.Name
		}
		for _, record := range review.Collection.Contexts {
			contextNames[record.Context.ID] = templateNames[record.Context.TemplateID]
		}
		for _, workspace := range review.Collection.Workspaces {
			result.Workspaces = append(result.Workspaces, FinalOperatorConsoleWorkspace{WorkspaceID: workspace.ID, ContextID: workspace.ContextID, Context: contextNames[workspace.ContextID], ProjectRoot: workspace.ProjectRoot, Applied: workspace.LastSuccessfulEntry != nil})
		}
	}
	return result, result.Validate()
}

func (s FinalOperatorConsoleSnapshot) Validate() error {
	if s.Task != TaskOperatorConsoleSnapshot || s.Workspaces == nil {
		return fmt.Errorf("final operator-console snapshot metadata is invalid")
	}
	if err := s.Cluster.Validate(); err != nil {
		return fmt.Errorf("final operator-console cluster is invalid: %w", err)
	}
	if err := s.Review.Validate(); err != nil {
		return fmt.Errorf("final operator-console Permission Inbox is invalid: %w", err)
	}
	if s.Cluster.Authority == FinalClusterAuthorityPresent {
		if !s.Review.CollectionPresent || s.Cluster.Generation != s.Review.CollectionGeneration || s.Cluster.CollectionRevision != s.Review.CollectionRevision {
			return fmt.Errorf("final operator-console observations are not coherent")
		}
	} else if s.Review.CollectionPresent {
		return fmt.Errorf("final operator-console cluster lacks selected authority")
	}
	for _, workspace := range s.Workspaces {
		if err := workspace.Validate(); err != nil {
			return err
		}
	}
	return nil
}
