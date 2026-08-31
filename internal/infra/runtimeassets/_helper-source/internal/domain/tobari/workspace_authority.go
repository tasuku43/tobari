package tobari

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	WorkspaceTemplateSchemaVersion = 2
	ContextBindingSchemaVersion    = 2
	PolicyMemorySchemaVersion      = 2
	WorkspaceBindingSchemaVersion  = 3

	WorkspaceTemplateReferenceKind                    = "workspace-template"
	WorkspaceTemplateRevisionReferenceKind            = "workspace-template-revision"
	WorkspaceTemplateChangePlanReferenceKind          = "workspace-template-change-plan"
	WorkspaceTemplatePolicyMigrationPlanReferenceKind = "workspace-template-policy-migration-plan"
	ContextReferenceKind                              = "context"
	ContextActivationPlanReferenceKind                = "context-activation-plan"
	WorkspaceReferenceKind                            = "workspace"
	WorkspaceTemplateCatalogTargetKind                = "workspace-template-catalog"
	WorkspaceTemplateCatalogTargetID                  = "workspace-template-catalog"
)

var (
	ErrWorkspaceTemplateExists                = errors.New("Workspace Template already exists")
	ErrWorkspaceTemplateNotFound              = errors.New("Workspace Template does not exist")
	ErrWorkspaceTemplateRevisionNotFound      = errors.New("Workspace Template revision does not exist")
	ErrWorkspaceTemplateProtected             = errors.New("Workspace Template is still referenced")
	ErrDefaultTemplateSelectionRequired       = errors.New("default Workspace Template selection is required")
	ErrPreReleaseLegacyAuthority              = errors.New("pre-release legacy authority is present or unsafe")
	ErrFinalAuthorityMigrationRequired        = errors.New("final authority store migration is required")
	ErrFinalAuthorityNotFound                 = errors.New("final authority is not initialized")
	ErrLegacyExecutablePolicy                 = errors.New("legacy executable policy is unsupported")
	ErrContextBindingExists                   = errors.New("Context already exists")
	ErrContextBindingNotFound                 = errors.New("Context does not exist")
	ErrContextBindingProtected                = errors.New("Context still owns live authority")
	ErrWorkspaceBindingNotFound               = errors.New("Workspace does not exist")
	ErrWorkspaceBindingProtected              = errors.New("Workspace still has a live attachment")
	ErrWorkspaceEntryReconciliationConfirmed  = errors.New("Workspace entry reconciliation is confirmed but the interactive attachment did not start")
	ErrWorkspaceEntryTemplatePolicyInactive   = errors.New("Workspace entry requires current Template policy activation")
	ErrWorkspaceEntryPolicyMemoryInactive     = errors.New("Workspace entry requires current Policy Memory activation")
	ErrWorkspaceEntryObservationUnavailable   = errors.New("Workspace entry precondition observation is unavailable")
	ErrWorkspaceRuntimePreparationUncertain   = errors.New("Workspace Runtime preparation may have changed local Docker state")
	ErrWorkspaceEntryInterrupted              = errors.New("Workspace entry reconciliation requires exact recovery")
	ErrWorkspaceEntryCanceledBeforeDecision   = errors.New("Workspace entry was canceled before a durable reconciliation decision")
	ErrWorkspaceEntryRuntimeNotCurrent        = errors.New("Workspace entry runtime is confirmed missing or mismatched")
	ErrWorkspaceEntryProtectionNotCurrent     = errors.New("Workspace entry protection is confirmed missing, stopped, or drifted")
	ErrFinalAuthorityMutationRecoveryRequired = errors.New("final-authority mutation requires exact recovery")
	ErrPolicyMemoryTargetNotFound             = errors.New("Policy Memory target does not exist")
	ErrPolicyReviewChanged                    = errors.New("reviewed Policy Memory collection changed")
)

const (
	workspaceTemplateRefPrefix                    = "wtpl1_"
	workspaceTemplateRevisionRefPrefix            = "wtrev1_"
	workspaceTemplateChangePlanRefPrefix          = "wtplan1_"
	workspaceTemplatePolicyMigrationPlanRefPrefix = "wtpmplan1_"
	contextRefPrefix                              = "ctx1_"
	contextActivationPlanRefPrefix                = "ctxplan1_"
	workspaceRefPrefix                            = "wsp1_"
)

// WorkspaceTemplateID, ContextID, and WorkspaceID are distinct V1 authority
// types. Their byte grammar is shared, but a value cannot cross owners without
// an explicit checked conversion at the migration boundary.
type WorkspaceTemplateID string
type ContextID string
type WorkspaceID string

func (id WorkspaceTemplateID) Validate() error {
	return validateAuthorityUUID("Workspace Template", string(id))
}
func (id ContextID) Validate() error   { return validateAuthorityUUID("Context", string(id)) }
func (id WorkspaceID) Validate() error { return validateAuthorityUUID("Workspace", string(id)) }

func ParseWorkspaceTemplateID(value string) (WorkspaceTemplateID, error) {
	id := WorkspaceTemplateID(value)
	return id, id.Validate()
}

func ParseContextID(value string) (ContextID, error) {
	id := ContextID(value)
	return id, id.Validate()
}

func ParseWorkspaceID(value string) (WorkspaceID, error) {
	id := WorkspaceID(value)
	return id, id.Validate()
}

func IssueWorkspaceTemplateID(now time.Time, source io.Reader) (WorkspaceTemplateID, error) {
	value, err := issueAuthorityUUID("Workspace Template", now, source)
	return WorkspaceTemplateID(value), err
}

func IssueContextID(now time.Time, source io.Reader) (ContextID, error) {
	value, err := issueAuthorityUUID("Context", now, source)
	return ContextID(value), err
}

func IssueWorkspaceID(now time.Time, source io.Reader) (WorkspaceID, error) {
	value, err := issueAuthorityUUID("Workspace", now, source)
	return WorkspaceID(value), err
}

func validateAuthorityUUID(kind, value string) error {
	if !contextIDPattern.MatchString(value) {
		return fmt.Errorf("%s ID is invalid", kind)
	}
	return nil
}

