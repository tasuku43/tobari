package credentialhost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	maxVisibleOutputBytes = 64 << 10
	maxProcessStdoutBytes = 64 << 10
	maxProcessStderrBytes = 16 << 10
	refreshTimeout        = 45 * time.Second
)

var (
	ErrCommandFailed      = errors.New("AWS CLI host command failed")
	ErrOutputLimit        = errors.New("AWS CLI host command output exceeded its limit")
	ErrVisibleOutput      = errors.New("AWS CLI visible output delivery failed")
	ErrInvalidCredentials = errors.New("AWS CLI returned invalid temporary credentials")

	accessKeyPattern    = regexp.MustCompile(`^[A-Z0-9]{16,128}$`)
	secretKeyPattern    = regexp.MustCompile(`^[A-Za-z0-9/+=_-]{20,128}$`)
	sessionTokenPattern = regexp.MustCompile(`^[A-Za-z0-9/+=_-]+$`)
)

type OutputStream uint8

const (
	OutputStdout OutputStream = iota + 1
	OutputStderr
)

// VisibleOutput receives bounded AWS CLI device-login text. The caller owns
// presentation, prompt collection, and any trusted browser opening.
type VisibleOutput func(OutputStream, []byte) error

// Driver owns only trusted-host AWS CLI state and fixed process execution.
type Driver struct {
	runner    CommandRunner
	tempRoot  string
	now       func() time.Time
	removeAll func(string) error
}

func NewDriver(runner CommandRunner) *Driver {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &Driver{runner: runner, now: time.Now, removeAll: os.RemoveAll}
}

// Login runs the fixed device-code login command in a private temporary HOME.
// State is returned only after command success and strict cache packing.
func (d *Driver) Login(
	ctx context.Context,
	awsExecutable string,
	profile ProfileConfig,
	visible VisibleOutput,
) (state State, resultErr error) {
	if ctx == nil {
		return State{}, ErrCommandFailed
	}
	if err := validateProfile(profile); err != nil {
		return State{}, err
	}
	canonical, digest, err := resolveExecutable(awsExecutable)
	if err != nil {
		return State{}, err
	}
	home, err := d.prepareHome(profile, nil)
	if err != nil {
		return State{}, ErrCommandFailed
	}
	defer func() {
		if err := d.cleanupHome(home); err != nil {
			state.Clear()
			resultErr = ErrCommandFailed
		}
	}()

	visibleOutput := newVisibleLimiter(maxVisibleOutputBytes, visible)
	command := Command{
		Path: canonical,
		Args: []string{
			"sso", "login",
			"--profile", fixedProfileName,
			"--use-device-code",
			"--no-browser",
			"--no-cli-pager",
		},
		Env:    commandEnvironment(home),
		Dir:    home,
		Stdout: visibleOutput.writer(OutputStdout),
		Stderr: visibleOutput.writer(OutputStderr),
	}
	runErr := d.runner.Run(ctx, command)
	if ctx.Err() != nil {
		return State{}, ctx.Err()
	}
	if outputErr := visibleOutput.err(); outputErr != nil {
		return State{}, outputErr
	}
	if runErr != nil {
		return State{}, ErrCommandFailed
	}
	if err := verifyExecutable(canonical, digest); err != nil {
		return State{}, err
	}
	cache, err := packCache(cachePath(home))
	if err != nil {
		return State{}, err
	}
	state, err = newState(profile, canonical, digest, cache)
	if err != nil {
		return State{}, ErrInvalidState
	}
	return state, nil
}

// Refresh materializes opaque state into a private HOME, revalidates the
// executable digest, and exports one temporary credential tuple. The returned
// State contains any cache update made by the AWS CLI.
func (d *Driver) Refresh(ctx context.Context, state State) (TemporaryCredentials, State, error) {
	if state.DriverID() == ConsoleDriverID {
		return d.refreshConsole(ctx, state)
	}
	return d.refresh(ctx, state)
}

