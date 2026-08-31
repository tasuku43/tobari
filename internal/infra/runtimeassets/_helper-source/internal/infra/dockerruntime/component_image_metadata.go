package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
)

var errComponentImageMissing = errors.New("component image is missing")

type componentImageObservationError struct{ cause error }

func (e componentImageObservationError) Error() string { return e.cause.Error() }
func (e componentImageObservationError) Unwrap() error { return e.cause }

const (
	componentImageInspectLimit   = 64 * 1024
	componentImageInspectTimeout = 15 * time.Second
)

// componentImageMetadata is the complete Docker-owned image authority used by
// shared components and extraction helpers. In particular, Config.Volumes is
// inspected before any container create so an image cannot silently allocate
// an anonymous writable volume outside the reviewed Compose mount closure.
type componentImageMetadata struct {
	ID           string               `json:"Id"`
	RepoDigests  []string             `json:"RepoDigests"`
	Architecture string               `json:"Architecture"`
	OS           string               `json:"Os"`
	Config       componentImageConfig `json:"Config"`
}

type componentImageConfig struct {
	User       string                     `json:"User"`
	Labels     map[string]string          `json:"Labels"`
	Entrypoint []string                   `json:"Entrypoint"`
	Volumes    map[string]json.RawMessage `json:"Volumes"`
}

func (r *Runtime) inspectBoundedComponentImage(ctx context.Context, image string) (componentImageMetadata, error) {
	inspectContext, cancel := context.WithTimeout(ctx, componentImageInspectTimeout)
	defer cancel()
	stdout := &boundedBuffer{limit: componentImageInspectLimit / 2}
	stderr := &boundedBuffer{limit: componentImageInspectLimit / 2}
	err := r.runner.Run(
		inspectContext, []string{"image", "inspect", "--format", componentImageInspectFormat, image},
		os.Environ(), nil, stdout, stderr,
	)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return componentImageMetadata{}, ctxErr
	}
	if stdout.overflow || stderr.overflow {
		return componentImageMetadata{}, fmt.Errorf("component image metadata exceeds %d bytes", componentImageInspectLimit)
	}
	if err != nil {
		if isMissingRuntimeImageInspect(err, stderr.buffer.Bytes(), image) || isMissingRuntimeImageInspect(err, stdout.buffer.Bytes(), image) {
			return componentImageMetadata{}, fmt.Errorf("%w: %s", errComponentImageMissing, image)
		}
		return componentImageMetadata{}, componentImageObservationError{cause: fmt.Errorf("inspect component image metadata: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))}
	}
	if len(bytes.TrimSpace(stderr.buffer.Bytes())) != 0 {
		return componentImageMetadata{}, fmt.Errorf("component image inspect emitted diagnostic output")
	}
	var metadata componentImageMetadata
	if err := decodeStrictJSON(bytes.TrimSpace(stdout.buffer.Bytes()), &metadata); err != nil {
		return componentImageMetadata{}, fmt.Errorf("decode component image metadata: %w", err)
	}
	return metadata, nil
}

func (r *Runtime) pullComponentImage(ctx context.Context, image string) error {
	output := &boundedBuffer{limit: componentImageInspectLimit}
	if err := r.runner.Run(ctx, []string{"pull", image}, os.Environ(), nil, output, output); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("pull component image: %w: %s", err, boundedDiagnostic(output.buffer.Bytes()))
	}
	if output.overflow {
		return fmt.Errorf("component image pull diagnostic exceeds %d bytes", componentImageInspectLimit)
	}
	return nil
}

const componentImageInspectFormat = `{"Id":{{json .Id}},"RepoDigests":{{json .RepoDigests}},` +
	`"Architecture":{{json .Architecture}},"Os":{{json .Os}},"Config":{` +
	`"User":{{json .Config.User}},"Labels":{{json .Config.Labels}},` +
	`"Entrypoint":{{json .Config.Entrypoint}},"Volumes":{{json .Config.Volumes}}}}`

func validateComponentImageVolumes(volumes map[string]json.RawMessage, allowed ...string) error {
	allow := make(map[string]struct{}, len(allowed))
	for _, destination := range allowed {
		allow[destination] = struct{}{}
	}
	for destination := range volumes {
		if _, ok := allow[destination]; !ok {
			return fmt.Errorf("component image declares unreviewed volume %q", destination)
		}
	}
	return nil
}

func (r *Runtime) resolveVolumeSafeOPAImageID(ctx context.Context, image string) (string, error) {
	metadata, err := r.inspectBoundedComponentImage(ctx, image)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		var observationError componentImageObservationError
		if errors.Is(err, errComponentImageMissing) || errors.As(err, &observationError) {
			return "", fault.WithClassification(fault.Wrap(
				fault.KindUnavailable, "cluster_component_image_unavailable",
				"The pinned OPA image could not be observed safely before shared-cluster startup.", true, err,
				fault.NextAction{Command: "doctor", Reason: "Inspect Docker and pinned OPA image availability before retrying."},
			), fault.PhasePrecondition, fault.ChangeNone)
		}
		return "", incompatibleOPAImage(err)
	}
	if imageIDPattern.MatchString(metadata.ID) && validateComponentImageVolumes(metadata.Config.Volumes) == nil {
		return metadata.ID, nil
	}
	cause := validateComponentImageVolumes(metadata.Config.Volumes)
	if cause == nil {
		cause = fmt.Errorf("OPA image identity is invalid")
	}
	return "", incompatibleOPAImage(cause)
}

func incompatibleOPAImage(cause error) error {
	return fault.WithClassification(fault.Wrap(
		fault.KindRejected, "cluster_resource_conflict",
		"The selected OPA image does not satisfy Tobari's volume-safe shared-component contract.", false, cause,
		fault.NextAction{Command: "doctor", Reason: "Inspect the pinned OPA image identity and declared volume metadata before retrying."},
	), fault.PhasePrecondition, fault.ChangeNone)
}
