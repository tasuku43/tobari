package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/authproviders"
	"github.com/tasuku43/tobari/internal/infra/rootkey"
)

const (
	maxBrokerControlOutput = 64 * 1024
	brokerControlTimeout   = 5 * time.Second
	brokerLoginTimeout     = 15 * time.Minute
)

type brokerControlError struct{ Code string }

func (e brokerControlError) Error() string {
	if e.Code == "" {
		return "Auth Broker request failed"
	}
	return "Auth Broker request failed: " + e.Code
}

type brokerControlResponse struct {
	SchemaVersion int     `json:"schema_version"`
	OK            bool    `json:"ok"`
	State         string  `json:"state,omitempty"`
	EpochID       string  `json:"epoch_id,omitempty"`
	Provider      string  `json:"provider,omitempty"`
	Revision      string  `json:"revision,omitempty"`
	AccountLabel  *string `json:"account_label,omitempty"`
	Handle        string  `json:"handle,omitempty"`
	Changed       *bool   `json:"changed,omitempty"`
	Error         *struct {
		Code string `json:"code"`
	} `json:"error,omitempty"`
}

type boundedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (w *boundedBuffer) Write(data []byte) (int, error) {
	remaining := w.limit - w.buffer.Len()
	if remaining <= 0 {
		w.overflow = true
		return 0, fmt.Errorf("bounded output exceeded")
	}
	if len(data) > remaining {
		_, _ = w.buffer.Write(data[:remaining])
		w.overflow = true
		return remaining, fmt.Errorf("bounded output exceeded")
	}
	return w.buffer.Write(data)
}

func (r *Runtime) authProviderDirectory() string {
	return filepath.Join(r.configDirectory, "auth", "providers")
}

func (r *Runtime) authProviderProjectionPath() string {
	return filepath.Join(r.stateDirectory, "auth", "projection", "providers.json")
}

func (r *Runtime) authContextsDirectory() string {
	return filepath.Join(r.stateDirectory, "auth", "contexts")
}

func (r *Runtime) authRuntimeDirectory() string {
	return filepath.Join(r.stateDirectory, "auth", "runtime")
}

func (r *Runtime) loadAuthProviders() (authbroker.Projection, error) {
	loader, err := authproviders.New(r.authProviderDirectory())
	if err != nil {
		return authbroker.Projection{}, err
	}
	projection, err := loader.Load()
	if err != nil {
		if errors.Is(err, authbroker.ErrAmbiguousHTTPBinding) {
			return authbroker.Projection{}, fault.Wrap(
				fault.KindRejected, "ambiguous_provider_http_binding",
				"Credential-provider HTTP bindings overlap and cannot be selected safely.", false, err,
				fault.NextAction{Command: "doctor", Reason: "Remove the overlapping exact host, port, header, and source-format binding."},
			)
		}
		return authbroker.Projection{}, fault.Wrap(
			fault.KindRejected, "invalid_provider_manifest",
			"The credential-provider manifest collection is invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the owner-controlled XDG provider manifests."},
		)
	}
	return projection, nil
}

func (r *Runtime) prepareAuthProjection() (authbroker.Projection, error) {
	if err := rootkey.PrepareBrokerDirectories(r.stateDirectory); err != nil {
		return authbroker.Projection{}, classifyRootKeyError(err)
	}
	if err := r.ensurePrivateDirectory(filepath.Dir(r.authProviderDirectory())); err != nil {
		return authbroker.Projection{}, fmt.Errorf("prepare auth provider configuration directory: %w", err)
	}
	if _, err := os.Lstat(r.authProviderDirectory()); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(r.authProviderDirectory(), 0o700); err != nil {
			return authbroker.Projection{}, fmt.Errorf("prepare auth provider directory: %w", err)
		}
	} else if err != nil {
		return authbroker.Projection{}, fmt.Errorf("inspect auth provider directory: %w", err)
	}
	projection, err := r.loadAuthProviders()
	if err != nil {
		return authbroker.Projection{}, err
	}
	if err := writeAtomicJSON(r.authProviderProjectionPath(), projection); err != nil {
		return authbroker.Projection{}, fmt.Errorf("write normalized auth provider projection: %w", err)
	}
	return projection, nil
}

func findAuthProvider(projection authbroker.Projection, id string) (authbroker.Provider, bool) {
	for _, provider := range projection.Providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return authbroker.Provider{}, false
}

