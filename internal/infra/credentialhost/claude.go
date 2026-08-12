package credentialhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

const (
	ClaudeDriverID       = "anthropic_claude_setup_token"
	ClaudeAccountLabel   = "claude-user-inference"
	claudeVersionOutput  = "2.1.220 (Claude Code)"
	claudeTemporaryTag   = "tobari-claude-host-*"
	claudeConfigName     = ".claude"
	maxClaudeVersionSize = 128

	defaultClaudeLoginTimeout = 10 * time.Minute
	claudeProcessWaitDelay    = 2 * time.Second
)

var (
	ErrClaudeExecutable     = errors.New("Claude Code executable is invalid")
	ErrClaudeVersion        = errors.New("Claude Code version is unsupported")
	ErrClaudeLoginSetup     = errors.New("Claude Code login setup failed")
	ErrClaudeTTYRequired    = errors.New("Claude Code login terminal streams are required")
	ErrClaudeLoginCancelled = errors.New("Claude Code login was cancelled")
	ErrClaudeLoginFailed    = errors.New("Claude Code login failed")
	ErrClaudeOutputLimit    = errors.New("Claude Code output exceeded its limit")
	ErrClaudeOutputFraming  = errors.New("Claude Code output framing is invalid")
	ErrClaudeTokenCapture   = errors.New("Claude Code setup token capture failed")
	ErrClaudeVisibleOutput  = errors.New("Claude Code visible output delivery failed")
	ErrClaudeLoginCleanup   = errors.New("Claude Code login cleanup failed")
)

// ClaudeLoginStreams are the trusted-host terminal input and the single
// reviewed visible-output destination for the setup-token flow. The child
// receives a private PTY for output; provider bytes remain isolated through
// completion and only fixed parser-authored guidance can cross Output after
// exact validation.
type ClaudeLoginStreams struct {
	Stdin  io.Reader
	Output io.Writer
}

// ClaudeCommand is the complete fixed process boundary for Claude Code
// version validation and setup-token acquisition. Terminal requests a private
// PTY whose combined output is written only to Stdout.
type ClaudeCommand struct {
	Path     string
	Args     []string
	Env      []string
	Dir      string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Terminal bool
}

// ClaudeCommandRunner permits synthetic tests without a browser, network, or
// installed Claude Code binary.
type ClaudeCommandRunner interface {
	Run(context.Context, ClaudeCommand) error
}

// ClaudeExecRunner executes a canonical digest-bound absolute binary without
// a shell. Interactive execution uses a private PTY so Ink observes a terminal
// while all emitted bytes still pass through the secret-suppressing parser.
type ClaudeExecRunner struct{}

func (ClaudeExecRunner) Run(ctx context.Context, command ClaudeCommand) error {
	if command.Terminal {
		return runClaudeTerminalCommand(ctx, command)
	}
	process := exec.CommandContext(ctx, command.Path, command.Args...) // #nosec G204 -- ClaudeDriver validates and digest-binds the absolute executable; argv is fixed.
	process.Env = append([]string(nil), command.Env...)
	process.Dir = command.Dir
	process.Stdin = command.Stdin
	process.Stdout = command.Stdout
	process.Stderr = command.Stderr
	process.WaitDelay = claudeProcessWaitDelay
	return process.Run()
}

// claudeOutputParser is the injectable streaming boundary between the private
// PTY and visible host output. Finish must clear all retained terminal bytes,
// including any token candidate, whether parsing succeeds or fails.
type claudeOutputParser interface {
	io.Writer
	Finish() ([]byte, error)
}

// claudeOutputParserFactory creates one parser per login attempt. Keeping the
// seam package-private prevents production wiring from replacing the fixed
// secret-suppression contract while unit tests can still inject a sentinel.
type claudeOutputParserFactory func(io.Writer) claudeOutputParser

// ClaudeDriver owns the one reviewed Claude Code 2.1.220 setup-token flow.
type ClaudeDriver struct {
	runner        ClaudeCommandRunner
	parserFactory claudeOutputParserFactory
	tempRoot      string
	removeAll     func(string) error
	timeout       time.Duration
}

func NewClaudeDriver(runner ClaudeCommandRunner) *ClaudeDriver {
	if runner == nil {
		runner = ClaudeExecRunner{}
	}
	parserFactory := func(visible io.Writer) claudeOutputParser {
		return newClaudeSetupOutputParser(visible)
	}
	return &ClaudeDriver{
		runner: runner, parserFactory: parserFactory, removeAll: os.RemoveAll,
		timeout: defaultClaudeLoginTimeout,
	}
}

// ClaudeCredential keeps the captured setup token private and binds it to the
// exact executable digest used for acquisition. Token returns a copy as the
// only disclosure path.
type ClaudeCredential struct {
	token          []byte
	accountLabel   string
	driverID       string
	driverRevision string
}

