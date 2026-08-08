//go:build unix

package authproviders

import (
	"os"
	"syscall"
	"testing"
	"time"
)

type ownershipFileInfo struct {
	metadata *syscall.Stat_t
}

func (ownershipFileInfo) Name() string       { return "provider.json" }
func (ownershipFileInfo) Size() int64        { return 1 }
func (ownershipFileInfo) Mode() os.FileMode  { return 0o600 }
func (ownershipFileInfo) ModTime() time.Time { return time.Time{} }
func (ownershipFileInfo) IsDir() bool        { return false }
func (i ownershipFileInfo) Sys() any         { return i.metadata }

func TestValidateCurrentUserOwnerRejectsDifferentUID(t *testing.T) {
	current := ownershipFileInfo{metadata: &syscall.Stat_t{Uid: uint32(os.Geteuid())}}
	if err := validateCurrentUserOwner(current); err != nil {
		t.Fatalf("validateCurrentUserOwner(current): %v", err)
	}
	otherUID := uint32(os.Geteuid()) + 1
	other := ownershipFileInfo{metadata: &syscall.Stat_t{Uid: otherUID}}
	if err := validateCurrentUserOwner(other); err == nil {
		t.Fatal("validateCurrentUserOwner accepted a different UID")
	}
	if err := validateCurrentUserOwner(ownershipFileInfo{}); err == nil {
		t.Fatal("validateCurrentUserOwner accepted missing ownership metadata")
	}
}
