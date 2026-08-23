// Command sitegen derives public documentation data from Tobari's executable
// contracts and reviewed source files. It is read-only unless --write is used.
package main

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const generatedDirectory = "docs/architecture-site/src/generated"
const sourceSnapshotFile = "docs/architecture-site/source-snapshot.txt"

const agentHelpTimeout = 30 * time.Second
const maximumSourceArchiveBytes = 128 << 20
const maximumArchivedFileBytes = 32 << 20

type agentHelpRunner func(args ...string) (json.RawMessage, error)

type rootHelp struct {
	SchemaVersion int `json:"schema_version"`
	Commands      []struct {
		Path      string `json:"path"`
		Namespace string `json:"namespace"`
	} `json:"commands"`
}

type catalogDocument struct {
	GeneratedFrom string            `json:"generated_from"`
	Root          json.RawMessage   `json:"root"`
	Scopes        []json.RawMessage `json:"scopes"`
	CommandCount  int               `json:"command_count"`
	Faults        []faultOccurrence `json:"faults"`
}

type faultOccurrence struct {
	Code        string       `json:"code"`
	Kind        string       `json:"kind"`
	Phase       string       `json:"phase,omitempty"`
	ChangeState string       `json:"change_state,omitempty"`
	Retryable   bool         `json:"retryable"`
	Command     string       `json:"command"`
	NextActions []nextAction `json:"next_actions"`
}

type nextAction struct {
	Command string `json:"command"`
	Reason  string `json:"reason"`
}

type scopeHelp struct {
	Commands []struct {
		Path     string `json:"path"`
		Contract struct {
			Errors []struct {
				Code        string       `json:"code"`
				Kind        string       `json:"kind"`
				Phase       string       `json:"phase"`
				ChangeState string       `json:"change_state"`
				Retryable   bool         `json:"retryable"`
				NextActions []nextAction `json:"next_actions"`
			} `json:"errors"`
		} `json:"contract"`
	} `json:"commands"`
	ErrorContract struct {
		GlobalErrors []struct {
			Code        string       `json:"code"`
			Kind        string       `json:"kind"`
			Phase       string       `json:"phase"`
			ChangeState string       `json:"change_state"`
			Retryable   bool         `json:"retryable"`
			NextActions []nextAction `json:"next_actions"`
		} `json:"global_errors"`
	} `json:"error_contract"`
}

type componentVersionDocument struct {
	GeneratedFrom string                 `json:"generated_from"`
	Components    []componentVersion     `json:"components"`
	Schemas       []schemaVersion        `json:"schemas"`
	Runtime       runtimeVersionContract `json:"runtime"`
}

type componentVersion struct {
	Component string `json:"component"`
	Version   string `json:"version"`
	Identity  string `json:"identity"`
	Authority string `json:"authority"`
	Note      string `json:"note,omitempty"`
}

type schemaVersion struct {
	Contract  string `json:"contract"`
	Version   int    `json:"version"`
	Authority string `json:"authority"`
}

type runtimeVersionContract struct {
	DefaultSelector string   `json:"default_selector"`
	RuntimeAPI      string   `json:"runtime_api"`
	LifetimeCommand string   `json:"lifetime_command"`
	MetadataVersion string   `json:"metadata_version"`
	Architectures   []string `json:"architectures"`
	LocalBuild      bool     `json:"local_build"`
	MovingSelector  bool     `json:"moving_selector"`
}

