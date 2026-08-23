package tobari

import (
	"strings"
	"testing"
)

func validMigrationReport() MigrationReport {
	recoveryID := "sha256:" + strings.Repeat("b", 64)
	return MigrationReport{
		Task: TaskMigrationApply, Source: MigrationSourcePreV1ContextPolicyRuntime,
		Changed: true, RecoveryID: &recoveryID, ResearchAuthDisposition: ResearchAuthNotPresent,
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
		"task":        func(value *MigrationReport) { value.Task = "manifest.list" },
		"source":      func(value *MigrationReport) { value.Source = "unknown" },
		"nil items":   func(value *MigrationReport) { value.Contexts = nil },
		"duplicate":   func(value *MigrationReport) { value.Contexts = append(value.Contexts, value.Contexts[0]) },
		"no recovery": func(value *MigrationReport) { value.RecoveryID = nil },
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
	report.Changed, report.RecoveryID = false, nil
	report.Contexts[0].State = MigrationContextCurrent
	if err := report.Validate(); err != nil {
		t.Fatalf("current report: %v", err)
	}
}
