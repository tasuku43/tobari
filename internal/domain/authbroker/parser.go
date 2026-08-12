package authbroker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// ParseProvider parses one bounded schema-v1 provider document.
// It rejects duplicate object keys before decoding because encoding/json
// otherwise keeps the last value, making a reviewed manifest ambiguous.
func ParseProvider(data []byte) (Provider, error) {
	if len(data) == 0 || len(data) > MaxProviderDocumentBytes || !utf8.Valid(data) {
		return Provider{}, fmt.Errorf("provider document must contain 1..%d bytes", MaxProviderDocumentBytes)
	}
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return Provider{}, err
	}
	if err := validateProviderJSONKeys(data); err != nil {
		return Provider{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var provider Provider
	if err := decoder.Decode(&provider); err != nil {
		return Provider{}, fmt.Errorf("decode provider document: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Provider{}, fmt.Errorf("provider document contains trailing data")
	}
	if err := provider.Validate(); err != nil {
		return Provider{}, err
	}
	return provider, nil
}

func validateProviderJSONKeys(data []byte) error {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode provider object: %w", err)
	}
	if root == nil {
		return fmt.Errorf("provider document must be a JSON object")
	}
	var schemaVersion int
	if err := json.Unmarshal(root["schema_version"], &schemaVersion); err != nil {
		return fmt.Errorf("provider schema_version must be an integer: %w", err)
	}
	allowedProviderKeys := []string{
		"schema_version", "id", "display_name", "acquisition", "credential",
		"workspace_projections", "header_bindings",
	}
	if err := rejectUnknownKeys("provider", root, allowedProviderKeys...); err != nil {
		return err
	}
	if err := validateObjectKeys("acquisition", root["acquisition"], "mode", "helper"); err != nil {
		return err
	}
	if err := validateObjectKeys("credential", root["credential"], "kind"); err != nil {
		return err
	}
	if err := validateArrayObjectKeys("workspace_projections", root["workspace_projections"], "kind", "name", "path", "template"); err != nil {
		return err
	}
	var bindings []map[string]json.RawMessage
	if len(root["header_bindings"]) == 0 {
		return fmt.Errorf("header_bindings must be an array of objects")
	}
	if err := json.Unmarshal(root["header_bindings"], &bindings); err != nil || bindings == nil {
		return fmt.Errorf("header_bindings must be an array of objects")
	}
	for index, binding := range bindings {
		label := fmt.Sprintf("header_bindings[%d]", index)
		if binding == nil {
			return fmt.Errorf("%s must be an object", label)
		}
		if err := rejectUnknownKeys(label, binding, "target", "source", "destination", "secret_headers"); err != nil {
			return err
		}
		if err := validateObjectKeys(label+".target", binding["target"], "scheme", "host", "port"); err != nil {
			return err
		}
		if err := validateObjectKeys(label+".source", binding["source"], "header", "formats"); err != nil {
			return err
		}
		if err := validateObjectKeys(label+".destination", binding["destination"], "header", "format", "secret_field"); err != nil {
			return err
		}
	}
	return nil
}

func validateObjectKeys(label string, data json.RawMessage, allowed ...string) error {
	if len(data) == 0 {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("%s must be an object: %w", label, err)
	}
	if object == nil {
		return fmt.Errorf("%s must be an object", label)
	}
	return rejectUnknownKeys(label, object, allowed...)
}

func validateArrayObjectKeys(label string, data json.RawMessage, allowed ...string) error {
	if len(data) == 0 {
		return nil
	}
	var objects []map[string]json.RawMessage
	if err := json.Unmarshal(data, &objects); err != nil {
		return fmt.Errorf("%s must be an array of objects: %w", label, err)
	}
	for index, object := range objects {
		if object == nil {
			return fmt.Errorf("%s[%d] must be an object", label, index)
		}
		if err := rejectUnknownKeys(fmt.Sprintf("%s[%d]", label, index), object, allowed...); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnknownKeys(label string, object map[string]json.RawMessage, allowed ...string) error {
	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}
	for key := range object {
		if _, exists := known[key]; !exists {
			return fmt.Errorf("%s contains unknown field %q", label, key)
		}
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return fmt.Errorf("scan provider document: %w", err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("provider document contains trailing data")
		}
		return fmt.Errorf("provider document contains trailing data: %w", err)
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array is not closed")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}
