package workspaceauthoritymigration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/workspaceauthoritystore"
)

const (
	migrationTemplateID  = "01912345-6789-7abc-8def-0123456789a1"
	migrationContextID   = tobari.ContextID("01912345-6789-7abc-8def-0123456789a2")
	migrationWorkspaceID = "01912345-6789-7abc-8def-0123456789a3"
)

type migrationFixture struct {
	root         string
	finalRoot    string
	transaction  string
	cutoff       string
	projectState string
	research     string
	home         string
	freshAuth    string
	prepared     PreparedPredecessor
	port         *fixturePreflight
	engine       *Engine
	homeBytes    []byte
}

type fixturePreflight struct {
	mu                  sync.Mutex
	prepared            PreparedPredecessor
	cutoff              string
	sources             []SourceItem
	finalRoot           string
	quiescence          Quiescence
	quiescenceSequence  []Quiescence
	quiescenceCallCount int
	prepareCalls        int
}

func (p *fixturePreflight) Prepare(context.Context) (PreparedPredecessor, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prepareCalls++
	return p.prepared, nil
}

func (p *fixturePreflight) ObserveQuiescence(context.Context) (Quiescence, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quiescenceCallCount++
	if len(p.quiescenceSequence) > 0 {
		value := p.quiescenceSequence[0]
		p.quiescenceSequence = p.quiescenceSequence[1:]
		return value, nil
	}
	return p.quiescence, nil
}

func (p *fixturePreflight) ObserveReaders(context.Context) (ReaderDisposition, error) {
	cutoffPresent := pathExists(p.cutoff)
	allCanonical := true
	for _, source := range p.sources {
		allCanonical = allCanonical && pathExists(source.Path)
	}
	store, _ := workspaceauthoritystore.New(p.finalRoot)
	_, finalPhysical, finalErr := store.ReadComplete(context.Background())
	if finalErr != nil {
		return ReaderDisposition{}, finalErr
	}
	if cutoffPresent {
		return ReaderDisposition{
			PredecessorComplete: allCanonical, FinalAbsent: !finalPhysical,
		}, nil
	}
	return ReaderDisposition{
		PredecessorUnavailable: true, FinalComplete: finalPhysical, FinalAbsent: !finalPhysical,
	}, nil
}

