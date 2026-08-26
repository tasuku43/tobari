package workspaceauthoritystore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type migrationSourceStageFixture struct {
	committed   bool
	aborted     bool
	verifyErr   error
	completeErr error
}

func (s *migrationSourceStageFixture) ExpectedIdentity() (tobari.SemanticDigest, error) {
	return tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64)), nil
}

func (s *migrationSourceStageFixture) Commit(context.Context) error { s.committed = true; return nil }
func (s *migrationSourceStageFixture) Verify(context.Context) error { return s.verifyErr }
func (s *migrationSourceStageFixture) Rollback(context.Context) error {
	s.committed = false
	return nil
}
func (s *migrationSourceStageFixture) Complete(context.Context) error {
	if s.completeErr != nil {
		err := s.completeErr
		s.completeErr = nil
		return err
	}
	return nil
}
func (s *migrationSourceStageFixture) Abort(context.Context) error { s.aborted = true; return nil }

func migrationSourcePreparer(called *bool) InstallationMigrationSourcePreparer {
	return func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (InstallationMigrationSourceStage, error) {
		if called != nil {
			*called = true
		}
		return &migrationSourceStageFixture{}, nil
	}
}

func migrationRuntimeObserver(context.Context, tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error) {
	return tobari.SemanticDigest("sha256:" + strings.Repeat("9", 64)), nil
}

func legacyMigrationFixture(t *testing.T) (*Store, *Mutator, tobari.WorkspaceAuthorityCollection, []byte) {
	t.Helper()
	collection := storeCollectionFixture(t)
	encoded, err := json.Marshal(collection)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(t.TempDir(), "state")
	root := filepath.Join(parent, "authority")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, legacyAuthorityFileName), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	store.legacyGuard = mutationLegacyGuard{}
	mutator, err := NewMutator(context.Background(), store, &mutationLifecycle{}, &templateRuntimeRevisionFixture{}, &deletionAuthorityFixture{}, &policyActivationFixture{}, &finalSettlementFixture{})
	if err != nil {
		t.Fatal(err)
	}
	mutator.clock = func() time.Time { return time.Unix(1, 0).UTC() }
	mutator.entropy = bytes.NewReader(bytes.Repeat([]byte{7}, 1024))
	return store, mutator, collection, encoded
}

func TestInstallationMigrationPlanIsReadOnlyAndApplyIsStaleBound(t *testing.T) {
	store, mutator, _, original := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(store.root, legacyAuthorityFileName)); err != nil || !bytes.Equal(got, original) {
		t.Fatalf("plan mutated legacy source: %v", err)
	}
	changedBytes := append(append([]byte(nil), original...), '\n')
	if err := os.WriteFile(filepath.Join(store.root, legacyAuthorityFileName), changedBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(&called))
	if !errors.Is(err, tobari.ErrMigrationSourceChanged) || called {
		t.Fatalf("stale migration err=%v publisher_called=%t", err, called)
	}
	if _, err := os.Lstat(filepath.Join(store.root, activeFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale plan activated generation: %v", err)
	}
}

func TestInstallationMigrationRevalidatesLegacyBytesAfterSourceStaging(t *testing.T) {
	store, mutator, _, original := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	stage := &migrationSourceStageFixture{}
	_, err = mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (InstallationMigrationSourceStage, error) {
		changed := append(append([]byte(nil), original...), '\n')
		if err := os.WriteFile(filepath.Join(store.root, legacyAuthorityFileName), changed, 0o600); err != nil {
			return nil, err
		}
		return stage, nil
	})
	if !errors.Is(err, tobari.ErrMigrationSourceChanged) || !stage.aborted || stage.committed {
		t.Fatalf("stale-after-stage err=%v stage=%+v", err, stage)
	}
	if _, err := os.Lstat(filepath.Join(store.root, activeFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staged migration activated authority: %v", err)
	}
	if _, err := os.Lstat(mutator.installationMigrationTransactionPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale staged migration left transaction journal: %v", err)
	}
}

