package authbroker

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	TaskLogin  = "auth.login"
	TaskImport = "auth.import"
	TaskStatus = "auth.status"
	TaskLogout = "auth.logout"

	// ManifestAuthReentryGuidance is emitted only when authoritative project
	// projection evidence identifies at least one exact Workspace that needs
	// reconciliation.
	ManifestAuthReentryGuidance = "One or more Workspace authentication projections are stale or missing. Run each listed action from its exact working directory."

	CredentialCatalogTargetKind = "auth-credentials"
	CredentialCatalogTargetID   = "active-context-auth-credentials"

	// Workspace activation output is bounded independently from host discovery
	// so an adapter cannot turn a semantic result into unbounded CLI output.
	MaxWorkspaceActivationItems         = 1024
	MaxWorkspaceActivationProviders     = maxProviders
	MaxWorkspaceActivationBindingChecks = 1024
	MaxWorkspaceActivationRootBytes     = 4096
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
	WorkspaceActivationUnavailable     WorkspaceActivationState = "unavailable"
	WorkspaceActivationUnresolved      WorkspaceActivationState = "unresolved"
)

func (s WorkspaceActivationState) Validate() error {
	switch s {
	case WorkspaceActivationNotApplicable, WorkspaceActivationReady, WorkspaceActivationReentryRequired,
		WorkspaceActivationUnavailable, WorkspaceActivationUnresolved:
		return nil
	default:
		return fmt.Errorf("Workspace activation state is invalid: %q", s)
	}
}

type WorkspaceActivationCoverage string

const (
	WorkspaceActivationCoverageNotApplicable WorkspaceActivationCoverage = "not_applicable"
	WorkspaceActivationCoverageExhaustive    WorkspaceActivationCoverage = "exhaustive"
	WorkspaceActivationCoverageUnavailable   WorkspaceActivationCoverage = "unavailable"
)

func (c WorkspaceActivationCoverage) Validate() error {
	switch c {
	case WorkspaceActivationCoverageNotApplicable, WorkspaceActivationCoverageExhaustive,
		WorkspaceActivationCoverageUnavailable:
		return nil
	default:
		return fmt.Errorf("Workspace activation coverage is invalid: %q", c)
	}
}

type WorkspaceProviderProjectionState string

const (
	WorkspaceProviderProjectionNotApplicable WorkspaceProviderProjectionState = "not_applicable"
	WorkspaceProviderProjectionCurrent       WorkspaceProviderProjectionState = "current"
	WorkspaceProviderProjectionMissing       WorkspaceProviderProjectionState = "missing"
	WorkspaceProviderProjectionStale         WorkspaceProviderProjectionState = "stale"
	WorkspaceProviderProjectionUnavailable   WorkspaceProviderProjectionState = "unavailable"
)

func (s WorkspaceProviderProjectionState) Validate() error {
	switch s {
	case WorkspaceProviderProjectionNotApplicable, WorkspaceProviderProjectionCurrent,
		WorkspaceProviderProjectionMissing, WorkspaceProviderProjectionStale,
		WorkspaceProviderProjectionUnavailable:
		return nil
	default:
		return fmt.Errorf("Workspace provider projection state is invalid: %q", s)
	}
}

// WorkspaceProviderActivation is one provider projection observation within
// an exact Context/project scope. It never carries a handle or binding digest.
type WorkspaceProviderActivation struct {
	Provider string                           `json:"provider"`
	State    WorkspaceProviderProjectionState `json:"state"`
}

func (a WorkspaceProviderActivation) Validate() error {
	if err := ValidateProviderID(a.Provider); err != nil {
		return err
	}
	return a.State.Validate()
}

// WorkspaceActivationAction is complete only together with its working
// directory. The Context flag prevents a later current-Context change from
// retargeting the action.
type WorkspaceActivationAction struct {
	WorkingDirectory string   `json:"working_directory"`
	Argv             []string `json:"argv"`
}

type WorkspaceActivationScopeState string

const (
	WorkspaceActivationScopeComplete   WorkspaceActivationScopeState = "complete"
	WorkspaceActivationScopeIncomplete WorkspaceActivationScopeState = "incomplete"
)

