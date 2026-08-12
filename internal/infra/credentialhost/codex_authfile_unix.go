//go:build darwin || linux

package credentialhost

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
)

func requirePrivateCodexDirectory(path string) error {
	info, err := os.Lstat(path)
	metadata, metadataOK := infoSyscallStat(info)
	ownerUID, ownerOK := codexEffectiveUID()
	if err != nil || !metadataOK || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
		!ownerOK || info.Mode().Perm() != 0o700 || metadata.Uid != ownerUID {
		return ErrCodexLoginSetup
	}
	return nil
}

func readPrivateCodexAuthFile(directory, name string, limit int64) ([]byte, error) {
	if err := requirePrivateCodexDirectory(directory); err != nil || filepath.Base(name) != name ||
		name == "." || limit <= 0 {
		return nil, ErrCodexAuthCapture
	}
	path := filepath.Join(directory, name)
	descriptor, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, ErrCodexAuthCapture
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = syscall.Close(descriptor)
		return nil, ErrCodexAuthCapture
	}
	var metadata syscall.Stat_t
	ownerUID, ownerOK := codexEffectiveUID()
	if err := syscall.Fstat(descriptor, &metadata); err != nil ||
		!ownerOK || metadata.Mode&syscall.S_IFMT != syscall.S_IFREG || metadata.Uid != ownerUID ||
		metadata.Mode&0o077 != 0 || metadata.Nlink != 1 || metadata.Size <= 0 || metadata.Size > limit {
		_ = file.Close()
		return nil, ErrCodexAuthCapture
	}
	content, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || int64(len(content)) != metadata.Size {
		clear(content)
		return nil, ErrCodexAuthCapture
	}
	return content, nil
}

func infoSyscallStat(info os.FileInfo) (*syscall.Stat_t, bool) {
	if info == nil {
		return nil, false
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	return metadata, ok
}

func codexEffectiveUID() (uint32, bool) {
	uid := os.Geteuid()
	if uid < 0 || uint64(uid) > uint64(^uint32(0)) {
		return 0, false
	}
	// #nosec G115 -- negative and wider values are rejected above.
	return uint32(uid), true
}
