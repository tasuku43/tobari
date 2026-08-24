package tobari

import (
	"reflect"
	"strings"
	"testing"
)

// TestWorkspaceTemplateChangeMatrixFreezesEveryClosedMutationDimension keeps
// the five public Template configuration tasks on one pure transition model.
// Each delta is applied to the complete current body and must preserve every
// previously accepted, unrelated dimension.
func TestWorkspaceTemplateChangeMatrixFreezesEveryClosedMutationDimension(t *testing.T) {
	current := templateBodyFixture("a")

	shellValue := "xterm-256color"
	withShell, err := ApplyWorkspaceTemplateChange(current, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeShell,
		Shell: []ManifestShellEnvironmentSetting{{
			Variable: "TERM", Source: ManifestShellEnvironmentLiteral, Value: &shellValue,
		}},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	withInherit, err := ApplyWorkspaceTemplateChange(withShell, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeGit,
		Git:  &ManifestGitIdentitySetting{Source: ManifestGitIdentityInherit},
	}, nil)
	if err != nil || withInherit.SessionDefaults.GitIdentity == nil ||
		withInherit.SessionDefaults.GitIdentity.Source != ManifestGitIdentityInherit ||
		!reflect.DeepEqual(withInherit.SessionDefaults.ShellEnvironment, withShell.SessionDefaults.ShellEnvironment) {
		t.Fatalf("Git inherit transition=%+v err=%v", withInherit.SessionDefaults, err)
	}

	name, email := "Example User", "user@example.com"
	withLiteral, err := ApplyWorkspaceTemplateChange(withInherit, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeGit,
		Git:  &ManifestGitIdentitySetting{Source: ManifestGitIdentityLiteral, Name: &name, Email: &email},
	}, nil)
	if err != nil || withLiteral.SessionDefaults.GitIdentity == nil ||
		withLiteral.SessionDefaults.GitIdentity.Name == nil || *withLiteral.SessionDefaults.GitIdentity.Name != name ||
		!reflect.DeepEqual(withLiteral.SessionDefaults.ShellEnvironment, withShell.SessionDefaults.ShellEnvironment) {
		t.Fatalf("Git literal transition=%+v err=%v", withLiteral.SessionDefaults, err)
	}

	aws := testAWSBootstrap()
	withAWS, err := ApplyWorkspaceTemplateChange(withLiteral, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeBootstrapAWS,
		AWS:  &aws,
	}, nil)
	if err != nil || withAWS.CreationDefaults.Bootstrap == nil ||
		!reflect.DeepEqual(withAWS.SessionDefaults, withLiteral.SessionDefaults) ||
		!reflect.DeepEqual(withAWS.EntryDefaults, withLiteral.EntryDefaults) {
		t.Fatalf("AWS transition=%+v err=%v", withAWS, err)
	}

	eks := testEKSBootstrap(t)
	withEKS, err := ApplyWorkspaceTemplateChange(withAWS, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeBootstrapEKS,
		EKS:  &eks,
	}, nil)
	if err != nil || withEKS.CreationDefaults.Bootstrap == nil || withEKS.CreationDefaults.Bootstrap.EKS == nil ||
		!reflect.DeepEqual(withEKS.CreationDefaults.Bootstrap.AWS, withAWS.CreationDefaults.Bootstrap.AWS) ||
		!reflect.DeepEqual(withEKS.SessionDefaults, withAWS.SessionDefaults) {
		t.Fatalf("EKS transition=%+v err=%v", withEKS, err)
	}

	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	revision := "sha256:" + strings.Repeat("b", 64)
	runtimeRef := RuntimeRevisionRef(runtimeID, revision)
	binding := RuntimeBinding{RuntimeID: runtimeID, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"}
	withRuntime, err := ApplyWorkspaceTemplateChange(withEKS, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: runtimeRef,
	}, &binding)
	if err != nil || !reflect.DeepEqual(withRuntime.EntryDefaults.Runtime, binding) ||
		!reflect.DeepEqual(withRuntime.CreationDefaults, withEKS.CreationDefaults) ||
		!reflect.DeepEqual(withRuntime.SessionDefaults, withEKS.SessionDefaults) ||
		!reflect.DeepEqual(withRuntime.Policy, withEKS.Policy) || !reflect.DeepEqual(withRuntime.Boundary, withEKS.Boundary) {
		t.Fatalf("Runtime transition=%+v err=%v", withRuntime, err)
	}

	withoutGit, err := ApplyWorkspaceTemplateChange(withRuntime, WorkspaceTemplateChange{
		Kind: WorkspaceTemplateChangeGit,
		Git:  &ManifestGitIdentitySetting{Source: ManifestGitIdentityDefault},
	}, nil)
	if err != nil || withoutGit.SessionDefaults.GitIdentity != nil ||
		!reflect.DeepEqual(withoutGit.SessionDefaults.ShellEnvironment, withRuntime.SessionDefaults.ShellEnvironment) ||
		!reflect.DeepEqual(withoutGit.CreationDefaults, withRuntime.CreationDefaults) ||
		!reflect.DeepEqual(withoutGit.EntryDefaults, withRuntime.EntryDefaults) {
		t.Fatalf("Git default transition=%+v err=%v", withoutGit, err)
	}
}
