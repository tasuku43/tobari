package credentialhost

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	codexAccountCanary = "account-synthetic-123"
	codexAccessCanary  = "access.synthetic.canary.0123456789"
	codexRefreshCanary = "refresh.synthetic.canary.0123456789"
	codexRefreshTime   = "2026-08-10T01:02:03.456789Z"
	codexAccessField   = "access_" + "token"
	codexRefreshField  = "refresh_" + "token"
)

type fakeCodexRunner struct {
	mu    sync.Mutex
	calls int
	run   func(int, context.Context, Command) error
}

func (r *fakeCodexRunner) Run(ctx context.Context, command Command) error {
	r.mu.Lock()
	call := r.calls
	r.calls++
	r.mu.Unlock()
	if r.run == nil {
		return nil
	}
	return r.run(call, ctx, command)
}

func (r *fakeCodexRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestCodexLoginUsesExactVersionDeviceFlowEnvironmentAndCanonicalState(t *testing.T) {
	target := testCodexExecutable(t)
	link := filepath.Join(t.TempDir(), "codex-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	canonical, digest, err := resolveExecutable(link)
	if err != nil {
		t.Fatal(err)
	}

	stdin := strings.NewReader("trusted terminal input")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	for name, value := range map[string]string{
		"OPENAI_API_KEY":        "ambient-openai-key",
		"CODEX_ACCESS_TOKEN":    "ambient-codex-token",
		"CODEX_HOME":            "/ambient/codex",
		"BROWSER":               "/ambient/browser",
		"HTTPS_PROXY":           "http://proxy.example",
		"HTTP_PROXY":            "http://proxy.example",
		"ALL_PROXY":             "http://proxy.example",
		"NO_PROXY":              "proxy.example",
		"LD_PRELOAD":            "/ambient/inject.so",
		"DYLD_INSERT_LIBRARIES": "/ambient/inject.dylib",
	} {
		t.Setenv(name, value)
	}
	var home string
	var codexHome string
	runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
		if command.Path != canonical {
			t.Fatalf("command path = %q, want %q", command.Path, canonical)
		}
		if call == 0 {
			home = environmentValue(t, command.Env, "HOME")
			codexHome = environmentValue(t, command.Env, "CODEX_HOME")
			assertPrivatePath(t, home, 0o700)
			assertPrivatePath(t, codexHome, 0o700)
			if filepath.Dir(codexHome) != home {
				t.Fatalf("CODEX_HOME %q is not inside HOME %q", codexHome, home)
			}
		}
		wantEnvironment := []string{
			"HOME=" + home,
			"CODEX_HOME=" + codexHome,
			"DISABLE_AUTOUPDATER=1",
			"NO_COLOR=1",
			"LC_ALL=C",
			"PATH=/usr/bin:/bin",
		}
		if command.Dir != home || !reflect.DeepEqual(command.Env, wantEnvironment) {
			t.Fatalf("command dir/env = %q %#v, want %q %#v", command.Dir, command.Env, home, wantEnvironment)
		}
		switch call {
		case 0:
			if !reflect.DeepEqual(command.Args, []string{"--version"}) || command.Stdin != nil || command.Stdout == nil || command.Stderr == nil {
				t.Fatalf("version command = %#v", command)
			}
			_, err := command.Stdout.Write([]byte(codexVersionOutput + "\r\n"))
			return err
		case 1:
			wantArgs := []string{
				"login", "--device-auth",
				"-c", `cli_auth_credentials_store="file"`,
				"-c", "check_for_update_on_startup=false",
			}
			if !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != stdin {
				t.Fatalf("login command = %#v", command)
			}
			if _, err := command.Stdout.Write([]byte("Open the displayed device page\n")); err != nil {
				return err
			}
			if _, err := command.Stderr.Write([]byte("Enter the displayed code\n")); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(codexHome, codexAuthFileName), codexAuthFixture(t, codexAccountCanary, false), 0o600)
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}}
	driver := NewCodexDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), link, CodexLoginStreams{
		Stdin: stdin, Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 2 || credential.DriverID() != CodexDriverID ||
		credential.DriverRevision() != digest || credential.AccountLabel() != codexAccountCanary {
		t.Fatalf("calls=%d driver=%q revision=%q account=%q", runner.callCount(), credential.DriverID(), credential.DriverRevision(), credential.AccountLabel())
	}
	if stdout.String() != "Open the displayed device page\n" || stderr.String() != "Enter the displayed code\n" {
		t.Fatalf("visible output = stdout %q stderr %q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary Codex HOME was not removed: %v", err)
	}

	idToken := codexIDToken(t, codexAccountCanary, false)
	encoded, err := credential.Encode()
	if err != nil {
		t.Fatal(err)
	}
	wantAuth := expectedCodexAuthJSON(
		idToken, codexAccessCanary, codexRefreshCanary, codexAccountCanary,
	)
	wantEncoded := fmt.Sprintf(
		`{"schema_version":1,"codex_executable":{"path":%q,"sha256":%q,"version":"0.146.0"},"auth":%s}`,
		canonical, digest, wantAuth,
	)
	if string(encoded) != wantEncoded {
		t.Fatalf("encoded state differs\n got: %s\nwant: %s", encoded, wantEncoded)
	}
	authJSON, err := credential.AuthJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(authJSON) != wantAuth {
		t.Fatalf("auth.json differs\n got: %s\nwant: %s", authJSON, wantAuth)
	}
	decoded, err := DecodeCodexCredential(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reencoded, err := decoded.Encode()
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatalf("canonical round trip error=%v equal=%t", err, bytes.Equal(encoded, reencoded))
	}
	for _, rendered := range []string{fmt.Sprintf("%v", credential), fmt.Sprintf("%#v", credential)} {
		if !strings.Contains(rendered, "redacted") || strings.Contains(rendered, codexAccessCanary) || strings.Contains(rendered, codexRefreshCanary) {
			t.Fatalf("credential formatting leaked: %q", rendered)
		}
	}
	credential.Clear()
	if credential.AccountLabel() != "" || credential.DriverRevision() != "" {
		t.Fatalf("cleared credential retained metadata: %v", credential)
	}
	if _, err := credential.Encode(); !errors.Is(err, ErrInvalidCodexCredential) {
		t.Fatalf("cleared Encode error = %v", err)
	}
	var nilCredential *CodexCredential
	nilCredential.Clear()
}

