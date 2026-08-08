package cli

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestClusterUpProgressIsSuppressedForMachineReadableErrors(t *testing.T) {
	t.Parallel()
	ctx := withErrorFormat(context.Background(), errorFormatJSON)
	if invocationErrorFormat(ctx) != errorFormatJSON {
		t.Fatal("test context did not select JSON error format")
	}
	if clusterUpProgressAllowed(ctx) {
		t.Fatal("progress remained enabled for JSON error output")
	}
	if !clusterUpProgressAllowed(context.Background()) {
		t.Fatal("progress was disabled for normal human output")
	}
}

func TestClusterUpProgressRendersActiveUpdatesAndCompletion(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := newClusterUpProgress(&output, true)
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressUpdated,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressCompleted,
	})
	progress.Close()

	got := output.String()
	for _, expected := range []string{
		applyStyleToken(true, styleAccent, "⠋"),
		applyStyleToken(true, styleAccent, "⠙"),
		applyStyleToken(true, styleSuccess, "✓"),
		"verify readiness",
	} {
		if !strings.Contains(got, expected) {
			t.Fatalf("progress output %q lacks %q", got, expected)
		}
	}
}

func TestClusterUpProgressRendersFailureWithoutRuntimeDiagnostics(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := newClusterUpProgress(&output, true)
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressStartServices, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressStartServices, Status: tobari.ClusterUpProgressFailed,
	})
	progress.Close()

	got := output.String()
	if !strings.Contains(got, applyStyleToken(true, styleDanger, "✗")+" start services") {
		t.Fatalf("failure output = %q", got)
	}
	if strings.Contains(got, "docker") || strings.Contains(got, "secret") {
		t.Fatalf("progress output leaked diagnostics: %q", got)
	}
	if progress.current != "" || !progress.failed {
		t.Fatalf("progress state = %+v", progress)
	}
}

func TestClusterUpProgressCanDisableColor(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := newClusterUpProgress(&output, false)
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepareImages, Status: tobari.ClusterUpProgressCompleted,
	})
	progress.Close()

	got := output.String()
	if strings.Contains(got, ansiStyleTokens[styleAccent]) || strings.Contains(got, ansiStyleTokens[styleSuccess]) {
		t.Fatalf("color escape remained in disabled output: %q", got)
	}
	if !strings.Contains(got, "⠋ prepare environment") || !strings.Contains(got, "✓ prepare environment") {
		t.Fatalf("plain progress output = %q", got)
	}
}

func TestClusterUpProgressGroupsInternalStepsIntoThreePhases(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	progress := newClusterUpProgress(&output, false)
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepare, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressPrepareImages, Status: tobari.ClusterUpProgressCompleted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressStartServices, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressConnectNetworks, Status: tobari.ClusterUpProgressCompleted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressStarted,
	})
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressVerifyStatus, Status: tobari.ClusterUpProgressCompleted,
	})
	progress.Close()

	got := output.String()
	for _, phase := range []string{"prepare environment", "start services", "verify readiness"} {
		if !strings.Contains(got, phase) {
			t.Fatalf("grouped progress output %q lacks %q", got, phase)
		}
	}
	for _, detail := range []string{"validate policy", "prepare images", "start Gateway and OPA", "verify cluster status"} {
		if strings.Contains(got, detail) {
			t.Fatalf("grouped progress output %q leaked internal detail %q", got, detail)
		}
	}
}

func TestClusterUpProgressStartAdvancesWithoutRuntimeUpdate(t *testing.T) {
	t.Parallel()
	output := &lockedProgressBuffer{}
	progress := newClusterUpProgress(output, true)
	progress.Start()
	defer progress.Close()
	progress.Report(tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressStarted,
	})

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), applyStyleToken(true, styleAccent, "⠙")) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("independent spinner did not advance: %q", output.String())
}

func TestClusterUpProgressUsesFastHumanFriendlyInterval(t *testing.T) {
	t.Parallel()
	if clusterUpSpinnerInterval != 100*time.Millisecond {
		t.Fatalf("spinner interval = %s, want 100ms", clusterUpSpinnerInterval)
	}
}

type lockedProgressBuffer struct {
	mu     sync.Mutex
	output bytes.Buffer
}

func (b *lockedProgressBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.output.Write(value)
}

func (b *lockedProgressBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.output.String()
}
