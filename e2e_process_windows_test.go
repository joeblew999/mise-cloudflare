package e2e

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so killGroup ends it with everything it started (wrangler,
// workerd), not only mise: Windows' own, as e2e_process_test.go is elsewhere.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

// killGroup kills cmd and every process under it (taskkill's tree).
func killGroup(cmd *exec.Cmd) {
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
