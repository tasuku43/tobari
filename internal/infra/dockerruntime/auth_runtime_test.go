package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
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

func syntheticClaudeAuthorizationURL() string {
	query := url.Values{
		"code":                  {"true"},
		"client_id":             {claudeLoginClientID},
		"response_type":         {"code"},
		"redirect_uri":          {claudeLoginRedirectURI},
		"scope":                 {claudeLoginScopes},
		"code_challenge_method": {"S256"},
		"code_challenge":        {strings.Repeat("A", 43)},
		"state":                 {strings.Repeat("B", 43)},
	}
	return claudeLoginURLPrefix + query.Encode()
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
		{name: "current", registry: "matching", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionCurrent, wantSummary: authbroker.WorkspaceActivationReady},
		{name: "broker stale", registry: "matching", bindingState: "stale", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionStale, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
		{name: "missing registry", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionMissing, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
		{name: "stale revision", registry: "stale", status: authbroker.ProviderStatus{Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: authDoctorRevision}, want: authbroker.WorkspaceProviderProjectionStale, wantSummary: authbroker.WorkspaceActivationReentryRequired, wantAction: true},
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
		{Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: authDoctorRevision},
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
	type testCase struct {
		name     string
		provider authbroker.Provider
		want     bool
	}
	tests := []testCase{
		{name: "other reused helper", provider: authbroker.Provider{ID: "other", Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionBuiltinHelper, Helper: "aws-sso"}}},
		{name: "stdin import", provider: authbroker.Provider{ID: authbroker.BuiltinGitHubProviderID, Acquisition: authbroker.Acquisition{Mode: authbroker.AcquisitionStdinImport}}},
	}
	for _, providerID := range authbroker.ReviewedLoginProviderIDs() {
		helper, found := authbroker.ReviewedLoginProviderHelper(providerID)
		if !found {
			t.Fatalf("reviewed provider %q has no helper", providerID)
		}
		tests = append(tests,
			testCase{
				name: providerID,
				provider: authbroker.Provider{
					ID: providerID,
					Acquisition: authbroker.Acquisition{
						Mode: authbroker.AcquisitionBuiltinHelper, Helper: helper,
					},
				},
				want: true,
			},
			testCase{
				name: providerID + " wrong helper",
				provider: authbroker.Provider{
					ID: providerID,
					Acquisition: authbroker.Acquisition{
						Mode: authbroker.AcquisitionBuiltinHelper, Helper: "wrong-helper",
					},
				},
			},
		)
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
	if strings.Count(visible.String(), loginBrowserOpenedFeedback) != 1 {
		t.Fatalf("browser-open feedback = %q", visible.String())
	}
	if err := filter.flush(); err != nil {
		t.Fatal(err)
	}
}

type codexLoginVisibleFixture struct {
	SchemaVersion int      `json:"schema_version"`
	Chunks        []string `json:"chunks"`
}

type codexLoginVisibleAnswer struct {
	SchemaVersion   int      `json:"schema_version"`
	TobariOpenCount int      `json:"tobari_browser_open_count"`
	PlainFragments  []string `json:"plain_fragments"`
}

func TestLoginVisibleOutputProjectsNativeCodexBrowserFlowWithoutOpeningURL(t *testing.T) {
	var fixture codexLoginVisibleFixture
	readLoginVisibleFixture(t, "openai_native_login_visible.json", &fixture)
	var answer codexLoginVisibleAnswer
	readLoginVisibleFixture(t, "openai_native_login_visible_answer.json", &answer)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 || len(fixture.Chunks) == 0 {
		t.Fatalf("fixture/answer version or chunks are invalid: %+v %+v", fixture, answer)
	}

	for _, color := range []bool{false, true} {
		t.Run(fmt.Sprintf("color=%t", color), func(t *testing.T) {
			var visible bytes.Buffer
			opened := []string{}
			filter := &loginVisibleOutput{
				destination: &visible,
				color:       color,
				openBrowser: func(target string) error {
					opened = append(opened, target)
					return nil
				},
			}
			for _, chunk := range fixture.Chunks {
				if _, err := filter.Write([]byte(chunk)); err != nil {
					t.Fatal(err)
				}
			}
			if err := filter.flush(); err != nil {
				t.Fatal(err)
			}
			if len(opened) != answer.TobariOpenCount || strings.Contains(visible.String(), loginBrowserOpenedFeedback) {
				t.Fatalf("opened=%q visible=%q", opened, visible.String())
			}
			plain := stripLoginOwnedStyles(visible.String())
			for _, fragment := range answer.PlainFragments {
				if !strings.Contains(plain, fragment) {
					t.Fatalf("plain output %q lacks %q", plain, fragment)
				}
			}
			if strings.Contains(visible.String(), "\x1b") || strings.Contains(visible.String(), `\u001B`) {
				t.Fatalf("plain output retained terminal controls: %q", visible.String())
			}
		})
	}
}

func TestProjectLoginVisibleTextAcceptsOnlyApprovedSGR(t *testing.T) {
	input := loginSGRUpstreamAccent + "approved" + loginSGRReset + " " + "\x1b[31munknown" + loginSGRReset
	for _, color := range []bool{false, true} {
		output := projectLoginVisibleText(input, color)
		plain := stripLoginOwnedStyles(output)
		if !strings.Contains(plain, `approved \u001B[31munknown`) || strings.Contains(plain, "\x1b") {
			t.Fatalf("color=%t projected output = %q", color, output)
		}
		if color != strings.Contains(output, loginStyleAccent) {
			t.Fatalf("color=%t accent output = %q", color, output)
		}
	}
}

func readLoginVisibleFixture(t *testing.T, name string, destination any) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, destination); err != nil {
		t.Fatal(err)
	}
}

