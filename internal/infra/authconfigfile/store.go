// Package authconfigfile persists non-secret authentication setup metadata.
// It is deliberately separate from every credential store.
package authconfigfile

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
)

var (
	ErrUnsafePath  = errors.New("authentication configuration path is unsafe")
	ErrInvalidData = errors.New("authentication configuration data is invalid")
)

// Store owns one injected non-secret configuration path.
type Store struct {
	path string
}

// New returns a store without creating files or directories.
func New(path string) *Store { return &Store{path: path} }

// Decode strictly decodes one bounded schema-versioned document.
func Decode(reader io.Reader) (authn.UserConfiguration, error) {
	if reader == nil {
		return authn.UserConfiguration{}, ErrInvalidData
	}
	limited := io.LimitReader(reader, authn.MaxUserConfigurationBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil || len(data) > authn.MaxUserConfigurationBytes {
		return authn.UserConfiguration{}, ErrInvalidData
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var configuration authn.UserConfiguration
	if err := decoder.Decode(&configuration); err != nil {
		return authn.UserConfiguration{}, ErrInvalidData
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return authn.UserConfiguration{}, ErrInvalidData
	}
	if err := configuration.Validate(); err != nil {
		return authn.UserConfiguration{}, ErrInvalidData
	}
	return configuration.Clone(), nil
}

// Encode validates and returns the canonical bounded document.
func Encode(configuration authn.UserConfiguration) ([]byte, error) {
	if err := configuration.Validate(); err != nil {
		return nil, ErrInvalidData
	}
	data, err := json.Marshal(configuration)
	if err != nil || len(data)+1 > authn.MaxUserConfigurationBytes {
		return nil, ErrInvalidData
	}
	return append(data, '\n'), nil
}

// Load reads a safe regular file. Unix requires owner-only mode; Windows
// enforces regular shape but makes no portable ACL-ownership claim. Missing is
// not an error; every corrupt or unsafe present state fails closed.
func (s *Store) Load(ctx context.Context) (authn.UserConfiguration, bool, error) {
	if err := validateStoreContext(ctx, s); err != nil {
		return authn.UserConfiguration{}, false, err
	}
	parent, name, parentInfo, parentPresent, err := inspectStoreParent(s.path)
	if err != nil {
		return authn.UserConfiguration{}, false, err
	}
	if !parentPresent {
		return authn.UserConfiguration{}, false, nil
	}
	root, err := openVerifiedRoot(parent, parentInfo)
	if err != nil {
		return authn.UserConfiguration{}, false, err
	}
	defer func() { _ = root.Close() }()

	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return authn.UserConfiguration{}, false, nil
	}
	if err != nil || !safeFileInfo(info) {
		return authn.UserConfiguration{}, true, ErrUnsafePath
	}
	file, err := root.Open(name)
	if err != nil {
		return authn.UserConfiguration{}, true, ErrUnsafePath
	}
	defer file.Close()
	opened, err := file.Stat()
	current, currentErr := root.Lstat(name)
	if err != nil || currentErr != nil || !safeFileInfo(current) || !os.SameFile(info, opened) || !os.SameFile(opened, current) {
		return authn.UserConfiguration{}, true, ErrUnsafePath
	}
	configuration, err := Decode(file)
	if err != nil {
		return authn.UserConfiguration{}, true, err
	}
	if err := ctx.Err(); err != nil {
		return authn.UserConfiguration{}, true, err
	}
	if err := revalidateStoreParent(parent, parentInfo); err != nil {
		return authn.UserConfiguration{}, true, err
	}
	if err := revalidateStoreTarget(s.path, opened); err != nil {
		return authn.UserConfiguration{}, true, err
	}
	return configuration, true, nil
}

// Save replaces the target through a same-directory temporary file. Unix
// requires owner-only mode and persists a successful rename with a directory
// sync. Windows enforces regular shape but portable mode bits do not establish
// the file's ACL. It never creates the parent directory. Windows requests
// replace-existing behavior, but the portable API does not guarantee atomicity
// or durability there.
// Errors returned once replacement begins intentionally do not promise that
// the previous configuration remains active.
func (s *Store) Save(ctx context.Context, configuration authn.UserConfiguration) (err error) {
	if err := validateStoreContext(ctx, s); err != nil {
		return err
	}
	data, err := Encode(configuration)
	if err != nil {
		return err
	}
	parent, name, parentInfo, parentPresent, err := inspectStoreParent(s.path)
	if err != nil {
		return err
	}
	if !parentPresent {
		return ErrUnsafePath
	}
	root, err := openVerifiedRoot(parent, parentInfo)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if err := validateReplaceTarget(root, name); err != nil {
		return err
	}

	temporary, temporaryName, err := createRootTemporary(root)
	if err != nil {
		return fmt.Errorf("create authentication configuration temporary file: %w", err)
	}
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = root.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("set authentication configuration permissions: %w", err)
	}
	temporaryInfo, statErr := temporary.Stat()
	rootTemporaryInfo, rootStatErr := root.Lstat(temporaryName)
	if statErr != nil || rootStatErr != nil || !safeFileInfo(temporaryInfo) || !safeFileInfo(rootTemporaryInfo) || !os.SameFile(temporaryInfo, rootTemporaryInfo) {
		return ErrUnsafePath
	}
	written, writeErr := temporary.Write(data)
	if writeErr != nil {
		return fmt.Errorf("write authentication configuration: %w", writeErr)
	}
	if written != len(data) {
		return fmt.Errorf("write authentication configuration: %w", io.ErrShortWrite)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync authentication configuration: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close authentication configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Revalidate the requested directory identity, the staged file identity,
	// and the target shape immediately before replacement. Root.Rename remains
	// confined to the opened directory even if its path is renamed later.
	if err := validateTemporary(root, temporaryName, temporaryInfo, int64(len(data))); err != nil {
		return err
	}
	if err := revalidateStoreParent(parent, parentInfo); err != nil {
		return err
	}
	if err := validateReplaceTarget(root, name); err != nil {
		return err
	}
	if err := root.Rename(temporaryName, name); err != nil {
		return err
	}
	committed = true
	if err := validateTemporary(root, name, temporaryInfo, int64(len(data))); err != nil {
		return err
	}
	if err := syncDirectory(root); err != nil {
		return err
	}
	if err := revalidateStoreParent(parent, parentInfo); err != nil {
		return err
	}
	if err := revalidateStoreTarget(s.path, temporaryInfo); err != nil {
		return err
	}
	return nil
}

// Status reconciles persistent state without writing or repairing it.
func (s *Store) Status(ctx context.Context) authn.ConfigurationStatus {
	configuration, present, err := s.Load(ctx)
	if err != nil {
		problem := "invalid_data"
		if errors.Is(err, ErrUnsafePath) {
			problem = "unsafe_file"
		}
		return authn.ConfigurationStatus{State: authn.ConfigurationStateInvalid, Problem: problem}
	}
	if !present {
		return authn.ConfigurationStatus{State: authn.ConfigurationStateMissing}
	}
	return authn.ConfigurationStatus{State: authn.ConfigurationStateValid, SchemaVersion: configuration.SchemaVersion, Method: configuration.Method}
}

func validateStoreContext(ctx context.Context, store *Store) error {
	if ctx == nil || store == nil || !validStorePath(store.path) {
		return ErrUnsafePath
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func validStorePath(path string) bool {
	if path == "" || !filepath.IsAbs(path) || os.IsPathSeparator(path[len(path)-1]) {
		return false
	}
	name := filepath.Base(path)
	return name != "." && name != ".." && !filepath.IsAbs(name)
}

func safeFileInfo(info os.FileInfo) bool {
	// Windows FileMode permission bits do not represent the target ACL. Shape
	// remains enforceable there, but owner-only access needs a derived platform
	// policy rather than a portable-mode claim.
	return info != nil && info.Mode()&os.ModeSymlink == 0 && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm() == 0o600)
}

func inspectStoreParent(path string) (string, string, fs.FileInfo, bool, error) {
	parent := filepath.Dir(path)
	name := filepath.Base(path)
	info, err := os.Lstat(parent)
	if errors.Is(err, fs.ErrNotExist) {
		return parent, name, nil, false, nil
	}
	if err != nil || !safeDirectoryInfo(info) {
		return "", "", nil, false, ErrUnsafePath
	}
	return parent, name, info, true, nil
}

func openVerifiedRoot(path string, expected fs.FileInfo) (*os.Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrUnsafePath
	}
	opened, statErr := root.Stat(".")
	if statErr != nil || !safeDirectoryInfo(opened) || !os.SameFile(expected, opened) {
		_ = root.Close()
		return nil, ErrUnsafePath
	}
	return root, nil
}

func revalidateStoreParent(path string, expected fs.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil || !safeDirectoryInfo(current) || !os.SameFile(expected, current) {
		return ErrUnsafePath
	}
	return nil
}

func revalidateStoreTarget(path string, expected fs.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil || !safeFileInfo(current) || !os.SameFile(expected, current) {
		return ErrUnsafePath
	}
	return nil
}

func createRootTemporary(root *os.Root) (*os.File, string, error) {
	if root == nil {
		return nil, "", ErrUnsafePath
	}
	for attempt := 0; attempt < 100; attempt++ {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := fmt.Sprintf(".auth-config-%x", random[:])
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		return file, name, nil
	}
	return nil, "", fmt.Errorf("could not allocate a unique authentication configuration temporary file")
}

func validateReplaceTarget(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || !safeFileInfo(info) {
		return ErrUnsafePath
	}
	return nil
}

func validateTemporary(root *os.Root, name string, expected fs.FileInfo, size int64) error {
	current, err := root.Lstat(name)
	if err != nil || !safeFileInfo(current) || !os.SameFile(expected, current) || current.Size() != size {
		return ErrUnsafePath
	}
	return nil
}

func safeDirectoryInfo(info fs.FileInfo) bool {
	return info != nil && info.Mode()&os.ModeSymlink == 0 && info.IsDir() && (runtime.GOOS == "windows" || info.Mode().Perm()&0o077 == 0)
}
