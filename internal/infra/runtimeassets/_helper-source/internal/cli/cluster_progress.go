package cli

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const clusterUpSpinnerInterval = interactiveSpinnerInterval

var clusterUpSpinnerFrames = interactiveSpinnerFrames

type clusterUpProgressPhase string

const (
	clusterUpPhasePrepare clusterUpProgressPhase = "prepare environment"
	clusterUpPhaseStart   clusterUpProgressPhase = "start services"
	clusterUpPhaseVerify  clusterUpProgressPhase = "verify readiness"
)

var clusterUpProgressPhases = map[tobari.ClusterUpProgressStep]clusterUpProgressPhase{
	tobari.ClusterUpProgressPrepare:           clusterUpPhasePrepare,
	tobari.ClusterUpProgressPolicy:            clusterUpPhasePrepare,
	tobari.ClusterUpProgressPrepareImages:     clusterUpPhasePrepare,
	tobari.ClusterUpProgressStartServices:     clusterUpPhaseStart,
	tobari.ClusterUpProgressConnectNetworks:   clusterUpPhaseStart,
	tobari.ClusterUpProgressWaitForHealth:     clusterUpPhaseVerify,
	tobari.ClusterUpProgressReconcileProjects: clusterUpPhaseVerify,
	tobari.ClusterUpProgressFinalize:          clusterUpPhaseVerify,
	tobari.ClusterUpProgressVerifyStatus:      clusterUpPhaseVerify,
}

var clusterUpProgressPhaseLastSteps = map[tobari.ClusterUpProgressStep]bool{
	tobari.ClusterUpProgressPrepareImages:   true,
	tobari.ClusterUpProgressConnectNetworks: true,
	tobari.ClusterUpProgressVerifyStatus:    true,
}

type clusterUpProgress struct {
	out         io.Writer
	interactive bool
	color       bool
	mu          sync.Mutex
	current     clusterUpProgressPhase
	failed      bool
	frame       int
	started     bool
	closed      bool
	stop        chan struct{}
	done        chan struct{}
}

func newClusterUpProgress(out io.Writer, color bool) *clusterUpProgress {
	return &clusterUpProgress{out: out, interactive: true, color: color}
}

// Start advances the active spinner independently from runtime progress
// events. Runtime events still control the checklist state; this ticker only
// improves feedback while one event takes time to complete.
func (p *clusterUpProgress) Start() {
	if p == nil || p.out == nil || !p.interactive {
		return
	}
	p.mu.Lock()
	if p.started || p.closed {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	stop, done := p.stop, p.done
	p.mu.Unlock()

	go func() {
		ticker := time.NewTicker(clusterUpSpinnerInterval)
		defer ticker.Stop()
		defer close(done)
		for {
			select {
			case <-ticker.C:
				p.tick()
			case <-stop:
				return
			}
		}
	}()
}

func clusterUpProgressAllowed(ctx context.Context) bool {
	return invocationErrorFormat(ctx) != errorFormatJSON
}

func (p *clusterUpProgress) Report(event tobari.ClusterUpProgress) {
	if p == nil || p.out == nil {
		return
	}
	if err := event.Validate(); err != nil {
		return
	}
	phase, ok := clusterUpProgressPhases[event.Step]
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	switch event.Status {
	case tobari.ClusterUpProgressStarted:
		if p.current != phase {
			p.current = phase
			p.frame = 0
			p.writeLineLocked(p.spinner(), string(phase), false, styleAccent)
		}
		p.failed = false
	case tobari.ClusterUpProgressUpdated:
		if p.current == phase {
			p.frame++
			p.writeLineLocked(p.spinner(), string(phase), false, styleAccent)
		}
	case tobari.ClusterUpProgressCompleted:
		if clusterUpProgressPhaseLastSteps[event.Step] && p.current == phase {
			p.finishLocked(phase, "✓", styleSuccess, false)
		}
	case tobari.ClusterUpProgressFailed:
		if p.current != phase {
			p.current = phase
		}
		p.finishLocked(phase, "×", styleDanger, true)
	}
}

func (p *clusterUpProgress) Fail() {
	if p == nil || p.out == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.failed {
		return
	}
	if p.current != "" {
		p.finishLocked(p.current, "×", styleDanger, true)
		return
	}
	p.writeLineLocked("×", "cluster startup failed", true, styleDanger)
	p.failed = true
}

func (p *clusterUpProgress) Close() {
	if p == nil || p.out == nil {
		return
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	if p.current != "" && !p.failed {
		p.finishLocked(p.current, "×", styleDanger, true)
	}
	p.closed = true
	stop, done := p.stop, p.done
	started := p.started
	if started {
		close(stop)
	}
	p.mu.Unlock()
	if started {
		<-done
	}
}

func (p *clusterUpProgress) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.current == "" {
		return
	}
	p.frame++
	p.writeLineLocked(p.spinner(), string(p.current), false, styleAccent)
}

func (p *clusterUpProgress) finishLocked(label clusterUpProgressPhase, marker string, token styleToken, failed bool) {
	p.current = ""
	p.failed = failed
	p.writeLineLocked(marker, string(label), true, token)
}

func (p *clusterUpProgress) spinner() string {
	return interactiveSpinnerFrame(p.frame)
}

func (p *clusterUpProgress) writeLineLocked(marker, label string, newline bool, token styleToken) {
	if p.interactive {
		_, _ = fmt.Fprint(p.out, "\r\x1b[2K")
	}
	_, _ = fmt.Fprint(p.out, applyStyleToken(p.color, token, marker))
	_, _ = fmt.Fprint(p.out, " ", label)
	if newline {
		_, _ = fmt.Fprintln(p.out)
	}
}
