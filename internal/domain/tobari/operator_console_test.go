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
			ManifestCount: 1, PolicyRevision: strings.Repeat("a", 64),
			EvaluatorIdentity:  PolicyEvaluatorIdentity{SchemaVersion: 1, Version: "tobari-evaluator-v1", Digest: authorityDigest("b")},
			PolicyDataIdentity: PolicyDataIdentity{SchemaVersion: 1, Digest: authorityDigest("c")},
			PolicyProjection:   "valid", PrincipalRegistry: "valid", GatewayProjection: "valid",
			Components: []ComponentStatus{
				{Name: "gateway", State: "running", Health: "healthy"},
				{Name: "opa", State: "running", Health: "healthy"},
			},
		},
		Workspaces:  WorkspaceListResult{Task: TaskWorkspaceList, Items: []WorkspaceListItem{}},
		WindowLines: 10_000,
		ReviewItems: []PolicyReviewItem{},
		Rules: PolicyRuleReport{
			Task: TaskPolicyRules, PolicyProjectionIdentity: validPolicyProjectionIdentity(), Items: []PolicyRule{},
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
