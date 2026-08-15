package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/cli"
)

func TestGenerateRejectsUnsafeSourceRef(t *testing.T) {
	for _, sourceRef := range []string{"", "main", "--output=/tmp/archive", "HEAD^{tree}"} {
		t.Run(sourceRef, func(t *testing.T) {
			if _, err := generate(t.TempDir(), sourceRef); err == nil || !strings.Contains(err.Error(), "source ref") {
				t.Fatalf("generate(%q) error = %v, want source-ref rejection", sourceRef, err)
			}
		})
	}
}

func TestGenerateCatalogUsesExecutableAgentHelp(t *testing.T) {
	document, err := generateCatalog(currentAgentHelp)
	if err != nil {
		t.Fatalf("generateCatalog() error = %v", err)
	}

	rootRaw, err := currentAgentHelp()
	if err != nil {
		t.Fatalf("agentHelp() error = %v", err)
	}
	if !bytes.Equal(document.Root, rootRaw) {
		t.Fatal("generated root catalog does not preserve executable agent help")
	}

	var root rootHelp
	if err := json.Unmarshal(document.Root, &root); err != nil {
		t.Fatalf("decode generated root help: %v", err)
	}
	if root.SchemaVersion <= 0 {
		t.Fatalf("root schema version = %d, want positive", root.SchemaVersion)
	}
	if document.CommandCount != len(root.Commands) {
		t.Fatalf("command count = %d, root commands = %d", document.CommandCount, len(root.Commands))
	}
	// The current public surface has 32 commands. Keep this as a lower bound so
	// a reviewed capability addition does not require an unrelated count edit.
	if document.CommandCount < 32 {
		t.Fatalf("command count = %d, want at least 32", document.CommandCount)
	}

	rootPaths := make(map[string]string, len(root.Commands))
	namespaceOrder := make([]string, 0, len(root.Commands))
	seenNamespaces := make(map[string]bool)
	for _, command := range root.Commands {
		if command.Path == "" || command.Namespace == "" {
			t.Fatalf("root command is incomplete: path=%q namespace=%q", command.Path, command.Namespace)
		}
		if _, exists := rootPaths[command.Path]; exists {
			t.Fatalf("root command %q is duplicated", command.Path)
		}
		rootPaths[command.Path] = command.Namespace
		if !seenNamespaces[command.Namespace] {
			seenNamespaces[command.Namespace] = true
			namespaceOrder = append(namespaceOrder, command.Namespace)
		}
	}
	for _, required := range []string{
		"doctor", "help", "version",
		"cluster up", "cluster status", "cluster down",
		"policy candidates", "policy allow", "policy deny",
		"context list", "context show", "context use",
		"runtime init", "runtime build",
		"tobari", "status", "list", "delete",
		"auth login", "auth import", "auth status", "auth logout",
	} {
		if _, exists := rootPaths[required]; !exists {
			t.Errorf("required public command %q is missing", required)
		}
	}

	if len(document.Scopes) != len(namespaceOrder) {
		t.Fatalf("scope count = %d, unique namespaces = %d", len(document.Scopes), len(namespaceOrder))
	}
	scopedCounts := make(map[string]int, len(root.Commands))
	for index, raw := range document.Scopes {
		if !json.Valid(raw) {
			t.Fatalf("scope %d is not valid JSON", index)
		}
		expected, err := currentAgentHelp(namespaceOrder[index])
		if err != nil {
			t.Fatalf("agentHelp(%q) error = %v", namespaceOrder[index], err)
		}
		if !bytes.Equal(raw, expected) {
			t.Fatalf("scope %q does not preserve executable agent help", namespaceOrder[index])
		}
		var scope scopeHelp
		if err := json.Unmarshal(raw, &scope); err != nil {
			t.Fatalf("decode scope %q: %v", namespaceOrder[index], err)
		}
		for _, command := range scope.Commands {
			if _, exists := rootPaths[command.Path]; !exists {
				t.Errorf("scoped command %q is absent from root help", command.Path)
			}
			scopedCounts[command.Path]++
		}
	}
	for path := range rootPaths {
		if count := scopedCounts[path]; count != 1 {
			t.Errorf("command %q appears %d times in scoped help, want exactly one", path, count)
		}
	}

	globalUnknown := 0
	commandFault := false
	doctorInvalidArguments := false
	for _, occurrence := range document.Faults {
		if occurrence.Code == "unknown_command" && occurrence.Command == "(global)" {
			globalUnknown++
			if occurrence.Kind != "invalid_input" || occurrence.Retryable {
				t.Errorf("global unknown_command = %+v", occurrence)
			}
		}
		if occurrence.Command != "(global)" {
			commandFault = true
		}
		if occurrence.Code == "invalid_arguments" && occurrence.Command == "doctor" {
			doctorInvalidArguments = true
		}
	}
	if globalUnknown != 1 {
		t.Errorf("global unknown_command occurrences = %d, want 1", globalUnknown)
	}
	if !commandFault {
		t.Error("generated fault reference contains no command faults")
	}
	if !doctorInvalidArguments {
		t.Error("generated fault reference is missing doctor invalid_arguments")
	}
}

