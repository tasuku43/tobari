package credentialhost

import (
	"bytes"
	"context"
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

const githubTokenCanary = "ghp_synthetic_token_canary_1234567890"

type fakeGitHubRunner struct {
	mu    sync.Mutex
	calls int
	run   func(int, context.Context, GitHubCommand) error
}

func (r *fakeGitHubRunner) Run(ctx context.Context, command GitHubCommand) error {
	r.mu.Lock()
	call := r.calls
	r.calls++
	r.mu.Unlock()
	if r.run == nil {
		return nil
	}
	return r.run(call, ctx, command)
}

func (r *fakeGitHubRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type githubExitCodeError int

func (e githubExitCodeError) Error() string { return "synthetic process exit" }
func (e githubExitCodeError) ExitCode() int { return int(e) }

func TestGitHubLoginUsesCanonicalExecutableExactCommandsEnvironmentAndTTY(t *testing.T) {
	target := testGitHubExecutable(t)
	link := filepath.Join(t.TempDir(), "gh-link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatal(err)
	}

	stdin := strings.NewReader("trusted terminal input")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	for name, value := range map[string]string{
		"GH_TOKEN":                       "ambient-gh-token",
		"GITHUB_TOKEN":                   "ambient-github-token",
		"GH_ENTERPRISE_TOKEN":            "ambient-enterprise-token",
		"GITHUB_ENTERPRISE_TOKEN":        "ambient-github-enterprise-token",
		"GH_HOST":                        "enterprise.example",
		"GH_REPO":                        "owner/repository",
		"GH_CONFIG_DIR":                  "/ambient/config",
		"GH_PROMPT_DISABLED":             "0",
		"GH_BROWSER":                     "/ambient/gh-browser",
		"BROWSER":                        "/ambient/browser",
		"NO_COLOR":                       "0",
		"LD_PRELOAD":                     "/ambient/inject.so",
		"DYLD_INSERT_LIBRARIES":          "/ambient/inject.dylib",
		"HTTPS_PROXY":                    "http://proxy.example",
		"ALL_PROXY":                      "http://proxy.example",
		"AWS_ACCESS_KEY_ID":              "ambient-credential",
		"GOOGLE_APPLICATION_CREDENTIALS": "/ambient/credential.json",
	} {
		t.Setenv(name, value)
	}
	var configDirectory string
	runner := &fakeGitHubRunner{}
	runner.run = func(call int, _ context.Context, command GitHubCommand) error {
		if command.Path != canonical {
			t.Fatalf("command path = %q, want canonical %q", command.Path, canonical)
		}
		if call == 0 {
			configDirectory = environmentValue(t, command.Env, "GH_CONFIG_DIR")
			assertPrivatePath(t, configDirectory, 0o700)
		}
		wantEnvironment := []string{
			"HOME=" + configDirectory,
			"GH_CONFIG_DIR=" + configDirectory,
			"GH_PROMPT_DISABLED=1",
			"GH_BROWSER=/bin/true",
			"NO_COLOR=1",
			"LC_ALL=C",
			"PATH=/usr/bin:/bin",
		}
		if command.Dir != configDirectory || !reflect.DeepEqual(command.Env, wantEnvironment) {
			t.Fatalf("command dir/env = %q %#v, want %q %#v", command.Dir, command.Env, configDirectory, wantEnvironment)
		}
		switch call {
		case 0:
			wantArgs := []string{"auth", "login", "--hostname", "github.com", "--web", "--insecure-storage"}
			if !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != stdin || command.Stdout != &stdout || command.Stderr != &stderr {
				t.Fatalf("login command = %#v", command)
			}
			if _, err := command.Stdout.Write([]byte("Open https://github.com/login/device\n")); err != nil {
				return err
			}
			_, err := command.Stderr.Write([]byte("Enter the displayed code\n"))
			return err
		case 1:
			wantArgs := []string{"auth", "status", "--active", "--hostname", "github.com", "--json", "hosts"}
			if !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != nil || command.Stderr != io.Discard {
				t.Fatalf("status command = %#v", command)
			}
			_, err := command.Stdout.Write([]byte(
				`{"hosts":{"github.com":[{"active":false,"login":"inactive","state":"success"},{"active":true,"login":"octo-user","state":"success"}]}}`,
			))
			return err
		case 2:
			wantArgs := []string{"auth", "token", "--hostname", "github.com"}
			if !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != nil || command.Stderr != io.Discard {
				t.Fatalf("token command = %#v", command)
			}
			_, err := command.Stdout.Write([]byte(githubTokenCanary + "\r\n"))
			return err
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}
	driver := NewGitHubDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), link, GitHubLoginStreams{
		Stdin: stdin, Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if runner.callCount() != 3 || credential.AccountLabel() != "octo-user" || string(credential.Token()) != githubTokenCanary {
		t.Fatalf("calls=%d account=%q token match=%t", runner.callCount(), credential.AccountLabel(), string(credential.Token()) == githubTokenCanary)
	}
	if stdout.String() != "Open https://github.com/login/device\n" || stderr.String() != "Enter the displayed code\n" {
		t.Fatalf("visible streams = stdout %q stderr %q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(configDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary GH_CONFIG_DIR was not removed: %v", err)
	}
	returned := credential.Token()
	returned[0] = 'X'
	if string(credential.Token()) != githubTokenCanary {
		t.Fatal("Token did not return an independent copy")
	}
	for _, rendered := range []string{fmt.Sprintf("%v", credential), fmt.Sprintf("%#v", credential)} {
		if strings.Contains(rendered, githubTokenCanary) || !strings.Contains(rendered, "redacted") {
			t.Fatalf("credential formatting leaked: %q", rendered)
		}
	}
	credential.Clear()
	if len(credential.Token()) != 0 || credential.AccountLabel() != "" {
		t.Fatalf("credential was not empty after Clear: %v", credential)
	}
	var nilCredential *GitHubCredential
	nilCredential.Clear()
}

func TestGitHubLoginRejectsExecutableDigestChangeBeforeCapture(t *testing.T) {
	executable := testGitHubExecutable(t)
	var configDirectory string
	runner := &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
		if call != 0 {
			t.Fatalf("unexpected capture command %d", call)
		}
		configDirectory = command.Dir
		return os.WriteFile(executable, []byte("changed synthetic gh executable"), 0o700)
	}}
	driver := NewGitHubDriver(runner)
	driver.tempRoot = t.TempDir()
	credential, err := driver.Login(context.Background(), executable, githubTestStreams())
	if !errors.Is(err, ErrGitHubExecutable) || len(credential.Token()) != 0 || runner.callCount() != 1 {
		t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
	}
	if _, statErr := os.Stat(configDirectory); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary GH_CONFIG_DIR was not removed: %v", statErr)
	}
}

