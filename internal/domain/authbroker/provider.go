// Package authbroker defines the secret-free provider and result contracts for
// Tobari's Context-scoped authentication broker.
package authbroker

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	LegacyProviderSchemaVersion = 1
	ProviderSchemaVersion       = 2
	LegacyProviderSchemaID      = "tobari.auth-provider.v1"
	ProviderSchemaID            = "tobari.auth-provider.v2"
	MaxProviderDocumentBytes    = 64 * 1024
	MaxPrimarySecretBytes       = 32 * 1024
	MaxTemplateBytes            = 32 * 1024
	maxProviders                = 64
	maxProjections              = 32
	maxBindings                 = 64
	maxSigningBindings          = 8
	maxSecretHeaders            = 32
)

var (
	providerIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	helperIDPattern   = regexp.MustCompile(`^[a-z0-9]+(?:[._-][a-z0-9]+)*$`)
	environmentName   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
	homePathSegment   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type AcquisitionMode string

const (
	AcquisitionBuiltinHelper AcquisitionMode = "builtin_helper"
	AcquisitionStdinImport   AcquisitionMode = "stdin_import"
)

func (m AcquisitionMode) Validate() error {
	switch m {
	case AcquisitionBuiltinHelper, AcquisitionStdinImport:
		return nil
	default:
		return fmt.Errorf("provider acquisition mode is invalid: %q", m)
	}
}

type CredentialKind string

const (
	// CredentialPrimarySecret is the schema-v1 static credential spelling. It
	// remains valid only so owner manifests and existing built-ins keep their
	// exact public contract.
	CredentialPrimarySecret           CredentialKind = "primary_secret"
	CredentialAWSSSOSession           CredentialKind = "aws_sso_session"
	CredentialDatadogOAuthSession     CredentialKind = "datadog_oauth_session"      // #nosec G101 -- public credential-kind discriminator, not a credential.
	CredentialOpenAICodexOAuthSession CredentialKind = "openai_codex_oauth_session" // #nosec G101 -- public credential-kind discriminator, not a credential.
)

func (k CredentialKind) Validate() error {
	switch k {
	case CredentialPrimarySecret, CredentialAWSSSOSession, CredentialDatadogOAuthSession,
		CredentialOpenAICodexOAuthSession:
		return nil
	default:
		return fmt.Errorf("provider credential kind is invalid: %q", k)
	}
}

type SigningBindingKind string

const SigningBindingAWSSigV4 SigningBindingKind = "aws_sigv4"

func (k SigningBindingKind) Validate() error {
	if k != SigningBindingAWSSigV4 {
		return fmt.Errorf("signing binding kind is invalid: %q", k)
	}
	return nil
}

type WorkspaceProjectionKind string

const (
	WorkspaceProjectionEnvironment  WorkspaceProjectionKind = "env"
	WorkspaceProjectionCompleteFile WorkspaceProjectionKind = "complete_file"
)

func (k WorkspaceProjectionKind) Validate() error {
	switch k {
	case WorkspaceProjectionEnvironment, WorkspaceProjectionCompleteFile:
		return nil
	default:
		return fmt.Errorf("Workspace projection kind is invalid: %q", k)
	}
}

type SourceFormat string

const (
	SourceFormatRaw    SourceFormat = "raw"
	SourceFormatBearer SourceFormat = "bearer"
	SourceFormatToken  SourceFormat = "token"
)

func (f SourceFormat) Validate() error {
	switch f {
	case SourceFormatRaw, SourceFormatBearer, SourceFormatToken:
		return nil
	default:
		return fmt.Errorf("header source format is invalid: %q", f)
	}
}

type DestinationFormat string

const (
	DestinationFormatPreserveScheme DestinationFormat = "preserve_scheme"
	DestinationFormatRaw            DestinationFormat = "raw"
	DestinationFormatBearer         DestinationFormat = "bearer"
	DestinationFormatToken          DestinationFormat = "token"
)

func (f DestinationFormat) Validate() error {
	switch f {
	case DestinationFormatPreserveScheme, DestinationFormatRaw,
		DestinationFormatBearer, DestinationFormatToken:
		return nil
	default:
		return fmt.Errorf("header destination format is invalid: %q", f)
	}
}

type Acquisition struct {
	Mode   AcquisitionMode `json:"mode"`
	Helper string          `json:"helper,omitempty"`
}

type Credential struct {
	Kind CredentialKind `json:"kind"`
}

// WorkspaceProjection describes one complete projection into a Workspace.
// A complete_file replaces the entire relative HOME file; it never patches an
// existing file or escapes the Workspace HOME.
type WorkspaceProjection struct {
	Kind     WorkspaceProjectionKind `json:"kind"`
	Name     string                  `json:"name,omitempty"`
	Path     string                  `json:"path,omitempty"`
	Template string                  `json:"template"`
}

type BindingTarget struct {
	Scheme string `json:"scheme"`
	Host   string `json:"host"`
	Port   int    `json:"port"`
}

type BindingSource struct {
	Header  string         `json:"header"`
	Formats []SourceFormat `json:"formats"`
}

// NormalizedBindingSource is the one-format source shape sent to the Gateway
// and broker after a public manifest's formats array has been expanded.
type NormalizedBindingSource struct {
	Header string       `json:"header"`
	Format SourceFormat `json:"format"`
}

type BindingDestination struct {
	Header      string            `json:"header"`
	Format      DestinationFormat `json:"format"`
	SecretField CredentialKind    `json:"secret_field"`
}

// HeaderBinding recognizes one exact HTTPS request header. Source describes
// the project-handle value and Destination describes the post-authorization
// credential value without exposing either value in the manifest.
type HeaderBinding struct {
	Target        BindingTarget      `json:"target"`
	Source        BindingSource      `json:"source"`
	Destination   BindingDestination `json:"destination"`
	SecretHeaders []string           `json:"secret_headers"`
}

// AWSSigV4Target is deliberately a suffix allowlist rather than an arbitrary
// endpoint. The aws_sigv4 plan accepts only the reviewed commercial AWS suffix
// enumerated by validation below.
type AWSSigV4Target struct {
	Scheme      string   `json:"scheme"`
	Port        int      `json:"port"`
	DNSSuffixes []string `json:"dns_suffixes"`
}

type AWSSigV4Source struct {
	AuthorizationHeader string `json:"authorization_header"`
	SecurityTokenHeader string `json:"security_token_header"`
}

// AWSSigV4Binding contains only non-secret request-recognition metadata. The
// signing algorithm, canonicalization, and credential exchange are fixed by
// reviewed infrastructure and cannot be supplied by a manifest.
type AWSSigV4Binding struct {
	Target        AWSSigV4Target `json:"target"`
	Source        AWSSigV4Source `json:"source"`
	SecretHeaders []string       `json:"secret_headers"`
}

// SigningBinding is a closed discriminated union. A new behavior requires a
// new typed field and domain validation rather than an executable manifest.
type SigningBinding struct {
	Kind     SigningBindingKind `json:"kind"`
	AWSSigV4 *AWSSigV4Binding   `json:"aws_sigv4,omitempty"`
}

// Provider is a parsed schema-v1 or schema-v2 provider manifest. It contains
// no real credential, root key, or project-bound handle.
type Provider struct {
	SchemaVersion        int                   `json:"schema_version"`
	ID                   string                `json:"id"`
	DisplayName          string                `json:"display_name"`
	Acquisition          Acquisition           `json:"acquisition"`
	Credential           Credential            `json:"credential"`
	WorkspaceProjections []WorkspaceProjection `json:"workspace_projections"`
	HeaderBindings       []HeaderBinding       `json:"header_bindings"`
	SigningBindings      []SigningBinding      `json:"signing_bindings,omitempty"`
}

func ValidateProviderID(id string) error {
	if len(id) == 0 || len(id) > 64 || !providerIDPattern.MatchString(id) {
		return fmt.Errorf("provider ID must be a lowercase identifier of at most 64 bytes")
	}
	return nil
}

func (p Provider) Validate() error {
	_, err := NormalizeProviders([]Provider{p})
	return err
}

func validateProvider(p Provider) error {
	if p.SchemaVersion != LegacyProviderSchemaVersion && p.SchemaVersion != ProviderSchemaVersion {
		return fmt.Errorf(
			"provider %q schema_version must be %d or %d",
			p.ID, LegacyProviderSchemaVersion, ProviderSchemaVersion,
		)
	}
	if err := ValidateProviderID(p.ID); err != nil {
		return err
	}
	if err := validateDisplayText("provider display_name", p.DisplayName, 96); err != nil {
		return err
	}
	if err := p.Acquisition.Mode.Validate(); err != nil {
		return err
	}
	switch p.Acquisition.Mode {
	case AcquisitionBuiltinHelper:
		if len(p.Acquisition.Helper) == 0 || len(p.Acquisition.Helper) > 64 ||
			!helperIDPattern.MatchString(p.Acquisition.Helper) {
			return fmt.Errorf("builtin_helper acquisition requires one bounded helper identifier")
		}
	case AcquisitionStdinImport:
		if p.Acquisition.Helper != "" {
			return fmt.Errorf("stdin_import acquisition cannot declare a helper")
		}
	}
	if err := p.Credential.Kind.Validate(); err != nil {
		return err
	}
	if err := validateCredentialPlan(p); err != nil {
		return err
	}
	if len(p.WorkspaceProjections) == 0 || len(p.WorkspaceProjections) > maxProjections {
		return fmt.Errorf("provider %q must declare 1..%d Workspace projections", p.ID, maxProjections)
	}
	handleProjections := 0
	for index, projection := range p.WorkspaceProjections {
		if err := validateWorkspaceProjection(projection); err != nil {
			return fmt.Errorf("provider %q Workspace projection %d: %w", p.ID, index, err)
		}
		handleProjections += strings.Count(projection.Template, "${HANDLE}")
	}
	if handleProjections == 0 {
		return fmt.Errorf("provider %q must project ${HANDLE} at least once", p.ID)
	}
	if len(p.HeaderBindings) > maxBindings {
		return fmt.Errorf("provider %q cannot declare more than %d header bindings", p.ID, maxBindings)
	}
	for index, binding := range p.HeaderBindings {
		if err := validateHeaderBinding(binding); err != nil {
			return fmt.Errorf("provider %q header binding %d: %w", p.ID, index, err)
		}
		if binding.Destination.SecretField != p.Credential.Kind {
			return fmt.Errorf(
				"provider %q header binding %d secret_field must match credential kind %q",
				p.ID, index, p.Credential.Kind,
			)
		}
	}
	if len(p.SigningBindings) > maxSigningBindings {
		return fmt.Errorf("provider %q cannot declare more than %d signing bindings", p.ID, maxSigningBindings)
	}
	for index, binding := range p.SigningBindings {
		if err := validateSigningBinding(binding); err != nil {
			return fmt.Errorf("provider %q signing binding %d: %w", p.ID, index, err)
		}
	}
	if len(p.HeaderBindings)+len(p.SigningBindings) == 0 {
		return fmt.Errorf("provider %q must declare at least one credential binding", p.ID)
	}
	if p.Credential.Kind == CredentialAWSSSOSession {
		if err := validateAWSWorkspaceProjection(p.WorkspaceProjections); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
	}
	if p.Credential.Kind == CredentialDatadogOAuthSession {
		if err := validateDatadogWorkspaceProjection(p.WorkspaceProjections); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
	}
	if p.Credential.Kind == CredentialOpenAICodexOAuthSession {
		if err := validateOpenAICodexWorkspaceProjection(p.WorkspaceProjections); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
	}
	if p.ID == "anthropic" || p.Acquisition.Helper == "claude-setup-token" {
		if err := validateAnthropicClaudePlan(p); err != nil {
			return fmt.Errorf("provider %q: %w", p.ID, err)
		}
	}
	return nil
}

func validateCredentialPlan(p Provider) error {
	switch p.SchemaVersion {
	case LegacyProviderSchemaVersion:
		if p.Credential.Kind != CredentialPrimarySecret {
			return fmt.Errorf("schema-v1 provider credential kind must be %q", CredentialPrimarySecret)
		}
		if len(p.SigningBindings) != 0 {
			return fmt.Errorf("schema-v1 provider cannot declare signing bindings")
		}
	case ProviderSchemaVersion:
		switch p.Credential.Kind {
		case CredentialAWSSSOSession:
			if p.ID != "aws" || p.Acquisition.Mode != AcquisitionBuiltinHelper || p.Acquisition.Helper != "aws-sso" {
				return fmt.Errorf("aws_sso_session is reserved for the reviewed aws/aws-sso built-in plan")
			}
			if len(p.HeaderBindings) != 0 || len(p.SigningBindings) != 1 ||
				p.SigningBindings[0].Kind != SigningBindingAWSSigV4 {
				return fmt.Errorf("aws_sso_session must declare exactly one aws_sigv4 signing binding and no header binding")
			}
		case CredentialDatadogOAuthSession:
			if p.ID != "datadog" || p.Acquisition.Mode != AcquisitionBuiltinHelper || p.Acquisition.Helper != "pup-oauth" {
				return fmt.Errorf("datadog_oauth_session is reserved for the reviewed datadog/pup-oauth built-in plan")
			}
			if len(p.HeaderBindings) != 1 || len(p.SigningBindings) != 0 {
				return fmt.Errorf("datadog_oauth_session must declare exactly one header binding and no signing binding")
			}
			binding := p.HeaderBindings[0]
			if binding.Target != (BindingTarget{Scheme: "https", Host: "api.datadoghq.com", Port: 443}) ||
				binding.Source.Header != "authorization" || len(binding.Source.Formats) != 1 ||
				binding.Source.Formats[0] != SourceFormatBearer ||
				binding.Destination.Header != "authorization" ||
				binding.Destination.Format != DestinationFormatBearer ||
				binding.Destination.SecretField != CredentialDatadogOAuthSession ||
				len(binding.SecretHeaders) != 1 || binding.SecretHeaders[0] != "authorization" {
				return fmt.Errorf("datadog_oauth_session binding does not match the reviewed Datadog US1 bearer contract")
			}
		case CredentialOpenAICodexOAuthSession:
			if p.ID != "openai" || p.DisplayName != "OpenAI account for Codex" ||
				p.Acquisition.Mode != AcquisitionBuiltinHelper || p.Acquisition.Helper != "codex-chatgpt-oauth" {
				return fmt.Errorf("openai_codex_oauth_session is reserved for the reviewed openai/codex-chatgpt-oauth built-in plan")
			}
			if len(p.HeaderBindings) != 1 || len(p.SigningBindings) != 0 {
				return fmt.Errorf("openai_codex_oauth_session must declare exactly one header binding and no signing binding")
			}
			binding := p.HeaderBindings[0]
			if binding.Target != (BindingTarget{Scheme: "https", Host: "chatgpt.com", Port: 443}) ||
				binding.Source.Header != "authorization" || len(binding.Source.Formats) != 1 ||
				binding.Source.Formats[0] != SourceFormatBearer ||
				binding.Destination.Header != "authorization" ||
				binding.Destination.Format != DestinationFormatBearer ||
				binding.Destination.SecretField != CredentialOpenAICodexOAuthSession ||
				strings.Join(binding.SecretHeaders, ",") != "authorization,chatgpt-account-id,x-openai-fedramp" {
				return fmt.Errorf("openai_codex_oauth_session binding does not match the reviewed Codex ChatGPT contract")
			}
		default:
			return fmt.Errorf("schema-v2 provider credential kind is invalid")
		}
	}
	return nil
}

const openAICodexWorkspaceAuthTemplate = `{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}","refresh_token":"","account_id":null},"last_refresh":"1970-01-01T00:00:00Z"}`

func validateOpenAICodexWorkspaceProjection(projections []WorkspaceProjection) error {
	if len(projections) != 1 {
		return fmt.Errorf("openai_codex_oauth_session must declare exactly the reviewed Codex auth-file projection")
	}
	projection := projections[0]
	if projection.Kind != WorkspaceProjectionCompleteFile || projection.Name != "" ||
		projection.Path != ".codex/auth.json" || projection.Template != openAICodexWorkspaceAuthTemplate {
		return fmt.Errorf("openai_codex_oauth_session projection does not match the reviewed Codex 0.146.0 contract")
	}
	return nil
}

func validateAnthropicClaudePlan(p Provider) error {
	if p.SchemaVersion != LegacyProviderSchemaVersion || p.ID != "anthropic" ||
		p.DisplayName != "Anthropic account for Claude Code" ||
		p.Acquisition != (Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "claude-setup-token"}) ||
		p.Credential.Kind != CredentialPrimarySecret || len(p.WorkspaceProjections) != 1 ||
		len(p.HeaderBindings) != 1 || len(p.SigningBindings) != 0 {
		return fmt.Errorf("Claude setup-token must use the reviewed schema-v1 Anthropic built-in plan")
	}
	projection := p.WorkspaceProjections[0]
	if projection.Kind != WorkspaceProjectionEnvironment || projection.Name != "CLAUDE_CODE_OAUTH_TOKEN" ||
		projection.Path != "" || projection.Template != "${HANDLE}" {
		return fmt.Errorf("Claude setup-token projection does not match the reviewed environment contract")
	}
	binding := p.HeaderBindings[0]
	if binding.Target != (BindingTarget{Scheme: "https", Host: "api.anthropic.com", Port: 443}) ||
		binding.Source.Header != "authorization" || len(binding.Source.Formats) != 1 ||
		binding.Source.Formats[0] != SourceFormatBearer ||
		binding.Destination.Header != "authorization" || binding.Destination.Format != DestinationFormatBearer ||
		binding.Destination.SecretField != CredentialPrimarySecret ||
		len(binding.SecretHeaders) != 1 || binding.SecretHeaders[0] != "authorization" {
		return fmt.Errorf("Claude setup-token binding does not match the reviewed Anthropic bearer contract")
	}
	return nil
}

