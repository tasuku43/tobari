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
	permissionSessionMaxAccepts = 64
)

type interactiveWorkspaceAttachment struct {
	runtime       *Runtime
	lifetime      context.Context
	session       tobari.InteractiveAttachmentSession
	listener      net.Listener
	waits         *permissionWaitRegistry
	owned         bool
	heartbeatStop chan struct{}
	heartbeatDone chan struct{}
	activeMu      sync.Mutex
	active        map[net.Conn]struct{}
	accepted      int
	acceptWindow  time.Time
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
	clock         func() time.Time
}

// interactiveWorkspacePrincipal is the one private identity consumed by the
// canonical WP07 attachment registry. The frozen Gateway wire keeps the
// context_id/project_id/context spellings; final callers put ContextID,
// WorkspaceID, and Context presentation in those values. TemplateID is
// validated by the final binding before this projection and never becomes a
// frozen wire token or session selector.
type interactiveWorkspacePrincipal struct {
	contextID           string
	workspaceID         string
	contextPresentation string
	projectRoot         string
}

func (p interactiveWorkspacePrincipal) validate() error {
	if err := tobari.ValidateWorkspaceManifestID(p.contextID); err != nil {
		return fmt.Errorf("interactive attachment Context ID is invalid: %w", err)
	}
	if err := tobari.ValidateWorkspaceID(p.workspaceID); err != nil {
		return fmt.Errorf("interactive attachment Workspace ID is invalid: %w", err)
	}
	if err := tobari.ValidateName(p.contextPresentation); err != nil {
		return fmt.Errorf("interactive attachment Context presentation is invalid: %w", err)
	}
	if err := tobari.ValidateCanonicalRoot(p.projectRoot); err != nil {
		return fmt.Errorf("interactive attachment Project root is invalid: %w", err)
	}
	return nil
}

func legacyInteractiveWorkspacePrincipal(workspace tobari.Workspace) (interactiveWorkspacePrincipal, error) {
	if err := workspace.Validate(); err != nil {
		return interactiveWorkspacePrincipal{}, err
	}
	principal := interactiveWorkspacePrincipal{
		contextID: workspace.WorkspaceManifestID, workspaceID: workspace.ID,
		contextPresentation: workspace.WorkspaceManifestName, projectRoot: workspace.Root,
	}
	return principal, principal.validate()
}

func finalInteractiveWorkspacePrincipal(binding tobari.WorkspaceSessionBinding) (interactiveWorkspacePrincipal, error) {
	identity, err := binding.Identity()
	if err != nil {
		return interactiveWorkspacePrincipal{}, err
	}
	return finalInteractiveWorkspacePrincipalFromIdentity(identity)
}

func finalInteractiveWorkspacePrincipalFromIdentity(identity tobari.WorkspaceSessionIdentity) (interactiveWorkspacePrincipal, error) {
	if err := identity.Validate(); err != nil {
		return interactiveWorkspacePrincipal{}, err
	}
	principal := interactiveWorkspacePrincipal{
		contextID: string(identity.ContextID), workspaceID: string(identity.WorkspaceID),
		contextPresentation: identity.ContextPresentation, projectRoot: identity.ProjectRoot,
	}
	return principal, principal.validate()
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
	return filepath.Join(r.shortTemporaryDirectory, fmt.Sprintf("tobari-permission-%d-%x", os.Getuid(), digest[:8]))
}

func (r *Runtime) interactiveAttachmentSocketPath(session tobari.InteractiveAttachmentSession) string {
	return filepath.Join(r.interactiveAttachmentSocketDirectory(), session.IngestionEndpoint)
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
	if err := r.permissionIngestionTransport.Validate(); err != nil {
		return fmt.Errorf("select permission ingestion support profile: %w", err)
	}
	if r.permissionIngestionTransport == tobari.PermissionSessionTransportUnix {
		if err := ensurePermissionSocketDirectory(r.interactiveAttachmentSocketDirectory()); err != nil {
			return fmt.Errorf("prepare interactive attachment socket directory: %w", err)
		}
	}
	if err := ensurePermissionRegistryDirectory(r.interactiveAttachmentDirectory()); err != nil {
		return fmt.Errorf("prepare interactive attachment directory: %w", err)
	}
	return r.withInteractiveAttachmentLock(ctx, func() error {
		path := r.interactiveAttachmentSessionRegistryPath()
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if err := initializeBytes(path, mustJSONBytes(emptyInteractiveAttachmentSessionRegistry()), 0o600); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("inspect interactive attachment registry: %w", err)
		}
		if err := requireOwnerOnlyRegularFile(path); err != nil {
			return fmt.Errorf("validate interactive attachment registry: %w", err)
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(path, &registry); err != nil {
			return err
		}
		return registry.Validate()
	})
}

func ensurePermissionRegistryDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return requireOwnerOnlyPath(path, true)
}

func ensurePermissionSocketDirectory(path string) error {
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return requireOwnerOnlyPath(path, true)
}

func requireOwnerOnlyPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	wantMode := os.FileMode(0o600)
	if directory {
		wantMode = 0o700
	}
	if info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != wantMode ||
		(directory && !info.IsDir()) || (!directory && info.Mode()&os.ModeSocket == 0) {
		return fmt.Errorf("permission attachment path is not owner-only expected type")
	}
	ownerUID, ok := fileOwnerUID(info)
	if !ok || ownerUID != os.Getuid() {
		return fmt.Errorf("permission attachment path is not owned by the current host user")
	}
	return nil
}

func requireOwnerOnlyRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("permission attachment registry is not an owner-only regular file")
	}
	ownerUID, ok := fileOwnerUID(info)
	if !ok || ownerUID != os.Getuid() {
		return fmt.Errorf("permission attachment registry is not owned by the current host user")
	}
	return nil
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

func (r *Runtime) exactFrozenPrincipalFingerprint(principal interactiveWorkspacePrincipal) (string, error) {
	if err := principal.validate(); err != nil {
		return "", err
	}
	registry, err := r.readProjectPrincipalRegistry()
	if err != nil {
		return "", err
	}
	matches := make([]projectPrincipalBinding, 0, 1)
	for _, binding := range registry.Bindings {
		if binding.WorkspaceManifestID == principal.contextID && binding.ProjectID == principal.workspaceID {
			matches = append(matches, binding)
		}
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("canonical interactive attachment principal join is not unique")
	}
	if matches[0].WorkspaceManifestName != principal.contextPresentation || matches[0].ProjectRoot != principal.projectRoot {
		return "", fmt.Errorf("canonical interactive attachment principal projection is stale")
	}
	return frozenPrincipalFingerprint(matches[0]), nil
}

func sameInteractiveSessionAuthority(left, right tobari.InteractiveAttachmentSession) bool {
	return left.SameAuthority(right)
}

func permissionSessionLeaseCurrent(session tobari.InteractiveAttachmentSession, now time.Time) bool {
	if err := session.Validate(); err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	return err == nil && now.Before(expires)
}

// cleanupExpiredPermissionSessionSocket reconciles only the exact socket named
// by a validated expired session. Live, malformed, replaced, or foreign nodes
// are evidence of ambiguous authority and remain untouched.
func (r *Runtime) cleanupExpiredPermissionSessionSocket(session tobari.InteractiveAttachmentSession) error {
	if session.IngestionTransport == tobari.PermissionSessionTransportTCP {
		return nil
	}
	if session.IngestionTransport != tobari.PermissionSessionTransportUnix {
		return fmt.Errorf("expired interactive attachment transport is invalid")
	}
	path := r.interactiveAttachmentSocketPath(session)
	if filepath.Dir(path) != r.interactiveAttachmentSocketDirectory() || filepath.Base(path) != session.IngestionEndpoint {
		return fmt.Errorf("expired interactive attachment socket is outside its owner directory")
	}
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect expired interactive attachment socket: %w", err)
	}
	if err := requireOwnerOnlyPath(path, false); err != nil {
		return fmt.Errorf("expired interactive attachment socket is unsafe: %w", err)
	}
	connection, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
	if dialErr == nil {
		_ = connection.Close()
		return fmt.Errorf("expired interactive attachment socket is still live")
	}
	if !isConnectionRefused(dialErr) && !errors.Is(dialErr, os.ErrNotExist) {
		return fmt.Errorf("expired interactive attachment socket state is ambiguous: %w", dialErr)
	}
	after, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !os.SameFile(before, after) {
		return fmt.Errorf("expired interactive attachment socket changed during cleanup")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove expired interactive attachment socket: %w", err)
	}
	return nil
}

