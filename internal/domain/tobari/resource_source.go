package tobari

import (
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

var (
	ErrResourceSourceMissing                 = errors.New("resource source is missing")
	ErrResourceSourceInvalid                 = errors.New("resource source is invalid")
	ErrResourceSourceModified                = errors.New("resource source has unapplied changes")
	ErrResourceSourceChanged                 = errors.New("resource source changed during apply")
	ErrResourceSourceRecoveryRequired        = errors.New("resource source publication requires recovery")
	ErrWorkspaceTemplateChangePlanStale      = errors.New("Workspace Template change plan is stale")
	ErrWorkspaceTemplatePolicyMigrationStale = errors.New("Workspace Template policy migration plan is stale")
	ErrDirectTemplateMutationRetired         = errors.New("direct Template mutation is retired; edit source and use Plan/Apply")
	ErrDirectContextMutationRetired          = errors.New("direct Context mutation is retired; create source and use Plan/Apply")
	ErrResourceIdentityDeleted               = errors.New("resource identity was deleted and cannot be reused")
)

const (
	WorkspaceTemplateSourceSchemaVersion      = "tobari.dev/template/v1"
	ContextSourceSchemaVersion                = "tobari.dev/context/v1"
	WorkspaceTemplatePolicySchemaVersion      = "tobari.dev/template-policy/v1"
	WorkspaceTemplatePolicyAlphaSchemaVersion = "tobari.dev/template-policy/v1alpha1"
)

// WorkspaceTemplateSource is the complete two-document user-editable
// declaration for one Template. Revision history and every lower-lifetime
// authority deliberately remain outside this aggregate.
type WorkspaceTemplateSource struct {
	Template WorkspaceTemplateSourceDocument
	Policy   WorkspaceTemplatePolicySourceDocument
}

type WorkspaceTemplateDraft struct {
	ID     WorkspaceTemplateID       `json:"workspace_template_id"`
	Name   string                    `json:"name"`
	Body   WorkspaceTemplateBody     `json:"body"`
	Source ResourceSourceObservation `json:"source"`
}

func (d WorkspaceTemplateDraft) Validate() error {
	if err := d.ID.Validate(); err != nil {
		return err
	}
	if d.Source.State == ResourceSourceInvalid {
		if d.Name != "" {
			return fmt.Errorf("invalid draft source cannot claim a display name")
		}
		return d.Source.Validate()
	}
	if err := ValidateName(d.Name); err != nil {
		return err
	}
	if err := d.Body.Validate(); err != nil {
		return err
	}
	if err := d.Source.Validate(); err != nil {
		return err
	}
	return nil
}

type WorkspaceTemplateSourceDocument struct {
	SchemaVersion    string                               `json:"schema"`
	TemplateID       WorkspaceTemplateID                  `json:"workspace_template_id"`
	Name             string                               `json:"name"`
	BaseRevision     *SemanticDigest                      `json:"base_revision"`
	SourceAccess     ManifestSourceAccess                 `json:"source_access"`
	EntryDefaults    WorkspaceTemplateSourceEntryDefaults `json:"entry_defaults"`
	SessionDefaults  WorkspaceTemplateSessionDefaults     `json:"session_defaults"`
	CreationDefaults WorkspaceTemplateCreationDefaults    `json:"creation_defaults"`
}

// RuntimeSourceRef is the complete Runtime authority users can select in
// template.yaml. Mutable presentation metadata and infrastructure image
// selectors are resolved from the immutable revision during Plan/Apply and
// never become editable Template source.
type RuntimeSourceRef struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
}

func (r RuntimeSourceRef) Validate() error {
	if r.ID != StandardRuntimeID {
		if err := ValidateRuntimeID(r.ID); err != nil {
			return err
		}
	}
	return ValidateDigest(r.Revision)
}

func RuntimeSourceRefFrom(binding RuntimeBinding) RuntimeSourceRef {
	return RuntimeSourceRef{ID: binding.RuntimeID, Revision: binding.Revision}
}

func (r RuntimeSourceRef) Matches(binding RuntimeBinding) bool {
	return r.ID == binding.RuntimeID && r.Revision == binding.Revision
}

type WorkspaceTemplateSourceEntryDefaults struct {
	Runtime RuntimeSourceRef `json:"runtime"`
}

func (e WorkspaceTemplateSourceEntryDefaults) Validate() error { return e.Runtime.Validate() }

type WorkspaceTemplatePolicySourceDocument struct {
	SchemaVersion string                                `json:"schema"`
	TemplateID    WorkspaceTemplateID                   `json:"workspace_template_id"`
	Boundary      WorkspaceTemplatePolicyBoundarySource `json:"boundary"`
	Semantic      WorkspaceTemplatePolicySemanticSource `json:"semantic"`
}

type WorkspaceTemplatePolicyBoundarySource struct {
	Methods WorkspaceTemplateMethodBoundarySource `json:"methods"`
}

type WorkspaceTemplateMethodBoundarySource struct {
	Deny []string `json:"deny"`
}

type WorkspaceTemplatePolicySemanticSource struct {
	AgentProfile    string                             `json:"agent_profile"`
	NativeReadiness ManifestNativeReadiness            `json:"native_readiness"`
	Protocols       WorkspaceTemplateSemanticProtocols `json:"protocols"`
	Providers       WorkspaceTemplateSemanticProviders `json:"providers"`
}

// WorkspaceTemplatePolicyAlphaSourceDocument is the exact predecessor source
// envelope accepted only by the explicit non-activating migration workflow.
// Ordinary source reads and Template Plan/Apply never decode this type.
type WorkspaceTemplatePolicyAlphaSourceDocument struct {
	SchemaVersion string                                     `json:"schema"`
	TemplateID    WorkspaceTemplateID                        `json:"workspace_template_id"`
	Boundary      WorkspaceTemplatePolicyAlphaBoundarySource `json:"boundary"`
	Semantic      WorkspaceTemplatePolicyBody                `json:"semantic"`
}

type WorkspaceTemplatePolicyAlphaBoundarySource struct {
	DestinationCeiling ManifestPolicyDestinationCeiling `json:"destination_ceiling"`
	MethodPolicy       ManifestMethodPolicy             `json:"method_policy"`
}

type WorkspaceTemplateAlphaSource struct {
	Template WorkspaceTemplateSourceDocument
	Policy   WorkspaceTemplatePolicyAlphaSourceDocument
}

