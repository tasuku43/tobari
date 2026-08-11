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
	CodexDriverID              = "openai_codex_chatgpt_oauth"
	CodexVersion               = "0.146.0"
	codexVersionOutput         = "codex-cli " + CodexVersion
	codexTemporaryDirectoryTag = "tobari-codex-host-*"
	codexConfigDirectoryName   = ".codex"
	codexAuthFileName          = "auth.json"
	codexVersionTimeout        = 5 * time.Second
	defaultCodexLoginTimeout   = 10 * time.Minute
	maxCodexVersionBytes       = 128
	maxCodexVisibleOutputBytes = 64 << 10
)

var (
	ErrCodexExecutable     = errors.New("Codex CLI executable is invalid")
	ErrCodexVersion        = errors.New("Codex CLI version is unsupported")
	ErrCodexLoginSetup     = errors.New("Codex CLI login setup failed")
	ErrCodexLoginStreams   = errors.New("Codex CLI login streams are required")
	ErrCodexLoginCancelled = errors.New("Codex CLI login was cancelled")
	ErrCodexLoginTimeout   = errors.New("Codex CLI login timed out")
	ErrCodexLoginFailed    = errors.New("Codex CLI login failed")
	ErrCodexOutputLimit    = errors.New("Codex CLI output exceeded its limit")
	ErrCodexVisibleOutput  = errors.New("Codex CLI visible output delivery failed")
	ErrCodexAuthCapture    = errors.New("Codex CLI authentication capture failed")
	ErrCodexLoginCleanup   = errors.New("Codex CLI login cleanup failed")
)

// CodexLoginStreams are the trusted-host terminal streams used by the fixed
// device-code login. The driver bounds visible output but never interprets a
// device code, URL, or provider diagnostic from these streams.
type CodexLoginStreams struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// CodexDriver owns the one reviewed trusted-host Codex 0.146.0 ChatGPT OAuth
// acquisition flow. Its runner seam exists only for deterministic local tests.
type CodexDriver struct {
	runner       CommandRunner
	tempRoot     string
	removeAll    func(string) error
	loginTimeout time.Duration
}

func NewCodexDriver(runner CommandRunner) *CodexDriver {
	if runner == nil {
		runner = ExecRunner{}
	}
	return &CodexDriver{
		runner:       runner,
		removeAll:    os.RemoveAll,
		loginTimeout: defaultCodexLoginTimeout,
	}
}

// Login runs only Codex's public device-code flow in a private file-backed
// HOME. A result is returned after exact version, executable identity, state,
// and cleanup checks all succeed.
func (d *CodexDriver) Login(
	ctx context.Context,
	executable string,
	streams CodexLoginStreams,
) (credential CodexCredential, resultErr error) {
	if ctx == nil {
		return CodexCredential{}, ErrCodexLoginFailed
	}
	if streams.Stdin == nil || streams.Stdout == nil || streams.Stderr == nil {
		return CodexCredential{}, ErrCodexLoginStreams
	}
	if ctx.Err() != nil {
		return CodexCredential{}, ErrCodexLoginCancelled
	}
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		return CodexCredential{}, ErrCodexExecutable
	}
	canonical, digest, err := resolveExecutable(executable)
	if err != nil {
		return CodexCredential{}, ErrCodexExecutable
	}
	home, codexHome, err := d.prepareCodexHome()
	if err != nil {
		return CodexCredential{}, ErrCodexLoginSetup
	}
	defer func() {
		if err := d.cleanupCodexHome(home); err != nil {
			credential.Clear()
			resultErr = ErrCodexLoginCleanup
		}
	}()

	environment := codexEnvironment(home, codexHome)
	if err := d.requireCodexVersion(ctx, canonical, digest, home, environment); err != nil {
		return CodexCredential{}, err
	}
	credential, resultErr = d.acquireCodexCredential(
		ctx,
		canonical,
		digest,
		home,
		codexHome,
		environment,
		streams,
	)
	if resultErr != nil {
		credential.Clear()
		return CodexCredential{}, resultErr
	}
	return credential, nil
}

func (d *CodexDriver) requireCodexVersion(
	ctx context.Context,
	executable string,
	digest string,
	home string,
	environment []string,
) error {
	stdout := newBoundedBuffer(maxCodexVersionBytes)
	stderr := newBoundedBuffer(maxCodexVersionBytes)
	bounded, cancel := context.WithTimeout(ctx, codexVersionTimeout)
	defer cancel()
	runErr := d.runner.Run(bounded, Command{
		Path:   executable,
		Args:   []string{"--version"},
		Env:    append([]string(nil), environment...),
		Dir:    home,
		Stdout: stdout,
		Stderr: stderr,
	})
	if ctx.Err() != nil || codexCommandCancelled(ctx, runErr) {
		return ErrCodexLoginCancelled
	}
	if bounded.Err() != nil || runErr != nil || stdout.err() != nil || stderr.err() != nil ||
		len(stderr.bytes()) != 0 || !exactCodexVersion(stdout.bytes()) {
		return ErrCodexVersion
	}
	if err := verifyExecutable(executable, digest); err != nil {
		return ErrCodexExecutable
	}
	return nil
}

