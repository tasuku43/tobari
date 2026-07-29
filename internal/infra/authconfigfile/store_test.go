package authconfigfile

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
)

func validConfiguration(method authn.Method) authn.UserConfiguration {
	return authn.UserConfiguration{
		SchemaVersion: authn.UserConfigurationSchemaVersion,
		Method:        method,
		Parameters:    []authn.PublicParameter{{Name: "public_client_id", Value: "example-public-client"}},
	}
}

func TestCodecStrictlyDecodesVersionedBoundedFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/user-configuration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := Decode(bytes.NewReader(data))
	if err != nil || configuration.Method != authn.MethodOAuth2 || len(configuration.Parameters) != 2 {
		t.Fatalf("Decode() = %+v, %v", configuration, err)
	}
	for name, document := range map[string]string{
		"unknown field":  `{"schema_version":1,"method":"pat","parameters":[],"token":"forbidden"}`,
		"unknown schema": `{"schema_version":2,"method":"pat","parameters":[]}`,
		"trailing value": `{"schema_version":1,"method":"pat","parameters":[]} {}`,
		"missing list":   `{"schema_version":1,"method":"pat"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(strings.NewReader(document)); !errors.Is(err, ErrInvalidData) {
				t.Fatalf("Decode() error = %v", err)
			}
		})
	}
	oversized := bytes.Repeat([]byte("x"), authn.MaxUserConfigurationBytes+1)
	if _, err := Decode(bytes.NewReader(oversized)); !errors.Is(err, ErrInvalidData) {
		t.Fatalf("oversized Decode() error = %v", err)
	}
}

func TestStoreReplacesAndReportsConfiguration(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "auth.json")
	store := New(path)
	if status := store.Status(context.Background()); status.State != authn.ConfigurationStateMissing {
		t.Fatalf("missing status = %+v", status)
	}
	first := validConfiguration(authn.MethodPAT)
	if err := store.Save(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	firstInfo, err := os.Lstat(path)
	if err != nil || (runtime.GOOS != "windows" && firstInfo.Mode().Perm() != 0o600) || !firstInfo.Mode().IsRegular() {
		t.Fatalf("saved file = %+v, %v", firstInfo, err)
	}
	second := validConfiguration(authn.MethodOAuth2)
	if err := store.Save(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	secondInfo, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && os.SameFile(firstInfo, secondInfo) {
		t.Fatal("replacement reused the original file identity")
	}
	loaded, present, err := store.Load(context.Background())
	if err != nil || !present || !reflect.DeepEqual(loaded, second) {
		t.Fatalf("Load() = %+v, %t, %v", loaded, present, err)
	}
	status := store.Status(context.Background())
	if status.State != authn.ConfigurationStateValid || status.Method != authn.MethodOAuth2 || status.SchemaVersion != 1 {
		t.Fatalf("valid status = %+v", status)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary residue = %+v, %v", entries, err)
	}
}

func TestStoreTreatsMissingParentAsMissingWithoutCreatingIt(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "missing")
	store := New(filepath.Join(parent, "auth.json"))
	if configuration, present, err := store.Load(context.Background()); err != nil || present || !reflect.DeepEqual(configuration, authn.UserConfiguration{}) {
		t.Fatalf("Load missing parent = %+v, %t, %v", configuration, present, err)
	}
	if status := store.Status(context.Background()); status.State != authn.ConfigurationStateMissing {
		t.Fatalf("missing-parent status = %+v", status)
	}
	if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("Save missing parent error = %v", err)
	}
	if _, err := os.Lstat(parent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("store created default parent: %v", err)
	}
}

func TestStoreRejectsUnsafeAndCorruptFilesWithoutRepair(t *testing.T) {
	directory := t.TempDir()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(directory, "auth.json")
	if err := os.WriteFile(target, []byte(`{"schema_version":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := New(target)
	before, _ := os.ReadFile(target)
	if _, present, err := store.Load(context.Background()); !present || !errors.Is(err, ErrInvalidData) {
		t.Fatalf("corrupt Load() present=%t error=%v", present, err)
	}
	if status := store.Status(context.Background()); status.State != authn.ConfigurationStateInvalid || status.Problem != "invalid_data" {
		t.Fatalf("corrupt status = %+v", status)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("read-only status repaired corrupt state")
	}

	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(context.Background()); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("permissive file error = %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(directory, "missing"), target); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Load(context.Background()); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink error = %v", err)
	}
	if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("save over symlink error = %v", err)
	}
}

