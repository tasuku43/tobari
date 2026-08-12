package dockerruntime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tasuku43/tobari/internal/domain/authbroker"
)

func TestReviewedHostLoginDriverRegistryMatchesDomain(t *testing.T) {
	drivers := reviewedHostLoginDrivers()
	if err := validateReviewedHostLoginDrivers(drivers); err != nil {
		t.Fatal(err)
	}
	if _, found := reviewedHostLoginDriverForProvider(authbroker.BuiltinChatworkProviderID); found {
		t.Fatal("Chatwork entered the compiled host-login driver registry")
	}
}

func TestReviewedHostLoginDriverRegistryRejectsDrift(t *testing.T) {
	valid := reviewedHostLoginDrivers()
	mutate := func(index int, change func(*reviewedHostLoginDriver)) []reviewedHostLoginDriver {
		result := append([]reviewedHostLoginDriver(nil), valid...)
		change(&result[index])
		return result
	}
	tests := map[string]struct {
		drivers []reviewedHostLoginDriver
		want    string
	}{
		"missing driver": {
			drivers: append([]reviewedHostLoginDriver(nil), valid[:len(valid)-1]...),
			want:    "driver count",
		},
		"wrong order": {
			drivers: func() []reviewedHostLoginDriver {
				result := append([]reviewedHostLoginDriver(nil), valid...)
				result[0], result[1] = result[1], result[0]
				return result
			}(),
			want: "driver 0 provider",
		},
		"provider outside domain": {
			drivers: mutate(0, func(driver *reviewedHostLoginDriver) { driver.providerID = "example" }),
			want:    "driver 0 provider",
		},
		"executable missing": {
			drivers: mutate(0, func(driver *reviewedHostLoginDriver) { driver.executable = "" }),
			want:    "executable",
		},
		"executable reused": {
			drivers: mutate(1, func(driver *reviewedHostLoginDriver) { driver.executable = valid[0].executable }),
			want:    "duplicates executable",
		},
		"driver kind reused": {
			drivers: mutate(1, func(driver *reviewedHostLoginDriver) { driver.kind = valid[0].kind }),
			want:    "kind",
		},
		"persistence contract changed": {
			drivers: mutate(0, func(driver *reviewedHostLoginDriver) { driver.persistDriverDetails = true }),
			want:    "persist_driver_details",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateReviewedHostLoginDrivers(test.drivers)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateReviewedHostLoginDrivers() error = %v, want %q", err, test.want)
			}
		})
	}
}

func validateReviewedHostLoginDrivers(drivers []reviewedHostLoginDriver) error {
	expectedProviders := authbroker.ReviewedLoginProviderIDs()
	if len(drivers) != len(expectedProviders) {
		return fmt.Errorf("reviewed host-login driver count = %d, want %d", len(drivers), len(expectedProviders))
	}
	seenExecutables := make(map[string]struct{}, len(drivers))
	seenKinds := make(map[reviewedHostLoginDriverKind]struct{}, len(drivers))
	for index, driver := range drivers {
		providerID := expectedProviders[index]
		if driver.providerID != providerID {
			return fmt.Errorf("driver %d provider = %q, want %q", index, driver.providerID, providerID)
		}
		if _, found := authbroker.ReviewedLoginProviderHelper(providerID); !found {
			return fmt.Errorf("driver %q has no reviewed domain helper", providerID)
		}
		wantKind, wantExecutable, wantPersistence, found := reviewedHostLoginDriverContract(providerID)
		if !found {
			return fmt.Errorf("driver %q has no compiled contract", providerID)
		}
		if driver.kind != wantKind {
			return fmt.Errorf("driver %q kind = %d, want %d", providerID, driver.kind, wantKind)
		}
		if _, duplicate := seenExecutables[driver.executable]; duplicate {
			return fmt.Errorf("driver %q duplicates executable %q", providerID, driver.executable)
		}
		seenExecutables[driver.executable] = struct{}{}
		if driver.executable != wantExecutable {
			return fmt.Errorf("driver %q executable = %q, want %q", providerID, driver.executable, wantExecutable)
		}
		if _, duplicate := seenKinds[driver.kind]; duplicate {
			return fmt.Errorf("driver %q duplicates kind %d", providerID, driver.kind)
		}
		seenKinds[driver.kind] = struct{}{}
		if driver.persistDriverDetails != wantPersistence {
			return fmt.Errorf(
				"driver %q persist_driver_details = %t, want %t",
				providerID, driver.persistDriverDetails, wantPersistence,
			)
		}
	}
	return nil
}

func reviewedHostLoginDriverContract(
	providerID string,
) (reviewedHostLoginDriverKind, string, bool, bool) {
	switch providerID {
	case authbroker.BuiltinGitHubProviderID:
		return reviewedHostLoginDriverGitHub, "gh", false, true
	case authbroker.BuiltinAWSProviderID:
		return reviewedHostLoginDriverAWS, "aws", true, true
	case authbroker.BuiltinDatadogProviderID:
		return reviewedHostLoginDriverDatadog, "pup", true, true
	case authbroker.BuiltinOpenAIProviderID:
		return reviewedHostLoginDriverOpenAI, "codex", true, true
	case authbroker.BuiltinAnthropicProviderID:
		return reviewedHostLoginDriverAnthropic, "claude", false, true
	default:
		return 0, "", false, false
	}
}
