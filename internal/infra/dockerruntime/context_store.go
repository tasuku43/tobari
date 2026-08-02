package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxContextManifestBytes = 16 * 1024
	maxContextStoreFileSize = 8 * 1024 * 1024
)

type activeContextDocument struct {
	Name string `json:"name"`
}

func (r *Runtime) contextsDirectory() string {
	return filepath.Join(r.configDirectory, "contexts")
}

func (r *Runtime) contextDirectory(name string) string {
	return filepath.Join(r.contextsDirectory(), name)
}

func (r *Runtime) contextManifestPath(name string) string {
	return filepath.Join(r.contextDirectory(name), "context.json")
}

func (r *Runtime) contextPolicyDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "policy")
}

func (r *Runtime) contextCredentialConfig(name string) string {
	return filepath.Join(r.contextDirectory(name), "credentials.json")
}

func (r *Runtime) contextCredentialDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "credentials")
}

func (r *Runtime) activeContextPath() string {
	return filepath.Join(r.contextsDirectory(), "active.json")
}

func (r *Runtime) contextPaths(name string) tobari.ContextStorePaths {
	return tobari.ContextStorePaths{
		PolicyDirectory:     r.contextPolicyDirectory(name),
		CredentialConfig:    r.contextCredentialConfig(name),
		CredentialDirectory: r.contextCredentialDirectory(name),
	}
}

func (r *Runtime) legacyContextStorePaths() tobari.ContextStorePaths {
	return tobari.ContextStorePaths{
		PolicyDirectory:     filepath.Join(r.configDirectory, "policy"),
		CredentialConfig:    filepath.Join(r.configDirectory, "credentials.json"),
		CredentialDirectory: filepath.Join(r.configDirectory, "credentials"),
	}
}

// diagnosticContextStores resolves the stores to inspect without initializing
// the Context catalog. Doctor is observational, so a missing active marker
// deliberately keeps the legacy paths until cluster up performs migration.
func (r *Runtime) diagnosticContextStores() (tobari.ContextStorePaths, error) {
	if _, err := os.Lstat(r.activeContextPath()); errors.Is(err, os.ErrNotExist) {
		return r.legacyContextStorePaths(), nil
	} else if err != nil {
		return tobari.ContextStorePaths{}, fmt.Errorf("inspect active Context: %w", err)
	}
	name, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextStorePaths{}, err
	}
	paths := r.contextPaths(name)
	if err := paths.Validate(); err != nil {
		return tobari.ContextStorePaths{}, err
	}
	return paths, nil
}

func (r *Runtime) ensureContextStore() error {
	if err := r.ensurePrivateDirectory(r.contextsDirectory()); err != nil {
		return fmt.Errorf("prepare Context directory: %w", err)
	}
	image := tobari.BuiltinImageSelector
	if _, err := os.Lstat(r.contextManifestPath(tobari.DefaultContextName)); errors.Is(err, os.ErrNotExist) {
		if configured, err := r.configuredDefaultImage(); err != nil {
			return err
		} else {
			image = configured
		}
	} else if err != nil {
		return fmt.Errorf("inspect default Context manifest: %w", err)
	}
	if err := r.ensureContext(tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion,
		Name:          tobari.DefaultContextName,
		AgentProfile:  tobari.DefaultProfile,
		Image:         image,
		PolicyMode:    tobari.ContextPolicyModeGuided,
	}, true); err != nil {
		return err
	}
	if _, err := os.Lstat(r.activeContextPath()); errors.Is(err, os.ErrNotExist) {
		if err := r.writeActiveContext(tobari.DefaultContextName); err != nil {
			return fmt.Errorf("initialize active Context: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect active Context: %w", err)
	} else if _, err := r.readActiveContext(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) ensureContext(manifest tobari.ContextManifest, migrateLegacy bool) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	directory := r.contextDirectory(manifest.Name)
	if err := r.ensurePrivateDirectory(directory); err != nil {
		return fmt.Errorf("prepare Context %q: %w", manifest.Name, err)
	}
	if err := r.ensurePrivateDirectory(r.contextPolicyDirectory(manifest.Name)); err != nil {
		return fmt.Errorf("prepare Context %q policy: %w", manifest.Name, err)
	}
	if err := r.ensurePrivateDirectory(r.contextCredentialDirectory(manifest.Name)); err != nil {
		return fmt.Errorf("prepare Context %q credentials: %w", manifest.Name, err)
	}
	if migrateLegacy && manifest.Name == tobari.DefaultContextName {
		if err := r.migrateLegacyDefaultStores(); err != nil {
			return err
		}
	}
	for _, name := range []string{"data.json", "tobari.rego", "tobari_test.rego"} {
		if err := initializeFile(filepath.Join(r.contextPolicyDirectory(manifest.Name), name), "opa/policy/"+name, 0o600); err != nil {
			return err
		}
	}
	if err := initializeBytes(
		r.contextCredentialConfig(manifest.Name),
		[]byte("{\n  \"version\": \"v1\",\n  \"profiles\": {}\n}\n"), 0o600,
	); err != nil {
		return err
	}
	if _, err := os.Lstat(r.contextManifestPath(manifest.Name)); errors.Is(err, os.ErrNotExist) {
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context manifest: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect Context manifest: %w", err)
	}
	return nil
}