func main() {
	var write, check bool
	var sourceRef string
	flag.BoolVar(&write, "write", false, "write generated documentation data")
	flag.BoolVar(&check, "check", false, "fail if generated documentation data is stale")
	flag.StringVar(&sourceRef, "source-ref", "", "override the pinned committed source ref used for public documentation evidence")
	flag.Parse()
	if write == check {
		fmt.Fprintln(os.Stderr, "sitegen: choose exactly one of --write or --check")
		os.Exit(2)
	}

	root, err := repositoryRoot()
	if err != nil {
		fatal(err)
	}
	if sourceRef == "" {
		sourceRef, err = pinnedSourceRef(root)
		if err != nil {
			fatal(err)
		}
	}
	outputs, err := generate(root, sourceRef)
	if err != nil {
		fatal(err)
	}
	for name, content := range outputs {
		path := filepath.Join(root, generatedDirectory, name)
		if write {
			// #nosec G301 -- generated documentation is committed public repository content.
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				fatal(err)
			}
			// #nosec G306 -- generated documentation is intentionally world-readable public content.
			if err := os.WriteFile(path, content, 0o644); err != nil {
				fatal(err)
			}
			continue
		}
		// #nosec G304 -- root is Git-derived and name comes only from the fixed generator output map.
		existing, err := os.ReadFile(path)
		if err != nil {
			fatal(fmt.Errorf("generated file %s is missing: %w", filepath.ToSlash(path), err))
		}
		if !bytes.Equal(existing, content) {
			fatal(fmt.Errorf("generated file %s is stale; run task site:generate", filepath.ToSlash(path)))
		}
	}
}

func pinnedSourceRef(root string) (string, error) {
	// #nosec G304 -- root is resolved by Git and sourceSnapshotFile is a fixed repository path.
	content, err := os.ReadFile(filepath.Join(root, sourceSnapshotFile))
	if err != nil {
		return "", fmt.Errorf("read public documentation source snapshot: %w", err)
	}
	ref := strings.TrimSpace(string(content))
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref) {
		return "", errors.New("public documentation source snapshot must be one full lowercase commit SHA")
	}
	// #nosec G204 -- ref is restricted above to one lowercase full commit SHA and is passed without a shell.
	command := exec.Command("git", "cat-file", "-e", ref+"^{commit}")
	command.Dir = root
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("public documentation source snapshot %s is not a local commit: %w", ref, err)
	}
	return ref, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sitegen:", err)
	os.Exit(1)
}

func repositoryRoot() (string, error) {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func generate(root, sourceRef string) (map[string][]byte, error) {
	if !validSourceRef(sourceRef) {
		return nil, errors.New("source ref must be HEAD or one full lowercase commit SHA")
	}
	runHelp, cleanup, err := buildCommittedAgentHelpRunner(root, sourceRef)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return generateWithAgentHelp(root, sourceRef, runHelp)
}

func validSourceRef(sourceRef string) bool {
	return sourceRef == "HEAD" || regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(sourceRef)
}

func generateWithAgentHelp(root, sourceRef string, runHelp agentHelpRunner) (map[string][]byte, error) {
	catalog, err := generateCatalog(runHelp)
	if err != nil {
		return nil, err
	}
	catalog.GeneratedFrom = fmt.Sprintf("%s at commit %s", catalog.GeneratedFrom, sourceRef)
	versions, err := generateVersions(root, sourceRef, catalog)
	if err != nil {
		return nil, err
	}
	catalogJSON, err := canonicalJSON(catalog)
	if err != nil {
		return nil, err
	}
	versionJSON, err := canonicalJSON(versions)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		"catalog.json":            catalogJSON,
		"component-versions.json": versionJSON,
	}, nil
}

