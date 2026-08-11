package credentialhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SSODriverID            = "aws_cli_sso"
	ConsoleDriverID        = "aws_cli_console_login"
	stateSchemaVersion     = 1
	fixedProfileName       = "tobari"
	fixedSSOSessionName    = "tobari"
	fixedOutputFormat      = "json"
	fixedRegistrationScope = "sso:account:access"

	// The complete canonical driver state must fit the Broker's existing
	// bounded secret ingress and one encrypted companion frame. Cache content
	// is base64url-expanded inside that state, so its raw aggregate is tighter.
	maxEncodedStateBytes = 32 << 10
	maxCacheFiles        = 8
	maxCacheFileBytes    = 20 << 10
	maxCacheTotalBytes   = 20 << 10
	maxExecutableBytes   = 512 << 20
)

var (
	ErrInvalidProfile    = errors.New("AWS host profile is invalid")
	ErrInvalidExecutable = errors.New("AWS executable is invalid")
	ErrInvalidState      = errors.New("AWS host state is invalid")
	ErrInvalidCache      = errors.New("AWS SSO cache is invalid")

	startURLPattern         = regexp.MustCompile(`^https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.awsapps\.com/start$`)
	commercialRegionPattern = regexp.MustCompile(`^(?:us-(?:east|west)|eu-(?:central|north|south|west)|ap-(?:east|northeast|south|southeast)|ca-(?:central|west)|sa-east|me-(?:central|south)|af-south|il-central|mx-central|nz-north)-[0-9]+$`)
	accountIDPattern        = regexp.MustCompile(`^[0-9]{12}$`)
	roleNamePattern         = regexp.MustCompile(`^[A-Za-z0-9_+=,.@-]{1,64}$`)
	cacheNamePattern        = regexp.MustCompile(`^[0-9a-f]{40}\.json$`)
	digestPattern           = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// ProfileConfig contains the caller-collected, non-secret values used to
// render the one supported AWS CLI profile. Profile and SSO session names,
// output format, and registration scope are fixed by this package.
type ProfileConfig struct {
	StartURL  string
	SSORegion string
	AccountID string
	RoleName  string
}

// State is the opaque, schema-versioned AWS CLI host state. Its fields remain
// private so formatting cannot expose cached SSO material.
type State struct {
	payload statePayload
	console consoleStatePayload
}

type statePayload struct {
	SchemaVersion int              `json:"schema_version"`
	Driver        string           `json:"driver"`
	Profile       stateProfile     `json:"profile"`
	Executable    stateExecutable  `json:"aws_executable"`
	Cache         []stateCacheFile `json:"sso_cache"`
}

type stateProfile struct {
	Name               string `json:"name"`
	SSOSession         string `json:"sso_session"`
	StartURL           string `json:"sso_start_url"`
	SSORegion          string `json:"sso_region"`
	AccountID          string `json:"sso_account_id"`
	RoleName           string `json:"sso_role_name"`
	Output             string `json:"output"`
	RegistrationScopes string `json:"sso_registration_scopes"`
}

type stateExecutable struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type stateCacheFile struct {
	Name             string `json:"name"`
	ContentBase64URL string `json:"content_base64url"`
}

// Encode returns the only accepted canonical JSON representation of State.
func (s State) Encode() ([]byte, error) {
	if s.console.SchemaVersion != 0 {
		return encodeConsoleState(s.console)
	}
	payload := clonePayload(s.payload)
	if err := validateStatePayload(payload); err != nil {
		return nil, ErrInvalidState
	}
	sort.Slice(payload.Cache, func(i, j int) bool { return payload.Cache[i].Name < payload.Cache[j].Name })
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxEncodedStateBytes {
		return nil, ErrInvalidState
	}
	return encoded, nil
}

// DecodeState accepts only canonical schema-v1 state produced by Encode.
func DecodeState(encoded []byte) (State, error) {
	if len(encoded) == 0 || len(encoded) > maxEncodedStateBytes {
		return State{}, ErrInvalidState
	}
	var version struct {
		SchemaVersion int    `json:"schema_version"`
		Driver        string `json:"driver"`
	}
	if err := json.Unmarshal(encoded, &version); err != nil {
		return State{}, ErrInvalidState
	}
	if version.SchemaVersion != 1 {
		return State{}, ErrInvalidState
	}
	if version.Driver == ConsoleDriverID {
		return decodeConsoleState(encoded)
	}
	if version.Driver != SSODriverID {
		return State{}, ErrInvalidState
	}
	var payload statePayload
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return State{}, ErrInvalidState
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return State{}, ErrInvalidState
	}
	if err := validateStatePayload(payload); err != nil {
		return State{}, ErrInvalidState
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return State{}, ErrInvalidState
	}
	return State{payload: clonePayload(payload)}, nil
}

// Profile returns a copy of the non-secret profile inputs.
func (s State) Profile() ProfileConfig {
	return ProfileConfig{
		StartURL:  s.payload.Profile.StartURL,
		SSORegion: s.payload.Profile.SSORegion,
		AccountID: s.payload.Profile.AccountID,
		RoleName:  s.payload.Profile.RoleName,
	}
}