func (r *Runtime) migrateLegacyDefaultStores() error {
	legacyPolicy := filepath.Join(r.configDirectory, "policy")
	if err := r.copyStoreFilesIfPresent(legacyPolicy, r.contextPolicyDirectory(tobari.DefaultContextName)); err != nil {
		return fmt.Errorf("migrate legacy policy: %w", err)
	}
	legacyCredentials := filepath.Join(r.configDirectory, "credentials")
	if err := r.copyStoreFilesIfPresent(legacyCredentials, r.contextCredentialDirectory(tobari.DefaultContextName)); err != nil {
		return fmt.Errorf("migrate legacy credentials: %w", err)
	}
	legacyConfig := filepath.Join(r.configDirectory, "credentials.json")
	if err := r.copyFileIfPresent(legacyConfig, r.contextCredentialConfig(tobari.DefaultContextName)); err != nil {
		return fmt.Errorf("migrate legacy credential metadata: %w", err)
	}
	return nil
}

func (r *Runtime) copyStoreFilesIfPresent(source, destination string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("legacy store must be an owner-only directory")
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("legacy store contains an unsafe entry")
		}
		if err := r.copyFileIfPresent(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) copyFileIfPresent(source, destination string) error {
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextStoreFileSize {
		return fmt.Errorf("legacy file %s is unsafe", filepath.Base(source))
	}
	if _, err := os.Lstat(destination); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(source) // #nosec G304 -- source is an exact legacy store child.
	if err != nil {
		return err
	}
	return initializeBytes(destination, data, 0o600)
}

