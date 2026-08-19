package dockerruntime

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticPupWorkspaceAuthorizationURL(callbackPort int, scopes string) string {
	return syntheticPupWorkspaceAuthorizationURLWithOrg(callbackPort, scopes, "")
}

func syntheticPupWorkspaceAuthorizationURLWithOrg(callbackPort int, scopes, orgUUID string) string {
	target := fmt.Sprintf(
		"https://%s%s?response_type=code&client_id=client-example-123&redirect_uri=%s&state=%s&scope=%s&code_challenge=%s&code_challenge_method=S256",
		pupWorkspaceAuthorizationHost,
		pupWorkspaceAuthorizationPath,
		url.QueryEscape(fmt.Sprintf("http://127.0.0.1:%d/oauth/callback", callbackPort)),
		strings.Repeat("s", 32),
		url.QueryEscape(scopes),
		strings.Repeat("c", 43),
	)
	if orgUUID != "" {
		target += "&dd_oid=" + url.QueryEscape(orgUUID)
	}
	return target
}

func TestPupWorkspaceLoginAuthorizationAcceptsReviewedRememberedOrgHint(t *testing.T) {
	for _, orgUUID := range []string{
		"11111111-2222-3333-4444-555555555555",
		"AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE",
	} {
		target := syntheticPupWorkspaceAuthorizationURLWithOrg(8000, "dashboards_read metrics_read", orgUUID)
		authorization, ok := parsePupWorkspaceLoginAuthorizationURL(target)
		if !ok || authorization.callbackPort != 8000 || !validLoginBrowserTarget(target) {
			t.Fatalf("reviewed pup org target = (%+v, %t), want callback port 8000", authorization, ok)
		}
	}
}

func TestPupWorkspaceLoginAuthorizationRejectsUnreviewedOrgHints(t *testing.T) {
	valid := syntheticPupWorkspaceAuthorizationURL(8000, "dashboards_read metrics_read")
	validWithOrg := syntheticPupWorkspaceAuthorizationURLWithOrg(
		8000, "dashboards_read metrics_read", "11111111-2222-3333-4444-555555555555",
	)
	for name, target := range map[string]string{
		"empty":            valid + "&dd_oid=",
		"short":            valid + "&dd_oid=11111111-2222-3333-4444-55555555555",
		"non-hex":          valid + "&dd_oid=zzzzzzzz-2222-3333-4444-555555555555",
		"missing hyphens":  valid + "&dd_oid=11111111222233334444555555555555",
		"duplicate":        validWithOrg + "&dd_oid=aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"unknown neighbor": validWithOrg + "&prompt=consent",
		"missing required": strings.Replace(validWithOrg, "response_type=code&", "", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := parsePupWorkspaceLoginAuthorizationURL(target); ok {
				t.Fatalf("unreviewed pup org target accepted: %s", target)
			}
		})
	}
}

