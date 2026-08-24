package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
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
	"strings"
	"sync"
	"time"

	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

const (
	workspaceServiceSocketEnv       = "TOBARI_SERVICE_SOCKET"
	workspaceServiceMessageLimit    = 32 << 10
	workspaceServiceReadyTimeout    = 5 * time.Second
	workspaceServiceHeaderLimit     = 32 << 10
	workspaceServiceSetupTimeout    = 5 * time.Second
	workspaceServiceShutdownTimeout = 2 * time.Second
	workspaceServicePendingLimit    = 64
	workspaceServiceExposureLimit   = 16
	workspaceServiceConnectionLimit = 64
)

const workspaceServiceAgentProgram = `import json,os,queue,select,socket,sys,threading,uuid
path=sys.argv[1]; limit=32768; write_lock=threading.Lock(); responses={}; responses_lock=threading.Lock()
if os.path.lexists(path): raise SystemExit(1)
server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM); server.bind(path); os.chmod(path,0o600); server.listen(16)
def emit(value):
 data=(json.dumps(value,separators=(',',':'))+'\n').encode()
 with write_lock: os.write(1,data)
def read_responses():
 while True:
  line=sys.stdin.buffer.readline(limit+1)
  if not line or len(line)>limit: os._exit(0)
  try: value=json.loads(line)
  except Exception: os._exit(1)
  client=value.get('client_id')
  with responses_lock: target=responses.get(client)
  if target is not None: target.put(value)
def serve(connection):
 client=uuid.uuid4().hex; target=queue.Queue(maxsize=1)
 with responses_lock: responses[client]=target
 try:
  connection.settimeout(5); data=b''
  while b'\n' not in data and len(data)<=limit:
   part=connection.recv(4096)
   if not part: return
   data+=part
  if len(data)>limit or data.count(b'\n')!=1 or not data.endswith(b'\n'): return
  request=json.loads(data); request['client_id']=client; emit(request); connection.settimeout(None)
  while True:
   try: response=target.get(timeout=.1); break
   except queue.Empty:
    readable,_,_=select.select([connection],[],[],0)
    if readable and connection.recv(1,socket.MSG_PEEK)==b'':
     emit({'schema_version':1,'operation':'cancel','client_id':client}); return
  response.pop('client_id',None); connection.sendall((json.dumps(response,separators=(',',':'))+'\n').encode())
 finally:
  with responses_lock: responses.pop(client,None)
  connection.close()
threading.Thread(target=read_responses,daemon=True).start(); emit({'schema_version':1,'ready':True})
try:
 while True:
  connection,_=server.accept(); threading.Thread(target=serve,args=(connection,),daemon=True).start()
finally:
 server.close()
 try: os.unlink(path)
 except FileNotFoundError: pass`

const workspaceServiceStreamProgram = `import os,select,socket,sys
s=socket.create_connection(('127.0.0.1',int(sys.argv[1])),5); inputs=[sys.stdin.buffer,s]
while inputs:
 readable,_,_=select.select(inputs,[],[],1)
 for source in readable:
  data=source.read1(65536) if source is sys.stdin.buffer else source.recv(65536)
  if not data:
   inputs.remove(source)
   try: s.shutdown(socket.SHUT_WR)
   except OSError: pass
   continue
  (s.sendall(data) if source is sys.stdin.buffer else os.write(1,data))`

type workspaceServiceControlRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	ClientID      string `json:"client_id"`
	Port          int    `json:"port,omitempty"`
	ExposureID    string `json:"exposure_id,omitempty"`
}

type workspaceServiceControlResponse struct {
	SchemaVersion int                             `json:"schema_version"`
	ClientID      string                          `json:"client_id,omitempty"`
	OK            bool                            `json:"ok"`
	Code          string                          `json:"code,omitempty"`
	Exposure      *tobari.ServiceExposure         `json:"exposure,omitempty"`
	Status        *tobari.ServiceAttachmentStatus `json:"status,omitempty"`
}

type workspaceServicePending struct {
	request  tobari.ServiceRequest
	clientID string
}

type workspaceServiceExposureRuntime struct {
	exposure tobari.ServiceExposure
	listener *net.TCPListener
	cancel   context.CancelFunc
	active   map[net.Conn]struct{}
	closing  bool
}

