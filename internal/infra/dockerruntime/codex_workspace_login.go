package dockerruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	workspaceLoginLineLimit = 16 << 10
	codexLoginLineLimit     = workspaceLoginLineLimit
	codexLoginFrameLimit    = 64 << 10
	workspaceLoginURLBudget = 8
)

var (
	codexSynchronizedFrameStart = []byte("\x1b[?2026h")
	codexSynchronizedFrameEnd   = []byte("\x1b[?2026l")
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

const codexLoopbackProxyProgram = workspaceLoopbackProxyProgram

type workspaceCallbackListener func(string) (net.Listener, error)

type codexCallbackListener = workspaceCallbackListener

type workspaceLoginBridge struct {
	ctx       context.Context
	runtime   *Runtime
	container string
	projectID string
	listen    workspaceCallbackListener

	mu       sync.Mutex
	listener net.Listener
}

func newWorkspaceLoginBridge(ctx context.Context, runtime *Runtime, container, projectID string) *workspaceLoginBridge {
	return &workspaceLoginBridge{
		ctx: ctx, runtime: runtime, container: container, projectID: projectID,
		listen: func(address string) (net.Listener, error) { return net.Listen("tcp4", address) },
	}
}

type codexWorkspaceLoginBridge = workspaceLoginBridge

func newCodexWorkspaceLoginBridge(ctx context.Context, runtime *Runtime, container, projectID string) *workspaceLoginBridge {
	return newWorkspaceLoginBridge(ctx, runtime, container, projectID)
}

type workspaceLoginBrowserAction struct {
	callbackPort  int
	relayCallback bool
}

func (b *workspaceLoginBridge) trigger(target string) bool {
	action, ok := parseWorkspaceLoginBrowserAction(target)
	if !ok {
		return false
	}
	if err := b.runtime.verifyOwnedProjectResource(b.ctx, "container", b.container, b.projectID, projectWorkRole); err != nil {
		return false
	}
	if !action.relayCallback {
		return b.runtime.browser.Open(b.ctx, target) == nil
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
	return true
}

func parseWorkspaceLoginBrowserAction(target string) (workspaceLoginBrowserAction, bool) {
	if target == githubDeviceURL {
		return workspaceLoginBrowserAction{}, true
	}
	if validWorkspaceClaudeLoginAuthorizationURL(target) {
		return workspaceLoginBrowserAction{}, true
	}
	authorization, ok := parseWorkspaceLoginAuthorizationURL(target)
	if !ok {
		return workspaceLoginBrowserAction{}, false
	}
	return workspaceLoginBrowserAction{callbackPort: authorization.callbackPort, relayCallback: true}, true
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

type workspaceLoginOutputObserver struct {
	mu              sync.Mutex
	seen            map[[sha256.Size]byte]struct{}
	claudeCandidate string
	trigger         func(string) bool
}

type codexLoginOutputObserver = workspaceLoginOutputObserver

func (t *workspaceLoginOutputObserver) observe(line string) {
	line = strings.TrimRight(line, "\r")
	if target, ok := githubBrowserURLFromNonInteractiveLine(line); ok {
		t.observeTarget(target)
		return
	}
	target, ok := workspaceAuthorizationURLFromLine(line)
	if ok {
		t.observeTarget(target)
		return
	}
	t.observeWrappedClaudeTarget(line)
}

func (t *workspaceLoginOutputObserver) observeWrappedClaudeTarget(line string) {
	t.mu.Lock()
	fragment := strings.TrimSpace(line)
	if start := strings.Index(fragment, claudeLoginURLPrefix); start >= 0 {
		fragment = fragment[start:]
		t.claudeCandidate = ""
	} else if t.claudeCandidate == "" {
		t.mu.Unlock()
		return
	}
	if fragment == "" || len(t.claudeCandidate)+len(fragment) > workspaceLoginLineLimit ||
		!isClaudeURLFragment(fragment) {
		t.claudeCandidate = ""
		t.mu.Unlock()
		return
	}
	t.claudeCandidate += fragment
	if !validWorkspaceClaudeLoginAuthorizationURL(t.claudeCandidate) {
		t.mu.Unlock()
		return
	}
	target := t.claudeCandidate
	t.claudeCandidate = ""
	t.mu.Unlock()
	t.observeTarget(target)
}

func isClaudeURLFragment(value string) bool {
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune(":/?&=+%._~-", character) {
			continue
		}
		return false
	}
	return true
}

func githubBrowserURLFromNonInteractiveLine(line string) (string, bool) {
	const prefix = "Open this URL to continue in your web browser: "
	const styledPrefix = "\x1b[0;1;39mOpen this URL\x1b[0m to continue in your web browser: "
	// GitHub CLI 2.96.0 emits either the plain prefix or this one exact
	// mgutz/ansi Bold rendering. Do not strip arbitrary terminal controls here:
	// output framing is compatibility evidence, not URL authority.
	if line != prefix+githubDeviceURL && line != styledPrefix+githubDeviceURL {
		return "", false
	}
	return githubDeviceURL, true
}

func (t *workspaceLoginOutputObserver) observeTarget(target string) {
	digest := sha256.Sum256([]byte(target))
	t.mu.Lock()
	if t.seen == nil {
		t.seen = make(map[[sha256.Size]byte]struct{})
	}
	if _, duplicate := t.seen[digest]; duplicate || len(t.seen) >= workspaceLoginURLBudget {
		t.mu.Unlock()
		return
	}
	t.seen[digest] = struct{}{}
	t.mu.Unlock()
	_ = t.trigger(target)
}

func codexAuthorizationURLFromLine(line string) (string, bool) {
	return workspaceAuthorizationURLFromLine(line)
}

func workspaceAuthorizationURLFromLine(line string) (string, bool) {
	if target, ok := claudeWorkspaceHyperlinkTarget(line); ok {
		return target, true
	}
	var target string
	urlCount := 0
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "https://") || strings.HasPrefix(field, "http://") {
			urlCount++
		}
		if _, ok := parseWorkspaceLoginAuthorizationURL(field); !ok && !validWorkspaceClaudeLoginAuthorizationURL(field) {
			continue
		}
		if target != "" {
			return "", false
		}
		target = field
	}
	return target, target != "" && urlCount == 1
}

