package dockerruntime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	workspaceBrowserMessageLimit = 20 << 10
	workspaceBrowserSocketEnv    = "TOBARI_BROWSER_SOCKET"
	workspaceBrowserOpenerPath   = "/run/tobari-open"
)

const workspaceBrowserAgentProgram = `import json,os,select,socket,sys
path=sys.argv[1]
limit=20480
if os.path.lexists(path): raise SystemExit(1)
server=socket.socket(socket.AF_UNIX,socket.SOCK_STREAM)
try:
 server.bind(path); os.chmod(path,0o600); server.listen(4)
 while True:
  readable,_,_=select.select([server,sys.stdin.buffer],[],[],0.25)
  if sys.stdin.buffer in readable:
   if os.read(0,1)==b'': break
   raise SystemExit(1)
  if server not in readable: continue
  connection,_=server.accept()
  with connection:
   connection.settimeout(5)
   request=b''
   while b'\n' not in request and len(request)<=limit:
    chunk=connection.recv(4096)
    if not chunk: break
    request+=chunk
   if len(request)>limit or request.count(b'\n')!=1 or not request.endswith(b'\n'):
    continue
   os.write(1,request)
   response=sys.stdin.buffer.readline(4097)
   if not response or len(response)>4096 or not response.endswith(b'\n'): break
   connection.sendall(response)
finally:
 server.close()
 try: os.unlink(path)
 except FileNotFoundError: pass`

type workspaceBrowserRequest struct {
	SchemaVersion int    `json:"schema_version"`
	Target        string `json:"target"`
}

type workspaceBrowserResponse struct {
	SchemaVersion int  `json:"schema_version"`
	OK            bool `json:"ok"`
}

type workspaceBrowserChannel struct {
	socketPath string
	cancel     context.CancelFunc
	requestIn  *io.PipeReader
	response   *io.PipeWriter
	done       chan struct{}
	closeOnce  sync.Once
}

func (r *Runtime) startWorkspaceBrowserChannel(
	ctx context.Context, bridge *workspaceLoginBridge, container string,
) (*workspaceBrowserChannel, error) {
	socketPath, err := newWorkspaceBrowserSocketPath()
	if err != nil {
		return nil, fmt.Errorf("create Workspace browser channel identity: %w", err)
	}
	channel := &workspaceBrowserChannel{socketPath: socketPath}
	runner, ok := r.runner.(workspaceBrowserControlRunner)
	if !ok {
		return channel, nil
	}

	controlContext, cancel := context.WithCancel(ctx)
	responseReader, responseWriter := io.Pipe()
	requestReader, requestWriter := io.Pipe()
	channel.cancel = cancel
	channel.requestIn = requestReader
	channel.response = responseWriter
	channel.done = make(chan struct{})
	uid, gid := currentIDs()
	args := []string{
		"exec", "-i", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		container, "python3", "-c", workspaceBrowserAgentProgram, socketPath,
	}
	go func() {
		defer close(channel.done)
		_ = runner.RunWorkspaceBrowserControl(
			controlContext, args, os.Environ(), responseReader, requestWriter, io.Discard,
		)
		_ = requestWriter.Close()
		_ = responseReader.Close()
	}()
	go channel.serve(bridge)
	return channel, nil
}

func newWorkspaceBrowserSocketPath() (string, error) {
	var entropy [16]byte
	if _, err := io.ReadFull(rand.Reader, entropy[:]); err != nil {
		return "", err
	}
	return "/run/tobari-browser-" + hex.EncodeToString(entropy[:]) + ".sock", nil
}

func (c *workspaceBrowserChannel) environment() []string {
	return []string{
		"BROWSER=" + workspaceBrowserOpenerPath,
		"GH_BROWSER=" + workspaceBrowserOpenerPath,
		workspaceBrowserSocketEnv + "=" + c.socketPath,
	}
}

func (c *workspaceBrowserChannel) serve(bridge *workspaceLoginBridge) {
	if c.requestIn == nil || c.response == nil {
		return
	}
	scanner := bufio.NewScanner(c.requestIn)
	scanner.Buffer(make([]byte, 4096), workspaceBrowserMessageLimit)
	for scanner.Scan() {
		request, ok := decodeWorkspaceBrowserRequest(scanner.Bytes())
		if ok {
			ok = bridge.trigger(request.Target)
		}
		encoded, err := json.Marshal(workspaceBrowserResponse{SchemaVersion: 1, OK: ok})
		if err != nil {
			return
		}
		encoded = append(encoded, '\n')
		if _, err := c.response.Write(encoded); err != nil {
			return
		}
	}
}

func decodeWorkspaceBrowserRequest(line []byte) (workspaceBrowserRequest, bool) {
	if len(line) == 0 || len(line) > workspaceBrowserMessageLimit {
		return workspaceBrowserRequest{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(line))
	var request workspaceBrowserRequest
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return workspaceBrowserRequest{}, false
	}
	seen := make(map[string]struct{}, 2)
	for decoder.More() {
		keyToken, err := decoder.Token()
		key, keyOK := keyToken.(string)
		if err != nil || !keyOK {
			return workspaceBrowserRequest{}, false
		}
		if _, duplicate := seen[key]; duplicate {
			return workspaceBrowserRequest{}, false
		}
		seen[key] = struct{}{}
		switch key {
		case "schema_version":
			if err := decoder.Decode(&request.SchemaVersion); err != nil {
				return workspaceBrowserRequest{}, false
			}
		case "target":
			if err := decoder.Decode(&request.Target); err != nil {
				return workspaceBrowserRequest{}, false
			}
		default:
			return workspaceBrowserRequest{}, false
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') || decoder.Decode(&struct{}{}) != io.EOF {
		return workspaceBrowserRequest{}, false
	}
	if request.SchemaVersion != 1 || request.Target == "" || len(request.Target) > workspaceLoginLineLimit {
		return workspaceBrowserRequest{}, false
	}
	return request, true
}

func (c *workspaceBrowserChannel) close() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.response != nil {
			_ = c.response.Close()
		}
		if c.done != nil {
			select {
			case <-c.done:
			case <-time.After(500 * time.Millisecond):
			}
		}
		if c.cancel != nil {
			c.cancel()
		}
		if c.requestIn != nil {
			_ = c.requestIn.Close()
		}
	})
}
