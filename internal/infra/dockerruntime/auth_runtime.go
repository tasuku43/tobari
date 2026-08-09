package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	githubEphemeralPlaintextWarning = "! Authentication credentials saved in plain text"
	githubManualBrowserFallback     = "The host browser did not open; visit " + githubDeviceURL + " manually to continue.\n"
	awsBrowserLinePrefix            = "Open "
	maxLoginVisibleLine             = 64 * 1024
	maxLoginVisibleBytes            = 64 * 1024
	hostBrowserOpenTimeout          = 5 * time.Second
)

var errLoginVisibleOutputLimit = errors.New("host login visible output exceeded its limit")

type loginVisibleOutput struct {
	mu            sync.Mutex
	destination   io.Writer
	openBrowser   func(string) error
	consoleRegion string
	pending       []byte
	opened        bool
	written       int
	visible       int
	failure       error
}

func (w *loginVisibleOutput) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != nil {
		return 0, w.failure
	}
	remaining := maxLoginVisibleBytes - w.written
	if remaining <= 0 {
		w.failure = errLoginVisibleOutputLimit
		return 0, w.failure
	}
	if len(data) > remaining {
		written, err := w.write(data[:remaining])
		w.written += written
		if err != nil {
			w.failure = err
			return written, err
		}
		w.failure = errLoginVisibleOutputLimit
		return written, w.failure
	}
	written, err := w.write(data)
	w.written += written
	if err != nil {
		w.failure = err
	}
	return written, err
}

func (w *loginVisibleOutput) write(data []byte) (int, error) {
	for index, value := range data {
		w.pending = append(w.pending, value)
		if len(w.pending) > maxLoginVisibleLine {
			w.pending = w.pending[:len(w.pending)-1]
			return index, errLoginVisibleOutputLimit
		}
		if value == '\n' {
			if err := w.flushPending(); err != nil {
				return index + 1, err
			}
		}
	}
	return len(data), nil
}

func (w *loginVisibleOutput) flush() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failure != nil {
		return w.failure
	}
	if err := w.flushPending(); err != nil {
		w.failure = err
		return err
	}
	return nil
}

func (w *loginVisibleOutput) flushPending() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := append([]byte(nil), w.pending...)
	w.pending = nil
	hasNewline := bytes.HasSuffix(line, []byte{'\n'})
	if hasNewline {
		line = line[:len(line)-1]
	}
	line = bytes.TrimSuffix(line, []byte{'\r'})
	normalized := string(line)
	if normalized == githubEphemeralPlaintextWarning {
		return nil
	}
	visible := projectLoginVisibleText(normalized)
	if hasNewline {
		visible += "\n"
	}
	if err := w.writeVisible(visible); err != nil {
		return err
	}
	target, recognized := loginBrowserTarget(normalized, w.consoleRegion)
	if !w.opened && recognized && w.openBrowser != nil {
		w.opened = true
		if err := w.openBrowser(target); err != nil {
			return w.writeVisible(manualBrowserFallback(target))
		}
	}
	return nil
}

func (w *loginVisibleOutput) writeVisible(value string) error {
	if len(value) > maxLoginVisibleBytes-w.visible {
		return errLoginVisibleOutputLimit
	}
	written, err := io.WriteString(w.destination, value)
	w.visible += written
	if err != nil {
		return err
	}
	if written != len(value) {
		return io.ErrShortWrite
	}
	return nil
}

func projectLoginVisibleText(value string) string {
	var output strings.Builder
	for _, character := range value {
		switch {
		case character == '\\':
			output.WriteString(`\\`)
		case character == '\u2028' || character == '\u2029' || unicode.Is(unicode.C, character):
			if character <= 0xffff {
				_, _ = fmt.Fprintf(&output, `\u%04X`, character)
			} else {
				_, _ = fmt.Fprintf(&output, `\U%08X`, character)
			}
		default:
			output.WriteRune(character)
		}
	}
	return output.String()
}

func loginBrowserTarget(line, consoleRegion string) (string, bool) {
	if strings.Contains(line, githubDeviceURL) {
		return githubDeviceURL, true
	}
	if awsSSODeviceURLPattern.MatchString(line) {
		return line, true
	}
	if consoleRegion != "" && validAWSConsoleAuthorizationURL(line, consoleRegion) {
		return line, true
	}
	if !strings.HasPrefix(line, awsBrowserLinePrefix) {
		return "", false
	}
	target := strings.TrimPrefix(line, awsBrowserLinePrefix)
	if !awsSSODeviceURLPattern.MatchString(target) {
		return "", false
	}
	return target, true
}

