// Command archlint enforces the repository's four-layer import contract.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type listedPackage struct {
	ImportPath string
	Imports    []string
	Dir        string
	GoFiles    []string
	CgoFiles   []string
}

type violation struct {
	From   string
	To     string
	Reason string
}

type goTarget struct {
	goos   string
	goarch string
}

var productionTargets = []goTarget{
	{}, // Native target retains production CgoFiles when cgo is enabled.
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
	{goos: "darwin", goarch: "amd64"},
	{goos: "darwin", goarch: "arm64"},
	{goos: "windows", goarch: "amd64"},
}

// allowedCLIThirdPartyImports is intentionally empty in the public template.
// A derived project may add an exact presentation-only package path after an
// accepted ADR; prefixes, module-wide entries, SDKs, and transports belong in
// infrastructure and must not be added here.
var allowedCLIThirdPartyImports = []string{}

func main() {
	root, err := filepath.Abs(".")
	if err != nil {
		fatal(err)
	}
	module, packages, err := loadPackages(root)
	if err != nil {
		fatal(err)
	}
	violations := findViolations(module, packages)
	sourceViolations, err := findSourceViolations(module, packages)
	if err != nil {
		fatal(err)
	}
	violations = append(violations, sourceViolations...)
	sortViolations(violations)
	if len(violations) != 0 {
		fmt.Fprintln(os.Stderr, "architecture import violations:")
		for _, item := range violations {
			fmt.Fprintf(os.Stderr, "  %s -> %s: %s\n", item.From, item.To, item.Reason)
		}
		os.Exit(1)
	}
	fmt.Printf("archlint: OK (%d packages)\n", len(packages))
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "archlint: %v\n", err)
	os.Exit(1)
}

