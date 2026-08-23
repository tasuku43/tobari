package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/doctor"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

type runnerCall struct{ args []string }
type recordingRunner struct {
	runs        []runnerCall
	outputs     []runnerCall
	outputData  []byte
	outputErr   error
	outputQueue [][]byte
	onOutput    func(int)
}

type localBaseBuildRunner struct {
	runs    []runnerCall
	outputs []runnerCall
}

func (r *localBaseBuildRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}

func (r *localBaseBuildRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) >= 4 && args[0] == "image" && args[1] == "inspect" && args[3] == "{{.Id}}" {
		return nil, errors.New("image not found")
	}
	return compatibleImageInspection(), nil
}

type ownershipInspectFailureRunner struct {
	outputs []runnerCall
	runs    []runnerCall
}

type interruptedClusterDownRunner struct {
	recordingRunner
	interrupted bool
}

func (r *interruptedClusterDownRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if !r.interrupted && len(args) > 0 && args[0] == "compose" && slices.Contains(args, "down") {
		r.interrupted = true
		return context.Canceled
	}
	return r.recordingRunner.Run(ctx, args, environment, input, output, errorOutput)
}

type interruptedClusterUpRunner struct {
	clusterUpProgressRunner
	interrupted bool
}

type rollbackClusterUpRunner struct {
	clusterUpProgressRunner
	activationErr               error
	rollbackErr                 error
	predecessorGatewayID        string
	predecessorOPAID            string
	predecessorBrokerID         string
	predecessorProfile          tobari.SharedClusterAppliedProfile
	predecessorRevision         string
	predecessorNetworks         map[string]string
	predecessorNetworkProjects  map[string]string
	prepareImagesErr            error
	networkErr                  error
	healthErr                   error
	cleanupErr                  error
	cleanupCalls                int
	rollbackVerificationDrift   bool
	rollbackGatewayObservations int
	policyPublishes             []string
	rollingBack                 bool
	composeCalls                []runnerCall
	composeEnvironments         [][]string
}

func (r *rollbackClusterUpRunner) bindPredecessor(state tobari.State) {
	r.predecessorGatewayID = state.Applied.GatewayImageID
	r.predecessorOPAID = state.Applied.OPAImageID
	r.predecessorBrokerID = state.Applied.AuthBrokerImageID
	r.predecessorProfile = state.Applied.PermissionProfile
	r.predecessorRevision = state.AggregateRevision
}

func (r *rollbackClusterUpRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if slices.Contains(args, "tobari-policy-publish") {
		r.policyPublishes = append(r.policyPublishes, strings.Join(args, " "))
	}
	if len(args) >= 3 && args[0] == "inspect" && args[2] == appliedClusterInspectTemplate {
		data, err := r.Output(ctx, args, environment)
		if len(data) > 0 {
			if err != nil && bytes.Contains(data, []byte("No such object")) {
				_, _ = errorOutput.Write([]byte("Error: No such object: " + args[len(args)-1] + "\n"))
			} else {
				_, _ = output.Write(data)
			}
		}
		return err
	}
	if len(args) >= 3 && args[0] == "inspect" && args[2] == "{{json .NetworkSettings.Networks}}" {
		data, err := r.Output(context.Background(), args, environment)
		if err != nil {
			_, _ = errorOutput.Write(data)
			return err
		}
		_, _ = output.Write(data)
		return nil
	}
	if len(args) > 0 && args[0] == "compose" && slices.Contains(args, "up") {
		if r.predecessorGatewayID != "" && slices.Contains(environment, "TOBARI_GATEWAY_IMAGE="+r.predecessorGatewayID) {
			r.rollingBack = true
		}
		r.composeCalls = append(r.composeCalls, runnerCall{args: append([]string{}, args...)})
		r.composeEnvironments = append(r.composeEnvironments, append([]string{}, environment...))
		switch len(r.composeCalls) {
		case 1:
			if r.activationErr != nil {
				return r.activationErr
			}
		case 2:
			if r.rollbackErr != nil {
				return r.rollbackErr
			}
		}
	}
	if r.networkErr != nil && r.composed && len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
		err := r.networkErr
		r.networkErr = nil
		return err
	}
	if len(args) > 0 && args[0] == "compose" && slices.Contains(args, "down") {
		r.cleanupCalls++
		if r.cleanupErr != nil {
			return r.cleanupErr
		}
	}
	return r.clusterUpProgressRunner.Run(ctx, args, environment, input, output, errorOutput)
}

func (r *rollbackClusterUpRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	if slices.Contains(args, "tobari-policy-publish") {
		r.policyPublishes = append(r.policyPublishes, strings.Join(args, " "))
	}
	usingPredecessor := len(r.composeCalls) == 0 || len(r.composeCalls) >= 2 || r.activationErr != nil || r.rollingBack
	if usingPredecessor && r.predecessorGatewayID != "" &&
		len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer && args[2] == "/opa" {
		if r.predecessorRevision != "" && strings.Contains(strings.Join(args, " "), strconv.Quote(r.predecessorRevision)) {
			return []byte("true\n"), nil
		}
		return nil, errors.New("requested policy revision is not active")
	}
	if usingPredecessor && len(args) >= 3 && args[0] == "inspect" && args[2] == appliedClusterInspectTemplate {
		switch args[len(args)-1] {
		case gatewayContainer:
			if r.predecessorGatewayID != "" {
				payload := appliedClusterTestPayloadForProfile(gatewayContainer, r.predecessorGatewayID, r.predecessorProfile)
				if r.rollingBack {
					r.rollbackGatewayObservations++
					if r.rollbackVerificationDrift && r.rollbackGatewayObservations > 1 {
						payload = bytes.Replace(payload, []byte(`"container_id":"`+strings.Repeat("9", 64)+`"`), []byte(`"container_id":"`+strings.Repeat("8", 64)+`"`), 1)
					}
				}
				if len(r.predecessorNetworks) != 0 {
					var observation appliedClusterComponentObservation
					_ = json.Unmarshal(payload, &observation)
					for network, address := range r.predecessorNetworks {
						observation.Networks[network] = json.RawMessage(`{"IPAddress":` + strconv.Quote(address) + `}`)
					}
					payload, _ = json.Marshal(observation)
				}
				return payload, nil
			}
		case opaContainer:
			if r.predecessorOPAID != "" {
				return appliedClusterTestPayload(opaContainer, r.predecessorOPAID), nil
			}
		case authBrokerContainer:
			if r.predecessorBrokerID != "" {
				return appliedClusterTestPayload(authBrokerContainer, r.predecessorBrokerID), nil
			}
		}
	}
	if r.prepareImagesErr != nil && len(args) >= 1 && args[0] == "image" &&
		strings.Contains(strings.Join(args, " "), tobari.RuntimeImageAPILabel) {
		err := r.prepareImagesErr
		r.prepareImagesErr = nil
		return nil, err
	}
	if r.networkErr != nil && r.composed && len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
		err := r.networkErr
		r.networkErr = nil
		return nil, err
	}
	if r.healthErr != nil && r.composed && len(args) >= 3 && args[0] == "inspect" &&
		strings.Contains(args[2], `"state":"{{.State.Status}}"`) {
		r.healthErr = nil
		return []byte("{"), nil
	}
	if len(args) >= 3 && args[0] == "network" && args[1] == "inspect" {
		network := args[len(args)-1]
		if projectID := r.predecessorNetworkProjects[network]; projectID != "" {
			format := ""
			if len(args) > 3 {
				format = args[3]
			}
			switch {
			case strings.Contains(format, ownerLabel):
				return []byte(ownerValue + "\n"), nil
			case strings.Contains(format, projectIDLabel):
				return []byte(projectID + "\n"), nil
			case strings.Contains(format, projectRoleLabel):
				return []byte(projectNetRole + "\n"), nil
			}
		}
	}
	if usingPredecessor && len(r.predecessorNetworks) != 0 && len(args) >= 3 && args[0] == "inspect" &&
		strings.Contains(args[2], "NetworkSettings.Networks") && args[len(args)-1] == gatewayContainer {
		networks := map[string]map[string]string{
			"tobari-control": {"IPAddress": "172.28.0.2"},
			"tobari-egress":  {"IPAddress": "172.29.0.2"},
		}
		for network, address := range r.predecessorNetworks {
			networks[network] = map[string]string{"IPAddress": address}
		}
		return json.Marshal(networks)
	}
	if usingPredecessor && len(args) >= 3 && args[0] == "inspect" && args[2] == "{{.Image}}" {
		switch args[len(args)-1] {
		case gatewayContainer:
			if r.predecessorGatewayID != "" {
				return []byte(r.predecessorGatewayID + "\n"), nil
			}
		case opaContainer:
			if r.predecessorOPAID != "" {
				return []byte(r.predecessorOPAID + "\n"), nil
			}
		case authBrokerContainer:
			if r.predecessorBrokerID != "" {
				return []byte(r.predecessorBrokerID + "\n"), nil
			}
		}
	}
	return r.clusterUpProgressRunner.Output(ctx, args, environment)
}

func (r *interruptedClusterUpRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if !r.interrupted && len(args) > 0 && args[0] == "compose" && slices.Contains(args, "up") {
		r.interrupted = true
		return context.Canceled
	}
	return r.clusterUpProgressRunner.Run(ctx, args, environment, input, output, errorOutput)
}

type policyProbeRunner struct {
	outputs []runnerCall
}

type boundedAppliedInspectRunner struct {
	payloads map[string][][]byte
	calls    []runnerCall
}

func (r *boundedAppliedInspectRunner) Run(
	_ context.Context, args, _ []string, _ io.Reader, output, errorOutput io.Writer,
) error {
	r.calls = append(r.calls, runnerCall{args: append([]string{}, args...)})
	container := args[len(args)-1]
	queue := r.payloads[container]
	if len(queue) == 0 {
		_, _ = errorOutput.Write([]byte("Error: No such object: " + container + "\n"))
		return errors.New("No such object")
	}
	payload := queue[0]
	if len(queue) > 1 {
		r.payloads[container] = queue[1:]
	}
	_, err := output.Write(payload)
	return err
}

func (*boundedAppliedInspectRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("unbounded Output must not be used for applied identity")
}

type clusterUpProgressRunner struct {
	events             []string
	composeEnvironment []string
	networkConnections []runnerCall
	policyQueries      []runnerCall
	companionEpoch     string
	composed           bool
	appliedImage       map[string]string
	appliedProfile     *tobari.SharedClusterAppliedProfile
	appliedGatewayNets map[string]string
	onAppliedInspect   func()
	appliedInspected   bool
}

type freshAuthorityRunner struct {
	clusterUpProgressRunner
	resourceCalls      map[string]int
	presentAt          map[string]int
	ambiguousAt        map[string]int
	cleanupStarted     bool
	remainAfterCleanup string
	onSecondFence      func()
	secondFenceCalled  bool
}

func freshListResource(args []string) (string, string, bool) {
	if len(args) < 6 || args[1] != "ls" {
		return "", "", false
	}
	kind := args[0]
	if kind != "container" && kind != "network" && kind != "volume" {
		return "", "", false
	}
	filterIndex := slices.Index(args, "--filter")
	if filterIndex < 0 || filterIndex+1 >= len(args) {
		return "", "", false
	}
	filter := args[filterIndex+1]
	filter = strings.TrimPrefix(filter, "name=^")
	filter = strings.TrimSuffix(filter, "$")
	if kind == "container" {
		filter = strings.TrimPrefix(filter, "/")
	}
	if filter == "" {
		return "", "", false
	}
	return kind, filter, true
}

func (r *freshAuthorityRunner) Run(
	ctx context.Context, args, environment []string, input io.Reader, output, errorOutput io.Writer,
) error {
	if kind, name, ok := freshListResource(args); ok {
		if r.resourceCalls == nil {
			r.resourceCalls = map[string]int{}
		}
		key := kind + ":" + name
		r.resourceCalls[key]++
		call := r.resourceCalls[key]
		if call == 2 && !r.secondFenceCalled && r.onSecondFence != nil {
			r.secondFenceCalled = true
			r.onSecondFence()
		}
		if r.ambiguousAt[key] == call {
			_, _ = io.WriteString(errorOutput, "daemon returned unrelated diagnostic\n")
			return errors.New("synthetic ambiguous inspect failure")
		}
		if r.presentAt[key] == call || (r.cleanupStarted && r.remainAfterCleanup == key) {
			_, _ = io.WriteString(output, name+"\n")
			return nil
		}
		return nil
	}
	if len(args) > 0 && args[0] == "compose" && slices.Contains(args, "down") {
		r.cleanupStarted = true
	}
	return r.clusterUpProgressRunner.Run(ctx, args, environment, input, output, errorOutput)
}

func (r *freshAuthorityRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	return r.clusterUpProgressRunner.Output(ctx, args, environment)
}

func appliedClusterTestPayload(container, imageID string) []byte {
	return appliedClusterTestPayloadForProfile(container, imageID, "")
}

func appliedClusterTestPayloadForProfile(
	container, imageID string, profile tobari.SharedClusterAppliedProfile,
) []byte {
	component, role := "", ""
	environment := []string{}
	mounts := []string{}
	switch container {
	case gatewayContainer:
		component, role = "gateway", gatewayRole
		switch profile {
		case tobari.SharedClusterProfileUnix:
			environment = []string{
				"TOBARI_PERMISSION_INGESTION_TRANSPORT=unix",
				"TOBARI_PERMISSION_INGESTION_DIRECTORY=/run/tobari/permission-ingestion",
			}
			mounts = []string{"/run/tobari/permission-ingestion"}
		case tobari.SharedClusterProfileLoopbackTCP:
			environment = []string{"TOBARI_PERMISSION_INGESTION_TRANSPORT=loopback_tcp"}
		}
		if brokerRuntimeEnabled {
			environment = append(environment,
				"TOBARI_AUTH_PROVIDER_PROJECTION="+prePlatformAuthProviderProjection,
				"TOBARI_AUTH_BROKER_SOCKET="+prePlatformAuthBrokerSocket,
				"TOBARI_AUTH_BROKER_TIMEOUT_SECONDS="+prePlatformAuthBrokerTimeout,
			)
			mounts = append(mounts, prePlatformAuthProviderMount, prePlatformAuthRuntimeMount)
		}
	case opaContainer:
		component = "opa"
	case authBrokerContainer:
		component = "auth-broker"
	}
	data, _ := json.Marshal(appliedClusterComponentObservation{
		ContainerID: strings.Repeat("9", 64), Owner: ownerValue, Component: component, Role: role,
		ImageID: imageID, State: "running", Health: "healthy",
		Environment: environment, MountDestinations: mounts, Networks: appliedTestNetworkPayload(container),
	})
	return data
}

