package tobari

import (
	"strings"
	"testing"
)

func validMigrationReport() MigrationReport {
	backup := "/tmp/tobari/migrations/pre-v1-0123456789ab"
	return MigrationReport{
		Task: TaskMigrationApply, Source: MigrationSourcePreV1ContextPolicyRuntime,
		Changed: true, Backup: &backup,
		Contexts: []MigrationContextResult{{
			ID: "018bcfe5-687b-7000-8000-000000000077", Name: "default",
			State: MigrationContextMigrated, Runtime: "standard",
			PolicyRevision: "sha256:" + strings.Repeat("a", 64),
		}},
	}
}

func TestMigrationReportRequiresConsistentTaskScopeAndBackup(t *testing.T) {
	report := validMigrationReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	tests := map[string]func(*MigrationReport){
		"task":        func(value *MigrationReport) { value.Task = "context.list" },
		"source":      func(value *MigrationReport) { value.Source = "unknown" },
		"nil items":   func(value *MigrationReport) { value.Contexts = nil },
		"duplicate":   func(value *MigrationReport) { value.Contexts = append(value.Contexts, value.Contexts[0]) },
		"no backup":   func(value *MigrationReport) { value.Backup = nil },
		"wrong state": func(value *MigrationReport) { value.Contexts[0].State = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := validMigrationReport()
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid migration report accepted")
			}
		})
	}
}

func TestMigrationReportAllowsCurrentNoOpWithoutBackup(t *testing.T) {
	report := validMigrationReport()
	report.Changed, report.Backup = false, nil
	report.Contexts[0].State = MigrationContextCurrent
	if err := report.Validate(); err != nil {
		t.Fatalf("current report: %v", err)
	}
}
