package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const aggregateSchemaVersion = 1

var (
	regoPackagePattern     = regexp.MustCompile(`(?m)^package[ \t]+tobari\.http[ \t]*$`)
	regoInputSchemaPattern = regexp.MustCompile(`input\.schema_version[ \t]*==[ \t]*([0-9]+)`)
)

type aggregateProjection struct {
	Revision            string
	PolicyDirectory     string
	CredentialConfig    string
	CredentialDirectory string
	ContextCount        int
}

type aggregateContext struct {
	manifest         tobari.ContextManifest
	paths            tobari.ContextStorePaths
	data             map[string]any
	policy           policyDataFile
	rego             []byte
	creds            map[string]any
	graphqlEndpoints []tobari.GraphQLEndpoint
}

func (r *Runtime) aggregateRoot() string {
	return filepath.Join(r.stateDirectory, "cluster-projections")
}

func (r *Runtime) readAggregateContexts(ctx context.Context) ([]aggregateContext, error) {
	return r.readAggregateContextsWithTransactions(ctx, nil)
}

func (r *Runtime) readAggregateContextsWithTransactions(
	ctx context.Context, transactions map[string]*policySourceTransaction,
) ([]aggregateContext, error) {
	list, err := r.ListContexts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]aggregateContext, 0, len(list.Items))
	for _, summary := range list.Items {
		manifest, paths, err := r.resolveContext(summary.Name)
		if err != nil {
			return nil, err
		}
		var policy policyDataFile
		if transaction := transactions[paths.PolicyDirectory]; transaction != nil {
			journal, exists, journalErr := readPolicySourceJournal(paths.PolicyDirectory)
			if journalErr != nil || !exists || !reflect.DeepEqual(journal, transaction.journal) {
				return nil, fmt.Errorf("Context %q policy transaction changed during aggregate generation", manifest.Name)
			}
			policy, err = readPolicyDataDuringTransaction(paths.PolicyDirectory)
		} else {
			policy, err = readPolicyData(paths.PolicyDirectory)
		}
		if err != nil {
			return nil, fmt.Errorf("Context %q policy: %w", manifest.Name, err)
		}
		if err := validateContextPolicyLayout(paths.PolicyDirectory, manifest.PolicyMode); err != nil {
			return nil, fmt.Errorf("Context %q policy layout: %w", manifest.Name, err)
		}
		var document map[string]any
		if err := json.Unmarshal(policy.source, &document); err != nil {
			return nil, err
		}
		contextData, ok := document["tobari"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Context %q policy data has no tobari object", manifest.Name)
		}
		var rego []byte
		if manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			rego, err = runtimeassets.Read("opa/policy/tobari.rego")
		} else {
			rego, err = readOwnerPolicyFile(filepath.Join(paths.PolicyDirectory, "tobari.rego"), maxPolicyPreflight)
		}
		if err != nil {
			return nil, fmt.Errorf("Context %q policy evaluator: %w", manifest.Name, err)
		}
		creds, err := readContextCredentialDocument(paths.CredentialConfig)
		if err != nil {
			return nil, fmt.Errorf("Context %q credentials: %w", manifest.Name, err)
		}
		items = append(items, aggregateContext{
			manifest: manifest, paths: paths, data: contextData,
			policy: policy, rego: rego, creds: creds,
			graphqlEndpoints: append([]tobari.GraphQLEndpoint{}, policy.graphqlEndpoints...),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].manifest.ID < items[j].manifest.ID })
	return items, nil
}

func readContextCredentialDocument(path string) (map[string]any, error) {
	data, err := readOwnerPolicyFile(path, 256*1024)
	if err != nil {
		return nil, err
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return nil, err
	}
	document := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	if document["version"] != "v1" {
		return nil, fmt.Errorf("credentials.json version must be v1")
	}
	if _, ok := document["profiles"].(map[string]any); !ok {
		return nil, fmt.Errorf("credentials.json profiles must be an object")
	}
	return document, nil
}

func aggregateNamespace(id string) string {
	return "c" + strings.ReplaceAll(id, "-", "")
}

