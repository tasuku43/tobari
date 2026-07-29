package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestJSONOutputMatchesCatalogContract couples the executable renderers to the
// schema version, envelope, and logical item fields published by CommandOutput.
// It fails whether a renderer drifts alone or its catalog declaration drifts
// without a matching public output change.
func TestJSONOutputMatchesCatalogContract(t *testing.T) {
	type probe struct {
		path  string
		args  []string
		build func() (*CLI, *bytes.Buffer, *bytes.Buffer)
		view  string
	}
	newDefault := func() (*CLI, *bytes.Buffer, *bytes.Buffer) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		return New(strings.NewReader(""), stdout, stderr), stdout, stderr
	}
	probes := []probe{
		{
			path: "doctor", args: []string{"doctor", "--format=json"},
			build: func() (*CLI, *bytes.Buffer, *bytes.Buffer) {
				return newTestCLI(passingInspector("ready"))
			},
		},
		{path: "help", args: []string{"help", "--format=agent"}, build: newDefault, view: "index"},
		{path: "sample list", args: []string{"sample", "list", "--format=json"}, build: newDefault},
		{path: "sample read", args: []string{"sample", "read", "--id", "smp_2f4a6c8e0b1d", "--format=json"}, build: newDefault},
	}

	for _, current := range probes {
		t.Run(strings.ReplaceAll(current.path, " ", "_"), func(t *testing.T) {
			command, stdout, stderr := current.build()
			spec, found := command.catalog.Lookup(current.path)
			if !found {
				t.Fatalf("catalog lacks %q", current.path)
			}
			if !containsOutputFormat(spec.Agent.Output.Formats, OutputFormatJSON) {
				t.Fatalf("%q does not declare JSON output", current.path)
			}
			if code := runCLI(command, current.args); code != ExitOK {
				t.Fatalf("Run(%v) code = %d, stderr = %q", current.args, code, stderr.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("Run(%v) stderr = %q", current.args, stderr.String())
			}

			var document map[string]json.RawMessage
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatalf("output is not a JSON object: %v\n%s", err, stdout.String())
			}
			for _, catalogOnly := range []string{"delivery", "collection_coverage"} {
				if _, exists := document[catalogOnly]; exists {
					t.Fatalf("runtime success output unexpectedly contains catalog-only field %q: %s", catalogOnly, stdout.String())
				}
			}
			var schemaVersion int
			if err := json.Unmarshal(document["schema_version"], &schemaVersion); err != nil {
				t.Fatalf("schema_version is missing or invalid: %v", err)
			}
			if schemaVersion != spec.Agent.Output.JSONSchemaVersion {
				t.Fatalf("schema_version = %d, catalog = %d", schemaVersion, spec.Agent.Output.JSONSchemaVersion)
			}
			if current.view != "" {
				var view string
				if err := json.Unmarshal(document["view"], &view); err != nil {
					t.Fatalf("view is missing or invalid: %v", err)
				}
				if view != current.view {
					t.Fatalf("view = %q, want %q", view, current.view)
				}
			}
			if err := validatePaginationJSONMarker(document, spec.Agent.Pagination); err != nil {
				t.Fatalf("pagination output violates catalog: %v", err)
			}
			envelope, exists := document[spec.Agent.Output.JSONEnvelope]
			if !exists {
				t.Fatalf("JSON lacks declared envelope %q: %s", spec.Agent.Output.JSONEnvelope, stdout.String())
			}

			items := decodeOutputItems(t, envelope)
			wantFields := make([]string, 0, len(spec.Agent.Output.Fields))
			for _, field := range spec.Agent.Output.Fields {
				wantFields = append(wantFields, field.Name)
			}
			sort.Strings(wantFields)
			for index, item := range items {
				gotFields := make([]string, 0, len(item))
				for field := range item {
					gotFields = append(gotFields, field)
				}
				sort.Strings(gotFields)
				if !reflect.DeepEqual(gotFields, wantFields) {
					t.Fatalf("envelope %q item %d fields = %v, catalog = %v", spec.Agent.Output.JSONEnvelope, index, gotFields, wantFields)
				}
			}
		})
	}
}

func TestPagedJSONCompletionCursorIsAlwaysPresentString(t *testing.T) {
	contract := &PaginationContract{
		CursorOutput: OutputField{Name: "next_cursor"},
		Completion:   PaginationCompletionEmptyCursor,
	}
	tests := map[string]struct {
		document string
		wantErr  bool
	}{
		"continuation": {document: `{"schema_version":1,"items":[],"next_cursor":"opaque"}`},
		"complete":     {document: `{"schema_version":1,"items":[],"next_cursor":""}`},
		"missing":      {document: `{"schema_version":1,"items":[]}`, wantErr: true},
		"null":         {document: `{"schema_version":1,"items":[],"next_cursor":null}`, wantErr: true},
		"non string":   {document: `{"schema_version":1,"items":[],"next_cursor":false}`, wantErr: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var document map[string]json.RawMessage
			if err := json.Unmarshal([]byte(test.document), &document); err != nil {
				t.Fatal(err)
			}
			err := validatePaginationJSONMarker(document, contract)
			if (err != nil) != test.wantErr {
				t.Fatalf("validatePaginationJSONMarker() error = %v, wantErr = %t", err, test.wantErr)
			}
		})
	}

	contract.Completion = PaginationCompletionUnknown
	if err := validatePaginationJSONMarker(map[string]json.RawMessage{"next_cursor": json.RawMessage(`""`)}, contract); err == nil {
		t.Fatal("unknown completion rule passed output validation")
	}
}

// validatePaginationJSONMarker is reused by every renderer probe above. A
// derived paged command therefore cannot pass its fixture contract with an
// omitted, null, or non-string completion cursor.
func validatePaginationJSONMarker(document map[string]json.RawMessage, pagination *PaginationContract) error {
	if pagination == nil {
		return nil
	}
	if pagination.Completion != PaginationCompletionEmptyCursor {
		return fmt.Errorf("unsupported completion rule %q", pagination.Completion)
	}
	raw, exists := document[pagination.CursorOutput.Name]
	if !exists {
		return fmt.Errorf("top-level cursor field %q is missing", pagination.CursorOutput.Name)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return fmt.Errorf("top-level cursor field %q must be a string; only an empty string marks completion", pagination.CursorOutput.Name)
	}
	var cursor string
	if err := json.Unmarshal(trimmed, &cursor); err != nil {
		return fmt.Errorf("top-level cursor field %q must be a string: %w", pagination.CursorOutput.Name, err)
	}
	return nil
}

func decodeOutputItems(t *testing.T, envelope json.RawMessage) []map[string]json.RawMessage {
	t.Helper()
	trimmed := bytes.TrimSpace(envelope)
	if len(trimmed) == 0 {
		t.Fatal("declared JSON envelope is empty")
	}
	var items []map[string]json.RawMessage
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &items); err != nil {
			t.Fatalf("declared JSON envelope is not an object array: %v", err)
		}
	case '{':
		var item map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &item); err != nil {
			t.Fatalf("declared JSON envelope is not an object: %v", err)
		}
		items = []map[string]json.RawMessage{item}
	default:
		t.Fatalf("declared JSON envelope must contain an object or object array: %s", trimmed)
	}
	if len(items) == 0 {
		t.Fatal("contract probe requires at least one output item")
	}
	return items
}
