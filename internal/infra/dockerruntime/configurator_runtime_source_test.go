package dockerruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestConfiguratorManagedRuntimeSourceFreezesAndBuildsExactRevision(t *testing.T) {
	runtime, draft, sourcePath := configuratorManagedRuntimeFixture(t)
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, _, _ := runtime.configuratorRuntimeSourcePaths(draft)
	dockerfile := filepath.Join(working, "Dockerfile")
	file, err := os.OpenFile(dockerfile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nRUN true\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	frozen, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if frozen == nil || !frozen.Changed || frozen.BaseRevision == frozen.FrozenRevision {
		t.Fatalf("frozen=%+v", frozen)
	}
	binding, err := runtime.ApplyConfiguratorRuntimeSource(context.Background(), draft, *frozen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if binding.RuntimeID != draft.Runtime.RuntimeID || binding.Revision != string(frozen.FrozenRevision) {
		t.Fatalf("built binding=%+v frozen=%+v", binding, frozen)
	}
	if observed, err := digestRuntimeSnapshot(context.Background(), sourcePath); err != nil || observed != binding.Revision {
		t.Fatalf("canonical source digest=%q err=%v want=%q", observed, err, binding.Revision)
	}
}

func TestConfiguratorManagedRuntimeFreezeResumesDurableEmptyFrozenRoot(t *testing.T) {
	runtime, draft, _ := configuratorManagedRuntimeFixture(t)
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, frozenRoot, err := runtime.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(filepath.Join(working, "Dockerfile"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\nRUN true\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ensureDurableConfiguratorFrozenRoot(frozenRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatalf("durable empty frozen-root residue was not replay-safe: %v", err)
	}
}

func TestConfiguratorManagedRuntimeSourceRejectsCanonicalCASDrift(t *testing.T) {
	runtime, draft, sourcePath := configuratorManagedRuntimeFixture(t)
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, _, _ := runtime.configuratorRuntimeSourcePaths(draft)
	if err := os.WriteFile(filepath.Join(working, "agent.txt"), []byte("agent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frozen, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "concurrent.txt"), []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyConfiguratorRuntimeSource(context.Background(), draft, *frozen, nil); !errors.Is(err, tobari.ErrResourceSourceChanged) {
		t.Fatalf("Runtime source CAS drift error=%v", err)
	}
}

func TestRuntimeAssistPublishesUnbuiltTargetSourceWithoutBuildingRevision(t *testing.T) {
	root := t.TempDir()
	runner := newManagedRuntimeBuildRunner()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	target, err := runtime.CreateRuntime(context.Background(), "unbuilt-tools", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{
		Context:      tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID},
		Template:     tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}},
		PolicyMemory: memory,
	}
	targetBase, err := digestRuntimeSnapshot(context.Background(), target.Runtime.SourcePath)
	if err != nil {
		t.Fatal(err)
	}
	seed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, target.Runtime.ID, tobari.SemanticDigest(targetBase))
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	home, err := runtime.configuratorHome(draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	configuratorRoot, err := runtime.ConfiguratorRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configuratorRoot, draft.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, _, err := runtime.configuratorRuntimeSourcePaths(draft)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(working, "agent.txt"), []byte("reviewed source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frozen, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), draft)
	if err != nil || frozen == nil || !frozen.Changed {
		t.Fatalf("frozen source=%+v err=%v", frozen, err)
	}
	if len(target.Runtime.Revisions) != 0 {
		t.Fatalf("new Runtime unexpectedly built before assistance: %+v", target.Runtime.Revisions)
	}
	if published, err := runtime.ConfiguratorRuntimeSourcePublished(context.Background(), draft, *frozen); err != nil || published {
		t.Fatalf("unpublished Runtime source reported published=%v err=%v", published, err)
	}
	if err := runtime.ApplyConfiguratorRuntimeSourceOnly(context.Background(), draft, *frozen); err != nil {
		t.Fatal(err)
	}
	published, err := runtime.ResolveRuntimeReference(context.Background(), tobari.RuntimeRef(target.Runtime.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(published.Revisions) != 0 || len(runner.runs) != 0 {
		t.Fatalf("source-only publication built Runtime authority: revisions=%+v docker_calls=%+v", published.Revisions, runner.runs)
	}
	if observed, err := digestRuntimeSnapshot(context.Background(), published.SourcePath); err != nil || observed != string(frozen.FrozenRevision) {
		t.Fatalf("published source digest=%q want=%q err=%v", observed, frozen.FrozenRevision, err)
	}
	if err := runtime.ApplyConfiguratorRuntimeSourceOnly(context.Background(), draft, *frozen); err != nil {
		t.Fatalf("exact source-only replay failed: %v", err)
	}
	if published, err := runtime.ConfiguratorRuntimeSourcePublished(context.Background(), draft, *frozen); err != nil || !published {
		t.Fatalf("published Runtime source was not authority-verified: published=%v err=%v", published, err)
	}
	nextBase, err := runtime.ObserveManagedRuntimeSourceRevision(context.Background(), tobari.RuntimeRef(target.Runtime.ID))
	if err != nil || nextBase != frozen.FrozenRevision {
		t.Fatalf("published source generation=%s want=%s err=%v", nextBase, frozen.FrozenRevision, err)
	}
	nextSeed, err := tobari.NewRuntimeAssistConfiguratorSeed(snapshot.Template.Current.Body.EntryDefaults.Runtime, target.Runtime.ID, nextBase)
	if err != nil {
		t.Fatal(err)
	}
	nextDraft, err := tobari.NewConfiguratorDraft(nextSeed, tobari.ConfiguratorAgentCodex, "")
	if err != nil {
		t.Fatal(err)
	}
	if nextDraft.ID == draft.ID {
		t.Fatal("published Runtime source generation reused the previous task draft")
	}
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), nextDraft); err != nil {
		t.Fatalf("next Runtime assist generation did not start from published source: %v", err)
	}
	nextFrozen, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), nextDraft)
	if err != nil || nextFrozen == nil || nextFrozen.Changed || nextFrozen.BaseRevision != nextBase || nextFrozen.FrozenRevision != nextBase {
		t.Fatalf("next Runtime assist source=%+v err=%v", nextFrozen, err)
	}
	if err := os.WriteFile(filepath.Join(published.SourcePath, "concurrent.txt"), []byte("drift\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ApplyConfiguratorRuntimeSourceOnly(context.Background(), nextDraft, *nextFrozen); !errors.Is(err, tobari.ErrResourceSourceChanged) {
		t.Fatalf("unchanged reviewed source ignored concurrent canonical drift: %v", err)
	}
}

func TestConfiguratorManagedRuntimePrepareResumesAgentEditedWorkingTree(t *testing.T) {
	runtime, draft, _ := configuratorManagedRuntimeFixture(t)
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, _, _ := runtime.configuratorRuntimeSourcePaths(draft)
	if err := os.WriteFile(filepath.Join(working, "agent.txt"), []byte("retained edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := digestRuntimeSnapshot(context.Background(), working)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatalf("edited working tree did not resume: %v", err)
	}
	after, err := digestRuntimeSnapshot(context.Background(), working)
	if err != nil || after != before {
		t.Fatalf("resumed digest=%q want=%q err=%v", after, before, err)
	}
}

func TestConfiguratorManagedRuntimePrepareSettlesEitherCrashHalf(t *testing.T) {
	for _, test := range []struct {
		name       string
		removePath func(working, metadata string) string
	}{
		{name: "working committed before metadata", removePath: func(_, metadata string) string { return metadata }},
		{name: "metadata committed before working", removePath: func(working, _ string) string { return working }},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime, draft, _ := configuratorManagedRuntimeFixture(t)
			if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
				t.Fatal(err)
			}
			working, metadata, _, err := runtime.configuratorRuntimeSourcePaths(draft)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(test.removePath(working, metadata)); err != nil {
				t.Fatal(err)
			}
			if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
				t.Fatalf("interrupted prepare did not settle: %v", err)
			}
			stored, err := readConfiguratorRuntimeSourceMetadata(metadata)
			if err != nil || stored.validateFor(draft) != nil {
				t.Fatalf("settled metadata=%+v err=%v", stored, err)
			}
			if observed, err := digestRuntimeSnapshot(context.Background(), working); err != nil || observed != string(stored.BaseRevision) {
				t.Fatalf("settled working digest=%q metadata=%q err=%v", observed, stored.BaseRevision, err)
			}
		})
	}
}

