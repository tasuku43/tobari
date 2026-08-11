package tobari

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ContextSchemaVersion        = 5
	LegacyContextSchemaVersion  = 1
	LegacyContextSchemaVersion2 = 2
	LegacyContextSchemaVersion3 = 3
	LegacyContextSchemaVersion4 = 4
	DefaultContextName          = "default"

	TaskContextList   = "context.list"
	TaskContextShow   = "context.show"
	TaskContextCreate = "context.create"
	TaskContextUse    = "context.use"
	TaskConfigShell   = "config.shell"
	TaskConfigGit     = "config.git"
	TaskRuntimeInit   = "runtime.init"
	TaskRuntimeBuild  = "runtime.build"

	ContextCatalogTargetKind        = "contexts"
	ContextCatalogTargetID          = "context-catalog"
	ContextTargetKind               = "context"
	ActiveContextTargetID           = "active-context"
	ContextRuntimeTargetKind        = "context-runtime"
	ActiveContextRuntimeID          = "active-context-runtime"
	ContextRuntimeRecipeFile        = "runtime/Dockerfile"
	OfficialRuntimeBase             = "ghcr.io/tasuku43/tobari/runtime:latest"
	ContextShellTargetKind          = "context-shell-environment"
	ContextShellTargetID            = "context-shell-environment"
	MaxContextShellValueBytes       = 4096
	ContextGitIdentityTargetKind    = "context-git-identity"
	ContextGitIdentityTargetID      = "context-git-identity"
	MaxContextGitIdentityValueBytes = 4096
)

// ContextObservationState distinguishes durable Context authority from
// display-only first-use defaults and stored manifests that still require an
// authorized migration. Only Persisted may supply authority to a mutation.
type ContextObservationState string

const (
	ContextObservationPersisted        ContextObservationState = "persisted"
	ContextObservationSyntheticDefault ContextObservationState = "synthetic_default"
	ContextObservationLegacyUnmigrated ContextObservationState = "legacy_unmigrated"
)

func (s ContextObservationState) Validate() error {
	switch s {
	case ContextObservationPersisted, ContextObservationSyntheticDefault, ContextObservationLegacyUnmigrated:
		return nil
	default:
		return fmt.Errorf("Context observation state is invalid: %q", s)
	}
}

// ContextObservation carries authority only when State is persisted. Display
// names for fresh or legacy state cannot be passed to a mutation as a stable
// Context binding.
type ContextObservation struct {
	State    ContextObservationState
	Name     string
	Manifest *ContextManifest
}

func (o ContextObservation) Validate() error {
	if err := o.State.Validate(); err != nil {
		return err
	}
	if err := ValidateName(o.Name); err != nil {
		return err
	}
	if o.State == ContextObservationPersisted {
		if o.Manifest == nil || o.Manifest.Name != o.Name {
			return fmt.Errorf("persisted Context observation lacks its authoritative manifest")
		}
		return o.Manifest.Validate()
	}
	if o.Manifest != nil {
		return fmt.Errorf("non-persisted Context observation cannot carry authority")
	}
	if o.State == ContextObservationSyntheticDefault && o.Name != DefaultContextName {
		return fmt.Errorf("synthetic Context observation must select the default display name")
	}
	return nil
}

