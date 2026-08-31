package dockerruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

const maximumWorkspaceMountGuardContainers = 1024

type workspaceMountGuardObservation struct {
	ID        string   `json:"id"`
	Owner     string   `json:"owner"`
	Component string   `json:"component"`
	Workspace string   `json:"workspace"`
	Role      string   `json:"role"`
	Running   bool     `json:"running"`
	Env       []string `json:"env"`
	Mounts    []struct {
		Type        string `json:"type"`
		Source      string `json:"source"`
		Destination string `json:"destination"`
		RW          bool   `json:"rw"`
	} `json:"mounts"`
}

func workspaceMountGuardFault(code, message string, kind fault.Kind) error {
	return fault.WithClassification(fault.New(
		kind, code, message, false,
		fault.NextAction{Command: "workspace list", Reason: "Inspect exact Workspace roots before retrying entry."},
	), fault.PhasePrecondition, fault.ChangeNone)
}

func workspaceMountGuardUnverified(message string) error {
	return workspaceMountGuardFault("workspace_entry_overlap_unverified", message, fault.KindContract)
}

func workspaceMountGuardBlocked() error {
	return workspaceMountGuardFault(
		"workspace_entry_overlap_unsafe",
		"Workspace entry cannot start while a read-write ancestor Workspace is live.",
		fault.KindRejected,
	)
}

func workspaceMountCleanupFailed(cause error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindUnavailable,
		"workspace_entry_cleanup_failed",
		"Workspace entry stopped before start, but its unstarted container could not be removed.",
		false,
		cause,
		fault.NextAction{Command: "status", Reason: "Inspect the partial Workspace runtime before another entry attempt."},
	), fault.PhaseMutation, fault.ChangePartial)
}

// requireWorkspaceMountGuardBeforeMutation classifies the exact target using
// read-only Docker observations. A current running target needs no mount
// materialization, so only an absent, stopped, or stale target enters the
// installation-wide ancestor guard before its first Docker mutation.
func (r *Runtime) requireWorkspaceMountGuardBeforeMutation(
	ctx context.Context, targetRoot, container, workspaceID, expectedSpec string,
) error {
	if targetRoot == "" || container == "" || workspaceID == "" || expectedSpec == "" {
		return workspaceMountGuardUnverified("Workspace mount guard target authority is incomplete.")
	}
	exists, err := r.projectResourceExists(ctx, "container", container)
	if err != nil {
		return workspaceMountGuardUnverified("Workspace mount guard could not classify the target container.")
	}
	if !exists {
		return r.requireNoLiveWritableWorkspaceAncestor(ctx, targetRoot)
	}
	if err := r.verifyOwnedProjectResource(ctx, "container", container, workspaceID, projectWorkRole); err != nil {
		return workspaceMountGuardUnverified("Workspace mount guard could not verify the target container.")
	}
	observedSpec, err := r.projectContainerSpecHash(ctx, container)
	if err != nil {
		return workspaceMountGuardUnverified("Workspace mount guard could not verify the target specification.")
	}
	component, err := r.inspectContainer(ctx, projectWorkRole, container)
	if err != nil {
		return workspaceMountGuardUnverified("Workspace mount guard could not verify the target state.")
	}
	if observedSpec == expectedSpec && component.State == "running" {
		return nil
	}
	return r.requireNoLiveWritableWorkspaceAncestor(ctx, targetRoot)
}

