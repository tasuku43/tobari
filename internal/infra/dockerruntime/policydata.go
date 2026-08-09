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
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	maxPolicyDataBytes    = 1024 * 1024
	maxPolicyPreflight    = 4 * 1024 * 1024
	maxPolicyFiles        = 128
	policySchemaVersion   = 2
	policyRulesDataName   = "rules"
	learnedPolicyDataName = "learned_allows"
	learnedDenyDataName   = "learned_denies"
)

type policyDataFile struct {
	document          map[string]json.RawMessage
	tobari            map[string]json.RawMessage
	ruleData          map[string]json.RawMessage
	rules             []tobari.LearnedPolicyRule
	baselineDenyRules []tobari.PolicyBaselineDenyRule
	denyRules         []tobari.PolicyDenyRule
	graphqlEndpoints  []tobari.GraphQLEndpoint
	source            []byte
}

func validateNoDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var parseValue func() error
	parseValue = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, isDelimiter := token.(json.Delim)
		if !isDelimiter {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("JSON object key is not a string")
				}
				if seen[key] {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = true
				if err := parseValue(); err != nil {
					return err
				}
			}
			closing, err := decoder.Token()
			if err != nil || closing != json.Delim('}') {
				return fmt.Errorf("JSON object is not closed")
			}
		case '[':
			for decoder.More() {
				if err := parseValue(); err != nil {
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
	if err := parseValue(); err != nil {
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

func decodePolicyObject(raw json.RawMessage, name string) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	value := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	return value, nil
}

func decodePolicyArray(object map[string]json.RawMessage, name string) ([]json.RawMessage, error) {
	raw, exists := object[name]
	if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	value := []json.RawMessage{}
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, fmt.Errorf("%s must be an array", name)
	}
	return value, nil
}

func validatePolicyDataShape(tobariData map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	var schemaVersion int
	if err := json.Unmarshal(tobariData["schema_version"], &schemaVersion); err != nil || schemaVersion != policySchemaVersion {
		return nil, fmt.Errorf("data.json schema_version must be %d", policySchemaVersion)
	}
	boundary, err := decodePolicyObject(tobariData["boundary"], "data.json boundary")
	if err != nil {
		return nil, err
	}
	if _, err := decodePolicyObject(boundary["ports"], "data.json boundary.ports"); err != nil {
		return nil, err
	}
	if _, err := decodePolicyArray(boundary, "authorities"); err != nil {
		return nil, fmt.Errorf("data.json boundary: %w", err)
	}
	if raw, exists := boundary["graphql_endpoints"]; exists {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("data.json boundary.graphql_endpoints must be an array")
		}
		var endpoints []tobari.GraphQLEndpoint
		if err := json.Unmarshal(raw, &endpoints); err != nil || endpoints == nil {
			return nil, fmt.Errorf("data.json boundary.graphql_endpoints must be an array")
		}
		seen := map[tobari.GraphQLEndpoint]struct{}{}
		for _, endpoint := range endpoints {
			if err := endpoint.Validate(); err != nil {
				return nil, fmt.Errorf("data.json boundary.graphql_endpoints: %w", err)
			}
			if _, duplicate := seen[endpoint]; duplicate {
				return nil, fmt.Errorf("data.json boundary.graphql_endpoints contains a duplicate endpoint")
			}
			seen[endpoint] = struct{}{}
		}
	}
	methods, err := decodePolicyObject(boundary["methods"], "data.json boundary.methods")
	if err != nil {
		return nil, err
	}
	if _, err := decodePolicyArray(methods, "read"); err != nil {
		return nil, fmt.Errorf("data.json boundary.methods: %w", err)
	}
	if _, err := decodePolicyArray(methods, "write"); err != nil {
		return nil, fmt.Errorf("data.json boundary.methods: %w", err)
	}
	if _, err := decodePolicyObject(tobariData["credentials"], "data.json credentials"); err != nil {
		return nil, err
	}
	ruleData, err := decodePolicyObject(tobariData[policyRulesDataName], "data.json rules")
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"baseline_denies", learnedPolicyDataName, learnedDenyDataName} {
		if _, err := decodePolicyArray(ruleData, name); err != nil {
			return nil, fmt.Errorf("data.json rules: %w", err)
		}
	}
	return ruleData, nil
}

