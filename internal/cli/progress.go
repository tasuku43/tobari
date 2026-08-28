package cli

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// The Braille spinner is Tobari's shared active-work signature. It is motion,
// not an authority or completion signal; typed progress events still decide
// the label and terminal outcome.
const interactiveSpinnerInterval = 100 * time.Millisecond

var interactiveSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func interactiveSpinnerFrame(index int) string {
	if index < 0 {
		index = 0
	}
	return interactiveSpinnerFrames[index%len(interactiveSpinnerFrames)]
}

type interactiveWorkProgress struct {
	out         io.Writer
	label       string
	interactive bool
	style       bool
	done        chan struct{}
	finished    chan struct{}
	once        sync.Once
}

func newInteractiveWorkProgress(out io.Writer, label string, interactive, style bool) *interactiveWorkProgress {
	return &interactiveWorkProgress{
		out: out, label: label, interactive: interactive, style: style,
		done: make(chan struct{}), finished: make(chan struct{}),
	}
}

func (p *interactiveWorkProgress) Start() {
	if p == nil || !p.interactive || p.out == nil {
		return
	}
	go func() {
		defer close(p.finished)
		delay := time.NewTimer(250 * time.Millisecond)
		defer delay.Stop()
		select {
		case <-p.done:
			return
		case <-delay.C:
		}
		ticker := time.NewTicker(interactiveSpinnerInterval)
		defer ticker.Stop()
		frame := 0
		for {
			marker := applyStyleToken(p.style, styleAccent, interactiveSpinnerFrame(frame))
			_, _ = fmt.Fprintf(p.out, "\r\x1b[2K%s %s", marker, p.label)
			frame++
			select {
			case <-p.done:
				_, _ = io.WriteString(p.out, "\r\x1b[2K")
				return
			case <-ticker.C:
			}
		}
	}()
}

func (p *interactiveWorkProgress) Stop() {
	if p == nil || !p.interactive {
		return
	}
	p.once.Do(func() { close(p.done) })
	<-p.finished
}
