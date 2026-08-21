package dockerruntime

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func hostLoopbackGrant(t *testing.T, route tobari.AttachmentHostLoopbackRoute, port int) tobari.AttachmentGrant {
	t.Helper()
	denial := tobari.PolicyDenial{
		PolicyProtocolIdentity: tobari.PolicyProtocolIdentity{Scheme: "http", Protocol: tobari.PolicyProtocolHTTP},
		Timestamp:              "2026-08-17T12:00:00Z", RequestID: strings.Repeat("1", 32),
		ContextID: route.ContextID, ContextName: route.ContextName,
		ProjectID: route.ProjectID, ProjectRoot: route.ProjectRoot,
		Host: tobari.HostLoopbackHostname, Port: port, Method: "GET", Path: "/health",
		Reason: "review", StatusCode: 403, Learnable: true,
		DestinationKind: tobari.PolicyDestinationHostLoopback, AuthorityLifetime: tobari.AuthorityLifetimeAttachment,
		AttachmentEpochID: route.EpochID,
	}
	candidate, err := tobari.NewPolicyCandidate(denial)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := tobari.NewAttachmentGrantFromCandidate(tobari.PolicyDecisionAllow, candidate)
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func connectHostLoopback(route tobari.AttachmentHostLoopbackRoute, token string, port int) (net.Conn, error) {
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", route.RelayPort)), time.Second)
	if err != nil {
		return nil, err
	}
	handshake := append([]byte("C"+token), 0, 0)
	binary.BigEndian.PutUint16(handshake[len(handshake)-2:], uint16(port))
	if _, err := connection.Write(handshake); err != nil {
		_ = connection.Close()
		return nil, err
	}
	return connection, nil
}

func assertNoTargetDial(t *testing.T, target *net.TCPListener) {
	t.Helper()
	if err := target.SetDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if unexpected, err := target.Accept(); err == nil {
		_ = unexpected.Close()
		t.Fatal("unauthorized relay connection reached physical-host loopback")
	} else if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("target accept error = %v, want timeout", err)
	}
}

func TestHostLoopbackRelayRequiresTokenAndPortGrantBeforeTargetDial(t *testing.T) {
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	port := target.Addr().(*net.TCPAddr).Port
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "c"), filepath.Join(t.TempDir(), "s"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := runtime.beginHostLoopbackAttachment(context.Background(), projectRuntimeInstance(t, runtime))
	if err != nil {
		t.Fatal(err)
	}
	defer attachment.Close(context.Background())

	connection, err := connectHostLoopback(attachment.route, strings.Repeat("0", 64), port)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	assertNoTargetDial(t, target)

	connection, err = connectHostLoopback(attachment.route, attachment.route.RelayToken, port)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	assertNoTargetDial(t, target)

	grant := hostLoopbackGrant(t, attachment.route, port)
	if _, err := runtime.ApplyAttachmentGrantDecisionSet(context.Background(), []tobari.AttachmentGrant{grant}); err != nil {
		t.Fatal(err)
	}
	connection, err = connectHostLoopback(attachment.route, attachment.route.RelayToken, port+1)
	if err != nil {
		t.Fatal(err)
	}
	_ = connection.Close()
	assertNoTargetDial(t, target)

	connection, err = connectHostLoopback(attachment.route, attachment.route.RelayToken, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.SetDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	accepted, err := target.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer accepted.Close()
	ack := make([]byte, 2)
	if _, err := io.ReadFull(connection, ack); err != nil || string(ack) != "OK" {
		t.Fatalf("relay acknowledgement = %q, %v", ack, err)
	}
	_ = connection.Close()
}

func TestHostLoopbackConcurrentAttachmentBorrowsOwnerWithoutExtendingLifetime(t *testing.T) {
	runtime, err := newRuntime(filepath.Join(t.TempDir(), "c"), filepath.Join(t.TempDir(), "s"), &recordingRunner{})
	if err != nil {
		t.Fatal(err)
	}
	project := projectRuntimeInstance(t, runtime)
	owner, err := runtime.beginHostLoopbackAttachment(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	borrower, err := runtime.beginHostLoopbackAttachment(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	if !owner.owned || borrower.owned || borrower.epochID != owner.epochID {
		t.Fatalf("attachment ownership owner=%v borrower=%v epochs=%s/%s", owner.owned, borrower.owned, owner.epochID, borrower.epochID)
	}
	grant := hostLoopbackGrant(t, owner.route, 3000)
	if _, err := runtime.ApplyAttachmentGrantDecisionSet(context.Background(), []tobari.AttachmentGrant{grant}); err != nil {
		t.Fatal(err)
	}
	if err := borrower.Close(context.Background()); err != nil || !runtime.hostLoopbackRelayActive(owner.route) {
		t.Fatalf("borrower close affected owner: %v", err)
	}
	var activeGrants tobari.AttachmentGrantRegistry
	if err := readStrictJSON(runtime.attachmentGrantRegistryPath(), &activeGrants); err != nil {
		t.Fatal(err)
	}
	if len(activeGrants.Grants) != 1 || activeGrants.Grants[0].ID != grant.ID {
		t.Fatalf("borrower close changed owning attachment grants: %+v", activeGrants.Grants)
	}
	if err := owner.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if runtime.hostLoopbackRelayActive(owner.route) {
		t.Fatal("owner route remained active after owning attachment exited")
	}
	var routes tobari.HostLoopbackRegistry
	if err := readStrictJSON(runtime.hostLoopbackRegistryPath(), &routes); err != nil {
		t.Fatal(err)
	}
	if len(routes.Routes) != 0 {
		t.Fatalf("route survived owner exit: %+v", routes)
	}
	var grants tobari.AttachmentGrantRegistry
	if err := readStrictJSON(runtime.attachmentGrantRegistryPath(), &grants); err != nil {
		t.Fatal(err)
	}
	if len(grants.Grants) != 0 {
		t.Fatalf("attachment grant survived owner exit: %+v", grants.Grants)
	}
}
