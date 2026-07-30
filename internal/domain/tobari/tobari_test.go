package tobari

import (
	"path/filepath"
	"testing"
)

func validState(root string) State {
	return State{
		SchemaVersion: 2, RuntimeDirectory: filepath.Join(root, "runtime"),
		PolicyDirectory:  filepath.Join(root, "policy"),
		CredentialConfig: filepath.Join(root, "credentials.json"),
		CredentialDir:    filepath.Join(root, "credentials"), AssetVersion: "asset",
		ProxyEndpoint: "http://gateway:8080", Tobari: []Instance{},
	}
}

func TestStateRejectsDuplicateNamesRootsAndIDs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	base := Instance{
		ID: "tbr_0123456789abcdef0123456789abcdef", Name: "work",
		Root: filepath.Join(root, "a"), Container: "tobari-work",
		Network: "tobari-work-net", HomeVolume: "tobari-work-home",
	}
	tests := map[string]Instance{
		"id":   {ID: base.ID, Name: "other", Root: filepath.Join(root, "b"), Container: "tobari-other", Network: "tobari-other-net", HomeVolume: "tobari-other-home"},
		"name": {ID: "tbr_abcdef0123456789abcdef0123456789", Name: base.Name, Root: filepath.Join(root, "b"), Container: base.Container, Network: base.Network, HomeVolume: base.HomeVolume},
		"root": {ID: "tbr_abcdef0123456789abcdef0123456789", Name: "other", Root: base.Root, Container: "tobari-other", Network: "tobari-other-net", HomeVolume: "tobari-other-home"},
	}
	for name, duplicate := range tests {
		t.Run(name, func(t *testing.T) {
			state := validState(root)
			state.Tobari = []Instance{base, duplicate}
			if err := state.Validate(); err == nil {
				t.Fatal("duplicate state was accepted")
			}
		})
	}
}

func TestFindPreservesOpaqueIDBytes(t *testing.T) {
	t.Parallel()
	state := validState(t.TempDir())
	instance := Instance{
		ID: "tbr_0123456789abcdef0123456789abcdef", Name: "policy",
		Root: state.PolicyDirectory, Container: "tobari-policy",
		Network: "tobari-policy-net", HomeVolume: "tobari-policy-home",
	}
	state.Tobari = []Instance{instance}
	got, ok := state.Find(instance.ID)
	if !ok || got.ID != instance.ID {
		t.Fatalf("Find() = (%+v, %t)", got, ok)
	}
	if _, ok := state.Find("TBR_0123456789abcdef0123456789abcdef"); ok {
		t.Fatal("normalized ID unexpectedly matched")
	}
}

func TestMapHostCWDRejectsSiblingPrefix(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	root := filepath.Join(base, "root")
	sibling := filepath.Join(base, "root-other")
	if _, err := MapHostCWD(root, sibling); err == nil {
		t.Fatal("sibling path was accepted")
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

func TestListResultPreservesEmptyScope(t *testing.T) {
	t.Parallel()
	if err := (ListResult{Task: TaskList, Items: []ItemStatus{}}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (ListResult{Task: TaskList}).Validate(); err == nil {
		t.Fatal("unknown nil collection was accepted")
	}
}
