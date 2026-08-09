package dockerruntime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/companionruntime"
)

type fakeCredentialCompanionProcess struct {
	aborts    int
	detaches  int
	abortErr  error
	detachErr error
}

func (p *fakeCredentialCompanionProcess) Abort() error {
	p.aborts++
	return p.abortErr
}

func (p *fakeCredentialCompanionProcess) Detach() error {
	p.detaches++
	return p.detachErr
}

type fakeCredentialCompanionLauncher struct {
	events           *[]string
	waitErr          error
	startErr         error
	waits            int
	starts           int
	startedEpoch     string
	startedContainer string
	startedState     string
	process          fakeCredentialCompanionProcess
}

func (l *fakeCredentialCompanionLauncher) WaitForStopped(_ context.Context, stateDirectory string) error {
	l.waits++
	if l.events != nil {
		*l.events = append(*l.events, "wait:"+stateDirectory)
	}
	return l.waitErr
}

func (l *fakeCredentialCompanionLauncher) Start(bootstrap *companionruntime.Bootstrap) (companionruntime.Process, error) {
	l.starts++
	if bootstrap != nil {
		l.startedEpoch = bootstrap.EpochID()
		l.startedContainer = bootstrap.ContainerID()
		l.startedState = bootstrap.StateDirectory()
	}
	if l.events != nil {
		*l.events = append(*l.events, "start")
	}
	if l.startErr != nil {
		return nil, l.startErr
	}
	return &l.process, nil
}

type credentialCompanionRunner struct {
	events       []string
	runs         [][]string
	outputs      [][]string
	epoch        string
	statusStates []string
	statusEpoch  string
	identity     []byte
}

func (r *credentialCompanionRunner) Run(
	_ context.Context,
	args []string,
	_ []string,
	_ io.Reader,
	out io.Writer,
	_ io.Writer,
) error {
	r.runs = append(r.runs, append([]string(nil), args...))
	operationIndex := slices.Index(args, "authbroker.control") + 1
	if operationIndex <= 0 || operationIndex >= len(args) {
		return fmt.Errorf("unexpected Docker run argv: %v", args)
	}
	operation := args[operationIndex]
	r.events = append(r.events, operation)
	switch operation {
	case "companion_prepare":
		epochIndex := slices.Index(args, "--epoch-id")
		if epochIndex < 0 || epochIndex+1 >= len(args) {
			return fmt.Errorf("missing epoch: %v", args)
		}
		r.epoch = args[epochIndex+1]
		_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":"prepared","epoch_id":%q}`+"\n", r.epoch)
	case "companion_status":
		state := "ready"
		if len(r.statusStates) != 0 {
			state = r.statusStates[0]
			r.statusStates = r.statusStates[1:]
		}
		epoch := r.epoch
		if r.statusEpoch != "" {
			epoch = r.statusEpoch
		}
		if state == "absent" {
			epoch = ""
		}
		_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":%q,"epoch_id":%q}`+"\n", state, epoch)
	default:
		return fmt.Errorf("unexpected control operation: %s", operation)
	}
	return nil
}

func (r *credentialCompanionRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, append([]string(nil), args...))
	r.events = append(r.events, "inspect")
	return append([]byte(nil), r.identity...), nil
}

func newCredentialCompanionTestRuntime(
	t *testing.T,
	runner *credentialCompanionRunner,
	launcher *fakeCredentialCompanionLauncher,
) *Runtime {
	t.Helper()
	root := t.TempDir()
	uid, gid := currentIDs()
	runner.identity = []byte(fmt.Sprintf(
		`{"id":"%s","owner":"default","component":"auth-broker","user":"%d:%d"}`,
		strings.Repeat("a", 64), uid, gid,
	))
	launcher.events = &runner.events
	return &Runtime{
		stateDirectory:   root,
		runner:           runner,
		companion:        launcher,
		companionEntropy: bytes.NewReader(bytes.Repeat([]byte{0x23}, 32)),
	}
}

