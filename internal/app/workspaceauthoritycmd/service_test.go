package workspaceauthoritycmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	templateID       tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	contextID        tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a2"
	workspaceID      tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
	otherContextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a4"
	otherWorkspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a5"
	otherTemplateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a6"
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
	template          tobari.WorkspaceTemplate
	snapshot          tobari.ContextAuthoritySnapshot
	copyPublication   tobari.WorkspaceTemplateCopyPublication
	updatePublication tobari.WorkspaceTemplateRevisionPublication
	policy            tobari.PolicyCandidatePublication
	reset             tobari.PolicyRuleResetPublication
	reviewed          tobari.PolicyMemoryReviewedSetPublication
	reviewedSet       tobari.PolicyMemoryReviewedDecisionSet
	lastRef           string
	calls             int
	entryErr          error
	createContextErr  error
	deleteContextErr  error
	readErr           error
}

func (f *fakePort) ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
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
func (f *fakePort) UpdateWorkspaceTemplateByReference(_ context.Context, ref string, _ tobari.WorkspaceTemplateChange) (tobari.WorkspaceTemplateRevisionPublication, error) {
	f.calls++
	f.lastRef = ref
	return f.updatePublication, nil
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
	if f.createContextErr != nil {
		return tobari.ContextAuthoritySnapshot{}, f.createContextErr
	}
	return f.snapshot, nil
}
func (f *fakePort) EnterContextByReference(_ context.Context, ref string, _ tobari.WorkspaceSessionRequest, _ io.Reader, _ io.Writer, _ io.Writer) (tobari.ContextEntryPublication, error) {
	f.calls++
	f.lastRef = ref
	if f.entryErr != nil {
		return tobari.ContextEntryPublication{}, f.entryErr
	}
	return tobari.ContextEntryPublication{Snapshot: f.snapshot, Outcome: tobari.WorkspaceSessionOutcome{ExitCode: 0, CleanupIssues: []tobari.WorkspaceAttachmentCleanupIssue{}}}, nil
}
func (f *fakePort) DeleteContextByReference(_ context.Context, ref string) (tobari.ContextDeleteResult, error) {
	f.calls++
	f.lastRef = ref
	if f.deleteContextErr != nil {
		return tobari.ContextDeleteResult{}, f.deleteContextErr
	}
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
func (f *fakePort) ResetPolicyMemoryRuleByReference(_ context.Context, ref string) (tobari.PolicyRuleResetPublication, error) {
	f.calls++
	f.lastRef = ref
	return f.reset, nil
}
func (f *fakePort) ApplyReviewedPolicyMemory(_ context.Context, set tobari.PolicyMemoryReviewedDecisionSet) (tobari.PolicyMemoryReviewedSetPublication, error) {
	f.calls++
	f.reviewedSet = set.Clone()
	return f.reviewed.Clone(), nil
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

func TestTemplateConfigurationBindsCommandToTypedDeltaAndRuntimeParent(t *testing.T) {
	template := templateFixture(t)
	templateRef, _ := tobari.WorkspaceTemplateRef(template.ID)
	value := "xterm-256color"
	shell := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeShell, Shell: []tobari.ManifestShellEnvironmentSetting{{Variable: "TERM", Source: tobari.ManifestShellEnvironmentLiteral, Value: &value}}}
	nextBody, err := tobari.ApplyWorkspaceTemplateChange(template.Current.Body, shell, nil)
	if err != nil {
		t.Fatal(err)
	}
	next, changed, err := tobari.AdvanceWorkspaceTemplateRevision(template.Current, nextBody)
	if err != nil || !changed {
		t.Fatal(err)
	}
	updatedTemplate := template.Clone()
	updatedTemplate.Current = next
	updatedTemplate.Retained = append(updatedTemplate.Retained, next)
	fake := &fakePort{updatePublication: tobari.WorkspaceTemplateRevisionPublication{Template: updatedTemplate, Previous: template.Current, Current: next, Changed: true}}
	service := NewTemplateService(fake)
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	publication, err := service.UpdateConfiguration(
		context.Background(), intent(TaskTemplateConfigShell, operation.EffectWrite, target, mustTemplateConfigurationImpact(t, TaskTemplateConfigShell)), templateRef, shell,
	)
	if err != nil || !publication.Changed || fake.calls != 1 || fake.lastRef != templateRef {
		t.Fatalf("shell publication=%#v calls=%d ref=%q err=%v", publication, fake.calls, fake.lastRef, err)
	}

	before := fake.calls
	if _, err := service.UpdateConfiguration(
		context.Background(), intent(TaskTemplateConfigGit, operation.EffectWrite, target, mustTemplateConfigurationImpact(t, TaskTemplateConfigGit)), templateRef, shell,
	); err == nil || fake.calls != before {
		t.Fatalf("command/change mismatch reached adapter: calls=%d err=%v", fake.calls, err)
	}

	runtimeID := "01912345-6789-7abc-8def-0123456789b7"
	revision := string(digest("b"))
	revisionRef := tobari.RuntimeRevisionRef(runtimeID, revision)
	resolved := tobari.RuntimeBinding{RuntimeID: runtimeID, Name: "managed", Revision: revision, Ordinal: 3, Image: "tobari-runtime-managed:bbbbbbbbbbbb"}
	runtimeChange := tobari.WorkspaceTemplateChange{Kind: tobari.WorkspaceTemplateChangeRuntime, RuntimeRevisionRef: revisionRef}
	runtimeBody, err := tobari.ApplyWorkspaceTemplateChange(template.Current.Body, runtimeChange, &resolved)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRevision, runtimeChanged, err := tobari.AdvanceWorkspaceTemplateRevision(template.Current, runtimeBody)
	if err != nil || !runtimeChanged {
		t.Fatal(err)
	}
	runtimeTemplate := template.Clone()
	runtimeTemplate.Current = runtimeRevision
	runtimeTemplate.Retained = append(runtimeTemplate.Retained, runtimeRevision)
	fake.updatePublication = tobari.WorkspaceTemplateRevisionPublication{
		Template: runtimeTemplate, Previous: template.Current, Current: runtimeRevision, ResolvedRuntime: &resolved, Changed: true,
	}
	target.ParentID = revisionRef
	if _, err := service.UpdateConfiguration(
		context.Background(), intent(TaskTemplateRuntimeSet, operation.EffectWrite, target, mustTemplateConfigurationImpact(t, TaskTemplateRuntimeSet)), templateRef, runtimeChange,
	); err != nil || fake.calls != before+1 {
		t.Fatalf("Runtime exact-parent update calls=%d err=%v", fake.calls, err)
	}
	target.ParentID = ""
	if _, err := service.UpdateConfiguration(
		context.Background(), intent(TaskTemplateRuntimeSet, operation.EffectWrite, target, mustTemplateConfigurationImpact(t, TaskTemplateRuntimeSet)), templateRef, runtimeChange,
	); err == nil || fake.calls != before+1 {
		t.Fatalf("Runtime update without exact parent reached adapter: calls=%d err=%v", fake.calls, err)
	}
}

func mustTemplateConfigurationImpact(t *testing.T, command string) operation.Impact {
	t.Helper()
	impact, err := TemplateConfigurationImpact(command)
	if err != nil {
		t.Fatal(err)
	}
	return impact
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

func TestContextEntryClassifiesSupportedAuthorityBoundariesWithoutUnknownMutation(t *testing.T) {
	contextRef, _ := tobari.ContextRef(contextID)
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
	for _, test := range []struct {
		name   string
		err    error
		code   string
		phase  fault.Phase
		change fault.ChangeState
	}{
		{name: "Template policy stale", err: tobari.ErrWorkspaceEntryTemplatePolicyInactive, code: "workspace_entry_template_policy_inactive", phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{name: "Policy Memory stale", err: tobari.ErrWorkspaceEntryPolicyMemoryInactive, code: "workspace_entry_policy_memory_inactive", phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{name: "observation unavailable", err: errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, errors.New("synthetic private observation")), code: "workspace_entry_observation_unavailable", phase: fault.PhaseObservation, change: fault.ChangeNotApplicable},
		{name: "durable decision interrupted", err: errors.Join(tobari.ErrWorkspaceEntryInterrupted, context.DeadlineExceeded), code: "workspace_entry_interrupted", phase: fault.PhaseMutation, change: fault.ChangePartial},
		{name: "published before attachment", err: errors.Join(tobari.ErrWorkspaceEntryReconciliationConfirmed, context.Canceled), code: "workspace_entry_attachment_unavailable", phase: fault.PhaseAttachment, change: fault.ChangeConfirmed},
		{name: "canceled before decision", err: errors.Join(tobari.ErrWorkspaceEntryCanceledBeforeDecision, context.Canceled), code: "workspace_entry_canceled", phase: fault.PhasePrecondition, change: fault.ChangeNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakePort{snapshot: snapshotFixture(t, true, true), entryErr: test.err}
			service := NewContextService(fake)
			_, err := service.Enter(context.Background(), intent(TaskContextEnter, operation.EffectCreate, target, ContextEnterImpact()), contextRef, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || public.Phase != test.phase || public.ChangeState != test.change || public.Code == "unclassified_mutation_outcome" {
				t.Fatalf("fault=%#v ok=%t", public, ok)
			}
			if len(public.NextActions) != 1 || public.NextActions[0].Command == "context enter" {
				t.Fatalf("recovery is not read-only: %#v", public.NextActions)
			}
		})
	}
}

func TestContextCreateDeleteRawCancellationDoesNotBorrowEntryPreconditionClassification(t *testing.T) {
	templateRef, _ := tobari.WorkspaceTemplateRef(templateID)
	contextRef, _ := tobari.ContextRef(contextID)
	for _, test := range []struct {
		name string
		run  func(*ContextService) error
		fake *fakePort
	}{
		{
			name: "create",
			fake: &fakePort{snapshot: snapshotFixture(t, false, false), createContextErr: context.Canceled},
			run: func(service *ContextService) error {
				_, err := service.Create(context.Background(), intent(TaskContextCreate, operation.EffectCreate, operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: templateRef}, ContextCreateImpact()), templateRef, "/workspace/example")
				return err
			},
		},
		{
			name: "delete",
			fake: &fakePort{snapshot: snapshotFixture(t, false, false), deleteContextErr: context.Canceled},
			run: func(service *ContextService) error {
				_, err := service.Delete(context.Background(), intent(TaskContextDelete, operation.EffectWrite, operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}, ContextDeleteImpact()), contextRef)
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			public, ok := fault.PublicCopy(test.run(NewContextService(test.fake)))
			if !ok || public.Code != "unclassified_mutation_outcome" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeUnknown {
				t.Fatalf("fault=%#v ok=%t", public, ok)
			}
		})
	}
}

func TestPolicyMemoryActionRejectsCrossBoundaryAndKeepsCandidateRef(t *testing.T) {
	snapshot := snapshotFixture(t, true, true)
	previous := snapshot.PolicyMemory
	effect := policyEffect("/items")
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	body := effect.RuleBody(candidate.ID)
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
	fake := &fakePort{policy: tobari.PolicyCandidatePublication{Candidate: candidate, RuleID: rule.ID, Previous: previous, Memory: publication}}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidate.ID}
	result, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, target, PolicyMemoryImpact()), candidate.ID)
	if err != nil || result.Persistent == nil || result.Persistent.Candidate.ID != candidate.ID || fake.lastRef != candidate.ID {
		t.Fatalf("allow=%#v ref=%q err=%v", result, fake.lastRef, err)
	}
	bad := snapshot
	bad.Template.Current.Body.Boundary.DestinationCeiling.Authorities[0].Host = "other.example.dev"
	if err := bad.PolicyMemory.ValidateFor(bad.Context, bad.Template.Current); err == nil {
		t.Fatal("cross-Boundary Policy Memory passed")
	}
}

