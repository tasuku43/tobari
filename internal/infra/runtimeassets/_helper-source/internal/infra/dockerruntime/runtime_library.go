package dockerruntime

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimeSourceFileMode = 0o600
	maxRuntimeSourceFiles = 1024
	maxRuntimeSourceDirs  = 256
	maxRuntimeSourceFile  = int64(32 * 1024 * 1024)
	maxRuntimeSourceTotal = int64(64 * 1024 * 1024)
	runtimeSourceCopySize = 128 * 1024
)

const managedRuntimeTemplate = `# This directory is the complete build context for this Tobari Runtime.
# Add scripts and configuration beside this Dockerfile and COPY them explicitly.
FROM %s

USER root

# Example:
# COPY install-tools.sh /tmp/install-tools.sh
# RUN /tmp/install-tools.sh && rm /tmp/install-tools.sh

USER tobari
`

func (r *Runtime) runtimesDirectory() string { return filepath.Join(r.configDirectory, "runtimes") }
func (r *Runtime) runtimeDirectory(name string) string {
	return filepath.Join(r.runtimesDirectory(), name)
}
func (r *Runtime) runtimeSourceDirectory(name string) string {
	return filepath.Join(r.runtimeDirectory(name), "source")
}
func (r *Runtime) runtimeRevisionsDirectory(name string) string {
	return filepath.Join(r.runtimeDirectory(name), "revisions")
}
func (r *Runtime) runtimeManifestPath(name string) string {
	return filepath.Join(r.runtimeDirectory(name), "runtime.json")
}

func (r *Runtime) standardRuntimeManifest() tobari.RuntimeManifest {
	image := r.defaultRuntimeImage()
	digest := sha256.Sum256([]byte("tobari-standard-runtime\x00" + image))
	return tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion,
		ID:            tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{
			Ordinal: 1, Revision: "sha256:" + hex.EncodeToString(digest[:]), Image: image,
			CreatedAt: time.Unix(0, 0).UTC(),
		}},
	}
}

func (r *Runtime) resolveRuntimeBinding(selection string) (tobari.RuntimeBinding, error) {
	name, ordinal, err := tobari.ParseRuntimeSelection(selection)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	manifest, err := r.readRuntimeManifest(name)
	if err != nil {
		return tobari.RuntimeBinding{}, err
	}
	return manifest.Binding(ordinal)
}

// SetContextRuntime explicitly replaces one Context's exact Runtime binding.
// Runtime builds never call this method.
func (r *Runtime) SetContextRuntime(ctx context.Context, contextName, selection string) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	binding, err := r.resolveRuntimeBinding(selection)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	var result tobari.ManifestReport
	err = r.withContextStoreLock(func() error {
		active, err := r.readDefaultManifestName()
		if err != nil {
			return err
		}
		if contextName == "" {
			contextName = active
		}
		manifest, err := r.readContextManifest(contextName)
		if err != nil {
			return err
		}
		previous := manifest
		manifest.Runtime = nil
		copy := binding
		manifest.RuntimeBinding = &copy
		manifest.Image = binding.Image
		manifest, err = r.publishWorkspaceManifestUpdate(previous, manifest)
		if err != nil {
			return fmt.Errorf("write Context Runtime binding: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskManifestRuntimeSet, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func (r *Runtime) withRuntimeStoreLock(ctx context.Context, action func() error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.runtimeStoreLockAttempt != nil {
		r.runtimeStoreLockAttempt()
	}
	if err := r.ensurePrivateDirectory(r.configDirectory); err != nil {
		return err
	}
	path := filepath.Join(r.configDirectory, "runtimes.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("Runtime lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed owner-only configuration child.
	if err != nil {
		return err
	}
	acquired := false
	defer func() {
		if acquired {
			unlockProjectFile(file)
		}
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close Runtime store lock: %w", closeErr)
		}
	}()
	for {
		locked, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return lockErr
		}
		if locked {
			acquired = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return action()
}

func (r *Runtime) readRuntimeManifest(name string) (tobari.RuntimeManifest, error) {
	if name == tobari.StandardRuntimeName {
		return r.standardRuntimeManifest(), nil
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.RuntimeManifest{}, err
	}
	if err := requirePrivateDirectory(r.runtimesDirectory()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
		}
		return tobari.RuntimeManifest{}, fmt.Errorf("inspect Runtime catalog: %w", err)
	}
	if err := requirePrivateDirectory(r.runtimeDirectory(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
		}
		return tobari.RuntimeManifest{}, fmt.Errorf("inspect Runtime directory: %w", err)
	}
	path := r.runtimeManifestPath(name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime manifest must be a regular owner-only file")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
	} else {
		return tobari.RuntimeManifest{}, err
	}
	var manifest tobari.RuntimeManifest
	if err := readStrictJSON(path, &manifest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
		}
		return tobari.RuntimeManifest{}, fmt.Errorf("read Runtime manifest: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return tobari.RuntimeManifest{}, fmt.Errorf("validate Runtime manifest: %w", err)
	}
	if manifest.Name != name || manifest.Kind != tobari.RuntimeKindManaged || manifest.SourcePath != r.runtimeSourceDirectory(name) {
		return tobari.RuntimeManifest{}, fmt.Errorf("Runtime manifest does not match its store path")
	}
	for _, revision := range manifest.Revisions {
		expected := filepath.Join(r.runtimeRevisionsDirectory(name), strings.TrimPrefix(revision.Revision, "sha256:"), "source")
		if revision.SnapshotPath != expected {
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime revision snapshot does not match its semantic identity")
		}
		if err := requirePrivateDirectory(expected); err != nil {
			return tobari.RuntimeManifest{}, fmt.Errorf("inspect Runtime revision snapshot: %w", err)
		}
	}
	return manifest, nil
}

// ListRuntimes observes the complete local Runtime catalog without creating it.
func (r *Runtime) ListRuntimes(ctx context.Context) (tobari.RuntimeListResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeListResult{}, err
	}
	items := []tobari.RuntimeSummary{tobari.RuntimeSummaryFrom(r.standardRuntimeManifest())}
	entries, err := os.ReadDir(r.runtimesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		result := tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: items}
		return result, result.Validate()
	}
	if err != nil {
		return tobari.RuntimeListResult{}, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return tobari.RuntimeListResult{}, fmt.Errorf("Runtime catalog contains a symbolic link")
		}
		if !entry.IsDir() {
			continue
		}
		manifest, err := r.readRuntimeManifest(entry.Name())
		if err != nil {
			return tobari.RuntimeListResult{}, err
		}
		items = append(items, tobari.RuntimeSummaryFrom(manifest))
	}
	sort.Slice(items[1:], func(i, j int) bool { return items[i+1].Name < items[j+1].Name })
	result := tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: items}
	return result, result.Validate()
}

