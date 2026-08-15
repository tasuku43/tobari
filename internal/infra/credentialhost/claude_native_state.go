package credentialhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

const (
	ClaudeNativeDriverID     = "anthropic_claude_native_oauth"
	ClaudeNativeAccountLabel = "claude-user-native"
	claudeNativeVersion      = "2.1.220"
	maxClaudeNativeState     = 32 << 10
	maxClaudeNativeToken     = 16 << 10
)

var (
	ErrInvalidClaudeNativeCredential = errors.New("Claude native OAuth credential state is invalid")
	ErrClaudeExecutable              = errors.New("Claude Code executable is invalid")
	ErrClaudeVersion                 = errors.New("Claude Code version is unsupported")
	ErrClaudeLoginSetup              = errors.New("Claude Code login setup failed")
	ErrClaudeTTYRequired             = errors.New("Claude Code login terminal streams are required")
	ErrClaudeLoginCancelled          = errors.New("Claude Code login was cancelled")
	ErrClaudeLoginFailed             = errors.New("Claude Code login failed")
	ErrClaudeOutputLimit             = errors.New("Claude Code output exceeded its limit")
	ErrClaudeTokenCapture            = errors.New("Claude Code native credential capture failed")
	ErrClaudeLoginCleanup            = errors.New("Claude Code login cleanup failed")
	claudeImageIDPattern             = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ClaudeNativeCredential is the canonical encrypted-at-rest form of the
// native Linux credential written by one exact Context-image Claude client.
// It is always redacted; Encode is the only complete disclosure path.
type ClaudeNativeCredential struct {
	payload claudeNativeCredentialPayload
}

type ClaudeCredentialCaptureStage string

const (
	ClaudeCaptureFileExport       ClaudeCredentialCaptureStage = "credential_export"
	ClaudeCaptureArchiveEnvelope  ClaudeCredentialCaptureStage = "archive_envelope"
	ClaudeCaptureFilePermissions  ClaudeCredentialCaptureStage = "file_permissions"
	ClaudeCaptureDocumentEncoding ClaudeCredentialCaptureStage = "document_encoding"
	ClaudeCaptureDocumentJSON     ClaudeCredentialCaptureStage = "document_json"
	ClaudeCaptureOAuthRecord      ClaudeCredentialCaptureStage = "oauth_record"
	ClaudeCaptureOAuthCoreFields  ClaudeCredentialCaptureStage = "oauth_core_fields"
	ClaudeCaptureOAuthValueShape  ClaudeCredentialCaptureStage = "oauth_token_shape"
	ClaudeCaptureOAuthExpiry      ClaudeCredentialCaptureStage = "oauth_expiry"
	ClaudeCaptureOAuthScopeSet    ClaudeCredentialCaptureStage = "oauth_scope_set"
	ClaudeCaptureOAuthEntitlement ClaudeCredentialCaptureStage = "oauth_entitlement"
	ClaudeCaptureCanonicalRecord  ClaudeCredentialCaptureStage = "canonical_record"
)

// ClaudeCredentialCaptureError identifies only a fixed, secret-free capture
// stage. It never retains provider output or a credential value.
type ClaudeCredentialCaptureError struct {
	stage ClaudeCredentialCaptureStage
	cause error
}

func NewClaudeCredentialCaptureError(stage ClaudeCredentialCaptureStage, cause error) error {
	if !validClaudeCredentialCaptureStage(stage) {
		stage = ClaudeCaptureCanonicalRecord
	}
	safeCause := ErrInvalidClaudeNativeCredential
	if errors.Is(cause, ErrClaudeTokenCapture) {
		safeCause = ErrClaudeTokenCapture
	}
	return &ClaudeCredentialCaptureError{stage: stage, cause: safeCause}
}

func validClaudeCredentialCaptureStage(stage ClaudeCredentialCaptureStage) bool {
	switch stage {
	case ClaudeCaptureFileExport, ClaudeCaptureArchiveEnvelope, ClaudeCaptureFilePermissions,
		ClaudeCaptureDocumentEncoding, ClaudeCaptureDocumentJSON, ClaudeCaptureOAuthRecord,
		ClaudeCaptureOAuthCoreFields, ClaudeCaptureOAuthValueShape, ClaudeCaptureOAuthExpiry,
		ClaudeCaptureOAuthScopeSet, ClaudeCaptureOAuthEntitlement, ClaudeCaptureCanonicalRecord:
		return true
	default:
		return false
	}
}

func (e *ClaudeCredentialCaptureError) Error() string {
	return "Claude Code native credential capture failed at diagnostic stage " + string(e.stage)
}

func (e *ClaudeCredentialCaptureError) Unwrap() error { return e.cause }

func (e *ClaudeCredentialCaptureError) DiagnosticStage() ClaudeCredentialCaptureStage {
	return e.stage
}

type claudeNativeCredentialPayload struct {
	SchemaVersion int                      `json:"schema_version"`
	Executable    claudeNativeExecutable   `json:"claude_executable"`
	Session       claudeNativeOAuthSession `json:"session"`
}

type claudeNativeExecutable struct {
	ImageID string `json:"image_id"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

type claudeNativeAuthFile struct {
	OAuth claudeNativeOAuthSession `json:"claudeAiOauth"`
}

// claudeNativeOAuthSession is Tobari's owned at-rest subset of the pinned
// Claude file. The two entitlement labels are non-secret native-client state;
// account identities and refresh-expiry metadata are deliberately excluded.
type claudeNativeOAuthSession struct {
	AccessToken      string   `json:"access_token"`
	RefreshToken     string   `json:"refresh_token"`
	ExpiresAt        int64    `json:"expires_at"`
	Scopes           []string `json:"scopes"`
	SubscriptionType string   `json:"subscription_type"`
	RateLimitTier    string   `json:"rate_limit_tier"`
}

func NewClaudeNativeCredential(
	authJSON []byte,
	imageID string,
	executableDigest string,
	observedVersion string,
) (ClaudeNativeCredential, error) {
	auth, err := parseClaudeNativeAuth(authJSON)
	if err != nil {
		return ClaudeNativeCredential{}, err
	}
	credential := ClaudeNativeCredential{payload: claudeNativeCredentialPayload{
		SchemaVersion: 1,
		Executable: claudeNativeExecutable{
			ImageID: imageID, Path: "/usr/local/bin/claude", SHA256: executableDigest, Version: observedVersion,
		},
		Session: auth.OAuth,
	}}
	if err := validateClaudeNativeCredentialPayload(credential.payload); err != nil {
		credential.Clear()
		return ClaudeNativeCredential{}, NewClaudeCredentialCaptureError(ClaudeCaptureCanonicalRecord, err)
	}
	return credential, nil
}

func DecodeClaudeNativeCredential(encoded []byte) (ClaudeNativeCredential, error) {
	if len(encoded) == 0 || len(encoded) > maxClaudeNativeState || !utf8.Valid(encoded) {
		return ClaudeNativeCredential{}, ErrInvalidClaudeNativeCredential
	}
	if _, err := decodeGitHubJSON(encoded); err != nil {
		return ClaudeNativeCredential{}, ErrInvalidClaudeNativeCredential
	}
	var payload claudeNativeCredentialPayload
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return ClaudeNativeCredential{}, ErrInvalidClaudeNativeCredential
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ClaudeNativeCredential{}, ErrInvalidClaudeNativeCredential
	}
	if err := validateClaudeNativeCredentialPayload(payload); err != nil {
		return ClaudeNativeCredential{}, err
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return ClaudeNativeCredential{}, ErrInvalidClaudeNativeCredential
	}
	return ClaudeNativeCredential{payload: cloneClaudeNativePayload(payload)}, nil
}

func (c ClaudeNativeCredential) Encode() ([]byte, error) {
	if err := validateClaudeNativeCredentialPayload(c.payload); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(c.payload)
	if err != nil || len(encoded) == 0 || len(encoded) > maxClaudeNativeState {
		return nil, ErrInvalidClaudeNativeCredential
	}
	return encoded, nil
}

func (c ClaudeNativeCredential) AccountLabel() string   { return ClaudeNativeAccountLabel }
func (c ClaudeNativeCredential) DriverID() string       { return ClaudeNativeDriverID }
func (c ClaudeNativeCredential) DriverRevision() string { return c.payload.Executable.SHA256 }
func (c ClaudeNativeCredential) OAuthScopes() []string {
	return append([]string(nil), c.payload.Session.Scopes...)
}
func (ClaudeNativeCredential) String() string {
	return "credentialhost.ClaudeNativeCredential{redacted}"
}
func (ClaudeNativeCredential) GoString() string {
	return "credentialhost.ClaudeNativeCredential{redacted}"
}

func (c *ClaudeNativeCredential) Clear() {
	if c == nil {
		return
	}
	c.payload.Session.AccessToken = ""
	c.payload.Session.RefreshToken = ""
	c.payload = claudeNativeCredentialPayload{}
}

func parseClaudeNativeAuth(encoded []byte) (claudeNativeAuthFile, error) {
	if len(encoded) == 0 || len(encoded) > maxClaudeNativeState || !utf8.Valid(encoded) {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureDocumentEncoding, ErrInvalidClaudeNativeCredential)
	}
	decoded, err := decodeGitHubJSON(encoded)
	if err != nil {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureDocumentJSON, ErrInvalidClaudeNativeCredential)
	}
	root, ok := decoded.(map[string]any)
	if !ok {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureDocumentJSON, ErrInvalidClaudeNativeCredential)
	}
	oauthDocument, ok := root["claudeAiOauth"].(map[string]any)
	if !ok {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthRecord, ErrInvalidClaudeNativeCredential)
	}
	auth := claudeNativeAuthFile{OAuth: claudeNativeOAuthSession{}}
	if auth.OAuth.AccessToken, ok = oauthDocument["accessToken"].(string); !ok {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthCoreFields, ErrInvalidClaudeNativeCredential)
	}
	if auth.OAuth.RefreshToken, ok = oauthDocument["refreshToken"].(string); !ok {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthCoreFields, ErrInvalidClaudeNativeCredential)
	}
	if auth.OAuth.ExpiresAt, ok = claudeNativeJSONInt64(oauthDocument["expiresAt"]); !ok {
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthCoreFields, ErrInvalidClaudeNativeCredential)
	}
	rawScopes, ok := oauthDocument["scopes"].([]any)
	if !ok {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthCoreFields, ErrInvalidClaudeNativeCredential)
	}
	auth.OAuth.Scopes = make([]string, len(rawScopes))
	for index, rawScope := range rawScopes {
		scope, valid := rawScope.(string)
		if !valid {
			clearClaudeNativeAuth(&auth)
			return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthCoreFields, ErrInvalidClaudeNativeCredential)
		}
		auth.OAuth.Scopes[index] = scope
	}
	if auth.OAuth.SubscriptionType, ok = oauthDocument["subscriptionType"].(string); !ok {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthEntitlement, ErrInvalidClaudeNativeCredential)
	}
	if auth.OAuth.RateLimitTier, ok = oauthDocument["rateLimitTier"].(string); !ok {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthEntitlement, ErrInvalidClaudeNativeCredential)
	}
	if !validClaudeNativeSecret(auth.OAuth.AccessToken) || !validClaudeNativeSecret(auth.OAuth.RefreshToken) {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthValueShape, ErrInvalidClaudeNativeCredential)
	}
	if auth.OAuth.ExpiresAt < 946684800000 || auth.OAuth.ExpiresAt > 7258118400000 {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthExpiry, ErrInvalidClaudeNativeCredential)
	}
	normalizedScopes, err := authbroker.NormalizeOAuthScopes(auth.OAuth.Scopes)
	if err != nil {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthScopeSet, ErrInvalidClaudeNativeCredential)
	}
	auth.OAuth.Scopes = normalizedScopes
	if !validClaudeNativeEntitlement(auth.OAuth.SubscriptionType) ||
		!validClaudeNativeEntitlement(auth.OAuth.RateLimitTier) {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, NewClaudeCredentialCaptureError(ClaudeCaptureOAuthEntitlement, ErrInvalidClaudeNativeCredential)
	}
	return cloneClaudeNativeAuth(auth), nil
}

func validateClaudeNativeCredentialPayload(payload claudeNativeCredentialPayload) error {
	if payload.SchemaVersion != 1 || payload.Executable.Path != "/usr/local/bin/claude" ||
		payload.Executable.Version != claudeNativeVersion ||
		!claudeImageIDPattern.MatchString(payload.Executable.ImageID) ||
		!digestPattern.MatchString(payload.Executable.SHA256) {
		return ErrInvalidClaudeNativeCredential
	}
	return validateClaudeNativeOAuth(payload.Session)
}

func validateClaudeNativeOAuth(oauth claudeNativeOAuthSession) error {
	if !validClaudeNativeSecret(oauth.AccessToken) || !validClaudeNativeSecret(oauth.RefreshToken) ||
		oauth.ExpiresAt < 946684800000 || oauth.ExpiresAt > 7258118400000 ||
		!canonicalClaudeNativeScopes(oauth.Scopes) ||
		!validClaudeNativeEntitlement(oauth.SubscriptionType) ||
		!validClaudeNativeEntitlement(oauth.RateLimitTier) {
		return ErrInvalidClaudeNativeCredential
	}
	return nil
}

func validClaudeNativeEntitlement(value string) bool {
	return authbroker.ValidateSecretFreeText("Claude native entitlement", value, 128) == nil
}

func canonicalClaudeNativeScopes(scopes []string) bool {
	normalized, err := authbroker.NormalizeOAuthScopes(scopes)
	return err == nil && slices.Equal(scopes, normalized)
}

func claudeNativeJSONInt64(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	return parsed, err == nil
}

func validClaudeNativeSecret(value string) bool {
	if len(value) < 8 || len(value) > maxClaudeNativeToken {
		return false
	}
	for _, character := range value {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}

func clearClaudeNativeAuth(auth *claudeNativeAuthFile) {
	if auth == nil {
		return
	}
	auth.OAuth.AccessToken = ""
	auth.OAuth.RefreshToken = ""
}

func cloneClaudeNativePayload(payload claudeNativeCredentialPayload) claudeNativeCredentialPayload {
	clone := payload
	clone.Session = cloneClaudeNativeOAuth(payload.Session)
	return clone
}

func cloneClaudeNativeAuth(auth claudeNativeAuthFile) claudeNativeAuthFile {
	clone := auth
	clone.OAuth = cloneClaudeNativeOAuth(auth.OAuth)
	return clone
}

func cloneClaudeNativeOAuth(oauth claudeNativeOAuthSession) claudeNativeOAuthSession {
	clone := oauth
	clone.Scopes = append([]string(nil), oauth.Scopes...)
	return clone
}
