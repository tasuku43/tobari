package cli

import (
	"context"
	"fmt"

	"github.com/tasuku43/tobari/internal/domain/buildidentity"
	"github.com/tasuku43/tobari/internal/domain/fault"
	"github.com/tasuku43/tobari/internal/domain/operation"
	"github.com/tasuku43/tobari/internal/infra/dockerruntime"
)

type versionJSONDocument struct {
	SchemaVersion int                   `json:"schema_version"`
	BuildIdentity versionJSONProjection `json:"build_identity"`
}

type versionJSONProjection struct {
	Version                 string `json:"version"`
	Commit                  string `json:"commit"`
	ResolverChannel         string `json:"resolver_channel"`
	DevelopmentSource       bool   `json:"development_source"`
	CapabilityProfile       string `json:"capability_profile"`
	GatewayRequiredAPI      int    `json:"gateway_required_api"`
	GatewaySelectedAPI      int    `json:"gateway_selected_api"`
	AuthBrokerRequiredAPI   int    `json:"auth_broker_required_api,omitempty"`
	AuthBrokerSelectedAPI   int    `json:"auth_broker_selected_api,omitempty"`
	Compatible              bool   `json:"compatible"`
	DevelopmentBuildCommand string `json:"development_build_command"`
	DevelopmentBinary       string `json:"development_binary"`
}

func runVersion(ctx context.Context, c *CLI, command CommandSpec, _ operation.Intent, inputs ParsedInputs) int {
	format, err := parseSuccessFormat(inputs.One("--format"))
	if err != nil {
		return c.failUsage(ctx, "invalid_arguments", err.Error()+"; usage: "+command.Usage(), "help version", "Correct the command arguments.")
	}
	identity, err := dockerruntime.BuildIdentity(c.Version, c.Commit)
	if err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract, "invalid_build_identity",
			"The executable build identity is invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed executable and embedded runtime metadata."},
		))
	}
	if err := identity.Validate(); err != nil {
		return c.fail(ctx, fault.Wrap(
			fault.KindContract, "invalid_build_identity",
			"The executable build identity is invalid.", false, err,
			fault.NextAction{Command: "doctor", Reason: "Inspect the installed executable and embedded runtime metadata."},
		))
	}
	output, err := renderVersion(identity, format, format == successFormatText && humanStyleAllowed(ctx, c, c.Out))
	if err != nil {
		return c.fail(ctx, err)
	}
	return c.emitResult(ctx, output)
}

func renderVersion(identity buildidentity.Identity, format successFormat, color bool) ([]byte, error) {
	buildCommand, binary, development := identity.DevelopmentRecovery()
	if format == successFormatJSON {
		output, err := marshalCommandJSON("version", versionJSONDocument{
			SchemaVersion: 1,
			BuildIdentity: versionJSONProjection{Version: identity.Version, Commit: identity.Commit,
				ResolverChannel: string(identity.ResolverChannel), DevelopmentSource: identity.DevelopmentSource,
				CapabilityProfile:  string(identity.CapabilityProfile),
				GatewayRequiredAPI: identity.Gateway.RequiredAPI, GatewaySelectedAPI: identity.Gateway.SelectedAPI,
				AuthBrokerRequiredAPI: identity.AuthBroker.RequiredAPI, AuthBrokerSelectedAPI: identity.AuthBroker.SelectedAPI,
				Compatible: identity.Compatible(), DevelopmentBuildCommand: buildCommand, DevelopmentBinary: binary,
			},
		})
		if err != nil {
			return nil, fault.Wrap(fault.KindContract, "output_encoding_failed", "The build identity JSON could not be encoded.", false, err)
		}
		return append(output, '\n'), nil
	}
	if format != successFormatText {
		return nil, fmt.Errorf("unsupported version output format %q", format)
	}
	marker, token, state := "✓", styleSuccess, "compatible"
	if !identity.Compatible() {
		marker, token, state = "!", styleWarning, "incompatible or incomplete"
	}
	output := newHumanOutput(color)
	output.heading(marker, "Tobari build", token)
	output.row("Version", identity.Version, styleText)
	output.row("Commit", identity.Commit, styleText)
	output.row("Resolver", string(identity.ResolverChannel), styleText)
	output.row("Capabilities", string(identity.CapabilityProfile), styleText)
	output.row("Gateway API", fmt.Sprintf("required %d, selected %d", identity.Gateway.RequiredAPI, identity.Gateway.SelectedAPI), styleText)
	if identity.CapabilityProfile.IncludesExperimental() {
		output.row("Auth Broker API", fmt.Sprintf("required %d, selected %d", identity.AuthBroker.RequiredAPI, identity.AuthBroker.SelectedAPI), styleText)
	}
	output.row("Compatibility", state, token)
	if development {
		output.row("Build", buildCommand, styleAccent)
		output.row("Binary", binary, styleAccent)
	}
	return output.bytes(), nil
}

func versionOutputFields() []OutputField {
	fields := []OutputField{
		{Name: "version", Type: OutputFieldTypeString, Description: "Release or development version embedded in the executable."},
		{Name: "commit", Type: OutputFieldTypeString, Description: "Full source commit embedded in the executable, or unknown when unavailable."},
		{Name: "resolver_channel", Type: OutputFieldTypeString, Description: "Compiled image authority: published or development."},
		{Name: "development_source", Type: OutputFieldTypeBoolean, Description: "Whether build metadata proves the contributor development resolver."},
		{Name: "capability_profile", Type: OutputFieldTypeString, Description: "Immutable capability surface: standard or experimental."},
		{Name: "gateway_required_api", Type: OutputFieldTypeInteger, Description: "Gateway API required by canonical source."},
		{Name: "gateway_selected_api", Type: OutputFieldTypeInteger, Description: "Gateway API selected by the compiled resolver."},
	}
	if buildIdentityHasBroker() {
		fields = append(fields,
			OutputField{Name: "auth_broker_required_api", Type: OutputFieldTypeInteger, Description: "Auth Broker API required by canonical source."},
			OutputField{Name: "auth_broker_selected_api", Type: OutputFieldTypeInteger, Description: "Auth Broker API selected by the compiled resolver."},
		)
	}
	return append(fields,
		OutputField{Name: "compatible", Type: OutputFieldTypeBoolean, Description: "Whether source metadata is complete and selected component APIs match source requirements."},
		OutputField{Name: "development_build_command", Type: OutputFieldTypeString, Description: "Exact repository build command only for a proven development build; otherwise empty."},
		OutputField{Name: "development_binary", Type: OutputFieldTypeString, Description: "Exact repository binary path only for a proven development build; otherwise empty."},
	)
}

func buildIdentityHasBroker() bool {
	identity, err := dockerruntime.BuildIdentity("dev", buildidentity.UnknownCommit)
	return err == nil && identity.CapabilityProfile.IncludesExperimental()
}
