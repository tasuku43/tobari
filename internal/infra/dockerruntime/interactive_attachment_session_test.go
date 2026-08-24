package dockerruntime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func prepareInteractiveSessionPrincipal(t *testing.T, runtime *Runtime, workspace tobari.Workspace) projectPrincipalBinding {
	t.Helper()
	binding := projectPrincipalBinding{
		ProjectID: workspace.ID, WorkspaceManifestID: workspace.WorkspaceManifestID,
		WorkspaceManifestName: workspace.WorkspaceManifestName, ProjectRoot: workspace.Root,
		WorkspaceIP: "172.30.0.2", GatewayIP: "172.30.0.1", Network: "tobari-test-network",
	}
	if err := runtime.ensureProjectPrincipalRegistry(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.replaceProjectPrincipalRegistry(context.Background(), []projectPrincipalBinding{binding}); err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestInteractiveSessionOwnsWaitWithoutHostLoopbackStoreOrRoute(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	workspace := projectRuntimeInstance(t, runtime)
	binding := prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	if _, err := os.Lstat(runtime.hostLoopbackDirectory()); !os.IsNotExist(err) {
		t.Fatalf("permission session created Host Loopback store: %v", err)
	}
	if owner.session.OwnerKind != tobari.PermissionSessionOwnerInteractive || owner.session.FrozenPrincipalFingerprint != frozenPrincipalFingerprint(binding) {
		t.Fatalf("canonical owner session = %+v", owner.session)
	}
	directoryInfo, err := os.Lstat(runtime.interactiveAttachmentSocketDirectory())
	if err != nil || directoryInfo.Mode().Perm() != 0o700 || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("permission socket directory mode = %v, %v", directoryInfo, err)
	}
	socketInfo, err := os.Lstat(runtime.interactiveAttachmentSocketPath(owner.session))
	if err != nil || socketInfo.Mode().Perm() != 0o600 || socketInfo.Mode()&os.ModeSocket == 0 || socketInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("permission socket mode = %v, %v", socketInfo, err)
	}
	borrower, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer borrower.Close(context.Background())
	if borrower.owned || borrower.session.AttachmentID != owner.session.AttachmentID || borrower.session.IngestionNonce != owner.session.IngestionNonce {
		t.Fatalf("borrower session = %+v, owner = %+v", borrower.session, owner.session)
	}

	now := time.Now().UTC()
	record := permissionWaitRecordFixtureForInfra()
	record.FrozenPrincipalFingerprint = owner.session.FrozenPrincipalFingerprint
	record.WorkspaceManifestID = workspace.WorkspaceManifestID
	record.WorkspaceID = workspace.ID
	record.AttachmentID = owner.session.AttachmentID
	record.CreatedAt = now.Format(time.RFC3339Nano)
	record.ExpiresAt = now.Add(tobari.PermissionWaitLease).Format(time.RFC3339Nano)
	if ack, err := registerPermissionWait(runtime, owner.session, record); err != nil || ack != "OK" {
		t.Fatalf("registration without Host Loopback = %q, %v", ack, err)
	}
	owner.waits.mu.Lock()
	_, exists := owner.waits.records[record.ID]
	owner.waits.mu.Unlock()
	if !exists {
		t.Fatal("acknowledged wait was not retained")
	}
	owner.waits.observer = &dispositionObserverStub{
		results: []tobari.PermissionWaitResult{tobari.PermissionWaitResultAllow},
		done:    []bool{true},
	}
	client, err := newPermissionWaitOwnerClient(runtime, owner.session)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.WaitPermission(context.Background(), record.ID)
	if err != nil || result != tobari.PermissionWaitResultAllow {
		t.Fatalf("owner wait transport = %q, %v", result, err)
	}
}