func TestStoreRejectsUnsafeParentAndNonRegularTarget(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Run("parent symlink", func(t *testing.T) {
			base := t.TempDir()
			realParent := filepath.Join(base, "real")
			if err := os.Mkdir(realParent, 0o700); err != nil {
				t.Fatal(err)
			}
			linkedParent := filepath.Join(base, "linked")
			if err := os.Symlink(realParent, linkedParent); err != nil {
				t.Fatal(err)
			}
			store := New(filepath.Join(linkedParent, "auth.json"))
			if _, _, err := store.Load(context.Background()); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("Load through parent symlink error = %v", err)
			}
			if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("Save through parent symlink error = %v", err)
			}
			if _, err := os.Lstat(filepath.Join(realParent, "auth.json")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe parent received target: %v", err)
			}
		})
	}

	t.Run("target directory", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(directory, "auth.json")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		store := New(target)
		if _, present, err := store.Load(context.Background()); !present || !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Load target directory present=%t error=%v", present, err)
		}
		if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Save over target directory error = %v", err)
		}
		info, err := os.Lstat(target)
		if err != nil || !info.IsDir() {
			t.Fatalf("target directory changed: info=%v error=%v", info, err)
		}
	})
}

func TestStoreRejectsPermissiveParentAndTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows ACLs are not represented by Unix mode bits")
	}

	t.Run("parent", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		store := New(filepath.Join(directory, "auth.json"))
		if _, _, err := store.Load(context.Background()); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Load permissive parent error = %v", err)
		}
		if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Save permissive parent error = %v", err)
		}
	})

	t.Run("target", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Chmod(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(directory, "auth.json")
		store := New(path)
		if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, present, err := store.Load(context.Background()); !present || !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Load permissive target present=%t error=%v", present, err)
		}
		if err := store.Save(context.Background(), validConfiguration(authn.MethodOAuth2)); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Save over permissive target error = %v", err)
		}
	})
}

func TestStoreParentAndTargetRevalidationRejectChangedObjects(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "configuration")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openVerifiedRoot(parent, parentInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	target := filepath.Join(parent, "auth.json")
	if err := os.WriteFile(target, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateReplaceTarget(root, "auth.json"); err != nil {
		t.Fatalf("valid target rejected: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateReplaceTarget(root, "auth.json"); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("changed target error = %v", err)
	}

	moved := filepath.Join(base, "moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := revalidateStoreParent(parent, parentInfo); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("changed parent error = %v", err)
	}
	if err := revalidateStoreTarget(target, parentInfo); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("mismatched target identity error = %v", err)
	}
}

func TestRootTemporaryCreationStaysWithOpenedDirectoryAfterPathSwap(t *testing.T) {
	base := t.TempDir()
	parent := filepath.Join(base, "configuration")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openVerifiedRoot(parent, parentInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	moved := filepath.Join(base, "moved")
	if err := os.Rename(parent, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	temporary, name, err := createRootTemporary(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	defer root.Remove(name)
	if _, err := os.Lstat(filepath.Join(moved, name)); err != nil {
		t.Fatalf("opened root did not receive temporary file: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement path unexpectedly received temporary file: %v", err)
	}
	if err := revalidateStoreParent(parent, parentInfo); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("path swap was not detected after staging: %v", err)
	}
}

func TestStoreTemporaryValidationRejectsChangedIdentityAndSize(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		t.Fatal(err)
	}
	root, err := openVerifiedRoot(parent, parentInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	temporary, err := os.CreateTemp(parent, ".auth-config-*")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(temporary.Name())
	defer root.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := temporary.Stat()
	if err != nil {
		t.Fatal(err)
	}
	contents := []byte("staged")
	if _, err := temporary.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := temporary.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateTemporary(root, name, expected, int64(len(contents))); err != nil {
		t.Fatalf("valid temporary rejected: %v", err)
	}
	if err := validateTemporary(root, name, expected, int64(len(contents)+1)); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("changed temporary size error = %v", err)
	}
	replacement, replacementName, err := createRootTemporary(root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Remove(replacementName)
	if _, err := replacement.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := root.Rename(replacementName, name); err != nil {
		t.Fatal(err)
	}
	if err := validateTemporary(root, name, expected, int64(len(contents))); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("changed temporary identity error = %v", err)
	}
}

func TestStoreRejectsDirectoryLikeAndRelativePaths(t *testing.T) {
	for _, path := range []string{string(filepath.Separator), t.TempDir() + string(filepath.Separator), "auth.json"} {
		store := New(path)
		if _, _, err := store.Load(context.Background()); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Load(%q) error = %v", path, err)
		}
		if err := store.Save(context.Background(), validConfiguration(authn.MethodPAT)); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("Save(%q) error = %v", path, err)
		}
	}
}
