package tobari

import (
	"fmt"
	"sort"
)

const FinalDefaultPairObservationSchemaVersion = 1

const FinalDefaultPairSelectionSchemaVersion = 1

type FinalDefaultPairSelectionChoiceKind string

const (
	FinalDefaultPairSelectionUse    FinalDefaultPairSelectionChoiceKind = "use"
	FinalDefaultPairSelectionCreate FinalDefaultPairSelectionChoiceKind = "create"
)

// FinalDefaultPairSelectionChoice binds either one Context from the observed
// candidate snapshot or explicit creation at CanonicalCWD. ContextID remains
// internal selection authority and is never reconstructed from a path.
type FinalDefaultPairSelectionChoice struct {
	Kind        FinalDefaultPairSelectionChoiceKind
	ContextID   ContextID
	WorkspaceID WorkspaceID
}

// FinalDefaultPairCandidate is one Workspace whose Project root contains the
// invocation CWD. Snapshot owns the exact semantic authority
// used after selection; Context itself carries no location.
type FinalDefaultPairCandidate struct {
	Snapshot ContextAuthoritySnapshot
}

func (c FinalDefaultPairCandidate) Validate(cwd string) error {
	if err := c.Snapshot.Validate(); err != nil {
		return err
	}
	if c.Snapshot.Workspace == nil {
		return fmt.Errorf("default-pair candidate has no Workspace")
	}
	return ValidateRootContains(c.Snapshot.Workspace.ProjectRoot, cwd)
}

func (c FinalDefaultPairCandidate) Clone() FinalDefaultPairCandidate {
	return FinalDefaultPairCandidate{Snapshot: c.Snapshot.Clone()}
}

// FinalDefaultPairSelection is the complete read-only root-selection snapshot.
// Candidates contain CanonicalCWD and are ordered nearest-first. An exact
// candidate is reused directly; ancestors require an explicit use-or-create
// choice and a nested root is never inferred.
type FinalDefaultPairSelection struct {
	SchemaVersion        int
	CollectionPresent    bool
	CollectionGeneration uint64
	CollectionRevision   SemanticDigest
	CurrentContextID     *ContextID
	CanonicalCWD         string
	DefaultTemplate      *WorkspaceTemplate
	Candidates           []FinalDefaultPairCandidate
}

func NewFinalDefaultPairSelection(collection WorkspaceAuthorityCollection, present bool, cwd string) (FinalDefaultPairSelection, error) {
	if err := ValidateCanonicalRoot(cwd); err != nil {
		return FinalDefaultPairSelection{}, err
	}
	result := FinalDefaultPairSelection{SchemaVersion: FinalDefaultPairSelectionSchemaVersion, CollectionPresent: present, CanonicalCWD: cwd, Candidates: []FinalDefaultPairCandidate{}}
	if !present {
		if collection.SchemaVersion != 0 {
			return FinalDefaultPairSelection{}, fmt.Errorf("absent final authority carried collection content")
		}
		return result, result.Validate()
	}
	if err := collection.Validate(); err != nil {
		return FinalDefaultPairSelection{}, err
	}
	result.CollectionGeneration = collection.Generation
	result.CollectionRevision = collection.Revision
	if collection.CurrentContextID != nil {
		value := *collection.CurrentContextID
		result.CurrentContextID = &value
	}
	if collection.DefaultTemplateID != nil {
		for index := range collection.Templates {
			if collection.Templates[index].ID == *collection.DefaultTemplateID {
				value := collection.Templates[index].Clone()
				result.DefaultTemplate = &value
				break
			}
		}
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairSelection{}, err
	}
	byContext := make(map[ContextID]ContextAuthoritySnapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byContext[snapshot.Context.ID] = snapshot
	}
	for _, workspace := range collection.Workspaces {
		if !containsRoot(workspace.ProjectRoot, cwd) {
			continue
		}
		snapshot, exists := byContext[workspace.ContextID]
		if !exists {
			return FinalDefaultPairSelection{}, fmt.Errorf("Workspace Context snapshot is unavailable")
		}
		focused, err := snapshot.SelectWorkspace(workspace.ID)
		if err != nil {
			return FinalDefaultPairSelection{}, err
		}
		result.Candidates = append(result.Candidates, FinalDefaultPairCandidate{Snapshot: focused})
	}
	sort.Slice(result.Candidates, func(left, right int) bool {
		leftRoot := result.Candidates[left].Snapshot.Workspace.ProjectRoot
		rightRoot := result.Candidates[right].Snapshot.Workspace.ProjectRoot
		if len(leftRoot) != len(rightRoot) {
			return len(leftRoot) > len(rightRoot)
		}
		return leftRoot < rightRoot
	})
	return result, result.Validate()
}

