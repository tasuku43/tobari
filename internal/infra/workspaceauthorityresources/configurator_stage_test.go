package workspaceauthorityresources

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritysource"
)

func TestTemplateApplyRequiresExactConfirmedConfiguratorPlan(t *testing.T) {
	const plan = "wtplan1_01912345-6789-7abc-8def-0123456789ab_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, test := range []struct {
		name    string
		receipt workspaceauthoritysource.ConfiguratorStageReceipt
		present bool
		wantErr bool
	}{
		{name: "ordinary source", present: false},
		{name: "review pending", present: true, receipt: workspaceauthoritysource.ConfiguratorStageReceipt{PlanRef: plan}},
		{name: "another plan", present: true, receipt: workspaceauthoritysource.ConfiguratorStageReceipt{PlanRef: plan + "0", ApplyConfirmed: true}},
		{name: "exact confirmed", present: true, receipt: workspaceauthoritysource.ConfiguratorStageReceipt{PlanRef: plan, ApplyConfirmed: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateConfiguratorStageApplyAuthorization(test.receipt, test.present, plan)
			if test.name == "ordinary source" || test.name == "exact confirmed" {
				if err != nil {
					t.Fatalf("authorized Apply rejected: %v", err)
				}
				return
			}
			if !errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale) {
				t.Fatalf("unconfirmed Configurator Apply error=%v", err)
			}
		})
	}
}

func TestTaskStageDiscoveryIgnoresPreReleaseAggregateReceipt(t *testing.T) {
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{}
	receipt.Submission.Draft.ProjectRoot = "/workspace/example"
	receipt.Submission.Draft.Task = tobari.ConfiguratorTaskAggregate
	if isTaskScopedProjectStage(receipt, receipt.Submission.Draft.ProjectRoot) {
		t.Fatal("pre-release aggregate stage entered task-scoped recovery discovery")
	}
	receipt.Submission.Draft.Task = tobari.ConfiguratorTaskPolicy
	if !isTaskScopedProjectStage(receipt, receipt.Submission.Draft.ProjectRoot) {
		t.Fatal("task-scoped policy stage was hidden from recovery discovery")
	}
}

func TestPolicyAssistStageRecoveryWaitsForExactConfirmedApplySettlement(t *testing.T) {
	submission := tobari.ConfiguratorSubmission{}
	submission.Draft.Task = tobari.ConfiguratorTaskPolicy
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{Submission: submission}
	if confirmed, err := validatePolicyAssistStageRecoveryReceipt(receipt, submission); err != nil || confirmed {
		t.Fatalf("unconfirmed receipt confirmed=%v err=%v", confirmed, err)
	}
	receipt.ApplyConfirmed = true
	receipt.PlanRef = "wtplan1_01912345-6789-7abc-8def-0123456789ab_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if confirmed, err := validatePolicyAssistStageRecoveryReceipt(receipt, submission); err != nil || !confirmed {
		t.Fatalf("confirmed receipt confirmed=%v err=%v", confirmed, err)
	}
	different := submission
	different.Revision = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := validatePolicyAssistStageRecoveryReceipt(receipt, different); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("different frozen submission error=%v", err)
	}
	if settle, err := policyAssistStageCanSettle(receipt, "", false); err != nil || !settle {
		t.Fatalf("settled canonical Apply did not release Stage: settle=%v err=%v", settle, err)
	}
	if settle, err := policyAssistStageCanSettle(receipt, receipt.PlanRef, true); err != nil || settle {
		t.Fatalf("pending exact Apply settlement was skipped: settle=%v err=%v", settle, err)
	}
	if _, err := policyAssistStageCanSettle(receipt, receipt.PlanRef+"0", true); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("different pending Apply settlement error=%v", err)
	}
}

func TestPolicyAssistStageProtectsItsContextFromDeletionUntilSettlement(t *testing.T) {
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	otherID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ad")
	receipt := workspaceauthoritysource.ConfiguratorStageReceipt{}
	receipt.Submission.Draft.ContextID = contextID
	if !configuratorStageProtectsContextDelete(receipt, true, contextID) {
		t.Fatal("unconfirmed policy Stage did not protect its Context")
	}
	receipt.ApplyConfirmed = true
	if !configuratorStageProtectsContextDelete(receipt, true, contextID) {
		t.Fatal("confirmed policy Stage did not protect its Context")
	}
	if configuratorStageProtectsContextDelete(receipt, true, otherID) {
		t.Fatal("policy Stage protected an unrelated Context")
	}
	if configuratorStageProtectsContextDelete(receipt, false, contextID) {
		t.Fatal("settled policy Stage continued protecting its Context")
	}
}