var (
	ErrContextExists        = errors.New("Context already exists")
	ErrContextNotFound      = errors.New("Context does not exist")
	ErrRuntimeRecipeExists  = errors.New("Context runtime recipe already exists")
	ErrRuntimeRecipeMissing = errors.New("Context runtime recipe does not exist")
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var contextIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidateContextID accepts the stable UUIDv7 authority identity stored in a
// Context manifest. Display names are intentionally not accepted here.
func ValidateContextID(id string) error {
	if !contextIDPattern.MatchString(id) {
		return fmt.Errorf("Context ID is invalid")
	}
	return nil
}

// NewContextID creates a host-issued stable Context identity.
func NewContextID(now time.Time, source io.Reader) (string, error) {
	if source == nil {
		return "", fmt.Errorf("Context ID entropy source is required")
	}
	if now.UnixMilli() < 0 || now.UnixMilli() >= 1<<48 {
		return "", fmt.Errorf("Context ID timestamp is outside UUIDv7 range")
	}
	var value [16]byte
	milliseconds := uint64(now.UnixMilli())
	for index := 5; index >= 0; index-- {
		value[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	if _, err := io.ReadFull(source, value[6:]); err != nil {
		return "", fmt.Errorf("read Context ID entropy: %w", err)
	}
	value[6] = 0x70 | (value[6] & 0x0f)
	value[8] = 0x80 | (value[8] & 0x3f)
	id := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
	if err := ValidateContextID(id); err != nil {
		return "", err
	}
	return id, nil
}

func NewProductionContextID() (string, error) {
	return NewContextID(time.Now().UTC(), rand.Reader)
}

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

// ContextClusterStatus reports how context use relates to the installation-wide
// shared cluster. It is explicit in the result so callers do not have to infer
// whether selecting a Context also applied its policy and credential mounts.
type ContextClusterStatus string

const (
	ContextClusterStatusNotApplicable     ContextClusterStatus = "not_applicable"
	ContextClusterStatusNotConfigured     ContextClusterStatus = "not_configured"
	ContextClusterStatusNotRunning        ContextClusterStatus = "not_running"
	ContextClusterStatusAlreadyReady      ContextClusterStatus = "already_ready"
	ContextClusterStatusReconciled        ContextClusterStatus = "reconciled"
	ContextClusterStatusDefaultUpdated    ContextClusterStatus = "default_updated"
	ContextClusterStatusRequiresReconcile ContextClusterStatus = "requires_reconcile"
)

func (s ContextClusterStatus) Validate() error {
	switch s {
	case ContextClusterStatusNotApplicable, ContextClusterStatusNotConfigured,
		ContextClusterStatusNotRunning, ContextClusterStatusAlreadyReady,
		ContextClusterStatusReconciled, ContextClusterStatusDefaultUpdated,
		ContextClusterStatusRequiresReconcile:
		return nil
	default:
		return fmt.Errorf("context cluster status is invalid: %q", s)
	}
}

// ContextShellEnvironmentSource selects how one allowlisted shell variable is
// resolved for each new interactive Workspace session. Default is a public
// update value only; manifests persist only inherit and literal overrides.
type ContextShellEnvironmentSource string

const (
	ContextShellEnvironmentDefault ContextShellEnvironmentSource = "default"
	ContextShellEnvironmentInherit ContextShellEnvironmentSource = "inherit"
	ContextShellEnvironmentLiteral ContextShellEnvironmentSource = "literal"
)

var contextShellEnvironmentVariables = []string{"COLORTERM", "NO_COLOR", "PS1", "TERM"}

func ContextShellEnvironmentVariables() []string {
	return append([]string(nil), contextShellEnvironmentVariables...)
}

// InitialContextShellEnvironment makes exported PS1 inheritance the ordinary
// Context behavior while retaining the built-in prompt when the launcher did
// not export PS1.
func InitialContextShellEnvironment() []ContextShellEnvironmentSetting {
	return []ContextShellEnvironmentSetting{{Variable: "PS1", Source: ContextShellEnvironmentInherit}}
}

func ValidateContextShellEnvironmentVariable(value string) error {
	for _, allowed := range contextShellEnvironmentVariables {
		if value == allowed {
			return nil
		}
	}
	return fmt.Errorf("Context shell environment variable %q is not allowlisted", value)
}

func ValidateContextShellEnvironmentValue(value string) error {
	if !utf8.ValidString(value) || len(value) > MaxContextShellValueBytes || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("shell environment value must be valid UTF-8 without NUL and at most %d bytes", MaxContextShellValueBytes)
	}
	return nil
}

// ContextShellEnvironmentSetting is one persisted or reported variable
// policy. Value is present only for literal, including an explicit empty value.
type ContextShellEnvironmentSetting struct {
	Variable string                        `json:"variable"`
	Source   ContextShellEnvironmentSource `json:"source"`
	Value    *string                       `json:"value,omitempty"`
}

func (s ContextShellEnvironmentSetting) Validate(allowDefault bool) error {
	if err := ValidateContextShellEnvironmentVariable(s.Variable); err != nil {
		return err
	}
	switch s.Source {
	case ContextShellEnvironmentDefault:
		if !allowDefault {
			return fmt.Errorf("default shell environment source is not persisted")
		}
		if s.Value != nil {
			return fmt.Errorf("default shell environment source cannot contain a value")
		}
	case ContextShellEnvironmentInherit:
		if s.Value != nil {
			return fmt.Errorf("inherited shell environment source cannot contain a value")
		}
	case ContextShellEnvironmentLiteral:
		if s.Value == nil {
			return fmt.Errorf("literal shell environment source requires a value")
		}
		if err := ValidateContextShellEnvironmentValue(*s.Value); err != nil {
			return fmt.Errorf("literal %w", err)
		}
	default:
		return fmt.Errorf("Context shell environment source is invalid: %q", s.Source)
	}
	return nil
}

func validateContextShellEnvironment(settings []ContextShellEnvironmentSetting, complete bool) error {
	seen := make(map[string]struct{}, len(settings))
	for _, setting := range settings {
		if err := setting.Validate(complete); err != nil {
			return err
		}
		if _, duplicate := seen[setting.Variable]; duplicate {
			return fmt.Errorf("Context shell environment variable %q is duplicated", setting.Variable)
		}
		seen[setting.Variable] = struct{}{}
	}
	if complete && len(seen) != len(contextShellEnvironmentVariables) {
		return fmt.Errorf("Context shell environment report must contain every allowlisted variable")
	}
	return nil
}

// CompleteContextShellEnvironment expands persisted overrides into the fixed
// public variable inventory so callers never infer a missing setting.
func CompleteContextShellEnvironment(overrides []ContextShellEnvironmentSetting) ([]ContextShellEnvironmentSetting, error) {
	if err := validateContextShellEnvironment(overrides, false); err != nil {
		return nil, err
	}
	byName := make(map[string]ContextShellEnvironmentSetting, len(overrides))
	for _, setting := range overrides {
		byName[setting.Variable] = setting
	}
	complete := make([]ContextShellEnvironmentSetting, 0, len(contextShellEnvironmentVariables))
	for _, variable := range contextShellEnvironmentVariables {
		setting, found := byName[variable]
		if !found {
			setting = ContextShellEnvironmentSetting{Variable: variable, Source: ContextShellEnvironmentDefault}
		}
		complete = append(complete, setting)
	}
	return complete, nil
}

func DefaultContextShellEnvironmentReport() []ContextShellEnvironmentSetting {
	complete := make([]ContextShellEnvironmentSetting, 0, len(contextShellEnvironmentVariables))
	for _, variable := range contextShellEnvironmentVariables {
		complete = append(complete, ContextShellEnvironmentSetting{
			Variable: variable,
			Source:   ContextShellEnvironmentDefault,
		})
	}
	return complete
}

// ApplyContextShellEnvironmentSetting returns a deterministic persisted
// override list. Selecting default removes that variable's override.
func ApplyContextShellEnvironmentSetting(
	overrides []ContextShellEnvironmentSetting, change ContextShellEnvironmentSetting,
) ([]ContextShellEnvironmentSetting, error) {
	return ApplyContextShellEnvironmentSettings(overrides, []ContextShellEnvironmentSetting{change})
}

// ApplyContextShellEnvironmentSettings validates a complete staged change set
// before returning one deterministic persisted override list. No partial
// result is returned when any change is invalid or targets a variable twice.
func ApplyContextShellEnvironmentSettings(
	overrides []ContextShellEnvironmentSetting, changes []ContextShellEnvironmentSetting,
) ([]ContextShellEnvironmentSetting, error) {
	if err := validateContextShellEnvironment(overrides, false); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("Context shell environment change set is empty")
	}
	seenChanges := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if err := change.Validate(true); err != nil {
			return nil, err
		}
		if _, duplicate := seenChanges[change.Variable]; duplicate {
			return nil, fmt.Errorf("Context shell environment change for %q is duplicated", change.Variable)
		}
		seenChanges[change.Variable] = struct{}{}
	}
	byName := make(map[string]ContextShellEnvironmentSetting, len(overrides)+len(changes))
	for _, setting := range overrides {
		byName[setting.Variable] = setting
	}
	for _, change := range changes {
		if change.Source == ContextShellEnvironmentDefault {
			delete(byName, change.Variable)
		} else {
			byName[change.Variable] = change
		}
	}
	result := make([]ContextShellEnvironmentSetting, 0, len(byName))
	for _, variable := range contextShellEnvironmentVariables {
		if setting, found := byName[variable]; found {
			result = append(result, setting)
		}
	}
	return result, nil
}

