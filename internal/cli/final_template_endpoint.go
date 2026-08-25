package cli

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/tasuku43/tobari/internal/domain/tobari"
)

// parseBoundedGraphQLEndpoint parses the public CLI scalar before it enters
// the domain. The domain then validates the normalized exact rule and all
// Template destination/method ceilings before publication.
func parseBoundedGraphQLEndpoint(raw string) (tobari.ManifestPolicyExactRule, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Hostname() == "" || parsed.Port() == "" || parsed.Path == "" || parsed.Path[0] != '/' {
		return tobari.ManifestPolicyExactRule{}, fmt.Errorf("GraphQL endpoint must be an exact HTTPS URL with an explicit port and path")
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return tobari.ManifestPolicyExactRule{}, fmt.Errorf("GraphQL endpoint port is invalid")
	}
	endpoint := tobari.ManifestPolicyExactRule{Scheme: parsed.Scheme, Host: parsed.Hostname(), Port: port, Method: "POST", Path: parsed.EscapedPath()}
	if err := endpoint.Validate(); err != nil {
		return tobari.ManifestPolicyExactRule{}, fmt.Errorf("GraphQL endpoint is invalid: %w", err)
	}
	return endpoint, nil
}
