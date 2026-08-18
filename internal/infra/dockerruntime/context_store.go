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
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	maxContextManifestFixedJSONBytes = 16 * 1024
	maxActiveContextDocumentBytes    = 16 * 1024
	maxJSONEncodedByteExpansion      = 6
	contextGitIdentityValueCount     = 2
)

// encoding/json can expand one input byte to a six-byte escape. Reserve that
// worst case for every bounded shell and Git identity scalar, plus the fixed
// 16 KiB allowance for the manifest's bounded identity/runtime fields,
// JSON structure, and indentation. The inventory length is derived from the
// domain so adding an allowlisted shell variable cannot silently invalidate an
// otherwise valid manifest.
var maxContextManifestBytes = int64(
	maxContextManifestFixedJSONBytes +
		maxJSONEncodedByteExpansion*(len(tobari.ContextShellEnvironmentVariables())*tobari.MaxContextShellValueBytes+
			contextGitIdentityValueCount*tobari.MaxContextGitIdentityValueBytes),
)

type activeContextDocument struct {
	Name string `json:"name"`
}

type observedContext struct {
	state    tobari.ContextObservationState
	manifest tobari.ContextManifest
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

func (r *Runtime) activeContextPath() string {
	return filepath.Join(r.contextsDirectory(), "active.json")
}

func (r *Runtime) contextPaths(name string) tobari.ContextStorePaths {
	return tobari.ContextStorePaths{
		PolicyDirectory:   r.contextPolicyDirectory(name),
		RuntimeDirectory:  r.contextRuntimeDirectory(name),
		RuntimeDockerfile: r.contextRuntimeDockerfile(name),
	}
}

// diagnosticContextStores resolves the stores to inspect without initializing
// the Context catalog.
func (r *Runtime) diagnosticContextStores() (tobari.ContextStorePaths, error) {
	if _, err := os.Lstat(r.activeContextPath()); errors.Is(err, os.ErrNotExist) {
		return r.contextPaths(tobari.DefaultContextName), nil
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
	return r.withContextStoreLock(r.ensureContextStoreUnlocked)
}

func (r *Runtime) ensureContextStoreUnlocked() error {
	if err := r.ensurePrivateDirectory(r.contextsDirectory()); err != nil {
		return fmt.Errorf("prepare Context directory: %w", err)
	}
	image := r.defaultRuntimeImage()
	if _, err := os.Lstat(r.contextManifestPath(tobari.DefaultContextName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect default Context manifest: %w", err)
	}
	defaultManifest := tobari.ContextManifest{
		SchemaVersion:        tobari.ContextSchemaVersion,
		Name:                 tobari.DefaultContextName,
		AgentProfile:         tobari.DefaultProfile,
		Image:                image,
		PolicyMode:           tobari.ContextPolicyModeGuided,
		SourceAccess:         tobari.ContextSourceAccessReadWrite,
		PolicyPresetOrigin:   tobari.DefaultPolicyPresetOrigin,
		PolicyPresetRevision: tobari.DefaultPolicyPresetRevision(),
		ShellEnvironment:     tobari.InitialContextShellEnvironment(),
	}
	if existing, err := r.readContextManifestRaw(tobari.DefaultContextName); err == nil {
		defaultManifest = existing
	} else if !errors.Is(err, tobari.ErrContextNotFound) {
		return err
	}
	if defaultManifest.ID == "" {
		var err error
		defaultManifest.ID, err = r.identities.newContextID()
		if err != nil {
			return err
		}
	}
	return r.initializeContextStoreUnlocked(defaultManifest)
}

// initializeContextStoreUnlocked completes mutation-owned initialization with
// the selected default manifest. The caller must hold the Context-store lock.
func (r *Runtime) initializeContextStoreUnlocked(defaultManifest tobari.ContextManifest) error {
	return r.initializeContextStoreUnlockedWithPreset(defaultManifest, nil)
}

func (r *Runtime) initializeContextStoreUnlockedWithPreset(defaultManifest tobari.ContextManifest, snapshot []byte) error {
	if err := r.ensurePrivateDirectory(r.contextsDirectory()); err != nil {
		return fmt.Errorf("prepare Context directory: %w", err)
	}
	if err := r.ensureContextWithPresetSnapshot(defaultManifest, snapshot); err != nil {
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

func (r *Runtime) withContextStoreLock(action func() error) error {
	if err := r.ensurePrivateDirectory(r.configDirectory); err != nil {
		return err
	}
	path := filepath.Join(r.configDirectory, "contexts.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("Context lock is not a regular file")
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

func (r *Runtime) ensureContext(manifest tobari.ContextManifest) error {
	return r.ensureContextWithPresetSnapshot(manifest, nil)
}

func (r *Runtime) ensureContextWithPresetSnapshot(manifest tobari.ContextManifest, snapshot []byte) error {
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
	if len(snapshot) > 0 {
		path := r.contextPresetPath(manifest.Name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if err := initializeBytes(path, snapshot, 0o600); err != nil {
				return fmt.Errorf("write Context %q policy preset snapshot: %w", manifest.Name, err)
			}
		} else if err != nil {
			return fmt.Errorf("inspect Context %q policy preset snapshot: %w", manifest.Name, err)
		}
	}
	if err := r.ensureContextPreset(manifest); err != nil {
		return fmt.Errorf("prepare Context %q policy preset: %w", manifest.Name, err)
	}
	domainsDirectory := filepath.Join(r.contextPolicyDirectory(manifest.Name), policyDomainsName)
	if err := r.ensurePrivateDirectory(domainsDirectory); err != nil {
		return fmt.Errorf("prepare Context %q policy domains: %w", manifest.Name, err)
	}
	if manifest.PolicyMode == tobari.ContextPolicyModeAdvanced {
		for _, name := range []string{"tobari.rego", "tobari_test.rego"} {
			if err := initializeFile(filepath.Join(r.contextPolicyDirectory(manifest.Name), name), "opa/policy/"+name, 0o600); err != nil {
				return err
			}
		}
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

func (r *Runtime) readContextManifestRaw(name string) (tobari.ContextManifest, error) {
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
	if err := manifest.Validate(); err != nil {
		return tobari.ContextManifest{}, err
	}
	if manifest.Name != name {
		return tobari.ContextManifest{}, fmt.Errorf("Context manifest name does not match its path")
	}
	if _, err := r.readContextPreset(manifest); err != nil {
		return tobari.ContextManifest{}, fmt.Errorf("read Context policy preset: %w", err)
	}
	return manifest, nil
}

func (r *Runtime) readContextManifest(name string) (tobari.ContextManifest, error) {
	return r.readContextManifestRaw(name)
}

func (r *Runtime) readActiveContext() (string, error) {
	info, err := os.Lstat(r.activeContextPath())
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxActiveContextDocumentBytes {
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
	if _, err := r.readContextManifestRaw(document.Name); err != nil {
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

func (r *Runtime) observeActiveContextName() (string, error) {
	name, err := r.readActiveContext()
	if errors.Is(err, os.ErrNotExist) {
		return tobari.DefaultContextName, nil
	}
	return name, err
}

// observeContext never initializes the Context catalog. A synthetic default is
// display state only and carries no stable authority manifest.
func (r *Runtime) observeContext(name string) (observedContext, error) {
	explicit := name != ""
	if !explicit {
		var err error
		name, err = r.observeActiveContextName()
		if err != nil {
			return observedContext{}, err
		}
	}
	manifest, err := r.readContextManifestRaw(name)
	if errors.Is(err, tobari.ErrContextNotFound) {
		if explicit || name != tobari.DefaultContextName {
			return observedContext{}, err
		}
		return observedContext{
			state: tobari.ContextObservationSyntheticDefault,
			manifest: tobari.ContextManifest{
				SchemaVersion: tobari.ContextSchemaVersion, Name: tobari.DefaultContextName,
				AgentProfile: tobari.DefaultProfile, Image: r.defaultRuntimeImage(), PolicyMode: tobari.ContextPolicyModeGuided,
				SourceAccess:       tobari.ContextSourceAccessReadWrite,
				PolicyPresetOrigin: tobari.DefaultPolicyPresetOrigin,
				ShellEnvironment:   tobari.InitialContextShellEnvironment(),
			},
		}, nil
	}
	if err != nil {
		return observedContext{}, err
	}
	return observedContext{state: tobari.ContextObservationPersisted, manifest: manifest}, nil
}

// ObserveContext exposes only a validated stable manifest as authority.
func (r *Runtime) ObserveContext(ctx context.Context, name string) (tobari.ContextObservation, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextObservation{}, err
	}
	observed, err := r.observeContext(name)
	if err != nil {
		return tobari.ContextObservation{}, err
	}
	result := tobari.ContextObservation{State: observed.state, Name: observed.manifest.Name}
	if observed.state == tobari.ContextObservationPersisted {
		manifest := observed.manifest
		result.Manifest = &manifest
	}
	return result, result.Validate()
}

// resolveContext resolves an explicit display name to its trusted manifest, or
// uses the current Context only when the caller omitted a name.
func (r *Runtime) resolveContext(name string) (tobari.ContextManifest, tobari.ContextStorePaths, error) {
	if name == "" {
		return r.activeContext()
	}
	if err := r.ensureContextStore(); err != nil {
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

func (r *Runtime) contextByID(id string) (tobari.ContextManifest, tobari.ContextStorePaths, error) {
	if err := tobari.ValidateContextID(id); err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
	}
	var selected *tobari.ContextManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := r.readContextManifest(entry.Name())
		if err != nil {
			return tobari.ContextManifest{}, tobari.ContextStorePaths{}, err
		}
		if manifest.ID != id {
			continue
		}
		if selected != nil {
			return tobari.ContextManifest{}, tobari.ContextStorePaths{}, fmt.Errorf("Context ID is ambiguous")
		}
		copy := manifest
		selected = &copy
	}
	if selected == nil {
		return tobari.ContextManifest{}, tobari.ContextStorePaths{}, fmt.Errorf("%w: Context ID", tobari.ErrContextNotFound)
	}
	return *selected, r.contextPaths(selected.Name), nil
}

func (r *Runtime) ResolveContext(ctx context.Context, name string) (tobari.ContextManifest, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextManifest{}, err
	}
	manifest, _, err := r.resolveContext(name)
	return manifest, err
}

// ListContexts returns the complete host-owned Context collection.
func (r *Runtime) ListContexts(ctx context.Context) (tobari.ContextListResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextListResult{}, err
	}
	active, err := r.observeActiveContextName()
	if err != nil {
		return tobari.ContextListResult{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if errors.Is(err, os.ErrNotExist) {
		result := tobari.ContextListResult{
			Task: tobari.TaskContextList, ContextState: tobari.ContextObservationSyntheticDefault,
			Active: tobari.DefaultContextName, Items: []tobari.ContextSummary{},
		}
		return result, result.Validate()
	}
	if err != nil {
		return tobari.ContextListResult{}, err
	}
	items := make([]tobari.ContextSummary, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return tobari.ContextListResult{}, fmt.Errorf("Context directory contains a symbolic link")
		}
		if !entry.IsDir() {
			continue
		}
		manifest, err := r.readContextManifestRaw(entry.Name())
		if err != nil {
			return tobari.ContextListResult{}, err
		}
		runtimeReport, err := r.contextRuntimeReport(manifest)
		if err != nil {
			return tobari.ContextListResult{}, err
		}
		nativeReadiness, err := tobari.ResolveContextNativeReadiness(manifest.NativeReadiness, manifest.PolicyPresetOrigin)
		if err != nil {
			return tobari.ContextListResult{}, err
		}
		preset, err := r.readContextPreset(manifest)
		if err != nil {
			return tobari.ContextListResult{}, err
		}
		items = append(items, tobari.ContextSummary{
			ID: manifest.ID, Name: manifest.Name, ContextState: tobari.ContextObservationPersisted, Active: manifest.Name == active,
			AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
			SourceAccess:         manifest.SourceAccess,
			PolicyPresetOrigin:   manifest.PolicyPresetOrigin,
			PolicyPresetRevision: manifest.PolicyPresetRevision,
			NativeReadiness:      nativeReadiness,
			MethodPolicy:         preset.MethodPolicy,
			RuntimeStatus:        runtimeReport.Status,
			Bootstrap:            tobari.ContextBootstrapReportFrom(manifest.Bootstrap),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	activeState := tobari.ContextObservationSyntheticDefault
	for _, item := range items {
		if item.Active {
			activeState = item.ContextState
			break
		}
	}
	result := tobari.ContextListResult{Task: tobari.TaskContextList, ContextState: activeState, Active: active, Items: items}
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
	observed, err := r.observeContext(name)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	active, err := r.observeActiveContextName()
	if err != nil {
		return tobari.ContextReport{}, err
	}
	if observed.state == tobari.ContextObservationPersisted {
		return r.contextReport(ctx, tobari.TaskContextShow, observed.manifest, active)
	}
	return r.nonPersistedContextReport(observed, active)
}

// ConfigureContextShell atomically updates one staged set of distinct
// allowlisted shell environment settings in an explicit or current Context. Inherited values are deliberately
// resolved later at session entry rather than persisted here.
func (r *Runtime) ConfigureContextShell(
	ctx context.Context, name string, changes []tobari.ContextShellEnvironmentSetting,
) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if _, err := tobari.ApplyContextShellEnvironmentSettings(nil, changes); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		if name == "" {
			name = active
		}
		manifest, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		settings, err := tobari.ApplyContextShellEnvironmentSettings(manifest.ShellEnvironment, changes)
		if err != nil {
			return err
		}
		manifest.ShellEnvironment = settings
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context shell environment: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigShell, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// ConfigureContextGit atomically updates the Context-owned Git identity pair.
// Selecting default removes the persisted override; inherited host values are
// resolved later for a specific Workspace root and never stored here.
func (r *Runtime) ConfigureContextGit(
	ctx context.Context, name string, change tobari.ContextGitIdentitySetting,
) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := change.Validate(true); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		if name == "" {
			name = active
		}
		manifest, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		manifest.GitIdentity = persistedContextGitIdentity(change)
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context Git identity: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigGit, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func persistedContextGitIdentity(setting tobari.ContextGitIdentitySetting) *tobari.ContextGitIdentitySetting {
	if setting.Source == tobari.ContextGitIdentityDefault {
		return nil
	}
	result := setting
	if setting.Name != nil {
		value := *setting.Name
		result.Name = &value
	}
	if setting.Email != nil {
		value := *setting.Email
		result.Email = &value
	}
	return &result
}

// CreateContext initializes one named Context without accepting any secret.
func (r *Runtime) CreateContext(
	ctx context.Context, name string, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess,
) (tobari.ContextReport, error) {
	return r.CreateContextWithPreset(ctx, name, image, mode, sourceAccess, tobari.DefaultPolicyPresetOrigin, tobari.ContextNativeReadinessEnabled)
}

func (r *Runtime) CreateContextWithPreset(
	ctx context.Context, name string, image string, mode tobari.ContextPolicyMode, sourceAccess tobari.ContextSourceAccess, presetOrigin string, readinessSelections ...tobari.ContextNativeReadiness,
) (tobari.ContextReport, error) {
	nativeReadiness := tobari.ContextNativeReadinessEnabled
	if len(readinessSelections) > 1 {
		return tobari.ContextReport{}, fmt.Errorf("Context native readiness selection is invalid")
	}
	if len(readinessSelections) == 1 {
		nativeReadiness = readinessSelections[0]
	}
	return r.CreateContextWithComposition(ctx, name, image, mode, sourceAccess, tobari.ContextCreateComposition{
		PolicyPresetOrigin: presetOrigin,
		NativeReadiness:    nativeReadiness,
	})
}

func (r *Runtime) CreateContextWithComposition(
	ctx context.Context,
	name string,
	image string,
	mode tobari.ContextPolicyMode,
	sourceAccess tobari.ContextSourceAccess,
	composition tobari.ContextCreateComposition,
) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	if err := composition.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	preset, normalizedPreset, presetRevision, err := r.resolvePolicyPresetSnapshot(composition.PolicyPresetOrigin)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	if composition.MethodPolicy != nil {
		preset, err = tobari.ComposePolicyPresetMethodPolicy(preset, composition.MethodPolicy.Clone())
		if err != nil {
			return tobari.ContextReport{}, err
		}
		preset, normalizedPreset, presetRevision, err = tobari.NormalizePolicyPreset(preset)
		if err != nil {
			return tobari.ContextReport{}, err
		}
	}
	manifest := tobari.ContextManifest{
		SchemaVersion: tobari.ContextSchemaVersion, Name: name,
		AgentProfile: tobari.DefaultProfile, Image: r.resolveBuiltinImageSelector(image), PolicyMode: mode,
		SourceAccess:         sourceAccess,
		PolicyPresetOrigin:   composition.PolicyPresetOrigin,
		PolicyPresetRevision: presetRevision,
		NativeReadiness:      composition.NativeReadiness,
		ShellEnvironment:     tobari.InitialContextShellEnvironment(),
		Bootstrap:            composition.Bootstrap,
	}
	id, err := r.identities.newContextID()
	if err != nil {
		return tobari.ContextReport{}, err
	}
	manifest.ID = id
	if err := manifest.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	var active string
	if err := r.withContextStoreLock(func() error {
		if _, err := os.Lstat(r.contextManifestPath(name)); err == nil {
			return tobari.ErrContextExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if name == tobari.DefaultContextName {
			if err := r.initializeContextStoreUnlockedWithPreset(manifest, normalizedPreset); err != nil {
				return err
			}
		} else {
			if err := r.ensureContextStoreUnlocked(); err != nil {
				return err
			}
			if err := r.ensureContextWithPresetSnapshot(manifest, normalizedPreset); err != nil {
				return err
			}
		}

		var err error
		active, err = r.readActiveContext()
		return err
	}); err != nil {
		return tobari.ContextReport{}, err
	}
	report, err := r.contextReport(ctx, tobari.TaskContextCreate, manifest, active)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	if _, configured, loadErr := r.LoadState(ctx); loadErr != nil {
		return tobari.ContextReport{}, loadErr
	} else if configured {
		report.Cluster = tobari.ContextClusterStatusRequiresReconcile
	}
	return report, report.Validate()
}

// DeleteContext removes one exact non-current Context store after proving that
// no durable Workspace remains bound to its stable Context identity. The
// caller holds the installation lifecycle lock across this check-and-delete.
func (r *Runtime) DeleteContext(ctx context.Context, name string) (tobari.ContextDeleteResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if name == tobari.DefaultContextName {
		return tobari.ContextDeleteResult{}, tobari.ErrContextProtected
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	active, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	if active == name {
		return tobari.ContextDeleteResult{}, tobari.ErrContextActive
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	for _, project := range projects {
		if project.ContextID == manifest.ID || project.ContextName == name {
			return tobari.ContextDeleteResult{}, tobari.ErrContextHasWorkspaces
		}
	}
	cluster := tobari.ContextClusterStatusNotApplicable
	if _, configured, err := r.LoadState(ctx); err != nil {
		return tobari.ContextDeleteResult{}, err
	} else if configured {
		cluster = tobari.ContextClusterStatusRequiresReconcile
	}
	if err := r.withContextStoreLock(func() error {
		current, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		if current.ID != manifest.ID {
			return fmt.Errorf("Context identity changed before deletion")
		}
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		if active == name {
			return tobari.ErrContextActive
		}
		authDirectory := filepath.Join(r.authContextsDirectory(), manifest.ID)
		if info, err := os.Lstat(authDirectory); err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("Context authentication state is unsafe")
			}
			if err := os.RemoveAll(authDirectory); err != nil { // #nosec G301 -- exact validated Context UUID child.
				return fmt.Errorf("remove Context authentication state: %w", err)
			}
			if err := syncDirectoryIfPresent(r.authContextsDirectory()); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Context authentication state: %w", err)
		}
		contextDirectory := r.contextDirectory(name)
		if err := requirePrivateDirectory(contextDirectory); err != nil {
			return fmt.Errorf("inspect Context store before deletion: %w", err)
		}
		if err := os.RemoveAll(contextDirectory); err != nil { // #nosec G301 -- exact validated owner-only Context child.
			return fmt.Errorf("remove Context store: %w", err)
		}
		if err := syncDirectoryIfPresent(r.contextsDirectory()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return tobari.ContextDeleteResult{}, err
	}
	result := tobari.ContextDeleteResult{
		Task: tobari.TaskContextDelete, ID: manifest.ID, Name: manifest.Name, Deleted: true, Cluster: cluster,
	}
	return result, result.Validate()
}

// UseContext changes only the default used when execution omits a Context.
func (r *Runtime) UseContext(ctx context.Context, name string) (tobari.ContextReport, error) {
	return r.UseContextWithProgress(ctx, name, nil)
}

// UseContextWithProgress keeps default selection independent from cluster
// reconciliation while satisfying the progress-aware application port.
func (r *Runtime) UseContextWithProgress(
	ctx context.Context, name string, progress tobari.ClusterUpProgressSink,
) (tobari.ContextReport, error) {
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
	active, err := r.readActiveContext()
	if err != nil {
		return tobari.ContextReport{}, err
	}
	_ = progress
	if err := r.selectContext(active, name); err != nil {
		return tobari.ContextReport{}, err
	}
	status := tobari.ContextClusterStatusDefaultUpdated
	if active == name {
		status = tobari.ContextClusterStatusAlreadyReady
	}
	return r.contextUseReport(ctx, manifest, name, status)
}

func (r *Runtime) contextUseReport(
	ctx context.Context, manifest tobari.ContextManifest, active string,
	clusterStatus tobari.ContextClusterStatus,
) (tobari.ContextReport, error) {
	result, err := r.contextReport(ctx, tobari.TaskContextUse, manifest, active)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	result.Cluster = clusterStatus
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (r *Runtime) selectContext(previous, next string) error {
	if previous == next {
		return nil
	}
	return r.writeActiveContext(next)
}

func (r *Runtime) restoreContextSelection(name string, state tobari.State) error {
	if _, err := r.readContextManifest(name); err != nil {
		return fmt.Errorf("previous Context is unavailable: %w", err)
	}
	if err := r.writeActiveContext(name); err != nil {
		return fmt.Errorf("restore active Context marker: %w", err)
	}
	if err := r.writeState(state); err != nil {
		return fmt.Errorf("restore shared state: %w", err)
	}
	return nil
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
