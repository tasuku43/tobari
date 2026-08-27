package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	workspacePermissionSocketEnv       = "TOBARI_PERMISSION_SOCKET"
	workspacePermissionChannelEnv      = "TOBARI_PERMISSION_CHANNEL"
	workspacePermissionAttachmentEnv   = "TOBARI_PERMISSION_ATTACHMENT"
	workspacePermissionOwnerBindingEnv = "TOBARI_PERMISSION_OWNER_BINDING"
	workspacePermissionVerifyKeyEnv    = "TOBARI_PERMISSION_VERIFY_KEY"
	workspacePermissionReadyTimeout    = 5 * time.Second
	workspacePermissionCloseTimeout    = 2 * time.Second
	workspacePermissionRequestWindow   = tobari.PermissionWaitLease
	workspacePermissionRequestMax      = 64
	workspacePermissionActiveMax       = tobari.PermissionWaitMaxLive
)

const workspacePermissionAgentProgram = `import json,os,queue,select,secrets,socket,struct,sys,threading
path=sys.argv[1]; request_limit=4096; response_limit=1024; write_lock=threading.Lock(); responses={}; responses_lock=threading.Lock(); slots=threading.BoundedSemaphore(8); stopping=threading.Event(); exit_code=[0]
def strict(raw):
 def pairs(values):
  result={}
  for key,value in values:
   if key in result: raise ValueError('duplicate')
   result[key]=value
  return result
 return json.loads(raw,object_pairs_hook=pairs)
def stop(code):
 if code and not exit_code[0]: exit_code[0]=code
 stopping.set()
 try: server.close()
 except Exception: pass
 try: os.unlink(path)
 except FileNotFoundError: pass
 except Exception:
  if not exit_code[0]: exit_code[0]=1
def emit(value):
 try:
  data=(json.dumps(value,separators=(',',':'))+'\n').encode()
  if len(data)>request_limit: raise ValueError('oversize')
  with write_lock: os.write(1,data)
 except Exception:
  stop(1); return False
 return True
def read_responses():
 while not stopping.is_set():
  line=sys.stdin.buffer.readline(response_limit+2)
  if not line: stop(0); return
  if len(line)>response_limit+1 or not line.endswith(b'\n'): stop(1); return
  try: value=strict(line)
  except Exception: stop(1); return
  client=value.get('client_id')
  with responses_lock: target=responses.get(client)
  if target is not None: target.put(value)
def serve(connection):
 client=secrets.token_hex(16); target=queue.Queue(maxsize=1)
 try:
  credential=connection.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12); pid,uid,_=struct.unpack('3i',credential)
  if pid<1 or uid!=os.getuid(): return
  connection.settimeout(5); data=b''
  while b'\n' not in data and len(data)<=request_limit:
   part=connection.recv(1024)
   if not part: return
   data+=part
  if len(data)>request_limit or data.count(b'\n')!=1 or not data.endswith(b'\n'): return
  request=strict(data)
  if set(request)!={'schema_version','operation','permission_wait_id','request_nonce'} or request.get('schema_version')!=1 or request.get('operation')!='wait' or not isinstance(request.get('permission_wait_id'),str) or not isinstance(request.get('request_nonce'),str): return
  request['client_id']=client
  with responses_lock: responses[client]=target
  if not emit(request): return
  connection.settimeout(None)
  while not stopping.is_set():
   try: response=target.get(timeout=.1); break
   except queue.Empty:
    readable,_,_=select.select([connection],[],[],0)
    if readable and connection.recv(1,socket.MSG_PEEK)==b'': emit({'schema_version':1,'operation':'cancel','client_id':client}); return
  else: return
  if not isinstance(response,dict) or response.get('schema_version')!=1 or response.get('client_id')!=client: return
  encoded=(json.dumps(response,separators=(',',':'))+'\n').encode()
  if len(encoded)>response_limit: return
  connection.sendall(encoded)
 finally:
  with responses_lock: responses.pop(client,None)
  connection.close(); slots.release()
if os.path.lexists(path): raise SystemExit(1)
server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); server.bind(path); os.chmod(path,0o600); server.listen(8)
threading.Thread(target=read_responses,daemon=True).start()
if not emit({'schema_version':1,'ready':True}): raise SystemExit(1)
try:
 while not stopping.is_set():
  try: connection,_=server.accept()
  except OSError:
   if stopping.is_set(): break
   raise
  if not slots.acquire(False): connection.close(); continue
  threading.Thread(target=serve,args=(connection,),daemon=True).start()
finally: stop(exit_code[0])
raise SystemExit(exit_code[0])`

