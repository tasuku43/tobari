package cli

import (
	"context"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// runtimeCatalogCLI is shared test support for the retained Runtime surface.
// The predecessor Context/Manifest CLI tests that originally lived in this
// file were removed with the final-only authority cut.
type runtimeCatalogCLI struct {
	buildLog              string
	buildErr              error
	buildCreates          bool
	manifest              tobari.RuntimeManifest
	snapshotManifest      *tobari.RuntimeManifest
	list                  []tobari.RuntimeSummary
	listCalls             int
	lifecycleReads        int
	replaceAfterRead      bool
	showCalls             int
	historyCalls          int
	buildCalls            int
	lastBuild             string
	createCalls           int
	lastCreate            string
	lastBase              tobari.RuntimeCopySource
	recovery              *tobari.RuntimeBuildRecovery
	recoveryErr           error
	recoveryReads         int
	recoveries            int
	recoveredRef          string
	recoveredKind         tobari.RuntimeBuildRecoveryKind
	lifecycleActivities   []tobari.RuntimeLifecycleActivity
	lifecycleAvailability tobari.RuntimeAvailability
	lifecycleErr          error
}

func testRuntimeManifest() tobari.RuntimeManifest {
	return tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion,
		ID:            "018bcfe5-687b-7000-8000-000000000077",
		Name:          "frontend",
		Kind:          tobari.RuntimeKindManaged,
		SourcePath:    "/config/runtimes/frontend/source",
		Revisions:     []tobari.RuntimeRevision{},
	}
}

func readyRuntimeManifest() tobari.RuntimeManifest {
	manifest := testRuntimeManifest()
	manifest.Revisions = []tobari.RuntimeRevision{{
		Ordinal:      1,
		Revision:     "sha256:" + strings.Repeat("a", 64),
		Image:        "tobari-runtime-frontend:aaaaaaaaaaaa",
		ImageDigest:  "sha256:" + strings.Repeat("b", 64),
		CreatedAt:    time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		SnapshotPath: "/config/runtimes/frontend/snapshots/aaaaaaaaaaaa",
	}}
	return manifest
}

func readyRuntimeManifestWithHistory() tobari.RuntimeManifest {
	manifest := readyRuntimeManifest()
	manifest.Revisions = append(manifest.Revisions, tobari.RuntimeRevision{
		Ordinal:      2,
		Revision:     "sha256:" + strings.Repeat("c", 64),
		Image:        "tobari-runtime-frontend:cccccccccccc",
		ImageDigest:  "sha256:" + strings.Repeat("d", 64),
		CreatedAt:    time.Date(2026, 8, 20, 1, 0, 0, 0, time.UTC),
		SnapshotPath: "/config/runtimes/frontend/snapshots/cccccccccccc",
	})
	return manifest
}

func runtimeReviewList(manifest tobari.RuntimeManifest) []tobari.RuntimeSummary {
	standard := tobari.RuntimeSummary{
		ID:         tobari.StandardRuntimeID,
		RuntimeRef: tobari.StandardRuntimeID,
		Name:       tobari.StandardRuntimeName,
		Kind:       tobari.RuntimeKindBuiltin,
		Ready:      true,
		Head:       1,
		Revision:   "sha256:" + strings.Repeat("0", 64),
	}
	return []tobari.RuntimeSummary{standard, tobari.RuntimeSummaryFrom(manifest)}
}

func (f *runtimeCatalogCLI) runtimeManifest() tobari.RuntimeManifest {
	if f.manifest.SchemaVersion != 0 {
		return f.manifest
	}
	return testRuntimeManifest()
}

func (f *runtimeCatalogCLI) ListRuntimes(context.Context) (tobari.RuntimeListResult, error) {
	f.listCalls++
	items := f.list
	if items == nil {
		items = []tobari.RuntimeSummary{}
	}
	return tobari.RuntimeListResult{Task: tobari.TaskRuntimeList, Items: items}, nil
}

func (f *runtimeCatalogCLI) ShowRuntime(context.Context, string) (tobari.RuntimeReport, error) {
	f.showCalls++
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeShow, Runtime: f.runtimeManifest()}, nil
}