func canonicalJSON(value any) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func generateCatalog(runHelp agentHelpRunner) (catalogDocument, error) {
	if runHelp == nil {
		return catalogDocument{}, errors.New("agent help runner is required")
	}
	rootRaw, err := runHelp()
	if err != nil {
		return catalogDocument{}, err
	}
	var root rootHelp
	if err := json.Unmarshal(rootRaw, &root); err != nil {
		return catalogDocument{}, fmt.Errorf("decode root agent help: %w", err)
	}
	if root.SchemaVersion <= 0 || len(root.Commands) == 0 {
		return catalogDocument{}, errors.New("root agent help is empty")
	}

	seenNamespaces := map[string]bool{}
	seenCommands := map[string]int{}
	seenGlobalFaults := map[string]bool{}
	var scopes []json.RawMessage
	var faults []faultOccurrence
	for _, command := range root.Commands {
		if command.Path == "" || command.Namespace == "" {
			return catalogDocument{}, errors.New("root agent help contains an incomplete command")
		}
		if seenNamespaces[command.Namespace] {
			continue
		}
		seenNamespaces[command.Namespace] = true
		raw, err := runHelp(command.Namespace)
		if err != nil {
			return catalogDocument{}, err
		}
		var scope scopeHelp
		if err := json.Unmarshal(raw, &scope); err != nil {
			return catalogDocument{}, fmt.Errorf("decode %s agent help: %w", command.Namespace, err)
		}
		for _, scopedCommand := range scope.Commands {
			seenCommands[scopedCommand.Path]++
			for _, declared := range scopedCommand.Contract.Errors {
				faults = append(faults, faultOccurrence{
					Code: declared.Code, Kind: declared.Kind, Phase: declared.Phase, ChangeState: declared.ChangeState, Retryable: declared.Retryable,
					Command: scopedCommand.Path, NextActions: declared.NextActions,
				})
			}
		}
		for _, declared := range scope.ErrorContract.GlobalErrors {
			if seenGlobalFaults[declared.Code] {
				continue
			}
			seenGlobalFaults[declared.Code] = true
			faults = append(faults, faultOccurrence{
				Code: declared.Code, Kind: declared.Kind, Phase: declared.Phase, ChangeState: declared.ChangeState, Retryable: declared.Retryable,
				Command: "(global)", NextActions: declared.NextActions,
			})
		}
		scopes = append(scopes, raw)
	}
	for _, command := range root.Commands {
		if count := seenCommands[command.Path]; count != 1 {
			return catalogDocument{}, fmt.Errorf("command %q appears %d times in scoped help; want exactly one", command.Path, count)
		}
	}
	sort.SliceStable(faults, func(i, j int) bool {
		if faults[i].Code == faults[j].Code {
			return faults[i].Command < faults[j].Command
		}
		return faults[i].Code < faults[j].Code
	})
	return catalogDocument{
		GeneratedFrom: fmt.Sprintf("committed tobari agent help schema %d", root.SchemaVersion),
		Root:          rootRaw, Scopes: scopes, CommandCount: len(root.Commands), Faults: faults,
	}, nil
}

func buildCommittedAgentHelpRunner(root, sourceRef string) (agentHelpRunner, func(), error) {
	temporaryRoot, err := os.MkdirTemp("", "tobari-sitegen-")
	if err != nil {
		return nil, nil, fmt.Errorf("create committed CLI build directory: %w", err)
	}
	cleanup := func() {
		_ = os.RemoveAll(temporaryRoot)
	}
	sourceRoot := filepath.Join(temporaryRoot, "source")
	if err := os.Mkdir(sourceRoot, 0o700); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create committed CLI source directory: %w", err)
	}
	if err := exportCommittedTree(root, sourceRef, sourceRoot); err != nil {
		cleanup()
		return nil, nil, err
	}

	binaryPath := filepath.Join(temporaryRoot, "tobari")
	// #nosec G204 -- executable and build arguments are fixed; binaryPath is below a private MkdirTemp directory.
	command := exec.Command("go", "build", "-mod=readonly", "-trimpath", "-o", binaryPath, "./cmd/tobari")
	command.Dir = sourceRoot
	command.Env = environmentWithOverride(os.Environ(), "GOWORK", "off")
	if output, err := command.CombinedOutput(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf(
			"build Tobari CLI at %s: %w: %s",
			sourceRef,
			err,
			strings.TrimSpace(string(output)),
		)
	}
	return binaryAgentHelpRunner(binaryPath), cleanup, nil
}

