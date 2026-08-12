package dockerruntime

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/credentialhost"
)

type githubOnlyResolver struct{ name string }

func (r *githubOnlyResolver) Resolve(name string) (string, error) {
	r.name = name
	if name != "gh" {
		return "", errors.New("unexpected executable")
	}
	return "/usr/bin/gh", nil
}

type githubOnlyAcquirer struct{ calls int }

func (a *githubOnlyAcquirer) LoginGitHub(_ context.Context, executable string, _ credentialhost.GitHubLoginStreams) (hostCredentialPayload, error) {
	a.calls++
	if executable != "/usr/bin/gh" {
		return hostCredentialPayload{}, errors.New("unexpected executable")
	}
	return hostCredentialPayload{secret: []byte("synthetic-token"), accountLabel: "octo-user"}, nil
}

func TestProviderHostExecutableIsGitHubOnly(t *testing.T) {
	if providerHostExecutable("github") != "gh" {
		t.Fatal("GitHub executable missing")
	}
	for _, provider := range []string{"aws", "datadog", "openai", "anthropic", "chatwork"} {
		if providerHostExecutable(provider) != "" {
			t.Fatalf("retired provider %q has executable", provider)
		}
	}
}

func TestGitHubHostLoginRejectsRetiredProviderBeforeAcquisition(t *testing.T) {
	resolver := &githubOnlyResolver{}
	acquirer := &githubOnlyAcquirer{}
	runtime := &Runtime{hostCLIs: resolver, credentialHost: acquirer}
	for _, provider := range []string{"aws", "datadog", "openai", "anthropic", "chatwork"} {
		_, err := runtime.runHostCredentialLoginOnTTY(context.Background(), "018bcfe5-687b-7000-8000-000000000099", provider, strings.NewReader(""), io.Discard)
		if err == nil {
			t.Fatalf("retired provider %q login accepted", provider)
		}
	}
	if resolver.name != "" || acquirer.calls != 0 {
		t.Fatalf("retired provider crossed resolver/acquirer: %q/%d", resolver.name, acquirer.calls)
	}
}