func readOwnerPolicyFile(path string, maximum int) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s must be a regular owner-only file", filepath.Base(path))
	}
	if info.Size() > int64(maximum) {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), maximum)
	}
	file, err := os.Open(path) // #nosec G304 -- caller supplies an exact state-owned policy child.
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspect opened %s: %w", filepath.Base(path), err)
	}
	if !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 ||
		!os.SameFile(info, opened) {
		return nil, fmt.Errorf("%s changed during safe open", filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(maximum)+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if len(data) > maximum {
		return nil, fmt.Errorf("%s exceeds %d bytes", filepath.Base(path), maximum)
	}
	return data, nil
}

func validateOwnerPolicyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect policy directory: %w", err)
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("policy directory must be an owner-only directory")
	}
	return nil
}

func readPolicyData(policyDirectory string) (policyDataFile, error) {
	if err := validateOwnerPolicyDirectory(policyDirectory); err != nil {
		return policyDataFile{}, err
	}
	path := filepath.Join(policyDirectory, "data.json")
	data, err := readOwnerPolicyFile(path, maxPolicyDataBytes)
	if err != nil {
		return policyDataFile{}, err
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return policyDataFile{}, fmt.Errorf("validate data.json: %w", err)
	}
	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &document); err != nil {
		return policyDataFile{}, fmt.Errorf("decode data.json: %w", err)
	}
	rawTobari, exists := document["tobari"]
	if !exists {
		return policyDataFile{}, fmt.Errorf("data.json must contain a tobari object")
	}
	tobariData, err := decodePolicyObject(rawTobari, "data.json tobari")
	if err != nil {
		return policyDataFile{}, fmt.Errorf("decode data.json tobari object: %w", err)
	}
	ruleData, err := validatePolicyDataShape(tobariData)
	if err != nil {
		return policyDataFile{}, err
	}
	graphqlEndpoints := []tobari.GraphQLEndpoint{}
	boundary, _ := decodePolicyObject(tobariData["boundary"], "data.json boundary")
	if raw, exists := boundary["graphql_endpoints"]; exists {
		if err := json.Unmarshal(raw, &graphqlEndpoints); err != nil {
			return policyDataFile{}, fmt.Errorf("decode graphql_endpoints: %w", err)
		}
	}
	rules := []tobari.LearnedPolicyRule{}
	if rawRules := ruleData[learnedPolicyDataName]; rawRules != nil {
		if err := json.Unmarshal(rawRules, &rules); err != nil {
			return policyDataFile{}, fmt.Errorf("decode %s: %w", learnedPolicyDataName, err)
		}
	}
	// Rules written before Context became an authority scope cannot be safely
	// assigned to any Context. Keep them inert and require a fresh denial rather
	// than guessing. Any partially scoped entry remains an error.
	rules = slices.DeleteFunc(rules, func(rule tobari.LearnedPolicyRule) bool {
		return rule.ContextID == "" && rule.ContextName == "" && rule.ProjectRoot == ""
	})
	if err := tobari.ValidateLearnedPolicyRules(rules); err != nil {
		return policyDataFile{}, fmt.Errorf("validate %s: %w", learnedPolicyDataName, err)
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	baselineDenyRules := []tobari.PolicyBaselineDenyRule{}
	if rawDenyRules := ruleData["baseline_denies"]; rawDenyRules != nil {
		if err := json.Unmarshal(rawDenyRules, &baselineDenyRules); err != nil {
			return policyDataFile{}, fmt.Errorf("decode baseline_denies: %w", err)
		}
	}
	for _, rule := range baselineDenyRules {
		if err := rule.Validate(); err != nil {
			return policyDataFile{}, fmt.Errorf("validate baseline_denies: %w", err)
		}
	}
	denyRules := []tobari.PolicyDenyRule{}
	if rawDenyRules := ruleData[learnedDenyDataName]; rawDenyRules != nil {
		if err := json.Unmarshal(rawDenyRules, &denyRules); err != nil {
			return policyDataFile{}, fmt.Errorf("decode %s: %w", learnedDenyDataName, err)
		}
	}
	denyRules = slices.DeleteFunc(denyRules, func(rule tobari.PolicyDenyRule) bool {
		return rule.ContextID == "" && rule.ContextName == "" && rule.ProjectRoot == ""
	})
	denyRuleSet := tobari.PolicyDenyRuleSet{Baseline: baselineDenyRules, Exact: denyRules}
	if err := denyRuleSet.Validate(); err != nil {
		return policyDataFile{}, fmt.Errorf("validate %s: %w", learnedDenyDataName, err)
	}
	sort.Slice(denyRules, func(i, j int) bool { return denyRules[i].ID < denyRules[j].ID })
	return policyDataFile{
		document: document, tobari: tobariData, ruleData: ruleData, rules: rules,
		baselineDenyRules: baselineDenyRules, denyRules: denyRules,
		graphqlEndpoints: append([]tobari.GraphQLEndpoint{}, graphqlEndpoints...),
		source:           append([]byte{}, data...),
	}, nil
}

