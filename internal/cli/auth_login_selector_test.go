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
		context.Background(), "default", []authbroker.ProviderStatus{{
			Provider: "github", State: authbroker.ProviderCredentialConfigured, CredentialRevision: "revision:1",
		}}, strings.NewReader("\n"), output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "github" {
		t.Fatalf("selected provider = %q", provider)
	}
	for _, want := range []string{
		"Tobari · Provider login", "Context: default", "Choose a provider first", "Choose a provider",
		"GitHub (configured)", "GitHub CLI (gh)", "rotates the Context grant", "revokes previous Workspace handles",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("selector output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("numbered fallback contains terminal controls: %q", output.String())
	}
}

func TestAuthLoginProviderSelectorUsesRawTerminalAndRestoresIt(t *testing.T) {
	mode := &selectorModeFake{}
	selector := newAuthLoginProviderSelectorWithStyle(false)
	selector.wizard.mode = mode
	output := &bytes.Buffer{}

	provider, err := selector.Select(
		context.Background(), "default", []authbroker.ProviderStatus{{
			Provider: "github", State: authbroker.ProviderCredentialNotConfigured,
		}}, strings.NewReader("\r"), output,
	)
	if err != nil {
		t.Fatal(err)
	}
	if provider != "github" || mode.entered != 1 || mode.restored != 1 {
		t.Fatalf("provider/terminal = %q/%d/%d", provider, mode.entered, mode.restored)
	}
	if !strings.Contains(output.String(), "Tobari · Provider login") ||
		!strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("raw selector output = %q", output.String())
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
