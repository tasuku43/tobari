package dockerruntime

import "net/url"

type workspaceLoginQueryField struct {
	required bool
	validate func(string) bool
}

// workspaceLoginQuerySchema is deliberately narrower than a generic OAuth
// parser. It keeps one provider's reviewed field set closed while centralizing
// the membership, requiredness, and singleton checks that every field needs.
type workspaceLoginQuerySchema map[string]workspaceLoginQueryField

func (schema workspaceLoginQuerySchema) valid(query url.Values) bool {
	if len(schema) == 0 {
		return false
	}
	for name, field := range schema {
		if name == "" || field.validate == nil {
			return false
		}
	}
	for name, values := range query {
		field, known := schema[name]
		if !known || len(values) != 1 || !field.validate(values[0]) {
			return false
		}
	}
	for name, field := range schema {
		if field.required {
			if _, present := query[name]; !present {
				return false
			}
		}
	}
	return true
}

func exactWorkspaceLoginQueryValue(expected string) func(string) bool {
	return func(actual string) bool {
		return actual == expected
	}
}

func nonEmptyWorkspaceLoginQueryValue(value string) bool {
	return value != ""
}
