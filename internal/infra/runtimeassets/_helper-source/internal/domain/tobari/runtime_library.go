package tobari

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	RuntimeSchemaVersion         = 1
	RuntimeReferenceKind         = "runtime"
	RuntimeRevisionReferenceKind = "runtime-revision"
	StandardRuntimeID            = "builtin/standard"
	StandardRuntimeName          = "standard"

	TaskRuntimeList    = "runtime.list"
	TaskRuntimeShow    = "runtime.show"
	TaskRuntimeCreate  = "runtime.create"
	TaskRuntimeBuildV1 = "runtime.build"
	TaskRuntimeHistory = "runtime.history"

	RuntimeCatalogTargetKind = "runtimes"
	RuntimeCatalogTargetID   = "runtime-catalog"
)

var (
	ErrRuntimeExists                       = errors.New("Runtime already exists")
	ErrRuntimeNotFound                     = errors.New("Runtime does not exist")
	ErrRuntimeNotReady                     = errors.New("Runtime revision is not ready")
	ErrRuntimePrunePlanStale               = errors.New("Runtime prune plan requires a fresh review")
	ErrRuntimePruneInterrupted             = errors.New("Runtime prune requires reconciliation")
	ErrRuntimeRetirementObservationUnknown = errors.New("Runtime lifecycle observation is incomplete")
	ErrRuntimeRevisionNotFound             = errors.New("Runtime revision does not exist")
	ErrRuntimeRevisionUnrestorable         = errors.New("Runtime revision cannot be restored exactly")
	ErrRuntimeLifecycleActive              = errors.New("Runtime lifecycle mutation is already active")
	ErrRuntimeRestoreInterrupted           = errors.New("Runtime restore requires reconciliation")
)

// RuntimeKind distinguishes the compiled standard Runtime from a managed
// editable source tree.
type RuntimeKind string

const (
	RuntimeKindBuiltin RuntimeKind = "builtin"
	RuntimeKindManaged RuntimeKind = "managed"
)

func (k RuntimeKind) Validate() error {
	switch k {
	case RuntimeKindBuiltin, RuntimeKindManaged:
		return nil
	default:
		return fmt.Errorf("Runtime kind is invalid: %q", k)
	}
}

// RuntimeCopySource identifies the editable source used once to initialize a
// standalone managed Runtime. It is selection input, not persisted lineage.
type RuntimeCopySource string

func ParseRuntimeCopySource(value string) (RuntimeCopySource, error) {
	if value == "" {
		return "", fmt.Errorf("Runtime source Base is required")
	}
	if value != StandardRuntimeName {
		if err := ValidateName(value); err != nil {
			return "", fmt.Errorf("Runtime source Base: %w", err)
		}
	}
	return RuntimeCopySource(value), nil
}

func (b RuntimeCopySource) Validate() error {
	_, err := ParseRuntimeCopySource(string(b))
	return err
}

// RuntimeRevision is one immutable successful semantic build.
type RuntimeRevision struct {
	Ordinal      int       `json:"ordinal"`
	Revision     string    `json:"revision"`
	Image        string    `json:"image"`
	ImageDigest  string    `json:"image_digest"`
	CreatedAt    time.Time `json:"created_at"`
	RuntimeRef   string    `json:"runtime_ref,omitempty"`
	RevisionRef  string    `json:"revision_ref,omitempty"`
	SnapshotPath string    `json:"snapshot_path,omitempty"`
}

func (r RuntimeRevision) Validate(kind RuntimeKind) error {
	if r.Ordinal < 1 {
		return fmt.Errorf("Runtime revision ordinal must be positive")
	}
	if err := ValidateDigest(r.Revision); err != nil {
		return fmt.Errorf("Runtime revision: %w", err)
	}
	if err := ValidateImageSelector(r.Image); err != nil {
		return fmt.Errorf("Runtime revision image: %w", err)
	}
	if kind == RuntimeKindManaged {
		if err := ValidateDigest(r.ImageDigest); err != nil {
			return fmt.Errorf("Runtime image digest: %w", err)
		}
		if r.SnapshotPath == "" {
			return fmt.Errorf("managed Runtime revision snapshot path is required")
		}
		if !filepath.IsAbs(r.SnapshotPath) || filepath.Clean(r.SnapshotPath) != r.SnapshotPath {
			return fmt.Errorf("managed Runtime revision snapshot path must be canonical and absolute")
		}
	} else if r.SnapshotPath != "" {
		return fmt.Errorf("built-in Runtime cannot expose a managed snapshot path")
	}
	if r.CreatedAt.IsZero() || r.CreatedAt.Location() != time.UTC {
		return fmt.Errorf("Runtime revision creation time must be non-zero UTC")
	}
	return nil
}

