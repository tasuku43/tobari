package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestAuthLoginProviderSelectorUsesNumberedFallback(t *testing.T) {
	selector := newAuthLoginProviderSelectorWithStyle(false)
	selector.wizard.mode = nil
	output := &bytes.Buffer{}

	provider, err := selector.Select(
		context.Background(), "default", []authbroker.ProviderStatus{
			{Provider: "anthropic", State: authbroker.ProviderCredentialNotConfigured},
			{Provider: "aws", State: authbroker.ProviderCredentialNotConfigured},
			{Provider: "datadog", State: authbroker.ProviderCredentialNotConfigured},
			{Provider: "github", State: authbroker.ProviderCredentialConfigured, Configured: true, CredentialRevision: "revision:1"},
			{Provider: "openai", State: authbroker.ProviderCredentialNotConfigured},
		},
		strings.NewReader("3\n"), output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "datadog" {
		t.Fatalf("selected provider = %q", provider)
	}
	for _, want := range []string{
		"Tobari · Provider login", "Context: default", "Choose a provider first", "Choose a provider",
		"GitHub CLI (gh), selected automatically", "AWS CLI (aws), selected automatically",
		"Tool: pup, selected automatically", "IAM Identity Center", "reviewed device flow", "reviewed US1 OAuth flow",
		"Codex 0.146.0, selected automatically", "reviewed ChatGPT device OAuth flow",
		"Claude Code 2.1.220, selected automatically", "reviewed inference setup-token OAuth flow",
		"GitHub (configured)", "rotates the Context grant", "revokes previous Workspace handles",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("selector output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("numbered fallback contains terminal controls: %q", output.String())
	}
}

func TestAuthLoginProviderOptionsNameExactReviewedAgentPairings(t *testing.T) {
	tests := []struct {
		provider    string
		label       string
		description string
	}{
		{provider: "openai", label: "OpenAI", description: "Tool: Codex 0.146.0, selected automatically. Login: reviewed ChatGPT device OAuth flow."},
		{provider: "anthropic", label: "Anthropic", description: "Tool: Claude Code 2.1.220, selected automatically. Login: reviewed inference setup-token OAuth flow."},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			option := authLoginProviderOption(authbroker.ProviderStatus{
				Provider: test.provider, State: authbroker.ProviderCredentialNotConfigured,
			})
			if option.value != test.provider || option.label != test.label || option.description != test.description {
				t.Fatalf("provider option = %+v", option)
			}
		})
	}
}

func TestAuthLoginProviderSelectorCancellation(t *testing.T) {
	selector := newAuthLoginProviderSelectorWithStyle(false)
	selector.wizard.mode = nil
	_, err := selector.Select(
		context.Background(), "default", []authbroker.ProviderStatus{{
			Provider: "github", State: authbroker.ProviderCredentialNotConfigured,
		}}, strings.NewReader("q\n"), &bytes.Buffer{},
	)
	if err != context.Canceled {
		t.Fatalf("Select() error = %v, want context.Canceled", err)
	}
}