func policyEffect(path string) tobari.PolicyCandidateEffect {
	return tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: path,
		Segments: []string{}, Examples: []string{path},
	}
}

func reviewedApplicationFixture(t *testing.T) (tobari.PolicyMemoryReviewedDecisionSet, tobari.PolicyMemoryReviewedSetPublication) {
	t.Helper()
	snapshot := snapshotFixture(t, true, true)
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, policyEffect("/reviewed"))
	if err != nil {
		t.Fatal(err)
	}
	previous, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{snapshot.Template},
		[]tobari.WorkspaceAuthorityContextRecord{{
			Context: snapshot.Context, PolicyMemory: snapshot.PolicyMemory,
			ActiveTemplatePolicy: snapshot.ActiveTemplatePolicy, ActivePolicyMemory: snapshot.ActivePolicyMemory,
			ActivePolicyMemoryRef: snapshot.ActivePolicyMemoryRef,
		}},
		[]tobari.WorkspaceBinding{*snapshot.Workspace}, []tobari.PolicyCandidateAuthority{candidate}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := tobari.NewPolicyMemoryReviewedDecision(
		candidate.ID,
		[]tobari.PolicyCandidateAuthority{candidate}, []tobari.PolicyMemoryRule{}, tobari.PolicyMemoryAllow,
		candidate.Effect.RuleBody(candidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSet(previous, []tobari.PolicyMemoryReviewedDecision{decision})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryAllow, decision.Rule)
	if err != nil {
		t.Fatal(err)
	}
	memory, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &snapshot.PolicyMemory)
	if err != nil || !changed {
		t.Fatalf("reviewed memory: changed=%t err=%v", changed, err)
	}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: memory.Revision}
	nextRecord := tobari.WorkspaceAuthorityContextRecord{
		Context: snapshot.Context, PolicyMemory: memory, ActiveTemplatePolicy: snapshot.ActiveTemplatePolicy,
		ActivePolicyMemory: ptrMemory(memory), ActivePolicyMemoryRef: &memoryReceipt,
	}
	next, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		previous.Templates, []tobari.WorkspaceAuthorityContextRecord{nextRecord}, previous.Workspaces,
		[]tobari.PolicyCandidateAuthority{}, previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("reviewed collection: changed=%t err=%v", changed, err)
	}
	projection, err := tobari.BuildReviewedWorkspacePolicyProjection(next, []tobari.ContextID{contextID})
	if err != nil {
		t.Fatal(err)
	}
	settlement := tobari.PolicyMemoryReviewedSettlementReceipt{
		DecisionSetDigest: set.Digest, PlanDigest: projection.PlanDigest, ContentDigest: projection.ContentDigest,
		AggregateRevision: strings.Repeat("a", 64), PolicyArtifact: digest("b"), GatewayArtifact: digest("c"), PrincipalDigest: digest("d"),
	}
	publication, err := tobari.NewPolicyMemoryReviewedSetPublication(previous, next, set, settlement)
	if err != nil {
		t.Fatal(err)
	}
	return set, publication
}

