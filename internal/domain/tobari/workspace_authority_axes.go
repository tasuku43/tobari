package tobari

import (
	"fmt"
	"reflect"
)

const ContextAuthorityAxesSchemaVersion = 1

// ContextAuthorityAxes is the task-readable desired, active, and applied
// authority for one final Context. Each axis remains independent: callers
// must not infer that current Template or Policy Memory authority is active
// merely because an activation receipt exists.
type ContextAuthorityAxes struct {
	SchemaVersion                    int
	ContextID                        ContextID
	TemplateID                       WorkspaceTemplateID
	DesiredTemplateGeneration        uint64
	DesiredTemplateRevision          SemanticDigest
	DesiredTemplatePolicySliceDigest SemanticDigest
	ActiveTemplatePolicySliceDigest  *SemanticDigest
	CurrentPolicyMemoryRevision      SemanticDigest
	ActivePolicyMemoryRevision       *SemanticDigest
	AppliedEntry                     *WorkspaceAppliedEntry
}

func NewContextAuthorityAxes(snapshot ContextAuthoritySnapshot) (ContextAuthorityAxes, error) {
	if err := snapshot.Validate(); err != nil {
		return ContextAuthorityAxes{}, err
	}
	result := ContextAuthorityAxes{
		SchemaVersion:                    ContextAuthorityAxesSchemaVersion,
		ContextID:                        snapshot.Context.ID,
		TemplateID:                       snapshot.Template.ID,
		DesiredTemplateGeneration:        snapshot.Template.Current.Generation,
		DesiredTemplateRevision:          snapshot.Template.Current.Revision,
		DesiredTemplatePolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest,
		CurrentPolicyMemoryRevision:      snapshot.PolicyMemory.Revision,
	}
	if snapshot.ActiveTemplatePolicy != nil {
		value := snapshot.ActiveTemplatePolicy.PolicySliceDigest
		result.ActiveTemplatePolicySliceDigest = &value
	}
	if snapshot.ActivePolicyMemoryRef != nil {
		value := snapshot.ActivePolicyMemoryRef.Revision
		result.ActivePolicyMemoryRevision = &value
	}
	if snapshot.Workspace != nil && snapshot.Workspace.LastSuccessfulEntry != nil {
		value := *snapshot.Workspace.LastSuccessfulEntry
		result.AppliedEntry = &value
	}
	return result, result.ValidateFor(snapshot)
}

func (a ContextAuthorityAxes) Validate() error {
	if a.SchemaVersion != ContextAuthorityAxesSchemaVersion || a.ContextID.Validate() != nil || a.TemplateID.Validate() != nil || a.DesiredTemplateGeneration == 0 {
		return fmt.Errorf("Context authority axes metadata is invalid")
	}
	for name, digest := range map[string]SemanticDigest{
		"desired Template revision":     a.DesiredTemplateRevision,
		"desired Template policy slice": a.DesiredTemplatePolicySliceDigest,
		"current Policy Memory":         a.CurrentPolicyMemoryRevision,
	} {
		if err := digest.Validate(); err != nil {
			return fmt.Errorf("Context authority axes %s: %w", name, err)
		}
	}
	if a.ActiveTemplatePolicySliceDigest != nil {
		if err := a.ActiveTemplatePolicySliceDigest.Validate(); err != nil {
			return fmt.Errorf("Context authority axes active Template policy: %w", err)
		}
	}
	if a.ActivePolicyMemoryRevision != nil {
		if err := a.ActivePolicyMemoryRevision.Validate(); err != nil {
			return fmt.Errorf("Context authority axes active Policy Memory: %w", err)
		}
	}
	if a.AppliedEntry != nil {
		if err := a.AppliedEntry.Validate(); err != nil {
			return err
		}
		if a.AppliedEntry.ContextID != a.ContextID || a.AppliedEntry.TemplateID != a.TemplateID {
			return fmt.Errorf("Context authority axes AppliedEntry has another owner")
		}
	}
	return nil
}

func (a ContextAuthorityAxes) ValidateFor(snapshot ContextAuthoritySnapshot) error {
	if err := a.Validate(); err != nil {
		return err
	}
	if err := snapshot.Validate(); err != nil {
		return err
	}
	want, err := newContextAuthorityAxesUnchecked(snapshot)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(a, want) {
		return fmt.Errorf("Context authority axes do not match the complete snapshot")
	}
	return nil
}

func newContextAuthorityAxesUnchecked(snapshot ContextAuthoritySnapshot) (ContextAuthorityAxes, error) {
	if err := snapshot.Validate(); err != nil {
		return ContextAuthorityAxes{}, err
	}
	result := ContextAuthorityAxes{
		SchemaVersion:                    ContextAuthorityAxesSchemaVersion,
		ContextID:                        snapshot.Context.ID,
		TemplateID:                       snapshot.Template.ID,
		DesiredTemplateGeneration:        snapshot.Template.Current.Generation,
		DesiredTemplateRevision:          snapshot.Template.Current.Revision,
		DesiredTemplatePolicySliceDigest: snapshot.Template.Current.Slices.PolicySliceDigest,
		CurrentPolicyMemoryRevision:      snapshot.PolicyMemory.Revision,
	}
	if snapshot.ActiveTemplatePolicy != nil {
		value := snapshot.ActiveTemplatePolicy.PolicySliceDigest
		result.ActiveTemplatePolicySliceDigest = &value
	}
	if snapshot.ActivePolicyMemoryRef != nil {
		value := snapshot.ActivePolicyMemoryRef.Revision
		result.ActivePolicyMemoryRevision = &value
	}
	if snapshot.Workspace != nil && snapshot.Workspace.LastSuccessfulEntry != nil {
		value := *snapshot.Workspace.LastSuccessfulEntry
		result.AppliedEntry = &value
	}
	return result, nil
}

func (a ContextAuthorityAxes) Clone() ContextAuthorityAxes {
	result := a
	if a.ActiveTemplatePolicySliceDigest != nil {
		value := *a.ActiveTemplatePolicySliceDigest
		result.ActiveTemplatePolicySliceDigest = &value
	}
	if a.ActivePolicyMemoryRevision != nil {
		value := *a.ActivePolicyMemoryRevision
		result.ActivePolicyMemoryRevision = &value
	}
	if a.AppliedEntry != nil {
		value := *a.AppliedEntry
		result.AppliedEntry = &value
	}
	return result
}
