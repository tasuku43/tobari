package workspaceauthoritystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	authorityStoreSchemaVersion = 2
	activeFileName              = "active.json"
	legacyAuthorityFileName     = "authority.json"
	maxPointerBytes             = 4 << 10
	maxManifestBytes            = 16 << 20
)

var authorityConceptDirectories = []string{"contexts", "generations", "journal", "policy-memory", "templates", "tombstones", "workspaces"}

type activeGenerationPointer struct {
	SchemaVersion    int    `json:"schema_version"`
	GenerationDigest string `json:"generation_digest"`
}

type authorityObjectRef struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Path     string `json:"path"`
	Digest   string `json:"digest"`
}

type contextGenerationRef struct {
	Context            authorityObjectRef  `json:"context"`
	PolicyMemory       authorityObjectRef  `json:"policy_memory"`
	ActivePolicyMemory *authorityObjectRef `json:"active_policy_memory,omitempty"`
}

type authorityGenerationManifest struct {
	SchemaVersion                   int                               `json:"schema_version"`
	Generation                      uint64                            `json:"generation"`
	CollectionRevision              tobari.SemanticDigest             `json:"collection_revision"`
	InstallationMigrationProvenance *tobari.InstallationMigrationPlan `json:"installation_migration_provenance,omitempty"`
	Templates                       []authorityObjectRef              `json:"templates"`
	Contexts                        []contextGenerationRef            `json:"contexts"`
	Workspaces                      []authorityObjectRef              `json:"workspaces"`
	PendingCandidates               []tobari.PolicyCandidateAuthority `json:"pending_candidates"`
	DefaultTemplateID               *tobari.WorkspaceTemplateID       `json:"default_workspace_template_id,omitempty"`
	CurrentContextID                *tobari.ContextID                 `json:"current_context_id,omitempty"`
}

