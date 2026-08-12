package credentialhost

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const maxExecutableBytes = 512 << 20

var (
	ErrInvalidExecutable = errors.New("host executable is invalid")
	ErrOutputLimit       = errors.New("host command output exceeded its limit")
)

func resolveExecutable(value string) (string, string, error) {
	if value == "" || strings.IndexByte(value, 0) >= 0 {
		return "", "", ErrInvalidExecutable
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", "", ErrInvalidExecutable
	}
	canonical, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil || !filepath.IsAbs(canonical) || filepath.Clean(canonical) != canonical {
		return "", "", ErrInvalidExecutable
	}
	digest, err := hashExecutable(canonical)
	if err != nil {
		return "", "", err
	}
	return canonical, digest, nil
}

func verifyExecutable(path, expectedDigest string) error {
	canonical, digest, err := resolveExecutable(path)
	if err != nil || canonical != path || digest != expectedDigest {
		return ErrInvalidExecutable
	}
	return nil
}

func hashExecutable(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 || info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > maxExecutableBytes {
		return "", ErrInvalidExecutable
	}
	// #nosec G304 -- resolveExecutable canonicalizes the trusted PATH result;
	// this boundary rejects symlinks, non-regular/world-writable executables,
	// bounds size, and rebinds the bytes to the captured digest before use.
	file, err := os.Open(path)
	if err != nil {
		return "", ErrInvalidExecutable
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxExecutableBytes+1))
	if err != nil || written != info.Size() {
		return "", ErrInvalidExecutable
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type boundedBuffer struct {
	mu      sync.Mutex
	limit   int
	content bytes.Buffer
	failure error
}

func newBoundedBuffer(limit int) *boundedBuffer { return &boundedBuffer{limit: limit} }
func (b *boundedBuffer) Write(content []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil {
		return 0, b.failure
	}
	if len(content) > b.limit-b.content.Len() {
		b.failure = ErrOutputLimit
		return 0, b.failure
	}
	return b.content.Write(content)
}
func (b *boundedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.content.Bytes()...)
}
func (b *boundedBuffer) err() error { b.mu.Lock(); defer b.mu.Unlock(); return b.failure }