func manualBrowserFallback(target string) string {
	if target == githubDeviceURL {
		return githubManualBrowserFallback
	}
	return fmt.Sprintf("The host browser did not open; visit %s manually to continue.\n", target)
}

func (r *Runtime) LoginAuth(
	ctx context.Context, contextName, providerID, method string, input io.Reader, errOut io.Writer,
) (authbroker.Result, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.Result{}, err
	}
	if !supportsBuiltinAuthHelper(provider) {
		return authbroker.Result{}, fault.New(
			fault.KindUnsupported, "provider_login_unsupported",
			"The selected provider does not support interactive login.", false,
			fault.NextAction{Command: "help auth import", Reason: "Use protected stdin import for a compatible user provider."},
		)
	}
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return authbroker.Result{}, authLoginTerminalRequiredFault()
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.Result{}, err
	}
	response, err := r.runHostCredentialLogin(ctx, manifest.ID, provider.ID, input, errOut, method)
	if err != nil {
		return authbroker.Result{}, classifyHostLoginError(err, provider.ID, method)
	}
	return buildAuthResult(authbroker.TaskLogin, manifest.Name, manifest.ID, provider.ID, response, true)
}

func authLoginTerminalRequiredFault() error {
	return fault.New(
		fault.KindInvalidInput,
		"auth_login_tty_required",
		"Built-in provider login requires interactive terminal streams on stdin and stderr.",
		false,
		fault.NextAction{Command: "help auth login", Reason: "Run trusted-host provider login from an interactive terminal."},
	)
}

func supportsBuiltinAuthHelper(provider authbroker.Provider) bool {
	if provider.Acquisition.Mode != authbroker.AcquisitionBuiltinHelper {
		return false
	}
	return (provider.ID == "github" && provider.Acquisition.Helper == "github-gh") ||
		(provider.ID == "aws" && provider.Acquisition.Helper == "aws-sso") ||
		(provider.ID == "datadog" && provider.Acquisition.Helper == "pup-oauth")
}

