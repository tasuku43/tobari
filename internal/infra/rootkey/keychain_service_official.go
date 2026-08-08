//go:build !tobari_dev

package rootkey

func runtimeKeychainService() (string, error) {
	return keychainService, nil
}