func newMigrationFixture(t *testing.T, research bool) *migrationFixture {
	t.Helper()
	root := t.TempDir()
	configRoot := filepath.Join(root, "config")
	stateRoot := filepath.Join(root, "state")
	transactionParent := filepath.Join(root, "transactions")
	for _, directory := range []string{configRoot, stateRoot, transactionParent} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cutoff := filepath.Join(configRoot, "contexts")
	if err := os.Mkdir(cutoff, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(cutoff, "manifest.json"), []byte(`{"schema_version":2}`))
	projectState := filepath.Join(stateRoot, "project-state.json")
	writeFixtureFile(t, projectState, []byte(`{"workspace":"preserved"}`))
	home := filepath.Join(stateRoot, "workspace-home")
	if err := os.Mkdir(home, 0o700); err != nil {
		t.Fatal(err)
	}
	homeBytes := []byte("native-auth-byte-for-byte\n")
	writeFixtureFile(t, filepath.Join(home, "auth.json"), homeBytes)

	sources := []SourceItem{
		fixtureSource(t, "contexts", cutoff, filepath.Join(configRoot, ".wp11-contexts"), true, false),
		fixtureSource(t, "project-state", projectState, filepath.Join(stateRoot, ".wp11-project-state.json"), false, false),
	}
	var researchPath string
	if research {
		researchPath = filepath.Join(configRoot, "research-auth")
		if err := os.Mkdir(researchPath, 0o700); err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, filepath.Join(researchPath, "vault.enc"), []byte("synthetic-ciphertext"))
		sources = append(sources, fixtureSource(t, "research-auth", researchPath, filepath.Join(configRoot, ".wp11-research-auth"), false, true))
		rootKeyPath := filepath.Join(configRoot, "root.key")
		writeFixtureFile(t, rootKeyPath, []byte("synthetic-linux-key"))
		rootKeySource := fixtureSource(t, "research-root-key", rootKeyPath, filepath.Join(configRoot, ".wp11-root.key"), false, true)
		rootKeySource.LinuxRootKey = true
		sources = append(sources, rootKeySource)
	}

	body := migrationTemplateBody()
	finalRevision, err := tobari.NewWorkspaceTemplateRevision(tobari.WorkspaceTemplateID(migrationTemplateID), 1, body)
	if err != nil {
		t.Fatal(err)
	}
	legacyRevision := digest("a")
	homeDigest := digest("b")
	resolvedSpec := digest("c")
	applied := &tobari.PredecessorWorkspaceAppliedEntry{
		ManifestGeneration: 1, ManifestRevision: legacyRevision,
		RuntimeID: tobari.StandardRuntimeID, RuntimeRevision: finalRevision.Slices.RuntimeRevision,
		ResolvedSpec: resolvedSpec, ReconciledAt: time.Unix(1, 0).UTC(),
	}
	input := tobari.WorkspaceAuthorityMigrationInput{
		Templates: []tobari.PredecessorTemplate{{
			ID: migrationTemplateID, Name: "restricted", CurrentGeneration: 1, CurrentRevision: legacyRevision,
			Revisions: []tobari.PredecessorTemplateRevision{{Generation: 1, Revision: legacyRevision, Body: predecessorBody(body)}},
		}},
		Workspaces: []tobari.PredecessorWorkspace{{
			ID: migrationWorkspaceID, ProjectRoot: "/workspace/example", ManifestID: migrationTemplateID,
			Home: home, HomeDigest: homeDigest, CreationDefaults: finalRevision.Slices.CreationDefaultsDigest,
			LastSuccessfulEntry: applied,
			DockerObservation: tobari.PredecessorWorkspaceDockerObservation{
				State: tobari.PredecessorDockerObservationExactOwned, WorkspaceID: migrationWorkspaceID,
				ManifestGeneration: 1, ManifestRevision: legacyRevision, RuntimeID: tobari.StandardRuntimeID,
				RuntimeRevision: finalRevision.Slices.RuntimeRevision, ResolvedSpec: resolvedSpec,
			},
		}},
		ContextAssignments: []tobari.ContextIDAssignment{{ProjectRoot: "/workspace/example", PredecessorManifestID: migrationTemplateID, ContextID: migrationContextID}},
		PolicySets:         []tobari.PredecessorPolicySet{{ManifestID: migrationTemplateID, WorkspaceID: migrationWorkspaceID, ProjectRoot: "/workspace/example", Rules: []tobari.PredecessorPolicyRule{}}},
		PendingCandidates:  []tobari.PredecessorPendingCandidate{},
		DefaultManifestID:  stringPointer(migrationTemplateID),
	}
	if research {
		input.ResearchAuthority = tobari.PredecessorResearchAuthority{Present: true, Platform: tobari.ResearchAuthorityLinux}
	}
	prepared := PreparedPredecessor{
		Input: input, Sources: sources,
		StandardHomes:  []PreservedHome{{WorkspaceID: tobari.WorkspaceID(migrationWorkspaceID), Path: home, Digest: homeDigest}},
		FreshAuthPaths: []string{filepath.Join(configRoot, "final-research-auth")},
	}
	finalRoot := filepath.Join(configRoot, "workspace-authority")
	port := &fixturePreflight{
		prepared: prepared, cutoff: cutoff, sources: sources, finalRoot: finalRoot,
		quiescence: Quiescence{ClusterStopped: true},
	}
	transaction := filepath.Join(transactionParent, "wp11")
	engine, err := New(finalRoot, transaction, port)
	if err != nil {
		t.Fatal(err)
	}
	return &migrationFixture{
		root: root, finalRoot: finalRoot, transaction: transaction, cutoff: cutoff,
		projectState: projectState, research: researchPath, home: home,
		freshAuth: prepared.FreshAuthPaths[0], prepared: prepared, port: port, engine: engine, homeBytes: homeBytes,
	}
}

func TestEngineAppliesOneJournaledCutoverAndSecondApplyIsIdempotent(t *testing.T) {
	fixture := newMigrationFixture(t, true)
	result, err := fixture.engine.Apply(context.Background())
	if err != nil || !result.Changed || result.ResearchDisposition != tobari.ResearchAuthReauthenticationRequired || len(result.ContextAssignments) != 1 || result.ContextAssignments[0].ContextID != migrationContextID {
		t.Fatalf("apply=%#v err=%v", result, err)
	}
	assertFinalSelected(t, fixture)
	assertHomeBytes(t, fixture)
	for _, source := range fixture.prepared.Sources {
		if pathExists(source.Path) || !pathExists(source.BackupPath) {
			t.Fatalf("source %q was not exactly quarantined", source.Key)
		}
	}
	second, err := fixture.engine.Apply(context.Background())
	if err != nil || second.Changed || second.CollectionRevision != result.CollectionRevision || second.ContextAssignments[0].ContextID != migrationContextID || fixture.port.prepareCalls != 1 {
		t.Fatalf("second=%#v calls=%d err=%v", second, fixture.port.prepareCalls, err)
	}
	assertHomeBytes(t, fixture)
}

