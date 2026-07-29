package realmcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

type fakeRuntime struct {
	upCalls   int
	downCalls int
	state     realm.State
	upErr     error
	downErr   error
	execSeen  realm.ExecRequest
}

func (f *fakeRuntime) ResolveRoot(_ context.Context, root string) (string, error) { return root, nil }
func (f *fakeRuntime) CurrentDirectory(context.Context) (string, error)           { return f.state.Root, nil }
func (f *fakeRuntime) IsTerminal(io.Writer) bool                                  { return false }
func (f *fakeRuntime) Up(_ context.Context, _ string) (realm.State, error) {
	f.upCalls++
	return f.state, f.upErr
}
func (f *fakeRuntime) LoadState(context.Context) (realm.State, bool, error) {
	return f.state, true, nil
}
func (f *fakeRuntime) Inspect(context.Context, realm.State) (realm.Status, error) {
	return realm.Status{
		Configured: true, Running: true, Root: f.state.Root, Proxy: f.state.ProxyEndpoint,
		Policy: f.state.PolicyDirectory, Components: []realm.ComponentStatus{},
	}, nil
}
func (f *fakeRuntime) Exec(_ context.Context, _ realm.State, request realm.ExecRequest, _ io.Reader, _ io.Writer, _ io.Writer) (int, error) {
	f.execSeen = request
	return 23, nil
}
func (f *fakeRuntime) Logs(context.Context, realm.State, realm.LogRequest) ([]byte, error) {
	return []byte("safe\n"), nil
}
func (f *fakeRuntime) Down(context.Context, realm.State, bool) error {
	f.downCalls++
	return f.downErr
}
func (f *fakeRuntime) Doctor(context.Context, string) (doctor.Report, error) {
	return doctor.Report{Checks: []doctor.Check{
		{Name: "docker", Status: doctor.CheckStatusPass, Detail: "available"},
	}}, nil
}

func testState() realm.State {
	return realm.State{
		SchemaVersion: 1, Root: "/tmp/root", RuntimeDirectory: "/tmp/runtime",
		PolicyDirectory: "/tmp/policy", CredentialConfig: "/tmp/credentials.json",
		CredentialDir: "/tmp/credentials", AssetVersion: "abc",
		ProxyEndpoint: "http://tobari-gateway:8080",
	}
}

func TestUpValidatesIntentBeforeRuntime(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState()}
	service := New(runtime)
	_, err := service.Up(context.Background(), operation.Intent{
		Command: "up", Effect: operation.EffectCreate,
		Target: operation.TargetRef{Kind: realm.TargetKind, ParentID: "wrong"},
		Impact: operation.Impact{
			Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
			AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo,
		},
	}, "/tmp/root")
	if err == nil {
		t.Fatal("invalid target was accepted")
	}
	if runtime.upCalls != 0 {
		t.Fatalf("runtime calls = %d, want 0", runtime.upCalls)
	}
}

func TestExecPreservesChildExitCode(t *testing.T) {
	t.Parallel()
	service := New(&fakeRuntime{state: testState()})
	code, err := service.Exec(
		context.Background(),
		realm.ExecRequest{Command: []string{"sh", "-c", "exit 23"}},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if code != 23 {
		t.Fatalf("code = %d, want 23", code)
	}
}

func TestMutationAdapterFailuresBecomeReconciliationFaults(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		run  func(*Service, operation.Intent) error
		code string
	}{
		{
			name: "up",
			run: func(service *Service, intent operation.Intent) error {
				_, err := service.Up(context.Background(), intent, "/tmp/root")
				return err
			},
			code: "realm_start_failed",
		},
		{
			name: "down",
			run: func(service *Service, intent operation.Intent) error {
				_, err := service.Down(context.Background(), intent, false)
				return err
			},
			code: "realm_stop_failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &fakeRuntime{
				state: testState(), upErr: errors.New("private Docker error"),
				downErr: errors.New("private Docker error"),
			}
			effect := operation.EffectCreate
			target := operation.TargetRef{Kind: realm.TargetKind, ParentID: realm.TargetID}
			if test.name == "down" {
				effect = operation.EffectWrite
				target = operation.TargetRef{Kind: realm.TargetKind, ID: realm.TargetID}
			}
			err := test.run(New(runtime), operation.Intent{
				Command: test.name, Effect: effect, Target: target,
				Impact: operation.Impact{
					Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo,
					AccessChange: operation.DeclarationNo,
					Destructive:  map[bool]operation.Declaration{true: operation.DeclarationYes, false: operation.DeclarationNo}[test.name == "down"],
				},
			})
			structured, ok := fault.PublicCopy(err)
			if !ok || structured.Code != test.code || structured.Retryable {
				t.Fatalf("fault = %#v, want non-retryable %s", err, test.code)
			}
			if len(structured.NextActions) != 1 || structured.NextActions[0].Command != "status" {
				t.Fatalf("next actions = %+v", structured.NextActions)
			}
		})
	}
}

func TestExecUsesCurrentDirectoryOnlyAsAnImplicitHint(t *testing.T) {
	t.Parallel()
	runtime := &fakeRuntime{state: testState()}
	service := New(runtime)
	_, err := service.Exec(
		context.Background(),
		realm.ExecRequest{Command: []string{"pwd"}, Interactive: true},
		bytes.NewReader(nil), io.Discard, io.Discard,
	)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.execSeen.HostCWD != runtime.state.Root || runtime.execSeen.CWDExplicit {
		t.Fatalf("exec request = %+v", runtime.execSeen)
	}
}
