package dockerruntime_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

type blockingLegacyAuthorityGuard struct {
	runtime    *dockerruntime.Runtime
	afterFirst func()
	calls      int
}

func (g *blockingLegacyAuthorityGuard) ConfirmNoPreReleaseLegacyAuthority(ctx context.Context, finalInitialized bool) error {
	g.calls++
	err := g.runtime.ConfirmNoPreReleaseLegacyAuthority(ctx, finalInitialized)
	if g.calls == 1 && g.afterFirst != nil {
		g.afterFirst()
	}
	return err
}

type countingLifecycleAuthority struct {
	runtime *dockerruntime.Runtime
	calls   int
}

func (a *countingLifecycleAuthority) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	a.calls++
	return a.runtime.WithLifecycleLock(ctx, action)
}

func TestConfigOnlyLegacyResearchAuthorityBlocksFreshFinalStoreWithoutMutation(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config-home")
	stateHome := filepath.Join(root, "state-home")
	dataHome := filepath.Join(root, "data-home")
	for _, path := range []string{configHome, stateHome, dataHome} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	providerRoot := filepath.Join(configHome, "tobari", "auth", "providers")
	if err := os.MkdirAll(providerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerRoot, "legacy.json"), []byte("legacy-provider"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := dockerruntime.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspaceauthoritystore.NewFinalOnly(filepath.Join(stateHome, "tobari", "workspace-authority"), runtime)
	if err != nil {
		t.Fatal(err)
	}
	before := legacyGuardTree(t, root)
	if _, _, err := store.ReadComplete(context.Background()); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("legacy final-store error = %v", err)
	}
	after := legacyGuardTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("legacy rejection mutated installation:\nbefore=%v\nafter=%v", before, after)
	}
	for _, forbidden := range []string{
		filepath.Join(stateHome, "tobari", "workspace-authority"),
		filepath.Join(stateHome, "tobari", "installation.lock"),
		filepath.Join(stateHome, "tobari", "auth"),
	} {
		if _, err := os.Lstat(forbidden); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy rejection created %q: %v", forbidden, err)
		}
	}
}

func TestConfigOnlyLegacyResearchAuthorityAppearingDuringFreshFinalObservationFailsClosed(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config-home")
	stateHome := filepath.Join(root, "state-home")
	dataHome := filepath.Join(root, "data-home")
	for _, path := range []string{configHome, stateHome, dataHome} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	runtime, err := dockerruntime.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	guard := &blockingLegacyAuthorityGuard{
		runtime: runtime,
		afterFirst: func() {
			if err := os.MkdirAll(filepath.Join(configHome, "tobari", "auth", "providers"), 0o700); err != nil {
				t.Fatal(err)
			}
		},
	}
	store, err := workspaceauthoritystore.NewFinalOnly(filepath.Join(stateHome, "tobari", "workspace-authority"), guard)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.ReadComplete(context.Background()); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("legacy authority appearing between reads was accepted: %v", err)
	}
	if guard.calls != 2 {
		t.Fatalf("fresh final observation guard calls = %d, want 2", guard.calls)
	}
	if _, err := os.Lstat(filepath.Join(stateHome, "tobari", "workspace-authority")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy drift rejection created final authority: %v", err)
	}
}

