package dockerruntime

import (
	"context"
	"crypto/sha256"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	workspaceLoginLineLimit = 16 << 10
	workspaceLoginURLBudget = 8
)

const workspaceLoopbackProxyProgram = `import os,selectors,socket,sys
s=socket.create_connection(("127.0.0.1",int(sys.argv[1])),5)
q=selectors.DefaultSelector()
q.register(0,selectors.EVENT_READ)
q.register(s,selectors.EVENT_READ)
stdin_open=True
while True:
 for key,_ in q.select():
  if key.fileobj==0:
   data=os.read(0,65536)
   if not data:
    q.unregister(0); stdin_open=False; s.shutdown(socket.SHUT_WR); continue
   s.sendall(data)
  else:
   data=s.recv(65536)
   if not data: raise SystemExit(0)
   os.write(1,data)`

type workspaceCallbackListener func(string) (net.Listener, error)

type workspaceLoginBridge struct {
	ctx       context.Context
	runtime   *Runtime
	container string
	projectID string
	listen    workspaceCallbackListener

	mu       sync.Mutex
	listener net.Listener
	attempts int
	seen     map[[sha256.Size]byte]struct{}
}

func newWorkspaceLoginBridge(ctx context.Context, runtime *Runtime, container, projectID string) *workspaceLoginBridge {
	return &workspaceLoginBridge{
		ctx: ctx, runtime: runtime, container: container, projectID: projectID,
		listen: func(address string) (net.Listener, error) { return net.Listen("tcp4", address) },
		seen:   make(map[[sha256.Size]byte]struct{}),
	}
}

func (b *workspaceLoginBridge) trigger(target string) bool {
	action, ok := parseWorkspaceLoginBrowserAction(target)
	if !ok {
		return false
	}
	digest := sha256.Sum256([]byte(target))
	b.mu.Lock()
	if b.attempts >= workspaceLoginURLBudget {
		b.mu.Unlock()
		return false
	}
	b.attempts++
	if _, duplicate := b.seen[digest]; duplicate {
		b.mu.Unlock()
		return true
	}
	b.mu.Unlock()
	if err := b.runtime.verifyOwnedProjectResource(b.ctx, "container", b.container, b.projectID, projectWorkRole); err != nil {
		return false
	}
	if !action.relayCallback {
		if b.runtime.browser.Open(b.ctx, target) != nil {
			return false
		}
		b.markOpened(digest)
		return true
	}
	listener, err := b.listen(net.JoinHostPort("127.0.0.1", strconv.Itoa(action.callbackPort)))
	if err != nil {
		return false
	}
	b.mu.Lock()
	if b.listener != nil {
		b.mu.Unlock()
		_ = listener.Close()
		return false
	}
	b.listener = listener
	b.mu.Unlock()

	go b.serve(listener, action.callbackPort)
	if err := b.runtime.browser.Open(b.ctx, target); err != nil {
		b.closeListener(listener)
		return false
	}
	b.markOpened(digest)
	return true
}

func (b *workspaceLoginBridge) markOpened(digest [sha256.Size]byte) {
	b.mu.Lock()
	b.seen[digest] = struct{}{}
	b.mu.Unlock()
}

func (b *workspaceLoginBridge) serve(listener net.Listener, callbackPort int) {
	defer b.closeListener(listener)
	stop := make(chan struct{})
	go func() {
		select {
		case <-b.ctx.Done():
			_ = listener.Close()
		case <-stop:
		}
	}()
	defer close(stop)
	connection, err := listener.Accept()
	if err != nil {
		return
	}
	_ = listener.Close()
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(2 * time.Minute))
	uid, gid := currentIDs()
	args := []string{
		"exec", "-i", "--user", strconv.Itoa(uid) + ":" + strconv.Itoa(gid),
		b.container, "python3", "-c", workspaceLoopbackProxyProgram, strconv.Itoa(callbackPort),
	}
	_ = b.runtime.runner.Run(b.ctx, args, os.Environ(), connection, connection, io.Discard)
}

func (b *workspaceLoginBridge) closeListener(listener net.Listener) {
	b.mu.Lock()
	if b.listener == listener {
		b.listener = nil
	}
	b.mu.Unlock()
	_ = listener.Close()
}

func (b *workspaceLoginBridge) close() {
	b.mu.Lock()
	listener := b.listener
	b.listener = nil
	b.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
}
