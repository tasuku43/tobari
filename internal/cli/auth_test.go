package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/authcmd"
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

type authCLIRuntime struct {
	result             authbroker.Result
	statusResult       authbroker.StatusResult
	err                error
	secret             []byte
	contextName        string
	provider           string
	method             string
	statusCalls        int
	importCalls        int
	loginCalls         int
	inputTerminal      bool
	inputTerminalCalls int
	errorTerminal      bool
	errorTerminalCalls int
}

func (r *authCLIRuntime) IsInputTerminal(io.Reader) bool {
	r.inputTerminalCalls++
	return r.inputTerminal
}

func (r *authCLIRuntime) IsTerminal(io.Writer) bool {
	r.errorTerminalCalls++
	return r.errorTerminal
}

type authPanicReader struct{}

func (authPanicReader) Read([]byte) (int, error) {
	panic("credential stdin was read")
}

type authCountingReader struct {
	reader    io.Reader
	readCalls int
}

func (r *authCountingReader) Read(data []byte) (int, error) {
	r.readCalls++
	return r.reader.Read(data)
}

func (r *authCLIRuntime) LoginAuth(
	_ context.Context, contextName, provider, method string, _ io.Reader, _ io.Writer,
) (authbroker.Result, error) {
	r.loginCalls++
	r.contextName, r.provider, r.method = contextName, provider, method
	return r.result, r.err
}

func (r *authCLIRuntime) ImportAuth(_ context.Context, contextName, provider string, input io.Reader) (authbroker.Result, error) {
	r.importCalls++
	r.contextName, r.provider = contextName, provider
	r.secret, _ = io.ReadAll(input)
	return r.result, r.err
}

func (r *authCLIRuntime) AuthStatus(_ context.Context, contextName string) (authbroker.StatusResult, error) {
	r.statusCalls++
	r.contextName = contextName
	return r.statusResult, r.err
}

func (r *authCLIRuntime) LogoutAuth(_ context.Context, contextName, provider string) (authbroker.Result, error) {
	r.contextName, r.provider = contextName, provider
	return r.result, r.err
}

func authCLIResult(task string) authbroker.Result {
	return authCLIResultForProvider(task, authcmd.BuiltinGitHubProviderID)
}

func authCLIResultForProvider(task, provider string) authbroker.Result {
	label := "octocat"
	return authbroker.Result{
		Task: task, Provider: provider, Context: "default",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", Configured: true,
		AccountLabel: &label, StorageBackend: authbroker.StorageBackendXDGFile,
		BrokerState:        authbroker.BrokerStateReady,
		CredentialRevision: strings.Repeat("b", 64),
		WorkspaceActivation: authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: authbroker.ContextAuthActivationGuidance,
		},
	}
}

func authCLIStatusResult(configured bool) authbroker.StatusResult {
	var label *string
	revision := ""
	state := authbroker.ProviderCredentialNotConfigured
	if configured {
		value := "octocat"
		label = &value
		revision = strings.Repeat("d", 64)
		state = authbroker.ProviderCredentialConfigured
	}
	activation := authbroker.WorkspaceActivation{State: authbroker.WorkspaceActivationNotApplicable}
	if configured {
		activation = authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: authbroker.ContextAuthActivationGuidance,
		}
	}
	return authbroker.StatusResult{
		Task: authbroker.TaskStatus, Context: "default",
		ContextID:      "018bcfe5-687b-7000-8000-000000000099",
		StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady,
		Providers: []authbroker.ProviderStatus{{
			Provider: authcmd.BuiltinGitHubProviderID, State: state, Configured: configured,
			AccountLabel: label, CredentialRevision: revision,
		}},
		WorkspaceActivation: activation,
	}
}

func authCLILogoutResult() authbroker.Result {
	return authbroker.Result{
		Task: authbroker.TaskLogout, Provider: authcmd.BuiltinGitHubProviderID, Context: "default",
		ContextID: "018bcfe5-687b-7000-8000-000000000099", Configured: false,
		StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady,
		WorkspaceActivation: authbroker.WorkspaceActivation{
			State:    authbroker.WorkspaceActivationReentryRequired,
			Guidance: authbroker.ContextAuthRemovalGuidance,
		},
	}
}

