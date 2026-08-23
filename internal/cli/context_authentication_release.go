//go:build !tobari_research

package cli

import "github.com/tasuku43/tobari/internal/domain/tobari"

// contextAuthenticationJSONProjection is deliberately smaller on the release
// surface: standard diagnostics can report only native Workspace ownership or
// that authentication was not applicable to the task.
type contextAuthenticationJSONProjection struct {
	Mode string `json:"mode"`
}

func contextAuthenticationJSON(authentication tobari.ManifestAuthentication) contextAuthenticationJSONProjection {
	mode := contextAuthenticationMode(authentication)
	if mode != tobari.ManifestAuthenticationModeNative {
		mode = tobari.ManifestAuthenticationModeNotApplicable
	}
	return contextAuthenticationJSONProjection{Mode: mode}
}