func (d *Driver) refresh(ctx context.Context, state State) (
	credentials TemporaryCredentials, updated State, resultErr error,
) {
	if ctx == nil {
		return TemporaryCredentials{}, State{}, ErrCommandFailed
	}
	if err := validateStatePayload(state.payload); err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	boundedContext, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	profile := state.Profile()
	home, err := d.prepareHome(profile, state.payload.Cache)
	if err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	defer func() {
		if err := d.cleanupHome(home); err != nil {
			credentials.Clear()
			updated.Clear()
			resultErr = ErrCommandFailed
		}
	}()
	if err := verifyExecutable(state.payload.Executable.Path, state.payload.Executable.SHA256); err != nil {
		return TemporaryCredentials{}, State{}, err
	}

	stdout := newBoundedBuffer(maxProcessStdoutBytes)
	stderr := newBoundedBuffer(maxProcessStderrBytes)
	command := Command{
		Path: state.payload.Executable.Path,
		Args: []string{
			"configure", "export-credentials",
			"--profile", fixedProfileName,
			"--format", "process",
			"--no-cli-pager",
			"--cli-connect-timeout", "10",
			"--cli-read-timeout", "30",
		},
		Env:    commandEnvironment(home),
		Dir:    home,
		Stdout: stdout,
		Stderr: stderr,
	}
	runErr := d.runner.Run(boundedContext, command)
	if boundedContext.Err() != nil {
		return TemporaryCredentials{}, State{}, boundedContext.Err()
	}
	if stdout.err() != nil || stderr.err() != nil {
		return TemporaryCredentials{}, State{}, ErrOutputLimit
	}
	if runErr != nil {
		return TemporaryCredentials{}, State{}, ErrCommandFailed
	}
	credentials, err = parseProcessCredentials(stdout.bytes(), d.currentTime())
	if err != nil {
		return TemporaryCredentials{}, State{}, err
	}
	if err := verifyExecutable(state.payload.Executable.Path, state.payload.Executable.SHA256); err != nil {
		return TemporaryCredentials{}, State{}, err
	}
	cache, err := packCache(cachePath(home))
	if err != nil {
		return TemporaryCredentials{}, State{}, err
	}
	updated, err = newState(profile, state.payload.Executable.Path, state.payload.Executable.SHA256, cache)
	if err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	return credentials, updated, nil
}

func (d *Driver) cleanupHome(home string) error {
	removeAll := d.removeAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	return removeAll(home)
}

// TemporaryCredentials keeps secret values private and redacts both ordinary
// and Go-syntax formatting. Explicit accessors are the only disclosure path.
type TemporaryCredentials struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	expiration      time.Time
}

func (c TemporaryCredentials) AccessKeyID() string     { return c.accessKeyID }
func (c TemporaryCredentials) SecretAccessKey() string { return c.secretAccessKey }
func (c TemporaryCredentials) SessionToken() string    { return c.sessionToken }
func (c TemporaryCredentials) Expiration() time.Time   { return c.expiration }
func (TemporaryCredentials) String() string            { return "credentialhost.TemporaryCredentials{redacted}" }
func (TemporaryCredentials) GoString() string          { return "credentialhost.TemporaryCredentials{redacted}" }

func (c *TemporaryCredentials) Clear() {
	if c == nil {
		return
	}
	c.accessKeyID = ""
	c.secretAccessKey = ""
	c.sessionToken = ""
	c.expiration = time.Time{}
}

type processCredentials struct {
	Version         int    `json:"Version"`
	AccessKeyID     string `json:"AccessKeyId"`
	SecretAccessKey string `json:"SecretAccessKey"`
	SessionToken    string `json:"SessionToken"`
	Expiration      string `json:"Expiration"`
}

func parseProcessCredentials(encoded []byte, now time.Time) (TemporaryCredentials, error) {
	if len(encoded) == 0 || len(encoded) > maxProcessStdoutBytes {
		return TemporaryCredentials{}, ErrInvalidCredentials
	}
	var response processCredentials
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return TemporaryCredentials{}, ErrInvalidCredentials
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return TemporaryCredentials{}, ErrInvalidCredentials
	}
	expiration, err := time.Parse(time.RFC3339, response.Expiration)
	if err != nil || response.Version != 1 || !accessKeyPattern.MatchString(response.AccessKeyID) ||
		!secretKeyPattern.MatchString(response.SecretAccessKey) ||
		len(response.SessionToken) < 16 || len(response.SessionToken) > 16384 ||
		!sessionTokenPattern.MatchString(response.SessionToken) || !expiration.After(now) {
		return TemporaryCredentials{}, ErrInvalidCredentials
	}
	return TemporaryCredentials{
		accessKeyID:     response.AccessKeyID,
		secretAccessKey: response.SecretAccessKey,
		sessionToken:    response.SessionToken,
		expiration:      expiration.UTC(),
	}, nil
}

func (d *Driver) prepareHome(profile ProfileConfig, cache []stateCacheFile) (home string, resultErr error) {
	configuration, err := renderProfile(profile)
	if err != nil {
		return "", err
	}
	return d.prepareHomeState(configuration, filepath.Join(".aws", "sso", "cache"), cacheNamePattern, cache)
}

