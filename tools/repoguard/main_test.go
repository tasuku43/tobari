package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/tools/internal/projectconfig"
)

func TestRepositoryPathsSkipsTrackedDeletionAndKeepsUntrackedRenameDestination(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	writeRepositoryFixture(t, root, "old-path.txt", "public fixture\n")
	runGit(t, root, "add", "old-path.txt")
	if err := os.Rename(filepath.Join(root, "old-path.txt"), filepath.Join(root, "new-path.txt")); err != nil {
		t.Fatal(err)
	}

	status := runGit(t, root, "status", "--short", "--untracked-files=all")
	if !strings.Contains(status, "old-path.txt") || !strings.Contains(status, "new-path.txt") {
		t.Fatalf("working tree does not contain the deletion/destination reproduction:\n%s", status)
	}

	paths, err := repositoryPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if containsRepositoryPath(paths, "old-path.txt") {
		t.Fatalf("repositoryPaths() retained missing tracked deletion: %v", paths)
	}
	if !containsRepositoryPath(paths, "new-path.txt") {
		t.Fatalf("repositoryPaths() omitted untracked rename destination: %v", paths)
	}
	if err := validateRepositoryPaths(root, paths); err != nil {
		t.Fatalf("validateRepositoryPaths() rejected valid rename state: %v", err)
	}
}

func TestRepositoryPathsFailsClosedWhenGitEnumerationFails(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "README.md", "not a Git repository\n")
	if paths, err := repositoryPaths(root); err == nil || !strings.Contains(err.Error(), "git ls-files") {
		t.Fatalf("repositoryPaths() = %v, %v; want Git enumeration error", paths, err)
	}
}

func TestCheckTextDetectsPublicLeaksAndUnsafeSecrets(t *testing.T) {
	config := projectconfig.Config{Profile: "ready"}
	text := strings.Join([]string{
		"home=/Users" + "/alice/private",
		"docs=https://service." + "corp/runbook",
		"api_" + "key=real-production-value",
		"TODO_" + "TEMPLATE",
		"internal-ticket-123",
	}, "\n")
	issues := checkText("README.md", text, config, []string{"internal-ticket"}, "public")
	if len(issues) != 5 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestCheckTextAppliesTheConfiguredDocumentationLocale(t *testing.T) {
	english := projectconfig.Config{PublicGuard: projectconfig.PublicGuard{DocumentationLocale: "en"}}
	issues := checkText("README.md", "日本語の説明", english, nil, "public")
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "documentation_locale") {
		t.Fatalf("English locale issues = %#v", issues)
	}
	englishThreeLetter := projectconfig.Config{PublicGuard: projectconfig.PublicGuard{DocumentationLocale: "eng"}}
	if issues := checkText("README.md", "日本語の説明", englishThreeLetter, nil, "public"); len(issues) != 1 {
		t.Fatalf("three-letter English locale issues = %#v", issues)
	}

	japanese := projectconfig.Config{PublicGuard: projectconfig.PublicGuard{DocumentationLocale: "ja"}}
	if issues := checkText("README.md", "日本語の説明", japanese, nil, "public"); len(issues) != 0 {
		t.Fatalf("Japanese locale issues = %#v", issues)
	}
	if issues := checkText("fixture.json", "日本語の外部データ", english, nil, "public"); len(issues) != 0 {
		t.Fatalf("non-Markdown external data was treated as documentation prose: %#v", issues)
	}
	for path, text := range map[string]string{
		"README.md":        "```json\n{\"provider_text\":\"日本語\"}\n```",
		"docs/provider.md": "> 日本語の引用データ",
	} {
		if issues := checkText(path, text, english, nil, "public"); len(issues) != 0 {
			t.Fatalf("external text %q was treated as trusted prose: %#v", path, issues)
		}
	}
	if issues := checkText("docs/work/active/context.md", "日本語の作業中説明", english, nil, "public"); len(issues) != 1 {
		t.Fatalf("active work prose issues = %#v", issues)
	}
	if issues := checkTextWithLocaleExemption("docs/work/complete/context.md", "日本語の歴史的証拠", english, nil, "public", true); len(issues) != 0 {
		t.Fatalf("historical work evidence was treated as trusted prose: %#v", issues)
	}
	for _, text := range []string{
		"Use `日本語の識別子` exactly.",
		"Use ``multi-line\n日本語の識別子`` exactly.",
		"See [English label](https://example.com/日本語).",
		"See [English label](https://example.com/a(日本語)).",
		"[reference]: docs/日本語.md",
	} {
		if issues := checkText("README.md", text, english, nil, "public"); len(issues) != 0 {
			projected, visible := projectDocumentationLocale(strings.Split(text, "\n"))
			t.Fatalf("non-prose Markdown %q issues = %#v; projected = %q, visible = %v", text, issues, projected, visible)
		}
	}
	for _, text := range []string{
		"Use `unclosed 日本語 prose.",
		"See [日本語のラベル](https://example.com/reference).",
		`See \[label](日本語の説明).`,
		`\[reference]: 日本語の説明`,
		`See [English label](https://example.com/reference "日本語のタイトル").`,
	} {
		if issues := checkText("README.md", text, english, nil, "public"); len(issues) != 1 {
			t.Fatalf("trusted Markdown prose %q issues = %#v", text, issues)
		}
	}
	issues = checkText("README.md", "trusted prose\n\n日本語の説明\n", english, nil, "public")
	if len(issues) != 1 || issues[0].Line != 3 {
		t.Fatalf("trusted prose issues = %#v", issues)
	}
}

