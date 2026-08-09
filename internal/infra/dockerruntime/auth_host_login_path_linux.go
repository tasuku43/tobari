//go:build linux

package dockerruntime

import (
	"os"
	"strconv"
)

func hostLoginInputPath(file *os.File) (string, error) {
	return os.Readlink("/proc/self/fd/" + strconv.FormatUint(uint64(file.Fd()), 10))
}
