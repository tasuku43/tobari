package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/configuratorcmd"
	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeAssistDraftFixture struct {
	firstEntryConfiguratorDraftFixture
	source tobari.ConfiguratorRuntimeSource
}

func (f runtimeAssistDraftFixture) Freeze(_ context.Context, draft tobari.ConfiguratorDraft) (tobari.ConfiguratorSubmission, error) {
	*f.order = append(*f.order, "draft freeze")
	submission, err := tobari.NewConfiguratorSubmission(draft, f.body)
	if err != nil {
		return tobari.ConfiguratorSubmission{}, err
	}
	return submission.WithRuntimeSource(f.source)
}

type runtimeAssistRunnerFixture struct {
	firstEntryConfiguratorRunnerFixture
	published bool
}

type policyAssistTemplateFixture struct {
	t        *testing.T
	order    *[]string
	template tobari.WorkspaceTemplate
	fp       string
}

type policyAssistContextReadFixture struct {
	finalAuthorityReadFixture
	snapshot tobari.ContextAuthoritySnapshot
}

func (f policyAssistContextReadFixture) ReadContextAuthorityByReference(_ context.Context, ref string) (tobari.ContextAuthoritySnapshot, error) {
	want, err := tobari.ContextRef(f.snapshot.Context.ID)
	if err != nil || ref != want {
		return tobari.ContextAuthoritySnapshot{}, tobari.ErrContextBindingNotFound
	}
	return f.snapshot.Clone(), nil
}

func (f policyAssistContextReadFixture) ReadCurrentContextAuthority(context.Context) (tobari.ContextAuthoritySnapshot, error) {
	return f.snapshot.Clone(), nil
}

func (f policyAssistContextReadFixture) SetCurrentContextByReference(context.Context, string) (tobari.ContextSelectionResult, error) {
	return tobari.ContextSelectionResult{}, errors.New("unexpected current Context mutation")
}

func (f policyAssistTemplateFixture) PlanWorkspaceTemplateSourceByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateChangePlan, error) {
	*f.order = append(*f.order, "template plan")
	id, err := tobari.ParseWorkspaceTemplateRef(ref)
	if err != nil || id != f.template.ID {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	source, err := tobari.NewWorkspaceTemplateSource(f.template)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{f.template.Clone()}, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, err
	}
	return tobari.NewWorkspaceTemplateChangePlan(collection, f.template.ID, source, f.template.Current.Body.EntryDefaults.Runtime, nil, f.fp)
}

func (f policyAssistTemplateFixture) ApplyWorkspaceTemplateSourceByReference(_ context.Context, ref string) (tobari.WorkspaceTemplateRevisionPublication, error) {
	*f.order = append(*f.order, "template apply")
	id, err := tobari.ParseWorkspaceTemplateChangePlanRef(ref)
	if err != nil || id != f.template.ID {
		return tobari.WorkspaceTemplateRevisionPublication{}, err
	}
	current := f.template.Current.Clone()
	return tobari.WorkspaceTemplateRevisionPublication{Template: f.template.Clone(), Previous: current, Current: current, Changed: false}, nil
}

func (f runtimeAssistRunnerFixture) ApplyConfiguratorRuntimeSourceOnly(_ context.Context, _ tobari.ConfiguratorDraft, _ tobari.ConfiguratorRuntimeSource) error {
	*f.order = append(*f.order, "source publish")
	return nil
}
func (f runtimeAssistRunnerFixture) ConfiguratorRuntimeSourcePublished(context.Context, tobari.ConfiguratorDraft, tobari.ConfiguratorRuntimeSource) (bool, error) {
	return f.published, nil
}

