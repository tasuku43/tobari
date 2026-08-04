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
	"sort"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
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
	rules := []tobari.LearnedPolicyRule{}
	if rawRules := ruleData[learnedPolicyDataName]; rawRules != nil {
		if err := json.Unmarshal(rawRules, &rules); err != nil {
			return policyDataFile{}, fmt.Errorf("decode %s: %w", learnedPolicyDataName, err)
		}
	}
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
	denyRuleSet := tobari.PolicyDenyRuleSet{Baseline: baselineDenyRules, Exact: denyRules}
	if err := denyRuleSet.Validate(); err != nil {
		return policyDataFile{}, fmt.Errorf("validate %s: %w", learnedDenyDataName, err)
	}
	sort.Slice(denyRules, func(i, j int) bool { return denyRules[i].ID < denyRules[j].ID })
	return policyDataFile{
		document: document, tobari: tobariData, ruleData: ruleData, rules: rules,
		baselineDenyRules: baselineDenyRules, denyRules: denyRules,
		source: append([]byte{}, data...),
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
	file, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		return nil, err
	}
	return append([]tobari.LearnedPolicyRule{}, file.rules...), nil
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
	if err := r.ApplyPolicy(ctx, state); err != nil {
		return err
	}
	return nil
}