func (r *clusterUpProgressRunner) Run(_ context.Context, args, environment []string, _ io.Reader, out, errorOutput io.Writer) error {
	if _, _, ok := freshListResource(args); ok {
		return nil
	}
	if len(args) == 5 && args[0] == "image" && args[1] == "inspect" && args[2] == "--format" && args[3] == "{{.Id}}" {
		identity := "sha256:" + strings.Repeat("2", 64)
		if args[4] == "tobari-auth-broker:dev" {
			identity = "sha256:" + strings.Repeat("3", 64)
		}
		_, _ = io.WriteString(out, identity+"\n")
		return nil
	}
	if len(args) >= 3 && args[0] == "inspect" && args[2] == appliedClusterInspectTemplate {
		if !r.appliedInspected && r.onAppliedInspect != nil {
			r.appliedInspected = true
			r.onAppliedInspect()
		}
		data, err := r.Output(context.Background(), args, environment)
		if len(data) > 0 {
			if err != nil && bytes.Contains(data, []byte("No such object")) {
				_, _ = errorOutput.Write([]byte("Error: No such object: " + args[len(args)-1] + "\n"))
			} else {
				_, _ = out.Write(data)
			}
		}
		return err
	}
	if len(args) >= 3 && args[0] == "inspect" && args[2] == "{{json .NetworkSettings.Networks}}" {
		data, err := r.Output(context.Background(), args, environment)
		if err != nil {
			_, _ = errorOutput.Write(data)
			return err
		}
		_, _ = out.Write(data)
		return nil
	}
	if len(args) >= 2 && args[0] == "container" && args[1] == "cp" {
		archive, err := exposureHelperArchiveBytes(syntheticExposureHelperELF("arm64"), "arm64", nil)
		if err != nil {
			return err
		}
		_, err = out.Write(archive)
		return err
	}
	if len(args) > 0 && args[0] == "compose" {
		r.events = append(r.events, "compose")
		r.composeEnvironment = append([]string{}, environment...)
		r.composed = true
	}
	if len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
		r.networkConnections = append(r.networkConnections, runnerCall{args: append([]string{}, args...)})
	}
	if slices.Contains(args, "authbroker.control") {
		operationIndex := slices.Index(args, "authbroker.control") + 1
		operation := args[operationIndex]
		switch operation {
		case "unlock", "health":
			_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"unlocked"}`+"\n")
		case "companion_prepare":
			epochIndex := slices.Index(args, "--epoch-id")
			r.companionEpoch = args[epochIndex+1]
			_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":"prepared","epoch_id":%q}`+"\n", r.companionEpoch)
		case "companion_status":
			_, _ = fmt.Fprintf(out, `{"schema_version":1,"ok":true,"state":"ready","epoch_id":%q}`+"\n", r.companionEpoch)
		}
	}
	return nil
}

func (r *clusterUpProgressRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	if len(args) >= 3 && args[0] == "exec" && args[1] == opaContainer && args[2] == "/opa" {
		r.policyQueries = append(r.policyQueries, runnerCall{args: append([]string{}, args...)})
		if !r.composed {
			return nil, errors.New("OPA is not running")
		}
		return []byte("true"), nil
	}
	if len(args) > 0 && args[0] == "run" {
		return []byte("tobari-network-guard v1 gateway\n"), nil
	}
	if len(args) >= 2 && args[0] == "network" && args[1] == "connect" {
		r.networkConnections = append(r.networkConnections, runnerCall{args: append([]string{}, args...)})
		return []byte{}, nil
	}
	if len(args) >= 2 && args[0] == "volume" && args[1] == "inspect" {
		return []byte(ownerValue + "\n"), nil
	}
	if len(args) >= 2 && args[0] == "volume" && args[1] == "create" {
		return []byte(policyBundleVolume + "\n"), nil
	}
	if len(args) >= 1 && args[0] == "image" {
		if strings.Contains(strings.Join(args, " "), tobari.RuntimeImageAPILabel) {
			return compatibleImageInspection(), nil
		}
		if strings.Contains(strings.Join(args, " "), exposureHelperAPILabel) {
			source, err := runtimeassets.ExposureHelperSourceVersion()
			if err != nil {
				return nil, err
			}
			return []byte(fmt.Sprintf(`{"architecture":"arm64","os":"linux","exposure_api":"1","exposure_source":%q,"permission_api":"1","permission_source":%q}`, source, source)), nil
		}
		image := args[len(args)-1]
		switch image {
		case "tobari-auth-broker:dev":
			r.events = append(r.events, "auth-broker-image")
			return []byte(authBrokerMetadata("arm64", "")), nil
		case "tobari-gateway:dev":
			r.events = append(r.events, "gateway-image")
			return []byte(gatewayMetadata("arm64", "")), nil
		default:
			return nil, fmt.Errorf("unexpected shared image inspection: %s", image)
		}
	}
	if len(args) >= 1 && args[0] == "version" {
		return []byte(`{"Os":"linux","Arch":"arm64"}`), nil
	}
	if len(args) >= 2 && args[0] == "exec" && args[1] == opaContainer {
		return []byte("true\n"), nil
	}
	if len(args) >= 3 && args[0] == "inspect" {
		container := args[len(args)-1]
		if container == authBrokerContainer && !brokerRuntimeEnabled {
			return []byte("No such object"), errors.New("No such object")
		}
		if args[2] == appliedClusterInspectTemplate {
			switch container {
			case gatewayContainer:
				profile := tobari.SharedClusterProfileUnix
				if r.appliedProfile != nil {
					profile = *r.appliedProfile
				}
				image := testGatewayDigest
				if r.appliedImage[container] != "" {
					image = r.appliedImage[container]
				}
				payload := appliedClusterTestPayloadForProfile(container, image, profile)
				if r.appliedGatewayNets != nil {
					var observation appliedClusterComponentObservation
					_ = json.Unmarshal(payload, &observation)
					observation.Networks = make(map[string]json.RawMessage, len(r.appliedGatewayNets))
					for network, address := range r.appliedGatewayNets {
						observation.Networks[network] = json.RawMessage(`{"IPAddress":` + strconv.Quote(address) + `}`)
					}
					payload, _ = json.Marshal(observation)
				}
				return payload, nil
			case opaContainer:
				image := "sha256:" + strings.Repeat("2", 64)
				if r.appliedImage[container] != "" {
					image = r.appliedImage[container]
				}
				return appliedClusterTestPayload(container, image), nil
			case authBrokerContainer:
				image := "sha256:" + strings.Repeat("3", 64)
				if r.appliedImage[container] != "" {
					image = r.appliedImage[container]
				}
				return appliedClusterTestPayload(container, image), nil
			}
		}
		if strings.Contains(args[2], `"id"`) {
			uid, gid := currentIDs()
			return []byte(fmt.Sprintf(
				`{"id":"%s","owner":"default","component":"auth-broker","user":"%d:%d"}`,
				strings.Repeat("a", 64), uid, gid,
			)), nil
		}
		if strings.Contains(args[2], ownerLabel) {
			return []byte(ownerValue + "\n"), nil
		}
		if strings.Contains(args[2], componentLabel) {
			switch container {
			case gatewayContainer:
				return []byte("gateway\n"), nil
			case opaContainer:
				return []byte("opa\n"), nil
			case authBrokerContainer:
				return []byte("auth-broker\n"), nil
			}
		}
		if args[2] == "{{json .Config.Env}}" || args[2] == "{{json .Mounts}}" {
			return []byte(`[]`), nil
		}
		if args[2] == "{{.Image}}" {
			switch container {
			case gatewayContainer:
				return []byte(testGatewayDigest + "\n"), nil
			case opaContainer:
				return []byte("sha256:" + strings.Repeat("2", 64) + "\n"), nil
			case authBrokerContainer:
				return []byte("sha256:" + strings.Repeat("3", 64) + "\n"), nil
			}
		}
		if strings.Contains(args[2], "NetworkSettings.Networks") {
			return []byte(`{}`), nil
		}
		return []byte(`{"state":"running","health":"healthy"}`), nil
	}
	return []byte{}, nil
}

func TestClusterUpWithProgressReportsEachRuntimeStageInOrder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &clusterUpProgressRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"),
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:dev",
		gateway:      sharedImageSelection{Image: "tobari-gateway:dev"},
		authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev"},
	}
	runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte{0x41}, 32), nil
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 256))
	var events []tobari.ClusterUpProgress
	state, err := runtime.ClusterUpWithProgress(context.Background(), func(event tobari.ClusterUpProgress) {
		events = append(events, event)
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSteps := []tobari.ClusterUpProgressStep{
		tobari.ClusterUpProgressPrepare,
		tobari.ClusterUpProgressPolicy,
		tobari.ClusterUpProgressPrepareImages,
		tobari.ClusterUpProgressStartServices,
		tobari.ClusterUpProgressConnectNetworks,
		tobari.ClusterUpProgressWaitForHealth,
		tobari.ClusterUpProgressReconcileProjects,
		tobari.ClusterUpProgressFinalize,
	}
	if len(events) != len(wantSteps)*2 {
		t.Fatalf("progress event count = %d, events = %+v", len(events), events)
	}
	for index, step := range wantSteps {
		start, complete := events[index*2], events[index*2+1]
		if start != (tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressStarted}) ||
			complete != (tobari.ClusterUpProgress{Step: step, Status: tobari.ClusterUpProgressCompleted}) {
			t.Fatalf("stage %q events = %+v, %+v", step, start, complete)
		}
	}
	gatewayIndex := slices.Index(runner.events, "gateway-image")
	composeIndex := slices.Index(runner.events, "compose")
	validOrder := gatewayIndex >= 0 && composeIndex > gatewayIndex
	if brokerRuntimeEnabled {
		authIndex := slices.Index(runner.events, "auth-broker-image")
		validOrder = authIndex >= 0 && gatewayIndex > authIndex && composeIndex > gatewayIndex
	} else if slices.Contains(runner.events, "auth-broker-image") {
		validOrder = false
	}
	if !validOrder {
		t.Fatalf("shared image preparation order = %v", runner.events)
	}
	if len(runner.policyQueries) < 2 {
		t.Fatalf("cluster up policy readiness queries = %v", runner.policyQueries)
	}
	finalPolicyQuery := strings.Join(runner.policyQueries[len(runner.policyQueries)-1].args, "\n")
	if !strings.Contains(finalPolicyQuery, state.AggregateRevision) ||
		!strings.Contains(finalPolicyQuery, "/v1/data/tobari/http/decision") {
		t.Fatalf("cluster up final policy readiness query = %v", runner.policyQueries[len(runner.policyQueries)-1].args)
	}
	joinedEnvironment := strings.Join(runner.composeEnvironment, "\n")
	bindings := []string{"TOBARI_GATEWAY_IMAGE=" + testGatewayDigest}
	if brokerRuntimeEnabled {
		bindings = append(bindings, "TOBARI_AUTH_BROKER_IMAGE=tobari-auth-broker:dev")
	}
	for _, binding := range bindings {
		if strings.Count(joinedEnvironment, binding) != 1 {
			t.Fatalf("compose environment lacks one verified %q binding: %s", binding, joinedEnvironment)
		}
	}
	wantNetworkConnections := [][]string{
		{"network", "connect", "--alias", "gateway", "tobari-control", gatewayContainer},
		{"network", "connect", "--alias", "opa", "tobari-control", opaContainer},
		{"network", "connect", "--alias", "gateway", "tobari-egress", gatewayContainer},
	}
	if brokerRuntimeEnabled {
		wantNetworkConnections = [][]string{
			{"network", "connect", "--alias", "gateway", "tobari-control", gatewayContainer},
			{"network", "connect", "--alias", "opa", "tobari-control", opaContainer},
			{"network", "connect", "--alias", "auth-broker", "tobari-control", authBrokerContainer},
			{"network", "connect", "--alias", "gateway", "tobari-egress", gatewayContainer},
			{"network", "connect", "--alias", "auth-broker", "tobari-egress", authBrokerContainer},
		}
	}
	if len(runner.networkConnections) != len(wantNetworkConnections) {
		t.Fatalf("shared network connections = %v", runner.networkConnections)
	}
	for index, want := range wantNetworkConnections {
		if !slices.Equal(runner.networkConnections[index].args, want) {
			t.Fatalf("shared network connection %d = %v, want %v", index, runner.networkConnections[index].args, want)
		}
	}
}

func TestClusterUpResumesAfterInterruptedComposeStartup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &interruptedClusterUpRunner{}
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"),
		runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:dev",
		gateway:      sharedImageSelection{Image: "tobari-gateway:dev"},
		authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev"},
	}
	runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte{0x41}, 32), nil
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 256))

	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted ClusterUp() error = %v, want context cancellation", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("exact fresh cleanup journal exists=%t error=%v", exists, err)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
		t.Fatalf("exact fresh cleanup state exists=%t error=%v", exists, err)
	}
	if _, err := runtime.ClusterUp(context.Background()); err != nil {
		t.Fatalf("resumed ClusterUp() error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("cluster journal after resumed startup = exists:%t error:%v", exists, err)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || !exists {
		t.Fatalf("cluster state after resumed startup = exists:%t error:%v", exists, err)
	}
}

