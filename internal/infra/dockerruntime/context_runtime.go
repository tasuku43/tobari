package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	contextRuntimeFileMode = 0o600
	maxRuntimeRecipeBytes  = 256 * 1024
)

const contextRuntimeTemplate = `# This file defines the runtime for the active Tobari Context.
# Add the tools and configuration your coding agent needs.
FROM ghcr.io/tasuku43/tobari/runtime:latest

USER root

# Example:
# RUN apt-get update \
#     && apt-get install -y --no-install-recommends nodejs npm \
#     && rm -rf /var/lib/apt/lists/*

USER tobari
`

func (r *Runtime) contextRuntimeDirectory(name string) string {
	return filepath.Join(r.contextDirectory(name), "runtime")
}

func (r *Runtime) contextRuntimeDockerfile(name string) string {
	return filepath.Join(r.contextRuntimeDirectory(name), "Dockerfile")
}

func (r *Runtime) contextRuntimeSourceDigest(name string) (string, error) {
	if err := requirePrivateDirectory(r.contextRuntimeDirectory(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", tobari.ErrRuntimeRecipeMissing
		}
		return "", fmt.Errorf("inspect runtime recipe directory: %w", err)
	}
	path := r.contextRuntimeDockerfile(name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", tobari.ErrRuntimeRecipeMissing
	}
	if err != nil {
		return "", fmt.Errorf("inspect runtime Dockerfile: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("runtime Dockerfile must be a regular owner-only file")
	}
	if info.Size() == 0 || info.Size() > maxRuntimeRecipeBytes {
		return "", fmt.Errorf("runtime Dockerfile must be between 1 and %d bytes", maxRuntimeRecipeBytes)
	}
	data, err := os.ReadFile(path) // #nosec G304 -- path is the fixed child of a validated Context.
	if err != nil {
		return "", fmt.Errorf("read runtime Dockerfile: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func (r *Runtime) contextRuntimeReport(manifest tobari.ContextManifest) (tobari.ContextRuntimeReport, error) {
	if manifest.Runtime == nil {
		return tobari.ContextRuntimeReport{
			Kind:   tobari.ContextRuntimeKindOfficial,
			Status: tobari.ContextRuntimeStatusOfficial,
		}, nil
	}
	recipe := *manifest.Runtime
	if err := recipe.Validate(); err != nil {
		return tobari.ContextRuntimeReport{}, err
	}
	report := tobari.ContextRuntimeReport{
		Kind:          recipe.Kind,
		Status:        tobari.ContextRuntimeStatusPendingBuild,
		Dockerfile:    r.contextRuntimeDockerfile(manifest.Name),
		BaseReference: recipe.BaseReference,
		SourceDigest:  recipe.SourceDigest,
	}
	sourceDigest, err := r.contextRuntimeSourceDigest(manifest.Name)
	if errors.Is(err, tobari.ErrRuntimeRecipeMissing) {
		report.Status = tobari.ContextRuntimeStatusInvalid
		return report, nil
	}
	if err != nil {
		return tobari.ContextRuntimeReport{}, err
	}
	report.SourceDigest = sourceDigest
	if recipe.LastBuild == nil {
		return report, nil
	}
	report.ImageDigest = recipe.LastBuild.ImageDigest
	if recipe.LastBuild.SourceDigest != sourceDigest || manifest.Image != recipe.LastBuild.Image {
		return report, nil
	}
	report.Status = tobari.ContextRuntimeStatusReady
	return report, nil
}

func (r *Runtime) contextReport(ctx context.Context, task string, manifest tobari.ContextManifest, active string) (tobari.ContextReport, error) {
	runtimeReport, err := r.contextRuntimeReport(manifest)
	if err != nil {
		return tobari.ContextReport{}, err
	}
	result := tobari.ContextReport{
		Task:         task,
		Name:         manifest.Name,
		Active:       manifest.Name == active,
		AgentProfile: manifest.AgentProfile,
		Image:        manifest.Image,
		PolicyMode:   manifest.PolicyMode,
		Stores:       r.contextPaths(manifest.Name),
		Runtime:      runtimeReport,
	}
	if err := result.Validate(); err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// InitRuntime creates the active Context's recipe without changing its
// selected image. The manifest update is atomic; an existing recipe is never
// overwritten.
func (r *Runtime) InitRuntime(ctx context.Context) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withClusterLock(func() error {
		manifest, _, err := r.activeContext()
		if err != nil {
			return err
		}
		if manifest.Runtime != nil {
			return tobari.ErrRuntimeRecipeExists
		}
		directory := r.contextRuntimeDirectory(manifest.Name)
		if err := r.ensurePrivateDirectory(directory); err != nil {
			return fmt.Errorf("prepare Context runtime directory: %w", err)
		}
		path := r.contextRuntimeDockerfile(manifest.Name)
		if _, err := os.Lstat(path); err == nil {
			return tobari.ErrRuntimeRecipeExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect Context runtime recipe: %w", err)
		}

		previous := manifest
		manifest.SchemaVersion = tobari.ContextSchemaVersion
		manifest.Runtime = &tobari.ContextRuntimeRecipe{
			Kind:          tobari.ContextRuntimeKindDockerfile,
			File:          tobari.ContextRuntimeRecipeFile,
			BaseReference: tobari.OfficialRuntimeBase,
		}
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			return fmt.Errorf("write runtime recipe metadata: %w", err)
		}
		if err := initializeBytes(path, []byte(contextRuntimeTemplate), contextRuntimeFileMode); err != nil {
			_ = writeAtomicJSON(r.contextManifestPath(previous.Name), previous)
			return fmt.Errorf("write runtime Dockerfile: %w", err)
		}
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		result, err = r.contextReport(ctx, tobari.TaskRuntimeInit, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

// BuildRuntime builds, validates, and atomically selects the active Context's
// generated local image. The previous selected image remains authoritative
// until every step succeeds.
func (r *Runtime) BuildRuntime(ctx context.Context) (tobari.ContextReport, error) {
	if err := ctx.Err(); err != nil {
		return tobari.ContextReport{}, err
	}
	var result tobari.ContextReport
	err := r.withClusterLock(func() error {
		manifest, _, err := r.activeContext()
		if err != nil {
			return err
		}
		if manifest.Runtime == nil {
			return tobari.ErrRuntimeRecipeMissing
		}
		recipe := *manifest.Runtime
		if err := recipe.Validate(); err != nil {
			return err
		}
		sourceDigest, err := r.contextRuntimeSourceDigest(manifest.Name)
		if err != nil {
			return err
		}
		image := managedRuntimeImage(manifest.Name, sourceDigest)
		var output bytes.Buffer
		if err := r.runner.Run(
			ctx,
			[]string{"build", "--tag", image, "--file", r.contextRuntimeDockerfile(manifest.Name), r.contextRuntimeDirectory(manifest.Name)},
			os.Environ(), nil, &output, &output,
		); err != nil {
			return fmt.Errorf("build Context runtime: %w: %s", err, boundedDiagnostic(output.Bytes()))
		}
		if err := r.validateCompatibleImage(ctx, image); err != nil {
			return err
		}
		imageDigest, err := r.inspectImageDigest(ctx, image)
		if err != nil {
			return err
		}
		previous := manifest
		manifest.SchemaVersion = tobari.ContextSchemaVersion
		manifest.Image = image
		recipe.SourceDigest = sourceDigest
		recipe.LastBuild = &tobari.ContextRuntimeBuild{
			Image: image, ImageDigest: imageDigest, SourceDigest: sourceDigest,
		}
		manifest.Runtime = &recipe
		if err := manifest.Validate(); err != nil {
			return err
		}
		if err := writeAtomicJSON(r.contextManifestPath(manifest.Name), manifest); err != nil {
			_ = writeAtomicJSON(r.contextManifestPath(previous.Name), previous)
			return fmt.Errorf("promote Context runtime: %w", err)
		}
		active, err := r.readActiveContext()
		if err != nil {
			return err
		}
		result, err = r.contextReport(ctx, tobari.TaskRuntimeBuild, manifest, active)
		return err
	})
	if err != nil {
		return tobari.ContextReport{}, err
	}
	return result, nil
}

func (r *Runtime) inspectImageDigest(ctx context.Context, image string) (string, error) {
	output, err := r.runner.Output(ctx, []string{"image", "inspect", "--format", "{{.Id}}", image}, os.Environ())
	if err != nil {
		return "", fmt.Errorf("inspect built Context runtime: %w: %s", err, boundedDiagnostic(output))
	}
	digest := strings.TrimSpace(string(output))
	if err := tobari.ValidateDigest(digest); err != nil {
		return "", fmt.Errorf("built Context runtime returned invalid image identity: %w", err)
	}
	return digest, nil
}

func managedRuntimeImage(contextName, sourceDigest string) string {
	short := strings.TrimPrefix(sourceDigest, "sha256:")
	if len(short) > 12 {
		short = short[:12]
	}
	return "tobari-context-" + contextName + ":" + short
}
