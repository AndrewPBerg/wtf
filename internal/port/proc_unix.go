//go:build !windows

package port

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr sets Unix-specific process attributes to detach the child
// into its own process group so it survives after wtf exits.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGTERM to the process group (negative PID)
// to catch child processes spawned by the dev server.
func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
}
