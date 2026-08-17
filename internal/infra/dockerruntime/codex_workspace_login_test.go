package dockerruntime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticCodexAuthorizationURL(challenge, state string) string {
	return syntheticCodexAuthorizationURLWithPort(challenge, state, 1455)
}

func syntheticCodexAuthorizationURLWithPort(challenge, state string, callbackPort int) string {
	return syntheticCodexAuthorizationURLWithOriginator(challenge, state, callbackPort, codexOriginatorCLI)
}

func syntheticCodexAuthorizationURLWithOriginator(challenge, state string, callbackPort int, originator string) string {
	return fmt.Sprintf("https://%s%s?response_type=code&client_id=%s&redirect_uri=http%%3A%%2F%%2Flocalhost%%3A%d%%2Fauth%%2Fcallback&scope=openid%%20profile%%20email%%20offline_access%%20api.connectors.read%%20api.connectors.invoke&code_challenge=%s&code_challenge_method=S256&id_token_add_organizations=true&codex_cli_simplified_flow=true&state=%s&originator=%s", codexAuthorizationHost, codexAuthorizationPath, codexAuthorizationClientID, callbackPort, challenge, state, originator)
}

func TestParseCodexLoginAuthorizationURLAllowsOnlyReviewedSemantics(t *testing.T) {
	valid := syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 64), 27890)
	reorderedScopes := strings.Replace(valid,
		"openid%20profile%20email%20offline_access%20api.connectors.read%20api.connectors.invoke",
		"api.connectors.invoke%20offline_access%20email%20openid%20api.connectors.read%20profile", 1)
	reducedScopes := strings.Replace(valid,
		"openid%20profile%20email%20offline_access%20api.connectors.read%20api.connectors.invoke",
		"profile%20openid", 1)
	withoutOptionalFlags := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(valid,
		"&id_token_add_organizations=true", ""), "&codex_cli_simplified_flow=true", ""), "&originator=codex_cli_rs", "")
	tui := strings.Replace(valid, "originator="+codexOriginatorCLI, "originator="+codexOriginatorTUI, 1)
	for _, target := range []string{valid, tui, reorderedScopes, reducedScopes, withoutOptionalFlags, strings.Replace(valid, "localhost%3A27890", "127.0.0.1%3A27890", 1)} {
		authorization, ok := parseCodexLoginAuthorizationURL(target)
		if !ok || authorization.callbackPort != 27890 {
			t.Fatalf("reviewed target rejected: port=%d ok=%t", authorization.callbackPort, ok)
		}
	}
	for _, target := range []string{
		strings.Replace(valid, "localhost%3A27890", "localhost%3A443", 1),
		strings.Replace(valid, "localhost%3A27890", "localhost", 1),
		strings.Replace(valid, "localhost%3A27890", "example.com%3A27890", 1),
		strings.Replace(valid, codexAuthorizationClientID, "other-client", 1),
		strings.Replace(valid, "api.connectors.invoke", "admin", 1),
		strings.Replace(valid, "S256", "plain", 1),
		strings.Replace(valid, "originator=codex_cli_rs", "originator=other", 1),
		valid + "&audience=other",
		strings.Replace(valid, "%2Fauth%2Fcallback", "%2Fother%2Fcallback", 1),
	} {
		if _, ok := parseCodexLoginAuthorizationURL(target); ok {
			t.Fatalf("unsafe target accepted: %q", target)
		}
	}
}

type codexBridgeRunner struct {
	projectID string
	mu        sync.Mutex
	runs      [][]string
}

func (r *codexBridgeRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, ownerLabel):
		return []byte(ownerValue), nil
	case strings.Contains(joined, projectIDLabel):
		return []byte(r.projectID), nil
	case strings.Contains(joined, projectRoleLabel):
		return []byte(projectWorkRole), nil
	default:
		return nil, nil
	}
}

func (r *codexBridgeRunner) Run(_ context.Context, args, _ []string, input io.Reader, output, _ io.Writer) error {
	r.mu.Lock()
	r.runs = append(r.runs, append([]string{}, args...))
	r.mu.Unlock()
	request := make([]byte, 4)
	if _, err := io.ReadFull(input, request); err != nil {
		return err
	}
	_, err := io.WriteString(output, "pong")
	return err
}

type singleConnectionListener struct {
	connection net.Conn
	closed     chan struct{}
	once       sync.Once
}

func (l *singleConnectionListener) Accept() (net.Conn, error) { return l.connection, nil }
func (l *singleConnectionListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}
func (l *singleConnectionListener) Addr() net.Addr { return &net.TCPAddr{} }

func TestCodexWorkspaceLoginBridgeOpensAndRelaysOnlyToSelectedWorkspace(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	server, client := net.Pipe()
	defer client.Close()
	listener := &singleConnectionListener{connection: server, closed: make(chan struct{})}
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	var listenedAddress string
	bridge.listen = func(address string) (net.Listener, error) { listenedAddress = address; return listener, nil }
	target := syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890)
	if !bridge.trigger(target) {
		t.Fatal("valid native Codex login did not start bridge")
	}
	if len(browser.targets) != 1 || browser.targets[0] != target || listenedAddress != "127.0.0.1:27890" {
		t.Fatalf("browser targets/address = %q/%q", browser.targets, listenedAddress)
	}
	_, _ = io.WriteString(client, "ping")
	response := make([]byte, 4)
	if _, err := io.ReadFull(client, response); err != nil || string(response) != "pong" {
		t.Fatalf("callback response = %q, %v", response, err)
	}
	select {
	case <-listener.closed:
	case <-time.After(time.Second):
		t.Fatal("callback listener was not closed")
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.runs) != 1 || !containsArgSequence(runner.runs[0], container, "python3", "-c", workspaceLoopbackProxyProgram, "27890") {
		t.Fatalf("relay argv = %q", runner.runs)
	}
}

func TestCodexWorkspaceLoginBridgeFailsClosedBeforeBrowserOpen(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	target := syntheticCodexAuthorizationURL(strings.Repeat("c", 43), strings.Repeat("s", 43))
	for _, test := range []struct {
		name    string
		browser *recordingBrowser
		listen  workspaceCallbackListener
	}{
		{name: "port collision", browser: &recordingBrowser{}, listen: func(string) (net.Listener, error) { return nil, errors.New("address already in use") }},
		{name: "browser failure", browser: &recordingBrowser{err: errors.New("opener unavailable")}, listen: func(string) (net.Listener, error) { return net.Listen("tcp4", "127.0.0.1:0") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: &codexBridgeRunner{projectID: projectID}, browser: test.browser}, container, projectID)
			bridge.listen = test.listen
			if bridge.trigger(target) {
				t.Fatal("failed bridge reported success")
			}
			bridge.close()
			if test.name == "port collision" && len(test.browser.targets) != 0 {
				t.Fatalf("port collision opened browser: %q", test.browser.targets)
			}
		})
	}
}