func TestAppliedClusterObservationIsSingleCallBoundedAndStrict(t *testing.T) {
	t.Parallel()
	valid := appliedClusterTestPayload(gatewayContainer, testGatewayDigest)
	for _, test := range []struct {
		name    string
		payload []byte
		missing bool
		wantErr bool
	}{
		{name: "valid with whitespace", payload: append(append([]byte{}, valid...), []byte("\n \t")...)},
		{name: "stopped", payload: bytes.Replace(valid, []byte(`"state":"running"`), []byte(`"state":"exited"`), 1), wantErr: true},
		{name: "wrong role", payload: bytes.Replace(valid, []byte(`"role":"enforcement"`), []byte(`"role":"observer"`), 1), wantErr: true},
		{name: "trailing value", payload: append(append([]byte{}, valid...), []byte("\n{}")...), wantErr: true},
		{name: "duplicate field", payload: bytes.Replace(valid, []byte(`"owner":"default"`), []byte(`"owner":"default","owner":"default"`), 1), wantErr: true},
		{name: "overflow", payload: bytes.Repeat([]byte{'x'}, appliedClusterInspectLimit+1), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &boundedAppliedInspectRunner{payloads: map[string][][]byte{gatewayContainer: {test.payload}}}
			runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
			if err != nil {
				t.Fatal(err)
			}
			_, missing, err := runtime.observeAppliedClusterComponent(context.Background(), "gateway", gatewayContainer)
			if (err != nil) != test.wantErr || missing != test.missing {
				t.Fatalf("observation = missing:%t error:%v", missing, err)
			}
			if len(runner.calls) != 1 {
				t.Fatalf("observation calls = %d", len(runner.calls))
			}
		})
	}
	runner := &boundedAppliedInspectRunner{payloads: map[string][][]byte{}}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, missing, err := runtime.observeAppliedClusterComponent(context.Background(), "gateway", gatewayContainer); err != nil || !missing {
		t.Fatalf("missing observation = %t, %v", missing, err)
	}
}

func TestAppliedClusterObservationUsesOneSnapshotPerComponent(t *testing.T) {
	t.Parallel()
	changed := bytes.Replace(
		appliedClusterTestPayload(gatewayContainer, testGatewayDigest),
		[]byte(`"image_id":"`+testGatewayDigest+`"`),
		[]byte(`"image_id":"sha256:`+strings.Repeat("8", 64)+`"`), 1,
	)
	runner := &boundedAppliedInspectRunner{payloads: map[string][][]byte{
		gatewayContainer: {appliedClusterTestPayload(gatewayContainer, testGatewayDigest), changed},
		opaContainer:     {appliedClusterTestPayload(opaContainer, "sha256:"+strings.Repeat("2", 64))},
	}}
	if brokerRuntimeEnabled {
		runner.payloads[authBrokerContainer] = [][]byte{
			appliedClusterTestPayload(authBrokerContainer, "sha256:"+strings.Repeat("3", 64)),
		}
	}
	runtime, err := newRuntime(t.TempDir(), t.TempDir(), runner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.observeAppliedClusterSnapshot(context.Background()); err == nil {
		t.Fatal("changed component tuple passed the second observation fence")
	}
	wantCalls := 6
	if len(runner.calls) != wantCalls {
		t.Fatalf("snapshot calls=%d, want %d", len(runner.calls), wantCalls)
	}
}

func TestSchemaOneMigrationPublishesVerifiedPrePlatformEntry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &rollbackClusterUpRunner{
		predecessorGatewayID: "sha256:" + strings.Repeat("4", 64),
		predecessorOPAID:     "sha256:" + strings.Repeat("5", 64),
	}
	if brokerRuntimeEnabled {
		runner.predecessorBrokerID = "sha256:" + strings.Repeat("6", 64)
	}
	runtime := configuredClusterUpRuntime(t, root, runner)
	legacy := runtimeState(root)
	legacy.RuntimeDirectory = prePlatformRuntimeDirectory(root)
	legacy.AssetVersion = prePlatformAssetVersion
	materializePrePlatformRuntime(t, legacy.RuntimeDirectory)
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := runtime.LoadState(context.Background())
	if err != nil || !exists || loaded.SchemaVersion != 1 {
		t.Fatalf("pre-migration state = %+v exists:%t error:%v", loaded, exists, err)
	}
	migrated, err := runtime.migratePrePlatformSharedClusterState(context.Background(), loaded)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.SchemaVersion != 2 || migrated.Applied.PermissionProfile != tobari.SharedClusterProfilePrePlatform ||
		migrated.Applied.GatewayImageID != runner.predecessorGatewayID || migrated.Applied.OPAImageID != runner.predecessorOPAID {
		t.Fatalf("migrated state = %+v", migrated)
	}
	if brokerRuntimeEnabled && migrated.Applied.AuthBrokerImageID != runner.predecessorBrokerID {
		t.Fatalf("migrated Auth Broker identity = %q", migrated.Applied.AuthBrokerImageID)
	}
	if len(runner.composeCalls) != 0 {
		t.Fatalf("schema migration mutated Docker: %v", runner.composeCalls)
	}
}

func TestSchemaOneMigrationRejectsCurrentOrUnhealthyPredecessor(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		current bool
		mutate  func([]byte) []byte
	}{
		{name: "current assets", current: true},
		{name: "stopped Gateway", mutate: func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`"state":"running"`), []byte(`"state":"exited"`), 1)
		}},
		{name: "wrong Gateway role", mutate: func(payload []byte) []byte {
			return bytes.Replace(payload, []byte(`"role":"enforcement"`), []byte(`"role":"observer"`), 1)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			gateway := appliedClusterTestPayload(gatewayContainer, "sha256:"+strings.Repeat("4", 64))
			if test.mutate != nil {
				gateway = test.mutate(gateway)
			}
			runner := &boundedAppliedInspectRunner{payloads: map[string][][]byte{
				gatewayContainer: {gateway},
				opaContainer:     {appliedClusterTestPayload(opaContainer, "sha256:"+strings.Repeat("5", 64))},
			}}
			if brokerRuntimeEnabled {
				runner.payloads[authBrokerContainer] = [][]byte{appliedClusterTestPayload(authBrokerContainer, "sha256:"+strings.Repeat("6", 64))}
			}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			legacy := runtimeState(root)
			legacy.RuntimeDirectory = prePlatformRuntimeDirectory(root)
			legacy.AssetVersion = prePlatformAssetVersion
			if test.current {
				legacy.AssetVersion, err = runtimeassets.Version()
				if err != nil {
					t.Fatal(err)
				}
				if err := runtimeassets.Materialize(legacy.RuntimeDirectory); err != nil {
					t.Fatal(err)
				} else {
					for _, name := range []string{"compose.permission-unix.yaml", "compose.permission-loopback_tcp.yaml"} {
						if err := os.Remove(filepath.Join(legacy.RuntimeDirectory, name)); err != nil {
							t.Fatal(err)
						}
					}
				}
			} else {
				materializePrePlatformRuntime(t, legacy.RuntimeDirectory)
			}
			if err := runtime.writeState(legacy); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.migratePrePlatformSharedClusterState(context.Background(), legacy); err == nil {
				t.Fatal("unsafe predecessor was migrated")
			}
			loaded, exists, err := runtime.LoadState(context.Background())
			if err != nil || !exists || loaded.SchemaVersion != 1 {
				t.Fatalf("failed migration rewrote predecessor: %+v exists:%t error:%v", loaded, exists, err)
			}
		})
	}
}

func TestFailedActivationRestoresExactAppliedServiceIDsAndProfile(t *testing.T) {
	t.Parallel()
	for _, profile := range []tobari.SharedClusterAppliedProfile{
		tobari.SharedClusterProfileUnix,
		tobari.SharedClusterProfileLoopbackTCP,
	} {
		t.Run(string(profile), func(t *testing.T) {
			root := t.TempDir()
			activationErr := errors.New("candidate activation failed")
			runner := &rollbackClusterUpRunner{activationErr: activationErr, predecessorProfile: profile}
			runtime := configuredClusterUpRuntime(t, root, runner)
			candidate, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			retained := retainedAppliedState(t, root, candidate.AggregateRevision, profile)
			runner.bindPredecessor(retained)
			if err := runtimeassets.Materialize(retained.RuntimeDirectory); err != nil {
				t.Fatal(err)
			}
			if err := runtime.writeState(retained); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, activationErr) {
				t.Fatalf("ClusterUp() error = %v", err)
			}
			if len(runner.composeCalls) != 2 || len(runner.composeEnvironments) != 2 {
				t.Fatalf("compose calls = %+v", runner.composeCalls)
			}
			rollbackArgs := runner.composeCalls[1].args
			wantProfile := filepath.Join(retained.RuntimeDirectory, "compose.permission-"+string(profile)+".yaml")
			if !slices.Contains(rollbackArgs, wantProfile) || !slices.Contains(rollbackArgs, "--force-recreate") {
				t.Fatalf("rollback argv = %v", rollbackArgs)
			}
			rollbackEnvironment := strings.Join(runner.composeEnvironments[1], "\n")
			for _, binding := range []string{
				"TOBARI_GATEWAY_IMAGE=" + retained.Applied.GatewayImageID,
				"TOBARI_OPA_IMAGE=" + retained.Applied.OPAImageID,
			} {
				if strings.Count(rollbackEnvironment, binding) != 1 {
					t.Fatalf("rollback environment omits exact %q", binding)
				}
			}
			if brokerRuntimeEnabled && strings.Count(rollbackEnvironment, "TOBARI_AUTH_BROKER_IMAGE="+retained.Applied.AuthBrokerImageID) != 1 {
				t.Fatal("rollback environment omitted exact retained Auth Broker image")
			}
			for _, candidateID := range []string{testGatewayDigest, "sha256:" + strings.Repeat("2", 64), "sha256:" + strings.Repeat("3", 64)} {
				if candidateID != retained.Applied.GatewayImageID && strings.Contains(rollbackEnvironment, candidateID) {
					t.Fatalf("rollback substituted candidate service image %q", candidateID)
				}
			}
		})
	}
}

func TestFailedActivationRestoresMigratedPrePlatformBaseOnly(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	activationErr := errors.New("candidate activation failed")
	runner := &rollbackClusterUpRunner{
		activationErr:        activationErr,
		predecessorGatewayID: "sha256:" + strings.Repeat("4", 64),
		predecessorOPAID:     "sha256:" + strings.Repeat("5", 64),
	}
	if brokerRuntimeEnabled {
		runner.predecessorBrokerID = "sha256:" + strings.Repeat("6", 64)
	}
	runtime := configuredClusterUpRuntime(t, root, runner)
	candidate, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	legacy := candidate
	legacy.RuntimeDirectory = prePlatformRuntimeDirectory(root)
	legacy.AssetVersion = prePlatformAssetVersion
	runner.predecessorRevision = legacy.AggregateRevision
	materializePrePlatformRuntime(t, legacy.RuntimeDirectory)
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, activationErr) {
		t.Fatalf("ClusterUp() error = %v", err)
	}
	if len(runner.composeCalls) != 2 {
		t.Fatalf("compose calls = %+v", runner.composeCalls)
	}
	for _, argument := range runner.composeCalls[1].args {
		if strings.Contains(argument, "compose.permission-") {
			t.Fatalf("pre-platform rollback used successor profile: %v", runner.composeCalls[1].args)
		}
	}
	rollbackEnvironment := strings.Join(runner.composeEnvironments[1], "\n")
	for _, binding := range []string{
		"TOBARI_GATEWAY_IMAGE=" + runner.predecessorGatewayID,
		"TOBARI_OPA_IMAGE=" + runner.predecessorOPAID,
	} {
		if strings.Count(rollbackEnvironment, binding) != 1 {
			t.Fatalf("pre-platform rollback omits %q", binding)
		}
	}
}

func TestRollbackFailureJoinsActivationEvidence(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	activationErr := errors.New("activation failed")
	rollbackErr := errors.New("rollback failed")
	runner := &rollbackClusterUpRunner{
		activationErr: activationErr, rollbackErr: rollbackErr,
		predecessorProfile: tobari.SharedClusterProfileUnix,
	}
	runtime := configuredClusterUpRuntime(t, root, runner)
	candidate, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	retained := candidate
	retained.SchemaVersion = 2
	retained.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: candidate.AggregateRevision, AssetVersion: candidate.AssetVersion,
		ComposeAssets:     testComposeAssets(t, tobari.SharedClusterProfileUnix),
		GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
		OPAImageID:        "sha256:" + strings.Repeat("5", 64),
		PermissionProfile: tobari.SharedClusterProfileUnix,
	}
	if brokerRuntimeEnabled {
		retained.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
	}
	runner.bindPredecessor(retained)
	if err := runtime.writeState(retained); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.ClusterUp(context.Background())
	if !errors.Is(err, activationErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("joined rollback error = %v", err)
	}
}

func TestRollbackVerificationDriftRetainsRecoveryJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	activationErr := errors.New("activation failed")
	runner := &rollbackClusterUpRunner{
		activationErr: activationErr, rollbackVerificationDrift: true,
		predecessorProfile: tobari.SharedClusterProfileUnix,
	}
	runtime := configuredClusterUpRuntime(t, root, runner)
	candidate, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	retained := candidate
	retained.SchemaVersion = 2
	retained.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: candidate.AggregateRevision, AssetVersion: candidate.AssetVersion,
		ComposeAssets:     testComposeAssets(t, tobari.SharedClusterProfileUnix),
		GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
		OPAImageID:        "sha256:" + strings.Repeat("5", 64),
		PermissionProfile: tobari.SharedClusterProfileUnix,
	}
	if brokerRuntimeEnabled {
		retained.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
	}
	runner.bindPredecessor(retained)
	if err := runtime.writeState(retained); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.ClusterUp(context.Background())
	if !errors.Is(err, activationErr) || !strings.Contains(err.Error(), "rollback did not complete") {
		t.Fatalf("rollback verification error = %v", err)
	}
	if runner.rollbackGatewayObservations != 2 {
		t.Fatalf("rollback Gateway fence observations = %d; error = %v", runner.rollbackGatewayObservations, err)
	}
	if _, exists, journalErr := runtime.readClusterJournal(); journalErr != nil || !exists {
		t.Fatalf("rollback verification journal exists=%t error=%v", exists, journalErr)
	}
	loaded, exists, loadErr := runtime.LoadState(context.Background())
	if loadErr != nil || !exists || loaded.Applied != retained.Applied {
		t.Fatalf("rollback verification state = %+v exists=%t error=%v", loaded, exists, loadErr)
	}
}