func (s WorkspaceActivationScopeState) Validate() error {
	switch s {
	case WorkspaceActivationScopeComplete, WorkspaceActivationScopeIncomplete:
		return nil
	default:
		return fmt.Errorf("Workspace activation scope state is invalid: %q", s)
	}
}

// WorkspaceActivationItem owns one exact logical Workspace observation.
// ProjectID is identity; Root is a separately validated working-directory
// fact and is never inferred from presentation order or labels.
type WorkspaceActivationItem struct {
	ProjectID           string                        `json:"workspace_id"`
	Root                string                        `json:"project_root"`
	Context             string                        `json:"workspace_manifest"`
	WorkspaceManifestID string                        `json:"workspace_manifest_id"`
	ScopeState          WorkspaceActivationScopeState `json:"scope_state"`
	State               WorkspaceActivationState      `json:"state"`
	Providers           []WorkspaceProviderActivation `json:"providers"`
	NextAction          *WorkspaceActivationAction    `json:"next_action"`
}

func NewWorkspaceActivationItem(
	projectID, root, contextName, contextID string,
	providers []WorkspaceProviderActivation,
	unresolved bool,
) (WorkspaceActivationItem, error) {
	item := WorkspaceActivationItem{
		ProjectID: projectID, Root: root, Context: contextName, WorkspaceManifestID: contextID,
		ScopeState: WorkspaceActivationScopeComplete,
		Providers:  append(make([]WorkspaceProviderActivation, 0, len(providers)), providers...),
	}
	sort.Slice(item.Providers, func(left, right int) bool {
		return item.Providers[left].Provider < item.Providers[right].Provider
	})
	counts := map[WorkspaceProviderProjectionState]int{}
	for _, provider := range item.Providers {
		counts[provider.State]++
	}
	item.State = summarizeWorkspaceProviderStates(counts)
	if unresolved {
		item.ScopeState = WorkspaceActivationScopeIncomplete
		item.State = WorkspaceActivationUnresolved
	}
	if item.State == WorkspaceActivationReentryRequired {
		item.NextAction = &WorkspaceActivationAction{
			WorkingDirectory: root,
			Argv:             []string{"tobari", "--manifest", contextName},
		}
	}
	if err := item.Validate(); err != nil {
		return WorkspaceActivationItem{}, err
	}
	return item, nil
}

func (i WorkspaceActivationItem) Validate() error {
	if err := tobari.ValidateWorkspaceID(i.ProjectID); err != nil {
		return err
	}
	if err := tobari.ValidateCanonicalRoot(i.Root); err != nil {
		return err
	}
	if !utf8.ValidString(i.Root) || len(i.Root) > MaxWorkspaceActivationRootBytes {
		return fmt.Errorf("Workspace activation root exceeds its output bound")
	}
	if !contextNamePattern.MatchString(i.Context) || !contextIDPattern.MatchString(i.WorkspaceManifestID) {
		return fmt.Errorf("Workspace activation Context binding is invalid")
	}
	if err := i.State.Validate(); err != nil {
		return err
	}
	if err := i.ScopeState.Validate(); err != nil {
		return err
	}
	if i.Providers == nil {
		return fmt.Errorf("Workspace provider activation collection is absent")
	}
	if len(i.Providers) > MaxWorkspaceActivationProviders {
		return fmt.Errorf("Workspace provider activation collection exceeds %d items", MaxWorkspaceActivationProviders)
	}
	seen := make(map[string]struct{}, len(i.Providers))
	counts := map[WorkspaceProviderProjectionState]int{}
	for index, provider := range i.Providers {
		if err := provider.Validate(); err != nil {
			return fmt.Errorf("Workspace provider activation %d: %w", index, err)
		}
		if _, duplicate := seen[provider.Provider]; duplicate {
			return fmt.Errorf("Workspace provider activation %q is duplicated", provider.Provider)
		}
		if index > 0 && i.Providers[index-1].Provider >= provider.Provider {
			return fmt.Errorf("Workspace provider activation collection is not in provider order")
		}
		seen[provider.Provider] = struct{}{}
		counts[provider.State]++
	}
	want := summarizeWorkspaceProviderStates(counts)
	if i.State != want && !(i.State == WorkspaceActivationUnresolved &&
		(i.ScopeState == WorkspaceActivationScopeIncomplete || counts[WorkspaceProviderProjectionUnavailable] > 0)) {
		return fmt.Errorf("Workspace activation state %q does not match provider observations", i.State)
	}
	if i.ScopeState == WorkspaceActivationScopeIncomplete && i.State != WorkspaceActivationUnresolved {
		return fmt.Errorf("incomplete Workspace scope must remain unresolved")
	}
	if i.State == WorkspaceActivationReentryRequired {
		if i.NextAction == nil || i.NextAction.WorkingDirectory != i.Root ||
			len(i.NextAction.Argv) != 3 || i.NextAction.Argv[0] != "tobari" ||
			i.NextAction.Argv[1] != "--manifest" || i.NextAction.Argv[2] != i.Context {
			return fmt.Errorf("Workspace re-entry action does not bind its exact root and Context")
		}
	} else if i.NextAction != nil {
		return fmt.Errorf("Workspace activation state %q cannot declare a next action", i.State)
	}
	return nil
}

