package workspaceauthoritysource

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const sourceTemplateID tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789a1"
const sourceContextID tobari.ContextID = "01912345-6789-7abc-8def-0123456789a2"
const otherTemplateID tobari.WorkspaceTemplateID = "01912345-6789-7abc-8def-0123456789b1"

func sourceTemplateFixture(t *testing.T) tobari.WorkspaceTemplateSource {
	t.Helper()
	body := tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "public_https", Authorities: []tobari.ManifestPolicyAuthority{}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile:      tobari.DefaultProfile,
			NativeReadiness:   tobari.ManifestNativeReadinessEnabled,
			BaselineGrants:    []tobari.ManifestPolicyExactRule{},
			BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{},
			MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{},
			BaselineDenies:    []tobari.ManifestPolicyExactRule{},
			GraphQLEndpoints:  []tobari.ManifestPolicyExactRule{},
			MCPEndpoints:      []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{
			RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName,
			Revision: "sha256:" + strings.Repeat("f", 64), Ordinal: 1, Image: "tobari-runtime:test",
		}},
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(sourceTemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: sourceTemplateID, Name: "tools", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision}}
	source, err := tobari.NewWorkspaceTemplateSource(template)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func sourceRuntimeBindingFixture() tobari.RuntimeBinding {
	return tobari.RuntimeBinding{RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: "sha256:" + strings.Repeat("f", 64), Ordinal: 1, Image: "tobari-runtime:test"}
}