func TestPredecessorHostLoopbackSchemaBlocksFreshFinalStoreWithoutMutation(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	stateHome := filepath.Join(root, "state")
	dataHome := filepath.Join(root, "data")
	for _, path := range []string{configHome, stateHome, dataHome} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	legacyRoot := filepath.Join(configHome, "tobari", "host-loopback")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"routes.json": `{"schema_version":1,"routes":[]}`,
		"grants.json": `{"schema_version":1,"grants":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(legacyRoot, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := dockerruntime.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	store, err := workspaceauthoritystore.NewFinalOnly(filepath.Join(stateHome, "tobari", "workspace-authority"), runtime)
	if err != nil {
		t.Fatal(err)
	}
	before := legacyGuardTree(t, root)
	if _, _, err := store.ReadComplete(context.Background()); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("predecessor Host Loopback final-store error = %v", err)
	}
	after := legacyGuardTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("predecessor Host Loopback rejection mutated installation:\nbefore=%v\nafter=%v", before, after)
	}
	for _, forbidden := range []string{
		filepath.Join(stateHome, "tobari", "workspace-authority"),
		filepath.Join(stateHome, "tobari", "lifecycle.lock"),
	} {
		if _, err := os.Lstat(forbidden); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("predecessor Host Loopback rejection created %q: %v", forbidden, err)
		}
	}
}

func TestEveryDeclaredLegacyRootBlocksFreshFinalStoreWithoutMutation(t *testing.T) {
	paths := []string{
		"config/tobari/contexts",
		"state/tobari/roots",
		"state/tobari/instances",
		"state/tobari/auth/projects",
		"state/tobari/state.json",
		"state/tobari/project-journal.json",
		"state/tobari/cluster-reconcile.json",
		"config/tobari/migrations",
		"state/tobari/migrations",
		"state/tobari/cluster-projections",
		"config/tobari/principal-registry",
		"state/tobari/auth",
		"config/tobari/auth",
		"data/tobari/profiles",
		"config/tobari/host-loopback",
		"config/tobari/interactive-attachments",
		"state/tobari/service-exposure",
	}
	for _, relative := range paths {
		t.Run(filepath.Base(relative), func(t *testing.T) {
			root := t.TempDir()
			configHome := filepath.Join(root, "config")
			stateHome := filepath.Join(root, "state")
			dataHome := filepath.Join(root, "data")
			for _, path := range []string{configHome, stateHome, dataHome} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("XDG_STATE_HOME", stateHome)
			t.Setenv("XDG_DATA_HOME", dataHome)
			legacy := filepath.Join(root, filepath.FromSlash(relative))
			if filepath.Ext(legacy) == ".json" {
				if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(legacy, []byte("{must-not-be-decoded\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(legacy, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(legacy, "must-not-be-decoded"), []byte("{hostile predecessor bytes\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			runtime, err := dockerruntime.New(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			store, err := workspaceauthoritystore.NewFinalOnly(filepath.Join(stateHome, "tobari", "workspace-authority"), runtime)
			if err != nil {
				t.Fatal(err)
			}
			before := legacyGuardTree(t, root)
			if _, _, err := store.ReadComplete(context.Background()); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
				t.Fatalf("legacy root %q final-store error = %v", relative, err)
			}
			after := legacyGuardTree(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("legacy root %q rejection mutated installation:\nbefore=%v\nafter=%v", relative, before, after)
			}
			for _, forbidden := range []string{
				filepath.Join(stateHome, "tobari", "workspace-authority"),
				filepath.Join(stateHome, "tobari", "lifecycle.lock"),
			} {
				if _, err := os.Lstat(forbidden); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("legacy root %q rejection created %q: %v", relative, forbidden, err)
				}
			}
		})
	}
}

func TestConfigOnlyLegacyAuthorityRejectsTemplateCreateBeforeLifecycleLock(t *testing.T) {
	root := t.TempDir()
	configHome := filepath.Join(root, "config")
	stateHome := filepath.Join(root, "state")
	dataHome := filepath.Join(root, "data")
	for _, path := range []string{configHome, dataHome} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	legacy := filepath.Join(configHome, "tobari", "contexts")
	if err := os.MkdirAll(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "legacy.json"), []byte("{must-not-be-decoded\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runtime, err := dockerruntime.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	guard := &blockingLegacyAuthorityGuard{runtime: runtime}
	store, err := workspaceauthoritystore.NewFinalOnly(filepath.Join(stateHome, "tobari", "workspace-authority"), guard)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := &countingLifecycleAuthority{runtime: runtime}
	mutator, err := workspaceauthoritystore.NewMutator(context.Background(), store, lifecycle, runtime, runtime, runtime, runtime)
	if err != nil {
		t.Fatal(err)
	}
	body := legacyGuardTemplateBody()
	if err := body.Validate(); err != nil {
		t.Fatalf("legacy Template fixture is invalid: %v", err)
	}
	before := legacyGuardTree(t, root)
	if _, err := mutator.CreateWorkspaceTemplate(context.Background(), "standard", body); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
		t.Fatalf("legacy Template create error = %v", err)
	}
	if guard.calls != 1 {
		t.Fatalf("legacy guard calls = %d, want one pre-lock observation", guard.calls)
	}
	if lifecycle.calls != 0 {
		t.Fatalf("legacy Template create entered lifecycle lock %d times", lifecycle.calls)
	}
	after := legacyGuardTree(t, root)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("legacy Template create mutated installation:\nbefore=%v\nafter=%v", before, after)
	}
	for _, forbidden := range []string{
		filepath.Join(stateHome, "tobari"),
		filepath.Join(stateHome, "tobari", "lifecycle.lock"),
		filepath.Join(stateHome, "tobari", "workspace-authority"),
	} {
		if _, err := os.Lstat(forbidden); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy Template create created %q: %v", forbidden, err)
		}
	}
}

func legacyGuardTemplateBody() tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadWrite,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{},
			MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{},
			GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{
			RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("f", 64), Ordinal: 1, Image: "tobari-runtime:test",
		}},
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}

func legacyGuardTree(t *testing.T, root string) []string {
	t.Helper()
	items := []string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		items = append(items, relative+":"+entry.Type().String())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sort.Strings(items)
	return items
}