func TestRollbackJournalClearFailureKeepsExactPreviousAuthority(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	activationErr := errors.New("activation failed")
	clearErr := errors.New("rollback journal clear failed")
	runner := &rollbackClusterUpRunner{activationErr: activationErr}
	runtime := configuredClusterUpRuntime(t, root, runner)
	candidate, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	retained := candidate
	retained.SchemaVersion = 2
	retained.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: candidate.AggregateRevision, AssetVersion: candidate.AssetVersion,
		ComposeAssets:     testComposeAssets(t, tobari.SharedClusterProfileUnix),
		GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
		OPAImageID:        "sha256:" + strings.Repeat("5", 64),
		PermissionProfile: tobari.SharedClusterProfileUnix,
	}
	if brokerRuntimeEnabled {
		retained.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
	}
	runner.bindPredecessor(retained)
	if err := runtime.writeState(retained); err != nil {
		t.Fatal(err)
	}
	runtime.clusterJournalClearHook = func() error { return clearErr }
	_, err = runtime.ClusterUp(context.Background())
	if !errors.Is(err, activationErr) || !errors.Is(err, clearErr) {
		t.Fatalf("rollback journal cleanup error = %v", err)
	}
	loaded, exists, loadErr := runtime.LoadState(context.Background())
	if loadErr != nil || !exists || loaded != retained {
		t.Fatalf("rollback clear failure state = %+v exists=%t error=%v", loaded, exists, loadErr)
	}
	if _, exists, journalErr := runtime.readClusterJournal(); journalErr != nil || !exists {
		t.Fatalf("rollback clear failure journal exists=%t error=%v", exists, journalErr)
	}
}

func TestActivationFailuresRestorePolicyComponentsAndPrincipalTopology(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name         string
		configure    func(*rollbackClusterUpRunner, error)
		wantInjected bool
	}{
		{name: "after policy publish during image preparation", configure: func(runner *rollbackClusterUpRunner, err error) { runner.prepareImagesErr = err }},
		{name: "during Compose activation", wantInjected: true, configure: func(runner *rollbackClusterUpRunner, err error) { runner.activationErr = err }},
		{name: "during network join", wantInjected: true, configure: func(runner *rollbackClusterUpRunner, err error) { runner.networkErr = err }},
		{name: "during health observation", configure: func(runner *rollbackClusterUpRunner, err error) { runner.healthErr = err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			injected := errors.New("synthetic activation failure")
			runner := &rollbackClusterUpRunner{}
			runtime := configuredClusterUpRuntime(t, root, runner)
			candidate, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			retained := candidate
			retained.SchemaVersion = 2
			retained.AggregateRevision = strings.Repeat("e", 64)
			retained.Applied = tobari.SharedClusterAppliedEntry{
				AggregateRevision: retained.AggregateRevision, AssetVersion: retained.AssetVersion,
				ComposeAssets:     testComposeAssets(t, tobari.SharedClusterProfileLoopbackTCP),
				GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
				OPAImageID:        "sha256:" + strings.Repeat("5", 64),
				PermissionProfile: tobari.SharedClusterProfileLoopbackTCP,
			}
			if brokerRuntimeEnabled {
				retained.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
			}
			runner.bindPredecessor(retained)
			binding := principalTestBinding(
				"01912345-6789-7abc-8def-0123456789ab", "172.30.0.3", "172.30.0.2", "tobari-project-net",
			)
			runner.predecessorNetworks = map[string]string{binding.Network: binding.GatewayIP}
			runner.predecessorNetworkProjects = map[string]string{binding.Network: binding.ProjectID}
			if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{binding}); err != nil {
				t.Fatal(err)
			}
			if err := runtime.writeState(retained); err != nil {
				t.Fatal(err)
			}
			test.configure(runner, injected)
			_, activationErr := runtime.ClusterUp(context.Background())
			if activationErr == nil || (test.wantInjected && !errors.Is(activationErr, injected)) {
				t.Fatalf("ClusterUp() error = %v", activationErr)
			}
			loaded, exists, err := runtime.LoadState(context.Background())
			if err != nil || !exists || loaded.Applied != retained.Applied {
				t.Fatalf("last-successful state = %+v exists:%t error:%v", loaded, exists, err)
			}
			principals, err := runtime.readProjectPrincipalRegistry()
			if err != nil || !slices.Equal(principals.Bindings, []projectPrincipalBinding{binding}) {
				t.Fatalf("restored principals = %+v error:%v", principals, err)
			}
			if len(runner.policyPublishes) < 2 ||
				!strings.Contains(runner.policyPublishes[0], candidate.AggregateRevision) ||
				!strings.Contains(runner.policyPublishes[len(runner.policyPublishes)-1], retained.AggregateRevision) {
				t.Fatalf("policy publication sequence = %v", runner.policyPublishes)
			}
			if _, journalExists, err := runtime.readClusterJournal(); err != nil || journalExists {
				t.Fatalf("successful rollback journal = exists:%t error:%v activation:%v", journalExists, err, activationErr)
			}
		})
	}
}

func TestStatePublicationOutcomeControlsRollback(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name          string
		hook          func(*Runtime, error) func(tobari.State, func() error) error
		wantRollback  bool
		wantCommitted bool
	}{
		{
			name: "pre-rename failure retains previous",
			hook: func(_ *Runtime, injected error) func(tobari.State, func() error) error {
				return func(tobari.State, func() error) error { return injected }
			},
			wantRollback: true,
		},
		{
			name: "post-rename failure keeps exact new",
			hook: func(_ *Runtime, injected error) func(tobari.State, func() error) error {
				return func(_ tobari.State, commit func() error) error {
					if err := commit(); err != nil {
						return err
					}
					return injected
				}
			},
			wantCommitted: true,
		},
		{
			name: "drift after publication is unknown",
			hook: func(runtime *Runtime, injected error) func(tobari.State, func() error) error {
				return func(candidate tobari.State, commit func() error) error {
					if err := commit(); err != nil {
						return err
					}
					drifted := candidate
					drifted.RecentError = "synthetic drift"
					if err := writeAtomicJSON(runtime.statePath(), drifted); err != nil {
						return err
					}
					return injected
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			injected := errors.New("state publication failed")
			runner := &rollbackClusterUpRunner{predecessorProfile: tobari.SharedClusterProfileUnix}
			runtime := configuredClusterUpRuntime(t, root, runner)
			candidate, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			retained := retainedAppliedState(t, root, candidate.AggregateRevision, tobari.SharedClusterProfileUnix)
			runner.bindPredecessor(retained)
			if err := runtimeassets.Materialize(retained.RuntimeDirectory); err != nil {
				t.Fatal(err)
			}
			if err := runtime.writeState(retained); err != nil {
				t.Fatal(err)
			}
			runtime.clusterStateWriteHook = test.hook(runtime, injected)
			_, err = runtime.ClusterUp(context.Background())
			if !errors.Is(err, injected) {
				t.Fatalf("ClusterUp() error = %v", err)
			}
			wantCalls := 1
			if test.wantRollback {
				wantCalls = 2
			}
			if len(runner.composeCalls) != wantCalls {
				t.Fatalf("compose calls = %d, want %d", len(runner.composeCalls), wantCalls)
			}
			loaded, exists, loadErr := runtime.LoadState(context.Background())
			if loadErr != nil || !exists {
				t.Fatalf("published state = %+v exists:%t error:%v", loaded, exists, loadErr)
			}
			if test.wantCommitted && (loaded.SchemaVersion != 2 || loaded.Applied.GatewayImageID != testGatewayDigest) {
				t.Fatalf("exact new state was not retained: %+v", loaded)
			}
			if _, journalExists, journalErr := runtime.readClusterJournal(); journalErr != nil || !journalExists {
				t.Fatalf("publication failure journal = exists:%t error:%v", journalExists, journalErr)
			}
		})
	}
}

func TestCommittedStateSurvivesJournalCleanupFailure(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &rollbackClusterUpRunner{}
	runtime := configuredClusterUpRuntime(t, root, runner)
	injected := errors.New("journal cleanup failed")
	runtime.clusterJournalClearHook = func() error { return injected }
	_, err := runtime.ClusterUp(context.Background())
	if !errors.Is(err, injected) || len(runner.composeCalls) != 1 {
		t.Fatalf("ClusterUp() = %v compose calls=%d", err, len(runner.composeCalls))
	}
	loaded, exists, loadErr := runtime.LoadState(context.Background())
	if loadErr != nil || !exists || loaded.SchemaVersion != 2 || loaded.Applied.GatewayImageID != testGatewayDigest {
		t.Fatalf("committed state after journal failure = %+v exists:%t error:%v", loaded, exists, loadErr)
	}
	if _, journalExists, journalErr := runtime.readClusterJournal(); journalErr != nil || !journalExists {
		t.Fatalf("journal cleanup failure = exists:%t error:%v", journalExists, journalErr)
	}
}

func TestFreshStatePublicationRecoveryMatrix(t *testing.T) {
	t.Parallel()
	t.Run("pre-rename failure cleans candidate exactly", func(t *testing.T) {
		root := t.TempDir()
		runner := &rollbackClusterUpRunner{}
		runtime := configuredClusterUpRuntime(t, root, runner)
		injected := errors.New("pre-rename publication failed")
		runtime.clusterStateWriteHook = func(tobari.State, func() error) error { return injected }
		if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
			t.Fatalf("ClusterUp() error = %v", err)
		}
		if runner.cleanupCalls != 1 {
			t.Fatalf("fresh cleanup calls = %d", runner.cleanupCalls)
		}
		if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
			t.Fatalf("fresh failed publication state exists=%t error=%v", exists, err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
			t.Fatalf("fresh successful cleanup journal exists=%t error=%v", exists, err)
		}
	})

	t.Run("post-rename error retains exact new authority and retry", func(t *testing.T) {
		root := t.TempDir()
		runner := &rollbackClusterUpRunner{}
		runtime := configuredClusterUpRuntime(t, root, runner)
		injected := errors.New("post-rename publication failed")
		runtime.clusterStateWriteHook = func(_ tobari.State, commit func() error) error {
			if err := commit(); err != nil {
				return err
			}
			return injected
		}
		if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
			t.Fatalf("ClusterUp() error = %v", err)
		}
		if runner.cleanupCalls != 0 {
			t.Fatalf("committed fresh activation cleanup calls = %d", runner.cleanupCalls)
		}
		loaded, exists, err := runtime.LoadState(context.Background())
		if err != nil || !exists || loaded.SchemaVersion != 2 {
			t.Fatalf("fresh committed state = %+v exists=%t error=%v", loaded, exists, err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
			t.Fatalf("outcome-unknown journal exists=%t error=%v", exists, err)
		}
		runtime.clusterStateWriteHook = nil
		if _, err := runtime.ClusterUp(context.Background()); err != nil {
			t.Fatalf("fresh committed retry = %v", err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
			t.Fatalf("fresh committed retry journal exists=%t error=%v", exists, err)
		}
	})

	t.Run("drift after publication stops mutation", func(t *testing.T) {
		root := t.TempDir()
		runner := &rollbackClusterUpRunner{}
		runtime := configuredClusterUpRuntime(t, root, runner)
		injected := errors.New("publication outcome unknown")
		runtime.clusterStateWriteHook = func(candidate tobari.State, commit func() error) error {
			if err := commit(); err != nil {
				return err
			}
			drifted := candidate
			drifted.RecentError = "synthetic drift"
			if err := writeAtomicJSON(runtime.statePath(), drifted); err != nil {
				return err
			}
			return injected
		}
		if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
			t.Fatalf("ClusterUp() error = %v", err)
		}
		if runner.cleanupCalls != 0 {
			t.Fatalf("unknown fresh authority cleanup calls = %d", runner.cleanupCalls)
		}
		composeCalls := len(runner.composeCalls)
		runtime.clusterStateWriteHook = nil
		if _, err := runtime.ClusterUp(context.Background()); err == nil {
			t.Fatal("fresh drift retry unexpectedly mutated")
		}
		if len(runner.composeCalls) != composeCalls || runner.cleanupCalls != 0 {
			t.Fatalf("fresh drift retry mutated compose=%d cleanup=%d", len(runner.composeCalls), runner.cleanupCalls)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
			t.Fatalf("fresh drift recovery journal exists=%t error=%v", exists, err)
		}
	})

	t.Run("journal clear failure recovers committed state", func(t *testing.T) {
		root := t.TempDir()
		runner := &rollbackClusterUpRunner{}
		runtime := configuredClusterUpRuntime(t, root, runner)
		injected := errors.New("journal clear failed")
		failures := 1
		runtime.clusterJournalClearHook = func() error {
			if failures > 0 {
				failures--
				return injected
			}
			return nil
		}
		if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
			t.Fatalf("ClusterUp() error = %v", err)
		}
		if runner.cleanupCalls != 0 {
			t.Fatalf("journal failure cleanup calls = %d", runner.cleanupCalls)
		}
		if _, err := runtime.ClusterUp(context.Background()); err != nil {
			t.Fatalf("journal recovery retry = %v", err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
			t.Fatalf("journal recovery exists=%t error=%v", exists, err)
		}
	})

	t.Run("failed cleanup is recoverable by explicit down", func(t *testing.T) {
		root := t.TempDir()
		injected := errors.New("fresh cleanup failed")
		runner := &rollbackClusterUpRunner{cleanupErr: injected}
		runtime := configuredClusterUpRuntime(t, root, runner)
		runtime.clusterStateWriteHook = func(tobari.State, func() error) error {
			return errors.New("state publication failed")
		}
		if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
			t.Fatalf("ClusterUp() error = %v", err)
		}
		if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
			t.Fatalf("failed-cleanup state exists=%t error=%v", exists, err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
			t.Fatalf("failed-cleanup journal exists=%t error=%v", exists, err)
		}
		runner.cleanupErr = nil
		recovered, err := runtime.RecoverInterruptedClusterDown(context.Background(), false)
		if err != nil || !recovered || runner.cleanupCalls != 2 {
			t.Fatalf("explicit down recovery recovered=%t cleanup=%d error=%v", recovered, runner.cleanupCalls, err)
		}
		if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
			t.Fatalf("explicit down recovery journal exists=%t error=%v", exists, err)
		}
	})
}