const workspacePermissionSocketAbsentProgram = `import os,sys
raise SystemExit(1 if os.path.lexists(sys.argv[1]) else 0)`

type workspacePermissionControlRequest struct {
	SchemaVersion    int    `json:"schema_version"`
	Operation        string `json:"operation"`
	ClientID         string `json:"client_id,omitempty"`
	PermissionWaitID string `json:"permission_wait_id,omitempty"`
	RequestNonce     string `json:"request_nonce,omitempty"`
}

type workspacePermissionControlResponse struct {
	SchemaVersion    int                           `json:"schema_version"`
	ClientID         string                        `json:"client_id"`
	ChannelID        string                        `json:"channel_id"`
	AttachmentID     string                        `json:"attachment_id"`
	OwnerBinding     string                        `json:"owner_binding"`
	PermissionWaitID string                        `json:"permission_wait_id"`
	RequestNonce     string                        `json:"request_nonce"`
	OK               bool                          `json:"ok"`
	Result           tobari.PermissionWaitResult   `json:"result,omitempty"`
	Error            *permissionWaitTransportFault `json:"error,omitempty"`
	Signature        string                        `json:"signature"`
}

type workspacePermissionSignedPayload struct {
	SchemaVersion    int                           `json:"schema_version"`
	ClientID         string                        `json:"client_id"`
	ChannelID        string                        `json:"channel_id"`
	AttachmentID     string                        `json:"attachment_id"`
	OwnerBinding     string                        `json:"owner_binding"`
	PermissionWaitID string                        `json:"permission_wait_id"`
	RequestNonce     string                        `json:"request_nonce"`
	OK               bool                          `json:"ok"`
	Result           tobari.PermissionWaitResult   `json:"result,omitempty"`
	Error            *permissionWaitTransportFault `json:"error,omitempty"`
}

func (r workspacePermissionControlResponse) payload() workspacePermissionSignedPayload {
	return workspacePermissionSignedPayload{
		SchemaVersion: r.SchemaVersion, ClientID: r.ClientID, ChannelID: r.ChannelID,
		AttachmentID: r.AttachmentID, OwnerBinding: r.OwnerBinding,
		PermissionWaitID: r.PermissionWaitID, RequestNonce: r.RequestNonce,
		OK: r.OK, Result: r.Result, Error: r.Error,
	}
}

type workspacePermissionChannel struct {
	lifetime     context.Context
	socket       string
	container    string
	channelID    string
	attachmentID string
	ownerBinding string
	verifyKey    ed25519.PublicKey
	signingKey   ed25519.PrivateKey
	runner       workspacePermissionControlRunner
	cancel       context.CancelFunc
	writer       *io.PipeWriter
	done         chan struct{}
	execDone     chan error
	once         sync.Once
	writeMu      sync.Mutex
	mu           sync.Mutex
	active       map[string]context.CancelFunc
	accepted     int
	windowStart  time.Time
	failed       error
	closing      bool
	observer     interface {
		WaitPermission(context.Context, string) (tobari.PermissionWaitResult, error)
	}
}

