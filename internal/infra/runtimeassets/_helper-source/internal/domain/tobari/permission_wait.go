package tobari

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	PermissionWaitRecordSchema                                   = 2
	PermissionWaitLease                                          = 15 * time.Minute
	PermissionWaitMaxLive                                        = 8
	PermissionWaitMaxAttempts                                    = 3
	PermissionWaitRequestLimit                                   = 4 * 1024
	PermissionWaitResponseLimit                                  = 1024
	PermissionSessionLease                                       = 30 * time.Second
	PermissionSessionSchema                                      = 2
	PermissionSessionOwnerInteractive                            = "interactive_workspace"
	PermissionSessionTransportUnix    PermissionSessionTransport = "unix"
	PermissionSessionTransportTCP     PermissionSessionTransport = "loopback_tcp"

	PermissionWaitResultAllow   PermissionWaitResult = "allow"
	PermissionWaitResultDeny    PermissionWaitResult = "deny"
	PermissionWaitResultExpired PermissionWaitResult = "expired"
)

var (
	permissionWaitIDPattern        = regexp.MustCompile(`^pwt_[0-9a-f]{32}$`)
	permissionWaitPrincipalPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	permissionSessionNoncePattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	permissionSessionSocketPattern = regexp.MustCompile(`^pws_[0-9a-f]{32}\.sock$`)
)

// PermissionWaitResult is the complete successful helper result vocabulary.
// It carries no authority, evidence, policy, or retry operation.
type PermissionWaitResult string

// PermissionSessionTransport is the complete private ingestion adapter union.
// Trusted host composition selects one member; records cannot request a
// fallback or a different platform adapter.
type PermissionSessionTransport string

func (t PermissionSessionTransport) Validate() error {
	switch t {
	case PermissionSessionTransportUnix, PermissionSessionTransportTCP:
		return nil
	default:
		return fmt.Errorf("permission session transport is invalid")
	}
}

func (r PermissionWaitResult) Validate() error {
	switch r {
	case PermissionWaitResultAllow, PermissionWaitResultDeny, PermissionWaitResultExpired:
		return nil
	default:
		return fmt.Errorf("permission wait result is invalid")
	}
}

// InteractiveAttachmentSession is the bounded private successor registry
// record for the one canonical interactive owner. It is not a public resource.
type InteractiveAttachmentSession struct {
	SchemaVersion              int    `json:"schema_version"`
	WorkspaceManifestID        string `json:"workspace_manifest_id"`
	WorkspaceID                string `json:"workspace_id"`
	AttachmentID               string `json:"attachment_id"`
	OwnerKind                  string `json:"owner_kind"`
	FrozenPrincipalFingerprint string `json:"frozen_principal_fingerprint"`
	// OwnerPID is bounded liveness/audit correlation only. It is never a join
	// key without the owner-only socket, process-instance nonce, and epoch.
	OwnerPID           int                        `json:"owner_pid"`
	IngestionTransport PermissionSessionTransport `json:"ingestion_transport"`
	IngestionEndpoint  string                     `json:"ingestion_endpoint"`
	IngestionNonce     string                     `json:"ingestion_nonce"`
	CreatedAt          string                     `json:"created_at"`
	LeaseIssuedAt      string                     `json:"lease_issued_at"`
	ExpiresAt          string                     `json:"expires_at"`
}

