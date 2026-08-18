package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticAWSSSOWorkspaceAuthorizationURL(callbackPort int) string {
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {"synthetic-client-id_123"},
		"redirect_uri":          {fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", callbackPort)},
		"state":                 {"01234567-89ab-4def-8abc-0123456789ab"},
		"scopes":                {awsSSOWorkspaceScope},
		"code_challenge":        {strings.Repeat("c", 43)},
		"code_challenge_method": {"S256"},
	}
	return "https://oidc.ap-northeast-1.amazonaws.com/authorize?" + query.Encode()
}

func TestAWSSSOWorkspaceAuthorizationAllowsOnlyPinnedAuthorizationCodeSemantics(t *testing.T) {
	valid := syntheticAWSSSOWorkspaceAuthorizationURL(36901)
	authorization, ok := parseAWSSSOWorkspaceAuthorizationURL(valid)
	if !ok || authorization.callbackPort != 36901 || !validLoginBrowserTarget(valid) {
		t.Fatalf("reviewed AWS SSO target = (%+v, %t)", authorization, ok)
	}
	action, ok := parseWorkspaceLoginBrowserAction(valid)
	if !ok || !action.relayCallback || action.callbackPort != 36901 {
		t.Fatalf("AWS SSO browser action = (%+v, %t)", action, ok)
	}

	for name, target := range map[string]string{
		"scheme":          strings.Replace(valid, "https://", "http://", 1),
		"host":            strings.Replace(valid, "oidc.ap-northeast-1.amazonaws.com", "oidc.ap-northeast-1.amazonaws.com.evil.example", 1),
		"host case":       strings.Replace(valid, "oidc.ap-northeast-1.amazonaws.com", "OIDC.ap-northeast-1.amazonaws.com", 1),
		"partition":       strings.Replace(valid, "amazonaws.com", "amazonaws.com.cn", 1),
		"explicit port":   strings.Replace(valid, "amazonaws.com/", "amazonaws.com:443/", 1),
		"region":          strings.Replace(valid, "ap-northeast-1", "us-gov-west-1", 1),
		"path":            strings.Replace(valid, "/authorize?", "/token?", 1),
		"client":          strings.Replace(valid, "synthetic-client-id_123", "client%2Fother", 1),
		"state":           strings.Replace(valid, "01234567-89ab-4def-8abc-0123456789ab", "01234567-89AB-4DEF-8ABC-0123456789AB", 1),
		"challenge":       strings.Replace(valid, strings.Repeat("c", 43), strings.Repeat("c", 42), 1),
		"method":          strings.Replace(valid, "S256", "plain", 1),
		"scope":           strings.Replace(valid, url.QueryEscape(awsSSOWorkspaceScope), url.QueryEscape(awsSSOWorkspaceScope+" other"), 1),
		"callback host":   strings.Replace(valid, "127.0.0.1%3A36901", "localhost%3A36901", 1),
		"callback port":   strings.Replace(valid, "127.0.0.1%3A36901", "127.0.0.1%3A443", 1),
		"callback path":   strings.Replace(valid, "%2Foauth%2Fcallback", "%2Fother", 1),
		"extra query":     valid + "&audience=other",
		"duplicate query": valid + "&state=01234567-89ab-4def-8abc-0123456789ab",
		"fragment":        valid + "#fragment",
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := parseAWSSSOWorkspaceAuthorizationURL(target); ok {
				t.Fatalf("unsafe AWS SSO target accepted: %s", target)
			}
		})
	}
}

func TestAWSSSOWorkspaceLoginBridgeRelaysOnlyToSelectedWorkspace(t *testing.T) {
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
	target := syntheticAWSSSOWorkspaceAuthorizationURL(36901)
	if !bridge.trigger(target) {
		t.Fatal("valid native AWS SSO login did not start bridge")
	}
	if len(browser.targets) != 1 || browser.targets[0] != target || listenedAddress != "127.0.0.1:36901" {
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
	if len(runner.runs) != 1 || !containsArgSequence(runner.runs[0], container, "python3", "-c", workspaceLoopbackProxyProgram, "36901") {
		t.Fatalf("relay argv = %q", runner.runs)
	}
}
