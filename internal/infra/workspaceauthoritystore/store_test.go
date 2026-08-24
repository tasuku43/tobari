package workspaceauthoritystore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestEncodeCompleteUsesTheExactBoundedStoreContract(t *testing.T) {
	collection := storeCollectionFixture(t)
	got, err := EncodeComplete(collection)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(collection)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		index := 0
		for index < len(got) && index < len(want) && got[index] == want[index] {
			index++
		}
		start := index - 40
		if start < 0 {
			start = 0
		}
		end := index + 80
		if end > len(got) {
			end = len(got)
		}
		wantEnd := index + 80
		if wantEnd > len(want) {
			wantEnd = len(want)
		}
		t.Fatalf("bounded encoding drift at %d: got=%q want=%q", index, got[start:end], want[start:wantEnd])
	}

	buffer := boundedJSONBuffer{maximum: MaxAuthorityBytes}
	exact := bytes.Repeat([]byte{'x'}, MaxAuthorityBytes)
	if err := buffer.write(exact); err != nil || buffer.Len() != MaxAuthorityBytes {
		t.Fatalf("exact boundary len=%d err=%v", buffer.Len(), err)
	}
	if err := buffer.write([]byte{'x'}); err == nil || buffer.Len() != MaxAuthorityBytes {
		t.Fatalf("over-bound write len=%d err=%v", buffer.Len(), err)
	}
}

const (
	storeTemplateID  tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
	storeContextID   tobari.ContextID           = "01912345-6789-7abc-8def-0123456789a2"
	storeWorkspaceID tobari.WorkspaceID         = "01912345-6789-7abc-8def-0123456789a3"
)

type legacyGuardFake struct {
	errors      []error
	after       func(int)
	calls       int
	initialized []bool
}

func (f *legacyGuardFake) ConfirmNoPreReleaseLegacyAuthority(_ context.Context, finalInitialized bool) error {
	f.calls++
	f.initialized = append(f.initialized, finalInitialized)
	if f.after != nil {
		f.after(f.calls)
	}
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	if len(f.errors) > 1 {
		f.errors = f.errors[1:]
	}
	return err
}

func storeCollectionFixture(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	body := tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess: tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{
				Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}},
			},
			MethodPolicy: tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants:    []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: "/items"}},
			BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{},
			BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{
			RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("f", 64), Ordinal: 1, Image: tobari.OfficialRuntimeBase,
		}},
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(storeTemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{
		SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: storeTemplateID, Name: "restricted",
		Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()},
	}
	binding := tobari.ContextBinding{
		SchemaVersion: tobari.ContextBindingSchemaVersion, ID: storeContextID,
		ProjectRoot: "/workspace/example", TemplateID: storeTemplateID,
	}
	memory, _, err := tobari.PublishPolicyMemory(storeContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{
		ContextID: storeContextID, TemplateID: storeTemplateID, PolicySliceDigest: revision.Slices.PolicySliceDigest,
	}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: storeContextID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	record := tobari.WorkspaceAuthorityContextRecord{
		Context: binding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt,
		ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt,
	}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: storeContextID, TemplateID: storeTemplateID, TemplateRevision: revision.Revision,
		EntrySliceDigest: revision.Slices.EntrySliceDigest, RuntimeID: revision.Slices.RuntimeID,
		RuntimeRevision: revision.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("7", 64)),
		ReconciledAt: time.Unix(1, 0).UTC(),
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: storeWorkspaceID, ContextID: storeContextID,
		ProjectRoot: binding.ProjectRoot, Home: "/workspace/home", CreationDefaults: revision.Slices.CreationDefaultsDigest,
		LastSuccessfulEntry: &applied,
	}
	effect := tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/candidate",
		Segments: []string{}, Examples: []string{"/candidate"},
	}
	candidate, err := tobari.NewPolicyCandidateAuthority(storeContextID, storeWorkspaceID, effect)
	if err != nil {
		t.Fatal(err)
	}
	defaultID := storeTemplateID
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record},
		[]tobari.WorkspaceBinding{workspace}, []tobari.PolicyCandidateAuthority{candidate}, &defaultID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestWorkspaceAuthorityCollectionMakesWorkspaceHomeAnExclusiveOwnerBoundary(t *testing.T) {
	collection := storeCollectionFixture(t)
	secondTemplateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789b1")
	secondContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789b2")
	secondWorkspaceID := tobari.WorkspaceID("01912345-6789-7abc-8def-0123456789b3")
	secondTemplate, err := tobari.CopyWorkspaceTemplateRevision(secondTemplateID, "second", collection.Templates[0].Current)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(secondContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	contextBinding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: secondContextID, ProjectRoot: collection.Contexts[0].Context.ProjectRoot, TemplateID: secondTemplateID}
	templateReceipt := tobari.TemplatePolicyActivationReceipt{ContextID: secondContextID, TemplateID: secondTemplateID, PolicySliceDigest: secondTemplate.Current.Slices.PolicySliceDigest}
	memoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: secondContextID, Revision: memory.Revision}
	activeMemory := memory.Clone()
	record := tobari.WorkspaceAuthorityContextRecord{Context: contextBinding, PolicyMemory: memory, ActiveTemplatePolicy: &templateReceipt, ActivePolicyMemory: &activeMemory, ActivePolicyMemoryRef: &memoryReceipt}
	applied := tobari.WorkspaceAppliedEntry{
		ContextID: secondContextID, TemplateID: secondTemplateID, TemplateRevision: secondTemplate.Current.Revision,
		EntrySliceDigest: secondTemplate.Current.Slices.EntrySliceDigest, RuntimeID: secondTemplate.Current.Slices.RuntimeID,
		RuntimeRevision: secondTemplate.Current.Slices.RuntimeRevision, ResolvedSpec: tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), ReconciledAt: time.Unix(2, 0).UTC(),
	}
	workspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: secondWorkspaceID, ContextID: secondContextID,
		ProjectRoot: contextBinding.ProjectRoot, Home: collection.Workspaces[0].Home,
		CreationDefaults: secondTemplate.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &applied,
	}
	templates := append(collection.Templates, secondTemplate)
	contexts := append(collection.Contexts, record)
	workspaces := append(collection.Workspaces, workspace)
	if _, _, err := tobari.PublishWorkspaceAuthorityCollection(templates, contexts, workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil); err == nil {
		t.Fatal("two Workspace IDs shared one standard home authority")
	}
	workspaces[1].Home = "/workspace/home-" + string(secondWorkspaceID)
	if _, _, err := tobari.PublishWorkspaceAuthorityCollection(templates, contexts, workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil); err != nil {
		t.Fatalf("unique WorkspaceID-derived homes failed: %v", err)
	}
}