func TestCodexLoginRejectsUnsupportedOrAmbiguousVersionBeforeLogin(t *testing.T) {
	tests := map[string]struct {
		stdout string
		stderr string
	}{
		"older":        {stdout: "codex-cli 0.145.0\n"},
		"newer":        {stdout: "codex-cli 0.147.0\n"},
		"leading text": {stdout: "version codex-cli 0.146.0\n"},
		"two lines":    {stdout: "codex-cli 0.146.0\nextra\n"},
		"stderr":       {stdout: "codex-cli 0.146.0\n", stderr: "warning\n"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
				if call != 0 {
					t.Fatalf("unexpected login call %d", call)
				}
				_, _ = command.Stdout.Write([]byte(test.stdout))
				_, _ = command.Stderr.Write([]byte(test.stderr))
				return nil
			}}
			driver := NewCodexDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
			if !errors.Is(err, ErrCodexVersion) || credential.AccountLabel() != "" || runner.callCount() != 1 {
				t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
			}
		})
	}
}

func TestCodexLoginRequiresAbsoluteExecutableAndCompleteStreamsBeforeRunning(t *testing.T) {
	runner := &fakeCodexRunner{}
	driver := NewCodexDriver(runner)
	if _, err := driver.Login(context.Background(), "codex", codexTestStreams()); !errors.Is(err, ErrCodexExecutable) {
		t.Fatalf("relative executable error = %v", err)
	}
	if _, err := driver.Login(context.Background(), testCodexExecutable(t), CodexLoginStreams{}); !errors.Is(err, ErrCodexLoginStreams) {
		t.Fatalf("missing streams error = %v", err)
	}
	if runner.callCount() != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.callCount())
	}
}

func TestCodexLoginRechecksExecutableDigestBeforeAndAfterAcquisition(t *testing.T) {
	for name, mutateCall := range map[string]int{"after version": 0, "after login": 1} {
		t.Run(name, func(t *testing.T) {
			executable := testCodexExecutable(t)
			runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
				switch call {
				case 0:
					_, _ = command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				case 1:
					codexHome := environmentValue(t, command.Env, "CODEX_HOME")
					if err := os.WriteFile(filepath.Join(codexHome, codexAuthFileName), codexAuthFixture(t, codexAccountCanary, false), 0o600); err != nil {
						return err
					}
				default:
					t.Fatalf("unexpected call %d", call)
				}
				if call == mutateCall {
					return os.WriteFile(executable, []byte("changed synthetic Codex executable"), 0o700)
				}
				return nil
			}}
			driver := NewCodexDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), executable, codexTestStreams())
			wantCalls := 1
			if mutateCall == 1 {
				wantCalls = 2
			}
			if !errors.Is(err, ErrCodexExecutable) || credential.AccountLabel() != "" || runner.callCount() != wantCalls {
				t.Fatalf("credential=%v error=%v calls=%d want=%d", credential, err, runner.callCount(), wantCalls)
			}
		})
	}
}

