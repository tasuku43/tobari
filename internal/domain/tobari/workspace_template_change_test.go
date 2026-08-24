package tobari

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceTemplateChangesApplyOnlyTheirClosedDimension(t *testing.T) {
	current := templateBodyFixture("a")
	shellValue := "xterm-256color"
	shell := WorkspaceTemplateChange{Kind: WorkspaceTemplateChangeShell, Shell: []ManifestShellEnvironmentSetting{{Variable: "TERM", Source: ManifestShellEnvironmentLiteral, Value: &shellValue}}}
	withShell, err := ApplyWorkspaceTemplateChange(current, shell, nil)
	if err != nil {
		t.Fatal(err)
	}
	name, email := "Example User", "user@example.com"
	git := WorkspaceTemplateChange{Kind: WorkspaceTemplateChangeGit, Git: &ManifestGitIdentitySetting{Source: ManifestGitIdentityLiteral, Name: &name, Email: &email}}
	withBoth, err := ApplyWorkspaceTemplateChange(withShell, git, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withBoth.SessionDefaults.ShellEnvironment, withShell.SessionDefaults.ShellEnvironment) ||
		withBoth.SessionDefaults.GitIdentity == nil || withBoth.Policy.BaselineGrants[0].Path != current.Policy.BaselineGrants[0].Path {
		t.Fatalf("typed Git delta changed unrelated authority: %#v", withBoth)
	}

	lastName, lastEmail := "Last User", "last@example.com"
	last := WorkspaceTemplateChange{Kind: WorkspaceTemplateChangeGit, Git: &ManifestGitIdentitySetting{Source: ManifestGitIdentityLiteral, Name: &lastName, Email: &lastEmail}}
	withLast, err := ApplyWorkspaceTemplateChange(withBoth, last, nil)
	if err != nil || withLast.SessionDefaults.GitIdentity == nil || *withLast.SessionDefaults.GitIdentity.Name != lastName ||
		!reflect.DeepEqual(withLast.SessionDefaults.ShellEnvironment, withBoth.SessionDefaults.ShellEnvironment) {
		t.Fatalf("same-field last successful delta=%#v err=%v", withLast.SessionDefaults, err)
	}
}

func TestWorkspaceTemplateRuntimeChangeRequiresExactResolvedRevision(t *testing.T) {
	current := templateBodyFixture("a")
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	revision := "sha256:" + strings.Repeat("b", 64)
	ref := RuntimeRevisionRef(runtimeID, revision)
	change := WorkspaceTemplateChange{Kind: WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: ref}
	binding := RuntimeBinding{RuntimeID: runtimeID, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"}
	next, err := ApplyWorkspaceTemplateChange(current, change, &binding)
	if err != nil || !reflect.DeepEqual(next.EntryDefaults.Runtime, binding) {
		t.Fatalf("resolved Runtime=%#v err=%v", next.EntryDefaults.Runtime, err)
	}
	if _, err := ApplyWorkspaceTemplateChange(current, change, nil); err == nil {
		t.Fatal("Runtime change accepted no resolved authority")
	}
	wrong := binding
	wrong.Revision = "sha256:" + strings.Repeat("c", 64)
	if _, err := ApplyWorkspaceTemplateChange(current, change, &wrong); err == nil {
		t.Fatal("Runtime change accepted another resolved revision")
	}
	publication := WorkspaceTemplateRevisionPublication{ResolvedRuntime: &binding}
	encoded, err := json.Marshal(publication)
	if err != nil || strings.Contains(string(encoded), "ResolvedRuntime") || strings.Contains(string(encoded), "resolved_runtime") {
		t.Fatalf("private resolved Runtime leaked through JSON: %s err=%v", encoded, err)
	}
}

func TestWorkspaceTemplateChangeCloneIsolatesNestedInput(t *testing.T) {
	value := "xterm"
	change := WorkspaceTemplateChange{Kind: WorkspaceTemplateChangeShell, Shell: []ManifestShellEnvironmentSetting{{Variable: "TERM", Source: ManifestShellEnvironmentLiteral, Value: &value}}}
	clone := change.Clone()
	*clone.Shell[0].Value = "screen"
	if *change.Shell[0].Value != value {
		t.Fatal("Workspace Template change clone shares nested input")
	}
}