func (d *Driver) prepareHomeState(
	configuration []byte,
	relativeCacheDirectory string,
	namePattern *regexp.Regexp,
	cache []stateCacheFile,
) (home string, resultErr error) {
	if len(configuration) == 0 || relativeCacheDirectory == "" || namePattern == nil {
		return "", ErrInvalidState
	}
	var err error
	home, err = os.MkdirTemp(d.tempRoot, "tobari-aws-host-*")
	if err != nil {
		return "", err
	}
	cleanupPath := home
	failed := true
	defer func() {
		if failed {
			if err := d.cleanupHome(cleanupPath); err != nil {
				resultErr = ErrCommandFailed
			}
			home = ""
		}
	}()
	if err := os.Chmod(home, 0o700); err != nil { // #nosec G302 -- home is a directory; owner-only 0700 is the required private mode.
		return "", err
	}
	homeInfo, err := os.Lstat(home)
	if err != nil || !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 || homeInfo.Mode().Perm()&0o077 != 0 {
		return "", ErrInvalidState
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return "", err
	}
	defer root.Close()
	openedHome, err := root.Stat(".")
	if err != nil || !openedHome.IsDir() || openedHome.Mode().Perm()&0o077 != 0 || !os.SameFile(homeInfo, openedHome) {
		return "", ErrInvalidState
	}
	if err := root.Mkdir(".aws", 0o700); err != nil {
		return "", err
	}
	if err := root.WriteFile(filepath.Join(".aws", "config"), configuration, 0o600); err != nil {
		return "", err
	}
	if err := root.WriteFile(filepath.Join(".aws", "credentials"), nil, 0o600); err != nil {
		return "", err
	}
	if err := root.MkdirAll(relativeCacheDirectory, 0o700); err != nil {
		return "", err
	}
	if err := root.Chmod(relativeCacheDirectory, 0o700); err != nil { // #nosec G302 -- cache is a directory; owner-only 0700 is the required private mode.
		return "", err
	}
	for _, item := range cache {
		content, decodeErr := base64.RawURLEncoding.DecodeString(item.ContentBase64URL)
		defer clear(content)
		if decodeErr != nil || !namePattern.MatchString(item.Name) || !validJSONObject(content) {
			return "", ErrInvalidState
		}
		path := filepath.Join(relativeCacheDirectory, item.Name)
		file, openErr := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if openErr != nil {
			return "", ErrInvalidState
		}
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return "", ErrInvalidState
		}
	}
	failed = false
	return home, nil
}

func cachePath(home string) string {
	return filepath.Join(home, ".aws", "sso", "cache")
}

func commandEnvironment(home string) []string {
	return []string{
		"HOME=" + home,
		"AWS_CONFIG_FILE=" + filepath.Join(home, ".aws", "config"),
		"AWS_SHARED_CREDENTIALS_FILE=" + filepath.Join(home, ".aws", "credentials"),
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_PAGER=",
		"AWS_CLI_AUTO_PROMPT=off",
		"AWS_SDK_LOAD_CONFIG=1",
		"LC_ALL=C",
		"PATH=/usr/bin:/bin",
	}
}

func (d *Driver) currentTime() time.Time {
	if d.now == nil {
		return time.Now()
	}
	return d.now()
}

type visibleLimiter struct {
	mu        sync.Mutex
	remaining int
	callback  VisibleOutput
	failure   error
}

type visibleWriter struct {
	limiter *visibleLimiter
	stream  OutputStream
}

func newVisibleLimiter(limit int, callback VisibleOutput) *visibleLimiter {
	return &visibleLimiter{remaining: limit, callback: callback}
}

func (l *visibleLimiter) writer(stream OutputStream) io.Writer {
	return visibleWriter{limiter: l, stream: stream}
}

func (w visibleWriter) Write(content []byte) (int, error) {
	l := w.limiter
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failure != nil {
		return 0, l.failure
	}
	if len(content) > l.remaining {
		l.failure = ErrOutputLimit
		return 0, l.failure
	}
	if l.callback != nil {
		visible := append([]byte(nil), content...)
		if err := l.callback(w.stream, visible); err != nil {
			l.failure = ErrVisibleOutput
			return 0, l.failure
		}
	}
	l.remaining -= len(content)
	return len(content), nil
}

func (l *visibleLimiter) err() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.failure
}

type boundedBuffer struct {
	mu      sync.Mutex
	limit   int
	content bytes.Buffer
	failure error
}

func newBoundedBuffer(limit int) *boundedBuffer {
	return &boundedBuffer{limit: limit}
}

func (b *boundedBuffer) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil {
		return 0, b.failure
	}
	if len(content) > b.limit-b.content.Len() {
		b.failure = ErrOutputLimit
		return 0, b.failure
	}
	return b.content.Write(content)
}

func (b *boundedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.content.Bytes()...)
}

func (b *boundedBuffer) err() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.failure
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f || strings.ContainsRune("\u2028\u2029", character) {
			return true
		}
	}
	return false
}