func stripLoginOwnedStyles(value string) string {
	return strings.NewReplacer(
		loginSGRReset, "",
		loginStyleMuted, "",
		loginStyleAccent, "",
		loginStyleSuccess, "",
	).Replace(value)
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
	if strings.Contains(visible.String(), loginBrowserOpenedFeedback) {
		t.Fatalf("failed browser open reported success: %q", visible.String())
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

func TestLoginVisibleOutputProjectsAndOpensOnlyExactClaudeHyperlinkOnce(t *testing.T) {
	target := syntheticClaudeAuthorizationURL()
	approved := claudeHyperlinkPrefix + target + "\a" + target + claudeHyperlinkClose + "\n"
	var visible bytes.Buffer
	opened := []string{}
	filter := &loginVisibleOutput{destination: &visible, openBrowser: func(candidate string) error {
		opened = append(opened, candidate)
		return nil
	}}
	for _, line := range []string{
		strings.Replace(approved, "claude.com", "evil.example", 2),
		strings.Replace(approved, claudeLoginClientID, "wrong-client", 2),
		strings.Replace(approved, strings.Repeat("A", 43), "short", 2),
		approved,
		approved,
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
	if strings.Count(visible.String(), loginBrowserOpenedFeedback) != 1 ||
		!strings.Contains(visible.String(), "If the browser didn't open, visit: "+target) ||
		strings.ContainsAny(visible.String(), "\x1b\a") {
		t.Fatalf("visible output = %q", visible.String())
	}
}

type claudeLoginVisibleFixture struct {
	SchemaVersion               int      `json:"schema_version"`
	AuthorizationURLPlaceholder string   `json:"authorization_url_placeholder"`
	PromptVisibleAfterChunk     int      `json:"prompt_visible_after_chunk"`
	Chunks                      []string `json:"chunks"`
}

type claudeLoginVisibleAnswer struct {
	SchemaVersion   int      `json:"schema_version"`
	TobariOpenCount int      `json:"tobari_browser_open_count"`
	PlainFragments  []string `json:"plain_fragments"`
	AbsentFragments []string `json:"absent_fragments"`
}

func TestClaudeLoginVisibleOutputUsesFixedImmediateSecretFreeUI(t *testing.T) {
	var fixture claudeLoginVisibleFixture
	readLoginVisibleFixture(t, "anthropic_native_login_visible.json", &fixture)
	var answer claudeLoginVisibleAnswer
	readLoginVisibleFixture(t, "anthropic_native_login_visible_answer.json", &answer)
	if fixture.SchemaVersion != 1 || answer.SchemaVersion != 1 ||
		fixture.AuthorizationURLPlaceholder == "" || fixture.PromptVisibleAfterChunk <= 0 {
		t.Fatalf("fixture/answer is invalid: %+v %+v", fixture, answer)
	}
	target := syntheticClaudeAuthorizationURL()

	for _, color := range []bool{false, true} {
		t.Run(fmt.Sprintf("color=%t", color), func(t *testing.T) {
			var visible bytes.Buffer
			opened := []string{}
			filter := &loginVisibleOutput{
				destination: &visible,
				provider:    authbroker.BuiltinAnthropicProviderID,
				color:       color,
				openBrowser: func(candidate string) error {
					opened = append(opened, candidate)
					return nil
				},
			}
			for index, rawChunk := range fixture.Chunks {
				chunk := strings.ReplaceAll(rawChunk, fixture.AuthorizationURLPlaceholder, target)
				if _, err := filter.Write([]byte(chunk)); err != nil {
					t.Fatal(err)
				}
				if index+1 == fixture.PromptVisibleAfterChunk &&
					!strings.Contains(stripLoginOwnedStyles(visible.String()), "If Claude shows a code, paste it here:\r\n> ") {
					t.Fatalf("prompt was not visible before flush: %q", visible.String())
				}
			}
			if err := filter.flush(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(opened, []string{target}) || len(opened) != answer.TobariOpenCount {
				t.Fatalf("browser opens = %q", opened)
			}
			rawPlain := stripLoginOwnedStyles(visible.String())
			if hasBareLF(rawPlain) {
				t.Fatalf("Claude raw-mode output contains bare LF: %q", rawPlain)
			}
			plain := strings.ReplaceAll(rawPlain, claudeLoginTTYLineEnding, "\n")
			for _, fragment := range answer.PlainFragments {
				if !strings.Contains(plain, fragment) {
					t.Fatalf("plain output %q lacks %q", plain, fragment)
				}
			}
			for _, fragment := range append(answer.AbsentFragments, target) {
				if strings.Contains(plain, fragment) {
					t.Fatalf("plain output %q contains forbidden %q", plain, fragment)
				}
			}
			if strings.ContainsAny(plain, "\x00\x07\x1b\r") {
				t.Fatalf("terminal controls reached fixed UI: %q", plain)
			}
		})
	}
}

func TestClaudeLoginVisibleOutputShowsExactURLOnlyWhenBrowserOpenFails(t *testing.T) {
	target := syntheticClaudeAuthorizationURL()
	line := claudeHyperlinkPrefix + target + "\a" + target + claudeHyperlinkClose + "\r\n"
	var visible bytes.Buffer
	filter := &loginVisibleOutput{
		destination: &visible,
		provider:    authbroker.BuiltinAnthropicProviderID,
		openBrowser: func(string) error {
			return os.ErrNotExist
		},
	}
	for _, chunk := range []string{"Opening browser to sign in…\r\n", line, "Paste code here if prompted > "} {
		if _, err := filter.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	rawPlain := stripLoginOwnedStyles(visible.String())
	plain := strings.ReplaceAll(rawPlain, claudeLoginTTYLineEnding, "\n")
	if hasBareLF(rawPlain) || strings.Count(plain, target) != 1 || !strings.Contains(plain, "! Browser did not open.\nVisit: "+target) ||
		!strings.HasSuffix(plain, "If Claude shows a code, paste it here:\n> ") {
		t.Fatalf("fallback output = %q", plain)
	}
}

func TestClaudeLoginVisibleOutputFramesUnrecognizedLinesForRawTTY(t *testing.T) {
	var visible bytes.Buffer
	filter := &loginVisibleOutput{
		destination: &visible,
		provider:    authbroker.BuiltinAnthropicProviderID,
	}
	for _, chunk := range []string{claudeLoginPastePrompt, "\r\nInvalid code. Please retry.\r\n"} {
		if _, err := filter.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	want := claudeLoginPromptFeedback + claudeLoginTTYLineEnding + "Invalid code. Please retry." + claudeLoginTTYLineEnding
	if output := visible.String(); output != want || hasBareLF(output) {
		t.Fatalf("raw TTY framing = %q", output)
	}
}

func hasBareLF(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] == '\n' && (index == 0 || value[index-1] != '\r') {
			return true
		}
	}
	return false
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
		{goos: "darwin", executable: "/usr/bin/open", target: syntheticClaudeAuthorizationURL()},
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
		"https://auth.openai.com/codex/device",
		"https://auth.openai.com/oauth/authorize?response_type=code&state=synthetic",
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
		syntheticClaudeAuthorizationURL() + "#fragment",
		strings.Replace(syntheticClaudeAuthorizationURL(), "claude.com", "claude.com.evil.example", 1),
		strings.Replace(syntheticClaudeAuthorizationURL(), claudeLoginClientID, "wrong-client", 1),
		strings.Replace(syntheticClaudeAuthorizationURL(), "code=true", "code=false", 1),
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

func TestClassifyHostOpenAIAvailabilityFailuresExposeOnlyFixedDiagnosticStage(t *testing.T) {
	secretCanary := "synthetic-local-path-and-output-canary"
	tests := []struct {
		name  string
		err   error
		stage hostCLIUnavailableStage
	}{
		{
			name:  "resolver",
			err:   hostCLIUnavailableError{provider: "openai", stage: hostCLIStageCodexChatGPTAppBundle},
			stage: hostCLIStageCodexChatGPTAppBundle,
		},
		{
			name:  "executable identity",
			err:   fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrCodexExecutable),
			stage: hostCLIStageCodexExecutableIdentity,
		},
		{
			name:  "version observation",
			err:   fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrCodexVersion),
			stage: hostCLIStageCodexVersionObservation,
		},
		{
			name:  "unknown stage",
			err:   hostCLIUnavailableError{provider: "openai", stage: hostCLIUnavailableStage(secretCanary)},
			stage: hostCLIStageDriverDependency,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "openai"))
			if !ok || public.Code != "openai_cli_unavailable" || public.Kind != fault.KindUnavailable || public.Retryable {
				t.Fatalf("classified fault = %+v, ok=%t", public, ok)
			}
			if !strings.Contains(public.Message, string(test.stage)) || strings.Contains(public.Error(), secretCanary) ||
				strings.Contains(public.Message, credentialhost.ErrCodexExecutable.Error()) ||
				strings.Contains(public.Message, credentialhost.ErrCodexVersion.Error()) {
				t.Fatalf("public diagnostic = %+v", public)
			}
		})
	}
}

