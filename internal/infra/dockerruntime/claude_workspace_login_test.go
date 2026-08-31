package dockerruntime

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticClaudeWorkspaceAuthorizationURL() string {
	query := url.Values{
		"code": {"true"}, "client_id": {claudeLoginClientID}, "response_type": {"code"},
		"redirect_uri": {claudeLoginRedirectURI}, "scope": {claudeWorkspaceLoginScope},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"},
		"state": {strings.Repeat("b", 43)},
	}
	return claudeLoginURLPrefix + query.Encode()
}

func TestClaudeWorkspaceLoginBridgeOpensHostWithoutCallbackListener(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge, err := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	if err != nil {
		t.Fatal(err)
	}
	defer bridge.close()
	listenCalls := 0
	bridge.listen = func(string) (net.Listener, error) {
		listenCalls++
		return nil, fmt.Errorf("Claude remote callback must not bind a host listener")
	}
	target := syntheticClaudeWorkspaceAuthorizationURL()
	if !bridge.trigger(target) {
		t.Fatal("reviewed Claude URL did not open")
	}
	if !reflect.DeepEqual(browser.targets, []string{target}) || listenCalls != 0 {
		t.Fatalf("browser targets/listener calls = %q/%d", browser.targets, listenCalls)
	}
}

func TestClaudeWorkspaceLoginRejectsChangedAuthorizationSemantics(t *testing.T) {
	valid := syntheticClaudeWorkspaceAuthorizationURL()
	for _, target := range []string{
		strings.Replace(valid, "claude.com", "claude.example", 1),
		strings.Replace(valid, url.QueryEscape(claudeLoginClientID), "other-client", 1),
		strings.Replace(valid, url.QueryEscape(claudeWorkspaceLoginScope), url.QueryEscape(claudeWorkspaceLoginScope+" user:admin"), 1),
		strings.Replace(valid, url.QueryEscape(claudeWorkspaceLoginScope), url.QueryEscape("user:profile user:inference"), 1),
		valid + "&extra=true",
	} {
		if validWorkspaceClaudeLoginAuthorizationURL(target) {
			t.Fatalf("unsafe Claude target accepted: %q", target)
		}
	}
}
