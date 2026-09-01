package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type serviceExposureRunner struct{ recordingRunner }

type fixedServiceBrowser struct {
	outcome tobari.ServiceOpenOutcome
	targets []string
}

func (b *fixedServiceBrowser) Dispatch(_ context.Context, target string) tobari.ServiceOpenOutcome {
	b.targets = append(b.targets, target)
	return b.outcome
}

func (r *serviceExposureRunner) RunWorkspaceServiceControl(ctx context.Context, _ []string, _ []string, in io.Reader, out, _ io.Writer) error {
	if _, err := io.WriteString(out, `{"schema_version":1,"ready":true}`+"\n"); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, in); close(done) }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (r *serviceExposureRunner) RunWorkspaceServiceStream(ctx context.Context, args []string, _ []string, in io.Reader, out, _ io.Writer) error {
	port, err := strconv.Atoi(args[len(args)-1])
	if err != nil {
		return err
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return err
	}
	defer connection.Close()
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		_, _ = io.Copy(connection, in)
		if tcp, ok := connection.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() { defer wait.Done(); _, _ = io.Copy(out, connection) }()
	wait.Wait()
	return ctx.Err()
}

func TestWorkspaceServiceOwnerRendezvousExactAuthorityAndTeardown(t *testing.T) {
	base := t.TempDir()
	runner := &serviceExposureRunner{}
	runtime, err := newRuntimeWithData(base+"/config", base+"/state", base+"/data", runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := runtime.startWorkspaceServiceController(context.Background(), instance, container)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	if _, err := os.Lstat(controller.recordPath); err != nil {
		t.Fatalf("atomic owner record: %v", err)
	}

	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	reached := make(chan string, 2)
	go func() {
		for {
			connection, err := target.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				header, _ := readServiceHeader(bufio.NewReader(connection))
				reached <- string(header)
				_, _ = io.WriteString(connection, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nOK")
			}()
		}
	}()
	clientID := strings.Repeat("a", 32)
	controller.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: clientID, Port: target.Addr().(*net.TCPAddr).Port})
	anchor, err := runtime.anchorServiceOwners(context.Background())
	if err != nil || len(anchor.records) != 1 {
		t.Fatalf("owner anchor=%+v err=%v", anchor, err)
	}
	if _, err := runtime.callServiceOwner(context.Background(), anchor.records[0], serviceRendezvousRequest{Operation: "snapshot"}); err != nil {
		t.Fatalf("owner snapshot call: %v", err)
	}
	pending, err := runtime.ReviewServiceRequests(context.Background())
	if err != nil || len(pending.Requests) != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	exposure, err := runtime.AllowServiceRequest(context.Background(), pending.Requests[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	wrong, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(wrong, "GET / HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", exposure.HostPort)
	_ = wrong.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, _ = io.ReadAll(wrong)
	wrong.Close()
	select {
	case value := <-reached:
		t.Fatalf("wrong authority reached Workspace: %q", value)
	default:
	}

	valid, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(valid, "GET /ready HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", testServiceAuthority(t, exposure))
	response, _ := io.ReadAll(valid)
	valid.Close()
	if !strings.Contains(string(response), "200 OK") || !strings.HasSuffix(string(response), "OK") {
		t.Fatalf("response=%q", response)
	}
	select {
	case line := <-reached:
		if !strings.HasPrefix(line, "GET /ready HTTP/1.1\r\n") || !strings.Contains(line, "\r\nHost: "+testServiceAuthority(t, exposure)+"\r\n") {
			t.Fatalf("request=%q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("exact authority did not reach Workspace")
	}

	record := serviceRendezvousRecord{SchemaVersion: 1, AttachmentID: controller.attachmentID, Nonce: controller.nonce, SocketName: filepath.Base(controller.rendezvousSocket), OwnerPID: os.Getpid()}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.callServiceOwner(context.Background(), record, serviceRendezvousRequest{Operation: "snapshot"}); err == nil {
		t.Fatal("owner exit retained review authority")
	}
	if _, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), 100*time.Millisecond); err == nil {
		t.Fatal("owner exit retained listener")
	}
	if _, err := os.Lstat(controller.recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owner record remains: %v", err)
	}
}

func newTestServiceController(t *testing.T, runner *serviceExposureRunner) (*Runtime, *workspaceServiceController) {
	t.Helper()
	base := t.TempDir()
	runtime, err := newRuntimeWithData(base+"/config", base+"/state", base+"/data", runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	container, _, err := tobari.ProjectResourceNames(instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	controller, err := runtime.startWorkspaceServiceController(context.Background(), instance, container)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close(context.Background()) })
	return runtime, controller
}

func allowTestService(t *testing.T, runtime *Runtime, controller *workspaceServiceController, port int) tobari.ServiceExposure {
	t.Helper()
	controller.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: strings.Repeat("b", 32), Port: port})
	pending, err := runtime.ReviewServiceRequests(context.Background())
	if err != nil || len(pending.Requests) != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	exposure, err := runtime.AllowServiceRequest(context.Background(), pending.Requests[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	return exposure
}

func testServiceAuthority(t *testing.T, exposure tobari.ServiceExposure) string {
	t.Helper()
	label, port, err := tobari.ParseServiceExposureURL(exposure.URL)
	if err != nil || port != exposure.HostPort {
		t.Fatalf("exposure URL=%q port=%d err=%v", exposure.URL, exposure.HostPort, err)
	}
	return "svc-" + label + ".localhost:" + strconv.Itoa(port)
}

func TestWorkspaceServiceHelperCancellationClosesControlConnection(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "tobari-svc-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	socket := filepath.Join(directory, "service.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	disconnected := make(chan error, 1)
	requestReceived := make(chan struct{})
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			disconnected <- err
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		if _, err := reader.ReadBytes('\n'); err != nil {
			disconnected <- err
			return
		}
		close(requestReceived)
		_, err = reader.ReadByte()
		disconnected <- err
	}()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := (&ServiceExposureClient{socket: socket}).RequestService(ctx, 3000)
		result <- err
	}()
	select {
	case <-requestReceived:
	case <-time.After(time.Second):
		t.Fatal("helper request did not reach control socket")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("cancelled helper request succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled helper request remained blocked")
	}
	select {
	case err := <-disconnected:
		if err == nil {
			t.Fatal("server did not observe helper disconnect")
		}
	case <-time.After(time.Second):
		t.Fatal("helper cancellation retained the control connection")
	}
}