func (s FinalDefaultPairSelection) Validate() error {
	if s.SchemaVersion != FinalDefaultPairSelectionSchemaVersion || ValidateCanonicalRoot(s.CanonicalCWD) != nil || s.Candidates == nil {
		return fmt.Errorf("final default-pair selection metadata is invalid")
	}
	if !s.CollectionPresent {
		if s.CollectionGeneration != 0 || s.CollectionRevision != "" || s.CurrentContextID != nil || s.DefaultTemplate != nil || len(s.Candidates) != 0 {
			return fmt.Errorf("absent final authority carries selection state")
		}
		return nil
	}
	if s.CollectionGeneration == 0 || s.CollectionRevision.Validate() != nil {
		return fmt.Errorf("final default-pair selection receipt is invalid")
	}
	if s.CurrentContextID != nil && s.CurrentContextID.Validate() != nil {
		return fmt.Errorf("final default-pair current Context is invalid")
	}
	if s.DefaultTemplate != nil {
		if err := s.DefaultTemplate.Validate(); err != nil {
			return err
		}
	}
	seen := make(map[WorkspaceID]bool, len(s.Candidates))
	previousRoot := ""
	for index, candidate := range s.Candidates {
		if err := candidate.Validate(s.CanonicalCWD); err != nil {
			return fmt.Errorf("default-pair candidate %d is invalid: %w", index, err)
		}
		if seen[candidate.Snapshot.Workspace.ID] {
			return fmt.Errorf("default-pair candidate Workspace IDs must be unique")
		}
		seen[candidate.Snapshot.Workspace.ID] = true
		root := candidate.Snapshot.Workspace.ProjectRoot
		if index > 0 && (len(previousRoot) < len(root) || (len(previousRoot) == len(root) && previousRoot > root)) {
			return fmt.Errorf("default-pair candidates must be ordered nearest-first")
		}
		previousRoot = root
	}
	return nil
}

func (s FinalDefaultPairSelection) Clone() FinalDefaultPairSelection {
	result := s
	if s.CurrentContextID != nil {
		value := *s.CurrentContextID
		result.CurrentContextID = &value
	}
	if s.DefaultTemplate != nil {
		value := s.DefaultTemplate.Clone()
		result.DefaultTemplate = &value
	}
	result.Candidates = make([]FinalDefaultPairCandidate, len(s.Candidates))
	for index := range s.Candidates {
		result.Candidates[index] = s.Candidates[index].Clone()
	}
	return result
}

func (s FinalDefaultPairSelection) SameReceipt(other FinalDefaultPairSelection) bool {
	return s.CollectionPresent == other.CollectionPresent && s.CollectionGeneration == other.CollectionGeneration && s.CollectionRevision == other.CollectionRevision && s.CanonicalCWD == other.CanonicalCWD
}

func (s FinalDefaultPairSelection) RequiresChoice() bool {
	if len(s.Candidates) == 0 {
		return false
	}
	nearest := s.Candidates[0].Snapshot.Workspace.ProjectRoot
	if nearest != s.CanonicalCWD {
		return true
	}
	if s.CurrentContextID != nil {
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Workspace.ProjectRoot == nearest && candidate.Snapshot.Context.ID == *s.CurrentContextID {
				return false
			}
		}
		return true
	}
	return len(s.Candidates) > 1 && s.Candidates[1].Snapshot.Workspace.ProjectRoot == nearest
}