func TestParseCodexAuthRejectsNonManagedOrAmbiguousState(t *testing.T) {
	idToken := codexIDToken(t, codexAccountCanary, false)
	valid := expectedCodexAuthJSON(
		idToken, codexAccessCanary, codexRefreshCanary, codexAccountCanary,
	)
	if auth, label, err := parseCodexAuth([]byte(valid)); err != nil || label != codexAccountCanary || auth.Tokens.AccountID != codexAccountCanary {
		t.Fatalf("valid auth label=%q error=%v", label, err)
	}
	noAccountToken := codexIDToken(t, "", false)
	fedrampToken := codexIDToken(t, codexAccountCanary, true)
	accessKey := `"` + codexAccessField + `"`
	refreshKey := `"` + codexRefreshField + `"`
	tests := map[string]string{
		"empty":              "",
		"invalid json":       `{`,
		"trailing json":      valid + `{}`,
		"duplicate root":     strings.Replace(valid, `"auth_mode":"chatgpt"`, `"auth_mode":"chatgpt","auth_mode":"chatgpt"`, 1),
		"unknown root":       strings.TrimSuffix(valid, "}") + `,"agent_identity":"private"}`,
		"personal token":     strings.TrimSuffix(valid, "}") + `,"personal_access_token":"private"}`,
		"bedrock":            strings.TrimSuffix(valid, "}") + `,"bedrock_api_key":{"token":"private"}}`,
		"wrong mode":         strings.Replace(valid, `"chatgpt"`, `"chatgptAuthTokens"`, 1),
		"api key":            strings.Replace(valid, `"OPENAI_API_KEY":null`, `"OPENAI_API_KEY":"sk-private"`, 1),
		"missing api key":    strings.Replace(valid, `,"OPENAI_API_KEY":null`, "", 1),
		"tokens null":        fmt.Sprintf(`{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":null,"last_refresh":%q}`, codexRefreshTime),
		"unknown token":      strings.Replace(valid, `"account_id":`, `"unknown":true,"account_id":`, 1),
		"duplicate token":    strings.Replace(valid, accessKey+`:`, accessKey+`:"duplicate",`+accessKey+`:`, 1),
		"null id token":      strings.Replace(valid, fmt.Sprintf(`"id_token":%q`, idToken), `"id_token":null`, 1),
		"empty access":       strings.Replace(valid, fmt.Sprintf(`%s:%q`, accessKey, codexAccessCanary), accessKey+`:""`, 1),
		"control refresh":    strings.Replace(valid, fmt.Sprintf(`%s:%q`, refreshKey, codexRefreshCanary), refreshKey+`:"refresh\nprivate"`, 1),
		"account null":       strings.Replace(valid, fmt.Sprintf(`"account_id":%q`, codexAccountCanary), `"account_id":null`, 1),
		"account mismatch":   strings.Replace(valid, fmt.Sprintf(`"account_id":%q`, codexAccountCanary), `"account_id":"account-other"`, 1),
		"claim absent":       strings.Replace(valid, fmt.Sprintf(`"id_token":%q`, idToken), fmt.Sprintf(`"id_token":%q`, noAccountToken), 1),
		"fedramp":            strings.Replace(valid, fmt.Sprintf(`"id_token":%q`, idToken), fmt.Sprintf(`"id_token":%q`, fedrampToken), 1),
		"invalid timestamp":  strings.Replace(valid, codexRefreshTime, "2026-08-10", 1),
		"offset timestamp":   strings.Replace(valid, codexRefreshTime, "2026-08-10T10:02:03.456789+09:00", 1),
		"invalid account id": strings.Replace(valid, codexAccountCanary, "account/id", 1),
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			if auth, label, err := parseCodexAuth([]byte(encoded)); !errors.Is(err, ErrCodexAuthCapture) || label != "" || auth.AuthMode != "" {
				t.Fatalf("auth=%v label=%q error=%v", auth, label, err)
			}
		})
	}
	oversize := strings.Replace(valid, codexAccessCanary, strings.Repeat("a", maxCodexTokenBytes+1), 1)
	if _, _, err := parseCodexAuth([]byte(oversize)); !errors.Is(err, ErrCodexAuthCapture) {
		t.Fatalf("oversize token error = %v", err)
	}
}

