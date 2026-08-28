package workspaceauthoritycmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type TemplateReadPort interface {
	ListWorkspaceTemplates(context.Context) ([]tobari.WorkspaceTemplate, error)
	DiscoverWorkspaceTemplate(context.Context, string) (tobari.WorkspaceTemplate, error)
}

type TemplateSourceObservationPort interface {
	ObserveWorkspaceTemplateSource(context.Context, tobari.WorkspaceTemplate) (tobari.ResourceSourceObservation, error)
}

type TemplateDraftReadPort interface {
	ListWorkspaceTemplateDrafts(context.Context) ([]tobari.WorkspaceTemplateDraft, error)
	DiscoverWorkspaceTemplateDraft(context.Context, string) (tobari.WorkspaceTemplateDraft, error)
}

type TemplateApplyPort interface {
	ApplyWorkspaceTemplateSourceByReference(context.Context, string) (tobari.WorkspaceTemplateRevisionPublication, error)
}

type TemplatePlanPort interface {
	PlanWorkspaceTemplateSourceByReference(context.Context, string) (tobari.WorkspaceTemplateChangePlan, error)
}

type TemplatePolicyMigrationPlanPort interface {
	PlanWorkspaceTemplatePolicyMigrationByReference(context.Context, string) (tobari.WorkspaceTemplatePolicyMigrationPlan, error)
}

type TemplatePolicyMigrationApplyPort interface {
	ApplyWorkspaceTemplatePolicyMigrationByReference(context.Context, string) (tobari.WorkspaceTemplatePolicyMigrationResult, error)
}

type TemplateDraftCreatePort interface {
	CreateWorkspaceTemplateDraft(context.Context, string, tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplateDraft, error)
}

type TemplateDraftCopyPort interface {
	CopyWorkspaceTemplateDraftByRevisionReference(context.Context, string, string) (tobari.WorkspaceTemplateDraft, error)
}

type TemplateDefaultPort interface {
	SetDefaultWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateSelectionResult, error)
}

type TemplateDeletePort interface {
	DeleteWorkspaceTemplateByReference(context.Context, string) (tobari.WorkspaceTemplateDeleteResult, error)
}

type templateMutationPolicy struct{}

func (templateMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.WorkspaceTemplateCatalogTargetKind && intent.Target.ParentID == tobari.WorkspaceTemplateCatalogTargetID && intent.Target.ID == "":
		return nil
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.WorkspaceTemplateReferenceKind && intent.Target.ID == "" && intent.Target.ParentID != "":
		_, _, err := tobari.ParseWorkspaceTemplateRevisionRef(intent.Target.ParentID)
		return err
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.WorkspaceTemplateReferenceKind && intent.Target.ID != "":
		if _, err := tobari.ParseWorkspaceTemplateRef(intent.Target.ID); err != nil {
			return err
		}
		if intent.Target.ParentID != "" {
			_, _, err := tobari.ParseRuntimeRevisionRef(intent.Target.ParentID)
			return err
		}
		return nil
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.WorkspaceTemplateChangePlanReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseWorkspaceTemplateChangePlanRef(intent.Target.ID)
		return err
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseWorkspaceTemplatePolicyMigrationPlanRef(intent.Target.ID)
		return err
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Workspace Template mutation target is not owned by Tobari", false)
	}
}

type TemplateService struct {
	read             TemplateReadPort
	draftCreate      TemplateDraftCreatePort
	draftCopy        TemplateDraftCopyPort
	defaultSet       TemplateDefaultPort
	delete           TemplateDeletePort
	apply            TemplateApplyPort
	planner          TemplatePlanPort
	migrationPlanner TemplatePolicyMigrationPlanPort
	migrationApply   TemplatePolicyMigrationApplyPort
	sources          TemplateSourceObservationPort
	drafts           TemplateDraftReadPort
	mutator          *execution.Invoker
}