func (f policyDataFile) withPolicyRules(
	rules []tobari.LearnedPolicyRule, denyRules []tobari.PolicyDenyRule,
) ([]byte, error) {
	if err := tobari.ValidateLearnedPolicyRules(rules); err != nil {
		return nil, err
	}
	denyRuleSet := tobari.PolicyDenyRuleSet{Baseline: f.baselineDenyRules, Exact: denyRules}
	if err := denyRuleSet.Validate(); err != nil {
		return nil, err
	}
	rules = append([]tobari.LearnedPolicyRule{}, rules...)
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	denyRules = append([]tobari.PolicyDenyRule{}, denyRules...)
	sort.Slice(denyRules, func(i, j int) bool { return denyRules[i].ID < denyRules[j].ID })
	rawRules, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	ruleData := make(map[string]json.RawMessage, len(f.ruleData)+2)
	for key, value := range f.ruleData {
		ruleData[key] = append(json.RawMessage{}, value...)
	}
	ruleData[learnedPolicyDataName] = rawRules
	rawDenyRules, err := json.Marshal(denyRules)
	if err != nil {
		return nil, err
	}
	ruleData[learnedDenyDataName] = rawDenyRules
	rawRuleData, err := json.Marshal(ruleData)
	if err != nil {
		return nil, err
	}
	tobariData := make(map[string]json.RawMessage, len(f.tobari)+1)
	for key, value := range f.tobari {
		tobariData[key] = append(json.RawMessage{}, value...)
	}
	tobariData[policyRulesDataName] = rawRuleData
	rawTobari, err := json.Marshal(tobariData)
	if err != nil {
		return nil, err
	}
	document := make(map[string]json.RawMessage, len(f.document))
	for key, value := range f.document {
		document[key] = append(json.RawMessage{}, value...)
	}
	document["tobari"] = rawTobari
	output, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(output, '\n'), nil
}

func (f policyDataFile) withRules(rules []tobari.LearnedPolicyRule) ([]byte, error) {
	return f.withPolicyRules(rules, f.denyRules)
}

func (f policyDataFile) withDenyRules(denyRules []tobari.PolicyDenyRule) ([]byte, error) {
	return f.withPolicyRules(f.rules, denyRules)
}

// ReadLearnedPolicyRules returns the validated CLI-owned rule collection while
// preserving absence as a known empty collection.
func (r *Runtime) ReadLearnedPolicyRules(
	ctx context.Context, state tobari.State,
) ([]tobari.LearnedPolicyRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := state.Validate(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(state.PolicyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		file, err := readPolicyData(state.PolicyDirectory)
		if err != nil {
			return nil, err
		}
		return append([]tobari.LearnedPolicyRule{}, file.rules...), nil
	}
	contexts, err := r.readAggregateContexts(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]tobari.LearnedPolicyRule, 0)
	for _, item := range contexts {
		file, err := readPolicyData(item.paths.PolicyDirectory)
		if err != nil {
			return nil, fmt.Errorf("Context %q policy: %w", item.manifest.Name, err)
		}
		for _, rule := range file.rules {
			if rule.ContextID != item.manifest.ID || rule.ContextName != item.manifest.Name {
				return nil, fmt.Errorf("Context %q learned rule has mismatched authority scope", item.manifest.Name)
			}
			rules = append(rules, rule)
		}
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].ID < rules[j].ID })
	return rules, tobari.ValidateLearnedPolicyRules(rules)
}