type workspaceServiceController struct {
	runtime          *Runtime
	principal        interactiveWorkspacePrincipal
	container        string
	attachmentID     string
	nonce            string
	workspaceSocket  string
	rendezvousSocket string
	recordPath       string
	controlCancel    context.CancelFunc
	attachmentCtx    context.Context
	controlResponse  *io.PipeWriter
	controlMu        sync.Mutex
	rendezvous       *net.UnixListener
	mu               sync.Mutex
	pending          map[string]*workspaceServicePending
	exposures        map[string]*workspaceServiceExposureRuntime
	closed           bool
	done             chan struct{}
	once             sync.Once
	closeReceipt     tobari.ServiceCleanupReceipt
	closeErr         error
}

type serviceRendezvousRecord struct {
	SchemaVersion int    `json:"schema_version"`
	AttachmentID  string `json:"attachment_id"`
	ContextID     string `json:"context_id"`
	WorkspaceID   string `json:"workspace_id"`
	Context       string `json:"context"`
	ProjectRoot   string `json:"project_root"`
	Nonce         string `json:"nonce"`
	SocketName    string `json:"socket_name"`
	OwnerPID      int    `json:"owner_pid"`
	OwnerUID      int    `json:"owner_uid"`
}

type serviceRendezvousRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Operation     string `json:"operation"`
	AttachmentID  string `json:"attachment_id"`
	Nonce         string `json:"nonce"`
	RequestID     string `json:"request_id,omitempty"`
	ExposureID    string `json:"exposure_id,omitempty"`
	ReviewerPID   int    `json:"reviewer_pid"`
}

type serviceRendezvousResponse struct {
	SchemaVersion int                      `json:"schema_version"`
	AttachmentID  string                   `json:"attachment_id"`
	ContextID     string                   `json:"context_id"`
	WorkspaceID   string                   `json:"workspace_id"`
	OK            bool                     `json:"ok"`
	Code          string                   `json:"code,omitempty"`
	Requests      []tobari.ServiceRequest  `json:"requests"`
	Exposures     []tobari.ServiceExposure `json:"exposures"`
	Exposure      *tobari.ServiceExposure  `json:"exposure,omitempty"`
}

