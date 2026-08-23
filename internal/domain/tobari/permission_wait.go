package tobari

import (
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"time"
)

const (
	PermissionWaitRecordSchema  = 2
	PermissionWaitLease         = 15 * time.Minute
	PermissionWaitMaxLive       = 8
	PermissionWaitMaxAttempts   = 3
	PermissionWaitRequestLimit  = 4 * 1024
	PermissionWaitResponseLimit = 1024

	PermissionWaitResultAllow   PermissionWaitResult = "allow"
	PermissionWaitResultDeny    PermissionWaitResult = "deny"
	PermissionWaitResultExpired PermissionWaitResult = "expired"
)

var (
	permissionWaitIDPattern        = regexp.MustCompile(`^pwt_[0-9a-f]{32}$`)
	permissionWaitPrincipalPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// PermissionWaitResult is the complete successful helper result vocabulary.
// It carries no authority, evidence, policy, or retry operation.
type PermissionWaitResult string

func (r PermissionWaitResult) Validate() error {
	switch r {
	case PermissionWaitResultAllow, PermissionWaitResultDeny, PermissionWaitResultExpired:
		return nil
	default:
		return fmt.Errorf("permission wait result is invalid")
	}
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
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Method string `json:"method"`
	Path   string `json:"path"`
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
	return nil
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