func TestCodexLoginRejectsUnsafeAuthFile(t *testing.T) {
	tests := map[string]func(*testing.T, string) error{
		"group readable": func(t *testing.T, codexHome string) error {
			return os.WriteFile(filepath.Join(codexHome, codexAuthFileName), codexAuthFixture(t, codexAccountCanary, false), 0o640)
		},
		"symlink": func(t *testing.T, codexHome string) error {
			outside := filepath.Join(t.TempDir(), "outside-auth.json")
			if err := os.WriteFile(outside, codexAuthFixture(t, codexAccountCanary, false), 0o600); err != nil {
				return err
			}
			return os.Symlink(outside, filepath.Join(codexHome, codexAuthFileName))
		},
		"directory": func(_ *testing.T, codexHome string) error {
			return os.Mkdir(filepath.Join(codexHome, codexAuthFileName), 0o700)
		},
	}
	for name, writeAuth := range tests {
		t.Run(name, func(t *testing.T) {
			runner := codexRunnerWithAuthWriter(t, writeAuth)
			driver := NewCodexDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
			if !errors.Is(err, ErrCodexAuthCapture) || credential.AccountLabel() != "" || runner.callCount() != 2 {
				t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
			}
		})
	}
}

func TestCodexLoginBoundsVisibleOutputAndDelivery(t *testing.T) {
	t.Run("limit", func(t *testing.T) {
		runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				return err
			}
			_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxCodexVisibleOutputBytes+1))
			return nil
		}}
		driver := NewCodexDriver(runner)
		driver.tempRoot = t.TempDir()
		credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
		if !errors.Is(err, ErrCodexOutputLimit) || credential.AccountLabel() != "" {
			t.Fatalf("credential=%v error=%v", credential, err)
		}
	})

	t.Run("writer", func(t *testing.T) {
		runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				return err
			}
			_, _ = command.Stdout.Write([]byte("visible"))
			return nil
		}}
		driver := NewCodexDriver(runner)
		driver.tempRoot = t.TempDir()
		streams := codexTestStreams()
		streams.Stdout = failingCodexWriter{}
		credential, err := driver.Login(context.Background(), testCodexExecutable(t), streams)
		if !errors.Is(err, ErrCodexVisibleOutput) || credential.AccountLabel() != "" {
			t.Fatalf("credential=%v error=%v", credential, err)
		}
	})
}

func TestCodexLoginTimeoutCancellationCleanupAndEnterpriseDisableAreFailClosed(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		var home string
		runner := &fakeCodexRunner{run: func(call int, ctx context.Context, command Command) error {
			home = command.Dir
			if call == 0 {
				_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}}
		driver := NewCodexDriver(runner)
		driver.tempRoot = t.TempDir()
		driver.loginTimeout = 5 * time.Millisecond
		credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
		if !errors.Is(err, ErrCodexLoginTimeout) || credential.AccountLabel() != "" {
			t.Fatalf("credential=%v error=%v", credential, err)
		}
		if _, statErr := os.Stat(home); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("temporary home remains after timeout: %v", statErr)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		started := make(chan struct{})
		executable := testCodexExecutable(t)
		runner := &fakeCodexRunner{run: func(call int, ctx context.Context, command Command) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				return err
			}
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}
		driver := NewCodexDriver(runner)
		driver.tempRoot = t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := driver.Login(ctx, executable, codexTestStreams())
			result <- err
		}()
		<-started
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, ErrCodexLoginCancelled) {
				t.Fatalf("cancel error = %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Codex login did not honor cancellation")
		}
	})

	// Some ChatGPT Enterprise administrators disable device-code login. That
	// host policy is a normal provider failure and must never commit partial
	// state or trigger a browser-flow fallback.
	t.Run("enterprise device flow disabled", func(t *testing.T) {
		runner := &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
			if call == 0 {
				_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
				return err
			}
			return errors.New(codexRefreshCanary + " provider policy detail")
		}}
		driver := NewCodexDriver(runner)
		driver.tempRoot = t.TempDir()
		credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
		if !errors.Is(err, ErrCodexLoginFailed) || strings.Contains(err.Error(), codexRefreshCanary) || credential.AccountLabel() != "" {
			t.Fatalf("credential=%v error=%v", credential, err)
		}
	})
}

