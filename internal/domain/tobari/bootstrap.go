package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	ContextBootstrapSchemaVersion = 1
	ContextBootstrapAdapterAWS    = "aws_iam_identity_center"
	MaxContextBootstrapValueBytes = 2048

	ContextBootstrapTargetKind = "context-workspace-bootstrap"
	ContextBootstrapTargetID   = "context-workspace-bootstrap"

	ContextBootstrapNotConfigured = "not_configured"
	ContextBootstrapConfigured    = "configured"

	WorkspaceBootstrapNotConfigured = "not_configured"
	WorkspaceBootstrapNotApplied    = "not_applied"
	WorkspaceBootstrapCurrent       = "current"
	WorkspaceBootstrapOlder         = "older"
)

var ErrContextBootstrapSourceChanged = fmt.Errorf("Context bootstrap source changed during review")

var (
	awsBootstrapNamePattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	awsBootstrapAccountPattern  = regexp.MustCompile(`^[0-9]{12}$`)
	awsBootstrapRolePattern     = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{1,64}$`)
	awsBootstrapRegionPattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)
	awsBootstrapOutputPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	awsBootstrapScopePattern    = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]{1,128}$`)
	awsBootstrapStartURLPattern = regexp.MustCompile(`^https://[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?\.awsapps\.com/start/?$`)
)

// ContextAWSBootstrap is the closed, secret-free subset of one AWS shared
// config profile and its IAM Identity Center session. It contains no
// credential, token cache, helper, filesystem path, or executable selection.
type ContextAWSBootstrap struct {
	Profile               string   `json:"profile"`
	SSOSession            string   `json:"sso_session"`
	SSOStartURL           string   `json:"sso_start_url"`
	SSORegion             string   `json:"sso_region"`
	SSORegistrationScopes []string `json:"sso_registration_scopes"`
	AccountID             string   `json:"sso_account_id"`
	RoleName              string   `json:"sso_role_name"`
	Region                string   `json:"region,omitempty"`
	Output                string   `json:"output,omitempty"`
}

func (a ContextAWSBootstrap) Validate() error {
	if !awsBootstrapNamePattern.MatchString(a.Profile) {
		return fmt.Errorf("AWS bootstrap profile name is invalid")
	}
	if !awsBootstrapNamePattern.MatchString(a.SSOSession) {
		return fmt.Errorf("AWS bootstrap SSO session name is invalid")
	}
	if !awsBootstrapStartURLPattern.MatchString(a.SSOStartURL) || strings.Contains(a.SSOStartURL, "..") {
		return fmt.Errorf("AWS bootstrap SSO start URL must be an exact AWS access-portal HTTPS start URL")
	}
	if len(a.SSOStartURL) > MaxContextBootstrapValueBytes {
		return fmt.Errorf("AWS bootstrap SSO start URL is too long")
	}
	if !awsBootstrapRegionPattern.MatchString(a.SSORegion) {
		return fmt.Errorf("AWS bootstrap SSO region is invalid")
	}
	if !awsBootstrapAccountPattern.MatchString(a.AccountID) {
		return fmt.Errorf("AWS bootstrap account ID is invalid")
	}
	if !awsBootstrapRolePattern.MatchString(a.RoleName) {
		return fmt.Errorf("AWS bootstrap role name is invalid")
	}
	if a.Region != "" && !awsBootstrapRegionPattern.MatchString(a.Region) {
		return fmt.Errorf("AWS bootstrap region is invalid")
	}
	if a.Output != "" && !awsBootstrapOutputPattern.MatchString(a.Output) {
		return fmt.Errorf("AWS bootstrap output is invalid")
	}
	if a.SSORegistrationScopes == nil {
		return fmt.Errorf("AWS bootstrap registration scopes must be explicit")
	}
	if len(a.SSORegistrationScopes) > 16 {
		return fmt.Errorf("AWS bootstrap has too many registration scopes")
	}
	seen := make(map[string]struct{}, len(a.SSORegistrationScopes))
	for index, scope := range a.SSORegistrationScopes {
		if !awsBootstrapScopePattern.MatchString(scope) {
			return fmt.Errorf("AWS bootstrap registration scope %d is invalid", index)
		}
		if _, duplicate := seen[scope]; duplicate {
			return fmt.Errorf("AWS bootstrap registration scopes must be unique")
		}
		seen[scope] = struct{}{}
		if index > 0 && a.SSORegistrationScopes[index-1] >= scope {
			return fmt.Errorf("AWS bootstrap registration scopes must be sorted")
		}
	}
	return nil
}

func (a ContextAWSBootstrap) Clone() ContextAWSBootstrap {
	result := a
	result.SSORegistrationScopes = append([]string{}, a.SSORegistrationScopes...)
	return result
}

// ContextBootstrapSnapshot is one immutable semantic revision used only when
// a future Workspace home is first created.
type ContextBootstrapSnapshot struct {
	SchemaVersion int                 `json:"schema_version"`
	Generation    uint64              `json:"generation"`
	Revision      string              `json:"revision"`
	AWS           ContextAWSBootstrap `json:"aws"`
}

