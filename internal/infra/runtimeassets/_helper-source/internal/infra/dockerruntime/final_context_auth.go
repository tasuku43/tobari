package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/capabilitysurface"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

// WithFinalContextAuthObservation shares an already-existing lifecycle lock
// without creating state. A fresh installation is observed lock-free and the
// underlying lifecycle seam rejects state appearing during the observation.
func (r *Runtime) WithFinalContextAuthObservation(ctx context.Context, action func(context.Context) error) error {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if action == nil {
		return fmt.Errorf("final Context authentication observation is unavailable")
	}
	return r.withLifecycleObservation(ctx, action)
}

func (r *Runtime) ResolveFinalContextProvider(ctx context.Context, authority authbroker.ContextAuthenticationAuthority, providerID string) (authbroker.ContextProviderAuthority, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ContextProviderAuthority{}, fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := authority.ValidateFor(authority.ContextRef); err != nil {
		return authbroker.ContextProviderAuthority{}, err
	}
	if err := ctx.Err(); err != nil {
		return authbroker.ContextProviderAuthority{}, err
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.ContextProviderAuthority{}, err
	}
	provider, found := findAuthProvider(projection, providerID)
	if !found {
		return authbroker.ContextProviderAuthority{}, fault.New(fault.KindNotFound, "provider_not_installed", "The credential provider is not installed.", false, fault.NextAction{Command: "auth status", Reason: "Inspect the exact final Context credential inventory."})
	}
	target := authbroker.ContextProviderAuthority{Context: authority, Provider: provider}
	if err := target.ValidateFor(authority.ContextRef, providerID); err != nil {
		return authbroker.ContextProviderAuthority{}, err
	}
	return target, nil
}

func (r *Runtime) ObserveFinalContextProvider(ctx context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := target.ValidateFor(target.Context.ContextRef, target.Provider.ID); err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyRootKeyError(err)
	}
	state, err := r.brokerState(ctx)
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	if state != authbroker.BrokerStateReady {
		return authbroker.ProviderStatus{Provider: target.Provider.ID, State: authbroker.ProviderCredentialUnavailable}, backend, state, nil
	}
	response, err := r.runBrokerControl(ctx, nil, "status", "--context-id", string(target.Context.ContextID), "--provider", target.Provider.ID)
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyBrokerError(err, "auth status")
	}
	status, err := finalProviderStatus(target.Provider.ID, response)
	return status, backend, state, err
}

func (r *Runtime) ObserveFinalContextInventory(ctx context.Context, authority authbroker.ContextAuthenticationAuthority) (authbroker.ContextStatusObservation, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ContextStatusObservation{}, fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := authority.ValidateFor(authority.ContextRef); err != nil {
		return authbroker.ContextStatusObservation{}, err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.ContextStatusObservation{}, classifyRootKeyError(err)
	}
	state, err := r.brokerState(ctx)
	if err != nil {
		return authbroker.ContextStatusObservation{}, err
	}
	if state == authbroker.BrokerStateUnavailable {
		return authbroker.NewContextStatusObservation(authority, backend, state, []authbroker.ProviderStatus{}, false)
	}
	response, err := r.runBrokerControl(ctx, nil, "context_status", "--context-id", string(authority.ContextID))
	if err != nil {
		return authbroker.ContextStatusObservation{}, classifyBrokerError(err, "auth status")
	}
	if response.Complete == nil {
		return authbroker.ContextStatusObservation{}, fmt.Errorf("Auth Broker Context inventory omitted coverage")
	}
	if !*response.Complete {
		return authbroker.NewContextStatusObservation(authority, backend, authbroker.BrokerStateLocked, []authbroker.ProviderStatus{}, false)
	}
	configured := make(map[string]authbroker.ProviderStatus, len(response.Providers))
	for _, item := range response.Providers {
		configured[item.Provider] = authbroker.ProviderStatus{Provider: item.Provider, State: authbroker.ProviderCredentialConfigured, AccountLabel: item.AccountLabel, CredentialRevision: item.Revision}
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.ContextStatusObservation{}, err
	}
	for _, provider := range projection.Providers {
		if _, present := configured[provider.ID]; !present {
			configured[provider.ID] = authbroker.ProviderStatus{Provider: provider.ID, State: authbroker.ProviderCredentialNotConfigured}
		}
	}
	providers := make([]authbroker.ProviderStatus, 0, len(configured))
	for _, provider := range configured {
		providers = append(providers, provider)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Provider < providers[j].Provider })
	return authbroker.NewContextStatusObservation(authority, backend, authbroker.BrokerStateReady, providers, true)
}