func newAuthCLI(input io.Reader, runtime *authCLIRuntime) (*CLI, *bytes.Buffer, *bytes.Buffer) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	command := newCLI(input, stdout, stderr, DefaultCatalog(), passingInspector("unused"))
	command.auth = authcmd.New(runtime)
	return command, stdout, stderr
}

func TestAuthImportReadsSecretOnlyFromStdinAndEmitsSecretFreeJSON(t *testing.T) {
	const secret = "synthetic-secret-canary"
	runtime := &authCLIRuntime{result: authCLIResult(authbroker.TaskImport)}
	command, stdout, stderr := newAuthCLI(strings.NewReader(secret), runtime)

	if code := runCLI(command, []string{"auth", "import", "github", "--format=json"}); code != ExitOK {
		t.Fatalf("auth import code = %d, stderr = %q", code, stderr.String())
	}
	if string(runtime.secret) != secret || runtime.provider != "github" || runtime.contextName != "" ||
		runtime.importCalls != 1 || runtime.inputTerminalCalls != 1 {
		t.Fatalf("runtime secret/provider/context = %q/%q/%q", runtime.secret, runtime.provider, runtime.contextName)
	}
	if strings.Contains(stdout.String(), secret) || strings.Contains(stderr.String(), secret) || stderr.Len() != 0 {
		t.Fatalf("secret appeared in output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	var document map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if string(document["schema_version"]) != "1" {
		t.Fatalf("schema_version = %s", document["schema_version"])
	}
	var auth map[string]json.RawMessage
	if err := json.Unmarshal(document["auth"], &auth); err != nil {
		t.Fatal(err)
	}
	gotFields := make([]string, 0, len(auth))
	for field := range auth {
		gotFields = append(gotFields, field)
	}
	sort.Strings(gotFields)
	wantFields := []string{
		"account_label", "broker_state", "configured", "context", "context_id",
		"credential_revision", "provider", "storage_backend", "workspace_activation",
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("auth fields = %v, want %v", gotFields, wantFields)
	}
}

func TestAuthImportRejectsInteractiveTerminalBeforeReadOrRuntimeMutation(t *testing.T) {
	runtime := &authCLIRuntime{result: authCLIResult(authbroker.TaskImport), inputTerminal: true}
	input := &authCountingReader{reader: strings.NewReader("synthetic-secret-canary")}
	command, stdout, stderr := newAuthCLI(input, runtime)

	if code := runCLI(command, []string{"auth", "import", "github"}); code != ExitUsage {
		t.Fatalf("auth import code = %d, stderr = %q", code, stderr.String())
	}
	if input.readCalls != 0 || runtime.importCalls != 0 || len(runtime.secret) != 0 || stdout.Len() != 0 {
		t.Fatalf(
			"terminal stdin reads/runtime calls/secret/stdout = %d/%d/%q/%q",
			input.readCalls, runtime.importCalls, runtime.secret, stdout.String(),
		)
	}
	if runtime.inputTerminalCalls != 1 || !strings.Contains(stderr.String(), "code: invalid_credential_input") ||
		strings.Contains(stderr.String(), "synthetic-secret-canary") {
		t.Fatalf("terminal stdin check/error = %d/%q", runtime.inputTerminalCalls, stderr.String())
	}
}

func TestAuthLoginAndLogoutDispatchThroughFixedMutationHandlers(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		provider string
		method   string
		result   authbroker.Result
	}{
		{name: "github login", args: []string{"auth", "login", "github", "--format=json"}, provider: "github", result: authCLIResult(authbroker.TaskLogin)},
		{name: "aws login", args: []string{"auth", "login", "aws", "--format=json"}, provider: "aws", method: "identity-center", result: authCLIResultForProvider(authbroker.TaskLogin, "aws")},
		{name: "aws console login", args: []string{"auth", "login", "aws", "--method=console", "--format=json"}, provider: "aws", method: "console", result: authCLIResultForProvider(authbroker.TaskLogin, "aws")},
		{name: "logout", args: []string{"auth", "logout", "github", "--format=json"}, provider: "github", result: authCLILogoutResult()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			login := strings.Contains(test.name, "login")
			runtime := &authCLIRuntime{result: test.result, inputTerminal: login, errorTerminal: login}
			command, stdout, stderr := newAuthCLI(strings.NewReader(""), runtime)
			if code := runCLI(command, test.args); code != ExitOK {
				t.Fatalf("%s code = %d, stderr = %q", test.name, code, stderr.String())
			}
			if runtime.provider != test.provider || runtime.method != test.method ||
				!strings.Contains(stdout.String(), `"provider":"`+test.provider+`"`) || stderr.Len() != 0 {
				t.Fatalf("%s provider/method/stdout/stderr = %q/%q/%q/%q", test.name, runtime.provider, runtime.method, stdout.String(), stderr.String())
			}
		})
	}
}