// ReadPolicyDenyRules returns both trusted host-authored baseline deny
// matchers and the validated CLI-owned exact deny collection.
func (r *Runtime) ReadPolicyDenyRules(
	ctx context.Context, state tobari.State,
) (tobari.PolicyDenyRuleSet, error) {
	if err := ctx.Err(); err != nil {
		return tobari.PolicyDenyRuleSet{}, err
	}
	if err := state.Validate(); err != nil {
		return tobari.PolicyDenyRuleSet{}, err
	}
	if !strings.HasPrefix(state.PolicyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		file, err := readPolicyData(state.PolicyDirectory)
		if err != nil {
			return tobari.PolicyDenyRuleSet{}, err
		}
		result := tobari.PolicyDenyRuleSet{
			Baseline: append([]tobari.PolicyBaselineDenyRule{}, file.baselineDenyRules...),
			Exact:    append([]tobari.PolicyDenyRule{}, file.denyRules...),
		}
		return result, result.Validate()
	}
	contexts, err := r.readAggregateContexts(ctx)
	if err != nil {
		return tobari.PolicyDenyRuleSet{}, err
	}
	result := tobari.PolicyDenyRuleSet{Baseline: []tobari.PolicyBaselineDenyRule{}, Exact: []tobari.PolicyDenyRule{}}
	for _, item := range contexts {
		file, err := readPolicyData(item.paths.PolicyDirectory)
		if err != nil {
			return tobari.PolicyDenyRuleSet{}, fmt.Errorf("Context %q policy: %w", item.manifest.Name, err)
		}
		for _, baseline := range file.baselineDenyRules {
			baseline.ContextID = item.manifest.ID
			result.Baseline = append(result.Baseline, baseline)
		}
		for _, rule := range file.denyRules {
			if rule.ContextID != item.manifest.ID || rule.ContextName != item.manifest.Name {
				return tobari.PolicyDenyRuleSet{}, fmt.Errorf("Context %q exact deny has mismatched authority scope", item.manifest.Name)
			}
			result.Exact = append(result.Exact, rule)
		}
	}
	sort.Slice(result.Exact, func(i, j int) bool { return result.Exact[i].ID < result.Exact[j].ID })
	return result, result.Validate()
}

func copyPolicyForPreflight(sourceDirectory string, dataJSON []byte) (string, error) {
	if err := validateOwnerPolicyDirectory(sourceDirectory); err != nil {
		return "", err
	}
	parent := filepath.Dir(sourceDirectory)
	temporary, err := os.MkdirTemp(parent, ".tobari-policy-preflight-*")
	if err != nil {
		return "", fmt.Errorf("create policy preflight directory: %w", err)
	}
	cleanup := func(cause error) (string, error) {
		_ = os.RemoveAll(temporary)
		return "", cause
	}
	total, files := 0, 0
	err = filepath.WalkDir(sourceDirectory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDirectory, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		target := filepath.Join(temporary, relative)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if info.Mode().Perm()&0o077 != 0 {
				return fmt.Errorf("policy path %s must be an owner-only directory", relative)
			}
			return os.Mkdir(target, 0o700)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return fmt.Errorf("policy path %s must be a regular owner-only file", relative)
		}
		files++
		if files > maxPolicyFiles {
			return fmt.Errorf("policy directory exceeds %d files", maxPolicyFiles)
		}
		data := dataJSON
		if relative != "data.json" {
			data, err = readOwnerPolicyFile(path, maxPolicyPreflight-total)
			if err != nil {
				return err
			}
		}
		total += len(data)
		if total > maxPolicyPreflight {
			return fmt.Errorf("policy directory exceeds %d bytes", maxPolicyPreflight)
		}
		if err := os.WriteFile(target, data, 0o600); err != nil { // #nosec G306 -- policy copies are owner-only.
			return err
		}
		return nil
	})
	if err != nil {
		return cleanup(fmt.Errorf("copy policy for preflight: %w", err))
	}
	if files == 0 {
		return cleanup(fmt.Errorf("policy directory is empty"))
	}
	return temporary, nil
}