func (r *Runtime) LoginFinalContextProvider(ctx context.Context, target authbroker.ContextProviderAuthority, method string, input io.Reader, errOut io.Writer) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := target.ValidateFor(target.Context.ContextRef, target.Provider.ID); err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	if !supportsBuiltinAuthHelper(target.Provider) {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "provider_login_unsupported", "The selected provider does not support interactive login.", false)
	}
	driver, reviewed := reviewedHostLoginDriverForProvider(target.Provider.ID)
	if !reviewed {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "provider_login_unsupported", "The selected provider does not support interactive login.", false)
	}
	runtimeImage := ""
	if driver.kind == reviewedHostLoginDriverDatadog || driver.kind == reviewedHostLoginDriverAnthropic {
		resolvedImage, err := r.resolveFinalContextLoginRuntimeImage(ctx, target.Context.Runtime)
		if err != nil {
			return authbroker.ProviderStatus{}, "", "", err
		}
		runtimeImage = resolvedImage
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyRootKeyError(err)
	}
	if err := r.requireUnlockedAuthBroker(ctx); err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	response, err := r.runHostCredentialLoginOnTTYForRuntime(ctx, string(target.Context.ContextID), runtimeImage, target.Provider.ID, input, errOut, method)
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyHostLoginError(err, target.Provider.ID, method)
	}
	status, err := finalProviderStatus(target.Provider.ID, response)
	return status, backend, authbroker.BrokerStateReady, err
}

func (r *Runtime) ImportFinalContextProvider(ctx context.Context, target authbroker.ContextProviderAuthority, secret io.Reader) (authbroker.ProviderStatus, authbroker.StorageBackend, authbroker.BrokerState, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := target.ValidateFor(target.Context.ContextRef, target.Provider.ID); err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	if target.Provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
		return authbroker.ProviderStatus{}, "", "", fault.New(fault.KindUnsupported, "provider_import_unsupported", "The selected provider does not support credential import.", false)
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyRootKeyError(err)
	}
	if err := r.requireUnlockedAuthBroker(ctx); err != nil {
		return authbroker.ProviderStatus{}, "", "", err
	}
	response, err := r.runBrokerControl(ctx, secret, "import", "--context-id", string(target.Context.ContextID), "--provider", target.Provider.ID)
	if err != nil {
		return authbroker.ProviderStatus{}, "", "", classifyBrokerError(err, "auth import "+target.Provider.ID)
	}
	status, err := finalProviderStatus(target.Provider.ID, response)
	return status, backend, authbroker.BrokerStateReady, err
}

func (r *Runtime) LogoutFinalContextProvider(ctx context.Context, target authbroker.ContextProviderAuthority) (authbroker.ProviderStatus, bool, authbroker.StorageBackend, authbroker.BrokerState, error) {
	if !capabilitysurface.Compiled().IncludesResearch() {
		return authbroker.ProviderStatus{}, false, "", "", fault.New(fault.KindUnsupported, "research_auth_unavailable", "Research authentication is absent from this capability surface.", false)
	}
	if err := target.ValidateFor(target.Context.ContextRef, target.Provider.ID); err != nil {
		return authbroker.ProviderStatus{}, false, "", "", err
	}
	backend, err := authStorageBackend()
	if err != nil {
		return authbroker.ProviderStatus{}, false, "", "", classifyRootKeyError(err)
	}
	if err := r.requireUnlockedAuthBroker(ctx); err != nil {
		return authbroker.ProviderStatus{}, false, "", "", err
	}
	response, err := r.runBrokerControl(ctx, nil, "logout", "--context-id", string(target.Context.ContextID), "--provider", target.Provider.ID)
	if err != nil {
		return authbroker.ProviderStatus{}, false, "", "", classifyBrokerError(err, "auth logout")
	}
	if response.Changed == nil {
		return authbroker.ProviderStatus{}, false, "", "", fmt.Errorf("Auth Broker logout omitted change state")
	}
	return authbroker.ProviderStatus{Provider: target.Provider.ID, State: authbroker.ProviderCredentialNotConfigured}, *response.Changed, backend, authbroker.BrokerStateReady, nil
}

func finalProviderStatus(provider string, response brokerControlResponse) (authbroker.ProviderStatus, error) {
	status := authbroker.ProviderStatus{Provider: provider}
	switch response.State {
	case "ready", "":
		status.State = authbroker.ProviderCredentialConfigured
		status.AccountLabel = response.AccountLabel
		status.CredentialRevision = response.Revision
	case "not_configured":
		status.State = authbroker.ProviderCredentialNotConfigured
	case "locked":
		status.State = authbroker.ProviderCredentialUnavailable
	default:
		return authbroker.ProviderStatus{}, fmt.Errorf("Auth Broker provider state is invalid")
	}
	return status, status.Validate()
}
