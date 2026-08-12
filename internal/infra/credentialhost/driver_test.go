package credentialhost

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRunner struct {
	mu    sync.Mutex
	calls int
	run   func(context.Context, Command) error
}

func (r *fakeRunner) Run(ctx context.Context, command Command) error {
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	if r.run == nil {
		return nil
	}
	return r.run(ctx, command)
}

func (r *fakeRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func TestLoginUsesExactCommandPrivateHomeAndPacksCache(t *testing.T) {
	executable := testExecutable(t)
	canonical, _, err := resolveExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	var temporaryHome string
	var visible bytes.Buffer
	var streams []OutputStream
	runner := &fakeRunner{}
	runner.run = func(_ context.Context, command Command) error {
		wantArgs := []string{
			"sso", "login", "--profile", "tobari", "--use-device-code", "--no-browser", "--no-cli-pager",
		}
		if command.Path != canonical || !reflect.DeepEqual(command.Args, wantArgs) {
			t.Fatalf("command path/args = %q %q", command.Path, command.Args)
		}
		home := driverEnvironmentValue(t, command.Env, "HOME")
		temporaryHome = home
		if command.Dir != home || !reflect.DeepEqual(command.Env, commandEnvironment(home)) {
			t.Fatalf("command dir/env = %q %#v", command.Dir, command.Env)
		}
		assertDriverPrivatePath(t, home, 0o700)
		assertDriverPrivatePath(t, filepath.Join(home, ".aws"), 0o700)
		assertDriverPrivatePath(t, filepath.Join(home, ".aws", "config"), 0o600)
		assertDriverPrivatePath(t, filepath.Join(home, ".aws", "credentials"), 0o600)
		assertDriverPrivatePath(t, cachePath(home), 0o700)
		configuration, readErr := os.ReadFile(filepath.Join(home, ".aws", "config"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		wantConfiguration, renderErr := renderProfile(testProfile())
		if renderErr != nil {
			t.Fatal(renderErr)
		}
		if !bytes.Equal(configuration, wantConfiguration) {
			t.Fatalf("config = %q, want %q", configuration, wantConfiguration)
		}
		writeCacheFile(t, cachePath(home), testCacheName, testCacheContent("login-cache-secret"))
		if _, writeErr := command.Stdout.Write([]byte("Open the verification URL\n")); writeErr != nil {
			return writeErr
		}
		_, writeErr := command.Stderr.Write([]byte("Device code ready\n"))
		return writeErr
	}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	state, err := driver.Login(context.Background(), executable, testProfile(), func(stream OutputStream, content []byte) error {
		streams = append(streams, stream)
		_, writeErr := visible.Write(content)
		return writeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 1 || !reflect.DeepEqual(streams, []OutputStream{OutputStdout, OutputStderr}) {
		t.Fatalf("calls=%d streams=%v", runner.callCount(), streams)
	}
	if visible.String() != "Open the verification URL\nDevice code ready\n" {
		t.Fatalf("visible output = %q", visible.String())
	}
	if _, err := os.Stat(temporaryHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary HOME was not removed: %v", err)
	}
	encoded := mustEncodeState(t, state)
	decoded, err := DecodeState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.payload.Executable.Path != canonical || len(decoded.payload.Cache) != 1 {
		t.Fatalf("state = %#v", decoded)
	}
}

func TestRefreshUsesExactCommandAndReturnsUpdatedState(t *testing.T) {
	executable := testExecutable(t)
	state := directState(t, executable, testCacheContent("old-cache-secret"))
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	expiration := now.Add(time.Hour)
	accessKey := "ASIA1234567890ABCDEF"
	secretKey := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"
	sessionToken := "sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/="
	var temporaryHome string
	runner := &fakeRunner{}
	runner.run = func(ctx context.Context, command Command) error {
		wantArgs := []string{
			"configure", "export-credentials",
			"--profile", "tobari",
			"--format", "process",
			"--no-cli-pager",
			"--cli-connect-timeout", "10",
			"--cli-read-timeout", "30",
		}
		if command.Path != state.payload.Executable.Path || !reflect.DeepEqual(command.Args, wantArgs) {
			t.Fatalf("command path/args = %q %q", command.Path, command.Args)
		}
		home := driverEnvironmentValue(t, command.Env, "HOME")
		temporaryHome = home
		if command.Dir != home || !reflect.DeepEqual(command.Env, commandEnvironment(home)) {
			t.Fatalf("command dir/env = %q %#v", command.Dir, command.Env)
		}
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("refresh command has no deadline")
		}
		remaining := time.Until(deadline)
		if remaining <= 44*time.Second || remaining > refreshTimeout {
			t.Fatalf("refresh deadline remaining = %s", remaining)
		}
		oldContent, readErr := os.ReadFile(filepath.Join(cachePath(home), testCacheName))
		if readErr != nil || !bytes.Contains(oldContent, []byte("old-cache-secret")) {
			t.Fatalf("materialized cache = %q, error=%v", oldContent, readErr)
		}
		writeCacheFileReplace(t, cachePath(home), testCacheName, testCacheContent("updated-cache-secret"))
		response := fmt.Sprintf(
			`{"Version":1,"AccessKeyId":%q,"SecretAccessKey":%q,"SessionToken":%q,"Expiration":%q}`,
			accessKey, secretKey, sessionToken, expiration.Format(time.RFC3339),
		)
		if _, writeErr := command.Stdout.Write([]byte(response)); writeErr != nil {
			return writeErr
		}
		_, writeErr := command.Stderr.Write([]byte("non-secret diagnostic"))
		return writeErr
	}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.now = func() time.Time { return now }
	credentials, updated, err := driver.Refresh(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.AccessKeyID() != accessKey || credentials.SecretAccessKey() != secretKey ||
		credentials.SessionToken() != sessionToken || !credentials.Expiration().Equal(expiration) {
		t.Fatal("temporary credentials did not round-trip")
	}
	for _, rendered := range []string{fmt.Sprintf("%v", credentials), fmt.Sprintf("%#v", credentials)} {
		if strings.Contains(rendered, secretKey) || strings.Contains(rendered, sessionToken) || !strings.Contains(rendered, "redacted") {
			t.Fatalf("credential formatting leaked: %q", rendered)
		}
	}
	updatedContent := decodeCacheForTest(t, updated.payload.Cache[0])
	if !bytes.Contains(updatedContent, []byte("updated-cache-secret")) || bytes.Contains(updatedContent, []byte("old-cache-secret")) {
		t.Fatalf("updated cache = %q", updatedContent)
	}
	encoded := mustEncodeState(t, updated)
	if _, err := DecodeState(encoded); err != nil {
		t.Fatalf("updated state does not decode: %v", err)
	}
	if _, err := os.Stat(temporaryHome); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary HOME was not removed: %v", err)
	}
}

func TestRefreshRejectsExecutableDigestMismatchBeforeCommand(t *testing.T) {
	executable := testExecutable(t)
	state := directState(t, executable, testCacheContent("cache-secret"))
	if err := os.WriteFile(executable, []byte("changed executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	_, _, err := driver.Refresh(context.Background(), state)
	if !errors.Is(err, ErrInvalidExecutable) {
		t.Fatalf("error = %v", err)
	}
	if runner.callCount() != 0 {
		t.Fatalf("runner calls = %d", runner.callCount())
	}
}

func TestLoginAndRefreshRejectWritableExecutableDriftBeforeCommand(t *testing.T) {
	for _, operation := range []string{"login", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			executable := testExecutable(t)
			loginRunner := &fakeRunner{run: func(_ context.Context, command Command) error {
				writeCacheFile(
					t, cachePath(driverEnvironmentValue(t, command.Env, "HOME")),
					testCacheName, testCacheContent("login-cache-secret"),
				)
				return nil
			}}
			loginDriver := NewDriver(loginRunner)
			loginDriver.tempRoot = t.TempDir()
			state, err := loginDriver.Login(context.Background(), executable, testProfile(), nil)
			if err != nil || loginRunner.callCount() != 1 {
				t.Fatalf("initial login state=%v error=%v calls=%d", state, err, loginRunner.callCount())
			}
			if err := os.Chmod(executable, 0o777); err != nil {
				t.Fatal(err)
			}

			runner := &fakeRunner{}
			driver := NewDriver(runner)
			driver.tempRoot = t.TempDir()
			switch operation {
			case "login":
				_, err = driver.Login(context.Background(), executable, testProfile(), nil)
			case "refresh":
				_, _, err = driver.Refresh(context.Background(), state)
			default:
				t.Fatalf("unknown operation %q", operation)
			}
			if !errors.Is(err, ErrInvalidExecutable) {
				t.Fatalf("error = %v", err)
			}
			if runner.callCount() != 0 {
				t.Fatalf("runner calls = %d", runner.callCount())
			}
		})
	}
}

func TestLoginHonorsContextCancellation(t *testing.T) {
	executable := testExecutable(t)
	started := make(chan struct{})
	runner := &fakeRunner{run: func(ctx context.Context, _ Command) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := driver.Login(ctx, executable, testProfile(), nil)
		result <- err
	}()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login did not honor cancellation")
	}
}

func TestLoginRejectsVisibleOutputOverflow(t *testing.T) {
	executable := testExecutable(t)
	runner := &fakeRunner{run: func(_ context.Context, command Command) error {
		_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxVisibleOutputBytes+1))
		return nil
	}}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	visibleBytes := 0
	state, err := driver.Login(context.Background(), executable, testProfile(), func(_ OutputStream, content []byte) error {
		visibleBytes += len(content)
		return nil
	})
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("error = %v", err)
	}
	if visibleBytes > maxVisibleOutputBytes || state.payload.SchemaVersion != 0 {
		t.Fatalf("visible bytes=%d state=%#v", visibleBytes, state)
	}
}