func TestConfiguratorManagedRuntimeBuildRemainsBoundToFrozenRevisionAfterPromotionDrift(t *testing.T) {
	runtime, draft, sourcePath := configuratorManagedRuntimeFixture(t)
	if err := runtime.PrepareConfiguratorRuntimeSource(context.Background(), draft); err != nil {
		t.Fatal(err)
	}
	working, _, _, _ := runtime.configuratorRuntimeSourcePaths(draft)
	if err := os.WriteFile(filepath.Join(working, "agent.txt"), []byte("reviewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	frozen, err := runtime.FreezeConfiguratorRuntimeSource(context.Background(), draft)
	if err != nil {
		t.Fatal(err)
	}
	runtime.configuratorRuntimeAfterPromotion = func() {
		if err := os.WriteFile(filepath.Join(sourcePath, "concurrent.txt"), []byte("later edit\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binding, err := runtime.ApplyConfiguratorRuntimeSource(context.Background(), draft, *frozen, nil)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Revision != string(frozen.FrozenRevision) {
		t.Fatalf("built revision=%q want frozen=%q", binding.Revision, frozen.FrozenRevision)
	}
	if observed, err := digestRuntimeSnapshot(context.Background(), sourcePath); err != nil || observed == binding.Revision {
		t.Fatalf("test did not create canonical post-promotion drift: digest=%q err=%v", observed, err)
	}
	replayed, err := runtime.ApplyConfiguratorRuntimeSource(context.Background(), draft, *frozen, nil)
	if err != nil {
		t.Fatalf("published frozen Runtime did not replay after canonical drift: %v", err)
	}
	if replayed != binding {
		t.Fatalf("replayed binding=%+v want=%+v", replayed, binding)
	}
}

func configuratorManagedRuntimeFixture(t *testing.T) (*Runtime, tobari.ConfiguratorDraft, string) {
	t.Helper()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), newManagedRuntimeBuildRunner())
	if err != nil {
		t.Fatal(err)
	}
	created, err := runtime.CreateRuntime(context.Background(), "project-tools", tobari.RuntimeCopySource(tobari.StandardRuntimeName))
	if err != nil {
		t.Fatal(err)
	}
	built, err := runtime.BuildManagedRuntime(context.Background(), created.Runtime.Name, nil)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := built.Runtime.Binding(1)
	if err != nil {
		t.Fatal(err)
	}
	body := configuratorRuntimeBodyFixture()
	body.EntryDefaults.Runtime = binding
	templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789ab")
	contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789ac")
	revision, err := tobari.NewWorkspaceTemplateRevision(templateID, 1, body)
	if err != nil {
		t.Fatal(err)
	}
	template := tobari.WorkspaceTemplate{SchemaVersion: tobari.WorkspaceTemplateSchemaVersion, ID: templateID, Name: tobari.DefaultManifestName, Current: revision, Retained: []tobari.WorkspaceTemplateRevision{revision.Clone()}}
	memory, _, err := tobari.PublishPolicyMemory(contextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := tobari.ContextAuthoritySnapshot{Context: tobari.ContextBinding{SchemaVersion: tobari.ContextBindingSchemaVersion, ID: contextID, TemplateID: templateID}, Template: template, PolicyMemory: memory}
	seed, err := tobari.NewEvolveConfiguratorSeed("/workspace/example", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tobari.NewConfiguratorDraft(seed, tobari.ConfiguratorAgentCodex, templateID)
	if err != nil {
		t.Fatal(err)
	}
	home, err := runtime.finalContextHome(contextID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	configuratorRoot, err := runtime.ConfiguratorRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(configuratorRoot, draft.ID), 0o700); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(created.Runtime.SourcePath, string(created.Runtime.ID)) {
		t.Fatal("managed Runtime source path lost owner")
	}
	return runtime, draft, created.Runtime.SourcePath
}
