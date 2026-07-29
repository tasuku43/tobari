package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommandOutputKeepsDiagnosticsOutOfMachineReadableStdout(t *testing.T) {
	command := exec.Command(os.Args[0], "-test.run=TestArchlintCommandOutputHelper")
	command.Env = append(os.Environ(), "ARCHLINT_COMMAND_OUTPUT_HELPER=1")
	var reported strings.Builder

	stdout, stderr, err := commandOutput(command, &reported)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(stdout), `{"ImportPath":"example.test/tool"}`; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got, want := string(stderr), "go: downloading example.test/dependency\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	if got, want := reported.String(), string(stderr); got != want {
		t.Fatalf("reported diagnostics = %q, want %q", got, want)
	}
}

func TestArchlintCommandOutputHelper(t *testing.T) {
	if os.Getenv("ARCHLINT_COMMAND_OUTPUT_HELPER") != "1" {
		return
	}
	_, _ = os.Stdout.WriteString(`{"ImportPath":"example.test/tool"}`)
	_, _ = os.Stderr.WriteString("go: downloading example.test/dependency\n")
	os.Exit(0)
}

func TestFindViolationsEnforcesFourLayers(t *testing.T) {
	const module = "example.test/tool"
	packages := []listedPackage{
		{ImportPath: module + "/internal/domain/model", Imports: []string{module + "/internal/app/usecase"}},
		{ImportPath: module + "/internal/app/usecase", Imports: []string{module + "/internal/infra/store", module + "/internal/domain/model"}},
		{ImportPath: module + "/internal/infra/store", Imports: []string{module + "/internal/cli"}},
		{ImportPath: module + "/internal/cli", Imports: []string{module + "/internal/app/usecase", module + "/internal/infra/store"}},
	}

	got := findViolations(module, packages)
	want := []violation{
		{From: module + "/internal/app/usecase", To: module + "/internal/infra/store", Reason: "app may only import app and domain"},
		{From: module + "/internal/domain/model", To: module + "/internal/app/usecase", Reason: "domain may only import domain"},
		{From: module + "/internal/infra/store", To: module + "/internal/cli", Reason: "infra may only import infra and domain"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestFindViolationsRejectsUnclassifiedInternalPackages(t *testing.T) {
	const module = "example.test/tool"
	got := findViolations(module, []listedPackage{{
		ImportPath: module + "/internal/domain/model",
		Imports:    []string{module + "/internal/shared"},
	}})
	if len(got) != 1 || got[0].To != module+"/internal/shared" {
		t.Fatalf("violations = %#v", got)
	}
}

func TestFindViolationsEnforcesThinCommandAndClassifiedCLIImports(t *testing.T) {
	const module = "example.test/tool"
	packages := []listedPackage{
		{
			ImportPath: module + "/cmd/tool",
			Imports: []string{
				module + "/internal/cli",
				module + "/internal/app/usecase",
			},
		},
		{
			ImportPath: module + "/internal/cli",
			Imports:    []string{module + "/internal/shared"},
		},
	}

	got := findViolations(module, packages)
	want := []violation{
		{
			From:   module + "/cmd/tool",
			To:     module + "/internal/app/usecase",
			Reason: "cmd entrypoints may only import context, os signal handling, and cli",
		},
		{
			From:   module + "/internal/cli",
			To:     module + "/internal/shared",
			Reason: "dependencies must stay inside cmd, domain, app, infra, or cli",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestFindViolationsRejectsSideEffectAndThirdPartyImportsOutsideInfra(t *testing.T) {
	const module = "example.test/tool"
	packages := []listedPackage{
		{
			ImportPath: module + "/cmd/tool",
			Imports: []string{
				"os",
				"fmt",
				"example.com/cli-framework",
				module + "/internal/cli",
			},
		},
		{
			ImportPath: module + "/internal/domain/model",
			Imports:    []string{"strings", "os", "log", "example.com/uuid"},
		},
		{
			ImportPath: module + "/internal/app/usecase",
			Imports:    []string{"context", "net/http", "log/slog", "C"},
		},
		{
			ImportPath: module + "/internal/infra/client",
			Imports:    []string{"net/http", "example.com/sdk"},
		},
	}

	got := findViolations(module, packages)
	want := []violation{
		{
			From:   module + "/cmd/tool",
			To:     "example.com/cli-framework",
			Reason: "cmd entrypoints may only import context, os signal handling, and cli",
		},
		{
			From:   module + "/cmd/tool",
			To:     "fmt",
			Reason: "cmd entrypoints may only import context, os signal handling, and cli",
		},
		{
			From:   module + "/internal/app/usecase",
			To:     "C",
			Reason: "app may not import I/O or process packages",
		},
		{
			From:   module + "/internal/app/usecase",
			To:     "log/slog",
			Reason: "app may not import I/O or process packages",
		},
		{
			From:   module + "/internal/app/usecase",
			To:     "net/http",
			Reason: "app may not import I/O or process packages",
		},
		{
			From:   module + "/internal/domain/model",
			To:     "example.com/uuid",
			Reason: "domain may not import third-party packages; own a port or domain type instead",
		},
		{
			From:   module + "/internal/domain/model",
			To:     "log",
			Reason: "domain may not import I/O or process packages",
		},
		{
			From:   module + "/internal/domain/model",
			To:     "os",
			Reason: "domain may not import I/O or process packages",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestCLIThirdPartyImportsRequireExactPresentationAllowlist(t *testing.T) {
	const module = "example.test/tool"
	const presentation = "example.com/terminal/render"
	const sibling = "example.com/terminal/render/unsafe"
	const sdk = "example.com/vendor/sdk"
	packages := []listedPackage{
		{ImportPath: module + "/internal/cli", Imports: []string{presentation, sibling, sdk}},
		{ImportPath: module + "/internal/infra/client", Imports: []string{sdk}},
	}

	got := findViolationsWithCLIAllowlist(module, packages, []string{presentation})
	want := []violation{
		{
			From:   module + "/internal/cli",
			To:     sibling,
			Reason: "cli may not import third-party packages by default; keep SDKs and transports in infra or add an ADR-backed exact presentation-only allowlist entry",
		},
		{
			From:   module + "/internal/cli",
			To:     sdk,
			Reason: "cli may not import third-party packages by default; keep SDKs and transports in infra or add an ADR-backed exact presentation-only allowlist entry",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}

	withoutException := findViolations(module, packages)
	if len(withoutException) != 3 {
		t.Fatalf("default violations = %#v, want all three CLI imports rejected", withoutException)
	}
}

func TestInspectSourceRejectsApplicationPresentationIOButAllowsFormatting(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "usecase.go")
	source := `package usecase
import (
  formatter "fmt"
  "strings"
)
func run() {
  _ = formatter.Sprintf("%s", "value")
  _, _ = formatter.Sscanf("42", "%d", new(int))
  formatter.Printf("secret=%s", "value")
  formatter.Fprintln(strings.Builder{}, "value")
  formatter.Scanln(new(string))
  formatter.Fscanf(strings.NewReader("42"), "%d", new(int))
  println("bypass")
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectSourceFile("app", path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"fmt.Printf", "fmt.Fprintln", "fmt.Scanln", "fmt.Fscanf", "println"}
	if len(got) != len(want) {
		t.Fatalf("violations = %#v, want %v", got, want)
	}
	for index, name := range want {
		if got[index].To != name {
			t.Fatalf("violations = %#v, want %v", got, want)
		}
	}
}

func TestInspectSourceRejectsCLIDetachedContextAndBuiltinOutputButAllowsFMT(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "render.go")
	source := `package cli
import (
  "context"
  "fmt"
)
func render() {
  _ = context.Background()
  fmt.Println("safe reviewed output")
  print("bypass")
  println("bypass")
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectSourceFile("cli", path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"context.Background", "print", "println"}
	if len(got) != len(want) {
		t.Fatalf("violations = %#v, want %v", got, want)
	}
	for index, name := range want {
		if got[index].To != name {
			t.Fatalf("violations = %#v, want %v", got, want)
		}
	}
}

func TestInspectSourceRestrictsAuthenticationBindingIssuanceToInfrastructure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "binding.go")
	source := `package binding
import security "example.test/tool/internal/domain/authn"
func issue() { _, _ = security.NewBindingID("ephemeral-binding") }
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, layer := range []string{"cmd", "domain", "app", "cli"} {
		t.Run(layer, func(t *testing.T) {
			got, err := inspectSourceFile(layer, path)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range got {
				if item.To == "security.NewBindingID" {
					found = true
				}
			}
			if !found {
				t.Fatalf("violations = %#v, want binding issuance rejection", got)
			}
		})
	}
	got, err := inspectSourceFile("infra", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("infrastructure binding issuance violations = %#v", got)
	}

	dotPath := filepath.Join(directory, "dot_binding.go")
	dotSource := `package binding
import . "example.test/tool/internal/domain/authn"
func issue() { _, _ = NewBindingID("ephemeral-binding") }
`
	if err := os.WriteFile(dotPath, []byte(dotSource), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = inspectSourceFile("app", dotPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Reason != "dot imports bypass static API-boundary checks" {
		t.Fatalf("dot-import violations = %#v, want boundary rejection", got)
	}
}

func TestFindSourceViolationsScansDomainAndCLIForBuiltinOutput(t *testing.T) {
	const module = "example.test/tool"
	root := t.TempDir()
	domainDirectory := filepath.Join(root, "internal", "domain", "model")
	cliDirectory := filepath.Join(root, "internal", "cli")
	writeTestFile(t, root, "internal/domain/model/model.go", "package model\nfunc render() { print(\"bypass\") }\n")
	writeTestFile(t, root, "internal/cli/render.go", "package cli\nfunc render() { println(\"bypass\") }\n")

	got, err := findSourceViolations(module, []listedPackage{
		{ImportPath: module + "/internal/domain/model", Dir: domainDirectory, GoFiles: []string{"model.go"}},
		{ImportPath: module + "/internal/cli", Dir: cliDirectory, GoFiles: []string{"render.go"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []violation{
		{
			From:   filepath.Join(cliDirectory, "render.go") + ":2",
			To:     "println",
			Reason: "cli must render through its injected output streams",
		},
		{
			From:   filepath.Join(domainDirectory, "model.go") + ":2",
			To:     "print",
			Reason: "domain may not perform presentation I/O",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestFindViolationsScansAllProductionPackagesButExemptsTools(t *testing.T) {
	const module = "example.test/tool"
	packages := []listedPackage{
		{ImportPath: module + "/pkg/public"},
		{
			ImportPath: module + "/tools/archlint",
			Imports:    []string{"os", module + "/tools/internal/config"},
		},
	}
	want := []violation{{
		From:   module + "/pkg/public",
		To:     module + "/pkg/public",
		Reason: "production packages must stay inside cmd, domain, app, infra, or cli",
	}}
	if got := findViolations(module, packages); !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestFindViolationsRejectsUnclassifiedProductionPackageItself(t *testing.T) {
	const module = "example.test/tool"
	got := findViolations(module, []listedPackage{{ImportPath: module + "/internal/shared"}})
	want := []violation{{
		From:   module + "/internal/shared",
		To:     module + "/internal/shared",
		Reason: "production packages must stay inside cmd, domain, app, infra, or cli",
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("violations = %#v, want %#v", got, want)
	}
}

func TestInspectSourceRejectsDetachedContextsAndHTTPDefaults(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "client.go")
	source := `package client
import (
  "context"
  web "net/http"
)
func call() {
  _ = context.Background()
  _ = context.WithoutCancel(context.Background())
  _, _ = web.Get("https://example.invalid")
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectSourceFile("infra", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 || got[0].To != "context.Background" || got[1].To != "context.WithoutCancel" || got[2].To != "context.Background" || got[3].To != "http.Get" {
		t.Fatalf("violations = %#v", got)
	}
	if !strings.Contains(got[0].Reason, "propagate") || !strings.Contains(got[3].Reason, "finite timeout") {
		t.Fatalf("violations = %#v", got)
	}
}

func TestInspectSourceAllowsCallerContextAndExplicitHTTPClient(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "client.go")
	source := `package client
import (
  "context"
  "net/http"
)
func call(ctx context.Context, client *http.Client, request *http.Request) {
  _, _ = client.Do(request.WithContext(ctx))
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectSourceFile("infra", path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("violations = %#v", got)
	}
}

func TestInspectSourceRestrictsCommandEntrypointSelectors(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "main.go")
	source := `package main
import (
  "context"
  "os"
  "os/signal"
  "syscall"
)
func run() {
  ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
  _, _ = ctx, stop
  _ = os.Args
  _ = os.Getenv("SECRET")
  _, _ = syscall.Open("path", 0, 0)
  _ = context.TODO()
  println("bypass")
}
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := inspectSourceFile("cmd", path)
	if err != nil {
		t.Fatal(err)
	}
	wanted := []string{"os.Getenv", "syscall.Open", "context.TODO", "println"}
	for _, want := range wanted {
		found := false
		for _, item := range got {
			if item.To == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("violations = %#v, want %s", got, want)
		}
	}
	if len(got) != len(wanted) {
		t.Fatalf("violations = %#v, want exactly %d", got, len(wanted))
	}
}

func TestLoadPackagesIncludesPlatformSpecificProductionFiles(t *testing.T) {
	t.Setenv("GOCACHE", t.TempDir())
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.test/platform\n\ngo 1.26\n")
	writeTestFile(t, root, "internal/app/usecase/usecase.go", "package usecase\n")
	writeTestFile(t, root, "internal/app/usecase/usecase_windows.go", "//go:build windows\n\npackage usecase\n\nimport _ \"net/http\"\n")

	module, packages, err := loadPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	violations := findViolations(module, packages)
	if len(violations) != 1 || violations[0].To != "net/http" {
		t.Fatalf("platform-specific violations = %#v", violations)
	}
}

func writeTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPackagesScansWholeModuleIncludingTools(t *testing.T) {
	t.Setenv("GOCACHE", t.TempDir())
	root := filepath.Clean(filepath.Join("..", ".."))
	module, packages, err := loadPackages(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		module + "/cmd/agentic-cli-foundry":   false,
		module + "/internal/domain/operation": false,
		module + "/tools/archlint":            false,
	}
	for _, item := range packages {
		if _, exists := want[item.ImportPath]; exists {
			want[item.ImportPath] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("go list ./... did not include %s", path)
		}
	}
}
