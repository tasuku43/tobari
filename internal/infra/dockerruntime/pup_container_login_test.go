package dockerruntime

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type pupContainerRunner struct {
	calls              [][]string
	executable         []byte
	version            string
	preflightErr       error
	status             string
	invalidNativeState bool
	cleanupErr         error
}

type unavailablePupContainerRunner struct{ calls int }

func (r *unavailablePupContainerRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("synthetic image unavailable")
}

func (r *unavailablePupContainerRunner) Run(_ context.Context, arguments []string, _ []string, _ io.Reader, _ io.Writer, stderr io.Writer) error {
	if len(arguments) >= 2 && arguments[0] == "image" && arguments[1] == "inspect" {
		_, _ = io.WriteString(stderr, "Error: No such image: "+arguments[len(arguments)-1])
		return errors.New("synthetic image unavailable")
	}
	r.calls++
	return nil
}

func (r *pupContainerRunner) Output(_ context.Context, arguments, _ []string) ([]byte, error) {
	if len(arguments) > 1 && arguments[0] == "image" && arguments[1] == "inspect" {
		return compatibleImageInspection(), nil
	}
	return nil, fmt.Errorf("unexpected output call: %v", arguments)
}

func (r *pupContainerRunner) Run(
	_ context.Context, arguments, _ []string, stdin io.Reader, stdout, stderr io.Writer,
) error {
	r.calls = append(r.calls, append([]string(nil), arguments...))
	imageID := "sha256:" + strings.Repeat("a", 64)
	switch {
	case reflect.DeepEqual(arguments[:min(2, len(arguments))], []string{"image", "inspect"}):
		_, _ = io.WriteString(stdout, strings.ReplaceAll(string(compatibleImageInspection()), strings.Repeat("c", 64), strings.Repeat("a", 64)))
	case len(arguments) >= 4 && arguments[0] == "container" && arguments[1] == "inspect":
		_, _ = io.WriteString(stdout, imageID+"\n")
	case len(arguments) >= 4 && arguments[0] == "container" && arguments[1] == "start" && arguments[2] == "--attach":
		if r.preflightErr != nil {
			return r.preflightErr
		}
		version := r.version
		if version == "" {
			version = "pup 1.11.0\n"
		}
		_, _ = io.WriteString(stdout, version)
	case len(arguments) == 4 && arguments[0] == "container" && arguments[1] == "cp" && strings.HasSuffix(arguments[2], pupExecutablePath):
		_, _ = stdout.Write(tarFixture("usr/local/bin/pup", 0o755, r.executable))
	case len(arguments) >= 2 && arguments[0] == "container" && arguments[1] == "exec" && slices.Contains(arguments, "login"):
		_, _ = io.WriteString(stderr, syntheticPupAuthorizationLine()+"\n")
		callback, err := io.ReadAll(stdin)
		if err != nil || !strings.Contains(string(callback), "code=synthetic-code") {
			return credentialhost.ErrPupLoginFailed
		}
		_, _ = io.WriteString(stderr, "Login successful.\n")
	case len(arguments) >= 2 && arguments[0] == "container" && arguments[1] == "exec" && slices.Contains(arguments, "status"):
		status := r.status
		if status == "" {
			status = `{"authenticated":true,"expires_at":"2030-01-01T00:00:00Z","has_refresh":true,"org":null,"scopes":["dashboards_read","metrics_read"],"site":"datadoghq.com","status":"valid","token_type":"Bearer"}`
		}
		_, _ = io.WriteString(stdout, status)
	case len(arguments) == 4 && arguments[0] == "container" && arguments[1] == "cp":
		name := pathBase(arguments[2])
		content := pupCredentialFixture(name)
		if r.invalidNativeState && name == "client_datadoghq_com.json" {
			content = []byte(`{}`)
		}
		_, _ = stdout.Write(tarFixture(name, 0o600, content))
	case len(arguments) >= 3 && arguments[0] == "container" && arguments[1] == "rm":
		return r.cleanupErr
	}
	return nil
}

