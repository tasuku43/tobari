package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

func syntheticAWSConsoleAuthorizationURL(region string) string {
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {awsConsoleClientID},
		"state":                 {"00000000-1111-2222-3333-444444444444"},
		"code_challenge_method": {"SHA-256"},
		"scope":                 {"openid"},
		"redirect_uri":          {"https://" + region + ".signin.aws.amazon.com/v1/sessions/confirmation"},
		"code_challenge":        {strings.Repeat("A", 43)},
	}
	return "https://" + region + ".signin.aws.amazon.com/v1/authorize?" + query.Encode()
}

func TestProviderBindingCollisionUsesDistinctPublicFault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-controlled provider manifests require Unix ownership semantics")
	}
	configDirectory := t.TempDir()
	providerDirectory := filepath.Join(configDirectory, "auth", "providers")
	if err := os.MkdirAll(providerDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	document := []byte(`{
  "schema_version": 1,
  "id": "overlap-ci",
  "display_name": "Overlap CI",
  "acquisition": {"mode": "stdin_import"},
  "credential": {"kind": "primary_secret"},
  "workspace_projections": [{"kind":"env","name":"OVERLAP_TOKEN","template":"${HANDLE}"}],
  "header_bindings": [{
    "target": {"scheme":"https","host":"api.github.com","port":443},
    "source": {"header":"authorization","formats":["bearer"]},
    "destination": {"header":"authorization","format":"bearer","secret_field":"primary_secret"},
    "secret_headers": ["authorization"]
  }]
}`)
	if err := os.WriteFile(filepath.Join(providerDirectory, "overlap.json"), document, 0o600); err != nil {
		t.Fatal(err)
	}
	instance := &Runtime{configDirectory: configDirectory}
	_, err := instance.loadAuthProviders()
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "ambiguous_provider_http_binding" || public.Kind != fault.KindRejected || public.Retryable {
		t.Fatalf("binding-collision fault = %+v, ok=%t", public, ok)
	}
}