func TestGitHubLoginCancellationStopsBeforeCaptureAndCleansUp(t *testing.T) {
	t.Run("exit 130", func(t *testing.T) {
		executable := testGitHubExecutable(t)
		var configDirectory string
		runner := &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
			if call != 0 {
				t.Fatalf("unexpected capture command %d", call)
			}
			configDirectory = command.Dir
			return githubExitCodeError(130)
		}}
		driver := NewGitHubDriver(runner)
		driver.tempRoot = t.TempDir()
		_, err := driver.Login(context.Background(), executable, githubTestStreams())
		if !errors.Is(err, ErrGitHubLoginCancelled) || runner.callCount() != 1 {
			t.Fatalf("error=%v calls=%d", err, runner.callCount())
		}
		if _, statErr := os.Stat(configDirectory); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("temporary GH_CONFIG_DIR was not removed: %v", statErr)
		}
	})

	t.Run("context", func(t *testing.T) {
		executable := testGitHubExecutable(t)
		started := make(chan struct{})
		var configDirectory string
		runner := &fakeGitHubRunner{run: func(_ int, ctx context.Context, command GitHubCommand) error {
			configDirectory = command.Dir
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}}
		driver := NewGitHubDriver(runner)
		driver.tempRoot = t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := driver.Login(ctx, executable, githubTestStreams())
			result <- err
		}()
		<-started
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, ErrGitHubLoginCancelled) || runner.callCount() != 1 {
				t.Fatalf("error=%v calls=%d", err, runner.callCount())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("GitHub login did not honor context cancellation")
		}
		if _, statErr := os.Stat(configDirectory); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("temporary GH_CONFIG_DIR was not removed: %v", statErr)
		}
	})
}

