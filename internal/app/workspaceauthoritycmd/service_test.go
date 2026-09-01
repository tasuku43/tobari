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

func TestTemplateSourceRecoveryFaultRetainsPartialMutationState(t *testing.T) {
	err := templateFault(tobari.ErrResourceSourceRecoveryRequired, func(error) error {
		return errors.New("unexpected fallback")
	})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "resource_source_recovery_required" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial {
		t.Fatalf("resource source recovery fault = %+v/%v", public, err)
	}
}

func digest(c string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(c, 64))
}

func bodyFixture(path string) tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadOnly, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: path}}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: "tobari-runtime:test"}},
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
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}
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
		value := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: workspaceID, ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: template.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied}
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
	draftCopy         tobari.WorkspaceTemplateDraft
	plan              tobari.WorkspaceTemplateChangePlan
	planErr           error
	migrationPlan     tobari.WorkspaceTemplatePolicyMigrationPlan
	migrationResult   tobari.WorkspaceTemplatePolicyMigrationResult
	contextPlan       tobari.ContextActivationPlan
	contextPlanErr    error
	updatePublication tobari.WorkspaceTemplateRevisionPublication
	policy            tobari.PolicyCandidatePublication
	reset             tobari.PolicyRuleResetPublication
	reviewed          tobari.PolicyMemoryReviewedSetPublication
	reviewedSet       tobari.PolicyMemoryReviewedDecisionSet
	createdBody       tobari.WorkspaceTemplateBody
	lastRef           string
	calls             int
	entryErr          error
	createContextErr  error
	deleteContextErr  error
	readErr           error
}

func (f *fakePort) PlanWorkspaceTemplatePolicyMigrationByReference(_ context.Context, ref string) (tobari.WorkspaceTemplatePolicyMigrationPlan, error) {
	f.calls++
	f.lastRef = ref
	return f.migrationPlan, nil
}