func summarizeWorkspaceProviderStates(counts map[WorkspaceProviderProjectionState]int) WorkspaceActivationState {
	reentry := counts[WorkspaceProviderProjectionMissing]+counts[WorkspaceProviderProjectionStale] > 0
	unavailable := counts[WorkspaceProviderProjectionUnavailable] > 0
	if reentry && unavailable {
		return WorkspaceActivationUnresolved
	}
	if reentry {
		return WorkspaceActivationReentryRequired
	}
	if unavailable {
		return WorkspaceActivationUnavailable
	}
	if counts[WorkspaceProviderProjectionCurrent] > 0 {
		return WorkspaceActivationReady
	}
	return WorkspaceActivationNotApplicable
}

// WorkspaceActivation gives a bounded, secret-free, Context-scoped collection
// of project projection observations. It never claims that a running process
// environment or file was changed.
type WorkspaceActivation struct {
	State               WorkspaceActivationState    `json:"state"`
	Coverage            WorkspaceActivationCoverage `json:"coverage"`
	Context             string                      `json:"workspace_manifest"`
	WorkspaceManifestID string                      `json:"workspace_manifest_id"`
	Workspaces          []WorkspaceActivationItem   `json:"workspaces"`
	Guidance            string                      `json:"guidance"`
}

func NotApplicableWorkspaceActivation() WorkspaceActivation {
	return WorkspaceActivation{
		State: WorkspaceActivationNotApplicable, Coverage: WorkspaceActivationCoverageNotApplicable,
		Workspaces: []WorkspaceActivationItem{},
	}
}

func UnavailableWorkspaceActivation(contextName, contextID string) WorkspaceActivation {
	return WorkspaceActivation{
		State: WorkspaceActivationUnavailable, Coverage: WorkspaceActivationCoverageUnavailable,
		Context: contextName, WorkspaceManifestID: contextID, Workspaces: []WorkspaceActivationItem{},
	}
}

func NewWorkspaceActivation(
	contextName, contextID string, workspaces []WorkspaceActivationItem,
) (WorkspaceActivation, error) {
	activation := WorkspaceActivation{
		Coverage: WorkspaceActivationCoverageExhaustive, Context: contextName, WorkspaceManifestID: contextID,
		Workspaces: append(make([]WorkspaceActivationItem, 0, len(workspaces)), workspaces...),
	}
	sort.Slice(activation.Workspaces, func(left, right int) bool {
		return activation.Workspaces[left].ProjectID < activation.Workspaces[right].ProjectID
	})
	counts := map[WorkspaceActivationState]int{}
	for _, workspace := range activation.Workspaces {
		counts[workspace.State]++
	}
	activation.State = summarizeWorkspaceStates(counts)
	if activation.State == WorkspaceActivationReentryRequired {
		activation.Guidance = ManifestAuthReentryGuidance
	}
	if err := activation.Validate(); err != nil {
		return WorkspaceActivation{}, err
	}
	return activation, nil
}

