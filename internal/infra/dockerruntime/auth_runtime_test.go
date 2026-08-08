package dockerruntime

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
)

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

func TestInteractiveBrokerLoginRequiresRealTerminalBeforeDockerExec(t *testing.T) {
	t.Parallel()
	runner := &brokerProtocolRunner{}
	runtime := &Runtime{runner: runner}
	_, err := runtime.runInteractiveBrokerLogin(
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

func TestBuildAuthResultDeclaresContextCredentialActivationContract(t *testing.T) {
	result, err := buildAuthResult(
		authbroker.TaskImport,
		"default",
		"018bcfe5-687b-7000-8000-000000000099",
		"github",
		brokerControlResponse{Revision: strings.Repeat("a", 64)},
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.WorkspaceActivation.State != authbroker.WorkspaceActivationReentryRequired ||
		result.WorkspaceActivation.Guidance != authbroker.ContextAuthActivationGuidance {
		t.Fatalf("Workspace activation = %+v", result.WorkspaceActivation)
	}
}

func TestLoginOutputFilterPreservesTTYStreamAndSuppressesMachineResult(t *testing.T) {
	var visible bytes.Buffer
	opened := []string{}
	filter := newLoginOutputFilter(&visible, func(target string) error {
		opened = append(opened, target)
		return nil
	})
	chunks := []string{
		"! First copy your one-time code: SYNTH-ETIC\nOpen this URL in your browser:\nhttps://github.com/login/de",
		"vice\n! Authentication credentials saved in plain text\n",
		"diagnostic: ! Authentication credentials saved in plain text\n",
		"Waiting for authentication...\n",
		loginResultPrefix[:9],
		loginResultPrefix[9:] + `{"schema_version":1,"ok":true,"provider":"github"}` + "\r\n",
	}
	for _, chunk := range chunks {
		if _, err := filter.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(visible.String(), "schema_version") || strings.Contains(visible.String(), "TOBARI_AUTH_BROKER_RESULT") {
		t.Fatalf("machine response reached public TTY output: %q", visible.String())
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
	line, found := filter.responseLine()
	if !found || string(line) != `{"schema_version":1,"ok":true,"provider":"github"}` {
		t.Fatalf("captured result = %q, found=%t", line, found)
	}
}

func TestLoginOutputFilterFallsBackToOneFixedManualURL(t *testing.T) {
	var visible bytes.Buffer
	opens := 0
	filter := newLoginOutputFilter(&visible, func(target string) error {
		opens++
		if target != githubDeviceURL {
			t.Fatalf("browser target = %q", target)
		}
		return os.ErrNotExist
	})
	if _, err := filter.Write([]byte(
		"Open https://example.com/login manually\n" + githubDeviceURL + "\n" + githubDeviceURL + "\n",
	)); err != nil {
		t.Fatal(err)
	}
	_, _ = filter.responseLine()
	if opens != 1 {
		t.Fatalf("browser opens = %d, want 1", opens)
	}
	if strings.Count(visible.String(), githubManualBrowserFallback) != 1 || !strings.Contains(visible.String(), "https://example.com/login") {
		t.Fatalf("visible output = %q", visible.String())
	}
}

func TestHostBrowserCommandAcceptsOnlyFixedGitHubDeviceURL(t *testing.T) {
	tests := []struct {
		goos       string
		executable string
	}{
		{goos: "darwin", executable: "/usr/bin/open"},
		{goos: "linux", executable: "xdg-open"},
	}
	for _, test := range tests {
		executable, args, err := hostBrowserCommand(test.goos, githubDeviceURL)
		if err != nil || executable != test.executable || len(args) != 1 || args[0] != githubDeviceURL {
			t.Fatalf("hostBrowserCommand(%q) = %q, %q, %v", test.goos, executable, args, err)
		}
	}
	for _, target := range []string{"https://example.com/login", githubDeviceURL + "?next=example"} {
		if _, _, err := hostBrowserCommand("darwin", target); err == nil {
			t.Fatalf("unsafe browser target %q was accepted", target)
		}
	}
	if _, _, err := hostBrowserCommand("windows", githubDeviceURL); err == nil {
		t.Fatal("unsupported host OS was accepted")
	}
}

func TestClassifyBrokerLoginFailuresUsesStableSecretFreeFaults(t *testing.T) {
	for _, code := range []string{
		"login_failed", "token_capture_failed", "login_setup_failed",
		"account_capture_failed", "login_cleanup_failed",
	} {
		t.Run(code, func(t *testing.T) {
			err := classifyBrokerError(brokerControlError{Code: code}, "auth login github")
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "github_login_failed" || public.Retryable || strings.Contains(public.Message, code) {
				t.Fatalf("classified fault = %+v, ok=%t", public, ok)
			}
		})
	}
	public, ok := fault.PublicCopy(classifyBrokerError(brokerControlError{Code: "login_cancelled"}, "auth login github"))
	if !ok || public.Code != "github_login_cancelled" || public.Retryable {
		t.Fatalf("cancel fault = %+v, ok=%t", public, ok)
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
