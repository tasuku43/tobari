package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
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
	policySchemaVersion   = 1
	learnedPolicyDataName = "learned_allows"
	learnedDenyDataName   = "learned_denies"
	policyDomainsName     = "domains"
	policyAllowFileName   = "allow.json"
	policyDenyFileName    = "deny.json"
)

type policyDataFile struct {
	rules            []tobari.LearnedPolicyRule
	denyRules        []tobari.PolicyDenyRule
	graphqlEndpoints []tobari.GraphQLEndpoint
	allows           map[string]policyDomainAllow
	denies           map[string]policyDomainDeny
	sources          map[string][]byte
	source           []byte
}

type policyDomainAllow struct {
	SchemaVersion    int                        `json:"schema_version"`
	Host             string                     `json:"host"`
	GraphQLEndpoints []tobari.GraphQLEndpoint   `json:"graphql_endpoints"`
	Rules            []tobari.LearnedPolicyRule `json:"rules"`
}

type policyDomainDeny struct {
	SchemaVersion int                     `json:"schema_version"`
	Host          string                  `json:"host"`
	Rules         []tobari.PolicyDenyRule `json:"rules"`
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

func validatePolicyDomain(domain string) error {
	if domain == "" || len(domain) > 253 || domain != strings.ToLower(domain) ||
		domain == "." || domain == ".." || strings.ContainsAny(domain, "/\\*") || containsSpaceOrControl(domain) ||
		net.ParseIP(domain) != nil {
		return fmt.Errorf("policy domain is not a canonical lowercase host")
	}
	for _, label := range strings.Split(domain, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("policy domain is not a canonical lowercase host")
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return fmt.Errorf("policy domain is not a canonical lowercase host")
			}
		}
	}
	return nil
}

func containsSpaceOrControl(value string) bool {
	for _, character := range value {
		if character <= ' ' || character == 0x7f {
			return true
		}
	}
	return false
}

