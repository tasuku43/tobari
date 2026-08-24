package workspaceauthoritycmd

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type templateBatchPort struct {
	template   tobari.WorkspaceTemplate
	calls      int
	lastRef    string
	lastChange tobari.WorkspaceTemplateChange
}

func (p *templateBatchPort) UpdateWorkspaceTemplateByReference(
	_ context.Context,
	ref string,
	change tobari.WorkspaceTemplateChange,
) (tobari.WorkspaceTemplateRevisionPublication, error) {
	p.calls++
	p.lastRef = ref
	p.lastChange = change.Clone()
	previous := p.template.Current.Clone()
	var resolved *tobari.RuntimeBinding
	if change.Kind == tobari.WorkspaceTemplateChangeRuntime {
		id, revision, err := tobari.ParseRuntimeRevisionRef(change.RuntimeRevisionRef)
		if err != nil {
			return tobari.WorkspaceTemplateRevisionPublication{}, err
		}
		value := tobari.RuntimeBinding{RuntimeID: id, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"}
		resolved = &value
	}
	nextBody, err := tobari.ApplyWorkspaceTemplateChange(previous.Body, change, resolved)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	next, changed, err := tobari.AdvanceWorkspaceTemplateRevision(previous, nextBody)
	if err != nil {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	if changed {
		p.template.Current = next.Clone()
		p.template.Retained = append(p.template.Retained, next.Clone())
	}
	return tobari.WorkspaceTemplateRevisionPublication{
		Template: p.template.Clone(), Previous: previous, Current: next,
		ResolvedRuntime: resolved, Changed: changed,
	}, nil
}

func (p *templateBatchPort) UpdateWorkspaceTemplateBootstrapByReference(
	ctx context.Context,
	ref string,
	request tobari.WorkspaceTemplateBootstrapRequest,
) (tobari.WorkspaceTemplateRevisionPublication, tobari.WorkspaceTemplateChange, error) {
	change := tobari.WorkspaceTemplateChange{Kind: request.Kind}
	if request.Kind == tobari.WorkspaceTemplateChangeBootstrapAWS && request.Action != tobari.WorkspaceTemplateBootstrapRemove {
		value := tobari.ManifestAWSBootstrap{
			Profile: request.Selector, SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
			SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"},
			AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
		}
		change.AWS = &value
	}
	publication, err := p.UpdateWorkspaceTemplateByReference(ctx, ref, change)
	return publication, change, err
}

func TestTemplateBootstrapTaskBindsExactTargetAndResolvedChange(t *testing.T) {
	port := &templateBatchPort{template: templateFixture(t)}
	service := NewTemplateService(port)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	request := tobari.WorkspaceTemplateBootstrapRequest{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, Action: tobari.WorkspaceTemplateBootstrapConfigure, Selector: "engineering"}
	impact, _ := TemplateConfigurationImpact(TaskTemplateBootstrapAWS)
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	publication, err := service.UpdateBootstrap(context.Background(), intent(TaskTemplateBootstrapAWS, operation.EffectWrite, target, impact), templateRef, request)
	if err != nil || !publication.Changed || port.calls != 1 || port.lastRef != templateRef || port.lastChange.Kind != tobari.WorkspaceTemplateChangeBootstrapAWS || port.lastChange.AWS == nil {
		t.Fatalf("publication=%+v calls=%d ref=%q change=%+v err=%v", publication, port.calls, port.lastRef, port.lastChange, err)
	}
	before := port.calls
	wrongTarget := target
	wrongTarget.ParentID = "unexpected-parent"
	if _, err := service.UpdateBootstrap(context.Background(), intent(TaskTemplateBootstrapAWS, operation.EffectWrite, wrongTarget, impact), templateRef, request); err == nil || port.calls != before {
		t.Fatalf("bootstrap accepted parent authority: calls=%d err=%v", port.calls, err)
	}
}

func TestTemplateConfigurationTaskMatrixBindsOnlyRuntimeParent(t *testing.T) {
	port := &templateBatchPort{template: templateFixture(t)}
	service := NewTemplateService(port)
	templateRef, _ := tobari.WorkspaceTemplateRef(port.template.ID)
	shellValue := "xterm-256color"
	name, email := "Example User", "user@example.com"
	aws := tobari.ManifestAWSBootstrap{
		Profile: "engineering", SSOSession: "company", SSOStartURL: "https://example.awsapps.com/start",
		SSORegion: "us-east-1", SSORegistrationScopes: []string{"sso:account:access"},
		AccountID: "123456789012", RoleName: "Developer", Region: "ap-northeast-1", Output: "json",
	}
	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	runtimeRevision := "sha256:" + strings.Repeat("b", 64)
	runtimeRef := tobari.RuntimeRevisionRef(runtimeID, runtimeRevision)

	tests := []struct {
		command string
		change  tobari.WorkspaceTemplateChange
		parent  string
	}{
		{TaskTemplateConfigShell, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &shellValue}}}, ""},
		{TaskTemplateConfigGit, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeGit, Git: &tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityInherit}}, ""},
		{TaskTemplateConfigGit, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeGit, Git: &tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityDefault}}, ""},
		{TaskTemplateConfigGit, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeGit, Git: &tobari.ManifestGitIdentitySetting{Source: tobari.ManifestGitIdentityLiteral, Name: &name, Email: &email}}, ""},
		{TaskTemplateBootstrapAWS, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeBootstrapAWS, AWS: &aws}, ""},
		{TaskTemplateBootstrapEKS, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeBootstrapEKS}, ""},
		{TaskTemplateRuntimeSet, tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: runtimeRef}, runtimeRef},
	}

	for index, test := range tests {
		impact, err := TemplateConfigurationImpact(test.command)
		if err != nil {
			t.Fatal(err)
		}
		target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef, ParentID: test.parent}
		beforeCalls := port.calls
		publication, err := service.UpdateConfiguration(context.Background(), intent(test.command, operation.EffectWrite, target, impact), templateRef, test.change)
		if err != nil || port.calls != beforeCalls+1 || port.lastRef != templateRef || port.lastChange.Kind != test.change.Kind {
			t.Fatalf("case %d command=%q publication=%+v calls=%d change=%+v err=%v", index, test.command, publication, port.calls, port.lastChange, err)
		}
	}

	beforeCalls := port.calls
	impact, _ := TemplateConfigurationImpact(TaskTemplateConfigShell)
	wrongParent := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef, ParentID: runtimeRef}
	if _, err := service.UpdateConfiguration(context.Background(), intent(TaskTemplateConfigShell, operation.EffectWrite, wrongParent, impact), templateRef, tests[0].change); err == nil || port.calls != beforeCalls {
		t.Fatalf("non-Runtime task accepted Runtime parent: calls=%d err=%v", port.calls, err)
	}
	impact, _ = TemplateConfigurationImpact(TaskTemplateRuntimeSet)
	missingParent := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	if _, err := service.UpdateConfiguration(context.Background(), intent(TaskTemplateRuntimeSet, operation.EffectWrite, missingParent, impact), templateRef, tests[len(tests)-1].change); err == nil || port.calls != beforeCalls {
		t.Fatalf("Runtime task omitted exact parent: calls=%d err=%v", port.calls, err)
	}
}