func TestCodexLoginCleanupFailureSuppressesCredential(t *testing.T) {
	runner := codexRunnerWithAuthWriter(t, func(t *testing.T, codexHome string) error {
		return os.WriteFile(filepath.Join(codexHome, codexAuthFileName), codexAuthFixture(t, codexAccountCanary, false), 0o600)
	})
	driver := NewCodexDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.removeAll = func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		return errors.New(codexRefreshCanary + " cleanup detail")
	}
	credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
	if !errors.Is(err, ErrCodexLoginCleanup) || strings.Contains(err.Error(), codexRefreshCanary) || credential.AccountLabel() != "" {
		t.Fatalf("credential=%v error=%v", credential, err)
	}
}

func TestDecodeCodexCredentialRejectsNonCanonicalOrChangedEnvelope(t *testing.T) {
	runner := codexRunnerWithAuthWriter(t, func(t *testing.T, codexHome string) error {
		return os.WriteFile(filepath.Join(codexHome, codexAuthFileName), codexAuthFixture(t, codexAccountCanary, false), 0o600)
	})
	driver := NewCodexDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), testCodexExecutable(t), codexTestStreams())
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := credential.Encode()
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string][]byte{
		"pretty":        append([]byte("\n"), encoded...),
		"unknown":       []byte(strings.TrimSuffix(string(encoded), "}") + `,"unknown":true}`),
		"wrong version": []byte(strings.Replace(string(encoded), `"version":"0.146.0"`, `"version":"0.147.0"`, 1)),
		"wrong schema":  []byte(strings.Replace(string(encoded), `"schema_version":1`, `"schema_version":2`, 1)),
		"missing null":  []byte(strings.Replace(string(encoded), `,"OPENAI_API_KEY":null`, "", 1)),
		"trailing":      append(append([]byte(nil), encoded...), []byte(`{}`)...),
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			decoded, err := DecodeCodexCredential(candidate)
			if !errors.Is(err, ErrInvalidCodexCredential) || decoded.AccountLabel() != "" {
				t.Fatalf("decoded=%v error=%v", decoded, err)
			}
		})
	}
}

type failingCodexWriter struct{}

func (failingCodexWriter) Write([]byte) (int, error) {
	return 0, errors.New("synthetic visible writer failure")
}

func codexRunnerWithAuthWriter(t *testing.T, writeAuth func(*testing.T, string) error) *fakeCodexRunner {
	t.Helper()
	return &fakeCodexRunner{run: func(call int, _ context.Context, command Command) error {
		switch call {
		case 0:
			_, err := command.Stdout.Write([]byte(codexVersionOutput + "\n"))
			return err
		case 1:
			return writeAuth(t, environmentValue(t, command.Env, "CODEX_HOME"))
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}}
}

func codexTestStreams() CodexLoginStreams {
	return CodexLoginStreams{Stdin: strings.NewReader("trusted terminal input"), Stdout: io.Discard, Stderr: io.Discard}
}

func testCodexExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("synthetic Codex 0.146.0 executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func codexAuthFixture(t *testing.T, accountID string, fedramp bool) []byte {
	t.Helper()
	var account any = accountID
	if accountID == "" {
		account = nil
	}
	encoded, err := json.Marshal(map[string]any{
		"auth_mode":      codexAuthMode,
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token":        codexIDToken(t, accountID, fedramp),
			codexAccessField:  codexAccessCanary,
			codexRefreshField: codexRefreshCanary,
			"account_id":      account,
		},
		"last_refresh": codexRefreshTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func expectedCodexAuthJSON(idValue, accessValue, refreshValue, accountValue string) string {
	format := `{"auth_mode":"chatgpt","OPENAI_API_KEY":null,"tokens":{"id_token":%q,"` +
		codexAccessField + `":%q,"` + codexRefreshField + `":%q,"account_id":%q},"last_refresh":%q}`
	return fmt.Sprintf(
		format, idValue, accessValue, refreshValue, accountValue, codexRefreshTime,
	)
}

func codexIDToken(t *testing.T, accountID string, fedramp bool) string {
	t.Helper()
	claims := map[string]any{"chatgpt_account_is_fedramp": fedramp}
	if accountID != "" {
		claims["chatgpt_account_id"] = accountID
	}
	payload, err := json.Marshal(map[string]any{
		"sub":                         "user-synthetic",
		"https://api.openai.com/auth": claims,
	})
	if err != nil {
		t.Fatal(err)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	body := base64.RawURLEncoding.EncodeToString(payload)
	signature := base64.RawURLEncoding.EncodeToString([]byte("synthetic-signature"))
	return header + "." + body + "." + signature
}
