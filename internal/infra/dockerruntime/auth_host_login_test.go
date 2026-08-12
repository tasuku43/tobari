package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

const hostLoginContextID = "018bcfe5-687b-7000-8000-000000000099"

type fakeHostCLIResolver struct {
	path  string
	err   error
	names []string
}

func (r *fakeHostCLIResolver) Resolve(name string) (string, error) {
	r.names = append(r.names, name)
	return r.path, r.err
}

type fakeHostCredentialAcquirer struct {
	githubPayload  hostCredentialPayload
	githubErr      error
	githubPath     string
	githubStreams  credentialhost.GitHubLoginStreams
	awsPayload     hostCredentialPayload
	awsErr         error
	awsPath        string
	awsProfile     credentialhost.ProfileConfig
	consoleProfile credentialhost.ConsoleProfileConfig
	consoleInput   io.Reader
	awsVisible     []byte
	awsCalls       int
	pupPayload     hostCredentialPayload
	pupErr         error
	pupPath        string
	pupVisible     []byte
	pupCalls       int
	codexPayload   hostCredentialPayload
	codexErr       error
	codexPath      string
	codexStreams   credentialhost.CodexLoginStreams
	codexCalls     int
	claudePayload  hostCredentialPayload
	claudeErr      error
	claudePath     string
	claudeStreams  credentialhost.ClaudeLoginStreams
	claudeCalls    int
}

func (a *fakeHostCredentialAcquirer) LoginCodex(
	_ context.Context,
	path string,
	streams credentialhost.CodexLoginStreams,
) (hostCredentialPayload, error) {
	a.codexCalls++
	a.codexPath = path
	a.codexStreams = streams
	if streams.Stdout != nil {
		_, _ = streams.Stdout.Write([]byte("Complete device authorization in your browser\n"))
	}
	return a.codexPayload, a.codexErr
}

func (a *fakeHostCredentialAcquirer) LoginClaude(
	_ context.Context,
	path string,
	streams credentialhost.ClaudeLoginStreams,
) (hostCredentialPayload, error) {
	a.claudeCalls++
	a.claudePath = path
	a.claudeStreams = streams
	if streams.Output != nil {
		_, _ = streams.Output.Write([]byte("Complete Claude authorization in your browser\n"))
	}
	return a.claudePayload, a.claudeErr
}

func (a *fakeHostCredentialAcquirer) LoginPup(
	_ context.Context,
	path string,
	_ io.Reader,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	a.pupCalls++
	a.pupPath = path
	if visible != nil {
		a.pupVisible = []byte("OAuth login complete\n")
		_ = visible(credentialhost.OutputStderr, a.pupVisible)
	}
	return a.pupPayload, a.pupErr
}

func (a *fakeHostCredentialAcquirer) LoginAWSConsole(
	_ context.Context,
	path string,
	profile credentialhost.ConsoleProfileConfig,
	input io.Reader,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	a.awsCalls++
	a.awsPath = path
	a.consoleProfile = profile
	a.consoleInput = input
	if visible != nil {
		a.awsVisible = []byte(syntheticAWSConsoleAuthorizationURL(profile.Region) + "\n")
		_ = visible(credentialhost.OutputStderr, a.awsVisible)
	}
	return a.awsPayload, a.awsErr
}

func (a *fakeHostCredentialAcquirer) LoginGitHub(
	_ context.Context,
	path string,
	streams credentialhost.GitHubLoginStreams,
) (hostCredentialPayload, error) {
	a.githubPath = path
	a.githubStreams = streams
	if streams.Stdout != nil {
		_, _ = streams.Stdout.Write([]byte("Open " + githubDeviceURL + "\n"))
	}
	return a.githubPayload, a.githubErr
}

func (a *fakeHostCredentialAcquirer) LoginAWS(
	_ context.Context,
	path string,
	profile credentialhost.ProfileConfig,
	visible credentialhost.VisibleOutput,
) (hostCredentialPayload, error) {
	a.awsCalls++
	a.awsPath = path
	a.awsProfile = profile
	if visible != nil {
		a.awsVisible = []byte("https://device.sso.us-east-1.amazonaws.com/\n")
		_ = visible(credentialhost.OutputStderr, a.awsVisible)
	}
	return a.awsPayload, a.awsErr
}

