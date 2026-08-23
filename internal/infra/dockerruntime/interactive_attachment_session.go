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
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	permissionSessionHeartbeat  = 10 * time.Second
	permissionSessionCleanup    = 3 * time.Second
	permissionSessionHandshake  = 65
	permissionSessionMaxClients = 8
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
	closeDone     chan struct{}
	closeErr      error
	transportOnce sync.Once
	transportErr  error
	authorityOnce sync.Once
	authorityDone chan struct{}
	authorityErr  error
	heartbeatOnce sync.Once
}

func (r *Runtime) interactiveAttachmentDirectory() string {
	return filepath.Join(r.configDirectory, "interactive-attachments")
}

// interactiveAttachmentSocketDirectory is short enough for the Unix sockaddr
// limit on both Darwin and Linux. Its config-derived name isolates concurrent
// Tobari installations owned by the same host user; the directory and socket
// remain owner-only. The registry stores only a validated basename.
func (r *Runtime) interactiveAttachmentSocketDirectory() string {
	digest := sha256.Sum256([]byte(r.configDirectory))
	return filepath.Join("/tmp", fmt.Sprintf("tobari-permission-%d-%x", os.Getuid(), digest[:8]))
}

func (r *Runtime) interactiveAttachmentSocketPath(session tobari.InteractiveAttachmentSession) string {
	return filepath.Join(r.interactiveAttachmentSocketDirectory(), session.IngestionSocket)
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
	if err := r.ensurePrivateDirectory(r.interactiveAttachmentSocketDirectory()); err != nil {
		return fmt.Errorf("prepare interactive attachment socket directory: %w", err)
	}
	path := r.interactiveAttachmentSessionRegistryPath()
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := initializeBytes(path, mustJSONBytes(emptyInteractiveAttachmentSessionRegistry()), 0o600); err != nil && !errors.Is(err, os.ErrExist) {
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

func sameInteractiveSessionAuthority(left, right tobari.InteractiveAttachmentSession) bool {
	return left.SchemaVersion == right.SchemaVersion &&
		left.WorkspaceManifestID == right.WorkspaceManifestID && left.WorkspaceID == right.WorkspaceID &&
		left.AttachmentID == right.AttachmentID && left.OwnerKind == right.OwnerKind &&
		left.FrozenPrincipalFingerprint == right.FrozenPrincipalFingerprint &&
		left.OwnerPID == right.OwnerPID && left.IngestionSocket == right.IngestionSocket &&
		left.IngestionNonce == right.IngestionNonce && left.CreatedAt == right.CreatedAt
}

func permissionSessionLeaseCurrent(session tobari.InteractiveAttachmentSession, now time.Time) bool {
	if err := session.Validate(); err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	return err == nil && now.Before(expires)
}

func (r *Runtime) borrowInteractiveWorkspaceAttachment(
	ctx context.Context, expected tobari.InteractiveAttachmentSession, fingerprint string,
) (*interactiveWorkspaceAttachment, error) {
	if expected.FrozenPrincipalFingerprint != fingerprint || !permissionSessionLeaseCurrent(expected, time.Now()) || !r.permissionSessionActive(expected) {
		return nil, fmt.Errorf("canonical interactive attachment owner is stale or unavailable")
	}
	var verified tobari.InteractiveAttachmentSession
	err := r.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		current := findInteractiveSession(registry, expected.WorkspaceManifestID, expected.WorkspaceID)
		if current == nil || !sameInteractiveSessionAuthority(*current, expected) ||
			current.FrozenPrincipalFingerprint != fingerprint || !permissionSessionLeaseCurrent(*current, time.Now()) {
			return fmt.Errorf("canonical interactive attachment owner changed concurrently")
		}
		verified = *current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &interactiveWorkspaceAttachment{runtime: r, session: verified, owned: false}, nil
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

	// Startup is a mutation boundary, so it compactly removes every expired
	// validated lease before capacity is assessed. Malformed or ambiguous state
	// still fails closed before any cleanup.
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
			expires, _ := time.Parse(time.RFC3339Nano, session.ExpiresAt)
			if !now.Before(expires) {
				continue
			}
			if session.WorkspaceManifestID == workspace.WorkspaceManifestID && session.WorkspaceID == workspace.ID {
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
		return r.borrowInteractiveWorkspaceAttachment(ctx, *existing, fingerprint)
	}

	epochID, err := newAttachmentEpochID()
	if err != nil {
		return nil, err
	}
	nonce, err := newPermissionSessionNonce()
	if err != nil {
		return nil, err
	}
	socketName := "pws_" + strings.TrimPrefix(epochID, "att_") + ".sock"
	socketPath := filepath.Join(r.interactiveAttachmentSocketDirectory(), socketName)
	if _, err := os.Lstat(socketPath); err == nil {
		return nil, fmt.Errorf("interactive attachment socket already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect interactive attachment socket: %w", err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen for interactive attachment: %w", err)
	}
	listener.SetUnlinkOnClose(true)
	if err := os.Chmod(socketPath, 0o600); err != nil { // #nosec G302 -- same-host owner-only ingestion socket.
		closeErr := listener.Close()
		return nil, errors.Join(fmt.Errorf("protect interactive attachment socket: %w", err), closeErr)
	}
	now := time.Now().UTC()
	session := tobari.InteractiveAttachmentSession{
		SchemaVersion:       tobari.PermissionSessionSchema,
		WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
		AttachmentID: epochID, OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: fingerprint, OwnerPID: os.Getpid(),
		IngestionSocket: socketName, IngestionNonce: nonce,
		CreatedAt: now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	if err := session.Validate(); err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	attachment := &interactiveWorkspaceAttachment{
		runtime: r, session: session, listener: listener, owned: true,
		active: map[net.Conn]struct{}{}, heartbeatStop: make(chan struct{}), heartbeatDone: make(chan struct{}), authorityDone: make(chan struct{}), closeDone: make(chan struct{}),
	}
	waits, err := newPermissionWaitRegistry(session, r)
	if err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	attachment.waits = waits

	var winner *tobari.InteractiveAttachmentSession
	err = r.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		if current := findInteractiveSession(registry, workspace.WorkspaceManifestID, workspace.ID); current != nil {
			copy := *current
			winner = &copy
			return nil
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
		return nil, errors.Join(err, attachment.closeTransport())
	}
	if winner != nil {
		if closeErr := attachment.closeTransport(); closeErr != nil {
			return nil, closeErr
		}
		return r.borrowInteractiveWorkspaceAttachment(ctx, *winner, fingerprint)
	}
	go attachment.serve()
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
			a.activeMu.Lock()
			closing := a.closing
			a.activeMu.Unlock()
			if !closing {
				a.failClosed()
			}
			return
		}
		if !a.track(connection) {
			_ = connection.Close()
			continue
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
	if len(a.active) >= permissionSessionMaxClients {
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
	if err := connection.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return
	}
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
	if err := session.Validate(); err != nil || requirePrivateDirectory(r.interactiveAttachmentSocketDirectory()) != nil {
		return false
	}
	socketPath := r.interactiveAttachmentSocketPath(session)
	info, err := os.Lstat(socketPath)
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return false
	}
	connection, err := net.DialTimeout("unix", socketPath, 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer func() { _ = connection.Close() }()
	if err := connection.SetDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		return false
	}
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
					a.failClosed()
					return
				}
			}
		}
	}()
}

