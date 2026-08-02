package dockerruntime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const projectContainerHome = "/var/lib/tobari"

// projectContainerRoot returns the container path for one selected project
// root. A project below the host home keeps its home-relative path below the
// container HOME; all other projects retain the mirrored /workspace path.
func (r *Runtime) projectContainerRoot(root string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for project mount: %w", err)
	}
	home, err = canonicalPathWithMissing(home)
	if err != nil {
		return "", fmt.Errorf("resolve user home for project mount: %w", err)
	}
	return projectContainerRootForHostHome(root, home)
}

// projectContainerRootForHostHome is kept separate from Runtime so the
// home-relative mapping can be tested with a synthetic host home. Both paths
// are canonicalized before comparing them so a symlink cannot change the
// selected mount destination.
func projectContainerRootForHostHome(root, hostHome string) (string, error) {
	root, err := canonicalPathWithMissing(root)
	if err != nil {
		return "", fmt.Errorf("resolve project root for project mount: %w", err)
	}
	hostHome, err = canonicalPathWithMissing(hostHome)
	if err != nil {
		return "", fmt.Errorf("resolve host home for project mount: %w", err)
	}
	if root == hostHome || isPathAncestor(root, hostHome) {
		return "", fmt.Errorf("project root must not be the host home or one of its ancestors")
	}
	if !isPathAncestor(hostHome, root) {
		return tobari.ProjectWorkspaceRoot(root)
	}
	relative, err := filepath.Rel(hostHome, root)
	if err != nil {
		return "", fmt.Errorf("derive project path relative to host home: %w", err)
	}
	if relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("project root is not below the host home")
	}
	return filepath.ToSlash(filepath.Join(projectContainerHome, relative)), nil
}

func (r *Runtime) projectContainerCWD(root, cwd string) (string, error) {
	containerRoot, err := r.projectContainerRoot(root)
	if err != nil {
		return "", err
	}
	return mapProjectCWDToContainer(root, cwd, containerRoot)
}

func mapProjectCWDToContainer(root, cwd, containerRoot string) (string, error) {
	if !filepath.IsAbs(root) || !filepath.IsAbs(cwd) || !filepath.IsAbs(containerRoot) {
		return "", fmt.Errorf("project root, cwd, and container root must be absolute")
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(cwd))
	if err != nil {
		return "", fmt.Errorf("derive project cwd: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", fmt.Errorf("cwd %q is outside project root %q", cwd, root)
	}
	if relative == "." {
		return filepath.ToSlash(filepath.Clean(containerRoot)), nil
	}
	return filepath.ToSlash(filepath.Join(containerRoot, relative)), nil
}
