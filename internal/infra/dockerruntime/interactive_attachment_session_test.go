package dockerruntime

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
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
	if ack, err := registerPermissionWait(owner.session, record); err != nil || ack != "OK" {
		t.Fatalf("registration without Host Loopback = %q, %v", ack, err)
	}
	owner.waits.mu.Lock()
	_, exists := owner.waits.records[record.ID]
	owner.waits.mu.Unlock()
	if !exists {
		t.Fatal("acknowledged wait was not retained")
	}
}

func registerPermissionWait(session tobari.InteractiveAttachmentSession, record tobari.PermissionWaitRecord) (string, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(session.IngestionPort)), time.Second)
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
	if ack, err := registerPermissionWait(forged, record); err == nil || strings.Trim(ack, "\x00") != "" {
		t.Fatalf("forged registration = %q, %v", ack, err)
	}

	service := owner.session
	service.OwnerKind = "service_exposure_controller"
	registry := tobari.InteractiveAttachmentSessionRegistry{SchemaVersion: tobari.PermissionSessionSchema, Sessions: []tobari.InteractiveAttachmentSession{service}}
	if err := registry.Validate(); err == nil {
		t.Fatal("service controller entered canonical interactive registry")
	}
}

func TestInteractiveSessionRejectsPIDReuseStaleEndpointAndCanceledCleanup(t *testing.T) {
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
	if err := owner.Close(canceled); err != nil {
		t.Fatal(err)
	}
	if runtime.permissionSessionActive(owner.session) {
		t.Fatal("interactive transport survived cleanup")
	}
	if err := readStrictJSON(runtime.interactiveAttachmentSessionRegistryPath(), &registry); err != nil || len(registry.Sessions) != 0 {
		t.Fatalf("authority survived canceled cleanup: %+v, %v", registry, err)
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
		IngestionPort: 65534, IngestionNonce: strings.Repeat("d", 64), CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(20 * time.Second).Format(time.RFC3339Nano),
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
