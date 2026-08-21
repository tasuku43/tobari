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
	RuntimeSchemaVersion = 1
	StandardRuntimeID    = "builtin/standard"
	StandardRuntimeName  = "standard"

	TaskRuntimeList    = "runtime.list"
	TaskRuntimeShow    = "runtime.show"
	TaskRuntimeCreate  = "runtime.create"
	TaskRuntimeBuildV1 = "runtime.build"
	TaskRuntimeHistory = "runtime.history"

	RuntimeCatalogTargetKind = "runtimes"
	RuntimeCatalogTargetID   = "runtime-catalog"
)

var (
	ErrRuntimeExists   = errors.New("Runtime already exists")
	ErrRuntimeNotFound = errors.New("Runtime does not exist")
	ErrRuntimeNotReady = errors.New("Runtime revision is not ready")
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

// RuntimeRevision is one immutable successful semantic build.
type RuntimeRevision struct {
	Ordinal      int       `json:"ordinal"`
	Revision     string    `json:"revision"`
	Image        string    `json:"image"`
	ImageDigest  string    `json:"image_digest"`
	CreatedAt    time.Time `json:"created_at"`
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
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Kind       RuntimeKind `json:"kind"`
	Ready      bool        `json:"ready"`
	Head       int         `json:"head,omitempty"`
	Revision   string      `json:"revision,omitempty"`
	SourcePath string      `json:"source_path,omitempty"`
}

func RuntimeSummaryFrom(manifest RuntimeManifest) RuntimeSummary {
	summary := RuntimeSummary{ID: manifest.ID, Name: manifest.Name, Kind: manifest.Kind, SourcePath: manifest.SourcePath}
	if head, ok := manifest.Head(); ok {
		summary.Ready, summary.Head, summary.Revision = true, head.Ordinal, head.Revision
	}
	return summary
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
	if s.Ready != (s.Head > 0 && s.Revision != "") {
		return fmt.Errorf("Runtime summary ready state is inconsistent")
	}
	if s.Revision != "" {
		if err := ValidateDigest(s.Revision); err != nil {
			return err
		}
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
	id, err := NewContextID(now, source)
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