func (s WorkspaceTemplateAlphaSource) Validate() error {
	if s.Template.SchemaVersion != WorkspaceTemplateSourceSchemaVersion || s.Policy.SchemaVersion != WorkspaceTemplatePolicyAlphaSchemaVersion {
		return fmt.Errorf("Template alpha source schema is unsupported")
	}
	if err := s.Template.validate(); err != nil {
		return err
	}
	if s.Policy.TemplateID != s.Template.TemplateID {
		return fmt.Errorf("Template alpha source identity does not match")
	}
	if s.Policy.Semantic.SemanticModules != nil {
		return fmt.Errorf("Template alpha source contains final semantic modules")
	}
	boundary := WorkspaceTemplateBoundary{
		SourceAccess:       s.Template.SourceAccess,
		DestinationCeiling: s.Policy.Boundary.DestinationCeiling,
		MethodPolicy:       s.Policy.Boundary.MethodPolicy.Clone(),
	}
	if err := boundary.Validate(); err != nil {
		return fmt.Errorf("Template alpha source Boundary: %w", err)
	}
	if err := s.Policy.Semantic.Validate(boundary); err != nil {
		return fmt.Errorf("Template alpha source Semantic Policy: %w", err)
	}
	return nil
}

func (s WorkspaceTemplateAlphaSource) Body(resolved RuntimeBinding) (WorkspaceTemplateBody, error) {
	if err := s.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	if err := resolved.Validate(); err != nil || !s.Template.EntryDefaults.Runtime.Matches(resolved) {
		return WorkspaceTemplateBody{}, fmt.Errorf("Template alpha source Runtime does not match exact resolved revision")
	}
	body := WorkspaceTemplateBody{
		Boundary: WorkspaceTemplateBoundary{
			SourceAccess:       s.Template.SourceAccess,
			DestinationCeiling: s.Policy.Boundary.DestinationCeiling,
			MethodPolicy:       s.Policy.Boundary.MethodPolicy.Clone(),
		},
		Policy:           s.Policy.Semantic.Clone(),
		EntryDefaults:    WorkspaceTemplateEntryDefaults{Runtime: resolved},
		SessionDefaults:  s.Template.SessionDefaults.Clone(),
		CreationDefaults: s.Template.CreationDefaults.Clone(),
	}
	return body, body.Validate()
}

func (s WorkspaceTemplateAlphaSource) MigrateToV1(resolved RuntimeBinding) (WorkspaceTemplateSource, error) {
	body, err := s.Body(resolved)
	if err != nil {
		return WorkspaceTemplateSource{}, err
	}
	if err := validateV1ConvertibleBoundary(body.Boundary); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	modules, err := migrateAlphaPolicyBody(body.Policy)
	if err != nil {
		return WorkspaceTemplateSource{}, err
	}
	semantic := WorkspaceTemplatePolicySemanticSource{
		AgentProfile:    body.Policy.AgentProfile,
		NativeReadiness: body.Policy.NativeReadiness,
		Protocols:       modules.Protocols,
		Providers:       modules.Providers,
	}
	result := WorkspaceTemplateSource{
		Template: s.Template,
		Policy: WorkspaceTemplatePolicySourceDocument{
			SchemaVersion: WorkspaceTemplatePolicySchemaVersion,
			TemplateID:    s.Policy.TemplateID,
			Boundary:      WorkspaceTemplatePolicyBoundarySource{Methods: WorkspaceTemplateMethodBoundarySource{Deny: deniedMethodsFromPolicy(body.Boundary.MethodPolicy)}},
			Semantic:      semantic,
		},
	}
	if err := result.Validate(); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	return result, nil
}

func (d WorkspaceTemplateSourceDocument) validate() error {
	if d.SchemaVersion != WorkspaceTemplateSourceSchemaVersion {
		return fmt.Errorf("Template source schema is unsupported")
	}
	if err := d.TemplateID.Validate(); err != nil {
		return fmt.Errorf("Template source identity: %w", err)
	}
	if err := ValidateName(d.Name); err != nil {
		return fmt.Errorf("Template source name: %w", err)
	}
	if d.BaseRevision != nil {
		if err := d.BaseRevision.Validate(); err != nil {
			return fmt.Errorf("Template source base revision: %w", err)
		}
	}
	if err := d.EntryDefaults.Validate(); err != nil {
		return fmt.Errorf("Template source Runtime: %w", err)
	}
	if err := d.SourceAccess.Validate(); err != nil {
		return fmt.Errorf("Template source access: %w", err)
	}
	if err := d.SessionDefaults.Validate(); err != nil {
		return fmt.Errorf("Template session defaults: %w", err)
	}
	if err := d.CreationDefaults.Validate(); err != nil {
		return fmt.Errorf("Template creation defaults: %w", err)
	}
	return nil
}

func (b WorkspaceTemplatePolicyBoundarySource) Clone() WorkspaceTemplatePolicyBoundarySource {
	return WorkspaceTemplatePolicyBoundarySource{Methods: WorkspaceTemplateMethodBoundarySource{Deny: append([]string{}, b.Methods.Deny...)}}
}

func (s WorkspaceTemplatePolicySemanticSource) modules() WorkspaceTemplateSemanticModules {
	// Whole-module omission in source means known-none. The strict source
	// topology reader rejects a present module with missing allow/deny sets;
	// normalization here expands only the value-type representation of omitted
	// modules before the compiled authority validator runs.
	return (WorkspaceTemplateSemanticModules{Protocols: s.Protocols, Providers: s.Providers}).normalized(false)
}

func semanticSourceFromPolicy(policy WorkspaceTemplatePolicyBody) (WorkspaceTemplatePolicySemanticSource, error) {
	modules := policy.SemanticModules
	if modules == nil {
		converted, err := migrateAlphaPolicyBody(policy)
		if err != nil {
			return WorkspaceTemplatePolicySemanticSource{}, err
		}
		modules = &converted
	}
	normalized := modules.Normalize()
	return WorkspaceTemplatePolicySemanticSource{
		AgentProfile: policy.AgentProfile, NativeReadiness: policy.NativeReadiness,
		Protocols: normalized.Protocols, Providers: normalized.Providers,
	}, nil
}

func NewWorkspaceTemplateSource(template WorkspaceTemplate) (WorkspaceTemplateSource, error) {
	if err := template.Validate(); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	if err := validateV1ConvertibleBoundary(template.Current.Body.Boundary); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	semantic, err := semanticSourceFromPolicy(template.Current.Body.Policy)
	if err != nil {
		return WorkspaceTemplateSource{}, err
	}
	source := WorkspaceTemplateSource{Template: WorkspaceTemplateSourceDocument{
		SchemaVersion:    WorkspaceTemplateSourceSchemaVersion,
		TemplateID:       template.ID,
		Name:             template.Name,
		BaseRevision:     semanticDigestPointer(template.Current.Revision),
		SourceAccess:     template.Current.Body.Boundary.SourceAccess,
		EntryDefaults:    WorkspaceTemplateSourceEntryDefaults{Runtime: RuntimeSourceRefFrom(template.Current.Body.EntryDefaults.Runtime)},
		SessionDefaults:  template.Current.Body.SessionDefaults.Clone(),
		CreationDefaults: template.Current.Body.CreationDefaults.Clone(),
	}, Policy: WorkspaceTemplatePolicySourceDocument{
		SchemaVersion: WorkspaceTemplatePolicySchemaVersion,
		TemplateID:    template.ID,
		Boundary:      WorkspaceTemplatePolicyBoundarySource{Methods: WorkspaceTemplateMethodBoundarySource{Deny: deniedMethodsFromPolicy(template.Current.Body.Boundary.MethodPolicy)}},
		Semantic:      semantic,
	}}
	return source, source.Validate()
}