var (
	workspacePermissionClientIDPattern  = regexp.MustCompile(`^[0-9a-f]{32}$`)
	workspacePermissionChannelIDPattern = regexp.MustCompile(`^pwc_[0-9a-f]{32}$`)
	workspacePermissionNoncePattern     = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

func workspacePermissionEntropy(prefix string, size int) (string, error) {
	raw := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func newWorkspacePermissionSocket() (string, error) {
	identity, err := workspacePermissionEntropy("", 16)
	if err != nil {
		return "", fmt.Errorf("generate Workspace permission socket: %w", err)
	}
	return "/tmp/tobari-permission-" + identity + ".sock", nil
}

func workspacePermissionOwnerBinding(session tobari.InteractiveAttachmentSession) string {
	digest := sha256.Sum256([]byte("tobari-permission-channel-v1\x00" +
		session.WorkspaceManifestID + "\x00" + session.WorkspaceID + "\x00" +
		session.AttachmentID + "\x00" + session.IngestionNonce))
	return hex.EncodeToString(digest[:])
}

func (r *Runtime) startWorkspacePermissionChannel(
	ctx context.Context, attachment *interactiveWorkspaceAttachment, container string,
) (*workspacePermissionChannel, error) {
	runner, ok := r.runner.(workspacePermissionControlRunner)
	if !ok {
		return nil, fmt.Errorf("Workspace permission control runner is unavailable")
	}
	if attachment == nil || attachment.session.Validate() != nil {
		return nil, fmt.Errorf("interactive permission attachment is required")
	}
	observer, err := newPermissionWaitOwnerClient(r, attachment.session)
	if err != nil {
		return nil, err
	}
	socketPath, err := newWorkspacePermissionSocket()
	if err != nil {
		return nil, err
	}
	channelID, err := workspacePermissionEntropy("pwc_", 16)
	if err != nil {
		return nil, fmt.Errorf("generate Workspace permission channel identity: %w", err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate Workspace permission response key: %w", err)
	}
	channel := &workspacePermissionChannel{
		lifetime: r.lifetimeParent(ctx),
		socket:   socketPath, container: container, channelID: channelID,
		attachmentID: attachment.session.AttachmentID,
		ownerBinding: workspacePermissionOwnerBinding(attachment.session),
		verifyKey:    publicKey, signingKey: privateKey, runner: runner,
		done: make(chan struct{}), execDone: make(chan error, 1),
		active: map[string]context.CancelFunc{}, observer: observer, windowStart: time.Now(),
	}
	// The control process lifetime is closed explicitly by channel.Close. It is
	// intentionally detached from child cancellation so stdin can be closed and
	// the in-container socket can be verified absent before authority teardown.
	controlContext, cancel := context.WithCancel(channel.lifetime)
	channel.cancel = cancel
	responseReader, responseWriter := io.Pipe()
	requestReader, requestWriter := io.Pipe()
	channel.writer = responseWriter
	ready := make(chan struct{})
	go func() {
		args := []string{"exec", "-i", container, "python3", "-c", workspacePermissionAgentProgram, socketPath}
		runErr := runner.RunWorkspacePermissionControl(controlContext, args, os.Environ(), responseReader, requestWriter, io.Discard)
		_ = requestWriter.CloseWithError(runErr)
		channel.execDone <- runErr
		close(channel.execDone)
	}()
	go channel.serve(requestReader, ready)
	timer := time.NewTimer(workspacePermissionReadyTimeout)
	defer timer.Stop()
	select {
	case <-ready:
		if err := ctx.Err(); err != nil {
			return nil, errors.Join(err, channel.Close())
		}
		return channel, nil
	case <-channel.done:
		return nil, errors.Join(fmt.Errorf("Workspace permission control stopped before readiness"), channel.Close())
	case <-timer.C:
		return nil, errors.Join(fmt.Errorf("Workspace permission control did not become ready"), channel.Close())
	case <-ctx.Done():
		return nil, errors.Join(ctx.Err(), channel.Close())
	}
}

func (c *workspacePermissionChannel) environment() []string {
	if c == nil || c.socket == "" || len(c.verifyKey) != ed25519.PublicKeySize {
		return nil
	}
	return []string{
		workspacePermissionSocketEnv + "=" + c.socket,
		workspacePermissionChannelEnv + "=" + c.channelID,
		workspacePermissionAttachmentEnv + "=" + c.attachmentID,
		workspacePermissionOwnerBindingEnv + "=" + c.ownerBinding,
		workspacePermissionVerifyKeyEnv + "=" + base64.RawURLEncoding.EncodeToString(c.verifyKey),
	}
}

func (c *workspacePermissionChannel) fail(err error) {
	if err == nil {
		err = fmt.Errorf("Workspace permission control failed")
	}
	c.mu.Lock()
	if c.failed == nil {
		c.failed = err
	}
	cancels := make([]context.CancelFunc, 0, len(c.active))
	for _, cancel := range c.active {
		cancels = append(cancels, cancel)
	}
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	c.writeMu.Lock()
	if c.writer != nil {
		_ = c.writer.CloseWithError(err)
	}
	c.writeMu.Unlock()
}

func (c *workspacePermissionChannel) acceptRequest(now time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failed != nil {
		return c.failed
	}
	if c.closing {
		return fmt.Errorf("Workspace permission channel is closing")
	}
	if now.Sub(c.windowStart) >= workspacePermissionRequestWindow {
		c.windowStart = now
		c.accepted = 0
	}
	if now.Before(c.windowStart) {
		return fmt.Errorf("Workspace permission request clock moved backwards")
	}
	if c.accepted >= workspacePermissionRequestMax {
		return fmt.Errorf("Workspace permission request window is exhausted")
	}
	c.accepted++
	return nil
}

func (c *workspacePermissionChannel) serve(reader io.Reader, ready chan struct{}) {
	defer close(c.done)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), tobari.PermissionWaitRequestLimit)
	readyOnce := sync.Once{}
	readySeen := false
	for scanner.Scan() {
		line := append([]byte{}, scanner.Bytes()...)
		var marker struct {
			SchemaVersion int  `json:"schema_version"`
			Ready         bool `json:"ready"`
		}
		if decodeStrictJSON(line, &marker) == nil && marker.SchemaVersion == 1 && marker.Ready {
			if readySeen {
				c.fail(fmt.Errorf("Workspace permission bridge emitted duplicate readiness"))
				return
			}
			readySeen = true
			readyOnce.Do(func() { close(ready) })
			continue
		}
		if !readySeen {
			c.fail(fmt.Errorf("Workspace permission bridge emitted a request before readiness"))
			return
		}
		if err := c.acceptRequest(time.Now()); err != nil {
			c.fail(err)
			return
		}
		var request workspacePermissionControlRequest
		if decodeStrictJSON(line, &request) != nil || request.SchemaVersion != 1 ||
			!workspacePermissionClientIDPattern.MatchString(request.ClientID) {
			c.fail(fmt.Errorf("Workspace permission bridge emitted an invalid request"))
			return
		}
		switch request.Operation {
		case "wait":
			if tobari.ValidatePermissionWaitID(request.PermissionWaitID) != nil ||
				!workspacePermissionNoncePattern.MatchString(request.RequestNonce) {
				c.fail(fmt.Errorf("Workspace permission bridge emitted an invalid wait"))
				return
			}
			if err := c.startWait(request); err != nil {
				if respondErr := c.respondFault(request, err); respondErr != nil {
					c.fail(respondErr)
					return
				}
			}
		case "cancel":
			if request.PermissionWaitID != "" || request.RequestNonce != "" {
				c.fail(fmt.Errorf("Workspace permission bridge emitted an invalid cancellation"))
				return
			}
			c.mu.Lock()
			cancel := c.active[request.ClientID]
			c.mu.Unlock()
			if cancel != nil {
				cancel()
			}
		default:
			c.fail(fmt.Errorf("Workspace permission bridge emitted an unknown operation"))
			return
		}
	}
	scanErr := scanner.Err()
	c.mu.Lock()
	closing := c.closing
	c.mu.Unlock()
	if !closing {
		if scanErr == nil {
			scanErr = io.ErrUnexpectedEOF
		}
		c.fail(fmt.Errorf("read Workspace permission control: %w", scanErr))
	}
	c.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(c.active))
	for _, cancel := range c.active {
		cancels = append(cancels, cancel)
	}
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (c *workspacePermissionChannel) startWait(request workspacePermissionControlRequest) error {
	ctx, cancel := context.WithTimeout(c.lifetime, tobari.PermissionWaitLease)
	c.mu.Lock()
	if c.failed != nil || c.closing || len(c.active) >= workspacePermissionActiveMax {
		c.mu.Unlock()
		cancel()
		return fault.New(fault.KindUnavailable, "permission_wait_unavailable", "permission wait channel is unavailable", true)
	}
	if _, exists := c.active[request.ClientID]; exists {
		c.mu.Unlock()
		cancel()
		return fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport returned a duplicate client", false)
	}
	c.active[request.ClientID] = cancel
	c.mu.Unlock()
	go c.wait(ctx, cancel, request)
	return nil
}

func (c *workspacePermissionChannel) wait(
	ctx context.Context, cancel context.CancelFunc, request workspacePermissionControlRequest,
) {
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.active, request.ClientID)
		c.mu.Unlock()
	}()
	result, err := c.observer.WaitPermission(ctx, request.PermissionWaitID)
	transport := permissionWaitTransportResponseFor(result, err)
	response := workspacePermissionControlResponse{
		SchemaVersion: 1, ClientID: request.ClientID, ChannelID: c.channelID,
		AttachmentID: c.attachmentID, OwnerBinding: c.ownerBinding,
		PermissionWaitID: request.PermissionWaitID, RequestNonce: request.RequestNonce,
		OK: transport.OK, Result: transport.Result, Error: transport.Error,
	}
	if err := c.signAndRespond(&response); err != nil {
		c.fail(err)
	}
}

