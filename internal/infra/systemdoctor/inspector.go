// Package systemdoctor provides the degraded doctor adapter used when the XDG
// Docker runtime cannot be constructed.
package systemdoctor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tasuku43/tobari/internal/domain/doctor"
)

const (
	maxDockerObservationBytes    = 4096
	maxDockerObservationDuration = 5 * time.Second
)

type boundedOutput struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	remaining := maxDockerObservationBytes + 1 - w.buffer.Len()
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		_, _ = w.buffer.Write(data[:remaining])
	}
	if w.buffer.Len() > maxDockerObservationBytes || len(data) > remaining {
		w.exceeded = true
	}
	return len(data), nil
}

// Inspector retains the runtime-construction failure while continuing checks
// that do not require trusted XDG authority.
type Inspector struct {
	runtimeError error
}

// New creates the fallback observer. Production supplies the runtime
// construction error; omission is retained for narrow tests.
func New(runtimeError ...error) *Inspector {
	inspector := &Inspector{}
	if len(runtimeError) != 0 {
		inspector.runtimeError = runtimeError[0]
	}
	return inspector
}

// ObserveDoctorCheck implements doctorcmd.InspectorPort by structural typing.
func (i *Inspector) ObserveDoctorCheck(
	ctx context.Context, root string, id doctor.CheckID,
) (doctor.Observation, error) {
	if ctx == nil {
		return doctor.Observation{}, fmt.Errorf("inspection context is nil")
	}
	if err := ctx.Err(); err != nil {
		return doctor.Observation{}, err
	}
	switch id {
	case doctor.CheckIDDockerCLI:
		if _, err := exec.LookPath("docker"); err != nil {
			return observation(doctor.CheckStatusFail, "docker was not found on PATH"), nil
		}
		return observation(doctor.CheckStatusPass, "docker is available"), nil
	case doctor.CheckIDDockerEngine:
		return i.observeDocker(ctx, []string{"version", "--format", "{{.Server.Version}}"}, "Docker Engine is unavailable")
	case doctor.CheckIDDockerContext:
		return i.observeDocker(ctx, []string{"context", "show"}, "Docker context could not be read")
	case doctor.CheckIDDockerCompose:
		return i.observeDocker(ctx, []string{"compose", "version", "--short"}, "Docker Compose v2 is unavailable")
	case doctor.CheckIDProxyPort:
		return observation(doctor.CheckStatusPass, "Gateway has no host-published port"), nil
	case doctor.CheckIDRoot:
		return i.observeRoot(root), nil
	case doctor.CheckIDRootSharing:
		return observation(doctor.CheckStatusWarn, "path is valid; Docker bind sharing is checked when a Workspace is created"), nil
	case doctor.CheckIDContext:
		return observation(doctor.CheckStatusFail, "Tobari XDG runtime paths could not be resolved safely"), nil
	case doctor.CheckIDOwnedResources:
		result, err := i.observeDocker(
			ctx, []string{"ps", "-a", "--filter", "label=io.tobari.owner=default", "--format", "{{.Names}}"},
			"owned Docker resources could not be listed",
		)
		if err == nil && result.Status == doctor.CheckStatusPass && strings.TrimSpace(result.Detail) == "" {
			result.Detail = "no residual containers"
		}
		return result, err
	default:
		return doctor.Observation{}, fmt.Errorf("fallback observer cannot inspect %q without trusted XDG runtime paths", id)
	}
}

func (i *Inspector) observeDocker(
	ctx context.Context, arguments []string, failure string,
) (doctor.Observation, error) {
	path, err := exec.LookPath("docker")
	if err != nil {
		return observation(doctor.CheckStatusFail, failure), nil
	}
	observationContext, cancel := context.WithTimeout(ctx, maxDockerObservationDuration)
	defer cancel()
	command := exec.CommandContext(observationContext, path, arguments...) // #nosec G204 -- LookPath resolves Docker and callers supply only the closed diagnostic argv set.
	command.Env = os.Environ()
	output := &boundedOutput{}
	command.Stdout = output
	command.Stderr = io.Discard
	err = command.Run()
	if contextErr := ctx.Err(); contextErr != nil {
		return doctor.Observation{}, contextErr
	}
	if err != nil || output.exceeded {
		return observation(doctor.CheckStatusFail, failure), nil
	}
	return observation(doctor.CheckStatusPass, strings.TrimSpace(output.buffer.String())), nil
}

func (i *Inspector) observeRoot(root string) doctor.Observation {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return observation(doctor.CheckStatusFail, "the prospective project root could not be resolved")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return observation(doctor.CheckStatusFail, "the prospective project root could not be resolved")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return observation(doctor.CheckStatusFail, "the prospective project root is not an existing directory")
	}
	home, homeErr := os.UserHomeDir()
	return classifyResolvedRoot(resolved, home, homeErr, i == nil || i.runtimeError != nil)
}

func classifyResolvedRoot(resolved, home string, homeErr error, runtimeUnavailable bool) doctor.Observation {
	if resolved == string(filepath.Separator) {
		return observation(doctor.CheckStatusFail, "the prospective project root is not safe for Tobari")
	}
	if homeErr == nil && pathContains(resolved, home) {
		return observation(doctor.CheckStatusFail, "the prospective project root is not safe for Tobari")
	}
	if homeErr != nil || runtimeUnavailable {
		return observation(doctor.CheckStatusWarn, "the root exists; XDG-specific path protection could not be validated")
	}
	return observation(doctor.CheckStatusPass, filepath.Clean(resolved))
}

func pathContains(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	return root == candidate || strings.HasPrefix(candidate, root+string(filepath.Separator))
}

func observation(status doctor.CheckStatus, detail string) doctor.Observation {
	return doctor.Observation{Status: status, Detail: detail}
}