func exportCommittedTree(root, sourceRef, destination string) error {
	// #nosec G204 -- sourceRef is a Git object argument after --, never shell text.
	command := exec.Command("git", "archive", "--format=tar", sourceRef)
	command.Dir = root
	archive, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("archive source at %s: %s", sourceRef, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return fmt.Errorf("archive source at %s: %w", sourceRef, err)
	}
	if len(archive) > maximumSourceArchiveBytes {
		return fmt.Errorf("source archive at %s exceeds %d bytes", sourceRef, maximumSourceArchiveBytes)
	}

	reader := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read source archive at %s: %w", sourceRef, err)
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("source archive at %s contains unsafe path %q", sourceRef, header.Name)
		}
		target := filepath.Join(destination, name)
		mode := header.FileInfo().Mode().Perm()
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("create archived directory %s: %w", name, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maximumArchivedFileBytes {
				return fmt.Errorf("archived file %s has unsupported size %d", name, header.Size)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return fmt.Errorf("create parent for archived file %s: %w", name, err)
			}
			// #nosec G304 -- name is local, traversal-checked, and extracted below a fresh private directory.
			file, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return fmt.Errorf("create archived file %s: %w", name, err)
			}
			_, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("extract archived file %s: %w", name, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close archived file %s: %w", name, closeErr)
			}
		default:
			return fmt.Errorf("source archive at %s contains unsupported entry %q", sourceRef, header.Name)
		}
	}
}

func environmentWithOverride(environment []string, name, value string) []string {
	prefix := name + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			result = append(result, entry)
		}
	}
	return append(result, prefix+value)
}

func binaryAgentHelpRunner(binaryPath string) agentHelpRunner {
	return func(args ...string) (json.RawMessage, error) {
		fullArgs := append([]string{"help"}, args...)
		fullArgs = append(fullArgs, "--format", "agent")
		ctx, cancel := context.WithTimeout(context.Background(), agentHelpTimeout)
		defer cancel()
		var out, errOut bytes.Buffer
		// #nosec G204 -- binaryPath is the just-built private temp binary; args come from its own bounded Catalog help.
		command := exec.CommandContext(ctx, binaryPath, fullArgs...)
		command.Stdin = strings.NewReader("")
		command.Stdout = &out
		command.Stderr = &errOut
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("run %q: %w", strings.Join(fullArgs, " "), ctx.Err())
			}
			return nil, fmt.Errorf("run %q: %w: %s", strings.Join(fullArgs, " "), err, strings.TrimSpace(errOut.String()))
		}
		content := bytes.TrimSpace(out.Bytes())
		if !json.Valid(content) {
			return nil, errors.New("CLI agent help returned invalid JSON")
		}
		return append(json.RawMessage(nil), content...), nil
	}
}

func committedFile(root, sourceRef, path string) ([]byte, error) {
	if !validSourceRef(sourceRef) {
		return nil, errors.New("source ref must be HEAD or one full lowercase commit SHA")
	}
	// #nosec G204 -- sourceRef/path are separate Git object arguments, never shell text; every path caller is fixed.
	command := exec.Command("git", "show", sourceRef+":"+path)
	command.Dir = root
	output, err := command.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("read %s at %s: %s", path, sourceRef, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("read %s at %s: %w", path, sourceRef, err)
	}
	return output, nil
}

func requiredMatch(content []byte, expression, label string) (string, error) {
	match := regexp.MustCompile(expression).FindSubmatch(content)
	if len(match) != 2 {
		return "", fmt.Errorf("cannot derive %s from committed source", label)
	}
	return string(match[1]), nil
}

func requiredInt(content []byte, expression, label string) (int, error) {
	value, err := requiredMatch(content, expression, label)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", label, err)
	}
	return parsed, nil
}

func contextReportSchemaVersion(runtimeCatalogSource []byte) (int, error) {
	return requiredInt(
		runtimeCatalogSource,
		`JSONEnvelope:\s*"(?:workspace_manifest|context)",[^\r\n]*JSONSchemaVersion:\s*([0-9]+)`,
		"public Workspace Manifest report schema",
	)
}

func envValues(content []byte) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && name != "" {
			values[name] = value
		}
	}
	return values
}

