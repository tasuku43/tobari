package cli

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	firstEntryAntiFlicker = 250 * time.Millisecond
	firstEntryElapsed     = time.Second
	firstEntryWaitReason  = 10 * time.Second
	firstEntryHeartbeat   = 30 * time.Second
)

type firstEntryProgressTiming struct {
	antiFlicker time.Duration
	elapsed     time.Duration
	waitReason  time.Duration
	heartbeat   time.Duration
}

func defaultFirstEntryProgressTiming() firstEntryProgressTiming {
	return firstEntryProgressTiming{
		antiFlicker: firstEntryAntiFlicker,
		elapsed:     firstEntryElapsed,
		waitReason:  firstEntryWaitReason,
		heartbeat:   firstEntryHeartbeat,
	}
}

func firstEntryProgressAllowed(ctx context.Context) bool {
	return invocationErrorFormat(ctx) != errorFormatJSON
}

type firstEntryProgress struct {
	out             io.Writer
	timing          firstEntryProgressTiming
	now             func() time.Time
	existingContext bool
	interactive     bool
	color           bool

	mu        sync.Mutex
	current   tobari.FirstEntryStage
	startedAt time.Time
	lastIndex int
	frame     int
	heading   bool
	stop      chan struct{}
	done      chan struct{}
}

func newFirstEntryProgress(out io.Writer, existingContext, interactive, color bool) *firstEntryProgress {
	return newFirstEntryProgressWithTiming(out, existingContext, interactive, color, defaultFirstEntryProgressTiming())
}

func newFirstEntryProgressWithTiming(
	out io.Writer, existingContext, interactive, color bool, timing firstEntryProgressTiming,
) *firstEntryProgress {
	return &firstEntryProgress{
		out:             out,
		timing:          timing,
		now:             time.Now,
		existingContext: existingContext,
		interactive:     interactive,
		color:           color,
		lastIndex:       -1,
	}
}

func (p *firstEntryProgress) Apply(event tobari.FirstEntryProgress) error {
	if err := event.Validate(); err != nil {
		return err
	}
	if event.State == tobari.FirstEntryStageRunning {
		return p.Start(event.Stage)
	}
	return p.Finish(event.State)
}

func (p *firstEntryProgress) Start(stage tobari.FirstEntryStage) error {
	if p == nil || p.out == nil {
		return nil
	}
	event := tobari.FirstEntryProgress{Stage: stage, State: tobari.FirstEntryStageRunning}
	if err := event.Validate(); err != nil {
		return err
	}
	index := tobari.FirstEntryStageIndex(stage)
	p.mu.Lock()
	if p.current != "" || index <= p.lastIndex {
		p.mu.Unlock()
		return fmt.Errorf("first-entry progress stage is out of order")
	}
	if !p.heading {
		heading := "Tobari · Starting Workspace"
		if p.existingContext {
			heading = "Tobari · Entering Workspace"
		}
		fmt.Fprintln(p.out, applyStyleToken(p.color, styleAccent, heading))
		fmt.Fprintln(p.out)
		p.heading = true
	}
	p.current = stage
	p.startedAt = p.now()
	p.frame = 0
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	stop, done := p.stop, p.done
	p.mu.Unlock()

	go p.run(stage, stop, done)
	return nil
}

func (p *firstEntryProgress) Finish(state tobari.FirstEntryStageState) error {
	if p == nil || p.out == nil {
		return nil
	}
	p.mu.Lock()
	stage := p.current
	if stage == "" {
		p.mu.Unlock()
		return fmt.Errorf("first-entry progress has no running stage")
	}
	event := tobari.FirstEntryProgress{Stage: stage, State: state}
	if err := event.Validate(); err != nil {
		p.mu.Unlock()
		return err
	}
	if state == tobari.FirstEntryStagePending || state == tobari.FirstEntryStageRunning {
		p.mu.Unlock()
		return fmt.Errorf("first-entry progress cannot finish as %q", state)
	}
	stop, done := p.stop, p.done
	p.current = ""
	p.lastIndex = tobari.FirstEntryStageIndex(stage)
	close(stop)
	p.mu.Unlock()
	<-done

	p.mu.Lock()
	p.writeLocked(firstEntryStageMarker(state), p.stageLabel(stage), "")
	p.mu.Unlock()
	return nil
}

func (p *firstEntryProgress) run(stage tobari.FirstEntryStage, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	antiFlicker := time.NewTimer(p.timing.antiFlicker)
	defer antiFlicker.Stop()
	select {
	case <-antiFlicker.C:
		p.writeRunning(stage, false)
	case <-stop:
		return
	}
	if p.interactive {
		p.runInteractive(stage, stop)
		return
	}

	elapsedDelay := p.timing.elapsed - p.timing.antiFlicker
	if elapsedDelay < 0 {
		elapsedDelay = 0
	}
	elapsedTimer := time.NewTimer(elapsedDelay)
	defer elapsedTimer.Stop()
	select {
	case <-elapsedTimer.C:
		p.writeRunning(stage, true)
	case <-stop:
		return
	}

	waitDelay := p.timing.waitReason - p.timing.elapsed
	if waitDelay < 0 {
		waitDelay = 0
	}
	waitTimer := time.NewTimer(waitDelay)
	defer waitTimer.Stop()
	select {
	case <-waitTimer.C:
		p.writeWaiting(stage)
	case <-stop:
		return
	}

	heartbeat := time.NewTicker(p.timing.heartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-heartbeat.C:
			p.writeWaiting(stage)
		case <-stop:
			return
		}
	}
}

