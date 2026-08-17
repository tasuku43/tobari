package dockerruntime

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticGitHubAuthorizationURL(callbackPort int, state, scope string) string {
	return fmt.Sprintf(
		"https://%s%s?client_id=%s&redirect_uri=http%%3A%%2F%%2F127.0.0.1%%3A%d%%2Fcallback&scope=%s&state=%s",
		githubAuthorizationHost, githubAuthorizationPath, githubAuthorizationClientID,
		callbackPort, url.QueryEscape(scope), state,
	)
}

func TestParseGitHubLoginAuthorizationURLAllowsOnlyPinnedHTTPSLoginSemantics(t *testing.T) {
	valid := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	encodedScopes := url.QueryEscape(githubAuthorizationScope)
	for _, target := range []string{
		valid,
		strings.Replace(valid, encodedScopes, url.QueryEscape("workflow gist repo read:org"), 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo read:org gist"), 1),
	} {
		authorization, ok := parseGitHubLoginAuthorizationURL(target)
		if !ok || authorization.callbackPort != 37405 || !validLoginBrowserTarget(target) {
			t.Fatalf("reviewed GitHub target rejected: port=%d ok=%t target=%q", authorization.callbackPort, ok, target)
		}
	}
	for _, target := range []string{
		strings.Replace(valid, githubAuthorizationHost, "github.com.evil.example", 1),
		strings.Replace(valid, githubAuthorizationPath, "/login/device", 1),
		strings.Replace(valid, githubAuthorizationClientID, "other-client", 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo read:org gist admin:org"), 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo gist workflow"), 1),
		strings.Replace(valid, "127.0.0.1%3A37405", "localhost%3A37405", 1),
		strings.Replace(valid, "127.0.0.1%3A37405", "127.0.0.1%3A443", 1),
		strings.Replace(valid, "%2Fcallback", "%2Fother", 1),
		valid + "&allow_signup=true",
		valid + "#fragment",
	} {
		if _, ok := parseGitHubLoginAuthorizationURL(target); ok {
			t.Fatalf("unsafe GitHub target accepted: %q", target)
		}
	}
}

func TestGitHubDeviceLoginBridgeFailsClosedBeforeHostOpenOrListener(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		runner    *codexBridgeRunner
		browser   *recordingBrowser
		wantOpens int
	}{
		{name: "ownership mismatch", runner: &codexBridgeRunner{projectID: "018bcfe5-687b-7000-8000-000000000099"}, browser: &recordingBrowser{}},
		{name: "browser failure", runner: &codexBridgeRunner{projectID: projectID}, browser: &recordingBrowser{err: fmt.Errorf("opener unavailable")}, wantOpens: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: test.runner, browser: test.browser}, container, projectID)
			defer bridge.close()
			listenCalls := 0
			bridge.listen = func(string) (net.Listener, error) {
				listenCalls++
				return nil, fmt.Errorf("device flow must not bind a callback listener")
			}
			if bridge.trigger(githubDeviceURL) {
				t.Fatal("failed device host open reported success")
			}
			if len(test.browser.targets) != test.wantOpens || listenCalls != 0 {
				t.Fatalf("browser targets/listener calls = %q/%d", test.browser.targets, listenCalls)
			}
		})
	}
}

func TestWorkspaceLoginDriverRegistryKeepsCallbackProviderContractsDisjoint(t *testing.T) {
	for _, test := range []struct {
		id     string
		target string
		port   int
	}{
		{id: "github-oauth", target: syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope), port: 37405},
		{id: "codex", target: syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890), port: 27890},
		{id: "pup", target: syntheticPupWorkspaceAuthorizationURL(8000, "dashboards_read metrics_read"), port: 8000},
	} {
		driver, action, ok := selectWorkspaceLoginDriver(test.target, reviewedWorkspaceLoginDrivers())
		if !ok || driver.id != test.id || !action.relayCallback || action.callbackPort != test.port {
			t.Fatalf("workspace driver/action = (%q, %+v, %t), want %q callback port %d", driver.id, action, ok, test.id, test.port)
		}
	}
}