func TestGenerateUsesCommittedAgentHelp(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	outputs, err := generate(root, "HEAD")
	if err != nil {
		t.Fatalf("generate(root, HEAD) error = %v", err)
	}
	generatedCatalog := outputs["catalog.json"]
	if len(generatedCatalog) == 0 {
		t.Fatal("generate(root, HEAD) omitted catalog.json")
	}
	providerFixture := bytes.TrimSpace(committedForTest(
		t,
		root,
		"internal/infra/authproviders/testdata/synthetic-provider-v1.json",
	))
	if got := bytes.TrimSpace(outputs["provider-manifest-example.json"]); !bytes.Equal(got, providerFixture) {
		t.Fatal("generated provider manifest example does not preserve the committed canonical fixture")
	}

	runCommittedHelp, cleanup, err := buildCommittedAgentHelpRunner(root, "HEAD")
	if err != nil {
		t.Fatalf("build committed agent help runner: %v", err)
	}
	defer cleanup()
	committedDocument, err := generateCatalog(runCommittedHelp)
	if err != nil {
		t.Fatalf("generate committed catalog: %v", err)
	}
	committedDocument.GeneratedFrom = fmt.Sprintf("%s at commit %s", committedDocument.GeneratedFrom, "HEAD")
	wantCommitted, err := canonicalJSON(committedDocument)
	if err != nil {
		t.Fatalf("encode committed catalog: %v", err)
	}
	if !bytes.Equal(generatedCatalog, wantCommitted) {
		t.Fatal("generate(root, HEAD) does not preserve the committed CLI agent help")
	}

	workingDocument, err := generateCatalog(currentAgentHelp)
	if err != nil {
		t.Fatalf("generate working-tree catalog: %v", err)
	}
	workingCatalog, err := canonicalJSON(workingDocument)
	if err != nil {
		t.Fatalf("encode working-tree catalog: %v", err)
	}
	if !bytes.Equal(wantCommitted, workingCatalog) && bytes.Equal(generatedCatalog, workingCatalog) {
		t.Fatal("generate(root, HEAD) leaked the working-tree CLI catalog")
	}
}

