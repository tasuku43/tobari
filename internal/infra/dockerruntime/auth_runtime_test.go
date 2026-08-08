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
	filter := newLoginOutputFilter(&visible)
	chunks := []string{
		"Open this URL in your browser:\nhttps://github.com/login/device\n",
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
	line, found := filter.responseLine()
	if !found || string(line) != `{"schema_version":1,"ok":true,"provider":"github"}` {
		t.Fatalf("captured result = %q, found=%t", line, found)
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