// ResolveRuntimeReference derives every bounded local Runtime reference and
// compares the supplied opaque value unchanged. It never decodes a name,
// ordinal, path, image selector, or Docker identity from the reference.
func (r *Runtime) ResolveRuntimeReference(ctx context.Context, reference string) (tobari.RuntimeManifest, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeManifest{}, err
	}
	if err := tobari.ValidateRuntimeRef(reference); err != nil {
		return tobari.RuntimeManifest{}, err
	}
	if reference == tobari.StandardRuntimeID {
		return r.standardRuntimeManifest(), nil
	}
	// Compare the requested ID only after each current manifest is read. Never
	// carry a mutable name from an earlier catalog snapshot into a second read.
	return r.resolveManagedRuntimeReferenceUnlocked(reference)
}

func (r *Runtime) ShowRuntime(ctx context.Context, name string) (tobari.RuntimeReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	manifest, err := r.readRuntimeManifest(name)
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	result := tobari.RuntimeReport{Task: tobari.TaskRuntimeShow, Runtime: manifest}
	return result, result.Validate()
}

func (r *Runtime) RuntimeHistory(ctx context.Context, name string) (tobari.RuntimeReport, error) {
	report, err := r.ShowRuntime(ctx, name)
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	report.Task = tobari.TaskRuntimeHistory
	return report, report.Validate()
}

// CreateRuntime initializes one standalone managed source tree from an exact
// source Base without building it or retaining Base lineage.
func (r *Runtime) CreateRuntime(ctx context.Context, name string, base tobari.RuntimeCopySource) (tobari.RuntimeReport, error) {
	var result tobari.RuntimeReport
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		var createErr error
		result, createErr = r.createRuntimeLifecycleLocked(lockContext, name, base)
		return createErr
	})
	return result, err
}

