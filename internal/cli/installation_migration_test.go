package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/installationmigrationcmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type installationMigrationCLIFixture struct {
	plan       tobari.InstallationMigrationPlan
	result     tobari.InstallationMigrationResult
	applyCalls int
	appliedRef string
}

func (f *installationMigrationCLIFixture) PlanInstallationMigration(context.Context) (tobari.InstallationMigrationPlan, error) {
	return f.plan, nil
}

func (f *installationMigrationCLIFixture) ApplyInstallationMigration(_ context.Context, ref string) (tobari.InstallationMigrationResult, error) {
	f.applyCalls++
	f.appliedRef = ref
	return f.result, nil
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

func TestInstallationMigrationApplyRejectsNonOpaqueReferenceBeforePort(t *testing.T) {
	command, fixture, stdout, stderr := newInstallationMigrationCLI(t)
	if code := command.RunContext(context.Background(), []string{"installation", "migration", "apply", "--plan=authority.json", "--format=json"}); code != ExitUsage {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if fixture.applyCalls != 0 {
		t.Fatalf("apply calls=%d", fixture.applyCalls)
	}
}
