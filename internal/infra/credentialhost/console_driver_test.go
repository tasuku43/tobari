package credentialhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const testConsoleCacheName = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.json"

func testConsoleCacheContent(token string) []byte {
	payload := struct {
		AccessToken struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
		} `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}{RefreshToken: "dummy-refresh-token"}
	payload.AccessToken.AccessKeyID = "ASIAEXAMPLE"
	payload.AccessToken.SecretAccessKey = token
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestConsoleLoginAndRefreshUseExactPrivateState(t *testing.T) {
	executable := testExecutable(t)
	canonical, _, err := resolveExecutable(executable)
	if err != nil {
		t.Fatal(err)
	}
	profile := ConsoleProfileConfig{Region: "us-east-1"}
	loginSession := "arn:aws:sts::123456789012:assumed-role/Developer/session"
	authorizationInput := strings.NewReader("authorization-code\n")
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC)
	expiration := now.Add(time.Hour)
	accessKey := "ASIA1234567890ABCDEF"
	secretKey := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN"
	sessionToken := "sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/="
	call := 0
	var loginHome, refreshHome string
	runner := &fakeRunner{run: func(_ context.Context, command Command) error {
		call++
		home := environmentValue(t, command.Env, "HOME")
		if environmentValue(t, command.Env, "AWS_LOGIN_CACHE_DIRECTORY") != consoleCachePath(home) {
			t.Fatalf("console cache environment = %#v", command.Env)
		}
		switch call {
		case 1:
			if command.Path != canonical || !reflect.DeepEqual(command.Args, []string{"--version"}) || command.Stdin != nil {
				t.Fatalf("version command = %#v", command)
			}
			_, err := command.Stdout.Write([]byte("aws-cli/2.36.11 Python/3.14\n"))
			return err
		case 2:
			loginHome = home
			wantArgs := []string{
				"login", "--remote", "--profile", "tobari", "--region", "us-east-1",
				"--no-cli-pager", "--no-cli-auto-prompt",
			}
			if command.Path != canonical || !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != authorizationInput {
				t.Fatalf("login command path/args/stdin = %q %#v %T", command.Path, command.Args, command.Stdin)
			}
			configuration := "[profile tobari]\nregion = us-east-1\noutput = json\nlogin_session = " + loginSession + "\n"
			if err := os.WriteFile(filepath.Join(home, ".aws", "config"), []byte(configuration), 0o600); err != nil {
				return err
			}
			writeCacheFile(t, consoleCachePath(home), testConsoleCacheName, testConsoleCacheContent("login-secret"))
			_, err := command.Stderr.Write([]byte("Please visit https://signin.aws.amazon.com/example\n"))
			return err
		case 3:
			refreshHome = home
			wantArgs := []string{
				"configure", "export-credentials", "--profile", "tobari", "--format", "process",
				"--no-cli-pager", "--cli-connect-timeout", "10", "--cli-read-timeout", "30",
			}
			if command.Path != canonical || !reflect.DeepEqual(command.Args, wantArgs) || command.Stdin != nil {
				t.Fatalf("refresh command = %#v", command)
			}
			cached, readErr := os.ReadFile(filepath.Join(consoleCachePath(home), testConsoleCacheName))
			if readErr != nil || !bytes.Contains(cached, []byte("login-secret")) {
				t.Fatalf("materialized console cache = %q, error=%v", cached, readErr)
			}
			writeCacheFileReplace(t, consoleCachePath(home), testConsoleCacheName, testConsoleCacheContent("refreshed-secret"))
			response := fmt.Sprintf(
				`{"Version":1,"AccessKeyId":%q,"SecretAccessKey":%q,"SessionToken":%q,"Expiration":%q}`,
				accessKey, secretKey, sessionToken, expiration.Format(time.RFC3339),
			)
			_, err := command.Stdout.Write([]byte(response))
			return err
		default:
			t.Fatalf("unexpected command %d: %#v", call, command)
			return nil
		}
	}}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	driver.now = func() time.Time { return now }
	var visible bytes.Buffer
	state, err := driver.ConsoleLogin(
		context.Background(), executable, profile, authorizationInput,
		func(_ OutputStream, content []byte) error { _, writeErr := visible.Write(content); return writeErr },
	)
	if err != nil {
		t.Fatal(err)
	}
	if call != 2 || state.DriverID() != ConsoleDriverID || state.AccountID() != "123456789012" ||
		state.DriverRevision() == "" || !strings.Contains(visible.String(), "signin.aws.amazon.com") {
		t.Fatalf("console login state/calls/output = %v/%q/%q/%d/%q", state, state.DriverID(), state.AccountID(), call, visible.String())
	}
	encoded, err := state.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeState(encoded)
	if err != nil || decoded.DriverID() != ConsoleDriverID || decoded.console.Profile.LoginSession != loginSession {
		t.Fatalf("decoded console state = %v, error=%v", decoded, err)
	}
	credentials, updated, err := driver.Refresh(context.Background(), decoded)
	if err != nil {
		t.Fatal(err)
	}
	defer credentials.Clear()
	defer updated.Clear()
	if call != 3 || credentials.AccessKeyID() != accessKey || updated.DriverID() != ConsoleDriverID ||
		!bytes.Contains(decodeCacheForTest(t, updated.console.Cache[0]), []byte("refreshed-secret")) {
		t.Fatalf("console refresh calls/credentials/state = %d/%v/%v", call, credentials, updated)
	}
	for _, home := range []string{loginHome, refreshHome} {
		if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("temporary console HOME remains: %q %v", home, err)
		}
	}
}

func TestConsoleLoginRejectsOldAWSCLIBeforeProviderCall(t *testing.T) {
	executable := testExecutable(t)
	runner := &fakeRunner{run: func(_ context.Context, command Command) error {
		if !reflect.DeepEqual(command.Args, []string{"--version"}) {
			t.Fatalf("unexpected provider command = %#v", command.Args)
		}
		_, err := command.Stdout.Write([]byte("aws-cli/2.31.9 Python/3.13\n"))
		return err
	}}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	_, err := driver.ConsoleLogin(
		context.Background(), executable, ConsoleProfileConfig{Region: "us-east-1"},
		strings.NewReader("must-not-be-read"), nil,
	)
	if !errors.Is(err, ErrConsoleLoginUnsupported) || runner.callCount() != 1 {
		t.Fatalf("old CLI error/calls = %v/%d", err, runner.callCount())
	}
}

func TestConsoleStateRejectsMismatchedAccountAndHostileConfig(t *testing.T) {
	profile := ConsoleProfileConfig{Region: "us-east-1"}
	for _, configuration := range []string{
		"[profile tobari]\nregion = us-east-1\noutput = json\n",
		"[profile tobari]\nregion = us-west-2\noutput = json\nlogin_session = arn:aws:iam::123456789012:user/Admin\n",
		"[profile tobari]\nregion = us-east-1\noutput = json\nlogin_session = arn:aws-cn:iam::123456789012:user/Admin\n",
		"[profile tobari]\nregion = us-east-1\noutput = json\nlogin_session = arn:aws:iam::123456789012:user/Admin\ncredential_process = evil\n",
	} {
		if _, _, err := parseConsoleProfile([]byte(configuration), profile); !errors.Is(err, ErrInvalidState) {
			t.Fatalf("hostile config error = %v for %q", err, configuration)
		}
	}
}
