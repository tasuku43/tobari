package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestPolicyRulesPathTemplateConformsAcrossTextJSONAndAgentHelp(t *testing.T) {
	rule := syntheticPathTemplatePolicyRule(t)
	runtime := &policyReviewRuntimeApplyingFake{policyReviewRuntimeFake: policyReviewRuntimeFake{
		state: tobari.State{PolicyDirectory: "/tmp/synthetic-policy"}, rules: []tobari.LearnedPolicyRule{rule},
	}}

	var textOut, textErr bytes.Buffer
	textCLI := newCLI(strings.NewReader(""), &textOut, &textErr, DefaultCatalog(), nil)
	textCLI.tobari = tobaricmd.New(runtime)
	if code := textCLI.RunContext(context.Background(), []string{"policy", "rules"}); code != ExitOK || textErr.Len() != 0 {
		t.Fatalf("policy rules text code=%d stdout=%q stderr=%q", code, textOut.String(), textErr.String())
	}
	for _, want := range []string{
		"/items/{id}", "path_template", "/items/123, /items/456", rule.ID,
		rule.WorkspaceManifestID, rule.ProjectID, "http", "unknown",
	} {
		if !strings.Contains(textOut.String(), want) {
			t.Fatalf("policy rules text %q lacks %q", textOut.String(), want)
		}
	}

	var jsonOut, jsonErr bytes.Buffer
	jsonCLI := newCLI(strings.NewReader(""), &jsonOut, &jsonErr, DefaultCatalog(), nil)
	jsonCLI.tobari = tobaricmd.New(runtime)
	if code := jsonCLI.RunContext(context.Background(), []string{"policy", "rules", "--format", "json"}); code != ExitOK || jsonErr.Len() != 0 {
		t.Fatalf("policy rules JSON code=%d stdout=%q stderr=%q", code, jsonOut.String(), jsonErr.String())
	}
	var document policyRulesDocument
	if err := json.Unmarshal(jsonOut.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || len(document.PolicyRules) != 1 {
		t.Fatalf("policy rules document = %+v", document)
	}
	item := document.PolicyRules[0]
	if item.ID != rule.ID || item.Decision != tobari.PolicyDecisionAllow || item.Match != tobari.PolicyMatchPathTemplate ||
		item.WorkspaceManifestID != rule.WorkspaceManifestID || item.WorkspaceID != rule.ProjectID ||
		item.Path != rule.Path || item.Protocol != tobari.PolicyProtocolHTTP || item.StateChange != tobari.PolicyStateChangeUnknown ||
		!reflect.DeepEqual(item.Examples, rule.Examples) || !reflect.DeepEqual(item.SourceCandidates, rule.SourceCandidates) {
		t.Fatalf("policy rule JSON item = %+v, rule = %+v", item, rule)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(jsonOut.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	var rawItems []map[string]json.RawMessage
	if err := json.Unmarshal(raw["policy_rules"], &rawItems); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"segments", "context", "context_id", "project_id", "instance_id"} {
		if _, found := rawItems[0][forbidden]; found {
			t.Fatalf("policy rule JSON exposed retired or internal field %q: %s", forbidden, jsonOut.String())
		}
	}

	var helpOut, helpErr bytes.Buffer
	helpCLI := newCLI(strings.NewReader(""), &helpOut, &helpErr, DefaultCatalog(), nil)
	if code := helpCLI.RunContext(context.Background(), []string{"help", "policy", "rules", "--format", "agent"}); code != ExitOK || helpErr.Len() != 0 {
		t.Fatalf("policy rules agent help code=%d stdout=%q stderr=%q", code, helpOut.String(), helpErr.String())
	}
	var help agentDocument
	if err := json.Unmarshal(helpOut.Bytes(), &help); err != nil {
		t.Fatal(err)
	}
	if len(help.Commands) != 1 {
		t.Fatalf("policy rules agent commands = %+v", help.Commands)
	}
	match := outputFieldByName(t, help.Commands[0].Contract.Output.Fields, "match")
	if !reflect.DeepEqual(match.Enum, []string{tobari.PolicyMatchExact, tobari.PolicyMatchPathTemplate}) {
		t.Fatalf("policy rules agent match enum = %v", match.Enum)
	}

	spec, found := DefaultCatalog().Lookup("policy rules")
	if !found {
		t.Fatal("policy rules is absent")
	}
	encodingFault := commandErrorByCode(t, spec.Agent.Errors, "output_encoding_failed")
	if encodingFault.Retryable || len(encodingFault.NextActions) != 1 || encodingFault.NextActions[0].Command != "version" {
		t.Fatalf("policy rules output encoding recovery = %+v", encodingFault)
	}
	if runtime.applyCalls != 0 || len(runtime.rules) != 1 {
		t.Fatalf("policy rules mutated state: calls=%d rules=%+v", runtime.applyCalls, runtime.rules)
	}
}

func TestPolicyDomainVocabulariesConformAtEveryCatalogProjection(t *testing.T) {
	t.Parallel()
	type projection struct {
		command string
		field   string
		want    []string
	}
	projections := []projection{
		{command: "policy apply-reviewed", field: "decisions[].decision", want: tobari.PolicyDecisionValues()},
		{command: "policy apply-reviewed", field: "decisions[].match", want: tobari.PolicyMatchValues()},
		{command: "policy apply-reviewed", field: "decisions[].protocol", want: tobari.PolicyProtocolValues()},
		{command: "policy apply-reviewed", field: "decisions[].state_change", want: tobari.PolicyStateChangeValues()},
		{command: "policy rules", field: "decision", want: tobari.PolicyDecisionValues()},
		{command: "policy rules", field: "match", want: tobari.PolicyMatchValues()},
		{command: "policy rules", field: "protocol", want: tobari.PolicyProtocolValues()},
		{command: "policy rules", field: "state_change", want: tobari.PolicyStateChangeValues()},
		{command: "policy reset", field: "decision", want: tobari.PolicyDecisionValues()},
		{command: "policy candidates", field: "protocol", want: tobari.PolicyProtocolValues()},
		{command: "policy candidates", field: "state_change", want: tobari.PolicyStateChangeValues()},
		{command: "review permissions", field: "protocol", want: tobari.PolicyProtocolValues()},
		{command: "review permissions", field: "state_change", want: tobari.PolicyStateChangeValues()},
		{command: "cluster denials", field: "items[].protocol", want: tobari.PolicyProtocolValues()},
		{command: "cluster denials", field: "items[].state_change", want: tobari.PolicyStateChangeValues()},
		{command: "policy allow", field: "protocol", want: tobari.PolicyProtocolValues()},
		{command: "policy allow", field: "state_change", want: tobari.PolicyStateChangeValues()},
		{command: "policy deny", field: "protocol", want: tobari.PolicyProtocolValues()},
		{command: "policy deny", field: "state_change", want: tobari.PolicyStateChangeValues()},
	}
	catalog := DefaultCatalog()
	for _, projection := range projections {
		projection := projection
		t.Run(strings.ReplaceAll(projection.command+"_"+projection.field, " ", "_"), func(t *testing.T) {
			spec, found := commandSpecByPath(catalog, projection.command)
			if !found {
				t.Fatalf("command %q is absent", projection.command)
			}
			field := catalogOutputFieldAtPath(t, spec.Agent.Output.Fields, projection.field)
			if !reflect.DeepEqual(field.Enum, projection.want) {
				t.Fatalf("%s %s enum = %v, want %v", projection.command, projection.field, field.Enum, projection.want)
			}
		})
	}
}

func commandSpecByPath(catalog Catalog, path string) (CommandSpec, bool) {
	for _, command := range catalog.commands {
		if command.Path == path {
			return command, true
		}
	}
	return CommandSpec{}, false
}

func syntheticPathTemplatePolicyRule(t *testing.T) tobari.LearnedPolicyRule {
	t.Helper()
	base := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-23T00:00:00Z", RequestID: "7185da2688d7469aae9cd9068e920b0b",
		WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789ad", WorkspaceManifestName: "synthetic",
		ProjectID: "01912345-6789-7abc-8def-0123456789ab", ProjectRoot: "/workspace/synthetic",
		Host: "api.example.com", Port: 443, Method: "GET", Path: "/items/123",
		Reason: "synthetic denial", StatusCode: 403, Learnable: true,
	}
	second := base
	second.Timestamp = "2026-08-23T00:01:00Z"
	second.RequestID = "8185da2688d7469aae9cd9068e920b0b"
	second.Path = "/items/456"
	candidates := make([]tobari.PolicyCandidate, 0, 2)
	for _, denial := range []tobari.PolicyDenial{base, second} {
		candidate, err := tobari.NewPolicyCandidate(denial)
		if err != nil {
			t.Fatal(err)
		}
		candidates = append(candidates, candidate)
	}
	items, err := tobari.PolicyReviewItems(candidates, []tobari.LearnedPolicyRule{})
	if err != nil || len(items) != 1 || items[0].Template == nil {
		t.Fatalf("template items = %+v, err = %v", items, err)
	}
	rule, err := tobari.NewPathTemplateLearnedPolicyRule(*items[0].Template)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func outputFieldByName(t *testing.T, fields []OutputField, name string) OutputField {
	t.Helper()
	for _, field := range fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("output field %q is absent", name)
	return OutputField{}
}

func commandErrorByCode(t *testing.T, errors []CommandError, code string) CommandError {
	t.Helper()
	for _, declared := range errors {
		if declared.Code == code {
			return declared
		}
	}
	t.Fatalf("command error %q is absent", code)
	return CommandError{}
}

func catalogOutputFieldAtPath(t *testing.T, fields []OutputField, path string) OutputField {
	t.Helper()
	parts := strings.Split(path, ".")
	for index, part := range parts {
		array := strings.HasSuffix(part, "[]")
		name := strings.TrimSuffix(part, "[]")
		field := outputFieldByName(t, fields, name)
		if index == len(parts)-1 {
			if array {
				t.Fatalf("terminal field path %q names an array item without a child", path)
			}
			return field
		}
		if array {
			if field.Type != OutputFieldTypeArray || field.Items == nil || field.Items.Type != OutputFieldTypeObject {
				t.Fatalf("field path %q does not traverse an object array at %q", path, part)
			}
			fields = field.Items.Fields
			continue
		}
		if field.Type != OutputFieldTypeObject {
			t.Fatalf("field path %q does not traverse an object at %q", path, part)
		}
		fields = field.Fields
	}
	t.Fatalf("output field path %q is empty", path)
	return OutputField{}
}
