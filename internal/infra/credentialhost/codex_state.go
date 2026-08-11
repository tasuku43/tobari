package credentialhost

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	codexCredentialSchemaVersion = 1
	maxCodexAuthBytes            = 24 << 10
	maxCodexTokenBytes           = 16 << 10
	maxCodexAccountIDBytes       = 128
	codexAuthMode                = "chatgpt"
)

var (
	ErrInvalidCodexCredential = errors.New("Codex OAuth credential state is invalid")
	codexAccountIDPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
)

// CodexCredential is canonical encrypted-at-rest state captured from one
// isolated Codex ChatGPT OAuth login. Formatting is always redacted; Encode
// is the sole complete disclosure path for committing the state to the Broker.
type CodexCredential struct {
	payload codexCredentialPayload
}

type codexCredentialPayload struct {
	SchemaVersion int             `json:"schema_version"`
	Executable    codexExecutable `json:"codex_executable"`
	Auth          codexAuthState  `json:"auth"`
}

type codexExecutable struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

type codexAuthState struct {
	AuthMode     string         `json:"auth_mode"`
	OpenAIAPIKey *string        `json:"OPENAI_API_KEY"`
	Tokens       codexTokenData `json:"tokens"`
	LastRefresh  string         `json:"last_refresh"`
}

type codexTokenData struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
}

func (c CodexCredential) Encode() ([]byte, error) {
	if err := validateCodexCredentialPayload(c.payload); err != nil {
		return nil, ErrInvalidCodexCredential
	}
	encoded, err := json.Marshal(c.payload)
	if err != nil || len(encoded) == 0 || len(encoded) > maxEncodedStateBytes {
		return nil, ErrInvalidCodexCredential
	}
	return encoded, nil
}

// AuthJSON returns the exact canonical auth.json shape understood by Codex
// 0.146.0. Callers must treat the returned bytes as secret and clear them.
func (c CodexCredential) AuthJSON() ([]byte, error) {
	if err := validateCodexCredentialPayload(c.payload); err != nil {
		return nil, ErrInvalidCodexCredential
	}
	encoded, err := json.Marshal(c.payload.Auth)
	if err != nil || len(encoded) == 0 || len(encoded) > maxCodexAuthBytes {
		return nil, ErrInvalidCodexCredential
	}
	return encoded, nil
}

