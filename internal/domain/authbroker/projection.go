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

// Projection is the deterministic, collision-free provider view consumed by
// infrastructure. All slices are non-nil and ordered by stable identifiers.
type Projection struct {
	SchemaVersion  int                       `json:"schema_version"`
	Providers      []Provider                `json:"providers"`
	Environment    []EnvironmentProjection   `json:"environment"`
	CompleteFiles  []CompleteFileProjection  `json:"complete_files"`
	HeaderBindings []NormalizedHeaderBinding `json:"header_bindings"`
	SecretHeaders  []string                  `json:"secret_headers"`
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
		sort.Slice(normalized[index].WorkspaceProjections, func(a, b int) bool {
			left := normalized[index].WorkspaceProjections[a]
			right := normalized[index].WorkspaceProjections[b]
			return projectionKey(left) < projectionKey(right)
		})
		sort.Slice(normalized[index].HeaderBindings, func(a, b int) bool {
			return bindingKey(normalized[index].HeaderBindings[a]) < bindingKey(normalized[index].HeaderBindings[b])
		})
	}
	sort.Slice(normalized, func(a, b int) bool { return normalized[a].ID < normalized[b].ID })

	projection := Projection{
		SchemaVersion:  ProviderSchemaVersion,
		Providers:      normalized,
		Environment:    []EnvironmentProjection{},
		CompleteFiles:  []CompleteFileProjection{},
		HeaderBindings: []NormalizedHeaderBinding{},
		SecretHeaders:  []string{},
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

func cloneProvider(provider Provider) Provider {
	clone := provider
	clone.WorkspaceProjections = append([]WorkspaceProjection(nil), provider.WorkspaceProjections...)
	clone.HeaderBindings = make([]HeaderBinding, len(provider.HeaderBindings))
	for index, binding := range provider.HeaderBindings {
		clone.HeaderBindings[index] = binding
		clone.HeaderBindings[index].Source.Formats = append([]SourceFormat(nil), binding.Source.Formats...)
		clone.HeaderBindings[index].SecretHeaders = append([]string(nil), binding.SecretHeaders...)
	}
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
