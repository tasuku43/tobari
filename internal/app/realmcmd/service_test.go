package realmcmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/realm"
)

type fakeRuntime struct {
	upCalls   int
	downCalls int
	state     realm.State
}

func (f *fakeRuntime) ResolveRoot(_ context.Context, root string) (string, error) { return root, nil }
func (f *fakeRuntime) CurrentDirectory(context.Context) (string, error)           { return f.state.Root, nil }
func (f *fakeRuntime) IsTerminal(io.Writer) bool                                  { return false }
func (f *fakeRuntime) Up(_ context.Context, _ string) (realm.State, error) {
	f.upCalls++
	return f.state, nil
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
func (f *fakeRuntime) Exec(context.Context, realm.State, realm.ExecRequest, io.Reader, io.Writer, io.Writer) (int, error) {
	return 23, nil
}
func (f *fakeRuntime) Logs(context.Context, realm.State, realm.LogRequest) ([]byte, error) {
	return []byte("safe\n"), nil
}
func (f *fakeRuntime) Down(context.Context, realm.State, bool) error {
	f.downCalls++
	return nil
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