func validateDatadogWorkspaceProjection(projections []WorkspaceProjection) error {
	want := map[string]string{
		"DD_ACCESS_TOKEN": "${HANDLE}",
		"DD_SITE":         "datadoghq.com",
	}
	if len(projections) != len(want) {
		return fmt.Errorf("datadog_oauth_session must declare exactly the reviewed pup environment projection")
	}
	seen := make(map[string]struct{}, len(projections))
	for _, projection := range projections {
		if projection.Kind != WorkspaceProjectionEnvironment || projection.Path != "" ||
			want[projection.Name] != projection.Template {
			return fmt.Errorf("datadog_oauth_session projection %q does not match the reviewed pup contract", projection.Name)
		}
		if _, duplicate := seen[projection.Name]; duplicate {
			return fmt.Errorf("datadog_oauth_session projection contains duplicate environment %q", projection.Name)
		}
		seen[projection.Name] = struct{}{}
	}
	return nil
}

func validateSigningBinding(binding SigningBinding) error {
	if err := binding.Kind.Validate(); err != nil {
		return err
	}
	if binding.Kind != SigningBindingAWSSigV4 || binding.AWSSigV4 == nil {
		return fmt.Errorf("aws_sigv4 signing binding requires exactly one aws_sigv4 contract")
	}
	return validateAWSSigV4Binding(*binding.AWSSigV4)
}

