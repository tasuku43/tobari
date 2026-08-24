package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
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

type finalSettlementReadinessRunner struct {
	candidate finalGatewaySettlementCandidate
	starting  bool
	inspect   int
}

func (r *finalSettlementReadinessRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
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
	candidate     finalGatewaySettlementCandidate
	workspaces    map[tobari.WorkspaceID]*tobari.WorkspacePolicyPrincipalAuthority
	workspaceNets map[tobari.WorkspaceID]string
	onCompose     func()
	selected      bool
	composeCalls  int
	policyEffects int
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
	if container == opaContainer {
		image := candidate.OPAImageID
		return appliedClusterComponentObservation{
			ContainerID: strings.Repeat("e", 64), Owner: ownerValue, Component: "opa", ImageID: image,
			State: "running", Health: "healthy", Environment: []string{"PATH=/usr/bin"},
			MountDestinations: finalOPAMountDestinations(),
			Networks:          map[string]json.RawMessage{"tobari-control": json.RawMessage(`{"IPAddress":"172.28.0.3"}`)},
		}
	}
	image := "sha256:" + strings.Repeat("f", 64)
	if r.selected {
		image = candidate.GatewayImageID
	}
	networks := make(map[string]json.RawMessage, len(candidate.GatewayNetworks))
	for _, network := range candidate.GatewayNetworks {
		payload, _ := json.Marshal(map[string]string{"IPAddress": network.Address})
		networks[network.Name] = payload
	}
	return appliedClusterComponentObservation{
		ContainerID: candidate.ReviewedGateway, Owner: ownerValue, Component: "gateway", Role: gatewayRole, ImageID: image,
		State: "running", Health: "healthy", Environment: append([]string{"PATH=/usr/bin"}, candidate.GatewayEnv...),
		MountDestinations: finalGatewayMountDestinations(candidate.Profile),
		Networks:          networks,
	}
}

func (r *finalGatewaySettlementRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	if len(args) >= 2 && args[0] == "image" && args[1] == "inspect" {
		_, _ = io.WriteString(out, r.candidate.OPAImageID+"\n")
		return nil
	}
	if len(args) > 0 && args[0] == "compose" {
		if r.onCompose != nil {
			r.onCompose()
		}
		r.selected = true
		r.composeCalls++
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
	if slices.Contains(args, "--interactive") {
		r.policyEffects++
	}
	return nil
}

func (r *finalGatewaySettlementRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) > 0 && args[0] == "image" {
		metadata := strings.Replace(gatewayMetadata("amd64", ""), testGatewayDigest, r.candidate.GatewayImageID, 1)
		return []byte(metadata), nil
	}
	if len(args) > 0 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"amd64"}`), nil
	}
	if len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer {
		return []byte("true"), nil
	}
	if len(args) >= 4 && args[0] == "inspect" && args[2] == "{{json .NetworkSettings.Networks}}" &&
		(args[len(args)-1] == gatewayContainer || args[len(args)-1] == opaContainer) {
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
