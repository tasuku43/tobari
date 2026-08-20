package migrationcmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type fakeMigration struct {
	calls  int
	report tobari.MigrationReport
	err    error
}

func (f *fakeMigration) MigrateInstallation(_ context.Context, _ io.Writer) (tobari.MigrationReport, error) {
	f.calls++
	return f.report, f.err
}

func migrationIntent() operation.Intent {
	return operation.Intent{
		Command: "migrate apply", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.MigrationTargetKind, ID: tobari.MigrationTargetID},
		Impact: operation.Impact{Cardinality: operation.CardinalityMany, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationYes},
	}
}

func applicationMigrationReport() tobari.MigrationReport {
	backup := "/tmp/tobari/migrations/pre-v1-0123456789ab"
	return tobari.MigrationReport{
		Task: tobari.TaskMigrationApply, Source: tobari.MigrationSourcePreV1ContextPolicyRuntime,
		Changed: true, Backup: &backup,
		Contexts: []tobari.MigrationContextResult{{ID: "018bcfe5-687b-7000-8000-000000000077", Name: "default", State: tobari.MigrationContextMigrated, Runtime: "standard", PolicyRevision: "sha256:" + strings.Repeat("a", 64)}},
	}
}

func TestApplyValidatesIntentBeforeMigration(t *testing.T) {
	fake := &fakeMigration{report: applicationMigrationReport()}
	service := New(fake)
	intent := migrationIntent()
	intent.Target.ID = "another-installation"
	if _, err := service.Apply(context.Background(), intent, io.Discard); err == nil {
		t.Fatal("invalid intent accepted")
	}
	if fake.calls != 0 {
		t.Fatalf("migration calls = %d", fake.calls)
	}
}

func TestApplyReturnsValidatedMigration(t *testing.T) {
	fake := &fakeMigration{report: applicationMigrationReport()}
	result, err := New(fake).Apply(context.Background(), migrationIntent(), io.Discard)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.Changed || fake.calls != 1 {
		t.Fatalf("result=%+v calls=%d", result, fake.calls)
	}
}

func TestApplyClassifiesSupportedMigrationFailures(t *testing.T) {
	tests := map[string]struct {
		err  error
		code string
	}{
		"unsupported": {tobari.ErrMigrationNotSupported, "migration_not_supported"},
		"unsafe":      {tobari.ErrMigrationSourceUnsafe, "migration_source_rejected"},
		"conflict":    {tobari.ErrMigrationRuntimeConflict, "migration_runtime_conflict"},
		"runtime":     {tobari.ErrMigrationRuntimeFailed, "migration_runtime_failed"},
		"backup":      {tobari.ErrMigrationBackupFailed, "migration_backup_failed"},
		"changed":     {tobari.ErrMigrationSourceChanged, "migration_source_changed"},
		"write":       {tobari.ErrMigrationWriteFailed, "migration_incomplete"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := New(&fakeMigration{err: test.err}).Apply(context.Background(), migrationIntent(), io.Discard)
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code || public.Retryable {
				t.Fatalf("fault = %#v, ok=%v", public, ok)
			}
		})
	}
}

func TestApplyCollapsesUnknownPostActionOutcome(t *testing.T) {
	_, err := New(&fakeMigration{err: errors.New("unknown")}).Apply(context.Background(), migrationIntent(), io.Discard)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "unclassified_mutation_outcome" || public.Retryable {
		t.Fatalf("fault = %#v, ok=%v", public, ok)
	}
}