func TestInstallationMigrationRuntimeDriftAfterStagingHasZeroPublication(t *testing.T) {
	store, mutator, _, original := legacyMigrationFixture(t)
	calls := 0
	observer := func(context.Context, tobari.WorkspaceAuthorityCollection) (tobari.SemanticDigest, error) {
		calls++
		value := "9"
		if calls >= 3 {
			value = "8"
		}
		return tobari.SemanticDigest("sha256:" + strings.Repeat(value, 64)), nil
	}
	plan, err := mutator.PlanInstallationMigration(context.Background(), observer)
	if err != nil {
		t.Fatal(err)
	}
	stage := &migrationSourceStageFixture{}
	_, err = mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, observer, func(context.Context, tobari.WorkspaceAuthorityCollection, bool) (InstallationMigrationSourceStage, error) {
		return stage, nil
	})
	if !errors.Is(err, tobari.ErrMigrationSourceChanged) || stage.committed || !stage.aborted {
		t.Fatalf("Runtime-stale migration = %v stage=%+v", err, stage)
	}
	got, readErr := os.ReadFile(filepath.Join(store.root, legacyAuthorityFileName))
	if readErr != nil || !bytes.Equal(got, original) {
		t.Fatalf("Runtime-stale migration changed legacy authority: equal=%t err=%v", bytes.Equal(got, original), readErr)
	}
	if _, err := os.Lstat(filepath.Join(store.root, activeFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Runtime-stale migration activated generation: %v", err)
	}
}

func TestInstallationMigrationPublishesVerifiedGenerationThenRetiresAuthorityJSON(t *testing.T) {
	store, mutator, source, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	published := false
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, func(_ context.Context, collection tobari.WorkspaceAuthorityCollection, _ bool) (InstallationMigrationSourceStage, error) {
		published = collection.Generation == source.Generation && collection.Revision == source.Revision
		return &migrationSourceStageFixture{}, nil
	})
	if err != nil || !published || result.ActiveGeneration != source.Generation || result.ActiveRevision != source.Revision {
		t.Fatalf("result=%+v published=%t err=%v", result, published, err)
	}
	if _, err := os.Lstat(filepath.Join(store.root, legacyAuthorityFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("authority.json remains: %v", err)
	}
	active, present, err := store.ReadComplete(context.Background())
	if err != nil || !present || active.Generation != source.Generation || active.Revision != source.Revision {
		t.Fatalf("active=%+v present=%t err=%v", active, present, err)
	}
	if _, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver); !errors.Is(err, tobari.ErrMigrationNotSupported) {
		t.Fatalf("replan err=%v", err)
	}
}

func TestInstallationMigrationRestartRollsBackPreAcceptanceAndReappliesSamePlan(t *testing.T) {
	store, mutator, source, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	stage := &migrationSourceStageFixture{}
	preparer := func(_ context.Context, _ tobari.WorkspaceAuthorityCollection, _ bool) (InstallationMigrationSourceStage, error) {
		return stage, nil
	}
	originalRename := mutator.rename
	interrupted := errors.New("synthetic crash after authority publish")
	failed := false
	mutator.rename = func(from, to string) error {
		if err := originalRename(from, to); err != nil {
			return err
		}
		if !failed && from == store.root+".migration-stage" && to == store.root {
			failed = true
			return interrupted
		}
		return nil
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, preparer); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("interrupted migration = %v", err)
	}
	if _, err := os.Lstat(mutator.installationMigrationTransactionPath()); err != nil {
		t.Fatalf("durable transaction missing: %v", err)
	}
	mutator.rename = originalRename
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, preparer)
	if err != nil || result.ActiveRevision != source.Revision || !result.Changed {
		t.Fatalf("restart settlement = %+v/%v", result, err)
	}
	if _, err := os.Lstat(mutator.installationMigrationTransactionPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("transaction remained: %v", err)
	}
}