func classifyHostLoginError(err error, provider string, methods ...string) error {
	method := ""
	if len(methods) == 1 {
		method = methods[0]
	}
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}
	var unavailable hostCLIUnavailableError
	if errors.As(err, &unavailable) {
		code := provider + "_cli_unavailable"
		name := provider
		if provider == "github" {
			name = "GitHub"
		} else if provider == "aws" {
			name = "AWS"
		} else if provider == "datadog" {
			name = "Datadog pup"
		}
		return fault.New(
			fault.KindUnavailable, code,
			"The trusted-host "+name+" CLI is unavailable; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Install the reviewed host CLI and retry this login."},
		)
	}
	if provider == "github" {
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "github_login_cancelled",
				"GitHub login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host GitHub login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrGitHubExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindRejected, "github_login_failed",
				"GitHub login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host GitHub login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login github")
	}
	if provider == "datadog" {
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "datadog_login_timeout",
				"The bounded Datadog OAuth login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new Datadog login and complete browser consent within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "datadog_login_cancelled",
				"Datadog OAuth login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host Datadog login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "datadog_login_failed",
				"Datadog OAuth login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the isolated trusted-host pup login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login datadog")
	}
	if provider != "aws" {
		return classifyBrokerError(err, "auth login "+provider)
	}
	if method == awsConsoleMethod {
		if errors.Is(err, credentialhost.ErrConsoleLoginUnsupported) {
			return fault.New(
				fault.KindUnsupported, "aws_console_login_unsupported",
				"The trusted-host AWS CLI does not support console-based login; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Install AWS CLI 2.32 or newer on the trusted host, then retry console login."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidProfile) {
			return fault.New(
				fault.KindInvalidInput, "aws_console_config_invalid",
				"The AWS console login configuration is invalid; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "help auth login", Reason: "Provide a valid commercial AWS region for console login."},
			)
		}
		if hostLoginTimedOut(err) {
			return fault.New(
				fault.KindRejected, "aws_console_login_timeout",
				"The bounded AWS console login timed out; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Start a new AWS console login and complete it within the bounded window."},
			)
		}
		if hostLoginCancelled(err) {
			return fault.New(
				fault.KindRejected, "aws_console_login_cancelled",
				"AWS console login was cancelled; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS console login when ready."},
			)
		}
		if errors.Is(err, credentialhost.ErrInvalidExecutable) {
			return classifyHostLoginError(hostCLIUnavailableError{provider: provider}, provider, method)
		}
		if hostLoginFailureIsCredentialDriver(err) {
			return fault.New(
				fault.KindUnavailable, "aws_console_login_failed",
				"AWS console login did not complete; the previous Context credential remains unchanged.", false,
				fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS console login after inspecting the failure."},
			)
		}
		return classifyBrokerError(err, "auth login aws")
	}
	if errors.Is(err, credentialhost.ErrInvalidProfile) {
		return fault.New(
			fault.KindInvalidInput, "aws_sso_config_invalid",
			"The AWS IAM Identity Center login configuration is invalid; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "help auth login", Reason: "Provide valid AWS IAM Identity Center login fields."},
		)
	}
	if hostLoginTimedOut(err) {
		return fault.New(
			fault.KindRejected, "aws_sso_login_timeout",
			"The bounded AWS IAM Identity Center device login timed out; the previous Context credential remains unchanged.",
			false,
			fault.NextAction{Command: "auth login", Reason: "Start a new AWS IAM Identity Center login and complete it within the bounded window."},
		)
	}
	if hostLoginCancelled(err) {
		return fault.New(
			fault.KindRejected, "aws_sso_login_cancelled",
			"AWS IAM Identity Center login was cancelled; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS IAM Identity Center login when ready."},
		)
	}
	if errors.Is(err, credentialhost.ErrInvalidExecutable) {
		return classifyHostLoginError(hostCLIUnavailableError{provider: provider}, provider, method)
	}
	if hostLoginFailureIsCredentialDriver(err) {
		return fault.New(
			fault.KindUnavailable, "aws_sso_login_failed",
			"AWS IAM Identity Center login did not complete; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host AWS IAM Identity Center login after inspecting the failure."},
		)
	}
	return classifyBrokerError(err, "auth login aws")
}

func (r *Runtime) ImportAuth(
	ctx context.Context, contextName, providerID string, secret io.Reader,
) (authbroker.Result, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.Result{}, err
	}
	if provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
		return authbroker.Result{}, fault.New(
			fault.KindUnsupported, "provider_import_unsupported",
			"The selected provider does not support credential import.", false,
			fault.NextAction{Command: "auth login", Reason: "Use the provider's reviewed built-in acquisition helper."},
		)
	}
	if secret == nil {
		return authbroker.Result{}, fault.New(fault.KindInvalidInput, "invalid_credential_input", "Credential stdin is unavailable.", false)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.Result{}, err
	}
	response, err := r.runBrokerControl(
		ctx, secret, "import", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.Result{}, classifyBrokerError(err, "auth import "+provider.ID)
	}
	return buildAuthResult(authbroker.TaskImport, manifest.Name, manifest.ID, provider.ID, response, true)
}

