//go:build !tobari_research

package capabilitysurface

// Compiled returns the release surface for all builds that do not explicitly
// satisfy the complete research build tuple.
func Compiled() CapabilitySurface { return CapabilitySurfaceRelease }