// DriverID returns the fixed reviewed driver variant encoded by this state.
func (s State) DriverID() string {
	if s.console.SchemaVersion == consoleStateSchemaVersion {
		return ConsoleDriverID
	}
	if s.payload.SchemaVersion == stateSchemaVersion {
		return SSODriverID
	}
	return ""
}

// AccountID returns the validated 12-digit AWS account identity without
// exposing the provider session ARN or cache content.
func (s State) AccountID() string {
	if s.console.SchemaVersion == consoleStateSchemaVersion {
		return s.console.Profile.AccountID
	}
	return s.payload.Profile.AccountID
}

// DriverRevision returns the non-secret SHA-256 identity of the pinned AWS
// executable. Broker records bind refresh requests to this exact revision.
func (s State) DriverRevision() string {
	if s.console.SchemaVersion == consoleStateSchemaVersion {
		return s.console.Executable.SHA256
	}
	return s.payload.Executable.SHA256
}

func (State) String() string   { return "credentialhost.State{redacted}" }
func (State) GoString() string { return "credentialhost.State{redacted}" }

// Clear releases references to opaque SSO cache material after the caller has
// encoded or replaced the state.
func (s *State) Clear() {
	if s == nil {
		return
	}
	for index := range s.payload.Cache {
		s.payload.Cache[index].ContentBase64URL = ""
	}
	for index := range s.console.Cache {
		s.console.Cache[index].ContentBase64URL = ""
	}
	s.payload = statePayload{}
	s.console = consoleStatePayload{}
}

func newState(profile ProfileConfig, executablePath, executableDigest string, cache []stateCacheFile) (State, error) {
	payload := statePayload{
		SchemaVersion: stateSchemaVersion,
		Driver:        SSODriverID,
		Profile: stateProfile{
			Name:               fixedProfileName,
			SSOSession:         fixedSSOSessionName,
			StartURL:           profile.StartURL,
			SSORegion:          profile.SSORegion,
			AccountID:          profile.AccountID,
			RoleName:           profile.RoleName,
			Output:             fixedOutputFormat,
			RegistrationScopes: fixedRegistrationScope,
		},
		Executable: stateExecutable{Path: executablePath, SHA256: executableDigest},
		Cache:      append([]stateCacheFile(nil), cache...),
	}
	if err := validateStatePayload(payload); err != nil {
		return State{}, err
	}
	return State{payload: payload}, nil
}

func validateStatePayload(payload statePayload) error {
	if payload.SchemaVersion != stateSchemaVersion || payload.Driver != SSODriverID ||
		payload.Profile.Name != fixedProfileName ||
		payload.Profile.SSOSession != fixedSSOSessionName ||
		payload.Profile.Output != fixedOutputFormat ||
		payload.Profile.RegistrationScopes != fixedRegistrationScope {
		return ErrInvalidState
	}
	profile := ProfileConfig{
		StartURL:  payload.Profile.StartURL,
		SSORegion: payload.Profile.SSORegion,
		AccountID: payload.Profile.AccountID,
		RoleName:  payload.Profile.RoleName,
	}
	if err := validateProfile(profile); err != nil {
		return ErrInvalidState
	}
	if !filepath.IsAbs(payload.Executable.Path) || filepath.Clean(payload.Executable.Path) != payload.Executable.Path ||
		!digestPattern.MatchString(payload.Executable.SHA256) {
		return ErrInvalidState
	}
	if len(payload.Cache) == 0 || len(payload.Cache) > maxCacheFiles {
		return ErrInvalidState
	}
	total := 0
	previous := ""
	for _, item := range payload.Cache {
		if !cacheNamePattern.MatchString(item.Name) || item.Name <= previous ||
			filepath.Base(item.Name) != item.Name || strings.Contains(item.Name, "..") {
			return ErrInvalidState
		}
		previous = item.Name
		content, err := base64.RawURLEncoding.DecodeString(item.ContentBase64URL)
		if err != nil || len(content) == 0 || len(content) > maxCacheFileBytes || !validJSONObject(content) {
			return ErrInvalidState
		}
		total += len(content)
		if total > maxCacheTotalBytes {
			return ErrInvalidState
		}
		if base64.RawURLEncoding.EncodeToString(content) != item.ContentBase64URL {
			return ErrInvalidState
		}
	}
	return nil
}

func validateProfile(profile ProfileConfig) error {
	parsed, err := url.Parse(profile.StartURL)
	if err != nil || !startURLPattern.MatchString(profile.StartURL) ||
		parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.Port() != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.RawPath != "" || parsed.String() != profile.StartURL ||
		!commercialRegionPattern.MatchString(profile.SSORegion) ||
		!accountIDPattern.MatchString(profile.AccountID) || !roleNamePattern.MatchString(profile.RoleName) {
		return ErrInvalidProfile
	}
	return nil
}

