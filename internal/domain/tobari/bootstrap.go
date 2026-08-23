package tobari

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	ManifestBootstrapSchemaVersion = 1
	ManifestBootstrapAdapterAWS    = "aws_iam_identity_center"
	ManifestBootstrapAdapterEKS    = "kubernetes_eks"
	MaxContextBootstrapValueBytes  = 2048

	ManifestBootstrapTargetKind = "workspace-manifest-bootstrap"
	ManifestBootstrapTargetID   = "workspace-manifest-bootstrap"

	ManifestBootstrapNotConfigured = "not_configured"
	ManifestBootstrapConfigured    = "configured"

	WorkspaceBootstrapNotConfigured = "not_configured"
	WorkspaceBootstrapNotApplied    = "not_applied"
	WorkspaceBootstrapCurrent       = "current"
	WorkspaceBootstrapOlder         = "older"

	ManifestBootstrapDiscoveryAvailable   = "available"
	ManifestBootstrapDiscoveryMissing     = "missing"
	ManifestBootstrapDiscoveryRejected    = "rejected"
	ManifestBootstrapCandidateAvailable   = "available"
	ManifestBootstrapCandidateUnavailable = "unavailable"
)

var ErrContextBootstrapSourceChanged = fmt.Errorf("Workspace Manifest bootstrap source changed during review")
var ErrContextBootstrapDependency = fmt.Errorf("Workspace Manifest bootstrap adapter dependency is still configured")

var (
	awsBootstrapNamePattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	awsBootstrapAccountPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	awsBootstrapRolePattern      = regexp.MustCompile(`^[A-Za-z0-9+=,.@_-]{1,64}$`)
	awsBootstrapRegionPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}[a-z0-9]$`)
	awsBootstrapOutputPattern    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
	awsBootstrapScopePattern     = regexp.MustCompile(`^[\x21\x23-\x5B\x5D-\x7E]{1,128}$`)
	awsBootstrapStartURLPattern  = regexp.MustCompile(`^https://[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?\.awsapps\.com/start/?$`)
	eksBootstrapNamePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@/-]{0,252}$`)
	eksBootstrapClusterPattern   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,99}$`)
	eksBootstrapNamespacePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,61}[a-z0-9])?$`)
	eksBootstrapEndpointPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,250}[a-z0-9])?$`)
)

