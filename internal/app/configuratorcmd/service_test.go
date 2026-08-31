package configuratorcmd

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type draftFixture struct {
	draft            tobari.ConfiguratorDraft
	submission       tobari.ConfiguratorSubmission
	reserveCalls     int
	materializeCalls int
	freezeCalls      int
	adoptCalls       int
	order            *[]string
	pending          *tobari.ConfiguratorSubmission
	taskDraft        tobari.ConfiguratorDraft
	taskSubmission   tobari.ConfiguratorSubmission
	taskFrozen       bool
	taskConfirmed    bool
	completeErr      error
	completeCanceled bool
	completeWait     bool
	confirmErr       error
}

func (f *draftFixture) Reserve(context.Context, tobari.ConfiguratorSeed, tobari.ConfiguratorAgent) (tobari.ConfiguratorDraft, error) {
	f.reserveCalls++
	if f.order != nil {
		*f.order = append(*f.order, "reserve")
	}
	return f.draft, nil
}
func (f *draftFixture) Materialize(context.Context, tobari.ConfiguratorDraft) error {
	f.materializeCalls++
	if f.order != nil {
		*f.order = append(*f.order, "materialize")
	}
	return nil
}
func (f *draftFixture) Freeze(context.Context, tobari.ConfiguratorDraft) (tobari.ConfiguratorSubmission, error) {
	f.freezeCalls++
	return f.submission, nil
}
func (f *draftFixture) PendingTask(context.Context, string, tobari.ConfiguratorTask, string) (tobari.ConfiguratorDraft, tobari.ConfiguratorSubmission, bool, bool, error) {
	return f.taskDraft, f.taskSubmission, f.taskFrozen, f.taskConfirmed, nil
}
func (f *draftFixture) ConfirmTask(context.Context, tobari.ConfiguratorSubmission) error {
	if f.order != nil {
		*f.order = append(*f.order, "task confirm")
	}
	return f.confirmErr
}
func (f *draftFixture) CompleteTask(ctx context.Context, _ tobari.ConfiguratorSubmission) error {
	if f.order != nil {
		*f.order = append(*f.order, "task settle")
	}
	f.completeCanceled = ctx.Err() != nil
	if f.completeWait {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.completeErr
}
func (f *draftFixture) RetireUnmaterializedTask(context.Context, tobari.ConfiguratorDraft) error {
	if f.order != nil {
		*f.order = append(*f.order, "task retire")
	}
	return nil
}
func (f *draftFixture) ArmHomeAdoption(context.Context, tobari.ConfiguratorSubmission) error {
	if f.order != nil {
		*f.order = append(*f.order, "home arm")
	}
	return nil
}
func (f *draftFixture) PendingHomeAdoption(context.Context, string) (tobari.ConfiguratorSubmission, bool, error) {
	if f.pending != nil {
		return *f.pending, true, nil
	}
	return tobari.ConfiguratorSubmission{}, false, nil
}
func (f *draftFixture) AdoptHome(_ context.Context, _ tobari.ConfiguratorSubmission, _ tobari.ContextAuthoritySnapshot, settle ...func() error) error {
	f.adoptCalls++
	if len(settle) > 0 && settle[0] != nil {
		return settle[0]()
	}
	return nil
}

type runnerFixture struct {
	prepareCalls    int
	prepareErr      error
	runCalls        int
	runErr          error
	acquireErr      error
	sourceOnlyErr   error
	sourceOnlyHook  func()
	sourcePublished bool
	order           *[]string
}

type submissionFixture struct {
	stage              tobari.ConfiguratorStage
	calls              int
	order              *[]string
	pendingPublication *tobari.ConfiguratorSubmission
	policyPublished    bool
	policyPendingPlan  string
}

func (f *submissionFixture) StageConfiguratorSubmission(context.Context, tobari.ConfiguratorSubmission) (tobari.ConfiguratorStage, error) {
	f.calls++
	return f.stage, nil
}
func (f *submissionFixture) DiscardConfiguratorStage(context.Context, tobari.ConfiguratorSubmission, tobari.ConfiguratorStage) error {
	return nil
}
func (f *submissionFixture) PendingConfiguratorStage(context.Context, tobari.WorkspaceTemplateID) (tobari.ConfiguratorPendingStage, bool, error) {
	return tobari.ConfiguratorPendingStage{}, false, nil
}
func (f *submissionFixture) PendingConfiguratorStageForProject(context.Context, string) (tobari.ConfiguratorPendingStage, bool, error) {
	return tobari.ConfiguratorPendingStage{}, false, nil
}
func (f *submissionFixture) BindConfiguratorStagePlan(_ context.Context, pending tobari.ConfiguratorPendingStage, planRef string) (tobari.ConfiguratorPendingStage, error) {
	pending.PlanRef = planRef
	return pending, pending.Validate()
}
func (f *submissionFixture) ConfirmConfiguratorStageApply(_ context.Context, pending tobari.ConfiguratorPendingStage) (tobari.ConfiguratorPendingStage, error) {
	pending.ApplyConfirmed = true
	return pending, pending.Validate()
}
func (f *submissionFixture) ConfirmConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission, tobari.ContextAuthoritySnapshot) error {
	return nil
}
func (f *submissionFixture) BeginConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error {
	if f.order != nil {
		*f.order = append(*f.order, "publication barrier")
	}
	return nil
}
func (f *submissionFixture) CompleteConfiguratorPublication(context.Context, tobari.ConfiguratorSubmission) error {
	return nil
}
func (f *submissionFixture) PendingConfiguratorPublicationForProject(context.Context, string) (tobari.ConfiguratorSubmission, bool, error) {
	if f.pendingPublication != nil {
		return *f.pendingPublication, true, nil
	}
	return tobari.ConfiguratorSubmission{}, false, nil
}
func (f *submissionFixture) ConfiguratorPolicyPublished(context.Context, tobari.ConfiguratorSubmission) (bool, string, error) {
	return f.policyPublished, f.policyPendingPlan, nil
}