func validateAWSSigV4Binding(binding AWSSigV4Binding) error {
	if binding.Target.Scheme != "https" || binding.Target.Port != 443 {
		return fmt.Errorf("aws_sigv4 target must be exact https port 443")
	}
	wantSuffixes := map[string]struct{}{
		"amazonaws.com": {},
	}
	if len(binding.Target.DNSSuffixes) != len(wantSuffixes) {
		return fmt.Errorf("aws_sigv4 target must declare the reviewed AWS DNS suffixes")
	}
	seenSuffixes := make(map[string]struct{}, len(binding.Target.DNSSuffixes))
	for _, suffix := range binding.Target.DNSSuffixes {
		if _, allowed := wantSuffixes[suffix]; !allowed {
			return fmt.Errorf("aws_sigv4 target contains unreviewed DNS suffix %q", suffix)
		}
		if _, duplicate := seenSuffixes[suffix]; duplicate {
			return fmt.Errorf("aws_sigv4 target contains duplicate DNS suffix %q", suffix)
		}
		seenSuffixes[suffix] = struct{}{}
	}
	if binding.Source.AuthorizationHeader != "authorization" ||
		binding.Source.SecurityTokenHeader != "x-amz-security-token" {
		return fmt.Errorf("aws_sigv4 source headers must be the reviewed AWS authorization and security-token headers")
	}
	if len(binding.SecretHeaders) != 2 {
		return fmt.Errorf("aws_sigv4 secret_headers must contain authorization and x-amz-security-token")
	}
	wantHeaders := map[string]struct{}{
		"authorization":        {},
		"x-amz-security-token": {},
	}
	seenHeaders := make(map[string]struct{}, len(binding.SecretHeaders))
	for _, header := range binding.SecretHeaders {
		if err := validateHeaderName(header); err != nil {
			return fmt.Errorf("aws_sigv4 secret_headers: %w", err)
		}
		if _, allowed := wantHeaders[header]; !allowed {
			return fmt.Errorf("aws_sigv4 secret_headers contains unreviewed header %q", header)
		}
		if _, duplicate := seenHeaders[header]; duplicate {
			return fmt.Errorf("aws_sigv4 secret_headers contains duplicate header %q", header)
		}
		seenHeaders[header] = struct{}{}
	}
	return nil
}