func (r *Runtime) inspectAuthProviderProjectionIntegrity() string {
	expected, err := r.loadAuthProviders()
	if err != nil {
		return "invalid"
	}
	data, err := readOwnerPolicyFile(r.authProviderProjectionPath(), 4*1024*1024)
	if err != nil {
		return "invalid"
	}
	var observed authbroker.Projection
	if err := decodeStrictJSON(data, &observed); err != nil {
		return "invalid"
	}
	expectedJSON, expectedErr := json.Marshal(expected)
	observedJSON, observedErr := json.Marshal(observed)
	if expectedErr != nil || observedErr != nil || !bytes.Equal(expectedJSON, observedJSON) {
		return "invalid"
	}
	return "valid"
}

func (r *Runtime) runBrokerControl(
	ctx context.Context, input io.Reader, arguments ...string,
) (brokerControlResponse, error) {
	if err := ctx.Err(); err != nil {
		return brokerControlResponse{}, err
	}
	expectation, err := brokerControlExpectationFor(arguments)
	if err != nil {
		return brokerControlResponse{}, err
	}
	args := []string{"exec", "-i", authBrokerContainer, "python", "-m", "authbroker.control"}
	args = append(args, arguments...)
	controlContext, cancel := context.WithTimeout(ctx, brokerControlTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: maxBrokerControlOutput}
	stderr := &boundedBuffer{limit: maxBrokerControlOutput}
	runErr := r.runner.Run(controlContext, args, os.Environ(), input, stdout, stderr)
	if !stdout.overflow {
		response, decodeErr := decodeBrokerControlResponse(bytes.TrimSpace(stdout.buffer.Bytes()), expectation)
		if decodeErr == nil {
			if response.OK {
				// An exact authoritative success frame confirms the operation even
				// when Docker reports a later process or context failure.
				return response, nil
			}
			if response.Error != nil && response.Error.Code != "" {
				if expectation.mutationOutcomeSensitive() && response.Error.Code == "transport_error" {
					return brokerControlResponse{}, brokerMutationOutcomeUnknown{}
				}
				return brokerControlResponse{}, brokerControlError{Code: response.Error.Code}
			}
		}
	}
	if expectation.mutationOutcomeSensitive() {
		return brokerControlResponse{}, brokerMutationOutcomeUnknown{}
	}
	if err := ctx.Err(); err != nil {
		return brokerControlResponse{}, err
	}
	if errors.Is(controlContext.Err(), context.DeadlineExceeded) {
		return brokerControlResponse{}, fmt.Errorf("Auth Broker control command exceeded its bounded timeout")
	}
	if runErr != nil {
		return brokerControlResponse{}, fmt.Errorf("Auth Broker control command failed: %w", runErr)
	}
	return brokerControlResponse{}, fmt.Errorf("Auth Broker returned an invalid bounded response")
}

func (r *Runtime) brokerState(ctx context.Context) (authbroker.BrokerState, error) {
	component, err := r.inspectContainer(ctx, "auth-broker", authBrokerContainer)
	if err != nil {
		return authbroker.BrokerStateUnavailable, err
	}
	if component.State != "running" || (component.Health != "healthy" && component.Health != "none") {
		return authbroker.BrokerStateUnavailable, nil
	}
	response, err := r.runBrokerControl(ctx, nil, "health")
	if err != nil {
		return authbroker.BrokerStateUnavailable, nil
	}
	switch response.State {
	case "unlocked":
		return authbroker.BrokerStateReady, nil
	case "locked":
		return authbroker.BrokerStateLocked, nil
	default:
		return authbroker.BrokerStateUnavailable, nil
	}
}

func (r *Runtime) requireAuthBroker(ctx context.Context) error {
	if _, configured, err := r.LoadState(ctx); err != nil {
		return err
	} else if !configured {
		return authBrokerUnavailableFault()
	}
	return r.requireUnlockedAuthBroker(ctx)
}

func (r *Runtime) requireUnlockedAuthBroker(ctx context.Context) error {
	state, err := r.brokerState(ctx)
	if err != nil || state == authbroker.BrokerStateUnavailable {
		return authBrokerUnavailableFault()
	}
	if state == authbroker.BrokerStateLocked {
		return fault.New(
			fault.KindUnavailable, "auth_broker_locked",
			"The Auth Broker is locked and cannot use Context credentials.", false,
			fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared cluster and unlock the Auth Broker."},
		)
	}
	return nil
}

func authBrokerUnavailableFault() error {
	return fault.New(
		fault.KindUnavailable, "auth_broker_unavailable",
		"The shared Auth Broker is unavailable.", true,
		fault.NextAction{Command: "cluster up", Reason: "Start or reconcile the shared cluster before using authentication."},
	)
}

