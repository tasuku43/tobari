package workspaceauthoritystore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	authorityFileName     = "authority.json"
	MaxAuthorityBytes     = 64 << 20
	maxWorkspaceTemplates = 1024
	maxContexts           = 16 * 1024
	maxWorkspaces         = 16 * 1024
	maxPendingCandidates  = 64 * 1024
)

// Store observes one owner-only, atomically published final-authority
// envelope. This reader never creates the root, a lock, or an empty store and
// never falls back to predecessor Manifest authority.
type Store struct {
	root        string
	legacyGuard LegacyAuthorityGuard
}

// LegacyAuthorityGuard proves that no unsupported pre-release authority is
// present in the closed predecessor-root inventory. It is a presence-only,
// zero-mutation observation: implementations must never decode, adopt, move,
// delete, or reinterpret legacy bytes.
type LegacyAuthorityGuard interface {
	ConfirmNoPreReleaseLegacyAuthority(context.Context, bool) error
}

func New(root string) (*Store, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(filepath.Separator) {
		return nil, fmt.Errorf("final Workspace authority root must be an exact absolute child path")
	}
	return &Store{root: root}, nil
}

// NewFinalOnly constructs the ordinary final reader. Raw New remains reserved
// for atomic publication/read-back inside the final authority owner and tests.
func NewFinalOnly(root string, legacyGuard LegacyAuthorityGuard) (*Store, error) {
	store, err := New(root)
	if err != nil {
		return nil, err
	}
	if legacyGuard == nil {
		return nil, fmt.Errorf("pre-release legacy authority guard is required")
	}
	store.legacyGuard = legacyGuard
	return store, nil
}

func (s *Store) ReadComplete(ctx context.Context) (tobari.WorkspaceAuthorityCollection, bool, error) {
	if s == nil || s.legacyGuard == nil {
		return s.readCompleteRaw(ctx)
	}
	first, firstPresent, err := s.readCompleteRaw(ctx)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := s.confirmNoLegacy(ctx, firstPresent); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	second, secondPresent, err := s.readCompleteRaw(ctx)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := s.confirmNoLegacy(ctx, secondPresent); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if firstPresent != secondPresent || firstPresent && (first.Generation != second.Generation || first.Revision != second.Revision) {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority changed during coherent observation")
	}
	return second, secondPresent, nil
}

// ReadFinalRuntimeProtectionAuthority returns the narrow final-owner
// projection injected into WP03 Runtime lifecycle observation. The Store keeps
// complete-envelope selection and legacy-presence fencing here; dockerruntime
// receives no store path and cannot rediscover predecessor Manifest state.
func (s *Store) ReadFinalRuntimeProtectionAuthority(ctx context.Context) (tobari.FinalRuntimeProtectionAuthority, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return tobari.FinalRuntimeProtectionAuthority{}, err
	}
	return tobari.NewFinalRuntimeProtectionAuthority(collection, present)
}

// ConfirmSelected re-observes the final-only boundary while a caller holds the
// installation lifecycle lock. Mutators call it immediately before an
// external effect or envelope publication so legacy appearance or envelope
// drift cannot authorize a later write.
func (s *Store) ConfirmSelected(ctx context.Context, collection tobari.WorkspaceAuthorityCollection, present bool) error {
	if s == nil || s.legacyGuard == nil {
		return nil
	}
	if err := s.confirmNoLegacy(ctx, present); err != nil {
		return err
	}
	current, currentPresent, err := s.readCompleteRaw(ctx)
	if err != nil {
		return err
	}
	if err := s.confirmNoLegacy(ctx, currentPresent); err != nil {
		return err
	}
	if currentPresent != present || present && (current.Generation != collection.Generation || current.Revision != collection.Revision) {
		return fmt.Errorf("final Workspace authority changed before mutation")
	}
	return nil
}

func (s *Store) confirmNoLegacy(ctx context.Context, finalInitialized bool) error {
	if err := s.legacyGuard.ConfirmNoPreReleaseLegacyAuthority(ctx, finalInitialized); err != nil {
		return fmt.Errorf("%w; reset or recreate the development installation before using final authority: %v", tobari.ErrPreReleaseLegacyAuthority, err)
	}
	return nil
}

