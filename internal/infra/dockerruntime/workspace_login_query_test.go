package dockerruntime

import (
	"net/url"
	"regexp"
	"testing"
)

func TestWorkspaceLoginQuerySchemaValidatesClosedSingletonFields(t *testing.T) {
	schema := workspaceLoginQuerySchema{
		"required_exact": {
			required: true,
			validate: exactWorkspaceLoginQueryValue("expected"),
		},
		"required_shaped": {
			required: true,
			validate: regexp.MustCompile(`^[a-z]{3}$`).MatchString,
		},
		"optional_shaped": {
			validate: regexp.MustCompile(`^[0-9]{2}$`).MatchString,
		},
	}

	for name, query := range map[string]url.Values{
		"required only": {
			"required_exact":  {"expected"},
			"required_shaped": {"abc"},
		},
		"with optional": {
			"required_exact":  {"expected"},
			"required_shaped": {"abc"},
			"optional_shaped": {"42"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if !schema.valid(query) {
				t.Fatalf("reviewed query rejected: %#v", query)
			}
		})
	}

	for name, query := range map[string]url.Values{
		"unknown": {
			"required_exact":  {"expected"},
			"required_shaped": {"abc"},
			"neighbor":        {"value"},
		},
		"missing required": {
			"required_exact": {"expected"},
		},
		"empty required values": {
			"required_exact":  {},
			"required_shaped": {"abc"},
		},
		"duplicate required": {
			"required_exact":  {"expected", "expected"},
			"required_shaped": {"abc"},
		},
		"duplicate optional": {
			"required_exact":  {"expected"},
			"required_shaped": {"abc"},
			"optional_shaped": {"42", "43"},
		},
		"malformed required": {
			"required_exact":  {"unexpected"},
			"required_shaped": {"abc"},
		},
		"malformed optional": {
			"required_exact":  {"expected"},
			"required_shaped": {"abc"},
			"optional_shaped": {"x"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if schema.valid(query) {
				t.Fatalf("unreviewed query accepted: %#v", query)
			}
		})
	}
}

func TestWorkspaceLoginQuerySchemaRejectsInvalidDefinitions(t *testing.T) {
	for name, schema := range map[string]workspaceLoginQuerySchema{
		"empty schema": {},
		"empty field name": {
			"": {validate: exactWorkspaceLoginQueryValue("value")},
		},
		"missing validator": {
			"field": {},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if schema.valid(url.Values{}) {
				t.Fatal("invalid closed query schema accepted")
			}
		})
	}
}
