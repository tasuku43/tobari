package credentialhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	githubHost                  = "github.com"
	maxGitHubStatusBytes        = 64 << 10
	maxGitHubTokenBytes         = 32 << 10
	githubProcessWaitDelay      = 2 * time.Second
	githubTemporaryDirectoryTag = "tobari-gh-host-*"
)

var (
	ErrGitHubExecutable     = errors.New("GitHub CLI executable is invalid")
	ErrGitHubLoginSetup     = errors.New("GitHub CLI login setup failed")
	ErrGitHubTTYRequired    = errors.New("GitHub CLI login streams are required")
	ErrGitHubLoginCancelled = errors.New("GitHub CLI login was cancelled")
	ErrGitHubLoginFailed    = errors.New("GitHub CLI login failed")
	ErrGitHubAccountCapture = errors.New("GitHub CLI account capture failed")
	ErrGitHubTokenCapture   = errors.New("GitHub CLI token capture failed")
	ErrGitHubOutputLimit    = errors.New("GitHub CLI output exceeded its limit")
	ErrGitHubLoginCleanup   = errors.New("GitHub CLI login cleanup failed")

	githubAccountPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?$`)
)

// GitHubLoginStreams are the trusted-host terminal streams used only by the
// interactive login command. The caller owns terminal validation, visible
// output projection, and any fixed browser-open side effect derived from it.
type GitHubLoginStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// GitHubCommand is the complete process boundary for the fixed GitHub CLI
// acquisition flow. Login receives the trusted-host terminal streams; status
// and token capture receive private bounded writers instead.
type GitHubCommand struct {
	Path   string
	Args   []string
	Env    []string
	Dir    string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// GitHubCommandRunner permits deterministic process tests without network
// access or an installed GitHub CLI.
type GitHubCommandRunner interface {
	Run(context.Context, GitHubCommand) error
}

// GitHubExecRunner executes a canonical digest-bound absolute binary path
// without a shell.
type GitHubExecRunner struct{}

func (GitHubExecRunner) Run(ctx context.Context, command GitHubCommand) error {
	process := exec.CommandContext(ctx, command.Path, command.Args...) // #nosec G204 -- GitHubDriver validates and digest-binds the absolute executable; argv is fixed.
	process.Env = append([]string(nil), command.Env...)
	process.Dir = command.Dir
	process.Stdin = command.Stdin
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	process.WaitDelay = githubProcessWaitDelay
	return process.Run()
}

// GitHubDriver owns the one reviewed trusted-host GitHub CLI login flow.
type GitHubDriver struct {
	runner    GitHubCommandRunner
	tempRoot  string
	removeAll func(string) error
}

func NewGitHubDriver(runner GitHubCommandRunner) *GitHubDriver {
	if runner == nil {
		runner = GitHubExecRunner{}
	}
	return &GitHubDriver{
		runner:    runner,
		removeAll: os.RemoveAll,
	}
}

// GitHubCredential keeps the captured token private and redacts both ordinary
// and Go-syntax formatting. Token returns a copy as the only disclosure path.
type GitHubCredential struct {
	token        []byte
	accountLabel string
}

func (c GitHubCredential) Token() []byte        { return append([]byte(nil), c.token...) }
func (c GitHubCredential) AccountLabel() string { return c.accountLabel }
func (GitHubCredential) String() string         { return "credentialhost.GitHubCredential{redacted}" }
func (GitHubCredential) GoString() string       { return "credentialhost.GitHubCredential{redacted}" }

// Clear overwrites and releases the private token after the caller has
// committed it to the Broker. Copies previously returned by Token remain the
// caller's responsibility.
func (c *GitHubCredential) Clear() {
	if c == nil {
		return
	}
	clear(c.token)
	c.token = nil
	c.accountLabel = ""
}

// Login runs the fixed GitHub CLI device flow in a private GH_CONFIG_DIR. It
// returns a credential only after exact account and bounded token capture and
// after the temporary directory has been removed successfully.
func (d *GitHubDriver) Login(
	ctx context.Context,
	ghExecutable string,
	streams GitHubLoginStreams,
) (credential GitHubCredential, resultErr error) {
	if ctx == nil {
		return GitHubCredential{}, ErrGitHubLoginFailed
	}
	if streams.Stdin == nil || streams.Stdout == nil || streams.Stderr == nil {
		return GitHubCredential{}, ErrGitHubTTYRequired
	}
	if ctx.Err() != nil {
		return GitHubCredential{}, ErrGitHubLoginCancelled
	}
	canonical, digest, err := resolveExecutable(ghExecutable)
	if err != nil {
		return GitHubCredential{}, ErrGitHubExecutable
	}
	if err := verifyGitHubExecutable(canonical, digest); err != nil {
		return GitHubCredential{}, err
	}
	configDirectory, err := d.prepareGitHubConfigDirectory()
	if err != nil {
		return GitHubCredential{}, ErrGitHubLoginSetup
	}
	defer func() {
		if err := d.cleanupGitHubConfigDirectory(configDirectory); err != nil {
			credential.Clear()
			resultErr = ErrGitHubLoginCleanup
		}
	}()

	credential, resultErr = d.acquireGitHubCredential(
		ctx,
		canonical,
		digest,
		configDirectory,
		streams,
	)
	if resultErr != nil {
		credential = GitHubCredential{}
	}
	return credential, resultErr
}

func (d *GitHubDriver) acquireGitHubCredential(
	ctx context.Context,
	executable string,
	digest string,
	configDirectory string,
	streams GitHubLoginStreams,
) (GitHubCredential, error) {
	environment := githubEnvironment(configDirectory)
	loginErr := d.runner.Run(ctx, GitHubCommand{
		Path: executable,
		Args: []string{
			"auth", "login",
			"--hostname", githubHost,
			"--web",
			"--insecure-storage",
		},
		Env:    append([]string(nil), environment...),
		Dir:    configDirectory,
		Stdin:  streams.Stdin,
		Stdout: streams.Stdout,
		Stderr: streams.Stderr,
	})
	if githubCommandCancelled(ctx, loginErr) {
		return GitHubCredential{}, ErrGitHubLoginCancelled
	}
	if loginErr != nil {
		return GitHubCredential{}, ErrGitHubLoginFailed
	}
	if err := verifyGitHubExecutable(executable, digest); err != nil {
		return GitHubCredential{}, err
	}

	statusOutput := newBoundedBuffer(maxGitHubStatusBytes)
	statusErr := d.runner.Run(ctx, GitHubCommand{
		Path: executable,
		Args: []string{
			"auth", "status",
			"--active",
			"--hostname", githubHost,
			"--json", "hosts",
		},
		Env:    append([]string(nil), environment...),
		Dir:    configDirectory,
		Stdout: statusOutput,
		Stderr: io.Discard,
	})
	if githubCommandCancelled(ctx, statusErr) {
		return GitHubCredential{}, ErrGitHubLoginCancelled
	}
	if statusOutput.err() != nil {
		return GitHubCredential{}, ErrGitHubOutputLimit
	}
	if statusErr != nil {
		return GitHubCredential{}, ErrGitHubAccountCapture
	}
	accountLabel, err := parseGitHubAccount(statusOutput.bytes())
	if err != nil {
		return GitHubCredential{}, err
	}
	if err := verifyGitHubExecutable(executable, digest); err != nil {
		return GitHubCredential{}, err
	}

	// Allow at most the 32 KiB opaque token plus gh's one LF or CRLF framing.
	tokenOutput := newBoundedBuffer(maxGitHubTokenBytes + 2)
	tokenErr := d.runner.Run(ctx, GitHubCommand{
		Path: executable,
		Args: []string{
			"auth", "token",
			"--hostname", githubHost,
		},
		Env:    append([]string(nil), environment...),
		Dir:    configDirectory,
		Stdout: tokenOutput,
		Stderr: io.Discard,
	})
	if githubCommandCancelled(ctx, tokenErr) {
		return GitHubCredential{}, ErrGitHubLoginCancelled
	}
	if tokenOutput.err() != nil {
		return GitHubCredential{}, ErrGitHubOutputLimit
	}
	if tokenErr != nil {
		return GitHubCredential{}, ErrGitHubTokenCapture
	}
	if err := verifyGitHubExecutable(executable, digest); err != nil {
		return GitHubCredential{}, err
	}
	tokenBytes := tokenOutput.bytes()
	defer clear(tokenBytes)
	token, err := parseGitHubToken(tokenBytes)
	if err != nil {
		return GitHubCredential{}, err
	}
	return GitHubCredential{token: token, accountLabel: accountLabel}, nil
}

func (d *GitHubDriver) prepareGitHubConfigDirectory() (string, error) {
	root := d.tempRoot
	if root == "" {
		root = os.TempDir()
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	configDirectory, err := os.MkdirTemp(filepath.Clean(absoluteRoot), githubTemporaryDirectoryTag)
	if err != nil {
		return "", err
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(configDirectory)
		}
	}()
	if err := os.Chmod(configDirectory, 0o700); err != nil { // #nosec G302 -- configDirectory is a directory; owner-only 0700 is the required private mode.
		return "", err
	}
	info, err := os.Lstat(configDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", ErrGitHubLoginSetup
	}
	failed = false
	return configDirectory, nil
}

func (d *GitHubDriver) cleanupGitHubConfigDirectory(configDirectory string) error {
	removeAll := d.removeAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	return removeAll(configDirectory)
}

func verifyGitHubExecutable(path, digest string) error {
	if err := verifyExecutable(path, digest); err != nil {
		return ErrGitHubExecutable
	}
	return nil
}

func githubEnvironment(configDirectory string) []string {
	return []string{
		"HOME=" + configDirectory,
		"GH_CONFIG_DIR=" + configDirectory,
		"GH_PROMPT_DISABLED=1",
		"GH_BROWSER=/bin/true",
		"NO_COLOR=1",
		"LC_ALL=C",
		"PATH=/usr/bin:/bin",
	}
}

func githubCommandCancelled(ctx context.Context, commandErr error) bool {
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if errors.Is(commandErr, context.Canceled) || errors.Is(commandErr, context.DeadlineExceeded) {
		return true
	}
	var exitCoder interface{ ExitCode() int }
	if errors.As(commandErr, &exitCoder) {
		code := exitCoder.ExitCode()
		if code == 130 || code == -int(syscall.SIGINT) {
			return true
		}
	}
	var exitError *exec.ExitError
	if errors.As(commandErr, &exitError) {
		if status, ok := exitError.Sys().(syscall.WaitStatus); ok && status.Signaled() && status.Signal() == syscall.SIGINT {
			return true
		}
	}
	return false
}

func parseGitHubAccount(encoded []byte) (string, error) {
	if len(encoded) == 0 || len(encoded) > maxGitHubStatusBytes || !utf8.Valid(encoded) {
		return "", ErrGitHubAccountCapture
	}
	value, err := decodeGitHubJSON(encoded)
	if err != nil {
		return "", ErrGitHubAccountCapture
	}
	document, ok := value.(map[string]any)
	if !ok || len(document) != 1 {
		return "", ErrGitHubAccountCapture
	}
	hosts, ok := document["hosts"].(map[string]any)
	if !ok {
		return "", ErrGitHubAccountCapture
	}
	entries, ok := hosts[githubHost].([]any)
	if !ok || len(entries) == 0 || len(entries) > 16 {
		return "", ErrGitHubAccountCapture
	}
	activeLogins := make([]string, 0, 1)
	for _, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		active, activeOK := entry["active"].(bool)
		state, stateOK := entry["state"].(string)
		if !activeOK || !active || !stateOK || state != "success" {
			continue
		}
		login, ok := entry["login"].(string)
		if !ok || len(login) > 64 || !githubAccountPattern.MatchString(login) {
			return "", ErrGitHubAccountCapture
		}
		activeLogins = append(activeLogins, login)
	}
	if len(activeLogins) != 1 {
		return "", ErrGitHubAccountCapture
	}
	return activeLogins[0], nil
}

func parseGitHubToken(output []byte) ([]byte, error) {
	framed := output
	if bytes.HasSuffix(framed, []byte("\r\n")) {
		framed = framed[:len(framed)-2]
	} else if bytes.HasSuffix(framed, []byte("\n")) {
		framed = framed[:len(framed)-1]
	}
	if len(framed) == 0 || len(framed) > maxGitHubTokenBytes ||
		bytes.IndexByte(framed, '\n') >= 0 || bytes.IndexByte(framed, '\r') >= 0 {
		return nil, ErrGitHubTokenCapture
	}
	return append([]byte(nil), framed...), nil
}

func decodeGitHubJSON(encoded []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	value, err := readGitHubJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("unexpected trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func readGitHubJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, errors.New("duplicate JSON object key")
			}
			value, err := readGitHubJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if closing != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			value, err := readGitHubJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		closing, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if closing != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}
		return array, nil
	default:
		return nil, errors.New("invalid JSON delimiter")
	}
}