func (f *fakePort) ApplyWorkspaceTemplatePolicyMigrationByReference(_ context.Context, ref string) (tobari.WorkspaceTemplatePolicyMigrationResult, error) {
	f.calls++
	f.lastRef = ref
	return f.migrationResult, nil
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
func (f *fakePort) CreateWorkspaceTemplate(_ context.Context, _ string, body tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplate, error) {
	f.calls++
	f.createdBody = body.Clone()
	return f.template, nil
}
func (f *fakePort) CopyWorkspaceTemplateByRevisionReference(_ context.Context, ref, name string) (tobari.WorkspaceTemplateCopyPublication, error) {
	f.calls++
	f.lastRef = ref
	return f.copyPublication, nil
}
func (f *fakePort) CopyWorkspaceTemplateDraftByRevisionReference(_ context.Context, ref, name string) (tobari.WorkspaceTemplateDraft, error) {
	f.calls++
	f.lastRef = ref
	return f.draftCopy, nil
}
func (f *fakePort) PlanWorkspaceTemplateSourceByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateChangePlan, error) {
	f.lastRef = ref
	return f.plan, f.planErr
}
func (f *fakePort) PlanContextSourceByReference(_ context.Context, ref string) (tobari.ContextActivationPlan, error) {
	f.lastRef = ref
	return f.contextPlan, f.contextPlanErr
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

func (f *fakePort) EnterCurrentFinalDefaultPair(_ context.Context, observation tobari.FinalDefaultPairObservation, _ string, _ tobari.WorkspaceSessionRequest, _ io.Reader, _ io.Writer, _ io.Writer) (tobari.ContextEntryPublication, error) {
	f.calls++
	ref, _ := tobari.ContextRef(observation.Context.Context.ID)
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

func TestTemplatePublicCopyCreatesOnlyAnUnpublishedDraft(t *testing.T) {
	source := templateFixture(t)
	newID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a4")
	revision := source.Current.Revision
	draft := tobari.WorkspaceTemplateDraft{
		ID: newID, Name: "copied", Body: source.Current.Body.Clone(),
		Source: tobari.ResourceSourceObservation{Path: "/tmp/tobari/templates/" + string(newID) + "/template.yaml", State: tobari.ResourceSourceModified, SourceRevision: &revision},
	}
	fake := &fakePort{template: source, draftCopy: draft}
	service := NewTemplateService(fake)
	revisionRef, _ := tobari.WorkspaceTemplateRevisionRef(templateID, source.Current.Revision)
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: revisionRef}
	view, err := service.CopyDraft(context.Background(), intent(TaskTemplateCopy, operation.EffectCreate, target, TemplateCreateImpact()), revisionRef, "copied")
	if err != nil || view.Draft.ID != newID || view.Draft.Source.ActiveRevision != nil || fake.lastRef != revisionRef || fake.calls != 1 {
		t.Fatalf("draft=%+v ref=%q calls=%d err=%v", view, fake.lastRef, fake.calls, err)
	}
	before := fake.calls
	if _, err := service.CopyDraft(context.Background(), intent(TaskTemplateCopy, operation.EffectCreate, target, TemplateCreateImpact()), "copied", "copied"); err == nil || fake.calls != before {
		t.Fatalf("invalid copy reached port: calls=%d before=%d err=%v", fake.calls, before, err)
	}
}

func TestTemplateServiceHasNoRetiredDirectMutationMethods(t *testing.T) {
	typeOf := reflect.TypeOf((*TemplateService)(nil))
	for _, forbidden := range []string{"Create", "Copy", "UpdateBootstrap", "UpdateConfiguration"} {
		if method, exists := typeOf.MethodByName(forbidden); exists {
			t.Errorf("retired direct Template mutation method remains reachable: %+v", method)
		}
	}
	if method, exists := reflect.TypeOf((*ContextService)(nil)).MethodByName("Create"); exists {
		t.Errorf("retired direct Context mutation method remains reachable: %+v", method)
	}
}

func TestTemplatePolicyMigrationIsExplicitSourceOnlyPlanAndApply(t *testing.T) {
	body := bodyFixture("/items")
	body.Boundary.DestinationCeiling = tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}
	body.Boundary.MethodPolicy = tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: "restricted", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	v1, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	alpha := tobari.WorkspaceTemplateAlphaSource{Template: v1.Template, Policy: tobari.WorkspaceTemplatePolicyAlphaSourceDocument{
		SchemaVersion: tobari.WorkspaceTemplatePolicyAlphaSchemaVersion, TemplateID: templateID,
		Boundary: tobari.WorkspaceTemplatePolicyAlphaBoundarySource{DestinationCeiling: body.Boundary.DestinationCeiling, MethodPolicy: body.Boundary.MethodPolicy},
		Semantic: body.Policy.Clone(),
	}}
	migrated, err := alpha.MigrateToV1(body.EntryDefaults.Runtime)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.NewWorkspaceTemplatePolicyMigrationPlan(template, alpha, migrated, strings.Repeat("a", 64), strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	widened := migrated.Clone()
	widened.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules = append(widened.Policy.Semantic.Protocols.HTTP.Generic.Allow.Rules, tobari.SemanticHTTPRule{
		SemanticRuleAuthority: tobari.SemanticRuleAuthority{Scheme: "https", Host: "other.example.dev", Port: 443},
		Method:                "GET", Path: "/extra",
	})
	if _, err := tobari.NewWorkspaceTemplatePolicyMigrationPlan(template, alpha, widened, strings.Repeat("a", 64), strings.Repeat("c", 64)); err == nil {
		t.Fatal("migration plan accepted an arbitrary widened V1 target")
	}
	result := tobari.WorkspaceTemplatePolicyMigrationResult{TemplateID: templateID, TemplateRef: plan.TemplateRef, ActiveRevision: revision.Revision, SourceFingerprint: plan.TargetFingerprint, Changed: true}
	fake := &fakePort{migrationPlan: plan, migrationResult: result}
	service := NewTemplateService(fake)
	observed, err := service.PlanPolicyMigration(context.Background(), plan.TemplateRef)
	if err != nil || observed.PlanRef != plan.PlanRef || fake.lastRef != plan.TemplateRef {
		t.Fatalf("plan=%+v ref=%q err=%v", observed, fake.lastRef, err)
	}
	applyIntent := intent(TaskTemplateMigrationApply, operation.EffectWrite, operation.TargetRef{Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, ID: plan.PlanRef}, TemplatePolicyMigrationImpact())
	applied, err := service.ApplyPolicyMigration(context.Background(), applyIntent, plan.PlanRef)
	if err != nil || applied.ActiveRevision != revision.Revision || !applied.Changed || fake.lastRef != plan.PlanRef {
		t.Fatalf("result=%+v ref=%q err=%v", applied, fake.lastRef, err)
	}
	if TemplatePolicyMigrationImpact().AccessChange != operation.DeclarationNo {
		t.Fatal("source-only migration declared an authority access change")
	}
}

func TestTemplatePlanClassifiesUnknownPlannerFailureAsObservationFault(t *testing.T) {
	templateRef, err := tobari.WorkspaceTemplateRef(templateID)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakePort{template: templateFixture(t), planErr: errors.New("synthetic planner failure")}
	_, err = NewTemplateService(fake).Plan(context.Background(), templateRef)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "template_plan_read_failed" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable {
		t.Fatalf("planner failure fault=%#v ok=%t", public, ok)
	}
}

func TestTemplatePlanClassifiesMissingTemplateAsObservationFault(t *testing.T) {
	templateRef, err := tobari.WorkspaceTemplateRef(templateID)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakePort{planErr: tobari.ErrWorkspaceTemplateNotFound}
	_, err = NewTemplateService(fake).Plan(context.Background(), templateRef)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "template_not_found" || public.Kind != fault.KindNotFound || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable || len(public.NextActions) != 1 || public.NextActions[0].Command != "template list" {
		t.Fatalf("missing Template fault=%#v ok=%t", public, ok)
	}
}

func TestContextPlanClassifiesLegacyAndUnknownPlannerFailuresAsReadFaults(t *testing.T) {
	contextRef, err := tobari.ContextRef(contextID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		err  error
		code string
		kind fault.Kind
	}{
		{name: "legacy", err: fmt.Errorf("%w: synthetic legacy root", tobari.ErrPreReleaseLegacyAuthority), code: "legacy_state_present", kind: fault.KindRejected},
		{name: "unknown", err: errors.New("synthetic planner failure"), code: "context_plan_read_failed", kind: fault.KindUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &fakePort{contextPlanErr: test.err}
			_, err := NewContextService(fake).Plan(context.Background(), contextRef)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || public.Kind != test.kind || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable {
				t.Fatalf("planner failure fault=%#v ok=%t", public, ok)
			}
		})
	}
}

