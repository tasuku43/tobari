package credentialhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	ClaudeNativeDriverID     = "anthropic_claude_native_oauth"
	ClaudeNativeAccountLabel = "claude-user-native"
	claudeNativeVersion      = "2.1.220"
	claudeNativeClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
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
	claudeNativeScopes               = []string{
		"org:create_api_key",
		"user:profile",
		"user:inference",
		"user:sessions:claude_code",
		"user:mcp_servers",
		"user:file_upload",
	}
)

// ClaudeNativeCredential is the canonical encrypted-at-rest form of the
// native Linux credential written by one exact Context-image Claude client.
// It is always redacted; Encode is the only complete disclosure path.
type ClaudeNativeCredential struct {
	payload claudeNativeCredentialPayload
}

type claudeNativeCredentialPayload struct {
	SchemaVersion int                    `json:"schema_version"`
	Executable    claudeNativeExecutable `json:"claude_executable"`
	Auth          claudeNativeAuthFile   `json:"auth"`
}

type claudeNativeExecutable struct {
	ImageID string `json:"image_id"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

type claudeNativeAuthFile struct {
	OAuth claudeNativeOAuth `json:"claudeAiOauth"`
}

type claudeNativeOAuth struct {
	AccessToken           string   `json:"accessToken"`
	RefreshToken          string   `json:"refreshToken"`
	ExpiresAt             int64    `json:"expiresAt"`
	RefreshTokenExpiresAt *int64   `json:"refreshTokenExpiresAt,omitempty"`
	Scopes                []string `json:"scopes"`
	SubscriptionType      *string  `json:"subscriptionType"`
	RateLimitTier         *string  `json:"rateLimitTier"`
	ClientID              string   `json:"clientId"`
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
		Auth: auth,
	}}
	if err := validateClaudeNativeCredentialPayload(credential.payload); err != nil {
		credential.Clear()
		return ClaudeNativeCredential{}, err
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
	c.payload.Auth.OAuth.AccessToken = ""
	c.payload.Auth.OAuth.RefreshToken = ""
	c.payload = claudeNativeCredentialPayload{}
}

func parseClaudeNativeAuth(encoded []byte) (claudeNativeAuthFile, error) {
	if len(encoded) == 0 || len(encoded) > maxClaudeNativeState || !utf8.Valid(encoded) {
		return claudeNativeAuthFile{}, ErrInvalidClaudeNativeCredential
	}
	if _, err := decodeGitHubJSON(encoded); err != nil {
		return claudeNativeAuthFile{}, ErrInvalidClaudeNativeCredential
	}
	var auth claudeNativeAuthFile
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&auth); err != nil {
		return claudeNativeAuthFile{}, ErrInvalidClaudeNativeCredential
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return claudeNativeAuthFile{}, ErrInvalidClaudeNativeCredential
	}
	if err := validateClaudeNativeOAuth(auth.OAuth); err != nil {
		clearClaudeNativeAuth(&auth)
		return claudeNativeAuthFile{}, err
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
	return validateClaudeNativeOAuth(payload.Auth.OAuth)
}

func validateClaudeNativeOAuth(oauth claudeNativeOAuth) error {
	if !validClaudeNativeSecret(oauth.AccessToken) || !validClaudeNativeSecret(oauth.RefreshToken) ||
		oauth.ExpiresAt < 946684800000 || oauth.ExpiresAt > 7258118400000 ||
		(oauth.RefreshTokenExpiresAt != nil && (*oauth.RefreshTokenExpiresAt < oauth.ExpiresAt || *oauth.RefreshTokenExpiresAt > 7258118400000)) ||
		!slices.Equal(oauth.Scopes, claudeNativeScopes) || oauth.ClientID != claudeNativeClientID ||
		!validClaudeNullableEnum(oauth.SubscriptionType, "pro", "max", "team", "enterprise") ||
		!validClaudeNullableText(oauth.RateLimitTier) {
		return ErrInvalidClaudeNativeCredential
	}
	return nil
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

func validClaudeNullableEnum(value *string, allowed ...string) bool {
	return value == nil || slices.Contains(allowed, *value)
}

func validClaudeNullableText(value *string) bool {
	if value == nil {
		return true
	}
	return len(*value) > 0 && len(*value) <= 128 && strings.IndexFunc(*value, func(character rune) bool {
		return character < 0x21 || character > 0x7e
	}) < 0
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
	clone.Auth = cloneClaudeNativeAuth(payload.Auth)
	return clone
}

func cloneClaudeNativeAuth(auth claudeNativeAuthFile) claudeNativeAuthFile {
	clone := auth
	clone.OAuth.Scopes = append([]string(nil), auth.OAuth.Scopes...)
	if auth.OAuth.RefreshTokenExpiresAt != nil {
		value := *auth.OAuth.RefreshTokenExpiresAt
		clone.OAuth.RefreshTokenExpiresAt = &value
	}
	clone.OAuth.SubscriptionType = cloneStringPointer(auth.OAuth.SubscriptionType)
	clone.OAuth.RateLimitTier = cloneStringPointer(auth.OAuth.RateLimitTier)
	return clone
}