func serviceEntropy(prefix string, bytesCount int) (string, error) {
	raw := make([]byte, bytesCount)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

func (r *Runtime) serviceExposureLiveDirectory() string {
	return filepath.Join(r.stateDirectory, "service-exposure", "live")
}

func (r *Runtime) serviceExposureSocketDirectory() string {
	return filepath.Join("/tmp", "tobari-"+strconv.Itoa(os.Getuid()), "service-review")
}

func (r *Runtime) startWorkspaceServiceController(ctx context.Context, instance tobari.Workspace, container string) (*workspaceServiceController, error) {
	principal, err := legacyInteractiveWorkspacePrincipal(instance)
	if err != nil {
		return nil, err
	}
	return r.startWorkspaceServiceControllerForPrincipal(ctx, principal, container)
}

func (r *Runtime) startWorkspaceServiceControllerForPrincipal(ctx context.Context, principal interactiveWorkspacePrincipal, container string) (*workspaceServiceController, error) {
	if err := principal.validate(); err != nil {
		return nil, err
	}
	runner, ok := r.runner.(workspaceServiceControlRunner)
	if !ok {
		return &workspaceServiceController{}, nil
	}
	attachmentID, err := newAttachmentEpochID()
	if err != nil {
		return nil, err
	}
	nonce, err := serviceEntropy("", 32)
	if err != nil {
		return nil, err
	}
	workspaceIdentity, err := serviceEntropy("", 16)
	if err != nil {
		return nil, err
	}
	controller := &workspaceServiceController{
		runtime: r, principal: principal, container: container, attachmentID: attachmentID, nonce: nonce,
		workspaceSocket: "/run/tobari-service-" + workspaceIdentity + ".sock",
		pending:         map[string]*workspaceServicePending{}, exposures: map[string]*workspaceServiceExposureRuntime{}, done: make(chan struct{}),
	}
	if err := r.ensurePrivateDirectory(r.serviceExposureLiveDirectory()); err != nil {
		return nil, err
	}
	if err := r.ensurePrivateDirectory(r.serviceExposureSocketDirectory()); err != nil {
		return nil, err
	}
	socketName := "owner-" + nonce[:32] + ".sock"
	controller.rendezvousSocket = filepath.Join(r.serviceExposureSocketDirectory(), socketName)
	controller.recordPath = filepath.Join(r.serviceExposureLiveDirectory(), attachmentID+".json")
	rendezvous, err := net.ListenUnix("unix", &net.UnixAddr{Name: controller.rendezvousSocket, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("listen for service review: %w", err)
	}
	controller.rendezvous = rendezvous
	if err := os.Chmod(controller.rendezvousSocket, 0o600); err != nil {
		_ = controller.Close(ctx)
		return nil, err
	}
	record := serviceRendezvousRecord{
		SchemaVersion: 1, AttachmentID: attachmentID,
		ContextID: principal.contextID, WorkspaceID: principal.workspaceID,
		Context: principal.contextPresentation, ProjectRoot: principal.projectRoot,
		Nonce: nonce, SocketName: socketName, OwnerPID: os.Getpid(), OwnerUID: os.Getuid(),
	}
	if err := writeAtomicJSON(controller.recordPath, record); err != nil {
		_ = controller.Close(ctx)
		return nil, err
	}
	go controller.serveRendezvous()

	controlContext, cancel := context.WithCancel(ctx)
	controller.controlCancel = cancel
	controller.attachmentCtx = controlContext
	responseReader, responseWriter := io.Pipe()
	requestReader, requestWriter := io.Pipe()
	controller.controlResponse = responseWriter
	ready := make(chan struct{})
	go func() {
		args := []string{"exec", "-i", container, "python3", "-c", workspaceServiceAgentProgram, controller.workspaceSocket}
		_ = runner.RunWorkspaceServiceControl(controlContext, args, os.Environ(), responseReader, requestWriter, io.Discard)
		_ = requestWriter.Close()
		_ = responseReader.Close()
	}()
	go controller.serveControl(requestReader, ready)
	select {
	case <-ready:
		return controller, nil
	case <-time.After(workspaceServiceReadyTimeout):
		_ = controller.Close(ctx)
		return nil, fmt.Errorf("Workspace service control did not become ready")
	case <-ctx.Done():
		_ = controller.Close(ctx)
		return nil, ctx.Err()
	}
}

func (c *workspaceServiceController) environment() []string {
	if c == nil || c.workspaceSocket == "" {
		return []string{}
	}
	return []string{workspaceServiceSocketEnv + "=" + c.workspaceSocket}
}

func (c *workspaceServiceController) serveControl(reader io.Reader, ready chan struct{}) {
	defer close(c.done)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), workspaceServiceMessageLimit)
	readyOnce := sync.Once{}
	for scanner.Scan() {
		line := append([]byte{}, scanner.Bytes()...)
		var marker struct {
			SchemaVersion int  `json:"schema_version"`
			Ready         bool `json:"ready"`
		}
		if json.Unmarshal(line, &marker) == nil && marker.SchemaVersion == 1 && marker.Ready {
			readyOnce.Do(func() { close(ready) })
			continue
		}
		var request workspaceServiceControlRequest
		if decodeStrictJSON(line, &request) != nil || request.SchemaVersion != 1 || len(request.ClientID) != 32 {
			continue
		}
		go c.handleControl(request)
	}
}

func (c *workspaceServiceController) handleControl(request workspaceServiceControlRequest) {
	switch request.Operation {
	case "request":
		c.submit(request)
	case "status":
		status := c.snapshotAttachmentStatus()
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: request.ClientID, OK: true, Status: &status})
	case "stop":
		code := ""
		err := c.stop(request.ExposureID)
		if err != nil {
			code = "service_exposure_not_found"
		}
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: request.ClientID, OK: err == nil, Code: code})
	case "cancel":
		c.withdrawClient(request.ClientID)
	default:
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: request.ClientID, Code: "invalid_service_operation"})
	}
}

