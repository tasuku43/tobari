package credentialhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

const (
	consoleVersionTimeout = 5 * time.Second
	minimumConsoleMajor   = 2
	minimumConsoleMinor   = 32
)

var (
	ErrConsoleLoginUnsupported = errors.New("AWS CLI console login is unsupported")
	awsCLIVersionPattern       = regexp.MustCompile(`(?:^|\s)aws-cli/([0-9]+)\.([0-9]+)\.[0-9]+(?:\s|$)`)
)

// ConsoleLogin runs the fixed cross-device AWS console-based login flow. The
// caller's terminal is used only by the reviewed AWS executable for the
// returned authorization code; Tobari never parses that code.
func (d *Driver) ConsoleLogin(
	ctx context.Context,
	awsExecutable string,
	profile ConsoleProfileConfig,
	input io.Reader,
	visible VisibleOutput,
) (state State, resultErr error) {
	if ctx == nil || input == nil {
		return State{}, ErrCommandFailed
	}
	if err := validateConsoleProfile(profile); err != nil {
		return State{}, err
	}
	canonical, digest, err := resolveExecutable(awsExecutable)
	if err != nil {
		return State{}, err
	}
	configuration, err := renderConsoleProfile(profile, "")
	if err != nil {
		return State{}, err
	}
	home, err := d.prepareHomeState(
		configuration, filepath.Join(".aws", "login", "cache"), consoleCacheNamePattern, nil,
	)
	if err != nil {
		return State{}, ErrCommandFailed
	}
	defer func() {
		if err := d.cleanupHome(home); err != nil {
			state.Clear()
			resultErr = ErrCommandFailed
		}
	}()
	if err := d.requireConsoleLoginVersion(ctx, canonical, home); err != nil {
		return State{}, err
	}

	visibleOutput := newVisibleLimiter(maxVisibleOutputBytes, visible)
	command := Command{
		Path: canonical,
		Args: []string{
			"login", "--remote",
			"--profile", fixedProfileName,
			"--region", profile.Region,
			"--no-cli-pager",
			"--no-cli-auto-prompt",
		},
		Env:    consoleCommandEnvironment(home),
		Dir:    home,
		Stdin:  input,
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
	updatedConfiguration, err := readPrivateHomeFile(home, filepath.Join(".aws", "config"), 8*1024)
	if err != nil {
		return State{}, err
	}
	loginSession, accountID, err := parseConsoleProfile(updatedConfiguration, profile)
	clear(updatedConfiguration)
	if err != nil {
		return State{}, err
	}
	cache, err := packCacheWithPattern(consoleCachePath(home), consoleCacheNamePattern)
	if err != nil {
		return State{}, err
	}
	state, err = newConsoleState(profile, loginSession, accountID, canonical, digest, cache)
	if err != nil {
		return State{}, ErrInvalidState
	}
	return state, nil
}

func (d *Driver) requireConsoleLoginVersion(ctx context.Context, executable, home string) error {
	bounded, cancel := context.WithTimeout(ctx, consoleVersionTimeout)
	defer cancel()
	stdout := newBoundedBuffer(1024)
	stderr := newBoundedBuffer(1024)
	err := d.runner.Run(bounded, Command{
		Path: executable, Args: []string{"--version"}, Env: consoleCommandEnvironment(home), Dir: home,
		Stdout: stdout, Stderr: stderr,
	})
	if bounded.Err() != nil {
		return bounded.Err()
	}
	if err != nil || stdout.err() != nil || stderr.err() != nil {
		return ErrConsoleLoginUnsupported
	}
	version := append(append([]byte(nil), stdout.bytes()...), ' ')
	version = bytes.TrimSpace(append(version, stderr.bytes()...))
	matches := awsCLIVersionPattern.FindSubmatch(version)
	if len(matches) != 3 {
		return ErrConsoleLoginUnsupported
	}
	major, majorErr := strconv.Atoi(string(matches[1]))
	minor, minorErr := strconv.Atoi(string(matches[2]))
	if majorErr != nil || minorErr != nil || major < minimumConsoleMajor ||
		(major == minimumConsoleMajor && minor < minimumConsoleMinor) {
		return ErrConsoleLoginUnsupported
	}
	return nil
}

func (d *Driver) refreshConsole(ctx context.Context, state State) (
	credentials TemporaryCredentials, updated State, resultErr error,
) {
	if ctx == nil {
		return TemporaryCredentials{}, State{}, ErrCommandFailed
	}
	if err := validateConsoleStatePayload(state.console); err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	boundedContext, cancel := context.WithTimeout(ctx, refreshTimeout)
	defer cancel()

	profile := ConsoleProfileConfig{Region: state.console.Profile.Region}
	configuration, err := renderConsoleProfile(profile, state.console.Profile.LoginSession)
	if err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	home, err := d.prepareHomeState(
		configuration, filepath.Join(".aws", "login", "cache"), consoleCacheNamePattern, state.console.Cache,
	)
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
	if err := verifyExecutable(state.console.Executable.Path, state.console.Executable.SHA256); err != nil {
		return TemporaryCredentials{}, State{}, err
	}

	stdout := newBoundedBuffer(maxProcessStdoutBytes)
	stderr := newBoundedBuffer(maxProcessStderrBytes)
	command := Command{
		Path: state.console.Executable.Path,
		Args: []string{
			"configure", "export-credentials",
			"--profile", fixedProfileName,
			"--format", "process",
			"--no-cli-pager",
			"--cli-connect-timeout", "10",
			"--cli-read-timeout", "30",
		},
		Env: consoleCommandEnvironment(home), Dir: home, Stdout: stdout, Stderr: stderr,
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
	if err := verifyExecutable(state.console.Executable.Path, state.console.Executable.SHA256); err != nil {
		return TemporaryCredentials{}, State{}, err
	}
	cache, err := packCacheWithPattern(consoleCachePath(home), consoleCacheNamePattern)
	if err != nil {
		return TemporaryCredentials{}, State{}, err
	}
	updated, err = newConsoleState(
		profile, state.console.Profile.LoginSession, state.console.Profile.AccountID,
		state.console.Executable.Path, state.console.Executable.SHA256, cache,
	)
	if err != nil {
		return TemporaryCredentials{}, State{}, ErrInvalidState
	}
	return credentials, updated, nil
}

func consoleCachePath(home string) string {
	return filepath.Join(home, ".aws", "login", "cache")
}

func consoleCommandEnvironment(home string) []string {
	return append(commandEnvironment(home), "AWS_LOGIN_CACHE_DIRECTORY="+consoleCachePath(home))
}

func readPrivateHomeFile(home, relative string, limit int64) ([]byte, error) {
	if home == "" || relative == "" || limit <= 0 {
		return nil, ErrInvalidState
	}
	root, err := os.OpenRoot(home)
	if err != nil {
		return nil, ErrInvalidState
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() <= 0 || info.Size() > limit {
		return nil, ErrInvalidState
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, ErrInvalidState
	}
	opened, statErr := file.Stat()
	content, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) ||
		readErr != nil || closeErr != nil || int64(len(content)) != info.Size() {
		clear(content)
		return nil, ErrInvalidState
	}
	return content, nil
}
