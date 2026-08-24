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
	maxAuthorityBytes     = 64 << 20
	maxWorkspaceTemplates = 1024
	maxContexts           = 16 * 1024
	maxWorkspaces         = 16 * 1024
	maxPendingCandidates  = 64 * 1024
)

// Store observes one owner-only, atomically published final-authority
// envelope. This reader never creates the root, a lock, or an empty store and
// never falls back to predecessor Manifest authority.
type Store struct {
	root string
}

func New(root string) (*Store, error) {
	if root == "" || !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(filepath.Separator) {
		return nil, fmt.Errorf("final Workspace authority root must be an exact absolute child path")
	}
	return &Store{root: root}, nil
}

func (s *Store) ReadComplete(ctx context.Context) (tobari.WorkspaceAuthorityCollection, bool, error) {
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
	if len(collection.Templates) > maxWorkspaceTemplates || len(collection.Contexts) > maxContexts ||
		len(collection.Workspaces) > maxWorkspaces || len(collection.PendingCandidates) > maxPendingCandidates {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("final Workspace authority collection exceeds bounded counts")
	}
	if err := collection.Validate(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, fmt.Errorf("validate final Workspace authority: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return tobari.WorkspaceAuthorityCollection{}, false, err
	}
	return collection.Clone(), true, nil
}

func readAuthorityFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect final Workspace authority file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
		return nil, fmt.Errorf("final Workspace authority file must be a real owner-only regular file")
	}
	if info.Size() <= 0 || info.Size() > maxAuthorityBytes {
		return nil, fmt.Errorf("final Workspace authority file must contain 1..%d bytes", maxAuthorityBytes)
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
	data, err := io.ReadAll(io.LimitReader(file, maxAuthorityBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read final Workspace authority file: %w", err)
	}
	if len(data) == 0 || len(data) > maxAuthorityBytes {
		return nil, fmt.Errorf("final Workspace authority file must contain 1..%d bytes", maxAuthorityBytes)
	}
	return data, nil
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