func TestEngineRollbackRestoresExactPredecessorAndNeverMergesFreshAuth(t *testing.T) {
	fixture := newMigrationFixture(t, true)
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fixture.freshAuth, []byte("fresh"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Rollback(context.Background()); err == nil {
		t.Fatal("fresh canonical auth did not block rollback")
	}
	if err := os.Remove(fixture.freshAuth); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPredecessorSelected(t, fixture)
	assertHomeBytes(t, fixture)
	for _, source := range fixture.prepared.Sources {
		if !pathExists(source.Path) || pathExists(source.BackupPath) {
			t.Fatalf("source %q was not exactly restored", source.Key)
		}
	}
	rolled := fixture.finalRoot + rolledFinalSuffix
	store, _ := workspaceauthoritystore.New(rolled)
	if _, present, err := store.ReadComplete(context.Background()); err != nil || !present {
		t.Fatalf("private rolled final present=%t err=%v", present, err)
	}
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatalf("second rollback must be idempotent: %v", err)
	}
	if _, err := fixture.engine.Apply(context.Background()); err == nil {
		t.Fatal("rolled-back transaction unexpectedly supported ambiguous reapply")
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineReconcilesOnlyJournalOwnedPartialFinalStage(t *testing.T) {
	tests := map[string]func(*Engine) func(){
		"after stage directory": func(engine *Engine) func() {
			engine.afterStageMkdir = func() error { return errors.New("synthetic process death after stage mkdir") }
			return func() { engine.afterStageMkdir = nil }
		},
		"after authority write": func(engine *Engine) func() {
			engine.afterStageWrite = func() error { return errors.New("synthetic process death after stage write") }
			return func() { engine.afterStageWrite = nil }
		},
	}
	for name, installCrash := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newMigrationFixture(t, false)
			clearCrash := installCrash(fixture.engine)
			if _, err := fixture.engine.Apply(context.Background()); err == nil {
				t.Fatal("injected final-stage interruption succeeded")
			}
			journal, present, err := fixture.engine.readJournal()
			if err != nil || !present || journal.Phase != phasePrepared || journal.Moved != 0 {
				t.Fatalf("journal=%#v present=%t err=%v", journal, present, err)
			}
			assertPredecessorSelected(t, fixture)
			if !pathExists(fixture.finalRoot + finalStageSuffix) {
				t.Fatal("injected interruption did not leave the expected partial stage")
			}
			clearCrash()
			if _, err := fixture.engine.Apply(context.Background()); err != nil {
				t.Fatal(err)
			}
			assertFinalSelected(t, fixture)
		})
	}

	t.Run("pre-journal stage is not claimed", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		if err := os.Mkdir(fixture.finalRoot+finalStageSuffix, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("preexisting reserved stage was claimed")
		}
		if _, present, err := fixture.engine.readJournal(); err != nil || present {
			t.Fatalf("unsafe pre-journal stage published journal present=%t err=%v", present, err)
		}
		assertPredecessorSelected(t, fixture)
	})

	t.Run("rollback resumes interrupted noncanonical cleanup", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		fixture.engine.afterStageWrite = func() error { return errors.New("synthetic process death after stage write") }
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("injected final-stage interruption succeeded")
		}
		fixture.engine.afterStageWrite = nil
		fixture.engine.removeAll = func(path string) error {
			if err := os.Remove(filepath.Join(path, authorityFileName)); err != nil {
				return err
			}
			return errors.New("synthetic process death during noncanonical cleanup")
		}
		if err := fixture.engine.Rollback(context.Background()); err == nil {
			t.Fatal("injected cleanup interruption succeeded")
		}
		assertPredecessorSelected(t, fixture)
		journal, _, err := fixture.engine.readJournal()
		if err != nil || journal.Phase != phaseRollback || !pathExists(fixture.finalRoot+finalStageSuffix) {
			t.Fatalf("rollback journal=%#v err=%v", journal, err)
		}
		fixture.engine.removeAll = os.RemoveAll
		if err := fixture.engine.Rollback(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertPredecessorSelected(t, fixture)
		if pathExists(fixture.finalRoot + finalStageSuffix) {
			t.Fatal("resumed rollback retained final stage")
		}
	})
}