func (f *runtimeCatalogCLI) RuntimeHistory(context.Context, string) (tobari.RuntimeReport, error) {
	f.historyCalls++
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeHistory, Runtime: f.runtimeManifest()}, nil
}

func (f *runtimeCatalogCLI) CreateRuntime(_ context.Context, name string, base tobari.RuntimeCopySource) (tobari.RuntimeReport, error) {
	f.createCalls++
	f.lastCreate = name
	f.lastBase = base
	manifest := f.runtimeManifest()
	manifest.Name = name
	manifest.Revisions = []tobari.RuntimeRevision{}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeCreate, Runtime: manifest, Created: true}, nil
}

func (f *runtimeCatalogCLI) ResolveRuntimeReference(_ context.Context, reference string) (tobari.RuntimeManifest, error) {
	manifest := f.runtimeManifest()
	if tobari.RuntimeRef(manifest.ID) != reference {
		return tobari.RuntimeManifest{}, tobari.ErrRuntimeNotFound
	}
	return manifest, nil
}

func (f *runtimeCatalogCLI) BuildManagedRuntimeByReference(_ context.Context, reference string, diagnostics io.Writer) (tobari.RuntimeReport, error) {
	f.buildCalls++
	manifest, err := f.ResolveRuntimeReference(context.Background(), reference)
	if err != nil {
		return tobari.RuntimeReport{}, err
	}
	f.lastBuild = manifest.Name
	if diagnostics != nil {
		_, _ = io.WriteString(diagnostics, f.buildLog)
	}
	if f.buildErr != nil {
		return tobari.RuntimeReport{}, f.buildErr
	}
	if f.buildCreates {
		return tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, Built: true}, nil
	}
	return tobari.RuntimeReport{Task: tobari.TaskRuntimeBuildV1, Runtime: manifest, NoChange: true}, nil
}

