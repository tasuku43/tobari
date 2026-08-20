//go:build darwin || linux

package terminal

import (
	"reflect"
	"syscall"
	"testing"

	"github.com/creack/pty"
)

func TestRawModesDeclareReadCompletionAndRestoreTerminal(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    Mode
		minimum byte
		timeout byte
	}{
		{name: "polling", mode: New(), minimum: 0, timeout: 1},
		{name: "stream", mode: NewStream(), minimum: 1, timeout: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			master, input, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer master.Close()
			defer input.Close()

			original, err := getTermios(input.Fd())
			if err != nil {
				t.Fatalf("read original terminal mode: %v", err)
			}
			restore, err := test.mode.Enter(input)
			if err != nil {
				t.Fatalf("enter raw mode: %v", err)
			}
			raw, err := getTermios(input.Fd())
			if err != nil {
				t.Fatalf("read raw terminal mode: %v", err)
			}
			if raw.Lflag&canonicalModeFlag != 0 || raw.Cc[syscall.VMIN] != test.minimum || raw.Cc[syscall.VTIME] != test.timeout {
				t.Fatalf("raw mode canonical=%t VMIN=%d VTIME=%d, want false/%d/%d", raw.Lflag&canonicalModeFlag != 0, raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME], test.minimum, test.timeout)
			}
			if err := restore(); err != nil {
				t.Fatalf("restore terminal mode: %v", err)
			}
			restored, err := getTermios(input.Fd())
			if err != nil {
				t.Fatalf("read restored terminal mode: %v", err)
			}
			if !reflect.DeepEqual(restored, original) {
				t.Fatalf("restored terminal mode differs from original")
			}
		})
	}
}
