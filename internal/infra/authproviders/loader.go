// Package authproviders loads reviewed built-in and owner-controlled provider
// manifests into the pure authbroker projection contract.
package authproviders

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

const (
	BuiltinAWSProviderID          = "aws"
	BuiltinAnthropicProviderID    = "anthropic"
	BuiltinChatworkProviderID     = "chatwork"
	BuiltinDatadogProviderID      = "datadog"
	BuiltinGitHubProviderID       = "github"
	BuiltinOpenAIProviderID       = "openai"
	UserProviderRelativeDirectory = "tobari/auth/providers"
)

//go:embed builtins/*.json
var builtinDocuments embed.FS

// Loader owns the exact provider-directory read boundary. Directory is
// supplied by the composition root as
// ${XDG_CONFIG_HOME}/tobari/auth/providers; this package does not rediscover or
// silently fall back to another configuration root.
type Loader struct {
	directory string
}

func New(directory string) (*Loader, error) {
	if directory == "" || !filepath.IsAbs(directory) || filepath.Clean(directory) != directory {
		return nil, fmt.Errorf("provider directory must be a canonical absolute path")
	}
	return &Loader{directory: directory}, nil
}

func (l *Loader) Load() (authbroker.Projection, error) {
	if l == nil || l.directory == "" {
		return authbroker.Projection{}, fmt.Errorf("provider loader is not configured")
	}
	providers, builtinIDs, err := loadBuiltins()
	if err != nil {
		return authbroker.Projection{}, err
	}
	users, err := loadUserProviders(l.directory, builtinIDs)
	if err != nil {
		return authbroker.Projection{}, err
	}
	providers = append(providers, users...)
	projection, err := authbroker.NormalizeProviders(providers)
	if err != nil {
		return authbroker.Projection{}, fmt.Errorf("normalize provider collection: %w", err)
	}
	return projection, nil
}

// Builtins returns the normalized built-in-only projection. The same parser
// and domain validation path is used for built-in and user documents.
func Builtins() (authbroker.Projection, error) {
	providers, _, err := loadBuiltins()
	if err != nil {
		return authbroker.Projection{}, err
	}
	return authbroker.NormalizeProviders(providers)
}

func loadBuiltins() ([]authbroker.Provider, map[string]struct{}, error) {
	names, err := fs.Glob(builtinDocuments, "builtins/*.json")
	if err != nil {
		return nil, nil, fmt.Errorf("list built-in providers: %w", err)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, nil, fmt.Errorf("built-in provider collection is empty")
	}
	providers := make([]authbroker.Provider, 0, len(names))
	ids := make(map[string]struct{}, len(names))
	for _, name := range names {
		data, err := builtinDocuments.ReadFile(name)
		if err != nil {
			return nil, nil, fmt.Errorf("read built-in provider %s: %w", name, err)
		}
		provider, err := authbroker.ParseProvider(data)
		if err != nil {
			return nil, nil, fmt.Errorf("parse built-in provider %s: %w", name, err)
		}
		if _, exists := ids[provider.ID]; exists {
			return nil, nil, fmt.Errorf("built-in provider ID %q is duplicated", provider.ID)
		}
		ids[provider.ID] = struct{}{}
		providers = append(providers, provider)
	}
	return providers, ids, nil
}

func loadUserProviders(directory string, builtinIDs map[string]struct{}) ([]authbroker.Provider, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []authbroker.Provider{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect user provider directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("user provider path must be a real directory")
	}
	if err := validateOwnerOnlyDirectory(info); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read user provider directory: %w", err)
	}
	providers := make([]authbroker.Provider, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		provider, err := readUserProvider(path)
		if err != nil {
			return nil, fmt.Errorf("load user provider %s: %w", entry.Name(), err)
		}
		if _, builtin := builtinIDs[provider.ID]; builtin {
			return nil, fmt.Errorf("user provider cannot override built-in provider %q", provider.ID)
		}
		if provider.SchemaVersion != authbroker.ProviderSchemaVersion {
			return nil, fmt.Errorf("user provider %q must use schema_version %d", provider.ID, authbroker.ProviderSchemaVersion)
		}
		if provider.Acquisition.Mode != authbroker.AcquisitionStdinImport {
			return nil, fmt.Errorf("user provider %q must use stdin_import acquisition", provider.ID)
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func readUserProvider(path string) (authbroker.Provider, error) {
	lstat, err := os.Lstat(path)
	if err != nil {
		return authbroker.Provider{}, fmt.Errorf("inspect provider file: %w", err)
	}
	if lstat.Mode()&os.ModeSymlink != 0 || !lstat.Mode().IsRegular() {
		return authbroker.Provider{}, fmt.Errorf("provider file must be a regular non-symlink file")
	}
	if err := validateOwnerOnlyFile(lstat); err != nil {
		return authbroker.Provider{}, err
	}
	file, err := os.Open(path) // #nosec G304 -- path is one ReadDir entry below the injected, validated provider directory.
	if err != nil {
		return authbroker.Provider{}, fmt.Errorf("open provider file: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return authbroker.Provider{}, fmt.Errorf("inspect opened provider file: %w", err)
	}
	if !os.SameFile(lstat, opened) || !opened.Mode().IsRegular() {
		return authbroker.Provider{}, fmt.Errorf("provider file changed while it was opened")
	}
	if err := validateOwnerOnlyFile(opened); err != nil {
		return authbroker.Provider{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, authbroker.MaxProviderDocumentBytes+1))
	if err != nil {
		return authbroker.Provider{}, fmt.Errorf("read provider file: %w", err)
	}
	if len(data) > authbroker.MaxProviderDocumentBytes {
		return authbroker.Provider{}, fmt.Errorf("provider file exceeds %d bytes", authbroker.MaxProviderDocumentBytes)
	}
	provider, err := authbroker.ParseProvider(data)
	if err != nil {
		return authbroker.Provider{}, err
	}
	return provider, nil
}

func validateOwnerOnlyDirectory(info os.FileInfo) error {
	permissions := info.Mode().Perm()
	if permissions&0o077 != 0 || permissions&0o500 != 0o500 {
		return fmt.Errorf("user provider directory must be owner-readable and owner-searchable with no group or world permissions")
	}
	return validateCurrentUserOwner(info)
}

func validateOwnerOnlyFile(info os.FileInfo) error {
	permissions := info.Mode().Perm()
	if permissions&0o077 != 0 || permissions&0o400 == 0 || permissions&0o111 != 0 {
		return fmt.Errorf("provider file must be owner-readable, non-executable, and have no group or world permissions")
	}
	return validateCurrentUserOwner(info)
}
