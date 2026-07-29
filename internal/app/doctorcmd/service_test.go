package doctorcmd

import (
	"context"
	"errors"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/doctor"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/operation"
)

type fakeInspector struct {
	report doctor.Report
	err    error
	calls  int
	after  func()
}

func (f *fakeInspector) Inspect(context.Context) (doctor.Report, error) {
	f.calls++
	if f.after != nil {
		f.after()
	}
	return f.report, f.err
}

func doctorReadIntent() operation.Intent {
	return operation.Intent{Command: "doctor", Effect: operation.EffectRead}
}

func TestRunReturnsValidatedReport(t *testing.T) {
	inspector := &fakeInspector{report: doctor.Report{Checks: []doctor.Check{
		{Name: "runtime", Status: doctor.CheckStatusPass, Detail: "runtime-version test/test"},
	}}}

	report, err := New(inspector).Run(context.Background(), doctorReadIntent())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if inspector.calls != 1 {
		t.Fatalf("Inspect() calls = %d, want 1", inspector.calls)
	}
	if len(report.Checks) != 1 || report.Checks[0].Name != "runtime" {
		t.Fatalf("report = %+v", report)
	}
}

func TestRunRejectsInvalidIntentBeforeInspection(t *testing.T) {
	tests := []operation.Intent{
		{},
		{Command: "doctor", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: "system", ID: "local"}},
		{Command: "version", Effect: operation.EffectRead},
	}
	for _, intent := range tests {
		inspector := &fakeInspector{report: doctor.Report{Checks: []doctor.Check{{Name: "runtime", Status: doctor.CheckStatusPass}}}}
		if _, err := New(inspector).Run(context.Background(), intent); err == nil {
			t.Errorf("Run(%+v) succeeded", intent)
		}
		if inspector.calls != 0 {
			t.Errorf("Run(%+v) inspected %d times", intent, inspector.calls)
		}
	}
}

func TestRunFailsClosedForMissingOrInvalidDependencies(t *testing.T) {
	if _, err := New(nil).Run(context.Background(), doctorReadIntent()); err == nil {
		t.Fatal("Run() with nil inspector succeeded")
	}
	if _, err := New((*fakeInspector)(nil)).Run(context.Background(), doctorReadIntent()); err == nil {
		t.Fatal("Run() with typed nil inspector succeeded")
	}

	inspector := &fakeInspector{}
	if _, err := New(inspector).Run(context.Background(), doctorReadIntent()); err == nil {
		t.Fatal("Run() accepted an empty report")
	}
	if inspector.calls != 1 {
		t.Fatalf("Inspect() calls = %d, want 1", inspector.calls)
	}
}

func TestRunPropagatesInspectorAndContextErrors(t *testing.T) {
	want := errors.New("offline probe failed")
	inspector := &fakeInspector{err: want}
	if _, err := New(inspector).Run(context.Background(), doctorReadIntent()); !errors.Is(err, want) {
		t.Fatalf("Run() error = %v, want wrapped %v", err, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inspector = &fakeInspector{}
	if _, err := New(inspector).Run(ctx, doctorReadIntent()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if inspector.calls != 0 {
		t.Fatalf("canceled Run() inspected %d times", inspector.calls)
	}
}

func TestRunSuppressesSuccessWhenCanceledDuringInspection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	inspector := &fakeInspector{
		report: doctor.Report{Checks: []doctor.Check{{Name: "runtime", Status: doctor.CheckStatusPass}}},
		after:  cancel,
	}
	report, err := New(inspector).Run(ctx, doctorReadIntent())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v, want context.Canceled", err)
	}
	if report.Checks != nil {
		t.Fatalf("Run() report = %+v, want zero report", report)
	}
	if inspector.calls != 1 {
		t.Fatalf("Inspect() calls = %d, want 1", inspector.calls)
	}
}
