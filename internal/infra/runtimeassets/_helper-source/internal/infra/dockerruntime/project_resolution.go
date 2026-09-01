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
	"time"
	"unicode"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

// ResolveProjectRoot resolves a project root and rejects host-management paths
// that must never be read-write mounted into an untrusted Tobari.
func (r *Runtime) ResolveProjectRoot(ctx context.Context, value string) (string, error) {
	resolved, err := r.resolveCanonicalRoot(ctx, value)
	if err != nil {
		return "", err
	}
	if err := r.validateProjectRoot(resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func (r *Runtime) resolveCanonicalRoot(ctx context.Context, value string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("make root absolute: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve root symlinks: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory")
	}
	return filepath.Clean(resolved), nil
}

func (r *Runtime) validateProjectRoot(root string) error {
	if containsDockerMountDelimiter(root) {
		return fmt.Errorf("project root cannot be encoded as an exact Docker bind source")
	}
	if root == string(filepath.Separator) {
		return fmt.Errorf("filesystem root cannot be a Tobari project root")
	}
	homeDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve user home for project-root protection: %w", err)
	}
	home, err := canonicalPathWithMissing(homeDirectory)
	if err != nil {
		return fmt.Errorf("resolve user home for project-root protection: %w", err)
	}
	if root == home || isPathAncestor(root, home) {
		return fmt.Errorf("user home or its ancestor cannot be a Tobari project root")
	}
	protectedPaths := map[string]string{
		"configuration":     r.configDirectory,
		"state":             r.stateDirectory,
		"data":              r.dataDirectory,
		"docker config":     filepath.Join(home, ".docker"),
		"docker socket":     filepath.Join(string(filepath.Separator), "var", "run", "docker.sock"),
		"docker run socket": filepath.Join(string(filepath.Separator), "run", "docker.sock"),
		"docker data":       filepath.Join(string(filepath.Separator), "var", "lib", "docker"),
		"docker runtime":    filepath.Join(string(filepath.Separator), "var", "run", "docker"),
	}
	for name, candidate := range protectedPaths {
		protected, pathErr := canonicalPathWithMissing(candidate)
		if pathErr != nil {
			return fmt.Errorf("resolve protected %s path: %w", name, pathErr)
		}
		for _, protectedPath := range []string{protected, filepath.Clean(candidate)} {
			if isPathAncestor(root, protectedPath) || isPathAncestor(protectedPath, root) {
				return fmt.Errorf("project root overlaps protected %s path", name)
			}
		}
	}
	return nil
}

// validateExactProjectBindSource closes the path-resolution boundary again at
// the Docker effect. A durable Workspace root is canonical authority, but an
// overlapping read-write Workspace can still replace a nested directory after
// planning. Docker's --mount syntax is comma-delimited, so accepting a comma
// would also let one valid host path be parsed as another source plus options.
func (r *Runtime) validateExactProjectBindSource(ctx context.Context, expected string) error {
	if containsDockerMountDelimiter(expected) {
		return fmt.Errorf("project bind source contains unsupported Docker mount syntax")
	}
	resolved, err := r.ResolveProjectRoot(ctx, expected)
	if err != nil {
		return fmt.Errorf("re-resolve project bind source: %w", err)
	}
	if resolved != expected {
		return fmt.Errorf("project bind source changed from %q to %q", expected, resolved)
	}
	return nil
}

func containsDockerMountDelimiter(value string) bool {
	return strings.ContainsRune(value, ',') || strings.IndexFunc(value, unicode.IsControl) >= 0
}

func isPathAncestor(ancestor, candidate string) bool {
	return ancestor == candidate || (ancestor != string(filepath.Separator) && strings.HasPrefix(candidate, ancestor+string(filepath.Separator))) || ancestor == string(filepath.Separator)
}

func canonicalPathWithMissing(path string) (string, error) {
	return canonicalPathWithMissingMode(path, true)
}

func canonicalPathWithMissingStrict(path string) (string, error) {
	return canonicalPathWithMissingMode(path, false)
}