type immediateHostLoginProfileReader struct{}

func (immediateHostLoginProfileReader) ReadAWSProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ProfileConfig, error) {
	return readAWSLoginProfile(
		ctx, input, output,
		func(context.Context, io.Reader) error { return nil },
		func(input io.Reader, destination []byte) (int, error) { return input.Read(destination) },
	)
}

func (immediateHostLoginProfileReader) ReadAWSConsoleProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ConsoleProfileConfig, error) {
	return readAWSConsoleLoginProfile(
		ctx, input, output,
		func(context.Context, io.Reader) error { return nil },
		func(input io.Reader, destination []byte) (int, error) { return input.Read(destination) },
	)
}

type fixedConsoleProfileReader struct {
	profile credentialhost.ConsoleProfileConfig
}

func (fixedConsoleProfileReader) ReadAWSProfile(
	context.Context, io.Reader, io.Writer,
) (credentialhost.ProfileConfig, error) {
	return credentialhost.ProfileConfig{}, errors.New("unexpected identity-center profile read")
}

func (r fixedConsoleProfileReader) ReadAWSConsoleProfile(
	context.Context, io.Reader, io.Writer,
) (credentialhost.ConsoleProfileConfig, error) {
	return r.profile, nil
}

type waitingHostLoginProfileReader struct {
	waitInput func(context.Context, io.Reader) error
}

func (r waitingHostLoginProfileReader) ReadAWSProfile(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
) (credentialhost.ProfileConfig, error) {
	return readAWSLoginProfile(
		ctx, input, output, r.waitInput,
		func(input io.Reader, destination []byte) (int, error) { return input.Read(destination) },
	)
}

type cancelBeforeReturnHostLoginProfileReader struct {
	cancel context.CancelFunc
}

func (r cancelBeforeReturnHostLoginProfileReader) ReadAWSProfile(
	context.Context,
	io.Reader,
	io.Writer,
) (credentialhost.ProfileConfig, error) {
	r.cancel()
	return credentialhost.ProfileConfig{
		StartURL:  "https://example.awsapps.com/start",
		SSORegion: "us-east-1",
		AccountID: "123456789012",
		RoleName:  "Developer",
	}, nil
}

type cancelAfterReadInput struct {
	input  io.Reader
	cancel context.CancelFunc
	once   sync.Once
}

func (r *cancelAfterReadInput) Read(destination []byte) (int, error) {
	count, err := r.input.Read(destination)
	r.once.Do(r.cancel)
	return count, err
}

type promptSignalWriter struct {
	once    sync.Once
	written chan struct{}
}

func (w *promptSignalWriter) Write(content []byte) (int, error) {
	w.once.Do(func() { close(w.written) })
	return len(content), nil
}

type hostLoginDockerRunner struct {
	response string
	runErr   error
	args     []string
	input    []byte
	calls    int
}

func (r *hostLoginDockerRunner) Run(
	_ context.Context,
	args []string,
	_ []string,
	input io.Reader,
	stdout io.Writer,
	_ io.Writer,
) error {
	r.calls++
	r.args = append([]string(nil), args...)
	if input != nil {
		r.input, _ = io.ReadAll(input)
	}
	_, _ = io.WriteString(stdout, r.response)
	return r.runErr
}

