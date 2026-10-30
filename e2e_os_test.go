//go:build !windows

package e2e

import (
	"os/exec"
	"syscall"
)

// ownGroup starts cmd in a process group of its own, so killGroup ends it with everything it started (wrangler,
// workerd), not only mise. e2e_os_windows_test.go is the same on Windows.
func ownGroup(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }

// killGroup kills cmd's process group.
func killGroup(cmd *exec.Cmd) { syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

// longPath is p as is: only Windows has short names (e2e_os_windows_test.go).
func longPath(p string) string { return p }
