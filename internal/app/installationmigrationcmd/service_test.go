package installationmigrationcmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type migrationPortFixture struct {
	plan       tobari.InstallationMigrationPlan
	result     tobari.InstallationMigrationResult
	planErr    error
	applyErr   error
	applyCalls int
	appliedRef string
}

func (f *migrationPortFixture) PlanInstallationMigration(context.Context) (tobari.InstallationMigrationPlan, error) {
	return f.plan, f.planErr
}

func (f *migrationPortFixture) ApplyInstallationMigration(_ context.Context, ref string) (tobari.InstallationMigrationResult, error) {
	f.applyCalls++
	f.appliedRef = ref
	return f.result, f.applyErr
}

func installationMigrationFixture(t *testing.T) (tobari.InstallationMigrationPlan, tobari.InstallationMigrationResult) {
	t.Helper()
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{}, []tobari.WorkspaceAuthorityContextRecord{},
		[]tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.NewInstallationMigrationPlan(tobari.SemanticDigest("sha256:"+strings.Repeat("a", 64)), tobari.SemanticDigest("sha256:"+strings.Repeat("b", 64)), collection)
	if err != nil {
		t.Fatal(err)
	}
	result := tobari.InstallationMigrationResult{
		SchemaVersion: 1, PlanRef: plan.PlanRef, ActiveGeneration: collection.Generation,
		ActiveRevision: collection.Revision, Changed: true,
	}
	return plan, result
}

func installationMigrationIntent(planRef string) operation.Intent {
	return operation.Intent{
		Command: TaskApply, Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.InstallationMigrationPlanReferenceKind, ID: planRef},
		Impact: Impact(),
	}
}

func TestPlanAndApplyPreserveOneExactMigrationReference(t *testing.T) {
	plan, result := installationMigrationFixture(t)
	port := &migrationPortFixture{plan: plan, result: result}
	service := New(port)
	observed, err := service.Plan(context.Background())
	if err != nil || observed.PlanRef != plan.PlanRef {
		t.Fatalf("plan=%+v err=%v", observed, err)
	}
	applied, err := service.Apply(context.Background(), installationMigrationIntent(plan.PlanRef), plan.PlanRef)
	if err != nil || applied.PlanRef != plan.PlanRef || port.applyCalls != 1 || port.appliedRef != plan.PlanRef {
		t.Fatalf("result=%+v calls=%d ref=%q err=%v", applied, port.applyCalls, port.appliedRef, err)
	}
}

func TestApplyRejectsIntentOrReferenceBeforePort(t *testing.T) {
	plan, result := installationMigrationFixture(t)
	port := &migrationPortFixture{plan: plan, result: result}
	service := New(port)
	intent := installationMigrationIntent(plan.PlanRef)
	intent.Target.ID = "implan1_" + strings.Repeat("b", 64)
	if _, err := service.Apply(context.Background(), intent, plan.PlanRef); err == nil {
		t.Fatal("mismatched intent was accepted")
	}
	if _, err := service.Apply(context.Background(), installationMigrationIntent("bad"), "bad"); err == nil {
		t.Fatal("invalid reference was accepted")
	}
	if port.applyCalls != 0 {
		t.Fatalf("apply calls=%d", port.applyCalls)
	}
}

func TestMigrationDomainFailuresHaveStablePublicCodes(t *testing.T) {
	plan, result := installationMigrationFixture(t)
	tests := []struct {
		err  error
		code string
	}{
		{tobari.ErrMigrationNotSupported, "installation_migration_not_supported"},
		{tobari.ErrMigrationSourceUnsafe, "installation_migration_source_rejected"},
		{tobari.ErrMigrationSourceChanged, "installation_migration_plan_stale"},
		{tobari.ErrMigrationWriteFailed, "installation_migration_incomplete"},
		{errors.New("unknown"), "installation_migration_failed"},
	}
	for _, test := range tests {
		port := &migrationPortFixture{plan: plan, result: result, applyErr: test.err}
		_, err := New(port).Apply(context.Background(), installationMigrationIntent(plan.PlanRef), plan.PlanRef)
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != test.code || public.Retryable {
			t.Errorf("input=%v fault=%+v ok=%t", test.err, public, ok)
		}
	}
}

func TestMigrationInvalidResultsRetainVerificationState(t *testing.T) {
	plan, result := installationMigrationFixture(t)
	invalidPlan := plan
	invalidPlan.PlanRef = ""
	_, err := New(&migrationPortFixture{plan: invalidPlan}).Plan(context.Background())
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_installation_migration_plan" || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeUnknown {
		t.Fatalf("invalid plan fault=%+v/%v", public, err)
	}
	result.PlanRef = ""
	_, err = New(&migrationPortFixture{result: result}).Apply(context.Background(), installationMigrationIntent(plan.PlanRef), plan.PlanRef)
	public, ok = fault.PublicCopy(err)
	if !ok || public.Code != "invalid_installation_migration_result" || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeUnknown {
		t.Fatalf("invalid result fault=%+v/%v", public, err)
	}
}
