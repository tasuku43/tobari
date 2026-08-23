package tobari

import "fmt"

const TaskOperatorConsoleSnapshot = "serve.snapshot"

// OperatorConsoleSnapshot is the complete typed observation rendered by one
// browser refresh. It preserves each child task identity and collection scope;
// presentation must not reconstruct relationships from labels or order.
type OperatorConsoleSnapshot struct {
	Task        string              `json:"task"`
	Cluster     ClusterStatus       `json:"cluster"`
	Workspaces  WorkspaceListResult `json:"workspaces"`
	WindowLines int                 `json:"window_lines"`
	ReviewItems []PolicyReviewItem  `json:"review_items"`
	Rules       PolicyRuleReport    `json:"rules"`
}

func (s OperatorConsoleSnapshot) Validate() error {
	if s.Task != TaskOperatorConsoleSnapshot {
		return fmt.Errorf("operator console snapshot task identity is invalid")
	}
	if err := s.Cluster.Validate(); err != nil || s.Cluster.Task != TaskClusterStatus {
		return fmt.Errorf("operator console cluster snapshot is invalid")
	}
	if !s.Cluster.Configured || !s.Cluster.Running {
		return fmt.Errorf("operator console requires a ready cluster")
	}
	if err := s.Workspaces.Validate(); err != nil || s.Workspaces.Task != TaskWorkspaceList {
		return fmt.Errorf("operator console Workspace snapshot is invalid")
	}
	if s.WindowLines < 1 || s.ReviewItems == nil {
		return fmt.Errorf("operator console review scope is invalid")
	}
	seen := make(map[string]struct{}, len(s.ReviewItems))
	for _, item := range s.ReviewItems {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("operator console review item is invalid: %w", err)
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return fmt.Errorf("operator console review item IDs must be unique")
		}
		seen[item.ID] = struct{}{}
	}
	if err := s.Rules.Validate(); err != nil || s.Rules.Task != TaskPolicyRules {
		return fmt.Errorf("operator console policy rule snapshot is invalid")
	}
	return nil
}
