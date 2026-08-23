package tobari

import (
	"strings"
	"testing"
)

func validOperatorConsoleSnapshotFixture() OperatorConsoleSnapshot {
	return OperatorConsoleSnapshot{
		Task: TaskOperatorConsoleSnapshot,
		Cluster: ClusterStatus{
			Task: TaskClusterStatus, Configured: true, Running: true,
			Policy: "/tmp/policy", ManifestCount: 1, PolicyRevision: strings.Repeat("a", 64),
			PolicyProjection: "valid", PrincipalRegistry: "valid", GatewayProjection: "valid",
			Components: []ComponentStatus{
				{Name: "gateway", State: "running", Health: "healthy"},
				{Name: "opa", State: "running", Health: "healthy"},
			},
		},
		Workspaces:  WorkspaceListResult{Task: TaskWorkspaceList, Items: []WorkspaceListItem{}},
		WindowLines: 10_000,
		ReviewItems: []PolicyReviewItem{},
		Rules: PolicyRuleReport{
			Task: TaskPolicyRules, PolicyDirectory: "/tmp/policy", Items: []PolicyRule{},
		},
	}
}

func TestOperatorConsoleSnapshotPreservesKnownEmptyScopes(t *testing.T) {
	t.Parallel()
	if err := validOperatorConsoleSnapshotFixture().Validate(); err != nil {
		t.Fatalf("valid empty snapshot rejected: %v", err)
	}
}

func TestOperatorConsoleSnapshotRejectsUnknownReviewAndUnreadyCluster(t *testing.T) {
	t.Parallel()
	unknown := validOperatorConsoleSnapshotFixture()
	unknown.ReviewItems = nil
	if err := unknown.Validate(); err == nil {
		t.Fatal("unknown review collection was accepted")
	}
	unready := validOperatorConsoleSnapshotFixture()
	unready.Cluster.Running = false
	if err := unready.Validate(); err == nil {
		t.Fatal("unready cluster was accepted")
	}
}
