package tobari

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	WorkspaceStateSchemaVersion = 2
	DefaultProfile              = "default"

	TaskEnter         = "tobari.enter"
	TaskStatus        = "tobari.status"
	TaskDelete        = "tobari.delete"
	TaskWorkspaceList = "tobari.project-list"

	CurrentDirectoryTargetKind = "current-directory-tobari"
	CurrentDirectoryTargetID   = "current-directory"
)

// ErrProjectExists means a caller requested creation at a root that became
// indexed after its read-only selection snapshot was taken.
var ErrProjectExists = errors.New("a project already exists at the requested root")

// RuntimeDiagnostic describes recoverable Docker health. It is deliberately
// separate from logical Tobari existence: a missing container never changes a
// valid Workspace into not-exists.
type RuntimeDiagnostic string

const (
	RuntimeDiagnosticUnknown     RuntimeDiagnostic = "unknown"
	RuntimeDiagnosticReady       RuntimeDiagnostic = "ready"
	RuntimeDiagnosticMissing     RuntimeDiagnostic = "missing"
	RuntimeDiagnosticDegraded    RuntimeDiagnostic = "degraded"
	RuntimeDiagnosticUnreachable RuntimeDiagnostic = "unreachable"
	RuntimeDiagnosticIncomplete  RuntimeDiagnostic = "incomplete"
)

func (d RuntimeDiagnostic) Validate() error {
	switch d {
	case RuntimeDiagnosticUnknown, RuntimeDiagnosticReady, RuntimeDiagnosticMissing,
		RuntimeDiagnosticDegraded, RuntimeDiagnosticUnreachable, RuntimeDiagnosticIncomplete:
		return nil
	default:
		return fmt.Errorf("runtime diagnostic is invalid: %q", d)
	}
}

// AttachmentObservation reports transient session state independently from
// logical Workspace existence and recoverable runtime diagnostics.
type AttachmentObservation string

const (
	AttachmentNotApplicable AttachmentObservation = "not_applicable"
	AttachmentDetached      AttachmentObservation = "detached"
	AttachmentAttached      AttachmentObservation = "attached"
)

func (o AttachmentObservation) Validate(exists bool) error {
	if !exists && o == AttachmentNotApplicable {
		return nil
	}
	if exists && (o == AttachmentDetached || o == AttachmentAttached) {
		return nil
	}
	return fmt.Errorf("attachment observation does not match logical existence")
}

// DesiredEntry is the exact next entry-applied identity projected from the
// selected Manifest. It is a read model, not a second persisted desired state.
type DesiredEntry struct {
	ManifestGeneration uint64 `json:"manifest_generation"`
	ManifestRevision   string `json:"manifest_revision"`
	EntryRevision      string `json:"entry_revision"`
	RuntimeID          string `json:"runtime_id"`
	RuntimeRevision    string `json:"runtime_revision"`
}

func NewDesiredEntry(manifest WorkspaceManifest) (DesiredEntry, error) {
	if err := manifest.ValidatePublished(); err != nil {
		return DesiredEntry{}, err
	}
	if manifest.RuntimeBinding == nil {
		return DesiredEntry{}, fmt.Errorf("Workspace Manifest has no exact Runtime binding")
	}
	result := DesiredEntry{
		ManifestGeneration: manifest.Desired.Generation,
		ManifestRevision:   manifest.Desired.Revision,
		EntryRevision:      manifest.Desired.EntryRevision,
		RuntimeID:          manifest.RuntimeBinding.RuntimeID,
		RuntimeRevision:    manifest.RuntimeBinding.Revision,
	}
	return result, result.Validate()
}

func (e DesiredEntry) Validate() error {
	if e.ManifestGeneration == 0 {
		return fmt.Errorf("desired Manifest generation must be positive")
	}
	for name, value := range map[string]string{
		"Manifest": e.ManifestRevision, "entry": e.EntryRevision, "Runtime": e.RuntimeRevision,
	} {
		if err := ValidateDigest(value); err != nil {
			return fmt.Errorf("desired %s revision: %w", name, err)
		}
	}
	if e.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(e.RuntimeID); err != nil {
			return fmt.Errorf("desired Runtime ID: %w", err)
		}
	}
	return nil
}