type contextAuthorityObject struct {
	SchemaVersion         int                                     `json:"schema_version"`
	Context               tobari.ContextBinding                   `json:"context"`
	ContextHome           string                                  `json:"context_home,omitempty"`
	CreationDefaults      tobari.SemanticDigest                   `json:"creation_defaults_digest,omitempty"`
	ActiveTemplatePolicy  *tobari.TemplatePolicyActivationReceipt `json:"active_template_policy,omitempty"`
	ActivePolicyMemoryRef *tobari.PolicyMemoryActivationReceipt   `json:"active_policy_memory_receipt,omitempty"`
	SupersededAllows      []tobari.PolicyMemoryAllowSupersession  `json:"superseded_policy_memory_allows,omitempty"`
	SupersededCandidates  []tobari.PolicyCandidateSupersession    `json:"superseded_policy_candidates,omitempty"`
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestFileComponent(digest string) (string, error) {
	if len(digest) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(digest, "sha256:") {
		return "", fmt.Errorf("authority digest is invalid")
	}
	component := strings.TrimPrefix(digest, "sha256:")
	if _, err := hex.DecodeString(component); err != nil {
		return "", fmt.Errorf("authority digest is invalid: %w", err)
	}
	return component, nil
}

func encodeAuthorityObject(value any) ([]byte, string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 || len(data) > MaxAuthorityBytes {
		return nil, "", fmt.Errorf("authority object exceeds bounded size")
	}
	return data, digestBytes(data), nil
}

func objectRef(concept, id string, value any) (authorityObjectRef, []byte, error) {
	data, digest, err := encodeAuthorityObject(value)
	if err != nil {
		return authorityObjectRef{}, nil, err
	}
	component, err := digestFileComponent(digest)
	if err != nil {
		return authorityObjectRef{}, nil, err
	}
	return authorityObjectRef{ID: id, Revision: digest, Path: filepath.ToSlash(filepath.Join(concept, id, component+".json")), Digest: digest}, data, nil
}

type preparedAuthorityGeneration struct {
	manifest       authorityGenerationManifest
	manifestData   []byte
	manifestDigest string
	pointerData    []byte
	objects        map[string][]byte
}

func prepareAuthorityGeneration(collection tobari.WorkspaceAuthorityCollection) (preparedAuthorityGeneration, error) {
	return prepareAuthorityGenerationWithMigrationProvenance(collection, nil)
}

func prepareAuthorityGenerationWithMigrationProvenance(collection tobari.WorkspaceAuthorityCollection, provenance *tobari.InstallationMigrationPlan) (preparedAuthorityGeneration, error) {
	if err := collection.Validate(); err != nil {
		return preparedAuthorityGeneration{}, err
	}
	if provenance != nil && (provenance.Validate() != nil || !installationMigrationPlanMatchesCollection(*provenance, collection)) {
		return preparedAuthorityGeneration{}, fmt.Errorf("installation migration provenance does not match the target collection")
	}
	prepared := preparedAuthorityGeneration{objects: map[string][]byte{}}
	manifest := authorityGenerationManifest{
		SchemaVersion: authorityStoreSchemaVersion, Generation: collection.Generation,
		CollectionRevision: collection.Revision,
		Templates:          []authorityObjectRef{}, Contexts: []contextGenerationRef{}, Workspaces: []authorityObjectRef{},
		PendingCandidates: make([]tobari.PolicyCandidateAuthority, len(collection.PendingCandidates)),
	}
	copy(manifest.PendingCandidates, collection.PendingCandidates)
	if provenance != nil {
		value := *provenance
		manifest.InstallationMigrationProvenance = &value
	}
	if collection.DefaultTemplateID != nil {
		value := *collection.DefaultTemplateID
		manifest.DefaultTemplateID = &value
	}
	if collection.CurrentContextID != nil {
		value := *collection.CurrentContextID
		manifest.CurrentContextID = &value
	}
	for _, template := range collection.Templates {
		ref, data, err := objectRef("templates", string(template.ID), template)
		if err != nil {
			return prepared, err
		}
		manifest.Templates = append(manifest.Templates, ref)
		prepared.objects[ref.Path] = data
	}
	for _, record := range collection.Contexts {
		contextObject := contextAuthorityObject{SchemaVersion: authorityStoreSchemaVersion, Context: record.Context, ContextHome: record.ContextHome, CreationDefaults: record.CreationDefaults, ActiveTemplatePolicy: record.ActiveTemplatePolicy, ActivePolicyMemoryRef: record.ActivePolicyMemoryRef, SupersededAllows: record.SupersededAllows, SupersededCandidates: record.SupersededCandidates}
		contextRef, contextData, err := objectRef("contexts", string(record.Context.ID), contextObject)
		if err != nil {
			return prepared, err
		}
		memoryRef, memoryData, err := objectRef("policy-memory", string(record.Context.ID), record.PolicyMemory)
		if err != nil {
			return prepared, err
		}
		entry := contextGenerationRef{Context: contextRef, PolicyMemory: memoryRef}
		prepared.objects[contextRef.Path] = contextData
		prepared.objects[memoryRef.Path] = memoryData
		if record.ActivePolicyMemory != nil {
			activeRef, activeData, err := objectRef("policy-memory", string(record.Context.ID), record.ActivePolicyMemory)
			if err != nil {
				return prepared, err
			}
			entry.ActivePolicyMemory = &activeRef
			prepared.objects[activeRef.Path] = activeData
		}
		manifest.Contexts = append(manifest.Contexts, entry)
	}
	for _, workspace := range collection.Workspaces {
		ref, data, err := objectRef("workspaces", string(workspace.ID), workspace)
		if err != nil {
			return prepared, err
		}
		manifest.Workspaces = append(manifest.Workspaces, ref)
		prepared.objects[ref.Path] = data
	}
	manifestData, manifestDigest, err := encodeAuthorityObject(manifest)
	if err != nil {
		return prepared, err
	}
	pointerData, _, err := encodeAuthorityObject(activeGenerationPointer{SchemaVersion: authorityStoreSchemaVersion, GenerationDigest: manifestDigest})
	if err != nil {
		return prepared, err
	}
	prepared.manifest, prepared.manifestData, prepared.manifestDigest, prepared.pointerData = manifest, manifestData, manifestDigest, pointerData
	return prepared, nil
}

func (s *Store) readGenerationRaw(ctx context.Context) (tobari.WorkspaceAuthorityCollection, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	rootInfo, err := os.Lstat(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceAuthorityCollection{}, false, nil
	}
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := validateOwnedDirectoryInfo(rootInfo); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority root: %w", err)
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if len(entries) == 1 && entries[0].Name() == legacyAuthorityFileName {
		legacyData, readErr := readAuthorityFile(filepath.Join(s.root, legacyAuthorityFileName))
		if readErr != nil {
			return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("%w: %v", tobari.ErrPreReleaseLegacyAuthority, readErr)
		}
		if markerErr := rejectLegacyAdvancedAuthorityBytes(legacyData); markerErr != nil {
			return tobari.WorkspaceAuthorityCollection{}, false, markerErr
		}
		var legacy tobari.WorkspaceAuthorityCollection
		if decodeErr := decodeStrictJSON(legacyData, &legacy); decodeErr != nil {
			return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("%w: unsupported authority.json: %v", tobari.ErrPreReleaseLegacyAuthority, decodeErr)
		}
		if validationErr := legacy.Validate(); validationErr != nil {
			return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("%w: unsupported authority.json: %v", tobari.ErrPreReleaseLegacyAuthority, validationErr)
		}
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("%w: explicit stale-bound Plan and Apply are required", tobari.ErrFinalAuthorityMigrationRequired)
	}
	wantNames := append(append([]string(nil), authorityConceptDirectories...), activeFileName)
	sort.Strings(wantNames)
	gotNames := make([]string, len(entries))
	hasActive := false
	knownConcepts := make(map[string]struct{}, len(authorityConceptDirectories))
	for _, name := range authorityConceptDirectories {
		knownConcepts[name] = struct{}{}
	}
	for i, entry := range entries {
		gotNames[i] = entry.Name()
		hasActive = hasActive || entry.Name() == activeFileName
	}
	sort.Strings(gotNames)
	if !hasActive {
		conceptNames := append([]string(nil), authorityConceptDirectories...)
		sort.Strings(conceptNames)
		ownedInitialStage := len(gotNames) == 1 && gotNames[0] == "journal"
		if !ownedInitialStage && !equalStrings(gotNames, conceptNames) {
			return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority store is partial or mixed")
		}
		for _, entry := range entries {
			if _, known := knownConcepts[entry.Name()]; !known {
				return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority store is partial or mixed")
			}
			info, inspectErr := os.Lstat(filepath.Join(s.root, entry.Name()))
			if inspectErr != nil || validateOwnedDirectoryInfo(info) != nil {
				return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority initial publication is unsafe")
			}
		}
		if ownedInitialStage {
			journalEntries, readErr := os.ReadDir(filepath.Join(s.root, "journal"))
			if readErr != nil || len(journalEntries) > 1 || len(journalEntries) == 1 && journalEntries[0].Name() != filepath.Base(mutationStagePath(s.root)) {
				return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority initial publication journal is partial or mixed")
			}
		}
		return tobari.WorkspaceAuthorityCollection{}, false, nil
	}
	if !equalStrings(gotNames, wantNames) {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority store is partial or mixed")
	}
	for _, name := range authorityConceptDirectories {
		info, err := os.Lstat(filepath.Join(s.root, name))
		if err != nil || validateOwnedDirectoryInfo(info) != nil {
			return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority %s directory is unsafe", name)
		}
	}
	pointerData, err := readBoundedOwnedFile(filepath.Join(s.root, activeFileName), maxPointerBytes)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	var pointer activeGenerationPointer
	if err := decodeStrictJSON(pointerData, &pointer); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("decode authority active pointer: %w", err)
	}
	if pointer.SchemaVersion != authorityStoreSchemaVersion {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("%w: final generation store schema %d is unsupported; reset and recreate this pre-release installation", tobari.ErrPreReleaseLegacyAuthority, pointer.SchemaVersion)
	}
	component, err := digestFileComponent(pointer.GenerationDigest)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	manifestData, err := readBoundedOwnedFile(filepath.Join(s.root, "generations", component+".json"), maxManifestBytes)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if digestBytes(manifestData) != pointer.GenerationDigest {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("authority generation digest mismatch")
	}
	var manifest authorityGenerationManifest
	if err := decodeStrictJSON(manifestData, &manifest); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	collection, err := s.collectionFromManifest(ctx, manifest)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	return collection, true, nil
}

