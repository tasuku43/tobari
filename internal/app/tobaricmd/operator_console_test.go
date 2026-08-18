package tobaricmd

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestOperatorConsoleSnapshotComposesTypedReadTasks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime := &projectRuntimeFake{
		fakeRuntime: &fakeRuntime{state: testState(root)},
		cwd:         root,
	}
	result, err := New(runtime).OperatorConsoleSnapshot(context.Background(), 200)
	if err != nil {
		t.Fatal(err)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	if result.Task != tobari.TaskOperatorConsoleSnapshot ||
		result.Cluster.Task != tobari.TaskClusterStatus ||
		result.Workspaces.Task != tobari.TaskProjectList ||
		result.Rules.Task != tobari.TaskPolicyRules || result.WindowLines != 200 {
		t.Fatalf("snapshot task composition = %+v", result)
	}
	if result.Workspaces.Items == nil || result.ReviewItems == nil || result.Rules.Items == nil {
		t.Fatalf("known empty scopes were lost: %+v", result)
	}
}