func (s FinalDefaultPairSelection) AutomaticChoice() (FinalDefaultPairSelectionChoice, bool) {
	if s.RequiresChoice() {
		return FinalDefaultPairSelectionChoice{}, false
	}
	if len(s.Candidates) == 0 {
		return FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionCreate}, true
	}
	if s.CurrentContextID != nil {
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Workspace.ProjectRoot == s.CanonicalCWD && candidate.Snapshot.Context.ID == *s.CurrentContextID {
				return FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionUse, ContextID: candidate.Snapshot.Context.ID, WorkspaceID: candidate.Snapshot.Workspace.ID}, true
			}
		}
	}
	return FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionUse, ContextID: s.Candidates[0].Snapshot.Context.ID, WorkspaceID: s.Candidates[0].Snapshot.Workspace.ID}, true
}

func (s FinalDefaultPairSelection) ValidateChoice(choice FinalDefaultPairSelectionChoice) error {
	switch choice.Kind {
	case FinalDefaultPairSelectionCreate:
		if choice.ContextID != "" || choice.WorkspaceID != "" {
			return fmt.Errorf("create choice must not contain Context or Workspace identity")
		}
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Workspace.ProjectRoot == s.CanonicalCWD &&
				(s.CurrentContextID == nil || candidate.Snapshot.Context.ID == *s.CurrentContextID) {
				return fmt.Errorf("current directory already has a default Context")
			}
		}
		return nil
	case FinalDefaultPairSelectionUse:
		if choice.ContextID.Validate() != nil {
			return fmt.Errorf("use choice requires a valid Context ID")
		}
		matches := 0
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Context.ID == choice.ContextID && (choice.WorkspaceID == "" || candidate.Snapshot.Workspace.ID == choice.WorkspaceID) {
				matches++
			}
		}
		if choice.WorkspaceID != "" && choice.WorkspaceID.Validate() != nil {
			return fmt.Errorf("use choice has an invalid Workspace ID")
		}
		if matches != 1 {
			return fmt.Errorf("selected Context and Workspace are not unique in the snapshot")
		}
		return nil
	default:
		return fmt.Errorf("default-pair selection choice kind is invalid")
	}
}

func (s FinalDefaultPairSelection) Observation(choice FinalDefaultPairSelectionChoice) (FinalDefaultPairObservation, error) {
	if err := s.Validate(); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	if err := s.ValidateChoice(choice); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	root := s.CanonicalCWD
	var selected *ContextAuthoritySnapshot
	if choice.Kind == FinalDefaultPairSelectionUse {
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Context.ID == choice.ContextID && (choice.WorkspaceID == "" || candidate.Snapshot.Workspace.ID == choice.WorkspaceID) {
				value := candidate.Snapshot.Clone()
				selected = &value
				root = value.Workspace.ProjectRoot
				template := value.Template.Clone()
				s.DefaultTemplate = &template
				break
			}
		}
	}
	result := FinalDefaultPairObservation{
		SchemaVersion: FinalDefaultPairObservationSchemaVersion, CollectionPresent: s.CollectionPresent,
		CollectionGeneration: s.CollectionGeneration, CollectionRevision: s.CollectionRevision,
		ProjectRoot: root, Context: selected,
	}
	if s.DefaultTemplate != nil {
		value := s.DefaultTemplate.Clone()
		result.DefaultTemplate = &value
	}
	return result, result.Validate()
}

// FinalDefaultPairObservation is the complete command-local authority used by
// bare entry and status. The collection receipt is private correlation; the
// public task projects only final Template, Context, and Workspace facts.
type FinalDefaultPairObservation struct {
	SchemaVersion        int
	CollectionPresent    bool
	CollectionGeneration uint64
	CollectionRevision   SemanticDigest
	ProjectRoot          string
	DefaultTemplate      *WorkspaceTemplate
	Context              *ContextAuthoritySnapshot
}

