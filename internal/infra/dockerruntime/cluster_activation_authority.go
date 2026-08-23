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
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	clusterComposeAssetLimit      = 128 * 1024
	freshResourceInspectLimit     = 4 * 1024
	freshResourceInspectTimeout   = 2 * time.Second
	freshResourceAuthorityTimeout = 20 * time.Second
)

// freshClusterResourceAuthority is the closed deletion authority for an
// activation that began from confirmed state absence. It is intentionally a
// fixed set rather than a discovery result.
type freshClusterResourceAuthority struct {
	Containers []string `json:"containers"`
	Networks   []string `json:"networks"`
	Volumes    []string `json:"volumes"`
}

func expectedFreshClusterResourceAuthority() freshClusterResourceAuthority {
	containers := []string{gatewayContainer, opaContainer}
	if brokerRuntimeEnabled {
		containers = append(containers, authBrokerContainer)
	}
	slices.Sort(containers)
	return freshClusterResourceAuthority{
		Containers: containers,
		Networks:   []string{"tobari-control", "tobari-egress"},
		Volumes:    []string{"tobari-gateway-ca", policyBundleVolume, "tobari-public-ca"},
	}
}

func (a freshClusterResourceAuthority) Validate() error {
	expected := expectedFreshClusterResourceAuthority()
	if !slices.Equal(a.Containers, expected.Containers) ||
		!slices.Equal(a.Networks, expected.Networks) || !slices.Equal(a.Volumes, expected.Volumes) {
		return fmt.Errorf("fresh shared-cluster resource authority is incomplete or ambiguous")
	}
	return nil
}

func (r *Runtime) proveFreshClusterResourcesAbsent(ctx context.Context) (freshClusterResourceAuthority, error) {
	authority := expectedFreshClusterResourceAuthority()
	if err := r.verifyFreshClusterResourcesAbsent(ctx, authority); err != nil {
		return freshClusterResourceAuthority{}, err
	}
	return authority, nil
}

