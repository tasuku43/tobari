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
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const (
	githubEphemeralPlaintextWarning = "! Authentication credentials saved in plain text"
	githubManualBrowserFallback     = "The host browser did not open; visit " + githubDeviceURL + " manually to continue.\n"
	maxLoginVisibleLine             = 64 * 1024
	maxLoginVisibleBytes            = 64 * 1024
	hostBrowserOpenTimeout          = 5 * time.Second
)

var errLoginVisibleOutputLimit = errors.New("host login visible output exceeded its limit")

type loginVisibleOutput struct {
	mu          sync.Mutex
	destination io.Writer
	openBrowser func(string) error
	pending     []byte
	opened      bool
	written     int
	visible     int
	failure     error
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
	target, recognized := loginBrowserTarget(normalized)
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

func loginBrowserTarget(line string) (string, bool) {
	if strings.Contains(line, githubDeviceURL) {
		return githubDeviceURL, true
	}
	return "", false
}

func manualBrowserFallback(target string) string {
	return githubManualBrowserFallback
}

func (r *Runtime) LoginAuth(
	ctx context.Context, contextName, providerID, method string, input io.Reader, errOut io.Writer,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	if !supportsBuiltinAuthHelper(provider) {
		return authbroker.MutationObservation{}, fault.New(
			fault.KindUnsupported, "provider_login_unsupported",
			"The selected provider does not support interactive login.", false,
			fault.NextAction{Command: "help auth import", Reason: "Use protected stdin import for a compatible user provider."},
		)
	}
	if !r.IsInputTerminal(input) || !r.IsTerminal(errOut) {
		return authbroker.MutationObservation{}, authLoginTerminalRequiredFault()
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runHostCredentialLogin(ctx, manifest.ID, provider.ID, input, errOut, method)
	if err != nil {
		return authbroker.MutationObservation{}, classifyHostLoginError(err, provider.ID, method)
	}
	return r.buildAuthMutationObservation(ctx, authbroker.TaskLogin, manifest.Name, manifest.ID, provider.ID, response, true, true, backend)
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
	return provider.ID == "github" && provider.Acquisition.Helper == "github-gh"
}

func classifyHostLoginError(err error, provider string, _ ...string) error {
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}
	var unavailable hostCLIUnavailableError
	if errors.As(err, &unavailable) {
		return fault.New(
			fault.KindUnavailable, "github_cli_unavailable",
			"The trusted-host GitHub CLI is unavailable; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Install the reviewed host CLI and retry this login."},
		)
	}
	if hostLoginCancelled(err) {
		return fault.New(
			fault.KindRejected, "github_login_cancelled",
			"GitHub login was cancelled; the previous Context credential remains unchanged.", false,
			fault.NextAction{Command: "auth login", Reason: "Retry the trusted-host GitHub login when ready."},
		)
	}
	if errors.Is(err, credentialhost.ErrGitHubExecutable) {
		return classifyHostLoginError(hostCLIUnavailableError{provider: provider}, provider)
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

func (r *Runtime) ImportAuth(
	ctx context.Context, contextName, providerID string, secret io.Reader,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	if provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
		return authbroker.MutationObservation{}, fault.New(
			fault.KindUnsupported, "provider_import_unsupported",
			"The selected provider does not support credential import.", false,
			fault.NextAction{Command: "auth login", Reason: "Use the provider's reviewed built-in acquisition helper."},
		)
	}
	if secret == nil {
		return authbroker.MutationObservation{}, fault.New(fault.KindInvalidInput, "invalid_credential_input", "Credential stdin is unavailable.", false)
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runBrokerControl(
		ctx, secret, "import", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.MutationObservation{}, classifyBrokerError(err, "auth import "+provider.ID)
	}
	return r.buildAuthMutationObservation(ctx, authbroker.TaskImport, manifest.Name, manifest.ID, provider.ID, response, true, true, backend)
}

func (r *Runtime) AuthStatus(ctx context.Context, contextName string) (authbroker.StatusObservation, error) {
	observed, err := r.observeContext(contextName)
	if err != nil {
		if errors.Is(err, tobari.ErrContextNotFound) {
			return authbroker.StatusObservation{}, fault.New(
				fault.KindNotFound, "context_not_found", "The selected Context does not exist.", false,
				fault.NextAction{Command: "context list", Reason: "Choose an existing Context before using authentication."},
			)
		}
		return authbroker.StatusObservation{}, err
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.StatusObservation{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.StatusObservation{}, classifyRootKeyError(err)
	}
	result := authbroker.StatusObservation{
		ContextState: observed.state,
		Context:      observed.manifest.Name, ContextID: observed.manifest.ID,
		StorageBackend: backend, BrokerState: authbroker.BrokerStateUnavailable,
		Providers: []authbroker.ProviderStatus{},
		Workspaces: authbroker.WorkspaceObservation{
			Coverage:   authbroker.WorkspaceActivationCoverageNotApplicable,
			Workspaces: []authbroker.WorkspaceProjectionObservation{},
		},
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
	if observed.state != tobari.ContextObservationPersisted {
		return result, nil
	}
	manifest := observed.manifest
	if _, configured, stateErr := r.LoadState(ctx); stateErr != nil {
		return authbroker.StatusObservation{}, stateErr
	} else if configured {
		state, stateErr := r.brokerState(ctx)
		if stateErr == nil && state != authbroker.BrokerStateUnavailable {
			result.BrokerState = state
		}
	}
	for index := range result.Providers {
		status := result.Providers[index]
		if result.BrokerState == authbroker.BrokerStateReady {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", manifest.ID, "--provider", status.Provider,
			)
			if statusErr != nil {
				return authbroker.StatusObservation{}, classifyBrokerError(statusErr, "auth status")
			}
			if response.State == "configured" {
				status.State = authbroker.ProviderCredentialConfigured
				status.CredentialRevision = response.Revision
				status.AccountLabel, err = validatedAccountLabel(response.AccountLabel)
				if err != nil {
					return authbroker.StatusObservation{}, err
				}
			} else {
				status.State = authbroker.ProviderCredentialNotConfigured
			}
		}
		result.Providers[index] = status
	}
	result.Workspaces = r.observeWorkspaceActivation(ctx, manifest.ID, result.Providers, projection)
	return result, nil
}

func (r *Runtime) LogoutAuth(
	ctx context.Context, contextName, providerID string,
) (authbroker.MutationObservation, error) {
	manifest, provider, err := r.authOperationTarget(ctx, contextName, providerID)
	if err != nil {
		return authbroker.MutationObservation{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.MutationObservation{}, classifyRootKeyError(err)
	}
	if err := r.requireAuthBroker(ctx); err != nil {
		return authbroker.MutationObservation{}, err
	}
	response, err := r.runBrokerControl(
		ctx, nil, "logout", "--context-id", manifest.ID, "--provider", provider.ID,
	)
	if err != nil {
		return authbroker.MutationObservation{}, classifyBrokerError(err, "auth logout")
	}
	changed := response.Changed != nil && *response.Changed
	return r.buildAuthMutationObservation(ctx, authbroker.TaskLogout, manifest.Name, manifest.ID, provider.ID, response, false, changed, backend)
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

func (r *Runtime) buildAuthMutationObservation(
	ctx context.Context,
	_ string, contextName, contextID, provider string,
	response brokerControlResponse,
	configured, changed bool,
	backend authbroker.StorageBackend,
) (authbroker.MutationObservation, error) {
	result := authbroker.MutationObservation{
		ContextState: tobari.ContextObservationPersisted,
		Provider:     provider, Context: contextName, ContextID: contextID,
		Configured: configured, StorageBackend: backend, BrokerState: authbroker.BrokerStateReady,
		Changed: changed, Providers: []authbroker.ProviderStatus{},
		Workspaces: authbroker.WorkspaceObservation{Coverage: authbroker.WorkspaceActivationCoverageUnavailable, Workspaces: []authbroker.WorkspaceProjectionObservation{}},
	}
	if !changed {
		result.Workspaces = authbroker.WorkspaceObservation{Coverage: authbroker.WorkspaceActivationCoverageNotApplicable, Workspaces: []authbroker.WorkspaceProjectionObservation{}}
	}
	if configured {
		result.CredentialRevision = response.Revision
		result.AccountLabel = response.AccountLabel
	}
	if changed {
		projection, projectionErr := r.loadAuthProviders()
		if projectionErr == nil {
			statuses := make([]authbroker.ProviderStatus, 0, len(projection.Providers))
			for _, installed := range projection.Providers {
				status := authbroker.ProviderStatus{Provider: installed.ID, State: authbroker.ProviderCredentialUnavailable}
				observed, statusErr := r.runBrokerControl(
					ctx, nil, "status", "--context-id", contextID, "--provider", installed.ID,
				)
				if statusErr == nil {
					switch observed.State {
					case "not_configured":
						status.State = authbroker.ProviderCredentialNotConfigured
					case "configured":
						status.State = authbroker.ProviderCredentialConfigured
						status.CredentialRevision = observed.Revision
					}
				}
				statuses = append(statuses, status)
			}
			result.Providers = statuses
			result.Workspaces = r.observeWorkspaceActivation(ctx, contextID, statuses, projection)
		}
	}
	return result, nil
}

func (r *Runtime) observeWorkspaceActivation(
	ctx context.Context,
	targetContextID string,
	statuses []authbroker.ProviderStatus,
	projection authbroker.Projection,
) authbroker.WorkspaceObservation {
	unavailable := func() authbroker.WorkspaceObservation {
		return authbroker.WorkspaceObservation{
			Coverage:   authbroker.WorkspaceActivationCoverageUnavailable,
			Workspaces: []authbroker.WorkspaceProjectionObservation{},
		}
	}
	if len(statuses) > authbroker.MaxWorkspaceActivationProviders ||
		len(projection.Providers) > authbroker.MaxWorkspaceActivationProviders {
		return unavailable()
	}
	projects, err := r.ListProjects(ctx)
	if err != nil || len(projects) > authbroker.MaxWorkspaceActivationItems {
		return unavailable()
	}
	statusByProvider := make(map[string]authbroker.ProviderStatus, len(statuses))
	providerByID := make(map[string]authbroker.Provider, len(projection.Providers))
	for _, status := range statuses {
		statusByProvider[status.Provider] = status
	}
	for _, provider := range projection.Providers {
		providerByID[provider.ID] = provider
	}
	type bindingCheck struct {
		providerIndex  int
		workspaceIndex int
		projectID      string
		providerID     string
		revision       string
		bindings       []byte
	}
	workspaces := make([]authbroker.WorkspaceProjectionObservation, 0, len(projects))
	checks := make([]bindingCheck, 0)
	bindingChecks := 0
	for _, project := range projects {
		workspace := authbroker.WorkspaceProjectionObservation{
			ProjectID: project.ID, Root: project.Root, ProjectContextID: project.ContextID,
			Incomplete: project.Incomplete, Providers: []authbroker.WorkspaceProviderObservation{},
		}
		registry, registryErr := r.readProjectAuthRegistry(project.ID)
		if registryErr != nil {
			workspaces = append(workspaces, workspace)
			continue
		}
		workspace.RegistryAvailable = true
		workspace.RegistryProjectID = registry.ProjectID
		if len(registry.Providers) > authbroker.MaxWorkspaceActivationProviders {
			return unavailable()
		}
		observed := make(map[string]projectAuthProviderBinding, len(registry.Providers))
		providerIDs := make(map[string]struct{}, len(statuses)+len(registry.Providers))
		for _, status := range statuses {
			providerIDs[status.Provider] = struct{}{}
		}
		for _, binding := range registry.Providers {
			observed[binding.Provider] = binding
			providerIDs[binding.Provider] = struct{}{}
		}
		if len(providerIDs) > authbroker.MaxWorkspaceActivationProviders {
			return unavailable()
		}
		ordered := make([]string, 0, len(providerIDs))
		for providerID := range providerIDs {
			ordered = append(ordered, providerID)
		}
		sort.Strings(ordered)
		for _, providerID := range ordered {
			status, installed := statusByProvider[providerID]
			current, projected := observed[providerID]
			fact := authbroker.WorkspaceProviderObservation{Provider: providerID, BindingState: authbroker.BrokerBindingNotObserved}
			if projected {
				fact.RegistryPresent = true
				fact.RegistryRevision = current.Revision
				fact.RegistryBindingDigest = current.BindingDigest
			}
			var encoded []byte
			if provider, exists := providerByID[providerID]; exists {
				_, encodedBindings, digest, bindingErr := brokerBindingsForProvider(projection, provider.ID)
				if bindingErr == nil {
					fact.ExpectedBindingDigest = digest
					encoded = encodedBindings
				}
			}
			workspace.Providers = append(workspace.Providers, fact)
			if project.ContextID == targetContextID && installed && status.State == authbroker.ProviderCredentialConfigured && projected &&
				fact.ExpectedBindingDigest != "" && current.Revision == status.CredentialRevision &&
				current.BindingDigest == fact.ExpectedBindingDigest {
				checks = append(checks, bindingCheck{
					workspaceIndex: len(workspaces), providerIndex: len(workspace.Providers) - 1,
					projectID: project.ID, providerID: providerID, revision: current.Revision, bindings: encoded,
				})
				bindingChecks++
			}
		}
		workspaces = append(workspaces, workspace)
	}
	if bindingChecks > authbroker.MaxWorkspaceActivationBindingChecks {
		return unavailable()
	}
	for _, check := range checks {
		fact := &workspaces[check.workspaceIndex].Providers[check.providerIndex]
		fact.BindingProvider = check.providerID
		fact.BindingRevision = check.revision
		fact.BindingState = authbroker.BrokerBindingUnavailable
		binding, bindingErr := r.runBrokerControl(
			ctx, nil, "binding_status", "--context-id", workspaces[check.workspaceIndex].ProjectContextID,
			"--project-id", check.projectID, "--provider", check.providerID,
			"--revision", check.revision, "--bindings", string(check.bindings),
		)
		if bindingErr == nil {
			switch binding.State {
			case "ready":
				fact.BindingState = authbroker.BrokerBindingReady
			case "missing":
				fact.BindingState = authbroker.BrokerBindingMissing
			case "stale":
				fact.BindingState = authbroker.BrokerBindingStale
			}
		}
	}
	return authbroker.WorkspaceObservation{
		Coverage:   authbroker.WorkspaceActivationCoverageExhaustive,
		Workspaces: workspaces,
	}
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