// RuntimeManifest is the authoritative installation-wide Runtime record.
type RuntimeManifest struct {
	SchemaVersion int               `json:"schema_version"`
	ID            string            `json:"id"`
	RuntimeRef    string            `json:"runtime_ref,omitempty"`
	Name          string            `json:"name"`
	Kind          RuntimeKind       `json:"kind"`
	SourcePath    string            `json:"source_path,omitempty"`
	Revisions     []RuntimeRevision `json:"revisions"`
}

func (m RuntimeManifest) Validate() error {
	if m.SchemaVersion != RuntimeSchemaVersion {
		return fmt.Errorf("Runtime schema version must be %d", RuntimeSchemaVersion)
	}
	if err := m.Kind.Validate(); err != nil {
		return err
	}
	if err := ValidateName(m.Name); err != nil {
		return fmt.Errorf("Runtime name: %w", err)
	}
	if m.Kind == RuntimeKindBuiltin {
		if m.ID != StandardRuntimeID || m.Name != StandardRuntimeName || m.SourcePath != "" {
			return fmt.Errorf("built-in Runtime identity is invalid")
		}
	} else {
		if err := ValidateRuntimeID(m.ID); err != nil {
			return err
		}
		if m.SourcePath == "" {
			return fmt.Errorf("managed Runtime source path is required")
		}
		if !filepath.IsAbs(m.SourcePath) || filepath.Clean(m.SourcePath) != m.SourcePath {
			return fmt.Errorf("managed Runtime source path must be canonical and absolute")
		}
	}
	if m.RuntimeRef != "" && m.RuntimeRef != RuntimeRef(m.ID) {
		return fmt.Errorf("Runtime reference does not match Runtime ID")
	}
	if m.Revisions == nil {
		return fmt.Errorf("Runtime revisions must be present")
	}
	seen := make(map[string]struct{}, len(m.Revisions))
	for index, revision := range m.Revisions {
		if err := revision.Validate(m.Kind); err != nil {
			return err
		}
		if revision.Ordinal != index+1 {
			return fmt.Errorf("Runtime revision ordinals must be contiguous")
		}
		if _, exists := seen[revision.Revision]; exists {
			return fmt.Errorf("Runtime revision digest is duplicated")
		}
		if revision.RuntimeRef != "" && revision.RuntimeRef != RuntimeRef(m.ID) {
			return fmt.Errorf("Runtime revision owner reference is invalid")
		}
		if revision.RevisionRef != "" && revision.RevisionRef != RuntimeRevisionRef(m.ID, revision.Revision) {
			return fmt.Errorf("Runtime revision reference is invalid")
		}
		seen[revision.Revision] = struct{}{}
	}
	if m.Kind == RuntimeKindBuiltin && len(m.Revisions) != 1 {
		return fmt.Errorf("built-in Runtime must contain exactly one revision")
	}
	return nil
}

func (m RuntimeManifest) Head() (RuntimeRevision, bool) {
	if len(m.Revisions) == 0 {
		return RuntimeRevision{}, false
	}
	return m.Revisions[len(m.Revisions)-1], true
}

// RuntimeBinding is the exact authority stored by one Context. Name and
// ordinal are review metadata; ID plus revision are persistent identity.
type RuntimeBinding struct {
	RuntimeID string `json:"runtime_id"`
	Name      string `json:"name"`
	Revision  string `json:"revision"`
	Ordinal   int    `json:"ordinal"`
	Image     string `json:"image"`
}

// Selection returns the exact human selection for this stable binding without
// exposing its identity or immutable revision.
func (b RuntimeBinding) Selection() (string, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	selection := fmt.Sprintf("%s@%d", b.Name, b.Ordinal)
	if _, _, err := ParseRuntimeSelection(selection); err != nil {
		return "", err
	}
	return selection, nil
}

// ParseRuntimeSelection parses the human review syntax. The returned ordinal
// is presentation input only; infrastructure resolves it to stable ID+digest
// before a Context manifest is committed.
func ParseRuntimeSelection(value string) (string, int, error) {
	if value == "" || value == StandardRuntimeName {
		return StandardRuntimeName, 1, nil
	}
	index := strings.LastIndexByte(value, '@')
	if index < 1 || index == len(value)-1 {
		return "", 0, fmt.Errorf("Runtime selection must be standard or name@ordinal")
	}
	name := value[:index]
	if err := ValidateName(name); err != nil {
		return "", 0, fmt.Errorf("Runtime selection name: %w", err)
	}
	ordinal, err := strconv.Atoi(value[index+1:])
	if err != nil || ordinal < 1 {
		return "", 0, fmt.Errorf("Runtime selection ordinal must be positive")
	}
	return name, ordinal, nil
}