func TestTemplateMutationFaultClassifiesUnknownOutcomeBeforeCLINormalization(t *testing.T) {
	public, ok := fault.PublicCopy(templateMutationFault(errors.New("synthetic mutation failure")))
	if !ok || public.Code != "unclassified_mutation_outcome" || public.Kind != fault.KindContract || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeUnknown || public.Retryable {
		t.Fatalf("mutation failure fault=%#v ok=%t", public, ok)
	}
}

func TestContextMutationFaultClassifiesStaleActivationPlanBeforeMutation(t *testing.T) {
	public, ok := fault.PublicCopy(contextMutationFault(tobari.ErrWorkspaceTemplateChangePlanStale))
	if !ok || public.Code != "context_activation_plan_stale" || public.Kind != fault.KindRejected || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || public.Retryable {
		t.Fatalf("stale Context activation fault=%#v ok=%t", public, ok)
	}
}

func TestContextMutationFaultPreservesStructuredPostDecisionOutcome(t *testing.T) {
	interrupted := fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"final_authority_mutation_interrupted",
		"The final-authority mutation crossed its durable decision boundary but external settlement did not complete.",
		false,
		tobari.ErrContextBindingProtected,
		fault.NextAction{Command: "status", Reason: "Read the preserved decision and recover it through the exact initiating command."},
	), fault.PhaseMutation, fault.ChangePartial)
	public, ok := fault.PublicCopy(contextMutationFault(interrupted))
	if !ok || public.Code != "final_authority_mutation_interrupted" || public.Kind != fault.KindUnavailable || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial || public.Retryable {
		t.Fatalf("post-decision Context fault=%#v ok=%t", public, ok)
	}
	protected, ok := fault.PublicCopy(contextMutationFault(tobari.ErrContextBindingProtected))
	if !ok || protected.Code != "context_in_use" || protected.Phase != fault.PhasePrecondition || protected.ChangeState != fault.ChangeNone {
		t.Fatalf("pre-decision protected Context fault=%#v ok=%t", protected, ok)
	}
}