// ContextGitIdentitySource selects the narrow Git identity projection owned by
// a Context. Default is represented explicitly in reports and omitted from
// manifests; inherit and literal are the only persisted overrides.
type ContextGitIdentitySource string

const (
	ContextGitIdentityDefault ContextGitIdentitySource = "default"
	ContextGitIdentityInherit ContextGitIdentitySource = "inherit"
	ContextGitIdentityLiteral ContextGitIdentitySource = "literal"
)

// ContextGitIdentitySetting is an atomic user.name and user.email policy.
// Name and Email are present together only for a literal setting.
type ContextGitIdentitySetting struct {
	Source ContextGitIdentitySource `json:"source"`
	Name   *string                  `json:"name"`
	Email  *string                  `json:"email"`
}

func ValidateContextGitIdentityValue(value string) error {
	if value == "" || !utf8.ValidString(value) || len(value) > MaxContextGitIdentityValueBytes {
		return fmt.Errorf("Git identity value must be non-empty valid UTF-8 and at most %d bytes", MaxContextGitIdentityValueBytes)
	}
	if strings.IndexFunc(value, func(character rune) bool {
		return character <= '\u001f' ||
			(character >= '\u007f' && character <= '\u009f') ||
			character == '\u2028' || character == '\u2029' ||
			unicode.Is(unicode.Cf, character)
	}) >= 0 {
		return fmt.Errorf("Git identity value cannot contain control, format, or Unicode line-separator characters")
	}
	return nil
}