func (b RuntimeBinding) Validate() error {
	if b.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(b.RuntimeID); err != nil {
			return err
		}
	}
	if err := ValidateName(b.Name); err != nil {
		return fmt.Errorf("Runtime binding name: %w", err)
	}
	if b.Ordinal < 1 {
		return fmt.Errorf("Runtime binding ordinal must be positive")
	}
	if err := ValidateDigest(b.Revision); err != nil {
		return fmt.Errorf("Runtime binding revision: %w", err)
	}
	if err := ValidateImageSelector(b.Image); err != nil {
		return fmt.Errorf("Runtime binding image: %w", err)
	}
	if b.RuntimeID == StandardRuntimeID && b.Name != StandardRuntimeName {
		return fmt.Errorf("standard Runtime binding name is invalid")
	}
	return nil
}

func (m RuntimeManifest) Binding(ordinal int) (RuntimeBinding, error) {
	if err := m.Validate(); err != nil {
		return RuntimeBinding{}, err
	}
	if ordinal < 1 || ordinal > len(m.Revisions) {
		return RuntimeBinding{}, ErrRuntimeNotReady
	}
	revision := m.Revisions[ordinal-1]
	return RuntimeBinding{RuntimeID: m.ID, Name: m.Name, Revision: revision.Revision, Ordinal: revision.Ordinal, Image: revision.Image}, nil
}

type RuntimeSummary struct {
	ID          string      `json:"id"`
	RuntimeRef  string      `json:"runtime_ref"`
	Name        string      `json:"name"`
	Kind        RuntimeKind `json:"kind"`
	Ready       bool        `json:"ready"`
	Head        int         `json:"head,omitempty"`
	Revision    string      `json:"revision,omitempty"`
	RevisionRef string      `json:"revision_ref,omitempty"`
	SourcePath  string      `json:"source_path,omitempty"`
}

func RuntimeSummaryFrom(manifest RuntimeManifest) RuntimeSummary {
	summary := RuntimeSummary{ID: manifest.ID, RuntimeRef: RuntimeRef(manifest.ID), Name: manifest.Name, Kind: manifest.Kind, SourcePath: manifest.SourcePath}
	if head, ok := manifest.Head(); ok {
		summary.Ready, summary.Head, summary.Revision = true, head.Ordinal, head.Revision
	}
	return summary
}

func RuntimeRef(runtimeID string) string { return runtimeID }

func ValidateRuntimeRef(reference string) error {
	if reference == StandardRuntimeID {
		return nil
	}
	return ValidateRuntimeID(reference)
}

func (s RuntimeSummary) Validate() error {
	if err := s.Kind.Validate(); err != nil {
		return err
	}
	if err := ValidateName(s.Name); err != nil {
		return err
	}
	if s.Kind == RuntimeKindBuiltin {
		if s.ID != StandardRuntimeID || s.Name != StandardRuntimeName || s.SourcePath != "" || !s.Ready || s.Head != 1 {
			return fmt.Errorf("built-in Runtime summary is invalid")
		}
	} else {
		if err := ValidateRuntimeID(s.ID); err != nil {
			return err
		}
		if !filepath.IsAbs(s.SourcePath) || filepath.Clean(s.SourcePath) != s.SourcePath {
			return fmt.Errorf("managed Runtime summary source path is invalid")
		}
	}
	if s.RuntimeRef != s.ID {
		return fmt.Errorf("Runtime reference does not match Runtime ID")
	}
	if s.Ready != (s.Head > 0 && s.Revision != "") {
		return fmt.Errorf("Runtime summary ready state is inconsistent")
	}
	if s.Revision != "" {
		if err := ValidateDigest(s.Revision); err != nil {
			return err
		}
		if s.RevisionRef != "" && s.RevisionRef != RuntimeRevisionRef(s.ID, s.Revision) {
			return fmt.Errorf("Runtime revision reference is invalid")
		}
	} else if s.RevisionRef != "" {
		return fmt.Errorf("draft Runtime cannot expose a revision reference")
	}
	return nil
}

func RuntimeRevisionRef(runtimeID, revision string) string {
	return runtimeID + "/" + revision
}

