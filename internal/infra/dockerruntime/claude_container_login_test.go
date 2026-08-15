package dockerruntime

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type claudeContainerRunner struct {
	calls             [][]string
	executable        []byte
	credentialArchive []byte
	cleanupErr        error
	waitForLoginEnd   bool
	loginInput        io.Reader
}

func (r *claudeContainerRunner) Run(
	ctx context.Context, arguments, _ []string, stdin io.Reader, stdout, _ io.Writer,
) error {
	r.calls = append(r.calls, append([]string(nil), arguments...))
	switch {
	case reflect.DeepEqual(arguments[:min(2, len(arguments))], []string{"image", "inspect"}):
		_, _ = io.WriteString(stdout, "sha256:"+strings.Repeat("a", 64)+"\n")
	case len(arguments) >= 4 && arguments[0] == "container" && arguments[1] == "exec" && arguments[len(arguments)-1] == "--version":
		_, _ = io.WriteString(stdout, "2.1.220 (Claude Code)\n")
	case len(arguments) == 4 && arguments[0] == "container" && arguments[1] == "cp" && strings.HasSuffix(arguments[2], ":/usr/local/bin/claude"):
		_, _ = stdout.Write(tarFixture("usr/local/bin/claude", 0o755, r.executable))
	case len(arguments) >= 2 && arguments[0] == "container" && arguments[1] == "exec" && slices.Contains(arguments, "--claudeai"):
		r.loginInput = stdin
		if r.waitForLoginEnd {
			<-ctx.Done()
			return ctx.Err()
		}
		_, _ = io.WriteString(stdout, "Opening browser to sign in…\n")
		_, _ = io.Copy(io.Discard, stdin)
		_, _ = io.WriteString(stdout, "Login successful.\n")
	case len(arguments) == 4 && arguments[0] == "container" && arguments[1] == "cp" && strings.HasSuffix(arguments[2], ":/var/lib/tobari/.claude/.credentials.json"):
		_, _ = stdout.Write(r.credentialArchive)
	case len(arguments) >= 3 && arguments[0] == "container" && arguments[1] == "rm":
		return r.cleanupErr
	}
	return nil
}

func (r *claudeContainerRunner) Output(_ context.Context, arguments, _ []string) ([]byte, error) {
	if len(arguments) > 1 && arguments[0] == "image" && arguments[1] == "inspect" {
		return compatibleImageInspection(), nil
	}
	return nil, fmt.Errorf("unexpected Output call: %v", arguments)
}

func tarFixture(name string, mode int64, content []byte) []byte {
	var output bytes.Buffer
	archive := tar.NewWriter(&output)
	_ = archive.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(content)), Typeflag: tar.TypeReg})
	_, _ = archive.Write(content)
	_ = archive.Close()
	return output.Bytes()
}

func claudeNativeLoginFixture() []byte {
	return []byte(`{"claudeAiOauth":{"access` + `Token":"dummy-access-token","refresh` + `Token":"dummy-refresh-token","expiresAt":4102444800000,"refreshTokenExpiresAt":4102445800000,"scopes":["org:create_api_key","user:profile","user:inference","user:sessions:claude_code","user:mcp_servers","user:file_upload"],"subscriptionType":"max","rateLimitTier":"default_claude_max_5x"}}`)
}

func claudeNativeLoginScopes() []string {
	return []string{
		"org:create_api_key",
		"user:file_upload",
		"user:inference",
		"user:mcp_servers",
		"user:profile",
		"user:sessions:claude_code",
	}
}

func newClaudeContainerRuntime(t *testing.T, runner *claudeContainerRunner) (*Runtime, string) {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := runtime.activeContext()
	if err != nil {
		t.Fatal(err)
	}
	return runtime, manifest.ID
}

func TestClaudeContainerLoginUsesIsolatedContextImageAndCanonicalizesNativeState(t *testing.T) {
	executable := []byte("synthetic reviewed Claude executable bytes")
	runner := &claudeContainerRunner{
		executable: executable,
		credentialArchive: tarFixture(
			"var/lib/tobari/.claude/.credentials.json", 0o600, claudeNativeLoginFixture(),
		),
	}
	runtime, contextID := newClaudeContainerRuntime(t, runner)
	var visibleBuffer bytes.Buffer
	visible := &loginVisibleOutput{
		destination:       &visibleBuffer,
		provider:          authbroker.BuiltinAnthropicProviderID,
		claudeOAuthScopes: claudeNativeLoginScopes(),
	}
	input := strings.NewReader("paste-code\n")
	payload, err := runtime.loginClaudeInContextContainer(
		context.Background(), contextID, input, visible,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer payload.clear()
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(executable))
	if payload.accountLabel != credentialhost.ClaudeNativeAccountLabel ||
		payload.driverID != credentialhost.ClaudeNativeDriverID || payload.driverRevision != wantDigest {
		t.Fatalf("payload metadata = %#v", payload)
	}
	decoded, err := credentialhost.DecodeClaudeNativeCredential(payload.secret)
	if err != nil || decoded.DriverRevision() != wantDigest {
		t.Fatalf("canonical state=%#v error=%v", decoded, err)
	}
	wantProgress := []string{
		claudeLoginOpeningFeedback,
		"Login successful.",
		claudeLoginCaptureFeedback,
	}
	position := 0
	for _, expected := range wantProgress {
		index := strings.Index(visibleBuffer.String()[position:], expected)
		if index < 0 {
			t.Fatalf("visible progress = %q; missing %q", visibleBuffer.String(), expected)
		}
		position += index + len(expected)
	}
	if runner.loginInput != input {
		t.Fatalf("Claude login input was wrapped: got %T, want original %T", runner.loginInput, input)
	}

	var create, login, cleanup []string
	for _, call := range runner.calls {
		switch {
		case len(call) >= 2 && call[0] == "container" && call[1] == "create":
			create = call
		case len(call) >= 2 && call[0] == "container" && call[1] == "exec" && slices.Contains(call, "--claudeai"):
			login = call
		case len(call) >= 2 && call[0] == "container" && call[1] == "rm":
			cleanup = call
		}
	}
	if len(create) == 0 || len(login) == 0 || len(cleanup) == 0 ||
		!containsArgSequence(create, "--cap-drop", "ALL") || !containsArgSequence(create, "--security-opt", "no-new-privileges") ||
		!containsArgSequence(create, "--memory", claudeLoginMemoryLimit) ||
		!containsArgSequence(create, "--memory-swap", claudeLoginMemoryLimit) ||
		!containsArgSequence(create, "--entrypoint", "/usr/bin/tini") ||
		!slices.Equal(create[len(create)-3:], []string{"--", "/usr/bin/sleep", "infinity"}) ||
		!containsArgSequence(login, "/usr/local/bin/claude", "auth", "login", "--claudeai") {
		t.Fatalf("container calls = %#v", runner.calls)
	}
	joined := strings.Join(create, " ")
	for _, forbidden := range []string{"--mount", "--volume", "/var/run/docker.sock", runtime.configDirectory, runtime.stateDirectory, runtime.dataDirectory} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("isolated create argv contains %q: %v", forbidden, create)
		}
	}
}

