package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestContextPolicySnapshotUsesContextOwnedLayout(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContext(context.Background(), "policy-check", tobari.BuiltinImageSelector, tobari.ManifestSourceAccessReadWrite); err != nil {
		t.Fatal(err)
	}
	manifest, err := runtime.readContextManifestRaw("policy-check")
	if err != nil {
		t.Fatal(err)
	}
	policyPath := runtime.contextPolicyPath("policy-check")
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(policyPath), "preset.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy preset snapshot exists or could not be inspected: %v", err)
	}
	policy, err := runtime.readContextPolicy(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Name != "default" || manifest.PolicyRevision != tobari.DefaultContextPolicyRevision() {
		t.Fatalf("Context policy manifest/snapshot = %+v / %+v", manifest, policy)
	}
}