func newPupContextRuntime(t *testing.T, runner commandRunner) (*Runtime, string) {
	t.Helper()
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
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

func pathBase(value string) string {
	index := strings.LastIndex(value, "/")
	if index < 0 {
		return value
	}
	return value[index+1:]
}

func pupCredentialFixture(name string) []byte {
	switch name {
	case "client_datadoghq_com.json":
		return []byte(`{"client_id":"client-example-123","client_name":"datadog-pup-cli","redirect_uris":["http://127.0.0.1:8000/oauth/callback"],"registered_at":1800000000,"site":"datadoghq.com"}`)
	case "tokens_datadoghq_com.json":
		return []byte(`{"__default__":{"access_token":"dummy-access-token","refresh_token":"dummy-refresh-token","token_type":"Bearer","expires_in":3600,"issued_at":1800000001,"scope":"dashboards_read metrics_read","client_id":"client-example-123"}}`)
	case "sessions.json":
		return []byte(`[{"site":"datadoghq.com","org":null}]`)
	default:
		return nil
	}
}

func syntheticPupAuthorizationLine() string {
	return "If the browser doesn't open, visit: https://app.datadoghq.com/oauth2/v1/authorize?response_type=code&client_id=client-example-123&redirect_uri=http%3A%2F%2F127.0.0.1%3A8000%2Foauth%2Fcallback&state=" + strings.Repeat("s", 32) + "&scope=dashboards_read+metrics_read&code_challenge=" + strings.Repeat("c", 43) + "&code_challenge_method=S256"
}

func TestPupAuthorizationURLValidationIsExactButScopeNamesRemainDynamic(t *testing.T) {
	target := strings.TrimPrefix(syntheticPupAuthorizationLine(), "If the browser doesn't open, visit: ")
	if !validPupLoginAuthorizationURL(target) || !validLoginBrowserTarget(target) {
		t.Fatalf("reviewed pup authorization URL was rejected: %s", target)
	}
	for name, changed := range map[string]string{
		"host":           strings.Replace(target, "app.datadoghq.com", "example.com", 1),
		"path":           strings.Replace(target, "/oauth2/v1/authorize", "/other", 1),
		"redirect":       strings.Replace(target, "127.0.0.1%3A8000", "127.0.0.1%3A8080", 1),
		"scope ordering": strings.Replace(target, "dashboards_read+metrics_read", "metrics_read+dashboards_read", 1),
		"extra query":    target + "&extra=value",
	} {
		t.Run(name, func(t *testing.T) {
			if validPupLoginAuthorizationURL(changed) {
				t.Fatalf("changed pup authorization URL accepted: %s", changed)
			}
		})
	}
}

type fakePupRelay struct {
	completed bool
	closed    bool
}

func (r *fakePupRelay) Complete(err error) { r.completed = err == nil }
func (r *fakePupRelay) Close() error       { r.closed = true; return nil }

func TestPupContainerLoginUsesSelectedContextImageNativeStateAndNoHostMount(t *testing.T) {
	executable := []byte("synthetic reviewed pup executable bytes")
	runner := &pupContainerRunner{executable: executable}
	runtime, contextID := newPupContextRuntime(t, runner)
	relay := &fakePupRelay{}
	runtime.pupRelayFactory = func(_ context.Context, input io.WriteCloser) (pupLoginRelay, error) {
		go func() {
			_, _ = io.WriteString(input, "http://127.0.0.1:8000/oauth/callback?code=synthetic-code&state="+strings.Repeat("s", 32)+"\n")
			_ = input.Close()
		}()
		return relay, nil
	}
	var visible strings.Builder
	payload, err := runtime.loginPupInContextContainer(context.Background(), contextID, strings.NewReader("ignored"), &visible)
	if err != nil {
		t.Fatal(err)
	}
	defer payload.clear()
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(executable))
	if payload.accountLabel != credentialhost.PupAccountLabel || payload.driverID != credentialhost.PupDriverID ||
		payload.driverRevision != wantDigest || !relay.completed || !relay.closed {
		t.Fatalf("payload=%#v relay=%#v", payload, relay)
	}
	if !strings.Contains(visible.String(), pupLoginValidationFeedback) || strings.Contains(visible.String(), "dummy-access-token") {
		t.Fatalf("visible output = %q", visible.String())
	}
	var preflightCreate, loginCreate, login []string
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "container" && call[1] == "create" {
			if slices.Contains(call, componentLabel+"=pup-login-preflight") {
				preflightCreate = call
			} else if slices.Contains(call, componentLabel+"=pup-login") {
				loginCreate = call
			}
		}
		if len(call) >= 2 && call[0] == "container" && call[1] == "exec" && slices.Contains(call, "login") {
			login = call
		}
	}
	if len(preflightCreate) == 0 || len(loginCreate) == 0 || len(login) == 0 ||
		!containsArgSequence(preflightCreate, "--network", "none") ||
		!containsArgSequence(preflightCreate, "--entrypoint", pupExecutablePath) ||
		!slices.Contains(preflightCreate, "sha256:"+strings.Repeat("a", 64)) ||
		!containsArgSequence(loginCreate, "--cap-drop", "ALL") ||
		!containsArgSequence(loginCreate, "--security-opt", "no-new-privileges") ||
		!slices.Contains(loginCreate, "sha256:"+strings.Repeat("a", 64)) ||
		!containsArgSequence(login, pupExecutablePath, "--no-agent", "auth", "login", "--site", credentialhost.PupSite, "--callback-port", "8000") ||
		!containsArgSequence(login, "--env", "BROWSER=/bin/false") {
		t.Fatalf("container calls = %#v", runner.calls)
	}
	joined := strings.Join(loginCreate, " ")
	for _, forbidden := range []string{"--mount", "--volume", "/var/run/docker.sock", runtime.configDirectory, runtime.stateDirectory, runtime.dataDirectory} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("isolated create argv contains %q: %v", forbidden, loginCreate)
		}
	}
}

