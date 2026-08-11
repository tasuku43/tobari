//go:build !darwin && !linux

package credentialhost

// The reviewed host acquisition boundary currently requires Unix O_NOFOLLOW
// and owner-UID checks. Other platforms fail closed until an equivalent file
// identity contract is implemented and tested.
func requirePrivateCodexDirectory(string) error {
	return ErrCodexLoginSetup
}

func readPrivateCodexAuthFile(string, string, int64) ([]byte, error) {
	return nil, ErrCodexAuthCapture
}