func TestClaudeContainerLoginRejectsGrantedScopeOutsideObservedRequest(t *testing.T) {
	runner := &claudeContainerRunner{
		executable: []byte("synthetic executable"),
		credentialArchive: tarFixture(
			".credentials.json", 0o600, claudeNativeLoginFixture(),
		),
	}
	runtime, contextID := newClaudeContainerRuntime(t, runner)
	visible := &loginVisibleOutput{
		destination:       io.Discard,
		provider:          authbroker.BuiltinAnthropicProviderID,
		claudeOAuthScopes: []string{"user:inference"},
	}
	payload, err := runtime.loginClaudeInContextContainer(
		context.Background(), contextID, strings.NewReader("code\n"), visible,
	)
	if len(payload.secret) != 0 || !errors.Is(err, credentialhost.ErrInvalidClaudeNativeCredential) {
		t.Fatalf("payload=%#v error=%v", payload, err)
	}
	var diagnostic *credentialhost.ClaudeCredentialCaptureError
	if !errors.As(err, &diagnostic) || diagnostic.DiagnosticStage() != credentialhost.ClaudeCaptureOAuthScopeSet {
		t.Fatalf("diagnostic=%#v error=%v", diagnostic, err)
	}
}

func TestClaudeCredentialArchiveReportsOnlySecretFreeDiagnosticStages(t *testing.T) {
	tests := []struct {
		name    string
		archive []byte
		stage   credentialhost.ClaudeCredentialCaptureStage
	}{
		{name: "invalid archive", archive: []byte("not a tar archive"), stage: credentialhost.ClaudeCaptureArchiveEnvelope},
		{name: "public permissions", archive: tarFixture(".credentials.json", 0o644, claudeNativeLoginFixture()), stage: credentialhost.ClaudeCaptureFilePermissions},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, err := readClaudeCredentialArchive(test.archive)
			if len(content) != 0 || !errors.Is(err, credentialhost.ErrClaudeTokenCapture) {
				t.Fatalf("content=%q error=%v", content, err)
			}
			var diagnostic *credentialhost.ClaudeCredentialCaptureError
			if !errors.As(err, &diagnostic) || diagnostic.DiagnosticStage() != test.stage || strings.Contains(err.Error(), "canary") {
				t.Fatalf("diagnostic=%#v error=%v", diagnostic, err)
			}
		})
	}
}

func containsArgSequence(arguments []string, sequence ...string) bool {
	for index := 0; index+len(sequence) <= len(arguments); index++ {
		if slices.Equal(arguments[index:index+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func TestClaudeContainerLoginCleanupFailurePreventsCredentialCommit(t *testing.T) {
	runner := &claudeContainerRunner{
		executable:        []byte("synthetic executable"),
		credentialArchive: tarFixture(".credentials.json", 0o600, claudeNativeLoginFixture()),
		cleanupErr:        os.ErrPermission,
	}
	runtime, contextID := newClaudeContainerRuntime(t, runner)
	payload, err := runtime.loginClaudeInContextContainer(context.Background(), contextID, strings.NewReader("code\n"), io.Discard)
	if !errors.Is(err, credentialhost.ErrClaudeLoginCleanup) || len(payload.secret) != 0 {
		t.Fatalf("payload=%#v error=%v", payload, err)
	}
}

func TestClaudeContainerLoginPreservesDeadlineWhenCleanupAlsoFails(t *testing.T) {
	runner := &claudeContainerRunner{
		executable:      []byte("synthetic executable"),
		cleanupErr:      os.ErrPermission,
		waitForLoginEnd: true,
	}
	runtime, contextID := newClaudeContainerRuntime(t, runner)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	payload, err := runtime.loginClaudeInContextContainer(ctx, contextID, strings.NewReader(""), io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, credentialhost.ErrClaudeLoginCleanup) || len(payload.secret) != 0 {
		t.Fatalf("payload=%#v error=%v", payload, err)
	}
}
