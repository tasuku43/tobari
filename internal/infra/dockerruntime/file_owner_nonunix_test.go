//go:build !unix

package dockerruntime

import (
	"errors"
	"testing"
)

func TestNonUnixFileOwnerAdapterFailsClosed(t *testing.T) {
	if uid, ok := fileOwnerUID(nil); ok || uid != 0 {
		t.Fatalf("unsupported non-Unix owner result = (%d, %t)", uid, ok)
	}
	if isConnectionRefused(errors.New("synthetic connection refusal")) {
		t.Fatal("unsupported non-Unix runtime accepted Unix socket cleanup evidence")
	}
}