func (a WorkspaceActivation) Validate() error {
	if err := a.State.Validate(); err != nil {
		return err
	}
	if err := a.Coverage.Validate(); err != nil {
		return err
	}
	if a.Workspaces == nil {
		return fmt.Errorf("Workspace activation collection is absent")
	}
	if len(a.Workspaces) > MaxWorkspaceActivationItems {
		return fmt.Errorf("Workspace activation collection exceeds %d items", MaxWorkspaceActivationItems)
	}
	if a.Coverage == WorkspaceActivationCoverageNotApplicable {
		if a.State != WorkspaceActivationNotApplicable || len(a.Workspaces) != 0 || a.Guidance != "" {
			return fmt.Errorf("not-applicable Workspace activation cannot claim observations or guidance")
		}
		return nil
	}
	if !contextNamePattern.MatchString(a.Context) || !contextIDPattern.MatchString(a.WorkspaceManifestID) {
		return fmt.Errorf("Workspace activation scope is invalid")
	}
	if a.Coverage == WorkspaceActivationCoverageUnavailable {
		if a.State != WorkspaceActivationUnavailable || len(a.Workspaces) != 0 || a.Guidance != "" {
			return fmt.Errorf("unavailable Workspace activation coverage cannot claim scoped observations")
		}
		return nil
	}
	seen := make(map[string]struct{}, len(a.Workspaces))
	counts := map[WorkspaceActivationState]int{}
	for index, workspace := range a.Workspaces {
		if err := workspace.Validate(); err != nil {
			return fmt.Errorf("Workspace activation %d: %w", index, err)
		}
		if workspace.Context != a.Context || workspace.WorkspaceManifestID != a.WorkspaceManifestID {
			return fmt.Errorf("Workspace activation escapes its Context scope")
		}
		if _, duplicate := seen[workspace.ProjectID]; duplicate {
			return fmt.Errorf("Workspace activation project %q is duplicated", workspace.ProjectID)
		}
		if index > 0 && a.Workspaces[index-1].ProjectID >= workspace.ProjectID {
			return fmt.Errorf("Workspace activation collection is not in project order")
		}
		seen[workspace.ProjectID] = struct{}{}
		counts[workspace.State]++
	}
	want := summarizeWorkspaceStates(counts)
	if a.State != want {
		return fmt.Errorf("Workspace activation summary %q does not match scoped rows", a.State)
	}
	if a.State == WorkspaceActivationReentryRequired {
		if a.Guidance != ManifestAuthReentryGuidance {
			return fmt.Errorf("Workspace activation guidance is not the exact re-entry contract")
		}
	} else if a.Guidance != "" {
		return fmt.Errorf("Workspace activation guidance must be empty for state %q", a.State)
	}
	return nil
}

func summarizeWorkspaceStates(counts map[WorkspaceActivationState]int) WorkspaceActivationState {
	if counts[WorkspaceActivationUnresolved] > 0 ||
		(counts[WorkspaceActivationReentryRequired] > 0 && counts[WorkspaceActivationUnavailable] > 0) {
		return WorkspaceActivationUnresolved
	}
	if counts[WorkspaceActivationReentryRequired] > 0 {
		return WorkspaceActivationReentryRequired
	}
	if counts[WorkspaceActivationUnavailable] > 0 {
		return WorkspaceActivationUnavailable
	}
	if counts[WorkspaceActivationReady] > 0 {
		return WorkspaceActivationReady
	}
	return WorkspaceActivationNotApplicable
}

type MutationChange string

const (
	MutationChangeChanged  MutationChange = "changed"
	MutationChangeNoChange MutationChange = "no_change"
)

func (c MutationChange) Validate() error {
	switch c {
	case MutationChangeChanged, MutationChangeNoChange:
		return nil
	default:
		return fmt.Errorf("auth mutation change state is invalid: %q", c)
	}
}