func prepareContextPolicyPreflight(
	manifest tobari.ContextManifest, sourceDirectory string, dataJSON []byte,
) (string, error) {
	if manifest.PolicyMode == tobari.ContextPolicyModeAdvanced {
		return copyPolicyForPreflight(sourceDirectory, dataJSON)
	}
	if manifest.PolicyMode != tobari.ContextPolicyModeGuided {
		return "", fmt.Errorf("Context policy mode is invalid")
	}
	if err := validateOwnerPolicyDirectory(sourceDirectory); err != nil {
		return "", err
	}
	if _, err := readPolicyData(sourceDirectory); err != nil {
		return "", err
	}
	rego, err := runtimeassets.Read("opa/policy/tobari.rego")
	if err != nil {
		return "", err
	}
	tests, err := runtimeassets.Read("opa/policy/tobari_test.rego")
	if err != nil {
		return "", err
	}
	if len(dataJSON)+len(rego)+len(tests) > maxPolicyPreflight {
		return "", fmt.Errorf("guided policy preflight exceeds %d bytes", maxPolicyPreflight)
	}
	temporary, err := os.MkdirTemp(filepath.Dir(sourceDirectory), ".tobari-guided-preflight-*")
	if err != nil {
		return "", fmt.Errorf("create guided policy preflight directory: %w", err)
	}
	cleanup := func(cause error) (string, error) {
		_ = os.RemoveAll(temporary)
		return "", cause
	}
	if err := os.Chmod(temporary, 0o700); err != nil { // #nosec G302 -- guided preflight must remain owner-only.
		return cleanup(err)
	}
	for name, data := range map[string][]byte{
		"data.json": dataJSON, "tobari.rego": rego, "tobari_test.rego": tests,
	} {
		if err := os.WriteFile(filepath.Join(temporary, name), data, 0o600); err != nil { // #nosec G306 -- preflight is owner-only.
			return cleanup(err)
		}
	}
	return temporary, nil
}

