package authcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type authRuntimeFake struct {
	result             authbroker.MutationObservation
	statusResult       authbroker.StatusObservation
	err                error
	inputTerminal      bool
	inputTerminalCalls int
	errorTerminal      bool
	errorTerminalCalls int
	loginCalls         int
	importCalls        int
	statusCalls        int
	logoutCalls        int
	secret             []byte
	contextName        string
	provider           string
	method             string
}

func (f *authRuntimeFake) IsInputTerminal(io.Reader) bool {
	f.inputTerminalCalls++
	return f.inputTerminal
}

func (f *authRuntimeFake) IsTerminal(io.Writer) bool {
	f.errorTerminalCalls++
	return f.errorTerminal
}

func (f *authRuntimeFake) LoginAuth(
	_ context.Context, contextName, provider, method string, _ io.Reader, _ io.Writer,
) (authbroker.MutationObservation, error) {
	f.loginCalls++
	f.contextName, f.provider, f.method = contextName, provider, method
	return f.result, f.err
}

func (f *authRuntimeFake) ImportAuth(_ context.Context, contextName, provider string, input io.Reader) (authbroker.MutationObservation, error) {
	f.importCalls++
	f.contextName, f.provider = contextName, provider
	f.secret, _ = io.ReadAll(input)
	return f.result, f.err
}

func (f *authRuntimeFake) AuthStatus(_ context.Context, contextName string) (authbroker.StatusObservation, error) {
	f.statusCalls++
	f.contextName = contextName
	return f.statusResult, f.err
}

func (f *authRuntimeFake) LogoutAuth(_ context.Context, contextName, provider string) (authbroker.MutationObservation, error) {
	f.logoutCalls++
	f.contextName, f.provider = contextName, provider
	return f.result, f.err
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) {
	panic("credential stdin was read")
}

type countingReader struct {
	reader    io.Reader
	read      int
	readCalls int
}

func (r *countingReader) Read(data []byte) (int, error) {
	r.readCalls++
	count, err := r.reader.Read(data)
	r.read += count
	return count, err
}

func validAuthResult(task string) authbroker.Result {
	return validAuthResultForProvider(task, BuiltinGitHubProviderID)
}

func validAuthResultForProvider(task, provider string) authbroker.Result {
	label := "octocat"
	activation, err := authbroker.NewWorkspaceActivation(
		"default", "018bcfe5-687b-7000-8000-000000000099", []authbroker.WorkspaceActivationItem{},
	)
	if err != nil {
		panic(err)
	}
	return authbroker.Result{
		ContextState: tobari.ContextObservationPersisted,
		Task:         task, Provider: provider, Context: "default",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", Configured: true,
		AccountLabel: &label, StorageBackend: authbroker.StorageBackendXDGFile,
		BrokerState:         authbroker.BrokerStateReady,
		CredentialRevision:  strings.Repeat("a", 64),
		Change:              authbroker.MutationChangeChanged,
		WorkspaceActivation: activation,
	}
}

func unconfiguredAuthResult(task, provider string) authbroker.Result {
	activation := authbroker.NotApplicableWorkspaceActivation()
	change := authbroker.MutationChangeNoChange
	if task == authbroker.TaskLogout {
		var err error
		activation, err = authbroker.NewWorkspaceActivation(
			"default", "018bcfe5-687b-7000-8000-000000000099", []authbroker.WorkspaceActivationItem{},
		)
		if err != nil {
			panic(err)
		}
		change = authbroker.MutationChangeChanged
	}
	return authbroker.Result{
		ContextState: tobari.ContextObservationPersisted,
		Task:         task, Provider: provider, Context: "default",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", Configured: false,
		StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady,
		Change: change, WorkspaceActivation: activation,
	}
}

func authStatusResult(contextName string, configured bool) authbroker.StatusResult {
	var label *string
	revision := ""
	state := authbroker.ProviderCredentialNotConfigured
	if configured {
		value := "octocat"
		label = &value
		revision = strings.Repeat("c", 64)
		state = authbroker.ProviderCredentialConfigured
	}
	activation, err := authbroker.NewWorkspaceActivation(
		contextName, "018bcfe5-687b-7000-8000-000000000099", []authbroker.WorkspaceActivationItem{},
	)
	if err != nil {
		panic(err)
	}
	return authbroker.StatusResult{
		Task: authbroker.TaskStatus, ContextState: tobari.ContextObservationPersisted, Context: contextName,
		ContextID:      "018bcfe5-687b-7000-8000-000000000099",
		StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady,
		Providers: []authbroker.ProviderStatus{{
			Provider: BuiltinGitHubProviderID, State: state,
			AccountLabel: label, CredentialRevision: revision,
		}},
		WorkspaceActivation: activation,
	}
}