func validateAWSWorkspaceProjection(projections []WorkspaceProjection) error {
	want := map[string]string{
		"AWS_ACCESS_KEY_ID":         "${HANDLE}",
		"AWS_SECRET_ACCESS_KEY":     "${HANDLE}",
		"AWS_SESSION_TOKEN":         "${HANDLE}",
		"AWS_EC2_METADATA_DISABLED": "true",
	}
	if len(projections) != len(want) {
		return fmt.Errorf("aws_sso_session must declare exactly the reviewed AWS CLI environment projection")
	}
	seen := make(map[string]struct{}, len(projections))
	for _, projection := range projections {
		if projection.Kind != WorkspaceProjectionEnvironment || projection.Path != "" {
			return fmt.Errorf("aws_sso_session projection must contain only environment variables")
		}
		template, ok := want[projection.Name]
		if !ok || projection.Template != template {
			return fmt.Errorf("aws_sso_session projection %q does not match the reviewed AWS CLI contract", projection.Name)
		}
		if _, duplicate := seen[projection.Name]; duplicate {
			return fmt.Errorf("aws_sso_session projection contains duplicate environment %q", projection.Name)
		}
		seen[projection.Name] = struct{}{}
	}
	return nil
}

func validateWorkspaceProjection(projection WorkspaceProjection) error {
	if err := projection.Kind.Validate(); err != nil {
		return err
	}
	switch projection.Kind {
	case WorkspaceProjectionEnvironment:
		if projection.Path != "" {
			return fmt.Errorf("env projection cannot declare path")
		}
		if !environmentName.MatchString(projection.Name) {
			return fmt.Errorf("env projection name is invalid")
		}
		if isReservedEnvironment(projection.Name) {
			return fmt.Errorf("env projection cannot replace Tobari or process control variable %q", projection.Name)
		}
		if len(projection.Template) > 4096 {
			return fmt.Errorf("env projection template exceeds 4096 bytes")
		}
		for _, character := range projection.Template {
			if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
				return fmt.Errorf("env projection template contains a control character")
			}
		}
	case WorkspaceProjectionCompleteFile:
		if projection.Name != "" {
			return fmt.Errorf("complete_file projection cannot declare name")
		}
		if err := ValidateRelativeHomePath(projection.Path); err != nil {
			return err
		}
		if len(projection.Template) > MaxTemplateBytes {
			return fmt.Errorf("complete_file projection template exceeds %d bytes", MaxTemplateBytes)
		}
		for _, character := range projection.Template {
			if character == '\x00' || character == '\r' ||
				(unicode.IsControl(character) && character != '\n' && character != '\t') ||
				character == '\u2028' || character == '\u2029' {
				return fmt.Errorf("complete_file projection template contains an unsafe control character")
			}
		}
	}
	return validateTemplate(projection.Template)
}

