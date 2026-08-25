// Package terminalstyle owns the narrow host-environment boundary used by
// terminal presentation policy.
package terminalstyle

import "os"

// NoColorRequested reports whether NO_COLOR is present. Its value is ignored,
// matching the presence-only contract and avoiding any unbounded parsing.
func NoColorRequested() bool {
	_, present := os.LookupEnv("NO_COLOR")
	return present
}

// IntegrationFaultDiagnosticsRequested is a test-harness-only switch for one
// bounded, secret-free fault-contract diagnostic. It is intentionally owned
// by the existing host-environment adapter rather than the CLI package.
func IntegrationFaultDiagnosticsRequested() bool {
	return os.Getenv("TOBARI_INTEGRATION_FAULT_DIAGNOSTICS") == "true"
}
