package dockerruntime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalPolicyActivationRunner struct {
	runtime         *Runtime
	gatewaySource   func() string
	gatewayRole     string
	gatewayState    string
	gatewayHealth   string
	gatewayNetworks map[string]json.RawMessage
	policyStageRuns int
}

func (r *finalPolicyActivationRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, errOut io.Writer) error {
	switch {
	case len(args) >= 4 && args[0] == "inspect" && args[len(args)-1] == gatewayContainer:
		observation := appliedClusterComponentObservation{
			ContainerID: strings.Repeat("a", 64), Owner: ownerValue, Component: "gateway", Role: r.gatewayRole,
			ImageID: "sha256:" + strings.Repeat("b", 64), State: r.gatewayState, Health: r.gatewayHealth,
			Environment: []string{}, MountDestinations: []string{}, Networks: r.gatewayNetworks,
		}
		data, _ := json.Marshal(observation)
		_, _ = out.Write(data)
		return nil
	case len(args) >= 4 && args[0] == "container" && args[1] == "inspect" && args[len(args)-1] == gatewayContainer:
		source := ""
		if r.gatewaySource != nil {
			source = r.gatewaySource()
		}
		data, _ := json.Marshal(map[string]any{
			"container_id": strings.Repeat("a", 64), "owner": ownerValue, "component": "gateway", "role": r.gatewayRole,
			"image_id": "sha256:" + strings.Repeat("b", 64), "state": r.gatewayState, "health": r.gatewayHealth,
			"networks": r.gatewayNetworks,
			"mounts":   []map[string]string{{"type": "bind", "source": source, "destination": "/run/tobari/config/gateway.json"}},
		})
		_, _ = out.Write(data)
		return nil
	case len(args) > 0 && args[0] == "run":
		if slices.Contains(args, "--interactive") {
			r.policyStageRuns++
		}
		return nil
	default:
		_, _ = errOut.Write(nil)
		return nil
	}
}

func (r *finalPolicyActivationRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer {
		return []byte("true"), nil
	}
	if len(args) > 0 && (args[0] == "inspect" || args[0] == "volume") {
		return []byte(ownerValue + "\n"), nil
	}
	return nil, nil
}

func finalPolicyActivationFixture(t *testing.T) (*Runtime, *finalPolicyActivationRunner, tobari.WorkspaceAuthorityCollection) {
	t.Helper()
	root := t.TempDir()
	runner := &finalPolicyActivationRunner{
		gatewayRole: gatewayRole, gatewayState: "running", gatewayHealth: "healthy",
		gatewayNetworks: map[string]json.RawMessage{
			"tobari-control": json.RawMessage(`{"IPAddress":"172.20.0.2"}`),
			"tobari-egress":  json.RawMessage(`{"IPAddress":"172.21.0.2"}`),
		},
	}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runner.runtime = runtime
	runner.gatewaySource = func() string {
		var found string
		_ = filepath.WalkDir(runtime.aggregateRoot(), func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() && entry.Name() == "gateway.json" && found == "" {
				found = path
			}
			return nil
		})
		return found
	}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	return runtime, runner, finalProjectionCollectionFixture(t, "")
}

func largeFinalPolicyCollectionFixture(t *testing.T) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	collection := finalProjectionCollectionFixture(t, "")
	template := collection.Templates[0].Clone()
	body := template.Current.Body.Clone()
	body.Policy.Mode = tobari.ManifestPolicyModeAdvanced
	body.Policy.AdvancedPolicy = &tobari.WorkspaceTemplateAdvancedPolicySources{
		Tobari:     "package tobari.http\n\nimport rego.v1\ndecision := {\"allow\": false} if { input.schema_version == 1; data.tobari.schema_version == 1; false }\n#" + strings.Repeat("a", 192*1024),
		TobariTest: "package tobari.http_test\n\nimport rego.v1\n#" + strings.Repeat("b", 192*1024),
	}
	revision, err := tobari.NewWorkspaceTemplateRevision(template.ID, template.Current.Generation+1, body)
	if err != nil {
		t.Fatal(err)
	}
	template.Current = revision
	template.Retained = []tobari.WorkspaceTemplateRevision{revision.Clone()}
	record := collection.Contexts[0].Clone()
	receipt := tobari.TemplatePolicyActivationReceipt{ContextID: record.Context.ID, TemplateID: template.ID, PolicySliceDigest: revision.Slices.PolicySliceDigest}
	record.ActiveTemplatePolicy = &receipt
	large, _, err := tobari.PublishWorkspaceAuthorityCollection([]tobari.WorkspaceTemplate{template}, []tobari.WorkspaceAuthorityContextRecord{record}, collection.Workspaces, collection.PendingCandidates, collection.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	return large
}