func TestHistoricalWorkPacketLocaleExemptionsFollowTerminalStatus(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"docs/work/active/context.md",
		"docs/work/active/goal.md",
		"docs/work/complete/context.md",
		"docs/work/complete/goal.md",
		"docs/work/draft/goal.md",
		"docs/work/malformed/goal.md",
		"docs/work/superseded/goal.md",
	}
	writeRepositoryFixture(t, root, "docs/work/active/goal.md", workGoal("Active", "", "- [ ] In progress\n"))
	writeRepositoryFixture(t, root, "docs/work/active/context.md", "日本語\n")
	writeRepositoryFixture(t, root, "docs/work/complete/goal.md", workGoal("Complete", "", "- [x] Done\n"))
	writeRepositoryFixture(t, root, "docs/work/complete/context.md", "日本語\n")
	writeRepositoryFixture(t, root, "docs/work/draft/goal.md", workGoal("Draft", "", "- [ ] Pending\n"))
	writeRepositoryFixture(t, root, "docs/work/malformed/goal.md", "# Missing metadata\n")
	writeRepositoryFixture(t, root, "docs/work/superseded/goal.md", workGoal("Superseded", "../active/goal.md", "- [x] Historical\n"))

	exemptions, err := historicalWorkPacketLocaleExemptions(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"docs/work/complete/context.md", "docs/work/superseded/goal.md"} {
		if !workPacketLocaleExempt(path, exemptions) {
			t.Errorf("%s did not receive its historical exemption", path)
		}
	}
	for _, path := range []string{"docs/work/active/context.md", "docs/work/draft/goal.md", "docs/work/malformed/goal.md", "README.md"} {
		if workPacketLocaleExempt(path, exemptions) {
			t.Errorf("%s received a locale exemption", path)
		}
	}
}

