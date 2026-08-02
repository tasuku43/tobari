package tobari

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
)

const (
	ContextSchemaVersion       = 2
	LegacyContextSchemaVersion = 1
	DefaultContextName         = "default"

	TaskContextList   = "context.list"
	TaskContextShow   = "context.show"
	TaskContextCreate = "context.create"
	TaskContextUse    = "context.use"
	TaskRuntimeInit   = "runtime.init"
	TaskRuntimeBuild  = "runtime.build"

	ContextCatalogTargetKind = "contexts"
	ContextCatalogTargetID   = "context-catalog"
	ContextTargetKind        = "context"
	ActiveContextTargetID    = "active-context"
	ContextRuntimeTargetKind = "context-runtime"
	ActiveContextRuntimeID   = "active-context-runtime"
	ContextRuntimeRecipeFile = "runtime/Dockerfile"
	OfficialRuntimeBase      = "ghcr.io/tasuku43/tobari/runtime:latest"
)

var (
	ErrContextExists        = errors.New("Context already exists")
	ErrContextNotFound      = errors.New("Context does not exist")
	ErrRuntimeRecipeExists  = errors.New("Context runtime recipe already exists")
	ErrRuntimeRecipeMissing = errors.New("Context runtime recipe does not exist")
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

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

// ContextRuntimeKind identifies the source of a Context runtime.
type ContextRuntimeKind string

const (
	ContextRuntimeKindOfficial   ContextRuntimeKind = "official"
	ContextRuntimeKindDockerfile ContextRuntimeKind = "dockerfile"
)

func (k ContextRuntimeKind) Validate() error {
	switch k {
	case ContextRuntimeKindOfficial, ContextRuntimeKindDockerfile:
		return nil
	default:
		return fmt.Errorf("context runtime kind is invalid: %q", k)
	}
}

// ContextRuntimeStatus is the user-facing state of the selected runtime
// recipe. A pending recipe never replaces the last selected image.
type ContextRuntimeStatus string

const (
	ContextRuntimeStatusOfficial     ContextRuntimeStatus = "official"
	ContextRuntimeStatusPendingBuild ContextRuntimeStatus = "pending_build"
	ContextRuntimeStatusReady        ContextRuntimeStatus = "ready"
	ContextRuntimeStatusInvalid      ContextRuntimeStatus = "invalid"
)

func (s ContextRuntimeStatus) Validate() error {
	switch s {
	case ContextRuntimeStatusOfficial, ContextRuntimeStatusPendingBuild,
		ContextRuntimeStatusReady, ContextRuntimeStatusInvalid:
		return nil
	default:
		return fmt.Errorf("context runtime status is invalid: %q", s)
	}
}

// ContextRuntimeBuild is the last successful build record for a recipe.
// Image is a Tobari-managed local reference; ImageDigest is the immutable
// Docker image identity used for diagnostics and drift detection.
type ContextRuntimeBuild struct {
	Image        string `json:"image"`
	ImageDigest  string `json:"image_digest"`
	SourceDigest string `json:"source_digest"`
}

func (b ContextRuntimeBuild) Validate() error {
	if err := ValidateImageSelector(b.Image); err != nil || b.Image == BuiltinImageSelector {
		if err == nil {
			err = fmt.Errorf("managed runtime image cannot be builtin")
		}
		return fmt.Errorf("runtime build image: %w", err)
	}
	if err := ValidateDigest(b.ImageDigest); err != nil {
		return fmt.Errorf("runtime image digest: %w", err)
	}
	if err := ValidateDigest(b.SourceDigest); err != nil {
		return fmt.Errorf("runtime source digest: %w", err)
	}
	return nil
}

// ContextRuntimeRecipe describes the host-owned runtime/Dockerfile source.
// File is deliberately fixed so the routine workflow has no path selector.
type ContextRuntimeRecipe struct {
	Kind          ContextRuntimeKind   `json:"kind"`
	File          string               `json:"file"`
	BaseReference string               `json:"base_reference"`
	SourceDigest  string               `json:"source_digest,omitempty"`
	LastBuild     *ContextRuntimeBuild `json:"last_build,omitempty"`
}

func (r ContextRuntimeRecipe) Validate() error {
	if r.Kind != ContextRuntimeKindDockerfile {
		return fmt.Errorf("runtime recipe kind must be %q", ContextRuntimeKindDockerfile)
	}
	if r.File != ContextRuntimeRecipeFile {
		return fmt.Errorf("runtime recipe file must be %q", ContextRuntimeRecipeFile)
	}
	if r.BaseReference == "" || r.BaseReference == BuiltinImageSelector {
		return fmt.Errorf("runtime recipe base reference must be a local or OCI image")
	}
	if err := ValidateImageSelector(r.BaseReference); err != nil {
		return fmt.Errorf("runtime recipe base reference: %w", err)
	}
	if r.SourceDigest != "" {
		if err := ValidateDigest(r.SourceDigest); err != nil {
			return fmt.Errorf("runtime recipe source digest: %w", err)
		}
	}
	if r.LastBuild != nil {
		if err := r.LastBuild.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ContextRuntimeReport is the safe projection of runtime recipe state. The
// resolved Dockerfile path is host diagnostic metadata, never persisted in
// the manifest or mounted into a Workspace.
type ContextRuntimeReport struct {
	Kind          ContextRuntimeKind   `json:"kind"`
	Status        ContextRuntimeStatus `json:"status"`
	Dockerfile    string               `json:"dockerfile,omitempty"`
	BaseReference string               `json:"base_reference,omitempty"`
	SourceDigest  string               `json:"source_digest,omitempty"`
	ImageDigest   string               `json:"image_digest,omitempty"`
}

func (r ContextRuntimeReport) Validate() error {
	if r.Kind == "" && r.Status == "" {
		return nil // schema-1 fixture compatibility; infrastructure fills this in.
	}
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if err := r.Status.Validate(); err != nil {
		return err
	}
	if r.Kind == ContextRuntimeKindOfficial && r.Status != ContextRuntimeStatusOfficial {
		return fmt.Errorf("official runtime must have official status")
	}
	if r.Kind == ContextRuntimeKindDockerfile && r.Status == ContextRuntimeStatusOfficial {
		return fmt.Errorf("Dockerfile runtime cannot have official status")
	}
	if r.Dockerfile != "" && (!filepath.IsAbs(r.Dockerfile) || filepath.Clean(r.Dockerfile) != r.Dockerfile) {
		return fmt.Errorf("runtime Dockerfile must be a canonical absolute path")
	}
	if r.BaseReference != "" {
		if err := ValidateImageSelector(r.BaseReference); err != nil {
			return err
		}
	}
	if r.SourceDigest != "" {
		if err := ValidateDigest(r.SourceDigest); err != nil {
			return fmt.Errorf("runtime source digest: %w", err)
		}
	}
	if r.ImageDigest != "" {
		if err := ValidateDigest(r.ImageDigest); err != nil {
			return fmt.Errorf("runtime image digest: %w", err)
		}
	}
	return nil
}

// ValidateDigest accepts the immutable sha256 identity used by local OCI
// images and Context recipe sources.
func ValidateDigest(value string) error {
	if !digestPattern.MatchString(value) {
		return fmt.Errorf("digest must be sha256 followed by 64 lowercase hex characters")
	}
	return nil
}

// ContextManifest is the trusted, secret-free logical composition record.
// Paths are deliberately resolved by infrastructure rather than persisted in
// the manifest so stores remain independently protected.
type ContextManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Name          string                `json:"name"`
	AgentProfile  string                `json:"agent_profile"`
	Image         string                `json:"image"`
	PolicyMode    ContextPolicyMode     `json:"policy_mode"`
	Runtime       *ContextRuntimeRecipe `json:"runtime,omitempty"`
}

func (m ContextManifest) Validate() error {
	if m.SchemaVersion != ContextSchemaVersion && m.SchemaVersion != LegacyContextSchemaVersion {
		return fmt.Errorf("context schema version must be %d or %d", LegacyContextSchemaVersion, ContextSchemaVersion)
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
	if err := m.PolicyMode.Validate(); err != nil {
		return err
	}
	if m.Runtime != nil {
		if err := m.Runtime.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ContextStorePaths are trusted infrastructure-resolved paths. They are
// included in host diagnostics, never in Workspace mounts as a whole.
type ContextStorePaths struct {
	PolicyDirectory     string `json:"policy_directory"`
	CredentialConfig    string `json:"credential_config"`
	CredentialDirectory string `json:"credential_directory"`
	RuntimeDirectory    string `json:"runtime_directory,omitempty"`
	RuntimeDockerfile   string `json:"runtime_dockerfile,omitempty"`
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
	if (p.RuntimeDirectory == "") != (p.RuntimeDockerfile == "") {
		return fmt.Errorf("context runtime paths must be provided together")
	}
	for name, value := range map[string]string{
		"runtime directory":  p.RuntimeDirectory,
		"runtime Dockerfile": p.RuntimeDockerfile,
	} {
		if value != "" && (!filepath.IsAbs(value) || filepath.Clean(value) != value) {
			return fmt.Errorf("context %s must be a canonical absolute path", name)
		}
	}
	return nil
}

// ContextSummary is one item in the complete local Context collection.
type ContextSummary struct {
	Name          string               `json:"name"`
	Active        bool                 `json:"active"`
	AgentProfile  string               `json:"agent_profile"`
	Image         string               `json:"image"`
	PolicyMode    ContextPolicyMode    `json:"policy_mode"`
	RuntimeStatus ContextRuntimeStatus `json:"runtime_status,omitempty"`
}

func (s ContextSummary) Validate() error {
	manifest := ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		Name:          s.Name,
		AgentProfile:  s.AgentProfile,
		Image:         s.Image,
		PolicyMode:    s.PolicyMode,
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if s.RuntimeStatus != "" {
		if err := s.RuntimeStatus.Validate(); err != nil {
			return err
		}
	}
	return nil
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
	Task         string               `json:"task"`
	Name         string               `json:"name"`
	Active       bool                 `json:"active"`
	AgentProfile string               `json:"agent_profile"`
	Image        string               `json:"image"`
	PolicyMode   ContextPolicyMode    `json:"policy_mode"`
	Stores       ContextStorePaths    `json:"stores"`
	Runtime      ContextRuntimeReport `json:"runtime"`
}

func (r ContextReport) Validate() error {
	if r.Task != TaskContextShow && r.Task != TaskContextCreate && r.Task != TaskContextUse &&
		r.Task != TaskRuntimeInit && r.Task != TaskRuntimeBuild {
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
	if err := r.Stores.Validate(); err != nil {
		return err
	}
	return r.Runtime.Validate()
}
