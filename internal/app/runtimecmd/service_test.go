package runtimecmd

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type runtimeFake struct {
	manifest  tobari.RuntimeManifest
	creates   int
	base      tobari.RuntimeCopySource
	builds    int
	resolves  int
	buildErr  error
	buildName string
	createErr error
}

func runtimeFixture() tobari.RuntimeManifest {
	return tobari.RuntimeManifest{SchemaVersion: tobari.RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077", Name: "frontend", Kind: tobari.RuntimeKindManaged, SourcePath: "/tmp/tobari/runtimes/frontend/source", Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: "sha256:" + strings.Repeat("a", 64), Image: "tobari-runtime-frontend:aaaaaaaaaaaa", ImageDigest: "sha256:" + strings.Repeat("b", 64), CreatedAt: time.Unix(1, 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/frontend/revisions/aaaaaaaa/source"}}}
}

func standardRuntimeFixture() tobari.RuntimeManifest {
	return tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{Ordinal: 1, Revision: "sha256:" + strings.Repeat("c", 64), Image: "ghcr.io/example/tobari:standard", CreatedAt: time.Unix(1, 0).UTC()}},
	}
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
func (f *runtimeFake) CreateRuntime(_ context.Context, _ string, base tobari.RuntimeCopySource) (tobari.RuntimeReport, error) {
	f.creates++
	f.base = base
	if f.createErr != nil {
		return tobari.RuntimeReport{}, f.createErr
	}
	manifest := f.manifest
	manifest.Revisions = []tobari.RuntimeRevision{}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}, nil
}
func (f *runtimeFake) ResolveRuntimeReference(_ context.Context, reference string) (tobari.RuntimeManifest, error) {
	f.resolves++
	if reference == tobari.StandardRuntimeID {
		return standardRuntimeFixture(), nil
	}
	if reference != tobari.RuntimeRef(f.manifest.ID) {
		return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
	}
	return f.manifest, nil
}

func TestRuntimeBuildRejectsStandardReferenceBeforeBuildAdapter(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	intent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: tobari.StandardRuntimeID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	_, err := New(fake).Build(context.Background(), intent, tobari.StandardRuntimeID, nil)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "runtime_not_managed" || fake.resolves != 0 || fake.builds != 0 {
		t.Fatalf("standard build fault/resolve/build calls = %+v/%v/%d/%d", public, err, fake.resolves, fake.builds)
	}
}

func TestRuntimeReadsPublishOnlyReferenceKindsWithConsumers(t *testing.T) {
	manifest := runtimeFixture()
	manifest.Revisions[0].RevisionRef = tobari.RuntimeRevisionRef(manifest.ID, manifest.Revisions[0].Revision)
	fake := &runtimeFake{manifest: manifest}
	service := New(fake)
	listed, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shown, err := service.Show(context.Background(), manifest.Name)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].RuntimeRef != manifest.ID || listed.Items[0].RevisionRef != "" {
		t.Fatalf("Runtime list references = %+v", listed.Items)
	}
	if shown.Runtime.RuntimeRef != manifest.ID || shown.Runtime.Revisions[0].RuntimeRef != manifest.ID || shown.Runtime.Revisions[0].RevisionRef != "" {
		t.Fatalf("Runtime report references = %+v", shown.Runtime)
	}
}
func (f *runtimeFake) BuildManagedRuntimeByReference(_ context.Context, reference string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	f.builds++
	if reference != tobari.RuntimeRef(f.manifest.ID) {
		return tobari.RuntimeReport{}, tobari.ErrRuntimeNotFound
	}
	f.buildName = f.manifest.Name
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, "build\n")
	}
	if f.buildErr != nil {
		return tobari.RuntimeReport{}, f.buildErr
	}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: f.manifest, Built: true}, nil
}