func canonicalPathWithMissingMode(path string, preserveDanglingSymlink bool) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	missing := make([]string, 0)
	for {
		if _, statErr := os.Lstat(current); statErr == nil {
			resolved, evalErr := filepath.EvalSymlinks(current)
			if evalErr != nil {
				if !preserveDanglingSymlink || !errors.Is(evalErr, os.ErrNotExist) {
					return "", evalErr
				}
				// A dangling management symlink is still a protected lexical
				// path. Preserve it rather than treating the missing target as
				// a discovery failure.
				resolved = current
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no existing ancestor for %q", path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

// ResolveImageSelector applies explicit CLI input before the XDG default.
func (r *Runtime) ResolveImageSelector(ctx context.Context, explicit string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if explicit != "" {
		return r.resolveBuiltinImageSelector(explicit)
	}
	if _, err := os.Lstat(r.activeContextPath()); err == nil {
		name, activeErr := r.readDefaultManifestName()
		if activeErr != nil {
			return "", activeErr
		}
		manifest, manifestErr := r.readContextManifestRaw(name)
		if manifestErr != nil {
			return "", manifestErr
		}
		return r.resolveContextImageFor(ctx, manifest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect active Context: %w", err)
	}
	return r.defaultRuntimeImage()
}

func (r *Runtime) prepareContextImages(ctx context.Context) error {
	baseImage, err := r.defaultRuntimeImage()
	if err != nil {
		return err
	}
	if r.imageResolver().ShouldPullRuntimeImage(baseImage) {
		if err := r.pullOfficialRuntimeImage(ctx, baseImage); err != nil {
			return err
		}
	}
	if r.imageResolver().ShouldBuildRuntimeImage(baseImage) {
		if err := r.ensureLocalBaseRuntimeImage(ctx, baseImage); err != nil {
			return err
		}
	}
	if err := r.validateCompatibleImage(ctx, baseImage); err != nil {
		return fmt.Errorf("Tobari base runtime image: %w", err)
	}
	if err := r.materializeWorkspaceHelpers(ctx, baseImage); err != nil {
		return err
	}
	list, err := r.ListContexts(ctx)
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		manifest, _, err := r.resolveContext(item.Name)
		if err != nil {
			return err
		}
		image, err := r.resolveContextImageFor(ctx, manifest)
		if err != nil {
			return err
		}
		if r.imageResolver().ShouldPullRuntimeImage(image) {
			if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
				return err
			}
		}
		if r.imageResolver().ShouldBuildRuntimeImage(image) {
			if err := r.ensureLocalBaseRuntimeImage(ctx, image); err != nil {
				return err
			}
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return fmt.Errorf("Context %q runtime image: %w", manifest.Name, err)
		}
	}
	return nil
}

// prepareActiveContextImage preserves the focused single-Context image check;
// shared cluster reconciliation calls prepareContextImages instead.
func (r *Runtime) prepareActiveContextImage(ctx context.Context) error {
	manifest, _, err := r.activeContext()
	if err != nil {
		return err
	}
	image, err := r.resolveContextImageFor(ctx, manifest)
	if err != nil {
		return err
	}
	if r.imageResolver().ShouldPullRuntimeImage(image) {
		if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
			return err
		}
	}
	if r.imageResolver().ShouldBuildRuntimeImage(image) {
		if err := r.ensureLocalBaseRuntimeImage(ctx, image); err != nil {
			return err
		}
	}
	return r.validateCompatibleImage(ctx, image)
}

func (r *Runtime) ensureLocalBaseRuntimeImage(ctx context.Context, image string) error {
	return r.ensureLocalBaseRuntimeImageWithDiagnostics(ctx, image, nil)
}

func (r *Runtime) ensureLocalBaseRuntimeImageWithDiagnostics(ctx context.Context, image string, diagnostics io.Writer) error {
	if _, err := r.inspectRuntimeImageID(ctx, image); err == nil {
		return nil
	} else if !errors.Is(err, errRuntimeImageMissing) {
		return err
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return err
	}
	helperSourceDirectory := filepath.Join(runtimeDirectory, "helper-source")
	if err := runtimeassets.MaterializeExposureHelperSource(helperSourceDirectory); err != nil {
		return err
	}
	versions, err := runtimeassets.Versions()
	if err != nil {
		return err
	}
	helperSourceVersion, err := runtimeassets.ExposureHelperSourceVersion()
	if err != nil {
		return err
	}
	uid, gid := currentIDs()
	args := []string{
		"buildx", "build", "--progress=plain", "--load",
		"--tag", image,
		"--file", filepath.Join(runtimeDirectory, "tobari", "Dockerfile"),
		"--build-arg", "GO_BUILDER_IMAGE=" + versions["GO_BUILDER_IMAGE"],
		"--build-arg", "TOBARI_EXPOSURE_HELPER_SOURCE=" + helperSourceVersion,
		"--build-arg", fmt.Sprintf("TOBARI_UID=%d", uid),
		"--build-arg", fmt.Sprintf("TOBARI_GID=%d", gid),
		"--build-context", "helper-source=" + helperSourceDirectory,
		filepath.Join(runtimeDirectory, "tobari"),
	}
	var tail runtimeBuildDiagnosticTail
	stream := io.MultiWriter(&bestEffortDiagnosticWriter{writer: diagnostics}, &tail)
	if err := r.runner.Run(ctx, args, os.Environ(), nil, stream, stream); err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "runtime_image_build_failed",
			"the pinned agent-ready base could not be built locally", false,
			fmt.Errorf("build canonical standard Runtime: %w: %s", err, boundedDiagnostic(tail.Bytes())),
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker build support and network access for pinned agent downloads."},
		)
	}
	return nil
}