func TestClassifyAnthropicAvailabilityFailuresExposeOnlyContextRuntimeStages(t *testing.T) {
	secretCanary := "synthetic-context-image-and-output-canary"
	tests := []struct {
		name  string
		err   error
		stage hostCLIUnavailableStage
	}{
		{name: "context selection", err: hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeContextSelection}, stage: hostCLIStageClaudeContextSelection},
		{name: "image contract", err: hostCLIUnavailableError{provider: "anthropic", stage: hostCLIStageClaudeImageContract}, stage: hostCLIStageClaudeImageContract},
		{name: "executable identity", err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeExecutable), stage: hostCLIStageClaudeExecutableIdentity},
		{name: "version observation", err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeVersion), stage: hostCLIStageClaudeVersionObservation},
		{name: "unknown stage", err: hostCLIUnavailableError{provider: "anthropic", stage: hostCLIUnavailableStage(secretCanary)}, stage: hostCLIStageDriverDependency},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "anthropic"))
			if !ok || public.Code != "anthropic_cli_unavailable" || public.Kind != fault.KindUnavailable || public.Retryable {
				t.Fatalf("classified fault = %+v, ok=%t", public, ok)
			}
			if !strings.Contains(public.Message, string(test.stage)) || !strings.Contains(public.Message, "Context runtime") ||
				strings.Contains(public.Error(), secretCanary) || strings.Contains(public.Message, "trusted-host") ||
				len(public.NextActions) != 1 || public.NextActions[0].Command != "help runtime" {
				t.Fatalf("public diagnostic = %+v", public)
			}
		})
	}
}