func (c *workspacePermissionChannel) respondFault(request workspacePermissionControlRequest, err error) error {
	transport := permissionWaitTransportResponseFor("", err)
	response := workspacePermissionControlResponse{
		SchemaVersion: 1, ClientID: request.ClientID, ChannelID: c.channelID,
		AttachmentID: c.attachmentID, OwnerBinding: c.ownerBinding,
		PermissionWaitID: request.PermissionWaitID, RequestNonce: request.RequestNonce,
		Error: transport.Error,
	}
	return c.signAndRespond(&response)
}

func (c *workspacePermissionChannel) signAndRespond(response *workspacePermissionControlResponse) error {
	payload, err := json.Marshal(response.payload())
	if err != nil {
		return err
	}
	response.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(c.signingKey, payload))
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded)+1 > tobari.PermissionWaitResponseLimit {
		return fmt.Errorf("Workspace permission response exceeds its contract")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.writer == nil {
		return fmt.Errorf("Workspace permission response channel is closed")
	}
	if _, err := c.writer.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write Workspace permission response: %w", err)
	}
	return nil
}

func (c *workspacePermissionChannel) verifySocketAbsent(ctx context.Context) error {
	args := []string{"exec", c.container, "python3", "-c", workspacePermissionSocketAbsentProgram, c.socket}
	if err := c.runner.RunWorkspacePermissionControl(ctx, args, os.Environ(), nil, io.Discard, io.Discard); err != nil {
		return fmt.Errorf("verify Workspace permission socket cleanup: %w", err)
	}
	return nil
}