func materializeCollection(t *testing.T, root string, collection tobari.WorkspaceAuthorityCollection) []byte {
	t.Helper()
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(collection)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, authorityFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestStoreAbsentObservationCreatesNothing(t *testing.T) {
	root := filepath.Join(t.TempDir(), "final-authority")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if templates, err := store.ListWorkspaceTemplates(context.Background()); err != nil || templates == nil || len(templates) != 0 {
		t.Fatalf("templates=%#v err=%v", templates, err)
	}
	if contexts, err := store.ListContextAuthority(context.Background()); err != nil || contexts == nil || len(contexts) != 0 {
		t.Fatalf("contexts=%#v err=%v", contexts, err)
	}
	if workspaces, err := store.ListWorkspaceAuthority(context.Background()); err != nil || workspaces == nil || len(workspaces) != 0 {
		t.Fatalf("workspaces=%#v err=%v", workspaces, err)
	}
	templateRef, _ := tobari.WorkspaceTemplateRef(storeTemplateID)
	contextRef, _ := tobari.ContextRef(storeContextID)
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	if _, err := store.DiscoverWorkspaceTemplate(context.Background(), "restricted"); !errors.Is(err, tobari.ErrWorkspaceTemplateNotFound) {
		t.Fatalf("missing template error=%v", err)
	}
	if _, err := store.ReadContextAuthorityByReference(context.Background(), contextRef); !errors.Is(err, tobari.ErrContextBindingNotFound) {
		t.Fatalf("missing Context error=%v", err)
	}
	if _, err := store.ReadWorkspaceAuthorityByReference(context.Background(), workspaceRef); !errors.Is(err, tobari.ErrWorkspaceBindingNotFound) {
		t.Fatalf("missing Workspace error=%v", err)
	}
	if _, err := tobari.ParseWorkspaceTemplateRef(templateRef); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only observation created final store: %v", err)
	}
}