func TestGenerateVersionsDerivesCommittedAuthorities(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatalf("repositoryRoot() error = %v", err)
	}
	catalog, err := generateCatalog(currentAgentHelp)
	if err != nil {
		t.Fatalf("generateCatalog() error = %v", err)
	}
	// Product schema versions must come from the same committed source snapshot
	// as the other component facts, not from dirty executable scoped help. The
	// root remains available because the Agent help schema is an executable
	// contract in its own right.
	catalog.Scopes = nil
	document, err := generateVersions(root, "HEAD", catalog)
	if err != nil {
		t.Fatalf("generateVersions() error = %v", err)
	}

	goMod := committedForTest(t, root, "go.mod")
	wantGo := captureForTest(t, goMod, `(?m)^go ([0-9]+\.[0-9]+\.[0-9]+)$`)
	goComponent := componentForTest(t, document, "Go")
	if goComponent.Version != wantGo || goComponent.Identity != "go "+wantGo {
		t.Errorf("Go component = %+v, want version %q from HEAD go.mod", goComponent, wantGo)
	}

	versionValues := envForTest(committedForTest(
		t, root, "internal/infra/runtimeassets/assets/versions.env",
	))
	digestComponents := map[string]string{
		"OPA":                      "OPA_IMAGE",
		"mitmproxy":                "MITMPROXY_IMAGE",
		"Base runtime build image": "DEBIAN_IMAGE",
	}
	digestPattern := regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	for componentName, authorityName := range digestComponents {
		component := componentForTest(t, document, componentName)
		want := versionValues[authorityName]
		if want == "" {
			t.Fatalf("HEAD versions.env does not define %s", authorityName)
		}
		if component.Identity != want {
			t.Errorf("%s identity = %q, want HEAD %s %q", componentName, component.Identity, authorityName, want)
		}
		if !digestPattern.MatchString(component.Identity) {
			t.Errorf("%s identity %q is not immutable", componentName, component.Identity)
		}
	}
	for componentName, authorityName := range map[string]string{
		"Gateway":     "GATEWAY_IMAGE",
		"Auth Broker": "AUTH_BROKER_IMAGE",
	} {
		component := componentForTest(t, document, componentName)
		if historical := versionValues[authorityName]; historical != "" {
			if component.Identity != historical {
				t.Errorf("%s identity = %q, want historical HEAD %s %q", componentName, component.Identity, authorityName, historical)
			}
			if historical != "unpublished" && !digestPattern.MatchString(historical) {
				t.Errorf("%s historical identity %q is not immutable", componentName, historical)
			}
			continue
		}
		if component.Version != "release-generated V1" || component.Identity != "injected immutable digest" ||
			component.Authority != "component-lock.json generated by the release workflow" {
			t.Errorf("%s release-generated identity = %+v", componentName, component)
		}
	}

	contextSource := committedForTest(t, root, "internal/domain/tobari/context.go")
	wantSelector := captureForTest(t, contextSource, `OfficialRuntimeBase\s*=\s*"([^"]+)"`)
	if document.Runtime.DefaultSelector != wantSelector {
		t.Errorf("default runtime selector = %q, want HEAD value %q", document.Runtime.DefaultSelector, wantSelector)
	}
	wantLocalBuild := wantSelector == "tobari-runtime:base"
	if document.Runtime.LocalBuild != wantLocalBuild {
		t.Errorf("default runtime selector = %+v, want local_build=%t", document.Runtime, wantLocalBuild)
	}
	wantMoving := !wantLocalBuild && !strings.Contains(wantSelector, "@sha256:")
	if document.Runtime.MovingSelector != wantMoving {
		t.Errorf("default runtime selector = %+v, want moving_selector=%t", document.Runtime, wantMoving)
	}

	tobariSource := committedForTest(t, root, "internal/domain/tobari/tobari.go")
	wantRuntimeAPI := captureForTest(t, tobariSource, `RuntimeImageAPI\s*=\s*"([^"]+)"`)
	if document.Runtime.RuntimeAPI != wantRuntimeAPI {
		t.Errorf("runtime API = %q, want HEAD value %q", document.Runtime.RuntimeAPI, wantRuntimeAPI)
	}

	wantContextSchema := captureIntForTest(
		t, contextSource, `ContextSchemaVersion\s*=\s*([0-9]+)`,
	)
	providerSource := committedForTest(t, root, "internal/domain/authbroker/provider.go")
	wantProviderSchema := captureIntForTest(
		t, providerSource, `(?m)^\s*ProviderSchemaVersion\s*=\s*([0-9]+)`,
	)
	wantOwnerProviderSchema := wantProviderSchema
	if match := regexp.MustCompile(`(?m)^\s*LegacyProviderSchemaVersion\s*=\s*([0-9]+)`).FindSubmatch(providerSource); len(match) == 2 {
		wantOwnerProviderSchema, err = strconv.Atoi(string(match[1]))
		if err != nil {
			t.Fatal(err)
		}
	}
	catalogSource := committedForTest(t, root, "internal/cli/runtime_catalog.go")
	wantContextReportSchema, err := contextReportSchemaVersion(catalogSource)
	if err != nil {
		t.Fatalf("derive committed Context report schema: %v", err)
	}
	if wantContextSchema < 1 || wantContextReportSchema < 1 || wantProviderSchema < 1 || wantOwnerProviderSchema < 1 {
		t.Fatalf(
			"HEAD public schema authorities must be positive: Context manifest=%d report=%d owner provider=%d projection=%d",
			wantContextSchema, wantContextReportSchema, wantOwnerProviderSchema, wantProviderSchema,
		)
	}
	for contract, want := range map[string]int{
		"Context manifest":        wantContextSchema,
		"Public Context report":   wantContextReportSchema,
		"Owner provider manifest": wantOwnerProviderSchema,
		"Normalized provider projection / reviewed built-in manifest": wantProviderSchema,
	} {
		if got := schemaForTest(t, document, contract).Version; got != want {
			t.Errorf("%s schema = %d, want HEAD authority %d", contract, got, want)
		}
	}

	for contract, authority := range map[string][2]string{
		"Gateway image API":     {"gateway/Dockerfile", `io\.tobari\.gateway-api="([0-9]+)"`},
		"Gateway OPA input":     {"gateway/addon/tobari_gateway.py", `policy_input\s*=\s*\{\s*"schema_version":\s*([0-9]+),`},
		"Auth Broker image API": {"authbroker/Dockerfile", `io\.tobari\.auth-broker-api="([0-9]+)"`},
		"Auth Broker control/runtime protocol and static vault": {"authbroker/__init__.py", `(?m)^SCHEMA_VERSION\s*=\s*([0-9]+)$`},
	} {
		want := captureIntForTest(t, committedForTest(t, root, authority[0]), authority[1])
		if got := schemaForTest(t, document, contract).Version; got != want {
			t.Errorf("%s schema = %d, want HEAD authority %d", contract, got, want)
		}
	}
}