// requireNoLiveWritableWorkspaceAncestor runs under the installation-wide
// project lock immediately before Docker starts a stopped or newly created
// work container. A live read-write strict ancestor can replace the target's
// directory entry while Docker resolves its bind source. Docker records the
// configured path rather than a host-selected inode, so path checks before or
// after start cannot close that exchange race.
func (r *Runtime) requireNoLiveWritableWorkspaceAncestor(ctx context.Context, targetRoot string) error {
	if !filepath.IsAbs(targetRoot) || filepath.Clean(targetRoot) != targetRoot || containsDockerMountDelimiter(targetRoot) {
		return workspaceMountGuardUnverified("Workspace mount guard received an invalid target root.")
	}
	listing, err := r.runner.Output(ctx, []string{
		"ps", "-a", "--no-trunc",
		"--filter", "label=" + ownerLabel + "=" + ownerValue,
		"--filter", "label=" + componentLabel + "=tobari",
		"--filter", "label=" + projectRoleLabel + "=" + projectWorkRole,
		"--format", "{{.ID}}",
	}, os.Environ())
	if err != nil {
		return workspaceMountGuardUnverified("Workspace mount guard could not enumerate owned work containers.")
	}
	if len(listing) > maximumWorkspaceMountGuardContainers*65 {
		return workspaceMountGuardUnverified("Workspace mount guard container evidence exceeded its bound.")
	}
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(listing)), "\n") {
		id := strings.TrimSpace(line)
		if id == "" {
			continue
		}
		if !containerIDPattern.MatchString(id) {
			return workspaceMountGuardUnverified("Workspace mount guard observed an invalid container identity.")
		}
		if _, duplicate := seen[id]; duplicate {
			return workspaceMountGuardUnverified("Workspace mount guard observed a duplicate container identity.")
		}
		seen[id] = struct{}{}
		if len(seen) > maximumWorkspaceMountGuardContainers {
			return workspaceMountGuardUnverified("Workspace mount guard container count exceeded its bound.")
		}
		observation, observeErr := r.observeWorkspaceMountGuardContainer(ctx, id)
		if observeErr != nil {
			return observeErr
		}
		source, writable, sourceErr := observation.projectSource()
		if sourceErr != nil {
			return workspaceMountGuardUnverified("Workspace mount guard observed contradictory work-container mounts.")
		}
		if !observation.Running {
			continue
		}
		if writable && source != targetRoot && isPathAncestor(source, targetRoot) {
			return workspaceMountGuardBlocked()
		}
	}
	return nil
}

func (r *Runtime) observeWorkspaceMountGuardContainer(ctx context.Context, id string) (workspaceMountGuardObservation, error) {
	format := `{"id":{{json .Id}},"owner":{{json (index .Config.Labels "` + ownerLabel + `")}},` +
		`"component":{{json (index .Config.Labels "` + componentLabel + `")}},` +
		`"workspace":{{json (index .Config.Labels "` + projectIDLabel + `")}},` +
		`"role":{{json (index .Config.Labels "` + projectRoleLabel + `")}},` +
		`"running":{{json .State.Running}},"env":{{json .Config.Env}},"mounts":[` +
		`{{range $index, $mount := .Mounts}}{{if $index}},{{end}}` +
		`{"type":{{json $mount.Type}},"source":{{json $mount.Source}},` +
		`"destination":{{json $mount.Destination}},"rw":{{json $mount.RW}}}{{end}}]}`
	output, err := r.runner.Output(ctx, []string{"inspect", "--format", format, id}, os.Environ())
	if err != nil {
		return workspaceMountGuardObservation{}, workspaceMountGuardUnverified("Workspace mount guard could not inspect an owned work container.")
	}
	var observation workspaceMountGuardObservation
	if decodeStrictJSON(output, &observation) != nil || observation.ID != id || observation.Owner != ownerValue ||
		observation.Component != "tobari" || observation.Role != projectWorkRole || observation.Workspace == "" {
		return workspaceMountGuardObservation{}, workspaceMountGuardUnverified("Workspace mount guard observed foreign or malformed work-container authority.")
	}
	return observation, nil
}

func (o workspaceMountGuardObservation) projectSource() (string, bool, error) {
	var workspaceRoot string
	for _, value := range o.Env {
		if !strings.HasPrefix(value, "TOBARI_ROOT=") {
			continue
		}
		if workspaceRoot != "" {
			return "", false, fmt.Errorf("duplicate Workspace root environment")
		}
		workspaceRoot = strings.TrimPrefix(value, "TOBARI_ROOT=")
	}
	if !filepath.IsAbs(workspaceRoot) || filepath.Clean(workspaceRoot) != workspaceRoot {
		return "", false, fmt.Errorf("invalid Workspace root environment")
	}
	var source string
	var writable bool
	for _, mount := range o.Mounts {
		if mount.Destination != workspaceRoot {
			continue
		}
		if source != "" || mount.Type != "bind" || !filepath.IsAbs(mount.Source) ||
			filepath.Clean(mount.Source) != mount.Source || containsDockerMountDelimiter(mount.Source) {
			return "", false, fmt.Errorf("invalid project source mount")
		}
		source, writable = mount.Source, mount.RW
	}
	if source == "" {
		return "", false, fmt.Errorf("missing project source mount")
	}
	return source, writable, nil
}
