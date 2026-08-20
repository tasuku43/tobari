//go:build darwin || linux

package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestRunInteractivePTYColorizesStdoutAndKeepsStderrSeparate(t *testing.T) {
	hostMaster, hostInput := openTestPTY(t)
	var stdout, stderr bytes.Buffer
	command := exec.Command("/bin/sh", "-c", `printf '%s' '{"ok":true}'; printf '%s' 'diagnostic' >&2`)

	if err := runInteractivePTY(context.Background(), command, hostInput, &stdout, &stderr); err != nil {
		t.Fatalf("runInteractivePTY() error = %v", err)
	}
	if visible := stripPTYSGR(stdout.Bytes()); visible != `{"ok":true}` {
		t.Fatalf("visible stdout = %q, want JSON without reformatting", visible)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("\x1b[")) {
		t.Fatalf("stdout was not colorized: %q", stdout.Bytes())
	}
	if got := stderr.String(); got != "diagnostic" {
		t.Fatalf("stderr = %q, want unchanged diagnostic", got)
	}
	_ = hostMaster
}

func TestRunInteractivePTYPropagatesInitialWindowSize(t *testing.T) {
	hostMaster, hostInput := openTestPTY(t)
	if err := pty.Setsize(hostMaster, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("pty.Setsize() error = %v", err)
	}
	var stdout bytes.Buffer
	command := exec.Command("/bin/sh", "-c", "stty size")

	if err := runInteractivePTY(context.Background(), command, hostInput, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("runInteractivePTY() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "24 80") {
		t.Fatalf("child window size output = %q, want 24 80", stdout.String())
	}
}

func TestRunInteractivePTYPropagatesWindowResize(t *testing.T) {
	hostMaster, hostInput := openTestPTY(t)
	if err := pty.Setsize(hostMaster, &pty.Winsize{Rows: 24, Cols: 80}); err != nil {
		t.Fatalf("pty.Setsize() error = %v", err)
	}
	var stdout notifyingBuffer
	ready := make(chan struct{})
	stdout.ready = ready
	command := exec.Command("/bin/sh", "-c", `trap 'stty size' WINCH; stty size; sleep 1`)
	runResult := make(chan error, 1)
	go func() {
		runResult <- runInteractivePTY(context.Background(), command, hostInput, &stdout, &bytes.Buffer{})
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("child did not report its initial window size")
	}
	if err := pty.Setsize(hostMaster, &pty.Winsize{Rows: 40, Cols: 100}); err != nil {
		t.Fatalf("pty.Setsize() resize error = %v", err)
	}
	if err := syscall.Kill(os.Getpid(), syscall.SIGWINCH); err != nil {
		t.Fatalf("SIGWINCH = %v", err)
	}

	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("runInteractivePTY() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runInteractivePTY() did not finish after resize")
	}
	if output := stdout.String(); !strings.Contains(output, "24 80") || !strings.Contains(output, "40 100") {
		t.Fatalf("child window sizes = %q, want initial and resized dimensions", output)
	}
}

func TestRunInteractivePTYForwardsInputAfterIdlePeriod(t *testing.T) {
	hostMaster, hostInput := openTestPTY(t)
	var stdout notifyingBuffer
	ready := make(chan struct{})
	stdout.ready = ready
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/sh", "-c", `printf 'ready\n'; IFS= read -r value; printf 'received:%s\n' "$value"`)
	runResult := make(chan error, 1)
	go func() {
		runResult <- runInteractivePTY(ctx, command, hostInput, &stdout, &bytes.Buffer{})
	}()

	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("child did not become ready for input")
	}
	// Exceed the selector mode's VTIME=1 idle interval. A streaming relay must
	// keep waiting for later bytes instead of treating that timeout as EOF.
	time.Sleep(250 * time.Millisecond)
	if _, err := hostMaster.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write delayed host input: %v", err)
	}

	select {
	case err := <-runResult:
		if err != nil {
			t.Fatalf("runInteractivePTY() error = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("child did not receive input after the idle period")
	}
	if output := stdout.String(); !strings.Contains(output, "received:hello") {
		t.Fatalf("child output = %q, want delayed input receipt", output)
	}
}

func TestRunInteractivePTYPreservesChildExitStatus(t *testing.T) {
	_, hostInput := openTestPTY(t)
	command := exec.Command("/bin/sh", "-c", "exit 37")
	err := runInteractivePTY(context.Background(), command, hostInput, &bytes.Buffer{}, &bytes.Buffer{})
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 37 {
		t.Fatalf("runInteractivePTY() error = %v, want child exit 37", err)
	}
}

func TestStructuredOutputColorEnabledRequiresInteractiveStreamsAndHonorsNOColor(t *testing.T) {
	clearNOColorForTest(t)
	_, input := openTestPTY(t)
	_, output := openTestPTY(t)
	if !structuredOutputColorEnabled(input, output, nil) {
		t.Fatal("interactive streams unexpectedly disabled structured color")
	}
	if structuredOutputColorEnabled(input, output, []string{"NO_COLOR=1"}) {
		t.Fatal("child NO_COLOR unexpectedly enabled structured color")
	}
	if structuredOutputColorEnabled(input, bytes.NewBuffer(nil), nil) {
		t.Fatal("non-terminal output unexpectedly enabled structured color")
	}

	t.Setenv("NO_COLOR", "")
	if structuredOutputColorEnabled(input, output, nil) {
		t.Fatal("presence-only NO_COLOR unexpectedly enabled structured color")
	}
}

func TestBoundedPTYDimension(t *testing.T) {
	max := int(^uint16(0))
	for input, want := range map[int]uint16{
		-1:      0,
		0:       0,
		80:      80,
		max:     ^uint16(0),
		max + 1: ^uint16(0),
	} {
		if got := boundedPTYDimension(input); got != want {
			t.Errorf("boundedPTYDimension(%d) = %d, want %d", input, got, want)
		}
	}
}

func clearNOColorForTest(t *testing.T) {
	t.Helper()
	value, present := os.LookupEnv("NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if present {
			_ = os.Setenv("NO_COLOR", value)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	})
}

func openTestPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = master.Close()
		_ = slave.Close()
	})
	return master, slave
}

func stripPTYSGR(data []byte) string {
	var output bytes.Buffer
	for index := 0; index < len(data); {
		if data[index] == 0x1b && index+2 < len(data) && data[index+1] == '[' {
			if end := bytes.IndexByte(data[index+2:], 'm'); end >= 0 {
				index += end + 3
				continue
			}
		}
		output.WriteByte(data[index])
		index++
	}
	return output.String()
}

type notifyingBuffer struct {
	bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (b *notifyingBuffer) Write(data []byte) (int, error) {
	b.once.Do(func() { close(b.ready) })
	return b.Buffer.Write(data)
}