func TestEngineMacOSResearchQuarantineNeverRequiresFilesystemRootKey(t *testing.T) {
	fixture := newMigrationFixture(t, true)
	rootKey := fixture.port.prepared.Sources[len(fixture.port.prepared.Sources)-1]
	if !rootKey.LinuxRootKey {
		t.Fatal("fixture root key source is missing")
	}
	if err := os.Remove(rootKey.Path); err != nil {
		t.Fatal(err)
	}
	fixture.port.prepared.Sources = fixture.port.prepared.Sources[:len(fixture.port.prepared.Sources)-1]
	fixture.port.sources = fixture.port.prepared.Sources
	fixture.prepared = fixture.port.prepared
	fixture.port.prepared.Input.ResearchAuthority.Platform = tobari.ResearchAuthorityMacOS
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFinalSelected(t, fixture)
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineKeepsOneCompleteReaderSetAcrossForwardCrashPoints(t *testing.T) {
	t.Run("quiescence loss at cutoff keeps predecessor selected", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		fixture.port.quiescenceSequence = []Quiescence{
			{ClusterStopped: true},
			{ClusterStopped: true},
			{ClusterStopped: false},
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("reader cutoff proceeded after cluster started")
		}
		assertPredecessorSelectedWithFinalCandidate(t, fixture)
	})

	t.Run("source drift after final candidate keeps predecessor selected", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		prepareAndPublishCandidate(t, fixture)
		writeFixtureFileReplace(t, fixture.projectState, []byte(`{"drift":true}`))
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("source drift before cutoff succeeded")
		}
		assertPredecessorSelectedWithFinalCandidate(t, fixture)
	})

	t.Run("lost quiescence after final candidate keeps predecessor selected", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		prepareAndPublishCandidate(t, fixture)
		fixture.port.quiescence.LiveAttachments = 1
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("lost quiescence before cutoff succeeded")
		}
		assertPredecessorSelectedWithFinalCandidate(t, fixture)
	})

	t.Run("EXDEV before cutoff keeps predecessor selected", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		original := fixture.engine.rename
		failed := false
		fixture.engine.rename = func(source, target string) error {
			if source == fixture.cutoff && !failed {
				failed = true
				return syscall.EXDEV
			}
			return original(source, target)
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("injected cross-device cutoff move succeeded")
		}
		assertPredecessorSelectedWithFinalCandidate(t, fixture)
		journal, present, err := fixture.engine.readJournal()
		if err != nil || !present || journal.Phase != phaseFinalPublished || journal.Moved != 0 {
			t.Fatalf("journal=%#v present=%t err=%v", journal, present, err)
		}
		fixture.engine.rename = original
		if _, err := fixture.engine.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertFinalSelected(t, fixture)
	})

	t.Run("crash after cutoff exposes final and resumes subordinate", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		original := fixture.engine.rename
		failed := false
		fixture.engine.rename = func(source, target string) error {
			if source == fixture.projectState && !failed {
				failed = true
				return errors.New("synthetic crash before subordinate rename")
			}
			return original(source, target)
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("injected subordinate crash succeeded")
		}
		assertFinalSelected(t, fixture)
		journal, _, err := fixture.engine.readJournal()
		if err != nil || journal.Phase != phaseMoving || journal.Moved != 1 {
			t.Fatalf("journal=%#v err=%v", journal, err)
		}
		fixture.engine.rename = original
		if _, err := fixture.engine.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertFinalSelected(t, fixture)
	})

	t.Run("crash after cutoff rename before journal receipt resumes", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		journal := prepareAndPublishCandidate(t, fixture)
		if err := os.Rename(fixture.cutoff, journal.Sources[0].BackupPath); err != nil {
			t.Fatal(err)
		}
		assertFinalSelected(t, fixture)
		if _, err := fixture.engine.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertFinalSelected(t, fixture)
	})
}

