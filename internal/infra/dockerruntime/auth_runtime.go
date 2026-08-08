package dockerruntime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

const (
	loginResultPrefix               = "\x1eTOBARI_AUTH_BROKER_RESULT:"
	githubEphemeralPlaintextWarning = "! Authentication credentials saved in plain text"
	githubManualBrowserFallback     = "The host browser did not open; visit " + githubDeviceURL + " manually to continue.\n"
	maxLoginVisibleLine             = 64 * 1024
	hostBrowserOpenTimeout          = 5 * time.Second
)

type loginVisibleOutput struct {
	destination io.Writer
	openBrowser func(string) error
	pending     []byte
	overflow    bool
	opened      bool
}

func (w *loginVisibleOutput) Write(data []byte) (int, error) {
	for _, value := range data {
		if w.overflow {
			if _, err := w.destination.Write([]byte{value}); err != nil {
				return 0, err
			}
			if value == '\n' {
				w.overflow = false
			}
			continue
		}
		w.pending = append(w.pending, value)
		if len(w.pending) > maxLoginVisibleLine {
			if _, err := w.destination.Write(w.pending); err != nil {
				return 0, err
			}
			w.pending = nil
			w.overflow = value != '\n'
			continue
		}
		if value == '\n' {
			if err := w.flushPending(); err != nil {
				return 0, err
			}
		}
	}
	return len(data), nil
}

func (w *loginVisibleOutput) flushPending() error {
	if len(w.pending) == 0 {
		return nil
	}
	line := append([]byte(nil), w.pending...)
	w.pending = nil
	normalized := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
	if normalized == githubEphemeralPlaintextWarning {
		return nil
	}
	if _, err := w.destination.Write(line); err != nil {
		return err
	}
	if !w.opened && strings.Contains(normalized, githubDeviceURL) && w.openBrowser != nil {
		w.opened = true
		if err := w.openBrowser(githubDeviceURL); err != nil {
			_, writeErr := io.WriteString(w.destination, githubManualBrowserFallback)
			return writeErr
		}
	}
	return nil
}

// loginOutputFilter owns the fixed GitHub acquisition UX while withholding the
// broker's final machine response from public CLI output. The response prefix
// is emitted only by the trusted control helper.
type loginOutputFilter struct {
	mu             sync.Mutex
	destination    *loginVisibleOutput
	prefixPosition int
	prefixPending  []byte
	atLineStart    bool
	capturing      bool
	response       []byte
	responseCount  int
	overflow       bool
}

func newLoginOutputFilter(destination io.Writer, openBrowser func(string) error) *loginOutputFilter {
	return &loginOutputFilter{
		destination: &loginVisibleOutput{destination: destination, openBrowser: openBrowser},
		atLineStart: true,
	}
}

func (w *loginOutputFilter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, value := range data {
		if w.capturing {
			if value == '\n' {
				w.capturing = false
				w.responseCount++
				w.atLineStart = true
				continue
			}
			if value != '\r' {
				if len(w.response) >= maxBrokerControlOutput {
					w.overflow = true
				} else {
					w.response = append(w.response, value)
				}
			}
			continue
		}
		if w.atLineStart {
			if value == loginResultPrefix[w.prefixPosition] {
				w.prefixPending = append(w.prefixPending, value)
				w.prefixPosition++
				if w.prefixPosition == len(loginResultPrefix) {
					w.prefixPending = nil
					w.prefixPosition = 0
					w.capturing = true
				}
				continue
			}
			if len(w.prefixPending) != 0 {
				if _, err := w.destination.Write(w.prefixPending); err != nil {
					return 0, err
				}
				w.prefixPending = nil
				w.prefixPosition = 0
			}
		}
		if _, err := w.destination.Write([]byte{value}); err != nil {
			return 0, err
		}
		w.atLineStart = value == '\n'
	}
	return len(data), nil
}

func (w *loginOutputFilter) responseLine() ([]byte, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.prefixPending) != 0 {
		_, _ = w.destination.Write(w.prefixPending)
		w.prefixPending = nil
		w.prefixPosition = 0
	}
	_ = w.destination.flushPending()
	return bytes.TrimSpace(append([]byte(nil), w.response...)),
		w.responseCount == 1 && !w.capturing && !w.overflow
}