func issueAuthorityUUID(kind string, now time.Time, source io.Reader) (string, error) {
	if source == nil {
		return "", fmt.Errorf("%s ID entropy source is required", kind)
	}
	if now.UnixMilli() < 0 || now.UnixMilli() >= 1<<48 {
		return "", fmt.Errorf("%s ID timestamp is outside UUIDv7 range", kind)
	}
	var value [16]byte
	milliseconds := uint64(now.UnixMilli())
	for index := 5; index >= 0; index-- {
		value[index] = byte(milliseconds)
		milliseconds >>= 8
	}
	if _, err := io.ReadFull(source, value[6:]); err != nil {
		return "", fmt.Errorf("read %s ID entropy: %w", kind, err)
	}
	value[6] = 0x70 | (value[6] & 0x0f)
	value[8] = 0x80 | (value[8] & 0x3f)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

// SemanticDigest is a complete sha256 semantic identity. Field ownership,
// rather than this shared byte grammar, decides whether two digests are
// interchangeable.
type SemanticDigest string

func (d SemanticDigest) Validate() error { return ValidateDigest(string(d)) }

// PolicyEvaluatorIdentity identifies the fixed Tobari-owned evaluator that
// interprets canonical policy data. It is deliberately independent from the
// policy-data identity below: a remembered decision must not imply a new
// evaluator, while an evaluator update must remain visible when data is
// unchanged.
type PolicyEvaluatorIdentity struct {
	SchemaVersion int            `json:"schema_version"`
	Version       string         `json:"version"`
	Digest        SemanticDigest `json:"digest"`
}

func (i PolicyEvaluatorIdentity) Validate() error {
	if i.SchemaVersion != 1 || i.Version == "" || strings.ContainsAny(i.Version, " \t\r\n") {
		return fmt.Errorf("policy evaluator identity metadata is invalid")
	}
	if err := i.Digest.Validate(); err != nil {
		return fmt.Errorf("policy evaluator identity digest: %w", err)
	}
	return nil
}

// PolicyDataIdentity identifies the complete canonical typed data projected
// into one aggregate. It excludes evaluator bytes and is carried alongside
// the aggregate revision and active publication receipt.
type PolicyDataIdentity struct {
	SchemaVersion int            `json:"schema_version"`
	Digest        SemanticDigest `json:"digest"`
}

func (i PolicyDataIdentity) Validate() error {
	if i.SchemaVersion != 1 {
		return fmt.Errorf("policy data identity metadata is invalid")
	}
	if err := i.Digest.Validate(); err != nil {
		return fmt.Errorf("policy data identity digest: %w", err)
	}
	return nil
}

func semanticIdentity(value any) (SemanticDigest, error) {
	digest, err := semanticDigest(value)
	return SemanticDigest(digest), err
}

// WorkspaceTemplateBoundary is the complete immutable source/network ceiling
// fixed by one WorkspaceTemplateID. Baseline and remembered policy may narrow
// it, but no revision under the same ID can change it.
type WorkspaceTemplateBoundary struct {
	SourceAccess       ManifestSourceAccess             `json:"source_access"`
	DestinationCeiling ManifestPolicyDestinationCeiling `json:"destination_ceiling"`
	MethodPolicy       ManifestMethodPolicy             `json:"method_policy"`
}

func (b WorkspaceTemplateBoundary) Validate() error {
	if err := b.SourceAccess.Validate(); err != nil {
		return err
	}
	policy := ManifestPolicy{
		SchemaVersion: ManifestPolicySchemaVersion, Name: "workspace-template-boundary",
		DestinationCeiling: b.DestinationCeiling, MethodPolicy: b.MethodPolicy,
		BaselineGrants: []ManifestPolicyExactRule{}, BaselineTemplates: []ManifestPolicyPathTemplateRule{},
		MCPBaselineGrants: []ManifestPolicyMCPRule{}, BaselineDenies: []ManifestPolicyExactRule{},
		GraphQLEndpoints: []ManifestPolicyExactRule{}, MCPEndpoints: []ManifestPolicyExactRule{},
	}
	return policy.Validate()
}

func (b WorkspaceTemplateBoundary) Clone() WorkspaceTemplateBoundary {
	result := b
	result.DestinationCeiling.Authorities = append([]ManifestPolicyAuthority{}, b.DestinationCeiling.Authorities...)
	result.MethodPolicy = b.MethodPolicy.Clone()
	return result
}

// WorkspaceTemplatePolicyBody is the complete static baseline. Terminal
// destination and method ceilings live only in Boundary and are supplied for
// validation rather than duplicated here.
type WorkspaceTemplatePolicyBody struct {
	AgentProfile      string                            `json:"agent_profile"`
	NativeReadiness   ManifestNativeReadiness           `json:"native_readiness"`
	SemanticModules   *WorkspaceTemplateSemanticModules `json:"semantic_modules,omitempty"`
	BaselineGrants    []ManifestPolicyExactRule         `json:"baseline_grants"`
	BaselineTemplates []ManifestPolicyPathTemplateRule  `json:"baseline_templates"`
	MCPBaselineGrants []ManifestPolicyMCPRule           `json:"mcp_baseline_grants"`
	BaselineDenies    []ManifestPolicyExactRule         `json:"baseline_denies"`
	GraphQLEndpoints  []ManifestPolicyExactRule         `json:"graphql_endpoints"`
	MCPEndpoints      []ManifestPolicyExactRule         `json:"mcp_endpoints"`
}

func (p WorkspaceTemplatePolicyBody) Validate(boundary WorkspaceTemplateBoundary) error {
	if err := boundary.Validate(); err != nil {
		return err
	}
	if err := ValidateName(p.AgentProfile); err != nil {
		return fmt.Errorf("Template agent profile: %w", err)
	}
	if err := p.NativeReadiness.Validate(); err != nil {
		return err
	}
	if p.SemanticModules != nil {
		if len(p.BaselineGrants) != 0 || len(p.BaselineTemplates) != 0 || len(p.MCPBaselineGrants) != 0 || len(p.BaselineDenies) != 0 || len(p.GraphQLEndpoints) != 0 || len(p.MCPEndpoints) != 0 {
			return fmt.Errorf("Template policy cannot mix final semantic modules with predecessor static fields")
		}
		deniedMethods := []string{}
		for _, override := range boundary.MethodPolicy.Overrides {
			if override.Decision == ManifestMethodDeny {
				deniedMethods = append(deniedMethods, override.Method)
			}
		}
		if err := p.SemanticModules.Validate(deniedMethods); err != nil {
			return err
		}
	}
	policy := ManifestPolicy{
		SchemaVersion: ManifestPolicySchemaVersion, Name: "workspace-template",
		DestinationCeiling: boundary.DestinationCeiling, MethodPolicy: boundary.MethodPolicy,
		BaselineGrants: p.BaselineGrants, BaselineTemplates: p.BaselineTemplates,
		MCPBaselineGrants: p.MCPBaselineGrants, BaselineDenies: p.BaselineDenies,
		GraphQLEndpoints: p.GraphQLEndpoints, MCPEndpoints: p.MCPEndpoints,
	}
	return policy.Validate()
}

func (p WorkspaceTemplatePolicyBody) Clone() WorkspaceTemplatePolicyBody {
	result := p
	if p.SemanticModules != nil {
		modules := p.SemanticModules.Clone()
		result.SemanticModules = &modules
	}
	result.BaselineGrants = append([]ManifestPolicyExactRule{}, p.BaselineGrants...)
	result.BaselineTemplates = append([]ManifestPolicyPathTemplateRule{}, p.BaselineTemplates...)
	for index := range result.BaselineTemplates {
		result.BaselineTemplates[index].Segments = append([]string{}, p.BaselineTemplates[index].Segments...)
	}
	result.MCPBaselineGrants = append([]ManifestPolicyMCPRule{}, p.MCPBaselineGrants...)
	result.BaselineDenies = append([]ManifestPolicyExactRule{}, p.BaselineDenies...)
	result.GraphQLEndpoints = append([]ManifestPolicyExactRule{}, p.GraphQLEndpoints...)
	result.MCPEndpoints = append([]ManifestPolicyExactRule{}, p.MCPEndpoints...)
	return result
}

type WorkspaceTemplateEntryDefaults struct {
	Runtime RuntimeBinding `json:"runtime"`
}

func (e WorkspaceTemplateEntryDefaults) Validate() error { return e.Runtime.Validate() }

type WorkspaceTemplateSessionDefaults struct {
	ShellEnvironment []ManifestShellEnvironmentSetting `json:"shell_environment"`
	GitIdentity      *ManifestGitIdentitySetting       `json:"git_identity,omitempty"`
}

func (s WorkspaceTemplateSessionDefaults) Validate() error {
	if s.ShellEnvironment == nil {
		return fmt.Errorf("Template shell environment collection is unknown")
	}
	if err := validateContextShellEnvironment(s.ShellEnvironment, false); err != nil {
		return err
	}
	if s.GitIdentity != nil {
		return s.GitIdentity.Validate(false)
	}
	return nil
}

func (s WorkspaceTemplateSessionDefaults) Clone() WorkspaceTemplateSessionDefaults {
	result := s
	result.ShellEnvironment = cloneContextShellEnvironment(s.ShellEnvironment)
	if s.GitIdentity != nil {
		git := cloneContextGitIdentitySetting(*s.GitIdentity)
		result.GitIdentity = &git
	}
	return result
}

type WorkspaceTemplateCreationDefaults struct {
	Bootstrap *ManifestBootstrapSnapshot `json:"bootstrap,omitempty"`
}

func (c WorkspaceTemplateCreationDefaults) Validate() error {
	if c.Bootstrap != nil {
		return c.Bootstrap.Validate()
	}
	return nil
}

func (c WorkspaceTemplateCreationDefaults) Clone() WorkspaceTemplateCreationDefaults {
	result := c
	if c.Bootstrap != nil {
		bootstrap := c.Bootstrap.Clone()
		result.Bootstrap = &bootstrap
	}
	return result
}

// WorkspaceTemplateBody is the complete immutable static definition consumed
// after the hard cutover. No ordinary operation needs predecessor Manifest
// bytes or an untyped parallel body to derive policy, entry, session, or
// creation effects.
type WorkspaceTemplateBody struct {
	Boundary         WorkspaceTemplateBoundary         `json:"boundary"`
	Policy           WorkspaceTemplatePolicyBody       `json:"policy"`
	EntryDefaults    WorkspaceTemplateEntryDefaults    `json:"entry_defaults"`
	SessionDefaults  WorkspaceTemplateSessionDefaults  `json:"session_defaults"`
	CreationDefaults WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
}

func (b WorkspaceTemplateBody) Validate() error {
	if err := b.Boundary.Validate(); err != nil {
		return err
	}
	if err := b.Policy.Validate(b.Boundary); err != nil {
		return err
	}
	if err := b.EntryDefaults.Validate(); err != nil {
		return err
	}
	if err := b.SessionDefaults.Validate(); err != nil {
		return err
	}
	return b.CreationDefaults.Validate()
}

func (b WorkspaceTemplateBody) Clone() WorkspaceTemplateBody {
	return WorkspaceTemplateBody{
		Boundary: b.Boundary.Clone(), Policy: b.Policy.Clone(), EntryDefaults: b.EntryDefaults,
		SessionDefaults: b.SessionDefaults.Clone(), CreationDefaults: b.CreationDefaults.Clone(),
	}
}

func (b WorkspaceTemplateBody) slices() (WorkspaceTemplateSlices, error) {
	if err := b.Validate(); err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	boundary, err := semanticIdentity(b.Boundary)
	if err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	policy, err := semanticIdentity(struct {
		Boundary SemanticDigest
		Body     WorkspaceTemplatePolicyBody
	}{boundary, b.Policy})
	if err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	entry, err := semanticIdentity(b.EntryDefaults)
	if err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	session, err := semanticIdentity(b.SessionDefaults)
	if err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	creation, err := semanticIdentity(b.CreationDefaults)
	if err != nil {
		return WorkspaceTemplateSlices{}, err
	}
	return WorkspaceTemplateSlices{
		BoundaryFingerprint: boundary, PolicySliceDigest: policy, EntrySliceDigest: entry,
		SessionDefaultsDigest: session, CreationDefaultsDigest: creation,
		RuntimeID: b.EntryDefaults.Runtime.RuntimeID, RuntimeRevision: SemanticDigest(b.EntryDefaults.Runtime.Revision),
	}, nil
}

func WorkspaceTemplateRef(id WorkspaceTemplateID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	return workspaceTemplateRefPrefix + string(id), nil
}

func ParseWorkspaceTemplateRef(reference string) (WorkspaceTemplateID, error) {
	if !strings.HasPrefix(reference, workspaceTemplateRefPrefix) {
		return "", fmt.Errorf("Workspace Template reference is invalid")
	}
	id, err := ParseWorkspaceTemplateID(strings.TrimPrefix(reference, workspaceTemplateRefPrefix))
	if err != nil || reference != workspaceTemplateRefPrefix+string(id) {
		return "", fmt.Errorf("Workspace Template reference is invalid")
	}
	return id, nil
}

func WorkspaceTemplateRevisionRef(id WorkspaceTemplateID, revision SemanticDigest) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	if err := revision.Validate(); err != nil {
		return "", err
	}
	return workspaceTemplateRevisionRefPrefix + string(id) + "_" + strings.TrimPrefix(string(revision), "sha256:"), nil
}

func ParseWorkspaceTemplateRevisionRef(reference string) (WorkspaceTemplateID, SemanticDigest, error) {
	if !strings.HasPrefix(reference, workspaceTemplateRevisionRefPrefix) {
		return "", "", fmt.Errorf("Workspace Template revision reference is invalid")
	}
	value := strings.TrimPrefix(reference, workspaceTemplateRevisionRefPrefix)
	separator := strings.LastIndexByte(value, '_')
	if separator <= 0 || separator == len(value)-1 {
		return "", "", fmt.Errorf("Workspace Template revision reference is invalid")
	}
	id, idErr := ParseWorkspaceTemplateID(value[:separator])
	revision := SemanticDigest("sha256:" + value[separator+1:])
	if idErr != nil || revision.Validate() != nil {
		return "", "", fmt.Errorf("Workspace Template revision reference is invalid")
	}
	exact, _ := WorkspaceTemplateRevisionRef(id, revision)
	if reference != exact {
		return "", "", fmt.Errorf("Workspace Template revision reference is invalid")
	}
	return id, revision, nil
}

func ContextRef(id ContextID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	return contextRefPrefix + string(id), nil
}

func ParseContextRef(reference string) (ContextID, error) {
	if !strings.HasPrefix(reference, contextRefPrefix) {
		return "", fmt.Errorf("Context reference is invalid")
	}
	id, err := ParseContextID(strings.TrimPrefix(reference, contextRefPrefix))
	if err != nil || reference != contextRefPrefix+string(id) {
		return "", fmt.Errorf("Context reference is invalid")
	}
	return id, nil
}

func WorkspaceRef(id WorkspaceID) (string, error) {
	if err := id.Validate(); err != nil {
		return "", err
	}
	return workspaceRefPrefix + string(id), nil
}

func ParseWorkspaceRef(reference string) (WorkspaceID, error) {
	if !strings.HasPrefix(reference, workspaceRefPrefix) {
		return "", fmt.Errorf("Workspace reference is invalid")
	}
	id, err := ParseWorkspaceID(strings.TrimPrefix(reference, workspaceRefPrefix))
	if err != nil || reference != workspaceRefPrefix+string(id) {
		return "", fmt.Errorf("Workspace reference is invalid")
	}
	return id, nil
}

// WorkspaceTemplateSlices are the independently activated semantic parts of
// one complete immutable Template revision.
type WorkspaceTemplateSlices struct {
	BoundaryFingerprint    SemanticDigest `json:"boundary_fingerprint"`
	PolicySliceDigest      SemanticDigest `json:"policy_slice_digest"`
	EntrySliceDigest       SemanticDigest `json:"entry_slice_digest"`
	SessionDefaultsDigest  SemanticDigest `json:"session_defaults_digest"`
	CreationDefaultsDigest SemanticDigest `json:"creation_defaults_digest"`
	RuntimeID              string         `json:"runtime_id"`
	RuntimeRevision        SemanticDigest `json:"runtime_revision"`
}

func (s WorkspaceTemplateSlices) Validate() error {
	for name, digest := range map[string]SemanticDigest{
		"Boundary fingerprint": s.BoundaryFingerprint,
		"policy slice":         s.PolicySliceDigest,
		"entry slice":          s.EntrySliceDigest,
		"session defaults":     s.SessionDefaultsDigest,
		"creation defaults":    s.CreationDefaultsDigest,
		"Runtime revision":     s.RuntimeRevision,
	} {
		if err := digest.Validate(); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	if s.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(s.RuntimeID); err != nil {
			return fmt.Errorf("Template Runtime ID: %w", err)
		}
	}
	return nil
}

type WorkspaceTemplateRevision struct {
	SchemaVersion int                     `json:"schema_version"`
	TemplateID    WorkspaceTemplateID     `json:"workspace_template_id"`
	Generation    uint64                  `json:"generation"`
	Revision      SemanticDigest          `json:"revision"`
	Body          WorkspaceTemplateBody   `json:"body"`
	Slices        WorkspaceTemplateSlices `json:"slices"`
}

