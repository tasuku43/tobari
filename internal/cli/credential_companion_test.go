package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/infra/companionruntime"
)

func TestCredentialCompanionArgZeroIsExactAndPrivate(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		value string
		want  bool
	}{
		{value: companionruntime.PrivateArg0, want: true},
		{value: "./" + companionruntime.PrivateArg0, want: false},
		{value: "tobari", want: false},
		{value: "", want: false},
	} {
		if got := IsCredentialCompanionArg0(test.value); got != test.want {
			t.Fatalf("IsCredentialCompanionArg0(%q) = %t; want %t", test.value, got, test.want)
		}
	}
	for _, spec := range DefaultCatalog().Commands() {
		if strings.Contains(spec.Path, companionruntime.PrivateArg0) ||
			strings.Contains(spec.Summary, companionruntime.PrivateArg0) {
			t.Fatalf("public catalog exposed private companion mode: %+v", spec)
		}
	}
}

func TestCredentialCompanionRejectsInvalidBootstrapSilently(t *testing.T) {
	t.Parallel()
	if code := RunCredentialCompanionContext(
		context.Background(),
		nil,
		strings.NewReader(""),
	); code != ExitUnavailable {
		t.Fatalf("invalid bootstrap exit = %d; want %d", code, ExitUnavailable)
	}
	if code := RunCredentialCompanionContext(
		context.Background(),
		[]string{"--canary"},
		strings.NewReader(""),
	); code != ExitUsage {
		t.Fatalf("argument rejection exit = %d; want %d", code, ExitUsage)
	}
}
