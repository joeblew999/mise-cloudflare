package main

import "syscall"

// longPath is p with Windows' long names (C:\Users\runneradmin, not C:\Users\RUNNER~1), as mise compares a ceiling.
func longPath(p string) string {
	u, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	n, err := syscall.GetLongPathName(u, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return p
	}
	return syscall.UTF16ToString(buf[:n])
}