func (c *workspacePermissionChannel) Close() error {
	if c == nil || c.cancel == nil {
		return nil
	}
	var result error
	c.once.Do(func() {
		c.mu.Lock()
		c.closing = true
		cancels := make([]context.CancelFunc, 0, len(c.active))
		for _, cancel := range c.active {
			cancels = append(cancels, cancel)
		}
		c.mu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		c.writeMu.Lock()
		if c.writer != nil {
			result = c.writer.Close()
			c.writer = nil
		}
		c.writeMu.Unlock()

		var runErr error
		forced := false
		timer := time.NewTimer(workspacePermissionCloseTimeout)
		select {
		case runErr = <-c.execDone:
			timer.Stop()
		case <-timer.C:
			forced = true
			c.cancel()
			select {
			case runErr = <-c.execDone:
			case <-time.After(workspacePermissionCloseTimeout):
				result = errors.Join(result, fmt.Errorf("Workspace permission control shutdown timed out"))
			}
		}
		c.cancel()
		result = errors.Join(result, workspacePermissionControlCloseError(runErr, forced))
		select {
		case <-c.done:
		case <-time.After(workspacePermissionCloseTimeout):
			result = errors.Join(result, fmt.Errorf("Workspace permission response shutdown timed out"))
		}
		cleanup, cancel := context.WithTimeout(c.lifetime, workspacePermissionCloseTimeout)
		defer cancel()
		result = errors.Join(result, c.verifySocketAbsent(cleanup))
		c.mu.Lock()
		result = errors.Join(result, c.failed)
		c.mu.Unlock()
	})
	return result
}

func workspacePermissionControlCloseError(runErr error, forced bool) error {
	if runErr == nil || errors.Is(runErr, context.Canceled) || forced {
		return nil
	}
	return fmt.Errorf("Workspace permission control failed: %w", runErr)
}

// PermissionWaitClient is the attachment-local helper adapter. It has no
// policy, Docker, filesystem traversal, or request execution capability.
type PermissionWaitClient struct {
	socket       string
	channelID    string
	attachmentID string
	ownerBinding string
	verifyKey    ed25519.PublicKey
}

