package dockerruntime

import (
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
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const hostLoopbackHealthHandshakeBytes = 65

type hostLoopbackAttachment struct {
	runtime   *Runtime
	projectID string
	epochID   string
	route     tobari.AttachmentHostLoopbackRoute
	listener  net.Listener
	owned     bool
	active    map[net.Conn]struct{}
	activeMu  sync.Mutex
	closing   bool
	once      sync.Once
}

func (r *Runtime) hostLoopbackDirectory() string {
	return filepath.Join(r.configDirectory, "host-loopback")
}

func (r *Runtime) hostLoopbackRegistryPath() string {
	return filepath.Join(r.hostLoopbackDirectory(), "routes.json")
}

func (r *Runtime) attachmentGrantRegistryPath() string {
	return filepath.Join(r.hostLoopbackDirectory(), "grants.json")
}

func emptyHostLoopbackRegistry() tobari.HostLoopbackRegistry {
	return tobari.HostLoopbackRegistry{SchemaVersion: tobari.HostLoopbackRegistrySchema, Routes: []tobari.AttachmentHostLoopbackRoute{}}
}

func emptyAttachmentGrantRegistry() tobari.AttachmentGrantRegistry {
	return tobari.AttachmentGrantRegistry{SchemaVersion: tobari.HostLoopbackRegistrySchema, Grants: []tobari.AttachmentGrant{}}
}

func (r *Runtime) ensureHostLoopbackStore(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.ensurePrivateDirectory(r.hostLoopbackDirectory()); err != nil {
		return fmt.Errorf("prepare Host Loopback directory: %w", err)
	}
	for path, value := range map[string]any{
		r.hostLoopbackRegistryPath():    emptyHostLoopbackRegistry(),
		r.attachmentGrantRegistryPath(): emptyAttachmentGrantRegistry(),
	} {
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if err := initializeBytes(path, mustJSONBytes(value), 0o600); err != nil {
				return err
			}
		} else if err != nil {
			return fmt.Errorf("inspect Host Loopback store: %w", err)
		}
	}
	var routes tobari.HostLoopbackRegistry
	if err := readStrictJSON(r.hostLoopbackRegistryPath(), &routes); err != nil {
		return err
	}
	if err := routes.Validate(); err != nil {
		return err
	}
	var grants tobari.AttachmentGrantRegistry
	if err := readStrictJSON(r.attachmentGrantRegistryPath(), &grants); err != nil {
		return err
	}
	return grants.Validate()
}

func newAttachmentEpochID() (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate attachment epoch: %w", err)
	}
	return "att_" + hex.EncodeToString(raw), nil
}

func newHostLoopbackRelayToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", fmt.Errorf("generate Host Loopback relay token: %w", err)
	}
	return hex.EncodeToString(raw), nil
}

func (r *Runtime) withHostLoopbackLock(ctx context.Context, action func() error) error {
	return r.withConfigFileLock(ctx, "host-loopback.lock", "Host Loopback registry", action)
}

