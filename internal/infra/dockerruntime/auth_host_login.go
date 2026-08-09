package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const (
	awsHostDriverID      = "aws_cli_sso"
	maxAWSLoginFieldSize = 1024
)

var (
	hostDriverRevisionPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	hostAWSAccountPattern     = regexp.MustCompile(`^[0-9]{12}$`)

	errHostLoginPrompt      = errors.New("host credential login prompt failed")
	errHostCredentialResult = errors.New("host credential result is invalid")
)

type hostCLIUnavailableError struct{ provider string }

func (hostCLIUnavailableError) Error() string { return "trusted host provider CLI is unavailable" }

type hostCLIResolver interface {
	Resolve(string) (string, error)
}

type pathHostCLIResolver struct {
	lookPath func(string) (string, error)
}

func newPathHostCLIResolver() pathHostCLIResolver {
	return pathHostCLIResolver{lookPath: exec.LookPath}
}

// Resolve accepts only the two source-reviewed driver names and an absolute,
// canonical, non-writable executable under a conventional host installation
// root. PATH may select among those installations, but a relative, project,
// home-local, or temporary directory cannot become provider authority.
func (r pathHostCLIResolver) Resolve(name string) (string, error) {
	if r.lookPath == nil || (name != "gh" && name != "aws") {
		return "", hostCLIUnavailableError{provider: name}
	}
	selected, err := r.lookPath(name)
	if err != nil || !filepath.IsAbs(selected) || filepath.Base(selected) != name {
		return "", hostCLIUnavailableError{provider: name}
	}
	canonical, err := filepath.EvalSymlinks(selected)
	if err != nil || !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical ||
		!trustedHostCLIPath(canonical) {
		return "", hostCLIUnavailableError{provider: name}
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return "", hostCLIUnavailableError{provider: name}
	}
	return canonical, nil
}

