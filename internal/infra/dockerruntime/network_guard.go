package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"regexp"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const networkGuardEntrypoint = "/opt/tobari/network-guard.sh"

var imageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func networkGuardFailure(message string, err error) error {
	return fault.Wrap(
		fault.KindUnavailable,
		"network_guard_failed",
		message,
		false,
		err,
		fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine network-namespace and nftables support."},
	)
}

func (r *Runtime) gatewayRuntimeImageID(ctx context.Context) (string, error) {
	if _, err := r.inspectContainer(ctx, "gateway", gatewayContainer); err != nil {
		return "", err
	}
	output, err := r.runner.Output(
		ctx,
		[]string{"inspect", "--format", "{{.Image}}", gatewayContainer},
		os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("inspect Gateway image identity: %w: %s", err, boundedDiagnostic(output))
	}
	identity := strings.TrimSpace(string(output))
	if !imageIDPattern.MatchString(identity) {
		return "", fmt.Errorf("Gateway image identity is invalid")
	}
	return identity, nil
}

func (r *Runtime) runNetworkGuardHelper(
	ctx context.Context, imageID, target, mode string, arguments ...string,
) error {
	if !imageIDPattern.MatchString(imageID) {
		return fmt.Errorf("network guard image identity is invalid")
	}
	if mode != "gateway" && mode != "workspace" {
		return fmt.Errorf("network guard mode is invalid")
	}
	args := []string{
		"run", "--rm",
		"--network", "container:" + target,
		"--user", "0:0",
		"--read-only",
		"--cap-drop", "ALL",
		"--cap-add", "NET_ADMIN",
		"--security-opt", "no-new-privileges:true",
		"--entrypoint", networkGuardEntrypoint,
		imageID,
		mode,
	}
	args = append(args, arguments...)
	output, err := r.runner.Output(ctx, args, os.Environ())
	if err != nil {
		return fmt.Errorf("run %s network guard: %w: %s", mode, err, boundedDiagnostic(output))
	}
	want := "tobari-network-guard " + tobari.NetworkGuardRevision + " " + mode
	if strings.TrimSpace(string(output)) != want {
		return fmt.Errorf("%s network guard verification output is invalid", mode)
	}
	return nil
}

func (r *Runtime) ensureGatewayNetworkGuard(ctx context.Context) error {
	imageID, err := r.gatewayRuntimeImageID(ctx)
	if err != nil {
		return networkGuardFailure("Gateway network guard could not identify its reviewed image", err)
	}
	if err := r.runNetworkGuardHelper(ctx, imageID, gatewayContainer, "gateway"); err != nil {
		return networkGuardFailure("Gateway forwarding could not be closed and verified", err)
	}
	return nil
}

func (r *Runtime) projectNetworkSubnet(ctx context.Context, network string) (string, error) {
	output, err := r.runner.Output(
		ctx,
		[]string{"network", "inspect", "--format", "{{json .IPAM.Config}}", network},
		os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("inspect project network subnet: %w: %s", err, boundedDiagnostic(output))
	}
	var configurations []struct {
		Subnet string `json:"Subnet"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &configurations); err != nil {
		return "", fmt.Errorf("decode project network subnet: %w", err)
	}
	var subnet netip.Prefix
	for _, configuration := range configurations {
		candidate, parseErr := netip.ParsePrefix(configuration.Subnet)
		if parseErr != nil || !candidate.IsValid() || !candidate.Addr().Is4() || candidate != candidate.Masked() {
			continue
		}
		if subnet.IsValid() {
			return "", fmt.Errorf("project network has ambiguous IPv4 subnets")
		}
		subnet = candidate
	}
	if !subnet.IsValid() || subnet.Bits() < 8 || subnet.Bits() > 30 {
		return "", fmt.Errorf("project network has no usable IPv4 subnet")
	}
	return subnet.String(), nil
}

func validateProjectNetworkEndpoints(subnet, workspaceIP, gatewayIP string) error {
	prefix, err := netip.ParsePrefix(subnet)
	if err != nil || !prefix.IsValid() || !prefix.Addr().Is4() || prefix != prefix.Masked() {
		return fmt.Errorf("project network subnet is invalid")
	}
	workspace, workspaceErr := netip.ParseAddr(workspaceIP)
	gateway, gatewayErr := netip.ParseAddr(gatewayIP)
	if workspaceErr != nil || gatewayErr != nil || !workspace.Is4() || !gateway.Is4() ||
		!prefix.Contains(workspace) || !prefix.Contains(gateway) || workspace == gateway {
		return fmt.Errorf("project network endpoints are invalid")
	}
	return nil
}

func (r *Runtime) ensureWorkspaceNetworkGuard(
	ctx context.Context,
	project tobari.ProjectInstance,
	container, network, subnet, gatewayIP string,
) error {
	expectedContainer, expectedNetwork, err := tobari.ProjectResourceNames(project.ID)
	if err != nil {
		return err
	}
	if container != expectedContainer || network != expectedNetwork {
		return networkGuardFailure("Workspace network guard target is not owned by the selected Workspace", fmt.Errorf("project resource identity mismatch"))
	}
	if err := r.verifyOwnedProjectResource(ctx, "container", container, project.ID, projectWorkRole); err != nil {
		return networkGuardFailure("Workspace network guard target ownership could not be verified", err)
	}
	imageID, err := r.gatewayRuntimeImageID(ctx)
	if err != nil {
		return networkGuardFailure("Workspace network guard could not identify the reviewed helper image", err)
	}
	if err := r.runNetworkGuardHelper(ctx, imageID, container, "workspace", gatewayIP, subnet); err != nil {
		return networkGuardFailure("Workspace network route could not be closed and verified", err)
	}
	return nil
}

func completeNetworkGuardState() tobari.NetworkGuardState {
	return tobari.NetworkGuardState{
		Revision:              tobari.NetworkGuardRevision,
		WorkspaceOutputClosed: true, TransparentHTTPReady: true,
		SyntheticDNSReady: true, GatewayForwardingDisabled: true,
		GatewayForwardPolicyClosed: true,
	}
}