func NewWorkspaceTemplateDraftSource(id WorkspaceTemplateID, name string, body WorkspaceTemplateBody) (WorkspaceTemplateSource, error) {
	if err := id.Validate(); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	if err := ValidateName(name); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	if err := body.Validate(); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	if err := validateV1ConvertibleBoundary(body.Boundary); err != nil {
		return WorkspaceTemplateSource{}, err
	}
	semantic, err := semanticSourceFromPolicy(body.Policy)
	if err != nil {
		return WorkspaceTemplateSource{}, err
	}
	source := WorkspaceTemplateSource{Template: WorkspaceTemplateSourceDocument{
		SchemaVersion: WorkspaceTemplateSourceSchemaVersion, TemplateID: id, Name: name, BaseRevision: nil,
		SourceAccess: body.Boundary.SourceAccess, EntryDefaults: WorkspaceTemplateSourceEntryDefaults{Runtime: RuntimeSourceRefFrom(body.EntryDefaults.Runtime)},
		SessionDefaults: body.SessionDefaults.Clone(), CreationDefaults: body.CreationDefaults.Clone(),
	}, Policy: WorkspaceTemplatePolicySourceDocument{
		SchemaVersion: WorkspaceTemplatePolicySchemaVersion, TemplateID: id,
		Boundary: WorkspaceTemplatePolicyBoundarySource{Methods: WorkspaceTemplateMethodBoundarySource{Deny: deniedMethodsFromPolicy(body.Boundary.MethodPolicy)}},
		Semantic: semantic,
	}}
	return source, source.Validate()
}

func (s WorkspaceTemplateSource) Validate() error {
	if s.Template.SchemaVersion != WorkspaceTemplateSourceSchemaVersion || s.Policy.SchemaVersion != WorkspaceTemplatePolicySchemaVersion {
		return fmt.Errorf("Template source schema is unsupported")
	}
	if err := s.Template.validate(); err != nil {
		return err
	}
	if s.Policy.TemplateID != s.Template.TemplateID {
		return fmt.Errorf("Template source identity does not match")
	}
	boundary, err := s.compiledBoundary()
	if err != nil {
		return fmt.Errorf("Template source Boundary: %w", err)
	}
	if err := boundary.Validate(); err != nil {
		return fmt.Errorf("Template source Boundary: %w", err)
	}
	if err := ValidateName(s.Policy.Semantic.AgentProfile); err != nil {
		return fmt.Errorf("Template agent profile: %w", err)
	}
	if err := s.Policy.Semantic.NativeReadiness.Validate(); err != nil {
		return fmt.Errorf("Template native readiness: %w", err)
	}
	if err := s.Policy.Semantic.modules().Validate(s.Policy.Boundary.Methods.Deny); err != nil {
		return fmt.Errorf("Template source Semantic Policy: %w", err)
	}
	return nil
}

func (s WorkspaceTemplateSource) Clone() WorkspaceTemplateSource {
	result := s
	if s.Template.BaseRevision != nil {
		value := *s.Template.BaseRevision
		result.Template.BaseRevision = &value
	}
	result.Template.SessionDefaults = s.Template.SessionDefaults.Clone()
	result.Template.CreationDefaults = s.Template.CreationDefaults.Clone()
	result.Policy.Boundary = s.Policy.Boundary.Clone()
	modules := s.Policy.Semantic.modules().Clone()
	result.Policy.Semantic.Protocols = modules.Protocols
	result.Policy.Semantic.Providers = modules.Providers
	return result
}

func (s WorkspaceTemplateSource) Body(resolved RuntimeBinding) (WorkspaceTemplateBody, error) {
	if err := s.Validate(); err != nil {
		return WorkspaceTemplateBody{}, err
	}
	if err := resolved.Validate(); err != nil || !s.Template.EntryDefaults.Runtime.Matches(resolved) {
		return WorkspaceTemplateBody{}, fmt.Errorf("Template source Runtime does not match exact resolved revision")
	}
	boundary, err := s.compiledBoundary()
	if err != nil {
		return WorkspaceTemplateBody{}, err
	}
	modules := s.Policy.Semantic.modules().Normalize()
	body := WorkspaceTemplateBody{
		Boundary: boundary, Policy: WorkspaceTemplatePolicyBody{
			AgentProfile: s.Policy.Semantic.AgentProfile, NativeReadiness: s.Policy.Semantic.NativeReadiness,
			SemanticModules: &modules,
			BaselineGrants:  []ManifestPolicyExactRule{}, BaselineTemplates: []ManifestPolicyPathTemplateRule{},
			MCPBaselineGrants: []ManifestPolicyMCPRule{}, BaselineDenies: []ManifestPolicyExactRule{},
			GraphQLEndpoints: []ManifestPolicyExactRule{}, MCPEndpoints: []ManifestPolicyExactRule{},
		},
		EntryDefaults: WorkspaceTemplateEntryDefaults{Runtime: resolved}, SessionDefaults: s.Template.SessionDefaults.Clone(),
		CreationDefaults: s.Template.CreationDefaults.Clone(),
	}
	return body, body.Validate()
}

func (s WorkspaceTemplateSource) compiledBoundary() (WorkspaceTemplateBoundary, error) {
	if s.Policy.Boundary.Methods.Deny == nil {
		return WorkspaceTemplateBoundary{}, fmt.Errorf("method deny collection must be explicit")
	}
	denied := append([]string{}, s.Policy.Boundary.Methods.Deny...)
	seen := make(map[string]struct{}, len(denied))
	for _, method := range denied {
		if !httpMethodPattern.MatchString(method) {
			return WorkspaceTemplateBoundary{}, fmt.Errorf("method deny is invalid")
		}
		if _, duplicate := seen[method]; duplicate {
			return WorkspaceTemplateBoundary{}, fmt.Errorf("method deny is duplicated")
		}
		seen[method] = struct{}{}
	}
	sort.Strings(denied)
	overrides := make([]ManifestMethodOverride, 0, len(denied))
	for _, method := range denied {
		overrides = append(overrides, ManifestMethodOverride{Method: method, Decision: ManifestMethodDeny})
	}
	return WorkspaceTemplateBoundary{
		SourceAccess:       s.Template.SourceAccess,
		DestinationCeiling: ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []ManifestPolicyAuthority{}},
		MethodPolicy:       ManifestMethodPolicy{Default: ManifestMethodExactReview, Overrides: overrides},
	}, nil
}