func sourceMigrationCollectionFixture(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	source := sourceTemplateFixture(t)
	body, err := source.Body(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(sourceTemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: sourceTemplateID, Name: source.Template.Name, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision}}
	if err := tobari.InitializeWorkspaceTemplateMetadata(&template); err != nil {
		t.Fatal(err)
	}
	binding := tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: sourceContextID, ProjectRoot: "/workspace/example", TemplateID: sourceTemplateID}
	memory, _, err := tobari.PublishPolicyMemory(sourceContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	collection, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{{Context: binding, PolicyMemory: memory}}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func TestPublishTemplateCreatesAbsentXDGConfigParent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent-xdg-config", "tobari")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Dir(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("XDG config parent unexpectedly exists before publication: %v", err)
	}

	want := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), want); err != nil {
		t.Fatalf("publish into absent XDG config parent: %v", err)
	}
	parentInfo, err := os.Lstat(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateDirectory(parentInfo); err != nil {
		t.Fatalf("created XDG config parent is not private: %v", err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePrivateDirectory(info); err != nil {
		t.Fatalf("created source root is not private: %v", err)
	}
	got, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil || !present {
		t.Fatalf("read published Template: present=%t err=%v", present, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("published Template mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func writeAlphaTemplateSourceFixture(t *testing.T, store *Store) tobari.WorkspaceTemplate {
	t.Helper()
	v1 := sourceTemplateFixture(t)
	finalBody, err := v1.Body(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	alphaPolicy := tobari.WorkspaceTemplatePolicyBody{
		AgentProfile: tobari.DefaultProfile, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
		BaselineGrants: []tobari.ManifestPolicyExactRule{}, BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{},
		MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{}, BaselineDenies: []tobari.ManifestPolicyExactRule{},
		GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
	}
	alphaBody := finalBody
	alphaBody.Policy = alphaPolicy
	revision, err := tobari.NewWorkspaceTemplateRevision(sourceTemplateID, 1, alphaBody)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: sourceTemplateID, Name: "tools", Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision}}
	if err := tobari.InitializeWorkspaceTemplateMetadata(&template); err != nil {
		t.Fatal(err)
	}
	document := v1.Template
	document.BaseRevision = &revision.Revision
	alpha := tobari.WorkspaceTemplatePolicyAlphaSourceDocument{
		SchemaVersion: tobari.WorkspaceTemplatePolicyAlphaSchemaVersion,
		TemplateID:    sourceTemplateID,
		Boundary: tobari.WorkspaceTemplatePolicyAlphaBoundarySource{
			DestinationCeiling: alphaBody.Boundary.DestinationCeiling,
			MethodPolicy:       alphaBody.Boundary.MethodPolicy,
		},
		Semantic: alphaPolicy,
	}
	if err := store.PublishTemplate(context.Background(), v1); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	templateData, _ := encodeCanonicalYAML(document)
	policyData, _ := encodeCanonicalYAML(alpha)
	if err := os.WriteFile(path, templateData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), policyFileName), policyData, 0o600); err != nil {
		t.Fatal(err)
	}
	return template
}

func TestExplicitTemplatePolicyMigrationIsLosslessAndNonActivating(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	template := writeAlphaTemplateSourceFixture(t, store)
	if _, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); !present || !errors.Is(err, tobari.ErrResourceSourceInvalid) {
		t.Fatalf("ordinary V1 read accepted alpha: present=%t err=%v", present, err)
	}
	alpha, migrated, sourceFingerprint, targetFingerprint, present, err := store.ReadTemplatePolicyMigrationSnapshot(context.Background(), sourceTemplateID, sourceRuntimeBindingFixture())
	if err != nil || !present {
		t.Fatalf("migration snapshot: present=%t err=%v", present, err)
	}
	plan, err := tobari.NewWorkspaceTemplatePolicyMigrationPlan(template, alpha, migrated, sourceFingerprint, targetFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	got, changed, err := store.ApplyTemplatePolicyMigration(context.Background(), plan, sourceRuntimeBindingFixture())
	if err != nil || !changed || got != targetFingerprint {
		t.Fatalf("migration apply = %q/%t/%v", got, changed, err)
	}
	current, fingerprint, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil || !present || fingerprint != targetFingerprint || current.Policy.SchemaVersion != tobari.WorkspaceTemplatePolicySchemaVersion {
		t.Fatalf("V1 read = %+v/%q/%t/%v", current, fingerprint, present, err)
	}
	if current.Template.BaseRevision == nil || *current.Template.BaseRevision != template.Current.Revision {
		t.Fatal("source migration changed the active/base revision binding")
	}
	if replay, changed, err := store.ApplyTemplatePolicyMigration(context.Background(), plan, sourceRuntimeBindingFixture()); err != nil || changed || replay != targetFingerprint {
		t.Fatalf("same-plan replay = %q/%t/%v", replay, changed, err)
	}
}

func TestExplicitTemplatePolicyMigrationRecoversPublicationBoundaries(t *testing.T) {
	for _, boundary := range []string{
		"template_base_repair_journal_temp_written:prepared",
		"template_base_repair_journal_temp_synced:prepared",
		"template_base_repair_journal_renamed:prepared",
		"template_base_repair_journal_parent_synced:prepared",
		"template_base_repair_before_discard_rename",
		"template_base_repair_discard_renamed",
		"template_base_repair_discard_sync",
		"template_base_repair_before_publish_rename",
		"template_base_repair_published_renamed",
		"template_base_repair_publish_sync",
		"template_base_repair_quarantine_removed",
		"template_base_repair_discard_removed",
		"template_base_repair_cleanup_sync",
		"template_base_repair_journal_removed",
	} {
		t.Run(boundary, func(t *testing.T) {
			store, err := New(filepath.Join(t.TempDir(), "config"))
			if err != nil {
				t.Fatal(err)
			}
			template := writeAlphaTemplateSourceFixture(t, store)
			alpha, migrated, sourceFingerprint, targetFingerprint, _, err := store.ReadTemplatePolicyMigrationSnapshot(context.Background(), sourceTemplateID, sourceRuntimeBindingFixture())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := tobari.NewWorkspaceTemplatePolicyMigrationPlan(template, alpha, migrated, sourceFingerprint, targetFingerprint)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected migration interruption")
			fired := false
			store.phase = func(observed string) error {
				if !fired && observed == boundary {
					fired = true
					return injected
				}
				return nil
			}
			if _, _, err := store.ApplyTemplatePolicyMigration(context.Background(), plan, sourceRuntimeBindingFixture()); !errors.Is(err, injected) {
				t.Fatalf("boundary %q was not observed: %v", boundary, err)
			}
			store.phase = func(string) error { return nil }
			fingerprint, _, err := store.ApplyTemplatePolicyMigration(context.Background(), plan, sourceRuntimeBindingFixture())
			if err != nil || fingerprint != targetFingerprint {
				t.Fatalf("same-plan recovery = %q/%v", fingerprint, err)
			}
			if _, observed, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); err != nil || !present || observed != targetFingerprint {
				t.Fatalf("recovered V1 source = %q/%t/%v", observed, present, err)
			}
		})
	}
}

func TestTemplateSourceRuntimeReferenceExposesOnlyStableIDAndRevision(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	source := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, forbidden := range []string{"runtime_id:", "name: standard", "ordinal:", "image:"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("template.yaml exposed internal Runtime field %q:\n%s", forbidden, text)
		}
	}
	for _, required := range []string{"runtime:", "id: builtin/standard", "revision: sha256:"} {
		if !strings.Contains(text, required) {
			t.Fatalf("template.yaml lacks Runtime source field %q:\n%s", required, text)
		}
	}
	data = bytes.Replace(data, []byte("    revision:"), []byte("    name: standard\n    revision:"), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); !errors.Is(err, tobari.ErrResourceSourceInvalid) {
		t.Fatalf("editable Runtime presentation metadata was accepted: %v", err)
	}
}