func renderProfile(profile ProfileConfig) ([]byte, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	configuration := "[profile " + fixedProfileName + "]\n" +
		"sso_session = " + fixedSSOSessionName + "\n" +
		"sso_account_id = " + profile.AccountID + "\n" +
		"sso_role_name = " + profile.RoleName + "\n" +
		"output = " + fixedOutputFormat + "\n\n" +
		"[sso-session " + fixedSSOSessionName + "]\n" +
		"sso_start_url = " + profile.StartURL + "\n" +
		"sso_region = " + profile.SSORegion + "\n" +
		"sso_registration_scopes = " + fixedRegistrationScope + "\n"
	return []byte(configuration), nil
}

func resolveExecutable(path string) (string, string, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return "", "", ErrInvalidExecutable
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", "", ErrInvalidExecutable
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil || !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		return "", "", ErrInvalidExecutable
	}
	digest, err := hashExecutable(canonical)
	if err != nil {
		return "", "", err
	}
	return canonical, digest, nil
}

func verifyExecutable(path, expectedDigest string) error {
	canonical, digest, err := resolveExecutable(path)
	if err != nil || canonical != path || digest != expectedDigest {
		return ErrInvalidExecutable
	}
	return nil
}

func hashExecutable(path string) (string, error) {
	parent := filepath.Dir(path)
	name := filepath.Base(path)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || name == "." || name == string(filepath.Separator) {
		return "", ErrInvalidExecutable
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode()&os.ModeSymlink != 0 {
		return "", ErrInvalidExecutable
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return "", ErrInvalidExecutable
	}
	defer root.Close()
	openedParent, err := root.Stat(".")
	if err != nil || !openedParent.IsDir() || !os.SameFile(parentInfo, openedParent) {
		return "", ErrInvalidExecutable
	}
	info, err := root.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 ||
		info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > maxExecutableBytes {
		return "", ErrInvalidExecutable
	}
	file, err := root.Open(name)
	if err != nil {
		return "", ErrInvalidExecutable
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o111 == 0 ||
		opened.Mode().Perm()&0o022 != 0 || !os.SameFile(info, opened) {
		return "", ErrInvalidExecutable
	}
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxExecutableBytes+1))
	if err != nil || written != info.Size() || written > maxExecutableBytes {
		return "", ErrInvalidExecutable
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func packCache(cacheDirectory string) ([]stateCacheFile, error) {
	return packCacheWithPattern(cacheDirectory, cacheNamePattern)
}

func packCacheWithPattern(cacheDirectory string, namePattern *regexp.Regexp) ([]stateCacheFile, error) {
	if namePattern == nil {
		return nil, ErrInvalidCache
	}
	directoryInfo, err := os.Lstat(cacheDirectory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 ||
		directoryInfo.Mode().Perm()&0o077 != 0 {
		return nil, ErrInvalidCache
	}
	root, err := os.OpenRoot(cacheDirectory)
	if err != nil {
		return nil, ErrInvalidCache
	}
	defer root.Close()
	openedDirectory, err := root.Stat(".")
	if err != nil || !openedDirectory.IsDir() || openedDirectory.Mode().Perm()&0o077 != 0 ||
		!os.SameFile(directoryInfo, openedDirectory) {
		return nil, ErrInvalidCache
	}
	directory, err := root.Open(".")
	if err != nil {
		return nil, ErrInvalidCache
	}
	entries, readDirectoryErr := directory.ReadDir(-1)
	closeDirectoryErr := directory.Close()
	if readDirectoryErr != nil || closeDirectoryErr != nil || len(entries) == 0 || len(entries) > maxCacheFiles {
		return nil, ErrInvalidCache
	}
	packed := make([]stateCacheFile, 0, len(entries))
	total := int64(0)
	for _, entry := range entries {
		name := entry.Name()
		if !namePattern.MatchString(name) || filepath.Base(name) != name || strings.Contains(name, "..") ||
			entry.Type()&os.ModeSymlink != 0 {
			return nil, ErrInvalidCache
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 ||
			info.Size() <= 0 || info.Size() > maxCacheFileBytes {
			return nil, ErrInvalidCache
		}
		total += info.Size()
		if total > maxCacheTotalBytes {
			return nil, ErrInvalidCache
		}
		file, err := root.Open(name)
		if err != nil {
			return nil, ErrInvalidCache
		}
		opened, statErr := file.Stat()
		content, readErr := io.ReadAll(io.LimitReader(file, maxCacheFileBytes+1))
		defer clear(content)
		closeErr := file.Close()
		if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) ||
			readErr != nil || closeErr != nil || int64(len(content)) != info.Size() || !validJSONObject(content) {
			return nil, ErrInvalidCache
		}
		packed = append(packed, stateCacheFile{
			Name:             name,
			ContentBase64URL: base64.RawURLEncoding.EncodeToString(content),
		})
	}
	sort.Slice(packed, func(i, j int) bool { return packed[i].Name < packed[j].Name })
	return packed, nil
}

func validJSONObject(content []byte) bool {
	var value map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value == nil {
		return false
	}
	var trailing any
	return errors.Is(decoder.Decode(&trailing), io.EOF)
}

func clonePayload(payload statePayload) statePayload {
	cloned := payload
	cloned.Cache = append([]stateCacheFile(nil), payload.Cache...)
	return cloned
}