func TestCommandFailureDoesNotExposeRunnerSecret(t *testing.T) {
	executable := testExecutable(t)
	secret := "runner-secret-must-not-escape"
	runner := &fakeRunner{run: func(_ context.Context, _ Command) error {
		return errors.New("synthetic failure: " + secret)
	}}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	_, err := driver.Login(context.Background(), executable, testProfile(), nil)
	if !errors.Is(err, ErrCommandFailed) || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe error = %v", err)
	}
}

func TestLoginAndRefreshFailClosedWhenPrivateHomeCleanupFails(t *testing.T) {
	executable := testExecutable(t)
	cleanupErr := errors.New("synthetic cleanup failure")
	loginRunner := &fakeRunner{run: func(_ context.Context, command Command) error {
		writeCacheFile(t, cachePath(driverEnvironmentValue(t, command.Env, "HOME")), testCacheName, testCacheContent("login-cache-secret"))
		return nil
	}}
	loginDriver := NewDriver(loginRunner)
	loginDriver.tempRoot = t.TempDir()
	loginDriver.removeAll = func(string) error { return cleanupErr }
	loginState, err := loginDriver.Login(context.Background(), executable, testProfile(), nil)
	if !errors.Is(err, ErrCommandFailed) || loginState.payload.SchemaVersion != 0 {
		t.Fatalf("login state=%#v error=%v", loginState, err)
	}

	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	refreshState := directState(t, executable, testCacheContent("old-cache-secret"))
	refreshRunner := &fakeRunner{run: func(_ context.Context, command Command) error {
		response := fmt.Sprintf(
			`{"Version":1,"AccessKeyId":"ASIA1234567890ABCDEF","SecretAccessKey":"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN","SessionToken":"sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ","Expiration":%q}`,
			now.Add(time.Hour).Format(time.RFC3339),
		)
		_, writeErr := command.Stdout.Write([]byte(response))
		return writeErr
	}}
	refreshDriver := NewDriver(refreshRunner)
	refreshDriver.tempRoot = t.TempDir()
	refreshDriver.now = func() time.Time { return now }
	refreshDriver.removeAll = func(string) error { return cleanupErr }
	credentials, updated, err := refreshDriver.Refresh(context.Background(), refreshState)
	if !errors.Is(err, ErrCommandFailed) || credentials.AccessKeyID() != "" || updated.payload.SchemaVersion != 0 {
		t.Fatalf("credentials=%#v updated=%#v error=%v", credentials, updated, err)
	}
}