func TestTemplatePolicyV1SchemaRoundTripsExactly(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	want := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	policyPath := filepath.Join(filepath.Dir(path), policyFileName)
	data, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "schema: tobari.dev/template-policy/v1\n") {
		t.Fatalf("policy.yaml lacks exact final schema token:\n%s", data)
	}
	if strings.Contains(string(data), "schema_version:") || strings.Contains(string(data), "tobari.dev/template-policy/v1alpha1\n") {
		t.Fatalf("policy.yaml claims a numeric or transitional schema:\n%s", data)
	}
	for _, required := range []string{"boundary:\n", "methods:\n", "deny: []", "semantic:\n", "protocols:\n", "http:\n", "generic:\n", "graphql:\n", "mcp:\n", "git:\n", "oci:\n", "providers:\n", "aws:\n", "kubernetes:\n"} {
		if !strings.Contains(string(data), required) {
			t.Fatalf("policy.yaml lacks V1 topology %q:\n%s", required, data)
		}
	}
	got, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil || !present {
		t.Fatalf("read round trip: present=%t err=%v", present, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("source round trip changed content:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestTemplatePolicyV1RequiresBoundaryAndPresentModuleTopology(t *testing.T) {
	valid := []byte("boundary:\n  methods:\n    deny: []\nsemantic:\n  protocols:\n    http: {}\n  providers: {}\n")
	if err := validateFinalPolicySourceTopology(valid); err != nil {
		t.Fatalf("absent known-none modules were rejected: %v", err)
	}
	invalid := map[string][]byte{
		"missing boundary":            []byte("semantic:\n  protocols:\n    http: {}\n  providers: {}\n"),
		"missing deny":                []byte("boundary:\n  methods: {}\nsemantic:\n  protocols:\n    http: {}\n  providers: {}\n"),
		"null deny":                   []byte("boundary:\n  methods:\n    deny: null\nsemantic:\n  protocols:\n    http: {}\n  providers: {}\n"),
		"present module missing deny": []byte("boundary:\n  methods:\n    deny: []\nsemantic:\n  protocols:\n    http:\n      generic:\n        allow:\n          rules: []\n  providers: {}\n"),
		"null module":                 []byte("boundary:\n  methods:\n    deny: []\nsemantic:\n  protocols:\n    http:\n      oci: null\n  providers: {}\n"),
	}
	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateFinalPolicySourceTopology(source); err == nil {
				t.Fatal("invalid final policy topology was accepted")
			}
		})
	}
}

func TestTemplatePolicyRejectsNumericUnknownAndTransitionalSchema(t *testing.T) {
	for name, schemaLine := range map[string]string{
		"numeric predecessor": "schema_version: 1",
		"unknown":             "schema: tobari.dev/template-policy/v2",
		"transitional alpha":  "schema: tobari.dev/template-policy/v1alpha1",
	} {
		t.Run(name, func(t *testing.T) {
			store, err := New(filepath.Join(t.TempDir(), "config"))
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PublishTemplate(context.Background(), sourceTemplateFixture(t)); err != nil {
				t.Fatal(err)
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			policyPath := filepath.Join(filepath.Dir(path), policyFileName)
			data, err := os.ReadFile(policyPath)
			if err != nil {
				t.Fatal(err)
			}
			data = bytes.Replace(data, []byte("schema: tobari.dev/template-policy/v1"), []byte(schemaLine), 1)
			if err := os.WriteFile(policyPath, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); !present || !errors.Is(err, tobari.ErrResourceSourceInvalid) {
				t.Fatalf("schema accepted: present=%t err=%v", present, err)
			}
		})
	}
}

func TestStorePublishesConceptSeparatedClosedSources(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	source := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	contextSource := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: sourceContextID, ProjectRoot: "/workspace/example", TemplateID: sourceTemplateID}
	if err := store.PublishContext(context.Background(), contextSource); err != nil {
		t.Fatal(err)
	}
	templatePath, _ := store.TemplatePath(sourceTemplateID)
	contextPath, _ := store.ContextPath(sourceContextID)
	for _, path := range []string{templatePath, filepath.Join(filepath.Dir(templatePath), policyFileName), contextPath} {
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			t.Fatalf("source %q = %v/%v", path, info, statErr)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(templatePath))
	if err != nil || len(entries) != 2 {
		t.Fatalf("Template source set = %v/%v", entries, err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".rego") {
			t.Fatalf("Rego escaped into source set: %q", entry.Name())
		}
	}
	observed, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil || !present || observed.Template.Name != "tools" {
		t.Fatalf("read Template = %+v/%v/%v", observed, present, err)
	}
}