func TestWorkspaceServiceReturnsFixed502AndRecordsPassiveUnavailableState(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	temporary, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := temporary.Addr().(*net.TCPAddr).Port
	temporary.Close()
	exposure := allowTestService(t, runtime, controller, port)
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(connection, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", testServiceAuthority(t, exposure))
	response, _ := io.ReadAll(connection)
	connection.Close()
	if string(response) != string(workspaceServiceUnavailableResponse) || bytes.Contains(response, []byte(strconv.Itoa(port))) {
		t.Fatalf("502 response=%q", response)
	}
	list := controller.snapshotExposures()
	if len(list.Exposures) != 1 || list.Exposures[0].State != tobari.ServiceStateUnavailable {
		t.Fatalf("passive state=%+v", list)
	}
}

func TestWorkspaceServiceRejectsAmbiguousHeadersAndWrongKeepaliveAuthority(t *testing.T) {
	authority := "svc-0123456789abcdef0123456789abcdef.localhost:54321"
	for name, raw := range map[string]string{
		"absent host":          "GET / HTTP/1.1\r\nUser-Agent: test\r\n\r\n",
		"duplicate host":       "GET / HTTP/1.1\r\nHost: " + authority + "\r\nHost: " + authority + "\r\n\r\n",
		"transfer plus length": "POST / HTTP/1.1\r\nHost: " + authority + "\r\nTransfer-Encoding: chunked\r\nContent-Length: 4\r\n\r\n",
		"folded":               "GET / HTTP/1.1\r\nHost: " + authority + "\r\n X: y\r\n\r\n",
		"absolute mismatch":    "GET http://localhost:54321/ HTTP/1.1\r\nHost: " + authority + "\r\n\r\n",
		"numeric loopback":     "GET / HTTP/1.1\r\nHost: 127.0.0.1:54321\r\n\r\n",
		"bare localhost":       "GET / HTTP/1.1\r\nHost: localhost:54321\r\n\r\n",
		"sibling origin":       "GET / HTTP/1.1\r\nHost: svc-ffffffffffffffffffffffffffffffff.localhost:54321\r\n\r\n",
		"wrong port":           "GET / HTTP/1.1\r\nHost: svc-0123456789abcdef0123456789abcdef.localhost:54322\r\n\r\n",
		"noncanonical case":    "GET / HTTP/1.1\r\nHost: SVC-0123456789ABCDEF0123456789ABCDEF.LOCALHOST:54321\r\n\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			header := []byte(raw)
			request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(header)))
			if err == nil && validateServiceRequestHeader(header, request, authority) == nil {
				t.Fatal("ambiguous request passed")
			}
		})
	}
	exactAbsolute := []byte("GET http://" + authority + "/ready HTTP/1.1\r\nHost: " + authority + "\r\n\r\n")
	request, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(exactAbsolute)))
	if err != nil || validateServiceRequestHeader(exactAbsolute, request, authority) != nil {
		t.Fatalf("exact absolute-form authority was rejected: %v", err)
	}

	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	second := make(chan string, 1)
	go func() {
		connection, err := target.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		_, _ = readServiceHeader(reader)
		_, _ = io.WriteString(connection, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
		line, _ := reader.ReadString('\n')
		second <- line
	}()
	exposure := allowTestService(t, runtime, controller, target.Addr().(*net.TCPAddr).Port)
	host, _ := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	_, _ = fmt.Fprintf(host, "GET /one HTTP/1.1\r\nHost: %s\r\n\r\nGET /two HTTP/1.1\r\nHost: localhost:%d\r\n\r\n", testServiceAuthority(t, exposure), exposure.HostPort)
	reader := bufio.NewReader(host)
	_, _ = readServiceHeader(reader)
	body := make([]byte, 2)
	_, _ = io.ReadFull(reader, body)
	host.Close()
	select {
	case line := <-second:
		if line != "" {
			t.Fatalf("wrong second authority reached Workspace: %q", line)
		}
	case <-time.After(time.Second):
		t.Fatal("Workspace stream did not close after wrong keepalive authority")
	}
}