func TestApplyReviewedConsumesOneExplicitCompleteSetAndReturnsExhaustiveResult(t *testing.T) {
	set, publication := reviewedApplicationFixture(t)
	fake := &fakePort{reviewed: publication}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ParentID: tobari.PolicyDecisionSetID}
	result, err := service.ApplyReviewed(
		context.Background(), intent(TaskPolicyApply, operation.EffectCreate, target, PolicyMemoryImpact()), set,
	)
	if err != nil || fake.calls != 1 || !reflect.DeepEqual(fake.reviewedSet, set) ||
		len(result.AppliedDecisions) != 1 || result.AppliedDecisions[0].ReviewItemID != set.Decisions[0].ReviewItemID {
		t.Fatalf("result=%#v set=%#v calls=%d err=%v", result, fake.reviewedSet, fake.calls, err)
	}
	if _, err := tobari.ParseContextRef(result.AppliedDecisions[0].ContextRef); err != nil {
		t.Fatalf("result Context reference is not actionable: %v", err)
	}
	if _, err := tobari.ParseWorkspaceTemplateRef(result.AppliedDecisions[0].TemplateRef); err != nil {
		t.Fatalf("result Template reference is not actionable: %v", err)
	}
	if _, err := tobari.ParseWorkspaceRef(result.AppliedDecisions[0].ObservingWorkspaceRef); err != nil {
		t.Fatalf("result Workspace reference is not actionable: %v", err)
	}
}

