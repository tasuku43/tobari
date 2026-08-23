package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineOutputInterpretationFixtureHasReviewedAnswerKey(t *testing.T) {
	fixture := readMachineOutputFixture(t, "machine-output-contract-fixture.json")
	answer := readMachineOutputFixture(t, "machine-output-contract-answer-key.json")
	cases := fixture["cases"].(map[string]any)
	cluster := cases["unconfigured_cluster"].(map[string]any)
	if cluster["configured"] != false || cluster["running"] != false ||
		cluster["policy"] != nil || cluster["policy_revision"] != nil ||
		cluster["workspace_manifest_count"] != float64(0) ||
		cluster["policy_projection"] != "unavailable" ||
		len(cluster["components"].([]any)) != 0 {
		t.Fatalf("unconfigured fixture facts = %+v", cluster)
	}
	if _, retained := cluster["proxy"]; retained {
		t.Fatalf("unconfigured fixture contains prohibited proxy field: %+v", cluster)
	}
	if _, retained := cluster["context_count"]; retained {
		t.Fatalf("unconfigured fixture contains retired Context field: %+v", cluster)
	}
	facts := answer["facts"].(map[string]any)
	for name, value := range facts {
		if value != true && name != "internal_completion_is_public" {
			t.Fatalf("answer fact %q = %v", name, value)
		}
	}
	if facts["internal_completion_is_public"] != false || answer["routine_success_external_processing_count"] != float64(0) {
		t.Fatalf("answer key = %+v", answer)
	}
}