// reconcileExpiredInteractiveSessionsLocked is the single canonical
// registry-wide owner reconciliation used by attachment startup and global
// Gateway replacement. The caller holds the interactive-attachment lock.
// Current rows must prove a responsive owner and always block global mutation;
// an unresponsive current row is ambiguous. Expired rows are removable only
// when their exact transport endpoint is absent or refused.
func (r *Runtime) reconcileExpiredInteractiveSessionsLocked(
	registry *tobari.InteractiveAttachmentSessionRegistry, now time.Time,
) (bool, error) {
	if registry == nil {
		return false, fmt.Errorf("interactive attachment registry is required")
	}
	if err := registry.Validate(); err != nil {
		return false, err
	}
	kept := registry.Sessions[:0]
	changed := false
	for _, session := range registry.Sessions {
		if permissionSessionLeaseCurrent(session, now) {
			if !r.permissionSessionActive(session) {
				return false, fmt.Errorf("current interactive attachment owner is unresponsive or ambiguous")
			}
			kept = append(kept, session)
			continue
		}
		connection, dialErr := r.dialPermissionSession(session, 100*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return false, fmt.Errorf("expired interactive attachment endpoint is still live")
		}
		if !isConnectionRefused(dialErr) && !errors.Is(dialErr, os.ErrNotExist) {
			return false, fmt.Errorf("expired interactive attachment endpoint is ambiguous: %w", dialErr)
		}
		if err := r.cleanupExpiredPermissionSessionSocket(session); err != nil {
			return false, err
		}
		changed = true
	}
	registry.Sessions = kept
	if err := registry.Validate(); err != nil {
		return false, err
	}
	return changed, nil
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

// confirmExactInteractiveWorkspaceAttachment closes the lifecycle-lock release
// boundary before session-owned side effects start. A different valid owner is
// still a mismatch: the caller must retain the exact epoch/nonce/lease/wait
// authority acquired under the installation lifecycle lock.
func (r *Runtime) confirmExactInteractiveWorkspaceAttachment(
	ctx context.Context, principal interactiveWorkspacePrincipal, expected tobari.InteractiveAttachmentSession,
) error {
	if err := principal.validate(); err != nil {
		return err
	}
	if err := expected.Validate(); err != nil {
		return err
	}
	if expected.WorkspaceManifestID != principal.contextID || expected.WorkspaceID != principal.workspaceID ||
		!permissionSessionLeaseCurrent(expected, time.Now()) {
		return fmt.Errorf("canonical interactive attachment no longer matches its final principal")
	}
	verifyPair := func() error {
		fingerprint, err := r.exactFrozenPrincipalFingerprint(principal)
		if err != nil {
			return err
		}
		if expected.FrozenPrincipalFingerprint != fingerprint {
			return fmt.Errorf("canonical interactive attachment principal changed")
		}
		return r.withInteractiveAttachmentLock(ctx, func() error {
			var registry tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(r.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
				return err
			}
			if err := registry.Validate(); err != nil {
				return err
			}
			current := findInteractiveSession(registry, principal.contextID, principal.workspaceID)
			if current == nil || !sameInteractiveSessionAuthority(*current, expected) ||
				!permissionSessionLeaseCurrent(*current, time.Now()) {
				return fmt.Errorf("canonical interactive attachment owner changed")
			}
			return nil
		})
	}
	if err := verifyPair(); err != nil {
		return err
	}
	if !r.permissionSessionActive(expected) {
		return fmt.Errorf("canonical interactive attachment owner is unavailable")
	}
	if r.finalSessionAfterLiveness != nil {
		r.finalSessionAfterLiveness()
	}
	return verifyPair()
}

func (r *Runtime) beginInteractiveWorkspaceAttachment(ctx context.Context, workspace tobari.Workspace) (*interactiveWorkspaceAttachment, error) {
	principal, err := legacyInteractiveWorkspacePrincipal(workspace)
	if err != nil {
		return nil, err
	}
	return r.beginInteractiveWorkspaceAttachmentForPrincipal(ctx, principal)
}

func (r *Runtime) beginInteractiveWorkspaceAttachmentForPrincipal(ctx context.Context, principal interactiveWorkspacePrincipal) (*interactiveWorkspaceAttachment, error) {
	if err := principal.validate(); err != nil {
		return nil, err
	}
	if err := r.ensureInteractiveAttachmentStore(ctx); err != nil {
		return nil, err
	}
	fingerprint, err := r.exactFrozenPrincipalFingerprint(principal)
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
		changed, err := r.reconcileExpiredInteractiveSessionsLocked(&registry, time.Now())
		if err != nil {
			return err
		}
		for _, session := range registry.Sessions {
			if session.WorkspaceManifestID == principal.contextID && session.WorkspaceID == principal.workspaceID {
				copy := session
				existing = &copy
			}
		}
		if changed {
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
	listener, endpoint, err := r.listenPermissionSession(epochID)
	if err != nil {
		return nil, fmt.Errorf("listen for interactive attachment: %w", err)
	}
	if r.permissionIngestionTransport == tobari.PermissionSessionTransportUnix {
		path := filepath.Join(r.interactiveAttachmentSocketDirectory(), endpoint)
		if err := os.Chmod(path, 0o600); err != nil { // #nosec G302 -- same-host owner-only ingestion socket.
			closeErr := listener.Close()
			return nil, errors.Join(fmt.Errorf("protect interactive attachment socket: %w", err), closeErr)
		}
	}
	now := time.Now().UTC()
	session := tobari.InteractiveAttachmentSession{
		SchemaVersion:       tobari.PermissionSessionSchema,
		WorkspaceManifestID: principal.contextID, WorkspaceID: principal.workspaceID,
		AttachmentID: epochID, OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: fingerprint, OwnerPID: os.Getpid(),
		IngestionTransport: r.permissionIngestionTransport, IngestionEndpoint: endpoint, IngestionNonce: nonce,
		CreatedAt: now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	if err := session.Validate(); err != nil {
		return nil, errors.Join(err, listener.Close())
	}
	attachment := &interactiveWorkspaceAttachment{
		runtime: r, lifetime: r.lifetimeParent(ctx), session: session, listener: listener, owned: true,
		active: map[net.Conn]struct{}{}, heartbeatStop: make(chan struct{}), heartbeatDone: make(chan struct{}), authorityDone: make(chan struct{}), closeDone: make(chan struct{}),
		clock: time.Now, acceptWindow: now,
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
		if current := findInteractiveSession(registry, principal.contextID, principal.workspaceID); current != nil {
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
		tracked, exhausted := a.track(connection)
		if !tracked {
			_ = connection.Close()
			if exhausted {
				a.failClosed()
				return
			}
			continue
		}
		go a.handle(connection)
	}
}

func (a *interactiveWorkspaceAttachment) track(connection net.Conn) (bool, bool) {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	if a.closing {
		return false, false
	}
	if len(a.active) >= permissionSessionMaxClients {
		return false, true
	}
	now := time.Now()
	if now.Before(a.acceptWindow) {
		return false, true
	}
	if now.Sub(a.acceptWindow) >= tobari.PermissionSessionLease {
		a.acceptWindow = now
		a.accepted = 0
	}
	a.accepted++
	if a.accepted >= permissionSessionMaxAccepts {
		return false, true
	}
	a.active[connection] = struct{}{}
	return true, false
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
		(header[0] != 'S' && header[0] != 'W' && header[0] != 'Q') ||
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
	if header[0] == 'Q' {
		a.handlePermissionWaitQuery(connection, payload)
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
	if err := session.Validate(); err != nil {
		return false
	}
	connection, err := r.dialPermissionSession(session, 250*time.Millisecond)
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
			case <-ticker.C:
				if err := a.renew(); err != nil {
					a.failClosed()
					return
				}
			}
		}
	}()
}

func (a *interactiveWorkspaceAttachment) renew() error {
	ctx, cancel := context.WithTimeout(a.lifetime, 2*time.Second)
	defer cancel()
	var issued, expires time.Time
	if err := a.runtime.withInteractiveAttachmentLock(ctx, func() error {
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
			return err
		}
		// The clock observation belongs inside the authority mutation lock. A
		// ticker timestamp captured before suspension must never revive a lease.
		issued = a.clock().UTC()
		expires = issued.Add(tobari.PermissionSessionLease)
		current := findInteractiveSession(registry, a.session.WorkspaceManifestID, a.session.WorkspaceID)
		if current == nil || !sameInteractiveSessionAuthority(*current, a.session) || !permissionSessionLeaseCurrent(*current, issued) {
			return fmt.Errorf("interactive attachment owner changed")
		}
		renewed := *current
		renewed.LeaseIssuedAt = issued.Format(time.RFC3339Nano)
		renewed.ExpiresAt = expires.Format(time.RFC3339Nano)
		if err := renewed.ValidateRenewal(*current); err != nil {
			return err
		}
		for index := range registry.Sessions {
			if registry.Sessions[index].AttachmentID == a.session.AttachmentID {
				registry.Sessions[index] = renewed
			}
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		return writeAtomicJSON(a.runtime.interactiveAttachmentSessionRegistryPath(), registry)
	}); err != nil {
		return err
	}
	if err := a.waits.RenewOwner(issued, expires); err != nil {
		return err
	}
	return nil
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
		cleanup, cancel := context.WithTimeout(a.lifetime, permissionSessionCleanup)
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
