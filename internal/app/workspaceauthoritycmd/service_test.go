package workspaceauthoritycmd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	templateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	contextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a2"
	workspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
)

func digest(c string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(c, 64))
}

func bodyFixture(path string) tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: path}}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: tobari.OfficialRuntimeBase}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}

func templateFixture(t *testing.T) tobari.WorkspaceTemplate {
	t.Helper()
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, bodyFixture("/items"))
	if err != nil {
		t.Fatal(err)
	}
	return tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "restricted", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
}

func snapshotFixture(t *testing.T, workspace, active bool) tobari.ContextAuthoritySnapshot {
	t.Helper()
	template := templateFixture(t)
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, ProjectRoot: "/workspace/example", TemplateID: templateID}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := tobari.ContextAuthoritySnapshot{Context: binding, Template: template, PolicyMemory: memory}
	if active {
		tr := tobari.TemplatePolicyActivationReceipt{ContextID: contextID, TemplateID: templateID, PolicySliceDigest: template.Current.Slices.PolicySliceDigest}
		mr := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
		result.ActiveTemplatePolicy, result.ActivePolicyMemory, result.ActivePolicyMemoryRef = &tr, ptrMemory(memory), &mr
	}
	if workspace {
		applied := tobari.WorkspaceAppliedEntry{ContextID: contextID, TemplateID: templateID, TemplateRevision: template.Current.Revision, EntrySliceDigest: template.Current.Slices.EntrySliceDigest, RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: template.Current.Slices.RuntimeRevision, ResolvedSpec: digest("7"), ReconciledAt: time.Unix(1, 0).UTC()}
		value := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: binding.ProjectRoot, Home: "/workspace/home", CreationDefaults: template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
		result.Workspace = &value
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func ptrMemory(value tobari.PolicyMemoryRevision) *tobari.PolicyMemoryRevision {
	clone := value.Clone()
	return &clone
}

type fakePort struct {
	template        tobari.WorkspaceTemplate
	snapshot        tobari.ContextAuthoritySnapshot
	copyPublication tobari.WorkspaceTemplateCopyPublication
	policy          tobari.PolicyCandidatePublication
	lastRef         string
	calls           int
}

func (f *fakePort) ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error) {
	return []tobari.WorkspaceTemplate{f.template}, nil
}
func (f *fakePort) DiscoverWorkspaceTemplate(context.Context, string) (tobari.WorkspaceTemplate, error) {
	return f.template, nil
}
func (f *fakePort) CreateWorkspaceTemplate(_ context.Context, _ string, _ tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplate, error) {
	f.calls++
	return f.template, nil
}
func (f *fakePort) CopyWorkspaceTemplateByRevisionReference(_ context.Context, ref, name string) (tobari.WorkspaceTemplateCopyPublication, error) {
	f.calls++
	f.lastRef = ref
	return f.copyPublication, nil
}
func (f *fakePort) SetDefaultWorkspaceTemplateByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateSelectionResult, error) {
	f.calls++
	f.lastRef = ref
	return tobari.WorkspaceTemplateSelectionResult{TemplateID: templateID, Selected: true}, nil
}
func (f *fakePort) DeleteWorkspaceTemplateByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateDeleteResult, error) {
	f.calls++
	f.lastRef = ref
	return tobari.WorkspaceTemplateDeleteResult{TemplateID: templateID, Deleted: true}, nil
}
func (f *fakePort) ListContextAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	return []tobari.ContextAuthoritySnapshot{f.snapshot}, nil
}
func (f *fakePort) ReadContextAuthorityByReference(_ context.Context, ref string) (tobari.ContextAuthoritySnapshot, error) {
	f.lastRef = ref
	return f.snapshot, nil
}
func (f *fakePort) CreateContextByTemplateReference(_ context.Context, ref, root string) (tobari.ContextAuthoritySnapshot, error) {
	f.calls++
	f.lastRef = ref
	return f.snapshot, nil
}
func (f *fakePort) EnterContextByReference(_ context.Context, ref string, _ tobari.WorkspaceSessionRequest, _ io.Reader, _ io.Writer, _ io.Writer) (tobari.ContextEntryPublication, error) {
	f.calls++
	f.lastRef = ref
	return tobari.ContextEntryPublication{Snapshot: f.snapshot, Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}, nil
}
func (f *fakePort) DeleteContextByReference(_ context.Context, ref string) (tobari.ContextDeleteResult, error) {
	f.calls++
	f.lastRef = ref
	return tobari.ContextDeleteResult{ContextID: contextID, Deleted: true}, nil
}
func (f *fakePort) ListWorkspaceAuthority(context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	return []tobari.ContextAuthoritySnapshot{f.snapshot}, nil
}
func (f *fakePort) ReadWorkspaceAuthorityByReference(_ context.Context, ref string) (tobari.ContextAuthoritySnapshot, error) {
	f.lastRef = ref
	return f.snapshot, nil
}
func (f *fakePort) DeleteWorkspaceByReference(_ context.Context, ref string, _ bool) (tobari.WorkspaceAuthorityDeleteResult, error) {
	f.calls++
	f.lastRef = ref
	return tobari.WorkspaceAuthorityDeleteResult{WorkspaceID: workspaceID, Deleted: true}, nil
}
func (f *fakePort) AllowPolicyCandidateByReference(_ context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	f.calls++
	f.lastRef = ref
	return f.policy, nil
}
func (f *fakePort) DenyPolicyCandidateByReference(_ context.Context, ref string) (tobari.PolicyCandidatePublication, error) {
	return f.AllowPolicyCandidateByReference(context.Background(), ref)
}

