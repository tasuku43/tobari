package tobari

import "fmt"

const FinalDefaultPairObservationSchemaVersion = 1

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
		if result.DefaultTemplate != nil && snapshot.Context.ProjectRoot == projectRoot && snapshot.Context.TemplateID == result.DefaultTemplate.ID {
			value := snapshot.Clone()
			result.Context = &value
			break
		}
	}
	return result, result.Validate()
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
	if o.Context.Context.ProjectRoot != o.ProjectRoot || o.Context.Context.TemplateID != o.DefaultTemplate.ID || o.Context.Template.ID != o.DefaultTemplate.ID || o.Context.Template.Current.Revision != o.DefaultTemplate.Current.Revision {
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
	if p.Previous.ProjectRoot != projectRoot || p.Current.ProjectRoot != projectRoot || p.Current.DefaultTemplate == nil || p.Current.Context == nil || p.Current.Context.Context.TemplateID != p.Current.DefaultTemplate.ID || p.Current.Context.Context.ProjectRoot != projectRoot {
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