func TestPermissionOwnerTransportCancellationRetainsReconnectBudget(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	now := time.Now().UTC()
	record := permissionWaitRecordFixtureForInfra()
	record.FrozenPrincipalFingerprint = owner.session.FrozenPrincipalFingerprint
	record.WorkspaceManifestID, record.WorkspaceID, record.AttachmentID = workspace.WorkspaceManifestID, workspace.ID, owner.session.AttachmentID
	record.CreatedAt, record.ExpiresAt = now.Format(time.RFC3339Nano), now.Add(tobari.PermissionWaitLease).Format(time.RFC3339Nano)
	if ack, err := registerPermissionWait(runtime, owner.session, record); err != nil || ack != "OK" {
		t.Fatalf("register = %q, %v", ack, err)
	}
	started := make(chan struct{})
	owner.waits.observer = &dispositionObserverStub{onCall: func() {
		select {
		case <-started:
		default:
			close(started)
		}
	}}
	client, _ := newPermissionWaitOwnerClient(runtime, owner.session)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, waitErr := client.WaitPermission(ctx, record.ID)
		done <- waitErr
	}()
	<-started
	cancel()
	if err := <-done; !hasInfrastructureFaultCode(err, "permission_wait_interrupted") {
		t.Fatalf("canceled owner wait = %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		owner.waits.mu.Lock()
		entry := owner.waits.records[record.ID]
		active := entry != nil && entry.access.Active
		attempts := 0
		if entry != nil {
			attempts = entry.access.Attempts
		}
		owner.waits.mu.Unlock()
		if !active {
			if attempts != 1 {
				t.Fatalf("canceled attempt count = %d", attempts)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("canceled wait remained active")
		}
		time.Sleep(time.Millisecond)
	}
	owner.waits.observer = &dispositionObserverStub{results: []tobari.PermissionWaitResult{tobari.PermissionWaitResultDeny}, done: []bool{true}}
	result, err := client.WaitPermission(context.Background(), record.ID)
	if err != nil || result != tobari.PermissionWaitResultDeny {
		t.Fatalf("reconnected owner wait = %q, %v", result, err)
	}
}

func TestLoopbackTCPInteractiveSessionOwnsWaitWithoutUnixMountSource(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	runtime.permissionIngestionTransport = tobari.PermissionSessionTransportTCP
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	if owner.session.IngestionTransport != tobari.PermissionSessionTransportTCP || !strings.HasPrefix(owner.session.IngestionEndpoint, "127.0.0.1:") {
		t.Fatalf("loopback owner endpoint = %+v", owner.session)
	}
	if _, err := os.Lstat(runtime.interactiveAttachmentSocketDirectory()); !os.IsNotExist(err) {
		t.Fatalf("loopback profile created a Unix mount source: %v", err)
	}
	registryDirectory, err := os.Lstat(runtime.interactiveAttachmentDirectory())
	if err != nil || !registryDirectory.IsDir() || registryDirectory.Mode().Perm() != 0o700 || registryDirectory.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("registry directory = %v, %v", registryDirectory, err)
	}
	registryFile, err := os.Lstat(runtime.interactiveAttachmentSessionRegistryPath())
	if err != nil || !registryFile.Mode().IsRegular() || registryFile.Mode().Perm() != 0o600 || registryFile.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("registry file = %v, %v", registryFile, err)
	}
	if !runtime.permissionSessionActive(owner.session) {
		t.Fatal("loopback TCP owner did not pass nonce-first liveness")
	}
	now := time.Now().UTC()
	record := permissionWaitRecordFixtureForInfra()
	record.FrozenPrincipalFingerprint = owner.session.FrozenPrincipalFingerprint
	record.WorkspaceManifestID, record.WorkspaceID, record.AttachmentID = workspace.WorkspaceManifestID, workspace.ID, owner.session.AttachmentID
	record.CreatedAt, record.ExpiresAt = now.Format(time.RFC3339Nano), now.Add(tobari.PermissionWaitLease).Format(time.RFC3339Nano)
	if ack, err := registerPermissionWait(runtime, owner.session, record); err != nil || ack != "OK" {
		t.Fatalf("loopback registration = %q, %v", ack, err)
	}
	mismatch := owner.session
	mismatch.IngestionTransport = tobari.PermissionSessionTransportUnix
	mismatch.IngestionEndpoint = "pws_0123456789abcdef0123456789abcdef.sock"
	if runtime.permissionSessionActive(mismatch) {
		t.Fatal("Unix record was accepted by loopback TCP support profile")
	}
}

func TestPermissionSessionPlatformSelectionIsClosed(t *testing.T) {
	if got := permissionSessionTransportForGOOS("linux"); got != tobari.PermissionSessionTransportUnix {
		t.Fatalf("Linux transport = %q", got)
	}
	if got := permissionSessionTransportForGOOS("darwin"); got != tobari.PermissionSessionTransportTCP {
		t.Fatalf("macOS transport = %q", got)
	}
	for _, unsupported := range []string{"windows", "freebsd", ""} {
		if got := permissionSessionTransportForGOOS(unsupported); got != "" {
			t.Fatalf("unsupported %q transport = %q", unsupported, got)
		}
	}
}

