package dockerruntime

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type permissionChannelObserverStub struct {
	result  tobari.PermissionWaitResult
	err     error
	ids     chan string
	started chan struct{}
	release chan struct{}
}

func (s *permissionChannelObserverStub) WaitPermission(ctx context.Context, id string) (tobari.PermissionWaitResult, error) {
	if s.ids != nil {
		s.ids <- id
	}
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return s.result, s.err
}

type permissionChannelRunner struct {
	recordingRunner
	args       chan []string
	mainExited chan struct{}
	verifyCall int
	once       sync.Once
}

// Ordinary project-runtime fixtures opt into the dedicated control seam
// explicitly without recording its long-lived exec as the attached child.
func (r *recordingRunner) RunWorkspacePermissionControl(
	ctx context.Context, args, _ []string, in io.Reader, out, _ io.Writer,
) error {
	if len(args) > 4 && args[4] == workspacePermissionSocketAbsentProgram {
		return nil
	}
	if _, err := io.WriteString(out, "{\"schema_version\":1,\"ready\":true}\n"); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, in)
	if err != nil && !errorsIsPipeClosure(err) {
		return err
	}
	return ctx.Err()
}

func (r *projectExitRunner) RunWorkspacePermissionControl(
	ctx context.Context, args, _ []string, in io.Reader, out, _ io.Writer,
) error {
	if len(args) > 4 && args[4] == workspacePermissionSocketAbsentProgram {
		return nil
	}
	if _, err := io.WriteString(out, "{\"schema_version\":1,\"ready\":true}\n"); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, in)
	if err != nil && !errorsIsPipeClosure(err) {
		return err
	}
	return ctx.Err()
}

type permissionUnsupportedRunner struct{ delegate recordingRunner }

func (r *permissionUnsupportedRunner) Run(
	ctx context.Context, args, environment []string, in io.Reader, out, errOut io.Writer,
) error {
	return r.delegate.Run(ctx, args, environment, in, out, errOut)
}

func (r *permissionUnsupportedRunner) Output(ctx context.Context, args, environment []string) ([]byte, error) {
	return r.delegate.Output(ctx, args, environment)
}

func (r *permissionChannelRunner) RunWorkspacePermissionControl(
	ctx context.Context, args, _ []string, in io.Reader, out, _ io.Writer,
) error {
	if len(args) > 4 && args[4] == workspacePermissionSocketAbsentProgram {
		r.verifyCall++
		return nil
	}
	if r.args != nil {
		r.args <- append([]string{}, args...)
	}
	if _, err := io.WriteString(out, "{\"schema_version\":1,\"ready\":true}\n"); err != nil {
		return err
	}
	_, err := io.Copy(io.Discard, in)
	r.once.Do(func() {
		if r.mainExited != nil {
			close(r.mainExited)
		}
	})
	if err != nil && !errorsIsPipeClosure(err) {
		return err
	}
	return ctx.Err()
}

func errorsIsPipeClosure(err error) bool {
	return err == io.ErrClosedPipe || strings.Contains(err.Error(), "closed pipe")
}

func newSignedTestChannel(t *testing.T, observer *permissionChannelObserverStub) (*workspacePermissionChannel, *bufio.Reader, *io.PipeWriter) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	return &workspacePermissionChannel{
		channelID:    "pwc_0123456789abcdef0123456789abcdef",
		attachmentID: "att_0123456789abcdef0123456789abcdef",
		ownerBinding: strings.Repeat("a", 64), verifyKey: publicKey, signingKey: privateKey,
		writer: writer, active: map[string]context.CancelFunc{}, observer: observer,
		windowStart: time.Now(),
	}, bufio.NewReader(reader), writer
}

