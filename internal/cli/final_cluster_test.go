package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func TestFinalClusterUpPublicResultOwnsOnlyTheCatalogEnvelopeVersion(t *testing.T) {
	encoded, err := finalClusterJSON("cluster up", "cluster_up", tobari.FinalClusterUpSchemaVersion, finalClusterUpPublicResult{
		Task:     workspaceauthoritycmd.TaskClusterUp,
		Applied:  true,
		Contexts: []finalClusterContextActivationPublicResult{},
	})
	if err != nil {
		t.Fatalf("encode final cluster-up result: %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatalf("decode cluster-up result: %v", err)
	}
	if envelope["schema_version"] != float64(tobari.FinalClusterUpSchemaVersion) {
		t.Fatalf("cluster-up outer schema version=%v", envelope["schema_version"])
	}
	if bytes.Contains(encoded, []byte(`"cluster_up":{"schema_version"`)) {
		t.Fatalf("cluster-up result exposed its private application validation version: %s", encoded)
	}
}

func TestFinalClusterDownPublicResultOwnsOnlyTheCatalogEnvelopeVersion(t *testing.T) {
	encoded, err := finalClusterJSON("cluster down", "cluster_down", tobari.FinalClusterDownSchemaVersion, finalClusterDownPublicResult{Task: workspaceauthoritycmd.TaskClusterDown, Stopped: true})
	if err != nil {
		t.Fatalf("encode final cluster-down result: %v", err)
	}
	if bytes.Count(encoded, []byte(`"schema_version"`)) != 1 {
		t.Fatalf("cluster-down result must expose exactly one Catalog-owned schema version: %s", encoded)
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope["schema_version"] != float64(tobari.FinalClusterDownSchemaVersion) {
		t.Fatalf("cluster-down outer schema version=%v", envelope["schema_version"])
	}
}

func TestFinalClusterStatusJSONMatchesCatalogForActiveStoppedAndAbsent(t *testing.T) {
	digest := tobari.SemanticDigest("sha256:" + string(bytes.Repeat([]byte("a"), 64)))
	aggregate := string(bytes.Repeat([]byte("b"), 64))
	evaluator := tobari.PolicyEvaluatorIdentity{SchemaVersion: 1, Version: "tobari-evaluator-v1", Digest: digest}
	policyData := tobari.PolicyDataIdentity{SchemaVersion: 1, Digest: digest}
	base := tobari.FinalClusterStatus{
		SchemaVersion: tobari.FinalClusterStatusSchemaVersion, Task: tobari.TaskClusterStatus,
		Contexts: []tobari.FinalClusterContextReceiptObservation{}, Components: []tobari.FinalClusterComponentObservation{},
	}
	tests := map[string]tobari.FinalClusterStatus{
		"absent": func() tobari.FinalClusterStatus {
			value := base
			value.Authority, value.Runtime, value.Receipt = tobari.FinalClusterAuthorityAbsent, tobari.FinalClusterRuntimeAbsent, tobari.FinalClusterReceiptAbsent
			return value
		}(),
		"stopped": func() tobari.FinalClusterStatus {
			value := base
			value.Authority, value.Generation, value.CollectionRevision = tobari.FinalClusterAuthorityPresent, 1, digest
			value.Runtime, value.Receipt = tobari.FinalClusterRuntimeStopped, tobari.FinalClusterReceiptStopped
			value.Components = []tobari.FinalClusterComponentObservation{{Name: "gateway", State: tobari.FinalClusterRuntimeStopped, Health: "stopped", Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact}}
			return value
		}(),
		"active": func() tobari.FinalClusterStatus {
			value := base
			value.Authority, value.Generation, value.CollectionRevision = tobari.FinalClusterAuthorityPresent, 1, digest
			value.Runtime, value.Receipt = tobari.FinalClusterRuntimeRunning, tobari.FinalClusterReceiptActive
			value.AggregateRevision, value.EvaluatorIdentity, value.PolicyDataIdentity = &aggregate, &evaluator, &policyData
			value.Components = []tobari.FinalClusterComponentObservation{{Name: "gateway", State: tobari.FinalClusterRuntimeRunning, Identity: tobari.FinalClusterEvidenceExact, Topology: tobari.FinalClusterEvidenceExact}}
			return value
		}(),
	}
	for name, status := range tests {
		t.Run(name, func(t *testing.T) {
			encoded, err := renderFinalClusterStatus("cluster status", status, successFormatJSON)
			if err != nil {
				t.Fatalf("render status JSON: %v", err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(encoded, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["schema_version"] != float64(tobari.FinalClusterStatusSchemaVersion) {
				t.Fatalf("status outer schema version=%v", envelope["schema_version"])
			}
			cluster, ok := envelope["cluster"].(map[string]any)
			if !ok {
				t.Fatalf("status cluster envelope=%T", envelope["cluster"])
			}
			if _, exists := cluster["schema_version"]; exists {
				t.Fatalf("status exposed its private validation schema: %s", encoded)
			}
		})
	}
}

func TestFinalClusterStatusJSONOmitsPrivateApplicationSchemaVersion(t *testing.T) {
	status := tobari.FinalClusterStatus{
		SchemaVersion: tobari.FinalClusterStatusSchemaVersion,
		Task:          tobari.TaskClusterStatus,
		Authority:     tobari.FinalClusterAuthorityAbsent,
		Runtime:       tobari.FinalClusterRuntimeAbsent,
		Receipt:       tobari.FinalClusterReceiptAbsent,
		Contexts:      []tobari.FinalClusterContextReceiptObservation{},
		Components:    []tobari.FinalClusterComponentObservation{},
	}
	encoded, err := renderFinalClusterStatus("cluster status", status, successFormatJSON)
	if err != nil {
		t.Fatalf("encode final cluster-status result: %v", err)
	}
	if bytes.Count(encoded, []byte(`"schema_version"`)) != 1 || bytes.Contains(encoded, []byte(`"cluster":{"schema_version"`)) {
		t.Fatalf("cluster-status result exposed a private schema version: %s", encoded)
	}
}
