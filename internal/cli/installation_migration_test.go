package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/installationmigrationcmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type installationMigrationCLIFixture struct {
	plan       tobari.InstallationMigrationPlan
	result     tobari.InstallationMigrationResult
	applyCalls int
	appliedRef string
	applyErr   error
}

func (f *installationMigrationCLIFixture) PlanInstallationMigration(context.Context) (tobari.InstallationMigrationPlan, error) {
	return f.plan, nil
}

func (f *installationMigrationCLIFixture) ApplyInstallationMigration(_ context.Context, ref string) (tobari.InstallationMigrationResult, error) {
	f.applyCalls++
	f.appliedRef = ref
	return f.result, f.applyErr
}

func newInstallationMigrationCLI(t *testing.T) (*CLI, *installationMigrationCLIFixture, *bytes.Buffer, *bytes.Buffer) {
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
	result := tobari.InstallationMigrationResult{SchemaVersion: 1, PlanRef: plan.PlanRef, ActiveGeneration: collection.Generation, ActiveRevision: collection.Revision, Changed: true}
	fixture := &installationMigrationCLIFixture{plan: plan, result: result}
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	command := newCLI(strings.NewReader(""), stdout, stderr, DefaultCatalog(), nil)
	command.installationMigration = installationmigrationcmd.New(fixture)
	return command, fixture, stdout, stderr
}

func TestInstallationMigrationPlanAndApplyRoundTripOpaqueReference(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "plan", "--format=json"}); code != ExitOK {
		t.Fatalf("plan code=%d stderr=%q", code, stderr.String())
	}
	var planDocument map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &planDocument); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, planDocument, []string{"installation_migration_plan", "schema_version"})
	var planProjection map[string]json.RawMessage
	if err := json.Unmarshal(planDocument["installation_migration_plan"], &planProjection); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, planProjection, []string{
		"context_count", "plan_ref", "policy_memory_count", "runtime_source_digest", "source_digest",
		"source_generation", "source_revision", "target_generation", "template_count", "workspace_count",
	})
	var planned struct {
		SchemaVersion int                              `json:"schema_version"`
		Plan          tobari.InstallationMigrationPlan `json:"installation_migration_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &planned); err != nil || planned.SchemaVersion != 1 || planned.Plan.PlanRef != fixture.plan.PlanRef {
		t.Fatalf("plan=%+v err=%v output=%s", planned, err, stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "apply", "--plan=" + planned.Plan.PlanRef, "--format=json"}); code != ExitOK {
		t.Fatalf("apply code=%d stderr=%q", code, stderr.String())
	}
	var applyDocument map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &applyDocument); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, applyDocument, []string{"installation_migration", "schema_version"})
	var applyProjection map[string]json.RawMessage
	if err := json.Unmarshal(applyDocument["installation_migration"], &applyProjection); err != nil {
		t.Fatal(err)
	}
	assertExactJSONKeys(t, applyProjection, []string{"active_generation", "active_revision", "changed", "plan_ref"})
	var applied struct {
		SchemaVersion int                                `json:"schema_version"`
		Result        tobari.InstallationMigrationResult `json:"installation_migration"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &applied); err != nil || applied.SchemaVersion != 1 || applied.Result.PlanRef != planned.Plan.PlanRef {
		t.Fatalf("result=%+v err=%v output=%s", applied, err, stdout.String())
	}
	if fixture.applyCalls != 1 || fixture.appliedRef != planned.Plan.PlanRef {
		t.Fatalf("apply calls=%d ref=%q", fixture.applyCalls, fixture.appliedRef)
	}
}

func TestInstallationMigrationPlanHumanOutputUsesStructuredCard(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "plan"}); code != ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"· Installation migration planned\n", "\nAuthority\n", "\nDetails\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("output omitted %q: %q", want, text)
		}
	}
	for label, want := range map[string]string{
		"Source":         "generation 1 · " + string(fixture.plan.SourceRevision),
		"Target":         "generation 1",
		"Resources":      "0 Templates · 0 Contexts · 0 Policy memories · 0 Workspaces",
		"Next":           ProgramName + " installation migration apply --plan=" + fixture.plan.PlanRef + " — Apply this exact reviewed migration.",
		"Plan reference": fixture.plan.PlanRef,
		"Source digest":  string(fixture.plan.SourceDigest),
		"Runtime source": string(fixture.plan.RuntimeSourceDigest),
	} {
		if !humanOutputHasRow(text, label, want) {
			t.Errorf("output omitted %s=%q: %q", label, want, text)
		}
	}
	if strings.HasPrefix(text, "Installation migration plan ") {
		t.Fatalf("output retained the flat migration summary: %q", text)
	}
}

func TestInstallationMigrationApplyHumanOutputUsesStructuredCard(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "apply", "--plan=" + fixture.plan.PlanRef}); code != ExitOK {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	text := stdout.String()
	for _, want := range []string{"✓ Installation migration applied\n", "\nAuthority\n", "\nDetails\n"} {
		if !strings.Contains(text, want) {
			t.Errorf("output omitted %q: %q", want, text)
		}
	}
	for label, want := range map[string]string{
		"Status":         "✓ active · migration committed",
		"Generation":     "1",
		"Revision":       string(fixture.result.ActiveRevision),
		"Changed":        "yes",
		"Next":           ProgramName + " doctor — Verify the migrated installation and inspect its final authority.",
		"Plan reference": fixture.result.PlanRef,
	} {
		if !humanOutputHasRow(text, label, want) {
			t.Errorf("output omitted %s=%q: %q", label, want, text)
		}
	}
	if fixture.applyCalls != 1 || fixture.appliedRef != fixture.plan.PlanRef {
		t.Fatalf("apply calls=%d ref=%q", fixture.applyCalls, fixture.appliedRef)
	}
}

func TestInstallationMigrationApplyRejectsNonOpaqueReferenceBeforePort(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "apply", "--plan=authority.json", "--format=json"}); code != ExitUsage {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if fixture.applyCalls != 0 {
		t.Fatalf("apply calls=%d", fixture.applyCalls)
	}
}

func TestInstallationMigrationApplyDeclaresPreMutationFallback(t *testing.T) {
	spec, found := DefaultCatalog().Lookup("installation migration apply")
	if !found {
		t.Fatal("installation migration apply is absent")
	}
	declared := commandErrorByCode(t, spec.Agent.Errors, "installation_migration_failed")
	if declared.Kind != fault.KindUnavailable || declared.Phase != fault.PhasePrecondition || declared.ChangeState != fault.ChangeNone {
		t.Fatalf("installation migration fallback = %+v", declared)
	}
}

func TestInstallationMigrationApplyFallbackNeverCollapsesToUndeclaredContract(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	fixture.applyErr = errors.New("synthetic pre-mutation observation failure")
	if code := command.RunContext(context.Background(), []string{"--error-format=json", "installation", "migration", "apply", "--plan=" + fixture.plan.PlanRef}); code != ExitUnavailable {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !json.Valid(stderr.Bytes()) || !strings.Contains(stderr.String(), "installation_migration_failed") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