func TestCheckTextDetectsQuotedJSONSecretsAndMarkerSubstrings(t *testing.T) {
	config := projectconfig.Config{Profile: "template"}
	unsafe := jsonSecretAssignment("client_"+"secret", "prod-contest-value")
	issues := checkText("config.json", unsafe, config, nil, "security")
	if len(issues) != 1 || issues[0].Message != "secret-like value assigned in source" {
		t.Fatalf("unsafe JSON issues = %#v", issues)
	}

	for _, value := range []string{
		jsonSecretAssignment("client_"+"secret", "dummy-value"),
		jsonSecretAssignment("access_"+"token", "${ACCESS_TOKEN}"),
		jsonSecretAssignment("pass"+"word", "env.PASSWORD"),
		jsonSecretAssignment("pass"+"word", "[redacted]"),
	} {
		if issues := checkText("config.json", value, config, nil, "security"); len(issues) != 0 {
			t.Errorf("safe example %q issues = %#v", value, issues)
		}
	}

	for _, value := range []string{
		jsonSecretAssignment("client_"+"secret", "production-dummy"),
		jsonSecretAssignment("client_"+"secret", "contest-token"),
		jsonSecretAssignment("client_"+"secret", "env.PASSWORD-extra"),
	} {
		issues := checkText("config.json", value, config, nil, "security")
		if len(issues) != 1 || issues[0].Message != "secret-like value assigned in source" {
			t.Errorf("embedded marker %q issues = %#v", value, issues)
		}
	}
}

func TestCheckSecretsDetectsAuthorizationHeadersAndCredentialURLs(t *testing.T) {
	header := "Authorization: Bearer " + "liveToken123"
	issues := checkSecrets("fixture.txt", header, 1)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "authorization header") {
		t.Fatalf("header issues = %#v", issues)
	}
	if issues := checkSecrets("fixture.txt", "Authorization: Bearer dummy-token", 1); len(issues) != 0 {
		t.Fatalf("example header issues = %#v", issues)
	}
	credentialURL := "https://user:" + "live-password@example.com/resource"
	issues = checkSecrets("fixture.txt", credentialURL, 1)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "credential-bearing URL") {
		t.Fatalf("URL issues = %#v", issues)
	}
}

func TestCheckTextRejectsReadyTemplateIdentityOutsideDefaults(t *testing.T) {
	config := projectconfig.Config{
		Profile: "ready",
		Project: projectconfig.Project{
			Name: "Acme Tool", BinaryName: "acme", GoModule: "github.com/acme/tool",
			GitHubOwner: "acme", GitHubRepository: "tool", Description: "An Acme tool.",
			FormulaClass: "Acme", SecurityContact: "security@acme.example",
		},
	}
	issues := checkText("README.md", "module "+projectconfig.Defaults.GoModule, config, nil, "public")
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "template identity") {
		t.Fatalf("identity issues = %#v", issues)
	}
	if issues := checkText("tools/internal/projectconfig/defaults.go", projectconfig.Defaults.GoModule, config, nil, "public"); len(issues) != 0 {
		t.Fatalf("protected defaults issues = %#v", issues)
	}
}

func TestCheckTextAllowsConfiguredIdentityContainingTemplateSubstring(t *testing.T) {
	config := projectconfig.Config{
		Profile: "ready",
		Project: projectconfig.Project{
			Name: "Acme Agentic CLI Foundry", BinaryName: "acme-agentic-cli-foundry", GoModule: "github.com/acme/acme-agentic-cli-foundry",
			GitHubOwner: "acme", GitHubRepository: "acme-agentic-cli-foundry", Description: "An Acme CLI template.",
			FormulaClass: "AcmeAgenticCliFoundry", SecurityContact: "security@acme.example",
		},
	}
	line := "github.com/acme/acme-agentic-cli-foundry acme-agentic-cli-foundry AcmeAgenticCliFoundry"
	if issues := checkText("README.md", line, config, nil, "public"); len(issues) != 0 {
		t.Fatalf("configured identity issues = %#v", issues)
	}
	line += " " + projectconfig.Defaults.GoModule
	if issues := checkText("README.md", line, config, nil, "public"); len(issues) != 1 {
		t.Fatalf("residual identity issues = %#v", issues)
	}
}

func TestValidateRepositoryPathsRejectsSymbolicLinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("public fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked.txt")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := validateRepositoryPaths(root, []string{"linked.txt"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("validateRepositoryPaths() error = %v", err)
	}
}

