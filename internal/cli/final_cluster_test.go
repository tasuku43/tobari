package cli

import (
	"bytes"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
)

func TestFinalClusterUpPublicResultOwnsOnlyTheCatalogEnvelopeVersion(t *testing.T) {
	encoded, err := finalClusterJSON("cluster up", "cluster_up", finalClusterUpPublicResult{
		Task:     workspaceauthoritycmd.TaskClusterUp,
		Applied:  true,
		Contexts: []finalClusterContextActivationPublicResult{},
	})
	if err != nil {
		t.Fatalf("encode final cluster-up result: %v", err)
	}
	if bytes.Count(encoded, []byte(`"schema_version"`)) != 1 {
		t.Fatalf("cluster-up result must expose exactly one Catalog-owned schema version: %s", encoded)
	}
	if bytes.Contains(encoded, []byte(`"cluster_up":{"schema_version"`)) {
		t.Fatalf("cluster-up result exposed its private application validation version: %s", encoded)
	}
}

func TestFinalClusterDownPublicResultOwnsOnlyTheCatalogEnvelopeVersion(t *testing.T) {
	encoded, err := finalClusterJSON("cluster down", "cluster_down", finalClusterDownPublicResult{Task: workspaceauthoritycmd.TaskClusterDown, Stopped: true})
	if err != nil {
		t.Fatalf("encode final cluster-down result: %v", err)
	}
	if bytes.Count(encoded, []byte(`"schema_version"`)) != 1 {
		t.Fatalf("cluster-down result must expose exactly one Catalog-owned schema version: %s", encoded)
	}
}
