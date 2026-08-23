package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/tobaricmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type catalogDomainOutputConformanceFixture struct {
	SchemaVersion      int `json:"schema_version"`
	TemplateCandidates struct {
		ObservedAt          []string `json:"observed_at"`
		RequestIDs          []string `json:"request_ids"`
		WorkspaceManifestID string   `json:"workspace_manifest_id"`
		WorkspaceManifest   string   `json:"workspace_manifest"`
		WorkspaceID         string   `json:"workspace_id"`
		ProjectRoot         string   `json:"project_root"`
		Scheme              string   `json:"scheme"`
		Host                string   `json:"host"`
		Port                int      `json:"port"`
		Method              string   `json:"method"`
		Paths               []string `json:"paths"`
	} `json:"template_candidates"`
}

type catalogDomainOutputConformanceAnswer struct {
	SchemaVersion           int      `json:"schema_version"`
	PolicyMatchValues       []string `json:"policy_match_values"`
	PolicyProtocolValues    []string `json:"policy_protocol_values"`
	PolicyDecisionValues    []string `json:"policy_decision_values"`
	PolicyStateChangeValues []string `json:"policy_state_change_values"`
	Template                struct {
		Match               string   `json:"match"`
		Protocol            string   `json:"protocol"`
		StateChange         string   `json:"state_change"`
		Path                string   `json:"path"`
		Examples            []string `json:"examples"`
		EmptyProtocolFields []string `json:"empty_protocol_fields"`
	} `json:"template"`
	ProducedReferencePaths                []string `json:"produced_reference_paths"`
	CopySchemaForbiddenFields             []string `json:"copy_schema_forbidden_fields"`
	RetiredIdentityFields                 []string `json:"retired_identity_fields"`
	RoutineSuccessExternalProcessingCount int      `json:"routine_success_external_processing_count"`
}

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

func TestCatalogDomainOutputConformanceCorpusMatchesCanonicalSources(t *testing.T) {
	answer := readCatalogDomainOutputConformanceAnswer(t)
	if answer.SchemaVersion != 1 || answer.RoutineSuccessExternalProcessingCount != 0 {
		t.Fatalf("conformance answer metadata = %+v", answer)
	}
	for name, gotWant := range map[string]struct{ got, want []string }{
		"match":        {tobari.PolicyMatchValues(), answer.PolicyMatchValues},
		"protocol":     {tobari.PolicyProtocolValues(), answer.PolicyProtocolValues},
		"decision":     {tobari.PolicyDecisionValues(), answer.PolicyDecisionValues},
		"state_change": {tobari.PolicyStateChangeValues(), answer.PolicyStateChangeValues},
	} {
		if !reflect.DeepEqual(gotWant.got, gotWant.want) {
			t.Fatalf("%s values = %v, want %v", name, gotWant.got, gotWant.want)
		}
	}

	rule := syntheticPathTemplatePolicyRule(t)
	domainRule, err := tobari.NewPolicyRuleFromLearned(rule)
	if err != nil {
		t.Fatal(err)
	}
	items := policyRuleOutputs(tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: "/workspace/synthetic-policy", Items: []tobari.PolicyRule{domainRule},
	}, "tobari policy reset")
	if len(items) != 1 {
		t.Fatalf("policy rule outputs = %+v", items)
	}
	item := items[0]
	if item.Match != answer.Template.Match || item.Protocol != answer.Template.Protocol ||
		item.StateChange != answer.Template.StateChange || item.Path != answer.Template.Path ||
		!reflect.DeepEqual(item.Examples, answer.Template.Examples) {
		t.Fatalf("template output = %+v, answer = %+v", item, answer.Template)
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, name := range answer.Template.EmptyProtocolFields {
		if value, exists := fields[name]; !exists || value != "" {
			t.Fatalf("template protocol field %q = %#v, exists=%t", name, value, exists)
		}
	}
	for _, name := range answer.RetiredIdentityFields {
		if _, exists := fields[name]; exists {
			t.Fatalf("template output contains retired identity field %q", name)
		}
	}

	produced := recursiveReferenceProducerSpec().ProducedRefs()
	paths := make([]string, len(produced))
	for index, reference := range produced {
		paths[index] = reference.Field
	}
	if !reflect.DeepEqual(paths, answer.ProducedReferencePaths) {
		t.Fatalf("produced reference paths = %v, want %v", paths, answer.ProducedReferencePaths)
	}

	catalog := DefaultCatalog()
	for _, path := range []string{"manifest create", "runtime create"} {
		spec, found := catalog.Lookup(path)
		if !found {
			t.Fatalf("copy command %q is absent", path)
		}
		if strings.Contains(spec.Args, "--base") {
			t.Fatalf("copy command %q retains --base: %q", path, spec.Args)
		}
		forbidden := make(map[string]struct{}, len(answer.CopySchemaForbiddenFields))
		for _, name := range answer.CopySchemaForbiddenFields {
			forbidden[name] = struct{}{}
		}
		for _, name := range catalogOutputFieldNames(spec.Agent.Output.Fields) {
			if _, exists := forbidden[name]; exists {
				t.Fatalf("copy command %q output contains provenance field %q", path, name)
			}
		}
		for _, input := range spec.Agent.Inputs {
			if input.Name == "--base" {
				t.Fatalf("copy command %q retains --base input", path)
			}
		}
	}
}

