//go:build tobari_dev && tobari_research

package cli

import (
	"github.com/tasuku43/tobari/internal/domain/authbroker"
	"github.com/tasuku43/tobari/internal/domain/tobari"
)

type contextAuthenticationJSONProjection struct {
	Mode               string                              `json:"mode"`
	BrokerState        string                              `json:"broker_state,omitempty"`
	DeclaredBindings   authbroker.AuthenticationRoute      `json:"declared_bindings,omitempty"`
	UndeclaredBindings authbroker.AuthenticationRoute      `json:"undeclared_bindings,omitempty"`
	Providers          []contextAuthProviderJSONProjection `json:"providers"`
}

type contextAuthProviderJSONProjection struct {
	Provider           string  `json:"provider"`
	State              string  `json:"state"`
	AccountLabel       *string `json:"account_label"`
	CredentialRevision *string `json:"credential_revision"`
}

func contextAuthenticationJSON(authentication tobari.ManifestAuthentication) contextAuthenticationJSONProjection {
	providers := make([]contextAuthProviderJSONProjection, 0, len(authentication.Providers))
	if authentication.Providers == nil {
		providers = nil
	} else {
		for _, provider := range authentication.Providers {
			providers = append(providers, contextAuthProviderJSONProjection{
				Provider: provider.Provider, State: provider.State, AccountLabel: provider.AccountLabel,
				CredentialRevision: optionalString(provider.CredentialRevision),
			})
		}
	}
	projection := contextAuthenticationJSONProjection{
		Mode: contextAuthenticationMode(authentication), Providers: providers,
		BrokerState: authentication.BrokerState,
	}
	if projection.Mode == tobari.ManifestAuthenticationModeBroker || projection.Mode == tobari.ManifestAuthenticationModeNotApplicable {
		projection.DeclaredBindings = authbroker.AuthenticationRouteBrokerRequired
		projection.UndeclaredBindings = authbroker.AuthenticationRouteWorkspaceOwnedCompatibility
	}
	return projection
}
