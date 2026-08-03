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
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	gatewayAPIKey  = "io.tobari.gateway-api" // #nosec G101 -- stable image-contract label key, not a credential.
	gatewayRoleKey = "io.tobari.gateway-role"
	gatewayAPI     = "1"
	gatewayRole    = "enforcement"
)

type gatewayImageMetadata struct {
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	OS           string   `json:"Os"`
	Config       struct {
		User       string            `json:"User"`
		Labels     map[string]string `json:"Labels"`
		Entrypoint []string          `json:"Entrypoint"`
	} `json:"Config"`
}

type dockerServerMetadata struct {
	Architecture string `json:"Arch"`
	OS           string `json:"Os"`
}

func (r *Runtime) prepareGatewayImage(
	ctx context.Context, state tobari.State, environment []string, sourceBuild bool,
) (string, error) {
	if sourceBuild {
		image := "tobari-gateway:" + state.AssetVersion
		if err := r.buildGatewayImage(ctx, state, environment, image); err != nil {
			return "", err
		}
		if err := r.verifyGatewayImage(ctx, image, false); err != nil {
			return "", err
		}
		return image, nil
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return "", err
	}
	image := versions["GATEWAY_IMAGE"]
	if err := r.verifyGatewayImage(ctx, image, true); err != nil {
		return "", err
	}
	return image, nil
}

func (r *Runtime) buildGatewayImage(
	ctx context.Context, state tobari.State, environment []string, image string,
) error {
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	var output bytes.Buffer
	err = r.runner.Run(
		ctx,
		[]string{
			"build",
			"--build-arg", "MITMPROXY_IMAGE=" + versions["MITMPROXY_IMAGE"],
			"--tag", image,
			filepath.Join(state.RuntimeDirectory, "gateway"),
		},
		environment, nil, &output, &output,
	)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "gateway_source_build_failed",
			"The Gateway source build did not complete; inspect Docker and retry.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker and host access before retrying."},
		)
	}
	return nil
}

func (r *Runtime) verifyGatewayImage(ctx context.Context, image string, requireDigest bool) error {
	if requireDigest {
		if err := validateGatewayImageReference(image); err != nil {
			return fault.Wrap(
				fault.KindContract, "gateway_image_incompatible",
				"The configured Gateway image is not an immutable digest reference.", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
			)
		}
	}

	metadata, err := r.inspectGatewayImage(ctx, image)
	if err != nil && requireDigest {
		if _, pullErr := r.runner.Output(ctx, []string{"pull", image}, os.Environ()); pullErr != nil {
			return fault.Wrap(
				fault.KindUnavailable, "gateway_image_unavailable",
				"The verified Gateway image could not be obtained; check Docker registry access and retry.", true, pullErr,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker and registry access."},
				fault.NextAction{Command: "cluster up", Reason: "Retry the shared cluster after the image is available."},
			)
		}
		metadata, err = r.inspectGatewayImage(ctx, image)
	}
	if err != nil {
		return fault.Wrap(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not satisfy Tobari's verified image contract.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}

	if requireDigest && !hasDigest(metadata.RepoDigests, image) {
		return fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image did not resolve to the configured immutable digest.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	labels := metadata.Config.Labels
	if labels[gatewayAPIKey] != gatewayAPI || labels[gatewayRoleKey] != gatewayRole {
		return fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not declare Tobari's enforcement API contract.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	if isRootImageUser(metadata.Config.User) {
		return fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image must declare a non-root default user.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	if len(metadata.Config.Entrypoint) != 1 || metadata.Config.Entrypoint[0] != "/opt/tobari/entrypoint.sh" {
		return fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not declare Tobari's enforcement entrypoint.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Gateway image entrypoint contract."},
		)
	}

	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "gateway_image_unavailable",
			"Docker Engine platform information could not be read before Gateway startup.", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine readiness."},
		)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS == "" || imageArch == "" || serverOS == "" || serverArch == "" || imageOS != serverOS || imageArch != serverArch {
		return fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image architecture does not match the Docker Engine.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine and Gateway image platforms."},
		)
	}
	return nil
}

func (r *Runtime) inspectGatewayImage(ctx context.Context, image string) (gatewayImageMetadata, error) {
	output, err := r.runner.Output(
		ctx, []string{"image", "inspect", "--format", "{{json .}}", image}, os.Environ(),
	)
	if err != nil {
		return gatewayImageMetadata{}, err
	}
	var metadata gatewayImageMetadata
	decoder := json.NewDecoder(bytes.NewReader(output))
	if err := decoder.Decode(&metadata); err != nil {
		return gatewayImageMetadata{}, fmt.Errorf("decode Gateway image metadata: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return gatewayImageMetadata{}, fmt.Errorf("Gateway image metadata contains trailing data")
	}
	return metadata, nil
}

func (r *Runtime) inspectDockerServer(ctx context.Context) (dockerServerMetadata, error) {
	output, err := r.runner.Output(
		ctx, []string{"version", "--format", "{{json .Server}}"}, os.Environ(),
	)
	if err != nil {
		return dockerServerMetadata{}, err
	}
	var metadata dockerServerMetadata
	if err := json.Unmarshal(output, &metadata); err != nil {
		return dockerServerMetadata{}, fmt.Errorf("decode Docker Engine platform: %w", err)
	}
	return metadata, nil
}

func validateGatewayImageReference(image string) error {
	name, digest, found := strings.Cut(image, "@sha256:")
	if !found || name == "" || len(digest) != 64 {
		return fmt.Errorf("Gateway image reference must contain a 64-character sha256 digest")
	}
	for _, character := range digest {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return fmt.Errorf("Gateway image digest contains a non-hex character")
		}
	}
	if strings.Trim(digest, "0") == "" {
		return fmt.Errorf("Gateway image digest cannot be all zeroes")
	}
	return nil
}

func hasDigest(repoDigests []string, image string) bool {
	_, digest, found := strings.Cut(image, "@sha256:")
	if !found {
		return false
	}
	needle := "@sha256:" + digest
	for _, candidate := range repoDigests {
		if strings.HasSuffix(candidate, needle) {
			return true
		}
	}
	return false
}

func isRootImageUser(user string) bool {
	switch strings.TrimSpace(strings.ToLower(user)) {
	case "", "0", "0:0", "root", "root:root":
		return true
	default:
		return false
	}
}

func normalizePlatform(osName, architecture string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(osName)) {
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		osName = ""
	}
	switch strings.ToLower(strings.TrimSpace(architecture)) {
	case "amd64", "x86_64":
		architecture = "amd64"
	case "arm64", "aarch64":
		architecture = "arm64"
	case "arm", "armhf", "armv7":
		architecture = "arm"
	default:
		architecture = ""
	}
	return osName, architecture
}
