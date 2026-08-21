package dockerruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	projectGitContainerDirectory = "/run/tobari/git"
	projectGitContainerConfig    = projectGitContainerDirectory + "/config"
	maxProjectGitConfigBytes     = 32 * 1024
)

func (r *Runtime) projectGitDirectory(id string) (string, error) {
	directory, err := r.projectDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "git"), nil
}

func (r *Runtime) projectGitConfigPath(id string) (string, error) {
	directory, err := r.projectGitDirectory(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "config"), nil
}

func (r *Runtime) reconcileProjectGitIdentity(
	ctx context.Context,
	manifest tobari.ContextManifest,
	instance tobari.ProjectInstance,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := manifest.Validate(); err != nil {
		return err
	}
	if err := instance.Validate(); err != nil {
		return err
	}
	identity, err := r.resolveProjectGitIdentity(ctx, manifest, instance.Root)
	if err != nil {
		return err
	}
	config, err := encodeProjectGitConfig(identity)
	if err != nil {
		return fmt.Errorf("encode Workspace Git identity projection: %w", err)
	}
	if err := r.writeProjectGitConfig(instance.ID, config); err != nil {
		return fmt.Errorf("write Workspace Git identity projection: %w", err)
	}
	return nil
}

func (r *Runtime) resolveProjectGitIdentity(
	ctx context.Context,
	manifest tobari.ContextManifest,
	root string,
) (*projectGitIdentity, error) {
	setting := manifest.GitIdentity
	if setting == nil || setting.Source == tobari.ContextGitIdentityDefault {
		return nil, nil
	}
	switch setting.Source {
	case tobari.ContextGitIdentityInherit:
		if r.gitIdentity == nil {
			return nil, gitIdentityResolutionFailed()
		}
		identity, err := r.gitIdentity.Resolve(ctx, root)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			// Resolver and process errors intentionally collapse to one stable,
			// secret-free public fault. In particular, never attach raw Git
			// diagnostics or the personal identity values to this boundary.
			return nil, gitIdentityResolutionFailed()
		}
		if identity == nil {
			return nil, nil
		}
		if err := validateProjectGitIdentity(identity); err != nil {
			return nil, gitIdentityResolutionFailed()
		}
		return identity, nil
	case tobari.ContextGitIdentityLiteral:
		if setting.Name == nil || setting.Email == nil {
			return nil, fmt.Errorf("literal Git identity is incomplete")
		}
		identity := &projectGitIdentity{Name: *setting.Name, Email: *setting.Email}
		if err := validateProjectGitIdentity(identity); err != nil {
			return nil, err
		}
		return identity, nil
	default:
		return nil, fmt.Errorf("persisted Git identity source is invalid")
	}
}

func validateProjectGitIdentity(identity *projectGitIdentity) error {
	if identity == nil {
		return nil
	}
	if err := validateProjectGitIdentityValue(identity.Name); err != nil {
		return fmt.Errorf("Git identity name: %w", err)
	}
	if err := validateProjectGitIdentityValue(identity.Email); err != nil {
		return fmt.Errorf("Git identity email: %w", err)
	}
	return nil
}

func encodeProjectGitConfig(identity *projectGitIdentity) ([]byte, error) {
	if err := validateProjectGitIdentity(identity); err != nil {
		return nil, err
	}
	var config strings.Builder
	config.WriteString("[include]\n\tpath = \"/etc/gitconfig\"\n")
	if identity != nil {
		config.WriteString("[user]\n\tname = \"")
		config.WriteString(quoteProjectGitConfigValue(identity.Name))
		config.WriteString("\"\n\temail = \"")
		config.WriteString(quoteProjectGitConfigValue(identity.Email))
		config.WriteString("\"\n")
	}
	if config.Len() > maxProjectGitConfigBytes {
		return nil, fmt.Errorf("generated Git identity projection is oversized")
	}
	return []byte(config.String()), nil
}

func quoteProjectGitConfigValue(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "\"", "\\\"")
}

func (r *Runtime) writeProjectGitConfig(id string, data []byte) error {
	if len(data) == 0 || len(data) > maxProjectGitConfigBytes {
		return fmt.Errorf("Git identity projection content is empty or oversized")
	}
	projectDirectory, err := r.projectDirectory(id)
	if err != nil {
		return err
	}
	if err := requirePrivateDirectory(projectDirectory); err != nil {
		return fmt.Errorf("project instance directory is unsafe: %w", err)
	}
	directory, err := r.projectGitDirectory(id)
	if err != nil {
		return err
	}
	if err := ensureNewOrPrivateDirectory(directory); err != nil {
		return fmt.Errorf("Git identity projection directory is unsafe: %w", err)
	}
	if err := syncDirectory(projectDirectory); err != nil {
		return fmt.Errorf("sync Git identity projection parent: %w", err)
	}
	path, err := r.projectGitConfigPath(id)
	if err != nil {
		return err
	}
	existing, exists, err := inspectProjectGitConfig(path)
	if err != nil {
		return err
	}
	if exists && existing.Mode().Perm() == 0o600 {
		current, readErr := os.ReadFile(path) // #nosec G304 -- exact regular file under a validated private project directory.
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(current, data) {
			return nil
		}
	}
	temporary, err := os.CreateTemp(directory, ".tobari-git-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, _, err := inspectProjectGitConfig(path); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	info, exists, err := inspectProjectGitConfig(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("Git identity projection file disappeared after replacement")
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("Git identity projection file is not owner-only")
	}
	directoryFile, err := os.Open(directory) // #nosec G304 -- exact validated runtime-owned directory.
	if err != nil {
		return err
	}
	defer directoryFile.Close()
	return directoryFile.Sync()
}

func ensureNewOrPrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(path, 0o700); err != nil {
			return err
		}
		return requirePrivateDirectory(path)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o700 {
		return fmt.Errorf("path is not an owner-only regular directory")
	}
	return nil
}

func inspectProjectGitConfig(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, true, fmt.Errorf("Git identity projection file is unsafe")
	}
	if info.Size() > maxProjectGitConfigBytes {
		return nil, true, fmt.Errorf("Git identity projection file is oversized")
	}
	return info, true, nil
}