func mutationObservation(result authbroker.Result) authbroker.MutationObservation {
	coverage := result.WorkspaceActivation.Coverage
	return authbroker.MutationObservation{
		ContextState: result.ContextState, Provider: result.Provider, Context: result.Context, ContextID: result.ContextID,
		Configured: result.Configured, AccountLabel: result.AccountLabel, StorageBackend: result.StorageBackend,
		BrokerState: result.BrokerState, CredentialRevision: result.CredentialRevision,
		Changed: result.Change == authbroker.MutationChangeChanged, Providers: []authbroker.ProviderStatus{},
		Workspaces: authbroker.WorkspaceObservation{Coverage: coverage, Workspaces: []authbroker.WorkspaceProjectionObservation{}},
	}
}

func statusObservation(result authbroker.StatusResult) authbroker.StatusObservation {
	return authbroker.StatusObservation{
		ContextState: result.ContextState, Context: result.Context, ContextID: result.ContextID,
		StorageBackend: result.StorageBackend, BrokerState: result.BrokerState,
		Providers: append([]authbroker.ProviderStatus{}, result.Providers...),
		Workspaces: authbroker.WorkspaceObservation{
			Coverage: result.WorkspaceActivation.Coverage, Workspaces: []authbroker.WorkspaceProjectionObservation{},
		},
	}
}

func authIntent(command string) operation.Intent {
	return operation.Intent{
		Command: command, Effect: operation.EffectWrite,
		Target: operation.TargetRef{
			Kind: authbroker.CredentialCatalogTargetKind,
			ID:   authbroker.CredentialCatalogTargetID,
		},
		Impact: MutationImpact(),
	}
}

func TestImportValidatesIntentBeforeReadingStdinOrCallingRuntime(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport))}
	service := New(fake)
	intent := authIntent("auth import")
	intent.Target.ID = "wrong-target"

	_, err := service.Import(context.Background(), intent, "default", BuiltinGitHubProviderID, panicReader{})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_mutation_contract" || public.Retryable {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if fake.importCalls != 0 {
		t.Fatalf("ImportAuth() calls = %d, want 0", fake.importCalls)
	}
}

func TestImportRejectsAlteredCompleteImpactBeforeReadingStdin(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport))}
	service := New(fake)
	intent := authIntent("auth import")
	intent.Impact.Destructive = operation.DeclarationNo

	_, err := service.Import(context.Background(), intent, "default", BuiltinGitHubProviderID, panicReader{})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_mutation_contract" || public.Retryable {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if fake.importCalls != 0 {
		t.Fatalf("ImportAuth() calls = %d, want 0", fake.importCalls)
	}
}

func TestImportCancellationBeforeActionReadsNoStdin(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport))}
	service := New(fake)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := service.Import(ctx, authIntent("auth import"), "default", BuiltinGitHubProviderID, panicReader{})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "operation_canceled" || !public.Retryable {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if fake.importCalls != 0 {
		t.Fatalf("ImportAuth() calls = %d, want 0", fake.importCalls)
	}
}

func TestImportBoundsAndForwardsCredentialOnlyAfterValidation(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport))}
	service := New(fake)
	const secret = "synthetic-private-token"

	result, err := service.Import(
		context.Background(), authIntent("auth import"), "default", BuiltinGitHubProviderID,
		strings.NewReader(secret),
	)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	if result.Task != authbroker.TaskImport || fake.inputTerminalCalls != 1 || fake.importCalls != 1 ||
		fake.contextName != "default" || fake.provider != BuiltinGitHubProviderID || string(fake.secret) != secret {
		t.Fatalf("result/call = %+v, calls=%d context=%q provider=%q secret=%q", result, fake.importCalls, fake.contextName, fake.provider, fake.secret)
	}
}