func TestContextDeleteKeepsTemplateStageLeaseUntilCallerFinishesDeletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback serializes through the process-level adapter path")
	}
	sources, err := workspaceauthoritysource.New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{sources: sources}
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	releaseDelete, err := adapter.acquireContextDeleteTemplateStageBarrierLocked(context.Background(), contextID, templateID)
	if err != nil {
		t.Fatal(err)
	}
	if releaseStage, acquireErr := sources.AcquireConfiguratorStageLease(context.Background(), templateID); !errors.Is(acquireErr, tobari.ErrContextBindingProtected) {
		if acquireErr == nil {
			_ = releaseStage()
		}
		t.Fatalf("Stage entered while Context deletion owned the lease: %v", acquireErr)
	}
	if err := releaseDelete(); err != nil {
		t.Fatal(err)
	}
	releaseStage, err := sources.AcquireConfiguratorStageLease(context.Background(), templateID)
	if err != nil {
		t.Fatalf("settled Context deletion did not release Stage lease: %v", err)
	}
	if err := releaseStage(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateDeleteRetiresUnpublishedCreateAndCopyDraftSources(t *testing.T) {
	sources, err := workspaceauthoritysource.New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{sources: sources}
	body := realResourcesMigrationCollection(t, t.TempDir()).Templates[0].Current.Body
	tests := []struct {
		name         string
		templateName string
		id           tobari.WorkspaceTemplateID
		authorityErr error
	}{
		{name: "create draft before first authority", templateName: "create-draft", id: "01912345-6789-7abc-8def-0123456789d1", authorityErr: tobari.ErrFinalAuthorityNotFound},
		{name: "copy draft beside active authority", templateName: "copy-draft", id: "01912345-6789-7abc-8def-0123456789d2", authorityErr: tobari.ErrWorkspaceTemplateNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, err := tobari.NewWorkspaceTemplateDraftSource(test.id, test.templateName, body)
			if err != nil {
				t.Fatal(err)
			}
			if err := sources.PublishTemplate(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			result, handled, err := adapter.deleteUnpublishedWorkspaceTemplateDraft(context.Background(), test.id, test.authorityErr)
			if err != nil || !handled || !result.Deleted || result.TemplateID != test.id {
				t.Fatalf("draft delete result=%#v handled=%t err=%v", result, handled, err)
			}
			if _, present, err := sources.ReadTemplate(context.Background(), test.id); err != nil || present {
				t.Fatalf("draft source remained: present=%t err=%v", present, err)
			}
		})
	}
}

func TestTemplateDeleteDoesNotCollapseActiveRecoveryIntoDraftCleanup(t *testing.T) {
	sources, err := workspaceauthoritysource.New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	id := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789d3")
	body := realResourcesMigrationCollection(t, t.TempDir()).Templates[0].Current.Body
	source, err := tobari.NewWorkspaceTemplateDraftSource(id, "active-recovery", body)
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{sources: sources}
	_, handled, err := adapter.deleteUnpublishedWorkspaceTemplateDraft(context.Background(), id, tobari.ErrFinalAuthorityMutationRecoveryRequired)
	if !errors.Is(err, tobari.ErrFinalAuthorityMutationRecoveryRequired) || handled {
		t.Fatalf("active recovery error=%v handled=%t", err, handled)
	}
	if _, present, err := sources.ReadTemplate(context.Background(), id); err != nil || !present {
		t.Fatalf("active source was removed: present=%t err=%v", present, err)
	}
}

func TestContextDeleteRetiresUnpublishedDraftSource(t *testing.T) {
	sources, err := workspaceauthoritysource.New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	id := tobari.ContextID("01912345-6789-7abc-8def-0123456789d4")
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789d5")
	source, err := tobari.NewContextSource(tobari.ContextBinding{
		SchemaVersion: tobari.ContextBindingSchemaVersion,
		ID:            id,
		TemplateID:    templateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.PublishContext(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{sources: sources}
	result, handled, err := adapter.deleteUnpublishedContextDraft(context.Background(), id, tobari.ErrContextBindingNotFound)
	if err != nil || !handled || !result.Deleted || result.ContextID != id {
		t.Fatalf("draft delete result=%#v handled=%t err=%v", result, handled, err)
	}
	if _, present, err := sources.ReadContext(context.Background(), id); err != nil || present {
		t.Fatalf("draft source remained: present=%t err=%v", present, err)
	}
}

func TestContextDeleteDoesNotCollapseActiveRecoveryIntoDraftCleanup(t *testing.T) {
	sources, err := workspaceauthoritysource.New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	id := tobari.ContextID("01912345-6789-7abc-8def-0123456789d6")
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789d7")
	source, err := tobari.NewContextSource(tobari.ContextBinding{
		SchemaVersion: tobari.ContextBindingSchemaVersion,
		ID:            id,
		TemplateID:    templateID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := sources.PublishContext(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	adapter := &Adapter{sources: sources}
	_, handled, err := adapter.deleteUnpublishedContextDraft(context.Background(), id, tobari.ErrFinalAuthorityMutationRecoveryRequired)
	if !errors.Is(err, tobari.ErrFinalAuthorityMutationRecoveryRequired) || handled {
		t.Fatalf("active recovery error=%v handled=%t", err, handled)
	}
	if _, present, err := sources.ReadContext(context.Background(), id); err != nil || !present {
		t.Fatalf("active source was removed: present=%t err=%v", present, err)
	}
}

func TestPublicTemplateApplyCannotCrossBootstrapPublicationBarrier(t *testing.T) {
	if err := validateConfiguratorPublicationApplyAuthorization(false, false); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("public Apply crossed bootstrap publication barrier: %v", err)
	}
	if err := validateConfiguratorPublicationApplyAuthorization(true, false); err != nil {
		t.Fatalf("confirmed staged Configurator Apply was rejected: %v", err)
	}
	if err := validateConfiguratorPublicationApplyAuthorization(false, true); err != nil {
		t.Fatalf("exact initializer recovery was rejected: %v", err)
	}
}

func TestExactInitializerRejectsSourceChangedAfterReviewedDraft(t *testing.T) {
	templateRef := "wt1_01912345-6789-7abc-8def-0123456789ab"
	contextRef := "ctx1_01912345-6789-7abc-8def-0123456789ac"
	revision := tobari.SemanticDigest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	fingerprint := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	templatePlan := tobari.WorkspaceTemplateChangePlan{TemplateRef: templateRef, SourceRevision: revision, SourceFingerprint: fingerprint}
	if err := validateInitializerTemplatePlan(templatePlan, templateRef, revision, fingerprint); err != nil {
		t.Fatal(err)
	}
	templatePlan.SourceFingerprint = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	if !errors.Is(validateInitializerTemplatePlan(templatePlan, templateRef, revision, fingerprint), tobari.ErrWorkspaceTemplateChangePlanStale) {
		t.Fatal("changed Template source was accepted")
	}
	contextPlan := tobari.ContextActivationPlan{ContextRef: contextRef, TemplateRef: templateRef, SourceFingerprint: fingerprint}
	if err := validateInitializerContextPlan(contextPlan, contextRef, templateRef, fingerprint); err != nil {
		t.Fatal(err)
	}
	contextPlan.TemplateRef = "wtpl1_01912345-6789-7abc-8def-0123456789ff"
	if !errors.Is(validateInitializerContextPlan(contextPlan, contextRef, templateRef, fingerprint), tobari.ErrWorkspaceTemplateChangePlanStale) {
		t.Fatal("changed Context source was accepted")
	}
}

func TestConfiguratorAttachmentRequiresExactLiveContextAuthority(t *testing.T) {
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	binding := tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Ordinal: 1, Image: "tobari-runtime:test"}
	body := tobari.WorkspaceTemplateBody{
		Boundary:         tobari.WorkspaceTemplateBoundary{SourceAccess: tobari.ManifestSourceAccessReadWrite, DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}}, MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}}},
		Policy:           tobari.WorkspaceTemplatePolicyBody{AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled, BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{}},
		EntryDefaults:    tobari.WorkspaceTemplateEntryDefaults{Runtime: binding},
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}
	seed, err := tobari.NewPolicyAssistConfiguratorSeed(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfiguratorAttachmentAuthority(draft, snapshot); err != nil {
		t.Fatalf("exact authority rejected: %v", err)
	}
	stale := snapshot.Clone()
	stale.Context.TemplateID = tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ff")
	if !errors.Is(validateConfiguratorAttachmentAuthority(draft, stale), tobari.ErrWorkspaceTemplateChangePlanStale) {
		t.Fatal("replaced Context authority was accepted for attachment")
	}
	bootstrapSeed, err := tobari.NewBootstrapConfiguratorSeed("/workspace/example", body)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapDraft, err := tobari.NewConfiguratorDraft(bootstrapSeed, tobari.ConfiguratorAgentCodex, templateID, contextID)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapSubmission, err := tobari.NewConfiguratorSubmission(bootstrapDraft, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfiguratorPublicationAttachmentCollection(bootstrapSubmission, tobari.WorkspaceAuthorityCollection{}, false); err != nil {
		t.Fatalf("pre-authority bootstrap publication attachment rejected: %v", err)
	}
	publishedRevision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, bootstrapSubmission.Body)
	if err != nil {
		t.Fatal(err)
	}
	publishedTemplate := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: publishedRevision, Retained: []tobari.WorkspaceTemplateRevision{publishedRevision.Clone()}}
	publishedCollection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{publishedTemplate},
		[]tobari.WorkspaceAuthorityContextRecord{{Context: snapshot.Context, PolicyMemory: memory}},
		[]tobari.WorkspaceBinding{},
		[]tobari.PolicyCandidateAuthority{},
		&templateID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConfiguratorPublicationAttachmentCollection(bootstrapSubmission, publishedCollection, true); err != nil {
		t.Fatalf("authority-published bootstrap adoption recovery rejected: %v", err)
	}
}

func TestBootstrapConfiguratorAttachmentRejectsConcurrentCollectionCreation(t *testing.T) {
	if err := validateBootstrapConfiguratorAttachmentState(false); err != nil {
		t.Fatalf("still-empty bootstrap rejected: %v", err)
	}
	if !errors.Is(validateBootstrapConfiguratorAttachmentState(true), tobari.ErrWorkspaceTemplateChangePlanStale) {
		t.Fatal("bootstrap crossed concurrent collection creation")
	}
}