func TestValidateRepositoryPathsRejectsSymbolicDirectoryComponents(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "fixture.txt"), []byte("external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "linked-dir")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if err := validateRepositoryPaths(root, []string{"linked-dir/fixture.txt"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("validateRepositoryPaths() error = %v", err)
	}
}

func TestCheckWorkPacketsAcceptsLifecycleStatuses(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/draft/goal.md", workGoal("Draft", "", "- [ ] Pending\n"))
	writeRepositoryFixture(t, root, "docs/work/accepted/goal.md", workGoal("Accepted", "", "- [ ] Approved but not active\n"))
	writeRepositoryFixture(t, root, "docs/work/active/goal.md", workGoal("Active", "", "- [ ] In progress\n"))
	completeGoal := strings.Replace(workGoal("Complete", "", "- [x] Proven <!-- - [ ] Inline example only -->\n\n```md\n+ [ ] Fenced example only\n```\n"), "## Acceptance criteria", "## Acceptance criteria <!-- inline heading comment -->", 1)
	writeRepositoryFixture(t, root, "docs/work/complete/goal.md", completeGoal)
	writeRepositoryFixture(t, root, "docs/work/complete/tasks.md", "# Tasks\n\n* [x] Done\n10. [x] Document an example\n\n    ```md\n    - [ ] Nested fenced example only\n    ```\n")
	writeRepositoryFixture(t, root, "docs/work/replacement/goal.md", workGoal("Draft", "", "- [ ] Replacement\n"))
	writeRepositoryFixture(t, root, "docs/work/old/goal.md", workGoal("Superseded", "../replacement/goal.md", "- [ ] Historical\n")+"\n- Successor: ../body-fake/goal.md\n")
	paths := []string{
		"docs/work/accepted/goal.md",
		"docs/work/active/goal.md",
		"docs/work/complete/goal.md",
		"docs/work/complete/tasks.md",
		"docs/work/draft/goal.md",
		"docs/work/old/goal.md",
		"docs/work/replacement/goal.md",
	}
	issues, err := checkWorkPackets(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("valid work packet issues = %#v", issues)
	}
}

