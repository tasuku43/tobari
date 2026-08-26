package workspaceauthoritystore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// ObserveMutationRecovery reads the reserved final-authority decision and
// stage paths without acquiring a lifecycle lock or changing either artifact.
// It is intentionally shared by status and doctor so read-only diagnostics
// describe the same recovery fact that blocks another mutation.
func (s *Store) ObserveMutationRecovery(ctx context.Context) (tobari.FinalAuthorityMutationObservation, error) {
	var result tobari.FinalAuthorityMutationObservation
	if s == nil || s.root == "" {
		return result, fmt.Errorf("final Workspace authority store is unavailable")
	}
	if ctx == nil {
		return result, fmt.Errorf("final mutation recovery observation context is nil")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	decisionPath, stagePath := mutationRecoveryArtifactPaths(s.root)
	info, err := os.Lstat(decisionPath)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return result, fmt.Errorf("inspect final mutation decision: %w", err)
	default:
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || !ownedByCurrentUser(info) {
			return result, fmt.Errorf("final mutation decision is unsafe")
		}
		data, readErr := readEffectDecisionFile(decisionPath)
		if readErr != nil {
			return result, fmt.Errorf("read final mutation decision: %w", readErr)
		}
		var decision effectDecision
		if decodeErr := decodeStrictJSON(data, &decision); decodeErr != nil {
			return result, fmt.Errorf("decode final mutation decision: %w", decodeErr)
		}
		if validateErr := decision.validate(); validateErr != nil {
			return result, fmt.Errorf("validate final mutation decision: %w", validateErr)
		}
		result.ActiveDecision = true
		result.Operation = decision.Operation
		result.Target = decision.Target
	}

	if err := ctx.Err(); err != nil {
		return tobari.FinalAuthorityMutationObservation{}, err
	}
	stageInfo, err := os.Lstat(stagePath)
	if errors.Is(err, os.ErrNotExist) {
		return result, result.Validate()
	}
	if err != nil {
		return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("inspect final mutation stage: %w", err)
	}
	if stageInfo.Mode()&os.ModeSymlink != 0 || !ownedByCurrentUser(stageInfo) {
		return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("final mutation stage is unsafe")
	}
	if stageInfo.IsDir() {
		if stageInfo.Mode().Perm() != 0o700 {
			return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("final mutation stage directory is unsafe")
		}
		entries, readErr := os.ReadDir(stagePath)
		if readErr != nil {
			return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("read final mutation stage directory: %w", readErr)
		}
		if len(entries) > 1 || len(entries) == 1 && entries[0].Name() != authorityFileName {
			return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("final mutation stage is foreign or mixed")
		}
	} else if !stageInfo.Mode().IsRegular() || stageInfo.Mode().Perm() != 0o600 {
		return tobari.FinalAuthorityMutationObservation{}, fmt.Errorf("final mutation stage is unsafe")
	}
	result.StagePresent = true
	return result, result.Validate()
}

// mutationRecoveryArtifactPaths is kept in one place for tests and for the
// final-only boundary audit. The paths are suffixes of the final authority
// root, never predecessor Context paths.
func mutationRecoveryArtifactPaths(root string) (decision, stage string) {
	return filepath.Join(root, "journal", "mutation-decision.json"), mutationStagePath(root)
}

func mutationStagePath(root string) string { return filepath.Join(root, "journal", "active.tmp") }