func TestEngineRollbackRequiresQuiescenceBeforeAnyRestoreAndBeforeCutoff(t *testing.T) {
	for name, quiescence := range map[string]Quiescence{
		"cluster running": {ClusterStopped: false},
		"attachment live": {ClusterStopped: true, LiveAttachments: 1},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newMigrationFixture(t, false)
			if _, err := fixture.engine.Apply(context.Background()); err != nil {
				t.Fatal(err)
			}
			fixture.port.quiescence = quiescence
			if err := fixture.engine.Rollback(context.Background()); err == nil {
				t.Fatal("rollback proceeded without quiescence")
			}
			assertFinalSelected(t, fixture)
			journal, _, err := fixture.engine.readJournal()
			if err != nil || journal.Phase != phaseCommitted || journal.Moved != len(journal.Sources) {
				t.Fatalf("journal mutated before rollback quiescence: %#v err=%v", journal, err)
			}
		})
	}

	t.Run("quiescence lost after subordinate restore blocks cutoff", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		if _, err := fixture.engine.Apply(context.Background()); err != nil {
			t.Fatal(err)
		}
		fixture.port.quiescenceSequence = []Quiescence{
			{ClusterStopped: true},
			{ClusterStopped: true},
			{ClusterStopped: true},
			{ClusterStopped: false},
		}
		if err := fixture.engine.Rollback(context.Background()); err == nil {
			t.Fatal("rollback cutoff proceeded after quiescence was lost")
		}
		assertFinalSelected(t, fixture)
		journal, _, err := fixture.engine.readJournal()
		if err != nil || journal.Phase != phaseRollback || journal.Moved != 1 {
			t.Fatalf("rollback journal=%#v err=%v", journal, err)
		}
		if !pathExists(fixture.projectState) || pathExists(fixture.cutoff) {
			t.Fatal("rollback did not stop between subordinate restore and cutoff")
		}
	})
}