func ValidateRelativeHomePath(value string) error {
	if value == "" || len(value) > 240 || !utf8.ValidString(value) || strings.Contains(value, "\\") ||
		strings.ContainsRune(value, '\x00') || path.IsAbs(value) || path.Clean(value) != value {
		return fmt.Errorf("Workspace complete-file path must be a clean relative HOME path")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." || len(segment) > 128 ||
			!homePathSegment.MatchString(segment) {
			return fmt.Errorf("Workspace complete-file path contains an unsafe segment")
		}
	}
	return nil
}

func validateTemplate(value string) error {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("Workspace projection template is empty or invalid")
	}
	const maxPlaceholders = 32
	allowed := map[string]bool{
		"HANDLE":       true,
		"PROVIDER_ID":  true,
		"DISPLAY_NAME": true,
	}
	handleCount := 0
	placeholderCount := 0
	remaining := value
	for {
		start := strings.Index(remaining, "${")
		if start < 0 {
			break
		}
		end := strings.IndexByte(remaining[start+2:], '}')
		if end < 0 {
			return fmt.Errorf("Workspace projection template has an unterminated placeholder")
		}
		name := remaining[start+2 : start+2+end]
		if !allowed[name] {
			return fmt.Errorf("Workspace projection template uses unsupported placeholder %q", name)
		}
		placeholderCount++
		if placeholderCount > maxPlaceholders {
			return fmt.Errorf("Workspace projection template has too many placeholders")
		}
		if name == "HANDLE" {
			handleCount++
		}
		remaining = remaining[start+2+end+1:]
	}
	if strings.Contains(remaining, "${") {
		return fmt.Errorf("Workspace projection template has an unterminated placeholder")
	}
	if handleCount > 1 {
		return fmt.Errorf("Workspace projection template cannot contain ${HANDLE} more than once")
	}
	return nil
}

