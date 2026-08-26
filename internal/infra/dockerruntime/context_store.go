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
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
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
		maxJSONEncodedByteExpansion*(len(tobari.ManifestShellEnvironmentVariables())*tobari.MaxContextShellValueBytes+
			contextGitIdentityValueCount*tobari.MaxContextGitIdentityValueBytes),
)

type defaultManifestDocument struct {
	SchemaVersion       int    `json:"schema_version"`
	WorkspaceManifestID string `json:"workspace_manifest_id"`
}

type legacyActiveContextDocument struct {
	Name string `json:"name"`
}

type observedContext struct {
	state    tobari.ManifestObservationState
	manifest tobari.WorkspaceManifest
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

func (r *Runtime) manifestRevisionsDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "revisions")
}

// manifestRevisionPath uses generation only to distinguish history receipts
// such as A->B->A. ManifestID plus the semantic digest remains authority;
// generation is correlation metadata and never authorizes different content.
func (r *Runtime) manifestRevisionPath(name string, generation uint64, revision string) (string, error) {
	if err := tobari.ValidateName(name); err != nil {
		return "", err
	}
	if generation == 0 {
		return "", fmt.Errorf("Manifest generation must be positive")
	}
	if err := tobari.ValidateDigest(revision); err != nil {
		return "", err
	}
	return filepath.Join(r.manifestRevisionsDirectory(name), fmt.Sprintf("%020d-%s.json", generation, strings.TrimPrefix(revision, "sha256:"))), nil
}

func (r *Runtime) retainWorkspaceManifestRevision(manifest tobari.WorkspaceManifest) error {
	if err := manifest.ValidatePublished(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.manifestRevisionsDirectory(manifest.Name)); err != nil {
		return fmt.Errorf("prepare Manifest revisions: %w", err)
	}
	path, err := r.manifestRevisionPath(manifest.Name, manifest.Desired.Generation, manifest.Desired.Revision)
	if err != nil {
		return err
	}
	if existing, readErr := readWorkspaceManifestRevision(path); readErr == nil {
		existingBody, existingErr := json.Marshal(existing)
		candidateBody, candidateErr := json.Marshal(manifest)
		if existingErr != nil || candidateErr != nil || !bytes.Equal(existingBody, candidateBody) {
			return fmt.Errorf("retained Manifest revision identity conflicts")
		}
		return nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	return writeAtomicJSON(path, manifest)
}

func readWorkspaceManifestRevision(path string) (tobari.WorkspaceManifest, error) {
	var manifest tobari.WorkspaceManifest
	if err := readStrictJSON(path, &manifest); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	if err := manifest.ValidatePublished(); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	return manifest, nil
}

func (r *Runtime) contextPolicyDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "policy")
}

func (r *Runtime) activeContextPath() string {
	return filepath.Join(r.contextsDirectory(), "active.json")
}

func (r *Runtime) defaultManifestPath() string {
	return filepath.Join(r.contextsDirectory(), "default.json")
}

func (r *Runtime) contextPaths(name string) tobari.ManifestStorePaths {
	return tobari.ManifestStorePaths{
		PolicyDirectory: r.contextPolicyDirectory(name),
	}
}

