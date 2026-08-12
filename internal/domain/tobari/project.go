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
	ProjectStateSchemaVersion = 1
	DefaultProfile            = "default"

	TaskEnter       = "tobari.enter"
	TaskStatus      = "tobari.status"
	TaskDelete      = "tobari.delete"
	TaskProjectList = "tobari.project-list"

	CurrentDirectoryTargetKind = "current-directory-tobari"
	CurrentDirectoryTargetID   = "current-directory"
)

// ErrProjectExists means a caller requested creation at a root that became
// indexed after its read-only selection snapshot was taken.
var ErrProjectExists = errors.New("a project already exists at the requested root")

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

// ProjectStatus is the CWD-scoped lifecycle result. Exists is the only
// user-facing logical lifecycle bit; Runtime is diagnostic detail.
type ProjectStatus struct {
	Task         string                  `json:"task"`
	ContextState ContextObservationState `json:"context_state"`
	Exists       bool                    `json:"exists"`
	Root         string                  `json:"root,omitempty"`
	ID           string                  `json:"id,omitempty"`
	Home         string                  `json:"home,omitempty"`
	ContextID    string                  `json:"context_id,omitempty"`
	ContextName  string                  `json:"context,omitempty"`
	Runtime      RuntimeDiagnostic       `json:"runtime"`
	Attachment   AttachmentObservation   `json:"attachment"`
}