func DecodeCodexCredential(encoded []byte) (CodexCredential, error) {
	if len(encoded) == 0 || len(encoded) > maxEncodedStateBytes || !utf8.Valid(encoded) {
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	var payload codexCredentialPayload
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	if err := validateCodexCredentialPayload(payload); err != nil {
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	return CodexCredential{payload: cloneCodexCredentialPayload(payload)}, nil
}

func (c CodexCredential) AccountLabel() string {
	return c.payload.Auth.Tokens.AccountID
}
func (c CodexCredential) DriverID() string { return CodexDriverID }
func (c CodexCredential) DriverRevision() string {
	return c.payload.Executable.SHA256
}
func (CodexCredential) String() string   { return "credentialhost.CodexCredential{redacted}" }
func (CodexCredential) GoString() string { return "credentialhost.CodexCredential{redacted}" }

func (c *CodexCredential) Clear() {
	if c == nil {
		return
	}
	c.payload.Auth.Tokens.IDToken = ""
	c.payload.Auth.Tokens.AccessToken = ""
	c.payload.Auth.Tokens.RefreshToken = ""
	c.payload.Auth.Tokens.AccountID = ""
	c.payload = codexCredentialPayload{}
}

func newCodexCredential(
	executablePath string,
	executableDigest string,
	auth codexAuthState,
	accountLabel string,
) (CodexCredential, error) {
	credential := CodexCredential{payload: codexCredentialPayload{
		SchemaVersion: codexCredentialSchemaVersion,
		Executable: codexExecutable{
			Path: executablePath, SHA256: executableDigest, Version: CodexVersion,
		},
		Auth: cloneCodexAuthState(auth),
	}}
	if credential.AccountLabel() != accountLabel {
		credential.Clear()
		return CodexCredential{}, ErrInvalidCodexCredential
	}
	if err := validateCodexCredentialPayload(credential.payload); err != nil {
		credential.Clear()
		return CodexCredential{}, err
	}
	return credential, nil
}

func parseCodexAuth(encoded []byte) (codexAuthState, string, error) {
	if len(encoded) == 0 || len(encoded) > maxCodexAuthBytes || !utf8.Valid(encoded) {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	decoded, err := decodeGitHubJSON(encoded)
	if err != nil {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	document, ok := decoded.(map[string]any)
	if !ok || !hasExactCodexKeys(document, "auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh") {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	authMode, ok := document["auth_mode"].(string)
	if !ok || authMode != codexAuthMode || document["OPENAI_API_KEY"] != nil {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	tokenObject, ok := document["tokens"].(map[string]any)
	if !ok || !hasExactCodexKeys(tokenObject, "id_token", "access_token", "refresh_token", "account_id") {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	idToken, ok := tokenObject["id_token"].(string)
	if !ok {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	accessToken, ok := tokenObject["access_token"].(string)
	if !ok {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	refreshToken, ok := tokenObject["refresh_token"].(string)
	if !ok {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	accountID, ok := tokenObject["account_id"].(string)
	if !ok || !validCodexAccountID(accountID) {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	lastRefresh, ok := document["last_refresh"].(string)
	if !ok || !strings.HasSuffix(lastRefresh, "Z") {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	parsedRefresh, err := time.Parse(time.RFC3339Nano, lastRefresh)
	if err != nil {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	lastRefresh = parsedRefresh.UTC().Format(time.RFC3339Nano)
	claimAccountID, err := codexJWTAccountID(idToken)
	if err != nil || !validCodexSecret(idToken) || !validCodexSecret(accessToken) || !validCodexSecret(refreshToken) {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	if claimAccountID == nil || accountID != *claimAccountID {
		return codexAuthState{}, "", ErrCodexAuthCapture
	}
	accountLabel := accountID
	auth := codexAuthState{
		AuthMode:     codexAuthMode,
		OpenAIAPIKey: nil,
		Tokens:       codexTokenData{idToken, accessToken, refreshToken, accountID},
		LastRefresh:  lastRefresh,
	}
	return auth, accountLabel, nil
}

func validateCodexCredentialPayload(payload codexCredentialPayload) error {
	if payload.SchemaVersion != codexCredentialSchemaVersion || payload.Executable.Version != CodexVersion ||
		!filepath.IsAbs(payload.Executable.Path) || filepath.Clean(payload.Executable.Path) != payload.Executable.Path ||
		!digestPattern.MatchString(payload.Executable.SHA256) || payload.Auth.AuthMode != codexAuthMode ||
		payload.Auth.OpenAIAPIKey != nil {
		return ErrInvalidCodexCredential
	}
	parsedRefresh, err := time.Parse(time.RFC3339Nano, payload.Auth.LastRefresh)
	if err != nil || !strings.HasSuffix(payload.Auth.LastRefresh, "Z") ||
		parsedRefresh.UTC().Format(time.RFC3339Nano) != payload.Auth.LastRefresh {
		return ErrInvalidCodexCredential
	}
	tokens := payload.Auth.Tokens
	claimAccountID, err := codexJWTAccountID(tokens.IDToken)
	if err != nil || !validCodexSecret(tokens.IDToken) || !validCodexSecret(tokens.AccessToken) ||
		!validCodexSecret(tokens.RefreshToken) {
		return ErrInvalidCodexCredential
	}
	if !validCodexAccountID(tokens.AccountID) {
		return ErrInvalidCodexCredential
	}
	if claimAccountID == nil || tokens.AccountID != *claimAccountID {
		return ErrInvalidCodexCredential
	}
	return nil
}

func codexJWTAccountID(token string) (*string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return nil, ErrCodexAuthCapture
	}
	decodedParts := make([][]byte, len(parts))
	defer func() {
		for _, part := range decodedParts {
			clear(part)
		}
	}()
	for index, part := range parts {
		if len(part) > maxCodexTokenBytes {
			return nil, ErrCodexAuthCapture
		}
		decoded, err := base64.RawURLEncoding.DecodeString(part)
		if err != nil || len(decoded) == 0 {
			return nil, ErrCodexAuthCapture
		}
		decodedParts[index] = decoded
	}
	if len(decodedParts[0]) > maxCodexTokenBytes || !utf8.Valid(decodedParts[0]) {
		return nil, ErrCodexAuthCapture
	}
	header, err := decodeGitHubJSON(decodedParts[0])
	if err != nil {
		return nil, ErrCodexAuthCapture
	}
	if _, ok := header.(map[string]any); !ok {
		return nil, ErrCodexAuthCapture
	}
	payloadBytes := decodedParts[1]
	if len(payloadBytes) > maxCodexTokenBytes || !utf8.Valid(payloadBytes) {
		return nil, ErrCodexAuthCapture
	}
	value, err := decodeGitHubJSON(payloadBytes)
	if err != nil {
		return nil, ErrCodexAuthCapture
	}
	payload, ok := value.(map[string]any)
	if !ok {
		return nil, ErrCodexAuthCapture
	}
	claimSet, present := payload["https://api.openai.com/auth"]
	if !present {
		return nil, nil
	}
	claims, ok := claimSet.(map[string]any)
	if !ok {
		return nil, ErrCodexAuthCapture
	}
	if fedramp, present := claims["chatgpt_account_is_fedramp"]; present {
		value, ok := fedramp.(bool)
		if !ok || value {
			return nil, ErrCodexAuthCapture
		}
	}
	claim, present := claims["chatgpt_account_id"]
	if !present || claim == nil {
		return nil, nil
	}
	accountID, ok := claim.(string)
	if !ok || !validCodexAccountID(accountID) {
		return nil, ErrCodexAuthCapture
	}
	return &accountID, nil
}

func validCodexSecret(value string) bool {
	if len(value) < 8 || len(value) > maxCodexTokenBytes {
		return false
	}
	for _, current := range value {
		if current < 0x21 || current > 0x7e {
			return false
		}
	}
	return true
}

func validCodexAccountID(value string) bool {
	return len(value) <= maxCodexAccountIDBytes && codexAccountIDPattern.MatchString(value)
}

func hasExactCodexKeys(document map[string]any, expected ...string) bool {
	if len(document) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, present := document[key]; !present {
			return false
		}
	}
	return true
}

func cloneCodexCredentialPayload(payload codexCredentialPayload) codexCredentialPayload {
	clone := payload
	clone.Auth = cloneCodexAuthState(payload.Auth)
	return clone
}

func cloneCodexAuthState(auth codexAuthState) codexAuthState {
	clone := auth
	clone.OpenAIAPIKey = cloneStringPointer(auth.OpenAIAPIKey)
	return clone
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