func TestAuthLoginRejectsRedirectedTerminalBeforeRuntimeMutation(t *testing.T) {
	t.Parallel()
	runtime := &authCLIRuntime{
		result: authCLIResult(authbroker.TaskLogin), inputTerminal: true, errorTerminal: false,
	}
	command, stdout, stderr := newAuthCLI(strings.NewReader(""), runtime)
	if code := runCLI(command, []string{"auth", "login", "github"}); code != ExitUsage {
		t.Fatalf("auth login code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.loginCalls != 0 || runtime.inputTerminalCalls != 1 || runtime.errorTerminalCalls != 1 || stdout.Len() != 0 {
		t.Fatalf(
			"login/input-terminal/error-terminal calls and stdout = %d/%d/%d/%q",
			runtime.loginCalls, runtime.inputTerminalCalls, runtime.errorTerminalCalls, stdout.String(),
		)
	}
	if !strings.Contains(stderr.String(), "code: auth_login_tty_required") ||
		!strings.Contains(stderr.String(), "help auth login") {
		t.Fatalf("terminal error = %q", stderr.String())
	}
}

func TestAuthImportRejectsCredentialInArgvBeforeReadingStdin(t *testing.T) {
	runtime := &authCLIRuntime{result: authCLIResult(authbroker.TaskImport)}
	command, stdout, stderr := newAuthCLI(strings.NewReader("stdin-secret"), runtime)

	if code := runCLI(command, []string{"auth", "import", "github", "argv-secret"}); code != ExitUsage {
		t.Fatalf("auth import code = %d, stderr = %q", code, stderr.String())
	}
	if len(runtime.secret) != 0 || stdout.Len() != 0 || strings.Contains(stderr.String(), "stdin-secret") {
		t.Fatalf("invalid argv crossed stdin/runtime boundary: secret=%q stdout=%q stderr=%q", runtime.secret, stdout.String(), stderr.String())
	}
}

func TestAuthImportRejectsExplicitEmptyContextBeforeReadingStdin(t *testing.T) {
	runtime := &authCLIRuntime{result: authCLIResult(authbroker.TaskImport)}
	command, stdout, stderr := newAuthCLI(authPanicReader{}, runtime)

	if code := runCLI(command, []string{"auth", "import", "github", "--context="}); code != ExitUsage {
		t.Fatalf("auth import code = %d, stderr = %q", code, stderr.String())
	}
	if len(runtime.secret) != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "code: invalid_context_name") {
		t.Fatalf("empty Context crossed stdin/runtime boundary: secret=%q stdout=%q stderr=%q", runtime.secret, stdout.String(), stderr.String())
	}
}