func TestStoreProjectsExactFinalRuntimeProtectionAuthorityWithoutCreatingState(t *testing.T) {
	t.Run("absent", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		store, err := New(root)
		if err != nil {
			t.Fatal(err)
		}
		authority, err := store.ReadFinalRuntimeProtectionAuthority(context.Background())
		if err != nil || authority.Present || authority.Templates == nil || authority.Contexts == nil || authority.Workspaces == nil {
			t.Fatalf("absent protection authority = %+v/%v", authority, err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("protection read created final store: %v", err)
		}
	})
	t.Run("complete", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		collection := storeCollectionFixture(t)
		materializeCollection(t, root, collection)
		store, err := New(root)
		if err != nil {
			t.Fatal(err)
		}
		authority, err := store.ReadFinalRuntimeProtectionAuthority(context.Background())
		if err != nil || !authority.Present || authority.CollectionGeneration != collection.Generation || authority.CollectionRevision != collection.Revision || len(authority.Templates) != 1 || len(authority.Contexts) != 1 || len(authority.Workspaces) != 1 {
			t.Fatalf("complete protection authority = %+v/%v", authority, err)
		}
	})
}

func TestFinalOnlyStoreAcceptsFreshOrCompleteFinalOnlyWhenLegacyIsAbsent(t *testing.T) {
	ctx := context.Background()
	t.Run("fresh empty", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		guard := &legacyGuardFake{}
		store, err := NewFinalOnly(root, guard)
		if err != nil {
			t.Fatal(err)
		}
		if _, present, err := store.ReadComplete(ctx); err != nil || present {
			t.Fatalf("fresh present=%t err=%v", present, err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("fresh observation created state: %v", err)
		}
		if !reflect.DeepEqual(guard.initialized, []bool{false, false}) {
			t.Fatalf("fresh guard dimensions = %v", guard.initialized)
		}
	})

	t.Run("final", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		want := storeCollectionFixture(t)
		materializeCollection(t, root, want)
		guard := &legacyGuardFake{}
		store, err := NewFinalOnly(root, guard)
		if err != nil {
			t.Fatal(err)
		}
		got, present, err := store.ReadComplete(ctx)
		if err != nil || !present || got.Revision != want.Revision {
			t.Fatalf("final present=%t revision=%q err=%v", present, got.Revision, err)
		}
		if !reflect.DeepEqual(guard.initialized, []bool{true, true}) {
			t.Fatalf("final guard dimensions = %v", guard.initialized)
		}
	})

	t.Run("legacy present", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		store, _ := NewFinalOnly(root, &legacyGuardFake{errors: []error{errors.New("legacy Context root is present")}})
		if _, _, err := store.ReadComplete(ctx); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) || !strings.Contains(err.Error(), "reset or recreate") {
			t.Fatalf("legacy error=%v", err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("legacy rejection created final state: %v", err)
		}
	})

	t.Run("final envelope changes between observations", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		want := storeCollectionFixture(t)
		guard := &legacyGuardFake{
			after: func(call int) {
				if call == 1 {
					materializeCollection(t, root, want)
				}
			},
		}
		store, err := NewFinalOnly(root, guard)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ReadComplete(ctx); err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("envelope drift error=%v", err)
		}
	})

	t.Run("legacy appears between observations", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		want := storeCollectionFixture(t)
		materializeCollection(t, root, want)
		store, err := NewFinalOnly(root, &legacyGuardFake{errors: []error{nil, errors.New("legacy root appeared")}})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := store.ReadComplete(ctx); err == nil || !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
			t.Fatalf("legacy drift error=%v", err)
		}
	})
}

func TestStoreReturnsOneCoherentFinalAuthorityEnvelope(t *testing.T) {
	root := filepath.Join(t.TempDir(), "final-authority")
	want := storeCollectionFixture(t)
	materializeCollection(t, root, want)
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || got.Revision != want.Revision || got.Generation != want.Generation {
		t.Fatalf("complete=%#v present=%t err=%v", got, present, err)
	}
	got.Templates[0].Current.Body.Policy.BaselineGrants[0].Path = "/changed"
	again, _, err := store.ReadComplete(context.Background())
	if err != nil || again.Templates[0].Current.Body.Policy.BaselineGrants[0].Path != "/items" {
		t.Fatalf("store observation was mutated: %#v err=%v", again, err)
	}
	templates, err := store.ListWorkspaceTemplates(context.Background())
	if err != nil || len(templates) != 1 {
		t.Fatalf("templates=%#v err=%v", templates, err)
	}
	selected, err := store.DiscoverWorkspaceTemplate(context.Background(), "")
	if err != nil || selected.ID != storeTemplateID {
		t.Fatalf("default=%#v err=%v", selected, err)
	}
	contextRef, _ := tobari.ContextRef(storeContextID)
	contextSnapshot, err := store.ReadContextAuthorityByReference(context.Background(), contextRef)
	if err != nil || contextSnapshot.Context.ID != storeContextID || contextSnapshot.Workspace == nil {
		t.Fatalf("Context=%#v err=%v", contextSnapshot, err)
	}
	workspaceRef, _ := tobari.WorkspaceRef(storeWorkspaceID)
	workspaceSnapshot, err := store.ReadWorkspaceAuthorityByReference(context.Background(), workspaceRef)
	if err != nil || workspaceSnapshot.Workspace == nil || workspaceSnapshot.Workspace.ID != storeWorkspaceID {
		t.Fatalf("Workspace=%#v err=%v", workspaceSnapshot, err)
	}
}