func TestCredentialCompanionLifecycleBindsExactContainerAndEpoch(t *testing.T) {
	t.Parallel()
	runner := &credentialCompanionRunner{statusStates: []string{"prepared", "ready"}}
	launcher := &fakeCredentialCompanionLauncher{}
	runtime := newCredentialCompanionTestRuntime(t, runner, launcher)
	rootKey := bytes.Repeat([]byte{0x51}, 32)
	if err := runtime.startCredentialCompanion(context.Background(), rootKey); err != nil {
		t.Fatal(err)
	}
	uid, gid := currentIDs()
	if launcher.waits != 1 || launcher.starts != 1 || launcher.process.aborts != 0 || launcher.process.detaches != 1 {
		t.Fatalf("launcher waits=%d starts=%d process=%+v", launcher.waits, launcher.starts, launcher.process)
	}
	if launcher.startedContainer != strings.Repeat("a", 64) || launcher.startedState != runtime.stateDirectory ||
		!companionruntime.ValidEpochID(launcher.startedEpoch) || launcher.startedEpoch != runner.epoch {
		t.Fatalf("bootstrap container=%q state=%q epoch=%q prepared=%q", launcher.startedContainer, launcher.startedState, launcher.startedEpoch, runner.epoch)
	}
	wantEvents := []string{
		"inspect", "companion_prepare", "wait:" + runtime.stateDirectory,
		"start", "companion_status", "companion_status",
	}
	if !slices.Equal(runner.events, wantEvents) {
		t.Fatalf("events = %v, want %v", runner.events, wantEvents)
	}
	wantInspect := []string{
		"inspect", "--format",
		`{"id":{{json .Id}},"owner":{{json (index .Config.Labels "io.tobari.owner")}},"component":{{json (index .Config.Labels "io.tobari.component")}},"user":{{json .Config.User}}}`,
		authBrokerContainer,
	}
	if len(runner.outputs) != 1 || !slices.Equal(runner.outputs[0], wantInspect) {
		t.Fatalf("inspect argv = %v, want %v", runner.outputs, wantInspect)
	}
	prepare := runner.runs[0]
	wantPrefix := []string{"exec", "-i", authBrokerContainer, "python", "-m", "authbroker.control", "companion_prepare", "--epoch-id"}
	if len(prepare) != len(wantPrefix)+1 || !slices.Equal(prepare[:len(wantPrefix)], wantPrefix) || prepare[len(wantPrefix)] != launcher.startedEpoch {
		t.Fatalf("prepare argv = %v", prepare)
	}
	if got := strconv.Itoa(uid) + ":" + strconv.Itoa(gid); !bytes.Contains(runner.identity, []byte(got)) {
		t.Fatalf("identity does not bind current user %q: %s", got, runner.identity)
	}
}

func TestCredentialCompanionReadinessMismatchAbortsChild(t *testing.T) {
	t.Parallel()
	runner := &credentialCompanionRunner{statusEpoch: "companion-e1_" + strings.Repeat("A", 43)}
	launcher := &fakeCredentialCompanionLauncher{}
	runtime := newCredentialCompanionTestRuntime(t, runner, launcher)
	err := runtime.startCredentialCompanion(context.Background(), bytes.Repeat([]byte{0x51}, 32))
	if err == nil || launcher.process.aborts != 1 || launcher.process.detaches != 0 {
		t.Fatalf("error=%v process=%+v", err, launcher.process)
	}
}

func TestCredentialCompanionRejectsContainerIdentityBeforePrepare(t *testing.T) {
	t.Parallel()
	runner := &credentialCompanionRunner{}
	launcher := &fakeCredentialCompanionLauncher{}
	runtime := newCredentialCompanionTestRuntime(t, runner, launcher)
	runner.identity = []byte(`{"id":"` + strings.Repeat("a", 64) + `","owner":"other","component":"auth-broker","user":"1000:1000"}`)
	err := runtime.startCredentialCompanion(context.Background(), bytes.Repeat([]byte{0x51}, 32))
	if err == nil || len(runner.runs) != 0 || launcher.starts != 0 {
		t.Fatalf("error=%v runs=%v starts=%d", err, runner.runs, launcher.starts)
	}
}