// ManifestAWSBootstrap is the closed, secret-free subset of one AWS shared
// config profile and its IAM Identity Center session. It contains no
// credential, token cache, helper, filesystem path, or executable selection.
type ManifestAWSBootstrap struct {
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

func (a ManifestAWSBootstrap) Validate() error {
	if err := ValidateContextAWSBootstrapProfileName(a.Profile); err != nil {
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

func ValidateContextAWSBootstrapProfileName(value string) error {
	if !awsBootstrapNamePattern.MatchString(value) {
		return fmt.Errorf("AWS bootstrap profile name is invalid")
	}
	return nil
}

func (a ManifestAWSBootstrap) Clone() ManifestAWSBootstrap {
	result := a
	result.SSORegistrationScopes = append([]string{}, a.SSORegistrationScopes...)
	return result
}

// ManifestAWSBootstrapCandidate is one profile resolved together with its
// referenced IAM Identity Center session. Unavailable candidates carry only a
// selector and bounded reason; they can never be used as authority.
type ManifestAWSBootstrapCandidate struct {
	Profile  string                     `json:"profile"`
	State    string                     `json:"state"`
	Reason   string                     `json:"reason,omitempty"`
	Snapshot *ManifestBootstrapSnapshot `json:"snapshot,omitempty"`
}

func (c ManifestAWSBootstrapCandidate) Validate() error {
	if !validBootstrapCandidateLabel(c.Profile, 64) {
		return fmt.Errorf("AWS bootstrap candidate profile is invalid")
	}
	switch c.State {
	case ManifestBootstrapCandidateAvailable:
		if ValidateContextAWSBootstrapProfileName(c.Profile) != nil {
			return fmt.Errorf("available AWS bootstrap candidate profile is invalid")
		}
		if c.Reason != "" || c.Snapshot == nil || c.Snapshot.EKS != nil || c.Snapshot.AWS.Profile != c.Profile {
			return fmt.Errorf("available AWS bootstrap candidate is incomplete")
		}
		return c.Snapshot.Validate()
	case ManifestBootstrapCandidateUnavailable:
		if c.Snapshot != nil || !validBootstrapDiscoveryReason(c.Reason) {
			return fmt.Errorf("unavailable AWS bootstrap candidate is invalid")
		}
		return nil
	default:
		return fmt.Errorf("AWS bootstrap candidate state is invalid")
	}
}

// ManifestEKSBootstrapCandidate is one kubeconfig context resolved against one
// already-reviewed AWS candidate. Available candidates contain the complete
// composed snapshot so presentation never infers compatibility from labels.
type ManifestEKSBootstrapCandidate struct {
	WorkspaceManifestName string                     `json:"manifest_name"`
	State                 string                     `json:"state"`
	Reason                string                     `json:"reason,omitempty"`
	Snapshot              *ManifestBootstrapSnapshot `json:"snapshot,omitempty"`
}

func (c ManifestEKSBootstrapCandidate) Validate(awsRevision string) error {
	if !validBootstrapCandidateLabel(c.WorkspaceManifestName, 253) {
		return fmt.Errorf("EKS bootstrap candidate context is invalid")
	}
	switch c.State {
	case ManifestBootstrapCandidateAvailable:
		if !eksBootstrapNamePattern.MatchString(c.WorkspaceManifestName) || strings.Contains(c.WorkspaceManifestName, "..") {
			return fmt.Errorf("available EKS bootstrap candidate context is invalid")
		}
		if c.Reason != "" || c.Snapshot == nil || c.Snapshot.EKS == nil || c.Snapshot.EKS.WorkspaceManifestName != c.WorkspaceManifestName {
			return fmt.Errorf("available EKS bootstrap candidate is incomplete")
		}
		if err := c.Snapshot.Validate(); err != nil {
			return err
		}
		base, err := NewContextBootstrapSnapshot(c.Snapshot.Generation, c.Snapshot.AWS)
		if err != nil || base.Revision != awsRevision {
			return fmt.Errorf("EKS bootstrap candidate does not bind the selected AWS semantic revision")
		}
		return nil
	case ManifestBootstrapCandidateUnavailable:
		if c.Snapshot != nil || !validBootstrapDiscoveryReason(c.Reason) {
			return fmt.Errorf("unavailable EKS bootstrap candidate is invalid")
		}
		return nil
	default:
		return fmt.Errorf("EKS bootstrap candidate state is invalid")
	}
}

type ManifestAWSBootstrapDiscovery struct {
	State      string                          `json:"state"`
	Reason     string                          `json:"reason,omitempty"`
	Candidates []ManifestAWSBootstrapCandidate `json:"candidates"`
}

func (d ManifestAWSBootstrapDiscovery) Validate() error {
	if d.Candidates == nil {
		return fmt.Errorf("AWS bootstrap candidate collection is absent")
	}
	if d.State != ManifestBootstrapDiscoveryAvailable {
		if (d.State != ManifestBootstrapDiscoveryMissing && d.State != ManifestBootstrapDiscoveryRejected) ||
			len(d.Candidates) != 0 || !validBootstrapDiscoveryReason(d.Reason) {
			return fmt.Errorf("AWS bootstrap discovery failure is invalid")
		}
		return nil
	}
	if d.Reason != "" {
		return fmt.Errorf("available AWS bootstrap discovery contains a failure reason")
	}
	seen := map[string]struct{}{}
	for _, candidate := range d.Candidates {
		if err := candidate.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[candidate.Profile]; duplicate {
			return fmt.Errorf("AWS bootstrap candidate is duplicated")
		}
		seen[candidate.Profile] = struct{}{}
	}
	return nil
}

type ManifestEKSBootstrapDiscovery struct {
	State       string                          `json:"state"`
	Reason      string                          `json:"reason,omitempty"`
	AWSRevision string                          `json:"aws_revision"`
	Candidates  []ManifestEKSBootstrapCandidate `json:"candidates"`
}

func (d ManifestEKSBootstrapDiscovery) Validate() error {
	if d.Candidates == nil || ValidateDigest(d.AWSRevision) != nil {
		return fmt.Errorf("EKS bootstrap discovery scope is invalid")
	}
	if d.State != ManifestBootstrapDiscoveryAvailable {
		if (d.State != ManifestBootstrapDiscoveryMissing && d.State != ManifestBootstrapDiscoveryRejected) ||
			len(d.Candidates) != 0 || !validBootstrapDiscoveryReason(d.Reason) {
			return fmt.Errorf("EKS bootstrap discovery failure is invalid")
		}
		return nil
	}
	if d.Reason != "" {
		return fmt.Errorf("available EKS bootstrap discovery contains a failure reason")
	}
	seen := map[string]struct{}{}
	for _, candidate := range d.Candidates {
		if err := candidate.Validate(d.AWSRevision); err != nil {
			return err
		}
		if _, duplicate := seen[candidate.WorkspaceManifestName]; duplicate {
			return fmt.Errorf("EKS bootstrap candidate is duplicated")
		}
		seen[candidate.WorkspaceManifestName] = struct{}{}
	}
	return nil
}

func validBootstrapDiscoveryReason(value string) bool {
	return value != "" && len(value) <= 512 && utf8.ValidString(value) && strings.IndexByte(value, 0) < 0 &&
		strings.IndexFunc(value, func(r rune) bool { return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029' }) < 0
}

func validBootstrapCandidateLabel(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && utf8.ValidString(value) && strings.IndexByte(value, 0) < 0
}

// ManifestEKSBootstrap is the closed, secret-free service target selected from
// one host kubeconfig. Authentication remains a canonical AWS CLI get-token
// exec bound to the sibling AWS bootstrap profile; no source exec bytes or
// credential material are retained.
type ManifestEKSBootstrap struct {
	WorkspaceManifestName    string `json:"manifest_name"`
	ClusterName              string `json:"cluster_name"`
	Region                   string `json:"region"`
	Server                   string `json:"server"`
	CertificateAuthorityData string `json:"certificate_authority_data"`
	Namespace                string `json:"namespace,omitempty"`
}

func (e ManifestEKSBootstrap) Validate() error {
	if !eksBootstrapNamePattern.MatchString(e.WorkspaceManifestName) || strings.Contains(e.WorkspaceManifestName, "..") {
		return fmt.Errorf("EKS bootstrap context name is invalid")
	}
	if !eksBootstrapClusterPattern.MatchString(e.ClusterName) {
		return fmt.Errorf("EKS bootstrap cluster name is invalid")
	}
	if !awsBootstrapRegionPattern.MatchString(e.Region) {
		return fmt.Errorf("EKS bootstrap region is invalid")
	}
	if e.Namespace != "" && !eksBootstrapNamespacePattern.MatchString(e.Namespace) {
		return fmt.Errorf("EKS bootstrap namespace is invalid")
	}
	if len(e.Server) == 0 || len(e.Server) > MaxContextBootstrapValueBytes {
		return fmt.Errorf("EKS bootstrap server is invalid")
	}
	if !strings.HasPrefix(e.Server, "https://") || strings.ContainsAny(strings.TrimPrefix(e.Server, "https://"), "/@:?#") {
		return fmt.Errorf("EKS bootstrap server must be an exact HTTPS origin")
	}
	host := strings.TrimPrefix(e.Server, "https://")
	suffix := "." + e.Region + ".eks.amazonaws.com"
	prefix := strings.TrimSuffix(host, suffix)
	if host == "" || host != strings.ToLower(host) || !strings.HasSuffix(host, suffix) || !eksBootstrapEndpointPattern.MatchString(prefix) || strings.Contains(prefix, "..") {
		return fmt.Errorf("EKS bootstrap server is not a canonical commercial EKS endpoint")
	}
	if len(e.CertificateAuthorityData) == 0 || len(e.CertificateAuthorityData) > 128*1024 {
		return fmt.Errorf("EKS bootstrap certificate authority data is invalid")
	}
	certificate, err := base64.StdEncoding.DecodeString(e.CertificateAuthorityData)
	if err != nil || len(certificate) == 0 || base64.StdEncoding.EncodeToString(certificate) != e.CertificateAuthorityData {
		return fmt.Errorf("EKS bootstrap certificate authority data is not canonical base64")
	}
	if pool := x509.NewCertPool(); !pool.AppendCertsFromPEM(certificate) {
		return fmt.Errorf("EKS bootstrap certificate authority data contains no certificate")
	}
	return nil
}

// ManifestBootstrapSnapshot is one immutable semantic revision used only when
// a future Workspace home is first created.
type ManifestBootstrapSnapshot struct {
	SchemaVersion int                   `json:"schema_version"`
	Generation    uint64                `json:"generation"`
	Revision      string                `json:"revision"`
	AWS           ManifestAWSBootstrap  `json:"aws"`
	EKS           *ManifestEKSBootstrap `json:"kubernetes_eks,omitempty"`
}

func NewContextBootstrapSnapshot(generation uint64, aws ManifestAWSBootstrap) (ManifestBootstrapSnapshot, error) {
	return newContextBootstrapSnapshot(generation, aws, nil)
}

func NewContextBootstrapSnapshotWithEKS(generation uint64, aws ManifestAWSBootstrap, eks ManifestEKSBootstrap) (ManifestBootstrapSnapshot, error) {
	return newContextBootstrapSnapshot(generation, aws, &eks)
}

func newContextBootstrapSnapshot(generation uint64, aws ManifestAWSBootstrap, eks *ManifestEKSBootstrap) (ManifestBootstrapSnapshot, error) {
	aws.SSORegistrationScopes = append([]string{}, aws.SSORegistrationScopes...)
	sort.Strings(aws.SSORegistrationScopes)
	if generation == 0 {
		return ManifestBootstrapSnapshot{}, fmt.Errorf("Workspace Manifest bootstrap generation must be positive")
	}
	if err := aws.Validate(); err != nil {
		return ManifestBootstrapSnapshot{}, err
	}
	if eks != nil {
		copy := *eks
		eks = &copy
		if err := eks.Validate(); err != nil {
			return ManifestBootstrapSnapshot{}, err
		}
	}
	snapshot := ManifestBootstrapSnapshot{SchemaVersion: ManifestBootstrapSchemaVersion, Generation: generation, AWS: aws, EKS: eks}
	snapshot.Revision = snapshot.semanticRevision()
	return snapshot, nil
}

func (s ManifestBootstrapSnapshot) semanticRevision() string {
	fields := []string{
		fmt.Sprintf("schema=%d", s.SchemaVersion),
		"adapter=" + ManifestBootstrapAdapterAWS,
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
	if s.EKS != nil {
		fields = append(fields,
			"adapter="+ManifestBootstrapAdapterEKS,
			"eks.manifest_name="+s.EKS.WorkspaceManifestName,
			"eks.cluster_name="+s.EKS.ClusterName,
			"eks.region="+s.EKS.Region,
			"eks.server="+s.EKS.Server,
			"eks.certificate_authority_data="+s.EKS.CertificateAuthorityData,
			"eks.namespace="+s.EKS.Namespace,
		)
	}
	digest := sha256.Sum256([]byte(strings.Join(fields, "\x00")))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s ManifestBootstrapSnapshot) Validate() error {
	if s.SchemaVersion != ManifestBootstrapSchemaVersion || s.Generation == 0 {
		return fmt.Errorf("Workspace Manifest bootstrap snapshot version or generation is invalid")
	}
	if err := s.AWS.Validate(); err != nil {
		return err
	}
	if s.EKS != nil {
		if err := s.EKS.Validate(); err != nil {
			return err
		}
	}
	if s.Revision != s.semanticRevision() {
		return fmt.Errorf("Workspace Manifest bootstrap revision does not match its semantic snapshot")
	}
	return nil
}

func (s ManifestBootstrapSnapshot) Clone() ManifestBootstrapSnapshot {
	result := s
	result.AWS = s.AWS.Clone()
	if s.EKS != nil {
		copy := *s.EKS
		result.EKS = &copy
	}
	return result
}

type ManifestBootstrapPreview struct {
	WorkspaceManifestName string                  `json:"workspace_manifest"`
	Current               ManifestBootstrapReport `json:"current"`
	Candidate             ManifestBootstrapReport `json:"candidate"`
	Changes               []string                `json:"changes"`
}

func NewContextBootstrapPreview(contextName string, current *ManifestBootstrapSnapshot, candidate ManifestBootstrapSnapshot) (ManifestBootstrapPreview, error) {
	if err := ValidateName(contextName); err != nil {
		return ManifestBootstrapPreview{}, err
	}
	if err := candidate.Validate(); err != nil {
		return ManifestBootstrapPreview{}, err
	}
	changes := diffContextAWSBootstrap(current, candidate)
	result := ManifestBootstrapPreview{WorkspaceManifestName: contextName, Current: ManifestBootstrapReportFrom(current), Candidate: ManifestBootstrapReportFrom(&candidate), Changes: changes}
	return result, result.Validate()
}

func (p ManifestBootstrapPreview) Validate() error {
	if err := ValidateName(p.WorkspaceManifestName); err != nil {
		return err
	}
	if err := p.Current.Validate(); err != nil {
		return err
	}
	if err := p.Candidate.Validate(); err != nil {
		return err
	}
	if p.Candidate.Resolved().State != ManifestBootstrapConfigured || p.Changes == nil {
		return fmt.Errorf("Workspace Manifest bootstrap preview is incomplete")
	}
	for index, change := range p.Changes {
		if change == "" || (index > 0 && p.Changes[index-1] >= change) {
			return fmt.Errorf("Workspace Manifest bootstrap preview changes are invalid")
		}
	}
	if len(p.Changes) == 0 && p.Current.Resolved().Revision != p.Candidate.Revision {
		return fmt.Errorf("Workspace Manifest bootstrap preview misses a semantic change")
	}
	return nil
}

func diffContextAWSBootstrap(current *ManifestBootstrapSnapshot, candidate ManifestBootstrapSnapshot) []string {
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
	if current.EKS == nil && candidate.EKS != nil {
		changes = append(changes, "kubernetes_eks")
	} else if current.EKS != nil && candidate.EKS == nil {
		changes = append(changes, "kubernetes_eks")
	} else if current.EKS != nil && candidate.EKS != nil {
		add("kubernetes_eks.manifest_name", current.EKS.WorkspaceManifestName != candidate.EKS.WorkspaceManifestName)
		add("kubernetes_eks.cluster_name", current.EKS.ClusterName != candidate.EKS.ClusterName)
		add("kubernetes_eks.region", current.EKS.Region != candidate.EKS.Region)
		add("kubernetes_eks.server", current.EKS.Server != candidate.EKS.Server)
		add("kubernetes_eks.certificate_authority_data", current.EKS.CertificateAuthorityData != candidate.EKS.CertificateAuthorityData)
		add("kubernetes_eks.namespace", current.EKS.Namespace != candidate.EKS.Namespace)
	}
	sort.Strings(changes)
	return changes
}

type ManifestBootstrapReport struct {
	State      string   `json:"state"`
	Generation uint64   `json:"generation,omitempty"`
	Revision   string   `json:"revision,omitempty"`
	Adapters   []string `json:"adapters"`
	AWSProfile string   `json:"aws_profile,omitempty"`
	EKSContext string   `json:"kubernetes_eks_context,omitempty"`
}

func ManifestBootstrapReportFrom(snapshot *ManifestBootstrapSnapshot) ManifestBootstrapReport {
	if snapshot == nil {
		return ManifestBootstrapReport{State: ManifestBootstrapNotConfigured, Adapters: []string{}}
	}
	report := ManifestBootstrapReport{
		State: ManifestBootstrapConfigured, Generation: snapshot.Generation, Revision: snapshot.Revision,
		Adapters: []string{ManifestBootstrapAdapterAWS}, AWSProfile: snapshot.AWS.Profile,
	}
	if snapshot.EKS != nil {
		report.Adapters = append(report.Adapters, ManifestBootstrapAdapterEKS)
		report.EKSContext = snapshot.EKS.WorkspaceManifestName
	}
	return report
}

func (r ManifestBootstrapReport) Validate() error {
	if r.State == "" && r.Generation == 0 && r.Revision == "" && r.AWSProfile == "" && r.EKSContext == "" && r.Adapters == nil {
		return nil // legacy in-memory producers resolve this as not_configured
	}
	if r.Adapters == nil {
		return fmt.Errorf("Workspace Manifest bootstrap adapter collection must be explicit")
	}
	switch r.State {
	case ManifestBootstrapNotConfigured:
		if r.Generation != 0 || r.Revision != "" || r.AWSProfile != "" || r.EKSContext != "" || len(r.Adapters) != 0 {
			return fmt.Errorf("unconfigured Workspace Manifest bootstrap contains configured metadata")
		}
	case ManifestBootstrapConfigured:
		if r.Generation == 0 || ValidateDigest(r.Revision) != nil || r.AWSProfile == "" || len(r.Adapters) < 1 || len(r.Adapters) > 2 || r.Adapters[0] != ManifestBootstrapAdapterAWS {
			return fmt.Errorf("configured Workspace Manifest bootstrap metadata is invalid")
		}
		if len(r.Adapters) == 1 && r.EKSContext != "" {
			return fmt.Errorf("configured Workspace Manifest bootstrap EKS metadata is inconsistent")
		}
		if len(r.Adapters) == 2 && (r.Adapters[1] != ManifestBootstrapAdapterEKS || r.EKSContext == "") {
			return fmt.Errorf("configured Workspace Manifest bootstrap EKS metadata is invalid")
		}
	default:
		return fmt.Errorf("Workspace Manifest bootstrap state is invalid")
	}
	return nil
}

type WorkspaceBootstrapReport struct {
	State           string `json:"state"`
	AppliedRevision string `json:"applied_revision,omitempty"`
	CurrentRevision string `json:"current_revision,omitempty"`
}

func ResolveWorkspaceBootstrapReport(appliedRevision string, current *ManifestBootstrapSnapshot) (WorkspaceBootstrapReport, error) {
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

func (r ManifestBootstrapReport) Resolved() ManifestBootstrapReport {
	if r.State == "" {
		return ManifestBootstrapReportFrom(nil)
	}
	return r
}

func (r WorkspaceBootstrapReport) Resolved() WorkspaceBootstrapReport {
	if r.State == "" {
		return WorkspaceBootstrapReport{State: WorkspaceBootstrapNotConfigured}
	}
	return r
}
