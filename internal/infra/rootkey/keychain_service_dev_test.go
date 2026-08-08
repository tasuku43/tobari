//go:build tobari_dev

package rootkey

import "testing"

func TestDevelopmentKeychainServiceOverrideIsBounded(t *testing.T) {
	t.Setenv("TOBARI_TEST_KEYCHAIN_SERVICE", "io.tobari.integration.12345")
	service, err := runtimeKeychainService()
	if err != nil || service != "io.tobari.integration.12345" {
		t.Fatalf("development Keychain service = %q, err=%v", service, err)
	}
	t.Setenv("TOBARI_TEST_KEYCHAIN_SERVICE", "io.tobari.auth-root.v1")
	if _, err := runtimeKeychainService(); err == nil {
		t.Fatal("development override accepted a non-integration service")
	}
}
