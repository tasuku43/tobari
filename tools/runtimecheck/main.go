// Command runtimecheck validates the canonical base image source and its
// embedded runtime snapshot before a build or publication workflow runs.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	baseDockerfile   = "runtimes/base/Dockerfile"
	baseEntrypoint   = "runtimes/base/entrypoint.sh"
	snapshotDir      = "internal/infra/runtimeassets/assets/tobari"
	baseRuntimeJSON  = "runtimes/base/runtime.json"
	baseLockJSON     = "runtimes/base/runtime.lock.json"
	manifestJSON     = "runtimes/manifest.json"
	versionsEnv      = "internal/infra/runtimeassets/assets/versions.env"
	canonicalSource  = "https://github.com/tasuku43/tobari"
	canonicalLicense = "MIT"
	canonicalUser    = "tobari"
	canonicalRuntime = "1"
	canonicalLife    = "sleep infinity"
)

var digestReference = regexp.MustCompile(`^debian@sha256:[0-9a-f]{64}$`)

type runtimeMetadata struct {
	SchemaVersion          int      `json:"schema_version"`
	Name                   string   `json:"name"`
	Package                string   `json:"package"`
	Kind                   string   `json:"kind"`
	Parent                 *string  `json:"parent"`
	RuntimeAPI             string   `json:"runtime_api"`
	RuntimeLifetimeCommand string   `json:"runtime_lifetime_command"`
	Entrypoint             []string `json:"entrypoint"`
	User                   string   `json:"user"`
	Architectures          []string `json:"architectures"`
	Source                 string   `json:"source"`
	License                string   `json:"license"`
}

type runtimeLock struct {
	SchemaVersion int `json:"schema_version"`
	BaseImage     struct {
		Name      string `json:"name"`
		Tag       string `json:"tag"`
		Reference string `json:"reference"`
	} `json:"base_image"`
}

type manifest struct {
	SchemaVersion int `json:"schema_version"`
	Images        []struct {
		Name    string  `json:"name"`
		Path    string  `json:"path"`
		Package string  `json:"package"`
		Parent  *string `json:"parent"`
	} `json:"images"`
}

func main() {
	rootFlag := flag.String("root", ".", "repository root")
	printBaseImage := flag.Bool("print-base-image", false, "print the validated Debian image reference")
	flag.Parse()
	root, err := filepath.Abs(*rootFlag)
	if err != nil {
		fatal(err)
	}

	baseImage, err := validate(root)
	if err != nil {
		fatal(err)
	}
	if *printBaseImage {
		fmt.Println(baseImage)
		return
	}
	fmt.Println("runtimecheck: OK")
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "runtimecheck: %v\n", err)
	os.Exit(1)
}