func NewFinalDefaultPairObservation(collection WorkspaceAuthorityCollection, present bool, projectRoot string) (FinalDefaultPairObservation, error) {
	if err := ValidateCanonicalRoot(projectRoot); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	result := FinalDefaultPairObservation{SchemaVersion: FinalDefaultPairObservationSchemaVersion, CollectionPresent: present, ProjectRoot: projectRoot}
	if !present {
		if collection.SchemaVersion != 0 {
			return FinalDefaultPairObservation{}, fmt.Errorf("absent final authority carried collection content")
		}
		return result, result.Validate()
	}
	if err := collection.Validate(); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	result.CollectionGeneration = collection.Generation
	result.CollectionRevision = collection.Revision
	if collection.DefaultTemplateID != nil {
		for index := range collection.Templates {
			if collection.Templates[index].ID == *collection.DefaultTemplateID {
				value := collection.Templates[index].Clone()
				result.DefaultTemplate = &value
				break
			}
		}
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairObservation{}, err
	}
	exact := make([]ContextAuthoritySnapshot, 0, 1)
	for _, snapshot := range snapshots {
		focused, focusErr := snapshot.SelectWorkspaceAtRoot(projectRoot)
		if focusErr != nil {
			return FinalDefaultPairObservation{}, focusErr
		}
		if focused.Workspace != nil {
			exact = append(exact, focused)
		}
	}
	selected := -1
	if len(exact) == 1 {
		selected = 0
	} else if result.DefaultTemplate != nil {
		for index := range exact {
			if exact[index].Context.TemplateID != result.DefaultTemplate.ID {
				continue
			}
			if selected >= 0 {
				selected = -1
				break
			}
			selected = index
		}
	}
	if selected >= 0 {
		value := exact[selected].Clone()
		result.Context = &value
		template := value.Template.Clone()
		result.DefaultTemplate = &template
	}
	return result, result.Validate()
}