func deniedMethodsFromPolicy(policy ManifestMethodPolicy) []string {
	result := []string{}
	if policy.Default != ManifestMethodExactReview {
		return nil
	}
	for _, override := range policy.Overrides {
		if override.Decision != ManifestMethodDeny {
			return nil
		}
		result = append(result, override.Method)
	}
	sort.Strings(result)
	return result
}

func (s WorkspaceTemplateSource) SemanticRevision(resolved RuntimeBinding) (SemanticDigest, error) {
	body, err := s.Body(resolved)
	if err != nil {
		return "", err
	}
	return semanticIdentity(body)
}

func (s WorkspaceTemplateSource) ValidateFor(template WorkspaceTemplate) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := template.Validate(); err != nil {
		return err
	}
	if s.Template.TemplateID != template.ID || s.Policy.TemplateID != template.ID {
		return fmt.Errorf("Template source identity does not match its active Template")
	}
	if s.Template.BaseRevision == nil || *s.Template.BaseRevision != template.Current.Revision {
		return fmt.Errorf("Template source base revision does not match current active authority")
	}
	return nil
}

func semanticDigestPointer(value SemanticDigest) *SemanticDigest {
	copy := value
	return &copy
}

// ContextSource is the user-visible declaration for one immutable Context
// binding. Policy Memory and Workspace state are intentionally unrepresentable.
type ContextSource struct {
	SchemaVersion string              `json:"schema"`
	ContextID     ContextID           `json:"context_id"`
	ProjectRoot   string              `json:"project_root"`
	TemplateID    WorkspaceTemplateID `json:"workspace_template_id"`
}

func NewContextSource(binding ContextBinding) (ContextSource, error) {
	if err := binding.Validate(); err != nil {
		return ContextSource{}, err
	}
	source := ContextSource{
		SchemaVersion: ContextSourceSchemaVersion,
		ContextID:     binding.ID,
		ProjectRoot:   binding.ProjectRoot,
		TemplateID:    binding.TemplateID,
	}
	return source, source.Validate()
}

func (s ContextSource) Validate() error {
	if s.SchemaVersion != ContextSourceSchemaVersion {
		return fmt.Errorf("Context source schema is unsupported")
	}
	binding := ContextBinding{
		SchemaVersion: ContextBindingSchemaVersion,
		ID:            s.ContextID,
		ProjectRoot:   s.ProjectRoot,
		TemplateID:    s.TemplateID,
	}
	return binding.Validate()
}

func (s ContextSource) ValidateFor(binding ContextBinding) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := binding.Validate(); err != nil {
		return err
	}
	if s.ContextID != binding.ID || s.ProjectRoot != binding.ProjectRoot || s.TemplateID != binding.TemplateID {
		return fmt.Errorf("Context source does not match its immutable active binding")
	}
	return nil
}

func ContextSourceSemanticRevision(binding ContextBinding) (SemanticDigest, error) {
	source, err := NewContextSource(binding)
	if err != nil {
		return "", err
	}
	return semanticIdentity(source)
}

type ContextDraft struct {
	Source      ContextSource             `json:"source"`
	Observation ResourceSourceObservation `json:"observation"`
}

func (d ContextDraft) Validate() error {
	if d.Observation.State == ResourceSourceInvalid {
		if d.Source.ContextID.Validate() != nil || d.Observation.Validate() != nil || d.Observation.ActiveRevision != nil || d.Observation.SourceRevision != nil {
			return fmt.Errorf("invalid Context draft observation is invalid")
		}
		return nil
	}
	if err := d.Source.Validate(); err != nil {
		return err
	}
	if err := d.Observation.Validate(); err != nil {
		return err
	}
	if d.Observation.ActiveRevision != nil || d.Observation.State != ResourceSourceModified {
		return fmt.Errorf("Context draft observation is invalid")
	}
	return nil
}

const ContextActivationPlanSchemaVersion = 1

type ContextActivationPlan struct {
	SchemaVersion        int                         `json:"schema_version"`
	PlanRef              string                      `json:"plan_ref"`
	ContextRef           string                      `json:"context_ref"`
	SourceFingerprint    string                      `json:"source_fingerprint"`
	ProjectRoot          string                      `json:"project_root"`
	TemplateRef          string                      `json:"template_ref"`
	TemplateRevision     SemanticDigest              `json:"template_revision"`
	DuplicateBinding     bool                        `json:"duplicate_binding"`
	NoOp                 bool                        `json:"no_op"`
	SourceAccess         ManifestSourceAccess        `json:"source_access"`
	Runtime              RuntimeBinding              `json:"runtime"`
	Boundary             WorkspaceTemplateBoundary   `json:"boundary"`
	Semantic             WorkspaceTemplatePolicyBody `json:"semantic"`
	BoundaryFingerprint  SemanticDigest              `json:"boundary_fingerprint"`
	PolicySliceDigest    SemanticDigest              `json:"policy_slice_digest"`
	NewPolicyMemoryOwner ContextID                   `json:"new_policy_memory_owner"`
}

type contextActivationPlanAuthority ContextActivationPlan

func NewContextActivationPlan(collection WorkspaceAuthorityCollection, source ContextSource, fingerprint string) (ContextActivationPlan, error) {
	if err := collection.Validate(); err != nil {
		return ContextActivationPlan{}, err
	}
	if err := source.Validate(); err != nil {
		return ContextActivationPlan{}, err
	}
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != fingerprint {
		return ContextActivationPlan{}, fmt.Errorf("Context source fingerprint is invalid")
	}
	var template *WorkspaceTemplate
	for index := range collection.Templates {
		if collection.Templates[index].ID == source.TemplateID {
			value := collection.Templates[index].Clone()
			template = &value
			break
		}
	}
	if template == nil {
		return ContextActivationPlan{}, ErrWorkspaceTemplateNotFound
	}
	noOp := false
	for _, record := range collection.Contexts {
		if record.Context.ID == source.ContextID {
			if err := source.ValidateFor(record.Context); err != nil {
				return ContextActivationPlan{}, ErrResourceSourceModified
			}
			noOp = true
			continue
		}
		if record.Context.ProjectRoot == source.ProjectRoot && record.Context.TemplateID == source.TemplateID {
			return ContextActivationPlan{}, ErrContextBindingExists
		}
	}
	contextRef, _ := ContextRef(source.ContextID)
	templateRef, _ := WorkspaceTemplateRef(source.TemplateID)
	plan := ContextActivationPlan{SchemaVersion: ContextActivationPlanSchemaVersion, ContextRef: contextRef, SourceFingerprint: fingerprint,
		ProjectRoot: source.ProjectRoot, TemplateRef: templateRef, TemplateRevision: template.Current.Revision, DuplicateBinding: false, NoOp: noOp,
		SourceAccess: template.Current.Body.Boundary.SourceAccess, Runtime: template.Current.Body.EntryDefaults.Runtime,
		Boundary: template.Current.Body.Boundary.Clone(), Semantic: template.Current.Body.Policy.Clone(), NewPolicyMemoryOwner: source.ContextID,
		BoundaryFingerprint: template.Current.Slices.BoundaryFingerprint, PolicySliceDigest: template.Current.Slices.PolicySliceDigest}
	digest, err := semanticIdentity(contextActivationPlanAuthority(plan))
	if err != nil {
		return ContextActivationPlan{}, err
	}
	plan.PlanRef = contextActivationPlanRefPrefix + string(source.ContextID) + "_" + strings.TrimPrefix(string(digest), "sha256:")
	return plan, plan.Validate()
}

