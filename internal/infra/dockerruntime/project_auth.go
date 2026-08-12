package dockerruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const projectAuthRegistrySchema = 1

type brokerCredentialBinding struct {
	ProviderID    string                              `json:"provider_id"`
	Target        *authbroker.BindingTarget           `json:"target,omitempty"`
	Source        *authbroker.NormalizedBindingSource `json:"source,omitempty"`
	Destination   *authbroker.BindingDestination      `json:"destination,omitempty"`
	SecretHeaders []string                            `json:"secret_headers,omitempty"`
}

type projectAuthFile struct {
	Path    string
	Content []byte
	Digest  string
}

type projectAuthProjection struct {
	Environment []string
	Files       []projectAuthFile
	Providers   []projectAuthProviderBinding
}

type projectAuthProviderBinding struct {
	Provider      string `json:"provider"`
	Revision      string `json:"revision"`
	BindingDigest string `json:"binding_digest"`
}

type projectAuthRegistryEntry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type projectAuthRegistry struct {
	SchemaVersion int                          `json:"schema_version"`
	ProjectID     string                       `json:"project_id"`
	Providers     []projectAuthProviderBinding `json:"providers"`
	Files         []projectAuthRegistryEntry   `json:"files"`
}

func brokerBindingsForProvider(
	projection authbroker.Projection, providerID string,
) ([]brokerCredentialBinding, []byte, string, error) {
	bindings := make([]brokerCredentialBinding, 0)
	for _, binding := range projection.HeaderBindings {
		if binding.ProviderID != providerID {
			continue
		}
		target, source, destination := binding.Target, binding.Source, binding.Destination
		bindings = append(bindings, brokerCredentialBinding{
			ProviderID: providerID, Target: &target, Source: &source, Destination: &destination,
			SecretHeaders: append([]string(nil), binding.SecretHeaders...),
		})
	}
	encoded, err := json.Marshal(bindings)
	if err != nil || len(bindings) == 0 || len(encoded) > maxBrokerControlOutput {
		return nil, nil, "", fault.New(
			fault.KindContract, "invalid_provider_manifest",
			"The provider's normalized credential-binding projection is invalid.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the credential-provider manifests."},
		)
	}
	digest := sha256.Sum256(encoded)
	return bindings, encoded, "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (r *Runtime) projectAuthRegistryPath(projectID string) string {
	return filepath.Join(r.stateDirectory, "auth", "projects", projectID+".json")
}

func (r *Runtime) reconcileProjectAuth(
	ctx context.Context,
	instance tobari.ProjectInstance,
) (projectAuthProjection, error) {
	if err := instance.Validate(); err != nil {
		return projectAuthProjection{}, err
	}
	if err := r.requireUnlockedAuthBroker(ctx); err != nil {
		return projectAuthProjection{}, err
	}
	providerProjection, err := r.loadAuthProviders()
	if err != nil {
		return projectAuthProjection{}, err
	}
	desired := projectAuthProjection{Environment: []string{}, Files: []projectAuthFile{}, Providers: []projectAuthProviderBinding{}}
	for _, provider := range providerProjection.Providers {
		status, err := r.runBrokerControl(
			ctx, nil, "status", "--context-id", instance.ContextID, "--provider", provider.ID,
		)
		if err != nil {
			return projectAuthProjection{}, classifyBrokerError(err, "tobari")
		}
		switch status.State {
		case "not_configured":
			continue
		case "configured":
		default:
			return projectAuthProjection{}, fault.New(
				fault.KindUnavailable, "auth_broker_locked",
				"The Auth Broker cannot issue the Workspace authentication projection.", false,
				fault.NextAction{Command: "cluster up", Reason: "Reconcile and unlock the shared Auth Broker."},
			)
		}
		_, encodedBindings, bindingDigest, err := brokerBindingsForProvider(providerProjection, provider.ID)
		if err != nil {
			return projectAuthProjection{}, err
		}
		issued, err := r.runBrokerControl(
			ctx, nil, "issue_handle",
			"--context-id", instance.ContextID,
			"--project-id", instance.ID,
			"--provider", provider.ID,
			"--bindings", string(encodedBindings),
		)
		if err != nil {
			return projectAuthProjection{}, classifyBrokerError(err, "tobari")
		}
		if issued.Provider != provider.ID || issued.Revision != status.Revision || !validProjectHandle(issued.Handle) {
			return projectAuthProjection{}, fault.New(
				fault.KindContract, "invalid_auth_handle_result",
				"The Auth Broker returned an invalid Workspace authentication projection.", false,
				fault.NextAction{Command: "doctor", Reason: "Inspect Broker and provider projection consistency."},
			)
		}
		desired.Providers = append(desired.Providers, projectAuthProviderBinding{
			Provider: provider.ID, Revision: issued.Revision,
			BindingDigest: bindingDigest,
		})
		for _, item := range providerProjection.Environment {
			if item.ProviderID == provider.ID {
				desired.Environment = append(
					desired.Environment,
					item.Name+"="+renderProviderTemplate(item.Template, issued.Handle, provider),
				)
			}
		}
		for _, item := range providerProjection.CompleteFiles {
			if item.ProviderID != provider.ID {
				continue
			}
			content := []byte(renderProviderTemplate(item.Template, issued.Handle, provider))
			digest := sha256.Sum256(content)
			desired.Files = append(desired.Files, projectAuthFile{
				Path: item.Path, Content: content, Digest: "sha256:" + hex.EncodeToString(digest[:]),
			})
		}
	}
	sort.Strings(desired.Environment)
	sort.Slice(desired.Files, func(left, right int) bool { return desired.Files[left].Path < desired.Files[right].Path })
	sort.Slice(desired.Providers, func(left, right int) bool {
		return desired.Providers[left].Provider < desired.Providers[right].Provider
	})
	if err := r.reconcileProjectAuthFiles(instance, desired.Files, desired.Providers); err != nil {
		return projectAuthProjection{}, err
	}
	return desired, nil
}

