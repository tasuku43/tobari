package tobari

import (
	"crypto/rand"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	ProjectStateSchemaVersion = 1
	DefaultProfile            = "default"

	TaskEnter       = "tobari.enter"
	TaskStatus      = "tobari.status"
	TaskDelete      = "tobari.delete"
	TaskProjectList = "tobari.project-list"

	CurrentDirectoryTargetKind = "current-directory-tobari"
	CurrentDirectoryTargetID   = "current-directory"
)

// RuntimeDiagnostic describes recoverable Docker health. It is deliberately
// separate from logical Tobari existence: a missing container never changes a
// valid ProjectInstance into not-exists.
type RuntimeDiagnostic string

const (
	RuntimeDiagnosticUnknown     RuntimeDiagnostic = "unknown"
	RuntimeDiagnosticReady       RuntimeDiagnostic = "ready"
	RuntimeDiagnosticMissing     RuntimeDiagnostic = "missing"
	RuntimeDiagnosticDegraded    RuntimeDiagnostic = "degraded"
	RuntimeDiagnosticUnreachable RuntimeDiagnostic = "unreachable"
)

func (d RuntimeDiagnostic) Validate() error {
	switch d {
	case RuntimeDiagnosticUnknown, RuntimeDiagnosticReady, RuntimeDiagnosticMissing,
		RuntimeDiagnosticDegraded, RuntimeDiagnosticUnreachable:
		return nil
	default:
		return fmt.Errorf("runtime diagnostic is invalid: %q", d)
	}
}

// ProjectStatus is the CWD-scoped lifecycle result. Exists is the only
// user-facing logical lifecycle bit; Runtime is diagnostic detail.
type ProjectStatus struct {
	Task    string            `json:"task"`
	Exists  bool              `json:"exists"`
	Root    string            `json:"root,omitempty"`
	ID      string            `json:"id,omitempty"`
	Home    string            `json:"home,omitempty"`
	Runtime RuntimeDiagnostic `json:"runtime"`
}

func (s ProjectStatus) Validate() error {
	if s.Task != TaskStatus {
		return fmt.Errorf("project status task identity is invalid")
	}
	if err := s.Runtime.Validate(); err != nil {
		return err
	}
	if !s.Exists {
		if s.Root != "" || s.ID != "" || s.Home != "" {
			return fmt.Errorf("not-existing project status contains identity")
		}
		return nil
	}
	if err := ValidateCanonicalRoot(s.Root); err != nil {
		return err
	}
	if err := ValidateProjectID(s.ID); err != nil {
		return err
	}
	if s.Home == "" || filepath.IsAbs(s.Home) == false || filepath.Clean(s.Home) != s.Home {
		return fmt.Errorf("project home is invalid")
	}
	return nil
}

// ProjectListItem is one local logical Tobari with runtime diagnostics.
type ProjectListItem struct {
	Root    string            `json:"root"`
	ID      string            `json:"id"`
	Home    string            `json:"home"`
	Runtime RuntimeDiagnostic `json:"runtime"`
}

func (i ProjectListItem) Validate() error {
	if err := ValidateCanonicalRoot(i.Root); err != nil {
		return err
	}
	if err := ValidateProjectID(i.ID); err != nil {
		return err
	}
	if i.Home == "" || !filepath.IsAbs(i.Home) || filepath.Clean(i.Home) != i.Home {
		return fmt.Errorf("project home is invalid")
	}
	return i.Runtime.Validate()
}

// ProjectListResult preserves the complete local logical-state observation,
// including a known empty result.
type ProjectListResult struct {
	Task  string            `json:"task"`
	Items []ProjectListItem `json:"items"`
}

func (r ProjectListResult) Validate() error {
	if r.Task != TaskProjectList || r.Items == nil {
		return fmt.Errorf("project list task or scope is invalid")
	}
	seen := make(map[string]bool, len(r.Items))
	for _, item := range r.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if seen[item.ID] {
			return fmt.Errorf("project list IDs must be unique")
		}
		seen[item.ID] = true
	}
	return nil
}

var projectIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// RootIndex binds one canonical host root to its stable logical Tobari ID.
// It is stored independently so parent lookup does not need Docker access.
type RootIndex struct {
	SchemaVersion int    `json:"schema_version"`
	Root          string `json:"root"`
	InstanceID    string `json:"instance_id"`
}

