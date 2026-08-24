package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type finalClusterLifecycleRunner struct {
	base                 *finalGatewaySettlementRunner
	cleanStart           bool
	removedComponents    map[string]bool
	componentRemoveCalls map[string]int
	removedNetworks      map[string]bool
	networkRemoveCalls   map[string]int
	volumeRemoveCalls    int
	failNetworkOnce      bool
	missingGateway       *FinalGatewayNetworkAddress
	networkConnectCalls  int
	recordStatusCalls    bool
	statusCalls          [][]string
}

func newFinalClusterLifecycleRunner(base *finalGatewaySettlementRunner) *finalClusterLifecycleRunner {
	return &finalClusterLifecycleRunner{
		base: base, cleanStart: true,
		removedComponents: map[string]bool{}, componentRemoveCalls: map[string]int{},
		removedNetworks: map[string]bool{}, networkRemoveCalls: map[string]int{},
	}
}

func (r *finalClusterLifecycleRunner) componentMissing(name string) bool {
	if name == authBrokerContainer && !brokerRuntimeEnabled {
		return true
	}
	return r.removedComponents[name] || r.cleanStart && !r.base.selected
}

func (r *finalClusterLifecycleRunner) writeMissing(name string, errOut io.Writer) error {
	_, _ = fmt.Fprintf(errOut, "Error: No such object: %s", name)
	return errors.New("No such object")
}

