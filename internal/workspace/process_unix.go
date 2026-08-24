//go:build !windows

package workspace

import (
	"os/exec"
	"syscall"
)

func isolateProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func stopProcess(pid int) error {
	return syscall.Kill(-pid, syscall.SIGTERM)
}