func TestEngineRollbackRecoveryRestoresSubordinatesBeforeCutoff(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	journal, _, err := fixture.engine.readJournal()
	if err != nil {
		t.Fatal(err)
	}
	journal.Phase = phaseRollback
	if err := fixture.engine.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	last := journal.Sources[len(journal.Sources)-1]
	if last.Cutoff {
		t.Fatal("cutoff was not ordered first")
	}
	if err := os.Rename(last.BackupPath, last.Path); err != nil {
		t.Fatal(err)
	}
	assertFinalSelected(t, fixture)
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineRollbackRecoversCutoffRestoreBeforeJournalReceipt(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	journal, _, err := fixture.engine.readJournal()
	if err != nil {
		t.Fatal(err)
	}
	journal.Phase = phaseRollback
	for index := len(journal.Sources) - 1; index > 0; index-- {
		item := journal.Sources[index]
		if err := os.Rename(item.BackupPath, item.Path); err != nil {
			t.Fatal(err)
		}
	}
	journal.Moved = 1
	if err := fixture.engine.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	cutoff := journal.Sources[0]
	if err := os.Rename(cutoff.BackupPath, cutoff.Path); err != nil {
		t.Fatal(err)
	}
	// The cutoff has already selected the complete predecessor while final
	// canonical bytes remain available but are no longer the selected reader.
	disposition, err := fixture.port.ObserveReaders(context.Background())
	if err != nil || !disposition.PredecessorComplete || disposition.FinalComplete || disposition.FinalAbsent {
		t.Fatalf("post-cutoff rollback disposition=%#v err=%v", disposition, err)
	}
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineRollbackClassifiesFinalRetirementAfterRename(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := fixture.engine.rename
	fixture.engine.rename = func(source, target string) error {
		if source == fixture.finalRoot && target == fixture.finalRoot+rolledFinalSuffix {
			if err := original(source, target); err != nil {
				return err
			}
			return errors.New("synthetic process death after final retirement rename")
		}
		return original(source, target)
	}
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineClassifiesPostRenameJournalAndSourceSuccessByReadback(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	journalHookCalls := 0
	fixture.engine.afterJournalRename = func() error {
		journalHookCalls++
		return errors.New("synthetic post-journal-rename failure")
	}
	original := fixture.engine.rename
	postSource := false
	fixture.engine.rename = func(source, target string) error {
		if source == fixture.cutoff && !postSource {
			postSource = true
			if err := original(source, target); err != nil {
				return err
			}
			return errors.New("synthetic post-source-rename failure")
		}
		return original(source, target)
	}
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	if journalHookCalls == 0 || !postSource {
		t.Fatalf("journal hooks=%d post-source=%t", journalHookCalls, postSource)
	}
	assertFinalSelected(t, fixture)
}

func TestEngineFailsClosedBeforeMutationOnUnsafeOrChangedPreflight(t *testing.T) {
	tests := map[string]func(*migrationFixture){
		"cluster running": func(f *migrationFixture) { f.port.quiescence.ClusterStopped = false },
		"attachment live": func(f *migrationFixture) { f.port.quiescence.LiveAttachments = 1 },
		"source drift": func(f *migrationFixture) {
			writeFixtureFileReplace(t, filepath.Join(f.cutoff, "manifest.json"), []byte(`{"changed":true}`))
		},
		"unsafe source mode": func(f *migrationFixture) {
			if err := os.Chmod(f.projectState, 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"backup collision": func(f *migrationFixture) { writeFixtureFile(t, f.prepared.Sources[1].BackupPath, []byte("collision")) },
		"home in mutation set": func(f *migrationFixture) {
			f.port.prepared.Sources[1].Path = f.home
			f.port.prepared.Sources[1].BackupPath = filepath.Join(filepath.Dir(f.home), ".home-backup")
			digest, kind, err := digestOwnedPath(f.home)
			if err != nil {
				t.Fatal(err)
			}
			f.port.prepared.Sources[1].Digest, f.port.prepared.Sources[1].Kind = digest, kind
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newMigrationFixture(t, false)
			mutate(fixture)
			if _, err := fixture.engine.Apply(context.Background()); err == nil {
				t.Fatal("unsafe preflight passed")
			}
			if !pathExists(fixture.cutoff) || pathExists(fixture.finalRoot) {
				t.Fatal("failed preflight mutated reader authority")
			}
		})
	}
}

func TestEngineFailsClosedOnIncompleteLinuxResearchOrSymlinkedSource(t *testing.T) {
	t.Run("Linux root key missing from exact set", func(t *testing.T) {
		fixture := newMigrationFixture(t, true)
		rootKey := fixture.port.prepared.Sources[len(fixture.port.prepared.Sources)-1]
		fixture.port.prepared.Sources = fixture.port.prepared.Sources[:len(fixture.port.prepared.Sources)-1]
		fixture.port.sources = fixture.port.prepared.Sources
		if err := os.Remove(rootKey.Path); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("incomplete Linux research set migrated")
		}
		assertPredecessorSelected(t, fixture)
	})

	t.Run("symlinked source", func(t *testing.T) {
		fixture := newMigrationFixture(t, false)
		if err := os.Remove(fixture.projectState); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(filepath.Dir(fixture.projectState), "unsafe-target")
		writeFixtureFile(t, target, []byte("unsafe"))
		if err := os.Symlink(target, fixture.projectState); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("symlinked predecessor source migrated")
		}
		if !pathExists(fixture.cutoff) || pathExists(fixture.finalRoot) {
			t.Fatal("symlinked-source rejection mutated authority")
		}
	})
}

func TestEngineRejectsOverBoundFinalEnvelopeBeforeJournalOrCutoff(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	advanced := tobari.WorkspaceTemplateAdvancedPolicySources{
		Tobari:     strings.Repeat("x", tobari.WorkspaceTemplateAdvancedPolicyMaxBytes-64),
		TobariTest: "package tobari_template\ntest := true\n",
	}
	if err := advanced.Validate(); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 8; index++ {
		body := predecessorBody(migrationTemplateBody())
		body.Policy.Mode = tobari.ManifestPolicyModeAdvanced
		body.Policy.AdvancedPolicy = &advanced
		id := fmt.Sprintf("01912345-6789-7abc-8def-%012x", 0x123456789a4+index)
		fixture.port.prepared.Input.Templates = append(fixture.port.prepared.Input.Templates, tobari.PredecessorTemplate{
			ID: id, Name: fmt.Sprintf("large-%02d", index), CurrentGeneration: 1, CurrentRevision: digest("d"),
			Revisions: []tobari.PredecessorTemplateRevision{{Generation: 1, Revision: digest("d"), Body: body}},
		})
	}

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := fixture.engine.Apply(context.Background()); err == nil {
			t.Fatal("over-bound final authority migrated")
		}
		if pathExists(fixture.engine.journalPath()) || pathExists(fixture.finalRoot) || pathExists(fixture.finalRoot+finalStageSuffix) || !pathExists(fixture.cutoff) {
			t.Fatal("over-bound rejection created journal, stage, final authority, or moved cutoff")
		}
	}
}

func TestEngineUsesSameFilesystemSiblingTargetsAcrossIndependentRoots(t *testing.T) {
	fixture := newMigrationFixture(t, true)
	seen := map[string]bool{}
	sourcePaths := map[string]bool{}
	for _, source := range fixture.prepared.Sources {
		sourcePaths[source.Path] = true
	}
	original := fixture.engine.rename
	fixture.engine.rename = func(source, target string) error {
		if sourcePaths[source] {
			if filepath.Dir(source) != filepath.Dir(target) {
				t.Fatalf("cross-parent authority rename: %s -> %s", source, target)
			}
			seen[source] = true
		}
		return original(source, target)
	}
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, source := range fixture.prepared.Sources {
		if !seen[source.Path] {
			t.Fatalf("source %s was not moved", source.Path)
		}
	}
}

func TestEngineLockSurvivesStaleFileAndExcludesConcurrentHolder(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if err := fixture.engine.ensureTransactionRoot(); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(fixture.transaction, lockFileName)
	writeFixtureFile(t, lockPath, []byte("stale process marker"))
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatalf("kernel-released stale lock file blocked Apply recovery: %v", err)
	}
	holder, err := fixture.engine.acquireLock()
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.Rollback(context.Background()); err == nil {
		t.Fatal("live concurrent migration holder was not excluded")
	}
	holder()
	if err := fixture.engine.Rollback(context.Background()); err != nil {
		t.Fatalf("released kernel lock blocked Rollback recovery: %v", err)
	}
	assertPredecessorSelected(t, fixture)
}

