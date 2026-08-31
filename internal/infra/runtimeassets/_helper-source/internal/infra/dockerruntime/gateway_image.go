package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

const (
	gatewayAPIKey  = "io.tobari.gateway-api" // #nosec G101 -- stable image-contract label key, not a credential.
	gatewayRoleKey = "io.tobari.gateway-role"
	gatewayRole    = "enforcement"
)

type gatewayImageMetadata struct {
	ID           string   `json:"Id"`
	RepoDigests  []string `json:"RepoDigests"`
	Architecture string   `json:"Architecture"`
	OS           string   `json:"Os"`
	Config       struct {
		User       string                     `json:"User"`
		Labels     map[string]string          `json:"Labels"`
		Entrypoint []string                   `json:"Entrypoint"`
		Volumes    map[string]json.RawMessage `json:"Volumes"`
	} `json:"Config"`
}

type dockerServerMetadata struct {
	Architecture string `json:"Arch"`
	OS           string `json:"Os"`
}

func (r *Runtime) prepareGatewayImage(ctx context.Context) (string, string, error) {
	selection, err := r.imageResolver().GatewayImage(ctx, r)
	if err != nil {
		return "", "", err
	}
	if selection.BuildIfMissing {
		if err := r.ensureLocalGatewayImage(ctx, selection.Image); err != nil {
			return "", "", err
		}
	}
	identity, err := r.verifyGatewayImage(ctx, selection.Image, selection.RequireDigest)
	if err != nil {
		return "", "", err
	}
	return selection.Image, identity, nil
}

func (r *Runtime) ensureLocalGatewayImage(ctx context.Context, image string) error {
	present, err := r.observeLocalGatewayImage(ctx, image)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fault.Wrap(
			fault.KindUnavailable, "gateway_image_unavailable",
			"The selected local Gateway image could not be observed safely.", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker image availability before retrying."},
		)
	}
	if present {
		return nil
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return err
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	args := []string{
		"buildx", "build", "--progress=plain", "--load",
		"--tag", image,
		"--file", filepath.Join(runtimeDirectory, "gateway", "Dockerfile"),
		"--build-arg", "MITMPROXY_IMAGE=" + versions["MITMPROXY_IMAGE"],
		filepath.Join(runtimeDirectory, "gateway"),
	}
	var output bytes.Buffer
	if err := r.runner.Run(ctx, args, os.Environ(), nil, &output, &output); err != nil {
		cause := fmt.Errorf("build pinned Gateway: %w: %s", err, boundedDiagnostic(output.Bytes()))
		return fault.Wrap(
			fault.KindUnavailable, "gateway_image_build_failed",
			"the pinned Gateway could not be built locally", false, cause,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker build support and network access for the pinned Gateway inputs."},
		)
	}
	return nil
}