// NewFinalDefaultPairContextObservation binds an already-selected Context ID
// to one collection receipt. Unlike root selection, it may return a Context
// that does not have a Workspace yet; Context location is not inferred from
// the caller's current directory.
func NewFinalDefaultPairContextObservation(collection WorkspaceAuthorityCollection, present bool, projectRoot string, contextID ContextID) (FinalDefaultPairObservation, error) {
	if err := contextID.Validate(); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	if err := ValidateCanonicalRoot(projectRoot); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	if !present {
		return FinalDefaultPairObservation{}, ErrContextBindingNotFound
	}
	if err := collection.Validate(); err != nil {
		return FinalDefaultPairObservation{}, err
	}
	result := FinalDefaultPairObservation{
		SchemaVersion: FinalDefaultPairObservationSchemaVersion, CollectionPresent: true,
		CollectionGeneration: collection.Generation, CollectionRevision: collection.Revision,
		ProjectRoot: projectRoot,
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairObservation{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.ID != contextID {
			continue
		}
		focused, focusErr := snapshot.SelectWorkspaceAtRoot(projectRoot)
		if focusErr != nil {
			return FinalDefaultPairObservation{}, focusErr
		}
		template := snapshot.Template.Clone()
		result.DefaultTemplate = &template
		value := focused.Clone()
		result.Context = &value
		return result, result.Validate()
	}
	return FinalDefaultPairObservation{}, ErrContextBindingNotFound
}

func (o FinalDefaultPairObservation) Validate() error {
	if o.SchemaVersion != FinalDefaultPairObservationSchemaVersion || ValidateCanonicalRoot(o.ProjectRoot) != nil {
		return fmt.Errorf("final default-pair observation metadata is invalid")
	}
	if !o.CollectionPresent {
		if o.CollectionGeneration != 0 || o.CollectionRevision != "" || o.DefaultTemplate != nil || o.Context != nil {
			return fmt.Errorf("absent final default-pair observation carries authority")
		}
		return nil
	}
	if o.CollectionGeneration == 0 || o.CollectionRevision.Validate() != nil {
		return fmt.Errorf("final default-pair collection receipt is invalid")
	}
	if o.DefaultTemplate == nil {
		if o.Context != nil {
			return fmt.Errorf("default-pair Context exists without default Template")
		}
		return nil
	}
	if err := o.DefaultTemplate.Validate(); err != nil {
		return err
	}
	if o.Context == nil {
		return nil
	}
	if err := o.Context.Validate(); err != nil {
		return err
	}
	if o.Context.Context.TemplateID != o.DefaultTemplate.ID || o.Context.Template.ID != o.DefaultTemplate.ID || o.Context.Template.Current.Revision != o.DefaultTemplate.Current.Revision || o.Context.Workspace != nil && o.Context.Workspace.ProjectRoot != o.ProjectRoot {
		return fmt.Errorf("default-pair Context does not match the selected Template and Project")
	}
	return nil
}

func (o FinalDefaultPairObservation) Clone() FinalDefaultPairObservation {
	result := o
	if o.DefaultTemplate != nil {
		value := o.DefaultTemplate.Clone()
		result.DefaultTemplate = &value
	}
	if o.Context != nil {
		value := o.Context.Clone()
		result.Context = &value
	}
	return result
}

func (o FinalDefaultPairObservation) SameReceipt(other FinalDefaultPairObservation) bool {
	return o.CollectionPresent == other.CollectionPresent && o.CollectionGeneration == other.CollectionGeneration && o.CollectionRevision == other.CollectionRevision && o.ProjectRoot == other.ProjectRoot
}

type FinalDefaultPairPublication struct {
	Previous FinalDefaultPairObservation
	Current  FinalDefaultPairObservation
	Changed  bool
}

func (p FinalDefaultPairPublication) ValidateFor(projectRoot string, freshBody WorkspaceTemplateBody) error {
	if err := p.Previous.Validate(); err != nil {
		return fmt.Errorf("default-pair predecessor: %w", err)
	}
	if err := p.Current.Validate(); err != nil {
		return fmt.Errorf("default-pair current: %w", err)
	}
	if err := freshBody.Validate(); err != nil {
		return err
	}
	if p.Previous.ProjectRoot != projectRoot || p.Current.ProjectRoot != projectRoot || p.Current.DefaultTemplate == nil || p.Current.Context == nil || p.Current.Context.Context.TemplateID != p.Current.DefaultTemplate.ID || p.Current.Context.Workspace != nil && p.Current.Context.Workspace.ProjectRoot != projectRoot {
		return fmt.Errorf("default-pair publication does not establish the requested pair")
	}
	if !p.Previous.CollectionPresent {
		if p.Current.DefaultTemplate.Name != DefaultManifestName || p.Current.DefaultTemplate.Current.Generation != 1 || len(p.Current.DefaultTemplate.Retained) != 1 || p.Current.DefaultTemplate.Current.Body.Validate() != nil || p.Current.DefaultTemplate.Current.Revision != p.Current.DefaultTemplate.Retained[0].Revision {
			return fmt.Errorf("fresh default-pair publication did not create one generation-1 default Template")
		}
		want, err := NewWorkspaceTemplateRevision(p.Current.DefaultTemplate.ID, 1, freshBody)
		if err != nil || want.Revision != p.Current.DefaultTemplate.Current.Revision {
			return fmt.Errorf("fresh default-pair publication changed the reviewed Template body")
		}
	}
	if p.Previous.Context == nil && (p.Current.Context.Workspace != nil || p.Current.Context.ActiveTemplatePolicy != nil || p.Current.Context.ActivePolicyMemory != nil || p.Current.Context.ActivePolicyMemoryRef != nil || len(p.Current.Context.PolicyMemory.Rules) != 0 || p.Current.Context.PolicyMemory.Generation != 1) {
		return fmt.Errorf("default-pair initialization created lower-lifetime authority")
	}
	wantChanged := !p.Previous.SameReceipt(p.Current)
	if p.Changed != wantChanged {
		return fmt.Errorf("default-pair publication disposition is inconsistent")
	}
	if !p.Changed && (p.Previous.DefaultTemplate == nil || p.Previous.Context == nil || p.Previous.CollectionGeneration != p.Current.CollectionGeneration || p.Previous.CollectionRevision != p.Current.CollectionRevision) {
		return fmt.Errorf("unchanged default-pair publication is not an exact replay")
	}
	return nil
}
