package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func finalSettlementComponentFixture(profile tobari.SharedClusterAppliedProfile) (finalGatewaySettlementCandidate, appliedClusterComponentObservation, appliedClusterComponentObservation) {
	candidate := finalGatewaySettlementCandidate{
		GatewayImageID: "sha256:" + strings.Repeat("a", 64), OPAImageID: "sha256:" + strings.Repeat("b", 64),
		ReviewedGateway: strings.Repeat("d", 64),
		GatewayEnv:      selectedFinalGatewayEnvironment(profile), Profile: profile,
		Principals: []FinalWorkspacePrincipalRow{},
		GatewayNetworks: []FinalGatewayNetworkAddress{
			{Name: "tobari-control", Address: "172.28.0.2"},
			{Name: "tobari-egress", Address: "172.29.0.2"},
		},
		OPANetworks:     []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: "172.28.0.3"}},
		GatewayArtifact: tobari.SemanticDigest("sha256:" + strings.Repeat("c", 64)),
	}
	if brokerRuntimeEnabled {
		candidate.AuthBrokerImage = "tobari-auth-broker:successor"
		candidate.AuthBrokerImageID = "sha256:" + strings.Repeat("9", 64)
		candidate.AuthBrokerNetworks = []FinalGatewayNetworkAddress{
			{Name: "tobari-control", Address: "172.28.0.4"},
			{Name: "tobari-egress", Address: "172.29.0.4"},
		}
	}
	gateway := appliedClusterComponentObservation{
		ContainerID: strings.Repeat("d", 64), Owner: ownerValue, Component: "gateway", Role: gatewayRole,
		ImageID: candidate.GatewayImageID, State: "running", Health: "healthy",
		Environment:       append([]string{"PATH=/usr/bin"}, candidate.GatewayEnv...),
		MountDestinations: finalGatewayMountDestinations(profile),
		NetworkAddresses:  map[string]string{"tobari-control": "172.28.0.2", "tobari-egress": "172.29.0.2"},
	}
	opa := appliedClusterComponentObservation{
		ContainerID: strings.Repeat("e", 64), Owner: ownerValue, Component: "opa",
		ImageID: candidate.OPAImageID, State: "running", Health: "healthy",
		Environment: []string{"PATH=/usr/bin"}, MountDestinations: finalOPAMountDestinations(),
		NetworkAddresses: map[string]string{"tobari-control": "172.28.0.3"},
	}
	return candidate, gateway, opa
}

func TestFinalSettlementEffectClassRequiresExactSelectedComponentClosure(t *testing.T) {
	t.Setenv("TOBARI_OPA_TIMEOUT_SECONDS", "")
	t.Setenv("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "")
	candidate, gateway, opa := finalSettlementComponentFixture(tobari.SharedClusterProfileUnix)
	principalDigest, err := digestFinalValue(candidate.Principals)
	if err != nil {
		t.Fatal(err)
	}
	active := finalPolicyActivationRecord{
		Material: FinalWorkspacePolicyProjection{Gateway: FinalGatewayComponentAuthority{Networks: candidate.GatewayNetworks}},
		Receipt:  FinalAggregatePublicationReceipt{GatewayArtifact: candidate.GatewayArtifact, PrincipalDigest: principalDigest},
	}
	registry := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}}
	if got := classifyFinalSettlementEffect(true, active, candidate, registry, gateway, opa, true); got != finalGatewaySettlementOPA {
		t.Fatalf("exact selected closure class=%q", got)
	}
	if got := classifyFinalSettlementEffect(true, active, candidate, registry, gateway, opa, false); got != finalGatewaySettlementFull {
		t.Fatalf("live Gateway artifact drift class=%q", got)
	}

	for _, test := range []struct {
		name   string
		mutate func(*appliedClusterComponentObservation, *appliedClusterComponentObservation)
	}{
		{name: "stale Gateway same bytes", mutate: func(gateway, _ *appliedClusterComponentObservation) {
			gateway.ImageID = "sha256:" + strings.Repeat("f", 64)
		}},
		{name: "stale OPA", mutate: func(_ *appliedClusterComponentObservation, opa *appliedClusterComponentObservation) {
			opa.ImageID = "sha256:" + strings.Repeat("f", 64)
		}},
		{name: "permission profile environment", mutate: func(gateway, _ *appliedClusterComponentObservation) {
			gateway.Environment = append(gateway.Environment[:1], gateway.Environment[2:]...)
		}},
		{name: "Gateway mount closure", mutate: func(gateway, _ *appliedClusterComponentObservation) {
			gateway.MountDestinations = gateway.MountDestinations[1:]
		}},
		{name: "OPA mount closure", mutate: func(_ *appliedClusterComponentObservation, opa *appliedClusterComponentObservation) {
			opa.MountDestinations = opa.MountDestinations[:1]
		}},
		{name: "Gateway topology", mutate: func(gateway, _ *appliedClusterComponentObservation) {
			gateway.NetworkAddresses["foreign"] = "172.30.0.2"
		}},
		{name: "OPA topology", mutate: func(_ *appliedClusterComponentObservation, opa *appliedClusterComponentObservation) {
			opa.NetworkAddresses["foreign"] = "172.30.0.3"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			changedGateway := gateway
			changedGateway.Environment = append([]string(nil), gateway.Environment...)
			changedGateway.MountDestinations = append([]string(nil), gateway.MountDestinations...)
			changedGateway.NetworkAddresses = cloneStringMap(gateway.NetworkAddresses)
			changedOPA := opa
			changedOPA.MountDestinations = append([]string(nil), opa.MountDestinations...)
			changedOPA.NetworkAddresses = cloneStringMap(opa.NetworkAddresses)
			test.mutate(&changedGateway, &changedOPA)
			if got := classifyFinalSettlementEffect(true, active, candidate, registry, changedGateway, changedOPA, true); got != finalGatewaySettlementFull {
				t.Fatalf("drifted selected closure class=%q", got)
			}
		})
	}
}

func TestFinalResearchClosureDriftForcesFullSettlement(t *testing.T) {
	if !brokerRuntimeEnabled {
		t.Skip("research closure exists only on the research surface")
	}
	candidate, gateway, opa := finalSettlementComponentFixture(tobari.SharedClusterProfileUnix)
	principalDigest, err := digestFinalValue(candidate.Principals)
	if err != nil {
		t.Fatal(err)
	}
	active := finalPolicyActivationRecord{
		Material: FinalWorkspacePolicyProjection{Gateway: FinalGatewayComponentAuthority{Networks: candidate.GatewayNetworks}},
		Receipt:  FinalAggregatePublicationReceipt{GatewayArtifact: candidate.GatewayArtifact, PrincipalDigest: principalDigest},
	}
	registry := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}}
	exact := appliedClusterComponentObservation{
		ContainerID: strings.Repeat("7", 64), Owner: ownerValue, Component: "auth-broker",
		ImageID: candidate.AuthBrokerImageID, State: "running", Health: "healthy",
		NetworkAddresses: map[string]string{"tobari-control": "172.28.0.4", "tobari-egress": "172.29.0.4"},
	}
	if !selectedFinalResearchClosureExact(candidate, exact, false, "ready", nil) {
		t.Fatal("same-version healthy research closure is not exact")
	}
	if got := classifyFinalSettlementEffect(true, active, candidate, registry, gateway, opa, true); got != finalGatewaySettlementOPA {
		t.Fatalf("same-version healthy closure class=%q", got)
	}
	for _, test := range []struct {
		name      string
		broker    appliedClusterComponentObservation
		missing   bool
		companion string
	}{
		{name: "missing", broker: exact, missing: true, companion: "absent"},
		{name: "unhealthy", broker: func() appliedClusterComponentObservation { value := exact; value.Health = "unhealthy"; return value }(), companion: "ready"},
		{name: "wrong image", broker: func() appliedClusterComponentObservation {
			value := exact
			value.ImageID = "sha256:" + strings.Repeat("6", 64)
			return value
		}(), companion: "ready"},
		{name: "companion lost", broker: exact, companion: "stopped"},
	} {
		t.Run(test.name, func(t *testing.T) {
			closureExact := selectedFinalResearchClosureExact(candidate, test.broker, test.missing, test.companion, nil)
			if closureExact {
				t.Fatal("drifted research closure classified exact")
			}
			if got := classifyFinalSettlementEffect(true, active, candidate, registry, gateway, opa, closureExact); got != finalGatewaySettlementFull {
				t.Fatalf("drifted research closure class=%q", got)
			}
		})
	}
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func TestFinalSettlementReplaysJournaledManagedEnvironmentAcrossProcessAmbientDrift(t *testing.T) {
	t.Setenv("TOBARI_OPA_TIMEOUT_SECONDS", "7")
	t.Setenv("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "41")
	if brokerRuntimeEnabled {
		t.Setenv("TOBARI_AUTH_BROKER_TIMEOUT_SECONDS", "83")
	}
	selected := selectedFinalGatewayEnvironment(tobari.SharedClusterProfileUnix)
	t.Setenv("TOBARI_OPA_TIMEOUT_SECONDS", "3")
	t.Setenv("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "")
	if brokerRuntimeEnabled {
		t.Setenv("TOBARI_AUTH_BROKER_TIMEOUT_SECONDS", "70")
	}
	applied := applyFinalGatewayEnvironment(os.Environ(), selected)
	want := map[string]string{}
	for _, entry := range selected {
		key, value, _ := strings.Cut(entry, "=")
		want[key] = value
	}
	got := map[string]string{}
	for _, entry := range applied {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			if _, selectedKey := want[key]; selectedKey {
				if _, duplicate := got[key]; duplicate {
					t.Fatalf("journaled environment duplicated %s", key)
				}
				got[key] = value
			}
		}
	}
	if len(got) != len(want) {
		t.Fatalf("journaled environment incomplete: got=%v want=%v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("journaled environment %s=%q want %q", key, got[key], value)
		}
	}
	if got["TOBARI_OPA_TIMEOUT_SECONDS"] != "7" || got["TOBARI_UPSTREAM_TIMEOUT_SECONDS"] != "41" {
		t.Fatalf("ambient drift selected new timeout values: %v", got)
	}
	if brokerRuntimeEnabled && got["TOBARI_AUTH_BROKER_TIMEOUT_SECONDS"] != "83" {
		t.Fatalf("research ambient drift selected a new Broker timeout: %v", got)
	}
}

func TestFinalGatewayComposeEnvironmentPreservesGatewayConfigBindSource(t *testing.T) {
	const bindSource = "/owner/state/aggregate/revision/gateway.json"
	selected := selectedFinalGatewayEnvironment(tobari.SharedClusterProfileLoopbackTCP)
	environment := applyFinalGatewayComposeEnvironment([]string{
		"TOBARI_GATEWAY_CONFIG=" + bindSource,
		"HOME=/owner/home",
	}, selected)
	if got := environmentValue(environment, "TOBARI_GATEWAY_CONFIG"); got != bindSource {
		t.Fatalf("Gateway config bind source=%q want %q", got, bindSource)
	}
	if got := environmentValue(environment, "HOME"); got != "/owner/home" {
		t.Fatalf("Docker plugin discovery HOME=%q want host authority", got)
	}
	if got := environmentValue(environment, "TOBARI_OPA_TIMEOUT_SECONDS"); got != "2" {
		t.Fatalf("Gateway Compose timeout=%q want selected value", got)
	}
}

func TestFinalComponentMountClosureIncludesTmpfsAndInheritedImageVolume(t *testing.T) {
	if !strings.Contains(appliedClusterInspectTemplate, ".HostConfig.Tmpfs") {
		t.Fatal("applied component observation omits Docker tmpfs destinations")
	}
	want := finalGatewayMountDestinations(tobari.SharedClusterProfileLoopbackTCP)
	for _, destination := range []string{"/tmp", finalGatewayInheritedCAPath} {
		if !slices.Contains(want, destination) {
			t.Fatalf("final Gateway mount closure omits %s: %v", destination, want)
		}
	}
}

func TestFinalResearchRestartUsesJournaledSuccessorBrokerImage(t *testing.T) {
	if !brokerRuntimeEnabled {
		t.Skip("Auth Broker successor authority exists only on the research surface")
	}
	runtime, _, _ := finalGatewayCoordinatorFixture(t)
	journal, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present {
		t.Fatalf("read interrupted restart journal: present=%t err=%v", present, err)
	}
	predecessorImageID := "sha256:" + strings.Repeat("8", 64)
	if journal.Candidate.AuthBrokerImageID == predecessorImageID {
		t.Fatal("fixture did not select a distinct successor Broker image")
	}
	environment := finalGatewayReplacementEnvironment(
		[]string{"TOBARI_AUTH_BROKER_IMAGE=" + predecessorImageID}, journal.Candidate,
	)
	if got := environmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE"); got != journal.Candidate.AuthBrokerImageID {
		t.Fatalf("upgrade restart image=%q want journaled successor %q", got, journal.Candidate.AuthBrokerImageID)
	}
	for _, entry := range environment {
		if entry == "TOBARI_AUTH_BROKER_IMAGE="+predecessorImageID {
			t.Fatal("upgrade restart reused predecessor Broker identity")
		}
	}

	sameVersion := journal.Candidate
	sameVersion.AuthBrokerImageID = predecessorImageID
	environment = finalGatewayReplacementEnvironment(nil, sameVersion)
	if got := environmentValue(environment, "TOBARI_AUTH_BROKER_IMAGE"); got != predecessorImageID {
		t.Fatalf("same-version restart image=%q want %q", got, predecessorImageID)
	}
}

