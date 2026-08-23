package tobari

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	HostLoopbackHostname           = "host.tobari.test"
	HostLoopbackURLTemplate        = "http://host.tobari.test:{port}"
	HostLoopbackCapabilitySchema   = 1
	HostLoopbackRegistrySchema     = 1
	MinHostLoopbackPort            = 1024
	MaxHostLoopbackPort            = 65535
	AuthorityLifetimePersistent    = "persistent"
	AuthorityLifetimeAttachment    = "attachment"
	PolicyDestinationExternal      = "external"
	PolicyDestinationHostLoopback  = "host_loopback"
	HostLoopbackAudienceWorkspace  = "workspace"
	HostLoopbackAccessPolicyReview = "policy_review_required"
	HostLoopbackProtocolHTTP       = "http"
)

var (
	attachmentEpochPattern   = regexp.MustCompile(`^att_[0-9a-f]{32}$`)
	hostLoopbackRoutePattern = regexp.MustCompile(`^hlr_[0-9a-f]{32}$`)
	attachmentGrantPattern   = regexp.MustCompile(`^pag_[0-9a-f]{32}$`)
	relayTokenPattern        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func ValidateHostLoopbackPort(port int) error {
	if port < MinHostLoopbackPort || port > MaxHostLoopbackPort {
		return fmt.Errorf("host loopback port must be between %d and %d", MinHostLoopbackPort, MaxHostLoopbackPort)
	}
	return nil
}

func ValidateAttachmentEpochID(value string) error {
	if !attachmentEpochPattern.MatchString(value) {
		return fmt.Errorf("attachment epoch ID is invalid")
	}
	return nil
}

func hostLoopbackRouteID(epochID, contextID, projectID string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"tobari-host-loopback-route-v1", epochID, contextID, projectID,
	}, "\x00")))
	return "hlr_" + hex.EncodeToString(sum[:16])
}

// AttachmentHostLoopbackRoute is the one active transport route for a
// Workspace. Relay coordinates never enter Workspace capability information,
// OPA input, audit, or review output.
type AttachmentHostLoopbackRoute struct {
	ID      string `json:"id"`
	EpochID string `json:"attachment_epoch_id"`
	// These three tags are frozen Gateway route schema-v1 tokens. Current Go
	// names retain the Workspace/Manifest domain model without creating a wire
	// alias that the Gateway could interpret ambiguously.
	WorkspaceManifestID   string `json:"context_id"`
	WorkspaceManifestName string `json:"context"`
	ProjectID             string `json:"project_id"`
	ProjectRoot           string `json:"project_root"`
	Hostname              string `json:"hostname"`
	RelayPort             int    `json:"relay_port"`
	RelayToken            string `json:"relay_token"`
}

func NewAttachmentHostLoopbackRoute(
	epochID string, project Workspace, relayPort int, relayToken string,
) (AttachmentHostLoopbackRoute, error) {
	route := AttachmentHostLoopbackRoute{
		ID: hostLoopbackRouteID(epochID, project.WorkspaceManifestID, project.ID), EpochID: epochID,
		WorkspaceManifestID: project.WorkspaceManifestID, WorkspaceManifestName: project.WorkspaceManifestName,
		ProjectID: project.ID, ProjectRoot: project.Root,
		Hostname: HostLoopbackHostname, RelayPort: relayPort, RelayToken: relayToken,
	}
	if err := route.Validate(); err != nil {
		return AttachmentHostLoopbackRoute{}, err
	}
	return route, nil
}