func TestImportRejectsInteractiveTerminalBeforeReadOrRuntimeMutation(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport)), inputTerminal: true}
	service := New(fake)
	input := &countingReader{reader: strings.NewReader("synthetic-private-token")}

	_, err := service.Import(
		context.Background(), authIntent("auth import"), "default", BuiltinGitHubProviderID, input,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_credential_input" || public.Retryable {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if fake.inputTerminalCalls != 1 || input.readCalls != 0 || fake.importCalls != 0 || len(fake.secret) != 0 {
		t.Fatalf(
			"terminal checks/reads/runtime calls/secret = %d/%d/%d/%q",
			fake.inputTerminalCalls, input.readCalls, fake.importCalls, fake.secret,
		)
	}
	if strings.Contains(err.Error(), "synthetic-private-token") {
		t.Fatalf("Import() echoed credential input: %q", err)
	}
}

func TestImportRejectsOversizedCredentialBeforeRuntime(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskImport))}
	service := New(fake)
	input := &countingReader{reader: bytes.NewReader(bytes.Repeat([]byte{'x'}, authbroker.MaxPrimarySecretBytes+128))}

	_, err := service.Import(context.Background(), authIntent("auth import"), "default", BuiltinGitHubProviderID, input)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "invalid_credential_input" {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if fake.importCalls != 0 || input.read > authbroker.MaxPrimarySecretBytes+1 {
		t.Fatalf("ImportAuth() calls = %d, stdin bytes read = %d", fake.importCalls, input.read)
	}
}

func TestImportCollapsesUnknownPostActionOutcomeAndHidesSecret(t *testing.T) {
	const canary = "private-provider-canary"
	fake := &authRuntimeFake{err: errors.New(canary)}
	service := New(fake)

	_, err := service.Import(
		context.Background(), authIntent("auth import"), "default", BuiltinGitHubProviderID,
		strings.NewReader("private-input-canary"),
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindContract || public.Code != "unclassified_mutation_outcome" || public.Retryable {
		t.Fatalf("Import() fault = %+v, ok=%t", public, ok)
	}
	if strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "private-input-canary") {
		t.Fatalf("Import() exposed secret-bearing error: %q", err)
	}
}

func TestImportPreservesTypedBrokerFaultWithoutPrivateCause(t *testing.T) {
	const canary = "private-broker-cause"
	fake := &authRuntimeFake{err: fault.Wrap(
		fault.KindUnavailable, "auth_broker_unavailable", "The Auth Broker is unavailable.", true, errors.New(canary),
	)}
	service := New(fake)

	_, err := service.Import(
		context.Background(), authIntent("auth import"), "default", BuiltinGitHubProviderID,
		strings.NewReader("private-input-canary"),
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_broker_unavailable" || strings.Contains(err.Error(), canary) || errors.Unwrap(err) != nil {
		t.Fatalf("Import() fault = %+v, err=%#v", public, err)
	}
}