func validate(root string) (string, error) {
	metadata, err := readJSON[runtimeMetadata](root, baseRuntimeJSON)
	if err != nil {
		return "", err
	}
	if metadata.SchemaVersion != 1 || metadata.Name != "base" || metadata.Package != "tobari-runtime" || metadata.Kind != "base" {
		return "", errors.New("base runtime metadata has an invalid identity")
	}
	if metadata.Parent != nil || metadata.RuntimeAPI != canonicalRuntime || metadata.RuntimeLifetimeCommand != canonicalLife || metadata.User != canonicalUser {
		return "", errors.New("base runtime metadata does not declare the Tobari runtime contract")
	}
	if !sameStrings(metadata.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		return "", errors.New("base runtime metadata has an unexpected entrypoint")
	}
	if !sameStrings(metadata.Architectures, []string{"linux/amd64", "linux/arm64"}) {
		return "", errors.New("base runtime metadata must support linux/amd64 and linux/arm64")
	}
	if metadata.Source != canonicalSource || metadata.License != canonicalLicense {
		return "", errors.New("base runtime metadata has an invalid public source or license")
	}

	lock, err := readJSON[runtimeLock](root, baseLockJSON)
	if err != nil {
		return "", err
	}
	if lock.SchemaVersion != 1 || lock.BaseImage.Name != "debian" || lock.BaseImage.Tag == "" || !digestReference.MatchString(lock.BaseImage.Reference) {
		return "", errors.New("base runtime lock does not contain a digest-pinned Debian reference")
	}
	versions, err := readRegularFile(filepath.Join(root, versionsEnv))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", versionsEnv, err)
	}
	if envValue(string(versions), "DEBIAN_VERSION") != lock.BaseImage.Tag || envValue(string(versions), "DEBIAN_IMAGE") != lock.BaseImage.Reference {
		return "", errors.New("embedded runtime Debian pins do not match the base image lock")
	}

	rootManifest, err := readJSON[manifest](root, manifestJSON)
	if err != nil {
		return "", err
	}
	if rootManifest.SchemaVersion != 1 || len(rootManifest.Images) != 1 {
		return "", errors.New("runtime manifest must contain exactly the published base image")
	}
	image := rootManifest.Images[0]
	if image.Name != "base" || image.Path != "base" || image.Package != "tobari-runtime" || image.Parent != nil {
		return "", errors.New("runtime manifest has an invalid base image entry")
	}

	for _, relative := range []string{baseDockerfile, baseEntrypoint} {
		if err := compareCanonicalSnapshot(root, relative); err != nil {
			return "", err
		}
	}
	dockerfile, err := readRegularFile(filepath.Join(root, baseDockerfile))
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(dockerfile), "ARG DEBIAN_IMAGE="+lock.BaseImage.Reference) {
		return "", errors.New("base Dockerfile default does not match the digest lock")
	}
	spec := string(dockerfile)
	for _, required := range []string{
		"ARG DEBIAN_IMAGE=",
		"io.tobari.runtime-api=\"1\"",
		"io.tobari.runtime-lifetime-command=\"sleep infinity\"",
		"org.opencontainers.image.source=\"" + canonicalSource + "\"",
		"bash ca-certificates tini",
		"USER tobari",
		"ENTRYPOINT [\"/usr/bin/tini\", \"--\", \"/usr/local/bin/tobari-entrypoint\"]",
		"CMD [\"sleep\", \"infinity\"]",
	} {
		if !strings.Contains(spec, required) {
			return "", fmt.Errorf("base Dockerfile is missing %q", required)
		}
	}
	aptPackages := spec
	if start := strings.Index(aptPackages, "apt-get install"); start >= 0 {
		aptPackages = aptPackages[start:]
	}
	if end := strings.Index(aptPackages, "&& rm -rf"); end >= 0 {
		aptPackages = aptPackages[:end]
	}
	for _, forbidden := range []string{"curl", "git", "python3"} {
		if strings.Contains(aptPackages, forbidden) {
			return "", fmt.Errorf("base Dockerfile contains toolbox package %q", forbidden)
		}
	}

	return lock.BaseImage.Reference, nil
}

func readJSON[T any](root, relative string) (T, error) {
	var value T
	data, err := readRegularFile(filepath.Join(root, relative))
	if err != nil {
		return value, fmt.Errorf("read %s: %w", relative, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode %s: %w", relative, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return value, fmt.Errorf("decode %s: trailing JSON value", relative)
		}
		return value, fmt.Errorf("decode %s: %w", relative, err)
	}
	return value, nil
}

func readRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return os.ReadFile(path) // #nosec G304 -- paths are fixed repository-relative inputs.
}

func compareCanonicalSnapshot(root, relative string) error {
	source, err := readRegularFile(filepath.Join(root, relative))
	if err != nil {
		return fmt.Errorf("read canonical %s: %w", relative, err)
	}
	snapshotRelative := filepath.ToSlash(filepath.Join(snapshotDir, filepath.Base(relative)))
	snapshot, err := readRegularFile(filepath.Join(root, snapshotRelative))
	if err != nil {
		return fmt.Errorf("read embedded snapshot %s: %w", snapshotRelative, err)
	}
	if !bytes.Equal(source, snapshot) {
		return fmt.Errorf("embedded snapshot %s is out of sync with %s; run scripts/sync-runtime-base.sh", snapshotRelative, relative)
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func envValue(contents, key string) string {
	for _, line := range strings.Split(contents, "\n") {
		name, value, found := strings.Cut(line, "=")
		if found && name == key {
			return value
		}
	}
	return ""
}
