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
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	runtimeSourceFileMode = 0o600
	maxRuntimeSourceFiles = 1024
	maxRuntimeSourceDirs  = 256
	maxRuntimeSourceFile  = 2 * 1024 * 1024
	maxRuntimeSourceTotal = 16 * 1024 * 1024
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
func (r *Runtime) SetContextRuntime(ctx context.Context, contextName, selection string) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	binding, err := r.resolveRuntimeBinding(selection)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err = r.withContextStoreLock(func() error {
		active, err := r.readActiveContext()
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
		manifest.Runtime = nil
		copy := binding
		manifest.RuntimeBinding = &copy
		manifest.Image = binding.Image
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context Runtime binding: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskContextRuntimeSet, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
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

// CreateRuntime initializes one managed source tree without building it.
func (r *Runtime) CreateRuntime(ctx context.Context, name string) (tobari.RuntimeReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.RuntimeReport{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
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
		committed := false
		defer func() {
			if !committed {
				_ = os.RemoveAll(r.runtimeDirectory(name))
			}
		}()
		if err := r.ensurePrivateDirectory(r.runtimeDirectory(name)); err != nil {
			return err
		}
		if err := r.ensurePrivateDirectory(r.runtimeSourceDirectory(name)); err != nil {
			return err
		}
		if err := r.ensurePrivateDirectory(r.runtimeRevisionsDirectory(name)); err != nil {
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
		if err := initializeBytes(filepath.Join(manifest.SourcePath, "Dockerfile"), []byte(fmt.Sprintf(managedRuntimeTemplate, r.defaultRuntimeImage())), runtimeSourceFileMode); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.runtimeManifestPath(name), manifest); err != nil {
			return err
		}
		committed = true
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
	data     []byte
}

func (r *Runtime) readRuntimeSource(name string) ([]runtimeSourceEntry, string, error) {
	root := r.runtimeSourceDirectory(name)
	if err := requirePrivateDirectory(root); err != nil {
		return nil, "", fmt.Errorf("inspect Runtime source: %w", err)
	}
	sourceRoot, err := os.OpenRoot(root)
	if err != nil {
		return nil, "", fmt.Errorf("open Runtime source: %w", err)
	}
	defer sourceRoot.Close()
	entries := make([]runtimeSourceEntry, 0)
	directories := 0
	total := int64(0)
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == "." || filepath.Clean(relative) != relative || filepath.IsAbs(relative) {
			return fmt.Errorf("Runtime source path is invalid")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Runtime source contains a symbolic link: %s", relative)
		}
		if entry.IsDir() {
			directories++
			if directories > maxRuntimeSourceDirs {
				return fmt.Errorf("Runtime source contains too many directories")
			}
			if info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("Runtime source directory must be owner-only: %s", relative)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Runtime source contains a special file: %s", relative)
		}
		if info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("Runtime source file must be owner-only: %s", relative)
		}
		if info.Size() < 0 || info.Size() > maxRuntimeSourceFile {
			return fmt.Errorf("Runtime source file is too large: %s", relative)
		}
		if len(entries) >= maxRuntimeSourceFiles {
			return fmt.Errorf("Runtime source contains too many files")
		}
		total += info.Size()
		if total > maxRuntimeSourceTotal {
			return fmt.Errorf("Runtime source is too large")
		}
		file, err := sourceRoot.Open(relative) // #nosec G304 -- relative is below the opened managed source root and revalidated after bounded read.
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxRuntimeSourceFile+1))
		closeErr := file.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) > maxRuntimeSourceFile {
			return fmt.Errorf("Runtime source file is too large: %s", relative)
		}
		after, err := sourceRoot.Lstat(relative)
		if err != nil {
			return err
		}
		if !after.Mode().IsRegular() || after.Mode()&os.ModeSymlink != 0 || after.Size() != int64(len(data)) || after.Mode().Perm() != info.Mode().Perm() {
			return fmt.Errorf("Runtime source changed during snapshot: %s", relative)
		}
		entries = append(entries, runtimeSourceEntry{relative: filepath.ToSlash(relative), mode: info.Mode().Perm(), data: data})
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	if len(entries) == 0 {
		return nil, "", fmt.Errorf("Runtime source is empty")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].relative < entries[j].relative })
	var digest hash.Hash = sha256.New()
	var length [8]byte
	for _, entry := range entries {
		binary.BigEndian.PutUint64(length[:], uint64(len(entry.relative)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write([]byte(entry.relative))
		binary.BigEndian.PutUint64(length[:], uint64(entry.mode.Perm()))
		_, _ = digest.Write(length[:])
		binary.BigEndian.PutUint64(length[:], uint64(len(entry.data)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(entry.data)
	}
	return entries, "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func writeRuntimeSnapshot(root string, entries []runtimeSourceEntry) error {
	if err := os.Mkdir(root, 0o700); err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(root, filepath.FromSlash(entry.relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := initializeBytes(path, entry.data, entry.mode); err != nil {
			return err
		}
	}
	return nil
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
		entries, revision, err := r.readRuntimeSource(name)
		if err != nil {
			return err
		}
		for _, existing := range manifest.Revisions {
			if existing.Revision == revision {
				result = tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, NoChange: true}
				return result.Validate()
			}
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
		if err := writeRuntimeSnapshot(snapshot, entries); err != nil {
			return err
		}
		dockerfile := filepath.Join(snapshot, "Dockerfile")
		if info, err := os.Lstat(dockerfile); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("Runtime source requires a regular Dockerfile")
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