func TestRuntimeAssistSeparatesExecutionRuntimeFromUnbuiltTargetAndPublishesSourceOnly(t *testing.T) {
	order := []string{}
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	target := testRuntimeManifest()
	target.RuntimeRef = tobari.RuntimeRef(target.ID)
	baseRevision := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, target.ID, baseRevision)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	source := tobari.ConfiguratorRuntimeSource{
		SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion,
		RuntimeID:     target.ID, BaseRevision: baseRevision,
		FrozenRevision: tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64)), Changed: true,
	}
	drafts := runtimeAssistDraftFixture{firstEntryConfiguratorDraftFixture: firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: seed.Initial}, source: source}
	runner := runtimeAssistRunnerFixture{firstEntryConfiguratorRunnerFixture: firstEntryConfiguratorRunnerFixture{order: &order}}
	readiness := &firstEntryReadinessFixture{order: &order}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{manifest: target})
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, selectionErr: context.Canceled}
	command.finalEntryReadiness = readiness
	command.firstUseSetup = firstEntrySetupFixture{order: &order, choice: firstUseSetupCodex}
	command.configuratorReview = configuratorSubmissionReviewerFixture{order: &order, action: configuratorSubmissionApply}
	command.configurator = configuratorcmd.New(drafts, runner, configuratorStageFixture{order: &order}, runner)

	if code := command.RunContext(context.Background(), []string{"runtime", "assist", "--id", target.RuntimeRef}); code != ExitOK {
		t.Fatalf("runtime assist exit=%d order=%v stdout=%q stderr=%q", code, order, stdout.String(), stderr.String())
	}
	want := []string{"setup", "readiness", "draft reserve", "draft materialize", "runtime prepare", "agent", "draft freeze", "submission review", "task confirm", "source publish", "task settle"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("runtime assist order=%v want=%v", order, want)
	}
	if draft.Runtime != snapshot.Template.Current.Body.EntryDefaults.Runtime || draft.TargetRuntimeID != target.ID || draft.Runtime.RuntimeID == draft.TargetRuntimeID {
		t.Fatalf("execution/target Runtime separation failed: draft=%+v", draft)
	}
	for _, phrase := range []string{"Runtime source reviewed", "editable source only", "runtime build --id " + target.RuntimeRef, target.SourcePath} {
		if !strings.Contains(stdout.String(), phrase) {
			t.Fatalf("Runtime assist output omitted %q: %q", phrase, stdout.String())
		}
	}
}

func TestRuntimeAssistSettlesConfirmedNoOpWithoutRestartingAgent(t *testing.T) {
	order := []string{}
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	target := testRuntimeManifest()
	target.RuntimeRef = tobari.RuntimeRef(target.ID)
	base := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, target.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, seed.Initial)
	if err != nil {
		t.Fatal(err)
	}
	submission, err = submission.WithRuntimeSource(tobari.ConfiguratorRuntimeSource{SchemaVersion: tobari.ConfiguratorRuntimeSourceSchemaVersion, RuntimeID: target.ID, BaseRevision: base, FrozenRevision: base, Changed: false})
	if err != nil {
		t.Fatal(err)
	}
	drafts := runtimeAssistDraftFixture{firstEntryConfiguratorDraftFixture: firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: seed.Initial, taskSubmission: submission, taskFrozen: true, taskConfirmed: true}, source: *submission.RuntimeSource}
	runner := runtimeAssistRunnerFixture{firstEntryConfiguratorRunnerFixture: firstEntryConfiguratorRunnerFixture{order: &order}, published: true}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{manifest: target})
	command.finalDefaultPair = &firstEntryPairFixture{order: &order, selectionErr: context.Canceled}
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseSetup = firstEntrySetupFixture{order: &order, choice: firstUseSetupClaude}
	command.configurator = configuratorcmd.New(drafts, runner, configuratorStageFixture{order: &order}, runner)
	if code := command.RunContext(context.Background(), []string{"runtime", "assist", "--id", target.RuntimeRef, "--agent", "claude"}); code != ExitUnavailable {
		t.Fatalf("mismatched recovery agent exit=%d order=%v stdout=%q stderr=%q", code, order, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(order, []string{}) || !strings.Contains(stderr.String(), "configuration_task_recovery_required") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("mismatched recovery crossed retained agent or fault contract: order=%v stderr=%q", order, stderr.String())
	}
	order = order[:0]
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"runtime", "assist", "--id", target.RuntimeRef}); code != ExitOK {
		t.Fatalf("recovery exit=%d order=%v stdout=%q stderr=%q", code, order, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(order, []string{"task settle"}) {
		t.Fatalf("confirmed no-op restarted mutable work: %v", order)
	}
}

func TestAssistRecoverySentinelMatchesEachPublicCatalogFault(t *testing.T) {
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, DefaultCatalog(), nil)
	for _, test := range []struct {
		name string
		task tobari.ConfiguratorTask
		path string
		err  error
		code string
	}{
		{name: "runtime source", task: tobari.ConfiguratorTaskRuntime, path: "runtime assist", err: tobari.ErrResourceSourceRecoveryRequired, code: "configuration_task_recovery_required"},
		{name: "policy source", task: tobari.ConfiguratorTaskPolicy, path: "policy assist", err: tobari.ErrResourceSourceRecoveryRequired, code: "configuration_task_recovery_required"},
		{name: "runtime attachment", task: tobari.ConfiguratorTaskRuntime, path: "runtime assist", err: tobari.ErrContextBindingProtected, code: "configuration_task_busy"},
		{name: "policy attachment", task: tobari.ConfiguratorTaskPolicy, path: "policy assist", err: tobari.ErrContextBindingProtected, code: "configuration_task_busy"},
		{name: "runtime authority settlement", task: tobari.ConfiguratorTaskRuntime, path: "runtime assist", err: tobari.ErrFinalAuthorityMutationRecoveryRequired, code: "final_authority_mutation_recovery_required"},
		{name: "policy authority settlement", task: tobari.ConfiguratorTaskPolicy, path: "policy assist", err: tobari.ErrFinalAuthorityMutationRecoveryRequired, code: "final_authority_mutation_recovery_required"},
		{name: "runtime unknown observation", task: tobari.ConfiguratorTaskRuntime, path: "runtime assist", err: io.ErrUnexpectedEOF, code: "configuration_task_observation_failed"},
		{name: "policy unknown observation", task: tobari.ConfiguratorTaskPolicy, path: "policy assist", err: io.ErrUnexpectedEOF, code: "configuration_task_observation_failed"},
		{name: "runtime caller cancellation", task: tobari.ConfiguratorTaskRuntime, path: "runtime assist", err: context.Canceled, code: "operation_canceled"},
		{name: "policy caller cancellation", task: tobari.ConfiguratorTaskPolicy, path: "policy assist", err: context.Canceled, code: "operation_canceled"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := command.normalizeFault(withCommandPath(context.Background(), test.path), classifyAssistRecoveryError(test.task, test.err))
			if got.Code != test.code {
				t.Fatalf("normalized recovery fault=%+v", got)
			}
		})
	}
}

