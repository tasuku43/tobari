//go:build tobari_dev

package rootkey

import (
	"fmt"
	"os"
	"regexp"
)

var integrationKeychainServicePattern = regexp.MustCompile(`^io\.tobari\.integration\.[0-9]{1,20}$`)

func runtimeKeychainService() (string, error) {
	service := os.Getenv("TOBARI_TEST_KEYCHAIN_SERVICE")
	if service == "" {
		return keychainService, nil
	}
	if !integrationKeychainServicePattern.MatchString(service) {
		return "", fmt.Errorf("test Keychain service is invalid")
	}
	return service, nil
}