func TestInstallationMigrationRestartCompletesAcceptedComponentCleanup(t *testing.T) {
	store, mutator, source, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	stage := &migrationSourceStageFixture{completeErr: errors.New("synthetic accepted cleanup crash")}
	preparer := func(_ context.Context, _ tobari.WorkspaceAuthorityCollection, _ bool) (InstallationMigrationSourceStage, error) {
		return stage, nil
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, preparer); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("accepted cleanup interruption = %v", err)
	}
	if _, err := os.Lstat(store.root + ".migration-old"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("accepted migration retained rollback authority: %v", err)
	}
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, preparer)
	if err != nil || result.ActiveRevision != source.Revision {
		t.Fatalf("accepted restart settlement = %+v/%v", result, err)
	}
}

func completeInstallationMigrationFixture(t *testing.T) (*Store, *Mutator, tobari.InstallationMigrationPlan, tobari.InstallationMigrationResult) {
	t.Helper()
	store, mutator, _, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil))
	if err != nil {
		t.Fatal(err)
	}
	return store, mutator, plan, result
}

func TestInstallationMigrationAcceptedReceiptBindsExactPlanAuthority(t *testing.T) {
	_, mutator, plan, result := completeInstallationMigrationFixture(t)
	receipt, err := mutator.newInstallationMigrationAcceptedReceipt(plan, result)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*installationMigrationAcceptedReceipt)
	}{
		{name: "arbitrary plan reference", mutate: func(value *installationMigrationAcceptedReceipt) {
			value.Plan.PlanRef = "implan1_" + strings.Repeat("a", 64)
		}},
		{name: "altered legacy source digest", mutate: func(value *installationMigrationAcceptedReceipt) {
			value.Plan.SourceDigest = tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64))
		}},
		{name: "altered active revision", mutate: func(value *installationMigrationAcceptedReceipt) {
			value.ActiveRevision = tobari.SemanticDigest("sha256:" + strings.Repeat("c", 64))
		}},
		{name: "altered installation identity", mutate: func(value *installationMigrationAcceptedReceipt) {
			value.InstallationIdentity = tobari.SemanticDigest("sha256:" + strings.Repeat("d", 64))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := receipt
			test.mutate(&changed)
			// Even a local writer that recomputes the unkeyed content digest
			// cannot make an authority-inconsistent receipt valid.
			changed.ReceiptDigest = installationMigrationAcceptedReceiptDigest(changed)
			if err := mutator.validateInstallationMigrationAcceptedReceipt(changed); err == nil {
				t.Fatal("altered accepted receipt validated")
			}
		})
	}
}

func TestInstallationMigrationAcceptedReceiptRejectsFullySelfConsistentForgedPlan(t *testing.T) {
	store, mutator, originalPlan, _ := completeInstallationMigrationFixture(t)
	active, present, err := store.readGenerationRaw(context.Background())
	if err != nil || !present {
		t.Fatalf("active authority = %+v/%t/%v", active, present, err)
	}
	forgedPlan, err := tobari.NewInstallationMigrationPlan(
		tobari.SemanticDigest("sha256:"+strings.Repeat("a", 64)),
		tobari.SemanticDigest("sha256:"+strings.Repeat("b", 64)),
		active,
	)
	if err != nil || forgedPlan.PlanRef == originalPlan.PlanRef {
		t.Fatalf("forged plan = %+v/%v", forgedPlan, err)
	}
	forgedResult := tobari.InstallationMigrationResult{SchemaVersion: 1, PlanRef: forgedPlan.PlanRef, ActiveGeneration: active.Generation, ActiveRevision: active.Revision, Changed: true}
	forgedReceipt, err := mutator.newInstallationMigrationAcceptedReceipt(forgedPlan, forgedResult)
	if err != nil {
		t.Fatalf("fully self-consistent forged receipt fixture = %v", err)
	}
	data, err := json.Marshal(forgedReceipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mutator.installationMigrationAcceptedReceiptPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), forgedPlan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("fully self-consistent forged plan returned success: %v", err)
	}
}