func TestGitHubLoginCleanupFailureSuppressesCredentialAndErrorDetail(t *testing.T) {
	executable := testGitHubExecutable(t)
	runner := successfulGitHubRunner(t, githubTokenCanary)
	driver := NewGitHubDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.removeAll = func(path string) error {
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
		return errors.New("cleanup-detail-should-not-escape")
	}
	credential, err := driver.Login(context.Background(), executable, githubTestStreams())
	if !errors.Is(err, ErrGitHubLoginCleanup) || len(credential.Token()) != 0 || credential.AccountLabel() != "" {
		t.Fatalf("credential=%v error=%v", credential, err)
	}
	if strings.Contains(err.Error(), "cleanup-detail-should-not-escape") || strings.Contains(err.Error(), githubTokenCanary) {
		t.Fatalf("cleanup error leaked detail: %q", err)
	}
}

func TestParseGitHubAccountRejectsAmbiguousOrMalformedStatus(t *testing.T) {
	valid := `{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`
	if login, err := parseGitHubAccount([]byte(valid)); err != nil || login != "octo-user" {
		t.Fatalf("valid status: login=%q error=%v", login, err)
	}
	tests := map[string]string{
		"empty":                  "",
		"invalid json":           `{`,
		"trailing value":         valid + ` {}`,
		"unknown root field":     `{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]},"other":true}`,
		"duplicate root key":     `{"hosts":{"github.com":[]},"hosts":{"github.com":[]}}`,
		"missing github host":    `{"hosts":{"example.com":[]}}`,
		"empty host entries":     `{"hosts":{"github.com":[]}}`,
		"inactive":               `{"hosts":{"github.com":[{"active":false,"login":"octo-user","state":"success"}]}}`,
		"failed state":           `{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"failure"}]}}`,
		"two active accounts":    `{"hosts":{"github.com":[{"active":true,"login":"one","state":"success"},{"active":true,"login":"two","state":"success"}]}}`,
		"invalid account label":  `{"hosts":{"github.com":[{"active":true,"login":"-octo","state":"success"}]}}`,
		"duplicate nested field": `{"hosts":{"github.com":[{"active":true,"active":true,"login":"octo-user","state":"success"}]}}`,
	}
	for name, encoded := range tests {
		t.Run(name, func(t *testing.T) {
			if login, err := parseGitHubAccount([]byte(encoded)); !errors.Is(err, ErrGitHubAccountCapture) || login != "" {
				t.Fatalf("login=%q error=%v", login, err)
			}
		})
	}
	entries := strings.Repeat(`{"active":false},`, 16) + `{"active":true,"login":"octo-user","state":"success"}`
	if _, err := parseGitHubAccount([]byte(`{"hosts":{"github.com":[` + entries + `]}}`)); !errors.Is(err, ErrGitHubAccountCapture) {
		t.Fatalf("17-entry status error = %v", err)
	}
	invalidUTF8 := append([]byte(`{"hosts":{"ignored":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`)...)
	if _, err := parseGitHubAccount(invalidUTF8); !errors.Is(err, ErrGitHubAccountCapture) {
		t.Fatalf("invalid UTF-8 status error = %v", err)
	}
}

func TestParseGitHubTokenEnforcesOpaqueTokenBoundsAndFraming(t *testing.T) {
	exactLimit := bytes.Repeat([]byte("x"), maxGitHubTokenBytes)
	for name, value := range map[string][]byte{
		"lf framing":   []byte(githubTokenCanary + "\n"),
		"crlf framing": []byte(githubTokenCanary + "\r\n"),
		"no framing":   []byte(githubTokenCanary),
		"exact limit":  exactLimit,
	} {
		t.Run(name, func(t *testing.T) {
			token, err := parseGitHubToken(value)
			if err != nil || len(token) == 0 || bytes.ContainsAny(token, "\r\n") {
				t.Fatalf("token length=%d error=%v", len(token), err)
			}
		})
	}
	for name, value := range map[string][]byte{
		"empty":            {},
		"only framing":     []byte("\n"),
		"embedded newline": []byte("token\nvalue\n"),
		"embedded return":  []byte("token\rvalue\n"),
		"double framing":   []byte("token\n\n"),
		"oversize":         bytes.Repeat([]byte("x"), maxGitHubTokenBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if token, err := parseGitHubToken(value); !errors.Is(err, ErrGitHubTokenCapture) || token != nil {
				t.Fatalf("token length=%d error=%v", len(token), err)
			}
		})
	}
}

