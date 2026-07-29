package authn

import (
	"fmt"
	"strings"
)

const (
	// UserConfigurationSchemaVersion is the only persistent public-settings
	// schema understood by this template version.
	UserConfigurationSchemaVersion = 1
	// MaxUserConfigurationBytes bounds strict decoding before allocation.
	MaxUserConfigurationBytes = 16 * 1024
	maxPublicParameters       = 16
)

// PublicParameter is one persistable, non-secret authentication setting.
// Derived projects define the supported names and semantic validation.
type PublicParameter struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// UserConfiguration is non-secret setup metadata. Credential-bearing values
// are deliberately absent and belong to a separate infrastructure boundary.
type UserConfiguration struct {
	SchemaVersion int               `json:"schema_version"`
	Method        Method            `json:"method"`
	Parameters    []PublicParameter `json:"parameters"`
}

// Clone returns a configuration with independent parameter storage.
func (c UserConfiguration) Clone() UserConfiguration {
	clone := c
	clone.Parameters = append([]PublicParameter(nil), c.Parameters...)
	return clone
}

// Validate rejects unknown schemas and unbounded, duplicate, or unsafe public
// parameters. Provider-specific allowed names remain a derived policy.
func (c UserConfiguration) Validate() error {
	if c.SchemaVersion != UserConfigurationSchemaVersion {
		return fmt.Errorf("authentication configuration schema is unsupported")
	}
	if err := c.Method.Validate(); err != nil {
		return err
	}
	if c.Parameters == nil {
		return fmt.Errorf("authentication configuration parameters are unknown")
	}
	if len(c.Parameters) > maxPublicParameters {
		return fmt.Errorf("authentication configuration has too many public parameters")
	}
	seen := make(map[string]struct{}, len(c.Parameters))
	for _, parameter := range c.Parameters {
		if err := validateParameterName(parameter.Name); err != nil {
			return err
		}
		if err := validateMetadata("public authentication parameter", parameter.Value, true); err != nil {
			return err
		}
		if _, exists := seen[parameter.Name]; exists {
			return fmt.Errorf("public authentication parameter names must be unique")
		}
		seen[parameter.Name] = struct{}{}
	}
	return nil
}

func validateParameterName(value string) error {
	if value == "" || len(value) > 64 || strings.Trim(value, "_") != value {
		return fmt.Errorf("public authentication parameter name is missing or invalid")
	}
	for index, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case index > 0 && r >= '0' && r <= '9':
		case index > 0 && r == '_':
		default:
			return fmt.Errorf("public authentication parameter name is missing or invalid")
		}
	}
	for _, part := range strings.Split(value, "_") {
		switch part {
		case "token", "secret", "credential", "password", "authorization", "code", "verifier", "pat":
			return fmt.Errorf("credential-bearing authentication parameter names are forbidden")
		}
	}
	return nil
}

// ConfigurationState is a read-only persistence reconciliation result.
type ConfigurationState string

const (
	ConfigurationStateMissing ConfigurationState = "missing"
	ConfigurationStateValid   ConfigurationState = "valid"
	ConfigurationStateInvalid ConfigurationState = "invalid"
)

// ConfigurationStatus exposes only safe setup metadata and a stable problem
// code. It never carries a rejected value or filesystem error string.
type ConfigurationStatus struct {
	State         ConfigurationState `json:"state"`
	SchemaVersion int                `json:"schema_version,omitempty"`
	Method        Method             `json:"method,omitempty"`
	Problem       string             `json:"problem,omitempty"`
}