func validWorkspaceClaudeLoginAuthorizationURL(target string) bool {
	scopes, ok := claudeLoginAuthorizationScopes(target)
	if !ok {
		return false
	}
	want := strings.Fields(claudeWorkspaceLoginScope)
	slices.Sort(want)
	return slices.Equal(scopes, want)
}

func claudeWorkspaceHyperlinkTarget(line string) (string, bool) {
	const open = "\x1b]8;"
	const close = "\x1b]8;;\a"
	if !strings.HasPrefix(line, open) || !strings.HasSuffix(line, close) || strings.Count(line, open) != 2 {
		return "", false
	}
	rest := strings.TrimPrefix(line, open)
	openEnd := strings.IndexByte(rest, '\a')
	if openEnd <= 0 {
		return "", false
	}
	header := rest[:openEnd]
	separator := strings.IndexByte(header, ';')
	if separator < 0 {
		return "", false
	}
	parameters, target := header[:separator], header[separator+1:]
	label := strings.TrimSuffix(rest[openEnd+1:], close)
	if !validWorkspaceClaudeLoginAuthorizationURL(target) {
		return "", false
	}
	if parameters == "" {
		if label != target {
			return "", false
		}
		return target, true
	}
	if !validClaudeWorkspaceHyperlinkParameters(parameters) {
		return "", false
	}
	visibleLabel := strings.NewReplacer("\x1b[37m", "", "\x1b[39m", "").Replace(label)
	if visibleLabel == "" || strings.ContainsRune(visibleLabel, '\x1b') || !strings.Contains(target, visibleLabel) {
		return "", false
	}
	return target, true
}