func validateHeaderBinding(binding HeaderBinding) error {
	if binding.Target.Scheme != "https" {
		return fmt.Errorf("header binding scheme must be exact https")
	}
	if err := validateExactHost(binding.Target.Host); err != nil {
		return err
	}
	if binding.Target.Port < 1 || binding.Target.Port > 65535 {
		return fmt.Errorf("header binding port must be in 1..65535")
	}
	if err := validateHeaderName(binding.Source.Header); err != nil {
		return fmt.Errorf("header binding source header: %w", err)
	}
	if len(binding.Source.Formats) == 0 || len(binding.Source.Formats) > 3 {
		return fmt.Errorf("header binding source must declare 1..3 formats")
	}
	seenFormats := make(map[SourceFormat]struct{}, len(binding.Source.Formats))
	for _, format := range binding.Source.Formats {
		if err := format.Validate(); err != nil {
			return err
		}
		if _, exists := seenFormats[format]; exists {
			return fmt.Errorf("header binding source contains duplicate format %q", format)
		}
		seenFormats[format] = struct{}{}
	}
	if err := validateHeaderName(binding.Destination.Header); err != nil {
		return fmt.Errorf("header binding destination header: %w", err)
	}
	if err := binding.Destination.Format.Validate(); err != nil {
		return err
	}
	if err := binding.Destination.SecretField.Validate(); err != nil {
		return fmt.Errorf("header binding destination secret_field: %w", err)
	}
	if binding.Destination.SecretField != CredentialPrimarySecret &&
		binding.Destination.SecretField != CredentialDatadogOAuthSession &&
		binding.Destination.SecretField != CredentialOpenAICodexOAuthSession {
		return fmt.Errorf("header binding destination must reference a reviewed header credential")
	}
	if len(binding.SecretHeaders) == 0 || len(binding.SecretHeaders) > maxSecretHeaders {
		return fmt.Errorf("header binding must declare 1..%d secret_headers", maxSecretHeaders)
	}
	seen := make(map[string]struct{}, len(binding.SecretHeaders))
	sourceHeader := strings.ToLower(binding.Source.Header)
	destinationHeader := strings.ToLower(binding.Destination.Header)
	containsSourceHeader := false
	containsDestinationHeader := false
	for _, header := range binding.SecretHeaders {
		if err := validateHeaderName(header); err != nil {
			return fmt.Errorf("secret_headers: %w", err)
		}
		normalized := strings.ToLower(header)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("secret_headers contains duplicate header %q", normalized)
		}
		seen[normalized] = struct{}{}
		containsSourceHeader = containsSourceHeader || normalized == sourceHeader
		containsDestinationHeader = containsDestinationHeader || normalized == destinationHeader
	}
	if !containsSourceHeader || !containsDestinationHeader {
		return fmt.Errorf("secret_headers must include the source and destination headers")
	}
	return nil
}