func TestApplyReviewedRejectsInvalidOrSubstitutedSetBeforeSemanticSuccess(t *testing.T) {
	set, publication := reviewedApplicationFixture(t)
	fake := &fakePort{reviewed: publication}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ParentID: tobari.PolicyDecisionSetID}
	_, err := service.ApplyReviewed(
		context.Background(), intent(TaskPolicyApply, operation.EffectCreate, target, PolicyMemoryImpact()), tobari.PolicyMemoryReviewedDecisionSet{},
	)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_policy_review_set" || fake.calls != 0 {
		t.Fatalf("invalid set reached adapter: fault=%#v calls=%d", public, fake.calls)
	}

	otherDecision, err := tobari.NewPolicyMemoryReviewedDecision(
		set.Decisions[0].Candidates[0].ID,
		[]tobari.PolicyCandidateAuthority{set.Decisions[0].Candidates[0]}, []tobari.PolicyMemoryRule{}, tobari.PolicyMemoryDeny,
		set.Decisions[0].Candidates[0].Effect.RuleBody(set.Decisions[0].Candidates[0].ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	otherSet, err := tobari.NewPolicyMemoryReviewedDecisionSet(publication.Previous, []tobari.PolicyMemoryReviewedDecision{otherDecision})
	if err != nil {
		t.Fatal(err)
	}
	fake.reviewed.DecisionSet = otherSet
	_, err = service.ApplyReviewed(
		context.Background(), intent(TaskPolicyApply, operation.EffectCreate, target, PolicyMemoryImpact()), set,
	)
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "invalid_policy_memory_result" || fake.calls != 1 {
		t.Fatalf("substituted set was accepted: fault=%#v calls=%d", public, fake.calls)
	}
}

func candidatePublicationFixture(t *testing.T, candidate tobari.PolicyCandidateAuthority, resultingEffect tobari.PolicyCandidateEffect, ruleCandidate string, decision tobari.PolicyMemoryDecision) tobari.PolicyCandidatePublication {
	t.Helper()
	snapshot := snapshotFixture(t, true, true)
	previous := snapshot.PolicyMemory
	rule, err := tobari.NewPolicyMemoryRule(contextID, decision, resultingEffect.RuleBody(ruleCandidate))
	if err != nil {
		t.Fatal(err)
	}
	current, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &previous)
	if err != nil || !changed {
		t.Fatalf("publish candidate memory: changed=%t err=%v", changed, err)
	}
	snapshot.PolicyMemory = current
	snapshot.ActivePolicyMemory = ptrMemory(current)
	receipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: current.Revision}
	snapshot.ActivePolicyMemoryRef = &receipt
	return tobari.PolicyCandidatePublication{
		Candidate: candidate, RuleID: rule.ID, Previous: previous,
		Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true},
	}
}

