package dockerruntime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	permissionSessionHeartbeat = 10 * time.Second
	permissionSessionCleanup   = 3 * time.Second
	permissionSessionHandshake = 65
)

type interactiveWorkspaceAttachment struct {
	runtime       *Runtime
	session       tobari.InteractiveAttachmentSession
	listener      net.Listener
	waits         *permissionWaitRegistry
	owned         bool
	heartbeatStop chan struct{}
	heartbeatDone chan struct{}
	activeMu      sync.Mutex
	active        map[net.Conn]struct{}
	closing       bool
	once          sync.Once
}

func (r *Runtime) interactiveAttachmentDirectory() string {
	return filepath.Join(r.configDirectory, "interactive-attachments")
}

func (r *Runtime) interactiveAttachmentSessionRegistryPath() string {
	return filepath.Join(r.interactiveAttachmentDirectory(), "sessions.json")
}

func emptyInteractiveAttachmentSessionRegistry() tobari.InteractiveAttachmentSessionRegistry {
	return tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{}}
}

func (r *Runtime) ensureInteractiveAttachmentStore(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.interactiveAttachmentDirectory()); err != nil {
		return fmt.Errorf("prepare interactive attachment directory: %w", err)
	}
	path := r.interactiveAttachmentSessionRegistryPath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := initializeBytes(path, mustJSONBytes(emptyInteractiveAttachmentSessionRegistry()), 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("inspect interactive attachment registry: %w", err)
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(path, &registry); err != nil {
		return err
	}
	return registry.Validate()
}

func (r *Runtime) withInteractiveAttachmentLock(ctx context.Context, action func() error) error {
	return r.withConfigFileLock(ctx, "interactive-attachment.lock", "interactive attachment registry", action)
}

func newPermissionSessionNonce() (string, error) {
	var raw [32]byte
	if _, err := io.ReadFull(rand.Reader, raw[:]); err != nil {
		return "", fmt.Errorf("generate permission session nonce: %w", err)
	}
	return hex.EncodeToString(raw[:]), nil
}

func frozenPrincipalFingerprint(binding projectPrincipalBinding) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"tobari-frozen-principal-v1", binding.ProjectID, binding.WorkspaceManifestID,
		binding.WorkspaceManifestName, binding.ProjectRoot, binding.WorkspaceIP,
		binding.GatewayIP, binding.Network,
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func (r *Runtime) exactFrozenPrincipalFingerprint(workspaceManifestID, workspaceID string) (string, error) {
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return "", err
	}
	matches := make([]projectPrincipalBinding, 0, 1)
	for _, binding := range registry.Bindings {
		if binding.WorkspaceManifestID == workspaceManifestID && binding.ProjectID == workspaceID {
			matches = append(matches, binding)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("canonical interactive attachment principal join is not unique")
	}
	return frozenPrincipalFingerprint(matches[0]), nil
}

