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
	"unicode"
	"unicode/utf8"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
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
	Kind          authbroker.SigningBindingKind       `json:"kind,omitempty"`
	AWSSigV4      *authbroker.AWSSigV4Binding         `json:"aws_sigv4,omitempty"`
}

type projectAuthFile struct {
	Path    string
	Content []byte
	Digest  string
}

type projectAuthProjection struct {
	Environment []string
	Files       []projectAuthFile
	JSONMerges  []projectAuthJSONMerge
	Providers   []projectAuthProviderBinding
}

type projectAuthJSONMerge struct {
	Path    string
	Content []byte
	Fields  []string
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

type projectAuthJSONMergeRegistryEntry struct {
	Path   string   `json:"path"`
	Fields []string `json:"fields"`
}

type projectAuthRegistry struct {
	SchemaVersion int                                 `json:"schema_version"`
	ProjectID     string                              `json:"workspace_id"`
	Providers     []projectAuthProviderBinding        `json:"providers"`
	Files         []projectAuthRegistryEntry          `json:"files"`
	JSONMerges    []projectAuthJSONMergeRegistryEntry `json:"json_merges"`
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
	for _, binding := range projection.SigningBindings {
		if binding.ProviderID != providerID {
			continue
		}
		var aws *authbroker.AWSSigV4Binding
		if binding.AWSSigV4 != nil {
			value := *binding.AWSSigV4
			value.Target.DNSSuffixes = append([]string(nil), binding.AWSSigV4.Target.DNSSuffixes...)
			value.SecretHeaders = append([]string(nil), binding.AWSSigV4.SecretHeaders...)
			aws = &value
		}
		bindings = append(bindings, brokerCredentialBinding{
			ProviderID: providerID,
			Kind:       binding.Kind,
			AWSSigV4:   aws,
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
	instance tobari.Workspace,
) (projectAuthProjection, error) {
	if err := instance.Validate(); err != nil {
		return projectAuthProjection{}, err
	}
	if !capabilityprofile.Compiled().IncludesExperimental() {
		return projectAuthProjection{
			Environment: []string{}, Files: []projectAuthFile{}, JSONMerges: []projectAuthJSONMerge{},
			Providers: []projectAuthProviderBinding{},
		}, nil
	}
	if err := r.requireUnlockedAuthBroker(ctx); err != nil {
		return projectAuthProjection{}, err
	}
	providerProjection, err := r.loadAuthProviders()
	if err != nil {
		return projectAuthProjection{}, err
	}
	desired := projectAuthProjection{
		Environment: []string{}, Files: []projectAuthFile{}, JSONMerges: []projectAuthJSONMerge{},
		Providers: []projectAuthProviderBinding{},
	}
	for _, provider := range providerProjection.Providers {
		status, err := r.runBrokerControl(
			ctx, nil, "status", "--manifest-id", instance.WorkspaceManifestID, "--provider", provider.ID,
		)
		if err != nil {
			return projectAuthProjection{}, classifyBrokerError(err, "tobari")
		}
		switch status.State {
		case "not_configured":
			continue
		case "ready":
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
			"--manifest-id", instance.WorkspaceManifestID,
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
				rendered, renderErr := renderProviderTemplate(
					item.Template, issued.Handle, provider, issued.OAuthScopes,
					issued.ClaudeSubscriptionType, issued.ClaudeRateLimitTier,
				)
				if renderErr != nil {
					return projectAuthProjection{}, renderErr
				}
				desired.Environment = append(
					desired.Environment,
					item.Name+"="+rendered,
				)
			}
		}
		for _, item := range providerProjection.CompleteFiles {
			if item.ProviderID != provider.ID {
				continue
			}
			rendered, renderErr := renderProviderTemplate(
				item.Template, issued.Handle, provider, issued.OAuthScopes,
				issued.ClaudeSubscriptionType, issued.ClaudeRateLimitTier,
			)
			if renderErr != nil {
				return projectAuthProjection{}, renderErr
			}
			content := []byte(rendered)
			digest := sha256.Sum256(content)
			desired.Files = append(desired.Files, projectAuthFile{
				Path: item.Path, Content: content, Digest: "sha256:" + hex.EncodeToString(digest[:]),
			})
		}
		for _, item := range providerProjection.JSONMerges {
			if item.ProviderID != provider.ID {
				continue
			}
			rendered, renderErr := renderProviderTemplate(
				item.Template, issued.Handle, provider, nil,
				issued.ClaudeSubscriptionType, issued.ClaudeRateLimitTier,
			)
			if renderErr != nil {
				return projectAuthProjection{}, renderErr
			}
			fields, parseErr := projectAuthJSONMergeFields([]byte(rendered))
			if parseErr != nil {
				return projectAuthProjection{}, fault.Wrap(
					fault.KindContract, "invalid_provider_manifest",
					"The provider's Workspace JSON projection is invalid.", false, parseErr,
					fault.NextAction{Command: "doctor", Reason: "Inspect the credential-provider manifests."},
				)
			}
			desired.JSONMerges = append(desired.JSONMerges, projectAuthJSONMerge{
				Path: item.Path, Content: []byte(rendered), Fields: fields,
			})
		}
	}
	sort.Strings(desired.Environment)
	sort.Slice(desired.Files, func(left, right int) bool { return desired.Files[left].Path < desired.Files[right].Path })
	sort.Slice(desired.JSONMerges, func(left, right int) bool { return desired.JSONMerges[left].Path < desired.JSONMerges[right].Path })
	sort.Slice(desired.Providers, func(left, right int) bool {
		return desired.Providers[left].Provider < desired.Providers[right].Provider
	})
	if err := r.reconcileProjectAuthFiles(instance, desired.Files, desired.JSONMerges, desired.Providers); err != nil {
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
	instance tobari.Workspace,
	desired []projectAuthFile,
	desiredJSONMerges []projectAuthJSONMerge,
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
	desiredJSONByPath := make(map[string]projectAuthJSONMerge, len(desiredJSONMerges))
	for _, merge := range desiredJSONMerges {
		if err := authbroker.ValidateRelativeHomePath(merge.Path); err != nil {
			return fmt.Errorf("invalid Workspace authentication JSON path: %w", err)
		}
		if _, collision := desiredByPath[merge.Path]; collision {
			return fmt.Errorf("Workspace authentication projections collide on %s", merge.Path)
		}
		fields, parseErr := projectAuthJSONMergeFields(merge.Content)
		if parseErr != nil || strings.Join(fields, "\x00") != strings.Join(merge.Fields, "\x00") {
			return fmt.Errorf("Workspace authentication JSON projection is invalid")
		}
		if _, duplicate := desiredJSONByPath[merge.Path]; duplicate {
			return fmt.Errorf("Workspace authentication JSON projection path is duplicated")
		}
		desiredJSONByPath[merge.Path] = merge
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
	for _, merge := range desiredJSONMerges {
		if err := mergeProjectAuthJSON(home, merge); err != nil {
			return err
		}
	}
	for _, entry := range registry.JSONMerges {
		if _, retained := desiredJSONByPath[entry.Path]; retained {
			continue
		}
		if err := removeProjectAuthJSONFields(home, entry); err != nil {
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
		JSONMerges:    make([]projectAuthJSONMergeRegistryEntry, 0, len(desiredJSONMerges)),
	}
	copy(next.Providers, providers)
	for _, file := range desired {
		next.Files = append(next.Files, projectAuthRegistryEntry{Path: file.Path, Digest: file.Digest})
	}
	for _, merge := range desiredJSONMerges {
		next.JSONMerges = append(next.JSONMerges, projectAuthJSONMergeRegistryEntry{
			Path: merge.Path, Fields: append([]string(nil), merge.Fields...),
		})
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
			JSONMerges:    []projectAuthJSONMergeRegistryEntry{},
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
	if registry.JSONMerges == nil {
		registry.JSONMerges = []projectAuthJSONMergeRegistryEntry{}
	}
	if err := validateProjectAuthRegistry(registry, projectID); err != nil {
		return projectAuthRegistry{}, err
	}
	return registry, nil
}

func validateProjectAuthRegistry(registry projectAuthRegistry, projectID string) error {
	if registry.SchemaVersion != projectAuthRegistrySchema || registry.ProjectID != projectID || registry.Providers == nil || registry.Files == nil || registry.JSONMerges == nil {
		return fmt.Errorf("Workspace authentication file ownership is invalid")
	}
	providers := make(map[string]struct{}, len(registry.Providers))
	for _, provider := range registry.Providers {
		if authbroker.ValidateProviderID(provider.Provider) != nil || !validAuthRevision(provider.Revision) || !validSHA256(provider.BindingDigest) {
			return fmt.Errorf("Workspace authentication ownership contains an invalid provider binding")
		}
		if _, duplicate := providers[provider.Provider]; duplicate {
			return fmt.Errorf("Workspace authentication ownership contains a duplicate provider binding")
		}
		providers[provider.Provider] = struct{}{}
	}
	seen := make(map[string]struct{}, len(registry.Files))
	for _, entry := range registry.Files {
		if err := authbroker.ValidateRelativeHomePath(entry.Path); err != nil || !validSHA256(entry.Digest) {
			return fmt.Errorf("Workspace authentication file ownership contains an invalid entry")
		}
		if _, duplicate := seen[entry.Path]; duplicate {
			return fmt.Errorf("Workspace authentication file ownership contains a duplicate path")
		}
		seen[entry.Path] = struct{}{}
	}
	mergePaths := make(map[string]struct{}, len(registry.JSONMerges))
	for _, entry := range registry.JSONMerges {
		if err := authbroker.ValidateRelativeHomePath(entry.Path); err != nil || len(entry.Fields) == 0 || len(entry.Fields) > 16 {
			return fmt.Errorf("Workspace authentication JSON ownership contains an invalid entry")
		}
		if _, collision := seen[entry.Path]; collision {
			return fmt.Errorf("Workspace authentication file ownership contains a path collision")
		}
		if _, duplicate := mergePaths[entry.Path]; duplicate {
			return fmt.Errorf("Workspace authentication JSON ownership contains a duplicate path")
		}
		mergePaths[entry.Path] = struct{}{}
		last := ""
		for _, field := range entry.Fields {
			if !validProjectAuthJSONField(field) || (last != "" && field <= last) {
				return fmt.Errorf("Workspace authentication JSON ownership contains an invalid field")
			}
			last = field
		}
	}
	return nil
}

func projectAuthJSONMergeFields(content []byte) ([]string, error) {
	if len(content) == 0 || len(content) > authbroker.MaxTemplateBytes || validateNoDuplicateJSONKeys(content) != nil {
		return nil, fmt.Errorf("JSON merge must be one bounded object without duplicate keys")
	}
	var document map[string]json.RawMessage
	if err := decodeStrictJSON(content, &document); err != nil || len(document) == 0 || len(document) > 16 {
		return nil, fmt.Errorf("JSON merge must contain 1..16 top-level fields")
	}
	fields := make([]string, 0, len(document))
	for field := range document {
		if !validProjectAuthJSONField(field) {
			return nil, fmt.Errorf("JSON merge field is invalid")
		}
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields, nil
}

func validProjectAuthJSONField(field string) bool {
	if field == "" || len(field) > 64 || !utf8.ValidString(field) {
		return false
	}
	for _, character := range field {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func readProjectAuthJSON(root *os.Root, relative string) (map[string]json.RawMessage, bool, error) {
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, false, fmt.Errorf("Workspace authentication JSON target is not a private regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > 256*1024 {
		return nil, false, fmt.Errorf("Workspace authentication JSON target is empty or oversized")
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, false, fmt.Errorf("open Workspace authentication JSON target: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) || opened.Mode().Perm() != 0o600 || !opened.Mode().IsRegular() {
		return nil, false, fmt.Errorf("Workspace authentication JSON target changed while it was opened")
	}
	content, err := io.ReadAll(io.LimitReader(file, 256*1024+1))
	if err != nil || len(content) > 256*1024 || int64(len(content)) != opened.Size() || validateNoDuplicateJSONKeys(content) != nil {
		return nil, false, fmt.Errorf("Workspace authentication JSON target is invalid")
	}
	var document map[string]json.RawMessage
	if err := decodeStrictJSON(content, &document); err != nil || document == nil {
		return nil, false, fmt.Errorf("Workspace authentication JSON target is invalid")
	}
	return document, true, nil
}

func mergeProjectAuthJSON(root *os.Root, merge projectAuthJSONMerge) error {
	projected := map[string]json.RawMessage{}
	if err := decodeStrictJSON(merge.Content, &projected); err != nil {
		return fmt.Errorf("decode Workspace authentication JSON projection: %w", err)
	}
	document, _, err := readProjectAuthJSON(root, merge.Path)
	if err != nil {
		return err
	}
	changed := false
	for field, value := range projected {
		if current, exists := document[field]; !exists || !bytes.Equal(bytes.TrimSpace(current), bytes.TrimSpace(value)) {
			document[field] = append(json.RawMessage(nil), value...)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) > 256*1024 {
		return fmt.Errorf("encode Workspace authentication JSON projection")
	}
	return atomicWriteAuthFile(root, merge.Path, append(encoded, '\n'))
}

func removeProjectAuthJSONFields(root *os.Root, entry projectAuthJSONMergeRegistryEntry) error {
	document, exists, err := readProjectAuthJSON(root, entry.Path)
	if err != nil || !exists {
		return err
	}
	for _, field := range entry.Fields {
		delete(document, field)
	}
	if len(document) == 0 {
		if err := root.Remove(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove empty Workspace authentication JSON target: %w", err)
		}
		return nil
	}
	encoded, err := json.Marshal(document)
	if err != nil || len(encoded) > 256*1024 {
		return fmt.Errorf("encode Workspace authentication JSON cleanup")
	}
	return atomicWriteAuthFile(root, entry.Path, append(encoded, '\n'))
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
		ProjectID     json.RawMessage `json:"workspace_id"`
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
