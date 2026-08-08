//go:build !linux && !darwin

package terminal

// IsTerminal reports false on hosts without the reviewed termios adapter.
func IsTerminal(any) bool { return false }