func TestCheckWorkPacketsReadsMetadataOnlyFromFirstH1HeaderBlock(t *testing.T) {
	root := t.TempDir()
	valid := "```md\n# Fake work goal\n- Status: Paused\n```\n\n# Work Goal\n\n- Status: Draft\n- Successor: None\n\n## Outcome\n\n- Status: Paused\n- Successor: ../body-fake/goal.md\n\n## Acceptance criteria\n\n- [ ] Pending\n"
	writeRepositoryFixture(t, root, "docs/work/valid/goal.md", valid)
	hidden := "# Work Goal\n\n<!--\n- Status: Draft\n- Successor: None\n-->\n\n## Acceptance criteria\n\n- [ ] Pending\n"
	writeRepositoryFixture(t, root, "docs/work/hidden/goal.md", hidden)
	detached := "# Work Goal\n\nIntro before metadata.\n\n- Status: Draft\n\n## Acceptance criteria\n\n- [ ] Pending\n"
	writeRepositoryFixture(t, root, "docs/work/detached/goal.md", detached)

	issues, err := checkWorkPackets(root, []string{
		"docs/work/detached/goal.md",
		"docs/work/hidden/goal.md",
		"docs/work/valid/goal.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("metadata boundary issues = %#v", issues)
	}
	for _, item := range issues {
		if item.Path == "docs/work/valid/goal.md" || !strings.Contains(item.Message, "exactly one Status") {
			t.Errorf("unexpected metadata issue = %#v", item)
		}
	}
}

func TestCheckWorkPacketsDoesNotTreatUnicodeWhitespaceAsMetadataSeparation(t *testing.T) {
	root := t.TempDir()
	goal := "# Work Goal\n\n\u00a0\n- Status: Complete\n\n## Acceptance criteria\n\n- [x] Hidden behind a nonblank paragraph\n"
	writeRepositoryFixture(t, root, "docs/work/unicode-metadata/goal.md", goal)
	issues, err := checkWorkPackets(root, []string{"docs/work/unicode-metadata/goal.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "exactly one Status") {
		t.Fatalf("Unicode metadata issues = %#v", issues)
	}
}

func TestCheckWorkPacketsRejectsCompletionMarkersHiddenInHTMLComments(t *testing.T) {
	root := t.TempDir()
	goal := "# Work Goal\n\n- Status: Complete\n\n<!--\n## Acceptance criteria\n- [x] Hidden acceptance\n-->\n<!-- inline prefix -->## Acceptance criteria\n<!-- inline prefix -->- [x] Hidden acceptance\n\n## Completion definition\n"
	writeRepositoryFixture(t, root, "docs/work/hidden/goal.md", goal)
	writeRepositoryFixture(t, root, "docs/work/hidden/tasks.md", "# Tasks\n\n<!-- - [x] Hidden task -->\n<!-- inline prefix -->- [x] Hidden task\n")
	issues, err := checkWorkPackets(root, []string{"docs/work/hidden/goal.md", "docs/work/hidden/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	joined := workIssueMessages(issues)
	for _, want := range []string{"must contain acceptance criteria checkboxes", "tasks.md must contain task checkboxes"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues do not contain %q: %#v", want, issues)
		}
	}
}

func TestCheckWorkPacketsDoesNotStartCommentsFromCodeOrEscapedLiterals(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/comment-literals/goal.md", workGoal("Complete", "", "- [x] Proven\n"))
	tasks := "# Tasks\n\n- [x] Explain literal `<!--`\n- [x] Explain escaped \\<!--\n- [x] Explain multiline `literal\n  <!--`\n\nTop-level indented example follows.\n\n    <!--\n- [ ] Pending visible task\n"
	writeRepositoryFixture(t, root, "docs/work/comment-literals/tasks.md", tasks)
	issues, err := checkWorkPackets(root, []string{
		"docs/work/comment-literals/goal.md",
		"docs/work/comment-literals/tasks.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Line != 11 || !strings.Contains(issues[0].Message, "unchecked task") {
		t.Fatalf("literal comment issues = %#v", issues)
	}
}

func TestCheckWorkPacketsDoesNotTreatUnicodeWhitespaceAsFenceClose(t *testing.T) {
	root := t.TempDir()
	acceptance := "- [x] Proven\n\n```md\n```\u00a0\n- [x] Hidden checked example\n```\n- [ ] Real unchecked acceptance\n"
	writeRepositoryFixture(t, root, "docs/work/unicode-fence/goal.md", workGoal("Complete", "", acceptance))
	writeRepositoryFixture(t, root, "docs/work/unicode-fence/tasks.md", "# Tasks\n\n- [x] Done\n")
	issues, err := checkWorkPackets(root, []string{"docs/work/unicode-fence/goal.md", "docs/work/unicode-fence/tasks.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unchecked acceptance criterion") {
		t.Fatalf("Unicode fence issues = %#v", issues)
	}
}

func TestCheckWorkPacketsUsesCommonMarkTabColumnsForListsAndFences(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/tab-columns/goal.md", workGoal("Complete", "", "- [x] Proven\n"))
	tasks := "# Tasks\n\n- [x] Done\n-\t[ ] Tab-separated task\n\n10. [x] Document fenced example\n\n    ```md\n    example\n\t```\n- [ ] Visible after tab-indented close\n"
	writeRepositoryFixture(t, root, "docs/work/tab-columns/tasks.md", tasks)
	issues, err := checkWorkPackets(root, []string{
		"docs/work/tab-columns/goal.md",
		"docs/work/tab-columns/tasks.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Line != 4 || issues[1].Line != 11 {
		t.Fatalf("tab-column issues = %#v", issues)
	}
	for _, item := range issues {
		if !strings.Contains(item.Message, "unchecked task") {
			t.Errorf("unexpected tab-column issue = %#v", item)
		}
	}
}

func TestCheckWorkPacketsNormalizesTabsRelativeToListContainerForFences(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/relative-tab/goal.md", workGoal("Complete", "", "- [x] Proven\n"))
	tasks := "# Tasks\n\n- Example:\n\n  \t```md\n  \t- [x] Fenced example only\n  \t```\n"
	writeRepositoryFixture(t, root, "docs/work/relative-tab/tasks.md", tasks)
	issues, err := checkWorkPackets(root, []string{
		"docs/work/relative-tab/goal.md",
		"docs/work/relative-tab/tasks.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "tasks.md must contain task checkboxes") {
		t.Fatalf("relative-tab fence issues = %#v", issues)
	}
}

func TestCheckWorkPacketsRejectsUnknownAndInconsistentCompleteState(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/unknown/goal.md", workGoal("Paused", "", "- [x] Done\n"))
	writeRepositoryFixture(t, root, "docs/work/incomplete/goal.md", workGoal("Complete", "", "- [ ] Acceptance remains\n"))
	writeRepositoryFixture(t, root, "docs/work/incomplete/tasks.md", "# Tasks\n\n- [x] Done\n+ [ ] Still open\n1) [ ] Ordered work\n\nTop-level example follows.\n\n    ```\n* [ ] Must not be hidden by an indented pseudo-fence\n")
	paths := []string{
		"docs/work/incomplete/goal.md",
		"docs/work/incomplete/tasks.md",
		"docs/work/unknown/goal.md",
	}
	issues, err := checkWorkPackets(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	joined := workIssueMessages(issues)
	for _, want := range []string{"unchecked acceptance criterion", "unchecked task", "Draft, Accepted, Active, Complete, or Superseded"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues do not contain %q: %#v", want, issues)
		}
	}
	pseudoFenceItemFound := false
	for _, item := range issues {
		if item.Path == "docs/work/incomplete/tasks.md" && item.Line == 10 && strings.Contains(item.Message, "unchecked task") {
			pseudoFenceItemFound = true
		}
	}
	if !pseudoFenceItemFound {
		t.Errorf("top-level four-space pseudo-fence hid its following task: %#v", issues)
	}
}

func TestCheckWorkPacketsChecksEveryVisibleAcceptanceSection(t *testing.T) {
	root := t.TempDir()
	goal := workGoal("Complete", "", "- [x] First acceptance\n") + "\n## Notes\n\nVisible notes.\n\n## Acceptance criteria\n\n- [ ] Later acceptance remains\n"
	writeRepositoryFixture(t, root, "docs/work/repeated-acceptance/goal.md", goal)
	writeRepositoryFixture(t, root, "docs/work/repeated-acceptance/tasks.md", "# Tasks\n\n- [x] Done\n")
	issues, err := checkWorkPackets(root, []string{
		"docs/work/repeated-acceptance/goal.md",
		"docs/work/repeated-acceptance/tasks.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "unchecked acceptance criterion") {
		t.Fatalf("repeated acceptance issues = %#v", issues)
	}
}

func TestCheckWorkPacketsRejectsTemplateSuccessorsAndCycles(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/_template/goal.md", workGoal("Draft", "", "- [ ] Template\n"))
	writeRepositoryFixture(t, root, "docs/work/template-target/goal.md", workGoal("Superseded", "../_template/goal.md", "- [x] Historical\n"))
	writeRepositoryFixture(t, root, "docs/work/cycle-a/goal.md", workGoal("Superseded", "../cycle-b/goal.md", "- [x] Historical\n"))
	writeRepositoryFixture(t, root, "docs/work/cycle-b/goal.md", workGoal("Superseded", "../cycle-a/goal.md", "- [x] Historical\n"))
	paths := []string{
		"docs/work/_template/goal.md",
		"docs/work/cycle-a/goal.md",
		"docs/work/cycle-b/goal.md",
		"docs/work/template-target/goal.md",
	}
	issues, err := checkWorkPackets(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	joined := workIssueMessages(issues)
	for _, want := range []string{"non-template", "contains a cycle"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues do not contain %q: %#v", want, issues)
		}
	}
}

func TestCheckWorkPacketsRequiresExplicitRegularSupersedingGoal(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/missing/goal.md", workGoal("Superseded", "None", "- [x] Historical\n"))
	writeRepositoryFixture(t, root, "docs/work/broken/goal.md", workGoal("Superseded", "../absent/goal.md", "- [x] Historical\n"))
	paths := []string{"docs/work/broken/goal.md", "docs/work/missing/goal.md"}
	issues, err := checkWorkPackets(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	joined := workIssueMessages(issues)
	for _, want := range []string{"explicit Successor path", "is not a repository work goal"} {
		if !strings.Contains(joined, want) {
			t.Errorf("issues do not contain %q: %#v", want, issues)
		}
	}

	external := filepath.Join(t.TempDir(), "goal.md")
	if err := os.WriteFile(external, []byte(workGoal("Draft", "", "- [ ] External\n")), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "docs", "work", "linked", "goal.md")
	if err := os.MkdirAll(filepath.Dir(linked), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, linked); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := checkWorkPackets(root, []string{"docs/work/linked/goal.md"}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symlink work packet error = %v", err)
	}
}

func TestCheckWorkPacketsRejectsMarkdownSuccessorSyntax(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "docs/work/replacement/goal.md", workGoal("Draft", "", "- [ ] Replacement\n"))
	writeRepositoryFixture(t, root, "docs/work/linked/goal.md", workGoal("Superseded", "[replacement](../replacement/goal.md)", "- [x] Historical\n"))
	writeRepositoryFixture(t, root, "docs/work/unicode/goal.md", workGoal("Superseded", "../replacement/goal.md\u00a0", "- [x] Historical\n"))
	paths := []string{"docs/work/linked/goal.md", "docs/work/replacement/goal.md", "docs/work/unicode/goal.md"}
	issues, err := checkWorkPackets(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("Markdown successor issues = %#v", issues)
	}
	for _, item := range issues {
		if !strings.Contains(item.Message, "canonical") {
			t.Errorf("unexpected successor issue = %#v", item)
		}
	}
}

func TestCheckTextAllowsDocumentedExamplesAndReleasePlaceholders(t *testing.T) {
	config := projectconfig.Config{Profile: "template"}
	if issues := checkText("example.env", "api_key=dummy-value", config, nil, "security"); len(issues) != 0 {
		t.Fatalf("example issues = %#v", issues)
	}
	marker := "@" + "@"
	formula := "url \"" + marker + "MACOS_ARM64_URL" + marker + "\"\nsha256 \"" + marker + "MACOS_ARM64_SHA256" + marker + "\""
	if issues := checkText("Formula/agentic-cli-foundry.rb.template", formula, config, nil, "public"); len(issues) != 0 {
		t.Fatalf("formula issues = %#v", issues)
	}
}

func TestCheckFilesystemShapeRejectsClaudePolicyAndRootBuildArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "agentic-cli-foundry"), []byte("binary fixture\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	config := projectconfig.Config{Project: projectconfig.Project{BinaryName: "agentic-cli-foundry"}}
	issues, err := checkFilesystemShape(root, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("issues = %#v", issues)
	}
	messages := issues[0].Message + "\n" + issues[1].Message
	for _, expected := range []string{"Claude-specific", "root build artifact"} {
		if !strings.Contains(messages, expected) {
			t.Errorf("issues do not contain %q: %#v", expected, issues)
		}
	}
}

func TestCheckFilesystemShapeDoesNotTreatIgnoredLocalFilesAsPublishable(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".env", ".DS_Store"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("local-only\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	config := projectconfig.Config{Project: projectconfig.Project{BinaryName: "agentic-cli-foundry"}}
	issues, err := checkFilesystemShape(root, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("ignored local files became shape issues: %#v", issues)
	}
}

func TestCheckAgentHarnessRequiresRepositorySkills(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, ".agents/skills/bootstrap-derived-cli/SKILL.md", "# Skill\n")
	writeRepositoryFixture(t, root, ".agents/skills/bootstrap-derived-cli/agents/openai.yaml", "interface: {}\n")
	writeRepositoryFixture(t, root, ".agents/skills/add-capability/SKILL.md", "# Skill\n")

	if issues := checkAgentHarness(root); len(issues) != 0 {
		t.Fatalf("valid harness issues = %#v", issues)
	}

	if err := os.Remove(filepath.Join(root, ".agents", "skills", "add-capability", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if issues := checkAgentHarness(root); len(issues) != 1 || !strings.Contains(issues[0].Message, "required Codex harness file") {
		t.Fatalf("missing skill issues = %#v", issues)
	}
}

func TestCheckPathRejectsParallelAgentPolicyFiles(t *testing.T) {
	config := projectconfig.Config{Project: projectconfig.Project{BinaryName: "agentic-cli-foundry"}}
	for _, path := range []string{"CLAUDE.md", "docs/Claude.md"} {
		issues := checkWorkingTreeArtifact(path, config)
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "Claude-specific") {
			t.Errorf("checkPath(%q) = %#v", path, issues)
		}
	}
}

func TestCheckMarkdownLinksAllowsPublishableFilesAndSkipsExternalTargets(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "README.md", "# Root\n")
	writeRepositoryFixture(t, root, "docs/guide.md", strings.Join([]string{
		"[root](../README.md#root)",
		"[external](https://example.com/docs)",
		"[mail](mailto:security@agentic-cli-foundry.example)",
		"[same page](#section)",
		"[root reference]: ../README.md",
		"```text",
		"[example only](missing.md)",
		"```",
	}, "\n"))
	paths := []string{"README.md", "docs/guide.md"}
	issues, err := checkMarkdownLinks(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestCheckMarkdownLinksRejectsUnsafeOrUnpublishedTargets(t *testing.T) {
	root := t.TempDir()
	writeRepositoryFixture(t, root, "README.md", "# Root\n")
	writeRepositoryFixture(t, root, "docs/bad.md", strings.Join([]string{
		"[missing](missing.md)",
		"[escape](../../outside.md)",
		"[absolute](/README.md)",
		"[noncanonical](../docs/../README.md)",
		"[directory](../fixtures/)",
		"[link](../linked.md)",
	}, "\n"))
	if err := os.Mkdir(filepath.Join(root, "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	paths := []string{"README.md", "docs/bad.md", "linked.md"}
	issues, err := checkMarkdownLinks(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 6 {
		t.Fatalf("issues = %#v", issues)
	}
	messages := make([]string, 0, len(issues))
	for _, item := range issues {
		messages = append(messages, item.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, expected := range []string{"publishable regular file", "escapes the repository", "repository-relative", "not canonical", "symbolic link"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("issues do not contain %q: %#v", expected, issues)
		}
	}
}

func writeRepositoryFixture(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func workGoal(status, successor, acceptance string) string {
	metadata := "- Status: " + status + "\n"
	if successor != "" {
		metadata += "- Successor: " + successor + "\n"
	}
	return "# Work Goal\n\n" + metadata + "\n## Acceptance criteria\n\n" + acceptance + "\n## Completion definition\n"
}

func workIssueMessages(issues []issue) string {
	values := make([]string, len(issues))
	for index, item := range issues {
		values[index] = item.Message
	}
	return strings.Join(values, "\n")
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func containsRepositoryPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func jsonSecretAssignment(name, value string) string {
	return `{"` + name + `":"` + value + `"}`
}