func transformContextRego(item aggregateContext) ([]byte, error) {
	if !regoPackagePattern.Match(item.rego) {
		return nil, fmt.Errorf("Context %q policy must declare package tobari.http", item.manifest.Name)
	}
	if bytes.Contains(item.rego, []byte("data.tobari_contexts")) || bytes.Contains(item.rego, []byte("package tobari.system")) || bytes.Contains(item.rego, []byte("package tobari.contexts")) {
		return nil, fmt.Errorf("Context %q policy crosses the reserved routing namespace", item.manifest.Name)
	}
	schemaMatches := regoInputSchemaPattern.FindAllSubmatch(item.rego, -1)
	if len(schemaMatches) != 1 || string(schemaMatches[0][1]) != "1" {
		return nil, fmt.Errorf("Context %q policy must target source input schema 1", item.manifest.Name)
	}
	packageName := "package tobari.contexts." + aggregateNamespace(item.manifest.ID) + ".http"
	if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
		packageName = "package tobari.system.guided"
	}
	transformed := regoPackagePattern.ReplaceAll(item.rego, []byte(packageName))
	transformed = bytes.ReplaceAll(transformed, []byte("data.tobari"), []byte("data.tobari_contexts[input.principal.context_id]"))
	return transformed, nil
}

func aggregateRouter(items []aggregateContext) ([]byte, error) {
	var builder strings.Builder
	builder.WriteString("package tobari.http\n\nimport rego.v1\n\n")
	builder.WriteString("default decision := {\"allow\": false, \"reason\": \"unknown or invalid Context authority\", \"credential_profile\": null, \"status_code\": 403, \"learnable\": false}\n\n")
	builder.WriteString("decision := result if {\n")
	builder.WriteString("  input.schema_version == 1\n")
	builder.WriteString("  input.principal.cluster == \"default\"\n")
	builder.WriteString("  data.tobari_contexts[input.principal.context_id]\n")
	builder.WriteString("  object.get(input.request, \"graphql\", null) != null\n")
	builder.WriteString("  result := data.tobari.system.guided.decision\n")
	builder.WriteString("}\n\n")
	for _, item := range items {
		if err := item.manifest.Validate(); err != nil {
			return nil, err
		}
		builder.WriteString("decision := result if {\n")
		builder.WriteString("  input.schema_version == 1\n")
		builder.WriteString("  input.principal.cluster == \"default\"\n")
		builder.WriteString("  input.principal.context_id == \"")
		builder.WriteString(item.manifest.ID)
		builder.WriteString("\"\n")
		builder.WriteString("  object.get(input.request, \"graphql\", null) == null\n")
		builder.WriteString("  result := data.")
		if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			builder.WriteString("tobari.system.guided")
		} else {
			builder.WriteString("tobari.contexts.")
			builder.WriteString(aggregateNamespace(item.manifest.ID))
			builder.WriteString(".http")
		}
		builder.WriteString(".decision\n}\n\n")
	}
	return []byte(builder.String()), nil
}

func rewriteCredentialProjection(item aggregateContext) (map[string]any, error) {
	profiles := item.creds["profiles"].(map[string]any)
	encoded, err := json.Marshal(profiles)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, err
	}
	for name, raw := range cloned {
		profile, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Context %q credential profile %q is invalid", item.manifest.Name, name)
		}
		secret, ok := profile["secret_file"].(string)
		if !ok || filepath.Base(secret) != strings.TrimPrefix(secret, "/run/tobari/credentials/") {
			return nil, fmt.Errorf("Context %q credential profile %q secret path is invalid", item.manifest.Name, name)
		}
		profile["secret_file"] = "/run/tobari/credentials/" + item.manifest.ID + "/" + filepath.Base(secret)
	}
	endpoints := append([]tobari.GraphQLEndpoint{}, item.graphqlEndpoints...)
	return map[string]any{
		"name":              item.manifest.Name,
		"profiles":          cloned,
		"graphql_endpoints": endpoints,
	}, nil
}

