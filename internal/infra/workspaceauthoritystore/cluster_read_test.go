package workspaceauthoritystore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterReadRuntimeFixture struct {
	statuses     []tobari.FinalClusterStatus
	observed     []tobari.WorkspaceAuthorityCollection
	present      []bool
	logs         []byte
	denials      tobari.DenialRead
	logCalls     int
	denialCalls  int
	afterLog     func()
	afterDenials func()
}

func (f *finalClusterReadRuntimeFixture) ObserveFinalCluster(
	_ context.Context,
	collection tobari.WorkspaceAuthorityCollection,
	present bool,
) (tobari.FinalClusterStatus, error) {
	f.observed = append(f.observed, collection.Clone())
	f.present = append(f.present, present)
	index := len(f.observed) - 1
	if index >= len(f.statuses) {
		index = len(f.statuses) - 1
	}
	return f.statuses[index], nil
}

func (f *finalClusterReadRuntimeFixture) ReadFinalClusterLogs(context.Context, tobari.LogRequest) ([]byte, error) {
	f.logCalls++
	if f.afterLog != nil {
		f.afterLog()
	}
	return append([]byte{}, f.logs...), nil
}

func (f *finalClusterReadRuntimeFixture) ReadFinalClusterDenials(context.Context, int) (tobari.DenialRead, error) {
	f.denialCalls++
	if f.afterDenials != nil {
		f.afterDenials()
	}
	return f.denials, nil
}

func finalClusterReadStatus(collection tobari.WorkspaceAuthorityCollection) tobari.FinalClusterStatus {
	contexts := make([]tobari.FinalClusterContextReceiptObservation, len(collection.Contexts))
	for index, record := range collection.Contexts {
		contexts[index] = tobari.FinalClusterContextReceiptObservation{
			ContextID:      record.Context.ID,
			TemplatePolicy: record.ActiveTemplatePolicy,
			PolicyMemory:   record.ActivePolicyMemoryRef,
		}
	}
	aggregateRevision := strings.Repeat("a", 64)
	evaluatorIdentity := tobari.PolicyEvaluatorIdentity{
		SchemaVersion: 1,
		Version:       "test-evaluator-v1",
		Digest:        tobari.SemanticDigest("sha256:" + strings.Repeat("b", 64)),
	}
	policyDataIdentity := tobari.PolicyDataIdentity{
		SchemaVersion: 1,
		Digest:        tobari.SemanticDigest("sha256:" + strings.Repeat("c", 64)),
	}
	return tobari.FinalClusterStatus{
		SchemaVersion:      tobari.FinalClusterStatusSchemaVersion,
		Task:               tobari.TaskClusterStatus,
		Authority:          tobari.FinalClusterAuthorityPresent,
		Generation:         collection.Generation,
		CollectionRevision: collection.Revision,
		AggregateRevision:  &aggregateRevision,
		EvaluatorIdentity:  &evaluatorIdentity,
		PolicyDataIdentity: &policyDataIdentity,
		TemplateCount:      len(collection.Templates),
		ContextCount:       len(collection.Contexts),
		WorkspaceCount:     len(collection.Workspaces),
		Runtime:            tobari.FinalClusterRuntimeRunning,
		Receipt:            tobari.FinalClusterReceiptActive,
		Contexts:           contexts,
		Components: []tobari.FinalClusterComponentObservation{
			{Name: "gateway", State: tobari.FinalClusterRuntimeRunning, Health: "healthy", Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact},
			{Name: "opa", State: tobari.FinalClusterRuntimeRunning, Health: "healthy", Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact},
		},
	}
}

func newFinalClusterReadFixture(
	t *testing.T,
	collection *tobari.WorkspaceAuthorityCollection,
	guard *legacyGuardFake,
	runtime *finalClusterReadRuntimeFixture,
) (string, *ClusterReadAdapter) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "authority")
	if collection != nil {
		materializeCollection(t, root, *collection)
	}
	store, err := NewFinalOnly(root, guard)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewClusterReadAdapter(store, runtime)
	if err != nil {
		t.Fatal(err)
	}
	return root, adapter
}

