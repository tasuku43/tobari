package tobari

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// WorkspaceManifestRevision identifies one complete immutable desired
// declaration and each activation slice. Generation is correlation only; the
// sha256 values are semantic authority.
type WorkspaceManifestRevision struct {
	Generation                uint64 `json:"manifest_generation"`
	Revision                  string `json:"manifest_revision"`
	BoundaryRevision          string `json:"boundary_revision"`
	ClusterProjectionRevision string `json:"cluster_projection_revision"`
	EntryRevision             string `json:"entry_revision"`
	SessionDefaultsRevision   string `json:"session_defaults_revision"`
	CreationDefaultsRevision  string `json:"creation_defaults_revision"`
}

func (r WorkspaceManifestRevision) Validate() error {
	if r.Generation == 0 {
		return fmt.Errorf("Manifest generation must be positive")
	}
	for name, value := range map[string]string{
		"Manifest": r.Revision, "Boundary": r.BoundaryRevision,
		"cluster projection": r.ClusterProjectionRevision, "entry": r.EntryRevision,
		"session defaults": r.SessionDefaultsRevision, "creation defaults": r.CreationDefaultsRevision,
	} {
		if err := ValidateDigest(value); err != nil {
			return fmt.Errorf("%s revision: %w", name, err)
		}
	}
	return nil
}

const (
	WorkspaceManifestSchemaVersion = 2
	DefaultManifestName            = "default"

	TaskManifestList       = "manifest.list"
	TaskManifestShow       = "manifest.show"
	TaskManifestCreate     = "manifest.create"
	TaskManifestDelete     = "manifest.delete"
	TaskManifestDefaultSet = "manifest.default.set"
	TaskManifestRuntimeSet = "manifest.runtime.set"
	TaskConfigShell        = "config.shell"
	TaskConfigGit          = "config.git"
	TaskConfigBootstrapAWS = "config.bootstrap.aws"
	TaskConfigBootstrapEKS = "config.bootstrap.kubernetes.eks"
	TaskRuntimeInit        = "runtime.init"
	TaskRuntimeBuild       = "runtime.build"

	ManifestCatalogTargetKind        = "workspace-manifests"
	ManifestCatalogTargetID          = "workspace-manifest-catalog"
	ManifestTargetKind               = "workspace-manifest"
	DefaultManifestSelectionTargetID = "default-manifest-selection"
	ManifestRuntimeTargetKind        = "workspace-manifest-runtime"
	ActiveContextRuntimeID           = "default-manifest-runtime"
	ManifestRuntimeBindingTargetKind = "workspace-manifest-runtime-binding"
	ManifestRuntimeBindingTargetID   = "workspace-manifest-runtime-binding"
	ManifestRuntimeRecipeFile        = "runtime/Dockerfile"
	OfficialRuntimeBase              = "tobari-runtime:base"
	ManifestShellTargetKind          = "workspace-manifest-shell-environment"
	ManifestShellTargetID            = "workspace-manifest-shell-environment"
	MaxContextShellValueBytes        = 4096
	ManifestGitIdentityTargetKind    = "workspace-manifest-git-identity"
	ManifestGitIdentityTargetID      = "workspace-manifest-git-identity"
	MaxContextGitIdentityValueBytes  = 4096
)

// ManifestNativeReadiness selects the trusted binary's finite native-client
// compatibility overlay independently from the Context-owned policy snapshot.
type ManifestNativeReadiness string

const (
	ManifestNativeReadinessEnabled  ManifestNativeReadiness = "enabled"
	ManifestNativeReadinessDisabled ManifestNativeReadiness = "disabled"
)

func (r ManifestNativeReadiness) Validate() error {
	switch r {
	case ManifestNativeReadinessEnabled, ManifestNativeReadinessDisabled:
		return nil
	default:
		return fmt.Errorf("context native readiness is invalid: %q", r)
	}
}

// ResolveContextNativeReadiness resolves the explicit readiness setting.
func ResolveContextNativeReadiness(value ManifestNativeReadiness) (ManifestNativeReadiness, error) {
	if value != "" {
		return value, value.Validate()
	}
	return ManifestNativeReadinessEnabled, nil
}

// ManifestCreateComposition is the complete policy selection used to create one
// immutable Context snapshot. MethodPolicy is nil when direct mode retains the
// fixed built-in default method policy.
type ManifestCreateComposition struct {
	NativeReadiness  ManifestNativeReadiness
	MethodPolicy     *ManifestMethodPolicy
	Bootstrap        *ManifestBootstrapSnapshot
	RuntimeSelection string
	CopyFrom         *ManifestCopySnapshot
}

