package completioncmd

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type completionRuntimeFake struct {
	contexts     tobari.ManifestListResult
	runtimes     tobari.RuntimeListResult
	histories    map[string]tobari.RuntimeReport
	contextErr   error
	runtimeErr   error
	historyErr   error
	listCalls    int
	historyCalls int
}

func (f *completionRuntimeFake) ListContexts(context.Context) (tobari.ManifestListResult, error) {
	return f.contexts, f.contextErr
}

func (f *completionRuntimeFake) ListRuntimes(context.Context) (tobari.RuntimeListResult, error) {
	f.listCalls++
	return f.runtimes, f.runtimeErr
}

func (f *completionRuntimeFake) RuntimeHistory(_ context.Context, name string) (tobari.RuntimeReport, error) {
	f.historyCalls++
	if f.historyErr != nil {
		return tobari.RuntimeReport{}, f.historyErr
	}
	return f.histories[name], nil
}

func completionRuntimeManifest(name string, revisions int) tobari.RuntimeManifest {
	items := make([]tobari.RuntimeRevision, revisions)
	for index := range items {
		digestRune := string(rune('a' + index))
		imageDigestRune := string(rune('c' + index))
		items[index] = tobari.RuntimeRevision{
			Ordinal: index + 1, Revision: "sha256:" + strings.Repeat(digestRune, 64),
			Image: "tobari-runtime-" + name + ":revision", ImageDigest: "sha256:" + strings.Repeat(imageDigestRune, 64),
			CreatedAt: time.Unix(int64(index+1), 0).UTC(), SnapshotPath: "/tmp/tobari/runtimes/" + name + "/revision/source",
		}
	}
	return tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion, ID: "018bcfe5-687b-7000-8000-000000000077",
		Name: name, Kind: tobari.RuntimeKindManaged, SourcePath: "/tmp/tobari/runtimes/" + name + "/source", Revisions: items,
	}
}

func completionRuntimeFixture() *completionRuntimeFake {
	managed := completionRuntimeManifest("sre", 2)
	standard := tobari.RuntimeSummary{
		ID: tobari.StandardRuntimeID, Name: tobari.StandardRuntimeName, Kind: tobari.RuntimeKindBuiltin,
		RuntimeRef: tobari.StandardRuntimeID, Ready: true, Head: 1, Revision: "sha256:" + strings.Repeat("9", 64),
	}
	standard.RevisionRef = tobari.RuntimeRevisionRef(standard.ID, standard.Revision)
	return &completionRuntimeFake{
		contexts: tobari.ManifestListResult{
			Task: tobari.TaskManifestList, ManifestState: tobari.ManifestObservationAbsent,
			Items: []tobari.ManifestSummary{},
		},
		runtimes:  tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: []tobari.RuntimeSummary{standard, tobari.RuntimeSummaryFrom(managed)}},
		histories: map[string]tobari.RuntimeReport{"sre": {Task: tobari.TaskRuntimeHistory, Runtime: managed}},
	}
}

func TestServiceReturnsTypedRuntimeCandidates(t *testing.T) {
	fake := completionRuntimeFixture()
	service := New(fake)

	names, err := service.Candidates(context.Background(), CandidateRuntimeName)
	if err != nil || !reflect.DeepEqual(names, []string{"standard", "sre"}) {
		t.Fatalf("runtime names = %v, %v", names, err)
	}
	managed, err := service.Candidates(context.Background(), CandidateManagedRuntimeName)
	if err != nil || !reflect.DeepEqual(managed, []string{"sre"}) {
		t.Fatalf("managed names = %v, %v", managed, err)
	}
	references, err := service.Candidates(context.Background(), CandidateReadyRuntimeReference)
	if err != nil || !reflect.DeepEqual(references, []string{"standard", "sre@2", "sre@1"}) {
		t.Fatalf("ready references = %v, %v", references, err)
	}
	if fake.historyCalls != 1 {
		t.Fatalf("history calls = %d, want 1", fake.historyCalls)
	}
}

func TestServiceReturnsNoManifestCandidateForAnAbsentCatalog(t *testing.T) {
	values, err := New(completionRuntimeFixture()).Candidates(context.Background(), CandidateManifestName)
	if err != nil || !reflect.DeepEqual(values, []string{}) {
		t.Fatalf("context candidates = %#v, %v", values, err)
	}
}

func TestServiceClassifiesReadAndCandidateFailures(t *testing.T) {
	t.Run("template read", func(t *testing.T) {
		fake := completionRuntimeFixture()
		fake.contextErr = errors.New("private path detail")
		_, err := New(fake).Candidates(context.Background(), CandidateManifestName)
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "completion_template_read_failed" || strings.Contains(public.Message, "private path detail") {
			t.Fatalf("fault = %+v, %v", public, err)
		}
	})
	t.Run("runtime read", func(t *testing.T) {
		fake := completionRuntimeFixture()
		fake.runtimeErr = errors.New("private path detail")
		_, err := New(fake).Candidates(context.Background(), CandidateRuntimeName)
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "completion_runtime_read_failed" || strings.Contains(public.Message, "private path detail") {
			t.Fatalf("fault = %+v, %v", public, err)
		}
	})
	t.Run("invalid kind", func(t *testing.T) {
		_, err := New(completionRuntimeFixture()).Candidates(context.Background(), CandidateKind("unknown"))
		public, ok := fault.PublicCopy(err)
		if !ok || public.Code != "invalid_completion_candidates" {
			t.Fatalf("fault = %+v, %v", public, err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		fake := completionRuntimeFixture()
		fake.runtimeErr = context.Canceled
		_, err := New(fake).Candidates(context.Background(), CandidateRuntimeName)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCandidateValidationRejectsTSVStructureAndDuplicates(t *testing.T) {
	for _, values := range [][]string{{""}, {"safe", "safe"}, {"unsafe\tvalue"}, {"unsafe\nvalue"}, {"unsafe\u2028value"}} {
		if err := validateCandidates(values); err == nil {
			t.Fatalf("values %#v passed validation", values)
		}
	}
}
