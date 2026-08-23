package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/runtimecmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestRuntimeLifecycleCommandsPublishOnlySemanticRevisionProjection(t *testing.T) {
	manifest := readyRuntimeManifest()
	commands := [][]string{
		{"runtime", "list", "--format=json"},
		{"review", "runtimes", "--format=json"},
		{"runtime", "show", "--name", manifest.Name, "--format=json"},
		{"runtime", "history", "--name", manifest.Name, "--format=json"},
		{"runtime", "build", "--id", manifest.ID, "--format=json"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args[:2], "_"), func(t *testing.T) {
			fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest), lifecycleAvailability: tobari.RuntimeAvailabilityPruned}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			command.tobari = nil
			if code := command.RunContext(context.Background(), args); code != ExitOK {
				t.Fatalf("%v code/stderr = %d/%q", args, code, stderr.String())
			}
			var document any
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			assertNoRuntimeInfrastructureProjectionKeys(t, document)
			if !strings.Contains(stdout.String(), `"source_digest"`) || !strings.Contains(stdout.String(), `"availability":{"state":"pruned"}`) {
				t.Fatalf("%v omitted semantic head evidence: %s", args, stdout.String())
			}
		})
	}
}

func TestRuntimeLifecycleHumanOutputNeverLeaksInfrastructureIdentity(t *testing.T) {
	manifest := readyRuntimeManifest()
	commands := [][]string{
		{"runtime", "list"},
		{"review", "runtimes"},
		{"runtime", "show", "--name", manifest.Name},
		{"runtime", "history", "--name", manifest.Name},
		{"runtime", "build", "--id", manifest.ID},
	}
	for _, args := range commands {
		t.Run(strings.Join(args[:2], "_"), func(t *testing.T) {
			fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest), lifecycleAvailability: tobari.RuntimeAvailabilityPruned}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			command.tobari = nil
			if code := command.RunContext(context.Background(), args); code != ExitOK {
				t.Fatalf("%v code/stderr = %d/%q", args, code, stderr.String())
			}
			for _, forbidden := range []string{
				manifest.Revisions[0].Image,
				manifest.Revisions[0].ImageDigest,
				manifest.Revisions[0].SnapshotPath,
			} {
				if forbidden != "" && strings.Contains(stdout.String(), forbidden) {
					t.Fatalf("%v leaked Runtime infrastructure value %q: %s", args, forbidden, stdout.String())
				}
			}
			if !strings.Contains(stdout.String(), manifest.Revisions[0].Revision) {
				t.Fatalf("%v omitted semantic source digest: %s", args, stdout.String())
			}
		})
	}
}

func TestRuntimeListAndRedirectedReviewKeepReadySeparateFromPrunedAvailability(t *testing.T) {
	manifest := readyRuntimeManifest()
	for _, args := range [][]string{{"runtime", "list", "--format=json"}, {"review", "runtimes", "--format=json"}} {
		fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest), lifecycleAvailability: tobari.RuntimeAvailabilityPruned}
		var stdout, stderr bytes.Buffer
		command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
		command.runtime = runtimecmd.New(fake)
		if code := command.RunContext(context.Background(), args); code != ExitOK {
			t.Fatalf("%v code/stderr = %d/%q", args, code, stderr.String())
		}
		var document struct {
			Runtimes struct {
				Items []map[string]any `json:"items"`
			} `json:"runtimes"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Runtimes.Items) != 2 {
			t.Fatalf("%v items = %+v", args, document.Runtimes.Items)
		}
		standard, managed := document.Runtimes.Items[0], document.Runtimes.Items[1]
		if standard["storage"] != nil || standard["ready"] != true || nestedState(standard["availability"]) != "unknown" {
			t.Fatalf("%v standard = %+v", args, standard)
		}
		if managed["ready"] != true || nestedState(managed["availability"]) != "pruned" {
			t.Fatalf("%v managed = %+v", args, managed)
		}
		wantKeys := []string{"availability", "head", "id", "kind", "last_used", "name", "ready", "revision_ref", "runtime_ref", "snapshot", "source_digest", "source_path", "storage"}
		gotKeys := mapKeys(managed)
		if !reflect.DeepEqual(gotKeys, wantKeys) {
			t.Fatalf("%v managed keys = %v, want %v", args, gotKeys, wantKeys)
		}
	}
}

func TestRuntimeReportRevisionExactKeysAndStandardNullableStorage(t *testing.T) {
	manifest := readyRuntimeManifest()
	for _, test := range []struct {
		name        string
		manifest    tobari.RuntimeManifest
		list        []tobari.RuntimeSummary
		wantStorage bool
	}{
		{name: "managed", manifest: manifest, list: runtimeReviewList(manifest), wantStorage: true},
		{name: "standard", manifest: manifest, list: runtimeReviewList(manifest), wantStorage: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := test.manifest.Name
			if test.name == "standard" {
				name = tobari.StandardRuntimeName
			}
			fake := &runtimeCatalogCLI{manifest: test.manifest, list: test.list}
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
			command.runtime = runtimecmd.New(fake)
			if code := command.RunContext(context.Background(), []string{"runtime", "show", "--name", name, "--format=json"}); code != ExitOK {
				t.Fatalf("code/stderr = %d/%q", code, stderr.String())
			}
			var document struct {
				Runtime struct {
					Runtime struct {
						Revisions []map[string]any `json:"revisions"`
					} `json:"runtime"`
				} `json:"runtime"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			revision := document.Runtime.Runtime.Revisions[0]
			wantKeys := []string{"availability", "created_at", "last_used", "ordinal", "runtime_ref", "snapshot", "source_digest", "storage"}
			if test.name == "managed" {
				wantKeys = append(wantKeys, "revision_ref")
				sort.Strings(wantKeys)
			}
			if got := mapKeys(revision); !reflect.DeepEqual(got, wantKeys) {
				t.Fatalf("revision keys = %v, want %v", got, wantKeys)
			}
			if (revision["storage"] != nil) != test.wantStorage {
				t.Fatalf("storage = %#v, want object=%t", revision["storage"], test.wantStorage)
			}
		})
	}
}

