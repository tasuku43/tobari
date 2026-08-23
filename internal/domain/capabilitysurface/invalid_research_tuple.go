//go:build tobari_research && !tobari_dev

package capabilitysurface

// A research surface without the development resolver is not an admitted
// tuple. Keep the failure at compile time so runtime input cannot select it.
var _ = tobari_research_requires_development_resolver