func (c *workspaceServiceController) submit(input workspaceServiceControlRequest) {
	if tobari.ValidateServicePort(input.Port) != nil {
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: input.ClientID, Code: "invalid_service_port"})
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: input.ClientID, Code: "service_attachment_closed"})
		return
	}
	if len(c.pending) >= workspaceServicePendingLimit {
		c.mu.Unlock()
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: input.ClientID, Code: "service_request_limit"})
		return
	}
	for _, active := range c.exposures {
		if active.exposure.TargetPort == input.Port {
			exposure := active.exposure
			c.mu.Unlock()
			c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: input.ClientID, OK: true, Exposure: &exposure})
			return
		}
	}
	requestID, err := serviceEntropy("srq_", 16)
	if err != nil {
		c.mu.Unlock()
		c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: input.ClientID, Code: "service_request_failed"})
		return
	}
	request := tobari.ServiceRequest{
		SchemaVersion: 1, ID: requestID, AttachmentID: c.attachmentID,
		ContextID: c.principal.contextID, WorkspaceID: c.principal.workspaceID,
		Context: c.principal.contextPresentation, ProjectRoot: c.principal.projectRoot,
		TargetPort: input.Port, State: tobari.ServiceStatePending,
	}
	c.pending[requestID] = &workspaceServicePending{request: request, clientID: input.ClientID}
	c.mu.Unlock()
}

func (c *workspaceServiceController) respond(response workspaceServiceControlResponse) {
	encoded, _ := json.Marshal(response)
	encoded = append(encoded, '\n')
	c.controlMu.Lock()
	defer c.controlMu.Unlock()
	if c.controlResponse != nil {
		_, _ = c.controlResponse.Write(encoded)
	}
}

func (c *workspaceServiceController) withdrawClient(clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, pending := range c.pending {
		if pending.clientID == clientID {
			delete(c.pending, id)
		}
	}
}

func (c *workspaceServiceController) snapshotRequests() []tobari.ServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]tobari.ServiceRequest, 0, len(c.pending))
	for _, pending := range c.pending {
		result = append(result, pending.request)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func (c *workspaceServiceController) snapshotAttachmentStatus() tobari.ServiceAttachmentStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := tobari.ServiceAttachmentStatus{SchemaVersion: 1, Scope: tobari.ServiceAttachmentScope, AttachmentID: c.attachmentID, Pending: []tobari.ServicePendingStatus{}, Exposures: []tobari.ServiceExposure{}}
	for _, pending := range c.pending {
		result.Pending = append(result.Pending, tobari.ServicePendingStatus{TargetPort: pending.request.TargetPort, State: tobari.ServiceStatePending})
	}
	sort.Slice(result.Pending, func(i, j int) bool { return result.Pending[i].TargetPort < result.Pending[j].TargetPort })
	for _, active := range c.exposures {
		exposure := active.exposure
		exposure.Connections = len(active.active)
		if exposure.Connections > 0 {
			exposure.State = tobari.ServiceStateRelaying
		}
		result.Exposures = append(result.Exposures, exposure)
	}
	sort.Slice(result.Exposures, func(i, j int) bool { return result.Exposures[i].ID < result.Exposures[j].ID })
	return result
}

func (c *workspaceServiceController) snapshotExposures() tobari.ServiceAttachmentStatus {
	return c.snapshotAttachmentStatus()
}

func (c *workspaceServiceController) allow(requestID string) (tobari.ServiceExposure, error) {
	c.mu.Lock()
	pending, ok := c.pending[requestID]
	if !ok || c.closed {
		c.mu.Unlock()
		return tobari.ServiceExposure{}, errors.New("request is stale")
	}
	if len(c.exposures) >= workspaceServiceExposureLimit {
		c.mu.Unlock()
		return tobari.ServiceExposure{}, errors.New("service exposure limit reached")
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		c.mu.Unlock()
		return tobari.ServiceExposure{}, err
	}
	exposureID, err := serviceEntropy("exp_", 16)
	if err != nil {
		_ = listener.Close()
		c.mu.Unlock()
		return tobari.ServiceExposure{}, err
	}
	originLabel, err := serviceEntropy("", 16)
	if err != nil {
		_ = listener.Close()
		c.mu.Unlock()
		return tobari.ServiceExposure{}, err
	}
	hostPort := listener.Addr().(*net.TCPAddr).Port
	accessURL, err := tobari.ServiceExposureURL(hostPort, originLabel)
	if err != nil {
		_ = listener.Close()
		c.mu.Unlock()
		return tobari.ServiceExposure{}, err
	}
	exposure := tobari.ServiceExposure{
		SchemaVersion: 1, ID: exposureID, RequestID: requestID, AttachmentID: c.attachmentID,
		ContextID: c.principal.contextID, WorkspaceID: c.principal.workspaceID,
		Context: c.principal.contextPresentation, ProjectRoot: c.principal.projectRoot,
		TargetPort: pending.request.TargetPort, HostPort: hostPort, URL: accessURL,
		State: tobari.ServiceStateListening,
	}
	exposureContext, cancel := context.WithCancel(c.attachmentCtx)
	active := &workspaceServiceExposureRuntime{exposure: exposure, listener: listener, cancel: cancel, active: map[net.Conn]struct{}{}}
	c.exposures[exposureID] = active
	delete(c.pending, requestID)
	clientID := pending.clientID
	c.mu.Unlock()
	go c.serveExposure(exposureContext, active)
	c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: clientID, OK: true, Exposure: &exposure})
	return exposure, nil
}