func (r *Runtime) beginHostLoopbackAttachment(
	ctx context.Context, project tobari.Workspace, epochID string,
) (*hostLoopbackAttachment, error) {
	if err := project.Validate(); err != nil {
		return nil, err
	}
	if err := tobari.ValidateAttachmentEpochID(epochID); err != nil {
		return nil, err
	}
	if err := r.ensureHostLoopbackStore(ctx); err != nil {
		return nil, err
	}
	token, err := newHostLoopbackRelayToken()
	if err != nil {
		return nil, err
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("listen for Host Loopback: %w", err)
	}
	route, err := tobari.NewAttachmentHostLoopbackRoute(epochID, project, listener.Addr().(*net.TCPAddr).Port, token)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	attachment := &hostLoopbackAttachment{
		runtime: r, projectID: project.ID, epochID: epochID, route: route,
		listener: listener, owned: true, active: map[net.Conn]struct{}{},
	}
	go attachment.serve()
	err = r.withHostLoopbackLock(ctx, func() error {
		var registry tobari.HostLoopbackRegistry
		if err := readStrictJSON(r.hostLoopbackRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		activeEpochs := map[string]struct{}{}
		retained := registry.Routes[:0]
		for _, existing := range registry.Routes {
			if r.hostLoopbackRelayActive(existing) {
				retained = append(retained, existing)
				activeEpochs[existing.EpochID] = struct{}{}
			}
		}
		registry.Routes = retained
		var grants tobari.AttachmentGrantRegistry
		if err := readStrictJSON(r.attachmentGrantRegistryPath(), &grants); err != nil {
			return err
		}
		if err := grants.Validate(); err != nil {
			return err
		}
		kept := grants.Grants[:0]
		for _, grant := range grants.Grants {
			if _, active := activeEpochs[grant.EpochID]; active {
				kept = append(kept, grant)
			}
		}
		grants.Grants = kept
		if err := writeAtomicJSON(r.attachmentGrantRegistryPath(), grants); err != nil {
			return err
		}
		for _, existing := range registry.Routes {
			if existing.ProjectID == project.ID {
				if existing.WorkspaceManifestID != project.WorkspaceManifestID || existing.EpochID != epochID {
					return fmt.Errorf("Host Loopback route does not belong to the canonical interactive attachment")
				}
				attachment.route = existing
				attachment.owned = false
				return writeAtomicJSON(r.hostLoopbackRegistryPath(), registry)
			}
		}
		registry.Routes = append(registry.Routes, route)
		sort.Slice(registry.Routes, func(i, j int) bool { return registry.Routes[i].ID < registry.Routes[j].ID })
		if err := registry.Validate(); err != nil {
			return err
		}
		return writeAtomicJSON(r.hostLoopbackRegistryPath(), registry)
	})
	if err != nil {
		attachment.closeRelay()
		return nil, err
	}
	if !attachment.owned {
		attachment.closeRelay()
	}
	return attachment, nil
}

func (a *hostLoopbackAttachment) serve() {
	for {
		connection, err := a.listener.Accept()
		if err != nil {
			return
		}
		if !a.track(connection) {
			_ = connection.Close()
			return
		}
		go a.relay(connection)
	}
}

func (a *hostLoopbackAttachment) track(connection net.Conn) bool {
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	if a.closing {
		return false
	}
	a.active[connection] = struct{}{}
	return true
}

func (a *hostLoopbackAttachment) untrack(connection net.Conn) {
	a.activeMu.Lock()
	delete(a.active, connection)
	a.activeMu.Unlock()
	_ = connection.Close()
}

func (a *hostLoopbackAttachment) relay(inbound net.Conn) {
	defer a.untrack(inbound)
	_ = inbound.SetDeadline(time.Now().Add(3 * time.Second))
	header := make([]byte, hostLoopbackHealthHandshakeBytes)
	if _, err := io.ReadFull(inbound, header); err != nil ||
		subtle.ConstantTimeCompare(header[1:], []byte(a.route.RelayToken)) != 1 ||
		(header[0] != 'C' && header[0] != 'P') {
		return
	}
	if header[0] == 'P' {
		_, _ = inbound.Write([]byte("OK"))
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(inbound, portBytes); err != nil {
		return
	}
	targetPort := int(binary.BigEndian.Uint16(portBytes))
	if err := tobari.ValidateHostLoopbackPort(targetPort); err != nil || !a.runtime.hostLoopbackPortAllowed(a.projectID, a.epochID, targetPort) {
		return
	}
	outbound, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(targetPort)), 3*time.Second)
	if err != nil || !a.track(outbound) {
		if outbound != nil {
			_ = outbound.Close()
		}
		return
	}
	defer a.untrack(outbound)
	if _, err := inbound.Write([]byte("OK")); err != nil {
		return
	}
	_ = inbound.SetDeadline(time.Time{})
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, _ = io.Copy(outbound, inbound)
		if tcp, ok := outbound.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer wait.Done()
		_, _ = io.Copy(inbound, outbound)
		if tcp, ok := inbound.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	wait.Wait()
}

func (r *Runtime) hostLoopbackPortAllowed(projectID, epochID string, targetPort int) bool {
	var routes tobari.HostLoopbackRegistry
	if err := readStrictJSON(r.hostLoopbackRegistryPath(), &routes); err != nil || routes.Validate() != nil {
		return false
	}
	active := false
	for _, route := range routes.Routes {
		active = active || (route.ProjectID == projectID && route.EpochID == epochID)
	}
	if !active {
		return false
	}
	var grants tobari.AttachmentGrantRegistry
	if err := readStrictJSON(r.attachmentGrantRegistryPath(), &grants); err != nil || grants.Validate() != nil {
		return false
	}
	for _, grant := range grants.Grants {
		if grant.ProjectID == projectID && grant.EpochID == epochID && grant.TargetPort == targetPort && grant.Decision == tobari.PolicyDecisionAllow {
			return true
		}
	}
	return false
}