func TestAssistCatalogAcceptsExactComposedBoundaryClassifications(t *testing.T) {
	command := newCLI(strings.NewReader(""), io.Discard, io.Discard, DefaultCatalog(), nil)
	next := fault.NextAction{Command: "doctor", Reason: "Inspect the exact source boundary."}
	for _, test := range []struct {
		name   string
		path   string
		public error
	}{
		{name: "policy Context absence", path: "policy assist", public: fault.WithClassification(fault.New(fault.KindNotFound, "context_not_found", "missing", false, next), fault.PhaseObservation, fault.ChangeNotApplicable)},
		{name: "policy Context read", path: "policy assist", public: fault.WithClassification(fault.New(fault.KindUnavailable, "context_read_failed", "unavailable", false, next), fault.PhaseObservation, fault.ChangeNotApplicable)},
		{name: "policy Template plan read", path: "policy assist", public: fault.WithClassification(fault.New(fault.KindUnavailable, "template_plan_read_failed", "unavailable", false, next), fault.PhaseObservation, fault.ChangeNotApplicable)},
		{name: "policy Template recovery", path: "policy assist", public: fault.WithClassification(fault.New(fault.KindUnavailable, "resource_source_recovery_required", "partial", false, next), fault.PhaseMutation, fault.ChangePartial)},
		{name: "policy Template result", path: "policy assist", public: fault.WithClassification(fault.New(fault.KindContract, "invalid_template_apply_result", "invalid", false, next), fault.PhaseVerification, fault.ChangeUnknown)},
		{name: "policy boundary output", path: "policy assist", public: configuratorBoundaryOutputFaultFor("policy assist", io.ErrClosedPipe)},
		{name: "Runtime lifecycle observation", path: "runtime assist", public: fault.WithClassification(fault.New(fault.KindRejected, "runtime_retirement_observation_unknown", "unknown", false, next), fault.PhaseObservation, fault.ChangeNotApplicable)},
		{name: "Runtime readiness", path: "runtime assist", public: fault.WithClassification(fault.New(fault.KindRejected, "runtime_not_ready", "not ready", false, next), fault.PhasePrecondition, fault.ChangeNone)},
		{name: "Runtime source recovery", path: "runtime assist", public: fault.WithClassification(fault.New(fault.KindUnavailable, "resource_source_recovery_required", "partial", false, next), fault.PhaseMutation, fault.ChangePartial)},
		{name: "Runtime boundary output", path: "runtime assist", public: configuratorBoundaryOutputFaultFor("runtime assist", io.ErrClosedPipe)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := command.normalizeFault(withCommandPath(context.Background(), test.path), test.public)
			if got.Code == "undeclared_fault_contract" {
				t.Fatalf("%s rejected composed fault: %+v", test.path, got)
			}
		})
	}
}