func TestInteractiveSessionRejectsUnsafeRegistrySource(t *testing.T) {
	for _, target := range []string{"directory", "file"} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
			workspace := projectRuntimeInstance(t, runtime)
			prepareInteractiveSessionPrincipal(t, runtime, workspace)
			if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
				t.Fatal(err)
			}
			path := runtime.interactiveAttachmentDirectory()
			mode := os.FileMode(0o755)
			if target == "file" {
				path = runtime.interactiveAttachmentSessionRegistryPath()
				mode = 0o644
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace); err == nil {
				t.Fatalf("unsafe registry %s was accepted", target)
			}
		})
	}
}

func TestInteractiveSessionRejectsUnsafeSocketDirectoryBeforeRegistryMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		create func(string) error
	}{
		{name: "broad mode", create: func(path string) error { return os.Mkdir(path, 0o755) }},
		{name: "symlink", create: func(path string) error { return os.Symlink(t.TempDir(), path) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
			workspace := projectRuntimeInstance(t, runtime)
			prepareInteractiveSessionPrincipal(t, runtime, workspace)
			socketDirectory := runtime.interactiveAttachmentSocketDirectory()
			if err := test.create(socketDirectory); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Remove(socketDirectory) })
			if _, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace); err == nil {
				t.Fatal("unsafe permission socket directory was accepted")
			}
			if _, err := os.Lstat(runtime.interactiveAttachmentDirectory()); !os.IsNotExist(err) {
				t.Fatalf("interactive registry mutated before socket directory rejection: %v", err)
			}
		})
	}
}

func TestPermissionSessionLivenessRejectsBroadAndSymlinkedSockets(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	path := runtime.interactiveAttachmentSocketPath(owner.session)
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("broad-mode permission socket was considered live")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	symlinked := owner.session
	symlinked.AttachmentID = "att_ffffffffffffffffffffffffffffffff"
	symlinked.IngestionEndpoint = "pws_ffffffffffffffffffffffffffffffff.sock"
	symlinkPath := runtime.interactiveAttachmentSocketPath(symlinked)
	if err := os.Symlink(path, symlinkPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(symlinkPath)
	if runtime.permissionSessionActive(symlinked) {
		t.Fatal("symlinked permission socket was considered live")
	}
}

func registerPermissionWait(runtime *Runtime, session tobari.InteractiveAttachmentSession, record tobari.PermissionWaitRecord) (string, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	connection, err := runtime.dialPermissionSession(session, time.Second)
	if err != nil {
		return "", err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(500 * time.Millisecond))
	frame := append([]byte("W"+session.IngestionNonce), make([]byte, 4)...)
	binary.BigEndian.PutUint32(frame[len(frame)-4:], uint32(len(payload)))
	frame = append(frame, payload...)
	if _, err := connection.Write(frame); err != nil {
		return "", err
	}
	ack := make([]byte, 2)
	_, err = io.ReadFull(connection, ack)
	return string(ack), err
}

func TestInteractiveSessionRejectsForgedNonceAndServiceControllerRecord(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	now := time.Now().UTC()
	record := permissionWaitRecordFixtureForInfra()
	record.FrozenPrincipalFingerprint = owner.session.FrozenPrincipalFingerprint
	record.WorkspaceManifestID, record.WorkspaceID, record.AttachmentID = workspace.WorkspaceManifestID, workspace.ID, owner.session.AttachmentID
	record.CreatedAt, record.ExpiresAt = now.Format(time.RFC3339Nano), now.Add(tobari.PermissionWaitLease).Format(time.RFC3339Nano)
	forged := owner.session
	forged.IngestionNonce = strings.Repeat("0", 64)
	if ack, err := registerPermissionWait(runtime, forged, record); err == nil || strings.Trim(ack, "\x00") != "" {
		t.Fatalf("forged registration = %q, %v", ack, err)
	}

	service := owner.session
	service.OwnerKind = "service_exposure_controller"
	registry := tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{service}}
	if err := registry.Validate(); err == nil {
		t.Fatal("service controller entered canonical interactive registry")
	}
}

