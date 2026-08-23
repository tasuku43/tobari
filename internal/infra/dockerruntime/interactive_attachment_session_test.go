package dockerruntime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
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
}

func registerPermissionWait(runtime *Runtime, session tobari.InteractiveAttachmentSession, record tobari.PermissionWaitRecord) (string, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	connection, err := net.DialTimeout("unix", runtime.interactiveAttachmentSocketPath(session), time.Second)
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
	if err := owner.renew(time.Now()); err == nil {
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
		IngestionSocket: "pws_0123456789abcdef0123456789abcdef.sock", IngestionNonce: strings.Repeat("d", 64), CreatedAt: now.Format(time.RFC3339Nano), LeaseIssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(20 * time.Second).Format(time.RFC3339Nano),
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
		if err := owner.renew(issued); err != nil {
			t.Fatalf("heartbeat %d: %v", heartbeat, err)
		}
		var registry tobari.InteractiveAttachmentSessionRegistry
		if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 {
			t.Fatalf("heartbeat %d registry = %+v, %v", heartbeat, registry, err)
		}
		if err := registry.Sessions[0].Validate(); err != nil || registry.Sessions[0].LeaseIssuedAt != issued.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("heartbeat %d lease = %+v, %v", heartbeat, registry.Sessions[0], err)
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
	if err := owner.renew(expires.Add(time.Nanosecond)); err == nil {
		t.Fatal("expired interactive attachment lease was resurrected")
	}
	var registry tobari.InteractiveAttachmentSessionRegistry
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 1 || registry.Sessions[0].ExpiresAt != owner.session.ExpiresAt {
		t.Fatalf("expired lease was rewritten: %+v, %v", registry, err)
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
	if err := owner.renew(time.Now().UTC()); err == nil {
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
			IngestionSocket: fmt.Sprintf("pws_%032x.sock", index+1), IngestionNonce: fmt.Sprintf("%064x", index+129),
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