func (s ContextGitIdentitySetting) Validate(allowDefault bool) error {
	switch s.Source {
	case ContextGitIdentityDefault:
		if !allowDefault {
			return fmt.Errorf("default Git identity source is not persisted")
		}
		if s.Name != nil || s.Email != nil {
			return fmt.Errorf("default Git identity source cannot contain name or email")
		}
	case ContextGitIdentityInherit:
		if s.Name != nil || s.Email != nil {
			return fmt.Errorf("inherited Git identity source cannot contain name or email")
		}
	case ContextGitIdentityLiteral:
		if s.Name == nil || s.Email == nil {
			return fmt.Errorf("literal Git identity source requires both name and email")
		}
		if err := ValidateContextGitIdentityValue(*s.Name); err != nil {
			return fmt.Errorf("literal Git identity name: %w", err)
		}
		if err := ValidateContextGitIdentityValue(*s.Email); err != nil {
			return fmt.Errorf("literal Git identity email: %w", err)
		}
	default:
		return fmt.Errorf("Context Git identity source is invalid: %q", s.Source)
	}
	return nil
}

func DefaultContextGitIdentityReport() ContextGitIdentitySetting {
	return ContextGitIdentitySetting{Source: ContextGitIdentityDefault}
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
	SchemaVersion    int                              `json:"schema_version"`
	ID               string                           `json:"id"`
	Name             string                           `json:"name"`
	AgentProfile     string                           `json:"agent_profile"`
	Image            string                           `json:"image"`
	PolicyMode       ContextPolicyMode                `json:"policy_mode"`
	Runtime          *ContextRuntimeRecipe            `json:"runtime,omitempty"`
	ShellEnvironment []ContextShellEnvironmentSetting `json:"shell_environment,omitempty"`
	GitIdentity      *ContextGitIdentitySetting       `json:"git_identity,omitempty"`
}