func (r *finalClusterLifecycleRunner) Run(ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer) error {
	if r.recordStatusCalls {
		r.statusCalls = append(r.statusCalls, append([]string(nil), args...))
	}
	if len(args) >= 6 && args[0] == "network" && args[1] == "connect" && args[len(args)-1] == gatewayContainer {
		r.networkConnectCalls++
		if r.missingGateway != nil && args[len(args)-2] == r.missingGateway.Name {
			r.base.candidate.GatewayNetworks = append(r.base.candidate.GatewayNetworks, *r.missingGateway)
			r.missingGateway = nil
		}
		return nil
	}
	if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" {
		if len(args) >= 5 && strings.Contains(args[3], `"repo_tags"`) {
			for index := len(args) - 1; index >= 4; index-- {
				reference := args[index]
				imageID := r.base.candidate.OPAImageID
				tags, digests := []string{}, []string{}
				if reference == r.base.candidate.AuthBrokerImage {
					imageID = r.base.candidate.AuthBrokerImageID
				}
				if strings.Contains(reference, "@") {
					digests = append(digests, reference)
				} else {
					tags = append(tags, reference)
				}
				payload, _ := json.Marshal(statusClusterImageObservation{ID: imageID, RepoTags: tags, RepoDigests: digests})
				_, _ = fmt.Fprintln(out, string(payload))
			}
			return nil
		}
		if len(args) >= 5 && strings.Contains(args[3], `"runtime"`) {
			_, _ = fmt.Fprintf(out, `{"id":"sha256:%s","owner":"","component":"","runtime":"","revision":"","api":"%s","lifetime":"%s","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
				strings.Repeat("6", 64), tobari.RuntimeImageAPI, tobari.RuntimeImageLifetimeCommand)
			return nil
		}
		imageID := r.base.candidate.OPAImageID
		if args[len(args)-1] == "tobari-gateway:test" {
			imageID = r.base.candidate.GatewayImageID
		} else if args[len(args)-1] == r.base.candidate.AuthBrokerImage {
			imageID = r.base.candidate.AuthBrokerImageID
		}
		_, _ = io.WriteString(out, imageID+"\n")
		return nil
	}
	if len(args) >= 2 && args[0] == "inspect" {
		name := args[len(args)-1]
		if r.componentMissing(name) {
			return r.writeMissing(name, errOut)
		}
	}
	if len(args) > 0 && args[0] == "compose" {
		r.cleanStart = false
	}
	if len(args) > 1 && (args[0] == "volume" && args[1] == "rm") {
		r.volumeRemoveCalls++
	}
	return r.base.Run(ctx, args, environment, in, out, errOut)
}

func (r *finalClusterLifecycleRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	if r.recordStatusCalls {
		r.statusCalls = append(r.statusCalls, append([]string(nil), args...))
	}
	if len(args) == 5 && args[0] == "inspect" && args[2] == appliedClusterInspectTemplate && args[3] == opaContainer && args[4] == authBrokerContainer {
		var output bytes.Buffer
		for index := len(args) - 1; index >= 3; index-- {
			name := args[index]
			payload, err := json.Marshal(r.base.component(name))
			if err != nil {
				return nil, err
			}
			_, _ = fmt.Fprintln(&output, string(payload))
		}
		return output.Bytes(), nil
	}
	if len(args) >= 2 && args[0] == "container" && args[1] == "rm" {
		id := args[len(args)-1]
		for _, name := range []string{gatewayContainer, opaContainer, authBrokerContainer} {
			if id == r.base.component(name).ContainerID {
				r.removedComponents[name] = true
				r.componentRemoveCalls[name]++
				return nil, nil
			}
		}
		return nil, fmt.Errorf("unexpected component removal %q", id)
	}
	if len(args) >= 3 && args[0] == "network" && args[1] == "rm" {
		name := args[len(args)-1]
		if r.failNetworkOnce {
			r.failNetworkOnce = false
			return []byte("injected network removal interruption"), errors.New("injected network removal interruption")
		}
		r.removedNetworks[name] = true
		r.networkRemoveCalls[name]++
		return nil, nil
	}
	if len(args) >= 2 && args[0] == "volume" && args[1] == "rm" {
		r.volumeRemoveCalls++
		return nil, nil
	}
	if len(args) != 0 {
		name := args[len(args)-1]
		if len(args) >= 2 && args[0] == "network" && args[1] == "inspect" && (name == "tobari-control" || name == "tobari-egress") {
			if r.removedNetworks[name] || r.cleanStart && !r.base.selected {
				return []byte("Error: No such network: " + name), errors.New("No such network")
			}
			if len(args) >= 4 && strings.Contains(args[3], ownerLabel) {
				return []byte(ownerValue + "\n"), nil
			}
		}
		if (args[0] == "inspect" || args[0] == "container" && len(args) > 1 && args[1] == "inspect") && r.componentMissing(name) {
			return []byte("Error: No such object: " + name), errors.New("No such object")
		}
		if len(args) >= 3 && args[0] == "inspect" && args[2] == appliedClusterInspectTemplate && (name == gatewayContainer || name == opaContainer || name == authBrokerContainer) {
			payload, err := json.Marshal(r.base.component(name))
			if err != nil {
				return nil, err
			}
			return payload, nil
		}
		if args[0] == "network" && r.removedNetworks[name] {
			return []byte("Error: No such network: " + name), errors.New("No such network")
		}
	}
	return r.base.Output(ctx, args, environment)
}

func inactiveFinalClusterCollection(t *testing.T, active tobari.WorkspaceAuthorityCollection) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	contexts := make([]tobari.WorkspaceAuthorityContextRecord, len(active.Contexts))
	for index := range active.Contexts {
		contexts[index] = active.Contexts[index].Clone()
		contexts[index].ActiveTemplatePolicy = nil
		contexts[index].ActivePolicyMemory = nil
		contexts[index].ActivePolicyMemoryRef = nil
	}
	inactive, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		active.Templates, contexts, active.Workspaces, active.PendingCandidates, active.DefaultTemplateID, &active,
	)
	if err != nil || !changed {
		t.Fatalf("publish inactive final cluster fixture: changed=%t err=%v", changed, err)
	}
	return inactive
}

func finalClusterLifecycleFixture(t *testing.T) (*Runtime, *finalClusterLifecycleRunner, tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.WorkspacePolicyProjection) {
	t.Helper()
	base := finalProjectionCollectionFixture(t, "")
	previous := inactiveFinalClusterCollection(t, base)
	transition, err := tobari.PlanWorkspaceAuthorityClusterReconciliation(previous)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tobari.BuildActiveWorkspacePolicyProjection(transition.Next)
	if err != nil {
		t.Fatal(err)
	}
	runtime, baseRunner, _ := finalGatewayCoordinatorPlanFixture(t, transition.Next, plan)
	if err := runtime.removeFinalGatewaySettlementJournal(); err != nil {
		t.Fatal(err)
	}
	runner := newFinalClusterLifecycleRunner(baseRunner)
	runtime.runner = runner
	return runtime, runner, previous, transition.Next, plan
}

func reconcileCleanFinalCluster(t *testing.T) (*Runtime, *finalClusterLifecycleRunner, tobari.WorkspaceAuthorityCollection) {
	t.Helper()
	runtime, runner, previous, active, _ := finalClusterLifecycleFixture(t)
	if err := runtime.ReconcileFinalClusterAuthority(context.Background(), previous, active, "cluster-up", "clean-cluster-up"); err != nil {
		t.Fatalf("reconcile clean final cluster: %v", err)
	}
	return runtime, runner, active
}

func TestStatusHomeProductionAdaptersStayWithinFrozenDockerCallBudget(t *testing.T) {
	runtime, runner, active := reconcileCleanFinalCluster(t)
	workspaceFixture := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	snapshots, err := workspaceFixture.ContextSnapshots()
	if err != nil || len(snapshots) != 1 || snapshots[0].Workspace == nil {
		t.Fatalf("status authority snapshots=%d err=%v", len(snapshots), err)
	}
	plan, err := tobari.BuildHotWorkspacePolicyProjection(workspaceFixture, finalProjectionContextID)
	if err != nil || len(plan.Contexts) != 1 || plan.Contexts[0].Principal == nil {
		t.Fatalf("status Workspace plan=%#v err=%v", plan, err)
	}
	principal := *plan.Contexts[0].Principal
	runner.base.workspaces[principal.WorkspaceID] = &principal
	resolver := runtime.images.(testImageResolver)
	resolver.runtimeImage = tobari.OfficialRuntimeBase
	runtime.images = resolver
	binding, err := runtime.standardRuntimeManifest().Binding(1)
	if err != nil {
		t.Fatal(err)
	}

	runner.statusCalls = nil
	runner.recordStatusCalls = true
	defer func() { runner.recordStatusCalls = false }()
	cluster, err := runtime.ObserveFinalCluster(context.Background(), active, true)
	if err != nil || cluster.Runtime != tobari.FinalClusterRuntimeRunning {
		t.Fatalf("status cluster=%#v err=%v", cluster, err)
	}
	runtimeStatus, err := runtime.ObserveStatusRuntime(context.Background(), binding)
	if err != nil || runtimeStatus.Authority != tobari.StatusRuntimeAuthorityReady || runtimeStatus.Availability != tobari.RuntimeAvailabilityAvailable {
		t.Fatalf("status Runtime=%#v err=%v", runtimeStatus, err)
	}
	workspace, err := runtime.ObserveStatusWorkspace(context.Background(), snapshots[0])
	if err != nil || workspace.State != tobari.StatusWorkspaceRuntimeRunning {
		t.Fatalf("status Workspace=%#v err=%v", workspace, err)
	}
	if got := len(runner.statusCalls); got != tobari.StatusHomeDockerCallsPerAttempt {
		t.Fatalf("production status Docker calls=%d want=%d: %v", got, tobari.StatusHomeDockerCallsPerAttempt, runner.statusCalls)
	}
	imageAuthorityCalls := 0
	for _, call := range runner.statusCalls {
		if len(call) >= 5 && call[0] == "image" && call[1] == "inspect" && strings.Contains(call[3], `"repo_tags"`) {
			imageAuthorityCalls++
			wantReferences := 1
			if brokerRuntimeEnabled {
				wantReferences = 2
			}
			if len(call[4:]) != wantReferences {
				t.Fatalf("cluster image authority references=%v want=%d", call[4:], wantReferences)
			}
		}
	}
	if imageAuthorityCalls != 1 {
		t.Fatalf("cluster image authority Docker calls=%d want=1: %v", imageAuthorityCalls, runner.statusCalls)
	}
}

func TestFinalClusterCleanDaemonReconcilesThroughBootstrapAndPublishesActiveAuthority(t *testing.T) {
	runtime, runner, active := reconcileCleanFinalCluster(t)
	for path, mode := range map[string]os.FileMode{
		runtime.hostLoopbackDirectory():                    0o700,
		runtime.hostLoopbackRegistryPath():                 0o600,
		runtime.attachmentGrantRegistryPath():              0o600,
		runtime.interactiveAttachmentDirectory():           0o700,
		runtime.interactiveAttachmentSessionRegistryPath(): 0o600,
	} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("canonical attachment authority %s: %v", path, err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("canonical attachment authority %s mode=%v want=%o", path, info.Mode().Perm(), mode)
		}
	}
	if runner.base.composeCalls != 1 {
		t.Fatalf("clean activation Compose calls=%d, want one journaled exact bootstrap", runner.base.composeCalls)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("clean activation omitted active receipt: %v", err)
	}
	if _, present, err := runtime.readFinalClusterBootstrap(); err != nil || present {
		t.Fatalf("clean activation retained bootstrap journal: present=%t err=%v", present, err)
	}
	status, err := runtime.ObserveFinalCluster(context.Background(), active, true)
	if err != nil || status.Runtime != tobari.FinalClusterRuntimeRunning || status.Receipt != tobari.FinalClusterReceiptActive || status.Validate() != nil {
		t.Fatalf("clean activation status=%#v err=%v validate=%v", status, err, status.Validate())
	}
	wantComponents := 2
	if brokerRuntimeEnabled {
		wantComponents = 4
	}
	if len(status.Components) != wantComponents {
		t.Fatalf("surface-selected component closure=%#v", status.Components)
	}
}

func TestFinalClusterBootstrapRepairsMissingGatewayControlBeforeReadiness(t *testing.T) {
	runtime, runner, previous, active, _ := finalClusterLifecycleFixture(t)
	for index, network := range runner.base.candidate.GatewayNetworks {
		if network.Name != "tobari-control" {
			continue
		}
		missing := network
		runner.missingGateway = &missing
		runner.base.candidate.GatewayNetworks = append(
			append([]FinalGatewayNetworkAddress{}, runner.base.candidate.GatewayNetworks[:index]...),
			runner.base.candidate.GatewayNetworks[index+1:]...,
		)
		break
	}
	if runner.missingGateway == nil {
		t.Fatal("fixture has no Gateway control attachment")
	}
	if err := runtime.ReconcileFinalClusterAuthority(context.Background(), previous, active, "cluster-up", "clean-cluster-topology"); err != nil {
		t.Fatalf("reconcile clean final cluster topology: %v", err)
	}
	if runner.networkConnectCalls != 1 {
		t.Fatalf("Gateway control repair calls=%d, want one", runner.networkConnectCalls)
	}
	if len(runner.base.restartCalls) == 0 || !slices.Equal(runner.base.restartCalls[0], []string{gatewayContainer}) {
		t.Fatalf("Gateway topology restart calls=%v", runner.base.restartCalls)
	}
}

func TestFinalClusterCleanActivationInterruptionReplaysWithoutSecondPhysicalSettlement(t *testing.T) {
	runtime, runner, previous, active, _ := finalClusterLifecycleFixture(t)
	injected := errors.New("interrupt clean activation after physical settlement")
	interrupted := false
	runtime.finalGatewayAfterEffect = func(boundary string) error {
		if boundary == "components_replaced" && !interrupted {
			interrupted = true
			return injected
		}
		return nil
	}
	if err := runtime.ReconcileFinalClusterAuthority(context.Background(), previous, active, "cluster-up", "clean-cluster-up-replay"); !errors.Is(err, injected) {
		t.Fatalf("clean activation interruption=%v", err)
	}
	composeCalls := runner.base.composeCalls
	if composeCalls != 1 {
		t.Fatalf("physical activation calls before replay=%d", composeCalls)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interruption published active receipt early: %v", err)
	}
	runtime.finalGatewayAfterEffect = nil
	if err := runtime.ReconcileFinalClusterAuthority(context.Background(), previous, active, "cluster-up", "clean-cluster-up-replay"); err != nil {
		t.Fatalf("clean activation replay: %v", err)
	}
	if runner.base.composeCalls != composeCalls {
		t.Fatalf("same-action replay repeated physical settlement: %d -> %d", composeCalls, runner.base.composeCalls)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("replay omitted active receipt: %v", err)
	}
}

func finalClusterDownTransition(t *testing.T, active tobari.WorkspaceAuthorityCollection) tobari.WorkspaceAuthorityClusterDownTransition {
	t.Helper()
	transition, err := tobari.PlanWorkspaceAuthorityClusterDown(active)
	if err != nil {
		t.Fatal(err)
	}
	return transition
}

func finalClusterDownJournalFixture(t *testing.T, runtime *Runtime, runner *finalClusterLifecycleRunner, active tobari.WorkspaceAuthorityCollection, phase string) (finalClusterStoppedReceipt, tobari.WorkspaceAuthorityCollection) {
	t.Helper()
	transition := finalClusterDownTransition(t, active)
	receipt, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	journal := finalClusterStoppedReceipt{
		SchemaVersion: finalClusterStoppedSchema, Operation: "cluster-down", DecisionRef: "down-phase-replay", Phase: phase,
		PreviousGeneration: active.Generation, PreviousRevision: active.Revision,
		NextGeneration: transition.Next.Generation, NextRevision: transition.Next.Revision,
		Active: receipt, Gateway: runner.base.component(gatewayContainer), OPA: runner.base.component(opaContainer),
	}
	if brokerRuntimeEnabled {
		broker := runner.base.component(authBrokerContainer)
		journal.AuthBroker, journal.CompanionState = &broker, "ready"
	}
	return journal, transition.Next
}

func markFinalClusterRuntimeRemoved(runner *finalClusterLifecycleRunner) {
	for _, name := range []string{gatewayContainer, opaContainer} {
		runner.removedComponents[name] = true
	}
	if brokerRuntimeEnabled {
		runner.removedComponents[authBrokerContainer] = true
	}
	for _, name := range []string{"tobari-control", "tobari-egress"} {
		runner.removedNetworks[name] = true
	}
}

func assertFinalClusterDownConsequence(t *testing.T, runtime *Runtime, runner *finalClusterLifecycleRunner, next tobari.WorkspaceAuthorityCollection) {
	t.Helper()
	if err := runtime.ConfirmFinalClusterDownSettled(context.Background(), next); err != nil {
		t.Fatalf("confirm final cluster down: %v", err)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("down retained active receipt: %v", err)
	}
	if _, present, err := runtime.readFinalClusterStopped(runtime.finalClusterStoppedReceiptPath()); err != nil || !present {
		t.Fatalf("down stopped receipt present=%t err=%v", present, err)
	}
	if _, present, err := runtime.readFinalClusterStopped(runtime.finalClusterDownJournalPath()); err != nil || present {
		t.Fatalf("down journal present=%t err=%v", present, err)
	}
	if runner.volumeRemoveCalls != 0 {
		t.Fatalf("down attempted retained volume removal %d times", runner.volumeRemoveCalls)
	}
	for _, volume := range []string{"tobari-gateway-ca", policyBundleVolume, "tobari-public-ca"} {
		if err := runtime.verifyOwned(context.Background(), "volume", volume); err != nil {
			t.Fatalf("retained volume %s: %v", volume, err)
		}
	}
	status, err := runtime.ObserveFinalCluster(context.Background(), next, true)
	if err != nil || status.Runtime != tobari.FinalClusterRuntimeStopped || status.Receipt != tobari.FinalClusterReceiptStopped || status.Validate() != nil {
		t.Fatalf("stopped status=%#v err=%v validate=%v", status, err, status.Validate())
	}
}

func TestFinalClusterDownRemovesExactSurfaceAndRetainsOwnedVolumes(t *testing.T) {
	runtime, runner, active := reconcileCleanFinalCluster(t)
	transition := finalClusterDownTransition(t, active)
	if err := runtime.SettleFinalClusterDown(context.Background(), active, transition.Next, "cluster-down", "normal-down"); err != nil {
		t.Fatal(err)
	}
	assertFinalClusterDownConsequence(t, runtime, runner, transition.Next)
	for _, name := range []string{gatewayContainer, opaContainer} {
		if runner.componentRemoveCalls[name] != 1 {
			t.Fatalf("component %s removal calls=%d", name, runner.componentRemoveCalls[name])
		}
	}
	if brokerRuntimeEnabled && runner.componentRemoveCalls[authBrokerContainer] != 1 {
		t.Fatalf("Auth Broker removal calls=%d", runner.componentRemoveCalls[authBrokerContainer])
	}
}

func TestFinalClusterDownInterruptionAndDurablePhasesReplayExactly(t *testing.T) {
	t.Run("physical removal before runtime phase", func(t *testing.T) {
		runtime, runner, active := reconcileCleanFinalCluster(t)
		transition := finalClusterDownTransition(t, active)
		runner.failNetworkOnce = true
		if err := runtime.SettleFinalClusterDown(context.Background(), active, transition.Next, "cluster-down", "interrupted-down"); err == nil {
			t.Fatal("injected down interruption succeeded")
		}
		journal, present, err := runtime.readFinalClusterStopped(runtime.finalClusterDownJournalPath())
		if err != nil || !present || journal.Phase != finalClusterDownPrepared {
			t.Fatalf("interrupted down journal=%#v present=%t err=%v", journal, present, err)
		}
		if err := runtime.SettleFinalClusterDown(context.Background(), active, transition.Next, "cluster-down", "interrupted-down"); err != nil {
			t.Fatalf("same-action down replay: %v", err)
		}
		assertFinalClusterDownConsequence(t, runtime, runner, transition.Next)
		for name, calls := range runner.componentRemoveCalls {
			if calls != 1 {
				t.Fatalf("component %s repeated removal %d times", name, calls)
			}
		}
	})

	for _, phase := range []string{finalClusterDownRuntime, finalClusterDownAuthority} {
		t.Run(phase, func(t *testing.T) {
			runtime, runner, active := reconcileCleanFinalCluster(t)
			journal, next := finalClusterDownJournalFixture(t, runtime, runner, active, phase)
			markFinalClusterRuntimeRemoved(runner)
			if err := runtime.writeFinalClusterStopped(runtime.finalClusterDownJournalPath(), journal); err != nil {
				t.Fatal(err)
			}
			if err := runtime.SettleFinalClusterDown(context.Background(), active, next, journal.Operation, journal.DecisionRef); err != nil {
				t.Fatalf("resume durable phase %s: %v", phase, err)
			}
			assertFinalClusterDownConsequence(t, runtime, runner, next)
		})
	}
}

func TestFinalClusterStatusReportsEveryValidLifecycleJournalAndCollectionDrift(t *testing.T) {
	t.Run("final envelope without active receipt is fresh absence", func(t *testing.T) {
		runtime, _, previous, _, _ := finalClusterLifecycleFixture(t)
		status, err := runtime.ObserveFinalCluster(context.Background(), previous, true)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeAbsent || status.Receipt != tobari.FinalClusterReceiptAbsent {
			t.Fatalf("fresh final status=%#v err=%v", status, err)
		}
	})

	t.Run("bootstrap", func(t *testing.T) {
		runtime, _, _, active, plan := finalClusterLifecycleFixture(t)
		if err := runtime.ensureFinalClusterBaseComponents(context.Background(), plan, "cluster-up", "status-bootstrap"); err != nil {
			t.Fatal(err)
		}
		status, err := runtime.ObserveFinalCluster(context.Background(), active, true)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeDrifted || status.Receipt != tobari.FinalClusterReceiptUnknown {
			t.Fatalf("valid bootstrap journal status=%#v err=%v", status, err)
		}
	})

	t.Run("gateway settlement", func(t *testing.T) {
		runtime, runner, _, active, _ := finalClusterLifecycleFixture(t)
		runner.cleanStart = false
		_, _, journal := finalGatewayCoordinatorFixture(t)
		if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
			t.Fatal(err)
		}
		status, err := runtime.ObserveFinalCluster(context.Background(), active, true)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeDrifted || status.Receipt != tobari.FinalClusterReceiptUnknown {
			t.Fatalf("valid Gateway journal status=%#v err=%v", status, err)
		}
	})

	t.Run("down", func(t *testing.T) {
		runtime, runner, active := reconcileCleanFinalCluster(t)
		journal, _ := finalClusterDownJournalFixture(t, runtime, runner, active, finalClusterDownPrepared)
		if err := runtime.writeFinalClusterStopped(runtime.finalClusterDownJournalPath(), journal); err != nil {
			t.Fatal(err)
		}
		status, err := runtime.ObserveFinalCluster(context.Background(), active, true)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeDrifted || status.Receipt != tobari.FinalClusterReceiptUnknown {
			t.Fatalf("valid down journal status=%#v err=%v", status, err)
		}
	})

	t.Run("active receipt collection drift", func(t *testing.T) {
		runtime, _, active := reconcileCleanFinalCluster(t)
		drifted := inactiveFinalClusterCollection(t, active)
		status, err := runtime.ObserveFinalCluster(context.Background(), drifted, true)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeDrifted || status.Receipt != tobari.FinalClusterReceiptDrifted {
			t.Fatalf("active receipt collection drift status=%#v err=%v", status, err)
		}
	})

	t.Run("active receipt without final envelope", func(t *testing.T) {
		runtime, _, _ := reconcileCleanFinalCluster(t)
		status, err := runtime.ObserveFinalCluster(context.Background(), tobari.WorkspaceAuthorityCollection{}, false)
		if err != nil || status.Runtime != tobari.FinalClusterRuntimeDrifted || status.Receipt != tobari.FinalClusterReceiptDrifted {
			t.Fatalf("orphaned active receipt status=%#v err=%v", status, err)
		}
	})
}