func TestRuntimeCreateUsesCatalogScopeAndBuildUsesRuntimeReference(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	service := New(fake)
	createIntent := operation.Intent{Command: "runtime create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}
	created, err := service.Create(context.Background(), createIntent, "frontend", "standard")
	if err != nil || !created.Created || fake.creates != 1 || fake.base != tobari.RuntimeCopySource("standard") {
		t.Fatalf("create = %+v/%v calls=%d base=%q", created, err, fake.creates, fake.base)
	}

	buildIntent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: tobari.RuntimeRef(fake.manifest.ID)}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	var diagnostics strings.Builder
	built, err := service.Build(context.Background(), buildIntent, fake.manifest.ID, &diagnostics)
	if err != nil || !built.Built || fake.builds != 1 || fake.buildName != "frontend" || diagnostics.String() != "build\n" || built.Runtime.RuntimeRef != fake.manifest.ID {
		t.Fatalf("build = %+v/%v calls=%d name=%q diagnostics=%q", built, err, fake.builds, fake.buildName, diagnostics.String())
	}
}

func TestRuntimeCreateRejectsInvalidBaseBeforeAdapter(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	service := New(fake)
	intent := operation.Intent{Command: "runtime create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}
	_, err := service.Create(context.Background(), intent, "frontend", "frontend@1")
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "invalid_runtime_copy_source" || fake.creates != 0 {
		t.Fatalf("invalid Base fault/calls = %+v/%v/%d", public, err, fake.creates)
	}
}

func TestRuntimeCreateClassifiesMissingBaseAndPreservesSourceFault(t *testing.T) {
	intent := operation.Intent{Command: "runtime create", Effect: operation.EffectCreate, Target: operation.TargetRef{Kind: tobari.RuntimeCatalogTargetKind, ParentID: tobari.RuntimeCatalogTargetID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationNo, Destructive: operation.DeclarationNo}}
	for _, test := range []struct {
		name string
		err  error
		code string
	}{
		{name: "missing", err: tobari.ErrRuntimeNotFound, code: "runtime_copy_source_not_found"},
		{name: "invalid source", err: fault.New(fault.KindRejected, "runtime_source_invalid", "Runtime source changed during copy.", false), code: "runtime_source_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := &runtimeFake{manifest: runtimeFixture(), createErr: test.err}
			_, err := New(fake).Create(context.Background(), intent, "mobile", "frontend")
			public, ok := fault.PublicCopy(err)
			if !ok || public.Code != test.code {
				t.Fatalf("create fault = %+v/%v", public, err)
			}
		})
	}
}

func TestRuntimeMutationRejectsWrongTargetBeforeAdapter(t *testing.T) {
	fake := &runtimeFake{manifest: runtimeFixture()}
	service := New(fake)
	intent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.ManifestRuntimeTargetKind, ID: tobari.ActiveContextRuntimeID}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}
	if _, err := service.Build(context.Background(), intent, fake.manifest.ID, nil); err == nil || fake.builds != 0 {
		t.Fatalf("wrong target error/calls = %v/%d", err, fake.builds)
	}
}

func TestRuntimeBuildPreservesReviewedSourceValidationFault(t *testing.T) {
	privateCause := errors.New("private source validation cause")
	fake := &runtimeFake{manifest: runtimeFixture(), buildErr: fault.Wrap(
		fault.KindRejected,
		"runtime_source_invalid",
		"Runtime source file \"bin/tool\" is 33554433 bytes; the limit is 33554432 bytes (32 MiB).",
		false,
		privateCause,
	)}
	service := New(fake)
	intent := operation.Intent{Command: "runtime build", Effect: operation.EffectWrite, Target: operation.TargetRef{Kind: tobari.RuntimeReferenceKind, ID: tobari.RuntimeRef(fake.manifest.ID)}, Impact: operation.Impact{Cardinality: operation.CardinalityOne, Notification: operation.DeclarationNo, AccessChange: operation.DeclarationYes, Destructive: operation.DeclarationNo}}

	_, err := service.Build(context.Background(), intent, fake.manifest.ID, nil)
	public, ok := fault.PublicCopy(err)
	if !ok || public.Code != "runtime_source_invalid" || public.Kind != fault.KindRejected || public.Retryable || strings.Contains(public.Message, privateCause.Error()) {
		t.Fatalf("public source fault = %+v/%v", public, err)
	}
}