func readMachineOutputFixture(t *testing.T, name string) map[string]any {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

func TestRecursiveOutputFieldValidationRejectsInvalidShapes(t *testing.T) {
	t.Parallel()
	validLeaf := OutputField{
		Name: "state", Type: OutputFieldTypeString, Description: "Declared state.",
		Enum: []string{"ready", "unavailable"},
	}
	tests := []struct {
		name  string
		field OutputField
		want  string
	}{
		{
			name:  "object without children",
			field: OutputField{Name: "item", Type: OutputFieldTypeObject, Description: "Item."},
			want:  "object fields are unknown",
		},
		{
			name:  "array without items",
			field: OutputField{Name: "items", Type: OutputFieldTypeArray, Description: "Items."},
			want:  "array item shape is unknown",
		},
		{
			name:  "scalar with children",
			field: OutputField{Name: "state", Type: OutputFieldTypeString, Description: "State.", Fields: []OutputField{validLeaf}},
			want:  "scalar cannot declare children",
		},
		{
			name:  "enum on integer",
			field: OutputField{Name: "count", Type: OutputFieldTypeInteger, Description: "Count.", Enum: []string{"1"}},
			want:  "enum requires string type",
		},
		{
			name:  "duplicate enum",
			field: OutputField{Name: "state", Type: OutputFieldTypeString, Description: "State.", Enum: []string{"ready", "ready"}},
			want:  "enum value \"ready\" is declared more than once",
		},
		{
			name:  "empty enum sentinel",
			field: OutputField{Name: "state", Type: OutputFieldTypeString, Description: "State.", Enum: []string{"", "ready"}},
			want:  "enum cannot contain an empty sentinel",
		},
		{
			name:  "nullable opaque reference",
			field: OutputField{Name: "id", Type: OutputFieldTypeString, Description: "Opaque ID.", Nullable: true, ReferenceKind: "policy-candidate"},
			want:  "opaque reference cannot be nullable",
		},
		{
			name:  "object with array item marker",
			field: OutputField{Name: "item", Type: OutputFieldTypeObject, Description: "Item.", Fields: []OutputField{validLeaf}, Items: &OutputField{Type: OutputFieldTypeString, Description: "Value."}},
			want:  "object cannot declare array items",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := catalogWithDoctorOutputField(t, test.field)
			if err := catalog.Validate(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRecursiveJSONValidationRejectsShapeTypeEnumAndNullabilityDrift(t *testing.T) {
	output := CommandOutput{
		Formats: []OutputFormat{OutputFormatJSON}, DefaultFormat: OutputFormatJSON,
		Fields: []OutputField{
			{Name: "state", Type: OutputFieldTypeString, Description: "Finite state.", Enum: []string{"ready", "locked"}},
			{Name: "note", Type: OutputFieldTypeString, Description: "Optional nullable note.", Optional: true, Nullable: true},
			{Name: "items", Type: OutputFieldTypeArray, Description: "Nested items.", Items: &OutputField{
				Type: OutputFieldTypeObject, Description: "One item.", Fields: []OutputField{
					{Name: "count", Type: OutputFieldTypeInteger, Description: "Exact count."},
				},
			}},
		},
		Delivery: OutputDeliveryComplete, CollectionCoverage: CollectionCoverageNotApplicable,
		JSONEnvelope: "result", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 1,
	}
	valid := `{"schema_version":1,"result":{"state":"ready","note":null,"items":[{"count":1}]}}`
	if err := validateJSONDocument(output, nil, []byte(valid)); err != nil {
		t.Fatalf("valid recursive document: %v", err)
	}
	invalid := map[string]string{
		"missing":     `{"schema_version":1,"result":{"items":[{"count":1}]}}`,
		"extra":       `{"schema_version":1,"result":{"state":"ready","items":[],"surprise":true}}`,
		"type":        `{"schema_version":1,"result":{"state":"ready","items":[{"count":"1"}]}}`,
		"enum":        `{"schema_version":1,"result":{"state":"unknown","items":[]}}`,
		"nullability": `{"schema_version":1,"result":{"state":null,"items":[]}}`,
		"envelope":    `{"schema_version":1,"result":[]}`,
		"top-level":   `{"schema_version":1,"result":{"state":"ready","items":[]},"extra":0}`,
	}
	for name, document := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateJSONDocument(output, nil, []byte(document)); err == nil {
				t.Fatal("invalid recursive document passed validation")
			}
		})
	}
}

func TestStructuredErrorFallbackIsACompleteNonRecursiveContractDocument(t *testing.T) {
	contract := CommandOutput{
		Fields: defaultAgentErrorFields(), JSONEnvelope: "error",
		JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
	}
	if err := validateJSONDocument(contract, nil, structuredErrorContractFallback); err != nil {
		t.Fatalf("structured error fallback: %v", err)
	}
	_, err := marshalErrorJSON(errorDocument{SchemaVersion: 2, Error: errorPayload{Kind: "not-a-kind"}})
	if err == nil {
		t.Fatal("malformed structured error unexpectedly passed the renderer contract")
	}
}

func TestRecursiveOutputFieldValidationRejectsExcessiveDepth(t *testing.T) {
	t.Parallel()
	field := OutputField{Name: "leaf", Type: OutputFieldTypeString, Description: "Leaf."}
	for depth := 0; depth < maxOutputFieldDepth+1; depth++ {
		field = OutputField{
			Name: "nested", Type: OutputFieldTypeObject, Description: "Nested object.",
			Fields: []OutputField{field},
		}
	}
	catalog := catalogWithDoctorOutputField(t, field)
	if err := catalog.Validate(); err == nil || !strings.Contains(err.Error(), "exceeds maximum depth") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func catalogWithDoctorOutputField(t *testing.T, field OutputField) Catalog {
	t.Helper()
	catalog := DefaultCatalog()
	for index := range catalog.commands {
		if catalog.commands[index].Path == "doctor" {
			catalog.commands[index].Agent.Output.Fields = []OutputField{field}
			return catalog
		}
	}
	t.Fatal("catalog lacks doctor")
	return Catalog{}
}

func TestInternalCatalogVisibilityExcludesCompletionFromPublicSurfaces(t *testing.T) {
	t.Parallel()
	catalog := DefaultCatalog()
	if _, found := catalog.Lookup("policy apply-reviewed"); found {
		t.Fatal("public lookup exposes internal completion command")
	}
	for _, command := range catalog.Commands() {
		if command.Path == "policy apply-reviewed" {
			t.Fatal("public command list exposes internal completion command")
		}
	}
	internal, found := catalog.lookupRegistered("policy apply-reviewed")
	if !found || internal.Visibility != CommandVisibilityInternal {
		t.Fatalf("registered completion = %+v, found = %t", internal, found)
	}
	if _, _, found := catalog.Match([]string{"policy", "apply-reviewed"}); found {
		t.Fatal("public routing exposes internal completion command")
	}
	policyCommands, exact := catalog.Select("policy")
	if exact || len(policyCommands) == 0 {
		t.Fatalf("Select(policy) = %d commands, exact=%t", len(policyCommands), exact)
	}
	for _, command := range policyCommands {
		if command.Path == "policy apply-reviewed" {
			t.Fatal("public namespace help exposes internal completion command")
		}
	}
}

func TestProgramFilteredCatalogProjectionRetainsGlobalReferenceClosure(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatalf("global catalog: %v", err)
	}
	if _, found := catalog.Lookup(ExposureProgramName); found {
		t.Fatal("host projection exposes helper root")
	}
	if _, found := catalog.ForProgram(ExposureProgramName).Lookup("review"); found {
		t.Fatal("helper projection exposes host review")
	}
}

func TestExactAgentHelpDeclaresExecutableMachineInvocations(t *testing.T) {
	t.Parallel()
	command := newReferenceTestCLI(strings.NewReader(""), nil, nil)
	output, err := command.renderAgentHelp("status", true, mustSelect(t, command.catalog, "status"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(output)
	for _, want := range []string{
		`"schema_version":1`,
		expectedSurfaceText(`"success_json":"tobari status --format=json"`),
		expectedSurfaceText(`"error_json":"tobari --error-format=json status --format=json"`),
		`"global_flag_position":"before_command"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("agent help lacks %s: %s", want, text)
		}
	}
}

func mustSelect(t *testing.T, catalog Catalog, selector string) []CommandSpec {
	t.Helper()
	commands, exact := catalog.Select(selector)
	if !exact || len(commands) != 1 {
		t.Fatalf("Select(%q) = %d commands, exact=%t", selector, len(commands), exact)
	}
	return commands
}