func NewWorkspaceTemplateRevision(id WorkspaceTemplateID, generation uint64, body WorkspaceTemplateBody) (WorkspaceTemplateRevision, error) {
	revision := WorkspaceTemplateRevision{SchemaVersion: WorkspaceTemplateSchemaVersion, TemplateID: id, Generation: generation, Body: body.Clone()}
	if err := id.Validate(); err != nil {
		return WorkspaceTemplateRevision{}, err
	}
	if generation == 0 {
		return WorkspaceTemplateRevision{}, fmt.Errorf("Template generation must be positive")
	}
	slices, err := revision.Body.slices()
	if err != nil {
		return WorkspaceTemplateRevision{}, err
	}
	revision.Slices = slices
	digest, err := semanticIdentity(revision.Body)
	if err != nil {
		return WorkspaceTemplateRevision{}, err
	}
	revision.Revision = digest
	return revision, revision.Validate()
}

func (r WorkspaceTemplateRevision) Validate() error {
	if r.SchemaVersion != WorkspaceTemplateSchemaVersion {
		return fmt.Errorf("Template revision schema version must be %d", WorkspaceTemplateSchemaVersion)
	}
	if err := r.TemplateID.Validate(); err != nil {
		return err
	}
	if r.Generation == 0 {
		return fmt.Errorf("Template generation must be positive")
	}
	if err := r.Body.Validate(); err != nil {
		return err
	}
	wantSlices, err := r.Body.slices()
	if err != nil {
		return err
	}
	if r.Slices != wantSlices {
		return fmt.Errorf("Template slices do not match the complete canonical body")
	}
	want, err := semanticIdentity(r.Body)
	if err != nil {
		return err
	}
	if r.Revision != want {
		return fmt.Errorf("Template revision does not match its complete canonical body")
	}
	return nil
}

func (r WorkspaceTemplateRevision) Clone() WorkspaceTemplateRevision {
	result := r
	result.Body = r.Body.Clone()
	return result
}

func AdvanceWorkspaceTemplateRevision(previous WorkspaceTemplateRevision, body WorkspaceTemplateBody) (WorkspaceTemplateRevision, bool, error) {
	if err := previous.Validate(); err != nil {
		return WorkspaceTemplateRevision{}, false, err
	}
	next, err := NewWorkspaceTemplateRevision(previous.TemplateID, previous.Generation+1, body)
	if err != nil {
		return WorkspaceTemplateRevision{}, false, err
	}
	if next.Revision == previous.Revision {
		return previous, false, nil
	}
	return next, true, nil
}

func ValidateWorkspaceTemplateHistory(revisions []WorkspaceTemplateRevision) error {
	if len(revisions) == 0 {
		return fmt.Errorf("Template revision history is empty")
	}
	for index, revision := range revisions {
		if err := revision.Validate(); err != nil {
			return err
		}
		if index == 0 {
			continue
		}
		previous := revisions[index-1]
		if revision.TemplateID != previous.TemplateID {
			return fmt.Errorf("Template history crosses identity")
		}
		if revision.Generation <= previous.Generation {
			return fmt.Errorf("Template history generations must increase")
		}
		if revision.Revision == previous.Revision {
			return fmt.Errorf("Template history contains a semantic no-op publication")
		}
	}
	return nil
}

type WorkspaceTemplate struct {
	SchemaVersion    int                                 `json:"schema_version"`
	ID               WorkspaceTemplateID                 `json:"workspace_template_id"`
	Name             string                              `json:"name"`
	Metadata         *WorkspaceTemplateMetadataRevision  `json:"metadata_revision,omitempty"`
	RetainedMetadata []WorkspaceTemplateMetadataRevision `json:"retained_metadata_revisions,omitempty"`
	Current          WorkspaceTemplateRevision           `json:"current"`
	Retained         []WorkspaceTemplateRevision         `json:"retained"`
}

type WorkspaceTemplateMetadataRevision struct {
	Generation uint64         `json:"generation"`
	Name       string         `json:"name"`
	Revision   SemanticDigest `json:"revision"`
}

func NewWorkspaceTemplateMetadataRevision(generation uint64, name string) (WorkspaceTemplateMetadataRevision, error) {
	if generation == 0 {
		return WorkspaceTemplateMetadataRevision{}, fmt.Errorf("Template metadata generation must be positive")
	}
	if err := ValidateName(name); err != nil {
		return WorkspaceTemplateMetadataRevision{}, err
	}
	revision, err := semanticIdentity(struct{ Name string }{name})
	if err != nil {
		return WorkspaceTemplateMetadataRevision{}, err
	}
	return WorkspaceTemplateMetadataRevision{Generation: generation, Name: name, Revision: revision}, nil
}

func (r WorkspaceTemplateMetadataRevision) Validate() error {
	want, err := NewWorkspaceTemplateMetadataRevision(r.Generation, r.Name)
	if err != nil || r.Revision != want.Revision {
		return fmt.Errorf("Template metadata revision is invalid")
	}
	return nil
}

func AdvanceWorkspaceTemplateMetadata(template *WorkspaceTemplate, name string) (bool, error) {
	if template == nil || ValidateName(name) != nil {
		return false, fmt.Errorf("Template metadata change is invalid")
	}
	if template.Name == name {
		return false, nil
	}
	if template.Metadata == nil {
		baseline, err := NewWorkspaceTemplateMetadataRevision(1, template.Name)
		if err != nil {
			return false, err
		}
		template.Metadata = &baseline
		template.RetainedMetadata = []WorkspaceTemplateMetadataRevision{baseline}
	}
	next, err := NewWorkspaceTemplateMetadataRevision(template.Metadata.Generation+1, name)
	if err != nil {
		return false, err
	}
	template.Name = name
	template.Metadata = &next
	template.RetainedMetadata = append(template.RetainedMetadata, next)
	return true, nil
}

func InitializeWorkspaceTemplateMetadata(template *WorkspaceTemplate) error {
	if template == nil {
		return fmt.Errorf("Template metadata owner is unavailable")
	}
	if template.Metadata != nil || template.RetainedMetadata != nil {
		return template.Validate()
	}
	metadata, err := NewWorkspaceTemplateMetadataRevision(1, template.Name)
	if err != nil {
		return err
	}
	template.Metadata = &metadata
	template.RetainedMetadata = []WorkspaceTemplateMetadataRevision{metadata}
	return nil
}

