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
	Kind      FinalDefaultPairSelectionChoiceKind
	ContextID ContextID
}

// FinalDefaultPairCandidate is one default-Template Workspace whose Project
// root contains the invocation CWD. Snapshot owns the exact semantic authority
// used after selection; Context itself carries no location.
type FinalDefaultPairCandidate struct {
	Snapshot ContextAuthoritySnapshot
}

func (c FinalDefaultPairCandidate) Validate(cwd string, templateID WorkspaceTemplateID) error {
	if err := c.Snapshot.Validate(); err != nil {
		return err
	}
	if c.Snapshot.Context.TemplateID != templateID || c.Snapshot.Template.ID != templateID || c.Snapshot.Workspace == nil {
		return fmt.Errorf("default-pair candidate belongs to another Template")
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
	if collection.DefaultTemplateID == nil {
		return result, result.Validate()
	}
	for index := range collection.Templates {
		if collection.Templates[index].ID == *collection.DefaultTemplateID {
			value := collection.Templates[index].Clone()
			result.DefaultTemplate = &value
			break
		}
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairSelection{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.TemplateID != *collection.DefaultTemplateID || snapshot.Workspace == nil || !containsRoot(snapshot.Workspace.ProjectRoot, cwd) {
			continue
		}
		result.Candidates = append(result.Candidates, FinalDefaultPairCandidate{Snapshot: snapshot.Clone()})
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
		if s.CollectionGeneration != 0 || s.CollectionRevision != "" || s.DefaultTemplate != nil || len(s.Candidates) != 0 {
			return fmt.Errorf("absent final authority carries selection state")
		}
		return nil
	}
	if s.CollectionGeneration == 0 || s.CollectionRevision.Validate() != nil {
		return fmt.Errorf("final default-pair selection receipt is invalid")
	}
	if s.DefaultTemplate == nil {
		if len(s.Candidates) != 0 {
			return fmt.Errorf("default-pair candidates exist without a default Template")
		}
		return nil
	}
	if err := s.DefaultTemplate.Validate(); err != nil {
		return err
	}
	seen := make(map[ContextID]bool, len(s.Candidates))
	previousRoot := ""
	for index, candidate := range s.Candidates {
		if err := candidate.Validate(s.CanonicalCWD, s.DefaultTemplate.ID); err != nil {
			return fmt.Errorf("default-pair candidate %d is invalid: %w", index, err)
		}
		if seen[candidate.Snapshot.Context.ID] {
			return fmt.Errorf("default-pair candidate Context IDs must be unique")
		}
		seen[candidate.Snapshot.Context.ID] = true
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
	return len(s.Candidates) > 0 && s.Candidates[0].Snapshot.Workspace.ProjectRoot != s.CanonicalCWD
}

func (s FinalDefaultPairSelection) AutomaticChoice() (FinalDefaultPairSelectionChoice, bool) {
	if s.RequiresChoice() {
		return FinalDefaultPairSelectionChoice{}, false
	}
	if len(s.Candidates) == 0 {
		return FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionCreate}, true
	}
	return FinalDefaultPairSelectionChoice{Kind: FinalDefaultPairSelectionUse, ContextID: s.Candidates[0].Snapshot.Context.ID}, true
}

func (s FinalDefaultPairSelection) ValidateChoice(choice FinalDefaultPairSelectionChoice) error {
	switch choice.Kind {
	case FinalDefaultPairSelectionCreate:
		if choice.ContextID != "" {
			return fmt.Errorf("create choice must not contain a Context ID")
		}
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Workspace.ProjectRoot == s.CanonicalCWD {
				return fmt.Errorf("current directory already has a default Context")
			}
		}
		return nil
	case FinalDefaultPairSelectionUse:
		if choice.ContextID.Validate() != nil {
			return fmt.Errorf("use choice requires a valid Context ID")
		}
		for _, candidate := range s.Candidates {
			if candidate.Snapshot.Context.ID == choice.ContextID {
				return nil
			}
		}
		return fmt.Errorf("selected Context is not present in the snapshot")
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
			if candidate.Snapshot.Context.ID == choice.ContextID {
				value := candidate.Snapshot.Clone()
				selected = &value
				root = value.Workspace.ProjectRoot
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
	if collection.DefaultTemplateID == nil {
		return result, result.Validate()
	}
	for index := range collection.Templates {
		if collection.Templates[index].ID == *collection.DefaultTemplateID {
			value := collection.Templates[index].Clone()
			result.DefaultTemplate = &value
			break
		}
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairObservation{}, err
	}
	for _, snapshot := range snapshots {
		if result.DefaultTemplate != nil && snapshot.Workspace != nil && snapshot.Workspace.ProjectRoot == projectRoot && snapshot.Context.TemplateID == result.DefaultTemplate.ID {
			value := snapshot.Clone()
			result.Context = &value
			break
		}
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
	result, err := NewFinalDefaultPairObservation(collection, present, projectRoot)
	if err != nil {
		return FinalDefaultPairObservation{}, err
	}
	if !present || result.DefaultTemplate == nil {
		return FinalDefaultPairObservation{}, ErrContextBindingNotFound
	}
	snapshots, err := collection.ContextSnapshots()
	if err != nil {
		return FinalDefaultPairObservation{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.ID != contextID {
			continue
		}
		if snapshot.Context.TemplateID != result.DefaultTemplate.ID {
			return FinalDefaultPairObservation{}, fmt.Errorf("selected Context belongs to another Template")
		}
		if snapshot.Workspace != nil && snapshot.Workspace.ProjectRoot != projectRoot {
			return FinalDefaultPairObservation{}, fmt.Errorf("selected Context Workspace belongs to another Project")
		}
		value := snapshot.Clone()
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