func NewPermissionWaitClientFromEnvironment() (*PermissionWaitClient, error) {
	socketPath := os.Getenv(workspacePermissionSocketEnv)
	if matched, _ := regexp.MatchString(`^/tmp/tobari-permission-[0-9a-f]{32}\.sock$`, socketPath); !matched {
		return nil, fmt.Errorf("permission wait attachment channel is unavailable")
	}
	info, err := os.Lstat(socketPath) // #nosec G703 -- the exact anchored /tmp filename grammar above excludes traversal and separators.
	if err != nil || info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return nil, fmt.Errorf("permission wait attachment channel is unsafe")
	}
	ownerUID, ok := fileOwnerUID(info)
	if !ok || ownerUID != os.Getuid() {
		return nil, fmt.Errorf("permission wait attachment channel owner is invalid")
	}
	channelID := os.Getenv(workspacePermissionChannelEnv)
	attachmentID := os.Getenv(workspacePermissionAttachmentEnv)
	ownerBinding := os.Getenv(workspacePermissionOwnerBindingEnv)
	encodedKey := os.Getenv(workspacePermissionVerifyKeyEnv)
	verifyKey, decodeErr := base64.RawURLEncoding.DecodeString(encodedKey)
	if !workspacePermissionChannelIDPattern.MatchString(channelID) ||
		tobari.ValidateAttachmentEpochID(attachmentID) != nil || !workspacePermissionNoncePattern.MatchString(ownerBinding) ||
		decodeErr != nil || len(verifyKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("permission wait attachment identity is invalid")
	}
	return &PermissionWaitClient{
		socket: socketPath, channelID: channelID, attachmentID: attachmentID,
		ownerBinding: ownerBinding, verifyKey: ed25519.PublicKey(verifyKey),
	}, nil
}

func (c *PermissionWaitClient) WaitPermission(ctx context.Context, id string) (tobari.PermissionWaitResult, error) {
	if err := tobari.ValidatePermissionWaitID(id); err != nil {
		return "", invalidPermissionWaitFault()
	}
	requestNonce, err := workspacePermissionEntropy("", 32)
	if err != nil {
		return "", fault.New(fault.KindInternal, "permission_wait_transport_failed", "permission wait transport could not create request identity", false)
	}
	request := workspacePermissionControlRequest{SchemaVersion: 1, Operation: "wait", PermissionWaitID: id, RequestNonce: requestNonce}
	encoded, _ := json.Marshal(request)
	if len(encoded)+1 > tobari.PermissionWaitRequestLimit {
		return "", invalidPermissionWaitFault()
	}
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return "", permissionWaitTransportInterruption(err)
	}
	defer connection.Close()
	cancelDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.SetDeadline(time.Now())
		case <-cancelDone:
		}
	}()
	defer close(cancelDone)
	if _, err := connection.Write(append(encoded, '\n')); err != nil {
		return "", permissionWaitTransportInterruption(err)
	}
	line, err := bufio.NewReader(io.LimitReader(connection, tobari.PermissionWaitResponseLimit+1)).ReadBytes('\n')
	if err != nil || len(line) > tobari.PermissionWaitResponseLimit {
		return "", permissionWaitTransportInterruption(err)
	}
	var response workspacePermissionControlResponse
	if decodeStrictJSON(bytes.TrimSuffix(line, []byte{'\n'}), &response) != nil {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response is invalid", false)
	}
	signature, err := base64.RawURLEncoding.DecodeString(response.Signature)
	payload, marshalErr := json.Marshal(response.payload())
	if err != nil || marshalErr != nil || len(signature) != ed25519.SignatureSize ||
		!ed25519.Verify(c.verifyKey, payload, signature) {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response is unauthenticated", false)
	}
	if response.SchemaVersion != 1 || !workspacePermissionClientIDPattern.MatchString(response.ClientID) ||
		response.ChannelID != c.channelID || response.AttachmentID != c.attachmentID ||
		response.OwnerBinding != c.ownerBinding || response.PermissionWaitID != id || response.RequestNonce != requestNonce {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response belongs to a different attachment request", false)
	}
	transport := permissionWaitTransportResponse{
		SchemaVersion: response.SchemaVersion, OK: response.OK, Result: response.Result, Error: response.Error,
	}
	if err := transport.Validate(); err != nil {
		return "", fault.New(fault.KindContract, "permission_wait_transport_failed", "permission wait transport response is invalid", false)
	}
	if !transport.OK {
		return "", fault.New(transport.Error.Kind, transport.Error.Code, transport.Error.Message, transport.Error.Retryable)
	}
	return transport.Result, nil
}