func TestContextAndWorkspaceActionsUseOnlyExactRefs(t *testing.T) {
	template := templateFixture(t)
	contextRef, _ := tobari.ContextRef(contextID)
	workspaceRef, _ := tobari.WorkspaceRef(workspaceID)
	fake := &fakePort{template: template, snapshot: snapshotFixture(t, false, false)}
	contexts := NewContextService(fake)
	fake.snapshot = snapshotFixture(t, true, true)
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
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
		{name: "standard Runtime preparation uncertain", err: errors.Join(tobari.ErrWorkspaceRuntimePreparationUncertain, errors.New("synthetic BuildKit failure")), code: "workspace_runtime_preparation_uncertain", phase: fault.PhaseMutation, change: fault.ChangeUnknown},
		{name: "observation unavailable", err: errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, errors.New("synthetic private observation")), code: "workspace_entry_observation_unavailable", phase: fault.PhaseObservation, change: fault.ChangeNotApplicable},
		{name: "exclusive Home operation", err: errors.Join(tobari.ErrWorkspaceEntryObservationUnavailable, tobari.ErrContextBindingProtected), code: "workspace_entry_busy", phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{name: "mutation recovery required", err: errors.Join(tobari.ErrFinalAuthorityMutationRecoveryRequired, errors.New("active decision")), code: "final_authority_mutation_recovery_required", phase: fault.PhasePrecondition, change: fault.ChangeNone},
		{name: "reconciliation fence release interrupted", err: errors.Join(tobari.ErrWorkspaceEntryInterrupted, errors.New("synthetic reconciliation fence release failure")), code: "workspace_entry_interrupted", phase: fault.PhaseMutation, change: fault.ChangePartial},
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

func TestCurrentDefaultPairEntryMapsRuntimeRepairSentinelForRootFallback(t *testing.T) {
	snapshot := snapshotFixture(t, true, true)
	template := snapshot.Template.Clone()
	observation := tobari.FinalDefaultPairObservation{
		SchemaVersion: tobari.FinalDefaultPairObservationSchemaVersion, CollectionPresent: true,
		CollectionGeneration: 9, CollectionRevision: digest("9"), ProjectRoot: snapshot.Workspace.ProjectRoot,
		DefaultTemplate: &template, Context: &snapshot,
	}
	if err := observation.Validate(); err != nil {
		t.Fatal(err)
	}
	fake := &fakePort{snapshot: snapshot, entryErr: errors.Join(tobari.ErrWorkspaceEntryRuntimeNotCurrent, tobari.ErrRuntimeNotReady)}
	service := NewContextService(fake)
	contextRef, _ := tobari.ContextRef(snapshot.Context.ID)
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
	_, err := service.EnterCurrentDefaultPair(context.Background(), intent(TaskContextEnter, operation.EffectCreate, target, ContextEnterImpact()), observation, observation.ProjectRoot, tobari.NewWorkspaceShellSession(), strings.NewReader(""), io.Discard, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "workspace_entry_repair_required" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || !public.Retryable {
		t.Fatalf("current Runtime repair fault=%#v ok=%t err=%v", public, ok, err)
	}
}

func TestContextCreateDeleteRawCancellationDoesNotBorrowEntryPreconditionClassification(t *testing.T) {
	contextRef, _ := tobari.ContextRef(contextID)
	for _, test := range []struct {
		name string
		run  func(*ContextService) error
		fake *fakePort
	}{
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

func TestPolicyMemoryValidatesReferencesBeforePortAvailability(t *testing.T) {
	service := NewPolicyMemoryService(nil)
	for _, test := range []struct {
		name string
		run  func() error
		code string
	}{
		{name: "allow", run: func() error {
			_, err := service.Allow(context.Background(), intent(TaskPolicyAllow, operation.EffectWrite, operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: "bad"}, PolicyMemoryImpact()), "bad")
			return err
		}, code: "invalid_policy_candidate_ref"},
		{name: "deny", run: func() error {
			_, err := service.Deny(context.Background(), intent(TaskPolicyDeny, operation.EffectWrite, operation.TargetRef{Kind: tobari.PolicyCandidateKind, ID: "bad"}, PolicyMemoryImpact()), "bad")
			return err
		}, code: "invalid_policy_candidate_ref"},
		{name: "reset", run: func() error {
			_, err := service.Reset(context.Background(), intent(TaskPolicyReset, operation.EffectWrite, operation.TargetRef{Kind: tobari.PolicyRuleKind, ID: "bad"}, PolicyMemoryImpact()), "bad")
			return err
		}, code: "invalid_policy_rule_ref"},
	} {
		t.Run(test.name, func(t *testing.T) {
			public, ok := fault.PublicCopy(test.run())
			if !ok || public.Code != test.code || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
				t.Fatalf("fault=%#v ok=%t", public, ok)
			}
		})
	}

	_, err := service.ApplyReviewed(context.Background(), intent(TaskPolicyApply, operation.EffectCreate, operation.TargetRef{Kind: tobari.PolicyDecisionSetKind, ParentID: tobari.PolicyDecisionSetID}, PolicyMemoryImpact()), tobari.PolicyMemoryReviewedDecisionSet{})
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_policy_review_set" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone {
		t.Fatalf("invalid reviewed set fault=%#v ok=%t", public, ok)
	}
}

func TestPolicyMemoryMutationFaultPreservesPreflightAuthorityReadFailure(t *testing.T) {
	public, ok := fault.PublicCopy(policyMemoryMutationFault(fault.WithClassification(
		fault.New(fault.KindUnavailable, "final_authority_read_failed", "synthetic authority read failure", false, fault.NextAction{Command: "status", Reason: "read"}),
		fault.PhaseObservation,
		fault.ChangeNotApplicable,
	)))
	if !ok || public.Code != "final_authority_read_failed" || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable {
		t.Fatalf("preflight authority read fault=%#v ok=%t", public, ok)
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

func TestExhaustiveAuthorityListsRejectDuplicateContextIDs(t *testing.T) {
	firstContext := snapshotFixture(t, false, false)
	if _, err := NewContextList([]ContextSnapshot{firstContext, firstContext.Clone()}); err == nil {
		t.Fatal("duplicate Context ID was accepted")
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

func TestPreReleaseLegacyAuthorityHasOneZeroMutationGuidanceFault(t *testing.T) {
	for name, sentinel := range map[string]error{
		"pre_release_envelope": tobari.ErrPreReleaseLegacyAuthority,
		"executable_policy":    fmt.Errorf("%w: %w", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy),
	} {
		t.Run(name, func(t *testing.T) {
			legacy := fmt.Errorf("%w: synthetic legacy root", sentinel)
			readService := NewTemplateService(&fakePort{template: templateFixture(t), readErr: legacy})
			_, err := readService.List(context.Background())
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhaseObservation || public.ChangeState != fault.ChangeNotApplicable ||
				len(public.NextActions) != 1 || public.NextActions[0].Command != "help" || !strings.Contains(public.NextActions[0].Reason, "reset/recreate") {
				t.Fatalf("read legacy fault = %#v, ok=%t", public, ok)
			}

			mutationPort := &fakePort{template: templateFixture(t), snapshot: snapshotFixture(t, false, false), deleteContextErr: legacy}
			contextService := NewContextService(mutationPort)
			ref, _ := tobari.ContextRef(contextID)
			intent := intent(TaskContextDelete, operation.EffectWrite, operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: ref}, ContextDeleteImpact())
			_, err = contextService.Delete(context.Background(), intent, ref)
			public, ok = fault.PublicCopy(err)
			if !ok || public.Code != "legacy_state_present" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || mutationPort.calls != 1 {
				t.Fatalf("mutation legacy fault = %#v ok=%t calls=%d", public, ok, mutationPort.calls)
			}
		})
	}
}
