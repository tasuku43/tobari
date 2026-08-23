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

func (r *Runtime) withRuntimeStoreLock(action func() error) error {
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
	defer file.Close()
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return lockErr
		}
		if acquired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer unlockProjectFile(file)
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
	err := r.withRuntimeStoreLock(func() error {
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

func freezeRuntimeSnapshot(root string) error {
	snapshotRoot, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer snapshotRoot.Close()
	var directories []string
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
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
		if err := snapshotRoot.Chmod(relative, 0o400); err != nil { // #nosec G302 -- immutable Runtime snapshots are intentionally owner-readable.
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

func removeRuntimeSnapshot(root string) {
	snapshotRoot, err := os.OpenRoot(root)
	if err == nil {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr == nil && entry.IsDir() {
				relative, relativeErr := filepath.Rel(root, path)
				if relativeErr == nil {
					_ = snapshotRoot.Chmod(relative, 0o700) // #nosec G302 -- cleanup restores owner-only traversal before removal.
				}
			}
			return nil
		})
		_ = snapshotRoot.Close()
	}
	_ = os.RemoveAll(filepath.Dir(root))
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

func managedLibraryRuntimeImage(name, revision string) string {
	short := strings.TrimPrefix(revision, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	return "tobari-runtime-" + name + ":" + short
}

// BuildManagedRuntime snapshots, builds, validates, and appends one immutable
// successful revision. It never writes a Context.
func (r *Runtime) BuildManagedRuntime(ctx context.Context, name string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if name == tobari.StandardRuntimeName {
		return tobari.RuntimeReport{}, fmt.Errorf("built-in Runtime cannot be built")
	}
	var result tobari.RuntimeReport
	err := r.withRuntimeStoreLock(func() error {
		manifest, err := r.readRuntimeManifest(name)
		if err != nil {
			return err
		}
		revisionsRoot := r.runtimeRevisionsDirectory(name)
		if err := requirePrivateDirectory(revisionsRoot); err != nil {
			return err
		}
		temporary, err := os.MkdirTemp(revisionsRoot, ".snapshot-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(temporary)
		snapshot := filepath.Join(temporary, "source")
		revision, err := copyRuntimeSource(ctx, r.runtimeSourceDirectory(name), snapshot)
		if err != nil {
			return err
		}
		for _, existing := range manifest.Revisions {
			if existing.Revision == revision {
				result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, NoChange: true}
				return result.Validate()
			}
		}
		dockerfile := filepath.Join(snapshot, "Dockerfile")
		if info, err := os.Lstat(dockerfile); err != nil || !info.Mode().IsRegular() {
			return runtimeSourceInvalid("Runtime source requires a regular file named \"Dockerfile\" at its root.")
		}
		image := managedLibraryRuntimeImage(name, revision)
		pullBase, err := runtimeSourceUsesRefreshableBase(dockerfile, r.defaultRuntimeImage(), r.imageResolver().ShouldPullRuntimeImage(r.defaultRuntimeImage()))
		if err != nil {
			return err
		}
		args := []string{"buildx", "build", "--progress=plain", "--load"}
		if pullBase {
			args = append(args, "--pull")
		}
		args = append(args, "--tag", image, "--file", dockerfile, snapshot)
		var tail runtimeBuildDiagnosticTail
		stream := io.MultiWriter(&bestEffortDiagnosticWriter{writer: diagnostics}, &tail)
		if err := r.runner.Run(ctx, args, os.Environ(), nil, stream, stream); err != nil {
			return fmt.Errorf("build Runtime: %w: %s", err, boundedDiagnostic(tail.Bytes()))
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return err
		}
		imageDigest, err := r.inspectImageDigest(ctx, image)
		if err != nil {
			return err
		}
		final := filepath.Join(revisionsRoot, strings.TrimPrefix(revision, "sha256:"), "source")
		if _, err := os.Lstat(filepath.Dir(final)); err == nil {
			return fmt.Errorf("Runtime snapshot already exists without history authority")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(filepath.Dir(final), 0o700); err != nil {
			return err
		}
		if err := os.Rename(snapshot, final); err != nil {
			return err
		}
		if err := freezeRuntimeSnapshot(final); err != nil {
			removeRuntimeSnapshot(final)
			return err
		}
		createdAt := time.Now().UTC()
		if r.identities.now != nil {
			createdAt = r.identities.now().UTC()
		}
		manifest.Revisions = append(manifest.Revisions, tobari.RuntimeRevision{
			Ordinal: len(manifest.Revisions) + 1, Revision: revision, Image: image, ImageDigest: imageDigest,
			CreatedAt: createdAt, SnapshotPath: final,
		})
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.runtimeManifestPath(name), manifest); err != nil {
			removeRuntimeSnapshot(final)
			return err
		}
		result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, Built: true}
		return result.Validate()
	})
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	return result, nil
}
