// Package capabilityprofile identifies the compile-time product surface of one
// Tobari binary. Profiles are deliberately not runtime configurable.
package capabilityprofile

import "fmt"

// Profile is the closed maturity set compiled into one executable.
type Profile string

const (
	ProfileStandard     Profile = "standard"
	ProfileExperimental Profile = "experimental"
)

// Validate rejects unknown profiles before a build can claim compatibility.
func (p Profile) Validate() error {
	switch p {
	case ProfileStandard, ProfileExperimental:
		return nil
	default:
		return fmt.Errorf("capability profile is invalid: %q", p)
	}
}

// IncludesExperimental reports whether development-only capabilities are part
// of this immutable build surface.
func (p Profile) IncludesExperimental() bool {
	return p == ProfileExperimental
}