func (t WorkspaceTemplate) Validate() error {
	if t.SchemaVersion != WorkspaceTemplateSchemaVersion || t.ID.Validate() != nil || ValidateName(t.Name) != nil {
		return fmt.Errorf("Workspace Template identity is invalid")
	}
	if err := ValidateWorkspaceTemplateHistory(t.Retained); err != nil {
		return err
	}
	if t.Metadata != nil || t.RetainedMetadata != nil {
		if t.Metadata == nil || len(t.RetainedMetadata) == 0 {
			return fmt.Errorf("Template metadata history is incomplete")
		}
		for index, revision := range t.RetainedMetadata {
			if err := revision.Validate(); err != nil {
				return err
			}
			if index > 0 && (revision.Generation <= t.RetainedMetadata[index-1].Generation || revision.Revision == t.RetainedMetadata[index-1].Revision) {
				return fmt.Errorf("Template metadata history is invalid")
			}
		}
		latest := t.RetainedMetadata[len(t.RetainedMetadata)-1]
		if *t.Metadata != latest || latest.Name != t.Name {
			return fmt.Errorf("Template current metadata is not retained exactly")
		}
	}
	found := false
	for _, revision := range t.Retained {
		if revision.TemplateID != t.ID {
			return fmt.Errorf("Template retained revision owner is inconsistent")
		}
		if revision.Generation == t.Current.Generation && revision.Revision == t.Current.Revision {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("Template current revision is not retained exactly")
	}
	latest := t.Retained[len(t.Retained)-1]
	if t.Current.TemplateID != latest.TemplateID || t.Current.Generation != latest.Generation || t.Current.Revision != latest.Revision {
		return fmt.Errorf("Template current revision is not the latest retained generation")
	}
	if err := t.Current.Validate(); err != nil {
		return err
	}
	return nil
}

func (t WorkspaceTemplate) Clone() WorkspaceTemplate {
	result := t
	result.Current = t.Current.Clone()
	result.Retained = make([]WorkspaceTemplateRevision, len(t.Retained))
	if t.Metadata != nil {
		value := *t.Metadata
		result.Metadata = &value
	}
	if t.RetainedMetadata != nil {
		result.RetainedMetadata = append([]WorkspaceTemplateMetadataRevision{}, t.RetainedMetadata...)
	}
	for index := range t.Retained {
		result.Retained[index] = t.Retained[index].Clone()
	}
	return result
}

// ValidateWorkspaceTemplateAuthorities proves that one exhaustive observation
// does not assign contradictory content or names to installation-owned
// Template authority. Repeated snapshots may embed the same Template only when
// its complete validated value is identical.
func ValidateWorkspaceTemplateAuthorities(templates []WorkspaceTemplate) error {
	if templates == nil {
		return fmt.Errorf("Workspace Template authority collection is unknown")
	}
	byID := make(map[WorkspaceTemplateID]SemanticDigest, len(templates))
	byName := make(map[string]WorkspaceTemplateID, len(templates))
	for _, template := range templates {
		if err := template.Validate(); err != nil {
			return err
		}
		digest, err := semanticIdentity(template)
		if err != nil {
			return err
		}
		if previous, exists := byID[template.ID]; exists && previous != digest {
			return fmt.Errorf("one Workspace Template ID has contradictory authority")
		}
		if previous, exists := byName[template.Name]; exists && previous != template.ID {
			return fmt.Errorf("one Workspace Template name identifies multiple Templates")
		}
		byID[template.ID] = digest
		byName[template.Name] = template.ID
	}
	return nil
}

// CopyWorkspaceTemplateRevision creates a fresh independent Template at
// generation one from one exact immutable source revision. No lineage or
// source authority is retained in the result.
func CopyWorkspaceTemplateRevision(id WorkspaceTemplateID, name string, source WorkspaceTemplateRevision) (WorkspaceTemplate, error) {
	if err := source.Validate(); err != nil {
		return WorkspaceTemplate{}, err
	}
	if source.TemplateID == id {
		return WorkspaceTemplate{}, fmt.Errorf("Template copy requires a fresh Template ID")
	}
	if err := ValidateName(name); err != nil {
		return WorkspaceTemplate{}, err
	}
	revision, err := NewWorkspaceTemplateRevision(id, 1, source.Body)
	if err != nil {
		return WorkspaceTemplate{}, err
	}
	result := WorkspaceTemplate{
		SchemaVersion: WorkspaceTemplateSchemaVersion, ID: id, Name: name,
		Current: revision, Retained: []WorkspaceTemplateRevision{revision.Clone()},
	}
	if err := InitializeWorkspaceTemplateMetadata(&result); err != nil {
		return WorkspaceTemplate{}, err
	}
	return result, result.Validate()
}

// WorkspaceTemplateEntryAuthority is the complete static entry material
// derived from one unchanged Template revision. It lets context enter and
// cluster up consume final authority without consulting predecessor state.
type WorkspaceTemplateEntryAuthority struct {
	TemplateID       WorkspaceTemplateID               `json:"workspace_template_id"`
	TemplateRevision SemanticDigest                    `json:"workspace_template_revision"`
	EntrySliceDigest SemanticDigest                    `json:"entry_slice_digest"`
	SourceAccess     ManifestSourceAccess              `json:"source_access"`
	AgentProfile     string                            `json:"agent_profile"`
	Runtime          RuntimeBinding                    `json:"runtime"`
	SessionDefaults  WorkspaceTemplateSessionDefaults  `json:"session_defaults"`
	CreationDefaults WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
}

func (a WorkspaceTemplateEntryAuthority) ValidateFor(revision WorkspaceTemplateRevision) error {
	if err := revision.Validate(); err != nil {
		return err
	}
	if a.TemplateID != revision.TemplateID || a.TemplateRevision != revision.Revision || a.EntrySliceDigest != revision.Slices.EntrySliceDigest ||
		a.SourceAccess != revision.Body.Boundary.SourceAccess || a.AgentProfile != revision.Body.Policy.AgentProfile || a.Runtime != revision.Body.EntryDefaults.Runtime {
		return fmt.Errorf("Template entry authority does not bind its exact revision")
	}
	if err := a.SourceAccess.Validate(); err != nil {
		return err
	}
	if err := ValidateName(a.AgentProfile); err != nil {
		return fmt.Errorf("Template entry authority agent profile: %w", err)
	}
	if err := a.SessionDefaults.Validate(); err != nil {
		return err
	}
	if err := a.CreationDefaults.Validate(); err != nil {
		return err
	}
	session, err := semanticIdentity(a.SessionDefaults)
	if err != nil || session != revision.Slices.SessionDefaultsDigest {
		return fmt.Errorf("Template entry authority session defaults are inconsistent")
	}
	creation, err := semanticIdentity(a.CreationDefaults)
	if err != nil || creation != revision.Slices.CreationDefaultsDigest {
		return fmt.Errorf("Template entry authority creation defaults are inconsistent")
	}
	return nil
}

func DeriveWorkspaceTemplateEntryAuthority(revision WorkspaceTemplateRevision) (WorkspaceTemplateEntryAuthority, error) {
	if err := revision.Validate(); err != nil {
		return WorkspaceTemplateEntryAuthority{}, err
	}
	result := WorkspaceTemplateEntryAuthority{
		TemplateID: revision.TemplateID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, SourceAccess: revision.Body.Boundary.SourceAccess,
		AgentProfile: revision.Body.Policy.AgentProfile, Runtime: revision.Body.EntryDefaults.Runtime,
		SessionDefaults: revision.Body.SessionDefaults.Clone(), CreationDefaults: revision.Body.CreationDefaults.Clone(),
	}
	return result, result.ValidateFor(revision)
}

type ContextBinding struct {
	SchemaVersion int                 `json:"schema_version"`
	ID            ContextID           `json:"context_id"`
	TemplateID    WorkspaceTemplateID `json:"workspace_template_id"`
}

func (c ContextBinding) Validate() error {
	if c.SchemaVersion != ContextBindingSchemaVersion {
		return fmt.Errorf("Context schema version must be %d", ContextBindingSchemaVersion)
	}
	if err := c.ID.Validate(); err != nil {
		return err
	}
	return c.TemplateID.Validate()
}

func ValidateContextBindings(bindings []ContextBinding) error {
	if bindings == nil {
		return fmt.Errorf("Context collection is unknown")
	}
	ids := make(map[ContextID]struct{}, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if _, exists := ids[binding.ID]; exists {
			return fmt.Errorf("Context IDs must be unique")
		}
		ids[binding.ID] = struct{}{}
	}
	return nil
}

type PolicyMemoryDecision string

const (
	PolicyMemoryAllow PolicyMemoryDecision = "allow"
	PolicyMemoryDeny  PolicyMemoryDecision = "deny"
)

func (d PolicyMemoryDecision) Validate() error {
	if d != PolicyMemoryAllow && d != PolicyMemoryDeny {
		return fmt.Errorf("Policy Memory decision is invalid")
	}
	return nil
}

type PolicyMemoryRuleBody struct {
	PolicyProtocolIdentity
	Match            string   `json:"match"`
	Host             string   `json:"host"`
	Port             int      `json:"port"`
	Method           string   `json:"method"`
	Path             string   `json:"path"`
	Segments         []string `json:"segments"`
	Examples         []string `json:"examples"`
	SourceCandidates []string `json:"source_candidates"`
}

func (b PolicyMemoryRuleBody) Clone() PolicyMemoryRuleBody {
	result := b
	result.Segments = append([]string{}, b.Segments...)
	result.Examples = append([]string{}, b.Examples...)
	result.SourceCandidates = append([]string{}, b.SourceCandidates...)
	return result
}

func (b PolicyMemoryRuleBody) Validate(decision PolicyMemoryDecision) error {
	if err := decision.Validate(); err != nil {
		return err
	}
	if err := b.PolicyProtocolIdentity.Validate(); err != nil {
		return err
	}
	if err := validatePolicyMemoryDestinationAuthority(ManifestPolicyAuthority{Scheme: b.Scheme, Host: b.Host, Port: b.Port}); err != nil {
		return err
	}
	if !httpMethodPattern.MatchString(b.Method) {
		return fmt.Errorf("Policy Memory method is invalid")
	}
	if b.Match == PolicyMatchExact {
		if err := validatePolicyPath(b.Path); err != nil {
			return err
		}
		if err := (SemanticRequestEffect{Scheme: b.Scheme, Host: b.Host, Port: b.Port, Method: b.Method, Path: b.Path, Identity: b.PolicyProtocolIdentity}).Validate(); err != nil {
			return fmt.Errorf("Policy Memory semantic effect: %w", err)
		}
		if len(b.Segments) != 0 || len(b.Examples) != 1 || b.Examples[0] != b.Path {
			return fmt.Errorf("exact Policy Memory rule evidence is invalid")
		}
	} else if b.Match == PolicyMatchPathTemplate && decision == PolicyMemoryAllow && b.EffectiveProtocol() == PolicyProtocolHTTP {
		if err := validatePathTemplate(b.Path, b.Segments); err != nil {
			return err
		}
		if len(b.Examples) < 2 {
			return fmt.Errorf("path-template Policy Memory rule has insufficient evidence")
		}
		for _, example := range b.Examples {
			if !pathTemplateMatches(b.Segments, example) {
				return fmt.Errorf("Policy Memory example does not match its path template")
			}
		}
	} else {
		return fmt.Errorf("Policy Memory match is invalid")
	}
	if err := validateSortedUniquePaths(b.Examples); err != nil {
		return err
	}
	if err := validateSortedUniqueCandidateIDs(b.SourceCandidates); err != nil {
		return err
	}
	minimum := 1
	if b.Match == PolicyMatchPathTemplate {
		minimum = 2
	}
	if len(b.SourceCandidates) < minimum {
		return fmt.Errorf("Policy Memory rule has insufficient source evidence")
	}
	if decision == PolicyMemoryDeny && (b.Match != PolicyMatchExact || len(b.SourceCandidates) != 1) {
		return fmt.Errorf("Policy Memory Deny must remain one exact decision")
	}
	return nil
}

type PolicyMemoryRule struct {
	ID       string               `json:"id"`
	Decision PolicyMemoryDecision `json:"decision"`
	Body     PolicyMemoryRuleBody `json:"body"`
}

func NewPolicyMemoryRule(contextID ContextID, decision PolicyMemoryDecision, body PolicyMemoryRuleBody) (PolicyMemoryRule, error) {
	if err := contextID.Validate(); err != nil {
		return PolicyMemoryRule{}, err
	}
	if err := body.Validate(decision); err != nil {
		return PolicyMemoryRule{}, err
	}
	rule := PolicyMemoryRule{Decision: decision, Body: body.Clone()}
	rule.ID = policyMemoryRuleID(contextID, decision, rule.Body)
	return rule, rule.Validate(contextID)
}

func policyMemoryRuleID(contextID ContextID, decision PolicyMemoryDecision, body PolicyMemoryRuleBody) string {
	material := struct {
		ContextID ContextID
		Decision  PolicyMemoryDecision
		Body      PolicyMemoryRuleBody
	}{contextID, decision, body}
	encoded, _ := semanticIdentity(material)
	bytes, _ := hex.DecodeString(strings.TrimPrefix(string(encoded), "sha256:"))
	return "pmr_" + hex.EncodeToString(bytes[:16])
}

func ValidatePolicyMemoryRuleID(id string) error {
	if len(id) != len("pmr_")+32 || !strings.HasPrefix(id, "pmr_") {
		return fmt.Errorf("Policy Memory rule ID is invalid")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(id, "pmr_")); err != nil {
		return fmt.Errorf("Policy Memory rule ID is invalid")
	}
	return nil
}

func (r PolicyMemoryRule) Validate(contextID ContextID) error {
	if err := contextID.Validate(); err != nil {
		return err
	}
	if err := ValidatePolicyMemoryRuleID(r.ID); err != nil {
		return err
	}
	if err := r.Body.Validate(r.Decision); err != nil {
		return err
	}
	if r.ID != policyMemoryRuleID(contextID, r.Decision, r.Body) {
		return fmt.Errorf("Policy Memory rule ID does not bind its Context and content")
	}
	return nil
}

func (r PolicyMemoryRule) Clone() PolicyMemoryRule {
	result := r
	result.Body = r.Body.Clone()
	return result
}

// NewLearnedPolicyRuleFromPolicyMemory projects one current Context-owned
// Allow into the legacy-free policy candidate matcher. The project scope is
// supplied by the current observing Workspace because Policy Memory survives
// Workspace replacement and deliberately stores no stale Workspace identity.
func NewLearnedPolicyRuleFromPolicyMemory(
	contextID ContextID, contextName string, projectID WorkspaceID, projectRoot string, rule PolicyMemoryRule,
) (LearnedPolicyRule, error) {
	if err := rule.Validate(contextID); err != nil {
		return LearnedPolicyRule{}, err
	}
	if rule.Decision != PolicyMemoryAllow {
		return LearnedPolicyRule{}, fmt.Errorf("Policy Memory rule is not an Allow")
	}
	if err := validatePolicyScope(string(contextID), contextName, projectRoot); err != nil {
		return LearnedPolicyRule{}, err
	}
	if err := projectID.Validate(); err != nil {
		return LearnedPolicyRule{}, err
	}
	body := rule.Body
	result := LearnedPolicyRule{
		PolicyProtocolIdentity: body.PolicyProtocolIdentity,
		Match:                  body.Match,
		WorkspaceManifestID:    string(contextID),
		WorkspaceManifestName:  contextName,
		ProjectID:              string(projectID),
		ProjectRoot:            projectRoot,
		Host:                   body.Host,
		Port:                   body.Port,
		Method:                 body.Method,
		Path:                   body.Path,
		Segments:               append([]string{}, body.Segments...),
		Examples:               append([]string{}, body.Examples...),
		SourceCandidates:       append([]string{}, body.SourceCandidates...),
	}
	result.ID = learnedRuleIDWithIdentity(
		result.Match, result.WorkspaceManifestID, result.ProjectID, result.Host, result.Port,
		result.Method, result.Path, result.Examples, result.SourceCandidates, result.PolicyProtocolIdentity,
	)
	return result, result.Validate()
}

// NewPolicyDenyRuleFromPolicyMemory projects one current Context-owned Deny
// into the exact-deny matcher used to suppress repeat candidates. The current
// Workspace supplies the live project binding; no historical Workspace is
// reconstructed from Policy Memory evidence.
func NewPolicyDenyRuleFromPolicyMemory(
	contextID ContextID, contextName string, projectID WorkspaceID, projectRoot string, rule PolicyMemoryRule,
) (PolicyDenyRule, error) {
	if err := rule.Validate(contextID); err != nil {
		return PolicyDenyRule{}, err
	}
	if rule.Decision != PolicyMemoryDeny {
		return PolicyDenyRule{}, fmt.Errorf("Policy Memory rule is not a Deny")
	}
	if err := validatePolicyScope(string(contextID), contextName, projectRoot); err != nil {
		return PolicyDenyRule{}, err
	}
	if err := projectID.Validate(); err != nil {
		return PolicyDenyRule{}, err
	}
	body := rule.Body
	result := PolicyDenyRule{
		PolicyProtocolIdentity: body.PolicyProtocolIdentity,
		WorkspaceManifestID:    string(contextID),
		WorkspaceManifestName:  contextName,
		ProjectID:              string(projectID),
		ProjectRoot:            projectRoot,
		Host:                   body.Host,
		Port:                   body.Port,
		Method:                 body.Method,
		Path:                   body.Path,
		SourceCandidates:       append([]string{}, body.SourceCandidates...),
	}
	result.ID = policyDenyRuleIDWithIdentity(
		result.WorkspaceManifestID, result.ProjectID, result.Host, result.Port,
		result.Method, result.Path, result.SourceCandidates, result.PolicyProtocolIdentity,
	)
	return result, result.Validate()
}

type PolicyMemoryRevision struct {
	SchemaVersion int                `json:"schema_version"`
	ContextID     ContextID          `json:"context_id"`
	Generation    uint64             `json:"generation"`
	Revision      SemanticDigest     `json:"revision"`
	Rules         []PolicyMemoryRule `json:"rules"`
}

func PublishPolicyMemory(contextID ContextID, rules []PolicyMemoryRule, previous *PolicyMemoryRevision) (PolicyMemoryRevision, bool, error) {
	if err := contextID.Validate(); err != nil {
		return PolicyMemoryRevision{}, false, err
	}
	if rules == nil {
		return PolicyMemoryRevision{}, false, fmt.Errorf("Policy Memory rules are unknown")
	}
	cloned := make([]PolicyMemoryRule, len(rules))
	for index, rule := range rules {
		cloned[index] = rule.Clone()
	}
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].ID < cloned[j].ID })
	generation := uint64(1)
	if previous != nil {
		if err := previous.Validate(); err != nil {
			return PolicyMemoryRevision{}, false, err
		}
		if previous.ContextID != contextID {
			return PolicyMemoryRevision{}, false, fmt.Errorf("Policy Memory owner changed")
		}
		generation = previous.Generation + 1
	}
	revision, err := semanticIdentity(cloned)
	if err != nil {
		return PolicyMemoryRevision{}, false, err
	}
	if previous != nil && previous.Revision == revision {
		return previous.Clone(), false, nil
	}
	result := PolicyMemoryRevision{SchemaVersion: PolicyMemorySchemaVersion, ContextID: contextID, Generation: generation, Revision: revision, Rules: cloned}
	return result, true, result.Validate()
}