// trustedHostCLIPath limits reviewed provider drivers to conventional
// installation roots outside project and temporary directories. PATH still
// selects among installations, but cannot make a repository-owned absolute
// directory executable authority.
func trustedHostCLIPath(path string) bool {
	for _, root := range []string{
		"/bin", "/usr/bin", "/usr/local", "/opt/homebrew", "/opt/local",
		"/nix/store", "/snap",
	} {
		relative, err := filepath.Rel(root, path)
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type hostCredentialPayload struct {
	secret         []byte
	accountLabel   string
	driverID       string
	driverRevision string
}

func (hostCredentialPayload) String() string {
	return "dockerruntime.hostCredentialPayload{redacted}"
}

func (hostCredentialPayload) GoString() string {
	return "dockerruntime.hostCredentialPayload{redacted}"
}

func (p *hostCredentialPayload) clear() {
	if p == nil {
		return
	}
	clear(p.secret)
	p.secret = nil
	p.accountLabel = ""
	p.driverID = ""
	p.driverRevision = ""
}

type hostCredentialAcquirer interface {
	LoginGitHub(
		context.Context,
		string,
		credentialhost.GitHubLoginStreams,
	) (hostCredentialPayload, error)
	LoginAWS(
		context.Context,
		string,
		credentialhost.ProfileConfig,
		credentialhost.VisibleOutput,
	) (hostCredentialPayload, error)
}

// hostLoginProfileReader owns interactive, context-bounded collection of the
// non-secret AWS profile fields. Keeping this terminal capability narrower
// than the credential acquirer prevents the provider driver and Broker commit
// from starting before prompt input has completed.
type hostLoginProfileReader interface {
	ReadAWSProfile(
		context.Context,
		io.Reader,
		io.Writer,
	) (credentialhost.ProfileConfig, error)
}

type osHostLoginProfileReader struct {
	waitInput func(context.Context, io.Reader) error
	openInput func(io.Reader) (*os.File, error)
}

func (r osHostLoginProfileReader) ReadAWSProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ProfileConfig, error) {
	if ctx == nil {
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	if err := ctx.Err(); err != nil {
		return credentialhost.ProfileConfig{}, err
	}
	if !terminal.IsCanonical(input) {
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	openInput := r.openInput
	if openInput == nil {
		openInput = openHostLoginInput
	}
	privateInput, err := openInput(input)
	if err != nil || !terminal.IsCanonical(privateInput) {
		if privateInput != nil {
			_ = privateInput.Close()
		}
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	waitInput := r.waitInput
	if waitInput == nil {
		waitInput = waitHostLoginInput
	}
	profile, readErr := readAWSLoginProfile(
		ctx, privateInput, output,
		func(ctx context.Context, input io.Reader) error {
			if err := waitInput(ctx, input); err != nil {
				return err
			}
			if !terminal.IsCanonical(input) {
				return errHostLoginPrompt
			}
			return nil
		},
		readHostLoginInput,
	)
	closeErr := privateInput.Close()
	if closeErr != nil {
		if readErr != nil {
			return credentialhost.ProfileConfig{}, errors.Join(readErr, errHostLoginPrompt)
		}
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	return profile, readErr
}

type osHostCredentialAcquirer struct {
	github *credentialhost.GitHubDriver
	aws    *credentialhost.Driver
}

func newOSHostCredentialAcquirer() *osHostCredentialAcquirer {
	return &osHostCredentialAcquirer{
		github: credentialhost.NewGitHubDriver(nil),
		aws:    credentialhost.NewDriver(nil),
	}
}

func (a *osHostCredentialAcquirer) LoginGitHub(
	ctx context.Context,
	executable string,
	streams credentialhost.GitHubLoginStreams,
) (hostCredentialPayload, error) {
	if a == nil || a.github == nil {
		return hostCredentialPayload{}, credentialhost.ErrGitHubLoginSetup
	}
	credential, err := a.github.Login(ctx, executable, streams)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer credential.Clear()
	return hostCredentialPayload{
		secret:       credential.Token(),
		accountLabel: credential.AccountLabel(),
	}, nil
}

func (a *osHostCredentialAcquirer) LoginAWS(
	ctx context.Context,
	executable string,
	profile credentialhost.ProfileConfig,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	if a == nil || a.aws == nil {
		return hostCredentialPayload{}, credentialhost.ErrCommandFailed
	}
	state, err := a.aws.Login(ctx, executable, profile, visible)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer state.Clear()
	encoded, err := state.Encode()
	if err != nil {
		return hostCredentialPayload{}, err
	}
	return hostCredentialPayload{
		secret:         encoded,
		accountLabel:   profile.AccountID,
		driverID:       awsHostDriverID,
		driverRevision: state.DriverRevision(),
	}, nil
}

func (r *Runtime) runHostCredentialLogin(
	ctx context.Context,
	contextID string,
	provider string,
	input io.Reader,
	errOut io.Writer,
) (brokerControlResponse, error) {
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return brokerControlResponse{}, authLoginTerminalRequiredFault()
	}
	return r.runHostCredentialLoginOnTTY(ctx, contextID, provider, input, errOut)
}

func (r *Runtime) runHostCredentialLoginOnTTY(
	ctx context.Context,
	contextID string,
	provider string,
	input io.Reader,
	errOut io.Writer,
) (brokerControlResponse, error) {
	if r.hostCLIs == nil || r.credentialHost == nil {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
	}

	loginContext, cancel := context.WithTimeout(ctx, brokerLoginTimeout)
	defer cancel()
	executableName := providerHostExecutable(provider)
	executable, err := r.hostCLIs.Resolve(executableName)
	if err != nil {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
	}

	visible := &loginVisibleOutput{
		destination: errOut,
		openBrowser: r.loginBrowserOpener(loginContext),
	}
	var payload hostCredentialPayload
	switch provider {
	case "github":
		payload, err = r.credentialHost.LoginGitHub(
			loginContext,
			executable,
			credentialhost.GitHubLoginStreams{
				Stdin: input, Stdout: visible, Stderr: visible,
			},
		)
	case "aws":
		var profile credentialhost.ProfileConfig
		if r.hostLoginProfiles == nil {
			return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
		}
		profile, err = r.hostLoginProfiles.ReadAWSProfile(loginContext, input, errOut)
		if err == nil {
			err = loginContext.Err()
		}
		if err == nil {
			payload, err = r.credentialHost.LoginAWS(
				loginContext,
				executable,
				profile,
				func(_ credentialhost.OutputStream, content []byte) error {
					_, writeErr := visible.Write(content)
					return writeErr
				},
			)
		}
	default:
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
	}
	flushErr := visible.flush()
	if err != nil {
		payload.clear()
		return brokerControlResponse{}, err
	}
	if flushErr != nil {
		payload.clear()
		return brokerControlResponse{}, flushErr
	}
	defer payload.clear()
	if err := validateHostCredentialPayload(provider, payload); err != nil {
		return brokerControlResponse{}, err
	}

	arguments := []string{
		"login",
		"--context-id", contextID,
		"--provider", provider,
		"--account-label", payload.accountLabel,
	}
	if provider == "aws" {
		arguments = append(
			arguments,
			"--driver-id", payload.driverID,
			"--driver-revision", payload.driverRevision,
		)
	}
	return r.runBrokerControl(loginContext, bytes.NewReader(payload.secret), arguments...)
}

func providerHostExecutable(provider string) string {
	switch provider {
	case "github":
		return "gh"
	case "aws":
		return "aws"
	default:
		return ""
	}
}

func (r *Runtime) loginBrowserOpener(ctx context.Context) func(string) error {
	opener := r.browser
	if opener == nil {
		opener = osHostBrowserOpener{}
	}
	return func(target string) error {
		openContext, cancel := context.WithTimeout(ctx, hostBrowserOpenTimeout)
		defer cancel()
		return opener.Open(openContext, target)
	}
}

func readAWSLoginProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	waitInput func(context.Context, io.Reader) error,
	readInput func(io.Reader, []byte) (int, error),
) (credentialhost.ProfileConfig, error) {
	if ctx == nil || input == nil || output == nil || waitInput == nil || readInput == nil {
		return credentialhost.ProfileConfig{}, errHostLoginPrompt
	}
	const maxBufferedProfileSize = 4 * (maxAWSLoginFieldSize + 2)
	pending := make([]byte, 0, maxBufferedProfileSize)
	available := make([]byte, maxBufferedProfileSize)
	inputEnded := false
	values := make([]string, 0, 4)
	for _, prompt := range []string{
		"AWS IAM Identity Center access portal URL",
		"AWS IAM Identity Center region",
		"AWS account ID",
		"AWS role name",
	} {
		if err := ctx.Err(); err != nil {
			return credentialhost.ProfileConfig{}, err
		}
		if _, err := fmt.Fprintf(output, "%s: ", prompt); err != nil {
			return credentialhost.ProfileConfig{}, errHostLoginPrompt
		}
		for bytes.IndexByte(pending, '\n') < 0 {
			if len(pending) >= maxAWSLoginFieldSize+2 || inputEnded {
				return credentialhost.ProfileConfig{}, errHostLoginPrompt
			}
			if err := waitInput(ctx, input); err != nil {
				if contextErr := ctx.Err(); contextErr != nil {
					return credentialhost.ProfileConfig{}, contextErr
				}
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return credentialhost.ProfileConfig{}, err
				}
				return credentialhost.ProfileConfig{}, errHostLoginPrompt
			}
			if err := ctx.Err(); err != nil {
				return credentialhost.ProfileConfig{}, err
			}
			// Production readInput is one nonblocking read on the private
			// terminal description. A readiness flush or partial line therefore
			// returns to this bounded loop instead of entering a hidden second
			// read.
			count, readErr := readInput(input, available[:maxBufferedProfileSize-len(pending)])
			if err := ctx.Err(); err != nil {
				return credentialhost.ProfileConfig{}, err
			}
			if count > 0 {
				pending = append(pending, available[:count]...)
			}
			if readErr != nil {
				if errors.Is(readErr, syscall.EAGAIN) || errors.Is(readErr, syscall.EWOULDBLOCK) {
					continue
				}
				if !errors.Is(readErr, io.EOF) {
					return credentialhost.ProfileConfig{}, errHostLoginPrompt
				}
				inputEnded = true
			}
			if count == 0 && readErr == nil {
				return credentialhost.ProfileConfig{}, errHostLoginPrompt
			}
		}
		lineEnd := bytes.IndexByte(pending, '\n')
		line := pending[:lineEnd+1]
		if len(line) > maxAWSLoginFieldSize+2 {
			return credentialhost.ProfileConfig{}, errHostLoginPrompt
		}
		pending = pending[lineEnd+1:]
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		if len(line) == 0 || len(line) > maxAWSLoginFieldSize || bytes.IndexByte(line, 0) >= 0 {
			return credentialhost.ProfileConfig{}, credentialhost.ErrInvalidProfile
		}
		values = append(values, string(line))
	}
	if err := ctx.Err(); err != nil {
		return credentialhost.ProfileConfig{}, err
	}
	return credentialhost.ProfileConfig{
		StartURL: values[0], SSORegion: values[1],
		AccountID: values[2], RoleName: values[3],
	}, nil
}

func validateHostCredentialPayload(provider string, payload hostCredentialPayload) error {
	if len(payload.secret) == 0 || len(payload.secret) > 32*1024 ||
		strings.IndexByte(payload.accountLabel, 0) >= 0 {
		return errHostCredentialResult
	}
	switch provider {
	case "github":
		if payload.accountLabel == "" || payload.driverID != "" || payload.driverRevision != "" {
			return errHostCredentialResult
		}
	case "aws":
		if !hostAWSAccountPattern.MatchString(payload.accountLabel) ||
			payload.driverID != awsHostDriverID ||
			!hostDriverRevisionPattern.MatchString(payload.driverRevision) {
			return errHostCredentialResult
		}
	default:
		return errHostCredentialResult
	}
	return nil
}

func hostLoginTimedOut(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded)
}

func hostLoginCancelled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, credentialhost.ErrGitHubLoginCancelled)
}

