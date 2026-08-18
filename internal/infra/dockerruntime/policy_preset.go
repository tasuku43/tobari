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
	"sort"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const maxPolicyPresetBytes = 64 * 1024

func (r *Runtime) policyPresetCustomDirectory() string {
	return filepath.Join(r.configDirectory, "policy-presets", "custom")
}

func decodePolicyPreset(data []byte) (tobari.PolicyPreset, []byte, string, error) {
	if len(data) == 0 || len(data) > maxPolicyPresetBytes {
		return tobari.PolicyPreset{}, nil, "", fmt.Errorf("policy preset size is invalid")
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var preset tobari.PolicyPreset
	if err := decoder.Decode(&preset); err != nil {
		return tobari.PolicyPreset{}, nil, "", fmt.Errorf("decode policy preset: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.PolicyPreset{}, nil, "", fmt.Errorf("policy preset contains trailing data")
	}
	return tobari.NormalizePolicyPreset(preset)
}

func (r *Runtime) resolvePolicyPreset(origin string) (tobari.PolicyPreset, []byte, string, error) {
	if err := tobari.ValidatePolicyPresetOrigin(origin); err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	if preset, ok := tobari.BuiltinPolicyPreset(origin); ok {
		return tobari.NormalizePolicyPreset(preset)
	}
	name := strings.TrimPrefix(origin, "custom/")
	path := filepath.Join(r.policyPresetCustomDirectory(), name+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxPolicyPresetBytes {
		return tobari.PolicyPreset{}, nil, "", fmt.Errorf("custom policy preset is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- selector is validated and bounded below the owner store.
	if err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	preset, normalized, revision, err := decodePolicyPreset(data)
	if err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	if preset.Name != name {
		return tobari.PolicyPreset{}, nil, "", fmt.Errorf("custom policy preset name does not match its selector")
	}
	return preset, normalized, revision, nil
}

func (r *Runtime) resolvePolicyPresetSnapshot(origin string) (tobari.PolicyPreset, []byte, string, error) {
	if err := tobari.ValidatePolicyPresetOrigin(origin); err != nil {
		return tobari.PolicyPreset{}, nil, "", err
	}
	if preset, ok := tobari.BuiltinPolicyPresetSnapshot(origin); ok {
		return tobari.NormalizePolicyPreset(preset)
	}
	return r.resolvePolicyPreset(origin)
}

func (r *Runtime) contextPresetPath(name string) string {
	return filepath.Join(r.contextPolicyDirectory(name), "preset.json")
}

func (r *Runtime) ensureContextPreset(manifest tobari.ContextManifest) error {
	path := r.contextPresetPath(manifest.Name)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxPolicyPresetBytes {
			return fmt.Errorf("Context policy preset snapshot is unsafe")
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path is an owned Context child.
		if err != nil {
			return err
		}
		_, normalized, revision, err := decodePolicyPreset(data)
		if err != nil {
			return err
		}
		if revision != manifest.PolicyPresetRevision || !bytes.Equal(data, normalized) {
			return fmt.Errorf("Context policy preset snapshot does not match its manifest")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, normalized, revision, err := r.resolvePolicyPresetSnapshot(manifest.PolicyPresetOrigin)
	if err != nil {
		return err
	}
	if revision != manifest.PolicyPresetRevision {
		return fmt.Errorf("policy preset revision changed before Context snapshot")
	}
	return initializeBytes(path, normalized, 0o600)
}

func (r *Runtime) readContextPreset(manifest tobari.ContextManifest) (tobari.PolicyPreset, error) {
	path := r.contextPresetPath(manifest.Name)
	info, err := os.Lstat(path)
	if err != nil {
		return tobari.PolicyPreset{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Size() > maxPolicyPresetBytes {
		return tobari.PolicyPreset{}, fmt.Errorf("Context policy preset snapshot is unsafe")
	}
	data, err := os.ReadFile(path) // #nosec G304 -- owned Context child.
	if err != nil {
		return tobari.PolicyPreset{}, err
	}
	preset, _, revision, err := decodePolicyPreset(data)
	if err != nil {
		return tobari.PolicyPreset{}, err
	}
	if revision != manifest.PolicyPresetRevision {
		return tobari.PolicyPreset{}, fmt.Errorf("Context policy preset revision mismatch")
	}
	_, normalized, _, err := decodePolicyPreset(data)
	if err != nil || !bytes.Equal(data, normalized) {
		return tobari.PolicyPreset{}, fmt.Errorf("Context policy preset snapshot is not normalized")
	}
	return preset, nil
}

func policyPresetSummary(origin string, preset tobari.PolicyPreset, revision string) tobari.PolicyPresetSummary {
	return tobari.PolicyPresetSummary{Origin: origin, Revision: revision, Guardrail: preset.Guardrail, BaselineGrantCount: len(preset.BaselineGrants) + len(preset.BaselineTemplates) + len(preset.MCPBaselineGrants), DestinationCeiling: preset.DestinationCeiling.Mode, DestinationCount: len(preset.DestinationCeiling.Authorities), MethodDefault: preset.MethodPolicy.Default, MethodOverrideCount: len(preset.MethodPolicy.Overrides)}
}

func (r *Runtime) ListPolicyPresets(ctx context.Context) (tobari.PolicyPresetResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	origins := []string{"builtin/offline", tobari.DefaultPolicyPresetOrigin, "builtin/reviewed-exact", "builtin/get-only-reviewed", "builtin/public-get-reviewed"}
	entries, err := os.ReadDir(r.policyPresetCustomDirectory())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return tobari.PolicyPresetResult{}, err
	}
	for _, entry := range entries {
		info, infoErr := entry.Info()
		if infoErr != nil || !entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") || info.Mode().Perm()&0o077 != 0 || info.Size() > maxPolicyPresetBytes {
			return tobari.PolicyPresetResult{}, fmt.Errorf("custom policy preset catalog contains unsafe entry %q", entry.Name())
		}
		origin := "custom/" + strings.TrimSuffix(entry.Name(), ".json")
		if err := tobari.ValidatePolicyPresetOrigin(origin); err != nil {
			return tobari.PolicyPresetResult{}, fmt.Errorf("custom policy preset catalog contains invalid entry %q", entry.Name())
		}
		origins = append(origins, origin)
	}
	sort.Strings(origins)
	items := make([]tobari.PolicyPresetSummary, 0, len(origins))
	for _, origin := range origins {
		preset, _, revision, err := r.resolvePolicyPreset(origin)
		if err != nil {
			return tobari.PolicyPresetResult{}, err
		}
		items = append(items, policyPresetSummary(origin, preset, revision))
	}
	return tobari.PolicyPresetResult{Task: tobari.TaskPolicyPresetList, Items: items}, nil
}

func (r *Runtime) ShowPolicyPreset(ctx context.Context, origin string) (tobari.PolicyPresetResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	preset, _, revision, err := r.resolvePolicyPreset(origin)
	if err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	return r.policyPresetResult(tobari.TaskPolicyPresetShow, origin, preset, revision), nil
}

func (r *Runtime) ValidatePolicyPreset(ctx context.Context, origin string) (tobari.PolicyPresetResult, error) {
	result, err := r.ShowPolicyPreset(ctx, origin)
	if err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	result.Task = tobari.TaskPolicyPresetValidate
	return result, nil
}

func (r *Runtime) policyPresetResult(task, origin string, preset tobari.PolicyPreset, revision string) tobari.PolicyPresetResult {
	limitations := []string{"Immediate grants are semantic network effects available to every process in the Context; they do not identify an agent executable.", "MCP payload arguments and responses are not authorization dimensions.", "No immediate grant is automatically safe or read-only.", "Source changes affect only future Context creation; trusted binary native-readiness updates apply to existing builtin/agent-ready Contexts.", "The preset contains no executable policy, secret, wildcard, inheritance, include, or remote fetch."}
	scope := "One Context-wide immutable network ceiling and normalized baseline snapshot."
	if origin == tobari.DefaultPolicyPresetOrigin {
		scope = "The installed binary's current effective agent-ready source; Context snapshots persist the core preset and select the binary readiness overlay by exact origin."
	}
	result := tobari.PolicyPresetResult{Task: task, Origin: origin, Revision: revision, Preset: &preset, Scope: scope, Limitations: limitations}
	if strings.HasPrefix(origin, "custom/") {
		result.SourcePath = filepath.Join(r.policyPresetCustomDirectory(), strings.TrimPrefix(origin, "custom/")+".json")
	}
	return result
}

func (r *Runtime) InitPolicyPreset(ctx context.Context, name string) (tobari.PolicyPresetResult, error) {
	if err := ctx.Err(); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if err := tobari.ValidatePolicyPresetOrigin("custom/" + name); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if err := r.ensurePrivateDirectory(filepath.Dir(r.policyPresetCustomDirectory())); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	if err := r.ensurePrivateDirectory(r.policyPresetCustomDirectory()); err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	preset := tobari.PolicyPreset{SchemaVersion: 1, Name: name, Guardrail: tobari.PolicyPresetGuardrailMethodPolicy, DestinationCeiling: tobari.PolicyPresetDestinationCeiling{Mode: "public_https", Authorities: []tobari.PolicyPresetAuthority{}}, MethodPolicy: tobari.PolicyPresetMethodPolicy{Default: tobari.PolicyPresetMethodDeny, Overrides: []tobari.PolicyPresetMethodOverride{}}, BaselineGrants: []tobari.PolicyPresetExactRule{}, BaselineTemplates: []tobari.PolicyPresetPathTemplateRule{}, MCPBaselineGrants: []tobari.PolicyPresetMCPRule{}, BaselineDenies: []tobari.PolicyPresetExactRule{}, GraphQLEndpoints: []tobari.PolicyPresetExactRule{}, MCPEndpoints: []tobari.PolicyPresetExactRule{}}
	normalizedPreset, normalized, revision, err := tobari.NormalizePolicyPreset(preset)
	if err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	path := filepath.Join(r.policyPresetCustomDirectory(), name+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G304 -- validated fixed owner-store child.
	if err != nil {
		return tobari.PolicyPresetResult{}, err
	}
	_, writeErr := file.Write(normalized)
	closeErr := file.Close()
	if writeErr != nil {
		return tobari.PolicyPresetResult{}, writeErr
	}
	if closeErr != nil {
		return tobari.PolicyPresetResult{}, closeErr
	}
	return r.policyPresetResult(tobari.TaskPolicyPresetInit, "custom/"+name, normalizedPreset, revision), nil
}
