package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestDefaultCatalogPublishesNamedTobariReferenceFlow(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	discover, found := catalog.Lookup("list")
	if !found {
		t.Fatal("list command is absent")
	}
	wantProduced := []ProducedRef{{Kind: tobari.ReferenceKind, Field: "id"}}
	if discover.Role != RoleDiscover || !reflect.DeepEqual(discover.ProducedRefs(), wantProduced) {
		t.Fatalf("list reference contract = %+v", discover.ProducedRefs())
	}
	for _, path := range []string{"shell", "exec", "logs", "detach"} {
		command, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("%s command is absent", path)
		}
		wantConsumed := []ConsumedRef{{Kind: tobari.ReferenceKind, Argument: "--id"}}
		if command.Role != RoleAct || !reflect.DeepEqual(command.ConsumedRefs(), wantConsumed) {
			t.Errorf("%s reference contract = %+v", path, command.ConsumedRefs())
		}
	}
	attach, found := catalog.Lookup("attach")
	if !found || attach.Agent.FixedTarget == nil ||
		attach.Agent.FixedTarget.Kind != tobari.ClusterTargetKind ||
		len(attach.ConsumedRefs()) != 0 {
		t.Fatalf("attach fixed target = %+v", attach.Agent.FixedTarget)
	}
}

func TestTobariListRendererPreservesOpaqueIDAndEmptyScope(t *testing.T) {
	t.Parallel()
	id := "tbr_0123456789abcdef0123456789abcdef"
	result := tobari.ListResult{
		Task: tobari.TaskList,
		Items: []tobari.ItemStatus{{
			ID: id, Name: "work", Root: "/tmp/work", Running: true, Container: "tobari-work",
		}},
	}
	output, err := renderTobariList(result, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document tobariListDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Tobari) != 1 || document.Tobari[0].ID != id {
		t.Fatalf("list output = %+v", document)
	}
	empty, err := renderTobariList(
		tobari.ListResult{Task: tobari.TaskList, Items: []tobari.ItemStatus{}},
		successFormatJSON,
	)
	if err != nil || string(empty) != "{\"schema_version\":1,\"tobari\":[]}\n" {
		t.Fatalf("empty list = %q, error = %v", empty, err)
	}
}

func TestClusterStatusRendererExposesXDGPolicyAndTobariCount(t *testing.T) {
	t.Parallel()
	status := tobari.ClusterStatus{
		Task: tobari.TaskClusterStatus, Configured: true, Running: true,
		Proxy: "http://gateway:8080", Policy: "/tmp/config/tobari/policy",
		TobariCount: 2, Components: []tobari.ComponentStatus{},
	}
	output := string(renderClusterStatusText(status))
	for _, expected := range []string{
		"policy: /tmp/config/tobari/policy", "tobari_count: 2", "running: true",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("status output %q lacks %q", output, expected)
		}
	}
}