func TestPolicyCandidatePublicationBindsRequestedAuthorityAndDecision(t *testing.T) {
	effect := policyEffect("/items")
	otherEffect := policyEffect("/other")
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	otherCandidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, otherEffect)
	if err != nil {
		t.Fatal(err)
	}
	otherContextCandidate, err := tobari.NewPolicyCandidateAuthority(otherContextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	otherWorkspaceCandidate, err := tobari.NewPolicyCandidateAuthority(contextID, otherWorkspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		requested   tobari.PolicyCandidateAuthority
		publication tobari.PolicyCandidatePublication
	}{
		{name: "opposite decision", requested: candidate, publication: candidatePublicationFixture(t, candidate, effect, candidate.ID, tobari.PolicyMemoryDeny)},
		{name: "other candidate rule", requested: candidate, publication: candidatePublicationFixture(t, candidate, otherEffect, otherCandidate.ID, tobari.PolicyMemoryAllow)},
		{name: "wrong exact effect", requested: candidate, publication: candidatePublicationFixture(t, candidate, otherEffect, candidate.ID, tobari.PolicyMemoryAllow)},
		{name: "cross Context", requested: otherContextCandidate, publication: candidatePublicationFixture(t, otherContextCandidate, effect, otherContextCandidate.ID, tobari.PolicyMemoryAllow)},
		{name: "cross observing Workspace", requested: otherWorkspaceCandidate, publication: candidatePublicationFixture(t, otherWorkspaceCandidate, effect, otherWorkspaceCandidate.ID, tobari.PolicyMemoryAllow)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakePort{policy: test.publication}
			service := NewPolicyMemoryService(fake)
			target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: test.requested.ID}
			_, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, target, PolicyMemoryImpact()), test.requested.ID)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "invalid_policy_memory_result" || fake.calls != 1 {
				t.Fatalf("fault=%#v ok=%t calls=%d err=%v", public, ok, fake.calls, err)
			}
		})
	}

	noOp := candidatePublicationFixture(t, candidate, effect, candidate.ID, tobari.PolicyMemoryAllow)
	noOp.Previous = noOp.Memory.Snapshot.PolicyMemory.Clone()
	noOp.Memory.PreviousRevision = noOp.Previous.Revision
	noOp.Memory.Changed = false
	fake := &fakePort{policy: noOp}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: candidate.ID}
	if _, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, target, PolicyMemoryImpact()), candidate.ID); err == nil {
		t.Fatal("unchanged candidate authority was accepted as successful mutation")
	}

	collateral := candidatePublicationFixture(t, candidate, effect, candidate.ID, tobari.PolicyMemoryAllow)
	unrelatedCandidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, otherEffect)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedRule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryAllow, otherEffect.RuleBody(unrelatedCandidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	requestedRule := collateral.Memory.Snapshot.PolicyMemory.Rules[0]
	current, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{requestedRule, unrelatedRule}, &collateral.Previous)
	if err != nil || !changed {
		t.Fatalf("publish collateral memory: changed=%t err=%v", changed, err)
	}
	collateral.Memory.Snapshot.PolicyMemory = current
	collateral.Memory.Snapshot.ActivePolicyMemory = ptrMemory(current)
	collateral.Memory.Snapshot.ActivePolicyMemoryRef = &tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: current.Revision}
	fake = &fakePort{policy: collateral}
	service = NewPolicyMemoryService(fake)
	if _, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, target, PolicyMemoryImpact()), candidate.ID); err == nil {
		t.Fatal("candidate mutation with collateral authority change was accepted")
	}
}