func TestPrepareHomeReportsCleanupFailureAfterPartialCacheMaterialization(t *testing.T) {
	driver := NewDriver(&fakeRunner{})
	driver.tempRoot = t.TempDir()
	var cleanupPath string
	driver.removeAll = func(path string) error {
		cleanupPath = path
		return errors.New("synthetic cleanup failure")
	}
	_, err := driver.prepareHome(testProfile(), []stateCacheFile{
		{
			Name:             "0000000000000000000000000000000000000000.json",
			ContentBase64URL: base64.RawURLEncoding.EncodeToString(testCacheContent("materialized-secret")),
		},
		{
			Name:             "1111111111111111111111111111111111111111.json",
			ContentBase64URL: "not-canonical-base64!",
		},
	})
	if !errors.Is(err, ErrCommandFailed) || cleanupPath == "" {
		t.Fatalf("cleanup path=%q error=%v", cleanupPath, err)
	}
	if _, statErr := os.Stat(filepath.Join(cachePath(cleanupPath), "0000000000000000000000000000000000000000.json")); statErr != nil {
		t.Fatalf("partial cache setup was not reached: %v", statErr)
	}
	if removeErr := os.RemoveAll(cleanupPath); removeErr != nil {
		t.Fatal(removeErr)
	}
}

func TestRefreshRejectsStrictProcessJSONAndOutputOverflow(t *testing.T) {
	executable := testExecutable(t)
	state := directState(t, executable, testCacheContent("cache-secret"))
	tests := map[string]struct {
		write func(Command)
		want  error
	}{
		"unknown JSON field": {
			write: func(command Command) {
				_, _ = command.Stdout.Write([]byte(`{"Version":1,"AccessKeyId":"ASIA1234567890ABCDEF","SecretAccessKey":"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN","SessionToken":"sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ","Expiration":"2099-01-01T00:00:00Z","Unknown":true}`))
			},
			want: ErrInvalidCredentials,
		},
		"stdout overflow": {
			write: func(command Command) {
				_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxProcessStdoutBytes+1))
			},
			want: ErrOutputLimit,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runner := &fakeRunner{run: func(_ context.Context, command Command) error {
				test.write(command)
				return nil
			}}
			driver := NewDriver(runner)
			driver.tempRoot = t.TempDir()
			_, _, err := driver.Refresh(context.Background(), state)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func directState(t *testing.T, executable string, content []byte) State {
	t.Helper()
	canonical, digest, err := resolveExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	state, err := newState(testProfile(), canonical, digest, []stateCacheFile{{
		Name:             testCacheName,
		ContentBase64URL: base64.RawURLEncoding.EncodeToString(content),
	}})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func driverEnvironmentValue(t *testing.T, environment []string, key string) string {
	t.Helper()
	prefix := key + "="
	for _, item := range environment {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix)
		}
	}
	t.Fatalf("environment is missing %s: %v", key, environment)
	return ""
}

func assertDriverPrivatePath(t *testing.T, path string, permission os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != permission {
		t.Fatalf("%s permissions = %o, want %o", path, info.Mode().Perm(), permission)
	}
}

func writeCacheFileReplace(t *testing.T, directory, name string, content []byte) {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func decodeCacheForTest(t *testing.T, item stateCacheFile) []byte {
	t.Helper()
	content, err := base64.RawURLEncoding.DecodeString(item.ContentBase64URL)
	if err != nil {
		t.Fatal(err)
	}
	return content
}