func TestStoreFailsClosedOnUnsafePartialOrMalformedAuthority(t *testing.T) {
	valid := storeCollectionFixture(t)
	tests := map[string]func(*testing.T, string){
		"root mode": func(t *testing.T, root string) {
			materializeCollection(t, root, valid)
			if err := os.Chmod(root, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"missing envelope": func(t *testing.T, root string) {
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"mixed root": func(t *testing.T, root string) {
			materializeCollection(t, root, valid)
			if err := os.WriteFile(filepath.Join(root, "stale.json"), []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"file mode": func(t *testing.T, root string) {
			materializeCollection(t, root, valid)
			if err := os.Chmod(filepath.Join(root, authorityFileName), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"unknown field": func(t *testing.T, root string) {
			data := materializeCollection(t, root, valid)
			writeAuthorityBytes(t, root, append(data[:len(data)-1], []byte(`,"unknown":true}`)...))
		},
		"duplicate key": func(t *testing.T, root string) {
			data := materializeCollection(t, root, valid)
			data = []byte(strings.Replace(string(data), `"schema_version":1`, `"schema_version":1,"schema_version":1`, 1))
			writeAuthorityBytes(t, root, data)
		},
		"trailing value": func(t *testing.T, root string) {
			data := materializeCollection(t, root, valid)
			writeAuthorityBytes(t, root, append(data, []byte(` {}`)...))
		},
		"truncated": func(t *testing.T, root string) {
			data := materializeCollection(t, root, valid)
			writeAuthorityBytes(t, root, data[:len(data)/2])
		},
		"oversized": func(t *testing.T, root string) {
			materializeCollection(t, root, valid)
			if err := os.Truncate(filepath.Join(root, authorityFileName), MaxAuthorityBytes+1); err != nil {
				t.Fatal(err)
			}
		},
		"bounded count": func(t *testing.T, root string) {
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			templates := strings.Repeat(`{},`, maxWorkspaceTemplates) + `{}`
			payload := `{"schema_version":1,"generation":1,"revision":"sha256:` + strings.Repeat("0", 64) + `","workspace_templates":[` + templates + `],"contexts":[],"workspaces":[],"pending_candidates":[]}`
			writeAuthorityBytes(t, root, []byte(payload))
		},
	}
	for name, prepare := range tests {
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "final-authority")
			prepare(t, root)
			store, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := store.ReadComplete(context.Background()); err == nil {
				t.Fatal("unsafe, partial, or malformed final authority passed")
			}
		})
	}
}

func TestStoreRejectsSymlinkedRootAndAuthorityFile(t *testing.T) {
	valid := storeCollectionFixture(t)
	t.Run("root", func(t *testing.T) {
		parent := t.TempDir()
		realRoot := filepath.Join(parent, "real")
		materializeCollection(t, realRoot, valid)
		root := filepath.Join(parent, "linked")
		if err := os.Symlink(realRoot, root); err != nil {
			t.Fatal(err)
		}
		store, _ := New(root)
		if _, _, err := store.ReadComplete(context.Background()); err == nil {
			t.Fatal("symlinked root passed")
		}
	})
	t.Run("file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "final-authority")
		data := materializeCollection(t, root, valid)
		path := filepath.Join(root, authorityFileName)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(t.TempDir(), authorityFileName)
		if err := os.WriteFile(outside, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
		store, _ := New(root)
		if _, _, err := store.ReadComplete(context.Background()); err == nil {
			t.Fatal("symlinked authority file passed")
		}
	})
}

func TestStoreValidatesRootAndCancellationBeforeObservation(t *testing.T) {
	for _, root := range []string{"", "relative", filepath.Clean(string(filepath.Separator))} {
		if _, err := New(root); err == nil {
			t.Fatalf("unsafe root %q passed", root)
		}
	}
	root := filepath.Join(t.TempDir(), "final-authority")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := store.ReadComplete(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation=%v", err)
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled observation created final store: %v", err)
	}
}

func writeAuthorityBytes(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, authorityFileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