func (f *runnerFixture) PrepareConfiguratorRuntime(_ context.Context, binding tobari.RuntimeBinding) error {
	f.prepareCalls++
	if f.order != nil {
		*f.order = append(*f.order, "runtime prepare")
	}
	if f.prepareErr != nil {
		return f.prepareErr
	}
	return binding.Validate()
}
func (f *runnerFixture) ConfiguratorRuntimeSourcePublished(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource) (bool, error) {
	return f.sourcePublished, nil
}
func (f *runnerFixture) AcquireConfiguratorAuthorAttachment(context.Context, tobari.ConfiguratorDraft) (func() error, error) {
	if f.order != nil {
		*f.order = append(*f.order, "attachment")
	}
	if f.acquireErr != nil {
		return nil, f.acquireErr
	}
	return func() error { return nil }, nil
}
func (f *runnerFixture) AcquireConfiguratorPublicationAttachment(context.Context, tobari.ConfiguratorSubmission) (func() error, error) {
	return func() error { return nil }, nil
}
func (f *runnerFixture) ApplyConfiguratorRuntimeSource(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource, io.Writer) (tobari.RuntimeBinding, error) {
	return tobari.RuntimeBinding{}, nil
}
func (f *runnerFixture) ApplyConfiguratorRuntimeSourceOnly(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource) error {
	if f.sourceOnlyHook != nil {
		f.sourceOnlyHook()
	}
	return f.sourceOnlyErr
}
func (f *runnerFixture) RunConfigurator(_ context.Context, _ tobari.ConfiguratorDraft, isolation tobari.ConfiguratorIsolation, _ io.Reader, _ io.Writer) error {
	f.runCalls++
	if f.runErr != nil {
		return f.runErr
	}
	return isolation.Validate()
}

func TestAuthorUsesOneValidatedDirectEgressBoundaryAndFreezesSubmission(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, body)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	drafts := &draftFixture{draft: draft, submission: submission, order: &order}
	runner := &runnerFixture{order: &order}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	got, err := service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if got.Revision != submission.Revision || drafts.reserveCalls != 1 || drafts.materializeCalls != 1 || runner.prepareCalls != 1 || runner.runCalls != 1 || drafts.freezeCalls != 1 {
		t.Fatalf("submission/calls got=%+v reserve=%d materialize=%d runtime=%d run=%d freeze=%d", got, drafts.reserveCalls, drafts.materializeCalls, runner.prepareCalls, runner.runCalls, drafts.freezeCalls)
	}
	if !reflect.DeepEqual(order, []string{"reserve", "attachment", "materialize", "runtime prepare"}) {
		t.Fatalf("author boundary order=%v", order)
	}
}

