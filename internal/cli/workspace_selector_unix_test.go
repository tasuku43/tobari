//go:build darwin || linux

package cli

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestReadSelectorByteTreatsCharDevicePollAsTimeout(t *testing.T) {
	file, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	_, err = readSelectorByte(context.Background(), file)
	if !errors.Is(err, errSelectorTimeout) {
		t.Fatalf("char-device zero-byte read error = %v, want selector timeout", err)
	}
}