func TestClassifyAnthropicRuntimeFailuresUsesDistinctSecretFreeFaults(t *testing.T) {
	secretCanary := "synthetic-claude-runtime-detail-canary"
	tests := []struct {
		err     error
		code    string
		command string
	}{
		{err: context.DeadlineExceeded, code: "anthropic_login_timeout", command: "auth login"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeLoginSetup), code: "anthropic_login_setup_failed", command: "doctor"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeLoginFailed), code: "anthropic_authorization_failed", command: "auth login"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeOutputLimit), code: "anthropic_login_output_failed", command: "help runtime"},
		{err: fmt.Errorf("%s: %w", secretCanary, errLoginVisibleOutputLimit), code: "anthropic_login_output_failed", command: "help runtime"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeTokenCapture), code: "anthropic_credential_capture_failed", command: "auth login"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrInvalidClaudeNativeCredential), code: "anthropic_credential_capture_failed", command: "auth login"},
		{err: fmt.Errorf("%s: %w", secretCanary, credentialhost.ErrClaudeLoginCleanup), code: "anthropic_login_cleanup_failed", command: "doctor"},
		{err: errHostCredentialResult, code: "anthropic_login_failed", command: "auth login"},
	}
	for _, test := range tests {
		public, ok := fault.PublicCopy(classifyHostLoginError(test.err, "anthropic"))
		if !ok || public.Code != test.code || public.Kind == "" || public.Retryable ||
			len(public.NextActions) != 1 || public.NextActions[0].Command != test.command ||
			strings.Contains(public.Error(), secretCanary) {
			t.Fatalf("classified fault = %+v, ok=%t", public, ok)
		}
	}

	joined := errors.Join(context.DeadlineExceeded, credentialhost.ErrClaudeLoginCleanup)
	public, ok := fault.PublicCopy(classifyHostLoginError(joined, "anthropic"))
	if !ok || public.Code != "anthropic_login_timeout" {
		t.Fatalf("joined timeout classification = %+v, ok=%t", public, ok)
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