func intent(command string, effect operation.Effect, target operation.TargetRef, impact operation.Impact) operation.Intent {
	return operation.Intent{Command: command, Effect: effect, Target: target, Impact: impact}
}

func TestTemplateActionsKeepExactReferenceAndValidateFreshCopy(t *testing.T) {
	source := templateFixture(t)
	newID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a4")
	copied, err := tobari.CopyWorkspaceTemplateRevision(newID, "copied", source.Current)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakePort{template: source, copyPublication: tobari.WorkspaceTemplateCopyPublication{Source: source.Current, Created: copied}}
	service := NewTemplateService(fake)
	revisionRef, _ := tobari.WorkspaceTemplateRevisionRef(templateID, source.Current.Revision)
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: revisionRef}
	view, err := service.Copy(context.Background(), intent(TaskTemplateCopy, operation.EffectCreate, target, TemplateCreateImpact()), revisionRef, "copied")
	if err != nil || view.Template.ID != newID || fake.lastRef != revisionRef {
		t.Fatalf("copy=%#v ref=%q err=%v", view, fake.lastRef, err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(templateID)
	target = operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	if _, err := service.SetDefault(context.Background(), intent(TaskTemplateDefaultSet, operation.EffectWrite, target, TemplateDefaultImpact()), templateRef); err != nil || fake.lastRef != templateRef {
		t.Fatalf("default ref=%q err=%v", fake.lastRef, err)
	}
	before := fake.calls
	if _, err := service.Copy(context.Background(), intent(TaskTemplateCopy, operation.EffectCreate, target, TemplateCreateImpact()), "restricted", "copied"); err == nil || fake.calls != before {
		t.Fatal("name-selected copy reached port")
	}
}

func TestContextAndWorkspaceActionsUseOnlyExactRefs(t *testing.T) {
	template := templateFixture(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(templateID)
	contextRef, _ := tobari.ContextRef(contextID)
	workspaceRef, _ := tobari.WorkspaceRef(workspaceID)
	fake := &fakePort{template: template, snapshot: snapshotFixture(t, false, false)}
	contexts := NewContextService(fake)
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: templateRef}
	created, err := contexts.Create(context.Background(), intent(TaskContextCreate, operation.EffectCreate, target, ContextCreateImpact()), templateRef, "/workspace/example")
	if err != nil || created.ContextRef != contextRef || fake.lastRef != templateRef {
		t.Fatalf("create=%#v ref=%q err=%v", created, fake.lastRef, err)
	}
	fake.snapshot = snapshotFixture(t, true, true)
	target = operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
	entered, err := contexts.Enter(context.Background(), intent(TaskContextEnter, operation.EffectCreate, target, ContextEnterImpact()), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	if err != nil || entered.ContextRef != contextRef || entered.WorkspaceRef != workspaceRef || fake.lastRef != contextRef {
		t.Fatalf("enter=%#v ref=%q err=%v", entered, fake.lastRef, err)
	}
	workspaces := NewWorkspaceService(fake)
	view, err := workspaces.Status(context.Background(), workspaceRef)
	if err != nil || view.WorkspaceRef != workspaceRef || fake.lastRef != workspaceRef {
		t.Fatalf("status=%#v ref=%q err=%v", view, fake.lastRef, err)
	}
	target = operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ID: workspaceRef}
	if _, err := workspaces.Delete(context.Background(), intent(TaskWorkspaceDelete, operation.EffectWrite, target, WorkspaceDeleteImpact()), workspaceRef, false); err != nil || fake.lastRef != workspaceRef {
		t.Fatalf("delete ref=%q err=%v", fake.lastRef, err)
	}
}

func TestPolicyMemoryActionRejectsCrossBoundaryAndKeepsCandidateRef(t *testing.T) {
	snapshot := snapshotFixture(t, true, true)
	previous := snapshot.PolicyMemory
	body := tobari.PolicyMemoryRuleBody{PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP}, Match: tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/items", Segments: []string{}, Examples: []string{"/items"}, SourceCandidates: []string{"pcy_0123456789abcdef0123456789abcdef"}}
	rule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryAllow, body)
	if err != nil {
		t.Fatal(err)
	}
	current, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &previous)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.PolicyMemory = current
	snapshot.ActivePolicyMemory = ptrMemory(current)
	receipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: current.Revision}
	snapshot.ActivePolicyMemoryRef = &receipt
	publication := tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true}
	candidate := body.SourceCandidates[0]
	fake := &fakePort{policy: tobari.PolicyCandidatePublication{CandidateID: candidate, RuleID: rule.ID, Memory: publication}}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidate}
	result, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, target, PolicyMemoryImpact()), candidate)
	if err != nil || result.CandidateID != candidate || fake.lastRef != candidate {
		t.Fatalf("allow=%#v ref=%q err=%v", result, fake.lastRef, err)
	}
	bad := snapshot
	bad.Template.Current.Body.Boundary.DestinationCeiling.Authorities[0].Host = "other.example.dev"
	if err := bad.PolicyMemory.ValidateFor(bad.Context, bad.Template.Current); err == nil {
		t.Fatal("cross-Boundary Policy Memory passed")
	}
}

func TestInvalidMutationIntentFailsBeforePort(t *testing.T) {
	fake := &fakePort{template: templateFixture(t)}
	service := NewTemplateService(fake)
	ref, _ := tobari.WorkspaceTemplateRef(templateID)
	wrong := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: ref}
	_, err := service.Create(context.Background(), intent(TaskTemplateCreate, operation.EffectCreate, wrong, TemplateCreateImpact()), "restricted", bodyFixture("/items"))
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_mutation_contract" || fake.calls != 0 {
		t.Fatalf("fault=%#v ok=%v calls=%d", public, ok, fake.calls)
	}
}