func TestTemplatePublicationNeverExposesPartialSourcePair(t *testing.T) {
	phases := []string{"source_file_written:policy.yaml", "source_file_written:template.yaml", "source_stage_durable", "source_before_publish", "source_directory_published"}
	for _, failedPhase := range phases {
		t.Run(failedPhase, func(t *testing.T) {
			store, err := New(filepath.Join(t.TempDir(), "config"))
			if err != nil {
				t.Fatal(err)
			}
			store.phase = func(phase string) error {
				if phase == failedPhase {
					return errors.New("injected source publication failure")
				}
				return nil
			}
			if err := store.PublishTemplate(context.Background(), sourceTemplateFixture(t)); err == nil {
				t.Fatal("injected publication failure was ignored")
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			if _, err := os.Lstat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("partial Template source became visible: %v", err)
			}
			entries, err := os.ReadDir(filepath.Dir(filepath.Dir(path)))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".source-stage-") {
					t.Fatalf("source staging directory remains: %q", entry.Name())
				}
			}
		})
	}
}

func TestTemplatePublicationPreservesExistingDesiredFiles(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	source := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	edited := []byte("# desired user edit\n")
	original, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(edited, original...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishTemplate(context.Background(), source); !errors.Is(err, tobari.ErrResourceSourceChanged) {
		t.Fatalf("replacement of desired source = %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, append(edited, original...)) {
		t.Fatal("existing desired source was overwritten")
	}
}

func TestMigrationSourcePreparationAndCommitFailuresHaveNoCanonicalMutation(t *testing.T) {
	for _, failedPhase := range []string{"migration_template_staged:" + string(sourceTemplateID), "migration_context_staged:" + string(sourceContextID), "migration_source_committed:templates", "migration_source_committed:contexts"} {
		t.Run(failedPhase, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "config")
			store, _ := New(root)
			store.phase = func(phase string) error {
				if phase == failedPhase {
					return errors.New("injected migration source failure")
				}
				return nil
			}
			stage, err := store.PrepareInstallationMigrationSources(context.Background(), sourceMigrationCollectionFixture(t))
			if strings.HasPrefix(failedPhase, "migration_source_committed:") {
				if err != nil {
					t.Fatal(err)
				}
				err = stage.Commit(context.Background())
			}
			if err == nil {
				t.Fatal("injected migration source failure was ignored")
			}
			for _, concept := range []string{"templates", "contexts"} {
				if _, err := os.Lstat(filepath.Join(root, concept)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("canonical %s source became visible: %v", concept, err)
				}
			}
			if stage != nil {
				_ = stage.Abort(context.Background())
			}
			if _, err := os.Lstat(filepath.Join(root, migrationSourceStageName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("migration source stage remains: %v", err)
			}
		})
	}
}

func TestMigrationSourceVerificationBindsExactPublishedBytes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "config")
	store, _ := New(root)
	stage, err := store.PrepareInstallationMigrationSources(context.Background(), sourceMigrationCollectionFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append([]byte("# byte-only drift\n"), data...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stage.Verify(context.Background()); !errors.Is(err, tobari.ErrResourceSourceInvalid) {
		t.Fatalf("byte-drift migration Verify = %v", err)
	}
}

func TestMigrationSourceRenameCrashRecoveryRollsBackAndSamePlanRetries(t *testing.T) {
	boundaries := []string{
		"migration_source_rename_prepared:templates",
		"migration_source_renamed:templates",
		"migration_source_synced:templates",
		"migration_source_committed:templates",
		"migration_source_rename_prepared:contexts",
		"migration_source_renamed:contexts",
		"migration_source_synced:contexts",
		"migration_source_committed:contexts",
	}
	for _, boundary := range boundaries {
		t.Run(boundary, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "config")
			store, _ := New(root)
			collection := sourceMigrationCollectionFixture(t)
			stage, err := store.PrepareInstallationMigrationSources(context.Background(), collection)
			if err != nil {
				t.Fatal(err)
			}
			store.phase = func(observed string) error {
				if observed == boundary {
					panic("synthetic process crash")
				}
				return nil
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("crash boundary was not reached")
					}
				}()
				_ = stage.Commit(context.Background())
			}()

			reopenedStore, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := reopenedStore.PrepareInstallationMigrationSources(context.Background(), collection, true)
			if err != nil {
				t.Fatalf("reopen after %s: %v", boundary, err)
			}
			if err := reopened.Rollback(context.Background()); err != nil {
				t.Fatalf("rollback after %s: %v", boundary, err)
			}
			if err := reopened.Abort(context.Background()); err != nil {
				t.Fatalf("abort after %s: %v", boundary, err)
			}
			for _, concept := range []string{"templates", "contexts"} {
				if _, err := os.Lstat(filepath.Join(root, concept)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("rollback after %s left canonical %s: %v", boundary, concept, err)
				}
			}
			retry, err := reopenedStore.PrepareInstallationMigrationSources(context.Background(), collection)
			if err != nil {
				t.Fatalf("same-plan reprepare after %s: %v", boundary, err)
			}
			if err := retry.Commit(context.Background()); err != nil {
				t.Fatalf("same-plan recommit after %s: %v", boundary, err)
			}
			if err := retry.Verify(context.Background()); err != nil {
				t.Fatalf("same-plan verify after %s: %v", boundary, err)
			}
			if err := retry.Complete(context.Background()); err != nil {
				t.Fatalf("same-plan completion after %s: %v", boundary, err)
			}
		})
	}
}