func (r *Runtime) beginInteractiveWorkspaceAttachment(ctx context.Context, workspace tobari.Workspace) (*interactiveWorkspaceAttachment, error) {
	if err := workspace.Validate(); err != nil {
		return nil, err
	}
	if err := r.ensureInteractiveAttachmentStore(ctx); err != nil {
		return nil, err
	}
	fingerprint, err := r.exactFrozenPrincipalFingerprint(workspace.WorkspaceManifestID, workspace.ID)
	if err != nil {
		return nil, fmt.Errorf("bind canonical interactive attachment principal: %w", err)
	}

	// Inspect only the exact pair while locked. Expired records are removed by
	// bounded time evidence; live candidates are probed once after unlocking.
	var existing *tobari.InteractiveAttachmentSession
	err = r.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		now := time.Now()
		kept := registry.Sessions[:0]
		for _, session := range registry.Sessions {
			if session.WorkspaceManifestID == workspace.WorkspaceManifestID && session.WorkspaceID == workspace.ID {
				expires, _ := time.Parse(time.RFC3339Nano, session.ExpiresAt)
				if !now.Before(expires) {
					continue
				}
				copy := session
				existing = &copy
			}
			kept = append(kept, session)
		}
		if len(kept) != len(registry.Sessions) {
			registry.Sessions = kept
			return writeAtomicJSON(r.interactiveAttachmentSessionRegistryPath(), registry)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.FrozenPrincipalFingerprint != fingerprint || !r.permissionSessionActive(*existing) {
			return nil, fmt.Errorf("canonical interactive attachment owner is stale or unavailable")
		}
		// Re-read under lock and require the exact nonce/epoch record to be
		// unchanged across the bounded probe. No timing/name/PID join is used.
		err := r.withInteractiveAttachmentLock(ctx, func() error {
			var registry tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
				return err
			}
			current := findInteractiveSession(registry, workspace.WorkspaceManifestID, workspace.ID)
			if current == nil || current.AttachmentID != existing.AttachmentID || current.IngestionNonce != existing.IngestionNonce || current.OwnerPID != existing.OwnerPID {
				return fmt.Errorf("canonical interactive attachment owner changed concurrently")
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		return &interactiveWorkspaceAttachment{runtime: r, session: *existing, owned: false}, nil
	}

	epochID, err := newAttachmentEpochID()
	if err != nil {
		return nil, err
	}
	nonce, err := newPermissionSessionNonce()
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("listen for interactive attachment: %w", err)
	}
	now := time.Now().UTC()
	session := tobari.InteractiveAttachmentSession{
		SchemaVersion:       tobari.PermissionSessionSchema,
		WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
		AttachmentID: epochID, OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: fingerprint, OwnerPID: os.Getpid(),
		IngestionPort: listener.Addr().(*net.TCPAddr).Port, IngestionNonce: nonce,
		CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	if err := session.Validate(); err != nil {
		_ = listener.Close()
		return nil, err
	}
	attachment := &interactiveWorkspaceAttachment{
		runtime: r, session: session, listener: listener, owned: true,
		active: map[net.Conn]struct{}{}, heartbeatStop: make(chan struct{}), heartbeatDone: make(chan struct{}),
	}
	waits, err := newPermissionWaitRegistry(session, r)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	attachment.waits = waits
	go attachment.serve()

	err = r.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		if findInteractiveSession(registry, workspace.WorkspaceManifestID, workspace.ID) != nil {
			return fmt.Errorf("canonical interactive attachment owner appeared concurrently")
		}
		registry.Sessions = append(registry.Sessions, session)
		sort.Slice(registry.Sessions, func(i, j int) bool {
			if registry.Sessions[i].WorkspaceManifestID != registry.Sessions[j].WorkspaceManifestID {
				return registry.Sessions[i].WorkspaceManifestID < registry.Sessions[j].WorkspaceManifestID
			}
			return registry.Sessions[i].WorkspaceID < registry.Sessions[j].WorkspaceID
		})
		if err := registry.Validate(); err != nil {
			return err
		}
		return writeAtomicJSON(r.interactiveAttachmentSessionRegistryPath(), registry)
	})
	if err != nil {
		attachment.closeTransport()
		return nil, err
	}
	attachment.startHeartbeat()
	return attachment, nil
}

func findInteractiveSession(registry tobari.InteractiveAttachmentSessionRegistry, manifestID, workspaceID string) *tobari.InteractiveAttachmentSession {
	var found *tobari.InteractiveAttachmentSession
	for _, session := range registry.Sessions {
		if session.WorkspaceManifestID != manifestID || session.WorkspaceID != workspaceID {
			continue
		}
		if found != nil {
			return nil
		}
		copy := session
		found = &copy
	}
	return found
}

func (a *interactiveWorkspaceAttachment) serve() {
	for {
		connection, err := a.listener.Accept()
		if err != nil {
			return
		}
		if !a.track(connection) {
			_ = connection.Close()
			return
		}
		go a.handle(connection)
	}
}

func (a *interactiveWorkspaceAttachment) track(connection net.Conn) bool {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	if a.closing {
		return false
	}
	a.active[connection] = struct{}{}
	return true
}

