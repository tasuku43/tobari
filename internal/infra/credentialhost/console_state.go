package credentialhost

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const consoleStateSchemaVersion = 2

var (
	consoleCacheNamePattern = regexp.MustCompile(`^[0-9a-f]{64}\.json$`)
	loginSessionPattern     = regexp.MustCompile(`^arn:aws:(?:iam|sts)::([0-9]{12}):[A-Za-z0-9+=,.@_/-]{1,512}$`)
)

// ConsoleProfileConfig is the sole non-secret input Tobari supplies to AWS
// console-based login. AWS selects and returns the authenticated session.
type ConsoleProfileConfig struct {
	Region string
}

type consoleStatePayload struct {
	SchemaVersion int                 `json:"schema_version"`
	Profile       consoleStateProfile `json:"profile"`
	Executable    stateExecutable     `json:"aws_executable"`
	Cache         []stateCacheFile    `json:"login_cache"`
}

type consoleStateProfile struct {
	Name         string `json:"name"`
	Region       string `json:"region"`
	Output       string `json:"output"`
	LoginSession string `json:"login_session"`
	AccountID    string `json:"account_id"`
}

func validateConsoleProfile(profile ConsoleProfileConfig) error {
	if !commercialRegionPattern.MatchString(profile.Region) {
		return ErrInvalidProfile
	}
	return nil
}

func renderConsoleProfile(profile ConsoleProfileConfig, loginSession string) ([]byte, error) {
	if err := validateConsoleProfile(profile); err != nil {
		return nil, err
	}
	configuration := "[profile " + fixedProfileName + "]\n" +
		"region = " + profile.Region + "\n" +
		"output = " + fixedOutputFormat + "\n"
	if loginSession != "" {
		accountID, err := accountIDFromLoginSession(loginSession)
		if err != nil || accountID == "" {
			return nil, ErrInvalidState
		}
		configuration += "login_session = " + loginSession + "\n"
	}
	return []byte(configuration), nil
}

func parseConsoleProfile(configuration []byte, expected ConsoleProfileConfig) (string, string, error) {
	if len(configuration) == 0 || len(configuration) > 8*1024 || validateConsoleProfile(expected) != nil ||
		bytes.IndexByte(configuration, 0) >= 0 {
		return "", "", ErrInvalidState
	}
	values := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(configuration))
	scanner.Buffer(make([]byte, 1024), 8*1024)
	sectionSeen := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "[profile "+fixedProfileName+"]" && !sectionSeen {
			sectionSeen = true
			continue
		}
		if !sectionSeen || strings.HasPrefix(line, "[") {
			return "", "", ErrInvalidState
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return "", "", ErrInvalidState
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if value == "" || values[key] != "" || (key != "region" && key != "output" && key != "login_session") {
			return "", "", ErrInvalidState
		}
		values[key] = value
	}
	if scanner.Err() != nil || !sectionSeen || len(values) != 3 ||
		values["region"] != expected.Region || values["output"] != fixedOutputFormat {
		return "", "", ErrInvalidState
	}
	accountID, err := accountIDFromLoginSession(values["login_session"])
	if err != nil {
		return "", "", err
	}
	return values["login_session"], accountID, nil
}

func accountIDFromLoginSession(session string) (string, error) {
	matches := loginSessionPattern.FindStringSubmatch(session)
	if len(matches) != 2 || !accountIDPattern.MatchString(matches[1]) {
		return "", ErrInvalidState
	}
	return matches[1], nil
}

func newConsoleState(
	profile ConsoleProfileConfig,
	loginSession, accountID, executablePath, executableDigest string,
	cache []stateCacheFile,
) (State, error) {
	payload := consoleStatePayload{
		SchemaVersion: consoleStateSchemaVersion,
		Profile: consoleStateProfile{
			Name: fixedProfileName, Region: profile.Region, Output: fixedOutputFormat,
			LoginSession: loginSession, AccountID: accountID,
		},
		Executable: stateExecutable{Path: executablePath, SHA256: executableDigest},
		Cache:      append([]stateCacheFile(nil), cache...),
	}
	if err := validateConsoleStatePayload(payload); err != nil {
		return State{}, err
	}
	return State{console: payload}, nil
}

func validateConsoleStatePayload(payload consoleStatePayload) error {
	if payload.SchemaVersion != consoleStateSchemaVersion ||
		payload.Profile.Name != fixedProfileName || payload.Profile.Output != fixedOutputFormat ||
		validateConsoleProfile(ConsoleProfileConfig{Region: payload.Profile.Region}) != nil {
		return ErrInvalidState
	}
	accountID, err := accountIDFromLoginSession(payload.Profile.LoginSession)
	if err != nil || payload.Profile.AccountID != accountID {
		return ErrInvalidState
	}
	if !filepath.IsAbs(payload.Executable.Path) || filepath.Clean(payload.Executable.Path) != payload.Executable.Path ||
		!digestPattern.MatchString(payload.Executable.SHA256) {
		return ErrInvalidState
	}
	return validateCacheFiles(payload.Cache, consoleCacheNamePattern)
}

func encodeConsoleState(payload consoleStatePayload) ([]byte, error) {
	payload = cloneConsolePayload(payload)
	if err := validateConsoleStatePayload(payload); err != nil {
		return nil, ErrInvalidState
	}
	sort.Slice(payload.Cache, func(i, j int) bool { return payload.Cache[i].Name < payload.Cache[j].Name })
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > maxEncodedStateBytes {
		return nil, ErrInvalidState
	}
	return encoded, nil
}

func decodeConsoleState(encoded []byte) (State, error) {
	var payload consoleStatePayload
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return State{}, ErrInvalidState
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return State{}, ErrInvalidState
	}
	if err := validateConsoleStatePayload(payload); err != nil {
		return State{}, ErrInvalidState
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(encoded, canonical) {
		return State{}, ErrInvalidState
	}
	return State{console: cloneConsolePayload(payload)}, nil
}

func validateCacheFiles(cache []stateCacheFile, namePattern *regexp.Regexp) error {
	if len(cache) == 0 || len(cache) > maxCacheFiles || namePattern == nil {
		return ErrInvalidState
	}
	total := 0
	previous := ""
	for _, item := range cache {
		if !namePattern.MatchString(item.Name) || item.Name <= previous ||
			filepath.Base(item.Name) != item.Name || strings.Contains(item.Name, "..") {
			return ErrInvalidState
		}
		previous = item.Name
		content, err := base64.RawURLEncoding.DecodeString(item.ContentBase64URL)
		if err != nil || len(content) == 0 || len(content) > maxCacheFileBytes || !validJSONObject(content) {
			return ErrInvalidState
		}
		total += len(content)
		if total > maxCacheTotalBytes || base64.RawURLEncoding.EncodeToString(content) != item.ContentBase64URL {
			return ErrInvalidState
		}
	}
	return nil
}

func cloneConsolePayload(payload consoleStatePayload) consoleStatePayload {
	cloned := payload
	cloned.Cache = append([]stateCacheFile(nil), payload.Cache...)
	return cloned
}