func (s ProjectStatus) Validate() error {
	if s.Task != TaskStatus {
		return fmt.Errorf("project status task identity is invalid")
	}
	if err := s.Runtime.Validate(); err != nil {
		return err
	}
	if err := s.Attachment.Validate(s.Exists); err != nil {
		return err
	}
	if err := s.ContextState.Validate(); err != nil {
		return err
	}
	if s.ContextState == ContextObservationSyntheticDefault {
		if s.Exists || s.ContextID != "" || s.ContextName != DefaultContextName {
			return fmt.Errorf("non-persisted project status claims persisted authority")
		}
	} else {
		if err := ValidateContextID(s.ContextID); err != nil {
			return err
		}
	}
	if err := ValidateName(s.ContextName); err != nil {
		return fmt.Errorf("project Context name: %w", err)
	}
	if !s.Exists {
		if s.Root != "" || s.ID != "" || s.Home != "" || s.Runtime != RuntimeDiagnosticUnknown {
			return fmt.Errorf("not-existing project status contains Workspace identity or runtime state")
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
	Root        string            `json:"root"`
	ID          string            `json:"id"`
	Home        string            `json:"home"`
	ContextID   string            `json:"context_id"`
	ContextName string            `json:"context"`
	Runtime     RuntimeDiagnostic `json:"runtime"`
}

func (i ProjectListItem) Validate() error {
	if err := ValidateCanonicalRoot(i.Root); err != nil {
		return err
	}
	if err := ValidateProjectID(i.ID); err != nil {
		return err
	}
	if err := ValidateContextID(i.ContextID); err != nil {
		return err
	}
	if err := ValidateName(i.ContextName); err != nil {
		return fmt.Errorf("project Context name: %w", err)
	}
	if i.Home == "" || !filepath.IsAbs(i.Home) || filepath.Clean(i.Home) != i.Home {
		return fmt.Errorf("project home is invalid")
	}
	return i.Runtime.Validate()
}

// ProjectListResult preserves the complete local logical-state observation,
// including a known empty result.
type ProjectListResult struct {
	Task string `json:"task"`
	// CurrentID identifies the nearest Workspace selected by the caller's
	// canonical current directory. It is presentation metadata and is not
	// part of the machine-readable list envelope.
	CurrentID string            `json:"-"`
	Items     []ProjectListItem `json:"items"`
}

func (r ProjectListResult) Validate() error {
	if r.Task != TaskProjectList || r.Items == nil {
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
		binding := item.Root + "\x00" + item.ContextID
		if seenBindings[binding] {
			return fmt.Errorf("project list root and Context bindings must be unique")
		}
		seenIDs[item.ID] = true
		seenBindings[binding] = true
	}
	if r.CurrentID != "" {
		if err := ValidateProjectID(r.CurrentID); err != nil {
			return fmt.Errorf("project list current ID is invalid: %w", err)
		}
		if !seenIDs[r.CurrentID] {
			return fmt.Errorf("project list current ID is not present in items")
		}
	}
	return nil
}

// ProjectDeleteResult records the exact logical target whose state was
// confirmed deleted. It is separate from ProjectStatus because deletion is a
// completed mutation, not an observation that the target still exists.
type ProjectDeleteResult struct {
	Task        string `json:"task"`
	Deleted     bool   `json:"deleted"`
	Root        string `json:"root"`
	ID          string `json:"id"`
	Home        string `json:"home"`
	ContextID   string `json:"context_id"`
	ContextName string `json:"context"`
}

func (r ProjectDeleteResult) Validate() error {
	if r.Task != TaskDelete || !r.Deleted {
		return fmt.Errorf("project delete result is incomplete")
	}
	if err := ValidateCanonicalRoot(r.Root); err != nil {
		return err
	}
	if err := ValidateProjectID(r.ID); err != nil {
		return err
	}
	if err := ValidateContextID(r.ContextID); err != nil {
		return err
	}
	if err := ValidateName(r.ContextName); err != nil {
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
	SchemaVersion int    `json:"schema_version"`
	Root          string `json:"root"`
	InstanceID    string `json:"instance_id"`
	ContextID     string `json:"context_id"`
	ContextName   string `json:"context"`
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
	if err := ValidateProjectID(r.InstanceID); err != nil {
		return err
	}
	if err := ValidateContextID(r.ContextID); err != nil {
		return err
	}
	return ValidateName(r.ContextName)
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
		binding := index.Root + "\x00" + index.ContextID
		if _, exists := seenBindings[binding]; exists {
			return fmt.Errorf("root indexes must contain one Workspace per root and Context")
		}
		if _, exists := seenIDs[index.InstanceID]; exists {
			return fmt.Errorf("root indexes must contain unique Workspace IDs")
		}
		seenBindings[binding] = struct{}{}
		seenIDs[index.InstanceID] = struct{}{}
	}
	return nil
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
	ContextID     string         `json:"context_id"`
	ContextName   string         `json:"context"`
	Profile       string         `json:"profile"`
	Image         string         `json:"image"`
	Runtime       ProjectRuntime `json:"runtime"`
	// Incomplete is an in-memory cleanup-only marker for a root index whose
	// instance record is missing. It is never persisted or used to rebuild a
	// runtime with guessed mutable fields.
	Incomplete bool `json:"-"`
}

// ProjectSelectionCandidate is the presentation-independent identity and
// runtime state of one Workspace that contains a canonical current directory.
// ID is an internal binding value; it is never a routine user input.
type ProjectSelectionCandidate struct {
	ID          string
	Root        string
	ContextID   string
	ContextName string
	Runtime     RuntimeDiagnostic
}

func (c ProjectSelectionCandidate) Validate(cwd string) error {
	if err := ValidateCanonicalRoot(cwd); err != nil {
		return err
	}
	if err := ValidateProjectID(c.ID); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(c.Root); err != nil {
		return err
	}
	if err := ValidateContextID(c.ContextID); err != nil {
		return err
	}
	if err := ValidateName(c.ContextName); err != nil {
		return err
	}
	if !containsRoot(c.Root, cwd) {
		return fmt.Errorf("selection candidate root does not contain the current directory")
	}
	return c.Runtime.Validate()
}

// ProjectSelection is a complete read-only snapshot used to resolve an
// ambiguous current-directory entry. Candidates are ordered nearest-first.
// CanCreate is false when the current directory is already an indexed root.
type ProjectSelection struct {
	CWD        string                      `json:"-"`
	Candidates []ProjectSelectionCandidate `json:"-"`
	CanCreate  bool                        `json:"-"`
}

func (s ProjectSelection) Validate() error {
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
func (s ProjectSelection) RequiresChoice() bool {
	return len(s.Candidates) > 0 && s.Candidates[0].Root != s.CWD
}

func (s ProjectSelection) Candidate(id string) (ProjectSelectionCandidate, bool) {
	for _, candidate := range s.Candidates {
		if candidate.ID == id {
			return candidate, true
		}
	}
	return ProjectSelectionCandidate{}, false
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

func (s ProjectSelection) ValidateChoice(choice ProjectSelectionChoice) error {
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
	if err := ValidateContextID(p.ContextID); err != nil {
		return err
	}
	if err := ValidateName(p.ContextName); err != nil {
		return fmt.Errorf("project Context name: %w", err)
	}
	if p.Profile != DefaultProfile {
		return fmt.Errorf("profile is invalid")
	}
	if err := ValidateImageSelector(p.Image); err != nil {
		return err
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

// ProjectInstanceRequest declares the complete durable identity and runtime
// selection required to create a logical project before Docker resources
// exist.
type ProjectInstanceRequest struct {
	Root        string
	ContextID   string
	ContextName string
	Image       string
}

// NewProjectInstance creates the durable logical state before any Docker
// resource exists.
func NewProjectInstance(now time.Time, source io.Reader, request ProjectInstanceRequest) (ProjectInstance, error) {
	if err := ValidateCanonicalRoot(request.Root); err != nil {
		return ProjectInstance{}, err
	}
	if err := ValidateImageSelector(request.Image); err != nil {
		return ProjectInstance{}, err
	}
	if err := ValidateContextID(request.ContextID); err != nil {
		return ProjectInstance{}, err
	}
	if err := ValidateName(request.ContextName); err != nil {
		return ProjectInstance{}, err
	}
	id, err := NewProjectID(now, source)
	if err != nil {
		return ProjectInstance{}, err
	}
	instance := ProjectInstance{
		SchemaVersion: ProjectStateSchemaVersion,
		ID:            id,
		Root:          request.Root,
		ContextID:     request.ContextID,
		ContextName:   request.ContextName,
		Profile:       DefaultProfile,
		Image:         request.Image,
		Runtime:       ProjectRuntime{},
	}
	if err := instance.Validate(); err != nil {
		return ProjectInstance{}, err
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
	if err := ValidateContextID(contextID); err != nil {
		return nil, err
	}
	result := make([]RootIndex, 0, len(indexes))
	for _, index := range indexes {
		if index.ContextID == contextID {
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