func TestContextReportSchemaDerivationAllowsInterposedCatalogFields(t *testing.T) {
	source := []byte(`JSONEnvelope: "context", JSONEnvelopeType: OutputFieldTypeObject, JSONSchemaVersion: 7,`)
	got, err := contextReportSchemaVersion(source)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("Context report schema = %d, want 7", got)
	}
}

func currentAgentHelp(args ...string) (json.RawMessage, error) {
	var out, errOut bytes.Buffer
	command := cli.New(strings.NewReader(""), &out, &errOut)
	command.Version = "documentation"
	fullArgs := append([]string{"help"}, args...)
	fullArgs = append(fullArgs, "--format", "agent")
	if code := command.RunContext(context.Background(), fullArgs); code != cli.ExitOK {
		return nil, fmt.Errorf(
			"run %q: exit %d: %s",
			strings.Join(fullArgs, " "),
			code,
			strings.TrimSpace(errOut.String()),
		)
	}
	content := bytes.TrimSpace(out.Bytes())
	if !json.Valid(content) {
		return nil, fmt.Errorf("CLI agent help returned invalid JSON")
	}
	return append(json.RawMessage(nil), content...), nil
}

func committedForTest(t *testing.T, root, path string) []byte {
	t.Helper()
	content, err := committedFile(root, "HEAD", path)
	if err != nil {
		t.Fatalf("read %s from HEAD: %v", path, err)
	}
	return content
}

func captureForTest(t *testing.T, content []byte, expression string) string {
	t.Helper()
	match := regexp.MustCompile(expression).FindSubmatch(content)
	if len(match) != 2 {
		t.Fatalf("expression %q did not match committed source", expression)
	}
	return string(match[1])
}

func captureIntForTest(t *testing.T, content []byte, expression string) int {
	t.Helper()
	value := captureForTest(t, content, expression)
	result := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			t.Fatalf("captured non-integer value %q", value)
		}
		result = result*10 + int(character-'0')
	}
	return result
}

func envForTest(content []byte) map[string]string {
	values := make(map[string]string)
	for _, line := range strings.Split(string(content), "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found && name != "" {
			values[name] = value
		}
	}
	return values
}

func componentForTest(
	t *testing.T, document componentVersionDocument, name string,
) componentVersion {
	t.Helper()
	for _, component := range document.Components {
		if component.Component == name {
			return component
		}
	}
	t.Fatalf("component %q is missing", name)
	return componentVersion{}
}

func schemaForTest(
	t *testing.T, document componentVersionDocument, contract string,
) schemaVersion {
	t.Helper()
	for _, schema := range document.Schemas {
		if schema.Contract == contract {
			return schema
		}
	}
	t.Fatalf("schema contract %q is missing", contract)
	return schemaVersion{}
}