func TestPupContainerLoginFailsBeforeContainerMutationWhenSelectedImageIsUnavailable(t *testing.T) {
	runner := &unavailablePupContainerRunner{}
	runtime, contextID := newPupContextRuntime(t, runner)
	payload, err := runtime.loginPupInContextContainer(context.Background(), contextID, strings.NewReader(""), io.Discard)
	payload.clear()
	var unavailable hostCLIUnavailableError
	if !errors.As(err, &unavailable) || unavailable.stage != hostCLIStagePupImageContract || runner.calls != 0 {
		t.Fatalf("error=%v stage=%q mutation calls=%d", err, unavailable.stage, runner.calls)
	}
}

func TestPupContainerLoginRejectsInvalidVersionBeforeCredentialContainer(t *testing.T) {
	runner := &pupContainerRunner{executable: []byte("synthetic pup"), version: "pup development build\n"}
	runtime, contextID := newPupContextRuntime(t, runner)
	payload, err := runtime.loginPupInContextContainer(context.Background(), contextID, strings.NewReader(""), io.Discard)
	payload.clear()
	var unavailable hostCLIUnavailableError
	if !errors.As(err, &unavailable) || unavailable.stage != hostCLIStagePupVersionObservation {
		t.Fatalf("error=%v stage=%q", err, unavailable.stage)
	}
	for _, call := range runner.calls {
		if slices.Contains(call, componentLabel+"=pup-login") {
			t.Fatalf("credential-bearing login container created after invalid preflight: %v", call)
		}
	}
}

func TestPupContainerLoginRejectsMissingExecutableBeforeCredentialContainer(t *testing.T) {
	runner := &pupContainerRunner{preflightErr: errors.New("synthetic missing executable")}
	runtime, contextID := newPupContextRuntime(t, runner)
	payload, err := runtime.loginPupInContextContainer(context.Background(), contextID, strings.NewReader(""), io.Discard)
	payload.clear()
	var unavailable hostCLIUnavailableError
	if !errors.As(err, &unavailable) || unavailable.stage != hostCLIStagePupExecutableIdentity {
		t.Fatalf("error=%v stage=%q", err, unavailable.stage)
	}
	for _, call := range runner.calls {
		if slices.Contains(call, componentLabel+"=pup-login") {
			t.Fatalf("credential-bearing login container created after missing executable: %v", call)
		}
	}
}

func TestPupContainerLoginDistinguishesCaptureAndNativeStateContracts(t *testing.T) {
	for _, test := range []struct {
		name   string
		runner *pupContainerRunner
		stage  hostCLIUnavailableStage
	}{
		{name: "status capture", runner: &pupContainerRunner{executable: []byte("synthetic pup"), status: `{}`}, stage: hostCLIStagePupCaptureContract},
		{name: "native state", runner: &pupContainerRunner{executable: []byte("synthetic pup"), invalidNativeState: true}, stage: hostCLIStagePupStateContract},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, contextID := newPupContextRuntime(t, test.runner)
			runtime.pupRelayFactory = func(_ context.Context, input io.WriteCloser) (pupLoginRelay, error) {
				go func() {
					_, _ = io.WriteString(input, "http://127.0.0.1:8000/oauth/callback?code=synthetic-code&state="+strings.Repeat("s", 32)+"\n")
					_ = input.Close()
				}()
				return &fakePupRelay{}, nil
			}
			payload, err := runtime.loginPupInContextContainer(context.Background(), contextID, strings.NewReader(""), io.Discard)
			payload.clear()
			var unavailable hostCLIUnavailableError
			if !errors.As(err, &unavailable) || unavailable.stage != test.stage {
				t.Fatalf("error=%v stage=%q want=%q", err, unavailable.stage, test.stage)
			}
		})
	}
}

func TestPupContainerLoginRejectsUnknownContextBeforeDocker(t *testing.T) {
	runner := &pupContainerRunner{}
	runtime, _ := newPupContextRuntime(t, runner)
	payload, err := runtime.loginPupInContextContainer(context.Background(), strings.Repeat("0", 64), strings.NewReader(""), io.Discard)
	payload.clear()
	var unavailable hostCLIUnavailableError
	if !errors.As(err, &unavailable) || unavailable.stage != hostCLIStagePupContextSelection || len(runner.calls) != 0 {
		t.Fatalf("error=%v stage=%q Docker calls=%v", err, unavailable.stage, runner.calls)
	}
}