func TestWorkspaceServiceWebSocketUpgradeRelaysBoundedBackpressureWithoutLoss(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		connection, err := target.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		reader := bufio.NewReader(connection)
		_, _ = readServiceHeader(reader)
		_, _ = io.WriteString(connection, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		_, _ = io.Copy(connection, reader)
	}()
	exposure := allowTestService(t, runtime, controller, target.Addr().(*net.TCPAddr).Port)
	host, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	_ = host.SetDeadline(time.Now().Add(10 * time.Second))
	_, _ = fmt.Fprintf(host, "GET /socket HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n", testServiceAuthority(t, exposure))
	header, err := readServiceHeader(bufio.NewReader(host))
	if err != nil || !bytes.Contains(header, []byte("101 Switching Protocols")) {
		t.Fatalf("upgrade=%q err=%v", header, err)
	}
	payload := bytes.Repeat([]byte("tobari-backpressure-"), (4<<20)/len("tobari-backpressure-"))
	writeDone := make(chan error, 1)
	go func() { _, err := host.Write(payload); writeDone <- err }()
	received := make([]byte, len(payload))
	if _, err := io.ReadFull(host, received); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil || !bytes.Equal(received, payload) {
		t.Fatalf("write=%v equal=%t", err, bytes.Equal(received, payload))
	}
}

func TestWorkspaceServicePendingCancellationDenyAndForeignStopCreateNoListener(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	client := strings.Repeat("c", 32)
	controller.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: client, Port: 3000})
	controller.withdrawClient(client)
	requests, err := runtime.ReviewServiceRequests(context.Background())
	if err != nil || len(requests.Requests) != 0 || len(controller.snapshotExposures().Exposures) != 0 {
		t.Fatalf("withdraw requests=%+v exposures=%+v err=%v", requests, controller.snapshotExposures(), err)
	}
	controller.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: client, Port: 3001})
	requests, _ = runtime.ReviewServiceRequests(context.Background())
	if len(requests.Requests) != 1 {
		t.Fatalf("deny setup=%+v", requests)
	}
	if err := runtime.DenyServiceRequest(context.Background(), requests.Requests[0].ID); err != nil {
		t.Fatal(err)
	}
	requests, _ = runtime.ReviewServiceRequests(context.Background())
	if len(requests.Requests) != 0 || len(controller.snapshotExposures().Exposures) != 0 {
		t.Fatalf("deny retained authority: requests=%+v exposures=%+v", requests, controller.snapshotExposures())
	}

	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	exposure := allowTestService(t, runtime, controller, target.Addr().(*net.TCPAddr).Port)
	if err := controller.stop("exp_ffffffffffffffffffffffffffffffff"); err == nil {
		t.Fatal("foreign exposure stop succeeded")
	}
	connection, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatalf("foreign stop closed owned listener: %v", err)
	}
	connection.Close()
}