func (p *firstEntryProgress) runInteractive(stage tobari.FirstEntryStage, stop <-chan struct{}) {
	p.writeRunning(stage, false)
	ticker := time.NewTicker(interactiveSpinnerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.writeRunning(stage, p.now().Sub(p.startedAt) >= p.timing.elapsed)
		case <-stop:
			return
		}
	}
}

func (p *firstEntryProgress) writeRunning(stage tobari.FirstEntryStage, elapsed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != stage {
		return
	}
	detail := ""
	if elapsed {
		detail = firstEntryElapsedText(p.now().Sub(p.startedAt))
	}
	if p.now().Sub(p.startedAt) >= p.timing.waitReason {
		detail = firstEntryElapsedText(p.now().Sub(p.startedAt)) + " · " + p.stageWaitReason(stage)
	}
	marker := "..."
	if p.interactive {
		marker = interactiveSpinnerFrame(p.frame)
		p.frame++
	}
	p.writeLockedWithToken(marker, p.stageLabel(stage), detail, styleAccent, false)
}

func (p *firstEntryProgress) writeWaiting(stage tobari.FirstEntryStage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current != stage {
		return
	}
	detail := firstEntryElapsedText(p.now().Sub(p.startedAt)) + " · " + p.stageWaitReason(stage)
	p.writeLockedWithToken("...", p.stageLabel(stage), detail, styleText, true)
}

func (p *firstEntryProgress) writeLocked(marker, label, detail string) {
	token := styleText
	switch marker {
	case "✓":
		token = styleSuccess
	case "!":
		token = styleWarning
	case "×":
		token = styleDanger
	}
	p.writeLockedWithToken(marker, label, detail, token, true)
}

func (p *firstEntryProgress) writeLockedWithToken(marker, label, detail string, token styleToken, newline bool) {
	if p.interactive {
		_, _ = fmt.Fprint(p.out, "\r\x1b[2K")
	}
	styledMarker := applyStyleToken(p.color, token, marker)
	if detail == "" {
		_, _ = fmt.Fprintf(p.out, "%s %s", styledMarker, label)
		if newline {
			_, _ = fmt.Fprintln(p.out)
		}
		return
	}
	_, _ = fmt.Fprintf(p.out, "%s %s · %s", styledMarker, label, detail)
	if newline {
		_, _ = fmt.Fprintln(p.out)
	}
}

func firstEntryElapsedText(elapsed time.Duration) string {
	if elapsed < 0 {
		elapsed = 0
	}
	return elapsed.Truncate(time.Second).String()
}

func (p *firstEntryProgress) stageLabel(stage tobari.FirstEntryStage) string {
	switch stage {
	case tobari.FirstEntryCheckRequirements:
		if p.existingContext {
			return "Check environment"
		}
		return "Check requirements"
	case tobari.FirstEntryResolveContext:
		if p.existingContext {
			return "Use Context"
		}
		return "Save setup"
	case tobari.FirstEntryPrepareProtection:
		if p.existingContext {
			return "Ensure protection"
		}
		return "Prepare protection"
	case tobari.FirstEntryPrepareWorkspace:
		if p.existingContext {
			return "Ensure Workspace"
		}
		return "Prepare Workspace"
	case tobari.FirstEntryEnterWorkspace:
		return "Enter Workspace"
	default:
		return "Unknown stage"
	}
}

func (p *firstEntryProgress) stageWaitReason(stage tobari.FirstEntryStage) string {
	switch stage {
	case tobari.FirstEntryCheckRequirements:
		return "waiting for the selected Docker Engine"
	case tobari.FirstEntryResolveContext:
		if p.existingContext {
			return "checking the selected Context"
		}
		return "saving the reviewed setup"
	case tobari.FirstEntryPrepareProtection:
		if p.existingContext {
			return "confirming Gateway and OPA readiness"
		}
		return "waiting for Gateway and OPA readiness"
	case tobari.FirstEntryPrepareWorkspace:
		if p.existingContext {
			return "confirming Workspace readiness"
		}
		return "waiting for Workspace reconciliation"
	case tobari.FirstEntryEnterWorkspace:
		return "waiting for child handoff"
	default:
		return "waiting for a bounded checkpoint"
	}
}

func firstEntryStageMarker(state tobari.FirstEntryStageState) string {
	switch state {
	case tobari.FirstEntryStageSucceeded:
		return "✓"
	case tobari.FirstEntryStageSkipped:
		return "–"
	case tobari.FirstEntryStageBlocked:
		return "!"
	case tobari.FirstEntryStageFailed:
		return "×"
	case tobari.FirstEntryStageUnknown:
		return "?"
	default:
		return "·"
	}
}