func TestAuthorAcquiresAttachmentBeforeManagedHomeMaterialization(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	drafts := &draftFixture{draft: draft, order: &order}
	runner := &runnerFixture{acquireErr: tobari.ErrContextBindingProtected, order: &order}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	if _, err := service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentClaude, nil, io.Discard); err == nil {
		t.Fatalf("attachment rejection error = %v", err)
	}
	if !reflect.DeepEqual(order, []string{"reserve", "attachment"}) || drafts.materializeCalls != 0 {
		t.Fatalf("order=%v materialize=%d", order, drafts.materializeCalls)
	}
}

func TestAuthorPreservesNativeLoginBridgeLossAsRetainedMaterialFault(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	drafts := &draftFixture{draft: draft}
	runner := &runnerFixture{runErr: tobari.ErrNativeLoginBridgeUnavailable}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	_, err = service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_material_retained" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeConfirmed || public.Retryable {
		t.Fatalf("bridge-loss outcome=%+v ok=%v err=%v", public, ok, err)
	}
	if drafts.materializeCalls != 1 || drafts.freezeCalls != 0 || runner.runCalls != 1 {
		t.Fatalf("bridge-loss calls materialize=%d freeze=%d run=%d", drafts.materializeCalls, drafts.freezeCalls, runner.runCalls)
	}
}

func TestAuthorPreservesCallerCancellationAfterMaterializationAsRetainedMaterialFault(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	drafts := &draftFixture{draft: draft}
	runner := &runnerFixture{runErr: context.Canceled}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	_, err = service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_material_retained" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeConfirmed || public.Retryable {
		t.Fatalf("cancel outcome=%+v ok=%v err=%v", public, ok, err)
	}
	if drafts.materializeCalls != 1 || drafts.freezeCalls != 0 || runner.runCalls != 1 {
		t.Fatalf("cancel calls materialize=%d freeze=%d run=%d", drafts.materializeCalls, drafts.freezeCalls, runner.runCalls)
	}
}

func TestAuthorPreservesRuntimePreparationFailureAfterMaterializationAsRetainedMaterialFault(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	drafts := &draftFixture{draft: draft}
	runner := &runnerFixture{prepareErr: fault.New(fault.KindUnavailable, "runtime_image_unavailable", "unavailable", true)}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	_, err = service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_material_retained" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeConfirmed || public.Retryable {
		t.Fatalf("Runtime preparation outcome=%+v ok=%v err=%v", public, ok, err)
	}
	if drafts.materializeCalls != 1 || drafts.freezeCalls != 0 || runner.prepareCalls != 1 || runner.runCalls != 0 {
		t.Fatalf("Runtime preparation calls materialize=%d freeze=%d prepare=%d run=%d", drafts.materializeCalls, drafts.freezeCalls, runner.prepareCalls, runner.runCalls)
	}
}

func TestAssistRetainedAndCleanupRecoveryRemainTaskScoped(t *testing.T) {
	for _, test := range []struct {
		name    string
		task    tobari.ConfiguratorTask
		command string
		target  string
	}{
		{name: "Runtime", task: tobari.ConfiguratorTaskRuntime, command: "help runtime assist", target: "Runtime reference"},
		{name: "Policy", task: tobari.ConfiguratorTaskPolicy, command: "help policy assist", target: "Context reference"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, outcome := range []error{MaterialRetainedFault(test.task, context.Canceled), CleanupIncompleteFault(test.task, io.ErrClosedPipe)} {
				public, ok := fault.PublicCopy(outcome)
				if !ok || len(public.NextActions) != 1 || public.NextActions[0].Command != test.command || !strings.Contains(public.NextActions[0].Reason, test.target) || public.NextActions[0].Command == "status" {
					t.Fatalf("task-scoped recovery=%+v ok=%v", public, ok)
				}
			}
		})
	}
}