func (r PolicyMemoryRevision) Validate() error {
	if r.SchemaVersion != PolicyMemorySchemaVersion || r.Generation == 0 {
		return fmt.Errorf("Policy Memory revision metadata is invalid")
	}
	if err := r.ContextID.Validate(); err != nil {
		return err
	}
	if r.Rules == nil {
		return fmt.Errorf("Policy Memory rules are unknown")
	}
	previous := ""
	for _, rule := range r.Rules {
		if err := rule.Validate(r.ContextID); err != nil {
			return err
		}
		if previous != "" && rule.ID <= previous {
			return fmt.Errorf("Policy Memory rules must be unique and sorted")
		}
		previous = rule.ID
	}
	want, err := semanticIdentity(r.Rules)
	if err != nil {
		return err
	}
	if r.Revision != want {
		return fmt.Errorf("Policy Memory revision does not bind its complete rules")
	}
	return nil
}

// ValidateFor proves that one Context-owned memory remains inside its bound
// Template's fixed terminal Boundary. Baseline policy is a separate tier and
// is deliberately not merged into the remembered revision.
func (r PolicyMemoryRevision) ValidateFor(context ContextBinding, template WorkspaceTemplateRevision) error {
	if err := context.Validate(); err != nil {
		return err
	}
	if err := template.Validate(); err != nil {
		return err
	}
	if r.ContextID != context.ID || template.TemplateID != context.TemplateID {
		return fmt.Errorf("Policy Memory owner does not match its Context and Template")
	}
	return r.validateInsideBoundary(template.Body.Boundary)
}

func (r PolicyMemoryRevision) validateInsideBoundary(boundary WorkspaceTemplateBoundary) error {
	if err := r.Validate(); err != nil {
		return err
	}
	if err := boundary.Validate(); err != nil {
		return err
	}
	for _, rule := range r.Rules {
		path := rule.Body.Path
		if rule.Body.Match == PolicyMatchPathTemplate {
			path = rule.Body.Examples[0]
		}
		exact := ManifestPolicyExactRule{
			Scheme: rule.Body.Scheme, Host: rule.Body.Host, Port: rule.Body.Port,
			Method: rule.Body.Method, Path: path,
		}
		if !contextPolicyRuleInsideDestination(boundary.DestinationCeiling, exact) {
			return fmt.Errorf("Policy Memory rule exceeds the fixed Template Boundary")
		}
		if rule.Decision == PolicyMemoryAllow && boundary.MethodPolicy.Decision(rule.Body.Method) == ManifestMethodDeny {
			return fmt.Errorf("Policy Memory Allow exceeds the fixed Template method Boundary")
		}
	}
	return nil
}

func (r PolicyMemoryRevision) Clone() PolicyMemoryRevision {
	result := r
	result.Rules = make([]PolicyMemoryRule, len(r.Rules))
	for index, rule := range r.Rules {
		result.Rules[index] = rule.Clone()
	}
	return result
}

type WorkspaceAppliedEntry struct {
	ContextID        ContextID           `json:"context_id"`
	TemplateID       WorkspaceTemplateID `json:"workspace_template_id"`
	TemplateRevision SemanticDigest      `json:"workspace_template_revision"`
	EntrySliceDigest SemanticDigest      `json:"entry_slice_digest"`
	RuntimeID        string              `json:"runtime_id"`
	RuntimeRevision  SemanticDigest      `json:"runtime_revision"`
	ResolvedSpec     SemanticDigest      `json:"resolved_spec_revision"`
	ReconciledAt     time.Time           `json:"reconciled_at"`
}

// WorkspaceEntryReconciliationPlan is the complete, decision-bound runtime
// outcome that one explicit Context entry intends to publish. Infrastructure
// may resolve image and container details, but it cannot change the Context,
// Template, Workspace, Runtime, or creation-default authority selected here.
type WorkspaceEntryReconciliationPlan struct {
	Workspace        WorkspaceBinding                  `json:"workspace"`
	Applied          WorkspaceAppliedEntry             `json:"applied_entry"`
	Authority        WorkspaceTemplateEntryAuthority   `json:"entry_authority"`
	CreationDefaults WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
	Network          WorkspaceRuntimeNetworkAuthority  `json:"network_authority"`
}

// WorkspaceRuntimeNetworkAuthority is the exact decision-bound private
// topology for one final Workspace. The Docker bridge owns .1, the shared
// Gateway owns .2, and the Workspace owns .3. Binding these distinct addresses
// before container creation prevents Docker's dynamic allocator from assigning
// the future Gateway address to the first Workspace.
type WorkspaceRuntimeNetworkAuthority struct {
	Network       string `json:"network"`
	Subnet        string `json:"subnet"`
	DockerGateway string `json:"docker_gateway"`
	GatewayIP     string `json:"gateway_ip"`
	WorkspaceIP   string `json:"workspace_ip"`
}

func (a WorkspaceRuntimeNetworkAuthority) ValidateFor(id WorkspaceID) error {
	_, network, err := ProjectResourceNames(string(id))
	if err != nil || a.Network != network {
		return fmt.Errorf("Workspace network authority has another owner: %w", err)
	}
	base, ok := parseWorkspaceRuntimeSubnet(a.Subnet)
	if !ok {
		return fmt.Errorf("Workspace network authority subnet is invalid")
	}
	dockerGateway, dockerOK := parseCanonicalIPv4(a.DockerGateway)
	gateway, gatewayOK := parseCanonicalIPv4(a.GatewayIP)
	workspace, workspaceOK := parseCanonicalIPv4(a.WorkspaceIP)
	if !dockerOK || !gatewayOK || !workspaceOK ||
		dockerGateway != [4]byte{base[0], base[1], base[2], 1} ||
		gateway != [4]byte{base[0], base[1], base[2], 2} ||
		workspace != [4]byte{base[0], base[1], base[2], 3} {
		return fmt.Errorf("Workspace network authority endpoints are invalid")
	}
	return nil
}

func parseWorkspaceRuntimeSubnet(value string) ([4]byte, bool) {
	address, suffix, found := strings.Cut(value, "/")
	if !found || suffix != "24" {
		return [4]byte{}, false
	}
	parsed, ok := parseCanonicalIPv4(address)
	if !ok || parsed[0] != 10 || parsed[1] < 64 || parsed[1] > 127 || parsed[3] != 0 {
		return [4]byte{}, false
	}
	return parsed, true
}

func parseCanonicalIPv4(value string) ([4]byte, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 4 {
		return [4]byte{}, false
	}
	var parsed [4]byte
	for index, part := range parts {
		if part == "" || len(part) > 1 && part[0] == '0' {
			return [4]byte{}, false
		}
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 || number > 255 {
			return [4]byte{}, false
		}
		parsed[index] = byte(number)
	}
	return parsed, true
}

func (p WorkspaceEntryReconciliationPlan) ValidateFor(snapshot ContextAuthoritySnapshot) error {
	if err := snapshot.Validate(); err != nil {
		return err
	}
	if err := p.Workspace.ValidateFor(snapshot.Context); err != nil {
		return err
	}
	if err := p.Applied.ValidateForRevision(snapshot.Context, snapshot.Template.Current); err != nil {
		return err
	}
	if err := p.Authority.ValidateFor(snapshot.Template.Current); err != nil {
		return fmt.Errorf("Workspace entry plan authority: %w", err)
	}
	if err := p.CreationDefaults.Validate(); err != nil {
		return fmt.Errorf("Workspace entry retained creation authority: %w", err)
	}
	creationDigest, err := semanticIdentity(p.CreationDefaults)
	if err != nil || creationDigest != p.Workspace.CreationDefaults {
		return fmt.Errorf("Workspace entry retained creation authority is inconsistent")
	}
	retained := false
	for _, revision := range snapshot.Template.Retained {
		if revision.Slices.CreationDefaultsDigest == p.Workspace.CreationDefaults && revision.Body.CreationDefaults.Validate() == nil {
			if !reflectWorkspaceTemplateCreationDefaults(revision.Body.CreationDefaults, p.CreationDefaults) {
				return fmt.Errorf("Workspace entry retained creation authority is ambiguous")
			}
			retained = true
		}
	}
	if !retained {
		return fmt.Errorf("Workspace entry retained creation authority is unavailable")
	}
	if err := p.Network.ValidateFor(p.Workspace.ID); err != nil {
		return err
	}
	if p.Workspace.LastSuccessfulEntry == nil || *p.Workspace.LastSuccessfulEntry != p.Applied {
		return fmt.Errorf("Workspace entry plan does not publish its exact AppliedEntry")
	}
	if snapshot.Workspace != nil {
		if p.Workspace.ID != snapshot.Workspace.ID || p.Workspace.ContextID != snapshot.Workspace.ContextID || p.Workspace.ProjectRoot != snapshot.Workspace.ProjectRoot || p.Workspace.Home != snapshot.Workspace.Home || p.Workspace.CreationDefaults != snapshot.Workspace.CreationDefaults {
			return fmt.Errorf("Workspace entry plan changed create-once Workspace authority")
		}
	} else if p.Workspace.CreationDefaults != snapshot.Template.Current.Slices.CreationDefaultsDigest {
		return fmt.Errorf("new Workspace entry plan does not bind current creation defaults")
	}
	return nil
}

func (p WorkspaceEntryReconciliationPlan) Clone() WorkspaceEntryReconciliationPlan {
	result := p
	result.Authority.SessionDefaults = p.Authority.SessionDefaults.Clone()
	result.Authority.CreationDefaults = p.Authority.CreationDefaults.Clone()
	result.CreationDefaults = p.CreationDefaults.Clone()
	if p.Workspace.LastSuccessfulEntry != nil {
		entry := *p.Workspace.LastSuccessfulEntry
		result.Workspace.LastSuccessfulEntry = &entry
	}
	return result
}

func reflectWorkspaceTemplateCreationDefaults(left, right WorkspaceTemplateCreationDefaults) bool {
	leftEncoded, leftErr := semanticIdentity(left)
	rightEncoded, rightErr := semanticIdentity(right)
	return leftErr == nil && rightErr == nil && leftEncoded == rightEncoded
}

