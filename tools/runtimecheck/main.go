// Command runtimecheck validates the canonical base image source and its
// embedded runtime snapshot before a local or validation-only build runs.
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

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	baseDockerfile       = "runtimes/base/Dockerfile"
	baseEntrypoint       = "runtimes/base/entrypoint.sh"
	baseGitHubCLIWrapper = "runtimes/base/gh"
	baseAWSKey           = "runtimes/base/aws-cli-public-key.asc"
	snapshotDir          = "internal/infra/runtimeassets/assets/tobari"
	baseRuntimeJSON      = "runtimes/base/runtime.json"
	baseLockJSON         = "runtimes/base/runtime.lock.json"
	baseWorkflow         = ".github/workflows/runtime-base.yml"
	taskfile             = "Taskfile.yml"
	checkScript          = "scripts/check.sh"
	versionsEnv          = "internal/infra/runtimeassets/assets/versions.env"
	canonicalSource      = "https://github.com/tasuku43/tobari"
	canonicalLicense     = "NOASSERTION"
	canonicalUser        = "tobari"
	canonicalRuntime     = "1"
	canonicalLife        = "sleep infinity"
	canonicalPackage     = "tobari/runtime"
)

var digestReference = regexp.MustCompile(`^debian@sha256:[0-9a-f]{64}$`)
var builderReference = regexp.MustCompile(`^golang@sha256:[0-9a-f]{64}$`)
var versionReference = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var checksumReference = regexp.MustCompile(`^[0-9a-f]{64}$`)

var canonicalBaseTools = []string{
	"bash",
	"ca-certificates",
	"curl",
	"git",
	"jq",
	"openssh-client",
	"python3",
	"tini",
	"gh",
	"aws",
	"claude",
	"codex",
}

type runtimeMetadata struct {
	SchemaVersion          int      `json:"schema_version"`
	Name                   string   `json:"name"`
	Version                string   `json:"version"`
	Package                string   `json:"package"`
	Kind                   string   `json:"kind"`
	Parent                 *string  `json:"parent"`
	RuntimeAPI             string   `json:"runtime_api"`
	RuntimeLifetimeCommand string   `json:"runtime_lifetime_command"`
	Entrypoint             []string `json:"entrypoint"`
	User                   string   `json:"user"`
	Architectures          []string `json:"architectures"`
	Tools                  []string `json:"tools"`
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
	Tools struct {
		GH     toolLock          `json:"gh"`
		AWSCLI toolLock          `json:"aws_cli"`
		Claude agentArtifactLock `json:"claude"`
		Codex  agentArtifactLock `json:"codex"`
	} `json:"tools"`
}

type toolLock struct {
	Version string `json:"version"`
	Source  string `json:"source"`
}

type agentArtifactLock struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	Source        string `json:"source"`
	LicenseReview string `json:"license_review"`
	Platforms     map[string]struct {
		Asset  string `json:"asset"`
		SHA256 string `json:"sha256"`
		Size   int    `json:"size"`
	} `json:"platforms"`
}

