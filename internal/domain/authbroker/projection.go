package authbroker

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrAmbiguousHTTPBinding identifies a provider collection whose HTTP handle
// recognition ranges overlap. Callers may expose a stable recovery fault
// without parsing validation error text.
var ErrAmbiguousHTTPBinding = errors.New("ambiguous provider HTTP binding")

type EnvironmentProjection struct {
	ProviderID string `json:"provider_id"`
	Name       string `json:"name"`
	Template   string `json:"template"`
}

type CompleteFileProjection struct {
	ProviderID string `json:"provider_id"`
	Path       string `json:"path"`
	Template   string `json:"template"`
}

type NormalizedHeaderBinding struct {
	ProviderID    string                  `json:"provider_id"`
	Target        BindingTarget           `json:"target"`
	Source        NormalizedBindingSource `json:"source"`
	Destination   BindingDestination      `json:"destination"`
	SecretHeaders []string                `json:"secret_headers"`
}

type NormalizedSigningBinding struct {
	ProviderID string             `json:"provider_id"`
	Kind       SigningBindingKind `json:"kind"`
	AWSSigV4   *AWSSigV4Binding   `json:"aws_sigv4,omitempty"`
}

// Projection is the deterministic, collision-free provider view consumed by
// infrastructure. All slices are non-nil and ordered by stable identifiers.
type Projection struct {
	SchemaVersion   int                        `json:"schema_version"`
	Providers       []Provider                 `json:"providers"`
	Environment     []EnvironmentProjection    `json:"environment"`
	CompleteFiles   []CompleteFileProjection   `json:"complete_files"`
	HeaderBindings  []NormalizedHeaderBinding  `json:"header_bindings"`
	SigningBindings []NormalizedSigningBinding `json:"signing_bindings"`
	SecretHeaders   []string                   `json:"secret_headers"`
}

