package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/app/workspaceauthoritycmd"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterErrorFixture struct{ err error }

type finalClusterDownCLIFixture struct {
	purge bool
}

func (f *finalClusterDownCLIFixture) Down(_ context.Context, purge bool) (tobari.WorkspaceAuthorityClusterDownPlan, error) {
	f.purge = purge
	digest := tobari.SemanticDigest("sha256:" + strings.Repeat("a", 64))
	return tobari.WorkspaceAuthorityClusterDownPlan{
		SchemaVersion: 1, Purge: purge, PreviousGeneration: 1, PreviousRevision: digest,
		NextGeneration: 1, NextRevision: digest, EnvelopeChanged: false,
	}, nil
}

func (f finalClusterErrorFixture) Reconcile(context.Context, operation.Intent) (workspaceauthoritycmd.FinalClusterReconciliation, error) {
	return workspaceauthoritycmd.FinalClusterReconciliation{}, f.err
}

func TestFinalClusterUpEmitsDeclaredResourceConflict(t *testing.T) {
	structured := fault.WithClassification(fault.New(
		fault.KindRejected,
		"cluster_resource_conflict",
		"Fresh shared-cluster resources are present or could not be proved absent.",
		false,
		fault.NextAction{Command: "doctor", Reason: "Inspect exact Docker and Tobari ownership state before another cluster activation."},
	), fault.PhasePrecondition, fault.ChangeNone)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalCluster = finalClusterErrorFixture{err: structured}
	if code := command.RunContext(context.Background(), []string{"cluster", "up"}); code != ExitRejected {
		t.Fatalf("cluster up exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "cluster_resource_conflict") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("cluster up fault=%q", stderr.String())
	}
}

func TestFinalClusterUpEmitsDeclaredGatewayBuildFault(t *testing.T) {
	structured := fault.New(
		fault.KindUnavailable,
		"gateway_image_build_failed",
		"The pinned Gateway image could not be built.",
		false,
		fault.NextAction{Command: "doctor", Reason: "Inspect Docker build support and network access for the pinned Gateway inputs."},
	)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalCluster = finalClusterErrorFixture{err: structured}
	if code := command.RunContext(context.Background(), []string{"cluster", "up"}); code != ExitUnavailable {
		t.Fatalf("cluster up exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "gateway_image_build_failed") || strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("cluster up Gateway build fault=%q", stderr.String())
	}
}

func TestFinalClusterUpEmitsDeclaredMutationRecoveryPrecondition(t *testing.T) {
	structured := fault.WithClassification(fault.New(
		fault.KindUnavailable,
		"final_authority_mutation_recovery_required",
		"A preserved final-authority mutation must be recovered through its exact initiating command before another mutation; do not remove authority files manually.",
		false,
		fault.NextAction{Command: "status", Reason: "Read the preserved decision and recover it through the exact initiating command."},
	), fault.PhasePrecondition, fault.ChangeNone)
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	command.finalCluster = finalClusterErrorFixture{err: structured}
	if code := command.RunContext(context.Background(), []string{"--error-format", "json", "cluster", "up"}); code != ExitUnavailable {
		t.Fatalf("cluster up exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	var document errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
		t.Fatalf("decode cluster up recovery fault: %v; stderr=%q", err, stderr.String())
	}
	if document.Error.Code != "final_authority_mutation_recovery_required" ||
		document.Error.Phase != fault.PhasePrecondition || document.Error.ChangeState != fault.ChangeNone ||
		strings.Contains(stderr.String(), "undeclared_fault_contract") {
		t.Fatalf("cluster up recovery fault=%+v stderr=%q", document.Error, stderr.String())
	}
}

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
	encoded, err := finalClusterJSON("cluster down", "cluster_down", tobari.FinalClusterDownSchemaVersion, finalClusterDownPublicResult{Task: workspaceauthoritycmd.TaskClusterDown, Stopped: true, Purged: true})
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

func TestFinalClusterDownCatalogForwardsPurgeAndConfirmsItInJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newCLI(strings.NewReader(""), &stdout, &stderr, DefaultCatalog(), nil)
	port := &finalClusterDownCLIFixture{}
	command.finalClusterLifecycle = workspaceauthoritycmd.NewFinalClusterLifecycleService(port)
	if code := command.RunContext(context.Background(), []string{"cluster", "down", "--purge", "--format=json"}); code != ExitOK {
		t.Fatalf("cluster down exit=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !port.purge || !strings.Contains(stdout.String(), `"purged":true`) {
		t.Fatalf("cluster down purge=%t output=%q", port.purge, stdout.String())
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
			contextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789a2")
			templateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789a1")
			value.ContextCount = 1
			value.Contexts = []tobari.FinalClusterContextReceiptObservation{{
				ContextID: contextID,
				TemplatePolicy: &tobari.TemplatePolicyActivationReceipt{
					ContextID: contextID, TemplateID: templateID, PolicySliceDigest: digest,
				},
				PolicyMemory: &tobari.PolicyMemoryActivationReceipt{ContextID: contextID, Revision: digest},
			}}
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
			if name == "active" {
				context := cluster["contexts"].([]any)[0].(map[string]any)
				memory := context["policy_memory"].(map[string]any)
				if memory["revision"] != string(digest) {
					t.Fatalf("status policy-memory public revision = %v", memory)
				}
				if _, leaked := memory["policy_memory_revision"]; leaked {
					t.Fatalf("status leaked private receipt key: %s", encoded)
				}
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