func (r *Runtime) unlockAuthBroker(ctx context.Context) ([]byte, error) {
	state, err := r.waitForAuthBrokerControl(ctx)
	if err != nil {
		return nil, err
	}
	key, err := r.loadInstallationRootKey(ctx)
	if err != nil {
		return nil, err
	}
	if state == authbroker.BrokerStateReady {
		return key, nil
	}
	response, err := r.runBrokerControl(ctx, bytes.NewReader(key), "unlock")
	if err != nil {
		clear(key)
		return nil, classifyBrokerError(err, "cluster up")
	}
	if response.State != "unlocked" {
		clear(key)
		return nil, fault.New(
			fault.KindContract, "auth_broker_unlock_failed",
			"The Auth Broker did not confirm its unlocked state.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect Auth Broker and root-key provider state."},
		)
	}
	return key, nil
}

func (r *Runtime) loadInstallationRootKey(ctx context.Context) ([]byte, error) {
	if r.rootKeyLoader != nil {
		key, err := r.rootKeyLoader(ctx)
		if err != nil {
			return nil, classifyRootKeyError(err)
		}
		if len(key) != 32 {
			clear(key)
			return nil, classifyRootKeyError(rootkey.ErrUnsafe)
		}
		return key, nil
	}
	exists, err := rootkey.EncryptedStateExists(r.stateDirectory)
	if err != nil {
		return nil, classifyRootKeyError(err)
	}
	provider, err := rootkey.New(r.stateDirectory)
	if err != nil {
		return nil, classifyRootKeyError(err)
	}
	material, err := provider.LoadOrCreate(ctx, exists)
	if err != nil {
		return nil, classifyRootKeyError(err)
	}
	key := material.Bytes()
	if len(key) != 32 {
		clear(key)
		return nil, classifyRootKeyError(rootkey.ErrUnsafe)
	}
	return key, nil
}

func (r *Runtime) waitForAuthBrokerControl(ctx context.Context) (authbroker.BrokerState, error) {
	for attempt := 0; attempt < 60; attempt++ {
		state, err := r.brokerState(ctx)
		if err == nil && state != authbroker.BrokerStateUnavailable {
			return state, nil
		}
		select {
		case <-ctx.Done():
			return authbroker.BrokerStateUnavailable, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return authbroker.BrokerStateUnavailable, authBrokerUnavailableFault()
}

func classifyRootKeyError(err error) error {
	switch {
	case errors.Is(err, rootkey.ErrMissingWithVault):
		return fault.Wrap(
			fault.KindRejected, "root_key_missing_with_vault",
			"The installation root key is missing while encrypted Context vaults exist.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Restore the original root key or explicitly remove local authentication state."},
		)
	case errors.Is(err, rootkey.ErrUnsafe):
		return fault.Wrap(
			fault.KindRejected, "root_key_unsafe",
			"The root-key or Auth Broker state path is unsafe.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect XDG ownership, modes, and symbolic links."},
		)
	case errors.Is(err, rootkey.ErrDenied):
		return fault.Wrap(
			fault.KindUnavailable, "keychain_denied",
			"macOS Keychain access to the Tobari root key was denied.", false, err,
			fault.NextAction{Command: "cluster up", Reason: "Allow Keychain access and retry the cluster reconcile."},
		)
	default:
		return fault.Wrap(
			fault.KindUnavailable, "root_key_unavailable",
			"The installation root-key provider is unavailable.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the host root-key backend."},
		)
	}
}