func (s *Store) collectionFromManifest(ctx context.Context, manifest authorityGenerationManifest) (tobari.WorkspaceAuthorityCollection, error) {
	if manifest.SchemaVersion != authorityStoreSchemaVersion || manifest.Generation == 0 || manifest.CollectionRevision.Validate() != nil || manifest.Templates == nil || manifest.Contexts == nil || manifest.Workspaces == nil || manifest.PendingCandidates == nil {
		return tobari.WorkspaceAuthorityCollection{}, fmt.Errorf("authority generation manifest is invalid")
	}
	collection := tobari.WorkspaceAuthorityCollection{SchemaVersion: tobari.WorkspaceAuthorityCollectionSchemaVersion, Generation: manifest.Generation, Revision: manifest.CollectionRevision, Templates: []tobari.WorkspaceTemplate{}, Contexts: []tobari.WorkspaceAuthorityContextRecord{}, Workspaces: []tobari.WorkspaceBinding{}, PendingCandidates: make([]tobari.PolicyCandidateAuthority, len(manifest.PendingCandidates))}
	copy(collection.PendingCandidates, manifest.PendingCandidates)
	if manifest.DefaultTemplateID != nil {
		value := *manifest.DefaultTemplateID
		collection.DefaultTemplateID = &value
	}
	if manifest.CurrentContextID != nil {
		value := *manifest.CurrentContextID
		collection.CurrentContextID = &value
	}
	for _, ref := range manifest.Templates {
		var value tobari.WorkspaceTemplate
		if err := s.readTypedObject(ctx, "templates", ref.ID, ref, &value); err != nil {
			return collection, err
		}
		if string(value.ID) != ref.ID {
			return collection, fmt.Errorf("Template object identity mismatch")
		}
		collection.Templates = append(collection.Templates, value)
	}
	for _, refs := range manifest.Contexts {
		var object contextAuthorityObject
		if err := s.readTypedObject(ctx, "contexts", refs.Context.ID, refs.Context, &object); err != nil {
			return collection, err
		}
		if object.SchemaVersion != authorityStoreSchemaVersion || string(object.Context.ID) != refs.Context.ID {
			return collection, fmt.Errorf("Context object identity mismatch")
		}
		var memory tobari.PolicyMemoryRevision
		if err := s.readTypedObject(ctx, "policy-memory", refs.Context.ID, refs.PolicyMemory, &memory); err != nil {
			return collection, err
		}
		record := tobari.WorkspaceAuthorityContextRecord{Context: object.Context, ContextHome: object.ContextHome, CreationDefaults: object.CreationDefaults, PolicyMemory: memory, ActiveTemplatePolicy: object.ActiveTemplatePolicy, ActivePolicyMemoryRef: object.ActivePolicyMemoryRef, SupersededAllows: object.SupersededAllows, SupersededCandidates: object.SupersededCandidates}
		if refs.ActivePolicyMemory != nil {
			var active tobari.PolicyMemoryRevision
			if err := s.readTypedObject(ctx, "policy-memory", refs.Context.ID, *refs.ActivePolicyMemory, &active); err != nil {
				return collection, err
			}
			record.ActivePolicyMemory = &active
		}
		collection.Contexts = append(collection.Contexts, record)
	}
	for _, ref := range manifest.Workspaces {
		var value tobari.WorkspaceBinding
		if err := s.readTypedObject(ctx, "workspaces", ref.ID, ref, &value); err != nil {
			return collection, err
		}
		if string(value.ID) != ref.ID {
			return collection, fmt.Errorf("Workspace object identity mismatch")
		}
		collection.Workspaces = append(collection.Workspaces, value)
	}
	// Schema-v2 Context objects written before Context Home became an explicit
	// Context-owned fact omit these fields. Reconstruct only the exact mirrors
	// already bound by a retained Workspace; zero-Workspace Contexts cannot be
	// widened or guessed.
	contextIndexes := make(map[tobari.ContextID]int, len(collection.Contexts))
	for index := range collection.Contexts {
		contextIndexes[collection.Contexts[index].Context.ID] = index
	}
	for _, workspace := range collection.Workspaces {
		index, found := contextIndexes[workspace.ContextID]
		if !found || collection.Contexts[index].ContextHome != "" || collection.Contexts[index].CreationDefaults != "" {
			continue
		}
		collection.Contexts[index].ContextHome = workspace.Home
		collection.Contexts[index].CreationDefaults = workspace.CreationDefaults
	}
	if err := validateCollectionBounds(collection); err != nil {
		return collection, err
	}
	if err := collection.Validate(); err != nil {
		return collection, err
	}
	if manifest.InstallationMigrationProvenance != nil {
		if err := manifest.InstallationMigrationProvenance.Validate(); err != nil || !installationMigrationPlanMatchesCollection(*manifest.InstallationMigrationProvenance, collection) {
			return collection, fmt.Errorf("authority generation migration provenance is invalid")
		}
	}
	return collection.Clone(), nil
}

