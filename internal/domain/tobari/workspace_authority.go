package tobari

import (
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	WorkspaceTemplateSchemaVersion = 1
	ContextBindingSchemaVersion    = 1
	PolicyMemorySchemaVersion      = 1
	WorkspaceBindingSchemaVersion  = 3

	WorkspaceTemplateReferenceKind         = "workspace-template"
	WorkspaceTemplateRevisionReferenceKind = "workspace-template-revision"
	ContextReferenceKind                   = "context"
	WorkspaceReferenceKind                 = "workspace"

	WorkspaceTemplateAdvancedPolicyPath     = "tobari.rego"
	WorkspaceTemplateAdvancedPolicyTestPath = "tobari_test.rego"
	WorkspaceTemplateAdvancedPolicyMaxBytes = 4 * 1024 * 1024
)

const (
	workspaceTemplateRefPrefix         = "wtpl1_"
	workspaceTemplateRevisionRefPrefix = "wtrev1_"
	contextRefPrefix                   = "ctx1_"
	workspaceRefPrefix                 = "wsp1_"
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

// WorkspaceTemplateAdvancedPolicySources is the closed executable-source
// boundary for Advanced mode. Named fields make missing, renamed, duplicate,
// and extra files unrepresentable in final authority.
type WorkspaceTemplateAdvancedPolicySources struct {
	Tobari     string `json:"tobari_rego"`
	TobariTest string `json:"tobari_test_rego"`
}

func (s WorkspaceTemplateAdvancedPolicySources) Validate() error {
	if s.Tobari == "" || s.TobariTest == "" || len(s.Tobari)+len(s.TobariTest) > WorkspaceTemplateAdvancedPolicyMaxBytes {
		return fmt.Errorf("Template Advanced policy source pair is incomplete or too large")
	}
	for _, content := range []string{s.Tobari, s.TobariTest} {
		if !utf8.ValidString(content) || strings.IndexByte(content, 0) >= 0 {
			return fmt.Errorf("Template Advanced policy source content is invalid")
		}
	}
	return nil
}

type WorkspaceTemplateAdvancedPolicyFile struct {
	Path    string
	Content string
}

// NewWorkspaceTemplateAdvancedPolicySources converts a predecessor or trusted
// host snapshot only when it is the exact closed pair. Callers cannot silently
// ignore a missing, renamed, duplicate, or extra executable source.
func NewWorkspaceTemplateAdvancedPolicySources(files []WorkspaceTemplateAdvancedPolicyFile) (WorkspaceTemplateAdvancedPolicySources, error) {
	if len(files) != 2 {
		return WorkspaceTemplateAdvancedPolicySources{}, fmt.Errorf("Template Advanced policy requires exactly two sources")
	}
	result := WorkspaceTemplateAdvancedPolicySources{}
	seen := make(map[string]struct{}, 2)
	for _, file := range files {
		if _, exists := seen[file.Path]; exists {
			return WorkspaceTemplateAdvancedPolicySources{}, fmt.Errorf("Template Advanced policy source is duplicated")
		}
		seen[file.Path] = struct{}{}
		switch file.Path {
		case WorkspaceTemplateAdvancedPolicyPath:
			result.Tobari = file.Content
		case WorkspaceTemplateAdvancedPolicyTestPath:
			result.TobariTest = file.Content
		default:
			return WorkspaceTemplateAdvancedPolicySources{}, fmt.Errorf("Template Advanced policy source name is invalid")
		}
	}
	return result, result.Validate()
}

// Files materializes the one closed filesystem projection. It does not accept
// names from caller input.
func (s WorkspaceTemplateAdvancedPolicySources) Files() ([]WorkspaceTemplateAdvancedPolicyFile, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return []WorkspaceTemplateAdvancedPolicyFile{
		{Path: WorkspaceTemplateAdvancedPolicyPath, Content: s.Tobari},
		{Path: WorkspaceTemplateAdvancedPolicyTestPath, Content: s.TobariTest},
	}, nil
}

// WorkspaceTemplatePolicyBody is the complete static baseline. Terminal
// destination and method ceilings live only in Boundary and are supplied for
// validation rather than duplicated here.
type WorkspaceTemplatePolicyBody struct {
	AgentProfile      string                                  `json:"agent_profile"`
	Mode              ManifestPolicyMode                      `json:"mode"`
	NativeReadiness   ManifestNativeReadiness                 `json:"native_readiness"`
	BaselineGrants    []ManifestPolicyExactRule               `json:"baseline_grants"`
	BaselineTemplates []ManifestPolicyPathTemplateRule        `json:"baseline_templates"`
	MCPBaselineGrants []ManifestPolicyMCPRule                 `json:"mcp_baseline_grants"`
	BaselineDenies    []ManifestPolicyExactRule               `json:"baseline_denies"`
	GraphQLEndpoints  []ManifestPolicyExactRule               `json:"graphql_endpoints"`
	MCPEndpoints      []ManifestPolicyExactRule               `json:"mcp_endpoints"`
	AdvancedPolicy    *WorkspaceTemplateAdvancedPolicySources `json:"advanced_policy,omitempty"`
}

func (p WorkspaceTemplatePolicyBody) Validate(boundary WorkspaceTemplateBoundary) error {
	if err := boundary.Validate(); err != nil {
		return err
	}
	if err := ValidateName(p.AgentProfile); err != nil {
		return fmt.Errorf("Template agent profile: %w", err)
	}
	if err := p.Mode.Validate(); err != nil {
		return err
	}
	if err := p.NativeReadiness.Validate(); err != nil {
		return err
	}
	if p.Mode == ManifestPolicyModeGuided && p.AdvancedPolicy != nil {
		return fmt.Errorf("Guided Template cannot own Advanced policy sources")
	}
	if p.Mode == ManifestPolicyModeAdvanced {
		if p.AdvancedPolicy == nil {
			return fmt.Errorf("Advanced Template requires the exact policy source pair")
		}
		if err := p.AdvancedPolicy.Validate(); err != nil {
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
	result.BaselineGrants = append([]ManifestPolicyExactRule{}, p.BaselineGrants...)
	result.BaselineTemplates = append([]ManifestPolicyPathTemplateRule{}, p.BaselineTemplates...)
	for index := range result.BaselineTemplates {
		result.BaselineTemplates[index].Segments = append([]string{}, p.BaselineTemplates[index].Segments...)
	}
	result.MCPBaselineGrants = append([]ManifestPolicyMCPRule{}, p.MCPBaselineGrants...)
	result.BaselineDenies = append([]ManifestPolicyExactRule{}, p.BaselineDenies...)
	result.GraphQLEndpoints = append([]ManifestPolicyExactRule{}, p.GraphQLEndpoints...)
	result.MCPEndpoints = append([]ManifestPolicyExactRule{}, p.MCPEndpoints...)
	if p.AdvancedPolicy != nil {
		advanced := *p.AdvancedPolicy
		result.AdvancedPolicy = &advanced
	}
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
	slices, err := body.slices()
	if err != nil {
		return WorkspaceTemplateRevision{}, false, err
	}
	if slices.BoundaryFingerprint != previous.Slices.BoundaryFingerprint {
		return WorkspaceTemplateRevision{}, false, fmt.Errorf("Template Boundary is immutable")
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
		if revision.TemplateID != previous.TemplateID || revision.Slices.BoundaryFingerprint != previous.Slices.BoundaryFingerprint {
			return fmt.Errorf("Template history crosses identity or Boundary")
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
	SchemaVersion int                         `json:"schema_version"`
	ID            WorkspaceTemplateID         `json:"workspace_template_id"`
	Name          string                      `json:"name"`
	Current       WorkspaceTemplateRevision   `json:"current"`
	Retained      []WorkspaceTemplateRevision `json:"retained"`
}

func (t WorkspaceTemplate) Validate() error {
	if t.SchemaVersion != WorkspaceTemplateSchemaVersion || t.ID.Validate() != nil || ValidateName(t.Name) != nil {
		return fmt.Errorf("Workspace Template identity is invalid")
	}
	if err := ValidateWorkspaceTemplateHistory(t.Retained); err != nil {
		return err
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
	for index := range t.Retained {
		result.Retained[index] = t.Retained[index].Clone()
	}
	return result
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
	return result, result.Validate()
}

// WorkspaceTemplateEntryAuthority is the complete static entry material
// derived from one unchanged Template revision. It lets context enter and
// cluster up consume final authority without consulting predecessor state.
type WorkspaceTemplateEntryAuthority struct {
	TemplateID       WorkspaceTemplateID               `json:"workspace_template_id"`
	TemplateRevision SemanticDigest                    `json:"workspace_template_revision"`
	EntrySliceDigest SemanticDigest                    `json:"entry_slice_digest"`
	Runtime          RuntimeBinding                    `json:"runtime"`
	SessionDefaults  WorkspaceTemplateSessionDefaults  `json:"session_defaults"`
	CreationDefaults WorkspaceTemplateCreationDefaults `json:"creation_defaults"`
}

func (a WorkspaceTemplateEntryAuthority) ValidateFor(revision WorkspaceTemplateRevision) error {
	if err := revision.Validate(); err != nil {
		return err
	}
	if a.TemplateID != revision.TemplateID || a.TemplateRevision != revision.Revision || a.EntrySliceDigest != revision.Slices.EntrySliceDigest || a.Runtime != revision.Body.EntryDefaults.Runtime {
		return fmt.Errorf("Template entry authority does not bind its exact revision")
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
		EntrySliceDigest: revision.Slices.EntrySliceDigest, Runtime: revision.Body.EntryDefaults.Runtime,
		SessionDefaults: revision.Body.SessionDefaults.Clone(), CreationDefaults: revision.Body.CreationDefaults.Clone(),
	}
	return result, result.ValidateFor(revision)
}

type ContextBinding struct {
	SchemaVersion int                 `json:"schema_version"`
	ID            ContextID           `json:"context_id"`
	ProjectRoot   string              `json:"project_root"`
	TemplateID    WorkspaceTemplateID `json:"workspace_template_id"`
}

func (c ContextBinding) Validate() error {
	if c.SchemaVersion != ContextBindingSchemaVersion {
		return fmt.Errorf("Context schema version must be %d", ContextBindingSchemaVersion)
	}
	if err := c.ID.Validate(); err != nil {
		return err
	}
	if err := ValidateCanonicalRoot(c.ProjectRoot); err != nil {
		return err
	}
	return c.TemplateID.Validate()
}

func ValidateContextBindings(bindings []ContextBinding) error {
	if bindings == nil {
		return fmt.Errorf("Context collection is unknown")
	}
	ids := make(map[ContextID]struct{}, len(bindings))
	pairs := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return err
		}
		if _, exists := ids[binding.ID]; exists {
			return fmt.Errorf("Context IDs must be unique")
		}
		pair := binding.ProjectRoot + "\x00" + string(binding.TemplateID)
		if _, exists := pairs[pair]; exists {
			return fmt.Errorf("one Project and Template pair may have only one Context")
		}
		ids[binding.ID] = struct{}{}
		pairs[pair] = struct{}{}
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
	if err := (ManifestPolicyAuthority{Scheme: b.Scheme, Host: b.Host, Port: b.Port}).Validate(); err != nil {
		return err
	}
	if !httpMethodPattern.MatchString(b.Method) {
		return fmt.Errorf("Policy Memory method is invalid")
	}
	if b.Match == PolicyMatchExact {
		if err := validatePolicyPath(b.Path); err != nil {
			return err
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

func (r PolicyMemoryRule) Validate(contextID ContextID) error {
	if err := contextID.Validate(); err != nil {
		return err
	}
	if len(r.ID) != len("pmr_")+32 || !strings.HasPrefix(r.ID, "pmr_") {
		return fmt.Errorf("Policy Memory rule ID is invalid")
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(r.ID, "pmr_")); err != nil {
		return fmt.Errorf("Policy Memory rule ID is invalid")
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
	if w.ContextID != context.ID || w.ProjectRoot != context.ProjectRoot {
		return fmt.Errorf("Workspace binding does not belong to its Context")
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
