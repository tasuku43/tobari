package dockerruntime

import (
	"context"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type configuratorBridgeRunner struct {
	component string
	identity  string
	mu        sync.Mutex
	runs      [][]string
}

type gatedConfiguratorListener struct {
	delegate net.Listener
	gate     <-chan struct{}
}

func (l *gatedConfiguratorListener) Accept() (net.Conn, error) {
	<-l.gate
	return l.delegate.Accept()
}
func (l *gatedConfiguratorListener) Close() error   { return l.delegate.Close() }
func (l *gatedConfiguratorListener) Addr() net.Addr { return l.delegate.Addr() }

func (r *configuratorBridgeRunner) Output(_ context.Context, args, _ []string) ([]byte, error) {
	identity := r.identity
	if identity == "" {
		identity = args[len(args)-1]
	}
	return []byte(`{"id":"` + identity + `","owner":"` + ownerValue + `","component":"` + r.component + `"}`), nil
}

func (r *configuratorBridgeRunner) Run(_ context.Context, args, _ []string, input io.Reader, output, _ io.Writer) error {
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

func TestConfiguratorCodexLoginBridgeOpensAndRelaysToExactOwnedContainer(t *testing.T) {
	runner := &configuratorBridgeRunner{component: "configurator"}
	browser := &recordingBrowser{}
	containerID := strings.Repeat("c", 64)
	server, client := net.Pipe()
	defer client.Close()
	listener := &singleConnectionListener{connection: server, closed: make(chan struct{})}
	bridge := newConfiguratorLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, containerID, tobari.ConfiguratorAgentCodex)
	defer bridge.close()
	var listenedAddress string
	bridge.listen = func(address string) (net.Listener, error) {
		listenedAddress = address
		return listener, nil
	}
	target := syntheticCodexAuthorizationURLWithOriginator(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890, codexOriginatorTUI)
	if !bridge.trigger(target) {
		t.Fatal("valid Configurator Codex login did not start bridge")
	}
	if !reflect.DeepEqual(browser.targets, []string{target}) || listenedAddress != "127.0.0.1:27890" {
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
	if len(runner.runs) != 1 || !containsArgSequence(runner.runs[0], containerID, "python3", "-c", workspaceLoopbackProxyProgram, "27890") {
		t.Fatalf("relay argv = %q", runner.runs)
	}
}

func TestConfiguratorLoginBridgeSelectsOnlyTheChosenAgentAndOwnedRole(t *testing.T) {
	codexTarget := syntheticCodexAuthorizationURL(strings.Repeat("c", 43), strings.Repeat("s", 43))
	claudeTarget := syntheticClaudeWorkspaceAuthorizationURL()
	containerID := strings.Repeat("c", 64)
	for _, test := range []struct {
		name      string
		agent     tobari.ConfiguratorAgent
		component string
		identity  string
		target    string
	}{
		{name: "Codex rejects Claude", agent: tobari.ConfiguratorAgentCodex, component: "configurator", target: claudeTarget},
		{name: "Claude rejects Codex", agent: tobari.ConfiguratorAgentClaude, component: "configurator", target: codexTarget},
		{name: "wrong role rejects Codex", agent: tobari.ConfiguratorAgentCodex, component: "workspace", target: codexTarget},
		{name: "identity drift rejects Codex", agent: tobari.ConfiguratorAgentCodex, component: "configurator", identity: strings.Repeat("d", 64), target: codexTarget},
	} {
		t.Run(test.name, func(t *testing.T) {
			browser := &recordingBrowser{}
			bridge := newConfiguratorLoginBridge(context.Background(), &Runtime{runner: &configuratorBridgeRunner{component: test.component, identity: test.identity}, browser: browser}, containerID, test.agent)
			defer bridge.close()
			if bridge.trigger(test.target) {
				t.Fatal("unselected or unowned browser target was accepted")
			}
			if len(browser.targets) != 0 {
				t.Fatalf("rejected target opened browser: %q", browser.targets)
			}
		})
	}
}

func TestConfiguratorCodexCallbackRevalidatesImmutableContainerIdentity(t *testing.T) {
	containerID := strings.Repeat("c", 64)
	runner := &configuratorBridgeRunner{component: "configurator"}
	browser := &recordingBrowser{}
	server, client := net.Pipe()
	defer client.Close()
	listener := &singleConnectionListener{connection: server, closed: make(chan struct{})}
	accept := make(chan struct{})
	gated := &gatedConfiguratorListener{delegate: listener, gate: accept}
	bridge := newConfiguratorLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, containerID, tobari.ConfiguratorAgentCodex)
	defer bridge.close()
	bridge.listen = func(string) (net.Listener, error) { return gated, nil }
	target := syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890)
	if !bridge.trigger(target) {
		t.Fatal("valid Configurator Codex login did not open")
	}
	runner.identity = strings.Repeat("d", 64)
	close(accept)
	_, _ = io.WriteString(client, "ping")
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	response := make([]byte, 1)
	if count, err := client.Read(response); err == nil || count != 0 {
		t.Fatalf("identity-swapped callback reached relay: count=%d err=%v", count, err)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if len(runner.runs) != 0 {
		t.Fatalf("identity-swapped callback executed Docker relay: %q", runner.runs)
	}
}

func TestConfiguratorCodexRevalidatesIdentityImmediatelyBeforeBrowserOpen(t *testing.T) {
	containerID := strings.Repeat("c", 64)
	runner := &configuratorBridgeRunner{component: "configurator"}
	browser := &recordingBrowser{}
	bridge := newConfiguratorLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, containerID, tobari.ConfiguratorAgentCodex)
	defer bridge.close()
	server, client := net.Pipe()
	defer client.Close()
	listener := &singleConnectionListener{connection: server, closed: make(chan struct{})}
	bridge.listen = func(string) (net.Listener, error) {
		runner.identity = strings.Repeat("d", 64)
		return listener, nil
	}
	target := syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890)
	if bridge.trigger(target) {
		t.Fatal("identity drift between listener creation and browser open was accepted")
	}
	if len(browser.targets) != 0 {
		t.Fatalf("identity drift opened browser: %q", browser.targets)
	}
	select {
	case <-listener.closed:
	case <-time.After(time.Second):
		t.Fatal("identity-drift listener was not closed")
	}
}

func TestConfiguratorClaudeLoginBridgeOpensWithoutCallbackListener(t *testing.T) {
	runner := &configuratorBridgeRunner{component: "configurator"}
	browser := &recordingBrowser{}
	containerID := strings.Repeat("c", 64)
	bridge := newConfiguratorLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, containerID, tobari.ConfiguratorAgentClaude)
	defer bridge.close()
	listenCalls := 0
	bridge.listen = func(string) (net.Listener, error) {
		listenCalls++
		return nil, nil
	}
	target := syntheticClaudeWorkspaceAuthorizationURL()
	if !bridge.trigger(target) {
		t.Fatal("valid Configurator Claude login did not open")
	}
	if !reflect.DeepEqual(browser.targets, []string{target}) || listenCalls != 0 {
		t.Fatalf("browser targets/listener calls = %q/%d", browser.targets, listenCalls)
	}
}