func (d *CodexDriver) acquireCodexCredential(
	ctx context.Context,
	executable string,
	digest string,
	home string,
	codexHome string,
	environment []string,
	streams CodexLoginStreams,
) (CodexCredential, error) {
	if err := verifyExecutable(executable, digest); err != nil {
		return CodexCredential{}, ErrCodexExecutable
	}
	visibleOutput := newVisibleLimiter(maxCodexVisibleOutputBytes, func(stream OutputStream, content []byte) error {
		writer := streams.Stdout
		if stream == OutputStderr {
			writer = streams.Stderr
		}
		written, err := writer.Write(content)
		if err != nil {
			return err
		}
		if written != len(content) {
			return io.ErrShortWrite
		}
		return nil
	})
	timeout := d.loginTimeout
	if timeout <= 0 {
		timeout = defaultCodexLoginTimeout
	}
	bounded, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runErr := d.runner.Run(bounded, Command{
		Path: executable,
		Args: []string{
			"login", "--device-auth",
			"-c", `cli_auth_credentials_store="file"`,
			"-c", "check_for_update_on_startup=false",
		},
		Env:    append([]string(nil), environment...),
		Dir:    home,
		Stdin:  streams.Stdin,
		Stdout: visibleOutput.writer(OutputStdout),
		Stderr: visibleOutput.writer(OutputStderr),
	})
	if ctx.Err() != nil {
		return CodexCredential{}, ErrCodexLoginCancelled
	}
	if errors.Is(bounded.Err(), context.DeadlineExceeded) {
		return CodexCredential{}, ErrCodexLoginTimeout
	}
	if codexCommandCancelled(ctx, runErr) {
		return CodexCredential{}, ErrCodexLoginCancelled
	}
	if outputErr := visibleOutput.err(); outputErr != nil {
		if errors.Is(outputErr, ErrOutputLimit) {
			return CodexCredential{}, ErrCodexOutputLimit
		}
		return CodexCredential{}, ErrCodexVisibleOutput
	}
	if runErr != nil {
		return CodexCredential{}, ErrCodexLoginFailed
	}
	if err := verifyExecutable(executable, digest); err != nil {
		return CodexCredential{}, ErrCodexExecutable
	}
	authBytes, err := readPrivateCodexAuthFile(codexHome, codexAuthFileName, maxCodexAuthBytes)
	if err != nil {
		return CodexCredential{}, ErrCodexAuthCapture
	}
	defer clear(authBytes)
	auth, accountLabel, err := parseCodexAuth(authBytes)
	if err != nil {
		return CodexCredential{}, err
	}
	credential, err := newCodexCredential(executable, digest, auth, accountLabel)
	if err != nil {
		return CodexCredential{}, ErrCodexAuthCapture
	}
	return credential, nil
}

func (d *CodexDriver) prepareCodexHome() (home string, codexHome string, resultErr error) {
	root := d.tempRoot
	if root == "" {
		root = os.TempDir()
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	home, err = os.MkdirTemp(filepath.Clean(absoluteRoot), codexTemporaryDirectoryTag)
	if err != nil {
		return "", "", err
	}
	failed := true
	defer func() {
		if failed {
			if cleanupErr := d.cleanupCodexHome(home); cleanupErr != nil {
				resultErr = ErrCodexLoginSetup
			}
			home = ""
			codexHome = ""
		}
	}()
	if err := os.Chmod(home, 0o700); err != nil { // #nosec G302 -- owner-only temporary HOME requires traversal by its owner.
		return "", "", err
	}
	if err := requirePrivateCodexDirectory(home); err != nil {
		return "", "", err
	}
	codexHome = filepath.Join(home, codexConfigDirectoryName)
	if err := os.Mkdir(codexHome, 0o700); err != nil {
		return "", "", err
	}
	if err := os.Chmod(codexHome, 0o700); err != nil { // #nosec G302 -- owner-only CODEX_HOME requires traversal by its owner.
		return "", "", err
	}
	if err := requirePrivateCodexDirectory(codexHome); err != nil {
		return "", "", err
	}
	failed = false
	return home, codexHome, nil
}

func (d *CodexDriver) cleanupCodexHome(home string) error {
	removeAll := d.removeAll
	if removeAll == nil {
		removeAll = os.RemoveAll
	}
	return removeAll(home)
}

func codexEnvironment(home, codexHome string) []string {
	return []string{
		"HOME=" + home,
		"CODEX_HOME=" + codexHome,
		"DISABLE_AUTOUPDATER=1",
		"NO_COLOR=1",
		"LC_ALL=C",
		"PATH=/usr/bin:/bin",
	}
}

func exactCodexVersion(output []byte) bool {
	framed := output
	if bytes.HasSuffix(framed, []byte("\r\n")) {
		framed = framed[:len(framed)-2]
	} else if bytes.HasSuffix(framed, []byte("\n")) {
		framed = framed[:len(framed)-1]
	}
	return bytes.Equal(framed, []byte(codexVersionOutput))
}

func codexCommandCancelled(ctx context.Context, commandErr error) bool {
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if errors.Is(commandErr, context.Canceled) {
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
