package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDevContainerAcceptsJSONCImageSubset(t *testing.T) {
	t.Parallel()
	config, err := parseDevContainer([]byte(`{
		// A URL inside a string is not a comment.
		"$schema": "https://containers.dev/schema.json",
		"name": "work",
		"image": "workbench:dev",
		"customizations": {
			"editor": {"setting": true,},
		},
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if config.Image != "workbench:dev" || len(config.UnsupportedProperties()) != 0 {
		t.Fatalf("config = %+v", config)
	}
}

func TestParseDevContainerRejectsAmbiguousOrMalformedInput(t *testing.T) {
	t.Parallel()
	for name, data := range map[string]string{
		"duplicate image":        `{"image":"first:dev","image":"second:dev"}`,
		"non-string image":       `{"image":{"name":"workbench:dev"}}`,
		"non-string name":        `{"image":"workbench:dev","name":false}`,
		"invalid customizations": `{"image":"workbench:dev","customizations":[]}`,
		"unclosed comment":       `{"image":"workbench:dev", /* }`,
		"trailing data":          `{"image":"workbench:dev"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseDevContainer([]byte(data)); err == nil {
				t.Fatal("invalid Dev Container document was accepted")
			}
		})
	}
}

func TestParseDevContainerLeavesMissingImageForTaskValidation(t *testing.T) {
	t.Parallel()
	config, err := parseDevContainer([]byte(`{"name":"work"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := config.Validate(); err == nil {
		t.Fatal("missing image was accepted by task validation")
	}
}

func TestParseDevContainerRetainsUnsupportedPropertiesForApplicationRejection(t *testing.T) {
	t.Parallel()
	config, err := parseDevContainer([]byte(`{
		"image": "workbench:dev",
		"privileged": true,
		"mounts": ["source=/,target=/host,type=bind"]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	unsupported := config.UnsupportedProperties()
	if len(unsupported) != 2 || unsupported[0] != "mounts" || unsupported[1] != "privileged" {
		t.Fatalf("unsupported = %v", unsupported)
	}
}

func TestReadDevContainerRequiresContainedRegularFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	configDirectory := filepath.Join(root, ".devcontainer")
	if err := os.Mkdir(configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDirectory, "devcontainer.json")
	if err := os.WriteFile(configPath, []byte(`{"image":"workbench:dev"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	config, err := runtime.ReadDevContainer(
		context.Background(), root, ".devcontainer/devcontainer.json",
	)
	if err != nil || config.Image != "workbench:dev" {
		t.Fatalf("ReadDevContainer() = (%+v, %v)", config, err)
	}

	outside := t.TempDir()
	outside, _ = filepath.EvalSymlinks(outside)
	outsidePath := filepath.Join(outside, "devcontainer.json")
	if err := os.WriteFile(outsidePath, []byte(`{"image":"outside:dev"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(configDirectory, "escaped.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReadDevContainer(
		context.Background(), root, ".devcontainer/escaped.json",
	); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}

func TestReadDevContainerRejectsOversizedFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	path := filepath.Join(root, "devcontainer.json")
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", maxDevContainerBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if _, err := runtime.ReadDevContainer(context.Background(), root, path); err == nil {
		t.Fatal("oversized Dev Container file was accepted")
	}
}
