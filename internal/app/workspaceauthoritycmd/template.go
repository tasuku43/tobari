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

type TemplateCreatePort interface {
	CreateWorkspaceTemplate(context.Context, string, tobari.WorkspaceTemplateBody) (tobari.WorkspaceTemplate, error)
}

type TemplateCopyPort interface {
	CopyWorkspaceTemplateByRevisionReference(context.Context, string, string) (tobari.WorkspaceTemplateCopyPublication, error)
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
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.WorkspaceTemplateReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseWorkspaceTemplateRef(intent.Target.ID)
		return err
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Workspace Template mutation target is not owned by Tobari", false)
	}
}

type TemplateService struct {
	read       TemplateReadPort
	create     TemplateCreatePort
	copy       TemplateCopyPort
	defaultSet TemplateDefaultPort
	delete     TemplateDeletePort
	mutator    *execution.Invoker
}

func NewTemplateService(port any) *TemplateService {
	service := &TemplateService{mutator: execution.New(templateMutationPolicy{})}
	service.read, _ = port.(TemplateReadPort)
	service.create, _ = port.(TemplateCreatePort)
	service.copy, _ = port.(TemplateCopyPort)
	service.defaultSet, _ = port.(TemplateDefaultPort)
	service.delete, _ = port.(TemplateDeletePort)
	return service
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
	return result, nil
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
	return result, nil
}

func (s *TemplateService) Create(ctx context.Context, intent operation.Intent, name string, body tobari.WorkspaceTemplateBody) (TemplateView, error) {
	if s == nil || portcheck.IsNil(s.create) {
		return TemplateView{}, missingPort("Workspace Template create")
	}
	if err := tobari.ValidateName(name); err != nil {
		return TemplateView{}, invalidFault("invalid_template_name", "Workspace Template name is invalid", err, "template list")
	}
	if err := body.Validate(); err != nil {
		return TemplateView{}, invalidFault("invalid_template_body", "Workspace Template definition is invalid", err, "template create")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateCatalogTargetKind, ParentID: tobari.WorkspaceTemplateCatalogTargetID}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateCreate, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: TemplateCreateImpact()}
	var result TemplateView
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		created, err := s.create.CreateWorkspaceTemplate(actionContext, name, body.Clone())
		if err != nil {
			return templateMutationFault(err)
		}
		view, err := NewTemplateView(created)
		if err != nil {
			return contractFault("invalid_template_create_result", "created Workspace Template is invalid", err)
		}
		want, err := tobari.NewWorkspaceTemplateRevision(created.ID, 1, body)
		if err != nil || created.Name != name || created.Current.Generation != 1 || len(created.Retained) != 1 || created.Current.Revision != want.Revision {
			return contractFault("invalid_template_create_result", "created Workspace Template does not match the reviewed definition", fmt.Errorf("Template create result mismatch"))
		}
		result = view
		return nil
	})
	return result, err
}

func (s *TemplateService) Copy(ctx context.Context, intent operation.Intent, revisionRef, name string) (TemplateView, error) {
	if s == nil || portcheck.IsNil(s.copy) {
		return TemplateView{}, missingPort("Workspace Template copy")
	}
	sourceID, sourceDigest, err := tobari.ParseWorkspaceTemplateRevisionRef(revisionRef)
	if err != nil {
		return TemplateView{}, invalidFault("invalid_template_revision_ref", "Workspace Template revision reference is invalid", err, "template show")
	}
	if err := tobari.ValidateName(name); err != nil {
		return TemplateView{}, invalidFault("invalid_template_name", "Workspace Template name is invalid", err, "template list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceTemplateReferenceKind, ParentID: revisionRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskTemplateCopy, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: TemplateCreateImpact()}
	var result TemplateView
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		publication, err := s.copy.CopyWorkspaceTemplateByRevisionReference(actionContext, revisionRef, name)
		if err != nil {
			return templateMutationFault(err)
		}
		if err := publication.Source.Validate(); err != nil || publication.Source.TemplateID != sourceID || publication.Source.Revision != sourceDigest {
			return contractFault("invalid_template_copy_result", "Template copy source receipt is invalid", fmt.Errorf("copy source does not match the exact reference"))
		}
		created := publication.Created
		view, err := NewTemplateView(created)
		if err != nil {
			return contractFault("invalid_template_copy_result", "copied Workspace Template is invalid", err)
		}
		if created.ID == sourceID || created.Name != name || created.Current.Generation != 1 || len(created.Retained) != 1 || created.Current.Revision != publication.Source.Revision {
			return contractFault("invalid_template_copy_result", "copied Workspace Template is not a fresh independent exact fork", fmt.Errorf("copy result mismatch"))
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
	switch {
	case errors.Is(err, tobari.ErrWorkspaceTemplateExists):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_exists", "Workspace Template already exists", false, fault.NextAction{Command: "template list", Reason: "Choose another name or inspect the existing Template."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateNotFound), errors.Is(err, tobari.ErrWorkspaceTemplateRevisionNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "template_not_found", "Workspace Template authority no longer exists", false, fault.NextAction{Command: "template list", Reason: "Discover current Template references."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceTemplateProtected):
		return fault.WithClassification(fault.New(fault.KindRejected, "template_in_use", "Workspace Template is still selected or bound to a Context", false, fault.NextAction{Command: "context list", Reason: "Remove dependent Contexts and the default selection first."}), fault.PhasePrecondition, fault.ChangeNone)
	default:
		return err
	}
}
