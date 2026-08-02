package tobari

import (
	"errors"
	"fmt"
	"path/filepath"
)

const (
	ContextSchemaVersion = 1
	DefaultContextName   = "default"

	TaskContextList   = "context.list"
	TaskContextShow   = "context.show"
	TaskContextCreate = "context.create"
	TaskContextUse    = "context.use"

	ContextCatalogTargetKind = "contexts"
	ContextCatalogTargetID   = "context-catalog"
	ContextTargetKind        = "context"
	ActiveContextTargetID    = "active-context"
)

var (
	ErrContextExists   = errors.New("Context already exists")
	ErrContextNotFound = errors.New("Context does not exist")
)

// ContextPolicyMode selects the policy-development experience associated with
// a Context. It does not change Gateway authorization by itself.
type ContextPolicyMode string

const (
	ContextPolicyModeGuided   ContextPolicyMode = "guided"
	ContextPolicyModeAdvanced ContextPolicyMode = "advanced"
)

func (m ContextPolicyMode) Validate() error {
	switch m {
	case ContextPolicyModeGuided, ContextPolicyModeAdvanced:
		return nil
	default:
		return fmt.Errorf("context policy mode is invalid: %q", m)
	}
}

// ContextManifest is the trusted, secret-free logical composition record.
// Paths are deliberately resolved by infrastructure rather than persisted in
// the manifest so stores remain independently protected.
type ContextManifest struct {
	SchemaVersion int               `json:"schema_version"`
	Name          string            `json:"name"`
	AgentProfile  string            `json:"agent_profile"`
	Image         string            `json:"image"`
	PolicyMode    ContextPolicyMode `json:"policy_mode"`
}

func (m ContextManifest) Validate() error {
	if m.SchemaVersion != ContextSchemaVersion {
		return fmt.Errorf("context schema version must be %d", ContextSchemaVersion)
	}
	if err := ValidateName(m.Name); err != nil {
		return fmt.Errorf("context name: %w", err)
	}
	if err := ValidateName(m.AgentProfile); err != nil {
		return fmt.Errorf("context agent profile: %w", err)
	}
	if err := ValidateImageSelector(m.Image); err != nil {
		return fmt.Errorf("context image: %w", err)
	}
	return m.PolicyMode.Validate()
}

// ContextStorePaths are trusted infrastructure-resolved paths. They are
// included in host diagnostics, never in Workspace mounts as a whole.
type ContextStorePaths struct {
	PolicyDirectory     string `json:"policy_directory"`
	CredentialConfig    string `json:"credential_config"`
	CredentialDirectory string `json:"credential_directory"`
}

func (p ContextStorePaths) Validate() error {
	for name, value := range map[string]string{
		"policy directory":     p.PolicyDirectory,
		"credential config":    p.CredentialConfig,
		"credential directory": p.CredentialDirectory,
	} {
		if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("context %s must be a canonical absolute path", name)
		}
	}
	return nil
}

// ContextSummary is one item in the complete local Context collection.
type ContextSummary struct {
	Name         string            `json:"name"`
	Active       bool              `json:"active"`
	AgentProfile string            `json:"agent_profile"`
	Image        string            `json:"image"`
	PolicyMode   ContextPolicyMode `json:"policy_mode"`
}

func (s ContextSummary) Validate() error {
	manifest := ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		Name:          s.Name,
		AgentProfile:  s.AgentProfile,
		Image:         s.Image,
		PolicyMode:    s.PolicyMode,
	}
	return manifest.Validate()
}

// ContextListResult is a complete local Context observation. Empty is
// represented by an explicit non-nil Items collection, although a valid
// installation always contains the default Context.
type ContextListResult struct {
	Task   string           `json:"task"`
	Active string           `json:"active"`
	Items  []ContextSummary `json:"items"`
}

func (r ContextListResult) Validate() error {
	if r.Task != TaskContextList || r.Items == nil || r.Active == "" {
		return fmt.Errorf("context list task or scope is invalid")
	}
	if err := ValidateName(r.Active); err != nil {
		return fmt.Errorf("active context: %w", err)
	}
	seen := make(map[string]struct{}, len(r.Items))
	activeCount := 0
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, exists := seen[item.Name]; exists {
			return fmt.Errorf("context names must be unique")
		}
		seen[item.Name] = struct{}{}
		if item.Active {
			activeCount++
			if item.Name != r.Active {
				return fmt.Errorf("active context does not match the active item")
			}
		}
	}
	if _, exists := seen[r.Active]; !exists || activeCount != 1 {
		return fmt.Errorf("context list must contain exactly one active context")
	}
	return nil
}

// ContextReport is the complete selected Context view.
type ContextReport struct {
	Task         string            `json:"task"`
	Name         string            `json:"name"`
	Active       bool              `json:"active"`
	AgentProfile string            `json:"agent_profile"`
	Image        string            `json:"image"`
	PolicyMode   ContextPolicyMode `json:"policy_mode"`
	Stores       ContextStorePaths `json:"stores"`
}

func (r ContextReport) Validate() error {
	if r.Task != TaskContextShow && r.Task != TaskContextCreate && r.Task != TaskContextUse {
		return fmt.Errorf("context report task is invalid")
	}
	manifest := ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		Name:          r.Name,
		AgentProfile:  r.AgentProfile,
		Image:         r.Image,
		PolicyMode:    r.PolicyMode,
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	return r.Stores.Validate()
}
