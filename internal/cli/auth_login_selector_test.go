package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAuthLoginProviderSelectorUsesNumberedFallback(t *testing.T) {
	selector := newAuthLoginProviderSelectorWithStyle(false)
	selector.wizard.mode = nil
	output := &bytes.Buffer{}

	provider, err := selector.Select(
		context.Background(), "default", []string{"aws", "datadog", "github"},
		strings.NewReader("2\n"), output,
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
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("selector output = %q, want %q", output.String(), want)
		}
	}
	if strings.Contains(output.String(), "\x1b[") {
		t.Fatalf("numbered fallback contains terminal controls: %q", output.String())
	}
}

func TestAuthLoginProviderSelectorCancellation(t *testing.T) {
	selector := newAuthLoginProviderSelectorWithStyle(false)
	selector.wizard.mode = nil
	_, err := selector.Select(
		context.Background(), "default", []string{"github"}, strings.NewReader("q\n"), &bytes.Buffer{},
	)
	if err != context.Canceled {
		t.Fatalf("Select() error = %v, want context.Canceled", err)
	}
}