func TestFreshActivationRequiresTwoExactAbsenceFences(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name        string
		presentAt   map[string]int
		ambiguousAt map[string]int
	}{
		{name: "preexisting owned or foreign container", presentAt: map[string]int{"container:" + gatewayContainer: 1}},
		{name: "preexisting named volume", presentAt: map[string]int{"volume:" + policyBundleVolume: 1}},
		{name: "resource appears between fences", presentAt: map[string]int{"network:tobari-control": 2}},
		{name: "ambiguous observation", ambiguousAt: map[string]int{"volume:tobari-public-ca": 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &freshAuthorityRunner{presentAt: test.presentAt, ambiguousAt: test.ambiguousAt}
			runtime := configuredClusterUpRuntime(t, root, runner)
			if _, err := runtime.ClusterUp(context.Background()); err == nil {
				t.Fatal("fresh ClusterUp() accepted ambiguous or preexisting managed resources")
			}
			if runner.composed {
				t.Fatal("fresh activation reached Compose after absence authority failed")
			}
			if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
				t.Fatalf("unattempted fresh activation journal exists=%t error=%v", exists, err)
			}
		})
	}
}

func TestFreshActivationRequiresEmptyPrincipalAuthorityAtBothFences(t *testing.T) {
	t.Parallel()
	binding := principalTestBinding(
		"01912345-6789-7abc-8def-0123456789ab", "172.30.0.3", "172.30.0.2", "tobari-preexisting-network",
	)
	for _, test := range []struct {
		name      string
		configure func(*testing.T, *Runtime, *freshAuthorityRunner)
	}{
		{name: "preexisting principal", configure: func(t *testing.T, runtime *Runtime, _ *freshAuthorityRunner) {
			if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{binding}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "principal appears between fences", configure: func(t *testing.T, runtime *Runtime, runner *freshAuthorityRunner) {
			runner.onSecondFence = func() {
				if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{binding}); err != nil {
					t.Errorf("publish synthetic principal drift: %v", err)
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &freshAuthorityRunner{}
			runtime := configuredClusterUpRuntime(t, root, runner)
			test.configure(t, runtime, runner)
			if _, err := runtime.ClusterUp(context.Background()); err == nil || !strings.Contains(err.Error(), "principal") {
				t.Fatalf("fresh principal authority error = %v", err)
			}
			if runner.composed {
				t.Fatal("fresh principal drift reached Compose mutation")
			}
			observed, err := runtime.readProjectPrincipalRegistry()
			if err != nil || !slices.Equal(observed.Bindings, []projectPrincipalBinding{binding}) {
				t.Fatalf("preexisting principal authority changed: %+v, %v", observed, err)
			}
		})
	}
}

func TestFreshCleanupRevalidatesJournaledCandidateComposeClosure(t *testing.T) {
	t.Parallel()
	mutations := []struct {
		name   string
		mutate func(*testing.T, tobari.State)
	}{
		{name: "missing base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Remove(filepath.Join(state.RuntimeDirectory, "compose.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broad mode", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Chmod(filepath.Join(state.RuntimeDirectory, "compose.yaml"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked base", mutate: func(t *testing.T, state tobari.State) {
			path := filepath.Join(state.RuntimeDirectory, "compose.yaml")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(state.RuntimeDirectory, "versions.env"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing permission profile", mutate: func(t *testing.T, state tobari.State) {
			for _, name := range []string{"compose.permission-unix.yaml", "compose.permission-loopback_tcp.yaml"} {
				if err := os.Remove(filepath.Join(state.RuntimeDirectory, name)); err != nil {
					t.Fatal(err)
				}
			}
		}},
		{name: "tampered research profile", mutate: func(t *testing.T, state tobari.State) {
			if !brokerRuntimeEnabled {
				t.Skip("research Compose profile exists only in the experimental build")
			}
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &rollbackClusterUpRunner{}
			runtime := configuredClusterUpRuntime(t, root, runner)
			injected := errors.New("state publication failed")
			runtime.clusterStateWriteHook = func(candidate tobari.State, _ func() error) error {
				test.mutate(t, candidate)
				return injected
			}
			if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) ||
				!strings.Contains(err.Error(), "rollback did not complete") {
				t.Fatalf("fresh cleanup closure error = %v", err)
			}
			if runner.cleanupCalls != 0 {
				t.Fatalf("tampered candidate closure reached cleanup Compose %d times", runner.cleanupCalls)
			}
			if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
				t.Fatalf("tampered cleanup journal exists=%t error=%v", exists, err)
			}
			if recovered, err := runtime.RecoverInterruptedClusterDown(context.Background(), false); err == nil || recovered {
				t.Fatalf("tampered explicit recovery = (%t, %v)", recovered, err)
			}
			if runner.cleanupCalls != 0 {
				t.Fatalf("tampered retry reached cleanup Compose %d times", runner.cleanupCalls)
			}
		})
	}
}

func TestFreshCleanupVerifiesPostconditionAndRetainsJournalForRetry(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	injected := errors.New("state publication failed")
	runner := &freshAuthorityRunner{remainAfterCleanup: "container:" + gatewayContainer}
	runtime := configuredClusterUpRuntime(t, root, runner)
	runtime.clusterStateWriteHook = func(tobari.State, func() error) error { return injected }
	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) ||
		!strings.Contains(err.Error(), "rollback did not complete") {
		t.Fatalf("fresh cleanup postcondition error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
		t.Fatalf("partial fresh cleanup journal exists=%t error=%v", exists, err)
	}
	runner.remainAfterCleanup = ""
	recovered, err := runtime.RecoverInterruptedClusterDown(context.Background(), false)
	if err != nil || !recovered {
		t.Fatalf("explicit fresh cleanup retry = (%t, %v)", recovered, err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("completed fresh cleanup journal exists=%t error=%v", exists, err)
	}
}

func TestFreshResearchCleanupRequiresCredentialCompanionStop(t *testing.T) {
	if !brokerRuntimeEnabled {
		t.Skip("credential companion exists only in the research build")
	}
	root := t.TempDir()
	injected := errors.New("state publication failed")
	stopErr := errors.New("companion still owns session material")
	runner := &freshAuthorityRunner{}
	runtime := configuredClusterUpRuntime(t, root, runner)
	launcher := &fakeCredentialCompanionLauncher{waitErr: stopErr}
	runtime.companion = launcher
	runtime.clusterStateWriteHook = func(tobari.State, func() error) error { return injected }
	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("fresh cleanup companion error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
		t.Fatalf("live companion cleanup journal exists=%t error=%v", exists, err)
	}
	launcher.waitErr = nil
	recovered, err := runtime.RecoverInterruptedClusterDown(context.Background(), false)
	if err != nil || !recovered || launcher.waits < 2 {
		t.Fatalf("companion cleanup retry = (%t, %v), waits=%d", recovered, err, launcher.waits)
	}
}

func TestCandidateComponentAndProfileExpectationsPrecedeStatePublication(t *testing.T) {
	t.Parallel()
	wrongProfile := tobari.SharedClusterProfileLoopbackTCP
	for _, test := range []struct {
		name      string
		images    map[string]string
		profile   *tobari.SharedClusterAppliedProfile
		networks  map[string]string
		wantCause string
	}{
		{name: "wrong Gateway", images: map[string]string{gatewayContainer: "sha256:" + strings.Repeat("7", 64)}, wantCause: "component images"},
		{name: "wrong OPA", images: map[string]string{opaContainer: "sha256:" + strings.Repeat("7", 64)}, wantCause: "component images"},
		{name: "wrong permission profile", profile: &wrongProfile, wantCause: "permission projection"},
		{name: "missing shared network", networks: map[string]string{"tobari-control": "172.28.0.2"}, wantCause: "network topology"},
		{name: "extra network", networks: map[string]string{
			"tobari-control": "172.28.0.2", "tobari-egress": "172.29.0.2", "foreign": "172.30.0.2",
		}, wantCause: "network topology"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &freshAuthorityRunner{}
			runner.appliedImage = test.images
			runner.appliedProfile = test.profile
			runner.appliedGatewayNets = test.networks
			runtime := configuredClusterUpRuntime(t, root, runner)
			if _, err := runtime.ClusterUp(context.Background()); err == nil || !strings.Contains(err.Error(), test.wantCause) {
				t.Fatalf("candidate mismatch error = %v", err)
			}
			if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
				t.Fatalf("candidate mismatch published state exists=%t error=%v", exists, err)
			}
		})
	}
}

func TestCandidatePrincipalPublicationDriftPreventsStatePublication(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &freshAuthorityRunner{}
	runtime := configuredClusterUpRuntime(t, root, runner)
	var driftErr error
	drifted := principalTestBinding(
		"01912345-6789-7abc-8def-0123456789ab", "172.30.0.3", "172.30.0.2", "tobari-drift-network",
	)
	runner.onAppliedInspect = func() {
		driftErr = runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{drifted})
	}
	if _, err := runtime.ClusterUp(context.Background()); err == nil ||
		!strings.Contains(err.Error(), "principal publication drifted") {
		t.Fatalf("candidate principal drift error = %v, mutation error = %v", err, driftErr)
	}
	if driftErr != nil {
		t.Fatal(driftErr)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
		t.Fatalf("principal drift published state exists=%t error=%v", exists, err)
	}
	observed, err := runtime.readProjectPrincipalRegistry()
	if err != nil || !slices.Equal(observed.Bindings, []projectPrincipalBinding{drifted}) {
		t.Fatalf("principal cleanup erased concurrent authority: %+v, %v", observed, err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
		t.Fatalf("principal drift recovery journal exists=%t error=%v", exists, err)
	}
}

func TestExistingActivationValidatesRollbackComposeClosureBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, tobari.State)
	}{
		{name: "missing", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Remove(filepath.Join(state.RuntimeDirectory, "compose.permission-unix.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered", mutate: func(t *testing.T, state tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broad mode", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Chmod(filepath.Join(state.RuntimeDirectory, "compose.yaml"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlink", mutate: func(t *testing.T, state tobari.State) {
			path := filepath.Join(state.RuntimeDirectory, "compose.yaml")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(state.RuntimeDirectory, "versions.env"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered research build profile", mutate: func(t *testing.T, state tobari.State) {
			if !brokerRuntimeEnabled {
				t.Skip("research build profile exists only in the experimental build")
			}
			if err := os.WriteFile(
				filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"), []byte("tampered\n"), 0o600,
			); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &rollbackClusterUpRunner{predecessorProfile: tobari.SharedClusterProfileUnix}
			runtime := configuredClusterUpRuntime(t, root, runner)
			state, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			state.SchemaVersion = 2
			state.Applied = tobari.SharedClusterAppliedEntry{
				AggregateRevision: state.AggregateRevision, AssetVersion: state.AssetVersion,
				ComposeAssets:  testComposeAssets(t, tobari.SharedClusterProfileUnix),
				GatewayImageID: "sha256:" + strings.Repeat("4", 64),
				OPAImageID:     "sha256:" + strings.Repeat("5", 64), PermissionProfile: tobari.SharedClusterProfileUnix,
			}
			if brokerRuntimeEnabled {
				state.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
			}
			runner.bindPredecessor(state)
			if err := runtime.writeState(state); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, state)
			if _, err := runtime.ClusterUp(context.Background()); err == nil || !strings.Contains(err.Error(), "rollback closure") {
				t.Fatalf("rollback closure error = %v", err)
			}
			if len(runner.policyPublishes) != 0 || len(runner.composeCalls) != 0 {
				t.Fatalf("unsafe rollback closure mutated policy/Compose: policy=%v compose=%v", runner.policyPublishes, runner.composeCalls)
			}
		})
	}
}

func TestCrashRecoveryRevalidatesRollbackComposeClosureBeforeMutation(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, tobari.State)
	}{
		{name: "missing base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Remove(filepath.Join(state.RuntimeDirectory, "compose.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broad permission profile", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Chmod(filepath.Join(state.RuntimeDirectory, "compose.permission-unix.yaml"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked permission profile", mutate: func(t *testing.T, state tobari.State) {
			path := filepath.Join(state.RuntimeDirectory, "compose.permission-unix.yaml")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(state.RuntimeDirectory, "versions.env"), path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered research build profile", mutate: func(t *testing.T, state tobari.State) {
			if !brokerRuntimeEnabled {
				t.Skip("research Compose profile exists only in the experimental build")
			}
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runner := &rollbackClusterUpRunner{}
			runtime := configuredClusterUpRuntime(t, root, runner)
			candidate, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			retained := candidate
			retained.SchemaVersion = 2
			retained.Applied = tobari.SharedClusterAppliedEntry{
				AggregateRevision: retained.AggregateRevision, AssetVersion: retained.AssetVersion,
				ComposeAssets:  testComposeAssets(t, tobari.SharedClusterProfileUnix),
				GatewayImageID: "sha256:" + strings.Repeat("4", 64), OPAImageID: "sha256:" + strings.Repeat("5", 64),
				PermissionProfile: tobari.SharedClusterProfileUnix,
			}
			if brokerRuntimeEnabled {
				retained.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
			}
			if err := runtime.writeState(retained); err != nil {
				t.Fatal(err)
			}
			principals, err := runtime.readProjectPrincipalRegistry()
			if err != nil {
				t.Fatal(err)
			}
			_, receipt, err := runtime.captureCandidateComposeClosure(candidate, tobari.SharedClusterProfileUnix)
			if err != nil {
				t.Fatal(err)
			}
			images := candidateClusterImages{Gateway: testGatewayDigest, OPA: "sha256:" + strings.Repeat("2", 64)}
			if brokerRuntimeEnabled {
				images.AuthBroker = "sha256:" + strings.Repeat("3", 64)
			}
			if err := runtime.startClusterUpReconcile(
				&retained, candidate, tobari.SharedClusterProfileUnix, principals, nil, images, receipt,
			); err != nil {
				t.Fatal(err)
			}
			test.mutate(t, retained)
			if err := runtime.recoverInterruptedClusterUp(context.Background(), retained, true); err == nil ||
				!strings.Contains(err.Error(), "rollback Compose closure") {
				t.Fatalf("crash rollback closure error = %v", err)
			}
			if len(runner.composeCalls) != 0 || len(runner.policyPublishes) != 0 {
				t.Fatalf("crash rollback drift mutated Compose/policy: %v / %v", runner.composeCalls, runner.policyPublishes)
			}
			if _, exists, err := runtime.readClusterJournal(); err != nil || !exists {
				t.Fatalf("crash rollback journal exists=%t error=%v", exists, err)
			}
		})
	}
}

