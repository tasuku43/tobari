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

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	baseDockerfile        = "runtimes/base/Dockerfile"
	baseEntrypoint        = "runtimes/base/entrypoint.sh"
	baseAWSKey            = "runtimes/base/aws-cli-public-key.asc"
	claudeDockerfile      = "runtimes/claude/Dockerfile"
	claudeRuntimeJSON     = "runtimes/claude/runtime.json"
	claudeLockJSON        = "runtimes/claude/runtime.lock.json"
	codexDockerfile       = "runtimes/codex/Dockerfile"
	codexRuntimeJSON      = "runtimes/codex/runtime.json"
	codexLockJSON         = "runtimes/codex/runtime.lock.json"
	snapshotDir           = "internal/infra/runtimeassets/assets/tobari"
	baseRuntimeJSON       = "runtimes/base/runtime.json"
	baseLockJSON          = "runtimes/base/runtime.lock.json"
	manifestJSON          = "runtimes/manifest.json"
	baseWorkflow          = ".github/workflows/runtime-base.yml"
	versionsEnv           = "internal/infra/runtimeassets/assets/versions.env"
	canonicalSource       = "https://github.com/tasuku43/tobari"
	canonicalLicense      = "NOASSERTION"
	canonicalAgentLicense = "NOASSERTION"
	canonicalUser         = "tobari"
	canonicalRuntime      = "1"
	canonicalLife         = "sleep infinity"
	canonicalPackage      = "tobari/runtime"
)

var digestReference = regexp.MustCompile(`^debian@sha256:[0-9a-f]{64}$`)
var versionReference = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
var agentTagReference = regexp.MustCompile(`^claude\.[0-9]+\.[0-9]+\.[0-9]+-base\.[0-9]+\.[0-9]+\.[0-9]+-r[0-9]+$`)
var codexTagReference = regexp.MustCompile(`^codex\.[0-9]+\.[0-9]+\.[0-9]+-base\.[0-9]+\.[0-9]+\.[0-9]+-r[0-9]+$`)
var imageDigestReference = regexp.MustCompile(`^ghcr\.io/[^@\s]+@sha256:[0-9a-f]{64}$`)
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
		GH struct {
			Version string `json:"version"`
			Source  string `json:"source"`
		} `json:"gh"`
		AWSCLI struct {
			Version string `json:"version"`
			Source  string `json:"source"`
		} `json:"aws_cli"`
	} `json:"tools"`
}

type agentRuntimeMetadata struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Package       string `json:"package"`
	Kind          string `json:"kind"`
	Parent        string `json:"parent"`
	Agent         struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"agent"`
	Base struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"base"`
	Revision               int      `json:"revision"`
	RuntimeAPI             string   `json:"runtime_api"`
	RuntimeLifetimeCommand string   `json:"runtime_lifetime_command"`
	Entrypoint             []string `json:"entrypoint"`
	User                   string   `json:"user"`
	Architectures          []string `json:"architectures"`
	Tools                  []string `json:"tools"`
	Source                 string   `json:"source"`
	License                string   `json:"license"`
}