// NormalizeProviders validates and deep-copies provider manifests, canonicalizes
// HTTP names, and rejects any target or recognition ambiguity across providers.
func NormalizeProviders(providers []Provider) (Projection, error) {
	if len(providers) == 0 || len(providers) > maxProviders {
		return Projection{}, fmt.Errorf("provider collection must contain 1..%d providers", maxProviders)
	}
	normalized := make([]Provider, len(providers))
	for index, provider := range providers {
		if err := validateProvider(provider); err != nil {
			return Projection{}, err
		}
		normalized[index] = cloneProvider(provider)
		for bindingIndex := range normalized[index].HeaderBindings {
			binding := &normalized[index].HeaderBindings[bindingIndex]
			binding.Target.Host = strings.ToLower(binding.Target.Host)
			binding.Source.Header = strings.ToLower(binding.Source.Header)
			binding.Destination.Header = strings.ToLower(binding.Destination.Header)
			sort.Slice(binding.Source.Formats, func(a, b int) bool {
				return binding.Source.Formats[a] < binding.Source.Formats[b]
			})
			for headerIndex := range binding.SecretHeaders {
				binding.SecretHeaders[headerIndex] = strings.ToLower(binding.SecretHeaders[headerIndex])
			}
			sort.Strings(binding.SecretHeaders)
		}
		for bindingIndex := range normalized[index].SigningBindings {
			binding := &normalized[index].SigningBindings[bindingIndex]
			if binding.AWSSigV4 != nil {
				sort.Strings(binding.AWSSigV4.Target.DNSSuffixes)
				sort.Strings(binding.AWSSigV4.SecretHeaders)
			}
		}
		sort.Slice(normalized[index].WorkspaceProjections, func(a, b int) bool {
			left := normalized[index].WorkspaceProjections[a]
			right := normalized[index].WorkspaceProjections[b]
			return projectionKey(left) < projectionKey(right)
		})
		sort.Slice(normalized[index].HeaderBindings, func(a, b int) bool {
			return bindingKey(normalized[index].HeaderBindings[a]) < bindingKey(normalized[index].HeaderBindings[b])
		})
		sort.Slice(normalized[index].SigningBindings, func(a, b int) bool {
			return signingBindingKey(normalized[index].SigningBindings[a]) < signingBindingKey(normalized[index].SigningBindings[b])
		})
	}
	sort.Slice(normalized, func(a, b int) bool { return normalized[a].ID < normalized[b].ID })

	projection := Projection{
		SchemaVersion:   ProviderSchemaVersion,
		Providers:       normalized,
		Environment:     []EnvironmentProjection{},
		CompleteFiles:   []CompleteFileProjection{},
		HeaderBindings:  []NormalizedHeaderBinding{},
		SigningBindings: []NormalizedSigningBinding{},
		SecretHeaders:   []string{},
	}
	providerIDs := make(map[string]struct{}, len(normalized))
	displayNames := make(map[string]string, len(normalized))
	environmentOwners := make(map[string]string)
	fileOwners := make(map[string]string)
	bindingOwners := make(map[string][]bindingOwner)
	secretHeaders := make(map[string]struct{})
	for _, provider := range normalized {
		if _, exists := providerIDs[provider.ID]; exists {
			return Projection{}, fmt.Errorf("provider ID %q is duplicated", provider.ID)
		}
		providerIDs[provider.ID] = struct{}{}
		displayKey := strings.ToLower(provider.DisplayName)
		if owner, exists := displayNames[displayKey]; exists {
			return Projection{}, fmt.Errorf("providers %q and %q have an ambiguous display_name", owner, provider.ID)
		}
		displayNames[displayKey] = provider.ID
		for _, item := range provider.WorkspaceProjections {
			switch item.Kind {
			case WorkspaceProjectionEnvironment:
				if owner, exists := environmentOwners[item.Name]; exists {
					return Projection{}, fmt.Errorf("providers %q and %q collide on Workspace env %q", owner, provider.ID, item.Name)
				}
				environmentOwners[item.Name] = provider.ID
				projection.Environment = append(projection.Environment, EnvironmentProjection{
					ProviderID: provider.ID, Name: item.Name, Template: item.Template,
				})
			case WorkspaceProjectionCompleteFile:
				if owner, exists := fileOwners[item.Path]; exists {
					return Projection{}, fmt.Errorf("providers %q and %q collide on Workspace file %q", owner, provider.ID, item.Path)
				}
				fileOwners[item.Path] = provider.ID
				projection.CompleteFiles = append(projection.CompleteFiles, CompleteFileProjection{
					ProviderID: provider.ID, Path: item.Path, Template: item.Template,
				})
			}
		}
		for _, binding := range provider.HeaderBindings {
			key := recognitionKey(binding)
			for _, sourceFormat := range binding.Source.Formats {
				for _, owner := range bindingOwners[key] {
					if sourceFormatsOverlap(owner.format, sourceFormat) {
						return Projection{}, fmt.Errorf("%w: providers %q and %q overlap for %s", ErrAmbiguousHTTPBinding, owner.provider, provider.ID, key)
					}
				}
				bindingOwners[key] = append(bindingOwners[key], bindingOwner{provider: provider.ID, format: sourceFormat})
				secrets := append([]string(nil), binding.SecretHeaders...)
				projection.HeaderBindings = append(projection.HeaderBindings, NormalizedHeaderBinding{
					ProviderID: provider.ID, Target: binding.Target,
					Source:      NormalizedBindingSource{Header: binding.Source.Header, Format: sourceFormat},
					Destination: binding.Destination, SecretHeaders: secrets,
				})
				for _, header := range secrets {
					secretHeaders[header] = struct{}{}
				}
			}
		}
		for _, binding := range provider.SigningBindings {
			var aws *AWSSigV4Binding
			if binding.AWSSigV4 != nil {
				value := cloneAWSSigV4Binding(*binding.AWSSigV4)
				aws = &value
				for _, header := range value.SecretHeaders {
					secretHeaders[header] = struct{}{}
				}
			}
			projection.SigningBindings = append(projection.SigningBindings, NormalizedSigningBinding{
				ProviderID: provider.ID,
				Kind:       binding.Kind,
				AWSSigV4:   aws,
			})
		}
	}
	if err := validateNormalizedBindingCollisions(projection.HeaderBindings, projection.SigningBindings); err != nil {
		return Projection{}, err
	}
	for header := range secretHeaders {
		projection.SecretHeaders = append(projection.SecretHeaders, header)
	}
	sort.Slice(projection.Environment, func(a, b int) bool {
		return projection.Environment[a].Name < projection.Environment[b].Name
	})
	sort.Slice(projection.CompleteFiles, func(a, b int) bool {
		return projection.CompleteFiles[a].Path < projection.CompleteFiles[b].Path
	})
	sort.Slice(projection.HeaderBindings, func(a, b int) bool {
		left, right := projection.HeaderBindings[a], projection.HeaderBindings[b]
		leftKey := fmt.Sprintf("%s\x00%s\x00%05d\x00%s\x00%s\x00%s\x00%s", left.ProviderID, left.Target.Host, left.Target.Port, left.Source.Header, left.Source.Format, left.Destination.Header, left.Destination.Format)
		rightKey := fmt.Sprintf("%s\x00%s\x00%05d\x00%s\x00%s\x00%s\x00%s", right.ProviderID, right.Target.Host, right.Target.Port, right.Source.Header, right.Source.Format, right.Destination.Header, right.Destination.Format)
		return leftKey < rightKey
	})
	sort.Slice(projection.SigningBindings, func(a, b int) bool {
		left, right := projection.SigningBindings[a], projection.SigningBindings[b]
		return left.ProviderID+"\x00"+normalizedSigningBindingKey(left) <
			right.ProviderID+"\x00"+normalizedSigningBindingKey(right)
	})
	sort.Strings(projection.SecretHeaders)
	return projection, nil
}