func ParseContextActivationPlanRef(reference string) (ContextID, error) {
	if !strings.HasPrefix(reference, contextActivationPlanRefPrefix) {
		return "", fmt.Errorf("Context activation plan reference is invalid")
	}
	value := strings.TrimPrefix(reference, contextActivationPlanRefPrefix)
	separator := strings.LastIndexByte(value, '_')
	if separator <= 0 || len(value)-separator-1 != 64 {
		return "", fmt.Errorf("Context activation plan reference is invalid")
	}
	id, err := ParseContextID(value[:separator])
	if err != nil {
		return "", fmt.Errorf("Context activation plan reference is invalid")
	}
	if _, err := hex.DecodeString(value[separator+1:]); err != nil {
		return "", fmt.Errorf("Context activation plan reference is invalid")
	}
	return id, nil
}

func (p ContextActivationPlan) Validate() error {
	if p.SchemaVersion != ContextActivationPlanSchemaVersion || p.DuplicateBinding {
		return fmt.Errorf("Context activation plan metadata is invalid")
	}
	id, err := ParseContextActivationPlanRef(p.PlanRef)
	if err != nil {
		return err
	}
	parsed, err := ParseContextRef(p.ContextRef)
	if err != nil || parsed != id || p.NewPolicyMemoryOwner != id {
		return fmt.Errorf("Context activation plan identity is invalid")
	}
	if ValidateCanonicalRoot(p.ProjectRoot) != nil || p.TemplateRevision.Validate() != nil || p.BoundaryFingerprint.Validate() != nil || p.PolicySliceDigest.Validate() != nil || p.SourceAccess.Validate() != nil || p.Runtime.Validate() != nil || p.Boundary.Validate() != nil || p.Semantic.Validate(p.Boundary) != nil {
		return fmt.Errorf("Context activation plan authority is invalid")
	}
	if _, err := ParseWorkspaceTemplateRef(p.TemplateRef); err != nil {
		return fmt.Errorf("Context activation plan Template is invalid")
	}
	copy := p
	copy.PlanRef = ""
	digest, err := semanticIdentity(contextActivationPlanAuthority(copy))
	if err != nil {
		return err
	}
	want := contextActivationPlanRefPrefix + string(id) + "_" + strings.TrimPrefix(string(digest), "sha256:")
	if p.PlanRef != want {
		return fmt.Errorf("Context activation plan reference does not match authority")
	}
	return nil
}

type ResourceSourceState string

const (
	ResourceSourceInSync   ResourceSourceState = "in_sync"
	ResourceSourceModified ResourceSourceState = "modified"
	ResourceSourceMissing  ResourceSourceState = "missing"
	ResourceSourceInvalid  ResourceSourceState = "invalid"
)

func (s ResourceSourceState) Validate() error {
	switch s {
	case ResourceSourceInSync, ResourceSourceModified, ResourceSourceMissing, ResourceSourceInvalid:
		return nil
	default:
		return fmt.Errorf("resource source state is invalid")
	}
}

type ResourceSourceObservation struct {
	Path           string              `json:"source_path"`
	State          ResourceSourceState `json:"source_state"`
	SourceRevision *SemanticDigest     `json:"source_revision,omitempty"`
	ActiveRevision *SemanticDigest     `json:"active_revision,omitempty"`
}

const WorkspaceTemplatePolicyMigrationPlanSchemaVersion = 1

// WorkspaceTemplatePolicyMigrationPlan is a read-only content-addressed
// review of one alpha source pair and its exact non-activating V1 replacement.
type WorkspaceTemplatePolicyMigrationPlan struct {
	SchemaVersion     int            `json:"schema_version"`
	PlanRef           string         `json:"plan_ref"`
	TemplateRef       string         `json:"template_ref"`
	ActiveRevision    SemanticDigest `json:"active_revision"`
	SourceFingerprint string         `json:"source_fingerprint"`
	TargetFingerprint string         `json:"target_fingerprint"`
	SourceSchema      string         `json:"source_schema"`
	TargetSchema      string         `json:"target_schema"`
}

type workspaceTemplatePolicyMigrationPlanAuthority WorkspaceTemplatePolicyMigrationPlan

func NewWorkspaceTemplatePolicyMigrationPlan(template WorkspaceTemplate, alpha WorkspaceTemplateAlphaSource, migrated WorkspaceTemplateSource, sourceFingerprint, targetFingerprint string) (WorkspaceTemplatePolicyMigrationPlan, error) {
	if err := template.Validate(); err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	if err := alpha.Validate(); err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	if err := migrated.Validate(); err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	if !validSourceFingerprint(sourceFingerprint) || !validSourceFingerprint(targetFingerprint) || sourceFingerprint == targetFingerprint {
		return WorkspaceTemplatePolicyMigrationPlan{}, fmt.Errorf("Template policy migration fingerprints are invalid")
	}
	if alpha.Template.TemplateID != template.ID || migrated.Template.TemplateID != template.ID || alpha.Template.Name != template.Name || migrated.Template.Name != template.Name || alpha.Template.BaseRevision == nil || *alpha.Template.BaseRevision != template.Current.Revision || migrated.Template.BaseRevision == nil || *migrated.Template.BaseRevision != template.Current.Revision {
		return WorkspaceTemplatePolicyMigrationPlan{}, fmt.Errorf("Template policy migration does not bind the exact active Template")
	}
	before, err := alpha.Body(template.Current.Body.EntryDefaults.Runtime)
	if err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	after, err := migrated.Body(template.Current.Body.EntryDefaults.Runtime)
	if err != nil || !reflect.DeepEqual(before, template.Current.Body) || !reflect.DeepEqual(after.Boundary, before.Boundary) || !reflect.DeepEqual(after.EntryDefaults, before.EntryDefaults) || !reflect.DeepEqual(after.SessionDefaults, before.SessionDefaults) || !reflect.DeepEqual(after.CreationDefaults, before.CreationDefaults) || after.Policy.AgentProfile != before.Policy.AgentProfile || after.Policy.NativeReadiness != before.Policy.NativeReadiness {
		return WorkspaceTemplatePolicyMigrationPlan{}, fmt.Errorf("Template policy migration is not semantically in sync")
	}
	expected, err := alpha.MigrateToV1(template.Current.Body.EntryDefaults.Runtime)
	if err != nil || !reflect.DeepEqual(expected, migrated) {
		return WorkspaceTemplatePolicyMigrationPlan{}, fmt.Errorf("Template policy migration target is not the exact lossless conversion")
	}
	templateRef, _ := WorkspaceTemplateRef(template.ID)
	plan := WorkspaceTemplatePolicyMigrationPlan{
		SchemaVersion:     WorkspaceTemplatePolicyMigrationPlanSchemaVersion,
		TemplateRef:       templateRef,
		ActiveRevision:    template.Current.Revision,
		SourceFingerprint: sourceFingerprint,
		TargetFingerprint: targetFingerprint,
		SourceSchema:      WorkspaceTemplatePolicyAlphaSchemaVersion,
		TargetSchema:      WorkspaceTemplatePolicySchemaVersion,
	}
	digest, err := semanticIdentity(workspaceTemplatePolicyMigrationPlanAuthority(plan))
	if err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	plan.PlanRef = strings.Join([]string{
		workspaceTemplatePolicyMigrationPlanRefPrefix + string(template.ID),
		strings.TrimPrefix(string(plan.ActiveRevision), "sha256:"),
		plan.SourceFingerprint,
		plan.TargetFingerprint,
		strings.TrimPrefix(string(digest), "sha256:"),
	}, "_")
	return plan, plan.Validate()
}

