//go:build !unix

package authproviders

import (
	"os"
	"testing"
)

func TestValidateCurrentUserOwnerFailsClosedWhenUnsupported(t *testing.T) {
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCurrentUserOwner(info); err == nil {
		t.Fatal("validateCurrentUserOwner did not fail closed")
	}
}