func TestBatchDClusterReadUsesOneSelectedFinalAuthority(t *testing.T) {
	collection := storeCollectionFixture(t)
	status := finalClusterReadStatus(collection)
	runtime := &finalClusterReadRuntimeFixture{statuses: []tobari.FinalClusterStatus{status}, logs: []byte("== gateway ==\nready\n")}
	_, adapter := newFinalClusterReadFixture(t, &collection, &legacyGuardFake{}, runtime)

	got, err := adapter.ReadLogs(context.Background(), tobari.LogRequest{Component: "gateway", Tail: 20})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(runtime.logs) || runtime.logCalls != 1 || runtime.denialCalls != 0 || len(runtime.observed) != 2 {
		t.Fatalf("logs=%q reads=(%d,%d) observations=%d", got, runtime.logCalls, runtime.denialCalls, len(runtime.observed))
	}
	for index := range runtime.observed {
		if !runtime.present[index] || runtime.observed[index].Generation != collection.Generation || runtime.observed[index].Revision != collection.Revision {
			t.Fatalf("observation %d selected %#v present=%t", index, runtime.observed[index], runtime.present[index])
		}
	}
}

func TestBatchDClusterReadRejectsNonActiveAndLegacyAuthorityBeforeEffect(t *testing.T) {
	collection := storeCollectionFixture(t)
	active := finalClusterReadStatus(collection)
	stopped := active
	stopped.Runtime, stopped.Receipt = tobari.FinalClusterRuntimeStopped, tobari.FinalClusterReceiptStopped
	stopped.AggregateRevision, stopped.EvaluatorIdentity, stopped.PolicyDataIdentity = nil, nil, nil
	drifted := active
	drifted.Runtime, drifted.Receipt = tobari.FinalClusterRuntimeDrifted, tobari.FinalClusterReceiptDrifted
	drifted.AggregateRevision, drifted.EvaluatorIdentity, drifted.PolicyDataIdentity = nil, nil, nil
	absent := tobari.FinalClusterStatus{
		SchemaVersion: tobari.FinalClusterStatusSchemaVersion,
		Task:          tobari.TaskClusterStatus,
		Authority:     tobari.FinalClusterAuthorityAbsent,
		Runtime:       tobari.FinalClusterRuntimeAbsent,
		Receipt:       tobari.FinalClusterReceiptAbsent,
		Contexts:      []tobari.FinalClusterContextReceiptObservation{},
		Components:    []tobari.FinalClusterComponentObservation{},
	}

	tests := []struct {
		name       string
		collection *tobari.WorkspaceAuthorityCollection
		guard      *legacyGuardFake
		status     tobari.FinalClusterStatus
		wantLegacy bool
	}{
		{name: "absent", guard: &legacyGuardFake{}, status: absent},
		{name: "stopped", collection: &collection, guard: &legacyGuardFake{}, status: stopped},
		{name: "drifted", collection: &collection, guard: &legacyGuardFake{}, status: drifted},
		{name: "legacy", collection: &collection, guard: &legacyGuardFake{errors: []error{errors.New("legacy authority is present")}}, status: active, wantLegacy: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime := &finalClusterReadRuntimeFixture{statuses: []tobari.FinalClusterStatus{test.status}, logs: []byte("must not read")}
			root, adapter := newFinalClusterReadFixture(t, test.collection, test.guard, runtime)
			_, err := adapter.ReadLogs(context.Background(), tobari.LogRequest{Component: "gateway", Tail: 20})
			if err == nil {
				t.Fatal("non-active authority allowed a log effect")
			}
			if test.wantLegacy && !errors.Is(err, tobari.ErrPreReleaseLegacyAuthority) {
				t.Fatalf("legacy error=%v", err)
			}
			if runtime.logCalls != 0 || runtime.denialCalls != 0 {
				t.Fatalf("external read calls=(%d,%d)", runtime.logCalls, runtime.denialCalls)
			}
			if test.collection == nil {
				if _, statErr := os.Lstat(root); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("absent final read created authority root: %v", statErr)
				}
			}
		})
	}
}

