package workspaceauthoritysource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestConfiguratorStageReceiptPrecedesReplacementAndMakesItReplaySafe(t *testing.T) {
	ctx := context.Background()
	store, err := New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	initial := sourceTemplateFixture(t)
	if err := store.PublishTemplate(ctx, initial); err != nil {
		t.Fatal(err)
	}
	_, baseFingerprint, _, err := store.ReadTemplateSnapshot(ctx, sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	body, err := initial.Body(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	active, err := tobari.NewWorkspaceTemplateRevision(sourceTemplateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewDetachedEvolveConfiguratorSeed("/workspace/example", active, sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, sourceTemplateID, "01912345-6789-7abc-8def-0123456789ac")
	if err != nil {
		t.Fatal(err)
	}
	body.Boundary.SourceAccess = tobari.ManifestSourceAccessReadWrite
	proposed, err := tobari.NewWorkspaceTemplateDraftSource(sourceTemplateID, initial.Template.Name, body)
	if err != nil {
		t.Fatal(err)
	}
	base := draft.BaseTemplateRevision
	proposed.Template.BaseRevision = &base
	body, err = proposed.Body(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	revision, err := proposed.SemanticRevision(sourceRuntimeBindingFixture())
	if err != nil {
		t.Fatal(err)
	}
	submission, err := tobari.NewConfiguratorSubmission(draft, body, revision)
	if err != nil {
		t.Fatal(err)
	}
	postFingerprint, err := store.ConfiguratorTemplateFingerprint(proposed)
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := tobari.WorkspaceTemplateRef(sourceTemplateID)
	stage := tobari.ConfiguratorStage{SchemaVersion: tobari.ConfiguratorStageSchemaVersion, TemplateRef: ref, SourceRevision: revision, SourceFingerprint: postFingerprint}
	receipt := ConfiguratorStageReceipt{Submission: submission, Stage: stage, BaseFingerprint: baseFingerprint}
	if err := store.BeginConfiguratorStage(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	ids, err := store.ListConfiguratorStageIDs(ctx)
	if err != nil || !reflect.DeepEqual(ids, []tobari.WorkspaceTemplateID{sourceTemplateID}) {
		t.Fatalf("stage index=%v err=%v", ids, err)
	}
	if err := store.ReplaceTemplateSnapshot(ctx, proposed, baseFingerprint); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceTemplateSnapshot(ctx, proposed, baseFingerprint); err != nil {
		t.Fatalf("same post-crash replacement was not replay-safe: %v", err)
	}
	planRef := "wtplan1_" + string(sourceTemplateID) + "_" + strings.Repeat("c", 64)
	receipt, err = store.BindConfiguratorStagePlan(ctx, receipt, planRef)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err = store.ConfirmConfiguratorStageApply(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	observed, present, err := store.ReadConfiguratorStage(ctx, sourceTemplateID)
	if err != nil || !present || !reflect.DeepEqual(observed, receipt) {
		t.Fatalf("stage receipt=%+v present=%v err=%v", observed, present, err)
	}
	if err := store.ClearConfiguratorStage(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	if ids, err := store.ListConfiguratorStageIDs(ctx); err != nil || len(ids) != 0 {
		t.Fatalf("settled stage index=%v err=%v", ids, err)
	}
	if err := store.BeginConfiguratorPublication(ctx, submission); err != nil {
		t.Fatal(err)
	}
	if observed, present, err := store.ReadConfiguratorPublication(ctx, sourceTemplateID); err != nil || !present || !reflect.DeepEqual(observed, submission) {
		t.Fatalf("publication barrier=%+v present=%v err=%v", observed, present, err)
	}
	if err := store.BeginConfiguratorPublication(ctx, submission); err != nil {
		t.Fatalf("publication barrier was not replay-safe: %v", err)
	}
	if err := store.CompleteConfiguratorPublication(ctx, submission); err != nil {
		t.Fatal(err)
	}
	if _, present, err := store.ReadConfiguratorPublication(ctx, sourceTemplateID); err != nil || present {
		t.Fatalf("publication barrier remained after settlement: present=%v err=%v", present, err)
	}
}

func TestConfiguratorStageLeaseExcludesConcurrentMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback serializes through the process-level adapter path")
	}
	store, err := New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireConfiguratorStageLease(context.Background(), sourceTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := store.AcquireConfiguratorStageLease(context.Background(), sourceTemplateID); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("concurrent stage lease error=%v", err)
	}
}

func TestConfiguratorProjectStageLeaseExcludesOnlySameProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback serializes through the process-level adapter path")
	}
	store, err := New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireConfiguratorProjectStageLease(context.Background(), "/workspace/example")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := store.AcquireConfiguratorProjectStageLease(context.Background(), "/workspace/example"); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("same-Project stage lease error=%v", err)
	}
	other, err := store.AcquireConfiguratorProjectStageLease(context.Background(), "/workspace/other")
	if err != nil {
		t.Fatalf("different Project was unnecessarily blocked: %v", err)
	}
	if err := other(); err != nil {
		t.Fatal(err)
	}
}

func TestConfiguratorCatalogLeaseExcludesConcurrentDefaultBarrierChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows fallback serializes through the process-level adapter path")
	}
	store, err := New(filepath.Join(t.TempDir(), "sources"))
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireConfiguratorCatalogLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := store.AcquireConfiguratorCatalogLease(context.Background()); !errors.Is(err, tobari.ErrContextBindingProtected) {
		t.Fatalf("concurrent catalog lease error=%v", err)
	}
}

func TestConfiguratorCatalogLeaseRejectsSymlinkWithoutCreatingTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not portable on Windows")
	}
	root := filepath.Join(t.TempDir(), "sources")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireConfiguratorCatalogLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, ".configurator-catalog-locks", "catalog.lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.Symlink(target, lock); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireConfiguratorCatalogLease(context.Background()); err == nil {
		t.Fatal("symlinked Configurator catalog lock was accepted")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("symlink target was created or changed: %v", err)
	}
}

func TestConfiguratorCatalogLeaseRejectsHardlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hardlink ownership metadata is not portable on Windows")
	}
	root := filepath.Join(t.TempDir(), "sources")
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireConfiguratorCatalogLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(root, ".configurator-catalog-locks", "catalog.lock")
	outside := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.Link(lock, outside); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AcquireConfiguratorCatalogLease(context.Background()); err == nil {
		t.Fatal("hardlinked Configurator catalog lock was accepted")
	}
	after, err := os.Stat(outside)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || before.Mode() != after.Mode() || !os.SameFile(before, after) {
		t.Fatalf("hardlink target changed: before=%v after=%v", before, after)
	}
}

func TestConfiguratorReceiptDirectoryCreationSyncsEveryOwningParent(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "configuration")
	receipts := filepath.Join(root, ".configurator-publications")
	var synced []string
	if err := ensureDurableConfiguratorDirectoryTree(receipts, func(path string) error {
		synced = append(synced, path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{parent, root} {
		found := false
		for _, path := range synced {
			found = found || path == required
		}
		if !found {
			t.Fatalf("new receipt tree did not sync owning parent %q: %v", required, synced)
		}
	}
}