func TestInstallationMigrationGenerationCommitsExactPlanProvenance(t *testing.T) {
	store, _, plan, _ := completeInstallationMigrationFixture(t)
	active, present, err := store.readGenerationRaw(context.Background())
	if err != nil || !present {
		t.Fatalf("active authority = %+v/%t/%v", active, present, err)
	}
	provenance, provenancePresent, err := store.readActiveInstallationMigrationProvenance(context.Background(), active)
	if err != nil || !provenancePresent || provenance != plan {
		t.Fatalf("migration provenance = %+v/%t/%v", provenance, provenancePresent, err)
	}
	ordinary, err := prepareAuthorityGeneration(active)
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.manifest.InstallationMigrationProvenance != nil {
		t.Fatal("ordinary non-migration generation inherited migration provenance")
	}
	migrated, err := prepareAuthorityGenerationWithMigrationProvenance(active, &plan)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.manifest.InstallationMigrationProvenance == nil || *migrated.manifest.InstallationMigrationProvenance != plan || migrated.manifestDigest == ordinary.manifestDigest {
		t.Fatal("migration plan was not content-addressed by the generation manifest")
	}
}

func TestInstallationMigrationAcceptedReceiptRejectsActiveGenerationDrift(t *testing.T) {
	store, mutator, plan, _ := completeInstallationMigrationFixture(t)
	active, present, err := store.readGenerationRaw(context.Background())
	if err != nil || !present {
		t.Fatalf("active authority = %+v/%t/%v", active, present, err)
	}
	drift := active.Clone()
	drift.Generation++
	if err := drift.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := mutator.publishGeneration(drift); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("active drift returned success: %v", err)
	}
}

func TestInstallationMigrationAcceptedReceiptRejectsArbitraryPlanReference(t *testing.T) {
	_, mutator, _, _ := completeInstallationMigrationFixture(t)
	arbitrary := "implan1_" + strings.Repeat("a", 64)
	if _, err := mutator.ApplyInstallationMigration(context.Background(), arbitrary, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationSourceChanged) {
		t.Fatalf("arbitrary plan reference returned success: %v", err)
	}
}

func TestInstallationMigrationAcceptedReceiptRejectsAlteredCanonicalFields(t *testing.T) {
	store, mutator, plan, _ := completeInstallationMigrationFixture(t)
	path := filepath.Join(store.root, "journal", "installation-migration-accepted.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var receipt installationMigrationAcceptedReceipt
	if err := decodeStrictJSON(data, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Plan.RuntimeSourceDigest = tobari.SemanticDigest("sha256:" + strings.Repeat("e", 64))
	receipt.ReceiptDigest = installationMigrationAcceptedReceiptDigest(receipt)
	data, err = json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("altered canonical receipt returned success: %v", err)
	}
}

func TestInstallationMigrationAcceptedReceiptCannotBeCopiedAcrossInstallations(t *testing.T) {
	storeA, _, _, _ := completeInstallationMigrationFixture(t)
	storeB, mutatorB, planB, _ := completeInstallationMigrationFixture(t)
	foreign, err := os.ReadFile(filepath.Join(storeA.root, "journal", "installation-migration-accepted.json"))
	if err != nil {
		t.Fatal(err)
	}
	pathB := filepath.Join(storeB.root, "journal", "installation-migration-accepted.json")
	if err := os.WriteFile(pathB, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mutatorB.ApplyInstallationMigration(context.Background(), planB.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("foreign accepted receipt returned success: %v", err)
	}
}

func TestInstallationMigrationAcceptedTransactionReplacesMalformedReceiptStage(t *testing.T) {
	store, mutator, _, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	interrupted := errors.New("synthetic receipt response loss")
	mutator.installationMigrationBoundary = func(phase string) error {
		if phase == "accepted_receipt_temp_written" {
			return interrupted
		}
		return nil
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("interrupted receipt = %v", err)
	}
	temp := filepath.Join(store.root, "journal", "installation-migration-accepted.json.next")
	if err := os.WriteFile(temp, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	mutator.installationMigrationBoundary = nil
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil))
	if err != nil || result.PlanRef != plan.PlanRef {
		t.Fatalf("malformed stage settlement = %+v/%v", result, err)
	}
	if _, err := os.Lstat(temp); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("receipt stage remained: %v", err)
	}
}

