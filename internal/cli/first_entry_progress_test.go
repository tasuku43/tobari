package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestFirstEntryProgressUsesFiveCheckpointLabelsWithoutAuthorityClaims(t *testing.T) {
	var output bytes.Buffer
	progress := newFirstEntryProgressWithTiming(&output, false, firstEntryProgressTiming{
		antiFlicker: time.Hour, elapsed: 2 * time.Hour, waitReason: 3 * time.Hour, heartbeat: time.Hour,
	})
	for _, stage := range tobari.FirstEntryStages() {
		if err := progress.Start(stage); err != nil {
			t.Fatal(err)
		}
		if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
			t.Fatal(err)
		}
	}
	got := output.String()
	for _, label := range []string{"Check requirements", "Save setup", "Prepare protection", "Prepare Workspace", "Enter Workspace"} {
		if strings.Count(got, "✓ "+label+"\n") != 1 {
			t.Errorf("progress missing one %q checkpoint: %q", label, got)
		}
	}
	for _, forbidden := range []string{"Manifest", "reconcile", "revision", "principal", "image", "percent", "ETA"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("progress exposed %q: %q", forbidden, got)
		}
	}
}

func TestFirstEntryProgressUsesExistingContextLabelWithoutClaimingMutation(t *testing.T) {
	var output bytes.Buffer
	progress := newFirstEntryProgressWithTiming(&output, true, firstEntryProgressTiming{
		antiFlicker: time.Hour, elapsed: 2 * time.Hour, waitReason: 3 * time.Hour, heartbeat: time.Hour,
	})
	if err := progress.Start(tobari.FirstEntryResolveContext); err != nil {
		t.Fatal(err)
	}
	if err := progress.Finish(tobari.FirstEntryStageSucceeded); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "✓ Use Context\n" {
		t.Fatalf("existing Context checkpoint = %q", got)
	}
}

func TestFirstEntryProgressAntiFlickerElapsedWaitAndHeartbeatAreBounded(t *testing.T) {
	var output bytes.Buffer
	progress := newFirstEntryProgressWithTiming(&output, false, firstEntryProgressTiming{
		antiFlicker: 10 * time.Millisecond,
		elapsed:     20 * time.Millisecond,
		waitReason:  30 * time.Millisecond,
		heartbeat:   40 * time.Millisecond,
	})
	if err := progress.Start(tobari.FirstEntryPrepareProtection); err != nil {
		t.Fatal(err)
	}
	time.Sleep(85 * time.Millisecond)
	if err := progress.Finish(tobari.FirstEntryStageFailed); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if !strings.Contains(got, "… Prepare protection") ||
		!strings.Contains(got, "waiting for Gateway and OPA readiness") ||
		!strings.HasSuffix(got, "✗ Prepare protection\n") {
		t.Fatalf("bounded progress = %q", got)
	}
	if count := strings.Count(got, "waiting for Gateway and OPA readiness"); count < 2 || count > 3 {
		t.Fatalf("heartbeat count = %d, output=%q", count, got)
	}
	for _, forbidden := range []string{"\r", "\x1b", "%", "ETA"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("line progress exposed %q: %q", forbidden, got)
		}
	}
}

func TestFirstEntryProgressRejectsReorderingAndNonterminalFinish(t *testing.T) {
	progress := newFirstEntryProgressWithTiming(&bytes.Buffer{}, false, firstEntryProgressTiming{
		antiFlicker: time.Hour, elapsed: 2 * time.Hour, waitReason: 3 * time.Hour, heartbeat: time.Hour,
	})
	if err := progress.Start(tobari.FirstEntryPrepareWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := progress.Finish(tobari.FirstEntryStageRunning); err == nil {
		t.Fatal("running finish was accepted")
	}
	if err := progress.Finish(tobari.FirstEntryStageSkipped); err != nil {
		t.Fatal(err)
	}
	if err := progress.Start(tobari.FirstEntryResolveContext); err == nil {
		t.Fatal("backward stage was accepted")
	}
}

func TestFirstEntryProgressAppliesTypedSinkEvents(t *testing.T) {
	var output bytes.Buffer
	progress := newFirstEntryProgressWithTiming(&output, false, firstEntryProgressTiming{
		antiFlicker: time.Hour, elapsed: 2 * time.Hour, waitReason: 3 * time.Hour, heartbeat: time.Hour,
	})
	sink := tobari.FirstEntryProgressSink(func(event tobari.FirstEntryProgress) {
		if err := progress.Apply(event); err != nil {
			t.Errorf("apply progress: %v", err)
		}
	})
	sink(tobari.FirstEntryProgress{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageRunning})
	sink(tobari.FirstEntryProgress{Stage: tobari.FirstEntryPrepareWorkspace, State: tobari.FirstEntryStageSucceeded})
	sink(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageRunning})
	sink(tobari.FirstEntryProgress{Stage: tobari.FirstEntryEnterWorkspace, State: tobari.FirstEntryStageSucceeded})
	if got := output.String(); got != "✓ Prepare Workspace\n✓ Enter Workspace\n" {
		t.Fatalf("sink progress = %q", got)
	}
}