// WorkspaceStatus is the CWD-scoped lifecycle result. Exists is the only
// user-facing logical lifecycle bit; Runtime is diagnostic detail.
type WorkspaceStatus struct {
	Task                  string                   `json:"task"`
	ManifestState         ManifestObservationState `json:"workspace_manifest_state"`
	Exists                bool                     `json:"exists"`
	Root                  string                   `json:"root,omitempty"`
	ID                    string                   `json:"workspace_id,omitempty"`
	Home                  string                   `json:"home,omitempty"`
	WorkspaceManifestID   string                   `json:"workspace_manifest_id,omitempty"`
	WorkspaceManifestName string                   `json:"workspace_manifest,omitempty"`
	Runtime               RuntimeDiagnostic        `json:"runtime"`
	// RuntimeSelection is presentation metadata resolved from the bound
	// Context. Machine schema v1 retains the existing diagnostic field.
	RuntimeSelection string                   `json:"-"`
	Attachment       AttachmentObservation    `json:"attachment"`
	Bootstrap        WorkspaceBootstrapReport `json:"bootstrap"`
	Adoption         WorkspaceAdoptionState   `json:"adoption,omitempty"`
	Current          *AppliedEntry            `json:"current,omitempty"`
	Next             *DesiredEntry            `json:"next,omitempty"`
	LastFailure      *ReconciliationFailure   `json:"last_reconciliation_failure,omitempty"`
}

