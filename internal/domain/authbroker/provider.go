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
	ProviderSchemaVersion    = 1
	ProviderSchemaID         = "tobari.auth-provider.v1"
	MaxProviderDocumentBytes = 64 * 1024
	MaxPrimarySecretBytes    = 32 * 1024
	MaxTemplateBytes         = 32 * 1024
	maxProviders             = 64
	maxProjections           = 32
	maxBindings              = 64
	maxSecretHeaders         = 32
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

const CredentialPrimarySecret CredentialKind = "primary_secret"

func (k CredentialKind) Validate() error {
	if k != CredentialPrimarySecret {
		return fmt.Errorf("provider credential kind must be %q", CredentialPrimarySecret)
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

// Provider is the parsed schema-v1 provider manifest. It contains no real
// credential, root key, or project-bound handle.
type Provider struct {
	SchemaVersion        int                   `json:"schema_version"`
	ID                   string                `json:"id"`
	DisplayName          string                `json:"display_name"`
	Acquisition          Acquisition           `json:"acquisition"`
	Credential           Credential            `json:"credential"`
	WorkspaceProjections []WorkspaceProjection `json:"workspace_projections"`
	HeaderBindings       []HeaderBinding       `json:"header_bindings"`
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
	if p.SchemaVersion != ProviderSchemaVersion {
		return fmt.Errorf("provider %q schema_version must be %d", p.ID, ProviderSchemaVersion)
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
	if len(p.HeaderBindings) == 0 || len(p.HeaderBindings) > maxBindings {
		return fmt.Errorf("provider %q must declare 1..%d header bindings", p.ID, maxBindings)
	}
	for index, binding := range p.HeaderBindings {
		if err := validateHeaderBinding(binding); err != nil {
			return fmt.Errorf("provider %q header binding %d: %w", p.ID, index, err)
		}
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