func (r AttachmentHostLoopbackRoute) Validate() error {
	if !hostLoopbackRoutePattern.MatchString(r.ID) || r.ID != hostLoopbackRouteID(r.EpochID, r.WorkspaceManifestID, r.ProjectID) {
		return fmt.Errorf("host loopback route ID is invalid")
	}
	if err := ValidateAttachmentEpochID(r.EpochID); err != nil {
		return err
	}
	if err := ValidateWorkspaceManifestID(r.WorkspaceManifestID); err != nil {
		return fmt.Errorf("host loopback Workspace Manifest ID is invalid")
	}
	if err := ValidateName(r.WorkspaceManifestName); err != nil {
		return fmt.Errorf("host loopback Workspace Manifest name is invalid")
	}
	if err := ValidateWorkspaceID(r.ProjectID); err != nil {
		return fmt.Errorf("host loopback project ID is invalid")
	}
	if r.ProjectRoot == "" || r.ProjectRoot[0] != '/' {
		return fmt.Errorf("host loopback project root is invalid")
	}
	if r.Hostname != HostLoopbackHostname || r.RelayPort < MinHostLoopbackPort || r.RelayPort > MaxHostLoopbackPort || !relayTokenPattern.MatchString(r.RelayToken) {
		return fmt.Errorf("host loopback relay authority is invalid")
	}
	return nil
}

type HostLoopbackRegistry struct {
	SchemaVersion int                           `json:"schema_version"`
	Routes        []AttachmentHostLoopbackRoute `json:"routes"`
}

func (r HostLoopbackRegistry) Validate() error {
	if r.SchemaVersion != HostLoopbackRegistrySchema || r.Routes == nil || len(r.Routes) > 128 {
		return fmt.Errorf("host loopback registry is invalid")
	}
	seenIDs := map[string]struct{}{}
	seenProjects := map[string]struct{}{}
	for _, route := range r.Routes {
		if err := route.Validate(); err != nil {
			return err
		}
		if _, exists := seenIDs[route.ID]; exists {
			return fmt.Errorf("host loopback route ID is duplicated")
		}
		if _, exists := seenProjects[route.ProjectID]; exists {
			return fmt.Errorf("host loopback registry has several routes for one Workspace")
		}
		seenIDs[route.ID] = struct{}{}
		seenProjects[route.ProjectID] = struct{}{}
	}
	return nil
}

type HostLoopbackCapability struct {
	URLTemplate string `json:"url_template"`
	MinimumPort int    `json:"minimum_port"`
	MaximumPort int    `json:"maximum_port"`
	Lifetime    string `json:"lifetime"`
	Audience    string `json:"audience"`
	Access      string `json:"access"`
}

type HostLoopbackCapabilityProjection struct {
	SchemaVersion     int                    `json:"schema_version"`
	LocalhostMeans    string                 `json:"localhost_means"`
	HostHTTP          HostLoopbackCapability `json:"host_http"`
	HostDockerControl string                 `json:"host_docker_control"`
}

func NewHostLoopbackCapabilityProjection() HostLoopbackCapabilityProjection {
	return HostLoopbackCapabilityProjection{
		SchemaVersion:  HostLoopbackCapabilitySchema,
		LocalhostMeans: "workspace",
		HostHTTP: HostLoopbackCapability{
			URLTemplate: HostLoopbackURLTemplate,
			MinimumPort: MinHostLoopbackPort, MaximumPort: MaxHostLoopbackPort,
			Lifetime: AuthorityLifetimeAttachment, Audience: HostLoopbackAudienceWorkspace,
			Access: HostLoopbackAccessPolicyReview,
		},
		HostDockerControl: "unavailable",
	}
}

func (p HostLoopbackCapabilityProjection) Validate() error {
	if p != NewHostLoopbackCapabilityProjection() {
		return fmt.Errorf("host loopback capability projection is invalid")
	}
	return nil
}

// AttachmentGrant is separate from LearnedPolicyRule and can authorize only
// one exact Host Loopback effect for one active attachment.
type AttachmentGrant struct {
	ID                  string `json:"id"`
	Decision            string `json:"decision"`
	Lifetime            string `json:"lifetime"`
	DestinationKind     string `json:"destination_kind"`
	WorkspaceManifestID string `json:"context_id"`
	ProjectID           string `json:"project_id"`
	EpochID             string `json:"attachment_epoch_id"`
	Hostname            string `json:"host"`
	TargetPort          int    `json:"target_port"`
	Method              string `json:"method"`
	Path                string `json:"path"`
	SourceCandidate     string `json:"source_candidate"`
}