func TestAuthImportErrorOutputStripsCredentialAndPrivateCause(t *testing.T) {
	const secret = "synthetic-secret-canary"
	const privateCause = "private-broker-canary"
	runtime := &authCLIRuntime{err: fault.Wrap(
		fault.KindUnavailable, "auth_broker_unavailable", "The Auth Broker is unavailable.", true, errors.New(privateCause),
	)}
	command, stdout, stderr := newAuthCLI(strings.NewReader(secret), runtime)

	if code := runCLI(command, []string{"--error-format=json", "auth", "import", "github"}); code != ExitUnavailable {
		t.Fatalf("auth import code = %d, stderr = %q", code, stderr.String())
	}
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, secret) || strings.Contains(combined, privateCause) || stdout.Len() != 0 {
		t.Fatalf("error output exposed private material: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `"code":"auth_broker_unavailable"`) ||
		!strings.Contains(stderr.String(), `"retryable":true`) {
		t.Fatalf("error output = %q", stderr.String())
	}
}

func TestAuthStatusUsesExplicitCommandContextOverRootDefault(t *testing.T) {
	runtime := &authCLIRuntime{statusResult: authCLIStatusResult(true)}
	command, _, stderr := newAuthCLI(strings.NewReader(""), runtime)

	if code := runCLI(command, []string{"--context", "fallback", "auth", "status", "--context", "default", "--format=json"}); code != ExitOK {
		t.Fatalf("auth status code = %d, stderr = %q", code, stderr.String())
	}
	if runtime.contextName != "default" || runtime.statusCalls != 1 {
		t.Fatalf("status context/calls = %q/%d", runtime.contextName, runtime.statusCalls)
	}
}

func TestAuthHumanOutputContainsOnlySecretFreeResultFields(t *testing.T) {
	result := authCLIResult(authbroker.TaskImport)
	output, err := renderAuthResult(result, successFormatText, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Context credential configured", "default", "github", "octocat", "ready"} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("text output = %q, want %q", output, want)
		}
	}
	for _, forbidden := range []string{"vault bytes", "tobari-h1_", "root key", "token"} {
		if strings.Contains(strings.ToLower(string(output)), forbidden) {
			t.Fatalf("text output exposes %q: %s", forbidden, output)
		}
	}
}

func TestAuthMutationAndConfiguredStatusOutputsDeclareActivationContract(t *testing.T) {
	for _, task := range []string{authbroker.TaskLogin, authbroker.TaskImport, authbroker.TaskLogout} {
		result := authCLIResult(task)
		if task == authbroker.TaskLogout {
			result = authCLILogoutResult()
		}
		for _, format := range []successFormat{successFormatText, successFormatJSON} {
			output, err := renderAuthResult(result, format, false)
			if err != nil {
				t.Fatalf("renderAuthResult(%s, %s): %v", task, format, err)
			}
			expectedGuidance := authbroker.ContextAuthActivationGuidance
			if task == authbroker.TaskLogout {
				expectedGuidance = authbroker.ContextAuthRemovalGuidance
			}
			if !strings.Contains(string(output), expectedGuidance) {
				t.Fatalf("%s %s output lacks activation contract: %s", task, format, output)
			}
		}
	}

	status := authCLIStatusResult(true)
	for _, format := range []successFormat{successFormatText, successFormatJSON} {
		output, err := renderAuthStatus(status, format, false)
		if err != nil {
			t.Fatalf("renderAuthStatus(%s): %v", format, err)
		}
		if !strings.Contains(string(output), authbroker.ContextAuthActivationGuidance) {
			t.Fatalf("configured status %s lacks activation contract: %s", format, output)
		}
	}
}

func TestAuthJSONPreservesUnconfiguredNullAndEmptyState(t *testing.T) {
	result := authCLIStatusResult(false)
	output, err := renderAuthStatus(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Auth struct {
			Providers []authbroker.ProviderStatus `json:"providers"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Auth.Providers) != 1 || document.Auth.Providers[0].State != authbroker.ProviderCredentialNotConfigured ||
		document.Auth.Providers[0].Configured ||
		document.Auth.Providers[0].AccountLabel != nil || document.Auth.Providers[0].CredentialRevision != "" {
		t.Fatalf("unconfigured JSON = %s", output)
	}
}

func TestAuthStatusJSONDoesNotCallLockedProviderUnconfigured(t *testing.T) {
	result := authCLIStatusResult(false)
	result.BrokerState = authbroker.BrokerStateLocked
	result.Providers[0].State = authbroker.ProviderCredentialUnavailable
	output, err := renderAuthStatus(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"broker_state":"locked"`) ||
		!strings.Contains(string(output), `"state":"unavailable"`) ||
		strings.Contains(string(output), `"state":"not_configured"`) {
		t.Fatalf("locked status JSON = %s", output)
	}
}

func TestAuthStatusJSONPreservesKnownEmptyProviderCollection(t *testing.T) {
	result := authCLIStatusResult(false)
	result.Providers = []authbroker.ProviderStatus{}
	output, err := renderAuthStatus(result, successFormatJSON, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), `"providers":[]`) || strings.Contains(string(output), `"providers":null`) {
		t.Fatalf("empty provider status JSON = %s", output)
	}
}
