package dockerruntime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFinalWorkspaceAuthorityRootIsConfigurationOnly(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "state", "authority")
	got, err := runtime.FinalWorkspaceAuthorityRoot()
	if err != nil || got != want {
		t.Fatalf("FinalWorkspaceAuthorityRoot() = %q, %v; want %q", got, err, want)
	}
	if _, err := os.Lstat(filepath.Join(root, "state")); !os.IsNotExist(err) {
		t.Fatalf("root resolution created state: %v", err)
	}
}

func TestFinalAuthorityLegacyGuardIsNonCreatingAndSeparatesReusedRoots(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err != nil {
		t.Fatalf("fresh guard error = %v", err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("fresh guard created state: before=%v after=%v", before, after)
	}

	if err := os.MkdirAll(runtime.aggregateRoot(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.aggregateRoot(), "final-projection"), []byte("final cluster projection placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err == nil {
		t.Fatal("reused cluster root was accepted before first final envelope")
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), true); err != nil {
		t.Fatalf("reviewed final reused root blocked an initialized final store: %v", err)
	}

	if err := os.MkdirAll(runtime.contextsDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err == nil {
		t.Fatal("Context source root was accepted before first final initialization")
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), true); err != nil {
		t.Fatalf("final-owned Context source root self-blocked after initialization: %v", err)
	}
}

func TestFinalAuthorityLegacyGuardRejectsUnsafeLegacyPresenceWithoutReadingIt(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(runtime.contextsDirectory()), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "foreign")
	if err := os.WriteFile(target, []byte("must-not-be-read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, runtime.contextsDirectory()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err == nil {
		t.Fatal("symlinked legacy authority was accepted")
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "must-not-be-read" {
		t.Fatalf("legacy target changed: %q, %v", contents, err)
	}
}

func TestFinalAuthorityLegacyGuardRejectsConfigOnlyResearchAuthorityBeforeInitialization(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	providerRoot := filepath.Join(runtime.configDirectory, "auth", "providers")
	if err := os.MkdirAll(providerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	provider := filepath.Join(providerRoot, "legacy.json")
	if err := os.WriteFile(provider, []byte("legacy-provider-must-not-be-adopted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err == nil {
		t.Fatal("config-only predecessor research authority was accepted before final initialization")
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), true); err != nil {
		t.Fatalf("reviewed provider configuration self-blocked after final initialization: %v", err)
	}
	contents, err := os.ReadFile(provider)
	if err != nil || string(contents) != "legacy-provider-must-not-be-adopted" {
		t.Fatalf("guard changed provider authority: %q, %v", contents, err)
	}
}

func TestFinalAuthorityLegacyGuardClosedInventoryAndLifetime(t *testing.T) {
	newRuntime := func(t *testing.T) *Runtime {
		t.Helper()
		root := t.TempDir()
		runtime, err := newRuntimeWithData(
			filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
		)
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}

	reference := newRuntime(t)
	inventory := reference.preReleaseLegacyAuthorityPaths()
	wantLegacy := []string{
		reference.rootsDirectory(),
		reference.instancesDirectory(),
		reference.statePath(),
		reference.projectJournalPath(),
		reference.clusterJournalPath(),
		filepath.Join(reference.configDirectory, "migrations"),
		filepath.Join(reference.stateDirectory, "migrations"),
	}
	wantFirst := []string{
		reference.contextsDirectory(),
		reference.aggregateRoot(),
		reference.principalRegistryDirectory(),
		filepath.Join(reference.stateDirectory, "auth"),
		filepath.Join(reference.stateDirectory, "auth", "projects"),
		filepath.Join(reference.configDirectory, "auth"),
		filepath.Join(reference.dataDirectory, "profiles"),
		reference.hostLoopbackDirectory(),
		reference.interactiveAttachmentDirectory(),
		filepath.Join(reference.stateDirectory, "service-exposure"),
	}
	if !reflect.DeepEqual(inventory.legacyOnly, wantLegacy) || !reflect.DeepEqual(inventory.firstInitializationOnly, wantFirst) {
		t.Fatalf("closed legacy inventory drifted: %#v", inventory)
	}

	for index := range inventory.legacyOnly {
		runtime := newRuntime(t)
		path := runtime.preReleaseLegacyAuthorityPaths().legacyOnly[index]
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, initialized := range []bool{false, true} {
			if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), initialized); err == nil {
				t.Fatalf("legacy-only root %q was accepted with finalInitialized=%t", path, initialized)
			}
		}
	}
	for index := range inventory.firstInitializationOnly {
		runtime := newRuntime(t)
		path := runtime.preReleaseLegacyAuthorityPaths().firstInitializationOnly[index]
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "must-not-be-opened"), []byte("{hostile predecessor bytes\n"), 0); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err == nil {
			t.Fatalf("reused root %q was accepted before final initialization", path)
		}
		if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), true); err != nil {
			t.Fatalf("reused final root %q self-blocked after initialization: %v", path, err)
		}
	}

	// Context desired source is final-owned after the first envelope. Its
	// strict source adapter, not the predecessor presence guard, owns later
	// schema validation.
	runtime := newRuntime(t)
	if err := os.MkdirAll(runtime.contextsDirectory(), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime.contextsDirectory(), "final-source-placeholder"), []byte("final Context source placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), true); err != nil {
		t.Fatalf("final Context source root was rejected after initialization: %v", err)
	}

	// WP03 is preserved across the clean cut and must never be mistaken for
	// predecessor Workspace authority.
	runtime = newRuntime(t)
	for _, path := range []string{
		runtime.runtimesDirectory(),
		filepath.Join(runtime.stateDirectory, "runtime"),
		runtime.runtimeLifecycleDirectory(),
	} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := runtime.ConfirmNoPreReleaseLegacyAuthority(context.Background(), false); err != nil {
		t.Fatalf("WP03 Runtime authority was rejected by clean-break inventory: %v", err)
	}
}
