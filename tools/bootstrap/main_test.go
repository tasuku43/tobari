package main

import (
	"encoding/json"
	"errors"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/projectconfig"
)

func TestApplyPreviewsAndAppliesExactContentAndPathReplacements(t *testing.T) {
	root := t.TempDir()
	defaults := projectconfig.Defaults
	writeFixture(t, root, "README.md", "# "+defaults.Name+"\n\n"+defaults.Description+"\n")
	writeFixture(t, root, "cmd/"+defaults.BinaryName+"/main.go", "package main\n// "+defaults.GoModule+" "+defaults.BinaryName+" "+defaults.FormulaClass+"\n")
	protectedContents := defaults.BinaryName + " " + defaults.GoModule
	writeFixture(t, root, "tools/internal/projectconfig/defaults.go", protectedContents)
	target := projectconfig.Project{
		Name: "Acme Tool", BinaryName: "acme", GoModule: "github.com/acme/tool",
		GitHubOwner: "acme", GitHubRepository: "tool", Description: "A concise Acme command-line tool.",
		FormulaClass: "Acme", LicenseSPDX: "MIT", SecurityContact: "security@acme.example",
	}

	changed, renames, err := apply(root, replacements(target), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 || len(renames) != 1 {
		t.Fatalf("dry-run changes = %v, renames = %v", changed, renames)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", defaults.BinaryName)); err != nil {
		t.Fatal("dry-run changed the source tree")
	}

	if _, _, err := apply(root, replacements(target), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "cmd", "acme", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "github.com/acme/tool acme Acme") {
		t.Fatalf("updated source = %q", got)
	}
	protected, err := os.ReadFile(filepath.Join(root, "tools", "internal", "projectconfig", "defaults.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(protected) != protectedContents {
		t.Fatalf("source defaults changed: %q", protected)
	}
}

func TestHarnessBootstrapDocumentationUsesProtectedDefaultsConcept(t *testing.T) {
	sourcePath := filepath.Join("..", "..", "docs", "04_harness.md")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	start := strings.Index(text, "### `tools/bootstrap`")
	if start < 0 {
		t.Fatal("Harness bootstrap section is missing")
	}
	remainder := text[start+len("### `tools/bootstrap`"):]
	end := strings.Index(remainder, "\n### ")
	if end < 0 {
		t.Fatal("Harness bootstrap section has no following section")
	}
	section := remainder[:end]
	if !strings.Contains(section, "`projectconfig.Defaults`") {
		t.Fatal("Harness bootstrap section does not name the protected provenance source")
	}
	for _, replaceable := range []string{
		projectconfig.Defaults.Name,
		projectconfig.Defaults.BinaryName,
		projectconfig.Defaults.GoModule,
		projectconfig.Defaults.GitHubOwner + "/" + projectconfig.Defaults.GitHubRepository,
	} {
		if strings.Contains(section, replaceable) {
			t.Errorf("Harness bootstrap section contains replaceable identity %q", replaceable)
		}
	}

	root := t.TempDir()
	writeFixture(t, root, "docs/04_harness.md", section)
	if _, _, err := apply(root, replacements(configuredFixture().Project), false); err != nil {
		t.Fatal(err)
	}
	assertFileContents(t, filepath.Join(root, "docs", "04_harness.md"), section)
}

func TestApplyDoesNotRecursivelyReplaceTargetValues(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module "+projectconfig.Defaults.GoModule+"\n")
	target := projectconfig.Defaults
	target.GoModule = "github.com/acme/agentic-cli-foundry-pro"
	target.GitHubRepository = "different-repository"
	if _, _, err := apply(root, replacements(target), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "module github.com/acme/agentic-cli-foundry-pro\n" {
		t.Fatalf("replacement was recursive: %q", got)
	}
}

func TestApplyFormatsUpdatedGoFiles(t *testing.T) {
	root := t.TempDir()
	defaults := projectconfig.Defaults
	writeFixture(t, root, "main.go", "package main\n\nvar values = map[string]bool{\n\t\""+defaults.BinaryName+"\": false,\n\t\"longer-value\": false,\n}\n")
	target := defaults
	target.BinaryName = "a-much-longer-binary-name"

	if _, _, err := apply(root, replacements(target), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := format.Source([]byte("package main\n\nvar values = map[string]bool{\n\t\"a-much-longer-binary-name\": false,\n\t\"longer-value\": false,\n}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(want) {
		t.Fatalf("updated Go source = %q, want %q", data, want)
	}
}

func TestApplyRejectsInvalidUpdatedGoBeforeWriting(t *testing.T) {
	root := t.TempDir()
	defaults := projectconfig.Defaults
	originalGo := "package main\n\nconst name = \"" + defaults.Name + "\"\n"
	originalReadme := defaults.Name + "\n"
	writeFixture(t, root, "main.go", originalGo)
	writeFixture(t, root, "README.md", originalReadme)
	target := defaults
	target.Name = `invalid " quoted name`

	for _, dryRun := range []bool{true, false} {
		if _, _, err := apply(root, replacements(target), dryRun); err == nil || !strings.Contains(err.Error(), "format updated Go source main.go") {
			t.Fatalf("apply(dryRun=%t) error = %v", dryRun, err)
		}
		assertFileContents(t, filepath.Join(root, "main.go"), originalGo)
		assertFileContents(t, filepath.Join(root, "README.md"), originalReadme)
	}
}

func TestApplyPreflightsRenameCollisionsBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	defaults := projectconfig.Defaults
	writeFixture(t, root, "README.md", defaults.Name+"\n")
	writeFixture(t, root, "cmd/"+defaults.BinaryName+"/main.go", defaults.GoModule+"\n")
	writeFixture(t, root, "cmd/acme/keep.txt", "existing target\n")
	target := defaults
	target.Name = "Acme Tool"
	target.BinaryName = "acme"
	target.GoModule = "github.com/acme/tool"

	for _, dryRun := range []bool{true, false} {
		_, _, err := apply(root, replacements(target), dryRun)
		if err == nil || !strings.Contains(err.Error(), "rename target already exists") {
			t.Fatalf("apply(dryRun=%t) error = %v", dryRun, err)
		}
		readme, readErr := os.ReadFile(filepath.Join(root, "README.md"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(readme) != defaults.Name+"\n" {
			t.Fatalf("apply(dryRun=%t) wrote README before preflight completed: %q", dryRun, readme)
		}
		if _, statErr := os.Stat(filepath.Join(root, "cmd", defaults.BinaryName, "main.go")); statErr != nil {
			t.Fatalf("apply(dryRun=%t) moved source before preflight completed: %v", dryRun, statErr)
		}
	}
}

func TestApplyPreflightsReservedPathCollisionsBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	original := projectconfig.Defaults.Name + "\n"
	writeFixture(t, root, "README.md", original)
	writeFixture(t, root, "README.md.bootstrap.orig", "unrelated existing file\n")
	target := projectconfig.Defaults
	target.Name = "Acme Tool"

	for _, dryRun := range []bool{true, false} {
		_, _, err := apply(root, replacements(target), dryRun)
		if err == nil || !strings.Contains(err.Error(), "reserved path already exists") {
			t.Fatalf("apply(dryRun=%t) error = %v", dryRun, err)
		}
		readme, readErr := os.ReadFile(filepath.Join(root, "README.md"))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(readme) != original {
			t.Fatalf("apply(dryRun=%t) wrote before reserved-path preflight: %q", dryRun, readme)
		}
	}
}

func TestApplyRejectsSymbolicLinksWithoutReadingOrReplacingThem(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.txt")
	if err := os.WriteFile(external, []byte(projectconfig.Defaults.Name+" private material\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink(external, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	target := projectconfig.Defaults
	target.Name = "Acme Tool"

	for _, dryRun := range []bool{true, false} {
		_, _, err := apply(root, replacements(target), dryRun)
		if err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("apply(dryRun=%t) error = %v", dryRun, err)
		}
		info, statErr := os.Lstat(link)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("apply(dryRun=%t) replaced the symbolic link", dryRun)
		}
	}
}

func TestApplyRenamesRepositoryIdentityInPaths(t *testing.T) {
	root := t.TempDir()
	defaults := projectconfig.Defaults
	writeFixture(t, root, "docs/"+defaults.GitHubRepository+"-design.md", defaults.Name+"\n")
	target := defaults
	target.Name = "Acme Tool"
	target.GitHubRepository = "acme-tool"

	changed, renames, err := apply(root, replacements(target), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || len(renames) != 1 {
		t.Fatalf("changed = %v, renames = %v", changed, renames)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "acme-tool-design.md")); err != nil {
		t.Fatalf("renamed repository identity path is missing: %v", err)
	}
}

func TestApplyConfiguredPreviewsAndCommitsProfileWithIdentity(t *testing.T) {
	root := t.TempDir()
	config := configuredFixture()
	config.PublicGuard.DocumentationLocale = "ja"
	writeConfigFixture(t, root, config)
	writeFixture(t, root, "README.md", projectconfig.Defaults.Name+"\n")

	changed, _, err := applyConfigured(root, config, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 || changed[0] != ".harness/project.json" || changed[1] != "README.md" {
		t.Fatalf("dry-run changes = %v", changed)
	}
	previewConfig, err := projectconfig.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if previewConfig.Profile != "template" {
		t.Fatalf("dry-run profile = %q", previewConfig.Profile)
	}

	if _, _, err := applyConfigured(root, config, false); err != nil {
		t.Fatal(err)
	}
	readyConfig, err := projectconfig.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if readyConfig.Profile != "ready" {
		t.Fatalf("applied profile = %q", readyConfig.Profile)
	}
	if readyConfig.PublicGuard.DocumentationLocale != "ja" {
		t.Fatalf("applied documentation locale = %q", readyConfig.PublicGuard.DocumentationLocale)
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != config.Project.Name+"\n" {
		t.Fatalf("applied README = %q", readme)
	}
}

func TestApplyConfiguredRollsBackProfileAndContentOnCommitFailure(t *testing.T) {
	root := t.TempDir()
	config := configuredFixture()
	writeConfigFixture(t, root, config)
	writeFixture(t, root, "README.md", projectconfig.Defaults.Name+"\n")

	originalRename := commitRename
	calls := 0
	commitRename = func(oldPath, newPath string) error {
		calls++
		if calls == 4 {
			return errors.New("injected commit failure")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { commitRename = originalRename })

	if _, _, err := applyConfigured(root, config, false); err == nil || !strings.Contains(err.Error(), "injected commit failure") {
		t.Fatalf("applyConfigured() error = %v", err)
	}
	restoredConfig, err := projectconfig.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if restoredConfig.Profile != "template" {
		t.Fatalf("profile after rollback = %q", restoredConfig.Profile)
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != projectconfig.Defaults.Name+"\n" {
		t.Fatalf("README after rollback = %q", readme)
	}
	assertNoBootstrapResidue(t, root)
}

func configuredFixture() projectconfig.Config {
	return projectconfig.Config{
		SchemaVersion: projectconfig.SchemaVersion,
		Profile:       "template",
		Project: projectconfig.Project{
			Name: "Acme Tool", BinaryName: "acme", GoModule: "github.com/acme/tool",
			GitHubOwner: "acme", GitHubRepository: "tool", Description: "A concise Acme command-line tool.",
			FormulaClass: "Acme", LicenseSPDX: "MIT", SecurityContact: "security@acme.example",
		},
		PublicGuard: projectconfig.PublicGuard{DocumentationLocale: "en", DenylistFile: ".harness/denylist.txt"},
	}
}

func writeConfigFixture(t *testing.T, root string, config projectconfig.Config) {
	t.Helper()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, ".harness/project.json", string(append(data, '\n')))
}

func assertNoBootstrapResidue(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(entry.Name(), ".bootstrap.tmp") || strings.HasSuffix(entry.Name(), ".bootstrap.orig") {
			t.Errorf("bootstrap residue remains: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", path, data, want)
	}
}

func writeFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
