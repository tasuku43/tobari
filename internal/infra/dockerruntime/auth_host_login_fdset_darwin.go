//go:build darwin

package dockerruntime

import "syscall"

const hostFDSetWordBits = 32

func hostFDSetCapacity() int {
	return len(syscall.FdSet{}.Bits) * hostFDSetWordBits
}

func hostFDSet(set *syscall.FdSet, fd int) {
	set.Bits[fd/hostFDSetWordBits] |= int32(1) << uint(fd%hostFDSetWordBits)
}

func hostSelect(fd int, readSet *syscall.FdSet, timeout *syscall.Timeval) (bool, error) {
	if err := syscall.Select(fd+1, readSet, nil, nil, timeout); err != nil {
		return false, err
	}
	return readSet.Bits[fd/hostFDSetWordBits]&(int32(1)<<uint(fd%hostFDSetWordBits)) != 0, nil
}