func classifyBrokerError(err error, _ string) error {
	if public, ok := fault.PublicCopy(err); ok {
		return public
	}
	var unknown brokerMutationOutcomeUnknown
	if errors.As(err, &unknown) {
		return fault.New(
			fault.KindContract,
			"auth_mutation_outcome_unknown",
			"The Auth Broker mutation may have completed, but its final acknowledgement was not received.",
			false,
			fault.NextAction{Command: "auth status", Reason: "Reconcile the selected Context's credential state before another mutation."},
		)
	}
	var protocol brokerControlError
	if errors.As(err, &protocol) {
		switch protocol.Code {
		case "locked":
			return fault.New(
				fault.KindUnavailable, "auth_broker_locked", "The Auth Broker is locked.", false,
				fault.NextAction{Command: "cluster up", Reason: "Unlock it through cluster reconciliation."},
			)
		case "vault_integrity_failed", "vault_invalid", "vault_path_invalid":
			return fault.New(
				fault.KindRejected, "auth_vault_invalid", "The encrypted Context vault is invalid or could not be authenticated.", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect the Context vault without printing its contents."},
			)
		case "vault_version_unsupported":
			return fault.New(
				fault.KindUnsupported, "auth_vault_version_unsupported", "The encrypted Context vault version is unsupported.", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect the installed Tobari version and vault contract."},
			)
		case "credential_not_found":
			return fault.New(
				fault.KindNotFound, "provider_credential_not_configured", "The provider credential is not configured for this Context.", false,
				fault.NextAction{Command: "auth status", Reason: "Inspect Context authentication before entering the Workspace."},
			)
		case "handle_not_found", "handle_revoked":
			return fault.New(
				fault.KindRejected, "auth_handle_stale", "The Workspace authentication handle is stale or invalid.", false,
				fault.NextAction{Command: "tobari", Reason: "Leave and re-enter the Workspace to receive the current authentication revision."},
			)
		case "handle_binding_mismatch":
			return fault.New(fault.KindRejected, "auth_handle_binding_mismatch", "The Workspace authentication handle does not match its Context, project, or HTTP binding.", false)
		}
	}
	return fault.Wrap(
		fault.KindUnavailable, "auth_broker_request_failed", "The Auth Broker request could not be completed.", false, err,
		fault.NextAction{Command: "cluster up", Reason: "Reconcile the shared cluster and retry."},
	)
}

func authStorageBackend() (authbroker.StorageBackend, error) {
	switch runtime.GOOS {
	case "darwin":
		return authbroker.StorageBackendMacOSKeychain, nil
	case "linux":
		return authbroker.StorageBackendXDGFile, nil
	default:
		return "", fmt.Errorf("unsupported root-key host platform %q", runtime.GOOS)
	}
}

func (r *Runtime) resolveAuthContext(ctx context.Context, contextName string) (tobari.ContextManifest, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextManifest{}, err
	}
	manifest, _, err := r.resolveContext(contextName)
	if errors.Is(err, tobari.ErrContextNotFound) {
		return tobari.ContextManifest{}, fault.New(
			fault.KindNotFound, "context_not_found", "The selected Context does not exist.", false,
			fault.NextAction{Command: "context list", Reason: "Choose an existing Context before using authentication."},
		)
	}
	return manifest, err
}

func renderProviderTemplate(template, handle string, provider authbroker.Provider) string {
	replacer := strings.NewReplacer(
		"${HANDLE}", handle,
		"${PROVIDER_ID}", provider.ID,
		"${DISPLAY_NAME}", provider.DisplayName,
	)
	return replacer.Replace(template)
}