type bindingOwner struct {
	provider string
	format   SourceFormat
}

func sourceFormatsOverlap(left, right SourceFormat) bool {
	return left == right || left == SourceFormatRaw || right == SourceFormatRaw
}

func recognitionKey(binding HeaderBinding) string {
	return fmt.Sprintf("%s://%s:%d %s", binding.Target.Scheme, strings.ToLower(binding.Target.Host), binding.Target.Port, strings.ToLower(binding.Source.Header))
}

func projectionKey(item WorkspaceProjection) string {
	target := item.Name
	if item.Kind == WorkspaceProjectionCompleteFile {
		target = item.Path
	}
	return string(item.Kind) + "\x00" + target
}

func bindingKey(binding HeaderBinding) string {
	formats := make([]string, len(binding.Source.Formats))
	for index, format := range binding.Source.Formats {
		formats[index] = string(format)
	}
	return fmt.Sprintf("%s\x00%s\x00%05d\x00%s\x00%s\x00%s\x00%s", binding.Target.Scheme, binding.Target.Host, binding.Target.Port, binding.Source.Header, strings.Join(formats, ","), binding.Destination.Header, binding.Destination.Format)
}

func signingBindingKey(binding SigningBinding) string {
	if binding.AWSSigV4 == nil {
		return string(binding.Kind)
	}
	return string(binding.Kind) + "\x00" + strings.Join(binding.AWSSigV4.Target.DNSSuffixes, ",")
}

func normalizedSigningBindingKey(binding NormalizedSigningBinding) string {
	return signingBindingKey(SigningBinding{Kind: binding.Kind, AWSSigV4: binding.AWSSigV4})
}

