package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/migrationcmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type migrationCLI struct {
	calls  int
	report tobari.MigrationReport
}

func (f *migrationCLI) MigrateInstallation(_ context.Context, _ io.Writer) (tobari.MigrationReport, error) {
	f.calls++
	return f.report, nil
}

func TestMigrationApplyRendersCompleteJSONAndMutatesOnce(t *testing.T) {
	backup := "/tmp/tobari/migrations/pre-v1-0123456789ab"
	fake := &migrationCLI{report: tobari.MigrationReport{
		Task: tobari.TaskMigrationApply, Source: tobari.MigrationSourcePreV1ContextPolicyRuntime,
		Changed: true, Backup: &backup,
		Contexts: []tobari.MigrationContextResult{{
			ID: "018bcfe5-687b-7000-8000-000000000077", Name: "default",
			State: tobari.MigrationContextMigrated, Runtime: "standard",
			PolicyRevision: "sha256:" + strings.Repeat("a", 64),
		}},
	}}
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.migrate = migrationcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"migrate", "apply", "--format", "json"}); code != ExitOK {
		t.Fatalf("migrate apply code = %d, stderr = %q", code, stderr.String())
	}
	var document migrationDocument
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("migration JSON = %q: %v", stdout.String(), err)
	}
	if document.SchemaVersion != 1 || !document.Migration.Changed || len(document.Migration.Contexts) != 1 || fake.calls != 1 {
		t.Fatalf("migration document/calls = %+v/%d", document, fake.calls)
	}
}

func TestMigrationNamespaceAndInvalidInputDoNotMutate(t *testing.T) {
	fake := &migrationCLI{}
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	command.migrate = migrationcmd.New(fake)
	if code := command.RunContext(context.Background(), []string{"migrate"}); code != ExitOK || !strings.Contains(stdout.String(), "  apply  Migrate") {
		t.Fatalf("migrate namespace code/output = %d/%q, stderr = %q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := command.RunContext(context.Background(), []string{"migrate", "apply", "--format", "yaml"}); code != ExitUsage {
		t.Fatalf("invalid format code = %d, stderr = %q", code, stderr.String())
	}
	if fake.calls != 0 {
		t.Fatalf("migration calls = %d", fake.calls)
	}
}
