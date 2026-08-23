// Package capabilitysurface identifies the immutable command surface compiled
// into one Tobari executable. It is build authority, never runtime state.
package capabilitysurface

import "fmt"

// CapabilitySurface is the closed public/research build surface.
type CapabilitySurface string

const (
	CapabilitySurfaceRelease  CapabilitySurface = "release"
	CapabilitySurfaceResearch CapabilitySurface = "research"
)

// Validate rejects values that could be introduced by runtime configuration.
func (s CapabilitySurface) Validate() error {
	switch s {
	case CapabilitySurfaceRelease, CapabilitySurfaceResearch:
		return nil
	default:
		return fmt.Errorf("capability surface is invalid: %q", s)
	}
}

// IncludesResearch reports whether this immutable executable contains the
// repository-only research command and topology surface.
func (s CapabilitySurface) IncludesResearch() bool {
	return s == CapabilitySurfaceResearch
}