func validProjectHandle(value string) bool {
	const prefix = "tobari-h1_"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+43 {
		return false
	}
	encoded := value[len(prefix):]
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	return err == nil && len(decoded) == 32
}

func (r *Runtime) reconcileProjectAuthFiles(
	instance tobari.ProjectInstance,
	desired []projectAuthFile,
	providers []projectAuthProviderBinding,
) error {
	registry, err := r.readProjectAuthRegistry(instance.ID)
	if err != nil {
		return err
	}
	home, err := os.OpenRoot(r.projectHomePath(instance.ID))
	if err != nil {
		return fmt.Errorf("open Workspace home for authentication projection: %w", err)
	}
	defer home.Close()
	owned := make(map[string]string, len(registry.Files))
	for _, entry := range registry.Files {
		owned[entry.Path] = entry.Digest
	}
	desiredByPath := make(map[string]projectAuthFile, len(desired))
	for _, file := range desired {
		if err := authbroker.ValidateRelativeHomePath(file.Path); err != nil {
			return fmt.Errorf("invalid Workspace authentication file path: %w", err)
		}
		desiredByPath[file.Path] = file
		if err := prepareAuthFileParent(home, file.Path); err != nil {
			return err
		}
		if err := validateAuthFileTarget(home, file.Path, owned[file.Path]); err != nil {
			return err
		}
	}
	for _, entry := range registry.Files {
		if _, retained := desiredByPath[entry.Path]; retained {
			continue
		}
		if err := validateAuthFileTarget(home, entry.Path, entry.Digest); err != nil {
			return err
		}
	}
	for _, file := range desired {
		if current, readErr := home.ReadFile(file.Path); readErr == nil && digestBytes(current) == file.Digest {
			continue
		}
		if err := atomicWriteAuthFile(home, file.Path, file.Content); err != nil {
			return err
		}
	}
	for _, entry := range registry.Files {
		if _, retained := desiredByPath[entry.Path]; retained {
			continue
		}
		if err := home.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove obsolete Tobari-owned authentication file: %w", err)
		}
	}
	next := projectAuthRegistry{
		SchemaVersion: projectAuthRegistrySchema,
		ProjectID:     instance.ID,
		Providers:     make([]projectAuthProviderBinding, len(providers)),
		Files:         make([]projectAuthRegistryEntry, 0, len(desired)),
	}
	copy(next.Providers, providers)
	for _, file := range desired {
		next.Files = append(next.Files, projectAuthRegistryEntry{Path: file.Path, Digest: file.Digest})
	}
	if err := writeAtomicJSON(r.projectAuthRegistryPath(instance.ID), next); err != nil {
		return fmt.Errorf("persist Workspace authentication file ownership: %w", err)
	}
	return nil
}