func TestHostCredentialLoginRequiresRealTerminalBeforeHostOrDockerExecution(t *testing.T) {
	t.Parallel()
	runner := &brokerProtocolRunner{}
	runtime := &Runtime{runner: runner}
	_, err := runtime.runHostCredentialLogin(
		context.Background(),
		"018bcfe5-687b-7000-8000-000000000099",
		"github",
		strings.NewReader(""),
		io.Discard,
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Kind != fault.KindInvalidInput || public.Code != "auth_login_tty_required" || public.Retryable {
		t.Fatalf("terminal fault = %+v, ok=%t", public, ok)
	}
	if runner.calls != 0 {
		t.Fatalf("Docker runner calls = %d, want 0", runner.calls)
	}
	if strings.Contains(public.Message, "GitHub") ||
		(len(public.NextActions) == 1 && strings.Contains(public.NextActions[0].Reason, "GitHub")) {
		t.Fatalf("terminal fault is provider-specific: %+v", public)
	}
}

func TestTerminalDetectionRejectsNonTTYCharacterDevice(t *testing.T) {
	t.Parallel()
	device, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer device.Close()
	if isTerminalFile(device) {
		t.Fatalf("%s was accepted as an interactive terminal", os.DevNull)
	}
}

func TestBuildAuthResultPreservesConfirmedChangeAfterObservationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &Runtime{configDirectory: t.TempDir(), stateDirectory: t.TempDir(), runner: &brokerProtocolRunner{}}
	observed, err := runtime.buildAuthMutationObservation(
		ctx,
		authbroker.TaskImport,
		"default",
		"018bcfe5-687b-7000-8000-000000000099",
		"github",
		brokerControlResponse{Revision: strings.Repeat("a", 64)},
		true,
		true,
		authbroker.StorageBackendXDGFile,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authbroker.NewResult(authbroker.TaskImport, "default", "github", observed)
	if err != nil {
		t.Fatal(err)
	}
	if result.Change != authbroker.MutationChangeChanged ||
		result.WorkspaceActivation.Coverage != authbroker.WorkspaceActivationCoverageUnavailable ||
		result.WorkspaceActivation.Guidance != "" || len(result.WorkspaceActivation.Workspaces) != 0 {
		t.Fatalf("Workspace activation = %+v", result.WorkspaceActivation)
	}
}

func TestBuildAuthResultReportsNoOpLogoutWithoutPostSuccessObservation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime := &Runtime{configDirectory: t.TempDir(), stateDirectory: t.TempDir(), runner: &brokerProtocolRunner{}}
	observed, err := runtime.buildAuthMutationObservation(
		ctx, authbroker.TaskLogout, "default", "018bcfe5-687b-7000-8000-000000000099", "github",
		brokerControlResponse{}, false, false, authbroker.StorageBackendXDGFile,
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := authbroker.NewResult(authbroker.TaskLogout, "default", "github", observed)
	if err != nil {
		t.Fatal(err)
	}
	if result.Change != authbroker.MutationChangeNoChange || result.Configured ||
		result.WorkspaceActivation.Coverage != authbroker.WorkspaceActivationCoverageNotApplicable ||
		result.WorkspaceActivation.Guidance != "" || len(result.WorkspaceActivation.Workspaces) != 0 {
		t.Fatalf("no-op logout result = %+v", result)
	}
}

func derivedWorkspaceActivation(
	t *testing.T,
	contextName, contextID string,
	statuses []authbroker.ProviderStatus,
	observed authbroker.WorkspaceObservation,
) authbroker.WorkspaceActivation {
	t.Helper()
	result, err := authbroker.NewStatusResult(contextName, authbroker.StatusObservation{
		ContextState: tobari.ContextObservationPersisted, Context: contextName, ContextID: contextID,
		StorageBackend: authbroker.StorageBackendXDGFile, BrokerState: authbroker.BrokerStateReady,
		Providers: statuses, Workspaces: observed,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result.WorkspaceActivation
}

func TestObserveWorkspaceActivationUsesExactRegistryAndBrokerBindingState(t *testing.T) {
	tests := []struct {
		name         string
		registry     string
		bindingState string
		status       authbroker.ProviderStatus
		want         authbroker.WorkspaceProviderProjectionState
		wantSummary  authbroker.WorkspaceActivationState
		wantAction   bool
	}{
		{name: "current", registry: "matching", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionCurrent, wantSummary: authbroker.WorkspaceActivationReady},
		{name: "broker stale", registry: "matching", bindingState: "stale", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionStale, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
		{name: "missing registry", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionMissing, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
		{name: "stale revision", registry: "stale", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionStale, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
		{name: "provider unavailable", registry: "matching", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialUnavailable}, want: authbroker.WorkspaceProviderProjectionUnavailable, wantSummary: authbroker.WorkspaceActivationUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &authDoctorRunner{bindingState: test.bindingState}
			fixture := newAuthDoctorFixture(t, runner)
			switch test.registry {
			case "matching":
				fixture.writeRegistry(t, authDoctorRevision, fixture.digest)
			case "stale":
				fixture.writeRegistry(t, "revision_old", fixture.digest)
			}
			projection, err := fixture.runtime.loadAuthProviders()
			if err != nil {
				t.Fatal(err)
			}
			statuses := []authbroker.ProviderStatus{test.status}
			observed := fixture.runtime.observeWorkspaceActivation(
				context.Background(), fixture.project.ContextID, statuses, projection,
			)
			activation := derivedWorkspaceActivation(t, "default", fixture.project.ContextID, statuses, observed)
			if activation.Coverage != authbroker.WorkspaceActivationCoverageExhaustive ||
				activation.State != test.wantSummary || len(activation.Workspaces) != 1 ||
				len(activation.Workspaces[0].Providers) != 1 || activation.Workspaces[0].Providers[0].State != test.want ||
				(activation.Workspaces[0].NextAction != nil) != test.wantAction {
				t.Fatalf("activation = %+v", activation)
			}
			if test.wantAction && (activation.Workspaces[0].NextAction.WorkingDirectory != fixture.project.Root ||
				!reflect.DeepEqual(activation.Workspaces[0].NextAction.Argv, []string{"tobari", "--context", "default"})) {
				t.Fatalf("re-entry action = %+v", activation.Workspaces[0].NextAction)
			}
		})
	}
}

func TestObserveWorkspaceActivationDistinguishesZeroEligibleFromEnumerationFailure(t *testing.T) {
	runtime := newProjectStateRuntime(t)
	contextID := "018bcfe5-687b-7000-8000-000000000099"
	observed := runtime.observeWorkspaceActivation(
		context.Background(), contextID,
		[]authbroker.ProviderStatus{}, authbroker.Projection{Providers: []authbroker.Provider{}},
	)
	activation := derivedWorkspaceActivation(t, "default", contextID, []authbroker.ProviderStatus{}, observed)
	if activation.Coverage != authbroker.WorkspaceActivationCoverageExhaustive ||
		activation.State != authbroker.WorkspaceActivationNotApplicable || len(activation.Workspaces) != 0 {
		t.Fatalf("zero eligible activation = %+v", activation)
	}

	fixture := newAuthDoctorFixture(t, &authDoctorRunner{})
	if err := fixture.runtime.removeProjectRootIndex(fixture.project.Root); err != nil {
		t.Fatal(err)
	}
	observed = fixture.runtime.observeWorkspaceActivation(
		context.Background(), fixture.project.ContextID,
		[]authbroker.ProviderStatus{}, authbroker.Projection{Providers: []authbroker.Provider{}},
	)
	if observed.Coverage != authbroker.WorkspaceActivationCoverageUnavailable {
		t.Fatalf("enumeration observation = %+v", observed)
	}
	activation = derivedWorkspaceActivation(t, "default", fixture.project.ContextID, []authbroker.ProviderStatus{}, observed)
	if activation.Coverage != authbroker.WorkspaceActivationCoverageUnavailable ||
		activation.State != authbroker.WorkspaceActivationUnavailable || len(activation.Workspaces) != 0 {
		t.Fatalf("enumeration failure activation = %+v", activation)
	}
}

func TestObserveWorkspaceActivationMixedProviderUncertaintyDoesNotOfferAction(t *testing.T) {
	fixture := newAuthDoctorFixture(t, &authDoctorRunner{})
	projection, err := fixture.runtime.loadAuthProviders()
	if err != nil {
		t.Fatal(err)
	}
	statuses := []authbroker.ProviderStatus{
		{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: authDoctorRevision},
		{Provider: "aws", State: authbroker.ProviderCredentialUnavailable},
	}
	observed := fixture.runtime.observeWorkspaceActivation(
		context.Background(), fixture.project.ContextID, statuses, projection,
	)
	activation := derivedWorkspaceActivation(t, "default", fixture.project.ContextID, statuses, observed)
	if activation.State != authbroker.WorkspaceActivationUnresolved || len(activation.Workspaces) != 1 ||
		activation.Workspaces[0].NextAction != nil {
		t.Fatalf("mixed activation = %+v", activation)
	}
}

func TestObserveWorkspaceActivationStopsBeforeBrokerCallsWhenProviderBoundsAreExceeded(t *testing.T) {
	t.Run("status collection", func(t *testing.T) {
		runner := &authDoctorRunner{}
		fixture := newAuthDoctorFixture(t, runner)
		statuses := make([]authbroker.ProviderStatus, authbroker.MaxWorkspaceActivationProviders+1)
		for index := range statuses {
			statuses[index] = authbroker.ProviderStatus{
				Provider: fmt.Sprintf("provider-%03d", index), State: authbroker.ProviderCredentialUnavailable,
			}
		}
		observed := fixture.runtime.observeWorkspaceActivation(
			context.Background(), fixture.project.ContextID, statuses,
			authbroker.Projection{Providers: []authbroker.Provider{}},
		)
		if observed.Coverage != authbroker.WorkspaceActivationCoverageUnavailable || len(runner.controlCalls) != 0 {
			t.Fatalf("oversized status observation/calls = %+v/%v", observed, runner.controlCalls)
		}
	})

	t.Run("registry collection", func(t *testing.T) {
		runner := &authDoctorRunner{}
		fixture := newAuthDoctorFixture(t, runner)
		providers := make([]projectAuthProviderBinding, authbroker.MaxWorkspaceActivationProviders+1)
		for index := range providers {
			providers[index] = projectAuthProviderBinding{
				Provider: fmt.Sprintf("provider-%03d", index), Revision: "revision_1",
				BindingDigest: "sha256:" + strings.Repeat("a", 64),
			}
		}
		if err := writeAtomicJSON(fixture.runtime.projectAuthRegistryPath(fixture.project.ID), projectAuthRegistry{
			SchemaVersion: projectAuthRegistrySchema, ProjectID: fixture.project.ID,
			Providers: providers, Files: []projectAuthRegistryEntry{},
		}); err != nil {
			t.Fatal(err)
		}
		observed := fixture.runtime.observeWorkspaceActivation(
			context.Background(), fixture.project.ContextID,
			[]authbroker.ProviderStatus{}, authbroker.Projection{Providers: []authbroker.Provider{}},
		)
		if observed.Coverage != authbroker.WorkspaceActivationCoverageUnavailable || len(runner.controlCalls) != 0 {
			t.Fatalf("oversized registry observation/calls = %+v/%v", observed, runner.controlCalls)
		}
	})
}

func TestSupportsOnlyReviewedBuiltinAuthHelpers(t *testing.T) {
	tests := []struct {
		name     string
		provider authbroker.Provider
		want     bool
	}{
		{name: "github", provider: authbroker.Provider{ID: "github", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}, want: true},
		{name: "aws", provider: authbroker.Provider{ID: "aws", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "aws-sso"}}, want: true},
		{name: "datadog", provider: authbroker.Provider{ID: "datadog", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "pup-oauth"}}, want: true},
		{name: "openai", provider: authbroker.Provider{ID: "openai", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "codex-chatgpt-oauth"}}, want: true},
		{name: "anthropic", provider: authbroker.Provider{ID: "anthropic", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "claude-setup-token"}}, want: true},
		{name: "aws wrong helper", provider: authbroker.Provider{ID: "aws", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}},
		{name: "datadog wrong helper", provider: authbroker.Provider{ID: "datadog", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}},
		{name: "openai wrong helper", provider: authbroker.Provider{ID: "openai", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}},
		{name: "anthropic wrong helper", provider: authbroker.Provider{ID: "anthropic", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "github-gh"}}},
		{name: "other reused helper", provider: authbroker.Provider{ID: "other", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "aws-sso"}}},
		{name: "stdin import", provider: authbroker.Provider{ID: "github", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionStdinImport}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := supportsBuiltinAuthHelper(test.provider); got != test.want {
				t.Fatalf("supportsBuiltinAuthHelper(%+v) = %t, want %t", test.provider, got, test.want)
			}
		})
	}
}

