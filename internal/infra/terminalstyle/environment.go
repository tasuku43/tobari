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
