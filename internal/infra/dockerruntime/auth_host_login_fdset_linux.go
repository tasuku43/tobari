//go:build linux

package dockerruntime

import "syscall"

const hostFDSetWordBits = 64

func hostFDSetCapacity() int {
	return len(syscall.FdSet{}.Bits) * hostFDSetWordBits
}

func hostFDSet(set *syscall.FdSet, fd int) {
	set.Bits[fd/hostFDSetWordBits] |= int64(1) << uint(fd%hostFDSetWordBits)
}

func hostSelect(fd int, readSet *syscall.FdSet, timeout *syscall.Timeval) (bool, error) {
	ready, err := syscall.Select(fd+1, readSet, nil, nil, timeout)
	return ready > 0, err
}
