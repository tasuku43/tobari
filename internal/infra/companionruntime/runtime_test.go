//go:build !windows

package companionruntime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type captureBridgeRunner struct {
	request bridgeCommand
	err     error
}

func (r *captureBridgeRunner) Run(
	_ context.Context,
	request bridgeCommand,
	_ func(io.Reader, io.Writer) error,
) error {
	r.request = request
	return r.err
}

type unusedRefreshDriver struct{}

func (unusedRefreshDriver) Refresh(
	context.Context,
	credentialhost.State,
) (credentialhost.TemporaryCredentials, credentialhost.State, error) {
	return credentialhost.TemporaryCredentials{}, credentialhost.State{}, errors.New("unexpected refresh")
}

func TestPrivateRunUsesExactDockerBridgeAndNoSecretProcessMetadata(t *testing.T) {
	t.Parallel()
	stateDirectory := t.TempDir()
	if err := os.Chmod(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stateDirectory+"/auth", 0o700); err != nil {
		t.Fatal(err)
	}
	rootKey := bytes.Repeat([]byte("R"), 32)
	bootstrap, err := NewBootstrap(
		bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)), rootKey,
		strings.Repeat("a", 64), 501, 20, stateDirectory,
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := bootstrap.Encode()
	if err != nil {
		t.Fatal(err)
	}
	derivedKey := append([]byte(nil), encoded[len(encoded)-32:]...)
	bootstrap.Clear()
	defer clear(encoded)
	defer clear(derivedKey)
	wantErr := errors.New("synthetic bridge stop")
	runner := &captureBridgeRunner{err: wantErr}
	err = run(context.Background(), bytes.NewReader(encoded), runner, unusedRefreshDriver{})
	if !errors.Is(err, wantErr) {
		t.Fatalf("run error = %v, want %v", err, wantErr)
	}
	wantArgs := []string{
		"exec", "-i", "--user", "501:20", strings.Repeat("a", 64),
		"python3", "-m", "authbroker.companion_bridge",
	}
	if runner.request.Path != "docker" || !reflect.DeepEqual(runner.request.Args, wantArgs) {
		t.Fatalf("bridge = path:%q args:%v, want docker %v", runner.request.Path, runner.request.Args, wantArgs)
	}
	metadata := runner.request.Path + "\n" + strings.Join(runner.request.Args, "\n") + "\n" + strings.Join(runner.request.Env, "\n")
	for _, secret := range []string{string(rootKey), string(derivedKey)} {
		if strings.Contains(metadata, secret) {
			t.Fatal("bridge process metadata contains key material")
		}
	}
	for _, argument := range runner.request.Args {
		if argument == "sh" || argument == "bash" || argument == "-t" || argument == "--tty" {
			t.Fatalf("bridge argv contains shell or TTY: %v", runner.request.Args)
		}
	}
	for _, variable := range runner.request.Env {
		name := strings.SplitN(variable, "=", 2)[0]
		allowed := false
		for _, candidate := range dockerEnvironmentNames {
			allowed = allowed || name == candidate
		}
		if !allowed {
			t.Fatalf("bridge inherited unapproved environment variable %q", name)
		}
	}
}

func TestOSLauncherUsesExactPrivateArgZeroAndClosedEnvironment(t *testing.T) {
	t.Parallel()
	bootstrap, err := NewBootstrap(
		bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)), bytes.Repeat([]byte{0x32}, 32),
		strings.Repeat("a", 64), 501, 20, "/tmp/tobari-state",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrap.Clear()
	launcher := &OSLauncher{executable: func() (string, error) { return "/bin/cat", nil }}
	started, err := launcher.Start(bootstrap)
	if err != nil {
		t.Fatal(err)
	}
	process, ok := started.(*osStartedProcess)
	if !ok {
		t.Fatalf("process type = %T", started)
	}
	if process.command.Path != "/bin/cat" || !reflect.DeepEqual(process.command.Args, []string{PrivateArg0}) {
		t.Fatalf("path=%q argv=%v", process.command.Path, process.command.Args)
	}
	for _, variable := range process.command.Env {
		name := strings.SplitN(variable, "=", 2)[0]
		allowed := false
		for _, candidate := range dockerEnvironmentNames {
			allowed = allowed || name == candidate
		}
		if !allowed {
			t.Fatalf("launcher inherited unapproved environment variable %q", name)
		}
	}
	if err := process.Abort(); err != nil {
		t.Fatal(err)
	}
}

func TestLifetimeLockPreventsOverlapAndWaitsForRelease(t *testing.T) {
	t.Parallel()
	stateDirectory := t.TempDir()
	if err := os.Chmod(stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stateDirectory+"/auth", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prepareCompanionDirectory(stateDirectory); err != nil {
		t.Fatal(err)
	}
	lock, err := tryAcquireLifetimeLock(lifetimeLockPath(stateDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := tryAcquireLifetimeLock(lifetimeLockPath(stateDirectory)); !errors.Is(err, ErrAlreadyRunning) {
		if duplicate != nil {
			_ = duplicate.Close()
		}
		t.Fatalf("duplicate lock error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := WaitForStopped(ctx, stateDirectory); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitForStopped while held = %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := WaitForStopped(context.Background(), stateDirectory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(lifetimeLockPath(stateDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lock mode=%v", info.Mode().Perm())
	}
	ownerInfo, err := os.Stat(companionDirectory(stateDirectory))
	if err != nil {
		t.Fatal(err)
	}
	if ownerInfo.Mode().Perm() != 0o700 {
		t.Fatalf("owner mode=%v", ownerInfo.Mode().Perm())
	}
}