func (r *Runtime) verifyFreshClusterResourcesAbsent(
	ctx context.Context, authority freshClusterResourceAuthority,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	scanContext, cancel := context.WithTimeout(ctx, freshResourceAuthorityTimeout)
	defer cancel()
	for _, name := range authority.Containers {
		if err := r.requireDockerResourceAbsent(scanContext, "container", name); err != nil {
			return err
		}
	}
	for _, name := range authority.Networks {
		if err := r.requireDockerResourceAbsent(scanContext, "network", name); err != nil {
			return err
		}
	}
	for _, name := range authority.Volumes {
		if err := r.requireDockerResourceAbsent(scanContext, "volume", name); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) requireDockerResourceAbsent(ctx context.Context, kind, name string) error {
	inspectContext, cancel := context.WithTimeout(ctx, freshResourceInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: freshResourceInspectLimit / 2}
	stderr := &boundedBuffer{limit: freshResourceInspectLimit / 2}
	var args []string
	switch kind {
	case "container":
		args = []string{"container", "ls", "--all", "--filter", "name=^/" + name + "$", "--format", "{{.Names}}"}
	case "network", "volume":
		args = []string{kind, "ls", "--filter", "name=^" + name + "$", "--format", "{{.Name}}"}
	default:
		return fmt.Errorf("fresh shared-cluster resource kind is invalid")
	}
	err := r.runner.Run(
		inspectContext, args,
		os.Environ(), nil, stdout, stderr,
	)
	if stdout.overflow || stderr.overflow {
		return fmt.Errorf("inspect fresh %s %s exceeded bounded output", kind, name)
	}
	if err != nil {
		return fmt.Errorf("inspect fresh %s %s failed ambiguously: %w", kind, name, err)
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return fmt.Errorf("inspect fresh %s %s emitted diagnostic output", kind, name)
	}
	if len(bytes.TrimSpace(stdout.buffer.Bytes())) != 0 {
		return fmt.Errorf("fresh shared-cluster %s %s already exists or is ambiguous", kind, name)
	}
	return nil
}

func (r *Runtime) resolveCandidateImageID(ctx context.Context, reference string) (string, error) {
	inspectContext, cancel := context.WithTimeout(ctx, freshResourceInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: freshResourceInspectLimit / 2}
	stderr := &boundedBuffer{limit: freshResourceInspectLimit / 2}
	err := r.runner.Run(
		inspectContext, []string{"image", "inspect", "--format", "{{.Id}}", reference},
		os.Environ(), nil, stdout, stderr,
	)
	if stdout.overflow || stderr.overflow {
		return "", fmt.Errorf("candidate image identity exceeds bounded output")
	}
	if err != nil {
		return "", fmt.Errorf("inspect candidate image identity: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return "", fmt.Errorf("candidate image inspect emitted diagnostic output")
	}
	identity := strings.TrimSpace(stdout.buffer.String())
	if !imageIDPattern.MatchString(identity) || strings.Count(stdout.buffer.String(), "\n") > 1 {
		return "", fmt.Errorf("candidate image identity is invalid or ambiguous")
	}
	return identity, nil
}

func (r *Runtime) observeClusterContainerNetworks(
	ctx context.Context, container string,
) (map[string]json.RawMessage, error) {
	inspectContext, cancel := context.WithTimeout(ctx, appliedClusterInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	stderr := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	err := r.runner.Run(
		inspectContext,
		[]string{"inspect", "--format", "{{json .NetworkSettings.Networks}}", container},
		os.Environ(), nil, stdout, stderr,
	)
	if stdout.overflow || stderr.overflow {
		return nil, fmt.Errorf("Docker network observation exceeds %d bytes", appliedClusterInspectLimit)
	}
	if err != nil {
		return nil, fmt.Errorf("bounded Docker network observation failed: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return nil, fmt.Errorf("Docker network observation emitted diagnostic output")
	}
	data := stdout.buffer.Bytes()
	if err := validateNoDuplicateJSONKeys(data); err != nil {
		return nil, fmt.Errorf("Docker network observation is ambiguous: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var networks map[string]json.RawMessage
	if err := decoder.Decode(&networks); err != nil {
		return nil, fmt.Errorf("decode Docker network observation: %w", err)
	}
	if networks == nil {
		return nil, fmt.Errorf("Docker network observation is null")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("Docker network observation contains trailing data")
	}
	return networks, nil
}

func (r *Runtime) runBoundedNetworkMutation(ctx context.Context, args []string) error {
	mutationContext, cancel := context.WithTimeout(ctx, appliedClusterInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	stderr := &boundedBuffer{limit: appliedClusterInspectLimit / 2}
	err := r.runner.Run(mutationContext, args, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return fmt.Errorf("Docker network mutation output exceeds %d bytes", appliedClusterInspectLimit)
	}
	if err != nil {
		return fmt.Errorf("bounded Docker network mutation failed: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if len(bytes.TrimSpace(stdout.buffer.Bytes())) != 0 || len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return fmt.Errorf("Docker network mutation returned ambiguous output")
	}
	return nil
}

func (r *Runtime) captureCandidateComposeAssets(
	state tobari.State, profile tobari.SharedClusterAppliedProfile,
) (tobari.SharedClusterComposeAssets, error) {
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.SharedClusterComposeAssets{}, err
	}
	if state.AssetVersion != version {
		return tobari.SharedClusterComposeAssets{}, fmt.Errorf("candidate runtime asset version differs from the embedded closure")
	}
	assets := tobari.SharedClusterComposeAssets{}
	if assets.BaseSHA256, err = r.validateEmbeddedComposeAsset(state, "compose.yaml"); err != nil {
		return tobari.SharedClusterComposeAssets{}, err
	}
	if brokerRuntimeEnabled {
		if assets.BuildSHA256, err = r.validateEmbeddedComposeAsset(state, "compose.experimental.yaml"); err != nil {
			return tobari.SharedClusterComposeAssets{}, err
		}
	}
	transport, ok := profile.PermissionTransport()
	if !ok {
		return tobari.SharedClusterComposeAssets{}, fmt.Errorf("candidate permission profile is invalid")
	}
	assets.PermissionSHA256, err = r.validateEmbeddedComposeAsset(
		state, "compose.permission-"+string(transport)+".yaml",
	)
	if err != nil {
		return tobari.SharedClusterComposeAssets{}, err
	}
	return assets, nil
}

func (r *Runtime) validateEmbeddedComposeAsset(state tobari.State, name string) (string, error) {
	expected, err := runtimeassets.Read(name)
	if err != nil {
		return "", err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(expected))
	if err := r.validateRetainedComposeAsset(state, name, digest); err != nil {
		return "", err
	}
	return digest, nil
}

func (r *Runtime) validateRollbackClosure(state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	if err := validateAppliedSharedClusterEntryForBuild(state.Applied); err != nil {
		return err
	}
	if err := r.validateRuntimeDirectoryAuthority(state); err != nil {
		return err
	}
	assets := state.Applied.ComposeAssets
	if err := r.validateRetainedComposeAsset(state, "compose.yaml", assets.BaseSHA256); err != nil {
		return err
	}
	if brokerRuntimeEnabled {
		if assets.BuildSHA256 == "" {
			return fmt.Errorf("retained research Compose receipt omits its build profile")
		}
		if err := r.validateRetainedComposeAsset(state, "compose.experimental.yaml", assets.BuildSHA256); err != nil {
			return err
		}
	} else if assets.BuildSHA256 != "" {
		return fmt.Errorf("retained standard Compose receipt contains a research build profile")
	}
	if state.Applied.PermissionProfile == tobari.SharedClusterProfilePrePlatform {
		if assets.PermissionSHA256 != "" {
			return fmt.Errorf("retained pre-platform Compose receipt contains a successor permission profile")
		}
		return nil
	}
	transport, ok := state.Applied.PermissionProfile.PermissionTransport()
	if !ok || assets.PermissionSHA256 == "" {
		return fmt.Errorf("retained Compose receipt omits its applied permission profile")
	}
	return r.validateRetainedComposeAsset(
		state, "compose.permission-"+string(transport)+".yaml", assets.PermissionSHA256,
	)
}

func (r *Runtime) validateRuntimeDirectoryAuthority(state tobari.State) error {
	expected := filepath.Join(r.stateDirectory, "runtime", state.AssetVersion)
	if state.RuntimeDirectory != expected || filepath.Clean(state.RuntimeDirectory) != expected {
		return fmt.Errorf("retained runtime directory does not match its asset identity")
	}
	realState, stateErr := filepath.EvalSymlinks(r.stateDirectory)
	realRuntime, runtimeErr := filepath.EvalSymlinks(state.RuntimeDirectory)
	if stateErr != nil || runtimeErr != nil || realRuntime != filepath.Join(realState, "runtime", state.AssetVersion) {
		return fmt.Errorf("retained runtime directory has a symlinked or unavailable parent")
	}
	for _, directory := range []string{r.stateDirectory, filepath.Join(r.stateDirectory, "runtime"), state.RuntimeDirectory} {
		if err := requireOwnerOnlyPath(directory, true); err != nil {
			return fmt.Errorf("retained runtime directory is unsafe: %w", err)
		}
	}
	return nil
}

func (r *Runtime) validateRetainedComposeAsset(state tobari.State, name, expectedDigest string) error {
	if err := r.validateRuntimeDirectoryAuthority(state); err != nil {
		return err
	}
	path := filepath.Join(state.RuntimeDirectory, name)
	if filepath.Dir(path) != state.RuntimeDirectory || filepath.Base(path) != name {
		return fmt.Errorf("retained Compose asset path is invalid")
	}
	if err := requireOwnerOnlyRegularFile(path); err != nil {
		return fmt.Errorf("retained %s is unsafe: %w", name, err)
	}
	data, err := readOwnerPolicyFile(path, clusterComposeAssetLimit)
	if err != nil {
		return fmt.Errorf("read retained %s: %w", name, err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != expectedDigest {
		return fmt.Errorf("retained %s does not match its applied receipt", name)
	}
	return nil
}

func prePlatformComposeAssets() tobari.SharedClusterComposeAssets {
	assets := tobari.SharedClusterComposeAssets{BaseSHA256: prePlatformComposeSHA256}
	if brokerRuntimeEnabled {
		assets.BuildSHA256 = prePlatformExperimentalSHA256
	}
	return assets
}