func TestRuntimeCatalogNeverDeclaresInfrastructureOrLegacyRevisionFields(t *testing.T) {
	for _, path := range []string{"runtime list", "runtime show", "runtime history", "runtime build", "review runtimes"} {
		spec, ok := DefaultCatalog().lookupRegistered(path)
		if !ok {
			t.Fatal(path)
		}
		for _, forbidden := range []string{"image", "image_digest", "snapshot_path", "revision"} {
			if outputDeclaresField(spec.Agent.Output.Fields, forbidden) {
				t.Fatalf("%s declares forbidden public field %q", path, forbidden)
			}
		}
	}
}

func TestConfirmedRuntimeProjectionFaultMatchesBuildAndReviewCatalog(t *testing.T) {
	manifest := readyRuntimeManifest()
	intent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite,
		Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: manifest.ID},
		Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	publicationRecovery := tobari.RuntimeBuildRecovery{RuntimeID: manifest.ID, RuntimeRef: manifest.ID, Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryPublication}
	cleanupRecovery := tobari.RuntimeBuildRecovery{RuntimeID: manifest.ID, RuntimeRef: manifest.ID, Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryCleanup}
	drifted := manifest
	drifted.Revisions = append([]tobari.RuntimeRevision{}, manifest.Revisions...)
	drifted.Revisions[0].ImageDigest = "sha256:" + strings.Repeat("e", 64)
	tests := []struct {
		name     string
		path     string
		fake     *runtimeCatalogCLI
		wantCode string
		invoke   func(*runtimecmd.Service) error
	}{
		{name: "build canceled observation", path: "runtime build", fake: &runtimeCatalogCLI{manifest: manifest, buildCreates: true, lifecycleErr: context.Canceled}, wantCode: "runtime_build_observation_unknown", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Build(context.Background(), intent, manifest.ID, nil)
			return err
		}},
		{name: "review canceled observation", path: "review runtimes", fake: &runtimeCatalogCLI{manifest: manifest, lifecycleErr: context.Canceled}, wantCode: "runtime_build_observation_unknown", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Recover(context.Background(), intent, publicationRecovery)
			return err
		}},
		{name: "review cleanup canceled observation", path: "review runtimes", fake: &runtimeCatalogCLI{manifest: manifest, lifecycleErr: context.Canceled}, wantCode: "runtime_build_observation_unknown", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Recover(context.Background(), intent, cleanupRecovery)
			return err
		}},
		{name: "build authority drift", path: "runtime build", fake: &runtimeCatalogCLI{manifest: manifest, buildCreates: true, snapshotManifest: &drifted}, wantCode: "invalid_runtime_report_confirmed", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Build(context.Background(), intent, manifest.ID, nil)
			return err
		}},
		{name: "review authority drift", path: "review runtimes", fake: &runtimeCatalogCLI{manifest: manifest, snapshotManifest: &drifted}, wantCode: "invalid_runtime_report_confirmed", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Recover(context.Background(), intent, publicationRecovery)
			return err
		}},
		{name: "review cleanup authority drift", path: "review runtimes", fake: &runtimeCatalogCLI{manifest: manifest, snapshotManifest: &drifted}, wantCode: "invalid_runtime_report_confirmed", invoke: func(service *runtimecmd.Service) error {
			_, err := service.Recover(context.Background(), intent, cleanupRecovery)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.invoke(runtimecmd.New(test.fake))
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.wantCode || public.Phase != fault.PhaseVerification || public.ChangeState != fault.ChangeConfirmed {
				t.Fatalf("PublicCopy = %+v/%v", public, err)
			}
			spec, found := DefaultCatalog().Lookup(test.path)
			if !found {
				t.Fatal(test.path)
			}
			declared := commandErrorByCode(t, spec.Agent.Errors, public.Code)
			if declared.Kind != public.Kind || declared.Phase != public.Phase || declared.ChangeState != public.ChangeState ||
				declared.Retryable != public.Retryable || !reflect.DeepEqual(declared.NextActions, public.NextActions) {
				t.Fatalf("Catalog=%+v PublicCopy=%+v", declared, public)
			}
		})
	}
}

func assertNoRuntimeInfrastructureProjectionKeys(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{"image": true, "image_digest": true, "snapshot_path": true, "revision": true}
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if forbidden[key] {
					t.Fatalf("public Runtime JSON contains forbidden key %q", key)
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
}

func nestedState(value any) string {
	object, _ := value.(map[string]any)
	state, _ := object["state"].(string)
	return state
}

func mapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func outputDeclaresField(fields []OutputField, target string) bool {
	for _, field := range fields {
		if field.Name == target || outputDeclaresField(field.Fields, target) {
			return true
		}
		if field.Items != nil && outputDeclaresField(field.Items.Fields, target) {
			return true
		}
	}
	return false
}
