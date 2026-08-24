package workspaceauthoritycmd

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type ContextReadPort interface {
	ListContextAuthority(context.Context) ([]ContextSnapshot, error)
	ReadContextAuthorityByReference(context.Context, string) (ContextSnapshot, error)
}

type ContextCreatePort interface {
	CreateContextByTemplateReference(context.Context, string, string) (ContextSnapshot, error)
}

type ContextEnterPort interface {
	EnterContextByReference(context.Context, string, tobari.WorkspaceSessionRequest, io.Reader, io.Writer, io.Writer) (tobari.ContextEntryPublication, error)
}

type ContextDeletePort interface {
	DeleteContextByReference(context.Context, string) (tobari.ContextDeleteResult, error)
}

type ContextEntryResult struct {
	ContextRef   string
	WorkspaceRef string
	Snapshot     ContextSnapshot
	Outcome      tobari.WorkspaceSessionOutcome
}

type contextMutationPolicy struct{}

func (contextMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	switch {
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.ContextReferenceKind && intent.Target.ID == "" && intent.Target.ParentID != "":
		_, err := tobari.ParseWorkspaceTemplateRef(intent.Target.ParentID)
		return err
	case intent.Effect == operation.EffectCreate && intent.Target.Kind == tobari.WorkspaceReferenceKind && intent.Target.ID == "" && intent.Target.ParentID != "":
		_, err := tobari.ParseContextRef(intent.Target.ParentID)
		return err
	case intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ContextReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "":
		_, err := tobari.ParseContextRef(intent.Target.ID)
		return err
	default:
		return fault.New(fault.KindRejected, "mutation_rejected", "Context mutation target is not owned by Tobari", false)
	}
}

type ContextService struct {
	read    ContextReadPort
	create  ContextCreatePort
	enter   ContextEnterPort
	delete  ContextDeletePort
	mutator *execution.Invoker
}

func NewContextService(port any) *ContextService {
	service := &ContextService{mutator: execution.New(contextMutationPolicy{})}
	service.read, _ = port.(ContextReadPort)
	service.create, _ = port.(ContextCreatePort)
	service.enter, _ = port.(ContextEnterPort)
	service.delete, _ = port.(ContextDeletePort)
	return service
}

func ContextCreateImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}
}

func ContextEnterImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func ContextDeleteImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *ContextService) List(ctx context.Context) (ContextList, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return ContextList{}, missingPort("Context read")
	}
	snapshots, err := s.read.ListContextAuthority(ctx)
	if err != nil {
		return ContextList{}, readFault(err, "context_read_failed", "Contexts could not be read")
	}
	result, err := NewContextList(snapshots)
	if err != nil {
		return ContextList{}, contractFault("invalid_context_list", "Context list is invalid", err)
	}
	return result, nil
}

func (s *ContextService) Show(ctx context.Context, contextRef string) (ContextView, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return ContextView{}, missingPort("Context read")
	}
	id, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextView{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	snapshot, err := s.read.ReadContextAuthorityByReference(ctx, contextRef)
	if errors.Is(err, tobari.ErrContextBindingNotFound) {
		return ContextView{}, notFoundFault("context_not_found", "Context does not exist", "context list")
	}
	if err != nil {
		return ContextView{}, readFault(err, "context_read_failed", "Context could not be read")
	}
	if snapshot.Context.ID != id {
		return ContextView{}, contractFault("invalid_context", "Context read returned another authority", fmt.Errorf("Context reference mismatch"))
	}
	view, err := NewContextView(snapshot)
	if err != nil || view.ContextRef != contextRef {
		if err == nil {
			err = fmt.Errorf("Context reference was not re-emitted unchanged")
		}
		return ContextView{}, contractFault("invalid_context", "Context is invalid", err)
	}
	return view, nil
}

func (s *ContextService) Create(ctx context.Context, intent operation.Intent, templateRef, projectRoot string) (ContextView, error) {
	if s == nil || portcheck.IsNil(s.create) {
		return ContextView{}, missingPort("Context create")
	}
	templateID, err := tobari.ParseWorkspaceTemplateRef(templateRef)
	if err != nil {
		return ContextView{}, invalidFault("invalid_template_ref", "Workspace Template reference is invalid", err, "template list")
	}
	if err := tobari.ValidateCanonicalRoot(projectRoot); err != nil {
		return ContextView{}, invalidFault("invalid_project_root", "Project root is invalid", err, "status")
	}
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ParentID: templateRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextCreate, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: ContextCreateImpact()}
	var result ContextView
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		snapshot, err := s.create.CreateContextByTemplateReference(actionContext, templateRef, projectRoot)
		if err != nil {
			return contextMutationFault(err)
		}
		view, err := NewContextView(snapshot)
		if err != nil {
			return contractFault("invalid_context_create_result", "created Context is invalid", err)
		}
		if snapshot.Context.TemplateID != templateID || snapshot.Context.ProjectRoot != projectRoot || len(snapshot.PolicyMemory.Rules) != 0 || snapshot.PolicyMemory.Generation != 1 || snapshot.Workspace != nil || snapshot.ActiveTemplatePolicy != nil || snapshot.ActivePolicyMemory != nil || snapshot.ActivePolicyMemoryRef != nil {
			return contractFault("invalid_context_create_result", "created Context contains authority outside the requested empty binding", fmt.Errorf("Context create result mismatch"))
		}
		result = view
		return nil
	})
	return result, err
}

