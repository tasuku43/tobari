package workspaceauthoritycmd

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/app/execution"
	"github.com/tasuku43/tobari/internal/app/portcheck"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type FinalClusterStatusPort interface {
	Observe(context.Context) (tobari.FinalClusterStatus, error)
}

type FinalClusterDownPort interface {
	Down(context.Context) (tobari.WorkspaceAuthorityClusterDownPlan, error)
}

type FinalClusterLifecycleService struct {
	status  FinalClusterStatusPort
	down    FinalClusterDownPort
	mutator *execution.Invoker
}

func NewFinalClusterLifecycleService(port any) *FinalClusterLifecycleService {
	service := &FinalClusterLifecycleService{mutator: execution.New(finalClusterDownPolicy{})}
	service.status, _ = port.(FinalClusterStatusPort)
	service.down, _ = port.(FinalClusterDownPort)
	return service
}

func (s *FinalClusterLifecycleService) Status(ctx context.Context) (tobari.FinalClusterStatus, error) {
	if s == nil || portcheck.IsNil(s.status) {
		return tobari.FinalClusterStatus{}, missingPort("final cluster status")
	}
	result, err := s.status.Observe(ctx)
	if err != nil {
		return tobari.FinalClusterStatus{}, err
	}
	if err := result.Validate(); err != nil {
		return tobari.FinalClusterStatus{}, contractFault("invalid_cluster_status_result", "final cluster status result is invalid", err)
	}
	return result, nil
}

type FinalClusterDownResult struct {
	SchemaVersion      int                                      `json:"schema_version"`
	Task               string                                   `json:"task"`
	Stopped            bool                                     `json:"stopped"`
	Generation         uint64                                   `json:"generation"`
	CollectionRevision tobari.SemanticDigest                    `json:"collection_revision"`
	EnvelopeChanged    bool                                     `json:"envelope_changed"`
	Plan               tobari.WorkspaceAuthorityClusterDownPlan `json:"-"`
}

func newFinalClusterDownResult(plan tobari.WorkspaceAuthorityClusterDownPlan) (FinalClusterDownResult, error) {
	result := FinalClusterDownResult{SchemaVersion: tobari.FinalClusterLifecycleSchemaVersion, Task: tobari.TaskClusterDown, Stopped: true, Generation: plan.NextGeneration, CollectionRevision: plan.NextRevision, EnvelopeChanged: plan.EnvelopeChanged, Plan: plan}
	return result, result.Validate()
}

func (r FinalClusterDownResult) Validate() error {
	if r.SchemaVersion != tobari.FinalClusterLifecycleSchemaVersion || r.Task != tobari.TaskClusterDown || !r.Stopped || r.Plan.Validate() != nil || r.Generation != r.Plan.NextGeneration || r.CollectionRevision != r.Plan.NextRevision || r.EnvelopeChanged != r.Plan.EnvelopeChanged {
		return fmt.Errorf("final cluster down result is invalid")
	}
	return nil
}

type finalClusterDownPolicy struct{}

func (finalClusterDownPolicy) Check(_ context.Context, intent operation.Intent) error {
	if intent.Effect == operation.EffectWrite && intent.Target.Kind == tobari.ClusterTargetKind && intent.Target.ID == tobari.ClusterTargetID && intent.Target.ParentID == "" {
		return nil
	}
	return fault.New(fault.KindRejected, "mutation_rejected", "final cluster down target is not owned by Tobari", false)
}

func FinalClusterDownImpact() operation.Impact {
	return operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes}
}

func (s *FinalClusterLifecycleService) Down(ctx context.Context, intent operation.Intent) (FinalClusterDownResult, error) {
	if s == nil || portcheck.IsNil(s.down) {
		return FinalClusterDownResult{}, missingPort("final cluster down")
	}
	request := execution.Request{Intent: intent, ExpectedCommand: TaskClusterDown, ExpectedEffect: operation.EffectWrite, ExpectedTarget: operation.TargetRef{Kind: tobari.ClusterTargetKind, ID: tobari.ClusterTargetID}, ExpectedImpact: FinalClusterDownImpact()}
	var result FinalClusterDownResult
	err := s.mutator.Invoke(ctx, request, func(actionContext context.Context, _ operation.Intent) error {
		plan, err := s.down.Down(actionContext)
		if err != nil {
			if classified, ok := preReleaseLegacyMutationFault(err); ok {
				return classified
			}
			return err
		}
		confirmed, err := newFinalClusterDownResult(plan)
		if err != nil {
			return contractFault("invalid_cluster_down_result", "final cluster down result is invalid", err)
		}
		result = confirmed
		return nil
	})
	return result, err
}