func atomicWriteOwnerFile(path string, data []byte) error {
	directory := filepath.Dir(path)
	file, err := os.CreateTemp(directory, ".data.json.tobari-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() { _ = os.Remove(temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	handle, err := os.Open(directory) // #nosec G304 -- exact state-owned policy directory.
	if err != nil {
		return err
	}
	defer handle.Close()
	return handle.Sync()
}

// ApplyLearnedPolicyRules tests a complete private policy copy, atomically
// replaces only data.json, then uses the portable OPA activation boundary.
func (r *Runtime) ApplyLearnedPolicyRules(
	ctx context.Context, state tobari.State,
	expected, updated []tobari.LearnedPolicyRule,
) error {
	return r.applyPolicyData(ctx, state, expected, updated, nil, nil, false)
}

// ApplyPolicyDenyRules tests and atomically activates one complete policy data
// update while requiring the exact-deny snapshot used by discovery to remain
// unchanged.
func (r *Runtime) ApplyPolicyDenyRules(
	ctx context.Context, state tobari.State,
	expectedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) error {
	return r.applyPolicyData(ctx, state, expectedAllows, nil, expectedDenies, updatedDenies, true)
}

func (r *Runtime) applyPolicyData(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
	checkDenySnapshot bool,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if strings.HasPrefix(state.PolicyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		return r.applyAggregatePolicyData(
			ctx, state, expectedAllows, updatedAllows, expectedDenies, updatedDenies, checkDenySnapshot,
		)
	}
	if err := tobari.ValidateLearnedPolicyRules(expectedAllows); err != nil {
		return err
	}
	if updatedAllows == nil {
		updatedAllows = expectedAllows
	}
	if err := tobari.ValidateLearnedPolicyRules(updatedAllows); err != nil {
		return err
	}
	file, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"host policy data is not safe for managed learning", false, err,
		)
	}
	expectedAllows = append([]tobari.LearnedPolicyRule{}, expectedAllows...)
	sort.Slice(expectedAllows, func(i, j int) bool { return expectedAllows[i].ID < expectedAllows[j].ID })
	if !reflect.DeepEqual(file.rules, expectedAllows) {
		return fault.New(
			fault.KindRejected, "policy_data_changed",
			"learned policy rules changed after discovery", false,
		)
	}
	if !checkDenySnapshot {
		expectedDenies = append([]tobari.PolicyDenyRule{}, file.denyRules...)
		updatedDenies = append([]tobari.PolicyDenyRule{}, file.denyRules...)
	}
	denyExpected := tobari.PolicyDenyRuleSet{
		Baseline: file.baselineDenyRules, Exact: expectedDenies,
	}
	denyUpdated := tobari.PolicyDenyRuleSet{
		Baseline: file.baselineDenyRules, Exact: updatedDenies,
	}
	if err := denyExpected.Validate(); err != nil {
		return fault.Wrap(fault.KindRejected, "policy_data_invalid", "host policy deny data is invalid", false, err)
	}
	if !reflect.DeepEqual(file.denyRules, expectedDenies) {
		return fault.New(
			fault.KindRejected, "policy_data_changed",
			"policy deny rules changed after discovery", false,
		)
	}
	if err := denyUpdated.Validate(); err != nil {
		return fault.Wrap(fault.KindContract, "invalid_policy_deny", "exact policy deny update is invalid", false, err)
	}
	data, err := file.withPolicyRules(updatedAllows, updatedDenies)
	if err != nil {
		code := "invalid_learned_policy"
		message := "learned policy update is invalid"
		if checkDenySnapshot {
			code = "invalid_policy_deny"
			message = "exact policy deny update is invalid"
		}
		return fault.Wrap(
			fault.KindContract, code, message, false, err,
		)
	}
	preflight, err := copyPolicyForPreflight(state.PolicyDirectory, data)
	if err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_preflight_failed",
			"candidate policy could not be prepared for testing", false, err,
		)
	}
	defer func() { _ = os.RemoveAll(preflight) }()
	if err := r.testPolicyDirectory(ctx, preflight); err != nil {
		return fault.Wrap(
			fault.KindRejected, "policy_preflight_failed",
			"candidate policy failed OPA tests", false, err,
		)
	}
	current, err := readOwnerPolicyFile(
		filepath.Join(state.PolicyDirectory, "data.json"), maxPolicyDataBytes,
	)
	if err != nil || !bytes.Equal(current, file.source) {
		return fault.Wrap(
			fault.KindRejected, "policy_data_changed",
			"policy data changed while the candidate was being tested", false, err,
		)
	}
	if err := atomicWriteOwnerFile(filepath.Join(state.PolicyDirectory, "data.json"), data); err != nil {
		return fault.Wrap(
			fault.KindInternal, "policy_write_failed",
			"tested policy data could not be written atomically", false, err,
		)
	}
	if err := r.activatePolicyRevision(ctx, state, policyAuthorityReduces(expectedAllows, updatedAllows, expectedDenies, updatedDenies)); err != nil {
		return err
	}
	return nil
}

func policyAuthorityReduces(
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) bool {
	updatedAllowIDs := make(map[string]struct{}, len(updatedAllows))
	for _, rule := range updatedAllows {
		updatedAllowIDs[rule.ID] = struct{}{}
	}
	for _, rule := range expectedAllows {
		if _, retained := updatedAllowIDs[rule.ID]; !retained {
			return true
		}
	}
	expectedDenyIDs := make(map[string]struct{}, len(expectedDenies))
	for _, rule := range expectedDenies {
		expectedDenyIDs[rule.ID] = struct{}{}
	}
	for _, rule := range updatedDenies {
		if _, existed := expectedDenyIDs[rule.ID]; !existed {
			return true
		}
	}
	return false
}

