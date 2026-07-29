package systemdoctor

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestInspectReturnsValidOfflineRuntimeCheck(t *testing.T) {
	report, err := New().Inspect(context.Background())
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("report.Validate() error = %v", err)
	}
	if len(report.Checks) != 1 || report.Checks[0].Name != "runtime" {
		t.Fatalf("checks = %+v", report.Checks)
	}
	detail := report.Checks[0].Detail
	if !strings.Contains(detail, runtime.Version()) || !strings.Contains(detail, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Fatalf("runtime detail = %q", detail)
	}
}

func TestInspectHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Inspect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Inspect() error = %v, want context.Canceled", err)
	}
}
