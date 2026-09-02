package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// WithLifecycleLock serializes the installation-wide shared-cluster and
// CWD-owned project lifecycle. The application layer holds this boundary
// across its check-then-mutate sequence; lower-level state locks remain
// narrower and are acquired after this lock.
func (r *Runtime) WithLifecycleLock(ctx context.Context, action func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.lifecycleLockAttempt != nil {
		r.lifecycleLockAttempt()
	}
	if err := r.ensurePrivateDirectory(r.stateDirectory); err != nil {
		return fmt.Errorf("prepare lifecycle state directory: %w", err)
	}
	path := filepath.Join(r.stateDirectory, "lifecycle.lock")
	if info, err := os.Lstat(path); err == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("lifecycle lock is not a regular file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect lifecycle lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) // #nosec G304 -- fixed state child after lstat.
	if err != nil {
		return fmt.Errorf("open lifecycle lock: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("protect lifecycle lock: %w", err)
	}
	for {
		acquired, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock lifecycle state: %w", lockErr)
		}
		if acquired {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	defer unlockProjectFile(file)
	return action(ctx)
}

// withLifecycleObservation shares the installation lifecycle serialization
// only when its lock already exists. It never creates state. A fresh root can
// be observed without a lock; if state appears during that observation the
// result is rejected instead of being treated as a consistent snapshot.
func (r *Runtime) withLifecycleObservation(ctx context.Context, action func(context.Context) error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	stateInfo, err := os.Lstat(r.stateDirectory)
	if errors.Is(err, os.ErrNotExist) {
		if err := action(ctx); err != nil {
			return err
		}
		if _, err := os.Lstat(r.stateDirectory); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return fmt.Errorf("recheck lifecycle state directory: %w", err)
		}
		return tobariProtectionObservationChanged()
	}
	if err != nil {
		return fmt.Errorf("inspect lifecycle state directory: %w", err)
	}
	if !stateInfo.IsDir() || stateInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("lifecycle state directory is unsafe")
	}
	path := filepath.Join(r.stateDirectory, "lifecycle.lock")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return tobariProtectionObservationChanged()
	}
	if err != nil {
		return fmt.Errorf("inspect lifecycle lock: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("lifecycle lock is not a regular file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY, 0) // #nosec G304 -- validated existing fixed state child; observation never creates it.
	if err != nil {
		return fmt.Errorf("open lifecycle lock for observation: %w", err)
	}
	acquired := false
	defer func() {
		if acquired {
			unlockProjectFile(file)
		}
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close lifecycle observation lock: %w", closeErr)
		}
	}()
	for {
		locked, lockErr := tryLockProjectFile(file)
		if lockErr != nil {
			return fmt.Errorf("lock lifecycle state for observation: %w", lockErr)
		}
		if locked {
			acquired = true
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
	return action(ctx)
}

// WithLifecycleObservation exposes the read-only half of the installation
// lifecycle boundary to infrastructure adapters that must observe a journal
// and its authority receipt coherently. It never creates the lock or state.
func (r *Runtime) WithLifecycleObservation(ctx context.Context, action func(context.Context) error) error {
	return r.withLifecycleObservation(ctx, action)
}

func tobariProtectionObservationChanged() error {
	return tobari.RuntimeProtectionInventoryError{Reason: tobari.RuntimeProtectionInventoryObservationUnknown}
}