func TestWorkspacePermissionChannelForwardsOnlySignedWaitResult(t *testing.T) {
	observer := &permissionChannelObserverStub{result: tobari.PermissionWaitResultAllow, ids: make(chan string, 1)}
	channel, output, writer := newSignedTestChannel(t, observer)
	defer writer.Close()
	request := workspacePermissionControlRequest{
		SchemaVersion: 1, Operation: "wait", ClientID: strings.Repeat("b", 32),
		PermissionWaitID: "pwt_0123456789abcdef0123456789abcdef", RequestNonce: strings.Repeat("c", 64),
	}
	if err := channel.startWait(request); err != nil {
		t.Fatal(err)
	}
	select {
	case observed := <-observer.ids:
		if observed != request.PermissionWaitID {
			t.Fatalf("forwarded id = %q", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("wait was not forwarded")
	}
	line, err := output.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var response workspacePermissionControlResponse
	if err := decodeStrictJSON(bytesTrimLine(line), &response); err != nil || !response.OK ||
		response.Result != tobari.PermissionWaitResultAllow || response.Error != nil {
		t.Fatalf("response = %+v, %v", response, err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(response.Signature)
	payload, marshalErr := json.Marshal(response.payload())
	if err != nil || marshalErr != nil || !ed25519.Verify(channel.verifyKey, payload, signature) ||
		response.PermissionWaitID != request.PermissionWaitID || response.RequestNonce != request.RequestNonce ||
		response.AttachmentID != channel.attachmentID || response.OwnerBinding != channel.ownerBinding {
		t.Fatalf("signed response binding = %+v, %v, %v", response, err, marshalErr)
	}
}

func TestWorkspacePermissionChannelFailsClosedWithoutDedicatedRunner(t *testing.T) {
	root := t.TempDir()
	runtime, _ := newRuntime(root+"/config", root+"/state", &permissionUnsupportedRunner{})
	attachment := &interactiveWorkspaceAttachment{runtime: runtime, session: permissionSessionFixture()}
	if channel, err := runtime.startWorkspacePermissionChannel(context.Background(), attachment, "workspace-container"); err == nil || channel != nil {
		t.Fatalf("unsupported permission runner = %+v, %v", channel, err)
	}
}

func TestWorkspacePermissionChannelStartsBeforeChildAndClosesExecAndSocket(t *testing.T) {
	root := t.TempDir()
	runner := &permissionChannelRunner{args: make(chan []string, 1), mainExited: make(chan struct{})}
	runtime, _ := newRuntime(root+"/config", root+"/state", runner)
	attachment := &interactiveWorkspaceAttachment{runtime: runtime, session: permissionSessionFixture()}
	channel, err := runtime.startWorkspacePermissionChannel(context.Background(), attachment, "workspace-container")
	if err != nil {
		t.Fatal(err)
	}
	environment := channel.environment()
	for _, prefix := range []string{
		workspacePermissionSocketEnv + "=", workspacePermissionChannelEnv + "=",
		workspacePermissionAttachmentEnv + "=", workspacePermissionOwnerBindingEnv + "=",
		workspacePermissionVerifyKeyEnv + "=",
	} {
		count := 0
		for _, value := range environment {
			if strings.HasPrefix(value, prefix) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("permission channel environment %q count = %d in %q", prefix, count, environment)
		}
	}
	args := <-runner.args
	if len(args) < 7 || args[0] != "exec" || args[1] != "-i" || args[2] != "workspace-container" ||
		args[len(args)-1] != channel.socket {
		t.Fatalf("permission channel argv = %q", args)
	}
	if err := channel.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-runner.mainExited:
	default:
		t.Fatal("Workspace permission exec did not terminate before Close returned")
	}
	if runner.verifyCall != 1 {
		t.Fatalf("Workspace permission socket verification calls = %d", runner.verifyCall)
	}
}

func TestWorkspacePermissionChannelUnexpectedBridgeExitFailsClosed(t *testing.T) {
	requestReader, requestWriter := io.Pipe()
	_, responseWriter := io.Pipe()
	channel := &workspacePermissionChannel{
		writer: responseWriter, done: make(chan struct{}), active: map[string]context.CancelFunc{},
		windowStart: time.Now(),
	}
	ready := make(chan struct{})
	go channel.serve(requestReader, ready)
	if _, err := io.WriteString(requestWriter, "{\"schema_version\":1,\"ready\":true}\n"); err != nil {
		t.Fatal(err)
	}
	<-ready
	if err := requestWriter.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-channel.done:
	case <-time.After(time.Second):
		t.Fatal("unexpected bridge exit did not stop the host channel")
	}
	channel.mu.Lock()
	failed := channel.failed
	channel.mu.Unlock()
	if failed == nil || !strings.Contains(failed.Error(), io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("unexpected bridge exit fault = %v", failed)
	}
}

func bytesTrimLine(value []byte) []byte {
	return []byte(strings.TrimSuffix(string(value), "\n"))
}

func TestPermissionWaitClientAcceptsOnlyAuthenticatedExactResponse(t *testing.T) {
	path, err := newWorkspacePermissionSocket()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(true)
	defer listener.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	channelID := "pwc_0123456789abcdef0123456789abcdef"
	attachmentID := "att_0123456789abcdef0123456789abcdef"
	ownerBinding := strings.Repeat("a", 64)
	t.Setenv(workspacePermissionSocketEnv, path)
	t.Setenv(workspacePermissionChannelEnv, channelID)
	t.Setenv(workspacePermissionAttachmentEnv, attachmentID)
	t.Setenv(workspacePermissionOwnerBindingEnv, ownerBinding)
	t.Setenv(workspacePermissionVerifyKeyEnv, base64.RawURLEncoding.EncodeToString(publicKey))
	client, err := NewPermissionWaitClientFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan workspacePermissionControlRequest, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		line, _ := bufio.NewReader(connection).ReadBytes('\n')
		var request workspacePermissionControlRequest
		_ = decodeStrictJSON(bytesTrimLine(line), &request)
		requests <- request
		response := workspacePermissionControlResponse{
			SchemaVersion: 1, ClientID: strings.Repeat("b", 32), ChannelID: channelID,
			AttachmentID: attachmentID, OwnerBinding: ownerBinding,
			PermissionWaitID: request.PermissionWaitID, RequestNonce: request.RequestNonce,
			OK: true, Result: tobari.PermissionWaitResultDeny,
		}
		payload, _ := json.Marshal(response.payload())
		response.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
		encoded, _ := json.Marshal(response)
		_, _ = connection.Write(append(encoded, '\n'))
	}()
	id := "pwt_0123456789abcdef0123456789abcdef"
	result, err := client.WaitPermission(context.Background(), id)
	if err != nil || result != tobari.PermissionWaitResultDeny {
		t.Fatalf("WaitPermission() = %q, %v", result, err)
	}
	request := <-requests
	if request.PermissionWaitID != id || !workspacePermissionNoncePattern.MatchString(request.RequestNonce) {
		t.Fatalf("helper request = %+v", request)
	}
}

func TestPermissionWaitClientRejectsUnsignedAndReplayedSignedResults(t *testing.T) {
	path, err := newWorkspacePermissionSocket()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(true)
	defer listener.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv(workspacePermissionSocketEnv, path)
	t.Setenv(workspacePermissionChannelEnv, "pwc_0123456789abcdef0123456789abcdef")
	t.Setenv(workspacePermissionAttachmentEnv, "att_0123456789abcdef0123456789abcdef")
	t.Setenv(workspacePermissionOwnerBindingEnv, strings.Repeat("a", 64))
	t.Setenv(workspacePermissionVerifyKeyEnv, base64.RawURLEncoding.EncodeToString(publicKey))
	client, err := NewPermissionWaitClientFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for index := 0; index < 2; index++ {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			line, _ := bufio.NewReader(connection).ReadBytes('\n')
			var request workspacePermissionControlRequest
			_ = decodeStrictJSON(bytesTrimLine(line), &request)
			response := workspacePermissionControlResponse{
				SchemaVersion: 1, ClientID: strings.Repeat("b", 32),
				ChannelID:    "pwc_0123456789abcdef0123456789abcdef",
				AttachmentID: "att_0123456789abcdef0123456789abcdef", OwnerBinding: strings.Repeat("a", 64),
				PermissionWaitID: request.PermissionWaitID, RequestNonce: request.RequestNonce,
				OK: true, Result: tobari.PermissionWaitResultAllow,
			}
			if index == 0 {
				response.Signature = "invalid"
			} else {
				response.RequestNonce = strings.Repeat("d", 64)
				payload, _ := json.Marshal(response.payload())
				response.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
			}
			encoded, _ := json.Marshal(response)
			_, _ = connection.Write(append(encoded, '\n'))
			_ = connection.Close()
		}
	}()
	for _, name := range []string{"unsigned", "replayed"} {
		if _, err := client.WaitPermission(context.Background(), "pwt_0123456789abcdef0123456789abcdef"); !hasInfrastructureFaultCode(err, "permission_wait_transport_failed") {
			t.Fatalf("%s response = %v", name, err)
		}
	}
}