func copyCredentialFiles(source, destination string) error {
	if err := requirePrivateDirectory(source); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("credential store contains an unsafe entry")
		}
		if err := copyFileExclusive(filepath.Join(source, entry.Name()), filepath.Join(destination, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func hashCredentialFiles(digest hash.Hash, directory string) error {
	if err := requirePrivateDirectory(directory); err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("credential store contains an unsafe entry")
		}
		contents, err := readOwnerPolicyFile(filepath.Join(directory, entry.Name()), 64*1024)
		if err != nil {
			return err
		}
		digest.Write([]byte(entry.Name()))
		digest.Write([]byte{0})
		digest.Write(contents)
		digest.Write([]byte{0})
	}
	return nil
}

func copyFileExclusive(source, destination string) error {
	data, err := readOwnerPolicyFile(source, 64*1024)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, data, 0o600)
}

func (r *Runtime) buildAggregateProjection(ctx context.Context) (aggregateProjection, error) {
	return r.buildAggregateProjectionWithTransactions(ctx, nil)
}

func (r *Runtime) buildAggregateProjectionWithTransactions(
	ctx context.Context, transactions map[string]*policySourceTransaction,
) (aggregateProjection, error) {
	items, err := r.readAggregateContextsWithTransactions(ctx, transactions)
	if err != nil {
		return aggregateProjection{}, err
	}
	dataContexts := map[string]any{}
	credentialContexts := map[string]any{}
	hash := sha256.New()
	for _, item := range items {
		preflight, err := prepareContextPolicyPreflight(item.manifest, item.paths.PolicyDirectory, item.policy)
		if err != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy preflight: %w", item.manifest.Name, err)
		}
		testErr := r.testPolicyDirectory(ctx, preflight)
		_ = os.RemoveAll(preflight)
		if testErr != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q policy tests: %w", item.manifest.Name, testErr)
		}
		encoded, err := json.Marshal(item.data)
		if err != nil {
			return aggregateProjection{}, err
		}
		manifestBytes, _ := json.Marshal(item.manifest)
		hash.Write(manifestBytes)
		hash.Write([]byte{0})
		hash.Write(encoded)
		hash.Write([]byte{0})
		hash.Write(item.rego)
		hash.Write([]byte{0})
		credentialBytes, _ := json.Marshal(item.creds)
		hash.Write(credentialBytes)
		hash.Write([]byte{0})
		if err := hashCredentialFiles(hash, item.paths.CredentialDirectory); err != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q credential revision: %w", item.manifest.Name, err)
		}
		dataContexts[item.manifest.ID] = item.data
		projection, err := rewriteCredentialProjection(item)
		if err != nil {
			return aggregateProjection{}, err
		}
		credentialContexts[item.manifest.ID] = projection
	}
	revision := hex.EncodeToString(hash.Sum(nil))
	directory := filepath.Join(r.aggregateRoot(), revision)
	result := aggregateProjection{
		Revision: revision, PolicyDirectory: filepath.Join(directory, "policy"),
		CredentialConfig:    filepath.Join(directory, "credentials.json"),
		CredentialDirectory: filepath.Join(directory, "credentials"), ContextCount: len(items),
	}
	if _, err := os.Lstat(directory); err == nil {
		if err := r.testPolicyDirectory(ctx, result.PolicyDirectory); err != nil {
			return aggregateProjection{}, fmt.Errorf("validate existing aggregate policy: %w", err)
		}
		if err := verifyAggregatePolicySources(items, transactions); err != nil {
			return aggregateProjection{}, err
		}
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return aggregateProjection{}, err
	}
	if err := r.ensurePrivateDirectory(r.aggregateRoot()); err != nil {
		return aggregateProjection{}, err
	}
	temporary, err := os.MkdirTemp(r.aggregateRoot(), ".candidate-")
	if err != nil {
		return aggregateProjection{}, err
	}
	defer os.RemoveAll(temporary)
	if err := os.Chmod(temporary, 0o700); err != nil { // #nosec G302 -- candidate projection is an owner-only directory.
		return aggregateProjection{}, err
	}
	policyDirectory := filepath.Join(temporary, "policy")
	credentialDirectory := filepath.Join(temporary, "credentials")
	if err := os.MkdirAll(policyDirectory, 0o700); err != nil {
		return aggregateProjection{}, err
	}
	if err := os.MkdirAll(credentialDirectory, 0o700); err != nil {
		return aggregateProjection{}, err
	}
	router, err := aggregateRouter(items)
	if err != nil {
		return aggregateProjection{}, err
	}
	if err := os.WriteFile(filepath.Join(policyDirectory, "router.rego"), router, 0o600); err != nil {
		return aggregateProjection{}, err
	}
	canonicalGuided, err := runtimeassets.Read("opa/policy/tobari.rego")
	if err != nil {
		return aggregateProjection{}, err
	}
	guidedModule, err := transformContextRego(aggregateContext{
		manifest: tobari.ContextManifest{Name: "system", PolicyMode: tobari.ContextPolicyModeGuided},
		rego:     canonicalGuided,
	})
	if err != nil {
		return aggregateProjection{}, err
	}
	if err := os.WriteFile(filepath.Join(policyDirectory, "guided.rego"), guidedModule, 0o600); err != nil {
		return aggregateProjection{}, err
	}
	for _, item := range items {
		rego, err := transformContextRego(item)
		if err != nil {
			return aggregateProjection{}, err
		}
		regoName := aggregateNamespace(item.manifest.ID) + ".rego"
		if item.manifest.PolicyMode == tobari.ContextPolicyModeGuided {
			regoName = "guided.rego"
			if !bytes.Equal(guidedModule, rego) {
				return aggregateProjection{}, fmt.Errorf("guided Context policy logic diverged from the shared system module")
			}
		}
		if item.manifest.PolicyMode != tobari.ContextPolicyModeGuided {
			if _, err := os.Lstat(filepath.Join(policyDirectory, regoName)); errors.Is(err, os.ErrNotExist) {
				if err := os.WriteFile(filepath.Join(policyDirectory, regoName), rego, 0o600); err != nil {
					return aggregateProjection{}, err
				}
			} else if err != nil {
				return aggregateProjection{}, err
			}
		}
		if err := copyCredentialFiles(item.paths.CredentialDirectory, filepath.Join(credentialDirectory, item.manifest.ID)); err != nil {
			return aggregateProjection{}, fmt.Errorf("Context %q credential projection: %w", item.manifest.Name, err)
		}
	}
	dataDocument := map[string]any{"tobari_contexts": dataContexts, "tobari": map[string]any{
		"aggregate_schema_version": aggregateSchemaVersion,
		"aggregate_revision":       revision,
	}}
	if err := writeAtomicJSON(filepath.Join(policyDirectory, "data.json"), dataDocument); err != nil {
		return aggregateProjection{}, err
	}
	credentialDocument := map[string]any{"version": "v1", "contexts": credentialContexts}
	if err := writeAtomicJSON(filepath.Join(temporary, "credentials.json"), credentialDocument); err != nil {
		return aggregateProjection{}, err
	}
	candidatePolicy := filepath.Join(temporary, "policy")
	if err := r.testPolicyDirectory(ctx, candidatePolicy); err != nil {
		return aggregateProjection{}, fmt.Errorf("validate aggregate policy: %w", err)
	}
	if err := verifyAggregatePolicySources(items, transactions); err != nil {
		return aggregateProjection{}, err
	}
	if err := os.Rename(temporary, directory); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return aggregateProjection{}, err
		}
	}
	return result, nil
}

func verifyAggregatePolicySources(
	items []aggregateContext, transactions map[string]*policySourceTransaction,
) error {
	for _, item := range items {
		var current policyDataFile
		var err error
		if transactions[item.paths.PolicyDirectory] != nil {
			current, err = readPolicyDataDuringTransaction(item.paths.PolicyDirectory)
		} else {
			current, err = readPolicyData(item.paths.PolicyDirectory)
		}
		if err != nil || !reflect.DeepEqual(current.sources, item.policy.sources) {
			return fmt.Errorf("Context %q policy source changed during aggregate generation", item.manifest.Name)
		}
	}
	return nil
}