// Result is the secret-free result shared by login, stdin import, status, and
// logout. A nil AccountLabel means the provider did not make an account label
// available; it never stands for the primary secret.
type Result struct {
	Task                string                          `json:"task"`
	ManifestState       tobari.ManifestObservationState `json:"workspace_manifest_state"`
	Provider            string                          `json:"provider"`
	Context             string                          `json:"workspace_manifest"`
	WorkspaceManifestID string                          `json:"workspace_manifest_id"`
	Configured          bool                            `json:"configured"`
	AccountLabel        *string                         `json:"account_label"`
	StorageBackend      StorageBackend                  `json:"storage_backend"`
	BrokerState         BrokerState                     `json:"broker_state"`
	CredentialRevision  string                          `json:"credential_revision"`
	Change              MutationChange                  `json:"change"`
	WorkspaceActivation WorkspaceActivation             `json:"workspace_activation"`
}

func (r Result) Validate() error {
	switch r.Task {
	case TaskLogin, TaskImport, TaskLogout:
	default:
		return fmt.Errorf("auth result task is invalid: %q", r.Task)
	}
	if r.ManifestState != tobari.ManifestObservationPersisted {
		return fmt.Errorf("auth mutation result requires a persisted Context")
	}
	if !contextNamePattern.MatchString(r.Context) {
		return fmt.Errorf("auth result Context name is invalid")
	}
	if !contextIDPattern.MatchString(r.WorkspaceManifestID) {
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
	if err := r.Change.Validate(); err != nil {
		return err
	}
	if r.Task != TaskLogout && r.Change != MutationChangeChanged {
		return fmt.Errorf("successful auth login/import must confirm a changed credential")
	}
	if r.Change == MutationChangeNoChange {
		if r.Task != TaskLogout || r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageNotApplicable {
			return fmt.Errorf("only no-op logout may report no change")
		}
	} else if r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageExhaustive &&
		r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageUnavailable {
		return fmt.Errorf("changed auth mutation must report exhaustive or explicitly unavailable Context Workspace activation")
	}
	if r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageNotApplicable &&
		(r.WorkspaceActivation.Context != r.Context || r.WorkspaceActivation.WorkspaceManifestID != r.WorkspaceManifestID) {
		return fmt.Errorf("auth mutation Workspace scope does not match its Context")
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
// primary secret or project-bound handle. State is the sole configuration
// authority.
type ProviderStatus struct {
	Provider           string                  `json:"provider"`
	State              ProviderCredentialState `json:"state"`
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
		if !revisionPattern.MatchString(s.CredentialRevision) {
			return fmt.Errorf("configured provider status credential revision is invalid")
		}
	} else {
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
	Task                string                          `json:"task"`
	ManifestState       tobari.ManifestObservationState `json:"workspace_manifest_state"`
	Context             string                          `json:"workspace_manifest"`
	WorkspaceManifestID string                          `json:"workspace_manifest_id"`
	StorageBackend      StorageBackend                  `json:"storage_backend"`
	BrokerState         BrokerState                     `json:"broker_state"`
	Providers           []ProviderStatus                `json:"providers"`
	WorkspaceActivation WorkspaceActivation             `json:"workspace_activation"`
}

func (r StatusResult) Validate() error {
	if r.Task != TaskStatus {
		return fmt.Errorf("auth status result task must be %q", TaskStatus)
	}
	if !contextNamePattern.MatchString(r.Context) {
		return fmt.Errorf("auth status Context name is invalid")
	}
	if err := r.ManifestState.Validate(); err != nil {
		return err
	}
	if r.ManifestState == tobari.ManifestObservationAbsent {
		if r.Context != tobari.DefaultManifestName || r.WorkspaceManifestID != "" || r.BrokerState != BrokerStateUnavailable {
			return fmt.Errorf("non-persisted auth status claims persisted Context authority")
		}
	} else if !contextIDPattern.MatchString(r.WorkspaceManifestID) {
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
	}
	if err := r.WorkspaceActivation.Validate(); err != nil {
		return err
	}
	if r.ManifestState == tobari.ManifestObservationPersisted {
		if (r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageExhaustive &&
			r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageUnavailable) ||
			r.WorkspaceActivation.Context != r.Context || r.WorkspaceActivation.WorkspaceManifestID != r.WorkspaceManifestID {
			return fmt.Errorf("auth status Workspace activation does not preserve its Context scope")
		}
	} else if r.WorkspaceActivation.Coverage != WorkspaceActivationCoverageNotApplicable {
		return fmt.Errorf("non-persisted auth status cannot claim Workspace activation coverage")
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