func (c ClaudeCredential) Token() []byte          { return append([]byte(nil), c.token...) }
func (c ClaudeCredential) AccountLabel() string   { return c.accountLabel }
func (c ClaudeCredential) DriverID() string       { return c.driverID }
func (c ClaudeCredential) DriverRevision() string { return c.driverRevision }
func (ClaudeCredential) String() string           { return "credentialhost.ClaudeCredential{redacted}" }
func (ClaudeCredential) GoString() string         { return "credentialhost.ClaudeCredential{redacted}" }

// Clear overwrites and releases the private token after the caller commits it
// to the Broker. Copies previously returned by Token remain the caller's
// responsibility.
func (c *ClaudeCredential) Clear() {
	if c == nil {
		return
	}
	clear(c.token)
	c.token = nil
	c.accountLabel = ""
	c.driverID = ""
	c.driverRevision = ""
}

// Login validates the exact 2.1.220 binary in the same private environment as
// acquisition, runs only `setup-token`, and returns a credential only after
// successful strict output parsing, post-run digest verification, and cleanup.
func (d *ClaudeDriver) Login(
	ctx context.Context,
	claudeExecutable string,
	streams ClaudeLoginStreams,
) (credential ClaudeCredential, resultErr error) {
	if ctx == nil {
		return ClaudeCredential{}, ErrClaudeLoginFailed
	}
	if streams.Stdin == nil || streams.Output == nil {
		return ClaudeCredential{}, ErrClaudeTTYRequired
	}
	if d == nil || d.runner == nil || d.parserFactory == nil {
		return ClaudeCredential{}, ErrClaudeLoginSetup
	}
	if ctx.Err() != nil {
		return ClaudeCredential{}, ErrClaudeLoginCancelled
	}
	if !filepath.IsAbs(claudeExecutable) || filepath.Clean(claudeExecutable) != claudeExecutable {
		return ClaudeCredential{}, ErrClaudeExecutable
	}
	canonical, digest, err := resolveExecutable(claudeExecutable)
	if err != nil {
		return ClaudeCredential{}, ErrClaudeExecutable
	}
	if err := verifyClaudeExecutable(canonical, digest); err != nil {
		return ClaudeCredential{}, err
	}
	home, configDirectory, err := d.prepareClaudeHome()
	if err != nil {
		if errors.Is(err, ErrClaudeLoginCleanup) {
			return ClaudeCredential{}, err
		}
		return ClaudeCredential{}, ErrClaudeLoginSetup
	}
	defer func() {
		if err := d.cleanupClaudeHome(home); err != nil {
			credential.Clear()
			resultErr = ErrClaudeLoginCleanup
		}
	}()

	timeout := d.timeout
	if timeout <= 0 {
		timeout = defaultClaudeLoginTimeout
	}
	loginContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	environment := claudeEnvironment(home, configDirectory)
	if err := d.validateClaudeVersion(loginContext, canonical, digest, home, environment); err != nil {
		return ClaudeCredential{}, err
	}
	credential, resultErr = d.acquireClaudeCredential(
		loginContext, canonical, digest, home, environment, streams,
	)
	if resultErr != nil {
		credential.Clear()
		credential = ClaudeCredential{}
	}
	return credential, resultErr
}

func (d *ClaudeDriver) validateClaudeVersion(
	ctx context.Context,
	executable string,
	digest string,
	home string,
	environment []string,
) error {
	stdout := newBoundedBuffer(maxClaudeVersionSize)
	stderr := newBoundedBuffer(maxClaudeVersionSize)
	runErr := d.runner.Run(ctx, ClaudeCommand{
		Path: executable, Args: []string{"--version"},
		Env: append([]string(nil), environment...), Dir: home,
		Stdout: stdout, Stderr: stderr,
	})
	if claudeCommandTimedOut(ctx, runErr) {
		return context.DeadlineExceeded
	}
	if claudeCommandCancelled(ctx, runErr) {
		return ErrClaudeLoginCancelled
	}
	if stdout.err() != nil || stderr.err() != nil {
		return ErrClaudeOutputLimit
	}
	if runErr != nil || !validClaudeVersion(stdout.bytes(), stderr.bytes()) {
		return ErrClaudeVersion
	}
	return verifyClaudeExecutable(executable, digest)
}

