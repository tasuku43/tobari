package cli

import (
	"context"
	"strings"
	"testing"
)

func TestPreReleaseCleanBreakExposesNoMigrationCommandOrNamespace(t *testing.T) {
	catalog := DefaultCatalog()
	for _, path := range []string{"migrate", "migrate apply"} {
		if _, found := catalog.Lookup(path); found {
			t.Fatalf("predecessor migration path %q remains public", path)
		}
	}
	command, stdout, stderr := newTestCLI(passingInspector("unused"))
	if code := command.RunContext(context.Background(), []string{"migrate", "apply"}); code != ExitUsage {
		t.Fatalf("migrate apply code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "unknown command") {
		t.Fatalf("migrate apply did not fail as an unknown public command: %q", stderr.String())
	}
}