func validateNormalizedBindingCollisions(
	headers []NormalizedHeaderBinding,
	signing []NormalizedSigningBinding,
) error {
	for index, left := range signing {
		if left.AWSSigV4 == nil {
			continue
		}
		for _, right := range signing[index+1:] {
			if right.AWSSigV4 == nil {
				continue
			}
			for _, leftSuffix := range left.AWSSigV4.Target.DNSSuffixes {
				for _, rightSuffix := range right.AWSSigV4.Target.DNSSuffixes {
					if dnsSuffixesOverlap(leftSuffix, rightSuffix) {
						return fmt.Errorf(
							"%w: providers %q and %q overlap for AWS SigV4 DNS suffixes",
							ErrAmbiguousHTTPBinding, left.ProviderID, right.ProviderID,
						)
					}
				}
			}
		}
		for _, header := range headers {
			if header.Target.Scheme != left.AWSSigV4.Target.Scheme ||
				header.Target.Port != left.AWSSigV4.Target.Port ||
				header.Source.Header != left.AWSSigV4.Source.AuthorizationHeader {
				continue
			}
			for _, suffix := range left.AWSSigV4.Target.DNSSuffixes {
				if hostMatchesDNSSuffix(header.Target.Host, suffix) {
					return fmt.Errorf(
						"%w: providers %q and %q overlap for AWS SigV4 authority %q",
						ErrAmbiguousHTTPBinding, left.ProviderID, header.ProviderID, header.Target.Host,
					)
				}
			}
		}
	}
	return nil
}

func dnsSuffixesOverlap(left, right string) bool {
	return hostMatchesDNSSuffix(left, right) || hostMatchesDNSSuffix(right, left)
}

func hostMatchesDNSSuffix(host, suffix string) bool {
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

func cloneProvider(provider Provider) Provider {
	clone := provider
	clone.WorkspaceProjections = append([]WorkspaceProjection(nil), provider.WorkspaceProjections...)
	clone.HeaderBindings = make([]HeaderBinding, len(provider.HeaderBindings))
	for index, binding := range provider.HeaderBindings {
		clone.HeaderBindings[index] = binding
		clone.HeaderBindings[index].Source.Formats = append([]SourceFormat(nil), binding.Source.Formats...)
		clone.HeaderBindings[index].SecretHeaders = append([]string(nil), binding.SecretHeaders...)
	}
	clone.SigningBindings = make([]SigningBinding, len(provider.SigningBindings))
	for index, binding := range provider.SigningBindings {
		clone.SigningBindings[index] = binding
		if binding.AWSSigV4 != nil {
			value := cloneAWSSigV4Binding(*binding.AWSSigV4)
			clone.SigningBindings[index].AWSSigV4 = &value
		}
	}
	return clone
}

func cloneAWSSigV4Binding(binding AWSSigV4Binding) AWSSigV4Binding {
	clone := binding
	clone.Target.DNSSuffixes = append([]string(nil), binding.Target.DNSSuffixes...)
	clone.SecretHeaders = append([]string(nil), binding.SecretHeaders...)
	return clone
}

// NormalizedBindings returns the deterministic one-source-format bindings for
// one provider, ready for strict JSON marshaling to the Gateway and broker.
func (p Provider) NormalizedBindings() ([]NormalizedHeaderBinding, error) {
	projection, err := NormalizeProviders([]Provider{p})
	if err != nil {
		return nil, err
	}
	return append([]NormalizedHeaderBinding(nil), projection.HeaderBindings...), nil
}

// NormalizedSigningBindings returns the deterministic reviewed behavioral
// bindings for one provider without exposing credential material.
func (p Provider) NormalizedSigningBindings() ([]NormalizedSigningBinding, error) {
	projection, err := NormalizeProviders([]Provider{p})
	if err != nil {
		return nil, err
	}
	bindings := make([]NormalizedSigningBinding, len(projection.SigningBindings))
	for index, binding := range projection.SigningBindings {
		bindings[index] = binding
		if binding.AWSSigV4 != nil {
			value := cloneAWSSigV4Binding(*binding.AWSSigV4)
			bindings[index].AWSSigV4 = &value
		}
	}
	return bindings, nil
}
