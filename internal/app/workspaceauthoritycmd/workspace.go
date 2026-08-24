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

type WorkspaceReadPort interface {
	ListWorkspaceAuthority(context.Context) ([]ContextSnapshot, error)
	ReadWorkspaceAuthorityByReference(context.Context, string) (ContextSnapshot, error)
}

type WorkspaceDeletePort interface {
	DeleteWorkspaceByReference(context.Context, string, bool) (tobari.WorkspaceAuthorityDeleteResult, error)
}

type workspaceMutationPolicy struct{}

func (workspaceMutationPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.WorkspaceReferenceKind && intent.Target.ID != "" && intent.Target.ParentID == "" {
		_, err := tobari.ParseWorkspaceRef(intent.Target.ID)
		return err
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "Workspace mutation target is not owned by Tobari", false)
}

type WorkspaceService struct {
	read    WorkspaceReadPort
	delete  WorkspaceDeletePort
	mutator *execution.Invoker
}

func NewWorkspaceService(port any) *WorkspaceService {
	service := &WorkspaceService{mutator: execution.New(workspaceMutationPolicy{})}
	service.read, _ = port.(WorkspaceReadPort)
	service.delete, _ = port.(WorkspaceDeletePort)
	return service
}

func WorkspaceDeleteImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *WorkspaceService) List(ctx context.Context) (WorkspaceList, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return WorkspaceList{}, missingPort("Workspace read")
	}
	snapshots, err := s.read.ListWorkspaceAuthority(ctx)
	if err != nil {
		return WorkspaceList{}, readFault(err, "workspace_read_failed", "Workspaces could not be read")
	}
	result, err := NewWorkspaceList(snapshots)
	if err != nil {
		return WorkspaceList{}, contractFault("invalid_workspace_list", "Workspace list is invalid", err)
	}
	return result, nil
}

func (s *WorkspaceService) Status(ctx context.Context, workspaceRef string) (WorkspaceView, error) {
	if s == nil || portcheck.IsNil(s.read) {
		return WorkspaceView{}, missingPort("Workspace read")
	}
	id, err := tobari.ParseWorkspaceRef(workspaceRef)
	if err != nil {
		return WorkspaceView{}, invalidFault("invalid_workspace_ref", "Workspace reference is invalid", err, "workspace list")
	}
	snapshot, err := s.read.ReadWorkspaceAuthorityByReference(ctx, workspaceRef)
	if errors.Is(err, tobari.ErrWorkspaceBindingNotFound) {
		return WorkspaceView{}, notFoundFault("workspace_not_found", "Workspace does not exist", "workspace list")
	}
	if err != nil {
		return WorkspaceView{}, readFault(err, "workspace_read_failed", "Workspace could not be read")
	}
	if snapshot.Workspace == nil || snapshot.Workspace.ID != id {
		return WorkspaceView{}, contractFault("invalid_workspace", "Workspace read returned another authority", fmt.Errorf("Workspace reference mismatch"))
	}
	view, err := NewWorkspaceView(snapshot)
	if err != nil || view.WorkspaceRef != workspaceRef {
		if err == nil {
			err = fmt.Errorf("Workspace reference was not re-emitted unchanged")
		}
		return WorkspaceView{}, contractFault("invalid_workspace", "Workspace is invalid", err)
	}
	return view, nil
}

func (s *WorkspaceService) Delete(ctx context.Context, intent operation.Intent, workspaceRef string, force bool) (WorkspaceDeleteResult, error) {
	if s == nil || portcheck.IsNil(s.delete) {
		return WorkspaceDeleteResult{}, missingPort("Workspace delete")
	}
	id, err := tobari.ParseWorkspaceRef(workspaceRef)
	if err != nil {
		return WorkspaceDeleteResult{}, invalidFault("invalid_workspace_ref", "Workspace reference is invalid", err, "workspace list")
	}
	target := operation.TargetRef{Kind: tobari.WorkspaceReferenceKind, ID: workspaceRef}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskWorkspaceDelete, ExpectedEffect: operation.EffectWrite, ExpectedTarget: target, ExpectedImpact: WorkspaceDeleteImpact()}
	var result WorkspaceDeleteResult
	err = s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		deleted, err := s.delete.DeleteWorkspaceByReference(actionContext, workspaceRef, force)
		if err != nil {
			return workspaceMutationFault(err)
		}
		if !deleted.Deleted || deleted.WorkspaceID != id {
			return contractFault("invalid_workspace_delete_result", "Workspace deletion is not confirmed", fmt.Errorf("delete result mismatch"))
		}
		result = deleted
		return nil
	})
	return result, err
}

func workspaceMutationFault(err error) error {
	switch {
	case errors.Is(err, tobari.ErrWorkspaceBindingNotFound):
		return fault.WithClassification(fault.New(fault.KindNotFound, "workspace_not_found", "Workspace no longer exists", false, fault.NextAction{Command: "workspace list", Reason: "Discover current Workspace authority."}), fault.PhasePrecondition, fault.ChangeNone)
	case errors.Is(err, tobari.ErrWorkspaceBindingProtected):
		return fault.WithClassification(fault.New(fault.KindRejected, "workspace_attached", "Workspace still has a live attachment", false, fault.NextAction{Command: "workspace status", Reason: "Leave the exact Workspace or explicitly confirm forced cleanup."}), fault.PhasePrecondition, fault.ChangeNone)
	default:
		return err
	}
}