func TestInteractiveSessionRetainsDriftedAuthorityAndClosesTransport(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
		t.Fatal(err)
	}
	registry.Sessions[0].IngestionNonce = strings.Repeat("f", 64)
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), registry); err != nil {
		t.Fatal(err)
	}
	if err := owner.renew(); err == nil {
		t.Fatal("same-PID record with replaced nonce was renewed")
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := owner.Close(canceled); err == nil {
		t.Fatal("drifted authority cleanup was reported as successful")
	}
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("interactive transport survived cleanup")
	}
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].IngestionNonce != strings.Repeat("f", 64) {
		t.Fatalf("foreign drifted authority was deleted: %+v, %v", registry, err)
	}
}

func TestInteractiveSessionFailsClosedOnLiveStaleEndpoint(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	binding := prepareInteractiveSessionPrincipal(t, runtime, workspace)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	stale := tobari.InteractiveAttachmentSession{
		SchemaVersion: tobari.PermissionSessionSchema, WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
		AttachmentID: "att_0123456789abcdef0123456789abcdef", OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: frozenPrincipalFingerprint(binding), OwnerPID: os.Getpid(),
		IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: "pws_0123456789abcdef0123456789abcdef.sock", IngestionNonce: strings.Repeat("d", 64), CreatedAt: now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(20 * time.Second).Format(time.RFC3339Nano),
	}
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
		SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{stale},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace); err == nil {
		t.Fatal("live-lease stale endpoint was silently replaced")
	}
}

func TestInteractiveSessionRenewsAcrossMultipleHeartbeats(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	created, _ := time.Parse(time.RFC3339Nano, owner.session.CreatedAt)
	for heartbeat := 1; heartbeat <= 3; heartbeat++ {
		issued := created.Add(time.Duration(heartbeat) * permissionSessionHeartbeat)
		owner.clock = func() time.Time { return issued }
		if err := owner.renew(); err != nil {
			t.Fatalf("heartbeat %d: %v", heartbeat, err)
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 {
			t.Fatalf("heartbeat %d registry = %+v, %v", heartbeat, registry, err)
		}
		if err := registry.Sessions[0].Validate(); err != nil || registry.Sessions[0].LeaseIssuedAt != issued.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("heartbeat %d lease = %+v, %v", heartbeat, registry.Sessions[0], err)
		}
		persistedExpiry, _ := time.Parse(time.RFC3339Nano, registry.Sessions[0].ExpiresAt)
		owner.waits.mu.Lock()
		memoryExpiry := owner.waits.ownerExpiry
		owner.waits.mu.Unlock()
		if !memoryExpiry.Equal(persistedExpiry) {
			t.Fatalf("heartbeat %d registry/in-memory expiry = %s / %s", heartbeat, persistedExpiry, memoryExpiry)
		}
	}
}

func TestInteractiveSessionCannotResurrectExpiredLease(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	expires, _ := time.Parse(time.RFC3339Nano, owner.session.ExpiresAt)
	owner.clock = func() time.Time { return expires.Add(time.Nanosecond) }
	if err := owner.renew(); err == nil {
		t.Fatal("expired interactive attachment lease was resurrected")
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].ExpiresAt != owner.session.ExpiresAt {
		t.Fatalf("expired lease was rewritten: %+v, %v", registry, err)
	}
}

func TestInteractiveSessionRejectsNonAdvancingOrRolledBackRenewal(t *testing.T) {
	for _, test := range []struct {
		name   string
		offset time.Duration
	}{
		{name: "same wall clock", offset: 0},
		{name: "wall clock rollback", offset: -time.Nanosecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
			workspace := projectRuntimeInstance(t, runtime)
			prepareInteractiveSessionPrincipal(t, runtime, workspace)
			owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
			if err != nil {
				t.Fatal(err)
			}
			defer owner.Close(context.Background())
			issued, _ := time.Parse(time.RFC3339Nano, owner.session.LeaseIssuedAt)
			owner.clock = func() time.Time { return issued.Add(test.offset) }
			if err := owner.renew(); err == nil {
				t.Fatal("non-advancing lease was renewed")
			}
			var registry tobari.InteractiveAttachmentSessionRegistry
			if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].LeaseIssuedAt != owner.session.LeaseIssuedAt {
				t.Fatalf("non-advancing renewal rewrote registry: %+v, %v", registry, err)
			}
		})
	}
}

