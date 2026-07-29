package realm

import (
	"path/filepath"
	"testing"
)

func TestMapHostCWD(t *testing.T) {
	t.Parallel()
	root := filepath.Clean("/tmp/tobari-root")
	got, err := MapHostCWD(root, filepath.Join(root, "repo", "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "/workspace/repo/sub" {
		t.Fatalf("mapped cwd = %q", got)
	}
	if _, err := MapHostCWD(root, filepath.Clean("/tmp/outside")); err == nil {
		t.Fatal("outside cwd was accepted")
	}
}

func TestStatusKeepsEmptyConfiguredScope(t *testing.T) {
	t.Parallel()
	status := Status{
		Task: TaskStatus, Configured: true, Root: "/tmp/root",
		Proxy: "http://gateway:8080", Policy: "/tmp/policy",
		Components: []ComponentStatus{},
	}
	if err := status.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStateRejectsUnsafeRecentError(t *testing.T) {
	t.Parallel()
	state := State{
		SchemaVersion: 1, Root: "/tmp/root", RuntimeDirectory: "/tmp/runtime",
		PolicyDirectory: "/tmp/policy", CredentialConfig: "/tmp/credentials.json",
		CredentialDir: "/tmp/credentials", AssetVersion: "abc",
		ProxyEndpoint: "http://gateway:8080", RecentError: "line\nbreak",
	}
	if err := state.Validate(); err == nil {
		t.Fatal("unsafe recent error was accepted")
	}
}
