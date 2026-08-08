//go:build linux || darwin

package terminal

import "os"

// IsTerminal reports whether value is an actual host terminal. Character
// devices such as /dev/null are deliberately rejected by the termios probe.
func IsTerminal(value any) bool {
	file, ok := value.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	_, err = getTermios(file.Fd())
	return err == nil
}