func (r *Runtime) AuthStatus(ctx context.Context, contextName string) (authbroker.StatusResult, error) {
	manifest, err := r.resolveAuthContext(ctx, contextName)
	if err != nil {
		return authbroker.StatusResult{}, err
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.StatusResult{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.StatusResult{}, classifyRootKeyError(err)
	}
	result := authbroker.StatusResult{
		Task: authbroker.TaskStatus, Context: manifest.Name, ContextID: manifest.ID,
		StorageBackend: backend, BrokerState: authbroker.BrokerStateUnavailable,
		Providers:           []authbroker.ProviderStatus{},
		WorkspaceActivation: authbroker.WorkspaceActivation{State: authbroker.WorkspaceActivationNotApplicable},
	}
	for _, provider := range projection.Providers {
		result.Providers = append(result.Providers, authbroker.ProviderStatus{
			Provider: provider.ID,
			State:    authbroker.ProviderCredentialUnavailable,
		})
	}
	sort.Slice(result.Providers, func(left, right int) bool {
		return result.Providers[left].Provider < result.Providers[right].Provider
	})
	validate := func() (authbroker.StatusResult, error) {
		if err := result.Validate(); err != nil {
			return authbroker.StatusResult{}, fmt.Errorf("Auth Broker status result is invalid: %w", err)
		}
		return result, nil
	}
	if _, configured, stateErr := r.LoadState(ctx); stateErr != nil {
		return authbroker.StatusResult{}, stateErr
	} else if !configured {
		return validate()
	}
	state, err := r.brokerState(ctx)
	if err != nil || state == authbroker.BrokerStateUnavailable {
		return validate()
	}
	result.BrokerState = state
	configured := false
	for index := range result.Providers {
		status := result.Providers[index]
		if state == authbroker.BrokerStateReady {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", manifest.ID, "--provider", status.Provider,
			)
			if statusErr != nil {
				return authbroker.StatusResult{}, classifyBrokerError(statusErr, "auth status")
			}
			status.Configured = response.State == "ready"
			if status.Configured {
				configured = true
				status.State = authbroker.ProviderCredentialConfigured
				status.CredentialRevision = response.Revision
				status.AccountLabel, err = validatedAccountLabel(response.AccountLabel)
				if err != nil {
					return authbroker.StatusResult{}, err
				}
			} else {
				status.State = authbroker.ProviderCredentialNotConfigured
			}
		}
		result.Providers[index] = status
	}
	if configured {
		result.WorkspaceActivation = authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: authbroker.ContextAuthActivationGuidance,
		}
	}
	return validate()
}

func (r *Runtime) LogoutAuth(
	ctx context.Context, contextName, providerID string,
) (authbroker.Result, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.Result{}, err
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.Result{}, err
	}
	response, err := r.runBrokerControl(
		ctx, nil, "logout", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.Result{}, classifyBrokerError(err, "auth logout")
	}
	return buildAuthResult(authbroker.TaskLogout, manifest.Name, manifest.ID, provider.ID, response, false)
}

func (r *Runtime) authOperationTarget(
	ctx context.Context, contextName, providerID string,
) (manifestResult struct {
	ID   string
	Name string
}, provider authbroker.Provider, err error) {
	manifest, err := r.resolveAuthContext(ctx, contextName)
	if err != nil {
		return manifestResult, authbroker.Provider{}, err
	}
	manifestResult.ID, manifestResult.Name = manifest.ID, manifest.Name
	projection, err := r.loadAuthProviders()
	if err != nil {
		return manifestResult, authbroker.Provider{}, err
	}
	provider, found := findAuthProvider(projection, providerID)
	if !found {
		return manifestResult, authbroker.Provider{}, fault.New(
			fault.KindNotFound, "provider_not_installed", "The credential provider is not installed.", false,
			fault.NextAction{Command: "auth status", Reason: "Inspect the installed built-in and user provider collection."},
		)
	}
	return manifestResult, provider, nil
}

func buildAuthResult(
	task, contextName, contextID, provider string,
	response brokerControlResponse,
	configured bool,
) (authbroker.Result, error) {
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.Result{}, classifyRootKeyError(err)
	}
	guidance := authbroker.ContextAuthActivationGuidance
	if !configured {
		guidance = authbroker.ContextAuthRemovalGuidance
	}
	result := authbroker.Result{
		Task: task, Provider: provider, Context: contextName, ContextID: contextID,
		Configured: configured, StorageBackend: backend, BrokerState: authbroker.BrokerStateReady,
		WorkspaceActivation: authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: guidance,
		},
	}
	if configured {
		result.CredentialRevision = response.Revision
		result.AccountLabel, err = validatedAccountLabel(response.AccountLabel)
		if err != nil {
			return authbroker.Result{}, err
		}
	}
	if err := result.Validate(); err != nil {
		return authbroker.Result{}, fmt.Errorf("Auth Broker mutation result is invalid: %w", err)
	}
	return result, nil
}

func validatedAccountLabel(label *string) (*string, error) {
	if label == nil || *label == "" {
		return nil, nil
	}
	value := *label
	if authbroker.ValidateSecretFreeText("account label", value, 128) != nil {
		return nil, fault.New(
			fault.KindContract, "invalid_auth_broker_metadata",
			"The Auth Broker returned invalid non-secret account metadata.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Auth Broker and provider helper contract."},
		)
	}
	return &value, nil
}