func TestFinalStoppedClusterRestartReplaysOneReplacementAndRetiresReceiptAfterActiveConfirmation(t *testing.T) {
	runtime, runner, initial := finalGatewayCoordinatorFixture(t)
	if brokerRuntimeEnabled {
		component, componentErr := runtime.inspectContainer(context.Background(), "auth-broker", authBrokerContainer)
		state, err := runtime.brokerState(context.Background())
		if err != nil || state != "ready" {
			t.Fatalf("research Broker fixture component=%#v componentErr=%v state=%v err=%v", component, componentErr, state, err)
		}
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatalf("activate predecessor: %v", err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	stopped := finalClusterStoppedReceipt{
		SchemaVersion: finalClusterStoppedSchema, Operation: "cluster-down", DecisionRef: "down-decision",
		Phase: finalClusterDownAuthority, PreviousGeneration: initial.PreviousGeneration, PreviousRevision: initial.PreviousRevision,
		NextGeneration: initial.NextGeneration, NextRevision: initial.NextRevision,
		Active: active, Gateway: runner.component(gatewayContainer), OPA: runner.component(opaContainer),
	}
	if brokerRuntimeEnabled {
		stopped.AuthBroker = &appliedClusterComponentObservation{
			ContainerID: strings.Repeat("7", 64), Owner: ownerValue, Component: "auth-broker",
			ImageID: "sha256:" + strings.Repeat("8", 64), State: "running", Health: "healthy",
			NetworkAddresses: map[string]string{"tobari-control": "172.28.0.4", "tobari-egress": "172.29.0.4"},
		}
		stopped.CompanionState = "ready"
	}
	if err := runtime.writeFinalClusterStopped(runtime.finalClusterStoppedReceiptPath(), stopped); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatal(err)
	}

	restart := initial
	restart.Operation, restart.DecisionRef = "cluster-up", "restart-decision"
	restart.Phase, restart.Applied = finalGatewayPhasePrincipals, nil
	if err := runtime.writeFinalGatewaySettlementJournal(restart); err != nil {
		t.Fatal(err)
	}
	runner.selected = false
	replacements := 0
	runtime.finalGatewayReplaceComponents = func(_ context.Context, selected finalGatewaySettlementCandidate) error {
		replacements++
		if selected.AuthBrokerImageID != restart.Candidate.AuthBrokerImageID {
			t.Fatal("replacement escaped the journaled Broker successor")
		}
		runner.selected = true
		return nil
	}
	injected := errors.New("interrupt after physical restart")
	runtime.finalGatewayAfterEffect = func(boundary string) error {
		if boundary == "components_replaced" {
			return injected
		}
		return nil
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), restart); !errors.Is(err, injected) {
		t.Fatalf("restart interruption=%v", err)
	}
	if replacements != 1 {
		t.Fatalf("physical replacements=%d", replacements)
	}
	if _, present, err := runtime.readFinalClusterStopped(runtime.finalClusterStoppedReceiptPath()); err != nil || !present {
		t.Fatalf("interruption lost stopped receipt: present=%t err=%v", present, err)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("interruption published active authority early: %v", err)
	}

	runtime.finalGatewayAfterEffect = nil
	recovered, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present {
		t.Fatalf("read restart recovery: present=%t err=%v", present, err)
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), recovered); err != nil {
		t.Fatalf("same-action restart recovery: %v", err)
	}
	if replacements != 1 {
		t.Fatalf("recovery repeated physical replacement: %d", replacements)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("restart omitted active receipt: %v", err)
	}
	if _, present, err := runtime.readFinalClusterStopped(runtime.finalClusterStoppedReceiptPath()); err != nil || present {
		t.Fatalf("confirmed active restart retained stopped receipt: present=%t err=%v", present, err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("mocked restart unexpectedly changed predecessor Compose count: %d", runner.composeCalls)
	}
}

func environmentValue(environment []string, key string) string {
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if ok && name == key {
			return value
		}
	}
	return ""
}

type finalSettlementReadinessRunner struct {
	candidate finalGatewaySettlementCandidate
	starting  bool
	inspect   int
}

func (r *finalSettlementReadinessRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	if slices.Contains(args, "authbroker.control") {
		_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"ready","epoch_id":"companion-e1_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`+"\n")
		return nil
	}
	if len(args) < 2 || args[0] != "inspect" {
		return nil
	}
	r.inspect++
	component := "gateway"
	containerID := strings.Repeat("d", 64)
	imageID := r.candidate.GatewayImageID
	role := gatewayRole
	environment := append([]string{"PATH=/usr/bin"}, r.candidate.GatewayEnv...)
	mounts := finalGatewayMountDestinations(r.candidate.Profile)
	networks := map[string]json.RawMessage{
		"tobari-control": json.RawMessage(`{"IPAddress":"172.28.0.2"}`),
		"tobari-egress":  json.RawMessage(`{"IPAddress":"172.29.0.2"}`),
	}
	if args[len(args)-1] == opaContainer {
		component, containerID, imageID, role = "opa", strings.Repeat("e", 64), r.candidate.OPAImageID, ""
		environment = []string{"PATH=/usr/bin"}
		mounts = finalOPAMountDestinations()
		networks = map[string]json.RawMessage{"tobari-control": json.RawMessage(`{"IPAddress":"172.28.0.3"}`)}
	} else if args[len(args)-1] == authBrokerContainer {
		component, containerID, imageID, role = "auth-broker", strings.Repeat("7", 64), r.candidate.AuthBrokerImageID, ""
		environment, mounts = nil, nil
		networks = map[string]json.RawMessage{
			"tobari-control": json.RawMessage(`{"IPAddress":"172.28.0.4"}`),
			"tobari-egress":  json.RawMessage(`{"IPAddress":"172.29.0.4"}`),
		}
	}
	health := "healthy"
	if r.starting {
		health = "starting"
		r.starting = false
	}
	payload, _ := json.Marshal(appliedClusterComponentObservation{
		ContainerID: containerID, Owner: ownerValue, Component: component, Role: role,
		ImageID: imageID, State: "running", Health: health, Environment: environment,
		MountDestinations: mounts, Networks: networks,
	})
	_, _ = out.Write(payload)
	return nil
}

func (*finalSettlementReadinessRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, nil
}

