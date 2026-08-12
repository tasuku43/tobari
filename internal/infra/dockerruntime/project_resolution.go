package dockerruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
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

func isPathAncestor(ancestor, candidate string) bool {
	return ancestor == candidate || (ancestor != string(filepath.Separator) && strings.HasPrefix(candidate, ancestor+string(filepath.Separator))) || ancestor == string(filepath.Separator)
}

func canonicalPathWithMissing(path string) (string, error) {
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
				if !errors.Is(evalErr, os.ErrNotExist) {
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
		return r.resolveBuiltinImageSelector(explicit), nil
	}
	if _, err := os.Lstat(r.activeContextPath()); err == nil {
		name, activeErr := r.readActiveContext()
		if activeErr != nil {
			return "", activeErr
		}
		manifest, manifestErr := r.readContextManifestRaw(name)
		if manifestErr != nil {
			return "", manifestErr
		}
		return r.resolveBuiltinImageSelector(manifest.Image), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect active Context: %w", err)
	}
	return r.defaultRuntimeImage(), nil
}

func (r *Runtime) prepareContextImages(ctx context.Context) error {
	list, err := r.ListContexts(ctx)
	if err != nil {
		return err
	}
	for _, item := range list.Items {
		manifest, _, err := r.resolveContext(item.Name)
		if err != nil {
			return err
		}
		image := r.resolveBuiltinImageSelector(manifest.Image)
		if r.imageResolver().ShouldPullRuntimeImage(image) {
			if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
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
	image := r.resolveBuiltinImageSelector(manifest.Image)
	if r.imageResolver().ShouldPullRuntimeImage(image) {
		if err := r.pullOfficialRuntimeImage(ctx, image); err != nil {
			return err
		}
	}
	return r.validateCompatibleImage(ctx, image)
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

func (r *Runtime) resolveBuiltinImageSelector(image string) string {
	if image == tobari.BuiltinImageSelector {
		return r.defaultRuntimeImage()
	}
	return image
}

func (r *Runtime) validateCompatibleImage(ctx context.Context, image string) error {
	output, err := r.runner.Output(
		ctx,
		[]string{
			"image", "inspect", "--format",
			`{"api":{{json (index .Config.Labels "` + tobari.RuntimeImageAPILabel + `")}},"lifetime":{{json (index .Config.Labels "` + tobari.RuntimeImageLifetimeLabel + `")}},"user":{{json .Config.User}},"entrypoint":{{json .Config.Entrypoint}}}`,
			image,
		},
		os.Environ(),
	)
	if err != nil {
		return fault.Wrap(
			fault.KindUnavailable, "image_not_found",
			"selected Tobari image is not available locally; build or pull it explicitly", false, err,
			fault.NextAction{Command: "help tobari", Reason: "Read the compatible image contract."},
		)
	}
	var configuration struct {
		API        string   `json:"api"`
		Lifetime   string   `json:"lifetime"`
		User       string   `json:"user"`
		Entrypoint []string `json:"entrypoint"`
	}
	expectedEntrypoint := []string{"/usr/bin/tini", "--", "/usr/local/bin/tobari-entrypoint"}
	if err := json.Unmarshal(bytes.TrimSpace(output), &configuration); err != nil ||
		configuration.API != tobari.RuntimeImageAPI ||
		configuration.Lifetime != tobari.RuntimeImageLifetimeCommand ||
		configuration.User != "tobari" ||
		!equalStrings(configuration.Entrypoint, expectedEntrypoint) {
		return fault.New(
			fault.KindRejected, "incompatible_image",
			"selected image does not preserve the supported Tobari runtime API, lifetime command, user, and entrypoint", false,
			fault.NextAction{Command: "help tobari", Reason: "Extend the documented Tobari runtime base."},
		)
	}
	return nil
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