func (s *Store) readCompleteRaw(ctx context.Context) (tobari.WorkspaceAuthorityCollection, bool, error) {
	if s == nil || s.root == "" {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority store is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	rootInfo, err := os.Lstat(s.root)
	if errors.Is(err, os.ErrNotExist) {
		return tobari.WorkspaceAuthorityCollection{}, false, nil
	}
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("inspect final Workspace authority root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() || rootInfo.Mode().Perm() != 0o700 || !ownedByCurrentUser(rootInfo) {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority root must be a real owner-only directory")
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("enumerate final Workspace authority root: %w", err)
	}
	if len(entries) != 1 || entries[0].Name() != authorityFileName {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority store is partial or mixed")
	}

	path := filepath.Join(s.root, authorityFileName)
	data, err := readAuthorityFile(path)
	if err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := rejectLegacyAdvancedAuthorityBytes(data); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	rootAfter, err := os.Lstat(s.root)
	if err != nil || !os.SameFile(rootInfo, rootAfter) || rootAfter.Mode()&os.ModeSymlink != 0 ||
		!rootAfter.IsDir() || rootAfter.Mode().Perm() != 0o700 || !ownedByCurrentUser(rootAfter) {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority root changed during observation")
	}
	entriesAfter, err := os.ReadDir(s.root)
	if err != nil || len(entriesAfter) != 1 || entriesAfter[0].Name() != authorityFileName {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority store changed or became mixed during observation")
	}
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	var collection tobari.WorkspaceAuthorityCollection
	if err := decodeStrictJSON(data, &collection); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("decode final Workspace authority: %w", err)
	}
	if err := validateCollectionBounds(collection); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	if err := collection.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("validate final Workspace authority: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	return collection.Clone(), true, nil
}

