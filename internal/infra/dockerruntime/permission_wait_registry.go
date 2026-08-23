package dockerruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionDispositionObserver interface {
	ObservePermissionDisposition(context.Context, tobari.PermissionWaitRecord) (tobari.PermissionWaitResult, bool, error)
}

type permissionWaitEntry struct {
	record tobari.PermissionWaitRecord
	access tobari.PermissionWaitAccessState
}

// permissionWaitRegistry is attachment-owned bounded memory. Its only mutable
// state is correlation lifecycle; it has no policy mutation path.
type permissionWaitRegistry struct {
	mu       sync.Mutex
	session  tobari.InteractiveAttachmentSession
	observer permissionDispositionObserver
	records  map[string]*permissionWaitEntry
	now      func() time.Time
	wait     func(context.Context, time.Duration) error
}

func newPermissionWaitRegistry(session tobari.InteractiveAttachmentSession, observer permissionDispositionObserver) (*permissionWaitRegistry, error) {
	if err := session.Validate(); err != nil {
		return nil, err
	}
	if observer == nil {
		return nil, fmt.Errorf("permission disposition observer is required")
	}
	return &permissionWaitRegistry{
		session: session, observer: observer, records: map[string]*permissionWaitEntry{},
		now: time.Now, wait: waitPermissionObservation,
	}, nil
}

func waitPermissionObservation(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *permissionWaitRegistry) Register(record tobari.PermissionWaitRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	if record.WorkspaceManifestID != r.session.WorkspaceManifestID || record.WorkspaceID != r.session.WorkspaceID ||
		record.AttachmentID != r.session.AttachmentID || record.FrozenPrincipalFingerprint != r.session.FrozenPrincipalFingerprint {
		return fmt.Errorf("permission wait record does not belong to the attachment owner")
	}
	now := r.now()
	created, _ := time.Parse(time.RFC3339Nano, record.CreatedAt)
	expires, _ := time.Parse(time.RFC3339Nano, record.ExpiresAt)
	if created.After(now.Add(5*time.Second)) || !now.Before(expires) {
		return fmt.Errorf("permission wait record lease is not current")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, entry := range r.records {
		entryExpiry, _ := time.Parse(time.RFC3339Nano, entry.record.ExpiresAt)
		if !now.Before(entryExpiry) && !entry.access.Active {
			delete(r.records, id)
		}
	}
	if len(r.records) >= tobari.PermissionWaitMaxLive {
		return fmt.Errorf("permission wait attachment capacity is exhausted")
	}
	if _, exists := r.records[record.ID]; exists {
		return fmt.Errorf("permission wait record already exists")
	}
	r.records[record.ID] = &permissionWaitEntry{record: record}
	return nil
}

func invalidPermissionWaitFault() error {
	return fault.New(fault.KindNotFound, "invalid_permission_wait", "permission wait is unavailable", false)
}

func (r *permissionWaitRegistry) begin(id string) (tobari.PermissionWaitRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.records[id]
	if !exists {
		return tobari.PermissionWaitRecord{}, invalidPermissionWaitFault()
	}
	state, err := entry.access.StartAttempt()
	if err != nil {
		return tobari.PermissionWaitRecord{}, invalidPermissionWaitFault()
	}
	entry.access = state
	return entry.record, nil
}

func (r *permissionWaitRegistry) finish(id string, terminal bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	entry, exists := r.records[id]
	if !exists {
		return
	}
	state, err := entry.access.FinishAttempt(terminal)
	if err != nil {
		delete(r.records, id)
		return
	}
	entry.access = state
	if terminal {
		delete(r.records, id)
	}
}

// WaitPermission implements the read-only application observer port.
func (r *permissionWaitRegistry) WaitPermission(ctx context.Context, id string) (tobari.PermissionWaitResult, error) {
	record, err := r.begin(id)
	if err != nil {
		return "", err
	}
	terminal := false
	defer func() { r.finish(id, terminal) }()

	delays := [...]time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	attempt := 0
	for {
		expired, err := record.Expired(r.now())
		if err != nil {
			return "", fault.Wrap(fault.KindContract, "invalid_permission_wait_record", "permission wait record is invalid", false, err)
		}
		if expired {
			terminal = true
			return tobari.PermissionWaitResultExpired, nil
		}
		result, done, err := r.observer.ObservePermissionDisposition(ctx, record)
		if err != nil {
			return "", fault.Wrap(fault.KindUnavailable, "permission_wait_unavailable", "permission disposition is unavailable", true, err)
		}
		if done {
			if err := result.Validate(); err != nil {
				return "", fault.Wrap(fault.KindContract, "invalid_permission_wait_result", "permission wait result is invalid", false, err)
			}
			terminal = true
			return result, nil
		}
		delay := 5 * time.Second
		if attempt < len(delays) {
			delay = delays[attempt]
		}
		attempt++
		if err := r.wait(ctx, delay); err != nil {
			return "", fault.Wrap(fault.KindCanceled, "permission_wait_interrupted", "permission wait was interrupted", true, err)
		}
	}
}