// ParseRuntimeRevisionRef validates the opaque subordinate authority without
// accepting presentation names, ordinals, Docker selectors, or paths.
func ParseRuntimeRevisionRef(reference string) (string, string, error) {
	separator := strings.LastIndexByte(reference, '/')
	if separator <= 0 || separator == len(reference)-1 {
		return "", "", fmt.Errorf("Runtime revision reference is invalid")
	}
	runtimeID, revision := reference[:separator], reference[separator+1:]
	if ValidateRuntimeID(runtimeID) != nil || ValidateDigest(revision) != nil || reference != RuntimeRevisionRef(runtimeID, revision) {
		return "", "", fmt.Errorf("Runtime revision reference is invalid")
	}
	return runtimeID, revision, nil
}

// RuntimeReportWithReferences adds public opaque selections without changing
// the persisted Runtime authority record. Callers still resolve supplied
// references by deriving and comparing bounded candidates exactly.
// Revision references remain absent until their exact restore consumer joins
// the Catalog, so every published reference graph is complete at each commit.
func RuntimeReportWithReferences(report RuntimeReport) (RuntimeReport, error) {
	if err := report.Validate(); err != nil {
		return RuntimeReport{}, err
	}
	report.Runtime.RuntimeRef = RuntimeRef(report.Runtime.ID)
	for index := range report.Runtime.Revisions {
		report.Runtime.Revisions[index].RuntimeRef = report.Runtime.RuntimeRef
		report.Runtime.Revisions[index].RevisionRef = ""
	}
	if err := report.Validate(); err != nil {
		return RuntimeReport{}, err
	}
	return report, nil
}

// RuntimeReportWithRevisionReferences is used only by the atomic Catalog
// closure that also registers the exact runtime-revision restore consumer.
func RuntimeReportWithRevisionReferences(report RuntimeReport) (RuntimeReport, error) {
	report, err := RuntimeReportWithReferences(report)
	if err != nil {
		return RuntimeReport{}, err
	}
	if report.Runtime.Kind != RuntimeKindManaged {
		return report, nil
	}
	for index := range report.Runtime.Revisions {
		report.Runtime.Revisions[index].RevisionRef = RuntimeRevisionRef(report.Runtime.ID, report.Runtime.Revisions[index].Revision)
	}
	if err := report.Validate(); err != nil {
		return RuntimeReport{}, err
	}
	return report, nil
}

type RuntimeProtectionReason string

const (
	RuntimeProtectedByManifestCurrent   RuntimeProtectionReason = "manifest_current"
	RuntimeProtectedByManifestRetained  RuntimeProtectionReason = "manifest_retained"
	RuntimeProtectedByWorkspaceApplied  RuntimeProtectionReason = "workspace_applied"
	RuntimeProtectedByWorkspacePending  RuntimeProtectionReason = "workspace_pending"
	RuntimeProtectedByWorkspaceObserved RuntimeProtectionReason = "workspace_observed"
)

type RuntimeProtection struct {
	RuntimeID           string                  `json:"runtime_id"`
	RuntimeRevision     string                  `json:"runtime_revision"`
	Reason              RuntimeProtectionReason `json:"reason"`
	WorkspaceManifestID string                  `json:"workspace_manifest_id,omitempty"`
	ManifestRevision    string                  `json:"manifest_revision,omitempty"`
	WorkspaceID         string                  `json:"workspace_id,omitempty"`
}

func (p RuntimeProtection) Validate() error {
	if err := ValidateRuntimeID(p.RuntimeID); err != nil {
		return err
	}
	if err := ValidateDigest(p.RuntimeRevision); err != nil {
		return err
	}
	if p.ManifestRevision != "" {
		if err := ValidateDigest(p.ManifestRevision); err != nil {
			return fmt.Errorf("Runtime protection Manifest revision: %w", err)
		}
	}
	switch p.Reason {
	case RuntimeProtectedByManifestCurrent, RuntimeProtectedByManifestRetained:
		if ValidateWorkspaceManifestID(p.WorkspaceManifestID) != nil || p.ManifestRevision == "" || p.WorkspaceID != "" {
			return fmt.Errorf("Manifest Runtime protection owner is invalid")
		}
	case RuntimeProtectedByWorkspaceApplied, RuntimeProtectedByWorkspacePending, RuntimeProtectedByWorkspaceObserved:
		if ValidateWorkspaceManifestID(p.WorkspaceManifestID) != nil || p.ManifestRevision == "" || ValidateWorkspaceID(p.WorkspaceID) != nil {
			return fmt.Errorf("Workspace Runtime protection owner is invalid")
		}
	default:
		return fmt.Errorf("Runtime protection reason is invalid")
	}
	return nil
}