// Validate rejects a malformed or ambiguous root index before it becomes a
// selection authority.
func (r RootIndex) Validate() error {
	if r.SchemaVersion != ProjectStateSchemaVersion {
		return fmt.Errorf("root index schema version must be %d", ProjectStateSchemaVersion)
	}
	if err := ValidateCanonicalRoot(r.Root); err != nil {
		return err
	}
	return ValidateProjectID(r.InstanceID)
}

// ProjectRuntime is recoverable diagnostic state. Empty values mean that the
// resource has not yet been created or its last observation is unknown.
type ProjectRuntime struct {
	ContainerID string `json:"container_id"`
	NetworkID   string `json:"network_id"`
}

func (r ProjectRuntime) Validate() error {
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

// ProjectInstance is the durable logical Tobari record. Its existence does not
// depend on a work container or network being present.
type ProjectInstance struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	Root          string         `json:"root"`
	Profile       string         `json:"profile"`
	Image         string         `json:"image"`
	Runtime       ProjectRuntime `json:"runtime"`
}

// Validate rejects invalid durable logical state before runtime reconciliation.
func (p ProjectInstance) Validate() error {
	if p.SchemaVersion != ProjectStateSchemaVersion {
		return fmt.Errorf("instance state schema version must be %d", ProjectStateSchemaVersion)
	}
	if err := ValidateProjectID(p.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(p.Root); err != nil {
		return err
	}
	if p.Profile != DefaultProfile {
		return fmt.Errorf("profile is invalid")
	}
	if err := ValidateImageSelector(p.Image); err != nil {
		return err
	}
	return p.Runtime.Validate()
}

// ValidateProjectID accepts only a canonical UUIDv7 logical identity.
func ValidateProjectID(id string) error {
	if !projectIDPattern.MatchString(id) {
		return fmt.Errorf("Tobari project ID is invalid")
	}
	return nil
}

// NewProjectID produces a UUIDv7 logical identity from the supplied clock and
// entropy source. The source exists so callers can make creation deterministic
// in tests without weakening production entropy.
func NewProjectID(now time.Time, source io.Reader) (string, error) {
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
	if err := ValidateProjectID(id); err != nil {
		return "", err
	}
	return id, nil
}

// NewProjectInstance creates the durable logical state before any Docker
// resource exists.
func NewProjectInstance(now time.Time, source io.Reader, root, image string) (ProjectInstance, error) {
	if err := ValidateCanonicalRoot(root); err != nil {
		return ProjectInstance{}, err
	}
	if err := ValidateImageSelector(image); err != nil {
		return ProjectInstance{}, err
	}
	id, err := NewProjectID(now, source)
	if err != nil {
		return ProjectInstance{}, err
	}
	instance := ProjectInstance{
		SchemaVersion: ProjectStateSchemaVersion,
		ID:            id,
		Root:          root,
		Profile:       DefaultProfile,
		Image:         image,
		Runtime:       ProjectRuntime{},
	}
	if err := instance.Validate(); err != nil {
		return ProjectInstance{}, err
	}
	return instance, nil
}

// NewProductionProjectInstance uses the system clock and cryptographic entropy.
func NewProductionProjectInstance(root, image string) (ProjectInstance, error) {
	return NewProjectInstance(time.Now().UTC(), rand.Reader, root, image)
}

// ValidateCanonicalRoot accepts exactly the form produced by infrastructure
// after absolute-path, clean, symlink, and directory checks have completed.
func ValidateCanonicalRoot(root string) error {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("root must be a canonical absolute path")
	}
	return nil
}

// NearestRoot returns the containing indexed root with the longest canonical
// path. Inputs have already been canonicalized by infrastructure.
func NearestRoot(cwd string, indexes []RootIndex) (RootIndex, bool, error) {
	if err := ValidateCanonicalRoot(cwd); err != nil {
		return RootIndex{}, false, err
	}
	var (
		nearest RootIndex
		found   bool
	)
	for _, index := range indexes {
		if err := index.Validate(); err != nil {
			return RootIndex{}, false, err
		}
		if !containsRoot(index.Root, cwd) {
			continue
		}
		if !found || len(index.Root) > len(nearest.Root) {
			nearest, found = index, true
		}
	}
	return nearest, found, nil
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

func unsafeStateRune(r rune) bool {
	return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
}

// ProjectResourceNames derives owned Docker resource names from a stable ID.
// Runtime identifiers never become the source of logical identity.
func ProjectResourceNames(id string) (container, network string, err error) {
	if err := ValidateProjectID(id); err != nil {
		return "", "", err
	}
	short := strings.ReplaceAll(id[:13], "-", "")
	return "tobari-" + short + "-work", "tobari-" + short + "-net", nil
}
