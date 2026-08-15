package authbroker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/capabilityprofile"
)

func TestBuiltinProviderVocabularyAndReviewedLoginOrderAreClosed(t *testing.T) {
	wantBuiltins := []string{"anthropic", "aws", "chatwork", "datadog", "github", "openai"}
	wantKnownLogin := []string{"github", "aws", "datadog", "openai", "anthropic"}
	wantLogin := []string{"github", "datadog", "openai", "anthropic"}
	if capabilityprofile.Compiled().IncludesExperimental() {
		wantLogin = wantKnownLogin
	}
	if got := BuiltinProviderIDs(); !reflect.DeepEqual(got, wantBuiltins) {
		t.Fatalf("BuiltinProviderIDs() = %v, want %v", got, wantBuiltins)
	}
	if got := ReviewedLoginProviderIDs(); !reflect.DeepEqual(got, wantLogin) {
		t.Fatalf("ReviewedLoginProviderIDs() = %v, want %v", got, wantLogin)
	}
	if got := KnownReviewedLoginProviderIDs(); !reflect.DeepEqual(got, wantKnownLogin) {
		t.Fatalf("KnownReviewedLoginProviderIDs() = %v, want %v", got, wantKnownLogin)
	}
	if SupportsReviewedLoginProvider(BuiltinChatworkProviderID) {
		t.Fatal("Chatwork entered the reviewed host-login union")
	}
	for _, providerID := range wantLogin {
		if !SupportsReviewedLoginProvider(providerID) {
			t.Fatalf("reviewed provider %q is unsupported", providerID)
		}
		if helper, found := ReviewedLoginProviderHelper(providerID); !found || helper == "" {
			t.Fatalf("reviewed provider %q helper = %q, found=%t", providerID, helper, found)
		}
	}
	if !capabilityprofile.Compiled().IncludesExperimental() && SupportsReviewedLoginProvider(BuiltinAWSProviderID) {
		t.Fatal("standard profile activated AWS login")
	}
	if SupportsReviewedLoginProvider("example") {
		t.Fatal("unknown provider entered the reviewed host-login union")
	}
}

func TestProviderRegistryAccessorsReturnIndependentCopies(t *testing.T) {
	builtins := BuiltinProviderIDs()
	login := ReviewedLoginProviderIDs()
	builtins[0] = "changed"
	login[0] = "changed"
	if BuiltinProviderIDs()[0] != BuiltinAnthropicProviderID {
		t.Fatal("caller mutated the domain-owned built-in provider registry")
	}
	if ReviewedLoginProviderIDs()[0] != BuiltinGitHubProviderID {
		t.Fatal("caller mutated the domain-owned login provider registry")
	}
}

func TestValidateBuiltinProviderCollectionRejectsRegistryDrift(t *testing.T) {
	valid := validBuiltinProviderCollection()
	if err := ValidateBuiltinProviderCollection(valid); err != nil {
		t.Fatalf("valid built-in collection: %v", err)
	}

	tests := map[string]struct {
		providers []Provider
		want      string
	}{
		"missing manifest": {
			providers: append([]Provider(nil), valid[1:]...),
			want:      "missing registered provider",
		},
		"unknown manifest": {
			providers: append(append([]Provider(nil), valid...), Provider{ID: "example"}),
			want:      "unregistered provider",
		},
		"duplicate manifest": {
			providers: append(append([]Provider(nil), valid...), valid[0]),
			want:      "duplicates provider",
		},
		"login helper missing": {
			providers: mutateBuiltinProvider(valid, BuiltinGitHubProviderID, func(provider *Provider) {
				provider.Acquisition = Acquisition{Mode: AcquisitionStdinImport}
			}),
			want: "reviewed login helper",
		},
		"login helper mismatch": {
			providers: mutateBuiltinProvider(valid, BuiltinAWSProviderID, func(provider *Provider) {
				provider.Acquisition.Helper = "github-gh"
			}),
			want: "reviewed login helper",
		},
		"non-login helper": {
			providers: mutateBuiltinProvider(valid, BuiltinChatworkProviderID, func(provider *Provider) {
				provider.Acquisition = Acquisition{Mode: AcquisitionBuiltinHelper, Helper: "chatwork-helper"}
			}),
			want: "outside the reviewed login registry",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateBuiltinProviderCollection(test.providers)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateBuiltinProviderCollection() error = %v, want %q", err, test.want)
			}
		})
	}
}

func validBuiltinProviderCollection() []Provider {
	providers := make([]Provider, 0, len(knownBuiltinProviderIDs))
	for _, providerID := range BuiltinProviderIDs() {
		acquisition := Acquisition{Mode: AcquisitionStdinImport}
		if helper, found := KnownReviewedLoginProviderHelper(providerID); found {
			acquisition = Acquisition{Mode: AcquisitionBuiltinHelper, Helper: helper}
		}
		providers = append(providers, Provider{ID: providerID, Acquisition: acquisition})
	}
	return providers
}

func mutateBuiltinProvider(providers []Provider, providerID string, mutate func(*Provider)) []Provider {
	result := append([]Provider(nil), providers...)
	for index := range result {
		if result[index].ID == providerID {
			mutate(&result[index])
			return result
		}
	}
	return result
}