func generateVersions(root, sourceRef string, catalog catalogDocument) (componentVersionDocument, error) {
	goMod, err := committedFile(root, sourceRef, "go.mod")
	if err != nil {
		return componentVersionDocument{}, err
	}
	goVersion, err := requiredMatch(goMod, `(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)$`, "Go version")
	if err != nil {
		return componentVersionDocument{}, err
	}
	versionsFile, err := committedFile(root, sourceRef, "internal/infra/runtimeassets/assets/versions.env")
	if err != nil {
		return componentVersionDocument{}, err
	}
	values := envValues(versionsFile)
	for _, key := range []string{"MITMPROXY_VERSION", "MITMPROXY_IMAGE", "OPA_VERSION", "OPA_IMAGE", "DEBIAN_VERSION", "DEBIAN_IMAGE"} {
		if values[key] == "" {
			return componentVersionDocument{}, fmt.Errorf("versions.env does not define %s", key)
		}
	}
	contextSource, err := committedFile(root, sourceRef, "internal/domain/tobari/context.go")
	if err != nil {
		return componentVersionDocument{}, err
	}
	contextSchema, err := requiredInt(contextSource, `(?:Manifest|Context)SchemaVersion\s*=\s*([0-9]+)`, "Workspace Manifest schema")
	if err != nil {
		return componentVersionDocument{}, err
	}
	defaultSelector, err := requiredMatch(contextSource, `OfficialRuntimeBase\s*=\s*"([^"]+)"`, "default runtime selector")
	if err != nil {
		return componentVersionDocument{}, err
	}
	tobariSource, err := committedFile(root, sourceRef, "internal/domain/tobari/tobari.go")
	if err != nil {
		return componentVersionDocument{}, err
	}
	runtimeAPI, err := requiredMatch(tobariSource, `RuntimeImageAPI\s*=\s*"([^"]+)"`, "runtime API")
	if err != nil {
		return componentVersionDocument{}, err
	}
	lifetime, err := requiredMatch(tobariSource, `RuntimeImageLifetimeCommand\s*=\s*"([^"]+)"`, "runtime lifetime command")
	if err != nil {
		return componentVersionDocument{}, err
	}
	projectSource, err := committedFile(root, sourceRef, "internal/domain/tobari/project.go")
	if err != nil {
		return componentVersionDocument{}, err
	}
	projectSchema, err := requiredInt(projectSource, `(?:Workspace|Project)StateSchemaVersion\s*=\s*([0-9]+)`, "Workspace state schema")
	if err != nil {
		return componentVersionDocument{}, err
	}
	gatewaySource, err := committedFile(root, sourceRef, "gateway/addon/tobari_gateway.py")
	if err != nil {
		return componentVersionDocument{}, err
	}
	gatewayOPAInputSchema, err := requiredInt(
		gatewaySource,
		`policy_input\s*=\s*\{\s*"schema_version":\s*([0-9]+),`,
		"Gateway OPA input schema",
	)
	if err != nil {
		return componentVersionDocument{}, err
	}
	gatewayDockerfile, err := committedFile(root, sourceRef, "gateway/Dockerfile")
	if err != nil {
		return componentVersionDocument{}, err
	}
	gatewayImageAPI, err := requiredInt(gatewayDockerfile, `io\.tobari\.gateway-api="([0-9]+)"`, "Gateway image API")
	if err != nil {
		return componentVersionDocument{}, err
	}
	runtimeCatalogSource, err := committedFile(root, sourceRef, "internal/cli/runtime_catalog.go")
	if err != nil {
		return componentVersionDocument{}, err
	}
	contextReportSchema, err := contextReportSchemaVersion(runtimeCatalogSource)
	if err != nil {
		return componentVersionDocument{}, err
	}
	principalSource, err := committedFile(root, sourceRef, "internal/infra/dockerruntime/principal_registry.go")
	if err != nil {
		return componentVersionDocument{}, err
	}
	principalSchema, err := requiredInt(principalSource, `projectPrincipalRegistrySchema\s*=\s*([0-9]+)`, "principal registry schema")
	if err != nil {
		return componentVersionDocument{}, err
	}
	runtimeMetadata, err := committedFile(root, sourceRef, "runtimes/base/runtime.json")
	if err != nil {
		return componentVersionDocument{}, err
	}
	var metadata struct {
		Version       string   `json:"version"`
		RuntimeAPI    string   `json:"runtime_api"`
		Architectures []string `json:"architectures"`
	}
	if err := json.Unmarshal(runtimeMetadata, &metadata); err != nil {
		return componentVersionDocument{}, fmt.Errorf("decode runtime metadata: %w", err)
	}
	gatewayVersion := "embedded-source V1"
	gatewayIdentity := "source-derived local image"
	gatewayAuthority := "internal/infra/runtimeassets/assets/gateway"
	// Historical documentation snapshots may predate ADR 0033. Preserve what
	// that committed source actually selected rather than rewriting history.
	if values["GATEWAY_IMAGE"] != "" {
		gatewayIdentity = values["GATEWAY_IMAGE"]
		gatewayAuthority = "internal/infra/runtimeassets/assets/versions.env"
		gatewayVersion = "digest identity"
		if gatewayIdentity == "unpublished" {
			gatewayVersion = "unpublished V1 snapshot"
		}
	}
	localBuild := defaultSelector == "tobari-runtime:base"
	return componentVersionDocument{
		GeneratedFrom: "committed repository authorities and executable CLI help at " + sourceRef,
		Components: []componentVersion{
			{Component: "Go", Version: goVersion, Identity: "go " + goVersion, Authority: "go.mod"},
			{Component: "OPA", Version: values["OPA_VERSION"], Identity: values["OPA_IMAGE"], Authority: "internal/infra/runtimeassets/assets/versions.env", Note: "Version is declared; runtime identity is the immutable digest."},
			{Component: "mitmproxy", Version: values["MITMPROXY_VERSION"], Identity: values["MITMPROXY_IMAGE"], Authority: "internal/infra/runtimeassets/assets/versions.env", Note: "Version is declared; build base identity is the immutable digest."},
			{Component: "Gateway", Version: gatewayVersion, Identity: gatewayIdentity, Authority: gatewayAuthority},
			{Component: "Base runtime build image", Version: values["DEBIAN_VERSION"], Identity: values["DEBIAN_IMAGE"], Authority: "internal/infra/runtimeassets/assets/versions.env"},
			{Component: "Tobari CLI", Version: "dev unless release-injected", Identity: "documentation commit supplies source identity", Authority: "cmd/tobari/main.go and scripts/package-release.sh", Note: "The repository does not invent a release version for ordinary builds."},
		},
		Schemas: []schemaVersion{
			{Contract: "Agent help", Version: helpSchemaVersion(catalog), Authority: "internal/cli/help.go"},
			{Contract: "Workspace Manifest", Version: contextSchema, Authority: "internal/domain/tobari/context.go"},
			{Contract: "Public Workspace Manifest report", Version: contextReportSchema, Authority: "internal/cli/runtime_catalog.go"},
			{Contract: "Root index and Workspace instance", Version: projectSchema, Authority: "internal/domain/tobari/project.go"},
			{Contract: "Project principal registry", Version: principalSchema, Authority: "internal/infra/dockerruntime/principal_registry.go"},
			{Contract: "Gateway image API", Version: gatewayImageAPI, Authority: "gateway/Dockerfile"},
			{Contract: "Gateway OPA input", Version: gatewayOPAInputSchema, Authority: "gateway/addon/tobari_gateway.py"},
		},
		Runtime: runtimeVersionContract{
			DefaultSelector: defaultSelector,
			RuntimeAPI:      runtimeAPI,
			LifetimeCommand: lifetime,
			MetadataVersion: metadata.Version,
			Architectures:   metadata.Architectures,
			LocalBuild:      localBuild,
			MovingSelector:  !localBuild && !strings.Contains(defaultSelector, "@sha256:"),
		},
	}, nil
}

func helpSchemaVersion(catalog catalogDocument) int {
	var root rootHelp
	_ = json.Unmarshal(catalog.Root, &root)
	return root.SchemaVersion
}
