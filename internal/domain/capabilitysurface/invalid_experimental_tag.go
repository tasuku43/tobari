//go:build tobari_experimental

package capabilitysurface

// The predecessor tag is intentionally a hard error. Go otherwise ignores an
// unknown tag and would silently produce a narrowed release-surface binary.
var _ = tobari_experimental_build_tag_is_retired
