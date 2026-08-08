//go:build !darwin && !linux

package rootkey

func EncryptedStateExists(string) (bool, error) { return false, ErrUnavailable }
func PrepareBrokerDirectories(string) error     { return ErrUnavailable }