func (r *Runtime) readContextManifest(name string) (tobari.ContextManifest, error) {
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ContextManifest{}, err
	}
	path := r.contextManifestPath(name)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tobari.ContextManifest{}, fmt.Errorf("%w: %s", tobari.ErrContextNotFound, name)
		}
		return tobari.ContextManifest{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextManifestBytes {
		return tobari.ContextManifest{}, fmt.Errorf("Context manifest is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- name is validated and path is a Context child.
	if err != nil {
		return tobari.ContextManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest tobari.ContextManifest
	if err := decoder.Decode(&manifest); err != nil {
		return tobari.ContextManifest{}, fmt.Errorf("decode Context manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.ContextManifest{}, fmt.Errorf("Context manifest contains trailing data")
	}
	// Context manifests written before runtime image selection was promoted
	// into the bundle inherit the safe built-in image.
	if manifest.Image == "" {
		manifest.Image = tobari.BuiltinImageSelector
	}
	if err := manifest.Validate(); err != nil {
		return tobari.ContextManifest{}, err
	}
	if manifest.Name != name {
		return tobari.ContextManifest{}, fmt.Errorf("Context manifest name does not match its path")
	}
	return manifest, nil
}

func (r *Runtime) readActiveContext() (string, error) {
	info, err := os.Lstat(r.activeContextPath())
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextManifestBytes {
		return "", fmt.Errorf("active Context marker is unsafe")
	}
	data, err := os.ReadFile(r.activeContextPath()) // #nosec G304 -- fixed active Context path.
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document activeContextDocument
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("decode active Context: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("active Context contains trailing data")
	}
	if err := tobari.ValidateName(document.Name); err != nil {
		return "", err
	}
	if _, err := r.readContextManifest(document.Name); err != nil {
		return "", fmt.Errorf("active Context is unavailable: %w", err)
	}
	return document.Name, nil
}

func (r *Runtime) writeActiveContext(name string) error {
	if err := tobari.ValidateName(name); err != nil {
		return err
	}
	if _, err := r.readContextManifest(name); err != nil {
		return err
	}
	return writeAtomicJSON(r.activeContextPath(), activeContextDocument{Name: name})
}

func (r *Runtime) activeContext() (tobari.ContextManifest, tobari.ContextStorePaths, error) {
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	name, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	paths := r.contextPaths(name)
	if err := paths.Validate(); err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	return manifest, paths, nil
}

// ListContexts returns the complete host-owned Context collection.
func (r *Runtime) ListContexts(ctx context.Context) (tobari.ContextListResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextListResult{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextListResult{}, err
	}
	active, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextListResult{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return tobari.ContextListResult{}, err
	}
	items := make([]tobari.ContextSummary, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return tobari.ContextListResult{}, fmt.Errorf("Context directory contains a symbolic link")
		}
		manifest, err := r.readContextManifest(entry.Name())
		if err != nil {
			return tobari.ContextListResult{}, err
		}
		items = append(items, tobari.ContextSummary{
			Name: manifest.Name, Active: manifest.Name == active,
			AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	result := tobari.ContextListResult{Task: tobari.TaskContextList, Active: active, Items: items}
	if err := result.Validate(); err != nil {
		return tobari.ContextListResult{}, err
	}
	return result, nil
}

// ShowContext returns one named Context, or the active Context when name is empty.
func (r *Runtime) ShowContext(ctx context.Context, name string) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	active, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextReport{}, err
	}
	if name == "" {
		name = active
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	result := tobari.ContextReport{
		Task: tobari.TaskContextShow, Name: manifest.Name, Active: manifest.Name == active,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		Stores: r.contextPaths(manifest.Name),
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// CreateContext initializes one named Context without accepting any secret.
func (r *Runtime) CreateContext(ctx context.Context, name string, image string, mode tobari.ContextPolicyMode) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion, Name: name,
		AgentProfile: tobari.DefaultProfile, Image: image, PolicyMode: mode,
	}
	if err := manifest.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	if _, err := os.Lstat(r.contextManifestPath(name)); err == nil {
		return tobari.ContextReport{}, tobari.ErrContextExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContext(manifest, false); err != nil {
		return tobari.ContextReport{}, err
	}
	result := tobari.ContextReport{
		Task: tobari.TaskContextCreate, Name: name, Active: false,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		Stores: r.contextPaths(name),
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// UseContext changes only the host-owned active marker.
func (r *Runtime) UseContext(ctx context.Context, name string) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.writeActiveContext(name); err != nil {
		return tobari.ContextReport{}, err
	}
	result := tobari.ContextReport{
		Task: tobari.TaskContextUse, Name: manifest.Name, Active: true,
		AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
		Stores: r.contextPaths(name),
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// ActiveContextName exposes only the trusted selected name to the application
// layer so it can refuse to enter a cluster whose policy is stale.
func (r *Runtime) ActiveContextName(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	manifest, _, err := r.activeContext()
	if err != nil {
		return "", err
	}
	return manifest.Name, nil
}