// WorkspaceEntryReconciliationReceipt is bounded observed evidence returned
// only after the exact planned runtime/spec is healthy in its owned container.
// ContainerID is observation, not desired or persisted Workspace authority.
type WorkspaceEntryReconciliationReceipt struct {
	WorkspaceID WorkspaceID           `json:"workspace_id"`
	ContextID   ContextID             `json:"context_id"`
	Applied     WorkspaceAppliedEntry `json:"applied_entry"`
	ContainerID string                `json:"container_id"`
}

func (r WorkspaceEntryReconciliationReceipt) ValidateFor(plan WorkspaceEntryReconciliationPlan) error {
	if err := plan.Workspace.ID.Validate(); err != nil {
		return err
	}
	if r.WorkspaceID != plan.Workspace.ID || r.ContextID != plan.Workspace.ContextID || r.Applied != plan.Applied {
		return fmt.Errorf("Workspace entry receipt does not confirm its exact plan")
	}
	return validateWorkspaceContainerID(r.ContainerID)
}

func validateWorkspaceContainerID(value string) error {
	if len(value) != 64 {
		return fmt.Errorf("Workspace entry container observation is invalid")
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'f') || (character >= '0' && character <= '9')) {
			return fmt.Errorf("Workspace entry container observation is invalid")
		}
	}
	return nil
}

func (a WorkspaceAppliedEntry) Validate() error {
	if err := a.ContextID.Validate(); err != nil {
		return err
	}
	if err := a.TemplateID.Validate(); err != nil {
		return err
	}
	for name, digest := range map[string]SemanticDigest{
		"Template revision": a.TemplateRevision, "entry slice": a.EntrySliceDigest,
		"Runtime revision": a.RuntimeRevision, "resolved spec": a.ResolvedSpec,
	} {
		if err := digest.Validate(); err != nil {
			return fmt.Errorf("AppliedEntry %s: %w", name, err)
		}
	}
	if a.RuntimeID != StandardRuntimeID {
		if err := ValidateRuntimeID(a.RuntimeID); err != nil {
			return err
		}
	}
	if a.ReconciledAt.IsZero() || a.ReconciledAt.Location() != time.UTC {
		return fmt.Errorf("AppliedEntry reconciliation time must be non-zero UTC")
	}
	return nil
}