func TestBatchDClusterReadDenialsCorrelateExactContextTemplateAndWorkspace(t *testing.T) {
	collection := storeCollectionFixture(t)
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-24T00:00:00Z",
		RequestID:              "7185da2688d7469aae9cd9068e920b0b",
		WorkspaceManifestID:    string(storeContextID),
		WorkspaceManifestName:  collection.Templates[0].Name,
		ProjectID:              string(storeWorkspaceID),
		ProjectRoot:            collection.Contexts[0].Context.ProjectRoot,
		Host:                   "api.example.dev",
		Port:                   443,
		Method:                 "GET",
		Path:                   "/candidate",
		Reason:                 "request did not match an allow rule",
		StatusCode:             403,
		Learnable:              true,
	}
	status := finalClusterReadStatus(collection)
	runtime := &finalClusterReadRuntimeFixture{
		statuses: []tobari.FinalClusterStatus{status},
		denials:  tobari.DenialRead{Items: []tobari.PolicyDenial{denial}},
	}
	_, adapter := newFinalClusterReadFixture(t, &collection, &legacyGuardFake{}, runtime)

	window, err := adapter.ReadDenials(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := window.Validate(); err != nil || window.SchemaVersion != tobari.FinalClusterDenialSchemaVersion || len(window.Items) != 1 {
		t.Fatalf("window=%#v err=%v", window, err)
	}
	item := window.Items[0]
	if item.ContextID != storeContextID || item.WorkspaceTemplateID != storeTemplateID || item.TemplateName != collection.Templates[0].Name || item.WorkspaceID != storeWorkspaceID || item.ProjectRoot != collection.Contexts[0].Context.ProjectRoot {
		t.Fatalf("denial owner correlation=%#v", item)
	}
	encoded, err := json.Marshal(window)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"workspace_manifest_id", "workspace_manifest", "manifest_id"} {
		if containsJSONToken(encoded, forbidden) {
			t.Fatalf("final denial JSON exposes retired key %q: %s", forbidden, encoded)
		}
	}
}

func TestBatchDClusterReadRejectsStoreOrRuntimeDriftAfterEffect(t *testing.T) {
	collection := storeCollectionFixture(t)
	active := finalClusterReadStatus(collection)
	driftedStatus := active
	driftedStatus.Receipt = tobari.FinalClusterReceiptDrifted
	driftedStatus.AggregateRevision, driftedStatus.EvaluatorIdentity, driftedStatus.PolicyDataIdentity = nil, nil, nil

	t.Run("runtime receipt", func(t *testing.T) {
		runtime := &finalClusterReadRuntimeFixture{statuses: []tobari.FinalClusterStatus{active, driftedStatus}, logs: []byte("observed")}
		_, adapter := newFinalClusterReadFixture(t, &collection, &legacyGuardFake{}, runtime)
		if _, err := adapter.ReadLogs(context.Background(), tobari.LogRequest{Component: "gateway", Tail: 20}); !errors.Is(err, tobari.ErrFinalClusterObservationChanged) {
			t.Fatalf("runtime drift error=%v", err)
		}
		if runtime.logCalls != 1 || len(runtime.observed) != 2 {
			t.Fatalf("runtime drift calls=%d observations=%d", runtime.logCalls, len(runtime.observed))
		}
	})

	t.Run("final envelope", func(t *testing.T) {
		runtime := &finalClusterReadRuntimeFixture{statuses: []tobari.FinalClusterStatus{active}, logs: []byte("observed")}
		root, adapter := newFinalClusterReadFixture(t, &collection, &legacyGuardFake{}, runtime)
		runtime.afterLog = func() {
			newer, changed, err := tobari.PublishWorkspaceAuthorityCollection(
				collection.Templates, collection.Contexts, collection.Workspaces,
				[]tobari.PolicyCandidateAuthority{}, collection.DefaultTemplateID, &collection,
			)
			if err != nil || !changed {
				t.Fatalf("publish drift fixture: changed=%t err=%v", changed, err)
			}
			data, err := json.Marshal(newer)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, authorityFileName), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := adapter.ReadLogs(context.Background(), tobari.LogRequest{Component: "gateway", Tail: 20}); !errors.Is(err, tobari.ErrFinalClusterObservationChanged) {
			t.Fatalf("Store drift error=%v", err)
		}
		if runtime.logCalls != 1 || len(runtime.observed) != 1 {
			t.Fatalf("Store drift calls=%d observations=%d", runtime.logCalls, len(runtime.observed))
		}
	})
}

func containsJSONToken(encoded []byte, token string) bool {
	var value any
	if json.Unmarshal(encoded, &value) != nil {
		return false
	}
	return containsJSONValue(value, token)
}

func containsJSONValue(value any, token string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == token || containsJSONValue(child, token) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsJSONValue(child, token) {
				return true
			}
		}
	}
	return false
}
