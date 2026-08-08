package cli

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const runtimeBuildLogTailBytes = 64 * 1024

// runtimeBuildOutput forwards the complete purpose-bound Docker diagnostic
// stream and retains only a bounded tail for the final human summary.
type runtimeBuildOutput struct {
	out   io.Writer
	color bool

	mu       sync.Mutex
	tail     []byte
	pending  []byte
	metadata tobari.RuntimeBuildProgress
	prepared bool
	failed   bool
}

func newRuntimeBuildOutput(out io.Writer, color bool) *runtimeBuildOutput {
	return &runtimeBuildOutput{out: out, color: color}
}

func (o *runtimeBuildOutput) Write(data []byte) (int, error) {
	if o == nil {
		return len(data), nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.appendTail(data)
	o.writeProjectedLocked(data)
	return len(data), nil
}

func (o *runtimeBuildOutput) Flush() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.flushLocked()
}

func (o *runtimeBuildOutput) Report(event tobari.RuntimeBuildProgress) {
	if o == nil || event.Validate() != nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.metadata = event
	if event.Stage == tobari.RuntimeBuildStagePrepare && event.Status == tobari.RuntimeBuildProgressStarted && !o.prepared {
		o.prepared = true
		if o.out != nil {
			_, _ = fmt.Fprintf(o.out, "Building runtime for context %q...\n\n", event.ContextName)
		}
	}
	if event.Status == tobari.RuntimeBuildProgressFailed {
		o.failed = true
	}
}

func (o *runtimeBuildOutput) WriteFailureSummary() {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.failed || o.out == nil {
		return
	}
	o.flushLocked()
	if len(o.tail) > 0 && o.tail[len(o.tail)-1] != '\n' {
		_, _ = io.WriteString(o.out, "\n")
	}
	step, diagnostic := runtimeBuildFailureDetails(o.metadata.Stage, o.tail)
	marker := applyStyleToken(o.color, styleDanger, "×")
	heading := applyStyleToken(o.color, styleDanger, "Runtime build failed")
	_, _ = fmt.Fprintf(o.out, "\n%s %s\n\n", marker, heading)
	_, _ = fmt.Fprintf(o.out, "Failed step:\n  %s\n\n", escapeTSVCell(step))
	_, _ = fmt.Fprintf(o.out, "Error:\n  %s\n\n", escapeTSVCell(diagnostic))
	_, _ = fmt.Fprintf(o.out, "Dockerfile:\n  %s\n\n", escapeTSVCell(o.metadata.Dockerfile))

	switch o.metadata.Selection {
	case tobari.RuntimeBuildSelectionUnchanged:
		_, _ = io.WriteString(o.out, "State:\n  The previously selected runtime is unchanged.\n")
		if o.metadata.Stage == tobari.RuntimeBuildStageBuild {
			_, _ = io.WriteString(o.out, "  Docker build cache may contain intermediate layers; Tobari did not remove them.\n\n")
		} else {
			_, _ = fmt.Fprintf(o.out, "  The candidate image %s may remain locally; Tobari did not remove it.\n\n", escapeTSVCell(o.metadata.CandidateImage))
		}
	case tobari.RuntimeBuildSelectionUncertain:
		_, _ = io.WriteString(o.out, "State:\n  Runtime promotion could not be confirmed; inspect the active Context before retrying.\n\n")
	case tobari.RuntimeBuildSelectionPromoted:
		_, _ = io.WriteString(o.out, "State:\n  The runtime image was promoted, but the final Context report failed.\n\n")
	}

	_, _ = io.WriteString(o.out, "Next:\n")
	if o.metadata.Selection == tobari.RuntimeBuildSelectionUnchanged {
		_, _ = io.WriteString(o.out, "  Fix the Dockerfile or Docker problem, then run:\n  tobari runtime build\n")
	} else {
		_, _ = io.WriteString(o.out, "  Inspect the active Context with:\n  tobari context show\n")
	}
}

func (o *runtimeBuildOutput) writeProjectedLocked(data []byte) {
	if o.out == nil {
		return
	}
	data = append(o.pending, data...)
	o.pending = o.pending[:0]
	var projected strings.Builder
	for len(data) > 0 {
		if data[0] == '\n' {
			projected.WriteByte('\n')
			data = data[1:]
			continue
		}
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 && !utf8.FullRune(data) {
			o.pending = append(o.pending, data...)
			break
		}
		if r == utf8.RuneError && size == 1 {
			projected.WriteRune('�')
			data = data[1:]
			continue
		}
		writeExternalRune(&projected, r, true)
		data = data[size:]
	}
	_, _ = io.WriteString(o.out, projected.String())
}