func TestWorkspacePermissionHostBoundsDoNotTrustBridgeSemaphore(t *testing.T) {
	channel := &workspacePermissionChannel{active: map[string]context.CancelFunc{}, windowStart: time.Now()}
	for index := 0; index < workspacePermissionRequestMax; index++ {
		if err := channel.acceptRequest(channel.windowStart.Add(time.Second)); err != nil {
			t.Fatalf("bounded request %d = %v", index, err)
		}
	}
	if err := channel.acceptRequest(channel.windowStart.Add(time.Second)); err == nil {
		t.Fatal("host accepted request beyond its fixed window")
	}
	observer := &permissionChannelObserverStub{started: make(chan struct{}, workspacePermissionActiveMax), release: make(chan struct{})}
	channel, _, writer := newSignedTestChannel(t, observer)
	defer writer.Close()
	for index := 0; index < workspacePermissionActiveMax; index++ {
		request := workspacePermissionControlRequest{
			SchemaVersion: 1, Operation: "wait", ClientID: strings.Repeat(string("01234567"[index]), 32),
			PermissionWaitID: "pwt_0123456789abcdef0123456789abcdef", RequestNonce: strings.Repeat("c", 64),
		}
		if err := channel.startWait(request); err != nil {
			t.Fatal(err)
		}
	}
	for index := 0; index < workspacePermissionActiveMax; index++ {
		<-observer.started
	}
	overflow := workspacePermissionControlRequest{
		SchemaVersion: 1, Operation: "wait", ClientID: strings.Repeat("f", 32),
		PermissionWaitID: "pwt_0123456789abcdef0123456789abcdef", RequestNonce: strings.Repeat("d", 64),
	}
	if err := channel.startWait(overflow); err == nil {
		t.Fatal("host accepted more than eight active Workspace waits")
	}
	close(observer.release)
}

