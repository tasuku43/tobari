package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func TestRuntimePublicSurfaceUsesFinalAuthorityAndOpaqueNext(t *testing.T) {
	for _, path := range []string{"runtime list", "runtime show", "runtime create", "runtime history", "review runtimes", "runtime build", "runtime restore", "runtime delete", "runtime prune dry-run", "runtime prune apply"} {
		spec, found := DefaultCatalog().Lookup(path)
		if !found {
			t.Fatalf("Runtime Catalog path %q is absent", path)
		}
		encoded, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("marshal Runtime Catalog path %q: %v", path, err)
		}
		assertNoRetiredManifestPublicLanguage(t, "Catalog "+path, string(encoded))
	}

	manifest := readyRuntimeManifest()
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(&runtimeCatalogCLI{
		manifest:     manifest,
		list:         runtimeReviewList(manifest),
		buildCreates: true,
	})
	if code := command.RunContext(context.Background(), []string{"runtime", "build", "--id", manifest.ID}); code != ExitOK {
		t.Fatalf("runtime build code = %d, stderr = %q", code, stderr.String())
	}
	output := stdout.String()
	if !strings.Contains(output, invocationForPath("template list")) {
		t.Fatalf("runtime build Next did not use Template discovery: %q", output)
	}
	if !strings.Contains(output, tobari.RuntimeRevisionRef(manifest.ID, manifest.Revisions[0].Revision)) {
		t.Fatalf("runtime build omitted its opaque revision reference: %q", output)
	}
	for _, retired := range []string{"Workspace Manifest", "manifest show", "manifest runtime set", "--manifest"} {
		if strings.Contains(output, retired) {
			t.Fatalf("runtime build output retains retired language %q: %q", retired, output)
		}
	}
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "Next") && strings.Contains(line, manifest.Name+"@1") {
			t.Fatalf("runtime build Next reconstructed a name@ordinal selector: %q", line)
		}
	}
}

func TestRuntimeReviewRecoversFailedDraftWithoutInventingBuildHistory(t *testing.T) {
	manifest := testRuntimeManifest()
	recovery := tobari.RuntimeBuildRecovery{RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID), Name: manifest.Name, Kind: tobari.RuntimeBuildRecoveryFailed}
	fake := &runtimeCatalogCLI{manifest: manifest, list: runtimeReviewList(manifest), recovery: &recovery}
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader("\n"), &stdout, &stderr, DefaultCatalog(), nil)
	command.runtime = runtimecmd.New(fake)
	command.interactive = func(io.Reader, io.Writer, io.Writer) bool { return true }
	command.config = &terminalContextConfigurationWizard{style: false}

	if code := command.RunContext(context.Background(), []string{"review", "runtimes"}); code != ExitOK {
		t.Fatalf("failed draft recovery code = %d, stderr = %q", code, stderr.String())
	}
	if fake.recoveryReads != 1 || fake.recoveries != 1 || fake.buildCalls != 0 || fake.listCalls != 0 || fake.recoveredRef != recovery.RuntimeRef || fake.recoveredKind != recovery.Kind {
		t.Fatalf("failed draft recovery calls = reads:%d recoveries:%d builds:%d lists:%d ref:%q kind:%q", fake.recoveryReads, fake.recoveries, fake.buildCalls, fake.listCalls, fake.recoveredRef, fake.recoveredKind)
	}
	for _, want := range []string{"Runtime " + manifest.Name, "Status", "draft", "Recovery", "confirmed", invocationForPath("review runtimes")} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("failed draft recovery omitted %q: %q", want, stdout.String())
		}
	}
	for _, invented := range []string{"revision created", "unchanged · no revision created"} {
		if strings.Contains(stdout.String(), invented) {
			t.Errorf("failed draft recovery invented %q: %q", invented, stdout.String())
		}
	}
}

func TestRuntimeBuildFailureGuidanceUsesFinalRuntimeReconciliation(t *testing.T) {
	manifest := readyRuntimeManifest()
	for _, selection := range []tobari.RuntimeBuildSelectionState{
		tobari.RuntimeBuildSelectionUncertain,
		tobari.RuntimeBuildSelectionPromoted,
	} {
		t.Run(string(selection), func(t *testing.T) {
			var output bytes.Buffer
			diagnostics := newRuntimeBuildOutput(&output, false)
			diagnostics.Report(tobari.RuntimeBuildProgress{
				Stage:                 tobari.RuntimeBuildStageReport,
				Status:                tobari.RuntimeBuildProgressFailed,
				WorkspaceManifestName: "default",
				Dockerfile:            "/config/runtimes/frontend/Dockerfile",
				PreviousImage:         manifest.Revisions[0].Image,
				CandidateImage:        manifest.Revisions[0].Image,
				Selection:             selection,
			})
			diagnostics.WriteFailureSummary()
			text := output.String()
			if !strings.Contains(text, invocationForPath("review runtimes")) {
				t.Fatalf("failure guidance lacks Runtime reconciliation: %q", text)
			}
			for _, retired := range []string{"Workspace Manifest", "manifest show", "manifest runtime set", "--manifest"} {
				if strings.Contains(text, retired) {
					t.Fatalf("failure guidance retains retired language %q: %q", retired, text)
				}
			}
		})
	}
}

func TestRuntimeRecoveryWizardsUseFinalAuthorityVocabulary(t *testing.T) {
	manifest := readyRuntimeManifest()
	for _, test := range []struct {
		name   string
		invoke func(*CLI) (bool, error)
	}{
		{
			name: "build",
			invoke: func(command *CLI) (bool, error) {
				return confirmRuntimeBuildRecovery(context.Background(), command, tobari.RuntimeBuildRecovery{
					RuntimeID: manifest.ID, RuntimeRef: tobari.RuntimeRef(manifest.ID), Name: manifest.Name,
					Kind: tobari.RuntimeBuildRecoveryPublication,
				})
			},
		},
		{
			name: "delete",
			invoke: func(command *CLI) (bool, error) {
				return confirmRuntimeDeleteRecovery(context.Background(), command, tobari.RuntimeSummaryFrom(manifest))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := newCLI(strings.NewReader("\n"), &stdout, &stderr, DefaultCatalog(), nil)
			command.config = &terminalContextConfigurationWizard{style: false}
			confirmed, err := test.invoke(command)
			if err != nil || !confirmed {
				t.Fatalf("recovery review = %t/%v, stderr = %q", confirmed, err, stderr.String())
			}
			text := stderr.String()
			for _, retired := range []string{"Workspace Manifest", "manifest show", "manifest runtime set", "--manifest"} {
				if strings.Contains(text, retired) {
					t.Fatalf("recovery review retains retired language %q: %q", retired, text)
				}
			}
			if !strings.Contains(text, "Workspace Template") || !strings.Contains(text, "Context") {
				t.Fatalf("recovery review omitted final authority wording: %q", text)
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