// createRuntimeLifecycleLocked requires the installation lifecycle lock.
func (r *Runtime) createRuntimeLifecycleLocked(ctx context.Context, name string, base tobari.RuntimeCopySource) (tobari.RuntimeReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := base.Validate(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if name == tobari.StandardRuntimeName {
		return tobari.RuntimeReport{}, tobari.ErrRuntimeExists
	}
	var result tobari.RuntimeReport
	err := r.withRuntimeStoreLock(ctx, func() error {
		if _, err := os.Lstat(r.runtimeDirectory(name)); err == nil {
			return tobari.ErrRuntimeExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := r.ensurePrivateDirectory(r.runtimesDirectory()); err != nil {
			return err
		}
		staging, err := os.MkdirTemp(r.configDirectory, ".runtime-create-")
		if err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(staging) }() // #nosec G301 -- exact MkdirTemp-owned staging child.
		stagedSource := filepath.Join(staging, "source")
		if err := os.Mkdir(filepath.Join(staging, "revisions"), 0o700); err != nil {
			return err
		}
		id, err := r.identities.newRuntimeID()
		if err != nil {
			return err
		}
		manifest := tobari.RuntimeManifest{
			SchemaVersion: tobari.RuntimeSchemaVersion, ID: id, Name: name, Kind: tobari.RuntimeKindManaged,
			SourcePath: r.runtimeSourceDirectory(name), Revisions: []tobari.RuntimeRevision{},
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if base == tobari.RuntimeCopySource(tobari.StandardRuntimeName) {
			if err := os.Mkdir(stagedSource, 0o700); err != nil {
				return err
			}
			if err := initializeBytes(filepath.Join(stagedSource, "Dockerfile"), []byte(fmt.Sprintf(managedRuntimeTemplate, r.defaultRuntimeImage())), runtimeSourceFileMode); err != nil {
				return err
			}
		} else {
			baseManifest, err := r.readRuntimeManifest(string(base))
			if err != nil {
				return err
			}
			if _, err := copyRuntimeSource(ctx, baseManifest.SourcePath, stagedSource); err != nil {
				return err
			}
		}
		if err := writeAtomicJSON(filepath.Join(staging, "runtime.json"), manifest); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Rename(staging, r.runtimeDirectory(name)); err != nil {
			if errors.Is(err, os.ErrExist) {
				return tobari.ErrRuntimeExists
			}
			return err
		}
		result = tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}
		return result.Validate()
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	return result, nil
}

type runtimeSourceEntry struct {
	relative string
	mode     os.FileMode
	size     int64
	info     os.FileInfo
}

type runtimeSourceDirectoryEntry struct {
	relative string
	mode     os.FileMode
	info     os.FileInfo
}

type contextCheckedReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextCheckedReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func runtimeSourceInvalid(message string) error {
	return fault.New(
		fault.KindRejected,
		"runtime_source_invalid",
		message,
		false,
		fault.NextAction{Command: "runtime show", Reason: "Inspect the unchanged Runtime source path and history."},
	)
}

func runtimeSourcePathLabel(relative string) string {
	value := filepath.ToSlash(relative)
	quoted := strconv.QuoteToGraphic(value)
	if len(quoted) <= 512 {
		return quoted
	}
	digest := sha256.Sum256([]byte(value))
	return fmt.Sprintf("path-sha256:%x (projected path exceeded 512 bytes)", digest)
}

func runtimeSourceSizeMessage(kind, relative string, actual, limit int64) string {
	return fmt.Sprintf(
		"Runtime source %s %s is %d bytes; the limit is %d bytes (%d MiB).",
		kind,
		runtimeSourcePathLabel(relative),
		actual,
		limit,
		limit/(1024*1024),
	)
}

func copyRuntimeSource(ctx context.Context, root, snapshot string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := requirePrivateDirectory(root); err != nil {
		return "", fmt.Errorf("inspect Runtime source: %w", err)
	}
	sourceRoot, err := os.OpenRoot(root)
	if err != nil {
		return "", fmt.Errorf("open Runtime source: %w", err)
	}
	defer sourceRoot.Close()
	entries := make([]runtimeSourceEntry, 0)
	directoryEntries := make([]runtimeSourceDirectoryEntry, 0)
	total := int64(0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || filepath.Clean(relative) != relative || filepath.IsAbs(relative) {
			return runtimeSourceInvalid("Runtime source contains a path that is not a canonical relative child.")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return runtimeSourceInvalid(fmt.Sprintf("Runtime source path %s is a symbolic link; only regular files and directories are accepted.", runtimeSourcePathLabel(relative)))
		}
		if entry.IsDir() {
			if len(directoryEntries) >= maxRuntimeSourceDirs {
				return runtimeSourceInvalid(fmt.Sprintf("Runtime source contains %d directories; the limit is %d.", len(directoryEntries)+1, maxRuntimeSourceDirs))
			}
			if info.Mode().Perm()&0o077 != 0 {
				return runtimeSourceInvalid(fmt.Sprintf(
					"Runtime source directory %s has permissions %04o; remove all group/other permissions so it is owner-only (normally 0700).",
					runtimeSourcePathLabel(relative), info.Mode().Perm(),
				))
			}
			directoryEntries = append(directoryEntries, runtimeSourceDirectoryEntry{relative: filepath.ToSlash(relative), mode: info.Mode().Perm(), info: info})
			return nil
		}
		if !info.Mode().IsRegular() {
			return runtimeSourceInvalid(fmt.Sprintf("Runtime source path %s is a special file; only regular files and directories are accepted.", runtimeSourcePathLabel(relative)))
		}
		if info.Mode().Perm()&0o077 != 0 {
			return runtimeSourceInvalid(fmt.Sprintf(
				"Runtime source file %s has permissions %04o; remove all group/other permissions so it is owner-only (for example 0600 or 0700).",
				runtimeSourcePathLabel(relative), info.Mode().Perm(),
			))
		}
		if info.Size() < 0 || info.Size() > maxRuntimeSourceFile {
			return runtimeSourceInvalid(runtimeSourceSizeMessage("file", relative, info.Size(), maxRuntimeSourceFile))
		}
		if len(entries) >= maxRuntimeSourceFiles {
			return runtimeSourceInvalid(fmt.Sprintf("Runtime source contains %d files; the limit is %d.", len(entries)+1, maxRuntimeSourceFiles))
		}
		total += info.Size()
		if total > maxRuntimeSourceTotal {
			return runtimeSourceInvalid(fmt.Sprintf(
				"Runtime source totals %d bytes after file %s; the total limit is %d bytes (%d MiB).",
				total, runtimeSourcePathLabel(relative), maxRuntimeSourceTotal, maxRuntimeSourceTotal/(1024*1024),
			))
		}
		entries = append(entries, runtimeSourceEntry{relative: filepath.ToSlash(relative), mode: info.Mode().Perm(), size: info.Size(), info: info})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", runtimeSourceInvalid("Runtime source is empty; add a regular Dockerfile before building.")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relative < entries[j].relative })
	if err := os.Mkdir(snapshot, 0o700); err != nil {
		return "", err
	}
	for _, entry := range directoryEntries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		relative := filepath.FromSlash(entry.relative)
		current, err := sourceRoot.Lstat(relative)
		if err != nil {
			return "", err
		}
		if !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(entry.info, current) || current.Mode().Perm() != entry.mode {
			return "", runtimeSourceInvalid(fmt.Sprintf("Runtime source directory %s changed during snapshot; wait for edits to finish and retry.", runtimeSourcePathLabel(relative)))
		}
		if err := os.Mkdir(filepath.Join(snapshot, relative), 0o700); err != nil { // #nosec G301 -- final exact owner mode is restored after all children are copied.
			return "", err
		}
	}
	var digest hash.Hash = sha256.New()
	var length [8]byte
	buffer := make([]byte, runtimeSourceCopySize)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		binary.BigEndian.PutUint64(length[:], uint64(len(entry.relative)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(entry.relative))
		binary.BigEndian.PutUint64(length[:], uint64(entry.mode.Perm()))
		_, _ = digest.Write(length[:])
		binary.BigEndian.PutUint64(length[:], uint64(entry.size)) // #nosec G115 -- inventory rejects negative sizes and caps each value at 32 MiB.
		_, _ = digest.Write(length[:])

		relative := filepath.FromSlash(entry.relative)
		beforePath, err := sourceRoot.Lstat(relative)
		if err != nil {
			return "", err
		}
		if !beforePath.Mode().IsRegular() || beforePath.Mode()&os.ModeSymlink != 0 || !os.SameFile(entry.info, beforePath) || beforePath.Size() != entry.size || beforePath.Mode().Perm() != entry.mode {
			return "", runtimeSourceInvalid(fmt.Sprintf("Runtime source file %s changed during snapshot; wait for edits to finish and retry.", runtimeSourcePathLabel(relative)))
		}
		source, err := sourceRoot.Open(relative) // #nosec G304 -- os.Root confines the canonical relative child below the managed source root.
		if err != nil {
			return "", err
		}
		opened, err := source.Stat()
		if err != nil {
			_ = source.Close()
			return "", err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(beforePath, opened) || opened.Size() != entry.size || opened.Mode().Perm() != entry.mode {
			_ = source.Close()
			return "", runtimeSourceInvalid(fmt.Sprintf("Runtime source file %s changed during snapshot; wait for edits to finish and retry.", runtimeSourcePathLabel(relative)))
		}

		target := filepath.Join(snapshot, relative)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			_ = source.Close()
			return "", err
		}
		destination, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, entry.mode) // #nosec G304 -- target is below a new private snapshot and derives from a validated canonical relative path.
		if err != nil {
			_ = source.Close()
			return "", err
		}
		copied, copyErr := io.CopyBuffer(io.MultiWriter(destination, digest), contextCheckedReader{ctx: ctx, reader: io.LimitReader(source, entry.size+1)}, buffer)
		closeDestinationErr := destination.Close()
		afterOpened, statErr := source.Stat()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeDestinationErr != nil {
			return "", closeDestinationErr
		}
		if statErr != nil {
			return "", statErr
		}
		if closeSourceErr != nil {
			return "", closeSourceErr
		}
		afterPath, err := sourceRoot.Lstat(relative)
		if err != nil {
			return "", err
		}
		if copied != entry.size || !afterPath.Mode().IsRegular() || afterPath.Mode()&os.ModeSymlink != 0 ||
			!os.SameFile(beforePath, afterOpened) || !os.SameFile(beforePath, afterPath) ||
			afterOpened.Size() != entry.size || afterPath.Size() != entry.size ||
			afterOpened.Mode().Perm() != entry.mode || afterPath.Mode().Perm() != entry.mode {
			return "", runtimeSourceInvalid(fmt.Sprintf("Runtime source file %s changed during snapshot; wait for edits to finish and retry.", runtimeSourcePathLabel(relative)))
		}
		if err := os.Chmod(target, entry.mode); err != nil { // #nosec G302 -- validated owner-only source mode is copied exactly.
			return "", err
		}
	}
	for index := len(directoryEntries) - 1; index >= 0; index-- {
		entry := directoryEntries[index]
		if err := os.Chmod(filepath.Join(snapshot, filepath.FromSlash(entry.relative)), entry.mode); err != nil { // #nosec G302 -- validated owner-only source mode is copied exactly.
			return "", err
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func digestRuntimeSnapshot(ctx context.Context, root string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := requirePrivateDirectory(root); err != nil {
		return "", err
	}
	entries := make([]runtimeSourceEntry, 0)
	directories := 0
	total := int64(0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || filepath.Clean(relative) != relative || filepath.IsAbs(relative) {
			return fmt.Errorf("Runtime snapshot contains a non-canonical child")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime snapshot contains unsafe ownership or link evidence")
		}
		if entry.IsDir() {
			directories++
			if directories > maxRuntimeSourceDirs+1 {
				return fmt.Errorf("Runtime snapshot directory inventory exceeds the bound")
			}
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxRuntimeSourceFile || len(entries) >= maxRuntimeSourceFiles {
			return fmt.Errorf("Runtime snapshot inventory is invalid")
		}
		total += info.Size()
		if total > maxRuntimeSourceTotal {
			return fmt.Errorf("Runtime snapshot exceeds the source bound")
		}
		entries = append(entries, runtimeSourceEntry{relative: filepath.ToSlash(relative), mode: info.Mode().Perm(), size: info.Size(), info: info})
		return nil
	}); err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("Runtime snapshot is empty")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relative < entries[j].relative })
	digest := sha256.New()
	var length [8]byte
	buffer := make([]byte, runtimeSourceCopySize)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		binary.BigEndian.PutUint64(length[:], uint64(len(entry.relative)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(entry.relative))
		binary.BigEndian.PutUint64(length[:], uint64(entry.mode.Perm()))
		_, _ = digest.Write(length[:])
		binary.BigEndian.PutUint64(length[:], uint64(entry.size)) // #nosec G115 -- bounded non-negative snapshot size.
		_, _ = digest.Write(length[:])
		path := filepath.Join(root, filepath.FromSlash(entry.relative))
		file, err := os.Open(path) // #nosec G304 -- exact canonical child from bounded private snapshot inventory.
		if err != nil {
			return "", err
		}
		opened, statErr := file.Stat()
		if statErr != nil || !opened.Mode().IsRegular() || !os.SameFile(entry.info, opened) || opened.Size() != entry.size || opened.Mode().Perm() != entry.mode {
			_ = file.Close()
			if statErr != nil {
				return "", statErr
			}
			return "", fmt.Errorf("Runtime snapshot changed during rehash")
		}
		copied, copyErr := io.CopyBuffer(digest, io.LimitReader(file, entry.size+1), buffer)
		closeErr := file.Close()
		after, afterErr := os.Lstat(path)
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if afterErr != nil {
			return "", afterErr
		}
		if copied != entry.size || !after.Mode().IsRegular() || !os.SameFile(entry.info, after) || after.Size() != entry.size || after.Mode().Perm() != entry.mode {
			return "", fmt.Errorf("Runtime snapshot changed during rehash")
		}
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func syncRuntimeSnapshotTree(root string) (resultErr error) {
	snapshotRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := snapshotRoot.Close(); resultErr == nil && closeErr != nil {
			resultErr = closeErr
		}
	}()
	directories := make([]string, 0)
	files := 0
	total := int64(0)
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || filepath.Clean(relative) != relative || filepath.IsAbs(relative) {
			return fmt.Errorf("Runtime snapshot contains a non-canonical child")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime snapshot contains a symbolic link")
		}
		if entry.IsDir() {
			directories = append(directories, relative)
			if len(directories) > maxRuntimeSourceDirs+1 {
				return fmt.Errorf("Runtime snapshot directory inventory exceeds the bound")
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Runtime snapshot contains a non-regular child")
		}
		files++
		total += info.Size()
		if files > maxRuntimeSourceFiles || info.Size() < 0 || info.Size() > maxRuntimeSourceFile || total > maxRuntimeSourceTotal {
			return fmt.Errorf("Runtime snapshot file inventory exceeds the bound")
		}
		file, err := snapshotRoot.Open(relative)
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		return file.Close()
	}); err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		directory, err := snapshotRoot.Open(directories[index])
		if err != nil {
			return err
		}
		if err := directory.Sync(); err != nil {
			_ = directory.Close()
			return err
		}
		if err := directory.Close(); err != nil {
			return err
		}
	}
	return nil
}

func freezeRuntimeSnapshot(root string) (resultErr error) {
	snapshotRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() {
		if snapshotRoot != nil {
			if closeErr := snapshotRoot.Close(); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}
	}()
	var directories []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime snapshot contains unsafe ownership or link evidence")
		}
		if entry.IsDir() {
			relative, relativeErr := filepath.Rel(root, path)
			if relativeErr != nil {
				return relativeErr
			}
			directories = append(directories, relative)
			return nil
		}
		relative, relativeErr := filepath.Rel(root, path)
		if relativeErr != nil {
			return relativeErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Runtime snapshot contains a non-regular child")
		}
		// Preserve the exact semantic source mode. Revision immutability is
		// enforced by lifecycle ownership; discarding execute/read bits here
		// would make an exact restore and recovery rehash impossible.
		if err := snapshotRoot.Chmod(relative, info.Mode().Perm()); err != nil { // #nosec G302 -- validated owner-only semantic source mode is retained.
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	for index := len(directories) - 1; index >= 0; index-- {
		if err := snapshotRoot.Chmod(directories[index], 0o700); err != nil { // #nosec G302 -- Runtime snapshot directories are intentionally owner-only.
			return err
		}
	}
	return nil
}

func removeRuntimeSnapshot(root string) (resultErr error) {
	parent := filepath.Dir(root)
	if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		if err := os.RemoveAll(parent); err != nil { // #nosec G301 -- exact empty Runtime build transaction directory.
			return err
		}
		return syncDirectoryIfPresent(filepath.Dir(parent))
	} else if err != nil {
		return err
	}
	snapshotRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer func() {
		if snapshotRoot != nil {
			if closeErr := snapshotRoot.Close(); resultErr == nil && closeErr != nil {
				resultErr = closeErr
			}
		}
	}()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		return snapshotRoot.Chmod(relative, 0o700) // #nosec G302 -- cleanup restores owner-only traversal before exact removal.
	}); err != nil {
		return err
	}
	if err := snapshotRoot.Close(); err != nil {
		return err
	}
	snapshotRoot = nil
	if err := os.RemoveAll(parent); err != nil { // #nosec G301 -- exact Runtime snapshot transaction directory.
		return err
	}
	if _, err := os.Lstat(parent); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("Runtime snapshot directory was not removed")
		}
		return err
	}
	return syncDirectoryIfPresent(filepath.Dir(parent))
}

