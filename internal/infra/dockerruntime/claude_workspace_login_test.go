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

func syntheticClaudeWorkspaceAuthorizationURL() string {
	query := url.Values{
		"code":                  {"true"},
		"client_id":             {claudeLoginClientID},
		"response_type":         {"code"},
		"redirect_uri":          {claudeLoginRedirectURI},
		"scope":                 {claudeWorkspaceLoginScope},
		"code_challenge":        {strings.Repeat("a", 43)},
		"code_challenge_method": {"S256"},
		"state":                 {strings.Repeat("b", 43)},
	}
	return claudeLoginURLPrefix + query.Encode()
}

func TestClaudeWorkspaceLoginObserverOpensFragmentedPlainURLWithoutChangingOutput(t *testing.T) {
	target := syntheticClaudeWorkspaceAuthorizationURL()
	input := "Browser didn't open? Use the url below to sign in (c to copy)\n\n" + target + "\n\n Paste code here if prompted > "
	var destination bytes.Buffer
	var opened []string
	observer := &workspaceLoginOutputObserver{trigger: func(candidate string) bool {
		opened = append(opened, candidate)
		return true
	}}
	writer := &workspaceLoginObservingWriter{destination: &destination, observer: observer}
	for offset := 0; offset < len(input); {
		end := min(offset+3, len(input))
		if _, err := io.WriteString(writer, input[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if destination.String() != input {
		t.Fatalf("Claude output changed\n got: %q\nwant: %q", destination.String(), input)
	}
	if !reflect.DeepEqual(opened, []string{target}) {
		t.Fatalf("browser targets = %q", opened)
	}
}

func TestClaudeWorkspaceLoginObserverReassemblesTerminalWidthWrappedURL(t *testing.T) {
	target := syntheticClaudeWorkspaceAuthorizationURL()
	var wrapped strings.Builder
	for offset := 0; offset < len(target); offset += 97 {
		end := min(offset+97, len(target))
		wrapped.WriteString(target[offset:end])
		wrapped.WriteByte('\n')
	}
	input := "Browser didn't open? Use the url below to sign in (c to copy)\n\n" + wrapped.String() +
		"\n Paste code here if prompted > "
	var destination bytes.Buffer
	var opened []string
	observer := &workspaceLoginOutputObserver{trigger: func(candidate string) bool {
		opened = append(opened, candidate)
		return true
	}}
	writer := &workspaceLoginObservingWriter{destination: &destination, observer: observer}
	for offset := 0; offset < len(input); {
		end := min(offset+2, len(input))
		if _, err := io.WriteString(writer, input[offset:end]); err != nil {
			t.Fatal(err)
		}
		offset = end
	}
	if destination.String() != input {
		t.Fatalf("Claude wrapped output changed\n got: %q\nwant: %q", destination.String(), input)
	}
	if !reflect.DeepEqual(opened, []string{target}) {
		t.Fatalf("browser targets = %q", opened)
	}
}

func TestClaudeWorkspaceLoginObserverAcceptsOnlyExactLabeledHyperlink(t *testing.T) {
	target := syntheticClaudeWorkspaceAuthorizationURL()
	valid := "\x1b]8;;" + target + "\a" + target + "\x1b]8;;\a\n"
	wrapped := "\x1b]8;id=synthetic1;" + target + "\a\x1b[37m" + target[:83] + "\x1b[39m\x1b]8;;\a\r\r\n"
	for _, test := range []struct {
		name string
		line string
		want int
	}{
		{name: "exact", line: valid, want: 1},
		{name: "Claude width row", line: wrapped, want: 1},
		{name: "changed label", line: strings.Replace(valid, "\ahttps://", "\acopy https://", 1)},
		{name: "unsafe id", line: strings.Replace(wrapped, "id=synthetic1", "id=synthetic/1", 1)},
		{name: "unknown label SGR", line: strings.Replace(wrapped, "\x1b[37m", "\x1b[31m", 1)},
		{name: "prefix", line: "prefix " + valid},
		{name: "suffix", line: strings.TrimSuffix(valid, "\n") + " suffix\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			opened := 0
			observer := &workspaceLoginOutputObserver{trigger: func(string) bool { opened++; return true }}
			writer := &workspaceLoginObservingWriter{destination: io.Discard, observer: observer}
			_, _ = io.WriteString(writer, test.line)
			if opened != test.want {
				t.Fatalf("browser opens = %d, want %d", opened, test.want)
			}
		})
	}
}

func TestClaudeWorkspaceLoginBridgeOpensHostWithoutCallbackListener(t *testing.T) {
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