func TestWorkspacePermissionResponseWriteFailureFailsWholeChannel(t *testing.T) {
	channel, reader, writer := newSignedTestChannel(t, &permissionChannelObserverStub{})
	_ = reader
	_ = writer.Close()
	response := workspacePermissionControlResponse{
		SchemaVersion: 1, ClientID: strings.Repeat("b", 32), ChannelID: channel.channelID,
		AttachmentID: channel.attachmentID, OwnerBinding: channel.ownerBinding,
		PermissionWaitID: "pwt_0123456789abcdef0123456789abcdef", RequestNonce: strings.Repeat("c", 64),
		OK: true, Result: tobari.PermissionWaitResultAllow,
	}
	err := channel.signAndRespond(&response)
	if err == nil {
		t.Fatal("closed host response pipe accepted a result")
	}
	channel.fail(err)
	channel.mu.Lock()
	failed := channel.failed
	channel.mu.Unlock()
	if failed == nil {
		t.Fatal("host response failure did not fail the channel")
	}
}

func TestPermissionWaitClientRejectsUnsafeSocket(t *testing.T) {
	path, err := newWorkspacePermissionSocket()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	listener.SetUnlinkOnClose(true)
	defer listener.Close()
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	t.Setenv(workspacePermissionSocketEnv, path)
	if _, err := NewPermissionWaitClientFromEnvironment(); err == nil {
		t.Fatal("broad-mode helper socket was accepted")
	}
}

func TestWorkspacePermissionAgentIsTransportOnlyAndHostOwnsBounds(t *testing.T) {
	for _, required := range []string{"SO_PEERCRED", "uid!=os.getuid()", "request_limit=4096", "response_limit=1024", "BoundedSemaphore(8)"} {
		if !strings.Contains(workspacePermissionAgentProgram, required) {
			t.Fatalf("permission bridge omits %q", required)
		}
	}
	if workspacePermissionActiveMax != 8 || workspacePermissionRequestMax != 64 || workspacePermissionRequestWindow != 15*time.Minute {
		t.Fatalf("host bounds = active:%d requests:%d window:%s", workspacePermissionActiveMax, workspacePermissionRequestMax, workspacePermissionRequestWindow)
	}
	if faultValue := permissionWaitOwnerFault(); faultValue == nil {
		t.Fatal("owner fault is missing")
	} else if public, ok := fault.PublicCopy(faultValue); !ok || public.Retryable {
		t.Fatalf("owner fault retryability = %+v, %v", public, ok)
	}
	ownerInterruption, ownerOK := fault.PublicCopy(permissionWaitOwnerTransportInterruption(context.Background(), os.ErrNotExist))
	if !ownerOK || ownerInterruption.Code != "permission_wait_owner_unavailable" || ownerInterruption.Retryable {
		t.Fatalf("owner interruption = %+v, %t", ownerInterruption, ownerOK)
	}
	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceledOwner, canceledOwnerOK := fault.PublicCopy(permissionWaitOwnerTransportInterruption(canceledContext, context.Canceled))
	if !canceledOwnerOK || canceledOwner.Code != "permission_wait_interrupted" || !canceledOwner.Retryable {
		t.Fatalf("canceled owner transport = %+v, %t", canceledOwner, canceledOwnerOK)
	}
	bridgeInterruption, bridgeOK := fault.PublicCopy(permissionWaitTransportInterruption(os.ErrNotExist))
	if !bridgeOK || bridgeInterruption.Code != "permission_wait_interrupted" || !bridgeInterruption.Retryable {
		t.Fatalf("bridge interruption = %+v, %t", bridgeInterruption, bridgeOK)
	}
	for _, test := range []struct {
		code      string
		retryable bool
	}{
		{code: "permission_wait_owner_unavailable", retryable: true},
		{code: "permission_wait_interrupted", retryable: false},
	} {
		response := permissionWaitTransportResponse{SchemaVersion: 1, Error: &permissionWaitTransportFault{
			Kind: fault.KindUnavailable, Code: test.code, Message: "synthetic fault", Retryable: test.retryable,
		}}
		if err := response.Validate(); err == nil {
			t.Fatalf("fault %q accepted retryable=%t", test.code, test.retryable)
		}
	}
}