func loadPackages(root string) (string, []listedPackage, error) {
	moduleBytes, err := runGo(root, "list", "-m", "-f", "{{.Path}}")
	if err != nil {
		return "", nil, err
	}
	module := strings.TrimSpace(string(moduleBytes))
	if module == "" || module == "command-line-arguments" {
		return "", nil, errors.New("go list did not return a module path")
	}

	merged := make(map[string]listedPackage)
	for _, target := range productionTargets {
		var data []byte
		if target.goos == "" {
			data, err = runGo(root, "list", "-json", "./...")
		} else {
			data, err = runGoTarget(root, target, "list", "-json", "./...")
		}
		if err != nil {
			return "", nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		for decoder.More() {
			var item listedPackage
			if err := decoder.Decode(&item); err != nil {
				return "", nil, fmt.Errorf("decode go list output for %s/%s: %w", target.goos, target.goarch, err)
			}
			merged[item.ImportPath] = mergeListedPackage(merged[item.ImportPath], item)
		}
	}
	packages := make([]listedPackage, 0, len(merged))
	for _, item := range merged {
		packages = append(packages, item)
	}
	sort.Slice(packages, func(i, j int) bool { return packages[i].ImportPath < packages[j].ImportPath })
	return module, packages, nil
}

func runGo(root string, args ...string) ([]byte, error) {
	return runGoWithEnv(root, nil, args...)
}

func runGoTarget(root string, target goTarget, args ...string) ([]byte, error) {
	overrides := []string{"GOOS=" + target.goos, "GOARCH=" + target.goarch, "CGO_ENABLED=0"}
	return runGoWithEnv(root, overrides, args...)
}

func runGoWithEnv(root string, overrides []string, args ...string) ([]byte, error) {
	command := exec.Command("go", args...) // #nosec G204 -- this unexported helper is called only with fixed go list arguments above.
	command.Dir = root
	if len(overrides) != 0 {
		environment := make([]string, 0, len(os.Environ())+len(overrides))
		for _, value := range os.Environ() {
			if strings.HasPrefix(value, "GOOS=") || strings.HasPrefix(value, "GOARCH=") || strings.HasPrefix(value, "CGO_ENABLED=") {
				continue
			}
			environment = append(environment, value)
		}
		command.Env = append(environment, overrides...)
	}
	output, diagnostics, err := commandOutput(command, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("go %s: %w\n%s", strings.Join(args, " "), err, diagnostics)
	}
	return output, nil
}

func commandOutput(command *exec.Cmd, diagnosticWriter io.Writer) ([]byte, []byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if err == nil && stderr.Len() != 0 {
		if diagnosticWriter == nil {
			return stdout.Bytes(), stderr.Bytes(), errors.New("successful command diagnostics have no reporting stream")
		}
		if _, writeErr := diagnosticWriter.Write(stderr.Bytes()); writeErr != nil {
			return stdout.Bytes(), stderr.Bytes(), fmt.Errorf("report successful command diagnostics: %w", writeErr)
		}
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func mergeListedPackage(existing, incoming listedPackage) listedPackage {
	if existing.ImportPath == "" {
		existing.ImportPath = incoming.ImportPath
		existing.Dir = incoming.Dir
	}
	existing.Imports = mergeStrings(existing.Imports, incoming.Imports)
	existing.GoFiles = mergeStrings(existing.GoFiles, incoming.GoFiles)
	existing.CgoFiles = mergeStrings(existing.CgoFiles, incoming.CgoFiles)
	return existing
}

func mergeStrings(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		seen[value] = struct{}{}
	}
	merged := make([]string, 0, len(seen))
	for value := range seen {
		merged = append(merged, value)
	}
	sort.Strings(merged)
	return merged
}

func findViolations(module string, packages []listedPackage) []violation {
	return findViolationsWithCLIAllowlist(module, packages, allowedCLIThirdPartyImports)
}

func findViolationsWithCLIAllowlist(module string, packages []listedPackage, allowedCLIImports []string) []violation {
	var found []violation
	for _, item := range packages {
		fromLayer := packageLayer(module, item.ImportPath)
		if fromLayer == "" || fromLayer == "tool" {
			continue
		}
		if fromLayer == "unknown" {
			found = append(found, violation{
				From:   item.ImportPath,
				To:     item.ImportPath,
				Reason: "production packages must stay inside cmd, domain, app, infra, or cli",
			})
			continue
		}
		for _, imported := range item.Imports {
			reason := forbiddenImportReason(module, fromLayer, imported, allowedCLIImports)
			if reason != "" {
				found = append(found, violation{From: item.ImportPath, To: imported, Reason: reason})
			}
		}
	}
	sortViolations(found)
	return found
}

func sortViolations(found []violation) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].From != found[j].From {
			return found[i].From < found[j].From
		}
		if found[i].To != found[j].To {
			return found[i].To < found[j].To
		}
		return found[i].Reason < found[j].Reason
	})
}

func packageLayer(module, importPath string) string {
	toolPrefix := module + "/tools"
	if importPath == toolPrefix || strings.HasPrefix(importPath, toolPrefix+"/") {
		return "tool"
	}
	cmdPrefix := module + "/cmd"
	if importPath == cmdPrefix || strings.HasPrefix(importPath, cmdPrefix+"/") {
		return "cmd"
	}
	for _, layer := range []string{"domain", "app", "infra", "cli"} {
		prefix := module + "/internal/" + layer
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return layer
		}
	}
	if importPath == module || strings.HasPrefix(importPath, module+"/") {
		return "unknown"
	}
	return ""
}