func TestInstallationMigrationAcceptedTransactionReplacesForeignReceiptStage(t *testing.T) {
	foreignStore, _, _, _ := completeInstallationMigrationFixture(t)
	foreign, err := os.ReadFile(filepath.Join(foreignStore.root, "journal", "installation-migration-accepted.json"))
	if err != nil {
		t.Fatal(err)
	}
	store, mutator, _, _ := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	mutator.installationMigrationBoundary = func(phase string) error {
		if phase == "accepted_receipt_temp_written" {
			return errors.New("synthetic receipt response loss")
		}
		return nil
	}
	if _, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil)); !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("interrupted receipt = %v", err)
	}
	temp := filepath.Join(store.root, "journal", "installation-migration-accepted.json.next")
	if err := os.WriteFile(temp, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	mutator.installationMigrationBoundary = nil
	result, err := mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil))
	if err != nil || result.PlanRef != plan.PlanRef {
		t.Fatalf("foreign stage settlement = %+v/%v", result, err)
	}
}

func TestInstallationMigrationRestoresLegacyAuthorityWhenSwapSyncFails(t *testing.T) {
	store, mutator, _, original := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	originalSync := mutator.sync
	syncCalls := 0
	mutator.sync = func(path string) error {
		syncCalls++
		if syncCalls == 1 {
			return errors.New("injected migration swap sync failure")
		}
		return originalSync(path)
	}
	_, err = mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil))
	if !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("migration err=%v", err)
	}
	assertLegacyMigrationRestored(t, store, original)
}

func TestInstallationMigrationRestoresLegacyAuthorityWhenActiveReadBackFails(t *testing.T) {
	store, mutator, _, original := legacyMigrationFixture(t)
	plan, err := mutator.PlanInstallationMigration(context.Background(), migrationRuntimeObserver)
	if err != nil {
		t.Fatal(err)
	}
	originalRename := mutator.rename
	mutator.rename = func(from, to string) error {
		if err := originalRename(from, to); err != nil {
			return err
		}
		if from == store.root+".migration-stage" && to == store.root {
			return os.WriteFile(filepath.Join(store.root, activeFileName), []byte("{}"), 0o600)
		}
		return nil
	}
	_, err = mutator.ApplyInstallationMigration(context.Background(), plan.PlanRef, migrationRuntimeObserver, migrationSourcePreparer(nil))
	if !errors.Is(err, tobari.ErrMigrationWriteFailed) {
		t.Fatalf("migration err=%v", err)
	}
	assertLegacyMigrationRestored(t, store, original)
}

func assertLegacyMigrationRestored(t *testing.T, store *Store, original []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(store.root, legacyAuthorityFileName))
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("legacy authority was not restored exactly: bytes_equal=%t err=%v", bytes.Equal(got, original), err)
	}
	for _, suffix := range []string{".migration-stage", ".migration-old"} {
		if _, err := os.Lstat(store.root + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("reserved migration tree %s remains: %v", suffix, err)
		}
	}
	if _, _, err := store.ReadComplete(context.Background()); !errors.Is(err, tobari.ErrFinalAuthorityMigrationRequired) {
		t.Fatalf("ordinary read did not preserve migration-required fault: %v", err)
	}
}