func (c ManifestCreateComposition) Validate() error {
	if err := c.NativeReadiness.Validate(); err != nil {
		return err
	}
	if c.MethodPolicy != nil {
		if err := c.MethodPolicy.Validate(); err != nil {
			return err
		}
	}
	if c.Bootstrap != nil {
		if err := c.Bootstrap.Validate(); err != nil {
			return err
		}
	}
	if _, _, err := ParseRuntimeSelection(c.RuntimeSelection); err != nil {
		return err
	}
	if c.CopyFrom != nil {
		if err := c.CopyFrom.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c ManifestCreateComposition) Clone() ManifestCreateComposition {
	result := c
	if c.MethodPolicy != nil {
		policy := c.MethodPolicy.Clone()
		result.MethodPolicy = &policy
	}
	if c.Bootstrap != nil {
		bootstrap := c.Bootstrap.Clone()
		result.Bootstrap = &bootstrap
	}
	if c.CopyFrom != nil {
		base := c.CopyFrom.Clone()
		result.CopyFrom = &base
	}
	return result
}

// ManifestCopySnapshot is a validated, read-only snapshot used to initialize a
// standalone Context draft. Revision binds creation to all copyable Base bytes;
// it is not persisted as lineage in the created Context.
type ManifestCopySnapshot struct {
	ID               string
	Name             string
	Revision         string
	Desired          WorkspaceManifestRevision
	PolicyMode       ManifestPolicyMode
	SourceAccess     ManifestSourceAccess
	NativeReadiness  ManifestNativeReadiness
	MethodPolicy     ManifestMethodPolicy
	RuntimeSelection string
	RuntimeBinding   RuntimeBinding
	ShellEnvironment []ManifestShellEnvironmentSetting
	GitIdentity      ManifestGitIdentitySetting
	Bootstrap        *ManifestBootstrapSnapshot
}

func (b ManifestCopySnapshot) Validate() error {
	if err := ValidateWorkspaceManifestID(b.ID); err != nil {
		return err
	}
	if err := ValidateName(b.Name); err != nil {
		return err
	}
	if !digestPattern.MatchString(b.Revision) {
		return fmt.Errorf("Workspace Manifest create Base revision is invalid")
	}
	if err := b.Desired.Validate(); err != nil || b.Desired.Revision != b.Revision {
		return fmt.Errorf("Manifest copy source revision is invalid")
	}
	if err := b.PolicyMode.Validate(); err != nil {
		return err
	}
	if err := b.SourceAccess.Validate(); err != nil {
		return err
	}
	if err := b.NativeReadiness.Validate(); err != nil {
		return err
	}
	if err := b.MethodPolicy.Validate(); err != nil {
		return err
	}
	if _, _, err := ParseRuntimeSelection(b.RuntimeSelection); err != nil {
		return err
	}
	if err := b.RuntimeBinding.Validate(); err != nil {
		return err
	}
	selection, err := b.RuntimeBinding.Selection()
	if err != nil || selection != b.RuntimeSelection {
		return fmt.Errorf("Manifest copy source Runtime binding is inconsistent")
	}
	if err := validateContextShellEnvironment(b.ShellEnvironment, true); err != nil {
		return err
	}
	if err := b.GitIdentity.Validate(true); err != nil {
		return err
	}
	if b.Bootstrap != nil {
		if err := b.Bootstrap.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (b ManifestCopySnapshot) Clone() ManifestCopySnapshot {
	result := b
	result.MethodPolicy = b.MethodPolicy.Clone()
	result.ShellEnvironment = cloneContextShellEnvironment(b.ShellEnvironment)
	result.GitIdentity = cloneContextGitIdentitySetting(b.GitIdentity)
	if b.Bootstrap != nil {
		bootstrap := b.Bootstrap.Clone()
		result.Bootstrap = &bootstrap
	}
	return result
}

func cloneContextShellEnvironment(settings []ManifestShellEnvironmentSetting) []ManifestShellEnvironmentSetting {
	result := make([]ManifestShellEnvironmentSetting, len(settings))
	for index, setting := range settings {
		result[index] = setting
		if setting.Value != nil {
			value := *setting.Value
			result[index].Value = &value
		}
	}
	return result
}

func cloneContextGitIdentitySetting(setting ManifestGitIdentitySetting) ManifestGitIdentitySetting {
	result := setting
	if setting.Name != nil {
		value := *setting.Name
		result.Name = &value
	}
	if setting.Email != nil {
		value := *setting.Email
		result.Email = &value
	}
	return result
}

// ManifestObservationState distinguishes durable Context authority from a
// display-only first-use default. Only Persisted may supply authority to a mutation.
type ManifestObservationState string

const (
	ManifestObservationPersisted ManifestObservationState = "persisted"
	ManifestObservationAbsent    ManifestObservationState = "absent"
)

func (s ManifestObservationState) Validate() error {
	switch s {
	case ManifestObservationPersisted, ManifestObservationAbsent:
		return nil
	default:
		return fmt.Errorf("Workspace Manifest observation state is invalid: %q", s)
	}
}

// ManifestObservation carries authority only when State is persisted. Display
// names for fresh state cannot be passed to a mutation as a stable Context binding.
type ManifestObservation struct {
	State    ManifestObservationState
	Name     string
	Manifest *WorkspaceManifest
}

func (o ManifestObservation) Validate() error {
	if err := o.State.Validate(); err != nil {
		return err
	}
	if o.State == ManifestObservationAbsent {
		if o.Name != "" || o.Manifest != nil {
			return fmt.Errorf("absent Manifest observation cannot carry display or authority state")
		}
		return nil
	}
	if err := ValidateName(o.Name); err != nil {
		return err
	}
	if o.State == ManifestObservationPersisted {
		if o.Manifest == nil || o.Manifest.Name != o.Name {
			return fmt.Errorf("persisted Workspace Manifest observation lacks its authoritative manifest")
		}
		return o.Manifest.Validate()
	}
	return fmt.Errorf("non-persisted Workspace Manifest observation cannot carry authority")
}

var (
	ErrContextExists                 = errors.New("Workspace Manifest already exists")
	ErrContextNotFound               = errors.New("Workspace Manifest does not exist")
	ErrContextActive                 = errors.New("Workspace Manifest is default")
	ErrContextProtected              = errors.New("Workspace Manifest is protected")
	ErrContextHasWorkspaces          = errors.New("Workspace Manifest has Workspaces")
	ErrManifestCopySourceChanged     = errors.New("Workspace Manifest copy source changed")
	ErrRuntimeRecipeExists           = errors.New("Workspace Manifest runtime recipe already exists")
	ErrRuntimeRecipeMissing          = errors.New("Workspace Manifest runtime recipe does not exist")
	ErrContextBootstrapNotConfigured = errors.New("Workspace Manifest bootstrap is not configured")
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var contextIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidateWorkspaceManifestID accepts the stable UUIDv7 authority identity stored in a
// Context manifest. Display names are intentionally not accepted here.
func ValidateWorkspaceManifestID(id string) error {
	if !contextIDPattern.MatchString(id) {
		return fmt.Errorf("Workspace Manifest ID is invalid")
	}
	return nil
}

// NewWorkspaceManifestID creates a host-issued stable Context identity.
func NewWorkspaceManifestID(now time.Time, source io.Reader) (string, error) {
	if source == nil {
		return "", fmt.Errorf("Workspace Manifest ID entropy source is required")
	}
	if now.UnixMilli() < 0 || now.UnixMilli() >= 1<<48 {
		return "", fmt.Errorf("Workspace Manifest ID timestamp is outside UUIDv7 range")
	}
	var value [16]byte
	milliseconds := uint64(now.UnixMilli())
	for index := 5; index >= 0; index-- {
		value[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	if _, err := io.ReadFull(source, value[6:]); err != nil {
		return "", fmt.Errorf("read Workspace Manifest ID entropy: %w", err)
	}
	value[6] = 0x70 | (value[6] & 0x0f)
	value[8] = 0x80 | (value[8] & 0x3f)
	id := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16])
	if err := ValidateWorkspaceManifestID(id); err != nil {
		return "", err
	}
	return id, nil
}

// ManifestPolicyMode selects the policy-development experience associated with
// a Context. It does not change Gateway authorization by itself.
type ManifestPolicyMode string

const (
	ManifestPolicyModeGuided   ManifestPolicyMode = "guided"
	ManifestPolicyModeAdvanced ManifestPolicyMode = "advanced"
)

func (m ManifestPolicyMode) Validate() error {
	switch m {
	case ManifestPolicyModeGuided, ManifestPolicyModeAdvanced:
		return nil
	default:
		return fmt.Errorf("context policy mode is invalid: %q", m)
	}
}

// ManifestSourceAccess selects the write authority of the one direct project
// source bind. It does not describe the separately writable Workspace home or
// tmpfs mounts.
type ManifestSourceAccess string

const (
	ManifestSourceAccessReadOnly  ManifestSourceAccess = "read-only"
	ManifestSourceAccessReadWrite ManifestSourceAccess = "read-write"
)

func (a ManifestSourceAccess) Validate() error {
	switch a {
	case ManifestSourceAccessReadOnly, ManifestSourceAccessReadWrite:
		return nil
	default:
		return fmt.Errorf("context source access is invalid: %q", a)
	}
}

// ManifestRuntimeKind identifies the source of a Context runtime.
type ManifestRuntimeKind string

const (
	ManifestRuntimeKindOfficial   ManifestRuntimeKind = "official"
	ManifestRuntimeKindDockerfile ManifestRuntimeKind = "dockerfile"
	ManifestRuntimeKindManaged    ManifestRuntimeKind = "managed"
)

func (k ManifestRuntimeKind) Validate() error {
	switch k {
	case ManifestRuntimeKindOfficial, ManifestRuntimeKindDockerfile, ManifestRuntimeKindManaged:
		return nil
	default:
		return fmt.Errorf("context runtime kind is invalid: %q", k)
	}
}

// ManifestRuntimeStatus is the user-facing state of the selected runtime
// recipe. A pending recipe never replaces the last selected image.
type ManifestRuntimeStatus string

const (
	ManifestRuntimeStatusOfficial     ManifestRuntimeStatus = "official"
	ManifestRuntimeStatusPendingBuild ManifestRuntimeStatus = "pending_build"
	ManifestRuntimeStatusReady        ManifestRuntimeStatus = "ready"
	ManifestRuntimeStatusInvalid      ManifestRuntimeStatus = "invalid"
)

func (s ManifestRuntimeStatus) Validate() error {
	switch s {
	case ManifestRuntimeStatusOfficial, ManifestRuntimeStatusPendingBuild,
		ManifestRuntimeStatusReady, ManifestRuntimeStatusInvalid:
		return nil
	default:
		return fmt.Errorf("context runtime status is invalid: %q", s)
	}
}

// ManifestClusterStatus reports how context use relates to the installation-wide
// shared cluster. It is explicit in the result so callers do not have to infer
// whether selecting a Context also applied its policy and credential mounts.
type ManifestClusterStatus string

const (
	ManifestClusterStatusNotApplicable          ManifestClusterStatus = "not_applicable"
	ManifestClusterStatusNotConfigured          ManifestClusterStatus = "not_configured"
	ManifestClusterStatusNotRunning             ManifestClusterStatus = "not_running"
	ManifestClusterStatusAlreadyReady           ManifestClusterStatus = "already_ready"
	ManifestClusterStatusReconciled             ManifestClusterStatus = "reconciled"
	ManifestClusterStatusDefaultManifestUpdated ManifestClusterStatus = "default_updated"
	ManifestClusterStatusRequiresReconcile      ManifestClusterStatus = "requires_reconcile"
)

func (s ManifestClusterStatus) Validate() error {
	switch s {
	case ManifestClusterStatusNotApplicable, ManifestClusterStatusNotConfigured,
		ManifestClusterStatusNotRunning, ManifestClusterStatusAlreadyReady,
		ManifestClusterStatusReconciled, ManifestClusterStatusDefaultManifestUpdated,
		ManifestClusterStatusRequiresReconcile:
		return nil
	default:
		return fmt.Errorf("context cluster status is invalid: %q", s)
	}
}

// ManifestShellEnvironmentSource selects how one allowlisted shell variable is
// resolved for each new interactive Workspace session. Default is a public
// update value only; manifests persist only inherit and literal overrides.
type ManifestShellEnvironmentSource string

const (
	ManifestShellEnvironmentDefault ManifestShellEnvironmentSource = "default"
	ManifestShellEnvironmentInherit ManifestShellEnvironmentSource = "inherit"
	ManifestShellEnvironmentLiteral ManifestShellEnvironmentSource = "literal"
)

var contextShellEnvironmentVariables = []string{"COLORTERM", "NO_COLOR", "PS1", "TERM"}

func ManifestShellEnvironmentVariables() []string {
	return append([]string(nil), contextShellEnvironmentVariables...)
}

// InitialContextShellEnvironment makes exported PS1 inheritance the ordinary
// Context behavior while retaining the built-in prompt when the launcher did
// not export PS1.
func InitialContextShellEnvironment() []ManifestShellEnvironmentSetting {
	return []ManifestShellEnvironmentSetting{{Variable: "PS1", Source: ManifestShellEnvironmentInherit}}
}

func ValidateContextShellEnvironmentVariable(value string) error {
	for _, allowed := range contextShellEnvironmentVariables {
		if value == allowed {
			return nil
		}
	}
	return fmt.Errorf("Workspace Manifest shell environment variable %q is not allowlisted", value)
}

func ValidateContextShellEnvironmentValue(value string) error {
	if !utf8.ValidString(value) || len(value) > MaxContextShellValueBytes || strings.IndexByte(value, 0) >= 0 {
		return fmt.Errorf("shell environment value must be valid UTF-8 without NUL and at most %d bytes", MaxContextShellValueBytes)
	}
	return nil
}

// ManifestShellEnvironmentSetting is one persisted or reported variable
// policy. Value is present only for literal, including an explicit empty value.
type ManifestShellEnvironmentSetting struct {
	Variable string                         `json:"variable"`
	Source   ManifestShellEnvironmentSource `json:"source"`
	Value    *string                        `json:"value,omitempty"`
}

func (s ManifestShellEnvironmentSetting) Validate(allowDefault bool) error {
	if err := ValidateContextShellEnvironmentVariable(s.Variable); err != nil {
		return err
	}
	switch s.Source {
	case ManifestShellEnvironmentDefault:
		if !allowDefault {
			return fmt.Errorf("default shell environment source is not persisted")
		}
		if s.Value != nil {
			return fmt.Errorf("default shell environment source cannot contain a value")
		}
	case ManifestShellEnvironmentInherit:
		if s.Value != nil {
			return fmt.Errorf("inherited shell environment source cannot contain a value")
		}
	case ManifestShellEnvironmentLiteral:
		if s.Value == nil {
			return fmt.Errorf("literal shell environment source requires a value")
		}
		if err := ValidateContextShellEnvironmentValue(*s.Value); err != nil {
			return fmt.Errorf("literal %w", err)
		}
	default:
		return fmt.Errorf("Workspace Manifest shell environment source is invalid: %q", s.Source)
	}
	return nil
}

func validateContextShellEnvironment(settings []ManifestShellEnvironmentSetting, complete bool) error {
	seen := make(map[string]struct{}, len(settings))
	for _, setting := range settings {
		if err := setting.Validate(complete); err != nil {
			return err
		}
		if _, duplicate := seen[setting.Variable]; duplicate {
			return fmt.Errorf("Workspace Manifest shell environment variable %q is duplicated", setting.Variable)
		}
		seen[setting.Variable] = struct{}{}
	}
	if complete && len(seen) != len(contextShellEnvironmentVariables) {
		return fmt.Errorf("Workspace Manifest shell environment report must contain every allowlisted variable")
	}
	return nil
}

// CompleteContextShellEnvironment expands persisted overrides into the fixed
// public variable inventory so callers never infer a missing setting.
func CompleteContextShellEnvironment(overrides []ManifestShellEnvironmentSetting) ([]ManifestShellEnvironmentSetting, error) {
	if err := validateContextShellEnvironment(overrides, false); err != nil {
		return nil, err
	}
	byName := make(map[string]ManifestShellEnvironmentSetting, len(overrides))
	for _, setting := range overrides {
		byName[setting.Variable] = setting
	}
	complete := make([]ManifestShellEnvironmentSetting, 0, len(contextShellEnvironmentVariables))
	for _, variable := range contextShellEnvironmentVariables {
		setting, found := byName[variable]
		if !found {
			setting = ManifestShellEnvironmentSetting{Variable: variable, Source: ManifestShellEnvironmentDefault}
		}
		complete = append(complete, setting)
	}
	return complete, nil
}

func DefaultContextShellEnvironmentReport() []ManifestShellEnvironmentSetting {
	complete := make([]ManifestShellEnvironmentSetting, 0, len(contextShellEnvironmentVariables))
	for _, variable := range contextShellEnvironmentVariables {
		complete = append(complete, ManifestShellEnvironmentSetting{
			Variable: variable,
			Source:   ManifestShellEnvironmentDefault,
		})
	}
	return complete
}

// ApplyContextShellEnvironmentSetting returns a deterministic persisted
// override list. Selecting default removes that variable's override.
func ApplyContextShellEnvironmentSetting(
	overrides []ManifestShellEnvironmentSetting, change ManifestShellEnvironmentSetting,
) ([]ManifestShellEnvironmentSetting, error) {
	return ApplyContextShellEnvironmentSettings(overrides, []ManifestShellEnvironmentSetting{change})
}

// ApplyContextShellEnvironmentSettings validates a complete staged change set
// before returning one deterministic persisted override list. No partial
// result is returned when any change is invalid or targets a variable twice.
func ApplyContextShellEnvironmentSettings(
	overrides []ManifestShellEnvironmentSetting, changes []ManifestShellEnvironmentSetting,
) ([]ManifestShellEnvironmentSetting, error) {
	if err := validateContextShellEnvironment(overrides, false); err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, fmt.Errorf("Workspace Manifest shell environment change set is empty")
	}
	seenChanges := make(map[string]struct{}, len(changes))
	for _, change := range changes {
		if err := change.Validate(true); err != nil {
			return nil, err
		}
		if _, duplicate := seenChanges[change.Variable]; duplicate {
			return nil, fmt.Errorf("Workspace Manifest shell environment change for %q is duplicated", change.Variable)
		}
		seenChanges[change.Variable] = struct{}{}
	}
	byName := make(map[string]ManifestShellEnvironmentSetting, len(overrides)+len(changes))
	for _, setting := range overrides {
		byName[setting.Variable] = setting
	}
	for _, change := range changes {
		if change.Source == ManifestShellEnvironmentDefault {
			delete(byName, change.Variable)
		} else {
			byName[change.Variable] = change
		}
	}
	result := make([]ManifestShellEnvironmentSetting, 0, len(byName))
	for _, variable := range contextShellEnvironmentVariables {
		if setting, found := byName[variable]; found {
			result = append(result, setting)
		}
	}
	return result, nil
}

// ManifestGitIdentitySource selects the narrow Git identity projection owned by
// a Context. Default is represented explicitly in reports and omitted from
// manifests; inherit and literal are the only persisted overrides.
type ManifestGitIdentitySource string

const (
	ManifestGitIdentityDefault ManifestGitIdentitySource = "default"
	ManifestGitIdentityInherit ManifestGitIdentitySource = "inherit"
	ManifestGitIdentityLiteral ManifestGitIdentitySource = "literal"
)

// ManifestGitIdentitySetting is an atomic user.name and user.email policy.
// Name and Email are present together only for a literal setting.
type ManifestGitIdentitySetting struct {
	Source ManifestGitIdentitySource `json:"source"`
	Name   *string                   `json:"name"`
	Email  *string                   `json:"email"`
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

func (s ManifestGitIdentitySetting) Validate(allowDefault bool) error {
	switch s.Source {
	case ManifestGitIdentityDefault:
		if !allowDefault {
			return fmt.Errorf("default Git identity source is not persisted")
		}
		if s.Name != nil || s.Email != nil {
			return fmt.Errorf("default Git identity source cannot contain name or email")
		}
	case ManifestGitIdentityInherit:
		if s.Name != nil || s.Email != nil {
			return fmt.Errorf("inherited Git identity source cannot contain name or email")
		}
	case ManifestGitIdentityLiteral:
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
		return fmt.Errorf("Workspace Manifest Git identity source is invalid: %q", s.Source)
	}
	return nil
}

func DefaultContextGitIdentityReport() ManifestGitIdentitySetting {
	return ManifestGitIdentitySetting{Source: ManifestGitIdentityDefault}
}

// ManifestRuntimeBuild is the last successful build record for a recipe.
// Image is a Tobari-managed local reference; ImageDigest is the immutable
// Docker image identity used for diagnostics and drift detection.
type ManifestRuntimeBuild struct {
	Image        string `json:"image"`
	ImageDigest  string `json:"image_digest"`
	SourceDigest string `json:"source_digest"`
}

func (b ManifestRuntimeBuild) Validate() error {
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

// ManifestRuntimeRecipe describes the host-owned runtime/Dockerfile source.
// File is deliberately fixed so the routine workflow has no path selector.
type ManifestRuntimeRecipe struct {
	Kind          ManifestRuntimeKind   `json:"kind"`
	File          string                `json:"file"`
	BaseReference string                `json:"base_reference"`
	SourceDigest  string                `json:"source_digest,omitempty"`
	LastBuild     *ManifestRuntimeBuild `json:"last_build,omitempty"`
}

func (r ManifestRuntimeRecipe) Validate() error {
	if r.Kind != ManifestRuntimeKindDockerfile {
		return fmt.Errorf("runtime recipe kind must be %q", ManifestRuntimeKindDockerfile)
	}
	if r.File != ManifestRuntimeRecipeFile {
		return fmt.Errorf("runtime recipe file must be %q", ManifestRuntimeRecipeFile)
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

// ManifestRuntimeReport is the safe projection of one Context's exact Runtime
// binding. Legacy recipe fields remain internal-only while pre-public state is
// rejected or recreated.
type ManifestRuntimeReport struct {
	Kind          ManifestRuntimeKind   `json:"kind"`
	Status        ManifestRuntimeStatus `json:"status"`
	Image         string                `json:"image,omitempty"`
	Dockerfile    string                `json:"dockerfile,omitempty"`
	BaseReference string                `json:"base_reference,omitempty"`
	SourceDigest  string                `json:"source_digest,omitempty"`
	ImageDigest   string                `json:"image_digest,omitempty"`
	RuntimeID     string                `json:"runtime_id,omitempty"`
	Name          string                `json:"name,omitempty"`
	Revision      string                `json:"revision,omitempty"`
	Ordinal       int                   `json:"ordinal,omitempty"`
}

func (r ManifestRuntimeReport) Validate() error {
	if err := r.Kind.Validate(); err != nil {
		return err
	}
	if err := r.Status.Validate(); err != nil {
		return err
	}
	if r.Kind == ManifestRuntimeKindOfficial && r.Status != ManifestRuntimeStatusOfficial {
		return fmt.Errorf("official runtime must have official status")
	}
	if r.Kind == ManifestRuntimeKindDockerfile && r.Status == ManifestRuntimeStatusOfficial {
		return fmt.Errorf("Dockerfile runtime cannot have official status")
	}
	if r.RuntimeID != "" {
		if r.Kind != ManifestRuntimeKindManaged && r.Kind != ManifestRuntimeKindOfficial {
			return fmt.Errorf("revision-bound Runtime kind is invalid")
		}
		if r.Status != ManifestRuntimeStatusReady {
			if r.RuntimeID != StandardRuntimeID || r.Status != ManifestRuntimeStatusOfficial {
				return fmt.Errorf("Runtime reference must be ready or built-in standard")
			}
		}
		binding := RuntimeBinding{RuntimeID: r.RuntimeID, Name: r.Name, Revision: r.Revision, Ordinal: r.Ordinal, Image: r.Image}
		if err := binding.Validate(); err != nil {
			return err
		}
	} else if r.Kind == ManifestRuntimeKindManaged {
		return fmt.Errorf("managed Runtime reference requires stable identity")
	}
	if r.Dockerfile != "" && (!filepath.IsAbs(r.Dockerfile) || filepath.Clean(r.Dockerfile) != r.Dockerfile) {
		return fmt.Errorf("runtime Dockerfile must be a canonical absolute path")
	}
	if r.BaseReference != "" {
		if err := ValidateImageSelector(r.BaseReference); err != nil {
			return err
		}
	}
	if r.Image != "" {
		if err := ValidateImageSelector(r.Image); err != nil {
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

// WorkspaceManifest is the trusted, secret-free logical composition record.
// Paths are deliberately resolved by infrastructure rather than persisted in
// the manifest so stores remain independently protected.
type WorkspaceManifest struct {
	SchemaVersion    int                               `json:"schema_version"`
	ID               string                            `json:"workspace_manifest_id"`
	Name             string                            `json:"name"`
	Desired          WorkspaceManifestRevision         `json:"desired"`
	AgentProfile     string                            `json:"agent_profile"`
	Image            string                            `json:"image"`
	PolicyMode       ManifestPolicyMode                `json:"policy_mode"`
	SourceAccess     ManifestSourceAccess              `json:"source_access"`
	PolicyRevision   string                            `json:"policy_revision"`
	NativeReadiness  ManifestNativeReadiness           `json:"native_readiness,omitempty"`
	Runtime          *ManifestRuntimeRecipe            `json:"runtime,omitempty"`
	RuntimeBinding   *RuntimeBinding                   `json:"runtime_binding,omitempty"`
	ShellEnvironment []ManifestShellEnvironmentSetting `json:"shell_environment,omitempty"`
	GitIdentity      *ManifestGitIdentitySetting       `json:"git_identity,omitempty"`
	Bootstrap        *ManifestBootstrapSnapshot        `json:"bootstrap,omitempty"`
}

type workspaceManifestSemanticBody struct {
	AgentProfile     string
	Image            string
	PolicyMode       ManifestPolicyMode
	SourceAccess     ManifestSourceAccess
	PolicyRevision   string
	NativeReadiness  ManifestNativeReadiness
	Runtime          *ManifestRuntimeRecipe
	RuntimeBinding   *RuntimeBinding
	ShellEnvironment []ManifestShellEnvironmentSetting
	GitIdentity      *ManifestGitIdentitySetting
	Bootstrap        *ManifestBootstrapSnapshot
}

func semanticDigest(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest[:]), nil
}

// PublishWorkspaceManifest validates a complete draft, computes its canonical
// semantic and activation identities, and returns a new immutable desired
// revision. A semantic no-op retains the prior revision and generation.
func PublishWorkspaceManifest(draft WorkspaceManifest, previous *WorkspaceManifest) (WorkspaceManifest, error) {
	draft.Desired = WorkspaceManifestRevision{}
	if err := draft.Validate(); err != nil {
		return WorkspaceManifest{}, err
	}
	readiness, err := ResolveContextNativeReadiness(draft.NativeReadiness)
	if err != nil {
		return WorkspaceManifest{}, err
	}
	draft.NativeReadiness = readiness
	boundary, err := semanticDigest(struct {
		AgentProfile    string
		PolicyMode      ManifestPolicyMode
		SourceAccess    ManifestSourceAccess
		NativeReadiness ManifestNativeReadiness
	}{draft.AgentProfile, draft.PolicyMode, draft.SourceAccess, draft.NativeReadiness})
	if err != nil {
		return WorkspaceManifest{}, err
	}
	cluster, err := semanticDigest(struct{ PolicyRevision string }{draft.PolicyRevision})
	if err != nil {
		return WorkspaceManifest{}, err
	}
	entry, err := semanticDigest(struct {
		Image          string
		Runtime        *ManifestRuntimeRecipe
		RuntimeBinding *RuntimeBinding
	}{draft.Image, draft.Runtime, draft.RuntimeBinding})
	if err != nil {
		return WorkspaceManifest{}, err
	}
	sessionSettings := make([]struct {
		Variable     string
		Source       ManifestShellEnvironmentSource
		ValuePresent bool
		Value        string
	}, 0, len(draft.ShellEnvironment))
	for _, setting := range draft.ShellEnvironment {
		item := struct {
			Variable     string
			Source       ManifestShellEnvironmentSource
			ValuePresent bool
			Value        string
		}{Variable: setting.Variable, Source: setting.Source, ValuePresent: setting.Value != nil}
		if setting.Value != nil {
			item.Value = *setting.Value
		}
		sessionSettings = append(sessionSettings, item)
	}
	sort.Slice(sessionSettings, func(i, j int) bool { return sessionSettings[i].Variable < sessionSettings[j].Variable })
	session, err := semanticDigest(struct {
		ShellEnvironment any
		GitIdentity      *ManifestGitIdentitySetting
	}{sessionSettings, draft.GitIdentity})
	if err != nil {
		return WorkspaceManifest{}, err
	}
	creation, err := semanticDigest(draft.Bootstrap)
	if err != nil {
		return WorkspaceManifest{}, err
	}
	revision, err := semanticDigest(struct {
		Boundary          string
		ClusterProjection string
		Entry             string
		SessionDefaults   string
		CreationDefaults  string
	}{boundary, cluster, entry, session, creation})
	if err != nil {
		return WorkspaceManifest{}, err
	}
	generation := uint64(1)
	if previous != nil {
		if previous.ID != draft.ID || previous.Name != draft.Name {
			return WorkspaceManifest{}, fmt.Errorf("Manifest publication identity changed")
		}
		if err := previous.ValidatePublished(); err != nil {
			return WorkspaceManifest{}, err
		}
		if previous.Desired.BoundaryRevision != boundary {
			return WorkspaceManifest{}, fmt.Errorf("Manifest Boundary is immutable")
		}
		if previous.Desired.Revision == revision {
			draft.Desired = previous.Desired
			return draft, nil
		}
		generation = previous.Desired.Generation + 1
	}
	draft.Desired = WorkspaceManifestRevision{
		Generation: generation, Revision: revision, BoundaryRevision: boundary,
		ClusterProjectionRevision: cluster, EntryRevision: entry,
		SessionDefaultsRevision: session, CreationDefaultsRevision: creation,
	}
	if err := draft.Desired.Validate(); err != nil {
		return WorkspaceManifest{}, err
	}
	return draft, nil
}

// ValidatePublished requires the complete canonical revision metadata used by
// persisted authority and exact copy/reconciliation decisions.
func (m WorkspaceManifest) ValidatePublished() error {
	if err := m.Validate(); err != nil {
		return err
	}
	if err := m.Desired.Validate(); err != nil {
		return err
	}
	copy, err := PublishWorkspaceManifestWithoutPrevious(m)
	if err != nil {
		return err
	}
	complete, err := semanticDigest(struct {
		Boundary          string
		ClusterProjection string
		Entry             string
		SessionDefaults   string
		CreationDefaults  string
	}{m.Desired.BoundaryRevision, m.Desired.ClusterProjectionRevision, m.Desired.EntryRevision, m.Desired.SessionDefaultsRevision, m.Desired.CreationDefaultsRevision})
	if err != nil {
		return err
	}
	for name, pair := range map[string][2]string{
		"manifest":           {complete, m.Desired.Revision},
		"boundary":           {copy.Desired.BoundaryRevision, m.Desired.BoundaryRevision},
		"cluster projection": {copy.Desired.ClusterProjectionRevision, m.Desired.ClusterProjectionRevision},
		"entry":              {copy.Desired.EntryRevision, m.Desired.EntryRevision},
		"session defaults":   {copy.Desired.SessionDefaultsRevision, m.Desired.SessionDefaultsRevision},
		"creation defaults":  {copy.Desired.CreationDefaultsRevision, m.Desired.CreationDefaultsRevision},
	} {
		if pair[0] != pair[1] {
			return fmt.Errorf("Manifest %s revision does not match its canonical body: computed %s stored %s", name, pair[0], pair[1])
		}
	}
	return nil
}

func PublishWorkspaceManifestWithoutPrevious(draft WorkspaceManifest) (WorkspaceManifest, error) {
	desired := draft.Desired
	draft.Desired = WorkspaceManifestRevision{}
	published, err := publishWorkspaceManifestUnchecked(draft)
	if err != nil {
		return WorkspaceManifest{}, err
	}
	published.Desired.Generation = desired.Generation
	return published, nil
}

func publishWorkspaceManifestUnchecked(draft WorkspaceManifest) (WorkspaceManifest, error) {
	// Avoid recursion through ValidatePublished; PublishWorkspaceManifest only
	// calls the base Validate method before assigning revision metadata.
	return PublishWorkspaceManifest(draft, nil)
}

// RuntimeSelection returns the exact user-facing Runtime binding while
// preserving a truthful label for the pre-binding Dockerfile compatibility
// state.
func (m WorkspaceManifest) RuntimeSelection() (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	if m.RuntimeBinding != nil {
		return m.RuntimeBinding.Selection()
	}
	if m.Runtime != nil {
		return "context-owned Dockerfile", nil
	}
	return StandardRuntimeName + "@1", nil
}

func (m WorkspaceManifest) Validate() error {
	if m.SchemaVersion != WorkspaceManifestSchemaVersion {
		return fmt.Errorf("context schema version must be %d", WorkspaceManifestSchemaVersion)
	}
	if err := ValidateWorkspaceManifestID(m.ID); err != nil {
		return err
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
	if err := m.SourceAccess.Validate(); err != nil {
		return err
	}
	if !digestPattern.MatchString(m.PolicyRevision) {
		return fmt.Errorf("context policy revision is invalid")
	}
	if _, err := ResolveContextNativeReadiness(m.NativeReadiness); err != nil {
		return err
	}
	if m.Runtime != nil {
		if err := m.Runtime.Validate(); err != nil {
			return err
		}
	}
	if m.RuntimeBinding != nil {
		if m.Runtime != nil {
			return fmt.Errorf("Workspace Manifest cannot own both a Runtime binding and legacy recipe")
		}
		if err := m.RuntimeBinding.Validate(); err != nil {
			return err
		}
		if m.Image != m.RuntimeBinding.Image {
			return fmt.Errorf("Workspace Manifest image does not match its Runtime binding")
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
	if m.Bootstrap != nil {
		if err := m.Bootstrap.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// ManifestStorePaths are trusted infrastructure-resolved paths. They are
// included in host diagnostics, never in Workspace mounts as a whole.
type ManifestStorePaths struct {
	PolicyDirectory   string `json:"policy_directory"`
	RuntimeDirectory  string `json:"runtime_directory,omitempty"`
	RuntimeDockerfile string `json:"runtime_dockerfile,omitempty"`
}

func (p ManifestStorePaths) Validate() error {
	for name, value := range map[string]string{"policy directory": p.PolicyDirectory} {
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

// ManifestSummary is one item in the complete local Context collection.
type ManifestSummary struct {
	ID              string                    `json:"workspace_manifest_id"`
	Name            string                    `json:"name"`
	ManifestState   ManifestObservationState  `json:"workspace_manifest_state"`
	Default         bool                      `json:"default"`
	Desired         WorkspaceManifestRevision `json:"desired"`
	AgentProfile    string                    `json:"agent_profile"`
	Image           string                    `json:"image"`
	PolicyMode      ManifestPolicyMode        `json:"policy_mode"`
	SourceAccess    ManifestSourceAccess      `json:"source_access"`
	PolicyRevision  string                    `json:"policy_revision"`
	NativeReadiness ManifestNativeReadiness   `json:"native_readiness"`
	MethodPolicy    ManifestMethodPolicy      `json:"method_policy"`
	// RoutineAccess is the domain-evaluated effective summary from the actual
	// Context policy read. It does not extend schema-1 JSON.
	RoutineAccess *ManifestAccessSummary `json:"-"`
	RuntimeStatus ManifestRuntimeStatus  `json:"runtime_status,omitempty"`
	// RuntimeSelection is a human selection token derived from the exact
	// binding during the same read. It is intentionally outside schema-1 JSON,
	// whose established item projection remains unchanged.
	RuntimeSelection string                  `json:"-"`
	Bootstrap        ManifestBootstrapReport `json:"bootstrap"`
}

func (s ManifestSummary) Validate() error {
	if err := s.ManifestState.Validate(); err != nil {
		return err
	}
	if s.ManifestState != ManifestObservationPersisted {
		return fmt.Errorf("configured Workspace Manifest item must be persisted")
	}
	manifest := WorkspaceManifest{
		SchemaVersion:   WorkspaceManifestSchemaVersion,
		ID:              s.ID,
		Name:            s.Name,
		AgentProfile:    s.AgentProfile,
		Image:           s.Image,
		PolicyMode:      s.PolicyMode,
		SourceAccess:    s.SourceAccess,
		PolicyRevision:  s.PolicyRevision,
		NativeReadiness: s.NativeReadiness,
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := s.Desired.Validate(); err != nil {
		return err
	}
	if s.RuntimeStatus != "" {
		if err := s.RuntimeStatus.Validate(); err != nil {
			return err
		}
	}
	if s.RuntimeSelection == "" {
		return fmt.Errorf("Workspace Manifest summary requires an exact Runtime selection")
	}
	if s.RuntimeStatus == ManifestRuntimeStatusOfficial || s.RuntimeStatus == ManifestRuntimeStatusReady {
		if err := validateRuntimeDisplaySelection(s.RuntimeSelection); err != nil {
			return err
		}
	}
	if err := s.MethodPolicy.Validate(); err != nil {
		return err
	}
	if err := validateRoutineAccessProjection(s.RoutineAccess, s.SourceAccess, s.MethodPolicy); err != nil {
		return err
	}
	if err := s.Bootstrap.Validate(); err != nil {
		return err
	}
	return nil
}

// ManifestListResult is a complete local Context observation. Empty is
// represented by an explicit non-nil Items collection, although a valid
// installation always contains the default Context.
type ManifestListResult struct {
	Task              string                   `json:"task"`
	ManifestState     ManifestObservationState `json:"workspace_manifest_state"`
	DefaultManifestID string                   `json:"default_manifest_id,omitempty"`
	DefaultManifest   string                   `json:"default_manifest,omitempty"`
	Items             []ManifestSummary        `json:"items"`
}

// ManifestDeleteResult confirms removal without pretending the deleted Context
// can still produce a ManifestReport.
type ManifestDeleteResult struct {
	Task    string                `json:"task"`
	ID      string                `json:"id"`
	Name    string                `json:"name"`
	Deleted bool                  `json:"deleted"`
	Cluster ManifestClusterStatus `json:"cluster"`
}

func (r ManifestDeleteResult) Validate() error {
	if r.Task != TaskManifestDelete || !r.Deleted {
		return fmt.Errorf("Workspace Manifest deletion outcome is invalid")
	}
	if err := ValidateWorkspaceManifestID(r.ID); err != nil {
		return err
	}
	if err := ValidateName(r.Name); err != nil {
		return err
	}
	if r.Cluster != ManifestClusterStatusNotApplicable && r.Cluster != ManifestClusterStatusRequiresReconcile {
		return fmt.Errorf("Workspace Manifest deletion cluster outcome is invalid")
	}
	return nil
}

func (r ManifestListResult) Validate() error {
	if r.Task != TaskManifestList || r.Items == nil {
		return fmt.Errorf("manifest list task or scope is invalid")
	}
	if err := r.ManifestState.Validate(); err != nil {
		return err
	}
	if (r.DefaultManifestID == "") != (r.DefaultManifest == "") {
		return fmt.Errorf("default Manifest identity and name must be present together")
	}
	if r.DefaultManifest != "" {
		if err := ValidateWorkspaceManifestID(r.DefaultManifestID); err != nil {
			return fmt.Errorf("default Manifest ID: %w", err)
		}
		if err := ValidateName(r.DefaultManifest); err != nil {
			return fmt.Errorf("default Manifest name: %w", err)
		}
	}
	seen := make(map[string]struct{}, len(r.Items))
	defaultCount := 0
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, exists := seen[item.Name]; exists {
			return fmt.Errorf("context names must be unique")
		}
		seen[item.Name] = struct{}{}
		if item.Default {
			defaultCount++
			if item.Name != r.DefaultManifest || item.ID != r.DefaultManifestID {
				return fmt.Errorf("default Manifest does not match the default item")
			}
			if item.ManifestState != r.ManifestState {
				return fmt.Errorf("default Manifest state does not match the list observation state")
			}
		}
	}
	if r.ManifestState == ManifestObservationAbsent {
		if r.DefaultManifest != "" || defaultCount != 0 {
			return fmt.Errorf("absent default Manifest selection cannot claim authority")
		}
		return nil
	}
	if _, exists := seen[r.DefaultManifest]; !exists || defaultCount != 1 {
		return fmt.Errorf("Manifest list must contain exactly one default Manifest")
	}
	return nil
}

const (
	ManifestAuthBrokerNotApplicable = "not_applicable"
	ManifestAuthBrokerReady         = "ready"
	ManifestAuthBrokerLocked        = "locked"
	ManifestAuthBrokerUnavailable   = "unavailable"

	ManifestAuthProviderConfigured    = "configured"
	ManifestAuthProviderNotConfigured = "not_configured"
	ManifestAuthProviderUnavailable   = "unavailable"
)

// ManifestAuthProvider reports one provider's non-secret Context-owned state.
// It never contains a project handle or an upstream credential.
type ManifestAuthProvider struct {
	Provider           string  `json:"provider"`
	State              string  `json:"state"`
	AccountLabel       *string `json:"account_label"`
	CredentialRevision string  `json:"credential_revision"`
}

func (p ManifestAuthProvider) Validate() error {
	if len(p.Provider) > 64 || !regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`).MatchString(p.Provider) {
		return fmt.Errorf("Workspace Manifest authentication provider ID is invalid")
	}
	switch p.State {
	case ManifestAuthProviderConfigured:
		if !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`).MatchString(p.CredentialRevision) {
			return fmt.Errorf("configured Workspace Manifest authentication provider revision is invalid")
		}
	case ManifestAuthProviderNotConfigured, ManifestAuthProviderUnavailable:
		if p.AccountLabel != nil || p.CredentialRevision != "" {
			return fmt.Errorf("unconfigured Workspace Manifest authentication provider contains configured metadata")
		}
	default:
		return fmt.Errorf("Workspace Manifest authentication provider state is invalid")
	}
	if p.AccountLabel != nil {
		value := *p.AccountLabel
		if value == "" || len(value) > 128 || !utf8.ValidString(value) || strings.TrimSpace(value) != value ||
			strings.IndexFunc(value, func(character rune) bool {
				return character < ' ' || character == '\u007f' || character == '\u2028' || character == '\u2029'
			}) >= 0 {
			return fmt.Errorf("Workspace Manifest authentication account label is invalid")
		}
	}
	return nil
}

// ManifestAuthentication is an explicit observation. not_applicable is used
// only by mutation results whose task did not inspect the running broker.
type ManifestAuthentication struct {
	Mode        string                 `json:"mode,omitempty"`
	BrokerState string                 `json:"broker_state,omitempty"`
	Providers   []ManifestAuthProvider `json:"providers"`
}

const (
	ManifestAuthenticationModeNotApplicable = "not_applicable"
	ManifestAuthenticationModeNative        = "native_workspace"
	ManifestAuthenticationModeBroker        = "broker"
)

func (a ManifestAuthentication) Validate(observed bool) error {
	mode := a.Mode
	if mode == "" {
		if a.BrokerState == ManifestAuthBrokerNotApplicable {
			mode = ManifestAuthenticationModeNotApplicable
		} else if a.BrokerState != "" {
			mode = ManifestAuthenticationModeBroker
		}
	}
	if !observed {
		if mode != ManifestAuthenticationModeNotApplicable || a.BrokerState != ManifestAuthBrokerNotApplicable || a.Providers != nil {
			return fmt.Errorf("unobserved Workspace Manifest authentication state is invalid")
		}
		return nil
	}
	if mode == ManifestAuthenticationModeNative {
		if a.BrokerState != "" || a.Providers == nil || len(a.Providers) != 0 {
			return fmt.Errorf("native Workspace authentication state is invalid")
		}
		return nil
	}
	if mode != ManifestAuthenticationModeBroker {
		return fmt.Errorf("observed Workspace Manifest authentication mode is invalid")
	}
	switch a.BrokerState {
	case ManifestAuthBrokerReady, ManifestAuthBrokerLocked, ManifestAuthBrokerUnavailable:
	default:
		return fmt.Errorf("observed Workspace Manifest authentication broker state is invalid")
	}
	if a.Providers == nil {
		return fmt.Errorf("observed Workspace Manifest authentication provider collection is absent")
	}
	seen := make(map[string]struct{}, len(a.Providers))
	for _, provider := range a.Providers {
		if err := provider.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[provider.Provider]; duplicate {
			return fmt.Errorf("Workspace Manifest authentication provider is duplicated")
		}
		seen[provider.Provider] = struct{}{}
		if a.BrokerState != ManifestAuthBrokerReady && provider.State != ManifestAuthProviderUnavailable {
			return fmt.Errorf("unready Workspace Manifest authentication broker has an available provider state")
		}
	}
	return nil
}

// ManifestReport is the complete selected Context view.
type ManifestReport struct {
	Task            string                    `json:"task"`
	ManifestState   ManifestObservationState  `json:"workspace_manifest_state"`
	ID              string                    `json:"workspace_manifest_id"`
	Name            string                    `json:"name"`
	Default         bool                      `json:"default"`
	Desired         WorkspaceManifestRevision `json:"desired"`
	AgentProfile    string                    `json:"agent_profile"`
	Image           string                    `json:"image"`
	PolicyMode      ManifestPolicyMode        `json:"policy_mode"`
	SourceAccess    ManifestSourceAccess      `json:"source_access"`
	PolicyRevision  string                    `json:"policy_revision"`
	NativeReadiness ManifestNativeReadiness   `json:"native_readiness"`
	MethodPolicy    ManifestMethodPolicy      `json:"method_policy"`
	// RoutineAccess is computed from the actual policy read and retained only
	// for trusted-host presentation.
	RoutineAccess    *ManifestAccessSummary            `json:"-"`
	ShellEnvironment []ManifestShellEnvironmentSetting `json:"shell_environment"`
	GitIdentity      ManifestGitIdentitySetting        `json:"git_identity"`
	Stores           ManifestStorePaths                `json:"stores"`
	Runtime          ManifestRuntimeReport             `json:"runtime"`
	Cluster          ManifestClusterStatus             `json:"cluster"`
	Authentication   ManifestAuthentication            `json:"authentication"`
	Bootstrap        ManifestBootstrapReport           `json:"bootstrap"`
}

func (r ManifestReport) Validate() error {
	if r.Task != TaskManifestShow && r.Task != TaskManifestCreate && r.Task != TaskManifestDefaultSet &&
		r.Task != TaskConfigShell && r.Task != TaskConfigGit && r.Task != TaskConfigBootstrapAWS && r.Task != TaskConfigBootstrapEKS && r.Task != TaskManifestRuntimeSet && r.Task != TaskRuntimeInit && r.Task != TaskRuntimeBuild {
		return fmt.Errorf("context report task is invalid")
	}
	if err := r.ManifestState.Validate(); err != nil {
		return err
	}
	if r.ManifestState == ManifestObservationAbsent {
		if r.ID != "" || r.Desired != (WorkspaceManifestRevision{}) || r.Stores != (ManifestStorePaths{}) || r.Default {
			return fmt.Errorf("absent Manifest report claims persisted authority")
		}
		if err := ValidateName(r.Name); err != nil {
			return err
		}
		if r.AgentProfile != DefaultProfile || ValidateImageSelector(r.Image) != nil || r.PolicyMode != ManifestPolicyModeGuided || r.SourceAccess != ManifestSourceAccessReadWrite {
			return fmt.Errorf("absent Manifest report defaults are invalid")
		}
	} else {
		if _, err := ResolveContextNativeReadiness(r.NativeReadiness); err != nil {
			return err
		}
		manifest := WorkspaceManifest{
			SchemaVersion:  WorkspaceManifestSchemaVersion,
			ID:             r.ID,
			Name:           r.Name,
			AgentProfile:   r.AgentProfile,
			Image:          r.Image,
			PolicyMode:     r.PolicyMode,
			SourceAccess:   r.SourceAccess,
			PolicyRevision: r.PolicyRevision,
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := r.Desired.Validate(); err != nil {
			return err
		}
		if err := r.Stores.Validate(); err != nil {
			return err
		}
	}
	if err := r.MethodPolicy.Validate(); err != nil {
		return err
	}
	if err := validateRoutineAccessProjection(r.RoutineAccess, r.SourceAccess, r.MethodPolicy); err != nil {
		return err
	}
	if err := r.Runtime.Validate(); err != nil {
		return err
	}
	if r.Runtime.RuntimeID != "" && r.Runtime.Image != r.Image {
		return fmt.Errorf("Workspace Manifest report image does not match its Runtime binding")
	}
	if err := validateContextShellEnvironment(r.ShellEnvironment, true); err != nil {
		return err
	}
	if err := r.GitIdentity.Validate(true); err != nil {
		return err
	}
	if err := r.Bootstrap.Validate(); err != nil {
		return err
	}
	if err := r.Authentication.Validate(r.Task == TaskManifestShow); err != nil {
		return err
	}
	return r.Cluster.Validate()
}
