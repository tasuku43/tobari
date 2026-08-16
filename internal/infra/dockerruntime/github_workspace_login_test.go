package dockerruntime

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

func syntheticGitHubAuthorizationURL(callbackPort int, state, scope string) string {
	return fmt.Sprintf(
		"https://%s%s?client_id=%s&redirect_uri=http%%3A%%2F%%2F127.0.0.1%%3A%d%%2Fcallback&scope=%s&state=%s",
		githubAuthorizationHost,
		githubAuthorizationPath,
		githubAuthorizationClientID,
		callbackPort,
		url.QueryEscape(scope),
		state,
	)
}

func TestParseGitHubLoginAuthorizationURLAllowsOnlyPinnedHTTPSLoginSemantics(t *testing.T) {
	valid := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	encodedScopes := url.QueryEscape(githubAuthorizationScope)
	reorderedScopes := strings.Replace(valid, encodedScopes, url.QueryEscape("workflow gist repo read:org"), 1)
	withoutWorkflow := strings.Replace(valid, encodedScopes, url.QueryEscape("repo read:org gist"), 1)
	for _, target := range []string{valid, reorderedScopes, withoutWorkflow} {
		authorization, ok := parseGitHubLoginAuthorizationURL(target)
		if !ok || authorization.callbackPort != 37405 {
			t.Fatalf("reviewed GitHub target rejected: port=%d ok=%t target=%q", authorization.callbackPort, ok, target)
		}
		if !validLoginBrowserTarget(target) {
			t.Fatalf("reviewed GitHub target missing from browser allowlist: %q", target)
		}
	}

	for _, target := range []string{
		strings.Replace(valid, githubAuthorizationHost, "github.com.evil.example", 1),
		strings.Replace(valid, githubAuthorizationPath, "/login/device", 1),
		strings.Replace(valid, githubAuthorizationClientID, "other-client", 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo read:org gist admin:org"), 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo gist workflow"), 1),
		strings.Replace(valid, encodedScopes, url.QueryEscape("repo read:org gist gist"), 1),
		strings.Replace(valid, strings.Repeat("a", 20), strings.Repeat("a", 19), 1),
		strings.Replace(valid, strings.Repeat("a", 20), strings.Repeat("A", 20), 1),
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

func TestGitHubLoginObserverRecognizesFragmentedNoNewlinePromptWithoutChangingOutput(t *testing.T) {
	target := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	input := "\x1b[1mPress Enter\x1b[0m to open " + target + " in your browser... "
	var destination bytes.Buffer
	var opened []string
	observer := &workspaceLoginOutputObserver{trigger: func(candidate string) bool {
		opened = append(opened, candidate)
		return true
	}}
	writer := &workspaceLoginObservingWriter{destination: &destination, observer: observer}
	for offset := 0; offset < len(input); {
		end := offset + 3
		if end > len(input) {
			end = len(input)
		}
		if _, err := io.WriteString(writer, input[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if destination.String() != input {
		t.Fatalf("GitHub prompt changed\n got: %q\nwant: %q", destination.String(), input)
	}
	if len(opened) != 1 || opened[0] != target {
		t.Fatalf("browser triggers = %q", opened)
	}
	if _, err := io.WriteString(writer, "\n"+input); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 1 {
		t.Fatalf("replayed GitHub prompt reopened browser: %q", opened)
	}
}

func TestGitHubDeviceLoginPromptOpensExactHostTargetWithoutCallbackListener(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	defer bridge.close()
	listenCalls := 0
	bridge.listen = func(string) (net.Listener, error) {
		listenCalls++
		return nil, fmt.Errorf("device flow must not bind a callback listener")
	}

	prompt := "Press Enter to open " + githubDeviceURL + " in your browser... "
	var visible bytes.Buffer
	_, errOut := bridge.writers(io.Discard, &visible)
	for offset := 0; offset < len(prompt); {
		end := offset + 2
		if end > len(prompt) {
			end = len(prompt)
		}
		if _, err := io.WriteString(errOut, prompt[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if visible.String() != prompt {
		t.Fatalf("GitHub device prompt changed: got %q want %q", visible.String(), prompt)
	}
	if !reflect.DeepEqual(browser.targets, []string{githubDeviceURL}) {
		t.Fatalf("browser targets = %q", browser.targets)
	}
	if listenCalls != 0 {
		t.Fatalf("device flow callback listener calls = %d", listenCalls)
	}
}

func TestGitHubDeviceLoginNonInteractiveLineOpensExactHostTargetWithoutCallbackListener(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	defer bridge.close()
	listenCalls := 0
	bridge.listen = func(string) (net.Listener, error) {
		listenCalls++
		return nil, fmt.Errorf("device flow must not bind a callback listener")
	}

	line := "Open this URL to continue in your web browser: " + githubDeviceURL + "\n"
	var visible bytes.Buffer
	_, errOut := bridge.writers(io.Discard, &visible)
	for offset := 0; offset < len(line); {
		end := min(offset+3, len(line))
		if _, err := io.WriteString(errOut, line[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if visible.String() != line {
		t.Fatalf("GitHub non-interactive line changed: got %q want %q", visible.String(), line)
	}
	if !reflect.DeepEqual(browser.targets, []string{githubDeviceURL}) || listenCalls != 0 {
		t.Fatalf("browser targets/listener calls = %q/%d", browser.targets, listenCalls)
	}
}

func TestGitHubDeviceLoginStyledNonInteractiveLineOpensExactHostTargetWithoutChangingOutput(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	defer bridge.close()
	listenCalls := 0
	bridge.listen = func(string) (net.Listener, error) {
		listenCalls++
		return nil, fmt.Errorf("device flow must not bind a callback listener")
	}

	line := "\x1b[0;1;39mOpen this URL\x1b[0m to continue in your web browser: " + githubDeviceURL + "\n"
	var visible bytes.Buffer
	_, errOut := bridge.writers(io.Discard, &visible)
	for offset := 0; offset < len(line); {
		end := min(offset+3, len(line))
		if _, err := io.WriteString(errOut, line[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if visible.String() != line {
		t.Fatalf("styled GitHub non-interactive line changed: got %q want %q", visible.String(), line)
	}
	if !reflect.DeepEqual(browser.targets, []string{githubDeviceURL}) || listenCalls != 0 {
		t.Fatalf("browser targets/listener calls = %q/%d", browser.targets, listenCalls)
	}
}

func TestGitHubDeviceLoginNonInteractiveLineRejectsNeighboringTextAndTargets(t *testing.T) {
	for _, line := range []string{
		"Open this URL to continue in your web browser: https://github.com/login/device/\n",
		"Open this URL to continue in your web browser: https://github.com/login/device?next=example\n",
		"Open this URL to continue in your web browser: https://github.example/login/device\n",
		"Open this URL now: " + githubDeviceURL + "\n",
		"prefix Open this URL to continue in your web browser: " + githubDeviceURL + "\n",
		"Open this URL to continue in your web browser: " + githubDeviceURL + " trailing\n",
		"\x1b[1mOpen this URL\x1b[0m to continue in your web browser: " + githubDeviceURL + "\n",
		"\x1b[0;1;39mOpen this URL to continue in your web browser: \x1b[0m" + githubDeviceURL + "\n",
		"\x1b[0;1;39mOpen this URL\x1b[0m to continue in your web browser: " + githubDeviceURL + "\x1b[0m\n",
	} {
		var opened []string
		observer := &workspaceLoginOutputObserver{trigger: func(target string) bool {
			opened = append(opened, target)
			return true
		}}
		writer := &workspaceLoginObservingWriter{destination: io.Discard, observer: observer}
		_, _ = io.WriteString(writer, line)
		if len(opened) != 0 {
			t.Fatalf("hostile GitHub device line opened browser: %q -> %q", line, opened)
		}
	}
}

func TestGitHubDeviceLoginObserverRejectsNeighboringAndAmbiguousTargets(t *testing.T) {
	for _, prompt := range []string{
		"Press Enter to open https://github.com/login/device/ in your browser... ",
		"Press Enter to open https://github.com/login/device?next=example in your browser... ",
		"Press Enter to open https://github.example/login/device in your browser... ",
		"Press Enter to open " + githubDeviceURL + " now",
		"Press Enter to open " + githubDeviceURL + " " + githubDeviceURL + " in your browser... ",
	} {
		var opened []string
		observer := &workspaceLoginOutputObserver{trigger: func(target string) bool {
			opened = append(opened, target)
			return true
		}}
		writer := &workspaceLoginObservingWriter{destination: io.Discard, observer: observer}
		_, _ = io.WriteString(writer, prompt)
		if len(opened) != 0 {
			t.Fatalf("hostile GitHub device prompt opened browser: %q -> %q", prompt, opened)
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

func TestGitHubLoginObserverRejectsIncompleteAmbiguousAndOversizedNoNewlinePrompts(t *testing.T) {
	valid := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	second := syntheticGitHubAuthorizationURL(37406, strings.Repeat("b", 20), githubAuthorizationScope)
	for _, input := range []string{
		"Press Enter to open " + valid,
		"Press Enter to open " + valid + " in your browser... " + second + " in your browser... ",
		"Press Enter to open " + valid + " " + second + " in your browser... ",
		"Press Enter to open " + strings.Replace(valid, githubAuthorizationClientID, "other-client", 1) + " in your browser... ",
		strings.Repeat("x", workspaceLoginLineLimit+1) + valid + " in your browser... ",
	} {
		var opened int
		observer := &workspaceLoginOutputObserver{trigger: func(string) bool { opened++; return true }}
		writer := &workspaceLoginObservingWriter{destination: io.Discard, observer: observer}
		_, _ = io.WriteString(writer, input)
		if opened != 0 {
			t.Fatalf("hostile no-newline prompt opened browser: %q", input)
		}
	}
}

func TestGitHubNoNewlinePromptStartsSelectedWorkspaceBrowserAndCallbackBridge(t *testing.T) {
	projectID := "018bcfe5-687b-7000-8000-000000000001"
	container, _, err := tobari.ProjectResourceNames(projectID)
	if err != nil {
		t.Fatal(err)
	}
	runner := &codexBridgeRunner{projectID: projectID}
	browser := &recordingBrowser{}
	bridge := newWorkspaceLoginBridge(context.Background(), &Runtime{runner: runner, browser: browser}, container, projectID)
	defer bridge.close()
	var listenedAddress string
	bridge.listen = func(address string) (net.Listener, error) {
		listenedAddress = address
		return net.Listen("tcp4", "127.0.0.1:0")
	}

	target := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	_, errOut := bridge.writers(io.Discard, io.Discard)
	prompt := "\x1b[1mPress Enter\x1b[0m to open " + target + " in your browser... "
	for offset := 0; offset < len(prompt); {
		end := offset + 5
		if end > len(prompt) {
			end = len(prompt)
		}
		if _, err := io.WriteString(errOut, prompt[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if listenedAddress != "127.0.0.1:37405" {
		t.Fatalf("callback address = %q", listenedAddress)
	}
	if len(browser.targets) != 1 || browser.targets[0] != target {
		t.Fatalf("browser targets = %q", browser.targets)
	}
}

func TestWorkspaceLoginAuthorizationParserKeepsProviderContractsDisjoint(t *testing.T) {
	githubTarget := syntheticGitHubAuthorizationURL(37405, strings.Repeat("a", 20), githubAuthorizationScope)
	codexTarget := syntheticCodexAuthorizationURLWithPort(strings.Repeat("c", 43), strings.Repeat("s", 43), 27890)
	for _, test := range []struct {
		target string
		port   int
	}{
		{target: githubTarget, port: 37405},
		{target: codexTarget, port: 27890},
	} {
		authorization, ok := parseWorkspaceLoginAuthorizationURL(test.target)
		if !ok || authorization.callbackPort != test.port {
			t.Fatalf("workspace authorization = (%+v, %t), want port %d", authorization, ok, test.port)
		}
	}
}