func (a WorkspaceAppliedEntry) ValidateFor(context ContextBinding) error {
	if err := context.Validate(); err != nil {
		return err
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if a.ContextID != context.ID || a.TemplateID != context.TemplateID {
		return fmt.Errorf("AppliedEntry does not belong to its Context")
	}
	return nil
}

func (a WorkspaceAppliedEntry) ValidateForRevision(context ContextBinding, revision WorkspaceTemplateRevision) error {
	if err := a.ValidateFor(context); err != nil {
		return err
	}
	if err := revision.Validate(); err != nil {
		return err
	}
	if revision.TemplateID != context.TemplateID || a.TemplateRevision != revision.Revision || a.EntrySliceDigest != revision.Slices.EntrySliceDigest || a.RuntimeID != revision.Slices.RuntimeID || a.RuntimeRevision != revision.Slices.RuntimeRevision {
		return fmt.Errorf("AppliedEntry does not bind the exact Template entry slice and Runtime")
	}
	return nil
}

type WorkspaceBinding struct {
	SchemaVersion       int                    `json:"schema_version"`
	ID                  WorkspaceID            `json:"workspace_id"`
	ContextID           ContextID              `json:"context_id"`
	ProjectRoot         string                 `json:"project_root"`
	Home                string                 `json:"home"`
	CreationDefaults    SemanticDigest         `json:"creation_defaults_digest"`
	LastSuccessfulEntry *WorkspaceAppliedEntry `json:"last_successful_entry,omitempty"`
}

func (w WorkspaceBinding) ValidateFor(context ContextBinding) error {
	if w.SchemaVersion != WorkspaceBindingSchemaVersion {
		return fmt.Errorf("Workspace binding schema version must be %d", WorkspaceBindingSchemaVersion)
	}
	if err := w.ID.Validate(); err != nil {
		return err
	}
	if err := context.Validate(); err != nil {
		return err
	}
	if w.ContextID != context.ID {
		return fmt.Errorf("Workspace binding does not belong to its Context")
	}
	if err := ValidateCanonicalRoot(w.ProjectRoot); err != nil {
		return err
	}
	if w.Home == "" || !filepath.IsAbs(w.Home) || filepath.Clean(w.Home) != w.Home {
		return fmt.Errorf("Workspace home is invalid")
	}
	if err := w.CreationDefaults.Validate(); err != nil {
		return err
	}
	if w.LastSuccessfulEntry != nil {
		if err := w.LastSuccessfulEntry.ValidateFor(context); err != nil {
			return err
		}
	}
	return nil
}

type TemplatePolicyActivationReceipt struct {
	ContextID         ContextID           `json:"context_id"`
	TemplateID        WorkspaceTemplateID `json:"workspace_template_id"`
	PolicySliceDigest SemanticDigest      `json:"policy_slice_digest"`
}

func (r TemplatePolicyActivationReceipt) ValidateFor(context ContextBinding, revision WorkspaceTemplateRevision) error {
	if err := context.Validate(); err != nil {
		return err
	}
	if err := revision.Validate(); err != nil {
		return err
	}
	if r.ContextID != context.ID || r.TemplateID != context.TemplateID || r.TemplateID != revision.TemplateID || r.PolicySliceDigest != revision.Slices.PolicySliceDigest {
		return fmt.Errorf("Template policy activation receipt is inconsistent")
	}
	return nil
}

type PolicyMemoryActivationReceipt struct {
	ContextID ContextID      `json:"context_id"`
	Revision  SemanticDigest `json:"policy_memory_revision"`
}

func (r PolicyMemoryActivationReceipt) ValidateFor(context ContextBinding, memory PolicyMemoryRevision) error {
	if err := context.Validate(); err != nil {
		return err
	}
	if err := memory.Validate(); err != nil {
		return err
	}
	if r.ContextID != context.ID || r.ContextID != memory.ContextID || r.Revision != memory.Revision {
		return fmt.Errorf("Policy Memory activation receipt is inconsistent")
	}
	return nil
}

// ContextAuthoritySnapshot is one coherent final-authority read. Runtime
// observation remains a separate infrastructure-owned read dimension.
type ContextAuthoritySnapshot struct {
	Context               ContextBinding
	Template              WorkspaceTemplate
	PolicyMemory          PolicyMemoryRevision
	ActiveTemplatePolicy  *TemplatePolicyActivationReceipt
	ActivePolicyMemory    *PolicyMemoryRevision
	ActivePolicyMemoryRef *PolicyMemoryActivationReceipt
	Workspace             *WorkspaceBinding
}

// WorkspaceSessionBinding is the complete final-identity input to the
// canonical interactive attachment owner. It carries the exact current
// AppliedEntry and Docker observation established by Context entry; neither a
// mutable Template name nor a predecessor Manifest record can select session
// authority. ContextPresentation is the frozen private wire's display value,
// not an identity or selector.
type WorkspaceSessionBinding struct {
	AuthorityDigest       SemanticDigest                   `json:"authority_digest"`
	ContextID             ContextID                        `json:"context_id"`
	WorkspaceID           WorkspaceID                      `json:"workspace_id"`
	TemplateID            WorkspaceTemplateID              `json:"workspace_template_id"`
	TemplateRevision      SemanticDigest                   `json:"workspace_template_revision"`
	ProjectRoot           string                           `json:"project_root"`
	WorkspaceHome         string                           `json:"workspace_home"`
	ContextPresentation   string                           `json:"context_presentation"`
	AppliedEntry          WorkspaceAppliedEntry            `json:"applied_entry"`
	SessionDefaults       WorkspaceTemplateSessionDefaults `json:"session_defaults"`
	SessionDefaultsDigest SemanticDigest                   `json:"session_defaults_digest"`
	ContainerID           string                           `json:"container_id"`
}

// WorkspaceSessionIdentity is the persistent final identity used to observe
// the canonical WP07 owner across process invocations. It is derivable from
// one coherent final snapshot and deliberately excludes the entry receipt,
// ContainerID, resolved spec, and session defaults needed only when beginning
// or running a session.
type WorkspaceSessionIdentity struct {
	AuthorityDigest     SemanticDigest      `json:"authority_digest"`
	ContextID           ContextID           `json:"context_id"`
	WorkspaceID         WorkspaceID         `json:"workspace_id"`
	TemplateID          WorkspaceTemplateID `json:"workspace_template_id"`
	ContextPresentation string              `json:"context_presentation"`
	ProjectRoot         string              `json:"project_root"`
}

type workspaceSessionIdentityAuthority struct {
	ContextID           ContextID
	WorkspaceID         WorkspaceID
	TemplateID          WorkspaceTemplateID
	ContextPresentation string
	ProjectRoot         string
}

func workspaceSessionIdentityDigest(identity WorkspaceSessionIdentity) (SemanticDigest, error) {
	return semanticIdentity(workspaceSessionIdentityAuthority{
		ContextID: identity.ContextID, WorkspaceID: identity.WorkspaceID,
		TemplateID: identity.TemplateID, ContextPresentation: identity.ContextPresentation,
		ProjectRoot: identity.ProjectRoot,
	})
}

func NewWorkspaceSessionIdentity(snapshot ContextAuthoritySnapshot) (WorkspaceSessionIdentity, error) {
	if err := snapshot.Validate(); err != nil {
		return WorkspaceSessionIdentity{}, err
	}
	if snapshot.Workspace == nil {
		return WorkspaceSessionIdentity{}, fmt.Errorf("Workspace session identity requires one Workspace")
	}
	identity := WorkspaceSessionIdentity{
		ContextID: snapshot.Context.ID, WorkspaceID: snapshot.Workspace.ID,
		TemplateID: snapshot.Template.ID, ContextPresentation: snapshot.Template.Name,
		ProjectRoot: snapshot.Workspace.ProjectRoot,
	}
	var err error
	identity.AuthorityDigest, err = workspaceSessionIdentityDigest(identity)
	if err != nil {
		return WorkspaceSessionIdentity{}, err
	}
	return identity, identity.Validate()
}

func (i WorkspaceSessionIdentity) Validate() error {
	if err := i.AuthorityDigest.Validate(); err != nil {
		return err
	}
	if err := i.ContextID.Validate(); err != nil {
		return err
	}
	if err := i.WorkspaceID.Validate(); err != nil {
		return err
	}
	if err := i.TemplateID.Validate(); err != nil {
		return err
	}
	if err := ValidateName(i.ContextPresentation); err != nil {
		return fmt.Errorf("Workspace session Context presentation is invalid: %w", err)
	}
	if err := ValidateCanonicalRoot(i.ProjectRoot); err != nil {
		return err
	}
	want, err := workspaceSessionIdentityDigest(i)
	if err != nil || want != i.AuthorityDigest {
		return fmt.Errorf("Workspace session identity does not match its complete authority digest")
	}
	return nil
}

func (b WorkspaceSessionBinding) Identity() (WorkspaceSessionIdentity, error) {
	if err := b.Validate(); err != nil {
		return WorkspaceSessionIdentity{}, err
	}
	identity := WorkspaceSessionIdentity{
		ContextID: b.ContextID, WorkspaceID: b.WorkspaceID, TemplateID: b.TemplateID,
		ContextPresentation: b.ContextPresentation, ProjectRoot: b.ProjectRoot,
	}
	var err error
	identity.AuthorityDigest, err = workspaceSessionIdentityDigest(identity)
	if err != nil {
		return WorkspaceSessionIdentity{}, err
	}
	return identity, identity.Validate()
}

type workspaceSessionBindingAuthority struct {
	ContextID             ContextID
	WorkspaceID           WorkspaceID
	TemplateID            WorkspaceTemplateID
	TemplateRevision      SemanticDigest
	ProjectRoot           string
	WorkspaceHome         string
	ContextPresentation   string
	AppliedEntry          WorkspaceAppliedEntry
	SessionDefaults       WorkspaceTemplateSessionDefaults
	SessionDefaultsDigest SemanticDigest
	ContainerID           string
}

func workspaceSessionBindingDigest(binding WorkspaceSessionBinding) (SemanticDigest, error) {
	return semanticIdentity(workspaceSessionBindingAuthority{
		ContextID: binding.ContextID, WorkspaceID: binding.WorkspaceID,
		TemplateID: binding.TemplateID, TemplateRevision: binding.TemplateRevision,
		ProjectRoot: binding.ProjectRoot, WorkspaceHome: binding.WorkspaceHome,
		ContextPresentation: binding.ContextPresentation, AppliedEntry: binding.AppliedEntry,
		SessionDefaults: binding.SessionDefaults.Clone(), SessionDefaultsDigest: binding.SessionDefaultsDigest,
		ContainerID: binding.ContainerID,
	})
}

func NewWorkspaceSessionBinding(snapshot ContextAuthoritySnapshot, receipt WorkspaceEntryReconciliationReceipt) (WorkspaceSessionBinding, error) {
	if err := snapshot.Validate(); err != nil {
		return WorkspaceSessionBinding{}, err
	}
	if snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil {
		return WorkspaceSessionBinding{}, fmt.Errorf("Workspace session requires one last-successful AppliedEntry")
	}
	entry := *snapshot.Workspace.LastSuccessfulEntry
	if err := entry.ValidateForRevision(snapshot.Context, snapshot.Template.Current); err != nil {
		return WorkspaceSessionBinding{}, fmt.Errorf("Workspace session requires the current Template entry authority: %w", err)
	}
	if receipt.WorkspaceID != snapshot.Workspace.ID || receipt.ContextID != snapshot.Context.ID || receipt.Applied != entry {
		return WorkspaceSessionBinding{}, fmt.Errorf("Workspace session receipt does not belong to the final authority snapshot")
	}
	if err := validateWorkspaceContainerID(receipt.ContainerID); err != nil {
		return WorkspaceSessionBinding{}, err
	}
	binding := WorkspaceSessionBinding{
		ContextID: snapshot.Context.ID, WorkspaceID: snapshot.Workspace.ID,
		TemplateID: snapshot.Template.ID, TemplateRevision: snapshot.Template.Current.Revision,
		ProjectRoot: snapshot.Workspace.ProjectRoot, WorkspaceHome: snapshot.Workspace.Home,
		ContextPresentation: snapshot.Template.Name, AppliedEntry: entry,
		SessionDefaults:       snapshot.Template.Current.Body.SessionDefaults.Clone(),
		SessionDefaultsDigest: snapshot.Template.Current.Slices.SessionDefaultsDigest,
		ContainerID:           receipt.ContainerID,
	}
	digest, err := workspaceSessionBindingDigest(binding)
	if err != nil {
		return WorkspaceSessionBinding{}, err
	}
	binding.AuthorityDigest = digest
	return binding, binding.Validate()
}

func (b WorkspaceSessionBinding) Validate() error {
	if err := b.AuthorityDigest.Validate(); err != nil {
		return err
	}
	if err := b.ContextID.Validate(); err != nil {
		return err
	}
	if err := b.WorkspaceID.Validate(); err != nil {
		return err
	}
	if err := b.TemplateID.Validate(); err != nil {
		return err
	}
	if err := b.TemplateRevision.Validate(); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(b.ProjectRoot); err != nil {
		return err
	}
	if b.WorkspaceHome == "" || !filepath.IsAbs(b.WorkspaceHome) || filepath.Clean(b.WorkspaceHome) != b.WorkspaceHome {
		return fmt.Errorf("Workspace session home is invalid")
	}
	if err := ValidateName(b.ContextPresentation); err != nil {
		return fmt.Errorf("Workspace session Context presentation is invalid: %w", err)
	}
	if err := b.AppliedEntry.Validate(); err != nil {
		return err
	}
	if b.AppliedEntry.ContextID != b.ContextID || b.AppliedEntry.TemplateID != b.TemplateID || b.AppliedEntry.TemplateRevision != b.TemplateRevision {
		return fmt.Errorf("Workspace session AppliedEntry crosses final identity")
	}
	if err := b.SessionDefaults.Validate(); err != nil {
		return err
	}
	digest, err := semanticIdentity(b.SessionDefaults)
	if err != nil || digest != b.SessionDefaultsDigest {
		return fmt.Errorf("Workspace session defaults do not match their exact Template slice")
	}
	if err := validateWorkspaceContainerID(b.ContainerID); err != nil {
		return err
	}
	want, err := workspaceSessionBindingDigest(b)
	if err != nil || want != b.AuthorityDigest {
		return fmt.Errorf("Workspace session binding does not match its complete authority digest")
	}
	return nil
}

func (b WorkspaceSessionBinding) Clone() WorkspaceSessionBinding {
	result := b
	result.SessionDefaults = b.SessionDefaults.Clone()
	return result
}

func (s ContextAuthoritySnapshot) Validate() error {
	if err := s.Context.Validate(); err != nil {
		return err
	}
	if err := s.Template.Validate(); err != nil {
		return err
	}
	if s.Context.TemplateID != s.Template.ID {
		return fmt.Errorf("Context snapshot crosses Template authority")
	}
	if err := s.PolicyMemory.ValidateFor(s.Context, s.Template.Current); err != nil {
		return err
	}
	if s.ActiveTemplatePolicy != nil {
		found := false
		for _, revision := range s.Template.Retained {
			if s.ActiveTemplatePolicy.ValidateFor(s.Context, revision) == nil {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("active Template policy has no exact retained revision")
		}
	}
	if (s.ActivePolicyMemory == nil) != (s.ActivePolicyMemoryRef == nil) {
		return fmt.Errorf("active Policy Memory revision and receipt must be present together")
	}
	if s.ActivePolicyMemory != nil {
		if err := s.ActivePolicyMemory.ValidateFor(s.Context, s.Template.Current); err != nil {
			return err
		}
		if err := s.ActivePolicyMemoryRef.ValidateFor(s.Context, *s.ActivePolicyMemory); err != nil {
			return err
		}
	}
	if s.Workspace != nil {
		if err := s.Workspace.ValidateFor(s.Context); err != nil {
			return err
		}
		if s.Workspace.LastSuccessfulEntry != nil {
			found := false
			for _, revision := range s.Template.Retained {
				if s.Workspace.LastSuccessfulEntry.ValidateForRevision(s.Context, revision) == nil {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("Workspace AppliedEntry has no exact retained Template revision")
			}
		}
	}
	return nil
}

func (s ContextAuthoritySnapshot) Clone() ContextAuthoritySnapshot {
	result := s
	result.Template = s.Template.Clone()
	result.PolicyMemory = s.PolicyMemory.Clone()
	if s.ActiveTemplatePolicy != nil {
		value := *s.ActiveTemplatePolicy
		result.ActiveTemplatePolicy = &value
	}
	if s.ActivePolicyMemory != nil {
		value := s.ActivePolicyMemory.Clone()
		result.ActivePolicyMemory = &value
	}
	if s.ActivePolicyMemoryRef != nil {
		value := *s.ActivePolicyMemoryRef
		result.ActivePolicyMemoryRef = &value
	}
	if s.Workspace != nil {
		value := *s.Workspace
		if s.Workspace.LastSuccessfulEntry != nil {
			entry := *s.Workspace.LastSuccessfulEntry
			value.LastSuccessfulEntry = &entry
		}
		result.Workspace = &value
	}
	return result
}

type WorkspaceTemplateCopyPublication struct {
	Source  WorkspaceTemplateRevision
	Created WorkspaceTemplate
}

// WorkspaceTemplateRevisionPublication is the complete task-owned result of
// one exact-reference Template configuration write. The caller supplies one
// closed delta; the owner mutator derives the complete next body from the exact
// predecessor while holding the installation lifecycle authority.
type WorkspaceTemplateRevisionPublication struct {
	Template        WorkspaceTemplate
	Previous        WorkspaceTemplateRevision
	Current         WorkspaceTemplateRevision
	ResolvedRuntime *RuntimeBinding `json:"-"`
	Changed         bool
}

func (p WorkspaceTemplateRevisionPublication) ValidateFor(
	templateRef string,
	change WorkspaceTemplateChange,
) error {
	id, err := ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return err
	}
	if err := change.Validate(); err != nil {
		return err
	}
	if change.Kind == WorkspaceTemplateChangeRuntime {
		if p.ResolvedRuntime == nil {
			return fmt.Errorf("Template Runtime publication lacks exact resolved revision authority")
		}
	} else if p.ResolvedRuntime != nil {
		return fmt.Errorf("non-Runtime Template publication contains Runtime revision authority")
	}
	if err := p.Template.Validate(); err != nil {
		return err
	}
	if err := p.Previous.Validate(); err != nil {
		return err
	}
	if err := p.Current.Validate(); err != nil {
		return err
	}
	if p.Template.ID != id || p.Previous.TemplateID != id || p.Current.TemplateID != id ||
		p.Template.Current.Generation != p.Current.Generation || p.Template.Current.Revision != p.Current.Revision {
		return fmt.Errorf("Template revision publication crosses its exact target")
	}
	expectedBody, err := ApplyWorkspaceTemplateChange(p.Previous.Body, change, p.ResolvedRuntime)
	if err != nil {
		return err
	}
	expected, expectedChanged, err := AdvanceWorkspaceTemplateRevision(p.Previous, expectedBody)
	if err != nil {
		return err
	}
	expectedDigest, err := semanticIdentity(expected)
	if err != nil {
		return err
	}
	currentDigest, err := semanticIdentity(p.Current)
	if err != nil || currentDigest != expectedDigest || p.Changed != expectedChanged {
		return fmt.Errorf("Template revision publication does not bind the exact reviewed delta transition")
	}
	previousFound := false
	currentFound := false
	for _, retained := range p.Template.Retained {
		if retained.Generation == p.Previous.Generation && retained.Revision == p.Previous.Revision {
			previousDigest, digestErr := semanticIdentity(retained)
			wantDigest, wantErr := semanticIdentity(p.Previous)
			if digestErr != nil || wantErr != nil || previousDigest != wantDigest {
				return fmt.Errorf("Template predecessor revision authority differs from retained history")
			}
			previousFound = true
		}
		if retained.Generation == p.Current.Generation && retained.Revision == p.Current.Revision {
			currentFound = true
		}
	}
	if !previousFound || !currentFound {
		return fmt.Errorf("Template revision publication is not retained completely")
	}
	if p.Changed {
		if p.Current.Generation != p.Previous.Generation+1 || p.Current.Revision == p.Previous.Revision {
			return fmt.Errorf("changed Template publication has an invalid revision transition")
		}
	} else if p.Current.Generation != p.Previous.Generation || p.Current.Revision != p.Previous.Revision {
		return fmt.Errorf("no-op Template publication changed revision authority")
	}
	return nil
}

type WorkspaceTemplateSelectionResult struct {
	TemplateID WorkspaceTemplateID
	Selected   bool
}
type WorkspaceTemplateDeleteResult struct {
	TemplateID WorkspaceTemplateID
	Deleted    bool
}
type ContextDeleteResult struct {
	ContextID ContextID
	Deleted   bool
}
type WorkspaceAuthorityDeleteResult struct {
	WorkspaceID WorkspaceID
	Deleted     bool
}
type ContextEntryPublication struct {
	Snapshot ContextAuthoritySnapshot
	Outcome  WorkspaceSessionOutcome
}
type PolicyMemoryPublication struct {
	Snapshot         ContextAuthoritySnapshot
	PreviousRevision SemanticDigest
	Changed          bool
}

// PolicyCandidateAuthority is the complete immutable authority behind one
// opaque candidate reference. The ID binds the candidate to both its Context
// owner and the Workspace observation that produced the exact payload.
type PolicyCandidateAuthority struct {
	ID                   string                `json:"id"`
	ContextID            ContextID             `json:"context_id"`
	ObservingWorkspaceID WorkspaceID           `json:"observing_workspace_id"`
	PayloadDigest        SemanticDigest        `json:"payload_digest"`
	Effect               PolicyCandidateEffect `json:"effect"`
}

// PolicyCandidateEffect is the complete exact policy effect proposed by one
// candidate. SourceCandidates is deliberately absent: the opaque candidate ID
// is derived from this payload and is added only when a decision becomes a
// remembered rule.
type PolicyCandidateEffect struct {
	PolicyProtocolIdentity
	Match    string   `json:"match"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Segments []string `json:"segments"`
	Examples []string `json:"examples"`
}

func (c PolicyCandidateAuthority) Clone() PolicyCandidateAuthority {
	result := c
	result.Effect = c.Effect.Clone()
	return result
}

func (e PolicyCandidateEffect) Clone() PolicyCandidateEffect {
	result := e
	result.Segments = append([]string{}, e.Segments...)
	result.Examples = append([]string{}, e.Examples...)
	return result
}

func (e PolicyCandidateEffect) RuleBody(candidateID string) PolicyMemoryRuleBody {
	return PolicyMemoryRuleBody{
		PolicyProtocolIdentity: e.PolicyProtocolIdentity,
		Match:                  e.Match, Host: e.Host, Port: e.Port, Method: e.Method, Path: e.Path,
		Segments: append([]string{}, e.Segments...), Examples: append([]string{}, e.Examples...),
		SourceCandidates: []string{candidateID},
	}
}

func (e PolicyCandidateEffect) Validate() error {
	// Direct candidate actions remain exact-rule operations. Broader reviewed
	// compaction is owned by ApplyReviewed and is not inferred here.
	if e.Match != PolicyMatchExact {
		return fmt.Errorf("Policy candidate effect must be exact")
	}
	return e.RuleBody("pcy_00000000000000000000000000000000").Validate(PolicyMemoryAllow)
}

func policyCandidateEffectDigest(effect PolicyCandidateEffect) (SemanticDigest, error) {
	if err := effect.Validate(); err != nil {
		return "", err
	}
	return semanticIdentity(effect)
}

func NewPolicyCandidateAuthority(contextID ContextID, workspaceID WorkspaceID, effect PolicyCandidateEffect) (PolicyCandidateAuthority, error) {
	payload, err := policyCandidateEffectDigest(effect)
	if err != nil {
		return PolicyCandidateAuthority{}, err
	}
	candidate := PolicyCandidateAuthority{
		ID: policyCandidateAuthorityID(contextID, workspaceID, payload), ContextID: contextID,
		ObservingWorkspaceID: workspaceID, PayloadDigest: payload, Effect: effect.Clone(),
	}
	return candidate, candidate.Validate()
}

func policyCandidateAuthorityID(contextID ContextID, workspaceID WorkspaceID, payload SemanticDigest) string {
	digest, _ := semanticIdentity(struct {
		ContextID   ContextID
		WorkspaceID WorkspaceID
		Payload     SemanticDigest
	}{contextID, workspaceID, payload})
	return "pcy_" + strings.TrimPrefix(string(digest), "sha256:")[:32]
}

func (c PolicyCandidateAuthority) Validate() error {
	if err := ValidatePolicyCandidateID(c.ID); err != nil {
		return err
	}
	if err := c.ContextID.Validate(); err != nil {
		return err
	}
	if err := c.ObservingWorkspaceID.Validate(); err != nil {
		return err
	}
	if err := c.PayloadDigest.Validate(); err != nil {
		return err
	}
	payload, err := policyCandidateEffectDigest(c.Effect)
	if err != nil {
		return err
	}
	if payload != c.PayloadDigest {
		return fmt.Errorf("Policy candidate payload does not bind its exact effect")
	}
	if c.ID != policyCandidateAuthorityID(c.ContextID, c.ObservingWorkspaceID, c.PayloadDigest) {
		return fmt.Errorf("Policy candidate ID does not bind its complete authority")
	}
	return nil
}

type PolicyCandidatePublication struct {
	Candidate PolicyCandidateAuthority
	RuleID    string
	Previous  PolicyMemoryRevision
	Memory    PolicyMemoryPublication
}

// PolicyCandidateDecisionPublication is the internal direct-action result for
// the existing policy allow/deny task. Exactly one authority branch changes:
// persistent candidates publish Policy Memory, while Host Loopback candidates
// publish an attachment-local grant.
type PolicyCandidateDecisionPublication struct {
	Persistent *PolicyCandidatePublication
	Attachment *AttachmentGrantPublication
}

func NewPersistentPolicyCandidateDecisionPublication(value PolicyCandidatePublication) PolicyCandidateDecisionPublication {
	copy := value
	return PolicyCandidateDecisionPublication{Persistent: &copy}
}

func NewAttachmentPolicyCandidateDecisionPublication(value AttachmentGrantPublication) PolicyCandidateDecisionPublication {
	copy := value
	return PolicyCandidateDecisionPublication{Attachment: &copy}
}

func (p PolicyCandidateDecisionPublication) ValidateFor(candidateID string, decision PolicyMemoryDecision) error {
	if (p.Persistent == nil) == (p.Attachment == nil) {
		return fmt.Errorf("policy candidate decision must change exactly one authority branch")
	}
	if p.Persistent != nil {
		return p.Persistent.ValidateFor(candidateID, decision)
	}
	return p.Attachment.ValidateFor(candidateID, decision)
}

func (p PolicyCandidateDecisionPublication) ActiveRevision() string {
	if p.Persistent != nil {
		return string(p.Persistent.Memory.Snapshot.PolicyMemory.Revision)
	}
	if p.Attachment != nil {
		return p.Attachment.Activation.ActiveRevision
	}
	return ""
}

func (p PolicyCandidatePublication) ValidateFor(candidateID string, decision PolicyMemoryDecision) error {
	if err := ValidatePolicyCandidateID(candidateID); err != nil {
		return err
	}
	if err := decision.Validate(); err != nil {
		return err
	}
	if err := p.Candidate.Validate(); err != nil {
		return err
	}
	if p.Candidate.ID != candidateID {
		return fmt.Errorf("Policy candidate reference was not consumed unchanged")
	}
	if err := ValidatePolicyMemoryRuleID(p.RuleID); err != nil {
		return err
	}
	if err := p.Memory.Validate(); err != nil {
		return err
	}
	if !p.Memory.Changed {
		return fmt.Errorf("Policy candidate publication did not change authority")
	}
	if p.Memory.Snapshot.Context.ID != p.Candidate.ContextID || p.Memory.Snapshot.Workspace == nil || p.Memory.Snapshot.Workspace.ID != p.Candidate.ObservingWorkspaceID {
		return fmt.Errorf("Policy candidate publication crosses its Context or observing Workspace")
	}
	if err := p.Previous.ValidateFor(p.Memory.Snapshot.Context, p.Memory.Snapshot.Template.Current); err != nil {
		return err
	}
	if p.Previous.Revision != p.Memory.PreviousRevision || p.Previous.ContextID != p.Candidate.ContextID || p.Memory.Snapshot.PolicyMemory.Generation != p.Previous.Generation+1 {
		return fmt.Errorf("Policy candidate publication does not bind its exact previous revision")
	}
	for _, rule := range p.Previous.Rules {
		for _, source := range rule.Body.SourceCandidates {
			if source == candidateID {
				return fmt.Errorf("Policy candidate was already present in previous authority")
			}
		}
	}
	var resultingRule *PolicyMemoryRule
	for index := range p.Memory.Snapshot.PolicyMemory.Rules {
		rule := &p.Memory.Snapshot.PolicyMemory.Rules[index]
		if rule.ID != p.RuleID {
			continue
		}
		if rule.Decision != decision {
			return fmt.Errorf("Policy candidate publication has the wrong decision")
		}
		wantBody := p.Candidate.Effect.RuleBody(candidateID)
		wantDigest, err := semanticIdentity(wantBody)
		if err != nil {
			return err
		}
		gotDigest, err := semanticIdentity(rule.Body)
		if err != nil {
			return err
		}
		if gotDigest != wantDigest {
			return fmt.Errorf("Policy candidate publication rule does not match the requested exact effect")
		}
		resultingRule = rule
		break
	}
	if resultingRule == nil {
		return fmt.Errorf("Policy candidate publication has no resulting Policy Memory rule")
	}
	expectedRules := make([]PolicyMemoryRule, 0, len(p.Previous.Rules)+1)
	for _, rule := range p.Previous.Rules {
		expectedRules = append(expectedRules, rule.Clone())
	}
	expectedRules = append(expectedRules, resultingRule.Clone())
	want, changed, err := PublishPolicyMemory(p.Previous.ContextID, expectedRules, &p.Previous)
	if err != nil {
		return err
	}
	if !changed || want.Revision != p.Memory.Snapshot.PolicyMemory.Revision || want.Generation != p.Memory.Snapshot.PolicyMemory.Generation {
		return fmt.Errorf("Policy candidate publication changed authority beyond the exact requested rule")
	}
	return nil
}

type PolicyRuleResetPublication struct {
	RuleID      string
	RemovedFrom PolicyMemoryRevision
	Memory      PolicyMemoryPublication
}

func (p PolicyRuleResetPublication) ValidateFor(ruleID string) error {
	if err := ValidatePolicyMemoryRuleID(ruleID); err != nil {
		return err
	}
	if p.RuleID != ruleID {
		return fmt.Errorf("Policy Memory rule reference was not consumed unchanged")
	}
	if err := p.Memory.Validate(); err != nil {
		return err
	}
	if !p.Memory.Changed {
		return fmt.Errorf("Policy Memory reset did not change authority")
	}
	if err := p.RemovedFrom.ValidateFor(p.Memory.Snapshot.Context, p.Memory.Snapshot.Template.Current); err != nil {
		return err
	}
	if p.RemovedFrom.Revision != p.Memory.PreviousRevision || p.Memory.Snapshot.PolicyMemory.Generation != p.RemovedFrom.Generation+1 {
		return fmt.Errorf("Policy Memory reset does not bind its exact previous revision")
	}
	remaining := make([]PolicyMemoryRule, 0, len(p.RemovedFrom.Rules))
	found := false
	for _, rule := range p.RemovedFrom.Rules {
		if rule.ID == ruleID {
			found = true
			continue
		}
		remaining = append(remaining, rule.Clone())
	}
	if !found {
		return fmt.Errorf("reset Policy Memory rule was absent from previous authority")
	}
	for _, rule := range p.Memory.Snapshot.PolicyMemory.Rules {
		if rule.ID == ruleID {
			return fmt.Errorf("reset Policy Memory rule remains active")
		}
	}
	want, changed, err := PublishPolicyMemory(p.RemovedFrom.ContextID, remaining, &p.RemovedFrom)
	if err != nil {
		return err
	}
	if !changed || want.Revision != p.Memory.Snapshot.PolicyMemory.Revision || want.Generation != p.Memory.Snapshot.PolicyMemory.Generation {
		return fmt.Errorf("Policy Memory reset changed authority beyond the exact requested rule")
	}
	return nil
}

func (p PolicyMemoryPublication) Validate() error {
	if err := p.Snapshot.Validate(); err != nil {
		return err
	}
	if err := p.PreviousRevision.Validate(); err != nil {
		return err
	}
	if p.Snapshot.ActivePolicyMemory == nil || p.Snapshot.ActivePolicyMemoryRef == nil || p.Snapshot.ActivePolicyMemory.Revision != p.Snapshot.PolicyMemory.Revision {
		return fmt.Errorf("Policy Memory publication is not actively committed")
	}
	if p.Changed == (p.PreviousRevision == p.Snapshot.PolicyMemory.Revision) {
		return fmt.Errorf("Policy Memory publication change state is inconsistent")
	}
	return nil
}
