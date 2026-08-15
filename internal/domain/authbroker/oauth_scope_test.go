package authbroker

import (
	"slices"
	"strings"
	"testing"
)

func TestNormalizeOAuthScopesAcceptsProviderValuesWithoutACompiledNameList(t *testing.T) {
	want := []string{"future:capability", "user:inference"}
	got, err := NormalizeOAuthScopes([]string{"user:inference", "future:capability"})
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("normalized=%q error=%v", got, err)
	}
}

func TestNormalizeOAuthScopesRejectsAmbiguousOrUnboundedValues(t *testing.T) {
	for _, scopes := range [][]string{
		nil,
		{},
		{"same", "same"},
		{"contains space"},
		{"contains\"quote"},
		{"contains\\slash"},
		{strings.Repeat("x", 129)},
	} {
		if normalized, err := NormalizeOAuthScopes(scopes); err == nil || normalized != nil {
			t.Fatalf("scopes %q normalized as %q", scopes, normalized)
		}
	}
	tooMany := make([]string, 33)
	for index := range tooMany {
		tooMany[index] = "scope-" + strings.Repeat("x", index+1)
	}
	if _, err := NormalizeOAuthScopes(tooMany); err == nil {
		t.Fatal("too many OAuth scopes were accepted")
	}
}