func TestEngineDurablyPublishesNewTransactionRootBeforeJournalOrAuthority(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	parentSyncCalls := 0
	fixture.engine.syncRootParent = func(string) error {
		parentSyncCalls++
		return errors.New("synthetic parent-directory fsync failure")
	}
	if _, err := fixture.engine.Apply(context.Background()); err == nil {
		t.Fatal("migration proceeded without durable transaction-root publication")
	}
	if parentSyncCalls != 1 {
		t.Fatalf("parent sync calls=%d", parentSyncCalls)
	}
	if pathExists(fixture.engine.journalPath()) || pathExists(fixture.finalRoot) || pathExists(fixture.finalRoot+finalStageSuffix) || !pathExists(fixture.cutoff) {
		t.Fatal("transaction-root durability failure reached journal or authority mutation")
	}

	fixture.engine.syncRootParent = syncDirectory
	if _, err := fixture.engine.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertFinalSelected(t, fixture)
}

func TestEngineResyncsExistingTransactionRootParentWithoutChangingContents(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if err := fixture.engine.ensureTransactionRoot(); err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, filepath.Join(fixture.transaction, "preserved"), []byte("unchanged"))
	parentSyncCalls := 0
	fixture.engine.syncRootParent = func(parent string) error {
		parentSyncCalls++
		return syncDirectory(parent)
	}
	if err := fixture.engine.ensureTransactionRoot(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(fixture.transaction, "preserved"))
	if err != nil || string(data) != "unchanged" || parentSyncCalls != 1 {
		t.Fatalf("existing root changed: data=%q calls=%d err=%v", data, parentSyncCalls, err)
	}
}

func TestConcurrentFirstRunCannotReachEffectsBeforeParentDurability(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	second, err := New(fixture.finalRoot, fixture.transaction, fixture.port)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	blockedSync := func(parent string) error {
		entered <- struct{}{}
		<-release
		return syncDirectory(parent)
	}
	fixture.engine.syncRootParent = blockedSync
	second.syncRootParent = blockedSync
	results := make(chan error, 2)
	for _, engine := range []*Engine{fixture.engine, second} {
		go func(engine *Engine) {
			_, err := engine.Apply(context.Background())
			results <- err
		}(engine)
	}
	<-entered
	<-entered
	if pathExists(fixture.engine.journalPath()) || pathExists(fixture.finalRoot) || pathExists(fixture.finalRoot+finalStageSuffix) || !pathExists(fixture.cutoff) {
		t.Fatal("concurrent first run reached journal or authority before parent durability")
	}
	close(release)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("neither concurrent invocation completed after parent durability")
	}
	assertFinalSelected(t, fixture)
}

func TestExistingTransactionRootStillFailsClosedWhenParentSyncFails(t *testing.T) {
	fixture := newMigrationFixture(t, false)
	if err := fixture.engine.ensureTransactionRoot(); err != nil {
		t.Fatal(err)
	}
	fixture.engine.syncRootParent = func(string) error {
		return errors.New("synthetic existing-root parent sync failure")
	}
	if _, err := fixture.engine.Apply(context.Background()); err == nil {
		t.Fatal("existing root advanced after parent durability failed")
	}
	if pathExists(fixture.engine.journalPath()) || pathExists(fixture.finalRoot) || !pathExists(fixture.cutoff) {
		t.Fatal("existing-root parent sync failure reached journal or authority")
	}
}

func prepareAndPublishCandidate(t *testing.T, fixture *migrationFixture) migrationJournal {
	t.Helper()
	if err := fixture.engine.ensureTransactionRoot(); err != nil {
		t.Fatal(err)
	}
	journal, err := fixture.engine.prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := fixture.engine.publishFinal(context.Background(), journal.Collection); err != nil {
		t.Fatal(err)
	}
	journal.Phase = phaseFinalPublished
	if err := fixture.engine.writeJournal(journal); err != nil {
		t.Fatal(err)
	}
	return journal
}