func (r *Runtime) pullOfficialRuntimeImage(ctx context.Context, image string) error {
	var output bytes.Buffer
	err := r.runner.Run(
		ctx,
		[]string{"image", "pull", image},
		os.Environ(), nil, &output, &output,
	)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "runtime_image_unavailable",
			"official Tobari runtime image is not available; inspect Docker registry access before startup", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker registry access and the selected Context image."},
		)
	}
	return nil
}

func (r *Runtime) resolveBuiltinImageSelector(image string) (string, error) {
	if image == tobari.BuiltinImageSelector {
		return r.defaultRuntimeImage()
	}
	return image, nil
}

func (r *Runtime) validateCompatibleImage(ctx context.Context, image string) error {
	_, err := r.resolveCompatibleImageID(ctx, image)
	return err
}

var errRuntimeImageCompatibilityObservationBound = errors.New("Runtime image compatibility evidence exceeds the observation bound")
var errRuntimeImageMissing = errors.New("Runtime image is missing")

const runtimeImageInspectTimeout = 15 * time.Second

type runtimeImageCompatibilityEvidence struct {
	ID         string                     `json:"id"`
	API        string                     `json:"api"`
	Lifetime   string                     `json:"lifetime"`
	User       string                     `json:"user"`
	Entrypoint []string                   `json:"entrypoint"`
	Volumes    map[string]json.RawMessage `json:"volumes"`
}