func (m ContextManifest) Validate() error {
	if m.SchemaVersion != ContextSchemaVersion && m.SchemaVersion != LegacyContextSchemaVersion &&
		m.SchemaVersion != LegacyContextSchemaVersion2 && m.SchemaVersion != LegacyContextSchemaVersion3 &&
		m.SchemaVersion != LegacyContextSchemaVersion4 {
		return fmt.Errorf(
			"context schema version must be %d, %d, %d, %d, or %d",
			LegacyContextSchemaVersion, LegacyContextSchemaVersion2, LegacyContextSchemaVersion3,
			LegacyContextSchemaVersion4, ContextSchemaVersion,
		)
	}
	if m.SchemaVersion == ContextSchemaVersion || m.SchemaVersion == LegacyContextSchemaVersion4 ||
		m.SchemaVersion == LegacyContextSchemaVersion3 {
		if err := ValidateContextID(m.ID); err != nil {
			return err
		}
	} else if m.ID != "" {
		return fmt.Errorf("legacy Context manifest cannot contain a Context ID")
	}
	if m.SchemaVersion != ContextSchemaVersion && m.SchemaVersion != LegacyContextSchemaVersion4 &&
		len(m.ShellEnvironment) != 0 {
		return fmt.Errorf("legacy Context manifest cannot contain shell environment settings")
	}
	if m.SchemaVersion != ContextSchemaVersion && m.GitIdentity != nil {
		return fmt.Errorf("legacy Context manifest cannot contain a Git identity setting")
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
	if err := validateContextShellEnvironment(m.ShellEnvironment, false); err != nil {
		return err
	}
	if m.GitIdentity != nil {
		if err := m.GitIdentity.Validate(false); err != nil {
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
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	ContextState  ContextObservationState `json:"context_state"`
	Active        bool                    `json:"active"`
	AgentProfile  string                  `json:"agent_profile"`
	Image         string                  `json:"image"`
	PolicyMode    ContextPolicyMode       `json:"policy_mode"`
	RuntimeStatus ContextRuntimeStatus    `json:"runtime_status,omitempty"`
}

func (s ContextSummary) Validate() error {
	if err := s.ContextState.Validate(); err != nil {
		return err
	}
	if s.ContextState == ContextObservationSyntheticDefault {
		return fmt.Errorf("synthetic default is not a configured Context item")
	}
	manifest := ContextManifest{
		SchemaVersion: ContextSchemaVersion,
		ID:            s.ID,
		Name:          s.Name,
		AgentProfile:  s.AgentProfile,
		Image:         s.Image,
		PolicyMode:    s.PolicyMode,
	}
	if s.ContextState == ContextObservationLegacyUnmigrated {
		if s.ID != "" {
			return fmt.Errorf("legacy unmigrated Context cannot claim a stable ID")
		}
		if err := ValidateName(s.Name); err != nil {
			return err
		}
		if s.AgentProfile == "" || s.Image == "" || s.PolicyMode.Validate() != nil {
			return fmt.Errorf("legacy unmigrated Context display metadata is invalid")
		}
	} else if err := manifest.Validate(); err != nil {
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
	Task         string                  `json:"task"`
	ContextState ContextObservationState `json:"context_state"`
	Active       string                  `json:"active"`
	Items        []ContextSummary        `json:"items"`
}

func (r ContextListResult) Validate() error {
	if r.Task != TaskContextList || r.Items == nil || r.Active == "" {
		return fmt.Errorf("context list task or scope is invalid")
	}
	if err := r.ContextState.Validate(); err != nil {
		return err
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
			if item.ContextState != r.ContextState {
				return fmt.Errorf("active context state does not match the list observation state")
			}
		}
	}
	if r.ContextState == ContextObservationSyntheticDefault {
		if r.Active != DefaultContextName || activeCount != 0 || len(r.Items) != 0 {
			return fmt.Errorf("synthetic default Context list cannot claim an active configured Context")
		}
		return nil
	}
	if _, exists := seen[r.Active]; !exists || activeCount != 1 {
		return fmt.Errorf("context list must contain exactly one active context")
	}
	return nil
}

const (
	ContextAuthBrokerNotApplicable = "not_applicable"
	ContextAuthBrokerReady         = "ready"
	ContextAuthBrokerLocked        = "locked"
	ContextAuthBrokerUnavailable   = "unavailable"

	ContextAuthProviderConfigured    = "configured"
	ContextAuthProviderNotConfigured = "not_configured"
	ContextAuthProviderUnavailable   = "unavailable"
)

// ContextAuthProvider reports one provider's non-secret Context-owned state.
// It never contains a project handle or an upstream credential.
type ContextAuthProvider struct {
	Provider           string  `json:"provider"`
	State              string  `json:"state"`
	AccountLabel       *string `json:"account_label"`
	CredentialRevision string  `json:"credential_revision"`
}

func (p ContextAuthProvider) Validate() error {
	if len(p.Provider) > 64 || !regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`).MatchString(p.Provider) {
		return fmt.Errorf("Context authentication provider ID is invalid")
	}
	switch p.State {
	case ContextAuthProviderConfigured:
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`).MatchString(p.CredentialRevision) {
			return fmt.Errorf("configured Context authentication provider revision is invalid")
		}
	case ContextAuthProviderNotConfigured, ContextAuthProviderUnavailable:
		if p.AccountLabel != nil || p.CredentialRevision != "" {
			return fmt.Errorf("unconfigured Context authentication provider contains configured metadata")
		}
	default:
		return fmt.Errorf("Context authentication provider state is invalid")
	}
	if p.AccountLabel != nil {
		value := *p.AccountLabel
		if value == "" || len(value) > 128 || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
			strings.IndexFunc(value, func(character rune) bool {
				return character < ' ' || character == '\u007f' || character == '\u2028' || character == '\u2029'
			}) >= 0 {
			return fmt.Errorf("Context authentication account label is invalid")
		}
	}
	return nil
}

// ContextAuthentication is an explicit observation. not_applicable is used
// only by mutation results whose task did not inspect the running broker.
type ContextAuthentication struct {
	BrokerState string                `json:"broker_state"`
	Providers   []ContextAuthProvider `json:"providers"`
}

func (a ContextAuthentication) Validate(observed bool) error {
	if !observed {
		if a.BrokerState != ContextAuthBrokerNotApplicable || a.Providers != nil {
			return fmt.Errorf("unobserved Context authentication state is invalid")
		}
		return nil
	}
	switch a.BrokerState {
	case ContextAuthBrokerReady, ContextAuthBrokerLocked, ContextAuthBrokerUnavailable:
	default:
		return fmt.Errorf("observed Context authentication broker state is invalid")
	}
	if a.Providers == nil {
		return fmt.Errorf("observed Context authentication provider collection is absent")
	}
	seen := make(map[string]struct{}, len(a.Providers))
	for _, provider := range a.Providers {
		if err := provider.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[provider.Provider]; duplicate {
			return fmt.Errorf("Context authentication provider is duplicated")
		}
		seen[provider.Provider] = struct{}{}
		if a.BrokerState != ContextAuthBrokerReady && provider.State != ContextAuthProviderUnavailable {
			return fmt.Errorf("unready Context authentication broker has an available provider state")
		}
	}
	return nil
}

// ContextReport is the complete selected Context view.
type ContextReport struct {
	Task             string                           `json:"task"`
	ContextState     ContextObservationState          `json:"context_state"`
	ID               string                           `json:"id"`
	Name             string                           `json:"name"`
	Active           bool                             `json:"active"`
	AgentProfile     string                           `json:"agent_profile"`
	Image            string                           `json:"image"`
	PolicyMode       ContextPolicyMode                `json:"policy_mode"`
	ShellEnvironment []ContextShellEnvironmentSetting `json:"shell_environment"`
	GitIdentity      ContextGitIdentitySetting        `json:"git_identity"`
	Stores           ContextStorePaths                `json:"stores"`
	Runtime          ContextRuntimeReport             `json:"runtime"`
	Cluster          ContextClusterStatus             `json:"cluster"`
	Authentication   ContextAuthentication            `json:"authentication"`
}

func (r ContextReport) Validate() error {
	if r.Task != TaskContextShow && r.Task != TaskContextCreate && r.Task != TaskContextUse &&
		r.Task != TaskConfigShell && r.Task != TaskConfigGit && r.Task != TaskRuntimeInit && r.Task != TaskRuntimeBuild {
		return fmt.Errorf("context report task is invalid")
	}
	if err := r.ContextState.Validate(); err != nil {
		return err
	}
	if r.ContextState == ContextObservationSyntheticDefault {
		if r.Task != TaskContextShow || r.Name != DefaultContextName || !r.Active || r.ID != "" || r.Stores != (ContextStorePaths{}) {
			return fmt.Errorf("synthetic default Context report claims persisted authority")
		}
		if r.AgentProfile == "" || r.Image == "" || r.PolicyMode.Validate() != nil {
			return fmt.Errorf("synthetic default Context display metadata is invalid")
		}
	} else if r.ContextState == ContextObservationLegacyUnmigrated {
		if r.Task != TaskContextShow || r.ID != "" {
			return fmt.Errorf("legacy unmigrated Context report claims stable authority")
		}
		if err := ValidateName(r.Name); err != nil {
			return err
		}
		if r.AgentProfile == "" || r.Image == "" || r.PolicyMode.Validate() != nil {
			return fmt.Errorf("legacy unmigrated Context display metadata is invalid")
		}
		if err := r.Stores.Validate(); err != nil {
			return err
		}
	} else {
		manifest := ContextManifest{
			SchemaVersion: ContextSchemaVersion,
			ID:            r.ID,
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
	}
	if err := r.Runtime.Validate(); err != nil {
		return err
	}
	if err := validateContextShellEnvironment(r.ShellEnvironment, true); err != nil {
		return err
	}
	if err := r.GitIdentity.Validate(true); err != nil {
		return err
	}
	if err := r.Authentication.Validate(r.Task == TaskContextShow); err != nil {
		return err
	}
	return r.Cluster.Validate()
}