func TestFinalSettlementReadinessContinuesAfterComposeStartingAndRetryRemainsBounded(t *testing.T) {
	t.Setenv("TOBARI_OPA_TIMEOUT_SECONDS", "")
	t.Setenv("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "")
	candidate, _, _ := finalSettlementComponentFixture(tobari.SharedClusterProfileUnix)
	root := t.TempDir()
	runner := &finalSettlementReadinessRunner{candidate: candidate, starting: true}
	runtime, err := newRuntime(root+"/config", root+"/state", runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.waitForFinalSelectedComponents(context.Background(), candidate); err != nil {
		t.Fatalf("normal starting-to-healthy readiness: %v", err)
	}
	if runner.inspect < 5 {
		t.Fatalf("readiness did not wait for a complete healthy two-pass fence: inspections=%d", runner.inspect)
	}

	runner.starting = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := runtime.waitForFinalSelectedComponents(ctx, candidate); err == nil {
		t.Fatal("bounded readiness timeout succeeded")
	}
	runner.starting = false
	if err := runtime.waitForFinalSelectedComponents(context.Background(), candidate); err != nil {
		t.Fatalf("same selected candidate retry after readiness timeout: %v", err)
	}
}

func TestClusterAndFinalSettlementDurableDecisionsExcludeEachOtherBeforeEffects(t *testing.T) {
	for _, phase := range []string{clusterPhaseStarted, clusterPhaseRuntime} {
		t.Run("cluster up "+phase, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{}
			runtime, err := newRuntime(root+"/config", root+"/state", runner)
			if err != nil {
				t.Fatal(err)
			}
			profile := tobari.SharedClusterProfileUnix
			previous := appliedRuntimeState(t, root, profile)
			candidate := previous
			principals := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}}
			images := candidateClusterImages{Gateway: candidate.Applied.GatewayImageID, OPA: candidate.Applied.OPAImageID, AuthBroker: candidate.Applied.AuthBrokerImageID}
			if err := runtime.startClusterUpReconcile(&previous, candidate, profile, principals, nil, images, testCandidateComposeReceipt(t, candidate, profile)); err != nil {
				t.Fatal(err)
			}
			if phase == clusterPhaseRuntime {
				if err := runtime.markClusterUpRuntimeReconciled(candidate, principals); err != nil {
					t.Fatal(err)
				}
			}
			collection := finalProjectionCollectionFixture(t, "")
			if err := runtime.SettleFinalAuthority(context.Background(), collection, collection, finalProjectionContextID, "test", "decision"); err == nil {
				t.Fatal("final settlement accepted an interrupted cluster decision")
			}
			if len(runner.runs) != 0 || len(runner.outputs) != 0 {
				t.Fatalf("cluster journal conflict reached Docker effects: runs=%d outputs=%d", len(runner.runs), len(runner.outputs))
			}
			if err := runtime.clearClusterJournal(); err != nil {
				t.Fatal(err)
			}
			if err := runtime.requireNoInterruptedClusterReconcile(context.Background()); err != nil {
				t.Fatalf("cleared cluster recovery still blocks exact action: %v", err)
			}
		})
	}

	t.Run("active final settlement blocks both cluster mutations", func(t *testing.T) {
		root := t.TempDir()
		runner := &recordingRunner{}
		runtime, err := newRuntime(root+"/config", root+"/state", runner)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(runtime.finalGatewaySettlementRoot(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(runtime.finalGatewaySettlementJournalPath(), []byte("{ambiguous\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := runtime.ClusterUp(context.Background()); err == nil {
			t.Fatal("cluster up accepted an active or ambiguous final settlement")
		}
		if err := runtime.ClusterDown(context.Background(), runtimeState(root), false); err == nil {
			t.Fatal("cluster down accepted an active or ambiguous final settlement")
		}
		if len(runner.runs) != 0 || len(runner.outputs) != 0 {
			t.Fatalf("final decision conflict reached Docker effects: runs=%d outputs=%d", len(runner.runs), len(runner.outputs))
		}
	})
}

type finalGatewaySettlementRunner struct {
	candidate            finalGatewaySettlementCandidate
	workspaces           map[tobari.WorkspaceID]*tobari.WorkspacePolicyPrincipalAuthority
	workspaceNets        map[tobari.WorkspaceID]string
	onCompose            func()
	selected             bool
	composeCalls         int
	policyEffects        int
	companionEpoch       string
	replacementPending   bool
	gatewayOPAReachable  bool
	gatewayContainerID   string
	restartCalls         [][]string
	replacementServices  []string
	networkGuardCalls    int
	networkGuardFailures int
	events               []string
}

func TestFinalComponentNetworkConnectUsesDockerAllocationOnlyForSharedNetworks(t *testing.T) {
	for _, test := range []struct {
		name      string
		network   string
		address   string
		wantExact bool
	}{
		{name: "control", network: "tobari-control", address: "172.28.0.2"},
		{name: "egress", network: "tobari-egress", address: "172.29.0.2"},
		{name: "Workspace", network: "tobari-ws-example", address: "10.64.0.2", wantExact: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := finalComponentNetworkConnectArgs(gatewayContainer, "gateway", FinalGatewayNetworkAddress{Name: test.network, Address: test.address})
			wantPrefix := []string{"network", "connect", "--alias", "gateway"}
			if !slices.Equal(args[:len(wantPrefix)], wantPrefix) || args[len(args)-2] != test.network || args[len(args)-1] != gatewayContainer {
				t.Fatalf("connect args=%v", args)
			}
			hasIP := slices.Contains(args, "--ip")
			if hasIP != test.wantExact || hasIP && !slices.Contains(args, test.address) {
				t.Fatalf("connect args=%v exact=%t", args, test.wantExact)
			}
		})
	}
}

type finalComponentNetworkObservationRunner struct {
	recordingRunner
	networks string
}

func (r *finalComponentNetworkObservationRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	if len(args) >= 2 && args[0] == "inspect" {
		_, _ = io.WriteString(out, r.networks)
		return nil
	}
	return fmt.Errorf("unexpected network confirmation call: %v", args)
}

func TestFinalComponentNetworkConfirmationAcceptsFreshSharedAddressesOnly(t *testing.T) {
	runner := &finalComponentNetworkObservationRunner{networks: `{
  "tobari-control":{"IPAddress":"172.40.0.9","Aliases":["gateway"]},
  "tobari-egress":{"IPAddress":"172.41.0.8","Aliases":["gateway"]},
  "tobari-work-example":{"IPAddress":"10.64.0.2","Aliases":["gateway"]}
}`}
	runtime := &Runtime{runner: runner}
	expected := []FinalGatewayNetworkAddress{
		{Name: "tobari-control", Address: "172.28.0.2"},
		{Name: "tobari-egress", Address: "172.29.0.2"},
		{Name: "tobari-work-example", Address: "10.64.0.2"},
	}
	if err := runtime.confirmFinalComponentTopology(context.Background(), gatewayContainer, "gateway", expected); err != nil {
		t.Fatalf("Docker-assigned shared address was treated as stale authority: %v", err)
	}
	runner.networks = strings.Replace(runner.networks, "10.64.0.2", "10.64.0.9", 1)
	if err := runtime.confirmFinalComponentTopology(context.Background(), gatewayContainer, "gateway", expected); err == nil {
		t.Fatal("Workspace network address drift was accepted")
	}
}

func (r *finalGatewaySettlementRunner) workspaceResource(name string) (*tobari.WorkspacePolicyPrincipalAuthority, string) {
	for id, authority := range r.workspaces {
		container, network, err := tobari.ProjectResourceNames(string(id))
		if err == nil && (name == container || name == network) {
			return authority, network
		}
	}
	return nil, ""
}

func finalWorkspaceFixtureAddresses(id tobari.WorkspaceID) (string, string) {
	if strings.HasSuffix(string(id), "4") {
		return "172.30.0.4", "172.30.0.3"
	}
	return "172.30.0.2", "172.30.0.1"
}

func (r *finalGatewaySettlementRunner) component(container string) appliedClusterComponentObservation {
	candidate := r.candidate
	if container == authBrokerContainer {
		networks := map[string]json.RawMessage{}
		for _, network := range candidate.AuthBrokerNetworks {
			payload, _ := json.Marshal(map[string]any{"IPAddress": network.Address, "Aliases": []string{"auth-broker"}})
			networks[network.Name] = payload
		}
		return appliedClusterComponentObservation{
			ContainerID: strings.Repeat("7", 64), Owner: ownerValue, Component: "auth-broker",
			ImageID: candidate.AuthBrokerImageID, State: "running", Health: "healthy", Networks: networks,
		}
	}
	if container == opaContainer {
		image := candidate.OPAImageID
		health := "healthy"
		if r.replacementPending && !r.gatewayOPAReachable {
			health = "starting"
		}
		return appliedClusterComponentObservation{
			ContainerID: strings.Repeat("e", 64), Owner: ownerValue, Component: "opa", ImageID: image,
			State: "running", Health: health, Environment: []string{"PATH=/usr/bin"},
			MountDestinations: finalOPAMountDestinations(),
			Networks:          map[string]json.RawMessage{"tobari-control": json.RawMessage(`{"IPAddress":"172.28.0.3","Aliases":["opa"]}`)},
		}
	}
	image := "sha256:" + strings.Repeat("f", 64)
	if r.selected {
		image = candidate.GatewayImageID
	}
	networks := make(map[string]json.RawMessage, len(candidate.GatewayNetworks))
	for _, network := range candidate.GatewayNetworks {
		payload, _ := json.Marshal(map[string]any{"IPAddress": network.Address, "Aliases": []string{"gateway"}})
		networks[network.Name] = payload
	}
	health := "healthy"
	if r.replacementPending && !r.gatewayOPAReachable {
		health = "starting"
	}
	containerID := r.gatewayContainerID
	if containerID == "" {
		containerID = candidate.ReviewedGateway
	}
	return appliedClusterComponentObservation{
		ContainerID: containerID, Owner: ownerValue, Component: "gateway", Role: gatewayRole, ImageID: image,
		State: "running", Health: health, Environment: append([]string{"PATH=/usr/bin"}, candidate.GatewayEnv...),
		MountDestinations: finalGatewayMountDestinations(candidate.Profile),
		Networks:          networks,
	}
}

func (r *finalGatewaySettlementRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" {
		imageID := r.candidate.OPAImageID
		if r.candidate.AuthBrokerImage != "" && args[len(args)-1] == r.candidate.AuthBrokerImage {
			imageID = r.candidate.AuthBrokerImageID
		}
		_, _ = io.WriteString(out, imageID+"\n")
		return nil
	}
	if len(args) > 0 && args[0] == "compose" {
		r.events = append(r.events, "compose")
		if r.onCompose != nil {
			r.onCompose()
		}
		r.selected = true
		r.composeCalls++
		if index := slices.Index(args, "--force-recreate"); index >= 0 {
			r.replacementServices = append([]string(nil), args[index+1:]...)
			r.gatewayContainerID = strings.Repeat(strconv.Itoa(r.composeCalls%8+1), 64)
			r.replacementPending = true
			r.gatewayOPAReachable = false
		}
		return nil
	}
	if len(args) >= 3 && args[0] == "container" && args[1] == "restart" {
		r.events = append(r.events, "restart")
		r.restartCalls = append(r.restartCalls, append([]string(nil), args[2:]...))
		r.gatewayOPAReachable = true
		r.replacementPending = false
		for _, container := range args[2:] {
			_, _ = io.WriteString(out, container+"\n")
		}
		return nil
	}
	if slices.Contains(args, "authbroker.control") {
		operation := args[slices.Index(args, "authbroker.control")+1]
		state, epoch := "ready", r.companionEpoch
		switch operation {
		case "health":
			state = "unlocked"
			_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":%q}`+"\n", state)
			return nil
		case "companion_prepare":
			state = "prepared"
			index := slices.Index(args, "--epoch-id")
			if index >= 0 && index+1 < len(args) {
				r.companionEpoch = args[index+1]
				epoch = r.companionEpoch
			}
		}
		_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":%q,"epoch_id":%q}`+"\n", state, epoch)
		return nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[2] == "{{json .NetworkSettings.Networks}}" {
		payload, _ := json.Marshal(r.component(args[len(args)-1]).Networks)
		_, _ = out.Write(payload)
		return nil
	}
	if len(args) >= 2 && args[0] == "inspect" {
		observation := r.component(args[len(args)-1])
		payload, _ := json.Marshal(observation)
		_, _ = out.Write(payload)
		return nil
	}
	if len(args) >= 2 && args[0] == "container" && args[1] == "inspect" {
		observation := r.component(gatewayContainer)
		mounts := []map[string]string{{"type": "bind", "source": r.candidate.Aggregate.GatewayConfig, "destination": "/run/tobari/config/gateway.json"}}
		payload, _ := json.Marshal(map[string]any{
			"container_id": observation.ContainerID, "owner": ownerValue, "component": "gateway", "role": gatewayRole,
			"image_id": observation.ImageID, "state": observation.State, "health": observation.Health,
			"networks": observation.Networks, "mounts": mounts,
		})
		_, _ = out.Write(payload)
		return nil
	}
	if slices.Contains(args, "--interactive") && containsArgPrefix(args, "/bundle/.source-") {
		r.events = append(r.events, "policy")
		r.policyEffects++
	}
	return nil
}

func (r *finalGatewaySettlementRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) >= 4 && args[0] == "inspect" && strings.Contains(args[2], ".State.Status") && args[len(args)-1] == gatewayContainer {
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[2] == "{{.Image}}" && args[len(args)-1] == gatewayContainer {
		return []byte(r.candidate.GatewayImageID + "\n"), nil
	}
	if slices.Contains(args, networkGuardEntrypoint) {
		r.networkGuardCalls++
		r.events = append(r.events, "guard")
		if r.networkGuardFailures > 0 {
			r.networkGuardFailures--
			return []byte("injected Gateway network guard failure"), errors.New("injected Gateway network guard failure")
		}
		return []byte("tobari-network-guard " + tobari.NetworkGuardRevision + " gateway\n"), nil
	}
	if len(args) > 0 && args[0] == "image" {
		if len(args) > 0 && r.candidate.AuthBrokerImage != "" && args[len(args)-1] == r.candidate.AuthBrokerImage {
			return []byte(authBrokerMetadata("amd64", "")), nil
		}
		metadata := strings.Replace(gatewayMetadata("amd64", ""), testGatewayDigest, r.candidate.GatewayImageID, 1)
		return []byte(metadata), nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"amd64"}`), nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[len(args)-1] == authBrokerContainer && strings.Contains(args[2], ".State.Status") {
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[len(args)-1] == authBrokerContainer && strings.Contains(args[2], `"user"`) {
		uid, gid := currentIDs()
		return []byte(fmt.Sprintf(`{"id":"%s","owner":"%s","component":"auth-broker","user":"%d:%d"}`, strings.Repeat("7", 64), ownerValue, uid, gid)), nil
	}
	if len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer {
		return []byte("true"), nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[2] == "{{json .NetworkSettings.Networks}}" &&
		(args[len(args)-1] == gatewayContainer || args[len(args)-1] == opaContainer || args[len(args)-1] == authBrokerContainer) {
		payload, _ := json.Marshal(r.component(args[len(args)-1]).Networks)
		return payload, nil
	}
	if len(args) >= 4 && args[0] == "container" && args[1] == "inspect" {
		workspace, _ := r.workspaceResource(args[len(args)-1])
		if workspace == nil {
			return nil, nil
		}
		payload, _ := json.Marshal(finalWorkspaceContainerObservation{
			ID: strings.Repeat(string(workspace.WorkspaceID)[len(workspace.WorkspaceID)-1:], 64), Owner: ownerValue, Component: "tobari", Workspace: string(workspace.WorkspaceID),
			Role: projectWorkRole, Spec: string(workspace.AppliedEntry.ResolvedSpec), Running: true, Health: "healthy",
		})
		return payload, nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[len(args)-1] != gatewayContainer && args[len(args)-1] != opaContainer {
		workspace, network := r.workspaceResource(args[len(args)-1])
		if workspace == nil {
			return nil, nil
		}
		workspaceIP, _ := finalWorkspaceFixtureAddresses(workspace.WorkspaceID)
		payload, _ := json.Marshal(map[string]map[string]string{network: {"IPAddress": workspaceIP}})
		return payload, nil
	}
	if len(args) >= 5 && args[0] == "network" && args[1] == "inspect" {
		workspace, _ := r.workspaceResource(args[len(args)-1])
		if workspace == nil {
			return nil, nil
		}
		format := args[3]
		switch {
		case strings.Contains(format, ownerLabel):
			return []byte(ownerValue + "\n"), nil
		case strings.Contains(format, tobariIDLabel):
			return []byte(string(workspace.WorkspaceID) + "\n"), nil
		case strings.Contains(format, projectIDLabel):
			return []byte(string(workspace.WorkspaceID) + "\n"), nil
		case strings.Contains(format, projectRoleLabel):
			return []byte(projectNetRole + "\n"), nil
		case strings.Contains(format, ".IPAM.Config"):
			return []byte(`[{"Subnet":"172.30.0.0/24"}]`), nil
		}
	}
	if len(args) > 0 && (args[0] == "inspect" || args[0] == "volume" || args[0] == "network") {
		return []byte(ownerValue + "\n"), nil
	}
	return nil, nil
}

func finalGatewayCoordinatorFixture(t *testing.T, workspaceIDs ...tobari.WorkspaceID) (*Runtime, *finalGatewaySettlementRunner, finalGatewaySettlementJournal) {
	t.Helper()
	workspaceID := tobari.WorkspaceID("")
	if len(workspaceIDs) > 0 {
		workspaceID = workspaceIDs[0]
	}
	collection := finalProjectionCollectionFixture(t, workspaceID)
	plan, err := tobari.BuildHotWorkspacePolicyProjection(collection, finalProjectionContextID)
	if err != nil {
		t.Fatal(err)
	}
	return finalGatewayCoordinatorPlanFixture(t, collection, plan)
}

func finalGatewayCoordinatorPlanFixture(
	t *testing.T,
	collection tobari.WorkspaceAuthorityCollection,
	plan tobari.WorkspacePolicyProjection,
) (*Runtime, *finalGatewaySettlementRunner, finalGatewaySettlementJournal) {
	t.Helper()
	t.Setenv("TOBARI_OPA_TIMEOUT_SECONDS", "")
	t.Setenv("TOBARI_UPSTREAM_TIMEOUT_SECONDS", "")
	root := t.TempDir()
	runner := &finalGatewaySettlementRunner{workspaces: map[tobari.WorkspaceID]*tobari.WorkspacePolicyPrincipalAuthority{}, workspaceNets: map[tobari.WorkspaceID]string{}}
	runtime, err := newRuntimeWithData(filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{gateway: sharedImageSelection{Image: "tobari-gateway:test", RequireDigest: false}}
	if brokerRuntimeEnabled {
		runtime.images = testImageResolver{
			gateway:    sharedImageSelection{Image: "tobari-gateway:test", RequireDigest: false},
			authBroker: sharedImageSelection{Image: "tobari-auth-broker:successor", RequireDigest: false},
		}
		launcher := &fakeCredentialCompanionLauncher{}
		runtime.companion = launcher
		runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x23}, 64))
		runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
			return bytes.Repeat([]byte{0x51}, 32), nil
		}
	}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	aggregate, policyDigest, gatewayDigest, err := runtime.buildFinalSettlementArtifacts(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	version, err := runtimeassets.Version()
	if err != nil {
		t.Fatal(err)
	}
	runtimeDirectory := filepath.Join(runtime.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		t.Fatal(err)
	}
	profile := tobari.SharedClusterProfileUnix
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: runtimeDirectory, AggregateRevision: aggregate.AggregateRevision,
		ManifestCount: len(plan.Contexts), PolicyDirectory: aggregate.PolicyDirectory, GatewayConfig: aggregate.GatewayConfig, AssetVersion: version,
	}
	_, compose, err := runtime.captureCandidateComposeClosure(state, profile)
	if err != nil {
		t.Fatal(err)
	}
	principals := []FinalWorkspacePrincipalRow{}
	gatewayNetworks := []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: "172.28.0.2"}, {Name: "tobari-egress", Address: "172.29.0.2"}}
	for _, item := range plan.Contexts {
		if item.Principal == nil {
			continue
		}
		authority := *item.Principal
		_, network, namingErr := tobari.ProjectResourceNames(string(authority.WorkspaceID))
		if namingErr != nil {
			t.Fatal(namingErr)
		}
		authorityCopy := authority
		runner.workspaces[authority.WorkspaceID] = &authorityCopy
		runner.workspaceNets[authority.WorkspaceID] = network
		last := string(authority.WorkspaceID)[len(authority.WorkspaceID)-1:]
		workspaceIP, gatewayIP := finalWorkspaceFixtureAddresses(authority.WorkspaceID)
		principals = append(principals, FinalWorkspacePrincipalRow{
			ContextID: authority.ContextID, WorkspaceID: authority.WorkspaceID, TemplateID: authority.TemplateID,
			Presentation: authority.Presentation, ProjectRoot: authority.ProjectRoot, ContainerID: strings.Repeat(last, 64),
			ResolvedSpec: authority.AppliedEntry.ResolvedSpec, WorkspaceIP: workspaceIP, GatewayIP: gatewayIP, Network: network,
		})
		gatewayNetworks = append(gatewayNetworks, FinalGatewayNetworkAddress{Name: network, Address: gatewayIP})
	}
	slices.SortFunc(gatewayNetworks, func(left, right FinalGatewayNetworkAddress) int { return strings.Compare(left.Name, right.Name) })
	candidate := finalGatewaySettlementCandidate{
		Plan: plan.Clone(), Principals: principals,
		GatewayNetworks: gatewayNetworks,
		OPANetworks:     []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: "172.28.0.3"}},
		GatewayImageID:  "sha256:" + strings.Repeat("a", 64), OPAImageID: "sha256:" + strings.Repeat("b", 64),
		ReviewedGateway: strings.Repeat("d", 64), GatewayEnv: selectedFinalGatewayEnvironment(profile), Profile: profile,
		Compose: compose, Aggregate: aggregate, PolicyArtifact: policyDigest, GatewayArtifact: gatewayDigest,
	}
	if brokerRuntimeEnabled {
		candidate.AuthBrokerImage = "tobari-auth-broker:successor"
		candidate.AuthBrokerImageID = "sha256:" + strings.Repeat("9", 64)
		candidate.AuthBrokerNetworks = []FinalGatewayNetworkAddress{
			{Name: "tobari-control", Address: "172.28.0.4"},
			{Name: "tobari-egress", Address: "172.29.0.4"},
		}
	}
	runner.candidate = candidate
	journal := finalGatewaySettlementJournal{
		SchemaVersion: finalGatewaySettlementSchema, Operation: "context-entry", DecisionRef: "entry-decision",
		EffectClass: finalGatewaySettlementFull, Phase: finalGatewayPhasePrepared,
		PreviousGeneration: collection.Generation, PreviousRevision: collection.Revision,
		NextGeneration: collection.Generation, NextRevision: collection.Revision,
		PreviousPrincipals: projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{}},
		Candidate:          candidate,
	}
	if err := journal.validate(runtime); err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
		t.Fatal(err)
	}
	return runtime, runner, journal
}

func TestFinalSharedNetworkRefreshPersistsReplacementAddressesInRecoveryJournal(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	runner.candidate.GatewayNetworks = append([]FinalGatewayNetworkAddress{}, journal.Candidate.GatewayNetworks...)
	runner.candidate.OPANetworks = append([]FinalGatewayNetworkAddress{}, journal.Candidate.OPANetworks...)
	for index := range runner.candidate.GatewayNetworks {
		switch runner.candidate.GatewayNetworks[index].Name {
		case "tobari-control":
			runner.candidate.GatewayNetworks[index].Address = "172.40.0.9"
		case "tobari-egress":
			runner.candidate.GatewayNetworks[index].Address = "172.41.0.8"
		}
	}
	runner.candidate.OPANetworks[0].Address = "172.40.0.10"
	if brokerRuntimeEnabled {
		runner.candidate.AuthBrokerNetworks = append([]FinalGatewayNetworkAddress{}, journal.Candidate.AuthBrokerNetworks...)
		for index := range runner.candidate.AuthBrokerNetworks {
			if runner.candidate.AuthBrokerNetworks[index].Name == "tobari-control" {
				runner.candidate.AuthBrokerNetworks[index].Address = "172.40.0.11"
			} else {
				runner.candidate.AuthBrokerNetworks[index].Address = "172.41.0.11"
			}
		}
	}
	refreshed, err := runtime.refreshFinalSharedNetworkAddresses(context.Background(), journal.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.GatewayNetworks[0].Address != "172.40.0.9" || refreshed.GatewayNetworks[1].Address != "172.41.0.8" {
		t.Fatalf("shared addresses were not refreshed: before=%+v/%+v after=%+v/%+v", journal.Candidate.GatewayNetworks, journal.Candidate.OPANetworks, refreshed.GatewayNetworks, refreshed.OPANetworks)
	}
	journal.Candidate = refreshed
	journal.Phase = finalGatewayPhasePrincipals
	if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
		t.Fatal(err)
	}
	persisted, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present || !slices.Equal(persisted.Candidate.GatewayNetworks, refreshed.GatewayNetworks) || !slices.Equal(persisted.Candidate.OPANetworks, refreshed.OPANetworks) || !slices.Equal(persisted.Candidate.AuthBrokerNetworks, refreshed.AuthBrokerNetworks) {
		t.Fatalf("persisted=%+v present=%t err=%v refreshed=%+v", persisted.Candidate, present, err, refreshed)
	}
}

func finalReviewedMultiContextFixture(
	t *testing.T,
) (tobari.WorkspaceAuthorityCollection, tobari.WorkspaceAuthorityCollection, tobari.PolicyMemoryReviewedDecisionSet) {
	t.Helper()
	previousBase := finalProjectionCollectionFixture(t, finalProjectionWorkspaceA)
	secondTemplateID := tobari.WorkspaceTemplateID("01912345-6789-7abc-8def-0123456789d1")
	secondContextID := tobari.ContextID("01912345-6789-7abc-8def-0123456789d2")
	secondWorkspaceID := tobari.WorkspaceID("02912345-6789-7abc-8def-0123456789d4")
	secondTemplate, err := tobari.CopyWorkspaceTemplateRevision(secondTemplateID, "second", previousBase.Templates[0].Current)
	if err != nil {
		t.Fatal(err)
	}
	secondBinding := tobari.ContextBinding{
		SchemaVersion: tobari.ContextBindingSchemaVersion, ID: secondContextID,
		ProjectRoot: previousBase.Contexts[0].Context.ProjectRoot, TemplateID: secondTemplateID,
	}
	secondMemory, _, err := tobari.PublishPolicyMemory(secondContextID, []tobari.PolicyMemoryRule{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondTemplateReceipt := tobari.TemplatePolicyActivationReceipt{
		ContextID: secondContextID, TemplateID: secondTemplateID,
		PolicySliceDigest: secondTemplate.Current.Slices.PolicySliceDigest,
	}
	secondMemoryReceipt := tobari.PolicyMemoryActivationReceipt{ContextID: secondContextID, Revision: secondMemory.Revision}
	secondActive := secondMemory.Clone()
	secondRecord := tobari.WorkspaceAuthorityContextRecord{
		Context: secondBinding, PolicyMemory: secondMemory, ActiveTemplatePolicy: &secondTemplateReceipt,
		ActivePolicyMemory: &secondActive, ActivePolicyMemoryRef: &secondMemoryReceipt,
	}
	secondApplied := tobari.WorkspaceAppliedEntry{
		ContextID: secondContextID, TemplateID: secondTemplateID, TemplateRevision: secondTemplate.Current.Revision,
		EntrySliceDigest: secondTemplate.Current.Slices.EntrySliceDigest, RuntimeID: secondTemplate.Current.Slices.RuntimeID,
		RuntimeRevision: secondTemplate.Current.Slices.RuntimeRevision, ResolvedSpec: finalSessionDigest("8"),
		ReconciledAt: time.Unix(5, 0).UTC(),
	}
	secondWorkspace := tobari.WorkspaceBinding{
		SchemaVersion: tobari.WorkspaceBindingSchemaVersion, ID: secondWorkspaceID, ContextID: secondContextID,
		ProjectRoot: secondBinding.ProjectRoot, Home: "/workspace/home-" + string(secondWorkspaceID),
		CreationDefaults: secondTemplate.Current.Slices.CreationDefaultsDigest, LastSuccessfulEntry: &secondApplied,
	}
	httpEffect := tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "https", Protocol: tobari.PolicyProtocolHTTP},
		Match:                  tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "GET", Path: "/review-http",
		Segments: []string{}, Examples: []string{"/review-http"},
	}
	graphqlEffect := tobari.PolicyCandidateEffect{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{
			Scheme: "https", Protocol: tobari.PolicyProtocolGraphQL, GraphQLOperationType: "query", GraphQLRootField: "viewer",
		},
		Match: tobari.PolicyMatchExact, Host: "api.example.dev", Port: 443, Method: "POST", Path: "/graphql",
		Segments: []string{}, Examples: []string{"/graphql"},
	}
	httpCandidate, err := tobari.NewPolicyCandidateAuthority(finalProjectionContextID, finalProjectionWorkspaceA, httpEffect)
	if err != nil {
		t.Fatal(err)
	}
	graphqlCandidate, err := tobari.NewPolicyCandidateAuthority(secondContextID, secondWorkspaceID, graphqlEffect)
	if err != nil {
		t.Fatal(err)
	}
	pending := []tobari.PolicyCandidateAuthority{httpCandidate, graphqlCandidate}
	slices.SortFunc(pending, func(left, right tobari.PolicyCandidateAuthority) int { return strings.Compare(left.ID, right.ID) })
	previous, _, err := tobari.PublishWorkspaceAuthorityCollection(
		[]tobari.WorkspaceTemplate{previousBase.Templates[0].Clone(), secondTemplate},
		[]tobari.WorkspaceAuthorityContextRecord{previousBase.Contexts[0].Clone(), secondRecord},
		[]tobari.WorkspaceBinding{previousBase.Workspaces[0], secondWorkspace}, pending, previousBase.DefaultTemplateID, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	httpDecision, err := tobari.NewPolicyMemoryReviewedDecision(
		httpCandidate.ID, []tobari.PolicyCandidateAuthority{httpCandidate}, nil, tobari.PolicyMemoryAllow,
		httpCandidate.Effect.RuleBody(httpCandidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	graphqlDecision, err := tobari.NewPolicyMemoryReviewedDecision(
		graphqlCandidate.ID, []tobari.PolicyCandidateAuthority{graphqlCandidate}, nil, tobari.PolicyMemoryAllow,
		graphqlCandidate.Effect.RuleBody(graphqlCandidate.ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tobari.NewPolicyMemoryReviewedDecisionSet(
		previous, []tobari.PolicyMemoryReviewedDecision{httpDecision, graphqlDecision},
	)
	if err != nil {
		t.Fatal(err)
	}
	contexts := []tobari.WorkspaceAuthorityContextRecord{previous.Contexts[0].Clone(), previous.Contexts[1].Clone()}
	for index, decision := range set.Decisions {
		contextIndex := 0
		if contexts[0].Context.ID != decision.ContextID() {
			contextIndex = 1
		}
		rule, ruleErr := tobari.NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
		if ruleErr != nil {
			t.Fatal(ruleErr)
		}
		rules := append([]tobari.PolicyMemoryRule{}, contexts[contextIndex].PolicyMemory.Rules...)
		rules = append(rules, rule)
		memory, changed, publishErr := tobari.PublishPolicyMemory(decision.ContextID(), rules, &contexts[contextIndex].PolicyMemory)
		if publishErr != nil || !changed {
			t.Fatalf("reviewed Context %d memory: changed=%v err=%v", index, changed, publishErr)
		}
		contexts[contextIndex].PolicyMemory = memory
		active := memory.Clone()
		receipt := tobari.PolicyMemoryActivationReceipt{ContextID: decision.ContextID(), Revision: memory.Revision}
		contexts[contextIndex].ActivePolicyMemory = &active
		contexts[contextIndex].ActivePolicyMemoryRef = &receipt
	}
	next, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		previous.Templates, contexts, previous.Workspaces, []tobari.PolicyCandidateAuthority{}, previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("reviewed next collection: changed=%v err=%v", changed, err)
	}
	return previous, next, set
}

func finalReviewedNextCollection(
	t *testing.T,
	previous tobari.WorkspaceAuthorityCollection,
	set tobari.PolicyMemoryReviewedDecisionSet,
) tobari.WorkspaceAuthorityCollection {
	t.Helper()
	contexts := make([]tobari.WorkspaceAuthorityContextRecord, len(previous.Contexts))
	for index := range previous.Contexts {
		contexts[index] = previous.Contexts[index].Clone()
	}
	consumed := map[string]struct{}{}
	for _, decision := range set.Decisions {
		contextIndex := -1
		for index := range contexts {
			if contexts[index].Context.ID == decision.ContextID() {
				contextIndex = index
				break
			}
		}
		if contextIndex < 0 {
			t.Fatal("reviewed target Context is absent")
		}
		rule, err := tobari.NewPolicyMemoryRule(decision.ContextID(), decision.Decision, decision.Rule)
		if err != nil {
			t.Fatal(err)
		}
		rules := append([]tobari.PolicyMemoryRule{}, contexts[contextIndex].PolicyMemory.Rules...)
		rules = append(rules, rule)
		memory, changed, err := tobari.PublishPolicyMemory(decision.ContextID(), rules, &contexts[contextIndex].PolicyMemory)
		if err != nil || !changed {
			t.Fatalf("reviewed memory: changed=%v err=%v", changed, err)
		}
		contexts[contextIndex].PolicyMemory = memory
		active := memory.Clone()
		receipt := tobari.PolicyMemoryActivationReceipt{ContextID: decision.ContextID(), Revision: memory.Revision}
		contexts[contextIndex].ActivePolicyMemory = &active
		contexts[contextIndex].ActivePolicyMemoryRef = &receipt
		for _, candidate := range decision.Candidates {
			consumed[candidate.ID] = struct{}{}
		}
	}
	pending := make([]tobari.PolicyCandidateAuthority, 0, len(previous.PendingCandidates)-len(consumed))
	for _, candidate := range previous.PendingCandidates {
		if _, drop := consumed[candidate.ID]; !drop {
			pending = append(pending, candidate.Clone())
		}
	}
	next, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		previous.Templates, contexts, previous.Workspaces, pending, previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("reviewed next collection: changed=%v err=%v", changed, err)
	}
	return next
}

func TestFinalReviewedPolicyAuthorityUsesOneGlobalReceiptForHTTPAndGraphQL(t *testing.T) {
	previous, next, set := finalReviewedMultiContextFixture(t)
	previousPlan, err := tobari.BuildActiveWorkspacePolicyProjection(previous)
	if err != nil {
		t.Fatal(err)
	}
	runtime, runner, initial := finalGatewayCoordinatorPlanFixture(t, previous, previousPlan)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatalf("activate reviewed predecessor: %v", err)
	}
	beforeCompose := runner.composeCalls
	runner.onCompose = func() {
		journal, present, readErr := runtime.readFinalGatewaySettlementJournal()
		if readErr == nil && present {
			runner.candidate = journal.Candidate
		}
	}
	receipt, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, next, set, "policy-apply-reviewed", "reviewed-http-graphql",
	)
	if err != nil || receipt.Validate() != nil || runner.composeCalls != beforeCompose+1 {
		t.Fatalf("receipt=%#v compose=%d→%d err=%v validate=%v", receipt, beforeCompose, runner.composeCalls, err, receipt.Validate())
	}
	if !runner.gatewayOPAReachable {
		t.Fatal("replacement settlement did not restore Gateway-to-OPA reachability")
	}
	wantReplacement := []string{"gateway"}
	if brokerRuntimeEnabled {
		wantReplacement = append(wantReplacement, "auth-broker")
	}
	if !slices.Equal(runner.replacementServices, wantReplacement) {
		t.Fatalf("replacement services=%v want=%v", runner.replacementServices, wantReplacement)
	}
	if len(runner.restartCalls) == 0 || !slices.Equal(runner.restartCalls[len(runner.restartCalls)-1], []string{gatewayContainer}) {
		t.Fatalf("replacement restart order=%v", runner.restartCalls)
	}
	plan, err := tobari.BuildReviewedWorkspacePolicyProjection(next, []tobari.ContextID{finalProjectionContextID, "01912345-6789-7abc-8def-0123456789d2"})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.DecisionSetDigest != set.Digest || receipt.PlanDigest != plan.PlanDigest || receipt.ContentDigest != plan.ContentDigest {
		t.Fatalf("reviewed receipt did not bind one exact global plan: receipt=%#v plan=%#v", receipt, plan)
	}
	confirmed, err := runtime.ConfirmFinalReviewedPolicyAuthority(context.Background(), next, set)
	if err != nil || !reflect.DeepEqual(confirmed, receipt) {
		t.Fatalf("confirmed=%#v receipt=%#v err=%v", confirmed, receipt, err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil || active.Material.Plan.Mode != tobari.WorkspacePolicyProjectionReviewed || len(active.Material.Plan.TargetContextIDs) != 2 || len(active.Material.Plan.Contexts) != 2 {
		t.Fatalf("active reviewed projection=%#v err=%v", active.Material.Plan, err)
	}
	for index := range previous.Contexts {
		if !reflect.DeepEqual(previous.Contexts[index].ActiveTemplatePolicy, next.Contexts[index].ActiveTemplatePolicy) {
			t.Fatal("reviewed settlement adopted or changed a Template-policy axis")
		}
	}
	if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || present {
		t.Fatalf("reviewed global settlement retained journal: present=%v err=%v", present, err)
	}
}

func TestFinalReviewedPolicyAuthorityUsesOPAOnlyForByteIdenticalHTTPSet(t *testing.T) {
	previous, _, completeSet := finalReviewedMultiContextFixture(t)
	httpSet, err := tobari.NewPolicyMemoryReviewedDecisionSet(previous, []tobari.PolicyMemoryReviewedDecision{completeSet.Decisions[0]})
	if err != nil {
		t.Fatal(err)
	}
	next := finalReviewedNextCollection(t, previous, httpSet)
	previousPlan, err := tobari.BuildActiveWorkspacePolicyProjection(previous)
	if err != nil {
		t.Fatal(err)
	}
	runtime, runner, initial := finalGatewayCoordinatorPlanFixture(t, previous, previousPlan)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	beforeCompose := runner.composeCalls
	receipt, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, next, httpSet, "policy-apply-reviewed", "reviewed-http-only",
	)
	if err != nil || receipt.Validate() != nil || runner.composeCalls != beforeCompose {
		t.Fatalf("receipt=%#v compose=%d→%d err=%v", receipt, beforeCompose, runner.composeCalls, err)
	}
	confirmed, err := runtime.ConfirmFinalReviewedPolicyAuthority(context.Background(), next, httpSet)
	if err != nil || confirmed.DecisionSetDigest != httpSet.Digest || confirmed.ContentDigest != receipt.ContentDigest {
		t.Fatalf("confirmed=%#v receipt=%#v err=%v", confirmed, receipt, err)
	}
}

func TestFinalReviewedPolicyAuthorityRejectsDifferentValidNextBeforeAnyEffect(t *testing.T) {
	previous, _, completeSet := finalReviewedMultiContextFixture(t)
	setA, err := tobari.NewPolicyMemoryReviewedDecisionSet(previous, []tobari.PolicyMemoryReviewedDecision{completeSet.Decisions[0]})
	if err != nil {
		t.Fatal(err)
	}
	setB, err := tobari.NewPolicyMemoryReviewedDecisionSet(previous, []tobari.PolicyMemoryReviewedDecision{completeSet.Decisions[1]})
	if err != nil {
		t.Fatal(err)
	}
	nextB := finalReviewedNextCollection(t, previous, setB)
	previousPlan, err := tobari.BuildActiveWorkspacePolicyProjection(previous)
	if err != nil {
		t.Fatal(err)
	}
	runtime, runner, initial := finalGatewayCoordinatorPlanFixture(t, previous, previousPlan)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	beforeActive, err := os.ReadFile(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	beforePrincipals, err := os.ReadFile(runtime.principalRegistryPath())
	if err != nil {
		t.Fatal(err)
	}
	beforeCompose, beforePolicy := runner.composeCalls, runner.policyEffects
	if _, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, nextB, setA, "policy-apply-reviewed", "reviewed-set-a-next-b",
	); err == nil {
		t.Fatal("reviewed set A settled the valid next authority produced by set B")
	}
	afterActive, activeErr := os.ReadFile(runtime.finalPolicyActiveReceiptPath())
	afterPrincipals, principalErr := os.ReadFile(runtime.principalRegistryPath())
	if activeErr != nil || principalErr != nil || !bytes.Equal(beforeActive, afterActive) || !bytes.Equal(beforePrincipals, afterPrincipals) ||
		runner.composeCalls != beforeCompose || runner.policyEffects != beforePolicy {
		t.Fatalf("mismatched reviewed transition mutated authority: compose %d→%d policy %d→%d activeErr=%v principalErr=%v",
			beforeCompose, runner.composeCalls, beforePolicy, runner.policyEffects, activeErr, principalErr)
	}
	if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || present {
		t.Fatalf("mismatched reviewed transition published a journal: present=%t err=%v", present, err)
	}

	runner.onCompose = func() {
		journal, present, readErr := runtime.readFinalGatewaySettlementJournal()
		if readErr == nil && present {
			runner.candidate = journal.Candidate
		}
	}
	if _, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, nextB, setB, "policy-apply-reviewed", "reviewed-set-b",
	); err != nil {
		t.Fatalf("settle exact set B: %v", err)
	}
	if _, err := runtime.ConfirmFinalReviewedPolicyAuthority(context.Background(), nextB, setA); err == nil {
		t.Fatal("reviewed confirmation relabeled set B live authority as set A")
	}
	if _, err := runtime.ConfirmFinalReviewedPolicyAuthority(context.Background(), nextB, setB); err != nil {
		t.Fatalf("confirm exact set B: %v", err)
	}
}

func TestFinalReviewedPolicyAuthorityCanonicalOrderResumesOneExactSet(t *testing.T) {
	previous, next, set := finalReviewedMultiContextFixture(t)
	reversed, err := tobari.NewPolicyMemoryReviewedDecisionSet(previous, []tobari.PolicyMemoryReviewedDecision{
		set.Decisions[1], set.Decisions[0],
	})
	if err != nil || !reflect.DeepEqual(reversed, set) {
		t.Fatalf("reversed reviewed input did not preserve one set identity: reversed=%#v set=%#v err=%v", reversed, set, err)
	}
	previousPlan, err := tobari.BuildActiveWorkspacePolicyProjection(previous)
	if err != nil {
		t.Fatal(err)
	}
	runtime, runner, initial := finalGatewayCoordinatorPlanFixture(t, previous, previousPlan)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatal(err)
	}
	runner.onCompose = func() {
		journal, present, readErr := runtime.readFinalGatewaySettlementJournal()
		if readErr == nil && present {
			runner.candidate = journal.Candidate
		}
	}
	injected := errors.New("injected reviewed-set interruption")
	runtime.finalGatewayAfterEffect = func(boundary string) error {
		if boundary == "principals_published" {
			return injected
		}
		return nil
	}
	if _, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, next, set, "policy-apply-reviewed", "reviewed-canonical-order",
	); !errors.Is(err, injected) {
		t.Fatalf("first reviewed settlement error=%v", err)
	}
	if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || !present {
		t.Fatalf("reviewed interruption lost exact journal: present=%t err=%v", present, err)
	}
	runtime.finalGatewayAfterEffect = nil
	receipt, err := runtime.SettleFinalReviewedPolicyAuthority(
		context.Background(), previous, next, reversed, "policy-apply-reviewed", "reviewed-canonical-order",
	)
	if err != nil || receipt.DecisionSetDigest != set.Digest {
		t.Fatalf("canonical reversed-set recovery receipt=%#v err=%v", receipt, err)
	}
	if _, err := runtime.ConfirmFinalReviewedPolicyAuthority(context.Background(), next, reversed); err != nil {
		t.Fatalf("canonical reversed-set confirmation: %v", err)
	}
}

func TestFinalGatewaySettlementResumesEveryPostEffectBoundaryThroughSameDecision(t *testing.T) {
	for _, boundary := range []string{"components_replaced", "principals_published", "policy_active", "receipt_published"} {
		t.Run(boundary, func(t *testing.T) {
			runtime, runner, journal := finalGatewayCoordinatorFixture(t)
			injected := errors.New("injected " + boundary + " interruption")
			runtime.finalGatewayAfterEffect = func(observed string) error {
				if observed == boundary {
					return injected
				}
				return nil
			}
			if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); !errors.Is(err, injected) {
				t.Fatalf("first settlement error=%v", err)
			}
			if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || !present {
				t.Fatalf("interruption lost exact settlement journal: present=%v err=%v", present, err)
			}
			runtime.finalGatewayAfterEffect = nil
			recovered, present, err := runtime.readFinalGatewaySettlementJournal()
			if err != nil || !present {
				t.Fatalf("read same-action recovery journal: present=%v err=%v", present, err)
			}
			if err := runtime.resumeFinalGatewaySettlement(context.Background(), recovered); err != nil {
				t.Fatalf("same-action recovery: %v", err)
			}
			if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || present {
				t.Fatalf("recovery retained settlement journal: present=%v err=%v", present, err)
			}
			if runner.composeCalls != 1 {
				t.Fatalf("same-action recovery repeated component replacement %d times", runner.composeCalls)
			}
			if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
				t.Fatalf("recovery omitted exact active receipt: %v", err)
			}
		})
	}
}

func TestFinalGatewaySettlementRecoveryRejectsForgedProjectionIdentityAxes(t *testing.T) {
	for name, forge := range map[string]func(*FinalAggregateProjection){
		"evaluator": func(aggregate *FinalAggregateProjection) {
			aggregate.EvaluatorIdentity.Digest = tobari.SemanticDigest("sha256:" + strings.Repeat("7", 64))
		},
		"policy data": func(aggregate *FinalAggregateProjection) {
			aggregate.PolicyDataIdentity.Digest = tobari.SemanticDigest("sha256:" + strings.Repeat("8", 64))
		},
	} {
		t.Run(name, func(t *testing.T) {
			runtime, _, journal := finalGatewayCoordinatorFixture(t)
			forge(&journal.Candidate.Aggregate)
			if err := journal.validate(runtime); err != nil {
				t.Fatalf("syntactically valid forged journal fixture: %v", err)
			}
			if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
				t.Fatal(err)
			}
			if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err == nil || !strings.Contains(err.Error(), "aggregate differs") {
				t.Fatalf("forged %s identity recovery result=%v", name, err)
			}
			if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("forged %s identity published active receipt: %v", name, err)
			}
			if retained, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || !present || retained.Phase != finalGatewayPhasePrincipals {
				t.Fatalf("forged %s identity lost recoverable decision: phase=%q present=%t err=%v", name, retained.Phase, present, err)
			}
		})
	}
}

func TestFinalGatewaySettlementGuardsReplacementBeforePolicyAndReceipt(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err != nil {
		t.Fatalf("settle guarded replacement: %v", err)
	}
	if runner.networkGuardCalls != 1 {
		t.Fatalf("Gateway network guard calls=%d, want one", runner.networkGuardCalls)
	}
	restartIndex := slices.Index(runner.events, "restart")
	guardIndex := slices.Index(runner.events, "guard")
	policyIndex := -1
	if guardIndex >= 0 {
		if relative := slices.Index(runner.events[guardIndex+1:], "policy"); relative >= 0 {
			policyIndex = guardIndex + 1 + relative
		}
	}
	if restartIndex < 0 || guardIndex <= restartIndex || policyIndex <= guardIndex {
		t.Fatalf("replacement order=%v, want restart before guard before policy", runner.events)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("guarded replacement omitted active receipt: %v", err)
	}
}

func TestFinalGatewaySettlementGuardFailureRetainsDecisionAndRetryDoesNotReplaceAgain(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	runner.networkGuardFailures = 1
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err == nil {
		t.Fatal("Gateway network guard failure published final authority")
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("guard failure published active receipt: %v", err)
	}
	recovered, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present || recovered.Phase != finalGatewayPhasePrincipals || recovered.Applied != nil {
		t.Fatalf("guard failure decision=%#v present=%t err=%v", recovered, present, err)
	}
	if runner.composeCalls != 1 || runner.networkGuardCalls != 1 {
		t.Fatalf("guard failure effects: compose=%d guard=%d", runner.composeCalls, runner.networkGuardCalls)
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), recovered); err != nil {
		t.Fatalf("same-decision guard retry: %v", err)
	}
	if runner.composeCalls != 1 || runner.networkGuardCalls != 2 {
		t.Fatalf("guard retry repeated replacement or omitted guard: compose=%d guard=%d", runner.composeCalls, runner.networkGuardCalls)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); err != nil {
		t.Fatalf("guard retry omitted active receipt: %v", err)
	}
}

func TestFinalGatewaySettlementSecondGlobalSessionFenceBlocksOwnerAppearingBeforeReplacement(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	session := tobari.InteractiveAttachmentSession{
		SchemaVersion:       tobari.PermissionSessionSchema,
		WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789b2",
		WorkspaceID:         "01912345-6789-7abc-8def-0123456789b3",
		AttachmentID:        "att_" + strings.Repeat("9", 32), OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: strings.Repeat("9", 64), OwnerPID: os.Getpid(),
		IngestionTransport: tobari.PermissionSessionTransportUnix,
		IngestionEndpoint:  "pws_" + strings.Repeat("9", 32) + ".sock",
		IngestionNonce:     strings.Repeat("8", 64),
		CreatedAt:          now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano),
		ExpiresAt: now.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	var hookErr error
	runtime.finalGatewayAfterFirstSessionFence = func() {
		hookErr = writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
			SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{session},
		})
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err == nil {
		t.Fatal("owner appearing between global fences reached Gateway replacement")
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if runner.composeCalls != 0 {
		t.Fatalf("owner appearing between fences reached component replacement %d times", runner.composeCalls)
	}
	retained, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present || retained.Phase != finalGatewayPhaseFenced {
		t.Fatalf("owner block lost fenced same-action decision: phase=%q present=%v err=%v", retained.Phase, present, err)
	}
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
		SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{},
	}); err != nil {
		t.Fatal(err)
	}
	runtime.finalGatewayAfterFirstSessionFence = nil
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), retained); err != nil {
		t.Fatalf("same action after canonical owner closes: %v", err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("owner recovery replacement count=%d", runner.composeCalls)
	}
}

func TestFinalGatewaySettlementPublishesFirstWorkspacePrincipalAndAggregateInOneAction(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t, finalProjectionWorkspaceA)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err != nil {
		t.Fatalf("first Workspace settlement: %v", err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("first Workspace did not select the full Gateway route: compose=%d", runner.composeCalls)
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil || len(registry.Bindings) != 1 || registry.Bindings[0].ProjectID != string(finalProjectionWorkspaceA) ||
		registry.Bindings[0].WorkspaceManifestID != string(finalProjectionContextID) {
		t.Fatalf("first Workspace principal publication=%+v err=%v", registry, err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil || len(active.Material.Principals) != 1 || active.Material.Principals[0].WorkspaceID != finalProjectionWorkspaceA {
		t.Fatalf("first Workspace active aggregate=%+v err=%v", active.Material.Principals, err)
	}
	if _, present, err := runtime.readFinalGatewaySettlementJournal(); err != nil || present {
		t.Fatalf("first Workspace retained settlement journal: present=%v err=%v", present, err)
	}
}

func TestFinalGatewaySettlementNoOpUsesOPAOnlyWithoutComponentReplacement(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t, finalProjectionWorkspaceA)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err != nil {
		t.Fatal(err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil {
		t.Fatal(err)
	}
	noOp := finalGatewaySettlementJournal{
		SchemaVersion: finalGatewaySettlementSchema, Operation: "policy-allow", DecisionRef: "no-op-decision",
		EffectClass: finalGatewaySettlementOPA, Phase: finalGatewayPhasePrepared,
		PreviousGeneration: journal.NextGeneration, PreviousRevision: journal.NextRevision,
		NextGeneration: journal.NextGeneration, NextRevision: journal.NextRevision,
		PreviousActive: &active, PreviousPrincipals: registry, Candidate: journal.Candidate,
	}
	// A later action observes the replacement's current container identity;
	// reusing the predecessor candidate would not model a real prepared action.
	noOp.Candidate.ReviewedGateway = active.Material.Gateway.ContainerID
	if err := runtime.writeFinalGatewaySettlementJournal(noOp); err != nil {
		t.Fatal(err)
	}
	beforeCompose, beforePolicy := runner.composeCalls, runner.policyEffects
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), noOp); err != nil {
		t.Fatalf("OPA-only no-op settlement: %v", err)
	}
	if runner.composeCalls != beforeCompose || runner.policyEffects != beforePolicy {
		t.Fatalf("OPA-only no-op repeated effects: compose %d→%d policy %d→%d", beforeCompose, runner.composeCalls, beforePolicy, runner.policyEffects)
	}
}

func TestFinalGatewaySettlementWorkspaceDeleteRemovesPrincipalWithoutClusterPrerequisite(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	stale := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{{
		ProjectID: string(finalProjectionWorkspaceA), WorkspaceManifestID: string(finalProjectionContextID), WorkspaceManifestName: "restricted",
		ProjectRoot: "/workspace/example", WorkspaceIP: "172.30.0.2", GatewayIP: "172.30.0.1", Network: "tobari-019123456789-net",
	}}}
	if err := stale.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.replaceProjectPrincipalRegistry(context.Background(), stale.Bindings); err != nil {
		t.Fatal(err)
	}
	journal.Operation = "workspace-delete"
	journal.DecisionRef = "delete-decision"
	journal.PreviousPrincipals = stale
	if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
		t.Fatal(err)
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); err != nil {
		t.Fatalf("Workspace delete settlement: %v", err)
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil || len(registry.Bindings) != 0 {
		t.Fatalf("Workspace delete retained principal=%+v err=%v", registry, err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("Workspace delete did not reconcile Gateway topology exactly once: %d", runner.composeCalls)
	}
}

func TestFinalGatewaySettlementKeepsFenceUntilWorkspaceDeletePrincipalCAS(t *testing.T) {
	runtime, runner, journal := finalGatewayCoordinatorFixture(t)
	stale := projectPrincipalRegistry{SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{{
		ProjectID: string(finalProjectionWorkspaceA), WorkspaceManifestID: string(finalProjectionContextID), WorkspaceManifestName: "restricted",
		ProjectRoot: "/workspace/example", WorkspaceIP: "172.30.0.2", GatewayIP: "172.30.0.1", Network: "tobari-019123456789-net",
	}}}
	if err := runtime.replaceProjectPrincipalRegistry(context.Background(), stale.Bindings); err != nil {
		t.Fatal(err)
	}
	journal.Operation = "workspace-delete"
	journal.DecisionRef = "delete-fence-decision"
	journal.PreviousPrincipals = stale
	if err := runtime.writeFinalGatewaySettlementJournal(journal); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("injected death after component replacement")
	runtime.finalGatewayAfterEffect = func(boundary string) error {
		if boundary == "components_replaced" {
			return injected
		}
		return nil
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), journal); !errors.Is(err, injected) {
		t.Fatalf("replacement interruption=%v", err)
	}
	retained, present, err := runtime.readFinalGatewaySettlementJournal()
	if err != nil || !present || retained.Phase != finalGatewayPhasePrincipals || retained.Applied != nil {
		t.Fatalf("unsafe replacement boundary: phase=%q applied=%v present=%v err=%v", retained.Phase, retained.Applied != nil, present, err)
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil || len(registry.Bindings) != 0 {
		t.Fatalf("component replacement preceded candidate principal CAS: registry=%+v err=%v", registry, err)
	}
	if _, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("candidate active receipt appeared before recovery: %v", err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("component replacement count=%d", runner.composeCalls)
	}
	runtime.finalGatewayAfterEffect = nil
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), retained); err != nil {
		t.Fatalf("same-action Workspace delete recovery: %v", err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("same-action recovery repeated component replacement: %d", runner.composeCalls)
	}
}

func TestFinalGatewaySettlementContextDeleteRemovesActiveAxisInOneAction(t *testing.T) {
	runtime, runner, initial := finalGatewayCoordinatorFixture(t)
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), initial); err != nil {
		t.Fatalf("activate predecessor Context: %v", err)
	}
	previous := finalProjectionCollectionFixture(t, "")
	next, changed, err := tobari.PublishWorkspaceAuthorityCollection(
		previous.Templates, []tobari.WorkspaceAuthorityContextRecord{}, []tobari.WorkspaceBinding{}, []tobari.PolicyCandidateAuthority{},
		previous.DefaultTemplateID, &previous,
	)
	if err != nil || !changed {
		t.Fatalf("delete Context candidate: changed=%v err=%v", changed, err)
	}
	plan, err := tobari.BuildActiveWorkspacePolicyProjection(next)
	if err != nil || len(plan.Contexts) != 0 {
		t.Fatalf("deleted Context active plan=%#v err=%v", plan, err)
	}
	aggregate, policyDigest, gatewayDigest, err := runtime.buildFinalSettlementArtifacts(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	profile := tobari.SharedClusterProfileUnix
	version, err := runtimeassets.Version()
	if err != nil {
		t.Fatal(err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(runtime.stateDirectory, "runtime", version),
		AggregateRevision: aggregate.AggregateRevision, ManifestCount: finalComposeProjectionCount(0), PolicyDirectory: aggregate.PolicyDirectory,
		GatewayConfig: aggregate.GatewayConfig, AssetVersion: version,
	}
	_, compose, err := runtime.captureCandidateComposeClosure(state, profile)
	if err != nil {
		t.Fatal(err)
	}
	previousActive, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := runtime.readProjectPrincipalRegistry()
	if err != nil {
		t.Fatal(err)
	}
	candidate := finalGatewaySettlementCandidate{
		Plan: plan, Principals: []FinalWorkspacePrincipalRow{},
		GatewayNetworks: []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: "172.28.0.2"}, {Name: "tobari-egress", Address: "172.29.0.2"}},
		OPANetworks:     []FinalGatewayNetworkAddress{{Name: "tobari-control", Address: "172.28.0.3"}},
		GatewayImageID:  initial.Candidate.GatewayImageID, OPAImageID: initial.Candidate.OPAImageID,
		ReviewedGateway: initial.Candidate.ReviewedGateway, GatewayEnv: selectedFinalGatewayEnvironment(profile), Profile: profile,
		Compose: compose, Aggregate: aggregate, PolicyArtifact: policyDigest, GatewayArtifact: gatewayDigest,
	}
	if brokerRuntimeEnabled {
		candidate.AuthBrokerImage = initial.Candidate.AuthBrokerImage
		candidate.AuthBrokerImageID = initial.Candidate.AuthBrokerImageID
		candidate.AuthBrokerNetworks = append([]FinalGatewayNetworkAddress(nil), initial.Candidate.AuthBrokerNetworks...)
	}
	runner.candidate = candidate
	decision := finalGatewaySettlementJournal{
		SchemaVersion: finalGatewaySettlementSchema, Operation: "context-delete", DecisionRef: "context-delete-decision",
		EffectClass: finalGatewaySettlementFull, Phase: finalGatewayPhasePrepared,
		PreviousGeneration: previous.Generation, PreviousRevision: previous.Revision,
		NextGeneration: next.Generation, NextRevision: next.Revision,
		PreviousActive: &previousActive, PreviousPrincipals: registry, Candidate: candidate,
	}
	if err := runtime.writeFinalGatewaySettlementJournal(decision); err != nil {
		t.Fatal(err)
	}
	if err := runtime.resumeFinalGatewaySettlement(context.Background(), decision); err != nil {
		t.Fatalf("Context deletion settlement: %v", err)
	}
	active, err := runtime.readFinalPolicyActivation(runtime.finalPolicyActiveReceiptPath())
	if err != nil || len(active.Material.Plan.Contexts) != 0 {
		t.Fatalf("deleted Context remained active: contexts=%#v err=%v", active.Material.Plan.Contexts, err)
	}
	if err := runtime.ConfirmFinalContextDeletionSettled(context.Background(), next, finalProjectionContextID); err != nil {
		t.Fatalf("confirm deleted Context settlement: %v", err)
	}
	if runner.composeCalls != 1 {
		t.Fatalf("byte-identical Context deletion replaced Gateway: count=%d", runner.composeCalls)
	}
}