func validateExactHost(host string) error {
	if host == "" || len(host) > 253 || host != strings.ToLower(host) || strings.HasSuffix(host, ".") ||
		strings.ContainsAny(host, "*:/[]@?#") || strings.ContainsRune(host, '\x00') {
		return fmt.Errorf("header binding host must be one exact lowercase DNS name")
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return fmt.Errorf("header binding host must be a qualified DNS name")
	}
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("header binding host contains an invalid DNS label")
		}
		for _, character := range label {
			if !((character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-') {
				return fmt.Errorf("header binding host contains an invalid DNS label")
			}
		}
	}
	return nil
}

func validateHeaderName(name string) error {
	if name == "" || len(name) > 64 || name != strings.ToLower(name) {
		return fmt.Errorf("header name must be a bounded lowercase HTTP token")
	}
	for _, character := range name {
		if !isHTTPTokenCharacter(character) {
			return fmt.Errorf("header name must be a bounded lowercase HTTP token")
		}
	}
	if name == "host" || name == "content-length" || name == "proxy-authorization" ||
		name == "cookie" || name == "set-cookie" ||
		strings.HasPrefix(name, "x-tobari-") {
		return fmt.Errorf("header %q is reserved by the HTTP or Tobari control boundary", name)
	}
	return nil
}

func isHTTPTokenCharacter(character rune) bool {
	return (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') ||
		strings.ContainsRune("!#$%&'*+-.^_`|~", character)
}

func isReservedEnvironment(name string) bool {
	if strings.HasPrefix(name, "TOBARI_") {
		return true
	}
	switch name {
	case "HOME", "PATH", "SHELL", "USER", "LOGNAME", "HTTP_PROXY", "HTTPS_PROXY",
		"ALL_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR":
		return true
	default:
		return false
	}
}

func validateDisplayText(label, value string, maximum int) error {
	if value == "" || len(value) > maximum || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must be non-empty, trimmed, and at most %d bytes", label, maximum)
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return fmt.Errorf("%s contains an unsafe character", label)
		}
	}
	return nil
}
