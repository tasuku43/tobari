//go:build !tobari_experimental

package cli

import "testing"

func TestStandardCatalogHasNoBrokerAuthenticationCommands(t *testing.T) {
	for _, path := range []string{"auth login", "auth import", "auth logout", "auth status"} {
		if _, found := DefaultCatalog().Lookup(path); found {
			t.Fatalf("standard catalog exposed experimental command %q", path)
		}
	}
}
