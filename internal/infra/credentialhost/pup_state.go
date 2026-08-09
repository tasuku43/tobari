package credentialhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	PupDriverID           = "datadog_pup_oauth"
	PupSite               = "datadoghq.com"
	PupAccountLabel       = "datadog-us1"
	pupStateSchemaVersion = 1
	pupClientName         = "datadog-pup-cli"
	pupRefreshWindow      = 5 * 60
	maxPupTokenBytes      = 16 << 10
)

var (
	pupClientIDPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{8,512}$`)
	pupScopePattern    = regexp.MustCompile(`^[A-Za-z0-9:_-]+(?: [A-Za-z0-9:_-]+)*$`)
)

// PupState is the canonical encrypted-at-rest OAuth state captured from one
// isolated pup login. The bearer and refresh token remain private.
type PupState struct {
	payload pupStatePayload
}

type pupStatePayload struct {
	SchemaVersion int                  `json:"schema_version"`
	Site          string               `json:"site"`
	Executable    stateExecutable      `json:"pup_executable"`
	Client        pupClientCredentials `json:"client"`
	Token         pupTokenSet          `json:"token"`
}

type pupClientCredentials struct {
	ClientID     string   `json:"client_id"`
	ClientName   string   `json:"client_name"`
	RedirectURIs []string `json:"redirect_uris"`
	RegisteredAt int64    `json:"registered_at"`
	Site         string   `json:"site"`
}

type pupTokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	IssuedAt     int64  `json:"issued_at"`
	Scope        string `json:"scope"`
	ClientID     string `json:"client_id"`
}

func (s PupState) Encode() ([]byte, error) {
	if err := validatePupStatePayload(s.payload); err != nil {
		return nil, ErrInvalidPupState
	}
	encoded, err := json.Marshal(s.payload)
	if err != nil || len(encoded) == 0 || len(encoded) > maxEncodedStateBytes {
		return nil, ErrInvalidPupState
	}
	return encoded, nil
}

func DecodePupState(encoded []byte) (PupState, error) {
	if len(encoded) == 0 || len(encoded) > maxEncodedStateBytes {
		return PupState{}, ErrInvalidPupState
	}
	var payload pupStatePayload
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return PupState{}, ErrInvalidPupState
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return PupState{}, ErrInvalidPupState
	}
	if err := validatePupStatePayload(payload); err != nil {
		return PupState{}, ErrInvalidPupState
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return PupState{}, ErrInvalidPupState
	}
	return PupState{payload: payload}, nil
}

func (s PupState) Site() string           { return s.payload.Site }
func (s PupState) DriverID() string       { return PupDriverID }
func (s PupState) DriverRevision() string { return s.payload.Executable.SHA256 }
func (PupState) String() string           { return "credentialhost.PupState{redacted}" }
func (PupState) GoString() string         { return "credentialhost.PupState{redacted}" }

func (s *PupState) Clear() {
	if s == nil {
		return
	}
	s.payload.Token.AccessToken = ""
	s.payload.Token.RefreshToken = ""
	s.payload = pupStatePayload{}
}

func newPupState(executablePath, executableDigest string, client pupClientCredentials, token pupTokenSet) (PupState, error) {
	state := PupState{payload: pupStatePayload{
		SchemaVersion: pupStateSchemaVersion,
		Site:          PupSite,
		Executable:    stateExecutable{Path: executablePath, SHA256: executableDigest},
		Client:        client,
		Token:         token,
	}}
	if err := validatePupStatePayload(state.payload); err != nil {
		state.Clear()
		return PupState{}, err
	}
	return state, nil
}

func validatePupStatePayload(payload pupStatePayload) error {
	if payload.SchemaVersion != pupStateSchemaVersion || payload.Site != PupSite ||
		payload.Client.Site != PupSite || payload.Client.ClientName != pupClientName ||
		!filepath.IsAbs(payload.Executable.Path) || filepath.Clean(payload.Executable.Path) != payload.Executable.Path ||
		!digestPattern.MatchString(payload.Executable.SHA256) ||
		!pupClientIDPattern.MatchString(payload.Client.ClientID) || payload.Client.RegisteredAt <= 0 ||
		len(payload.Client.RedirectURIs) != 1 || !validPupRedirectURI(payload.Client.RedirectURIs[0]) {
		return ErrInvalidPupState
	}
	token := payload.Token
	if token.ClientID != payload.Client.ClientID || token.TokenType != "Bearer" ||
		token.ExpiresIn <= pupRefreshWindow || token.ExpiresIn > 24*60*60 || token.IssuedAt <= 0 ||
		!validPupSecret(token.AccessToken) || !validPupSecret(token.RefreshToken) ||
		len(token.Scope) == 0 || len(token.Scope) > 16*1024 || !pupScopePattern.MatchString(token.Scope) {
		return ErrInvalidPupState
	}
	return nil
}

func validPupSecret(value string) bool {
	if len(value) < 8 || len(value) > maxPupTokenBytes {
		return false
	}
	for _, current := range value {
		if current < 0x21 || current > 0x7e {
			return false
		}
	}
	return true
}

func validPupRedirectURI(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" ||
		parsed.Path != "/oauth/callback" || parsed.RawPath != "" || parsed.RawQuery != "" ||
		parsed.Fragment != "" || parsed.User != nil || parsed.String() != value {
		return false
	}
	switch parsed.Port() {
	case "8000", "8080", "8888", "9000":
		return strings.Count(parsed.Host, ":") == 1
	default:
		return false
	}
}
