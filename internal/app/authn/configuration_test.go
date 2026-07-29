package authn

import (
	"context"
	"errors"
	"testing"

	domainauthn "github.com/tasuku43/agentic-cli-foundry/internal/domain/authn"
	"github.com/tasuku43/agentic-cli-foundry/internal/domain/fault"
)

type configurationSourceStub struct {
	configuration domainauthn.UserConfiguration
	present       bool
	err           error
	calls         int
}

func (s *configurationSourceStub) Load(context.Context) (domainauthn.UserConfiguration, bool, error) {
	s.calls++
	return s.configuration, s.present, s.err
}

func validUserConfiguration(method domainauthn.Method) domainauthn.UserConfiguration {
	return domainauthn.UserConfiguration{SchemaVersion: domainauthn.UserConfigurationSchemaVersion, Method: method, Parameters: []domainauthn.PublicParameter{}}
}

func TestConfigurationResolverUsesExactFailClosedPrecedence(t *testing.T) {
	environment := &configurationSourceStub{configuration: validUserConfiguration(domainauthn.MethodOAuth2), present: true}
	persistent := &configurationSourceStub{configuration: validUserConfiguration(domainauthn.MethodPAT), present: true}
	resolved, err := NewConfigurationResolver(environment, persistent).Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Origin != ConfigurationOriginEnvironment || resolved.Configuration.Method != domainauthn.MethodOAuth2 || persistent.calls != 0 {
		t.Fatalf("resolved = %+v, persistent calls = %d", resolved, persistent.calls)
	}

	environment = &configurationSourceStub{present: false}
	persistent = &configurationSourceStub{configuration: validUserConfiguration(domainauthn.MethodPAT), present: true}
	resolved, err = NewConfigurationResolver(environment, persistent).Resolve(context.Background())
	if err != nil || resolved.Origin != ConfigurationOriginPersistent || resolved.Configuration.Method != domainauthn.MethodPAT {
		t.Fatalf("persistent resolved = %+v, err = %v", resolved, err)
	}
}

func TestConfigurationResolverNeverFallsBackFromInvalidPresentSource(t *testing.T) {
	for name, environment := range map[string]*configurationSourceStub{
		"decode error":  {present: true, err: errors.New("private source error")},
		"invalid value": {present: true, configuration: domainauthn.UserConfiguration{}},
	} {
		t.Run(name, func(t *testing.T) {
			persistent := &configurationSourceStub{configuration: validUserConfiguration(domainauthn.MethodPAT), present: true}
			_, err := NewConfigurationResolver(environment, persistent).Resolve(context.Background())
			var structured *fault.Error
			if !errors.As(err, &structured) || structured.Code != "authentication_configuration_invalid" {
				t.Fatalf("Resolve() error = %#v", err)
			}
			if persistent.calls != 0 {
				t.Fatalf("lower-priority source was probed %d times", persistent.calls)
			}
		})
	}
}

func TestConfigurationResolverFailsWhenMissingOrUnconfigured(t *testing.T) {
	missing := &configurationSourceStub{}
	_, err := NewConfigurationResolver(missing, &configurationSourceStub{}).Resolve(context.Background())
	var structured *fault.Error
	if !errors.As(err, &structured) || structured.Code != "authentication_configuration_missing" {
		t.Fatalf("missing error = %#v", err)
	}
	if _, err := NewConfigurationResolver(nil, missing).Resolve(context.Background()); err == nil {
		t.Fatal("nil configuration source passed")
	}
}