func forbiddenImportReason(module, from, imported string, allowedCLIImports []string) string {
	to := packageLayer(module, imported)
	if to != "" {
		return forbiddenReason(from, to)
	}

	switch from {
	case "cmd":
		if !allowedCommandStandardImport(imported) {
			return "cmd entrypoints may only import context, os signal handling, and cli"
		}
	case "domain":
		if imported == "context" {
			return "domain may not depend on execution context"
		}
		if isEffectfulStandardImport(imported) || isLoggingStandardImport(imported) {
			return "domain may not import I/O or process packages"
		}
		if isThirdPartyImport(imported) {
			return "domain may not import third-party packages; own a port or domain type instead"
		}
	case "app":
		if isEffectfulStandardImport(imported) || isLoggingStandardImport(imported) {
			return "app may not import I/O or process packages"
		}
		if isThirdPartyImport(imported) {
			return "app may not import third-party packages; own a port or domain type instead"
		}
	case "cli":
		if isEffectfulStandardImport(imported) {
			return "cli may not own filesystem, network, or process I/O; use an infrastructure adapter"
		}
		if isThirdPartyImport(imported) && !containsExact(allowedCLIImports, imported) {
			return "cli may not import third-party packages by default; keep SDKs and transports in infra or add an ADR-backed exact presentation-only allowlist entry"
		}
	}
	return ""
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func allowedCommandStandardImport(importPath string) bool {
	switch importPath {
	case "context", "os", "os/signal", "syscall":
		return true
	default:
		return false
	}
}

func isEffectfulStandardImport(importPath string) bool {
	for _, forbidden := range []string{
		"C",
		"database/sql",
		"net",
		"net/http",
		"os",
		"os/exec",
		"plugin",
		"runtime/cgo",
		"syscall",
	} {
		if importPath == forbidden || strings.HasPrefix(importPath, forbidden+"/") {
			return true
		}
	}
	return false
}

func isLoggingStandardImport(importPath string) bool {
	return importPath == "log" || strings.HasPrefix(importPath, "log/")
}

func isThirdPartyImport(importPath string) bool {
	first, _, _ := strings.Cut(importPath, "/")
	return strings.Contains(first, ".")
}

func forbiddenReason(from, to string) string {
	if to == "tool" {
		return "production packages may not import repository tools"
	}
	if to == "unknown" {
		return "dependencies must stay inside cmd, domain, app, infra, or cli"
	}
	if from == "unknown" {
		return "unclassified production packages may not own dependencies"
	}
	if from == "cmd" {
		if to != "cli" {
			return "cmd entrypoints may only import context, os signal handling, and cli"
		}
		return ""
	}
	if from == "cli" {
		if to == "cmd" {
			return "cli may not import cmd entrypoints"
		}
		return ""
	}
	if from == to {
		return ""
	}
	switch from {
	case "domain":
		return "domain may only import domain"
	case "app":
		if to != "domain" {
			return "app may only import app and domain"
		}
	case "infra":
		if to != "domain" {
			return "infra may only import infra and domain"
		}
	}
	return ""
}

func findSourceViolations(module string, packages []listedPackage) ([]violation, error) {
	var found []violation
	for _, item := range packages {
		layer := packageLayer(module, item.ImportPath)
		if layer != "cmd" && layer != "domain" && layer != "app" && layer != "infra" && layer != "cli" {
			continue
		}
		files := append(append([]string(nil), item.GoFiles...), item.CgoFiles...)
		for _, name := range files {
			path := filepath.Join(item.Dir, name)
			violations, err := inspectSourceFile(layer, path)
			if err != nil {
				return nil, err
			}
			found = append(found, violations...)
		}
	}
	sortViolations(found)
	return found, nil
}

func inspectSourceFile(layer, path string) ([]violation, error) {
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	aliases := make(map[string]string)
	for _, imported := range file.Imports {
		importPath := strings.Trim(imported.Path.Value, `"`)
		alias := ""
		if imported.Name != nil {
			alias = imported.Name.Name
		}
		if alias == "." && (layer == "cmd" || importPath == "context" || importPath == "net/http" ||
			(layer == "app" && importPath == "fmt") ||
			(layer != "infra" && strings.HasSuffix(importPath, "/internal/domain/authn"))) {
			return []violation{{
				From:   path,
				To:     importPath,
				Reason: "dot imports bypass static API-boundary checks",
			}}, nil
		}
		if alias == "" {
			alias = importPath
			if index := strings.LastIndex(importPath, "/"); index >= 0 {
				alias = importPath[index+1:]
			}
		}
		if alias != "_" {
			aliases[alias] = importPath
		}
	}

	var found []violation
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if identifier, ok := call.Fun.(*ast.Ident); ok && (identifier.Name == "print" || identifier.Name == "println") {
				position := set.Position(call.Pos())
				switch layer {
				case "cmd":
					found = append(found, violation{
						From:   fmt.Sprintf("%s:%d", path, position.Line),
						To:     identifier.Name,
						Reason: "cmd entrypoints may not render output directly",
					})
				case "domain":
					found = append(found, violation{
						From:   fmt.Sprintf("%s:%d", path, position.Line),
						To:     identifier.Name,
						Reason: "domain may not perform presentation I/O",
					})
				case "app":
					found = append(found, violation{
						From:   fmt.Sprintf("%s:%d", path, position.Line),
						To:     identifier.Name,
						Reason: "app may not render output directly",
					})
				case "cli":
					found = append(found, violation{
						From:   fmt.Sprintf("%s:%d", path, position.Line),
						To:     identifier.Name,
						Reason: "cli must render through its injected output streams",
					})
				}
			}
		}
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		position := set.Position(selector.Pos())
		from := fmt.Sprintf("%s:%d", path, position.Line)
		if selector.Sel.Name == "NewBindingID" && strings.HasSuffix(aliases[identifier.Name], "/internal/domain/authn") && layer != "infra" {
			found = append(found, violation{
				From:   from,
				To:     identifier.Name + ".NewBindingID",
				Reason: "authentication binding IDs may only be issued inside infrastructure",
			})
		}
		if layer == "cmd" {
			if reason := forbiddenCommandSelector(aliases[identifier.Name], selector.Sel.Name); reason != "" {
				found = append(found, violation{From: from, To: identifier.Name + "." + selector.Sel.Name, Reason: reason})
				return true
			}
		}
		if layer == "app" && aliases[identifier.Name] == "fmt" && forbiddenApplicationFMT(selector.Sel.Name) {
			found = append(found, violation{
				From:   from,
				To:     "fmt." + selector.Sel.Name,
				Reason: "app may format values but may not perform presentation I/O",
			})
		}
		switch aliases[identifier.Name] {
		case "context":
			if layer != "cmd" && (selector.Sel.Name == "Background" || selector.Sel.Name == "TODO" || selector.Sel.Name == "WithoutCancel") {
				found = append(found, violation{
					From:   from,
					To:     "context." + selector.Sel.Name,
					Reason: layer + " must propagate its caller context",
				})
			}
		case "net/http":
			if layer == "infra" && forbiddenHTTPDefault(selector.Sel.Name) {
				found = append(found, violation{
					From:   from,
					To:     "http." + selector.Sel.Name,
					Reason: "infra must use an explicit HTTP client with a finite timeout contract",
				})
			}
		}
		return true
	})
	return found, nil
}

func forbiddenApplicationFMT(name string) bool {
	return strings.HasPrefix(name, "Print") ||
		strings.HasPrefix(name, "Fprint") ||
		strings.HasPrefix(name, "Scan") ||
		strings.HasPrefix(name, "Fscan")
}

func forbiddenCommandSelector(importPath, name string) string {
	allowed := false
	switch importPath {
	case "context":
		allowed = name == "Background"
	case "os":
		switch name {
		case "Args", "Stdin", "Stdout", "Stderr", "Exit", "Interrupt":
			allowed = true
		}
	case "os/signal":
		allowed = name == "NotifyContext"
	case "syscall":
		switch name {
		case "SIGINT", "SIGTERM", "SIGHUP":
			allowed = true
		}
	default:
		return ""
	}
	if allowed {
		return ""
	}
	return "cmd entrypoints may only read process arguments and streams, install signal cancellation, call cli, and exit"
}

func forbiddenHTTPDefault(name string) bool {
	switch name {
	case "DefaultClient", "Get", "Head", "Post", "PostForm":
		return true
	default:
		return false
	}
}
