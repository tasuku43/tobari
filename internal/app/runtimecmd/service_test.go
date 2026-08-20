package runtimecmd

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeFake struct {
	manifest tobari.RuntimeManifest
	creates  int
	builds   int
}

func runtimeFixture() tobari.RuntimeManifest {
	return tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Kind: tobari.RuntimeKindManaged, SourcePath: "/tmp/tobari/runtimes/frontend/source", Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: "sha256:" + strings.Repeat("a", 64), Image: "tobari-runtime-frontend:aaaaaaaaaaaa", ImageDigest: "sha256:" + strings.Repeat("b", 64), CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/frontend/revisions/aaaaaaaa/source"}}}
}

func (f *runtimeFake) ListRuntimes(context.Context) (tobari.RuntimeListResult, error) {
	return tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: []tobari.RuntimeSummary{tobari.RuntimeSummaryFrom(f.manifest)}}, nil
}
func (f *runtimeFake) ShowRuntime(context.Context, string) (tobari.RuntimeReport, error) {
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeShow, Runtime: f.manifest}, nil
}
func (f *runtimeFake) RuntimeHistory(context.Context, string) (tobari.RuntimeReport, error) {
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeHistory, Runtime: f.manifest}, nil
}
func (f *runtimeFake) CreateRuntime(context.Context, string) (tobari.RuntimeReport, error) {
	f.creates++
	manifest := f.manifest
	manifest.Revisions = []tobari.RuntimeRevision{}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}, nil
}
func (f *runtimeFake) BuildManagedRuntime(_ context.Context, _ string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	f.builds++
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "build\n")
	}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: f.manifest, Built: true}, nil
}

func TestRuntimeCreateAndBuildUseCatalogFixedTargets(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	service := New(fake)
	createIntent := operation.Intent{Command: "runtime create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}
	created, err := service.Create(context.Background(), createIntent, "frontend")
	if err != nil || !created.Created || fake.creates != 1 {
		t.Fatalf("create = %+v/%v calls=%d", created, err, fake.creates)
	}

	buildIntent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ID: tobari.RuntimeCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	var diagnostics strings.Builder
	built, err := service.Build(context.Background(), buildIntent, "frontend", &diagnostics)
	if err != nil || !built.Built || fake.builds != 1 || diagnostics.String() != "build\n" {
		t.Fatalf("build = %+v/%v calls=%d diagnostics=%q", built, err, fake.builds, diagnostics.String())
	}
}

func TestRuntimeMutationRejectsWrongTargetBeforeAdapter(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	service := New(fake)
	intent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ContextRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	if _, err := service.Build(context.Background(), intent, "frontend", nil); err == nil || fake.builds != 0 {
		t.Fatalf("wrong target error/calls = %v/%d", err, fake.builds)
	}
}