func (c *workspaceServiceController) deny(requestID string) error {
	c.mu.Lock()
	pending, ok := c.pending[requestID]
	if ok {
		delete(c.pending, requestID)
	}
	c.mu.Unlock()
	if !ok {
		return errors.New("request is stale")
	}
	c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: pending.clientID, Code: "service_request_denied"})
	return nil
}

func (c *workspaceServiceController) stop(exposureID string) error {
	if tobari.ValidateServiceExposureID(exposureID) != nil {
		return errors.New("invalid exposure")
	}
	c.mu.Lock()
	active, ok := c.exposures[exposureID]
	if !ok {
		c.mu.Unlock()
		return errors.New("exposure not found")
	}
	if active.closing {
		c.mu.Unlock()
		return errors.New("exposure is already closing")
	}
	active.closing = true
	_ = active.listener.Close()
	active.cancel()
	connections := make([]net.Conn, 0, len(active.active))
	for connection := range active.active {
		connections = append(connections, connection)
	}
	c.mu.Unlock()
	for _, connection := range connections {
		_ = connection.Close()
	}
	deadline := time.Now().Add(workspaceServiceShutdownTimeout)
	for {
		c.mu.Lock()
		current, exists := c.exposures[exposureID]
		if !exists || current != active {
			c.mu.Unlock()
			return errors.New("exposure owner changed during stop")
		}
		if len(active.active) == 0 {
			delete(c.exposures, exposureID)
			c.mu.Unlock()
			return nil
		}
		c.mu.Unlock()
		if time.Now().After(deadline) {
			return errors.New("service exposure streams did not close")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (c *workspaceServiceController) serveExposure(ctx context.Context, active *workspaceServiceExposureRuntime) {
	for {
		connection, err := active.listener.Accept()
		if err != nil {
			return
		}
		c.mu.Lock()
		current, exists := c.exposures[active.exposure.ID]
		if c.closed || !exists || current != active || active.closing || len(active.active) >= workspaceServiceConnectionLimit {
			c.mu.Unlock()
			_ = connection.Close()
			return
		}
		active.active[connection] = struct{}{}
		c.mu.Unlock()
		go func() {
			defer func() { c.mu.Lock(); delete(active.active, connection); c.mu.Unlock(); _ = connection.Close() }()
			c.relayHTTP(ctx, active.exposure, connection)
		}()
	}
}

func (c *workspaceServiceController) setExposureState(id, state string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if active, exists := c.exposures[id]; exists {
		active.exposure.State = state
	}
}

func (c *workspaceServiceController) serveRendezvous() {
	for {
		connection, err := c.rendezvous.AcceptUnix()
		if err != nil {
			return
		}
		go c.handleRendezvous(connection)
	}
}

func (c *workspaceServiceController) handleRendezvous(connection *net.UnixConn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(workspaceServiceSetupTimeout))
	peerPID, peerUID, err := servicePeerIdentity(connection)
	if err != nil || (peerUID >= 0 && peerUID != os.Getuid()) {
		return
	}
	line, err := bufio.NewReader(io.LimitReader(connection, workspaceServiceMessageLimit+1)).ReadBytes('\n')
	if err != nil || len(line) > workspaceServiceMessageLimit {
		return
	}
	var request serviceRendezvousRequest
	if decodeStrictJSON(bytes.TrimSuffix(line, []byte{'\n'}), &request) != nil || request.SchemaVersion != 1 || request.ReviewerPID != peerPID || request.AttachmentID != c.attachmentID || request.Nonce != c.nonce {
		return
	}
	response := serviceRendezvousResponse{SchemaVersion: 1, AttachmentID: c.attachmentID, ContextID: c.principal.contextID, WorkspaceID: c.principal.workspaceID, Requests: []tobari.ServiceRequest{}, Exposures: []tobari.ServiceExposure{}}
	switch request.Operation {
	case "snapshot":
		response.OK = true
		response.Requests = c.snapshotRequests()
		response.Exposures = c.snapshotExposures().Exposures
	case "allow":
		exposure, err := c.allow(request.RequestID)
		response.OK = err == nil
		if err == nil {
			response.Exposure = &exposure
		} else {
			response.Code = "service_request_stale"
		}
	case "deny":
		err := c.deny(request.RequestID)
		response.OK = err == nil
		if err != nil {
			response.Code = "service_request_stale"
		}
	case "stop":
		err := c.stop(request.ExposureID)
		response.OK = err == nil
		if err != nil {
			response.Code = "service_exposure_stale"
		}
	default:
		response.Code = "invalid_service_operation"
	}
	encoded, _ := json.Marshal(response)
	_, _ = connection.Write(append(encoded, '\n'))
}

func (c *workspaceServiceController) Close(ctx context.Context) error {
	_, err := c.CloseWithReceipt(ctx)
	return err
}

func (c *workspaceServiceController) CloseWithReceipt(_ context.Context) (tobari.ServiceCleanupReceipt, error) {
	if c == nil {
		return tobari.ServiceCleanupReceipt{SchemaVersion: 1}, nil
	}
	c.once.Do(func() {
		c.closeReceipt.SchemaVersion = 1
		if c.workspaceSocket == "" {
			return
		}
		c.mu.Lock()
		c.closed = true
		pending := make(map[string]*workspaceServicePending, len(c.pending))
		for id, request := range c.pending {
			pending[id] = request
		}
		exposures := make(map[string]*workspaceServiceExposureRuntime, len(c.exposures))
		for id, active := range c.exposures {
			exposures[id] = active
		}
		connections := []net.Conn{}
		for _, active := range exposures {
			active.closing = true
			for connection := range active.active {
				connections = append(connections, connection)
			}
		}
		c.mu.Unlock()
		// Withdraw pending requests and close listener admission before
		// terminating streams. Owner state remains until closure is confirmed.
		c.closeReceipt.PendingWithdrawnCount = len(pending)
		for _, active := range exposures {
			_ = active.listener.Close()
			active.cancel()
		}
		for _, connection := range connections {
			_ = connection.Close()
		}
		for _, request := range pending {
			c.respond(workspaceServiceControlResponse{SchemaVersion: 1, ClientID: request.clientID, Code: "service_attachment_closed"})
		}
		deadline := time.Now().Add(workspaceServiceShutdownTimeout)
		for {
			c.mu.Lock()
			remaining := 0
			for _, active := range exposures {
				remaining += len(active.active)
			}
			c.mu.Unlock()
			if remaining == 0 {
				c.closeReceipt.ExposureClosedCount = len(exposures)
				c.closeReceipt.StreamClosedCount = len(connections)
				break
			}
			if time.Now().After(deadline) {
				c.closeErr = errors.New("service exposure streams did not close during attachment teardown")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		c.mu.Lock()
		c.pending = map[string]*workspaceServicePending{}
		c.exposures = map[string]*workspaceServiceExposureRuntime{}
		c.mu.Unlock()
		if c.rendezvous != nil {
			_ = c.rendezvous.Close()
		}
		if c.controlResponse != nil {
			_ = c.controlResponse.Close()
		}
		if c.controlCancel != nil {
			c.controlCancel()
		}
		_ = os.Remove(c.recordPath)
		_ = os.Remove(c.rendezvousSocket)
		if c.done != nil {
			select {
			case <-c.done:
			case <-time.After(workspaceServiceShutdownTimeout):
				c.closeErr = errors.New("service attachment control did not stop")
			}
		}
	})
	return c.closeReceipt, c.closeErr
}

// ServiceExposureClient is the attachment-local helper adapter.
type ServiceExposureClient struct {
	socket string
}

func NewServiceExposureClientFromEnvironment() (*ServiceExposureClient, error) {
	socket := os.Getenv(workspaceServiceSocketEnv)
	if socket == "" || !strings.HasPrefix(socket, "/run/tobari-service-") || !strings.HasSuffix(socket, ".sock") {
		return nil, errors.New("service attachment channel is unavailable")
	}
	return &ServiceExposureClient{socket: socket}, nil
}

func (c *ServiceExposureClient) call(ctx context.Context, request workspaceServiceControlRequest) (workspaceServiceControlResponse, error) {
	dialer := net.Dialer{}
	connection, err := dialer.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return workspaceServiceControlResponse{}, fault.Wrap(fault.KindUnavailable, "service_attachment_unavailable", "Workspace service attachment is unavailable", false, err)
	}
	defer connection.Close()
	cancelWatchDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.SetDeadline(time.Now())
		case <-cancelWatchDone:
		}
	}()
	defer close(cancelWatchDone)
	encoded, _ := json.Marshal(request)
	if _, err = connection.Write(append(encoded, '\n')); err != nil {
		return workspaceServiceControlResponse{}, err
	}
	line, err := bufio.NewReader(io.LimitReader(connection, workspaceServiceMessageLimit+1)).ReadBytes('\n')
	if err != nil {
		return workspaceServiceControlResponse{}, err
	}
	var response workspaceServiceControlResponse
	if decodeStrictJSON(bytes.TrimSuffix(line, []byte{'\n'}), &response) != nil || response.SchemaVersion != 1 {
		return workspaceServiceControlResponse{}, errors.New("invalid service attachment response")
	}
	if !response.OK {
		return response, fault.New(fault.KindRejected, response.Code, "Workspace service request was not accepted", false)
	}
	return response, nil
}

func (c *ServiceExposureClient) RequestService(ctx context.Context, port int) (tobari.ServiceExposure, error) {
	response, err := c.call(ctx, workspaceServiceControlRequest{SchemaVersion: 1, Operation: "request", Port: port})
	if err != nil {
		return tobari.ServiceExposure{}, err
	}
	if response.Exposure == nil {
		return tobari.ServiceExposure{}, errors.New("missing service exposure")
	}
	return *response.Exposure, nil
}
func (c *ServiceExposureClient) AttachmentServiceStatus(ctx context.Context) (tobari.ServiceAttachmentStatus, error) {
	response, err := c.call(ctx, workspaceServiceControlRequest{SchemaVersion: 1, Operation: "status"})
	if err != nil {
		return tobari.ServiceAttachmentStatus{}, err
	}
	if response.Status == nil {
		return tobari.ServiceAttachmentStatus{}, errors.New("missing service attachment status")
	}
	return *response.Status, nil
}
func (c *ServiceExposureClient) StopServiceExposure(ctx context.Context, id string) error {
	_, err := c.call(ctx, workspaceServiceControlRequest{SchemaVersion: 1, Operation: "stop", ExposureID: id})
	return err
}
func (*ServiceExposureClient) ReviewServiceRequests(context.Context) (tobari.ServiceReviewSnapshot, error) {
	return tobari.ServiceReviewSnapshot{}, errors.New("host review is unavailable inside the Workspace")
}
func (*ServiceExposureClient) ServiceStatus(context.Context) (tobari.ServiceStatusSnapshot, error) {
	return tobari.ServiceStatusSnapshot{}, errors.New("host status is unavailable inside the Workspace")
}
func (*ServiceExposureClient) AllowServiceRequest(context.Context, string) (tobari.ServiceExposure, error) {
	return tobari.ServiceExposure{}, errors.New("host review is unavailable inside the Workspace")
}
func (*ServiceExposureClient) DenyServiceRequest(context.Context, string) error {
	return errors.New("host review is unavailable inside the Workspace")
}
func (*ServiceExposureClient) OpenServiceExposure(context.Context, string) (tobari.ServiceOpenResult, error) {
	return tobari.ServiceOpenResult{}, errors.New("host browser opening is unavailable inside the Workspace")
}