func (r *Runtime) inspectRuntimeImageCompatibility(ctx context.Context, image string) (runtimeImageCompatibilityEvidence, error) {
	if tobari.ValidateImageSelector(image) != nil {
		return runtimeImageCompatibilityEvidence{}, fmt.Errorf("Runtime image compatibility selector is invalid")
	}
	format := `{"id":{{json .Id}},` +
		`"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},` +
		`"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},` +
		`"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}},"volumes":{{json .Config.Volumes}}}`
	stdout := &boundedBuffer{limit: 4096}
	stderr := &boundedBuffer{limit: 4096}
	inspectContext, cancel := context.WithTimeout(ctx, runtimeImageInspectTimeout)
	defer cancel()
	err := r.runner.Run(inspectContext, []string{"image", "inspect", "--format", format, image}, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return runtimeImageCompatibilityEvidence{}, errRuntimeImageCompatibilityObservationBound
	}
	if err != nil {
		if isMissingRuntimeImageInspect(err, stderr.buffer.Bytes(), image) || isMissingRuntimeImageInspect(err, stdout.buffer.Bytes(), image) {
			return runtimeImageCompatibilityEvidence{}, errRuntimeImageMissing
		}
		if ctx.Err() != nil {
			return runtimeImageCompatibilityEvidence{}, ctx.Err()
		}
		return runtimeImageCompatibilityEvidence{}, fmt.Errorf("inspect Runtime image compatibility: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	var evidence runtimeImageCompatibilityEvidence
	if decodeStrictJSON(bytes.TrimSpace(stdout.buffer.Bytes()), &evidence) != nil {
		return runtimeImageCompatibilityEvidence{}, fmt.Errorf("Runtime image compatibility evidence is malformed")
	}
	return evidence, nil
}

func (r *Runtime) inspectRuntimeImageID(ctx context.Context, image string) (string, error) {
	if tobari.ValidateImageSelector(image) != nil {
		return "", fmt.Errorf("Runtime image selector is invalid")
	}
	stdout := &boundedBuffer{limit: 4096}
	stderr := &boundedBuffer{limit: 4096}
	inspectContext, cancel := context.WithTimeout(ctx, runtimeImageInspectTimeout)
	defer cancel()
	err := r.runner.Run(inspectContext, []string{"image", "inspect", "--format", "{{.Id}}", image}, os.Environ(), nil, stdout, stderr)
	if stdout.overflow || stderr.overflow {
		return "", fmt.Errorf("Runtime image identity exceeds the observation bound")
	}
	if err != nil {
		if isMissingRuntimeImageInspect(err, stderr.buffer.Bytes(), image) || isMissingRuntimeImageInspect(err, stdout.buffer.Bytes(), image) {
			return "", errRuntimeImageMissing
		}
		return "", fmt.Errorf("inspect Runtime image identity: %w: %s", err, boundedDiagnostic(stderr.buffer.Bytes()))
	}
	id := strings.TrimSpace(stdout.buffer.String())
	if tobari.ValidateDigest(id) != nil {
		return "", fmt.Errorf("Runtime image identity is invalid")
	}
	return id, nil
}

func validRuntimeImageCompatibility(evidence runtimeImageCompatibilityEvidence) bool {
	expectedEntrypoint := []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}
	return tobari.ValidateDigest(evidence.ID) == nil &&
		evidence.API == tobari.RuntimeImageAPI &&
		evidence.Lifetime == tobari.RuntimeImageLifetimeCommand &&
		evidence.User == "tobari" &&
		equalStrings(evidence.Entrypoint, expectedEntrypoint) && len(evidence.Volumes) == 0
}

func (r *Runtime) resolveCompatibleImageID(ctx context.Context, image string) (string, error) {
	evidence, err := r.inspectRuntimeImageCompatibility(ctx, image)
	if errors.Is(err, errRuntimeImageMissing) {
		return "", fault.Wrap(
			fault.KindUnavailable, "image_not_found",
			"selected Tobari image is not available locally; build or pull it explicitly", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Read the compatible image contract."},
		)
	}
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil && !errors.Is(err, errRuntimeImageCompatibilityObservationBound) {
		if public, ok := fault.PublicCopy(err); ok {
			return "", public
		}
		return "", fault.Wrap(
			fault.KindUnavailable, "runtime_image_unavailable",
			"the selected Runtime image could not be inspected safely", true, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect Docker Engine and local image-store readiness."},
		)
	}
	if err != nil || !validRuntimeImageCompatibility(evidence) {
		return "", fault.New(
			fault.KindRejected, "incompatible_image",
			"selected image does not preserve the supported Tobari runtime API, lifetime command, user, entrypoint, and volume-free filesystem contract", false,
			fault.NextAction{Command: "help tobari", Reason: "Extend the documented Tobari runtime base."},
		)
	}
	return evidence.ID, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