func NewTemplateService(port any) *TemplateService {
	service := &TemplateService{mutator: execution.New(templateMutationPolicy{})}
	service.read, _ = port.(TemplateReadPort)
	service.draftCreate, _ = port.(TemplateDraftCreatePort)
	service.draftCopy, _ = port.(TemplateDraftCopyPort)
	service.defaultSet, _ = port.(TemplateDefaultPort)
	service.delete, _ = port.(TemplateDeletePort)
	service.apply, _ = port.(TemplateApplyPort)
	service.planner, _ = port.(TemplatePlanPort)
	service.migrationPlanner, _ = port.(TemplatePolicyMigrationPlanPort)
	service.migrationApply, _ = port.(TemplatePolicyMigrationApplyPort)
	service.sources, _ = port.(TemplateSourceObservationPort)
	service.drafts, _ = port.(TemplateDraftReadPort)
	return service
}

func TemplateApplyImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func TemplatePolicyMigrationImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}
}

func (s *TemplateService) PlanPolicyMigration(ctx context.Context, templateRef string) (tobari.WorkspaceTemplatePolicyMigrationPlan, error) {
	if s == nil || portcheck.IsNil(s.migrationPlanner) {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, missingPort("Workspace Template policy source migration planning")
	}
	if _, err := tobari.ParseWorkspaceTemplateRef(templateRef); err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	plan, err := s.migrationPlanner.PlanWorkspaceTemplatePolicyMigrationByReference(ctx, templateRef)
	if err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, templatePlanFault(err)
	}
	if err := plan.Validate(); err != nil || plan.TemplateRef != templateRef {
		return tobari.WorkspaceTemplatePolicyMigrationPlan{}, contractFault("invalid_template_policy_migration_plan", "Workspace Template policy migration plan is invalid", err)
	}
	return plan, nil
}

func (s *TemplateService) ApplyPolicyMigration(ctx context.Context, intent operation.Intent, planRef string) (tobari.WorkspaceTemplatePolicyMigrationResult, error) {
	if s == nil || portcheck.IsNil(s.migrationApply) {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, missingPort("Workspace Template policy source migration apply")
	}
	if _, err := tobari.ParseWorkspaceTemplatePolicyMigrationPlanRef(planRef); err != nil {
		return tobari.WorkspaceTemplatePolicyMigrationResult{}, invalidFault("invalid_template_policy_migration_plan_ref", "Workspace Template policy migration plan reference is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplatePolicyMigrationPlanReferenceKind, ID: planRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateMigrationApply, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: TemplatePolicyMigrationImpact()}
	var result tobari.WorkspaceTemplatePolicyMigrationResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		applied, err := s.migrationApply.ApplyWorkspaceTemplatePolicyMigrationByReference(actionContext, planRef)
		if err != nil {
			return templateMutationFault(err)
		}
		if err := applied.Validate(); err != nil {
			return contractFault("invalid_template_policy_migration_result", "Workspace Template policy migration result is invalid", err)
		}
		result = applied
		return nil
	})
	return result, err
}

func (s *TemplateService) Plan(ctx context.Context, templateRef string) (tobari.WorkspaceTemplateChangePlan, error) {
	if s == nil || portcheck.IsNil(s.planner) {
		return tobari.WorkspaceTemplateChangePlan{}, missingPort("Workspace Template change planning")
	}
	if _, err := tobari.ParseWorkspaceTemplateRef(templateRef); err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	plan, err := s.planner.PlanWorkspaceTemplateSourceByReference(ctx, templateRef)
	if err != nil {
		return tobari.WorkspaceTemplateChangePlan{}, templatePlanFault(err)
	}
	if err := plan.Validate(); err != nil || plan.TemplateRef != templateRef {
		if err == nil {
			err = fmt.Errorf("Template plan target does not match the request")
		}
		return tobari.WorkspaceTemplateChangePlan{}, contractFault("invalid_template_change_plan", "Workspace Template change plan is invalid", err)
	}
	return plan, nil
}

func TemplateCreateImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func TemplateDefaultImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func TemplateDeleteImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *TemplateService) List(ctx context.Context) (TemplateList, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return TemplateList{}, missingPort("Workspace Template read")
	}
	templates, err := s.read.ListWorkspaceTemplates(ctx)
	if err != nil {
		return TemplateList{}, readFault(err, "template_read_failed", "Workspace Templates could not be read")
	}
	result, err := NewTemplateList(templates)
	if err != nil {
		return TemplateList{}, contractFault("invalid_template_list", "Workspace Template list is invalid", err)
	}
	if !portcheck.IsNil(s.sources) {
		for index := range result.Items {
			observation, observeErr := s.sources.ObserveWorkspaceTemplateSource(ctx, result.Items[index].Template)
			if observeErr != nil {
				return TemplateList{}, readFault(observeErr, "template_source_read_failed", "Workspace Template source could not be inspected")
			}
			result.Items[index].Source = &observation
		}
	}
	if !portcheck.IsNil(s.drafts) {
		drafts, draftErr := s.drafts.ListWorkspaceTemplateDrafts(ctx)
		if draftErr != nil {
			return TemplateList{}, readFault(draftErr, "template_source_read_failed", "Workspace Template drafts could not be inspected")
		}
		result.Drafts = make([]TemplateDraftView, len(drafts))
		for index, draft := range drafts {
			view, viewErr := NewTemplateDraftView(draft)
			if viewErr != nil {
				return TemplateList{}, contractFault("invalid_template_list", "Workspace Template draft list is invalid", viewErr)
			}
			result.Drafts[index] = view
		}
	}
	return result, nil
}

func (s *TemplateService) ShowResource(ctx context.Context, name string) (TemplateResourceView, error) {
	active, err := s.Show(ctx, name)
	if err == nil {
		return TemplateResourceView{Active: &active}, nil
	}
	if portcheck.IsNil(s.drafts) {
		return TemplateResourceView{}, err
	}
	draft, draftErr := s.drafts.DiscoverWorkspaceTemplateDraft(ctx, name)
	if draftErr != nil {
		return TemplateResourceView{}, err
	}
	view, viewErr := NewTemplateDraftView(draft)
	if viewErr != nil {
		return TemplateResourceView{}, contractFault("invalid_template", "Workspace Template draft is invalid", viewErr)
	}
	return TemplateResourceView{Draft: &view}, nil
}

func (s *TemplateService) Show(ctx context.Context, name string) (TemplateView, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return TemplateView{}, missingPort("Workspace Template read")
	}
	if name != "" {
		if err := tobari.ValidateName(name); err != nil {
			return TemplateView{}, invalidFault("invalid_template_name", "Workspace Template name is invalid", err, "template list")
		}
	}
	template, err := s.read.DiscoverWorkspaceTemplate(ctx, name)
	if errors.Is(err, tobari.ErrWorkspaceTemplateNotFound) {
		return TemplateView{}, notFoundFault("template_not_found", "Workspace Template does not exist", "template list")
	}
	if err != nil {
		return TemplateView{}, readFault(err, "template_read_failed", "Workspace Template could not be read")
	}
	result, err := NewTemplateView(template)
	if err != nil {
		return TemplateView{}, contractFault("invalid_template", "Workspace Template is invalid", err)
	}
	if !portcheck.IsNil(s.sources) {
		observation, observeErr := s.sources.ObserveWorkspaceTemplateSource(ctx, template)
		if observeErr != nil {
			return TemplateView{}, readFault(observeErr, "template_source_read_failed", "Workspace Template source could not be inspected")
		}
		result.Source = &observation
	}
	return result, nil
}

type TemplateApplyResult struct {
	View    TemplateView
	Changed bool
}

func (s *TemplateService) Apply(ctx context.Context, intent operation.Intent, planRef string) (TemplateApplyResult, error) {
	if s == nil || portcheck.IsNil(s.apply) {
		return TemplateApplyResult{}, missingPort("Workspace Template source apply")
	}
	if _, err := tobari.ParseWorkspaceTemplateChangePlanRef(planRef); err != nil {
		return TemplateApplyResult{}, invalidFault("invalid_template_change_plan_ref", "Workspace Template change plan reference is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateChangePlanReferenceKind, ID: planRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateApply, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: TemplateApplyImpact()}
	var result TemplateApplyResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		publication, err := s.apply.ApplyWorkspaceTemplateSourceByReference(actionContext, planRef)
		if err != nil {
			return templateMutationFault(err)
		}
		if err := publication.Template.Validate(); err != nil || publication.Template.Current.Revision != publication.Current.Revision {
			if err == nil {
				err = fmt.Errorf("published Template revision does not match the Apply result")
			}
			return contractFault("invalid_template_apply_result", "Workspace Template source publication is invalid", err)
		}
		view, err := NewTemplateView(publication.Template)
		if err != nil {
			return contractFault("invalid_template_apply_result", "Workspace Template source publication is invalid", err)
		}
		if !portcheck.IsNil(s.sources) {
			observation, observeErr := s.sources.ObserveWorkspaceTemplateSource(actionContext, publication.Template)
			if observeErr != nil || observation.State != tobari.ResourceSourceInSync {
				return contractFault("invalid_template_apply_result", "Applied Workspace Template source is not current", observeErr)
			}
			view.Source = &observation
		}
		result = TemplateApplyResult{View: view, Changed: publication.Changed}
		return nil
	})
	return result, err
}