// readActiveInstallationMigrationProvenance reselects and verifies the exact
// content-addressed active manifest around a complete object read. A receipt
// can therefore settle only against provenance committed before the migration
// swap; receipt bytes can never introduce or replace that authority.
func (s *Store) readActiveInstallationMigrationProvenance(ctx context.Context, expected tobari.WorkspaceAuthorityCollection) (tobari.InstallationMigrationPlan, bool, error) {
	if s == nil || expected.Validate() != nil {
		return tobari.InstallationMigrationPlan{}, false, fmt.Errorf("active authority selection is invalid")
	}
	pointerPath := filepath.Join(s.root, activeFileName)
	firstPointer, err := readBoundedOwnedFile(pointerPath, maxPointerBytes)
	if err != nil {
		return tobari.InstallationMigrationPlan{}, false, err
	}
	var pointer activeGenerationPointer
	if err := decodeStrictJSON(firstPointer, &pointer); err != nil || pointer.SchemaVersion != authorityStoreSchemaVersion {
		return tobari.InstallationMigrationPlan{}, false, fmt.Errorf("decode authority active pointer: %w", err)
	}
	component, err := digestFileComponent(pointer.GenerationDigest)
	if err != nil {
		return tobari.InstallationMigrationPlan{}, false, err
	}
	manifestData, err := readBoundedOwnedFile(filepath.Join(s.root, "generations", component+".json"), maxManifestBytes)
	if err != nil || digestBytes(manifestData) != pointer.GenerationDigest {
		return tobari.InstallationMigrationPlan{}, false, errors.Join(fmt.Errorf("authority generation digest mismatch"), err)
	}
	var manifest authorityGenerationManifest
	if err := decodeStrictJSON(manifestData, &manifest); err != nil {
		return tobari.InstallationMigrationPlan{}, false, err
	}
	observed, err := s.collectionFromManifest(ctx, manifest)
	if err != nil || observed.Generation != expected.Generation || observed.Revision != expected.Revision {
		return tobari.InstallationMigrationPlan{}, false, errors.Join(fmt.Errorf("active authority changed during provenance selection"), err)
	}
	secondPointer, err := readBoundedOwnedFile(pointerPath, maxPointerBytes)
	if err != nil || !bytes.Equal(firstPointer, secondPointer) {
		return tobari.InstallationMigrationPlan{}, false, errors.Join(fmt.Errorf("active authority pointer changed during provenance selection"), err)
	}
	if manifest.InstallationMigrationProvenance == nil {
		return tobari.InstallationMigrationPlan{}, false, nil
	}
	return *manifest.InstallationMigrationProvenance, true, nil
}