func TestLoginVisibleOutputPreservesBoundedTTYStream(t *testing.T) {
	var visible bytes.Buffer
	opened := []string{}
	filter := &loginVisibleOutput{destination: &visible, openBrowser: func(target string) error {
		opened = append(opened, target)
		return nil
	}}
	chunks := []string{
		"! First copy your one-time code: SYNTH-ETIC\nOpen this URL in your browser:\nhttps://github.com/login/de",
		"vice\n! Authentication credentials saved in plain text\n",
		"diagnostic: ! Authentication credentials saved in plain text\n",
		"Waiting for authentication...\n",
	}
	for _, chunk := range chunks {
		if _, err := filter.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(visible.String(), "https://github.com/login/device") || !strings.Contains(visible.String(), "Waiting for authentication") {
		t.Fatalf("interactive helper output was lost: %q", visible.String())
	}
	if strings.Contains(visible.String(), "saved in plain text") {
		if !strings.Contains(visible.String(), "diagnostic: ! Authentication credentials saved in plain text") ||
			strings.Count(visible.String(), "saved in plain text") != 1 {
			t.Fatalf("ephemeral storage warning filtering was too broad: %q", visible.String())
		}
	}
	if len(opened) != 1 || opened[0] != githubDeviceURL {
		t.Fatalf("browser opens = %q", opened)
	}
	if err := filter.flush(); err != nil {
		t.Fatal(err)
	}
}

func TestLoginVisibleOutputFallsBackToOneFixedManualURL(t *testing.T) {
	var visible bytes.Buffer
	opens := 0
	filter := &loginVisibleOutput{destination: &visible, openBrowser: func(target string) error {
		opens++
		if target != githubDeviceURL {
			t.Fatalf("browser target = %q", target)
		}
		return os.ErrNotExist
	}}
	if _, err := filter.Write([]byte(
		"Open https://example.com/login manually\n" + githubDeviceURL + "\n" + githubDeviceURL + "\n",
	)); err != nil {
		t.Fatal(err)
	}
	_ = filter.flush()
	if opens != 1 {
		t.Fatalf("browser opens = %d, want 1", opens)
	}
	if strings.Count(visible.String(), githubManualBrowserFallback) != 1 || !strings.Contains(visible.String(), "https://example.com/login") {
		t.Fatalf("visible output = %q", visible.String())
	}
}

func TestLoginVisibleOutputOpensOnlyExactAWSDeviceURLOnce(t *testing.T) {
	const target = "https://device.sso.us-east-1.amazonaws.com/"
	var visible bytes.Buffer
	opened := []string{}
	filter := &loginVisibleOutput{destination: &visible, openBrowser: func(candidate string) error {
		opened = append(opened, candidate)
		return os.ErrNotExist
	}}
	for _, line := range []string{
		"Open https://example.com/\n",
		"prefix Open " + target + "\n",
		"Open " + target + "?code=secret\n",
		"Open " + target + "#fragment\n",
		target + "\n",
		"Open " + target + "\n",
	} {
		if _, err := filter.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	_ = filter.flush()
	if !reflect.DeepEqual(opened, []string{target}) {
		t.Fatalf("browser opens = %q", opened)
	}
	if strings.Count(visible.String(), "visit "+target+" manually") != 1 {
		t.Fatalf("visible output = %q", visible.String())
	}
}

func TestLoginVisibleOutputOpensOnlyRegionBoundAWSConsoleURLOnce(t *testing.T) {
	target := syntheticAWSConsoleAuthorizationURL("ap-northeast-1")
	var visible bytes.Buffer
	opened := []string{}
	filter := &loginVisibleOutput{
		destination:   &visible,
		consoleRegion: "ap-northeast-1",
		openBrowser: func(candidate string) error {
			opened = append(opened, candidate)
			return os.ErrNotExist
		},
	}
	for _, line := range []string{
		syntheticAWSConsoleAuthorizationURL("us-east-1") + "\n",
		target + "&state=duplicate\n",
		target + "#fragment\n",
		"prefix " + target + "\n",
		target + "\n",
		target + "\n",
	} {
		if _, err := filter.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if err := filter.flush(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(opened, []string{target}) {
		t.Fatalf("browser opens = %q", opened)
	}
	if strings.Count(visible.String(), "visit "+target+" manually") != 1 {
		t.Fatalf("visible output = %q", visible.String())
	}

	withoutMethod := &loginVisibleOutput{destination: io.Discard, openBrowser: func(string) error {
		t.Fatal("console URL opened outside console method")
		return nil
	}}
	if _, err := withoutMethod.Write([]byte(target + "\n")); err != nil {
		t.Fatal(err)
	}
}

func TestLoginVisibleOutputProjectsHostileProviderText(t *testing.T) {
	var visible bytes.Buffer
	opened := []string{}
	filter := &loginVisibleOutput{destination: &visible, openBrowser: func(target string) error {
		opened = append(opened, target)
		return nil
	}}
	chunks := [][]byte{
		[]byte("literal \\ ESC \x1b]8;;https://evil.example\x07 bidi \u202e zero \u200b line \u2028 para \u2029 invalid "),
		{0xff},
		[]byte(" UTF-8 split \xe2"),
		[]byte("\x82\xac\r\nOpen " + githubDeviceURL + "\n"),
	}
	for _, chunk := range chunks {
		if _, err := filter.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	if err := filter.flush(); err != nil {
		t.Fatal(err)
	}
	output := visible.String()
	if strings.ContainsAny(output, "\x00\x07\x1b\r\u2028\u2029\u202e\u200b") {
		t.Fatalf("hostile control reached terminal: %q", output)
	}
	for _, expected := range []string{`literal \\`, `\u001B`, `\u202E`, `\u200B`, `\u2028`, `\u2029`, "�", "€", githubDeviceURL} {
		if !strings.Contains(output, expected) {
			t.Fatalf("projected output %q lacks %q", output, expected)
		}
	}
	if !reflect.DeepEqual(opened, []string{githubDeviceURL}) {
		t.Fatalf("browser opens = %q", opened)
	}

	oversized := &loginVisibleOutput{destination: &visible}
	if written, err := oversized.Write(bytes.Repeat([]byte{'x'}, maxLoginVisibleLine+1)); !errors.Is(err, errLoginVisibleOutputLimit) || written != maxLoginVisibleBytes {
		t.Fatalf("oversized line written=%d error=%v", written, err)
	}
}

func TestHostBrowserCommandAcceptsOnlyReviewedLoginURLs(t *testing.T) {
	tests := []struct {
		goos       string
		executable string
		target     string
	}{
		{goos: "darwin", executable: "/usr/bin/open", target: githubDeviceURL},
		{goos: "linux", executable: "/usr/bin/xdg-open", target: githubDeviceURL},
		{goos: "darwin", executable: "/usr/bin/open", target: "https://device.sso.us-east-1.amazonaws.com/"},
		{goos: "linux", executable: "/usr/bin/xdg-open", target: "https://device.sso.us-gov-west-1.amazonaws.com/"},
		{goos: "darwin", executable: "/usr/bin/open", target: syntheticAWSConsoleAuthorizationURL("ap-northeast-1")},
	}
	for _, test := range tests {
		executable, args, err := hostBrowserCommand(test.goos, test.target)
		if err != nil || executable != test.executable || len(args) != 1 || args[0] != test.target {
			t.Fatalf("hostBrowserCommand(%q, %q) = %q, %q, %v", test.goos, test.target, executable, args, err)
		}
	}
	for _, target := range []string{
		"https://example.com/login",
		githubDeviceURL + "?next=example",
		"http://device.sso.us-east-1.amazonaws.com/",
		"https://device.sso.us-east-1.amazonaws.com",
		"https://device.sso.us-east-1.amazonaws.com/?code=secret",
		"https://device.sso.us-east-1.amazonaws.com/#fragment",
		"https://device.sso.US-EAST-1.amazonaws.com/",
		"https://device.sso.us-east-0.amazonaws.com/",
		"https://device.sso.us-east-1.amazonaws.com.evil.example/",
		strings.Replace(syntheticAWSConsoleAuthorizationURL("ap-northeast-1"), "response_type=code", "response_type=token", 1),
		syntheticAWSConsoleAuthorizationURL("ap-northeast-1") + "&scope=admin",
		strings.Replace(syntheticAWSConsoleAuthorizationURL("ap-northeast-1"), "signin.aws.amazon.com", "signin.aws.amazon.com.evil.example", 1),
	} {
		if _, _, err := hostBrowserCommand("darwin", target); err == nil {
			t.Fatalf("unsafe browser target %q was accepted", target)
		}
	}
	if _, _, err := hostBrowserCommand("windows", githubDeviceURL); err == nil {
		t.Fatal("unsupported host OS was accepted")
	}
}

func TestClassifyHostGitHubLoginFailuresUsesStableSecretFreeFaults(t *testing.T) {
	for _, driverErr := range []error{
		credentialhost.ErrGitHubLoginFailed,
		credentialhost.ErrGitHubTokenCapture,
		credentialhost.ErrGitHubLoginSetup,
		credentialhost.ErrGitHubAccountCapture,
		credentialhost.ErrGitHubLoginCleanup,
	} {
		t.Run(driverErr.Error(), func(t *testing.T) {
			err := classifyHostLoginError(driverErr, "github")
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "github_login_failed" || public.Retryable || strings.Contains(public.Message, driverErr.Error()) {
				t.Fatalf("classified fault = %+v, ok=%t", public, ok)
			}
		})
	}
	public, ok := fault.PublicCopy(classifyHostLoginError(credentialhost.ErrGitHubLoginCancelled, "github"))
	if !ok || public.Code != "github_login_cancelled" || public.Retryable {
		t.Fatalf("cancel fault = %+v, ok=%t", public, ok)
	}
	public, ok = fault.PublicCopy(classifyHostLoginError(hostCLIUnavailableError{provider: "github"}, "github"))
	if !ok || public.Code != "github_cli_unavailable" || public.Kind != fault.KindUnavailable || public.Retryable {
		t.Fatalf("CLI fault = %+v, ok=%t", public, ok)
	}
}

func TestClassifyHostAWSLoginFailuresUsesStableSecretFreeFaults(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		publicCode string
		kind       fault.Kind
	}{
		{name: "cancelled", err: context.Canceled, publicCode: "aws_sso_login_cancelled", kind: fault.KindRejected},
		{name: "timeout", err: context.DeadlineExceeded, publicCode: "aws_sso_login_timeout", kind: fault.KindRejected},
		{name: "invalid profile", err: credentialhost.ErrInvalidProfile, publicCode: "aws_sso_config_invalid", kind: fault.KindInvalidInput},
		{name: "command failed", err: credentialhost.ErrCommandFailed, publicCode: "aws_sso_login_failed", kind: fault.KindUnavailable},
		{name: "invalid cache", err: credentialhost.ErrInvalidCache, publicCode: "aws_sso_login_failed", kind: fault.KindUnavailable},
		{name: "executable changed", err: credentialhost.ErrInvalidExecutable, publicCode: "aws_cli_unavailable", kind: fault.KindUnavailable},
		{name: "CLI missing", err: hostCLIUnavailableError{provider: "aws"}, publicCode: "aws_cli_unavailable", kind: fault.KindUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := classifyHostLoginError(test.err, "aws")
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.publicCode || public.Kind != test.kind || public.Retryable ||
				strings.Contains(public.Message, test.err.Error()) {
				t.Fatalf("classified fault = %+v, ok=%t", public, ok)
			}
		})
	}
}

func TestClassifyHostDatadogLoginFailuresUsesStableSecretFreeFaults(t *testing.T) {
	tests := []struct {
		err  error
		code string
		kind fault.Kind
	}{
		{err: context.Canceled, code: "datadog_login_cancelled", kind: fault.KindRejected},
		{err: context.DeadlineExceeded, code: "datadog_login_timeout", kind: fault.KindRejected},
		{err: credentialhost.ErrPupLoginFailed, code: "datadog_login_failed", kind: fault.KindUnavailable},
		{err: credentialhost.ErrInvalidPupState, code: "datadog_login_failed", kind: fault.KindUnavailable},
		{err: credentialhost.ErrInvalidExecutable, code: "datadog_cli_unavailable", kind: fault.KindUnavailable},
		{err: hostCLIUnavailableError{provider: "datadog"}, code: "datadog_cli_unavailable", kind: fault.KindUnavailable},
	}
	for _, test := range tests {
		public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "datadog"))
		if !ok || public.Code != test.code || public.Kind != test.kind || public.Retryable ||
			strings.Contains(public.Message, test.err.Error()) {
			t.Fatalf("Datadog fault = %+v, ok=%t", public, ok)
		}
	}
}

func TestClassifyHostAWSConsoleLoginFailuresUsesDistinctFaults(t *testing.T) {
	tests := []struct {
		err  error
		code string
		kind fault.Kind
	}{
		{err: credentialhost.ErrConsoleLoginUnsupported, code: "aws_console_login_unsupported", kind: fault.KindUnsupported},
		{err: credentialhost.ErrInvalidProfile, code: "aws_console_config_invalid", kind: fault.KindInvalidInput},
		{err: context.Canceled, code: "aws_console_login_cancelled", kind: fault.KindRejected},
		{err: context.DeadlineExceeded, code: "aws_console_login_timeout", kind: fault.KindRejected},
		{err: credentialhost.ErrCommandFailed, code: "aws_console_login_failed", kind: fault.KindUnavailable},
	}
	for _, test := range tests {
		public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "aws", awsConsoleMethod))
		if !ok || public.Code != test.code || public.Kind != test.kind || public.Retryable {
			t.Fatalf("console fault = %+v, ok=%t", public, ok)
		}
	}
}

func TestInvalidBrokerAccountLabelDoesNotReachPublicError(t *testing.T) {
	label := "synthetic-secret-canary\n"
	_, err := validatedAccountLabel(&label)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_auth_broker_metadata" || strings.Contains(public.Error(), "synthetic-secret-canary") {
		t.Fatalf("public metadata fault = %+v, ok=%t", public, ok)
	}
}
