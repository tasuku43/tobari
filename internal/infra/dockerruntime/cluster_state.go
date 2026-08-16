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
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
	"github.com/tasuku43/tobari/internal/infra/runtimeassets"
)

func (r *Runtime) prepareState(ctx context.Context) (tobari.State, error) {
	for name, path := range map[string]string{
		"configuration": r.configDirectory,
		"state":         r.stateDirectory,
		"data":          r.dataDirectory,
	} {
		if err := r.ensurePrivateDirectory(path); err != nil {
			return tobari.State{}, fmt.Errorf("prepare %s directory: %w", name, err)
		}
	}
	if brokerRuntimeEnabled {
		if _, err := r.prepareAuthProjection(); err != nil {
			return tobari.State{}, fmt.Errorf("prepare Auth Broker provider projection: %w", err)
		}
	}
	version, err := runtimeassets.Version()
	if err != nil {
		return tobari.State{}, err
	}
	runtimeDirectory := filepath.Join(r.stateDirectory, "runtime", version)
	if err := runtimeassets.Materialize(runtimeDirectory); err != nil {
		return tobari.State{}, err
	}
	if err := r.ensureContextStore(); err != nil {
		return tobari.State{}, fmt.Errorf("prepare Context catalog: %w", err)
	}
	if err := r.withPolicyProjectionLock(ctx, func() error {
		return r.recoverAllPolicySourceTransactions(ctx)
	}); err != nil {
		return tobari.State{}, fmt.Errorf("recover interrupted Context policy source transaction: %w", err)
	}
	projection, err := r.buildAggregateProjection(ctx)
	if err != nil {
		return tobari.State{}, fmt.Errorf("prepare aggregate Context projection: %w", err)
	}
	if err := r.ensureProjectPrincipalRegistry(ctx); err != nil {
		return tobari.State{}, fmt.Errorf("validate project principal registry: %w", err)
	}
	state := tobari.State{
		SchemaVersion: 1, RuntimeDirectory: runtimeDirectory,
		AggregateRevision: projection.Revision, ContextCount: projection.ContextCount,
		PolicyDirectory: projection.PolicyDirectory, GatewayConfig: projection.GatewayConfig,
		AssetVersion: version,
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, err
	}
	return state, nil
}

func initializeFile(target, asset string, mode os.FileMode) error {
	data, err := runtimeassets.Read(asset)
	if err != nil {
		return err
	}
	return initializeBytes(target, data, mode)
}

func initializeBytes(target string, data []byte, mode os.FileMode) error {
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration path %s must be a regular file", filepath.Base(target))
		}
		if err := os.Chmod(target, mode); err != nil {
			return fmt.Errorf("set configuration file permissions: %w", err)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect configuration file: %w", err)
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode) // #nosec G304 -- fixed child and O_EXCL prevent overwrite.
	if err != nil {
		return fmt.Errorf("create configuration file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write configuration file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close configuration file: %w", err)
	}
	return nil
}

func (r *Runtime) statePath() string { return filepath.Join(r.stateDirectory, "state.json") }

func (r *Runtime) writeState(state tobari.State) error {
	if err := state.Validate(); err != nil {
		return err
	}
	return r.withClusterLock(func() error {
		if err := os.MkdirAll(r.stateDirectory, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(r.stateDirectory, 0o700); err != nil { // #nosec G302 -- shared state is owner-only.
			return err
		}
		return writeAtomicJSON(r.statePath(), state)
	})
}

func (r *Runtime) withClusterLock(action func() error) error {
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return fmt.Errorf("prepare shared state directory: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "cluster.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("cluster lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect cluster lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed state child after lstat.
	if err != nil {
		return fmt.Errorf("open cluster lock: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect cluster lock: %w", err)
	}
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock shared state: %w", lockErr)
		}
		if acquired {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer unlockProjectFile(file)
	return action()
}

// LoadState returns absence separately from corrupt state.
func (r *Runtime) LoadState(ctx context.Context) (tobari.State, bool, error) {
	if err := ctx.Err(); err != nil {
		return tobari.State{}, false, err
	}
	info, err := os.Lstat(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxProjectStateBytes {
		return tobari.State{}, false, fmt.Errorf("Tobari state file is unsafe")
	}
	data, err := os.ReadFile(r.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return tobari.State{}, false, nil
	}
	if err != nil {
		return tobari.State{}, false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var state tobari.State
	if err := decoder.Decode(&state); err != nil {
		return tobari.State{}, false, fmt.Errorf("decode Tobari state: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return tobari.State{}, false, fmt.Errorf("Tobari state contains trailing data")
	}
	if err := state.Validate(); err != nil {
		return tobari.State{}, false, err
	}
	return state, true, nil
}

// Attach creates one exact container, internal network, and persistent home.

func (r *Runtime) recordRecentError(state tobari.State, message string) error {
	state.RecentError = message
	return r.writeState(state)
}