func TestPolicyResetRequiresExactRemovalAndChangedAuthority(t *testing.T) {
	effect := policyEffect("/items")
	candidate, err := tobari.NewPolicyCandidateAuthority(contextID, workspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := snapshotFixture(t, true, true)
	empty := snapshot.PolicyMemory
	rule, err := tobari.NewPolicyMemoryRule(contextID, tobari.PolicyMemoryAllow, effect.RuleBody(candidate.ID))
	if err != nil {
		t.Fatal(err)
	}
	previous, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{rule}, &empty)
	if err != nil {
		t.Fatal(err)
	}
	current, changed, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, &previous)
	if err != nil || !changed {
		t.Fatalf("publish reset memory: changed=%t err=%v", changed, err)
	}
	snapshot.PolicyMemory = current
	snapshot.ActivePolicyMemory = ptrMemory(current)
	receipt := tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: current.Revision}
	snapshot.ActivePolicyMemoryRef = &receipt
	publication := tobari.PolicyRuleResetPublication{
		RuleID: rule.ID, RemovedFrom: previous,
		Memory: tobari.PolicyMemoryPublication{Snapshot: snapshot, PreviousRevision: previous.Revision, Changed: true},
	}
	fake := &fakePort{reset: publication}
	service := NewPolicyMemoryService(fake)
	target := operation.TargetRef{Kind: tobari.PolicyRuleKind, ID: rule.ID}
	if _, err := service.Reset(context.Background(), intent(TaskPolicyReset, operation.EffectWrite, target, PolicyMemoryImpact()), rule.ID); err != nil || fake.lastRef != rule.ID {
		t.Fatalf("reset ref=%q err=%v", fake.lastRef, err)
	}

	noOp := publication
	noOp.Memory.Snapshot.PolicyMemory = previous.Clone()
	noOp.Memory.Snapshot.ActivePolicyMemory = ptrMemory(previous)
	noOp.Memory.Snapshot.ActivePolicyMemoryRef = &tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: previous.Revision}
	noOp.Memory.PreviousRevision = previous.Revision
	noOp.Memory.Changed = false
	fake = &fakePort{reset: noOp}
	service = NewPolicyMemoryService(fake)
	if _, err := service.Reset(context.Background(), intent(TaskPolicyReset, operation.EffectWrite, target, PolicyMemoryImpact()), rule.ID); err == nil {
		t.Fatal("unchanged rule authority was accepted as successful reset")
	}
}