func (o *runtimeBuildOutput) flushLocked() {
	if len(o.pending) == 0 || o.out == nil {
		return
	}
	for range o.pending {
		_, _ = io.WriteString(o.out, "�")
	}
	o.pending = o.pending[:0]
}

func (o *runtimeBuildOutput) appendTail(data []byte) {
	if len(data) >= runtimeBuildLogTailBytes {
		o.tail = append(o.tail[:0], data[len(data)-runtimeBuildLogTailBytes:]...)
		return
	}
	if overflow := len(o.tail) + len(data) - runtimeBuildLogTailBytes; overflow > 0 {
		copy(o.tail, o.tail[overflow:])
		o.tail = o.tail[:len(o.tail)-overflow]
	}
	o.tail = append(o.tail, data...)
}

func runtimeBuildFailureDetails(stage tobari.RuntimeBuildStage, log []byte) (string, string) {
	step := runtimeBuildStageLabel(stage)
	diagnostic := "See the Docker/BuildKit output above."
	lines := strings.Split(strings.ReplaceAll(string(log), "\r\n", "\n"), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" {
			continue
		}
		if candidate, ok := buildkitStepLine(line); ok {
			step = candidate
			break
		}
	}
	fallbackDiagnostic := ""
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if !runtimeBuildDiagnosticLine(line) {
			continue
		}
		if runtimeBuildSpecificDiagnosticLine(line) {
			diagnostic = boundedRuntimeBuildSummaryLine(line)
			fallbackDiagnostic = ""
			break
		}
		if fallbackDiagnostic == "" {
			fallbackDiagnostic = boundedRuntimeBuildSummaryLine(line)
		}
	}
	if fallbackDiagnostic != "" {
		diagnostic = fallbackDiagnostic
	}
	return step, diagnostic
}

func buildkitStepLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	if strings.HasPrefix(trimmed, "#") {
		if separator := strings.IndexByte(trimmed, ' '); separator >= 0 {
			trimmed = strings.TrimSpace(trimmed[separator+1:])
		}
	}
	if strings.HasPrefix(trimmed, "[") {
		if end := strings.Index(trimmed, "] "); end >= 0 {
			candidate := strings.TrimSuffix(strings.TrimSpace(trimmed[end+2:]), ":")
			if candidate != "" {
				return boundedRuntimeBuildSummaryLine(candidate), true
			}
		}
	}
	if strings.Contains(line, "failed to read dockerfile") || strings.Contains(line, "Dockerfile parse error") {
		return "Parse Dockerfile", true
	}
	if strings.Contains(line, "Cannot connect to the Docker daemon") || strings.Contains(line, "error during connect") {
		return "Connect to Docker daemon", true
	}
	return "", false
}

func runtimeBuildDiagnosticLine(line string) bool {
	if line == "" {
		return false
	}
	lower := strings.ToLower(line)
	return strings.Contains(line, "ERROR:") || strings.Contains(lower, "error during connect") ||
		strings.Contains(lower, "cannot connect to the docker daemon") || strings.Contains(lower, "not found") ||
		strings.Contains(lower, "failed to resolve") || strings.Contains(lower, "failed to read dockerfile") ||
		strings.Contains(lower, "pull access denied") || strings.Contains(lower, "dockerfile parse error")
}

func runtimeBuildSpecificDiagnosticLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "/bin/sh:") || strings.Contains(lower, "not found") ||
		strings.Contains(lower, "error during connect") || strings.Contains(lower, "cannot connect to the docker daemon") ||
		strings.Contains(lower, "failed to resolve") || strings.Contains(lower, "failed to read dockerfile") ||
		strings.Contains(lower, "pull access denied") || strings.Contains(lower, "dockerfile parse error")
}

func runtimeBuildStageLabel(stage tobari.RuntimeBuildStage) string {
	switch stage {
	case tobari.RuntimeBuildStagePrepare:
		return "Prepare runtime build"
	case tobari.RuntimeBuildStageBuild:
		return "Docker/BuildKit build"
	case tobari.RuntimeBuildStageValidate:
		return "Validate Tobari runtime contract"
	case tobari.RuntimeBuildStageInspect:
		return "Inspect built image identity"
	case tobari.RuntimeBuildStagePromote:
		return "Promote runtime image to active Context"
	case tobari.RuntimeBuildStageReport:
		return "Read promoted Context state"
	default:
		return "Runtime build"
	}
}

func boundedRuntimeBuildSummaryLine(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 512 {
		value = string(runes[:512]) + "..."
	}
	return string(bytes.ToValidUTF8([]byte(value), []byte("�")))
}