func validSourceFingerprint(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func ParseWorkspaceTemplatePolicyMigrationPlanRef(reference string) (WorkspaceTemplateID, error) {
	if !strings.HasPrefix(reference, workspaceTemplatePolicyMigrationPlanRefPrefix) {
		return "", fmt.Errorf("Workspace Template policy migration plan reference is invalid")
	}
	parts := strings.Split(strings.TrimPrefix(reference, workspaceTemplatePolicyMigrationPlanRefPrefix), "_")
	if len(parts) != 5 {
		return "", fmt.Errorf("Workspace Template policy migration plan reference is invalid")
	}
	id, err := ParseWorkspaceTemplateID(parts[0])
	if err != nil {
		return "", fmt.Errorf("Workspace Template policy migration plan reference is invalid")
	}
	for _, value := range parts[1:] {
		decoded, decodeErr := hex.DecodeString(value)
		if decodeErr != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != value {
			return "", fmt.Errorf("Workspace Template policy migration plan reference is invalid")
		}
	}
	return id, nil
}

func WorkspaceTemplatePolicyMigrationPlanFromRef(reference string) (WorkspaceTemplatePolicyMigrationPlan, error) {
	id, err := ParseWorkspaceTemplatePolicyMigrationPlanRef(reference)
	if err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	parts := strings.Split(strings.TrimPrefix(reference, workspaceTemplatePolicyMigrationPlanRefPrefix), "_")
	templateRef, _ := WorkspaceTemplateRef(id)
	plan := WorkspaceTemplatePolicyMigrationPlan{
		SchemaVersion:     WorkspaceTemplatePolicyMigrationPlanSchemaVersion,
		PlanRef:           reference,
		TemplateRef:       templateRef,
		ActiveRevision:    SemanticDigest("sha256:" + parts[1]),
		SourceFingerprint: parts[2],
		TargetFingerprint: parts[3],
		SourceSchema:      WorkspaceTemplatePolicyAlphaSchemaVersion,
		TargetSchema:      WorkspaceTemplatePolicySchemaVersion,
	}
	if err := plan.Validate(); err != nil {
		return WorkspaceTemplatePolicyMigrationPlan{}, err
	}
	return plan, nil
}

func (p WorkspaceTemplatePolicyMigrationPlan) Validate() error {
	if p.SchemaVersion != WorkspaceTemplatePolicyMigrationPlanSchemaVersion || p.SourceSchema != WorkspaceTemplatePolicyAlphaSchemaVersion || p.TargetSchema != WorkspaceTemplatePolicySchemaVersion || !validSourceFingerprint(p.SourceFingerprint) || !validSourceFingerprint(p.TargetFingerprint) || p.SourceFingerprint == p.TargetFingerprint || p.ActiveRevision.Validate() != nil {
		return fmt.Errorf("Template policy migration plan authority is invalid")
	}
	id, err := ParseWorkspaceTemplatePolicyMigrationPlanRef(p.PlanRef)
	if err != nil {
		return err
	}
	parsed, err := ParseWorkspaceTemplateRef(p.TemplateRef)
	if err != nil || parsed != id {
		return fmt.Errorf("Template policy migration plan target is invalid")
	}
	copy := p
	copy.PlanRef = ""
	digest, err := semanticIdentity(workspaceTemplatePolicyMigrationPlanAuthority(copy))
	if err != nil {
		return err
	}
	want := strings.Join([]string{
		workspaceTemplatePolicyMigrationPlanRefPrefix + string(id),
		strings.TrimPrefix(string(p.ActiveRevision), "sha256:"),
		p.SourceFingerprint,
		p.TargetFingerprint,
		strings.TrimPrefix(string(digest), "sha256:"),
	}, "_")
	if p.PlanRef != want {
		return fmt.Errorf("Template policy migration plan reference does not match authority")
	}
	return nil
}

type WorkspaceTemplatePolicyMigrationResult struct {
	TemplateID        WorkspaceTemplateID `json:"workspace_template_id"`
	TemplateRef       string              `json:"template_ref"`
	ActiveRevision    SemanticDigest      `json:"active_revision"`
	SourceFingerprint string              `json:"source_fingerprint"`
	Changed           bool                `json:"changed"`
}

func (r WorkspaceTemplatePolicyMigrationResult) Validate() error {
	id, err := ParseWorkspaceTemplateRef(r.TemplateRef)
	if err != nil || id != r.TemplateID || r.ActiveRevision.Validate() != nil || !validSourceFingerprint(r.SourceFingerprint) {
		return fmt.Errorf("Template policy migration result is invalid")
	}
	return nil
}

const WorkspaceTemplateChangePlanSchemaVersion = 1

type WorkspaceTemplateChangeImpact string

const (
	WorkspaceTemplateChangeWidening WorkspaceTemplateChangeImpact = "widening"
	WorkspaceTemplateChangeReducing WorkspaceTemplateChangeImpact = "reducing"
	WorkspaceTemplateChangeMixed    WorkspaceTemplateChangeImpact = "mixed"
	WorkspaceTemplateChangeNoOp     WorkspaceTemplateChangeImpact = "no-op"
)

type WorkspaceTemplateChangeDiff struct {
	Name              bool `json:"name"`
	Boundary          bool `json:"boundary"`
	SemanticPolicy    bool `json:"semantic_policy"`
	Runtime           bool `json:"runtime"`
	SessionDefaults   bool `json:"session_defaults"`
	WorkspaceDefaults bool `json:"workspace_defaults"`
}

type WorkspaceTemplateChangeContext struct {
	ContextRef           string         `json:"context_ref"`
	PolicyMemoryRevision SemanticDigest `json:"policy_memory_revision"`
	WorkspaceRef         string         `json:"workspace_ref,omitempty"`
	WorkspaceRunning     bool           `json:"workspace_running"`
}

// WorkspaceTemplateChangePlan is a read-only, content-addressed review of one
// exact desired source pair against one exact active authority envelope.
type WorkspaceTemplateChangePlan struct {
	SchemaVersion          int                              `json:"schema_version"`
	PlanRef                string                           `json:"plan_ref"`
	TemplateRef            string                           `json:"template_ref"`
	ActiveRevision         *SemanticDigest                  `json:"active_revision,omitempty"`
	ActiveMetadataRevision *SemanticDigest                  `json:"active_metadata_revision,omitempty"`
	BaseRevision           *SemanticDigest                  `json:"base_revision,omitempty"`
	SourceFingerprint      string                           `json:"source_fingerprint"`
	SourceRevision         SemanticDigest                   `json:"source_revision"`
	Impact                 WorkspaceTemplateChangeImpact    `json:"impact"`
	Diff                   WorkspaceTemplateChangeDiff      `json:"diff"`
	Contexts               []WorkspaceTemplateChangeContext `json:"contexts"`
	AffectedContextCount   int                              `json:"affected_context_count"`
	RunningWorkspaceCount  int                              `json:"running_workspace_count"`
}

type workspaceTemplateChangePlanAuthority WorkspaceTemplateChangePlan

func NewWorkspaceTemplateChangePlan(collection WorkspaceAuthorityCollection, id WorkspaceTemplateID, source WorkspaceTemplateSource, resolved RuntimeBinding, runningWorkspaces map[WorkspaceID]bool, fingerprint string) (WorkspaceTemplateChangePlan, error) {
	if err := collection.Validate(); err != nil {
		return WorkspaceTemplateChangePlan{}, err
	}
	if err := source.Validate(); err != nil || source.Template.TemplateID != id {
		return WorkspaceTemplateChangePlan{}, fmt.Errorf("Template change source is invalid: %w", err)
	}
	decoded, err := hex.DecodeString(fingerprint)
	if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != fingerprint {
		return WorkspaceTemplateChangePlan{}, fmt.Errorf("Template source fingerprint is invalid")
	}
	var template *WorkspaceTemplate
	for index := range collection.Templates {
		if collection.Templates[index].ID == id {
			value := collection.Templates[index].Clone()
			template = &value
			break
		}
	}
	for _, existing := range collection.Templates {
		if existing.ID != id && existing.Name == source.Template.Name {
			return WorkspaceTemplateChangePlan{}, ErrWorkspaceTemplateExists
		}
	}
	if template == nil {
		if source.Template.BaseRevision != nil {
			return WorkspaceTemplateChangePlan{}, errors.Join(ErrResourceSourceModified, fmt.Errorf("draft Template source base revision must be null"))
		}
	} else if err := source.ValidateFor(*template); err != nil {
		return WorkspaceTemplateChangePlan{}, errors.Join(ErrResourceSourceModified, err)
	}
	body, err := source.Body(resolved)
	if err != nil {
		return WorkspaceTemplateChangePlan{}, err
	}
	sourceRevision, err := source.SemanticRevision(resolved)
	if err != nil {
		return WorkspaceTemplateChangePlan{}, err
	}
	templateRef, _ := WorkspaceTemplateRef(id)
	diff := WorkspaceTemplateChangeDiff{Name: true, Boundary: true, SemanticPolicy: true, Runtime: true, SessionDefaults: true, WorkspaceDefaults: true}
	if template != nil {
		diff = workspaceTemplateChangeDiff(template.Clone(), body, source.Template.Name)
	}
	contexts := make([]WorkspaceTemplateChangeContext, 0)
	running := 0
	workspaceByContext := make(map[ContextID]WorkspaceBinding, len(collection.Workspaces))
	for _, workspace := range collection.Workspaces {
		workspaceByContext[workspace.ContextID] = workspace
	}
	for _, record := range collection.Contexts {
		if record.Context.TemplateID != id {
			continue
		}
		contextRef, _ := ContextRef(record.Context.ID)
		item := WorkspaceTemplateChangeContext{ContextRef: contextRef, PolicyMemoryRevision: record.PolicyMemory.Revision}
		if workspace, ok := workspaceByContext[record.Context.ID]; ok {
			item.WorkspaceRef, _ = WorkspaceRef(workspace.ID)
			item.WorkspaceRunning = runningWorkspaces[workspace.ID]
			if item.WorkspaceRunning {
				running++
			}
		}
		contexts = append(contexts, item)
	}
	impact := WorkspaceTemplateChangeMixed
	if template != nil {
		impact = classifyWorkspaceTemplateChange(template.Current.Body, body, template.Name != source.Template.Name, diff)
	}
	plan := WorkspaceTemplateChangePlan{
		SchemaVersion: WorkspaceTemplateChangePlanSchemaVersion, TemplateRef: templateRef,
		BaseRevision:      source.Template.BaseRevision,
		SourceFingerprint: fingerprint, SourceRevision: sourceRevision, Impact: impact, Diff: diff,
		Contexts: contexts, AffectedContextCount: len(contexts), RunningWorkspaceCount: running,
	}
	if template != nil {
		plan.ActiveRevision = semanticDigestPointer(template.Current.Revision)
		metadata, _ := semanticIdentity(struct{ Name string }{template.Name})
		plan.ActiveMetadataRevision = semanticDigestPointer(metadata)
	}
	digest, err := semanticIdentity(workspaceTemplateChangePlanAuthority(plan))
	if err != nil {
		return WorkspaceTemplateChangePlan{}, err
	}
	plan.PlanRef = workspaceTemplateChangePlanRefPrefix + string(id) + "_" + strings.TrimPrefix(string(digest), "sha256:")
	return plan, plan.Validate()
}

func workspaceTemplateChangeDiff(template WorkspaceTemplate, next WorkspaceTemplateBody, nextName string) WorkspaceTemplateChangeDiff {
	current := template.Current.Body
	return WorkspaceTemplateChangeDiff{
		Name:              template.Name != nextName,
		Boundary:          !reflect.DeepEqual(current.Boundary, next.Boundary),
		SemanticPolicy:    !reflect.DeepEqual(current.Policy, next.Policy),
		Runtime:           !reflect.DeepEqual(current.EntryDefaults.Runtime, next.EntryDefaults.Runtime),
		SessionDefaults:   !reflect.DeepEqual(current.SessionDefaults, next.SessionDefaults),
		WorkspaceDefaults: !reflect.DeepEqual(current.CreationDefaults, next.CreationDefaults),
	}
}

func classifyWorkspaceTemplateChange(current, next WorkspaceTemplateBody, nameChanged bool, diff WorkspaceTemplateChangeDiff) WorkspaceTemplateChangeImpact {
	if !nameChanged && !diff.Boundary && !diff.SemanticPolicy && !diff.Runtime && !diff.SessionDefaults && !diff.WorkspaceDefaults {
		return WorkspaceTemplateChangeNoOp
	}
	// Source access is the one current boundary axis with a total, mechanically
	// provable ordering. All other authority changes are conservatively mixed;
	// presentation must never guess widening from collection size or order.
	other := diff.SemanticPolicy || diff.Runtime || diff.SessionDefaults || diff.WorkspaceDefaults || nameChanged ||
		!reflect.DeepEqual(current.Boundary.DestinationCeiling, next.Boundary.DestinationCeiling) ||
		!reflect.DeepEqual(current.Boundary.MethodPolicy, next.Boundary.MethodPolicy)
	if !other && current.Boundary.SourceAccess == ManifestSourceAccessReadOnly && next.Boundary.SourceAccess == ManifestSourceAccessReadWrite {
		return WorkspaceTemplateChangeWidening
	}
	if !other && current.Boundary.SourceAccess == ManifestSourceAccessReadWrite && next.Boundary.SourceAccess == ManifestSourceAccessReadOnly {
		return WorkspaceTemplateChangeReducing
	}
	return WorkspaceTemplateChangeMixed
}

func ParseWorkspaceTemplateChangePlanRef(reference string) (WorkspaceTemplateID, error) {
	if !strings.HasPrefix(reference, workspaceTemplateChangePlanRefPrefix) {
		return "", fmt.Errorf("Workspace Template change plan reference is invalid")
	}
	value := strings.TrimPrefix(reference, workspaceTemplateChangePlanRefPrefix)
	separator := strings.LastIndexByte(value, '_')
	if separator <= 0 || len(value)-separator-1 != 64 {
		return "", fmt.Errorf("Workspace Template change plan reference is invalid")
	}
	id, err := ParseWorkspaceTemplateID(value[:separator])
	if err != nil {
		return "", fmt.Errorf("Workspace Template change plan reference is invalid")
	}
	if _, err := hex.DecodeString(value[separator+1:]); err != nil {
		return "", fmt.Errorf("Workspace Template change plan reference is invalid")
	}
	return id, nil
}

func (p WorkspaceTemplateChangePlan) Validate() error {
	if p.SchemaVersion != WorkspaceTemplateChangePlanSchemaVersion {
		return fmt.Errorf("Template change plan schema is invalid")
	}
	id, err := ParseWorkspaceTemplateChangePlanRef(p.PlanRef)
	if err != nil {
		return err
	}
	templateID, err := ParseWorkspaceTemplateRef(p.TemplateRef)
	if err != nil || templateID != id {
		return fmt.Errorf("Template change plan target is inconsistent")
	}
	if (p.ActiveRevision == nil) != (p.BaseRevision == nil) {
		return fmt.Errorf("Template change plan active/base authority is inconsistent")
	}
	if p.ActiveRevision != nil {
		if err := p.ActiveRevision.Validate(); err != nil || *p.BaseRevision != *p.ActiveRevision {
			return fmt.Errorf("Template change plan active/base authority is inconsistent")
		}
	}
	if p.ActiveRevision != nil && (p.ActiveMetadataRevision == nil || p.ActiveMetadataRevision.Validate() != nil) {
		return fmt.Errorf("Template change plan metadata authority is invalid")
	}
	if p.ActiveRevision == nil && p.ActiveMetadataRevision != nil {
		return fmt.Errorf("draft Template plan claims active metadata")
	}
	if err := p.SourceRevision.Validate(); err != nil || p.AffectedContextCount != len(p.Contexts) || p.RunningWorkspaceCount < 0 || p.RunningWorkspaceCount > len(p.Contexts) {
		return fmt.Errorf("Template change plan evidence is invalid")
	}
	switch p.Impact {
	case WorkspaceTemplateChangeWidening, WorkspaceTemplateChangeReducing, WorkspaceTemplateChangeMixed, WorkspaceTemplateChangeNoOp:
	default:
		return fmt.Errorf("Template change plan impact is invalid")
	}
	observedRunning := 0
	for index, item := range p.Contexts {
		if _, err := ParseContextRef(item.ContextRef); err != nil || item.PolicyMemoryRevision.Validate() != nil {
			return fmt.Errorf("Template change plan Context evidence is invalid")
		}
		if item.WorkspaceRef != "" {
			if _, err := ParseWorkspaceRef(item.WorkspaceRef); err != nil {
				return fmt.Errorf("Template change plan Workspace evidence is invalid")
			}
		} else if item.WorkspaceRunning {
			return fmt.Errorf("Template change plan claims a running absent Workspace")
		}
		if item.WorkspaceRunning {
			observedRunning++
		}
		if index > 0 && p.Contexts[index-1].ContextRef >= item.ContextRef {
			return fmt.Errorf("Template change plan Context evidence is not canonical")
		}
	}
	if observedRunning != p.RunningWorkspaceCount {
		return fmt.Errorf("Template change plan running Workspace evidence is inconsistent")
	}
	copy := p
	copy.PlanRef = ""
	digest, err := semanticIdentity(workspaceTemplateChangePlanAuthority(copy))
	if err != nil {
		return err
	}
	want := workspaceTemplateChangePlanRefPrefix + string(id) + "_" + strings.TrimPrefix(string(digest), "sha256:")
	if p.PlanRef != want {
		return fmt.Errorf("Template change plan reference does not match authority")
	}
	return nil
}

func (o ResourceSourceObservation) Validate() error {
	if o.Path == "" || o.Path[0] != '/' {
		return fmt.Errorf("resource source path must be absolute")
	}
	if err := o.State.Validate(); err != nil {
		return err
	}
	if o.ActiveRevision != nil {
		if err := o.ActiveRevision.Validate(); err != nil {
			return err
		}
	}
	if o.SourceRevision != nil {
		if err := o.SourceRevision.Validate(); err != nil {
			return err
		}
	}
	if o.State == ResourceSourceInSync && (o.SourceRevision == nil || o.ActiveRevision == nil || *o.SourceRevision != *o.ActiveRevision) {
		return fmt.Errorf("current resource source identity is inconsistent")
	}
	if o.State == ResourceSourceModified && o.SourceRevision == nil {
		return fmt.Errorf("modified resource source identity is inconsistent")
	}
	if (o.State == ResourceSourceMissing || o.State == ResourceSourceInvalid) && o.SourceRevision != nil {
		return fmt.Errorf("unreadable resource source cannot claim an identity")
	}
	return nil
}