func TestAuthorReportsTransientCleanupUncertaintyBeforeBridgeLoss(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	drafts := &draftFixture{draft: draft}
	runner := &runnerFixture{runErr: errors.Join(tobari.ErrNativeLoginBridgeUnavailable, tobari.ErrConfiguratorTransientCleanupUnknown)}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	_, err = service.Author(context.Background(), intent, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_cleanup_incomplete" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangePartial || public.Retryable {
		t.Fatalf("cleanup outcome=%+v ok=%v err=%v", public, ok, err)
	}
}

func TestAuthorRejectsIntentBeforeDraftMutation(t *testing.T) {
	drafts := &draftFixture{}
	runner := &runnerFixture{}
	service := New(drafts, runner, nil, runner)
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", configuratorBodyFixture())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Author(context.Background(), operation.Intent{}, seed, tobari.ConfiguratorAgentCodex, nil, io.Discard); err == nil {
		t.Fatal("invalid intent accepted")
	}
	if drafts.reserveCalls != 0 || drafts.materializeCalls != 0 {
		t.Fatalf("draft calls reserve=%d materialize=%d", drafts.reserveCalls, drafts.materializeCalls)
	}
}

func TestPrepareRuntimeClassifiesPostBoundaryCancellationAsUnknownMutation(t *testing.T) {
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", configuratorBodyFixture())
	if err != nil {
		t.Fatal(err)
	}
	runner := &runnerFixture{prepareErr: context.Canceled}
	service := New(&draftFixture{}, runner, nil, runner)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	err = service.PrepareRuntime(context.Background(), intent, seed)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "unclassified_mutation_outcome" || public.Phase != fault.PhaseMutation || public.ChangeState != fault.ChangeUnknown || public.Retryable {
		t.Fatalf("Prepare Runtime outcome=%+v ok=%v", public, ok)
	}
}

func TestRuntimeAssistClassifiesExactSourceCASDriftBeforePublication(t *testing.T) {
	body := configuratorBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	targetID := "018bcfe5-687b-7000-8000-000000000077"
	base := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, targetID, base)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, tobari.WorkspaceTemplateBody{})
	if err != nil {
		t.Fatal(err)
	}
	submission, err = submission.WithRuntimeSource(tobari.ConfiguratorRuntimeSource{
		SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: targetID,
		BaseRevision: base, FrozenRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64)), Changed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	drafts := &draftFixture{draft: draft, submission: submission}
	runner := &runnerFixture{sourceOnlyErr: tobari.ErrResourceSourceChanged}
	service := New(drafts, runner, nil, runner)
	intent := operation.Intent{Command: "runtime assist", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: tobari.RuntimeRef(targetID)}, Impact: Impact()}
	err = service.ApplyRuntimeAssistSource(context.Background(), intent, submission, taskSettlementFactoryTest)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "resource_source_changed" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone || !public.Retryable {
		t.Fatalf("Runtime source CAS outcome=%+v ok=%v err=%v", public, ok, err)
	}
}