func TestInteractiveSessionRenewIgnoresStaleTickerAfterProcessSuspension(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	expires, _ := time.Parse(time.RFC3339Nano, owner.session.ExpiresAt)
	// A ticker value queued before expiry is intentionally unavailable to the
	// renewal method; only this post-suspension wall-clock observation is used.
	owner.clock = func() time.Time { return expires.Add(time.Nanosecond) }
	if err := owner.renew(); err == nil {
		t.Fatal("stale pre-suspension ticker resurrected an expired owner")
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].ExpiresAt != owner.session.ExpiresAt {
		t.Fatalf("stale tick rewrote authority: %+v, %v", registry, err)
	}
}

func TestInteractiveSessionHeartbeatFailureShutsTransportAndAuthority(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Second)
	registry.Sessions[0].LeaseIssuedAt = expired.Add(-tobari.PermissionSessionLease).Format(time.RFC3339Nano)
	registry.Sessions[0].ExpiresAt = expired.Format(time.RFC3339Nano)
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), registry); err != nil {
		t.Fatal(err)
	}
	owner.clock = func() time.Time { return time.Now().UTC() }
	if err := owner.renew(); err == nil {
		t.Fatal("expired heartbeat renewed")
	}
	owner.failClosed()
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("transport remained active after heartbeat failure")
	}
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 0 {
		t.Fatalf("authority remained after heartbeat failure: %+v, %v", registry, err)
	}
	if _, err := owner.waits.WaitPermission(context.Background(), "pwt_0123456789abcdef0123456789abcdef"); !hasInfrastructureFaultCode(err, "permission_wait_owner_unavailable") {
		t.Fatalf("wait registry remained available: %v", err)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveSessionAcceptFailureUsesFailClosedShutdown(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.listener.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		var registry tobari.InteractiveAttachmentSessionRegistry
		err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry)
		if err == nil && len(registry.Sessions) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("authority remained after accept failure: %+v, %v", registry, err)
		}
		time.Sleep(time.Millisecond)
	}
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("transport remained active after accept failure")
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestInteractiveSessionBoundsPartialIngestionClients(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	connections := make([]net.Conn, 0, permissionSessionMaxClients)
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < permissionSessionMaxClients; index++ {
		connection, err := net.DialTimeout("unix", runtime.interactiveAttachmentSocketPath(owner.session), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		connections = append(connections, connection)
	}
	deadline := time.Now().Add(time.Second)
	for {
		owner.activeMu.Lock()
		active := len(owner.active)
		owner.activeMu.Unlock()
		if active == permissionSessionMaxClients {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("active partial clients = %d", active)
		}
		time.Sleep(time.Millisecond)
	}
	overflow, err := net.DialTimeout("unix", runtime.interactiveAttachmentSocketPath(owner.session), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer overflow.Close()
	if err := overflow.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := overflow.Write([]byte("S" + owner.session.IngestionNonce)); err == nil {
		var response [2]byte
		if _, err := io.ReadFull(overflow, response[:]); err == nil {
			t.Fatal("ninth partial ingestion client was served")
		}
	}
	deadline = time.Now().Add(time.Second)
	for {
		var registry tobari.InteractiveAttachmentSessionRegistry
		err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry)
		if err == nil && len(registry.Sessions) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("concurrency exhaustion left authority live: %+v, %v", registry, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestLoopbackPermissionSessionBoundsConnectionsPerLease(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	runtime.permissionIngestionTransport = tobari.PermissionSessionTransportTCP
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	invalidProbe := func() {
		connection, err := net.DialTimeout("tcp4", owner.session.IngestionEndpoint, time.Second)
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(100 * time.Millisecond))
		_, _ = connection.Write([]byte("S" + strings.Repeat("0", 64)))
		var response [2]byte
		_, _ = io.ReadFull(connection, response[:])
	}
	for attempt := 0; attempt < permissionSessionMaxAccepts; attempt++ {
		invalidProbe()
	}
	deadline := time.Now().Add(time.Second)
	for {
		var registry tobari.InteractiveAttachmentSessionRegistry
		err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry)
		if err == nil && len(registry.Sessions) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("invalid-connection budget left authority live: %+v, %v", registry, err)
		}
		time.Sleep(time.Millisecond)
	}
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("rate-exhausted endpoint remained live")
	}
}