func TestMigrationSourceAcceptedCleanupCrashSettlesFromCanonicalBytes(t *testing.T) {
	for _, boundary := range []string{"migration_source_cleanup_prepared", "migration_source_cleanup_removed"} {
		t.Run(boundary, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "config")
			store, _ := New(root)
			collection := sourceMigrationCollectionFixture(t)
			stage, err := store.PrepareInstallationMigrationSources(context.Background(), collection)
			if err != nil {
				t.Fatal(err)
			}
			if err := stage.Commit(context.Background()); err != nil {
				t.Fatal(err)
			}
			store.phase = func(observed string) error {
				if observed == boundary {
					panic("synthetic process crash")
				}
				return nil
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Fatal("cleanup crash boundary was not reached")
					}
				}()
				_ = stage.Complete(context.Background())
			}()

			reopenedStore, err := New(root)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := reopenedStore.PrepareInstallationMigrationSources(context.Background(), collection, true)
			if err != nil {
				t.Fatalf("reopen accepted cleanup after %s: %v", boundary, err)
			}
			if err := reopened.Verify(context.Background()); err != nil {
				t.Fatalf("verify accepted cleanup after %s: %v", boundary, err)
			}
			if err := reopened.Complete(context.Background()); err != nil {
				t.Fatalf("settle accepted cleanup after %s: %v", boundary, err)
			}
			if _, err := os.Lstat(filepath.Join(root, migrationSourceStageName)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("accepted cleanup after %s retained stage: %v", boundary, err)
			}
		})
	}
}