func TestStageAcceptsOnlyExactEvolveSubmissionAndTemplateReference(t *testing.T) {
	body := configuratorBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	workspace := tobari.WorkspaceBinding{SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789ad"), ContextID: contextID, ProjectRoot: "/workspace/example", Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory, Workspace: &workspace}
	seed, err := tobari.NewEvolveConfiguratorSeed("/workspace/example", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, body)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := tobari.WorkspaceTemplateRef(templateID)
	stage := tobari.ConfiguratorStage{SchemaVersion: tobari.ConfiguratorStageSchemaVersion, TemplateRef: ref, SourceRevision: submission.SourceRevision, SourceFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	stager := &submissionFixture{stage: stage}
	service := New(&draftFixture{}, &runnerFixture{}, stager, nil)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	got, err := service.Stage(context.Background(), intent, submission)
	if err != nil || got != stage || stager.calls != 1 {
		t.Fatalf("stage=%+v calls=%d err=%v", got, stager.calls, err)
	}
}

func TestHomeAdoptionBarrierPrecedesHomeArmAndRecoversBarrierOnlyCrash(t *testing.T) {
	body := configuratorBodyFixture()
	seed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "01912345-6789-7abc-8def-0123456789ab", "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, body)
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	drafts := &draftFixture{order: &order}
	stager := &submissionFixture{order: &order, pendingPublication: &submission}
	service := New(drafts, &runnerFixture{}, stager, nil)
	intent := operation.Intent{Command: "configure", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ProjectConfigurationTargetKind, ID: tobari.ProjectConfigurationTargetID}, Impact: Impact()}
	if err := service.ArmHomeAdoption(context.Background(), intent, submission); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "publication barrier" || order[1] != "home arm" {
		t.Fatalf("arm order=%v", order)
	}
	pending, found, err := service.PendingHomeAdoption(context.Background(), draft.ProjectRoot)
	if err != nil || !found || pending.Revision != submission.Revision {
		t.Fatalf("barrier-only recovery=%+v found=%v err=%v", pending, found, err)
	}
}

func TestRuntimeAssistSettlementSurvivesCallerCancellationAfterPublication(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	order := []string{}
	drafts := &draftFixture{order: &order}
	ctx, cancel := context.WithCancel(context.Background())
	runner := &runnerFixture{sourceOnlyHook: cancel}
	service := New(drafts, runner, nil, runner)
	if err := service.ApplyRuntimeAssistSource(ctx, intent, submission, taskSettlementFactoryTest); err != nil {
		t.Fatal(err)
	}
	if drafts.completeCanceled || !reflect.DeepEqual(order, []string{"task confirm", "task settle"}) {
		t.Fatalf("settlement used canceled caller context or wrong order: canceled=%v order=%v", drafts.completeCanceled, order)
	}
}

func TestRuntimeAssistSettlementFailurePreservesConfirmedPublication(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	drafts := &draftFixture{completeErr: errors.New("disk unavailable")}
	runner := &runnerFixture{}
	service := New(drafts, runner, nil, runner)
	err := service.ApplyRuntimeAssistSource(context.Background(), intent, submission, taskSettlementFactoryTest)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_task_settlement_incomplete" || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeConfirmed {
		t.Fatalf("settlement outcome=%+v ok=%v err=%v", public, ok, err)
	}
}

func TestRuntimeAssistMissingSettlementContextPreservesConfirmedPublication(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	drafts := &draftFixture{}
	runner := &runnerFixture{}
	err := New(drafts, runner, nil, runner).ApplyRuntimeAssistSource(context.Background(), intent, submission)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_task_settlement_incomplete" || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeConfirmed {
		t.Fatalf("missing settlement context outcome=%+v ok=%v err=%v", public, ok, err)
	}
}

func TestRuntimeAssistSettlementDeadlinePreservesConfirmedPublication(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	drafts := &draftFixture{completeWait: true}
	runner := &runnerFixture{}
	shortSettlement := func() (context.Context, context.CancelFunc) {
		return context.WithTimeout(context.Background(), 10*time.Millisecond)
	}
	err := New(drafts, runner, nil, runner).ApplyRuntimeAssistSource(context.Background(), intent, submission, shortSettlement)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "configuration_task_settlement_incomplete" || public.ChangeState != fault.ChangeConfirmed {
		t.Fatalf("deadline settlement outcome=%+v ok=%v err=%v", public, ok, err)
	}
}

func TestCompletePublishedTaskIgnoresAlreadyCanceledCaller(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	order := []string{}
	drafts := &draftFixture{order: &order}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := New(drafts, &runnerFixture{}, nil, &runnerFixture{}).CompleteTask(ctx, intent, submission, taskSettlementFactoryTest); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"task settle"}) || drafts.completeCanceled {
		t.Fatalf("detached settlement did not reach Store: order=%v canceled=%v", order, drafts.completeCanceled)
	}
}

func TestReconcileRuntimeTaskSettlesOnlyAuthorityVerifiedPublication(t *testing.T) {
	submission, intent := runtimeAssistSubmissionForTaskTest(t)
	order := []string{}
	drafts := &draftFixture{order: &order, taskDraft: submission.Draft, taskSubmission: submission, taskFrozen: true, taskConfirmed: true}
	runner := &runnerFixture{sourcePublished: true}
	service := New(drafts, runner, nil, runner)
	recovery, found, err := service.ReconcileTask(context.Background(), seedForTaskRecovery(t, submission))
	if err != nil || !found || !recovery.Published || len(order) != 0 {
		t.Fatalf("Runtime reconciliation=%+v found=%v canceled=%v err=%v", recovery, found, drafts.completeCanceled, err)
	}
	if err := service.CompleteTask(context.Background(), intent, recovery.Submission, taskSettlementFactoryTest); err != nil {
		t.Fatal(err)
	}
}