func TestPupWorkspaceLoginAuthorizationAllowsOnlyPinnedUS1Semantics(t *testing.T) {
	for _, port := range []int{8000, 8080, 8888, 9000} {
		target := syntheticPupWorkspaceAuthorizationURL(port, "dashboards_read metrics_read")
		authorization, ok := parsePupWorkspaceLoginAuthorizationURL(target)
		if !ok || authorization.callbackPort != port || !validLoginBrowserTarget(target) {
			t.Fatalf("reviewed pup target = (%+v, %t), want port %d", authorization, ok, port)
		}
		action, ok := parseWorkspaceLoginBrowserAction(target)
		if !ok || !action.relayCallback || action.callbackPort != port {
			t.Fatalf("pup browser action = (%+v, %t), want callback relay %d", action, ok, port)
		}
	}

	valid := syntheticPupWorkspaceAuthorizationURL(8000, "dashboards_read metrics_read")
	for name, target := range map[string]string{
		"scheme":          strings.Replace(valid, "https://", "http://", 1),
		"host":            strings.Replace(valid, pupWorkspaceAuthorizationHost, "api.datadoghq.com", 1),
		"host case":       strings.Replace(valid, pupWorkspaceAuthorizationHost, "APP.DATADOGHQ.COM", 1),
		"explicit port":   strings.Replace(valid, pupWorkspaceAuthorizationHost, pupWorkspaceAuthorizationHost+":443", 1),
		"path":            strings.Replace(valid, pupWorkspaceAuthorizationPath, "/oauth2/authorize", 1),
		"client":          strings.Replace(valid, "client-example-123", "short", 1),
		"state":           strings.Replace(valid, strings.Repeat("s", 32), strings.Repeat("s", 31), 1),
		"challenge":       strings.Replace(valid, strings.Repeat("c", 43), strings.Repeat("c", 42), 1),
		"method":          strings.Replace(valid, "S256", "plain", 1),
		"callback host":   strings.Replace(valid, "127.0.0.1%3A8000", "localhost%3A8000", 1),
		"callback port":   strings.Replace(valid, "127.0.0.1%3A8000", "127.0.0.1%3A8001", 1),
		"callback path":   strings.Replace(valid, "%2Foauth%2Fcallback", "%2Fother", 1),
		"scope widening":  strings.Replace(valid, "dashboards_read+metrics_read", "dashboards_read+user_access_manage", 1),
		"scope ordering":  strings.Replace(valid, "dashboards_read+metrics_read", "metrics_read+dashboards_read", 1),
		"duplicate scope": strings.Replace(valid, "dashboards_read+metrics_read", "dashboards_read+dashboards_read", 1),
		"extra query":     valid + "&audience=other",
		"fragment":        valid + "#fragment",
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := parsePupWorkspaceLoginAuthorizationURL(target); ok {
				t.Fatalf("unsafe pup target accepted: %s", target)
			}
		})
	}
}

func TestPupWorkspaceScopeCeilingContainsReviewedClientModes(t *testing.T) {
	allScopes := strings.Fields(pupWorkspaceAuthorizationScopeCeiling)
	if len(allScopes) != 110 {
		t.Fatalf("pup 1.10.7 scope ceiling count = %d, want 110", len(allScopes))
	}
	seen := make(map[string]struct{}, len(allScopes))
	for _, scope := range allScopes {
		if _, duplicate := seen[scope]; duplicate {
			t.Fatalf("duplicate pup 1.10.7 scope: %q", scope)
		}
		seen[scope] = struct{}{}
	}
	slices.Sort(allScopes)
	if !validPupWorkspaceScopeSubset(strings.Join(allScopes, " ")) {
		t.Fatal("complete reviewed pup 1.10.7 scope ceiling was rejected")
	}

	for _, scopes := range []string{
		"dashboards_read",
		"dashboards_read metrics_read",
		"apps_run built_in_features user_access_read workflows_read",
	} {
		if !validPupWorkspaceScopeSubset(scopes) {
			t.Fatalf("reviewed pup scope subset rejected: %q", scopes)
		}
	}
	for _, scopes := range []string{"", "user_access_manage", "dashboards_read future_scope"} {
		if validPupWorkspaceScopeSubset(scopes) {
			t.Fatalf("unreviewed pup scope subset accepted: %q", scopes)
		}
	}
}

func TestPupWorkspaceLoginBridgeRelaysOnlyToSelectedWorkspace(t *testing.T) {
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
	target := syntheticPupWorkspaceAuthorizationURLWithOrg(
		8000, "dashboards_read metrics_read", "11111111-2222-3333-4444-555555555555",
	)
	if !bridge.trigger(target) {
		t.Fatal("valid native pup login did not start bridge")
	}
	if len(browser.targets) != 1 || browser.targets[0] != target || listenedAddress != "127.0.0.1:8000" {
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
	if len(runner.runs) != 1 || !containsArgSequence(runner.runs[0], container, "python3", "-c", workspaceLoopbackProxyProgram, "8000") {
		t.Fatalf("relay argv = %q", runner.runs)
	}
}