func assertFinalSelected(t *testing.T, fixture *migrationFixture) {
	t.Helper()
	disposition, err := fixture.port.ObserveReaders(context.Background())
	if err != nil || !disposition.PredecessorUnavailable || !disposition.FinalComplete || disposition.PredecessorComplete || disposition.FinalAbsent {
		t.Fatalf("final disposition=%#v err=%v", disposition, err)
	}
}

func assertPredecessorSelected(t *testing.T, fixture *migrationFixture) {
	t.Helper()
	disposition, err := fixture.port.ObserveReaders(context.Background())
	if err != nil || !disposition.PredecessorComplete || !disposition.FinalAbsent || disposition.PredecessorUnavailable || disposition.FinalComplete {
		t.Fatalf("predecessor disposition=%#v err=%v", disposition, err)
	}
}

func assertPredecessorSelectedWithFinalCandidate(t *testing.T, fixture *migrationFixture) {
	t.Helper()
	disposition, err := fixture.port.ObserveReaders(context.Background())
	if err != nil || !disposition.PredecessorComplete || disposition.PredecessorUnavailable || disposition.FinalComplete || disposition.FinalAbsent {
		t.Fatalf("predecessor+candidate disposition=%#v err=%v", disposition, err)
	}
}

func assertHomeBytes(t *testing.T, fixture *migrationFixture) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixture.home, "auth.json"))
	if err != nil || string(data) != string(fixture.homeBytes) {
		t.Fatalf("standard home changed: %q err=%v", data, err)
	}
}

func fixtureSource(t *testing.T, key, path, backup string, cutoff, research bool) SourceItem {
	t.Helper()
	digest, kind, err := digestOwnedPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return SourceItem{Key: key, Path: path, BackupPath: backup, Kind: kind, Digest: digest, Cutoff: cutoff, Research: research}
}

func migrationTemplateBody() tobari.WorkspaceTemplateBody {
	return tobari.WorkspaceTemplateBody{
		Boundary: tobari.WorkspaceTemplateBoundary{
			SourceAccess:       tobari.ManifestSourceAccessReadOnly,
			DestinationCeiling: tobari.ManifestPolicyDestinationCeiling{Mode: "exact", Authorities: []tobari.ManifestPolicyAuthority{{Scheme: "https", Host: "api.example.dev", Port: 443}}},
			MethodPolicy:       tobari.ManifestMethodPolicy{Default: tobari.ManifestMethodExactReview, Overrides: []tobari.ManifestMethodOverride{{Method: "GET", Decision: tobari.ManifestMethodAllow}}},
		},
		Policy: tobari.WorkspaceTemplatePolicyBody{
			AgentProfile: tobari.DefaultProfile, Mode: tobari.ManifestPolicyModeGuided, NativeReadiness: tobari.ManifestNativeReadinessEnabled,
			BaselineGrants:    []tobari.ManifestPolicyExactRule{{Scheme: "https", Host: "api.example.dev", Port: 443, Method: "GET", Path: "/items"}},
			BaselineTemplates: []tobari.ManifestPolicyPathTemplateRule{}, MCPBaselineGrants: []tobari.ManifestPolicyMCPRule{},
			BaselineDenies: []tobari.ManifestPolicyExactRule{}, GraphQLEndpoints: []tobari.ManifestPolicyExactRule{}, MCPEndpoints: []tobari.ManifestPolicyExactRule{},
		},
		EntryDefaults: tobari.WorkspaceTemplateEntryDefaults{Runtime: tobari.RuntimeBinding{
			RuntimeID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Revision: string(digest("f")), Ordinal: 1, Image: tobari.OfficialRuntimeBase,
		}},
		SessionDefaults:  tobari.WorkspaceTemplateSessionDefaults{ShellEnvironment: []tobari.ManifestShellEnvironmentSetting{}},
		CreationDefaults: tobari.WorkspaceTemplateCreationDefaults{},
	}
}

func predecessorBody(body tobari.WorkspaceTemplateBody) tobari.PredecessorTemplateBody {
	return tobari.PredecessorTemplateBody{
		Boundary: body.Boundary.Clone(), Policy: body.Policy.Clone(), EntryDefaults: body.EntryDefaults,
		SessionDefaults: body.SessionDefaults.Clone(), CreationDefaults: body.CreationDefaults.Clone(),
	}
}

func digest(character string) tobari.SemanticDigest {
	return tobari.SemanticDigest("sha256:" + strings.Repeat(character, 64))
}

func stringPointer(value string) *string { return &value }

func writeFixtureFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureFileReplace(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