func (s InteractiveAttachmentSession) Validate() error {
	if s.SchemaVersion != PermissionSessionSchema {
		return fmt.Errorf("interactive attachment session schema is invalid")
	}
	if ValidateWorkspaceManifestID(s.WorkspaceManifestID) != nil || ValidateWorkspaceID(s.WorkspaceID) != nil || ValidateAttachmentEpochID(s.AttachmentID) != nil {
		return fmt.Errorf("interactive attachment session identity is invalid")
	}
	if s.OwnerKind != PermissionSessionOwnerInteractive || !permissionWaitPrincipalPattern.MatchString(s.FrozenPrincipalFingerprint) {
		return fmt.Errorf("interactive attachment session join is invalid")
	}
	if s.OwnerPID < 1 || s.IngestionTransport.Validate() != nil || !permissionSessionNoncePattern.MatchString(s.IngestionNonce) {
		return fmt.Errorf("interactive attachment session owner endpoint is invalid")
	}
	switch s.IngestionTransport {
	case PermissionSessionTransportUnix:
		if !permissionSessionSocketPattern.MatchString(s.IngestionEndpoint) {
			return fmt.Errorf("interactive attachment session Unix endpoint is invalid")
		}
	case PermissionSessionTransportTCP:
		host, portText, found := strings.Cut(s.IngestionEndpoint, ":")
		port, err := strconv.Atoi(portText)
		if !found || host != "127.0.0.1" || err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != portText {
			return fmt.Errorf("interactive attachment session loopback endpoint is invalid")
		}
	}
	created, err := time.Parse(time.RFC3339Nano, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("interactive attachment session creation time is invalid")
	}
	issued, err := time.Parse(time.RFC3339Nano, s.LeaseIssuedAt)
	if err != nil || issued.Before(created) {
		return fmt.Errorf("interactive attachment session lease issue time is invalid")
	}
	expires, err := time.Parse(time.RFC3339Nano, s.ExpiresAt)
	if err != nil || !expires.After(issued) || expires.Sub(issued) > PermissionSessionLease {
		return fmt.Errorf("interactive attachment session lease is invalid")
	}
	return nil
}

// SameAuthority compares every stable owner field. Lease timestamps are the
// only mutable fields and must be checked through ValidateRenewal.
func (s InteractiveAttachmentSession) SameAuthority(other InteractiveAttachmentSession) bool {
	return s.SchemaVersion == other.SchemaVersion &&
		s.WorkspaceManifestID == other.WorkspaceManifestID && s.WorkspaceID == other.WorkspaceID &&
		s.AttachmentID == other.AttachmentID && s.OwnerKind == other.OwnerKind &&
		s.FrozenPrincipalFingerprint == other.FrozenPrincipalFingerprint &&
		s.OwnerPID == other.OwnerPID && s.IngestionTransport == other.IngestionTransport &&
		s.IngestionEndpoint == other.IngestionEndpoint && s.IngestionNonce == other.IngestionNonce &&
		s.CreatedAt == other.CreatedAt
}

