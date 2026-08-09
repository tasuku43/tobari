package credentialhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type pupLoginRunner struct {
	t         *testing.T
	malformed bool
	calls     int
}

type pupTimeoutRunner struct{}

func (pupTimeoutRunner) Run(ctx context.Context, _ Command) error {
	<-ctx.Done()
	return ctx.Err()
}

func (r *pupLoginRunner) Run(_ context.Context, command Command) error {
	r.calls++
	if command.Args[0] == "--no-agent" {
		if got, want := command.Args, []string{"--no-agent", "auth", "login", "--site", PupSite}; !reflect.DeepEqual(got, want) {
			r.t.Fatalf("pup argv = %#v", got)
		}
		environment := map[string]string{}
		for _, item := range command.Env {
			parts := strings.SplitN(item, "=", 2)
			environment[parts[0]] = parts[1]
		}
		for _, name := range []string{"AWS_ACCESS_KEY_ID", "DD_ACCESS_TOKEN", "HTTPS_PROXY", "BROWSER"} {
			if _, inherited := environment[name]; inherited {
				r.t.Fatalf("pup inherited %s", name)
			}
		}
		if environment["DD_TOKEN_STORAGE"] != "file" || environment["DD_SITE"] != PupSite ||
			environment["PUP_CONFIG_DIR"] == "" || environment["HOME"] != command.Dir {
			r.t.Fatalf("pup environment = %#v", environment)
		}
		config := environment["PUP_CONFIG_DIR"]
		client := pupClientCredentials{
			ClientID: "client-example-123", ClientName: pupClientName,
			RedirectURIs: []string{"http://127.0.0.1:8000/oauth/callback"},
			RegisteredAt: 1_800_000_000, Site: PupSite,
		}
		token := pupTokenSet{
			AccessToken: "dummy-access-token", RefreshToken: "dummy-refresh-token",
			TokenType: "Bearer", ExpiresIn: 3600, IssuedAt: 1_800_000_001,
			Scope: "dashboards_read metrics_read", ClientID: client.ClientID,
		}
		writePrivateJSON := func(name string, value any) {
			content, err := json.Marshal(value)
			if err != nil {
				r.t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config, name), content, 0o600); err != nil {
				r.t.Fatal(err)
			}
		}
		if r.malformed {
			writePrivateJSON("client_datadoghq_com.json", map[string]any{"client_id": "leak"})
		} else {
			writePrivateJSON("client_datadoghq_com.json", client)
		}
		writePrivateJSON("tokens_datadoghq_com.json", map[string]pupTokenSet{"__default__": token})
		writePrivateJSON("sessions.json", []map[string]any{{"site": PupSite, "org": nil}})
		_, _ = io.WriteString(command.Stderr, "OAuth login complete\n")
	}
	return nil
}

func TestPupLoginUsesFixedIsolatedPlanAndReturnsCanonicalOpaqueState(t *testing.T) {
	executable := testExecutable(t)
	runner := &pupLoginRunner{t: t}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	var visible strings.Builder
	state, err := driver.PupLogin(
		context.Background(), executable, strings.NewReader(""),
		func(_ OutputStream, content []byte) error { _, err := visible.Write(content); return err },
	)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Clear()
	if runner.calls != 1 || state.Site() != PupSite || state.DriverID() != PupDriverID ||
		!strings.Contains(visible.String(), "OAuth login complete") {
		t.Fatalf("pup state metadata = site=%q driver=%q calls=%d visible=%q", state.Site(), state.DriverID(), runner.calls, visible.String())
	}
	encoded, err := state.Encode()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePupState(encoded)
	if err != nil {
		t.Fatal(err)
	}
	defer decoded.Clear()
	if strings.Contains(state.String(), "access-token") || strings.Contains(state.GoString(), "refresh-token") {
		t.Fatal("pup state formatting exposed credential material")
	}
	entries, err := os.ReadDir(driver.tempRoot)
	if err != nil || len(entries) != 0 {
		t.Fatalf("pup temporary home was not removed: entries=%v err=%v", entries, err)
	}
}

func TestPupLoginRejectsMalformedFileStateAndCleansUp(t *testing.T) {
	runner := &pupLoginRunner{t: t, malformed: true}
	driver := NewDriver(runner)
	driver.tempRoot = t.TempDir()
	state, err := driver.PupLogin(context.Background(), testExecutable(t), strings.NewReader(""), nil)
	state.Clear()
	if !errors.Is(err, ErrInvalidPupState) {
		t.Fatalf("PupLogin malformed state error = %v", err)
	}
	entries, readErr := os.ReadDir(driver.tempRoot)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("pup malformed-state cleanup entries=%v err=%v", entries, readErr)
	}
}

func TestPupLoginOwnsTheBoundedCallbackDeadlineAndCleansUp(t *testing.T) {
	driver := NewDriver(pupTimeoutRunner{})
	driver.tempRoot = t.TempDir()
	driver.pupTimeout = 10 * time.Millisecond
	state, err := driver.PupLogin(
		context.Background(), testExecutable(t), strings.NewReader(""), nil,
	)
	state.Clear()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PupLogin timeout error = %v", err)
	}
	entries, readErr := os.ReadDir(driver.tempRoot)
	if readErr != nil || len(entries) != 0 {
		t.Fatalf("pup timeout cleanup entries=%v err=%v", entries, readErr)
	}
}
