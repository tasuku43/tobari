package credentialhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

var (
	ErrPupLoginFailed  = errors.New("pup OAuth login failed")
	ErrInvalidPupState = errors.New("pup OAuth state is invalid")
)

// PupLogin runs pup's fixed OAuth PKCE flow in a private file-backed config
// directory, then imports only the strict default US1 session into opaque state.
func (d *Driver) PupLogin(
	ctx context.Context,
	pupExecutable string,
	input io.Reader,
	visible VisibleOutput,
) (state PupState, resultErr error) {
	if ctx == nil || input == nil {
		return PupState{}, ErrPupLoginFailed
	}
	canonical, digest, err := resolveExecutable(pupExecutable)
	if err != nil {
		return PupState{}, err
	}
	home, configDir, err := d.preparePupHome()
	if err != nil {
		return PupState{}, ErrPupLoginFailed
	}
	defer func() {
		if err := d.cleanupHome(home); err != nil {
			state.Clear()
			resultErr = ErrPupLoginFailed
		}
	}()

	visibleOutput := newVisibleLimiter(maxVisibleOutputBytes, visible)
	command := Command{
		Path:   canonical,
		Args:   []string{"--no-agent", "auth", "login", "--site", PupSite},
		Env:    pupCommandEnvironment(home, configDir),
		Dir:    home,
		Stdin:  input,
		Stdout: visibleOutput.writer(OutputStdout),
		Stderr: visibleOutput.writer(OutputStderr),
	}
	timeout := d.pupTimeout
	if timeout <= 0 {
		timeout = defaultPupLoginTimeout
	}
	loginContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runErr := d.runner.Run(loginContext, command)
	if loginContext.Err() != nil {
		return PupState{}, loginContext.Err()
	}
	if outputErr := visibleOutput.err(); outputErr != nil {
		return PupState{}, outputErr
	}
	if runErr != nil {
		return PupState{}, ErrPupLoginFailed
	}
	if err := verifyExecutable(canonical, digest); err != nil {
		return PupState{}, err
	}
	client, token, err := readPupStateFiles(configDir)
	if err != nil {
		return PupState{}, err
	}
	state, err = newPupState(canonical, digest, client, token)
	if err != nil {
		return PupState{}, ErrInvalidPupState
	}
	return state, nil
}

func (d *Driver) preparePupHome() (string, string, error) {
	home, err := os.MkdirTemp(d.tempRoot, "tobari-pup-host-*")
	if err != nil {
		return "", "", err
	}
	failed := true
	defer func() {
		if failed {
			_ = d.cleanupHome(home)
		}
	}()
	if err := os.Chmod(home, 0o700); err != nil { // #nosec G302 -- owner-only directory requires execute permission for traversal.
		return "", "", err
	}
	info, err := os.Lstat(home)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", "", ErrInvalidPupState
	}
	configDir := filepath.Join(home, ".config", "pup")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return "", "", err
	}
	if err := os.Chmod(filepath.Join(home, ".config"), 0o700); err != nil { // #nosec G302 -- owner-only directory requires execute permission for traversal.
		return "", "", err
	}
	if err := os.Chmod(configDir, 0o700); err != nil { // #nosec G302 -- owner-only directory requires execute permission for traversal.
		return "", "", err
	}
	failed = false
	return home, configDir, nil
}

func pupCommandEnvironment(home, configDir string) []string {
	return []string{
		"HOME=" + home,
		"PUP_CONFIG_DIR=" + configDir,
		"DD_TOKEN_STORAGE=file",
		"DD_SITE=" + PupSite,
		"NO_COLOR=1",
		"LC_ALL=C",
		"PATH=/usr/bin:/bin",
	}
}

func readPupStateFiles(configDir string) (pupClientCredentials, pupTokenSet, error) {
	clientBytes, err := readPrivateHomeFile(configDir, "client_datadoghq_com.json", 8*1024)
	if err != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	defer clear(clientBytes)
	tokenBytes, err := readPrivateHomeFile(configDir, "tokens_datadoghq_com.json", 32*1024)
	if err != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	defer clear(tokenBytes)
	sessionsBytes, err := readPrivateHomeFile(configDir, "sessions.json", 8*1024)
	if err != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	defer clear(sessionsBytes)

	var client pupClientCredentials
	if err := decodeExactJSON(clientBytes, &client); err != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	var tokens map[string]pupTokenSet
	if err := decodeExactJSON(tokenBytes, &tokens); err != nil || len(tokens) != 1 {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	token, ok := tokens["__default__"]
	if !ok {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	var sessions []struct {
		Site    string  `json:"site"`
		Org     *string `json:"org"`
		OrgUUID *string `json:"org_uuid,omitempty"`
	}
	if err := decodeExactJSON(sessionsBytes, &sessions); err != nil || len(sessions) != 1 ||
		sessions[0].Site != PupSite || sessions[0].Org != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	if sessions[0].OrgUUID != nil && !validUUID(*sessions[0].OrgUUID) {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	if err := validatePupStatePayload(pupStatePayload{
		SchemaVersion: pupStateSchemaVersion, Site: PupSite,
		Executable: stateExecutable{Path: "/validated/later", SHA256: stringsRepeat("0", 64)},
		Client:     client, Token: token,
	}); err != nil {
		return pupClientCredentials{}, pupTokenSet{}, ErrInvalidPupState
	}
	return client, token, nil
}

func decodeExactJSON(encoded []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ErrInvalidPupState
	}
	return nil
}

func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, current := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if current != '-' {
				return false
			}
			continue
		}
		if !((current >= '0' && current <= '9') || (current >= 'a' && current <= 'f') || (current >= 'A' && current <= 'F')) {
			return false
		}
	}
	return true
}

func stringsRepeat(value string, count int) string {
	var result bytes.Buffer
	for range count {
		result.WriteString(value)
	}
	return result.String()
}
