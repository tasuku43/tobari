//go:build !tobari_experimental

package cli

import (
	"strings"
	"testing"
)

func TestStandardCatalogHasNoOperatorConsoleCommand(t *testing.T) {
	if _, found := DefaultCatalog().Lookup("serve"); found {
		t.Fatal("standard catalog exposed experimental command \"serve\"")
	}
	command, _, stderr := newTestCLI(passingInspector("unused"))
	if code := runCLI(command, []string{"serve"}); code != ExitUsage {
		t.Fatalf("standard serve code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "Unknown command") {
		t.Fatalf("standard serve failure = %q", stderr.String())
	}
}
