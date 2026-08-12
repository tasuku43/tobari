package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

var errHostCredentialResult = errors.New("host credential result is invalid")

type hostCLIUnavailableError struct{ provider string }

func (hostCLIUnavailableError) Error() string { return "trusted host provider CLI is unavailable" }

type hostCLIResolver interface{ Resolve(string) (string, error) }
type pathHostCLIResolver struct{ lookPath func(string) (string, error) }

func newPathHostCLIResolver() pathHostCLIResolver {
	return pathHostCLIResolver{lookPath: exec.LookPath}
}

func (r pathHostCLIResolver) Resolve(name string) (string, error) {
	if r.lookPath == nil || name != "gh" {
		return "", hostCLIUnavailableError{provider: name}
	}
	selected, err := r.lookPath(name)
	if err != nil || !filepath.IsAbs(selected) || filepath.Base(selected) != name {
		return "", hostCLIUnavailableError{provider: name}
	}
	canonical, err := filepath.EvalSymlinks(selected)
	if err != nil || !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical || !trustedHostCLIPath(canonical) {
		return "", hostCLIUnavailableError{provider: name}
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 {
		return "", hostCLIUnavailableError{provider: name}
	}
	return canonical, nil
}

func trustedHostCLIPath(value string) bool {
	for _, root := range []string{"/bin", "/usr/bin", "/usr/local", "/opt/homebrew", "/opt/local", "/nix/store", "/snap"} {
		relative, err := filepath.Rel(root, value)
		if err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

type hostCredentialPayload struct {
	secret       []byte
	accountLabel string
}

func (hostCredentialPayload) String() string { return "dockerruntime.hostCredentialPayload{redacted}" }
func (hostCredentialPayload) GoString() string {
	return "dockerruntime.hostCredentialPayload{redacted}"
}
func (p *hostCredentialPayload) clear() {
	if p != nil {
		clear(p.secret)
		p.secret = nil
		p.accountLabel = ""
	}
}

type hostCredentialAcquirer interface {
	LoginGitHub(context.Context, string, credentialhost.GitHubLoginStreams) (hostCredentialPayload, error)
}
type osHostCredentialAcquirer struct{ github *credentialhost.GitHubDriver }

func newOSHostCredentialAcquirer() *osHostCredentialAcquirer {
	return &osHostCredentialAcquirer{github: credentialhost.NewGitHubDriver(nil)}
}
func (a *osHostCredentialAcquirer) LoginGitHub(ctx context.Context, executable string, streams credentialhost.GitHubLoginStreams) (hostCredentialPayload, error) {
	if a == nil || a.github == nil {
		return hostCredentialPayload{}, credentialhost.ErrGitHubLoginSetup
	}
	credential, err := a.github.Login(ctx, executable, streams)
	if err != nil {
		return hostCredentialPayload{}, err
	}
	defer credential.Clear()
	return hostCredentialPayload{secret: credential.Token(), accountLabel: credential.AccountLabel()}, nil
}

func (r *Runtime) runHostCredentialLogin(ctx context.Context, contextID, provider string, input io.Reader, errOut io.Writer, methods ...string) (brokerControlResponse, error) {
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return brokerControlResponse{}, authLoginTerminalRequiredFault()
	}
	return r.runHostCredentialLoginOnTTY(ctx, contextID, provider, input, errOut, methods...)
}

func (r *Runtime) runHostCredentialLoginOnTTY(ctx context.Context, contextID, provider string, input io.Reader, errOut io.Writer, methods ...string) (brokerControlResponse, error) {
	if provider != "github" || len(methods) > 1 || len(methods) == 1 && methods[0] != "" || r.hostCLIs == nil || r.credentialHost == nil {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
	}
	loginContext, cancel := context.WithTimeout(ctx, brokerLoginTimeout)
	defer cancel()
	executable, err := r.hostCLIs.Resolve("gh")
	if err != nil {
		return brokerControlResponse{}, hostCLIUnavailableError{provider: provider}
	}
	visible := &loginVisibleOutput{destination: errOut, openBrowser: r.loginBrowserOpener(loginContext)}
	payload, err := r.credentialHost.LoginGitHub(loginContext, executable, credentialhost.GitHubLoginStreams{Stdin: input, Stdout: visible, Stderr: visible})
	if err != nil {
		payload.clear()
		return brokerControlResponse{}, err
	}
	defer payload.clear()
	if err := visible.flush(); err != nil {
		return brokerControlResponse{}, err
	}
	if err := validateHostCredentialPayload(provider, payload); err != nil {
		return brokerControlResponse{}, err
	}
	return r.runBrokerControl(loginContext, bytes.NewReader(payload.secret), "login", "--context-id", contextID, "--provider", provider, "--account-label", payload.accountLabel)
}

func providerHostExecutable(provider string) string {
	if provider == "github" {
		return "gh"
	}
	return ""
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

func validateHostCredentialPayload(provider string, payload hostCredentialPayload) error {
	if provider != "github" || len(payload.secret) == 0 || len(payload.secret) > 32*1024 || payload.accountLabel == "" || strings.IndexByte(payload.accountLabel, 0) >= 0 {
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
	return errors.Is(err, credentialhost.ErrGitHubExecutable) || errors.Is(err, credentialhost.ErrGitHubLoginSetup) ||
		errors.Is(err, credentialhost.ErrGitHubTTYRequired) || errors.Is(err, credentialhost.ErrGitHubLoginFailed) ||
		errors.Is(err, credentialhost.ErrGitHubAccountCapture) || errors.Is(err, credentialhost.ErrGitHubTokenCapture) ||
		errors.Is(err, credentialhost.ErrGitHubOutputLimit) || errors.Is(err, credentialhost.ErrGitHubLoginCleanup) ||
		errors.Is(err, errLoginVisibleOutputLimit) || errors.Is(err, errHostCredentialResult)
}