func (r *Runtime) LoginAuth(
	ctx context.Context, contextName, providerID string, input io.Reader, errOut io.Writer,
) (authbroker.Result, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.Result{}, err
	}
	if provider.Acquisition.Mode != authbroker.AcquisitionBuiltinHelper || provider.Acquisition.Helper != "github-gh" {
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
	response, err := r.runInteractiveBrokerLogin(ctx, manifest.ID, provider.ID, input, errOut)
	if err != nil {
		return authbroker.Result{}, classifyBrokerError(err, "auth login")
	}
	return buildAuthResult(authbroker.TaskLogin, manifest.Name, manifest.ID, provider.ID, response, true)
}

func (r *Runtime) runInteractiveBrokerLogin(
	ctx context.Context, contextID, provider string, input io.Reader, errOut io.Writer,
) (brokerControlResponse, error) {
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return brokerControlResponse{}, authLoginTerminalRequiredFault()
	}
	expectation := brokerControlExpectation{Operation: brokerControlLogin, Provider: provider}
	args := []string{
		"exec", "-i", "-t", authBrokerContainer,
		"python", "-m", "authbroker.control", "login",
		"--context-id", contextID, "--provider", provider,
	}
	loginContext, cancel := context.WithTimeout(ctx, brokerLoginTimeout)
	defer cancel()
	opener := r.browser
	if opener == nil {
		opener = osHostBrowserOpener{}
	}
	filter := newLoginOutputFilter(errOut, func(target string) error {
		openContext, stopOpen := context.WithTimeout(loginContext, hostBrowserOpenTimeout)
		defer stopOpen()
		return opener.Open(openContext, target)
	})
	_ = r.runner.Run(loginContext, args, os.Environ(), input, filter, filter)
	responseLine, found := filter.responseLine()
	if found {
		response, decodeErr := decodeBrokerControlResponse(responseLine, expectation)
		if decodeErr == nil {
			if response.OK {
				return response, nil
			}
			if response.Error != nil && response.Error.Code != "" {
				if response.Error.Code == "transport_error" {
					return brokerControlResponse{}, brokerMutationOutcomeUnknown{}
				}
				return brokerControlResponse{}, brokerControlError{Code: response.Error.Code}
			}
		}
	}
	return brokerControlResponse{}, brokerMutationOutcomeUnknown{}
}

func authLoginTerminalRequiredFault() error {
	return fault.New(
		fault.KindInvalidInput,
		"auth_login_tty_required",
		"GitHub login requires interactive terminal streams on stdin and stderr.",
		false,
		fault.NextAction{Command: "help auth login", Reason: "Run trusted-host GitHub login from an interactive terminal."},
	)
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
	if _, configured, stateErr := r.LoadState(ctx); stateErr != nil {
		return authbroker.StatusResult{}, stateErr
	} else if !configured {
		return authbroker.StatusResult{}, authBrokerUnavailableFault()
	}
	state, err := r.brokerState(ctx)
	if err != nil || state == authbroker.BrokerStateUnavailable {
		return authbroker.StatusResult{}, authBrokerUnavailableFault()
	}
	result := authbroker.StatusResult{
		Task: authbroker.TaskStatus, Context: manifest.Name, ContextID: manifest.ID,
		StorageBackend: backend, BrokerState: state, Providers: []authbroker.ProviderStatus{},
		WorkspaceActivation: authbroker.WorkspaceActivation{State: authbroker.WorkspaceActivationNotApplicable},
	}
	configured := false
	for _, provider := range projection.Providers {
		status := authbroker.ProviderStatus{
			Provider: provider.ID,
			State:    authbroker.ProviderCredentialUnavailable,
		}
		if state == authbroker.BrokerStateReady {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", manifest.ID, "--provider", provider.ID,
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
		result.Providers = append(result.Providers, status)
	}
	if configured {
		result.WorkspaceActivation = authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: authbroker.ContextAuthActivationGuidance,
		}
	}
	sort.Slice(result.Providers, func(left, right int) bool {
		return result.Providers[left].Provider < result.Providers[right].Provider
	})
	if err := result.Validate(); err != nil {
		return authbroker.StatusResult{}, fmt.Errorf("Auth Broker status result is invalid: %w", err)
	}
	return result, nil
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