func (s *Store) readTypedObject(ctx context.Context, concept, id string, ref authorityObjectRef, target any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	component, err := digestFileComponent(ref.Digest)
	if err != nil || ref.Revision != ref.Digest || ref.ID != id {
		return fmt.Errorf("authority object reference is invalid")
	}
	want := filepath.ToSlash(filepath.Join(concept, id, component+".json"))
	if ref.Path != want {
		return fmt.Errorf("authority object path is invalid")
	}
	data, err := readBoundedOwnedFile(filepath.Join(s.root, filepath.FromSlash(want)), MaxAuthorityBytes)
	if err != nil {
		return err
	}
	if digestBytes(data) != ref.Digest {
		return fmt.Errorf("authority object digest mismatch")
	}
	return decodeStrictJSON(data, target)
}

func validateOwnedDirectoryInfo(info os.FileInfo) error {
	if info == nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm() != 0o700 || !ownedByCurrentUser(info) {
		return fmt.Errorf("must be a real owner-only directory")
	}
	return nil
}

func readBoundedOwnedFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) || info.Size() <= 0 || info.Size() > maximum {
		return nil, fmt.Errorf("authority file is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is assembled from validated fixed components.
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maximum {
		return nil, fmt.Errorf("authority file exceeds bounded size")
	}
	return data, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func ensureAuthorityDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return validateOwnedDirectoryInfo(info)
}

func writeImmutableAuthorityFile(path string, data []byte) error {
	if existing, err := readBoundedOwnedFile(path, MaxAuthorityBytes); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return fmt.Errorf("immutable authority object already exists with other bytes")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ensureAuthorityDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- validated immutable object path.
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

type authorityDeletionTombstone struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	DeletedAt     string `json:"deleted_at"`
	ResultDigest  string `json:"result_digest"`
}

func (s *Store) IsAuthorityTombstoned(kind, id string) (bool, error) {
	if s == nil || (kind != "templates" && kind != "contexts" && kind != "workspaces") || id == "" || filepath.Base(id) != id {
		return false, fmt.Errorf("authority tombstone target is invalid")
	}
	path := filepath.Join(s.root, "tombstones", kind, id+".json")
	data, err := readBoundedOwnedFile(path, maxPointerBytes)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var tombstone authorityDeletionTombstone
	if err := decodeStrictJSON(data, &tombstone); err != nil || tombstone.SchemaVersion != 1 || tombstone.Kind != kind || tombstone.ID != id || tombstone.ResultDigest == "" {
		return false, fmt.Errorf("authority deletion tombstone is invalid")
	}
	return true, nil
}

func (m *Mutator) rejectTombstoned(kind, id string) error {
	deleted, err := m.store.IsAuthorityTombstoned(kind, id)
	if err != nil {
		return err
	}
	if deleted {
		return tobari.ErrResourceIdentityDeleted
	}
	return nil
}

func (m *Mutator) purgeDeletedAuthority(kind, id string, result any) error {
	if m == nil || m.store == nil || m.clock == nil || id == "" {
		return fmt.Errorf("authority deletion purge is unavailable")
	}
	resultData, resultDigest, err := encodeAuthorityObject(result)
	if err != nil || len(resultData) == 0 {
		return err
	}
	tombstone := authorityDeletionTombstone{SchemaVersion: 1, Kind: kind, ID: id, DeletedAt: m.clock().UTC().Format("2006-01-02T15:04:05.000000000Z"), ResultDigest: resultDigest}
	tombstoneData, _, err := encodeAuthorityObject(tombstone)
	if err != nil {
		return err
	}
	tombstoneDir := filepath.Join(m.store.root, "tombstones", kind)
	if err := ensureAuthorityDirectory(tombstoneDir); err != nil {
		return err
	}
	tombstonePath := filepath.Join(tombstoneDir, id+".json")
	if existingData, readErr := readBoundedOwnedFile(tombstonePath, maxPointerBytes); readErr == nil {
		var existing authorityDeletionTombstone
		if decodeStrictJSON(existingData, &existing) != nil || existing.SchemaVersion != 1 || existing.Kind != kind || existing.ID != id || existing.ResultDigest != resultDigest {
			return fmt.Errorf("authority deletion tombstone conflicts with exact result")
		}
	} else if errors.Is(readErr, os.ErrNotExist) {
		if err := writeImmutableAuthorityFile(tombstonePath, tombstoneData); err != nil {
			return err
		}
	} else {
		return readErr
	}
	concepts := []string{kind}
	if kind == "contexts" {
		concepts = append(concepts, "policy-memory")
	}
	for _, concept := range concepts {
		target := filepath.Join(m.store.root, concept, id)
		if filepath.Dir(target) != filepath.Join(m.store.root, concept) {
			return fmt.Errorf("authority purge target is invalid")
		}
		info, inspectErr := os.Lstat(target)
		if errors.Is(inspectErr, os.ErrNotExist) {
			continue
		}
		if inspectErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || !ownedByCurrentUser(info) {
			return fmt.Errorf("authority purge target is unsafe")
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
	}
	return m.sync(m.store.root)
}

func (m *Mutator) publishGeneration(next tobari.WorkspaceAuthorityCollection) error {
	prepared, err := prepareAuthorityGeneration(next)
	if err != nil {
		return err
	}
	parent := filepath.Dir(m.store.root)
	if err := ensureAuthorityDirectory(m.store.root); err != nil {
		return err
	}
	for _, name := range authorityConceptDirectories {
		if err := ensureAuthorityDirectory(filepath.Join(m.store.root, name)); err != nil {
			return err
		}
	}
	paths := make([]string, 0, len(prepared.objects))
	for path := range prepared.objects {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		if err := writeImmutableAuthorityFile(filepath.Join(m.store.root, filepath.FromSlash(relative)), prepared.objects[relative]); err != nil {
			return err
		}
	}
	// Immutable object bytes are not selectable until every containing
	// directory entry is durable from the object leaf back to the authority
	// root. This barrier must precede the generation manifest.
	if err := syncAuthorityTree(m.store.root, m.sync); err != nil {
		return err
	}
	component, _ := digestFileComponent(prepared.manifestDigest)
	if err := writeImmutableAuthorityFile(filepath.Join(m.store.root, "generations", component+".json"), prepared.manifestData); err != nil {
		return err
	}
	if err := m.sync(filepath.Join(m.store.root, "generations")); err != nil {
		return err
	}
	if err := m.sync(m.store.root); err != nil {
		return err
	}
	temporary := mutationStagePath(m.store.root)
	if err := writeMutationFile(temporary, prepared.pointerData); err != nil {
		return err
	}
	if err := m.sync(filepath.Dir(temporary)); err != nil {
		return err
	}
	if err := m.rename(temporary, filepath.Join(m.store.root, activeFileName)); err != nil {
		return err
	}
	if err := m.sync(filepath.Dir(temporary)); err != nil {
		return err
	}
	if err := m.sync(m.store.root); err != nil {
		return err
	}
	return m.sync(parent)
}

// PublishMigrationStage writes one complete concept-separated generation into
// this Store root without selecting any other installation root. It exists
// only for the dormant pre-public cutover engine, whose private sibling stage
// directory is atomically renamed into place after independent read-back.
// Ordinary application writers must use Mutator publication under the
// installation lifecycle lock.
func (s *Store) PublishMigrationStage(collection tobari.WorkspaceAuthorityCollection) error {
	if s == nil || s.root == "" {
		return fmt.Errorf("final Workspace authority migration stage is unavailable")
	}
	if info, err := os.Lstat(s.root); err == nil {
		if validateOwnedDirectoryInfo(info) != nil {
			return fmt.Errorf("final Workspace authority migration stage is unsafe")
		}
		entries, readErr := os.ReadDir(s.root)
		if readErr != nil || len(entries) != 0 {
			return fmt.Errorf("final Workspace authority migration stage is not empty")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	publisher := &Mutator{store: s, rename: os.Rename, sync: syncMutationDirectory}
	return publisher.publishGeneration(collection)
}

// syncAuthorityTree provides a leaf-to-root durability barrier for a complete
// prepared generation. It never follows symlinks and validates every
// directory before asking the platform to persist its entries.
func syncAuthorityTree(root string, sync func(string) error) error {
	if sync == nil {
		return fmt.Errorf("authority directory sync is unavailable")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || validateOwnedDirectoryInfo(rootInfo) != nil {
		return fmt.Errorf("authority root is unsafe for durable publication")
	}
	directories := []string{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("authority staging tree contains a symlink")
		}
		if !entry.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil || validateOwnedDirectoryInfo(info) != nil {
			return fmt.Errorf("authority staging directory is unsafe")
		}
		directories = append(directories, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(filepath.Clean(directories[i]), string(filepath.Separator))
		rightDepth := strings.Count(filepath.Clean(directories[j]), string(filepath.Separator))
		if leftDepth == rightDepth {
			return directories[i] < directories[j]
		}
		return leftDepth > rightDepth
	})
	for _, directory := range directories {
		if err := sync(directory); err != nil {
			return err
		}
	}
	return nil
}
