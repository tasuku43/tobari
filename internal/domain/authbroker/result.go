package authbroker

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	TaskLogin  = "auth.login"
	TaskImport = "auth.import"
	TaskStatus = "auth.status"
	TaskLogout = "auth.logout"

	// ContextAuthActivationGuidance is the stable public explanation of
	// credential ownership, project-bound handle isolation, and the activation
	// boundary for already-running sessions.
	ContextAuthActivationGuidance = "Credential ownership is Context-wide; each permanently bound project receives a distinct handle; existing sessions must leave and re-enter to receive the current authentication revision."
	ContextAuthRemovalGuidance    = "Credential removal is Context-wide; handles for every permanently bound project are invalidated; existing sessions must leave and re-enter to remove the stale authentication projection."

	CredentialCatalogTargetKind = "auth-credentials"
	CredentialCatalogTargetID   = "active-context-auth-credentials"
)

var (
	contextNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	contextIDPattern   = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	revisionPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

type StorageBackend string

const (
	StorageBackendMacOSKeychain StorageBackend = "macos_keychain"
	StorageBackendXDGFile       StorageBackend = "xdg_file"
)

func (b StorageBackend) Validate() error {
	switch b {
	case StorageBackendMacOSKeychain, StorageBackendXDGFile:
		return nil
	default:
		return fmt.Errorf("auth storage backend is invalid: %q", b)
	}
}

type BrokerState string

const (
	BrokerStateLocked      BrokerState = "locked"
	BrokerStateReady       BrokerState = "ready"
	BrokerStateUnavailable BrokerState = "unavailable"
)

func (s BrokerState) Validate() error {
	switch s {
	case BrokerStateLocked, BrokerStateReady, BrokerStateUnavailable:
		return nil
	default:
		return fmt.Errorf("auth broker state is invalid: %q", s)
	}
}

type WorkspaceActivationState string

const (
	WorkspaceActivationNotApplicable   WorkspaceActivationState = "not_applicable"
	WorkspaceActivationReady           WorkspaceActivationState = "ready"
	WorkspaceActivationReentryRequired WorkspaceActivationState = "workspace_reentry_required"
)

func (s WorkspaceActivationState) Validate() error {
	switch s {
	case WorkspaceActivationNotApplicable, WorkspaceActivationReady, WorkspaceActivationReentryRequired:
		return nil
	default:
		return fmt.Errorf("Workspace activation state is invalid: %q", s)
	}
}

// WorkspaceActivation gives bounded, secret-free guidance about when a changed
// credential projection can affect Workspace processes. It never claims that
// an already-running process environment was mutated.
type WorkspaceActivation struct {
	State    WorkspaceActivationState `json:"state"`
	Guidance string                   `json:"guidance"`
}

func (a WorkspaceActivation) Validate() error {
	if err := a.State.Validate(); err != nil {
		return err
	}
	if a.State == WorkspaceActivationReentryRequired {
		if a.Guidance != ContextAuthActivationGuidance &&
			a.Guidance != ContextAuthRemovalGuidance {
			return fmt.Errorf("Workspace activation guidance does not declare the Context credential activation contract")
		}
		return nil
	}
	if a.Guidance != "" {
		return fmt.Errorf("Workspace activation guidance must be empty for state %q", a.State)
	}
	return nil
}

// Result is the secret-free result shared by login, stdin import, status, and
// logout. A nil AccountLabel means the provider did not make an account label
// available; it never stands for the primary secret.
type Result struct {
	Task                string              `json:"task"`
	Provider            string              `json:"provider"`
	Context             string              `json:"context"`
	ContextID           string              `json:"context_id"`
	Configured          bool                `json:"configured"`
	AccountLabel        *string             `json:"account_label"`
	StorageBackend      StorageBackend      `json:"storage_backend"`
	BrokerState         BrokerState         `json:"broker_state"`
	CredentialRevision  string              `json:"credential_revision"`
	WorkspaceActivation WorkspaceActivation `json:"workspace_activation"`
}

func (r Result) Validate() error {
	switch r.Task {
	case TaskLogin, TaskImport, TaskLogout:
	default:
		return fmt.Errorf("auth result task is invalid: %q", r.Task)
	}
	if !contextNamePattern.MatchString(r.Context) {
		return fmt.Errorf("auth result Context name is invalid")
	}
	if !contextIDPattern.MatchString(r.ContextID) {
		return fmt.Errorf("auth result Context ID is invalid")
	}
	if err := ValidateProviderID(r.Provider); err != nil {
		return err
	}
	if r.Task == TaskLogin || r.Task == TaskImport {
		if !r.Configured {
			return fmt.Errorf("successful auth login/import result must be configured")
		}
	}
	if r.Task == TaskLogout && r.Configured {
		return fmt.Errorf("successful auth logout result cannot remain configured")
	}
	if r.Configured {
		if !revisionPattern.MatchString(r.CredentialRevision) {
			return fmt.Errorf("configured auth result credential revision is invalid")
		}
	} else {
		if r.CredentialRevision != "" {
			return fmt.Errorf("unconfigured auth result cannot declare a credential revision")
		}
		if r.AccountLabel != nil {
			return fmt.Errorf("unconfigured auth result cannot declare an account label")
		}
	}
	if r.AccountLabel != nil {
		if err := validateDisplayText("auth account label", *r.AccountLabel, 128); err != nil {
			return err
		}
	}
	if err := r.StorageBackend.Validate(); err != nil {
		return err
	}
	if err := r.BrokerState.Validate(); err != nil {
		return err
	}
	if (r.Task == TaskLogin || r.Task == TaskImport || r.Task == TaskLogout) && r.BrokerState != BrokerStateReady {
		return fmt.Errorf("successful auth mutation result requires a ready broker")
	}
	if err := r.WorkspaceActivation.Validate(); err != nil {
		return err
	}
	if r.WorkspaceActivation.State != WorkspaceActivationReentryRequired {
		return fmt.Errorf("successful auth mutation result must require Workspace re-entry")
	}
	expectedGuidance := ContextAuthActivationGuidance
	if !r.Configured {
		expectedGuidance = ContextAuthRemovalGuidance
	}
	if r.WorkspaceActivation.Guidance != expectedGuidance {
		return fmt.Errorf("auth mutation Workspace guidance does not match its configured state")
	}
	return nil
}

type ProviderCredentialState string

const (
	ProviderCredentialConfigured    ProviderCredentialState = "configured"
	ProviderCredentialNotConfigured ProviderCredentialState = "not_configured"
	ProviderCredentialUnavailable   ProviderCredentialState = "unavailable"
)

func (s ProviderCredentialState) Validate() error {
	switch s {
	case ProviderCredentialConfigured, ProviderCredentialNotConfigured, ProviderCredentialUnavailable:
		return nil
	default:
		return fmt.Errorf("provider credential state is invalid: %q", s)
	}
}

// ProviderStatus is one provider entry in the exhaustive Context-scoped auth
// status result. Configuration metadata is secret-free and cannot carry a
// primary secret or project-bound handle. Configured is retained as an explicit
// compatibility projection and is meaningful only with the accompanying State.
type ProviderStatus struct {
	Provider           string                  `json:"provider"`
	State              ProviderCredentialState `json:"state"`
	Configured         bool                    `json:"configured"`
	AccountLabel       *string                 `json:"account_label"`
	CredentialRevision string                  `json:"credential_revision"`
}

func (s ProviderStatus) Validate() error {
	if err := ValidateProviderID(s.Provider); err != nil {
		return err
	}
	if err := s.State.Validate(); err != nil {
		return err
	}
	if s.State == ProviderCredentialConfigured {
		if !s.Configured {
			return fmt.Errorf("configured provider state requires configured=true")
		}
		if !revisionPattern.MatchString(s.CredentialRevision) {
			return fmt.Errorf("configured provider status credential revision is invalid")
		}
	} else {
		if s.Configured {
			return fmt.Errorf("provider state %q requires configured=false", s.State)
		}
		if s.CredentialRevision != "" {
			return fmt.Errorf("provider state %q cannot declare a credential revision", s.State)
		}
		if s.AccountLabel != nil {
			return fmt.Errorf("provider state %q cannot declare an account label", s.State)
		}
	}
	if s.AccountLabel != nil {
		if err := validateDisplayText("auth account label", *s.AccountLabel, 128); err != nil {
			return err
		}
	}
	return nil
}

// StatusResult is the complete provider-status collection for one stable
// Context. Providers is non-nil even when the available provider set is empty.
type StatusResult struct {
	Task                string              `json:"task"`
	Context             string              `json:"context"`
	ContextID           string              `json:"context_id"`
	StorageBackend      StorageBackend      `json:"storage_backend"`
	BrokerState         BrokerState         `json:"broker_state"`
	Providers           []ProviderStatus    `json:"providers"`
	WorkspaceActivation WorkspaceActivation `json:"workspace_activation"`
}

func (r StatusResult) Validate() error {
	if r.Task != TaskStatus {
		return fmt.Errorf("auth status result task must be %q", TaskStatus)
	}
	if !contextNamePattern.MatchString(r.Context) {
		return fmt.Errorf("auth status Context name is invalid")
	}
	if !contextIDPattern.MatchString(r.ContextID) {
		return fmt.Errorf("auth status Context ID is invalid")
	}
	if err := r.StorageBackend.Validate(); err != nil {
		return err
	}
	if err := r.BrokerState.Validate(); err != nil {
		return err
	}
	if r.Providers == nil {
		return fmt.Errorf("auth status providers collection is absent")
	}
	seen := make(map[string]struct{}, len(r.Providers))
	configured := false
	for index, provider := range r.Providers {
		if err := provider.Validate(); err != nil {
			return fmt.Errorf("auth status provider %d: %w", index, err)
		}
		if _, exists := seen[provider.Provider]; exists {
			return fmt.Errorf("auth status provider %q is duplicated", provider.Provider)
		}
		seen[provider.Provider] = struct{}{}
		if r.BrokerState == BrokerStateLocked && provider.State != ProviderCredentialUnavailable {
			return fmt.Errorf("locked auth broker requires provider %q state to be unavailable", provider.Provider)
		}
		configured = configured || provider.Configured
	}
	if err := r.WorkspaceActivation.Validate(); err != nil {
		return err
	}
	if configured && r.WorkspaceActivation.State != WorkspaceActivationReentryRequired {
		return fmt.Errorf("configured auth status must declare Workspace re-entry guidance")
	}
	if configured && r.WorkspaceActivation.Guidance != ContextAuthActivationGuidance {
		return fmt.Errorf("configured auth status must declare Context-wide activation guidance")
	}
	if !configured && r.WorkspaceActivation.State == WorkspaceActivationReentryRequired {
		return fmt.Errorf("unconfigured auth status cannot require Workspace re-entry")
	}
	return nil
}

// ValidateSecretFreeText is exported for infrastructure that needs to validate
// provider-returned account labels before constructing a Result.
func ValidateSecretFreeText(label, value string, maximum int) error {
	if maximum < 1 || maximum > 4096 {
		return fmt.Errorf("text bound is invalid")
	}
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be non-empty, trimmed, and at most %d bytes", label, maximum)
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return fmt.Errorf("%s contains an unsafe character", label)
		}
	}
	return nil
}