func rebindSnapshot(t *testing.T, source tobari.ContextAuthoritySnapshot, newContextID tobari.ContextID, root string, newWorkspaceID tobari.WorkspaceID) tobari.ContextAuthoritySnapshot {
	t.Helper()
	result := source.Clone()
	result.Context.ID = newContextID
	result.Context.ProjectRoot = root
	memory, _, err := tobari.PublishPolicyMemory(newContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result.PolicyMemory = memory
	if result.ActiveTemplatePolicy != nil {
		result.ActiveTemplatePolicy.ContextID = newContextID
		result.ActivePolicyMemory = ptrMemory(memory)
		result.ActivePolicyMemoryRef = &tobari.PolicyMemoryActivationReceipt{ContextID: newContextID, Revision: memory.Revision}
	}
	if result.Workspace != nil {
		result.Workspace.ID = newWorkspaceID
		result.Workspace.ContextID = newContextID
		result.Workspace.ProjectRoot = root
		if result.Workspace.LastSuccessfulEntry != nil {
			result.Workspace.LastSuccessfulEntry.ContextID = newContextID
		}
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestExhaustiveAuthorityListsRejectContradictoryBindings(t *testing.T) {
	firstContext := snapshotFixture(t, false, false)
	duplicatePair := rebindSnapshot(t, firstContext, otherContextID, firstContext.Context.ProjectRoot, otherWorkspaceID)
	if _, err := NewContextList([]ContextSnapshot{firstContext, duplicatePair}); err == nil {
		t.Fatal("duplicate Project and Template Context pair was accepted")
	}

	firstWorkspace := snapshotFixture(t, true, true)
	secondForContext := firstWorkspace.Clone()
	secondForContext.Workspace.ID = otherWorkspaceID
	if err := secondForContext.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceList([]ContextSnapshot{firstWorkspace, secondForContext}); err == nil {
		t.Fatal("multiple Workspaces for one Context were accepted")
	}

	duplicateWorkspaceID := rebindSnapshot(t, firstWorkspace, otherContextID, "/workspace/other", workspaceID)
	if _, err := NewContextList([]ContextSnapshot{firstWorkspace, duplicateWorkspaceID}); err == nil {
		t.Fatal("duplicate Workspace ID across Context snapshots was accepted")
	}

	driftedTemplate := firstContext.Template.Clone()
	driftedBody := driftedTemplate.Current.Body.Clone()
	driftedBody.Policy.BaselineGrants[0].Path = "/other"
	driftedRevision, err := tobari.NewWorkspaceTemplateRevision(templateID, 2, driftedBody)
	if err != nil {
		t.Fatal(err)
	}
	driftedTemplate.Current = driftedRevision
	driftedTemplate.Retained = append(driftedTemplate.Retained, driftedRevision.Clone())
	driftedContext := rebindSnapshot(t, firstContext, otherContextID, "/workspace/other", otherWorkspaceID)
	driftedContext.Template = driftedTemplate
	if err := driftedContext.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewContextList([]ContextSnapshot{firstContext, driftedContext}); err == nil {
		t.Fatal("one Template ID with contradictory current authority was accepted by Context list")
	}
	if _, err := NewTemplateList([]tobari.WorkspaceTemplate{firstContext.Template, driftedTemplate}); err == nil {
		t.Fatal("one Template ID with contradictory current authority was accepted by Template list")
	}
	driftedWorkspace := rebindSnapshot(t, firstWorkspace, otherContextID, "/workspace/other", otherWorkspaceID)
	driftedWorkspace.Template = driftedTemplate.Clone()
	if err := driftedWorkspace.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceList([]ContextSnapshot{firstWorkspace, driftedWorkspace}); err == nil {
		t.Fatal("one Template ID with contradictory current authority was accepted by Workspace list")
	}

	otherRevision, err := tobari.NewWorkspaceTemplateRevision(otherTemplateID, 1, firstContext.Template.Current.Body)
	if err != nil {
		t.Fatal(err)
	}
	otherTemplate := tobari.WorkspaceTemplate{
		SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: otherTemplateID, Name: firstContext.Template.Name,
		Current: otherRevision, Retained: []tobari.WorkspaceTemplateRevision{otherRevision.Clone()},
	}
	nameCollision := rebindSnapshot(t, firstContext, otherContextID, "/workspace/other", otherWorkspaceID)
	nameCollision.Context.TemplateID = otherTemplateID
	nameCollision.Template = otherTemplate
	if err := nameCollision.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewContextList([]ContextSnapshot{firstContext, nameCollision}); err == nil {
		t.Fatal("one Template name with different IDs was accepted")
	}
	if _, err := NewTemplateList([]tobari.WorkspaceTemplate{firstContext.Template, otherTemplate}); err == nil {
		t.Fatal("one Template name with different IDs was accepted by Template list")
	}
	nameCollisionWorkspace := rebindSnapshot(t, firstWorkspace, otherContextID, "/workspace/other", otherWorkspaceID)
	nameCollisionWorkspace.Context.TemplateID = otherTemplateID
	nameCollisionWorkspace.Template = otherTemplate.Clone()
	nameCollisionWorkspace.ActiveTemplatePolicy.TemplateID = otherTemplateID
	nameCollisionWorkspace.Workspace.LastSuccessfulEntry.TemplateID = otherTemplateID
	if err := nameCollisionWorkspace.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkspaceList([]ContextSnapshot{firstWorkspace, nameCollisionWorkspace}); err == nil {
		t.Fatal("one Template name with different IDs was accepted by Workspace list")
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

func TestPreReleaseLegacyAuthorityHasOneZeroMutationGuidanceFault(t *testing.T) {
	legacy := fmt.Errorf("%w: synthetic legacy root", tobari.ErrPreReleaseLegacyAuthority)
	readService := NewTemplateService(&fakePort{template: templateFixture(t), readErr: legacy})
	_, err := readService.List(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "help" || !strings.Contains(public.NextActions[0].Reason, "reset/recreate") {
		t.Fatalf("read legacy fault = %#v, ok=%t", public, ok)
	}

	template := templateFixture(t)
	ref, _ := tobari.WorkspaceTemplateRef(template.ID)
	mutationPort := &fakePort{template: template, createContextErr: legacy}
	contextService := NewContextService(mutationPort)
	intent := intent(TaskContextCreate, operation.EffectCreate, operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: ref}, ContextCreateImpact())
	_, err = contextService.Create(context.Background(), intent, ref, "/workspace/example")
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || mutationPort.calls != 1 {
		t.Fatalf("mutation legacy fault = %#v ok=%t calls=%d", public, ok, mutationPort.calls)
	}
}
