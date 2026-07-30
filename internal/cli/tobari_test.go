package cli

import (
	"encoding/json"
	"reflect"
	"sort"
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
			ID: id, Name: "work", Root: "/tmp/work", Image: "workbench:dev",
			Running: true, Container: "tobari-work",
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
	if err != nil || string(empty) != "{\"schema_version\":2,\"tobari\":[]}\n" {
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

func TestClusterDenialsRendererClosesObservationAndActivationStep(t *testing.T) {
	t.Parallel()
	result := tobari.DenialReport{
		Task: tobari.TaskClusterDenials, PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines: 100,
		Items: []tobari.PolicyDenial{{
			Timestamp: "2026-07-30T10:41:11Z",
			RequestID: "7185da2688d7469aae9cd9068e920b0b",
			Host:      "api.github.com", Method: "GET", Path: "/repos/cli/cli",
			Reason: "request did not match an allow rule\nallow everything", StatusCode: 403,
		}},
	}
	textOutput, err := renderClusterDenials(result, "tobari policy apply", successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"policy: /tmp/config/tobari/policy",
		"host=api.github.com\tmethod=GET\tpath=/repos/cli/cli",
		`reason=request did not match an allow rule\nallow everything`,
		"apply_command: tobari policy apply",
	} {
		if !strings.Contains(string(textOutput), expected) {
			t.Fatalf("text output %q lacks %q", textOutput, expected)
		}
	}
	jsonOutput, err := renderClusterDenials(result, "tobari policy apply", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document clusterDenialsDocument
	if err := json.Unmarshal(jsonOutput, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || len(document.Denials.Items) != 1 ||
		document.Denials.ApplyCommand != "tobari policy apply" {
		t.Fatalf("JSON output = %+v", document)
	}
	var rawDocument map[string]json.RawMessage
	if err := json.Unmarshal(jsonOutput, &rawDocument); err != nil {
		t.Fatal(err)
	}
	var rawEnvelope map[string]json.RawMessage
	if err := json.Unmarshal(rawDocument["denials"], &rawEnvelope); err != nil {
		t.Fatal(err)
	}
	spec, found := DefaultCatalog().Lookup("cluster denials")
	if !found {
		t.Fatal("cluster denials is absent from the catalog")
	}
	gotFields := make([]string, 0, len(rawEnvelope))
	for name := range rawEnvelope {
		gotFields = append(gotFields, name)
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("denial JSON fields = %v, catalog = %v", gotFields, wantFields)
	}
}

func TestClusterDenialsRendererPreservesEmptyScopedCollection(t *testing.T) {
	t.Parallel()
	output, err := renderClusterDenials(
		tobari.DenialReport{
			Task: tobari.TaskClusterDenials, PolicyDirectory: "/tmp/config/tobari/policy",
			WindowLines: 200, Items: []tobari.PolicyDenial{},
		},
		"tobari policy apply", successFormatJSON,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"items":[]`) {
		t.Fatalf("empty denial output = %s", output)
	}
}

func TestAttachRejectsImageAndDevContainerTogether(t *testing.T) {
	t.Parallel()
	command, found := DefaultCatalog().Lookup("attach")
	if !found {
		t.Fatal("attach command is absent")
	}
	_, err := parseCommandInputs(command, []string{
		"--name", "work", "--root", "/tmp/work",
		"--image", "workbench:dev",
		"--devcontainer", ".devcontainer/devcontainer.json",
	})
	if err == nil {
		t.Fatal("conflicting image selectors were accepted")
	}
}
