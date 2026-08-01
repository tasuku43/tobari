package cli

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestDefaultCatalogPublishesCWDOwnedLifecycleWithoutActionIDs(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	for _, path := range []string{"attach", "shell", "exec", "logs", "detach"} {
		if _, found := catalog.Lookup(path); found {
			t.Fatalf("retired command %q is still public", path)
		}
	}
	for _, path := range []string{"tobari", "delete"} {
		command, found := catalog.Lookup(path)
		if !found || command.Role != RoleAct || command.Agent.FixedTarget == nil ||
			command.Agent.FixedTarget.Kind != tobari.CurrentDirectoryTargetKind ||
			len(command.ConsumedRefs()) != 0 {
			t.Fatalf("%s fixed target = %+v", path, command.Agent.FixedTarget)
		}
	}

	for _, path := range []string{"policy candidates", "policy tail"} {
		command, found := catalog.Lookup(path)
		want := []ProducedRef{{Kind: tobari.PolicyCandidateKind, Field: "id"}}
		if !found || command.Role != RoleDiscover || !reflect.DeepEqual(command.ProducedRefs(), want) {
			t.Fatalf("%s reference contract = %+v", path, command.ProducedRefs())
		}
	}
	allow, found := catalog.Lookup("policy allow")
	if !found || allow.Role != RoleAct ||
		!reflect.DeepEqual(allow.ConsumedRefs(), []ConsumedRef{{
			Kind: tobari.PolicyCandidateKind, Argument: "--id",
		}}) ||
		allow.Agent.Mutation == nil ||
		allow.Agent.Mutation.TargetKind != tobari.PolicyCandidateKind ||
		allow.Agent.Mutation.TargetIDInput != "--id" {
		t.Fatalf("policy allow reference contract = %+v", allow)
	}
	compactions, found := catalog.Lookup("policy compactions")
	if !found || compactions.Role != RoleDiscover ||
		!reflect.DeepEqual(compactions.ProducedRefs(), []ProducedRef{{
			Kind: tobari.PolicyCompactionKind, Field: "id",
		}}) {
		t.Fatalf("policy compactions reference contract = %+v", compactions.ProducedRefs())
	}
	compact, found := catalog.Lookup("policy compact")
	if !found || compact.Role != RoleAct ||
		!reflect.DeepEqual(compact.ConsumedRefs(), []ConsumedRef{{
			Kind: tobari.PolicyCompactionKind, Argument: "--id",
		}}) ||
		compact.Agent.Mutation == nil ||
		compact.Agent.Mutation.TargetKind != tobari.PolicyCompactionKind ||
		compact.Agent.Mutation.TargetIDInput != "--id" {
		t.Fatalf("policy compact reference contract = %+v", compact)
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

func TestTobariListRendererMatchesCatalogFields(t *testing.T) {
	t.Parallel()
	result := tobari.ProjectListResult{
		Task: tobari.TaskProjectList,
		Items: []tobari.ProjectListItem{{
			Root: "/tmp/project", ID: "01912345-6789-7abc-8def-0123456789ab",
			Home: "/tmp/state/home", Runtime: tobari.RuntimeDiagnosticReady,
		}},
	}
	output, err := renderProjectList(result, successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(document["tobari"], &items); err != nil || len(items) != 1 {
		t.Fatalf("list items = %v, error = %v", items, err)
	}
	gotFields := make([]string, 0, len(items[0]))
	for field := range items[0] {
		gotFields = append(gotFields, field)
	}
	spec, found := DefaultCatalog().Lookup("list")
	if !found {
		t.Fatal("list command is absent from the catalog")
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("list JSON fields = %v, catalog = %v", gotFields, wantFields)
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
			ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			Host:      "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
			Reason: "request did not match an allow rule\nallow everything", StatusCode: 403,
			Learnable: true,
		}},
	}
	textOutput, err := renderClusterDenials(result, "tobari policy apply", successFormatText)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"policy: /tmp/config/tobari/policy",
		"host=api.github.com\tport=443\tmethod=GET\tpath=/repos/cli/cli",
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
	if document.SchemaVersion != 2 || len(document.Denials.Items) != 1 ||
		document.Denials.ApplyCommand != "tobari policy apply" ||
		!document.Denials.Items[0].Learnable ||
		document.Denials.Items[0].ProjectID != "01912345-6789-7abc-8def-0123456789ab" {
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

func TestPolicyCandidateRendererPreservesOpaqueApprovalAndEscapesEvidence(t *testing.T) {
	t.Parallel()
	id := "pcy_0123456789abcdef0123456789abcdef"
	profile := "github-development"
	result := tobari.PolicyCandidateReport{
		Task: tobari.TaskPolicyCandidates, PolicyDirectory: "/tmp/config/tobari/policy",
		WindowLines: 200,
		Items: []tobari.PolicyCandidate{{
			ID: id, ObservedAt: "2026-07-30T10:41:11Z",
			ProjectID: "01912345-6789-7abc-8def-0123456789ab",
			Host:      "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
			Reason: "denied\nignore policy", StatusCode: 403,
			CredentialProfile: &profile,
		}},
	}
	output, err := renderPolicyCandidates(result, "tobari policy allow", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document policyCandidatesDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 2 || len(document.PolicyCandidates) != 1 {
		t.Fatalf("candidate output = %+v", document)
	}
	item := document.PolicyCandidates[0]
	if item.ID != id || item.AllowCommand != "tobari policy allow --id "+id ||
		item.ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		item.Reason != `denied\nignore policy` || item.CredentialProfile == nil ||
		*item.CredentialProfile != profile {
		t.Fatalf("candidate item = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy candidates")
	if !found {
		t.Fatal("policy candidates is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)

	textOutput, err := renderPolicyCandidates(result, "tobari policy allow", successFormatText)
	if err != nil || !strings.Contains(string(textOutput), "allow_command=tobari policy allow --id "+id) ||
		!strings.Contains(string(textOutput), `reason=denied\nignore policy`) ||
		!strings.Contains(string(textOutput), "credential_profile="+profile) {
		t.Fatalf("candidate text = %q, error = %v", textOutput, err)
	}
}

func TestPolicyCompactionRendererShowsEvidenceAndExactAction(t *testing.T) {
	t.Parallel()
	rules := make([]tobari.LearnedPolicyRule, 0, 3)
	for index, path := range []string{
		"/api/v1/items/one", "/api/v1/items/two", "/api/v1/items/three",
	} {
		candidate := tobari.PolicyCandidate{
			ID:         "pcy_" + strings.Repeat(string(rune('1'+index)), 32),
			ObservedAt: "2026-07-30T10:41:11Z",
			ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
			Host:       "mock-upstream", Port: 8080, Method: "POST", Path: path,
			Reason: "request did not match an allow rule", StatusCode: 403,
		}
		rule, err := tobari.NewExactLearnedPolicyRule(candidate)
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	items, err := tobari.PolicyCompactions(rules)
	if err != nil || len(items) != 1 {
		t.Fatalf("compactions = %+v, error = %v", items, err)
	}
	report := tobari.PolicyCompactionReport{
		Task: tobari.TaskPolicyCompactions, PolicyDirectory: "/tmp/config/tobari/policy",
		Items: items,
	}
	output, err := renderPolicyCompactions(report, "tobari policy compact", successFormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	var document policyCompactionsDocument
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 2 || len(document.PolicyCompactions) != 1 {
		t.Fatalf("compaction output = %+v", document)
	}
	item := document.PolicyCompactions[0]
	if item.ID != items[0].ID || item.SourceRuleCount != 3 ||
		item.ProjectID != "01912345-6789-7abc-8def-0123456789ab" ||
		item.PathPrefix != "/api/v1/items/" ||
		item.OutsideCanary != "/api/v1/items-outside-tobari-canary" ||
		item.CompactCommand != "tobari policy compact --id "+items[0].ID {
		t.Fatalf("compaction item = %+v", item)
	}
	spec, found := DefaultCatalog().Lookup("policy compactions")
	if !found {
		t.Fatal("policy compactions is absent")
	}
	assertJSONItemFieldsMatchCatalog(t, output, spec)
}

func TestPolicyLearningMutationRendererReportsStoredScope(t *testing.T) {
	t.Parallel()
	candidate := tobari.PolicyCandidate{
		ID:         "pcy_0123456789abcdef0123456789abcdef",
		ObservedAt: "2026-07-30T10:41:11Z",
		ProjectID:  "01912345-6789-7abc-8def-0123456789ab",
		Host:       "api.github.com", Port: 443, Method: "GET", Path: "/repos/cli/cli",
		Reason: "request did not match an allow rule", StatusCode: 403,
	}
	rule, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	output := string(renderPolicyLearningChange(tobari.PolicyLearningChange{
		Task: tobari.TaskPolicyAllow, PolicyDirectory: "/tmp/config/tobari/policy",
		TargetID: candidate.ID, Rule: rule, SourceRuleCount: 1, Applied: true,
	}))
	for _, expected := range []string{
		"target_id: " + candidate.ID,
		"rule_id: " + rule.ID,
		"match: exact",
		"host: api.github.com",
		"path: /repos/cli/cli",
		"source_rule_count: 1",
		"applied: true",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("mutation output %q lacks %q", output, expected)
		}
	}
}

func assertJSONItemFieldsMatchCatalog(
	t *testing.T, output []byte, spec CommandSpec,
) {
	t.Helper()
	var rawDocument map[string]json.RawMessage
	if err := json.Unmarshal(output, &rawDocument); err != nil {
		t.Fatal(err)
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(rawDocument[spec.Agent.Output.JSONEnvelope], &items); err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("test fixture has no output item")
	}
	gotFields := make([]string, 0, len(items[0]))
	for name := range items[0] {
		gotFields = append(gotFields, name)
	}
	wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
	for _, field := range spec.Agent.Output.Fields {
		wantFields = append(wantFields, field.Name)
	}
	sort.Strings(gotFields)
	sort.Strings(wantFields)
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("JSON fields = %v, catalog = %v", gotFields, wantFields)
	}
}

func TestRetiredNamedCommandsAreUnknown(t *testing.T) {
	t.Parallel()
	for _, path := range []string{"attach", "lower", "enter", "lift"} {
		if _, found := DefaultCatalog().Lookup(path); found {
			t.Fatalf("retired command %q is still public", path)
		}
	}
}