func decodeStrictPolicyJSON(data []byte, name string, target any) error {
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return fmt.Errorf("validate %s: %w", name, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: trailing JSON value", name)
		}
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

func validateDomainAllow(document policyDomainAllow, domain string) error {
	if document.SchemaVersion != policySchemaVersion || document.Host != domain {
		return fmt.Errorf("allow.json must use schema_version %d and match host %q", policySchemaVersion, domain)
	}
	if document.GraphQLEndpoints == nil || document.Rules == nil {
		return fmt.Errorf("allow.json collections must be explicit arrays")
	}
	seenEndpoints := map[tobari.GraphQLEndpoint]struct{}{}
	for _, endpoint := range document.GraphQLEndpoints {
		if err := endpoint.Validate(); err != nil || endpoint.Host != domain {
			return fmt.Errorf("allow.json GraphQL endpoint must be valid and match its domain")
		}
		if _, duplicate := seenEndpoints[endpoint]; duplicate {
			return fmt.Errorf("allow.json contains a duplicate GraphQL endpoint")
		}
		seenEndpoints[endpoint] = struct{}{}
	}
	for _, rule := range document.Rules {
		if rule.Host != domain {
			return fmt.Errorf("allow.json learned rule host must match its domain")
		}
	}
	if err := tobari.ValidateLearnedPolicyRules(document.Rules); err != nil {
		return fmt.Errorf("allow.json learned rules: %w", err)
	}
	return nil
}

func validateDomainDeny(document policyDomainDeny, domain string) error {
	if document.SchemaVersion != policySchemaVersion || document.Host != domain {
		return fmt.Errorf("deny.json must use schema_version %d and match host %q", policySchemaVersion, domain)
	}
	if document.Rules == nil {
		return fmt.Errorf("deny.json collections must be explicit arrays")
	}
	for _, rule := range document.Rules {
		if rule.Host != domain {
			return fmt.Errorf("deny.json exact rule host must match its domain")
		}
	}
	set := tobari.PolicyDenyRuleSet{Exact: document.Rules}
	if err := set.Validate(); err != nil {
		return fmt.Errorf("deny.json rules: %w", err)
	}
	return nil
}

func marshalPolicySource(document any) ([]byte, error) {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func composePolicyData(allows map[string]policyDomainAllow, denies map[string]policyDomainDeny) ([]byte, error) {
	domains := make([]string, 0, len(allows))
	for domain := range allows {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	endpoints := make([]tobari.GraphQLEndpoint, 0)
	learnedAllows := make([]tobari.LearnedPolicyRule, 0)
	learnedDenies := make([]tobari.PolicyDenyRule, 0)
	for _, domain := range domains {
		allow := allows[domain]
		deny, exists := denies[domain]
		if !exists {
			return nil, fmt.Errorf("policy domain %q is missing deny.json", domain)
		}
		endpoints = append(endpoints, allow.GraphQLEndpoints...)
		learnedAllows = append(learnedAllows, allow.Rules...)
		learnedDenies = append(learnedDenies, deny.Rules...)
	}
	if len(denies) != len(allows) {
		return nil, fmt.Errorf("policy contains a deny domain without allow.json")
	}
	sort.Slice(endpoints, func(i, j int) bool {
		left, right := endpoints[i], endpoints[j]
		return fmt.Sprintf("%s\x00%s\x00%05d\x00%s", left.Host, left.Scheme, left.Port, left.Path) <
			fmt.Sprintf("%s\x00%s\x00%05d\x00%s", right.Host, right.Scheme, right.Port, right.Path)
	})
	sort.Slice(learnedAllows, func(i, j int) bool { return learnedAllows[i].ID < learnedAllows[j].ID })
	sort.Slice(learnedDenies, func(i, j int) bool { return learnedDenies[i].ID < learnedDenies[j].ID })
	document := map[string]any{"tobari": map[string]any{
		"schema_version": policySchemaVersion,
		"boundary":       map[string]any{"graphql_endpoints": endpoints},
		"rules": map[string]any{
			learnedPolicyDataName: learnedAllows, learnedDenyDataName: learnedDenies,
		},
	}}
	return marshalPolicySource(document)
}

func emptyDomainAllow(domain string) policyDomainAllow {
	return policyDomainAllow{SchemaVersion: policySchemaVersion, Host: domain,
		GraphQLEndpoints: []tobari.GraphQLEndpoint{}, Rules: []tobari.LearnedPolicyRule{}}
}

func emptyDomainDeny(domain string) policyDomainDeny {
	return policyDomainDeny{SchemaVersion: policySchemaVersion, Host: domain, Rules: []tobari.PolicyDenyRule{}}
}

func readPolicyData(policyDirectory string) (policyDataFile, error) {
	if _, err := os.Lstat(policySourceJournalPath(policyDirectory)); err == nil {
		return policyDataFile{}, fmt.Errorf("policy source transaction is incomplete")
	} else if !errors.Is(err, os.ErrNotExist) {
		return policyDataFile{}, fmt.Errorf("inspect policy source transaction: %w", err)
	}
	domainsPath := filepath.Join(policyDirectory, policyDomainsName)
	before, err := os.Lstat(domainsPath)
	if err != nil {
		return policyDataFile{}, fmt.Errorf("inspect policy domains snapshot: %w", err)
	}
	file, err := readPolicyDataDuringTransaction(policyDirectory)
	if err != nil {
		return policyDataFile{}, err
	}
	confirmed, err := readPolicyDataDuringTransaction(policyDirectory)
	if err != nil {
		return policyDataFile{}, err
	}
	after, err := os.Lstat(domainsPath)
	if err != nil || !os.SameFile(before, after) || !reflect.DeepEqual(file.sources, confirmed.sources) {
		return policyDataFile{}, fmt.Errorf("policy domains changed during snapshot read")
	}
	if _, err := os.Lstat(policySourceJournalPath(policyDirectory)); err == nil {
		return policyDataFile{}, fmt.Errorf("policy source transaction started during snapshot read")
	} else if !errors.Is(err, os.ErrNotExist) {
		return policyDataFile{}, fmt.Errorf("inspect policy source transaction: %w", err)
	}
	return file, nil
}

func readPolicyDataDuringTransaction(policyDirectory string) (policyDataFile, error) {
	if err := validateOwnerPolicyDirectory(policyDirectory); err != nil {
		return policyDataFile{}, err
	}
	rootEntries, err := os.ReadDir(policyDirectory)
	if err != nil {
		return policyDataFile{}, err
	}
	for _, entry := range rootEntries {
		if entry.Name() != policyDomainsName && entry.Name() != "context.json" && entry.Name() != "tobari.rego" && entry.Name() != "tobari_test.rego" {
			return policyDataFile{}, fmt.Errorf("policy directory contains unsupported entry %q", entry.Name())
		}
	}
	domainsDirectory := filepath.Join(policyDirectory, policyDomainsName)
	return readPolicyDomains(domainsDirectory)
}

func validateContextPolicyLayout(policyDirectory string, mode tobari.ManifestPolicyMode) error {
	entries, err := os.ReadDir(policyDirectory)
	if err != nil {
		return err
	}
	expected := map[string]bool{policyDomainsName: true, "context.json": true}
	switch mode {
	case tobari.ManifestPolicyModeGuided:
	case tobari.ManifestPolicyModeAdvanced:
		expected["tobari.rego"] = true
		expected["tobari_test.rego"] = true
	default:
		return fmt.Errorf("Context policy mode is invalid")
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("%s Context policy must contain exactly %d managed entries", mode, len(expected))
	}
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return fmt.Errorf("%s Context policy contains unsupported entry %q", mode, entry.Name())
		}
	}
	return nil
}

func readPolicyDomains(domainsDirectory string) (policyDataFile, error) {
	if err := validateOwnerPolicyDirectory(domainsDirectory); err != nil {
		return policyDataFile{}, fmt.Errorf("validate policy domains: %w", err)
	}
	entries, err := os.ReadDir(domainsDirectory)
	if err != nil {
		return policyDataFile{}, err
	}
	if len(entries)*2 > maxPolicyFiles {
		return policyDataFile{}, fmt.Errorf("policy domains must contain at most %d domains", maxPolicyFiles/2)
	}
	file := policyDataFile{
		allows: map[string]policyDomainAllow{}, denies: map[string]policyDomainDeny{}, sources: map[string][]byte{},
		rules: []tobari.LearnedPolicyRule{}, denyRules: []tobari.PolicyDenyRule{}, graphqlEndpoints: []tobari.GraphQLEndpoint{},
	}
	total := 0
	for _, entry := range entries {
		domain := entry.Name()
		if err := validatePolicyDomain(domain); err != nil {
			return policyDataFile{}, fmt.Errorf("policy domain %q: %w", domain, err)
		}
		info, err := entry.Info()
		if err != nil || !entry.IsDir() || info.Mode().Perm()&0o077 != 0 {
			return policyDataFile{}, fmt.Errorf("policy domain %q must be an owner-only directory", domain)
		}
		directory := filepath.Join(domainsDirectory, domain)
		children, err := os.ReadDir(directory)
		if err != nil || len(children) != 2 || children[0].Name() != policyAllowFileName || children[1].Name() != policyDenyFileName {
			return policyDataFile{}, fmt.Errorf("policy domain %q must contain only allow.json and deny.json", domain)
		}
		allowPath := filepath.Join(directory, policyAllowFileName)
		allowData, err := readOwnerPolicyFile(allowPath, maxPolicyDataBytes)
		if err != nil {
			return policyDataFile{}, err
		}
		denyPath := filepath.Join(directory, policyDenyFileName)
		denyData, err := readOwnerPolicyFile(denyPath, maxPolicyDataBytes)
		if err != nil {
			return policyDataFile{}, err
		}
		total += len(allowData) + len(denyData)
		if total > maxPolicyPreflight {
			return policyDataFile{}, fmt.Errorf("policy domain sources exceed %d bytes", maxPolicyPreflight)
		}
		var allow policyDomainAllow
		if err := decodeStrictPolicyJSON(allowData, domain+"/allow.json", &allow); err != nil {
			return policyDataFile{}, err
		}
		if err := validateDomainAllow(allow, domain); err != nil {
			return policyDataFile{}, fmt.Errorf("validate %s/allow.json: %w", domain, err)
		}
		var deny policyDomainDeny
		if err := decodeStrictPolicyJSON(denyData, domain+"/deny.json", &deny); err != nil {
			return policyDataFile{}, err
		}
		if err := validateDomainDeny(deny, domain); err != nil {
			return policyDataFile{}, fmt.Errorf("validate %s/deny.json: %w", domain, err)
		}
		file.allows[domain], file.denies[domain] = allow, deny
		file.sources[filepath.Join(policyDomainsName, domain, policyAllowFileName)] = append([]byte{}, allowData...)
		file.sources[filepath.Join(policyDomainsName, domain, policyDenyFileName)] = append([]byte{}, denyData...)
		file.rules = append(file.rules, allow.Rules...)
		file.graphqlEndpoints = append(file.graphqlEndpoints, allow.GraphQLEndpoints...)
		file.denyRules = append(file.denyRules, deny.Rules...)
	}
	sort.Slice(file.rules, func(i, j int) bool { return file.rules[i].ID < file.rules[j].ID })
	if err := tobari.ValidateLearnedPolicyRules(file.rules); err != nil {
		return policyDataFile{}, fmt.Errorf("validate learned allows: %w", err)
	}
	set := tobari.PolicyDenyRuleSet{Exact: file.denyRules}
	if err := set.Validate(); err != nil {
		return policyDataFile{}, fmt.Errorf("validate deny rules: %w", err)
	}
	sort.Slice(file.denyRules, func(i, j int) bool { return file.denyRules[i].ID < file.denyRules[j].ID })
	file.source, err = composePolicyData(file.allows, file.denies)
	if err != nil {
		return policyDataFile{}, err
	}
	return file, nil
}

func (f policyDataFile) withPolicyRules(rules []tobari.LearnedPolicyRule, denyRules []tobari.PolicyDenyRule) (policyDataFile, error) {
	if err := tobari.ValidateLearnedPolicyRules(rules); err != nil {
		return policyDataFile{}, err
	}
	denySet := tobari.PolicyDenyRuleSet{Exact: denyRules}
	if err := denySet.Validate(); err != nil {
		return policyDataFile{}, err
	}
	result := policyDataFile{allows: map[string]policyDomainAllow{}, denies: map[string]policyDomainDeny{}, sources: map[string][]byte{}}
	for domain, allow := range f.allows {
		allow.Rules = []tobari.LearnedPolicyRule{}
		result.allows[domain] = allow
	}
	for domain, deny := range f.denies {
		deny.Rules = []tobari.PolicyDenyRule{}
		result.denies[domain] = deny
	}
	for _, rule := range rules {
		if err := validatePolicyDomain(rule.Host); err != nil {
			return policyDataFile{}, err
		}
		allow, exists := result.allows[rule.Host]
		if !exists {
			allow = emptyDomainAllow(rule.Host)
			result.denies[rule.Host] = emptyDomainDeny(rule.Host)
		}
		allow.Rules = append(allow.Rules, rule)
		result.allows[rule.Host] = allow
	}
	for _, rule := range denyRules {
		if err := validatePolicyDomain(rule.Host); err != nil {
			return policyDataFile{}, err
		}
		deny, exists := result.denies[rule.Host]
		if !exists {
			deny = emptyDomainDeny(rule.Host)
			result.allows[rule.Host] = emptyDomainAllow(rule.Host)
		}
		deny.Rules = append(deny.Rules, rule)
		result.denies[rule.Host] = deny
	}
	domains := make([]string, 0, len(result.allows))
	for domain := range result.allows {
		domains = append(domains, domain)
	}
	sort.Strings(domains)
	for _, domain := range domains {
		allow := result.allows[domain]
		sort.Slice(allow.Rules, func(i, j int) bool { return allow.Rules[i].ID < allow.Rules[j].ID })
		result.allows[domain] = allow
		deny := result.denies[domain]
		sort.Slice(deny.Rules, func(i, j int) bool { return deny.Rules[i].ID < deny.Rules[j].ID })
		result.denies[domain] = deny
		allowRelative := filepath.Join(policyDomainsName, domain, policyAllowFileName)
		allowData := append([]byte{}, f.sources[allowRelative]...)
		if original, exists := f.allows[domain]; !exists || !reflect.DeepEqual(original, allow) {
			var err error
			allowData, err = marshalPolicySource(allow)
			if err != nil {
				return policyDataFile{}, err
			}
		}
		denyRelative := filepath.Join(policyDomainsName, domain, policyDenyFileName)
		denyData := append([]byte{}, f.sources[denyRelative]...)
		if original, exists := f.denies[domain]; !exists || !reflect.DeepEqual(original, deny) {
			var err error
			denyData, err = marshalPolicySource(deny)
			if err != nil {
				return policyDataFile{}, err
			}
		}
		result.sources[allowRelative] = allowData
		result.sources[denyRelative] = denyData
	}
	result.rules = append([]tobari.LearnedPolicyRule{}, rules...)
	result.denyRules = append([]tobari.PolicyDenyRule{}, denyRules...)
	for _, domain := range domains {
		result.graphqlEndpoints = append(result.graphqlEndpoints, result.allows[domain].GraphQLEndpoints...)
	}
	var err error
	result.source, err = composePolicyData(result.allows, result.denies)
	if err != nil {
		return policyDataFile{}, err
	}
	return result, nil
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
			if rule.WorkspaceManifestID != item.manifest.ID || rule.WorkspaceManifestName != item.manifest.Name {
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
		result := tobari.PolicyDenyRuleSet{Exact: append([]tobari.PolicyDenyRule{}, file.denyRules...)}
		return result, result.Validate()
	}
	contexts, err := r.readAggregateContexts(ctx)
	if err != nil {
		return tobari.PolicyDenyRuleSet{}, err
	}
	result := tobari.PolicyDenyRuleSet{Exact: []tobari.PolicyDenyRule{}}
	for _, item := range contexts {
		file, err := readPolicyData(item.paths.PolicyDirectory)
		if err != nil {
			return tobari.PolicyDenyRuleSet{}, fmt.Errorf("Context %q policy: %w", item.manifest.Name, err)
		}
		for _, rule := range file.denyRules {
			if rule.WorkspaceManifestID != item.manifest.ID || rule.WorkspaceManifestName != item.manifest.Name {
				return tobari.PolicyDenyRuleSet{}, fmt.Errorf("Context %q exact deny has mismatched authority scope", item.manifest.Name)
			}
			result.Exact = append(result.Exact, rule)
		}
	}
	sort.Slice(result.Exact, func(i, j int) bool { return result.Exact[i].ID < result.Exact[j].ID })
	return result, result.Validate()
}

func copyPolicyForPreflight(sourceDirectory string, candidate policyDataFile) (string, error) {
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
	if len(candidate.source) > maxPolicyPreflight {
		return cleanup(fmt.Errorf("generated policy data exceeds %d bytes", maxPolicyPreflight))
	}
	if err := os.WriteFile(filepath.Join(temporary, "data.json"), candidate.source, 0o600); err != nil { // #nosec G306 -- policy copies are owner-only.
		return cleanup(err)
	}
	for _, name := range []string{"tobari.rego", "tobari_test.rego"} {
		data, err := readOwnerPolicyFile(filepath.Join(sourceDirectory, name), maxPolicyPreflight-len(candidate.source))
		if err != nil {
			return cleanup(fmt.Errorf("copy policy for preflight: %w", err))
		}
		if len(candidate.source)+len(data) > maxPolicyPreflight {
			return cleanup(fmt.Errorf("policy preflight exceeds %d bytes", maxPolicyPreflight))
		}
		if err := os.WriteFile(filepath.Join(temporary, name), data, 0o600); err != nil { // #nosec G306 -- policy copies are owner-only.
			return cleanup(err)
		}
	}
	return temporary, nil
}

func prepareContextPolicyPreflight(
	manifest tobari.WorkspaceManifest, sourceDirectory string, candidate policyDataFile,
) (string, error) {
	if err := validateContextPolicyLayout(sourceDirectory, manifest.PolicyMode); err != nil {
		return "", err
	}
	if manifest.PolicyMode == tobari.ManifestPolicyModeAdvanced {
		return copyPolicyForPreflight(sourceDirectory, candidate)
	}
	if manifest.PolicyMode != tobari.ManifestPolicyModeGuided {
		return "", fmt.Errorf("Context policy mode is invalid")
	}
	if err := validateOwnerPolicyDirectory(sourceDirectory); err != nil {
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
	if len(candidate.source)+len(rego)+len(tests) > maxPolicyPreflight {
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
		"data.json": candidate.source, "tobari.rego": rego, "tobari_test.rego": tests,
	} {
		if err := os.WriteFile(filepath.Join(temporary, name), data, 0o600); err != nil { // #nosec G306 -- preflight is owner-only.
			return cleanup(err)
		}
	}
	return temporary, nil
}

type policyCandidateValidationReceipt struct {
	policyDirectory string
	candidateDigest string
	preflightDigest string
}

func policyPreflightDigest(directory string) (string, error) {
	if err := validateOwnerPolicyDirectory(directory); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", fmt.Errorf("read policy preflight directory: %w", err)
	}
	names := []string{"data.json", "tobari.rego", "tobari_test.rego"}
	if len(entries) != len(names) {
		return "", errors.New("policy preflight must contain exactly the reviewed policy files")
	}
	files := make(map[string][]byte, len(names))
	remaining := maxPolicyPreflight
	for index, name := range names {
		if entries[index].Name() != name {
			return "", fmt.Errorf("policy preflight contains unsupported file %q", entries[index].Name())
		}
		data, err := readOwnerPolicyFile(filepath.Join(directory, name), remaining)
		if err != nil {
			return "", err
		}
		remaining -= len(data)
		files[name] = data
	}
	return policySourceDigest(files), nil
}

func (r *Runtime) testContextPolicyCandidate(
	ctx context.Context, manifest tobari.WorkspaceManifest, policyDirectory string, candidate policyDataFile,
) (policyCandidateValidationReceipt, error) {
	preflight, err := prepareContextPolicyPreflight(manifest, policyDirectory, candidate)
	if err != nil {
		return policyCandidateValidationReceipt{}, err
	}
	defer os.RemoveAll(preflight)
	before, err := policyPreflightDigest(preflight)
	if err != nil {
		return policyCandidateValidationReceipt{}, err
	}
	if err := r.testPolicyDirectory(ctx, preflight); err != nil {
		return policyCandidateValidationReceipt{}, err
	}
	after, err := policyPreflightDigest(preflight)
	if err != nil {
		return policyCandidateValidationReceipt{}, err
	}
	if before != after {
		return policyCandidateValidationReceipt{}, fmt.Errorf("policy preflight changed while OPA tested it")
	}
	return policyCandidateValidationReceipt{
		policyDirectory: policyDirectory,
		candidateDigest: policySourceDigest(candidate.sources),
		preflightDigest: before,
	}, nil
}

const (
	policySourceJournalSchema = 1
	policySourcePhasePrepared = "prepared"
	policySourcePhaseOldMoved = "old_moved"
	policySourcePhaseSwapped  = "swapped"
)

type policySourceJournal struct {
	SchemaVersion              int    `json:"schema_version"`
	Phase                      string `json:"phase"`
	StageName                  string `json:"stage_name"`
	BackupName                 string `json:"backup_name"`
	OriginalDigest             string `json:"original_digest"`
	CandidateDigest            string `json:"candidate_digest"`
	CandidateAggregateRevision string `json:"candidate_aggregate_revision"`
}

func (j policySourceJournal) Validate() error {
	if j.SchemaVersion != policySourceJournalSchema {
		return fmt.Errorf("policy source journal schema version must be %d", policySourceJournalSchema)
	}
	if j.Phase != policySourcePhasePrepared && j.Phase != policySourcePhaseOldMoved && j.Phase != policySourcePhaseSwapped {
		return fmt.Errorf("policy source journal phase is invalid")
	}
	if !safePolicyTransactionName(j.StageName, ".policy-domains-stage-") ||
		!safePolicyTransactionName(j.BackupName, ".policy-domains-backup-") {
		return fmt.Errorf("policy source journal path is invalid")
	}
	if strings.TrimPrefix(j.StageName, ".policy-domains-stage-") !=
		strings.TrimPrefix(j.BackupName, ".policy-domains-backup-") {
		return fmt.Errorf("policy source journal generation names do not match")
	}
	for _, digest := range []string{j.OriginalDigest, j.CandidateDigest} {
		if !validPolicyDigest(digest) {
			return fmt.Errorf("policy source journal digest is invalid")
		}
	}
	if j.CandidateAggregateRevision != "" && !validPolicyDigest(j.CandidateAggregateRevision) {
		return fmt.Errorf("policy source journal aggregate revision is invalid")
	}
	return nil
}

func safePolicyTransactionName(name, prefix string) bool {
	return name == filepath.Base(name) && strings.HasPrefix(name, prefix) && len(name) > len(prefix)
}

func validPolicyDigest(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func policySourceJournalPath(policyDirectory string) string {
	return filepath.Join(filepath.Dir(policyDirectory), ".policy-domains-transaction.json")
}

func policySourceDigest(sources map[string][]byte) string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, filepath.ToSlash(name))
	}
	sort.Strings(names)
	digest := sha256.New()
	for _, name := range names {
		digest.Write([]byte(name))
		digest.Write([]byte{0})
		digest.Write(sources[filepath.FromSlash(name)])
		digest.Write([]byte{0})
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func writePolicySourceJournal(policyDirectory string, journal policySourceJournal) error {
	if err := journal.Validate(); err != nil {
		return err
	}
	return writeAtomicJSON(policySourceJournalPath(policyDirectory), journal)
}

func readPolicySourceJournal(policyDirectory string) (policySourceJournal, bool, error) {
	path := policySourceJournalPath(policyDirectory)
	data, err := readOwnerPolicyFile(path, 64*1024)
	if errors.Is(err, os.ErrNotExist) || (err != nil && errors.Is(errors.Unwrap(err), os.ErrNotExist)) {
		return policySourceJournal{}, false, nil
	}
	if err != nil {
		return policySourceJournal{}, false, err
	}
	var journal policySourceJournal
	if err := decodeStrictPolicyJSON(data, filepath.Base(path), &journal); err != nil {
		return policySourceJournal{}, false, err
	}
	if err := journal.Validate(); err != nil {
		return policySourceJournal{}, false, err
	}
	return journal, true, nil
}

func writePolicySnapshotFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- path is a validated child of a new private generation.
	if err != nil {
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
	return file.Close()
}

func writePolicyDomainsSnapshot(directory string, sources map[string][]byte) error {
	if err := requirePrivateDirectory(directory); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if entries, err := os.ReadDir(directory); err != nil {
		return err
	} else if len(entries) != 0 {
		return fmt.Errorf("candidate policy snapshot directory is not empty")
	}
	names := make([]string, 0, len(sources))
	for relative := range sources {
		names = append(names, relative)
	}
	sort.Strings(names)
	seenDomains := map[string]struct{}{}
	for _, relative := range names {
		parts := strings.Split(filepath.ToSlash(relative), "/")
		if len(parts) != 3 || parts[0] != policyDomainsName ||
			(parts[2] != policyAllowFileName && parts[2] != policyDenyFileName) {
			return fmt.Errorf("candidate policy source path %q is invalid", relative)
		}
		if err := validatePolicyDomain(parts[1]); err != nil {
			return err
		}
		domainDirectory := filepath.Join(directory, parts[1])
		if _, exists := seenDomains[parts[1]]; !exists {
			if err := os.Mkdir(domainDirectory, 0o700); err != nil {
				return err
			}
			seenDomains[parts[1]] = struct{}{}
		}
		if err := writePolicySnapshotFile(filepath.Join(domainDirectory, parts[2]), sources[relative]); err != nil {
			return err
		}
	}
	file, err := readPolicyDomains(directory)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(file.sources, sources) {
		return fmt.Errorf("candidate policy source snapshot changed while being prepared")
	}
	for domain := range seenDomains {
		if err := syncDirectoryIfPresent(filepath.Join(directory, domain)); err != nil {
			return err
		}
	}
	return syncDirectoryIfPresent(directory)
}

type policySourceTransaction struct {
	policyDirectory             string
	journal                     policySourceJournal
	candidateValidation         *policyCandidateValidationReceipt
	candidateValidationConsumed bool
}

func (t *policySourceTransaction) bindCandidateValidation(receipt policyCandidateValidationReceipt) error {
	if receipt.policyDirectory != t.policyDirectory ||
		receipt.candidateDigest != t.journal.CandidateDigest ||
		!validPolicyDigest(receipt.preflightDigest) {
		return fmt.Errorf("candidate policy validation receipt does not match the source transaction")
	}
	t.candidateValidation = &receipt
	t.candidateValidationConsumed = false
	return nil
}

func (t *policySourceTransaction) consumeCandidateValidation(
	policyDirectory, candidateDigest, preflightDigest string,
) bool {
	receipt := t.candidateValidation
	if receipt == nil || t.candidateValidationConsumed ||
		receipt.policyDirectory != policyDirectory ||
		receipt.candidateDigest != candidateDigest ||
		receipt.preflightDigest != preflightDigest ||
		t.journal.CandidateDigest != candidateDigest {
		return false
	}
	t.candidateValidationConsumed = true
	return true
}

func (t *policySourceTransaction) verifyJournal() error {
	current, exists, err := readPolicySourceJournal(t.policyDirectory)
	if err != nil {
		return err
	}
	if !exists || !reflect.DeepEqual(current, t.journal) {
		return fmt.Errorf("policy source transaction journal changed")
	}
	return nil
}

func beginPolicySourceTransaction(
	policyDirectory string, expected, candidate map[string][]byte,
) (*policySourceTransaction, error) {
	if _, exists, err := readPolicySourceJournal(policyDirectory); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("policy source transaction is already pending")
	}
	equal, err := policySourcesEqual(policyDirectory, expected)
	if err != nil || !equal {
		return nil, fault.Wrap(fault.KindRejected, "policy_data_changed", "policy domain sources changed while the candidate was being tested", false, err)
	}
	contextDirectory := filepath.Dir(policyDirectory)
	stagePath, err := os.MkdirTemp(contextDirectory, ".policy-domains-stage-")
	if err != nil {
		return nil, err
	}
	stageName := filepath.Base(stagePath)
	cleanupStage := true
	defer func() {
		if cleanupStage {
			_ = os.RemoveAll(stagePath)
		}
	}()
	if err := writePolicyDomainsSnapshot(stagePath, candidate); err != nil {
		return nil, err
	}
	equal, err = policySourcesEqual(policyDirectory, expected)
	if err != nil || !equal {
		return nil, fault.Wrap(fault.KindRejected, "policy_data_changed", "policy domain sources changed while the replacement generation was being prepared", false, err)
	}
	suffix := strings.TrimPrefix(stageName, ".policy-domains-stage-")
	backupName := ".policy-domains-backup-" + suffix
	backupPath := filepath.Join(contextDirectory, backupName)
	journal := policySourceJournal{
		SchemaVersion: policySourceJournalSchema,
		Phase:         policySourcePhasePrepared, StageName: stageName, BackupName: backupName,
		OriginalDigest: policySourceDigest(expected), CandidateDigest: policySourceDigest(candidate),
		CandidateAggregateRevision: "",
	}
	if err := writePolicySourceJournal(policyDirectory, journal); err != nil {
		return nil, err
	}
	livePath := filepath.Join(policyDirectory, policyDomainsName)
	if err := os.Rename(livePath, backupPath); err != nil {
		_ = os.Remove(policySourceJournalPath(policyDirectory))
		return nil, err
	}
	backup, backupErr := readPolicyDomains(backupPath)
	if backupErr != nil || policySourceDigest(backup.sources) != journal.OriginalDigest {
		restoreErr := os.Rename(backupPath, livePath)
		if restoreErr == nil {
			_ = os.Remove(policySourceJournalPath(policyDirectory))
			return nil, fault.Wrap(
				fault.KindRejected, "policy_data_changed",
				"policy domain sources changed immediately before generation replacement", false,
				errors.Join(backupErr, fmt.Errorf("recovery generation digest does not match the tested source")),
			)
		}
		return nil, fault.Wrap(
			fault.KindInternal, "policy_write_failed",
			"changed policy source could not be restored and recovery remains pending", false,
			errors.Join(backupErr, restoreErr),
		)
	}
	journal.Phase = policySourcePhaseOldMoved
	if err := writePolicySourceJournal(policyDirectory, journal); err != nil {
		if restoreErr := os.Rename(backupPath, livePath); restoreErr == nil {
			_ = os.Remove(policySourceJournalPath(policyDirectory))
		} else {
			return nil, fault.Wrap(fault.KindInternal, "policy_write_failed", "policy source recovery remains pending", false, errors.Join(err, restoreErr))
		}
		return nil, err
	}
	if err := os.Rename(stagePath, livePath); err != nil {
		if restoreErr := os.Rename(backupPath, livePath); restoreErr == nil {
			_ = os.Remove(policySourceJournalPath(policyDirectory))
		} else {
			return nil, fault.Wrap(fault.KindInternal, "policy_write_failed", "policy source recovery remains pending", false, errors.Join(err, restoreErr))
		}
		return nil, err
	}
	cleanupStage = false
	journal.Phase = policySourcePhaseSwapped
	if err := writePolicySourceJournal(policyDirectory, journal); err != nil {
		return nil, err
	}
	if err := syncDirectoryIfPresent(policyDirectory); err != nil {
		return nil, err
	}
	if err := syncDirectoryIfPresent(contextDirectory); err != nil {
		return nil, err
	}
	return &policySourceTransaction{policyDirectory: policyDirectory, journal: journal}, nil
}