// rejectLegacyAdvancedAuthorityBytes is intentionally a bounded marker
// detector, not a predecessor decoder. It recognizes only the v1 final
// envelope's policy markers, never reads or interprets executable source, and
// leaves all other schema/JSON validation to the strict decoder below.
func rejectLegacyAdvancedAuthorityBytes(data []byte) error {
	var envelope struct {
		Templates []json.RawMessage `json:"workspace_templates"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil
	}
	for _, templateData := range envelope.Templates {
		var template struct {
			Current  json.RawMessage   `json:"current"`
			Retained []json.RawMessage `json:"retained"`
		}
		if err := json.Unmarshal(templateData, &template); err != nil {
			return nil
		}
		revisions := append([]json.RawMessage{}, template.Retained...)
		if len(template.Current) != 0 && string(template.Current) != "null" {
			revisions = append(revisions, template.Current)
		}
		for _, revisionData := range revisions {
			var revision struct {
				Body json.RawMessage `json:"body"`
			}
			if err := json.Unmarshal(revisionData, &revision); err != nil {
				return nil
			}
			var body struct {
				Policy json.RawMessage `json:"policy"`
			}
			if err := json.Unmarshal(revision.Body, &body); err != nil {
				return nil
			}
			var policy map[string]json.RawMessage
			if err := json.Unmarshal(body.Policy, &policy); err != nil {
				return nil
			}
			if rawMode, exists := policy["mode"]; exists {
				var mode string
				if json.Unmarshal(rawMode, &mode) == nil && (mode == "guided" || mode == "advanced") {
					if mode == "advanced" {
						return fmt.Errorf("%w: %w: persisted final authority contains legacy %s policy mode; reset or recreate the installation", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy, mode)
					}
					return fmt.Errorf("%w: persisted final authority contains legacy %s policy mode; reset or recreate the installation", tobari.ErrPreReleaseLegacyAuthority, mode)
				}
			}
			if _, exists := policy["advanced_policy"]; exists {
				return fmt.Errorf("%w: %w: persisted final authority contains legacy Advanced policy; reset or recreate the installation", tobari.ErrPreReleaseLegacyAuthority, tobari.ErrLegacyExecutablePolicy)
			}
		}
	}
	return nil
}

func readAuthorityFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect final Workspace authority file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		return nil, fmt.Errorf("final Workspace authority file must be a real owner-only regular file")
	}
	if info.Size() <= 0 || info.Size() > MaxAuthorityBytes {
		return nil, fmt.Errorf("final Workspace authority file must contain 1..%d bytes", MaxAuthorityBytes)
	}
	file, err := os.Open(path) // #nosec G304 -- Store owns the fixed child of one validated exact root.
	if err != nil {
		return nil, fmt.Errorf("open final Workspace authority file: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened final Workspace authority file: %w", err)
	}
	if !opened.Mode().IsRegular() || opened.Mode().Perm() != 0o600 || !ownedByCurrentUser(opened) || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("final Workspace authority file changed during safe open")
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxAuthorityBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read final Workspace authority file: %w", err)
	}
	if len(data) == 0 || len(data) > MaxAuthorityBytes {
		return nil, fmt.Errorf("final Workspace authority file must contain 1..%d bytes", MaxAuthorityBytes)
	}
	return data, nil
}

func validateCollectionBounds(collection tobari.WorkspaceAuthorityCollection) error {
	if len(collection.Templates) > maxWorkspaceTemplates || len(collection.Contexts) > maxContexts ||
		len(collection.Workspaces) > maxWorkspaces || len(collection.PendingCandidates) > maxPendingCandidates {
		return fmt.Errorf("final Workspace authority collection exceeds bounded counts")
	}
	return nil
}

func decodeStrictJSON(data []byte, target any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("JSON contains trailing data")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var parse func() error
	parse = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := parse(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return fmt.Errorf("JSON object is not closed")
			}
		case '[':
			for decoder.More() {
				if err := parse(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim(']') {
				return fmt.Errorf("JSON array is not closed")
			}
		default:
			return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
		}
		return nil
	}
	if err := parse(); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("JSON contains trailing data")
		}
		return err
	}
	return nil
}

func (s *Store) ListWorkspaceTemplates(ctx context.Context) ([]tobari.WorkspaceTemplate, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return nil, err
	}
	if !present {
		return []tobari.WorkspaceTemplate{}, nil
	}
	return cloneTemplates(collection.Templates), nil
}

func (s *Store) DiscoverWorkspaceTemplate(ctx context.Context, name string) (tobari.WorkspaceTemplate, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return tobari.WorkspaceTemplate{}, err
	}
	if !present {
		return tobari.WorkspaceTemplate{}, tobari.ErrWorkspaceTemplateNotFound
	}
	if name == "" && collection.DefaultTemplateID != nil {
		for _, template := range collection.Templates {
			if template.ID == *collection.DefaultTemplateID {
				return template.Clone(), nil
			}
		}
	}
	for _, template := range collection.Templates {
		if name != "" && template.Name == name {
			return template.Clone(), nil
		}
	}
	return tobari.WorkspaceTemplate{}, tobari.ErrWorkspaceTemplateNotFound
}

func (s *Store) ListContextAuthority(ctx context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	collection, present, err := s.ReadComplete(ctx)
	if err != nil {
		return nil, err
	}
	if !present {
		return []tobari.ContextAuthoritySnapshot{}, nil
	}
	return collection.ContextSnapshots()
}

func (s *Store) ReadContextAuthorityByReference(ctx context.Context, ref string) (tobari.ContextAuthoritySnapshot, error) {
	id, err := tobari.ParseContextRef(ref)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	snapshots, err := s.ListContextAuthority(ctx)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Context.ID == id {
			return snapshot.Clone(), nil
		}
	}
	return tobari.ContextAuthoritySnapshot{}, tobari.ErrContextBindingNotFound
}

func (s *Store) ListWorkspaceAuthority(ctx context.Context) ([]tobari.ContextAuthoritySnapshot, error) {
	snapshots, err := s.ListContextAuthority(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]tobari.ContextAuthoritySnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.Workspace != nil {
			result = append(result, snapshot.Clone())
		}
	}
	return result, nil
}

func (s *Store) ReadWorkspaceAuthorityByReference(ctx context.Context, ref string) (tobari.ContextAuthoritySnapshot, error) {
	id, err := tobari.ParseWorkspaceRef(ref)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	snapshots, err := s.ListWorkspaceAuthority(ctx)
	if err != nil {
		return tobari.ContextAuthoritySnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if snapshot.Workspace != nil && snapshot.Workspace.ID == id {
			return snapshot.Clone(), nil
		}
	}
	return tobari.ContextAuthoritySnapshot{}, tobari.ErrWorkspaceBindingNotFound
}

func cloneTemplates(values []tobari.WorkspaceTemplate) []tobari.WorkspaceTemplate {
	result := make([]tobari.WorkspaceTemplate, len(values))
	for index, value := range values {
		result[index] = value.Clone()
	}
	return result
}
