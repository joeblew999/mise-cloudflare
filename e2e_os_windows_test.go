package e2e

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so killGroup ends it with everything it started (wrangler,
// workerd), not only mise: Windows' own, as e2e_os_test.go is elsewhere.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killGroup kills cmd and every process under it (taskkill's tree).
func killGroup(cmd *exec.Cmd) {
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

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