type agentRuntimeLock struct {
	SchemaVersion int `json:"schema_version"`
	Parent        struct {
		Package   string `json:"package"`
		Version   string `json:"version"`
		Reference string `json:"reference"`
	} `json:"parent"`
	Agent struct {
		Name          string `json:"name"`
		Version       string `json:"version"`
		Source        string `json:"source"`
		LicenseReview string `json:"license_review"`
		Platforms     map[string]struct {
			Asset  string `json:"asset"`
			SHA256 string `json:"sha256"`
			Size   int    `json:"size"`
		} `json:"platforms"`
	} `json:"agent"`
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
	claude := flag.Bool("claude", false, "validate the Claude agent image")
	printClaudeParent := flag.Bool("print-claude-parent", false, "print the validated Claude parent image reference")
	printClaudeVersion := flag.Bool("print-claude-version", false, "print the validated Claude agent version")
	codex := flag.Bool("codex", false, "validate the Codex agent image")
	printCodexParent := flag.Bool("print-codex-parent", false, "print the validated Codex parent image reference")
	printCodexVersion := flag.Bool("print-codex-version", false, "print the validated Codex agent version")
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
	if *claude || *printClaudeParent || *printClaudeVersion {
		parent, err := validateClaude(root)
		if err != nil {
			fatal(err)
		}
		if *printClaudeParent {
			fmt.Println(parent)
			return
		}
		if *printClaudeVersion {
			metadata, err := readJSON[agentRuntimeMetadata](root, claudeRuntimeJSON)
			if err != nil {
				fatal(err)
			}
			fmt.Println(metadata.Agent.Version)
			return
		}
		fmt.Println("runtimecheck: Claude OK")
		return
	}
	if *codex || *printCodexParent || *printCodexVersion {
		parent, err := validateCodex(root)
		if err != nil {
			fatal(err)
		}
		if *printCodexParent {
			fmt.Println(parent)
			return
		}
		if *printCodexVersion {
			metadata, err := readJSON[agentRuntimeMetadata](root, codexRuntimeJSON)
			if err != nil {
				fatal(err)
			}
			fmt.Println(metadata.Agent.Version)
			return
		}
		fmt.Println("runtimecheck: Codex OK")
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
	if lock.SchemaVersion != 1 || lock.BaseImage.Name != "debian" || lock.BaseImage.Tag == "" || !digestReference.MatchString(lock.BaseImage.Reference) {
		return "", errors.New("base runtime lock does not contain a digest-pinned Debian reference")
	}
	if lock.Tools.GH.Version != tobari.AgentReadyGitHubCLIVersion || lock.Tools.GH.Source != "https://github.com/cli/cli/releases" ||
		!versionReference.MatchString(lock.Tools.AWSCLI.Version) || lock.Tools.AWSCLI.Source != "https://awscli.amazonaws.com/" {
		return "", errors.New("base runtime lock does not contain the approved common CLI pins")
	}
	claudeLock, err := readJSON[agentRuntimeLock](root, claudeLockJSON)
	if err != nil {
		return "", err
	}
	codexLock, err := readJSON[agentRuntimeLock](root, codexLockJSON)
	if err != nil {
		return "", err
	}
	if claudeLock.Agent.Name != "claude-code" || claudeLock.Agent.Version != tobari.AgentReadyClaudeVersion || claudeLock.Agent.Source != "https://downloads.claude.ai/claude-code-releases" || !validLicenseReview(claudeLock.Agent.LicenseReview) {
		return "", errors.New("base runtime does not use the reviewed Claude artifact lock")
	}
	if codexLock.Agent.Name != "codex-cli" || codexLock.Agent.Version != tobari.AgentReadyCodexVersion || codexLock.Agent.Source != "https://releases.openai.com/codex" || !validLicenseReview(codexLock.Agent.LicenseReview) {
		return "", errors.New("base runtime does not use the reviewed Codex artifact lock")
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
	if rootManifest.SchemaVersion != 1 || len(rootManifest.Images) == 0 {
		return "", errors.New("runtime manifest must contain the published base image")
	}
	baseFound := false
	for _, image := range rootManifest.Images {
		if image.Name != "base" {
			continue
		}
		if baseFound || image.Path != "base" || image.Package != canonicalPackage || image.Parent != nil {
			return "", errors.New("runtime manifest has an invalid base image entry")
		}
		baseFound = true
	}
	if !baseFound {
		return "", errors.New("runtime manifest is missing the base image entry")
	}

	for _, relative := range []string{baseDockerfile, baseEntrypoint, baseAWSKey} {
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
		"FROM ${DEBIAN_IMAGE} AS fetcher",
		"ARG GH_VERSION=" + lock.Tools.GH.Version,
		"ARG AWS_CLI_VERSION=" + lock.Tools.AWSCLI.Version,
		"ARG CLAUDE_CODE_VERSION=" + claudeLock.Agent.Version,
		"ARG CLAUDE_CODE_SHA256_X64=" + claudeLock.Agent.Platforms["linux/amd64"].SHA256,
		"ARG CLAUDE_CODE_SHA256_ARM64=" + claudeLock.Agent.Platforms["linux/arm64"].SHA256,
		"ARG CODEX_VERSION=" + codexLock.Agent.Version,
		"ARG CODEX_PACKAGE_SHA256_X64=" + codexLock.Agent.Platforms["linux/amd64"].SHA256,
		"ARG CODEX_PACKAGE_SHA256_ARM64=" + codexLock.Agent.Platforms["linux/arm64"].SHA256,
		"COPY aws-cli-public-key.asc /tmp/aws-cli-public-key.asc",
		"io.tobari.runtime-api=\"1\"",
		"io.tobari.runtime-lifetime-command=\"sleep infinity\"",
		"org.opencontainers.image.licenses=\"" + canonicalLicense + "\"",
		"org.opencontainers.image.source=\"" + canonicalSource + "\"",
		"COPY --from=fetcher /opt/aws-cli /opt/aws-cli",
		"COPY --from=fetcher /out/gh /usr/local/bin/gh",
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
	for _, forbidden := range []string{"ENV CODEX_HOME=", "/var/lib/tobari/.local/bin/claude", "/var/lib/tobari/.codex/packages", "claude update", "codex update"} {
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

	return lock.BaseImage.Reference, nil
}

func validLicenseReview(status string) bool {
	return status == "pending" || status == "approved"
}

func validateClaude(root string) (string, error) {
	if _, err := validate(root); err != nil {
		return "", fmt.Errorf("validate base for Claude: %w", err)
	}
	metadata, err := readJSON[agentRuntimeMetadata](root, claudeRuntimeJSON)
	if err != nil {
		return "", err
	}
	if metadata.SchemaVersion != 1 || metadata.Name != "claude" || metadata.Package != canonicalPackage || metadata.Kind != "agent" || !agentTagReference.MatchString(metadata.Version) {
		return "", errors.New("Claude runtime metadata has an invalid identity")
	}
	if metadata.Parent == "" || metadata.Agent.Name != "claude-code" || !versionReference.MatchString(metadata.Agent.Version) ||
		metadata.Base.Name != "base" || !versionReference.MatchString(metadata.Base.Version) || metadata.Revision != 1 {
		return "", errors.New("Claude runtime metadata has an invalid composition")
	}
	expectedTag := fmt.Sprintf("claude.%s-base.%s-r%d", metadata.Agent.Version, metadata.Base.Version, metadata.Revision)
	if metadata.Version != expectedTag {
		return "", errors.New("Claude runtime metadata tag does not match its composition")
	}
	if metadata.RuntimeAPI != canonicalRuntime || metadata.RuntimeLifetimeCommand != canonicalLife || metadata.User != canonicalUser {
		return "", errors.New("Claude runtime metadata does not preserve the Tobari runtime contract")
	}
	if !sameStrings(metadata.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		return "", errors.New("Claude runtime metadata has an unexpected entrypoint")
	}
	if !sameStrings(metadata.Architectures, []string{"linux/amd64", "linux/arm64"}) || !sameStrings(metadata.Tools, []string{"claude"}) {
		return "", errors.New("Claude runtime metadata has an unexpected architecture or tool set")
	}
	if metadata.Source != canonicalSource || metadata.License != canonicalAgentLicense {
		return "", errors.New("Claude runtime metadata has an invalid public source or license")
	}

	lock, err := readJSON[agentRuntimeLock](root, claudeLockJSON)
	if err != nil {
		return "", err
	}
	if lock.SchemaVersion != 1 || lock.Parent.Package != canonicalPackage || lock.Parent.Version != metadata.Base.Version || !imageDigestReference.MatchString(lock.Parent.Reference) || lock.Parent.Reference != metadata.Parent {
		return "", errors.New("Claude runtime lock does not contain the pinned Tobari parent")
	}
	if lock.Agent.Name != metadata.Agent.Name || lock.Agent.Version != metadata.Agent.Version || lock.Agent.Source != "https://downloads.claude.ai/claude-code-releases" || !validLicenseReview(lock.Agent.LicenseReview) {
		return "", errors.New("Claude runtime lock does not contain the reviewed Claude source metadata")
	}
	expectedPlatforms := map[string]string{
		"linux/amd64": "linux-x64/claude",
		"linux/arm64": "linux-arm64/claude",
	}
	if len(lock.Agent.Platforms) != len(expectedPlatforms) {
		return "", errors.New("Claude runtime lock has an unexpected architecture matrix")
	}
	for platform, asset := range expectedPlatforms {
		entry, ok := lock.Agent.Platforms[platform]
		if !ok || entry.Asset != asset || !checksumReference.MatchString(entry.SHA256) || entry.Size <= 0 {
			return "", fmt.Errorf("Claude runtime lock has an invalid %s artifact", platform)
		}
	}

	rootManifest, err := readJSON[manifest](root, manifestJSON)
	if err != nil {
		return "", err
	}
	claudeFound := false
	for _, image := range rootManifest.Images {
		if image.Name != "claude" {
			continue
		}
		if claudeFound || image.Path != "claude" || image.Package != canonicalPackage || image.Parent == nil || *image.Parent != "base" {
			return "", errors.New("runtime manifest has an invalid Claude image entry")
		}
		claudeFound = true
	}
	if !claudeFound {
		return "", errors.New("runtime manifest is missing the Claude image entry")
	}

	dockerfile, err := readRegularFile(filepath.Join(root, claudeDockerfile))
	if err != nil {
		return "", err
	}
	spec := string(dockerfile)
	if !strings.Contains(spec, "ARG BASE_IMAGE="+lock.Parent.Reference) {
		return "", errors.New("Claude Dockerfile default does not match the parent digest lock")
	}
	platforms := lock.Agent.Platforms
	for _, required := range []string{
		"FROM ${BASE_IMAGE}",
		"ARG CLAUDE_CODE_VERSION=" + lock.Agent.Version,
		"ARG CLAUDE_BASE_VERSION=" + lock.Parent.Version,
		"ARG CLAUDE_CODE_SHA256_X64=" + platforms["linux/amd64"].SHA256,
		"ARG CLAUDE_CODE_SHA256_ARM64=" + platforms["linux/arm64"].SHA256,
		"arm64) platform=linux-arm64;",
		"amd64) platform=linux-x64;",
		"https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/manifest.json",
		"https://downloads.claude.ai/claude-code-releases/${CLAUDE_CODE_VERSION}/${platform}/claude",
		"jq -er",
		"sha256sum --check --strict",
		"install -m 0755 /tmp/claude-code/claude /usr/local/bin/claude",
		"io.tobari.runtime-api=\"1\"",
		"io.tobari.runtime-lifetime-command=\"sleep infinity\"",
		"org.opencontainers.image.licenses=\"" + canonicalAgentLicense + "\"",
		"ENV HOME=/var/lib/tobari",
		"ENV PATH=\"/usr/local/bin:${PATH}\"",
		"ENV DISABLE_AUTOUPDATER=1",
		"USER tobari",
		"RUN claude --version",
	} {
		if !strings.Contains(spec, required) {
			return "", fmt.Errorf("Claude Dockerfile is missing %q", required)
		}
	}
	if strings.Contains(spec, "/var/lib/tobari/.local/bin") {
		return "", errors.New("Claude Dockerfile must keep the agent executable outside the Tobari home")
	}
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CMD ") || strings.HasPrefix(trimmed, "ENTRYPOINT ") {
			return "", errors.New("Claude Dockerfile must inherit the Tobari command and entrypoint")
		}
	}

	return lock.Parent.Reference, nil
}

func validateCodex(root string) (string, error) {
	if _, err := validate(root); err != nil {
		return "", fmt.Errorf("validate base for Codex: %w", err)
	}
	metadata, err := readJSON[agentRuntimeMetadata](root, codexRuntimeJSON)
	if err != nil {
		return "", err
	}
	if metadata.SchemaVersion != 1 || metadata.Name != "codex" || metadata.Package != canonicalPackage || metadata.Kind != "agent" || !codexTagReference.MatchString(metadata.Version) {
		return "", errors.New("Codex runtime metadata has an invalid identity")
	}
	if metadata.Parent == "" || metadata.Agent.Name != "codex-cli" || !versionReference.MatchString(metadata.Agent.Version) ||
		metadata.Base.Name != "base" || !versionReference.MatchString(metadata.Base.Version) || metadata.Revision != 1 {
		return "", errors.New("Codex runtime metadata has an invalid composition")
	}
	expectedTag := fmt.Sprintf("codex.%s-base.%s-r%d", metadata.Agent.Version, metadata.Base.Version, metadata.Revision)
	if metadata.Version != expectedTag {
		return "", errors.New("Codex runtime metadata tag does not match its composition")
	}
	if metadata.RuntimeAPI != canonicalRuntime || metadata.RuntimeLifetimeCommand != canonicalLife || metadata.User != canonicalUser {
		return "", errors.New("Codex runtime metadata does not preserve the Tobari runtime contract")
	}
	if !sameStrings(metadata.Entrypoint, []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}) {
		return "", errors.New("Codex runtime metadata has an unexpected entrypoint")
	}
	if !sameStrings(metadata.Architectures, []string{"linux/amd64", "linux/arm64"}) || !sameStrings(metadata.Tools, []string{"codex"}) {
		return "", errors.New("Codex runtime metadata has an unexpected architecture or tool set")
	}
	if metadata.Source != canonicalSource || metadata.License != canonicalAgentLicense {
		return "", errors.New("Codex runtime metadata has an invalid public source or license")
	}

	lock, err := readJSON[agentRuntimeLock](root, codexLockJSON)
	if err != nil {
		return "", err
	}
	if lock.SchemaVersion != 1 || lock.Parent.Package != canonicalPackage || lock.Parent.Version != metadata.Base.Version || !imageDigestReference.MatchString(lock.Parent.Reference) || lock.Parent.Reference != metadata.Parent {
		return "", errors.New("Codex runtime lock does not contain the pinned Tobari parent")
	}
	if lock.Agent.Name != metadata.Agent.Name || lock.Agent.Version != metadata.Agent.Version || lock.Agent.Source != "https://releases.openai.com/codex" || !validLicenseReview(lock.Agent.LicenseReview) {
		return "", errors.New("Codex runtime lock does not contain the reviewed Codex source metadata")
	}
	expectedPlatforms := map[string]string{
		"linux/amd64": "codex-package-x86_64-unknown-linux-musl.tar.gz",
		"linux/arm64": "codex-package-aarch64-unknown-linux-musl.tar.gz",
	}
	if len(lock.Agent.Platforms) != len(expectedPlatforms) {
		return "", errors.New("Codex runtime lock has an unexpected architecture matrix")
	}
	for platform, asset := range expectedPlatforms {
		entry, ok := lock.Agent.Platforms[platform]
		if !ok || entry.Asset != asset || !checksumReference.MatchString(entry.SHA256) || entry.Size <= 0 {
			return "", fmt.Errorf("Codex runtime lock has an invalid %s artifact", platform)
		}
	}

	rootManifest, err := readJSON[manifest](root, manifestJSON)
	if err != nil {
		return "", err
	}
	codexFound := false
	for _, image := range rootManifest.Images {
		if image.Name != "codex" {
			continue
		}
		if codexFound || image.Path != "codex" || image.Package != canonicalPackage || image.Parent == nil || *image.Parent != "base" {
			return "", errors.New("runtime manifest has an invalid Codex image entry")
		}
		codexFound = true
	}
	if !codexFound {
		return "", errors.New("runtime manifest is missing the Codex image entry")
	}

	dockerfile, err := readRegularFile(filepath.Join(root, codexDockerfile))
	if err != nil {
		return "", err
	}
	spec := string(dockerfile)
	if !strings.Contains(spec, "ARG BASE_IMAGE="+lock.Parent.Reference) {
		return "", errors.New("Codex Dockerfile default does not match the parent digest lock")
	}
	platforms := lock.Agent.Platforms
	for _, required := range []string{
		"FROM ${BASE_IMAGE}",
		"ARG CODEX_VERSION=" + lock.Agent.Version,
		"ARG CODEX_BASE_VERSION=" + lock.Parent.Version,
		"ARG CODEX_PACKAGE_SHA256_X64=" + platforms["linux/amd64"].SHA256,
		"ARG CODEX_PACKAGE_SHA256_ARM64=" + platforms["linux/arm64"].SHA256,
		"arm64) target=aarch64-unknown-linux-musl; package=\"codex-package-aarch64-unknown-linux-musl.tar.gz\";",
		"amd64) target=x86_64-unknown-linux-musl; package=\"codex-package-x86_64-unknown-linux-musl.tar.gz\";",
		"https://releases.openai.com/codex/releases/${CODEX_VERSION}/${package}",
		"sha256sum --check --strict",
		"tar -xzf",
		"jq -er '.version' \"${release_dir}/codex-package.json\"",
		"jq -er '.target' \"${release_dir}/codex-package.json\"",
		"jq -er '.entrypoint' \"${release_dir}/codex-package.json\"",
		"io.tobari.runtime-api=\"1\"",
		"io.tobari.runtime-lifetime-command=\"sleep infinity\"",
		"org.opencontainers.image.licenses=\"" + canonicalAgentLicense + "\"",
		"ENV HOME=/var/lib/tobari",
		"ENV CODEX_HOME=/var/lib/tobari/.codex",
		"ENV PATH=\"/usr/local/bin:${PATH}\"",
		"release_dir=\"/opt/tobari/codex/${CODEX_VERSION}-${target}\"",
		"test -x \"${release_dir}/bin/codex\"",
		"test -x \"${release_dir}/bin/codex-code-mode-host\"",
		"test -x \"${release_dir}/codex-path/rg\"",
		"test -x \"${release_dir}/codex-resources/bwrap\"",
		"test -x \"${release_dir}/codex-resources/zsh/bin/zsh\"",
		"ln -s \"${release_dir}/bin/codex\" /usr/local/bin/codex",
		"ln -s \"${release_dir}/bin/codex-code-mode-host\" /usr/local/bin/codex-code-mode-host",
		"ln -s \"${release_dir}/codex-path/rg\" /usr/local/bin/rg",
		"USER tobari",
		"RUN codex --version",
	} {
		if !strings.Contains(spec, required) {
			return "", fmt.Errorf("Codex Dockerfile is missing %q", required)
		}
	}
	if strings.Contains(spec, "/var/lib/tobari/.local/bin") || strings.Contains(spec, "/var/lib/tobari/.codex/packages") {
		return "", errors.New("Codex Dockerfile must keep the standalone package outside the Tobari home")
	}
	for _, line := range strings.Split(spec, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CMD ") || strings.HasPrefix(trimmed, "ENTRYPOINT ") {
			return "", errors.New("Codex Dockerfile must inherit the Tobari command and entrypoint")
		}
	}

	return lock.Parent.Reference, nil
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
