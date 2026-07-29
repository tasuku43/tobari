//go:build windows

package dockerruntime

func currentIDs() (int, int) {
	return 1000, 1000
}