func TestLoginRejectsUnsupportedHelperBeforeRuntime(t *testing.T) {
	fake := &authRuntimeFake{result: mutationObservation(validAuthResult(authbroker.TaskLogin))}
	service := New(fake)
	_, err := service.Login(
		context.Background(), authIntent("auth login"), "default", "example", "", strings.NewReader(""), io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindUnsupported || public.Code != "provider_login_unsupported" {
		t.Fatalf("Login() fault = %+v, ok=%t", public, ok)
	}
	if fake.loginCalls != 0 {
		t.Fatalf("LoginAuth() calls = %d, want 0", fake.loginCalls)
	}
}

func TestLoginSupportsReviewedBuiltinHelpers(t *testing.T) {
	if BuiltinGitHubProviderID != "github" || BuiltinAWSProviderID != "aws" ||
		BuiltinDatadogProviderID != "datadog" || BuiltinOpenAIProviderID != "openai" ||
		BuiltinAnthropicProviderID != "anthropic" {
		t.Fatalf(
			"built-in provider IDs = %q/%q/%q/%q/%q",
			BuiltinGitHubProviderID, BuiltinAWSProviderID, BuiltinDatadogProviderID,
			BuiltinOpenAIProviderID, BuiltinAnthropicProviderID,
		)
	}
	for _, provider := range authbroker.ReviewedLoginProviderIDs() {
		t.Run(provider, func(t *testing.T) {
			fake := &authRuntimeFake{
				result:        mutationObservation(validAuthResultForProvider(authbroker.TaskLogin, provider)),
				inputTerminal: true,
				errorTerminal: true,
			}
			result, err := New(fake).Login(
				context.Background(), authIntent("auth login"), "default", provider, "",
				strings.NewReader(""), io.Discard,
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Provider != provider || fake.provider != provider || fake.loginCalls != 1 {
				t.Fatalf("result/provider/calls = %+v/%q/%d", result, fake.provider, fake.loginCalls)
			}
			wantMethod := ""
			if provider == BuiltinAWSProviderID {
				wantMethod = string(LoginMethodIdentityCenter)
			}
			if fake.method != wantMethod {
				t.Fatalf("login method = %q, want %q", fake.method, wantMethod)
			}
		})
	}
}

func TestLoginRejectsAWSMethodForEveryNonAWSBuiltinBeforeTerminalInspection(t *testing.T) {
	for _, provider := range authbroker.ReviewedLoginProviderIDs() {
		if provider == BuiltinAWSProviderID {
			continue
		}
		t.Run(provider, func(t *testing.T) {
			fake := &authRuntimeFake{inputTerminal: true, errorTerminal: true}
			_, err := New(fake).Login(
				context.Background(), authIntent("auth login"), "default", provider,
				string(LoginMethodConsole), strings.NewReader(""), io.Discard,
			)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "auth_login_method_not_applicable" || fake.loginCalls != 0 ||
				fake.inputTerminalCalls != 0 || fake.errorTerminalCalls != 0 {
				t.Fatalf(
					"provider %q method fault/calls = %+v/%d/%d/%d",
					provider, public, fake.loginCalls, fake.inputTerminalCalls, fake.errorTerminalCalls,
				)
			}
		})
	}
}

func TestLoginSelectsConsoleAndRejectsMethodForGitHubBeforeRuntime(t *testing.T) {
	fake := &authRuntimeFake{
		result:        mutationObservation(validAuthResultForProvider(authbroker.TaskLogin, BuiltinAWSProviderID)),
		inputTerminal: true, errorTerminal: true,
	}
	_, err := New(fake).Login(
		context.Background(), authIntent("auth login"), "default", BuiltinAWSProviderID,
		string(LoginMethodConsole), strings.NewReader(""), io.Discard,
	)
	if err != nil || fake.loginCalls != 1 || fake.method != string(LoginMethodConsole) {
		t.Fatalf("console login error/calls/method = %v/%d/%q", err, fake.loginCalls, fake.method)
	}

	fake = &authRuntimeFake{inputTerminal: true, errorTerminal: true}
	_, err = New(fake).Login(
		context.Background(), authIntent("auth login"), "default", BuiltinGitHubProviderID,
		string(LoginMethodConsole), strings.NewReader(""), io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "auth_login_method_not_applicable" || fake.loginCalls != 0 ||
		fake.inputTerminalCalls != 0 || fake.errorTerminalCalls != 0 {
		t.Fatalf("GitHub method fault/calls = %+v/%d/%d/%d", public, fake.loginCalls, fake.inputTerminalCalls, fake.errorTerminalCalls)
	}
}

func TestLoginRequiresInputAndErrorTerminalsBeforeRuntime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		inputTerminal bool
		errorTerminal bool
		errorChecks   int
	}{
		{name: "stdin is redirected", inputTerminal: false, errorTerminal: true, errorChecks: 0},
		{name: "stderr is redirected", inputTerminal: true, errorTerminal: false, errorChecks: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &authRuntimeFake{
				result: mutationObservation(validAuthResult(authbroker.TaskLogin)), inputTerminal: test.inputTerminal, errorTerminal: test.errorTerminal,
			}
			_, err := New(fake).Login(
				context.Background(), authIntent("auth login"), "default", BuiltinGitHubProviderID, "",
				strings.NewReader(""), io.Discard,
			)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Kind != fault.KindInvalidInput || public.Code != "auth_login_tty_required" || public.Retryable {
				t.Fatalf("terminal fault = %+v, ok=%t", public, ok)
			}
			if fake.loginCalls != 0 || fake.inputTerminalCalls != 1 || fake.errorTerminalCalls != test.errorChecks {
				t.Fatalf(
					"login/input-terminal/error-terminal calls = %d/%d/%d",
					fake.loginCalls, fake.inputTerminalCalls, fake.errorTerminalCalls,
				)
			}
			if len(public.NextActions) != 1 || public.NextActions[0].Command != "help auth login" {
				t.Fatalf("next actions = %+v", public.NextActions)
			}
			if strings.Contains(public.Message, "GitHub") || strings.Contains(public.NextActions[0].Reason, "GitHub") {
				t.Fatalf("terminal fault is provider-specific: %+v", public)
			}
		})
	}
}