func TestResourceDirectoryDeletionResumesAfterQuarantineInterruption(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	if err := store.PublishTemplate(context.Background(), sourceTemplateFixture(t)); err != nil {
		t.Fatal(err)
	}
	store.phase = func(phase string) error {
		if phase == "source_delete_quarantined" {
			return errors.New("injected deletion interruption")
		}
		return nil
	}
	if err := store.DeleteTemplate(context.Background(), sourceTemplateID); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("interrupted delete = %v", err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	if _, err := os.Lstat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("quarantined source remains selectable: %v", err)
	}
	store.phase = func(string) error { return nil }
	if err := store.DeleteTemplate(context.Background(), sourceTemplateID); err != nil {
		t.Fatalf("resume quarantined delete: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(store.configRoot, ".source-delete-quarantine")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("delete quarantine remains: %v", err)
	}
}

func TestStoreTreatsMissingPairMemberAsMissingAndUnknownEntryAsInvalid(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	source := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	if err := os.Remove(filepath.Join(filepath.Dir(path), policyFileName)); err != nil {
		t.Fatal(err)
	}
	if _, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); err != nil || present {
		t.Fatalf("partial pair = present %v, err %v", present, err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "notes.txt"), []byte("untrusted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); err == nil {
		t.Fatal("unknown Template child accepted")
	}
}

func TestStoreRejectsHostileYAMLAndIdentityMismatch(t *testing.T) {
	for name, mutation := range map[string]string{
		"unknown":   "unknown_field: true\n",
		"duplicate": "schema: tobari.dev/template/v1\n",
		"alias":     "alias: &x value\ncopy: *x\n",
	} {
		t.Run(name, func(t *testing.T) {
			store, _ := New(filepath.Join(t.TempDir(), "config"))
			source := sourceTemplateFixture(t)
			if err := store.PublishTemplate(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			data, _ := os.ReadFile(path)
			if err := os.WriteFile(path, append(data, []byte(mutation)...), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, present, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID); !present || !errors.Is(err, tobari.ErrResourceSourceInvalid) {
				t.Fatalf("hostile YAML = present %v, err %v", present, err)
			}
		})
	}
}

func TestAdvanceTemplateBasePreservesCommentsAndRejectsStalePair(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	source := sourceTemplateFixture(t)
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	data, _ := os.ReadFile(path)
	data = append([]byte("# edited by an agent\n"), data...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, fingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	next := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	policyPath := filepath.Join(filepath.Dir(path), policyFileName)
	policyBefore, _ := os.ReadFile(policyPath)
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); err != nil {
		t.Fatal(err)
	}
	templateAfter, _ := os.ReadFile(path)
	policyAfter, _ := os.ReadFile(policyPath)
	if !strings.Contains(string(templateAfter), "# edited by an agent") || !strings.Contains(string(templateAfter), string(next)) {
		t.Fatalf("bookkeeping update lost source text:\n%s", templateAfter)
	}
	if string(policyAfter) != string(policyBefore) {
		t.Fatal("base update rewrote policy.yaml")
	}
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceChanged) {
		t.Fatalf("stale pair update = %v", err)
	}
}

func TestAdvanceTemplateBaseFencesInterveningEditAndSettlesPostRenameRetry(t *testing.T) {
	for _, test := range []struct {
		name  string
		phase string
	}{
		{name: "edit before directory CAS", phase: "template_base_stage_durable"},
		{name: "failure after directory publish", phase: "template_base_directory_published"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _ := New(filepath.Join(t.TempDir(), "config"))
			source := sourceTemplateFixture(t)
			source.Template.BaseRevision = nil
			next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PublishTemplate(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			_, fingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
			if err != nil {
				t.Fatal(err)
			}
			store.phase = func(phase string) error {
				if phase != test.phase {
					return nil
				}
				if test.phase == "template_base_stage_durable" {
					data, readErr := os.ReadFile(path)
					if readErr != nil {
						return readErr
					}
					return os.WriteFile(path, append([]byte("# concurrent edit\n"), data...), 0o600)
				}
				return errors.New("injected post-rename interruption")
			}
			_, err = store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil)
			if test.phase == "template_base_stage_durable" {
				if !errors.Is(err, tobari.ErrResourceSourceChanged) {
					t.Fatalf("intervening edit = %v", err)
				}
				data, _ := os.ReadFile(path)
				if !strings.HasPrefix(string(data), "# concurrent edit") {
					t.Fatal("intervening edit was overwritten")
				}
				return
			}
			if !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
				t.Fatalf("post-rename interruption = %v", err)
			}
			store.phase = func(string) error { return nil }
			if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); err != nil {
				t.Fatalf("idempotent bookkeeping settlement = %v", err)
			}
		})
	}
}

func TestAdvanceTemplateBaseLeftoverStageCleanupFailureIsRecoverable(t *testing.T) {
	for _, failure := range []string{"remove", "sync"} {
		t.Run(failure, func(t *testing.T) {
			store, _ := New(filepath.Join(t.TempDir(), "config"))
			source := sourceTemplateFixture(t)
			source.Template.BaseRevision = nil
			next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PublishTemplate(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			_, fingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
			if err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(filepath.Dir(filepath.Dir(path)), ".template-base-"+string(sourceTemplateID)+"-new")
			if err := os.Mkdir(stage, 0o700); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected leftover-stage cleanup failure")
			if failure == "remove" {
				store.removeAll = func(observed string) error {
					if observed != stage {
						t.Fatalf("cleanup target = %s", observed)
					}
					return injected
				}
			} else {
				store.sync = func(string) error { return injected }
			}
			if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) || !errors.Is(err, injected) {
				t.Fatalf("%s cleanup failure = %v", failure, err)
			}
			store.removeAll = os.RemoveAll
			store.sync = syncDirectory
			settled, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil)
			if err != nil || settled == "" {
				t.Fatalf("%s cleanup retry = %q/%v", failure, settled, err)
			}
			if _, err := os.Lstat(stage); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("%s cleanup retry retained stage: %v", failure, err)
			}
		})
	}
}