// ValidateRenewal requires one exact owner and a strictly advancing lease
// issue time. Equal or rolled-back wall clocks fail closed.
func (s InteractiveAttachmentSession) ValidateRenewal(previous InteractiveAttachmentSession) error {
	if err := previous.Validate(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	if !s.SameAuthority(previous) {
		return fmt.Errorf("interactive attachment session authority changed")
	}
	previousIssued, _ := time.Parse(time.RFC3339Nano, previous.LeaseIssuedAt)
	issued, _ := time.Parse(time.RFC3339Nano, s.LeaseIssuedAt)
	if !issued.After(previousIssued) {
		return fmt.Errorf("interactive attachment session lease did not advance")
	}
	return nil
}

type InteractiveAttachmentSessionRegistry struct {
	SchemaVersion int                            `json:"schema_version"`
	Sessions      []InteractiveAttachmentSession `json:"sessions"`
}

func (r InteractiveAttachmentSessionRegistry) Validate() error {
	if r.SchemaVersion != PermissionSessionSchema || r.Sessions == nil || len(r.Sessions) > 128 {
		return fmt.Errorf("interactive attachment session registry is invalid")
	}
	pairs := make(map[string]struct{}, len(r.Sessions))
	epochs := make(map[string]struct{}, len(r.Sessions))
	endpoints := make(map[string]struct{}, len(r.Sessions))
	nonces := make(map[string]struct{}, len(r.Sessions))
	for _, session := range r.Sessions {
		if err := session.Validate(); err != nil {
			return err
		}
		pair := session.WorkspaceManifestID + "\x00" + session.WorkspaceID
		if _, exists := pairs[pair]; exists {
			return fmt.Errorf("interactive attachment session owner is ambiguous")
		}
		if _, exists := epochs[session.AttachmentID]; exists {
			return fmt.Errorf("interactive attachment session epoch is duplicated")
		}
		endpoint := string(session.IngestionTransport) + "\x00" + session.IngestionEndpoint
		if _, exists := endpoints[endpoint]; exists {
			return fmt.Errorf("interactive attachment session endpoint is duplicated")
		}
		if _, exists := nonces[session.IngestionNonce]; exists {
			return fmt.Errorf("interactive attachment session process identity is duplicated")
		}
		pairs[pair] = struct{}{}
		epochs[session.AttachmentID] = struct{}{}
		endpoints[endpoint] = struct{}{}
		nonces[session.IngestionNonce] = struct{}{}
	}
	return nil
}

// PermissionWaitAccessState is the pure per-record concurrency and reconnect
// state. Starting an attempt consumes its reconnect budget; only a terminal
// observation consumes the record.
type PermissionWaitAccessState struct {
	Attempts int
	Active   bool
	Consumed bool
}

func (s PermissionWaitAccessState) Validate() error {
	if s.Attempts < 0 || s.Attempts > PermissionWaitMaxAttempts {
		return fmt.Errorf("permission wait attempt count is invalid")
	}
	if s.Active && s.Consumed {
		return fmt.Errorf("consumed permission wait cannot remain active")
	}
	return nil
}

func (s PermissionWaitAccessState) StartAttempt() (PermissionWaitAccessState, error) {
	if err := s.Validate(); err != nil {
		return PermissionWaitAccessState{}, err
	}
	if s.Active || s.Consumed || s.Attempts >= PermissionWaitMaxAttempts {
		return PermissionWaitAccessState{}, fmt.Errorf("permission wait is unavailable")
	}
	s.Attempts++
	s.Active = true
	return s, nil
}

func (s PermissionWaitAccessState) FinishAttempt(terminal bool) (PermissionWaitAccessState, error) {
	if err := s.Validate(); err != nil {
		return PermissionWaitAccessState{}, err
	}
	if !s.Active {
		return PermissionWaitAccessState{}, fmt.Errorf("permission wait has no active attempt")
	}
	s.Active = false
	s.Consumed = terminal
	return s, nil
}

// NewPermissionWaitID creates a non-authoritative attachment-local correlation
// value from 128 bits of caller-supplied CSPRNG entropy.
func NewPermissionWaitID(source io.Reader) (string, error) {
	if source == nil {
		return "", fmt.Errorf("permission wait entropy source is required")
	}
	var entropy [16]byte
	if _, err := io.ReadFull(source, entropy[:]); err != nil {
		return "", fmt.Errorf("read permission wait entropy: %w", err)
	}
	return "pwt_" + hex.EncodeToString(entropy[:]), nil
}

func ValidatePermissionWaitID(value string) error {
	if !permissionWaitIDPattern.MatchString(value) {
		return fmt.Errorf("permission wait ID is invalid")
	}
	return nil
}

// PermissionWaitEffect is one exact normalized ordinary external HTTP effect.
// Protocol-derived identities and Host Loopback are deliberately unrepresentable.
type PermissionWaitEffect struct {
	Scheme   string   `json:"scheme"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Segments []string `json:"segments"`
}

func (e PermissionWaitEffect) Validate() error {
	if err := (PolicyProtocolIdentity{Scheme: e.Scheme, Protocol: PolicyProtocolHTTP}).Validate(); err != nil {
		return fmt.Errorf("permission wait effect scheme: %w", err)
	}
	if !validNormalizedPolicyHost(e.Host) || e.Host == HostLoopbackHostname {
		return fmt.Errorf("permission wait effect host is invalid")
	}
	if e.Port < 1 || e.Port > 65535 {
		return fmt.Errorf("permission wait effect port is invalid")
	}
	if !httpMethodPattern.MatchString(e.Method) {
		return fmt.Errorf("permission wait effect method is invalid")
	}
	if err := validatePolicyPath(e.Path); err != nil {
		return fmt.Errorf("permission wait effect path: %w", err)
	}
	segments, err := NormalizePermissionWaitPath(e.Path)
	if err != nil {
		return err
	}
	if e.Segments == nil || len(e.Segments) != len(segments) {
		return fmt.Errorf("permission wait effect segments do not match the path")
	}
	for index := range segments {
		if e.Segments[index] != segments[index] {
			return fmt.Errorf("permission wait effect segments do not match the path")
		}
	}
	return nil
}

// NormalizePermissionWaitPath defines the strict subset for which Go and
// Gateway can bind byte-equivalent ordinary HTTP path segments. Gateway's
// urllib.parse.unquote preserves malformed percent escapes and replaces invalid
// UTF-8; those ambiguous inputs are deliberately unsupported for resume.
func NormalizePermissionWaitPath(path string) ([]string, error) {
	result := make([]string, 0)
	for _, segment := range strings.Split(path, "/") {
		if segment == "" {
			continue
		}
		decoded := make([]byte, 0, len(segment))
		for index := 0; index < len(segment); index++ {
			if segment[index] != '%' {
				decoded = append(decoded, segment[index])
				continue
			}
			if index+2 >= len(segment) {
				return nil, fmt.Errorf("permission wait effect path has an invalid percent escape")
			}
			var pair [1]byte
			if _, err := hex.Decode(pair[:], []byte(segment[index+1:index+3])); err != nil {
				return nil, fmt.Errorf("permission wait effect path has an invalid percent escape")
			}
			decoded = append(decoded, pair[0])
			index += 2
		}
		if !utf8.Valid(decoded) || bytes.IndexFunc(decoded, func(r rune) bool {
			return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
		}) >= 0 {
			return nil, fmt.Errorf("permission wait effect path segment is invalid UTF-8")
		}
		result = append(result, string(decoded))
	}
	return result, nil
}

// PermissionWaitRecord is the immutable schema-2 owner-ingestion record. The
// fingerprint binds the exact authenticated frozen schema-v1 principal record
// without copying its compatibility field names into this successor schema.
type PermissionWaitRecord struct {
	SchemaVersion              int                  `json:"schema_version"`
	ID                         string               `json:"permission_wait_id"`
	DenialCorrelationID        string               `json:"denial_correlation_id"`
	FrozenPrincipalFingerprint string               `json:"frozen_principal_fingerprint"`
	WorkspaceManifestID        string               `json:"workspace_manifest_id"`
	WorkspaceID                string               `json:"workspace_id"`
	AttachmentID               string               `json:"attachment_id"`
	Effect                     PermissionWaitEffect `json:"effect"`
	CreatedAt                  string               `json:"created_at"`
	ExpiresAt                  string               `json:"expires_at"`
}

func (r PermissionWaitRecord) Validate() error {
	if r.SchemaVersion != PermissionWaitRecordSchema {
		return fmt.Errorf("permission wait record schema version is invalid")
	}
	if err := ValidatePermissionWaitID(r.ID); err != nil {
		return err
	}
	if !requestIDPattern.MatchString(r.DenialCorrelationID) {
		return fmt.Errorf("permission wait denial correlation is invalid")
	}
	if !permissionWaitPrincipalPattern.MatchString(r.FrozenPrincipalFingerprint) {
		return fmt.Errorf("permission wait principal fingerprint is invalid")
	}
	if err := ValidateWorkspaceManifestID(r.WorkspaceManifestID); err != nil {
		return fmt.Errorf("permission wait Workspace Manifest ID is invalid")
	}
	if err := ValidateWorkspaceID(r.WorkspaceID); err != nil {
		return fmt.Errorf("permission wait Workspace ID is invalid")
	}
	if err := ValidateAttachmentEpochID(r.AttachmentID); err != nil {
		return fmt.Errorf("permission wait attachment ID is invalid")
	}
	if err := r.Effect.Validate(); err != nil {
		return err
	}
	created, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return fmt.Errorf("permission wait creation time is invalid")
	}
	expires, err := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	if err != nil {
		return fmt.Errorf("permission wait expiry time is invalid")
	}
	if !expires.After(created) || expires.Sub(created) > PermissionWaitLease {
		return fmt.Errorf("permission wait lease is invalid")
	}
	return nil
}

// Expired reports lease expiry against a caller-supplied clock observation.
// It does not represent attachment teardown or transport loss.
func (r PermissionWaitRecord) Expired(now time.Time) (bool, error) {
	if err := r.Validate(); err != nil {
		return false, err
	}
	expires, _ := time.Parse(time.RFC3339Nano, r.ExpiresAt)
	return !now.Before(expires), nil
}