func TestLoginAndLogoutUseOneFixedMutationBoundary(t *testing.T) {
	tests := []struct {
		name    string
		task    string
		command string
		call    func(*Service, operation.Intent) (authbroker.Result, error)
		calls   func(*authRuntimeFake) int
		result  authbroker.Result
	}{
		{
			name: "login", task: authbroker.TaskLogin, command: "auth login",
			call: func(service *Service, intent operation.Intent) (authbroker.Result, error) {
				return service.Login(
					context.Background(), intent, "default", BuiltinGitHubProviderID, "", strings.NewReader(""), io.Discard,
				)
			},
			calls:  func(fake *authRuntimeFake) int { return fake.loginCalls },
			result: validAuthResult(authbroker.TaskLogin),
		},
		{
			name: "logout", task: authbroker.TaskLogout, command: "auth logout",
			call: func(service *Service, intent operation.Intent) (authbroker.Result, error) {
				return service.Logout(context.Background(), intent, "default", BuiltinGitHubProviderID)
			},
			calls:  func(fake *authRuntimeFake) int { return fake.logoutCalls },
			result: unconfiguredAuthResult(authbroker.TaskLogout, BuiltinGitHubProviderID),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			login := test.name == "login"
			fake := &authRuntimeFake{result: mutationObservation(test.result), inputTerminal: login, errorTerminal: login}
			result, err := test.call(New(fake), authIntent(test.command))
			if err != nil {
				t.Fatalf("%s error = %v", test.name, err)
			}
			if result.Task != test.task || test.calls(fake) != 1 || fake.provider != BuiltinGitHubProviderID {
				t.Fatalf("%s result/calls/provider = %+v/%d/%q", test.name, result, test.calls(fake), fake.provider)
			}
		})
	}
}

func TestStatusRejectsResultForAnotherRequestedContext(t *testing.T) {
	fake := &authRuntimeFake{statusResult: statusObservation(authStatusResult("default", true))}
	service := New(fake)
	_, err := service.Status(context.Background(), "restricted")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindContract || public.Code != "invalid_auth_result" {
		t.Fatalf("Status() fault = %+v, ok=%t", public, ok)
	}
	if fake.statusCalls != 1 {
		t.Fatalf("AuthStatus() calls = %d", fake.statusCalls)
	}
}

func TestStatusPreservesExplicitUnconfiguredState(t *testing.T) {
	fake := &authRuntimeFake{statusResult: statusObservation(authStatusResult("default", false))}
	result, err := New(fake).Status(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Providers) != 1 || result.Providers[0].State != authbroker.ProviderCredentialNotConfigured ||
		result.Providers[0].AccountLabel != nil ||
		result.Providers[0].CredentialRevision != "" || result.WorkspaceActivation.State != authbroker.WorkspaceActivationNotApplicable {
		t.Fatalf("unconfigured status = %+v", result)
	}
}

func TestStatusPreservesUnavailableProviderStateWhenBrokerIsLocked(t *testing.T) {
	status := authStatusResult("default", false)
	status.BrokerState = authbroker.BrokerStateLocked
	status.Providers[0].State = authbroker.ProviderCredentialUnavailable
	fake := &authRuntimeFake{statusResult: statusObservation(status)}
	result, err := New(fake).Status(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if result.BrokerState != authbroker.BrokerStateLocked ||
		result.Providers[0].State != authbroker.ProviderCredentialUnavailable {
		t.Fatalf("locked status = %+v", result)
	}
}

func TestStatusPreservesUnavailableProviderStateWhenBrokerIsAbsent(t *testing.T) {
	status := authStatusResult("default", false)
	status.BrokerState = authbroker.BrokerStateUnavailable
	status.Providers[0].State = authbroker.ProviderCredentialUnavailable
	fake := &authRuntimeFake{statusResult: statusObservation(status)}
	result, err := New(fake).Status(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	if result.BrokerState != authbroker.BrokerStateUnavailable ||
		result.Providers[0].State != authbroker.ProviderCredentialUnavailable {
		t.Fatalf("unavailable status = %+v", result)
	}
}