func (s *TemplateService) CreateDraft(ctx context.Context, intent operation.Intent, name string, body tobari.WorkspaceTemplateBody) (TemplateDraftView, error) {
	if s == nil || portcheck.IsNil(s.draftCreate) {
		return TemplateDraftView{}, missingPort("Workspace Template draft create")
	}
	if err := tobari.ValidateName(name); err != nil {
		return TemplateDraftView{}, invalidFault("invalid_template_name", "Workspace Template name is invalid", err, "template list")
	}
	if err := body.Validate(); err != nil {
		return TemplateDraftView{}, invalidFault("invalid_template_body", "Workspace Template definition is invalid", err, "help template create")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateCatalogTargetKind, ParentID: tobari.WorkspaceTemplateCatalogTargetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateCreate, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: TemplateCreateImpact()}
	var result TemplateDraftView
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		draft, err := s.draftCreate.CreateWorkspaceTemplateDraft(actionContext, name, body.Clone())
		if err != nil {
			return templateMutationFault(err)
		}
		view, err := NewTemplateDraftView(draft)
		if err != nil {
			return contractFault("invalid_template_create_result", "created Workspace Template draft is invalid", err)
		}
		result = view
		return nil
	})
	return result, err
}

func (s *TemplateService) CopyDraft(ctx context.Context, intent operation.Intent, revisionRef, name string) (TemplateDraftView, error) {
	if s == nil || portcheck.IsNil(s.draftCopy) {
		return TemplateDraftView{}, missingPort("Workspace Template draft copy")
	}
	if _, _, err := tobari.ParseWorkspaceTemplateRevisionRef(revisionRef); err != nil {
		return TemplateDraftView{}, invalidFault("invalid_template_revision_ref", "Workspace Template revision reference is invalid", err, "template show")
	}
	if err := tobari.ValidateName(name); err != nil {
		return TemplateDraftView{}, invalidFault("invalid_template_name", "Workspace Template name is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: revisionRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateCopy, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: TemplateCreateImpact()}
	var result TemplateDraftView
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		draft, err := s.draftCopy.CopyWorkspaceTemplateDraftByRevisionReference(actionContext, revisionRef, name)
		if err != nil {
			return templateMutationFault(err)
		}
		view, err := NewTemplateDraftView(draft)
		if err != nil {
			return contractFault("invalid_template_copy_result", "copied Workspace Template draft is invalid", err)
		}
		if draft.Name != name || draft.Source.State != tobari.ResourceSourceModified || draft.Source.ActiveRevision != nil {
			return contractFault("invalid_template_copy_result", "copied Workspace Template draft is not unpublished desired source", fmt.Errorf("Template draft copy result mismatch"))
		}
		result = view
		return nil
	})
	return result, err
}

func (s *TemplateService) SetDefault(ctx context.Context, intent operation.Intent, templateRef string) (TemplateSelectionResult, error) {
	if s == nil || portcheck.IsNil(s.defaultSet) {
		return TemplateSelectionResult{}, missingPort("Workspace Template default selection")
	}
	id, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return TemplateSelectionResult{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateDefaultSet, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: TemplateDefaultImpact()}
	var result TemplateSelectionResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		selected, err := s.defaultSet.SetDefaultWorkspaceTemplateByReference(actionContext, templateRef)
		if err != nil {
			return templateMutationFault(err)
		}
		if !selected.Selected || selected.TemplateID != id {
			return contractFault("invalid_template_default_result", "Workspace Template default selection is invalid", fmt.Errorf("selection does not match exact reference"))
		}
		result = selected
		return nil
	})
	return result, err
}

