//go:build !linux && !darwin

package terminal

// IsTerminal reports false on hosts without the reviewed termios adapter.
func IsTerminal(any) bool { return false }

// IsCanonical reports false on hosts without the reviewed termios adapter.
func IsCanonical(any) bool { return false }