// RuntimeProtectionInventoryFaultReason classifies why a complete protection
// graph could not be observed. Destructive consumers must fail closed on every
// value; these are not protection reasons for a successfully observed edge.
type RuntimeProtectionInventoryFaultReason string

const (
	RuntimeProtectionInventoryIncomplete          RuntimeProtectionInventoryFaultReason = "incomplete_state"
	RuntimeProtectionInventoryMigrationUnverified RuntimeProtectionInventoryFaultReason = "migration_unverified"
	RuntimeProtectionInventoryObservationUnknown  RuntimeProtectionInventoryFaultReason = "unknown_observation"
)

// RuntimeProtectionInventoryError preserves one bounded fail-closed reason
// without exposing filesystem or Docker diagnostics above infrastructure.
type RuntimeProtectionInventoryError struct {
	Reason RuntimeProtectionInventoryFaultReason
}

func (e RuntimeProtectionInventoryError) Error() string {
	switch e.Reason {
	case RuntimeProtectionInventoryIncomplete:
		return "Runtime protection inventory contains incomplete state"
	case RuntimeProtectionInventoryMigrationUnverified:
		return "Runtime protection inventory contains migration-unverified state"
	case RuntimeProtectionInventoryObservationUnknown:
		return "Runtime protection inventory observation is unknown"
	default:
		return "Runtime protection inventory is unavailable"
	}
}

// RuntimeProtectionInventory is the complete trusted-host graph consumed by
// future Runtime retirement logic. It contains no inferred last-used value.
type RuntimeProtectionInventory struct {
	Complete bool                `json:"complete"`
	Items    []RuntimeProtection `json:"items"`
}

func (i RuntimeProtectionInventory) Validate() error {
	if !i.Complete || i.Items == nil {
		return fmt.Errorf("Runtime protection inventory is incomplete")
	}
	seen := make(map[string]struct{}, len(i.Items))
	for _, item := range i.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		identity := item.RuntimeID + "\x00" + item.RuntimeRevision + "\x00" + string(item.Reason) + "\x00" +
			item.WorkspaceManifestID + "\x00" + item.ManifestRevision + "\x00" + item.WorkspaceID
		if _, exists := seen[identity]; exists {
			return fmt.Errorf("Runtime protection inventory contains duplicate evidence")
		}
		seen[identity] = struct{}{}
	}
	return nil
}

type RuntimeListResult struct {
	Task  string           `json:"task"`
	Items []RuntimeSummary `json:"items"`
}

func (r RuntimeListResult) Validate() error {
	if r.Task != TaskRuntimeList || r.Items == nil {
		return fmt.Errorf("Runtime list is invalid")
	}
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type RuntimeReport struct {
	Task     string          `json:"task"`
	Runtime  RuntimeManifest `json:"runtime"`
	Created  bool            `json:"created,omitempty"`
	Built    bool            `json:"built,omitempty"`
	NoChange bool            `json:"no_change,omitempty"`
}

func (r RuntimeReport) Validate() error {
	switch r.Task {
	case TaskRuntimeShow, TaskRuntimeCreate, TaskRuntimeBuildV1, TaskRuntimeHistory:
	default:
		return fmt.Errorf("Runtime report task is invalid")
	}
	if err := r.Runtime.Validate(); err != nil {
		return err
	}
	if r.Created && r.Task != TaskRuntimeCreate {
		return fmt.Errorf("Runtime created state is invalid")
	}
	if (r.Built || r.NoChange) && r.Task != TaskRuntimeBuildV1 {
		return fmt.Errorf("Runtime build state is invalid")
	}
	if r.Built && r.NoChange {
		return fmt.Errorf("Runtime build cannot be new and unchanged")
	}
	if (r.Built || r.NoChange) && len(r.Runtime.Revisions) == 0 {
		return fmt.Errorf("Runtime build result requires a successful revision")
	}
	return nil
}

// NewRuntimeID issues a UUIDv7 Runtime identity from host-owned clock and entropy.
func NewRuntimeID(now time.Time, source io.Reader) (string, error) {
	id, err := NewWorkspaceManifestID(now, source)
	if err != nil {
		return "", fmt.Errorf("create Runtime ID: %w", err)
	}
	return id, nil
}

func ValidateRuntimeID(id string) error {
	if !contextIDPattern.MatchString(id) {
		return fmt.Errorf("Runtime ID is invalid")
	}
	return nil
}
