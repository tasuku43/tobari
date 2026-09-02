package workspaceauthoritycmd

import (
	"context"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextDeleteReadinessFixture struct {
	deleteCalls    int
	readinessCalls int
}

func (f *contextDeleteReadinessFixture) DeleteContextByReferenceWithReadiness(
	ctx context.Context,
	ref string,
	readiness func(context.Context) error,
) (tobari.ContextDeleteResult, error) {
	f.deleteCalls++
	if err := readiness(ctx); err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	return tobari.ContextDeleteResult{ContextID: id, Deleted: true}, nil
}

func (f *contextDeleteReadinessFixture) Check(context.Context) error {
	f.readinessCalls++
	return fault.WithClassification(
		fault.New(fault.KindUnavailable, "docker_context_unavailable", "synthetic selected Docker context is unavailable", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the selected Docker context."}),
		fault.PhasePrecondition,
		fault.ChangeNone,
	)
}

func TestContextDeletePreservesFreshReadinessFault(t *testing.T) {
	port := &contextDeleteReadinessFixture{}
	service := NewContextService(port, port)
	ref, err := tobari.ContextRef(contextID)
	if err != nil {
		t.Fatal(err)
	}
	deleteIntent := intent(
		TaskContextDelete,
		operation.EffectWrite,
		operation.TargetRef{Kind: tobari.ContextReferenceKind, ID: ref},
		ContextDeleteImpact(),
	)
	_, err = service.Delete(context.Background(), deleteIntent, ref)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "docker_context_unavailable" || public.Phase != fault.PhasePrecondition || public.ChangeState != fault.ChangeNone ||
		len(public.NextActions) != 1 || public.NextActions[0].Command != "doctor" {
		t.Fatalf("Context delete readiness fault = %#v, ok=%t, err=%v", public, ok, err)
	}
	if port.deleteCalls != 1 || port.readinessCalls != 1 {
		t.Fatalf("Context delete calls=%d readiness=%d", port.deleteCalls, port.readinessCalls)
	}
}