func TestInteractiveSessionConcurrentFirstEntriesShareWinner(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	start := make(chan struct{})
	results := make(chan *interactiveWorkspaceAttachment, 2)
	errorsFound := make(chan error, 2)
	var workers sync.WaitGroup
	for index := 0; index < 2; index++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			attachment, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
			if err != nil {
				errorsFound <- err
				return
			}
			results <- attachment
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
	attachments := make([]*interactiveWorkspaceAttachment, 0, 2)
	for attachment := range results {
		attachments = append(attachments, attachment)
	}
	if len(attachments) != 2 || attachments[0].session.AttachmentID != attachments[1].session.AttachmentID || attachments[0].owned == attachments[1].owned {
		t.Fatalf("concurrent entries = %+v", attachments)
	}
	for _, attachment := range attachments {
		if err := attachment.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInteractiveSessionCompactsFullExpiredRegistry(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-2 * tobari.PermissionSessionLease)
	sessions := make([]tobari.InteractiveAttachmentSession, 128)
	for index := range sessions {
		sessions[index] = tobari.InteractiveAttachmentSession{
			SchemaVersion:       tobari.PermissionSessionSchema,
			WorkspaceManifestID: fmt.Sprintf("01912345-6789-7abc-8def-%012x", index+1),
			WorkspaceID:         fmt.Sprintf("01912345-6789-7abc-9def-%012x", index+1),
			AttachmentID:        fmt.Sprintf("att_%032x", index+1), OwnerKind: tobari.PermissionSessionOwnerInteractive,
			FrozenPrincipalFingerprint: fmt.Sprintf("%064x", index+1), OwnerPID: os.Getpid(),
			IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: fmt.Sprintf("pws_%032x.sock", index+1), IngestionNonce: fmt.Sprintf("%064x", index+129),
			CreatedAt: created.Format(time.RFC3339Nano), LeaseIssuedAt: created.Format(time.RFC3339Nano), ExpiresAt: created.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
		}
	}
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: sessions}); err != nil {
		t.Fatal(err)
	}
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].AttachmentID != owner.session.AttachmentID {
		t.Fatalf("compacted registry = %+v, %v", registry, err)
	}
}

func TestGlobalFinalSessionFenceReconcilesOnlyExactExpiredOwners(t *testing.T) {
	newSession := func(_ *Runtime, suffix string, expired bool) tobari.InteractiveAttachmentSession {
		created := time.Now().UTC().Add(-2 * tobari.PermissionSessionLease)
		issued := created
		if !expired {
			created = time.Now().UTC()
			issued = created
		}
		return tobari.InteractiveAttachmentSession{
			SchemaVersion:       tobari.PermissionSessionSchema,
			WorkspaceManifestID: "01912345-6789-7abc-8def-0123456789b2",
			WorkspaceID:         "01912345-6789-7abc-8def-0123456789b3",
			AttachmentID:        "att_" + strings.Repeat(suffix, 32), OwnerKind: tobari.PermissionSessionOwnerInteractive,
			FrozenPrincipalFingerprint: strings.Repeat(suffix, 64), OwnerPID: os.Getpid(),
			IngestionTransport: tobari.PermissionSessionTransportUnix,
			IngestionEndpoint:  "pws_" + strings.Repeat(suffix, 32) + ".sock",
			IngestionNonce:     strings.Repeat(suffix, 64),
			CreatedAt:          created.Format(time.RFC3339Nano), LeaseIssuedAt: issued.Format(time.RFC3339Nano),
			ExpiresAt: issued.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
		}
	}

	t.Run("missing registry is exact zero owner", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
		if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); err != nil {
			t.Fatalf("missing registry: %v", err)
		}
	})

	t.Run("expired refused endpoint is compacted", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
		if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
			t.Fatal(err)
		}
		session := newSession(runtime, "e", true)
		if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
			SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{session},
		}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); err != nil {
			t.Fatalf("compact expired owner: %v", err)
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 0 {
			t.Fatalf("compacted registry=%+v err=%v", registry, err)
		}
	})

	t.Run("expired live endpoint still blocks", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
		if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
			t.Fatal(err)
		}
		session := newSession(runtime, "d", true)
		listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: runtime.interactiveAttachmentSocketPath(session), Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		if err := os.Chmod(runtime.interactiveAttachmentSocketPath(session), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
			SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{session},
		}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); err == nil || !strings.Contains(err.Error(), "still live") {
			t.Fatalf("expired live owner error=%v", err)
		}
	})

	t.Run("current unresponsive owner is ambiguous", func(t *testing.T) {
		root := t.TempDir()
		runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
		if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
			t.Fatal(err)
		}
		session := newSession(runtime, "c", false)
		if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{
			SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{session},
		}); err != nil {
			t.Fatal(err)
		}
		if err := runtime.ConfirmNoFinalWorkspaceSessions(context.Background()); err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("current unresponsive owner error=%v", err)
		}
	})
}

