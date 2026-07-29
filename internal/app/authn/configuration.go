package authn

import (
	"context"

	"github.com/tasuku43/agentic-cli-foundry/internal/app/portcheck"
	domainauthn "github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
)

// ConfigurationSource loads one complete non-secret configuration. Present is
// distinct from validity so an invalid high-priority source cannot fall back.
type ConfigurationSource interface {
	Load(context.Context) (configuration domainauthn.UserConfiguration, present bool, err error)
}

// ConfigurationStore is the application-owned persistence boundary. Its
// implementation stores only UserConfiguration, never credentials.
type ConfigurationStore interface {
	ConfigurationSource
	Save(context.Context, domainauthn.UserConfiguration) error
	Status(context.Context) domainauthn.ConfigurationStatus
}

// ConfigurationOrigin identifies the selected source without exposing values.
type ConfigurationOrigin string

const (
	ConfigurationOriginEnvironment ConfigurationOrigin = "environment"
	ConfigurationOriginPersistent  ConfigurationOrigin = "persistent"
)

// ResolvedConfiguration is one exact method selection. Authentication must not
// probe another method if the selected method later fails.
type ResolvedConfiguration struct {
	Configuration domainauthn.UserConfiguration
	Origin        ConfigurationOrigin
}

// ConfigurationResolver applies environment-over-persistent precedence.
type ConfigurationResolver struct {
	environment ConfigurationSource
	persistent  ConfigurationSource
}

// NewConfigurationResolver creates a fail-closed two-source resolver. Either
// source may report absence, but both ports must be configured.
func NewConfigurationResolver(environment, persistent ConfigurationSource) *ConfigurationResolver {
	return &ConfigurationResolver{environment: environment, persistent: persistent}
}

// Resolve selects exactly one valid source. A present invalid or failed source
// stops resolution and never probes the lower-priority source.
func (r *ConfigurationResolver) Resolve(ctx context.Context) (ResolvedConfiguration, error) {
	if ctx == nil {
		return ResolvedConfiguration{}, fault.New(fault.KindContract, "missing_authentication_configuration_context", "authentication configuration context is not configured", false)
	}
	if r == nil || portcheck.IsNil(r.environment) || portcheck.IsNil(r.persistent) {
		return ResolvedConfiguration{}, fault.New(fault.KindContract, "missing_authentication_configuration_source", "authentication configuration source is not configured", false)
	}
	if err := ctx.Err(); err != nil {
		return ResolvedConfiguration{}, configurationFault("authentication_configuration_canceled", fault.KindCanceled)
	}
	if resolved, present, err := loadConfiguration(ctx, r.environment, ConfigurationOriginEnvironment); err != nil || present {
		return resolved, err
	}
	if resolved, present, err := loadConfiguration(ctx, r.persistent, ConfigurationOriginPersistent); err != nil || present {
		return resolved, err
	}
	return ResolvedConfiguration{}, configurationFault("authentication_configuration_missing", fault.KindAuthentication)
}

func loadConfiguration(ctx context.Context, source ConfigurationSource, origin ConfigurationOrigin) (ResolvedConfiguration, bool, error) {
	configuration, present, err := source.Load(ctx)
	if err != nil {
		return ResolvedConfiguration{}, present, configurationFault("authentication_configuration_invalid", fault.KindAuthentication)
	}
	if !present {
		return ResolvedConfiguration{}, false, nil
	}
	if err := configuration.Validate(); err != nil {
		return ResolvedConfiguration{}, true, configurationFault("authentication_configuration_invalid", fault.KindAuthentication)
	}
	return ResolvedConfiguration{Configuration: configuration.Clone(), Origin: origin}, true, nil
}

func configurationFault(code string, kind fault.Kind) error {
	return fault.New(kind, code, "authentication configuration could not be resolved", false)
}