func (r *Runtime) readProjectAuthRegistry(projectID string) (projectAuthRegistry, error) {
	registryPath := r.projectAuthRegistryPath(projectID)
	if _, err := os.Lstat(registryPath); errors.Is(err, os.ErrNotExist) {
		return projectAuthRegistry{
			SchemaVersion: projectAuthRegistrySchema,
			ProjectID:     projectID,
			Providers:     []projectAuthProviderBinding{},
			Files:         []projectAuthRegistryEntry{},
		}, nil
	} else if err != nil {
		return projectAuthRegistry{}, fmt.Errorf("inspect Workspace authentication file ownership: %w", err)
	}
	data, err := readOwnerPolicyFile(registryPath, 256*1024)
	if err != nil {
		return projectAuthRegistry{}, fmt.Errorf("read Workspace authentication file ownership: %w", err)
	}
	var registry projectAuthRegistry
	if err := decodeStrictJSON(data, &registry); err != nil {
		return projectAuthRegistry{}, fmt.Errorf("decode Workspace authentication file ownership: %w", err)
	}
	if registry.SchemaVersion != projectAuthRegistrySchema || registry.ProjectID != projectID || registry.Files == nil {
		return projectAuthRegistry{}, fmt.Errorf("Workspace authentication file ownership is invalid")
	}
	if registry.Providers == nil {
		if !isRecoverableEmptyProviderRegistry(data, registry) {
			return projectAuthRegistry{}, fmt.Errorf("Workspace authentication file ownership is invalid")
		}
		registry.Providers = []projectAuthProviderBinding{}
		if err := writeAtomicJSON(registryPath, registry); err != nil {
			return projectAuthRegistry{}, fmt.Errorf("rewrite Workspace authentication file ownership: %w", err)
		}
	}
	providers := make(map[string]struct{}, len(registry.Providers))
	for _, provider := range registry.Providers {
		if authbroker.ValidateProviderID(provider.Provider) != nil || !validAuthRevision(provider.Revision) || !validSHA256(provider.BindingDigest) {
			return projectAuthRegistry{}, fmt.Errorf("Workspace authentication ownership contains an invalid provider binding")
		}
		if _, duplicate := providers[provider.Provider]; duplicate {
			return projectAuthRegistry{}, fmt.Errorf("Workspace authentication ownership contains a duplicate provider binding")
		}
		providers[provider.Provider] = struct{}{}
	}
	seen := make(map[string]struct{}, len(registry.Files))
	for _, entry := range registry.Files {
		if err := authbroker.ValidateRelativeHomePath(entry.Path); err != nil || !validSHA256(entry.Digest) {
			return projectAuthRegistry{}, fmt.Errorf("Workspace authentication file ownership contains an invalid entry")
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return projectAuthRegistry{}, fmt.Errorf("Workspace authentication file ownership contains a duplicate path")
		}
		seen[entry.Path] = struct{}{}
	}
	return registry, nil
}

func isRecoverableEmptyProviderRegistry(data []byte, registry projectAuthRegistry) bool {
	if registry.Providers != nil || registry.Files == nil || len(registry.Files) != 0 {
		return false
	}
	if validateNoDuplicateJSONKeys(data) != nil {
		return false
	}
	var raw struct {
		SchemaVersion json.RawMessage `json:"schema_version"`
		ProjectID     json.RawMessage `json:"project_id"`
		Providers     json.RawMessage `json:"providers"`
		Files         json.RawMessage `json:"files"`
	}
	if err := decodeStrictJSON(data, &raw); err != nil ||
		raw.SchemaVersion == nil || raw.ProjectID == nil || raw.Providers == nil || raw.Files == nil {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(raw.Providers), []byte("null")) &&
		bytes.Equal(bytes.TrimSpace(raw.Files), []byte("[]"))
}

func validAuthRevision(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') ||
			(character >= '0' && character <= '9') || (index > 0 && strings.ContainsRune("._:-", character)) {
			continue
		}
		return false
	}
	return true
}

func validSHA256(value string) bool {
	if len(value) != len("sha256:")+64 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func prepareAuthFileParent(root *os.Root, relative string) error {
	directory := path.Dir(relative)
	if directory == "." {
		return nil
	}
	current := ""
	for _, segment := range strings.Split(directory, "/") {
		if current == "" {
			current = segment
		} else {
			current += "/" + segment
		}
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("create Workspace authentication directory: %w", err)
			}
			info, err = root.Lstat(current)
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Workspace authentication path traverses an unsafe directory")
		}
	}
	return nil
}

func validateAuthFileTarget(root *os.Root, relative, ownedDigest string) error {
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		if ownedDigest != "" {
			return fmt.Errorf("Tobari-owned Workspace authentication file is missing")
		}
		return nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("Workspace authentication target is not a regular non-symlink file")
	}
	if ownedDigest == "" {
		return fault.New(
			fault.KindRejected, "auth_projection_file_exists",
			"A provider projection would overwrite a file that Tobari does not own.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the configured provider file path and Workspace home."},
		)
	}
	content, err := root.ReadFile(relative)
	if err != nil || len(content) > authbroker.MaxTemplateBytes+1024 || digestBytes(content) != ownedDigest {
		return fault.New(
			fault.KindRejected, "auth_projection_file_modified",
			"A Tobari-owned Workspace authentication file was changed outside reconciliation.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Workspace authentication file before replacing it."},
		)
	}
	return nil
}

func atomicWriteAuthFile(root *os.Root, relative string, content []byte) error {
	random := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return fmt.Errorf("generate authentication projection temporary name: %w", err)
	}
	temporary := path.Join(path.Dir(relative), ".tobari-auth-"+hex.EncodeToString(random))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create authentication projection temporary file: %w", err)
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = root.Remove(temporary)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect authentication projection temporary file: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("write authentication projection temporary file: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync authentication projection temporary file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close authentication projection temporary file: %w", err)
	}
	if err := root.Rename(temporary, relative); err != nil {
		return fmt.Errorf("replace authentication projection file: %w", err)
	}
	remove = false
	return nil
}