func hostLoginFailureIsCredentialDriver(err error) bool {
	return errors.Is(err, credentialhost.ErrGitHubExecutable) ||
		errors.Is(err, credentialhost.ErrGitHubLoginSetup) ||
		errors.Is(err, credentialhost.ErrGitHubTTYRequired) ||
		errors.Is(err, credentialhost.ErrGitHubLoginFailed) ||
		errors.Is(err, credentialhost.ErrGitHubAccountCapture) ||
		errors.Is(err, credentialhost.ErrGitHubTokenCapture) ||
		errors.Is(err, credentialhost.ErrGitHubOutputLimit) ||
		errors.Is(err, credentialhost.ErrGitHubLoginCleanup) ||
		errors.Is(err, credentialhost.ErrCommandFailed) ||
		errors.Is(err, credentialhost.ErrOutputLimit) ||
		errors.Is(err, credentialhost.ErrVisibleOutput) ||
		errors.Is(err, credentialhost.ErrInvalidCredentials) ||
		errors.Is(err, credentialhost.ErrInvalidExecutable) ||
		errors.Is(err, credentialhost.ErrInvalidState) ||
		errors.Is(err, credentialhost.ErrInvalidCache) ||
		errors.Is(err, errLoginVisibleOutputLimit) ||
		errors.Is(err, errHostLoginPrompt) ||
		errors.Is(err, errHostCredentialResult)
}
