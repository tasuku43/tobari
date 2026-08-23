//go:build tobari_dev && tobari_research

package capabilitysurface

// Compiled returns the repository-only research surface.
func Compiled() CapabilitySurface { return CapabilitySurfaceResearch }