func TestAdvanceTemplateBaseRecoveryRejectsEditAfterPublishedDirectory(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	source := sourceTemplateFixture(t)
	source.Template.BaseRevision = nil
	next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	_, fingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("injected interruption after directory publication")
	store.phase = func(phase string) error {
		if phase == "template_base_directory_published" {
			return interrupted
		}
		return nil
	}
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("interrupted bookkeeping = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	modified := append([]byte("# edit after crash\n"), data...)
	if err := os.WriteFile(path, modified, 0o600); err != nil {
		t.Fatal(err)
	}
	store.phase = func(string) error { return nil }
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceChanged) {
		t.Fatalf("edited post-publication recovery = %v", err)
	}
	observed, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(observed, modified) {
		t.Fatalf("intervening edit changed: equal=%t err=%v", bytes.Equal(observed, modified), err)
	}
}

func TestAdvanceTemplateBasePreservesWriteThroughOpenQuarantinedFile(t *testing.T) {
	store, _ := New(filepath.Join(t.TempDir(), "config"))
	source := sourceTemplateFixture(t)
	source.Template.BaseRevision = nil
	next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	_, fingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	openOld, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) // #nosec G304 -- test-owned exact source path.
	if err != nil {
		t.Fatal(err)
	}
	defer openOld.Close()
	wrote := false
	store.phase = func(phase string) error {
		if phase == "template_base_before_quarantine_cleanup" && !wrote {
			wrote = true
			if _, err := openOld.WriteString("# edit through old descriptor\n"); err != nil {
				return err
			}
			return openOld.Sync()
		}
		return nil
	}
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, fingerprint, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("open-descriptor edit settlement = %v", err)
	}
	_, currentFingerprint, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	store.phase = func(string) error { return nil }
	settledFingerprint, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, currentFingerprint, next, sourceRuntimeBindingFixture(), nil)
	if err != nil {
		t.Fatalf("open-descriptor recovery = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("# edit through old descriptor")) || !bytes.Contains(data, []byte(string(next))) {
		t.Fatalf("recovered source lost bytes: %s / %v", data, err)
	}
	if replay, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, settledFingerprint, next, sourceRuntimeBindingFixture(), nil); err != nil || replay != settledFingerprint {
		t.Fatalf("idempotent open-descriptor recovery = %q/%v", replay, err)
	}
}

func TestAdvanceTemplateBaseRepairSurvivesProcessDeathAtEveryBoundary(t *testing.T) {
	boundaries := []string{
		"template_base_repair_before_discard_rename",
		"template_base_repair_discard_renamed",
		"template_base_repair_discard_sync",
		"template_base_repair_before_publish_rename",
		"template_base_repair_published_renamed",
		"template_base_repair_publish_sync",
		"template_base_repair_quarantine_removed",
		"template_base_repair_discard_removed",
		"template_base_repair_cleanup_sync",
		"template_base_repair_journal_removed",
		"template_base_repair_journal_remove_synced",
	}
	for _, phase := range []string{"prepared", "discard_renaming", "discard_renamed", "published_renaming", "published", "cleanup_started"} {
		for _, boundary := range []string{"temp_written", "temp_synced", "renamed", "parent_synced"} {
			boundaries = append(boundaries, "template_base_repair_journal_"+boundary+":"+phase)
		}
	}
	for _, boundary := range boundaries {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			command := exec.Command(os.Args[0], "-test.run=^TestAdvanceTemplateBaseRepairCrashProcess$") // #nosec G204 -- exact current test binary and fixed selector.
			command.Env = append(os.Environ(), "TOBARI_TEMPLATE_REPAIR_ROOT="+root, "TOBARI_TEMPLATE_REPAIR_BOUNDARY="+boundary)
			err := command.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 91 {
				t.Fatalf("repair crash at %s = %v", boundary, err)
			}
			expected, err := os.ReadFile(filepath.Join(root, "expected-fingerprint"))
			if err != nil {
				t.Fatal(err)
			}
			store, err := New(filepath.Join(root, "config"))
			if err != nil {
				t.Fatal(err)
			}
			source := sourceTemplateFixture(t)
			source.Template.BaseRevision = nil
			next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
			if err != nil {
				t.Fatal(err)
			}
			settled, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, string(expected), next, sourceRuntimeBindingFixture(), nil)
			if err != nil {
				t.Fatalf("repair restart after %s = %v", boundary, err)
			}
			path, _ := store.TemplatePath(sourceTemplateID)
			data, err := os.ReadFile(path)
			if err != nil || !bytes.Contains(data, []byte("# edit through old descriptor")) || !bytes.Contains(data, []byte(string(next))) {
				t.Fatalf("repair after %s lost edited source: %s / %v", boundary, data, err)
			}
			if replay, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, settled, next, sourceRuntimeBindingFixture(), nil); err != nil || replay != settled {
				t.Fatalf("repair replay after %s = %q/%v", boundary, replay, err)
			}
			concept := filepath.Dir(filepath.Dir(path))
			matches, err := filepath.Glob(filepath.Join(concept, ".template-base-"+string(sourceTemplateID)+"-*"))
			if err != nil || len(matches) != 0 {
				t.Fatalf("repair after %s retained reserved paths %v/%v", boundary, matches, err)
			}
		})
	}
}