func policyMutationContexts(
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) ([]string, error) {
	ids := map[string]struct{}{}
	allowExpected := map[string]tobari.LearnedPolicyRule{}
	for _, rule := range expectedAllows {
		allowExpected[rule.ID] = rule
	}
	allowUpdated := map[string]tobari.LearnedPolicyRule{}
	for _, rule := range updatedAllows {
		allowUpdated[rule.ID] = rule
		if previous, ok := allowExpected[rule.ID]; !ok || !reflect.DeepEqual(previous, rule) {
			ids[rule.ContextID] = struct{}{}
		}
	}
	for id, rule := range allowExpected {
		if _, ok := allowUpdated[id]; !ok {
			ids[rule.ContextID] = struct{}{}
		}
	}
	denyExpected := map[string]tobari.PolicyDenyRule{}
	for _, rule := range expectedDenies {
		denyExpected[rule.ID] = rule
	}
	denyUpdated := map[string]tobari.PolicyDenyRule{}
	for _, rule := range updatedDenies {
		denyUpdated[rule.ID] = rule
		if previous, ok := denyExpected[rule.ID]; !ok || !reflect.DeepEqual(previous, rule) {
			ids[rule.ContextID] = struct{}{}
		}
	}
	for id, rule := range denyExpected {
		if _, ok := denyUpdated[id]; !ok {
			ids[rule.ContextID] = struct{}{}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("policy mutation has no Context target")
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		result = append(result, id)
	}
	sort.Strings(result)
	return result, nil
}

func rulesForContext(rules []tobari.LearnedPolicyRule, contextID string) []tobari.LearnedPolicyRule {
	result := make([]tobari.LearnedPolicyRule, 0)
	for _, rule := range rules {
		if rule.ContextID == contextID {
			result = append(result, rule)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func deniesForContext(rules []tobari.PolicyDenyRule, contextID string) []tobari.PolicyDenyRule {
	result := make([]tobari.PolicyDenyRule, 0)
	for _, rule := range rules {
		if rule.ContextID == contextID {
			result = append(result, rule)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (r *Runtime) applyAggregatePolicyData(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
	checkDenySnapshot bool,
) error {
	if updatedAllows == nil {
		updatedAllows = expectedAllows
	}
	if !checkDenySnapshot {
		currentDenies, err := r.ReadPolicyDenyRules(ctx, state)
		if err != nil {
			return err
		}
		expectedDenies = currentDenies.Exact
		updatedDenies = currentDenies.Exact
	}
	if err := tobari.ValidateLearnedPolicyRules(expectedAllows); err != nil {
		return err
	}
	if err := tobari.ValidateLearnedPolicyRules(updatedAllows); err != nil {
		return err
	}
	targetContexts, err := policyMutationContexts(expectedAllows, updatedAllows, expectedDenies, updatedDenies)
	if err != nil {
		return fault.Wrap(fault.KindContract, "invalid_policy_scope", "policy mutation Context scope is invalid", false, err)
	}
	if checkDenySnapshot && len(targetContexts) != 1 {
		return fault.New(
			fault.KindRejected, "policy_review_scope_mixed",
			"one reviewed Apply cannot span multiple Context policy sources", false,
		)
	}
	return r.withPolicyProjectionLock(ctx, func() error {
		stored, configured, err := r.LoadState(ctx)
		if err != nil || !configured {
			return fault.Wrap(fault.KindRejected, "policy_state_changed", "shared policy state changed after discovery", false, err)
		}
		if stored.AggregateRevision != state.AggregateRevision {
			return fault.New(fault.KindRejected, "policy_data_changed", "aggregate policy changed after discovery", false)
		}
		currentAllows, err := r.ReadLearnedPolicyRules(ctx, stored)
		if err != nil {
			return err
		}
		currentDenies, err := r.ReadPolicyDenyRules(ctx, stored)
		if err != nil {
			return err
		}
		sort.Slice(expectedAllows, func(i, j int) bool { return expectedAllows[i].ID < expectedAllows[j].ID })
		sort.Slice(expectedDenies, func(i, j int) bool { return expectedDenies[i].ID < expectedDenies[j].ID })
		if !reflect.DeepEqual(currentAllows, expectedAllows) || !reflect.DeepEqual(currentDenies.Exact, expectedDenies) {
			return fault.New(fault.KindRejected, "policy_data_changed", "policy decisions changed after discovery", false)
		}
		type contextUpdate struct {
			sourcePath string
			original   []byte
			candidate  []byte
		}
		updates := make([]contextUpdate, 0, len(targetContexts))
		for _, targetContext := range targetContexts {
			manifest, paths, err := r.contextByID(targetContext)
			if err != nil {
				return fault.Wrap(fault.KindRejected, "context_unavailable", "policy target Context is unavailable", false, err)
			}
			file, err := readPolicyData(paths.PolicyDirectory)
			if err != nil {
				return err
			}
			contextAllows := rulesForContext(updatedAllows, targetContext)
			contextDenies := deniesForContext(updatedDenies, targetContext)
			for _, rule := range contextAllows {
				if rule.ContextName != manifest.Name {
					return fault.New(fault.KindContract, "context_mismatch", "learned rule Context binding is inconsistent", false)
				}
			}
			for _, rule := range contextDenies {
				if rule.ContextName != manifest.Name {
					return fault.New(fault.KindContract, "context_mismatch", "deny rule Context binding is inconsistent", false)
				}
			}
			data, err := file.withPolicyRules(contextAllows, contextDenies)
			if err != nil {
				return err
			}
			preflight, err := prepareContextPolicyPreflight(manifest, paths.PolicyDirectory, data)
			if err != nil {
				return err
			}
			testErr := r.testPolicyDirectory(ctx, preflight)
			_ = os.RemoveAll(preflight)
			if testErr != nil {
				return fault.Wrap(fault.KindRejected, "policy_preflight_failed", "candidate Context policy failed OPA tests", false, testErr)
			}
			updates = append(updates, contextUpdate{
				sourcePath: filepath.Join(paths.PolicyDirectory, "data.json"),
				original:   append([]byte{}, file.source...), candidate: data,
			})
		}
		rollback := func() {
			for _, update := range updates {
				_ = atomicWriteOwnerFile(update.sourcePath, update.original)
			}
			rollbackContext, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			_ = r.activatePolicyRevision(rollbackContext, stored, false)
		}
		for index, update := range updates {
			current, readErr := readOwnerPolicyFile(update.sourcePath, maxPolicyDataBytes)
			if readErr != nil || !bytes.Equal(current, update.original) {
				return fault.Wrap(fault.KindRejected, "policy_data_changed", "policy data changed while the reviewed set was being tested", false, readErr)
			}
			if err := atomicWriteOwnerFile(update.sourcePath, update.candidate); err != nil {
				for rollbackIndex := 0; rollbackIndex < index; rollbackIndex++ {
					_ = atomicWriteOwnerFile(updates[rollbackIndex].sourcePath, updates[rollbackIndex].original)
				}
				return err
			}
		}
		projection, err := r.buildAggregateProjection(ctx)
		if err != nil {
			for _, update := range updates {
				_ = atomicWriteOwnerFile(update.sourcePath, update.original)
			}
			return fault.Wrap(fault.KindRejected, "aggregate_policy_invalid", "candidate aggregate policy was not activated", false, err)
		}
		candidateState := stored
		candidateState.AggregateRevision = projection.Revision
		candidateState.ContextCount = projection.ContextCount
		candidateState.PolicyDirectory = projection.PolicyDirectory
		candidateState.CredentialConfig = projection.CredentialConfig
		candidateState.CredentialDir = projection.CredentialDirectory
		if err := r.activatePolicyRevision(
			ctx, candidateState,
			policyAuthorityReduces(expectedAllows, updatedAllows, expectedDenies, updatedDenies),
		); err != nil {
			rollback()
			return err
		}
		if err := r.writeState(candidateState); err != nil {
			rollback()
			return fmt.Errorf("persist aggregate policy activation: %w", err)
		}
		return nil
	})
}

// ApplyPolicyDecisionSet records a bounded reviewed set in one Context source
// and performs exactly one aggregate activation. The one-source bound keeps
// source promotion atomic across process interruption.
func (r *Runtime) ApplyPolicyDecisionSet(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) error {
	return r.applyAggregatePolicyData(
		ctx, state, expectedAllows, updatedAllows, expectedDenies, updatedDenies, true,
	)
}
