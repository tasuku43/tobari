package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func newPolicyPresetTestRuntime(t *testing.T) *Runtime {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &contextSwitchRunner{})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func TestPolicyPresetStoreListsThreeBuiltinsAndCreatesOwnerOnlyCustomWithoutOverwrite(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	listed, err := runtime.ListPolicyPresets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 3 {
		t.Fatalf("fresh preset catalog = %+v", listed.Items)
	}
	created, err := runtime.InitPolicyPreset(context.Background(), "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if created.Origin != "custom/restricted" || created.Preset == nil || created.Preset.Guardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatalf("created preset = %+v", created)
	}
	path := filepath.Join(runtime.policyPresetCustomDirectory(), "restricted.json")
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("custom preset mode = %v", info.Mode())
	}
	before, _ := os.ReadFile(path)
	if _, err := runtime.InitPolicyPreset(context.Background(), "restricted"); err == nil {
		t.Fatal("custom preset was overwritten")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed init changed existing preset")
	}
}

func TestPolicyPresetListFailsClosedOnUnsafeCustomCatalogEntry(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if err := runtime.ensurePrivateDirectory(filepath.Dir(runtime.policyPresetCustomDirectory())); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensurePrivateDirectory(runtime.policyPresetCustomDirectory()); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(runtime.policyPresetCustomDirectory(), "unsafe.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ListPolicyPresets(context.Background()); err == nil {
		t.Fatal("unsafe custom catalog entry was silently omitted")
	}
}

func TestPolicyPresetStoreRejectsUnknownDuplicateExecutableAndUnsafeSources(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "hostile"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runtime.policyPresetCustomDirectory(), "hostile.json")
	fixtures := []string{
		`{"schema_version":1,"name":"hostile","name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[]}`,
		`{"schema_version":1,"name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[],"rego":"allow=true"}`,
		`{"schema_version":1,"name":"hostile","guardrail":"offline","authorities":[],"methods":[],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[],"include":"https://example.invalid/preset"}`,
		`{"schema_version":1,"name":"hostile","guardrail":"reviewed_exact","authorities":[{"scheme":"https","host":"127.0.0.1","port":443}],"methods":["GET"],"baseline_grants":[],"baseline_denies":[],"graphql_endpoints":[]}`,
	}
	for _, fixture := range fixtures {
		if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.ValidatePolicyPreset(context.Background(), "custom/hostile"); err == nil {
			t.Fatalf("hostile preset accepted: %s", fixture)
		}
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ValidatePolicyPreset(context.Background(), "custom/hostile"); err == nil {
		t.Fatal("group/world-readable preset accepted")
	}
}

func TestContextSnapshotsPresetAndIgnoresLaterSourceEdit(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "frozen"); err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateContextWithPreset(context.Background(), "frozen-context", tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite, "custom/frozen")
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(runtime.contextPresetPath("frozen-context"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(runtime.policyPresetCustomDirectory(), "frozen.json")
	edited := strings.Replace(string(before), `"offline"`, `"get_only_reviewed"`, 1)
	if err := os.WriteFile(source, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(runtime.contextPresetPath("frozen-context"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || created.PolicyGuardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatal("source edit changed Context snapshot authority")
	}
	manifest, err := runtime.readContextManifest("frozen-context")
	if err != nil {
		t.Fatal(err)
	}
	preset, err := runtime.readContextPreset(manifest)
	if err != nil || preset.Guardrail != tobari.PolicyPresetGuardrailOffline {
		t.Fatalf("snapshotted preset = %+v, %v", preset, err)
	}
}

func TestDefaultContextCanReuseSnapshottedCustomPresetAfterSourceEdit(t *testing.T) {
	runtime := newPolicyPresetTestRuntime(t)
	if _, err := runtime.InitPolicyPreset(context.Background(), "snapshot"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(runtime.policyPresetCustomDirectory(), "snapshot.json")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), `"offline"`, `"reviewed_exact"`, 1)
	if err := os.WriteFile(source, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CreateContextWithPreset(context.Background(), tobari.DefaultContextName, tobari.OfficialRuntimeBase, tobari.ContextPolicyModeGuided, tobari.ContextSourceAccessReadWrite, "custom/snapshot"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, data, 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := runtime.UseContext(context.Background(), tobari.DefaultContextName)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Active || report.PolicyPresetOrigin != "custom/snapshot" || report.PolicyGuardrail != tobari.PolicyPresetGuardrailReviewedExact {
		t.Fatalf("reused custom preset Context = %+v", report)
	}
}