func TestWorkspaceServiceConcurrentOwnersAndUnsafeRegistryFailWithoutReadCleanup(t *testing.T) {
	base := t.TempDir()
	runner := &serviceExposureRunner{}
	runtime, err := newRuntimeWithData(base+"/config", base+"/state", base+"/data", runner)
	if err != nil {
		t.Fatal(err)
	}
	instance := projectRuntimeInstance(t, runtime)
	container, _, _ := tobari.ProjectResourceNames(instance.ID)
	first, err := runtime.startWorkspaceServiceController(context.Background(), instance, container)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close(context.Background())
	second, err := runtime.startWorkspaceServiceController(context.Background(), instance, container)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close(context.Background())
	first.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: strings.Repeat("d", 32), Port: 3000})
	second.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: strings.Repeat("e", 32), Port: 3001})
	requests, err := runtime.ReviewServiceRequests(context.Background())
	if err != nil || len(requests.Requests) != 2 || requests.Requests[0].AttachmentID == requests.Requests[1].AttachmentID {
		t.Fatalf("concurrent requests=%+v err=%v", requests, err)
	}
	contextID, err := tobari.ParseContextID(first.principal.contextID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := tobari.ParseWorkspaceID(first.principal.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ObserveStatusServices(context.Background(), contextID, workspaceID); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("two live same-scope service owners were not ambiguous: %v", err)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests, _ = runtime.ReviewServiceRequests(context.Background())
	if len(requests.Requests) != 1 || requests.Requests[0].AttachmentID != second.attachmentID {
		t.Fatalf("owner exit affected peer=%+v", requests)
	}

	forged := filepath.Join(runtime.serviceExposureLiveDirectory(), "att_ffffffffffffffffffffffffffffffff.json")
	if err := os.WriteFile(forged, []byte(`{"schema_version":1,"attachment_id":"att_ffffffffffffffffffffffffffffffff","nonce":"bad","socket_name":"../../escape","owner_pid":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(runtime.serviceExposureLiveDirectory(), "att_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.json")
	if err := os.Symlink(second.recordPath, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReviewServiceRequests(context.Background()); err == nil {
		t.Fatal("unsafe registry did not fail closed")
	}
	for _, path := range []string{forged, symlink} {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("read cleaned unsafe record %s: %v", path, err)
		}
	}
	if _, err := os.Lstat(second.recordPath); err != nil {
		t.Fatalf("symlink cleanup followed target: %v", err)
	}
}

func TestWorkspaceServiceObservationDistinguishesPartialUnavailableAndKnownEmpty(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	controller.submit(workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", ClientID: strings.Repeat("f", 32), Port: 3000})
	staleAttachment, err := newAttachmentEpochID()
	if err != nil {
		t.Fatal(err)
	}
	staleNonce, err := serviceEntropy("", 32)
	if err != nil {
		t.Fatal(err)
	}
	stale := serviceRendezvousRecord{
		SchemaVersion: 1, AttachmentID: staleAttachment,
		ContextID: controller.principal.contextID, WorkspaceID: controller.principal.workspaceID,
		Context: controller.principal.contextPresentation, ProjectRoot: controller.principal.projectRoot,
		Nonce: staleNonce, SocketName: "owner-" + staleNonce[:32] + ".sock", OwnerPID: os.Getpid(), OwnerUID: os.Getuid(),
	}
	stalePath := filepath.Join(runtime.serviceExposureLiveDirectory(), staleAttachment+".json")
	if err := writeAtomicJSON(stalePath, stale); err != nil {
		t.Fatal(err)
	}

	status, err := runtime.ServiceStatus(context.Background())
	if err != nil || status.Observation != tobari.ServiceObservationPartial || status.ObservedOwnerCount != 1 || status.UnavailableOwnerCount != 1 || len(status.Requests) != 1 {
		t.Fatalf("partial status=%+v err=%v", status, err)
	}
	if _, err := os.Lstat(stalePath); err != nil {
		t.Fatalf("read removed unavailable owner: %v", err)
	}
	if err := runtime.DenyServiceRequest(context.Background(), status.Requests[0].ID); err == nil {
		t.Fatal("exact action proceeded through incomplete owner observation")
	}
	if err := controller.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = runtime.ServiceStatus(context.Background())
	if err != nil || status.Observation != tobari.ServiceObservationUnavailable || status.ObservedOwnerCount != 0 || status.UnavailableOwnerCount != 1 {
		t.Fatalf("unavailable status=%+v err=%v", status, err)
	}
	if err := os.Remove(stalePath); err != nil {
		t.Fatal(err)
	}
	status, err = runtime.ServiceStatus(context.Background())
	if err != nil || status.Observation != tobari.ServiceObservationComplete || status.ObservedOwnerCount != 0 || status.UnavailableOwnerCount != 0 || len(status.Requests) != 0 || len(status.Exposures) != 0 {
		t.Fatalf("known-empty status=%+v err=%v", status, err)
	}
}

func TestStatusServiceOwnerAnchorOmitsDeadSameScopeRecordsWithoutCleanup(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	runtime.serviceOwnerProcessAlive = func(pid int) bool { return pid == os.Getpid() }
	deadPaths := make([]string, 0, 2)
	for index, suffix := range []string{"a", "b"} {
		attachment := "att_" + strings.Repeat(suffix, 32)
		nonce := strings.Repeat(suffix, 64)
		record := serviceRendezvousRecord{
			SchemaVersion: 1, AttachmentID: attachment,
			ContextID: controller.principal.contextID, WorkspaceID: controller.principal.workspaceID,
			Context: controller.principal.contextPresentation, ProjectRoot: controller.principal.projectRoot,
			Nonce: nonce, SocketName: "owner-" + nonce[:32] + ".sock", OwnerPID: 31001 + index, OwnerUID: os.Getuid(),
		}
		path := filepath.Join(runtime.serviceExposureLiveDirectory(), attachment+".json")
		if err := writeAtomicJSON(path, record); err != nil {
			t.Fatal(err)
		}
		deadPaths = append(deadPaths, path)
	}
	contextID, err := tobari.ParseContextID(controller.principal.contextID)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := tobari.ParseWorkspaceID(controller.principal.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	status, err := runtime.ObserveStatusServices(context.Background(), contextID, workspaceID)
	if err != nil || status.Observation != tobari.ServiceObservationComplete || status.UnavailableOwnerCount != 0 {
		t.Fatalf("status with dead owners=%+v err=%v", status, err)
	}
	anchor, err := runtime.anchorServiceOwners(context.Background())
	if err != nil || len(anchor.records) != 1 || anchor.records[0].AttachmentID != controller.attachmentID {
		t.Fatalf("live owner anchor=%+v err=%v", anchor, err)
	}
	for _, path := range deadPaths {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("read-only anchor cleaned dead owner %s: %v", path, err)
		}
	}
}

func TestWorkspaceServiceAttachmentCleanupReportsOnlyConfirmedBoundedCounts(t *testing.T) {
	runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
	target, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	accepted := make(chan struct{})
	release := make(chan struct{})
	go func() {
		connection, acceptErr := target.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = readServiceHeader(bufio.NewReader(connection))
		close(accepted)
		<-release
	}()
	exposure := allowTestService(t, runtime, controller, target.Addr().(*net.TCPAddr).Port)
	host, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(exposure.HostPort)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fmt.Fprintf(host, "GET /hold HTTP/1.1\r\nHost: %s\r\n\r\n", testServiceAuthority(t, exposure))
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("relay did not reach target")
	}
	close(release)
	receipt, err := controller.CloseWithReceipt(context.Background())
	_ = host.Close()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 1 || receipt.PendingWithdrawnCount != 0 || receipt.ExposureClosedCount != 1 || receipt.StreamClosedCount != 1 {
		t.Fatalf("cleanup receipt=%+v", receipt)
	}
	if _, err := os.Lstat(controller.recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("confirmed cleanup retained owner state: %v", err)
	}
}

func TestWorkspaceServiceOpenReResolvesLiveOwnerAndReportsBoundedDispatchOutcome(t *testing.T) {
	for _, outcome := range []tobari.ServiceOpenOutcome{tobari.ServiceOpenNotDispatched, tobari.ServiceOpenRequested, tobari.ServiceOpenOutcomeUnknown} {
		t.Run(string(outcome), func(t *testing.T) {
			runtime, controller := newTestServiceController(t, &serviceExposureRunner{})
			exposure := allowTestService(t, runtime, controller, 3000)
			browser := &fixedServiceBrowser{outcome: outcome}
			runtime.serviceBrowser = browser
			result, err := runtime.OpenServiceExposure(context.Background(), exposure.ID)
			if err != nil || result.ID != exposure.ID || result.URL != exposure.URL || result.Outcome != outcome || !reflect.DeepEqual(browser.targets, []string{exposure.URL}) {
				t.Fatalf("open result=%+v targets=%v err=%v", result, browser.targets, err)
			}
			if err := runtime.StopServiceExposure(context.Background(), exposure.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.OpenServiceExposure(context.Background(), exposure.ID); err == nil {
				t.Fatal("stopped exposure retained browser authority")
			}
			if !reflect.DeepEqual(browser.targets, []string{exposure.URL}) {
				t.Fatalf("stale open reached browser: %v", browser.targets)
			}
		})
	}
}