func TestInteractiveSessionCompactionRemovesExactCrashedSocket(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	binding := prepareInteractiveSessionPrincipal(t, runtime, workspace)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-2 * tobari.PermissionSessionLease)
	expired := tobari.InteractiveAttachmentSession{
		SchemaVersion: tobari.PermissionSessionSchema, WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
		AttachmentID: "att_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: frozenPrincipalFingerprint(binding), OwnerPID: os.Getpid(),
		IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: "pws_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.sock", IngestionNonce: strings.Repeat("e", 64),
		CreatedAt: created.Format(time.RFC3339Nano), LeaseIssuedAt: created.Format(time.RFC3339Nano), ExpiresAt: created.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	socketPath := runtime.interactiveAttachmentSocketPath(expired)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(socketPath, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{expired}}); err != nil {
		t.Fatal(err)
	}
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	if _, err := os.Lstat(socketPath); !os.IsNotExist(err) {
		t.Fatalf("crashed expired socket remains after compaction: %v", err)
	}
}

func TestInteractiveSessionCompactionRetainsForeignExpiredSocketNode(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &recordingRunner{})
	workspace := projectRuntimeInstance(t, runtime)
	binding := prepareInteractiveSessionPrincipal(t, runtime, workspace)
	if err := runtime.ensureInteractiveAttachmentStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Add(-2 * tobari.PermissionSessionLease)
	expired := tobari.InteractiveAttachmentSession{
		SchemaVersion: tobari.PermissionSessionSchema, WorkspaceManifestID: workspace.WorkspaceManifestID, WorkspaceID: workspace.ID,
		AttachmentID: "att_dddddddddddddddddddddddddddddddd", OwnerKind: tobari.PermissionSessionOwnerInteractive,
		FrozenPrincipalFingerprint: frozenPrincipalFingerprint(binding), OwnerPID: os.Getpid(),
		IngestionTransport: tobari.PermissionSessionTransportUnix, IngestionEndpoint: "pws_dddddddddddddddddddddddddddddddd.sock", IngestionNonce: strings.Repeat("d", 64),
		CreatedAt: created.Format(time.RFC3339Nano), LeaseIssuedAt: created.Format(time.RFC3339Nano), ExpiresAt: created.Add(tobari.PermissionSessionLease).Format(time.RFC3339Nano),
	}
	socketPath := runtime.interactiveAttachmentSocketPath(expired)
	target := filepath.Join(t.TempDir(), "foreign")
	if err := os.WriteFile(target, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, socketPath); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(socketPath)
	if err := writeAtomicJSON(runtime.interactiveAttachmentSessionRegistryPath(), tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{expired}}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace); err == nil {
		t.Fatal("foreign expired socket node was silently compacted")
	}
	info, err := os.Lstat(socketPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("foreign socket node was changed: %v, %v", info, err)
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].AttachmentID != expired.AttachmentID {
		t.Fatalf("foreign socket authority was compacted: %+v, %v", registry, err)
	}
}

func TestServiceControllerStoreIsSeparateFromInteractiveSessionStore(t *testing.T) {
	root := t.TempDir()
	runtime, err := newRuntimeWithData(root+"/config", root+"/state", root+"/data", &serviceExposureRunner{})
	if err != nil {
		t.Fatal(err)
	}
	workspace := projectRuntimeInstance(t, runtime)
	prepareInteractiveSessionPrincipal(t, runtime, workspace)
	container, _, err := tobari.ProjectResourceNames(workspace.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := runtime.startWorkspaceServiceController(context.Background(), workspace, container)
	if err != nil {
		t.Fatal(err)
	}
	defer controller.Close(context.Background())
	owner, err := runtime.beginInteractiveWorkspaceAttachment(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(context.Background())
	if controller.attachmentID == owner.session.AttachmentID || strings.HasPrefix(controller.recordPath, runtime.interactiveAttachmentDirectory()+string(os.PathSeparator)) {
		t.Fatalf("service controller merged with permission owner: controller=%s owner=%s path=%s", controller.attachmentID, owner.session.AttachmentID, controller.recordPath)
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].AttachmentID != owner.session.AttachmentID {
		t.Fatalf("interactive registry contains service authority: %+v, %v", registry, err)
	}
}