func (t *policySourceTransaction) setCandidateAggregateRevision(revision string) error {
	if !validPolicyDigest(revision) {
		return fmt.Errorf("candidate aggregate revision is invalid")
	}
	if err := t.verifyJournal(); err != nil {
		return err
	}
	t.journal.CandidateAggregateRevision = revision
	return writePolicySourceJournal(t.policyDirectory, t.journal)
}

func (t *policySourceTransaction) commit() error {
	if err := t.verifyJournal(); err != nil {
		return err
	}
	current, err := readPolicyDataDuringTransaction(t.policyDirectory)
	if err != nil || policySourceDigest(current.sources) != t.journal.CandidateDigest {
		return fmt.Errorf("candidate policy source changed before commit: %w", err)
	}
	contextDirectory := filepath.Dir(t.policyDirectory)
	backupPath := filepath.Join(contextDirectory, t.journal.BackupName)
	backup, err := readPolicyDomains(backupPath)
	if err != nil || policySourceDigest(backup.sources) != t.journal.OriginalDigest {
		return fmt.Errorf("original policy source recovery snapshot is invalid: %w", err)
	}
	if err := os.RemoveAll(backupPath); err != nil {
		return err
	}
	if err := os.Remove(policySourceJournalPath(t.policyDirectory)); err != nil {
		return err
	}
	return syncDirectoryIfPresent(contextDirectory)
}