func (r *Runtime) addAuthDiagnostics(
	ctx context.Context,
	add func(string, doctor.CheckStatus, string),
) {
	projection, providerErr := r.loadAuthProviders()
	if providerErr != nil {
		add("auth_provider_manifests", doctor.CheckStatusFail, "credential-provider manifests are invalid or unsafe")
	} else {
		add("auth_provider_manifests", doctor.CheckStatusPass, fmt.Sprintf("%d credential-provider manifests normalize to projection schema V1", len(projection.Providers)))
	}

	vaultsExist, vaultErr := rootkey.EncryptedStateExists(r.stateDirectory)
	if vaultErr != nil {
		add("auth_vault_paths", doctor.CheckStatusFail, "Auth Broker vault paths are unsafe")
	} else if vaultsExist {
		add("auth_vault_paths", doctor.CheckStatusPass, "encrypted Context vault paths are owner-only")
	} else {
		add("auth_vault_paths", doctor.CheckStatusPass, "no encrypted Context vault is present")
	}

	provider, rootErr := rootkey.New(r.stateDirectory)
	if rootErr == nil && vaultErr == nil {
		backend, exists, inspectErr := provider.Inspect(ctx, vaultsExist)
		switch {
		case inspectErr != nil:
			add("auth_root_key", doctor.CheckStatusFail, "the "+string(backend)+" root-key backend is unavailable or inconsistent with encrypted state")
		case exists:
			add("auth_root_key", doctor.CheckStatusPass, "the "+string(backend)+" installation root key is available")
		default:
			add("auth_root_key", doctor.CheckStatusWarn, "the "+string(backend)+" installation root key will be created by cluster up")
		}
	} else {
		add("auth_root_key", doctor.CheckStatusFail, "the installation root-key backend is unavailable")
	}

	brokerState, brokerErr := r.brokerState(ctx)
	if brokerErr != nil || brokerState == authbroker.BrokerStateUnavailable {
		add("auth_broker", doctor.CheckStatusWarn, "Auth Broker is unavailable; run cluster up to reconcile it")
		return
	}
	if brokerState == authbroker.BrokerStateLocked {
		add("auth_broker", doctor.CheckStatusFail, "Auth Broker is locked; run cluster up to unlock it")
		return
	}
	add("auth_broker", doctor.CheckStatusPass, "Auth Broker is healthy and unlocked")
	companionState, _, companionErr := r.credentialCompanionStatus(ctx)
	if companionErr != nil || companionState != "ready" {
		add("credential_companion", doctor.CheckStatusWarn, "trusted-host credential refresh is unavailable; run cluster up to reconcile the companion")
	} else {
		add("credential_companion", doctor.CheckStatusPass, "trusted-host credential companion is authenticated and ready")
	}
	if providerErr != nil {
		return
	}
	contexts, err := r.ListContexts(ctx)
	if err != nil {
		add("auth_vault_integrity", doctor.CheckStatusFail, "Context identities could not be inspected")
		return
	}
	configured := make(map[string]map[string]projectAuthProviderBinding, len(contexts.Items))
	encodedBindings := make(map[string][]byte, len(projection.Providers))
	for _, item := range contexts.Items {
		configured[item.ID] = make(map[string]projectAuthProviderBinding)
		for _, provider := range projection.Providers {
			response, statusErr := r.runBrokerControl(
				ctx, nil, "status", "--context-id", item.ID, "--provider", provider.ID,
			)
			if statusErr != nil || response.Provider != provider.ID {
				add("auth_vault_integrity", doctor.CheckStatusFail, "an encrypted Context vault could not be authenticated")
				return
			}
			switch response.State {
			case "not_configured":
			case "ready":
				_, encoded, digest, bindingErr := brokerBindingsForProvider(projection, provider.ID)
				if bindingErr != nil || !validAuthRevision(response.Revision) {
					add("auth_vault_integrity", doctor.CheckStatusFail, "Context credential metadata is inconsistent with the provider projection")
					return
				}
				encodedBindings[provider.ID] = encoded
				configured[item.ID][provider.ID] = projectAuthProviderBinding{
					Provider: provider.ID, Revision: response.Revision, BindingDigest: digest,
				}
			default:
				add("auth_vault_integrity", doctor.CheckStatusFail, "Auth Broker returned an invalid Context credential state")
				return
			}
		}
	}
	add("auth_vault_integrity", doctor.CheckStatusPass, "encrypted Context vaults are readable without exposing contents")

	projects, projectErr := r.ListProjects(ctx)
	if projectErr != nil {
		add("auth_project_handles", doctor.CheckStatusFail, "project-bound authentication state could not be inspected")
		return
	}
	stale := 0
	for _, project := range projects {
		registry, registryErr := r.readProjectAuthRegistry(project.ID)
		if registryErr != nil {
			add("auth_project_handles", doctor.CheckStatusFail, "a project authentication ownership record is unsafe or invalid")
			return
		}
		observed := make(map[string]projectAuthProviderBinding, len(registry.Providers))
		for _, binding := range registry.Providers {
			observed[binding.Provider] = binding
		}
		expected := configured[project.ContextID]
		for providerID, binding := range expected {
			current, exists := observed[providerID]
			if !exists || current.Revision != binding.Revision || current.BindingDigest != binding.BindingDigest {
				stale++
				delete(observed, providerID)
				continue
			}
			response, statusErr := r.runBrokerControl(
				ctx,
				nil,
				"binding_status",
				"--context-id", project.ContextID,
				"--project-id", project.ID,
				"--provider", providerID,
				"--revision", current.Revision,
				"--bindings", string(encodedBindings[providerID]),
			)
			if statusErr != nil {
				add("auth_project_handles", doctor.CheckStatusFail, "project-bound authentication state could not be verified")
				return
			}
			switch response.State {
			case "ready":
			case "missing", "stale":
				stale++
			default:
				add("auth_project_handles", doctor.CheckStatusFail, "project-bound authentication state could not be verified")
				return
			}
			delete(observed, providerID)
		}
		stale += len(observed)
	}
	if stale != 0 {
		add("auth_project_handles", doctor.CheckStatusWarn, fmt.Sprintf("%d project authentication bindings require the next matching tobari entry", stale))
		return
	}
	add("auth_project_handles", doctor.CheckStatusPass, "project authentication bindings match current Context credentials and provider manifests")
}