func TestAdvanceTemplateBaseRepairCrashProcess(t *testing.T) {
	root := os.Getenv("TOBARI_TEMPLATE_REPAIR_ROOT")
	boundary := os.Getenv("TOBARI_TEMPLATE_REPAIR_BOUNDARY")
	if root == "" || boundary == "" {
		return
	}
	store, err := New(filepath.Join(root, "config"))
	if err != nil {
		t.Fatal(err)
	}
	source := sourceTemplateFixture(t)
	source.Template.BaseRevision = nil
	next, err := source.SemanticRevision(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PublishTemplate(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	path, _ := store.TemplatePath(sourceTemplateID)
	_, initial, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	openOld, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) // #nosec G304 -- test-owned exact source path.
	if err != nil {
		t.Fatal(err)
	}
	wrote := false
	store.phase = func(phase string) error {
		if phase == "template_base_before_quarantine_cleanup" && !wrote {
			wrote = true
			if _, err := openOld.WriteString("# edit through old descriptor\n"); err != nil {
				return err
			}
			return openOld.Sync()
		}
		return nil
	}
	if _, err := store.AdvanceTemplateBase(context.Background(), sourceTemplateID, initial, next, sourceRuntimeBindingFixture(), nil); !errors.Is(err, tobari.ErrResourceSourceRecoveryRequired) {
		t.Fatalf("prepare open-FD repair = %v", err)
	}
	if err := openOld.Close(); err != nil {
		t.Fatal(err)
	}
	_, expected, _, err := store.ReadTemplateSnapshot(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "expected-fingerprint"), []byte(expected), 0o600); err != nil {
		t.Fatal(err)
	}
	store.phase = func(phase string) error {
		if phase == boundary {
			os.Exit(91)
		}
		return nil
	}
	_, _ = store.AdvanceTemplateBase(context.Background(), sourceTemplateID, expected, next, sourceRuntimeBindingFixture(), nil)
	os.Exit(92)
}

func TestStoreRejectsSymlinkHardlinkUnsafeModeAndDirectoryIdentityDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "symlink", mutate: func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "target.yaml")
			if err := os.WriteFile(target, []byte("schema: tobari.dev/context/v1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "hardlink", mutate: func(t *testing.T, path string) {
			target := filepath.Join(t.TempDir(), "target.yaml")
			if err := os.WriteFile(target, []byte("schema: tobari.dev/context/v1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(target, path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "directory mode", mutate: func(t *testing.T, path string) {
			if err := os.Chmod(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _ := New(filepath.Join(t.TempDir(), "config"))
			source := tobari.ContextSource{SchemaVersion: tobari.ContextSourceSchemaVersion, ContextID: sourceContextID, ProjectRoot: "/workspace/example", TemplateID: sourceTemplateID}
			if err := store.PublishContext(context.Background(), source); err != nil {
				t.Fatal(err)
			}
			path, _ := store.ContextPath(sourceContextID)
			test.mutate(t, path)
			if _, _, err := store.ReadContext(context.Background(), sourceContextID); err == nil {
				t.Fatal("unsafe Context source accepted")
			}
		})
	}

	store, _ := New(filepath.Join(t.TempDir(), "config"))
	if err := store.PublishTemplate(context.Background(), sourceTemplateFixture(t)); err != nil {
		t.Fatal(err)
	}
	oldPath, _ := store.TemplatePath(sourceTemplateID)
	newPath, _ := store.TemplatePath(otherTemplateID)
	if err := os.Rename(filepath.Dir(oldPath), filepath.Dir(newPath)); err != nil {
		t.Fatal(err)
	}
	if _, _, present, err := store.ReadTemplateSnapshot(context.Background(), otherTemplateID); !present || !errors.Is(err, tobari.ErrResourceSourceInvalid) {
		t.Fatalf("directory/document identity drift = present %v, err %v", present, err)
	}
}