func (t *policySourceTransaction) rollback() error {
	if err := t.verifyJournal(); err != nil {
		return err
	}
	current, err := readPolicyDataDuringTransaction(t.policyDirectory)
	if err != nil || policySourceDigest(current.sources) != t.journal.CandidateDigest {
		return fmt.Errorf("candidate policy source changed during rollback: %w", err)
	}
	contextDirectory := filepath.Dir(t.policyDirectory)
	backupPath := filepath.Join(contextDirectory, t.journal.BackupName)
	backup, err := readPolicyDomains(backupPath)
	if err != nil || policySourceDigest(backup.sources) != t.journal.OriginalDigest {
		return fmt.Errorf("original policy source recovery snapshot is invalid: %w", err)
	}
	abandonedPath := filepath.Join(contextDirectory, t.journal.StageName)
	if err := os.Rename(filepath.Join(t.policyDirectory, policyDomainsName), abandonedPath); err != nil {
		return err
	}
	if err := os.Rename(backupPath, filepath.Join(t.policyDirectory, policyDomainsName)); err != nil {
		_ = os.Rename(abandonedPath, filepath.Join(t.policyDirectory, policyDomainsName))
		return err
	}
	if err := os.RemoveAll(abandonedPath); err != nil {
		return err
	}
	if err := os.Remove(policySourceJournalPath(t.policyDirectory)); err != nil {
		return err
	}
	if err := syncDirectoryIfPresent(t.policyDirectory); err != nil {
		return err
	}
	return syncDirectoryIfPresent(contextDirectory)
}