func (a *interactiveWorkspaceAttachment) untrack(connection net.Conn) {
	a.activeMu.Lock()
	delete(a.active, connection)
	a.activeMu.Unlock()
	_ = connection.Close()
}

func (a *interactiveWorkspaceAttachment) handle(connection net.Conn) {
	defer a.untrack(connection)
	_ = connection.SetDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, permissionSessionHandshake)
	if _, err := io.ReadFull(connection, header); err != nil ||
		(header[0] != 'S' && header[0] != 'W') ||
		subtle.ConstantTimeCompare(header[1:], []byte(a.session.IngestionNonce)) != 1 {
		return
	}
	if header[0] == 'S' {
		_, _ = connection.Write([]byte("OK"))
		return
	}
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(connection, lengthBytes); err != nil {
		return
	}
	length := binary.BigEndian.Uint32(lengthBytes)
	if length == 0 || length > tobari.PermissionWaitRequestLimit {
		return
	}
	payload := make([]byte, int(length))
	if _, err := io.ReadFull(connection, payload); err != nil {
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var record tobari.PermissionWaitRecord
	if err := decoder.Decode(&record); err != nil {
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return
	}
	if err := a.waits.Register(record); err != nil {
		return
	}
	_, _ = connection.Write([]byte("OK"))
}

func (r *Runtime) permissionSessionActive(session tobari.InteractiveAttachmentSession) bool {
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(session.IngestionPort)), 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := connection.Write([]byte("S" + session.IngestionNonce)); err != nil {
		return false
	}
	response := make([]byte, 2)
	_, err = io.ReadFull(connection, response)
	return err == nil && string(response) == "OK"
}

func (a *interactiveWorkspaceAttachment) startHeartbeat() {
	go func() {
		defer close(a.heartbeatDone)
		ticker := time.NewTicker(permissionSessionHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-a.heartbeatStop:
				return
			case now := <-ticker.C:
				if err := a.renew(now); err != nil {
					return
				}
			}
		}
	}()
}

func (a *interactiveWorkspaceAttachment) renew(now time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return a.runtime.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		current := findInteractiveSession(registry, a.session.WorkspaceManifestID, a.session.WorkspaceID)
		if current == nil || current.AttachmentID != a.session.AttachmentID || current.IngestionNonce != a.session.IngestionNonce || current.OwnerPID != os.Getpid() {
			return fmt.Errorf("interactive attachment owner changed")
		}
		for index := range registry.Sessions {
			if registry.Sessions[index].AttachmentID == a.session.AttachmentID {
				registry.Sessions[index].ExpiresAt = now.UTC().Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano)
			}
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		return writeAtomicJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), registry)
	})
}

func (a *interactiveWorkspaceAttachment) closeTransport() {
	if a.listener != nil {
		_ = a.listener.Close()
	}
	a.activeMu.Lock()
	a.closing = true
	for connection := range a.active {
		_ = connection.Close()
	}
	a.activeMu.Unlock()
}

func (a *interactiveWorkspaceAttachment) Close(_ context.Context) error {
	if !a.owned {
		return nil
	}
	var result error
	a.once.Do(func() {
		close(a.heartbeatStop)
		<-a.heartbeatDone
		// Transport disappears before its authority record. Cleanup is bounded
		// independently from child/caller cancellation.
		a.closeTransport()
		cleanup, cancel := context.WithTimeout(context.Background(), permissionSessionCleanup)
		defer cancel()
		result = a.runtime.withInteractiveAttachmentLock(cleanup, func() error {
			var registry tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
				return err
			}
			kept := registry.Sessions[:0]
			for _, session := range registry.Sessions {
				if session.AttachmentID != a.session.AttachmentID {
					kept = append(kept, session)
				}
			}
			registry.Sessions = kept
			if err := registry.Validate(); err != nil {
				return err
			}
			return writeAtomicJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), registry)
		})
	})
	return result
}