func (d *ClaudeDriver) acquireClaudeCredential(
	ctx context.Context,
	executable string,
	digest string,
	home string,
	environment []string,
	streams ClaudeLoginStreams,
) (ClaudeCredential, error) {
	parser := d.parserFactory(streams.Output)
	if parser == nil {
		return ClaudeCredential{}, ErrClaudeLoginSetup
	}
	runErr := d.runner.Run(ctx, ClaudeCommand{
		Path: executable, Args: []string{"setup-token"},
		Env: append([]string(nil), environment...), Dir: home,
		Stdin: streams.Stdin, Stdout: parser, Terminal: true,
	})
	// Finish clears every retained output byte on all paths. Any returned copy
	// is cleared below unless it becomes the credential's private storage.
	token, parseErr := parser.Finish()
	if token != nil {
		defer clear(token)
	}
	if err := verifyClaudeExecutable(executable, digest); err != nil {
		return ClaudeCredential{}, err
	}
	if claudeCommandTimedOut(ctx, runErr) {
		return ClaudeCredential{}, context.DeadlineExceeded
	}
	if claudeCommandCancelled(ctx, runErr) {
		return ClaudeCredential{}, ErrClaudeLoginCancelled
	}
	if runErr != nil {
		switch {
		case errors.Is(runErr, ErrClaudeTTYRequired):
			return ClaudeCredential{}, ErrClaudeTTYRequired
		case errors.Is(runErr, ErrClaudeLoginSetup):
			return ClaudeCredential{}, ErrClaudeLoginSetup
		case errors.Is(runErr, ErrClaudeOutputLimit),
			errors.Is(runErr, ErrClaudeOutputFraming),
			errors.Is(runErr, ErrClaudeVisibleOutput):
			if parseErr != nil {
				return ClaudeCredential{}, parseErr
			}
			return ClaudeCredential{}, runErr
		default:
			return ClaudeCredential{}, ErrClaudeLoginFailed
		}
	}
	if parseErr != nil {
		return ClaudeCredential{}, parseErr
	}
	return ClaudeCredential{
		token:          append([]byte(nil), token...),
		accountLabel:   ClaudeAccountLabel,
		driverID:       ClaudeDriverID,
		driverRevision: digest,
	}, nil
}

func (d *ClaudeDriver) prepareClaudeHome() (home string, configDirectory string, resultErr error) {
	root := d.tempRoot
	if root == "" {
		root = os.TempDir()
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	home, err = os.MkdirTemp(filepath.Clean(absoluteRoot), claudeTemporaryTag)
	if err != nil {
		return "", "", err
	}
	failed := true
	defer func() {
		if failed {
			removeAll := d.removeAll
			if removeAll == nil {
				removeAll = os.RemoveAll
			}
			if cleanupErr := removeAll(home); cleanupErr != nil {
				resultErr = ErrClaudeLoginCleanup
			}
			home = ""
			configDirectory = ""
		}
	}()
	if err := os.Chmod(home, 0o700); err != nil { // #nosec G302 -- owner-only 0700 is the required private directory mode.
		return "", "", err
	}
	homeInfo, err := os.Lstat(home)
	if err != nil || !homeInfo.IsDir() || homeInfo.Mode()&os.ModeSymlink != 0 || homeInfo.Mode().Perm()&0o077 != 0 {
		return "", "", ErrClaudeLoginSetup
	}
	openedRoot, err := os.OpenRoot(home)
	if err != nil {
		return "", "", err
	}
	defer openedRoot.Close()
	openedHome, err := openedRoot.Stat(".")
	if err != nil || !openedHome.IsDir() || openedHome.Mode().Perm()&0o077 != 0 || !os.SameFile(homeInfo, openedHome) {
		return "", "", ErrClaudeLoginSetup
	}
	if err := openedRoot.Mkdir(claudeConfigName, 0o700); err != nil {
		return "", "", err
	}
	configDirectory = filepath.Join(home, claudeConfigName)
	configInfo, err := os.Lstat(configDirectory)
	if err != nil || !configInfo.IsDir() || configInfo.Mode()&os.ModeSymlink != 0 || configInfo.Mode().Perm()&0o077 != 0 {
		return "", "", ErrClaudeLoginSetup
	}
	failed = false
	return home, configDirectory, nil
}

func (d *ClaudeDriver) cleanupClaudeHome(home string) error {
	removeAll := d.removeAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	return removeAll(home)
}

func claudeEnvironment(home, configDirectory string) []string {
	return []string{
		"HOME=" + home,
		"CLAUDE_CONFIG_DIR=" + configDirectory,
		"NO_COLOR=1",
		"LC_ALL=C",
		"DISABLE_AUTOUPDATER=1",
		"PATH=/usr/bin:/bin",
	}
}

func verifyClaudeExecutable(path, digest string) error {
	if err := verifyExecutable(path, digest); err != nil {
		return ErrClaudeExecutable
	}
	return nil
}

func validClaudeVersion(stdout, stderr []byte) bool {
	if len(stderr) != 0 {
		return false
	}
	framed := stdout
	if bytes.HasSuffix(framed, []byte("\r\n")) {
		framed = framed[:len(framed)-2]
	} else if bytes.HasSuffix(framed, []byte("\n")) {
		framed = framed[:len(framed)-1]
	}
	return bytes.Equal(framed, []byte(claudeVersionOutput))
}

func claudeCommandCancelled(ctx context.Context, commandErr error) bool {
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

func claudeCommandTimedOut(ctx context.Context, commandErr error) bool {
	return ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		errors.Is(commandErr, context.DeadlineExceeded) ||
		errors.Is(commandErr, os.ErrDeadlineExceeded)
}