func (r *Runtime) removeRuntimeBuildSnapshot(root string) error {
	if r.runtimeBuildSnapshotRemove != nil {
		return r.runtimeBuildSnapshotRemove(root)
	}
	return removeRuntimeSnapshot(root)
}

func (r *Runtime) syncRuntimeBuildSnapshot(root string) error {
	if r.runtimeBuildSnapshotSync != nil {
		return r.runtimeBuildSnapshotSync(root)
	}
	return syncRuntimeSnapshotTree(root)
}

func (r *Runtime) syncRuntimeBuildDirectory(path string) error {
	if r.runtimeBuildDirectorySync != nil {
		return r.runtimeBuildDirectorySync(path)
	}
	return syncDirectory(path)
}

func (r *Runtime) requireRuntimeBuildSnapshotRevision(ctx context.Context, root, expected string) error {
	if r.runtimeBuildRehashBoundary != nil {
		if err := r.runtimeBuildRehashBoundary(root, true); err != nil {
			return err
		}
	}
	var (
		observed string
		err      error
	)
	if r.runtimeBuildRehash != nil {
		observed, err = r.runtimeBuildRehash(ctx, root)
	} else {
		observed, err = digestRuntimeSnapshot(ctx, root)
	}
	if err != nil {
		return err
	}
	if observed != expected {
		return fmt.Errorf("Runtime build snapshot revision changed")
	}
	if r.runtimeBuildRehashBoundary != nil {
		if err := r.runtimeBuildRehashBoundary(root, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) renameRuntimeBuildSnapshot(source, target string) error {
	if r.runtimeBuildRename != nil {
		return r.runtimeBuildRename(source, target)
	}
	return os.Rename(source, target)
}

func (r *Runtime) freezeRuntimeBuildSnapshot(root string) error {
	if r.runtimeBuildFreeze != nil {
		return r.runtimeBuildFreeze(root)
	}
	return freezeRuntimeSnapshot(root)
}

func (r *Runtime) publishRuntimeBuildManifest(manifest tobari.RuntimeManifest) error {
	path := r.runtimeManifestPath(manifest.Name)
	var err error
	if r.runtimeBuildManifestWrite != nil {
		err = r.runtimeBuildManifestWrite(path, manifest)
	} else {
		err = writeAtomicJSON(path, manifest)
	}
	observed, observeErr := r.readRuntimeManifest(manifest.Name)
	if observeErr == nil && !reflect.DeepEqual(observed, manifest) {
		observeErr = fmt.Errorf("published Runtime manifest differs from exact build authority")
	}
	if err != nil || observeErr != nil {
		return errors.Join(err, observeErr)
	}
	return nil
}

func (r *Runtime) publishManagedRuntimeFinalTag(ctx context.Context, journal runtimeBuildJournal) error {
	publishErr := r.publishManagedRuntimeTag(ctx, journal)
	digest, observeErr := r.inspectManagedRuntimeBuildEvidence(ctx, journal.FinalImage, journal.RuntimeID, journal.Revision)
	if observeErr != nil || digest != journal.ImageDigest {
		if observeErr == nil {
			observeErr = fmt.Errorf("published Runtime image digest changed")
		}
		return errors.Join(publishErr, observeErr)
	}
	return nil
}

func (r *Runtime) releaseManagedRuntimeStagingTag(ctx context.Context, journal runtimeBuildJournal) error {
	digest, err := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
	if errors.Is(err, errManagedRuntimeImageMissing) {
		return nil
	}
	if err != nil || digest != journal.ImageDigest {
		return fmt.Errorf("Runtime staging tag authority changed before release: %w", err)
	}
	removeErr := r.runner.Run(ctx, []string{"image", "rm", journal.StagingImage}, os.Environ(), nil, io.Discard, io.Discard)
	_, observeErr := r.inspectManagedRuntimeBuildEvidence(ctx, journal.StagingImage, journal.RuntimeID, journal.Revision, journal.AttemptID)
	if errors.Is(observeErr, errManagedRuntimeImageMissing) {
		return nil
	}
	if observeErr == nil {
		observeErr = fmt.Errorf("Runtime staging tag remained after release")
	}
	return errors.Join(removeErr, observeErr)
}

func exactRuntimeBuildDirectory(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("Runtime build publication path is unsafe")
	}
	return true, nil
}

func (r *Runtime) publishManagedRuntimeSnapshot(ctx context.Context, journal runtimeBuildJournal) (string, error) {
	revisionsRoot := r.runtimeRevisionsDirectory(journal.RuntimeName)
	finalParent := filepath.Join(revisionsRoot, strings.TrimPrefix(journal.Revision, "sha256:"))
	final := filepath.Join(finalParent, "source")
	stagingPresent, err := exactRuntimeBuildDirectory(journal.SnapshotPath)
	if err != nil {
		return "", err
	}
	finalPresent, err := exactRuntimeBuildDirectory(final)
	if err != nil {
		return "", err
	}
	if stagingPresent && finalPresent {
		return "", fmt.Errorf("Runtime build snapshot exists at both staging and final authority")
	}
	if !stagingPresent && !finalPresent {
		return "", fmt.Errorf("Runtime build snapshot publication outcome is unknown")
	}
	if stagingPresent {
		if err := r.requireRuntimeBuildSnapshotRevision(ctx, journal.SnapshotPath, journal.Revision); err != nil {
			return "", err
		}
		parentPresent, err := exactRuntimeBuildDirectory(finalParent)
		if err != nil {
			return "", err
		}
		if !parentPresent {
			if err := os.Mkdir(finalParent, 0o700); err != nil {
				return "", err
			}
		} else {
			entries, err := os.ReadDir(finalParent)
			if err != nil || len(entries) != 0 {
				return "", fmt.Errorf("Runtime revision publication directory is not empty: %w", err)
			}
		}
		if err := r.syncRuntimeBuildDirectory(revisionsRoot); err != nil {
			return "", err
		}
		renameErr := r.renameRuntimeBuildSnapshot(journal.SnapshotPath, final)
		stagingPresent, stagingErr := exactRuntimeBuildDirectory(journal.SnapshotPath)
		finalObserved, finalErr := exactRuntimeBuildDirectory(final)
		if stagingErr != nil || finalErr != nil || stagingPresent || !finalObserved {
			return "", errors.Join(renameErr, stagingErr, finalErr)
		}
	}
	if err := r.freezeRuntimeBuildSnapshot(final); err != nil {
		return "", err
	}
	if err := r.syncRuntimeBuildSnapshot(final); err != nil {
		return "", err
	}
	if err := r.syncRuntimeBuildDirectory(finalParent); err != nil {
		return "", err
	}
	if err := r.syncRuntimeBuildDirectory(revisionsRoot); err != nil {
		return "", err
	}
	if err := r.requireRuntimeBuildSnapshotRevision(ctx, final, journal.Revision); err != nil {
		return "", err
	}
	return final, nil
}

func (r *Runtime) publishManagedRuntimeManifestRevision(_ context.Context, journal runtimeBuildJournal) (tobari.RuntimeManifest, error) {
	manifest, err := r.readRuntimeManifest(journal.RuntimeName)
	if err != nil || manifest.ID != journal.RuntimeID {
		return tobari.RuntimeManifest{}, fmt.Errorf("Runtime manifest authority changed during build publication: %w", err)
	}
	final := filepath.Join(r.runtimeRevisionsDirectory(journal.RuntimeName), strings.TrimPrefix(journal.Revision, "sha256:"), "source")
	createdAt, err := time.Parse(time.RFC3339Nano, journal.CreatedAt)
	if err != nil {
		return tobari.RuntimeManifest{}, err
	}
	expected := tobari.RuntimeRevision{Ordinal: len(manifest.Revisions) + 1, Revision: journal.Revision, Image: journal.FinalImage, ImageDigest: journal.ImageDigest, CreatedAt: createdAt, SnapshotPath: final}
	found := false
	for _, revision := range manifest.Revisions {
		if revision.Revision != journal.Revision {
			continue
		}
		found = true
		expected.Ordinal = revision.Ordinal
		if !reflect.DeepEqual(revision, expected) {
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime manifest revision publication authority changed")
		}
	}
	if !found {
		manifest.Revisions = append(manifest.Revisions, expected)
		if err := manifest.Validate(); err != nil {
			return tobari.RuntimeManifest{}, err
		}
		if err := r.publishRuntimeBuildManifest(manifest); err != nil {
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build manifest publication requires reconciliation: %w", err)
		}
	}
	observed, err := r.readRuntimeManifest(journal.RuntimeName)
	if err != nil || !reflect.DeepEqual(observed, manifest) {
		return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build manifest observation changed: %w", err)
	}
	return observed, nil
}

func (r *Runtime) resumeManagedRuntimePublicationLocked(ctx context.Context, journal runtimeBuildJournal) (tobari.RuntimeManifest, error) {
	for {
		switch journal.Phase {
		case runtimeBuildPhaseBuilt:
			if err := r.publishManagedRuntimeFinalTag(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			next := journal
			next.Phase = runtimeBuildPhaseFinalTagged
			if err := r.writeRuntimeBuildJournal(journal, next); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			journal = next
		case runtimeBuildPhaseFinalTagged:
			if err := r.publishManagedRuntimeFinalTag(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			if err := r.releaseManagedRuntimeStagingTag(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			next := journal
			next.Phase = runtimeBuildPhaseStagingReleased
			if err := r.writeRuntimeBuildJournal(journal, next); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			journal = next
		case runtimeBuildPhaseStagingReleased:
			if err := r.publishManagedRuntimeFinalTag(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build publication requires reconciliation: %w", err)
			}
			if _, err := r.publishManagedRuntimeSnapshot(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build publication requires reconciliation: %w", err)
			}
			next := journal
			next.Phase = runtimeBuildPhaseSnapshotPublished
			if err := r.writeRuntimeBuildJournal(journal, next); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			journal = next
		case runtimeBuildPhaseSnapshotPublished:
			if _, err := r.publishManagedRuntimeSnapshot(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build publication requires reconciliation: %w", err)
			}
			manifest, err := r.publishManagedRuntimeManifestRevision(ctx, journal)
			if err != nil {
				return tobari.RuntimeManifest{}, err
			}
			next := journal
			next.Phase = runtimeBuildPhaseManifestCommitted
			if err := r.writeRuntimeBuildJournal(journal, next); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			journal = next
			_ = manifest
		case runtimeBuildPhaseManifestCommitted:
			if err := r.validateCompletedRuntimeBuildAuthority(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			manifest, err := r.readRuntimeManifest(journal.RuntimeName)
			if err != nil {
				return tobari.RuntimeManifest{}, err
			}
			if err := r.completeRuntimeBuildJournal(ctx, journal); err != nil {
				return tobari.RuntimeManifest{}, err
			}
			return manifest, nil
		default:
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime build journal is not resumable publication authority")
		}
	}
}

func runtimeSourceUsesRefreshableBase(dockerfile, defaultImage string, refresh bool) (bool, error) {
	data, err := os.ReadFile(dockerfile) // #nosec G304 -- caller supplies a validated immutable snapshot child.
	if err != nil {
		return false, fmt.Errorf("read Runtime Dockerfile: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "FROM" {
			continue
		}
		for _, field := range fields[1:] {
			if strings.EqualFold(field, "AS") {
				break
			}
			if strings.HasPrefix(field, "--") {
				continue
			}
			return field == defaultImage && refresh, nil
		}
	}
	return false, nil
}

func managedLibraryRuntimeImage(name, runtimeID, revision string) string {
	return "tobari-runtime-" + name + "-" + runtimeID + ":" + strings.TrimPrefix(revision, "sha256:")
}

// BuildManagedRuntimeByReference holds the installation lifecycle lock across
// exact reference re-resolution and complete build publication. A same-name
// Runtime created after retirement cannot receive authority from the old ID.
func (r *Runtime) BuildManagedRuntimeByReference(ctx context.Context, reference string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	if err := tobari.ValidateRuntimeRef(reference); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if reference == tobari.StandardRuntimeID {
		return tobari.RuntimeReport{}, fmt.Errorf("built-in Runtime cannot be built")
	}
	var result tobari.RuntimeReport
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		var buildErr error
		result, buildErr = r.buildManagedRuntimeLifecycleLocked(lockContext, "", reference, diagnostics)
		return buildErr
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	return result, nil
}

// BuildManagedRuntime snapshots, builds, validates, and appends one immutable
// successful revision. It never writes a Context.
func (r *Runtime) BuildManagedRuntime(ctx context.Context, name string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	var result tobari.RuntimeReport
	err := r.WithLifecycleLock(ctx, func(lockContext context.Context) error {
		var buildErr error
		result, buildErr = r.buildManagedRuntimeLifecycleLocked(lockContext, name, "", diagnostics)
		return buildErr
	})
	return result, err
}

// buildManagedRuntimeLifecycleLocked requires the installation lifecycle lock.
// Migration uses it under its broader migration lock; public entry points
// acquire that lock exactly once before crossing this boundary.
func (r *Runtime) buildManagedRuntimeLifecycleLocked(ctx context.Context, name, expectedReference string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if name == tobari.StandardRuntimeName {
		return tobari.RuntimeReport{}, fmt.Errorf("built-in Runtime cannot be built")
	}
	var result tobari.RuntimeReport
	err := r.withRuntimeStoreLock(ctx, func() error {
		var manifest tobari.RuntimeManifest
		var err error
		if expectedReference != "" {
			manifest, err = r.resolveManagedRuntimeReferenceUnlocked(expectedReference)
			name = manifest.Name
		} else {
			manifest, err = r.readRuntimeManifest(name)
		}
		if err != nil {
			return err
		}
		revisionsRoot := r.runtimeRevisionsDirectory(name)
		if err := requirePrivateDirectory(revisionsRoot); err != nil {
			return err
		}
		journal, err := r.beginRuntimeBuildJournal(ctx, manifest.ID, manifest.Name)
		if err != nil {
			return err
		}
		if err := os.Mkdir(filepath.Dir(journal.SnapshotPath), 0o700); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		snapshot := journal.SnapshotPath
		revision, err := copyRuntimeSource(ctx, r.runtimeSourceDirectory(name), snapshot)
		if err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		if err := r.syncRuntimeBuildSnapshot(snapshot); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		if err := r.syncRuntimeBuildDirectory(filepath.Dir(snapshot)); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		if err := r.syncRuntimeBuildDirectory(r.runtimeLifecycleDirectory()); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		if err := r.requireRuntimeBuildSnapshotRevision(ctx, snapshot, revision); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		prepared := journal
		prepared.Revision = revision
		prepared.StagingImage = managedRuntimeStagingImage(manifest.ID, revision)
		prepared.FinalImage = managedLibraryRuntimeImage(name, manifest.ID, revision)
		prepared.Phase = runtimeBuildPhasePrepared
		if err := r.writeRuntimeBuildJournal(journal, prepared); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal, prepared)
		}
		journal = prepared
		orphanDisposition, orphanDigest, err := r.observeUnusedRuntimeStagingTag(ctx, journal)
		if err != nil {
			orphan := journal
			orphan.Phase = runtimeBuildPhaseOrphanStaging
			orphan.OrphanStaging = orphanDisposition
			orphan.ImageDigest = orphanDigest
			if journalErr := r.writeRuntimeBuildJournal(journal, orphan); journalErr != nil {
				return fmt.Errorf("Runtime staging conflict requires reconciliation: %w", errors.Join(err, journalErr))
			}
			return err
		}
		for _, existing := range manifest.Revisions {
			if existing.Revision == revision {
				observedDigest, inspectErr := r.inspectManagedRuntimeBuildEvidence(ctx, existing.Image, manifest.ID, revision)
				if inspectErr != nil || observedDigest != existing.ImageDigest {
					return r.rollbackRuntimeBuildBeforeDocker(ctx, tobari.ErrRuntimeNotReady, journal)
				}
				result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, NoChange: true}
				if err := result.Validate(); err != nil {
					return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
				}
				return r.completeRuntimeBuildJournal(ctx, journal)
			}
		}
		dockerfile := filepath.Join(snapshot, "Dockerfile")
		if info, err := os.Lstat(dockerfile); err != nil || !info.Mode().IsRegular() {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, runtimeSourceInvalid("Runtime source requires a regular file named \"Dockerfile\" at its root."), journal)
		}
		image := journal.StagingImage
		pullBase, err := runtimeSourceUsesRefreshableBase(dockerfile, r.defaultRuntimeImage(), r.imageResolver().ShouldPullRuntimeImage(r.defaultRuntimeImage()))
		if err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		args := []string{"buildx", "build", "--progress=plain", "--load",
			"--label", ownerLabel + "=" + ownerValue,
			"--label", componentLabel + "=" + managedRuntimeComponentLabel,
			"--label", managedRuntimeIDLabel + "=" + manifest.ID,
			"--label", managedRuntimeRevisionLabel + "=" + revision,
			"--label", managedRuntimeBuildAttemptLabel + "=" + journal.AttemptID,
		}
		if pullBase {
			args = append(args, "--pull")
		}
		args = append(args, "--tag", image, "--file", dockerfile, snapshot)
		building := journal
		building.Phase = runtimeBuildPhaseBuilding
		building.StagingArtifact = runtimeBuildStagingUnknown
		building.AttemptSettlement = runtimeBuildAttemptUnsettled
		if err := r.writeRuntimeBuildJournal(journal, building); err != nil {
			return r.rollbackRuntimeBuildBeforeDocker(ctx, err, journal)
		}
		journal = building
		var tail runtimeBuildDiagnosticTail
		stream := io.MultiWriter(&bestEffortDiagnosticWriter{writer: diagnostics}, &tail)
		if err := r.runner.Run(ctx, args, os.Environ(), nil, stream, stream); err != nil {
			return r.retainRuntimeBuildFailure(ctx, journal, fmt.Errorf("build Runtime: %w: %s", err, boundedDiagnostic(tail.Bytes())))
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return r.retainRuntimeBuildFailure(ctx, journal, err)
		}
		imageDigest, err := r.inspectManagedRuntimeBuildEvidence(ctx, image, manifest.ID, revision, journal.AttemptID)
		if err != nil {
			return r.retainRuntimeBuildFailure(ctx, journal, err)
		}
		if err := r.freezeRuntimeBuildSnapshot(snapshot); err != nil {
			return r.retainRuntimeBuildFailure(ctx, journal, fmt.Errorf("freeze Runtime build snapshot before publication: %w", err))
		}
		if err := r.syncRuntimeBuildSnapshot(snapshot); err != nil {
			return r.retainRuntimeBuildFailure(ctx, journal, fmt.Errorf("sync Runtime build snapshot before publication: %w", err))
		}
		if err := r.requireRuntimeBuildSnapshotRevision(ctx, snapshot, revision); err != nil {
			failed := journal
			failed.Phase = runtimeBuildPhaseFailed
			failed.ImageDigest = imageDigest
			failed.StagingArtifact = runtimeBuildStagingOwned
			failed.AttemptSettlement = runtimeBuildAttemptUnsettled
			if journalErr := r.writeRuntimeBuildJournal(journal, failed); journalErr != nil {
				return fmt.Errorf("Runtime build snapshot drift requires reconciliation: %w", errors.Join(err, journalErr))
			}
			return fmt.Errorf("Runtime build snapshot drifted before image publication: %w", err)
		}
		built := journal
		built.ImageDigest = imageDigest
		built.Phase = runtimeBuildPhaseBuilt
		built.StagingArtifact = runtimeBuildStagingOwned
		built.AttemptSettlement = runtimeBuildAttemptSettled
		createdAt := time.Now().UTC()
		if r.identities.now != nil {
			createdAt = r.identities.now().UTC()
		}
		built.CreatedAt = createdAt.Format(time.RFC3339Nano)
		if err := r.writeRuntimeBuildJournal(journal, built); err != nil {
			return err
		}
		published, err := r.resumeManagedRuntimePublicationLocked(ctx, built)
		if err != nil {
			return err
		}
		result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: published, Built: true}
		if err := result.Validate(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	return result, nil
}

func (r *Runtime) resolveManagedRuntimeReferenceUnlocked(reference string) (tobari.RuntimeManifest, error) {
	entries, err := os.ReadDir(r.runtimesDirectory())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
	}
	if err != nil {
		return tobari.RuntimeManifest{}, err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			return tobari.RuntimeManifest{}, fmt.Errorf("Runtime catalog contains an unsafe entry")
		}
		manifest, err := r.readRuntimeManifest(entry.Name())
		if err != nil {
			return tobari.RuntimeManifest{}, err
		}
		if tobari.RuntimeRef(manifest.ID) == reference {
			return manifest, nil
		}
	}
	return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
}
