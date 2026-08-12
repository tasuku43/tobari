package tobari

import (
	"path/filepath"
	"testing"
)

func validState(root string) State {
	return State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ContextCount: 1,
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
	}
}

func TestValidateImageSelectorRejectsOptionAndTransportSyntax(t *testing.T) {
	t.Parallel()
	for _, image := range []string{
		BuiltinImageSelector,
		"workbench:dev",
		"ghcr.io/example/workbench:1.2.3",
		"localhost:5000/example/workbench@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	} {
		if err := ValidateImageSelector(image); err != nil {
			t.Errorf("ValidateImageSelector(%q) = %v", image, err)
		}
	}
	for _, image := range []string{"", "--pull=always", "https://example.com/image", "UPPER/name", "name:bad tag"} {
		if err := ValidateImageSelector(image); err == nil {
			t.Errorf("ValidateImageSelector(%q) accepted invalid input", image)
		}
	}
}