func (a *hostLoopbackAttachment) closeRelay() {
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

func (r *Runtime) hostLoopbackRelayActive(route tobari.AttachmentHostLoopbackRoute) bool {
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(route.RelayPort)), 250*time.Millisecond)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(500 * time.Millisecond))
	if _, err := connection.Write([]byte("P" + route.RelayToken)); err != nil {
		return false
	}
	response := make([]byte, 2)
	_, err = io.ReadFull(connection, response)
	return err == nil && string(response) == "OK"
}

func (a *hostLoopbackAttachment) Close(_ context.Context) error {
	if !a.owned {
		return nil
	}
	var result error
	a.once.Do(func() {
		// Transport disappears before its attachment authority is deleted.
		a.closeRelay()
		cleanup, cancel := context.WithTimeout(context.Background(), permissionSessionCleanup)
		defer cancel()
		result = a.runtime.withHostLoopbackLock(cleanup, func() error {
			var routes tobari.HostLoopbackRegistry
			if err := readStrictJSON(a.runtime.hostLoopbackRegistryPath(), &routes); err != nil {
				return err
			}
			keptRoutes := routes.Routes[:0]
			for _, route := range routes.Routes {
				if route.EpochID != a.epochID {
					keptRoutes = append(keptRoutes, route)
				}
			}
			routes.Routes = keptRoutes
			if err := writeAtomicJSON(a.runtime.hostLoopbackRegistryPath(), routes); err != nil {
				return err
			}
			var grants tobari.AttachmentGrantRegistry
			if err := readStrictJSON(a.runtime.attachmentGrantRegistryPath(), &grants); err != nil {
				return err
			}
			keptGrants := grants.Grants[:0]
			for _, grant := range grants.Grants {
				if grant.EpochID != a.epochID {
					keptGrants = append(keptGrants, grant)
				}
			}
			grants.Grants = keptGrants
			return writeAtomicJSON(a.runtime.attachmentGrantRegistryPath(), grants)
		})
	})
	return result
}

func (r *Runtime) ApplyAttachmentGrantDecisionSet(
	ctx context.Context, grants []tobari.AttachmentGrant,
) (tobari.PolicyActivationReceipt, error) {
	if len(grants) == 0 || len(grants) > tobari.MaxPolicyReviewDecisions {
		return tobari.PolicyActivationReceipt{}, fmt.Errorf("attachment grant decision set is invalid")
	}
	var receipt tobari.PolicyActivationReceipt
	err := r.withHostLoopbackLock(ctx, func() error {
		var routes tobari.HostLoopbackRegistry
		if err := readStrictJSON(r.hostLoopbackRegistryPath(), &routes); err != nil {
			return err
		}
		if err := routes.Validate(); err != nil {
			return err
		}
		for _, grant := range grants {
			if err := grant.Validate(); err != nil {
				return err
			}
			active := false
			for _, route := range routes.Routes {
				active = active || (route.EpochID == grant.EpochID && route.ProjectID == grant.ProjectID && route.WorkspaceManifestID == grant.WorkspaceManifestID && route.Hostname == grant.Hostname)
			}
			if !active {
				return fmt.Errorf("attachment grant route is no longer active")
			}
		}
		var registry tobari.AttachmentGrantRegistry
		if err := readStrictJSON(r.attachmentGrantRegistryPath(), &registry); err != nil {
			return err
		}
		if err := registry.Validate(); err != nil {
			return err
		}
		for _, addition := range grants {
			kept := registry.Grants[:0]
			for _, existing := range registry.Grants {
				if existing.ProjectID == addition.ProjectID && existing.EpochID == addition.EpochID && existing.TargetPort == addition.TargetPort && existing.Method == addition.Method && existing.Path == addition.Path {
					continue
				}
				kept = append(kept, existing)
			}
			registry.Grants = append(kept, addition)
		}
		sort.Slice(registry.Grants, func(i, j int) bool { return registry.Grants[i].ID < registry.Grants[j].ID })
		if err := registry.Validate(); err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(registry, "", "  ")
		if err != nil {
			return err
		}
		if err := writeAtomicJSON(r.attachmentGrantRegistryPath(), registry); err != nil {
			return err
		}
		digest := sha256.Sum256(encoded)
		receipt = tobari.PolicyActivationReceipt{PolicyDirectory: r.hostLoopbackDirectory(), ActiveRevision: hex.EncodeToString(digest[:])}
		return receipt.Validate()
	})
	return receipt, err
}
