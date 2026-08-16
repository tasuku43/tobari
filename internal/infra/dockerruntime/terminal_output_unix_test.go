//go:build linux || darwin

package dockerruntime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/tasuku43/tobari/internal/infra/terminal"
)

const terminalOutputHelperEnvironment = "TOBARI_TERMINAL_OUTPUT_HELPER"

func TestTerminalOutputHelper(t *testing.T) {
	if os.Getenv(terminalOutputHelperEnvironment) != "1" {
		return
	}
	if !terminal.IsTerminal(os.Stdout) {
		_, _ = io.WriteString(os.Stderr, "stdout is not a terminal")
		os.Exit(41)
	}
	initial, err := pty.GetsizeFull(os.Stdout)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read initial size: %v", err)
		os.Exit(42)
	}
	resize := make(chan os.Signal, 1)
	signal.Notify(resize, syscall.SIGWINCH)
	_, _ = fmt.Fprintf(os.Stdout, "ready %dx%d\n", initial.Cols, initial.Rows)
	select {
	case <-resize:
	case <-time.After(2 * time.Second):
		_, _ = io.WriteString(os.Stderr, "resize signal not received")
		os.Exit(43)
	}
	resized, err := pty.GetsizeFull(os.Stdout)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read resized size: %v", err)
		os.Exit(44)
	}
	_, _ = fmt.Fprintf(os.Stdout, "resized %dx%d\n", resized.Cols, resized.Rows)
	os.Exit(0)
}

type terminalOutputCapture struct {
	mu    sync.Mutex
	data  bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (w *terminalOutputCapture) Write(input []byte) (int, error) {
	w.mu.Lock()
	written, err := w.data.Write(input)
	ready := strings.Contains(w.data.String(), "ready ")
	w.mu.Unlock()
	if ready {
		w.once.Do(func() { close(w.ready) })
	}
	return written, err
}

func (w *terminalOutputCapture) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.data.String()
}

func TestRunWithTerminalOutputPreservesTTYBytesAndResize(t *testing.T) {
	displayMaster, displaySlave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer displayMaster.Close()
	defer displaySlave.Close()
	if err := pty.Setsize(displayMaster, &pty.Winsize{Rows: 42, Cols: 120}); err != nil {
		t.Fatal(err)
	}

	capture := &terminalOutputCapture{ready: make(chan struct{})}
	var stderr bytes.Buffer
	result := make(chan error, 1)
	go func() {
		result <- runWithTerminalOutput(
			context.Background(), os.Args[0],
			[]string{"-test.run=^TestTerminalOutputHelper$"},
			append(os.Environ(), terminalOutputHelperEnvironment+"=1"),
			nil, displaySlave, capture, &stderr,
		)
	}()

	select {
	case <-capture.ready:
	case <-time.After(2 * time.Second):
		t.Fatalf("terminal helper did not report readiness: output=%q stderr=%q", capture.String(), stderr.String())
	}
	if err := pty.Setsize(displayMaster, &pty.Winsize{Rows: 55, Cols: 132}); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatal(err)
	}
	select {
	case runErr := <-result:
		if runErr != nil {
			t.Fatalf("run attached terminal helper: %v; stderr=%q", runErr, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("terminal helper did not complete after resize")
	}
	if got, want := capture.String(), "ready 120x42\nresized 132x55\n"; got != want {
		t.Fatalf("relayed output = %q, want byte-transparent %q", got, want)
	}
}

type terminalOutputRouteRunner struct {
	regularCalls  int
	terminalCalls int
}

func (r *terminalOutputRouteRunner) Run(
	context.Context, []string, []string, io.Reader, io.Writer, io.Writer,
) error {
	r.regularCalls++
	return nil
}

func (r *terminalOutputRouteRunner) RunWithTerminalOutput(
	_ context.Context, _ []string, _ []string, _ io.Reader,
	_ io.Writer, observedOutput io.Writer, _ io.Writer,
) error {
	r.terminalCalls++
	_, err := io.WriteString(observedOutput, "native output remains visible\n")
	return err
}

func (r *terminalOutputRouteRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

func TestEnterProjectRuntimeUsesTerminalPreservingRunnerForTTY(t *testing.T) {
	runner := &terminalOutputRouteRunner{}
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	manifest := projectRuntimeContext(t, runtime, instance)
	displayMaster, displaySlave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer displayMaster.Close()
	defer displaySlave.Close()

	code, err := runtime.EnterProjectRuntime(
		context.Background(), instance, manifest, instance.Root,
		displaySlave, displaySlave, displaySlave,
	)
	if err != nil || code != 0 {
		t.Fatalf("EnterProjectRuntime() = (%d, %v)", code, err)
	}
	if runner.terminalCalls != 1 || runner.regularCalls != 0 {
		t.Fatalf("terminal calls=%d regular calls=%d", runner.terminalCalls, runner.regularCalls)
	}
}