func TestReconcilePolicyTaskRejectsInvalidPendingPlanTuplesAtPortBoundary(t *testing.T) {
	submission := policyAssistSubmissionForTaskTest(t)
	exactPlan := "wtplan1_" + string(submission.Draft.TemplateID) + "_" + strings.Repeat("c", 64)
	otherTemplate := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789dd")
	for _, test := range []struct {
		name      string
		published bool
		plan      string
	}{
		{name: "malformed", plan: "not-a-plan"},
		{name: "other Template", plan: "wtplan1_" + string(otherTemplate) + "_" + strings.Repeat("d", 64)},
		{name: "published and pending", published: true, plan: exactPlan},
	} {
		t.Run(test.name, func(t *testing.T) {
			drafts := &draftFixture{taskDraft: submission.Draft, taskSubmission: submission, taskFrozen: true, taskConfirmed: true}
			stager := &submissionFixture{policyPublished: test.published, policyPendingPlan: test.plan}
			_, _, err := New(drafts, &runnerFixture{}, stager, &runnerFixture{}).ReconcileTask(context.Background(), seedForTaskRecovery(t, submission))
			if !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
				t.Fatalf("invalid pending Plan tuple error=%v", err)
			}
		})
	}

	drafts := &draftFixture{taskDraft: submission.Draft, taskSubmission: submission, taskFrozen: true, taskConfirmed: true}
	recovery, found, err := New(drafts, &runnerFixture{}, &submissionFixture{policyPendingPlan: exactPlan}, &runnerFixture{}).ReconcileTask(context.Background(), seedForTaskRecovery(t, submission))
	if err != nil || !found || recovery.PendingPlanRef != exactPlan || recovery.Published {
		t.Fatalf("exact pending Plan recovery=%+v found=%v err=%v", recovery, found, err)
	}
}

func runtimeAssistSubmissionForTaskTest(t *testing.T) (tobari.ConfiguratorSubmission, operation.Intent) {
	t.Helper()
	body := configuratorBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	targetID := "018bcfe5-687b-7000-8000-000000000077"
	base := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, targetID, base)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, tobari.WorkspaceTemplateBody{})
	if err != nil {
		t.Fatal(err)
	}
	submission, err = submission.WithRuntimeSource(tobari.ConfiguratorRuntimeSource{SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: targetID, BaseRevision: base, FrozenRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64)), Changed: true})
	if err != nil {
		t.Fatal(err)
	}
	intent := operation.Intent{Command: "runtime assist", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: tobari.RuntimeRef(targetID)}, Impact: Impact()}
	return submission, intent
}

func seedForTaskRecovery(t *testing.T, submission tobari.ConfiguratorSubmission) tobari.ConfiguratorSeed {
	t.Helper()
	if submission.Draft.Task == tobari.ConfiguratorTaskRuntime {
		seed, err := tobari.NewRuntimeAssistConfiguratorSeed(submission.Draft.Runtime, submission.Draft.TargetRuntimeID, submission.Draft.TargetRuntimeRevision)
		if err != nil {
			t.Fatal(err)
		}
		return seed
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(submission.Draft.TemplateID, 1, submission.Body)
	if err != nil || revision.Revision != submission.Draft.BaseTemplateRevision {
		t.Fatalf("reconstruct policy Template revision: revision=%+v err=%v", revision, err)
	}
	memory, _, err := tobari.PublishPolicyMemory(submission.Draft.ContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil || memory.Revision != submission.Draft.BasePolicyMemoryRevision {
		t.Fatalf("reconstruct policy memory revision: revision=%+v err=%v", memory, err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: submission.Draft.ContextID, TemplateID: submission.Draft.TemplateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: submission.Draft.TemplateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return seed
}

func policyAssistSubmissionForTaskTest(t *testing.T) tobari.ConfiguratorSubmission {
	t.Helper()
	body := configuratorBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, body)
	if err != nil {
		t.Fatal(err)
	}
	return submission
}

func taskSettlementFactoryTest() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func configuratorBodyFixture() tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary:        tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:          tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:   tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Ordinal: 1, Image: "tobari-runtime:test"}},
		SessionDefaults: tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}}, CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}