func policyDomainsDigest(path string) (string, bool, error) {
	file, err := readPolicyDomains(path)
	if errors.Is(err, os.ErrNotExist) || (err != nil && errors.Is(errors.Unwrap(err), os.ErrNotExist)) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return policySourceDigest(file.sources), true, nil
}

func recoverPolicySourceTransaction(policyDirectory, activeRevision string) error {
	journal, exists, err := readPolicySourceJournal(policyDirectory)
	if err != nil || !exists {
		return err
	}
	contextDirectory := filepath.Dir(policyDirectory)
	livePath := filepath.Join(policyDirectory, policyDomainsName)
	stagePath := filepath.Join(contextDirectory, journal.StageName)
	backupPath := filepath.Join(contextDirectory, journal.BackupName)
	liveDigest, liveExists, liveErr := policyDomainsDigest(livePath)
	stageDigest, stageExists, stageErr := policyDomainsDigest(stagePath)
	backupDigest, backupExists, backupErr := policyDomainsDigest(backupPath)
	if liveErr != nil || stageErr != nil || backupErr != nil {
		return fmt.Errorf("inspect interrupted policy source transaction: live=%v stage=%v backup=%v", liveErr, stageErr, backupErr)
	}
	removeJournal := func() error {
		if err := os.Remove(policySourceJournalPath(policyDirectory)); err != nil {
			return err
		}
		return syncDirectoryIfPresent(contextDirectory)
	}
	if journal.CandidateAggregateRevision != "" && activeRevision == journal.CandidateAggregateRevision &&
		liveExists && liveDigest == journal.CandidateDigest {
		if stageExists {
			return fmt.Errorf("committed policy source transaction retains an unexpected stage")
		}
		if backupExists {
			if backupDigest != journal.OriginalDigest {
				return fmt.Errorf("committed policy source transaction has an invalid recovery snapshot")
			}
			if err := os.RemoveAll(backupPath); err != nil {
				return err
			}
		}
		return removeJournal()
	}
	if liveExists && liveDigest == journal.OriginalDigest && !backupExists {
		if stageExists && stageDigest != journal.CandidateDigest {
			return fmt.Errorf("interrupted policy source stage does not match its journal")
		}
		if stageExists {
			if err := os.RemoveAll(stagePath); err != nil {
				return err
			}
		}
		return removeJournal()
	}
	if !backupExists || backupDigest != journal.OriginalDigest {
		return fmt.Errorf("interrupted policy source transaction has no valid rollback generation")
	}
	switch {
	case !liveExists && stageExists && stageDigest == journal.CandidateDigest:
		if err := os.Rename(backupPath, livePath); err != nil {
			return err
		}
		if err := os.RemoveAll(stagePath); err != nil {
			return err
		}
	case liveExists && liveDigest == journal.CandidateDigest && !stageExists:
		if err := os.Rename(livePath, stagePath); err != nil {
			return err
		}
		if err := os.Rename(backupPath, livePath); err != nil {
			_ = os.Rename(stagePath, livePath)
			return err
		}
		if err := os.RemoveAll(stagePath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("interrupted policy source transaction layout is ambiguous")
	}
	if err := syncDirectoryIfPresent(policyDirectory); err != nil {
		return err
	}
	return removeJournal()
}

func (r *Runtime) recoverAllPolicySourceTransactions(ctx context.Context) error {
	state, configured, err := r.LoadState(ctx)
	if err != nil {
		return err
	}
	activeRevision := ""
	if configured {
		activeRevision = state.AggregateRevision
	}
	contexts, err := r.ListContexts(ctx)
	if err != nil {
		return err
	}
	for _, summary := range contexts.Items {
		_, paths, err := r.resolveContext(summary.Name)
		if err != nil {
			return err
		}
		if err := recoverPolicySourceTransaction(paths.PolicyDirectory, activeRevision); err != nil {
			return fmt.Errorf("recover Context %q policy source: %w", summary.Name, err)
		}
	}
	return nil
}

func policySourcesEqual(policyDirectory string, expected map[string][]byte) (bool, error) {
	current, err := readPolicyData(policyDirectory)
	if err != nil {
		return false, err
	}
	return reflect.DeepEqual(current.sources, expected), nil
}

// ApplyLearnedPolicyRules tests a complete private policy copy, replaces one
// complete domains generation, then uses the portable OPA activation boundary.
func (r *Runtime) ApplyLearnedPolicyRules(
	ctx context.Context, state tobari.State,
	expected, updated []tobari.LearnedPolicyRule,
) (tobari.PolicyActivationReceipt, error) {
	return r.applyPolicyData(ctx, state, expected, updated, nil, nil, false)
}

// ApplyPolicyDenyRules tests and atomically activates one complete policy data
// update while requiring the exact-deny snapshot used by discovery to remain
// unchanged.
func (r *Runtime) ApplyPolicyDenyRules(
	ctx context.Context, state tobari.State,
	expectedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	return r.applyPolicyData(ctx, state, expectedAllows, nil, expectedDenies, updatedDenies, true)
}

func (r *Runtime) applyPolicyData(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
	checkDenySnapshot bool,
) (tobari.PolicyActivationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	if err := state.Validate(); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	if strings.HasPrefix(state.PolicyDirectory, r.aggregateRoot()+string(filepath.Separator)) {
		return r.applyAggregatePolicyData(
			ctx, state, expectedAllows, updatedAllows, expectedDenies, updatedDenies, checkDenySnapshot,
		)
	}
	var receipt tobari.PolicyActivationReceipt
	err := r.withPolicyProjectionLock(ctx, func() error {
		if err := recoverPolicySourceTransaction(state.PolicyDirectory, state.AggregateRevision); err != nil {
			return fault.Wrap(fault.KindRejected, "policy_state_changed", "an interrupted policy source transaction could not be recovered", false, err)
		}
		var applyErr error
		receipt, applyErr = r.applyStandalonePolicyData(
			ctx, state, expectedAllows, updatedAllows, expectedDenies, updatedDenies, checkDenySnapshot,
		)
		return applyErr
	})
	return receipt, err
}

func (r *Runtime) applyStandalonePolicyData(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
	checkDenySnapshot bool,
) (tobari.PolicyActivationReceipt, error) {
	receipt := tobari.PolicyActivationReceipt{
		PolicyDirectory: state.PolicyDirectory,
		ActiveRevision:  state.AggregateRevision,
	}
	if err := receipt.Validate(); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	if err := tobari.ValidateLearnedPolicyRules(expectedAllows); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	if updatedAllows == nil {
		updatedAllows = expectedAllows
	}
	if err := tobari.ValidateLearnedPolicyRules(updatedAllows); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	file, err := readPolicyData(state.PolicyDirectory)
	if err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindRejected, "policy_data_invalid",
			"host policy data is not safe for managed learning", false, err,
		)
	}
	expectedAllows = append([]tobari.LearnedPolicyRule{}, expectedAllows...)
	sort.Slice(expectedAllows, func(i, j int) bool { return expectedAllows[i].ID < expectedAllows[j].ID })
	if !reflect.DeepEqual(file.rules, expectedAllows) {
		return tobari.PolicyActivationReceipt{}, fault.New(
			fault.KindRejected, "policy_data_changed",
			"learned policy rules changed after discovery", false,
		)
	}
	if !checkDenySnapshot {
		expectedDenies = append([]tobari.PolicyDenyRule{}, file.denyRules...)
		updatedDenies = append([]tobari.PolicyDenyRule{}, file.denyRules...)
	}
	denyExpected := tobari.PolicyDenyRuleSet{Exact: expectedDenies}
	denyUpdated := tobari.PolicyDenyRuleSet{Exact: updatedDenies}
	if err := denyExpected.Validate(); err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(fault.KindRejected, "policy_data_invalid", "host policy deny data is invalid", false, err)
	}
	if !reflect.DeepEqual(file.denyRules, expectedDenies) {
		return tobari.PolicyActivationReceipt{}, fault.New(
			fault.KindRejected, "policy_data_changed",
			"policy deny rules changed after discovery", false,
		)
	}
	if err := denyUpdated.Validate(); err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(fault.KindContract, "invalid_policy_deny", "exact policy deny update is invalid", false, err)
	}
	data, err := file.withPolicyRules(updatedAllows, updatedDenies)
	if err != nil {
		code := "invalid_learned_policy"
		message := "learned policy update is invalid"
		if checkDenySnapshot {
			code = "invalid_policy_deny"
			message = "exact policy deny update is invalid"
		}
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindContract, code, message, false, err,
		)
	}
	preflight, err := copyPolicyForPreflight(state.PolicyDirectory, data)
	if err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindRejected, "policy_preflight_failed",
			"candidate policy could not be prepared for testing", false, err,
		)
	}
	defer func() { _ = os.RemoveAll(preflight) }()
	if err := r.testPolicyDirectory(ctx, preflight); err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindRejected, "policy_preflight_failed",
			"candidate policy failed OPA tests", false, err,
		)
	}
	equal, err := policySourcesEqual(state.PolicyDirectory, file.sources)
	if err != nil || !equal {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindRejected, "policy_data_changed",
			"policy domain sources changed while the candidate was being tested", false, err,
		)
	}
	transaction, err := beginPolicySourceTransaction(state.PolicyDirectory, file.sources, data.sources)
	if err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindInternal, "policy_write_failed",
			"tested policy domain sources could not be written", false, err,
		)
	}
	if err := r.activatePolicyRevision(ctx, state, policyAuthorityReduces(expectedAllows, updatedAllows, expectedDenies, updatedDenies)); err != nil {
		if rollbackErr := transaction.rollback(); rollbackErr != nil {
			return tobari.PolicyActivationReceipt{}, fault.Wrap(
				fault.KindInternal, "policy_write_failed",
				"failed policy activation left source recovery pending", false, errors.Join(err, rollbackErr),
			)
		}
		return tobari.PolicyActivationReceipt{}, err
	}
	if err := transaction.commit(); err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(
			fault.KindInternal, "policy_write_failed",
			"activated policy source transaction could not be finalized", false, err,
		)
	}
	return receipt, nil
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
			ids[rule.WorkspaceManifestID] = struct{}{}
		}
	}
	for id, rule := range allowExpected {
		if _, ok := allowUpdated[id]; !ok {
			ids[rule.WorkspaceManifestID] = struct{}{}
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
			ids[rule.WorkspaceManifestID] = struct{}{}
		}
	}
	for id, rule := range denyExpected {
		if _, ok := denyUpdated[id]; !ok {
			ids[rule.WorkspaceManifestID] = struct{}{}
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
		if rule.WorkspaceManifestID == contextID {
			result = append(result, rule)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func deniesForContext(rules []tobari.PolicyDenyRule, contextID string) []tobari.PolicyDenyRule {
	result := make([]tobari.PolicyDenyRule, 0)
	for _, rule := range rules {
		if rule.WorkspaceManifestID == contextID {
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
) (tobari.PolicyActivationReceipt, error) {
	if updatedAllows == nil {
		updatedAllows = expectedAllows
	}
	if err := tobari.ValidateLearnedPolicyRules(expectedAllows); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	if err := tobari.ValidateLearnedPolicyRules(updatedAllows); err != nil {
		return tobari.PolicyActivationReceipt{}, err
	}
	targetContexts, err := policyMutationContexts(expectedAllows, updatedAllows, expectedDenies, updatedDenies)
	if err != nil {
		return tobari.PolicyActivationReceipt{}, fault.Wrap(fault.KindContract, "invalid_policy_scope", "policy mutation Context scope is invalid", false, err)
	}
	if checkDenySnapshot && len(targetContexts) != 1 {
		return tobari.PolicyActivationReceipt{}, fault.New(
			fault.KindRejected, "policy_review_scope_mixed",
			"one reviewed Apply cannot span multiple Context policy sources", false,
		)
	}
	receipt := tobari.PolicyActivationReceipt{}
	err = r.withPolicyProjectionLock(ctx, func() error {
		if err := r.recoverAllPolicySourceTransactions(ctx); err != nil {
			return fault.Wrap(fault.KindRejected, "policy_state_changed", "an interrupted policy source transaction could not be recovered", false, err)
		}
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
		if !checkDenySnapshot {
			expectedDenies = append([]tobari.PolicyDenyRule{}, currentDenies.Exact...)
			updatedDenies = append([]tobari.PolicyDenyRule{}, currentDenies.Exact...)
		}
		sort.Slice(expectedAllows, func(i, j int) bool { return expectedAllows[i].ID < expectedAllows[j].ID })
		sort.Slice(expectedDenies, func(i, j int) bool { return expectedDenies[i].ID < expectedDenies[j].ID })
		if !reflect.DeepEqual(currentAllows, expectedAllows) || !reflect.DeepEqual(currentDenies.Exact, expectedDenies) {
			return fault.New(fault.KindRejected, "policy_data_changed", "policy decisions changed after discovery", false)
		}
		type contextUpdate struct {
			policyDirectory string
			original        map[string][]byte
			candidate       map[string][]byte
			validation      policyCandidateValidationReceipt
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
				if rule.WorkspaceManifestName != manifest.Name {
					return fault.New(fault.KindContract, "context_mismatch", "learned rule Context binding is inconsistent", false)
				}
			}
			for _, rule := range contextDenies {
				if rule.WorkspaceManifestName != manifest.Name {
					return fault.New(fault.KindContract, "context_mismatch", "deny rule Context binding is inconsistent", false)
				}
			}
			data, err := file.withPolicyRules(contextAllows, contextDenies)
			if err != nil {
				return err
			}
			validation, err := r.testContextPolicyCandidate(ctx, manifest, paths.PolicyDirectory, data)
			if err != nil {
				return fault.Wrap(fault.KindRejected, "policy_preflight_failed", "candidate Context policy failed OPA tests", false, err)
			}
			updates = append(updates, contextUpdate{
				policyDirectory: paths.PolicyDirectory,
				original:        file.sources,
				candidate:       data.sources,
				validation:      validation,
			})
		}
		transactions := make(map[string]*policySourceTransaction, len(updates))
		orderedTransactions := make([]*policySourceTransaction, 0, len(updates))
		rollback := func() error {
			failures := make([]error, 0)
			for index := len(orderedTransactions) - 1; index >= 0; index-- {
				if rollbackErr := orderedTransactions[index].rollback(); rollbackErr != nil {
					failures = append(failures, rollbackErr)
				}
			}
			rollbackContext, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if activationErr := r.activatePolicyRevision(rollbackContext, stored, false); activationErr != nil {
				failures = append(failures, activationErr)
			}
			return errors.Join(failures...)
		}
		rollbackAfter := func(cause error) error {
			if rollbackErr := rollback(); rollbackErr != nil {
				return fault.Wrap(
					fault.KindInternal, "policy_write_failed",
					"aggregate policy failure left source recovery pending", false, errors.Join(cause, rollbackErr),
				)
			}
			return cause
		}
		for _, update := range updates {
			transaction, beginErr := beginPolicySourceTransaction(update.policyDirectory, update.original, update.candidate)
			if beginErr != nil {
				return rollbackAfter(beginErr)
			}
			if bindErr := transaction.bindCandidateValidation(update.validation); bindErr != nil {
				orderedTransactions = append(orderedTransactions, transaction)
				return rollbackAfter(bindErr)
			}
			transactions[update.policyDirectory] = transaction
			orderedTransactions = append(orderedTransactions, transaction)
		}
		projection, err := r.buildAggregateProjectionWithTransactions(ctx, transactions)
		if err != nil {
			return rollbackAfter(fault.Wrap(fault.KindRejected, "aggregate_policy_invalid", "candidate aggregate policy was not activated", false, err))
		}
		for _, transaction := range orderedTransactions {
			if err := transaction.setCandidateAggregateRevision(projection.Revision); err != nil {
				return rollbackAfter(fmt.Errorf("record candidate aggregate policy revision: %w", err))
			}
		}
		candidateState := stored
		candidateState.AggregateRevision = projection.Revision
		candidateState.ManifestCount = projection.ManifestCount
		candidateState.PolicyDirectory = projection.PolicyDirectory
		candidateState.GatewayConfig = projection.GatewayConfig
		if candidateState.SchemaVersion == 2 {
			candidateState.Applied.AggregateRevision = projection.Revision
		}
		candidateReceipt := tobari.PolicyActivationReceipt{
			PolicyDirectory: candidateState.PolicyDirectory,
			ActiveRevision:  candidateState.AggregateRevision,
		}
		if err := candidateReceipt.Validate(); err != nil {
			return rollbackAfter(err)
		}
		if err := r.activatePolicyRevision(
			ctx, candidateState,
			policyAuthorityReduces(expectedAllows, updatedAllows, expectedDenies, updatedDenies),
		); err != nil {
			return rollbackAfter(err)
		}
		if err := r.writeState(candidateState); err != nil {
			return rollbackAfter(fmt.Errorf("persist aggregate policy activation: %w", err))
		}
		for _, transaction := range orderedTransactions {
			if err := transaction.commit(); err != nil {
				return fmt.Errorf("finalize policy source transaction: %w", err)
			}
		}
		receipt = candidateReceipt
		return nil
	})
	return receipt, err
}

// ApplyPolicyDecisionSet records a bounded reviewed set in one Context source
// and performs exactly one aggregate activation. The one-source bound keeps
// source promotion atomic across process interruption.
func (r *Runtime) ApplyPolicyDecisionSet(
	ctx context.Context, state tobari.State,
	expectedAllows, updatedAllows []tobari.LearnedPolicyRule,
	expectedDenies, updatedDenies []tobari.PolicyDenyRule,
) (tobari.PolicyActivationReceipt, error) {
	return r.applyAggregatePolicyData(
		ctx, state, expectedAllows, updatedAllows, expectedDenies, updatedDenies, true,
	)
}