func (s *ContextService) Enter(ctx context.Context, intent operation.Intent, contextRef string, session tobari.WorkspaceSessionRequest, in io.Reader, out, errOut io.Writer) (ContextEntryResult, error) {
	if s == nil || portcheck.IsNil(s.enter) {
		return ContextEntryResult{}, missingPort("Context entry")
	}
	contextID, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextEntryResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	if err := session.Validate(); err != nil {
		return ContextEntryResult{}, invalidFault("invalid_arguments", "Workspace session command is invalid", err, "help context enter")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ParentID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextEnter, ExpectedEffect: operation.EffectCreate, ExpectedTarget: target, ExpectedImpact: ContextEnterImpact()}
	var result ContextEntryResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		publication, err := s.enter.EnterContextByReference(actionContext, contextRef, session, in, out, errOut)
		if err != nil {
			return contextMutationFault(err)
		}
		if err := publication.Snapshot.Validate(); err != nil {
			return contractFault("invalid_context_entry_result", "Context entry authority is invalid", err)
		}
		if err := publication.Outcome.Validate(); err != nil {
			return contractFault("invalid_context_entry_result", "Workspace session outcome is invalid", err)
		}
		snapshot := publication.Snapshot
		if snapshot.Context.ID != contextID || snapshot.Workspace == nil || snapshot.Workspace.LastSuccessfulEntry == nil || snapshot.Workspace.LastSuccessfulEntry.TemplateRevision != snapshot.Template.Current.Revision || snapshot.ActiveTemplatePolicy == nil || snapshot.ActiveTemplatePolicy.PolicySliceDigest != snapshot.Template.Current.Slices.PolicySliceDigest || snapshot.ActivePolicyMemory == nil || snapshot.ActivePolicyMemoryRef == nil || snapshot.ActivePolicyMemory.Revision != snapshot.PolicyMemory.Revision {
			return contractFault("invalid_context_entry_result", "Context entry did not confirm current Template, Policy Memory, and Workspace authority", fmt.Errorf("entry publication mismatch"))
		}
		workspaceRef, err := tobari.WorkspaceRef(snapshot.Workspace.ID)
		if err != nil {
			return contractFault("invalid_context_entry_result", "Context entry Workspace reference is invalid", err)
		}
		result = ContextEntryResult{ContextRef: contextRef, WorkspaceRef: workspaceRef, Snapshot: snapshot.Clone(), Outcome: publication.Outcome}
		return nil
	})
	return result, err
}

func (s *ContextService) Delete(ctx context.Context, intent operation.Intent, contextRef string) (ContextDeleteResult, error) {
	if s == nil || portcheck.IsNil(s.delete) {
		return ContextDeleteResult{}, missingPort("Context delete")
	}
	id, err := tobari.ParseContextRef(contextRef)
	if err != nil {
		return ContextDeleteResult{}, invalidFault("invalid_context_ref", "Context reference is invalid", err, "context list")
	}
	target := operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: contextRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskContextDelete, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: ContextDeleteImpact()}
	var result ContextDeleteResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		deleted, err := s.delete.DeleteContextByReference(actionContext, contextRef)
		if err != nil {
			return contextMutationFault(err)
		}
		if !deleted.Deleted || deleted.ContextID != id {
			return contractFault("invalid_context_delete_result", "Context deletion is not confirmed", fmt.Errorf("delete result mismatch"))
		}
		result = deleted
		return nil
	})
	return result, err
}

func contextMutationFault(err error) error {
	switch {
	case errors.Is(err, tobari.ErrWorkspaceTemplateNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "template_not_found", "Workspace Template no longer exists", false, fault.NextAction{Command: "template list", Reason: "Discover current Template authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingExists):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_exists", "the Project and Workspace Template already have a Context", false, fault.NextAction{Command: "context list", Reason: "Use the existing Context reference."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "context_not_found", "Context no longer exists", false, fault.NextAction{Command: "context list", Reason: "Discover current Context authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrContextBindingProtected):
		return fault.WithClassification(fault.New(fault.KindRejected, "context_in_use", "Context still owns a Workspace, attachment, or research credential", false, fault.NextAction{Command: "context show", Reason: "Remove the exact blocking authority before Context deletion."}), fault.PhasePrecondition, fault.ChangeNone)
	default:
		return err
	}
}