// diagnosticContextStores resolves the stores to inspect without initializing
// the Context catalog.
func (r *Runtime) diagnosticContextStores() (tobari.ManifestStorePaths, error) {
	if _, err := os.Lstat(r.defaultManifestPath()); errors.Is(err, os.ErrNotExist) {
		return r.contextPaths(tobari.DefaultManifestName), nil
	} else if err != nil {
		return tobari.ManifestStorePaths{}, fmt.Errorf("inspect default Manifest: %w", err)
	}
	name, err := r.readDefaultManifestName()
	if err != nil {
		return tobari.ManifestStorePaths{}, err
	}
	paths := r.contextPaths(name)
	if err := paths.Validate(); err != nil {
		return tobari.ManifestStorePaths{}, err
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
	standardBinding, err := r.resolveRuntimeBinding(tobari.StandardRuntimeName)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(r.contextManifestPath(tobari.DefaultManifestName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect default Context manifest: %w", err)
	}
	defaultManifest := tobari.WorkspaceManifest{
		SchemaVersion:    tobari.WorkspaceManifestSchemaVersion,
		Name:             tobari.DefaultManifestName,
		AgentProfile:     tobari.DefaultProfile,
		Image:            tobari.BuiltinImageSelector,
		PolicyMode:       tobari.ManifestPolicyModeGuided,
		SourceAccess:     tobari.ManifestSourceAccessReadWrite,
		PolicyRevision:   tobari.DefaultContextPolicyRevision(),
		ShellEnvironment: tobari.InitialContextShellEnvironment(),
		RuntimeBinding:   &standardBinding,
	}
	if existing, err := r.readContextManifestRaw(tobari.DefaultManifestName); err == nil {
		defaultManifest = existing
	} else if !errors.Is(err, tobari.ErrContextNotFound) {
		return err
	}
	if defaultManifest.ID == "" {
		var err error
		defaultManifest.ID, err = r.identities.newWorkspaceManifestID()
		if err != nil {
			return err
		}
	}
	if defaultManifest.Desired.Generation == 0 {
		defaultManifest, err = tobari.PublishWorkspaceManifest(defaultManifest, nil)
		if err != nil {
			return err
		}
	}
	return r.initializeContextStoreUnlocked(defaultManifest)
}

// initializeContextStoreUnlocked completes mutation-owned initialization with
// the selected default manifest. The caller must hold the Context-store lock.
func (r *Runtime) initializeContextStoreUnlocked(defaultManifest tobari.WorkspaceManifest) error {
	return r.initializeContextStoreUnlockedWithPolicy(defaultManifest, nil)
}

func (r *Runtime) initializeContextStoreUnlockedWithPolicy(defaultManifest tobari.WorkspaceManifest, snapshot []byte) error {
	if err := r.ensurePrivateDirectory(r.contextsDirectory()); err != nil {
		return fmt.Errorf("prepare Context directory: %w", err)
	}
	if err := r.ensureContextWithPolicySnapshot(defaultManifest, snapshot); err != nil {
		return err
	}
	if _, err := os.Lstat(r.defaultManifestPath()); errors.Is(err, os.ErrNotExist) {
		selected := defaultManifest
		if _, legacyErr := os.Lstat(r.activeContextPath()); legacyErr == nil {
			legacyName, readErr := r.readLegacyActiveContextName()
			if readErr != nil {
				return readErr
			}
			selected, readErr = r.readContextManifestRaw(legacyName)
			if readErr != nil {
				return fmt.Errorf("migrate default Manifest selection: %w", readErr)
			}
		} else if !errors.Is(legacyErr, os.ErrNotExist) {
			return fmt.Errorf("inspect legacy active Context: %w", legacyErr)
		}
		if err := r.writeDefaultManifest(selected); err != nil {
			return fmt.Errorf("initialize default Manifest: %w", err)
		}
		if err := os.Remove(r.activeContextPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove migrated active Context marker: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect default Manifest: %w", err)
	} else if _, err := r.readDefaultManifestName(); err != nil {
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

func (r *Runtime) ensureContext(manifest tobari.WorkspaceManifest) error {
	return r.ensureContextWithPolicySnapshot(manifest, nil)
}

func (r *Runtime) ensureContextWithPolicySnapshot(manifest tobari.WorkspaceManifest, snapshot []byte) error {
	if err := manifest.ValidatePublished(); err != nil {
		return err
	}
	directory := r.contextDirectory(manifest.Name)
	if err := r.ensurePrivateDirectory(directory); err != nil {
		return fmt.Errorf("prepare Context %q: %w", manifest.Name, err)
	}
	if err := r.ensurePrivateDirectory(r.contextPolicyDirectory(manifest.Name)); err != nil {
		return fmt.Errorf("prepare Context %q policy: %w", manifest.Name, err)
	}
	if err := r.ensureContextPolicy(manifest, snapshot); err != nil {
		return fmt.Errorf("prepare Context %q context policy: %w", manifest.Name, err)
	}
	domainsDirectory := filepath.Join(r.contextPolicyDirectory(manifest.Name), policyDomainsName)
	if err := r.ensurePrivateDirectory(domainsDirectory); err != nil {
		return fmt.Errorf("prepare Context %q policy domains: %w", manifest.Name, err)
	}
	if manifest.PolicyMode == tobari.ManifestPolicyModeAdvanced {
		for _, name := range []string{"tobari.rego", "tobari_test.rego"} {
			if err := initializeFile(filepath.Join(r.contextPolicyDirectory(manifest.Name), name), "opa/policy/"+name, 0o600); err != nil {
				return err
			}
		}
	}
	if _, err := os.Lstat(r.contextManifestPath(manifest.Name)); errors.Is(err, os.ErrNotExist) {
		if err := r.retainWorkspaceManifestRevision(manifest); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write Context manifest: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("inspect Context manifest: %w", err)
	}
	return nil
}

// installContextWithCreationSnapshot stages every Context-owned creation file
// beside the Context catalog and publishes the complete standalone Context
// with one same-filesystem rename. The staging tree is never visible to a
// concurrent Context collection read.
func (r *Runtime) installContextWithCreationSnapshot(
	manifest tobari.WorkspaceManifest, snapshot []byte, advanced map[string][]byte,
) error {
	if err := manifest.ValidatePublished(); err != nil {
		return err
	}
	if len(snapshot) == 0 {
		_, generated, _, err := defaultContextPolicyBytes()
		if err != nil {
			return err
		}
		snapshot = generated
	}
	_, normalized, revision, err := decodeContextPolicy(snapshot)
	if err != nil || revision != manifest.PolicyRevision || !bytes.Equal(snapshot, normalized) {
		return fmt.Errorf("Context creation policy snapshot is invalid")
	}
	advancedSources := map[string][]byte{}
	if manifest.PolicyMode == tobari.ManifestPolicyModeAdvanced {
		for _, name := range []string{"tobari.rego", "tobari_test.rego"} {
			var source []byte
			if advanced != nil {
				source = advanced[name]
				if len(source) == 0 || len(source) > maxPolicyPreflight {
					return fmt.Errorf("advanced Context Base policy source is invalid")
				}
			} else {
				source, err = runtimeassets.Read("opa/policy/" + name)
				if err != nil {
					return err
				}
			}
			advancedSources[name] = source
		}
	}
	if err := r.ensurePrivateDirectory(r.contextsDirectory()); err != nil {
		return fmt.Errorf("prepare Context directory: %w", err)
	}
	staging, err := os.MkdirTemp(r.configDirectory, ".context-create-")
	if err != nil {
		return fmt.Errorf("stage Context creation: %w", err)
	}
	defer func() { _ = os.RemoveAll(staging) }()     // #nosec G301 -- exact MkdirTemp-owned staging child.
	if err := os.Chmod(staging, 0o700); err != nil { // #nosec G302 -- owner-only directory requires traversal.
		return fmt.Errorf("protect staged Context: %w", err)
	}
	policyDirectory := filepath.Join(staging, "policy")
	if err := os.Mkdir(policyDirectory, 0o700); err != nil {
		return fmt.Errorf("stage Context policy: %w", err)
	}
	if err := initializeBytes(filepath.Join(policyDirectory, "context.json"), normalized, 0o600); err != nil {
		return err
	}
	if err := os.Mkdir(filepath.Join(policyDirectory, policyDomainsName), 0o700); err != nil {
		return fmt.Errorf("stage Context learned-policy directory: %w", err)
	}
	for name, source := range advancedSources {
		if err := initializeBytes(filepath.Join(policyDirectory, name), source, 0o600); err != nil {
			return err
		}
	}
	if err := writeAtomicJSON(filepath.Join(staging, "context.json"), manifest); err != nil {
		return fmt.Errorf("stage Context manifest: %w", err)
	}
	if err := os.Mkdir(filepath.Join(staging, "revisions"), 0o700); err != nil {
		return fmt.Errorf("stage Manifest revision store: %w", err)
	}
	revisionPath := filepath.Join(staging, "revisions", strings.TrimPrefix(manifest.Desired.Revision, "sha256:")+".json")
	if err := writeAtomicJSON(revisionPath, manifest); err != nil {
		return fmt.Errorf("stage Manifest revision: %w", err)
	}
	target := r.contextDirectory(manifest.Name)
	if _, err := os.Lstat(target); err == nil {
		return tobari.ErrContextExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(staging, target); err != nil {
		return fmt.Errorf("publish Context creation: %w", err)
	}
	return syncDirectoryIfPresent(r.contextsDirectory())
}

func (r *Runtime) readContextManifestRaw(name string) (tobari.WorkspaceManifest, error) {
	if err := tobari.ValidateName(name); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	path := r.contextManifestPath(name)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tobari.WorkspaceManifest{}, fmt.Errorf("%w: %s", tobari.ErrContextNotFound, name)
		}
		return tobari.WorkspaceManifest{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxContextManifestBytes {
		return tobari.WorkspaceManifest{}, fmt.Errorf("Context manifest is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- name is validated and path is a Context child.
	if err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest tobari.WorkspaceManifest
	if err := decoder.Decode(&manifest); err != nil {
		return tobari.WorkspaceManifest{}, fmt.Errorf("decode Context manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.WorkspaceManifest{}, fmt.Errorf("Context manifest contains trailing data")
	}
	if err := manifest.ValidatePublished(); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	if manifest.Name != name {
		return tobari.WorkspaceManifest{}, fmt.Errorf("Context manifest name does not match its path")
	}
	if _, err := r.readContextPolicy(manifest); err != nil {
		return tobari.WorkspaceManifest{}, fmt.Errorf("read Context context policy: %w", err)
	}
	return manifest, nil
}

func (r *Runtime) readContextManifest(name string) (tobari.WorkspaceManifest, error) {
	return r.readContextManifestRaw(name)
}

func (r *Runtime) readLegacyActiveContextName() (string, error) {
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
	var document legacyActiveContextDocument
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

func (r *Runtime) readDefaultManifestName() (string, error) {
	info, err := os.Lstat(r.defaultManifestPath())
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxActiveContextDocumentBytes {
		return "", fmt.Errorf("default Manifest selector is unsafe")
	}
	data, err := os.ReadFile(r.defaultManifestPath()) // #nosec G304 -- fixed default Manifest path.
	if err != nil {
		return "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var document defaultManifestDocument
	if err := decoder.Decode(&document); err != nil {
		return "", fmt.Errorf("decode default Manifest selector: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("default Manifest selector contains trailing data")
	}
	if document.SchemaVersion != tobari.WorkspaceManifestSchemaVersion {
		return "", fmt.Errorf("default Manifest selector schema is invalid")
	}
	if err := tobari.ValidateWorkspaceManifestID(document.WorkspaceManifestID); err != nil {
		return "", err
	}
	manifest, err := r.contextByIDRaw(document.WorkspaceManifestID)
	if err != nil {
		return "", fmt.Errorf("default Manifest is unavailable: %w", err)
	}
	return manifest.Name, nil
}

func (r *Runtime) writeDefaultManifest(manifest tobari.WorkspaceManifest) error {
	if err := manifest.ValidatePublished(); err != nil {
		return err
	}
	current, err := r.readContextManifest(manifest.Name)
	if err != nil {
		return err
	}
	if current.ID != manifest.ID {
		return fmt.Errorf("default Manifest identity changed")
	}
	return writeAtomicJSON(r.defaultManifestPath(), defaultManifestDocument{
		SchemaVersion: tobari.WorkspaceManifestSchemaVersion, WorkspaceManifestID: manifest.ID,
	})
}

func (r *Runtime) publishWorkspaceManifestUpdate(previous, draft tobari.WorkspaceManifest) (tobari.WorkspaceManifest, error) {
	published, err := tobari.PublishWorkspaceManifest(draft, &previous)
	if err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	if published.Desired == previous.Desired {
		return published, nil
	}
	if err := r.retainWorkspaceManifestRevision(published); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	if err := writeAtomicJSON(r.contextManifestPath(published.Name), published); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	return published, nil
}

func (r *Runtime) activeContext() (tobari.WorkspaceManifest, tobari.ManifestStorePaths, error) {
	name, err := r.readDefaultManifestName()
	if err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	paths := r.contextPaths(name)
	if err := paths.Validate(); err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	return manifest, paths, nil
}

// ReadWorkspaceManifestByID resolves a published Manifest by stable identity
// without consulting or changing the installation default selector.
func (r *Runtime) ReadWorkspaceManifestByID(ctx context.Context, id string) (tobari.WorkspaceManifest, error) {
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	manifest, _, err := r.contextByID(id)
	return manifest, err
}

func (r *Runtime) observeDefaultManifestName() (string, error) {
	name, err := r.readDefaultManifestName()
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return name, err
}

// observeContext never initializes the Context catalog. A synthetic default is
// display state only and carries no stable authority manifest.
func (r *Runtime) observeContext(name string) (observedContext, error) {
	explicit := name != ""
	if !explicit {
		var err error
		name, err = r.observeDefaultManifestName()
		if err != nil {
			return observedContext{}, err
		}
		if name == "" {
			name = tobari.DefaultManifestName
		}
	}
	manifest, err := r.readContextManifestRaw(name)
	if errors.Is(err, tobari.ErrContextNotFound) {
		if explicit || name != tobari.DefaultManifestName {
			return observedContext{}, err
		}
		return observedContext{
			state: tobari.ManifestObservationAbsent,
			manifest: tobari.WorkspaceManifest{
				SchemaVersion: tobari.WorkspaceManifestSchemaVersion, Name: tobari.DefaultManifestName,
				AgentProfile: tobari.DefaultProfile, Image: tobari.BuiltinImageSelector, PolicyMode: tobari.ManifestPolicyModeGuided,
				SourceAccess:     tobari.ManifestSourceAccessReadWrite,
				ShellEnvironment: tobari.InitialContextShellEnvironment(),
			},
		}, nil
	}
	if err != nil {
		return observedContext{}, err
	}
	return observedContext{state: tobari.ManifestObservationPersisted, manifest: manifest}, nil
}

// ObserveContext exposes only a validated stable manifest as authority.
func (r *Runtime) ObserveContext(ctx context.Context, name string) (tobari.ManifestObservation, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestObservation{}, err
	}
	observed, err := r.observeContext(name)
	if err != nil {
		return tobari.ManifestObservation{}, err
	}
	result := tobari.ManifestObservation{State: observed.state}
	if observed.state == tobari.ManifestObservationPersisted {
		manifest := observed.manifest
		result.Name = manifest.Name
		result.Manifest = &manifest
	}
	return result, result.Validate()
}

// resolveContext resolves an explicit display name to its trusted manifest, or
// uses the current Context only when the caller omitted a name.
func (r *Runtime) resolveContext(name string) (tobari.WorkspaceManifest, tobari.ManifestStorePaths, error) {
	if name == "" {
		return r.activeContext()
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	paths := r.contextPaths(name)
	if err := paths.Validate(); err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	return manifest, paths, nil
}

func (r *Runtime) contextByID(id string) (tobari.WorkspaceManifest, tobari.ManifestStorePaths, error) {
	if err := tobari.ValidateWorkspaceManifestID(id); err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
	}
	var selected *tobari.WorkspaceManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, err := r.readContextManifest(entry.Name())
		if err != nil {
			return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, err
		}
		if manifest.ID != id {
			continue
		}
		if selected != nil {
			return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, fmt.Errorf("Context ID is ambiguous")
		}
		copy := manifest
		selected = &copy
	}
	if selected == nil {
		return tobari.WorkspaceManifest{}, tobari.ManifestStorePaths{}, fmt.Errorf("%w: Context ID", tobari.ErrContextNotFound)
	}
	return *selected, r.contextPaths(selected.Name), nil
}

func (r *Runtime) contextByIDRaw(id string) (tobari.WorkspaceManifest, error) {
	if err := tobari.ValidateWorkspaceManifestID(id); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	var selected *tobari.WorkspaceManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifest, readErr := r.readContextManifestRaw(entry.Name())
		if readErr != nil {
			return tobari.WorkspaceManifest{}, readErr
		}
		if manifest.ID == id {
			if selected != nil {
				return tobari.WorkspaceManifest{}, fmt.Errorf("Workspace Manifest ID is ambiguous")
			}
			copy := manifest
			selected = &copy
		}
	}
	if selected == nil {
		return tobari.WorkspaceManifest{}, fmt.Errorf("%w: Workspace Manifest ID", tobari.ErrContextNotFound)
	}
	return *selected, nil
}

func (r *Runtime) ResolveContext(ctx context.Context, name string) (tobari.WorkspaceManifest, error) {
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceManifest{}, err
	}
	manifest, _, err := r.resolveContext(name)
	return manifest, err
}

// ListContexts returns the complete host-owned Context collection.
func (r *Runtime) ListContexts(ctx context.Context) (tobari.ManifestListResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestListResult{}, err
	}
	active, err := r.observeDefaultManifestName()
	if err != nil {
		return tobari.ManifestListResult{}, err
	}
	entries, err := os.ReadDir(r.contextsDirectory())
	if errors.Is(err, os.ErrNotExist) {
		result := tobari.ManifestListResult{
			Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
			Items: []tobari.ManifestSummary{},
		}
		return result, result.Validate()
	}
	if err != nil {
		return tobari.ManifestListResult{}, err
	}
	items := make([]tobari.ManifestSummary, 0)
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return tobari.ManifestListResult{}, fmt.Errorf("Context directory contains a symbolic link")
		}
		if !entry.IsDir() {
			continue
		}
		manifest, err := r.readContextManifestRaw(entry.Name())
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		runtimeReport, err := r.contextRuntimeReport(manifest)
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		runtimeSelection, err := runtimeReport.Selection()
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		nativeReadiness, err := tobari.ResolveContextNativeReadiness(manifest.NativeReadiness)
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		policy, err := r.readContextPolicy(manifest)
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		routineAccess, err := tobari.SummarizeContextAccess(policy, manifest.SourceAccess, nativeReadiness)
		if err != nil {
			return tobari.ManifestListResult{}, err
		}
		items = append(items, tobari.ManifestSummary{
			ID: manifest.ID, Name: manifest.Name, ManifestState: tobari.ManifestObservationPersisted, Default: manifest.Name == active,
			Desired:      manifest.Desired,
			AgentProfile: manifest.AgentProfile, Image: manifest.Image, PolicyMode: manifest.PolicyMode,
			SourceAccess:     manifest.SourceAccess,
			PolicyRevision:   manifest.PolicyRevision,
			NativeReadiness:  nativeReadiness,
			MethodPolicy:     policy.MethodPolicy,
			RoutineAccess:    &routineAccess,
			RuntimeStatus:    runtimeReport.Status,
			RuntimeSelection: runtimeSelection,
			Bootstrap:        tobari.ManifestBootstrapReportFrom(manifest.Bootstrap),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	activeState := tobari.ManifestObservationAbsent
	defaultManifestID := ""
	for _, item := range items {
		if item.Default {
			activeState = item.ManifestState
			defaultManifestID = item.ID
			break
		}
	}
	result := tobari.ManifestListResult{Task: tobari.TaskManifestList, ManifestState: activeState, DefaultManifestID: defaultManifestID, DefaultManifest: active, Items: items}
	if err := result.Validate(); err != nil {
		return tobari.ManifestListResult{}, err
	}
	return result, nil
}

// ShowContext returns one named Context, or the active Context when name is empty.
func (r *Runtime) ShowContext(ctx context.Context, name string) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	observed, err := r.observeContext(name)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	active, err := r.observeDefaultManifestName()
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	if observed.state == tobari.ManifestObservationPersisted {
		return r.contextReport(ctx, tobari.TaskManifestShow, observed.manifest, active)
	}
	return r.nonPersistedContextReport(observed, active)
}

// ManifestCopySnapshot returns only the complete Context-owned settings whose
// lifetime permits initializing another standalone Context. Workspace homes,
// authentication, learned domain decisions, attachment state, and current
// selection never enter this snapshot.
func (r *Runtime) ManifestCopySnapshot(ctx context.Context, name string) (tobari.ManifestCopySnapshot, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	policy, _, _, _, baseRevision, err := r.contextCreateBaseMaterial(manifest)
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	runtimeReport, err := r.contextRuntimeReport(manifest)
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	runtimeSelection, err := runtimeReport.Selection()
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	shell, err := tobari.CompleteContextShellEnvironment(manifest.ShellEnvironment)
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	gitIdentity := tobari.DefaultContextGitIdentityReport()
	if manifest.GitIdentity != nil {
		gitIdentity = *manifest.GitIdentity
	}
	readiness, err := tobari.ResolveContextNativeReadiness(manifest.NativeReadiness)
	if err != nil {
		return tobari.ManifestCopySnapshot{}, err
	}
	base := tobari.ManifestCopySnapshot{
		ID: manifest.ID, Name: manifest.Name, Revision: baseRevision, Desired: manifest.Desired,
		PolicyMode: manifest.PolicyMode, SourceAccess: manifest.SourceAccess,
		NativeReadiness: readiness, MethodPolicy: policy.MethodPolicy,
		RuntimeSelection: runtimeSelection, RuntimeBinding: *manifest.RuntimeBinding, ShellEnvironment: shell,
		GitIdentity: gitIdentity, Bootstrap: manifest.Bootstrap,
	}
	return base.Clone(), base.Validate()
}

func (r *Runtime) contextCreateBaseMaterial(
	manifest tobari.WorkspaceManifest,
) (tobari.ManifestPolicy, []byte, map[string][]byte, string, string, error) {
	if err := manifest.ValidatePublished(); err != nil {
		return tobari.ManifestPolicy{}, nil, nil, "", "", err
	}
	policyPath := r.contextPolicyPath(manifest.Name)
	policyBytes, err := readOwnerPolicyFile(policyPath, maxContextPolicyBytes)
	if err != nil {
		return tobari.ManifestPolicy{}, nil, nil, "", "", err
	}
	policy, normalized, revision, err := decodeContextPolicy(policyBytes)
	if err != nil || revision != manifest.PolicyRevision || !bytes.Equal(policyBytes, normalized) {
		return tobari.ManifestPolicy{}, nil, nil, "", "", fmt.Errorf("Context create Base policy is invalid")
	}
	advanced := map[string][]byte{}
	if manifest.PolicyMode == tobari.ManifestPolicyModeAdvanced {
		for _, name := range []string{"tobari.rego", "tobari_test.rego"} {
			data, readErr := readOwnerPolicyFile(filepath.Join(r.contextPolicyDirectory(manifest.Name), name), maxPolicyPreflight)
			if readErr != nil {
				return tobari.ManifestPolicy{}, nil, nil, "", "", readErr
			}
			advanced[name] = data
		}
	}
	return policy, normalized, advanced, revision, manifest.Desired.Revision, nil
}

// ConfigureContextShell atomically updates one staged set of distinct
// allowlisted shell environment settings in an explicit or current Context. Inherited values are deliberately
// resolved later at session entry rather than persisted here.
func (r *Runtime) ConfigureContextShell(
	ctx context.Context, name string, changes []tobari.ManifestShellEnvironmentSetting,
) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if _, err := tobari.ApplyContextShellEnvironmentSettings(nil, changes); err != nil {
		return tobari.ManifestReport{}, err
	}
	var result tobari.ManifestReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readDefaultManifestName()
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
		previous := manifest
		settings, err := tobari.ApplyContextShellEnvironmentSettings(manifest.ShellEnvironment, changes)
		if err != nil {
			return err
		}
		manifest.ShellEnvironment = settings
		manifest, err = r.publishWorkspaceManifestUpdate(previous, manifest)
		if err != nil {
			return fmt.Errorf("write Context shell environment: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigShell, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

// ConfigureContextGit atomically updates the Context-owned Git identity pair.
// Selecting default removes the persisted override; inherited host values are
// resolved later for a specific Workspace root and never stored here.
func (r *Runtime) ConfigureContextGit(
	ctx context.Context, name string, change tobari.ManifestGitIdentitySetting,
) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if err := change.Validate(true); err != nil {
		return tobari.ManifestReport{}, err
	}
	var result tobari.ManifestReport
	err := r.withContextStoreLock(func() error {
		active, err := r.readDefaultManifestName()
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
		previous := manifest
		manifest.GitIdentity = persistedContextGitIdentity(change)
		manifest, err = r.publishWorkspaceManifestUpdate(previous, manifest)
		if err != nil {
			return fmt.Errorf("write Context Git identity: %w", err)
		}
		result, err = r.contextReport(ctx, tobari.TaskConfigGit, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func persistedContextGitIdentity(setting tobari.ManifestGitIdentitySetting) *tobari.ManifestGitIdentitySetting {
	if setting.Source == tobari.ManifestGitIdentityDefault {
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

func cloneShellEnvironmentManifest(settings []tobari.ManifestShellEnvironmentSetting) []tobari.ManifestShellEnvironmentSetting {
	result := make([]tobari.ManifestShellEnvironmentSetting, len(settings))
	for index, setting := range settings {
		result[index] = setting
		if setting.Value != nil {
			value := *setting.Value
			result[index].Value = &value
		}
	}
	return result
}

func cloneGitIdentityManifest(setting *tobari.ManifestGitIdentitySetting) *tobari.ManifestGitIdentitySetting {
	if setting == nil {
		return nil
	}
	result := *setting
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
	ctx context.Context, name string, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess,
) (tobari.ManifestReport, error) {
	return r.CreateContextWithReadiness(ctx, name, image, mode, sourceAccess, tobari.ManifestNativeReadinessEnabled)
}

func (r *Runtime) CreateContextWithReadiness(
	ctx context.Context, name string, image string, mode tobari.ManifestPolicyMode, sourceAccess tobari.ManifestSourceAccess, readinessSelections ...tobari.ManifestNativeReadiness,
) (tobari.ManifestReport, error) {
	nativeReadiness := tobari.ManifestNativeReadinessEnabled
	if len(readinessSelections) > 1 {
		return tobari.ManifestReport{}, fmt.Errorf("Context native readiness selection is invalid")
	}
	if len(readinessSelections) == 1 {
		nativeReadiness = readinessSelections[0]
	}
	return r.CreateContextWithComposition(ctx, name, image, mode, sourceAccess, tobari.ManifestCreateComposition{
		NativeReadiness: nativeReadiness,
	})
}

func (r *Runtime) CreateContextWithComposition(
	ctx context.Context,
	name string,
	image string,
	mode tobari.ManifestPolicyMode,
	sourceAccess tobari.ManifestSourceAccess,
	composition tobari.ManifestCreateComposition,
) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	if err := composition.Validate(); err != nil {
		return tobari.ManifestReport{}, err
	}
	runtimeBinding, err := r.resolveRuntimeBinding(composition.RuntimeSelection)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	var baseManifest tobari.WorkspaceManifest
	var advancedPolicy map[string][]byte
	policy, normalizedPolicy, policyRevision, err := defaultContextPolicyBytes()
	if composition.CopyFrom != nil {
		if composition.RuntimeSelection == composition.CopyFrom.RuntimeSelection {
			runtimeBinding = composition.CopyFrom.RuntimeBinding
		}
		baseManifest, err = r.readContextManifest(composition.CopyFrom.Name)
		if err == nil {
			var baseRevision string
			policy, normalizedPolicy, advancedPolicy, policyRevision, baseRevision, err = r.contextCreateBaseMaterial(baseManifest)
			if err == nil && (baseManifest.ID != composition.CopyFrom.ID || baseRevision != composition.CopyFrom.Revision ||
				baseManifest.Desired != composition.CopyFrom.Desired) {
				err = tobari.ErrManifestCopySourceChanged
			}
		}
	}
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	if composition.MethodPolicy != nil {
		policy, err = tobari.ComposeContextMethodPolicy(policy, composition.MethodPolicy.Clone())
		if err != nil {
			return tobari.ManifestReport{}, err
		}
		policy, normalizedPolicy, policyRevision, err = tobari.NormalizeContextPolicy(policy)
		if err != nil {
			return tobari.ManifestReport{}, err
		}
	}
	if mode != tobari.ManifestPolicyModeAdvanced || baseManifest.PolicyMode != tobari.ManifestPolicyModeAdvanced {
		advancedPolicy = nil
	}
	manifest := tobari.WorkspaceManifest{
		SchemaVersion: tobari.WorkspaceManifestSchemaVersion, Name: name,
		AgentProfile: tobari.DefaultProfile, Image: image, PolicyMode: mode,
		SourceAccess:     sourceAccess,
		PolicyRevision:   policyRevision,
		NativeReadiness:  composition.NativeReadiness,
		ShellEnvironment: tobari.InitialContextShellEnvironment(), Bootstrap: composition.Bootstrap,
		RuntimeBinding: &runtimeBinding,
	}
	if composition.CopyFrom != nil {
		manifest.AgentProfile = baseManifest.AgentProfile
		manifest.ShellEnvironment = cloneShellEnvironmentManifest(baseManifest.ShellEnvironment)
		manifest.GitIdentity = cloneGitIdentityManifest(baseManifest.GitIdentity)
		if composition.Bootstrap == nil && baseManifest.Bootstrap != nil {
			bootstrap := baseManifest.Bootstrap.Clone()
			manifest.Bootstrap = &bootstrap
		}
	}
	id, err := r.identities.newWorkspaceManifestID()
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	manifest.ID = id
	manifest, err = tobari.PublishWorkspaceManifest(manifest, nil)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	var active string
	if err := r.withContextStoreLock(func() error {
		if composition.CopyFrom != nil {
			current, readErr := r.readContextManifest(composition.CopyFrom.Name)
			if readErr != nil {
				return readErr
			}
			_, _, _, _, revision, readErr := r.contextCreateBaseMaterial(current)
			if readErr != nil {
				return readErr
			}
			if current.ID != composition.CopyFrom.ID || revision != composition.CopyFrom.Revision ||
				current.Desired != composition.CopyFrom.Desired {
				return tobari.ErrManifestCopySourceChanged
			}
		}
		if _, err := os.Lstat(r.contextManifestPath(name)); err == nil {
			return tobari.ErrContextExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		if name == tobari.DefaultManifestName {
			if err := r.installContextWithCreationSnapshot(manifest, normalizedPolicy, advancedPolicy); err != nil {
				return err
			}
			if err := r.writeDefaultManifest(manifest); err != nil {
				return err
			}
		} else {
			if err := r.installContextWithCreationSnapshot(manifest, normalizedPolicy, advancedPolicy); err != nil {
				return err
			}
		}

		var err error
		active, err = r.observeDefaultManifestName()
		return err
	}); err != nil {
		return tobari.ManifestReport{}, err
	}
	report, err := r.contextReport(ctx, tobari.TaskManifestCreate, manifest, active)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	if _, configured, loadErr := r.LoadState(ctx); loadErr != nil {
		return tobari.ManifestReport{}, loadErr
	} else if configured {
		report.Cluster = tobari.ManifestClusterStatusRequiresReconcile
	}
	return report, report.Validate()
}

// DeleteContext removes one exact non-current Context store after proving that
// no durable Workspace remains bound to its stable Context identity. The
// caller holds the installation lifecycle lock across this check-and-delete.
func (r *Runtime) DeleteContext(ctx context.Context, name string) (tobari.ManifestDeleteResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	if err := tobari.ValidateName(name); err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	if name == tobari.DefaultManifestName {
		return tobari.ManifestDeleteResult{}, tobari.ErrContextProtected
	}
	active, err := r.observeDefaultManifestName()
	if err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	if active == name {
		return tobari.ManifestDeleteResult{}, tobari.ErrContextActive
	}
	projects, err := r.ListProjects(ctx)
	if err != nil {
		return tobari.ManifestDeleteResult{}, err
	}
	for _, project := range projects {
		if project.WorkspaceManifestID == manifest.ID || project.WorkspaceManifestName == name {
			return tobari.ManifestDeleteResult{}, tobari.ErrContextHasWorkspaces
		}
	}
	cluster := tobari.ManifestClusterStatusNotApplicable
	if _, configured, err := r.LoadState(ctx); err != nil {
		return tobari.ManifestDeleteResult{}, err
	} else if configured {
		cluster = tobari.ManifestClusterStatusRequiresReconcile
	}
	if err := r.withContextStoreLock(func() error {
		current, err := r.readContextManifest(name)
		if err != nil {
			return err
		}
		if current.ID != manifest.ID {
			return fmt.Errorf("Context identity changed before deletion")
		}
		active, err := r.readDefaultManifestName()
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
		return tobari.ManifestDeleteResult{}, err
	}
	result := tobari.ManifestDeleteResult{
		Task: tobari.TaskManifestDelete, ID: manifest.ID, Name: manifest.Name, Deleted: true, Cluster: cluster,
	}
	return result, result.Validate()
}

// SetDefaultManifest changes only the default used when execution omits a Context.
func (r *Runtime) SetDefaultManifest(ctx context.Context, name string) (tobari.ManifestReport, error) {
	return r.SetDefaultManifestWithProgress(ctx, name, nil)
}

// SetDefaultManifestWithProgress keeps default selection independent from cluster
// reconciliation while satisfying the progress-aware application port.
func (r *Runtime) SetDefaultManifestWithProgress(
	ctx context.Context, name string, progress tobari.ClusterUpProgressSink,
) (tobari.ManifestReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ManifestReport{}, err
	}
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	active, err := r.observeDefaultManifestName()
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	_ = progress
	if err := r.selectContext(active, name); err != nil {
		return tobari.ManifestReport{}, err
	}
	status := tobari.ManifestClusterStatusDefaultManifestUpdated
	if active == name {
		status = tobari.ManifestClusterStatusAlreadyReady
	}
	return r.manifestDefaultSetReport(ctx, manifest, name, status)
}

func (r *Runtime) manifestDefaultSetReport(
	ctx context.Context, manifest tobari.WorkspaceManifest, active string,
	clusterStatus tobari.ManifestClusterStatus,
) (tobari.ManifestReport, error) {
	result, err := r.contextReport(ctx, tobari.TaskManifestDefaultSet, manifest, active)
	if err != nil {
		return tobari.ManifestReport{}, err
	}
	result.Cluster = clusterStatus
	if err := result.Validate(); err != nil {
		return tobari.ManifestReport{}, err
	}
	return result, nil
}

func (r *Runtime) selectContext(previous, next string) error {
	if previous == next {
		return nil
	}
	manifest, err := r.readContextManifest(next)
	if err != nil {
		return err
	}
	return r.writeDefaultManifest(manifest)
}

func (r *Runtime) restoreContextSelection(name string, state tobari.State) error {
	manifest, err := r.readContextManifest(name)
	if err != nil {
		return fmt.Errorf("previous Context is unavailable: %w", err)
	}
	if err := r.writeDefaultManifest(manifest); err != nil {
		return fmt.Errorf("restore active Context marker: %w", err)
	}
	if err := r.writeState(state); err != nil {
		return fmt.Errorf("restore shared state: %w", err)
	}
	return nil
}

// DefaultManifestName exposes only the trusted selected name to the application
// layer so it can refuse to enter a cluster whose policy is stale.
func (r *Runtime) DefaultManifestName(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	manifest, _, err := r.activeContext()
	if err != nil {
		return "", err
	}
	return manifest.Name, nil
}
