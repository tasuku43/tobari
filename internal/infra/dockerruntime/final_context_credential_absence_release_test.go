//go:build !tobari_dev || !tobari_research

package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestReleaseContextCredentialAbsenceRequiresBothAuthRootsExactlyAbsent(t *testing.T) {
	root := t.TempDir()
	runtime := &Runtime{stateDirectory: filepath.Join(root, "state"), configDirectory: filepath.Join(root, "config")}
	contextID := tobari.ContextID("0191f2b1-1b58-7f3e-8f1a-22db8b6bd101")
	if err := runtime.ConfirmContextCredentialAbsent(context.Background(), contextID); err != nil {
		t.Fatalf("clean release absence: %v", err)
	}
	for _, path := range []string{filepath.Join(runtime.stateDirectory, "auth"), filepath.Join(runtime.configDirectory, "auth")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "hostile-predecessor"), []byte("not decoded"), 0o000); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmContextCredentialAbsent(context.Background(), contextID); err == nil {
			t.Fatalf("auth root %s was treated as exact credential absence", path)
		}
		if err := os.RemoveAll(path); err != nil {
			t.Fatal(err)
		}
	}
}
