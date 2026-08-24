package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
)

// marshalCommandJSON closes the executable boundary between a catalog output
// declaration and its renderer. A renderer may not emit a JSON document that
// is wider, narrower, or differently typed than the exact catalog contract.
func marshalCommandJSON(path string, document any) ([]byte, error) {
	return marshalCommandJSONForProgram(ProgramName, path, document)
}

func marshalCommandJSONForProgram(program, path string, document any) ([]byte, error) {
	command, found := DefaultCatalog().ForProgram(program).lookupRegistered(path)
	if !found {
		return nil, fmt.Errorf("JSON renderer command %q is not registered", path)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	if err := validateJSONDocument(command.Agent.Output, command.Agent.Pagination, encoded); err != nil {
		return nil, fmt.Errorf("JSON renderer for %q violates its catalog contract: %w", path, err)
	}
	return encoded, nil
}

func marshalErrorJSON(document errorDocument) ([]byte, error) {
	encoded, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	contract := CommandOutput{
		Fields: defaultAgentErrorFields(), JSONEnvelope: "error",
		JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 2,
	}
	if err := validateJSONDocument(contract, nil, encoded); err != nil {
		return nil, fmt.Errorf("structured error renderer violates its contract: %w", err)
	}
	return encoded, nil
}

func validateJSONDocument(output CommandOutput, pagination *PaginationContract, encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("document is not valid JSON: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	document, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("document root must be an object")
	}
	allowed := map[string]struct{}{"schema_version": {}, output.JSONEnvelope: {}}
	if pagination != nil {
		allowed[pagination.CursorOutput.Name] = struct{}{}
	}
	for key := range document {
		if _, exists := allowed[key]; !exists {
			return fmt.Errorf("document has undeclared top-level field %q", key)
		}
	}
	for key := range allowed {
		if _, exists := document[key]; !exists {
			return fmt.Errorf("document is missing required top-level field %q", key)
		}
	}
	schemaVersion, ok := document["schema_version"].(json.Number)
	if !ok {
		return fmt.Errorf("schema_version must be an integer")
	}
	parsedVersion, err := strconv.ParseInt(string(schemaVersion), 10, 64)
	if err != nil || parsedVersion != int64(output.JSONSchemaVersion) {
		return fmt.Errorf("schema_version must equal %d", output.JSONSchemaVersion)
	}
	envelope := OutputField{Name: output.JSONEnvelope, Type: output.JSONEnvelopeType, Description: "Catalog JSON envelope.", Fields: output.Fields}
	if output.JSONEnvelopeType == OutputFieldTypeString {
		envelope = output.Fields[0]
	} else if output.JSONEnvelopeType == OutputFieldTypeArray {
		envelope.Fields = nil
		envelope.Items = &OutputField{Type: OutputFieldTypeObject, Description: "Catalog JSON item.", Fields: output.Fields}
	}
	if err := validateJSONFieldValue("$."+output.JSONEnvelope, envelope, document[output.JSONEnvelope]); err != nil {
		return err
	}
	if pagination != nil {
		cursor := pagination.CursorOutput
		// empty_cursor is the one catalog-declared empty-string sentinel: it
		// closes traversal and therefore is not an opaque continuation value.
		cursor.ReferenceKind = ""
		if err := validateJSONFieldValue("$."+cursor.Name, cursor, document[cursor.Name]); err != nil {
			return err
		}
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("document contains more than one JSON value")
	}
	return fmt.Errorf("document has trailing invalid JSON: %w", err)
}

func validateJSONFieldValue(path string, field OutputField, value any) error {
	if value == nil {
		if field.Nullable {
			return nil
		}
		return fmt.Errorf("%s must not be null", path)
	}
	switch field.Type {
	case OutputFieldTypeString:
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		if field.ReferenceKind != "" && text == "" {
			return fmt.Errorf("%s opaque reference must not be empty", path)
		}
		if len(field.Enum) > 0 && !containsString(field.Enum, text) {
			return fmt.Errorf("%s value %q is outside enum %v", path, text, field.Enum)
		}
	case OutputFieldTypeInteger:
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s must be an integer", path)
		}
		if _, err := strconv.ParseInt(string(number), 10, 64); err != nil {
			return fmt.Errorf("%s must be an integer", path)
		}
	case OutputFieldTypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case OutputFieldTypeObject:
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		if err := validateJSONObject(path, field.Fields, object); err != nil {
			return err
		}
	case OutputFieldTypeArray:
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		for index, item := range items {
			if err := validateJSONFieldValue(fmt.Sprintf("%s[%d]", path, index), *field.Items, item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s has unknown catalog type %q", path, field.Type)
	}
	return nil
}

func validateJSONObject(path string, fields []OutputField, object map[string]any) error {
	declared := make(map[string]OutputField, len(fields))
	for _, field := range fields {
		declared[field.Name] = field
	}
	for key := range object {
		if _, exists := declared[key]; !exists {
			return fmt.Errorf("%s has undeclared field %q", path, key)
		}
	}
	for _, field := range fields {
		value, exists := object[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return fmt.Errorf("%s is missing required field %q", path, field.Name)
		}
		if err := validateJSONFieldValue(path+"."+field.Name, field, value); err != nil {
			return err
		}
	}
	return nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}