func (a *interactiveWorkspaceAttachment) renew(now time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	expires := now.UTC().Add(tobari.PermissionSessionLease)
	if err := a.runtime.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		current := findInteractiveSession(registry, a.session.WorkspaceManifestID, a.session.WorkspaceID)
		if current == nil || !sameInteractiveSessionAuthority(*current, a.session) || !permissionSessionLeaseCurrent(*current, now) {
			return fmt.Errorf("interactive attachment owner changed")
		}
		for index := range registry.Sessions {
			if registry.Sessions[index].AttachmentID == a.session.AttachmentID {
				registry.Sessions[index].LeaseIssuedAt = now.UTC().Format(time.RFC3339Nano)
				registry.Sessions[index].ExpiresAt = expires.Format(time.RFC3339Nano)
			}
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		return writeAtomicJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), registry)
	}); err != nil {
		return err
	}
	return a.waits.RenewOwner(now.UTC(), expires)
}

func (a *interactiveWorkspaceAttachment) closeTransport() error {
	a.transportOnce.Do(func() {
		a.activeMu.Lock()
		a.closing = true
		connections := make([]net.Conn, 0, len(a.active))
		for connection := range a.active {
			connections = append(connections, connection)
		}
		a.activeMu.Unlock()
		var failures []error
		if a.listener != nil {
			if err := a.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				failures = append(failures, fmt.Errorf("close interactive attachment listener: %w", err))
			}
		}
		for _, connection := range connections {
			if err := connection.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				failures = append(failures, fmt.Errorf("close interactive attachment connection: %w", err))
			}
		}
		a.transportErr = errors.Join(failures...)
	})
	return a.transportErr
}

func (a *interactiveWorkspaceAttachment) stopHeartbeat() {
	a.heartbeatOnce.Do(func() { close(a.heartbeatStop) })
}

func (a *interactiveWorkspaceAttachment) cleanupAuthority() {
	a.authorityOnce.Do(func() {
		defer close(a.authorityDone)
		cleanup, cancel := context.WithTimeout(context.Background(), permissionSessionCleanup)
		defer cancel()
		a.authorityErr = a.runtime.withInteractiveAttachmentLock(cleanup, func() error {
			var registry tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
				return err
			}
			current := findInteractiveSession(registry, a.session.WorkspaceManifestID, a.session.WorkspaceID)
			if current == nil || !sameInteractiveSessionAuthority(*current, a.session) {
				return fmt.Errorf("interactive attachment authority changed before cleanup")
			}
			kept := registry.Sessions[:0]
			for _, session := range registry.Sessions {
				if !sameInteractiveSessionAuthority(session, a.session) {
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
}

// failClosed is shared by heartbeat and accept failures. It first removes all
// transport capability, then invalidates wait observation, and finally removes
// only the exact authority record with an independent bounded context.
func (a *interactiveWorkspaceAttachment) failClosed() {
	a.stopHeartbeat()
	_ = a.closeTransport()
	if a.waits != nil {
		a.waits.Invalidate()
	}
	a.cleanupAuthority()
}

func (a *interactiveWorkspaceAttachment) Close(_ context.Context) error {
	if !a.owned {
		return nil
	}
	a.once.Do(func() {
		defer close(a.closeDone)
		a.failClosed()
		<-a.heartbeatDone
		<-a.authorityDone
		a.closeErr = errors.Join(a.transportErr, a.authorityErr)
	})
	<-a.closeDone
	return a.closeErr
}