func NewAttachmentGrantFromCandidate(decision string, candidate PolicyCandidate) (AttachmentGrant, error) {
	if err := candidate.Validate(); err != nil {
		return AttachmentGrant{}, err
	}
	if candidate.EffectiveDestinationKind() != PolicyDestinationHostLoopback || candidate.EffectiveAuthorityLifetime() != AuthorityLifetimeAttachment {
		return AttachmentGrant{}, fmt.Errorf("policy candidate is not attachment-scoped host loopback")
	}
	grant := AttachmentGrant{
		Decision: decision, Lifetime: AuthorityLifetimeAttachment, DestinationKind: PolicyDestinationHostLoopback,
		WorkspaceManifestID: candidate.WorkspaceManifestID, ProjectID: candidate.ProjectID, EpochID: candidate.AttachmentEpochID,
		Hostname: candidate.Host, TargetPort: candidate.Port, Method: candidate.Method, Path: candidate.Path,
		SourceCandidate: candidate.ID,
	}
	grant.ID = attachmentGrantID(grant)
	if err := grant.Validate(); err != nil {
		return AttachmentGrant{}, err
	}
	return grant, nil
}

func attachmentGrantID(g AttachmentGrant) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"tobari-attachment-grant-v2", g.Decision, g.WorkspaceManifestID, g.ProjectID, g.EpochID,
		g.Hostname, strconv.Itoa(g.TargetPort), g.Method, g.Path, g.SourceCandidate,
	}, "\x00")))
	return "pag_" + hex.EncodeToString(sum[:16])
}

func (g AttachmentGrant) Validate() error {
	if !attachmentGrantPattern.MatchString(g.ID) || g.ID != attachmentGrantID(g) {
		return fmt.Errorf("attachment grant ID is invalid")
	}
	if g.Decision != PolicyDecisionAllow && g.Decision != PolicyDecisionDeny {
		return fmt.Errorf("attachment grant decision is invalid")
	}
	if g.Lifetime != AuthorityLifetimeAttachment || g.DestinationKind != PolicyDestinationHostLoopback {
		return fmt.Errorf("attachment grant authority kind is invalid")
	}
	if err := ValidateWorkspaceManifestID(g.WorkspaceManifestID); err != nil {
		return err
	}
	if err := ValidateWorkspaceID(g.ProjectID); err != nil {
		return err
	}
	if err := ValidateAttachmentEpochID(g.EpochID); err != nil {
		return err
	}
	if g.Hostname != HostLoopbackHostname || ValidateHostLoopbackPort(g.TargetPort) != nil {
		return fmt.Errorf("attachment grant host loopback target is invalid")
	}
	if !httpMethodPattern.MatchString(g.Method) {
		return fmt.Errorf("attachment grant method is invalid")
	}
	if err := validatePolicyPath(g.Path); err != nil {
		return fmt.Errorf("attachment grant path is invalid")
	}
	if err := ValidatePolicyCandidateID(g.SourceCandidate); err != nil {
		return err
	}
	return nil
}

type AttachmentGrantRegistry struct {
	SchemaVersion int               `json:"schema_version"`
	Grants        []AttachmentGrant `json:"grants"`
}

func (r AttachmentGrantRegistry) Validate() error {
	if r.SchemaVersion != HostLoopbackRegistrySchema || r.Grants == nil || len(r.Grants) > 512 {
		return fmt.Errorf("attachment grant registry is invalid")
	}
	seen := make(map[string]struct{}, len(r.Grants))
	for _, grant := range r.Grants {
		if err := grant.Validate(); err != nil {
			return err
		}
		key := strings.Join([]string{grant.ProjectID, grant.EpochID, strconv.Itoa(grant.TargetPort), grant.Method, grant.Path}, "\x00")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("attachment grant effect is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}