func TestAssistCatalogDeclaresSharedReadinessFaults(t *testing.T) {
	for _, path := range []string{"runtime assist", "policy assist"} {
		spec, found := DefaultCatalog().lookupRegistered(path)
		if !found {
			t.Fatalf("%s is absent", path)
		}
		declared := map[string]bool{}
		for _, item := range spec.Agent.Errors {
			declared[item.Code] = true
		}
		for _, code := range []string{"docker_cli_unavailable", "docker_engine_unavailable", "docker_context_unavailable", "docker_compose_unavailable", "docker_engine_incompatible", "invalid_readiness_profile", "invalid_readiness_observation"} {
			if !declared[code] {
				t.Errorf("%s does not declare %s", path, code)
			}
		}
		if declared["docker_manifest_unavailable"] {
			t.Errorf("%s retains retired docker_manifest_unavailable", path)
		}
	}
}

func TestPolicyAssistUsesContextRuntimeAndCanonicalTemplatePlanApply(t *testing.T) {
	order := []string{}
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, snapshot.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, seed.Initial)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := strings.Repeat("d", 64)
	stageRef, _ := tobari.WorkspaceTemplateRef(snapshot.Template.ID)
	stagePort := configuratorStageFixture{order: &order, ref: stageRef, fingerprint: fingerprint}
	drafts := firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: seed.Initial}
	runner := firstEntryConfiguratorRunnerFixture{order: &order}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalContexts = workspaceauthoritycmd.NewContextService(policyAssistContextReadFixture{snapshot: snapshot})
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.firstUseSetup = firstEntrySetupFixture{order: &order, choice: firstUseSetupClaude}
	command.configuratorReview = configuratorSubmissionReviewerFixture{order: &order, action: configuratorSubmissionApply}
	command.configuratorPlanReview = configuratorPlanReviewerFixture{order: &order}
	command.configurator = configuratorcmd.New(drafts, runner, stagePort, runner)
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(policyAssistTemplateFixture{t: t, order: &order, template: snapshot.Template, fp: fingerprint})

	if code := command.RunContext(context.Background(), []string{"policy", "assist"}); code != ExitOK {
		t.Fatalf("policy assist exit=%d order=%v stdout=%q stderr=%q", code, order, stdout.String(), stderr.String())
	}
	want := []string{"setup", "readiness", "draft reserve", "draft materialize", "runtime prepare", "agent", "draft freeze", "submission review", "stage", "template plan", "plan review", "task confirm", "template apply", "task settle"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("policy assist order=%v want=%v submission=%+v", order, want, submission)
	}
	for _, phrase := range []string{"Static policy reviewed", "Policy Memory", "unchanged", "tobari"} {
		if !strings.Contains(stdout.String(), phrase) {
			t.Fatalf("Policy assist output omitted %q: %q", phrase, stdout.String())
		}
	}
}

func TestPolicyAssistResumesExactPendingTemplateApplyWithoutAgentOrReplan(t *testing.T) {
	order := []string{}
	snapshot := finalCurrentContextEntrySnapshotFixture(t)
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentClaude, snapshot.Template.ID)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, seed.Initial)
	if err != nil {
		t.Fatal(err)
	}
	planRef := "wtplan1_" + string(snapshot.Template.ID) + "_" + strings.Repeat("d", 64)
	drafts := firstEntryConfiguratorDraftFixture{order: &order, draft: draft, body: seed.Initial, taskSubmission: submission, taskFrozen: true, taskConfirmed: true}
	stagePort := configuratorStageFixture{order: &order, policyPendingPlanRef: planRef}
	contextRef, err := tobari.ContextRef(snapshot.Context.ID)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.finalContexts = workspaceauthoritycmd.NewContextService(policyAssistContextReadFixture{snapshot: snapshot})
	command.finalEntryReadiness = &firstEntryReadinessFixture{order: &order}
	command.configurator = configuratorcmd.New(drafts, firstEntryConfiguratorRunnerFixture{order: &order}, stagePort, firstEntryConfiguratorRunnerFixture{order: &order})
	command.finalTemplates = workspaceauthoritycmd.NewTemplateService(policyAssistTemplateFixture{t: t, order: &order, template: snapshot.Template})

	if code := command.RunContext(context.Background(), []string{"policy", "assist", "--context", contextRef, "--agent", "claude"}); code != ExitOK {
		t.Fatalf("pending Apply recovery exit=%d order=%v stdout=%q stderr=%q", code, order, stdout.String(), stderr.String())
	}
	if want := []string{"template apply", "task settle"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("pending Apply recovery order=%v want=%v", order, want)
	}
}
