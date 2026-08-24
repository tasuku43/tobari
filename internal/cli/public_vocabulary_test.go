package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var retiredWorkspaceResourceLanguage = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\blogical tobari\b`),
	regexp.MustCompile(`(?i)\b(?:one|each|another|existing|current-directory) tobari\b`),
	regexp.MustCompile(`(?i)\bper-tobari\b`),
	regexp.MustCompile(`(?i)\btobari (?:id|home|ready|deleted|exists)\b`),
	regexp.MustCompile(`(?i)\b(?:create|delete|enter|re-enter|reuse|inspect|list) (?:a |the )?tobari\b`),
	regexp.MustCompile(`(?i)\bremove (?:a |the )tobari\b`),
	regexp.MustCompile(`(?i)\b(?:current|this) tobari\b`),
	regexp.MustCompile(`(?i)\bin %d tobari\b`),
}

var tobariProductQualifierLanguage = regexp.MustCompile(`(?i)\btobari(?:-owned| (?:implementation|design|executable|installation|release|version|runtime|build|binary|cli|repository|command|catalog|code|ca|control|services?|image|state))\b`)

func TestPublicVocabularyKeepsTobariAsProductAndWorkspaceAsResource(t *testing.T) {
	t.Parallel()
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{"README.md"} {
		path := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		assertNoRetiredWorkspaceResourceLanguage(t, relative, string(data))
	}
	docsRoot := filepath.Join(repositoryRoot, "docs")
	if err := filepath.WalkDir(docsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			for _, excluded := range []string{
				filepath.Join("docs", "decisions"),
				filepath.Join("docs", "work"),
				filepath.Join("docs", "architecture-site", "src", "generated"),
				filepath.Join("docs", "architecture-site", "src", "content", "docs", "generated"),
			} {
				if relative == excluded {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if extension := filepath.Ext(entry.Name()); extension != ".md" && extension != ".mdx" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		assertNoRetiredWorkspaceResourceLanguage(t, filepath.ToSlash(relative), string(data))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{"internal/cli", "internal/app/tobaricmd"} {
		root := filepath.Join(repositoryRoot, filepath.FromSlash(relative))
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatalf("unquote %s: %v", filepath.Join(relative, entry.Name()), err)
				}
				assertNoRetiredWorkspaceResourceLanguage(t, filepath.Join(relative, entry.Name()), value)
				return true
			})
		}
	}
}

func TestPublicVocabularyRuleAllowsProductAndOwnershipLanguage(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"Tobari prepares the Workspace.",
		"The current Tobari executable owns Tobari-owned state.",
		"The Tobari runtime contract applies to every Workspace.",
	} {
		assertNoRetiredWorkspaceResourceLanguage(t, "allowed", text)
	}
	for _, text := range []string{
		"Create a Tobari from this project.",
		"The logical Tobari remains available.",
		"Tobari ID is diagnostic.",
		"Enter the Tobari again.",
	} {
		matched := false
		for _, pattern := range retiredWorkspaceResourceLanguage {
			matched = matched || pattern.MatchString(text)
		}
		if !matched {
			t.Errorf("retired public resource language was not rejected: %q", text)
		}
	}
}

func assertNoRetiredWorkspaceResourceLanguage(t *testing.T, path, text string) {
	t.Helper()
	text = tobariProductQualifierLanguage.ReplaceAllString(text, "product-owned")
	for _, pattern := range retiredWorkspaceResourceLanguage {
		if match := pattern.FindString(text); match != "" {
			t.Errorf("%s uses retired duplicate Workspace resource language %q", path, match)
		}
	}
}
