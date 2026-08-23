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
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func (r *Runtime) prepareState(ctx context.Context) (tobari.State, error) {
	for name, path := range map[string]string{
		"configuration": r.configDirectory,
		"state":         r.stateDirectory,
		"data":          r.dataDirectory,
	} {
		if err := r.ensurePrivateDirectory(path); err != nil {
			return tobari.State{}, fmt.Errorf("prepare %s directory: %w", name, err)
		}
	}
	if brokerRuntimeEnabled {
		if _, err := r.prepareAuthProjection(); err != nil {
			return tobari.State{}, fmt.Errorf("prepare Auth Broker provider projection: %w", err)
		}
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.State{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return tobari.State{}, err
	}
	if err := runtimeassets.MaterializeExposureHelperSource(filepath.Join(runtimeDirectory, "helper-source")); err != nil {
		return tobari.State{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.State{}, fmt.Errorf("prepare Context catalog: %w", err)
	}
	if err := r.withPolicyProjectionLock(ctx, func() error {
		return r.recoverAllPolicySourceTransactions(ctx)
	}); err != nil {
		return tobari.State{}, fmt.Errorf("recover interrupted Context policy source transaction: %w", err)
	}
	projection, err := r.buildAggregateProjection(ctx)
	if err != nil {
		return tobari.State{}, fmt.Errorf("prepare aggregate Context projection: %w", err)
	}
	if err := r.ensureProjectPrincipalRegistry(ctx); err != nil {
		return tobari.State{}, fmt.Errorf("validate project principal registry: %w", err)
	}
	if err := r.ensureHostLoopbackStore(ctx); err != nil {
		return tobari.State{}, fmt.Errorf("validate Host Loopback store: %w", err)
	}
	if err := r.ensureInteractiveAttachmentStore(ctx); err != nil {
		return tobari.State{}, fmt.Errorf("validate interactive attachment store: %w", err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: runtimeDirectory,
		AggregateRevision: projection.Revision, ManifestCount: projection.ManifestCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: version,
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	return state, nil
}

func initializeFile(target, asset string, mode os.FileMode) error {
	data, err := runtimeassets.Read(asset)
	if err != nil {
		return err
	}
	return initializeBytes(target, data, mode)
}

func initializeBytes(target string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s must be a regular file", filepath.Base(target))
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set configuration file permissions: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect configuration file: %w", err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- fixed child and O_EXCL prevent overwrite.
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	return nil
}

func (r *Runtime) statePath() string { return filepath.Join(r.stateDirectory, "state.json") }

func (r *Runtime) writeState(state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	commit := func() error {
		return r.withClusterLock(func() error {
			if err := os.MkdirAll(r.stateDirectory, 0o700); err != nil {
				return err
			}
			if err := os.Chmod(r.stateDirectory, 0o700); err != nil { // #nosec G302 -- shared state is owner-only.
				return err
			}
			return writeAtomicJSON(r.statePath(), state)
		})
	}
	if r.clusterStateWriteHook != nil {
		return r.clusterStateWriteHook(state, commit)
	}
	return commit()
}

type statePublicationOutcome string

const (
	statePublicationPrevious statePublicationOutcome = "previous"
	statePublicationNew      statePublicationOutcome = "new"
	statePublicationUnknown  statePublicationOutcome = "unknown"
)

var containerIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (r *Runtime) publishStateWithVerification(
	ctx context.Context, previous *tobari.State, candidate tobari.State,
) (statePublicationOutcome, error) {
	writeErr := r.writeState(candidate)
	observed, exists, readErr := r.LoadState(ctx)
	if readErr == nil && exists && observed == candidate {
		return statePublicationNew, writeErr
	}
	previousVisible := readErr == nil && previous != nil && exists && observed == *previous
	if previous == nil {
		previousVisible = readErr == nil && !exists
	}
	if previousVisible {
		if writeErr == nil {
			writeErr = fmt.Errorf("shared state publication did not become visible")
		}
		return statePublicationPrevious, writeErr
	}
	return statePublicationUnknown, errors.Join(
		writeErr, readErr, fmt.Errorf("shared state publication authority is unknown"),
	)
}

func (r *Runtime) migratePrePlatformSharedClusterState(
	ctx context.Context, state tobari.State,
) (tobari.State, error) {
	if state.SchemaVersion != 1 {
		return tobari.State{}, fmt.Errorf("pre-platform state migration requires schema 1")
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	if err := r.validatePrePlatformRuntimeAuthority(state); err != nil {
		return tobari.State{}, err
	}
	if err := r.verifyAppliedClusterBuildShape(ctx); err != nil {
		return tobari.State{}, err
	}
	snapshot, err := r.observeAppliedClusterSnapshot(ctx)
	if err != nil {
		return tobari.State{}, fmt.Errorf("capture predecessor service image identities: %w", err)
	}
	if err := verifyPrePlatformGatewayProjection(snapshot.gateway); err != nil {
		return tobari.State{}, err
	}
	migrated := state
	migrated.SchemaVersion = 2
	migrated.Applied = tobari.SharedClusterAppliedEntry{
		AggregateRevision: state.AggregateRevision,
		AssetVersion:      state.AssetVersion,
		GatewayImageID:    snapshot.images.gateway,
		OPAImageID:        snapshot.images.opa,
		AuthBrokerImageID: snapshot.images.authBroker,
		PermissionProfile: tobari.SharedClusterProfilePrePlatform,
	}
	if err := migrated.Validate(); err != nil {
		return tobari.State{}, err
	}
	outcome, publicationErr := r.publishStateWithVerification(ctx, &state, migrated)
	if outcome != statePublicationNew || publicationErr != nil {
		return tobari.State{}, fmt.Errorf(
			"publish migrated shared-cluster applied entry (%s): %w", outcome, publicationErr,
		)
	}
	return migrated, nil
}

const (
	prePlatformAssetVersion  = "97f1c509bc4d7f2f"
	prePlatformComposeSize   = 3502
	prePlatformComposeSHA256 = "4d188071e431aae7d415a1b2beec2efbb798668c993e5f6d164cf2f58f314776"
)

func (r *Runtime) validatePrePlatformRuntimeAuthority(state tobari.State) error {
	if state.AssetVersion != prePlatformAssetVersion {
		return fmt.Errorf("schema-1 state does not name the reviewed predecessor asset version")
	}
	expected := filepath.Join(r.stateDirectory, "runtime", prePlatformAssetVersion)
	if filepath.Clean(state.RuntimeDirectory) != expected || state.RuntimeDirectory != expected {
		return fmt.Errorf("schema-1 runtime directory does not match its asset identity")
	}
	realState, stateErr := filepath.EvalSymlinks(r.stateDirectory)
	realRuntime, runtimeErr := filepath.EvalSymlinks(state.RuntimeDirectory)
	if stateErr != nil || runtimeErr != nil || realRuntime != filepath.Join(realState, "runtime", prePlatformAssetVersion) {
		return fmt.Errorf("schema-1 runtime directory has a symlinked or unavailable parent")
	}
	for _, directory := range []string{r.stateDirectory, filepath.Join(r.stateDirectory, "runtime"), state.RuntimeDirectory} {
		if err := requireOwnerOnlyPath(directory, true); err != nil {
			return fmt.Errorf("predecessor runtime directory is unsafe: %w", err)
		}
	}
	baseCompose := filepath.Join(state.RuntimeDirectory, "compose.yaml")
	if err := requireOwnerOnlyRegularFile(baseCompose); err != nil {
		return fmt.Errorf("pre-platform base compose asset is unsafe: %w", err)
	}
	data, err := readOwnerPolicyFile(baseCompose, prePlatformComposeSize)
	if err != nil {
		return fmt.Errorf("read pre-platform base compose asset: %w", err)
	}
	digest := sha256.Sum256(data)
	if len(data) != prePlatformComposeSize || fmt.Sprintf("%x", digest[:]) != prePlatformComposeSHA256 {
		return fmt.Errorf("pre-platform base compose asset identity is invalid")
	}
	for _, name := range []string{"compose.permission-unix.yaml", "compose.permission-loopback_tcp.yaml"} {
		path := filepath.Join(state.RuntimeDirectory, name)
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("schema-1 state conflicts with successor permission profile assets")
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect predecessor permission profile assets: %w", err)
		}
	}
	return nil
}

type appliedClusterImageIDs struct {
	gateway    string
	opa        string
	authBroker string
}

const (
	appliedClusterInspectLimit   = 32 * 1024
	appliedClusterInspectTimeout = 2 * time.Second
	appliedClusterScanTimeout    = 7 * time.Second
)

const appliedClusterInspectTemplate = `{"container_id":{{json .Id}},"owner":{{json (index .Config.Labels "io.tobari.owner")}},"component":{{json (index .Config.Labels "io.tobari.component")}},"role":{{json (index .Config.Labels "io.tobari.gateway-role")}},"image_id":{{json .Image}},"state":{{json .State.Status}},"health":{{if .State.Health}}{{json .State.Health.Status}}{{else}}"none"{{end}},"environment":{{json .Config.Env}},"mount_destinations":[{{range $index,$mount := .Mounts}}{{if $index}},{{end}}{{json $mount.Destination}}{{end}}]}`

type appliedClusterComponentObservation struct {
	ContainerID       string   `json:"container_id"`
	Owner             string   `json:"owner"`
	Component         string   `json:"component"`
	Role              string   `json:"role"`
	ImageID           string   `json:"image_id"`
	State             string   `json:"state"`
	Health            string   `json:"health"`
	Environment       []string `json:"environment"`
	MountDestinations []string `json:"mount_destinations"`
}

type appliedClusterSnapshot struct {
	images     appliedClusterImageIDs
	gateway    appliedClusterComponentObservation
	opa        appliedClusterComponentObservation
	authBroker appliedClusterComponentObservation
}

func (r *Runtime) observeAppliedClusterSnapshot(ctx context.Context) (appliedClusterSnapshot, error) {
	scanContext, cancel := context.WithTimeout(ctx, appliedClusterScanTimeout)
	defer cancel()
	first, err := r.observeAppliedClusterSnapshotPass(scanContext)
	if err != nil {
		return appliedClusterSnapshot{}, err
	}
	second, err := r.observeAppliedClusterSnapshotPass(scanContext)
	if err != nil {
		return appliedClusterSnapshot{}, fmt.Errorf("fence applied component tuple: %w", err)
	}
	if !sameAppliedClusterSnapshot(first, second) {
		return appliedClusterSnapshot{}, fmt.Errorf("applied component tuple changed while it was observed")
	}
	return first, nil
}

func (r *Runtime) observeAppliedClusterSnapshotPass(ctx context.Context) (appliedClusterSnapshot, error) {
	gateway, missing, err := r.observeAppliedClusterComponent(ctx, "gateway", gatewayContainer)
	if err != nil {
		return appliedClusterSnapshot{}, fmt.Errorf("observe Gateway applied identity: %w", err)
	}
	if missing {
		return appliedClusterSnapshot{}, fmt.Errorf("Gateway applied component is missing")
	}
	opa, missing, err := r.observeAppliedClusterComponent(ctx, "opa", opaContainer)
	if err != nil {
		return appliedClusterSnapshot{}, fmt.Errorf("observe OPA applied identity: %w", err)
	}
	if missing {
		return appliedClusterSnapshot{}, fmt.Errorf("OPA applied component is missing")
	}
	snapshot := appliedClusterSnapshot{
		images: appliedClusterImageIDs{gateway: gateway.ImageID, opa: opa.ImageID}, gateway: gateway, opa: opa,
	}
	if !brokerRuntimeEnabled {
		return snapshot, nil
	}
	authBroker, missing, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
	if err != nil {
		return appliedClusterSnapshot{}, fmt.Errorf("observe Auth Broker applied identity: %w", err)
	}
	if missing {
		return appliedClusterSnapshot{}, fmt.Errorf("Auth Broker applied component is missing")
	}
	snapshot.images.authBroker = authBroker.ImageID
	snapshot.authBroker = authBroker
	return snapshot, nil
}

func (r *Runtime) verifyAppliedClusterBuildShape(ctx context.Context) error {
	if brokerRuntimeEnabled {
		return nil
	}
	_, missing, err := r.observeAppliedClusterComponent(ctx, "auth-broker", authBrokerContainer)
	if err != nil {
		return fmt.Errorf("inspect unexpected Auth Broker identity: %w", err)
	}
	if !missing {
		return fmt.Errorf("standard shared cluster contains an Auth Broker container")
	}
	return nil
}

func sameAppliedClusterSnapshot(left, right appliedClusterSnapshot) bool {
	return left.images == right.images &&
		sameAppliedClusterComponent(left.gateway, right.gateway) &&
		sameAppliedClusterComponent(left.opa, right.opa) &&
		sameAppliedClusterComponent(left.authBroker, right.authBroker)
}

func sameAppliedClusterComponent(left, right appliedClusterComponentObservation) bool {
	return left.ContainerID == right.ContainerID && left.Owner == right.Owner &&
		left.Component == right.Component && left.Role == right.Role &&
		left.ImageID == right.ImageID && left.State == right.State && left.Health == right.Health &&
		slices.Equal(left.Environment, right.Environment) &&
		slices.Equal(left.MountDestinations, right.MountDestinations)
}

func (r *Runtime) observeAppliedClusterComponent(
	ctx context.Context, component, container string,
) (appliedClusterComponentObservation, bool, error) {
	observationContext, cancel := context.WithTimeout(ctx, appliedClusterInspectTimeout)
	defer cancel()
	output := &boundedBuffer{limit: appliedClusterInspectLimit}
	err := r.runner.Run(
		observationContext,
		[]string{"inspect", "--format", appliedClusterInspectTemplate, container},
		os.Environ(), nil, output, output,
	)
	data := append([]byte(nil), output.buffer.Bytes()...)
	if err != nil {
		if isMissingDockerResource(err, data) {
			return appliedClusterComponentObservation{}, true, nil
		}
		return appliedClusterComponentObservation{}, false, fmt.Errorf("bounded Docker inspect failed: %w", err)
	}
	if output.overflow {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("Docker inspect output exceeds %d bytes", appliedClusterInspectLimit)
	}
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("Docker inspect payload is ambiguous: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var observation appliedClusterComponentObservation
	if err := decoder.Decode(&observation); err != nil {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("decode Docker inspect payload: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("Docker inspect payload contains trailing data")
	}
	if !containerIDPattern.MatchString(observation.ContainerID) || !imageIDPattern.MatchString(observation.ImageID) ||
		observation.Owner != ownerValue || observation.Component != component ||
		observation.State != "running" || observation.Health != "healthy" {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("%s applied component identity or health is invalid", component)
	}
	if component == "gateway" && observation.Role != gatewayRole {
		return appliedClusterComponentObservation{}, false, fmt.Errorf("Gateway enforcement role is invalid")
	}
	return observation, false, nil
}

func verifyPrePlatformGatewayProjection(gateway appliedClusterComponentObservation) error {
	for _, entry := range gateway.Environment {
		if strings.HasPrefix(entry, "TOBARI_PERMISSION_INGESTION_") {
			return fmt.Errorf("schema-1 state conflicts with a successor Gateway permission profile")
		}
	}
	for _, destination := range gateway.MountDestinations {
		if destination == "/run/tobari/permission-ingestion" {
			return fmt.Errorf("schema-1 state conflicts with a successor Gateway permission mount")
		}
	}
	return nil
}

func (r *Runtime) withClusterLock(action func() error) error {
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return fmt.Errorf("prepare shared state directory: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "cluster.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("cluster lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect cluster lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed state child after lstat.
	if err != nil {
		return fmt.Errorf("open cluster lock: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect cluster lock: %w", err)
	}
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock shared state: %w", lockErr)
		}
		if acquired {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer unlockProjectFile(file)
	return action()
}

// LoadState returns absence separately from corrupt state.
func (r *Runtime) LoadState(ctx context.Context) (tobari.State, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, false, err
	}
	info, err := os.Lstat(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProjectStateBytes {
		return tobari.State{}, false, fmt.Errorf("Tobari state file is unsafe")
	}
	data, err := os.ReadFile(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state tobari.State
	if err := decoder.Decode(&state); err != nil {
		return tobari.State{}, false, fmt.Errorf("decode Tobari state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.State{}, false, fmt.Errorf("Tobari state contains trailing data")
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, false, err
	}
	if state.SchemaVersion == 2 {
		if err := validateAppliedSharedClusterEntryForBuild(state.Applied); err != nil {
			return tobari.State{}, false, err
		}
	}
	return state, true, nil
}

func validateAppliedSharedClusterEntryForBuild(entry tobari.SharedClusterAppliedEntry) error {
	if brokerRuntimeEnabled && entry.AuthBrokerImageID == "" {
		return fmt.Errorf("experimental applied shared-cluster entry omits Auth Broker")
	}
	if !brokerRuntimeEnabled && entry.AuthBrokerImageID != "" {
		return fmt.Errorf("standard applied shared-cluster entry contains Auth Broker")
	}
	return nil
}

// Attach creates one exact container, internal network, and persistent home.

func (r *Runtime) recordRecentError(state tobari.State, message string) error {
	state.RecentError = message
	return r.writeState(state)
}