func main() {
	rootFlag := flag.String("root", ".", "repository root")
	printBaseImage := flag.Bool("print-base-image", false, "print the validated Debian image reference")
	printGoBuilderImage := flag.Bool("print-go-builder-image", false, "print the validated Go builder image reference")
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
	if *printGoBuilderImage {
		builderImage, err := validatedGoBuilderImage(root)
		if err != nil {
			fatal(err)
		}
		fmt.Println(builderImage)
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
	if metadata.SchemaVersion != 1 || metadata.Name != "base" || metadata.Package != canonicalPackage || metadata.Kind != "base" || !versionReference.MatchString(metadata.Version) {
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
	if !sameStrings(metadata.Tools, canonicalBaseTools) {
		return "", errors.New("base runtime metadata has an unexpected common tool set")
	}

	lock, err := readJSON[runtimeLock](root, baseLockJSON)
	if err != nil {
		return "", err
	}
	if lock.SchemaVersion != 2 || lock.BaseImage.Name != "debian" || lock.BaseImage.Tag == "" || !digestReference.MatchString(lock.BaseImage.Reference) {
		return "", errors.New("base runtime lock does not contain a digest-pinned Debian reference")
	}
	if lock.Tools.GH.Version != tobari.AgentReadyGitHubCLIVersion || lock.Tools.GH.Source != "https://github.com/cli/cli/releases" ||
		!versionReference.MatchString(lock.Tools.AWSCLI.Version) || lock.Tools.AWSCLI.Source != "https://awscli.amazonaws.com/" {
		return "", errors.New("base runtime lock does not contain the approved common CLI pins")
	}
	if err := validateAgentArtifactLock(lock.Tools.Claude, "claude-code", tobari.AgentReadyClaudeVersion, "https://downloads.claude.ai/claude-code-releases", map[string]string{
		"linux/amd64": "linux-x64/claude",
		"linux/arm64": "linux-arm64/claude",
	}); err != nil {
		return "", fmt.Errorf("base runtime Claude artifact lock: %w", err)
	}
	if err := validateAgentArtifactLock(lock.Tools.Codex, "codex-cli", tobari.AgentReadyCodexVersion, "https://releases.openai.com/codex", map[string]string{
		"linux/amd64": "codex-package-x86_64-unknown-linux-musl.tar.gz",
		"linux/arm64": "codex-package-aarch64-unknown-linux-musl.tar.gz",
	}); err != nil {
		return "", fmt.Errorf("base runtime Codex artifact lock: %w", err)
	}
	versions, err := readRegularFile(filepath.Join(root, versionsEnv))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", versionsEnv, err)
	}
	if envValue(string(versions), "DEBIAN_VERSION") != lock.BaseImage.Tag || envValue(string(versions), "DEBIAN_IMAGE") != lock.BaseImage.Reference {
		return "", errors.New("embedded runtime Debian pins do not match the base image lock")
	}
	if _, err := validatedGoBuilderImage(root); err != nil {
		return "", err
	}
	if err := validateRetiredRuntimeSurface(root); err != nil {
		return "", err
	}

	for _, relative := range []string{baseDockerfile, baseEntrypoint, baseGitHubCLIWrapper, baseAWSKey} {
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
	wrapper, err := readRegularFile(filepath.Join(root, baseGitHubCLIWrapper))
	if err != nil {
		return "", err
	}
	wrapperText := string(wrapper)
	for _, required := range []string{
		`real_gh=/opt/tobari/bin/gh`,
		`if [ "$#" -eq 2 ] && [ "$1" = "auth" ] && [ "$2" = "login" ]`,
		`auth login --hostname github.com --git-protocol https --web`,
		`auth setup-git --hostname github.com`,
		`exec "$real_gh" "$@"`,
	} {
		if !strings.Contains(wrapperText, required) {
			return "", fmt.Errorf("base GitHub CLI wrapper is missing %q", required)
		}
	}
	for _, forbidden := range []string{"curl ", "wget ", "xdg-open", "eval ", "sh -c", "http://", "https://", "NO_COLOR="} {
		if strings.Contains(wrapperText, forbidden) {
			return "", fmt.Errorf("base GitHub CLI wrapper contains forbidden authority %q", forbidden)
		}
	}
	spec := string(dockerfile)
	for _, required := range []string{
		"ARG DEBIAN_IMAGE=",
		"ARG GO_BUILDER_IMAGE=golang@sha256:",
		"FROM --platform=$BUILDPLATFORM ${GO_BUILDER_IMAGE} AS exposure-helper-builder",
		"COPY --from=helper-source . .",
		"go build -tags=tobari_exposure_helper -buildvcs=false -trimpath",
		"-o /out/tobari-expose ./cmd/tobari-expose",
		`io.tobari.exposure-helper-api="1"`,
		`io.tobari.exposure-helper-source="${TOBARI_EXPOSURE_HELPER_SOURCE}"`,
		"COPY --from=exposure-helper-builder /out/tobari-expose /opt/tobari/libexec/tobari-expose",
		"COPY --from=exposure-helper-builder /out/identity.json /opt/tobari/libexec/tobari-expose.identity.json",
		"FROM ${DEBIAN_IMAGE} AS fetcher",
		"ARG GH_VERSION=" + lock.Tools.GH.Version,
		"ARG AWS_CLI_VERSION=" + lock.Tools.AWSCLI.Version,
		"ARG CLAUDE_CODE_VERSION=" + lock.Tools.Claude.Version,
		"ARG CLAUDE_CODE_SHA256_X64=" + lock.Tools.Claude.Platforms["linux/amd64"].SHA256,
		"ARG CLAUDE_CODE_SHA256_ARM64=" + lock.Tools.Claude.Platforms["linux/arm64"].SHA256,
		"ARG CODEX_VERSION=" + lock.Tools.Codex.Version,
		"ARG CODEX_PACKAGE_SHA256_X64=" + lock.Tools.Codex.Platforms["linux/amd64"].SHA256,
		"ARG CODEX_PACKAGE_SHA256_ARM64=" + lock.Tools.Codex.Platforms["linux/arm64"].SHA256,
		"COPY aws-cli-public-key.asc /tmp/aws-cli-public-key.asc",
		"io.tobari.runtime-api=\"1\"",
		"io.tobari.runtime-lifetime-command=\"sleep infinity\"",
		"org.opencontainers.image.licenses=\"" + canonicalLicense + "\"",
		"org.opencontainers.image.source=\"" + canonicalSource + "\"",
		"COPY --from=fetcher /opt/aws-cli /opt/aws-cli",
		"COPY --from=fetcher /out/gh /opt/tobari/bin/gh",
		"COPY gh /usr/local/bin/gh",
		"COPY --from=fetcher /out/claude /usr/local/bin/claude",
		"COPY --from=fetcher /out/codex /opt/tobari/codex",
		"https://github.com/cli/cli/releases/download/v",
		"https://awscli.amazonaws.com/",
		"https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/manifest.json",
		"https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/${claude_platform}/claude",
		"https://releases.openai.com/codex/releases/${CODEX_VERSION}/${codex_package}",
		"sha256sum --check --strict",
		"gpg --batch --verify",
		"ENV HOME=/var/lib/tobari",
		"ENV DISABLE_AUTOUPDATER=1",
		"ln -s \"${codex_release_dir}/bin/codex\" /usr/local/bin/codex",
		"ln -s \"${codex_release_dir}/bin/codex-code-mode-host\" /usr/local/bin/codex-code-mode-host",
		"ln -s \"${codex_release_dir}/codex-path/rg\" /usr/local/bin/rg",
		"USER tobari",
		"RUN claude --version && codex --version",
		"ENTRYPOINT [\"/usr/bin/tini\", \"--\", \"/usr/local/bin/tobari-entrypoint\"]",
		"CMD [\"sleep\", \"infinity\"]",
	} {
		if !strings.Contains(spec, required) {
			return "", fmt.Errorf("base Dockerfile is missing %q", required)
		}
	}
	aptPackages := spec
	if start := strings.LastIndex(aptPackages, "apt-get install"); start >= 0 {
		aptPackages = aptPackages[start:]
	}
	if end := strings.Index(aptPackages, "&& rm -rf"); end >= 0 {
		aptPackages = aptPackages[:end]
	}
	for _, required := range []string{"bash", "ca-certificates", "curl", "git", "jq", "openssh-client", "python3", "tini"} {
		found := false
		for _, token := range strings.Fields(aptPackages) {
			if token == required {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("base Dockerfile is missing common package %q", required)
		}
	}
	if !strings.Contains(spec, "ln -s /opt/aws-cli/v2/current/bin/aws /usr/local/bin/aws") {
		return "", errors.New("base Dockerfile does not expose the AWS CLI")
	}
	for _, forbidden := range []string{"COPY tobari-expose ", "ENV CODEX_HOME=", "/var/lib/tobari/.local/bin/claude", "/var/lib/tobari/.codex/packages", "claude update", "codex update"} {
		if strings.Contains(spec, forbidden) {
			return "", fmt.Errorf("base Dockerfile contains forbidden mutable agent installation %q", forbidden)
		}
	}
	workflow, err := readRegularFile(filepath.Join(root, baseWorkflow))
	if err != nil {
		return "", err
	}
	workflowText := string(workflow)
	if strings.Contains(workflowText, "packages: write") || strings.Contains(workflowText, "--push") || strings.Contains(workflowText, "docker login") {
		return "", errors.New("agent-ready base workflow must remain local-build validation only")
	}
	if !strings.Contains(workflowText, "--output type=cacheonly") {
		return "", errors.New("agent-ready base workflow must retain build-only validation")
	}
	for _, required := range []string{
		"--platform linux/amd64,linux/arm64",
		"--build-context helper-source=internal/infra/runtimeassets/_helper-source",
		"go_builder_image=$(go run ./tools/runtimecheck --print-go-builder-image)",
		"--build-arg \"GO_BUILDER_IMAGE=",
		"--build-arg \"TOBARI_EXPOSURE_HELPER_SOURCE=",
	} {
		if !strings.Contains(workflowText, required) {
			return "", fmt.Errorf("agent-ready base workflow is missing helper construction evidence %q", required)
		}
	}

	return lock.BaseImage.Reference, nil
}

func validLicenseReview(status string) bool {
	return status == "pending" || status == "approved"
}

func validateAgentArtifactLock(lock agentArtifactLock, name, version, source string, expectedPlatforms map[string]string) error {
	if lock.Name != name || lock.Version != version || lock.Source != source || !validLicenseReview(lock.LicenseReview) {
		return errors.New("identity, source, version, or license review is invalid")
	}
	if len(lock.Platforms) != len(expectedPlatforms) {
		return errors.New("architecture matrix is invalid")
	}
	for platform, asset := range expectedPlatforms {
		entry, ok := lock.Platforms[platform]
		if !ok || entry.Asset != asset || !checksumReference.MatchString(entry.SHA256) || entry.Size <= 0 {
			return fmt.Errorf("%s artifact is invalid", platform)
		}
	}
	return nil
}

func validatedGoBuilderImage(root string) (string, error) {
	versions, err := readRegularFile(filepath.Join(root, versionsEnv))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", versionsEnv, err)
	}
	builderImage := envValue(string(versions), "GO_BUILDER_IMAGE")
	if !builderReference.MatchString(builderImage) {
		return "", errors.New("embedded runtime Go builder image is not digest pinned")
	}
	dockerfile, err := readRegularFile(filepath.Join(root, baseDockerfile))
	if err != nil {
		return "", err
	}
	if !strings.Contains(string(dockerfile), "ARG GO_BUILDER_IMAGE="+builderImage) {
		return "", errors.New("base Dockerfile Go builder default does not match embedded runtime pins")
	}
	return builderImage, nil
}

func validateRetiredRuntimeSurface(root string) error {
	retiredPaths := []string{
		"runtimes/manifest.json",
		"runtimes/claude/Dockerfile",
		"runtimes/claude/runtime.json",
		"runtimes/claude/runtime.lock.json",
		"runtimes/codex/Dockerfile",
		"runtimes/codex/runtime.json",
		"runtimes/codex/runtime.lock.json",
		"scripts/build-runtime-claude.sh",
		"scripts/build-runtime-codex.sh",
		"scripts/check-runtime-claude.sh",
		"scripts/check-runtime-codex.sh",
		".github/workflows/runtime-claude.yml",
		".github/workflows/runtime-codex.yml",
	}
	for _, relative := range retiredPaths {
		if _, err := os.Lstat(filepath.Join(root, relative)); err == nil {
			return fmt.Errorf("retired per-agent Runtime path is present: %s", relative)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect retired Runtime path %s: %w", relative, err)
		}
	}
	for _, boundary := range []string{taskfile, checkScript} {
		contents, err := readRegularFile(filepath.Join(root, boundary))
		if err != nil {
			return fmt.Errorf("read %s: %w", boundary, err)
		}
		for _, retired := range []string{"runtime:claude", "runtime:codex", "check-runtime-claude", "check-runtime-codex"} {
			if strings.Contains(string(contents), retired) {
				return fmt.Errorf("%s retains retired per-agent Runtime entry %q", boundary, retired)
			}
		}
	}
	return nil
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
		if left[index] != right[index] { // #nosec G602 -- lengths are checked before paired indexing.
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