func TestFinalPolicyActivationPersistsExactReceiptAndSurvivesUnrelatedCollectionChange(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	receipt, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID)
	if err != nil {
		t.Fatal(err)
	}
	if runner.policyStageRuns == 0 {
		t.Fatal("hot policy activation performed no OPA publication")
	}
	effects := runner.policyStageRuns
	if repeated, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil || repeated != receipt {
		t.Fatalf("exact no-op activation receipt=%+v err=%v", repeated, err)
	}
	if runner.policyStageRuns != effects {
		t.Fatalf("exact no-op activation repeated OPA effect: before=%d after=%d", effects, runner.policyStageRuns)
	}
	defaultID := finalProjectionTemplateID
	changed, _, err := tobari.PublishWorkspaceAuthorityCollection(collection.Templates, collection.Contexts, collection.Workspaces, collection.PendingCandidates, &defaultID, &collection)
	if err != nil || changed.Revision == collection.Revision {
		t.Fatalf("pure collection change err=%v revision=%q", err, changed.Revision)
	}
	if err := runtime.ConfirmPolicyMemoryActive(context.Background(), changed, finalProjectionContextID, receipt); err != nil {
		t.Fatalf("Policy Memory confirm after pure collection change: %v", err)
	}
	templateReceipt := *changed.Contexts[0].ActiveTemplatePolicy
	if err := runtime.ConfirmTemplatePolicyActive(context.Background(), changed, finalProjectionContextID, templateReceipt); err != nil {
		t.Fatalf("Template policy confirm after pure collection change: %v", err)
	}
	if runner.policyStageRuns != effects {
		t.Fatalf("read-only confirmation repeated OPA effect: before=%d after=%d", effects, runner.policyStageRuns)
	}
	drifted := filepath.Join(runtime.aggregateRoot(), "confirmation-drift", "gateway.json")
	if err := os.MkdirAll(filepath.Dir(drifted), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(drifted, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner.gatewaySource = func() string { return drifted }
	if err := runtime.ConfirmPolicyMemoryActive(context.Background(), changed, finalProjectionContextID, receipt); err == nil || !strings.Contains(err.Error(), "Gateway configuration bytes changed") {
		t.Fatalf("read-only confirmation accepted mounted Gateway drift: %v", err)
	}
	if runner.policyStageRuns != effects {
		t.Fatalf("mounted drift confirmation repeated OPA effect: before=%d after=%d", effects, runner.policyStageRuns)
	}
	emptyMemory, _, err := tobari.PublishPolicyMemory(finalProjectionContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	contentDrift := changed
	contentDrift.Contexts = append([]tobari.WorkspaceAuthorityContextRecord{}, changed.Contexts...)
	contentDrift.Contexts[0] = changed.Contexts[0].Clone()
	contentDrift.Contexts[0].ActivePolicyMemory = &emptyMemory
	emptyReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: finalProjectionContextID, Revision: emptyMemory.Revision}
	contentDrift.Contexts[0].ActivePolicyMemoryRef = &emptyReceipt
	contentDrift, _, err = tobari.PublishWorkspaceAuthorityCollection(contentDrift.Templates, contentDrift.Contexts, contentDrift.Workspaces, contentDrift.PendingCandidates, contentDrift.DefaultTemplateID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmPolicyMemoryActive(context.Background(), contentDrift, finalProjectionContextID, receipt); err == nil {
		t.Fatal("confirmation accepted actual selected Policy Memory drift")
	}
}

func TestFinalPolicyActivationFailsBeforeEffectOnGatewayDriftOrRootDurabilityFailure(t *testing.T) {
	t.Run("Gateway artifact drift", func(t *testing.T) {
		runtime, runner, collection := finalPolicyActivationFixture(t)
		runner.gatewaySource = func() string {
			path := filepath.Join(runtime.aggregateRoot(), "drifted", "gateway.json")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		}
		if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil || !strings.Contains(err.Error(), "Gateway configuration bytes changed") {
			t.Fatalf("Gateway drift error=%v", err)
		}
		if runner.policyStageRuns != 0 {
			t.Fatalf("Gateway drift reached OPA effect %d times", runner.policyStageRuns)
		}
	})

	t.Run("first-use parent fsync", func(t *testing.T) {
		runtime, runner, collection := finalPolicyActivationFixture(t)
		runtime.finalPolicyRootSync = func(string) error { return errors.New("injected parent fsync failure") }
		if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil || !strings.Contains(err.Error(), "durably") {
			t.Fatalf("parent fsync error=%v", err)
		}
		if runner.policyStageRuns != 0 {
			t.Fatalf("parent fsync failure reached OPA effect %d times", runner.policyStageRuns)
		}
		if _, err := os.Lstat(runtime.finalPolicyActivationJournalPath()); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("parent fsync failure published journal: %v", err)
		}
	})
}

func TestFinalPolicyActivationRejectsGatewayHealthRoleAndExactTopologyDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*finalPolicyActivationRunner)
	}{
		{name: "stopped", mutate: func(r *finalPolicyActivationRunner) { r.gatewayState = "exited" }},
		{name: "unhealthy", mutate: func(r *finalPolicyActivationRunner) { r.gatewayHealth = "unhealthy" }},
		{name: "wrong role", mutate: func(r *finalPolicyActivationRunner) { r.gatewayRole = "alternate" }},
		{name: "missing control", mutate: func(r *finalPolicyActivationRunner) { delete(r.gatewayNetworks, "tobari-control") }},
		{name: "extra network", mutate: func(r *finalPolicyActivationRunner) {
			r.gatewayNetworks["unexpected"] = json.RawMessage(`{"IPAddress":"172.22.0.2"}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runtime, runner, collection := finalPolicyActivationFixture(t)
			test.mutate(runner)
			if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil {
				t.Fatal("Gateway drift authorized final policy activation")
			}
			if runner.policyStageRuns != 0 {
				t.Fatalf("Gateway drift reached OPA effect %d times", runner.policyStageRuns)
			}
		})
	}
}

func TestFinalPolicyActivationReobservesDockerAfterArtifactBuildBeforeEffect(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	runtime.finalProjectionBeforeEffect = func() {
		runner.gatewayNetworks["drift-after-build"] = json.RawMessage(`{"IPAddress":"172.23.0.2"}`)
	}
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil {
		t.Fatal("Docker drift after artifact build authorized policy effect")
	}
	if runner.policyStageRuns != 0 {
		t.Fatalf("post-build Docker drift reached OPA effect %d times", runner.policyStageRuns)
	}
	if _, err := os.Lstat(runtime.finalPolicyActivationJournalPath()); err != nil {
		t.Fatalf("post-build drift lost exact recovery decision: %v", err)
	}
}

func TestFinalPolicyActivationResumesJournalAfterConfirmedOPAEffect(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	runtime.finalPolicyAfterApply = func() error { return errors.New("injected interruption after OPA confirmation") }
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil {
		t.Fatal("post-effect interruption succeeded")
	}
	if _, err := os.Lstat(runtime.finalPolicyActivationJournalPath()); err != nil {
		t.Fatalf("post-effect interruption lost journal: %v", err)
	}
	runtime.finalPolicyAfterApply = nil
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil {
		t.Fatalf("resume final policy activation: %v", err)
	}
	if _, err := os.Lstat(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("resume did not publish active receipt: %v", err)
	}
	if _, err := os.Lstat(runtime.finalPolicyActivationJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("resume did not clear journal: %v", err)
	}
	if runner.policyStageRuns == 0 {
		t.Fatal("fixture did not reach OPA effect")
	}
}

func TestFinalPolicyActivationLargeReceiptUsesTaskOwnedRecoveryCodec(t *testing.T) {
	runtime, runner, _ := finalPolicyActivationFixture(t)
	collection := largeFinalPolicyCollectionFixture(t)
	runtime.finalPolicyAfterApply = func() error { return errors.New("large receipt interruption") }
	_, activationErr := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID)
	if activationErr == nil {
		t.Fatal("large activation interruption succeeded")
	}
	info, err := os.Lstat(runtime.finalPolicyActivationJournalPath())
	if err != nil {
		t.Fatalf("large activation failed before journal: activation=%v journal=%v", activationErr, err)
	}
	if info.Size() <= maxProjectStateBytes {
		t.Fatalf("large recovery journal size=%d", info.Size())
	}
	runtime.finalPolicyAfterApply = nil
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil {
		t.Fatalf("recover >128KiB activation journal: %v", err)
	}
	active, err := os.Lstat(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatalf("large active receipt missing: %v", err)
	}
	if active.Size() <= maxProjectStateBytes {
		t.Fatalf("large active receipt size=%d", active.Size())
	}
	if runner.policyStageRuns == 0 {
		t.Fatal("large fixture did not reach OPA publication")
	}
}

func TestFinalPolicyActivationRejectsOverCeilingRecordBeforeOPAEffect(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	runtime.finalPolicyActivationLimit = 1024
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("over-ceiling activation error=%v", err)
	}
	if runner.policyStageRuns != 0 {
		t.Fatalf("over-ceiling activation reached OPA effect %d times", runner.policyStageRuns)
	}
	if _, err := os.Lstat(runtime.finalPolicyActivationJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("over-ceiling activation published journal: %v", err)
	}
}

func TestFinalGatewayArtifactAllowsDifferentOwnedImmutablePathWithSameBytes(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	plan, err := tobari.BuildHotWorkspacePolicyProjection(collection, finalProjectionContextID)
	if err != nil {
		t.Fatal(err)
	}
	material, err := runtime.ObserveFinalWorkspacePolicyProjection(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	aggregate, err := runtime.BuildFinalAggregateProjection(context.Background(), material)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := runtime.NewFinalAggregatePublicationReceipt(material, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(runtime.aggregateRoot(), strings.Repeat("d", 64), "gateway.json")
	if err := os.MkdirAll(filepath.Dir(old), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(aggregate.GatewayConfig)
	if err := os.WriteFile(old, data, 0o600); err != nil {
		t.Fatal(err)
	}
	runner.gatewaySource = func() string { return old }
	if err := runtime.confirmLiveFinalGatewayArtifact(context.Background(), material, aggregate, receipt); err != nil {
		t.Fatalf("same bytes at previous immutable aggregate path: %v", err)
	}
}

func TestFinalAggregatePublicationReceiptBindsAggregateArtifactsAndAxes(t *testing.T) {
	runtime, _, collection := finalPolicyActivationFixture(t)
	if _, err := runtime.ActivatePolicyMemory(context.Background(), collection, finalProjectionContextID); err != nil {
		t.Fatal(err)
	}
	record, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}

	wrongRevision := record.Receipt
	wrongRevision.AggregateRevision = strings.Repeat("e", 64)
	if err := wrongRevision.ValidateFor(record.Material); err == nil {
		t.Fatal("receipt accepted a different aggregate revision")
	}
	wrongAxis := record.Receipt
	wrongAxis.PolicyMemoryReceipts = append([]tobari.PolicyMemoryActivationReceipt{}, wrongAxis.PolicyMemoryReceipts...)
	wrongAxis.PolicyMemoryReceipts[0].Revision = finalSessionDigest("9")
	if err := wrongAxis.ValidateFor(record.Material); err == nil {
		t.Fatal("receipt accepted a different per-Context memory axis")
	}
	wrongDigest := record.Receipt
	wrongDigest.ReceiptDigest = finalSessionDigest("8")
	if err := wrongDigest.ValidateFor(record.Material); err == nil {
		t.Fatal("receipt accepted a different publication digest")
	}

	original, err := os.ReadFile(record.Aggregate.GatewayConfig)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(record.Aggregate.GatewayConfig, append(original, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ConfirmFinalAggregatePublicationReceipt(record.Material, record.Aggregate, record.Receipt); err == nil {
		t.Fatal("receipt accepted changed Gateway artifact bytes")
	}
}

func TestPrepareFinalClusterPolicyReconciliationIsDormantAndZeroMutation(t *testing.T) {
	runtime, runner, collection := finalPolicyActivationFixture(t)
	material, aggregate, receipt, err := runtime.PrepareFinalClusterPolicyReconciliation(context.Background(), collection)
	if err != nil {
		t.Fatal(err)
	}
	if material.Plan.Mode != tobari.WorkspacePolicyProjectionCluster || aggregate.AggregateRevision != receipt.AggregateRevision {
		t.Fatalf("cluster candidate material=%+v aggregate=%+v receipt=%+v", material.Plan, aggregate, receipt)
	}
	if runner.policyStageRuns != 0 {
		t.Fatalf("dormant cluster candidate performed OPA effect %d times", runner.policyStageRuns)
	}
	for _, path := range []string{runtime.finalPolicyActivationJournalPath(), runtime.finalPolicyActiveReceiptPath()} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dormant cluster candidate published activation state %s: %v", path, err)
		}
	}
}