func validClaudeWorkspaceHyperlinkParameters(value string) bool {
	const prefix = "id="
	if !strings.HasPrefix(value, prefix) || len(value) <= len(prefix) || len(value) > len(prefix)+32 {
		return false
	}
	for _, character := range strings.TrimPrefix(value, prefix) {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

type workspaceLoginObservingWriter struct {
	destination  io.Writer
	observer     *workspaceLoginOutputObserver
	mu           sync.Mutex
	line         []byte
	discardLine  bool
	frame        []byte
	inFrame      bool
	discardFrame bool
}

type codexLoginObservingWriter = workspaceLoginObservingWriter

func (w *workspaceLoginObservingWriter) Write(input []byte) (int, error) {
	written, err := w.destination.Write(input)
	if written <= 0 {
		return written, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.observeTerminalFrames(input[:written])
	for _, value := range input[:written] {
		if value == '\n' {
			if !w.discardLine {
				w.observer.observe(string(w.line))
			}
			clear(w.line)
			w.line = w.line[:0]
			w.discardLine = false
			continue
		}
		if w.discardLine {
			continue
		}
		if len(w.line) >= codexLoginLineLimit {
			clear(w.line)
			w.line = w.line[:0]
			w.discardLine = true
			continue
		}
		w.line = append(w.line, value)
	}
	w.observeNoNewlinePrompt()
	return written, err
}

func (w *workspaceLoginObservingWriter) observeNoNewlinePrompt() {
	if w.discardLine || len(w.line) == 0 {
		return
	}
	target, ok := githubBrowserURLFromNoNewlinePrompt(string(w.line))
	if ok {
		w.observer.observeTarget(target)
	}
}

func githubBrowserURLFromNoNewlinePrompt(line string) (string, bool) {
	const trailer = " in your browser... "
	if !strings.HasSuffix(line, trailer) {
		return "", false
	}
	return githubBrowserURLFromPromptText(strings.TrimSuffix(line, trailer))
}

func githubBrowserURLFromPromptText(prompt string) (string, bool) {
	var target string
	urlCount := 0
	for _, field := range strings.Fields(prompt) {
		if strings.HasPrefix(field, "https://") || strings.HasPrefix(field, "http://") {
			urlCount++
		}
		if field != githubDeviceURL && !validGitHubLoginAuthorizationURL(field) {
			continue
		}
		if target != "" {
			return "", false
		}
		target = field
	}
	return target, target != "" && urlCount == 1
}

func (w *workspaceLoginObservingWriter) observeTerminalFrames(input []byte) {
	w.frame = append(w.frame, input...)
	for {
		if !w.inFrame {
			start := bytes.Index(w.frame, codexSynchronizedFrameStart)
			if start < 0 {
				w.retainFrameMarkerTail(codexSynchronizedFrameStart)
				return
			}
			w.frame = w.frame[start+len(codexSynchronizedFrameStart):]
			w.inFrame = true
			w.discardFrame = false
		}

		end := bytes.Index(w.frame, codexSynchronizedFrameEnd)
		if end >= 0 {
			if !w.discardFrame && end <= codexLoginFrameLimit {
				w.observeSynchronizedFrame(w.frame[:end])
			}
			w.frame = w.frame[end+len(codexSynchronizedFrameEnd):]
			w.inFrame = false
			w.discardFrame = false
			continue
		}
		if len(w.frame) > codexLoginFrameLimit {
			w.discardFrame = true
			w.retainFrameMarkerTail(codexSynchronizedFrameEnd)
		}
		return
	}
}

func (w *workspaceLoginObservingWriter) retainFrameMarkerTail(marker []byte) {
	keep := len(marker) - 1
	if keep < 0 {
		keep = 0
	}
	if len(w.frame) <= keep {
		return
	}
	copy(w.frame, w.frame[len(w.frame)-keep:])
	clear(w.frame[keep:])
	w.frame = w.frame[:keep]
}

func (w *workspaceLoginObservingWriter) observeSynchronizedFrame(frame []byte) {
	visible, ok := codexVisibleTerminalFrame(frame)
	if !ok {
		return
	}
	target, ok := codexAuthorizationURLFromTerminalText(visible)
	if ok {
		w.observer.observeTarget(target)
	}
}

func codexAuthorizationURLFromTerminalText(visible string) (string, bool) {
	const httpsPrefix = "https://"
	const httpPrefix = "http://"
	start := -1
	urlCount := 0
	for index := 0; index < len(visible); {
		httpsIndex := strings.Index(visible[index:], httpsPrefix)
		httpIndex := strings.Index(visible[index:], httpPrefix)
		next := -1
		switch {
		case httpsIndex >= 0 && httpIndex >= 0:
			next = min(httpsIndex, httpIndex)
		case httpsIndex >= 0:
			next = httpsIndex
		case httpIndex >= 0:
			next = httpIndex
		}
		if next < 0 {
			break
		}
		absolute := index + next
		urlCount++
		if start < 0 {
			start = absolute
		}
		index = absolute + len(httpPrefix)
	}
	if urlCount != 1 || start < 0 {
		return "", false
	}
	end := start
	for end < len(visible) && visible[end] != ' ' && visible[end] != '\t' && visible[end] != '\r' && visible[end] != '\n' {
		end++
	}
	target := visible[start:end]
	return target, validCodexLoginAuthorizationURL(target)
}

// codexVisibleTerminalFrame removes terminal presentation controls without
// interpreting cursor positions or Codex prose. This recovers URLs split only
// by terminal repaint sequences while leaving all URL authority to the strict
// semantic validator.
func codexVisibleTerminalFrame(frame []byte) (string, bool) {
	visible := make([]byte, 0, len(frame))
	for index := 0; index < len(frame); {
		value := frame[index]
		switch {
		case value == 0x1b:
			if index+1 >= len(frame) {
				return "", false
			}
			switch frame[index+1] {
			case '[':
				index += 2
				start := index
				for index < len(frame) && (frame[index] < 0x40 || frame[index] > 0x7e) {
					index++
				}
				if index >= len(frame) || index-start > 256 {
					return "", false
				}
				index++
			case ']':
				index += 2
				start := index
				terminated := false
				for index < len(frame) && index-start <= 4<<10 {
					if frame[index] == 0x07 {
						index++
						terminated = true
						break
					}
					if frame[index] == 0x1b && index+1 < len(frame) && frame[index+1] == '\\' {
						index += 2
						terminated = true
						break
					}
					index++
				}
				if !terminated {
					return "", false
				}
			default:
				index += 2
			}
		case value == '\r' || value == '\n' || value == '\t':
			visible = append(visible, ' ')
			index++
		case value < 0x20 || value == 0x7f:
			index++
		default:
			visible = append(visible, value)
			index++
		}
	}
	if len(visible) > codexLoginFrameLimit {
		return "", false
	}
	return string(visible), true
}

func (b *workspaceLoginBridge) writers(out, errOut io.Writer) (io.Writer, io.Writer) {
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}
	observer := &workspaceLoginOutputObserver{trigger: b.trigger}
	return &workspaceLoginObservingWriter{destination: out, observer: observer},
		&workspaceLoginObservingWriter{destination: errOut, observer: observer}
}