func (r *Runtime) observeLocalGatewayImage(ctx context.Context, image string) (bool, error) {
	inspectContext, cancel := context.WithTimeout(ctx, componentImageInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: 2048}
	stderr := &boundedBuffer{limit: 2048}
	err := r.runner.Run(inspectContext, []string{"image", "inspect", "--format", "{{.Id}}", image}, os.Environ(), nil, stdout, stderr)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	if stdout.overflow || stderr.overflow {
		return false, fmt.Errorf("local Gateway image observation exceeds bounded output")
	}
	if err != nil {
		if isMissingRuntimeImageInspect(err, stderr.buffer.Bytes(), image) || isMissingRuntimeImageInspect(err, stdout.buffer.Bytes(), image) {
			return false, nil
		}
		return false, fmt.Errorf("inspect local Gateway image: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return false, fmt.Errorf("local Gateway image inspect emitted diagnostic output")
	}
	identity := strings.TrimSpace(stdout.buffer.String())
	if !imageIDPattern.MatchString(identity) || strings.Count(stdout.buffer.String(), "\n") > 1 {
		return false, fmt.Errorf("local Gateway image identity is invalid or ambiguous")
	}
	return true, nil
}

func (r *Runtime) verifyGatewayImage(ctx context.Context, image string, requireDigest bool) (string, error) {
	if requireDigest {
		if err := validateGatewayImageReference(image); err != nil {
			return "", fault.Wrap(
				fault.KindContract, "gateway_image_incompatible",
				"The configured Gateway image is not an immutable digest reference.", false, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
			)
		}
	}

	metadata, err := r.inspectGatewayImage(ctx, image)
	if errors.Is(err, errComponentImageMissing) && requireDigest {
		if pullErr := r.pullComponentImage(ctx, image); pullErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			return "", fault.Wrap(
				fault.KindUnavailable, "gateway_image_unavailable",
				"The verified Gateway image could not be obtained; check Docker registry access and retry.", true, pullErr,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker and registry access."},
				fault.NextAction{Command: "cluster up", Reason: "Retry the shared cluster after the image is available."},
			)
		}
		metadata, err = r.inspectGatewayImage(ctx, image)
	}
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		var observationError componentImageObservationError
		if !requireDigest || errors.As(err, &observationError) {
			return "", fault.Wrap(
				fault.KindUnavailable, "gateway_image_unavailable",
				"The selected local Gateway image could not be inspected.", true, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker image availability."},
			)
		}
		return "", fault.Wrap(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not satisfy Tobari's verified image contract.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}

	if requireDigest && !hasDigest(metadata.RepoDigests, image) {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image did not resolve to the configured immutable digest.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	if !imageIDPattern.MatchString(metadata.ID) {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not expose a stable content identity.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image contract."},
		)
	}
	labels := metadata.Config.Labels
	selectedAPI := parseAPIOrZero(labels[gatewayAPIKey])
	if selectedAPI != buildidentity.RequiredGatewayAPI {
		return "", r.incompatibleComponentAPI("Gateway", selectedAPI, buildidentity.RequiredGatewayAPI, "gateway_image_incompatible")
	}
	if labels[gatewayRoleKey] != gatewayRole {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not declare Tobari's enforcement API contract.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	if isRootImageUser(metadata.Config.User) {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image must declare a non-root default user.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image configuration."},
		)
	}
	if len(metadata.Config.Entrypoint) != 1 || metadata.Config.Entrypoint[0] != "/opt/tobari/entrypoint.sh" {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image does not declare Tobari's enforcement entrypoint.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect the Gateway image entrypoint contract."},
		)
	}
	if err := validateComponentImageVolumes(metadata.Config.Volumes, finalGatewayInheritedCAPath); err != nil {
		return "", fault.Wrap(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image declares a writable volume outside Tobari's reviewed mount closure.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed Gateway image volume contract."},
		)
	}

	server, err := r.inspectDockerServer(ctx)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", fault.Wrap(
			fault.KindUnavailable, "gateway_image_unavailable",
			"Docker Engine platform information could not be read before Gateway startup.", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine readiness."},
		)
	}
	imageOS, imageArch := normalizePlatform(metadata.OS, metadata.Architecture)
	serverOS, serverArch := normalizePlatform(server.OS, server.Architecture)
	if imageOS == "" || imageArch == "" || serverOS == "" || serverArch == "" || imageOS != serverOS || imageArch != serverArch {
		return "", fault.New(
			fault.KindContract, "gateway_image_incompatible",
			"The Gateway image architecture does not match the Docker Engine.", false,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine and Gateway image platforms."},
		)
	}
	return metadata.ID, nil
}

func parseAPIOrZero(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func (r *Runtime) inspectGatewayImage(ctx context.Context, image string) (gatewayImageMetadata, error) {
	observed, err := r.inspectBoundedComponentImage(ctx, image)
	if err != nil {
		return gatewayImageMetadata{}, err
	}
	var metadata gatewayImageMetadata
	metadata.ID = observed.ID
	metadata.RepoDigests = observed.RepoDigests
	metadata.Architecture = observed.Architecture
	metadata.OS = observed.OS
	metadata.Config.User = observed.Config.User
	metadata.Config.Labels = observed.Config.Labels
	metadata.Config.Entrypoint = observed.Config.Entrypoint
	metadata.Config.Volumes = observed.Config.Volumes
	return metadata, nil
}

func (r *Runtime) inspectDockerServer(ctx context.Context) (dockerServerMetadata, error) {
	inspectContext, cancel := context.WithTimeout(ctx, componentImageInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: 2048}
	stderr := &boundedBuffer{limit: 2048}
	err := r.runner.Run(
		inspectContext, []string{"version", "--format", `{"Arch":{{json .Server.Arch}},"Os":{{json .Server.Os}}}`},
		os.Environ(), nil, stdout, stderr,
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return dockerServerMetadata{}, ctxErr
	}
	if stdout.overflow || stderr.overflow {
		return dockerServerMetadata{}, fmt.Errorf("Docker Engine platform metadata exceeds bounded output")
	}
	if err != nil {
		return dockerServerMetadata{}, err
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return dockerServerMetadata{}, fmt.Errorf("Docker Engine platform inspect emitted diagnostic output")
	}
	var metadata dockerServerMetadata
	if err := decodeStrictJSON(bytes.TrimSpace(stdout.buffer.Bytes()), &metadata); err != nil {
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