func NewContextBootstrapSnapshot(generation uint64, aws ContextAWSBootstrap) (ContextBootstrapSnapshot, error) {
	aws.SSORegistrationScopes = append([]string{}, aws.SSORegistrationScopes...)
	sort.Strings(aws.SSORegistrationScopes)
	if generation == 0 {
		return ContextBootstrapSnapshot{}, fmt.Errorf("Context bootstrap generation must be positive")
	}
	if err := aws.Validate(); err != nil {
		return ContextBootstrapSnapshot{}, err
	}
	snapshot := ContextBootstrapSnapshot{SchemaVersion: ContextBootstrapSchemaVersion, Generation: generation, AWS: aws}
	snapshot.Revision = snapshot.semanticRevision()
	return snapshot, nil
}

func (s ContextBootstrapSnapshot) semanticRevision() string {
	fields := []string{
		fmt.Sprintf("schema=%d", s.SchemaVersion),
		"adapter=" + ContextBootstrapAdapterAWS,
		"profile=" + s.AWS.Profile,
		"sso_session=" + s.AWS.SSOSession,
		"sso_start_url=" + s.AWS.SSOStartURL,
		"sso_region=" + s.AWS.SSORegion,
		"sso_registration_scopes=" + strings.Join(s.AWS.SSORegistrationScopes, " "),
		"sso_account_id=" + s.AWS.AccountID,
		"sso_role_name=" + s.AWS.RoleName,
		"region=" + s.AWS.Region,
		"output=" + s.AWS.Output,
	}
	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s ContextBootstrapSnapshot) Validate() error {
	if s.SchemaVersion != ContextBootstrapSchemaVersion || s.Generation == 0 {
		return fmt.Errorf("Context bootstrap snapshot version or generation is invalid")
	}
	if err := s.AWS.Validate(); err != nil {
		return err
	}
	if s.Revision != s.semanticRevision() {
		return fmt.Errorf("Context bootstrap revision does not match its semantic snapshot")
	}
	return nil
}

func (s ContextBootstrapSnapshot) Clone() ContextBootstrapSnapshot {
	result := s
	result.AWS = s.AWS.Clone()
	return result
}

type ContextBootstrapPreview struct {
	ContextName string                 `json:"context"`
	Current     ContextBootstrapReport `json:"current"`
	Candidate   ContextBootstrapReport `json:"candidate"`
	Changes     []string               `json:"changes"`
}