func (s *TemplateService) Delete(ctx context.Context, intent operation.Intent, templateRef string) (TemplateDeleteResult, error) {
	if s == nil || portcheck.IsNil(s.delete) {
		return TemplateDeleteResult{}, missingPort("Workspace Template delete")
	}
	id, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return TemplateDeleteResult{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ID: templateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateDelete, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: TemplateDeleteImpact()}
	var result TemplateDeleteResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		deleted, err := s.delete.DeleteWorkspaceTemplateByReference(actionContext, templateRef)
		if err != nil {
			return templateMutationFault(err)
		}
		if !deleted.Deleted || deleted.TemplateID != id {
			return contractFault("invalid_template_delete_result", "Workspace Template deletion is not confirmed", fmt.Errorf("delete result mismatch"))
		}
		result = deleted
		return nil
	})
	return result, err
}

func templateMutationFault(err error) error {
	return templateFault(err, func(cause error) error {
		return unclassifiedMutationFault("Workspace Template mutation returned an unclassified outcome", cause)
	})
}

func templatePlanFault(err error) error {
	return templateFault(err, func(cause error) error {
		return readFault(cause, "template_plan_read_failed", "Workspace Template change plan could not be read")
	})
}

func templateFault(err error, fallback func(error) error) error {
	if classified, ok := preReleaseLegacyMutationFault(err); ok {
		return classified
	}
	if classified, ok := finalAuthorityMutationRecoveryFault(err); ok {
		return classified
	}
	if _, ok := fault.PublicCopy(err); ok {
		return err
	}
	switch {
	case errors.Is(err, tobari.ErrResourceSourceRecoveryRequired):
		return fault.WithClassification(fault.New(fault.KindUnavailable, "resource_source_recovery_required", "The active authority changed but its file-backed source publication requires same-target recovery", false, fault.NextAction{Command: "template show", Reason: "Inspect the active and source identities before recovering the exact mutation."}), fault.PhaseMutation, fault.ChangePartial)
	case errors.Is(err, tobari.ErrResourceSourceMissing):
		return fault.WithClassification(fault.New(fault.KindNotFound, "resource_source_missing", "The exact resource source file is missing", false, fault.NextAction{Command: "template show", Reason: "Inspect the canonical source path and recreate this pre-release resource."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrResourceSourceInvalid):
		return fault.WithClassification(fault.New(fault.KindInvalidInput, "resource_source_invalid", "The exact resource source does not satisfy its current strict schema or closed file contract", false, fault.NextAction{Command: "template show", Reason: "Inspect the canonical source and correct the typed validation diagnostic."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrResourceSourceChanged):
		return fault.WithClassification(fault.New(fault.KindRejected, "resource_source_changed", "The Template source changed during Apply", true, fault.NextAction{Command: "template show", Reason: "Re-read the exact source and active identities before retrying Apply."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrResourceSourceModified):
		return fault.WithClassification(fault.New(fault.KindRejected, "resource_source_modified", "The Template source has unapplied changes", false, fault.NextAction{Command: "template show", Reason: "Inspect and explicitly apply the exact Template source first."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateChangePlanStale):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_change_plan_stale", "The reviewed Template change plan no longer matches source or active authority", false, fault.NextAction{Command: "template list", Reason: "Discover the Template again, then create and review a fresh exact change plan."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplatePolicyMigrationStale):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_policy_migration_plan_stale", "The reviewed Template policy migration no longer matches source or active authority", false, fault.NextAction{Command: "template list", Reason: "Rediscover the active Template before creating a fresh exact non-activating source migration plan."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateExists):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_exists", "Workspace Template already exists", false, fault.NextAction{Command: "template list", Reason: "Choose another name or inspect the existing Template."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateNotFound), errors.Is(err, tobari.ErrWorkspaceTemplateRevisionNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "template_not_found", "Workspace Template authority no longer exists", false, fault.NextAction{Command: "template list", Reason: "Discover current Template references."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateProtected):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_in_use", "Workspace Template is still selected or bound to a Context", false, fault.NextAction{Command: "context list", Reason: "Remove dependent Contexts and the default selection first."}), fault.PhasePrecondition, fault.ChangeNone)
	default:
		return fallback(err)
	}
}