func (*hostLoginDockerRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

type recordingBrowser struct {
	targets []string
	err     error
}

func (b *recordingBrowser) Open(_ context.Context, target string) error {
	b.targets = append(b.targets, target)
	return b.err
}

func TestHostGitHubLoginCommitsOnlyAfterAcquisitionUsingNonTTYControl(t *testing.T) {
	token := []byte("ghp_synthetic_host_token_canary_123456")
	resolver := &fakeHostCLIResolver{path: "/usr/local/bin/gh"}
	acquirer := &fakeHostCredentialAcquirer{githubPayload: hostCredentialPayload{
		secret: token, accountLabel: "octo-user",
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"github","revision":"` + strings.Repeat("a", 64) + `","account_label":"octo-user"}`}
	browser := &recordingBrowser{}
	runtime := &Runtime{
		runner: runner, browser: browser, hostCLIs: resolver, credentialHost: acquirer,
		hostLoginProfiles: immediateHostLoginProfileReader{},
	}
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "github",
		strings.NewReader("trusted terminal input"), &visible,
	)
	if err != nil || response.Provider != "github" || response.AccountLabel == nil || *response.AccountLabel != "octo-user" {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(resolver.names, []string{"gh"}) || acquirer.githubPath != resolver.path {
		t.Fatalf("resolver=%v driver path=%q", resolver.names, acquirer.githubPath)
	}
	wantArgs := []string{
		"exec", "-i", authBrokerContainer,
		"python", "-m", "authbroker.control",
		"login", "--context-id", hostLoginContextID,
		"--provider", "github", "--account-label", "octo-user",
	}
	if !reflect.DeepEqual(runner.args, wantArgs) || strings.Contains(strings.Join(runner.args, " "), " -t ") {
		t.Fatalf("Docker argv = %#v", runner.args)
	}
	if string(runner.input) != "ghp_synthetic_host_token_canary_123456" {
		t.Fatalf("broker stdin mismatch")
	}
	if !reflect.DeepEqual(browser.targets, []string{githubDeviceURL}) || !strings.Contains(visible.String(), githubDeviceURL) {
		t.Fatalf("browser=%v visible=%q", browser.targets, visible.String())
	}
	if bytes.Contains(token, []byte("synthetic")) {
		t.Fatalf("host token was not cleared after Broker commit")
	}
	if strings.Contains(visible.String(), "host_token_canary") {
		t.Fatalf("host token reached visible output: %q", visible.String())
	}
}

func TestHostDatadogLoginCommitsOnlyCanonicalPupState(t *testing.T) {
	state := []byte(`{"client":"oauth-canary"}`)
	resolver := &fakeHostCLIResolver{path: "/opt/homebrew/bin/pup"}
	acquirer := &fakeHostCredentialAcquirer{pupPayload: hostCredentialPayload{
		secret: state, accountLabel: credentialhost.PupAccountLabel,
		driverID: credentialhost.PupDriverID, driverRevision: strings.Repeat("d", 64),
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"datadog","revision":"` + strings.Repeat("e", 64) + `","account_label":"datadog-us1"}`}
	runtime := &Runtime{
		runner: runner, browser: &recordingBrowser{}, hostCLIs: resolver,
		credentialHost: acquirer, hostLoginProfiles: immediateHostLoginProfileReader{},
	}
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "datadog",
		strings.NewReader(""), &visible,
	)
	if err != nil || response.Provider != "datadog" || response.AccountLabel == nil ||
		*response.AccountLabel != credentialhost.PupAccountLabel {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(resolver.names, []string{"pup"}) || acquirer.pupPath != resolver.path ||
		acquirer.pupCalls != 1 {
		t.Fatalf("resolver=%v path=%q calls=%d", resolver.names, acquirer.pupPath, acquirer.pupCalls)
	}
	wantTail := []string{
		"login", "--context-id", hostLoginContextID,
		"--provider", "datadog", "--account-label", credentialhost.PupAccountLabel,
		"--driver-id", credentialhost.PupDriverID, "--driver-revision", strings.Repeat("d", 64),
	}
	if len(runner.args) < len(wantTail) || !reflect.DeepEqual(runner.args[len(runner.args)-len(wantTail):], wantTail) ||
		string(runner.input) != `{"client":"oauth-canary"}` {
		t.Fatalf("Datadog Broker argv/state = %#v/%q", runner.args, runner.input)
	}
	if !strings.Contains(visible.String(), "OAuth login complete") ||
		strings.Contains(visible.String(), "oauth-canary") {
		t.Fatalf("visible output = %q", visible.String())
	}
	if bytes.Contains(state, []byte("oauth-canary")) {
		t.Fatal("Datadog opaque state was not cleared after Broker commit")
	}
}

func TestHostOpenAILoginCommitsOnlyCanonicalCodexState(t *testing.T) {
	state := []byte(`{"schema_version":1,"opaque":"codex-oauth-canary"}`)
	resolver := &fakeHostCLIResolver{path: "/opt/homebrew/bin/codex"}
	acquirer := &fakeHostCredentialAcquirer{codexPayload: hostCredentialPayload{
		secret: state, accountLabel: "account-synthetic-123",
		driverID: credentialhost.CodexDriverID, driverRevision: strings.Repeat("f", 64),
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"openai","revision":"` + strings.Repeat("a", 64) + `","account_label":"account-synthetic-123"}`}
	runtime := &Runtime{
		runner: runner, browser: &recordingBrowser{}, hostCLIs: resolver,
		credentialHost: acquirer, hostLoginProfiles: immediateHostLoginProfileReader{},
	}
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "openai",
		strings.NewReader("trusted terminal input"), &visible,
	)
	if err != nil || response.Provider != "openai" || response.AccountLabel == nil ||
		*response.AccountLabel != "account-synthetic-123" {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(resolver.names, []string{"codex"}) || acquirer.codexPath != resolver.path ||
		acquirer.codexCalls != 1 || acquirer.codexStreams.Stdin == nil ||
		acquirer.codexStreams.Stdout == nil || acquirer.codexStreams.Stderr == nil {
		t.Fatalf("resolver=%v path=%q calls=%d streams=%+v", resolver.names, acquirer.codexPath, acquirer.codexCalls, acquirer.codexStreams)
	}
	wantTail := []string{
		"login", "--context-id", hostLoginContextID,
		"--provider", "openai", "--account-label", "account-synthetic-123",
		"--driver-id", credentialhost.CodexDriverID,
		"--driver-revision", strings.Repeat("f", 64),
	}
	if len(runner.args) < len(wantTail) ||
		!reflect.DeepEqual(runner.args[len(runner.args)-len(wantTail):], wantTail) ||
		string(runner.input) != `{"schema_version":1,"opaque":"codex-oauth-canary"}` {
		t.Fatalf("OpenAI Broker argv/state = %#v/%q", runner.args, runner.input)
	}
	if !strings.Contains(visible.String(), "device authorization") ||
		strings.Contains(visible.String(), "codex-oauth-canary") {
		t.Fatalf("visible output = %q", visible.String())
	}
	if bytes.Contains(state, []byte("codex-oauth-canary")) {
		t.Fatal("Codex OAuth state was not cleared after Broker commit")
	}
}

func TestHostAnthropicLoginCommitsOnlyCapturedClaudeToken(t *testing.T) {
	token := []byte("sk-ant-oat01-synthetic-token-canary")
	resolver := &fakeHostCLIResolver{path: "/usr/local/bin/claude"}
	acquirer := &fakeHostCredentialAcquirer{claudePayload: hostCredentialPayload{
		secret: token, accountLabel: credentialhost.ClaudeAccountLabel,
		driverID: credentialhost.ClaudeDriverID, driverRevision: strings.Repeat("e", 64),
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"anthropic","revision":"` + strings.Repeat("b", 64) + `","account_label":"` + credentialhost.ClaudeAccountLabel + `"}`}
	runtime := &Runtime{
		runner: runner, browser: &recordingBrowser{}, hostCLIs: resolver,
		credentialHost: acquirer, hostLoginProfiles: immediateHostLoginProfileReader{},
	}
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "anthropic",
		strings.NewReader("trusted terminal input"), &visible,
	)
	if err != nil || response.Provider != "anthropic" || response.AccountLabel == nil ||
		*response.AccountLabel != credentialhost.ClaudeAccountLabel {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	if !reflect.DeepEqual(resolver.names, []string{"claude"}) || acquirer.claudePath != resolver.path ||
		acquirer.claudeCalls != 1 || acquirer.claudeStreams.Stdin == nil ||
		acquirer.claudeStreams.Output == nil {
		t.Fatalf("resolver=%v path=%q calls=%d streams=%+v", resolver.names, acquirer.claudePath, acquirer.claudeCalls, acquirer.claudeStreams)
	}
	wantTail := []string{
		"login", "--context-id", hostLoginContextID,
		"--provider", "anthropic", "--account-label", credentialhost.ClaudeAccountLabel,
	}
	if len(runner.args) < len(wantTail) ||
		!reflect.DeepEqual(runner.args[len(runner.args)-len(wantTail):], wantTail) ||
		string(runner.input) != "sk-ant-oat01-synthetic-token-canary" {
		t.Fatalf("Anthropic Broker argv/token = %#v/%q", runner.args, runner.input)
	}
	if !strings.Contains(visible.String(), "Claude authorization") ||
		strings.Contains(visible.String(), "synthetic-token-canary") {
		t.Fatalf("visible output = %q", visible.String())
	}
	if bytes.Contains(token, []byte("synthetic-token-canary")) {
		t.Fatal("Claude setup token was not cleared after Broker commit")
	}
}

func TestHostAWSLoginPromptsFourFieldsAndCommitsOpaqueState(t *testing.T) {
	state := []byte(`{"schema_version":1,"opaque":"sso-cache-canary"}`)
	resolver := &fakeHostCLIResolver{path: "/usr/local/bin/aws"}
	acquirer := &fakeHostCredentialAcquirer{awsPayload: hostCredentialPayload{
		secret: state, accountLabel: "123456789012", driverID: awsHostDriverID,
		driverRevision: strings.Repeat("b", 64),
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"aws","revision":"` + strings.Repeat("c", 64) + `","account_label":"123456789012"}`}
	browser := &recordingBrowser{}
	runtime := &Runtime{
		runner: runner, browser: browser, hostCLIs: resolver, credentialHost: acquirer,
		hostLoginProfiles: immediateHostLoginProfileReader{},
	}
	input := strings.NewReader(
		"https://example.awsapps.com/start\n" +
			"us-east-1\n" +
			"123456789012\n" +
			"Developer-Role\n",
	)
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "aws", input, &visible,
	)
	if err != nil || response.Provider != "aws" {
		t.Fatalf("response=%+v error=%v", response, err)
	}
	wantProfile := credentialhost.ProfileConfig{
		StartURL: "https://example.awsapps.com/start", SSORegion: "us-east-1",
		AccountID: "123456789012", RoleName: "Developer-Role",
	}
	if acquirer.awsProfile != wantProfile || acquirer.awsPath != resolver.path ||
		!reflect.DeepEqual(resolver.names, []string{"aws"}) {
		t.Fatalf("profile=%+v path=%q resolver=%v", acquirer.awsProfile, acquirer.awsPath, resolver.names)
	}
	wantTail := []string{
		"login", "--context-id", hostLoginContextID,
		"--provider", "aws", "--account-label", "123456789012",
		"--driver-id", awsHostDriverID, "--driver-revision", strings.Repeat("b", 64),
	}
	if len(runner.args) < len(wantTail) || !reflect.DeepEqual(runner.args[len(runner.args)-len(wantTail):], wantTail) {
		t.Fatalf("Docker argv = %#v", runner.args)
	}
	if string(runner.input) != `{"schema_version":1,"opaque":"sso-cache-canary"}` {
		t.Fatalf("broker state stdin mismatch")
	}
	for _, prompt := range []string{
		"access portal URL", "Identity Center region", "AWS account ID", "AWS role name",
	} {
		if !strings.Contains(visible.String(), prompt) {
			t.Fatalf("missing prompt %q in %q", prompt, visible.String())
		}
	}
	const deviceURL = "https://device.sso.us-east-1.amazonaws.com/"
	if !reflect.DeepEqual(browser.targets, []string{deviceURL}) || !strings.Contains(visible.String(), deviceURL) {
		t.Fatalf("browser=%v visible=%q", browser.targets, visible.String())
	}
	if bytes.Contains(state, []byte("sso-cache-canary")) {
		t.Fatalf("AWS opaque state was not cleared after Broker commit")
	}
	if strings.Contains(visible.String(), "sso-cache-canary") {
		t.Fatalf("AWS state reached visible output: %q", visible.String())
	}
}

func TestHostAWSConsoleLoginCommitsDistinctDriverState(t *testing.T) {
	state := []byte(`{"schema_version":1,"opaque":"console-cache-canary"}`)
	resolver := &fakeHostCLIResolver{path: "/usr/local/bin/aws"}
	acquirer := &fakeHostCredentialAcquirer{awsPayload: hostCredentialPayload{
		secret: state, accountLabel: "123456789012", driverID: awsConsoleDriverID,
		driverRevision: strings.Repeat("d", 64),
	}}
	runner := &hostLoginDockerRunner{response: `{"schema_version":1,"ok":true,"provider":"aws","revision":"` + strings.Repeat("e", 64) + `","account_label":"123456789012"}`}
	input := strings.NewReader("authorization-code\n")
	browser := &recordingBrowser{}
	runtime := &Runtime{
		runner: runner, browser: browser, hostCLIs: resolver, credentialHost: acquirer,
		hostLoginProfiles: fixedConsoleProfileReader{profile: credentialhost.ConsoleProfileConfig{Region: "us-east-1"}},
	}
	var visible bytes.Buffer
	response, err := runtime.runHostCredentialLoginOnTTY(
		context.Background(), hostLoginContextID, "aws", input, &visible, awsConsoleMethod,
	)
	if err != nil || response.Provider != "aws" || acquirer.consoleProfile.Region != "us-east-1" ||
		acquirer.consoleInput != input {
		t.Fatalf("response/error/profile/input = %+v/%v/%+v/%T", response, err, acquirer.consoleProfile, acquirer.consoleInput)
	}
	wantTail := []string{
		"login", "--context-id", hostLoginContextID,
		"--provider", "aws", "--account-label", "123456789012",
		"--driver-id", awsConsoleDriverID, "--driver-revision", strings.Repeat("d", 64),
	}
	if len(runner.args) < len(wantTail) || !reflect.DeepEqual(runner.args[len(runner.args)-len(wantTail):], wantTail) ||
		string(runner.input) != `{"schema_version":1,"opaque":"console-cache-canary"}` {
		t.Fatalf("console broker argv/state = %#v/%q", runner.args, runner.input)
	}
	if !strings.Contains(visible.String(), "signin.aws.amazon.com") {
		t.Fatalf("console visible output = %q", visible.String())
	}
	if !reflect.DeepEqual(browser.targets, []string{syntheticAWSConsoleAuthorizationURL("us-east-1")}) {
		t.Fatalf("console browser targets = %q", browser.targets)
	}
}

func TestHostAcquisitionFailureDoesNotBeginBrokerMutation(t *testing.T) {
	for _, test := range []struct {
		provider string
		input    string
		failure  error
	}{
		{provider: "github", failure: credentialhost.ErrGitHubLoginFailed},
		{provider: "aws", input: "https://example.awsapps.com/start\nus-east-1\n123456789012\nDeveloper\n", failure: credentialhost.ErrCommandFailed},
		{provider: "datadog", failure: credentialhost.ErrPupLoginFailed},
		{provider: "openai", failure: credentialhost.ErrCodexLoginFailed},
		{provider: "anthropic", failure: credentialhost.ErrClaudeLoginFailed},
	} {
		t.Run(test.provider, func(t *testing.T) {
			acquirer := &fakeHostCredentialAcquirer{
				githubErr: test.failure, awsErr: test.failure, pupErr: test.failure,
				codexErr: test.failure, claudeErr: test.failure,
			}
			runner := &hostLoginDockerRunner{}
			runtime := &Runtime{
				runner:            runner,
				hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/" + providerHostExecutable(test.provider)},
				credentialHost:    acquirer,
				hostLoginProfiles: immediateHostLoginProfileReader{},
				browser:           &recordingBrowser{},
			}
			_, err := runtime.runHostCredentialLoginOnTTY(
				context.Background(), hostLoginContextID, test.provider,
				strings.NewReader(test.input), io.Discard,
			)
			if !errors.Is(err, test.failure) || runner.calls != 0 {
				t.Fatalf("error=%v Broker calls=%d", err, runner.calls)
			}
		})
	}
}

func TestHostAWSProfilePromptContextStopDoesNotAcquireOrMutate(t *testing.T) {
	for _, test := range []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
		trigger    func(context.CancelFunc)
		want       error
	}{
		{
			name: "cancel",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			trigger: func(cancel context.CancelFunc) { cancel() },
			want:    context.Canceled,
		},
		{
			name: "deadline",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), 100*time.Millisecond)
			},
			trigger: func(context.CancelFunc) {},
			want:    context.DeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, inputWriter, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			defer inputWriter.Close()

			ctx, stop := test.newContext()
			defer stop()
			acquirer := &fakeHostCredentialAcquirer{}
			runner := &hostLoginDockerRunner{}
			output := &promptSignalWriter{written: make(chan struct{})}
			runtime := &Runtime{
				runner:            runner,
				hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
				credentialHost:    acquirer,
				hostLoginProfiles: waitingHostLoginProfileReader{waitInput: waitHostLoginInput},
				browser:           &recordingBrowser{},
			}
			result := make(chan error, 1)
			go func() {
				_, loginErr := runtime.runHostCredentialLoginOnTTY(
					ctx, hostLoginContextID, "aws", input, output,
				)
				result <- loginErr
			}()

			select {
			case <-output.written:
			case <-time.After(time.Second):
				t.Fatal("AWS profile prompt did not begin")
			}
			started := time.Now()
			test.trigger(stop)
			select {
			case loginErr := <-result:
				if !errors.Is(loginErr, test.want) {
					t.Fatalf("login error = %v; want %v", loginErr, test.want)
				}
				if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
					t.Fatalf("prompt context stop took %s", elapsed)
				}
			case <-time.After(time.Second):
				t.Fatal("AWS profile prompt remained blocked after context stop")
			}
			if acquirer.awsCalls != 0 || runner.calls != 0 {
				t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
			}
		})
	}
}

func TestHostAWSProfilePromptPartialByteCancellationDoesNotAcquireOrMutate(t *testing.T) {
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer inputWriter.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	secondWait := make(chan struct{})
	waitCalls := 0
	profileReader := waitingHostLoginProfileReader{waitInput: func(ctx context.Context, input io.Reader) error {
		waitCalls++
		if waitCalls == 2 {
			close(secondWait)
		}
		return waitHostLoginInput(ctx, input)
	}}
	acquirer := &fakeHostCredentialAcquirer{}
	runner := &hostLoginDockerRunner{}
	output := &promptSignalWriter{written: make(chan struct{})}
	runtime := &Runtime{
		runner:            runner,
		hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
		credentialHost:    acquirer,
		hostLoginProfiles: profileReader,
		browser:           &recordingBrowser{},
	}
	result := make(chan error, 1)
	go func() {
		_, loginErr := runtime.runHostCredentialLoginOnTTY(
			ctx, hostLoginContextID, "aws", input, output,
		)
		result <- loginErr
	}()

	select {
	case <-output.written:
	case <-time.After(time.Second):
		t.Fatal("AWS profile prompt did not begin")
	}
	if _, err := inputWriter.Write([]byte{'h'}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondWait:
	case <-time.After(time.Second):
		t.Fatal("partial profile byte did not return to the readiness wait")
	}
	started := time.Now()
	cancel()
	select {
	case loginErr := <-result:
		if !errors.Is(loginErr, context.Canceled) {
			t.Fatalf("login error = %v", loginErr)
		}
		if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
			t.Fatalf("partial-line cancellation took %s", elapsed)
		}
	case <-time.After(time.Second):
		t.Fatal("partial profile line remained blocked after cancellation")
	}
	if acquirer.awsCalls != 0 || runner.calls != 0 {
		t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
	}
}

func TestAWSProfileReadCancellationWinsOverBufferedValidInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := &cancelAfterReadInput{
		input: strings.NewReader(
			"https://example.awsapps.com/start\n" +
				"us-east-1\n" +
				"123456789012\n" +
				"Developer\n",
		),
		cancel: cancel,
	}
	_, err := readAWSLoginProfile(
		ctx, input, io.Discard,
		func(context.Context, io.Reader) error { return nil },
		func(input io.Reader, destination []byte) (int, error) { return input.Read(destination) },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("profile read error = %v", err)
	}
}

func TestHostAWSProfileCancellationImmediatelyBeforeAcquisitionDoesNotMutate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	acquirer := &fakeHostCredentialAcquirer{}
	runner := &hostLoginDockerRunner{}
	runtime := &Runtime{
		runner:            runner,
		hostCLIs:          &fakeHostCLIResolver{path: "/usr/local/bin/aws"},
		credentialHost:    acquirer,
		hostLoginProfiles: cancelBeforeReturnHostLoginProfileReader{cancel: cancel},
		browser:           &recordingBrowser{},
	}
	_, err := runtime.runHostCredentialLoginOnTTY(
		ctx, hostLoginContextID, "aws", strings.NewReader("unused"), io.Discard,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("login error = %v", err)
	}
	if acquirer.awsCalls != 0 || runner.calls != 0 {
		t.Fatalf("AWS acquisitions=%d Broker mutations=%d", acquirer.awsCalls, runner.calls)
	}
}

func TestPathHostCLIResolverAcceptsOnlyTrustedInstallationRoots(t *testing.T) {
	resolver := pathHostCLIResolver{lookPath: func(string) (string, error) {
		return filepath.Join("project", "bin", "aws"), nil
	}}
	if _, err := resolver.Resolve("aws"); err == nil {
		t.Fatal("relative PATH-selected executable was accepted")
	}

	executable := filepath.Join(t.TempDir(), "aws")
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	resolver.lookPath = func(string) (string, error) { return executable, nil }
	if _, err := resolver.Resolve("aws"); err == nil {
		t.Fatal("repository- or temporary-root executable was accepted")
	}
	trusted := "/usr/bin/true"
	if _, err := os.Stat(trusted); err != nil {
		t.Skipf("trusted test executable is unavailable: %v", err)
	}
	resolver.lookPath = func(string) (string, error) { return "/usr/bin/aws", nil }
	// Model an ordinary /usr/bin installation without depending on aws being
	// present by checking the path predicate separately.
	if !trustedHostCLIPath(trusted) {
		t.Fatalf("trusted installation path %q was rejected", trusted)
	}
	if _, err := resolver.Resolve("unreviewed"); err == nil {
		t.Fatal("unreviewed driver name was accepted")
	}
}

func TestLoginVisibleOutputHasAggregateBound(t *testing.T) {
	filter := &loginVisibleOutput{destination: io.Discard}
	written, err := filter.Write(bytes.Repeat([]byte{'x'}, maxLoginVisibleBytes+1))
	if !errors.Is(err, errLoginVisibleOutputLimit) || written != maxLoginVisibleBytes {
		t.Fatalf("written=%d error=%v", written, err)
	}
}

func TestHostCredentialPayloadFormattingIsRedacted(t *testing.T) {
	payload := hostCredentialPayload{secret: []byte("secret-canary"), accountLabel: "account"}
	for _, rendered := range []string{
		payload.String(), payload.GoString(),
	} {
		if strings.Contains(rendered, "secret-canary") || !strings.Contains(rendered, "redacted") {
			t.Fatalf("payload formatting leaked: %q", rendered)
		}
	}
}