func NewContextBootstrapPreview(contextName string, current *ContextBootstrapSnapshot, candidate ContextBootstrapSnapshot) (ContextBootstrapPreview, error) {
	if err := ValidateName(contextName); err != nil {
		return ContextBootstrapPreview{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ContextBootstrapPreview{}, err
	}
	changes := diffContextAWSBootstrap(current, candidate)
	result := ContextBootstrapPreview{ContextName: contextName, Current: ContextBootstrapReportFrom(current), Candidate: ContextBootstrapReportFrom(&candidate), Changes: changes}
	return result, result.Validate()
}

func (p ContextBootstrapPreview) Validate() error {
	if err := ValidateName(p.ContextName); err != nil {
		return err
	}
	if err := p.Current.Validate(); err != nil {
		return err
	}
	if err := p.Candidate.Validate(); err != nil {
		return err
	}
	if p.Candidate.Resolved().State != ContextBootstrapConfigured || p.Changes == nil {
		return fmt.Errorf("Context bootstrap preview is incomplete")
	}
	for index, change := range p.Changes {
		if change == "" || (index > 0 && p.Changes[index-1] >= change) {
			return fmt.Errorf("Context bootstrap preview changes are invalid")
		}
	}
	if len(p.Changes) == 0 && p.Current.Resolved().Revision != p.Candidate.Revision {
		return fmt.Errorf("Context bootstrap preview misses a semantic change")
	}
	return nil
}

func diffContextAWSBootstrap(current *ContextBootstrapSnapshot, candidate ContextBootstrapSnapshot) []string {
	if current == nil {
		return []string{"aws"}
	}
	changes := []string{}
	add := func(name string, changed bool) {
		if changed {
			changes = append(changes, name)
		}
	}
	add("aws.profile", current.AWS.Profile != candidate.AWS.Profile)
	add("aws.region", current.AWS.Region != candidate.AWS.Region)
	add("aws.output", current.AWS.Output != candidate.AWS.Output)
	add("aws.sso_account_id", current.AWS.AccountID != candidate.AWS.AccountID)
	add("aws.sso_region", current.AWS.SSORegion != candidate.AWS.SSORegion)
	add("aws.sso_registration_scopes", strings.Join(current.AWS.SSORegistrationScopes, "\x00") != strings.Join(candidate.AWS.SSORegistrationScopes, "\x00"))
	add("aws.sso_role_name", current.AWS.RoleName != candidate.AWS.RoleName)
	add("aws.sso_session", current.AWS.SSOSession != candidate.AWS.SSOSession)
	add("aws.sso_start_url", current.AWS.SSOStartURL != candidate.AWS.SSOStartURL)
	sort.Strings(changes)
	return changes
}

type ContextBootstrapReport struct {
	State      string   `json:"state"`
	Generation uint64   `json:"generation,omitempty"`
	Revision   string   `json:"revision,omitempty"`
	Adapters   []string `json:"adapters"`
	AWSProfile string   `json:"aws_profile,omitempty"`
}

func ContextBootstrapReportFrom(snapshot *ContextBootstrapSnapshot) ContextBootstrapReport {
	if snapshot == nil {
		return ContextBootstrapReport{State: ContextBootstrapNotConfigured, Adapters: []string{}}
	}
	return ContextBootstrapReport{
		State: ContextBootstrapConfigured, Generation: snapshot.Generation, Revision: snapshot.Revision,
		Adapters: []string{ContextBootstrapAdapterAWS}, AWSProfile: snapshot.AWS.Profile,
	}
}

func (r ContextBootstrapReport) Validate() error {
	if r.State == "" && r.Generation == 0 && r.Revision == "" && r.AWSProfile == "" && r.Adapters == nil {
		return nil // legacy in-memory producers resolve this as not_configured
	}
	if r.Adapters == nil {
		return fmt.Errorf("Context bootstrap adapter collection must be explicit")
	}
	switch r.State {
	case ContextBootstrapNotConfigured:
		if r.Generation != 0 || r.Revision != "" || r.AWSProfile != "" || len(r.Adapters) != 0 {
			return fmt.Errorf("unconfigured Context bootstrap contains configured metadata")
		}
	case ContextBootstrapConfigured:
		if r.Generation == 0 || ValidateDigest(r.Revision) != nil || r.AWSProfile == "" || len(r.Adapters) != 1 || r.Adapters[0] != ContextBootstrapAdapterAWS {
			return fmt.Errorf("configured Context bootstrap metadata is invalid")
		}
	default:
		return fmt.Errorf("Context bootstrap state is invalid")
	}
	return nil
}

type WorkspaceBootstrapReport struct {
	State           string `json:"state"`
	AppliedRevision string `json:"applied_revision,omitempty"`
	CurrentRevision string `json:"current_revision,omitempty"`
}

func ResolveWorkspaceBootstrapReport(appliedRevision string, current *ContextBootstrapSnapshot) (WorkspaceBootstrapReport, error) {
	if appliedRevision != "" && ValidateDigest(appliedRevision) != nil {
		return WorkspaceBootstrapReport{}, fmt.Errorf("applied Workspace bootstrap revision is invalid")
	}
	if current == nil {
		if appliedRevision == "" {
			return WorkspaceBootstrapReport{State: WorkspaceBootstrapNotConfigured}, nil
		}
		return WorkspaceBootstrapReport{State: WorkspaceBootstrapOlder, AppliedRevision: appliedRevision}, nil
	}
	if err := current.Validate(); err != nil {
		return WorkspaceBootstrapReport{}, err
	}
	if appliedRevision == "" {
		return WorkspaceBootstrapReport{State: WorkspaceBootstrapNotApplied, CurrentRevision: current.Revision}, nil
	}
	state := WorkspaceBootstrapOlder
	if appliedRevision == current.Revision {
		state = WorkspaceBootstrapCurrent
	}
	return WorkspaceBootstrapReport{State: state, AppliedRevision: appliedRevision, CurrentRevision: current.Revision}, nil
}

func (r WorkspaceBootstrapReport) Validate() error {
	if r == (WorkspaceBootstrapReport{}) {
		return nil // legacy Workspace records and in-memory producers
	}
	switch r.State {
	case WorkspaceBootstrapNotConfigured:
		if r.AppliedRevision != "" || r.CurrentRevision != "" {
			return fmt.Errorf("unconfigured Workspace bootstrap has revisions")
		}
	case WorkspaceBootstrapNotApplied:
		if r.AppliedRevision != "" || ValidateDigest(r.CurrentRevision) != nil {
			return fmt.Errorf("not-applied Workspace bootstrap is invalid")
		}
	case WorkspaceBootstrapCurrent:
		if ValidateDigest(r.AppliedRevision) != nil || r.AppliedRevision != r.CurrentRevision {
			return fmt.Errorf("current Workspace bootstrap is invalid")
		}
	case WorkspaceBootstrapOlder:
		if ValidateDigest(r.AppliedRevision) != nil || (r.CurrentRevision != "" && ValidateDigest(r.CurrentRevision) != nil) || r.AppliedRevision == r.CurrentRevision {
			return fmt.Errorf("older Workspace bootstrap is invalid")
		}
	default:
		return fmt.Errorf("Workspace bootstrap state is invalid")
	}
	return nil
}

func (r ContextBootstrapReport) Resolved() ContextBootstrapReport {
	if r.State == "" {
		return ContextBootstrapReportFrom(nil)
	}
	return r
}

func (r WorkspaceBootstrapReport) Resolved() WorkspaceBootstrapReport {
	if r.State == "" {
		return WorkspaceBootstrapReport{State: WorkspaceBootstrapNotConfigured}
	}
	return r
}
