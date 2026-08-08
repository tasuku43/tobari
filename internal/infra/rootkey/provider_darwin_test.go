//go:build darwin

package rootkey

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os/user"
	"reflect"
	"strings"
	"testing"
)

type fakeExitError struct{ code int }

func (e fakeExitError) Error() string { return "security command failed" }
func (e fakeExitError) ExitCode() int { return e.code }

type fakeSecurityRunner struct {
	output     []byte
	outputErr  error
	runErr     error
	outputArgs []string
	runArgs    []string
	stdin      []byte
}

func TestSecurityCommandEnvironmentUsesAccountHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	environment, err := securityCommandEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	found := ""
	for _, entry := range environment {
		if strings.HasPrefix(entry, "HOME=") {
			if found != "" {
				t.Fatal("Keychain subprocess environment contains duplicate HOME entries")
			}
			found = strings.TrimPrefix(entry, "HOME=")
		}
	}
	if found != account.HomeDir {
		t.Fatalf("Keychain subprocess HOME = %q, want %q", found, account.HomeDir)
	}
}

func (f *fakeSecurityRunner) Output(_ context.Context, args []string) ([]byte, error) {
	f.outputArgs = append([]string(nil), args...)
	return append([]byte(nil), f.output...), f.outputErr
}

func (f *fakeSecurityRunner) Run(_ context.Context, args []string, in io.Reader, _, _ io.Writer) error {
	f.runArgs = append([]string(nil), args...)
	f.stdin, _ = io.ReadAll(in)
	return f.runErr
}

func TestMacOSProviderLoadsWithoutSecretArguments(t *testing.T) {
	key := bytes.Repeat([]byte{0x35}, Size)
	runner := &fakeSecurityRunner{output: []byte(base64.RawURLEncoding.EncodeToString(key) + "\n")}
	provider := newMacOSProvider(runner, bytes.NewReader(nil))
	material, err := provider.LoadOrCreate(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(material.Bytes(), key) || material.Backend() != BackendMacOSKeychain {
		t.Fatalf("unexpected material")
	}
	if !reflect.DeepEqual(runner.outputArgs, []string{"find-generic-password", "-a", keychainAccount, "-s", keychainService, "-w"}) {
		t.Fatalf("unexpected Keychain args: %v", runner.outputArgs)
	}
	if backend, exists, err := provider.Inspect(context.Background(), true); err != nil || !exists || backend != BackendMacOSKeychain {
		t.Fatalf("inspect existing Keychain key: backend=%q exists=%t err=%v", backend, exists, err)
	}
}

func TestMacOSProviderCreatesThroughStdin(t *testing.T) {
	runner := &fakeSecurityRunner{outputErr: fakeExitError{code: 44}}
	key := bytes.Repeat([]byte{0x51}, Size)
	provider := newMacOSProvider(runner, bytes.NewReader(key))
	material, err := provider.LoadOrCreate(context.Background(), false)
	if err != nil || !bytes.Equal(material.Bytes(), key) {
		t.Fatalf("create material: material=%x err=%v", material.Bytes(), err)
	}
	wantArgs := []string{"add-generic-password", "-a", keychainAccount, "-s", keychainService, "-U", "-w"}
	if !reflect.DeepEqual(runner.runArgs, wantArgs) {
		t.Fatalf("unexpected Keychain update args: %v", runner.runArgs)
	}
	encoded := base64.RawURLEncoding.EncodeToString(key)
	wantInput := encoded + "\n" + encoded + "\n"
	if string(runner.stdin) != wantInput {
		t.Fatal("root key was not transferred through stdin")
	}
	for _, arg := range runner.runArgs {
		if arg == encoded || bytes.Contains([]byte(arg), key) {
			t.Fatal("root key leaked into Keychain command arguments")
		}
	}
}

func TestMacOSProviderMissingKeyWithVaultDoesNotGenerate(t *testing.T) {
	runner := &fakeSecurityRunner{outputErr: fakeExitError{code: 44}}
	provider := newMacOSProvider(runner, bytes.NewReader(bytes.Repeat([]byte{1}, Size)))
	if _, err := provider.LoadOrCreate(context.Background(), true); !errors.Is(err, ErrMissingWithVault) {
		t.Fatalf("expected missing-with-vault, got %v", err)
	}
	if len(runner.runArgs) != 0 {
		t.Fatalf("provider attempted replacement: %v", runner.runArgs)
	}
}

func TestMacOSProviderClassifiesDeniedLookup(t *testing.T) {
	runner := &fakeSecurityRunner{outputErr: errors.New("interaction denied")}
	provider := newMacOSProvider(runner, bytes.NewReader(nil))
	if _, err := provider.LoadOrCreate(context.Background(), false); !errors.Is(err, ErrDenied) {
		t.Fatalf("expected denied, got %v", err)
	}
}