func TestPolicyRulesExactAllowDenyAndEmptyReportsConform(t *testing.T) {
	denial := syntheticPolicyDenial(t, tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		"7185da2688d7469aae9cd9068e920b0b", "/items/123")
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	allow, err := tobari.NewExactLearnedPolicyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	deny, err := tobari.NewExactPolicyDenyRule(candidate)
	if err != nil {
		t.Fatal(err)
	}
	items, err := tobari.CurrentPolicyRules([]tobari.LearnedPolicyRule{allow}, []tobari.PolicyDenyRule{deny})
	if err != nil {
		t.Fatal(err)
	}
	report := tobari.PolicyRuleReport{Task: tobari.TaskPolicyRules, PolicyDirectory: "/workspace/synthetic-policy", Items: items}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := renderPolicyRulesWithCommands(report, "tobari policy reset", successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document policyRulesDocument
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || len(document.PolicyRules) != 2 ||
		document.PolicyRules[0].Decision != tobari.PolicyDecisionAllow || document.PolicyRules[0].Match != tobari.PolicyMatchExact ||
		!reflect.DeepEqual(document.PolicyRules[0].Examples, []string{"/items/123"}) ||
		document.PolicyRules[1].Decision != tobari.PolicyDecisionDeny || document.PolicyRules[1].Match != tobari.PolicyMatchExact ||
		document.PolicyRules[1].Examples == nil || len(document.PolicyRules[1].Examples) != 0 {
		t.Fatalf("exact allow/deny document = %+v", document)
	}

	empty := tobari.PolicyRuleReport{Task: tobari.TaskPolicyRules, PolicyDirectory: "/workspace/synthetic-policy", Items: []tobari.PolicyRule{}}
	if err := empty.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err = renderPolicyRulesWithCommands(empty, "tobari policy reset", successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	spec, found := DefaultCatalog().Lookup("policy rules")
	if !found || spec.Agent.Output.CollectionCoverage != CollectionCoverageExhaustive ||
		document.SchemaVersion != 1 || document.PolicyRules == nil || len(document.PolicyRules) != 0 {
		t.Fatalf("empty exhaustive policy report = %+v, contract=%+v", document, spec.Agent.Output)
	}
}

func TestPolicyRulesExactProtocolsPreserveOnlyTheirOwnedCoordinates(t *testing.T) {
	answer := readCatalogDomainOutputConformanceAnswer(t)
	tests := []struct {
		identity tobari.PolicyProtocolIdentity
		request  string
		path     string
		want     map[string]string
	}{
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolGraphQL, GraphQLOperationType: tobari.GraphQLOperationMutation, GraphQLRootField: "updateIssue"}, request: "1185da2688d7469aae9cd9068e920b0b", path: "/graphql", want: map[string]string{"graphql_operation_type": "mutation", "graphql_root_field": "updateIssue"}},
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolMCP, MCPMethod: "tools/call", MCPToolName: "example.search"}, request: "2185da2688d7469aae9cd9068e920b0b", path: "/mcp", want: map[string]string{"mcp_method": "tools/call", "mcp_tool_name": "example.search"}},
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolAWS, AWSWireProtocol: tobari.AWSWireProtocolQuery, AWSService: "sts", AWSOperation: "GetCallerIdentity"}, request: "3185da2688d7469aae9cd9068e920b0b", path: "/", want: map[string]string{"aws_wire_protocol": "query", "aws_service": "sts", "aws_operation": "GetCallerIdentity"}},
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolKubernetes, KubernetesVerb: "watch", KubernetesResource: "core/v1/namespaces/example/pods", KubernetesDryRun: "none"}, request: "4185da2688d7469aae9cd9068e920b0b", path: "/api/v1/pods", want: map[string]string{"kubernetes_verb": "watch", "kubernetes_resource": "core/v1/namespaces/example/pods", "kubernetes_dry_run": "none"}},
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolGit, GitService: "upload-pack", GitRepository: "/example/repo.git"}, request: "5185da2688d7469aae9cd9068e920b0b", path: "/example/repo.git/info/refs", want: map[string]string{"git_service": "upload-pack", "git_repository": "/example/repo.git"}},
		{identity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolOCI, OCIAction: "pull", OCIRepository: "example/app", OCIObject: "manifest:latest"}, request: "6185da2688d7469aae9cd9068e920b0b", path: "/v2/example/app/manifests/latest", want: map[string]string{"oci_action": "pull", "oci_repository": "example/app", "oci_object": "manifest:latest"}},
	}
	items := make([]tobari.PolicyRule, 0, len(tests))
	for _, test := range tests {
		denial := syntheticPolicyDenial(t, test.identity, test.request, test.path)
		candidate, err := tobari.NewPolicyCandidate(denial)
		if err != nil {
			t.Fatalf("%s candidate: %v", test.identity.Protocol, err)
		}
		learned, err := tobari.NewExactLearnedPolicyRule(candidate)
		if err != nil {
			t.Fatalf("%s learned rule: %v", test.identity.Protocol, err)
		}
		rule, err := tobari.NewPolicyRuleFromLearned(learned)
		if err != nil {
			t.Fatalf("%s public rule: %v", test.identity.Protocol, err)
		}
		items = append(items, rule)
	}
	encoded, err := renderPolicyRulesWithCommands(tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: "/workspace/synthetic-policy", Items: items,
	}, "tobari policy reset", successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		SchemaVersion int              `json:"schema_version"`
		PolicyRules   []map[string]any `json:"policy_rules"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.SchemaVersion != 1 || len(raw.PolicyRules) != len(tests) {
		t.Fatalf("protocol policy document = %+v", raw)
	}
	for index, test := range tests {
		item := raw.PolicyRules[index]
		if item["protocol"] != test.identity.Protocol || item["state_change"] != test.identity.StateChangePotential() {
			t.Fatalf("%s protocol/state = %+v", test.identity.Protocol, item)
		}
		for _, field := range answer.Template.EmptyProtocolFields {
			want := test.want[field]
			if item[field] != want {
				t.Fatalf("%s field %s = %#v, want %q", test.identity.Protocol, field, item[field], want)
			}
		}
	}
}

func TestReviewedReceiptConformsForNestedExactAndPathTemplateDecisions(t *testing.T) {
	item, templateRule := syntheticPathTemplatePolicyScenario(t)
	exactDenial := syntheticPolicyDenial(t, tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		"9185da2688d7469aae9cd9068e920b0b", "/health")
	exactCandidate, err := tobari.NewPolicyCandidate(exactDenial)
	if err != nil {
		t.Fatal(err)
	}
	exactRule, err := tobari.NewExactLearnedPolicyRule(exactCandidate)
	if err != nil {
		t.Fatal(err)
	}
	exactReceipt, err := tobari.NewPolicyReviewAppliedAllow(exactCandidate.ID, exactRule)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt, err := tobari.NewPolicyReviewAppliedAllow(item.ID, templateRule)
	if err != nil {
		t.Fatal(err)
	}
	change := tobari.PolicyReviewChange{
		Task: tobari.TaskPolicyReviewApply, PolicyDirectory: "/workspace/synthetic-policy",
		AllowCount: 2, DenyCount: 0, Applied: true, ActiveRevision: strings.Repeat("a", 64),
		Decisions: []tobari.PolicyReviewAppliedDecision{exactReceipt, templateReceipt},
	}
	if err := change.Validate(); err != nil {
		t.Fatal(err)
	}
	output := string(renderPolicyReviewChange(change, false))
	if !strings.Contains(output, "1. Allow exact") || !strings.Contains(output, "2. Allow template") ||
		!strings.Contains(output, exactReceipt.ReviewItemID) || !strings.Contains(output, templateReceipt.ReviewItemID) {
		t.Fatalf("reviewed receipt output = %q", output)
	}
	spec, found := commandSpecByPath(DefaultCatalog(), "policy apply-reviewed")
	if !found {
		t.Fatal("policy apply-reviewed is absent")
	}
	field := catalogOutputFieldAtPath(t, spec.Agent.Output.Fields, "decisions[].match")
	if !reflect.DeepEqual(field.Enum, tobari.PolicyMatchValues()) {
		t.Fatalf("nested reviewed match enum = %v", field.Enum)
	}
}

func TestPolicyRulesOutputEncodingDriftRemainsFailClosed(t *testing.T) {
	rule := syntheticPathTemplatePolicyRule(t)
	domainRule, err := tobari.NewPolicyRuleFromLearned(rule)
	if err != nil {
		t.Fatal(err)
	}
	resultDocument := policyRulesDocument{SchemaVersion: 1, PolicyRules: policyRuleOutputs(tobari.PolicyRuleReport{
		Task: tobari.TaskPolicyRules, PolicyDirectory: "/workspace/synthetic-policy", Items: []tobari.PolicyRule{domainRule},
	}, "tobari policy reset")}
	encoded, err := json.Marshal(resultDocument)
	if err != nil {
		t.Fatal(err)
	}
	spec, found := DefaultCatalog().Lookup("policy rules")
	if !found {
		t.Fatal("policy rules is absent")
	}
	drifted := cloneAgentContract(spec.Agent).Output
	for index := range drifted.Fields {
		if drifted.Fields[index].Name == "match" {
			drifted.Fields[index].Enum = []string{tobari.PolicyMatchExact}
		}
	}
	validationErr := validateJSONDocument(drifted, nil, encoded)
	if validationErr == nil {
		t.Fatal("valid path-template output passed an exact-only declaration")
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	ctx := withErrorFormat(withCommandPath(context.Background(), "policy rules"), errorFormatJSON)
	code := command.fail(ctx, fault.Wrap(
		fault.KindContract, "output_encoding_failed", "policy rules JSON could not be encoded", false, validationErr,
	))
	if code != ExitContract || stdout.Len() != 0 {
		t.Fatalf("output drift code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var failure errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &failure); err != nil {
		t.Fatal(err)
	}
	if failure.SchemaVersion != 2 || failure.Error.Kind != fault.KindContract ||
		failure.Error.Code != "output_encoding_failed" || failure.Error.Phase != fault.PhasePresentation ||
		failure.Error.ChangeState != fault.ChangeNotApplicable || failure.Error.Retryable ||
		len(failure.Error.NextActions) != 1 || failure.Error.NextActions[0].Command != "version" {
		t.Fatalf("output drift fault = %+v", failure)
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
	_, rule := syntheticPathTemplatePolicyScenario(t)
	return rule
}

func syntheticPathTemplatePolicyScenario(t *testing.T) (tobari.PolicyReviewItem, tobari.LearnedPolicyRule) {
	t.Helper()
	fixture := readCatalogDomainOutputConformanceFixture(t)
	if fixture.SchemaVersion != 1 || len(fixture.TemplateCandidates.ObservedAt) != 2 ||
		len(fixture.TemplateCandidates.RequestIDs) != 2 || len(fixture.TemplateCandidates.Paths) != 2 {
		t.Fatalf("template fixture = %+v", fixture)
	}
	input := fixture.TemplateCandidates
	base := syntheticPolicyDenial(t, tobari.PolicyProtocolIdentity{Scheme: input.Scheme, Protocol: tobari.PolicyProtocolHTTP}, input.RequestIDs[0], input.Paths[0])
	second := base
	second.Timestamp = input.ObservedAt[1]
	second.RequestID = input.RequestIDs[1]
	second.Path = input.Paths[1]
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
	return items[0], rule
}

func syntheticPolicyDenial(t *testing.T, identity tobari.PolicyProtocolIdentity, requestID, path string) tobari.PolicyDenial {
	t.Helper()
	fixture := readCatalogDomainOutputConformanceFixture(t)
	input := fixture.TemplateCandidates
	return tobari.PolicyDenial{
		PolicyProtocolIdentity: identity,
		Timestamp:              input.ObservedAt[0], RequestID: requestID,
		WorkspaceManifestID: input.WorkspaceManifestID, WorkspaceManifestName: input.WorkspaceManifest,
		ProjectID: input.WorkspaceID, ProjectRoot: input.ProjectRoot,
		Host: input.Host, Port: input.Port, Method: input.Method, Path: path,
		Reason: "synthetic denial", StatusCode: 403, Learnable: true,
	}
}

func readCatalogDomainOutputConformanceFixture(t *testing.T) catalogDomainOutputConformanceFixture {
	t.Helper()
	var fixture catalogDomainOutputConformanceFixture
	readStrictJSONFixture(t, "catalog-domain-output-conformance-fixture.json", &fixture)
	return fixture
}

func readCatalogDomainOutputConformanceAnswer(t *testing.T) catalogDomainOutputConformanceAnswer {
	t.Helper()
	var answer catalogDomainOutputConformanceAnswer
	readStrictJSONFixture(t, "catalog-domain-output-conformance-answer-key.json", &answer)
	return answer
}

func readStrictJSONFixture(t *testing.T, name string, target any) {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("fixture %q contains trailing JSON: %v", name, err)
	}
}

func catalogOutputFieldNames(fields []OutputField) []string {
	names := make([]string, 0)
	for _, field := range fields {
		if field.Name != "" {
			names = append(names, field.Name)
		}
		names = append(names, catalogOutputFieldNames(field.Fields)...)
		if field.Items != nil {
			names = append(names, catalogOutputFieldNames([]OutputField{*field.Items})...)
		}
	}
	return names
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