func TestGitHubLoginRejectsBoundedCaptureOverflow(t *testing.T) {
	tests := map[string]struct {
		overflowCall int
		wantCalls    int
	}{
		"status": {overflowCall: 1, wantCalls: 2},
		"token":  {overflowCall: 2, wantCalls: 3},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			executable := testGitHubExecutable(t)
			runner := &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
				switch call {
				case 0:
					return nil
				case 1:
					if test.overflowCall == call {
						_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxGitHubStatusBytes+1))
						return nil
					}
					_, err := command.Stdout.Write([]byte(`{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`))
					return err
				case 2:
					_, _ = command.Stdout.Write(bytes.Repeat([]byte("x"), maxGitHubTokenBytes+3))
					return nil
				default:
					t.Fatalf("unexpected call %d", call)
					return nil
				}
			}}
			driver := NewGitHubDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), executable, githubTestStreams())
			if !errors.Is(err, ErrGitHubOutputLimit) || len(credential.Token()) != 0 || runner.callCount() != test.wantCalls {
				t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
			}
		})
	}
}

func TestGitHubLoginInvalidCaptureNeverReturnsCredential(t *testing.T) {
	tests := map[string]struct {
		invalidCall int
		want        error
		wantCalls   int
	}{
		"status": {invalidCall: 1, want: ErrGitHubAccountCapture, wantCalls: 2},
		"token":  {invalidCall: 2, want: ErrGitHubTokenCapture, wantCalls: 3},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			executable := testGitHubExecutable(t)
			var configDirectory string
			runner := &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
				configDirectory = command.Dir
				switch call {
				case 0:
					return nil
				case 1:
					status := `{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`
					if test.invalidCall == call {
						status = `{"hosts":{"github.com":[{"active":true,"login":"one","state":"success"},{"active":true,"login":"two","state":"success"}]}}`
					}
					_, err := command.Stdout.Write([]byte(status))
					return err
				case 2:
					_, err := command.Stdout.Write([]byte(githubTokenCanary + "\nsecond-line\n"))
					return err
				default:
					t.Fatalf("unexpected call %d", call)
					return nil
				}
			}}
			driver := NewGitHubDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), executable, githubTestStreams())
			if !errors.Is(err, test.want) || len(credential.Token()) != 0 || credential.AccountLabel() != "" || runner.callCount() != test.wantCalls {
				t.Fatalf("credential=%v error=%v calls=%d", credential, err, runner.callCount())
			}
			if _, statErr := os.Stat(configDirectory); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("temporary GH_CONFIG_DIR was not removed: %v", statErr)
			}
		})
	}
}

func TestGitHubLoginNeverReturnsRunnerErrorDetails(t *testing.T) {
	for name, failedCall := range map[string]int{"login": 0, "status": 1, "token": 2} {
		t.Run(name, func(t *testing.T) {
			executable := testGitHubExecutable(t)
			runner := &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
				if call == failedCall {
					return errors.New(githubTokenCanary + " runner detail")
				}
				switch call {
				case 1:
					_, err := command.Stdout.Write([]byte(`{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`))
					return err
				case 2:
					_, err := command.Stdout.Write([]byte(githubTokenCanary + "\n"))
					return err
				default:
					return nil
				}
			}}
			driver := NewGitHubDriver(runner)
			driver.tempRoot = t.TempDir()
			credential, err := driver.Login(context.Background(), executable, githubTestStreams())
			if err == nil || strings.Contains(err.Error(), githubTokenCanary) || len(credential.Token()) != 0 {
				t.Fatalf("credential=%v error=%v", credential, err)
			}
		})
	}
}

func TestGitHubLoginRequiresStreamsBeforeExecuting(t *testing.T) {
	runner := &fakeGitHubRunner{}
	driver := NewGitHubDriver(runner)
	_, err := driver.Login(context.Background(), testGitHubExecutable(t), GitHubLoginStreams{})
	if !errors.Is(err, ErrGitHubTTYRequired) || runner.callCount() != 0 {
		t.Fatalf("error=%v calls=%d", err, runner.callCount())
	}
}

func successfulGitHubRunner(t *testing.T, token string) *fakeGitHubRunner {
	t.Helper()
	return &fakeGitHubRunner{run: func(call int, _ context.Context, command GitHubCommand) error {
		switch call {
		case 0:
			return nil
		case 1:
			_, err := command.Stdout.Write([]byte(`{"hosts":{"github.com":[{"active":true,"login":"octo-user","state":"success"}]}}`))
			return err
		case 2:
			_, err := command.Stdout.Write([]byte(token + "\n"))
			return err
		default:
			t.Fatalf("unexpected command call %d", call)
			return nil
		}
	}}
}

func githubTestStreams() GitHubLoginStreams {
	return GitHubLoginStreams{
		Stdin:  strings.NewReader("trusted terminal input"),
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
}

func testGitHubExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("synthetic gh executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