func (s WorkspaceStatus) Validate() error {
	if s.Task != TaskStatus {
		return fmt.Errorf("project status task identity is invalid")
	}
	if err := s.Runtime.Validate(); err != nil {
		return err
	}
	if s.RuntimeSelection != "" {
		if err := validateRuntimeDisplaySelection(s.RuntimeSelection); err != nil && s.RuntimeSelection != "context-owned Dockerfile" {
			return err
		}
	}
	if err := s.Attachment.Validate(s.Exists); err != nil {
		return err
	}
	if err := s.ManifestState.Validate(); err != nil {
		return err
	}
	if err := s.Bootstrap.Validate(); err != nil {
		return err
	}
	if s.ManifestState == ManifestObservationAbsent {
		if s.Exists || s.WorkspaceManifestID != "" || s.WorkspaceManifestName != DefaultManifestName {
			return fmt.Errorf("non-persisted project status claims persisted authority")
		}
	} else {
		if err := ValidateWorkspaceManifestID(s.WorkspaceManifestID); err != nil {
			return err
		}
	}
	if err := ValidateName(s.WorkspaceManifestName); err != nil {
		return fmt.Errorf("project Workspace Manifest name: %w", err)
	}
	if !s.Exists {
		if s.Root != "" || s.ID != "" || s.Home != "" || s.Runtime != RuntimeDiagnosticUnknown || s.Current != nil || s.LastFailure != nil {
			return fmt.Errorf("not-existing project status contains Workspace identity or runtime state")
		}
		if s.Next != nil {
			if err := s.Next.Validate(); err != nil {
				return err
			}
		}
		return nil
	}
	if err := ValidateCanonicalRoot(s.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(s.ID); err != nil {
		return err
	}
	if s.Home == "" || filepath.IsAbs(s.Home) == false || filepath.Clean(s.Home) != s.Home {
		return fmt.Errorf("project home is invalid")
	}
	if s.Next == nil {
		return fmt.Errorf("existing Workspace status requires next entry identity")
	}
	if err := s.Next.Validate(); err != nil {
		return err
	}
	if s.Current != nil {
		if err := s.Current.Validate(); err != nil {
			return err
		}
	}
	if s.LastFailure != nil {
		if err := s.LastFailure.Validate(); err != nil {
			return err
		}
	}
	switch s.Adoption {
	case WorkspaceAdoptionNeverApplied:
		if s.Current != nil {
			return fmt.Errorf("never-applied Workspace has a current entry")
		}
	case WorkspaceAdoptionCurrent:
		if s.Current == nil || s.Current.EntryRevision != s.Next.EntryRevision {
			return fmt.Errorf("current Workspace entry identity is inconsistent")
		}
	case WorkspaceAdoptionPending:
		if s.Current == nil || s.Current.EntryRevision == s.Next.EntryRevision {
			return fmt.Errorf("pending Workspace entry identity is inconsistent")
		}
	default:
		return fmt.Errorf("Workspace adoption state is invalid")
	}
	return nil
}

// WorkspaceListItem is one local logical Tobari with runtime diagnostics.
type WorkspaceListItem struct {
	Root                  string                 `json:"root"`
	ID                    string                 `json:"workspace_id"`
	Home                  string                 `json:"home"`
	WorkspaceManifestID   string                 `json:"workspace_manifest_id"`
	WorkspaceManifestName string                 `json:"workspace_manifest"`
	Runtime               RuntimeDiagnostic      `json:"runtime"`
	Adoption              WorkspaceAdoptionState `json:"adoption"`
	Current               *AppliedEntry          `json:"current,omitempty"`
	Next                  DesiredEntry           `json:"next"`
	LastFailure           *ReconciliationFailure `json:"last_reconciliation_failure,omitempty"`
}

func (i WorkspaceListItem) Validate() error {
	if err := ValidateCanonicalRoot(i.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(i.ID); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(i.WorkspaceManifestID); err != nil {
		return err
	}
	if err := ValidateName(i.WorkspaceManifestName); err != nil {
		return fmt.Errorf("project Workspace Manifest name: %w", err)
	}
	if i.Home == "" || !filepath.IsAbs(i.Home) || filepath.Clean(i.Home) != i.Home {
		return fmt.Errorf("project home is invalid")
	}
	if err := i.Next.Validate(); err != nil {
		return err
	}
	if i.Current != nil {
		if err := i.Current.Validate(); err != nil {
			return err
		}
	}
	if i.LastFailure != nil {
		if err := i.LastFailure.Validate(); err != nil {
			return err
		}
	}
	switch i.Adoption {
	case WorkspaceAdoptionNeverApplied:
		if i.Current != nil {
			return fmt.Errorf("never-applied Workspace has a current entry")
		}
	case WorkspaceAdoptionCurrent:
		if i.Current == nil || i.Current.EntryRevision != i.Next.EntryRevision {
			return fmt.Errorf("current Workspace entry identity is inconsistent")
		}
	case WorkspaceAdoptionPending:
		if i.Current == nil || i.Current.EntryRevision == i.Next.EntryRevision {
			return fmt.Errorf("pending Workspace entry identity is inconsistent")
		}
	default:
		return fmt.Errorf("Workspace adoption state is invalid")
	}
	return i.Runtime.Validate()
}

// WorkspaceListResult preserves the complete local logical-state observation,
// including a known empty result.
type WorkspaceListResult struct {
	Task string `json:"task"`
	// CurrentID identifies the nearest Workspace selected by the caller's
	// canonical current directory. It is presentation metadata and is not
	// part of the machine-readable list envelope.
	CurrentID string              `json:"-"`
	Items     []WorkspaceListItem `json:"items"`
}

func (r WorkspaceListResult) Validate() error {
	if r.Task != TaskWorkspaceList || r.Items == nil {
		return fmt.Errorf("project list task or scope is invalid")
	}
	seenIDs := make(map[string]bool, len(r.Items))
	seenBindings := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if seenIDs[item.ID] {
			return fmt.Errorf("project list IDs must be unique")
		}
		binding := item.Root + "\x00" + item.WorkspaceManifestID
		if seenBindings[binding] {
			return fmt.Errorf("project list root and Workspace Manifest bindings must be unique")
		}
		seenIDs[item.ID] = true
		seenBindings[binding] = true
	}
	if r.CurrentID != "" {
		if err := ValidateWorkspaceID(r.CurrentID); err != nil {
			return fmt.Errorf("project list current ID is invalid: %w", err)
		}
		if !seenIDs[r.CurrentID] {
			return fmt.Errorf("project list current ID is not present in items")
		}
	}
	return nil
}

// WorkspaceDeleteResult records the exact logical target whose state was
// confirmed deleted. It is separate from WorkspaceStatus because deletion is a
// completed mutation, not an observation that the target still exists.
type WorkspaceDeleteResult struct {
	Task                  string `json:"task"`
	Deleted               bool   `json:"deleted"`
	Root                  string `json:"root"`
	ID                    string `json:"id"`
	Home                  string `json:"home"`
	WorkspaceManifestID   string `json:"workspace_manifest_id"`
	WorkspaceManifestName string `json:"workspace_manifest"`
}

func (r WorkspaceDeleteResult) Validate() error {
	if r.Task != TaskDelete || !r.Deleted {
		return fmt.Errorf("project delete result is incomplete")
	}
	if err := ValidateCanonicalRoot(r.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(r.ID); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(r.WorkspaceManifestID); err != nil {
		return err
	}
	if err := ValidateName(r.WorkspaceManifestName); err != nil {
		return err
	}
	if r.Home == "" || !filepath.IsAbs(r.Home) || filepath.Clean(r.Home) != r.Home {
		return fmt.Errorf("project home is invalid")
	}
	return nil
}

var projectIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// RootIndex binds one canonical host root to its stable logical Tobari ID.
// It is stored independently so parent lookup does not need Docker access.
type RootIndex struct {
	SchemaVersion         int    `json:"schema_version"`
	Root                  string `json:"root"`
	InstanceID            string `json:"workspace_id"`
	WorkspaceManifestID   string `json:"workspace_manifest_id"`
	WorkspaceManifestName string `json:"workspace_manifest"`
}

// Validate rejects a malformed or ambiguous root index before it becomes a
// selection authority.
func (r RootIndex) Validate() error {
	if r.SchemaVersion != WorkspaceStateSchemaVersion {
		return fmt.Errorf("root index schema version must be %d", WorkspaceStateSchemaVersion)
	}
	if err := ValidateCanonicalRoot(r.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(r.InstanceID); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(r.WorkspaceManifestID); err != nil {
		return err
	}
	return ValidateName(r.WorkspaceManifestName)
}

// ValidateRootIndexes rejects a root-index collection that could make a
// canonical root resolve to more than one logical Workspace. The filesystem
// adapter normally enforces this with the root-hash filename, but the domain
// invariant also protects selection and recovery from malformed collections.
func ValidateRootIndexes(indexes []RootIndex) error {
	seenBindings := make(map[string]struct{}, len(indexes))
	seenIDs := make(map[string]struct{}, len(indexes))
	for _, index := range indexes {
		if err := index.Validate(); err != nil {
			return err
		}
		binding := index.Root + "\x00" + index.WorkspaceManifestID
		if _, exists := seenBindings[binding]; exists {
			return fmt.Errorf("root indexes must contain one Workspace per root and Workspace Manifest")
		}
		if _, exists := seenIDs[index.InstanceID]; exists {
			return fmt.Errorf("root indexes must contain unique Workspace IDs")
		}
		seenBindings[binding] = struct{}{}
		seenIDs[index.InstanceID] = struct{}{}
	}
	return nil
}

// WorkspaceRuntime is recoverable diagnostic state. Empty values mean that the
// resource has not yet been created or its last observation is unknown.
type WorkspaceRuntime struct {
	ContainerID string `json:"container_id"`
	NetworkID   string `json:"network_id"`
}

// WorkspaceCreationApplied records the Manifest creation-default revision
// applied exactly once while the Workspace home was first created.
type WorkspaceCreationApplied struct {
	CreationDefaultsRevision string    `json:"creation_defaults_revision"`
	BootstrapRevision        string    `json:"bootstrap_revision,omitempty"`
	AppliedAt                time.Time `json:"applied_at"`
}

func (a WorkspaceCreationApplied) Validate() error {
	if err := ValidateDigest(a.CreationDefaultsRevision); err != nil {
		return fmt.Errorf("creation defaults revision: %w", err)
	}
	if a.BootstrapRevision != "" {
		if err := ValidateDigest(a.BootstrapRevision); err != nil {
			return fmt.Errorf("bootstrap revision: %w", err)
		}
	}
	if a.AppliedAt.IsZero() || a.AppliedAt.Location() != time.UTC {
		return fmt.Errorf("creation defaults applied time must be non-zero UTC")
	}
	return nil
}

// AppliedEntry is the last entry configuration confirmed after all runtime,
// network-guard, principal, and readiness work succeeded. RuntimeID plus
// RuntimeRevision is the authority consumed by Runtime protection readers;
// names, ordinals, and inferred last-used values are deliberately absent.
type AppliedEntry struct {
	ManifestGeneration uint64    `json:"manifest_generation"`
	ManifestRevision   string    `json:"manifest_revision"`
	EntryRevision      string    `json:"entry_revision"`
	RuntimeID          string    `json:"runtime_id"`
	RuntimeRevision    string    `json:"runtime_revision"`
	ResolvedSpec       string    `json:"resolved_spec_revision"`
	ReconciledAt       time.Time `json:"reconciled_at"`
}

func (a AppliedEntry) Validate() error {
	if a.ManifestGeneration == 0 {
		return fmt.Errorf("applied Manifest generation must be positive")
	}
	for name, value := range map[string]string{
		"Manifest": a.ManifestRevision, "entry": a.EntryRevision,
		"Runtime": a.RuntimeRevision, "resolved spec": a.ResolvedSpec,
	} {
		if err := ValidateDigest(value); err != nil {
			return fmt.Errorf("applied %s revision: %w", name, err)
		}
	}
	if a.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(a.RuntimeID); err != nil {
			return fmt.Errorf("applied Runtime ID: %w", err)
		}
	}
	if a.ReconciledAt.IsZero() || a.ReconciledAt.Location() != time.UTC {
		return fmt.Errorf("entry reconciliation time must be non-zero UTC")
	}
	return nil
}

type ReconciliationChangeState string

const (
	ReconciliationChangeNone    ReconciliationChangeState = "none"
	ReconciliationChangePartial ReconciliationChangeState = "partial"
	ReconciliationChangeUnknown ReconciliationChangeState = "unknown"
)

func (s ReconciliationChangeState) Validate() error {
	switch s {
	case ReconciliationChangeNone, ReconciliationChangePartial, ReconciliationChangeUnknown:
		return nil
	default:
		return fmt.Errorf("reconciliation change state is invalid")
	}
}

// ReconciliationFailure is the bounded latest failed/unknown entry attempt.
// It never replaces LastSuccessfulEntry and contains no external diagnostics.
type ReconciliationFailure struct {
	AttemptedGeneration       uint64                    `json:"attempted_generation"`
	AttemptedManifestRevision string                    `json:"attempted_manifest_revision"`
	AttemptedEntryRevision    string                    `json:"attempted_entry_revision"`
	Phase                     string                    `json:"phase"`
	Code                      string                    `json:"code"`
	ChangeState               ReconciliationChangeState `json:"change_state"`
	OccurredAt                time.Time                 `json:"occurred_at"`
}

func (f ReconciliationFailure) Validate() error {
	if f.AttemptedGeneration == 0 {
		return fmt.Errorf("attempted Manifest generation must be positive")
	}
	if err := ValidateDigest(f.AttemptedManifestRevision); err != nil {
		return fmt.Errorf("attempted Manifest revision: %w", err)
	}
	if err := ValidateDigest(f.AttemptedEntryRevision); err != nil {
		return fmt.Errorf("attempted entry revision: %w", err)
	}
	if !safeStateToken(f.Phase) || !safeStateToken(f.Code) {
		return fmt.Errorf("reconciliation phase or code is invalid")
	}
	if err := f.ChangeState.Validate(); err != nil {
		return err
	}
	if f.OccurredAt.IsZero() || f.OccurredAt.Location() != time.UTC {
		return fmt.Errorf("reconciliation failure time must be non-zero UTC")
	}
	return nil
}

func safeStateToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

type WorkspaceAdoptionState string

const (
	WorkspaceAdoptionNeverApplied WorkspaceAdoptionState = "never_applied"
	WorkspaceAdoptionCurrent      WorkspaceAdoptionState = "current"
	WorkspaceAdoptionPending      WorkspaceAdoptionState = "pending"
)

// AdoptionState is derived from desired and applied entry identities. It is
// intentionally not persisted as an independent boolean.
func (p Workspace) AdoptionState(desired WorkspaceManifestRevision) (WorkspaceAdoptionState, error) {
	if err := desired.Validate(); err != nil {
		return "", err
	}
	if p.LastSuccessfulEntry == nil {
		return WorkspaceAdoptionNeverApplied, nil
	}
	if p.LastSuccessfulEntry.EntryRevision == desired.EntryRevision {
		return WorkspaceAdoptionCurrent, nil
	}
	return WorkspaceAdoptionPending, nil
}

func (r WorkspaceRuntime) Validate() error {
	for name, value := range map[string]string{
		"container ID": r.ContainerID,
		"network ID":   r.NetworkID,
	} {
		if len(value) > 256 || strings.IndexFunc(value, unsafeStateRune) >= 0 {
			return fmt.Errorf("%s is unsafe", name)
		}
	}
	return nil
}

// Workspace is the durable logical Tobari record. Its existence does not
// depend on a work container or network being present.
type Workspace struct {
	SchemaVersion         int                      `json:"schema_version"`
	ID                    string                   `json:"id"`
	Root                  string                   `json:"root"`
	WorkspaceManifestID   string                   `json:"workspace_manifest_id"`
	WorkspaceManifestName string                   `json:"workspace_manifest"`
	Profile               string                   `json:"profile"`
	Image                 string                   `json:"image"`
	Runtime               WorkspaceRuntime         `json:"runtime"`
	CreationApplied       WorkspaceCreationApplied `json:"creation_applied"`
	LastSuccessfulEntry   *AppliedEntry            `json:"last_successful_entry,omitempty"`
	LastFailure           *ReconciliationFailure   `json:"last_reconciliation_failure,omitempty"`
	// Incomplete is an in-memory cleanup-only marker for a root index whose
	// instance record is missing. It is never persisted or used to rebuild a
	// runtime with guessed mutable fields.
	Incomplete bool `json:"-"`
}

// WorkspaceSelectionCandidate is the presentation-independent identity and
// runtime state of one Workspace that contains a canonical current directory.
// ID is an internal binding value; it is never a routine user input.
type WorkspaceSelectionCandidate struct {
	ID                    string
	Root                  string
	WorkspaceManifestID   string
	WorkspaceManifestName string
	Runtime               RuntimeDiagnostic
}

func (c WorkspaceSelectionCandidate) Validate(cwd string) error {
	if err := ValidateCanonicalRoot(cwd); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(c.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(c.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(c.WorkspaceManifestID); err != nil {
		return err
	}
	if err := ValidateName(c.WorkspaceManifestName); err != nil {
		return err
	}
	if !containsRoot(c.Root, cwd) {
		return fmt.Errorf("selection candidate root does not contain the current directory")
	}
	return c.Runtime.Validate()
}

// WorkspaceSelection is a complete read-only snapshot used to resolve an
// ambiguous current-directory entry. Candidates are ordered nearest-first.
// CanCreate is false when the current directory is already an indexed root.
type WorkspaceSelection struct {
	CWD        string                        `json:"-"`
	Candidates []WorkspaceSelectionCandidate `json:"-"`
	CanCreate  bool                          `json:"-"`
}

func (s WorkspaceSelection) Validate() error {
	if err := ValidateCanonicalRoot(s.CWD); err != nil {
		return err
	}
	if s.Candidates == nil {
		return fmt.Errorf("project selection candidates must be an explicit collection")
	}
	seenIDs := make(map[string]bool, len(s.Candidates))
	seenRoots := make(map[string]bool, len(s.Candidates))
	hasCurrentRoot := false
	for index, candidate := range s.Candidates {
		if err := candidate.Validate(s.CWD); err != nil {
			return fmt.Errorf("selection candidate %d is invalid: %w", index, err)
		}
		if seenIDs[candidate.ID] {
			return fmt.Errorf("selection candidate IDs must be unique")
		}
		if seenRoots[candidate.Root] {
			return fmt.Errorf("selection candidate roots must be unique")
		}
		seenIDs[candidate.ID] = true
		seenRoots[candidate.Root] = true
		if candidate.Root == s.CWD {
			hasCurrentRoot = true
		}
		if index > 0 {
			previous := s.Candidates[index-1]
			if len(previous.Root) < len(candidate.Root) ||
				(len(previous.Root) == len(candidate.Root) && previous.Root > candidate.Root) {
				return fmt.Errorf("selection candidates must be ordered nearest-first")
			}
		}
	}
	if s.CanCreate == hasCurrentRoot {
		return fmt.Errorf("selection create option does not match current-root presence")
	}
	return nil
}

// RequiresChoice is true only when one or more ancestor candidates exist and
// no exact current-root Workspace can be entered directly.
func (s WorkspaceSelection) RequiresChoice() bool {
	return len(s.Candidates) > 0 && s.Candidates[0].Root != s.CWD
}

func (s WorkspaceSelection) Candidate(id string) (WorkspaceSelectionCandidate, bool) {
	for _, candidate := range s.Candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return WorkspaceSelectionCandidate{}, false
}

type ProjectSelectionChoiceKind string

const (
	ProjectSelectionUse    ProjectSelectionChoiceKind = "use"
	ProjectSelectionCreate ProjectSelectionChoiceKind = "create"
)

// ProjectSelectionChoice is returned by the interactive selector. A use
// choice binds one candidate ID from the validated snapshot; create has no ID
// and always means the canonical current directory.
type ProjectSelectionChoice struct {
	Kind ProjectSelectionChoiceKind
	ID   string
}

func (s WorkspaceSelection) ValidateChoice(choice ProjectSelectionChoice) error {
	switch choice.Kind {
	case ProjectSelectionCreate:
		if choice.ID != "" {
			return fmt.Errorf("create choice must not contain a candidate ID")
		}
		if !s.CanCreate {
			return fmt.Errorf("current directory already has a Workspace")
		}
		return nil
	case ProjectSelectionUse:
		if choice.ID == "" {
			return fmt.Errorf("use choice requires a candidate ID")
		}
		candidate, found := s.Candidate(choice.ID)
		if !found {
			return fmt.Errorf("selection candidate is not present in the snapshot")
		}
		if candidate.Runtime == RuntimeDiagnosticIncomplete {
			return fmt.Errorf("selection candidate has incomplete logical state")
		}
		return nil
	default:
		return fmt.Errorf("selection choice kind is invalid")
	}
}

// Validate rejects invalid durable logical state before runtime reconciliation.
func (p Workspace) Validate() error {
	if p.SchemaVersion != WorkspaceStateSchemaVersion {
		return fmt.Errorf("instance state schema version must be %d", WorkspaceStateSchemaVersion)
	}
	if err := ValidateWorkspaceID(p.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(p.Root); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(p.WorkspaceManifestID); err != nil {
		return err
	}
	if err := ValidateName(p.WorkspaceManifestName); err != nil {
		return fmt.Errorf("project Workspace Manifest name: %w", err)
	}
	if p.Profile != DefaultProfile {
		return fmt.Errorf("profile is invalid")
	}
	if err := ValidateImageSelector(p.Image); err != nil {
		return err
	}
	if p.Incomplete {
		if p.CreationApplied != (WorkspaceCreationApplied{}) || p.Runtime != (WorkspaceRuntime{}) || p.LastSuccessfulEntry != nil || p.LastFailure != nil {
			return fmt.Errorf("cleanup-only Workspace invents unavailable instance state")
		}
		return nil
	}
	if err := p.CreationApplied.Validate(); err != nil {
		return err
	}
	if p.LastSuccessfulEntry != nil {
		if err := p.LastSuccessfulEntry.Validate(); err != nil {
			return err
		}
	}
	if p.LastFailure != nil {
		if err := p.LastFailure.Validate(); err != nil {
			return err
		}
	}
	return p.Runtime.Validate()
}

// ProjectWorkspaceRoot returns the container path that mirrors the absolute
// host root below the shared /workspace prefix. Only the selected root is
// mounted there; the full path keeps a nested host cwd unambiguous.
func ProjectWorkspaceRoot(root string) (string, error) {
	if err := ValidateCanonicalRoot(root); err != nil {
		return "", err
	}
	if root == string(filepath.Separator) {
		return "/workspace", nil
	}
	return "/workspace" + filepath.ToSlash(root), nil
}

// MapProjectCWD maps an absolute host cwd below a CWD-owned root into the
// mirrored container workspace path.
func MapProjectCWD(root, cwd string) (string, error) {
	workspaceRoot, err := ProjectWorkspaceRoot(root)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(cwd) || filepath.Clean(cwd) != cwd {
		return "", fmt.Errorf("cwd must be canonical and absolute")
	}
	relative, err := filepath.Rel(root, cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd below project root: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd must be inside the selected project root")
	}
	if relative == "." {
		return workspaceRoot, nil
	}
	return workspaceRoot + "/" + filepath.ToSlash(relative), nil
}

// ValidateWorkspaceID accepts only a canonical UUIDv7 logical identity.
func ValidateWorkspaceID(id string) error {
	if !projectIDPattern.MatchString(id) {
		return fmt.Errorf("Tobari project ID is invalid")
	}
	return nil
}

// NewWorkspaceID produces a UUIDv7 logical identity from the supplied clock and
// entropy source. The source exists so callers can make creation deterministic
// in tests without weakening production entropy.
func NewWorkspaceID(now time.Time, source io.Reader) (string, error) {
	if source == nil {
		return "", fmt.Errorf("project ID entropy source is required")
	}
	if now.UnixMilli() < 0 || now.UnixMilli() >= 1<<48 {
		return "", fmt.Errorf("project ID timestamp is outside UUIDv7 range")
	}
	var bytes [16]byte
	milliseconds := uint64(now.UnixMilli())
	for index := 5; index >= 0; index-- {
		bytes[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	if _, err := io.ReadFull(source, bytes[6:]); err != nil {
		return "", fmt.Errorf("read project ID entropy: %w", err)
	}
	bytes[6] = 0x70 | (bytes[6] & 0x0f)
	bytes[8] = 0x80 | (bytes[8] & 0x3f)
	id := fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16],
	)
	if err := ValidateWorkspaceID(id); err != nil {
		return "", err
	}
	return id, nil
}

// ProjectInstanceRequest declares the complete durable identity and runtime
// selection required to create a logical project before Docker resources
// exist.
type ProjectInstanceRequest struct {
	Root                     string
	WorkspaceManifestID      string
	WorkspaceManifestName    string
	Image                    string
	CreationDefaultsRevision string
	BootstrapRevision        string
	CreatedAt                time.Time
}

// NewProjectInstance creates the durable logical state before any Docker
// resource exists.
func NewProjectInstance(now time.Time, source io.Reader, request ProjectInstanceRequest) (Workspace, error) {
	if err := ValidateCanonicalRoot(request.Root); err != nil {
		return Workspace{}, err
	}
	if err := ValidateImageSelector(request.Image); err != nil {
		return Workspace{}, err
	}
	if err := ValidateWorkspaceManifestID(request.WorkspaceManifestID); err != nil {
		return Workspace{}, err
	}
	if err := ValidateName(request.WorkspaceManifestName); err != nil {
		return Workspace{}, err
	}
	if err := ValidateDigest(request.CreationDefaultsRevision); err != nil {
		return Workspace{}, fmt.Errorf("creation defaults revision: %w", err)
	}
	if request.CreatedAt.IsZero() || request.CreatedAt.Location() != time.UTC {
		return Workspace{}, fmt.Errorf("Workspace creation time must be non-zero UTC")
	}
	id, err := NewWorkspaceID(now, source)
	if err != nil {
		return Workspace{}, err
	}
	instance := Workspace{
		SchemaVersion:         WorkspaceStateSchemaVersion,
		ID:                    id,
		Root:                  request.Root,
		WorkspaceManifestID:   request.WorkspaceManifestID,
		WorkspaceManifestName: request.WorkspaceManifestName,
		Profile:               DefaultProfile,
		Image:                 request.Image,
		Runtime:               WorkspaceRuntime{},
		CreationApplied: WorkspaceCreationApplied{
			CreationDefaultsRevision: request.CreationDefaultsRevision,
			BootstrapRevision:        request.BootstrapRevision,
			AppliedAt:                request.CreatedAt,
		},
	}
	if err := instance.Validate(); err != nil {
		return Workspace{}, err
	}
	return instance, nil
}

// ValidateCanonicalRoot accepts exactly the form produced by infrastructure
// after absolute-path, clean, symlink, and directory checks have completed.
func ValidateCanonicalRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("root must be a canonical absolute path")
	}
	return nil
}

// ContainingRoots returns every indexed root containing cwd, ordered from the
// nearest root to the farthest ancestor. Inputs have already been canonicalized
// by infrastructure, but each index is still validated before use.
func ContainingRoots(cwd string, indexes []RootIndex) ([]RootIndex, error) {
	if err := ValidateCanonicalRoot(cwd); err != nil {
		return nil, err
	}
	if err := ValidateRootIndexes(indexes); err != nil {
		return nil, err
	}
	containing := make([]RootIndex, 0, len(indexes))
	for _, index := range indexes {
		if !containsRoot(index.Root, cwd) {
			continue
		}
		containing = append(containing, index)
	}
	sort.SliceStable(containing, func(left, right int) bool {
		if len(containing[left].Root) != len(containing[right].Root) {
			return len(containing[left].Root) > len(containing[right].Root)
		}
		return containing[left].Root < containing[right].Root
	})
	return containing, nil
}

// RootIndexesForContext returns only bindings owned by one stable Context.
func RootIndexesForContext(indexes []RootIndex, contextID string) ([]RootIndex, error) {
	if err := ValidateRootIndexes(indexes); err != nil {
		return nil, err
	}
	if err := ValidateWorkspaceManifestID(contextID); err != nil {
		return nil, err
	}
	result := make([]RootIndex, 0, len(indexes))
	for _, index := range indexes {
		if index.WorkspaceManifestID == contextID {
			result = append(result, index)
		}
	}
	return result, nil
}

// NearestRoot returns the containing indexed root with the longest canonical
// path. Inputs have already been canonicalized by infrastructure.
func NearestRoot(cwd string, indexes []RootIndex) (RootIndex, bool, error) {
	containing, err := ContainingRoots(cwd, indexes)
	if err != nil {
		return RootIndex{}, false, err
	}
	if len(containing) == 0 {
		return RootIndex{}, false, nil
	}
	return containing[0], true, nil
}

func containsRoot(root, candidate string) bool {
	if root == candidate {
		return true
	}
	if root == string(filepath.Separator) {
		return strings.HasPrefix(candidate, root)
	}
	return strings.HasPrefix(candidate, root+string(filepath.Separator))
}

// ValidateRootContains proves that one canonical selected Project root owns
// the canonical invocation directory. Root entry keeps these as distinct
// dimensions so an ancestor Workspace can open at the caller's descendant
// working directory without widening its mount.
func ValidateRootContains(root, candidate string) error {
	if err := ValidateCanonicalRoot(root); err != nil {
		return fmt.Errorf("selected root is invalid: %w", err)
	}
	if err := ValidateCanonicalRoot(candidate); err != nil {
		return fmt.Errorf("invocation root is invalid: %w", err)
	}
	if !containsRoot(root, candidate) {
		return fmt.Errorf("invocation root is outside the selected Project root")
	}
	return nil
}

func unsafeStateRune(r rune) bool {
	return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
}

// ProjectResourceNames derives owned Docker resource names from a stable ID.
// Runtime identifiers never become the source of logical identity.
func ProjectResourceNames(id string) (container, network string, err error) {
	if err := ValidateWorkspaceID(id); err != nil {
		return "", "", err
	}
	short := strings.ReplaceAll(id[:13], "-", "")
	return "tobari-" + short + "-work", "tobari-" + short + "-net", nil
}