func (f *runtimeCatalogCLI) ReadRuntimeLifecycleSnapshot(context.Context) (tobari.RuntimeLifecycleSnapshot, time.Time, error) {
	f.lifecycleReads++
	if f.lifecycleErr != nil {
		return tobari.RuntimeLifecycleSnapshot{}, time.Time{}, f.lifecycleErr
	}
	standard := tobari.RuntimeManifest{
		SchemaVersion: tobari.RuntimeSchemaVersion,
		ID:            tobari.StandardRuntimeID,
		Name:          tobari.StandardRuntimeName,
		Kind:          tobari.RuntimeKindBuiltin,
		Revisions: []tobari.RuntimeRevision{{
			Ordinal:   1,
			Revision:  "sha256:" + strings.Repeat("f", 64),
			Image:     "tobari-runtime:test",
			CreatedAt: time.Unix(1, 0).UTC(),
		}},
	}
	observedManifest := f.runtimeManifest()
	if f.snapshotManifest != nil {
		observedManifest = *f.snapshotManifest
	}
	runtimes := []tobari.RuntimeManifest{standard, observedManifest}
	if f.list != nil && !(f.replaceAfterRead && f.lifecycleReads > 1) {
		runtimes = []tobari.RuntimeManifest{}
		for _, summary := range f.list {
			if summary.Kind == tobari.RuntimeKindBuiltin {
				runtimes = append(runtimes, standard)
				continue
			}
			manifest := f.runtimeManifest()
			manifest.ID, manifest.Name, manifest.SourcePath = summary.ID, summary.Name, summary.SourcePath
			if !summary.Ready {
				manifest.Revisions = []tobari.RuntimeRevision{}
			} else if manifest.ID != summary.ID || len(manifest.Revisions) != summary.Head || manifest.Revisions[len(manifest.Revisions)-1].Revision != summary.Revision {
				manifest.Revisions = make([]tobari.RuntimeRevision, summary.Head)
				for index := range manifest.Revisions {
					digest := "sha256:" + strings.Repeat(string(rune('1'+index)), 64)
					if index == summary.Head-1 {
						digest = summary.Revision
					}
					manifest.Revisions[index] = tobari.RuntimeRevision{
						Ordinal:      index + 1,
						Revision:     digest,
						Image:        "tobari-runtime-" + summary.Name + ":test",
						ImageDigest:  "sha256:" + strings.Repeat(string(rune('a'+index)), 64),
						CreatedAt:    time.Unix(int64(index+1), 0).UTC(),
						SnapshotPath: "/config/runtimes/" + summary.Name + "/snapshots/" + digest[7:19],
					}
				}
			}
			runtimes = append(runtimes, manifest)
		}
	}
	items := []tobari.RuntimeMaterialObservation{}
	storage := []tobari.RuntimeStorageObservation{}
	for _, runtime := range runtimes {
		if runtime.Kind != tobari.RuntimeKindManaged {
			continue
		}
		observed := tobari.RuntimeStorageObservation{
			RuntimeID:          runtime.ID,
			Name:               runtime.Name,
			SourceLogicalBytes: 42,
			Snapshots:          []tobari.RuntimeSnapshotStorage{},
		}
		for _, revision := range runtime.Revisions {
			availability := f.lifecycleAvailability
			if availability == "" {
				availability = tobari.RuntimeAvailabilityMissing
			}
			items = append(items, tobari.RuntimeMaterialObservation{
				RuntimeID:           runtime.ID,
				Revision:            revision.Revision,
				TagRole:             tobari.RuntimeMaterialTagPublishedRevision,
				Availability:        availability,
				ObservationComplete: true,
			})
			observed.Snapshots = append(observed.Snapshots, tobari.RuntimeSnapshotStorage{
				Kind:                tobari.RuntimePruneCandidateRevision,
				Revision:            revision.Revision,
				SemanticFingerprint: revision.Revision,
				LogicalBytes:        100,
			})
		}
		storage = append(storage, observed)
	}
	sort.Slice(storage, func(i, j int) bool { return storage[i].RuntimeID < storage[j].RuntimeID })
	return tobari.RuntimeLifecycleSnapshot{
		CatalogComplete: true,
		Runtimes:        runtimes,
		Protection: tobari.RuntimeProtectionInventory{
			Complete: true,
			Items:    []tobari.RuntimeProtection{},
		},
		Materials: items,
		Storage:   storage,
		Journals: tobari.RuntimeLifecycleJournals{
			Complete:     true,
			Active:       append([]tobari.RuntimeLifecycleActivity{}, f.lifecycleActivities...),
			FailedBuilds: []tobari.RuntimeFailedBuildArtifact{},
		},
	}, time.Unix(1, 0).UTC(), nil
}

func (f *runtimeCatalogCLI) ReadRuntimeBuildRecovery(context.Context) (tobari.RuntimeBuildRecovery, bool, error) {
	f.recoveryReads++
	if f.recoveryErr != nil {
		return tobari.RuntimeBuildRecovery{}, false, f.recoveryErr
	}
	if f.recovery == nil {
		return tobari.RuntimeBuildRecovery{}, false, nil
	}
	return *f.recovery, true, nil
}

func (f *runtimeCatalogCLI) RecoverRuntimeBuildByReference(_ context.Context, runtimeRef string, kind tobari.RuntimeBuildRecoveryKind) error {
	f.recoveries++
	f.recoveredRef = runtimeRef
	f.recoveredKind = kind
	return f.recoveryErr
}

func (f *runtimeCatalogCLI) ReadRuntimeDeleteRecovery(context.Context) (tobari.RuntimeSummary, bool, error) {
	for _, activity := range f.lifecycleActivities {
		if activity.Kind == tobari.RuntimeLifecycleActivityDelete {
			return tobari.RuntimeSummaryFrom(f.runtimeManifest()), true, nil
		}
	}
	return tobari.RuntimeSummary{}, false, nil
}

func (f *runtimeCatalogCLI) DeleteManagedRuntimeByReference(context.Context, string) (tobari.RuntimeDeleteResult, error) {
	return tobari.RuntimeDeleteResult{}, tobari.ErrRuntimeNotFound
}

func commandErrorByCode(t *testing.T, errors []CommandError, code string) CommandError {
	t.Helper()
	for _, candidate := range errors {
		if candidate.Code == code {
			return candidate
		}
	}
	t.Fatalf("command errors do not declare %q: %+v", code, errors)
	return CommandError{}
}
