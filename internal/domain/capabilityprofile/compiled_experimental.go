//go:build tobari_experimental

package capabilityprofile

// Compiled returns the immutable capability profile selected by this build.
func Compiled() Profile { return ProfileExperimental }