func TestSchemaMigrationPostRenameErrorRetriesAsSchemaTwo(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &rollbackClusterUpRunner{
		predecessorGatewayID: "sha256:" + strings.Repeat("4", 64),
		predecessorOPAID:     "sha256:" + strings.Repeat("5", 64),
	}
	if brokerRuntimeEnabled {
		runner.predecessorBrokerID = "sha256:" + strings.Repeat("6", 64)
	}
	runtime := configuredClusterUpRuntime(t, root, runner)
	legacy := runtimeState(root)
	legacy.RuntimeDirectory = prePlatformRuntimeDirectory(root)
	legacy.AssetVersion = prePlatformAssetVersion
	runner.predecessorRevision = legacy.AggregateRevision
	materializePrePlatformRuntime(t, legacy.RuntimeDirectory)
	if err := runtime.writeState(legacy); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("post-rename fsync failed")
	runtime.clusterStateWriteHook = func(_ tobari.State, commit func() error) error {
		if err := commit(); err != nil {
			return err
		}
		return injected
	}
	if _, err := runtime.ClusterUp(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("migration ClusterUp() = %v", err)
	}
	if len(runner.composeCalls) != 0 {
		t.Fatalf("migration error mutated Docker: %v", runner.composeCalls)
	}
	loaded, exists, err := runtime.LoadState(context.Background())
	if err != nil || !exists || loaded.SchemaVersion != 2 || loaded.Applied.PermissionProfile != tobari.SharedClusterProfilePrePlatform {
		t.Fatalf("committed migration = %+v exists:%t error:%v", loaded, exists, err)
	}
	beforeReadOnly, err := os.ReadFile(runtime.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runtime.LoadState(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _ = runtime.InspectCluster(context.Background(), loaded)
	afterReadOnly, err := os.ReadFile(runtime.statePath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeReadOnly, afterReadOnly) || len(runner.composeCalls) != 0 {
		t.Fatalf("read-only load/status changed migrated authority or Docker")
	}
	runtime.clusterStateWriteHook = nil
	if _, err := runtime.ClusterUp(context.Background()); err != nil {
		t.Fatalf("schema-2 retry = %v", err)
	}
	if len(runner.composeCalls) != 1 {
		t.Fatalf("schema-2 retry compose calls = %d", len(runner.composeCalls))
	}
}

func TestRunClusterUpProgressStepReportsCompletionAndFailure(t *testing.T) {
	t.Parallel()
	var completed []tobari.ClusterUpProgress
	if err := runClusterUpProgressStep(
		func(event tobari.ClusterUpProgress) { completed = append(completed, event) },
		tobari.ClusterUpProgressPolicy,
		func() error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	want := []tobari.ClusterUpProgress{
		{Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressStarted},
		{Step: tobari.ClusterUpProgressPolicy, Status: tobari.ClusterUpProgressCompleted},
	}
	if len(completed) != len(want) || completed[0] != want[0] || completed[1] != want[1] {
		t.Fatalf("completed events = %+v, want %+v", completed, want)
	}
	failed := []tobari.ClusterUpProgress{}
	wantErr := errors.New("synthetic stage failure")
	if err := runClusterUpProgressStep(
		func(event tobari.ClusterUpProgress) { failed = append(failed, event) },
		tobari.ClusterUpProgressPrepareImages,
		func() error { return wantErr },
	); !errors.Is(err, wantErr) {
		t.Fatalf("failed step error = %v, want %v", err, wantErr)
	}
	if len(failed) != 2 || failed[0].Status != tobari.ClusterUpProgressStarted || failed[1].Status != tobari.ClusterUpProgressFailed {
		t.Fatalf("failed events = %+v", failed)
	}
}

func TestWaitForClusterReadyEmitsHealthUpdates(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{outputData: []byte(`{"state":"starting","health":"starting"}`)}
	runtime := &Runtime{runner: runner}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var events []tobari.ClusterUpProgress
	err := runtime.waitForClusterReady(ctx, func(event tobari.ClusterUpProgress) {
		events = append(events, event)
		if event.Status == tobari.ClusterUpProgressUpdated {
			cancel()
		}
	})
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v", err)
	}
	if len(events) != 1 || events[0] != (tobari.ClusterUpProgress{
		Step: tobari.ClusterUpProgressWaitForHealth, Status: tobari.ClusterUpProgressUpdated,
	}) {
		t.Fatalf("health progress events = %+v", events)
	}
}

func (r *ownershipInspectFailureRunner) Run(_ context.Context, args, _ []string, _ io.Reader, _, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	return nil
}

func (r *ownershipInspectFailureRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	return []byte("Docker daemon unavailable"), errors.New("Docker daemon unavailable")
}

func (r *policyProbeRunner) Run(context.Context, []string, []string, io.Reader, io.Writer, io.Writer) error {
	return nil
}

func (r *policyProbeRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "run" {
		return []byte("invalid mount config for type bind"), errors.New("policy bind is not accessible")
	}
	return nil, nil
}

func (r *recordingRunner) Run(_ context.Context, args, _ []string, _ io.Reader, out, _ io.Writer) error {
	r.runs = append(r.runs, runnerCall{args: append([]string{}, args...)})
	if slices.Contains(args, "authbroker.control") {
		_, _ = io.WriteString(out, `{"schema_version":1,"ok":true,"state":"unlocked"}`+"\n")
	}
	return nil
}
func (r *recordingRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if r.onOutput != nil {
		r.onOutput(len(r.outputs))
	}
	if len(r.outputQueue) > 0 {
		output := append([]byte{}, r.outputQueue[0]...)
		r.outputQueue = r.outputQueue[1:]
		return output, r.outputErr
	}
	if r.outputErr == nil && len(r.outputData) == 0 && len(args) >= 3 &&
		args[0] == "exec" && args[1] == opaContainer && args[2] == "/opa" {
		return []byte("true"), nil
	}
	if r.outputErr == nil && len(r.outputData) == 0 && len(args) > 0 &&
		(args[0] == "inspect" || (args[0] == "volume" && len(args) > 1 && args[1] == "inspect")) {
		return []byte(ownerValue + "\n"), nil
	}
	return append([]byte{}, r.outputData...), r.outputErr
}

type gatewayNetworkRunner struct {
	outputs  []runnerCall
	networks string
}

type hostileNetworkObservationRunner struct {
	stdout []byte
	stderr []byte
	err    error
	calls  int
}

func (r *hostileNetworkObservationRunner) Run(
	_ context.Context, _ []string, _ []string, _ io.Reader, output, errorOutput io.Writer,
) error {
	r.calls++
	_, _ = output.Write(r.stdout)
	_, _ = errorOutput.Write(r.stderr)
	return r.err
}

func (*hostileNetworkObservationRunner) Output(context.Context, []string, []string) ([]byte, error) {
	return nil, errors.New("unbounded network observation must not be used")
}

func (r *gatewayNetworkRunner) Run(
	_ context.Context, args, _ []string, _ io.Reader, output, _ io.Writer,
) error {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "inspect" {
		_, _ = io.WriteString(output, r.networks)
	}
	return nil
}

func (r *gatewayNetworkRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	r.outputs = append(r.outputs, runnerCall{args: append([]string{}, args...)})
	if len(args) > 0 && args[0] == "inspect" {
		return []byte(r.networks), nil
	}
	return []byte{}, nil
}

func runtimeState(root string) tobari.State {
	return tobari.State{
		SchemaVersion: 1, RuntimeDirectory: filepath.Join(root, "runtime"),
		AggregateRevision: strings.Repeat("a", 64), ManifestCount: 1,
		PolicyDirectory: filepath.Join(root, "policy"),
		GatewayConfig:   filepath.Join(root, "gateway.json"), AssetVersion: "asset",
	}
}

func testComposeAssets(t *testing.T, profile tobari.SharedClusterAppliedProfile) tobari.SharedClusterComposeAssets {
	t.Helper()
	if profile == tobari.SharedClusterProfilePrePlatform {
		return prePlatformComposeAssets()
	}
	digest := func(name string) string {
		data, err := runtimeassets.Read(name)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", sha256.Sum256(data))
	}
	assets := tobari.SharedClusterComposeAssets{BaseSHA256: digest("compose.yaml")}
	if brokerRuntimeEnabled {
		assets.BuildSHA256 = digest("compose.experimental.yaml")
	}
	transport, ok := profile.PermissionTransport()
	if !ok {
		t.Fatalf("profile %q has no permission transport", profile)
	}
	assets.PermissionSHA256 = digest("compose.permission-" + string(transport) + ".yaml")
	return assets
}

func testCandidateComposeReceipt(
	t *testing.T, state tobari.State, profile tobari.SharedClusterAppliedProfile,
) candidateComposeClosureReceipt {
	t.Helper()
	assets := testComposeAssets(t, profile)
	receipt := candidateComposeClosureReceipt{
		RuntimeDirectory: state.RuntimeDirectory, AssetVersion: state.AssetVersion, Profile: profile,
		Base: composeAssetReceipt{Path: filepath.Join(state.RuntimeDirectory, "compose.yaml"), OwnerUID: os.Getuid(), Mode: 0o600, SHA256: assets.BaseSHA256},
	}
	if brokerRuntimeEnabled {
		build := composeAssetReceipt{Path: filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"), OwnerUID: os.Getuid(), Mode: 0o600, SHA256: assets.BuildSHA256}
		receipt.Build = &build
	}
	transport, _ := profile.PermissionTransport()
	receipt.Permission = composeAssetReceipt{
		Path:     filepath.Join(state.RuntimeDirectory, "compose.permission-"+string(transport)+".yaml"),
		OwnerUID: os.Getuid(), Mode: 0o600, SHA256: assets.PermissionSHA256,
	}
	return receipt
}

func appliedRuntimeState(t *testing.T, root string, profile tobari.SharedClusterAppliedProfile) tobari.State {
	t.Helper()
	state := runtimeState(root)
	state.SchemaVersion = 2
	state.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: state.AggregateRevision,
		AssetVersion:      state.AssetVersion,
		ComposeAssets:     testComposeAssets(t, profile),
		GatewayImageID:    testGatewayDigest,
		OPAImageID:        "sha256:" + strings.Repeat("2", 64),
		PermissionProfile: profile,
	}
	if brokerRuntimeEnabled {
		state.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("3", 64)
	}
	return state
}

func configuredClusterUpRuntime(t *testing.T, root string, runner commandRunner) *Runtime {
	t.Helper()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	runtime.images = testImageResolver{
		runtimeImage: "tobari-runtime:dev",
		gateway:      sharedImageSelection{Image: "tobari-gateway:dev"},
		authBroker:   sharedImageSelection{Image: "tobari-auth-broker:dev"},
	}
	runtime.rootKeyLoader = func(context.Context) ([]byte, error) {
		return bytes.Repeat([]byte{0x41}, 32), nil
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	runtime.companionEntropy = bytes.NewReader(bytes.Repeat([]byte{0x42}, 256))
	return runtime
}

func materializePrePlatformRuntime(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", "compose.pre-platform.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "compose.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if brokerRuntimeEnabled {
		experimental, err := os.ReadFile(filepath.Join("testdata", "compose.pre-platform.experimental.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "compose.experimental.yaml"), experimental, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func prePlatformStateFixture(t *testing.T, root string, schema int) tobari.State {
	t.Helper()
	state := runtimeState(root)
	state.RuntimeDirectory = prePlatformRuntimeDirectory(root)
	state.AssetVersion = prePlatformAssetVersion
	materializePrePlatformRuntime(t, state.RuntimeDirectory)
	if schema == 2 {
		state.SchemaVersion = 2
		state.Applied = tobari.SharedClusterAppliedEntry{
			AggregateRevision: state.AggregateRevision, AssetVersion: state.AssetVersion,
			ComposeAssets:     prePlatformComposeAssets(),
			GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
			OPAImageID:        "sha256:" + strings.Repeat("5", 64),
			PermissionProfile: tobari.SharedClusterProfilePrePlatform,
		}
		if brokerRuntimeEnabled {
			state.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
		}
	}
	return state
}

func prePlatformRuntimeDirectory(root string) string {
	return filepath.Join(root, "state", "runtime", prePlatformAssetVersion)
}

func retainedAppliedState(t *testing.T, root, aggregate string, profile tobari.SharedClusterAppliedProfile) tobari.State {
	t.Helper()
	state := runtimeState(root)
	state.SchemaVersion = 2
	state.AggregateRevision = aggregate
	state.AssetVersion = "retained-asset"
	state.RuntimeDirectory = filepath.Join(root, "state", "runtime", state.AssetVersion)
	state.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: aggregate,
		AssetVersion:      "retained-asset",
		ComposeAssets:     testComposeAssets(t, profile),
		GatewayImageID:    "sha256:" + strings.Repeat("4", 64),
		OPAImageID:        "sha256:" + strings.Repeat("5", 64),
		PermissionProfile: profile,
	}
	if brokerRuntimeEnabled {
		state.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
	}
	return state
}

func TestResolveRuntimeHomesUsesXDGAndPortableFallbacks(t *testing.T) {
	t.Parallel()
	config := filepath.Join(string(filepath.Separator), "xdg", "config")
	state := filepath.Join(string(filepath.Separator), "xdg", "state")
	gotConfig, gotState, err := resolveRuntimeHomes(config, state, func() (string, error) {
		return "", errors.New("must not resolve home")
	})
	if err != nil || gotConfig != config || gotState != state {
		t.Fatalf("resolved (%q,%q,%v)", gotConfig, gotState, err)
	}
	home := filepath.Join(string(filepath.Separator), "home", "example")
	gotConfig, gotState, err = resolveRuntimeHomes("", "", func() (string, error) { return home, nil })
	if err != nil || gotConfig != filepath.Join(home, ".config") || gotState != filepath.Join(home, ".local", "state") {
		t.Fatalf("fallback (%q,%q,%v)", gotConfig, gotState, err)
	}
}

func TestResolveProjectRootRejectsProtectedManagementPaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntimeWithData(
		filepath.Join(root, "config"), filepath.Join(root, "state"), filepath.Join(root, "data"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.configDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.stateDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtime.dataDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	projectRoot, err = runtime.ResolveProjectRoot(context.Background(), projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	protected := map[string]string{
		"filesystem root": string(filepath.Separator),
		"user home":       home,
		"config":          runtime.configDirectory,
		"config child":    filepath.Join(runtime.configDirectory, "policy"),
		"config ancestor": filepath.Dir(runtime.configDirectory),
		"state":           runtime.stateDirectory,
		"data":            runtime.dataDirectory,
		"docker runtime":  filepath.Join(string(filepath.Separator), "var", "run"),
	}
	for name, candidate := range protected {
		name, candidate := name, candidate
		t.Run(name, func(t *testing.T) {
			if candidate != string(filepath.Separator) {
				if err := os.MkdirAll(candidate, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := runtime.ResolveProjectRoot(context.Background(), candidate); err == nil {
				t.Fatalf("ResolveProjectRoot(%q) unexpectedly succeeded", candidate)
			}
		})
	}
	if _, err := runtime.ResolveProjectRoot(context.Background(), projectRoot); err != nil {
		t.Fatalf("ordinary project root rejected: %v", err)
	}
}

func TestDoctorRejectsProtectedProspectiveRootsAfterSymlinkResolution(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	runner := &recordingRunner{}
	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), runner,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootAlias := filepath.Join(root, "root-alias")
	if err := os.Symlink(string(filepath.Separator), rootAlias); err != nil {
		t.Fatal(err)
	}
	for name, candidate := range map[string]string{
		"filesystem root":            string(filepath.Separator),
		"symlink to filesystem root": rootAlias,
	} {
		t.Run(name, func(t *testing.T) {
			report, err := runRuntimeDoctor(context.Background(), runtime, candidate)
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range report.Checks {
				if check.Name != "root" {
					continue
				}
				if check.Status != doctor.CheckStatusFail {
					t.Fatalf("root check = %+v, want fail", check)
				}
				if !strings.Contains(check.Detail, "cannot be a Tobari project root") {
					t.Fatalf("root failure detail = %q", check.Detail)
				}
				return
			}
			t.Fatal("doctor report did not contain a root check")
		})
	}
	if len(runner.runs) != 0 {
		t.Fatalf("doctor performed Docker mutations: %v", runner.runs)
	}
}

func TestDoctorDiagnosesUnsafeLearnedPolicyData(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.Mkdir(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dockerPath := filepath.Join(binDir, "docker")
	if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	runtime, err := newRuntime(
		filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{},
	)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	writeMinimalPolicyFixture(t, state)
	allowPath := filepath.Join(state.PolicyDirectory, policyDomainsName, "api.github.com", policyAllowFileName)
	if err := os.WriteFile(allowPath, []byte(`{"host":"api.github.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}

	report, err := runRuntimeDoctor(context.Background(), runtime, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range report.Checks {
		if check.Name != "policy_data" {
			continue
		}
		if check.Status != doctor.CheckStatusFail {
			t.Fatalf("policy_data check = %+v, want fail", check)
		}
		if !strings.Contains(check.Detail, "learned policy data is invalid or unsafe") ||
			!strings.Contains(check.Detail, "schema_version") {
			t.Fatalf("policy_data detail = %q", check.Detail)
		}
		return
	}
	t.Fatal("doctor report did not contain a policy_data check")
}

func TestLoadStateRejectsIncompleteAndTrailingDocuments(t *testing.T) {
	t.Parallel()
	for name, data := range map[string][]byte{
		"incomplete": []byte(`{"schema_version":1}`),
		"trailing":   append(mustJSON(t, runtimeState(t.TempDir())), []byte("\n{}\n")...),
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
			if err := os.MkdirAll(filepath.Dir(runtime.statePath()), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(runtime.statePath(), data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runtime.LoadState(context.Background()); err == nil {
				t.Fatal("invalid state was accepted")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestPolicyValidationUsesReadOnlyMountAndPolicyOwner(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err := runtime.testPolicy(context.Background(), runtimeState(root)); err != nil {
		t.Fatal(err)
	}
	args := runner.outputs[0].args
	uid, gid := currentIDs()
	wantUser := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	if !slices.Contains(args, wantUser) || !slices.Contains(args, "type=bind,src="+filepath.Join(root, "policy")+",dst=/policy,readonly") {
		t.Fatalf("policy argv = %v", args)
	}
}

func TestClusterDownStopsBeforeCleanupWhenOwnershipInspectionFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &ownershipInspectFailureRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), runtimeState(root), false); err == nil {
		t.Fatal("ClusterDown() ignored an ownership inspection failure")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("cleanup ran after ownership inspection failure: %v", runner.runs)
	}
}

func TestClusterDownPurgesMissingVolumesIdempotently(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputErr: errors.New("No such object")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	state := prePlatformStateFixture(t, root, 1)
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), state, true); err != nil {
		t.Fatalf("ClusterDown() = %v, want idempotent success for missing resources", err)
	}
	for _, call := range runner.outputs {
		if len(call.args) > 0 && call.args[0] == "volume" && slices.Contains(call.args, "rm") {
			t.Fatalf("missing volume was sent to rm: %v", call.args)
		}
	}
}

func TestClusterDownDoesNotRequireCurrentPermissionProfileAssets(t *testing.T) {
	t.Parallel()
	for _, schema := range []int{1, 2} {
		t.Run(fmt.Sprintf("schema-%d", schema), func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{outputData: []byte(ownerValue + "\n")}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			runtime.companion = &fakeCredentialCompanionLauncher{}
			// A retained pre-platform runtime has only the original base and,
			// in research builds, experimental Compose assets. The current host
			// permission profile must not become a cleanup dependency.
			runtime.permissionIngestionTransport = tobari.PermissionSessionTransportTCP
			state := prePlatformStateFixture(t, root, schema)
			if err := runtime.writeState(state); err != nil {
				t.Fatal(err)
			}
			if err := runtime.ClusterDown(context.Background(), state, false); err != nil {
				t.Fatal(err)
			}
			var down []string
			for _, call := range runner.runs {
				if len(call.args) > 0 && call.args[0] == "compose" && slices.Contains(call.args, "down") {
					down = call.args
				}
			}
			if len(down) == 0 || !slices.Contains(down, "--remove-orphans") {
				t.Fatalf("cluster down argv = %v", down)
			}
			for _, argument := range down {
				if strings.Contains(argument, "compose.permission-") {
					t.Fatalf("cluster down required a successor permission profile: %v", down)
				}
			}
			containsExperimental := slices.Contains(down, filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"))
			if containsExperimental != brokerRuntimeEnabled {
				t.Fatalf("cluster down research overlay presence = %t", containsExperimental)
			}
		})
	}
}

func TestClusterDownRevalidatesRetainedComposeAuthorityBeforeMutation(t *testing.T) {
	t.Parallel()
	stateFactories := []struct {
		name  string
		build func(*testing.T, *Runtime, string) tobari.State
	}{
		{name: "schema1-pre-platform", build: func(t *testing.T, _ *Runtime, root string) tobari.State {
			return prePlatformStateFixture(t, root, 1)
		}},
		{name: "schema2-pre-platform", build: func(t *testing.T, _ *Runtime, root string) tobari.State {
			return prePlatformStateFixture(t, root, 2)
		}},
		{name: "schema2-current", build: func(t *testing.T, runtime *Runtime, _ string) tobari.State {
			state, err := runtime.prepareState(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			state.SchemaVersion = 2
			state.Applied = tobari.SharedClusterAppliedEntry{
				AggregateRevision: state.AggregateRevision, AssetVersion: state.AssetVersion,
				ComposeAssets:  testComposeAssets(t, tobari.SharedClusterProfileUnix),
				GatewayImageID: "sha256:" + strings.Repeat("4", 64), OPAImageID: "sha256:" + strings.Repeat("5", 64),
				PermissionProfile: tobari.SharedClusterProfileUnix,
			}
			if brokerRuntimeEnabled {
				state.Applied.AuthBrokerImageID = "sha256:" + strings.Repeat("6", 64)
			}
			return state
		}},
	}
	mutations := []struct {
		name    string
		current bool
		build   bool
		mutate  func(*testing.T, tobari.State)
	}{
		{name: "missing base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Remove(filepath.Join(state.RuntimeDirectory, "compose.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered base", mutate: func(t *testing.T, state tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "broad base mode", mutate: func(t *testing.T, state tobari.State) {
			if err := os.Chmod(filepath.Join(state.RuntimeDirectory, "compose.yaml"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "symlinked base", mutate: func(t *testing.T, state tobari.State) {
			path := filepath.Join(state.RuntimeDirectory, "compose.yaml")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(state.RuntimeDirectory, "down-target")
			if err := os.WriteFile(target, []byte("target\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "tampered research overlay", build: true, mutate: func(t *testing.T, state tobari.State) {
			if err := os.WriteFile(filepath.Join(state.RuntimeDirectory, "compose.experimental.yaml"), []byte("tampered\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing successor permission receipt", current: true, mutate: func(t *testing.T, state tobari.State) {
			if err := os.Remove(filepath.Join(state.RuntimeDirectory, "compose.permission-unix.yaml")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, stateTest := range stateFactories {
		for _, mutation := range mutations {
			if mutation.current && stateTest.name != "schema2-current" {
				continue
			}
			if mutation.build && !brokerRuntimeEnabled {
				continue
			}
			for _, recovery := range []bool{false, true} {
				name := stateTest.name + "/" + mutation.name + "/explicit"
				if recovery {
					name = stateTest.name + "/" + mutation.name + "/journal-retry"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					runner := &recordingRunner{}
					runtime := configuredClusterUpRuntime(t, root, runner)
					runtime.companion = &fakeCredentialCompanionLauncher{}
					state := stateTest.build(t, runtime, root)
					if err := runtime.writeState(state); err != nil {
						t.Fatal(err)
					}
					if recovery {
						if err := runtime.startClusterReconcile(clusterOperationDown); err != nil {
							t.Fatal(err)
						}
					}
					mutation.mutate(t, state)
					if err := runtime.ClusterDown(context.Background(), state, false); err == nil ||
						!strings.Contains(err.Error(), "Compose authority") {
						t.Fatalf("unsafe down authority error = %v", err)
					}
					for _, call := range runner.runs {
						if len(call.args) > 0 && call.args[0] == "compose" && slices.Contains(call.args, "down") {
							t.Fatalf("unsafe down authority reached Compose: %v", call.args)
						}
					}
					_, journalExists, err := runtime.readClusterJournal()
					if err != nil || journalExists != recovery {
						t.Fatalf("down journal exists=%t want=%t error=%v", journalExists, recovery, err)
					}
				})
			}
		}
	}
}

func TestClusterDownResumesAfterInterruptedComposeCleanup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &interruptedClusterDownRunner{}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := prePlatformStateFixture(t, root, 1)
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), state, false); err == nil {
		t.Fatal("first ClusterDown() unexpectedly completed")
	}
	journal, exists, err := runtime.readClusterJournal()
	if err != nil || !exists || journal.Operation != clusterOperationDown || journal.Phase != clusterPhaseStarted {
		t.Fatalf("interrupted journal = (%+v, %t, %v)", journal, exists, err)
	}
	state, exists, err = runtime.LoadState(context.Background())
	if err != nil || !exists {
		t.Fatalf("retry state exists=%t error=%v", exists, err)
	}
	if err := runtime.ClusterDown(context.Background(), state, false); err != nil {
		t.Fatalf("resumed ClusterDown() error = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("cluster journal after resumed cleanup = exists:%t error:%v", exists, err)
	}
	if _, exists, err := runtime.LoadState(context.Background()); err != nil || exists {
		t.Fatalf("cluster state after resumed cleanup = exists:%t error:%v", exists, err)
	}
}

func TestPrepareStateUsesAggregateProjection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	state, err := runtime.prepareState(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 1 || state.ManifestCount != 1 || state.AggregateRevision == "" {
		t.Fatalf("state = %+v", state)
	}
	for path, want := range map[string]os.FileMode{
		state.PolicyDirectory: 0o700, filepath.Join(state.PolicyDirectory, "router.rego"): 0o600,
		state.GatewayConfig:                                0o600,
		runtime.interactiveAttachmentDirectory():           0o700,
		runtime.interactiveAttachmentSessionRegistryPath(): 0o600,
		runtime.interactiveAttachmentSocketDirectory():     0o700,
	} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode=%v err=%v want=%o", path, info.Mode().Perm(), err, want)
		}
	}
}

func TestPrepareStateRejectsSymlinkedManagementDirectoriesBeforeDocker(t *testing.T) {
	t.Parallel()
	for name, target := range map[string]string{
		"configuration": "config",
		"state":         "state",
		"data":          "data",
	} {
		name, target := name, target
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, "config")
			state := filepath.Join(root, "state")
			data := filepath.Join(root, "data")
			for _, path := range []string{config, state, data} {
				if path == filepath.Join(root, target) {
					continue
				}
				if err := os.MkdirAll(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			outside := filepath.Join(root, "outside-"+target)
			if err := os.Mkdir(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(root, target)); err != nil {
				t.Fatal(err)
			}
			runner := &recordingRunner{}
			runtime, err := newRuntimeWithData(config, state, data, runner)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.prepareState(context.Background()); err == nil {
				t.Fatal("prepareState() accepted a symlinked management directory")
			}
			if len(runner.outputs) != 0 || len(runner.runs) != 0 {
				t.Fatalf("Docker calls after unsafe directory = outputs %v runs %v", runner.outputs, runner.runs)
			}
		})
	}
}

func TestEnsurePrivateDirectoryTightensExistingDirectory(t *testing.T) {
	t.Parallel()
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "config"), filepath.Join(t.TempDir(), "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "existing")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ensurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

func TestSharedStateWriterIsAtomicAndSerialized(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	const writers = 16
	errs := make(chan error, writers)
	for index := 0; index < writers; index++ {
		index := index
		go func() {
			copy := state
			copy.RecentError = fmt.Sprintf("writer-%d", index)
			errs <- runtime.writeState(copy)
		}()
	}
	for range writers {
		if err := <-errs; err != nil {
			t.Fatalf("writeState() error = %v", err)
		}
	}
	loaded, exists, err := runtime.LoadState(context.Background())
	if err != nil || !exists || loaded.SchemaVersion != state.SchemaVersion {
		t.Fatalf("LoadState() = (%+v, %t, %v)", loaded, exists, err)
	}
	if _, err := os.Stat(filepath.Join(runtime.stateDirectory, "cluster.lock")); err != nil {
		t.Fatalf("cluster lock was not durable: %v", err)
	}
	entries, err := os.ReadDir(runtime.stateDirectory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("temporary state file remains: %s", entry.Name())
		}
	}
}

func TestInterruptedClusterReconcileFailsClosedInStatus(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	state := runtimeState(root)
	fresh := expectedFreshClusterResourceAuthority()
	images := candidateClusterImages{
		Gateway: testGatewayDigest, OPA: "sha256:" + strings.Repeat("2", 64),
	}
	if brokerRuntimeEnabled {
		images.AuthBroker = "sha256:" + strings.Repeat("3", 64)
	}
	if err := runtime.startClusterUpReconcile(nil, state, tobari.SharedClusterProfileUnix, projectPrincipalRegistry{
		SchemaVersion: projectPrincipalRegistrySchema, Bindings: []projectPrincipalBinding{},
	}, &fresh, images, testCandidateComposeReceipt(t, state, tobari.SharedClusterProfileUnix)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.InspectCluster(context.Background(), state); err == nil {
		t.Fatal("InspectCluster() succeeded with an interrupted reconcile journal")
	}
	if err := runtime.clearClusterJournal(); err != nil {
		t.Fatal(err)
	}
}

func TestClusterDownRecoversExactSchemaOneDownJournal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: []byte(ownerValue + "\n")}
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if err != nil {
		t.Fatal(err)
	}
	runtime.companion = &fakeCredentialCompanionLauncher{}
	state := prePlatformStateFixture(t, root, 1)
	if err := runtime.writeState(state); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(runtime.clusterJournalPath(), map[string]any{
		"schema_version": 1, "operation": clusterOperationDown, "phase": clusterPhaseStarted,
	}); err != nil {
		t.Fatal(err)
	}
	if err := runtime.ClusterDown(context.Background(), state, false); err != nil {
		t.Fatalf("ClusterDown() from predecessor journal = %v", err)
	}
	if _, exists, err := runtime.readClusterJournal(); err != nil || exists {
		t.Fatalf("predecessor down journal exists=%t error=%v", exists, err)
	}
}

func TestInterruptedClusterReconcilePublishesExplicitRecoveryActions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.startClusterReconcile(clusterOperationDown); err != nil {
		t.Fatal(err)
	}
	_, err = runtime.InspectCluster(context.Background(), runtimeState(root))
	structured, ok := fault.PublicCopy(err)
	if !ok || structured.Code != "cluster_reconcile_interrupted" || len(structured.NextActions) != 2 ||
		structured.NextActions[0].Command != "cluster up" || structured.NextActions[1].Command != "cluster down" {
		t.Fatalf("InspectCluster() fault = %+v, %v; want explicit cluster recovery actions", structured, err)
	}
	if err := runtime.clearClusterJournal(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveImageSelectorUsesInjectedResolverForBuiltin(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:dev"}
	got, err := runtime.ResolveImageSelector(context.Background(), tobari.BuiltinImageSelector)
	if err != nil || got != "tobari-runtime:dev" {
		t.Fatalf("builtin selector resolved %q, %v", got, err)
	}
	got, err = runtime.ResolveImageSelector(context.Background(), "")
	if err != nil || got != "tobari-runtime:dev" {
		t.Fatalf("missing config resolved %q, %v", got, err)
	}
}

func TestComposeEnvironmentUsesPinnedImages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	environment, err := runtime.composeEnvironment(appliedRuntimeState(t, root, tobari.SharedClusterProfileUnix))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(environment, "\n")
	for _, key := range []string{"TOBARI_MITMPROXY_IMAGE=", "TOBARI_GATEWAY_IMAGE=", "TOBARI_OPA_IMAGE=", "TOBARI_DEBIAN_IMAGE="} {
		index := strings.LastIndex(joined, key)
		if index < 0 || !strings.Contains(joined[index:], "@sha256:") {
			t.Fatalf("%s is not digest pinned", key)
		}
	}
	if !strings.Contains(joined, "TOBARI_PRINCIPAL_DIR="+runtime.principalRegistryDirectory()) {
		t.Fatalf("compose environment does not expose the dedicated principal directory: %s", joined)
	}
	for _, binding := range []string{
		"TOBARI_INTERACTIVE_ATTACHMENT_DIR=" + runtime.interactiveAttachmentDirectory(),
		"TOBARI_PERMISSION_INGESTION_DIR=" + runtime.interactiveAttachmentSocketDirectory(),
	} {
		if !strings.Contains(joined, binding) {
			t.Fatalf("compose environment omits permission attachment binding %q: %s", binding, joined)
		}
	}
	authBindings := []string{"TOBARI_AUTH_PROVIDER_DIR=", "TOBARI_AUTH_CONTEXTS_DIR=", "TOBARI_AUTH_RUNTIME_DIR=", "TOBARI_AUTH_BROKER_IMAGE="}
	for _, binding := range authBindings {
		present := strings.Contains(joined, binding)
		if brokerRuntimeEnabled && !present {
			t.Fatalf("experimental compose environment omits %q", binding)
		}
		if !brokerRuntimeEnabled && present {
			t.Fatalf("standard compose environment exposes %q", binding)
		}
	}
	if strings.Contains(joined, "TOBARI_PRINCIPAL_CONFIG=") {
		t.Fatal("compose environment still exposes the single-file principal configuration")
	}
	if strings.Contains(joined, "TOBARI_AUTH_PROVIDER_CONFIG=") {
		t.Fatal("compose environment still exposes the single-file auth provider projection")
	}
}

func TestPermissionIngestionComposeProfileIsClosed(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), &recordingRunner{})
	for _, test := range []struct {
		transport tobari.PermissionSessionTransport
		file      string
		hasSource bool
	}{
		{transport: tobari.PermissionSessionTransportUnix, file: "compose.permission-unix.yaml", hasSource: true},
		{transport: tobari.PermissionSessionTransportTCP, file: "compose.permission-loopback_tcp.yaml", hasSource: false},
	} {
		runtime.permissionIngestionTransport = test.transport
		args, err := runtime.permissionSessionComposeFileArgs("/runtime")
		if err != nil || len(args) != 2 || args[0] != "-f" || args[1] != filepath.Join("/runtime", test.file) {
			t.Fatalf("%s compose profile = %v, %v", test.transport, args, err)
		}
		profile, err := tobari.SharedClusterProfileForTransport(test.transport)
		if err != nil {
			t.Fatal(err)
		}
		environment, err := runtime.composeEnvironment(appliedRuntimeState(t, root, profile))
		if err != nil {
			t.Fatal(err)
		}
		hasSource := strings.Contains(strings.Join(environment, "\n"), "TOBARI_PERMISSION_INGESTION_DIR=")
		if hasSource != test.hasSource {
			t.Fatalf("%s Unix source presence = %t", test.transport, hasSource)
		}
	}
	runtime.permissionIngestionTransport = "tcp"
	if _, err := runtime.permissionSessionComposeFileArgs("/runtime"); err == nil {
		t.Fatal("unsupported permission ingestion profile was accepted")
	}
}

func TestPrepareActiveContextImageReusesAndValidatesLocalOfficialRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: compatibleImageInspection()}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if runtime.defaultRuntimeImage() != localBaseRuntimeImage {
		t.Skip("local official base build is not selected by the development resolver")
	}
	initializeTestWorkspaceManifest(t, runtime)
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("runtime image mutation calls = %v", runner.runs)
	}
	if len(runner.outputs) != 2 || runner.outputs[0].args[0] != "image" || runner.outputs[0].args[1] != "inspect" {
		t.Fatalf("runtime image inspect calls = %v", runner.outputs)
	}
}

func TestPrepareActiveContextImageBuildsMissingLocalOfficialRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &localBaseBuildRunner{}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	if runtime.defaultRuntimeImage() != localBaseRuntimeImage {
		t.Skip("local official base build is not selected by the development resolver")
	}
	if err := runtime.ensureContextStore(); err != nil {
		t.Fatal(err)
	}
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 1 || !slices.Equal(runner.runs[0].args[:4], []string{"buildx", "build", "--progress=plain", "--load"}) {
		t.Fatalf("runtime image build calls = %v", runner.runs)
	}
	if !containsArgs(runner.runs[0].args, "--tag") || !containsArgs(runner.runs[0].args, localBaseRuntimeImage) ||
		!containsArgs(runner.runs[0].args, "--build-arg") || !strings.Contains(strings.Join(runner.runs[0].args, "\n"), "TOBARI_UID=") {
		t.Fatalf("runtime image build argv = %v", runner.runs[0].args)
	}
	if len(runner.outputs) != 2 {
		t.Fatalf("runtime image inspection calls = %v", runner.outputs)
	}
}

func TestPrepareActiveContextImageDoesNotPullInjectedLocalRuntime(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	runner := &recordingRunner{outputData: compatibleImageInspection()}
	runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
	runtime.images = testImageResolver{runtimeImage: "tobari-runtime:dev"}
	initializeTestWorkspaceManifest(t, runtime)
	if err := runtime.prepareActiveContextImage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runner.runs) != 0 {
		t.Fatalf("local runtime image was pulled: %v", runner.runs)
	}
	if len(runner.outputs) != 1 || runner.outputs[0].args[len(runner.outputs[0].args)-1] != "tobari-runtime:dev" {
		t.Fatalf("runtime image inspect calls = %v", runner.outputs)
	}
}

func TestValidateCompatibleImageRequiresRuntimeContract(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		configuration string
		wantErr       bool
	}{
		"compatible": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
		},
		"unlabeled": {
			configuration: `{"api":"","lifetime":"sleep infinity","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
		"missing lifetime command": {
			configuration: `{"api":"1","user":"tobari","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
		"overridden entrypoint": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"tobari","entrypoint":["/bin/sh"]}`,
			wantErr:       true,
		},
		"overridden user": {
			configuration: `{"api":"1","lifetime":"sleep infinity","user":"root","entrypoint":["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]}`,
			wantErr:       true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &recordingRunner{outputData: []byte(test.configuration)}
			runtime, _ := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			err := runtime.validateCompatibleImage(context.Background(), "workbench:dev")
			if (err != nil) != test.wantErr {
				t.Fatalf("validateCompatibleImage() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestEnsureGatewayNetworkReconnectsAfterComposeReplacement(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		networks  string
		wantCalls int
	}{
		"already connected": {networks: `{"tobari-work-net":{}}`, wantCalls: 1},
		"reconnect":         {networks: `{"tobari-control":{}}`, wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &gatewayNetworkRunner{networks: test.networks}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureGatewayNetwork(context.Background(), "tobari-work-net"); err != nil {
				t.Fatal(err)
			}
			if len(runner.outputs) != test.wantCalls {
				t.Fatalf("Docker calls = %v, want %d", runner.outputs, test.wantCalls)
			}
			if test.wantCalls == 2 {
				want := []string{"network", "connect", "--alias", "gateway", "tobari-work-net", gatewayContainer}
				if !slices.Equal(runner.outputs[1].args, want) {
					t.Fatalf("reconnect argv = %v, want %v", runner.outputs[1].args, want)
				}
			}
		})
	}
}

func TestEnsureGatewayNetworkRejectsHostileBoundedObservations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		stdout []byte
		stderr []byte
		err    error
	}{
		{name: "trailing value", stdout: []byte(`{} {}`)},
		{name: "duplicate network", stdout: []byte(`{"n":{},"n":{}}`)},
		{name: "overflow", stdout: bytes.Repeat([]byte("x"), appliedClusterInspectLimit)},
		{name: "unexpected diagnostic", stdout: []byte(`{}`), stderr: []byte("warning")},
		{name: "inspect failure", stderr: []byte("daemon unavailable"), err: errors.New("inspect failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &hostileNetworkObservationRunner{stdout: test.stdout, stderr: test.stderr, err: test.err}
			runtime := &Runtime{runner: runner}
			if err := runtime.ensureGatewayNetwork(context.Background(), "tobari-control"); err == nil {
				t.Fatal("hostile network observation authorized mutation")
			}
			if runner.calls != 1 {
				t.Fatalf("network observation calls = %d", runner.calls)
			}
		})
	}
}

func TestEnsureAuthBrokerNetworkReconnectsAfterComposeReplacement(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		networks  string
		wantCalls int
	}{
		"already connected": {networks: `{"tobari-control":{}}`, wantCalls: 1},
		"reconnect":         {networks: `{}`, wantCalls: 2},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			runner := &gatewayNetworkRunner{networks: test.networks}
			runtime, err := newRuntime(filepath.Join(root, "config"), filepath.Join(root, "state"), runner)
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.ensureAuthBrokerNetwork(context.Background(), "tobari-control"); err != nil {
				t.Fatal(err)
			}
			if len(runner.outputs) != test.wantCalls {
				t.Fatalf("Docker calls = %v, want %d", runner.outputs, test.wantCalls)
			}
			if test.wantCalls == 2 {
				want := []string{"network", "connect", "--alias", "auth-broker", "tobari-control", authBrokerContainer}
				if !slices.Equal(runner.outputs[1].args, want) {
					t.Fatalf("reconnect argv = %v, want %v", runner.outputs[1].args, want)
				}
			}
		})
	}
}
